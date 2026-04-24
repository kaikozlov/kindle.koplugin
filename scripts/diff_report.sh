#!/usr/bin/env bash
set -euo pipefail

# diff_report.sh — Structured CSS diff analysis between Python (Calibre) and Go KFX→EPUB pipelines
#
# For each test KFX file, runs both pipelines, extracts CSS from output EPUBs,
# and produces a machine-readable markdown diff report.
#
# Output order: Summary → Per-File Analysis → Property Frequency Analysis

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
plugin_dir="$(cd "$script_dir/.." && pwd)"
repo_root="$(cd "$plugin_dir/.." && pwd)"

# --- Test files definition ---
# Format: label|input_path|python_ref_epub
declare -a TEST_FILES=(
  "Martyr|${repo_root}/REFERENCE/books/martyr/input.kfx|/tmp/martyr_python_ref.epub"
  "Elvis|${repo_root}/REFERENCE/KFX_DRM/decrypted/Elvis and the Underdogs_B009NG3090_decrypted.kfx-zip|/tmp/Elvis and the Underdogs_B009NG3090_calibre.epub"
  "HG|${repo_root}/REFERENCE/KFX_DRM/decrypted/The Hunger Games Trilogy_B004XJRQUQ_decrypted.kfx-zip|/tmp/The Hunger Games Trilogy_B004XJRQUQ_calibre.epub"
  "3B|${repo_root}/REFERENCE/KFX_DRM/decrypted/Three Below (Floors #2)_B008PL1YQ0_decrypted.kfx-zip|/tmp/Three Below (Floors #2)_B008PL1YQ0_calibre.epub"
)

tmpdir="$(mktemp -d /tmp/diff-report.XXXXXX)"
cleanup() { rm -rf "$tmpdir"; }
trap cleanup EXIT

mkdir -p "$tmpdir"

# --- Helper: Run Python pipeline ---
run_python() {
  local input="$1" output="$2"
  python3 "$script_dir/convert_kfx_python.py" --input "$input" --output "$output" 2>/dev/null
}

# --- Helper: Run Go pipeline ---
run_go() {
  local input="$1" output="$2"
  (cd "$plugin_dir" && go run ./cmd/kindle-helper convert --input "$input" --output "$output" 2>/dev/null)
}

# --- Helper: Extract CSS from EPUB ---
extract_css() {
  local epub="$1" outfile="$2"
  # Try OEBPS/stylesheet.css first, then fall back to any .css
  if unzip -p "$epub" OEBPS/stylesheet.css > "$outfile" 2>/dev/null; then
    return 0
  fi
  # Try to find any CSS file
  local css_entry
  css_entry=$(unzip -l "$epub" '*.css' 2>/dev/null | tail -1 | awk '{print $NF}')
  if [[ -n "$css_entry" ]]; then
    unzip -p "$epub" "$css_entry" > "$outfile" 2>/dev/null
    return 0
  fi
  # No CSS found
  echo "" > "$outfile"
  return 1
}

# --- Output buffers ---
report_header="$tmpdir/report_header.md"
summary_section="$tmpdir/report_summary.md"
per_file_output="$tmpdir/report_perfile.md"
prop_freq_section="$tmpdir/report_propfreq.md"
global_prop_diffs="$tmpdir/global_prop_diffs.txt"
summary_data="$tmpdir/summary.txt"

> "$report_header"
> "$summary_section"
> "$per_file_output"
> "$prop_freq_section"
> "$global_prop_diffs"
> "$summary_data"

# --- Header ---
{
  echo "# KFX→EPUB CSS Diff Report"
  echo ""
  echo "Generated: $(date -Iseconds)"
  echo ""
} > "$report_header"

files_processed=0

