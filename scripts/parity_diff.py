#!/usr/bin/env python3
"""Unified Go↔Calibre KFX→EPUB parity diff tool.

Replaces structural_diff_count.py, diff_kfx_parity.sh, and diff_report.sh
with a single entry point.

Usage:
  scripts/parity_diff.py                          # Summary for all books
  scripts/parity_diff.py --book 1984              # Detailed diff for one book
  scripts/parity_diff.py --book 1984 --diff       # Show full unified diffs
  scripts/parity_diff.py --book 1984 --images     # Show image metadata comparison
  scripts/parity_diff.py --metric                 # Machine-readable METRIC lines
  scripts/parity_diff.py --all --diff             # Full diffs for every book
  scripts/parity_diff.py --keep                   # Keep extracted EPUBs in /tmp
  scripts/parity_diff.py --css                    # CSS property-level diff
"""
import argparse
import difflib
import hashlib
import os
import re
import subprocess
import sys
import tempfile
import zipfile

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BOOKS_DIR = os.path.join(REPO_ROOT, "REFERENCE", "books")

BOOKS = [
    "martyr", "1984", "elvis", "familiars", "heated_rivalry",
    "hunger_games", "secrets_crown", "sunrise_reaping", "three_below", "throne_of_glass",
]

STRUCTURAL_EXTS = {".html", ".xhtml", ".css", ".opf", ".ncx", ".xml"}
IMAGE_EXTS = {".jpg", ".jpeg", ".png", ".gif", ".svg", ".jxr"}
TEXT_EXTS = STRUCTURAL_EXTS  # same set

MODIFIED_RE = re.compile(r'<meta property="dcterms:modified">.*?</meta>', re.DOTALL)


# ---------------------------------------------------------------------------
# Book discovery
# ---------------------------------------------------------------------------

def find_input(book_dir):
    """Find the KFX input file for a book directory."""
    for name in ("input.kfx-zip", "input.kfx"):
        path = os.path.join(book_dir, name)
        if os.path.exists(path):
            return path
    return None


# ---------------------------------------------------------------------------
# Conversion
# ---------------------------------------------------------------------------

def convert_book(input_path, output_path):
    """Run Go conversion. Returns (success, stderr)."""
    result = subprocess.run(
        ["go", "run", "./cmd/kindle-helper", "convert",
         "-input", input_path, "-output", output_path],
        capture_output=True, text=True, timeout=120,
        cwd=REPO_ROOT,
    )
    return result.returncode == 0, result.stderr


# ---------------------------------------------------------------------------
# Diffing
# ---------------------------------------------------------------------------

def file_category(fname):
    ext = os.path.splitext(fname)[1].lower()
    if ext in STRUCTURAL_EXTS:
        return "structural"
    if ext in IMAGE_EXTS:
        return "image"
    return "other"


def normalize_text(text, fname):
    """Normalize text for comparison. Strips timestamps from content.opf."""
    if fname.endswith("content.opf"):
        text = MODIFIED_RE.sub("", text)
    return text.strip()


class FileDiff:
    """One differing file between two EPUBs."""
    __slots__ = ("fname", "category", "kind", "ref_data", "go_data")

    def __init__(self, fname, category, kind, ref_data=None, go_data=None):
        self.fname = fname
        self.category = category
        self.kind = kind  # "missing_in_go", "extra_in_go", "content_diff", "timestamp_only"
        self.ref_data = ref_data
        self.go_data = go_data


def diff_epubs(cal_epub, go_epub):
    """Compare two EPUBs, return list of FileDiff."""
    diffs = []
    with zipfile.ZipFile(cal_epub) as cal_zf, zipfile.ZipFile(go_epub) as go_zf:
        cal_files = {f for f in cal_zf.namelist() if not f.endswith("/")}
        go_files = {f for f in go_zf.namelist() if not f.endswith("/")}

        for fname in sorted(cal_files | go_files):
            cat = file_category(fname)
            in_cal = fname in cal_files
            in_go = fname in go_files

            if not in_cal:
                diffs.append(FileDiff(fname, cat, "extra_in_go",
                                      go_data=go_zf.read(fname)))
                continue
            if not in_go:
                diffs.append(FileDiff(fname, cat, "missing_in_go",
                                      ref_data=cal_zf.read(fname)))
                continue

            cal_data = cal_zf.read(fname)
            go_data = go_zf.read(fname)

            if cal_data == go_data:
                continue

            # Text file — check if only timestamp differs
            if cat == "structural":
                try:
                    cal_text = cal_data.decode("utf-8", errors="replace")
                    go_text = go_data.decode("utf-8", errors="replace")
                except Exception:
                    diffs.append(FileDiff(fname, cat, "content_diff",
                                          cal_data, go_data))
                    continue

                if normalize_text(cal_text, fname) == normalize_text(go_text, fname):
                    diffs.append(FileDiff(fname, cat, "timestamp_only",
                                          cal_data, go_data))
                    continue

            diffs.append(FileDiff(fname, cat, "content_diff", cal_data, go_data))

    return diffs


