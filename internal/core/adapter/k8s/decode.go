package k8s

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// decodeStrict reads raw into v and errors on a field v does not have.
func decodeStrict(raw json.RawMessage, v any) error { return decode(raw, v, true) }

// decodeLenient reads raw into v and ignores a field v does not have.
func decodeLenient(raw json.RawMessage, v any) error { return decode(raw, v, false) }

// decode matches field names exactly, as the API server does. encoding/json
// alone ignores case and would read a misspelled "Ingress" as rules the
// cluster never applies, so such keys are removed, or rejected, before it
// sees them.
func decode(raw json.RawMessage, v any, strict bool) error {
	tree, err := decodeTree(raw)
	if err != nil {
		return err
	}
	if err := matchFields(tree, reflect.TypeOf(v), strict); err != nil {
		return err
	}
	exact, err := json.Marshal(tree)
	if err != nil {
		return err
	}
	return json.Unmarshal(exact, v)
}

// decodeTree reads raw as one generic JSON value, keeping numbers as written.
func decodeTree(raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("data after the JSON value")
	}
	return tree, nil
}

var unmarshaler = reflect.TypeFor[json.Unmarshaler]()

// matchFields walks tree along t and removes, or when strict rejects, every
// object key that names no field of its struct exactly. A value whose shape
// does not fit t is left for json.Unmarshal to reject.
func matchFields(tree any, t reflect.Type, strict bool) error {
	if t.Implements(unmarshaler) {
		return nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		return matchFields(tree, t.Elem(), strict)
	case reflect.Slice:
		list, _ := tree.([]any)
		for _, item := range list {
			if err := matchFields(item, t.Elem(), strict); err != nil {
				return err
			}
		}
	case reflect.Map:
		object, _ := tree.(map[string]any)
		for _, value := range object {
			if err := matchFields(value, t.Elem(), strict); err != nil {
				return err
			}
		}
	case reflect.Struct:
		object, _ := tree.(map[string]any)
		return matchStruct(object, t, strict)
	}
	return nil
}

// matchStruct is matchFields for the keys of one object read into the struct
// type t, whose fields all carry a json tag.
func matchStruct(object map[string]any, t reflect.Type, strict bool) error {
	fields := make(map[string]reflect.Type, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		if name, _, _ := strings.Cut(f.Tag.Get("json"), ","); name != "" {
			fields[name] = f.Type
		}
	}
	for key, value := range object {
		field, known := fields[key]
		switch {
		case known:
			if err := matchFields(value, field, strict); err != nil {
				return err
			}
		case strict:
			return fmt.Errorf("unknown field %q", key)
		default:
			delete(object, key)
		}
	}
	return nil
}

// intOrString is a field the API takes as an integer or a string.
type intOrString struct {
	isString bool
	number   int64
	text     string
}

// UnmarshalJSON errors on anything but an integer or a string; null leaves v
// unset.
func (v *intOrString) UnmarshalJSON(data []byte) error {
	if bytes.HasPrefix(data, []byte(`"`)) {
		v.isString = true
		return json.Unmarshal(data, &v.text)
	}
	return json.Unmarshal(data, &v.number)
}

// objectMeta is the part of metadata the adapter reads.
type objectMeta struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels"`
}

// hasPodTemplate reports whether body has the key
// spec.template.spec.containers.
func hasPodTemplate(body json.RawMessage) (bool, error) {
	tree, err := decodeTree(body)
	if err != nil {
		return false, err
	}
	object, _ := tree.(map[string]any)
	for _, name := range []string{"spec", "template", "spec"} {
		object, _ = object[name].(map[string]any)
	}
	_, ok := object["containers"]
	return ok, nil
}
