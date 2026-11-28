package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Soo-de/suiro/internal/api"
	"github.com/Soo-de/suiro/internal/model"
)

// closedSets are the sets whose growth would change the core's meaning, with
// the constants internal/model declares for each.
var closedSets = map[reflect.Type][]string{
	reflect.TypeFor[model.Verdict](): values(model.Allow, model.Deny, model.PlacementDependent, model.Unknown),
	reflect.TypeFor[model.Outcome](): values(model.Pass, model.Block, model.Undetermined),
	reflect.TypeFor[model.AnalysisStatus](): values(
		model.AnalysisDone, model.AnalysisPartial, model.AnalysisNoPreview, model.AnalysisFailed),
	reflect.TypeFor[model.AnalysisKind](): values(model.AnalysisBaseline, model.AnalysisPullRequest),
	reflect.TypeFor[model.Direction]():    values(model.Ingress, model.Egress),
	reflect.TypeFor[model.GateType]():     values(model.GateFilter, model.GateRoute, model.GateTransform),
	reflect.TypeFor[model.FindingKind](): values(
		model.FindingForbidden, model.FindingRequired, model.FindingHygiene, model.FindingUnmatched),
	reflect.TypeFor[model.ExpectationStatus](): values(model.Satisfied, model.Violated, model.Unverifiable),
}

// values returns the strings of set, sorted.
func values[S ~string](set ...S) []string {
	out := make([]string, 0, len(set))
	for _, v := range set {
		out = append(out, string(v))
	}
	slices.Sort(out)
	return out
}

func enumOf(s *openapi3.Schema) []string {
	out := make([]string, 0, len(s.Enum))
	for _, v := range s.Enum {
		out = append(out, fmt.Sprint(v))
	}
	slices.Sort(out)
	return out
}

// walkLeaves calls leaf for every scalar schema of the top-level responses,
// with the description of the array or map holding it.
func walkLeaves(t *testing.T, doc *openapi3.T, leaf func(at string, typ reflect.Type, s *openapi3.Schema, outer string)) {
	t.Helper()
	w := &schemaWalker{t: t, visited: map[string]bool{}}
	w.leaf = func(at string, typ reflect.Type, s *openapi3.Schema) { leaf(at, typ, s, w.outer) }
	for _, name := range slices.Sorted(maps.Keys(topLevelTypes)) {
		ref := doc.Components.Schemas[name]
		if ref == nil || ref.Value == nil {
			t.Errorf("components.schemas.%s is missing", name)
			continue
		}
		w.walk(name, topLevelTypes[name], &openapi3.SchemaRef{Ref: schemaPrefix + name, Value: ref.Value})
	}
}

func TestClosedSetsAreEnums(t *testing.T) {
	seen := map[reflect.Type]bool{}
	walkLeaves(t, loadSpec(t), func(at string, typ reflect.Type, s *openapi3.Schema, _ string) {
		want, closed := closedSets[typ]
		if !closed {
			if len(s.Enum) > 0 || s.Const != nil {
				t.Errorf("%s (%s): an open set must be a plain string, got enum %v const %v", at, typ, s.Enum, s.Const)
			}
			return
		}
		seen[typ] = true
		if got := enumOf(s); !slices.Equal(got, want) {
			t.Errorf("%s (%s): enum %v, want %v", at, typ, got, want)
		}
	})
	for typ := range closedSets {
		if !seen[typ] {
			t.Errorf("no response field of type %s was checked", typ)
		}
	}
}

// collectEnums gathers every enum below ref, each schema once.
func collectEnums(ref *openapi3.SchemaRef, at string, seen map[*openapi3.Schema]bool, out map[string][]string) {
	if ref == nil || ref.Value == nil || seen[ref.Value] {
		return
	}
	s := ref.Value
	seen[s] = true
	if len(s.Enum) > 0 {
		out[at] = enumOf(s)
	}
	for name, p := range s.Properties {
		collectEnums(p, at+"."+name, seen, out)
	}
	collectEnums(s.Items, at+"[]", seen, out)
	collectEnums(s.AdditionalProperties.Schema, at+"{}", seen, out)
	for i, part := range slices.Concat(s.AllOf, s.AnyOf, s.OneOf) {
		collectEnums(part, fmt.Sprintf("%s(%d)", at, i), seen, out)
	}
}

