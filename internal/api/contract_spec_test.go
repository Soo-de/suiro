package api_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Soo-de/suiro/internal/api"
	"github.com/Soo-de/suiro/internal/model"
)

const (
	mediaJSON    = "application/json"
	mediaProblem = "application/problem+json"
	schemaPrefix = "#/components/schemas/"
)

// opContract is what the API note states about one operation.
type opContract struct {
	id     string
	method string
	path   string
	// schema is the component of the 200 body.
	schema string
	// errors maps each error status the operation must define to the codes
	// that status can carry there.
	errors map[int][]string
}

func contracts() []opContract {
	internal := []string{"internal-error"}
	// invalid covers, among others, a malformed {id}.
	invalid := []string{"invalid-parameter"}
	return []opContract{
		{
			id: "listEnvironments", method: http.MethodGet, path: "/api/v1/environments", schema: "EnvironmentList",
			errors: map[int][]string{500: internal},
		},
		{
			id: "getAnalysis", method: http.MethodGet, path: "/api/v1/analyses/{id}", schema: "AnalysisSummary",
			errors: map[int][]string{400: invalid, 404: {"analysis-not-found"}, 500: internal},
		},
		{
			id: "listPaths", method: http.MethodGet, path: "/api/v1/analyses/{id}/paths", schema: "PathList",
			errors: map[int][]string{
				400: {"invalid-parameter", "scope-required"},
				404: {"analysis-not-found", "endpoint-not-found"},
				409: {"analysis-failed"},
				500: internal,
			},
		},
		{
			id: "getPathDetail", method: http.MethodGet, path: "/api/v1/analyses/{id}/paths/detail", schema: "PathDetail",
			errors: map[int][]string{
				400: invalid,
				404: {"analysis-not-found", "endpoint-not-found", "path-not-found"},
				409: {"analysis-failed"},
				410: {"explain-unavailable"},
				500: internal,
				503: {"explain-timeout"},
			},
		},
		{
			id: "listReasons", method: http.MethodGet, path: "/api/v1/reasons", schema: "ReasonList",
			errors: map[int][]string{500: internal},
		},
	}
}

func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	if len(api.Spec) == 0 {
		t.Fatal("api.Spec is empty")
	}
	doc, err := openapi3.NewLoader().LoadFromData(api.Spec)
	if err != nil {
		t.Fatalf("load api.Spec: %v", err)
	}
	return doc
}

// operationOf returns the operation c names, failing the test when it is
// missing.
func operationOf(t *testing.T, doc *openapi3.T, c opContract) (*openapi3.PathItem, *openapi3.Operation) {
	t.Helper()
	if doc.Paths == nil {
		t.Fatal("spec has no paths")
	}
	item := doc.Paths.Value(c.path)
	if item == nil {
		t.Fatalf("spec has no path %s", c.path)
	}
	op := item.GetOperation(c.method)
	if op == nil {
		t.Fatalf("spec has no %s %s", c.method, c.path)
	}
	return item, op
}

// example is one named example of one response.
type example struct {
	op     opContract
	status int
	media  string
	name   string
	schema *openapi3.SchemaRef
	// value is the example as encoding/json decodes it.
	value any
}

func (e example) String() string {
	return e.op.id + " " + strconv.Itoa(e.status) + " " + e.media + " example " + e.name
}

