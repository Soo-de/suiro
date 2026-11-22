// Package model holds the data types every layer shares. It has no behavior
// beyond constructors, validation and read-only accessors to its tables,
// and imports nothing else from this module, so collectors, the renderer,
// core, store and api can all depend on it without depending on each
// other.
//
// The types fall into six groups:
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
//   - Results (verdict.go, analysis.go): endpoints, placement classes, path
//     summaries and the detail Explain recomputes, the four-valued Verdict
//     (Allow, Deny, PlacementDependent, Unknown) and the three-valued gate
//     Outcome under it, before/after changes, expectations and findings.
//     The key types have one constructor and parser each (NewObjectKey,
//     NewGateID, PathKey.String and ParsePathKey).
//   - Reasons, files and callers (reason.go, registration.go, caller.go): the
//     reason codes every warning and Undetermined step carries, with their
//     level, producers and scope, the registration, ownership and
//     expectations file schemas, and the caller a result is filtered for.
//   - Versions (version.go): which change raises which version number.
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
//     model leaves out (UnexaminedIPv6). A rule that dropping leaves with no
//     peers or no ports is dropped whole and raises ReasonRuleOutsideModel;
//     it never widens to everything.
//   - Input whose meaning cannot be known (an invalid object, an unresolved
//     symbolic link, a custom kind that creates pods, an endpoint with no
//     placement) makes Undetermined only what it can affect. Reasons holds,
//     for every code, who raises it, its level and its scope.
//   - "Every" is never read from an empty list of the model. A source field
//     that means "every" when unwritten or empty, as a NetworkPolicy rule's
//     from, to and ports do, becomes an explicit form (Peer.All, AllPorts),
//     and Fragment.Validate rejects a rule with no peers or no ports.
//   - A default the platform fixes is applied as the platform applies it:
//     an omitted Kubernetes port protocol is TCP. Behavior that depends on
//     the installation (whether NetworkPolicies are enforced, whether ipBlock
//     matches pod addresses, VPC CNI SNAT) is a ClusterProfile field whose
//     unknown value makes Undetermined the steps it decides. Changes made
//     to an object on admission (mutating webhooks, policy engines) cannot
//     be seen from git and are a known limit.
//
// # Canonical order
//
// Every list in an Analysis or PathDetail that core returns is sorted by a
// total order, and a list field without omitempty is [] rather than null
// when empty, so the same Bundle gives byte-equal JSON however the work was
// scheduled:
//   - Endpoints by Key; Paths and Changes by Key in PathKey order (Source,
//     Destination, then the port range's Protocol and From); each
//     finding's Paths in PathKey order; Inputs by Key (Source, then
//     PullRequest).
//   - Findings by Rule, Subject, Kind, then Origin.Locator, which no two
//     findings share.
//   - Problems by Reason, Subject, then Origin (none first; Source, File,
//     Line, Resource, then Locator), then Message, so only identical
//     problems tie.
//   - Unexamined, every Reasons list and every list of IDs (Security
//     Groups, gate Members) by value; Owners by Team, then Application.
//   - Placement classes in class order: by NodeGroup, Subnet, Addresses
//     (compared as lists, each prefix by address, then length), then Name,
//     each class's Addresses sorted by address. Class pairs in pair order:
//     by source class, destination class, then SameNode, false first.
//     Endpoint.Classes, Path.Outcomes, a PathDetail's Classes and every
//     class pair list of a Change follow them.
//   - A class's Gates in path order; a step's Rules by Gate,
//     Origin.Locator, then Return (false first), and its Mappings by Gate,
//     Mapping.Protocol, then Mapping.Port; an endpoint's Services by Name,
//     and each one's Mappings by Protocol, then Port, which Kubernetes
//     keeps unique within a Service.
//
// JSON field names are the wire and storage names: the HTTP API, documents in
// the database, and the YAML files, which are read through a YAML-to-JSON
// conversion so one set of tags serves all three.
package model
