package k8s

import (
	"fmt"

	"github.com/Soo-de/suiro/internal/model"
)

// podTemplate is a pod template, and a Pod itself, which has the same shape.
type podTemplate struct {
	Metadata objectMeta `json:"metadata"`
	Spec     podSpec    `json:"spec"`
}

type podSpec struct {
	HostNetwork  bool               `json:"hostNetwork"`
	NodeName     string             `json:"nodeName"`
	NodeSelector map[string]string  `json:"nodeSelector"`
	Affinity     affinity           `json:"affinity"`
	Tolerations  []model.Toleration `json:"tolerations"`
	Containers   []container        `json:"containers"`
}

// affinity holds the required terms only; preferred ones do not limit
// placement.
type affinity struct {
	NodeAffinity struct {
		Required struct {
			NodeSelectorTerms []model.NodeSelectorTerm `json:"nodeSelectorTerms"`
		} `json:"requiredDuringSchedulingIgnoredDuringExecution"`
	} `json:"nodeAffinity"`
	PodAffinity     podAffinity `json:"podAffinity"`
	PodAntiAffinity podAffinity `json:"podAntiAffinity"`
}

type podAffinity struct {
	Required []model.PodAffinityTerm `json:"requiredDuringSchedulingIgnoredDuringExecution"`
}

type container struct {
	Name  string          `json:"name"`
	Ports []containerPort `json:"ports"`
}

type containerPort struct {
	Name          string `json:"name"`
	ContainerPort int64  `json:"containerPort"`
	HostPort      int64  `json:"hostPort"`
	Protocol      string `json:"protocol"`
}

// controller is a Deployment, StatefulSet, DaemonSet, ReplicaSet or Job.
type controller struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		Template podTemplate `json:"template"`
	} `json:"spec"`
}

type cronJob struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		JobTemplate struct {
			Spec struct {
				Template podTemplate `json:"template"`
			} `json:"spec"`
		} `json:"jobTemplate"`
	} `json:"spec"`
}

// podSource is what a workload kind says about its pods.
type podSource struct {
	// metadata is the object's own.
	metadata objectMeta
	template podTemplate
	// labelsAt and specAt locate the template's labels and spec.
	labelsAt, specAt string
}

func decodePodSource(o model.Object) (podSource, error) {
	switch o.Kind {
	case model.KindPod:
		var p podTemplate
		err := decodeLenient(o.Body, &p)
		return podSource{p.Metadata, p, "metadata.labels", "spec"}, err
	case model.KindCronJob:
		var c cronJob
		err := decodeLenient(o.Body, &c)
		const at = "spec.jobTemplate.spec.template"
		return podSource{c.Metadata, c.Spec.JobTemplate.Spec.Template, at + ".metadata.labels", at + ".spec"}, err
	default:
		var c controller
		err := decodeLenient(o.Body, &c)
		const at = "spec.template"
		return podSource{c.Metadata, c.Spec.Template, at + ".metadata.labels", at + ".spec"}, err
	}
}

// workload adds the workload o creates pods as, with the warnings its pods
// call for.
func (t *translation) workload(o model.Object) {
	p, err := decodePodSource(o)
	if err != nil {
		t.invalid(o, "")
		return
	}
	if bad := checkPodSource(p); bad != "" {
		t.invalid(o, bad)
		return
	}
	spec := p.template.Spec
	t.fragment.Workloads = append(t.fragment.Workloads, model.Workload{
		Key:         model.EndpointKey(o.Key),
		Kind:        o.Kind,
		Namespace:   p.metadata.Namespace,
		Name:        p.metadata.Name,
		PodLabels:   p.template.Metadata.Labels,
		Ports:       t.containerPorts(o, spec, p.specAt),
		Placement:   placement(spec),
		HostNetwork: spec.HostNetwork,
		Origin:      o.Origin,
	})
	if o.Kind == model.KindPod {
		t.flag(o, model.ReasonStandalonePod, "", "a Pod without a controller")
	}
	if spec.HostNetwork {
		t.flag(o, model.ReasonHostNetwork, p.specAt+".hostNetwork", "the pod uses the node's network")
	}
}