# --- Process each test file ---
for entry in "${TEST_FILES[@]}"; do
  IFS='|' read -r label input_path python_ref <<< "$entry"

  echo "Processing: $label ..." >&2

  # Check input exists
  if [[ ! -f "$input_path" ]]; then
    echo "  SKIP: input not found: $input_path" >&2
    {
      echo "### $label"
      echo ""
      echo "**SKIPPED**: Input file not found: \`$input_path\`"
      echo ""
    } >> "$per_file_output"
    echo "$label|SKIP|0|0|0|0" >> "$summary_data"
    continue
  fi

  go_output="$tmpdir/${label}_go.epub"

  # Run Python pipeline if ref doesn't exist
  if [[ ! -f "$python_ref" ]]; then
    echo "  Running Python pipeline for $label..." >&2
    if ! run_python "$input_path" "$python_ref" 2>/dev/null; then
      echo "  Python pipeline failed for $label" >&2
      {
        echo "### $label"
        echo ""
        echo "**ERROR**: Python pipeline failed"
        echo ""
      } >> "$per_file_output"
      echo "$label|ERROR|0|0|0|0" >> "$summary_data"
      continue
    fi
  fi
  py_output="$python_ref"

  # Run Go pipeline
  echo "  Running Go pipeline for $label..." >&2
  if ! run_go "$input_path" "$go_output"; then
    echo "  Go pipeline failed for $label" >&2
    {
      echo "### $label"
      echo ""
      echo "**ERROR**: Go pipeline failed"
      echo ""
    } >> "$per_file_output"
    echo "$label|ERROR|0|0|0|0" >> "$summary_data"
    continue
  fi

  # Extract CSS
  py_css="$tmpdir/${label}_python.css"
  go_css="$tmpdir/${label}_go.css"

  if ! extract_css "$py_output" "$py_css"; then
    echo "  No CSS in Python output for $label" >&2
  fi
  if ! extract_css "$go_output" "$go_css"; then
    echo "  No CSS in Go output for $label" >&2
  fi

  # Summary data file for this label
  label_summary="$tmpdir/${label}_summary.txt"

  # Run Python analysis for CSS diff — output to per_file buffer + accumulate prop diffs
  python3 - "$py_css" "$go_css" "$global_prop_diffs" "$label" "$label_summary" >> "$per_file_output" <<'PYEOF'
import sys
import re

py_css_file = sys.argv[1]
go_css_file = sys.argv[2]
global_prop_file = sys.argv[3]
label = sys.argv[4]
summary_file = sys.argv[5]

def parse_css(css_text):
    """Parse CSS into a dict of selector -> {property: value}"""
    rules = {}
    # Remove comments
    css_text = re.sub(r'/\*.*?\*/', '', css_text, flags=re.DOTALL)
    # Find all rule blocks
    pattern = re.compile(r'([^{}]+)\{([^{}]*)\}')
    for match in pattern.finditer(css_text):
        selector_raw = match.group(1).strip()
        props_raw = match.group(2).strip()
        # Parse properties
        props = {}
        for prop_line in props_raw.split(';'):
            prop_line = prop_line.strip()
            if ':' in prop_line:
                idx = prop_line.index(':')
                prop_name = prop_line[:idx].strip().lower()
                prop_value = prop_line[idx+1:].strip()
                if prop_name:
                    props[prop_name] = prop_value
        # Handle multiple selectors separated by commas
        for sel in selector_raw.split(','):
            sel = sel.strip()
            if sel:
                rules[sel] = props
    return rules

def read_file(path):
    try:
        with open(path, 'r') as f:
            return f.read()
    except:
        return ""

py_css = read_file(py_css_file)
go_css = read_file(go_css_file)

py_rules = parse_css(py_css)
go_rules = parse_css(go_css)

py_selectors = set(py_rules.keys())
go_selectors = set(go_rules.keys())

missing = sorted(py_selectors - go_selectors)  # In Python but not Go
extra = sorted(go_selectors - py_selectors)      # In Go but not Python
common = sorted(py_selectors & go_selectors)

# Find differing classes among common selectors
differing = {}
for sel in common:
    py_props = py_rules[sel]
    go_props = go_rules[sel]
    if py_props != go_props:
        all_keys = sorted(set(py_props.keys()) | set(go_props.keys()))
        diffs = []
        for key in all_keys:
            in_py = key in py_props
            in_go = key in go_props
            if in_py and not in_go:
                diffs.append(("removed", key, py_props[key], ""))
            elif in_go and not in_py:
                diffs.append(("added", key, "", go_props[key]))
            elif py_props[key] != go_props[key]:
                diffs.append(("changed", key, py_props[key], go_props[key]))
        differing[sel] = diffs

total_diffs = len(missing) + len(extra) + len(differing)

