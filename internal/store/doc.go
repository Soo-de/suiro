// Package store persists reads, analyses, rules, exceptions, the
// admin-entered cluster profiles, users, CI tokens and the audit log in
// PostgreSQL; schema changes are migrations under db/migrations. Store is
// the contract; ReadStore, AnalysisStore, RuleStore, ProfileStore,
// AccountStore and AuditLog are its parts, so each caller depends only on
// what it uses. Only the server holds a Store. It keeps only network-relevant
// fields of a resource (model.KeptFields): Secret values and environment
// variables never reach the database.
package store
