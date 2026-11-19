// Package conformance is the scenario suite every core.Adapter must pass.
// Fixtures carry expected verdicts taken from outside this code: Kubernetes
// documentation examples, netpol-analyzer output and connectivity measured
// on a test cluster. An adapter's own tests are not enough to trust it.
package conformance
