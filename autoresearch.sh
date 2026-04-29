#!/bin/bash
set -euo pipefail
# Branch parity audit: count missing branches in core conversion files
cd "$(dirname "$0")"
python3 scripts/audit_missing_branches.py --metric 2>&1
