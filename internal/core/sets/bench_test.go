package sets_test

import (
	"encoding/json"
	"net/netip"
	"os"
	"testing"

	"github.com/Soo-de/suiro/internal/core/sets"
	"github.com/Soo-de/suiro/internal/model"
)

// benchInput holds the two operands of every operation as NetworkPolicy rules
// write them: the port list of each rule, and ipBlocks with their except
// entries.
type benchInput struct {
	Ports struct {
		A [][]model.PortRange `json:"a"`
		B [][]model.PortRange `json:"b"`
	} `json:"ports"`
	IPBlocks struct {
		A []ipBlock `json:"a"`
		B []ipBlock `json:"b"`
	} `json:"ipBlocks"`
}

type ipBlock struct {
	CIDR   netip.Prefix   `json:"cidr"`
	Except []netip.Prefix `json:"except"`
}

// benchSizes take the first 1/divisor of the rules, so large is ten times
// small: a linear operation's time grows about as much as the "intervals"
// metric, a quadratic one's about as its square.
var benchSizes = []struct {
	name    string
	divisor int
}{
	{"small", 10},
	{"large", 1},
}

func loadBenchInput(b *testing.B) benchInput {
	b.Helper()
	data, err := os.ReadFile("testdata/networkpolicy.json")
	if err != nil {
		b.Fatal(err)
	}
	var in benchInput
	if err := json.Unmarshal(data, &in); err != nil {
		b.Fatal(err)
	}
	return in
}

func portUnion(s sets.Space, rules [][]model.PortRange) sets.GateSet {
	var all []model.PortRange
	for _, r := range rules {
		all = append(all, r...)
	}
	return s.Ports(all...)
}

func addrUnion(s sets.Space, blocks []ipBlock) sets.AddrSet {
	out := s.Addrs()
	for _, bl := range blocks {
		out = out.Union(s.Addrs(bl.CIDR).Diff(s.Addrs(bl.Except...)))
	}
	return out
}

func BenchmarkPortSets(b *testing.B) {
	in := loadBenchInput(b)
	for _, size := range benchSizes {
		s := sets.NewIntervalSpace()
		x := portUnion(s, in.Ports.A[:len(in.Ports.A)/size.divisor])
		y := portUnion(s, in.Ports.B[:len(in.Ports.B)/size.divisor])
		intervals := float64(len(x.Ranges()) + len(y.Ranges()))

		// Both ends of every range of either operand and the ports just
		// outside them, so probes hit and miss across the whole set.
		type probe struct {
			proto model.Protocol
			port  uint16
		}
		var probes []probe
		for _, r := range append(x.Ranges(), y.Ranges()...) {
			for _, p := range []uint16{r.From - 1, r.From, r.To, r.To + 1} {
				probes = append(probes, probe{r.Protocol, p})
			}
		}

		ops := []struct {
			name string
			run  func(sets.GateSet) sets.GateSet
		}{
			{"union", x.Union},
			{"intersect", x.Intersect},
			{"diff", x.Diff},
		}
		for _, op := range ops {
			b.Run(op.name+"/"+size.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					op.run(y)
				}
				b.ReportMetric(intervals, "intervals")
			})
		}
		b.Run("contains/"+size.name, func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				p := probes[i%len(probes)]
				x.Contains(p.proto, p.port)
				i++
			}
			b.ReportMetric(intervals, "intervals")
		})
	}
}

func BenchmarkAddrSets(b *testing.B) {
	in := loadBenchInput(b)
	for _, size := range benchSizes {
		s := sets.NewIntervalSpace()
		x := addrUnion(s, in.IPBlocks.A[:len(in.IPBlocks.A)/size.divisor])
		y := addrUnion(s, in.IPBlocks.B[:len(in.IPBlocks.B)/size.divisor])
		// Prefixes, not intervals, are what the interface can count; they grow
		// with the intervals.
		prefixes := float64(len(x.Prefixes()) + len(y.Prefixes()))

		// The first address of every prefix of either operand and the one
		// before it.
		var probes []netip.Addr
		for _, p := range append(x.Prefixes(), y.Prefixes()...) {
			probes = append(probes, p.Addr().Prev(), p.Addr())
		}

		ops := []struct {
			name string
			run  func(sets.AddrSet) sets.AddrSet
		}{
			{"union", x.Union},
			{"intersect", x.Intersect},
			{"diff", x.Diff},
		}
		for _, op := range ops {
			b.Run(op.name+"/"+size.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					op.run(y)
				}
				b.ReportMetric(prefixes, "prefixes")
			})
		}
		b.Run("contains/"+size.name, func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				x.Contains(probes[i%len(probes)])
				i++
			}
			b.ReportMetric(prefixes, "prefixes")
		})
	}
}
