# KFX→EPUB Parity Plan: Python Reference vs Go Implementation

**Generated:** 2026-04-18
**Last updated:** 2026-04-18
**Resolution order:** Structure → Function → Logic
**Status:** IN PROGRESS — Phase 1 (structural split + data tables) COMPLETE; Phase 2–3 remaining

---

## Structural Overview

The Python reference has two layers:

| Layer | Python Location | Go Location |
|-------|----------------|-------------|
| Plugin/UI layer | `Calibre_KFX_Input/*.py` (top-level) | `src/*.lua` (KOReader Lua) |
| Core conversion library | `Calibre_KFX_Input/kfxlib/` (~36 files, 25,760 lines) | `internal/kfx/` (~35 files, 15,010 lines) + supporting packages |

The top-level Python files are Calibre-specific UI/plugin infrastructure. These are correctly NOT ported to Go — replaced by KOReader Lua equivalents.

The Go code targets only the core conversion library from `kfxlib/`. That is the correct scope.

---

## 1. Structural Audit (File-by-File Comparison)

### Files correctly NOT ported (Calibre infrastructure / replaced by stdlib / library)

| Python File | Reason |
|---|---|
| `__init__.py` (top-level) | Calibre plugin registration |
| `action_base.py`, `action.py` | Calibre UI actions |
| `config.py` (top-level) | Calibre config widget |
| `gather_filetype.py`, `package_filetype.py` | Calibre filetype plugins |
| `jobs.py` | Calibre background job system |
| `kfx_input.py` | Calibre conversion widget |
| `metadata_reader.py` | Calibre metadata reader plugin |
| `original_source_epub.py` | Round-trip EPUB processing |
| `message_logging.py` | Replaced by Go `log` package |
| `version.py` | Not needed |
| `utilities.py` | Replaced by Go standard library |
| `ion.py`, `ion_binary.py`, `ion_text.py`, `ion_symbol_table.py` | Replaced by `amazon-ion/ion-go` |

### Files NOT needing a Go port (KPF-specific / round-trip only)

| Python File | Lines | Reason |
|---|---|---|
| `kpf_container.py` | 445 | KPF-specific SQLite container |
| `kpf_book.py` | 551 | KPF-specific fixes for pre-pub books |

### Structural parity map

| Python File | Lines | Go File(s) | Lines | Status |
|---|---|---|---|---|
| `yj_symbol_catalog.py` | 876 | `decode.go` | 257 | ⚠️ Partial — catalog exists, may miss some YJ symbols |
| `yj_container.py` | 385 | `state.go` + `fragments.go` | 613+187 | ✅ |
| `kfx_container.py` | 449 | `decode.go` + `state.go` | 257+613 | ✅ |
| `yj_book.py` | 348 | `kfx.go` (ConvertFile) | 361 | ✅ Orchestration ported |
| `yj_structure.py` | 1313 | `state.go` + `fragments.go` + `symbol_format.go` | 613+187+186 | ⚠️ Partial — `classify_symbol`, `check_consistency`, `walk_fragment` incomplete |
| `yj_metadata.py` | 885 | `yj_to_epub_metadata.go` | 332 | ⚠️ Partial — apply functions ported; getter/query functions missing |
| `yj_position_location.py` | 1324 | `yj_to_epub_navigation.go` (partial) | 519 | ❌ Most position/location mapping not ported |
| `yj_versions.py` | 1124 | Not ported | 0 | ❌ ~100 version/feature constants missing |
| `resources.py` | 804 | `yj_to_epub_resources.go` + `internal/jxr/` | 451+jxr | ⚠️ Partial — image conversion works; many helpers missing |
| `epub_output.py` | 1504 | `internal/epub/epub.go` | ~400 | ⚠️ Partial — zip/OPF works; nav/guide/page-map partial |
| `yj_to_epub.py` | 355 | `kfx.go` + `state.go` + `render.go` | 361+613+542 | ✅ |
| `yj_to_epub_content.py` | 1944 | `yj_to_epub_content.go` + `storyline.go` + `style_events.go` + `container.go` | 191+2429+327+74 | ✅ Split across files |
| `yj_to_epub_properties.py` | 2486 | `yj_to_epub_properties.go` + `yj_property_info.go` + `css_values.go` | 1814+1061+809 | ✅ Largest port |
| `yj_to_epub_navigation.py` | 541 | `yj_to_epub_navigation.go` | 519 | ⚠️ Partial — `reportMissingPositions`, `reportDuplicateAnchors` missing |
| `yj_to_epub_misc.py` | 611 | `yj_to_epub_misc.go` | 271 | ⚠️ Partial — SVG/image/block helpers partially ported |
| `yj_to_epub_metadata.py` | 290 | `yj_to_epub_metadata.go` | 332 | ✅ |
| `yj_to_epub_resources.py` | 374 | `yj_to_epub_resources.go` | 451 | ⚠️ Partial — JXR byte parity deferred |
| `yj_to_epub_illustrated_layout.py` | 408 | `yj_to_epub_illustrated_layout.go` | 101 | ⚠️ Partial — conditional page templates subset only |
| `yj_to_epub_notebook.py` | 703 | `yj_to_epub_notebook.go` | 16 | ❌ Stub only |
| `yj_to_image_book.py` | 353 | Not ported | 0 | ❌ CBZ/PDF output not in scope |
| `unpack_container.py` | 159 | `decode.go` | 257 | ✅ |
| `jxr_container.py` | 123 | `internal/jxr/` | jxr | ✅ |
| `jxr_image.py` | 2544 | `internal/jxr/` | jxr | ✅ |
| `jxr_misc.py` | 107 | `internal/jxr/` | jxr | ✅ |

