package sets_test

import (
	"encoding/binary"
	"net/netip"
	"slices"
	"testing"

	"github.com/Soo-de/suiro/internal/core/sets"
	"github.com/Soo-de/suiro/internal/model"
)

// Expected port sets follow the Kubernetes NetworkPolicy port rules: a rule's
// port and endPort are an inclusive range of one protocol, and the ports of
// several rules are additive.

func tcp(from, to uint16) model.PortRange {
	return model.PortRange{Protocol: model.ProtocolTCP, From: from, To: to}
}

func udp(from, to uint16) model.PortRange {
	return model.PortRange{Protocol: model.ProtocolUDP, From: from, To: to}
}

func both(from, to uint16) model.PortRange {
	return model.PortRange{Protocol: model.ProtocolAll, From: from, To: to}
}

type ranges = []model.PortRange

func checkPortSet(t *testing.T, name string, got sets.GateSet, want ranges) {
	t.Helper()
	if rs := got.Ranges(); !slices.Equal(rs, want) {
		t.Errorf("%s: Ranges() = %v, want %v", name, rs, want)
	}
	if got.IsEmpty() != (len(want) == 0) {
		t.Errorf("%s: IsEmpty() = %v, want %v", name, got.IsEmpty(), len(want) == 0)
	}
}

func TestPortSetConstruction(t *testing.T) {
	tests := []struct {
		name string
		in   ranges
		want ranges
	}{
		{"no ranges is empty", nil, nil},
		{"single port", ranges{tcp(6379, 6379)}, ranges{tcp(6379, 6379)}},
		{"port with endPort", ranges{tcp(32000, 32768)}, ranges{tcp(32000, 32768)}},
		{"single UDP port", ranges{udp(53, 53)}, ranges{udp(53, 53)}},
		{"ProtocolAll is TCP and UDP", ranges{both(53, 53)}, ranges{tcp(53, 53), udp(53, 53)}},
		{"all ports", ranges{model.AllPorts()}, ranges{tcp(0, 65535), udp(0, 65535)}},
		{"touching ports merge", ranges{tcp(80, 80), tcp(81, 81)}, ranges{tcp(80, 81)}},
		{"touching ranges merge in reverse order", ranges{tcp(90, 99), tcp(80, 89)}, ranges{tcp(80, 99)}},
		{"overlapping ranges merge", ranges{tcp(80, 90), tcp(85, 95)}, ranges{tcp(80, 95)}},
		{"nested range is absorbed", ranges{tcp(80, 90), tcp(83, 84)}, ranges{tcp(80, 90)}},
		{"duplicates collapse", ranges{udp(53, 53), udp(53, 53)}, ranges{udp(53, 53)}},
		{"gap of one port stays two", ranges{tcp(80, 80), tcp(82, 82)}, ranges{tcp(80, 80), tcp(82, 82)}},
		{"one range bridges two", ranges{tcp(10, 20), tcp(30, 40), tcp(21, 29)}, ranges{tcp(10, 40)}},
		{
			"sorted with TCP first",
			ranges{udp(1, 1), tcp(443, 443), udp(500, 500), tcp(80, 80)},
			ranges{tcp(80, 80), tcp(443, 443), udp(1, 1), udp(500, 500)},
		},
		{"protocols never merge", ranges{udp(81, 81), tcp(80, 80)}, ranges{tcp(80, 80), udp(81, 81)}},
		{"end of TCP does not touch start of UDP", ranges{tcp(65535, 65535), udp(0, 0)}, ranges{tcp(65535, 65535), udp(0, 0)}},
		{
			"ProtocolAll merges per protocol",
			ranges{tcp(54, 60), both(53, 53), udp(40, 52)},
			ranges{tcp(53, 60), udp(40, 53)},
		},
		{"port 0", ranges{tcp(0, 0)}, ranges{tcp(0, 0)}},
		{"port 65535", ranges{udp(65535, 65535)}, ranges{udp(65535, 65535)}},
		{"both edges stay apart", ranges{tcp(65535, 65535), tcp(0, 0)}, ranges{tcp(0, 0), tcp(65535, 65535)}},
		{"touching at the bottom", ranges{tcp(1, 1), tcp(0, 0)}, ranges{tcp(0, 1)}},
		{"touching at the top", ranges{tcp(65535, 65535), tcp(65534, 65534)}, ranges{tcp(65534, 65535)}},
		{"range after one ending at 65535", ranges{tcp(0, 65535), tcp(10, 20)}, ranges{tcp(0, 65535)}},
		{"two ranges ending at 65535", ranges{tcp(100, 65535), tcp(65535, 65535)}, ranges{tcp(100, 65535)}},
		{"halves cover every port", ranges{tcp(32768, 65535), tcp(0, 32767)}, ranges{tcp(0, 65535)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := sets.NewIntervalSpace()
			checkPortSet(t, "Ports", s.Ports(tc.in...), tc.want)
		})
	}
}

