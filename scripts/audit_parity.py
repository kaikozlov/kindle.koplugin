#!/usr/bin/env python3
"""
audit_parity.py — Honest Python→Go function parity audit.

A match is a NAME match plus a SUBSTANCE check. A Go function that merely
shares a Python function's name does not count as an implementation:
if its body is empty, returns only constants/nil/identity values, or only
returns errors, and the Python function is substantive, it is counted as
a STUB.

Classification per Python function:
  implemented        name-matched, Go body substantive relative to Python
  implemented_trivial both sides are trivial (Python function is a no-op too)
  stub_silent        name-matched but hollow; worse: pretends to be a port
  stub_admitted      name-matched, returns "not implemented" style errors
  thin               Go body exists but is far smaller than the Python body
  missing            no name-matched Go function at all
  excluded           explicitly waived in scripts/parity_exclusions.json
                     with a reviewable reason; NEVER counted as implemented

Metrics are honest by construction:
  - excluded functions are removed from the parity denominator but always
    reported on their own metric line and validated for schema + evidence
  --strict puts exclusions back in the denominator (worst-case view).

Usage:
  python3 scripts/audit_parity.py                  # Full audit + report
  python3 scripts/audit_parity.py --metric         # Just the METRIC lines
  python3 scripts/audit_parity.py --file yj_book   # Single file
  python3 scripts/audit_parity.py --json           # JSON output
  python3 scripts/audit_parity.py --report PARITY_REPORT.md
  python3 scripts/audit_parity.py --init-exclusions  # skeleton exclusions
"""

import ast
import json
import os
import re
import subprocess
import sys
import argparse
from functools import lru_cache
from dataclasses import dataclass, field
from typing import Optional

BASE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PY_DIR = os.path.join(BASE, "REFERENCE/KFX_Input/kfxlib")
GO_DIR = os.path.join(BASE, "internal/kfx")
PYTAGO_DIR = os.path.join(BASE, "REFERENCE/pytago_test_new/go_output")
GOFUNCINFO_TOOL = os.path.join(BASE, "scripts/gofuncinfo")
DEFAULT_EXCLUSIONS = os.path.join(BASE, "scripts/parity_exclusions.json")

# ---------------------------------------------------------------------------
# Honesty thresholds (tunable; tested in scripts/tests/test_audit_parity.py)
# ---------------------------------------------------------------------------
PY_TRIVIAL_MAX = 2   # py_nstmt <= this → Python function itself is trivial
PY_BIG = 8           # py_nstmt >= this → demand real Go substance
THIN_RATIO = 0.35    # go_nstmt < py_nstmt * ratio → "thin" suspect port

EXCLUSION_CATEGORIES = {
    "library-replacement",      # behavior provided by a Go library instead
    "alternate-architecture",   # behavior lives in differently-named Go code
    "output-mode-out-of-scope", # Calibre output mode this plugin never uses
    "input-mode-out-of-scope",  # Calibre input location/scan mode not ported
    "calibre-plugin-infra",     # Calibre plugin/GUI infrastructure
    "unused-direction",         # encode/serialize path never needed (decode-only)
    "debug-only",               # logging/reporting/bookkeeping only
}
EVIDENCE_REQUIRED = {"library-replacement", "alternate-architecture"}


@dataclass
class PyFunc:
    name: str
    class_name: Optional[str]
    line_start: int
    line_end: int
    args: str
    docstring_first_line: Optional[str]
    nstmt: int = 0
    nlit: int = 0

    @property
    def is_dunder(self):
        return self.name.startswith("__") and self.name.endswith("__")

    @property
    def is_private(self):
        return self.name.startswith("_") and not self.is_dunder

    @property
    def py_trivial(self):
        return self.substance <= PY_TRIVIAL_MAX

    @property
    def substance(self):
        return self.nstmt + self.nlit

    @property
    def key(self):
        return (self.class_name or "", self.name)


def snake_to_camel(name: str) -> str:
    parts = name.split("_")
    if len(parts) == 1:
        return name
    return parts[0].lower() + "".join(p.capitalize() for p in parts[1:])


