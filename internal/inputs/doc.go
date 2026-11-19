// Package inputs keeps the list of sources to analyse, loaded from the
// registration file (model.RegistrationsFile) that Helm mounts from the
// platform repository. It watches the collectors through rpc.Collector,
// fetches a snapshot only when an announced digest differs from the stored
// one, stores reads, assembles a model.Bundle per baseline and per open
// pull request, and calls core.Analyzer.
//
// It skips an analysis only when the latest analysis of the same line has
// the Bundle's fingerprint (model.Bundle.Fingerprint) and a finished status:
// model.AnalysisDone, model.AnalysisPartial or model.AnalysisNoPreview. The
// line of a baseline is store.AnalysisStore.LatestBaseline of its
// environment; that of a pull request is LatestPullRequest of its
// environment, repository and number. Comparing with the latest analysis
// alone means a pull request that returns to an earlier state is analysed
// again, so its latest result always matches its current inputs, and a
// failed analysis is retried.
package inputs
