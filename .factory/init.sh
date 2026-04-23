#!/bin/bash
set -e

cd /Users/kai/dev/projects/kindle.koplugin

# Ensure Go binary is built (~5 seconds)
go build ./cmd/kindle-helper/

echo "Python→Go parity port environment ready."
echo "Python reference: REFERENCE/Calibre_KFX_Input/kfxlib/"
echo "Go source: internal/kfx/"
echo "Test: go test ./internal/kfx/ -count=1 -timeout 120s"
echo "Vet: go vet ./..."
echo "Parity: bash scripts/parity_check.sh"
echo "Diff: bash scripts/diff_kfx_parity.sh --summary-only"
