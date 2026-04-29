#!/usr/bin/env python3
"""Count confirmed-missing branches between Python and Go across core conversion files.

Uses audit_branches.py to check each function and counts ✗ Missing branches.
Only checks core conversion files (not infrastructure).

Usage: python3 scripts/audit_missing_branches.py [--metric]
"""
import ast, os, subprocess, re, sys

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PY_DIR = os.path.join(REPO_ROOT, "REFERENCE/Calibre_KFX_Input/kfxlib")

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
    """Extract all function defs from a Python file."""
    py_path = os.path.join(PY_DIR, py_name)
    with open(py_path) as f:
        tree = ast.parse(f.read())

    funcs = []
    def visit(node, class_name=None):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            funcs.append({"name": node.name, "class": class_name})
            for child in node.body:
                visit(child, class_name)
        elif isinstance(node, ast.ClassDef):
            for child in node.body:
                visit(child, node.name)

    for node in tree.body:
        visit(node)
    return funcs


def audit_function(py_name, func_name):
    """Run audit_branches.py on a single function and return counts."""
    result = subprocess.run(
        ["python3", os.path.join(REPO_ROOT, "scripts/audit_branches.py"),
         "--file", py_name, "--function", func_name],
        capture_output=True, text=True, timeout=10,
        cwd=REPO_ROOT,
    )

    found = int(m.group(1)) if (m := re.search(r'Found in Go:\s*(\d+)', result.stdout)) else 0
    missing = int(m.group(1)) if (m := re.search(r'Missing in Go:\s*(\d+)', result.stdout)) else 0
    uncertain = int(m.group(1)) if (m := re.search(r'Uncertain:\s*(\d+)', result.stdout)) else 0

    return found, missing, uncertain


def main():
    metric_mode = "--metric" in sys.argv

    total_found = 0
    total_missing = 0
    total_uncertain = 0

    for py_name in CORE_FILES:
        funcs = get_functions(py_name)
        file_found = 0
        file_missing = 0
        file_uncertain = 0

        for func in funcs:
            if func["name"].startswith("__") and func["name"].endswith("__"):
                continue

            f, m, u = audit_function(py_name, func["name"])
            file_found += f
            file_missing += m
            file_uncertain += u

        total_found += file_found
        total_missing += file_missing
        total_uncertain += file_uncertain

        if not metric_mode:
            total = file_found + file_missing + file_uncertain
            print(f"  {py_name}: {file_missing} missing, {file_found} found, {file_uncertain} uncertain ({total} total)")

    if metric_mode:
        total = total_found + total_missing + total_uncertain
        print(f"METRIC missing_branches={total_missing}")
        print(f"METRIC found_branches={total_found}")
        print(f"METRIC uncertain_branches={total_uncertain}")
        print(f"METRIC total_branches={total}")
        pct = (total_found / total * 100) if total > 0 else 100
        print(f"METRIC branch_coverage_pct={pct:.1f}")
    else:
        total = total_found + total_missing + total_uncertain
        pct = (total_found / total * 100) if total > 0 else 100
        print(f"\nTOTAL: {total_missing} missing, {total_found} found, {total_uncertain} uncertain ({total} total, {pct:.1f}% coverage)")


if __name__ == "__main__":
    main()
