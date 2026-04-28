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

## Deep Dive: Body Class Split Root Cause (2026-04-28)

### Investigation Results

The body class split (52 diffs) requires understanding a 3-stage pipeline:

1. **Rendering** (`renderStoryline`): Go computes bodyStyle from the promoted container's 
   style fragment. The container style has keys: `box_align, font_family, font_weight, 
   language, line_height, style_name, width`. Note: NO `text_alignment` key — the KFX data 
   uses `box_align` for image containers, not `text_alignment`.

2. **Simplify Styles** (`simplifyStylesFull`): Processes the body element's inline style.
   Current body style includes `width` (non-heritable) which should only be on the img child.

3. **Style Catalog** (`fixupStylesAndClasses`): Counts inline styles and assigns class names.
   Body and img end up with the same style → same class → no -0/-1 split.

### Python's Correct Behavior

In Python:
1. Body starts with NO style from content rendering
2. `set_html_defaults` adds font-family, font-size, line-height, writing-mode
3. `simplify_styles` processes children:
   - Child element (div→figure) has `-kfx-box-align: center` from KFX data
   - Python converts `-kfx-box-align` to `text-align: center` through simplify_styles
   - Reverse inheritance promotes `text-align: center` to the body element
   - `width: 100%` stays on the img (not reverse-heritable)
4. `fixup_styles_and_classes` creates classes:
   - Body: text-align:center → `class_s1G-0`
   - Img: width:100% → `class_s1G-1`

### Go's Bug

Go's body style includes ALL container properties including `width:100%`.
This is because `effectiveStyle(styleFragments[promotedStyleID], bodyStyleValues)` returns
the full fragment style including non-heritable properties.

### Attempted Fix: Filter non-heritable properties

Tried filtering bodyStyle to only heritable properties. This produced empty body style
because:
1. The container style has `box_align` (not `text_alignment`) for image pages
2. `box_align` is not in the heritable keys list
3. After filtering, the body has only `font_family, font_weight, line_height` → all 
   match inherited defaults → stripped by simplify_styles
4. Body ends up with NO style → class_s1G-0 has no CSS properties

### Correct Fix (requires deeper work)

The fix requires ensuring that `box_align` gets converted to `text-align` in the body's
style BEFORE the style catalog runs. This could be done by:
1. Processing the body style through the same CSS property mapping that `processContentProps`
   uses, converting `box_align` → `-kfx-box-align` → then to `text-align` when appropriate
2. OR: running simplify_styles on the body element BEFORE computing the BodyStyle string
3. OR: adding `box_align` to the heritable keys list and letting simplify_styles handle it

This is a significant architectural change that affects the rendering pipeline.

## ThroneOfGlass Missing Figure Wrapper (5 diffs)
**Root Cause**: For figure-hinted images (layout hints contain "figure"), Go's promotedBodyContainer promotes the container style directly to <body>, losing the inner <figure> wrapper. Python keeps the <figure> as a child element of <body>.

**Evidence**:
- Go: `<body class="figure_s4N"><img .../></body>` — no figure wrapper
- Calibre: `<body class="class-2"><figure class="figure_s4N"><img .../></figure></body>` — has wrapper

**Fix Approach**:
1. Check if the promoted body's layout hints contain "figure"
2. If yes, don't promote — render with normal container wrapper
3. The <div> wrapper will be converted to <figure> by simplify_styles
4. This requires modifying promotedBodyContainer or the inline rendering path

**Related CSS Issues**:
- figure_s4N: Go has {font-size, text-align}, Calibre has {margin-bottom:0, margin-top:0}
- class_sU: Go has {font-style, margin-top, text-align}, Calibre has {margin-bottom:0, margin-top:3em}
- These are simplify_styles figure conversion issues

## SecretsCrown Remaining Issues (4 diffs)
1. xQ10: Extra `<div>` wrapper around img in body. Same promoted body figure issue as ThroneOfGlass.
2. xQ213/xQ875: class_220-0/220-1 ordering swap. This is a drop-cap style assignment order issue.
3. CSS: class_93-0 has extra properties (font-size, height) that Calibre strips.

## HeatedRivalry Whitespace (7 diffs)
Promoted heading body text starts on next line in Go but same line in Calibre.
This is a cosmetic whitespace issue in sectionXHTML. The text "Part One" should be
immediately after `<body>`, not on a new line. Complex to fix because some promoted
bodies need the newline (Martyr c56) and others don't (HeatedRivalry c83).

## HungerGames Whitespace (1 diff)
Same as HeatedRivalry: two self-closing `<img/>` elements on separate lines vs same line.