---

## 2. Function-Level Gap List

### 2.1 Missing Python functions (not in Go at all)

#### High Impact

| Python Function | File:Lines | What It Does |
|---|---|---|
| `classify_symbol` (full) | yj_structure.py:~600 | Symbol classification with prefix checking; Go has simplified version |
| `check_fragment_usage` | yj_structure.py:~200 | Fragment usage validation and dependency checking |
| `walk_fragment` / `determine_entity_dependencies` | yj_structure.py:~300 | Fragment dependency graph traversal |
| `rebuild_container_entity_map` | yj_structure.py:~150 | Entity mapping for container structures |
| `pid_for_eid` / `eid_for_pid` | yj_position_location.py:~200 | Bidirectional position↔entity ID lookup |
| `create_position_map` | yj_position_location.py:~200 | Build position map from content position info |
| `collect_content_position_info` | yj_position_location.py:~500 | Walk content tree collecting position information |
| `create_location_map` | yj_position_location.py:~200 | Build location map from location info |
| `collect_location_map_info` | yj_position_location.py:~100 | Collect location mapping information |
| `process_page_spread_page_template` (full) | yj_to_epub_content.py:212-350 | Facing page spread handling for fixed-layout |
| `process_section` comic/magazine branches | yj_to_epub_content.py:~50 | Non-reflowable section processing paths |

#### Medium Impact

| Python Function | File:Lines | What It Does |
|---|---|---|
| `get_reading_orders` / `ordered_section_names` | yj_structure.py:~80 | Reading order extraction and section ordering |
| `has_illustrated_layout_page_template_condition` | yj_structure.py:~30 | Illustrated layout detection |
| `get_ordered_image_resources` | yj_structure.py:~50 | Image ordering for CBZ/PDF output |
| `generate_approximate_locations` | yj_position_location.py:~150 | Generate approximate page locations |
| `collect_position_map_info` | yj_position_location.py:~100 | Collect position map information |
| `verify_position_info` | yj_position_location.py:~50 | Validate position information consistency |
| `create_approximate_page_list` | yj_position_location.py:~50 | Generate approximate page list |
| `get_metadata_value` / `get_feature_value` | yj_metadata.py:~60 | Generic metadata/feature value getters |
| `is_magazine` / `is_sample` / `is_fixed_layout` / `is_print_replica` / `is_image_based_fixed_layout` | yj_metadata.py:~100 | Book type detection queries |
| `get_cover_image_data` / `fix_cover_image_data` | yj_metadata.py:~100 | Cover image extraction and repair |
| `update_cover_section_and_storyline` | yj_metadata.py:~100 | Cover section and storyline updates |
| `report_missing_positions` | yj_to_epub_navigation.py:~30 | Report navigation positions that could not be resolved |
| `report_duplicate_anchors` | yj_to_epub_navigation.py:~30 | Report duplicate anchor names |
| `report_features_and_metadata` | yj_metadata.py:~200 | Debug reporting of features and metadata |
| `get_asset_id` / `cde_type` / `is_kfx_v1` | yj_metadata.py:~50 | Asset identification queries |

#### Low Impact

