package model

import "slices"

// ReasonCode names why something was not judged normally. Every warning,
// Undetermined step and Unknown verdict carries one. Codes travel as
// strings on every wire, so a new code needs no proto change.
type ReasonCode string

// ReasonLevel is what a reason does to verdicts.
type ReasonLevel string

const (
	// LevelGate makes chain steps Undetermined. Another step that blocks
	// still makes the path Deny (strong Kleene AND), and a passing step is
	// unaffected.
	LevelGate ReasonLevel = "gate"
	// LevelPartial is LevelGate for the affected steps, and also marks the
	// analysis partial because something outside the model acts on the
	// network.
	LevelPartial ReasonLevel = "partial"
	// LevelInput means the inputs behind a path are incomplete, so its gates
	// cannot be trusted: every path in the problem's scope is Unknown
	// whatever its gates say, and the analysis is partial.
	//
	// With ScopeSource the Subject is a SourceID, and the scope follows the
	// source's kind:
	//   - aws: every path that crosses an AWS gate.
	//   - git: every path with an end in its destination namespace or in a
	//     namespace its objects used (metadata.namespace) in its latest
	//     usable reads, which inputs supplies in Bundle.UsedNamespaces; so it
	//     also covers a ReasonConflictingResource object outside the
	//     destination namespaces. A namespace that the failing content writes
	//     into for the first time cannot be known: paths there keep the
	//     verdicts their gates give, and the analysis is AnalysisPartial with
	//     the source's reason.
	//   - cluster: only the namespaceSelector matches that would use the
	//     labels of its read, since the read holds nothing else. The read is
	//     not used, and those matches fall back as Namespace lists: to
	//     ClusterProfile.NamespaceLabels, else Undetermined
	//     (ReasonNamespaceLabelsUnknown). No path becomes Unknown through it.
	LevelInput ReasonLevel = "input"
	// LevelWarning is reported and changes no verdict.
	LevelWarning ReasonLevel = "warning"
)

// Producer is a component that raises reason codes.
type Producer string

const (
	ProducerRenderer       Producer = "renderer"
	ProducerGitCollector   Producer = "git-collector"
	ProducerCloudCollector Producer = "cloud-collector"
	// ProducerInputs raises into Bundle.Problems, before analysis.
	ProducerInputs Producer = "inputs"
	// ProducerAdapter is a core.Adapter, raising into Fragment.Problems
	// from one snapshot alone.
	ProducerAdapter Producer = "adapter"
	// ProducerCore is core.Analyzer, after merging every fragment and the
	// cluster profile.
	ProducerCore Producer = "core"
	// ProducerScope is package scope, when it resolves owners for a reader.
	ProducerScope Producer = "scope"
)

// ReasonScope is how far a reason reaches; the code's own comment says
// exactly which steps or paths.
type ReasonScope string

const (
	// ScopeNone reaches nothing: the reason is reported only.
	ScopeNone ReasonScope = "none"
	// ScopeStep reaches the chain steps the code's comment names.
	ScopeStep ReasonScope = "step"
	// ScopeEndpoint reaches every path of the endpoint the Subject names,
	// through an Undetermined endpoint step.
	ScopeEndpoint ReasonScope = "endpoint"
	// ScopeNamespace reaches every path with an end in the namespace of the
	// object the Subject names; for a cluster-scoped object, the scope of
	// its source as ScopeSource.
	ScopeNamespace ReasonScope = "namespace"
	// ScopeSource reaches the scope of the source the Subject names, as
	// LevelInput defines it.
	ScopeSource ReasonScope = "source"
	// ScopeExpectation reaches the status of the expectation the Subject
	// names, which becomes Unverifiable; no verdict changes.
	ScopeExpectation ReasonScope = "expectation"
	// ScopePaths reaches the paths the code's comment names.
	ScopePaths ReasonScope = "paths"
	// ScopeAnalysis reaches the whole analysis.
	ScopeAnalysis ReasonScope = "analysis"
)

