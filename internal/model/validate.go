package model

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"slices"
)

// The declared values of every enumerated field Fragment.Validate checks.
var (
	protocols        = set(ProtocolTCP, ProtocolUDP, ProtocolAll)
	layers           = set(LayerKubernetes, LayerAWSInstance, LayerAWSNetwork)
	gateTypes        = set(GateFilter, GateRoute, GateTransform)
	directions       = set(Ingress, Egress)
	actions          = set(ActionAllow, ActionDeny)
	routeTargetKinds = set(RouteLocal, RouteInternetGateway, RouteNATGateway, RouteTransitGateway,
		RoutePeering, RouteInterface, RouteInstance, RouteGatewayEndpoint, RouteOther)
	interfaceKinds = set(InterfaceNode, InterfaceDatabase, InterfaceLoadBalancer, InterfaceNATGateway,
		InterfaceVPCEndpoint, InterfaceInstance, InterfaceOther)
	resourceKinds    = set(ResourceDatabase, ResourceLoadBalancer, ResourceInstance, ResourceOther)
	toggles          = set(ToggleUnknown, ToggleOn, ToggleOff)
	profileSources   = set(ProfileFromAWS, ProfileFromAdmin)
	expectationKinds = set(ExpectRequired, ExpectForbidden)
	quantifiers      = set(QuantifyAll, QuantifyAny)
	ruleModes        = set(ModeWarn, ModeBlock)
)

