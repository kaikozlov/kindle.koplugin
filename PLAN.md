# CSS Class Parity Plan

## Current Status (2026-04-20)

| Book | Match | Differ | Summary |
|------|-------|--------|---------|
| **Martyr** | 102 | 0 | ✅ Perfect match |
| **Three Below** | 23 | 0 | ✅ Perfect match |
| **Hunger Games** | 105 | 7 | Image names, table p wrappers, composite styles |
| **Throne of Glass** | 64 | 7 | Figure classes, JXR images, heading font-weight |
| **Elvis** | 30 | 2 | Missing `<p class="tableright">` wrappers; extra `class_cpytxt2-3` |
| **Familiars** | 49 | 5 | Heading color, font-weight class splitting, composite styles |

All books also share:
- `toc.ncx` has extra `xmlns:mbp` namespace (trivial fix)
- `content.opf` missing trailing newline (trivial fix)
- Image manifest items differ for Throne of Glass (.jxr vs .jpg — JXR decoder needed)

---

## Defect Taxonomy

Every remaining diff falls into one of these categories. Each is listed with:
- **Root cause**: where the Go code diverges from Python
- **Python reference**: exact file and line
- **Go file**: where the fix goes
- **Affected books**: which books have this defect

---

### D1: Missing `text-indent: 0` on block elements

**Severity**: High — cascades into class index mismatches across all styles

**Affected**: Familiars (10 CSS rules), Hunger Games (image divs)

**Symptom**: Go omits `text-indent: 0` from CSS rules where Python includes it. In Familiars, styles like `class_s1E7-0` are missing `text-indent: 0`. In Hunger Games, image wrapper divs (`class_s6A`, `class_s3A`) produce `{margin-bottom: 3em}` instead of `{margin-bottom: 3em; text-indent: 0}`.

**Root cause**: Two sub-causes:

#### D1a: Body style inference omits text-indent

**Python reference**: `yj_to_epub_content.py` lines 42-68 (body style inference), plus `yj_to_epub_properties.py` lines 1655-1670 (body default injection)

**Go file**: `internal/kfx/storyline.go` `inferBodyStyleValues()`, `defaultInheritedBodyStyle()`, `renderStoryline()`

Python's body style inference captures `text-indent` from the first content paragraph and includes it in the body's style. This causes the body's heritable defaults to override `text-indent: "0"` with the paragraph's text-indent (e.g., "1.65em"). Then when `activeTextIndentNeedsReset()` is true, headings and paragraphs that don't have their own text-indent get `text-indent: 0` explicitly added (to reset the non-zero body default).

Go has `activeTextIndentNeedsReset()` in `storyline.go:2083` and uses it at lines 1728, 1779, but only for `headingClass()` and `paragraphClass()`. The body style inference in `inferBodyStyleValues()` may not include text-indent correctly.

**Fix steps**:
1. Verify `inferBodyStyleValues()` captures `$36` (text-indent) from the first content paragraph
2. Verify `defaultInheritedBodyStyle()` includes `$36`
3. Verify `activeTextIndentNeedsReset()` correctly detects non-zero body text-indent
4. Check that `headingClass()` and `paragraphClass()` add `text-indent: 0` when reset is needed
5. For image wrappers: verify `imageClasses()` also adds `text-indent: 0` when the style fragment doesn't include `$36` and reset is needed

#### D1b: `simplifyStylesElementFull` strips text-indent inconsistently

**Python reference**: `yj_to_epub_properties.py` lines 1958-1960 (stripping loop)

**Go file**: `internal/kfx/yj_to_epub_properties.go` lines 1632-1640

When the body has `text-indent: "0"` (from heritable defaults), the image wrapper div inherits `text-indent: "0"`. If the div's explicit style also has `text-indent: "0"` (added by D1a fix), then `sty["text-indent"]` = "0" matches `inherited["text-indent"]` = "0" → stripped. Python keeps it because Python's stripping loop runs against the *original* `inherited_properties` which may have a different value.

**Fix**: Verify the `comparisonInherited` map used for stripping matches what Python uses. Python's `inherited_properties` at the stripping point (line 1958) has already been modified by the tree walk (line 1919 adds non-heritable defaults, lines 1921-1924 pop for heading conversion). The Go code needs to match this exactly.

---

