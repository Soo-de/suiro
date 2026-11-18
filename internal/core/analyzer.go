package core

import (
	"context"

	"github.com/Soo-de/suiro/internal/model"
)

// Analyzer is the engine's entry point. inputs calls it in the server for
// every new input fingerprint; the CLI calls it in process on a local
// directory. It performs no I/O: everything it reads is in the Bundle.
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
	//  4. for every source/destination pair, every port range that behaves
	//     alike and every class pair, evaluate each chain step to Pass,
	//     Block or Undetermined, AND them (strong Kleene) into the class
	//     outcome, and combine class outcomes into the Verdict, applying
	//     input-level reasons over everything else;
	//  5. compare with b.Before at class level into Changes;
	//  6. evaluate b.Rules and the required expectations of the fragments
	//     (each source's, at the revision its snapshot read) against both
	//     sides' paths into Findings, marking a finding New when its status
	//     got worse.
	//
	// Status is AnalysisDone, or AnalysisPartial when an input- or
	// partial-level reason holds. Analyze returns an error only when it
	// cannot run at all (a cancelled ctx, no adapter for a snapshot kind);
	// unusable content becomes Unknown verdicts with reasons, never an
	// error and never a clean result.
	Analyze(ctx context.Context, b model.Bundle) (model.Analysis, error)
}
