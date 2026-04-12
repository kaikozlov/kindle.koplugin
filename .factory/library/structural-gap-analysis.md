# Structural Gap Analysis: Python vs Go KFX Conversion Pipeline

**Generated:** 2026-04-12
**Source Python files:**
- `REFERENCE/Calibre_KFX_Input/kfxlib/yj_to_epub_properties.py` (2487 lines)
- `REFERENCE/Calibre_KFX_Input/kfxlib/yj_to_epub_content.py` (1945 lines)

**Go files analyzed:**
- `internal/kfx/yj_to_epub_properties.go` — style catalog, simplify_styles, CSS output
- `internal/kfx/yj_property_info.go` — YJ_PROPERTY_INFO, property_value, convert_yj_properties
- `internal/kfx/kfx.go` — decode, render, HTML generation (5050 lines)
- `internal/kfx/render.go` — renderBookState orchestration
- `internal/kfx/state.go` — bookState, fragment organization
- `internal/kfx/yj_to_epub_content.go` — section processing, reading order

---

## 1. Python Function Inventory

### `yj_to_epub_properties.py` — Top-level and class methods

| Function/Method | Lines | Summary |
|---|---|---|
| `Prop.__init__` | ~L74 | Property holder (name, value map) |
| `KFX_EPUB_Properties.__init__` | ~L696 | Initialize CSS rules, font name tracking |
| `KFX_EPUB_Properties.Style` | ~L704 | Create Style object from data |
| `process_content_properties` | ~L1081-1088 | Extract YJ properties from content, convert to CSS |
| `convert_yj_properties` | ~L1088-1170 | Convert YJ property map → CSS declarations (Style obj) |
| `property_value` | ~L1175-1380 | Convert single YJ property value → CSS string |
| `fixup_styles_and_classes` | ~L1388-1620 | Main post-processing: simplify styles, create CSS classes |
| `inventory_style` | ~L1622-1632 | Validate styles against KNOWN_STYLES |
| `update_default_font_and_language` | ~L1634-1660 | Scan content to determine best font/language |
| `set_html_defaults` | ~L1662-1688 | Set body font-family, font-size, line-height, writing-mode |
| `simplify_styles` | ~L1690-1960 | Recursive style inheritance, rem→em, heading/paragraph conversion |
| `add_composite_and_equivalent_styles` | ~L1962-2020 | Collapse composite sides, add -webkit- equivalents |
| `fix_and_quote_font_family_list` | ~L2022 | Font family list processing |
| `split_and_fix_font_family_list` | ~L2024 | Split font family string and fix names |
| `strip_font_name` | ~L2026 | Remove quotes from font name |
| `fix_font_name` | ~L2030-2070 | Font name normalization (replacements, misspellings, prefix stripping) |
| `fix_language` | ~L2072 | Language code normalization (_ → -, case) |
| `fix_color_value` | ~L2080 | Integer → CSS color string |
| `add_color_opacity` | ~L2086 | Apply opacity to color value |
| `color_str` | ~L2092 | Integer + alpha → CSS color string |
| `color_int` | ~L2100 | CSS color string → integer |
| `int_to_alpha` | ~L2114 | Alpha int → float (0.0–1.0) |
| `alpha_to_int` | ~L2124 | Alpha float → int |
| `pixel_value` | ~L2134 | Extract pixel value from Ion struct |
| `adjust_pixel_value` | ~L2150 | PDF-backed pixel adjustment (/100) |
| `add_class` | ~L2158 | Add CSS class to element |
| `get_style` | ~L2164 | Parse style attribute → Style object |
| `set_style` | ~L2168 | Set style attribute from Style object |
| `add_style` | ~L2174 | Merge new styles onto element |
| `create_css_files` | ~L2182-2210 | Write STYLES_CSS_FILEPATH with font-faces, rules, media queries |
| `quote_font_name` | ~L2212 | Quote font name for CSS |
| `css_url` | ~L2220 | Wrap value in url() |
| `quote_css_str` | ~L2224 | Quote CSS string |
| `Style.__init__` | ~L2232 | Parse style string/dict/element |
| `Style.get_properties` | ~L2244 | Parse CSS string → property dict |
| `Style.tostring` | ~L2264 | Serialize to CSS string |
| `Style.keys/items/get/len` | ~L2268 | Dict-like accessors |
| `Style.pop` | ~L2284 | Remove property |
| `Style.copy` | ~L2290 | Shallow copy |
| `Style.update` | ~L2294 | Merge properties with conflict detection |
| `Style.partition` | ~L2312 | Split properties by name/prefix |
| `Style.remove_default_properties` | ~L2340 | Remove properties matching defaults |
| `zero_quantity` | ~L2346 | Normalize numeric value to "0" for comparison |
| `capitalize_font_name` | ~L2362 | Capitalize words in font name |
| `class_selector` | ~L2366 | "." + class_name |