### D2: Missing `<p class="tableright">` wrappers in table cells

**Severity**: Medium

**Affected**: Elvis

**Symptom**: Go renders `<td class="class_tableleft-0">2012028329</td>` but Python renders `<td class="class_tableleft-0"><p class="class_tableright">2012028329</p></td>`.

**Python reference**: `yj_to_epub_content.py` lines 1279-1282 — Python wraps `<td>` content in `<p>` when the content style has paragraph-like properties.

**Go file**: `internal/kfx/storyline.go` table rendering path

**Fix steps**:
1. Find the table cell rendering code in `storyline.go`
2. Add logic to wrap `<td>` content in `<p class="class_tableright">` when the cell's style has no block-level children but has text content and a non-zero margin
3. Register `class_tableright` as a static rule in the style catalog: `{margin-bottom: 0; margin-top: 0}`

---

### D3: Extra `class_cpytxt2-3` class

**Severity**: Low (cosmetic)

**Affected**: Elvis

**Symptom**: Go produces `.class_cpytxt2-3 {font-size: 0.769231em; margin-top: 0.3em; text-align: center}` which Python doesn't produce. The HTML uses `<div class="class_cpytxt2-3">` where Python uses `<p class="class_cpytxt2-0">`.

**Root cause**: The element that gets `class_cpytxt2-3` is rendered as a `<div>` with an image and text content. Python renders it as a `<p>` with image + text inside. The difference is in how the rendering pipeline handles elements with mixed image/text content — Python keeps them as a single `<p>`, Go splits them into separate elements.

**Python reference**: `yj_to_epub_content.py` lines 1294-1310 (inline render handling)

**Go file**: `internal/kfx/storyline.go` content rendering

**Fix steps**:
1. Identify the KFX content node that produces the logo + "FIRST EDITION" text
2. Trace how Python keeps it as a single `<p>` element vs Go splitting it
3. Fix the rendering to match Python's behavior for this case

---

### D4: Heading color missing

**Severity**: Low (cosmetic, only affects some heading styles)

**Affected**: Familiars

**Symptom**: Go's `heading_s2B3` and `heading_s3T` are missing `color: blue`. Python includes it.

**Root cause**: The KFX style fragment for these headings includes a color property that Go's `headingClass()` doesn't emit. The color property is likely filtered out during the heading style rendering.

**Python reference**: `yj_to_epub_content.py` heading rendering

**Go file**: `internal/kfx/storyline.go` `headingClass()`

**Fix steps**:
1. Check if `$569` (text color) is in the style fragment for these headings
2. Verify `headingClass()` preserves the color property in its CSS output
3. Check if `filterBodyDefaultDeclarations` strips the color

---

### D5: Font-weight class splitting

**Severity**: Low (cosmetic, different class count)

**Affected**: Familiars, Throne of Glass

**Symptom**: Python produces `class-2 {font-weight: normal}` and `class-3 {font-weight: 800; text-align: center}` as separate classes. Go produces `class-2 {font-weight: 800; text-align: center}` — missing the separate `font-weight: normal` class.

Also in Throne of Glass: Go produces `class_s2DN {font-weight: 800}` and `class_s2DP {font-weight: 800}` as two separate classes. Python deduplicates them into a single element with `<span>` wrapping.

**Root cause**: The rendering pipeline produces different style strings for elements that share the same visual style. Python's simplifyStyles deduplicates more aggressively.

**Fix**: Requires investigation of whether this is a rendering or simplification difference. Likely related to how `add_composite_and_equivalent_styles` works in Python (D8).

---

### D6: `class_s78S`/`class_s78V` composite style merging

**Severity**: Low (functionally equivalent)

**Affected**: Hunger Games

**Symptom**: Go has two separate classes:
```
.class_s78S-0 {text-align: center}
.class_s78S-1 {max-width: 70%; width: 70%}
.class_s78V {padding-bottom: 0.0375em; ... vertical-align: middle}
```

Python has:
```
.class_s78S {max-width: 70%; width: 70%}
.class_s78V {padding-bottom: 0.0375em; ... text-align: center; vertical-align: middle}
```

Python merges `text-align: center` from the parent element into `class_s78V` and eliminates the redundant `class_s78S-0`.

**Python reference**: `yj_to_epub_properties.py` lines 1973-2021 (`add_composite_and_equivalent_styles`)

