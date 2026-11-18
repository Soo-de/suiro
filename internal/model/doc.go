// Package model holds the data types every layer shares: resources and their
// identities, gates (the allow and deny rules of NetworkPolicy, Security
// Group, Network ACL and route tables), endpoints, paths, and the four-
// valued verdict (Allow, Deny, PlacementDependent, Unknown) with its reason
// code. It has no behavior beyond constructors and validation and imports
// nothing else from this module.
package model
