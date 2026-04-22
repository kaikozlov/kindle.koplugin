#!/bin/bash
set -e

# Ensure Go binary is built
cd /Users/kai/dev/projects/kindle.koplugin
go build ./cmd/kindle-helper/

echo "Go refactor environment ready."
echo "Python reference: REFERENCE/Calibre_KFX_Input/kfxlib/"
echo "Go source: internal/kfx/"
echo "Test: go test ./internal/kfx/ -count=1 -timeout 120s"
