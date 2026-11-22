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
	// AnalysisPartial means some input-level or partial-level reason holds,
	// a reached limit (ReasonAnalysisLimit) included; the affected paths say
	// which.
	AnalysisPartial AnalysisStatus = "partial"
	// AnalysisNoPreview is set by inputs for a pull request that changes
	// only what no source previews (a Terraform-only change without the CLI
	// step). It is never reported as clean.
	AnalysisNoPreview AnalysisStatus = "no_preview"
	// AnalysisFailed means Analyze returned an error, running past the time
	// budget included (ReasonAnalysisTimeout in Problems). A failed
	// analysis holds no endpoints or paths and is never reported as clean;
	// it is stored and shown, but never reused: inputs analyzes the same
	// Bundle again rather than skip it.
	AnalysisFailed AnalysisStatus = "failed"
)

// Bundle is everything one analysis rests on, assembled by inputs. Core
// reads nothing else.
type Bundle struct {
	Kind        AnalysisKind `json:"kind"`
	Environment string       `json:"environment"`
	// PullRequest is set for AnalysisPullRequest. Every source registered on
	// its repository is read at its test merge commit, other git sources at
	// their target branch, and the aws source as Snapshots says.
	PullRequest *PullRequest `json:"pullRequest,omitempty"`
	// Sources are every registered source of the environment, including
	// those without a usable snapshot, so an input-level problem can be
	// scoped to the source's namespaces (LevelInput). Sorted by ID.
	Sources []Source `json:"sources"`
	// Snapshots are the after side: at most one per source, sorted by Key.
	// For a pull request the aws snapshot is the read Before rests on, so
	// both sides share one AWS state and Changes show only what the pull
	// request does; inputs moves pull requests to a new aws read only once
	// a baseline on it is in force (see package inputs). Only when the read
	// Before rests on can no longer be used does the Bundle hold another,
	// and Before is then analyzed again (see Before).
	Snapshots []Snapshot `json:"snapshots"`
	// UsedNamespaces holds, for each git source, every namespace its
	// objects used (metadata.namespace) in its latest usable reads, which an
	// input-level problem of the source also covers (LevelInput). A
	// usable read is a complete read whose content rendered, so it carries
	// no input-level problem. A source read at its target branch takes that
	// key's last usable read; a source read at a pull request's test merge
	// commit takes the union of the last usable reads of the pull request
	// key and of the target branch key. Each list is sorted and without
	// repeats.
	UsedNamespaces map[SourceID][]string `json:"usedNamespaces,omitempty"`
	// Problems are what inputs found before analysis: no complete read yet
	// (ReasonSourceUnread), a read older than the last attempt
	// (ReasonSourceStale), an ignored snapshot (ReasonContractVersionUnknown,
	// ReasonDigestMismatch), a reason code this build does not know
	// (ReasonCodeUnknown), a Before it could not analyze again
	// (ReasonBeforeOutdated). The Subject is the SourceID each is about,
	// except for ReasonBeforeOutdated, whose Subject is Before's ID. Sorted
	// by Reason, then Subject.
	Problems []Problem `json:"problems,omitempty"`
	// Profile is the admin-entered cluster profile of Environment, as store
	// keeps it; values the aws snapshot reveals take precedence.
	Profile ClusterProfile `json:"profile"`
	// Rules are the forbidden paths and hygiene checks in force.
	Rules RuleSet `json:"rules"`
	// Before is the before side: for a pull request the baseline in force,
	// for a baseline the previous one; nil on first install, when every path
	// counts as new. Before is never compared when it was made with another
	// SchemaVersion or AlgorithmVersion or, for a pull request, rests on
	// another aws read than Snapshots holds: inputs first analyzes that
	// baseline again, with this build and the Bundle's aws read, and when
	// it cannot it raises ReasonBeforeOutdated, on which core leaves Changes
	// empty and marks New, as with no before side, every finding that is
	// not satisfied. Like every stored analysis it holds path summaries
	// only, which is all the comparison reads (see Change and Finding.New).
	Before *Analysis `json:"before,omitempty"`
	// Fingerprint identifies what the Bundle yields. inputs skips the
	// analysis when the latest analysis of the same baseline or pull request
	// has this fingerprint and a finished status (see package inputs). It is
	// what ComputeFingerprint returns: the SHA-256 of the RFC 8785 canonical
	// JSON of every other field of the Bundle, built from the fields as
	// stated here rather than from the Go struct's encoding, lists in the
	// order stated above and [] when empty, plus the Limits the analyzer
	// runs with, with these reductions:
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
	// inputs. SchemaVersion and AlgorithmVersion are part of it, so a build
	// that changes either never reuses an older result. A field added to
	// Bundle is added to the fingerprint.
	Fingerprint Digest `json:"fingerprint"`
	ToolVersion string `json:"toolVersion"`
	// SchemaVersion and AlgorithmVersion are the build's (see
	// AlgorithmVersion), or in a Bundle rebuilt to explain a stored
	// analysis, that analysis's; core rejects any other (core.ErrVersion).
	// The Analysis repeats them.
	SchemaVersion    int `json:"schemaVersion"`
	AlgorithmVersion int `json:"algorithmVersion"`
}

