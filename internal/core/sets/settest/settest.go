// Package settest checks that a sets.Space meets the contract of package
// sets: the set algebra laws, exact membership, values that operations
// never change, and the canonical forms. Inputs are generated and shrink to
// a minimal failing case. Every Space implementation's tests call Laws; the
// package is imported only from tests.
package settest

import (
	"encoding/binary"
	"net/netip"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/Soo-de/suiro/internal/core/sets"
	"github.com/Soo-de/suiro/internal/model"
)

// Laws checks s against the contract of package sets, one subtest per law
// group, for port sets and address sets alike. It uses s from the calling
// goroutine only.
func Laws(t *testing.T, s sets.Space) {
	t.Run("ports", func(t *testing.T) {
		g := portSets(s)
		u := s.Ports(model.AllPorts())
		checkAlgebra(t, g, u, s.Ports(), canonPorts)
		t.Run("membership", func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				a, b := g.Draw(rt, "a"), g.Draw(rt, "b")
				p, port := protocol.Draw(rt, "protocol"), rapid.Uint16().Draw(rt, "port")
				in := func(x sets.GateSet) bool { return x.Contains(p, port) }
				if in(a.Union(b)) != (in(a) || in(b)) ||
					in(a.Intersect(b)) != (in(a) && in(b)) ||
					in(a.Diff(b)) != (in(a) && !in(b)) {
					rt.Fatalf("membership of %s/%d disagrees with union, intersection or difference", p, port)
				}
			})
		})
		t.Run("construction", func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				r := portRange.Draw(rt, "range")
				x := s.Ports(r)
				for _, p := range []model.Protocol{model.ProtocolTCP, model.ProtocolUDP} {
					covered := r.Protocol == p || r.Protocol == model.ProtocolAll
					if x.Contains(p, r.From) != covered || x.Contains(p, r.To) != covered {
						rt.Fatalf("Ports(%v) does not hold its ends for %s", r, p)
					}
					if r.From > 0 && x.Contains(p, r.From-1) {
						rt.Fatalf("Ports(%v) holds %s/%d", r, p, r.From-1)
					}
					if r.To < 65535 && x.Contains(p, r.To+1) {
						rt.Fatalf("Ports(%v) holds %s/%d", r, p, r.To+1)
					}
				}
			})
		})
		t.Run("canonical form", func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				a := g.Draw(rt, "a")
				rs := a.Ranges()
				checkRanges(rt, rs)
				if a.IsEmpty() != (len(rs) == 0) {
					rt.Fatalf("IsEmpty is %v with Ranges %v", a.IsEmpty(), rs)
				}
				if !slices.Equal(s.Ports(rs...).Ranges(), rs) {
					rt.Fatalf("Ports(Ranges()...) differs from the set")
				}
			})
		})
	})

	t.Run("addresses", func(t *testing.T) {
		g := addrSets(s)
		u := s.Addrs(netip.MustParsePrefix("0.0.0.0/0"))
		checkAlgebra(t, g, u, s.Addrs(), canonAddrs)
		t.Run("membership", func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				a, b := g.Draw(rt, "a"), g.Draw(rt, "b")
				x := address.Draw(rt, "address")
				in := func(y sets.AddrSet) bool { return y.Contains(x) }
				if in(a.Union(b)) != (in(a) || in(b)) ||
					in(a.Intersect(b)) != (in(a) && in(b)) ||
					in(a.Diff(b)) != (in(a) && !in(b)) {
					rt.Fatalf("membership of %s disagrees with union, intersection or difference", x)
				}
			})
		})
		t.Run("construction", func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				p := prefix.Draw(rt, "prefix")
				x := s.Addrs(p)
				first, last := bounds(p)
				if !x.Contains(first) || !x.Contains(last) {
					rt.Fatalf("Addrs(%s) does not hold its ends", p)
				}
				if before := first.Prev(); before.IsValid() && x.Contains(before) {
					rt.Fatalf("Addrs(%s) holds %s", p, before)
				}
				if after := last.Next(); after.IsValid() && x.Contains(after) {
					rt.Fatalf("Addrs(%s) holds %s", p, after)
				}
			})
		})
		t.Run("ipv6 adds nothing", func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				p := prefix.Draw(rt, "prefix")
				v6 := netip.MustParsePrefix("2001:db8::/32")
				if !slices.Equal(s.Addrs(p, v6).Prefixes(), s.Addrs(p).Prefixes()) || !s.Addrs(v6).IsEmpty() {
					rt.Fatalf("an IPv6 prefix changed the set")
				}
			})
		})
		t.Run("canonical form", func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				a := g.Draw(rt, "a")
				ps := a.Prefixes()
				checkPrefixes(rt, ps)
				if a.IsEmpty() != (len(ps) == 0) {
					rt.Fatalf("IsEmpty is %v with Prefixes %v", a.IsEmpty(), ps)
				}
				if !slices.Equal(s.Addrs(ps...).Prefixes(), ps) {
					rt.Fatalf("Addrs(Prefixes()...) differs from the set")
				}
			})
		})
	})
}