func TestPortSetOperations(t *testing.T) {
	tests := []struct {
		name                      string
		a, b                      ranges
		union, inter, aDiff, bDif ranges
	}{
		{name: "both empty"},
		{
			name: "one empty",
			a:    ranges{tcp(80, 80), udp(53, 53)},
			union: ranges{
				tcp(80, 80), udp(53, 53),
			},
			aDiff: ranges{tcp(80, 80), udp(53, 53)},
		},
		{
			name:  "diff splits a range",
			a:     ranges{tcp(80, 90)},
			b:     ranges{tcp(85, 85)},
			union: ranges{tcp(80, 90)},
			inter: ranges{tcp(85, 85)},
			aDiff: ranges{tcp(80, 84), tcp(86, 90)},
		},
		{
			name:  "union of touching ports",
			a:     ranges{tcp(80, 80)},
			b:     ranges{tcp(81, 81)},
			union: ranges{tcp(80, 81)},
			aDiff: ranges{tcp(80, 80)},
			bDif:  ranges{tcp(81, 81)},
		},
		{
			name:  "TCP and UDP are disjoint",
			a:     ranges{tcp(0, 65535)},
			b:     ranges{udp(0, 65535)},
			union: ranges{tcp(0, 65535), udp(0, 65535)},
			aDiff: ranges{tcp(0, 65535)},
			bDif:  ranges{udp(0, 65535)},
		},
		{
			name:  "same port, other protocol",
			a:     ranges{tcp(53, 53)},
			b:     ranges{udp(53, 53)},
			union: ranges{tcp(53, 53), udp(53, 53)},
			aDiff: ranges{tcp(53, 53)},
			bDif:  ranges{udp(53, 53)},
		},
		{
			name:  "ProtocolAll minus UDP",
			a:     ranges{both(53, 53)},
			b:     ranges{udp(53, 53)},
			union: ranges{tcp(53, 53), udp(53, 53)},
			inter: ranges{udp(53, 53)},
			aDiff: ranges{tcp(53, 53)},
		},
		{
			name:  "all ports minus all TCP",
			a:     ranges{model.AllPorts()},
			b:     ranges{tcp(0, 65535)},
			union: ranges{tcp(0, 65535), udp(0, 65535)},
			inter: ranges{tcp(0, 65535)},
			aDiff: ranges{udp(0, 65535)},
		},
		{
			name:  "equal sets",
			a:     ranges{tcp(80, 90), udp(53, 53)},
			b:     ranges{udp(53, 53), tcp(80, 90)},
			union: ranges{tcp(80, 90), udp(53, 53)},
			inter: ranges{tcp(80, 90), udp(53, 53)},
		},
		{
			name:  "removing port 0",
			a:     ranges{tcp(0, 65535)},
			b:     ranges{tcp(0, 0)},
			union: ranges{tcp(0, 65535)},
			inter: ranges{tcp(0, 0)},
			aDiff: ranges{tcp(1, 65535)},
		},
		{
			name:  "removing port 65535",
			a:     ranges{udp(0, 65535)},
			b:     ranges{udp(65535, 65535)},
			union: ranges{udp(0, 65535)},
			inter: ranges{udp(65535, 65535)},
			aDiff: ranges{udp(0, 65534)},
		},
		{
			name:  "everything but the edges",
			a:     ranges{tcp(0, 65535)},
			b:     ranges{tcp(1, 65534)},
			union: ranges{tcp(0, 65535)},
			inter: ranges{tcp(1, 65534)},
			aDiff: ranges{tcp(0, 0), tcp(65535, 65535)},
		},
		{
			name:  "shared end port",
			a:     ranges{tcp(80, 90)},
			b:     ranges{tcp(90, 100)},
			union: ranges{tcp(80, 100)},
			inter: ranges{tcp(90, 90)},
			aDiff: ranges{tcp(80, 89)},
			bDif:  ranges{tcp(91, 100)},
		},
		{
			name:  "interleaved ranges",
			a:     ranges{tcp(10, 20), tcp(30, 40), tcp(50, 60)},
			b:     ranges{tcp(15, 35), tcp(55, 70)},
			union: ranges{tcp(10, 40), tcp(50, 70)},
			inter: ranges{tcp(15, 20), tcp(30, 35), tcp(55, 60)},
			aDiff: ranges{tcp(10, 14), tcp(36, 40), tcp(50, 54)},
			bDif:  ranges{tcp(21, 29), tcp(61, 70)},
		},
		{
			name:  "one range spans many",
			a:     ranges{tcp(1, 1), tcp(3, 3), tcp(5, 5)},
			b:     ranges{tcp(0, 7)},
			union: ranges{tcp(0, 7)},
			inter: ranges{tcp(1, 1), tcp(3, 3), tcp(5, 5)},
			bDif:  ranges{tcp(0, 0), tcp(2, 2), tcp(4, 4), tcp(6, 7)},
		},
		{
			name:  "complementary pieces fill a range",
			a:     ranges{udp(0, 9), udp(20, 29)},
			b:     ranges{udp(10, 19), udp(30, 39)},
			union: ranges{udp(0, 39)},
			aDiff: ranges{udp(0, 9), udp(20, 29)},
			bDif:  ranges{udp(10, 19), udp(30, 39)},
		},
		{
			name:  "operands of different lengths across protocols",
			a:     ranges{tcp(80, 80), tcp(443, 443), udp(53, 53), udp(123, 123)},
			b:     ranges{tcp(400, 500), udp(100, 200)},
			union: ranges{tcp(80, 80), tcp(400, 500), udp(53, 53), udp(100, 200)},
			inter: ranges{tcp(443, 443), udp(123, 123)},
			aDiff: ranges{tcp(80, 80), udp(53, 53)},
			bDif:  ranges{tcp(400, 442), tcp(444, 500), udp(100, 122), udp(124, 200)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := sets.NewIntervalSpace()
			a, b := s.Ports(tc.a...), s.Ports(tc.b...)
			checkPortSet(t, "a∪b", a.Union(b), tc.union)
			checkPortSet(t, "b∪a", b.Union(a), tc.union)
			checkPortSet(t, "a∩b", a.Intersect(b), tc.inter)
			checkPortSet(t, "b∩a", b.Intersect(a), tc.inter)
			checkPortSet(t, "a∖b", a.Diff(b), tc.aDiff)
			checkPortSet(t, "b∖a", b.Diff(a), tc.bDif)
		})
	}
}