// ComputeFingerprint returns the fingerprint of b analyzed under l, as
// Bundle.Fingerprint defines it, ignoring b.Fingerprint itself. It errors
// where ComputeDigest does.
func ComputeFingerprint(b Bundle, l Limits) (Digest, error) {
	panic("model: ComputeFingerprint is not implemented")
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
	Fingerprint Digest `json:"fingerprint"`
	ToolVersion string `json:"toolVersion"`
	// SchemaVersion is the version of this stored JSON and AlgorithmVersion
	// that of the analysis that made it; 0 in a record older than both.
	SchemaVersion    int           `json:"schemaVersion"`
	AlgorithmVersion int           `json:"algorithmVersion"`
	RuleSetVersion   int64         `json:"ruleSetVersion"`
	CreatedAt        time.Time     `json:"createdAt,omitzero"`
	Inputs           []SnapshotRef `json:"inputs"`
	// Profile is the merged cluster profile the verdicts assumed, with the
	// source of each value.
	Profile   ClusterProfile `json:"profile"`
	Endpoints []Endpoint     `json:"endpoints"`
	// Paths are computed for every ordered pair of two different endpoints
	// with at least one workload end, except from a link-local network, and
	// from the internet and external private networks to AWS resources,
	// which forbidden rules ask about. No other pair is computed: none
	// between AWS resources, from an AWS resource to a network (the
	// internet, an external private or a link-local one), between networks,
	// from a link-local network, or from an endpoint to itself.
	// Expectations leave those pairs out (see ExpectationStatus), and
	// Unexamined lists them (UnexaminedPairs).
	Paths      []Path       `json:"paths"`
	Changes    []Change     `json:"changes,omitempty"`
	Findings   []Finding    `json:"findings,omitempty"`
	Problems   []Problem    `json:"problems,omitempty"`
	Unexamined []Unexamined `json:"unexamined"`
	// Limits are the limits the analysis ran with; they are part of the
	// fingerprint, so a changed limit never reuses an older result.
	Limits Limits `json:"limits"`
}

// Limits are the counter limits an analyzer runs with, so that no input,
// however large, makes one analysis take the server. Reaching one never
// fails the analysis; it cuts off the pairs its field names, the same ones
// however the work was scheduled. Every endpoint stays listed, and a pair
// cut off gets, in place of its paths, one Unknown path per protocol over
// every port (tcp and udp, 0-65535) with ReasonAnalysisLimit, whose class
// pairs are all Undetermined, SameNode ones included wherever both ends
// are workloads, so no placement is left out. Every limit must be
// positive; the values come from measurement on a reference machine. Time
// is not one of them: the time budget is ctx's deadline, and running out
// fails the analysis, because where time runs out depends on the machine.
type Limits struct {
	// Endpoints is the most endpoints of one analysis. Past it, every pair
	// with an endpoint beyond the first Endpoints in Key order is cut off,
	// and those endpoints keep their classes before prefix cuts.
	Endpoints int `json:"endpoints"`
	// Rules is the most filter rules over every gate of one analysis. Past
	// it every pair is cut off, since any gate may decide any pair.
	Rules int `json:"rules"`
	// PrefixesPerEndpoint is the most rule prefixes that may split one
	// endpoint's placement classes. An endpoint past it keeps its classes
	// before prefix cuts, and every pair with it is cut off.
	PrefixesPerEndpoint int `json:"prefixesPerEndpoint"`
	// PortRangesPerPair is the most port ranges the paths of one pair may
	// cut the port space into; a pair past it is cut off.
	PortRangesPerPair int `json:"portRangesPerPair"`
	// Evaluations is the most chain evaluations of one analysis, each pair
	// × port range × class pair counted once, and once more for each backend
	// of a Service an expectation names. The budget goes to pairs in
	// canonical order, then to expectations in ID order: the pair or
	// expectation it runs out on and every later one are cut off, and an
	// expectation cut off is Unverifiable with ReasonAnalysisLimit.
	Evaluations int `json:"evaluations"`
}

