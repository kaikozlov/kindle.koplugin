# CSS Class Parity Plan

## Current Status (2026-04-20)

| Book | Match | Differ | Summary |
|------|-------|--------|---------|
| **Martyr** | 102 | 0 | ✅ Perfect match |
| **Three Below** | 23 | 0 | ✅ Perfect match |
| **Hunger Games** | 105 | 7 | Composite styles, text-indent |
| **Throne of Glass** | 67 | 4 | JXR images (2), figure margins (2) |
| **Elvis** | 30 | 2 | Missing `<p class="tableright">` wrappers |
| **Familiars** | 49 | 5 | Font-weight class splitting |

**Total: 341 match, 25 differ** (up from 258 match / 105 differ at start)

### Commits Made
1. `6fb90c0` — Fix trailing newline in content.opf (D12) ✅
2. `2f9d6ec` — Enable reverse inheritance for text-indent and body style parity (D1) ✅ **Hunger Games 25→105**
3. `674230c` — Resolve link color for heading `<a>` children (D4) ✅
4. `727ebe8` — Rename `class_` to `figure_` for figure elements (D8) ✅ **Throne of Glass 64→67**
5. `92a8532` — Strip vendor-prefixed alternate equivalents in simplify_styles ✅
6. `2ac1cb2` — Fix body style inheritance for inferred sections (D1-BODY) ✅
7. `aa39fdf` — Strip vendor-prefixed alternates in reverse inheritance ✅

---

## Architecture: Body Style Inheritance (SOLVED)

### The Problem

Go's rendering pipeline (`inferBodyStyleValues`) synthesizes a full body style from children's KFX style fragments and puts it on the body element's inline `style` attribute. Python's body never has these synthesized properties — it only gets `set_html_defaults` properties (font-family, font-size, line-height, writing-mode). Everything else comes from reverse inheritance during `simplify_styles`.

