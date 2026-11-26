package model

import "strings"

// fieldStep is one segment of a FieldPath: a field of an object and, for the
// "[]" and "[Key=value]" forms, which elements of the list it holds.
type fieldStep struct {
	name     string
	selector elementSelector
}

// elementSelector is zero for a plain field. A list selector with an empty key
// takes every element.
type elementSelector struct {
	list       bool
	key, value string
}

// steps splits p at the dots outside brackets, since a tag key may hold one.
func (p FieldPath) steps() []fieldStep {
	var out []fieldStep
	start, depth := 0, 0
	for i, r := range p {
		switch {
		case r == '[':
			depth++
		case r == ']':
			depth--
		case r == '.' && depth == 0:
			out = append(out, parseStep(string(p[start:i])))
			start = i + 1
		}
	}
	return append(out, parseStep(string(p[start:])))
}

func parseStep(s string) fieldStep {
	name, filter, list := strings.Cut(s, "[")
	key, value, _ := strings.Cut(strings.TrimSuffix(filter, "]"), "=")
	return fieldStep{name: name, selector: elementSelector{list: list, key: key, value: value}}
}

// keptNode is the paths merged by shared prefix: what is kept of one value.
type keptNode struct {
	// whole is set where a path ends.
	whole    bool
	children map[string]map[elementSelector]*keptNode
}

// add marks the value at the end of steps as kept.
func (n *keptNode) add(steps []fieldStep) {
	for _, s := range steps {
		if n.children == nil {
			n.children = map[string]map[elementSelector]*keptNode{}
		}
		if n.children[s.name] == nil {
			n.children[s.name] = map[elementSelector]*keptNode{}
		}
		next := n.children[s.name][s.selector]
		if next == nil {
			next = &keptNode{}
			n.children[s.name][s.selector] = next
		}
		n = next
	}
	n.whole = true
}

// reduceValue returns what the nodes covering v keep of it together: v itself
// where a path ends, else an object with only the kept fields v has, or nil
// when v is not an object.
func reduceValue(v any, nodes []*keptNode) any {
	// fields gathers, per field name, the selectors of every covering node.
	fields := map[string][]map[elementSelector]*keptNode{}
	for _, n := range nodes {
		if n.whole {
			return v
		}
		for name, selectors := range n.children {
			fields[name] = append(fields[name], selectors)
		}
	}
	object, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	for name, selectors := range fields {
		if field, present := object[name]; present {
			out[name] = reduceField(field, selectors)
		}
	}
	return out
}

// reduceField reduces the value of one field: a list keeps, in order, the
// elements some selector takes. A value that fits no selector becomes nil.
// No kept paths name one field both plainly and as a list.
func reduceField(v any, selectors []map[elementSelector]*keptNode) any {
	var plain []*keptNode
	for _, set := range selectors {
		if n := set[elementSelector{}]; n != nil {
			plain = append(plain, n)
		}
	}
	if len(plain) > 0 {
		return reduceValue(v, plain)
	}
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []any{}
	for _, element := range list {
		if nodes := covering(element, selectors); len(nodes) > 0 {
			out = append(out, reduceValue(element, nodes))
		}
	}
	return out
}

// covering returns the nodes whose list selector takes element, found by key
// lookup so that many tag keys cost no scan per element.
func covering(element any, selectors []map[elementSelector]*keptNode) []*keptNode {
	var out []*keptNode
	object, _ := element.(map[string]any)
	for _, set := range selectors {
		if n := set[elementSelector{list: true}]; n != nil {
			out = append(out, n)
		}
		// Each field of the element is a candidate [Key=value] filter.
		for key, field := range object {
			value, isString := field.(string)
			if !isString {
				continue
			}
			if n := set[elementSelector{list: true, key: key, value: value}]; n != nil {
				out = append(out, n)
			}
		}
	}
	return out
}