// ReasonSpec is one code's row of the reason table.
type ReasonSpec struct {
	Level ReasonLevel
	// RaisedBy is every component that may raise the code; no other one
	// does, which the adapter conformance suite checks for adapters.
	RaisedBy []Producer
	Scope    ReasonScope
}

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
	// object can override NetworkPolicy. For a namespaced object the
	// Kubernetes steps of every pod in its namespace are Undetermined; for a
	// cluster-scoped one, every Kubernetes step of the environment.
	ReasonPolicyKindUnsupported ReasonCode = "policy-kind-unsupported"
	// ReasonNamespaceLabelsUnknown: a namespaceSelector depends on labels of
	// a namespace that no git source declares, no usable cluster read holds
	// and the profile does not list (see Namespace); matching that peer is
	// Undetermined.
	ReasonNamespaceLabelsUnknown ReasonCode = "namespace-labels-unknown"
	// ReasonNamespaceLabelsConflict: two git sources declare one namespace
	// with different labels, so which apply after deployment cannot be
	// told; matching a namespaceSelector against it is Undetermined. The
	// Subject is the namespace.
	ReasonNamespaceLabelsConflict ReasonCode = "namespace-labels-conflict"
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
	// ReasonNodeSecurityGroupsMixed: the nodes of one node group carry
	// different Security Groups, so which ones a pod's node carries cannot
	// be told; the Security Group steps of the group's classes are
	// Undetermined, so no class mixes them (see PlacementClass).
	ReasonNodeSecurityGroupsMixed ReasonCode = "node-security-groups-mixed"
	// ReasonCustomNetworking: VPC CNI custom networking puts pods in other
	// subnets and groups than their nodes; their AWS steps are
	// Undetermined.
	ReasonCustomNetworking ReasonCode = "custom-networking"
	// ReasonPlacementUnknown: with an AWS layer, where a workload's pods run
	// cannot be told (node groups unknown, nodeName set, constraints on
	// fields the model lacks). The workload has the single class
	// ClassUnknown: core judges its Kubernetes steps as usual and adds one
	// Undetermined endpoint step in place of the AWS steps it cannot place.
	ReasonPlacementUnknown ReasonCode = "placement-unknown"
	// ReasonNoPlacement: with an AWS layer, the workload's constraints match
	// no node group, so its pods would never run; it has the single class
	// ClassUnknown and an Undetermined endpoint step, as for
	// ReasonPlacementUnknown.
	ReasonNoPlacement ReasonCode = "no-placement"
	// ReasonAWSReadStale: the last complete AWS read is older than the
	// configured limit; AWS steps are Undetermined.
	ReasonAWSReadStale ReasonCode = "aws-read-stale"
	// ReasonPodAddressesUnknown: ClusterProfile.PodCIDRs is unknown, so a
	// NetworkPolicy ipBlock cannot be compared with a pod; matching an
	// ipBlock peer against a pod endpoint is Undetermined, unless
	// ClusterProfile.IPBlockAppliesToPods is off.
	ReasonPodAddressesUnknown ReasonCode = "pod-addresses-unknown"
	// ReasonIPBlockPodMatchUnknown: ClusterProfile.IPBlockAppliesToPods is
	// unknown; matching an ipBlock peer whose addresses (its cidr less its
	// except blocks) overlap the pod CIDRs against a pod endpoint is
	// Undetermined.
	ReasonIPBlockPodMatchUnknown ReasonCode = "ipblock-pod-match-unknown"
	// ReasonSNATUnknown: ClusterProfile.ExternalSNAT is unknown, so whether
	// pod traffic leaving the VPC takes its node's address cannot be told;
	// on such paths the steps after the VPC boundary are Undetermined.
	ReasonSNATUnknown ReasonCode = "snat-unknown"
)

