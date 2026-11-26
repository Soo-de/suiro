package sets

import (
	"cmp"
	"encoding/binary"
	"math/bits"
	"net/netip"
	"slices"

	"github.com/Soo-de/suiro/internal/model"
)

// interval is the inclusive run of values lo..hi, with lo ≤ hi.
type interval struct{ lo, hi uint32 }

// intervals is a set of values in canonical form: sorted, and no two
// intervals overlap or touch. It is never changed once built.
type intervals []interval

// normalize returns the union of in, which may be in any order; it reorders
// in and reuses its storage. O(n log n) in len(in).
func normalize(in []interval) intervals {
	slices.SortFunc(in, func(a, b interval) int { return cmp.Compare(a.lo, b.lo) })
	out := intervals(in[:0])
	for _, v := range in {
		out = out.extend(v)
	}
	return out
}

// extend appends v, which must not start before the last interval does, and
// merges the two if they overlap or touch.
func (s intervals) extend(v interval) intervals {
	n := len(s)
	// The subtraction runs only when v starts after the last end, so it cannot
	// wrap around.
	if n == 0 || v.lo > s[n-1].hi && v.lo-s[n-1].hi > 1 {
		return append(s, v)
	}
	s[n-1].hi = max(s[n-1].hi, v.hi)
	return s
}

// union returns the values in s or o, in O(len(s) + len(o)).
func (s intervals) union(o intervals) intervals {
	out := make(intervals, 0, len(s)+len(o))
	for len(s) > 0 && len(o) > 0 {
		if s[0].lo <= o[0].lo {
			out, s = out.extend(s[0]), s[1:]
		} else {
			out, o = out.extend(o[0]), o[1:]
		}
	}
	// At most one of the two still has intervals.
	for _, v := range s {
		out = out.extend(v)
	}
	for _, v := range o {
		out = out.extend(v)
	}
	return out
}

// intersect returns the values in both s and o, in O(len(s) + len(o)).
func (s intervals) intersect(o intervals) intervals {
	out := make(intervals, 0, len(s)+len(o))
	for len(s) > 0 && len(o) > 0 {
		if lo, hi := max(s[0].lo, o[0].lo), min(s[0].hi, o[0].hi); lo <= hi {
			out = append(out, interval{lo, hi})
		}
		// The interval that ends first cannot meet anything further on.
		if s[0].hi < o[0].hi {
			s = s[1:]
		} else {
			o = o[1:]
		}
	}
	return out
}

// diff returns the values in s but not in o, in O(len(s) + len(o)): the
// nested loops only ever move forward through o.
func (s intervals) diff(o intervals) intervals {
	out := make(intervals, 0, len(s)+len(o))
	for _, v := range s {
		for len(o) > 0 && o[0].hi < v.lo {
			o = o[1:]
		}
		// Cut every overlapping interval of o out of v. One that reaches the
		// end of v stays in o, because it may cut the next interval of s too.
		covered := false
		for len(o) > 0 && o[0].lo <= v.hi {
			if o[0].lo > v.lo {
				out = append(out, interval{v.lo, o[0].lo - 1})
			}
			if o[0].hi >= v.hi {
				covered = true
				break
			}
			v.lo = o[0].hi + 1
			o = o[1:]
		}
		if !covered {
			out = append(out, v)
		}
	}
	return out
}

// contains reports whether x is in s, in O(log len(s)).
func (s intervals) contains(x uint32) bool {
	// i is the first interval that does not end before x.
	i, _ := slices.BinarySearchFunc(s, x, func(v interval, x uint32) int { return cmp.Compare(v.hi, x) })
	return i < len(s) && s[i].lo <= x
}

// intervalSpace builds sets of sorted interval lists.
type intervalSpace struct{}

func (intervalSpace) Ports(ranges ...model.PortRange) GateSet {
	var tcp, udp []interval
	for _, r := range ranges {
		if r.From > r.To {
			continue
		}
		v := interval{uint32(r.From), uint32(r.To)}
		if r.Protocol == model.ProtocolTCP || r.Protocol == model.ProtocolAll {
			tcp = append(tcp, v)
		}
		if r.Protocol == model.ProtocolUDP || r.Protocol == model.ProtocolAll {
			udp = append(udp, v)
		}
	}
	return portSet{tcp: normalize(tcp), udp: normalize(udp)}
}

