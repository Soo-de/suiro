package githubapp

import (
	"context"
	"time"
)

// Signer signs with the GitHub App's private key without exposing it. It is
// not a separate service: an implementation inside git-collector either
// signs with the key file mounted from a Secret (the lab) or asks AWS KMS,
// where the key cannot be exported and every signature is logged in
// CloudTrail (EKS). Helm values choose one (signer: file | kms); the code
// using it is the same.
type Signer interface {
	// Sign returns the RSASSA-PKCS1-v1_5 signature of a 32-byte SHA-256
	// digest, the RS256 signature GitHub verifies on the app JWT. It
	// rejects digests of any other length.
	Sign(ctx context.Context, digest []byte) ([]byte, error)
}

// TokenSource hands out installation tokens. The implementation here builds
// the app JWT (issued 60 s in the past, valid 10 minutes), has its digest
// signed by a Signer, and exchanges it for a token limited to one
// repository with contents:read, cached in memory until shortly before it
// expires (about an hour). Only git-collector holds a TokenSource.
type TokenSource interface {
	// Token returns a token for the repository at repoURL, reusing a cached
	// one while it has time left.
	Token(ctx context.Context, repoURL string) (Token, error)
}

// Token is an installation token. Value is a credential: it is passed to
// git through a credential helper, never through the environment or a
// command line, and never logged.
type Token struct {
	Value      string
	Repository string
	ExpiresAt  time.Time
}
