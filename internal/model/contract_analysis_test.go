package model_test

import (
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/Soo-de/suiro/internal/model"
)

func checkAnalysis(t *testing.T, a, want model.Analysis, valid bool) {
	t.Helper()
	first := a.Validate()
	if valid && first != nil {
		t.Errorf("Validate = %v, want nil", first)
	}
	if !valid && first == nil {
		t.Error("Validate = nil, want an error")
	}
	for range 3 {
		if again := a.Validate(); (again == nil) != (first == nil) || again != nil && again.Error() != first.Error() {
			t.Errorf("a later call gave %v, the first %v", again, first)
		}
	}
	if !reflect.DeepEqual(a, want) {
		t.Errorf("Validate changed the analysis:\ngot  %+v\nwant %+v", a, want)
	}
}

// place is a class at one location; its addresses are only there to order
// classes that share a location's node group and subnet.
func place(name, nodeGroup, subnet string, addresses ...string) model.PlacementClass {
	c := model.PlacementClass{Name: name, NodeGroup: nodeGroup, Subnet: subnet, Addresses: []netip.Prefix{}}
	for _, a := range addresses {
		c.Addresses = append(c.Addresses, netip.MustParsePrefix(a))
	}
	return c
}

func workload(key model.EndpointKey, classes ...model.PlacementClass) model.Endpoint {
	return model.Endpoint{Key: key, Kind: model.EndpointKindWorkload, Name: string(key), Classes: classes}
}

// pairAnalysis holds one path from a to b.
func pairAnalysis(a, b []model.PlacementClass, path model.Path) model.Analysis {
	path.Key = model.PathKey{Source: "a", Destination: "b", Ports: model.PortRange{Protocol: model.ProtocolTCP, From: 80, To: 80}}
	return model.Analysis{
		Kind: model.AnalysisBaseline, Status: model.AnalysisDone,
		Inputs:     []model.SnapshotRef{},
		Endpoints:  []model.Endpoint{workload("a", slices.Clone(a)...), workload("b", slices.Clone(b)...)},
		Paths:      []model.Path{path},
		Unexamined: []model.Unexamined{},
	}
}

func twoSubnets() []model.PlacementClass {
	return []model.PlacementClass{
		place("ng1/subnet-a", "ng1", "subnet-a", "10.0.1.0/24"),
		place("ng1/subnet-b", "ng1", "subnet-b", "10.0.2.0/24"),
	}
}

type pathCase struct {
	verdict  model.Verdict
	outcomes string
	valid    bool
}

func runPaths(t *testing.T, a, b []model.PlacementClass, sameNode bool, reasons []model.ReasonCode, cases []pathCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(string(c.verdict)+" "+c.outcomes, func(t *testing.T) {
			build := func() model.Analysis {
				return pairAnalysis(a, b, model.Path{
					Verdict: c.verdict, Outcomes: c.outcomes, SameNode: sameNode, Reasons: slices.Clone(reasons),
				})
			}
			checkAnalysis(t, build(), build(), c.valid)
		})
	}
}

// Both ends have the classes (ng1, subnet-a) and (ng1, subnet-b), so a
// SameNode path has 4 class pairs and 2 SameNode pairs. The verdicts are the
// table of the Verdict type.
func TestAnalysisValidateVerdictAndOutcomes(t *testing.T) {
	runPaths(t, twoSubnets(), twoSubnets(), true, nil, []pathCase{
		{model.Allow, "", true},
		{model.Deny, "", true},
		{model.Unknown, "", true},
		{model.PlacementDependent, "", false},

		{model.PlacementDependent, "PBPPPP", true},
		{model.PlacementDependent, "BBBBBP", true},
		{model.PlacementDependent, "PBUUUU", true},
		{model.Unknown, "PUUUUU", true},
		{model.Unknown, "BUUUUU", true},
		{model.Unknown, "UUUUUP", true},

		{model.Allow, "PBPPPP", false},
		{model.Deny, "PBPPPP", false},
		{model.Unknown, "PBPPPP", false},
		{model.Unknown, "PBUUUU", false},
		{model.PlacementDependent, "PUUUUU", false},
		{model.PlacementDependent, "BUUUUU", false},
		{model.Allow, "PUUUUU", false},
		{model.Deny, "BUUUUU", false},

		// All the same is written as "".
		{model.Allow, "PPPPPP", false},
		{model.Deny, "BBBBBB", false},
		{model.Unknown, "UUUUUU", false},
		{model.PlacementDependent, "PPPPPP", false},

		// One letter per class pair, from "P", "B" and "U".
		{model.PlacementDependent, "PBPPP", false},
		{model.PlacementDependent, "PBPPPPP", false},
		{model.PlacementDependent, "PBPP", false},
		{model.PlacementDependent, "PBPPPX", false},
		{model.PlacementDependent, "PBPPPp", false},
		{model.PlacementDependent, "PBPPP ", false},
		{model.Unknown, "PUUUUX", false},
	})
}

