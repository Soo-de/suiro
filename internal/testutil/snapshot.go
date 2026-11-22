package testutil

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/Soo-de/suiro/internal/model"
)

// Snapshot reads docs, rendered Kubernetes YAML of one or more documents
// separated by "---" lines, as the complete read of key from file. Each
// document that holds an object becomes an Object: its kind from apiVersion
// and kind, its key from model.NewObjectKey, its body from model.ReduceBody,
// and its Origin from key's source, file and the document's first line.
// Namespaces are taken as written, never defaulted. Objects are sorted by
// key, and the digest is model.ComputeDigest's. For a pull request key the
// caller sets PullRequest. A document it cannot read fails t.
func Snapshot(t testing.TB, key model.ReadKey, file, docs string) model.Snapshot {
	t.Helper()
	s := model.Snapshot{Key: key, ContractVersion: model.ContractVersion, Objects: []model.Object{}}
	for _, d := range split(docs) {
		raw, err := yaml.YAMLToJSON([]byte(d.text))
		if err != nil {
			t.Fatalf("%s:%d: %v", file, d.line, err)
		}
		if string(raw) == "null" {
			continue
		}
		var head struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Metadata   struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			t.Fatalf("%s:%d: %v", file, d.line, err)
		}
		if head.Kind == "" || head.Metadata.Name == "" {
			t.Fatalf("%s:%d: a document without kind or metadata.name", file, d.line)
		}
		kind := kindOf(head.APIVersion, head.Kind)
		body, err := model.ReduceBody(kind, raw)
		if err != nil {
			t.Fatalf("%s:%d: %v", file, d.line, err)
		}
		s.Objects = append(s.Objects, model.Object{
			Kind:   kind,
			Key:    model.NewObjectKey(kind, head.Metadata.Namespace, head.Metadata.Name),
			Origin: model.Origin{Source: key.Source, File: file, Line: d.line},
			Body:   body,
		})
	}
	slices.SortFunc(s.Objects, func(a, b model.Object) int { return strings.Compare(string(a.Key), string(b.Key)) })
	digest, err := model.ComputeDigest(s)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	s.Digest = digest
	return s
}

// kindOf returns the ObjectKind of a Kubernetes apiVersion and kind; the
// empty API group is "core".
func kindOf(apiVersion, kind string) model.ObjectKind {
	group := "core"
	if g, _, ok := strings.Cut(apiVersion, "/"); ok {
		group = g
	}
	return model.ObjectKind("k8s:" + group + "/" + kind)
}

type document struct {
	text string
	// line is the 1-based line of the document's first line that is
	// neither blank nor a comment.
	line int
}

// split cuts docs at its "---" lines, dropping documents with no content.
func split(docs string) []document {
	var out []document
	var lines []string
	start := 0
	flush := func() {
		if start != 0 {
			out = append(out, document{text: strings.Join(lines, "\n"), line: start})
		}
		lines, start = nil, 0
	}
	for i, line := range strings.Split(docs, "\n") {
		if l := strings.TrimRight(line, " \t"); l == "---" || strings.HasPrefix(l, "--- #") {
			flush()
			continue
		}
		lines = append(lines, line)
		if t := strings.TrimSpace(line); start == 0 && t != "" && !strings.HasPrefix(t, "#") {
			start = i + 1
		}
	}
	flush()
	return out
}
