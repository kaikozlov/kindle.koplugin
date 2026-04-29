#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
python3 scripts/structural_diff_count.py --metric 2>&1
