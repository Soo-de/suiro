package model

// ReasonCode names why something was not judged normally. Every warning,
// Undetermined step and Unknown verdict carries one; the status guide lists
// each code with what to do about it. Codes travel as strings on every wire,
// so a new code needs no proto change.
type ReasonCode string

// ReasonLevel is what a reason does to verdicts.
type ReasonLevel string

const (
	// LevelGate makes one chain step Undetermined. Another step that blocks
	// still makes the path Deny (strong Kleene AND), and a passing step is
	// unaffected.
	LevelGate ReasonLevel = "gate"
	// LevelPartial is LevelGate for the affected steps, and also marks the
	// analysis partial because something outside the model acts on the
	// network.
	LevelPartial ReasonLevel = "partial"
	// LevelInput means the inputs behind a path are incomplete, so its gates
	// cannot be trusted: every path with an endpoint in the problem's scope
	// is Unknown whatever its gates say, and the analysis is partial. The
	// scope is the source's destination namespace for a git source and every
	// path that crosses an AWS gate for an aws source.
	LevelInput ReasonLevel = "input"
	// LevelWarning is reported and changes no verdict.
	LevelWarning ReasonLevel = "warning"
)

// Gate-level reasons.
const (
	// ReasonPolicyEnforcementUnknown: the cluster profile does not say
	// whether NetworkPolicies are enforced. A NetworkPolicy step that would
	// block is Undetermined; one that passes stays Pass.
	ReasonPolicyEnforcementUnknown ReasonCode = "policy-enforcement-unknown"
	// ReasonHostNetwork: the pod uses the node's network namespace, so
	// NetworkPolicy does not hold as written; its Kubernetes steps are
	// Undetermined.
	ReasonHostNetwork ReasonCode = "host-network"
	// ReasonStandalonePod: a Pod without a controller; core adds an
	// Undetermined endpoint step to its paths.
	ReasonStandalonePod ReasonCode = "standalone-pod"
	// ReasonPolicyKindUnsupported: a Calico, Cilium or cluster-wide policy
	// object can override NetworkPolicy; the Kubernetes steps of the pods it
	// may cover are Undetermined.
	ReasonPolicyKindUnsupported ReasonCode = "policy-kind-unsupported"
	// ReasonNamespaceUnresolved: a namespaceSelector depends on labels of a
	// namespace no source declares; matching that peer is Undetermined.
	ReasonNamespaceUnresolved ReasonCode = "namespace-unresolved"
	// ReasonServicePortMismatch: with the VPC CNI, a Service whose port
	// differs from the container port makes NetworkPolicy enforcement on it
	// unreliable.
	ReasonServicePortMismatch ReasonCode = "service-port-mismatch"
	// ReasonRouteOutOfScope: the longest-prefix route leads to a transit
	// gateway, peering, VPN, local or carrier gateway.
	ReasonRouteOutOfScope ReasonCode = "route-out-of-scope"
	// ReasonRouteMiddlebox: a route more specific than local sends VPC
	// traffic through an appliance (an interface, a Gateway Load Balancer
	// endpoint, Network Firewall).
	ReasonRouteMiddlebox ReasonCode = "route-middlebox"
	// ReasonPrefixListUnresolved: a rule or route uses a managed prefix
	// list, which is not read.
	ReasonPrefixListUnresolved ReasonCode = "prefix-list-unresolved"
	// ReasonSecurityGroupUnresolved: a rule references a Security Group no
	// snapshot holds (another VPC or account).
	ReasonSecurityGroupUnresolved ReasonCode = "security-group-unresolved"
	// ReasonEphemeralPortsPartial: a stateless filter lets replies back on
	// only part of the client's ephemeral port range.
	ReasonEphemeralPortsPartial ReasonCode = "ephemeral-ports-partial"
	// ReasonPodSecurityGroups: Security Groups for pods are on, so pods may
	// not use their node's groups; their Security Group steps are
	// Undetermined.
	ReasonPodSecurityGroups ReasonCode = "pod-security-groups"
	// ReasonCustomNetworking: VPC CNI custom networking puts pods in other
	// subnets and groups than their nodes; their AWS steps are
	// Undetermined.
	ReasonCustomNetworking ReasonCode = "custom-networking"
	// ReasonPlacementUnknown: where pods run cannot be told (node groups
	// unknown, nodeName set, constraints on fields the model lacks); their
	// AWS steps are Undetermined.
	ReasonPlacementUnknown ReasonCode = "placement-unknown"
	// ReasonNoPlacement: the workload's constraints match no node group, so
	// its pods would never run; core adds an Undetermined endpoint step.
	ReasonNoPlacement ReasonCode = "no-placement"
	// ReasonAWSReadStale: the last complete AWS read is older than the
	// configured limit; AWS steps are Undetermined.
	ReasonAWSReadStale ReasonCode = "aws-read-stale"
)

