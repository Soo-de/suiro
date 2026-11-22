package testutil

import (
	"cmp"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/Soo-de/suiro/internal/model"
)

// Bundle builds a model.Bundle for core tests.
type Bundle struct {
	b model.Bundle
}

// NewBundle starts a baseline Bundle of environment with this build's
// SchemaVersion and AlgorithmVersion.
func NewBundle(environment string) *Bundle {
	return &Bundle{b: model.Bundle{
		Kind:             model.AnalysisBaseline,
		Environment:      environment,
		SchemaVersion:    model.SchemaVersion,
		AlgorithmVersion: model.AlgorithmVersion,
	}}
}

// Git adds the git source s was read from, registered on namespace. The
// source ID has the form "<team>/<name>", which also gives its owner; its
// repository is a placeholder rendered plain from the root.
func (x *Bundle) Git(s model.Snapshot, namespace string) *Bundle {
	team, name, _ := strings.Cut(string(s.Key.Source), "/")
	x.add(model.Source{
		ID:          s.Key.Source,
		Kind:        model.SourceGit,
		Environment: x.b.Environment,
		Owner:       model.Owner{Team: team, Application: name},
		Git: &model.GitSource{
			RepoURL:        "https://github.com/example/" + name,
			Path:           ".",
			TargetRevision: "main",
			Render:         model.RenderPlain,
			Namespace:      namespace,
		},
	}, s)
	return x
}

// AWS adds the environment's aws source, with s as its read.
func (x *Bundle) AWS(s model.Snapshot) *Bundle {
	x.add(model.Source{ID: s.Key.Source, Kind: model.SourceAWS, Environment: x.b.Environment, AWS: &model.AWSSource{}}, s)
	return x
}

// Cluster adds the environment's cluster source, with s as its read.
func (x *Bundle) Cluster(s model.Snapshot) *Bundle {
	x.add(model.Source{ID: s.Key.Source, Kind: model.SourceCluster, Environment: x.b.Environment, Cluster: &model.ClusterSource{}}, s)
	return x
}

// PullRequest makes the Bundle the analysis of pull request pr.
func (x *Bundle) PullRequest(pr model.PullRequest) *Bundle {
	x.b.Kind = model.AnalysisPullRequest
	x.b.PullRequest = &pr
	return x
}

// Profile sets the admin-entered cluster profile.
func (x *Bundle) Profile(p model.ClusterProfile) *Bundle {
	x.b.Profile = p
	return x
}

// Rules sets the rule set in force.
func (x *Bundle) Rules(r model.RuleSet) *Bundle {
	x.b.Rules = r
	return x
}

// Before sets the before side.
func (x *Bundle) Before(a *model.Analysis) *Bundle {
	x.b.Before = a
	return x
}

// Problem adds a problem inputs found before analysis.
func (x *Bundle) Problem(p model.Problem) *Bundle {
	x.b.Problems = append(x.b.Problems, p)
	return x
}

func (x *Bundle) add(src model.Source, s model.Snapshot) {
	x.b.Sources = append(x.b.Sources, src)
	x.b.Snapshots = append(x.b.Snapshots, s)
}

// Build returns the Bundle in the order model.Bundle states: Sources by ID,
// Snapshots by Key, Problems by Reason then Subject, and UsedNamespaces
// from the metadata.namespace of each git snapshot's objects. Fingerprint
// stays empty, since core copies it and never checks it. A body it cannot
// read fails t.
func (x *Bundle) Build(t testing.TB) model.Bundle {
	t.Helper()
	b := x.b
	b.Sources = slices.Clone(b.Sources)
	b.Snapshots = slices.Clone(b.Snapshots)
	b.Problems = slices.Clone(b.Problems)
	slices.SortFunc(b.Sources, func(p, q model.Source) int { return cmp.Compare(p.ID, q.ID) })
	slices.SortFunc(b.Snapshots, func(p, q model.Snapshot) int {
		return cmp.Or(cmp.Compare(p.Key.Source, q.Key.Source), cmp.Compare(p.Key.PullRequest, q.Key.PullRequest))
	})
	slices.SortFunc(b.Problems, func(p, q model.Problem) int {
		return cmp.Or(cmp.Compare(p.Reason, q.Reason), cmp.Compare(p.Subject, q.Subject))
	})
	if b.Sources == nil {
		b.Sources = []model.Source{}
	}
	if b.Snapshots == nil {
		b.Snapshots = []model.Snapshot{}
	}

	git := make(map[model.SourceID]bool)
	for _, src := range b.Sources {
		git[src.ID] = src.Kind == model.SourceGit
	}
	b.UsedNamespaces = nil
	for _, s := range b.Snapshots {
		if !git[s.Key.Source] {
			continue
		}
		for _, o := range s.Objects {
			var body struct {
				Metadata struct {
					Namespace string `json:"namespace"`
				} `json:"metadata"`
			}
			if err := json.Unmarshal(o.Body, &body); err != nil {
				t.Fatalf("%s: %v", o.Key, err)
			}
			if ns := body.Metadata.Namespace; ns != "" {
				if b.UsedNamespaces == nil {
					b.UsedNamespaces = make(map[model.SourceID][]string)
				}
				b.UsedNamespaces[s.Key.Source] = append(b.UsedNamespaces[s.Key.Source], ns)
			}
		}
	}
	for id, nss := range b.UsedNamespaces {
		slices.Sort(nss)
		b.UsedNamespaces[id] = slices.Compact(nss)
	}
	return b
}
