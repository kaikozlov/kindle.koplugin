#!/usr/bin/env python3
"""Compare Python and Go pipeline traces for KFX→EPUB parity verification.

Takes two trace JSON files (one from Python, one from Go) and produces a
stage-by-stage comparison report showing exactly where the pipelines diverge.

Usage:
    python scripts/compare_traces.py marty_python_trace.json marty_go_trace.json
    python scripts/compare_traces.py marty_python_trace.json marty_go_trace.json --verbose
"""

import argparse
import difflib
import json
import re
import sys
from collections import Counter


class ComparisonResult:
    def __init__(self):
        self.stages_matched = 0
        self.stages_differed = 0
        self.details = []  # list of stage reports
        self.errors = []

    @property
    def total_stages(self):
        return self.stages_matched + self.stages_differed


def load_trace(path):
    with open(path) as f:
        return json.load(f)


def compare_values(a, b, path=""):
    """Deep comparison returning list of (path, py_val, go_val) differences."""
    diffs = []
    if type(a) != type(b):
        diffs.append((path, a, b))
        return diffs
    if isinstance(a, dict):
        all_keys = sorted(set(list(a.keys()) + list(b.keys())))
        for key in all_keys:
            subpath = f"{path}.{key}" if path else key
            if key not in a:
                diffs.append((subpath, "<missing>", b[key]))
            elif key not in b:
                diffs.append((subpath, a[key], "<missing>"))
            else:
                diffs.extend(compare_values(a[key], b[key], subpath))
    elif isinstance(a, list):
        if len(a) != len(b):
            diffs.append((f"{path}.length", len(a), len(b)))
        for i in range(min(len(a), len(b))):
            diffs.extend(compare_values(a[i], b[i], f"{path}[{i}]"))
    elif a != b:
        diffs.append((path, a, b))
    return diffs


def truncate(s, max_len=120):
    s = str(s)
    if len(s) > max_len:
        return s[:max_len] + "..."
    return s


def compare_organize_fragments(py, go):
    """Compare organize_fragments stage."""
    report = {"stage": "organize_fragments", "status": "match", "details": []}
    py_types = py.get("fragment_types", {})
    go_types = go.get("fragment_types", {})

    py_keys = set(py_types.keys())
    go_keys = set(go_types.keys())

    missing_in_go = sorted(py_keys - go_keys)
    extra_in_go = sorted(go_keys - py_keys)
    common = sorted(py_keys & go_keys)

    if missing_in_go:
        report["status"] = "differ"
        report["details"].append(f"Fragment types missing in Go: {missing_in_go}")
    if extra_in_go:
        report["status"] = "differ"
        report["details"].append(f"Fragment types extra in Go: {extra_in_go}")

    count_diffs = []
    id_diffs = []
    for ft in common:
        py_count = py_types[ft].get("count", 0) if isinstance(py_types[ft], dict) else py_types[ft]
        go_count = go_types[ft].get("count", 0) if isinstance(go_types[ft], dict) else go_types[ft]
        if py_count != go_count:
            count_diffs.append(f"{ft}: Python={py_count}, Go={go_count}")
            report["status"] = "differ"

        py_ids = py_types[ft].get("ids", []) if isinstance(py_types[ft], dict) else []
        go_ids = go_types[ft].get("ids", []) if isinstance(go_types[ft], dict) else []
        if py_ids and go_ids and py_ids != go_ids:
            id_diffs.append(f"{ft}: {len(py_ids)} vs {len(go_ids)} IDs differ")
            report["status"] = "differ"

    if count_diffs:
        report["details"].append(f"Count mismatches: {'; '.join(count_diffs[:10])}")
    if id_diffs:
        report["details"].append(f"ID mismatches: {'; '.join(id_diffs[:10])}")

    matched = len(common) - len(count_diffs) - len(id_diffs)
    report["details"].insert(0, f"{matched}/{len(common)} fragment types match")

    return report


def compare_book_symbol_format(py, go):
    report = {"stage": "book_symbol_format", "status": "match", "details": []}
    py_fmt = py.get("format", "").lower()
    go_fmt = go.get("format", "").lower()
    if py_fmt != go_fmt:
        report["status"] = "differ"
        report["details"].append(f"Python={py_fmt}, Go={go_fmt}")
    else:
        report["details"].append(f"Both: {py_fmt}")
    return report


def compare_metadata(py, go):
    report = {"stage": "metadata", "status": "match", "details": []}
    diffs = compare_values(py, go)
    if diffs:
        report["status"] = "differ"
        for path, py_val, go_val in diffs[:10]:
            report["details"].append(f"  {path}: Python={truncate(py_val)}, Go={truncate(go_val)}")
    else:
        report["details"].append("All fields match")
    return report


