// Package conformance is the scenario suite every core.Adapter must pass.
// Fixtures carry expected verdicts taken from outside this code: Kubernetes
// documentation examples, netpol-analyzer output and connectivity measured
// on a test cluster. An adapter's own tests are not enough to trust it.
// Every fragment an adapter returns must pass model.Fragment.Validate and
// carry only reason codes whose model.Reasons row names
// model.ProducerAdapter, and the suite holds the cases that keep missing
// input from becoming an allow: a rule whose only port is SCTP, one whose
// only peer is IPv6, a mixed one, and an object the API server would reject.
package conformance