func checkPodSource(p podSource) string {
	if p.metadata.Namespace == "" {
		return "metadata.namespace"
	}
	if bad := checkLabels(p.template.Metadata.Labels, p.labelsAt); bad != "" {
		return bad
	}
	return checkContainers(p.template.Spec, p.specAt)
}

func checkContainers(spec podSpec, specAt string) string {
	if len(spec.Containers) == 0 {
		return specAt + ".containers"
	}
	names := make(map[string]bool, len(spec.Containers))
	for i, c := range spec.Containers {
		at := fmt.Sprintf("%s.containers[%d]", specAt, i)
		if c.Name == "" || names[c.Name] {
			return at + ".name"
		}
		names[c.Name] = true
		if bad := checkContainerPorts(c.Ports, at); bad != "" {
			return bad
		}
	}
	// The API checks the host network rule after every container.
	if spec.HostNetwork {
		return checkHostNetworkPorts(spec.Containers, specAt)
	}
	return ""
}

func checkContainerPorts(ports []containerPort, containerAt string) string {
	names := make(map[string]bool, len(ports))
	for i, p := range ports {
		at := fmt.Sprintf("%s.ports[%d]", containerAt, i)
		switch {
		case p.Name != "" && (!validPortName(p.Name) || names[p.Name]):
			return at + ".name"
		case !validPort(p.ContainerPort):
			return at + ".containerPort"
		case p.HostPort != 0 && !validPort(p.HostPort):
			return at + ".hostPort"
		case p.Protocol != "" && !validProtocol(p.Protocol):
			return at + ".protocol"
		}
		names[p.Name] = true
	}
	return ""
}

// checkHostNetworkPorts requires a written hostPort to equal its
// containerPort.
func checkHostNetworkPorts(containers []container, specAt string) string {
	for i, c := range containers {
		for j, p := range c.Ports {
			if p.HostPort != 0 && p.HostPort != p.ContainerPort {
				return fmt.Sprintf("%s.containers[%d].ports[%d].hostPort", specAt, i, j)
			}
		}
	}
	return ""
}

// containerPorts returns the modeled ports of a valid spec in container then
// port order, and flags every port with a hostPort.
func (t *translation) containerPorts(o model.Object, spec podSpec, specAt string) []model.ContainerPort {
	var out []model.ContainerPort
	for i, c := range spec.Containers {
		for j, p := range c.Ports {
			hostPort := p.HostPort
			if hostPort == 0 && spec.HostNetwork {
				hostPort = defaultHostPort(p.ContainerPort)
			}
			if hostPort != 0 {
				at := fmt.Sprintf("%s.containers[%d].ports[%d]", specAt, i, j)
				t.flag(o, model.ReasonHostPort, at, "the port is exposed on the node's address")
			}
			protocol, modeled := modelProtocol(withDefault(p.Protocol, defaultProtocol))
			if !modeled {
				continue
			}
			out = append(out, model.ContainerPort{
				Container: c.Name,
				Name:      p.Name,
				Port:      portNumber(p.ContainerPort),
				Protocol:  protocol,
				HostPort:  portNumber(hostPort),
			})
		}
	}
	return out
}

func placement(spec podSpec) model.Placement {
	return model.Placement{
		NodeSelector:    spec.NodeSelector,
		NodeTerms:       spec.Affinity.NodeAffinity.Required.NodeSelectorTerms,
		Tolerations:     spec.Tolerations,
		NodeName:        spec.NodeName,
		PodAffinity:     spec.Affinity.PodAffinity.Required,
		PodAntiAffinity: spec.Affinity.PodAntiAffinity.Required,
	}
}