# ---------------------------------------------------------------------------
# Report formatters
# ---------------------------------------------------------------------------

def fmt_bytes(n):
    if n < 1024:
        return f"{n} B"
    if n < 1024 * 1024:
        return f"{n / 1024:.1f} KB"
    return f"{n / (1024 * 1024):.1f} MB"


def image_metadata(data):
    """Extract basic JPEG metadata from raw bytes without PIL."""
    info = {}
    if data[:2] == b'\xff\xd8':
        info["format"] = "JPEG"
        # Check for JFIF marker (APP0)
        if len(data) > 4 and data[2:4] == b'\xff\xe0':
            info["jfif"] = "yes"
        else:
            info["jfif"] = "no"
    else:
        info["format"] = "unknown"
    info["size"] = fmt_bytes(len(data))
    info["sha256"] = hashlib.sha256(data).hexdigest()[:16]
    return info


def print_text_diff(fd):
    """Print unified diff for a structural FileDiff."""
    try:
        ref_text = fd.ref_data.decode("utf-8", errors="replace")
        go_text = fd.go_data.decode("utf-8", errors="replace")
    except Exception:
        print(f"  {fd.fname}: binary content differs")
        return

    lines = list(difflib.unified_diff(
        ref_text.splitlines(),
        go_text.splitlines(),
        fromfile=f"calibre/{fd.fname}",
        tofile=f"go/{fd.fname}",
        n=3,
    ))
    if lines:
        for line in lines:
            print(f"  {line}")
    else:
        # Only timestamp differed
        print(f"  {fd.fname}: (only dcterms:modified timestamp differs)")


def print_image_diff(fd):
    """Print metadata comparison for an image FileDiff."""
    ref_info = image_metadata(fd.ref_data)
    go_info = image_metadata(fd.go_data)
    print(f"  {fd.fname}:")
    print(f"    calibre: {ref_info['format']} jfif={ref_info['jfif']} {ref_info['size']} sha={ref_info['sha256']}")
    print(f"    go:      {go_info['format']} jfif={go_info['jfif']} {go_info['size']} sha={go_info['sha256']}")


def print_css_diff(fd):
    """Print CSS property-level diff."""
    try:
        ref_text = fd.ref_data.decode("utf-8", errors="replace")
        go_text = fd.go_data.decode("utf-8", errors="replace")
    except Exception:
        return

    ref_rules = parse_css(ref_text)
    go_rules = parse_css(go_text)

    ref_sels = set(ref_rules.keys())
    go_sels = set(go_rules.keys())

    missing = sorted(ref_sels - go_sels)
    extra = sorted(go_sels - ref_sels)
    common = sorted(ref_sels & go_sels)

    differing = []
    for sel in common:
        if ref_rules[sel] != go_rules[sel]:
            differing.append(sel)

    if not missing and not extra and not differing:
        return

    print(f"  CSS diff in {fd.fname}:")
    if missing:
        print(f"    Missing selectors (in Calibre, not in Go):")
        for sel in missing:
            props = ref_rules[sel]
            prop_str = "; ".join(f"{k}: {v}" for k, v in sorted(props.items()))
            print(f"      {sel} {{ {prop_str} }}")
    if extra:
        print(f"    Extra selectors (in Go, not in Calibre):")
        for sel in extra:
            props = go_rules[sel]
            prop_str = "; ".join(f"{k}: {v}" for k, v in sorted(props.items()))
            print(f"      {sel} {{ {prop_str} }}")
    if differing:
        print(f"    Differing selectors:")
        for sel in differing:
            rp = ref_rules[sel]
            gp = go_rules[sel]
            all_keys = sorted(set(rp.keys()) | set(gp.keys()))
            for key in all_keys:
                rv = rp.get(key, "—")
                gv = gp.get(key, "—")
                if rv != gv:
                    print(f"      {sel}: {key}: calibre={rv} go={gv}")


