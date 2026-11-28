package conformance

import (
	"embed"
	"path"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/Soo-de/suiro/internal/model"
	"github.com/Soo-de/suiro/internal/testutil" //nolint:depguard // the suite is itself a test helper
)

//go:embed testdata
var testdata embed.FS

// Scenario is one case of the suite: the sources of an environment, what an
// adapter must make of each, and the results the analysis of them must give.
// testdata/ORACLE.md says where each expected result comes from.
type Scenario struct {
	Name string
	// Oracle is the documentation page and section the results come from.
	Oracle  string
	Sources []Source
	// Profile is testutil.LabProfile unless the scenario sets another.
	Profile  model.ClusterProfile
	Paths    []Probe
	Statuses []StatusProbe
}

// Source is one source of a scenario.
type Source struct {
	ID   model.SourceID
	Kind model.SourceKind
	// Namespace is a git source's destination namespace.
	Namespace string
	Snapshot  model.Snapshot
	// Want is the fragment the source's adapter must return, with no Source,
	// origins that hold only a Locator and problems without a Message.
	Want model.Fragment
}

// Probe states the analysis result of one pair of endpoints over a port
// range: every path of the pair overlapping Key.Ports has Verdict, and the
// pair's paths cover the range.
type Probe struct {
	Key     model.PathKey
	Verdict model.Verdict
	// Reasons are the reasons of those paths; nil means they are not checked.
	Reasons []model.ReasonCode
}

// StatusProbe states the status of one expectation, named by its ID.
type StatusProbe struct {
	Expectation string
	Status      model.ExpectationStatus
}

// The file names Origin.File carries for a scenario source's two inputs.
const (
	manifestsFile    = "manifests.yaml"
	expectationsFile = model.ExpectationsFileName
)

type scenarioFile struct {
	Name    string `json:"name"`
	Oracle  string `json:"oracle"`
	Sources []struct {
		ID           model.SourceID   `json:"id"`
		Kind         model.SourceKind `json:"kind"`
		Namespace    string           `json:"namespace"`
		Manifests    string           `json:"manifests"`
		Expectations string           `json:"expectations"`
		Want         *model.Fragment  `json:"want"`
	} `json:"sources"`
	Profile *model.ClusterProfile `json:"profile"`
	Paths   []struct {
		Key     string             `json:"key"`
		Verdict model.Verdict      `json:"verdict"`
		Reasons []model.ReasonCode `json:"reasons"`
	} `json:"paths"`
	Statuses []struct {
		Expectation string                  `json:"expectation"`
		Status      model.ExpectationStatus `json:"status"`
	} `json:"statuses"`
}

// Load returns the scenarios of one source format, in name order; "k8s" is
// the only format. A scenario file that does not read strictly (an unknown or
// repeated field, an input testutil cannot read, an undeclared kind, verdict
// or status, a path key ParsePathKey rejects) fails t, as does a format with
// no scenario.
func Load(t testing.TB, format string) []Scenario {
	t.Helper()
	root := path.Join("testdata", format)
	entries, err := testdata.ReadDir(root)
	if err != nil {
		t.Fatalf("conformance: format %q: %v", format, err)
	}
	var out []Scenario
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, load(t, path.Join(root, e.Name(), "scenario.yaml")))
		}
	}
	if len(out) == 0 {
		t.Fatalf("conformance: format %q has no scenario", format)
	}
	return out
}