func TestPortSetContains(t *testing.T) {
	const T, U = model.ProtocolTCP, model.ProtocolUDP
	type probe struct {
		proto model.Protocol
		port  uint16
		want  bool
	}
	tests := []struct {
		name   string
		in     ranges
		probes []probe
	}{
		{"empty", nil, []probe{{T, 0, false}, {U, 0, false}, {T, 65535, false}, {U, 65535, false}}},
		{"both ends of a range", ranges{tcp(32000, 32768)}, []probe{
			{T, 31999, false}, {T, 32000, true}, {T, 32500, true}, {T, 32768, true}, {T, 32769, false},
			{U, 32000, false}, {U, 32768, false},
		}},
		{"several ranges", ranges{tcp(80, 80), tcp(443, 445), tcp(8080, 8090), udp(53, 53)}, []probe{
			{T, 0, false}, {T, 79, false}, {T, 80, true}, {T, 81, false}, {T, 442, false}, {T, 443, true},
			{T, 445, true}, {T, 446, false}, {T, 8080, true}, {T, 8090, true}, {T, 8091, false},
			{T, 65535, false}, {T, 53, false}, {U, 52, false}, {U, 53, true}, {U, 54, false}, {U, 80, false},
		}},
		{"edges", ranges{tcp(0, 0), udp(65535, 65535)}, []probe{
			{T, 0, true}, {T, 1, false}, {T, 65535, false},
			{U, 65535, true}, {U, 65534, false}, {U, 0, false},
		}},
		{"all ports", ranges{model.AllPorts()}, []probe{
			{T, 0, true}, {T, 65535, true}, {U, 0, true}, {U, 65535, true},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x := sets.NewIntervalSpace().Ports(tc.in...)
			for _, p := range tc.probes {
				if got := x.Contains(p.proto, p.port); got != p.want {
					t.Errorf("Contains(%s, %d) = %v, want %v", p.proto, p.port, got, p.want)
				}
			}
		})
	}
}

func TestPortSetOperandsUnchanged(t *testing.T) {
	tests := []struct {
		name string
		a, b ranges
	}{
		{"split", ranges{tcp(80, 90)}, ranges{tcp(85, 85)}},
		{"merge", ranges{tcp(80, 80), udp(53, 53)}, ranges{tcp(81, 81), udp(54, 54)}},
		{"interleaved", ranges{tcp(10, 20), tcp(30, 40), tcp(50, 60)}, ranges{tcp(15, 35), tcp(55, 70)}},
		{"empty operand", ranges{model.AllPorts()}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := sets.NewIntervalSpace()
			a, b := s.Ports(tc.a...), s.Ports(tc.b...)
			wantA, wantB := slices.Clone(a.Ranges()), slices.Clone(b.Ranges())
			ops := []struct {
				name string
				run  func()
			}{
				{"a∪b", func() { a.Union(b) }},
				{"b∪a", func() { b.Union(a) }},
				{"a∩b", func() { a.Intersect(b) }},
				{"b∩a", func() { b.Intersect(a) }},
				{"a∖b", func() { a.Diff(b) }},
				{"b∖a", func() { b.Diff(a) }},
				{"a∪a", func() { a.Union(a) }},
				{"a∖a", func() { a.Diff(a) }},
				{"Contains and IsEmpty", func() { a.Contains(model.ProtocolTCP, 85); b.IsEmpty() }},
			}
			for _, op := range ops {
				op.run()
				checkPortSet(t, "a after "+op.name, a, wantA)
				checkPortSet(t, "b after "+op.name, b, wantB)
			}
		})
	}
}

