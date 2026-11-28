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
	// useless marks omitempty on a type it never omits.
	useless bool
}

// canBeEmpty reports whether omitempty can ever leave out a value of typ.
func canBeEmpty(typ reflect.Type) bool {
	switch typ.Kind() {
	case reflect.Struct:
		return false
	case reflect.Array:
		return typ.Len() == 0
	}
	return true
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
		omitEmpty, omitZero := slices.Contains(opts, "omitempty"), slices.Contains(opts, "omitzero")
		out[name] = jsonField{
			typ:      f.Type,
			optional: omitZero || omitEmpty && canBeEmpty(f.Type),
			useless:  omitEmpty && !omitZero && !canBeEmpty(f.Type),
		}
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
	// leaf, when set, sees every schema of a scalar Go type.
	leaf func(at string, typ reflect.Type, s *openapi3.Schema)
	// outer is the description of the array or map around the schema being
	// walked, "" for a plain property.
	outer string
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

// statedTypes returns the types s names itself or through its allOf parts,
// sorted and without repeats.
func statedTypes(s *openapi3.Schema) []string {
	var out []string
	if s.Type != nil {
		out = slices.Clone(s.Type.Slice())
	}
	for _, part := range s.AllOf {
		if part.Value != nil {
			out = append(out, statedTypes(part.Value)...)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// offersNull reports whether s or a branch of it names null as a type or value.
func offersNull(s *openapi3.Schema) bool {
	if (s.Type != nil && slices.Contains(s.Type.Slice(), "null")) || slices.Contains(s.Enum, nil) {
		return true
	}
	return slices.ContainsFunc(slices.Concat(s.AllOf, s.AnyOf, s.OneOf), func(part *openapi3.SchemaRef) bool {
		return part.Value != nil && offersNull(part.Value)
	})
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
	if got := statedTypes(s); !slices.Equal(got, []string{want}) {
		t.Errorf("%s: schema types %v, Go type %s is %s", at, got, typ, want)
		return
	}
	if s.Nullable || s.VisitJSON(nil, openapi3.EnableJSONSchema2020()) == nil || offersNull(s) {
		t.Errorf("%s: schema allows null", at)
	}
	switch want {
	default:
		if w.leaf != nil {
			w.leaf(at, typ, s)
		}
	case "array":
		w.outer = s.Description
		w.walk(at+"[]", typ.Elem(), s.Items)
	case "object":
		if typ.Kind() != reflect.Map {
			w.object(at, typ, ref)
			return
		}
		if values := s.AdditionalProperties.Schema; values != nil {
			w.outer = s.Description
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
	checkUnsetEncoding(t, at, typ, sh.required)
	for _, name := range want {
		f := fields[name]
		if f.useless {
			t.Errorf("%s.%s: omitempty never omits a %s", at, name, f.typ)
		}
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
		w.outer = ""
		w.walk(at+"."+name, f.typ, prop)
	}
}

// checkUnsetEncoding encodes a value of typ with nothing set but its lists
// and maps that are always written, and demands exactly the required fields,
// none of them null.
func checkUnsetEncoding(t *testing.T, at string, typ reflect.Type, required []string) {
	t.Helper()
	value := reflect.New(typ).Elem()
	fields := jsonFields(typ)
	for _, f := range reflect.VisibleFields(typ) {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || f.Anonymous || fields[name].optional {
			continue
		}
		switch f.Type.Kind() {
		case reflect.Slice:
			value.FieldByIndex(f.Index).Set(reflect.MakeSlice(f.Type, 0, 0))
		case reflect.Map:
			value.FieldByIndex(f.Index).Set(reflect.MakeMap(f.Type))
		}
	}
	data, err := json.Marshal(value.Addr().Interface())
	if err != nil {
		t.Errorf("%s: encode an unset %s: %v", at, typ, err)
		return
	}
	object, ok := decodeAny(t, data).(map[string]any)
	if !ok {
		t.Errorf("%s: an unset %s encodes as %s, not an object", at, typ, data)
		return
	}
	got, want := slices.Sorted(maps.Keys(object)), slices.Sorted(slices.Values(required))
	want = slices.Compact(want)
	if !slices.Equal(got, want) {
		t.Errorf("%s: an unset %s writes %v, the schema requires %v", at, typ, got, want)
	}
	for _, key := range got {
		// A required pointer has no unset value that is not null.
		if f := fields[key].typ; f != nil && f.Kind() != reflect.Pointer && object[key] == nil {
			t.Errorf("%s.%s: an unset %s writes null", at, key, typ)
		}
	}
}

// The fields AnalysisSummary shares with model.Analysis keep the model's type
// and optionality; findings and problems are envelope lists, always written.
func TestSummarySharedFieldsFollowTheModel(t *testing.T) {
	ref := loadSpec(t).Components.Schemas["AnalysisSummary"]
	if ref == nil || ref.Value == nil {
		t.Fatal("components.schemas.AnalysisSummary is missing")
	}
	required := shapeOf(ref.Value).required
	summary := jsonFields(topLevelTypes["AnalysisSummary"])
	analysis := jsonFields(reflect.TypeFor[model.Analysis]())
	shared := []string{
		"id", "kind", "environment", "status", "createdAt", "fingerprint", "toolVersion", "schemaVersion",
		"algorithmVersion", "ruleSetVersion", "inputs", "profile", "limits", "unexamined", "endpoints",
	}
	for _, name := range shared {
		want, inModel := analysis[name]
		got, inSummary := summary[name]
		if !inModel || !inSummary {
			t.Errorf("%s: in model.Analysis %t, in AnalysisSummary %t", name, inModel, inSummary)
			continue
		}
		if got.typ != want.typ {
			t.Errorf("AnalysisSummary.%s is %s, model.Analysis has %s", name, got.typ, want.typ)
		}
		if got.optional != want.optional {
			t.Errorf("AnalysisSummary.%s: optional = %t, in model.Analysis %t", name, got.optional, want.optional)
		}
		if slices.Contains(required, name) == want.optional {
			t.Errorf("AnalysisSummary.%s: required must be %t, as model.Analysis writes it", name, !want.optional)
		}
	}
}

// The envelope shapes the API note states, whatever the Go types say. The
// summary's shared fields get their optionality from the model instead.
func TestEnvelopeSchemasFollowTheNote(t *testing.T) {
	type fieldSet struct{ required, optional []string }
	want := map[string]fieldSet{
		"EnvironmentList": {required: []string{"items"}, optional: []string{"next"}},
		"Environment":     {required: []string{"name"}, optional: []string{"baseline", "latestAttempt"}},
		"AnalysisRef":     {required: []string{"id", "status", "createdAt"}},
		"Edge":            {required: []string{"source", "destination", "verdicts"}},
		"EdgeGroup":       {optional: []string{"namespace", "endpoint"}},
		"VerdictCounts":   {required: []string{"allow", "deny", "placement", "unknown"}},
		"PathList":        {required: []string{"items"}, optional: []string{"next"}},
		"ReasonList":      {required: []string{"items"}},
		"Reason":          {required: []string{"code", "level", "scope"}},
		"Error":           {required: []string{"type", "title", "status", "detail", "code"}},
		"AnalysisSummary": {
			required: []string{
				"kind", "environment", "status", "fingerprint", "toolVersion", "schemaVersion",
				"algorithmVersion", "ruleSetVersion", "inputs", "profile", "limits", "unexamined",
				"endpoints", "edges", "findings", "problems",
			},
			optional: []string{"id", "createdAt"},
		},
	}
	doc := loadSpec(t)
	for _, name := range slices.Sorted(maps.Keys(want)) {
		ref := doc.Components.Schemas[name]
		if ref == nil || ref.Value == nil {
			t.Errorf("components.schemas.%s is missing", name)
			continue
		}
		sh := shapeOf(ref.Value)
		all := slices.Sorted(slices.Values(slices.Concat(want[name].required, want[name].optional)))
		if got := slices.Sorted(maps.Keys(sh.properties)); !slices.Equal(got, all) {
			t.Errorf("%s: properties %v, want %v", name, got, all)
		}
		required := slices.Compact(slices.Sorted(slices.Values(sh.required)))
		if wantRequired := slices.Sorted(slices.Values(want[name].required)); !slices.Equal(required, wantRequired) {
			t.Errorf("%s: required %v, want %v", name, required, wantRequired)
		}
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