// set is what the algebra laws need of GateSet and AddrSet.
type set[S any] interface {
	Intersect(S) S
	Union(S) S
	Diff(S) S
	IsEmpty() bool
}

// checkAlgebra checks the laws that hold for every set type, comparing sets
// by canon, their canonical form; u is the universe and empty the empty set.
func checkAlgebra[S set[S], C comparable](t *testing.T, g *rapid.Generator[S], u, empty S, canon func(S) []C) {
	eq := func(rt *rapid.T, law string, x, y S) {
		if !slices.Equal(canon(x), canon(y)) {
			rt.Fatalf("%s: %v != %v", law, canon(x), canon(y))
		}
	}
	three := func(rt *rapid.T) (S, S, S) { return g.Draw(rt, "a"), g.Draw(rt, "b"), g.Draw(rt, "c") }

	t.Run("commutative", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			a, b, _ := three(rt)
			eq(rt, "a∪b = b∪a", a.Union(b), b.Union(a))
			eq(rt, "a∩b = b∩a", a.Intersect(b), b.Intersect(a))
		})
	})
	t.Run("associative", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			a, b, c := three(rt)
			eq(rt, "(a∪b)∪c = a∪(b∪c)", a.Union(b).Union(c), a.Union(b.Union(c)))
			eq(rt, "(a∩b)∩c = a∩(b∩c)", a.Intersect(b).Intersect(c), a.Intersect(b.Intersect(c)))
		})
	})
	t.Run("distributive", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			a, b, c := three(rt)
			eq(rt, "a∩(b∪c) = (a∩b)∪(a∩c)", a.Intersect(b.Union(c)), a.Intersect(b).Union(a.Intersect(c)))
			eq(rt, "a∪(b∩c) = (a∪b)∩(a∪c)", a.Union(b.Intersect(c)), a.Union(b).Intersect(a.Union(c)))
		})
	})
	t.Run("absorption and idempotence", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			a, b, _ := three(rt)
			eq(rt, "a∪(a∩b) = a", a.Union(a.Intersect(b)), a)
			eq(rt, "a∩(a∪b) = a", a.Intersect(a.Union(b)), a)
			eq(rt, "a∪a = a", a.Union(a), a)
			eq(rt, "a∩a = a", a.Intersect(a), a)
		})
	})
	t.Run("identity", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			a, _, _ := three(rt)
			if !empty.IsEmpty() {
				rt.Fatalf("the set built from nothing is not empty")
			}
			eq(rt, "a∪∅ = a", a.Union(empty), a)
			eq(rt, "a∩∅ = ∅", a.Intersect(empty), empty)
			eq(rt, "a∩U = a", a.Intersect(u), a)
			eq(rt, "a∪U = U", a.Union(u), u)
			eq(rt, "a∖a = ∅", a.Diff(a), empty)
		})
	})
	t.Run("difference", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			a, b, _ := three(rt)
			if !a.Diff(b).Intersect(b).IsEmpty() {
				rt.Fatalf("(a∖b)∩b is not empty")
			}
			eq(rt, "(a∖b)∪(a∩b) = a", a.Diff(b).Union(a.Intersect(b)), a)
		})
	})
	t.Run("De Morgan", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			a, b, _ := three(rt)
			eq(rt, "U∖(a∪b) = (U∖a)∩(U∖b)", u.Diff(a.Union(b)), u.Diff(a).Intersect(u.Diff(b)))
			eq(rt, "U∖(a∩b) = (U∖a)∪(U∖b)", u.Diff(a.Intersect(b)), u.Diff(a).Union(u.Diff(b)))
		})
	})
	t.Run("operands unchanged", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			a, b, _ := three(rt)
			ca, cb := canon(a), canon(b)
			a.Union(b)
			a.Intersect(b)
			a.Diff(b)
			if !slices.Equal(canon(a), ca) || !slices.Equal(canon(b), cb) {
				rt.Fatalf("an operation changed its operands")
			}
		})
	})
}

func canonPorts(x sets.GateSet) []model.PortRange { return x.Ranges() }

func canonAddrs(x sets.AddrSet) []netip.Prefix { return x.Prefixes() }

