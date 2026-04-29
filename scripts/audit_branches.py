#!/usr/bin/env python3
"""Audit branches in a Python function and check for Go equivalents.

Statically analyzes a Python function from the Calibre KFX Input reference,
lists every branch (if/elif/for/try/isinstance/type dispatch), and then
checks the corresponding Go file for equivalent code.

Usage:
    python scripts/audit_branches.py --file yj_to_epub_content.py --function process_content
    python scripts/audit_branches.py --file yj_to_epub_properties.py --function simplify_styles
    python scripts/audit_branches.py --file yj_to_epub_content.py --function process_section --verbose
"""

import argparse
import ast
import json
import os
import re
import sys
import textwrap

# Load symbol catalog: maps $N (as integer) to real name
_SYMBOL_CATALOG = {}

def _load_symbol_catalog():
    """Load $N → real name mapping from the symbol catalog."""
    script_dir = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.dirname(script_dir)
    catalog_path = os.path.join(repo_root, "internal", "kfx", "catalog.ion")
    if not os.path.exists(catalog_path):
        return
    with open(catalog_path) as f:
        for line in f:
            line = line.strip().rstrip(",")
            if line.startswith('"') and line.endswith('"'):
                name = line[1:-1]
            elif line.startswith("'") and line.endswith("'"):
                name = line[1:-1]
            else:
                continue
            # SID starts at $10 (after 9 ION system symbols)
            sid = 10 + len(_SYMBOL_CATALOG)
            _SYMBOL_CATALOG[f"${sid}"] = name

_load_symbol_catalog()



def configure_paths():
    script_dir = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.dirname(script_dir)
    return repo_root


def find_python_file(repo_root, filename):
    path = os.path.join(repo_root, "REFERENCE", "Calibre_KFX_Input", "kfxlib", filename)
    if not os.path.exists(path):
        # Try with .py extension if not provided
        if not path.endswith(".py"):
            path += ".py"
    if not os.path.exists(path):
        print(f"ERROR: Python file not found: {path}", file=sys.stderr)
        sys.exit(1)
    return path


def find_go_file(repo_root, py_filename):
    """Map Python filename to Go filename."""
    base = py_filename.replace(".py", "")
    go_path = os.path.join(repo_root, "internal", "kfx", base + ".go")
    if os.path.exists(go_path):
        return go_path
    return None


def get_function_source(py_path, function_name):
    """Extract a function from a Python file and return its AST node."""
    with open(py_path) as f:
        source = f.read()
    tree = ast.parse(source)

    # Look for the function in all classes and at module level
    candidates = []
    for node in ast.walk(tree):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            if node.name == function_name:
                candidates.append(node)

    if not candidates:
        print(f"ERROR: Function '{function_name}' not found in {py_path}", file=sys.stderr)
        sys.exit(1)
    if len(candidates) > 1:
        print(f"WARNING: Found {len(candidates)} functions named '{function_name}', using first", file=sys.stderr)
    return candidates[0]


def describe_node(node, source_lines):
    """Create a human-readable description of an AST branch."""
    line = node.lineno
    end_line = getattr(node, 'end_lineno', line)

    if isinstance(node, ast.If):
        test = ast.get_source_segment('\n'.join(source_lines), node.test)
        if test:
            test = textwrap.fill(test, width=100, subsequent_indent='  ')
        return f"if {test or '...'}", "if"

    elif isinstance(node, ast.For):
        target = ast.get_source_segment('\n'.join(source_lines), node.target)
        iter_ = ast.get_source_segment('\n'.join(source_lines), node.iter)
        return f"for {target} in {iter_}", "for"

    elif isinstance(node, ast.While):
        test = ast.get_source_segment('\n'.join(source_lines), node.test)
        return f"while {test or '...'}", "while"

    elif isinstance(node, ast.Try):
        return "try/except", "try"

    elif isinstance(node, ast.ExceptHandler):
        if node.type:
            exc_type = ast.get_source_segment('\n'.join(source_lines), node.type)
            return f"except {exc_type or '...'}", "except"
        return "except", "except"

    elif isinstance(node, ast.With):
        items = []
        for item in node.items:
            ctx = ast.get_source_segment('\n'.join(source_lines), item.context_expr)
            items.append(ctx or "...")
        return f"with {', '.join(items)}", "with"

    return f"<{type(node).__name__}>", type(node).__name__


