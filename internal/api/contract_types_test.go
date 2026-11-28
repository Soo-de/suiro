package api_test

import (
	"bytes"
	"encoding"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Soo-de/suiro/internal/api"
	"github.com/Soo-de/suiro/internal/model"
)

// topLevelTypes maps each top-level response schema to its Go type.
var topLevelTypes = map[string]reflect.Type{
	"EnvironmentList": reflect.TypeFor[api.EnvironmentList](),
	"AnalysisSummary": reflect.TypeFor[api.AnalysisSummary](),
	"PathList":        reflect.TypeFor[api.PathList](),
	"PathDetail":      reflect.TypeFor[model.PathDetail](),
	"ReasonList":      reflect.TypeFor[api.ReasonList](),
	"Error":           reflect.TypeFor[api.Error](),
}

// roundTrip decodes data into a new value of type typ, rejecting unknown
// fields, and fails unless encoding it gives the same JSON value.
func roundTrip(t *testing.T, label string, typ reflect.Type, data []byte) {
	t.Helper()
	value := reflect.New(typ).Interface()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		t.Errorf("%s: decode into %s: %v", label, typ, err)
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		t.Errorf("%s: data after the JSON value", label)
	}
	out, err := json.Marshal(value)
	if err != nil {
		t.Errorf("%s: encode %s: %v", label, typ, err)
		return
	}
	want, got := decodeAny(t, data), decodeAny(t, out)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: %s changes the value\n got %s\nwant %s", label, typ, compact(got), compact(want))
	}
}

func TestExamplesRoundTripThroughGoTypes(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range responseExamples(t, loadSpec(t)) {
		name := e.op.schema
		if e.status != http.StatusOK {
			name = "Error"
		}
		seen[name] = true
		roundTrip(t, e.String(), topLevelTypes[name], []byte(compact(e.value)))
	}
	for name := range topLevelTypes {
		if !seen[name] {
			t.Errorf("no example of schema %s", name)
		}
	}
}

// The note's examples are the expected bodies, whatever the spec carries.
func TestNoteExamplesRoundTripThroughGoTypes(t *testing.T) {
	doc := loadSpec(t)
	schemas := map[string]string{}
	for _, c := range contracts() {
		schemas[c.id] = c.schema
	}
	for _, file := range slices.Sorted(maps.Keys(noteExamples)) {
		name := schemas[noteExamples[file]]
		data := readNote(t, file)
		roundTrip(t, file, topLevelTypes[name], data)
		ref := doc.Components.Schemas[name]
		if ref == nil || ref.Value == nil {
			t.Errorf("components.schemas.%s is missing", name)
			continue
		}
		if err := ref.Value.VisitJSON(decodeAny(t, data), openapi3.EnableJSONSchema2020()); err != nil {
			t.Errorf("%s does not validate against %s: %v", file, name, err)
		}
	}
}

// nullPaths returns where value holds null.
func nullPaths(value any, at string) []string {
	var out []string
	switch v := value.(type) {
	case nil:
		out = append(out, at)
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(v)) {
			out = append(out, nullPaths(v[key], at+"."+key)...)
		}
	case []any:
		for _, item := range v {
			out = append(out, nullPaths(item, at+"[]")...)
		}
	}
	return out
}

func TestExamplesHoldNoNull(t *testing.T) {
	for _, e := range responseExamples(t, loadSpec(t)) {
		if found := nullPaths(e.value, "$"); len(found) > 0 {
			t.Errorf("%s: null at %v", e, found)
		}
	}
}

// jsonField is one field of a struct as encoding/json writes it.
type jsonField struct {
	typ      reflect.Type
	optional bool
}

