package k8s

import "github.com/Soo-de/suiro/internal/model"

// Kubernetes API defaults, v1.25 and later. The API server writes them into
// an object on create and no cluster setting changes them.
const (
	// defaultProtocol is that of a NetworkPolicy, Service or container port.
	defaultProtocol    = "TCP"
	defaultServiceType = serviceClusterIP
	// namespaceNameLabel is set on every Namespace to its name, over any
	// written value.
	namespaceNameLabel = "kubernetes.io/metadata.name"
)

// The Service types of the API.
const (
	serviceClusterIP    = "ClusterIP"
	serviceNodePort     = "NodePort"
	serviceLoadBalancer = "LoadBalancer"
	serviceExternalName = "ExternalName"
)

// withDefault returns written, or def when nothing is written.
func withDefault(written, def string) string {
	if written == "" {
		return def
	}
	return written
}

// defaultPolicyTypes is a NetworkPolicy's policyTypes when none are written.
func defaultPolicyTypes(egressRules int) []model.Direction {
	if egressRules > 0 {
		return []model.Direction{model.Ingress, model.Egress}
	}
	return []model.Direction{model.Ingress}
}

// defaultTargetPort is a Service port's targetPort when unset.
func defaultTargetPort(port int64) int64 { return port }

// defaultHostPort is a container port's hostPort when unset in a pod on the
// host network.
func defaultHostPort(containerPort int64) int64 { return containerPort }
