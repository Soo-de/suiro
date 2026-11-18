// Package scope decides what a caller may see. It resolves each resource's
// owner (the registration it came from, otherwise the ownership rules,
// otherwise unowned) and filters results: a caller scoped to one application
// sees its own paths in full and other owners' gates only by location
// (owner, kind, direction, port), never by content.
package scope
