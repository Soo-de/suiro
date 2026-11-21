package model

import "net/netip"

// Verdict is the four-valued result of one path. Over a path's class
// results, of which there is always at least one, it is:
//
//	Unknown             an input-level reason covers the path
//	Allow               every class pair Pass
//	Deny                every class pair Block
//	PlacementDependent  some class pair Pass and some Block
//	Unknown             otherwise (some Undetermined, no Pass/Block split)
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
// resource's key ("aws:database/orders-db"), or EndpointInternet. Paths and
// changes are keyed by it, never by IP address.
type EndpointKey string

// EndpointInternet is the internet: every IPv4 address outside the VPCs.
const EndpointInternet EndpointKey = "internet"

// EndpointKind is what an endpoint is.
type EndpointKind string

const (
	EndpointKindWorkload EndpointKind = "workload"
	EndpointKindResource EndpointKind = "aws-resource"
	EndpointKindInternet EndpointKind = "internet"
)

// Endpoint is a graph node with its resolved placement classes.
type Endpoint struct {
	Key       EndpointKey  `json:"key"`
	Kind      EndpointKind `json:"kind"`
	Name      string       `json:"name"`
	Namespace string       `json:"namespace,omitempty"`
	// Classes are the places the endpoint can be: for a workload one per
	// (node group, subnet, address part) its constraints allow, for an AWS
	// resource one per interface, for the internet one per address part.
	// There is always at least one, so no path rests on zero class pairs:
	// a workload whose constraints match no node group, or whose placement
	// cannot be told, has the single class ClassUnknown, and core adds an
	// Undetermined endpoint step to its paths (ReasonNoPlacement,
	// ReasonPlacementUnknown).
	Classes []PlacementClass `json:"classes"`
	Source  SourceID         `json:"source,omitempty"`
	Object  ObjectKey        `json:"object,omitempty"`
	// Owner is filled by api.ScopeFilter; core leaves it zero.
	Owner Owner `json:"owner,omitzero"`
}

// PlacementClass is one place an endpoint can be: node group, subnet,
// Security Groups, and the part of the subnet's addresses it may hold.
//
// An address part splits a class only when some rule on the path covers the
// subnet partly (a /25 rule on a /24 subnet): which half a pod's address
// falls in is as unknown before deployment as which node it lands on, so
// it is placement too, and a split outcome makes the path
// PlacementDependent.
type PlacementClass struct {
	// Name is a short label for people: "<node group>/<subnet>",
	// "<interface>", "internet", with an address suffix when split.
	Name           string   `json:"name"`
	NodeGroup      string   `json:"nodeGroup,omitempty"`
	Subnet         string   `json:"subnet,omitempty"`
	SecurityGroups []string `json:"securityGroups,omitempty"`
	// Addresses are the addresses this class may hold, a subset of the
	// subnet's (or, for the internet, of the space outside the VPCs).
	Addresses []netip.Prefix `json:"addresses"`
	// PublicAddress says traffic from this class can carry a public address
	// to an internet gateway: the node's (pods leave the VPC as the node)
	// or the interface's.
	PublicAddress bool `json:"publicAddress,omitempty"`
}

// ClassUnknown is the Name of the one class of a workload whose placement
// cannot be told or matches no node group.
const ClassUnknown = "unknown"

// ClassPair is one combination of a path's source and destination classes.
// SameNode pairs exist only when both ends may share a node; between them
// only NetworkPolicy applies.
type ClassPair struct {
	Source      PlacementClass `json:"source"`
	Destination PlacementClass `json:"destination"`
	SameNode    bool           `json:"sameNode,omitempty"`
}

// PathKey identifies a path across analyses: source, destination and a
// port range of one protocol. Its string form, "<source> → <destination> :
// <protocol>/<from>[-<to>]", is the connection key of stored changes.
type PathKey struct {
	Source      EndpointKey `json:"source"`
	Destination EndpointKey `json:"destination"`
	Ports       PortRange   `json:"ports"`
}

// Path is the evaluation of one source/destination pair over one port
// range whose ports all behave alike in every gate. The paths of a pair
// partition both protocols' port space; neighboring ranges with equal
// results are merged.
type Path struct {
	Key     PathKey `json:"key"`
	Verdict Verdict `json:"verdict"`
	// Classes holds one result per class pair, so PlacementDependent can
	// name where the path is open and where it is closed. It is never empty
	// (see Endpoint.Classes).
	Classes []ClassResult `json:"classes"`
	// Reasons are the reason codes behind an Unknown verdict or an
	// Undetermined step, sorted and without repeats.
	Reasons []ReasonCode `json:"reasons,omitempty"`
}

// ClassResult is the chain of one class pair.
type ClassResult struct {
	Pair    ClassPair `json:"pair"`
	Outcome Outcome   `json:"outcome"`
	// Gates is the chain in path order. Every step is evaluated, not only
	// up to the first block, so a Deny lists every blocking step.
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
	// other member, its rules leave GateResult.Rules, and a Kubernetes
	// member also leaves Members while an AWS member keeps its ID there (a
	// resource ID). Layer, Type, Direction, Owners, the step's Outcome and
	// Reason always stay, as do the path's key and ports.
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
}

// RuleRef points at one deciding rule.
type RuleRef struct {
	Gate GateID `json:"gate"`
	// Return marks a rule a stateless filter applied to the replies.
	Return bool   `json:"return,omitempty"`
	Origin Origin `json:"origin"`
}

// Unexamined names a gate no analysis looks at, listed with every result so
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
)
