// Package inputs keeps the list of sources to analyze, loaded from the
// registration file (model.RegistrationsFile) that Helm mounts from the
// platform repository. It watches the collectors through rpc.Collector,
// fetches a snapshot only when an announced digest differs from the stored
// one, stores reads, assembles a model.Bundle per baseline and per open
// pull request, and calls core.Analyzer. The environment whose registration
// sets cluster gets one more source, "cluster/<environment>", read by
// cloud-collector; like the AWS source it raises a stale-read reason
// (model.ReasonClusterReadStale) once its last complete read is older than
// the configured limit.
//
// It recomputes the digest of every snapshot it fetches with
// model.ComputeDigest and rejects one that does not match, keeping the
// previous read (model.ReasonDigestMismatch). The check catches defects and
// version skew between services, not attacks: collectors are authenticated
// services of the same installation.
//
// It skips an analysis only when the latest analysis of the same line has
// the Bundle's fingerprint (model.Bundle.Fingerprint, computed with
// model.ComputeFingerprint) and a finished status: model.AnalysisDone,
// model.AnalysisPartial or model.AnalysisNoPreview. The line of a baseline
// is store.AnalysisStore.LatestBaseline of its environment; that of a pull
// request is LatestPullRequest of its environment, repository and number.
// Comparing with the latest analysis alone means a pull request that
// returns to an earlier state is analyzed again, so its latest result
// always matches its current inputs, and a failed analysis is retried.
//
// A Before made with another model.SchemaVersion or model.AlgorithmVersion
// is analyzed again with this build before anything is compared with it;
// when that fails, the Bundle carries model.ReasonBeforeOutdated (see
// model.Bundle.Before).
package inputs
