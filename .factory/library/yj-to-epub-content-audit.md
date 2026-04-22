# yj_to_epub_content.py → Go Branch Audit

**Date:** 2026-04-22
**Feature:** audit-yj-to-epub-content
**Commits:** 9f02b0e (epub:type noteref), b1ddcb2 (annotation merge)

## Summary

Comprehensive branch audit of `yj_to_epub_content.py` (1944 lines) against Go counterparts: `storyline.go` (2969 lines), `yj_to_epub_content.go` (1353 lines), `content_helpers.go` (830 lines), `style_events.go` (327 lines), `html.go` (432 lines).

## Architecture

Go restructured Python's monolithic `process_content` into a dispatch architecture in `renderNode`. Post-processing steps are handled at render-time by specialized functions.

## Fixes Applied

### 1. epub:type="noteref" on annotation links
- **Python:** `$616=$617` on style event → `epub:type="noteref"` on `<a>` element
- **Bug:** `epubTypeFromAnnotation` was only called in `applyContainerStyleEvents`, not `applyAnnotations`
- **Fix:** Added `epubTypeFromAnnotation` call in `applyAnnotations` link creation (storyline.go:2644)
- **Impact:** Fixes missing epub:type on footnote links in 1984

### 2. Annotation property merge with style fragment
- **Python L1142:** `self.add_kfx_style(style_event, style_event.pop("$157", None))` merges style fragment into annotation map
- **Bug:** Go's `applyAnnotations` only used the style fragment, losing annotation-specific properties
- **Fix:** Created `annotationSpanClass` that calls `effectiveStyle(styleFragments[styleID], annotationMap)` to merge both sources
- **Impact:** Ensures annotation properties like baseline-shift are processed

## Remaining Gaps (by severity)

### Critical (affect output)

1. **Position anchor placement** — Go puts `id` directly on element; Python creates zero-length `<span id="X"/>` at exact offset. Architectural difference in `applyPositionAnchors` vs Python's `locate_offset` with `zero_len=True`. Affects 1984 (a2BW, a2A2), SunriseReaping (a3TH), etc.

2. **$102 list indent** — Python applies padding-left from `$102` to lists/list-items; Go ignores it entirely.

3. **$683 annotations in standard pipeline** — Python processes MathML ($690), aria-label ($584), table alt_content ($749) as annotation types. Go only handles these in notebook code.

4. **$605 word_iteration nowrap** — Python adds `white-space: nowrap` for text containers; Go skips this.

5. **text-combine-upright in locate_offset_in** — Python collapses text length when `text-combine-upright: all`; Go doesn't.

6. **-kfx-render: inline in locate_offset_in** — Python treats elements with this style as single-character; Go doesn't.

7. **COMBINE_NESTED_DIVS general case** — Go only implements for table cells and image wrappers, not all divs.

8. **Superscript (vertical-align: super)** — SunriseReaping "4th" missing `<span class="class_s3WD">th</span>`. The annotation may exist but the style processing pipeline doesn't generate the CSS class. Needs investigation into whether the annotation's style fragment has `$31` or if the property comes from a different source.

9. **FXL position:absolute detection** — Python adds position:absolute when top/bottom/left/right present on non-positioned elements.

10. **fit_width % width propagation** — Python walks children to find and move % widths; Go doesn't.

### Moderate (validation/logging)

11. **$69 ignore → z-index** for fixed layout
12. **$475 fit_text**, **$684 pan_zoom_viewer**, **$429 backdrop style** — validation pops
13. **$629/$630/$700/$821/$755 table validation** — validation pops
14. **$686 kvg_content_type** — validation check
15. **$660 illustrated layout conditional** → epub-type
16. **replace_eol_with_br** — Go uses different whitespace normalization
17. **preformat_spaces** — NBSP conversion for spaces
18. **clean_text_for_lxml** — unexpected character sanitization
19. **$696 word_boundary_list** — validation

### Minor (logging or edge cases)

20. Unknown classification → warning
21. Unknown render value → error log
22. `$625` first_line_style type validation
23. COMBINE_NESTED_DIVS id/grandchild checks
24. Tail/text in non-span → error in locate_offset_in

## Functions Audited

| Python Function | Lines | Go Counterpart | Status |
|-----------------|-------|---------------|--------|
| process_content | 395-1265 | renderNode + specialized render* | ✅ Covered (architecture diff) |
| add_content | 330-345 | renderNode dispatch | ✅ Covered |
| process_section | 115-260 | renderSectionFragments | ✅ Covered |
| locate_offset_in | 1600-1667 | locateOffsetInFull | ✅ Covered |
| find_or_create_style_event_element | 1567-1600 | findOrCreateStyleEventElement | ✅ Covered |
| split_span | 1645-1653 | splitSpan | ✅ Covered |
| fix_vertical_align_properties | 1498-1517 | fixVerticalAlignProperties | ✅ Covered |
| combined_text | 1539-1555 | htmlElementText | ⚠️ Missing text-combine/img |
| content_text | 1519-1532 | resolveContentText | ✅ Covered |
| create_container | 1452-1467 | wrapNodeLink + others | ⚠️ Different approach |
| create_span_subcontainer | 1469-1480 | N/A (Go builds differently) | ✅ N/A |
| COMBINE_NESTED_DIVS | 1408-1448 | tableCellCombineNestedDivs | ⚠️ Partial (table cells only) |
| replace_eol_with_br | 1730-1756 | normalizeHTMLWhitespace | ⚠️ Different approach |
| preformat_spaces | 1766-1815 | N/A | ❌ Gap |
| add_kfx_style | 1817-1833 | effectiveStyle | ✅ Covered (different approach) |
| get_ruby_content | 1603-1615 | getRubyContent | ✅ Covered |
| is_inline_only | 1617-1625 | N/A | ❌ Gap |

## Test Results

- `go test ./internal/kfx/` — PASS (all tests)
- Original 6 books: 394/394 match (no regressions)
- New books: 1984 (15 differ), HeatedRivalry (0), SecretsCrown (4), SunriseReaping (32)
