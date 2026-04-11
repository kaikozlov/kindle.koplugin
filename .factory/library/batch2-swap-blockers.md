# Batch 2 Swap Blockers

**Date:** 2026-04-11
**Status:** Failed — 276 new diff lines vs 74 baseline

## Root Causes

The batch 2 swap (replacing `paragraphStyleDeclarations`, `linkStyleDeclarations`, `imageWrapperStyleDeclarations`, `imageStyleDeclarations` with `processContentProperties`) produces 276 new CSS diff lines vs the 74-line baseline. Three root causes:

### 1. Unit/precision mismatch between `processContentProperties` and old `*StyleDeclarations`

The old per-element functions use `cssLengthProperty()` which directly converts `$310` (lh) units to `em` via `magnitude*1.2`. `processContentProperties` → `convertYJProperties` → `propertyValue` outputs raw `lh` units (e.g., `2.66667lh`), which are then converted by `convertStyleUnits()` in `simplifyStylesElementFull`. The two-step conversion introduces precision differences.

Example: Old `cssLengthProperty` outputs `margin-top: 3.2em`. New pipeline outputs `margin-top: 2.66667lh` which converts to approximately `3.200004em`.

**Fix needed:** Either make `propertyValue` output `em` directly (violates architecture constraint), or make `convertStyleUnits` produce exactly matching precision.

### 2. Missing explicit default properties

Old `paragraphStyleDeclarations` explicitly emits `margin-bottom: 0` and `margin-top: 0` when `$49`/`$47` are absent. `processContentProperties` doesn't output absent properties. While both approaches are semantically equivalent (no margin = default 0), the Python reference EPUB was generated with the explicit defaults, causing diffs.

**Fix needed:** Either emit explicit defaults in `processContentProperties`, or accept these diffs as cosmetic.

### 3. Link color inheritance requires cross-style lookup

Old `paragraphStyleDeclarations` uses `colorDeclarations(style, linkStyle)` which checks `$576`/`$577` (link/visited color) in both the paragraph style AND the link style. `processContentProperties` only processes the paragraph style. The link color may only exist in the linkStyle.

**Partial fix:** Use `colorDeclarations(style, linkStyle)` after `processContentProperties` to add the color. This was implemented and tested — colors are restored, but class splitting increases diffs slightly.

## What Was Tried

1. Basic swap of all 4 functions to `processContentProperties`
2. Link style property inheritance (font-family, font-style, font-weight, font-variant, text-transform) — merged from linkStyle into style before `processContentProperties`
3. Link color from `colorDeclarations(style, linkStyle)` added to CSS map after `processContentProperties`
4. Image wrapper: `-kfx-box-align` → `text-align` conversion
5. Image property split: margin-top/text-align → wrapper, line-height/width/height → image

## What Would Be Needed

For a clean swap (no new diffs), the output of `processContentProperties` + `simplifyStylesElementFull` must produce byte-identical CSS to the old per-element functions. This requires either:

1. **Exact precision match** in the lh→em conversion path (modify `convertStyleUnits` or `propertyValue`)
2. **Explicit default emission** from `processContentProperties` for margin-top/margin-bottom
3. **Accepting these as cosmetic diffs** and updating the verification criteria

## Recommendation

The simplest fix would be option 3 — treat missing `margin-bottom: 0` and precision differences as cosmetic. If strict matching is required, the lh→em conversion precision needs investigation.