### `yj_to_epub_content.py` — Top-level and class methods

| Function/Method | Lines | Summary |
|---|---|---|
| `KFX_EPUB_Content.__init__` | ~L96 | Init context, style tracking |
| `process_reading_order` | ~L105-110 | Iterate reading orders, process sections |
| `process_section` | ~L112-210 | Main section dispatcher: comic/magazine/standard paths |
| `process_page_spread_page_template` | ~L212-350 | Page spread (facing pages, comics, PDF-backed) |
| `process_story` | ~L352-365 | Process a named story fragment |
| `add_content` | ~L367-380 | Dispatch content: text, content_list, or story |
| `process_content_list` | ~L382-388 | Iterate content list |
| `process_content` | ~L390-930 | **MAIN CONTENT RENDERER** — handles all content types ($269/$271/$274/$439/$270/$276/$277/$278/$454/$151/$455/$279/$596/$272), annotations ($683), conditional content ($591/$592/$663), style events ($142), dropcaps ($126/$125), first-line styles ($622), classifications ($615), position anchors |
| `create_container` | ~L932-945 | Wrap element in container div/a with property partitioning |
| `create_span_subcontainer` | ~L947-960 | Create span subcontainer for vertical-align |
| `fix_vertical_align_properties` | ~L962-980 | Convert -kfx-baseline-shift/style/table-vertical-align → vertical-align |
| `content_text` | ~L982-998 | Resolve text content from Ion ref |
| `combined_text` | ~L1000-1015 | Get combined text content of element |
| `locate_offset` | ~L1017-1035 | Find text offset position in element tree |
| `locate_offset_in` | ~L1037-1095 | Recursive offset location with span splitting |
| `split_span` | ~L1097-1110 | Split span at text offset |
| `reset_preformat` | ~L1112 | Reset preformat state |
| `preformat_spaces` | ~L1114-1145 | Normalize whitespace in element tree |
| `preformat_text` | ~L1147-1180 | Replace consecutive spaces with NBSP |
| `replace_eol_with_br` | ~L1182-1215 | Replace EOL chars with <br> elements |
| `prepare_book_parts` | ~L1217-1230 | EOL + whitespace normalization |
| `add_kfx_style` | ~L1232-1248 | Merge KFX style fragment into content |
| `clean_text_for_lxml` | ~L1250-1260 | Remove unexpected characters |
| `replace_element_with_container` | ~L1262-1270 | Wrap element in new container |
| `create_element_content_container` | ~L1272-1285 | Move element contents into new sub-element |
| `find_or_create_style_event_element` | ~L1287-1340 | Locate/create element at text offset for style events |
| `get_ruby_content` | ~L1342-1360 | Look up ruby content by name+ID |
| `is_inline_only` | ~L1362-1375 | Check if element tree is inline-only |
| `push_context/pop_context` | ~L1377-1385 | Context stack for error messages |
| `content_context` | ~L1377 | Property: current context string |

---

## 2. Go Function Inventory

### `yj_to_epub_properties.go` — Functions

