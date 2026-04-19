# Symbol Format Functions (A2)

## What was ported

From `REFERENCE/Calibre_KFX_Input/kfxlib/yj_structure.py`:

| Go Function | Python Source | Lines |
|-------------|---------------|-------|
| `classifySymbol` | `classify_symbol` | 1042-1087 |
| `classifySymbolWithResolver` | `classify_symbol` (with SHARED) | 1042-1087 |
| `allowedSymbolPrefix` | `allowed_symbol_prefix` | 1089-1090 |
| `checkSymbolTable` | `check_symbol_table` | 1099-1149 |
| `findSymbolReferences` | `find_symbol_references` | 1151-1174 |
| `getReadingOrders` | `get_reading_orders` | 1182-1189 |
| `orderedSectionNames` | `ordered_section_names` | 1194-1201 |
| `hasIllustratedLayoutPageTemplateCondition` | `has_illustrated_layout_page_template_condition` | 1228-1256 |
| `getOrderedImageResources` | `get_ordered_image_resources` | 1258-1298 |

## Key patterns

- `getReadingOrders` prefers `$538` (DocumentData) over `$258` (ReadingOrderMetadata)
- `orderedSectionNames` deduplicates section names while preserving insertion order
- `hasIllustratedLayoutPageTemplateCondition` scans section fragment data recursively for the pattern: `$171` → SExp[3] with operator in `["$294","$299","$298"]`, position `$183`, anchor `$266`
- `allowedSymbolPrefix` uses `strings.Contains("abcdefilnpstz", prefix)` matching Python's `symbol_prefix in "abcdefilnpstz"`
- `getOrderedImageResources` requires `collect_content_position_info` (A5 scope) for full implementation; currently validates constraints only

## Gaps for later milestones

- `getOrderedImageResources` full implementation requires `collect_content_position_info` (A5)
- `checkSymbolTable` rebuild/replacement logic requires fragment list manipulation (future milestone)
- `create_local_symbol` not yet ported (used in page list creation)
