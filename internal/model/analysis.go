package model

import "time"

// AnalysisID is a stored analysis's number; 0 before it is stored.
type AnalysisID int64

// AnalysisKind is what an analysis is for.
type AnalysisKind string

const (
	// AnalysisBaseline is the approved state: every source's target branch
	// plus the last complete AWS read. Its map is what the graph shows.
	AnalysisBaseline AnalysisKind = "baseline"
	// AnalysisPullRequest previews one pull request: the baseline with the
	// pull request's repository read at its test merge commit.
	AnalysisPullRequest AnalysisKind = "pr"
)

// AnalysisStatus is a stored analysis's state; the values match the
// database's.
type AnalysisStatus string

const (
	// AnalysisRunning is set by store while an analysis is computed.
	AnalysisRunning AnalysisStatus = "running"
	// AnalysisDone means every input was usable.
	AnalysisDone AnalysisStatus = "done"
	// AnalysisPartial means some input-level or partial-level reason holds;
	// the affected paths say which.
	AnalysisPartial AnalysisStatus = "partial"
	// AnalysisNoPreview is set by inputs for a pull request that changes
	// only what no source previews (a Terraform-only change without the CLI
	// step). It is never reported as clean.
	AnalysisNoPreview AnalysisStatus = "no_preview"
	// AnalysisFailed means Analyze returned an error.
	AnalysisFailed AnalysisStatus = "failed"
)

// Bundle is everything one analysis rests on, assembled by inputs. Core
// reads nothing else.
type Bundle struct {
	Kind        AnalysisKind `json:"kind"`
	Environment string       `json:"environment"`
	// PullRequest is set for AnalysisPullRequest. Every source registered on
	// its repository is read at its test merge commit; other sources at
	// their target branch.
	PullRequest *PullRequest `json:"pullRequest,omitempty"`
	// Sources are every registered source of the environment, including
	// those without a usable snapshot, so an input-level problem can be
	// scoped to the source's namespace.
	Sources []Source `json:"sources"`
	// Snapshots are the after side: at most one per source.
	Snapshots []Snapshot `json:"snapshots"`
	// Problems are what inputs found before analysis, each with a SourceID
	// as Subject: no complete read yet (ReasonSourceUnread), a read older
	// than the last attempt (ReasonSourceStale), an ignored snapshot
	// (ReasonContractVersionUnknown).
	Problems []Problem `json:"problems,omitempty"`
	// Profile holds the admin-entered cluster profile; values the aws
	// snapshot reveals take precedence.
	Profile ClusterProfile `json:"profile"`
	// Rules are the forbidden paths and hygiene checks in force.
	Rules RuleSet `json:"rules"`
	// Before is the before side: for a pull request the baseline in force,
	// for a baseline the previous one; nil on first install, when every path
	// counts as new. Core re-checks Rules against Before.Paths, so a rule
	// added since Before reports what was already violated as existing,
	// not new.
	Before *Analysis `json:"before,omitempty"`
	// Fingerprint is the SHA-256 over the sorted (source, digest) pairs of
	// Snapshots, Before's fingerprint, Profile, Rules.Version and
	// ToolVersion. inputs skips the analysis when one with this fingerprint
	// is stored.
	Fingerprint Digest `json:"fingerprint"`
	ToolVersion string `json:"toolVersion"`
}

// SnapshotRef records one snapshot an analysis used.
type SnapshotRef struct {
	Key      ReadKey `json:"key"`
	Revision string  `json:"revision,omitempty"`
	Digest   Digest  `json:"digest"`
}

// Analysis is the result of one Bundle: the map (endpoints and paths), the
// difference from the before side and the findings.
type Analysis struct {
	ID AnalysisID `json:"id,omitempty"`
	// Base is Before's ID; 0 when there was no before side.
	Base           AnalysisID     `json:"base,omitempty"`
	Kind           AnalysisKind   `json:"kind"`
	Environment    string         `json:"environment"`
	PullRequest    *PullRequest   `json:"pullRequest,omitempty"`
	Status         AnalysisStatus `json:"status"`
	Fingerprint    Digest         `json:"fingerprint"`
	ToolVersion    string         `json:"toolVersion"`
	RuleSetVersion int64          `json:"ruleSetVersion"`
	CreatedAt      time.Time      `json:"createdAt,omitzero"`
	Inputs         []SnapshotRef  `json:"inputs"`
	// Profile is the merged cluster profile the verdicts assumed, with the
	// source of each value.
	Profile    ClusterProfile `json:"profile"`
	Endpoints  []Endpoint     `json:"endpoints"`
	Paths      []Path         `json:"paths"`
	Changes    []Change       `json:"changes,omitempty"`
	Findings   []Finding      `json:"findings,omitempty"`
	Problems   []Problem      `json:"problems,omitempty"`
	Unexamined []Unexamined   `json:"unexamined"`
}

// Change is one path whose verdict or set of passing class pairs differs
// between the before and after sides. Opened and Closed compare at class
// level: a class pair that passes after and did not before is opened (Δ+);
// one that passed before and does not after is closed (Δ−). A change can
// have both. Ports are cut where either side's paths are, so both sides
// are compared over the same port ranges.
type Change struct {
	Key PathKey `json:"key"`
	// Before or After is empty when that side has no such path (an endpoint
	// added or removed).
	Before Verdict     `json:"before,omitempty"`
	After  Verdict     `json:"after,omitempty"`
	Opened []ClassPair `json:"opened,omitempty"`
	Closed []ClassPair `json:"closed,omitempty"`
}