// Partial-level reasons.
const (
	// ReasonAWSControllerEffect: a LoadBalancer Service, an ALB Ingress or a
	// TargetGroupBinding makes the AWS Load Balancer Controller write
	// Security Group rules the model does not see. Paths from the internet
	// and load balancers to the exposed pods are Undetermined.
	ReasonAWSControllerEffect ReasonCode = "aws-controller-effect"
)

// Input-level reasons. Their Subject is a SourceID, the source whose scope
// they cover; a problem that involves several sources is raised once for
// each.
const (
	ReasonRenderFailed  ReasonCode = "render-failed"
	ReasonRenderTimeout ReasonCode = "render-timeout"
	// ReasonRenderLimit: the render hit its memory or size limit, or the
	// files sent to it exceeded the message limit.
	ReasonRenderLimit ReasonCode = "render-limit"
	// ReasonRemoteResource: the render needs something outside the
	// repository (a remote chart dependency or base, a plugin, an exec
	// function), which the renderer refuses.
	ReasonRemoteResource ReasonCode = "remote-resource"
	// ReasonSourcePathMissing: the registered path does not exist at the
	// revision.
	ReasonSourcePathMissing ReasonCode = "source-path-missing"
	// ReasonAccessDenied: the collector cannot read a source it has never
	// read. (Losing access later keeps the earlier read, with
	// ReasonSourceStale.)
	ReasonAccessDenied ReasonCode = "access-denied"
	// ReasonSourceUnread: the source has no complete read yet.
	ReasonSourceUnread ReasonCode = "source-unread"
	// ReasonPullRequestNotMergeable: GitHub has no test merge commit.
	ReasonPullRequestNotMergeable ReasonCode = "pr-not-mergeable"
	// ReasonConflictingResource: two sources produce the same ObjectKey with
	// different content. Core raises it once for each source involved, with
	// that source as Subject and the object's Origin in that source, so the
	// scope of every source involved is Unknown.
	ReasonConflictingResource ReasonCode = "conflicting-resource"
	// ReasonIPFamilyUnsupported: the cluster or VPC is IPv6; the model is
	// IPv4.
	ReasonIPFamilyUnsupported ReasonCode = "ip-family-unsupported"
)