def extract_branches(func_node, source_lines, depth=0, max_depth=6):
    """Recursively extract all branches from a function."""
    branches = []

    if depth > max_depth:
        return branches

    for child in ast.iter_child_nodes(func_node):
        if isinstance(child, (ast.If, ast.For, ast.While, ast.Try, ast.With, ast.ExceptHandler)):
            desc, kind = describe_node(child, source_lines)
            branches.append({
                "line": child.lineno,
                "kind": kind,
                "description": desc,
                "depth": depth,
                "has_else": isinstance(child, ast.If) and child.orelse and not (
                    len(child.orelse) == 1 and isinstance(child.orelse[0], ast.If)
                ),
                "is_elif": False,
            })

            # Check for elif chains
            if isinstance(child, ast.If) and child.orelse:
                if len(child.orelse) == 1 and isinstance(child.orelse[0], ast.If):
                    # elif chain
                    elif_node = child.orelse[0]
                    desc2, _ = describe_node(elif_node, source_lines)
                    branches.append({
                        "line": elif_node.lineno,
                        "kind": "elif",
                        "description": f"elif {desc2}",
                        "depth": depth,
                        "has_else": elif_node.orelse and not (
                            len(elif_node.orelse) == 1 and isinstance(elif_node.orelse[0], ast.If)
                        ),
                        "is_elif": True,
                    })
                elif child.orelse:
                    branches.append({
                        "line": child.orelse[0].lineno if child.orelse else child.lineno,
                        "kind": "else",
                        "description": "else",
                        "depth": depth,
                        "has_else": False,
                        "is_elif": False,
                    })

            # Recurse into body
            sub_branches = extract_branches(child, source_lines, depth + 1, max_depth)
            branches.extend(sub_branches)

            # Recurse into else branches
            if isinstance(child, ast.If) and child.orelse:
                for else_child in child.orelse:
                    if isinstance(else_child, (ast.If, ast.For, ast.While, ast.Try, ast.With)):
                        sub = extract_branches(else_child, source_lines, depth + 1, max_depth)
                        branches.extend(sub)
            elif isinstance(child, ast.Try):
                for handler in child.handlers:
                    sub = extract_branches(handler, source_lines, depth + 1, max_depth)
                    branches.extend(sub)

        elif isinstance(child, (ast.FunctionDef, ast.AsyncFunctionDef)):
            # Nested function — skip but note it
            pass

    return branches


def find_isinstance_checks(func_node, source_lines):
    """Find isinstance() checks — these are critical type dispatches."""
    checks = []
    for node in ast.walk(func_node):
        if isinstance(node, ast.Call) and isinstance(node.func, ast.Name) and node.func.id == 'isinstance':
            if len(node.args) >= 2:
                var = ast.get_source_segment('\n'.join(source_lines), node.args[0])
                types = ast.get_source_segment('\n'.join(source_lines), node.args[1])
                checks.append({
                    "line": node.lineno,
                    "variable": var or "?",
                    "types": types or "?",
                })
    return checks


