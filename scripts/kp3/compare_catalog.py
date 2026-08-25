#!/usr/bin/env python3
"""Compare a repo YJ symbol catalog with Kindle Previewer's live KAF table."""

from __future__ import annotations

import argparse
from pathlib import Path
import re
import subprocess
import tempfile

from run_probe import DEFAULT_PREVIEWER, ROOT, compile_probes, fc_paths, kaf_command


def parse_go_catalog(path: Path) -> dict[int, str]:
    text = path.read_text(encoding="utf-8")
    try:
        body = text.split("symbols:", 1)[1]
    except IndexError as exc:
        raise SystemExit(f"no symbols list found in {path}") from exc
    symbols = re.findall(r'"((?:[^"\\]|\\.)*)"', body)
    return {10 + index: bytes(symbol, "utf-8").decode("unicode_escape") for index, symbol in enumerate(symbols)}


def parse_live(text: str) -> dict[int, str]:
    result: dict[int, str] = {}
    for line in text.splitlines():
        if not line.strip():
            continue
        key, name = line.split("\t", 1)
        result[int(key)] = name
    return result


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--previewer", type=Path, default=DEFAULT_PREVIEWER)
    parser.add_argument("--catalog", type=Path, default=ROOT / "internal" / "kfx" / "catalog.ion")
    args = parser.parse_args()

    fc, jar, javac = fc_paths(args.previewer)
    with tempfile.TemporaryDirectory(prefix="kp3-catalog-") as tmp:
        classes = Path(tmp) / "classes"
        compile_probes(jar, javac, classes)
        command = kaf_command(fc, jar, classes, "KafPropertyCatalog")
        print("+", " ".join(command))
        proc = subprocess.run(command, check=True, text=True, stdout=subprocess.PIPE)

    old = parse_go_catalog(args.catalog)
    live = parse_live(proc.stdout)
    shared_ids = sorted(set(old) & set(live))
    mismatches = [(i, old[i], live[i]) for i in shared_ids if old[i] != live[i]]
    missing = [(i, old[i]) for i in sorted(set(old) - set(live))]
    additions = [(i, live[i]) for i in sorted(set(live) - set(old)) if i >= min(old, default=0)]

    print(f"catalog={args.catalog}")
    print(f"catalog_entries={len(old)} range={min(old)}..{max(old)}")
    print(f"live_entries={len(live)} range={min(live)}..{max(live)}")
    print(f"shared_ids={len(shared_ids)} exact_matches={len(shared_ids) - len(mismatches)} mismatches={len(mismatches)}")
    if missing:
        print("missing_from_live:")
        for i, name in missing:
            print(f"  {i}\t{name}")
    if mismatches:
        print("mismatches:")
        for i, old_name, live_name in mismatches:
            print(f"  {i}\t{old_name}\t=>\t{live_name}")
    if additions:
        print("live_additions:")
        for i, name in additions:
            print(f"  {i}\t{name}")

    if mismatches or missing:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