// Validate reports the first broken invariant of a, or nil: every path's
// Outcomes is empty or has one letter per class pair of the path, not all
// the same, and agrees with its Verdict and Reasons as Verdict defines,
// and every list is in canonical order with no two entries tied, identical
// problems aside (see the package comment).
func (a Analysis) Validate() error {
	panic("model: Analysis.Validate is not implemented")
}

// Change is one path whose verdict or class pair outcomes differ between
// the before and after sides, or that has a class pair Undetermined on
// both. Core computes it from the two stored summaries alone
// (Endpoint.Classes and Path.Outcomes), never by analyzing a side again.
// Ports are cut where either side's paths are, so both sides are compared
// over the same port ranges.
//
// Classes are matched across the sides by location, never by
// configuration: the before and after classes of one endpoint align where
// their NodeGroup and Subnet are equal and their Addresses intersect, and
// each aligned part keeps one outcome per side because every class is
// homogeneous (see PlacementClass). ClassCluster aligns with ClassCluster,
// and ClassUnknown with ClassUnknown, whole. Two class pairs align where
// both ends align and SameNode is equal. A part only one side has is a
// placement added or removed, and counts as Block on the side that lacks
// it, since nothing passed there: an after-only pair that passes is
// opened, a before-only pair that passed is closed. A location that has no
// counterpart, such as a renamed node group or a replaced interface, is
// removed on one side and added on the other, which can over-report a
// change but never hide one.
//
// Each aligned pair goes by its before → after outcomes into one list, or
// into none when both are Pass or both Block:
//
//	Opened          Block → Pass
//	Closed          Pass → Block
//	PossiblyOpened  Block → Undetermined, Undetermined → Pass
//	PossiblyClosed  Pass → Undetermined, Undetermined → Block
//	Undetermined    Undetermined → Undetermined
//
// A pair is listed with its aligned location (the addresses both sides
// share, or one side's for a placement added or removed) and the other
// attributes of the after side, or of the before side for a placement
// removed.
type Change struct {
	Key PathKey `json:"key"`
	// Before or After is empty when that side has no such path (an endpoint
	// added or removed), so every pair counts as added or removed.
	Before         Verdict     `json:"before,omitempty"`
	After          Verdict     `json:"after,omitempty"`
	Opened         []ClassPair `json:"opened,omitempty"`
	Closed         []ClassPair `json:"closed,omitempty"`
	PossiblyOpened []ClassPair `json:"possiblyOpened,omitempty"`
	PossiblyClosed []ClassPair `json:"possiblyClosed,omitempty"`
	// Undetermined pairs may have changed either way, so they are never
	// read as unchanged.
	Undetermined []ClassPair `json:"undetermined,omitempty"`
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

// EndpointSelector picks endpoints: the union of every listed form. Each
// Endpoints key, each ResourceKinds entry and each namespace of Namespaces
// (with Labels) is a form of its own, and so is every other field that
// picks (WorkloadKind belongs to Workload). A form must pick something: one
// that picks nothing, such as an Endpoints key the analysis does not have
// or a workload renamed since, is never read as a narrower or satisfied
// selector. It makes a required expectation a FindingUnmatched as a whole.
// In a forbidden rule it yields a FindingUnmatched of its own while the
// forms that pick something are still checked, so a stale form never hides
// a violation the others find. An expectations file names endpoints the
// way their own platform does (see ExpectationsFile); All, ResourceKinds
// and Endpoints, which reach everything or use internal keys, are for
// forbidden rules.
type EndpointSelector struct {
	// All picks every endpoint.
	All bool `json:"all,omitempty"`
	// ResourceKinds picks every AWS resource of these kinds, so a built-in
	// forbidden rule can name every database.
	ResourceKinds []ResourceKind `json:"resourceKinds,omitempty"`
	// Internet picks EndpointInternet.
	Internet  bool          `json:"internet,omitempty"`
	Endpoints []EndpointKey `json:"endpoints,omitempty"`
	// Self picks every workload rendered from the expectation's own source
	// (Expectation.Origin.Source).
	Self bool `json:"self,omitempty"`
	// Workload picks the workload of that name among the expectation's own
	// source's workloads in the registration's namespace. WorkloadKind
	// ("Deployment", "StatefulSet", ...) tells apart two of them that share
	// the name; without it such a name is ambiguous and picks neither, so
	// the form picks nothing rather than a guess.
	Workload     string `json:"workload,omitempty"`
	WorkloadKind string `json:"workloadKind,omitempty"`
	// Application picks every workload of a registered application,
	// "<team>/<name>" as the registrations file names it.
	Application string `json:"application,omitempty"`
	// Namespaces with Labels pick the workloads in those namespaces whose
	// pod labels match; nil Labels picks every workload in them. Labels
	// apply only together with Namespaces. Labels outlive renames that
	// names do not.
	Namespaces []string       `json:"namespaces,omitempty"`
	Labels     *LabelSelector `json:"labels,omitempty"`
	// Service names a Kubernetes Service, "<namespace>/<name>", or in an
	// expectations file "<name>" in the registration's namespace. It picks
	// the workloads the Service selects, nothing when the Service does not
	// exist or selects none, and the expectation's Ports are then the
	// Service's ports (see ExpectationStatus). It is allowed only in
	// Destinations, and only alone in its selector.
	Service string `json:"service,omitempty"`
	// RDS picks the database endpoint of the RDS DB instance
	// (KindDBInstance) whose DBInstanceIdentifier is this.
	RDS string `json:"rds,omitempty"`
	// ARN picks the AWS resource the ARN names. Only its resource part is
	// compared, since stored objects keep no ARNs or account IDs, so only a
	// resource whose identity is kept can match: an RDS DB instance
	// ("db:<identifier>"), an EC2 instance ("instance/<id>") or a load
	// balancer ("loadbalancer/app/<name>/<id>",
	// "loadbalancer/net/<name>/<id>", "loadbalancer/<name>"). Any other ARN
	// picks nothing.
	ARN string `json:"arn,omitempty"`
	// Tags picks the AWS resources whose tags include every pair. Only RDS
	// and EC2 instances keep tags, and only under the keys
	// ClusterProfile.KeptTagKeys lists, so a pair with another key picks
	// nothing.
	Tags map[string]string `json:"tags,omitempty"`
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
	// Mode is set on forbidden rules by whoever edits them; a required
	// expectation from an expectations file always warns.
	Mode   RuleMode `json:"mode,omitempty"`
	Origin Origin   `json:"origin"`
}

// RuleMode is what a finding of a rule does to a pull request. The empty
// value, that of older records, reads as ModeWarn.
type RuleMode string

const (
	// ModeWarn findings warn when New.
	ModeWarn RuleMode = "warn"
	// ModeBlock findings stop the pull request when New: violated, or
	// unverifiable for any reason, a reached limit included. So a critical
	// rule never passes unchecked, and a pull request cannot get past it by
	// growing the input beyond what the analysis computes.
	ModeBlock RuleMode = "block"
)

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
// or edit them, the mode of each forbidden rule included.
type RuleSet struct {
	Version   int64         `json:"version"`
	Forbidden []Expectation `json:"forbidden,omitempty"`
	Hygiene   []HygieneRule `json:"hygiene,omitempty"`
}

// ExpectationStatus is an expectation's three-valued state; Unverifiable is
// the expectation-level name of Undetermined. It is computed from class
// outcomes, never from a path's Verdict, by one rule.
//
// Let S and D be the endpoints the selectors pick, through the forms that
// pick something (see EndpointSelector). For s ∈ S and d ∈ D, t(s, d)
// combines the class outcomes of every path from s to d whose port range
// overlaps Ports; a path that an input-level reason covers has only
// Undetermined ones, because its gates cannot be trusted (see Path). A port
// of Ports that no path from s to d covers adds Undetermined as well, so a
// missing path is never read as an answer. With Kleene AND for ∀ and
// Kleene OR for ∃:
//
//	required:  t(s, d) = AND of the outcomes,  T = Q_s s ∈ S  Q_d d ∈ D : t(s, d)
//	           T Pass → satisfied, T Block → violated, else unverifiable
//	forbidden: t(s, d) = OR of the outcomes,   T = OR over s ∈ S, d ∈ D of t(s, d)
//	           T Pass → violated, T Block → satisfied, else unverifiable
//
// So a required expectation is satisfied exactly when, under its
// quantifiers, every path it covers is Allow, which means the network
// provides the path, not that the service accepts the connection (see
// Verdict); a forbidden one is violated by any path it covers that is Allow
// or PlacementDependent, or Unknown with a passing class and no input-level
// reason. A certain answer survives Undetermined classes elsewhere: one
// blocking class makes a required t(s, d) Block, one passing class makes a
// forbidden one Pass. A path that an input-level reason covers can neither
// satisfy nor violate an expectation; alone, it leaves it unverifiable.
//
// Pairs that Analysis.Paths does not compute, s = d among them, are left
// out rather than read as missing paths, so a forbidden rule over every
// endpoint can be satisfied by the pairs that are computed; Unexamined
// lists the rest (UnexaminedPairs). When S or D is empty, or no pair is
// left, nothing is evaluated: the expectation yields a FindingUnmatched
// whose Status is Unverifiable, never Satisfied.
//
// When Destinations names a Service, D is its backend workloads. A required
// expectation takes them with ∀ whatever DestinationQuantifier says,
// because the Service may send a connection to any of them; a forbidden
// rule is violated when any backend is reached, as its OR over D already
// says. For each backend d, Ports are translated through the Service's port
// mappings to d's own target ports (a named target port can resolve
// differently per backend), and t(s, d) is judged on the translated ports.
// A port the Service lacks, or a named target port a backend does not
// declare, makes the expectation Unverifiable with
// ReasonServicePortUnresolved; a numeric target port needs no declaration,
// as in Kubernetes.
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
	// FindingUnmatched is an expectation left with nothing to evaluate, or
	// one form of a forbidden rule that picks nothing (see
	// EndpointSelector).
	FindingUnmatched FindingKind = "unmatched"
)