// Partial-level reasons.
const (
	// ReasonAWSControllerEffect: a LoadBalancer Service, an ALB Ingress or a
	// TargetGroupBinding makes the AWS Load Balancer Controller write
	// Security Group rules the model does not see. Paths from the internet
	// and load balancers to the exposed pods are Undetermined.
	ReasonAWSControllerEffect ReasonCode = "aws-controller-effect"
	// ReasonWorkloadKindUnsupported: an object of a kind outside the model
	// carries a pod template (spec.template.spec.containers, as an Argo
	// Rollout does), so it creates pods the model does not see. Every path
	// with an end in its namespace gets an Undetermined endpoint step. A
	// kind without a pod template raises ReasonKindUnrecognized instead.
	ReasonWorkloadKindUnsupported ReasonCode = "workload-kind-unsupported"
)

// Input-level reasons. The Subject is the SourceID whose scope the problem
// covers, and a problem that involves several sources is raised once for
// each; ReasonObjectInvalid instead names the object's ObjectKey
// (ScopeNamespace), and ReasonAnalysisLimit and ReasonAnalysisTimeout name
// theirs in their own comments.
const (
	ReasonRenderFailed  ReasonCode = "render-failed"
	ReasonRenderTimeout ReasonCode = "render-timeout"
	// ReasonRenderLimit: the render hit its memory limit, its result would
	// hold more objects than the snapshot object limit or not fit the
	// message size limit both ends share, the files sent to it exceeded
	// that message limit, or it would read a file sent truncated.
	ReasonRenderLimit ReasonCode = "render-limit"
	// ReasonRemoteResource: the render needs something outside the
	// repository that nothing fetched for it (a chart dependency from a
	// repository outside the allowed list or not pinned in Chart.lock, a
	// remote kustomize base, a plugin, an exec function), which the
	// renderer refuses.
	ReasonRemoteResource ReasonCode = "remote-resource"
	// ReasonChartDependencyUnavailable: git-collector could not fetch a
	// chart dependency that Chart.lock pins from an allowed repository.
	ReasonChartDependencyUnavailable ReasonCode = "chart-dependency-unavailable"
	// ReasonSourcePathMissing: the registered path does not exist at the
	// revision.
	ReasonSourcePathMissing ReasonCode = "source-path-missing"
	// ReasonSymlinkUnresolved: a symbolic link under the registered path
	// does not point to a regular file inside the repository, so a manifest
	// or template behind it may be missing from the render.
	ReasonSymlinkUnresolved ReasonCode = "symlink-unresolved"
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
	// ReasonObjectInvalid: an object the API server would reject (a CIDR
	// that does not parse, an except outside its cidr, endPort below port, an
	// unknown selector operator), so it is never applied and what its
	// namespace allows after deployment cannot be told. One invalid rule
	// makes the whole object invalid. Origin.Locator names the field.
	ReasonObjectInvalid ReasonCode = "object-invalid"
	// ReasonNumberTooLarge: a kept field holds a number beyond ±2^53, which
	// the canonical JSON of the digest cannot write exactly. Like any other
	// content problem it makes the read unusable as a whole: the snapshot
	// holds no objects, only this problem with the object in Origin, so no
	// read is ever published with an object missing.
	ReasonNumberTooLarge ReasonCode = "number-too-large"
	// ReasonSnapshotLimit: a read holds more objects than the snapshot
	// object limit, or its snapshot would not fit the message size limit
	// GetSnapshot shares with the server. The collector publishes it with no
	// objects and this problem, never cut short.
	ReasonSnapshotLimit ReasonCode = "snapshot-limit"
	// ReasonAnalysisLimit: one of the Limits was reached, and what its field
	// names is cut off. The Subject is the limit's JSON name; the Message
	// gives the limit and the value the input needed, written "at least N"
	// where that value cannot be known without finishing the work, as for
	// Evaluations.
	ReasonAnalysisLimit ReasonCode = "analysis-limit"
	// ReasonAnalysisTimeout: the analysis did not finish within its time
	// budget, so it is AnalysisFailed. The Subject is the environment.
	ReasonAnalysisTimeout ReasonCode = "analysis-timeout"
)