def snake_to_exported(name: str) -> str:
    return "".join(p.capitalize() for p in name.split("_"))


def count_stmts(node) -> tuple[int, int]:
    """Count statement nodes and literal elements, excluding nested
    function/class bodies (they are audited as their own functions)."""
    n = 0
    lit = 0
    for child in ast.iter_child_nodes(node):
        if isinstance(child, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
            continue
        if isinstance(child, ast.stmt):
            n += 1
        if isinstance(child, (ast.List, ast.Set, ast.Tuple)):
            lit += len(child.elts)
        elif isinstance(child, ast.Dict):
            lit += len(child.keys)
        sub_n, sub_lit = count_stmts(child)
        n += sub_n
        lit += sub_lit
    return n, lit


def extract_python_functions(filepath: str) -> list[PyFunc]:
    """Extract all function definitions from a Python file, preserving class context."""
    with open(filepath, "r") as f:
        source = f.read()
    try:
        tree = ast.parse(source)
    except SyntaxError:
        return []

    result = []

    def visit(node, class_name=None):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            arg_names = [a.arg for a in node.args.args if a.arg != "self"]
            doc = None
            doc_stmts = 0
            if (node.body and isinstance(node.body[0], ast.Expr) and
                isinstance(node.body[0].value, ast.Constant) and
                isinstance(node.body[0].value.value, str)):
                doc = node.body[0].value.value.split("\n")[0][:80]
                doc_stmts = 1
            nstmt, nlit = count_stmts(node)
            result.append(PyFunc(
                name=node.name,
                class_name=class_name,
                line_start=node.lineno,
                line_end=node.end_lineno or node.lineno,
                args=", ".join(arg_names),
                docstring_first_line=doc,
                nstmt=max(0, nstmt - doc_stmts),
                nlit=nlit,
            ))
            for child in node.body:
                visit(child)
        elif isinstance(node, ast.ClassDef):
            for child in node.body:
                visit(child, class_name=node.name)

    for node in tree.body:
        visit(node)
    return result


# ---------------------------------------------------------------------------
# Go side: name extraction (legacy, kept for --file same-file listing)
# ---------------------------------------------------------------------------

def extract_go_functions(filepath: str) -> dict[str, list[int]]:
    """Extract Go function names → list of line numbers where they appear."""
    if not os.path.exists(filepath):
        return {}
    with open(filepath, "r") as f:
        lines = f.readlines()

    funcs = {}
    for i, line in enumerate(lines, 1):
        m = re.match(r"^func\s+(?:\([^)]*\)\s+)?(\w+)\s*[\(]", line)
        if m:
            name = m.group(1)
            if name not in funcs:
                funcs[name] = []
            funcs[name].append(i)
    return funcs


# Name overrides: short Python names that have Go equivalents with different names
SHORT_NAME_MAP = {
    "fid": ["fid", "getFID", "getFid"],
    "ftype": ["ftype", "getFType", "getFtype", "getFTypes"],
    "keys": ["keys", "styleKeys"],
    "items": ["items", "styleItems"],
    "get": ["get", "styleGet"],
    "copy": ["copy", "styleCopy"],
    "pop": ["pop", "stylePop"],
    "clear": ["clear", "styleClear"],
    "update": ["update", "styleUpdate"],
    "partition": ["partition", "stylePartition"],
    "remove_default_properties": ["removeDefaultProperties", "styleRemoveDefaultProperties"],
    "tostring": ["tostring", "styleTostring", "String"],
    "Style": ["Style", "newStyle"],
    "advance": ["advance", "posDataAdvance"],
    "chunk": ["chunk", "posDataChunk"],
    "at_end": ["atEnd", "posDataAtEnd"],
    "head": ["head", "bookPartHead"],
    "body": ["body", "bookPartBody"],
    "__hash__": ["Hash", "ionHash"],
    "__contains__": ["Contains", "ionContains", "styleContains"],
    "__setitem__": ["SetItem", "ionSetItem", "styleSetItem"],
    "__new__": ["New", "ionNew"],
    "__ne__": ["Ne", "ionNe"],
    "__le__": ["Le", "ionLe"],
    "__gt__": ["Gt", "ionGt"],
    "__ge__": ["Ge", "ionGe"],
    "__copy__": ["Copy", "ionCopy", "styleCopy"],
    "__deepcopy__": ["Deepcopy", "ionDeepcopy"],
    "format": ["format", "ionFormat"],
    "deserialize": ["deserialize", "ionDeserialize", "deserializeContainer", "deserializeEntity"],
    "serialize": ["serialize", "ionSerialize", "serializeContainer", "serializeEntity"],
    "fixup": ["fixup", "epubFixup", "epubFixupNS"],
    "ion_type": ["ionType", "detectIonType"],
    "sort_key": ["sortKey"],
}


def expected_go_names(pf: PyFunc) -> list[str]:
    """All possible Go function names for a Python function."""
    names = []

    camel = snake_to_camel(pf.name)
    exported = snake_to_exported(pf.name)

    if pf.name in SHORT_NAME_MAP:
        names.extend(SHORT_NAME_MAP[pf.name])

    # Dunder names must NOT fall back to camel/exported forms: e.g.
    # snake_to_camel("__init__") == "Init" would false-match any Go init().
    # Only explicit mappings and the constructor/dunder aliases below apply.
    if not (pf.name.startswith("__") and pf.name.endswith("__")):
        names.extend([camel, exported])

    if pf.name == "__init__" and pf.class_name:
        cls_exported = snake_to_exported(pf.class_name)
        cls_camel = snake_to_camel(pf.class_name)
        names.extend([
            f"new{cls_exported}", f"New{cls_exported}",
            f"new{cls_camel}", f"New{cls_camel}",
            cls_camel, cls_exported,
        ])

    if pf.name in ("__repr__", "__str__"):
        names.extend(["String", "GoString"])
    if pf.name == "__eq__":
        names.append("Equal")
    if pf.name == "__lt__":
        names.append("Less")
    if pf.name == "__len__":
        names.append("Len")
    if pf.name == "__hash__":
        names.append("Hash")
    if pf.name == "__getitem__":
        names.extend(["Get", "At"])
    if pf.name == "__contains__":
        names.append("Contains")
    if pf.name == "__copy__":
        names.append("Copy")
    if pf.name == "__deepcopy__":
        names.append("DeepCopy")

    return list(dict.fromkeys(names))


# Which Python files to audit (only kfxlib conversion files, not Calibre plugin infra)
FILES_TO_AUDIT = [
    "yj_to_epub_content.py",
    "yj_to_epub_properties.py",
    "yj_to_epub_misc.py",
    "yj_to_epub_navigation.py",
    "yj_to_epub_resources.py",
    "yj_to_epub.py",
    "yj_to_epub_metadata.py",
    "yj_to_epub_illustrated_layout.py",
    "yj_to_epub_notebook.py",
    "yj_to_image_book.py",
    "yj_book.py",
    "yj_container.py",
    "yj_metadata.py",
    "yj_position_location.py",
    "yj_structure.py",
    "yj_symbol_catalog.py",
    "yj_versions.py",
    "epub_output.py",
    "ion.py",
    "ion_binary.py",
    "ion_symbol_table.py",
    "kfx_container.py",
]


# ---------------------------------------------------------------------------
# Go function substance index (from scripts/gofuncinfo)
# ---------------------------------------------------------------------------

@lru_cache(maxsize=1)
def gofuncinfo(path=None) -> dict:
    """Run (or load) gofuncinfo and index Go functions by lowercase name.

    Returns {"functions": [FuncInfo...], "by_lower": {name: [FuncInfo...]}}.
    """
    if path and os.path.exists(path):
        with open(path) as f:
            data = json.load(f)
    else:
        proc = subprocess.run(
            ["go", "run", "./" + os.path.relpath(GOFUNCINFO_TOOL, BASE).replace(os.sep, "/"),
             "internal", "cmd"],
            capture_output=True, text=True, cwd=BASE,
        )
        if proc.returncode != 0:
            raise RuntimeError(f"gofuncinfo failed: {proc.stderr.strip()}")
        data = json.loads(proc.stdout)

    funcs = data.get("functions", [])
    by_lower = {}
    for f in funcs:
        by_lower.setdefault(f["name"].lower(), []).append(f)
    data["by_lower"] = by_lower
    return data


def go_trivial(go_fn: dict) -> bool:
    return bool(go_fn.get("empty") or go_fn.get("const_only") or go_fn.get("error_only"))


def go_substance(go_fn: dict) -> int:
    return go_fn.get("nstmt", 0) + go_fn.get("nlit", 0)


def _resolve_callee(go_index: dict, callee: str, from_fn: dict) -> Optional[dict]:
    """Resolve a called name to a scanned Go function, preferring the same file."""
    candidates = go_index["by_lower"].get(callee.lower())
    if not candidates:
        return None
    for c in candidates:
        if c["file"] == from_fn["file"]:
            return c
    return candidates[0]


def transitive_substance(go_index: dict, go_fn: dict) -> int:
    """Total substance reachable through same-module calls, each function
    counted once (cycle-safe). Credits one-line delegation wrappers: a
    wrapper delegating to a substantive implementation is implemented, a
    wrapper calling only library functions is not."""
    cache = go_index.setdefault("_tsub_cache", {})

    def visit(fn: dict, visiting: set) -> int:
        key = (fn["file"], fn["name"], fn["line"])
        if key in cache:
            return cache[key]
        if key in visiting:
            return 0  # cycle
        visiting.add(key)
        total = go_substance(fn)
        for callee in fn.get("calls", []):
            target = _resolve_callee(go_index, callee, fn)
            if target is not None:
                total += visit(target, visiting)
        visiting.discard(key)
        cache[key] = total
        return total

    return visit(go_fn, set())


def internal_delegates(go_index: dict, go_fn: dict) -> list[str]:
    """Direct callees that resolve to scanned Go functions (for the report)."""
    out = []
    for callee in go_fn.get("calls", []):
        if _resolve_callee(go_index, callee, go_fn) is not None:
            out.append(callee)
    return out


def classify(py: PyFunc, go_fn: Optional[dict], excluded_entry: Optional[dict],
             tsub: Optional[int] = None) -> str:
    """Classify one Python function against its matched Go function."""
    if excluded_entry is not None:
        return "excluded"
    if go_fn is None:
        return "missing"
    if go_fn.get("notimpl"):
        return "stub_admitted"
    if go_trivial(go_fn):
        return "implemented_trivial" if py.py_trivial else "stub_silent"
    if py.substance >= PY_BIG:
        need = py.substance * THIN_RATIO
        if go_substance(go_fn) < need:
            if tsub is not None and tsub >= need:
                return "implemented_delegation"
            return "thin"
    return "implemented"


# ---------------------------------------------------------------------------
# Exclusions manifest
# ---------------------------------------------------------------------------

def load_exclusions(path: str) -> list[dict]:
    if not os.path.exists(path):
        return []
    with open(path) as f:
        data = json.load(f)
    entries = data.get("exclusions", data if isinstance(data, list) else [])
    return entries


def validate_exclusions(entries: list[dict], pyfuncs_by_file: dict[str, list[PyFunc]],
                         go_index: dict) -> tuple[list[str], list[dict], set]:
    """Validate exclusion entries.

    Returns (problems, valid_entries, valid_keys). Only valid entries may be
    applied — an invalid entry (unknown category, lazy reason, unresolvable
    or trivial evidence, stale target) is IGNORED for classification and
    reported as a problem, so a manifest of junk exclusions can never green
    the metric.
    """
    problems = []
    valid = set()
    valid_entries = []
    for i, e in enumerate(entries):
        where = f"exclusions[{i}]"
        entry_problems = []
        if e.get("category") not in EXCLUSION_CATEGORIES:
            entry_problems.append(
                f"{where}: unknown category {e.get('category')!r}; "
                f"allowed: {sorted(EXCLUSION_CATEGORIES)}")
        reason = (e.get("reason") or "").strip()
        if len(reason) < 10:
            entry_problems.append(f"{where}: reason too short to be reviewable "
                                  f"({len(reason)} chars, need >= 10)")
        py_file = e.get("py_file")
        if py_file not in pyfuncs_by_file:
            entry_problems.append(f"{where}: py_file {py_file!r} is not an audited file")
            problems.extend(entry_problems)
            continue
        matches = [pf for pf in pyfuncs_by_file[py_file]
                   if pf.name == e.get("py_name")
                   and (e.get("py_class") in (None, "", pf.class_name))]
        if not matches:
            entry_problems.append(f"{where}: no audited Python function matches "
                                  f"{py_file}:{e.get('py_class')}.{e.get('py_name')}")
            problems.extend(entry_problems)
            continue
        if e.get("category") in EVIDENCE_REQUIRED:
            ev = e.get("evidence") or []
            if not ev:
                entry_problems.append(f"{where}: category {e['category']} requires evidence")
            for ev_i, target in enumerate(ev):
                fn = find_go_evidence(go_index, target)
                if fn is None:
                    entry_problems.append(f"{where}.evidence[{ev_i}]: no Go function "
                                          f"{target.get('go_func')!r} in {target.get('go_file')!r}")
                elif go_trivial(fn) or fn["nstmt"] < 3:
                    entry_problems.append(f"{where}.evidence[{ev_i}]: {target.get('go_func')!r} "
                                          f"is itself trivial ({fn['file']}), not valid evidence")
        problems.extend(entry_problems)
        if not entry_problems:
            valid_entries.append(e)
            for pf in matches:
                valid.add((py_file, pf.class_name, pf.name))
                if not e.get("py_class"):
                    valid.add((py_file, None, pf.name))
    return problems, valid_entries, valid


def find_go_evidence(go_index: dict, target: dict) -> Optional[dict]:
    """Resolve an evidence pointer to a substantive Go function record."""
    want_file = (target.get("go_file") or "").split("/")[-1]
    for fn in go_index["by_lower"].get(target.get("go_func", "").lower(), []):
        if not want_file or fn["file"] == want_file:
            return fn
    return None


# ---------------------------------------------------------------------------
# The audit itself
# ---------------------------------------------------------------------------

def audit_file(py_name: str, go_funcs: dict = None,
               go_index: Optional[dict] = None,
               exclusions: Optional[list[dict]] = None) -> dict:
    """Audit a single Python file against its Go counterpart."""
    py_path = os.path.join(PY_DIR, py_name)
    go_name = py_name.replace(".py", ".go")
    go_path = os.path.join(GO_DIR, go_name)

    if not os.path.exists(py_path):
        return None

    if go_index is None:
        go_index = gofuncinfo()
    if exclusions is None:
        exclusions = []

    py_funcs = extract_python_functions(py_path)

    same_file = [fn for fn in go_index["functions"] if fn["file"] == go_name]
    same_file_by_lower = {}
    for fn in same_file:
        same_file_by_lower.setdefault(fn["name"].lower(), []).append(fn)

    entries = []
    for pf in py_funcs:
        excl = next((e for e in exclusions
                     if e.get("py_file") == py_name
                     and e.get("py_name") == pf.name
                     and e.get("py_class") in (None, "", pf.class_name)), None)

        go_fn = None
        same_file_match = True
        for cand in expected_go_names(pf):
            key = cand.lower()
            if key in same_file_by_lower:
                go_fn = same_file_by_lower[key][0]
                break
        if go_fn is None:
            same_file_match = False
            for cand in expected_go_names(pf):
                key = cand.lower()
                if key in go_index["by_lower"]:
                    go_fn = go_index["by_lower"][key][0]
                    break

        tsub = None
        status = classify(pf, go_fn, excl)
        if status == "thin":
            tsub = transitive_substance(go_index, go_fn)
            status = classify(pf, go_fn, excl, tsub)
        entry = {
            "py_name": pf.name,
            "py_class": pf.class_name,
            "py_line": pf.line_start,
            "py_nstmt": pf.nstmt,
            "py_substance": pf.substance,
            "py_trivial": pf.py_trivial,
            "go_name": go_fn["name"] if go_fn else None,
            "go_file": go_fn["file"] if go_fn else None,
            "go_line": go_fn["line"] if go_fn else None,
            "go_nstmt": go_fn["nstmt"] if go_fn else 0,
            "go_trivial": go_trivial(go_fn) if go_fn else None,
            "go_notimpl": bool(go_fn and go_fn.get("notimpl")),
            "go_dead": bool(go_fn and go_fn.get("called_by", 0) == 0),
            "cross_file": bool(go_fn and not same_file_match),
            "status": status,
            "excluded_entry": excl,
        }
        if status == "implemented_delegation" and go_fn is not None:
            entry["delegates"] = internal_delegates(go_index, go_fn)
        entries.append(entry)

    counts = {}
    for e in entries:
        counts[e["status"]] = counts.get(e["status"], 0) + 1

    return {
        "python_file": py_name,
        "go_file": go_name,
        "go_exists": os.path.exists(go_path),
        "python_function_count": len(py_funcs),
        "counts": counts,
        "entries": entries,
    }


def audit_all(exclusions_path: str = None, gofuncinfo_path: str = None) -> list[dict]:
    exclusions = load_exclusions(exclusions_path or DEFAULT_EXCLUSIONS)
    go_index = gofuncinfo(gofuncinfo_path)

    pyfuncs_by_file = {}
    for py_name in FILES_TO_AUDIT:
        py_path = os.path.join(PY_DIR, py_name)
        if os.path.exists(py_path):
            pyfuncs_by_file[py_name] = extract_python_functions(py_path)

    problems, valid_exclusions, _valid_keys = validate_exclusions(
        exclusions, pyfuncs_by_file, go_index)

    results = []
    for py_name in FILES_TO_AUDIT:
        result = audit_file(py_name, go_index=go_index, exclusions=valid_exclusions)
        if result:
            results.append(result)
    return results, exclusions, problems


STATUS_ORDER = ["stub_silent", "stub_admitted", "thin", "missing", "excluded",
                "implemented_trivial", "implemented_delegation", "implemented"]
STATUS_ICONS = {
    "implemented": "✓", "implemented_trivial": "○", "implemented_delegation": "→",
    "stub_silent": "✗", "stub_admitted": "✗", "thin": "≈",
    "missing": "∅", "excluded": "⊘",
}
GAP_STATUSES = {"stub_silent", "stub_admitted", "thin", "missing"}


def print_report(result, verbose=False):
    if result is None:
        return
    py = result["python_file"]
    go = result["go_file"]
    counts = result["counts"]
    total = result["python_function_count"]
    gaps = sum(counts.get(s, 0) for s in GAP_STATUSES)
    icon = "✓" if gaps == 0 else "✗"
    detail = " ".join(f"{STATUS_ICONS.get(s, '?')}{counts.get(s, 0)}"
                      for s in STATUS_ORDER if counts.get(s, 0))
    print(f"{icon} {py} → {go}  ({detail})  of {total}")

    by_status = {}
    for e in result["entries"]:
        by_status.setdefault(e["status"], []).append(e)

    for status in ["stub_silent", "stub_admitted", "thin", "missing", "excluded",
                   "implemented_trivial"]:
        for e in by_status.get(status, []):
            if status in ("implemented_trivial",) and not verbose:
                continue
            cls = f"{e['py_class']}." if e["py_class"] else ""
            loc = f"py:L{e['py_line']} nstmt={e['py_nstmt']}"
            if e["go_name"]:
                loc += f" → go:{e['go_file']}:L{e['go_line']} nstmt={e['go_nstmt']}"
                if e["go_dead"]:
                    loc += " [dead: no call sites]"
                if e["cross_file"]:
                    loc += " [cross-file match]"
            else:
                loc += " → (no Go match)"
            mark = STATUS_ICONS.get(status, "?")
            print(f"  {mark} {cls}{e['py_name']}  {loc}")
            if status == "excluded" and e["excluded_entry"]:
                ee = e["excluded_entry"]
                print(f"      ⊘ excluded [{ee.get('category')}]: {ee.get('reason')}")


def print_metric(results, exclusions):
    counts = {}
    for r in results:
        for k, v in r["counts"].items():
            counts[k] = counts.get(k, 0) + v
    total = sum(r["python_function_count"] for r in results)
    implemented = (counts.get("implemented", 0) + counts.get("implemented_trivial", 0)
                   + counts.get("implemented_delegation", 0))
    gaps = sum(counts.get(s, 0) for s in GAP_STATUSES)
    excluded = counts.get("excluded", 0)
    audited = total - excluded
    pct = (implemented / audited * 100) if audited > 0 else 100.0
    strict = (implemented / total * 100) if total > 0 else 100.0

    print(f"METRIC py_functions={total}")
    for status in STATUS_ORDER:
        print(f"METRIC {status}={counts.get(status, 0)}")
    print(f"METRIC excluded={excluded}")
    print(f"METRIC parity_pct={pct:.1f}")
    print(f"METRIC strict_parity_pct={strict:.1f}")
    print(f"METRIC gap_functions={gaps}")
    return counts


def write_report_file(results, exclusions, problems, path):
    lines = []
    lines.append("# Honest Parity Report — Python→Go function audit")
    lines.append("")
    lines.append("GENERATED by `python3 scripts/audit_parity.py --report ...` — do not edit by hand.")
    lines.append("Regenerate after any Go or Python reference change.")
    lines.append("")

    counts = {}
    for r in results:
        for k, v in r["counts"].items():
            counts[k] = counts.get(k, 0) + v
    total = sum(r["python_function_count"] for r in results)
    implemented = (counts.get("implemented", 0) + counts.get("implemented_trivial", 0)
                   + counts.get("implemented_delegation", 0))
    excluded = counts.get("excluded", 0)
    audited = total - excluded

    lines.append("## Summary")
    lines.append("")
    lines.append(f"- Python functions audited: **{total}**")
    lines.append(f"- Implemented (substantive): **{counts.get('implemented', 0)}**")
    lines.append(f"- Implemented (delegation wrappers): **{counts.get('implemented_delegation', 0)}**")
    lines.append(f"- Implemented (trivial↔trivial): **{counts.get('implemented_trivial', 0)}**")
    lines.append(f"- Stubs (silent name-only shims): **{counts.get('stub_silent', 0)}**")
    lines.append(f"- Stubs (admitted not-implemented): **{counts.get('stub_admitted', 0)}**")
    lines.append(f"- Thin (suspiciously small): **{counts.get('thin', 0)}**")
    lines.append(f"- Missing (no name match): **{counts.get('missing', 0)}**")
    lines.append(f"- Excluded (explicit, see below): **{excluded}**")
    lines.append(f"- **Parity: {implemented}/{audited} = {(implemented / audited * 100) if audited else 100:.1f}%** "
                 f"(strict, counting exclusions against parity: "
                 f"{(implemented / total * 100) if total else 100:.1f}%)")
    lines.append("")

    lines.append("## Real implementation gaps")
    lines.append("")
    lines.append("Functions counted as gaps (stub/thin/missing). These have no")
    lines.append("substantive Go implementation and no approved exclusion.")
    lines.append("")
    for r in results:
        gaps = [e for e in r["entries"] if e["status"] in GAP_STATUSES]
        if not gaps:
            continue
        lines.append(f"### {r['python_file']}")
        lines.append("")
        lines.append("| Python function | py nstmt | Go match | go nstmt | status |")
        lines.append("|---|---|---|---|---|")
        for e in sorted(gaps, key=lambda x: (x["status"], x["py_line"])):
            cls = f"{e['py_class']}." if e["py_class"] else ""
            go = f"`{e['go_name']}` ({e['go_file']}:L{e['go_line']})" if e["go_name"] else "—"
            extra = []
            if e["go_dead"]:
                extra.append("dead")
            if e["cross_file"]:
                extra.append("cross-file")
            status = e["status"] + (f" [{','.join(extra)}]" if extra else "")
            lines.append(f"| `{cls}{e['py_name']}` (L{e['py_line']}) | {e['py_nstmt']} | "
                         f"{go} | {e['go_nstmt']} | {status} |")
        lines.append("")

    lines.append("## Exclusions (explicit, reviewable)")
    lines.append("")
    lines.append("Waived functions with reasons. Never counted as implemented.")
    lines.append("")
    if problems:
        lines.append("### ⚠ Exclusion validation problems")
        lines.append("")
        for p in problems:
            lines.append(f"- {p}")
        lines.append("")
    for e in exclusions:
        cls = f"{e.get('py_class')}." if e.get("py_class") else ""
        lines.append(f"- **{e['py_file']} :: {cls}{e['py_name']}** — `{e.get('category')}`")
        lines.append(f"  - reason: {e.get('reason')}")
        for ev in e.get("evidence") or []:
            lines.append(f"  - evidence: `{ev.get('go_func')}` in `{ev.get('go_file')}`")
    lines.append("")

    with open(path, "w") as f:
        f.write("\n".join(lines) + "\n")


def init_exclusions(results, path):
    """Write a skeleton exclusions manifest for all current gaps, preserving
    any existing entries."""
    existing = load_exclusions(path) if os.path.exists(path) else []
    have = {(e.get("py_file"), e.get("py_class"), e.get("py_name")) for e in existing}
    skeleton = []
    for r in results:
        for e in r["entries"]:
            if e["status"] in GAP_STATUSES:
                k = (r["python_file"], e["py_class"], e["py_name"])
                if k in have:
                    continue
                skeleton.append({
                    "py_file": r["python_file"],
                    "py_class": e["py_class"],
                    "py_name": e["py_name"],
                    "category": "TODO-FILL-CATEGORY",
                    "reason": "TODO: justify (>=10 chars) or implement",
                    "evidence": [],
                })
    doc = {"exclusions": existing + skeleton}
    with open(path, "w") as f:
        json.dump(doc, f, indent=2)
        f.write("\n")
    print(f"wrote {len(skeleton)} new skeleton entries to {path} "
          f"({len(existing)} existing preserved); fill category/reason/evidence")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--file", "-f", help="Specific file to audit (without extension)")
    parser.add_argument("--verbose", "-v", action="store_true")
    parser.add_argument("--json", action="store_true")
    parser.add_argument("--metric", action="store_true", help="Only output METRIC lines")
    parser.add_argument("--report", metavar="PATH", help="Write markdown report to PATH")
    parser.add_argument("--init-exclusions", action="store_true",
                        help="Write skeleton exclusions manifest for current gaps")
    parser.add_argument("--exclusions", metavar="PATH", default=DEFAULT_EXCLUSIONS)
    parser.add_argument("--gofuncinfo", metavar="PATH", default=None,
                        help="Use pre-generated gofuncinfo JSON instead of running Go")
    args = parser.parse_args()

    os.chdir(BASE)

    if args.file:
        name = args.file.replace(".py", "").replace(".go", "")
        py_name = name + ".py"
        exclusions = load_exclusions(args.exclusions)
        go_index = gofuncinfo(args.gofuncinfo)
        result = audit_file(py_name, go_index=go_index, exclusions=exclusions)
        if result:
            if args.json:
                print(json.dumps(result, indent=2, default=str))
            else:
                print_report(result, verbose=True)
        return

    results, exclusions, problems = audit_all(args.exclusions, args.gofuncinfo)

    if args.init_exclusions:
        if args.exclusions == DEFAULT_EXCLUSIONS:
            parser.error("--init-exclusions writes TODO skeletons; pass an explicit "
                         "--exclusions PATH (not the real manifest)")
        init_exclusions(results, args.exclusions)
        return

    if args.metric:
        print_metric(results, exclusions)
        return

    for r in results:
        print_report(r, verbose=args.verbose)
        print()

    print_metric(results, exclusions)
    if problems:
        print("\n⚠ EXCLUSION VALIDATION PROBLEMS:")
        for p in problems:
            print(f"  - {p}")

    if args.report:
        write_report_file(results, exclusions, problems, args.report)
        print(f"\nreport written to {args.report}")

    counts = {}
    for r in results:
        for k, v in r["counts"].items():
            counts[k] = counts.get(k, 0) + v
    gaps = sum(counts.get(s, 0) for s in GAP_STATUSES)
    sys.exit(1 if (gaps > 0 or problems) else 0)


if __name__ == "__main__":
    main()
