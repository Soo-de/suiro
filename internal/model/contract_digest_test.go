package model_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/Soo-de/suiro/internal/model"
)

var digestForm = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func sha(s string) model.Digest {
	sum := sha256.Sum256([]byte(s))
	return model.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func mustDigest(t *testing.T, s model.Snapshot) model.Digest {
	t.Helper()
	d, err := model.ComputeDigest(s)
	if err != nil {
		t.Fatalf("ComputeDigest: %v", err)
	}
	if !digestForm.MatchString(string(d)) {
		t.Fatalf("digest %q is not sha256: and 64 lowercase hex digits", d)
	}
	return d
}

func TestComputeDigestEmpty(t *testing.T) {
	want := sha(`{"objects":[],"problems":[]}`)
	tests := []struct {
		name string
		s    model.Snapshot
	}{
		{"nil lists", model.Snapshot{}},
		{"empty lists", model.Snapshot{Objects: []model.Object{}, Problems: []model.Problem{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustDigest(t, tt.s); got != want {
				t.Errorf("digest = %s, want %s", got, want)
			}
		})
	}
}

func object(key string, body string) model.Object {
	return model.Object{
		Kind:   "k8s:core/ConfigMap",
		Key:    model.ObjectKey(key),
		Origin: model.Origin{Source: "payments/api", File: "a.yaml", Line: 3},
		Body:   json.RawMessage(body),
	}
}

// The bodies and their canonical forms are the examples of RFC 8785 sections
// 3.2.2 (without 1E30 in the numbers, and as written) and 3.2.3.
func TestComputeDigestRFC8785Examples(t *testing.T) {
	// Section 3.2.3: properties sorted by UTF-16 code units.
	sortingIn := `{
  "€": "Euro Sign",
  "\r": "Carriage Return",
  "דּ": "Hebrew Letter Dalet With Dagesh",
  "1": "One",
  "😀": "Emoji: Grinning Face",
  "\u0080": "Control",
  "ö": "Latin Small Letter O With Diaeresis"
}`
	sortingOut := `{"\r":"Carriage Return","1":"One",` +
		"\"\u0080\":\"Control\"," +
		"\"ö\":\"Latin Small Letter O With Diaeresis\"," +
		"\"€\":\"Euro Sign\"," +
		"\"\U0001F600\":\"Emoji: Grinning Face\"," +
		"\"דּ\":\"Hebrew Letter Dalet With Dagesh\"}"

	// Section 3.2.2: literals, numbers and strings.
	const stringIn = `"\u20ac$\u000F\u000aA'\u0042\u0022\u005c\\\"\/"`
	stringOut := "\"€" + `$\u000f\nA'B\"\\\\\"/"`
	valuesIn := `{
  "numbers": [333333333.33333329, 4.50,
              2e-3, 0.000000000000000000000000001],
  "string": ` + stringIn + `,
  "literals": [null, true, false]
}`
	valuesOut := `{"literals":[null,true,false],"numbers":[333333333.3333333,4.5,0.002,1e-27],"string":` + stringOut + `}`

	tests := []struct{ name, in, out string }{
		{"sorting", sortingIn, sortingOut},
		{"literals, numbers and strings", valuesIn, valuesOut},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := model.Snapshot{Objects: []model.Object{object("k8s:core/ConfigMap/payments/x", tt.in)}}
			envelope := `{"objects":[{"body":` + tt.out + `,"key":"k8s:core/ConfigMap/payments/x",` +
				`"kind":"k8s:core/ConfigMap","origin":{"file":"a.yaml","line":3,"source":"payments/api"}}],"problems":[]}`
			if got, want := mustDigest(t, s), sha(envelope); got != want {
				t.Errorf("digest = %s, want %s of\n%s", got, want, envelope)
			}
		})
	}
}