// Warning-level reasons.
const (
	// ReasonSourceStale: the latest read attempt failed; the analysis uses
	// the read that finished at the source's LastCompleteRead.
	ReasonSourceStale ReasonCode = "source-stale"
	// ReasonClusterReadStale: the last complete cluster read is older than
	// the configured limit, so its labels are not used: a namespace no git
	// source declares takes ClusterProfile.NamespaceLabels, or its labels
	// are unknown (ReasonNamespaceLabelsUnknown).
	ReasonClusterReadStale ReasonCode = "cluster-read-stale"
	// ReasonContractVersionUnknown: a snapshot of a contract version this
	// build does not know was ignored and the previous read kept.
	ReasonContractVersionUnknown ReasonCode = "contract-version-unknown"
	// ReasonKindUnrecognized: a Kubernetes kind in none of KeptFields,
	// UnsupportedKinds and IgnoredKinds, without a pod template (see
	// ReasonWorkloadKindUnsupported).
	ReasonKindUnrecognized ReasonCode = "kind-unrecognized"
	// ReasonDuplicateResource: two sources produce the same ObjectKey with
	// equal content.
	ReasonDuplicateResource ReasonCode = "duplicate-resource"
	// ReasonNamespaceMismatch: an object uses a namespace other than its
	// registration's destination; it is kept in the one it uses.
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
	// ReasonSymlinkSkipped: a symbolic link outside the registered path
	// that does not point to a regular file inside the repository was not
	// sent to the renderer; a render that needs it fails on its own.
	ReasonSymlinkSkipped ReasonCode = "symlink-skipped"
	// ReasonProfileConflict: an admin-entered cluster profile value differs
	// from the one read from AWS; the AWS value wins.
	ReasonProfileConflict ReasonCode = "profile-conflict"
	// ReasonExpectationsInvalid: an application's expectations file, or one
	// entry in it, does not validate (see ExpectationsFile); what was
	// rejected is not checked. The Subject is the file's path for the whole
	// file and the entry's ObjectKey for one entry.
	ReasonExpectationsInvalid ReasonCode = "expectations-invalid"
	// ReasonRegistrationNamespaceMissing: the cluster read has no namespace
	// of a registration's destination, so the application may be deployed
	// elsewhere than the analysis assumes.
	ReasonRegistrationNamespaceMissing ReasonCode = "registration-namespace-missing"
	// ReasonDigestMismatch: a fetched snapshot's Digest is not
	// ComputeDigest of its content; it was rejected and the previous read
	// kept.
	ReasonDigestMismatch ReasonCode = "digest-mismatch"
	// ReasonCodeUnknown: a problem carries a reason code this build does not
	// know, which counts as LevelInput (see LevelOf). Message names the
	// code.
	ReasonCodeUnknown ReasonCode = "reason-code-unknown"
	// ReasonBeforeOutdated: Before was made with another SchemaVersion or
	// AlgorithmVersion, or rests on another aws read than the pull request
	// it is compared with, and could not be analyzed again, so the analysis
	// reports no Changes (see Bundle.Before). The Subject is Before's ID.
	ReasonBeforeOutdated ReasonCode = "before-outdated"
	// ReasonServicePortUnresolved: an expectation names a port of an
	// existing Service that cannot be translated: the Service has no such
	// port, or its target port is a name that a backend workload does not
	// declare. The expectation is Unverifiable, never satisfied. The Subject
	// is the expectation's ID. A Service that does not exist is not this
	// reason: its form picks nothing (see EndpointSelector).
	ReasonServicePortUnresolved ReasonCode = "service-port-unresolved"
	// ReasonRuleOutsideModel: a rule was dropped whole because every peer
	// or every port it had is outside the model (IPv6, SCTP, ICMP).
	// Dropping it is exact for IPv4 TCP and UDP, which it never matched.
	// The Subject is the object's ObjectKey and Origin.Locator the rule.
	ReasonRuleOutsideModel ReasonCode = "rule-outside-model"
)

