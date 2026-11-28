package k8s

import (
	"fmt"

	"github.com/Soo-de/suiro/internal/model"
)

// expectation adds the required path o's entry states, or flags the first
// rule of model.ExpectationsFile it breaks.
func (t *translation) expectation(o model.Object) {
	var spec model.ExpectationSpec
	if err := decodeStrict(o.Body, &spec); err != nil {
		t.flag(o, model.ReasonExpectationsInvalid, "", "the entry does not parse")
		return
	}
	if bad, broken := checkExpectation(spec); broken {
		t.flag(o, model.ReasonExpectationsInvalid, bad, "the entry breaks a rule and is not checked")
		return
	}
	t.fragment.Expectations = append(t.fragment.Expectations, model.Expectation{
		ID:                    string(t.fragment.Source) + "#" + spec.Name,
		Kind:                  model.ExpectRequired,
		Name:                  spec.Name,
		Sources:               spec.Sources,
		Destinations:          spec.Destinations,
		Ports:                 spec.Ports,
		SourceQuantifier:      spec.SourceQuantifier,
		DestinationQuantifier: spec.DestinationQuantifier,
		Mode:                  model.ModeWarn,
		Origin:                o.Origin,
	})
}

// side is one selector of an entry under its field name.
type side struct {
	name     string
	selector model.EndpointSelector
}

// checkExpectation reports whether e breaks a rule, and the locator of the
// first one broken, which is "" for a rule about the entry as a whole.
func checkExpectation(e model.ExpectationSpec) (string, bool) {
	sides := []side{{"sources", e.Sources}, {"destinations", e.Destinations}}
	// Each of these rules is checked on both sides before the next one.
	for _, check := range []func(side) string{checkPicks, checkInternalForms, checkLabelScope} {
		for _, s := range sides {
			if bad := check(s); bad != "" {
				return bad, true
			}
		}
	}
	switch {
	case e.Sources.Service != "":
		return "sources.service", true
	case e.Destinations.Service != "" && pickingForms(e.Destinations) > 1:
		return "destinations.service", true
	case !picksOwnWorkloads(e.Sources) && !picksOwnWorkloads(e.Destinations):
		return "", true
	case len(e.Ports) == 0:
		return "ports", true
	}
	for i, p := range e.Ports {
		if p.Protocol != model.ProtocolTCP && p.Protocol != model.ProtocolUDP || p.From > p.To {
			return fmt.Sprintf("ports[%d]", i), true
		}
	}
	switch {
	case !validQuantifier(e.SourceQuantifier):
		return "sourceQuantifier", true
	case !validQuantifier(e.DestinationQuantifier):
		return "destinationQuantifier", true
	}
	return "", false
}

func picksOwnWorkloads(s model.EndpointSelector) bool { return s.Self || s.Workload != "" }

func validQuantifier(q model.Quantifier) bool {
	return q == model.QuantifyAll || q == model.QuantifyAny
}

// pickingForms counts the forms of s that pick something; workloadKind and
// labels only narrow another form.
func pickingForms(s model.EndpointSelector) int {
	n := 0
	for _, set := range []bool{
		s.All, len(s.ResourceKinds) > 0, s.Internet, len(s.Endpoints) > 0, s.Self,
		s.Workload != "", s.Application != "", len(s.Namespaces) > 0, s.Service != "",
		s.RDS != "", s.ARN != "", len(s.Tags) > 0,
	} {
		if set {
			n++
		}
	}
	return n
}

func checkPicks(s side) string {
	if pickingForms(s.selector) == 0 {
		return s.name
	}
	return ""
}

// checkInternalForms rejects the forms that only forbidden rules use.
func checkInternalForms(s side) string {
	switch {
	case s.selector.All:
		return s.name + ".all"
	case len(s.selector.Endpoints) > 0:
		return s.name + ".endpoints"
	case len(s.selector.ResourceKinds) > 0:
		return s.name + ".resourceKinds"
	}
	return ""
}

func checkLabelScope(s side) string {
	if s.selector.Labels != nil && len(s.selector.Namespaces) == 0 {
		return s.name + ".labels"
	}
	return ""
}