| Function | Summary |
|---|---|
| `newStyleCatalog` | Create empty style catalog |
| `(*styleCatalog).addStatic` | Add static CSS rule |
| `(*styleCatalog).bind` | Bind declarations to token |
| `(*styleCatalog).reserveClass` | Reserve unique class name |
| `(*styleCatalog).finalize` | Produce sorted CSS string |
| `(*styleCatalog).replacer` | Create string replacer |
| `(*styleCatalog).markReferenced` | Mark tokens as referenced |
| `(*styleCatalog).String` | Get CSS string |
| `finalizeStylesheet` | Sort and format stylesheet |
| `collectReferencedClasses` | Scan sections for used classes |
| `pruneUnusedResources` | Remove unreferenced images |
| `pruneUnusedStylesheetRules` | Remove unreferenced CSS rules |
| `parseDeclarationString` | Parse "prop: val; ..." → map |
| `styleStringFromMap` | Map → CSS string |
| `declarationListFromStyleMap` | Map → sorted declaration slice |
| `styleMetadataForBaseName` | Derive -kfx-style-name/hints metadata |
| `styleStringFromDeclarations` | Build style string from declarations |
| `mergeStyleStrings` | Merge multiple style strings |
| `stripStyleMetadata` | Remove -kfx-style-name/hints |
| `setElementStyleString` | Set style attr on htmlElement |
| `prependClassName` | Add class name (dedup) |
| `updateDefaultFontAndLanguage` | Port of Python update_default_font_and_language |
| `setHTMLDefaults` | Port of Python set_html_defaults |
| `fixupStylesAndClasses` | Port of Python fixup_styles_and_classes |
| `addStaticBodyClasses` | Add body class static rules |
| `fixupEmptyClassAttributes` | Remove empty class="" |
| `createCSSFiles` | Append catalog CSS to book stylesheet |
| `simplifyStylesFull` | Top-level simplify_styles driver |
| `simplifyStylesElementFull` | Per-element simplify (recursive) |
| `isReverseHeritableProperty` | Check if property is reverse-heritable |
| `allowsReverseInheritance` | Check if tag allows reverse inheritance |
| `mergeStyleMaps` | Merge two style maps |
| `equalStyleMaps` | Compare two style maps |
| `equalStringLists` | Compare two string lists |
| `directChildElements` | Get direct child htmlElements |
| `rootHasDirectText` | Check if element has direct text children |
| `applyReverseInheritance` | Apply reverse inheritance to children |
| `styleEntryForClassName` | Look up style entry by class name |
| `styleMapFromClass` | Parse declarations from class name |
| `styleInfoFromClasses` | Extract style from class attribute |
| `applyStyleToElementClasses` | Apply style as class to element |
| `walkHTMLElement` | Walk all elements in tree |
| `sanitizeCSSClassComponent` | Sanitize CSS class name |
| `classPrefixFromStyle` | Derive class prefix from layout hints |

### `yj_property_info.go` — Functions

| Function | Summary |
|---|---|
| `propertyValue` | Convert single YJ property → CSS string |
| `propertyValueStruct` | Handle struct-type values (length, color, shadow) |
| `propertyValueNumeric` | Handle numeric values (color, px) |
| `propertyValueList` | Handle list-type values (hints, collisions, transforms) |
| `convertYJProperties` | Convert full YJ property map → CSS declaration map |
| `processContentProperties` | Extract and convert YJ properties from content |
| `cssDeclarationsFromMap` | Convert CSS map → sorted declaration slice |
| `popMap` | Remove and return map entry with default |
| `mergeTextDecoration` | Merge two text-decoration values |
| `mergeEpubType` | Merge two epub-type values |
| `valueStr` | Format numeric value as string |
| `asFloat64` | Extract float64 from interface |
| `fixColorValue` | Numeric → CSS color string |
| `addColorOpacityStr` | Apply opacity to color string |
| `splitCSSValue` | Parse CSS value into quantity + unit |
| `formatCSSQuantity` | Format float for CSS output |
| `convertStyleUnits` | Convert lh/rem → em in style map |

