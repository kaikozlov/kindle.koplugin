#!/usr/bin/env python3
"""Dump raw KDF position/location map fragments using the KDF's own symbol table.

Research tooling for the Kindle Previewer position-map investigation.
Reads the unwrapped SQLite copy in a kp3 workdir, imports the KDF's
$ion_symbol_table, then decodes the position/location-related blobs.
Fragment ids are matched in both numeric ($264) and named (position_map)
spellings because different producer paths use either form.
Not used by the plugin at runtime.
"""
import argparse
import sqlite3
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "REFERENCE" / "KFX_Input"))
sys.path.insert(
    0,
    str(Path(__file__).resolve().parents[2] / "REFERENCE" / "KFX_Input" / "kfxlib" / "calibre-plugin-modules"),
)

from kfxlib.ion_symbol_table import LocalSymbolTable, SymbolTableCatalog  # noqa: E402
from kfxlib.ion_binary import IonBinary  # noqa: E402

FINGERPRINT = b"\xfa\x50\x0a\x5f"

# numeric and named spellings of the map fragment families
MAP_FRAGMENTS = [
    "$264", "position_map",
    "$265", "position_id_map",
    "$550", "location_map",
    "$621", "yj.location_pid_map",
    "$610", "yj.eidhash_eid_section_map",
    "$611", "yj.section_pid_count_map",
    "max_id", "max_eid_in_sections",
]
DEFAULT_WANT = [
    "$264", "position_map",
    "$265", "position_id_map",
    "$550", "location_map",
    "$621", "yj.location_pid_map",
    "$611", "yj.section_pid_count_map",
    "max_id",
]
TRUNCATE_AT = 2000


def unwrap(data: bytes) -> bytes:
    pos = 1024
    while len(data) >= pos + 1024 and data[pos:pos + 4] == FINGERPRINT:
        data = data[:pos] + data[pos + 1024:]
        pos += 1024 * 1024
    return data


def render(value, truncate: bool) -> str:
    text = repr(value)
    if truncate and len(text) > TRUNCATE_AT:
        return text[:TRUNCATE_AT] + f" …[truncated at {TRUNCATE_AT} chars, use --no-truncate]"
    return text


def dump(sqlite_path: Path, want: list[str], truncate: bool, spm: bool) -> None:
    con = sqlite3.connect(f"file:{sqlite_path}?mode=ro", uri=True)
    cur = con.cursor()
    rows = list(cur.execute("SELECT id,payload_type,payload_value FROM fragments"))

    # NOTE: LocalSymbolTable's first positional parameter is initial_import;
    # the catalog must be passed by keyword or the catalog object itself is
    # treated as a shared-table name and logged as unknown.
    catalog = SymbolTableCatalog()
    catalog.add_global_shared_symbol_tables()
    symtab = LocalSymbolTable(catalog=catalog)
    symtab.creating_yj_local_symbols = True
    ion = IonBinary(symtab)

    for fid, _ptype, value in rows:
        if fid == "$ion_symbol_table":
            ion.deserialize_annotated_value(bytes(value), expect_annotation="$ion_symbol_table", import_symbols=True)
            symtab.creating_yj_local_symbols = False
            print("== $ion_symbol_table ==")
            print("locals:", [symtab.get_symbol(n) for n in range(symtab.local_min_id, len(symtab.symbols) + 1)])
            break
    else:
        print("warning: no $ion_symbol_table fragment found; decoding with shared symbols only",
              file=sys.stderr)

    want_set = set(want)
    include_buckets = any(name in want_set for name in ("$610", "yj.eidhash_eid_section_map"))
    for fid, ptype, value in rows:
        matched = (
            fid in want_set
            or (spm and fid.endswith("-spm"))
            or (include_buckets and fid.startswith("eidbucket"))
        )
        if not matched:
            continue
        try:
            val = ion.deserialize_single_value(bytes(value))
        except Exception as e:  # noqa: BLE001 - report and continue with other fragments
            val = f"<decode error {e}>"
        print(f"== {fid} ({ptype}, {len(value)} bytes) ==")
        print("   ", render(val, truncate))
    con.close()


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("kdf", nargs="+", type=Path, help="unwrapped KDF SQLite copies (e.g. workdir/book.unwrapped.kdf)")
    parser.add_argument(
        "--fragments", nargs="*", default=None, metavar="ID",
        help=f"fragment ids to dump (numeric or named); default: {' '.join(DEFAULT_WANT)}",
    )
    parser.add_argument("--spm", action="store_true", help="also dump per-section <section>-spm fragments ($609)")
    parser.add_argument("--no-truncate", action="store_true", help="do not truncate long values")
    args = parser.parse_args()

    want = args.fragments if args.fragments is not None else DEFAULT_WANT
    for p in args.kdf:
        print(f"### {p}")
        dump(p, want, not args.no_truncate, args.spm)


if __name__ == "__main__":
    main()
