package k8s

import (
	"fmt"

	"github.com/Soo-de/suiro/internal/model"
)

type serviceObject struct {
	Metadata objectMeta  `json:"metadata"`
	Spec     serviceSpec `json:"spec"`
}

type serviceSpec struct {
	Type     string            `json:"type"`
	Selector map[string]string `json:"selector"`
	Ports    []servicePort     `json:"ports"`
}

type servicePort struct {
	Name       string      `json:"name"`
	Protocol   string      `json:"protocol"`
	Port       int64       `json:"port"`
	TargetPort intOrString `json:"targetPort"`
}

// service adds the port transform o puts in front of the pods it selects, or
// reports that it selects none.
func (t *translation) service(o model.Object) {
	var s serviceObject
	if err := decodeLenient(o.Body, &s); err != nil {
		t.invalid(o, "")
		return
	}
	if bad := checkService(s); bad != "" {
		t.invalid(o, bad)
		return
	}
	switch withDefault(s.Spec.Type, defaultServiceType) {
	case serviceExternalName:
		t.flag(o, model.ReasonServiceSelectsNoPods, "spec.type", "an ExternalName Service maps no pod ports")
		return
	case serviceLoadBalancer:
		t.flag(o, model.ReasonAWSControllerEffect, "spec.type",
			"the load balancer controller writes rules the model does not see")
	}
	if len(s.Spec.Selector) == 0 {
		t.flag(o, model.ReasonServiceSelectsNoPods, "spec.selector", "a Service without a selector maps no pod ports")
		return
	}
	t.fragment.Gates = append(t.fragment.Gates, model.Gate{
		ID:    model.GateID(o.Key),
		Layer: model.LayerKubernetes,
		Type:  model.GateTransform,
		Target: model.GateTarget{Pods: &model.PodTarget{
			Namespace: s.Metadata.Namespace,
			Selector:  model.LabelSelector{MatchLabels: s.Spec.Selector},
		}},
		Mappings: portMappings(s.Spec.Ports),
		Object:   o.Key,
		Origin:   o.Origin,
	})
}

func checkService(s serviceObject) string {
	if s.Metadata.Namespace == "" {
		return "metadata.namespace"
	}
	switch s.Spec.Type {
	case "", serviceClusterIP, serviceNodePort, serviceLoadBalancer, serviceExternalName:
	default:
		return "spec.type"
	}
	if bad := checkLabels(s.Spec.Selector, "spec.selector"); bad != "" {
		return bad
	}
	return checkServicePorts(s.Spec.Ports)
}

// checkServicePorts checks each port, then that no two share a number and
// protocol.
func checkServicePorts(ports []servicePort) string {
	names := make(map[string]bool, len(ports))
	for i, p := range ports {
		at := fmt.Sprintf("spec.ports[%d]", i)
		switch {
		case p.Name == "" && len(ports) > 1,
			p.Name != "" && (!validDNSLabel(p.Name) || names[p.Name]):
			return at + ".name"
		case !validPort(p.Port):
			return at + ".port"
		case p.Protocol != "" && !validProtocol(p.Protocol):
			return at + ".protocol"
		case !validTargetPort(p.TargetPort):
			return at + ".targetPort"
		}
		names[p.Name] = true
	}
	type key struct {
		port     int64
		protocol string
	}
	used := make(map[key]bool, len(ports))
	for i, p := range ports {
		k := key{p.Port, withDefault(p.Protocol, defaultProtocol)}
		if used[k] {
			return fmt.Sprintf("spec.ports[%d]", i)
		}
		used[k] = true
	}
	return ""
}

// validTargetPort accepts an unset targetPort (0 or ""), a port number or a
// port name.
func validTargetPort(v intOrString) bool {
	if v.isString {
		return v.text == "" || validPortName(v.text)
	}
	return v.number == 0 || validPort(v.number)
}

// portMappings returns the modeled ports of valid Service ports, in written
// order.
func portMappings(ports []servicePort) []model.PortMapping {
	var out []model.PortMapping
	for _, p := range ports {
		protocol, modeled := modelProtocol(withDefault(p.Protocol, defaultProtocol))
		if !modeled {
			continue
		}
		m := model.PortMapping{Name: p.Name, Protocol: protocol, Port: portNumber(p.Port)}
		switch target := p.TargetPort; {
		case target.text != "":
			m.TargetPortName = target.text
		case target.number != 0:
			m.TargetPort = portNumber(target.number)
		default:
			m.TargetPort = portNumber(defaultTargetPort(p.Port))
		}
		out = append(out, m)
	}
	return out
}
