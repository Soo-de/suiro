package conformance

import (
	"context"
	"encoding/json"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/Soo-de/suiro/internal/core"
	"github.com/Soo-de/suiro/internal/core/adapter/k8s"
	"github.com/Soo-de/suiro/internal/model"
)

// required are the scenarios that keep missing input from becoming an allow
// (see the package comment).
var required = []string{"np-sctp-only-rule", "np-ipv6-only-peer", "np-mixed-rule", "np-invalid"}

func TestScenariosWellFormed(t *testing.T) {
	scenarios := Load(t, "k8s")
	entries, err := testdata.ReadDir(path.Join("testdata", "k8s"))
	if err != nil {
		t.Fatal(err)
	}
	var dirs, names []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	for _, s := range scenarios {
		names = append(names, s.Name)
	}
	if !slices.Equal(dirs, names) {
		t.Fatalf("scenario names %v differ from their directories %v", names, dirs)
	}
	for _, name := range required {
		if !slices.Contains(names, name) {
			t.Errorf("required scenario %s is missing", name)
		}
	}

	oracle, err := testdata.ReadFile("testdata/ORACLE.md")
	if err != nil {
		t.Fatal(err)
	}
	rows := make(map[string]bool)
	for _, line := range strings.Split(string(oracle), "\n") {
		if cells := strings.Split(line, "|"); len(cells) > 2 {
			rows[strings.Trim(cells[1], " `")] = true
		}
	}

	for _, s := range scenarios {
		t.Run(s.Name, func(t *testing.T) {
			if !rows[s.Name] {
				t.Error("no row in testdata/ORACLE.md")
			}
			if strings.Contains(strings.TrimSpace(s.Oracle), "\n") {
				t.Error("oracle is more than one line")
			}
			checkProbes(t, s)
		})
	}
}

// checkProbes checks that every probe of s is about something s holds.
func checkProbes(t *testing.T, s Scenario) {
	t.Helper()
	ends := map[model.EndpointKey]bool{
		model.EndpointInternet:        true,
		model.EndpointExternalPrivate: true,
		model.EndpointLinkLocal:       true,
	}
	for _, n := range s.Profile.PrivateNetworks {
		ends[model.EndpointExternalPrivate+":"+model.EndpointKey(n.Name)] = true
	}
	for _, n := range s.Profile.LinkLocalNetworks {
		ends[model.EndpointLinkLocal+":"+model.EndpointKey(n.Name)] = true
	}
	expectations := make(map[string]bool)
	for _, src := range s.Sources {
		for _, w := range src.Want.Workloads {
			ends[w.Key] = true
		}
		for _, e := range src.Want.Expectations {
			expectations[e.ID] = true
		}
	}

	seen := make(map[model.PathKey]bool)
	for _, p := range s.Paths {
		for _, end := range []model.EndpointKey{p.Key.Source, p.Key.Destination} {
			if !ends[end] {
				t.Errorf("%s: %s is neither a wanted workload nor a network endpoint", p.Key, end)
			}
		}
		if seen[p.Key] {
			t.Errorf("%s: probed twice", p.Key)
		}
		seen[p.Key] = true
		for _, r := range p.Reasons {
			if _, ok := model.Reasons()[r]; !ok {
				t.Errorf("%s: reason %q is not declared", p.Key, r)
			}
		}
		if !slices.IsSorted(p.Reasons) {
			t.Errorf("%s: reasons are not sorted", p.Key)
		}
	}
	for _, p := range s.Statuses {
		if !expectations[p.Expectation] {
			t.Errorf("status of %q, which no want holds", p.Expectation)
		}
	}
}

func TestConformance(t *testing.T) {
	formats := []struct {
		name     string
		adapters map[model.SourceKind]func() core.Adapter
	}{
		{"k8s", map[model.SourceKind]func() core.Adapter{
			model.SourceGit:     k8s.New,
			model.SourceCluster: k8s.NewCluster,
		}},
	}
	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			for _, s := range Load(t, format.name) {
				t.Run(s.Name, func(t *testing.T) {
					for _, src := range s.Sources {
						newAdapter, ok := format.adapters[src.Kind]
						if !ok {
							t.Fatalf("source %s: no adapter for kind %q", src.ID, src.Kind)
						}
						checkSource(t, newAdapter(), src)
					}
				})
			}
		})
	}
}

