package model_test

import (
	"net/netip"
	"reflect"
	"slices"
	"testing"

	"github.com/Soo-de/suiro/internal/model"
)

// Positions of the gates in validFragment.
const (
	policyGate = iota
	groupGate
	aclGate
	routeGate
	serviceGate
)

// validFragment is a fresh fragment that holds every enumerated field and
// breaks no invariant.
func validFragment() model.Fragment {
	prefix := netip.MustParsePrefix
	tcp := func(port uint16) []model.PortRange {
		return []model.PortRange{{Protocol: model.ProtocolTCP, From: port, To: port}}
	}
	return model.Fragment{
		Source: "payments/api",
		Workloads: []model.Workload{{
			Key: "k8s:apps/Deployment/payments/api", Kind: model.KindDeployment, Namespace: "payments", Name: "api",
			Ports: []model.ContainerPort{{Container: "api", Port: 80, Protocol: model.ProtocolTCP}},
		}},
		Resources: []model.Resource{{Key: "aws:database/orders-db", Kind: model.ResourceDatabase, Name: "orders-db", Interfaces: []string{"eni-0abc"}}},
		Gates: []model.Gate{
			policyGate: {
				ID: "k8s:networking.k8s.io/NetworkPolicy/payments/allow#ingress", Layer: model.LayerKubernetes,
				Type: model.GateFilter, Direction: model.Ingress,
				Semantics: model.FilterSemantics{Stateful: true, Isolating: true},
				Target:    model.GateTarget{Pods: &model.PodTarget{Namespace: "payments"}},
				Rules: []model.Rule{
					{Action: model.ActionAllow, Peers: []model.Peer{{Pods: &model.PodPeer{Namespace: "payments"}}}, Ports: tcp(80)},
					{
						Action:     model.ActionAllow,
						Peers:      []model.Peer{{All: true}, {Prefix: prefix("10.0.0.0/8"), Except: []netip.Prefix{prefix("10.1.0.0/16")}}},
						NamedPorts: []model.NamedPort{{Protocol: model.ProtocolTCP, Name: "http"}},
					},
				},
			},
			groupGate: {
				ID: "aws:ec2/SecurityGroup/sg-0abc#egress", Layer: model.LayerAWSInstance,
				Type: model.GateFilter, Direction: model.Egress,
				Semantics: model.FilterSemantics{Stateful: true},
				Target:    model.GateTarget{SecurityGroup: "sg-0abc"},
				Rules: []model.Rule{
					{Action: model.ActionAllow, Peers: []model.Peer{{SecurityGroup: "sg-0def"}}, Ports: tcp(5432)},
					{Action: model.ActionAllow, Peers: []model.Peer{{PrefixList: "pl-0abc"}}, Ports: []model.PortRange{model.AllPorts()}},
				},
			},
			aclGate: {
				ID: "aws:ec2/NetworkAcl/acl-0abc#ingress", Layer: model.LayerAWSNetwork,
				Type: model.GateFilter, Direction: model.Ingress,
				Semantics: model.FilterSemantics{Ordered: true},
				Target:    model.GateTarget{Subnets: []string{"subnet-a"}},
				Rules: []model.Rule{
					{Action: model.ActionDeny, Order: 100, Peers: []model.Peer{{Prefix: prefix("0.0.0.0/0")}}, Ports: tcp(22)},
					{Action: model.ActionAllow, Order: 200, Peers: []model.Peer{{All: true}}, Ports: []model.PortRange{model.AllPorts()}},
				},
			},
			routeGate: {
				ID: "aws:ec2/RouteTable/rtb-0abc", Layer: model.LayerAWSNetwork, Type: model.GateRoute,
				Target: model.GateTarget{MainOf: "vpc-0abc"},
				Routes: []model.Route{{Destination: prefix("10.0.0.0/16"), Target: model.RouteTarget{Kind: model.RouteLocal}, Active: true}},
			},
			serviceGate: {
				ID: "k8s:core/Service/payments/api", Layer: model.LayerKubernetes, Type: model.GateTransform,
				Target:   model.GateTarget{Pods: &model.PodTarget{Namespace: "payments"}},
				Mappings: []model.PortMapping{{Protocol: model.ProtocolTCP, Port: 80, TargetPort: 8080}},
			},
		},
		Network: model.Network{
			Interfaces: []model.Interface{{ID: "eni-0abc", Subnet: "subnet-a", Kind: model.InterfaceDatabase}},
		},
		Profile: &model.ClusterProfile{
			NetworkPolicyEnforcement: model.ToggleOn,
			EphemeralPorts:           &model.PortRange{Protocol: model.ProtocolTCP, From: 32768, To: 60999},
			Sources:                  map[string]model.ProfileSource{"networkPolicyEnforcement": model.ProfileFromAWS},
		},
		Expectations: []model.Expectation{
			{
				ID: "payments/api#db", Kind: model.ExpectRequired, Name: "db",
				Sources:      model.EndpointSelector{Self: true},
				Destinations: model.EndpointSelector{RDS: "orders-db"},
				Ports:        tcp(5432), SourceQuantifier: model.QuantifyAll, DestinationQuantifier: model.QuantifyAny,
				Mode: model.ModeWarn,
			},
			{
				ID: "builtin.internet-db", Kind: model.ExpectForbidden, Name: "internet-db",
				Sources:      model.EndpointSelector{Internet: true},
				Destinations: model.EndpointSelector{ResourceKinds: []model.ResourceKind{model.ResourceDatabase}},
				Ports:        tcp(5432),
			},
		},
		Problems: []model.Problem{{Reason: model.ReasonRuleOutsideModel, Subject: "x"}},
	}
}