// ExpectationKind is what an expectation demands.
type ExpectationKind string

const (
	// ExpectRequired paths must work. Application owners declare them next
	// to their manifests.
	ExpectRequired ExpectationKind = "required"
	// ExpectForbidden paths must never open. The security team keeps them in
	// the service, versioned.
	ExpectForbidden ExpectationKind = "forbidden"
)

// Quantifier says how many members of a selector an expectation is about.
type Quantifier string

const (
	QuantifyAll Quantifier = "all"
	QuantifyAny Quantifier = "any"
)

// EndpointSelector picks endpoints: the union of every listed form. A
// selector that picks nothing is an error (FindingUnmatched), never a
// silently satisfied expectation.
type EndpointSelector struct {
	// All picks every endpoint.
	All bool `json:"all,omitempty"`
	// Internet picks EndpointInternet.
	Internet  bool          `json:"internet,omitempty"`
	Endpoints []EndpointKey `json:"endpoints,omitempty"`
	// Namespaces with Labels pick the workloads in those namespaces whose
	// pod labels match; nil Labels picks every workload in them.
	Namespaces []string       `json:"namespaces,omitempty"`
	Labels     *LabelSelector `json:"labels,omitempty"`
}

// Expectation is a required or forbidden path set.
//
// A required expectation holds when, with the source quantifier outside,
// Q_s s ∈ Sources, Q_d d ∈ Destinations, every port of Ports is Allow.
// A forbidden one is violated when some source reaches some destination on
// some port of Ports in some placement: it takes no quantifiers.
type Expectation struct {
	// ID is stable: the rule key for forbidden ("builtin.internet-db-ports"),
	// "<file>#<name>" for required.
	ID           string           `json:"id"`
	Kind         ExpectationKind  `json:"kind"`
	Name         string           `json:"name"`
	Sources      EndpointSelector `json:"sources"`
	Destinations EndpointSelector `json:"destinations"`
	Ports        []PortRange      `json:"ports"`
	// SourceQuantifier and DestinationQuantifier are required for
	// ExpectRequired and unused for ExpectForbidden.
	SourceQuantifier      Quantifier `json:"sourceQuantifier,omitempty"`
	DestinationQuantifier Quantifier `json:"destinationQuantifier,omitempty"`
	Origin                Origin     `json:"origin"`
}

// HygieneCheck is a built-in check on objects rather than paths.
type HygieneCheck string

const (
	CheckNamespaceWithoutPolicy HygieneCheck = "namespace-without-policy"
	CheckHostNetworkPod         HygieneCheck = "host-network-pod"
	// CheckShadowedRule flags a Network ACL rule that earlier rules cover
	// completely, so it never decides anything. It is a warning, not a
	// block.
	CheckShadowedRule HygieneCheck = "shadowed-acl-rule"
)

// HygieneRule is one enabled hygiene check.
type HygieneRule struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Check HygieneCheck `json:"check"`
}

// RuleSet is the forbidden paths and hygiene checks in force at one version.
// Built-in checks are seeded on install and enabled; an admin can disable
// or edit them.
type RuleSet struct {
	Version   int64         `json:"version"`
	Forbidden []Expectation `json:"forbidden,omitempty"`
	Hygiene   []HygieneRule `json:"hygiene,omitempty"`
}

// ExpectationStatus is an expectation's three-valued state. It is computed
// from class outcomes with Kleene quantifiers, not from the Verdict, so a
// certain answer survives Undetermined classes elsewhere:
//
//	required:  ∀ class pairs Pass  → satisfied; ∃ Block → violated; else unverifiable
//	forbidden: ∃ class pair Pass   → violated;  ∀ Block → satisfied; else unverifiable
type ExpectationStatus string

const (
	Satisfied    ExpectationStatus = "satisfied"
	Violated     ExpectationStatus = "violated"
	Unverifiable ExpectationStatus = "unverifiable"
)

// FindingKind is what a finding is about; the values match the database's.
type FindingKind string

const (
	FindingForbidden FindingKind = "forbidden"
	FindingRequired  FindingKind = "required"
	FindingHygiene   FindingKind = "hygiene"
	// FindingUnmatched is an expectation whose selectors pick nothing.
	FindingUnmatched FindingKind = "unmatched"
	FindingShadowed  FindingKind = "shadowed"
)

// Finding is an expectation or check that is not satisfied. Exceptions are
// not applied here: they are matched when results are read, so an expired
// exception warns again without a new analysis.
type Finding struct {
	Kind FindingKind `json:"kind"`
	// Rule is the expectation or hygiene rule ID.
	Rule string `json:"rule"`
	// Subject is the PathKey string, ObjectKey or expectation Origin the
	// finding is about; exceptions match it.
	Subject string            `json:"subject"`
	Status  ExpectationStatus `json:"status"`
	// New means the status got worse than on the before side (satisfied →
	// unverifiable → violated). Only new findings warn on a pull request;
	// the rest are existing problems listed on the open issues page.
	New     bool         `json:"new"`
	Paths   []PathKey    `json:"paths,omitempty"`
	Reasons []ReasonCode `json:"reasons,omitempty"`
	// Origin is the expectation's file and line.
	Origin Origin `json:"origin"`
	// Owner is the expectation's owner, the one to contact when another
	// team's change breaks it; filled by api.ScopeFilter.
	Owner Owner `json:"owner,omitzero"`
}
