package k8s

import (
	"fmt"
	"slices"

	"github.com/Soo-de/suiro/internal/model"
)

// networkPolicyObject is a whole NetworkPolicy body, decoded strictly: a
// field it does not name makes the object invalid.
type networkPolicyObject struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		PodSelector model.LabelSelector `json:"podSelector"`
		PolicyTypes []string            `json:"policyTypes"`
		Ingress     []ingressRule       `json:"ingress"`
		Egress      []egressRule        `json:"egress"`
	} `json:"spec"`
}

type ingressRule struct {
	Ports []policyPort `json:"ports"`
	From  []policyPeer `json:"from"`
}

type egressRule struct {
	Ports []policyPort `json:"ports"`
	To    []policyPeer `json:"to"`
}

// policyRule is an ingress or egress rule, with its peers under one name.
type policyRule struct {
	ports []policyPort
	peers []policyPeer
}

type policyPort struct {
	// Protocol is a pointer: a written "" is invalid, not the default.
	Protocol *string      `json:"protocol"`
	Port     *intOrString `json:"port"`
	EndPort  *int64       `json:"endPort"`
}

type policyPeer struct {
	PodSelector       *model.LabelSelector `json:"podSelector"`
	NamespaceSelector *model.LabelSelector `json:"namespaceSelector"`
	IPBlock           *ipBlock             `json:"ipBlock"`
}

type ipBlock struct {
	CIDR   string   `json:"cidr"`
	Except []string `json:"except"`
}

// ruleList is the rules of one direction and the names its fields have.
type ruleList struct {
	direction model.Direction
	rules     []policyRule
	// at locates the list, peersField names a rule's peer list.
	at, peersField string
}

// networkPolicy adds one filter gate per direction o isolates its pods in.
func (t *translation) networkPolicy(o model.Object) {
	var p networkPolicyObject
	if err := decodeStrict(o.Body, &p); err != nil {
		t.invalid(o, "")
		return
	}
	lists := ruleLists(p)
	if bad := checkNetworkPolicy(p, lists); bad != "" {
		t.invalid(o, bad)
		return
	}
	directions := policyDirections(p)
	for _, list := range lists {
		if !slices.Contains(directions, list.direction) {
			continue
		}
		t.fragment.Gates = append(t.fragment.Gates, model.Gate{
			ID:        model.NewGateID(o.Key, list.direction),
			Layer:     model.LayerKubernetes,
			Type:      model.GateFilter,
			Direction: list.direction,
			Semantics: model.FilterSemantics{Stateful: true, Isolating: true},
			Target: model.GateTarget{Pods: &model.PodTarget{
				Namespace: p.Metadata.Namespace,
				Selector:  p.Spec.PodSelector,
			}},
			Rules:  t.rules(o, p.Metadata.Namespace, list),
			Object: o.Key,
			Origin: o.Origin,
		})
	}
}

// ruleLists returns the ingress rules, then the egress rules.
func ruleLists(p networkPolicyObject) []ruleList {
	ingress := make([]policyRule, len(p.Spec.Ingress))
	for i, r := range p.Spec.Ingress {
		ingress[i] = policyRule{ports: r.Ports, peers: r.From}
	}
	egress := make([]policyRule, len(p.Spec.Egress))
	for i, r := range p.Spec.Egress {
		egress[i] = policyRule{ports: r.Ports, peers: r.To}
	}
	return []ruleList{
		{model.Ingress, ingress, "spec.ingress", "from"},
		{model.Egress, egress, "spec.egress", "to"},
	}
}

// policyDirections returns the directions of a valid policy's policyTypes.
func policyDirections(p networkPolicyObject) []model.Direction {
	if len(p.Spec.PolicyTypes) == 0 {
		return defaultPolicyTypes(len(p.Spec.Egress))
	}
	directions := make([]model.Direction, len(p.Spec.PolicyTypes))
	for i, name := range p.Spec.PolicyTypes {
		directions[i], _ = policyType(name)
	}
	return directions
}

func policyType(name string) (model.Direction, bool) {
	switch name {
	case "Ingress":
		return model.Ingress, true
	case "Egress":
		return model.Egress, true
	}
	return "", false
}

// checkNetworkPolicy checks every field, the rules of a direction
// policyTypes leaves out included.
func checkNetworkPolicy(p networkPolicyObject, lists []ruleList) string {
	if p.Metadata.Namespace == "" {
		return "metadata.namespace"
	}
	if bad := checkSelector(&p.Spec.PodSelector, "spec.podSelector"); bad != "" {
		return bad
	}
	for _, list := range lists {
		if bad := checkRules(list); bad != "" {
			return bad
		}
	}
	if len(p.Spec.PolicyTypes) > 2 {
		return "spec.policyTypes"
	}
	for i, name := range p.Spec.PolicyTypes {
		if _, ok := policyType(name); !ok {
			return fmt.Sprintf("spec.policyTypes[%d]", i)
		}
	}
	return ""
}

func checkRules(list ruleList) string {
	for i, r := range list.rules {
		at := fmt.Sprintf("%s[%d]", list.at, i)
		if bad := checkPolicyPorts(r.ports, at+".ports"); bad != "" {
			return bad
		}
		if bad := checkPeers(r.peers, at+"."+list.peersField); bad != "" {
			return bad
		}
	}
	return ""
}

