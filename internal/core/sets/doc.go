// Package sets is the set algebra under every gate: protocol/port sets
// (GateSet) and IPv4 address sets (AddrSet) with union, intersection and
// difference, built through a Space. The first Space keeps sorted interval
// lists (a prefix a/n is the interval [a, a+2^(32-n)-1]). Its laws
// (commutativity, De Morgan, absorption) must hold and are to be checked by
// property-based tests. A BDD representation can replace the interval lists later by
// providing another Space, without changing callers.
package sets