def compare_navigation(py, go):
    report = {"stage": "navigation", "status": "match", "details": []}

    py_toc = py.get("ncx_toc", [])
    go_toc = go.get("ncx_toc", [])
    if len(py_toc) != len(go_toc):
        report["status"] = "differ"
        report["details"].append(f"TOC entries: Python={len(py_toc)}, Go={len(go_toc)}")
    else:
        # Deep compare
        toc_diffs = compare_values(py_toc, go_toc)
        if toc_diffs:
            report["status"] = "differ"
            report["details"].append(f"TOC has {len(toc_diffs)} differences")
            for path, pv, gv in toc_diffs[:5]:
                report["details"].append(f"  {path}: Py={truncate(pv)}, Go={truncate(gv)}")
        else:
            report["details"].append(f"TOC: {len(py_toc)} entries match")

    py_guide = py.get("guide", [])
    go_guide = go.get("guide", [])
    if len(py_guide) != len(go_guide):
        report["status"] = "differ"
        report["details"].append(f"Guide entries: Python={len(py_guide)}, Go={len(go_guide)}")
    elif compare_values(py_guide, go_guide):
        guide_diffs = compare_values(py_guide, go_guide)
        report["status"] = "differ"
        report["details"].append(f"Guide has {len(guide_diffs)} differences")
    else:
        report["details"].append(f"Guide: {len(py_guide)} entries match")

    py_pages = py.get("pagemap", [])
    go_pages = go.get("pagemap", [])
    if len(py_pages) != len(go_pages):
        report["status"] = "differ"
        report["details"].append(f"Page map: Python={len(py_pages)}, Go={len(go_pages)}")
    else:
        report["details"].append(f"Page map: {len(py_pages)} entries")

    return report


def html_first_diff(py_html, go_html, context=80):
    """Find the first difference between two HTML strings and return context."""
    py_lines = py_html.splitlines()
    go_lines = go_html.splitlines()

    for i, (pl, gl) in enumerate(zip(py_lines, go_lines)):
        if pl != gl:
            # Find char-level diff in this line
            for j, (pc, gc) in enumerate(zip(pl, gl)):
                if pc != gc:
                    return {
                        "line": i + 1,
                        "char": j + 1,
                        "python_context": pl[max(0, j - context):j + context],
                        "go_context": gl[max(0, j - context):j + context],
                    }
            # One line is a prefix of the other
            if len(pl) != len(gl):
                return {
                    "line": i + 1,
                    "char": min(len(pl), len(gl)) + 1,
                    "python_context": truncate(pl, 200),
                    "go_context": truncate(gl, 200),
                }

    if len(py_lines) != len(go_lines):
        return {
            "line": min(len(py_lines), len(go_lines)) + 1,
            "char": 0,
            "python_context": f"{'more lines' if len(py_lines) > len(go_lines) else 'fewer lines'}",
            "go_context": f"{'more lines' if len(go_lines) > len(py_lines) else 'fewer lines'}",
        }
    return None


def compare_reading_order(py, go):
    report = {"stage": "reading_order", "status": "match", "details": []}

    py_sections = py if isinstance(py, dict) else {}
    go_sections = go if isinstance(go, dict) else {}

    # Normalize filenames: Python uses "/c0.xhtml", Go uses "c0.xhtml"
    def norm_fname(f):
        return f.lstrip("/")

    py_sections = {norm_fname(k): v for k, v in py_sections.items()}
    go_sections = {norm_fname(k): v for k, v in go_sections.items()}

    py_keys = set(py_sections.keys())
    go_keys = set(go_sections.keys())

    missing_in_go = sorted(py_keys - go_keys)
    extra_in_go = sorted(go_keys - py_keys)
    common = sorted(py_keys & go_keys)

    if missing_in_go:
        report["status"] = "differ"
        report["details"].append(f"Sections missing in Go ({len(missing_in_go)}): {missing_in_go[:5]}")
    if extra_in_go:
        report["status"] = "differ"
        report["details"].append(f"Sections extra in Go ({len(extra_in_go)}): {extra_in_go[:5]}")

    html_diffs = []
    class_diffs = []
    for fname in common:
        py_sec = py_sections[fname]
        go_sec = go_sections[fname]

        # Compare body class
        py_class = py_sec.get("body_class", "") if isinstance(py_sec, dict) else ""
        go_class = go_sec.get("body_class", "") if isinstance(go_sec, dict) else ""
        if py_class != go_class:
            class_diffs.append(f"{fname}: Python={py_class!r}, Go={go_class!r}")

        # Compare body HTML
        py_html = py_sec.get("body_html", "") if isinstance(py_sec, dict) else ""
        go_html = go_sec.get("body_html", "") if isinstance(go_sec, dict) else ""
        if py_html != go_html:
            first_diff = html_first_diff(py_html, go_html)
            diff_info = f"{fname}: length Py={len(py_html)}, Go={len(go_html)}"
            if first_diff:
                diff_info += f"\n    First diff at line {first_diff['line']} char {first_diff['char']}"
                diff_info += f"\n    Python: ...{first_diff['python_context']}..."
                diff_info += f"\n    Go:     ...{first_diff['go_context']}..."
            html_diffs.append(diff_info)

    if class_diffs:
        report["status"] = "differ"
        report["details"].append(f"Body class differences ({len(class_diffs)}):")
        for d in class_diffs[:10]:
            report["details"].append(f"  {d}")

    if html_diffs:
        report["status"] = "differ"
        report["details"].append(f"HTML differences ({len(html_diffs)} sections):")
        for d in html_diffs[:10]:
            report["details"].append(f"  {d}")

    matched = len(common) - len(html_diffs)
    report["details"].insert(0, f"{matched}/{len(common)} sections have matching HTML")
    if not class_diffs and not html_diffs:
        report["details"].insert(0, f"All {len(common)} sections match perfectly")

    return report


