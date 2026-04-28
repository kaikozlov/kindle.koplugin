# Autoresearch: Python-to-Go 1:1 Parity

## Objective
Achieve structural parity between Go KFX→EPUB conversion pipeline and the Calibre Python reference implementation. The Python source at `REFERENCE/Calibre_KFX_Input/kfxlib/` is the sole source of truth. Every branch, edge case, and type dispatch in Python must have a Go counterpart.

## Metrics
- **Primary**: `structural_diffs` (count, lower is better) — total structural text/CSS/HTML diffs between Go output and Calibre reference EPUBs across all 10 test books
- **Secondary**: `image_diffs` — image encoding differences (expected due to JXR→JPEG re-encoding, but track for regressions)
- **Secondary**: `missing_files` — files present in reference but absent in Go output
- **Secondary**: `books_tested` — how many of the 10 books completed conversion

## How to Run
`./autoresearch.sh` — converts all 10 books with Go, compares against Calibre reference EPUBs, outputs `METRIC name=number` lines.

## Files in Scope
- `internal/kfx/*.go` — All Go conversion pipeline files (port of `REFERENCE/Calibre_KFX_Input/kfxlib/*.py`)
- Key files by priority (most diffs originate here):
  - `yj_to_epub_content.go` (7637 lines, 253 functions) — content rendering, section processing
  - `yj_to_epub_properties.go` (4719 lines, 112 functions) — CSS style handling, simplify styles
  - `yj_to_epub_misc.go` (1627 lines) — body classes, page naming
  - `yj_to_epub_navigation.go` (967 lines) — TOC/NAV generation
  - `yj_to_epub_resources.go` (1798 lines) — image/resource handling
  - `yj_to_epub.go` (734 lines) — top-level orchestration
  - `yj_book.go` (593 lines) — fragment organization
  - `yj_to_image_book.go` (1481 lines) — image book conversion

## Off Limits
- `REFERENCE/` — Python source is read-only, never modified
- `scripts/` — tooling is stable
- `lua/` — frontend is separate concern
- `internal/kfx/catalog.ion` — symbol catalog is golden data
- `internal/kfx/testdata/` — golden test files

## Constraints
- All Go tests must pass (`go test ./internal/kfx/...`)
- No new external dependencies
- Code must compile (obviously)
- Every change must map to specific Python source (file, line, branch)

## Reference Materials
- **Python source**: `REFERENCE/Calibre_KFX_Input/kfxlib/` — 37 Python files, the source of truth
- **Pytago output**: `REFERENCE/pytago_test_new/go_output/` — Automated Python→Go transpilation. Not idiomatic Go but shows every Python branch in Go-like syntax. Use as a reference for what's missing.
- **Test books**: `REFERENCE/books/<name>/` — 10 books (1 CONT + 9 DRMION), each with `input.kfx(-zip)` and `calibre.epub`
- **Audit tool**: `python scripts/audit_branches.py --file <file>.py --function <func>` — Lists every Python branch and checks for Go equivalent

## Known Parity Gaps (from AGENTS.md)

### 1984 (16 structural diffs)
1. Body class index offset — `estimateBodyClass()` assigns class-3 vs Calibre's class-4
2. Image page body class — `class_sN` vs `figure_sN-0/1/2`
3. Missing margins — `class_s1MY`, `class_s1N2`, `class_s1NM` missing `margin-bottom: 0; margin-top: 0`
4. Missing `heading_s1NG` class — Go keeps as `class_s1NG`, Calibre creates separate `heading_s1NG` rule
5. Table image `<div>` wrapper — extra `<div class="class_s1S6">` inside `<td>` around `<a><img>`
6. TOC `<span>` vs `<p>` — navigation child entries use `<span>` in Go, `<p>` in Calibre
7. Missing page anchor — `page_134` absent in Go
8. XML attribute ordering — `epub:type` and `class` attribute order swapped on `<a>`

### Secrets Crown (18 structural diffs)
1. HD variant selection — Go uses `-resized`, Calibre uses `-hd-resized`
2. CSS class index swap — `class_220-0`/`class_220-1` wrong order
3. Missing margins — `class_2326` missing `margin-bottom: 0; margin-top: 0`
4. Image page body class — extra `text-align: center`

## Approach Strategy

1. **Branch audit first**: Use `scripts/audit_branches.py` to identify Python branches missing from Go
2. **Pytago reference**: Compare pytago output against Go code to find entire functions/blocks that are missing
3. **Test-driven**: Fix one diff at a time, verify with `scripts/diff_kfx_parity.sh --book <name>`
4. **Focus on structural over cosmetic**: CSS class ordering and attribute ordering are lower priority than missing content or incorrect HTML structure

## What's Been Tried

### Pre-autoresearch (already committed before session):
- JXR image conversion MIME type
- `<a>` class on link-wrapped elements
- Image heading anchor suppression
- Sunrise Reaping (now structurally perfect)
- COMBINE_NESTED_DIVS implementation
- Table cell `<div>` wrapper unwrapping
- `<a>` attribute ordering fix

### Autoresearch Session (2026-04-28): 75→53 structural diffs (-29%)

1. **Fix `</body>` placement** (-4 diffs, then +7 regression, then -7 fix)
   - `internal/epub/epub.go`: Match lxml compact serialization for single self-closing
     elements (ending `/>`) and SVG cover pages. Non-self-closing elements keep
     standard newline before `</body>`.

2. **Fix JXR image conversion: MIME type** (-11 structural + 22 missing = -33 total)
   - `internal/kfx/yj_to_epub_resources.go`: `parseResourceFragment` set JXR mime
     to `image/jpeg` instead of `image/jxr` when the mime field was empty. Fixed
     to set correct MIME type per format (jxr→image/jxr, jpg→image/jpeg, png→image/png).

### Current Status: 53 structural diffs

**Per-book breakdown:**
- Martyr: 0 ✓ | 1984: 0 ✓ | SunriseReaping: 1 | ThreeBelow: 1
- Elvis: 5 | ThroneOfGlass: 5 | SecretsCrown: 4
- HeatedRivalry: 7 | Familiars: 13 | HungerGames: 17

**52 of 53 are body class split** — single root cause requiring architectural fix:
- Go's promoted body style includes `width:100%` (non-heritable) from container fragment
- Python's body gets `text-align:center` through reverse inheritance, NOT from container
- KFX data uses `box_align` (not `text_alignment`) for image containers
- `box_align` → `text-align` conversion happens during simplify_styles in Python
- Fix requires converting `box_align` to `text-align` for promoted body styles
- See `autoresearch.ideas.md` for detailed investigation