**Go file**: `internal/kfx/yj_to_epub_properties.go`

**Fix steps**:
1. Port `add_composite_and_equivalent_styles` as a new pass after `simplifyStylesElementFull`
2. The function processes each element and its children:
   - Merges composite side styles (e.g., `margin-left` + `margin-right` + `margin-top` + `margin-bottom` → `margin`)
   - Applies alternate equivalent properties (`writing-mode` → `-webkit-writing-mode`)
   - Removes ineffective properties for inline elements
   - Moves shared heritable properties up from children to parent (and removes from children)
3. The key logic: if a parent has a property and ALL children have the same value, move it to parent and remove from children

---

### D7: Missing `margin-bottom: 0; margin-top: 0` on paragraphs

**Severity**: Low (browsers default to this anyway)

**Affected**: Hunger Games (`class_s79F`)

**Symptom**: Go's `.class_s79F {font-size: 0.75em; line-height: 1; text-align: center}` vs Python's `.class_s79F {font-size: 0.75em; line-height: 1; margin-bottom: 0; margin-top: 0; text-align: center}`.

**Root cause**: `margin-top: 0` and `margin-bottom: 0` are non-heritable defaults. For div/p/h elements, Go adds them to `explicitStyle` at line 1567 of `yj_to_epub_properties.go`. But `class_s79F` comes from a style that is applied to a `<p>` element. The margins should be present in the explicit style and survive stripping because the paragraph's inherited margins are `1em` (from paragraph conversion).

If the margins are missing, it means either:
1. The element isn't recognized as needing non-heritable defaults (not a div/p/h), or
2. The margins are being stripped by comparison against inherited `0` (from non-heritable defaults)

**Fix**: Debug why this specific class loses its margins. Check if the element tag is correctly set to `<p>` before the non-heritable defaults are added.

---

### D8: Figure class properties

**Severity**: Low

**Affected**: Throne of Glass

**Symptom**: Go produces `.figure_s2C {font-size: 1.04384em; margin-left: auto; margin-right: auto; text-transform: none; width: 30%}` while Python adds `font-style: normal; font-weight: normal; margin-bottom: 0; margin-top: 0`.

Also, Python has separate `.figure_s1J`, `.figure_s3TJ`, `.figure_s3U5`, `.figure_s4N` classes that Go doesn't produce.

**Root cause**: Python's figure rendering adds additional properties from the figure's style that Go doesn't include. The extra figure classes correspond to figure wrapper styles that Go merges differently.

**Fix**: Check how figure styles are built in Go vs Python and add the missing properties.

---

### D9: JXR image not converted

**Severity**: Medium (broken images)

**Affected**: Throne of Glass

**Symptom**: Go's `content.opf` references `image_rsrc43M.jxr` (media-type: image/jxr) while Python has `image_rsrc43M.jpg` and an additional `image_rsrc43N.jpg`. The CSS `background-image: eAV` in Go vs `background-image: url(image_rsrc43N.jpg)` in Python shows the image reference is also broken.

**Root cause**: The JXR decoder exists in `internal/jxr/` but isn't wired into the EPUB resource pipeline. When a `.jxr` image is encountered during conversion, it should be decoded to JPEG and the references updated.

**Fix steps**:
1. Wire `internal/jxr/` decoder into the resource pipeline in `internal/kfx/`
2. In `processResources()` or wherever images are handled, detect `.jxr` extension
3. Decode JXR → JPEG
4. Replace the resource in the EPUB with the JPEG version
5. Update all references (CSS `background-image`, HTML `<img src>`, manifest)

---

### D10: Heading `font-weight` missing in CSS

**Severity**: Low (cosmetic)

**Affected**: Throne of Glass

**Symptom**: Go produces `.heading_s20 {-webkit-hyphens: none; font-size: 3.34029em; font-weight: bold; ...}` but Python produces `.heading_s20 {-webkit-hyphens: none; font-size: 3.34029em; ...}` (no `font-weight: bold`). Conversely, some Go headings that omit `font-weight: bold` should include it.

**Root cause**: Default stripping inconsistency. `font-weight: bold` is the non-default value (default is `normal`). If the heading's style has `font-weight: bold` but the inherited value from the body is also `bold` (because body style includes `font-weight: normal` → not stripped → heading converts div→h1 → inherited pops font-weight → heading's bold doesn't match inherited → kept).