// jsonFields returns the fields of the struct typ by JSON name, with
// embedded structs flattened.
func jsonFields(typ reflect.Type) map[string]jsonField {
	out := map[string]jsonField{}
	for i := range typ.NumField() {
		f := typ.Field(i)
		tag := f.Tag.Get("json")
		name, options, _ := strings.Cut(tag, ",")
		if tag == "-" {
			continue
		}
		if f.Anonymous && name == "" {
			inner := f.Type
			if inner.Kind() == reflect.Pointer {
				inner = inner.Elem()
			}
			if inner.Kind() == reflect.Struct {
				maps.Copy(out, jsonFields(inner))
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		opts := strings.Split(options, ",")
		out[name] = jsonField{typ: f.Type, optional: slices.Contains(opts, "omitempty") || slices.Contains(opts, "omitzero")}
	}
	return out
}

// shape is what an object schema says once its allOf parts are merged.
type shape struct {
	properties map[string]*openapi3.SchemaRef
	required   []string
}

func shapeOf(s *openapi3.Schema) shape {
	out := shape{properties: map[string]*openapi3.SchemaRef{}, required: slices.Clone(s.Required)}
	maps.Copy(out.properties, s.Properties)
	for _, part := range s.AllOf {
		if part.Value != nil {
			inner := shapeOf(part.Value)
			maps.Copy(out.properties, inner.properties)
			out.required = append(out.required, inner.required...)
		}
	}
	return out
}

// schemaWalker compares schemas with the Go types they describe, visiting
// each pair once.
type schemaWalker struct {
	t       *testing.T
	visited map[string]bool
}

// jsonType is the JSON Schema type of a Go type that is not a pointer.
func jsonType(typ reflect.Type) string {
	text := reflect.TypeFor[encoding.TextMarshaler]()
	if typ.Implements(text) || reflect.PointerTo(typ).Implements(text) {
		return "string"
	}
	switch typ.Kind() {
	case reflect.Struct, reflect.Map:
		return "object"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	}
	return typ.Kind().String()
}

func (w *schemaWalker) walk(at string, typ reflect.Type, ref *openapi3.SchemaRef) {
	t := w.t
	t.Helper()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if ref == nil || ref.Value == nil {
		t.Errorf("%s: no schema for %s", at, typ)
		return
	}
	s := ref.Value
	want := jsonType(typ)
	// A schema may give its type through allOf, so only a stated type is compared.
	if s.Type != nil && !slices.Equal(s.Type.Slice(), []string{want}) {
		t.Errorf("%s: schema type %v, Go type %s is %s", at, s.Type.Slice(), typ, want)
		return
	}
	if s.Nullable {
		t.Errorf("%s: schema is nullable", at)
	}
	switch want {
	case "array":
		w.walk(at+"[]", typ.Elem(), s.Items)
	case "object":
		if typ.Kind() != reflect.Map {
			w.object(at, typ, ref)
			return
		}
		if values := s.AdditionalProperties.Schema; values != nil {
			w.walk(at+"{}", typ.Elem(), values)
		} else {
			t.Errorf("%s: map schema needs additionalProperties", at)
		}
	}
}

func (w *schemaWalker) object(at string, typ reflect.Type, ref *openapi3.SchemaRef) {
	t := w.t
	t.Helper()
	// Model types keep their names in the spec.
	if typ.PkgPath() == reflect.TypeFor[model.Path]().PkgPath() && ref.Ref != schemaPrefix+typ.Name() {
		t.Errorf("%s: model type %s must be $ref %s%s, got %q", at, typ.Name(), schemaPrefix, typ.Name(), ref.Ref)
	}
	if ref.Ref != "" {
		key := ref.Ref + " " + typ.String()
		if w.visited[key] {
			return
		}
		w.visited[key] = true
		at = typ.Name()
	}
	sh := shapeOf(ref.Value)
	fields := jsonFields(typ)
	got, want := slices.Sorted(maps.Keys(sh.properties)), slices.Sorted(maps.Keys(fields))
	if !slices.Equal(got, want) {
		t.Errorf("%s: schema properties %v, fields of %s %v", at, got, typ, want)
	}
	for _, name := range want {
		f := fields[name]
		prop, ok := sh.properties[name]
		if !ok {
			continue
		}
		required := slices.Contains(sh.required, name)
		if !f.optional && !required {
			t.Errorf("%s.%s: always written by %s, so it must be required", at, name, typ)
		}
		if f.optional && required {
			t.Errorf("%s.%s: omitted by %s when empty, so it must be optional", at, name, typ)
		}
		w.walk(at+"."+name, f.typ, prop)
	}
}

func TestSchemasMatchGoTypes(t *testing.T) {
	doc := loadSpec(t)
	w := &schemaWalker{t: t, visited: map[string]bool{}}
	for _, name := range slices.Sorted(maps.Keys(topLevelTypes)) {
		ref := doc.Components.Schemas[name]
		if ref == nil || ref.Value == nil {
			t.Errorf("components.schemas.%s is missing", name)
			continue
		}
		w.walk(name, topLevelTypes[name], &openapi3.SchemaRef{Ref: schemaPrefix + name, Value: ref.Value})
	}
}

// The envelope lists are always written, even where the model omits them.
func TestEnvelopeListsAreAlwaysPresent(t *testing.T) {
	lists := map[string][]string{
		"EnvironmentList": {"items"},
		"AnalysisSummary": {"endpoints", "edges", "findings", "problems"},
		"PathList":        {"items"},
		"ReasonList":      {"items"},
	}
	doc := loadSpec(t)
	for _, name := range slices.Sorted(maps.Keys(lists)) {
		ref := doc.Components.Schemas[name]
		if ref == nil || ref.Value == nil {
			t.Errorf("components.schemas.%s is missing", name)
			continue
		}
		required := shapeOf(ref.Value).required
		fields := jsonFields(topLevelTypes[name])
		for _, list := range lists[name] {
			if !slices.Contains(required, list) {
				t.Errorf("%s.%s must be required", name, list)
			}
			f, ok := fields[list]
			if !ok || f.optional || f.typ.Kind() != reflect.Slice {
				t.Errorf("%s.%s must be a Go slice without omitempty", name, list)
			}
		}
	}
}

// The summary leaves out the single paths and the pull request fields.
func TestSummaryLeavesOutPaths(t *testing.T) {
	fields := jsonFields(topLevelTypes["AnalysisSummary"])
	for _, name := range []string{"paths", "changes", "base", "pullRequest"} {
		if _, ok := fields[name]; ok {
			t.Errorf("AnalysisSummary must not have %q", name)
		}
	}
}