| Python Function | File:Lines | What It Does |
|---|---|---|
| `check_consistency` | yj_structure.py:~100 | Debug validation of book structure |
| `find_symbol_references` | yj_structure.py:~50 | Symbol usage tracking |
| `is_known_generator` / `is_known_feature` / `kindle_feature_version` | yj_versions.py:~100 | Version/capability checking |
| `is_known_metadata` / `is_known_aux_metadata` | yj_versions.py:~50 | Metadata validation |
| `process_scribe_notebook_page_section` (full) | yj_to_epub_notebook.py:~700 | Scribe notebook page rendering |
| `process_scribe_notebook_template_section` | yj_to_epub_notebook.py:~200 | Scribe notebook template rendering |
| `decode_stroke_values` / `adjust_color_for_density` | yj_to_epub_notebook.py:~100 | Stroke/color conversion for Scribe |
| `convert_book_to_cbz` / `convert_book_to_pdf` | yj_to_image_book.py:~350 | CBZ/PDF output conversion |
| `combine_images_into_pdf` / `combine_images_into_cbz` | yj_to_image_book.py:~200 | Image collection to output format |

### 2.2 Functions with partial Go implementations

| Python Function | Go Function | What's Missing |
|---|---|---|
| `organize_fragments_by_type` + `replace_ion_data` | `organizeFragments` | Index walk doesn't match Python's `book_data` struct |
| `classify_symbol` | `classifySymbolWithResolver` | Fragment ID classification incomplete; missing `allowed_symbol_prefix` |
| `unique_part_of_local_symbol` / `prefix_unique_part_of_symbol` | `uniquePartOfLocalSymbol` | Style catalog / fragment keys may still use `chooseFragmentIdentity` |
| `process_section` | `processSection` | Only reflowable path ported; comic/magazine/scribe branches missing |
| `simplify_styles` (~L1928 div/p/figure) | `simplifyStylesPostProcess` | div→p/figure/span unwrapping partial |
| `fixup_anchors_and_hrefs` | `replaceRenderedAnchorPlaceholders` | Full anchor URI resolution partial |
| `fixup_illustrated_layout_anchors` | `fixupIllustratedLayoutAnchors` | Only subset of `-kfx-amzn-condition` rewrite |
| `generate_epub` | `epub.Write` | Manifest ordering, spine ordering, nav structure differences |
| `convert_jxr_to_jpeg_or_png` | JXR decode path | Byte-level parity deferred |

---

## 3. Logic-Level Gap List

| Gap | Python Reference | Impact | Notes |
|---|---|---|---|
| `replace_ion_data` struct walk | yj_to_epub.py:~120 | Medium | Fragment data substitution may miss edge cases in index walk |
| Fragment ordering post-organize | yj_to_epub.py:`organize_fragments_by_type` | Medium | Fragment processing order may differ from Python |
| `BLOCK_CONTAINER_PROPERTIES` partition edge cases | yj_to_epub_content.py:`create_container` | Medium | Some properties may misassign between wrapper and content elements |
| Annotation $683 full processing | yj_to_epub_content.py:~880 | Medium | MathML restoration, aria-label, alt_content not fully ported |
| `process_content` conditional content paths | yj_to_epub_content.py:~500 | Medium | $591/$592/$663 conditional content evaluation |
| Cover SVG promotion logic | epub_output.py:~800 | Low | Content-based cover matching vs simple title matching |
| NCX play order numbering | epub_output.py | Low | Sequential play order assignment |
| OPF manifest item ordering | epub_output.py | Medium | File ordering within manifest must match Calibre |
| Image resource variant handling | yj_to_epub_resources.py | Medium | Variant resource selection may not match Python's logic |
| Font @font-face emission ordering | `create_css_files` | Medium | Font face rules may be in different order |
| Guide entry type mapping | epub_output.py | Low | Guide type string mapping may differ |
| Floating-point precision in CSS values | Various `property_value` calls | Low | Cosmetic diff source; `formatCSSQuantity` may produce slightly different values |

---

## 4. Prioritized Stepwise Execution Plan

Resolution order: **Structure → Function → Logic**. Each step includes a commit after success.

### Phase 1: Structural Parity — COMPLETE ✅

```
  1.1 Split kfx.go → 9 focused files                       ✅ DONE
  1.2 Complete ALTERNATE_EQUIVALENT_PROPERTIES (6→12)       ✅ DONE
```

### Phase 2: Function-Level Parity — IN PROGRESS

Ordered by impact-to-effort ratio within each stream.

---

#### Stream A: Core Conversion Fidelity (High Impact)

