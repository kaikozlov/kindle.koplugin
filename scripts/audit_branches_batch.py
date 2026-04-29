#!/usr/bin/env python3
"""Batch branch audit: count total branches with missing Go equivalents across all files."""

import ast, os, re, sys
from pathlib import Path

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PY_DIR = os.path.join(REPO_ROOT, "REFERENCE", "Calibre_KFX_Input", "kfxlib")
GO_DIR = os.path.join(REPO_ROOT, "internal", "kfx")

# Reuse the same file list as audit_parity.py
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
    "epub_output.py",
    "ion.py",
    "ion_binary.py",
    "ion_symbol_table.py",
    "kfx_container.py",
]

# Only audit core conversion files (not infrastructure)
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


def extract_python_functions(filepath):
    """Extract all function defs with their source."""
    with open(filepath) as f:
        source = f.read()
    try:
        tree = ast.parse(source)
    except SyntaxError:
        return []

    result = []
    def visit(node, class_name=None):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            result.append({
                "name": node.name,
                "class_name": class_name,
                "line_start": node.lineno,
                "line_end": node.end_lineno or node.lineno,
                "node": node,
                "source_lines": source.splitlines()[node.lineno-1:node.end_lineno],
            })
            for child in node.body:
                visit(child, class_name)
        elif isinstance(node, ast.ClassDef):
            for child in node.body:
                visit(child, node.name)

    for node in tree.body:
        visit(node)
    return result


def count_branches(node, depth=0):
    """Count all branch-like nodes in an AST subtree."""
    branches = []
    
    def visit(n, d=0):
        if isinstance(n, ast.If):
            branches.append({"type": "if", "line": n.lineno, "test": ast.dump(n.test)[:100]})
            for child in n.body:
                visit(child, d+1)
            for child in n.orelse:
                if child and isinstance(child, ast.If):
                    branches.append({"type": "elif", "line": child.lineno, "test": ast.dump(child.test)[:100]})
                visit(child, d+1)
        elif isinstance(n, ast.For):
            branches.append({"type": "for", "line": n.lineno, "target": ast.dump(n.target)[:50]})
            for child in n.body:
                visit(child, d+1)
            for child in n.orelse:
                visit(child, d+1)
        elif isinstance(n, ast.While):
            branches.append({"type": "while", "line": n.lineno})
            for child in n.body:
                visit(child, d+1)
        elif isinstance(n, ast.Try):
            branches.append({"type": "try", "line": n.lineno})
            for child in n.body:
                visit(child, d+1)
            for handler in n.handlers:
                visit(handler, d+1)
            for child in n.orelse:
                visit(child, d+1)
            for child in n.finalbody:
                visit(child, d+1)
        elif isinstance(n, ast.With):
            branches.append({"type": "with", "line": n.lineno})
            for child in n.body:
                visit(child, d+1)
        elif isinstance(n, ast.ExceptHandler):
            for child in n.body:
                visit(child, d+1)
        elif isinstance(n, (ast.ListComp, ast.DictComp, ast.SetComp, ast.GeneratorExp)):
            branches.append({"type": "comprehension", "line": n.lineno})
        else:
            for child in ast.iter_child_nodes(n):
                if isinstance(child, ast.AST):
                    visit(child, d+1)
    
    visit(node)
    return branches


def load_go_source(go_name):
    """Load Go source for a Python file's counterpart."""
    go_path = os.path.join(GO_DIR, go_name)
    if not os.path.exists(go_path):
        # Try alternative mappings
        alt = go_name.replace("ion.py", "ion_binary.go")
        alt_path = os.path.join(GO_DIR, alt)
        if os.path.exists(alt_path):
            go_path = alt_path
        else:
            return ""
    
    with open(go_path) as f:
        return f.read()


