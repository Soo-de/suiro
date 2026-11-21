package model

import "net/netip"

// GateID identifies one gate. Adapters write "<ObjectKey>#<direction>" for
// filters ("aws:ec2/SecurityGroup/sg-0abc#ingress",
// "k8s:networking.k8s.io/NetworkPolicy/payments/allow-db#egress") and the
// ObjectKey for route tables and Services. Steps core adds itself use
// "snat:<node group>", the NAT gateway's ObjectKey, and
// "endpoint:<EndpointKey>" for an endpoint-wide Undetermined step.
type GateID string

// Layer is the platform level a gate's rules come from.
type Layer string

const (
	// LayerKubernetes holds NetworkPolicies and Services.
	LayerKubernetes Layer = "kubernetes"
	// LayerAWSInstance holds Security Groups, which guard interfaces.
	LayerAWSInstance Layer = "aws-instance"
	// LayerAWSNetwork holds Network ACLs, route tables and NAT, which guard
	// subnets.
	LayerAWSNetwork Layer = "aws-network"
)

// GateType is what a gate does to a set of connections.
type GateType string

const (
	// GateFilter passes part of the set and blocks the rest.
	GateFilter GateType = "filter"
	// GateRoute decides whether a path out of a subnet exists at all, by
	// longest prefix match.
	GateRoute GateType = "route"
	// GateTransform rewrites addresses or ports (Service port to target
	// port, source address to the node's on leaving the VPC).
	GateTransform GateType = "transform"
)

// Direction is the side of a filter: ingress guards traffic arriving at its
// target, egress traffic leaving it.
type Direction string

const (
	Ingress Direction = "ingress"
	Egress  Direction = "egress"
)

// FilterSemantics describes how a filter decides by its properties, not by
// vendor name, so a new firewall with the same properties needs a new
// adapter and no new core code.
//
//	Security Group:  Stateful,  not Ordered, not Isolating
//	Network ACL:     stateless, Ordered,     not Isolating
//	NetworkPolicy:   Stateful,  not Ordered, Isolating
type FilterSemantics struct {
	// Stateful filters let the replies of an allowed connection back
	// without a rule. Stateless ones are checked again for the replies,
	// whose destination port is the client's ephemeral port.
	Stateful bool `json:"stateful"`
	// Ordered filters try rules by ascending Order and the first match
	// decides, allow or deny; a connection no rule matches is denied.
	// Unordered filters allow the union of their allow rules; they have no
	// deny rules.
	Ordered bool `json:"ordered"`
	// Isolating filters guard a target only once some gate of the same
	// Layer and Direction selects it; a target no such gate selects passes
	// everything in that direction.
	Isolating bool `json:"isolating"`
}

// Gate is one source of rules, as an adapter emits it: a NetworkPolicy in
// one direction, a Security Group in one direction, a Network ACL in one
// direction, a route table, a Service. Core joins the gates that guard the
// same target in the same Layer and Direction into one chain step (all
// Security Groups on an interface, all NetworkPolicies selecting a pod);
// GateRef names such a step.
type Gate struct {
	ID        GateID    `json:"id"`
	Layer     Layer     `json:"layer"`
	Type      GateType  `json:"type"`
	Direction Direction `json:"direction,omitempty"`
	// Semantics is set for filters.
	Semantics FilterSemantics `json:"semantics"`
	Target    GateTarget      `json:"target"`
	// Rules are set for filters, Routes for route gates, Mappings for
	// Service transforms.
	Rules    []Rule        `json:"rules,omitempty"`
	Routes   []Route       `json:"routes,omitempty"`
	Mappings []PortMapping `json:"mappings,omitempty"`
	Object   ObjectKey     `json:"object"`
	Origin   Origin        `json:"origin"`
}

// GateTarget says what a gate guards. Exactly one field is set.
type GateTarget struct {
	// Pods: a NetworkPolicy's podSelector or a Service's selector.
	Pods *PodTarget `json:"pods,omitempty"`
	// SecurityGroup: every interface that carries this group.
	SecurityGroup string `json:"securityGroup,omitempty"`
	// Subnets: the subnets associated with a Network ACL or route table.
	Subnets []string `json:"subnets,omitempty"`
	// MainOf: the VPC whose subnets without an explicit association use
	// this (main) route table.
	MainOf string `json:"mainOf,omitempty"`
}

// PodTarget is the pods in Namespace that Selector matches; an empty
// Selector matches every pod in the namespace.
type PodTarget struct {
	Namespace string        `json:"namespace"`
	Selector  LabelSelector `json:"selector"`
}

// Action is what a matching filter rule does.
type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
)

// Rule is one filter rule. It matches a connection when some peer matches
// the far end and some port matches the destination port. An empty list
// never stands for "every": every far end is Peer{All: true} and every port
// is AllPorts(), which the adapter writes when the source leaves the field
// unwritten (a NetworkPolicy rule without "from" or without "ports"). A
// rule with no Peers, or with neither Ports nor NamedPorts, is invalid
// (Fragment.Validate); one whose every peer or port the adapter dropped as
// outside the model is dropped whole (ReasonRuleOutsideModel).
type Rule struct {
	Action Action `json:"action"`
	// Order is the evaluation order in an Ordered gate (the Network ACL rule
	// number); 0 in unordered gates.
	Order      int         `json:"order,omitempty"`
	Peers      []Peer      `json:"peers,omitempty"`
	Ports      []PortRange `json:"ports,omitempty"`
	NamedPorts []NamedPort `json:"namedPorts,omitempty"`
	// Origin.Locator names the rule inside its object.
	Origin Origin `json:"origin"`
}

