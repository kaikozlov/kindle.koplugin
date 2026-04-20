# CSS Class Parity Plan

## Current Status (2026-04-20, session 3 — honest accounting)

### Text/CSS/Structure Parity (excluding image binaries)

| Book | Match | Differ | Status |
|------|-------|--------|--------|
| **Martyr** | 102 | 0 | ✅ Perfect |
| **Three Below** | 23 | 0 | ✅ Perfect |
| **Familiars** | 54 | 0 | ✅ Perfect |
| **Throne of Glass** | 71 | 0 | ✅ Perfect |
| **Elvis** | 30 | 2 | Table cell `<p>` wrappers |
| **Hunger Games** | 106 | 6 | Image variants, link wrappers, structural |

**Text total: 386 match, 8 differ**

### Full Parity (including image binary content)

| Book | Match | Differ | Notes |
|------|-------|--------|-------|
| Martyr | 110 | 5 | Cover identical, 5 JXR images pixel-different |
| Three Below | 24 | 16 | Cover identical, 16 JXR images pixel-different |
| Elvis | 31 | 32 | Cover identical, 30 JXR + 1 text diff |
| Familiars | 55 | 28 | Cover identical, 28 JXR images pixel-different |
| Hunger Games | 107 | 52 | Variant selection + JXR decode + structural |
| TOG | 72 | 7 | Cover identical, 7 JXR images pixel-different |

**Full total: 399 match, 140 differ** (138 are JXR image decode differences)

---

## JXR Image Decode Accuracy

### The Problem

Our `internal/jxr/` gray decoder produces pixels that differ from Python's libjxr→Pillow pipeline:
- 0.3% to 1.2% of pixels differ per image
- Max difference: 3-4 gray levels (out of 255)
- Average difference: ~1.1
- Visually imperceptible but breaks byte-level parity

### Root Cause

Likely rounding/quantization differences in:
1. Inverse discrete transform (first/second level)
2. Quantization table mapping (`quantMap` with `notScaledShift`)
3. Coefficient dequantization

### Python Pipeline (source of truth)

`resources.py:269-288`:
```python
def convert_jxr_to_jpeg_or_png(jxr_data, resource_name, return_mime=False):
    image_data = convert_jxr_to_tiff(jxr_data, resource_name)  # libjxr → TIFF
    img = Image.open(io.BytesIO(image_data))                    # Pillow → pixels
    img.save(outfile, "JPEG", quality=95, optimize=True)        # Pillow → JPEG
```

Three steps: libjxr decode → TIFF → Pillow re-encode. Our Go path: our decoder → Go stdlib JPEG (no Huffman optimize).

### What Needs Fixing

1. **JXR decode accuracy**: Audit `internal/jxr/decode.go` arithmetic against JPEG XR spec. Each macroblock coefficient processing step needs exact-match rounding.
2. **JPEG encoding**: Go's `image/jpeg` doesn't support Huffman optimization (`optimize=True`). Would need a third-party JPEG encoder or custom Huffman table generation to match Pillow's output.

### Impact

- **138 images** across all 6 books
- Non-JXR images (covers, some HG images) pass through correctly
- This is the largest remaining gap by file count

---

## Session 3 Architectural Fixes

### 1. JXR `Scaled=0` Support

`internal/jxr/jxr.go`: `SupportsFixtureGraySubset()` now accepts `Scaled <= 1` (was `== 1`). The `rsrc43M` image (1428×3 decoration strip) uses unscaled quantization.

### 2. ResourceResolver for background-image

Added `ResourceResolver func(symbol string) string` — resolves KFX resource symbols (`$479`/`$528`/`$175`) to CSS URL paths.

Threaded through: `processContentProperties` → `convertYJProperties` → `propertyValue` (and recursive struct/list helpers).

Python reference: `yj_to_epub_properties.py:1272-1273` — `self.process_external_resource(yj_value).filename`

Fixes `background-image: eAV` → `url(image_rsrc43N.jpg)` and prevents resource from being pruned.

### 3. Figure Margin Defaults

`comparisonInherited` now includes `margin-top: 1em` for elements with `tagChangedToFigure` OR `elem.Tag == "figure"`. Previously pre-converted figures (converted during rendering, not in simplify_styles) missed this.

---

## Remaining Non-Image Defects (8 files)

### D-HG: Hunger Games (6 text diffs)

1. **Image variant selection ($635)**: `buildResources` doesn't process `$635` variants. Python replaces with highest-resolution variant. Different resource IDs and pixel content.
2. **Missing `<a>` link wrappers**: Images not wrapped in `<a href="...">`.
3. **`<col>` attribute ordering**: `class` before `span` vs `span` before `class`.
4. **`colspan` on `<td>`**: Missing in Go output.
5. **`<div>` vs `<p>` structure**: c791.xhtml structural difference.
6. **Body class mismatch**: c791.xhtml different class assignment.

### D2: Elvis table cell wrappers (2 text diffs)

Go leaves bare text in `<td>`, Python wraps in `<p class="class_tableright">`.
Python reference: `yj_to_epub_content.py:816-830`.

---

## Next Steps (Priority Order)

1. **JXR decoder accuracy** — 138 images, biggest gap. Audit decode arithmetic vs spec.
2. **JPEG Huffman optimization** — Need `optimize=True` equivalent for byte parity.
3. **Hunger Games variant selection** — Wire `$635` into `buildResources`.
4. **Elvis table `<p>` wrappers** — Trace Python's table cell rendering.
5. **Hunger Games link wrappers** — Trace anchor/hyperlink handling for images.
