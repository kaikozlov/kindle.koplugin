# HG .class_s79F Property Redistribution Analysis

**Feature:** fix-hg-s79F-property-redistribution
**Status:** SKIPPED — structural diff, too risky to fix without regressions
**Date:** 2026-04-12

## Problem

HG `.class_s79F` has different properties between Go and Python:

- **Go:** `font-size: 0.75em; line-height: 1; text-align: center`
- **Python:** `font-size: 0.75em; line-height: 1; margin-bottom: 0; margin-top: 0`

## Root Cause

**Body promotion pipeline design difference.** In Go, when a section has a single container node with a style ID, `promotedBodyContainer()` promotes it to become the body style. The container's children become the Root's children, and the container's style becomes the BodyStyle.

For this specific section (c791.xhtml, containing two ad images):
1. The KFX data has a single container with styleID `s79F` containing two image children
2. Go promotes this container to the body → body gets s79F style, images are direct children of Root
3. The body stripping in `simplifyStylesFull` strips `margin-top: 0` and `margin-bottom: 0` (matching `nonHeritableDefaultProperties`) but keeps `text-align: center` (no default to match)

In Python:
1. No body promotion exists — the container stays as a `<div>` inside the body
2. The body gets a separate style (class-1 = text-align: center) from page template inference
3. The `<div>` is converted to `<p>` during `simplify_styles`
4. The `<p>` gets `margin: 0` from `non_heritable_default_properties`, but paragraph comparison inherited has `1em`, so `0 ≠ 1em` → margins survive
5. `text-align: center` on the `<p>` matches the body's inherited `text-align: center` → stripped

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

## Impact

- **Visual rendering:** Functionally equivalent — both center the images with no visible margins
- **CSS diff:** 2 lines (3 property differences) in HG stylesheet
- **No regressions:** This is the current behavior, not a regression

## Why Not Fixed

Fixing this would require either:
1. **Not promoting this container to body** — complex pipeline change affecting all sections
2. **Adjusting body stripping for promoted containers** — would need to track promotion state through the pipeline and apply paragraph-level comparison for margins, risking regressions across all 4 test files
3. **Adding margins back for body elements with paragraph-like content** — would change body element rendering globally

All approaches carry high regression risk. The visual impact is negligible.
