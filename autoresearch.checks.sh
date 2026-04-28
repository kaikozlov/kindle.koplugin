#!/bin/bash
set -euo pipefail

# autoresearch.checks.sh — Verify Go tests still pass after parity changes

cd "$(dirname "${BASH_SOURCE[0]}")"

# Run Go tests - suppress verbose success output, show only errors
go test ./internal/kfx/... 2>&1 | tail -20
go test ./cmd/kindle-helper/... 2>&1 | tail -10
