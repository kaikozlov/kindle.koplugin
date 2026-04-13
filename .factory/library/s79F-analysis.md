# HG .class_s79F Property Redistribution Analysis

**Feature:** fix-hg-s79F-body-promotion-margins
**Status:** PARTIALLY FIXED — margins now match, text-align:center still differs
**Date:** 2026-04-12

## Problem

HG `.class_s79F` has different properties between Go and Python:

- **Go (before fix):** `font-size: 0.75em; line-height: 1; text-align: center`
- **Go (after fix):** `font-size: 0.75em; line-height: 1; margin-bottom: 0; margin-top: 0; text-align: center`
- **Python:** `font-size: 0.75em; line-height: 1; margin-bottom: 0; margin-top: 0`

## Root Cause

**Body promotion pipeline design difference.** In Go, when a section has a single container node with a style ID, `promotedBodyContainer()` promotes it to become the body style. The container's children become the Root's children, and the container's style becomes the BodyStyle.

For this specific section (c791.xhtml, containing two ad images):
1. The KFX data has a single container with styleID `s79F` containing two image children
2. Go promotes this container to the body → body gets s79F style, images are direct children of Root
3. The s79F KFX style has no margin properties, so the body style has no margins
4. Without the fix, no margins are added → body class has only text-align:center
5. With the fix, paragraph-level margins (0) are added and survive comparison against 1em default

In Python:
1. No body promotion exists — the container stays as a `<div>` inside the body
2. The body gets a separate style (class-1 = text-align: center) from page template inference
3. The `<div>` is converted to `<p>` during `simplify_styles`
4. The `<p>` gets `margin: 0` from `non_heritable_default_properties`, but paragraph comparison inherited has `1em`, so `0 ≠ 1em` → margins survive
5. `text-align: center` on the `<p>` matches the body's inherited `text-align: center` → stripped

## Fix Applied

In `simplifyStylesFull`, when processing the body style for a promoted container (detected by presence of `-kfx-style-name`):

1. Check if the body has NO margin properties AND the Root has no block-level children
   (matching Python's div→p conversion condition)
2. If so, add paragraph-level margins (`margin-top: 0`, `margin-bottom: 0`) to the body style
3. Compare these margins against the paragraph default (1em) instead of the div default (0)
4. Result: `margin: 0` survives because `0 ≠ 1em`, matching Python's paragraph comparison

The `rootHasBlockChildren` check prevents adding margins to promoted bodies like s790 that
contain tables and divs (Python keeps these as `<div>`, not `<p>`).

## HTML Structure Difference

```html
<!-- Python -->
<body class="class-1">
  <p class="class_s79F"><img .../> <img .../></p>
</body>

<!-- Go -->
<body class="class_s79F">
  <img .../>
  <img .../>
</body>
```

## Remaining Diff

`text-align: center` still appears in Go's `.class_s79F` but not in Python's. This is because:
- In Go, text-align comes from the promoted container's KFX style and survives body stripping
  (no heritable default to match against)
- In Python, text-align on the `<p>` child matches the inherited text-align from the body → stripped

This is a fundamental structural difference that cannot be fixed without major pipeline changes.
The visual rendering is equivalent (text-align: center is on the body in both cases).

## Impact

- **Visual rendering:** Functionally equivalent
- **CSS diff:** Reduced from 3 property diffs to 1 on `.class_s79F`
- **No regressions:** All 4 test files have same raw diff counts as baseline