// checkSource holds one source's translation to what every adapter promises
// and to the source's wanted fragment.
func checkSource(t *testing.T, a core.Adapter, src Source) {
	t.Helper()
	if a.Kind() != src.Kind {
		t.Errorf("source %s: adapter kind %q, want %q", src.ID, a.Kind(), src.Kind)
	}
	got, err := a.Translate(context.Background(), src.Snapshot)
	if err != nil {
		t.Fatalf("source %s: Translate: %v", src.ID, err)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("source %s: %v", src.ID, err)
	}
	if got.Source != src.ID {
		t.Errorf("fragment source %q, want %q", got.Source, src.ID)
	}
	for _, p := range got.Problems {
		if !slices.Contains(model.Reasons()[p.Reason].RaisedBy, model.ProducerAdapter) {
			t.Errorf("source %s: reason %q is not an adapter's", src.ID, p.Reason)
		}
	}
	checkOrigins(t, src.Snapshot, got)
	checkAccounted(t, src.Snapshot, got)

	gotJSON, wantJSON := comparable(t, got), comparable(t, src.Want)
	if gotJSON != wantJSON {
		t.Errorf("source %s: fragment differs\n got: %s\nwant: %s", src.ID, gotJSON, wantJSON)
	}
}

// checkOrigins checks that everything made from an object says it came from
// that object's source, file and line.
func checkOrigins(t *testing.T, s model.Snapshot, f model.Fragment) {
	t.Helper()
	objects := make(map[model.ObjectKey]model.Origin, len(s.Objects))
	for _, o := range s.Objects {
		objects[o.Key] = o.Origin
	}
	check := func(key model.ObjectKey, got model.Origin) {
		want, ok := objects[key]
		if !ok {
			t.Errorf("source %s: the fragment names %s, which the snapshot does not hold", s.Key.Source, key)
			return
		}
		if got.Source != want.Source || got.File != want.File || got.Line != want.Line {
			t.Errorf("%s: origin %s %s:%d, want %s %s:%d", key,
				got.Source, got.File, got.Line, want.Source, want.File, want.Line)
		}
	}
	for _, w := range f.Workloads {
		check(model.ObjectKey(w.Key), w.Origin)
	}
	for _, n := range f.Namespaces {
		check(model.NewObjectKey(model.KindNamespace, "", n.Name), n.Origin)
	}
	for _, g := range f.Gates {
		check(g.Object, g.Origin)
		for _, r := range g.Rules {
			check(g.Object, r.Origin)
		}
	}
	for _, e := range f.Expectations {
		check(model.NewObjectKey(model.KindExpectation, "", e.ID), e.Origin)
	}
	for _, p := range f.Problems {
		if p.Origin == nil {
			t.Errorf("%s: problem %s has no origin", p.Subject, p.Reason)
			continue
		}
		check(model.ObjectKey(p.Subject), *p.Origin)
	}
}

// checkAccounted checks that no object of s was skipped silently.
func checkAccounted(t *testing.T, s model.Snapshot, f model.Fragment) {
	t.Helper()
	seen := make(map[model.ObjectKey]bool)
	for _, w := range f.Workloads {
		seen[model.ObjectKey(w.Key)] = true
	}
	for _, n := range f.Namespaces {
		seen[model.NewObjectKey(model.KindNamespace, "", n.Name)] = true
	}
	for _, g := range f.Gates {
		seen[g.Object] = true
	}
	for _, e := range f.Expectations {
		seen[model.NewObjectKey(model.KindExpectation, "", e.ID)] = true
	}
	for _, p := range f.Problems {
		seen[model.ObjectKey(p.Subject)] = true
	}
	ignored := model.IgnoredKinds()
	for _, o := range s.Objects {
		if !seen[o.Key] && !slices.Contains(ignored, o.Kind) {
			t.Errorf("%s is in neither the fragment nor a problem", o.Key)
		}
	}
}

// comparable returns f as the JSON a scenario's want states: without the
// source, the parts of origins that follow from the input's layout and
// problem messages. f is not changed.
func comparable(t *testing.T, f model.Fragment) string {
	t.Helper()
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	var c model.Fragment
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	c.Source = ""
	for _, o := range origins(&c) {
		*o = model.Origin{Locator: o.Locator}
	}
	for i := range c.Problems {
		c.Problems[i].Message = ""
	}
	out, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
