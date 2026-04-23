#!/usr/bin/env python3
"""Export yj_symbol_catalog.py YJ_SYMBOLS to canonical JSON.

Source: REFERENCE/Calibre_KFX_Input/kfxlib/yj_symbol_catalog.py

Usage:
    python3 scripts/export_yj_symbol_catalog.py > internal/kfx/testdata/yj_symbols_golden.json

The output contains:
  - name:          shared table name (e.g. "YJ_symbols")
  - version:       shared table version (e.g. 10)
  - symbol_count:  number of symbols in the list
  - symbols_raw:        list as-is from Python (includes "?" suffixes)
  - symbols_normalized: list with "?" stripped (what add_symbol stores)
"""

import json
import sys
import os

# Add the kfxlib directory to sys.path so we can import the module.
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PROJECT_ROOT = os.path.dirname(SCRIPT_DIR)
KFXLIB_DIR = os.path.join(PROJECT_ROOT, "REFERENCE", "Calibre_KFX_Input", "kfxlib")

sys.path.insert(0, KFXLIB_DIR)

from yj_symbol_catalog import YJ_SYMBOLS

symbols_raw = list(YJ_SYMBOLS.symbols)
symbols_normalized = [s.rstrip("?") for s in symbols_raw]

data = {
    "name": YJ_SYMBOLS.name,
    "version": YJ_SYMBOLS.version,
    "symbol_count": len(symbols_raw),
    "symbols_raw": symbols_raw,
    "symbols_normalized": symbols_normalized,
}

json.dump(data, sys.stdout, indent=2)
print()  # trailing newline
