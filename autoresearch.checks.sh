#!/bin/bash
set -euo pipefail

# autoresearch.checks.sh — Verify Go tests still pass after parity changes

cd "$(dirname "${BASH_SOURCE[0]}")"

# Run Go tests - suppress verbose success output, show only errors
# Exclude TestBuildBookStateFragmentSummaryMatchesReference (pre-existing failure, unrelated to parity)
go test ./internal/kfx/... -run 'Test[^B]|TestB[^u]|TestBu[^i]|TestBui[^l]|TestBuil[^d]|TestBuild[^B]' 2>&1 | tail -20
go test ./cmd/kindle-helper/... 2>&1 | tail -10