// No enum anywhere in the spec, parameters included, may be a set of its own.
func TestEveryEnumIsAClosedSet(t *testing.T) {
	doc := loadSpec(t)
	seen := map[*openapi3.Schema]bool{}
	enums := map[string][]string{}
	for name, ref := range doc.Components.Schemas {
		collectEnums(ref, name, seen, enums)
	}
	for _, c := range contracts() {
		item, op := operationOf(t, doc, c)
		for _, p := range slices.Concat(item.Parameters, op.Parameters) {
			if p.Value != nil {
				collectEnums(p.Value.Schema, c.id+" "+p.Value.Name, seen, enums)
			}
		}
	}
	for at, got := range enums {
		known := slices.ContainsFunc(slices.Collect(maps.Values(closedSets)), func(want []string) bool {
			return slices.Equal(got, want)
		})
		if !known {
			t.Errorf("%s: enum %v is not one of the closed sets", at, got)
		}
	}
}

func TestParameterEnums(t *testing.T) {
	doc := loadSpec(t)
	for _, c := range contracts() {
		item, op := operationOf(t, doc, c)
		for _, ref := range slices.Concat(item.Parameters, op.Parameters) {
			p := ref.Value
			if p == nil || p.Schema == nil || p.Schema.Value == nil {
				t.Errorf("%s: unresolved parameter %q", c.id, ref.Ref)
				continue
			}
			s := p.Schema.Value
			if s.Items != nil && s.Items.Value != nil {
				s = s.Items.Value
			}
			var want []string
			if p.Name == "verdict" {
				want = closedSets[reflect.TypeFor[model.Verdict]()]
			}
			if got := enumOf(s); !slices.Equal(got, want) {
				t.Errorf("%s %s: enum %v, want %v", c.id, p.Name, got, want)
			}
		}
	}
}

// mentions reports whether text names value as a word of its own.
func mentions(text, value string) bool {
	return regexp.MustCompile(`(^|[^\w-])` + regexp.QuoteMeta(value) + `($|[^\w-])`).MatchString(text)
}

// An open set is a plain string whose description gives its known values.
func TestOpenSetsDescribeTheirKnownValues(t *testing.T) {
	known := map[reflect.Type][]string{
		reflect.TypeFor[model.Layer](): values(
			model.LayerKubernetes, model.LayerAWSInstance, model.LayerAWSNetwork),
		reflect.TypeFor[model.EndpointKind](): values(
			model.EndpointKindWorkload, model.EndpointKindResource, model.EndpointKindInternet,
			model.EndpointKindExternalPrivate, model.EndpointKindLinkLocal),
		reflect.TypeFor[model.ResourceKind](): values(
			model.ResourceDatabase, model.ResourceLoadBalancer, model.ResourceInstance, model.ResourceOther),
		reflect.TypeFor[model.Protocol]():      values(model.ProtocolTCP, model.ProtocolUDP),
		reflect.TypeFor[model.Toggle]():        values(model.ToggleOn, model.ToggleOff),
		reflect.TypeFor[model.ProfileSource](): values(model.ProfileFromAWS, model.ProfileFromAdmin),
		reflect.TypeFor[model.Unexamined](): values(
			model.UnexaminedHostFirewall, model.UnexaminedApplication, model.UnexaminedTLS,
			model.UnexaminedAWSLayer, model.UnexaminedIPv6, model.UnexaminedPairs),
	}
	seen := map[reflect.Type]bool{}
	walkLeaves(t, loadSpec(t), func(at string, typ reflect.Type, s *openapi3.Schema, outer string) {
		want, ok := known[typ]
		if !ok {
			return
		}
		seen[typ] = true
		var missing []string
		for _, v := range want {
			if !mentions(s.Description+" "+outer, v) {
				missing = append(missing, v)
			}
		}
		if len(missing) > 0 {
			t.Errorf("%s (%s): description lacks %v", at, typ, missing)
		}
	})
	for typ := range known {
		if !seen[typ] {
			t.Errorf("no response field of type %s was checked", typ)
		}
	}
}

// Reason codes are too many to list in each description; every reason code
// schema points to listReasons, which serves them with their levels.
func TestReasonCodesPointToTheReasonList(t *testing.T) {
	seen := false
	walkLeaves(t, loadSpec(t), func(at string, typ reflect.Type, s *openapi3.Schema, outer string) {
		if typ != reflect.TypeFor[model.ReasonCode]() {
			return
		}
		seen = true
		if !mentions(s.Description+" "+outer, "listReasons") {
			t.Errorf("%s: reason code description does not point to listReasons", at)
		}
	})
	if !seen {
		t.Error("no response field of type model.ReasonCode was checked")
	}
}

func TestErrorCodeIsAnOpenSet(t *testing.T) {
	ref := loadSpec(t).Components.Schemas["Error"]
	if ref == nil || ref.Value == nil {
		t.Fatal("components.schemas.Error is missing")
	}
	code := shapeOf(ref.Value).properties["code"]
	if code == nil || code.Value == nil {
		t.Fatal("Error has no code property")
	}
	s := code.Value
	if !s.Type.Is("string") || len(s.Enum) > 0 || s.Const != nil {
		t.Errorf("Error.code must be a plain string, got type %v enum %v", s.Type, s.Enum)
	}
	for _, c := range contracts() {
		for _, codes := range c.errors {
			for _, v := range codes {
				if !mentions(s.Description, v) {
					t.Errorf("Error.code: description lacks %q", v)
				}
			}
		}
	}
}

