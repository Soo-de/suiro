package core

import (
	"context"
	"errors"

	"github.com/Soo-de/suiro/internal/core/sets"
	"github.com/Soo-de/suiro/internal/model"
)

// ErrNoPath is returned by Explain for a key that names no path of the
// Bundle.
var ErrNoPath = errors.New("core: no such path")

// ErrVersion is returned by Analyze and Explain for a Bundle whose
// SchemaVersion or AlgorithmVersion is not this build's.
var ErrVersion = errors.New("core: bundle of another schema or algorithm version")

// NewAnalyzer returns the Analyzer that translates each snapshot with the
// adapter of its kind, builds every set from a Space that newSpace returns
// (one per worker of each analysis, see sets.Space) and runs under opts. It
// errors when two adapters share a Kind, newSpace is nil, or a limit in
// opts is not positive.
func NewAnalyzer(adapters []Adapter, newSpace func() sets.Space, opts Options) (Analyzer, error) {
	panic("core: NewAnalyzer is not implemented")
}

// Options are the settings an Analyzer is built with. Limits live here and
// not in the Bundle, so a pull request cannot raise them.
type Options struct {
	// Limits are the counter limits of every analysis; each must be
	// positive. Analyze copies them into model.Analysis.Limits.
	Limits model.Limits
}

// Analyzer is the engine's entry point. inputs calls it in the server for
// every Bundle it does not skip (see package inputs); the CLI calls it in
// process on a local directory. It performs no I/O: everything it reads is
// in the Bundle. Pairs are independent, so it computes them in parallel, up
// to GOMAXPROCS at a time, and sorts the result into canonical order at the
// end: the output does not depend on scheduling, and parallelism shortens
// the time without changing what any limit counts.
type Analyzer interface {
	// Analyze computes the analysis of b in the steps the package comment
	// lists. Status is AnalysisDone, or AnalysisPartial when an input- or
	// partial-level reason holds, a reached limit included (see
	// model.Limits). The result passes model.Analysis.Validate. Analyze
	// returns an error only when it cannot run at all (a Bundle of another
	// version, ErrVersion; a canceled ctx or one past its deadline; no
	// adapter for a snapshot kind; a fragment that fails
	// model.Fragment.Validate, which is an adapter bug); it then returns no
	// partial result, and an error from ctx wraps ctx.Err().
	// Unusable content becomes Undetermined steps and Unknown verdicts with
	// reasons, never an error and never a clean result.
	Analyze(ctx context.Context, b model.Bundle) (model.Analysis, error)

	// Explain recomputes the path keyed k from b with the same steps
	// Analyze takes, so the detail agrees with the Path that Analyze
	// returns for k (see model.PathDetail). A k that Analyze would not
	// return (an endpoint not in b, a pair that is not computed, a port
	// range that is not exactly one path's) returns ErrNoPath; other errors
	// are those of Analyze. A Bundle rebuilt for a stored analysis carries
	// that analysis's versions, so the detail of one made by another
	// version returns ErrVersion and is shown as no longer available, like
	// one whose snapshots were removed.
	Explain(ctx context.Context, b model.Bundle, k model.PathKey) (model.PathDetail, error)
}
