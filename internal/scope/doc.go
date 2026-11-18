// Package scope decides what a caller may see; it implements
// api.ScopeFilter. It resolves each resource's owner (the registration it
// came from, otherwise the ownership rules of model.OwnershipFile, otherwise
// unowned) and filters results: a caller scoped to one application sees its
// own paths in full and other owners' gates only by location (owner, kind,
// direction, port), never by content. It never changes a verdict.
package scope
