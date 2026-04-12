# Image Property Split Analysis (fix-image-property-split)

## Conclusion

**The image property split is already correct.** No code changes are needed. The 87 cosmetic diffs are caused by class index renumbering during `fixupStylesAndClasses`, not by different style strings from image property splitting.

## Evidence

### Elvis (72 diff lines, 42 classes affected)

Sorted by properties, Go and Python produce **identical** image class strings:

- 7 wrapper styles (identical `margin-*`, `text-align`, `text-indent` values)
- 27 image styles (identical `width` values)
- Total: 34 `class_image*` rules in both outputs

Example wrapper match:
- Python: `.class_image-3 {margin-bottom: 1.5em; margin-top: 0.9996em}`
- Go: `.class_image-26 {margin-bottom: 1.5em; margin-top: 0.9996em}`

Same properties, different index. Pure renumbering.

### HG (45 diff lines, 35 classes affected)

Image classes are similarly identical when sorted by properties. The structural diffs in HG are NOT image-related:
- `.class_s79F`: margin vs text-align redistribution (table/paragraph issue)
- `.class_s78V`, `.class_s772-*`: missing text-align: center (table cell issue)
- `.class_s77D`: missing vertical-align: bottom variant (table issue)

### Martyr (0 diffs) and 3B (12 diffs)

Martyr already matches perfectly. 3B's diffs are mostly renumbering with a few non-image structural differences.

## Why Full BLOCK_CONTAINER_PROPERTIES Partition Doesn't Work

Attempting to use Python's full `BLOCK_CONTAINER_PROPERTIES` set (REVERSE_HERITABLE_PROPERTIES | BLOCK_ALIGNED_CONTAINER_PROPERTIES | {"display"}) causes regressions because:

1. **Different starting property sets**: Go's `effectiveStyle()` includes inherited properties from the KFX style chain, while Python's `content_style` only has explicitly set properties. This means Go's CSS map includes heritable properties (font-family, color, etc.) that Python's doesn't.

2. **Timing difference**: Go's `imageClasses()` runs during rendering (before simplifyStylesFull), while Python's `create_container` runs during content processing. The heritable properties haven't been stripped yet in Go when the partition happens.

3. **bodyInherited override**: Go's `simplifyStylesFull` overrides `bodyInherited["font-family"]` with the resolved default font when the body has a different font. This prevents stripping of heritable properties on the wrapper that Python would strip.

Results of attempting the full partition:
- Martyr: 0 → 8 diffs (WORSE) — font-family appears on wrapper div
- HG: 45 → 53 diffs (WORSE) — same issue
- Elvis: 72 → 72 (same)
- 3B: 12 → 12 (same)
- Tests: 10 → 12 failures (2 new failures from changed test expectations)

## Root Cause of Cosmetic Renumbering

The class index renumbering happens because `fixupStylesAndClasses` sorts by occurrence count (descending), then by encounter order. Styles with count=1 are sorted by encounter order, which depends on the rendering pipeline's traversal of KFX content. Go and Python traverse content in a slightly different order, causing the same style strings to be assigned different indices.

This is NOT fixable by changing the image property split. The split is already correct.
