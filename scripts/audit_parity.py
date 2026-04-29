#!/usr/bin/env python3
"""
audit_parity.py — Exact Python→Go function parity audit.

Rules:
  - Every Python def (including class methods, __init__, etc.) must have a 
    corresponding Go func with a matching name.
  - Python snake_case → Go camelCase (first letter also capitalized if exported).
  - Python Class.__init__ → Go NewClass() or part of struct initialization.
  - Functions must exist as SEPARATE named functions — inlining counts as MISSING.
  - Function order within each file should match Python order.

Usage:
  python3 scripts/audit_parity.py                  # Full audit
  python3 scripts/arity_parity.py --file yj_to_epub_content  # Single file
  python3 scripts/audit_parity.py --json            # JSON output
  python3 scripts/audit_parity.py --metric          # Just the METRIC line
"""

import ast
import re
import sys
import os
import json
import argparse
from dataclasses import dataclass, field
from typing import Optional

BASE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PY_DIR = os.path.join(BASE, "REFERENCE/Calibre_KFX_Input/kfxlib")
GO_DIR = os.path.join(BASE, "internal/kfx")
PYTAGO_DIR = os.path.join(BASE, "REFERENCE/pytago_test_new/go_output")


@dataclass
class PyFunc:
    name: str
    class_name: Optional[str]
    line_start: int
    line_end: int
    args: str  # argument names only
    docstring_first_line: Optional[str]
    
    @property
    def is_dunder(self):
        return self.name.startswith("__") and self.name.endswith("__")
    
    @property
    def is_private(self):
        return self.name.startswith("_") and not self.is_dunder


def snake_to_camel(name: str) -> str:
    """snake_case → camelCase (Go unexported style)"""
    parts = name.split("_")
    if len(parts) == 1:
        return name
    return parts[0].lower() + "".join(p.capitalize() for p in parts[1:])


def snake_to_exported(name: str) -> str:
    """snake_case → CamelCase (Go exported style)"""
    return "".join(p.capitalize() for p in name.split("_"))


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
            if (node.body and isinstance(node.body[0], ast.Expr) and
                isinstance(node.body[0].value, ast.Constant) and
                isinstance(node.body[0].value.value, str)):
                doc = node.body[0].value.value.split("\n")[0][:80]
            result.append(PyFunc(
                name=node.name,
                class_name=class_name,
                line_start=node.lineno,
                line_end=node.end_lineno or node.lineno,
                args=", ".join(arg_names),
                docstring_first_line=doc,
            ))
            # Nested functions
            for child in node.body:
                visit(child)
        elif isinstance(node, ast.ClassDef):
            for child in node.body:
                visit(child, class_name=node.name)
    
    for node in tree.body:
        visit(node)
    return result


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


def expected_go_names(pf: PyFunc) -> list[str]:
    """All possible Go function names for a Python function."""
    names = []
    
    camel = snake_to_camel(pf.name)
    exported = snake_to_exported(pf.name)
    names.extend([camel, exported])
    
    # __init__ → NewClassName
    if pf.name == "__init__" and pf.class_name:
        cls_camel = snake_to_camel(pf.class_name)
        cls_exported = snake_to_exported(pf.class_name)
        names.extend([
            f"New{snake_to_exported(pf.class_name)}",
            f"new{snake_to_exported(pf.class_name)}",
        ])
    
    # __repr__, __str__ → String(), GoString()
    if pf.name in ("__repr__", "__str__"):
        names.extend(["String", "GoString"])
    
    # __eq__ → Equal
    if pf.name == "__eq__":
        names.extend(["Equal"])
    
    # __lt__ → Less
    if pf.name == "__lt__":
        names.extend(["Less"])
    
    # __len__ → Len
    if pf.name == "__len__":
        names.extend(["Len"])
    
    # __hash__ → Hash
    if pf.name == "__hash__":
        names.extend(["Hash"])
    
    # __getitem__ → Get / At
    if pf.name == "__getitem__":
        names.extend(["Get", "At"])
    
    # __contains__ → Contains
    if pf.name == "__contains__":
        names.extend(["Contains"])
    
    # __copy__ → Copy
    if pf.name == "__copy__":
        names.extend(["Copy"])
    
    # __deepcopy__ → DeepCopy
    if pf.name == "__deepcopy__":
        names.extend(["DeepCopy"])
    
    # Class method: class_name.method → method name alone (Go receiver pattern)
    if pf.class_name and pf.name != "__init__":
        pass  # Already added camel/exported above
    
    return list(dict.fromkeys(names))  # dedupe preserving order


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


