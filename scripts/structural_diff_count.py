#!/usr/bin/env python3
"""Count structural diffs between Go and Calibre reference EPUBs.

Usage: python3 scripts/structural_diff_count.py [--metric]
"""
import os, sys, subprocess, zipfile, hashlib, re, json, tempfile

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BOOKS_DIR = os.path.join(REPO_ROOT, "REFERENCE", "books")

BOOKS = [
    "martyr", "1984", "elvis", "familiars", "heated_rivalry",
    "hunger_games", "secrets_crown", "sunrise_reaping", "three_below", "throne_of_glass",
]

# File types that are "structural" (text-based)
STRUCTURAL_EXTS = {".html", ".xhtml", ".css", ".opf", ".ncx", ".xml"}
# Image types
IMAGE_EXTS = {".jpg", ".jpeg", ".png", ".gif", ".svg", ".jxr"}

# Ignore timestamp-only diffs in content.opf
MODIFIED_RE = re.compile(r'<meta property="dcterms:modified">.*?</meta>', re.DOTALL)

def convert_book(input_path, output_path):
    """Run Go conversion."""
    result = subprocess.run(
        ["go", "run", "./cmd/kindle-helper", "convert",
         "-input", input_path, "-output", output_path],
        capture_output=True, text=True, timeout=120,
        cwd=REPO_ROOT,
    )
    return result.returncode == 0

def count_diffs(cal_epub, go_epub):
    """Count structural and image diffs between two EPUBs."""
    structural = 0
    image = 0
    details = []

    with zipfile.ZipFile(cal_epub) as cal_zf, zipfile.ZipFile(go_epub) as go_zf:
        cal_files = {f for f in cal_zf.namelist() if not f.endswith("/")}
        go_files = {f for f in go_zf.namelist() if not f.endswith("/")}

        # Check all files in either EPUB
        all_files = sorted(cal_files | go_files)

        for fname in all_files:
            ext = os.path.splitext(fname)[1].lower()

            in_cal = fname in cal_files
            in_go = fname in go_files

            if not in_cal:
                # Extra in Go
                if ext in STRUCTURAL_EXTS:
                    structural += 1
                    details.append(("extra_in_go", fname))
                elif ext in IMAGE_EXTS:
                    image += 1
                    details.append(("extra_in_go", fname))
                continue

            if not in_go:
                # Missing from Go
                if ext in STRUCTURAL_EXTS:
                    structural += 1
                    details.append(("missing_in_go", fname))
                elif ext in IMAGE_EXTS:
                    image += 1
                    details.append(("missing_in_go", fname))
                continue

            # Both exist — compare content
            cal_data = cal_zf.read(fname)
            go_data = go_zf.read(fname)

            if cal_data == go_data:
                continue

            # Identical?
            if ext in IMAGE_EXTS:
                image += 1
                details.append(("image_diff", fname))
                continue

            # Text file — check if only timestamp differs
            if ext in STRUCTURAL_EXTS:
                try:
                    cal_text = cal_data.decode("utf-8", errors="replace")
                    go_text = go_data.decode("utf-8", errors="replace")
                except:
                    structural += 1
                    details.append(("binary_diff", fname))
                    continue

                # Ignore dcterms:modified timestamp in content.opf
                if fname.endswith("content.opf"):
                    cal_norm = MODIFIED_RE.sub("", cal_text).strip()
                    go_norm = MODIFIED_RE.sub("", go_text).strip()
                    if cal_norm == go_norm:
                        continue

                structural += 1
                details.append(("text_diff", fname))

    return structural, image, details

def main():
    metric_mode = "--metric" in sys.argv
    total_structural = 0
    total_image = 0
    results = {}

    with tempfile.TemporaryDirectory() as tmpdir:
        for book in BOOKS:
            book_dir = os.path.join(BOOKS_DIR, book)
            cal_epub = os.path.join(book_dir, "calibre.epub")

            # Find input
            input_kfx = os.path.join(book_dir, "input.kfx-zip")
            if not os.path.exists(input_kfx):
                input_kfx = os.path.join(book_dir, "input.kfx")

            if not os.path.exists(cal_epub) or not os.path.exists(input_kfx):
                continue

            go_epub = os.path.join(tmpdir, f"{book}_go.epub")

            if not convert_book(input_kfx, go_epub):
                if metric_mode:
                    print(f"CONVERSION_FAILED {book}", file=sys.stderr)
                else:
                    print(f"  {book}: CONVERSION FAILED")
                continue

            structural, image, details = count_diffs(cal_epub, go_epub)
            results[book] = (structural, image)
            total_structural += structural
            total_image += image

            if not metric_mode:
                status = "PERFECT" if structural == 0 and image == 0 else f"{structural} structural, {image} image"
                print(f"  {book}: {status}")

    if metric_mode:
        print(f"METRIC structural_diffs={total_structural}")
        print(f"METRIC image_diffs={total_image}")
        print(f"METRIC total_diffs={total_structural + total_image}")
        print(f"METRIC books_perfect={sum(1 for s, i in results.values() if s == 0 and i == 0)}")
        print(f"METRIC books_total={len(results)}")
        # Print per-book details
        for book, (s, i) in sorted(results.items()):
            print(f"METRIC {book}={s}+{i}")
    else:
        print(f"\nTOTAL: {total_structural} structural, {total_image} image across {len(results)} books")
        perfect = sum(1 for s, i in results.values() if s == 0 and i == 0)
        print(f"Perfect books: {perfect}/{len(results)}")

if __name__ == "__main__":
    main()
