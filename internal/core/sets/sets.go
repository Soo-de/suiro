package sets

import (
	"net/netip"

	"github.com/Soo-de/suiro/internal/model"
)

// GateSet is a set of connections between one fixed source and destination
// in one class pair: a set of (protocol, destination port) pairs over TCP and
// UDP. Each gate's allowed part is a GateSet, and a path's open part is
// their intersection. Sets are values: operations return new sets and never
// change their operands. Sets from different Spaces must not be mixed.
type GateSet interface {
	Intersect(o GateSet) GateSet
	Union(o GateSet) GateSet
	// Diff is the set minus o.
	Diff(o GateSet) GateSet
	IsEmpty() bool
	Contains(proto model.Protocol, port uint16) bool
	// Ranges is the set as sorted, non-overlapping, non-adjacent ranges,
	// TCP first; it is the canonical form, so equal sets have equal Ranges.
	Ranges() []model.PortRange
}

// AddrSet is a set of IPv4 addresses. It decides how a rule's prefixes
// cover a placement class's addresses: wholly, not at all, or partly, which
// splits the class.
type AddrSet interface {
	Intersect(o AddrSet) AddrSet
	Union(o AddrSet) AddrSet
	// Diff is the set minus o.
	Diff(o AddrSet) AddrSet
	IsEmpty() bool
	Contains(a netip.Addr) bool
	// Prefixes is the set as the shortest sorted list of prefixes covering
	// exactly its addresses; equal sets have equal Prefixes.
	Prefixes() []netip.Prefix
}

// Space builds sets of one representation. The first Space keeps sorted
// interval lists; a BDD Space can replace it without changing callers,
// which receive the Space and never construct sets otherwise.
//
// A Space and the sets it built are used by one goroutine at a time,
// because an implementation may share state among them (a BDD node table).
// Core therefore takes a function that returns a new Space and calls it
// once per worker of every analysis; sets never cross workers. Mixing sets
// of two Spaces is a programming error and may panic.
type Space interface {
	// Ports returns the union of ranges. model.ProtocolAll stands for both
	// TCP and UDP. No ranges gives the empty set.
	Ports(ranges ...model.PortRange) GateSet
	// Addrs returns the union of the IPv4 prefixes; IPv6 prefixes are
	// outside the model and add nothing. No prefixes gives the empty set.
	Addrs(prefixes ...netip.Prefix) AddrSet
}

// NewIntervalSpace returns the Space of sorted interval lists. Its sets
// share no state.
func NewIntervalSpace() Space {
	panic("sets: NewIntervalSpace is not implemented")
}
