#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
python3 scripts/audit_missing_branches.py --metric 2>&1
