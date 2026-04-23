#!/usr/bin/env python3
"""Export yj_versions.py KNOWN_FEATURES structure to canonical JSON.

Source: REFERENCE/Calibre_KFX_Input/kfxlib/yj_versions.py

Usage:
    python3 scripts/export_yj_versions.py > internal/kfx/testdata/yj_versions_golden.json

The output contains:
  - feature_keys: sorted dict of {category: [sorted feature keys]}
  - feature_keys_hash: SHA-256 of the canonical feature_keys JSON (for quick comparison)
"""

import json
import sys
import os
import hashlib

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PROJECT_ROOT = os.path.dirname(SCRIPT_DIR)
KFXLIB_DIR = os.path.join(PROJECT_ROOT, "REFERENCE", "Calibre_KFX_Input", "kfxlib")

sys.path.insert(0, KFXLIB_DIR)

from yj_versions import KNOWN_FEATURES

# Extract just the key structure: category -> sorted list of feature keys
feature_keys = {}
for cat, keys in sorted(KNOWN_FEATURES.items()):
    feature_keys[cat] = sorted(keys.keys())

# Compute a stable hash of the feature keys for quick diff detection
# Use compact separators to match Go's json.Marshal output (no spaces).
canonical = json.dumps(feature_keys, sort_keys=True, separators=(',', ':'))
feature_keys_hash = hashlib.sha256(canonical.encode("utf-8")).hexdigest()

data = {
    "feature_keys": feature_keys,
    "feature_keys_hash": feature_keys_hash,
}

json.dump(data, sys.stdout, indent=2)
print()