**A1. Complete `replace_ion_data` / `organizeFragments` parity**
- Python: `yj_to_epub.py:organize_fragments_by_type` + `replace_ion_data`
- Go: `state.go:organizeFragments`
- Gap: Index walk doesn't match Python's `book_data` struct
- **~200 lines changed** in `state.go`
- Tests: Fragment summary snapshot comparison via `kfx_reference_snapshot.py`

**A2. Complete `classify_symbol` and symbol format wiring**
- Python: `yj_structure.py:classify_symbol`, `allowed_symbol_prefix`, `check_symbol_table`
- Go: `symbol_format.go:classifySymbolWithResolver`
- Gap: Fragment ID classification incomplete; style catalog / fragment keys may still use `chooseFragmentIdentity`
- **~150 lines changed** across `symbol_format.go`, `state.go`
- Tests: Symbol classification fixtures

**A3. Port `process_section` comic/magazine/fixed-layout branches**
- Python: `yj_to_epub_content.py:112-210`
- Go: `yj_to_epub_content.go:processSection`
- Gap: Only reflowable path ported
- **~300 lines new** in `yj_to_epub_content.go`
- Tests: Fixed-layout KFX fixtures

**A4. Port `process_page_spread_page_template` (full)**
- Python: `yj_to_epub_content.py:212-350`
- Go: Missing
- Gap: Facing page spread handling for fixed-layout / comics
- **~200 lines new** in `yj_to_epub_content.go`
- Tests: Facing-page KFX fixtures

**A5. Port `yj_position_location.py` (position/location mapping)**
- Python: `yj_position_location.py` (~1324 lines)
- Go: Minimal coverage in `yj_to_epub_navigation.go`
- Gap: `pid_for_eid`, `eid_for_pid`, `create_position_map`, `collect_content_position_info`, `create_location_map`, `generate_approximate_locations`, `create_approximate_page_list` — core position/location system
- **~800 lines new** in new file `internal/kfx/yj_position_location.go`
- Tests: Position map fixtures
- Key functions to port:
  - `ContentChunk` class → struct
  - `ConditionalTemplate` class → struct
  - `MatchReport` class → struct
  - `BookPosLoc.collect_content_position_info` → func
  - `BookPosLoc.collect_position_map_info` → func
  - `BookPosLoc.verify_position_info` → func
  - `BookPosLoc.create_position_map` → func
  - `BookPosLoc.pid_for_eid` → func
  - `BookPosLoc.eid_for_pid` → func
  - `BookPosLoc.collect_location_map_info` → func
  - `BookPosLoc.generate_approximate_locations` → func
  - `BookPosLoc.create_location_map` → func
  - `BookPosLoc.create_approximate_page_list` → func
  - `BookPosLoc.determine_approximate_pages` → func

---

#### Stream B: EPUB Output Quality (Medium Impact)

**B1. EPUB packaging parity (`epub_output.py`)**
- Python: `epub_output.py` (~1504 lines) — `generate_epub`, `create_opf`, `container_xml`, `create_ncx`, `create_epub3_nav`, `zip_epub`
- Go: `internal/epub/epub.go`
- Gap: Manifest ordering, spine ordering, guide types, NCX structure, cover handling
- **~400 lines changed** in `internal/epub/epub.go`
- Tests: Full EPUB structure comparison with reference EPUBs

**B2. Navigation reporting functions**
- Python: `yj_to_epub_navigation.py:report_missing_positions`, `report_duplicate_anchors`
- Go: `yj_to_epub_navigation.go`
- Gap: Diagnostic reporting functions missing
- **~80 lines new** in `yj_to_epub_navigation.go`
- Tests: Unit tests with anchor fixtures

**B3. Complete illustrated layout anchor rewrite**
- Python: `yj_to_epub_illustrated_layout.py:fixup_illustrated_layout_anchors`, `create_conditional_page_templates`
- Go: `yj_to_epub_illustrated_layout.go` (101 lines)
- Gap: Only subset of `-kfx-amzn-condition` handling
- **~100 lines new** in `yj_to_epub_illustrated_layout.go`
- Tests: Illustrated layout fixture

**B4. Resource variant handling**
- Python: `yj_to_epub_resources.py:get_external_resource`, `process_external_resource`
- Go: `yj_to_epub_resources.go`
- Gap: Variant resource selection incomplete
- **~80 lines new** in `yj_to_epub_resources.go`
- Tests: Resource fixture with variants

