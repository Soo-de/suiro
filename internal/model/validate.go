package model

import (
	"errors"
	"fmt"
	"maps"
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