func TestAnalysisValidateClassPairCount(t *testing.T) {
	var (
		ng1a    = place("ng1/subnet-a", "ng1", "subnet-a", "10.0.1.0/24")
		ng1b    = place("ng1/subnet-b", "ng1", "subnet-b", "10.0.2.0/24")
		ng1c    = place("ng1/subnet-c", "ng1", "subnet-c", "10.0.3.0/24")
		ng2a    = place("ng2/subnet-a", "ng2", "subnet-a", "10.0.1.0/24")
		low     = place("ng1/subnet-a low", "ng1", "subnet-a", "10.0.1.0/25")
		high    = place("ng1/subnet-a high", "ng1", "subnet-a", "10.0.1.128/25")
		unknown = place(model.ClassUnknown, "", "")
		cluster = place(model.ClassCluster, "", "")
	)
	type classes = []model.PlacementClass
	tests := []struct {
		name     string
		a, b     classes
		sameNode bool
		pairs    int
	}{
		{"two by two", classes{ng1a, ng1b}, classes{ng1a, ng1b}, false, 4},
		{"two by two on one node", classes{ng1a, ng1b}, classes{ng1a, ng1b}, true, 6},
		{"three by two", classes{ng1a, ng1b, ng1c}, classes{ng1a, ng1b}, false, 6},
		{"node group and subnet both match", classes{ng1a, ng1b}, classes{ng1b, ng2a}, true, 5},
		{"split source classes", classes{low, high}, classes{ng1a}, true, 4},
		{"split classes on both ends", classes{low, high}, classes{low, high}, true, 8},
		{"unknown placement on both ends", classes{unknown}, classes{unknown}, true, 2},
		{"unknown placement without SameNode", classes{unknown}, classes{unknown}, false, 1},
		{"cluster classes", classes{cluster}, classes{cluster}, false, 1},
		{"no shared location", classes{ng1a}, classes{ng2a}, true, 1},
	}
	// mixed returns a placement-dependent outcome string of n letters.
	mixed := func(n int) string {
		out := []byte("PB")
		for len(out) < n {
			out = append(out, 'P')
		}
		return string(out[:n])
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cases := []pathCase{{model.Allow, "", true}}
			// One class pair cannot have two different outcomes.
			if tt.pairs > 1 {
				cases = append(cases, pathCase{model.PlacementDependent, mixed(tt.pairs), true})
				cases = append(cases, pathCase{model.PlacementDependent, mixed(tt.pairs + 1), false})
			} else {
				cases = append(cases,
					pathCase{model.PlacementDependent, "PB", false},
					pathCase{model.Allow, "P", false},
					pathCase{model.PlacementDependent, "P", false},
				)
			}
			if tt.pairs > 2 {
				cases = append(cases, pathCase{model.PlacementDependent, mixed(tt.pairs - 1), false})
			}
			runPaths(t, tt.a, tt.b, tt.sameNode, nil, cases)
		})
	}
}

// An input-level reason makes every class pair Undetermined, so the path is
// Unknown with no outcome string. A code this build does not know counts as
// input-level (LevelOf).
func TestAnalysisValidateInputReasons(t *testing.T) {
	codes := []model.ReasonCode{"no-such-code-in-this-build"}
	for code, spec := range model.Reasons() {
		if spec.Level == model.LevelInput {
			codes = append(codes, code)
		}
	}
	slices.Sort(codes)
	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			runPaths(t, twoSubnets(), twoSubnets(), true, []model.ReasonCode{code}, []pathCase{
				{model.Unknown, "", true},
				{model.Allow, "", false},
				{model.Deny, "", false},
				{model.PlacementDependent, "PBPPPP", false},
				{model.Unknown, "PUUUUU", false},
				{model.Unknown, "BUUUUU", false},
			})
		})
	}
	t.Run("beside a gate reason", func(t *testing.T) {
		reasons := []model.ReasonCode{model.ReasonHostNetwork, model.ReasonRenderFailed}
		runPaths(t, twoSubnets(), twoSubnets(), true, reasons, []pathCase{
			{model.Unknown, "", true},
			{model.Deny, "", false},
			{model.Unknown, "PUUUUU", false},
		})
	})
}