// Finding is an expectation or check that is not satisfied. Exceptions are
// not applied here: they are matched when results are read, so an expired
// exception warns again without a new analysis.
//
// A forbidden expectation yields one finding per path it covers that is not
// satisfied, so a pull request that opens a new forbidden path warns even
// when the same rule is already violated elsewhere, and an exception for one
// path matches that path's finding alone; it also yields one
// FindingUnmatched for each form that picks nothing. A required
// expectation, and an expectation left with nothing to evaluate, yields one
// finding for the whole expectation.
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
	// New marks a finding the change made; only new findings warn on a pull
	// request, and the rest are existing problems listed on the open issues
	// page. A forbidden expectation's finding is New when its path's Change,
	// within the rule's Ports (as translated, for a Service), opens a class
	// pair in any way: Opened or PossiblyOpened, a placement added included.
	// Changes do not depend on rules, so a rule added since Before reports
	// what was already open as existing. Any other finding is New when its
	// status is worse (satisfied → unverifiable → violated) than that of
	// the before side's finding with the same Kind, Rule, Subject and
	// Origin.Locator, a missing one counting as satisfied.
	New bool `json:"new"`
	// Mode is the rule's mode when the analysis ran (see RuleMode); empty
	// for a hygiene finding, which only warns.
	Mode    RuleMode     `json:"mode,omitempty"`
	Paths   []PathKey    `json:"paths,omitempty"`
	Reasons []ReasonCode `json:"reasons,omitempty"`
	// Origin is where the expectation is declared, with Locator naming the
	// form for an unmatched form ("destinations.workload",
	// "sources.endpoints[1]"), or, for a hygiene finding, the object or rule
	// it is about.
	Origin Origin `json:"origin"`
	// Owner is the expectation's owner, or for a hygiene finding the
	// object's: the one to contact when another team's change breaks it;
	// filled by api.ScopeFilter.
	Owner Owner `json:"owner,omitzero"`
}
