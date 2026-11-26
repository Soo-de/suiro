package model

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// Verdict is the four-valued result of one path, read from the outcomes of
// its class pairs (see Path), of which there is always at least one:
//
//	Allow               every class pair Pass
//	Deny                every class pair Block
//	PlacementDependent  some class pair Pass and some Block
//	Unknown             otherwise (some Undetermined, no Pass/Block split)
//
// A path an input-level reason covers is Unknown, since every one of its
// class pairs is then Undetermined, whatever its gates say.
//
// Unknown is the path-level name of Undetermined, as Unverifiable is the
// expectation-level one (see the package comment). Allow reads "every
// examined gate allows it in every possible placement", never "the
// connection works": Analysis.Unexamined lists what was not looked at.
//
// A verdict is about the network layer alone. Deny is final: no identity
// layer (IAM, mTLS, tokens) opens what the network closes. Allow is an upper
// bound such a layer may still narrow; it stays a finding for security,
// because the network is the last barrier when identity is misconfigured or
// leaked. Identity layers can thus make Allow overstate what is open, never
// make Deny overstate what is closed.
type Verdict string

const (
	Allow              Verdict = "allow"
	Deny               Verdict = "deny"
	PlacementDependent Verdict = "placement"
	Unknown            Verdict = "unknown"
)

// Outcome is the three-valued judgment of one gate step, and of one class
// pair's whole chain: 1, 0 or ⊥ (Undetermined). A chain's outcome
// is the strong Kleene AND of its steps: Block if any step blocks, else
// Undetermined if any step is undetermined, else Pass.
type Outcome string

const (
	Pass         Outcome = "pass"
	Block        Outcome = "block"
	Undetermined Outcome = "undetermined"
)

// EndpointKey identifies a node of the graph across analyses: a workload's
// controller ObjectKey ("k8s:apps/Deployment/payments/api"), an AWS
// resource's key ("aws:database/orders-db"), EndpointInternet,
// EndpointExternalPrivate, EndpointLinkLocal, or a named network's
// "external-private:<name>" or "link-local:<name>". Paths and changes are
// keyed by it, never by IP address. No key holds a space (see
// NamedNetwork), so PathKey's written form parses back unambiguously.
type EndpointKey string

const (
	// EndpointInternet is the public IPv4 space: every address outside
	// SpecialPurposeIPv4, ClusterProfile.NotInternet, ClusterProfile.PodCIDRs
	// and the VPCs. A private source address cannot arrive through an
	// internet gateway, so private ranges are never the internet.
	EndpointInternet EndpointKey = "internet"
	// EndpointExternalPrivate is every private address outside the cluster
	// and the VPCs that no ClusterProfile.PrivateNetworks entry names: the
	// addresses of PrivateIPv4 and ClusterProfile.NotInternet, less the VPC
	// CIDRs, ClusterProfile.PodCIDRs and the named networks. Without it a
	// rule for an internal range such as 10.0.0.0/8 would face no endpoint.
	EndpointExternalPrivate EndpointKey = "external-private"
	// EndpointLinkLocal is the link-local block 169.254.0.0/16 (RFC 3927)
	// that no ClusterProfile.LinkLocalNetworks entry names. Without it,
	// egress to the instance metadata service or a node-local DNS cache
	// would face no endpoint; a forbidden rule names it, or a named part,
	// through EndpointSelector.Endpoints. Its addresses never leave their
	// link, and AWS filters them with no Security Group, Network ACL or
	// route, so a path there has Kubernetes steps only; what the host lets
	// through (the instance metadata hop limit, a host firewall) is not
	// examined.
	EndpointLinkLocal EndpointKey = "link-local"
)

// EndpointKind is what an endpoint is.
type EndpointKind string

const (
	EndpointKindWorkload EndpointKind = "workload"
	EndpointKindResource EndpointKind = "aws-resource"
	EndpointKindInternet EndpointKind = "internet"
	// EndpointKindExternalPrivate is EndpointExternalPrivate or a named
	// private network.
	EndpointKindExternalPrivate EndpointKind = "external-private"
	// EndpointKindLinkLocal is EndpointLinkLocal or a named link-local
	// network.
	EndpointKindLinkLocal EndpointKind = "link-local"
)

