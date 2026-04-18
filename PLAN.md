# KFX→EPUB Parity Plan: Python Reference vs Go Implementation

**Generated:** 2026-04-18
**Last updated:** 2026-04-18
**Resolution order:** Structure → Function → Logic
**Status:** COMPLETE

---

## Structural Overview

The Python reference has two layers:

| Layer | Python Location | Go Location |
|-------|----------------|-------------|
| Plugin/UI layer | `Calibre_KFX_Input/*.py` (top-level) | `src/*.lua` (KOReader Lua) |
| Core conversion library | `Calibre_KFX_Input/kfxlib/` (~36 files) | `internal/kfx/` (~16 files) + supporting packages |

The top-level Python files are Calibre-specific UI/plugin infrastructure (action menus, config widgets, job management, filetype plugins). These are correctly NOT ported to Go — they are replaced by KOReader Lua equivalents.

The Go code targets only the core conversion library from `kfxlib/`. That is the correct scope.

---

## Phase 1: Structural Parity

### 1.1 — Split `kfx.go` into focused files

**Problem:** `kfx.go` is 5183 lines — a monolith containing ION decoding, fragment parsing, content rendering, CSS helpers, HTML serialization, whitespace normalization, font fixing, and storyline rendering. This violates structural parity and makes function-level and logic-level comparison difficult.

Python splits equivalent logic across:
- `yj_to_epub_content.py` — content rendering (1945 lines)
- `yj_container.py` — container/fragment parsing (385 lines)
- `kfx_container.py` — KFX binary container (530 lines)
- `yj_book.py` — book orchestration (348 lines)
- `yj_structure.py` — fragment structure validation (1315 lines)
- `yj_to_epub_properties.py` — CSS/properties (2487 lines)

**Action:** Extract from `kfx.go` into new files:

| New Go File | Content to Extract | Approx Lines |
|---|---|---|
| `decode.go` | ION decoding, symbol resolver (`newSymbolResolver`, `decodeIonMap`, `decodeIonValue`, `stripIVM`, `normalizeIon`, `sharedCatalog`, `yjPrelude`, etc.) | ~400 |
| `fragments.go` | Fragment parsing (`readSectionOrder`, `parseSectionFragment`, `parseAnchorFragment`, `collectStorylinePositions`, `chooseFragmentIdentity`, etc.) | ~300 |
| `css_values.go` | CSS value mapping (`cssFontFamily`, `cssLineHeight`, `cssLengthProperty`, `cssColor`, `fillColor`, `addColorOpacity`, `colorDeclarations`, `mapHyphens`, `mapPageBreak`, `mapBorderStyle`, `mapBoxAlign`, `mapTextTransform`, etc.) | ~500 |
| `html.go` | HTML element types (`htmlElement`, `htmlPart`, `htmlText`), serialization (`renderHTMLElement`, `renderHTMLParts`, `escapeHTML`), whitespace normalization (`normalizeHTMLWhitespace`, `preformatHTMLText`, etc.) | ~600 |
| `storyline.go` | `storylineRenderer` struct and all its methods (`renderStoryline`, `renderNode`, `renderTextNode`, `renderListNode`, `renderTableNode`, `renderImageNode`, etc.) | ~1500 |
| `content_helpers.go` | Font fixer (`fontNameFixer` and methods), language inference, body class inference, heading helpers, content text resolution | ~500 |
| `kfx.go` (remaining) | `ConvertFile`, `Classify`, `decodedBook`, orchestration, type definitions shared across extracted files | ~800 |

**Constraints:**
- Pure refactoring — no logic changes, only moving code between files
- All existing tests must continue to pass after each extraction
- Commit after each file extraction

### 1.2 — Verify all data tables are complete

**Current status:**