**B5. Metadata getter/query functions**
- Python: `yj_metadata.py` (~885 lines) — `is_magazine`, `is_fixed_layout`, `get_metadata_value`, `get_feature_value`, `get_cover_image_data`, etc.
- Go: `yj_to_epub_metadata.go` (332 lines) — has apply functions but not getter/query functions
- Gap: Book type detection queries, cover image handling
- **~300 lines new** in new file `internal/kfx/yj_metadata_getters.go`
- Tests: Metadata fixtures

---

#### Stream C: Version Registry & Cleanup (Medium Impact)

**C1. Port `yj_versions.py` constants**
- Python: `yj_versions.py` (~1124 lines) — `KNOWN_KFX_GENERATORS`, `KNOWN_FEATURES`, `KNOWN_METADATA`, `KINDLE_VERSION_CAPABILITIES`, etc.
- Go: Not ported
- Gap: All ~100 version/capability constants missing
- **~600 lines new** in new file `internal/kfx/yj_versions.go`
- Tests: Unit tests for known generators/features

**C2. Move enum Prop entries from `kfx.go` to `yj_property_info.go`**
- Currently enum property entries remain in `kfx.go` (leftover from monolith)
- **~50 lines moved** from `kfx.go` to `yj_property_info.go`
- Tests: Existing tests must pass

**C3. Port `yj_structure.py` fragment validation functions**
- Python: `check_fragment_usage`, `walk_fragment`, `determine_entity_dependencies`, `rebuild_container_entity_map`
- Go: Not ported
- Gap: Fragment dependency resolution and validation
- **~300 lines new** in new file `internal/kfx/yj_structure.go`
- Tests: Fragment dependency fixtures

---

#### Stream D: Niche Features (Low Impact)

**D1. Complete notebook/Scribe support**
- Python: `yj_to_epub_notebook.py` (~703 lines)
- Go: `yj_to_epub_notebook.go` (16-line stub)
- Gap: Stroke decoding, SVG generation, annotation processing all missing
- **~500 lines new** in `yj_to_epub_notebook.go`
- Tests: Scribe notebook fixtures

**D2. CBZ/PDF image book output**
- Python: `yj_to_image_book.py` (~353 lines)
- Go: Not ported
- Gap: `convert_book_to_cbz`, `convert_book_to_pdf`, `get_ordered_images`, `combine_images_into_pdf/cbz`
- **~300 lines new** in new file `internal/kfx/yj_to_image_book.go`
- Tests: Image book fixtures

**D3. Floating-point precision alignment**
- Various CSS value output paths in `yj_property_info.go`
- Gap: `formatCSSQuantity` may produce slightly different float formatting than Python
- **~30 lines changed** in `yj_property_info.go`
- Tests: Diff comparison with reference EPUBs

---

### Phase 3: Logic-Level Parity — COMPLETE ✅ (previous phases)

```
  3.1 Complete data tables                                    ✅ DONE
  3.2 -webkit-border-spacing synthesis                        ✅ DONE
  3.3 vh/vw viewport unit conversion                          ✅ DONE
  3.4 -kfx-user-margin → -amzn-page-align                    ✅ DONE
  3.5 background-image crop → background-size                 ✅ DONE
  3.6 Low-impact simplify features                            ✅ DONE
  3.7 Class index ordering (cosmetic)                         ✅ DONE
```

---

## 5. Size Estimates

### Per-Step Estimates

| Stream | Step | New Lines | Changed Lines | Effort |
|---|---|---|---|---|
| A | A1: organizeFragments parity | 0 | ~200 | 2-3 hours |
| A | A2: classify_symbol + symbol format | ~50 | ~100 | 2-3 hours |
| A | A3: process_section branches | ~300 | ~20 | 3-4 hours |
| A | A4: page_spread_page_template | ~200 | ~10 | 2-3 hours |
| A | A5: position/location mapping | ~800 | ~30 | 8-10 hours |
| B | B1: EPUB packaging | ~100 | ~300 | 4-5 hours |
| B | B2: Navigation reporting | ~80 | ~10 | 1 hour |
| B | B3: Illustrated layout complete | ~100 | ~20 | 1-2 hours |
| B | B4: Resource variants | ~80 | ~10 | 1 hour |
| B | B5: Metadata getters | ~300 | ~0 | 2-3 hours |
| C | C1: yj_versions.go | ~600 | ~0 | 3-4 hours |
| C | C2: Prop enum cleanup | ~0 | ~50 moved | 30 min |
| C | C3: yj_structure.go | ~300 | ~0 | 2-3 hours |
| D | D1: Notebook support | ~500 | ~0 | 5-6 hours |
| D | D2: CBZ/PDF output | ~300 | ~0 | 3-4 hours |
| D | D3: Float precision | ~0 | ~30 | 1 hour |
| **Total** | | **~3,710 new** | **~780 changed** | **~40-52 hours** |

