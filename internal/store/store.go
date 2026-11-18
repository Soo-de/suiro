package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Soo-de/suiro/internal/model"
)

// ErrNotFound is returned when the asked-for record does not exist.
var ErrNotFound = errors.New("store: not found")

// Store is everything the server persists. The PostgreSQL implementation
// is the product; an in-memory one serves tests.
type Store interface {
	ReadStore
	AnalysisStore
	RuleStore
	AccountStore
	AuditLog
}

// ReadStore keeps collectors' reads. Snapshot bodies are stored once per
// digest; history comes from here because neither GitHub nor AWS keeps it.
type ReadStore interface {
	// SaveRead records a complete read. When the latest read of the same key
	// has the same digest, only its LastCompleteReadAt moves.
	SaveRead(ctx context.Context, s model.Snapshot) error
	// LatestReads returns the latest record of every ReadKey, for inputs to
	// compare with what collectors announce after a restart.
	LatestReads(ctx context.Context) ([]ReadRecord, error)
	// Snapshot loads a stored snapshot by digest.
	Snapshot(ctx context.Context, d model.Digest) (model.Snapshot, error)
}

// ReadRecord is a stored read without its body.
type ReadRecord struct {
	Key                model.ReadKey
	PullRequest        *model.PullRequest
	Revision           string
	Digest             model.Digest
	ContractVersion    int
	ReadAt             time.Time
	LastCompleteReadAt time.Time
}

// AnalysisStore keeps analyses with their paths, changes and findings.
type AnalysisStore interface {
	// FindAnalysis returns the ID of a stored analysis with this
	// fingerprint, or ErrNotFound; inputs skips the work when one exists.
	FindAnalysis(ctx context.Context, fingerprint model.Digest) (model.AnalysisID, error)
	// SaveAnalysis stores a and returns its new ID.
	SaveAnalysis(ctx context.Context, a model.Analysis) (model.AnalysisID, error)
	Analysis(ctx context.Context, id model.AnalysisID) (model.Analysis, error)
	// LatestBaseline returns the environment's newest baseline whose status
	// is AnalysisDone or AnalysisPartial.
	LatestBaseline(ctx context.Context, environment string) (model.Analysis, error)
	// LatestPullRequest returns the newest analysis of one pull request.
	LatestPullRequest(ctx context.Context, repoURL string, number int) (model.Analysis, error)
}

// RuleStore keeps forbidden-path rules, hygiene checks and exceptions,
// versioned: rows are appended, never changed in place.
type RuleStore interface {
	// RuleSet returns the active rules as of version; 0 means current.
	RuleSet(ctx context.Context, version int64) (model.RuleSet, error)
	// Rules returns the latest revision of every rule, disabled ones
	// included, for the admin page.
	Rules(ctx context.Context) ([]RuleRevision, error)
	// SaveRule appends a revision and returns the new rule-set version.
	SaveRule(ctx context.Context, r RuleRevision) (int64, error)
	// Exceptions returns the exceptions in force at t: not revoked and not
	// expired.
	Exceptions(ctx context.Context, at time.Time) ([]Exception, error)
	SaveException(ctx context.Context, e Exception) (int64, error)
	RevokeException(ctx context.Context, id int64, at time.Time) error
}

// RuleState is a rule revision's state.
type RuleState string

const (
	RuleActive   RuleState = "active"
	RuleDisabled RuleState = "disabled"
	RuleDeleted  RuleState = "deleted"
)

// RuleRevision is one revision of a forbidden path or hygiene rule. Exactly
// one of Path and Hygiene is set.
type RuleRevision struct {
	// Version is the rule-set version this revision created; set by
	// SaveRule.
	Version     int64
	Key         string
	Builtin     bool
	Name        string
	Description string
	Path        *model.Expectation
	Hygiene     *model.HygieneRule
	State       RuleState
	// ChangedBy is empty for the install seed.
	ChangedBy string
	ChangedAt time.Time
}

// Exception is a time-limited permission for one forbidden path or object.
// It is matched to findings when results are read, by Rule and Subject.
type Exception struct {
	ID          int64
	Rule        string
	Subject     string
	RequestedBy string
	Reason      string
	ExpiresAt   time.Time
	CreatedBy   string
	RevokedAt   time.Time
}

// AccountStore keeps local users and CI tokens. Neither a password nor a
// token is stored, only its hash.
type AccountStore interface {
	User(ctx context.Context, name string) (User, error)
	SaveUser(ctx context.Context, u User) error
	// CIToken looks a token up by the SHA-256 of its value.
	CIToken(ctx context.Context, hash model.Digest) (CIToken, error)
	SaveCIToken(ctx context.Context, t CIToken) (int64, error)
	RevokeCIToken(ctx context.Context, id int64, at time.Time) error
}

// User is a local account.
type User struct {
	Name string
	// PasswordHash is an Argon2id hash in PHC string form.
	PasswordHash string
	Role         model.Role
	Scopes       []model.Scope
	// DisabledAt is set instead of deleting, so the audit log keeps its
	// actor.
	DisabledAt time.Time
}

// CIToken is a CLI credential bound to one application: it may only ask
// for results within that application's scope, and it expires.
type CIToken struct {
	ID int64
	// Prefix is the token's first characters, to recognise it in lists.
	Prefix    string
	Hash      model.Digest
	Owner     model.Owner
	CreatedBy string
	ExpiresAt time.Time
	RevokedAt time.Time
}

// AuditLog records every change of rules, exceptions, tokens, users and the
// cluster profile with who and when. It is append-only: the server's
// database role has no UPDATE or DELETE on it.
type AuditLog interface {
	AppendAudit(ctx context.Context, e AuditEntry) error
}

// AuditEntry is one recorded change. Old and New never hold a password,
// token or other secret.
type AuditEntry struct {
	At time.Time
	// Actor is the user name at the time, or "system".
	Actor string
	// Action is "<object type>.<verb>", e.g. "forbidden_rule.update".
	Action     string
	ObjectType string
	ObjectID   string
	Old        json.RawMessage
	New        json.RawMessage
}
