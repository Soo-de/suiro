package model_test

import (
	"cmp"
	"encoding/binary"
	"net/netip"
	"slices"
	"testing"

	"github.com/Soo-de/suiro/internal/model"
)

// ianaSpecialPurpose is the IANA IPv4 Special-Purpose Address Registry as
// last updated on 2025-10-09
// (https://www.iana.org/assignments/iana-ipv4-special-registry/), one row
// per entry with its Globally Reachable column. reachable is "" where the
// registry leaves the column blank (a terminated entry).
var ianaSpecialPurpose = []struct {
	block     string
	name      string
	reachable string
}{
	{"0.0.0.0/8", "This network [RFC 791]", "false"},
	{"0.0.0.0/32", "This host on this network [RFC 1122]", "false"},
	{"10.0.0.0/8", "Private-Use [RFC 1918]", "false"},
	{"100.64.0.0/10", "Shared Address Space [RFC 6598]", "false"},
	{"127.0.0.0/8", "Loopback [RFC 1122]", "false"},
	{"169.254.0.0/16", "Link Local [RFC 3927]", "false"},
	{"172.16.0.0/12", "Private-Use [RFC 1918]", "false"},
	{"192.0.0.0/24", "IETF Protocol Assignments [RFC 6890]", "false"},
	{"192.0.0.0/29", "IPv4 Service Continuity Prefix [RFC 7335]", "false"},
	{"192.0.0.8/32", "IPv4 dummy address [RFC 7600]", "false"},
	{"192.0.0.9/32", "Port Control Protocol Anycast [RFC 7723]", "true"},
	{"192.0.0.10/32", "Traversal Using Relays around NAT Anycast [RFC 8155]", "true"},
	{"192.0.0.170/32", "NAT64/DNS64 Discovery [RFC 8880]", "false"},
	{"192.0.0.171/32", "NAT64/DNS64 Discovery [RFC 8880]", "false"},
	{"192.0.2.0/24", "Documentation (TEST-NET-1) [RFC 5737]", "false"},
	{"192.31.196.0/24", "AS112-v4 [RFC 7535]", "true"},
	{"192.52.193.0/24", "AMT [RFC 7450]", "true"},
	{"192.88.99.0/24", "Deprecated (6to4 Relay Anycast) [RFC 7526]", ""},
	{"192.88.99.2/32", "6a44-relay anycast address [RFC 6751]", "false"},
	{"192.168.0.0/16", "Private-Use [RFC 1918]", "false"},
	{"192.175.48.0/24", "Direct Delegation AS112 Service [RFC 7534]", "true"},
	{"198.18.0.0/15", "Benchmarking [RFC 2544]", "false"},
	{"198.51.100.0/24", "Documentation (TEST-NET-2) [RFC 5737]", "false"},
	{"203.0.113.0/24", "Documentation (TEST-NET-3) [RFC 5737]", "false"},
	{"240.0.0.0/4", "Reserved [RFC 1112]", "false"},
	{"255.255.255.255/32", "Limited Broadcast [RFC 8190]", "false"},
}

// multicast is the IPv4 multicast block of RFC 5771, which the
// special-purpose registry does not list.
const multicast = "224.0.0.0/4"

func TestSpecialPurposeIPv4MatchesTheRegistry(t *testing.T) {
	// Not globally reachable, less the more specific blocks inside them the
	// registry marks reachable, plus multicast.
	var notReachable, reachable []netip.Prefix
	for _, r := range ianaSpecialPurpose {
		switch r.reachable {
		case "false":
			notReachable = append(notReachable, netip.MustParsePrefix(r.block))
		case "true":
			reachable = append(reachable, netip.MustParsePrefix(r.block))
		}
	}
	notReachable = append(notReachable, netip.MustParsePrefix(multicast))
	want := spansOf(notReachable...).minus(spansOf(reachable...))

	got := model.SpecialPurposeIPv4()
	checkSortedDisjoint(t, got)
	g := spansOf(got...)
	if missing := want.minus(g); len(missing) != 0 {
		t.Errorf("SpecialPurposeIPv4 lacks %v", missing)
	}
	if extra := g.minus(want); len(extra) != 0 {
		t.Errorf("SpecialPurposeIPv4 holds %v, which the registry does not", extra)
	}
}