### `kfx.go` — Key Functions (5050 lines)

| Function | Summary |
|---|---|
| `Classify` | Determine if KFX is convertible |
| `ConvertFile` | Full KFX → EPUB conversion |
| `decodeKFX` | Decode KFX to decodedBook |
| `readSectionOrder` | Parse reading order from Ion |
| `parseSectionFragment` | Parse section fragment |
| `parseAnchorFragment` | Parse anchor fragment |
| `containerStyleDeclarations` | Container element CSS (legacy path) |
| `bodyStyleDeclarations` | Body element CSS (legacy path) |
| `spanStyleDeclarations` | Span element CSS (legacy path) |
| `headingStyleDeclarations` | Heading element CSS (legacy path) |
| `tableStyleDeclarations` | Table element CSS (legacy path) |
| `tableColumnStyleDeclarations` | Table column CSS |
| `tableCellStyleDeclarations` | Table cell CSS |
| `structuredContainerDeclarations` | thead/tbody/tfoot CSS |
| `cssFontFamily` | Resolve font-family with fixer |
| `cssLineHeight` | Convert line-height value |
| `cssLengthProperty` | Convert length property |
| `numericStyleValue` | Extract magnitude + unit |
| `cssColor` | Convert color int → CSS string |
| `fillColor` | Combine fill-color + opacity |
| `addColorOpacity` | Apply opacity to color |
| `colorDeclarations` | Link/visited color resolution |
| `(*storylineRenderer).renderStoryline` | Render storyline to HTML |
| `(*storylineRenderer).promoteCommonChildStyles` | Reverse inheritance |
| `(*storylineRenderer).renderNode` | Dispatch node rendering |
| `(*storylineRenderer).renderListNode` | Render <ol>/<ul> |
| `(*storylineRenderer).renderListItemNode` | Render <li> |
| `(*storylineRenderer).renderRuleNode` | Render <hr> |
| `(*storylineRenderer).renderHiddenNode` | Render display:none div |
| `(*storylineRenderer).renderFittedContainer` | Render inline-block container |
| `(*storylineRenderer).renderPluginNode` | Render iframe/audio/video/object |
| `(*storylineRenderer).renderSVGNode` | Render <svg> |
| `(*storylineRenderer).renderTableNode` | Render <table> with colgroup |
| `(*storylineRenderer).renderStructuredContainer` | Render thead/tbody/tfoot |
| `(*storylineRenderer).renderTableRow` | Render <tr> |
| `(*storylineRenderer).renderTableCell` | Render <td> |
| `(*storylineRenderer).renderInlinePart` | Render inline text/span |
| `(*storylineRenderer).renderImageNode` | Render <img> with wrapper |
| `(*storylineRenderer).renderTextNode` | Render <p>/<h1-6> |
| `(*storylineRenderer).applyAnnotations` | Apply style events + links to text |
| `(*storylineRenderer).applyPositionAnchors` | Add id attributes |
| `normalizeHTMLWhitespace` | Port of preformat_spaces |
| `renderHTMLElement` | Serialize htmlElement → HTML string |
| `applyCoverSVGPromotion` | Convert cover images to SVG |
| `normalizeLanguage` | Language code normalization |
| `newFontNameFixer` | Create font name fixer |
| `(*fontNameFixer).fixFontName` | Font name normalization |
| `(*fontNameFixer).fixAndQuoteFontFamilyList` | Full font family processing |
| `(*fontNameFixer).setDefaultFontFamily` | Set up default font replacement |
| `(*fontNameFixer).registerFontFamilies` | Register @font-face names |

### Other Go files

| File | Key Functions |
|---|---|
| `render.go` | `renderBookState`, `resolveRenderedAnchorURIs`, `prepareBookParts`, `materializeRenderedSections`, `cleanupRenderedSections` |
| `state.go` | `buildBookState`, `organizeFragments`, `loadContainerSource`, `collectContainerBlobs` |
| `yj_to_epub_content.go` | `processReadingOrder`, `processSection`, `renderSectionFragments`, `prepareBookParts` |

