# CSS Class Parity Plan

## Current Status (2026-04-20, session 2)

| Book | Match | Differ | Status |
|------|-------|--------|--------|
| **Martyr** | 102 | 0 | ✅ Perfect (byte-identical) |
| **Three Below** | 23 | 0 | ✅ Perfect |
| **Familiars** | 54 | 0 | ✅ **Fixed this session** (was 49/5) |
| **Elvis** | 30 | 2 | Table cell `<p>` wrappers |
| **Hunger Games** | 106 | 6 | Resource mapping, structural |
| **Throne of Glass** | 68 | 3 | JXR color decode + background-image URL |

**Total: 381 match, 11 differ** (was 341/25 at start of session)

### Comparison Script
```bash
bash /tmp/compare_all.sh
```

---

## Session 2 Commits

1. `986d5a4` — WIP: wrap bare htmlText in spans in applyAnnotations
2. `6711811` — Fix text merging in appendTextHTMLParts + span wrapping in applyAnnotations
3. `7dccb59` — WIP: understanding page marker ordering issue
4. `73b6251` — Fix page marker regression + Familiars parity: 341/25 → 379/10
5. `48c9e57` — Fix figure margin defaults for pre-converted figures (D8)

---

## Session 2 Architectural Fixes

### D5: Font-weight class splitting (SOLVED)

**Root cause**: Go's `applyAnnotations` produced bare `htmlText` nodes between annotation ranges. Python's `add_content` (yj_to_epub_content.py:364-370) wraps ALL text in `<span>` elements. Without spans, `simplify_styles` reverse inheritance skipped elements with bare text (Python line 1875-1876: `if elem.text or elem.tail: skip`).

**Fix (3 parts)**:

1. **`appendTextHTMLParts` (html.go)** — Merge with last htmlText child instead of always appending. Without merging, the annotation event loop's character-by-character processing creates one `htmlText` per character.

2. **`applyAnnotations` (storyline.go)** — Wrap all bare `htmlText` children of root in `<span>` elements after the event loop. Matches Python's `add_content` which always wraps text in spans.

3. **`locateOffsetIn` (html.go)** — When finding a wrapper span (no attrs, tag=span) at offset 0 that contains text, split the text to create an empty span for the page marker. Python processes position anchors BEFORE annotations (yj_to_epub_content.py:1094-1100 runs before line 1118), so its `locate_offset` always finds raw text and splits cleanly. Go processes annotations first, so we must handle pre-wrapped text.

**Result**: Familiars 49/5 → 54/0. Martyr preserved at 102/0. Hunger Games improved by 1.

### D8: Figure margin defaults (SOLVED)

**Root cause**: Go's rendering pipeline converts divs to figures in `applyStructuralNodeAttrs` (storyline.go:1139) during rendering, BEFORE `simplifyStylesElementFull` runs. The `comparisonInherited` for margin stripping only checked `tagChangedToFigure || elem.Tag == "p"`, missing pre-converted figures (`elem.Tag == "figure"`). All margin properties were stripped, resulting in no class assignment.

**Fix**: Added `elem.Tag == "figure"` to the comparisonInherited condition in `simplifyStylesElementFull`.

**Result**: TOG c4D.xhtml now has `<figure class="figure_s4N">` with `margin-bottom: 0; margin-top: 0`.

---

## Remaining Defects (11 files across 3 books)

### D9: JXR color images — 3 TOG diffs

**Files**: c3UB.xhtml, stylesheet.css, content.opf

**What's wrong**:
- `image_rsrc43M.jxr` not converted to `.jpg` — JXR decoder only handles grayscale (`DecodeGray8`), not color
- `background-image: eAV` in stylesheet instead of `url(image_rsrc43N.jpg)` — resource URL not resolved for background images

**Python reference**: `resources.py:269-288` uses Pillow (`Image.open`) to convert JXR→TIFF→JPEG/PNG. Handles all color modes.

**Go files**:
- `internal/jxr/decode.go` — Only `DecodeGray8`, needs full color decode
- `internal/kfx/yj_to_epub_resources.go:1112` — `convertJXRToJpegOrPNG` falls back to raw data when decode fails

**Options**:
1. Implement full JXR color decode in Go (big lift — JPEG XR spec is complex)
2. Shell out to external tool (ImageMagick, etc.) at build time
3. Use a CGO wrapper around libjxr

### D2: Table `<p>` wrappers — 2 Elvis diffs

**Files**: section-0_1_d4cc20d3c8b59e6c_1ad1.xhtml, stylesheet.css

**What's wrong**:
- Go: `<td class="class_tableleft-0">2012028329</td>`
- Python: `<td class="class_tableleft-0"><p class="class_tableright">2012028329</p></td>`
- Also: logo div split into separate div+text instead of single `<p>`

**Python reference**: `yj_to_epub_content.py:816-830` — table cell rendering wraps text in `<p>`

**Go file**: `internal/kfx/storyline.go` — `renderTableNode`

### D-HG: Hunger Games resource/structural — 6 diffs