// Warning-level reasons.
const (
	// ReasonSourceStale: the latest read attempt failed; the analysis uses
	// the read that finished at the source's LastCompleteRead.
	ReasonSourceStale ReasonCode = "source-stale"
	// ReasonContractVersionUnknown: a snapshot of a contract version this
	// build does not know was ignored and the previous read kept.
	ReasonContractVersionUnknown ReasonCode = "contract-version-unknown"
	// ReasonKindUnrecognized: a Kubernetes kind in none of KeptFields,
	// UnsupportedKinds and IgnoredKinds.
	ReasonKindUnrecognized ReasonCode = "kind-unrecognized"
	// ReasonDuplicateResource: two sources produce the same ObjectKey with
	// equal content.
	ReasonDuplicateResource ReasonCode = "duplicate-resource"
	// ReasonNamespaceMismatch: an object declares a namespace other than its
	// registration's destination; it is kept in the declared one.
	ReasonNamespaceMismatch ReasonCode = "namespace-mismatch"
	// ReasonRegistrationOverlap: two registrations read the same repository
	// path or target the same namespace.
	ReasonRegistrationOverlap ReasonCode = "registration-overlap"
	// ReasonUnowned: no registration produced the resource and no ownership
	// rule matches it; only callers with the "*" scope see its content.
	ReasonUnowned ReasonCode = "unowned"
	// ReasonOwnershipConflict: an ownership rule names another owner than
	// the registration the resource came from; the registration wins.
	ReasonOwnershipConflict ReasonCode = "ownership-conflict"
	// ReasonNamedPortUnresolved: a NetworkPolicy names a port no container
	// of the destination declares. As in Kubernetes, the rule then allows
	// nothing on that pod; the warning is there because it is usually a
	// typo.
	ReasonNamedPortUnresolved ReasonCode = "named-port-unresolved"
	// ReasonHostPort: a container hostPort exposes the pod on its node's
	// address, a path the model does not follow.
	ReasonHostPort ReasonCode = "host-port"
	// ReasonSymlinkSkipped: git-collector does not pass symbolic links to
	// the renderer.
	ReasonSymlinkSkipped ReasonCode = "symlink-skipped"
	// ReasonProfileConflict: an admin-entered cluster profile value differs
	// from the one read from AWS; the AWS value wins.
	ReasonProfileConflict ReasonCode = "profile-conflict"
	// ReasonExpectationsInvalid: an application's expectations file, or one
	// entry in it, does not validate (see ExpectationsFile); what was
	// rejected is not checked. The Subject is the file's path for the whole
	// file and the entry's ObjectKey for one entry.
	ReasonExpectationsInvalid ReasonCode = "expectations-invalid"
)

// ReasonLevels gives every reason code its level. A code missing here is a
// bug; tests check that every declared code is listed.
var ReasonLevels = map[ReasonCode]ReasonLevel{
	ReasonPolicyEnforcementUnknown: LevelGate,
	ReasonHostNetwork:              LevelGate,
	ReasonStandalonePod:            LevelGate,
	ReasonPolicyKindUnsupported:    LevelGate,
	ReasonNamespaceUnresolved:      LevelGate,
	ReasonServicePortMismatch:      LevelGate,
	ReasonRouteOutOfScope:          LevelGate,
	ReasonRouteMiddlebox:           LevelGate,
	ReasonPrefixListUnresolved:     LevelGate,
	ReasonSecurityGroupUnresolved:  LevelGate,
	ReasonEphemeralPortsPartial:    LevelGate,
	ReasonPodSecurityGroups:        LevelGate,
	ReasonCustomNetworking:         LevelGate,
	ReasonPlacementUnknown:         LevelGate,
	ReasonNoPlacement:              LevelGate,
	ReasonAWSReadStale:             LevelGate,

	ReasonAWSControllerEffect: LevelPartial,

	ReasonRenderFailed:            LevelInput,
	ReasonRenderTimeout:           LevelInput,
	ReasonRenderLimit:             LevelInput,
	ReasonRemoteResource:          LevelInput,
	ReasonSourcePathMissing:       LevelInput,
	ReasonAccessDenied:            LevelInput,
	ReasonSourceUnread:            LevelInput,
	ReasonPullRequestNotMergeable: LevelInput,
	ReasonConflictingResource:     LevelInput,
	ReasonIPFamilyUnsupported:     LevelInput,

	ReasonSourceStale:            LevelWarning,
	ReasonContractVersionUnknown: LevelWarning,
	ReasonKindUnrecognized:       LevelWarning,
	ReasonDuplicateResource:      LevelWarning,
	ReasonNamespaceMismatch:      LevelWarning,
	ReasonRegistrationOverlap:    LevelWarning,
	ReasonUnowned:                LevelWarning,
	ReasonOwnershipConflict:      LevelWarning,
	ReasonNamedPortUnresolved:    LevelWarning,
	ReasonHostPort:               LevelWarning,
	ReasonSymlinkSkipped:         LevelWarning,
	ReasonProfileConflict:        LevelWarning,
	ReasonExpectationsInvalid:    LevelWarning,
}
