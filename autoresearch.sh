#!/bin/bash
set -euo pipefail
# Structural parity audit: count missing functions between Python and Go
cd "$(dirname "$0")"
python3 scripts/audit_parity.py --metric 2>&1
