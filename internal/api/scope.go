package api

import "github.com/Soo-de/suiro/internal/model"

// ScopeFilter decides what a caller may see of a result. Package scope
// implements it from the registration and ownership files. It filters
// output only: verdicts are always computed on the whole model, because a
// path between two teams needs both teams' rules.
type ScopeFilter interface {
	// Owner resolves the owner of an object: the registration of the source
	// it came from; else the first ownership rule whose Match fits object;
	// else the zero Owner (unowned). AWS objects have no registration and
	// resolve by rules alone.
	Owner(source model.SourceID, object model.ObjectKey) model.Owner

	// Filter fills the Owner of every endpoint, step and finding, and
	// returns the part of a the caller may see, default deny:
	//   - a ScopeAll caller sees everything;
	//   - other callers see only endpoints, paths, changes and findings with
	//     at least one end their scopes match;
	//   - on those paths, steps whose members belong to other owners or to
	//     nobody keep only their location (layer, direction, owner, ports and
	//     outcome) and are marked Redacted; an AWS step also keeps its
	//     resource IDs, never its rules;
	//   - the outcome and verdict stay, so a developer still sees where a
	//     path is blocked.
	// Filter never changes a verdict and never reveals a hidden endpoint by
	// name.
	Filter(c model.Caller, a model.Analysis) model.Analysis
}
