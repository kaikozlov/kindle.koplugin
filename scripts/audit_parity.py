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
    trivial_shape: str = ""  # semantic no-op/identity/constant classification

    @property
    def is_dunder(self):
        return self.name.startswith("__") and self.name.endswith("__")

    @property
    def is_private(self):
        return self.name.startswith("_") and not self.is_dunder

    @property
    def py_trivial(self):
        """Semantically trivial: no-op, identity, or constant (shape-based,
        NOT size-based). `return self.lookup[x]` is substantive."""
        return bool(self.trivial_shape)

    @property
    def substance(self):
        return self.nstmt + self.nlit

    @property
    def key(self):
        return (self.class_name or "", self.name)

    @property
    def identity(self):
        """Def-precise identity: (def line, class, name).

        Python files legitimately contain multiple defs sharing a name
        (e.g. @property getter + .setter pairs). An exclusion that does
        not pin py_line would silently waive every def with that name."""
        return (self.line_start, self.class_name, self.name)


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


def _py_expr_shape(e: ast.expr, arg_pos: dict) -> str:
    """Semantically classify a Python return expression for trivial matching.

    Mirrors gofuncinfo's TrivialShape vocabulary. Constants carry their
    literal VALUE so compatibility can require value equality; identity
    returns carry the argument POSITION. Call/index/attribute returns are
    never trivial ("call:<name>" marks them for diagnostics).
    """
    if isinstance(e, ast.Constant):
        v = e.value
        if v is None:
            return "const:nil"
        if v is True:
            return "const:true"
        if v is False:
            return "const:false"
        if isinstance(v, (int, float)) and not isinstance(v, bool):
            return f"const:{'int' if isinstance(v, int) else 'float'}:{v}"
        if isinstance(v, str):
            return "const:empty-string" if v == "" else f"const:string:{v}"
        return ""
    if isinstance(e, ast.Name):
        if e.id in arg_pos:
            return f"arg:{arg_pos[e.id]}"
        return ""
    if isinstance(e, (ast.List, ast.Tuple, ast.Dict, ast.Set)):
        if len(getattr(e, "elts", []) or getattr(e, "keys", [])) == 0:
            return "const:empty-lit"
        return ""
    if isinstance(e, ast.Call):
        name = e.func.id if isinstance(e.func, ast.Name) else (
            e.func.attr if isinstance(e.func, ast.Attribute) else "?")
        return f"call:{name}"
    return ""


def py_trivial_shape(node: ast.FunctionDef, arg_pos: dict) -> str:
    """Classify a Python function body as no-op/identity/constant.

    Only true trivial SHAPES qualify: docstring + pass/... (void), a single
    constant return, or a single identity return of a parameter. Anything
    computed, called, indexed, or looked up is substantive — one line does
    not make it trivial. Returns "" for substantive bodies.
    """
    body = list(node.body)
    if body and (isinstance(body[0], ast.Expr)
                 and isinstance(body[0].value, ast.Constant)
                 and isinstance(body[0].value.value, str)):
        body = body[1:]
    if not body:
        return "void"
    shapes = []
    for stmt in body:
        if isinstance(stmt, ast.Pass):
            shapes.append("void")
            continue
        if isinstance(stmt, ast.Return):
            if stmt.value is None:
                shapes.append("void")
            else:
                s = _py_expr_shape(stmt.value, arg_pos)
                if not s or s.startswith("call:"):
                    return ""  # computed/call value: substantive
                shapes.append(s)
            continue
        return ""  # any other statement: substantive
    if not shapes:
        return "void"
    if any(s != shapes[0] for s in shapes[1:]):
        return ""
    return shapes[0]


def _norm_num(shape: str) -> str:
    """Normalize a const:<int|float>:<value> shape for cross-language
    comparison (Python 1 == Go 1; 1.0 == 1)."""
    body = shape[len("const:"):] if shape.startswith("const:") else shape
    kind, sep, value = body.partition(":")
    if kind not in ("int", "float") or not sep:
        return shape
    try:
        f = float(value)
        if f == int(f):
            return f"const:num:{int(f)}"
        return f"const:num:{f}"
    except ValueError:
        return shape


