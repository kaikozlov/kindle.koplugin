#!/bin/bash
set -euo pipefail
# Branch audit: count uncertain branches in core conversion files
cd "$(dirname "$0")"
python3 scripts/audit_missing_branches.py --metric 2>&1
