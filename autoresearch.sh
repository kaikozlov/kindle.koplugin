#!/bin/bash
set -euo pipefail

# autoresearch.sh — Parity benchmark: count structural diffs between Go and Calibre EPUBs
#
# For each test book, runs Go conversion and compares against Calibre reference.
# Reports structured METRIC lines for autoresearch loop consumption.

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_root="$script_dir"

tmpdir="$(mktemp -d /tmp/parity-bench.XXXXXX)"
cleanup() { rm -rf "$tmpdir"; }
trap cleanup EXIT

# Define fixtures: name|input_path|reference_epub
declare -a FIXTURES
FIXTURES+=("Martyr|${project_root}/REFERENCE/books/martyr/input.kfx|${project_root}/REFERENCE/books/martyr/calibre.epub")
FIXTURES+=("Elvis|${project_root}/REFERENCE/books/elvis/input.kfx-zip|${project_root}/REFERENCE/books/elvis/calibre.epub")
FIXTURES+=("SunriseReaping|${project_root}/REFERENCE/books/sunrise_reaping/input.kfx-zip|${project_root}/REFERENCE/books/sunrise_reaping/calibre.epub")
FIXTURES+=("SecretsCrown|${project_root}/REFERENCE/books/secrets_crown/input.kfx-zip|${project_root}/REFERENCE/books/secrets_crown/calibre.epub")
FIXTURES+=("1984|${project_root}/REFERENCE/books/1984/input.kfx-zip|${project_root}/REFERENCE/books/1984/calibre.epub")
FIXTURES+=("ThreeBelow|${project_root}/REFERENCE/books/three_below/input.kfx-zip|${project_root}/REFERENCE/books/three_below/calibre.epub")
FIXTURES+=("Familiars|${project_root}/REFERENCE/books/familiars/input.kfx-zip|${project_root}/REFERENCE/books/familiars/calibre.epub")
FIXTURES+=("HungerGames|${project_root}/REFERENCE/books/hunger_games/input.kfx-zip|${project_root}/REFERENCE/books/hunger_games/calibre.epub")
FIXTURES+=("ThroneOfGlass|${project_root}/REFERENCE/books/throne_of_glass/input.kfx-zip|${project_root}/REFERENCE/books/throne_of_glass/calibre.epub")
FIXTURES+=("HeatedRivalry|${project_root}/REFERENCE/books/heated_rivalry/input.kfx-zip|${project_root}/REFERENCE/books/heated_rivalry/calibre.epub")

total_structural=0
total_image=0
total_books=0
total_missing=0
failed_books=""

for fixture in "${FIXTURES[@]}"; do
    IFS='|' read -r name input ref <<< "$fixture"

    if [[ ! -f "$input" ]]; then
        echo "# SKIP: $name (no input file)"
        continue
    fi
    if [[ ! -f "$ref" ]]; then
        echo "# SKIP: $name (no reference)"
        continue
    fi

    total_books=$((total_books + 1))
    go_epub="$tmpdir/${name}.epub"
    go_dir="$tmpdir/${name}_go"
    ref_dir="$tmpdir/${name}_ref"

    # Run Go conversion
    if ! (cd "$project_root" && go run ./cmd/kindle-helper convert --input "$input" --output "$go_epub" 2>/dev/null); then
        echo "# ERROR: $name conversion failed"
        failed_books="${failed_books}${name},"
        continue
    fi

    # Unpack both EPUBs
    rm -rf "$go_dir" "$ref_dir"
    mkdir -p "$go_dir" "$ref_dir"
    unzip -qq "$go_epub" -d "$go_dir" 2>/dev/null || true
    unzip -qq "$ref" -d "$ref_dir" 2>/dev/null || true

    # Count diffs using Python (reuse the diff logic from diff_kfx_parity.sh)
    result=$(python3 - <<PY "$go_dir" "$ref_dir" "$name"
import sys, os, re, hashlib
from pathlib import Path

go_root = Path(sys.argv[1])
ref_root = Path(sys.argv[2])
name = sys.argv[3]

# Find OEBPS subdirs
go_oebps = go_root / "OEBPS"
ref_oebps = ref_root / "OEBPS"

text_exts = {".css", ".html", ".ncx", ".opf", ".xhtml", ".xml"}
modified_re = re.compile(r'<meta property="dcterms:modified">.*?</meta>', re.DOTALL)

structural = 0
image = 0
missing = 0
details = []

seen = set()
for root, side in [(ref_oebps, "ref"), (go_oebps, "go")]:
    if not root.exists():
        continue
    for path in sorted(root.iterdir()):
        if path.name in seen:
            continue
        if path.is_dir():
            continue
        seen.add(path.name)
        ref_path = ref_oebps / path.name
        go_path = go_oebps / path.name

        if not ref_path.exists() or ref_path.is_dir():
            missing += 1
            details.append(f"  {path.name}: extra in go")
            continue
        if not go_path.exists() or go_path.is_dir():
            missing += 1
            details.append(f"  {path.name}: missing in go")
            continue

        ref_bytes = ref_path.read_bytes()
        go_bytes = go_path.read_bytes()

        if ref_bytes == go_bytes:
            continue

        is_text = path.suffix.lower() in text_exts
        if is_text:
            # Check if only timestamp differs
            try:
                ref_text = ref_path.read_text("utf-8")
                go_text = go_path.read_text("utf-8")
                ref_norm = modified_re.sub("", ref_text).strip()
                go_norm = modified_re.sub("", go_text).strip()
                if ref_norm == go_norm:
                    continue
            except:
                pass
            structural += 1
            details.append(f"  {path.name}: structural diff")
        else:
            image += 1
            details.append(f"  {path.name}: image diff")

print(f"{structural}|{image}|{missing}")
if details:
    for d in details:
        print(d, file=sys.stderr)
PY
)

    IFS='|' read -r s i m <<< "$result"
    structural=${s:-0}
    image=${i:-0}
    mis=${m:-0}

    total_structural=$((total_structural + structural))
    total_image=$((total_image + image))
    total_missing=$((total_missing + mis))

    echo "# $name: ${structural} structural, ${image} image, ${mis} missing"
done

echo ""
echo "METRIC structural_diffs=${total_structural}"
echo "METRIC image_diffs=${total_image}"
echo "METRIC missing_files=${total_missing}"
echo "METRIC books_tested=${total_books}"

if [[ -n "$failed_books" ]]; then
    echo "# FAILED: $failed_books"
fi
