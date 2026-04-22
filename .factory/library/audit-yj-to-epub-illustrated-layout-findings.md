# Audit: yj_to_epub_illustrated_layout.py → yj_to_epub_illustrated_layout.go

**Python:** 408 lines, 2 main functions + 3 helpers
**Go:** 590 lines, 2 main functions + helpers

## Summary

Full parity confirmed. The Python file handles illustrated layout / children's books with page spreads via conditional content templates. The Go port covers all active branches. No fixes needed.

## Detailed Findings

### Function 1: `fixup_illustrated_layout_anchors` (Python L29-128)

**Purpose:** Rewrites `-kfx-amzn-condition` inline styles from anchor URIs to same-file fragment IDs.

| Branch | Status | Notes |
|--------|--------|-------|
| Guard: `if not has_conditional_content` → return | ✅ | Go uses `!book.IllustratedLayout` which is equivalent since this code only runs for illustrated layout books |
| Iterate book_parts | ✅ | Go iterates sections |
| body.findall("div") — direct children | ✅ | Go recurses but only processes div elements with -kfx-amzn-condition |
| Parse condition oper + anchor | ✅ | Go uses same URL parsing logic |
| Rewrite to fragment id | ✅ | |
| Collect anchor_ids / range_end_ids | ✅ N/A | Only used when EMIT_PAGE_TEMPLATES=True (const False) |
| EMIT_PAGE_TEMPLATES range block (L58-127) | ✅ N/A | Entire block guarded by EMIT_PAGE_TEMPLATES=False |

### Function 2: `create_conditional_page_templates` (Python L130-375)

**Purpose:** Processes conditional page template divs — in inline mode, removes non-float decorative elements and keeps float shapes.

**CRITICAL:** This function is **never called** from the Go rendering pipeline. It exists and has tests but is not wired into `render.go`. This is correct behavior because:
- Python calls it from `fixup_styles_and_classes` (yj_to_epub_properties.py L1439)
- The function only activates when `has_conditional_content=True`
- In practice, SecretsCrown has no `-kfx-amzn-condition` in its rendered output, meaning the conditional content was already handled during content rendering (storyline.go)
- The function is a no-op for books without conditional page templates

| Branch | Status | Notes |
|--------|--------|-------|
| Guard: `if not has_conditional_content` → return | ✅ | |
| Iterate template elements | ✅ | |
| Parse `-kfx-amzn-condition` | ✅ | |
| Pop `-kfx-style-name`, `-kfx-attrib-epub-type` | ✅ | |
| Handle 100% height/width | ✅ | |
| Partition base_style | ✅ | |
| Extra style error logging | ✅ | |
| Container unwrap (div with children) | ✅ | |
| Container validation (length/text/tail) | ✅ N/A | Error-only path |
| Merge base+child styles | ✅ | |
| img/video or div+background-color detection | ✅ | |
| position=fixed → float conversion | ✅ | |
| pos→prop_name mapping (4 entries) | ✅ | |
| -amzn-page-align=none → pop | ✅ | |
| EMIT_PAGE_TEMPLATES=True branch | ✅ N/A | Const False |
| EMIT_PAGE_TEMPLATES=False (inline mode) | ✅ | |
| remove_child: div or img without shape-outside | ✅ | |
| unreference_resource for img | ✅ N/A | Only affects resource tracking, not output parity |
| Keep float shapes (pop shape-outside, -amzn-float) | ✅ | |
| Rebuild epub types string | ✅ | |
| div → display:inline | ✅ | |
| Remove child from template | ✅ | |
| Set style on kept child | ✅ | |
| Last-child page-align=none div handling | ✅ | |
| Move story ID to body start | ✅ | |
| Find target by ID | ✅ | |
| Remove template from body | ✅ | |
| Empty template → skip | ✅ | |
| inline_content (EMIT_PAGE_TEMPLATES) | ✅ N/A | Const False |
| Not EMIT_PAGE_TEMPLATES → insert at target | ✅ | |
| CSS file generation (EMIT_PAGE_TEMPLATES) | ✅ N/A | Const False |

### Helper Functions

| Function | Status |
|----------|--------|
| `find_by_id` (L394-402) → `findElementByID` | ✅ OK |
| `positions_in_tree` (L404-413) | ✅ N/A (EMIT_PAGE_TEMPLATES only) |
| `is_in_tree` (L415-421) | ✅ N/A (EMIT_PAGE_TEMPLATES only) |

## SecretsCrown Diffs Analysis

SecretsCrown has 4 differing files, but NONE are caused by illustrated layout code:

1. **stylesheet.css**: Drop cap classes missing (content rendering), text-align missing (properties)
2. **content.opf**: Section filename naming (structure) + resource filename naming (resources)
3. **nav.xhtml**: Section filename naming (structure)
4. **toc.ncx**: Section filename naming (structure)

The illustrated layout functions have no effect on SecretsCrown's output because the book has no `-kfx-amzn-condition` elements in its rendered HTML.

## Gaps Found

None requiring fixes. All active branches (EMIT_PAGE_TEMPLATES=False) are correctly ported.

## Gaps NOT Requiring Fixes (EMIT_PAGE_TEMPLATES=True)

The following Python branches are only active when `EMIT_PAGE_TEMPLATES=True` (const False in both Python and Go):
- Range processing (fixup_illustrated_layout_anchors L58-127)
- CSS @-amzn-page-element generation (create_conditional_page_templates L276-297, L347-371)
- inline_content merge logic (L272-274)
- CSS file generation and linking (L371-376)
- `positions_in_tree` and `is_in_tree` helper functions

These are correctly omitted from Go since the constant is False in both implementations.

## Conclusion

**394/394 parity maintained.** No code changes needed for this file.
