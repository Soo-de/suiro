// Package model holds the data types every layer shares. It has no behavior
// beyond constructors and validation and imports nothing else from this
// module, so collectors, the renderer, core, store and api can all depend on
// it without depending on each other.
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
//     reason codes every warning and Unknown carries, the registration,
//     ownership and expectations file schemas, and the caller a result is
//     filtered for.
//
// JSON field names are the wire and storage names: the HTTP API, documents in
// the database, and the YAML files, which are read through a YAML-to-JSON
// conversion so one set of tags serves all three.
package model