func TestAnalysisValidateGateReasons(t *testing.T) {
	runPaths(t, twoSubnets(), twoSubnets(), true, []model.ReasonCode{model.ReasonRouteOutOfScope}, []pathCase{
		{model.Unknown, "", true},
		{model.Unknown, "PUUUUU", true},
		{model.Unknown, "BUUUUU", true},
		{model.Deny, "", true},
		{model.PlacementDependent, "PBUUUU", true},
	})
}

func TestAnalysisValidatePathEnds(t *testing.T) {
	base := func() model.Analysis {
		return pairAnalysis(twoSubnets(), twoSubnets(), model.Path{Verdict: model.Allow})
	}
	checkAnalysis(t, base(), base(), true)
	tests := []struct {
		name   string
		change func(a *model.Analysis)
	}{
		{"missing source", func(a *model.Analysis) { a.Paths[0].Key.Source = "0" }},
		{"missing destination", func(a *model.Analysis) { a.Paths[0].Key.Destination = "c" }},
		{"no endpoints", func(a *model.Analysis) { a.Endpoints = []model.Endpoint{} }},
		{"source without classes", func(a *model.Analysis) { a.Endpoints[0].Classes = []model.PlacementClass{} }},
		{"destination without classes", func(a *model.Analysis) { a.Endpoints[1].Classes = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, want := base(), base()
			tt.change(&a)
			tt.change(&want)
			checkAnalysis(t, a, want, false)
		})
	}
}

// Positions of the endpoints in orderedAnalysis.
const (
	databaseEnd = iota
	internetEnd
	apiEnd
	webEnd
)