func set[T comparable](values ...T) map[T]bool {
	m := make(map[T]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}

func checkEnum[T ~string](field string, v T, declared map[T]bool) error {
	if !declared[v] {
		return fmt.Errorf("%s: undeclared value %q", field, v)
	}
	return nil
}

// within prefixes a non-nil err with the place it is about.
func within(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}

// each checks every item and names the first that fails as "<list>[<index>]".
func each[T any](list string, items []T, check func(T) error) error {
	for i, item := range items {
		if err := check(item); err != nil {
			return within(err, "%s[%d]", list, i)
		}
	}
	return nil
}

func checkFragment(f Fragment) error {
	if err := each("workloads", f.Workloads, checkWorkload); err != nil {
		return err
	}
	err := each("resources", f.Resources, func(r Resource) error {
		return checkEnum("kind", r.Kind, resourceKinds)
	})
	if err != nil {
		return err
	}
	for i, g := range f.Gates {
		if err := checkGate(g); err != nil {
			return within(err, "gates[%d] (%s)", i, g.ID)
		}
	}
	err = each("network.interfaces", f.Network.Interfaces, func(n Interface) error {
		return checkEnum("kind", n.Kind, interfaceKinds)
	})
	if err != nil {
		return err
	}
	if f.Profile != nil {
		if err := checkProfile(*f.Profile); err != nil {
			return within(err, "profile")
		}
	}
	return each("expectations", f.Expectations, checkExpectation)
}

func checkWorkload(w Workload) error {
	return each("ports", w.Ports, func(p ContainerPort) error {
		return checkEnum("protocol", p.Protocol, protocols)
	})
}

func checkGate(g Gate) error {
	if err := errors.Join(checkEnum("layer", g.Layer, layers), checkEnum("type", g.Type, gateTypes)); err != nil {
		return err
	}
	if g.Type == GateFilter {
		if err := checkEnum("direction", g.Direction, directions); err != nil {
			return err
		}
	} else if g.Direction != "" {
		return fmt.Errorf("direction %q on a %s gate", g.Direction, g.Type)
	}
	t := g.Target
	if count(t.Pods != nil, t.SecurityGroup != "", len(t.Subnets) > 0, t.MainOf != "") != 1 {
		return errors.New("target: exactly one field must be set")
	}
	ordered := g.Type == GateFilter && g.Semantics.Ordered
	err := each("rules", g.Rules, func(r Rule) error { return checkRule(r, ordered) })
	if err != nil {
		return err
	}
	err = each("routes", g.Routes, func(r Route) error {
		return checkEnum("target.kind", r.Target.Kind, routeTargetKinds)
	})
	if err != nil {
		return err
	}
	return each("mappings", g.Mappings, func(m PortMapping) error {
		return checkEnum("protocol", m.Protocol, protocols)
	})
}

// count returns how many of flags are true.
func count(flags ...bool) int {
	n := 0
	for _, b := range flags {
		if b {
			n++
		}
	}
	return n
}

// checkRule checks one rule of a gate; ordered says the gate is an ordered
// filter.
func checkRule(r Rule, ordered bool) error {
	if err := checkEnum("action", r.Action, actions); err != nil {
		return err
	}
	switch {
	case !ordered && r.Action == ActionDeny:
		return errors.New("deny rule outside an ordered filter")
	case !ordered && r.Order != 0:
		return errors.New("order outside an ordered filter")
	case len(r.Peers) == 0:
		return errors.New("no peers")
	case len(r.Ports) == 0 && len(r.NamedPorts) == 0:
		return errors.New("no ports and no named ports")
	}
	if err := each("peers", r.Peers, checkPeer); err != nil {
		return err
	}
	if err := each("ports", r.Ports, checkPortRange); err != nil {
		return err
	}
	return each("namedPorts", r.NamedPorts, func(p NamedPort) error {
		return checkEnum("protocol", p.Protocol, protocols)
	})
}

func checkPeer(p Peer) error {
	if count(p.All, p.Prefix.IsValid(), p.SecurityGroup != "", p.PrefixList != "", p.Pods != nil) != 1 {
		return errors.New("exactly one form must be set")
	}
	if len(p.Except) > 0 && !p.Prefix.IsValid() {
		return errors.New("except without a prefix")
	}
	return nil
}

func checkPortRange(r PortRange) error {
	if r.From > r.To {
		return fmt.Errorf("from %d is above to %d", r.From, r.To)
	}
	return checkEnum("protocol", r.Protocol, protocols)
}

func checkProfile(p ClusterProfile) error {
	err := errors.Join(
		checkEnum("networkPolicyEnforcement", p.NetworkPolicyEnforcement, toggles),
		checkEnum("podSecurityGroups", p.PodSecurityGroups, toggles),
		checkEnum("externalSNAT", p.ExternalSNAT, toggles),
		checkEnum("customNetworking", p.CustomNetworking, toggles),
		checkEnum("ipBlockAppliesToPods", p.IPBlockAppliesToPods, toggles),
	)
	if err != nil {
		return err
	}
	if p.EphemeralPorts != nil {
		if err := checkPortRange(*p.EphemeralPorts); err != nil {
			return within(err, "ephemeralPorts")
		}
	}
	// In key order, so that the same profile reports the same error.
	for _, field := range slices.Sorted(maps.Keys(p.Sources)) {
		if err := checkEnum("sources."+field, p.Sources[field], profileSources); err != nil {
			return err
		}
	}
	return nil
}

func checkExpectation(e Expectation) error {
	if err := checkEnum("kind", e.Kind, expectationKinds); err != nil {
		return err
	}
	resourceKind := func(k ResourceKind) error { return checkEnum("kind", k, resourceKinds) }
	err := errors.Join(
		each("sources.resourceKinds", e.Sources.ResourceKinds, resourceKind),
		each("destinations.resourceKinds", e.Destinations.ResourceKinds, resourceKind),
		each("ports", e.Ports, checkPortRange),
		checkQuantifier("sourceQuantifier", e.SourceQuantifier, e.Kind),
		checkQuantifier("destinationQuantifier", e.DestinationQuantifier, e.Kind),
	)
	if err != nil || e.Mode == "" {
		return err
	}
	return checkEnum("mode", e.Mode, ruleModes)
}

// checkQuantifier allows an empty quantifier on a forbidden expectation only.
func checkQuantifier(field string, q Quantifier, kind ExpectationKind) error {
	if q == "" && kind == ExpectForbidden {
		return nil
	}
	return checkEnum(field, q, quantifiers)
}

// checkAnalysis is Analysis.Validate. Its cost is linear in the sizes of a's
// lists, plus one class count per run of paths between one pair of endpoints.
func checkAnalysis(a Analysis) error {
	err := errors.Join(
		ascending("inputs", a.Inputs, compareSnapshotRef),
		ascending("endpoints", a.Endpoints, compareEndpoint),
		ascending("paths", a.Paths, comparePath),
		ascending("changes", a.Changes, compareChange),
		ascending("findings", a.Findings, compareFinding),
		ascending("unexamined", a.Unexamined, cmp.Compare[Unexamined]),
	)
	if err != nil {
		return err
	}
	// Identical problems are the one tie allowed.
	if !slices.IsSortedFunc(a.Problems, compareProblem) {
		return errors.New("problems: out of order")
	}
	if err := each("endpoints", a.Endpoints, checkEndpointLists); err != nil {
		return err
	}
	if err := each("changes", a.Changes, checkChangeLists); err != nil {
		return err
	}
	if err := each("findings", a.Findings, checkFindingLists); err != nil {
		return err
	}
	return checkPaths(a.Endpoints, a.Paths)
}

// ascending errors unless every item sorts strictly after the one before it.
func ascending[T any](list string, items []T, compare func(a, b T) int) error {
	for i := 1; i < len(items); i++ {
		if compare(items[i-1], items[i]) >= 0 {
			return fmt.Errorf("%s[%d]: out of order or tied", list, i)
		}
	}
	return nil
}

func checkEndpointLists(e Endpoint) error {
	err := errors.Join(
		ascending("classes", e.Classes, compareClass),
		ascending("services", e.Services, func(a, b EndpointService) int { return cmp.Compare(a.Name, b.Name) }),
	)
	if err != nil {
		return err
	}
	if err := each("classes", e.Classes, checkClassLists); err != nil {
		return err
	}
	return each("services", e.Services, func(s EndpointService) error {
		return ascending("mappings", s.Mappings, compareMapping)
	})
}

func checkClassLists(c PlacementClass) error {
	return errors.Join(
		ascending("addresses", c.Addresses, func(a, b netip.Prefix) int { return a.Addr().Compare(b.Addr()) }),
		ascending("securityGroups", c.SecurityGroups, cmp.Compare[string]),
	)
}

func checkChangeLists(c Change) error {
	return errors.Join(
		checkPairs("opened", c.Opened),
		checkPairs("closed", c.Closed),
		checkPairs("possiblyOpened", c.PossiblyOpened),
		checkPairs("possiblyClosed", c.PossiblyClosed),
		checkPairs("undetermined", c.Undetermined),
	)
}

func checkPairs(list string, pairs []ClassPair) error {
	if err := ascending(list, pairs, comparePair); err != nil {
		return err
	}
	return each(list, pairs, func(p ClassPair) error {
		return errors.Join(
			within(checkClassLists(p.Source), "source"),
			within(checkClassLists(p.Destination), "destination"),
		)
	})
}

func checkFindingLists(f Finding) error {
	return errors.Join(
		ascending("paths", f.Paths, comparePathKey),
		ascending("reasons", f.Reasons, cmp.Compare[ReasonCode]),
	)
}

// pairEnds is what decides how many class pairs a path has.
type pairEnds struct {
	source, destination EndpointKey
	sameNode            bool
}

// checkPaths checks every path against the classes of its endpoints.
func checkPaths(endpoints []Endpoint, paths []Path) error {
	classes := make(map[EndpointKey][]PlacementClass, len(endpoints))
	for _, e := range endpoints {
		classes[e.Key] = e.Classes
	}
	// Paths of one pair of endpoints are adjacent, so the last count is
	// reused until the ends change.
	var counted pairEnds
	n := 0
	for i, p := range paths {
		ends := pairEnds{p.Key.Source, p.Key.Destination, p.SameNode}
		if i == 0 || ends != counted {
			source, hasSource := classes[ends.source]
			destination, hasDestination := classes[ends.destination]
			if !hasSource || !hasDestination {
				return fmt.Errorf("paths[%d] (%s): an end is not among the endpoints", i, p.Key)
			}
			counted, n = ends, countClassPairs(source, destination, ends.sameNode)
		}
		if err := checkPath(p, n); err != nil {
			return within(err, "paths[%d] (%s)", i, p.Key)
		}
	}
	return nil
}

// countClassPairs returns how many class pairs a path between the two ends
// has (see Path).
func countClassPairs(source, destination []PlacementClass, sameNode bool) int {
	n := len(source) * len(destination)
	if !sameNode {
		return n
	}
	type location struct{ nodeGroup, subnet string }
	at := make(map[location]int, len(destination))
	for _, d := range destination {
		at[location{d.NodeGroup, d.Subnet}]++
	}
	for _, s := range source {
		n += at[location{s.NodeGroup, s.Subnet}]
	}
	return n
}

// checkPath checks p, which has n class pairs.
func checkPath(p Path, n int) error {
	if n == 0 {
		return errors.New("an end has no placement class")
	}
	if err := ascending("reasons", p.Reasons, cmp.Compare[ReasonCode]); err != nil {
		return err
	}
	inputLevel := slices.ContainsFunc(p.Reasons, func(c ReasonCode) bool { return LevelOf(c) == LevelInput })
	if inputLevel && (p.Verdict != Unknown || p.Outcomes != "") {
		return errors.New("an input-level reason needs verdict unknown and no outcomes")
	}
	if p.Outcomes != "" {
		return checkOutcomes(p.Outcomes, p.Verdict, n)
	}
	if p.Verdict != Allow && p.Verdict != Deny && p.Verdict != Unknown {
		return fmt.Errorf("verdict %q without outcomes", p.Verdict)
	}
	return nil
}

// checkOutcomes checks a non-empty Path.Outcomes against its verdict.
func checkOutcomes(outcomes string, verdict Verdict, n int) error {
	var pass, block, undetermined int
	for _, letter := range outcomes {
		switch letter {
		case 'P':
			pass++
		case 'B':
			block++
		case 'U':
			undetermined++
		default:
			return fmt.Errorf("outcomes: unknown letter %q", letter)
		}
	}
	switch total := pass + block + undetermined; {
	case total != n:
		return fmt.Errorf("outcomes: %d letters for %d class pairs", total, n)
	case pass == total || block == total || undetermined == total:
		return errors.New("outcomes: all the same, so they must be empty")
	}
	want := Unknown
	if pass > 0 && block > 0 {
		want = PlacementDependent
	}
	if verdict != want {
		return fmt.Errorf("verdict %q, but the outcomes give %q", verdict, want)
	}
	return nil
}

func compareSnapshotRef(a, b SnapshotRef) int {
	return cmp.Or(cmp.Compare(a.Key.Source, b.Key.Source), cmp.Compare(a.Key.PullRequest, b.Key.PullRequest))
}

func compareEndpoint(a, b Endpoint) int { return cmp.Compare(a.Key, b.Key) }

// comparePathKey is PathKey order.
func comparePathKey(a, b PathKey) int {
	return cmp.Or(
		cmp.Compare(a.Source, b.Source),
		cmp.Compare(a.Destination, b.Destination),
		cmp.Compare(a.Ports.Protocol, b.Ports.Protocol),
		cmp.Compare(a.Ports.From, b.Ports.From),
	)
}

func comparePath(a, b Path) int { return comparePathKey(a.Key, b.Key) }

func compareChange(a, b Change) int { return comparePathKey(a.Key, b.Key) }

func compareFinding(a, b Finding) int {
	return cmp.Or(
		cmp.Compare(a.Rule, b.Rule),
		cmp.Compare(a.Subject, b.Subject),
		cmp.Compare(a.Kind, b.Kind),
		cmp.Compare(a.Origin.Locator, b.Origin.Locator),
	)
}

func compareMapping(a, b PortMapping) int {
	return cmp.Or(cmp.Compare(a.Protocol, b.Protocol), cmp.Compare(a.Port, b.Port))
}

// compareClass is class order.
func compareClass(a, b PlacementClass) int {
	return cmp.Or(
		cmp.Compare(a.NodeGroup, b.NodeGroup),
		cmp.Compare(a.Subnet, b.Subnet),
		slices.CompareFunc(a.Addresses, b.Addresses, func(x, y netip.Prefix) int {
			return cmp.Or(x.Addr().Compare(y.Addr()), cmp.Compare(x.Bits(), y.Bits()))
		}),
		cmp.Compare(a.Name, b.Name),
	)
}

// comparePair is pair order.
func comparePair(a, b ClassPair) int {
	return cmp.Or(
		compareClass(a.Source, b.Source),
		compareClass(a.Destination, b.Destination),
		cmp.Compare(count(a.SameNode), count(b.SameNode)),
	)
}