func checkFragment(t *testing.T, f model.Fragment, want model.Fragment, valid bool) {
	t.Helper()
	first := f.Validate()
	if valid && first != nil {
		t.Errorf("Validate = %v, want nil", first)
	}
	if !valid && first == nil {
		t.Error("Validate = nil, want an error")
	}
	if second := f.Validate(); (second == nil) != (first == nil) || second != nil && second.Error() != first.Error() {
		t.Errorf("a second call gave %v, the first %v", second, first)
	}
	if !reflect.DeepEqual(f, want) {
		t.Errorf("Validate changed the fragment:\ngot  %+v\nwant %+v", f, want)
	}
}

type fragmentChange struct {
	name   string
	change func(f *model.Fragment)
}

func runFragments(t *testing.T, valid bool, tests []fragmentChange) {
	t.Helper()
	base := validFragment()
	if err := base.Validate(); err != nil {
		t.Fatalf("the base fragment is invalid: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, want := validFragment(), validFragment()
			tt.change(&f)
			tt.change(&want)
			checkFragment(t, f, want, valid)
		})
	}
}

func TestFragmentValidateZero(t *testing.T) {
	checkFragment(t, model.Fragment{}, model.Fragment{}, true)
}

func TestFragmentValidateEachGateAlone(t *testing.T) {
	for i, g := range validFragment().Gates {
		t.Run(string(g.ID), func(t *testing.T) {
			f := model.Fragment{Gates: []model.Gate{validFragment().Gates[i]}}
			want := model.Fragment{Gates: []model.Gate{validFragment().Gates[i]}}
			checkFragment(t, f, want, true)
		})
	}
}

func TestFragmentValidateValid(t *testing.T) {
	runFragments(t, true, []fragmentChange{
		{"base", func(*model.Fragment) {}},
		{"rule with named ports only", func(f *model.Fragment) {
			f.Gates[policyGate].Rules[0].Ports = nil
			f.Gates[policyGate].Rules[0].NamedPorts = []model.NamedPort{{Protocol: model.ProtocolUDP, Name: "dns"}}
		}},
		{"rule with ports and named ports", func(f *model.Fragment) {
			f.Gates[policyGate].Rules[0].NamedPorts = []model.NamedPort{{Protocol: model.ProtocolTCP, Name: "http"}}
		}},
		{"prefix without except", func(f *model.Fragment) { f.Gates[policyGate].Rules[1].Peers[1].Except = nil }},
		{"filter without rules", func(f *model.Fragment) { f.Gates[policyGate].Rules = nil }},
		{"single port range", func(f *model.Fragment) {
			f.Gates[groupGate].Rules[0].Ports[0] = model.PortRange{Protocol: model.ProtocolTCP, From: 65535, To: 65535}
		}},
		{"deny without order in an ordered filter", func(f *model.Fragment) { f.Gates[aclGate].Rules[0].Order = 0 }},
		{"no profile", func(f *model.Fragment) { f.Profile = nil }},
		{"profile without ephemeral ports", func(f *model.Fragment) { f.Profile.EphemeralPorts = nil }},
	})
}

