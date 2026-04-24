#!/usr/bin/env bash
set -euo pipefail

# parity_check.sh — Run Python trace, Go trace, and compare for one or all fixture books.
#
# Usage:
#   scripts/parity_check.sh                    # check all available fixtures
#   scripts/parity_check.sh --book Martyr      # check one book
#   scripts/parity_check.sh --book Elvis
#   scripts/parity_check.sh --keep             # keep trace JSON files

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_root="$(cd "$script_dir/.." && pwd)"

trace_dir="$project_root/REFERENCE/books/parity_traces"
mkdir -p "$trace_dir"

keep=0
book=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --book) book="$2"; shift 2 ;;
    --keep) keep=1; shift ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done

# Define fixtures
declare -A FIXTURES
FIXTURES[Martyr]="$project_root/REFERENCE/books/martyr/input.kfx"
FIXTURES[ThreeBelow]="$project_root/REFERENCE/books/three_below/input.kfx-zip"
FIXTURES[Elvis]="$project_root/REFERENCE/books/elvis/input.kfx-zip"
FIXTURES[Familiars]="$project_root/REFERENCE/books/familiars/input.kfx-zip"
FIXTURES[HungerGames]="$project_root/REFERENCE/books/hunger_games/input.kfx-zip"
FIXTURES[ThroneOfGlass]="$project_root/REFERENCE/books/throne_of_glass/input.kfx-zip"

if [[ -n "$book" ]]; then
  if [[ -z "${FIXTURES[$book]+x}" ]]; then
    echo "ERROR: Unknown book '$book'. Available: ${!FIXTURES[@]}" >&2
    exit 2
  fi
  declare -A SELECTED
  SELECTED[$book]="${FIXTURES[$book]}"
else
  # Use all available fixtures
  declare -A SELECTED
  for name in "${!FIXTURES[@]}"; do
    SELECTED[$name]="${FIXTURES[$name]}"
  done
fi

tmpdir="$(mktemp -d /tmp/parity-check.XXXXXX)"
cleanup() {
  if [[ "$keep" -eq 0 ]]; then
    rm -rf "$tmpdir"
  else
    echo "Trace files kept in: $tmpdir"
  fi
}
trap cleanup EXIT

passed=0
failed=0
skipped=0

for name in $(echo "${!SELECTED[@]}" | tr ' ' '\n' | sort); do
  input="${SELECTED[$name]}"

  echo ""
  echo "============================================================"
  echo "  Checking: $name"
  echo "============================================================"

  if [[ ! -f "$input" ]]; then
    echo "  SKIP: input file not found"
    echo "    $input"
    skipped=$((skipped + 1))
    continue
  fi

  py_trace="$tmpdir/${name}_python.json"
  go_trace="$tmpdir/${name}_go.json"
  go_epub="$tmpdir/${name}_go.epub"

  # Run Python trace
  echo "  Running Python trace..."
  if ! python3 "$script_dir/trace_python.py" --input "$input" --output "$py_trace" 2>/dev/null; then
    echo "  ERROR: Python trace failed"
    failed=$((failed + 1))
    continue
  fi

  # Run Go trace
  echo "  Running Go trace..."
  if ! (cd "$project_root" && go run ./cmd/kindle-helper trace --input "$input" --output "$go_epub" --trace "$go_trace" 2>/dev/null); then
    echo "  ERROR: Go trace failed"
    failed=$((failed + 1))
    continue
  fi

  # Compare
  echo "  Comparing traces..."
  if python3 "$script_dir/compare_traces.py" "$py_trace" "$go_trace" 2>/dev/null; then
    echo "  ✓ PASS"
    passed=$((passed + 1))
  else
    echo "  ✗ FAIL"
    failed=$((failed + 1))
  fi

  # Save traces to golden directory
  if [[ "$keep" -eq 1 ]]; then
    cp "$py_trace" "$trace_dir/${name}_python.json"
    cp "$go_trace" "$trace_dir/${name}_go.json"
    echo "  Traces saved to $trace_dir/"
  fi
done

echo ""
echo "============================================================"
echo "  PARITY CHECK SUMMARY"
echo "============================================================"
echo "  Passed:  $passed"
echo "  Failed:  $failed"
echo "  Skipped: $skipped"
echo "============================================================"

if [[ "$failed" -gt 0 ]]; then
  exit 1
fi
exit 0