// Endpoint is a graph node with its resolved placement classes. It also
// keeps everything EndpointSelector matches, so a rule can be resolved
// over a stored analysis (see Finding.New).
type Endpoint struct {
	Key  EndpointKey  `json:"key"`
	Kind EndpointKind `json:"kind"`
	// ResourceKind is set for an AWS resource.
	ResourceKind ResourceKind `json:"resourceKind,omitempty"`
	// Name is, for an AWS resource, the identity the rds and arn selector
	// forms compare: an RDS DB instance's identifier, an EC2 instance's ID,
	// or a load balancer's "app/<name>/<id>", "net/<name>/<id>" or
	// "<name>".
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	// Labels are a workload's pod labels, and Tags an AWS resource's kept
	// tags (ClusterProfile.KeptTagKeys).
	Labels map[string]string `json:"labels,omitempty"`
	Tags   map[string]string `json:"tags,omitempty"`
	// Services are the Services that select a workload.
	Services []EndpointService `json:"services,omitempty"`
	// Classes are the places the endpoint can be, in class order (see the
	// package comment): for a workload one per (node group, subnet, address
	// part) its constraints allow, for an AWS resource one per interface,
	// for the internet and the external private and link-local networks
	// one per address part. In an environment without an AWS layer a
	// workload has the single class ClassCluster, whose Addresses are
	// ClusterProfile.PodCIDRs (none when unknown): node and subnet change no
	// verdict there. There is always at least one class, so no path rests
	// on zero class pairs: with an AWS layer, a workload whose constraints
	// match no node group, or whose placement cannot be told, has the single
	// class ClassUnknown (ReasonNoPlacement, ReasonPlacementUnknown).
	Classes []PlacementClass `json:"classes"`
	Source  SourceID         `json:"source,omitempty"`
	Object  ObjectKey        `json:"object,omitempty"`
	// Owner is filled by api.ScopeFilter; core leaves it zero.
	Owner Owner `json:"owner,omitzero"`
}

// EndpointService is a Service that selects a workload, resolved for that
// workload as an expectation naming the Service uses it.
type EndpointService struct {
	// Name is "<namespace>/<name>", as EndpointSelector.Service writes it.
	Name string `json:"name"`
	// Mappings are the Service's ports with TargetPort resolved for this
	// workload; a named target port the workload does not declare keeps
	// TargetPortName and leaves TargetPort 0 (ReasonServicePortUnresolved).
	Mappings []PortMapping `json:"mappings"`
}

// PlacementClass is one place an endpoint can be. Its location (NodeGroup,
// Subnet and Addresses) identifies it across analyses (see Change); its
// Security Groups and PublicAddress are attributes of that place, never
// part of its identity. No two classes of one endpoint share a location.
//
// Within one analysis a class is homogeneous: all of it meets every gate
// alike, so one outcome holds for the whole class on each path. Core cuts
// classes until that holds: by node group, by subnet, and by every prefix a
// rule or route compares the endpoint's addresses with that covers a class
// partly (a /25 rule on a /24 subnet). Which part a pod's address falls in
// is as unknown before deployment as which node it lands on, so it is
// placement too, and a split outcome makes the path PlacementDependent. A
// difference no cut separates, nodes of one group that carry different
// Security Groups, makes the steps it decides Undetermined
// (ReasonNodeSecurityGroupsMixed); a limit that stops the cuts cuts off the
// endpoint's pairs (see Limits).
type PlacementClass struct {
	// Name is a short label for people: "<node group>/<subnet>",
	// "<interface>", the network's endpoint key for the internet and the
	// external private and link-local networks, ClassCluster or
	// ClassUnknown, with an address suffix when split.
	Name           string   `json:"name"`
	NodeGroup      string   `json:"nodeGroup,omitempty"`
	Subnet         string   `json:"subnet,omitempty"`
	SecurityGroups []string `json:"securityGroups,omitempty"`
	// Addresses are the addresses this class may hold, a subset of the
	// subnet's (for a network endpoint, of its address space; for
	// ClassCluster, of the pod CIDRs), as the fewest prefixes that cover
	// them.
	Addresses []netip.Prefix `json:"addresses"`
	// PublicAddress says traffic from this class can carry a public address
	// to an internet gateway: the node's (pods leave the VPC as the node)
	// or the interface's.
	PublicAddress bool `json:"publicAddress,omitempty"`
}

const (
	// ClassUnknown is the Name of the one class of a workload whose
	// placement cannot be told or matches no node group; it occurs only
	// with an AWS layer.
	ClassUnknown = "unknown"
	// ClassCluster is the Name of the one class of a workload in an
	// environment without an AWS layer.
	ClassCluster = "cluster"
)

// ClassPair is one combination of a path's source and destination classes.
// A SameNode pair is both ends on one node, where only NetworkPolicy
// applies: two workload classes of the same NodeGroup and Subnet on a path
// whose SameNode is set, so it exists only with an AWS layer.
type ClassPair struct {
	Source      PlacementClass `json:"source"`
	Destination PlacementClass `json:"destination"`
	SameNode    bool           `json:"sameNode,omitempty"`
}