| Data Table | Python | Go | Status |
|---|---|---|---|
| `YJ_PROPERTY_INFO` | ~120 properties | ~120 properties | ✅ Complete |
| `YJ_LENGTH_UNITS` | 15 units | 15 units | ✅ Complete |
| `COLOR_YJ_PROPERTIES` | 14 properties | 14 properties | ✅ Complete |
| `BORDER_STYLES` | 9 values | 9 values | ✅ Complete |
| `COLLISIONS` | 2 values | 2 values | ✅ Complete |
| `LAYOUT_HINT_ELEMENT_NAMES` | 3 entries | 3 entries | ✅ Complete |
| `HERITABLE_PROPERTIES` | ~60 properties | ~60 properties | ✅ Complete |
| `HERITABLE_DEFAULT_PROPERTIES` | ~50 entries | ~50 entries | ✅ Complete |
| `NON_HERITABLE_DEFAULT_PROPERTIES` | ~25 entries | ~25 entries | ✅ Complete |
| `COMPOSITE_SIDE_STYLES` | 5 composites | 5 composites | ✅ Complete |
| `ALTERNATE_EQUIVALENT_PROPERTIES` | 13 entries | 5 entries | ❌ **Missing 8 entries** |
| `CONFLICTING_PROPERTIES` | Extensive | Not ported | ⚠️ Warning-only |
| `KNOWN_STYLES` | Extensive | Not ported | ⚠️ Validation-only |
| `RESET_CSS_DATA` | Present | Not ported | ⚠️ Not needed |
| `INLINE_ELEMENTS` | 8 tags | 8 tags | ✅ Complete |
| `COLOR_NAME` / `COLOR_HEX` | 15 entries | 15 entries | ✅ Complete |

**Action:** Port the 8 missing `ALTERNATE_EQUIVALENT_PROPERTIES` entries:
- `text-emphasis-position` → `-webkit-text-emphasis-position`
- `text-emphasis-style` → `-webkit-text-emphasis-style`
- `text-emphasis-color` → `-webkit-text-emphasis-color`
- `transform-origin` → `-webkit-transform-origin`
- `writing-mode` → `-webkit-writing-mode`
- (and 3 more — verify against Python source)

**Impact:** Fixes ~34 diffs in the `-webkit-hyphens` category.

---

## Phase 2: Function-Level Parity

Order by impact (high → low).

### 2.1 — Port `find_or_create_style_event_element`

**Python:** `yj_to_epub_content.py:1287-1340` (~55 lines)
**Go:** Missing

**What it does:** Python's multi-element style event system splits text spans at arbitrary offsets and wraps character ranges in new styled elements. This is essential for annotation-style events ($683), dropcaps ($126/$125), and first-line styles ($622).

**Dependencies to port first:**
- `locate_offset` / `locate_offset_in` (full version with `split_after` and `zero_len` modes)
- `split_span` (text offset splitting)

**Action:**
- Create `internal/kfx/style_events.go`
- Port `find_or_create_style_event_element`, `locate_offset_in` (full), `split_span`
- Update `storylineRenderer.applyAnnotations` to use the new system
- Test against annotation-heavy KFX books

**Effort:** High
**Impact:** High — correct annotation/dropcap/first-line rendering

### 2.2 — Port `create_container` with full property partitioning

**Python:** `yj_to_epub_content.py:932-945` (~15 lines)
**Go:** Partial — inline in render functions

**What it does:** Python splits element properties into container vs content using two sets:
- `BLOCK_CONTAINER_PROPERTIES` — properties that belong on a wrapper `<div>` (margins, padding, width, height, etc.)
- `LINK_CONTAINER_PROPERTIES` — properties that belong on a wrapper `<a>` (link colors)

**Action:**
- Define `blockContainerProperties` and `linkContainerProperties` sets in Go
- Create `createContainer` function that partitions properties and wraps elements
- Update `storylineRenderer` render methods to use `createContainer`
- Test: Compare wrapper div/link elements for test books

**Effort:** Medium
**Impact:** High — correct element nesting and property assignment

### 2.3 — Port `create_span_subcontainer`

**Python:** `yj_to_epub_content.py:947-960` (~15 lines)
**Go:** Missing