// orderedAnalysis is a fresh analysis whose every list is in the canonical
// order of the package comment. Where a list has several sort keys, each
// neighbor differs in an earlier key while its later keys run the other way.
func orderedAnalysis() model.Analysis {
	const (
		database model.EndpointKey = "aws:database/orders-db"
		api      model.EndpointKey = "k8s:apps/Deployment/payments/api"
		web      model.EndpointKey = "k8s:apps/Deployment/payments/web"
	)
	key := func(source, destination model.EndpointKey, protocol model.Protocol, from, to uint16) model.PathKey {
		return model.PathKey{Source: source, Destination: destination, Ports: model.PortRange{Protocol: protocol, From: from, To: to}}
	}
	keys := func() []model.PathKey {
		return []model.PathKey{
			key(model.EndpointInternet, web, model.ProtocolUDP, 500, 500),
			key(api, database, model.ProtocolUDP, 500, 500),
			key(api, web, model.ProtocolTCP, 80, 80),
			key(api, web, model.ProtocolTCP, 443, 443),
			key(api, web, model.ProtocolUDP, 0, 65535),
			key(web, database, model.ProtocolTCP, 0, 65535),
		}
	}
	paths := []model.Path{}
	for _, k := range keys() {
		paths = append(paths, model.Path{Key: k, Verdict: model.Allow})
	}
	paths[2] = model.Path{Key: keys()[2], Verdict: model.PlacementDependent, Outcomes: "PBPP"}
	paths[3] = model.Path{
		Key: keys()[3], Verdict: model.Unknown,
		Reasons: []model.ReasonCode{model.ReasonRouteMiddlebox, model.ReasonRouteOutOfScope},
	}

	withGroups := func(c model.PlacementClass) model.PlacementClass {
		c.SecurityGroups = []string{"sg-0aaa", "sg-0bbb"}
		return c
	}
	apiClasses := []model.PlacementClass{withGroups(twoSubnets()[0]), twoSubnets()[1]}
	webClasses := twoSubnets()

	pairs := func() []model.ClassPair {
		// Fresh classes each time, so a change to one pair's list stays there.
		a := func() model.PlacementClass {
			return withGroups(place("ng1/subnet-a", "ng1", "subnet-a", "10.0.1.0/26", "10.0.1.128/26"))
		}
		b := func() model.PlacementClass { return withGroups(twoSubnets()[1]) }
		return []model.ClassPair{
			{Source: a(), Destination: a()},
			{Source: a(), Destination: a(), SameNode: true},
			{Source: a(), Destination: b()},
			{Source: b(), Destination: a()},
		}
	}
	change := func(k model.PathKey) model.Change {
		return model.Change{
			Key: k, Before: model.Deny, After: model.Allow,
			Opened: pairs(), Closed: pairs(), PossiblyOpened: pairs(), PossiblyClosed: pairs(), Undetermined: pairs(),
		}
	}
	finding := func(rule, subject string, kind model.FindingKind, locator string) model.Finding {
		return model.Finding{
			Rule: rule, Subject: subject, Kind: kind, Status: model.Violated,
			Origin: model.Origin{Source: "payments/api", Locator: locator},
		}
	}
	findings := []model.Finding{
		finding("a", "b", model.FindingUnmatched, "z"),
		finding("b", "a", model.FindingRequired, "z"),
		finding("b", "b", model.FindingForbidden, "z"),
		finding("b", "b", model.FindingRequired, "a"),
		finding("b", "b", model.FindingRequired, "b"),
	}
	findings[0].Paths = keys()
	findings[1].Reasons = []model.ReasonCode{model.ReasonRouteMiddlebox, model.ReasonRouteOutOfScope}

	origin := func(source, file string, line int, resource, locator string) *model.Origin {
		return &model.Origin{Source: model.SourceID(source), File: file, Line: line, Resource: resource, Locator: locator}
	}
	problem := func(reason model.ReasonCode, subject string, o *model.Origin, message string) model.Problem {
		return model.Problem{Reason: reason, Subject: subject, Origin: o, Message: message}
	}
	const first, second = model.ReasonHostNetwork, model.ReasonHostPort
	problems := []model.Problem{
		problem(first, "s", nil, "z"),
		problem(second, "r", origin("z/z", "z", 9, "z", "z"), ""),
		problem(second, "s", nil, "y"),
		problem(second, "s", nil, "z"),
		problem(second, "s", origin("a/a", "b", 9, "z", "z"), ""),
		problem(second, "s", origin("b/b", "a", 2, "z", "z"), ""),
		problem(second, "s", origin("b/b", "a", 10, "i-1", "b"), ""),
		problem(second, "s", origin("b/b", "a", 10, "i-2", "a"), ""),
		problem(second, "s", origin("b/b", "a", 10, "i-2", "b"), "a"),
		problem(second, "s", origin("b/b", "a", 10, "i-2", "b"), "b"),
		problem(second, "s", origin("b/b", "b", 1, "", ""), ""),
	}

	return model.Analysis{
		Kind: model.AnalysisPullRequest, Status: model.AnalysisPartial,
		Inputs: []model.SnapshotRef{
			{Key: model.ReadKey{Source: "aws/prod", PullRequest: 9}, Digest: sha("a")},
			{Key: model.ReadKey{Source: "payments/api"}, Digest: sha("b")},
			{Key: model.ReadKey{Source: "payments/api", PullRequest: 2}, Digest: sha("c")},
			{Key: model.ReadKey{Source: "payments/api", PullRequest: 10}, Digest: sha("d")},
		},
		Profile: model.ClusterProfile{NodeGroups: []model.NodeGroup{{
			Name: "ng1", Subnets: []string{"subnet-a", "subnet-b"}, SecurityGroups: []string{"sg-0aaa", "sg-0bbb"},
		}}},
		Endpoints: []model.Endpoint{
			databaseEnd: {
				Key: database, Kind: model.EndpointKindResource, ResourceKind: model.ResourceDatabase, Name: "orders-db",
				// Node group before subnet, subnet before addresses.
				Classes: []model.PlacementClass{
					place("z", "", "subnet-b", "10.0.9.0/24"),
					place("y", "ng1", "subnet-a", "10.0.8.0/24"),
					place("x", "ng1", "subnet-b", "10.0.7.0/24"),
					place("w", "ng2", "subnet-a", "10.0.6.0/24"),
				},
			},
			internetEnd: {
				Key: model.EndpointInternet, Kind: model.EndpointKindInternet, Name: "internet",
				// Addresses as lists, each prefix by address then length, then
				// the name.
				Classes: []model.PlacementClass{
					place("z", "", "", "1.0.0.0/8"),
					place("y", "", "", "1.0.0.0/8", "3.0.0.0/8"),
					place("x", "", "", "1.0.0.0/9"),
					place("a", "", "", "2.0.0.0/8", "9.0.0.0/8"),
					place("b", "", "", "2.0.0.0/8", "9.0.0.0/8"),
				},
			},
			apiEnd: {
				Key: api, Kind: model.EndpointKindWorkload, Name: "api", Classes: apiClasses,
				Services: []model.EndpointService{
					{Name: "payments/api", Mappings: []model.PortMapping{
						{Protocol: model.ProtocolTCP, Port: 443, TargetPort: 8443},
						{Protocol: model.ProtocolUDP, Port: 53, TargetPort: 53},
						{Protocol: model.ProtocolUDP, Port: 500, TargetPort: 500},
					}},
					{Name: "payments/api-internal", Mappings: []model.PortMapping{
						{Protocol: model.ProtocolTCP, Port: 80, TargetPort: 8080},
						{Protocol: model.ProtocolTCP, Port: 443, TargetPort: 8443},
					}},
				},
			},
			webEnd: {Key: web, Kind: model.EndpointKindWorkload, Name: "web", Classes: webClasses},
		},
		Paths:    paths,
		Changes:  []model.Change{change(keys()[0]), change(keys()[2]), change(keys()[3]), change(keys()[4])},
		Findings: findings,
		Problems: problems,
		Unexamined: []model.Unexamined{
			model.UnexaminedApplication, model.UnexaminedHostFirewall, model.UnexaminedIPv6,
			model.UnexaminedTLS, model.UnexaminedPairs,
		},
	}
}