func load(t testing.TB, file string) Scenario {
	t.Helper()
	data, err := testdata.ReadFile(file)
	if err != nil {
		t.Fatalf("conformance: %v", err)
	}
	var f scenarioFile
	if err := yaml.UnmarshalStrict(data, &f); err != nil {
		t.Fatalf("conformance: %s: %v", file, err)
	}
	if f.Name == "" || f.Oracle == "" || len(f.Sources) == 0 {
		t.Fatalf("conformance: %s: name, oracle and sources are required", file)
	}

	s := Scenario{Name: f.Name, Oracle: f.Oracle, Profile: testutil.LabProfile()}
	if f.Profile != nil {
		s.Profile = *f.Profile
	}
	seen := make(map[model.SourceID]bool)
	for _, src := range f.Sources {
		git := src.Kind == model.SourceGit
		switch {
		case src.ID == "" || seen[src.ID]:
			t.Fatalf("conformance: %s: source id %q is empty or repeated", file, src.ID)
		case !git && src.Kind != model.SourceCluster:
			t.Fatalf("conformance: %s: source %s: kind %q is not git or cluster", file, src.ID, src.Kind)
		case git != (src.Namespace != ""):
			t.Fatalf("conformance: %s: source %s: a git source, and only it, has a namespace", file, src.ID)
		case !git && src.Expectations != "":
			t.Fatalf("conformance: %s: source %s: only a git source has expectations", file, src.ID)
		case src.Want == nil:
			t.Fatalf("conformance: %s: source %s: no want", file, src.ID)
		}
		seen[src.ID] = true
		if why := beyondWant(*src.Want); why != "" {
			t.Fatalf("conformance: %s: source %s: want %s", file, src.ID, why)
		}
		snapshot := testutil.Snapshot(t, model.ReadKey{Source: src.ID}, manifestsFile, src.Manifests)
		if src.Expectations != "" {
			snapshot = testutil.Expectations(t, snapshot, expectationsFile, src.Expectations)
		}
		s.Sources = append(s.Sources, Source{
			ID:        src.ID,
			Kind:      src.Kind,
			Namespace: src.Namespace,
			Snapshot:  snapshot,
			Want:      *src.Want,
		})
	}

	for _, p := range f.Paths {
		key, err := model.ParsePathKey(p.Key)
		if err != nil {
			t.Fatalf("conformance: %s: %v", file, err)
		}
		switch p.Verdict {
		case model.Allow, model.Deny, model.PlacementDependent, model.Unknown:
		default:
			t.Fatalf("conformance: %s: %s: verdict %q", file, p.Key, p.Verdict)
		}
		s.Paths = append(s.Paths, Probe{Key: key, Verdict: p.Verdict, Reasons: p.Reasons})
	}
	for _, p := range f.Statuses {
		switch p.Status {
		case model.Satisfied, model.Violated, model.Unverifiable:
		default:
			t.Fatalf("conformance: %s: %s: status %q", file, p.Expectation, p.Status)
		}
		s.Statuses = append(s.Statuses, StatusProbe{Expectation: p.Expectation, Status: p.Status})
	}
	return s
}

// beyondWant names what want states that a fixture must leave out, or "".
func beyondWant(want model.Fragment) string {
	if want.Source != "" {
		return "sets source"
	}
	for _, o := range origins(&want) {
		if *o != (model.Origin{Locator: o.Locator}) {
			return "has an origin with more than a locator"
		}
	}
	for _, p := range want.Problems {
		if p.Message != "" {
			return "has a problem message"
		}
	}
	return ""
}

// origins returns every origin of f that a Kubernetes adapter writes, for
// reading or changing in place.
func origins(f *model.Fragment) []*model.Origin {
	var out []*model.Origin
	for i := range f.Workloads {
		out = append(out, &f.Workloads[i].Origin)
	}
	for i := range f.Namespaces {
		out = append(out, &f.Namespaces[i].Origin)
	}
	for i := range f.Gates {
		out = append(out, &f.Gates[i].Origin)
		for j := range f.Gates[i].Rules {
			out = append(out, &f.Gates[i].Rules[j].Origin)
		}
	}
	for i := range f.Expectations {
		out = append(out, &f.Expectations[i].Origin)
	}
	for i := range f.Problems {
		if f.Problems[i].Origin != nil {
			out = append(out, f.Problems[i].Origin)
		}
	}
	return out
}
