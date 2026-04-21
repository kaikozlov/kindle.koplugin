# PLAN: Systematic Python→Go Parity for New Book Diffs

**Status**: In progress  
**Updated**: 2026-04-20  
**Goal**: All 10 test books produce text-identical output to Python Calibre reference  
**Approach**: Port Python logic 1:1 — file by file, function by function, branch by branch  
**Constraint**: Original 6 books must stay at 394/394 matching after every step

---

## Current Parity Status

### Original 6 books: **394/394 ✓**
| Book | Files | Match | Differ |
|------|-------|-------|--------|
| Martyr | 102 | 102 | 0 |
| Three Below | 23 | 23 | 0 |
| Elvis | 32 | 32 | 0 |
| Familiars | 54 | 54 | 0 |
| Hunger Games | 112 | 112 | 0 |
| TOG | 71 | 71 | 0 |

### New 4 books: **113 text files differ**
| Book | Match | Differ | Top Pattern |
|------|-------|--------|-------------|
| Heated Rivalry | 49 | 2 | Anchor merge, xml:lang |
| 1984 | 31 | 16 | Anchor merge, body class, epub:type |
| Sunrise on the Reaping | 15 | 33 | Image→h1 promotion (31 files!) |
| Secrets of the Crown | 1 | 4 | Structural differences |

---

## Architecture Gap: Rendering vs Simplify

**Root cause of most new-book diffs**: Go makes structural tag decisions during *rendering* (`storyline.go`) while Python defers them to `simplify_styles` (`yj_to_epub_properties.py`).

| Decision | Go (during rendering) | Python (during simplify_styles) |
|----------|----------------------|-------------------------------|
| `<div>` → `<h1>`-`<h6>` | `layoutHintHeadingTag()` in `storyline.go:1525` | `simplify_styles` line 1922: checks `kfx_layout_hints`, `contains_block_elem` |
| `<div>` → `<p>` | `renderInlineParagraphContainer()` in `storyline.go:1544` | `simplify_styles` line 1941: checks `contains_text and not contains_block_elem` |
| `<div>` → `<figure>` | `renderNode` layout hint check in `storyline.go:1463` | `simplify_styles` line 1934: checks `kfx_layout_hints == "figure" and contains_image` |

This means:
- Go decides tag type before seeing all children (only sees KFX node data)
- Python decides tag type after processing all children (sees full HTML subtree)
- Go can't handle cases that depend on the final content (e.g. image-only vs text headings)

---

## Step-by-Step Plan

### Step 1: Move heading/figure/paragraph decisions from rendering to simplify_styles
**Status**: ✅ Committed `81e7b34`
**Result**: Original 6 books: 394/394 ✓. Heading/paragraph conversion moved to simplify_styles.
**Lessons learned**: Figure conversion must stay in rendering (uses KFX node $761 property not available in CSS). Heading conversion in simplify_styles correctly keeps image-only headings as `<div>` (containsBlock=true from image wrapper `<div>`).  
**Files changed**: `storyline.go`, `yj_to_epub_properties.go`  
**Python reference**: `yj_to_epub_properties.py` lines 1858-1966  
**Estimated impact**: Fixes 31+ files in Sunrise Reaping, fixes cascading class index issues

**What to do**:

1. In `storyline.go`, change `layoutHintHeadingTag()` (line 1525) to return `""` — stop creating `<h1>`-`<h6>` during rendering. Instead, always create `<div>` with the heading class name (`heading_sXXX`). Keep the heading class name generation (`headingClass`) so the `-kfx-layout-hints: heading` metadata is embedded in the class string.

2. In `storyline.go`, change `renderInlineParagraphContainer()` (line 1544) to return `nil` — stop creating `<p>` during rendering. Instead, always create `<div>` with the paragraph class name.

3. In `storyline.go`, remove the figure conversion in `renderNode` (line 1463). Always create `<div>` with the figure class name.

4. In `yj_to_epub_properties.go` `simplifyStylesElementFull()` (line 1500), add the heading conversion logic. Port from Python `simplify_styles` lines 1858-1932:

```python
# Python line 1858
self.last_kfx_heading_level = kfx_heading_level = sty.pop("-kfx-heading-level", self.last_kfx_heading_level)

# Python line 1922
if elem.tag == "div" and not (self.fixed_layout or self.illustrated_layout or self.has_conditional_content):
    if "heading" in kfx_layout_hints and not contains_block_elem:
        elem.tag = "h" + kfx_heading_level
        inherited_properties.pop("font-size")
        inherited_properties.pop("font-weight")
        # ... margin popping
```

