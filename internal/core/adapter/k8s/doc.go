// Package k8s turns rendered Kubernetes objects (NetworkPolicy, workload pod
// templates, Service, Namespace) into model gates with Kubernetes semantics:
// a pod that no policy selects in a direction is open in that direction;
// once selected, only what some policy allows passes.
package k8s