func TestPathOutcomesPattern(t *testing.T) {
	ref := loadSpec(t).Components.Schemas["Path"]
	if ref == nil || ref.Value == nil {
		t.Fatal("components.schemas.Path is missing")
	}
	prop := ref.Value.Properties["outcomes"]
	if prop == nil || prop.Value == nil {
		t.Fatal("Path has no outcomes property")
	}
	s := prop.Value
	if !s.Type.Is("string") || len(s.Enum) > 0 || s.Pattern != "^[PBU]+$" {
		t.Errorf("outcomes: type %v enum %v pattern %q, want a string with pattern ^[PBU]+$", s.Type, s.Enum, s.Pattern)
	}
	cases := map[string]bool{
		"P": true, "B": true, "U": true, "PB": true, "UUBP": true,
		"": false, "X": false, "PXB": false, "pb": false, "P B": false,
	}
	for _, value := range slices.Sorted(maps.Keys(cases)) {
		err := s.VisitJSON(value, openapi3.EnableJSONSchema2020())
		if (err == nil) != cases[value] {
			t.Errorf("outcomes %q: valid = %t, want %t (%v)", value, err == nil, cases[value], err)
		}
	}
}

// reasonItem is one item of the reason list as the contract writes it.
type reasonItem struct {
	Code  model.ReasonCode  `json:"code"`
	Level model.ReasonLevel `json:"level"`
	Scope model.ReasonScope `json:"scope"`
}

// reasonItems decodes a ReasonList body, rejecting other fields.
func reasonItems(t *testing.T, data []byte) []reasonItem {
	t.Helper()
	var body struct {
		Items []reasonItem `json:"items"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("decode reason list: %v", err)
	}
	return body.Items
}

func marshalReasons(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(api.Reasons())
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReasonsListsEveryModelCode(t *testing.T) {
	want := model.Reasons()
	items := reasonItems(t, marshalReasons(t))
	var got []model.ReasonCode
	for _, item := range items {
		got = append(got, item.Code)
		spec, ok := want[item.Code]
		if !ok {
			t.Errorf("%q is not a model reason code", item.Code)
			continue
		}
		if item.Level != spec.Level || item.Scope != spec.Scope {
			t.Errorf("%s: level %q scope %q, want %q %q", item.Code, item.Level, item.Scope, spec.Level, spec.Scope)
		}
	}
	if codes := slices.Sorted(maps.Keys(want)); !slices.Equal(got, codes) {
		t.Errorf("codes = %v\nwant each once, sorted: %v", got, codes)
	}
}

// The order must not depend on map iteration.
func TestReasonsIsDeterministic(t *testing.T) {
	first := marshalReasons(t)
	for range 20 {
		if next := marshalReasons(t); !bytes.Equal(first, next) {
			t.Fatalf("api.Reasons() differs between calls:\n%s\n%s", first, next)
		}
	}
}

func TestReasonsValidatesAgainstItsSchema(t *testing.T) {
	ref := loadSpec(t).Components.Schemas["ReasonList"]
	if ref == nil || ref.Value == nil {
		t.Fatal("components.schemas.ReasonList is missing")
	}
	err := ref.Value.VisitJSON(decodeAny(t, marshalReasons(t)),
		openapi3.EnableJSONSchema2020(), openapi3.MultiErrors())
	if err != nil {
		t.Errorf("api.Reasons() does not validate: %v", err)
	}
}

// An example may show a part of the list, never a wrong entry.
func TestReasonExamplesAgreeWithTheModel(t *testing.T) {
	want := model.Reasons()
	for _, e := range responseExamples(t, loadSpec(t)) {
		if e.op.id != "listReasons" || e.status != http.StatusOK {
			continue
		}
		var previous model.ReasonCode
		for _, item := range reasonItems(t, []byte(compact(e.value))) {
			spec, ok := want[item.Code]
			if !ok || item.Level != spec.Level || item.Scope != spec.Scope {
				t.Errorf("%s: %+v does not match the model", e, item)
			}
			if item.Code <= previous {
				t.Errorf("%s: %q is out of order", e, item.Code)
			}
			previous = item.Code
		}
	}
}

func TestSpecIsTheEmbeddedFile(t *testing.T) {
	file, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(file) == 0 || !bytes.Equal(api.Spec, file) {
		t.Errorf("api.Spec holds %d bytes, openapi.yaml %d; they must be equal", len(api.Spec), len(file))
	}
}