---

## 3. Cross-Reference: Python ↔ Go Equivalents

### Functions with clear Go equivalents ✅

| Python Function | Go Equivalent | Notes |
|---|---|---|
| `process_content_properties` | `processContentProperties` | Direct port |
| `convert_yj_properties` | `convertYJProperties` | Direct port |
| `property_value` | `propertyValue` + helpers | Direct port |
| `YJ_PROPERTY_INFO` (dict) | `yjPropertyInfo` (map) | Direct port |
| `YJ_LENGTH_UNITS` | `yjLengthUnits` | Direct port |
| `COLOR_YJ_PROPERTIES` | `colorYJProperties` | Direct port |
| `BORDER_STYLES` | `borderStyles` | Direct port |
| `COLLISIONS` | `collisions` | Direct port |
| `HERITABLE_PROPERTIES` | `heritableProperties` | Direct port |
| `HERITABLE_DEFAULT_PROPERTIES` | `heritableDefaultProperties` | Direct port |
| `NON_HERITABLE_DEFAULT_PROPERTIES` | `nonHeritableDefaultProperties` | Direct port |
| `COMPOSITE_SIDE_STYLES` | `compositeSideStyles` | Direct port |
| `ALTERNATE_EQUIVALENT_PROPERTIES` | `alternateEquivalentProperties` | Partial port (missing some entries) |
| `fixup_styles_and_classes` | `fixupStylesAndClasses` | Ported |
| `simplify_styles` | `simplifyStylesElementFull` | Ported (core features) |
| `add_composite_and_equivalent_styles` | Integrated into `simplifyStylesElementFull` | Merged into simplify pass |
| `update_default_font_and_language` | `updateDefaultFontAndLanguage` | Simplified port |
| `set_html_defaults` | `setHTMLDefaults` | Ported |
| `create_css_files` | `createCSSFiles` | Ported |
| `fix_font_name` | `(*fontNameFixer).fixFontName` | Ported |
| `fix_and_quote_font_family_list` | `(*fontNameFixer).fixAndQuoteFontFamilyList` | Ported |
| `fix_color_value` | `fixColorValue` | Ported |
| `color_str` | Part of `fixColorValue` | Integrated |
| `int_to_alpha` | Inline in `fixColorValue` | Integrated |
| `capitalize_font_name` | `capitalizeFontName` | Ported |
| `quote_font_name` | `quoteFontName` | Ported |
| `class_selector` | Inline ("." + name) | Trivial |
| `process_reading_order` | `processReadingOrder` | Ported |
| `process_section` | `processSection` / `renderSectionFragments` | Ported (reflowable subset) |
| `add_content` | `renderNode` dispatch | Different architecture |
| `process_content` | `renderTextNode`/`renderListNode`/etc. | Different architecture (see §4) |
| `fix_vertical_align_properties` | In `convertYJProperties` post-processing | Partial port |
| `content_text` | `resolveContentText` | Ported |
| `prepare_book_parts` | `normalizeHTMLWhitespace` + EOL splitting | Ported |
| `preformat_spaces` | `normalizeHTMLWhitespace` | Ported |
| `replace_eol_with_br` | `normalizeHTMLTextParts` (EOL → <br>) | Ported |
| `add_kfx_style` | `effectiveStyle` in render | Different architecture |
| `replace_element_with_container` | Inline in render functions | Different architecture |
| `Style` (class) | `map[string]string` + helper funcs | Replaced by simpler data structure |

### Functions with NO Go equivalent ❌ (Gaps)

