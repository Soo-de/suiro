package model_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Soo-de/suiro/internal/model"
)

// GateOrigins is looked up by reflection, so a missing field fails these tests
// alone instead of stopping the package from compiling.
func TestPathDetailJSONGateOriginsField(t *testing.T) {
	f, ok := reflect.TypeFor[model.PathDetail]().FieldByName("GateOrigins")
	if !ok {
		t.Fatal("PathDetail has no field GateOrigins")
	}
	if got, want := f.Tag.Get("json"), "gateOrigins,omitempty"; got != want {
		t.Errorf("GateOrigins json tag = %q, want %q", got, want)
	}
	if f.Type.Kind() != reflect.Slice || f.Type.Elem().Name() != "GateOrigin" {
		t.Fatalf("GateOrigins is %v, want []GateOrigin", f.Type)
	}
	elem := f.Type.Elem()
	if elem.NumField() != 3 {
		t.Errorf("GateOrigin has %d fields, want 3", elem.NumField())
	}
	for _, want := range []struct {
		name, tag string
		typ       reflect.Type
	}{
		{"ID", "id", reflect.TypeFor[model.GateID]()},
		{"Object", "object", reflect.TypeFor[model.ObjectKey]()},
		{"Origin", "origin", reflect.TypeFor[model.Origin]()},
	} {
		g, ok := elem.FieldByName(want.name)
		if !ok {
			t.Errorf("GateOrigin has no field %s", want.name)
			continue
		}
		if g.Type != want.typ || g.Tag.Get("json") != want.tag {
			t.Errorf("GateOrigin.%s is %v with tag %q, want %v with tag %q", want.name, g.Type, g.Tag.Get("json"), want.typ, want.tag)
		}
	}
}

func TestPathDetailJSONWithoutGateOrigins(t *testing.T) {
	d := model.PathDetail{
		Key:     model.PathKey{Source: "internet", Destination: "aws:database/orders-db", Ports: model.PortRange{Protocol: model.ProtocolTCP, From: 5432, To: 5432}},
		Verdict: model.Deny,
		Classes: []model.ClassResult{},
	}
	for _, tt := range []struct {
		name  string
		empty bool
	}{{"nil", false}, {"empty list", true}} {
		t.Run(tt.name, func(t *testing.T) {
			v := d
			if tt.empty {
				f := reflect.ValueOf(&v).Elem().FieldByName("GateOrigins")
				if !f.IsValid() {
					t.Fatal("PathDetail has no field GateOrigins")
				}
				f.Set(reflect.MakeSlice(f.Type(), 0, 0))
			}
			raw, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if got, ok := fields["gateOrigins"]; ok {
				t.Errorf("a PathDetail without gate origins marshals gateOrigins = %s", got)
			}
		})
	}
}

func TestPathDetailJSONWithGateOrigins(t *testing.T) {
	const want = `[{"id":"k8s:networking.k8s.io/NetworkPolicy/payments/allow-db#egress",` +
		`"object":"k8s:networking.k8s.io/NetworkPolicy/payments/allow-db",` +
		`"origin":{"source":"payments/payments-api","file":"deploy/policy.yaml","line":12}}]`
	in := `{"key":{"source":"a","destination":"b","ports":{"protocol":"tcp","from":80,"to":80}},` +
		`"verdict":"deny","classes":[],"gateOrigins":` + want + `}`
	var d model.PathDetail
	if err := json.Unmarshal([]byte(in), &d); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if got := string(fields["gateOrigins"]); got != want {
		t.Errorf("gateOrigins = %s\nwant %s", got, want)
	}
}