def compare_stylesheet(py, go):
    report = {"stage": "stylesheet", "status": "match", "details": []}

    py_css = py.get("css_content", "")
    go_css = go.get("css_content", "")

    if py_css == go_css:
        report["details"].append(f"CSS matches ({len(py_css)} chars)")
        return report

    report["status"] = "differ"
    report["details"].append(f"CSS length: Python={len(py_css)}, Go={len(go_css)}")

    # Parse CSS into rules
    py_rules = parse_css_rules(py_css)
    go_rules = parse_css_rules(go_css)

    py_selectors = set(py_rules.keys())
    go_selectors = set(go_rules.keys())

    missing = sorted(py_selectors - go_selectors)
    extra = sorted(go_selectors - py_selectors)
    common = sorted(py_selectors & go_selectors)

    if missing:
        report["details"].append(f"Missing classes ({len(missing)}): {missing[:8]}")
    if extra:
        report["details"].append(f"Extra classes ({len(extra)}): {extra[:8]}")

    prop_diffs = []
    for sel in common:
        if py_rules[sel] != go_rules[sel]:
            py_props = py_rules[sel]
            go_props = go_rules[sel]
            all_keys = sorted(set(list(py_props.keys()) + list(go_props.keys())))
            diffs_for_sel = []
            for key in all_keys:
                if key not in py_props:
                    diffs_for_sel.append(f"+{key}: {go_props[key]}")
                elif key not in go_props:
                    diffs_for_sel.append(f"-{key}: {py_props[key]}")
                elif py_props[key] != go_props[key]:
                    diffs_for_sel.append(f"~{key}: Py={py_props[key]}, Go={go_props[key]}")
            if diffs_for_sel:
                prop_diffs.append(f"  {sel}: {'; '.join(diffs_for_sel[:3])}")

    if prop_diffs:
        report["details"].append(f"Property differences ({len(prop_diffs)} classes):")
        report["details"].extend(prop_diffs[:10])

    return report


def compare_final_sections(py, go):
    report = {"stage": "final_sections", "status": "match", "details": []}

    py_secs = py if isinstance(py, dict) else {}
    go_secs = go if isinstance(go, dict) else {}

    # Normalize filenames
    def norm_fname(f):
        return f.lstrip("/")

    py_secs = {norm_fname(k): v for k, v in py_secs.items()}
    go_secs = {norm_fname(k): v for k, v in go_secs.items()}

    py_keys = set(py_secs.keys())
    go_keys = set(go_secs.keys())

    missing_in_go = sorted(py_keys - go_keys)
    extra_in_go = sorted(go_keys - py_keys)
    common = sorted(py_keys & go_keys)

    diffs = []
    for fname in common:
        py_html = py_secs[fname] if isinstance(py_secs[fname], str) else ""
        go_html = go_secs[fname] if isinstance(go_secs[fname], str) else ""
        if py_html != go_html:
            diffs.append(f"{fname}: Py={len(py_html)}, Go={len(go_html)} chars")

    if missing_in_go:
        report["status"] = "differ"
        report["details"].append(f"Missing in Go: {missing_in_go[:5]}")
    if extra_in_go:
        report["status"] = "differ"
        report["details"].append(f"Extra in Go: {extra_in_go[:5]}")
    if diffs:
        report["status"] = "differ"
        report["details"].append(f"HTML differences ({len(diffs)} sections):")
        report["details"].extend([f"  {d}" for d in diffs[:10]])

    matched = len(common) - len(diffs)
    report["details"].insert(0, f"{matched}/{len(common)} final sections match")
    return report