// A result may be an operand of later operations, which must not change it or
// a sibling result derived from it.
func TestPortSetDerivedResultsIndependent(t *testing.T) {
	s := sets.NewIntervalSpace()
	base := s.Ports(tcp(10, 10), tcp(20, 20), tcp(30, 30)).Union(s.Ports(tcp(40, 40)))
	first := base.Union(s.Ports(tcp(50, 50)))
	second := base.Union(s.Ports(tcp(60, 60)))
	cut := base.Diff(s.Ports(tcp(10, 10)))
	narrowed := base.Intersect(s.Ports(tcp(30, 40)))

	prefix := ranges{tcp(10, 10), tcp(20, 20), tcp(30, 30), tcp(40, 40)}
	checkPortSet(t, "base", base, prefix)
	checkPortSet(t, "first", first, append(slices.Clone(prefix), tcp(50, 50)))
	checkPortSet(t, "second", second, append(slices.Clone(prefix), tcp(60, 60)))
	checkPortSet(t, "cut", cut, ranges{tcp(20, 20), tcp(30, 30), tcp(40, 40)})
	checkPortSet(t, "narrowed", narrowed, ranges{tcp(30, 30), tcp(40, 40)})
}

// The connection space is TCP and UDP only, and ProtocolAll names a set of
// protocols, not the protocol of a connection.
func TestPortSetContainsOtherProtocols(t *testing.T) {
	all := sets.NewIntervalSpace().Ports(model.AllPorts())
	for _, proto := range []model.Protocol{model.ProtocolAll, "sctp", "icmp", ""} {
		for _, port := range []uint16{0, 80, 65535} {
			if all.Contains(proto, port) {
				t.Errorf("Contains(%q, %d) = true, want false", proto, port)
			}
		}
	}
}

func TestPortSetKeepsNoCallerSlice(t *testing.T) {
	in := ranges{udp(53, 53), tcp(90, 99), both(7, 7), tcp(80, 89)}
	orig := slices.Clone(in)
	want := ranges{tcp(7, 7), tcp(80, 99), udp(7, 7), udp(53, 53)}

	x := sets.NewIntervalSpace().Ports(in...)
	if !slices.Equal(in, orig) {
		t.Errorf("Ports changed the caller's slice: %v, want %v", in, orig)
	}
	for i := range in {
		in[i] = tcp(1, 2)
	}
	checkPortSet(t, "after the caller changed its slice", x, want)
}

// A single already canonical range list is the case an implementation is most
// likely to store as it is.
func TestPortSetKeepsNoCanonicalCallerSlice(t *testing.T) {
	in := ranges{tcp(80, 80), tcp(443, 443), udp(53, 53)}
	want := slices.Clone(in)

	x := sets.NewIntervalSpace().Ports(in...)
	for i := range in {
		in[i] = tcp(1, 2)
	}
	checkPortSet(t, "after the caller changed its slice", x, want)
}

func TestPortSetRangesOwnedByCaller(t *testing.T) {
	s := sets.NewIntervalSpace()
	a := s.Ports(tcp(80, 90), udp(53, 53))
	// Results that equal an operand are where storage is most likely shared.
	tests := []struct {
		name string
		set  sets.GateSet
	}{
		{"constructed", a},
		{"union with empty", a.Union(s.Ports())},
		{"union of empty", s.Ports().Union(a)},
		{"intersection with all", a.Intersect(s.Ports(model.AllPorts()))},
		{"difference with empty", a.Diff(s.Ports())},
	}
	want := ranges{tcp(80, 90), udp(53, 53)}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rs := tc.set.Ranges()
			for i := range rs {
				rs[i] = udp(1, 2)
			}
			_ = append(rs, udp(3, 4))
			checkPortSet(t, "set", tc.set, want)
			checkPortSet(t, "operand", a, want)
		})
	}
}

// Expected address sets are CIDR arithmetic (RFC 4632): a prefix a/n is the
// 2^(32-n) addresses starting at its masked address. An ipBlock matches its
// cidr less every except entry.