def audit_file(py_name: str, go_funcs: dict = None) -> dict:
    """Audit a single Python file against its Go counterpart."""
    py_path = os.path.join(PY_DIR, py_name)
    go_name = py_name.replace(".py", ".go")
    go_path = os.path.join(GO_DIR, go_name)
    
    if not os.path.exists(py_path):
        return None
    
    py_funcs = extract_python_functions(py_path)
    if go_funcs is None:
        go_funcs = extract_go_functions(go_path)
    
    # Also extract pytago functions for reference
    pytago_path = os.path.join(PYTAGO_DIR, go_name)
    pytago_funcs = extract_go_functions(pytago_path)
    
    matched = []
    missing = []
    
    for pf in py_funcs:
        candidates = expected_go_names(pf)
        found = False
        for cand in candidates:
            if cand in go_funcs:
                matched.append({
                    "py_name": pf.name,
                    "py_class": pf.class_name,
                    "go_name": cand,
                    "py_line": pf.line_start,
                    "go_line": go_funcs[cand][0],
                })
                found = True
                break
        
        if not found:
            # Check if it's in pytago
            in_pytago = any(c in pytago_funcs for c in candidates)
            missing.append({
                "py_name": pf.name,
                "py_class": pf.class_name,
                "py_line_start": pf.line_start,
                "py_line_end": pf.line_end,
                "expected_go_names": candidates[:3],  # top 3
                "in_pytago": in_pytago,
                "is_dunder": pf.is_dunder,
                "is_private": pf.is_private,
            })
    
    return {
        "python_file": py_name,
        "go_file": go_name,
        "go_exists": os.path.exists(go_path),
        "python_function_count": len(py_funcs),
        "matched_count": len(matched),
        "missing_count": len(missing),
        "missing": missing,
        "matched": matched,
    }


def print_report(result, verbose=False):
    if result is None:
        return
    
    py = result["python_file"]
    go = result["go_file"]
    total = result["python_function_count"]
    matched = result["matched_count"]
    missing_count = result["missing_count"]
    pct = (matched / total * 100) if total > 0 else 100
    
    icon = "✓" if missing_count == 0 else "✗"
    print(f"{icon} {py} → {go}  ({matched}/{total} = {pct:.0f}%)")
    
    if missing_count > 0:
        # Group by class
        by_class = {}
        for m in result["missing"]:
            cls = m["py_class"] or "(top-level)"
            if cls not in by_class:
                by_class[cls] = []
            by_class[cls].append(m)
        
        for cls, items in sorted(by_class.items()):
            if cls != "(top-level)":
                print(f"  {cls}:")
            for m in items:
                prefix = f"    {cls}." if cls != "(top-level)" else "  "
                pytago_mark = " [pytago✓]" if m["in_pytago"] else ""
                dunder_mark = " [dunder]" if m["is_dunder"] else ""
                private_mark = " [private]" if m["is_private"] else ""
                print(f"{prefix}{m['py_name']}  L{m['py_line_start']}-{m['py_line_end']}  → {m['expected_go_names'][0]}{pytago_mark}{dunder_mark}{private_mark}")
    
    if verbose and result["matched"]:
        print(f"  Matched functions:")
        for m in result["matched"][:30]:
            cls = f" ({m['py_class']})" if m["py_class"] else ""
            print(f"    {m['py_name']}{cls} → {m['go_name']}  (py:L{m['py_line']} go:L{m['go_line']})")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--file", "-f", help="Specific file to audit (without extension)")
    parser.add_argument("--verbose", "-v", action="store_true")
    parser.add_argument("--json", action="store_true")
    parser.add_argument("--metric", action="store_true", help="Only output METRIC line")
    args = parser.parse_args()
    
    os.chdir(BASE)
    
    if args.file:
        name = args.file.replace(".py", "").replace(".go", "")
        py_name = name + ".py"
        result = audit_file(py_name)
        if result:
            if args.json:
                print(json.dumps(result, indent=2))
            else:
                print_report(result, verbose=True)
        return
    
    all_results = []
    total_matched = 0
    total_functions = 0
    total_missing = 0
    
    for py_name in FILES_TO_AUDIT:
        result = audit_file(py_name)
        if not result:
            continue
        all_results.append(result)
        total_matched += result["matched_count"]
        total_functions += result["python_function_count"]
        total_missing += result["missing_count"]
        
        if not args.metric:
            if result["missing_count"] > 0 or args.verbose:
                print_report(result, verbose=args.verbose)
                print()
    
    pct = (total_matched / total_functions * 100) if total_functions > 0 else 100
    
    if not args.metric:
        print(f"{'='*60}")
        print(f"TOTAL: {total_matched}/{total_functions} functions ({pct:.1f}%)")
        print(f"Missing: {total_missing}")
    
    print(f"METRIC missing_functions={total_missing}")
    print(f"METRIC total_functions={total_functions}")
    print(f"METRIC matched_functions={total_matched}")
    print(f"METRIC parity_pct={pct:.1f}")


if __name__ == "__main__":
    main()
