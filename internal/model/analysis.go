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
// database's. An analysis is stored only once it has finished, so no stored
// analysis is ever running.
type AnalysisStatus string

const (
	// AnalysisDone means every input was usable.
	AnalysisDone AnalysisStatus = "done"
	// AnalysisPartial means some input-level or partial-level reason holds;
	// the affected paths say which.
	AnalysisPartial AnalysisStatus = "partial"
	// AnalysisNoPreview is set by inputs for a pull request that changes
	// only what no source previews (a Terraform-only change without the CLI
	// step). It is never reported as clean.
	AnalysisNoPreview AnalysisStatus = "no_preview"
	// AnalysisFailed means Analyze returned an error. A failed analysis is
	// stored and shown, but never reused: inputs analyses the same Bundle
	// again rather than skip it.
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
	// scoped to the source's namespace. Sorted by ID.
	Sources []Source `json:"sources"`
	// Snapshots are the after side: at most one per source, sorted by Key.
	Snapshots []Snapshot `json:"snapshots"`
	// Problems are what inputs found before analysis, each with a SourceID
	// as Subject: no complete read yet (ReasonSourceUnread), a read older
	// than the last attempt (ReasonSourceStale), an ignored snapshot
	// (ReasonContractVersionUnknown). Sorted by Reason, then Subject.
	Problems []Problem `json:"problems,omitempty"`
	// Profile is the admin-entered cluster profile of Environment, as store
	// keeps it; values the aws snapshot reveals take precedence.
	Profile ClusterProfile `json:"profile"`
	// Rules are the forbidden paths and hygiene checks in force.
	Rules RuleSet `json:"rules"`
	// Before is the before side: for a pull request the baseline in force,
	// for a baseline the previous one; nil on first install, when every path
	// counts as new. Core re-checks Rules against Before.Paths, so a rule
	// added since Before reports what was already violated as existing,
	// not new.
	Before *Analysis `json:"before,omitempty"`
	// Fingerprint identifies what the Bundle yields. inputs skips the
	// analysis when the latest analysis of the same baseline or pull request
	// has this fingerprint and a finished status (see package inputs). It is
	// the SHA-256 of the JSON encoding of every other field of the Bundle,
	// lists in the order stated above, with these reductions:
	//   - PullRequest keeps only RepoURL and Number, so a new commit that
	//     changes no input reuses the stored analysis;
	//   - each snapshot keeps only its Key and Digest, which covers its
	//     objects and problems;
	//   - each problem keeps only Reason and Subject, so detail text that
	//     differs between attempts forces no new analysis;
	//   - Rules keeps only Version, which names its content;
	//   - for a pull request, Before keeps only its Fingerprint, empty when
	//     Before is nil; a baseline leaves Before out, because its Before is
	//     the latest baseline, the very analysis the skip compares it with.
	//
	// So two Bundles with the same fingerprint give the same endpoints,
	// paths and findings, and for a pull request the same Changes too; a
	// baseline's Base, Changes and the New mark of its findings follow its
	// own Before. Kind, Environment and the pull request's identity are part
	// of it, so a baseline never shares a fingerprint with a pull request
	// analysis, nor one pull request with another, however alike their
	// inputs. A field added to Bundle is added to the fingerprint.
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
	Base        AnalysisID     `json:"base,omitempty"`
	Kind        AnalysisKind   `json:"kind"`
	Environment string         `json:"environment"`
	PullRequest *PullRequest   `json:"pullRequest,omitempty"`
	Status      AnalysisStatus `json:"status"`
	// Fingerprint is the Bundle's, so it also names the kind, environment
	// and pull request.
	Fingerprint    Digest        `json:"fingerprint"`
	ToolVersion    string        `json:"toolVersion"`
	RuleSetVersion int64         `json:"ruleSetVersion"`
	CreatedAt      time.Time     `json:"createdAt,omitzero"`
	Inputs         []SnapshotRef `json:"inputs"`
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
	// ExpectRequired paths must work. Application owners declare them in
	// their repository's ExpectationsFile, next to their manifests.
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

// Expectation is a required or forbidden path set: the paths from Sources
// to Destinations over Ports. Its status follows the one rule in
// ExpectationStatus. A required expectation takes both quantifiers, the
// source quantifier outside; a forbidden one takes none and asks whether
// any source reaches any destination on any port in any placement.
type Expectation struct {
	// ID is stable and unique across sources: the rule key for forbidden
	// ("builtin.internet-db-ports"), "<SourceID>#<name>" for required
	// ("payments/payments-api#api-reaches-db").
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
	// block. Like every hygiene check it yields FindingHygiene findings, one
	// per shadowed rule, whose Subject is the Network ACL's ObjectKey and
	// whose Origin points at the rule.
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
// from class outcomes, never from a path's Verdict, by one rule.
//
// Let S and D be the endpoints the selectors pick. For s ∈ S and d ∈ D,
// t(s, d) combines the class outcomes of every path from s to d whose port
// range overlaps Ports. A path that an input-level reason covers adds
// Undetermined for each of its classes, whatever its gates say, because
// those gates cannot be trusted (the same reason makes its Verdict
// Unknown). With Kleene AND for ∀ and Kleene OR for ∃:
//
//	required:  t(s, d) = AND of the outcomes,  T = Q_s s ∈ S  Q_d d ∈ D : t(s, d)
//	           T Pass → satisfied, T Block → violated, else unverifiable
//	forbidden: t(s, d) = OR of the outcomes,   T = OR over s ∈ S, d ∈ D of t(s, d)
//	           T Pass → violated, T Block → satisfied, else unverifiable
//
// So a required expectation is satisfied exactly when, under its
// quantifiers, every path it covers is Allow; a forbidden one is violated
// by any path it covers that is Allow or PlacementDependent, or Unknown
// with a passing class and no input-level reason. A certain answer
// survives Undetermined classes elsewhere: one blocking class makes a
// required t(s, d) Block, one passing class makes a forbidden one Pass. A
// path that an input-level reason covers can neither satisfy nor violate
// an expectation; alone, it leaves it unverifiable. When S or D is empty
// nothing is evaluated: the expectation yields a FindingUnmatched whose
// Status is Unverifiable, never Satisfied.
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
	// FindingHygiene is a finding of any HygieneCheck, shadowed Network ACL
	// rules included; its Rule names the HygieneRule.
	FindingHygiene FindingKind = "hygiene"
	// FindingUnmatched is an expectation whose selectors pick nothing.
	FindingUnmatched FindingKind = "unmatched"
)

// Finding is an expectation or check that is not satisfied. Exceptions are
// not applied here: they are matched when results are read, so an expired
// exception warns again without a new analysis.
//
// A forbidden expectation yields one finding per path it covers that is not
// satisfied, so a pull request that opens a new forbidden path warns even
// when the same rule is already violated elsewhere, and an exception for one
// path matches that path's finding alone. A required expectation, and an
// expectation whose selectors pick nothing, yields one finding for the
// whole expectation.
type Finding struct {
	Kind FindingKind `json:"kind"`
	// Rule is the expectation or hygiene rule ID.
	Rule string `json:"rule"`
	// Subject is what the finding is about; exceptions match it: the
	// covered path's PathKey string for a forbidden expectation, the
	// expectation ID for a required or unmatched one, and the ObjectKey for
	// a hygiene check.
	Subject string `json:"subject"`
	// Status of a forbidden expectation's finding is that one path's
	// status: the ExpectationStatus rule applied to the path alone.
	Status ExpectationStatus `json:"status"`
	// New means the status got worse than on the before side (satisfied →
	// unverifiable → violated). For a forbidden expectation it is judged
	// over ports cut where either side's paths are, as Changes cuts them:
	// the finding is New when some piece of its path's port range within the
	// expectation's Ports had a better status on the before side. Only new
	// findings warn on a pull request; the rest are existing problems listed
	// on the open issues page.
	New     bool         `json:"new"`
	Paths   []PathKey    `json:"paths,omitempty"`
	Reasons []ReasonCode `json:"reasons,omitempty"`
	// Origin is where the expectation is declared or, for a hygiene
	// finding, the object or rule it is about.
	Origin Origin `json:"origin"`
	// Owner is the expectation's owner, or for a hygiene finding the
	// object's: the one to contact when another team's change breaks it;
	// filled by api.ScopeFilter.
	Owner Owner `json:"owner,omitzero"`
}
