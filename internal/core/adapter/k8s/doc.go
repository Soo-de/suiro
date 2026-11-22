// Package k8s implements core.Adapter for rendered Kubernetes objects
// (NetworkPolicy, workload pod templates, Service, Namespace). It restates
// them as model gates whose semantics carry Kubernetes behavior: a
// NetworkPolicy is an isolating, additive, stateful filter (a pod that no
// policy selects in a direction is open in that direction; once selected,
// only what some policy allows passes), a Service a port transform.
// Selectors stay unresolved for core. Unsupported policy kinds and
// controller-managed AWS objects are flagged with their reason codes. It
// also turns the source's expectation entries (model.KindExpectation) into
// the fragment's required expectations, and translates the snapshots of a
// model.SourceCluster source, which hold Namespace objects only, into the
// fragment's namespaces.
//
// It applies the defaults the API server writes into an object on create,
// which no cluster setting changes, from one table kept by Kubernetes
// version: an omitted NetworkPolicy or Service port protocol is TCP, an
// omitted policyTypes is Ingress plus Egress when egress rules are present.
// An unwritten "from", "to" or "ports" becomes model.Peer.All or
// model.AllPorts. An SCTP port or IPv6 ipBlock is dropped, and a rule left
// with nothing in that list is dropped whole
// (model.ReasonRuleOutsideModel). An object the API server would reject
// raises model.ReasonObjectInvalid.
package k8s
