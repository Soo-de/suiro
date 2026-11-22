package core

import (
	"context"
	"errors"

	"github.com/Soo-de/suiro/internal/model"
)

// ErrNoPath is returned by Explain for a key that names no path of the
// Bundle.
var ErrNoPath = errors.New("core: no such path")

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
	// Analyze computes one analysis:
	//
	//  1. translate each snapshot with the adapter of its kind and merge the
	//     fragments (an ObjectKey from two sources: equal bodies warn,
	//     different bodies make the scope of both sources Unknown, with one
	//     model.ReasonConflictingResource problem per source);
	//  2. merge the cluster profile (AWS-read values win over admin ones);
	//  3. resolve selectors, Service targets, Security Group membership and
	//     each endpoint's placement classes;
	//  4. for every computed pair (see model.Analysis.Paths), every port
	//     range that behaves alike and every class pair, evaluate each chain
	//     step to Pass, Block or Undetermined, AND them (strong Kleene) into
	//     the class outcome, combine class outcomes into the Verdict,
	//     applying input-level reasons over everything else, and keep only
	//     the path's summary;
	//  5. compare with b.Before at class level into Changes;
	//  6. evaluate b.Rules and the required expectations of the fragments
	//     (each source's, at the revision its snapshot read) against both
	//     sides' paths into Findings, marking a finding New when its status
	//     got worse.
	//
	// Status is AnalysisDone, or AnalysisPartial when an input- or
	// partial-level reason holds, a reached limit included (see
	// model.Limits). The result passes model.Analysis.Validate. Analyze
	// returns an error only when it cannot run at all (a canceled ctx or
	// one past its deadline, no adapter for a snapshot kind, a fragment that
	// fails model.Fragment.Validate, which is an adapter bug); it then
	// returns no partial result, and an error from ctx wraps ctx.Err().
	// Unusable content becomes Undetermined steps and Unknown verdicts with
	// reasons, never an error and never a clean result.
	Analyze(ctx context.Context, b model.Bundle) (model.Analysis, error)

	// Explain recomputes the path keyed k from b with the same steps
	// Analyze takes, so the detail agrees with the Path that Analyze
	// returns for k (see model.PathDetail). A k that Analyze would not
	// return (an endpoint not in b, a pair that is not computed, a port
	// range that is not exactly one path's) returns ErrNoPath; other errors
	// are those of Analyze.
	Explain(ctx context.Context, b model.Bundle, k model.PathKey) (model.PathDetail, error)
}
