# Full Inconsistency Report: Go vs Python Reference

**Generated:** 2026-04-19
**Scope:** Line-by-line comparison of all ported Go code against Python source of truth

---

## Summary

| Severity | Count | Description |
|----------|-------|-------------|
| **HIGH** | **49** | Affects conversion output — wrong/missing functionality |
| **MEDIUM** | **37** | Affects edge cases, logging, or diagnostics |
| **LOW** | **35** | Cosmetic, internal, or logging differences |
| **TOTAL** | **121** | |

---

## HIGH Severity (49 items)

### Stream A: Core Conversion Fidelity (13 HIGH)

| ID | Area | Issue | Python | Go |
|----|------|-------|--------|-----|
| A2-8 | classify_symbol | `getOrderedImageResources` is non-functional stub — always returns error | yj_structure.py:1258-1298: walks content position info, validates fixed-layout, collects ordered images | symbol_format.go: iterates section fragments but inner loop is empty (`_ = pt`), always returns "does not contain image resources" |
| A3-7 | process_section | Book type dispatch not wired — comic/magazine/scribe always fall to reflowable | yj_to_epub_content.py:138-184: dispatches by is_comic/is_children/is_magazine | yj_to_epub_content.go:335-341: `detectBookType()` exists but result not passed to `processSection`; always uses reflowable path |
| A3-10 | process_section | Magazine branch is empty stub — no output produced | yj_to_epub_content.py:150-184: processes conditional templates, dispatches layouts, creates book parts | yj_to_epub_content.go:409-425: only counts templates, logs, returns empty |
| A3-11 | process_section | Comic branch is stub returning false — no output | yj_to_epub_content.py:142-149: calls process_page_spread_page_template | yj_to_epub_content.go:388-403: logs "not yet ported", returns empty |
| A4-9 | page_spread | Leaf branch produces no XHTML content | yj_to_epub_content.py:337-343: creates new_book_part, calls process_content, links CSS | yj_to_epub_content.go:682-702: only creates pageSpreadSection struct, no HTML output |
| A5-5 | position/location | `CollectContentPositionInfo` is complete stub returning nil | yj_position_location.py:128-577: ~450 lines walking section content, tracking eid→section, building ContentChunks, conditional templates | yj_position_location.go:848-854: `return nil` |
| A5-6 | position/location | `CollectPositionMapInfo` completely missing | yj_position_location.py:601-834: ~230 lines parsing $264/$265/$609/$610/$611 fragments, SPIM, dictionary maps | Go: no equivalent method exists |
| A5-7 | position/location | `anchorEidOffset` missing position offset ($143) | yj_position_location.py:579-586: reads $183→$155 and $143 offset | yj_position_location.go:266-280: returns PositionID only, always returns offset=0 |
| A5-14 | position/location | `CreateLocationMap` is complete stub | yj_position_location.py:1116-1130: removes old $550/$621, builds new location IonStructs | yj_position_location.go:387-391: returns false always |
| A5-15 | position/location | `CreateApproximatePageList` mostly stub | yj_position_location.py:1132-1258: ~130 lines handling reading orders, book_navigation, nav_containers, page list output | yj_position_location.go:757-780: validates CDE type only, returns early for everything else |
| A5-18 | position/location | `CreatePositionMap` computes but discards results | yj_position_location.py:906-958: creates $264/$265 fragments with IonStruct formatting | yj_position_location.go:590-640: builds data structures but `_ = positionMap; _ = positionIDMap` |
| A5-19 | position/location | Byte-vs-rune indexing in whitespace lookback | yj_position_location.py:1299: `chunk.text[chunk_offset]` — character indexing | yj_position_location.go:427-429: `chunk.Text[chunkOffset]` — byte indexing, wrong for UTF-8 |
| A5-25 | position/location | `check_position_and_location_maps` top-level orchestration missing | yj_position_location.py:103-126: calls all position/location methods, validates | Go: no equivalent method |

### Stream B: EPUB Output Quality (13 HIGH)

