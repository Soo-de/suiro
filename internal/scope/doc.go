// Package scope decides what a caller may see; it implements
// api.ScopeFilter. It resolves each resource's owner (the registration it
// came from, otherwise the ownership rules of model.OwnershipFile, otherwise
// unowned) and filters results: a caller scoped to one application sees its
// own paths in full and other owners' gates only by location (layer, type,
// direction, owners, and the IDs of AWS members), never by content, as
// model.GateRef.Redacted defines. It never changes a verdict.
package scope
