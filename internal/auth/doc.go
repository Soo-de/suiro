// Package auth authenticates users (Argon2id password hashes) and CI tokens
// (stored as SHA-256 hashes, expiring, revocable, bound to one application)
// and authorizes actions with Casbin, using the policy file shipped in the
// Helm chart.
package auth