func TestFragmentValidateGateTarget(t *testing.T) {
	forms := []struct {
		name string
		set  func(g *model.GateTarget)
	}{
		{"pods", func(g *model.GateTarget) { g.Pods = &model.PodTarget{Namespace: "payments"} }},
		{"security group", func(g *model.GateTarget) { g.SecurityGroup = "sg-0abc" }},
		{"subnets", func(g *model.GateTarget) { g.Subnets = []string{"subnet-a"} }},
		{"main of", func(g *model.GateTarget) { g.MainOf = "vpc-0abc" }},
	}
	var broken, valid []fragmentChange
	for i, gate := range []string{"policy", "security group", "acl", "route", "service"} {
		broken = append(broken, fragmentChange{"no target on the " + gate + " gate", func(f *model.Fragment) {
			f.Gates[i].Target = model.GateTarget{}
		}})
	}
	for i, a := range forms {
		valid = append(valid, fragmentChange{a.name, func(f *model.Fragment) {
			f.Gates[groupGate].Target = model.GateTarget{}
			a.set(&f.Gates[groupGate].Target)
		}})
		for _, b := range forms[i+1:] {
			broken = append(broken, fragmentChange{a.name + " and " + b.name, func(f *model.Fragment) {
				f.Gates[groupGate].Target = model.GateTarget{}
				a.set(&f.Gates[groupGate].Target)
				b.set(&f.Gates[groupGate].Target)
			}})
		}
	}
	runFragments(t, true, valid)
	runFragments(t, false, broken)
}

func TestFragmentValidatePeers(t *testing.T) {
	prefix := netip.MustParsePrefix
	forms := []struct {
		name string
		set  func(p *model.Peer)
	}{
		{"all", func(p *model.Peer) { p.All = true }},
		{"prefix", func(p *model.Peer) { p.Prefix = prefix("10.0.0.0/8") }},
		{"security group", func(p *model.Peer) { p.SecurityGroup = "sg-0def" }},
		{"prefix list", func(p *model.Peer) { p.PrefixList = "pl-0abc" }},
		{"pods", func(p *model.Peer) { p.Pods = &model.PodPeer{Namespace: "payments"} }},
	}
	// The peer under test is the second one of the second rule.
	peer := func(f *model.Fragment) *model.Peer { return &f.Gates[policyGate].Rules[1].Peers[1] }
	except := []netip.Prefix{prefix("10.1.0.0/16")}
	var valid []fragmentChange
	broken := []fragmentChange{
		{"no form", func(f *model.Fragment) { *peer(f) = model.Peer{} }},
		{"except alone", func(f *model.Fragment) { *peer(f) = model.Peer{Except: except} }},
	}
	for i, a := range forms {
		valid = append(valid, fragmentChange{a.name, func(f *model.Fragment) {
			*peer(f) = model.Peer{}
			a.set(peer(f))
		}})
		if a.name != "prefix" {
			broken = append(broken, fragmentChange{"except with " + a.name, func(f *model.Fragment) {
				*peer(f) = model.Peer{Except: slices.Clone(except)}
				a.set(peer(f))
			}})
		}
		for _, b := range forms[i+1:] {
			broken = append(broken, fragmentChange{a.name + " and " + b.name, func(f *model.Fragment) {
				*peer(f) = model.Peer{}
				a.set(peer(f))
				b.set(peer(f))
			}})
		}
	}
	runFragments(t, true, valid)
	runFragments(t, false, broken)
}

func TestFragmentValidateBroken(t *testing.T) {
	reversed := model.PortRange{Protocol: model.ProtocolTCP, From: 81, To: 80}
	runFragments(t, false, []fragmentChange{
		{"no peers", func(f *model.Fragment) { f.Gates[policyGate].Rules[1].Peers = nil }},
		{"empty peers", func(f *model.Fragment) { f.Gates[groupGate].Rules[1].Peers = []model.Peer{} }},
		{"no ports", func(f *model.Fragment) { f.Gates[groupGate].Rules[1].Ports = nil }},
		{"empty ports and named ports", func(f *model.Fragment) {
			f.Gates[policyGate].Rules[1].Ports = []model.PortRange{}
			f.Gates[policyGate].Rules[1].NamedPorts = []model.NamedPort{}
		}},
		{"deny in a security group", func(f *model.Fragment) { f.Gates[groupGate].Rules[1].Action = model.ActionDeny }},
		{"deny in a network policy", func(f *model.Fragment) { f.Gates[policyGate].Rules[0].Action = model.ActionDeny }},
		{"order in a security group", func(f *model.Fragment) { f.Gates[groupGate].Rules[1].Order = 100 }},
		{"order in a network policy", func(f *model.Fragment) { f.Gates[policyGate].Rules[0].Order = 1 }},
		{"ordered rules in an unordered filter", func(f *model.Fragment) { f.Gates[aclGate].Semantics.Ordered = false }},
		{"policy filter without a direction", func(f *model.Fragment) { f.Gates[policyGate].Direction = "" }},
		{"acl filter without a direction", func(f *model.Fragment) { f.Gates[aclGate].Direction = "" }},
		{"route with ingress", func(f *model.Fragment) { f.Gates[routeGate].Direction = model.Ingress }},
		{"route with egress", func(f *model.Fragment) { f.Gates[routeGate].Direction = model.Egress }},
		{"transform with ingress", func(f *model.Fragment) { f.Gates[serviceGate].Direction = model.Ingress }},
		{"transform with egress", func(f *model.Fragment) { f.Gates[serviceGate].Direction = model.Egress }},
		{"reversed rule range", func(f *model.Fragment) { f.Gates[policyGate].Rules[0].Ports[0] = reversed }},
		{"reversed second rule range", func(f *model.Fragment) {
			f.Gates[aclGate].Rules[1].Ports = append(f.Gates[aclGate].Rules[1].Ports, reversed)
		}},
		{"reversed expectation range", func(f *model.Fragment) { f.Expectations[0].Ports[0] = reversed }},
		{"reversed second expectation range", func(f *model.Fragment) {
			f.Expectations[1].Ports = append(f.Expectations[1].Ports, reversed)
		}},
		{"reversed ephemeral ports", func(f *model.Fragment) { *f.Profile.EphemeralPorts = reversed }},
	})
}

