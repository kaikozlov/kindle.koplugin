# Autoresearch Ideas

## Body Class Split for Image Pages (52 diffs)
**Root Cause**: Go's promoted body style for image-only pages includes the image's `width:100%` property instead of the container's `text-align:center`. Both body and img end up with the same style → same class → no -0/-1 split.

**Evidence**:
- Go body declarations: `font-family: serif font-weight: normal line-height: 1lh width: 100%`
- Calibre body class (class_s1G-0): `text-align: center`
- Calibre img class (class_s1G-1): `width: 100%`

**Fix Approach**:
1. Study Python's `simplify_styles` for how body vs child properties are separated
2. The body should get container-level properties (text-align, margin, etc.)
3. The image should get image-specific properties (width, height)
4. This requires changes to `inferPromotedBodyStyle` or the simplify_styles reverse inheritance

**Affected Books**: HungerGames(17), Familiars(13), HeatedRivalry(7), Elvis(5), ThroneOfGlass(5), SecretsCrown(3)

## CSS Property: figure_sH width vs text-align
SunriseReaping's 1 remaining structural diff: Go has `width: 100%` but Calibre has `text-align: center` for the `.figure_sH` class. This is the same body class split issue manifested in a CSS rule.

## SecretsCrown Remaining Issues (4 structural)
1. Class index swap: `class_220-0`/`class_220-1` in wrong order
2. Extra `<div>` wrapper on some image pages
3. CSS property differences
