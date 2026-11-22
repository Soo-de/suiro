// Package sets is the set algebra under every gate: protocol/port sets
// (GateSet) and IPv4 address sets (AddrSet) with union, intersection and
// difference, built through a Space. The first Space keeps sorted interval
// lists (a prefix a/n is the interval [a, a+2^(32-n)-1]). The laws every
// Space must meet (commutativity, De Morgan, absorption, exact membership,
// canonical forms) are checked by settest.Laws, which each implementation's
// tests call. A BDD representation can replace the interval lists later by
// providing another Space, without changing callers.
package sets