// PathKey identifies a path across analyses: source, destination and a
// port range of one protocol. Its string form, "<source> → <destination> :
// <protocol>/<from>[-<to>]", is the connection key of stored changes.
//
// The ports are destination container ports: what NetworkPolicy and
// Security Groups see once a Service has translated the connection. Every
// step is judged on them, the source's egress included, because the
// translation happens at the source. A Service port shows only in a
// Service step (GateResult.Mappings) and in an expectation that names a
// Service.
type PathKey struct {
	Source      EndpointKey `json:"source"`
	Destination EndpointKey `json:"destination"`
	Ports       PortRange   `json:"ports"`
}

// String returns k's one written form, "<source> → <destination> :
// <protocol>/<from>[-<to>]", with "-<to>" left out for a single port
// ("tcp/5432"). Finding.Subject and exceptions match this exact text.
func (k PathKey) String() string {
	s := string(k.Source) + pathArrow + string(k.Destination) + pathColon +
		string(k.Ports.Protocol) + "/" + strconv.Itoa(int(k.Ports.From))
	if k.Ports.To != k.Ports.From {
		s += "-" + strconv.Itoa(int(k.Ports.To))
	}
	return s
}

// The separators of PathKey's written form.
const (
	pathArrow = " → "
	pathColon = " : "
)

// ParsePathKey is the inverse of PathKey.String. It accepts only that form,
// so ParsePathKey(k.String()) returns k for every valid k, and it errors on
// anything else ("tcp/5432-5432" included).
func ParsePathKey(s string) (PathKey, error) {
	bad := func(why string) (PathKey, error) {
		return PathKey{}, fmt.Errorf("model: path key %q: %s", s, why)
	}
	source, rest, arrow := strings.Cut(s, pathArrow)
	destination, ports, colon := strings.Cut(rest, pathColon)
	if !arrow || !colon {
		return bad("want <source> → <destination> : <protocol>/<ports>")
	}
	for _, end := range []string{source, destination} {
		if end == "" || strings.Contains(end, " ") {
			return bad("an endpoint is empty or holds a space")
		}
	}
	protocol, numbers, _ := strings.Cut(ports, "/")
	r := PortRange{Protocol: Protocol(protocol)}
	if r.Protocol != ProtocolTCP && r.Protocol != ProtocolUDP {
		return bad("protocol is not tcp or udp")
	}
	from, to, ranged := strings.Cut(numbers, "-")
	var ok bool
	if r.From, ok = parsePort(from); !ok {
		return bad("bad port")
	}
	r.To = r.From
	if ranged {
		if r.To, ok = parsePort(to); !ok || r.To <= r.From {
			return bad("bad end of range")
		}
	}
	return PathKey{Source: EndpointKey(source), Destination: EndpointKey(destination), Ports: r}, nil
}

// parsePort reads a port as PathKey.String writes it: decimal digits with no
// sign or leading zero.
func parsePort(s string) (uint16, bool) {
	n, err := strconv.ParseUint(s, 10, 16)
	return uint16(n), err == nil && (s == "0" || s[0] != '0')
}

// Path is the summary of one source/destination pair over one port range
// whose ports all behave alike in every gate. The paths of a pair partition
// both protocols' port space; neighboring ranges with equal results are
// merged. The chains behind it are not stored: core.Analyzer.Explain
// recomputes them as a PathDetail from the same Bundle.
//
// The path's class pairs are every source class with every destination
// class and, when SameNode is set, each pair of classes with the same
// NodeGroup and Subnet once more as a SameNode pair, in pair order (see the
// package comment), so the stored endpoints alone give them. A class pair's
// outcome is the strong Kleene AND of its chain's steps, or Undetermined
// when an input-level reason covers the path.
type Path struct {
	Key     PathKey `json:"key"`
	Verdict Verdict `json:"verdict"`
	// Reasons are the reason codes behind an Unknown verdict or an
	// Undetermined step, sorted and without repeats.
	Reasons []ReasonCode `json:"reasons,omitempty"`
	// SameNode says the two ends may share a node. It is set only with an
	// AWS layer and two workload ends: when their placement constraints let
	// them share a node, and on every such pair a limit cuts off (see
	// Limits). It is the same on every path of the pair.
	SameNode bool `json:"sameNode,omitempty"`
	// Outcomes holds the outcome of each class pair in pair order, one
	// letter each: "P" Pass, "B" Block, "U" Undetermined. It is empty when
	// every class pair has the same outcome, the one Verdict names (Allow
	// Pass, Deny Block, Unknown Undetermined). It is the only record of
	// class outcomes: counts by outcome are derived from it, and Change
	// compares it.
	Outcomes string `json:"outcomes,omitempty"`
}

// PathDetail is one path with its chains. Its Key, Verdict and Reasons
// equal those of the path's summary, and its Classes are the summary's
// class pairs, in the same order and with the same outcomes.
type PathDetail struct {
	Key     PathKey      `json:"key"`
	Verdict Verdict      `json:"verdict"`
	Reasons []ReasonCode `json:"reasons,omitempty"`
	// Classes holds one result per class pair, so PlacementDependent can
	// name where the path is open and where it is closed. It is never empty
	// (see Endpoint.Classes).
	Classes []ClassResult `json:"classes"`
}