| ID | Area | Issue | Python | Go |
|----|------|-------|--------|-----|
| B1-2 | EPUB packaging | Missing Arabic-Indic numeral conversion in `fix_html_id` | epub_output.py:481: regex converts ٠-٩/۰-۹ to ASCII digits | epub.go:fixHTMLID: replaces with `_` instead |
| B1-3 | EPUB packaging | Missing dot→underscore for illustrated layout in `fix_html_id` | epub_output.py:479: `id.replace(".", "_")` when illustrated_layout | epub.go:fixHTMLID: dots always allowed |
| B1-4 | EPUB packaging | Always outputs EPUB3, no version switching | epub_output.py:654-683: scans content for EPUB3 features, downgrades if not needed | epub.go: hardcoded `version="3.0"` |
| B1-5 | EPUB packaging | Guide section emitted unconditionally | epub_output.py:1071: only for EPUB2/compatible | epub.go: always emits when guide entries exist |
| B1-13 | EPUB packaging | Missing NCX mbp: namespace, descriptions, masthead, periodical classes | epub_output.py:1210-1243: full mbp support | epub.go:appendNCXPoints: no mbp namespace |
| B4-10 | Resources | Missing tile ($636) image reassembly | yj_to_epub_resources.py:90-105: combine_image_tiles with $638/$637/$797 | yj_to_epub_resources.go: always reads single $165 location |
| B4-11 | Resources | Missing JXR-to-JPEG conversion in resource processor | yj_to_epub_resources.py:149-152: convert_jxr_to_jpeg_or_png | yj_to_epub_resources.go: no JXR handling in getExternalResource |
| B4-12 | Resources | Missing PDF page extraction | yj_to_epub_resources.py:155-168: convert_pdf_page_to_image | yj_to_epub_resources.go: PDF resources returned as raw data |
| B5-6 | Metadata | `isImageBasedFixedLayout` oversimplified | yj_metadata.py:286-296: calls get_ordered_image_resources with full validation | yj_metadata_getters.go: `len(cat.ResourceFragments) > 0` |
| B5-7 | Metadata | `getGenerators` missing PACKAGE_VERSION_PLACEHOLDERS filtering | yj_metadata.py:393-398: filters placeholder version strings | yj_metadata_getters.go: returns raw values |
| B5-8 | Metadata | `isKfxV1` potentially broken key mapping | yj_metadata.py:331-335: checks `fragment.value.get("version", 0)` | yj_metadata_getters.go: iterates Generators map, key mapping may differ |
| B5-9 | Metadata | `fixCoverImageData` doesn't re-encode JFIF JPEG | yj_metadata.py:556-578: re-encodes with PIL when no JFIF marker | yj_metadata_getters.go: logs warning, returns original data |
| B1-1 | EPUB packaging | Default title "Unknown" vs "Untitled" | epub_output.py:438: `"Unknown"` | epub.go:32: `"Untitled"` |

### Stream C: Version Registry (2 HIGH)

| ID | Area | Issue | Python | Go |
|----|------|-------|--------|-----|
| C3-1 | Fragment validation | Missing `is_kpf_prepub` handling for $610 discovery seeding | yj_structure.py:721-724: adds $610 to unreferenced types when kpf_prepub | yj_structure.go: no is_kpf_prepub parameter |
| C3-5 | Fragment validation | Missing deep `ion_data_eq` duplicate detection | yj_structure.py:783-790: compares Ion values, detects content-differing duplicates, fatal error for multi-book contamination | yj_structure.go: silently skips all duplicates without content comparison |

### Stream D: Niche Features (18 HIGH)

| ID | Area | Issue | Python | Go |
|----|------|-------|--------|-----|
| D1-1 | Notebook | 4 class methods missing entirely (stubs only) | yj_to_epub_notebook.py:78-614: processNotebookContent, scribeNotebookStroke, scribeNotebookAnnotation, scribeAnnotationContent | yj_to_epub_notebook.go:377-394: stubs returning false |
| D1-3 | Notebook | No PNG density map generation for variable-density strokes | yj_to_epub_notebook.py:402-480: PIL-based density maps, PRNG, base64 | Go: not implemented |
| D1-4 | Notebook | No SVG path generation for normal strokes | yj_to_epub_notebook.py:482-515: creates SVG `<path>` elements | Go: not implemented |
| D2-1 | Image book | `getOrderedImages` returns 1 value instead of 3 | yj_to_image_book.py:101: returns (images, pids, content_pos_info) | yj_to_image_book.go:79: returns only []ImageResource |
| D2-2 | Image book | `convertBookToCBZ` and `convertBookToPDF` high-level methods missing | yj_to_image_book.py:22-99: full orchestration with metadata, TOC page resolution | Go: no equivalent entry points |
| D2-3 | Image book | `cropImage` doesn't scale margins from resource-space to pixel-space | resources.py:696: scales by orig_width/resource_width ratio | yj_to_image_book.go:240: uses offsets directly as pixel coords |
| D2-5 | Image book | CBZ drops PDF pages instead of converting | yj_to_image_book.py:326-330: convert_pdf_page_to_image | yj_to_image_book.go:316-317: logs warning, skips |
| D2-6 | Image book | CBZ drops JXR images instead of converting | yj_to_image_book.py:331-333: convert_jxr_to_jpeg_or_png | yj_to_image_book.go:318-319: logs warning, skips |
| D2-7 | Image book | PDF writer is custom — no PDF merging, no compression, quality loss | yj_to_image_book.py:215-294: uses pypdf.PdfWriter | yj_to_image_book.go:376-810: minimal custom writer |
| D2-9 | Image book | Missing TOC page number resolution for PDF outline | yj_to_image_book.py:56-95: position_of_anchor → pid_for_eid | Go: no equivalent |
| D2-11 | Image book | `getResourceImage` simplified tile handling — takes first tile only | yj_to_image_book.py:157-213: full combine_image_tiles | yj_to_image_book.go:140: ResourceRawData map lookup |

---

## MEDIUM Severity (37 items)

### Stream A (5 MEDIUM)

| ID | Issue |
|----|-------|
| A1-2/6 | $258 gets special ID override in Go but not Python |
| A1-5 | Missing log.info for non-SHORT book symbol format |
| A2-3 | `checkSymbolTable` missing/unused symbol checking entirely stubbed |
| A2-10 | `has_illustrated_layout_page_template_condition` traversal path differs |
| A3-9 | Missing $171 condition validation for overlay templates |