def parse_css(css_text):
    """Parse CSS into {selector: {property: value}}."""
    rules = {}
    css_text = re.sub(r'/\*.*?\*/', '', css_text, flags=re.DOTALL)
    for match in re.finditer(r'([^{}]+)\{([^{}]*)\}', css_text):
        selector_raw = match.group(1).strip()
        props_raw = match.group(2).strip()
        props = {}
        for prop_line in props_raw.split(';'):
            prop_line = prop_line.strip()
            if ':' in prop_line:
                idx = prop_line.index(':')
                props[prop_line[:idx].strip().lower()] = prop_line[idx + 1:].strip()
        for sel in selector_raw.split(','):
            sel = sel.strip()
            if sel:
                rules[sel] = props
    return rules


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="Unified Go↔Calibre KFX→EPUB parity diff tool",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""\
Examples:
  scripts/parity_diff.py                          # Summary for all books
  scripts/parity_diff.py --book 1984              # Details for one book
  scripts/parity_diff.py --book 1984 --diff       # Full text diffs
  scripts/parity_diff.py --book 1984 --images     # Image metadata comparison
  scripts/parity_diff.py --metric                 # Machine-readable output
  scripts/parity_diff.py --all --diff             # Full diffs for every book
  scripts/parity_diff.py --book 1984 --css        # CSS property-level diff
