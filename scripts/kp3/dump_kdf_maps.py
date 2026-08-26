#!/usr/bin/env python3
"""Dump raw KDF position/location map fragments using the KDF's own symbol table.

Research tooling for the Kindle Previewer position-map investigation.
Reads the unwrapped SQLite copy in a kp3 workdir, imports the KDF's
$ion_symbol_table, then decodes the position/location-related blobs.
Not used by the plugin at runtime.
"""
import sqlite3
import sys
from pathlib import Path

sys.path.insert(0, "REFERENCE/KFX_Input")
sys.path.insert(0, "REFERENCE/KFX_Input/kfxlib/calibre-plugin-modules")

from kfxlib.ion_symbol_table import LocalSymbolTable, SymbolTableCatalog  # noqa: E402
from kfxlib.ion_binary import IonBinary  # noqa: E402

FINGERPRINT = b"\xfa\x50\x0a\x5f"


def unwrap(data: bytes) -> bytes:
    pos = 1024
    while len(data) >= pos + 1024 and data[pos:pos + 4] == FINGERPRINT:
        data = data[:pos] + data[pos + 1024:]
        pos += 1024 * 1024
    return data


def dump(sqlite_path: Path, ids=None) -> None:
    con = sqlite3.connect(f"file:{sqlite_path}?mode=ro", uri=True)
    cur = con.cursor()
    rows = list(cur.execute("SELECT id,payload_type,payload_value FROM fragments"))

    catalog = SymbolTableCatalog()
    catalog.add_global_shared_symbol_tables()
    symtab = LocalSymbolTable(catalog)
    symtab.creating_yj_local_symbols = True
    ion = IonBinary(symtab)

    for fid, ptype, value in rows:
        if fid == "$ion_symbol_table":
            ion.deserialize_annotated_value(bytes(value), expect_annotation="$ion_symbol_table", import_symbols=True)
            symtab.creating_yj_local_symbols = False
            print("== $ion_symbol_table ==")
            print("locals:", [symtab.get_symbol(n) for n in range(symtab.local_min_id, len(symtab.symbols) + 1)])
            break

    want = ids or ["$264", "$265", "$550", "$621", "c0-spm", "yj.section_pid_count_map", "max_id", "max_eid_in_sections"]
    for fid, ptype, value in rows:
        base = fid.replace("-spm", "")
        if fid in want or (fid.endswith("-spm") and "spm" in want) or fid.startswith("eidbucket"):
            try:
                val = ion.deserialize_single_value(bytes(value))
            except Exception as e:
                val = f"<decode error {e}>"
            print(f"== {fid} ({ptype}, {len(value)} bytes) ==")
            print("   ", repr(val)[:2000])
    con.close()


if __name__ == "__main__":
    for arg in sys.argv[1:]:
        p = Path(arg)
        print(f"### {p}")
        dump(p)