// cidrs parses prefixes, keeping any host bits they are written with.
func cidrs(s ...string) []netip.Prefix {
	var out []netip.Prefix
	for _, p := range s {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}

func checkAddrSet(t *testing.T, name string, got sets.AddrSet, want []netip.Prefix) {
	t.Helper()
	if ps := got.Prefixes(); !slices.Equal(ps, want) {
		t.Errorf("%s: Prefixes() = %v, want %v", name, ps, want)
	}
	if got.IsEmpty() != (len(want) == 0) {
		t.Errorf("%s: IsEmpty() = %v, want %v", name, got.IsEmpty(), len(want) == 0)
	}
}

func TestAddrSetConstruction(t *testing.T) {
	tests := []struct {
		name string
		in   []netip.Prefix
		want []netip.Prefix
	}{
		{"no prefixes is empty", nil, nil},
		{"one prefix", cidrs("10.0.0.0/8"), cidrs("10.0.0.0/8")},
		{"single address", cidrs("192.0.2.7/32"), cidrs("192.0.2.7/32")},
		{"full space", cidrs("0.0.0.0/0"), cidrs("0.0.0.0/0")},
		{"first address", cidrs("0.0.0.0/32"), cidrs("0.0.0.0/32")},
		{"last address", cidrs("255.255.255.255/32"), cidrs("255.255.255.255/32")},
		{"host bits are masked", cidrs("10.1.2.3/8"), cidrs("10.0.0.0/8")},
		{"host bits inside an octet", cidrs("192.168.1.77/26"), cidrs("192.168.1.64/26")},
		{"host bits with length 0", cidrs("1.2.3.4/0"), cidrs("0.0.0.0/0")},
		{"same network written twice", cidrs("10.1.2.3/8", "10.200.0.1/8"), cidrs("10.0.0.0/8")},
		{"adjacent halves merge", cidrs("10.0.0.0/24", "10.0.1.0/24"), cidrs("10.0.0.0/23")},
		{"halves merge in reverse order", cidrs("10.0.1.0/24", "10.0.0.0/24"), cidrs("10.0.0.0/23")},
		{"gap stays two", cidrs("10.0.2.0/24", "10.0.0.0/24"), cidrs("10.0.0.0/24", "10.0.2.0/24")},
		{"touching but unaligned stays two", cidrs("10.0.1.0/24", "10.0.2.0/24"), cidrs("10.0.1.0/24", "10.0.2.0/24")},
		{"touching addresses unaligned", cidrs("10.0.0.1/32", "10.0.0.2/32"), cidrs("10.0.0.1/32", "10.0.0.2/32")},
		{"touching addresses aligned", cidrs("10.0.0.0/32", "10.0.0.1/32"), cidrs("10.0.0.0/31")},
		{
			"merges cascade",
			cidrs("10.0.0.192/26", "10.0.0.0/26", "10.0.0.128/26", "10.0.0.64/26"),
			cidrs("10.0.0.0/24"),
		},
		{
			"merged interval needs unequal prefixes",
			cidrs("10.0.3.0/24", "10.0.1.0/24", "10.0.2.0/24"),
			cidrs("10.0.1.0/24", "10.0.2.0/23"),
		},
		{"nested prefix is absorbed", cidrs("10.1.0.0/16", "10.0.0.0/8", "10.255.255.255/32"), cidrs("10.0.0.0/8")},
		{"halves of the space", cidrs("128.0.0.0/1", "0.0.0.0/1"), cidrs("0.0.0.0/0")},
		{"top of the space merges", cidrs("255.255.255.255/32", "255.255.255.254/32"), cidrs("255.255.255.254/31")},
		{"prefix after the full space", cidrs("0.0.0.0/0", "10.0.0.0/8", "255.255.255.255/32"), cidrs("0.0.0.0/0")},
		{
			"both edges stay apart",
			cidrs("255.255.255.255/32", "0.0.0.0/32"),
			cidrs("0.0.0.0/32", "255.255.255.255/32"),
		},
		{"IPv6 adds nothing", cidrs("2001:db8::/32", "::/0", "::1/128"), nil},
		{"IPv4-mapped adds nothing", cidrs("::ffff:10.0.0.0/104", "::ffff:0.0.0.0/96", "::ffff:10.0.0.1/128"), nil},
		{
			"IPv6 beside IPv4",
			cidrs("2001:db8::/32", "10.0.0.0/8", "::ffff:192.168.0.0/112"),
			cidrs("10.0.0.0/8"),
		},
		{
			"invalid prefixes add nothing",
			[]netip.Prefix{
				{},
				netip.PrefixFrom(netip.MustParseAddr("10.0.0.0"), 33),
				netip.PrefixFrom(netip.MustParseAddr("10.0.0.0"), -1),
				netip.PrefixFrom(netip.Addr{}, 8),
			},
			nil,
		},
		{
			"invalid beside valid",
			[]netip.Prefix{
				netip.PrefixFrom(netip.MustParseAddr("11.0.0.0"), 33),
				netip.MustParsePrefix("10.0.0.0/8"),
				{},
			},
			cidrs("10.0.0.0/8"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := sets.NewIntervalSpace()
			checkAddrSet(t, "Addrs", s.Addrs(tc.in...), tc.want)
		})
	}
}

func TestAddrSetOperations(t *testing.T) {
	tests := []struct {
		name                      string
		a, b                      []netip.Prefix
		union, inter, aDiff, bDif []netip.Prefix
	}{
		{name: "both empty"},
		{
			name:  "one empty",
			a:     cidrs("10.0.0.0/8"),
			union: cidrs("10.0.0.0/8"),
			aDiff: cidrs("10.0.0.0/8"),
		},
		{
			name:  "lower half removed",
			a:     cidrs("10.0.0.0/8"),
			b:     cidrs("10.0.0.0/9"),
			union: cidrs("10.0.0.0/8"),
			inter: cidrs("10.0.0.0/9"),
			aDiff: cidrs("10.128.0.0/9"),
		},
		{
			name:  "union of halves",
			a:     cidrs("10.0.0.0/24"),
			b:     cidrs("10.0.1.0/24"),
			union: cidrs("10.0.0.0/23"),
			aDiff: cidrs("10.0.0.0/24"),
			bDif:  cidrs("10.0.1.0/24"),
		},
		{
			name:  "union with a gap",
			a:     cidrs("10.0.0.0/24"),
			b:     cidrs("10.0.2.0/24"),
			union: cidrs("10.0.0.0/24", "10.0.2.0/24"),
			aDiff: cidrs("10.0.0.0/24"),
			bDif:  cidrs("10.0.2.0/24"),
		},
		{
			name:  "full space minus 10/8",
			a:     cidrs("0.0.0.0/0"),
			b:     cidrs("10.0.0.0/8"),
			union: cidrs("0.0.0.0/0"),
			inter: cidrs("10.0.0.0/8"),
			aDiff: cidrs(
				"0.0.0.0/5", "8.0.0.0/7", "11.0.0.0/8", "12.0.0.0/6",
				"16.0.0.0/4", "32.0.0.0/3", "64.0.0.0/2", "128.0.0.0/1",
			),
		},
		{
			// The ipBlock of the example NetworkPolicy in the Kubernetes docs.
			name:  "ipBlock with except",
			a:     cidrs("172.17.0.0/16"),
			b:     cidrs("172.17.1.0/24"),
			union: cidrs("172.17.0.0/16"),
			inter: cidrs("172.17.1.0/24"),
			aDiff: cidrs(
				"172.17.0.0/24", "172.17.2.0/23", "172.17.4.0/22", "172.17.8.0/21",
				"172.17.16.0/20", "172.17.32.0/19", "172.17.64.0/18", "172.17.128.0/17",
			),
		},
		{
			name:  "two except entries",
			a:     cidrs("192.168.0.0/24"),
			b:     cidrs("192.168.0.64/26", "192.168.0.192/26"),
			union: cidrs("192.168.0.0/24"),
			inter: cidrs("192.168.0.64/26", "192.168.0.192/26"),
			aDiff: cidrs("192.168.0.0/26", "192.168.0.128/26"),
		},
		{
			name:  "except entries cover the block",
			a:     cidrs("10.0.0.0/24"),
			b:     cidrs("10.0.0.128/25", "10.0.0.0/25"),
			union: cidrs("10.0.0.0/24"),
			inter: cidrs("10.0.0.0/24"),
		},
		{
			name:  "first address removed",
			a:     cidrs("10.0.0.0/24"),
			b:     cidrs("10.0.0.0/32"),
			union: cidrs("10.0.0.0/24"),
			inter: cidrs("10.0.0.0/32"),
			aDiff: cidrs(
				"10.0.0.1/32", "10.0.0.2/31", "10.0.0.4/30", "10.0.0.8/29",
				"10.0.0.16/28", "10.0.0.32/27", "10.0.0.64/26", "10.0.0.128/25",
			),
		},
		{
			name:  "last address removed",
			a:     cidrs("10.0.0.0/24"),
			b:     cidrs("10.0.0.255/32"),
			union: cidrs("10.0.0.0/24"),
			inter: cidrs("10.0.0.255/32"),
			aDiff: cidrs(
				"10.0.0.0/25", "10.0.0.128/26", "10.0.0.192/27", "10.0.0.224/28",
				"10.0.0.240/29", "10.0.0.248/30", "10.0.0.252/31", "10.0.0.254/32",
			),
		},
		{
			name:  "both ends of a block removed",
			a:     cidrs("10.0.0.0/29"),
			b:     cidrs("10.0.0.0/32", "10.0.0.7/32"),
			union: cidrs("10.0.0.0/29"),
			inter: cidrs("10.0.0.0/32", "10.0.0.7/32"),
			aDiff: cidrs("10.0.0.1/32", "10.0.0.2/31", "10.0.0.4/31", "10.0.0.6/32"),
		},
		{
			name:  "disjoint",
			a:     cidrs("10.0.0.0/8"),
			b:     cidrs("192.168.0.0/16"),
			union: cidrs("10.0.0.0/8", "192.168.0.0/16"),
			aDiff: cidrs("10.0.0.0/8"),
			bDif:  cidrs("192.168.0.0/16"),
		},
		{
			name:  "equal sets written differently",
			a:     cidrs("10.0.0.0/23"),
			b:     cidrs("10.0.1.9/24", "10.0.0.9/24"),
			union: cidrs("10.0.0.0/23"),
			inter: cidrs("10.0.0.0/23"),
		},
		{
			name:  "interleaved prefixes",
			a:     cidrs("10.0.0.0/24", "10.0.2.0/24", "10.0.4.0/24"),
			b:     cidrs("10.0.0.128/25", "10.0.1.0/24", "10.0.2.0/25", "10.0.4.0/22"),
			union: cidrs("10.0.0.0/23", "10.0.2.0/24", "10.0.4.0/22"),
			inter: cidrs("10.0.0.128/25", "10.0.2.0/25", "10.0.4.0/24"),
			aDiff: cidrs("10.0.0.0/25", "10.0.2.128/25"),
			bDif:  cidrs("10.0.1.0/24", "10.0.5.0/24", "10.0.6.0/23"),
		},
		{
			name:  "full space minus its upper half",
			a:     cidrs("0.0.0.0/0"),
			b:     cidrs("128.0.0.0/1"),
			union: cidrs("0.0.0.0/0"),
			inter: cidrs("128.0.0.0/1"),
			aDiff: cidrs("0.0.0.0/1"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := sets.NewIntervalSpace()
			a, b := s.Addrs(tc.a...), s.Addrs(tc.b...)
			checkAddrSet(t, "a∪b", a.Union(b), tc.union)
			checkAddrSet(t, "b∪a", b.Union(a), tc.union)
			checkAddrSet(t, "a∩b", a.Intersect(b), tc.inter)
			checkAddrSet(t, "b∩a", b.Intersect(a), tc.inter)
			checkAddrSet(t, "a∖b", a.Diff(b), tc.aDiff)
			checkAddrSet(t, "b∖a", b.Diff(a), tc.bDif)
		})
	}
}

// Removing one end of the whole space leaves one prefix of every length.
func TestAddrSetFullSpaceLessOneEnd(t *testing.T) {
	var lessFirst, lessLast []netip.Prefix
	// Ascending by address: 0.0.0.1/32, 0.0.0.2/31, ..., 128.0.0.0/1.
	for bits := 32; bits >= 1; bits-- {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(1)<<(32-bits))
		lessFirst = append(lessFirst, netip.PrefixFrom(netip.AddrFrom4(b), bits))
	}
	// Ascending by address: 0.0.0.0/1, 128.0.0.0/2, ..., 255.255.255.254/32.
	for bits := 1; bits <= 32; bits++ {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(uint64(0xffffffff)<<(33-bits)))
		lessLast = append(lessLast, netip.PrefixFrom(netip.AddrFrom4(b), bits))
	}
	tests := []struct {
		name    string
		removed string
		want    []netip.Prefix
	}{
		{"less the first address", "0.0.0.0/32", lessFirst},
		{"less the last address", "255.255.255.255/32", lessLast},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := sets.NewIntervalSpace()
			all := s.Addrs(cidrs("0.0.0.0/0")...)
			one := s.Addrs(cidrs(tc.removed)...)
			rest := all.Diff(one)
			checkAddrSet(t, "difference", rest, tc.want)
			checkAddrSet(t, "difference restored", rest.Union(one), cidrs("0.0.0.0/0"))
			checkAddrSet(t, "difference and removed", rest.Intersect(one), nil)
		})
	}
}