| Python Function | Lines | Impact | Notes |
|---|---|---|---|
| `inventory_style` | ~L1622 | **Low** | Validates styles against KNOWN_STYLES; log-only, no output effect |
| `pixel_value` | ~L2134 | **Low** | Go handles px values inline during rendering |
| `adjust_pixel_value` | ~L2150 | **Low** | PDF-backed pixel adjustment (/100); Go doesn't support PDF-backed |
| `add_style` | ~L2174 | **Medium** | Go uses different architecture (set-style vs merge-style) |
| `Style.remove_default_properties` | ~L2340 | **Low** | Integrated into simplify loop differently |
| `Style.partition` | ~L2312 | **Medium** | Used heavily in Python for property splitting; Go uses map operations |
| `Style.update` (conflict detection) | ~L2294 | **Low** | CONFLICTING_PROPERTIES not ported |
| `create_span_subcontainer` | ~L947 | **Medium** | Used for vertical-align conflicts (rare case) |
| `combined_text` | ~L1000 | **Low** | Used for word boundary validation |
| `locate_offset` / `locate_offset_in` | ~L1017-1095 | **Medium** | Go has `locateOffset` but simpler; no zero_len/split_after modes |
| `split_span` | ~L1097 | **Low** | Text offset splitting for style events |
| `find_or_create_style_event_element` | ~L1287 | **High** | Multi-span style event wrapping — **not ported** |
| `clean_text_for_lxml` | ~L1250 | **Low** | Character sanitization |
| `create_element_content_container` | ~L1272 | **Low** | Move element contents into sub-element |
| `get_ruby_content` | ~L1342 | **Medium** | Go has `getRubyContent` but simplified |
| `is_inline_only` | ~L1362 | **Low** | Used for render:inline decision |
| `css_url` | ~L2220 | **Low** | URL wrapping for CSS |
| `quote_css_str` | ~L2224 | **Low** | CSS string quoting |

---

## 4. Architecture Differences

### Python Architecture (class-based, DOM manipulation)

Python uses two main classes:
- `KFX_EPUB_Properties` — manages CSS rules, styles, font names, style simplification
- `KFX_EPUB_Content` — renders content to lxml elements, then `fixup_styles_and_classes` processes the DOM

The pipeline is:
1. `process_content` → build lxml element tree with inline styles
2. `fixup_styles_and_classes` → `simplify_styles` → `add_composite_and_equivalent_styles` → class assignment
3. `create_css_files` → write stylesheet

### Go Architecture (functional, token-based)

Go uses a flat functional approach:
- `storylineRenderer` renders storyline nodes to `htmlElement` tree with token-based class references
- `styleCatalog` manages class binding/tokenization
- `simplifyStylesElementFull` does inheritance/simplification on the `htmlElement` tree
- No lxml dependency; custom `htmlElement` type

Key differences:
- Go pre-assigns CSS classes during rendering (via `styleCatalog.bind`), Python assigns them in `fixup_styles_and_classes`
- Go uses `map[string]string` instead of the `Style` class
- Go doesn't use `Style.partition()` — instead uses direct map operations

---

## 5. simplify_styles Feature Comparison

### Features PORTED to Go ✅

| Feature | Python Location | Go Implementation |
|---|---|---|
| Inherited property tracking | `simplify_styles` L1700-1705 | `simplifyStylesElementFull` — `parentStyle` map |
| font-size normalization (em → 1em) | L1700 | ✅ Ported |
| lh → em conversion | L1713-1735 | ✅ `convertStyleUnits` |
| rem → em conversion | L1736-1752 | ✅ `convertStyleUnits` |
| Link color resolution (<a> tag) | L1828-1831 | ✅ Ported |
| Reverse inheritance | L1840-1875 | ✅ `applyReverseInheritance` |
| Empty span unwrapping | N/A (Go-specific) | ✅ Go removes empty no-class spans |
| div → p conversion (text, no block children) | L1930-1935 | ✅ `tagChangedToParagraph` |
| div → figure conversion (figure hint + image) | L1926-1930 | ✅ `tagChangedToFigure` |
| div → h1-h6 conversion (heading hint) | L1924-1926 | ✅ In heading comparison logic |
| Non-heritable default merging | L1890-1895 | ✅ Ported |
| background-* stripping (transparent/none) | L1951-1953 | ✅ Ported |
| Composite side collapsing | `add_composite_and_equivalent_styles` | ✅ Ported |
| Ineffective property warning | L1995-2016 | ✅ Log-only |
| -kfx-link-color/-kfx-visited-color removal | L1916-1918 | ✅ Ported |
| Heading font-weight preservation | L1956-1960 | ✅ Ported |
| Paragraph margin defaults (1em) | L1933-1935 | ✅ Ported |
| Heading margin pop from inherited | L1943-1950 | ✅ Ported |