// Peer is the far end a rule matches. Exactly one form is set: All, Prefix
// (with optional Except, a NetworkPolicy ipBlock), SecurityGroup,
// PrefixList or Pods. Prefix and Except are compared with the far end's
// addresses, which for a pod are the addresses of its placement class.
// Whether a NetworkPolicy ipBlock matches a pod at all is
// ClusterProfile.IPBlockAppliesToPods. IPv6 prefixes are outside the model:
// adapters drop them.
type Peer struct {
	// All matches every far end: pods, interfaces and addresses alike. It is
	// what an unwritten peer list means, which no prefix can say.
	All    bool           `json:"all,omitempty"`
	Prefix netip.Prefix   `json:"prefix,omitzero"`
	Except []netip.Prefix `json:"except,omitempty"`
	// SecurityGroup matches interfaces carrying the group, by identity, not
	// by address. A group not in any snapshot raises
	// ReasonSecurityGroupUnresolved.
	SecurityGroup string `json:"securityGroup,omitempty"`
	// PrefixList is a managed prefix list ID. Lists are not read, so a rule
	// using one raises ReasonPrefixListUnresolved.
	PrefixList string   `json:"prefixList,omitempty"`
	Pods       *PodPeer `json:"pods,omitempty"`
}

// PodPeer is a NetworkPolicy peer: pods selected by namespace and pod
// selectors, AND-ed when both appear in the same peer.
type PodPeer struct {
	// Namespace is the policy's own namespace, the only one matched when
	// NamespaceSelector is nil.
	Namespace string `json:"namespace"`
	// NamespaceSelector nil means Namespace only; empty means every
	// namespace.
	NamespaceSelector *LabelSelector `json:"namespaceSelector,omitempty"`
	// PodSelector nil means every pod in the selected namespaces.
	PodSelector *LabelSelector `json:"podSelector,omitempty"`
}

// RouteTargetKind is where a route sends traffic.
type RouteTargetKind string

const (
	RouteLocal           RouteTargetKind = "local"
	RouteInternetGateway RouteTargetKind = "internet-gateway"
	RouteNATGateway      RouteTargetKind = "nat-gateway"
	// The kinds below lead outside the model: a path that takes them is
	// Undetermined (ReasonRouteOutOfScope, ReasonRouteMiddlebox).
	RouteTransitGateway  RouteTargetKind = "transit-gateway"
	RoutePeering         RouteTargetKind = "peering"
	RouteInterface       RouteTargetKind = "interface"
	RouteInstance        RouteTargetKind = "instance"
	RouteGatewayEndpoint RouteTargetKind = "gateway-endpoint"
	RouteOther           RouteTargetKind = "other"
)

// RouteTarget is a route's next hop.
type RouteTarget struct {
	Kind RouteTargetKind `json:"kind"`
	ID   string          `json:"id,omitempty"`
}

// Route is one route of a route table.
type Route struct {
	Destination netip.Prefix `json:"destination,omitzero"`
	// DestinationPrefixList is set instead of Destination for prefix-list
	// routes.
	DestinationPrefixList string      `json:"destinationPrefixList,omitempty"`
	Target                RouteTarget `json:"target"`
	// Active is false for a blackhole route, which drops traffic.
	Active bool `json:"active"`
	// Propagated routes lose to static routes with the same prefix.
	Propagated bool   `json:"propagated,omitempty"`
	Origin     Origin `json:"origin"`
}

// PortMapping is one Service port: connections to Port reach the selected
// pods on TargetPort, or on the container port named TargetPortName.
type PortMapping struct {
	Name           string   `json:"name,omitempty"`
	Protocol       Protocol `json:"protocol"`
	Port           uint16   `json:"port"`
	TargetPort     uint16   `json:"targetPort,omitempty"`
	TargetPortName string   `json:"targetPortName,omitempty"`
}

// Fragment is what one adapter makes of one snapshot. Selectors stay
// unresolved: a NetworkPolicy may select pods another source declares, so
// core resolves them only after merging every fragment of the analysis.
type Fragment struct {
	Source     SourceID    `json:"source"`
	Workloads  []Workload  `json:"workloads,omitempty"`
	Namespaces []Namespace `json:"namespaces,omitempty"`
	Resources  []Resource  `json:"resources,omitempty"`
	Gates      []Gate      `json:"gates,omitempty"`
	Network    Network     `json:"network"`
	// Profile holds the cluster profile values the snapshot reveals (an aws
	// snapshot: node groups, VPC CNI settings); nil when it reveals none.
	Profile *ClusterProfile `json:"profile,omitempty"`
	// Expectations are the source's required paths, from the
	// KindExpectation objects of its snapshot (ExpectationsFile); only the
	// adapter of git snapshots sets them.
	Expectations []Expectation `json:"expectations,omitempty"`
	// Problems carry every object the adapter could not translate or chose
	// to flag; each object of the snapshot ends in the model, in
	// IgnoredKinds, or here.
	Problems []Problem `json:"problems,omitempty"`
}

// Validate reports the first broken invariant of f, or nil. The invariants
// are the ones the field comments state: exactly one GateTarget field and
// one Peer form set; every rule with at least one peer and one port or named
// port; deny rules and Order only in Ordered filters; Direction set on
// filters and empty on routes and transforms; From ≤ To in every PortRange;
// and every enumerated field holding a declared value. A fragment that
// fails it is an adapter bug, not content.
func (f Fragment) Validate() error {
	panic("model: Fragment.Validate is not implemented")
}