def _norm_str(shape: str) -> str:
    """Strip Go/Python string quoting so literals compare by content."""
    if shape.startswith("const:string:"):
        lit = shape[len("const:string:"):]
        if len(lit) >= 2 and lit[0] in "\"'" and lit[-1] == lit[0]:
            lit = lit[1:-1]
        return f"const:string:{lit}"
    return shape


def trivial_shapes_compatible(py_shape: str, go_shape: Optional[str]) -> bool:
    """Can a semantically-trivial Python function be satisfied by a
    semantically-trivial Go body? Requires shape AND value equality:
    True != false, 0 != 1, identity must hit the same argument position.
    """
    if not py_shape or not go_shape:
        return False
    p, g = py_shape, go_shape
    if p.startswith("const:empty-string"):
        p = "const:string:"
    if g.startswith("const:empty-string"):
        g = "const:string:"
    if p.startswith("const:empty-lit") or g.startswith("const:empty-lit"):
        return p.startswith("const:empty-lit") and g.startswith("const:empty-lit")
    if p.startswith("const:int:") or p.startswith("const:float:"):
        if not (g.startswith("const:int:") or g.startswith("const:float:")):
            return False
        return _norm_num(p) == _norm_num(g)
    if p.startswith("const:string:"):
        return g.startswith("const:string:") and _norm_str(p) == _norm_str(g)
    if p.startswith("arg:") or g.startswith("arg:"):
        return p == g  # both must be identity of the same argument position
    return p == g  # void/nil/true/false/… exact


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
            arg_pos = {}
            pos = 0
            for a in node.args.args:
                if a.arg != "self":
                    arg_pos[a.arg] = pos
                    pos += 1
            result.append(PyFunc(
                name=node.name,
                class_name=class_name,
                line_start=node.lineno,
                line_end=node.end_lineno or node.lineno,
                args=", ".join(arg_names),
                docstring_first_line=doc,
                nstmt=max(0, nstmt - doc_stmts),
                nlit=nlit,
                trivial_shape=py_trivial_shape(node, arg_pos),
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

# Go identifier names that are too generic to auto-match by name even when
# unique in the expected file: a `String`/`Equal`/`Get` that happens to exist
# proves nothing about a Python __repr__/__eq__/__getitem__. These require an
# explicit identity override.
GENERIC_GO_NAMES = {
    "String", "GoString", "Equal", "Less", "Len", "Get", "At", "Hash",
    "Contains", "Copy", "DeepCopy", "SetItem", "Ne", "Le", "Gt", "Ge", "New",
    "Clear", "Keys", "Items", "Format", "Init", "Head", "Body", "Walk",
}

DEFAULT_OVERRIDES = os.path.join(BASE, "scripts/parity_identity_overrides.json")


# ---------------------------------------------------------------------------
# Go function substance index (from scripts/gofuncinfo)
# ---------------------------------------------------------------------------

@lru_cache(maxsize=1)
def gofuncinfo(path=None) -> dict:
    """Run (or load) gofuncinfo and index Go functions.

    Indexing is EXACT-CASE (Go identifiers are case-sensitive: decodeKFX and
    DecodeKFX are different functions). Lowercased conflation of distinct
    identifiers is how evidence used to match the wrong function.
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
    by_name = {}
    for f in funcs:
        by_name.setdefault(f["name"], []).append(f)
    data["by_name"] = by_name
    return data


def go_index_lookup(go_index: dict, name: str, go_file: Optional[str] = None,
                    allow_multiple: bool = False) -> "list[dict]":
    """Exact-case lookup, optionally scoped to one file.

    Returns the candidate list; callers decide how ambiguity is handled.
    """
    if go_file:
        want = go_file.split("/")[-1]
        cands = [f for f in go_index["by_name"].get(name, []) if f["file"] == want]
        if cands:
            return cands
        return []
    return go_index["by_name"].get(name, [])


def go_trivial(go_fn: dict) -> bool:
    return bool(go_fn.get("empty") or go_fn.get("const_only") or go_fn.get("error_only"))


def go_substance(go_fn: dict) -> int:
    return go_fn.get("nstmt", 0) + go_fn.get("nlit", 0)


def _resolve_callee(go_index: dict, callee: str, from_fn: dict) -> Optional[dict]:
    """Resolve an unqualified Ident callee EXACT-CASE for delegation credit.

    Only same-file-unique or corpus-unique resolutions count; an ambiguous
    name (multiple candidates) yields no credit — we prefer false negatives
    over grafting an unrelated function's substance onto a wrapper.
    """
    cands = go_index_lookup(go_index, callee, from_fn["file"])
    if len(cands) == 1:
        return cands[0]
    if not cands:
        cands = go_index_lookup(go_index, callee)
    if len(cands) == 1:
        return cands[0]
    return None


def transitive_substance(go_index: dict, go_fn: dict) -> int:
    """Total substance reachable through unqualified Ident calls only, each
    function counted once (cycle-safe). Selector calls are excluded by
    gofuncinfo (IdentCalls) — they cannot prove the target. Credits one-line
    delegation wrappers only when they provably delegate within the corpus."""
    cache = go_index.setdefault("_tsub_cache", {})

    def visit(fn: dict, visiting: set) -> int:
        key = (fn["file"], fn["name"], fn["line"])
        if key in cache:
            return cache[key]
        if key in visiting:
            return 0  # cycle
        visiting.add(key)
        total = go_substance(fn)
        for callee in fn.get("ident_calls", []):
            target = _resolve_callee(go_index, callee, fn)
            if target is not None:
                total += visit(target, visiting)
        visiting.discard(key)
        cache[key] = total
        return total

    return visit(go_fn, set())


def internal_delegates(go_index: dict, go_fn: dict) -> list[str]:
    """Unqualified callees that resolve unambiguously (for the report)."""
    out = []
    for callee in go_fn.get("ident_calls", []):
        if _resolve_callee(go_index, callee, go_fn) is not None:
            out.append(callee)
    return out


def classify(py: PyFunc, go_fn: Optional[dict], excluded_entry: Optional[dict],
             override_entry: Optional[dict] = None,
             tsub: Optional[int] = None) -> str:
    """Classify one Python function against its matched Go function.

    trivial↔trivial credit requires SEMANTIC compatibility (shape + literal
    value equality + argument position), never mere size or kind.
    """
    if excluded_entry is not None:
        return "excluded"
    if override_entry is not None:
        return "mapped"
    if go_fn is None:
        return "missing"
    if go_fn.get("notimpl"):
        return "stub_admitted"
    if go_trivial(go_fn):
        if py.py_trivial and trivial_shapes_compatible(py.trivial_shape, go_fn.get("trivial_shape")):
            return "implemented_trivial"
        return "stub_silent"
    if py.substance >= PY_BIG:
        need = py.substance * THIN_RATIO
        if go_substance(go_fn) < need:
            if tsub is not None and tsub >= need:
                return "implemented_delegation"
            return "thin"
    return "implemented"


def load_overrides(path: str) -> list[dict]:
    """Identity overrides: explicit, reviewed cross-file/ambiguous mappings.

    An override says "Python (file,class,name[,line]) is implemented by Go
    (go_file,go_func)" — the ONLY way a cross-file or name-ambiguous match
    can count as implemented. Without one, such matches are unresolved_match.
    """
    if not path or not os.path.exists(path):
        return []
    with open(path) as f:
        data = json.load(f)
    return data.get("overrides", [])


def override_matches(entry: dict, pf: PyFunc, py_file: str) -> bool:
    if entry.get("py_file") != py_file:
        return False
    if entry.get("py_name") != pf.name:
        return False
    if entry.get("py_class") not in (None, "", pf.class_name):
        return False
    if entry.get("py_line") not in (None, 0) and entry["py_line"] != pf.line_start:
        return False
    return True


def validate_overrides(entries: list[dict], pyfuncs_by_file: dict,
                       go_index: dict) -> tuple[list[str], list[dict]]:
    """Validate identity overrides. Mapping targets must resolve EXACT-CASE
    to a substantive Go function; reasons must be reviewable; targets that
    are themselves trivial are rejected. Invalid entries are reported and
    never applied."""
    problems = []
    valid = []
    for i, e in enumerate(entries):
        where = f"overrides[{i}]"
        errs = []
        mapping = e.get("mapping") or {}
        if not mapping.get("go_file") or not mapping.get("go_func"):
            errs.append(f"{where}: mapping requires go_file and go_func")
        reason = (e.get("reason") or "").strip()
        if len(reason) < 10:
            errs.append(f"{where}: reason too short to be reviewable "
                        f"({len(reason)} chars, need >= 10)")
        py_file = e.get("py_file")
        if py_file not in pyfuncs_by_file:
            errs.append(f"{where}: py_file {py_file!r} is not an audited file")
        else:
            matches = [pf for pf in pyfuncs_by_file[py_file]
                       if pf.name == e.get("py_name")
                       and e.get("py_class") in (None, "", pf.class_name)]
            if not matches:
                errs.append(f"{where}: no audited Python function matches "
                            f"{py_file}:{e.get('py_class')}.{e.get('py_name')}")
            elif len(matches) > 1 and not e.get("py_line"):
                lines = sorted(pf.line_start for pf in matches)
                errs.append(f"{where}: {len(matches)} defs share this name "
                            f"(lines {lines}); py_line is required")
        if mapping.get("go_func"):
            fn = find_go_evidence(go_index, mapping)
            if fn is None:
                errs.append(f"{where}.mapping: no EXACT-CASE Go function "
                            f"{mapping.get('go_func')!r} in {mapping.get('go_file')!r}")
            elif go_trivial(fn) or fn["nstmt"] < 3:
                errs.append(f"{where}.mapping: {mapping.get('go_func')!r} is itself "
                            f"trivial ({fn['file']}), not a valid mapping target")
        problems.extend(errs)
        if not errs:
            valid.append(e)
    return problems, valid


# ---------------------------------------------------------------------------
# Exclusions manifest
# ---------------------------------------------------------------------------

def exclusion_matches(entry: dict, pf: PyFunc, py_file: str) -> bool:
    """Does this exclusion entry apply to this specific Python def?

    py_line (when set) pins the entry to exactly one def among
    same-name duplicates; without it the entry applies to every def
    sharing (file, class, name) — allowed only when the name is unique.
    """
    if entry.get("py_file") != py_file:
        return False
    if entry.get("py_name") != pf.name:
        return False
    if entry.get("py_class") not in (None, "", pf.class_name):
        return False
    if entry.get("py_line") not in (None, 0) and entry["py_line"] != pf.line_start:
        return False
    return True


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

    Returns (problems, valid_entries). Only valid entries may be applied —
    applied — an invalid entry (unknown category, lazy reason, unresolvable
    or trivial evidence, stale target) is IGNORED for classification and
    reported as a problem, so a manifest of junk exclusions can never green
    the metric.
    """
    problems = []
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
        # Duplicate-def disambiguation: a name may be defined more than once
        # (getter/setter pairs, overloads). An exclusion that does not pin
        # py_line would waive ALL of them — require an explicit line, and
        # verify it points at a real def (not a decorator or off-by-one).
        if len(matches) > 1 and not e.get("py_line"):
            lines = sorted(pf.line_start for pf in matches)
            entry_problems.append(
                f"{where}: {py_file}:{e.get('py_class')}.{e.get('py_name')} has "
                f"{len(matches)} defs (lines {lines}); py_line is required so "
                f"the exclusion cannot silently waive multiple defs")
        if e.get("py_line"):
            if not any(pf.line_start == e["py_line"] for pf in matches):
                lines = sorted(pf.line_start for pf in matches)
                entry_problems.append(
                    f"{where}: py_line {e['py_line']} does not match any def of "
                    f"{py_file}:{e.get('py_class')}.{e.get('py_name')} (defs at {lines})")
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
    return problems, valid_entries


def find_go_evidence(go_index: dict, target: dict) -> Optional[dict]:
    """Resolve an evidence pointer EXACT-CASE to a Go function record.

    Evidence must name the function as it is spelled in Go; decodeKFX does
    not satisfy evidence for DecodeKFX and vice versa.
    """
    want_file = (target.get("go_file") or "").split("/")[-1]
    cands = go_index_lookup(go_index, target.get("go_func", ""), want_file or None)
    if len(cands) == 1:
        return cands[0]
    if len(cands) > 1 and want_file:
        # Multiple same-named funcs in the file: disambiguate by line if given
        if target.get("go_line"):
            for c in cands:
                if c["line"] == target["go_line"]:
                    return c
        return None
    return cands[0] if len(cands) == 1 else None


# ---------------------------------------------------------------------------
# The audit itself
# ---------------------------------------------------------------------------

def audit_file(py_name: str, go_funcs: dict = None,
               go_index: Optional[dict] = None,
               exclusions: Optional[list[dict]] = None,
               overrides: Optional[list[dict]] = None) -> dict:
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
    if overrides is None:
        overrides = []

    py_funcs = extract_python_functions(py_path)

    same_file_by_name = {}
    for fn in go_index["functions"]:
        if fn["file"] == go_name:
            same_file_by_name.setdefault(fn["name"], []).append(fn)

    entries = []
    for pf in py_funcs:
        excl = next((e for e in exclusions if exclusion_matches(e, pf, py_name)), None)
        over = next((o for o in overrides if override_matches(o, pf, py_name)), None)

        # --- Matching (conservative by design) ---
        # 1. Automatic credit ONLY for an EXACT-CASE, UNIQUE, same-file match.
        # 2. Generic Go names (String/Equal/Get/…) never auto-match: one
        #    existing `String` proves nothing about a Python __repr__.
        # 3. Anything else (ambiguous, generic, cross-file) is
        #    unresolved_match — a GAP unless an explicit override maps it.
        go_fn = None
        match_note = None
        for cand in expected_go_names(pf):
            if cand in GENERIC_GO_NAMES:
                continue
            cands = same_file_by_name.get(cand, [])
            if len(cands) == 1:
                go_fn = cands[0]
                break
            if len(cands) > 1:
                match_note = f"ambiguous same-file candidates for {cand!r} " \
                             f"({[c['line'] for c in cands]})"
        if go_fn is None and over is not None:
            mapping = over.get("mapping") or {}
            cands = go_index_lookup(go_index, mapping.get("go_func", ""),
                                    mapping.get("go_file"))
            if len(cands) == 1:
                go_fn = cands[0]
        unresolved_reason = None
        if go_fn is None:
            # Cross-file exact-case unique matches are NOT auto-counted; note
            # them for the report so porting work is discoverable.
            cross = []
            ambiguous = False
            for cand in expected_go_names(pf):
                if cand in GENERIC_GO_NAMES:
                    continue
                cands = go_index_lookup(go_index, cand)
                if len(cands) == 1:
                    cross.append(cands[0])
                elif len(cands) > 1:
                    ambiguous = True
            if cross:
                unresolved_reason = (f"cross-file name-only match: "
                                     f"{cross[0]['name']} in {cross[0]['file']}:L{cross[0]['line']} "
                                     f"(requires explicit identity override)")
            elif ambiguous:
                unresolved_reason = ("multiple same-named Go functions across files; "
                                     "requires explicit identity override")
            elif match_note:
                unresolved_reason = match_note + " (requires explicit identity override)"
            elif any(cand in GENERIC_GO_NAMES for cand in expected_go_names(pf)):
                unresolved_reason = "only generic Go name candidates (String/Equal/Get/…); " \
                                    "requires explicit identity override"

        tsub = None
        status = classify(pf, go_fn, excl, over)
        if status == "thin":
            tsub = transitive_substance(go_index, go_fn)
            status = classify(pf, go_fn, excl, over, tsub)
        if go_fn is None and excl is None:
            status = "unresolved_match" if unresolved_reason else "missing"
        entry = {
            "py_name": pf.name,
            "py_class": pf.class_name,
            "py_line": pf.line_start,
            "py_nstmt": pf.nstmt,
            "py_substance": pf.substance,
            "py_trivial": pf.py_trivial,
            "py_trivial_shape": pf.trivial_shape,
            "go_name": go_fn["name"] if go_fn else None,
            "go_file": go_fn["file"] if go_fn else None,
            "go_line": go_fn["line"] if go_fn else None,
            "go_nstmt": go_fn["nstmt"] if go_fn else 0,
            "go_trivial": go_trivial(go_fn) if go_fn else None,
            "go_notimpl": bool(go_fn and go_fn.get("notimpl")),
            "go_dead": bool(go_fn and go_fn.get("called_by", 0) == 0),
            "cross_file": bool(go_fn and go_fn["file"] != go_name),
            "status": status,
            "unresolved_reason": unresolved_reason,
            "excluded_entry": excl,
            "override_entry": over,
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


def audit_all(exclusions_path: str = None, gofuncinfo_path: str = None,
              overrides_path: str = None) -> list[dict]:
    exclusions = load_exclusions(exclusions_path or DEFAULT_EXCLUSIONS)
    overrides = load_overrides(overrides_path or DEFAULT_OVERRIDES)
    go_index = gofuncinfo(gofuncinfo_path)

    pyfuncs_by_file = {}
    for py_name in FILES_TO_AUDIT:
        py_path = os.path.join(PY_DIR, py_name)
        if os.path.exists(py_path):
            pyfuncs_by_file[py_name] = extract_python_functions(py_path)

    problems, valid_exclusions = validate_exclusions(
        exclusions, pyfuncs_by_file, go_index)
    o_problems, valid_overrides = validate_overrides(
        overrides, pyfuncs_by_file, go_index)
    problems = problems + o_problems

    results = []
    for py_name in FILES_TO_AUDIT:
        result = audit_file(py_name, go_index=go_index,
                            exclusions=valid_exclusions,
                            overrides=valid_overrides)
        if result:
            results.append(result)
    return results, exclusions, problems


STATUS_ORDER = ["stub_silent", "stub_admitted", "thin", "missing", "unresolved_match",
                "excluded", "mapped", "implemented_trivial", "implemented_delegation",
                "implemented"]
STATUS_ICONS = {
    "implemented": "✓", "implemented_trivial": "○", "implemented_delegation": "→",
    "mapped": "⇢", "stub_silent": "✗", "stub_admitted": "✗", "thin": "≈",
    "missing": "∅", "unresolved_match": "?", "excluded": "⊘",
}
GAP_STATUSES = {"stub_silent", "stub_admitted", "thin", "missing", "unresolved_match"}


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

    for status in ["stub_silent", "stub_admitted", "thin", "missing", "unresolved_match",
                   "excluded", "mapped", "implemented_trivial"]:
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
            if e.get("unresolved_reason"):
                print(f"      ? {e['unresolved_reason']}")
            if status == "mapped" and e.get("override_entry"):
                oe = e["override_entry"]
                m = oe.get("mapping") or {}
                print(f"      ⇢ mapped to {m.get('go_file')}::{m.get('go_func')}: {oe.get('reason')}")
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
    mapped = counts.get("mapped", 0)
    audited = total - excluded - mapped
    pct = (implemented / audited * 100) if audited > 0 else 100.0
    strict = (implemented / total * 100) if total > 0 else 100.0

    print(f"METRIC py_functions={total}")
    for status in STATUS_ORDER:
        if status == "excluded":
            print(f"METRIC excluded={excluded}")  # exactly once
            continue
        print(f"METRIC {status}={counts.get(status, 0)}")
    print(f"METRIC structural_coverage_pct={pct:.1f}")
    print(f"METRIC strict_structural_coverage_pct={strict:.1f}")
    # Backward-compatible alias — this metric is STRUCTURAL COVERAGE (name +
    # body substance), NOT behavioral parity. Do not present it as proof of
    # semantic equivalence; behavioral evidence lives in branch/golden/
    # differential tests.
    print(f"METRIC parity_pct={pct:.1f}")
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
        at = f"@L{e['py_line']}" if e.get("py_line") else ""
        lines.append(f"- **{e['py_file']} :: {cls}{e['py_name']}{at}** — `{e.get('category')}`")
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
                # Duplicated names must be pinned per-def from the start.
                dup = sum(1 for other in r["entries"]
                          if other["py_name"] == e["py_name"]
                          and (other["py_class"] or None) == (e["py_class"] or None))
                entry = {
                    "py_file": r["python_file"],
                    "py_class": e["py_class"],
                    "py_name": e["py_name"],
                    "category": "TODO-FILL-CATEGORY",
                    "reason": "TODO: justify (>=10 chars) or implement",
                    "evidence": [],
                }
                if dup > 1:
                    entry["py_line"] = e["py_line"]
                skeleton.append(entry)
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
    parser.add_argument("--overrides", metavar="PATH", default=DEFAULT_OVERRIDES,
                        help="Identity overrides manifest (explicit cross-file/ambiguous mappings)")
    parser.add_argument("--gofuncinfo", metavar="PATH", default=None,
                        help="Use pre-generated gofuncinfo JSON instead of running Go")
    args = parser.parse_args()

    os.chdir(BASE)

    if args.file:
        name = args.file.replace(".py", "").replace(".go", "")
        py_name = name + ".py"
        py_path = os.path.join(PY_DIR, py_name)
        if not os.path.exists(py_path):
            print(f"ERROR: {py_path} not found", file=sys.stderr)
            sys.exit(1)
        exclusions = [e for e in load_exclusions(args.exclusions)
                      if e.get("py_file") == py_name]
        overrides = [o for o in load_overrides(args.overrides)
                     if o.get("py_file") == py_name]
        go_index = gofuncinfo(args.gofuncinfo)
        # Same validation as the full audit: an invalid/ambiguous exclusion
        # or override must be reported (and ignored) in single-file mode too.
        # Entries targeting other files are out of scope here, not invalid.
        problems, valid_exclusions = validate_exclusions(
            exclusions, {py_name: extract_python_functions(py_path)}, go_index)
        o_problems, valid_overrides = validate_overrides(
            overrides, {py_name: extract_python_functions(py_path)}, go_index)
        problems = problems + o_problems
        result = audit_file(py_name, go_index=go_index,
                            exclusions=valid_exclusions, overrides=valid_overrides)
        if result:
            if args.json:
                print(json.dumps(result, indent=2, default=str))
            else:
                print_report(result, verbose=True)
        if problems:
            print("\n⚠ VALIDATION PROBLEMS:")
            for p in problems:
                print(f"  - {p}")
            sys.exit(1)
        return

    results, exclusions, problems = audit_all(args.exclusions, args.gofuncinfo,
                                              args.overrides)

    if args.init_exclusions:
        if args.exclusions == DEFAULT_EXCLUSIONS:
            parser.error("--init-exclusions writes TODO skeletons; pass an explicit "
                         "--exclusions PATH (not the real manifest)")
        init_exclusions(results, args.exclusions)
        return

    if args.metric:
        print_metric(results, exclusions)
        if problems:
            print("⚠ VALIDATION PROBLEMS:")
            for p in problems:
                print(f"  - {p}")
            sys.exit(1)
        return

    if args.json:
        # Full-run JSON: the advertised --json flag now applies to the whole
        # audit, not only --file mode.
        print(json.dumps({"results": results,
                          "exclusions": exclusions,
                          "problems": problems}, indent=2, default=str))
        sys.exit(1 if (problems or any(
            s in r["counts"] for r in results for s in GAP_STATUSES)) else 0)

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
