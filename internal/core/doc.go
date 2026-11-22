// Package core is the analysis engine. It defines the Adapter contract every
// source format implements (adapters live below, in core/adapter) and the
// Analyzer that inputs and the CLI call: it translates snapshots into gates,
// resolves selectors and placement, gives every path a verdict, diffs
// against the before side and checks expectations (forbidden and required
// paths). Analyze keeps a summary per path; Explain recomputes one path's
// chains when they are asked for, so the detail is never stored. All set,
// path and verdict math lives here and in the packages below it;
// collectors, the renderer and the API never compute verdicts. Core takes
// and returns model types and performs no I/O.
//
// An analysis runs in six steps:
//
//  1. translate each snapshot with the adapter of its kind and merge the
//     fragments (an ObjectKey from two sources: equal bodies warn,
//     different bodies make the scope of both sources Unknown, with one
//     model.ReasonConflictingResource problem per source);
//  2. merge the cluster profile (AWS-read values win over admin ones);
//  3. take each namespace's labels in the order model.Namespace gives,
//     and resolve selectors, Service targets, Security Group membership
//     and each endpoint's placement classes;
//  4. for every computed pair (see model.Analysis.Paths), every port range
//     that behaves alike and every class pair, evaluate each chain step to
//     Pass, Block or Undetermined, AND them (strong Kleene) into the class
//     outcome, apply input-level reasons over everything else, and keep
//     only the path's summary (model.Path);
//  5. compare with the Bundle's Before by class pair outcomes into
//     Changes, from the stored summaries alone (see model.Change);
//  6. evaluate the Bundle's rules and the required expectations of the
//     fragments (each source's, at the revision its snapshot read) into
//     Findings, resolving selectors over the endpoints, and mark New as
//     model.Finding.New defines, re-checking the rules against Before.
//
// Explain takes steps 1 to 4 for one path and keeps its chains.
package core
