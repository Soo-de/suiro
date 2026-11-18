package model

// Role is a caller's role. Roles stay two; what a caller sees is narrowed by
// Scopes, not by more roles. Which role may do which action is a line in
// the Casbin policy file, never a role name in handler code.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Scope is a visibility pattern: "<team>/<application>", "<team>/*", or
// ScopeAll. A caller sees in full the paths and gates whose owner a scope
// matches.
type Scope string

// ScopeAll sees the whole map, every gate's content included (platform and
// security teams).
const ScopeAll Scope = "*"

// Caller is who a result is filtered for: a signed-in user, or a CI token,
// whose single scope is the application it is bound to.
type Caller struct {
	// Subject is the user name, or "ci-token:<prefix>".
	Subject string  `json:"subject"`
	Role    Role    `json:"role"`
	Scopes  []Scope `json:"scopes"`
}
