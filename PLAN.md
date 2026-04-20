# CSS Class Parity Plan

## Current Status (2026-04-20, session 3)

| Book | Match | Differ | Status |
|------|-------|--------|--------|
| **Martyr** | 102 | 0 | ✅ Perfect |
| **Three Below** | 23 | 0 | ✅ Perfect |
| **Familiars** | 54 | 0 | ✅ Perfect |
| **Throne of Glass** | 71 | 0 | ✅ **Fixed this session** (was 68/3) |
| **Elvis** | 30 | 2 | Table cell `<p>` wrappers |
| **Hunger Games** | 106 | 6 | Image variants, link wrappers, structural |

**Total: 386 match, 8 differ** (was 381/11)

Note: TOG went from 68 match to 71 match (+3 new files matched, image files now match)
Total files differ: 8 (across 2 books)

---

## Session 3 Commits

1. `48c9e57` — Fix figure margin defaults for pre-converted figures (D8)
2. `09dbb61` — Fix TOG parity: JXR decode + background-image resource resolution (71/0!)

---

## Session 3 Fixes

### JXR Image Decode (D9 — SOLVED)

**Root cause (3 parts):**

1. **JXR `Scaled=0` rejected**: `SupportsFixtureGraySubset()` required `Scaled == 1`. The `rsrc43M` image (1428×3 decoration) uses `Scaled=0` (unscaled quantization). Fix: accept `Scaled <= 1`.

2. **`background-image: eAV` not resolved**: Python resolves `$479`/`$528` symbol values via `self.process_external_resource()` (yj_to_epub_properties.py:1272-1273). Go's `propertyValue` was a pure function with no resource access. Fix: added `ResourceResolver` type, threaded through `convertYJProperties` → `propertyValue` call chain. When `$479`/`$528`/`$175` has a symbol value, resolver maps it to `url(filename.jpg)`.

3. **Resource pruned**: `pruneUnusedResources` removed `image_rsrc43N.jpg` because nothing referenced it (the raw `eAV` symbol wasn't a valid `url()` reference). Fixed automatically once the symbol was resolved to a URL.

**Implementation:**
- `internal/jxr/jxr.go`: `SupportsFixtureGraySubset` accepts `Scaled <= 1`
- `internal/kfx/yj_property_info.go`: Added `ResourceResolver` type, threaded through all property conversion functions
- `internal/kfx/storyline.go`: All 18 `processContentProperties` calls pass `r.resolveResource`
- `internal/kfx/render.go`: Resolver created from `resourceHrefByID`

---

## Remaining Defects (8 files across 2 books)

### D-HG: Hunger Games (6 diffs)

**Files**: c76J.xhtml, c791.xhtml, content.opf (+ image binary differences)

**Root causes** (multiple distinct issues):

1. **Image variant selection ($635)**: Python's `get_external_resource` (yj_to_epub_resources.py:162-170) replaces images with higher-resolution variants from `$635`. Go's `buildResources` iterates resources but doesn't check variants. Result: Go uses lower-res `rsrc7G7` (132KB), Python uses `rsrc7G8` (247KB).

2. **Missing `<a>` link wrappers**: Python wraps images in `<a href="https://itunes.apple.com/...">`. These are hyperlinks from the book's marketing pages. Go renders bare `<img>` without links. Python source: `yj_to_epub_content.py` anchor/hyperlink handling.

3. **`<col>` attribute ordering**: Go: `<col class="class-3" span="2"/>`, Python: `<col span="2" class="class-3"/>`. HTML attribute order doesn't affect rendering but breaks comparison.

4. **`colspan` on `<td>`**: Go omits `colspan="2"` where Python includes it. Table cell spanning logic difference.

5. **`<div>` vs `<p>` structure**: c791.xhtml — Go: two `<div>`s with images, Python: one `<p>` with both images. Structural rendering difference.

6. **Body class mismatch**: c791.xhtml — Go: `class_s79F`, Python: `class-1`. Different class assignment.

**Fix approach**: Each is a distinct investigation. Image variant selection is the most impactful (affects all resource filenames and sizes).

### D2: Elvis table cell wrappers (2 diffs)

**Files**: section-0_1_d4cc20d3c8b59e6c_1ad1.xhtml, stylesheet.css

**Root cause**: Go leaves bare text in `<td>`, Python wraps in `<p class="class_tableright">`.
**Python reference**: `yj_to_epub_content.py:816-830` — table cell rendering.
**Go file**: `internal/kfx/storyline.go` — `renderTableNode`

---

## Key Architecture Docs

### ResourceResolver (Session 3)

`ResourceResolver func(symbol string) string` — resolves KFX resource symbols to CSS URL paths.

Threaded through: `processContentProperties` → `convertYJProperties` → `propertyValue` → struct/list helpers.

In `propertyValue`, for `$479`/`$528`/`$175` (background-image/resource name), calls `resolveResource(symbol)` → returns `"url(image_rsrc43N.jpg)"`.

Created on `storylineRenderer` from `resourceHrefByID` map. Available to all style conversion in rendering pipeline.

### Image Variant Selection ($635) — NOT YET IMPLEMENTED

Python (yj_to_epub_resources.py:162-170):
```python
for rr in resource.pop("$635", []):
    variant = self.get_external_resource(rr, ignore_variants=True)
    if (USE_HIGHEST_RESOLUTION_IMAGE_VARIANT and variant is not None and
            variant.width > resource_width and variant.height > resource_height):
        raw_media, filename, ... = variant.raw_media, variant.filename, ...
```

Go's `buildResources` doesn't process `$635` variants. The `resourceProcessor` has the logic but isn't used for EPUB output. Need to either:
1. Add variant processing to `buildResources`
2. Or integrate `resourceProcessor` into the main pipeline

---

## Next Steps

1. **Hunger Games image variant selection**: Add `$635` variant processing to `buildResources` or switch to `resourceProcessor`
2. **Hunger Games link wrappers**: Trace Python's hyperlink handling for images
3. **Elvis table `<p>` wrappers**: Trace Python's table cell rendering
