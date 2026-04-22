# YJ-to-EPUB Properties Audit Findings

**Audited by:** worker (feature: audit-yj-to-epub-properties)
**Date:** 2026-04-21
**Commits:** f0b4843, bde42ac, 0e365a3

## What Was Audited

Python file: `REFERENCE/Calibre_KFX_Input/kfxlib/yj_to_epub_properties.py` (2486 lines)
Go files:
- `internal/kfx/yj_to_epub_properties.go` (2151 lines) — style catalog, simplify_styles, fixup_styles_and_classes
- `internal/kfx/yj_property_info.go` (1453 lines) — YJ_PROPERTY_INFO, property_value, convert_yj_properties
- `internal/kfx/css_values.go` (710 lines) — legacy CSS helpers

## Functions Audited

| Python Function | Go Counterpart | Status |
|----------------|---------------|--------|
| `convert_yj_properties` (L1089-1172) | `convertYJProperties` | ✅ Fixed |
| `property_value` (L1174-1386) | `propertyValue` + helpers | ✅ Fixed |
| `fixup_styles_and_classes` (L1388-1600) | `fixupStylesAndClasses` | ✅ Audited |
| `simplify_styles` (L1664-1985) | `simplifyStylesElementFull` | ✅ Fixed |
| `update_default_font_and_language` (L1604-1650) | `updateDefaultFontAndLanguage` | ⚠️ Simplified |
| `set_html_defaults` (L1652-1671) | `setHTMLDefaults` | ✅ Audited |
| `inventory_style` (L1582-1587) | (warning logging only) | ✅ Audited |
| `create_css_files` (L2239-2265) | `createCSSFiles` | ✅ Audited |
| `fix_color_value` / `color_str` | `fixColorValue` / `colorStr` | ✅ Audited |
| `fix_and_quote_font_family_list` | `cssFontFamily` (css_values.go) | ✅ Audited |
| `add_color_opacity` | `addColorOpacityStr` | ✅ Audited |
| `Style` class | `parseDeclarationString` + helpers | ✅ Audited |

## Fixes Applied

### Fix 1: text-decoration-color → text-decoration: none !important (commit f0b4843)
- **Python L286-289**: When `text-decoration-color` is `"rgba(255,255,255,0)"` and no `text-decoration` present, removes the color and sets `text-decoration: none !important`.
- **Go location**: `yj_property_info.go` in `convertYJProperties`, after fill-color handling.
- **Impact**: Fixes SecretsCrown stylesheet diffs for classes with `text-decoration-color: rgba(255,255,255,0)`.

### Fix 2: text-combine-upright + writing-mode interaction (commit f0b4843)
- **Python L276-278**: When `text-combine-upright` is `"all"`, removes `writing-mode: horizontal-tb`.
- **Go location**: `yj_property_info.go` in `convertYJProperties`, after text-decoration merge.
- **Impact**: Prevents spurious `writing-mode: horizontal-tb` in elements with `text-combine-upright: all`.

### Fix 3: Fill-color alpha mask (commit bde42ac)
- **Python L1290-1291**: When `$70` (fill-color) numeric value has zero alpha bits (`& 0xff000000 == 0`), sets alpha to fully opaque (`| 0xff000000`) before color conversion.
- **Go location**: `yj_property_info.go` in `propertyValueNumeric`.
- **Impact**: Ensures fill-color values with zero alpha produce correct opaque colors.

### Fix 4: Collision value sorting (commit bde42ac)
- **Python L1352**: Sorted collision values alphabetically before joining.
- **Go location**: `yj_property_info.go` in `propertyValueList` case `"$646"`.
- **Impact**: Minor — ensures deterministic collision value ordering.

### Fix 5: Heading style name prefix in simplify_styles (commit 0e365a3)
- **Python L1521-1540**: When `-kfx-layout-hints` contains "heading", class name prefix becomes "heading".
- **Go location**: `yj_to_epub_properties.go` in `simplifyStylesElementFull`.
- **Impact**: Ensures divs converted to headings during simplify_styles get correct "heading_" prefix in style name.

