// Package model holds the data types every layer shares. It has no behavior
// beyond constructors, validation and read-only accessors to its tables,
// and imports nothing else from this module, so collectors, the renderer,
// core, store and api can all depend on it without depending on each
// other.
//
// The types fall into five groups:
//
//   - Sources and reads (source.go, snapshot.go): what a collector reads, the
//     snapshot it publishes and the objects in it.
//   - Kept fields (fields.go): the whitelist of network-relevant fields a
//     stored object keeps. Secret values and environment variables are not in
//     it, so they never leave the renderer or the cloud-collector.
//   - The intermediate representation (network.go, gate.go): workloads,
//     namespaces, AWS topology, the cluster profile, and gates (the allow and
//     deny rules of NetworkPolicy, Security Group, Network ACL and route
//     tables, and the port and address transforms of Services). Adapters in
//     core translate snapshots into a Fragment of these.
//   - Results (verdict.go, analysis.go): endpoints, placement classes, paths,
//     the four-valued Verdict (Allow, Deny, PlacementDependent, Unknown) and
//     the three-valued gate Outcome under it, before/after changes,
//     expectations and findings.
//   - Reasons, files and callers (reason.go, registration.go, caller.go): the
//     reason codes every warning and Undetermined step carries, with their
//     level, producers and scope, the registration, ownership and
//     expectations file schemas, and the caller a result is filtered for.
//
// # Not knowing
//
// Each level has one name for "cannot be told", computed from the level
// below: a chain step or class pair is Undetermined (Outcome), a path is
// Unknown (Verdict), an expectation is Unverifiable (ExpectationStatus).
// None of them is ever read as allowed, denied, satisfied or violated.
//
// Missing or unreadable input never becomes an allow:
//   - An element outside the model whose meaning is certain (an SCTP or ICMP
//     port, an IPv6 peer) is dropped, because it touches only traffic the
//     model leaves out (UnexaminedIPv6). A rule left with no peers or no
//     ports is dropped whole and raises ReasonRuleOutsideModel; it never
//     widens to everything.
//   - Input whose meaning cannot be known (an invalid object, an unresolved
//     symbolic link, a custom kind that creates pods, an endpoint with no
//     placement) makes Undetermined only what it can affect. Reasons holds,
//     for every code, who raises it, its level and its scope.
//   - "Every" is never read from an empty list. A field the source leaves
//     unwritten that means "every" becomes an explicit form (Peer.All,
//     AllPorts), and Fragment.Validate rejects a rule with no peers or no
//     ports.
//   - A default the platform fixes is applied as the platform applies it:
//     an omitted Kubernetes port protocol is TCP. Behavior that depends on
//     the installation (whether NetworkPolicies are enforced, whether ipBlock
//     matches pod addresses, VPC CNI SNAT) is a ClusterProfile field whose
//     unknown value makes Undetermined the steps it decides. Changes made to an object on
//     admission (mutating webhooks, policy engines) cannot be seen from git
//     and are a known limit.
//
// JSON field names are the wire and storage names: the HTTP API, documents in
// the database, and the YAML files, which are read through a YAML-to-JSON
// conversion so one set of tags serves all three.
package model