// enumCase sets one enumerated field to a value.
type enumCase struct {
	field string
	// typ names the field's type in the package source.
	typ string
	set func(f *model.Fragment, v string)
	// empty says "" is accepted.
	empty bool
}

func TestFragmentValidateEnumerations(t *testing.T) {
	// Each case changes one field of the full base fragment and leaves the
	// rest as it is.
	cases := []enumCase{
		{field: "gate layer", typ: "Layer", set: func(f *model.Fragment, v string) { f.Gates[groupGate].Layer = model.Layer(v) }},
		{field: "route gate layer", typ: "Layer", set: func(f *model.Fragment, v string) { f.Gates[routeGate].Layer = model.Layer(v) }},
		{field: "filter direction", typ: "Direction", set: func(f *model.Fragment, v string) { f.Gates[policyGate].Direction = model.Direction(v) }},
		{field: "rule action", typ: "Action", set: func(f *model.Fragment, v string) { f.Gates[aclGate].Rules[1].Action = model.Action(v) }},
		{field: "rule port protocol", typ: "Protocol", set: func(f *model.Fragment, v string) {
			f.Gates[groupGate].Rules[0].Ports[0].Protocol = model.Protocol(v)
		}},
		{field: "named port protocol", typ: "Protocol", set: func(f *model.Fragment, v string) {
			f.Gates[policyGate].Rules[1].NamedPorts[0].Protocol = model.Protocol(v)
		}},
		{field: "mapping protocol", typ: "Protocol", set: func(f *model.Fragment, v string) {
			f.Gates[serviceGate].Mappings[0].Protocol = model.Protocol(v)
		}},
		{field: "container port protocol", typ: "Protocol", set: func(f *model.Fragment, v string) {
			f.Workloads[0].Ports[0].Protocol = model.Protocol(v)
		}},
		{field: "expectation port protocol", typ: "Protocol", set: func(f *model.Fragment, v string) {
			f.Expectations[0].Ports[0].Protocol = model.Protocol(v)
		}},
		{field: "ephemeral port protocol", typ: "Protocol", set: func(f *model.Fragment, v string) {
			f.Profile.EphemeralPorts.Protocol = model.Protocol(v)
		}},
		{field: "route target kind", typ: "RouteTargetKind", set: func(f *model.Fragment, v string) {
			f.Gates[routeGate].Routes[0].Target.Kind = model.RouteTargetKind(v)
		}},
		{field: "interface kind", typ: "InterfaceKind", set: func(f *model.Fragment, v string) {
			f.Network.Interfaces[0].Kind = model.InterfaceKind(v)
		}},
		{field: "resource kind", typ: "ResourceKind", set: func(f *model.Fragment, v string) { f.Resources[0].Kind = model.ResourceKind(v) }},
		{field: "selector resource kind", typ: "ResourceKind", set: func(f *model.Fragment, v string) {
			f.Expectations[1].Destinations.ResourceKinds[0] = model.ResourceKind(v)
		}},
		{field: "source selector resource kind", typ: "ResourceKind", set: func(f *model.Fragment, v string) {
			f.Expectations[1].Sources = model.EndpointSelector{ResourceKinds: []model.ResourceKind{model.ResourceKind(v)}}
		}},
		{field: "network policy enforcement", typ: "Toggle", empty: true, set: func(f *model.Fragment, v string) {
			f.Profile.NetworkPolicyEnforcement = model.Toggle(v)
		}},
		{field: "pod security groups", typ: "Toggle", empty: true, set: func(f *model.Fragment, v string) {
			f.Profile.PodSecurityGroups = model.Toggle(v)
		}},
		{field: "external SNAT", typ: "Toggle", empty: true, set: func(f *model.Fragment, v string) { f.Profile.ExternalSNAT = model.Toggle(v) }},
		{field: "custom networking", typ: "Toggle", empty: true, set: func(f *model.Fragment, v string) {
			f.Profile.CustomNetworking = model.Toggle(v)
		}},
		{field: "ipBlock applies to pods", typ: "Toggle", empty: true, set: func(f *model.Fragment, v string) {
			f.Profile.IPBlockAppliesToPods = model.Toggle(v)
		}},
		{field: "profile value source", typ: "ProfileSource", set: func(f *model.Fragment, v string) {
			f.Profile.Sources["networkPolicyEnforcement"] = model.ProfileSource(v)
		}},
		{field: "expectation kind", typ: "ExpectationKind", set: func(f *model.Fragment, v string) {
			f.Expectations[0].Kind = model.ExpectationKind(v)
		}},
		{field: "required source quantifier", typ: "Quantifier", set: func(f *model.Fragment, v string) {
			f.Expectations[0].SourceQuantifier = model.Quantifier(v)
		}},
		{field: "required destination quantifier", typ: "Quantifier", set: func(f *model.Fragment, v string) {
			f.Expectations[0].DestinationQuantifier = model.Quantifier(v)
		}},
		{field: "required rule mode", typ: "RuleMode", empty: true, set: func(f *model.Fragment, v string) {
			f.Expectations[0].Mode = model.RuleMode(v)
		}},
		{field: "forbidden rule mode", typ: "RuleMode", empty: true, set: func(f *model.Fragment, v string) {
			f.Expectations[1].Mode = model.RuleMode(v)
		}},
		{field: "problem reason", typ: "ReasonCode", set: func(f *model.Fragment, v string) { f.Problems[0].Reason = model.ReasonCode(v) }},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			var valid []fragmentChange
			broken := []fragmentChange{{"bogus", func(f *model.Fragment) { c.set(f, "bogus") }}}
			for v := range declared(t, c.typ) {
				// "" is judged below, per field.
				if v != "" {
					valid = append(valid, fragmentChange{v, func(f *model.Fragment) { c.set(f, v) }})
				}
			}
			empty := fragmentChange{"empty", func(f *model.Fragment) { c.set(f, "") }}
			if c.empty {
				valid = append(valid, empty)
			} else {
				broken = append(broken, empty)
			}
			runFragments(t, true, valid)
			runFragments(t, false, broken)
		})
	}
}