### Features NOT PORTED to Go ❌ (Gaps)

| Feature | Python Location | Impact | Notes |
|---|---|---|---|
| **Border-spacing from -webkit-border-h/v-spacing** | L1692 | **Medium** | `sty["border-spacing"] = sty["-webkit-border-horizontal-spacing"] + " " + sty["-webkit-border-vertical-spacing"]` |
| **-kfx-user-margin-*-percentage → -amzn-page-align** | L1694-1700 | **Medium** | Converts user margin percentages to page alignment property |
| **vh/vw viewport unit conversion** | L1753-1795 | **High** | Converts vh/vw to % with page-align awareness, including image dimension cross-conversion |
| **Negative padding stripping** | L1703 | **Low** | Discards invalid negative padding values |
| **outline-width removal when outline-style is none** | L1804 | **Low** | `if outline-width present and outline-style == "none": pop outline-width` |
| **OL/UL start attribute management** | L1806-1828 | **Low** | Ordered list value tracking for start/value attribute cleanup |
| **Position stripping for non-positioned elements** | L1690 | **Low** | `if position is static and top/bottom/left/right is 0: pop` |
| **background-image + -amzn-max-crop-percentage → background-size** | L1834-1840 | **Medium** | Converts crop percentages to `contain`/`cover` |
| **font-size rem normalization for inherited** | L1948 | **Low** | `if font-size in em: inherited["font-size"] = "1em"` (partially ported) |
| **fit_width handling (unknown width → % to px)** | L1796-1803 | **Medium** | Converts % margins to px when element width is unknown |
| **render:inline div→span conversion** | `process_content` ~L1290 | **Medium** | Conditional div→span conversion for inline rendering |
| **fit_tight width stripping** | `process_content` ~L1267 | **Low** | Removes width:100% when fit_tight is true |
| **COMBINE_NESTED_DIVS** | `process_content` ~L1393 | **Low** | Merges nested divs with identical properties |
| **create_container with property partitioning** | `process_content` ~L932 | **High** | Python partitions block/link container properties into wrapper elements; Go does this partially |
| **SVG/KVG shape processing** | `process_kvg_shape` (not shown) | **High** | KVG vector graphics rendering not ported |
| **Conditional page template evaluation** | `process_section` comic/magazine paths | **Medium** | Go only supports simple fixed-layout conditional evaluation |
| **Word boundary list validation** | `process_content` ~L935 | **Low** | Validates $696 word boundary offsets |
| **Annotation processing ($683)** | `process_content` ~L880 | **Medium** | MathML restoration, aria-label, alt_content; Go only does basic annotation |
| **Dropcap rendering** | `process_content` style_events | **Low** | Go has basic dropcap via `applyAnnotations`; Python has full style event system |

---

## 6. Key Gaps Summary

### High Impact (affects output fidelity)

1. **`find_or_create_style_event_element`** — Python's multi-element style event system splits text spans at arbitrary offsets and wraps ranges in new elements. Go's `applyAnnotations` is simpler (rune-level event matching) but doesn't handle all edge cases (e.g., events spanning multiple sibling elements).

2. **create_container with property partitioning** — Python's `create_container` splits properties into container vs content using `BLOCK_CONTAINER_PROPERTIES` / `LINK_CONTAINER_PROPERTIES` sets. Go partially handles this in render functions but doesn't have the full partition logic.