// Reasons returns the reason table: every declared code with its level,
// producers and scope. Every declared code has a row; a code missing here
// is a bug. It returns a deep copy.
func Reasons() map[ReasonCode]ReasonSpec {
	out := make(map[ReasonCode]ReasonSpec, len(reasons))
	for c, s := range reasons {
		s.RaisedBy = slices.Clone(s.RaisedBy)
		out[c] = s
	}
	return out
}

// LevelOf returns code's level. A code this build does not know, from a
// newer collector, counts as LevelInput over the scope of the source that
// carried it, and inputs reports ReasonCodeUnknown beside it: an unknown
// cause is never read as harmless.
func LevelOf(code ReasonCode) ReasonLevel {
	if s, ok := reasons[code]; ok {
		return s.Level
	}
	return LevelInput
}

// ReasonLevels returns every reason code with its level, as Reasons has it.
func ReasonLevels() map[ReasonCode]ReasonLevel {
	out := make(map[ReasonCode]ReasonLevel, len(reasons))
	for c, s := range reasons {
		out[c] = s.Level
	}
	return out
}

func row(l ReasonLevel, s ReasonScope, by ...Producer) ReasonSpec {
	return ReasonSpec{Level: l, RaisedBy: by, Scope: s}
}

var reasons = map[ReasonCode]ReasonSpec{
	ReasonPolicyEnforcementUnknown: row(LevelGate, ScopeStep, ProducerCore),
	ReasonHostNetwork:              row(LevelGate, ScopeStep, ProducerAdapter),
	ReasonStandalonePod:            row(LevelGate, ScopeEndpoint, ProducerAdapter),
	ReasonPolicyKindUnsupported:    row(LevelGate, ScopeStep, ProducerAdapter),
	ReasonNamespaceLabelsUnknown:   row(LevelGate, ScopeStep, ProducerCore),
	ReasonNamespaceLabelsConflict:  row(LevelGate, ScopeStep, ProducerCore),
	ReasonServicePortMismatch:      row(LevelGate, ScopeStep, ProducerCore),
	ReasonRouteOutOfScope:          row(LevelGate, ScopeStep, ProducerCore),
	ReasonRouteMiddlebox:           row(LevelGate, ScopeStep, ProducerCore),
	ReasonPrefixListUnresolved:     row(LevelGate, ScopeStep, ProducerCore),
	ReasonSecurityGroupUnresolved:  row(LevelGate, ScopeStep, ProducerCore),
	ReasonEphemeralPortsPartial:    row(LevelGate, ScopeStep, ProducerCore),
	ReasonPodSecurityGroups:        row(LevelGate, ScopeStep, ProducerAdapter, ProducerCore),
	ReasonNodeSecurityGroupsMixed:  row(LevelGate, ScopeStep, ProducerAdapter, ProducerCore),
	ReasonCustomNetworking:         row(LevelGate, ScopeStep, ProducerAdapter, ProducerCore),
	ReasonPlacementUnknown:         row(LevelGate, ScopeEndpoint, ProducerCore),
	ReasonNoPlacement:              row(LevelGate, ScopeEndpoint, ProducerCore),
	ReasonAWSReadStale:             row(LevelGate, ScopeStep, ProducerInputs),
	ReasonPodAddressesUnknown:      row(LevelGate, ScopeStep, ProducerCore),
	ReasonIPBlockPodMatchUnknown:   row(LevelGate, ScopeStep, ProducerCore),
	ReasonSNATUnknown:              row(LevelGate, ScopeStep, ProducerCore),

	ReasonAWSControllerEffect:     row(LevelPartial, ScopeStep, ProducerAdapter),
	ReasonWorkloadKindUnsupported: row(LevelPartial, ScopeNamespace, ProducerAdapter),

	ReasonRenderFailed:               row(LevelInput, ScopeSource, ProducerRenderer),
	ReasonRenderTimeout:              row(LevelInput, ScopeSource, ProducerRenderer),
	ReasonRenderLimit:                row(LevelInput, ScopeSource, ProducerRenderer, ProducerGitCollector),
	ReasonRemoteResource:             row(LevelInput, ScopeSource, ProducerRenderer),
	ReasonChartDependencyUnavailable: row(LevelInput, ScopeSource, ProducerGitCollector),
	ReasonSourcePathMissing:          row(LevelInput, ScopeSource, ProducerGitCollector, ProducerRenderer),
	ReasonSymlinkUnresolved:          row(LevelInput, ScopeSource, ProducerGitCollector),
	ReasonAccessDenied:               row(LevelInput, ScopeSource, ProducerGitCollector, ProducerCloudCollector),
	ReasonSourceUnread:               row(LevelInput, ScopeSource, ProducerInputs),
	ReasonPullRequestNotMergeable:    row(LevelInput, ScopeSource, ProducerGitCollector),
	ReasonConflictingResource:        row(LevelInput, ScopeSource, ProducerCore),
	ReasonIPFamilyUnsupported:        row(LevelInput, ScopeSource, ProducerAdapter),
	ReasonObjectInvalid:              row(LevelInput, ScopeNamespace, ProducerAdapter),
	ReasonNumberTooLarge:             row(LevelInput, ScopeSource, ProducerRenderer, ProducerCloudCollector),
	ReasonSnapshotLimit:              row(LevelInput, ScopeSource, ProducerGitCollector, ProducerCloudCollector),
	ReasonAnalysisLimit:              row(LevelInput, ScopePaths, ProducerCore),
	ReasonAnalysisTimeout:            row(LevelInput, ScopeAnalysis, ProducerInputs),

	ReasonSourceStale:                  row(LevelWarning, ScopeNone, ProducerInputs),
	ReasonClusterReadStale:             row(LevelWarning, ScopeNone, ProducerInputs),
	ReasonContractVersionUnknown:       row(LevelWarning, ScopeNone, ProducerInputs),
	ReasonKindUnrecognized:             row(LevelWarning, ScopeNone, ProducerAdapter),
	ReasonDuplicateResource:            row(LevelWarning, ScopeNone, ProducerCore),
	ReasonNamespaceMismatch:            row(LevelWarning, ScopeNone, ProducerRenderer),
	ReasonRegistrationOverlap:          row(LevelWarning, ScopeNone, ProducerInputs),
	ReasonUnowned:                      row(LevelWarning, ScopeNone, ProducerScope),
	ReasonOwnershipConflict:            row(LevelWarning, ScopeNone, ProducerScope),
	ReasonNamedPortUnresolved:          row(LevelWarning, ScopeNone, ProducerCore),
	ReasonHostPort:                     row(LevelWarning, ScopeNone, ProducerAdapter),
	ReasonSymlinkSkipped:               row(LevelWarning, ScopeNone, ProducerGitCollector),
	ReasonProfileConflict:              row(LevelWarning, ScopeNone, ProducerCore),
	ReasonExpectationsInvalid:          row(LevelWarning, ScopeNone, ProducerRenderer, ProducerAdapter),
	ReasonRuleOutsideModel:             row(LevelWarning, ScopeNone, ProducerAdapter),
	ReasonServicePortUnresolved:        row(LevelWarning, ScopeExpectation, ProducerCore),
	ReasonRegistrationNamespaceMissing: row(LevelWarning, ScopeNone, ProducerCore),
	ReasonDigestMismatch:               row(LevelWarning, ScopeNone, ProducerInputs),
	ReasonCodeUnknown:                  row(LevelWarning, ScopeNone, ProducerInputs),
	ReasonBeforeOutdated:               row(LevelWarning, ScopeNone, ProducerInputs),
}
