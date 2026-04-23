#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
plugin_dir="$(cd "$script_dir/.." && pwd)"
# The plugin directory IS the repo root (kindle.koplugin is the top-level project)
repo_root="$plugin_dir"

default_input="$repo_root/REFERENCE/kfx_examples/Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx"
default_reference="$repo_root/REFERENCE/martyr_calibre.epub"

input_path="$default_input"
reference_epub="$default_reference"
show_text_diffs=1
keep_tmp=0
ignore_modified=0

usage() {
  cat <<'EOF'
Usage: scripts/diff_kfx_parity.sh [options]

Options:
  --input PATH              KFX input file to convert
  --reference-epub PATH     Reference EPUB to compare against
  --summary-only            Print differing files only
  --keep-tmp                Keep unpacked temp directory
  --ignore-modified         Ignore content.opf diffs if dcterms:modified is the only change
  -h, --help                Show this help

Defaults:
  --input          REFERENCE/kfx_examples/Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx
  --reference-epub REFERENCE/martyr_calibre.epub
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --input)
      input_path="$2"
      shift 2
      ;;
    --reference-epub)
      reference_epub="$2"
      shift 2
      ;;
    --summary-only)
      show_text_diffs=0
      shift
      ;;
    --keep-tmp)
      keep_tmp=1
      shift
      ;;
    --ignore-modified)
      ignore_modified=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ! -f "$input_path" ]]; then
  echo "Input KFX not found: $input_path" >&2
  exit 2
fi

if [[ ! -f "$reference_epub" ]]; then
  echo "Reference EPUB not found: $reference_epub" >&2
  exit 2
fi

tmpdir="$(mktemp -d /tmp/kfx-parity.XXXXXX)"
cleanup() {
  if [[ "$keep_tmp" -eq 0 ]]; then
    rm -rf "$tmpdir"
  fi
}
trap cleanup EXIT

output_epub="$tmpdir/output.epub"
mkdir -p "$tmpdir/go" "$tmpdir/ref"

(
  cd "$plugin_dir"
  go run ./cmd/kindle-helper convert --input "$input_path" --output "$output_epub" >/dev/null
)

unzip -qq "$output_epub" -d "$tmpdir/go"
unzip -qq "$reference_epub" -d "$tmpdir/ref"

echo "Converted: $input_path"
echo "Reference: $reference_epub"
echo "Temp dir:  $tmpdir"

python3 - <<'PY' "$tmpdir" "$show_text_diffs" "$ignore_modified"
from pathlib import Path
import difflib
import hashlib
import re
import sys

base = Path(sys.argv[1])
show_text_diffs = sys.argv[2] == "1"
ignore_modified = sys.argv[3] == "1"
go_root = base / "go" / "OEBPS"
ref_root = base / "ref" / "OEBPS"

text_exts = {".css", ".html", ".ncx", ".opf", ".xhtml", ".xml"}
modified_re = re.compile(
    r"<meta property=\"dcterms:modified\">.*?</meta>",
    re.DOTALL,
)

def is_text_file(path: Path) -> bool:
    return path.suffix.lower() in text_exts

def read_text(path: Path):
    try:
        return path.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        return None

def only_modified_diff(name: str, ref_text: str, go_text: str) -> bool:
    if name != "content.opf":
        return False
    ref_norm = modified_re.sub("", ref_text).strip()
    go_norm = modified_re.sub("", go_text).strip()
    return ref_norm == go_norm

seen = set()
diffs = []

for root, side in ((ref_root, "ref"), (go_root, "go")):
    for path in sorted(root.iterdir()):
        if path.name in seen:
            continue
        seen.add(path.name)
        ref_path = ref_root / path.name
        go_path = go_root / path.name
        if not ref_path.exists():
            diffs.append((path.name, "extra in go", None, None))
            continue
        if not go_path.exists():
            diffs.append((path.name, "missing in go", None, None))
            continue
        ref_bytes = ref_path.read_bytes()
        go_bytes = go_path.read_bytes()
        if ref_bytes == go_bytes:
            continue
        ref_text = read_text(ref_path) if is_text_file(ref_path) else None
        go_text = read_text(go_path) if is_text_file(go_path) else None
        if ignore_modified and ref_text is not None and go_text is not None and only_modified_diff(path.name, ref_text, go_text):
            continue
        diffs.append((path.name, "different", ref_text, go_text))

if not diffs:
    print("\nNo diffs.")
    sys.exit(0)

print("\nDiffering files:")
for name, status, _, _ in diffs:
    print(f"  {name}: {status}")

if show_text_diffs:
    for name, status, ref_text, go_text in diffs:
        if ref_text is None or go_text is None:
            continue
        print(f"\n===== {name} =====")
        diff = difflib.unified_diff(
            ref_text.splitlines(),
            go_text.splitlines(),
            fromfile="ref",
            tofile="go",
            n=2,
        )
        print("\n".join(diff))

if not show_text_diffs:
    sys.exit(1)

for name, status, ref_text, go_text in diffs:
    if ref_text is not None and go_text is not None:
        continue
    ref_path = ref_root / name
    go_path = go_root / name
    if not ref_path.exists() or not go_path.exists():
        continue
    ref_bytes = ref_path.read_bytes()
    go_bytes = go_path.read_bytes()
    print(f"\n===== {name} =====")
    print(f"ref_bytes={len(ref_bytes)} go_bytes={len(go_bytes)}")
    print(f"ref_sha256={hashlib.sha256(ref_bytes).hexdigest()}")
    print(f"go_sha256={hashlib.sha256(go_bytes).hexdigest()}")

sys.exit(1)
PY