print(f"### {label}")
print(f"")
print(f"- **Total diffs**: {total_diffs}")
print(f"- Python CSS selectors: {len(py_selectors)}")
print(f"- Go CSS selectors: {len(go_selectors)}")
print(f"- Missing classes (Python only): {len(missing)}")
print(f"- Extra classes (Go only): {len(extra)}")
print(f"- Differing classes (both, props differ): {len(differing)}")
print(f"")

if missing:
    print(f"#### Missing Classes (in Python, not in Go)")
    print(f"")
    for sel in missing:
        props = py_rules[sel]
        prop_str = "; ".join(f"{k}: {v}" for k, v in sorted(props.items()))
        print(f"- `{sel}`: `{prop_str}`")
    print("")

if extra:
    print(f"#### Extra Classes (in Go, not in Python)")
    print(f"")
    for sel in extra:
        props = go_rules[sel]
        prop_str = "; ".join(f"{k}: {v}" for k, v in sorted(props.items()))
        print(f"- `{sel}`: `{prop_str}`")
    print("")

if differing:
    print(f"#### Differing Classes")
    print(f"")
    for sel in sorted(differing.keys()):
        diffs = differing[sel]
        print(f"- **`{sel}`**:")
        for diff_type, prop, py_val, go_val in diffs:
            if diff_type == "removed":
                print(f"  - `-{prop}: {py_val}` (removed in Go)")
            elif diff_type == "added":
                print(f"  - `+{prop}: {go_val}` (added in Go)")
            else:
                print(f"  - `~{prop}`: Python=`{py_val}` → Go=`{go_val}`")
            # Accumulate for global analysis
            with open(global_prop_file, 'a') as gf:
                gf.write(f"{prop}\t{diff_type}\t{label}\n")
        print("")

# Write summary data for bash to consume
with open(summary_file, 'w') as sf:
    sf.write(f"{label}|OK|{len(missing)}|{len(extra)}|{len(differing)}|{total_diffs}\n")

sys.exit(0)
PYEOF

  # Record summary data from Python output
  if [[ -f "$label_summary" ]]; then
    cat "$label_summary" >> "$summary_data"
  else
    echo "$label|ERROR|0|0|0|0" >> "$summary_data"
  fi

  files_processed=$((files_processed + 1))
done

# --- Build Summary section ---
{
  echo "## Summary"
  echo ""
  echo "| File | Status | Missing (Py→Go) | Extra (Go→Py) | Differing | Total Diffs |"
  echo "|------|--------|------------------|----------------|-----------|-------------|"

  total_missing=0
  total_extra=0
  total_differing=0
  total_total=0

  while IFS='|' read -r label status n_missing n_extra n_diff n_total; do
    echo "| $label | $status | $n_missing | $n_extra | $n_diff | $n_total |"
    total_missing=$((total_missing + ${n_missing:-0}))
    total_extra=$((total_extra + ${n_extra:-0}))
    total_differing=$((total_differing + ${n_diff:-0}))
    total_total=$((total_total + ${n_total:-0}))
  done < "$summary_data"

  echo "| **TOTAL** | | **$total_missing** | **$total_extra** | **$total_differing** | **$total_total** |"
  echo ""
} > "$summary_section"

# --- Build Property Frequency Analysis section ---
{
  echo "## Property Frequency Analysis"
  echo ""
  echo "Top 20 most commonly differing properties across all files:"
  echo ""
  echo "| Rank | Property | Count | Types |"
  echo "|------|----------|-------|-------|"

  if [[ -s "$global_prop_diffs" ]]; then
    python3 - "$global_prop_diffs" <<'PYEOF'
import sys
from collections import Counter

prop_file = sys.argv[1]
counter = Counter()
type_map = {}

with open(prop_file) as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        parts = line.split('\t')
        if len(parts) >= 2:
            prop = parts[0]
            diff_type = parts[1]
            counter[prop] += 1
            if prop not in type_map:
                type_map[prop] = set()
            type_map[prop].add(diff_type)

for rank, (prop, count) in enumerate(counter.most_common(20), 1):
    types = ", ".join(sorted(type_map[prop]))
    print(f"| {rank} | `{prop}` | {count} | {types} |")
PYEOF
  else
    echo "| - | (no differences found) | - | - |"
  fi

  echo ""
} > "$prop_freq_section"

# --- Emit final report in correct order ---
cat "$report_header"
cat "$summary_section"
echo ""
cat "$per_file_output"
cat "$prop_freq_section"
echo "---"
echo "Report complete. Files processed: $files_processed"