""",
    )
    parser.add_argument("--book", help="Single book to diff (e.g. '1984', 'martyr')")
    parser.add_argument("--all", action="store_true", help="Show details for every book (not just summary)")
    parser.add_argument("--diff", action="store_true", help="Show full unified text diffs for structural files")
    parser.add_argument("--images", action="store_true", help="Show image metadata comparison")
    parser.add_argument("--css", action="store_true", help="Show CSS property-level diff (implies --diff for CSS files)")
    parser.add_argument("--metric", action="store_true", help="Machine-readable METRIC lines (for autoresearch)")
    parser.add_argument("--keep", action="store_true", help="Keep extracted EPUBs in /tmp for inspection")
    parser.add_argument("--skip-convert", action="store_true", help="Skip Go conversion, use existing EPUB at /tmp/<book>_go.epub")
    args = parser.parse_args()

    # Determine which books to process
    if args.book:
        if args.book not in BOOKS:
            print(f"Unknown book '{args.book}'. Available: {', '.join(BOOKS)}", file=sys.stderr)
            sys.exit(2)
        selected = [args.book]
    else:
        selected = BOOKS

    # Mode
    verbose = args.book is not None or args.all
    show_diff = args.diff or args.css
    show_images = args.images
    show_css = args.css
    metric_mode = args.metric

    # Keep temp files if requested
    tmpdir_obj = None
    if args.keep:
        tmpdir = tempfile.mkdtemp(prefix="parity_diff_")
        print(f"Keeping output in: {tmpdir}", file=sys.stderr)
    else:
        tmpdir_obj = tempfile.TemporaryDirectory(prefix="parity_diff_")
        tmpdir = tmpdir_obj.name

    total_structural = 0
    total_image = 0
    total_timestamp = 0
    results = {}

    for book in selected:
        book_dir = os.path.join(BOOKS_DIR, book)
        cal_epub = os.path.join(book_dir, "calibre.epub")
        input_kfx = find_input(book_dir)

        if not cal_epub or not os.path.exists(cal_epub):
            if metric_mode:
                print(f"METRIC {book}=SKIP", file=sys.stderr)
            else:
                print(f"  {book}: SKIP (no calibre.epub)")
            continue

        if not input_kfx:
            if metric_mode:
                print(f"METRIC {book}=SKIP", file=sys.stderr)
            else:
                print(f"  {book}: SKIP (no input KFX)")
            continue

        # Convert
        go_epub = os.path.join(tmpdir, f"{book}_go.epub")
        if not args.skip_convert:
            if verbose and not metric_mode:
                print(f"Converting {book}...", file=sys.stderr)
            ok, stderr = convert_book(input_kfx, go_epub)
            if not ok:
                if metric_mode:
                    print(f"METRIC {book}=FAIL", file=sys.stderr)
                else:
                    print(f"  {book}: CONVERSION FAILED")
                    if stderr:
                        for line in stderr.strip().split("\n")[-5:]:
                            print(f"    {line}", file=sys.stderr)
                continue
        elif not os.path.exists(go_epub):
            print(f"  {book}: --skip-convert but no EPUB at {go_epub}", file=sys.stderr)
            continue

        # Diff
        diffs = diff_epubs(cal_epub, go_epub)

        structural = [d for d in diffs if d.category == "structural" and d.kind == "content_diff"]
        timestamp_only = [d for d in diffs if d.kind == "timestamp_only"]
        image_diffs = [d for d in diffs if d.category == "image"]
        missing = [d for d in diffs if d.kind == "missing_in_go"]
        extra = [d for d in diffs if d.kind == "extra_in_go"]

        n_structural = len(structural) + len(missing) + len(extra)
        n_image = len(image_diffs)
        results[book] = (n_structural, n_image)
        total_structural += n_structural
        total_image += n_image
        total_timestamp += len(timestamp_only)

        if metric_mode:
            continue

        # Report
        if verbose:
            if n_structural == 0 and n_image == 0:
                status = "✓ perfect"
            elif n_structural == 0:
                status = "~ structurally clean"
            else:
                status = f"✗ divergent ({n_structural} structural, {n_image} image)"
            print(f"\n{'=' * 60}")
            print(f"  {book}: {status}")
            print(f"{'=' * 60}")

            if timestamp_only:
                print(f"  Timestamp-only (ignored): {', '.join(d.fname for d in timestamp_only)}")

            if missing:
                print(f"\n  Missing from Go:")
                for d in missing:
                    print(f"    {d.fname}")

            if extra:
                print(f"\n  Extra in Go:")
                for d in extra:
                    print(f"    {d.fname}")

            if structural:
                print(f"\n  Structural diffs ({len(structural)}):")
                for d in structural:
                    if show_diff:
                        print()
                        print_text_diff(d)
                    else:
                        print(f"    {d.fname}")

            if image_diffs:
                if show_images:
                    print(f"\n  Image diffs ({len(image_diffs)}):")
                    for d in image_diffs:
                        print_image_diff(d)
                elif verbose:
                    print(f"\n  Image diffs: {len(image_diffs)} files (use --images to see details)")

            if show_css:
                css_diffs = [d for d in diffs if d.category == "structural"
                             and d.fname.endswith(".css")]
                if css_diffs:
                    print(f"\n  CSS property diff:")
                    for d in css_diffs:
                        print_css_diff(d)
        else:
            # Summary line
            if n_structural == 0 and n_image == 0:
                print(f"  {book}: perfect")
            elif n_structural == 0:
                print(f"  {book}: structurally clean ({n_image} image)")
            else:
                print(f"  {book}: divergent ({n_structural} structural, {n_image} image)")

    # Metric output
    if metric_mode:
        total = total_structural + total_image
        n_perfect = sum(1 for s, i in results.values() if s == 0 and i == 0)
        n_clean = sum(1 for s, i in results.values() if s == 0) - n_perfect
        n_divergent = sum(1 for s, i in results.values() if s > 0)
        print(f"METRIC structural_diffs={total_structural}")
        print(f"METRIC image_diffs={total_image}")
        print(f"METRIC total_diffs={total}")
        print(f"METRIC books_perfect={n_perfect}")
        print(f"METRIC books_structurally_clean={n_clean}")
        print(f"METRIC books_divergent={n_divergent}")
        print(f"METRIC books_total={len(results)}")
        for book, (s, i) in sorted(results.items()):
            print(f"METRIC {book}={s}+{i}")
        return

    # Final summary
    if not verbose or args.all:
        n_perfect = sum(1 for s, i in results.values() if s == 0 and i == 0)
        n_clean = sum(1 for s, i in results.values() if s == 0) - n_perfect
        n_divergent = sum(1 for s, i in results.values() if s > 0)
        print(f"\n{'=' * 60}")
        print(f"  SUMMARY: {total_structural} structural, {total_image} image diffs")
        print(f"  {n_perfect} perfect · {n_clean} structurally clean · {n_divergent} divergent")
        if total_timestamp:
            print(f"  {total_timestamp} timestamp-only diffs (ignored)")
        print(f"{'=' * 60}")

    # Exit code: 0 if perfect, 1 if structurally clean, 2 if divergent
    if total_structural > 0:
        sys.exit(2)
    if total_image > 0:
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()