func checkPolicyPorts(ports []policyPort, listAt string) string {
	for i, p := range ports {
		if bad := checkPolicyPort(p, fmt.Sprintf("%s[%d]", listAt, i)); bad != "" {
			return bad
		}
	}
	return ""
}

func checkPolicyPort(p policyPort, at string) string {
	switch {
	case p.Protocol != nil && !validProtocol(*p.Protocol):
		return at + ".protocol"
	case p.Port == nil:
		if p.EndPort != nil {
			return at + ".endPort"
		}
	case p.Port.isString:
		if p.EndPort != nil {
			return at + ".endPort"
		}
		if !validPortName(p.Port.text) {
			return at + ".port"
		}
	case !validPort(p.Port.number):
		return at + ".port"
	case p.EndPort != nil && (*p.EndPort < p.Port.number || !validPort(*p.EndPort)):
		return at + ".endPort"
	}
	return ""
}

func checkPeers(peers []policyPeer, listAt string) string {
	for i, p := range peers {
		if bad := checkPeer(p, fmt.Sprintf("%s[%d]", listAt, i)); bad != "" {
			return bad
		}
	}
	return ""
}

// checkPeer checks the fields of a peer, then which of them it sets.
func checkPeer(p policyPeer, at string) string {
	if bad := checkSelector(p.PodSelector, at+".podSelector"); bad != "" {
		return bad
	}
	if bad := checkSelector(p.NamespaceSelector, at+".namespaceSelector"); bad != "" {
		return bad
	}
	selects := p.PodSelector != nil || p.NamespaceSelector != nil
	if p.IPBlock == nil {
		if !selects {
			return at
		}
		return ""
	}
	if bad := checkIPBlock(*p.IPBlock, at+".ipBlock"); bad != "" {
		return bad
	}
	if selects {
		return at
	}
	return ""
}

// checkIPBlock accepts either address family, with every except block inside
// the cidr.
func checkIPBlock(b ipBlock, at string) string {
	cidr, ok := parseCIDR(b.CIDR)
	if !ok {
		return at + ".cidr"
	}
	for i, s := range b.Except {
		if except, ok := parseCIDR(s); !ok || !strictSubset(except, cidr) {
			return fmt.Sprintf("%s.except[%d]", at, i)
		}
	}
	return ""
}

// rules returns the modeled rules of one valid direction. A rule whose peers
// or ports were all outside the model is dropped and flagged: kept with an
// empty list it could only be read as allowing more than was written.
func (t *translation) rules(o model.Object, namespace string, list ruleList) []model.Rule {
	var out []model.Rule
	for i, r := range list.rules {
		at := fmt.Sprintf("%s[%d]", list.at, i)
		peers := modelPeers(namespace, r.peers)
		ports, named := modelPorts(r.ports)
		if len(peers) == 0 || len(ports)+len(named) == 0 {
			t.flag(o, model.ReasonRuleOutsideModel, at, "every peer or every port of the rule is outside the model")
			continue
		}
		origin := o.Origin
		origin.Locator = at
		out = append(out, model.Rule{
			Action: model.ActionAllow, Peers: peers, Ports: ports, NamedPorts: named, Origin: origin,
		})
	}
	return out
}

// modelPeers returns every far end for an unwritten or empty list, else the
// modeled peers, which may be none.
func modelPeers(namespace string, peers []policyPeer) []model.Peer {
	if len(peers) == 0 {
		return []model.Peer{{All: true}}
	}
	var out []model.Peer
	for _, p := range peers {
		if p.IPBlock == nil {
			out = append(out, model.Peer{Pods: &model.PodPeer{
				Namespace:         namespace,
				NamespaceSelector: p.NamespaceSelector,
				PodSelector:       p.PodSelector,
			}})
		} else if peer, modeled := prefixPeer(*p.IPBlock); modeled {
			out = append(out, peer)
		}
	}
	return out
}

// prefixPeer returns the peer of a valid ipBlock, and false for an IPv6 one,
// IPv4-mapped addresses included.
func prefixPeer(b ipBlock) (model.Peer, bool) {
	cidr, _ := parseCIDR(b.CIDR)
	if !cidr.Addr().Is4() {
		return model.Peer{}, false
	}
	peer := model.Peer{Prefix: cidr}
	for _, s := range b.Except {
		except, _ := parseCIDR(s)
		peer.Except = append(peer.Except, except)
	}
	return peer, true
}

// modelPorts returns every port for an unwritten or empty list, else the
// modeled numbered and named ports, which may be none.
func modelPorts(ports []policyPort) ([]model.PortRange, []model.NamedPort) {
	if len(ports) == 0 {
		return []model.PortRange{model.AllPorts()}, nil
	}
	var ranges []model.PortRange
	var named []model.NamedPort
	for _, p := range ports {
		written := defaultProtocol
		if p.Protocol != nil {
			written = *p.Protocol
		}
		protocol, modeled := modelProtocol(written)
		switch {
		case !modeled:
		case p.Port == nil:
			ranges = append(ranges, model.PortRange{Protocol: protocol, From: 0, To: maxPort})
		case p.Port.isString:
			named = append(named, model.NamedPort{Protocol: protocol, Name: p.Port.text})
		default:
			to := p.Port.number
			if p.EndPort != nil {
				to = *p.EndPort
			}
			ranges = append(ranges,
				model.PortRange{Protocol: protocol, From: portNumber(p.Port.number), To: portNumber(to)})
		}
	}
	return ranges, named
}
