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
package core
