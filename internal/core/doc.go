// Package core is the analysis engine. It turns gates into reachability
// between endpoints, gives every path a verdict, groups equivalent
// endpoints, diffs two analyses and checks expectations (forbidden and
// required paths). All set, path and verdict math lives here and in the
// packages below it; collectors, the renderer and the API never compute
// verdicts. Core takes and returns model types and performs no I/O.
package core