This is complex. Python's heading conversion pops `font-weight` from inherited (line 1922), which means the heading's own `font-weight: bold` doesn't match the empty inherited → kept. But if the heading's `font-weight` is `normal` (matching the default), and inherited was popped (empty), then `sty["font-weight"]` = "normal" ≠ "" → kept. This produces `font-weight: normal` in the CSS which Python then strips via some other mechanism.

The inverse pattern: Go includes `font-weight: bold` on headings where Python doesn't. This suggests the heading's font-weight was inherited from the body (not explicit) and should have been stripped.

**Fix**: Requires careful tracing of the inherited map flow through heading conversion for each affected heading style.

---

### D11: `toc.ncx` extra `xmlns:mbp` namespace

**Severity**: Trivial

**Affected**: All books

**Fix**: Remove `xmlns:mbp` from the NCX `<ncx>` element in `internal/epub/epub.go` line 248. Python never emits this namespace.

### D12: Missing trailing newline in `content.opf`

**Severity**: Trivial

**Affected**: All books

**Fix**: Add `\n` after `</package>` in `internal/epub/epub.go` OPF generation.

---

## Implementation Order

Ordered by impact (files fixed per effort unit):

### Phase 1: Trivial fixes (30 min)
1. **D11**: Remove `xmlns:mbp` from NCX
2. **D12**: Add trailing newline to content.opf

### Phase 2: text-indent: 0 (D1) ✅ DONE
3. ✅ Removed `$36` (text-indent) from body style during rendering
4. ✅ Flattened body children for reverse inheritance
5. ✅ Removed filterBodyDefaultDeclarations from rendering pipeline
6. Verified with: All books improved. Hunger Games 25→105 match.

### Phase 3: Non-heritable defaults for all block elements (2-3 hours)
6. **D7**: Fix missing `margin-bottom: 0; margin-top: 0` on paragraphs
7. Audit all block-level elements to ensure non-heritable defaults are added and stripped correctly

### Phase 4: Composite style merging (3-4 hours)
8. **D6**: Port `add_composite_and_equivalent_styles` from Python
9. This should also fix D5 (font-weight splitting) since composite merging deduplicates shared child properties

### Phase 5: Table and content structure (2-3 hours)
10. **D2**: Add `<p class="tableright">` wrappers in table cells
11. **D3**: Fix logo + "FIRST EDITION" rendering to keep as single `<p>`

### Phase 6: Heading properties (2-3 hours)
12. **D4**: Fix missing heading color
13. **D10**: Fix heading font-weight default stripping

### Phase 7: Figure classes (2-3 hours)
14. **D8**: Fix figure class properties to match Python

### Phase 8: JXR images (4-8 hours)
15. **D9**: Wire JXR decoder into EPUB resource pipeline

---

## Verification

After each phase, run the full comparison script against all 6 books. The target is:

| Book | Target |
|------|--------|
| Martyr | 102/102 match |
| Three Below | 23/23 match |
| Elvis | 32/32 match |
| Familiars | 54/54 match |
| Hunger Games | 112/112 match |
| Throne of Glass | 71/71 match |

Note: Throne of Glass image count may differ (D9 JXR conversion may add/remove images). The "match" count refers to text-only files (HTML, CSS, OPF, NCX, nav).

---

## Key Python Files to Port

| Python File | Go Target | Purpose |
|------------|-----------|---------|
| `yj_to_epub_content.py:42-68` | `storyline.go` | Body style inference |
| `yj_to_epub_content.py:1279-1310` | `storyline.go` | Table cell `<p>` wrapping |
| `yj_to_epub_content.py:1324-1345` | `storyline.go` | Image wrapper property partition |
| `yj_to_epub_properties.py:1973-2021` | `yj_to_epub_properties.go` | `add_composite_and_equivalent_styles` |
| `yj_to_epub_properties.py:1655-1670` | `yj_to_epub_properties.go` | Body default font/line-height/text-indent injection |
| `yj_to_epub_properties.py:1830` | `yj_to_epub_properties.go` | Non-heritable defaults for divs |
| `yj_to_epub_properties.py:1919-1924` | `yj_to_epub_properties.go` | Heading inherited property popping |
