// Package inputs keeps the list of sources to analyse, loaded from the
// registration file (model.RegistrationsFile) that Helm mounts from the
// platform repository. It watches the collectors through rpc.Collector,
// fetches a snapshot only when an announced digest differs from the stored
// one, stores reads, assembles a model.Bundle per baseline and per open
// pull request, and calls core.Analyzer. It skips an analysis when one with
// the same fingerprint is stored; the fingerprint covers the analysis kind,
// environment and pull request as well as every input
// (model.Bundle.Fingerprint).
package inputs
