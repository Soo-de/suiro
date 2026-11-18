// Package sets is the set algebra under every gate: IP prefixes and
// protocol/port ranges as sorted interval lists with union, intersection and
// difference. Its laws (commutativity, De Morgan, absorption) are checked
// with property-based tests. A BDD representation can replace the interval
// lists later without changing callers.
package sets
