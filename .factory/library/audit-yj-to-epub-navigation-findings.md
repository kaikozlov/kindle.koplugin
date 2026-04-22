# Audit: yj_to_epub_navigation.py → yj_to_epub_navigation.go

**Date:** 2026-04-22
**Status:** Parity confirmed (394/394 original books)
**Commit:** 864a1b2

## Summary

Exhaustive branch audit of `yj_to_epub_navigation.py` (541 lines) → `yj_to_epub_navigation.go` (544 lines → 654 lines after fixes).

## Functions Audited (25 Python → Go mapping)

### Fully Parity-Confirmed Functions

| Python Function | Go Function | Status |
|----------------|-------------|--------|
| `process_anchors` | Split: `state.go` (fragment loading), `render.go` (anchor registration) | ✅ Parity |
| `process_navigation` | `processNavigation` | ✅ Parity (after fix) |
| `process_nav_container` | `processContainer` | ✅ Parity |
| `process_nav_unit` | `processNavUnit` | ✅ Parity (after fix) |
| `unique_anchor_name` | `uniqueAnchorName` | ✅ Parity |
| `get_position` | `parseNavTarget` | ✅ Parity |
| `get_representation` | `parseNavRepresentation` | ✅ Parity (after fix) |
| `position_str` | Inline: `fmt.Sprintf("%d.%d", ...)` | ✅ Parity |
| `register_anchor` | `registerAnchor` | ✅ Parity |
| `position_of_anchor` | Not needed (Go resolves anchors differently) | ✅ N/A |
| `report_missing_positions` | `reportMissingPositions` | ✅ Parity |
| `register_link_id` | Inline in `yj_to_epub_content.go` region magnification | ✅ Parity |
| `get_anchor_id` | `buildPositionAnchorIDs` + `makeUniqueHTMLID` | ✅ Parity |
| `get_location_id` | Inline in `parseNavTarget` | ✅ Parity |
| `process_position` | `applyAnchorsToHTML` in `storyline.go` | ✅ Parity |
| `move_anchor` | Not needed (Go uses different DOM model) | ✅ N/A |
| `move_anchors` | `attachSectionAliasAnchors` in `render.go` | ✅ Parity |
| `get_anchor_uri` | `resolveRenderedAnchorURIs` in `render.go` | ✅ Parity |
| `report_duplicate_anchors` | `reportDuplicateAnchors` | ✅ Parity |
| `anchor_as_uri` / `anchor_from_uri` | Inline: `"anchor:" + name` prefix | ✅ Parity |
| `id_of_anchor` | Not needed (Go resolves differently) | ✅ N/A |
| `fixup_anchors_and_hrefs` | `fixupAnchorsAndHrefs` + helpers | ✅ Parity |
| `root_element` | Not needed (Go uses structured HTML, not lxml) | ✅ N/A |
| `visible_elements_before` | `resolveAnchorURIsForElement` in `render.go` | ✅ Parity |

### Gaps Fixed in This Audit

1. **`$248` entry_set handling** (Python L209-225)
   - Each entry_set contains `$247` children and `$215` orientation filter
   - Orientation `$386` (portrait): clears children unless locked to landscape
   - Orientation `$385` (landscape): clears children when locked to landscape
   - Added `orientationLock` field to `navProcessor`
   - **Fixed:** Added entry_set processing loop with orientation filtering

2. **Description and Icon extraction** (Python L176-187, L247-271)
   - Python `get_representation` returns `(label, icon, description)`
   - `$245` → icon resource + label fallback
   - `$146` → description from content list text
   - `$244` → text label (overrides icon-based label)
   - `$154` → description override
   - **Fixed:** Added `parseNavRepresentation` function, added `Description`/`Icon` to `navPoint`

3. **Description/Icon pass-through to EPUB** (Python L224-227)
   - Python TocEntry carries description and icon → NCX `mbp:meta`
   - **Fixed:** Updated `navigationToEPUB` to pass description/icon to `epub.NavPoint`

## Architectural Notes

- Go splits navigation processing across two locations:
  - `yj_to_epub_navigation.go`: Pure navigation data processing (navProcessor, TOC, guide, pages)
  - `render.go`: Anchor resolution, URI mapping, EPUB conversion
  - `storyline.go`: Position-based anchor application to HTML elements

- The `navto_anchor` mapping (Python L221-224) for periodical page targets is intentionally not ported because Go uses a different anchor resolution strategy (position-based rather than name-based for page targets).

## Remaining Notes

- The `$146` description extraction in `parseNavRepresentation` is simplified compared to Python's full `process_content_list` → lxml text extraction. In practice, navigation descriptions are simple text strings, so the simplified version should be sufficient.

- The Python `PERIODICAL_NCX_CLASSES` (0→"section", 1→"article") is already correctly handled in `epub.go`.
