#!/bin/bash
set -e

# Ensure Go binary is built
cd /Users/kai/dev/projects/kindle.koplugin
go build ./cmd/kindle-helper/

echo "Parity audit environment ready."
echo "Python reference: REFERENCE/Calibre_KFX_Input/kfxlib/"
echo "Go source: internal/kfx/"
echo "Compare original 6: bash /tmp/compare_all.sh"
echo "Compare new 4: bash /tmp/compare_new.sh"
