// Package store persists analyses, results, users and CI tokens in
// PostgreSQL; schema changes are migrations under db/migrations. It keeps
// only network-relevant fields of a resource: Secret values and environment
// variables never reach the database.
package store
