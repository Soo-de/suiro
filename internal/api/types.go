package api

import (
	"time"

	"github.com/Soo-de/suiro/internal/model"
)

// The response bodies that are not model types; openapi.yaml describes them.
// A list field without omitempty must be set to an empty slice rather than
// left nil, since the API never writes null.

// EnvironmentList is the body of listEnvironments.
type EnvironmentList struct {
	// Items are sorted by Name.
	Items []Environment `json:"items"`
	// Next is the cursor of the following page; empty on the last one.
	Next string `json:"next,omitempty"`
}

// Environment is one registered environment and the analyses to show for it.
type Environment struct {
	Name string `json:"name"`
	// Baseline is the baseline in force, done or partial; nil before the
	// first one.
	Baseline *AnalysisRef `json:"baseline,omitempty"`
	// LatestAttempt is the newest baseline analysis when it did not succeed:
	// set when it is newer than Baseline, or when Baseline is nil.
	LatestAttempt *AnalysisRef `json:"latestAttempt,omitempty"`
}

// AnalysisRef names one stored analysis.
type AnalysisRef struct {
	ID        model.AnalysisID     `json:"id"`
	Status    model.AnalysisStatus `json:"status"`
	CreatedAt time.Time            `json:"createdAt"`
}

// AnalysisSummary is the body of getAnalysis: a model.Analysis with its paths
// counted per group pair instead of listed. The fields it shares with
// model.Analysis mean what they mean there.
type AnalysisSummary struct {
	ID               model.AnalysisID     `json:"id,omitempty"`
	Kind             model.AnalysisKind   `json:"kind"`
	Environment      string               `json:"environment"`
	Status           model.AnalysisStatus `json:"status"`
	CreatedAt        time.Time            `json:"createdAt,omitzero"`
	Fingerprint      model.Digest         `json:"fingerprint"`
	ToolVersion      string               `json:"toolVersion"`
	SchemaVersion    int                  `json:"schemaVersion"`
	AlgorithmVersion int                  `json:"algorithmVersion"`
	RuleSetVersion   int64                `json:"ruleSetVersion"`
	Inputs           []model.SnapshotRef  `json:"inputs"`
	Profile          model.ClusterProfile `json:"profile"`
	Limits           model.Limits         `json:"limits"`
	Unexamined       []model.Unexamined   `json:"unexamined"`
	Endpoints        []model.Endpoint     `json:"endpoints"`
	// Edges are sorted by source group, then destination group: namespaces
	// before endpoints, each by name.
	Edges    []Edge          `json:"edges"`
	Findings []model.Finding `json:"findings"`
	Problems []model.Problem `json:"problems"`
}

// Edge counts the paths from one group to another.
type Edge struct {
	Source      EdgeGroup     `json:"source"`
	Destination EdgeGroup     `json:"destination"`
	Verdicts    VerdictCounts `json:"verdicts"`
}

// EdgeGroup is one end of an Edge. Exactly one field is set: Namespace for
// workloads, Endpoint for any other endpoint.
type EdgeGroup struct {
	Namespace string            `json:"namespace,omitempty"`
	Endpoint  model.EndpointKey `json:"endpoint,omitempty"`
}

// VerdictCounts counts paths, not connections, by verdict.
type VerdictCounts struct {
	Allow     int `json:"allow"`
	Deny      int `json:"deny"`
	Placement int `json:"placement"`
	Unknown   int `json:"unknown"`
}

// PathList is the body of listPaths: one page of path summaries.
type PathList struct {
	// Items are in PathKey order.
	Items []model.Path `json:"items"`
	// Next is an opaque cursor; empty on the last page.
	Next string `json:"next,omitempty"`
}

// ReasonList is the body of listReasons.
type ReasonList struct {
	// Items are sorted by Code.
	Items []Reason `json:"items"`
}

// Reason is one reason code and what it does to verdicts.
type Reason struct {
	Code  model.ReasonCode  `json:"code"`
	Level model.ReasonLevel `json:"level"`
	Scope model.ReasonScope `json:"scope"`
}

// Error is the body of every error response: RFC 9457 problem details with a
// code extension. It is unrelated to model.Problem.
type Error struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	// Detail is for people.
	Detail string `json:"detail"`
	// Code is for programs: stable and kebab-case.
	Code string `json:"code"`
}