3. **vh/vw viewport unit conversion** — Not ported. Python converts vh/vw units to percentages for page-aligned content, including cross-converting width↔height for images with wrong axis units.

4. **SVG/KVG shape processing** — `process_kvg_shape` is not ported. Go renders SVG elements as empty `<svg>` tags without internal shapes.

5. **Full conditional content evaluation** — Python supports magazine/comic layout paths with `evaluate_binary_condition`. Go only supports fixed-layout conditional evaluation.

### Medium Impact (affects specific book types)

6. **-kfx-user-margin-*-percentage → -amzn-page-align** — Not ported. Affects books with user-configured page margins.

7. **background-image + -amzn-max-crop-percentage → background-size** — Not ported. Affects books with hero images using crop percentages.

8. **-webkit-border-spacing → border-spacing synthesis** — Not ported in simplify_styles. May cause extra properties in output.

9. **Negative padding stripping** — Not ported. Rare but could produce invalid CSS.

10. **fit_width %→px conversion** — Not ported. Affects reflowable images with percentage margins.

### Low Impact (cosmetic or rare)

11. **inventory_style** — Style validation; log-only, no output effect.
12. **outline-width removal** — Minor CSS optimization.
13. **OL/UL start/value management** — Attribute cleanup during simplify.
14. **Position stripping for static elements** — Minor CSS cleanup.
15. **Word boundary validation** — Debug-only feature.

---

## 7. Data Table Completeness

| Data Table | Python | Go | Status |
|---|---|---|---|
| `YJ_PROPERTY_INFO` | ✅ Complete (all ~120 properties) | ✅ Complete | **Ported** |
| `YJ_LENGTH_UNITS` | ✅ 15 units | ✅ 15 units | **Ported** |
| `COLOR_YJ_PROPERTIES` | ✅ | ✅ | **Ported** |
| `BORDER_STYLES` | ✅ 9 values | ✅ 9 values | **Ported** |
| `COLLISIONS` | ✅ 2 values | ✅ 2 values | **Ported** |
| `LAYOUT_HINT_ELEMENT_NAMES` | ✅ 3 entries | ✅ 3 entries | **Ported** |
| `HERITABLE_PROPERTIES` | ✅ ~60 properties | ✅ ~60 properties | **Ported** |
| `HERITABLE_DEFAULT_PROPERTIES` | ✅ ~50 entries | ✅ ~50 entries | **Ported** |
| `NON_HERITABLE_DEFAULT_PROPERTIES` | ✅ ~25 entries | ✅ ~25 entries | **Ported** |
| `COMPOSITE_SIDE_STYLES` | ✅ 5 composites | ✅ 5 composites | **Ported** |
| `ALTERNATE_EQUIVALENT_PROPERTIES` | ✅ 13 entries | ⚠️ 5 entries | **Partial** — missing text-emphasis-position, text-emphasis-style, text-emphasis-color, transform-origin, writing-mode equivalents |
| `CONFLICTING_PROPERTIES` | ✅ Extensive | ❌ Not ported | **Missing** — only used for warning logs |
| `KNOWN_STYLES` | ✅ Extensive | ❌ Not ported | **Missing** — only used for validation |
| `GENERIC_FONT_NAMES` | ✅ ~60 names | ✅ ~5 core | **Partial** — Go only tracks generic CSS font families |
| `MISSPELLED_FONT_NAMES` | ✅ 2 entries | ✅ 2 entries | **Ported** |
| `LIST_STYLE_TYPES` | ✅ 10 entries | ✅ 10 entries | **Ported** |
| `CLASSIFICATION_EPUB_TYPE` | ✅ 3 entries | ✅ 3 entries | **Ported** |
| `RESET_CSS_DATA` | ✅ | ❌ Not ported | **Missing** — Go doesn't emit a CSS reset file |
| `INLINE_ELEMENTS` | ✅ 8 tags | ✅ 8 tags | **Ported** |
| `COLOR_NAME` / `COLOR_HEX` | ✅ 15 entries | ✅ 15 entries | **Ported** |
