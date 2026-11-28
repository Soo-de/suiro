package k8s

import "github.com/Soo-de/suiro/internal/model"

type namespaceObject struct {
	Metadata objectMeta `json:"metadata"`
}

// namespace adds the namespace o declares, with the automatic name label.
func (t *translation) namespace(o model.Object) {
	var n namespaceObject
	if err := decodeLenient(o.Body, &n); err != nil {
		t.invalid(o, "")
		return
	}
	labels := n.Metadata.Labels
	if labels == nil {
		labels = make(map[string]string, 1)
	}
	// Defaulting precedes validation, so a written name label never fails it.
	labels[namespaceNameLabel] = n.Metadata.Name
	if bad := checkLabels(labels, "metadata.labels"); bad != "" {
		t.invalid(o, bad)
		return
	}
	t.fragment.Namespaces = append(t.fragment.Namespaces,
		model.Namespace{Name: n.Metadata.Name, Labels: labels, Origin: o.Origin})
}
