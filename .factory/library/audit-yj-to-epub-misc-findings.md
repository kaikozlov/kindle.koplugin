# Audit: yj_to_epub_misc.py → yj_to_epub_misc.go + svg.go

**Python file:** `REFERENCE/Calibre_KFX_Input/kfxlib/yj_to_epub_misc.py` (611 lines, 15 functions)
**Go files:** `internal/kfx/yj_to_epub_misc.go` (271 lines) + `internal/kfx/svg.go` (279 lines)

## Function-by-Function Mapping

| # | Python Function | Lines | Go Counterpart | Status |
|---|----------------|-------|----------------|--------|
| 1 | `__init__` | 26-27 | N/A | ✅ Empty init, not needed |
| 2 | `set_condition_operators` | 29-66 | `yj_to_epub_misc.go`: dispatch table + `conditionOperatorArity` | ✅ Full parity |
| 3 | `evaluate_binary_condition` | 68-74 | `yj_to_epub_misc.go:evaluateBinary` | ✅ Full parity |
| 4 | `evaluate_condition` | 76-121 | `yj_to_epub_misc.go:evaluate` | ✅ Full parity |
| 5 | `add_svg_wrapper_to_block_image` | 123-195 | **MISSING** | ⚠️ Not needed for test books (FXL only) |
| 6 | `horizontal_fxl_block_images` | 197-233 | **MISSING** | ⚠️ Not needed for test books (FXL only) |
| 7 | `process_kvg_shape` | 235-286 | `svg.go:processKVGShape` | ⚠️ Partial — see gaps |
| 8 | `process_path` | 288-337 | `svg.go:processPath` | ⚠️ Partial — see gaps |
| 9 | `process_polygon` | 340-365 | `svg.go:processPolygon` | ✅ Full parity |
| 10 | `process_transform` | 367-407 | `svg.go:processTransform` | ✅ Full parity |
| 11 | `process_plugin` | 409-578 | `storyline.go:renderPluginNode` (partial) | ⚠️ Partial — see gaps |
| 12 | `process_plugin_uri` | 580-588 | **MISSING** | ⚠️ Only called from process_plugin |
| 13 | `process_bounds` | 590-604 | **MISSING** | ⚠️ Only called from process_plugin_uri |
| 14 | `px_to_int` | 606-607 | **MISSING** | ⚠️ Only called from add_svg_wrapper |
| 15 | `px_to_float` | 609-611 | **MISSING** | ⚠️ Only called from px_to_int |

## Fixes Applied (Commit 6ae9856)

### Fix 1: `$98` (transform) in propertyValueList
- **Python:** `yj_to_epub_properties.py:1345` calls `self.process_transform(yj_value, svg)`
- **Go was:** `fmt.Sprintf("%v", v)` — produced raw Go array format like `[1 0 0 1 10.5 20]`
- **Go now:** Calls `processTransform(v, false)` — produces proper CSS like `translate(10.5px,20px)`

### Fix 2: `$650` (polygon/shape-outside) in propertyValueList
- **Python:** `yj_to_epub_properties.py:1331-1332` calls `self.process_polygon(yj_value)`
- **Go was:** Fell through to default `fmt.Sprintf("%v", v)`
- **Go now:** Calls `processPolygon(v)` — produces proper CSS like `polygon(50% 0%, 100% 100%, 0% 100%)`

## Known Gaps (not affecting test books)

### processKVGShape `$270` branch (svg.go:30-42)
Python iterates `content_list` to find matching content by `$155`/`$598`, calls `process_content`, then renames the resulting element to SVG `<text>`. Go just creates an empty `<text>` element. This only affects KVG (Kindle Vector Graphics) content used in illustrated layout / children's books — none of the test books use this.

### processPath IonStruct path bundle lookup (svg.go:107-115)
Python looks up path bundles from `self.book_data["$692"]` and recursively processes. Go logs an error and returns empty string. This only affects KVG shapes with path bundles — none of the test books use this.

### processKVGShape missing `check_empty` (Python L285)
Python calls `self.check_empty(shape, "shape")` at the end to log any unexpected remaining keys. This is a debugging/validation function, not affecting output.

### Missing FXL/Plugin functions
- `add_svg_wrapper_to_block_image` — wraps block images in SVG containers for fixed-layout pages
- `horizontal_fxl_block_images` — positions images horizontally in FXL layouts
- `process_plugin` — handles 12+ plugin types (audio, button, hyperlink, image_sequence, scrollable, slideshow, video, webview, zoomable, etc.)
- `process_plugin_uri` / `process_bounds` — plugin sub-helpers
- `px_to_int` / `px_to_float` — pixel unit conversion helpers

These are only needed for fixed-layout books (print replica, illustrated layout, children's books) and plugin content. None of the current 6 test books are fixed-layout or contain plugin content. `storyline.go:renderPluginNode` handles the basic HTML/audio/video/image plugin cases needed for current test books.

## Test Results
- `go build ./cmd/kindle-helper/` ✅ Builds cleanly
- `go test ./internal/kfx/ -count=1 -timeout 120s` ✅ All tests pass
- `bash /tmp/compare_all.sh` ✅ 394/394 (102+23+32+54+112+71 = 394), 0 diffs