**Files**: c76J.xhtml, c791.xhtml, content.opf, stylesheet.css + image files

**What's wrong** (multiple distinct issues):
1. **Different image resource IDs**: Go uses rsrc7G7, Python uses rsrc7G8 — resource mapping/decryption difference
2. **Missing `<a>` link wrappers**: Python wraps images in `<a href="https://itunes.apple.com/...">`, Go doesn't
3. **`colspan` attribute ordering**: Go has `<col class="class-3" span="2"/>`, Python has `<col span="2" class="class-3"/>`
4. **`div` vs `p` structure**: c791.xhtml — Go creates separate divs for images, Python uses single `<p>`
5. **Different body class**: Go has `class="class_s79F"`, Python has `class="class-1"`

**Investigation needed**: Each diff is distinct. The image resource ID difference is likely a resource pipeline bug (decryption or variant selection). The link wrapping is a separate feature gap.

---

## Session 1 Commits (for reference)

1. `6fb90c0` — Fix trailing newline in content.opf (D12)
2. `2f9d6ec` — Enable reverse inheritance for text-indent and body style parity (D1)
3. `674230c` — Resolve link color for heading `<a>` children (D4)
4. `727ebe8` — Rename `class_` to `figure_` for figure elements (D8 naming)
5. `92a8532` — Strip vendor-prefixed alternate equivalents in simplify_styles
6. `2ac1cb2` — Fix body style inheritance for inferred sections (D1-BODY)
7. `aa39fdf` — Strip vendor-prefixed alternates in reverse inheritance

---

## Key Architecture Docs

### Body Style Inference (commit `2ac1cb2`)

`BodyStyleInferred` flag on `renderedSection`/`renderedStoryline`. When body style was INFERRED from children (not content-rendered), `simplifyStylesFull` uses only `set_html_defaults` properties (font-family, font-size, line-height, writing-mode). Content-rendered sections (promoted body containers) keep full style.

### Vendor-Prefixed Alternates (commit `aa39fdf`)

`alternateEquivalentProperties` map in `applyReverseInheritance`. When reverse inheritance removes a property from children, also remove its vendor-prefixed alternate (e.g., `hyphens` → `-webkit-hyphens`).

### Span Wrapping (commit `73b6251`)

Three-part fix matching Python's text handling:
1. `appendTextHTMLParts` merges consecutive text instead of per-char nodes
2. `applyAnnotations` wraps bare htmlText in `<span>` after event loop
3. `locateOffsetIn` splits wrapper spans for page markers (Python processes position anchors BEFORE annotations)

---

## Key Files Reference

| Go File | Purpose |
|---------|---------|
| `internal/kfx/yj_to_epub_properties.go` | simplify_styles, reverse inheritance, body style, figure defaults |
| `internal/kfx/storyline.go` | Rendering: applyAnnotations, renderTextNode, renderTableNode |
| `internal/kfx/html.go` | locateOffsetIn, appendTextHTMLParts, HTML tree utilities |
| `internal/kfx/render.go` | Section rendering orchestration, cleanupRenderedSections |
| `internal/kfx/kfx.go` | Data types (renderedSection, BodyStyleInferred) |
| `internal/kfx/yj_to_epub_resources.go` | JXR conversion, resource pipeline |
| `internal/jxr/decode.go` | JPEG XR decoder (grayscale only) |

| Python File | Lines | Purpose |
|------------|-------|---------|
| `yj_to_epub_properties.py` | 1675-1968 | `simplify_styles` — recursive style simplification |
| `yj_to_epub_properties.py` | 1876-1920 | Reverse inheritance |
| `yj_to_epub_properties.py` | 1934-1940 | Figure conversion + inherited margin defaults |
| `yj_to_epub_content.py` | 364-370 | `add_content` — wraps ALL text in `<span>` |
| `yj_to_epub_content.py` | 1094-1100 | Position anchors (BEFORE annotations) |
| `yj_to_epub_content.py` | 1118+ | Style events / annotations (AFTER position anchors) |
| `yj_to_epub_content.py` | 1555-1670 | `locate_offset` / `locate_offset_in` — zero_len splitting |
| `yj_to_epub_content.py` | 1657-1670 | `split_span` — text splitting for annotations/anchors |
| `epub_output.py` | 780-789 | `beautify_html` — strips attribute-less spans |
| `resources.py` | 269-288 | `convert_jxr_to_jpeg_or_png` — full color JXR decode |

---

## Next Steps (Priority Order)

### 1. JXR color decoder (D9) — 3 TOG diffs
Biggest single win. Options: full Go implementation, external tool, or CGO wrapper.
Also need to fix `background-image` URL resolution (raw symbol → resolved URL).

### 2. Hunger Games investigation (D-HG) — 6 diffs
Mixed bag of resource mapping, link wrapping, and structural issues.
Start with the image resource ID difference — if it's one bug causing multiple symptoms, could fix several at once.

### 3. Table `<p>` wrappers (D2) — 2 Elvis diffs
Python wraps table cell text in `<p>`. Requires tracing Python's table cell rendering and matching in Go's `renderTableNode`.
