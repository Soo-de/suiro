package testutil

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/Soo-de/suiro/internal/model"
)

// Expectations returns s with one model.KindExpectation object added per
// entry of doc, an expectations file read from file, the way the renderer
// publishes them. An entry's body is what model.ReduceBody keeps of it, so a
// field an adapter must reject survives, and its Origin line is that of its
// "- name:" line, so every entry starts with its name. Objects are sorted by
// key and the digest recomputed; s itself is not changed. A file of another
// apiVersion or kind, an entry without a name, or a second object with an
// entry's key fails t, since the renderer rejects such a file as a whole.
func Expectations(t testing.TB, s model.Snapshot, file, doc string) model.Snapshot {
	t.Helper()
	var f struct {
		APIVersion   string            `json:"apiVersion"`
		Kind         string            `json:"kind"`
		Expectations []json.RawMessage `json:"expectations"`
	}
	if err := yaml.UnmarshalStrict([]byte(doc), &f); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	if f.APIVersion != model.FileAPIVersion || f.Kind != model.ExpectationsFileKind {
		t.Fatalf("%s: apiVersion %q and kind %q are not an expectations file", file, f.APIVersion, f.Kind)
	}
	lines := entryLines(doc)
	if len(lines) != len(f.Expectations) {
		t.Fatalf("%s: %d entries but %d \"- name:\" lines", file, len(f.Expectations), len(lines))
	}

	s.Objects = slices.Clone(s.Objects)
	if s.Objects == nil {
		s.Objects = []model.Object{}
	}
	for i, entry := range f.Expectations {
		var head struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(entry, &head); err != nil {
			t.Fatalf("%s: entry %d: %v", file, i, err)
		}
		if head.Name == "" || head.Name != lines[i].name {
			t.Fatalf("%s:%d: entry %d is named %q, its line names %q", file, lines[i].line, i, head.Name, lines[i].name)
		}
		body, err := model.ReduceBody(model.KindExpectation, entry)
		if err != nil {
			t.Fatalf("%s:%d: %v", file, lines[i].line, err)
		}
		s.Objects = append(s.Objects, model.Object{
			Kind:   model.KindExpectation,
			Key:    model.NewObjectKey(model.KindExpectation, "", string(s.Key.Source)+"#"+head.Name),
			Origin: model.Origin{Source: s.Key.Source, File: file, Line: lines[i].line},
			Body:   body,
		})
	}
	slices.SortFunc(s.Objects, func(a, b model.Object) int { return strings.Compare(string(a.Key), string(b.Key)) })
	for i := 1; i < len(s.Objects); i++ {
		if key := s.Objects[i].Key; key == s.Objects[i-1].Key {
			t.Fatalf("%s: two objects are %s", file, key)
		}
	}
	digest, err := model.ComputeDigest(s)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	s.Digest = digest
	return s
}

type entryLine struct {
	line int
	name string
}

// entryLines returns the "- name:" lines of doc at the indentation of the
// first one, which are the lines its entries start on.
func entryLines(doc string) []entryLine {
	var out []entryLine
	indent := -1
	for i, line := range strings.Split(doc, "\n") {
		text := strings.TrimLeft(line, " ")
		if !strings.HasPrefix(text, "- name:") {
			continue
		}
		if indent < 0 {
			indent = len(line) - len(text)
		}
		if len(line)-len(text) != indent {
			continue
		}
		var head []struct {
			Name string `json:"name"`
		}
		e := entryLine{line: i + 1}
		if err := yaml.Unmarshal([]byte(text), &head); err == nil && len(head) == 1 {
			e.name = head[0].Name
		}
		out = append(out, e)
	}
	return out
}