Go currently has the `<div>` → `<p>` conversion at line 1749 but is missing the heading conversion. Add heading conversion BEFORE the paragraph conversion (matching Python's order: heading first, then figure, then paragraph).

5. Track `lastKfxHeadingLevel` as state in `simplifyStylesFull` (or pass it through). Python uses `self.last_kfx_heading_level` which persists across calls. Go needs the same.

6. The `-kfx-heading-level` and `-kfx-layout-hints` come from the element's style. Go currently embeds these in the class name (`heading_sXXX` → meta `"-kfx-style-name": "sXXX"`, `layoutHints: ["heading"]`). The `parseStyleTokens` function in `yj_to_epub_properties.go` already extracts layout hints from the class string. Verify that `-kfx-heading-level` is available in `sty` for heading elements.

**Testing**: After this step, all original 6 books must still pass. Sunrise Reaping should go from 33 diffs to ~2 diffs.

**Commit message format**: `Refactor: move heading/figure/paragraph tag decisions from rendering to simplify_styles (Python parity)`

---

### Step 2: Add `xml:lang` attribute to body and html elements
**Status**: ✅ Committed `a5103d9`
**Result**: HR c540 fixed (50/1), SR +1 (16/32). Also added epub:type infrastructure.
**What was done**: Port Python `fixup_styles_and_classes` partition of `-kfx-attrib-*` CSS properties → element attributes. Normalize language tags via `fix_language`.  
**Python reference**: `yj_to_epub_properties.py` lines 1604-1650 (`update_default_font_and_language`)  
**Estimated impact**: Fixes 3+ files across HR, 1984, SunriseReaping

**What to do**:

1. In Python, `update_default_font_and_language` (line 1604) scans content elements to find the language and default font. It sets `xml:lang` on the `<html>` and `<body>` elements.

2. Check what Go does for language. The language comes from the KFX book metadata. Find where Python reads it (likely `$389` fragment or `$11` property).

3. In `yj_to_epub_properties.go` `simplifyStylesFull()` or in `renderHTMLSection`, add `xml:lang` to the `<html>` and `<body>` elements when a non-empty language is detected.

**Python code to trace**:
```python
# yj_to_epub_properties.py line 1604
def update_default_font_and_language(self):
    def scan_content(elem, font_family, lang):
        ...
```

**Testing**: Heated Rivalry c540 should match. All original 6 books unchanged.

---

### Step 3: Add `xmlns:epub` namespace when `epub:type` is used
**Status**: Pending  
**Python reference**: `epub_output.py` line ~500 (HTML head generation), `yj_to_epub_properties.py` line 665  
**Estimated impact**: Fixes 3 files in 1984

**What to do**:

1. Python adds `xmlns:epub="http://www.idpf.org/2007/ops"` to the `<html>` element when any element in the document uses `epub:type` attribute.

2. In Go's HTML serialization (likely `html.go`), check if any element has `epub:type` set. If so, add `xmlns:epub` to the `<html>` element.

3. This is related to Step 4 (epub:type noteref) — when noteref is added, the namespace will be needed.

**Testing**: 1984 c1J7 should get `xmlns:epub` added. All original 6 books unchanged.

---

### Step 4: Add `epub:type="noteref"` annotation on footnote links
**Status**: Pending  
**Python reference**: `yj_to_epub_content.py` lines 482-540 (annotation processing), `yj_to_epub_properties.py` line 665  
**Estimated impact**: Fixes 2+ files in 1984

**What to do**:

1. In Python, `process_content` handles `$683` (annotations) which include `$690` (footnote link type). When annotation type is `$690`, the `<a>` element gets `epub:type="noteref"`.

2. Python code (line 486-540):
```python
if "$683" in content:
    for annotation in content["$683"]:
        annotation_type = ...
        if annotation_type == "$690" and content_type == "$270":
            # footnote link
            ...
```

3. In Go's `renderNode` or `storyline.go`, find where annotations (`$683`) are processed. Add handling for `$690` to set `epub:type="noteref"` on the resulting `<a>` element.

4. Also handle `$749` (sidebar/popfoot reference) per Python line 521.

**Testing**: 1984 c1J7 footnote link should have `epub:type="noteref"`. All original 6 books unchanged.

---

### Step 5: Fix position anchor separation (empty `<span id="X"/>` vs merged)
**Status**: Pending  
**Python reference**: `yj_to_epub_content.py` lines 1841-1908 (`find_or_create_style_event_element`)  
**Estimated impact**: Fixes 2+ files in HR and 1984

**What to do**:

1. In Python, `find_or_create_style_event_element` (line 1841) creates style event elements. When the event is a zero-length position anchor (no text to wrap), it creates an empty `<span id="X"/>` element as a separate sibling before the text span.

2. Python code (line 1863):
```python
if event_length > 0 and not is_dropcap:
    # Find/create a <span> that covers the offset range
    ...
else:
    # Zero-length event: create empty anchor
    anchor = etree.SubElement(elem, "span")
    anchor.set("id", event_id)
```

3. In Go, position anchors are applied in `applyPositionAnchors` or `applyContainerStyleEvents`. When a position anchor falls between elements (not on an existing element), Go should insert an empty `<span id="X"/>` instead of merging the id into the next element.

4. The current Go behavior merges `<span id="page_285" class="class_s594">they</span>`. The correct behavior is `<span id="page_285"/><span class="class_s594">they</span>`.

**Testing**: HR c3VD should separate the anchor. 1984 c1J7 should separate page anchors. All original 6 books unchanged.

---

### Step 6: Add superscript/subscript rendering ($261)
**Status**: Pending  
**Python reference**: `yj_to_epub_content.py` line ~460 (`$261` handling within `$270` template)  
**Estimated impact**: Fixes 1 file in SunriseReaping, likely more in other books

**What to do**:

1. Python handles `$261` (superscript/subscript) content type. This wraps text in a `<span>` with `vertical-align: super` or `vertical-align: sub` styling.

2. Find the Python code that handles `$261`. It's likely in `process_content` or the `$270` (page template) rendering path.

3. In Go's `renderNode`, add a case for `$261` that creates a `<span>` with the appropriate vertical-align style.

**Testing**: SunriseReaping c33 "4th" → "4<span class='...'>th</span>". All original 6 books unchanged.

---

### Step 7: Fix body class index differences
**Status**: Pending  
**Python reference**: `yj_to_epub_properties.py` lines 1388-1593 (`fixup_styles_and_classes`, `inventory_style`)  
**Estimated impact**: Fixes 4 files with body class differences, likely cascading

**What to do**:

1. Body classes differ (e.g., `class-4` in Go vs `class-5` in Python). This indicates the style catalog (CSS class index) produces different ordering/deduplication.

2. Python's `fixup_styles_and_classes` (line 1388) and `inventory_style` (line 1594) manage the style catalog. Classes are assigned based on a hash of the style properties.

3. Go's style catalog is in `yj_to_epub_properties.go`. Compare the class assignment logic:
   - Python: `inventory_style` creates a sorted declaration string and uses it as a key
   - Go: `styleStringFromDeclarations` creates the string, catalog deduplicates

4. The body class index difference is likely because:
   - Go creates heading/paragraph/figure elements during rendering, which produces different class strings
   - After Step 1 (moving tag decisions to simplify_styles), this may auto-fix
   - If not, compare `inventory_style` logic 1:1

**Testing**: 1984 body classes should match Python. All original 6 books unchanged.

---

### Step 8: Investigate Secrets of the Crown structural differences
**Status**: Pending  
**Python reference**: `yj_to_epub_content.py` lines 115-345 (`process_section`), `yj_to_epub_illustrated_layout.py`  
**Estimated impact**: Fixes 4 files in SecretsCrown

**What to do**:

1. SecretsCrown has completely different filenames (Go: `1.xhtml`, Python: `UYqzWVgySW_Gl4WQ-Od_xQ1.xhtml`). This suggests Go is generating different section filenames.

2. Python uses hash-based filenames from the KFX content. Go may be using a different naming scheme.

3. The cover reference also differs (`resource_7-*.bin` vs `image_7-*.jpg`).

4. This book may be a children's book or use illustrated layout. Check if Go handles the `is_children` or `is_comic` code path from Python's `process_section` (line 150-155):
```python
elif self.is_comic or self.is_children:
    self.process_page_spread_page_template(...)
```

5. Also check `yj_to_epub_illustrated_layout.py` — Go has `yj_to_epub_illustrated_layout.go` but it may be incomplete.

**Testing**: SecretsCrown should produce matching filenames and structure.

---

### Step 9: Final validation and comparison script update
**Status**: Pending  
**What to do**:

1. Update `/tmp/compare_all.sh` (or create a permanent version) to include all 10 books.

2. Run full comparison across all 10 books. Target: 0 text diffs, 0 structural diffs.

3. Run `go test ./...` to ensure no regressions.

4. Test with both DRMION input (Go decrypts) and decrypted kfx-zip input.

---

## Python→Go File Mapping Reference

| Python File | Lines | Go File | Status |
|------------|-------|---------|--------|
| `yj_to_epub_content.py` | 1944 | `storyline.go` (2856), `content_helpers.go` (830), `yj_to_epub_content.go` (1353) | Partial — missing $261 superscript, annotation details |
| `yj_to_epub_properties.py` | 2486 | `yj_to_epub_properties.go` (2015) | Partial — heading conversion missing from simplify_styles |
| `yj_to_epub_resources.py` | 374 | `yj_to_epub_resources.go` (1267) | Good — recently fixed variant selection |
| `yj_to_epub_navigation.py` | 541 | `yj_to_epub_navigation.go` (544) | Good |
| `yj_to_epub_metadata.py` | 290 | `yj_to_epub_metadata.go` (332) | Good |
| `yj_to_epub_misc.py` | 611 | `yj_to_epub_misc.go` (271) | Needs audit |
| `yj_to_epub_notebook.py` | 703 | `yj_to_epub_notebook.go` (1419) | Needs audit |
| `epub_output.py` | 1504 | `kfx.go` (556), `render.go` (560) | Partial — missing xml:lang, epub namespace |
| `yj_structure.py` | 1313 | `yj_structure.go` (1577) | Good |
| `yj_position_location.py` | 1324 | `yj_position_location.go` (1886) | Good |
| `resources.py` | 804 | `yj_to_epub_resources.go` (tile/JXR logic) | Good — image diffs are encoder-only |
| `yj_versions.py` | 1124 | `yj_versions.go` (1060) | Good |
| `yj_to_epub_illustrated_layout.py` | 408 | `yj_to_epub_illustrated_layout.go` (590) | Needs audit |

---

## Key Python Functions to Port 1:1

### `simplify_styles` — `yj_to_epub_properties.py:1675`
This is THE most critical function. It handles:
- Unit conversion (lh→em, rem→em, vh/vw→%)
- Heading conversion: `div → h1-h6` (line 1922)
- Figure conversion: `div → figure` (line 1934)
- Paragraph conversion: `div → p` (line 1941)
- Reverse inheritance (line 1891)
- Style stripping against inherited (line 1958)
- Font-size normalization (line 1958)

Go's `simplifyStylesElementFull` (line 1500) covers most of this but is missing:
- Heading conversion (Go does it in rendering instead)
- `last_kfx_heading_level` state tracking
- Full `-kfx-heading-level` extraction from `sty`

### `process_content` — `yj_to_epub_content.py:390`
Handles all KFX content types ($269, $271, $270, etc.).
Key branches Go may be missing or handling differently:
- `$261` superscript/subscript (line ~460)
- `$683` annotations — `$690` noteref, `$749` sidebar (line 482-540)
- `$754` conditional content (line 479)
- `$591`/`$592` crop/bleed (line 592-1000)
- `$663` region magnification (line 617)

### `find_or_create_style_event_element` — `yj_to_epub_content.py:1841`
Creates style event elements (position anchors, link wrappers).
Go's version may differ in:
- Zero-length anchor handling (separate `<span id=""/>` vs merged)
- Element type dispatch (Python handles img, svg, div, a, aside, figure, h1-h6, li, ruby, rb)

### `update_default_font_and_language` — `yj_to_epub_properties.py:1604`
Sets `xml:lang` on html/body. Go doesn't do this at all.

### `set_html_defaults` — `yj_to_epub_properties.py:1652`
Sets default styles on body and html. Go partially does this.

---

## Test Command

After each step, run:

```bash
# Build and test
go build ./cmd/kindle-helper/ && go test ./internal/kfx/ -count=1 -timeout 120s

# Original 6 books (must be 394/394)
bash /tmp/compare_all.sh

# New 4 books
bash /tmp/compare_new.sh
```

---

## Changelog

| Date | Step | Commit | Result |
|------|------|--------|--------|
| 2026-04-20 | Audit | — | Identified 9 patterns, 113 diffs across 4 new books |
| 2026-04-20 | Step 0 | `030171c` | Original 6 books: 394/394 ✓ |