// responseExamples returns every named example of every response of the
// five operations, in a stable order.
func responseExamples(t *testing.T, doc *openapi3.T) []example {
	t.Helper()
	var out []example
	for _, c := range contracts() {
		_, op := operationOf(t, doc, c)
		if op.Responses == nil {
			t.Fatalf("%s has no responses", c.id)
		}
		responses := op.Responses.Map()
		for _, code := range slices.Sorted(maps.Keys(responses)) {
			status, err := strconv.Atoi(code)
			if err != nil || responses[code].Value == nil {
				t.Errorf("%s: response %q is not a resolved numeric status", c.id, code)
				continue
			}
			content := responses[code].Value.Content
			for _, media := range slices.Sorted(maps.Keys(content)) {
				m := content[media]
				for _, name := range slices.Sorted(maps.Keys(m.Examples)) {
					ref := m.Examples[name]
					if ref == nil || ref.Value == nil || ref.Value.Value == nil {
						t.Errorf("%s %s %s example %q has no value", c.id, code, media, name)
						continue
					}
					out = append(out, example{
						op: c, status: status, media: media, name: name, schema: m.Schema,
						value: asJSON(t, ref.Value.Value),
					})
				}
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("the spec has no response examples")
	}
	return out
}

// asJSON turns v into the value encoding/json would decode from its JSON.
func asJSON(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	return decodeAny(t, data)
}

func decodeAny(t *testing.T, data []byte) any {
	t.Helper()
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return out
}

func compact(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

func readNote(t *testing.T, name string) []byte {
	t.Helper()
	data, err := fs.ReadFile(os.DirFS(filepath.Join("testdata", "note")), name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSpecIsValidOpenAPI31(t *testing.T) {
	doc := loadSpec(t)
	if doc.OpenAPI != "3.1.0" {
		t.Errorf("openapi = %q, want 3.1.0", doc.OpenAPI)
	}
	if err := doc.Validate(context.Background(), openapi3.EnableExamplesValidation()); err != nil {
		t.Errorf("spec is invalid: %v", err)
	}
}

func TestSpecOperations(t *testing.T) {
	doc := loadSpec(t)
	if doc.Paths == nil {
		t.Fatal("spec has no paths")
	}
	want := map[string]string{}
	for _, c := range contracts() {
		want[c.method+" "+c.path] = c.id
	}
	got := map[string]string{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			got[method+" "+path] = op.OperationID
		}
	}
	if !maps.Equal(got, want) {
		t.Errorf("operations = %v, want %v", got, want)
	}
}

type paramWant struct {
	in       string
	required bool
	// kind is the schema type; "array" takes items of itemKind.
	kind     string
	itemKind string
}

func TestSpecParameters(t *testing.T) {
	id := paramWant{in: "path", required: true, kind: "integer"}
	text := paramWant{in: "query", kind: "string"}
	required := paramWant{in: "query", required: true, kind: "string"}
	repeated := paramWant{in: "query", kind: "array", itemKind: "string"}
	want := map[string]map[string]paramWant{
		"listEnvironments": {},
		"getAnalysis":      {"id": id},
		"listPaths": {
			"id": id, "namespace": repeated, "endpoint": text, "source": text, "destination": text,
			"ports": text, "verdict": repeated, "limit": {in: "query", kind: "integer"}, "cursor": text,
		},
		"getPathDetail": {"id": id, "source": required, "destination": required, "ports": required},
		"listReasons":   {},
	}
	doc := loadSpec(t)
	for _, c := range contracts() {
		t.Run(c.id, func(t *testing.T) {
			item, op := operationOf(t, doc, c)
			got := map[string]*openapi3.Parameter{}
			for _, ref := range append(slices.Clone(item.Parameters), op.Parameters...) {
				if ref.Value == nil {
					t.Fatalf("unresolved parameter %q", ref.Ref)
				}
				got[ref.Value.Name] = ref.Value
			}
			names, wantNames := slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want[c.id]))
			if !slices.Equal(names, wantNames) {
				t.Fatalf("parameters = %v, want %v", names, wantNames)
			}
			for name, w := range want[c.id] {
				checkParameter(t, got[name], w)
			}
		})
	}
}

func checkParameter(t *testing.T, p *openapi3.Parameter, w paramWant) {
	t.Helper()
	if p.In != w.in || p.Required != w.required {
		t.Errorf("%s: in=%s required=%t, want in=%s required=%t", p.Name, p.In, p.Required, w.in, w.required)
	}
	if p.Schema == nil || p.Schema.Value == nil {
		t.Errorf("%s: no schema", p.Name)
		return
	}
	s := p.Schema.Value
	if !s.Type.Is(w.kind) {
		t.Errorf("%s: type = %v, want %s", p.Name, s.Type, w.kind)
		return
	}
	if w.kind != "array" {
		return
	}
	if s.Items == nil || s.Items.Value == nil || !s.Items.Value.Type.Is(w.itemKind) {
		t.Errorf("%s: items must be of type %s", p.Name, w.itemKind)
	}
	// A repeatable parameter is written name=a&name=b.
	if (p.Explode != nil && !*p.Explode) || (p.Style != "" && p.Style != "form") {
		t.Errorf("%s: style %q must repeat the parameter", p.Name, p.Style)
	}
}

func TestSpecListPathsLimit(t *testing.T) {
	doc := loadSpec(t)
	item, op := operationOf(t, doc, contracts()[2])
	for _, ref := range append(slices.Clone(item.Parameters), op.Parameters...) {
		if ref.Value == nil || ref.Value.Name != "limit" || ref.Value.Schema == nil {
			continue
		}
		s := ref.Value.Schema.Value
		if s.Default == nil || compact(s.Default) != "200" {
			t.Errorf("limit default = %v, want 200", s.Default)
		}
		for value, valid := range map[float64]bool{0: false, 1: true, 200: true, 1000: true, 1001: false} {
			err := s.VisitJSON(value, openapi3.EnableJSONSchema2020())
			if (err == nil) != valid {
				t.Errorf("limit %v: valid = %t, want %t (%v)", value, err == nil, valid, err)
			}
		}
		return
	}
	t.Fatal("listPaths has no limit parameter with a schema")
}

func TestSpecResponses(t *testing.T) {
	doc := loadSpec(t)
	for _, c := range contracts() {
		t.Run(c.id, func(t *testing.T) {
			_, op := operationOf(t, doc, c)
			if op.Responses == nil {
				t.Fatal("no responses")
			}
			responses := op.Responses.Map()
			required := append(slices.Collect(maps.Keys(c.errors)), http.StatusOK)
			for _, status := range required {
				if responses[strconv.Itoa(status)] == nil {
					t.Errorf("response %d is missing", status)
				}
			}
			for code, ref := range responses {
				status, err := strconv.Atoi(code)
				if err != nil || !slices.Contains(required, status) {
					t.Errorf("response %q is not one the API note gives this operation", code)
					continue
				}
				if ref.Value == nil {
					t.Errorf("response %s is unresolved", code)
					continue
				}
				media, schema := mediaJSON, c.schema
				if status != http.StatusOK {
					media, schema = mediaProblem, "Error"
				}
				checkContent(t, code, ref.Value.Content, media, schema)
			}
		})
	}
}

// checkContent demands one media type, its component schema and at least one
// named example.
func checkContent(t *testing.T, code string, content openapi3.Content, media, schema string) {
	t.Helper()
	got := slices.Sorted(maps.Keys(content))
	if !slices.Equal(got, []string{media}) {
		t.Errorf("response %s: media types = %v, want [%s]", code, got, media)
		return
	}
	m := content[media]
	if m.Schema == nil || m.Schema.Ref != schemaPrefix+schema {
		t.Errorf("response %s: schema must be $ref %s%s", code, schemaPrefix, schema)
	}
	if len(m.Examples) == 0 {
		t.Errorf("response %s has no named example", code)
	}
	if m.Example != nil {
		t.Errorf("response %s uses the unnamed example field", code)
	}
}

func TestExamplesValidateAgainstTheirSchemas(t *testing.T) {
	doc := loadSpec(t)
	for _, e := range responseExamples(t, doc) {
		if e.schema == nil || e.schema.Value == nil {
			t.Errorf("%s: no schema to validate against", e)
			continue
		}
		err := e.schema.Value.VisitJSON(e.value, openapi3.EnableJSONSchema2020(), openapi3.MultiErrors())
		if err != nil {
			t.Errorf("%s: %v", e, err)
		}
	}
}

// A schema that accepts anything would make the example validation vacuous.
func TestSchemasRejectBadBodies(t *testing.T) {
	doc := loadSpec(t)
	bad := []any{nil, "text", []any{}, map[string]any{}}
	for _, name := range []string{"EnvironmentList", "AnalysisSummary", "PathList", "PathDetail", "ReasonList", "Error"} {
		ref := doc.Components.Schemas[name]
		if ref == nil || ref.Value == nil {
			t.Errorf("components.schemas.%s is missing", name)
			continue
		}
		for _, value := range bad {
			if ref.Value.VisitJSON(value, openapi3.EnableJSONSchema2020()) == nil {
				t.Errorf("%s accepts %s", name, compact(value))
			}
		}
	}
}

func TestErrorExamplesFollowTheStatusTable(t *testing.T) {
	for _, e := range responseExamples(t, loadSpec(t)) {
		if e.status == http.StatusOK {
			continue
		}
		var body struct {
			Type   string `json:"type"`
			Title  string `json:"title"`
			Status int    `json:"status"`
			Detail string `json:"detail"`
			Code   string `json:"code"`
		}
		if err := json.Unmarshal([]byte(compact(e.value)), &body); err != nil {
			t.Errorf("%s: %v", e, err)
			continue
		}
		codes := e.op.errors[e.status]
		if !slices.Contains(codes, body.Code) {
			t.Errorf("%s: code %q, want one of %v", e, body.Code, codes)
		}
		if body.Type != "about:blank" || body.Status != e.status || body.Title != http.StatusText(e.status) {
			t.Errorf("%s: type=%q status=%d title=%q, want about:blank, %d, %q",
				e, body.Type, body.Status, body.Title, e.status, http.StatusText(e.status))
		}
		if body.Detail == "" {
			t.Errorf("%s: empty detail", e)
		}
	}
}

// noteExamples maps each example response of the API note to its operation.
var noteExamples = map[string]string{
	"environments.json":            "listEnvironments",
	"analysis-42.json":             "getAnalysis",
	"analysis-43.json":             "getAnalysis",
	"paths-destination-page1.json": "listPaths",
	"paths-destination-page2.json": "listPaths",
	"paths-namespace.json":         "listPaths",
	"detail-allow.json":            "getPathDetail",
	"detail-deny.json":             "getPathDetail",
	"detail-unknown.json":          "getPathDetail",
}

func TestSpecCarriesTheNoteExamples(t *testing.T) {
	examples := responseExamples(t, loadSpec(t))
	for _, file := range slices.Sorted(maps.Keys(noteExamples)) {
		t.Run(file, func(t *testing.T) {
			op := noteExamples[file]
			want := decodeAny(t, readNote(t, file))
			found := slices.ContainsFunc(examples, func(e example) bool {
				if e.op.id != op || e.status != http.StatusOK {
					return false
				}
				got := e.value
				if object, ok := got.(map[string]any); ok && op == "getPathDetail" {
					// The note predates gateOrigins, which has its own test.
					object = maps.Clone(object)
					delete(object, "gateOrigins")
					got = object
				}
				return reflect.DeepEqual(got, want)
			})
			if !found {
				t.Errorf("no 200 example of %s equals testdata/note/%s", op, file)
			}
		})
	}
}

// gateOrigin is the origin of a gate whose object is declared at the top of
// file. A NetworkPolicy gate adds its direction to the object key.
func gateOrigin(object model.ObjectKey, direction string, source model.SourceID, file string) model.GateOrigin {
	id := model.GateID(object)
	if direction != "" {
		id += model.GateID("#" + direction)
	}
	return model.GateOrigin{ID: id, Object: object, Origin: model.Origin{Source: source, File: file, Line: 1}}
}

// The gate origins of the note's scenario, by the verdict of its three detail
// examples, sorted by ID. See model.PathDetail.GateOrigins.
func noteGateOrigins() map[model.Verdict][]model.GateOrigin {
	service := gateOrigin("k8s:core/Service/data/postgres", "",
		"data/orders-db", "deploy/orders-db/service.yaml")
	postgres := gateOrigin("k8s:networking.k8s.io/NetworkPolicy/data/postgres", "ingress",
		"data/orders-db", "deploy/orders-db/netpol-postgres.yaml")
	apiPolicy := gateOrigin("k8s:networking.k8s.io/NetworkPolicy/shop/api", "egress",
		"shop/storefront", "deploy/storefront/netpol-api.yaml")
	deny := gateOrigin("k8s:networking.k8s.io/NetworkPolicy/shop/default-deny", "egress",
		"shop/storefront", "deploy/storefront/netpol-default-deny.yaml")
	web := gateOrigin("k8s:networking.k8s.io/NetworkPolicy/shop/web", "egress",
		"shop/storefront", "deploy/storefront/netpol-web.yaml")
	return map[model.Verdict][]model.GateOrigin{
		model.Allow:   {service, postgres, apiPolicy, deny},
		model.Deny:    {service, postgres, deny, web},
		model.Unknown: {service, postgres},
	}
}

func TestDetailExampleGateOrigins(t *testing.T) {
	want := noteGateOrigins()
	seen := map[model.Verdict]bool{}
	for _, e := range responseExamples(t, loadSpec(t)) {
		if e.op.id != "getPathDetail" || e.status != http.StatusOK {
			continue
		}
		var detail model.PathDetail
		if err := json.Unmarshal([]byte(compact(e.value)), &detail); err != nil {
			t.Errorf("%s: %v", e, err)
			continue
		}
		seen[detail.Verdict] = true
		if !slices.Equal(detail.GateOrigins, want[detail.Verdict]) {
			t.Errorf("%s: gateOrigins\n got %s\nwant %s",
				e, compact(detail.GateOrigins), compact(want[detail.Verdict]))
		}
	}
	for verdict := range want {
		if !seen[verdict] {
			t.Errorf("no detail example with verdict %s", verdict)
		}
	}
}

// parameterSchema returns the schema of the named parameter of c.
func parameterSchema(t *testing.T, doc *openapi3.T, c opContract, name string) *openapi3.Schema {
	t.Helper()
	item, op := operationOf(t, doc, c)
	for _, ref := range slices.Concat(item.Parameters, op.Parameters) {
		if p := ref.Value; p != nil && p.Name == name && p.Schema != nil && p.Schema.Value != nil {
			return p.Schema.Value
		}
	}
	t.Fatalf("%s has no %s parameter with a schema", c.id, name)
	return nil
}

// checkValues demands that s accepts every value of valid and no other.
func checkValues(t *testing.T, label string, s *openapi3.Schema, valid, invalid []any) {
	t.Helper()
	for _, value := range valid {
		if err := s.VisitJSON(value, openapi3.EnableJSONSchema2020()); err != nil {
			t.Errorf("%s rejects %s: %v", label, compact(value), err)
		}
	}
	for _, value := range invalid {
		if s.VisitJSON(value, openapi3.EnableJSONSchema2020()) == nil {
			t.Errorf("%s accepts %s", label, compact(value))
		}
	}
}

// The ports grammar is that of model.ParsePathKey: tcp or udp, one port or a
// range, decimal digits without sign or leading zero.
func TestSpecPortsParameter(t *testing.T) {
	valid := []any{"tcp/5432", "udp/53", "tcp/30000-32767", "tcp/0", "udp/0-65535", "tcp/0-5431", "tcp/65535"}
	invalid := []any{
		"", "tcp", "tcp/", "5432", "/5432", "all/53", "sctp/53", "TCP/53", "tcp:53", "tcp/http",
		"tcp/05432", "tcp/00", "tcp/1-02", "tcp/-1", "tcp/+1", "tcp/1-", "tcp/1-2-3", "tcp/100000",
		"tcp/1-100000", "tcp/53,udp/53", " tcp/53", "tcp/53 ", "xtcp/53", "tcp/53x", 53.0,
	}
	doc := loadSpec(t)
	for _, c := range contracts() {
		if c.id == "listPaths" || c.id == "getPathDetail" {
			checkValues(t, c.id+" ports", parameterSchema(t, doc, c, "ports"), valid, invalid)
		}
	}
}

// Analysis numbers start at 1; 0 is an analysis not yet stored.
func TestSpecAnalysisID(t *testing.T) {
	valid := []any{1.0, 42.0, float64(1 << 53)}
	invalid := []any{0.0, -1.0, 1.5, "42", "abc", ""}
	doc := loadSpec(t)
	for _, c := range contracts() {
		if strings.Contains(c.path, "{id}") {
			checkValues(t, c.id+" id", parameterSchema(t, doc, c, "id"), valid, invalid)
		}
	}
}
