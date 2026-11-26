package sets_test

import (
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
