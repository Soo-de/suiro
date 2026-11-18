#!/usr/bin/env bash
# Fails a pull request that modifies, renames or deletes an existing Go test
# file or anything under a testdata/ directory, or adds a Skip call, unless the
# PR carries the test-change-approved label. New test files are always fine.
#
# Expected results come from outside the code (documentation examples,
# netpol-analyzer, lab measurements), so a failing test is first assumed to be
# right. The label is given by the orchestrator only after checking that the
# test itself was wrong, not that the code could not meet it.
set -euo pipefail

base="${BASE:?BASE must be the pull request base commit}"
approved="${APPROVED:-false}"

touched="$(git diff --name-status "$base"...HEAD -- '*_test.go' ':(glob)**/testdata/**' |
	awk '$1 ~ /^(M|D|R)/ { print $1 "\t" $2 }')"

skips="$(git diff -U0 "$base"...HEAD -- '*.go' |
	grep -E '^\+[^+]' | grep -E '\.Skip(Now|f)?\(' || true)"

if [[ -z "$touched" && -z "$skips" ]]; then
	echo "No existing test or test data was changed, and no Skip was added."
	exit 0
fi

if [[ -n "$touched" ]]; then
	echo "Existing tests or test data changed (M=modified, D=deleted, R=renamed):"
	echo "$touched"
fi
if [[ -n "$skips" ]]; then
	echo "Added Skip calls:"
	echo "$skips"
fi

if [[ "$approved" == "true" ]]; then
	echo "Approved by the test-change-approved label."
	exit 0
fi

echo "::error::This PR weakens existing tests. Fix the code instead, or get the test-change-approved label after review."
exit 1
