// Package inputs keeps the list of sources to analyse, loaded from the
// registration file (model.RegistrationsFile) that Helm mounts from the
// platform repository. It watches the collectors through rpc.Collector,
// fetches a snapshot only when an announced digest differs from the stored
// one, stores reads, assembles a model.Bundle per baseline and per open
// pull request, and calls core.Analyzer. It skips re-analysis when the
// fingerprint of all inputs (snapshot digests, the before side, cluster
// profile, rule-set version, tool version) is unchanged.
package inputs