def compare_simple_stage(stage_name, py, go):
    """Generic comparison for simple key-value stages."""
    report = {"stage": stage_name, "status": "match", "details": []}
    diffs = compare_values(py, go)
    if diffs:
        report["status"] = "differ"
        for path, pv, gv in diffs[:10]:
            report["details"].append(f"  {path}: Python={truncate(pv)}, Go={truncate(gv)}")
    else:
        report["details"].append("All values match")
    return report


def parse_css_rules(css_text):
    """Parse CSS into selector -> {property: value}."""
    rules = {}
    css_text = re.sub(r'/\*.*?\*/', '', css_text, flags=re.DOTALL)
    pattern = re.compile(r'([^{}]+)\{([^{}]*)\}')
    for match in pattern.finditer(css_text):
        selector_raw = match.group(1).strip()
        props_raw = match.group(2).strip()
        props = {}
        for prop_line in props_raw.split(';'):
            prop_line = prop_line.strip()
            if ':' in prop_line:
                idx = prop_line.index(':')
                prop_name = prop_line[:idx].strip().lower()
                prop_value = prop_line[idx + 1:].strip()
                if prop_name:
                    props[prop_name] = prop_value
        for sel in selector_raw.split(','):
            sel = sel.strip()
            if sel:
                rules[sel] = props
    return rules


def compare_traces(py_trace, go_trace, verbose=False):
    """Main comparison function."""
    result = ComparisonResult()

    py_stages = py_trace.get("stages", {})
    go_stages = go_trace.get("stages", {})

    # Define comparison order and handlers
    stage_order = [
        "organize_fragments",
        "book_symbol_format",
        "content_features",
        "fonts",
        "document_data",
        "metadata",
        "navigation",
        "reading_order",
        "stylesheet",
        "final_sections",
    ]

    stage_handlers = {
        "organize_fragments": compare_organize_fragments,
        "book_symbol_format": compare_book_symbol_format,
        "navigation": compare_navigation,
        "reading_order": compare_reading_order,
        "stylesheet": compare_stylesheet,
        "final_sections": compare_final_sections,
    }

    for stage_name in stage_order:
        if stage_name not in py_stages and stage_name not in go_stages:
            continue

        py_data = py_stages.get(stage_name, {})
        go_data = go_stages.get(stage_name, {})

        if stage_name in stage_handlers:
            report = stage_handlers[stage_name](py_data, go_data)
        else:
            report = compare_simple_stage(stage_name, py_data, go_data)

        result.details.append(report)
        if report["status"] == "match":
            result.stages_matched += 1
        else:
            result.stages_differed += 1

    return result


def print_report(result, verbose=False):
    """Print a human-readable comparison report."""
    for report in result.details:
        status = "✓" if report["status"] == "match" else "✗"
        print(f"\n{status} STAGE: {report['stage']}")
        for detail in report["details"]:
            print(f"  {detail}")

    print(f"\n{'=' * 60}")
    print(f"SUMMARY: {result.stages_matched} stages match, {result.stages_differed} stages differ")
    if result.stages_differed == 0:
        print("RESULT: PASS — all pipeline stages match")
    else:
        print("RESULT: FAIL — differences found")
    print(f"{'=' * 60}")


def main():
    parser = argparse.ArgumentParser(
        description="Compare Python and Go KFX pipeline traces"
    )
    parser.add_argument("python_trace", help="Python trace JSON file")
    parser.add_argument("go_trace", help="Go trace JSON file")
    parser.add_argument("--verbose", "-v", action="store_true", help="Show more detail")
    parser.add_argument("--json", action="store_true", help="Output JSON report")
    args = parser.parse_args()

    py_trace = load_trace(args.python_trace)
    go_trace = load_trace(args.go_trace)

    result = compare_traces(py_trace, go_trace, verbose=args.verbose)

    if args.json:
        output = {
            "python_input": py_trace.get("input", ""),
            "go_input": go_trace.get("input", ""),
            "stages_matched": result.stages_matched,
            "stages_differed": result.stages_differed,
            "details": result.details,
        }
        print(json.dumps(output, indent=2, ensure_ascii=False))
    else:
        print(f"Python trace: {args.python_trace}")
        print(f"Go trace:     {args.go_trace}")
        print_report(result, verbose=args.verbose)

    sys.exit(1 if result.stages_differed > 0 else 0)


if __name__ == "__main__":
    main()