func TestAddrSetContains(t *testing.T) {
	// None of these is an IPv4 address, so no set holds them.
	notIPv4 := []netip.Addr{
		{},
		netip.MustParseAddr("::"),
		netip.MustParseAddr("::1"),
		netip.MustParseAddr("2001:db8::1"),
		netip.MustParseAddr("::ffff:0.0.0.0"),
		netip.MustParseAddr("::ffff:10.0.0.1"),
		netip.MustParseAddr("::ffff:255.255.255.255"),
	}
	tests := []struct {
		name   string
		in     []netip.Prefix
		in4    []string
		notIn4 []string
	}{
		{name: "empty", notIn4: []string{"0.0.0.0", "10.0.0.1", "255.255.255.255"}},
		{
			name:   "first and last address of a prefix",
			in:     cidrs("10.0.0.0/8"),
			in4:    []string{"10.0.0.0", "10.1.2.3", "10.255.255.255"},
			notIn4: []string{"0.0.0.0", "9.255.255.255", "11.0.0.0", "255.255.255.255"},
		},
		{
			name:   "single address",
			in:     cidrs("192.0.2.7/32"),
			in4:    []string{"192.0.2.7"},
			notIn4: []string{"192.0.2.6", "192.0.2.8"},
		},
		{
			name: "full space",
			in:   cidrs("0.0.0.0/0"),
			in4:  []string{"0.0.0.0", "10.0.0.1", "127.0.0.1", "255.255.255.255"},
		},
		{
			name: "several prefixes",
			in:   cidrs("10.0.0.0/24", "10.0.2.0/24", "192.168.0.0/16"),
			in4: []string{
				"10.0.0.0", "10.0.0.255", "10.0.2.0", "10.0.2.255", "192.168.0.0", "192.168.255.255",
			},
			notIn4: []string{
				"9.255.255.255", "10.0.1.0", "10.0.1.255", "10.0.3.0", "192.167.255.255", "192.169.0.0",
			},
		},
		{
			name:   "both edges",
			in:     cidrs("0.0.0.0/32", "255.255.255.255/32"),
			in4:    []string{"0.0.0.0", "255.255.255.255"},
			notIn4: []string{"0.0.0.1", "255.255.255.254", "128.0.0.0"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x := sets.NewIntervalSpace().Addrs(tc.in...)
			for _, a := range tc.in4 {
				if !x.Contains(netip.MustParseAddr(a)) {
					t.Errorf("Contains(%s) = false, want true", a)
				}
			}
			for _, a := range tc.notIn4 {
				if x.Contains(netip.MustParseAddr(a)) {
					t.Errorf("Contains(%s) = true, want false", a)
				}
			}
			for _, a := range notIPv4 {
				if x.Contains(a) {
					t.Errorf("Contains(%v) = true, want false for a non-IPv4 address", a)
				}
			}
		})
	}
}