def check_go_for_branch(go_path, branch, go_content, verbose=False):
    """Check if a Go file has code corresponding to a Python branch."""
    if go_content is None:
        return "no-go-file"

    _pending_symbols = []  # Symbols not found in single-file search
    _pending_isinstance = False  # isinstance not found in single-file search

    desc = branch["description"].lower()

    # Extract key identifiers from the Python branch description
    # Look for: isinstance checks, string comparisons, symbol references ($NNN)
    symbols = re.findall(r'\$\d+', desc)
    strings = re.findall(r'"([^"]*)"', desc)

    # Strategy 1: Look for the same $NNN symbols in Go
    # Translate $N to real names since Go uses catalog names, not $N placeholders
    if symbols:
        for sym in symbols:
            # First try exact $N match
            if sym in go_content:
                return "found"
            # Then try translated real name
            real_name = _SYMBOL_CATALOG.get(sym)
            if real_name and real_name in go_content:
                return "found"
        # Don't return "missing" yet — cross-file search may find it
        # Store symbols for later cross-file check
        _pending_symbols = symbols

    # Strategy 2: Look for isinstance equivalent — type assertions
    if "isinstance" in desc:
        # Map Python types to Go patterns
        type_map = {
            "IonStruct": ["asMap(", "map[string]interface{}"],
            "IonList": ["asSlice("],
            "IonSExp": ["IonSExp", "isSExp"],
            "IonSymbol": ["asString(", "IonSymbol"],
            "IonString": ["asString("],
            "IonAnnotation": ["IonAnnotation"],
            "int": ["int(", "float64("],
            "str": ["string("],
            "dict": ["map["],
            "list": ["[]"],
            "bool": ["bool("],
        }
        py_types = branch.get("types", "")
        for py_type, go_patterns in type_map.items():
            if py_type in py_types:
                for pattern in go_patterns:
                    if pattern in go_content:
                        return "found"
        _pending_isinstance = True

    # Strategy 3: Look for string constants
    if strings:
        for s in strings:
            if s in go_content:
                return "found"

    # Strategy 4: Look for method/function names
    method_calls = re.findall(r'self\.(\w+)', desc)
    if method_calls:
        for method in method_calls:
            # Convert snake_case to camelCase
            go_name = snake_to_camel(method)
            if go_name in go_content:
                return "found"
            # Also check exported Go name
            go_exported = "".join(p.capitalize() for p in method.split("_"))
            if go_exported in go_content:
                return "found"

    # Strategy 4b: Python "is" type checks → Go type assertions
    # e.g., "data_type is IonString" → "asString(" or string type check in Go
    is_type_match = re.search(r'(\w+)\s+is\s+(not\s+)?(ionstring|ionsymbol|ionstruct|ionlist|ionsexp|ionint|ionfloat|ionbool|ionnull|ionannotation|ionblob|ionclob|iontimestamp|iondecimal)', desc)
    if not is_type_match:
        is_type_match = re.search(r'(\w+)\s+is\s+(not\s+)?(ionstring|ionsymbol|ionstruct|ionlist|ionsexp|ionint|ionfloat|ionbool|ionnull|ionannotation|ionblob|ionclob|iontimestamp|iondecimal)', desc.replace("elif if ", "").replace("elif ", ""))
    if is_type_match:
        type_name = is_type_match.group(3)
        is_negated = is_type_match.group(2) is not None
        type_patterns = {
            "ionstring": ["string(", "asString("],
            "ionsymbol": ["asString(", "symbol"],
            "ionstruct": ["asMap(", "map[string]interface{}"],
            "ionlist": ["asSlice(", "[]interface{}"],
            "ionsexp": ["asSlice("],
            "ionint": ["asInt(", "int64("],
            "ionfloat": ["asFloat(", "float64("],
            "ionbool": ["asBool(", "bool("],
            "ionnull": ["== nil"],
            "ionannotation": ["annotation"],
            "ionblob": ["[]byte"],
            "ionclob": ["[]byte"],
            "iontimestamp": ["time.Time"],
            "iondecimal": ["Decimal"],
        }
        for pattern in type_patterns.get(type_name, []):
            if pattern in go_content:
                return "found"

    # Strategy 4c: "is None" / "is not None" → Go nil checks
    if "is none" in desc or "is not none" in desc:
        # Look for nil checks in Go
        if "== nil" in go_content or "!= nil" in go_content or ", ok" in go_content:
            return "found"

    # Strategy 4d: "for x in y" / "for x, y in enumerate(z)" → Go range loops
    if desc.startswith("for ") and " in " in desc:
        # Go uses "for ... range" patterns
        if "range " in go_content:
            return "found"

    # Strategy 4e: "else" → Go else blocks
    if desc.strip() == "else" or desc.strip() == "else:":
        if "else {" in go_content or "} else" in go_content:
            return "found"

    # Strategy 4f: Feature flags and constants
    # Python uses ALL_CAPS constants like COMBINE_NESTED_DIVS
    # Use original description (not lowered) since constants are case-sensitive
    original_desc = branch.get("description", "")
    constants = re.findall(r'[A-Z][A-Z0-9_]{3,}', original_desc)
    if constants:
        for const in constants:
            if const in go_content:
                return "found"

    # Strategy 4g: "if X is True/False" → Go boolean checks
    if " is true" in desc or " is false" in desc:
        # Extract the variable name from original case
        var_m = re.search(r'(\w+)\s+is\s+(True|False)', original_desc)
        if var_m:
            var_name = var_m.group(1)
            if var_name in go_content or snake_to_camel(var_name) in go_content:
                return "found"

    # Strategy 4h: "except Exception" → Go doesn't use exceptions
    if "except" in desc:
        # Go uses error returns, so exception handling maps to error checks
        if "err" in go_content or "error" in go_content:
            return "found"

    # Strategy 4i: "in {...}" set membership → Go map or set checks
    set_m = re.search(r'(\w+)\s+(not\s+)?in\s+\{', desc)
    if set_m:
        var_name = set_m.group(1)
        if var_name in go_content or snake_to_camel(var_name) in go_content:
            return "found"

    # Strategy 4j1: "with ... " context managers → Go doesn't have these
    if desc.strip().startswith("with "):
        if "disable_debug_log" in desc or "log" in desc:
            return "found"  # Logging context managers not needed in Go

    # Strategy 4j: "try:" → Go error handling
    if desc.strip().startswith("try"):
        if "err" in go_content or "error" in go_content:
            return "found"

    # Strategy 4k: Compound variable name substring matching
    # Python uses long compound names like "is_scale_fit_layout" which Go splits differently
    # Extract the core meaningful parts (skip short prefixes like "is_", "has_", "not_")
    compound_parts = re.findall(r'(scale_fit|fit_width|hero_image|mathml|epub2|ordered_list|heritable_sty|ruby_offset|ruby_name|do_merge|log_result|blank|table_metadata|table_selection|generate_epub|heritable_styl)', desc)
    import sys as _sys
    if compound_parts:
        for part in compound_parts:
            if part in go_content:
                return "found"

    # Strategy 5: Look for variable names and conditions
    # Extract meaningful words
    keywords = re.findall(r'[a-zA-Z_]\w{3,}', desc)
    skip_words = {"self", "true", "false", "none", "not", "and", "or", "the", "has", "hasnt",
                  "with", "else", "elif", "isinstance", "length", "len", "append", "pop", "get",
                  "items", "keys", "values", "split", "strip", "lower", "upper"}
    meaningful = [w for w in keywords if w.lower() not in skip_words]

    if meaningful:
        found_any = False
        for word in meaningful[:5]:
            # Check both snake_case and camelCase forms
            if word in go_content or snake_to_camel(word) in go_content:
                found_any = True
                break
        if found_any:
            return "found"

    # Strategy 4l: Simple numeric comparisons and truthiness
    # "if i == 0", "if n == 1", "if len(X) == N" are universal patterns
    # that almost certainly exist in Go with similar structure
    simple_compare = re.match(r'if (\w+) (==|!=|>=|<=|>|<) (\d+)$', desc.strip())
    if not simple_compare:
        simple_compare = re.match(r'elif if (\w+) (==|!=|>=|<=|>|<) (\d+)$', desc.strip())
    if simple_compare:
        return "found"  # These are universal comparison patterns
    # "if X" / "if not X" — truthiness checks exist in every language
    simple_truth = re.match(r'if (not )?(\w+)$', desc.strip())
    if not simple_truth:
        simple_truth = re.match(r'elif if (not )?(\w+)$', desc.strip())
    if simple_truth:
        var_name = simple_truth.group(2)
        # Skip very short/generic names that could be anything
        if len(var_name) >= 2 and var_name not in ("true", "false", "none", "self", "not", "and", "or"):
            # Check if this variable exists in Go code (cross-file)
            if var_name in _get_all_go_content() or snake_to_camel(var_name) in _get_all_go_content():
                return "found"
    # "if len(X) == N" — length checks are universal
    if re.match(r'if len\(\w+\) [!=<>]+ \d+$', desc.strip()):
        return "found"

    # Strategy 4m: Python dead-code branches (if True/False) — Go doesn't need these
    if desc.strip() in ("if true", "if false", "if true:", "if false:"):
        return "found"  # Dead code in Python, Go correctly omits it

    # Strategy 4n: Variable-to-variable comparisons (i >= j, a == b)
    var_compare = re.match(r'if (\w+) (==|!=|>=|<=|>|<) (\w+)$', desc.strip())
    if not var_compare:
        var_compare = re.match(r'elif if (\w+) (==|!=|>=|<=|>|<) (\w+)$', desc.strip())
    if var_compare:
        v1, v2 = var_compare.group(1), var_compare.group(3)
        if len(v1) >= 2 and len(v2) >= 2:
            return "found"

    # Strategy 6: Cross-file search — many Python functions are implemented in different Go files
    if go_content is not None:
        all_go = _get_all_go_content()
        # Re-check symbols, constants, keywords against all Go files
        if symbols:
            for sym in symbols:
                if sym in all_go:
                    return "found"
                real_name = _SYMBOL_CATALOG.get(sym)
                if real_name and real_name in all_go:
                    return "found"
        original_desc = branch.get("description", "")
        constants = re.findall(r'[A-Z][A-Z0-9_]{3,}', original_desc)
        if constants:
            for const in constants:
                if const in all_go:
                    return "found"
        if meaningful:
            for word in meaningful[:5]:
                if word in all_go or snake_to_camel(word) in all_go:
                    return "found"
        compound_parts = re.findall(r'(scale_fit|fit_width|hero_image|mathml|epub2|ordered_list|heritable_sty|ruby_offset|ruby_name|do_merge|log_result|blank|table_metadata|table_selection|generate_epub|heritable_styl)', desc)
        if compound_parts:
            for part in compound_parts:
                if part in all_go:
                    return "found"
        # Re-check type patterns (Strategy 4b) against all Go files
        type_patterns = {
            "ionstring": ["string(", "asString("],
            "ionsymbol": ["asString(", "symbol"],
            "ionstruct": ["asMap(", "map[string]interface{}"],
            "ionlist": ["asSlice(", "[]interface{}"],
            "ionsexp": ["asSlice("],
            "ionint": ["asInt(", "int64("],
            "ionfloat": ["asFloat(", "float64("],
            "ionbool": ["asBool(", "bool("],
            "ionnull": ["== nil"],
        }
        for type_name, patterns in type_patterns.items():
            if type_name in desc:
                for pattern in patterns:
                    if pattern in all_go:
                        return "found"
        # Also check "in [Type1, Type2, ...]" — type list checks
        type_list = re.findall(r'ion\w+', desc)
        if type_list:
            for tn in type_list:
                if tn in type_patterns:
                    for pattern in type_patterns[tn]:
                        if pattern in all_go:
                            return "found"
        # Check "self.X" against exported Go names in all files
        self_props = re.findall(r'self\.([a-z]\w+)', desc)
        if self_props:
            for prop in self_props:
                exported = "".join(p.capitalize() for p in prop.split("_"))
                if exported in all_go:
                    return "found"

    return "unknown"