// A forbidden expectation takes no quantifiers.
func TestFragmentValidateForbiddenQuantifiers(t *testing.T) {
	runFragments(t, false, []fragmentChange{
		{"bogus source", func(f *model.Fragment) { f.Expectations[1].SourceQuantifier = "bogus" }},
		{"bogus destination", func(f *model.Fragment) { f.Expectations[1].DestinationQuantifier = "bogus" }},
	})
	runFragments(t, true, []fragmentChange{
		{"none", func(f *model.Fragment) {
			f.Expectations[1].SourceQuantifier, f.Expectations[1].DestinationQuantifier = "", ""
		}},
	})
}

// The gate type decides which direction is valid, so each declared type is
// checked on a gate of that shape.
func TestFragmentValidateGateType(t *testing.T) {
	shapes := map[model.GateType]int{
		model.GateFilter:    policyGate,
		model.GateRoute:     routeGate,
		model.GateTransform: serviceGate,
	}
	var valid []fragmentChange
	for v := range declared(t, "GateType") {
		i, ok := shapes[model.GateType(v)]
		if !ok {
			t.Fatalf("no gate of type %q in the test fragment", v)
		}
		valid = append(valid, fragmentChange{v, func(f *model.Fragment) { f.Gates = []model.Gate{f.Gates[i]} }})
	}
	runFragments(t, true, valid)
	runFragments(t, false, []fragmentChange{
		{"bogus with a direction", func(f *model.Fragment) { f.Gates[policyGate].Type = "bogus" }},
		{"bogus without a direction", func(f *model.Fragment) { f.Gates[routeGate].Type = "bogus" }},
		{"empty with a direction", func(f *model.Fragment) { f.Gates[policyGate].Type = "" }},
		{"empty without a direction", func(f *model.Fragment) { f.Gates[routeGate].Type = "" }},
	})
}