func TestAddrSetKeepsNoCallerSlice(t *testing.T) {
	tests := []struct {
		name string
		in   []netip.Prefix
		want []netip.Prefix
	}{
		{
			"unsorted with host bits",
			cidrs("192.168.1.9/24", "2001:db8::/32", "10.0.1.0/24", "10.0.0.7/24"),
			cidrs("10.0.0.0/23", "192.168.1.0/24"),
		},
		{
			"already canonical",
			cidrs("10.0.0.0/24", "10.0.2.0/24"),
			cidrs("10.0.0.0/24", "10.0.2.0/24"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := slices.Clone(tc.in)
			x := sets.NewIntervalSpace().Addrs(in...)
			if !slices.Equal(in, tc.in) {
				t.Errorf("Addrs changed the caller's slice: %v, want %v", in, tc.in)
			}
			for i := range in {
				in[i] = netip.MustParsePrefix("203.0.113.0/24")
			}
			checkAddrSet(t, "after the caller changed its slice", x, tc.want)
		})
	}
}

func TestAddrSetPrefixesOwnedByCaller(t *testing.T) {
	s := sets.NewIntervalSpace()
	want := cidrs("10.0.0.0/24", "10.0.2.0/24")
	a := s.Addrs(want...)
	// Results that equal an operand are where storage is most likely shared.
	tests := []struct {
		name string
		set  sets.AddrSet
	}{
		{"constructed", a},
		{"union with empty", a.Union(s.Addrs())},
		{"union of empty", s.Addrs().Union(a)},
		{"intersection with all", a.Intersect(s.Addrs(cidrs("0.0.0.0/0")...))},
		{"difference with empty", a.Diff(s.Addrs())},
	}
	other := netip.MustParsePrefix("203.0.113.0/24")
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := tc.set.Prefixes()
			for i := range ps {
				ps[i] = other
			}
			_ = append(ps, other)
			checkAddrSet(t, "set", tc.set, want)
			checkAddrSet(t, "operand", a, want)
		})
	}
}