### New Files to Create

| File | Approx Lines | Purpose |
|---|---|---|
| `internal/kfx/yj_position_location.go` | ~800 | Position/location mapping (pid/eid, position maps, location maps) |
| `internal/kfx/yj_metadata_getters.go` | ~300 | Metadata getter/query functions (is_magazine, get_cover_image_data, etc.) |
| `internal/kfx/yj_versions.go` | ~600 | Version/feature/capability constants from yj_versions.py |
| `internal/kfx/yj_structure.go` | ~300 | Fragment validation and dependency resolution |
| `internal/kfx/yj_to_image_book.go` | ~300 | CBZ/PDF image book output |

---

## Execution Order (Recommended Sequence)

```
Phase 1 (Structure) — COMPLETE ✅
  ├── 1.1 Split kfx.go → 9 focused files                       ✅
  └── 1.2 Complete ALTERNATE_EQUIVALENT_PROPERTIES (6→12)       ✅

Phase 2 (Functions) — IN PROGRESS
  │
  │ Stream A: Core Conversion Fidelity
  ├── A1: organizeFragments / replace_ion_data parity           ~2-3h
  ├── A2: classify_symbol + symbol format wiring                ~2-3h
  ├── A3: process_section non-reflowable branches               ~3-4h
  ├── A4: process_page_spread_page_template                     ~2-3h
  └── A5: yj_position_location.go (position/location mapping)   ~8-10h
  │
  │ Stream B: EPUB Output Quality
  ├── B1: EPUB packaging parity (manifest/spine/nav/guide)      ~4-5h
  ├── B2: Navigation reporting (missing positions, dup anchors) ~1h
  ├── B3: Illustrated layout anchor rewrite (complete)          ~1-2h
  ├── B4: Resource variant handling                             ~1h
  └── B5: Metadata getter/query functions                       ~2-3h
  │
  │ Stream C: Version Registry & Cleanup
  ├── C1: yj_versions.go (constants)                            ~3-4h
  ├── C2: Move enum Prop entries → yj_property_info.go          ~30m
  └── C3: yj_structure.go (fragment validation)                 ~2-3h
  │
  │ Stream D: Niche Features
  ├── D1: Notebook/Scribe support                              ~5-6h
  ├── D2: CBZ/PDF image book output                            ~3-4h
  └── D3: Floating-point precision alignment                    ~1h

Phase 3 (Logic) — COMPLETE ✅ (previous work)
  ├── 3.1-3.7 All simplify_styles and data table features      ✅
```

---

## Previous Work Summary

### Files Created in Phase 1

| File | Lines | Purpose |
|---|---|---|
| `internal/kfx/decode.go` | ~257 | ION decoding, symbol resolver, shared catalog |
| `internal/kfx/fragments.go` | ~187 | Fragment parsing (sections, anchors, storylines) |
| `internal/kfx/css_values.go` | ~809 | CSS value mapping, style declaration builders |
| `internal/kfx/html.go` | ~384 | HTML types, serialization, whitespace normalization |
| `internal/kfx/storyline.go` | ~2429 | storylineRenderer and all methods |
| `internal/kfx/content_helpers.go` | ~830 | Font fixer, content helpers, utilities |
| `internal/kfx/style_events.go` | ~327 | DOM-level style event handling |
| `internal/kfx/container.go` | ~74 | Container creation with property partitioning |
| `internal/kfx/svg.go` | ~278 | SVG/KVG shape processing |

### kfx.go Reduction

5200 lines → 361 lines (93% reduction)

### Data Tables Completed

All core data tables ported: YJ_PROPERTY_INFO (~120 entries), YJ_LENGTH_UNITS (15), COLOR_YJ_PROPERTIES (14), BORDER_STYLES (9), COLLISIONS (2), LAYOUT_HINT_ELEMENT_NAMES (3), HERITABLE_PROPERTIES (~60), HERITABLE_DEFAULT_PROPERTIES (~50), NON_HERITABLE_DEFAULT_PROPERTIES (~25), COMPOSITE_SIDE_STYLES (5), ALTERNATE_EQUIVALENT_PROPERTIES (12), INLINE_ELEMENTS (8), COLOR_NAME/COLOR_HEX (15).
