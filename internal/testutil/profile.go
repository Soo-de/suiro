package testutil

import (
	"net/netip"

	"github.com/Soo-de/suiro/internal/model"
)

// LabProfile returns the cluster profile of the test cluster that
// connectivity is measured on: kubeadm with Calico, which enforces
// NetworkPolicies, the pod network 10.244.0.0/16 and no AWS layer.
// Whether Calico applies ipBlock to pod addresses has not been measured,
// so IPBlockAppliesToPods stays unknown.
func LabProfile() model.ClusterProfile {
	return model.ClusterProfile{
		CNI:                      "calico",
		NetworkPolicyEnforcement: model.ToggleOn,
		PodCIDRs:                 []netip.Prefix{netip.MustParsePrefix("10.244.0.0/16")},
	}
}