type analysisChange struct {
	name   string
	change func(a *model.Analysis)
}

// unordered returns, for the list get picks, a change for each swap of two
// neighbors and one for a repeated entry.
func unordered[T any](name string, get func(a *model.Analysis) *[]T) []analysisChange {
	sample := orderedAnalysis()
	var out []analysisChange
	for i := range len(*get(&sample)) - 1 {
		out = append(out, analysisChange{name + " swapped at " + string(rune('a'+i)), func(a *model.Analysis) {
			s := *get(a)
			s[i], s[i+1] = s[i+1], s[i]
		}})
	}
	return append(out, analysisChange{name + " repeated", func(a *model.Analysis) {
		s := get(a)
		*s = slices.Insert(*s, 1, (*s)[0])
	}})
}

func runAnalyses(t *testing.T, valid bool, tests []analysisChange) {
	t.Helper()
	base := orderedAnalysis()
	if err := base.Validate(); err != nil {
		t.Fatalf("the base analysis is invalid: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, want := orderedAnalysis(), orderedAnalysis()
			tt.change(&a)
			tt.change(&want)
			checkAnalysis(t, a, want, valid)
		})
	}
}

func TestAnalysisValidateOrdered(t *testing.T) {
	checkAnalysis(t, orderedAnalysis(), orderedAnalysis(), true)
}

func TestAnalysisValidateIdenticalProblems(t *testing.T) {
	var tests []analysisChange
	// A problem without an origin, one with an origin and the last one.
	for _, i := range []int{0, 4, 10} {
		tests = append(tests, analysisChange{"repeated " + strconv.Itoa(i), func(a *model.Analysis) {
			p := a.Problems[i]
			if p.Origin != nil {
				// An equal origin behind another pointer is still identical.
				o := *p.Origin
				p.Origin = &o
			}
			a.Problems = slices.Insert(a.Problems, i+1, p)
		}})
	}
	runAnalyses(t, true, tests)
}

func TestAnalysisValidateFailed(t *testing.T) {
	failed := func() model.Analysis {
		return model.Analysis{
			Kind: model.AnalysisBaseline, Status: model.AnalysisFailed,
			Inputs: []model.SnapshotRef{}, Endpoints: []model.Endpoint{}, Paths: []model.Path{},
			Problems:   []model.Problem{{Reason: model.ReasonAnalysisTimeout, Subject: "prod"}},
			Unexamined: []model.Unexamined{},
		}
	}
	checkAnalysis(t, failed(), failed(), true)
}

