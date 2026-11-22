// Package testutil builds test inputs from what a person can read and
// check: rendered Kubernetes YAML becomes a model.Snapshot the way the
// renderer publishes one, and a model.Bundle is put together around
// snapshots. It is imported only from tests.
//
// Expected results come from outside this code. Each test group keeps a
// testdata/ORACLE.md that names, for every case, where its expected result
// comes from (a Kubernetes documentation example, netpol-analyzer at a
// stated version and command, a measurement on the test cluster) and why
// the case is there; a case worked out by hand states its reasoning.
package testutil