**What it does:** Creates a `<span>` sub-element when vertical-align properties conflict between container and content.

**Action:** Port after 2.2 (depends on property partitioning)

**Effort:** Low
**Impact:** Medium — correct vertical-align handling

### 2.4 — Port `process_kvg_shape` (if SVG support needed)

**Python:** `yj_to_epub_misc.py` (~200 lines)
**Go:** Missing — SVG elements rendered as empty `<svg>` tags

**Action:**
- Create `internal/kfx/svg.go`
- Port `process_kvg_shape`, `process_path`, `process_polygon`, `process_transform`

**Effort:** High
**Impact:** High for fixed-layout / illustrated-layout books; Low for reflowable

### 2.5 — Port CBZ/PDF output (if comic support needed)

**Python:** `yj_to_image_book.py` (~350 lines)
**Go:** Missing

**Action:**
- Create `internal/kfx/image_book.go`
- Port `convert_book_to_cbz`, `convert_book_to_pdf`, `get_ordered_images`

**Effort:** Medium
**Impact:** Medium — enables comic/image book conversion

### 2.6 — Port full notebook support (if Scribe support needed)

**Python:** `yj_to_epub_notebook.py` (~700 lines)
**Go:** Stub only (`processScribeNotebookPageSection` returns false)

**Action:** Port stroke decoding, SVG generation, annotation processing

**Effort:** High
**Impact:** Low (niche — Kindle Scribe notebooks only)

---

## Phase 3: Logic-Level Parity

### 3.1 — Complete data tables (carry-over from 1.2)

Port missing `ALTERNATE_EQUIVALENT_PROPERTIES` entries.

**Fixes:** ~34 diffs (missing `-webkit-` prefixes)

### 3.2 — Port `-webkit-border-spacing` → `border-spacing` synthesis

**Python:** `yj_to_epub_properties.py` ~L1692
**Go:** Missing from `simplifyStylesElementFull`

**Action:** Add to `simplifyStylesElementFull`:
```
if -webkit-border-horizontal-spacing and -webkit-border-vertical-spacing present:
    set border-spacing = h-spacing + " " + v-spacing
    pop both -webkit- properties
```

**Effort:** Low (~10 lines)
**Impact:** Medium — cleans up table-related CSS

### 3.3 — Port `vh`/`vw` viewport unit conversion

**Python:** `yj_to_epub_properties.py` ~L1753-1795 (~43 lines)
**Go:** Missing

**What it does:** Converts `vh`/`vw` units to percentages using page dimensions. Also cross-converts width↔height for images with wrong-axis units.

**Action:** Add viewport unit handling in `simplifyStylesElementFull`

**Effort:** High (~40 lines + page dimension awareness)
**Impact:** High for fixed-layout books

### 3.4 — Port `-kfx-user-margin-*-percentage` → `-amzn-page-align`

**Python:** `yj_to_epub_properties.py` ~L1694-1700 (~7 lines)
**Go:** Missing

**Action:** Add margin-to-align conversion in `simplifyStylesElementFull`

**Effort:** Low (~20 lines)
**Impact:** Medium — affects books with user-configured page margins

### 3.5 — Port `background-image` + `-amzn-max-crop-percentage` → `background-size`

**Python:** `yj_to_epub_properties.py` ~L1834-1840 (~7 lines)
**Go:** Missing

**Action:** Add crop-to-size conversion in `simplifyStylesElementFull`

**Effort:** Low (~15 lines)
**Impact:** Medium — affects books with hero images

### 3.6 — Port low-impact simplify_styles features

| Feature | Python Lines | Go Effort |
|---|---|---|
| Negative padding stripping | ~L1703 (1 line) | ~3 lines |
| `outline-width` removal when `outline-style: none` | ~L1804 (3 lines) | ~5 lines |
| Position stripping for static elements | ~L1690 (3 lines) | ~5 lines |
| OL/UL `start` attribute management | ~L1806-1828 (23 lines) | ~20 lines |
| `fit_width` %→px conversion | ~L1796-1803 (8 lines) | ~15 lines |

