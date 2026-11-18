package model

import "net/netip"

// Protocol is a transport protocol of the connection space. The connection
// space is TCP and UDP over IPv4. Rules for other protocols (ICMP, SCTP,
// other IP protocol numbers) match no connection in it; adapters leave them
// out, which changes no verdict.
type Protocol string

const (
	ProtocolTCP Protocol = "tcp"
	ProtocolUDP Protocol = "udp"
	// ProtocolAll appears only in rules ("-1" in AWS, an omitted protocol in
	// some rule forms) and means both TCP and UDP.
	ProtocolAll Protocol = "all"
)

// PortRange is an inclusive range of destination ports of one protocol.
// Every port of a protocol is From 0, To 65535.
type PortRange struct {
	Protocol Protocol `json:"protocol"`
	From     uint16   `json:"from"`
	To       uint16   `json:"to"`
}

// NamedPort is a NetworkPolicy or Service port given by name. It is resolved
// against the destination workload's ContainerPort names; a name no
// container declares matches nothing (ReasonNamedPortUnresolved warns).
type NamedPort struct {
	Protocol Protocol `json:"protocol"`
	Name     string   `json:"name"`
}

// LabelSelector is a Kubernetes label selector: every MatchLabels pair and
// every MatchExpressions requirement must hold. An empty selector matches
// everything; where a nil selector means something else, the field that
// holds it says so.
type LabelSelector struct {
	MatchLabels      map[string]string  `json:"matchLabels,omitempty"`
	MatchExpressions []LabelRequirement `json:"matchExpressions,omitempty"`
}

// LabelRequirement is one selector expression. Operator is In, NotIn,
// Exists or DoesNotExist; node selector terms also allow Gt and Lt.
type LabelRequirement struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

// Workload is the set of pods one controller (Deployment, StatefulSet,
// DaemonSet, ReplicaSet, Job, CronJob) or a standalone Pod creates, as git
// declares it. It is an endpoint before core resolves its placement.
type Workload struct {
	// Key is the controller's ObjectKey; it is also the endpoint's key.
	Key       EndpointKey       `json:"key"`
	Kind      ObjectKind        `json:"kind"`
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	PodLabels map[string]string `json:"podLabels,omitempty"`
	Ports     []ContainerPort   `json:"ports,omitempty"`
	Placement Placement         `json:"placement"`
	// HostNetwork pods share the node's network namespace; NetworkPolicy
	// does not hold for them as written (ReasonHostNetwork).
	HostNetwork bool   `json:"hostNetwork,omitempty"`
	Origin      Origin `json:"origin"`
}

// ContainerPort is one port a container declares.
type ContainerPort struct {
	Container string   `json:"container"`
	Name      string   `json:"name,omitempty"`
	Port      uint16   `json:"port"`
	Protocol  Protocol `json:"protocol"`
	// HostPort also exposes the port on the node's address, a path the
	// model does not follow (ReasonHostPort).
	HostPort uint16 `json:"hostPort,omitempty"`
}

// Placement is what limits where a workload's pods can run; core derives
// the workload's placement classes from it and the node groups.
type Placement struct {
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// NodeTerms are the required node affinity terms; a node must match at
	// least one. Preferred terms do not limit placement and are not kept.
	NodeTerms   []NodeSelectorTerm `json:"nodeTerms,omitempty"`
	Tolerations []Toleration       `json:"tolerations,omitempty"`
	// NodeName pins the pods to one node, which git cannot map to a node
	// group (ReasonPlacementUnknown).
	NodeName string `json:"nodeName,omitempty"`
	// PodAffinity and PodAntiAffinity are the required terms only; they
	// decide whether two workloads can share a node.
	PodAffinity     []PodAffinityTerm `json:"podAffinity,omitempty"`
	PodAntiAffinity []PodAffinityTerm `json:"podAntiAffinity,omitempty"`
}

// NodeSelectorTerm holds when every requirement holds on the node's labels.
type NodeSelectorTerm struct {
	MatchExpressions []LabelRequirement `json:"matchExpressions"`
}

// Toleration lets pods run on nodes with a matching taint.
type Toleration struct {
	Key      string `json:"key,omitempty"`
	Operator string `json:"operator,omitempty"`
	Value    string `json:"value,omitempty"`
	Effect   string `json:"effect,omitempty"`
}

// PodAffinityTerm is a required pod (anti-)affinity term.
type PodAffinityTerm struct {
	LabelSelector     *LabelSelector `json:"labelSelector,omitempty"`
	Namespaces        []string       `json:"namespaces,omitempty"`
	NamespaceSelector *LabelSelector `json:"namespaceSelector,omitempty"`
	TopologyKey       string         `json:"topologyKey"`
}

// Namespace is a Kubernetes namespace and the labels namespaceSelectors
// match. A namespace no source declares still has the automatic
// kubernetes.io/metadata.name label; any other label on it is unknown
// (ReasonNamespaceUnresolved).
type Namespace struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
	Origin Origin            `json:"origin"`
}

// Network is the AWS topology one aws snapshot describes.
type Network struct {
	VPCs        []VPC        `json:"vpcs,omitempty"`
	Subnets     []Subnet     `json:"subnets,omitempty"`
	Interfaces  []Interface  `json:"interfaces,omitempty"`
	NATGateways []NATGateway `json:"natGateways,omitempty"`
}

// VPC is an AWS VPC and its IPv4 CIDRs; traffic inside them follows the
// local route.
type VPC struct {
	ID     string         `json:"id"`
	CIDRs  []netip.Prefix `json:"cidrs"`
	Origin Origin         `json:"origin"`
}

