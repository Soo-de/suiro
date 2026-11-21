package model

import (
	"encoding/json"
	"time"
)

// ContractVersion is the snapshot contract this build writes and accepts.
// A collector stamps it on every snapshot. A server that meets a version it
// does not know ignores that snapshot and keeps the previous complete read
// (ReasonContractVersionUnknown), so services mid-upgrade give stale results
// for a few minutes, never wrong ones.
const ContractVersion = 1

// Digest is a SHA-256 content hash written "sha256:" followed by 64
// lowercase hex digits.
type Digest string

// Snapshot is one complete read of a ReadKey as a collector publishes it.
//
// Only complete reads become snapshots. A read that fails part way
// (throttling, network) is not published; the previous snapshot stays and
// the source reports SourceReadError. A read that completes but finds the
// content unusable (a render error, an unmergeable pull request, a missing
// path) is published with no Objects and an input-level Problem, because
// that is the true state of the revision and must not look clean.
type Snapshot struct {
	Key ReadKey `json:"key"`
	// PullRequest is set when Key.PullRequest is not 0.
	PullRequest *PullRequest `json:"pullRequest,omitempty"`
	// Revision is the commit read; empty for AWS.
	Revision        string `json:"revision,omitempty"`
	ContractVersion int    `json:"contractVersion"`
	// Digest covers Objects and Problems, not Revision or read times, so a
	// commit that leaves the rendered objects unchanged keeps the digest and
	// triggers no new analysis. It is the SHA-256 of the JSON encoding of
	// {"objects": Objects sorted by Key, "problems": Problems sorted by
	// Reason then Subject}, with every Body compacted and its object keys
	// sorted.
	Digest Digest `json:"digest"`
	// ReadStartedAt and ReadFinishedAt bound the read. An AWS read is not
	// atomic: objects changed during the read may be mixed, and the next
	// read corrects them.
	ReadStartedAt  time.Time `json:"readStartedAt"`
	ReadFinishedAt time.Time `json:"readFinishedAt"`
	// Objects are sorted by Key and unique by Key.
	Objects  []Object  `json:"objects"`
	Problems []Problem `json:"problems,omitempty"`
}

// ObjectKind names a kind of source object as "<source>:<group>/<Kind>":
// "k8s:apps/Deployment", "k8s:core/Service" (core is Kubernetes' empty API
// group), "aws:ec2/SecurityGroup", "aws:eks/Nodegroup", and
// "suiro:core/Expectation" for Suiro's own expectations file. The API
// version is not part of it; v1 is assumed.
type ObjectKind string

// ObjectKey identifies one object across sources and reads:
// "<ObjectKind>/<namespace>/<name>" for namespaced Kubernetes objects,
// "<ObjectKind>/<name>" for cluster-scoped ones and for AWS objects, whose
// name is the resource ID ("aws:ec2/SecurityGroup/sg-0123"), and
// "<ObjectKind>/<Expectation.ID>" for an expectation entry. Two sources
// producing the same key is a duplicate (ReasonDuplicateResource when the
// bodies are equal, ReasonConflictingResource when they differ).
type ObjectKey string

// NewObjectKey returns the key of the object of kind named name: in
// namespace for a namespaced Kubernetes object, and with namespace "" for
// any other. Keys are built only here.
func NewObjectKey(kind ObjectKind, namespace, name string) ObjectKey {
	panic("model: NewObjectKey is not implemented")
}

// Parse splits k into the parts NewObjectKey takes. The kind runs to the
// second "/"; for KindExpectation the rest is the name, since an
// Expectation.ID may contain "/", and for any other kind the rest is a name
// or a namespace and a name. It errors on any other shape.
func (k ObjectKey) Parse() (kind ObjectKind, namespace, name string, err error) {
	panic("model: ObjectKey.Parse is not implemented")
}

// Object is one source object reduced to its kept fields.
type Object struct {
	Kind   ObjectKind `json:"kind"`
	Key    ObjectKey  `json:"key"`
	Origin Origin     `json:"origin"`
	// Body is the object in its source's own JSON shape (a Kubernetes
	// manifest; an AWS API output type with its PascalCase field names; one
	// ExpectationSpec entry), holding only the paths KeptFields lists for
	// Kind plus, for Kubernetes, KubeIdentityFields. Anything else, Secret
	// data and environment variables included, is removed before the object
	// leaves the renderer or the cloud-collector.
	Body json.RawMessage `json:"body"`
}

// Origin says where an object or one of its rules came from, so every
// verdict can name the rule that decided it.
type Origin struct {
	Source SourceID `json:"source"`
	// File is the repository-relative path of the file the object came from,
	// as far as the renderer can tell: the file itself for plain YAML, the
	// template named by Helm's "# Source:" comment, the origin annotation
	// for Kustomize when present and otherwise the kustomization directory;
	// the expectations file for an expectation entry.
	File string `json:"file,omitempty"`
	// Line is the 1-based line in the rendered output of File where the
	// object (or the rule, with Locator) starts. For templates it is the
	// line of the output, not of the template; for an expectation entry,
	// the line in the expectations file where the entry starts.
	Line int `json:"line,omitempty"`
	// Resource is the AWS resource ID for objects read from AWS.
	Resource string `json:"resource,omitempty"`
	// Locator points inside the object at one rule: "spec.ingress[0].from[1]"
	// for a NetworkPolicy peer, "IpPermissions[2]" for a Security Group
	// rule, "egress/100" for a Network ACL entry, "*" for its default rule,
	// "Routes[3]" for a route.
	Locator string `json:"locator,omitempty"`
}

// Problem is a reasoned warning or Unknown cause. Nothing unusual is dropped
// silently: every skipped object, failed render, unresolved reference or
// ignored snapshot ends as a Problem whose Reason says what happened and
// what it does to verdicts (see ReasonLevels).
type Problem struct {
	Reason ReasonCode `json:"reason"`
	// Subject is what the problem is about: an ObjectKey, SourceID,
	// EndpointKey, GateID or file path.
	Subject string `json:"subject"`
	// Message is detail for people. It never quotes a field outside
	// KeptFields, so it cannot carry a Secret value into a log. The text is
	// deterministic (repository-relative paths, no times, no temporary
	// directories), so the same failure yields the same snapshot digest; a
	// fixed source still changes the digest through its objects.
	Message string  `json:"message,omitempty"`
	Origin  *Origin `json:"origin,omitempty"`
}