**Why this matters**: When the body has `font-weight: bold` on its inline style, children inherit it via `parentStyle`. Headings with `font-weight: bold` from their style fragment match the inherited value → stripped by simplify_styles → never seen by reverse inheritance → body never promotes/strips it. In Python, headings keep `font-weight: bold` (doesn't match inherited `normal`), reverse inheritance sees it on ≥80% of children → promotes to body → removes from headings.

### The Fix (commit `2ac1cb2`)

Added `BodyStyleInferred` flag to `renderedStoryline`/`renderedSection`. Set when `inferBodyStyleValues()` is called (body has no style fragment ID and no style values). When `inferred=true`, `simplifyStylesFull` uses only `set_html_defaults` properties (font-family, font-size, line-height, writing-mode) on the body element, matching Python's behavior.

**Content-rendered sections** (promoted body containers like newspaper clippings with `styleID=sHA`) have `inferred=false` and keep their full style — matching Python where `process_content` creates the body with full style from `add_kfx_style`.

**Key files changed**:
- `internal/kfx/kfx.go` — Added `BodyStyleInferred` to `renderedStoryline` and `renderedSection`
- `internal/kfx/storyline.go` — Set flag when `inferBodyStyleValues` is called
- `internal/kfx/yj_to_epub_content.go` — Propagate flag from storyline to section
- `internal/kfx/yj_to_epub_properties.go` — Use minimal body style when `BodyStyleInferred=true`

### Python Evidence (confirmed by monkey-patching)

Python's body for TOG c1U section (heading_s20):
```
set_style #1: heading s20 fw='bold' (heading's own simplify_styles)
set_style #2: heading s20 fw='(not set)' (body's reverse inheritance, line 1915)
```
Reverse inheritance promoted `font-weight: bold` (4/5 children = 80%) and removed it from headings.

Python's body flow for INFERRED sections:
1. Body created by `process_content` with NO style from page template
2. `set_html_defaults` adds font-family, font-size, line-height, writing-mode
3. `simplify_styles` strips all defaults (match inherited)
4. Children processed with minimal `parentStyle`
5. Reverse inheritance promotes shared properties from children

Python's body flow for CONTENT-RENDERED sections (promoted body containers):
1. Top-level div gets style from `add_kfx_style` (KFX style fragment)
2. Div retagged to `<body>` — keeps full style
3. `set_html_defaults` adds font-family etc. if missing
4. `simplify_styles` processes body with full style — correct behavior

---

## Architecture: Vendor-Prefixed Alternates (SOLVED)

### The Problem

Go's rendering pipeline bakes `-webkit-hyphens: none` into heading style strings. When reverse inheritance strips `hyphens: none` from headings, the `-webkit-hyphens: none` survives because it's not a heritable property.

In Python, `add_composite_and_equivalent_styles` adds `-webkit-hyphens` AFTER simplify_styles. Since simplify_styles already stripped `hyphens: none` via reverse inheritance, there's nothing to add the prefix to.

### The Fix (commit `aa39fdf`)

In `applyReverseInheritance`, when removing a property from a child, also remove its vendor-prefixed alternate equivalent (looked up from `alternateEquivalentProperties`).

**File**: `internal/kfx/yj_to_epub_properties.go` in `applyReverseInheritance()`

---

## Remaining Defects (25 files across 4 books)

### D2: Missing `<p class="tableright">` wrappers in table cells
**Affected**: Elvis (2 files: c1L.xhtml + stylesheet.css)
**Root cause**: Python wraps `<td>` text content in `<p class="class_tableright">` elements via `renderTableNode`. Go leaves text bare in `<td>`.
**Python reference**: `yj_to_epub_content.py` lines 816-830 (table cell rendering)
**Go file**: `internal/kfx/storyline.go` — `renderTableNode` function

### D5: Font-weight class splitting
**Affected**: Familiars (5 files: stylesheet.css + 4 xhtml)
**Root cause**: Go's rendering pipeline (`paragraphClass`, `spanClass`) produces slightly different inline styles than Python's YJ→EPUB content conversion. When reverse inheritance runs on these different children, it produces different body/element classes. For example, Go might have `font-weight: 800` on elements where Python doesn't, or vice versa, causing class splitting.
**Investigation needed**: Trace specific Familiars sections where Go and Python produce different children styles — compare `paragraphClass` output vs Python's `process_content` output for the same KFX style fragment.

### D8: Figure non-heritable defaults
**Affected**: Throne of Glass (2 files: stylesheet.css + c4D.xhtml)
**Root cause**: Figure elements don't get non-heritable defaults (`margin-top: 0; margin-bottom: 0`) the same way Python does. Python's `simplify_styles` adds `non_heritable_default_properties` for divs (which includes margin: 0), and figures start as divs. Go adds these for `figure` tag but the figure in c4D doesn't get the margin class.
**Python reference**: `yj_to_epub_properties.py` line 1830 (`if elem.tag == "div": sty.update(self.non_heritable_default_properties, replace=False)`)
**Go file**: `internal/kfx/yj_to_epub_properties.go` — `simplifyStylesElementFull` non-heritable defaults block

### D9: JXR images
**Affected**: Throne of Glass (4 files: stylesheet.css, content.opf, c3UB.xhtml, image resource)
**Root cause**: JXR decoder exists in `internal/jxr/` but not wired into EPUB resource pipeline. Images referenced as `.jxr` files instead of converted to `.jpg`.
**Go files**: `internal/jxr/` (decoder), `internal/kfx/kfx.go` or `internal/kfx/yj_to_epub_content.go` (resource pipeline)

### D-HG: Hunger Games composite styles
**Affected**: Hunger Games (7 files)
**Root cause**: Multiple small differences in how composite styles, text-indent, and class variants are generated. Each needs individual investigation.
**Specific diffs**: class_s78C/s78E missing text-indent: 0; class_s78S split vs single; class_s78V missing text-align: center; class_s79F wrong properties; class_s790 missing text-indent

---

## Next Steps (Priority Order)

### 1. Investigate Familiars font-weight splitting (D5) — 5 files
Compare Go vs Python for the specific sections that produce `font-weight: 800` body classes. The rendering pipeline difference is the root cause — need to trace why Go's `paragraphClass` produces different styles than Python's content rendering for the same KFX style fragments.

### 2. Wire JXR decoder into EPUB pipeline (D9) — 4 TOG files
The `internal/jxr/` decoder exists. Need to:
1. Detect JXR image resources during conversion
2. Decode to JPEG
3. Replace `.jxr` references with `.jpg`
4. Update manifest entries in content.opf

### 3. Fix figure non-heritable defaults (D8) — 2 TOG files
Investigate why `figure` elements in TOG c4D don't get `margin: 0` defaults. Python's figures start as divs and get `non_heritable_default_properties` before being converted to `<figure>`. Go needs to match this.

### 4. Table `<p>` wrappers (D2) — 2 Elvis files
Python wraps table cell text in `<p>` elements. Requires changes to `renderTableNode` in `storyline.go`.

### 5. Hunger Games composite styles (D-HG) — 7 files
Individual investigation of each diff. May be partially resolved by other fixes.

---

## Key Files Reference

| Go File | Purpose |
|---------|---------|
| `internal/kfx/yj_to_epub_properties.go` | simplify_styles, reverse inheritance, body style handling |
| `internal/kfx/storyline.go` | Rendering pipeline: headingClass, paragraphClass, spanClass, renderTableNode |
| `internal/kfx/yj_property_info.go` | processContentProperties, cssDeclarationsFromMap |
| `internal/kfx/render.go` | Section rendering orchestration, pipeline phases |
| `internal/kfx/kfx.go` | Data types (renderedSection, decodedBook) |
| `internal/kfx/content_helpers.go` | filterBodyStyleValues, style merging helpers |
| `internal/kfx/yj_to_epub_content.go` | Section processing, body element creation |
| `internal/jxr/` | JPEG XR decoder (not yet wired) |

| Python File | Lines | Purpose |
|------------|-------|---------|
| `yj_to_epub_properties.py` | 1404 | `fixup_styles_and_classes` entry point |
| `yj_to_epub_properties.py` | 1652-1670 | `set_html_defaults` — adds font-family/size/line-height/writing-mode to body |
| `yj_to_epub_properties.py` | 1675-1968 | `simplify_styles` — recursive style simplification |
| `yj_to_epub_properties.py` | 1876-1920 | Reverse inheritance + children processing |
| `yj_to_epub_properties.py` | 1910-1915 | Reverse inheritance modifies children (line 1915: `self.set_style(child, child_sty)`) |
| `yj_to_epub_properties.py` | 1928-1968 | Heading conversion, non-heritable defaults, stripping loop |
| `yj_to_epub_properties.py` | 1973-2021 | `add_composite_and_equivalent_styles` — adds vendor prefixes AFTER simplify |
| `yj_to_epub_content.py` | 390-460 | `process_content` — creates styled elements, top-level → body |
| `yj_to_epub_content.py` | 816-830 | Table cell rendering (`<td>` text → `<p>` wrapping) |
| `yj_to_epub_content.py` | 1445-1460 | Top-level element retagged to `<body>` |

---

## Build & Test Commands

```bash
# Build
go build ./cmd/kindle-helper/

# Run tests
go test ./internal/kfx/ -count=1 -timeout 120s

# Convert all 6 books + compare against Calibre reference EPUBs
for f in REFERENCE/kfx_new/decrypted/*.kfx-zip; do
  name=$(basename "$f" .kfx-zip)
  ./kindle-helper convert --input "$f" --output "/tmp/cmp/${name}_go.epub" 2>/dev/null
done
./kindle-helper convert --input REFERENCE/kfx_examples/Martyr_*.kfx --output /tmp/cmp/martyr_go.epub 2>/dev/null

# Compare (normalizes timestamps, skips binary files)
for entry in \
  "martyr_go.epub:REFERENCE/martyr_calibre.epub:Martyr" \
  "Elvis and the Underdogs_B009NG3090_decrypted_go.epub:REFERENCE/kfx_new/calibre_epubs/Elvis and the Underdogs_B009NG3090_decrypted.epub:Elvis" \
  "The Familiars_B003VIWNQW_decrypted_go.epub:REFERENCE/kfx_new/calibre_epubs/The Familiars_B003VIWNQW_decrypted.epub:Familiars" \
  "Three Below (Floors #2)_B008PL1YQ0_decrypted_go.epub:REFERENCE/kfx_new/calibre_epubs/Three Below (Floors #2)_B008PL1YQ0_decrypted.epub:ThreeBelow" \
  "The Hunger Games Trilogy_B004XJRQUQ_decrypted_go.epub:REFERENCE/kfx_new/calibre_epubs/The Hunger Games Trilogy_B004XJRQUQ_decrypted.epub:HungerGames" \
  "Throne of Glass_B007N6JEII_decrypted_go.epub:REFERENCE/kfx_new/calibre_epubs/Throne of Glass_B007N6JEII_decrypted.epub:ThroneOfGlass"; do
  IFS=':' read -r go py label <<< "$entry"
  go_dir="/tmp/cmp_go_${label}"; py_dir="/tmp/cmp_py_${label}"
  rm -rf "$go_dir" "$py_dir"; mkdir -p "$go_dir" "$py_dir"
  unzip -q -o "/tmp/cmp/$go" -d "$go_dir" 2>/dev/null
  unzip -q -o "$py" -d "$py_dir" 2>/dev/null
  m=0; d=0
  while IFS= read -r f; do
    [ -z "$f" ] && continue; [ ! -f "$py_dir/$f" ] && continue
    go_n=$(sed 's/<meta property="dcterms:modified">[^<]*<\/meta>//' "$go_dir/$f")
    py_n=$(sed 's/<meta property="dcterms:modified">[^<]*<\/meta>//' "$py_dir/$f")
    if [ "$go_n" = "$py_n" ]; then m=$((m+1)); else d=$((d+1)); fi
  done < <(cd "$go_dir" && find . -type f -not -path './mimetype' | sed 's|^\./||' | sort | grep -vE '\.(jpg|jpeg|png|gif|svg|otf|ttf|woff2?)$')
  echo "SUM $label: $m match, $d differ"
  rm -rf "$go_dir" "$py_dir"
done

# Debug body style inference
KFX_DEBUG_BODY=1 ./kindle-helper convert --input <input> --output /tmp/debug.epub 2>&1 | grep 'body resolved'

# Debug Python body styles (monkey-patching)
python3 << 'PYEOF'
import sys, os
sys.path.insert(0, 'REFERENCE/Calibre_KFX_Input')
os.chdir('REFERENCE/Calibre_KFX_Input')
from kfxlib import YJ_Book
from kfxlib import yj_to_epub_properties
orig_set_style = yj_to_epub_properties.KFX_EPUB_Properties.set_style
def patched(self, elem, sty):
    if hasattr(elem, 'tag') and elem.tag == 'body':
        print(f'body set_style fw={sty.get("font-weight","NONE")} ta={sty.get("text-align","NONE")} keys={sorted(sty.keys())}')
    orig_set_style(self, elem, sty)
yj_to_epub_properties.KFX_EPUB_Properties.set_style = patched
book = YJ_Book('../kfx_new/decrypted/Throne of Glass_B007N6JEII_decrypted.kfx-zip')
book.decode_book(retain_yj_locals=True)
epub_data = book.convert_to_epub(epub2_desired=False)
PYEOF
```
