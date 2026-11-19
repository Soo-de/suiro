// Package k8s implements core.Adapter for rendered Kubernetes objects
// (NetworkPolicy, workload pod templates, Service, Namespace). It restates
// them as model gates whose semantics carry Kubernetes behavior: a
// NetworkPolicy is an isolating, additive, stateful filter (a pod that no
// policy selects in a direction is open in that direction; once selected,
// only what some policy allows passes), a Service a port transform.
// Selectors stay unresolved for core. Unsupported policy kinds and
// controller-managed AWS objects are flagged with their reason codes. It
// also turns the source's expectation entries (model.KindExpectation) into
// the fragment's required expectations.
package k8s