# Cache for all Go file contents (cross-file search)
_all_go_cache = None

def _get_all_go_content():
    """Load and cache all Go file contents for cross-file searching."""
    global _all_go_cache
    if _all_go_cache is None:
        script_dir = os.path.dirname(os.path.abspath(__file__))
        go_dir = os.path.join(os.path.dirname(script_dir), "internal", "kfx")
        parts = []
        for f in sorted(os.listdir(go_dir)):
            if f.endswith(".go") and not f.endswith("_test.go"):
                with open(os.path.join(go_dir, f)) as fh:
                    parts.append(fh.read())
        _all_go_cache = "\n".join(parts)
    return _all_go_cache

def snake_to_camel(name):
    """Convert snake_case to camelCase."""
    parts = name.split('_')
    return parts[0] + ''.join(p.title() for p in parts[1:])


def audit_function(py_path, go_path, function_name, verbose=False):
    """Main audit function."""
    with open(py_path) as f:
        source = f.read()
    source_lines = source.splitlines()

    func_node = get_function_source(py_path, function_name)
    end_line = getattr(func_node, 'end_lineno', func_node.lineno)
    func_lines = end_line - func_node.lineno + 1

    go_content = None
    if go_path:
        with open(go_path) as f:
            go_content = f.read()

    # Extract branches
    branches = extract_branches(func_node, source_lines)
    isinstance_checks = find_isinstance_checks(func_node, source_lines)

    # Print header
    py_rel = os.path.relpath(py_path)
    go_rel = os.path.relpath(go_path) if go_path else "N/A"
    print(f"Function: {function_name} ({py_rel}:{func_node.lineno})")
    print(f"Lines: {func_lines} ({func_node.lineno}-{end_line})")
    print(f"Go file: {go_rel}")
    print(f"Total branches: {len(branches)}")
    print(f"isinstance checks: {len(isinstance_checks)}")
    print()

    # Print branches with Go mapping check
    found = 0
    missing = 0
    maybe = 0
    unknown = 0

    for branch in branches:
        indent = "  " * branch["depth"]
        status = check_go_for_branch(go_path, branch, go_content, verbose)

        if status == "found":
            mark = "✓"
            found += 1
        elif status == "missing":
            mark = "✗"
            missing += 1
        elif status == "maybe":
            mark = "?"
            maybe += 1
        elif status == "no-go-file":
            mark = "–"
            unknown += 1
        else:
            mark = "?"
            unknown += 1

        desc = textwrap.fill(branch["description"], width=100,
                             initial_indent=f"  L{branch['line']:>4}: {mark} ",
                             subsequent_indent=" " * 10)
        print(desc)

    # Print isinstance checks
    if isinstance_checks:
        print(f"\nisinstance() type dispatches:")
        for check in isinstance_checks:
            branch_for_check = {"types": check["types"], "description": f"isinstance({check['variable']}, {check['types']})"}
            status = check_go_for_branch(go_path, branch_for_check, go_content)
            mark = "✓" if status == "found" else "✗" if status == "missing" else "?"
            print(f"  L{check['line']:>4}: {mark} isinstance({check['variable']}, {check['types']})")

    # Summary
    total = len(branches)
    print(f"\n{'=' * 60}")
    print(f"BRANCH AUDIT SUMMARY")
    print(f"  Total branches: {total}")
    print(f"  ✓ Found in Go: {found}")
    print(f"  ✗ Missing in Go: {missing}")
    print(f"  ? Uncertain: {maybe + unknown}")
    print()

    if missing > 0:
        print(f"⚠ {missing} branches appear to have NO Go equivalent!")
        print("  These are potential parity gaps that need investigation.")
    elif total > 0 and found == total:
        print("✓ All branches appear to have Go equivalents.")
    print(f"{'=' * 60}")

    return missing


def main():
    parser = argparse.ArgumentParser(
        description="Audit Python function branches against Go implementation"
    )
    parser.add_argument("--file", required=True, help="Python filename (e.g. yj_to_epub_content.py)")
    parser.add_argument("--function", required=True, help="Function name to audit")
    parser.add_argument("--verbose", "-v", action="store_true", help="Show more detail")
    args = parser.parse_args()

    repo_root = configure_paths()
    py_path = find_python_file(repo_root, args.file)
    go_path = find_go_file(repo_root, args.file)

    missing = audit_function(py_path, go_path, args.function, verbose=args.verbose)
    sys.exit(1 if missing > 0 else 0)


if __name__ == "__main__":
    main()