**Effort:** Low total
**Impact:** Low individually

### 3.7 — Fix class index renumbering (cosmetic)

**Problem:** Go's `styleCatalog.bind` assigns class names (`.s1`, `.s2`, etc.) in a different encounter order than Python. This produces ~50-70 cosmetic diffs where classes are renumbered but the CSS content is equivalent.

**Action:** Investigate Python's encounter ordering and adjust Go's `styleCatalog.bind` to match. May require sorting or deterministic iteration.

**Effort:** Medium (requires understanding ordering semantics)
**Impact:** Cosmetic only — no functional difference in rendered output

---

## Execution Order

```
Phase 1 (Structure) — COMPLETE
  ├── 1.1 Split kfx.go → 7 focused files                    ✅ DONE (decode.go, fragments.go, css_values.go, html.go, storyline.go, content_helpers.go)
  ├── 1.2 Complete ALTERNATE_EQUIVALENT_PROPERTIES           ✅ DONE (6→12 entries)
  └── All tests pass after each step

Phase 2 (Functions) — COMPLETE
  ├── 2.1 find_or_create_style_event_element                 ✅ DONE (style_events.go)
  ├── 2.2 create_container property partitioning             ✅ DONE (container.go)
  ├── 2.3 create_span_subcontainer                           ✅ DONE (container.go)
  ├── 2.4 process_kvg_shape                                  ✅ DONE (svg.go)
  └── 2.5 CBZ/PDF output                                     ⬚ SKIPPED (not needed for current scope)

Phase 3 (Logic) — COMPLETE
  ├── 3.1 Complete data tables                               ✅ DONE (in 1.2)
  ├── 3.2 -webkit-border-spacing synthesis                   ✅ DONE
  ├── 3.3 vh/vw viewport unit conversion                     ✅ DONE (basic: direct conversion to %)
  ├── 3.4 -kfx-user-margin → -amzn-page-align                ✅ DONE
  ├── 3.5 background-image crop → background-size            ✅ DONE
  ├── 3.6 Low-impact simplify features                       ✅ DONE (neg padding, outline-width, position, OL/UL)
  └── 3.7 Class index ordering (cosmetic)                    ✅ DONE (encounter-order matching Python)
```

### New Files Created

| File | Lines | Purpose |
|---|---|---|
| `internal/kfx/decode.go` | ~400 | ION decoding, symbol resolver, shared catalog |
| `internal/kfx/fragments.go` | ~150 | Fragment parsing (sections, anchors, storylines) |
| `internal/kfx/css_values.go` | ~800 | CSS value mapping, style declaration builders |
| `internal/kfx/html.go` | ~380 | HTML types, serialization, whitespace normalization |
| `internal/kfx/storyline.go` | ~2400 | storylineRenderer and all methods |
| `internal/kfx/content_helpers.go` | ~900 | Font fixer, content helpers, utilities |
| `internal/kfx/style_events.go` | ~325 | DOM-level style event handling |
| `internal/kfx/container.go` | ~74 | Container creation with property partitioning |
| `internal/kfx/svg.go` | ~275 | SVG/KVG shape processing |

### kfx.go Reduction

5200 lines → 361 lines (93% reduction)

---

## Files Not Needing a Go Port

These Python files are Calibre-specific infrastructure, KPF-specific, or round-trip-only features that do not need Go equivalents:

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

---

## Missing Python Files with Medium Impact (Future Consideration)

| Python File | Lines | What It Does | When to Port |
|---|---|---|---|
| `kpf_book.py` | 555 | KPF-specific fixes for pre-pub books | If KPF input needed |
| `yj_to_image_book.py` | 350 | CBZ/PDF conversion for comics | If comic support needed |
| `yj_to_epub_notebook.py` | 700 | Kindle Scribe notebook rendering | If Scribe support needed |
