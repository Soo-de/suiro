package model

// Version numbers. Each one is raised only by the changes listed for it; a
// change that fits none of them raises none.
//
//	ContractVersion  what a snapshot means: raised when the meaning of a kept
//	                 field, the Body format or the digest computation changes.
//	                 Adding a kept field does not raise it: an adapter reads a
//	                 field an older snapshot lacks as unknown.
//	SchemaVersion    the stored Analysis JSON: raised when a field is removed
//	                 or renamed, or a field is added whose zero value would
//	                 mean anything but "unknown, or an older record".
//	AlgorithmVersion what results mean: raised by every change that can change
//	                 a verdict, outcome or status, among them the analysis
//	                 rules, SpecialPurposeIPv4 and PrivateIPv4, the protocols
//	                 ProtocolAll covers, and a reason code's level or scope.
//	FileAPIVersion   the YAML files: raised by a breaking change to their
//	                 schema; an added optional field does not raise it.
//
// The proto package suiro.v1 only grows, which buf breaking checks.
// ToolVersion names the build for people and decides nothing.
//
// Three rules keep older records readable: a field added later reads as
// "unknown, or an older record" at its zero value; reason codes are only
// added, never removed, renamed or given another meaning; and a reason code a
// build does not know counts as LevelInput (see LevelOf).
const (
	// SchemaVersion is the stored Analysis JSON this build writes.
	SchemaVersion = 1
	// AlgorithmVersion is the analysis this build performs.
	AlgorithmVersion = 1
)