// The problems are given in reverse canonical order: Reason, Subject, Origin
// (none first; Source, File, Line, Resource, Locator), then Message.
func TestComputeDigestSortsObjectsAndProblems(t *testing.T) {
	origin := func(source, file string, line int, resource, locator string) *model.Origin {
		return &model.Origin{Source: model.SourceID(source), File: file, Line: line, Resource: resource, Locator: locator}
	}
	problem := func(reason, subject string, o *model.Origin, message string) model.Problem {
		return model.Problem{Reason: model.ReasonCode(reason), Subject: subject, Origin: o, Message: message}
	}
	sorted := []model.Problem{
		problem("a", "s", nil, "z"),
		problem("b", "r", origin("z/z", "z", 9, "z", "z"), ""),
		problem("b", "s", nil, "y"),
		problem("b", "s", nil, "z"),
		problem("b", "s", origin("a/a", "b", 9, "z", "z"), ""),
		problem("b", "s", origin("b/b", "a", 2, "", ""), ""),
		problem("b", "s", origin("b/b", "a", 10, "", ""), ""),
		problem("b", "s", origin("b/b", "a", 10, "i-1", "b"), ""),
		problem("b", "s", origin("b/b", "a", 10, "i-2", "a"), ""),
		problem("b", "s", origin("b/b", "a", 10, "i-2", "b"), "a"),
		problem("b", "s", origin("b/b", "a", 10, "i-2", "b"), "b"),
		problem("b", "s", origin("b/b", "b", 1, "", ""), ""),
	}
	reversed := slices.Clone(sorted)
	slices.Reverse(reversed)
	s := model.Snapshot{
		Objects: []model.Object{
			object("k8s:core/ConfigMap/payments/b", `{"n":2}`),
			object("k8s:core/ConfigMap/payments/a", `{"n":1}`),
		},
		Problems: reversed,
	}
	obj := func(name, n string) string {
		return `{"body":{"n":` + n + `},"key":"k8s:core/ConfigMap/payments/` + name + `",` +
			`"kind":"k8s:core/ConfigMap","origin":{"file":"a.yaml","line":3,"source":"payments/api"}}`
	}
	envelope := `{"objects":[` + obj("a", "1") + `,` + obj("b", "2") + `],"problems":[` +
		`{"message":"z","reason":"a","subject":"s"},` +
		`{"origin":{"file":"z","line":9,"locator":"z","resource":"z","source":"z/z"},"reason":"b","subject":"r"},` +
		`{"message":"y","reason":"b","subject":"s"},` +
		`{"message":"z","reason":"b","subject":"s"},` +
		`{"origin":{"file":"b","line":9,"locator":"z","resource":"z","source":"a/a"},"reason":"b","subject":"s"},` +
		`{"origin":{"file":"a","line":2,"source":"b/b"},"reason":"b","subject":"s"},` +
		`{"origin":{"file":"a","line":10,"source":"b/b"},"reason":"b","subject":"s"},` +
		`{"origin":{"file":"a","line":10,"locator":"b","resource":"i-1","source":"b/b"},"reason":"b","subject":"s"},` +
		`{"origin":{"file":"a","line":10,"locator":"a","resource":"i-2","source":"b/b"},"reason":"b","subject":"s"},` +
		`{"message":"a","origin":{"file":"a","line":10,"locator":"b","resource":"i-2","source":"b/b"},"reason":"b","subject":"s"},` +
		`{"message":"b","origin":{"file":"a","line":10,"locator":"b","resource":"i-2","source":"b/b"},"reason":"b","subject":"s"},` +
		`{"origin":{"file":"b","line":1,"source":"b/b"},"reason":"b","subject":"s"}]}`

	before := mustJSON(t, s)
	got := mustDigest(t, s)
	if want := sha(envelope); got != want {
		t.Errorf("digest = %s, want %s of\n%s", got, want, envelope)
	}
	if after := mustJSON(t, s); !bytes.Equal(before, after) {
		t.Errorf("ComputeDigest changed its operand:\nbefore %s\nafter  %s", before, after)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// digestSnapshot is a fresh snapshot with two objects and two problems.
func digestSnapshot() model.Snapshot {
	return model.Snapshot{
		Key:             model.ReadKey{Source: "payments/api"},
		Revision:        "abc",
		ContractVersion: model.ContractVersion,
		ReadStartedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ReadFinishedAt:  time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Objects: []model.Object{
			object("k8s:core/ConfigMap/payments/a", `{"a":1,"b":[1,2],"c":{"d":"A","e":100}}`),
			object("k8s:core/ConfigMap/payments/b", `{"f":4.5,"g":0.002,"h":0}`),
		},
		Problems: []model.Problem{
			{Reason: model.ReasonKindUnrecognized, Subject: "x", Message: "m", Origin: &model.Origin{Source: "payments/api", File: "a.yaml", Line: 1}},
			{Reason: model.ReasonUnowned, Subject: "y"},
		},
	}
}

func TestComputeDigestIgnores(t *testing.T) {
	want := mustDigest(t, digestSnapshot())
	tests := []struct {
		name   string
		change func(s *model.Snapshot)
	}{
		{"nothing", func(*model.Snapshot) {}},
		{"object order", func(s *model.Snapshot) { slices.Reverse(s.Objects) }},
		{"problem order", func(s *model.Snapshot) { slices.Reverse(s.Problems) }},
		{"key", func(s *model.Snapshot) { s.Key = model.ReadKey{Source: "other/app", PullRequest: 7} }},
		{"revision", func(s *model.Snapshot) { s.Revision = "def" }},
		{"pull request", func(s *model.Snapshot) { s.PullRequest = &model.PullRequest{Number: 7, HeadSHA: "h"} }},
		{"read times", func(s *model.Snapshot) { s.ReadStartedAt, s.ReadFinishedAt = time.Now(), time.Now() }},
		{"contract version", func(s *model.Snapshot) { s.ContractVersion = 99 }},
		{"digest field", func(s *model.Snapshot) { s.Digest = sha("something") }},
		{"body whitespace and key order", func(s *model.Snapshot) {
			s.Objects[0].Body = json.RawMessage("\n { \"c\" : {\"e\":100,\t\"d\":\"A\"} ,\"b\" : [ 1 , 2 ], \"a\":1 } \n")
		}},
		{"4.50 for 4.5", func(s *model.Snapshot) { s.Objects[1].Body = json.RawMessage(`{"f":4.50,"g":0.002,"h":0}`) }},
		{"2e-3 for 0.002", func(s *model.Snapshot) { s.Objects[1].Body = json.RawMessage(`{"f":4.5,"g":2e-3,"h":0}`) }},
		{"-0 for 0", func(s *model.Snapshot) { s.Objects[1].Body = json.RawMessage(`{"f":4.5,"g":0.002,"h":-0}`) }},
		{"1e2 for 100", func(s *model.Snapshot) {
			s.Objects[0].Body = json.RawMessage(`{"a":1,"b":[1,2],"c":{"d":"A","e":1e2}}`)
		}},
		{"escaped letter", func(s *model.Snapshot) {
			s.Objects[0].Body = json.RawMessage(`{"a":1,"b":[1,2],"c":{"d":"A","e":100}}`)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := digestSnapshot()
			tt.change(&s)
			if got := mustDigest(t, s); got != want {
				t.Errorf("digest = %s, want %s", got, want)
			}
		})
	}
}

func TestComputeDigestCovers(t *testing.T) {
	base := mustDigest(t, digestSnapshot())
	tests := []struct {
		name   string
		change func(s *model.Snapshot)
	}{
		{"problem message", func(s *model.Snapshot) { s.Problems[0].Message = "n" }},
		{"problem origin line", func(s *model.Snapshot) { s.Problems[0].Origin.Line = 2 }},
		{"problem origin removed", func(s *model.Snapshot) { s.Problems[0].Origin = nil }},
		{"problem reason", func(s *model.Snapshot) { s.Problems[1].Reason = model.ReasonHostPort }},
		{"problem subject", func(s *model.Snapshot) { s.Problems[1].Subject = "z" }},
		{"problem removed", func(s *model.Snapshot) { s.Problems = s.Problems[:1] }},
		{"problem repeated", func(s *model.Snapshot) { s.Problems = append(s.Problems, s.Problems[1]) }},
		{"object origin file", func(s *model.Snapshot) { s.Objects[0].Origin.File = "b.yaml" }},
		{"object origin line", func(s *model.Snapshot) { s.Objects[0].Origin.Line = 4 }},
		{"object origin source", func(s *model.Snapshot) { s.Objects[0].Origin.Source = "other/app" }},
		{"object origin locator", func(s *model.Snapshot) { s.Objects[0].Origin.Locator = "spec" }},
		{"object key", func(s *model.Snapshot) { s.Objects[0].Key = "k8s:core/ConfigMap/payments/c" }},
		{"object kind", func(s *model.Snapshot) { s.Objects[0].Kind = "k8s:core/Secret" }},
		{"object removed", func(s *model.Snapshot) { s.Objects = s.Objects[:1] }},
		{"body value", func(s *model.Snapshot) { s.Objects[1].Body = json.RawMessage(`{"f":4.5,"g":0.002,"h":1}`) }},
		{"body list order", func(s *model.Snapshot) {
			s.Objects[0].Body = json.RawMessage(`{"a":1,"b":[2,1],"c":{"d":"A","e":100}}`)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := digestSnapshot()
			tt.change(&s)
			if got := mustDigest(t, s); got == base {
				t.Errorf("digest unchanged: %s", got)
			}
		})
	}
}

// An unwritten and an empty field can mean different things to Kubernetes.
func TestComputeDigestKeepsEmptyFields(t *testing.T) {
	seen := make(map[model.Digest]string)
	for _, body := range []string{`{}`, `{"a":[]}`, `{"a":{}}`, `{"a":null}`, `{"a":""}`, `{"a":0}`, `{"a":false}`} {
		d := mustDigest(t, model.Snapshot{Objects: []model.Object{object("k8s:core/ConfigMap/payments/x", body)}})
		if other, ok := seen[d]; ok {
			t.Errorf("bodies %s and %s share a digest", other, body)
		}
		seen[d] = body
	}
}

func TestComputeDigestErrors(t *testing.T) {
	tests := []struct {
		name     string
		body     json.RawMessage
		tooLarge bool
	}{
		{"nil body", nil, false},
		{"empty body", json.RawMessage{}, false},
		{"truncated body", json.RawMessage(`{"a":`), false},
		{"two values", json.RawMessage(`{} {}`), false},
		{"2^53+1", json.RawMessage(`{"a":9007199254740993}`), true},
		{"-(2^53+1)", json.RawMessage(`{"a":-9007199254740993}`), true},
		{"fraction above 2^53", json.RawMessage(`{"a":9007199254740992.5}`), true},
		{"exponent form above 2^53", json.RawMessage(`{"a":9.007199254740993e15}`), true},
		{"1e16", json.RawMessage(`{"a":1e16}`), true},
		{"1e400", json.RawMessage(`{"a":1e400}`), true},
		{"nested in lists", json.RawMessage(`{"a":[{"b":[0,9007199254740993]}]}`), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The broken body comes second, after a valid one.
			s := model.Snapshot{Objects: []model.Object{
				object("k8s:core/ConfigMap/payments/a", `{}`),
				object("k8s:core/ConfigMap/payments/b", ""),
			}}
			s.Objects[1].Body = tt.body
			d, err := model.ComputeDigest(s)
			if err == nil {
				t.Fatalf("ComputeDigest = %s, want an error", d)
			}
			if tt.tooLarge && !errors.Is(err, model.ErrNumberTooLarge) {
				t.Errorf("error %q is not ErrNumberTooLarge", err)
			}
		})
	}
}

// RFC 7493 section 2.2 limits exact integers to the range of a double.
func TestComputeDigestAcceptsNumbers(t *testing.T) {
	for _, body := range []string{
		`{"a":9007199254740992}`,
		`{"a":-9007199254740992}`,
		`{"a":9007199254740992.0}`,
		`{"a":1e-400}`,
		`{"a":0.1}`,
		`{"a":"9007199254740993"}`,
		`{"9007199254740993":"1e400"}`,
	} {
		t.Run(body, func(t *testing.T) {
			mustDigest(t, model.Snapshot{Objects: []model.Object{object("k8s:core/ConfigMap/payments/x", body)}})
		})
	}
}

func TestComputeDigestLeavesOperand(t *testing.T) {
	s := digestSnapshot()
	slices.Reverse(s.Objects)
	slices.Reverse(s.Problems)
	s.Objects[0].Body = json.RawMessage(` { "z" : 1.50 , "a" : 1 } `)
	s.Digest = sha("stale")
	want := digestSnapshot()
	slices.Reverse(want.Objects)
	slices.Reverse(want.Problems)
	want.Objects[0].Body = json.RawMessage(` { "z" : 1.50 , "a" : 1 } `)
	want.Digest = sha("stale")

	first := mustDigest(t, s)
	if !reflect.DeepEqual(s, want) {
		t.Errorf("ComputeDigest changed its operand:\ngot  %+v\nwant %+v", s, want)
	}
	for range 5 {
		if got := mustDigest(t, s); got != first {
			t.Errorf("a later call gave %s, the first %s", got, first)
		}
	}
}
