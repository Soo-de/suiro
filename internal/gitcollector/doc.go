// Package gitcollector is the git-collector service. It fetches the
// registered repositories at the requested branch, sends their files to the
// renderer and returns snapshots to the server. It keeps the source list
// only in memory, has no database access, and is the only service that can
// authenticate to GitHub.
package gitcollector