### Stream A5 (8 MEDIUM)

| ID | Issue |
|----|-------|
| A5-2 | ContentChunk Text: empty string vs nil distinction lost |
| A5-3 | Text field can't distinguish "no text" from "empty text" |
| A5-8 | `has_non_image_render_inline` completely missing |
| A5-9 | $550 fragment structure validation missing |
| A5-10 | $621 fragment validation missing |
| A5-16 | Fixed-layout double-page-spread check missing from CreateApproximatePageList |
| A5-26 | section_stories / story_sections validation missing |
| A4-7 | Scale-fit font_size: Go returns 0 silently, Python would KeyError |

### Stream A (other) (4 MEDIUM)

| ID | Issue |
|----|-------|
| A3-3 | $174 not popped from section data |
| A3-5 | push_context/pop_context missing |
| A3-6 | check_empty missing |
| A4-10 | Final check_empty(page_template) missing |

### Stream B (8 MEDIUM)

| ID | Issue |
|----|-------|
| B1-7 | Missing spine page-progression-direction for RTL |
| B1-8 | Missing OPF metadata refinements (file-as, alternate-script) |
| B1-9 | OPF manifest item ordering may differ from Python |
| B1-10 | Font @font-face emission ordering may differ |
| B3-1 | Missing region magnification handling |
| B3-2 | Missing KFXConditionalNav class handling |
| B4-1 | Missing resource URI resolution for variant selection |
| B5-5 | `updateCoverSectionAndStoryline` missing $157 recursive style processing |

### Stream C (5 MEDIUM)

| ID | Issue |
|----|-------|
| C1-4 | KindleFeatureVersion doesn't replicate Python's True==1 equivalence |
| C2-1 | Duplicate color handling with divergent alpha thresholds between css_values.go and yj_property_info.go |
| C3-2 | Missing is_kpf_prepub EID def for $174 container |
| C3-3 | Missing is_sample/is_dictionary handling for unreferenced $597 fragments |
| C3-9 | EXPECTED_DICTIONARY_ANNOTATIONS checked unconditionally (should only check for dictionaries) |

### Stream D (4 MEDIUM)

| ID | Issue |
|----|-------|
| D2-4 | cropImage parameters semantically different (margins vs offsets) |
| D2-8 | No hard failure on unexpected image formats |
| D3-4 | valueStr(nil) returns "<nil>" instead of "" or "0" |
| D1-2 | color_str method call convention differs (standalone func vs receiver method) |

### Stream C (other) (3 MEDIUM)

| ID | Issue |
|----|-------|
| C3-4 | Missing is_kpf_prepub cleanup for $391/$266/$259/$260/$608 |
| C3-6 | Missing log_known_error semantics for duplicate $597 fragments |
| C3-7 | Missing is_dictionary/is_scribe_notebook handling in rebuild path |

---

## LOW Severity (35 items)

These are cosmetic/internal differences that won't affect conversion output:

- A1-1: Single-entry flattening missing (Go uses typed structs instead)
- A1-3: IonSExp not distinguished from IonList in Go
- A1-4: IonAnnotation handling not needed (stripped at decode)
- A1-7: None vs empty-string null check
- A1-8: Duplicate error message less detailed
- A2-2: create_local_symbol missing (write-path)
- A2-4: find_symbol_references adds all strings vs IonSymbol-only
- A2-5: No annotation symbol collection
- A2-6: replace_symbol_table_import missing (write-path)
- A2-7: Returns nil vs [] — functionally equivalent
- A3-1: Outer reading order loop handled upstream
- A3-2: Log severity level difference
- A3-4: DEBUG logging missing
- A4-2: push_context/pop_context missing
- A4-4: DEBUG logging missing
- A4-5: pop vs delete semantic difference (functional equivalent)
- A4-8: connected_pagination default handling
- A5-1: Type comparison uses %T vs isinstance
- A5-4: Python MatchReport.final has a bug; Go is correct
- A5-12: current_section_name None vs ""
- A5-13: Log level difference
- A5-21: Return type difference (nil vs None)
- A5-22: last_pii_ lazy init vs Go zero value
- A5-23: PosData class vs closures
- A5-24: REPORT_POSITION_DATA constant vs hardcoded true
- C1-1: Python trailing comma creates tuples (likely Python bug)
- C1-2: Missing SUPPORTS_HD_V1 dead constant
- C1-3: Extra IntVersionKey(1) fallback (correct adaptation)
- C1-5: Heterogeneous value type in KINDLE_VERSION_CAPABILITIES
- C2-2: mapBoxAlign extra "justify" case
- C3-7: Rebuild logic split into separate functions (architectural)
- C3-10: Missing log_error_once deduplication
- C3-12: CONTAINER_FRAGMENT_TYPES list→map
- D2-10: DEBUG_VARIANTS not defined
- D2-12: suffixLocation strings.Replace vs regex (identical)
- D3-8: valueStr(*float64(nil)) returns "0" vs Python's ""
