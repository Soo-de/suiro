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

	// Filter fills the Owner of every endpoint and finding and returns the
	// part of a the caller may see, default deny:
	//   - a ScopeAll caller sees everything;
	//   - other callers see only endpoints, paths, changes and findings with
	//     at least one end their scopes match;
	//   - on a pull request analysis, a caller allowed to ask about that pull
	//     request (one of its scopes matches the owner of a source registered
	//     on the pull request's repository) also sees every Change and every
	//     New finding, with the endpoints and paths they name, so a pull
	//     request that breaks another team's required path tells its author
	//     which expectation broke and who owns it;
	//   - verdicts stay, so a developer still sees where a path is blocked.
	// Filter never changes a verdict and never reveals a hidden endpoint by
	// name.
	Filter(c model.Caller, a model.Analysis) model.Analysis

	// FilterDetail fills the Owners of every step of d, a path of a, and
	// returns the part the caller may see, or false when Filter would not
	// show the caller that path. Each step is redacted member by member as
	// model.GateRef.Redacted defines: members of other owners or of nobody
	// lose their rules, and Kubernetes ones their IDs too; outcomes and the
	// verdict stay.
	FilterDetail(c model.Caller, a model.Analysis, d model.PathDetail) (model.PathDetail, bool)
}