func TestAnalysisValidateListOrder(t *testing.T) {
	type A = model.Analysis
	tests := slices.Concat(
		unordered("inputs", func(a *A) *[]model.SnapshotRef { return &a.Inputs }),
		unordered("endpoints", func(a *A) *[]model.Endpoint { return &a.Endpoints }),
		unordered("paths", func(a *A) *[]model.Path { return &a.Paths }),
		unordered("path reasons", func(a *A) *[]model.ReasonCode { return &a.Paths[3].Reasons }),
		unordered("changes", func(a *A) *[]model.Change { return &a.Changes }),
		unordered("findings", func(a *A) *[]model.Finding { return &a.Findings }),
		unordered("finding paths", func(a *A) *[]model.PathKey { return &a.Findings[0].Paths }),
		unordered("finding reasons", func(a *A) *[]model.ReasonCode { return &a.Findings[1].Reasons }),
		unordered("unexamined", func(a *A) *[]model.Unexamined { return &a.Unexamined }),
	)
	runAnalyses(t, false, tests)
}

func TestAnalysisValidateProblemOrder(t *testing.T) {
	// Identical neighbors are valid, so problems are only swapped.
	tests := unordered("problems", func(a *model.Analysis) *[]model.Problem { return &a.Problems })
	runAnalyses(t, false, tests[:len(tests)-1])
}

func TestAnalysisValidateEndpointOrder(t *testing.T) {
	type A = model.Analysis
	tests := slices.Concat(
		unordered("classes by node group and subnet", func(a *A) *[]model.PlacementClass { return &a.Endpoints[databaseEnd].Classes }),
		unordered("classes by addresses and name", func(a *A) *[]model.PlacementClass { return &a.Endpoints[internetEnd].Classes }),
		// Without the repeated entry, which would also break class order here
		// (see TestAnalysisValidateAddressTie).
		unordered("class addresses", func(a *A) *[]netip.Prefix { return &a.Endpoints[internetEnd].Classes[4].Addresses })[:1],
		unordered("class security groups", func(a *A) *[]string { return &a.Endpoints[apiEnd].Classes[0].SecurityGroups }),
		unordered("services", func(a *A) *[]model.EndpointService { return &a.Endpoints[apiEnd].Services }),
		unordered("service mappings", func(a *A) *[]model.PortMapping { return &a.Endpoints[apiEnd].Services[0].Mappings }),
		unordered("second service mappings", func(a *A) *[]model.PortMapping { return &a.Endpoints[apiEnd].Services[1].Mappings }),
		unordered("profile node group subnets", func(a *A) *[]string { return &a.Profile.NodeGroups[0].Subnets }),
		unordered("profile node group security groups", func(a *A) *[]string { return &a.Profile.NodeGroups[0].SecurityGroups }),
	)
	runAnalyses(t, false, tests)
}

// changePairLists are the class pair lists of a Change.
var changePairLists = []struct {
	name string
	get  func(c *model.Change) *[]model.ClassPair
}{
	{"opened", func(c *model.Change) *[]model.ClassPair { return &c.Opened }},
	{"closed", func(c *model.Change) *[]model.ClassPair { return &c.Closed }},
	{"possibly opened", func(c *model.Change) *[]model.ClassPair { return &c.PossiblyOpened }},
	{"possibly closed", func(c *model.Change) *[]model.ClassPair { return &c.PossiblyClosed }},
	{"undetermined", func(c *model.Change) *[]model.ClassPair { return &c.Undetermined }},
}

func TestAnalysisValidateChangeOrder(t *testing.T) {
	var tests []analysisChange
	for _, l := range changePairLists {
		// The second change shows that every one is checked.
		pairs := func(a *model.Analysis) *[]model.ClassPair { return l.get(&a.Changes[1]) }
		tests = append(tests, unordered(l.name, pairs)...)
	}
	runAnalyses(t, false, tests)
}