func TestPrivateIPv4MatchesTheRFCs(t *testing.T) {
	want := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),     // RFC 1918, section 3
		netip.MustParsePrefix("100.64.0.0/10"),  // RFC 6598, section 7
		netip.MustParsePrefix("172.16.0.0/12"),  // RFC 1918, section 3
		netip.MustParsePrefix("192.168.0.0/16"), // RFC 1918, section 3
	}
	got := model.PrivateIPv4()
	checkSortedDisjoint(t, got)
	if !slices.Equal(spansOf(got...), spansOf(want...)) {
		t.Errorf("PrivateIPv4 = %v, want %v", got, want)
	}
	// Private addresses are never the internet.
	if left := spansOf(got...).minus(spansOf(model.SpecialPurposeIPv4()...)); len(left) != 0 {
		t.Errorf("PrivateIPv4 holds %v outside SpecialPurposeIPv4", left)
	}
}

func TestAddressBlocksAreCopies(t *testing.T) {
	tests := []struct {
		name string
		get  func() []netip.Prefix
	}{
		{"SpecialPurposeIPv4", model.SpecialPurposeIPv4},
		{"PrivateIPv4", model.PrivateIPv4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.get()
			got := tt.get()
			got[0] = netip.MustParsePrefix("8.8.8.0/24")
			if !slices.Equal(tt.get(), want) {
				t.Errorf("changing the result of %s changed the next one", tt.name)
			}
		})
	}
}

// checkSortedDisjoint fails t unless ps is sorted by address and no two
// prefixes overlap.
func checkSortedDisjoint(t *testing.T, ps []netip.Prefix) {
	t.Helper()
	for i := 1; i < len(ps); i++ {
		if ps[i-1].Addr().Compare(ps[i].Addr()) >= 0 || ps[i-1].Overlaps(ps[i]) {
			t.Errorf("%v and %v are out of order or overlap", ps[i-1], ps[i])
		}
	}
}

// span is an inclusive range of IPv4 addresses as integers.
type span struct{ lo, hi uint32 }

func (s span) String() string {
	var lo, hi [4]byte
	binary.BigEndian.PutUint32(lo[:], s.lo)
	binary.BigEndian.PutUint32(hi[:], s.hi)
	return netip.AddrFrom4(lo).String() + "-" + netip.AddrFrom4(hi).String()
}

// spans is a sorted list of disjoint, non-adjacent spans, so equal address
// sets have equal spans.
type spans []span

func spansOf(ps ...netip.Prefix) spans {
	var out spans
	for _, p := range ps {
		lo := binary.BigEndian.Uint32(p.Masked().Addr().AsSlice())
		out = append(out, span{lo, lo | (1<<(32-p.Bits()) - 1)})
	}
	slices.SortFunc(out, func(a, b span) int { return cmp.Compare(a.lo, b.lo) })
	var merged spans
	for _, s := range out {
		if n := len(merged); n > 0 && uint64(s.lo) <= uint64(merged[n-1].hi)+1 {
			merged[n-1].hi = max(merged[n-1].hi, s.hi)
			continue
		}
		merged = append(merged, s)
	}
	return merged
}

// minus returns the addresses of a that are not in b.
func (a spans) minus(b spans) spans {
	var out spans
	for _, s := range a {
		// Cut s by every span of b that overlaps it, left to right.
		for _, c := range b {
			if c.hi < s.lo || c.lo > s.hi {
				continue
			}
			if c.lo > s.lo {
				out = append(out, span{s.lo, c.lo - 1})
			}
			if c.hi >= s.hi {
				s.lo, s.hi = 1, 0 // nothing left
				break
			}
			s.lo = c.hi + 1
		}
		if s.lo <= s.hi {
			out = append(out, s)
		}
	}
	return out
}