## Gaps Found But NOT Fixed (Out of Scope / Deferred)

### 1. update_default_font_and_language is simplified (VAL-PROP-025)
Python scans all elements tracking font-family and language usage, then selects the most-used matching font. Go just normalizes the language. This doesn't affect current diffs because the rendering pipeline handles font assignment differently.

### 2. FXL inline-block for sized spans/anchors (VAL-PROP-019)
Python sets `display: inline-block` for `<a>` and `<span>` with height/width in fixed-layout. Go doesn't implement this. Not relevant for reflowable books.

### 3. VH/VW cross-conversion for images (VAL-PROP-003)
Python cross-converts vh/vw units using image dimensions when the property axis doesn't match the unit axis. Go handles the simple case (direct conversion) but skips cross-conversion. Requires image dimension knowledge in simplify_styles.

### 4. REM unit conversion conditional (VAL-PROP-002)
Python only converts rem to em when `self.generate_epub2 or self.GENERATE_EPUB2_COMPATIBLE`. Go converts unconditionally. Not a problem for current books since they don't use rem units differently.

### 5. direction/unicode-bidi markup conversion (VAL-PROP-020 area)
Python converts `direction` and `unicode-bidi` CSS properties to HTML `dir` attributes and `<bdo>`/`<bdi>` elements. Go handles `-kfx-attrib-xml-lang` extraction but not direction/unicode-bidi markup conversion. This affects RTL text handling.

### 6. process_polygon for $650
Python has `self.process_polygon(yj_value)` for `-amzn-shape-outside` polygon values. Go returns `fmt.Sprintf("%v", v)` as fallback. Not used by test books.

### 7. adjust_pixel_value for PDF-backed books
Python divides pixel values by 100 for PDF-backed books. Go doesn't implement this. Not relevant for non-PDF books.

### 8. -kfx-attrib-epub-type extraction condition (VAL-PROP-020)
Python checks `self.generate_epub2` before adding `epub:type` attribute. Go always adds it. This may cause minor differences in EPUB2 mode but isn't relevant for EPUB3 output.

### 9. -kfx-attrib-valign unexpected tag check
Python checks `if name in ["colspan", "rowspan", "valign"] and e.tag not in ["tbody", "tr", "td"]` and logs an error. Go doesn't check tag context for these attributes.

### 10. COMPOSITE_SIDE_STYLES expansion in convert_yj_properties
Python expands shorthand side properties (e.g., `margin: 0` → `margin-top: 0; margin-right: 0; margin-bottom: 0; margin-left: 0`) in convert_yj_properties. Go doesn't expand in convertYJProperties but handles collapsing in simplify_styles. The expansion direction is the opposite of what Python does, but the final CSS output should be equivalent because both collapse to shorthand in add_composite_and_equivalent_styles.

## Remaining Diffs Analysis

The remaining diffs in the new books (1984: 15, SecretsCrown: 4, SunriseReaping: 32) are primarily caused by:

1. **Content rendering gaps** (yj_to_epub_content.py scope):
   - Missing `<span epub:type="noteref">` for footnote links
   - Missing `<span class="...">th</span>` superscript handling
   - Missing drop cap splitting (float:left font-size:4em spans)
   - Missing empty span for position anchors (`<span id="a2BW"/>`)
   - Missing xmlns:epub namespace on html element
   - `<span>` vs `<div>` structural differences

2. **EPUB output gaps** (epub_output.py scope):
   - Section filename conventions
   - Missing xmlns:epub on some sections
   - nav.xhtml / toc.ncx structure differences

3. **CSS class ordering** — cascading effect from content rendering differences causing style counting to diverge, producing different numeric suffixes for `.class-N` names.

These are all in the scope of future feature audits (audit-yj-to-epub-content, audit-epub-output).