func TestAddrSetOperandsUnchanged(t *testing.T) {
	tests := []struct {
		name string
		a, b []netip.Prefix
	}{
		{"split", cidrs("10.0.0.0/8"), cidrs("10.1.0.0/16")},
		{"merge", cidrs("10.0.0.0/24", "10.0.2.0/24"), cidrs("10.0.1.0/24", "10.0.3.0/24")},
		{"interleaved", cidrs("10.0.0.0/24", "10.0.2.0/24", "10.0.4.0/24"), cidrs("10.0.0.128/25", "10.0.4.0/22")},
		{"empty operand", cidrs("0.0.0.0/0"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := sets.NewIntervalSpace()
			a, b := s.Addrs(tc.a...), s.Addrs(tc.b...)
			wantA, wantB := slices.Clone(a.Prefixes()), slices.Clone(b.Prefixes())
			ops := []struct {
				name string
				run  func()
			}{
				{"a∪b", func() { a.Union(b) }},
				{"b∪a", func() { b.Union(a) }},
				{"a∩b", func() { a.Intersect(b) }},
				{"b∩a", func() { b.Intersect(a) }},
				{"a∖b", func() { a.Diff(b) }},
				{"b∖a", func() { b.Diff(a) }},
				{"a∪a", func() { a.Union(a) }},
				{"a∖a", func() { a.Diff(a) }},
				{"Contains and IsEmpty", func() { a.Contains(netip.MustParseAddr("10.0.0.1")); b.IsEmpty() }},
			}
			for _, op := range ops {
				op.run()
				checkAddrSet(t, "a after "+op.name, a, wantA)
				checkAddrSet(t, "b after "+op.name, b, wantB)
			}
		})
	}
}

// See TestPortSetDerivedResultsIndependent.
func TestAddrSetDerivedResultsIndependent(t *testing.T) {
	s := sets.NewIntervalSpace()
	one := func(p string) sets.AddrSet { return s.Addrs(cidrs(p)...) }
	base := s.Addrs(cidrs("10.0.0.0/32", "10.0.0.2/32", "10.0.0.4/32")...).Union(one("10.0.0.6/32"))
	first := base.Union(one("10.0.0.8/32"))
	second := base.Union(one("10.0.0.10/32"))
	cut := base.Diff(one("10.0.0.0/32"))
	narrowed := base.Intersect(one("10.0.0.4/30"))

	prefix := cidrs("10.0.0.0/32", "10.0.0.2/32", "10.0.0.4/32", "10.0.0.6/32")
	checkAddrSet(t, "base", base, prefix)
	checkAddrSet(t, "first", first, append(slices.Clone(prefix), cidrs("10.0.0.8/32")...))
	checkAddrSet(t, "second", second, append(slices.Clone(prefix), cidrs("10.0.0.10/32")...))
	checkAddrSet(t, "cut", cut, cidrs("10.0.0.2/32", "10.0.0.4/32", "10.0.0.6/32"))
	checkAddrSet(t, "narrowed", narrowed, cidrs("10.0.0.4/32", "10.0.0.6/32"))
}