// checkRanges fails rt unless rs is canonical: TCP and UDP only, TCP first,
// each range valid, sorted, and neither overlapping nor touching the next
// range of the same protocol.
func checkRanges(rt *rapid.T, rs []model.PortRange) {
	for i, r := range rs {
		if r.Protocol != model.ProtocolTCP && r.Protocol != model.ProtocolUDP || r.From > r.To {
			rt.Fatalf("Ranges holds %v", r)
		}
		if i == 0 {
			continue
		}
		prev := rs[i-1]
		switch {
		case prev.Protocol == model.ProtocolUDP && r.Protocol == model.ProtocolTCP:
			rt.Fatalf("Ranges lists UDP before TCP: %v", rs)
		case prev.Protocol == r.Protocol && int(prev.To)+1 >= int(r.From):
			rt.Fatalf("Ranges are unsorted, overlapping or touching: %v, %v", prev, r)
		}
	}
}

// checkPrefixes fails rt unless ps is the shortest list covering its
// addresses: masked IPv4 prefixes, sorted, disjoint, and no two of them the
// halves of one larger prefix, which together they would have to be.
func checkPrefixes(rt *rapid.T, ps []netip.Prefix) {
	seen := make(map[netip.Prefix]bool, len(ps))
	for i, p := range ps {
		if !p.Addr().Is4() || p != p.Masked() {
			rt.Fatalf("Prefixes holds %s", p)
		}
		if i > 0 {
			if _, last := bounds(ps[i-1]); last.Compare(p.Addr()) >= 0 {
				rt.Fatalf("Prefixes are unsorted or overlap: %s, %s", ps[i-1], p)
			}
		}
		seen[p] = true
	}
	for _, p := range ps {
		if p.Bits() == 0 {
			continue
		}
		// The sibling differs from p in p's last network bit.
		sibling := netip.PrefixFrom(addr(u32(p.Addr())^uint32(1)<<(32-p.Bits())), p.Bits())
		if seen[sibling] {
			rt.Fatalf("Prefixes is not the shortest: %s and %s are halves of one prefix", p, sibling)
		}
	}
}

// bounds returns the first and last address of the IPv4 prefix p.
func bounds(p netip.Prefix) (netip.Addr, netip.Addr) {
	p = p.Masked()
	return p.Addr(), addr(u32(p.Addr()) | ^uint32(0)>>p.Bits())
}

func u32(a netip.Addr) uint32 {
	b := a.As4()
	return binary.BigEndian.Uint32(b[:])
}

func addr(v uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}

var protocol = rapid.SampledFrom([]model.Protocol{model.ProtocolTCP, model.ProtocolUDP})

// port draws ports from a few that ranges share, so generated ranges overlap
// and touch, or from the whole space, ends included.
var port = rapid.OneOf(
	rapid.Uint16Range(0, 16),
	rapid.SampledFrom([]uint16{0, 1, 1023, 1024, 32767, 32768, 65534, 65535}),
	rapid.Uint16(),
)

var portRange = rapid.Custom(func(rt *rapid.T) model.PortRange {
	p := rapid.SampledFrom([]model.Protocol{model.ProtocolTCP, model.ProtocolUDP, model.ProtocolAll}).Draw(rt, "protocol")
	a, b := port.Draw(rt, "from"), port.Draw(rt, "to")
	return model.PortRange{Protocol: p, From: min(a, b), To: max(a, b)}
})

// prefix draws masked IPv4 prefixes, mostly inside 10.0.0.0/8 so that drawn
// prefixes nest and overlap, sometimes anywhere.
var prefix = rapid.Custom(func(rt *rapid.T) netip.Prefix {
	var a netip.Addr
	if rapid.Bool().Draw(rt, "anywhere") {
		a = addr(rapid.Uint32().Draw(rt, "address"))
	} else {
		a = netip.AddrFrom4([4]byte{10, 0, rapid.Byte().Draw(rt, "third"), rapid.Byte().Draw(rt, "fourth")})
	}
	bits := rapid.IntRange(0, 32).Draw(rt, "bits")
	return netip.PrefixFrom(a, bits).Masked()
})

// address draws an address at an end of a drawn prefix or just outside it,
// where membership is most often wrong.
var address = rapid.Custom(func(rt *rapid.T) netip.Addr {
	first, last := bounds(prefix.Draw(rt, "around"))
	switch rapid.IntRange(0, 3).Draw(rt, "where") {
	case 0:
		return first
	case 1:
		return last
	case 2:
		if p := first.Prev(); p.IsValid() {
			return p
		}
		return first
	default:
		if n := last.Next(); n.IsValid() {
			return n
		}
		return last
	}
})

func portSets(s sets.Space) *rapid.Generator[sets.GateSet] {
	return rapid.Custom(func(rt *rapid.T) sets.GateSet {
		return s.Ports(rapid.SliceOfN(portRange, 0, 4).Draw(rt, "ranges")...)
	})
}

func addrSets(s sets.Space) *rapid.Generator[sets.AddrSet] {
	return rapid.Custom(func(rt *rapid.T) sets.AddrSet {
		return s.Addrs(rapid.SliceOfN(prefix, 0, 4).Draw(rt, "prefixes")...)
	})
}