func (intervalSpace) Addrs(prefixes ...netip.Prefix) AddrSet {
	in := make([]interval, 0, len(prefixes))
	for _, p := range prefixes {
		if !p.IsValid() || !p.Addr().Is4() {
			continue
		}
		lo := addrValue(p.Masked().Addr())
		// Setting every host bit gives the last address; a /32 has none.
		in = append(in, interval{lo, lo | ^uint32(0)>>p.Bits()})
	}
	return addrSet{normalize(in)}
}

// portSet keeps the ports of each protocol apart. With n and m the
// intervals of two sets over both protocols, Union, Intersect and Diff are
// O(n + m), Contains is O(log n) and Ranges is O(n).
type portSet struct{ tcp, udp intervals }

func (s portSet) Intersect(o GateSet) GateSet {
	t := o.(portSet)
	return portSet{tcp: s.tcp.intersect(t.tcp), udp: s.udp.intersect(t.udp)}
}

func (s portSet) Union(o GateSet) GateSet {
	t := o.(portSet)
	return portSet{tcp: s.tcp.union(t.tcp), udp: s.udp.union(t.udp)}
}

func (s portSet) Diff(o GateSet) GateSet {
	t := o.(portSet)
	return portSet{tcp: s.tcp.diff(t.tcp), udp: s.udp.diff(t.udp)}
}

func (s portSet) IsEmpty() bool { return len(s.tcp) == 0 && len(s.udp) == 0 }

func (s portSet) Contains(proto model.Protocol, port uint16) bool {
	switch proto {
	case model.ProtocolTCP:
		return s.tcp.contains(uint32(port))
	case model.ProtocolUDP:
		return s.udp.contains(uint32(port))
	default:
		return false
	}
}

func (s portSet) Ranges() []model.PortRange {
	out := make([]model.PortRange, 0, len(s.tcp)+len(s.udp))
	out = appendRanges(out, model.ProtocolTCP, s.tcp)
	return appendRanges(out, model.ProtocolUDP, s.udp)
}

func appendRanges(out []model.PortRange, proto model.Protocol, s intervals) []model.PortRange {
	for _, v := range s {
		// A port set's intervals are built from uint16 bounds and stay within them.
		out = append(out, model.PortRange{Protocol: proto, From: uint16(v.lo), To: uint16(v.hi)}) //nolint:gosec
	}
	return out
}

// addrSet holds IPv4 addresses as their 32-bit values. With n and m the
// intervals of two sets, Union, Intersect and Diff are O(n + m), Contains
// is O(log n) and Prefixes is O(n), at most 62 prefixes per interval.
type addrSet struct{ addrs intervals }

func (s addrSet) Intersect(o AddrSet) AddrSet {
	return addrSet{s.addrs.intersect(o.(addrSet).addrs)}
}

func (s addrSet) Union(o AddrSet) AddrSet {
	return addrSet{s.addrs.union(o.(addrSet).addrs)}
}

func (s addrSet) Diff(o AddrSet) AddrSet {
	return addrSet{s.addrs.diff(o.(addrSet).addrs)}
}

func (s addrSet) IsEmpty() bool { return len(s.addrs) == 0 }

func (s addrSet) Contains(a netip.Addr) bool {
	return a.Is4() && s.addrs.contains(addrValue(a))
}

func (s addrSet) Prefixes() []netip.Prefix {
	out := make([]netip.Prefix, 0, len(s.addrs))
	// No prefix of the cover spans two intervals, because they do not touch.
	for _, v := range s.addrs {
		out = appendPrefixes(out, v)
	}
	return out
}

// appendPrefixes appends the shortest sorted list of prefixes covering v,
// at most 62 of them.
func appendPrefixes(out []netip.Prefix, v interval) []netip.Prefix {
	// 64 bits, so that stepping past the last address does not wrap around.
	for lo, end := uint64(v.lo), uint64(v.hi)+1; lo < end; {
		// The largest block starting at lo is limited by lo's alignment and by
		// the number of addresses left.
		host := min(bits.TrailingZeros32(uint32(lo)), bits.Len64(end-lo)-1) //nolint:gosec // lo ≤ v.hi
		out = append(out, netip.PrefixFrom(valueAddr(uint32(lo)), 32-host)) //nolint:gosec // lo ≤ v.hi
		lo += 1 << host
	}
	return out
}

// addrValue returns the 32-bit value of a, which must be an IPv4 address.
func addrValue(a netip.Addr) uint32 {
	b := a.As4()
	return binary.BigEndian.Uint32(b[:])
}

// valueAddr is the inverse of addrValue.
func valueAddr(v uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}