// ClassResult is the chain of one class pair.
type ClassResult struct {
	Pair ClassPair `json:"pair"`
	// Outcome is the class pair's outcome as Path defines it; under an
	// input-level reason it is Undetermined while Gates still show what
	// each step says.
	Outcome Outcome `json:"outcome"`
	// Gates is the chain in path order. Every step is evaluated, not only
	// up to the first block, so a Deny lists every blocking step. A Service
	// step (GateTransform) appears where a Service that selects the
	// destination maps a port onto the path's port; it passes. An SNAT step
	// appears only with an AWS layer, on traffic from a pod that leaves the
	// VPC.
	Gates []GateResult `json:"gates"`
}

// GateRef names one chain step: the gates of one Layer and Direction that
// guard the same target, joined (every Security Group on an interface,
// every NetworkPolicy selecting a pod). An Isolating step with no Members
// that is not Redacted means no policy selects the target, which passes.
type GateRef struct {
	Layer     Layer     `json:"layer"`
	Type      GateType  `json:"type"`
	Direction Direction `json:"direction,omitempty"`
	Members   []GateID  `json:"members"`
	// Owners are the distinct owners of Members, sorted by Team then
	// Application, with the zero Owner for a member nobody owns; filled by
	// api.ScopeFilter, core leaves them empty. A joined step can have
	// several (a platform default-deny policy and the application's own
	// policy on one pod). They stay when the step is redacted, so a block
	// always names whom to contact.
	Owners []Owner `json:"owners,omitempty"`
	// Redacted means a ScopeFilter removed content the caller may not see.
	// This is the one definition of redaction, applied per member: a member
	// whose owner one of the caller's scopes matches stays whole; for every
	// other member, its rules leave GateResult.Rules and its Service ports
	// GateResult.Mappings, and a Kubernetes member also leaves Members while
	// an AWS member keeps its ID there (a resource ID). Layer, Type,
	// Direction, Owners, the step's Outcome and Reason always stay, as do
	// the path's key and ports.
	Redacted bool `json:"redacted,omitempty"`
}

// GateResult is one step's outcome on a path.
type GateResult struct {
	Gate    GateRef `json:"gate"`
	Outcome Outcome `json:"outcome"`
	// Rules are the rules that decided: for a union filter every matching
	// allow rule; for an ordered filter the first match, or its default rule
	// (Locator "*") when nothing matched, for each direction it checked;
	// for a route the longest-prefix route.
	Rules []RuleRef `json:"rules,omitempty"`
	// Reason is set when Outcome is Undetermined.
	Reason ReasonCode `json:"reason,omitempty"`
	// Mappings is set on a Service step: the Service ports that lead to the
	// path's port, each with its Service, so a reader sees 80→8080.
	Mappings []MappingRef `json:"mappings,omitempty"`
	// TranslatedTo is set on an SNAT step: the addresses and Security Groups
	// the source takes from there on (its node's, or the NAT gateway's).
	// Every later step judges the source by them, so workloads behind the
	// same address look alike to it.
	TranslatedTo *PlacementClass `json:"translatedTo,omitempty"`
}

// RuleRef points at one deciding rule.
type RuleRef struct {
	Gate GateID `json:"gate"`
	// Return marks a rule a stateless filter applied to the replies.
	Return bool   `json:"return,omitempty"`
	Origin Origin `json:"origin"`
}

// MappingRef is one Service port of a Service step. A step joins every
// Service that selects the destination, so Gate tells apart two of them
// with the same port.
type MappingRef struct {
	// Gate is the Service's GateID, one of the step's Members.
	Gate    GateID      `json:"gate"`
	Mapping PortMapping `json:"mapping"`
}

// Unexamined names what no analysis looks at, listed with every result so
// that "allowed" is never read as "works".
type Unexamined string

const (
	UnexaminedHostFirewall Unexamined = "host-firewall"
	UnexaminedApplication  Unexamined = "application"
	UnexaminedTLS          Unexamined = "tls"
	// UnexaminedAWSLayer appears when the environment has no AWS source:
	// Security Groups, Network ACLs and routes were not examined, and
	// verdicts rest on the Kubernetes layer alone.
	UnexaminedAWSLayer Unexamined = "aws-layer"
	// UnexaminedIPv6 is always listed: the model is IPv4, adapters drop
	// IPv6 peers, rules and routes, and no result says anything about IPv6
	// traffic.
	UnexaminedIPv6 Unexamined = "ipv6"
	// UnexaminedPairs is always listed: the pairs Analysis.Paths does not
	// compute have no path, and no expectation is checked over them.
	UnexaminedPairs Unexamined = "uncomputed-pairs"
)