// Subnet is an AWS subnet.
type Subnet struct {
	ID   string       `json:"id"`
	VPC  string       `json:"vpc"`
	CIDR netip.Prefix `json:"cidr"`
	Zone string       `json:"zone"`
	// MapPublicIP says instances launched here get a public IPv4 address.
	MapPublicIP bool   `json:"mapPublicIP,omitempty"`
	Origin      Origin `json:"origin"`
}

// InterfaceKind is what an elastic network interface belongs to.
type InterfaceKind string

const (
	InterfaceNode         InterfaceKind = "node"
	InterfaceDatabase     InterfaceKind = "database"
	InterfaceLoadBalancer InterfaceKind = "load-balancer"
	InterfaceNATGateway   InterfaceKind = "nat-gateway"
	InterfaceVPCEndpoint  InterfaceKind = "vpc-endpoint"
	InterfaceInstance     InterfaceKind = "instance"
	InterfaceOther        InterfaceKind = "other"
)

// Interface is an elastic network interface: where Security Groups attach
// and where an AWS endpoint's addresses live.
type Interface struct {
	ID             string       `json:"id"`
	Subnet         string       `json:"subnet"`
	SecurityGroups []string     `json:"securityGroups"`
	PrivateIPs     []netip.Addr `json:"privateIPs"`
	// Prefixes are prefixes delegated to the interface for pod addresses.
	Prefixes  []netip.Prefix `json:"prefixes,omitempty"`
	PublicIPs []netip.Addr   `json:"publicIPs,omitempty"`
	Kind      InterfaceKind  `json:"kind"`
	// Instance is the attached EC2 instance, if any.
	Instance string `json:"instance,omitempty"`
	Origin   Origin `json:"origin"`
}

// NATGateway is an AWS NAT gateway. A public one lets its subnet's
// neighbours reach the internet and accepts nothing initiated from outside.
type NATGateway struct {
	ID        string       `json:"id"`
	Subnet    string       `json:"subnet"`
	Public    bool         `json:"public"`
	PublicIPs []netip.Addr `json:"publicIPs,omitempty"`
	Origin    Origin       `json:"origin"`
}

// ResourceKind is the kind of an AWS-side endpoint.
type ResourceKind string

const (
	ResourceDatabase     ResourceKind = "database"
	ResourceLoadBalancer ResourceKind = "load-balancer"
	ResourceInstance     ResourceKind = "instance"
	ResourceOther        ResourceKind = "other"
)

// Resource is an AWS-side endpoint (a database, a load balancer, a server)
// made of one or more interfaces. Each interface is one of its placement
// classes, the way a node group is for a workload.
type Resource struct {
	Key        EndpointKey  `json:"key"`
	Kind       ResourceKind `json:"kind"`
	Name       string       `json:"name"`
	Interfaces []string     `json:"interfaces"`
	Origin     Origin       `json:"origin"`
}

// NodeGroup is a group of nodes pods can be placed on: one placement class
// per subnet.
type NodeGroup struct {
	Name    string   `json:"name"`
	Subnets []string `json:"subnets"`
	// SecurityGroups are the groups on the nodes' interfaces; without pod
	// Security Groups, pods use them.
	SecurityGroups []string          `json:"securityGroups"`
	Labels         map[string]string `json:"labels,omitempty"`
	Taints         []Taint           `json:"taints,omitempty"`
}

// Taint keeps pods without a matching Toleration off a node group.
type Taint struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Effect string `json:"effect"`
}

// Toggle is a three-valued setting: on, off or unknown. Unknown is never
// read as either: the design forbids assuming an empty field.
type Toggle string

const (
	ToggleUnknown Toggle = ""
	ToggleOn      Toggle = "on"
	ToggleOff     Toggle = "off"
)

// ProfileSource says where a cluster profile value came from, so results
// can name their assumptions.
type ProfileSource string

const (
	ProfileFromAWS   ProfileSource = "aws"
	ProfileFromAdmin ProfileSource = "admin"
)

// ClusterProfile is the cluster settings an analysis rests on. Values read
// from AWS win; an admin fills what AWS cannot tell, and an admin value
// that disagrees with a read one raises ReasonProfileConflict.
type ClusterProfile struct {
	// CNI is the network plugin: "vpc-cni", "calico", "cilium"; "" unknown.
	CNI string `json:"cni,omitempty"`
	// NetworkPolicyEnforcement says whether NetworkPolicies are enforced at
	// all. The VPC CNI does not enforce them by default.
	NetworkPolicyEnforcement Toggle `json:"networkPolicyEnforcement,omitempty"`
	// NetworkPolicyMode is the VPC CNI mode, "standard" or "strict".
	NetworkPolicyMode string `json:"networkPolicyMode,omitempty"`
	PodSecurityGroups Toggle `json:"podSecurityGroups,omitempty"`
	// ExternalSNAT off (the VPC CNI default) means pod traffic leaving the
	// VPC takes the node's address and Security Groups.
	ExternalSNAT     Toggle `json:"externalSNAT,omitempty"`
	CustomNetworking Toggle `json:"customNetworking,omitempty"`
	// EphemeralPorts are the client ports pods use for outgoing
	// connections, which stateless filters must let back in. Nil means the
	// Linux default 32768-60999.
	EphemeralPorts *PortRange `json:"ephemeralPorts,omitempty"`
	// NodeGroups are the places pods can run. Nil means unknown
	// (ReasonPlacementUnknown) when the environment has an AWS layer, and
	// does not matter when it has none.
	NodeGroups []NodeGroup `json:"nodeGroups,omitempty"`
	// Sources names, per JSON field name above, where its value came from.
	// It is set in the merged profile an analysis assumed
	// (Analysis.Profile); in an admin-entered profile every value is the
	// admin's.
	Sources map[string]ProfileSource `json:"sources,omitempty"`
}
