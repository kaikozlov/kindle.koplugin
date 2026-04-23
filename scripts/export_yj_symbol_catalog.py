#!/usr/bin/env python3
"""Export YJ symbol catalog to canonical JSON for Go golden tests.

Sources:
  - REFERENCE/kfx_symbol_catalog.ion (real names, 842 symbols)
  - REFERENCE/Calibre_KFX_Input/kfxlib/yj_symbol_catalog.py (Python $N placeholders)

The golden compares Go's sharedTable() output against the real symbol names
from the ION catalog. Go resolves SID 10 to "language", SID 145 to "content", etc.

Usage:
    python3 scripts/export_yj_symbol_catalog.py > internal/kfx/testdata/yj_symbols_golden.json

Regenerate golden:
    python3 scripts/export_yj_symbol_catalog.py > internal/kfx/testdata/yj_symbols_golden.json
"""

import json
import re
import sys
import os

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PROJECT_ROOT = os.path.dirname(SCRIPT_DIR)

# Read the real symbol catalog (ION text format)
catalog_path = os.path.join(PROJECT_ROOT, "REFERENCE", "kfx_symbol_catalog.ion")
with open(catalog_path) as f:
    text = f.read()

# Extract symbol names from the ION text
symbols = re.findall(r'"([^"]+)"', text.split('symbols:')[1])

data = {
    "name": "YJ_symbols",
    "version": 10,
    "symbol_count": len(symbols),
    "symbols": symbols,
}

json.dump(data, sys.stdout, indent=2)
print()  # trailing newline