// A list with a single class pair has no pair order to break, so only the
// order inside one of its classes decides.
func TestAnalysisValidateChangeClassListOrder(t *testing.T) {
	ends := []struct {
		name string
		get  func(p *model.ClassPair) *model.PlacementClass
	}{
		{"source", func(p *model.ClassPair) *model.PlacementClass { return &p.Source }},
		{"destination", func(p *model.ClassPair) *model.PlacementClass { return &p.Destination }},
	}
	faults := []struct {
		name   string
		change func(c *model.PlacementClass)
	}{
		{"addresses swapped", func(c *model.PlacementClass) { slices.Reverse(c.Addresses) }},
		{"address repeated", func(c *model.PlacementClass) { c.Addresses = []netip.Prefix{c.Addresses[0], c.Addresses[0]} }},
		{"security groups swapped", func(c *model.PlacementClass) { slices.Reverse(c.SecurityGroups) }},
		{"security group repeated", func(c *model.PlacementClass) {
			c.SecurityGroups = []string{c.SecurityGroups[0], c.SecurityGroups[0]}
		}},
	}
	var valid, broken []analysisChange
	for _, l := range changePairLists {
		// single cuts the list to its first pair, in the second change.
		single := func(a *model.Analysis) *model.ClassPair {
			s := l.get(&a.Changes[1])
			*s = (*s)[:1]
			return &(*s)[0]
		}
		valid = append(valid, analysisChange{l.name + " with one pair", func(a *model.Analysis) { single(a) }})
		for _, end := range ends {
			for _, f := range faults {
				broken = append(broken, analysisChange{l.name + " " + end.name + " " + f.name, func(a *model.Analysis) {
					f.change(end.get(single(a)))
				}})
			}
		}
	}
	runAnalyses(t, true, valid)
	runAnalyses(t, false, broken)
}

// The endpoint has a single class, so no class order can be broken.
func TestAnalysisValidateAddressTie(t *testing.T) {
	only := func(addresses ...string) func(a *model.Analysis) {
		return func(a *model.Analysis) {
			a.Endpoints[internetEnd].Classes = []model.PlacementClass{place("a", "", "", addresses...)}
		}
	}
	runAnalyses(t, true, []analysisChange{{"two addresses", only("2.0.0.0/8", "9.0.0.0/8")}})
	runAnalyses(t, false, []analysisChange{
		{"first address repeated", only("2.0.0.0/8", "2.0.0.0/8", "9.0.0.0/8")},
		{"last address repeated", only("2.0.0.0/8", "9.0.0.0/8", "9.0.0.0/8")},
	})
}

// Entries that differ outside their sort keys are still tied.
func TestAnalysisValidateTies(t *testing.T) {
	runAnalyses(t, false, []analysisChange{
		{"inputs with one key", func(a *model.Analysis) {
			r := a.Inputs[0]
			r.Digest = sha("z")
			a.Inputs = slices.Insert(a.Inputs, 1, r)
		}},
		{"endpoints with one key", func(a *model.Analysis) {
			e := a.Endpoints[internetEnd]
			e.Name = "other"
			a.Endpoints = slices.Insert(a.Endpoints, internetEnd+1, e)
		}},
		{"paths that differ in the range end", func(a *model.Analysis) {
			p := a.Paths[0]
			p.Key.Ports.To = 600
			a.Paths = slices.Insert(a.Paths, 1, p)
		}},
		{"changes that differ in the range end", func(a *model.Analysis) {
			c := a.Changes[0]
			c.Key.Ports.To = 600
			a.Changes = slices.Insert(a.Changes, 1, c)
		}},
		{"finding paths that differ in the range end", func(a *model.Analysis) {
			k := a.Findings[0].Paths[0]
			k.Ports.To = 600
			a.Findings[0].Paths = slices.Insert(a.Findings[0].Paths, 1, k)
		}},
		{"findings that differ in status", func(a *model.Analysis) {
			f := a.Findings[2]
			f.Status = model.Unverifiable
			a.Findings = slices.Insert(a.Findings, 3, f)
		}},
		{"services with one name", func(a *model.Analysis) {
			s := a.Endpoints[apiEnd].Services[0]
			s.Mappings = []model.PortMapping{{Protocol: model.ProtocolTCP, Port: 1}}
			a.Endpoints[apiEnd].Services = slices.Insert(a.Endpoints[apiEnd].Services, 1, s)
		}},
		{"mappings of one port", func(a *model.Analysis) {
			m := a.Endpoints[apiEnd].Services[0].Mappings[0]
			m.TargetPort = 9000
			a.Endpoints[apiEnd].Services[0].Mappings = slices.Insert(a.Endpoints[apiEnd].Services[0].Mappings, 1, m)
		}},
	})
}