def find_go_function(go_source, py_func):
    """Find a Go function that matches a Python function."""
    # Convert Python name to possible Go names
    name = py_func["name"]
    cn = py_func["class_name"]
    
    candidates = []
    # snake_case to camelCase
    parts = name.split("_")
    camel = parts[0].lower() + "".join(p.capitalize() for p in parts[1:]) if len(parts) > 1 else parts[0].lower()
    exported = "".join(p.capitalize() for p in parts)
    candidates.extend([camel, exported])
    
    # Also check with class prefix
    if cn:
        cn_parts = cn.split("_")
        cn_camel = "".join(p.capitalize() for p in cn_parts)
        candidates.extend([cn_camel + exported, cn_camel + camel])
    
    # Search for function definition in Go source
    for cand in candidates:
        # Match func name or method
        pattern = rf'func\s+(?:\([^)]*\)\s+)?{re.escape(cand)}\s*\('
        match = re.search(pattern, go_source)
        if match:
            # Extract the function body
            start = match.start()
            # Find matching braces
            brace_pos = go_source.index('{', start)
            depth = 0
            end = brace_pos
            for i in range(brace_pos, len(go_source)):
                if go_source[i] == '{':
                    depth += 1
                elif go_source[i] == '}':
                    depth -= 1
                    if depth == 0:
                        end = i + 1
                        break
            return go_source[start:end]
    
    return ""


def estimate_branch_coverage(py_func, go_func_src):
    """Estimate how many Python branches are covered in Go."""
    py_branches = count_branches(py_func["node"])
    
    if not go_func_src:
        return len(py_branches), 0  # All missing
    
    # Simple heuristic: count Go branches (if/for/range/switch)
    go_if_count = len(re.findall(r'\bif\s', go_func_src))
    go_for_count = len(re.findall(r'\bfor\s', go_func_src))
    go_switch_count = len(re.findall(r'\bswitch\s', go_func_src))
    go_case_count = len(re.findall(r'\bcase\s', go_func_src))
    go_total_branches = go_if_count + go_for_count + go_switch_count + go_case_count
    
    # Count matched branches
    matched = min(len(py_branches), go_total_branches)
    missing = max(0, len(py_branches) - matched)
    
    return missing, matched


def main():
    metric_mode = "--metric" in sys.argv
    core_only = "--core" in sys.argv
    
    files = CORE_FILES if core_only else FILES_TO_AUDIT
    
    total_missing = 0
    total_matched = 0
    total_branches = 0
    per_file = {}
    
    for py_name in files:
        py_path = os.path.join(PY_DIR, py_name)
        go_name = py_name.replace(".py", ".go")
        
        if not os.path.exists(py_path):
            continue
        
        funcs = extract_python_functions(py_path)
        go_source = load_go_source(go_name)
        
        file_missing = 0
        file_matched = 0
        file_total = 0
        
        for func in funcs:
            # Skip __init__, __repr__, etc. for branch counting
            if func["name"].startswith("__") and func["name"].endswith("__"):
                continue
            # Skip very small functions (< 5 lines, unlikely to have meaningful branches)
            if func["line_end"] - func["line_start"] < 5:
                continue
            
            go_func = find_go_function(go_source, func)
            missing, matched = estimate_branch_coverage(func, go_func)
            
            file_missing += missing
            file_matched += matched
            file_total += missing + matched
        
        per_file[py_name] = (file_missing, file_matched, file_total)
        total_missing += file_missing
        total_matched += file_matched
        total_branches += file_total
        
        if not metric_mode:
            pct = (file_matched / file_total * 100) if file_total > 0 else 100
            print(f"  {py_name}: {file_missing} missing, {file_matched} matched ({pct:.0f}% of {file_total})")
    
    if metric_mode:
        print(f"METRIC missing_branches={total_missing}")
        print(f"METRIC total_branches={total_branches}")
        print(f"METRIC matched_branches={total_matched}")
        pct = (total_matched / total_branches * 100) if total_branches > 0 else 100
        print(f"METRIC branch_parity_pct={pct:.1f}")
    else:
        pct = (total_matched / total_branches * 100) if total_branches > 0 else 100
        print(f"\nTOTAL: {total_missing} missing branches, {total_matched} matched ({pct:.1f}% of {total_branches})")


if __name__ == "__main__":
    main()
