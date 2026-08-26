#!/usr/bin/env python3
"""Export the checked-in semantic YJ symbol catalog to canonical JSON.

`internal/kfx/catalog.ion` is the runtime source of truth for the Go converter.
Its names are recovered from Kindle Previewer, not from KFX Input's anonymous
`$N` placeholder table. `scripts/kp3/compare_catalog.py` checks the
property-bearing range against Previewer's live PropertyNameUtil table, while
`run_probe.py --symbol-range ...` can verify the non-property shared tail via
DigitalBook.nativeGetSymbolName.

Usage:
    python3 scripts/export_yj_symbol_catalog.py \
        > internal/kfx/testdata/yj_symbols_golden.json
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]


def parse_catalog(path: Path) -> list[str]:
    text = path.read_text(encoding="utf-8")
    try:
        body = text.split("symbols:", 1)[1]
    except IndexError as exc:
        raise SystemExit(f"no symbols list found in {path}") from exc
    return [
        bytes(symbol, "utf-8").decode("unicode_escape")
        for symbol in re.findall(r'"((?:[^"\\]|\\.)*)"', body)
    ]


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--catalog", type=Path, default=ROOT / "internal" / "kfx" / "catalog.ion",
        help="semantic YJ shared-symbol catalog to export",
    )
    args = parser.parse_args()
    symbols = parse_catalog(args.catalog)
    print(json.dumps({
        "name": "YJ_symbols",
        "version": 10,
        "symbol_count": len(symbols),
        "symbols": symbols,
    }, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()
