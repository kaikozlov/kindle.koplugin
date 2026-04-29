# Autoresearch Ideas

## Session 2026-04-28: 75→1 structural diffs (98.7% parity)

### Remaining Diff: 1984 c9 bare <div> wrapper

**Status**: Blocked — requires understanding a subtle Python rendering difference.

**The diff**:
- Go: `<body class="figure_sN-0"><a href="..." class="class_s29F"><img .../></a></body>`
- Cal: `<body class="figure_sN-0"><div><a href="..." class="class_s29F"><img .../></a></div></body>`

**Root cause investigation**:
- Go's rendering creates a `<div>` wrapper for BOTH c9 and c1K4 sections.
- `stripBareDivs` strips both divs (current behavior).
- Python's consolidate_html keeps c9's div (child `<a>` is non-block) but c1K4 never has a div.
- Python's PRE-consolidate HTML confirms: c9 has `<div>`, c1K4 does NOT.
- The difference is in the RENDERING, not post-processing.

**Data comparison (c9 posID=2320 vs c1K4 posID=3587)**:
- Node structure: IDENTICAL (type=text, style=sK, content_list with image)
- Template style: IDENTICAL (empty, uses promotedBodyContainer)
- Template values: IDENTICAL (empty map)
- Storyline data: IDENTICAL (content_list + story_name)
- Style fragments: sN and sK have NO overlapping properties (merge should succeed)
- Position anchors: Both content nodes have 1 anchor, templates have 0

**Why Python keeps c9's div**: Unknown. COMBINE_NESTED_DIVS should merge for BOTH
sections (same conditions). The difference must be in a very subtle aspect of Python's
processing order, template nesting, or side effects from processing other sections.

**Potential approaches**:
1. Accept the 1 diff (98.7% parity is very good)
2. Add Python debug to trace the exact rendering path for c9 vs c1K4
3. Try different approaches to make Go NOT create the div for c1K4
4. Use a per-section heuristic to decide whether to keep the div

### Completed Fixes (this session)
- ✅ Drop cap span splitting: split dropcap events at sub-event boundaries, fix sweep sort order
- ✅ Font-size stripping: remove font-size from promoted body CSS for image pages
- ✅ Position anchor check: block promotedBodyContainer when both template and child have anchors
- ✅ Figure+font-size check: skip promotion for figure containers with font-size overlap
- ✅ Page anchor offset: count `<br>` as 1 char in locateOffsetIn
- ✅ CSS margin auto for figure rules
- ✅ Inline image whitespace in renderHTMLParts
- ✅ stripBareDivs for single-child bodies
- ✅ Block-aware trailing `\n` in sectionXHTML
- ✅ Box-align → text-align + width removal for promoted image bodies
- ✅ JXR image MIME type
- ✅ `</body>` placement matching lxml

## Session 2026-04-28: c9 bare div fix → 0 structural diffs

### Root Cause
Python's COMBINE_NESTED_DIVS blocks merging when BOTH the outer container `<div>` 
AND inner container `<div>` have `id` attributes (from position anchors). This happens
when a content node AND its child container node both have position anchors.

For c9 in 1984: node 1312 (container) has a position anchor, and child node 955 
(also a container with content_list) also has a position anchor. Both get `id` attrs
on their `<div>` elements → COMBINE_NESTED_DIVS id check at line 1434 blocks merge.

The inner `<div>` survives as a bare wrapper (no style after simplify_styles).
Python's consolidate_html keeps it because its child `<a>` is non-block.

### Fix
- Extended `promotedBodyContainer` to return the container node ID
- Added double-anchor detection in `renderStoryline` that checks:
  1. Container node has position anchor (outer div)
  2. Child is a container node (has content_list) with position anchor (inner div)
- When detected, wraps body content in bare `<div>` with temporary `data-keep` attr
- `removeDataKeep()` strips the attr after `stripBareDivs` runs

### Key Insight
Only CONTAINER children (nodes with content_list) create their own `<div>` in
Python's rendering. Leaf nodes (text, images) don't create container divs, so
COMBINE_NESTED_DIVS doesn't apply to them. This is why the check must filter
for children with content_list.
