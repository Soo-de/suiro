// Package inputs keeps the list of sources to analyse, loaded from the
// registration file that Helm mounts from the platform repository. It asks
// the collectors for snapshots and skips re-analysis when the fingerprint of
// all inputs (commit SHAs, AWS read version, rule-set version, tool version)
// is unchanged.
package inputs
