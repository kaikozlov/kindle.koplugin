#!/usr/bin/env python3
"""Count branch coverage between Python and Go across core conversion files.

Honesty rules (2026 audit):
  - branches are only matched inside the resolved Go counterpart's BODY
    (audit_branches.resolve_go_body), never whole files / all sources
  - "weak" matches (universal-pattern heuristics) are reported separately
    and do NOT count as coverage
  - functions with a hollow/missing Go counterpart contribute uncertain
    branches, not silent coverage

Usage: python3 scripts/audit_missing_branches.py [--metric]
"""
import ast, contextlib, io, os, re, sys

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PY_DIR = os.path.join(REPO_ROOT, "REFERENCE/KFX_Input/kfxlib")
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import audit_branches  # noqa: E402

CORE_FILES = [
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
]


def get_functions(py_name):
    """Extract all function defs from a Python file, including nested functions."""
    py_path = os.path.join(PY_DIR, py_name)
    with open(py_path) as f:
        tree = ast.parse(f.read())

    funcs = []
    def visit(node, class_name=None):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            funcs.append({"name": node.name, "class": class_name})
            for child in ast.iter_child_nodes(node):
                visit(child, class_name)
        elif isinstance(node, ast.ClassDef):
            for child in ast.iter_child_nodes(node):
                visit(child, node.name)
        else:
            for child in ast.iter_child_nodes(node):
                visit(child, class_name)

    for node in tree.body:
        visit(node)
    return funcs


def audit_function(py_name, func_name):
    """Run audit_branches on a single function (in-process) and return counts."""
    py_path = os.path.join(PY_DIR, py_name)
    go_path = audit_branches.find_go_file(REPO_ROOT, py_name)

    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        try:
            audit_branches.audit_function(py_path, go_path, func_name)
        except SystemExit:
            pass
    text = out.getvalue()

    def num(label):
        m = re.search(label + r':\s*(\d+)', text)
        return int(m.group(1)) if m else 0

    return (num(r'✓ Found in Go'), num(r'~ Weak'), num(r'✗ Missing in Go'), num(r'\? Uncertain'))


def main():
    metric_mode = "--metric" in sys.argv

    total_found = 0
    total_weak = 0
    total_missing = 0
    total_uncertain = 0

    for py_name in CORE_FILES:
        funcs = get_functions(py_name)
        file_found = 0
        file_weak = 0
        file_missing = 0
        file_uncertain = 0

        for func in funcs:
            if func["name"].startswith("__") and func["name"].endswith("__"):
                continue

            f, w, m, u = audit_function(py_name, func["name"])
            file_found += f
            file_weak += w
            file_missing += m
            file_uncertain += u

        total_found += file_found
        total_weak += file_weak
        total_missing += file_missing
        total_uncertain += file_uncertain

        if not metric_mode:
            total = file_found + file_weak + file_missing + file_uncertain
            print(f"  {py_name}: {file_found} found, {file_weak} weak, "
                  f"{file_missing} missing, {file_uncertain} uncertain ({total} total)")

    total = total_found + total_weak + total_missing + total_uncertain
    pct = (total_found / total * 100) if total > 0 else 100
    print(f"METRIC found_branches={total_found}")
    print(f"METRIC weak_branches={total_weak}")
    print(f"METRIC missing_branches={total_missing}")
    print(f"METRIC uncertain_branches={total_uncertain}")
    print(f"METRIC total_branches={total}")
    print(f"METRIC branch_coverage_pct={pct:.1f}")
    if not metric_mode:
        print(f"\nTOTAL: {total_found} found, {total_weak} weak, {total_missing} missing, "
              f"{total_uncertain} uncertain ({total} total, {pct:.1f}% strong coverage)")


if __name__ == "__main__":
    main()
