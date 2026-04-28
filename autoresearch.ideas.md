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

## Figure Wrapper Investigation (ThroneOfGlass c4D)

### The Problem
ThroneOfGlass c4D: Go puts figure style on `<body>`, Calibre keeps `<figure>` as child.
But for SunriseReaping, Go's current behavior (promote to body) IS correct.

### Root Cause
The difference is in whether the image needs a wrapper for property separation:
- **SunriseReaping**: wrapper has only text-align:center → already on body → wrapper redundant
- **ThroneOfGlass**: wrapper has margins → NOT on body → wrapper needed

### Attempted Fix
Tried keeping <figure> wrappers in promoted body inline path, but this incorrectly
adds wrappers for SunriseReaping too (where they shouldn't exist).

### Correct Approach (not yet implemented)
The fix needs to detect whether the image's wrapper properties are ALREADY covered
by the body's promoted style. If so, the wrapper is redundant → unwrap.
If not, the wrapper is needed → keep it and clear the body's promoted style.

This requires comparing the wrapper's CSS properties against the body's CSS properties
at rendering time, which is architecturally complex.

### Alternative
Check if the imageClasses wrapper would have properties that are NOT on the body.
This could be done by comparing imageClasses wrapperProps against the body's declarations.
If any wrapper properties are missing from the body, keep the wrapper.

## Session 2026-04-28 (continued)

### Whitespace Fix: sectionXHTML block-aware trailing \n
- **Status**: ✅ Implemented (-3 diffs in 1984)
- `bodyEndsWithBlockElement()` checks if body content ends with a block element closing tag.
- Only adds trailing \n before </body> for block elements (p, div, h1-h6, etc.).
- Inline elements (<a>, <span>, <img>) don't get trailing \n, matching Python's beautify_html.
- Self-closing tags (/>) ARE treated as block for this purpose (needed for <img/>, <hr/> pages).

### Calibre Reference Regeneration
- **Status**: ✅ Done
- All 10 reference EPUBs regenerated from current REFERENCE/Calibre_KFX_Input code.
- Old references were from an outdated Calibre version with different whitespace handling.
- Fresh references are the correct source of truth.
- Some diffs changed: 1984 went from 0→8 structural (new trailing \n diffs, div wrapper, page anchors).
- HeatedRivalry went from 3→0 (old diffs were against stale reference).

### Margin Auto for Box-Align — BLOCKED
- **Status**: ❌ Blocked by style catalog class ordering cascade
- Python's create_container adds margin-left/right auto for box-align.
- Adding margin auto to body CSS changes the style string, which changes the class name
  in the catalog. This cascades to reorder ALL generic classes, causing massive regressions.
- The original box-align→text-align + width removal was carefully balanced.
- Cannot add margin auto without architectural changes to the style catalog.
- Possible approaches:
  1. Post-process the stylesheet CSS to add margin auto to figure classes.
  2. Separate margin auto into a different style declaration that doesn't affect class ordering.
  3. Accept the diff and focus on other improvements.

### Remaining 15 Diffs Breakdown (fresh references)
- 1984: 5 (c1K4 div wrapper, cDT page anchor, stylesheet margin, nav/toc page anchor)
- SecretsCrown: 4 (xQ10 div wrapper, xQ213/xQ875 class swap, stylesheet)
- ThroneOfGlass: 5 (c4D figure wrapper, c9/cV class swap, cM p wrapper, stylesheet)
- HungerGames: 1 (c791 inline image whitespace)

## Session 2026-04-28 (continued, 12 diffs remaining)

### Remaining 12 Diffs Analysis

**1984 (4 diffs):**
- c9: stripBareDivs incorrectly strips a bare div that Calibre keeps. Same structure as c1K4
  (which SHOULD be stripped). Net positive (+1 from xQ10, -1 from c9 regression).
- cDT: missing page_134 anchor. Go generates page_130-133 but not 134. Position anchor
  insertion issue specific to one page.
- nav/toc: reference page_134 which doesn't exist. Fixed by fixing cDT page anchor.

**SecretsCrown (3 diffs):**
- xQ213/xQ875: class_220-0/1 ordering swap. Drop-cap style vs paragraph style assigned
  in wrong order. Depends on section encounter order in rendering pipeline.
- stylesheet class_93-0: Go has extra font-size and height properties that Calibre strips.
  simplify_styles gap.

**ThroneOfGlass (5 diffs):**
- c4D: Missing <figure> wrapper. Go promotes image style to body, losing figure wrapper.
  Requires property-aware wrapper decision at render time.
- c9/cV: class-3/4 ordering swap (italic/center vs bold/uppercase). Different encounter order.
- cM: promotedBodyContainer Case 2 treats fragment reference as inline text, but Calibre
  renders it as <p> inside body with generic body class. Both Martyr c56 and ToG cM have
  identical node structure (content={index,name}) — can't distinguish without resolved
  content. Investigation showed BOTH use $146 content_list fragments, not $145 text.
- stylesheet class_sU: has font-style: italic; text-align: center that Calibre doesn't.
  Related to cM — if cM went through normal rendering, the <p> would absorb these properties.

### Investigation: ToG cM vs Martyr c56

Both sections have IDENTICAL node structure:
- Single node with keys=[content, id, style, type]
- content = {index: N, name: "content_X"} (fragment reference)
- No content_list at node level
- Fragment resolves to $146 (content_list) with multiple strings

Python processes both through the SAME code path:
- process_content creates <div>, add_content resolves $145 → <span>text</span>
- is_top_level renames <div> to <body>
- consolidate_html strips <span> → text directly in body

But Calibre output differs: Martyr c56 = inline text in body, ToG cM = <p> inside body.
This suggests Python's is_top_level or rendering differs between these sections.
Possible cause: the page template structure differs (different number of container wrappers).

### Hard Architectural Issues (not easily fixable)
1. **CSS class ordering**: Determined by style encounter order. Go processes sections in
   different order than Python. Fixing requires matching Python's exact order → risky.
2. **promotedBodyContainer Case 2**: Both inline text and container text nodes have the
   same fragment reference structure. Need access to resolved content to distinguish.
3. **Figure wrapper**: Requires comparing wrapper CSS vs body CSS at render time.
4. **stripBareDivs ambiguity**: c9 and c1K4 have identical structures but Python treats
   them differently. Root cause unknown.

### Attempted: Font-size/height stripping from image bodies
- **Status**: ❌ Reverted
- Stripping height alone: no benefit (already handled by renderStoryline bodyCSS deletion)
- Stripping font-size: regresses HungerGames c9 (removes legitimate font-size)
- Stripping font-size only when height present: no effect (height already stripped before post-processing)
- The class_93-0 font-size issue requires understanding why Python strips it from body
  but keeps it on the img. This is a simplify_styles reverse inheritance gap.
