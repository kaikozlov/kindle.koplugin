# PROMPT: Fix All Remaining Diffs — Python 1:1 Parity

## State

- Commit HEAD: `d9e3b77` (Elvis table cell `<p>` wrappers + logo FIRST EDITION text)
- All tests pass: `go test ./internal/kfx/ -count=1 -timeout 120s` is green
- Parity: **387/394 text files match, 7 differ** across 6 books
- 4 books perfect (Martyr 102/0, Three Below 23/0, Familiars 54/0, TOG 71/0)
- 2 books have diffs: Elvis 31/1, Hunger Games 106/6

## CRITICAL RULES

1. **Python source is the ONLY truth.** Never use book output comparison as your debugging tool. Trace the exact Python code path, understand every branch, then match it 1:1 in Go. Map Python file+line → Go file+line.
2. **Commit after every step** — success OR failure. Never accumulate more than one step of uncommitted changes.
3. **If a change introduces unexpected test failures or unexpected diffs, revert immediately**, commit the revert, then figure out why.
4. **Martyr must remain byte-identical.** Any change that breaks Martyr parity must be reverted.
5. **DO NOT be lazy.** Do not handwave a diff as "pre-existing" or "a deeper issue." Every single diff represents a failure to replicate Python logic. Fix them all.

## The 7 Remaining Diffs

### Diff A: Elvis `stylesheet.css` — Extra `font-size: 1em` in `.class_tableright`

```
Python: .class_tableright {margin-bottom: 0; margin-top: 0}
Go:     .class_tableright {font-size: 1em; margin-bottom: 0; margin-top: 0}
```

**Context:** Our previous session created `<p class="class_tableright">` for Elvis right-column table cells. The `paragraphClass` function includes `font-size: 1em` from the tableright style fragment's `$34: $61` (default font-size). Python's tableright class does NOT include font-size.

**Python path to trace:**
1. `yj_to_epub_properties.py` `simplify_styles` (line 1677+): The stripping loop at line 1966-1968 removes any property matching inherited value. For a `<p>` inside a `<td>` with `font-size: 0.625em`, the inherited font-size is normalized to `1em` (line 1679). The `<p>`'s font-size is also `1em` from the style fragment. They match → stripped.
2. `yj_to_epub_properties.py` `orig_sty` capture (line 1682): Captured AFTER the normalization at line 1679, so `orig_sty["font-size"]` = `"1em"`.
3. `yj_to_epub_properties.py` `parent_sty` (line 1849): `parent_sty["font-size"] = orig_sty["font-size"]` = `"1em"`. Children inherit `"1em"`.
4. Trace the full simplify_styles chain for: `<table>` → `<tbody>` → `<tr>` → `<td class="class_tableleft-0">` → `<p class="class_tableright">`. What is `inherited["font-size"]` at each level? Where does the `font-size: 1em` on the `<p>` come from in Go but not Python?

**Go files:**
- `internal/kfx/yj_to_epub_properties.go` `simplifyStylesElementFull` (line 1377+)
- `internal/kfx/storyline.go` `paragraphClass` (line 1972)

**Key question:** Why does Python strip `font-size: 1em` from the tableright `<p>` but Go doesn't? The normalize-to-1em logic exists in both. Is the inherited chain correct? Is `origFontSize` being propagated correctly?

### Diff B: Hunger Games `c76J.xhtml` — Image resource selection (different filenames)

```
Python: image_rsrc7G8.jpg, image_rsrc7GB.jpg, image_rsrc7GE.jpg, image_rsrc7GH.jpg, image_rsrc7GM.jpg, image_rsrc7GR.jpg
Go:     image_rsrc7G7.jpg, image_rsrc7GA.jpg, image_rsrc7GD.jpg, image_rsrc7GG.jpg, image_rsrc7GK.jpg, image_rsrc7GP.jpg
```

**Context:** Each image in this section has `$635` variant references. Python processes these in `get_external_resource` and replaces the base image with the highest-resolution variant.

**Python path to trace:**
1. `yj_to_epub_resources.py` `get_external_resource` (line 35): Called when processing `$164`/$175 references.
2. `yj_to_epub_resources.py` lines 162-172: `for rr in resource.pop("$635", []):` iterates variants, replaces if `USE_HIGHEST_RESOLUTION_IMAGE_VARIANT` and variant is larger.
3. Trace how `$635` variant references are stored in the KFX fragment data and how Go processes them.

**Go files:**
- `internal/kfx/yj_to_epub_resources.go` — search for `$635` or variant handling
- `internal/kfx/state.go` — where fragments/resources are loaded

**Key question:** Does Go process `$635` image variants at all? If not, implement it exactly matching Python lines 162-172.

### Diff C: Hunger Games `c76J.xhtml` — Missing `<a>` links on table cell images

```
Python: <td class="class_s772-0"><a href="https://itunes.apple.com/us/book/the-hunger-games/id739209702?mt=11"><img .../></a></td>
Go:     <td class="class_s772-0"><div><img .../></div></td>
```

**Context:** The $271 image nodes in these table cells have `$179` properties (link targets). Python wraps the image in `<a href="...">`. Go doesn't.

**Python path to trace:**
1. `yj_to_epub_content.py` `process_content` (line 1261): `link_to = content.pop("$179", None)`
2. `yj_to_epub_content.py` lines 1282-1286: `if link_to is not None: container_elem, container_style = self.create_container(content_elem, content_style, "a", LINK_CONTAINER_PROPERTIES)`
3. This happens BEFORE the render mode check (`render == "$283"`) and BEFORE the image container creation (`create_container(content_elem, content_style, "div", BLOCK_CONTAINER_PROPERTIES)`)
4. So the wrapping order is: first wrap in `<a>` (if $179), then wrap in `<div>` (if non-inline), then the image goes inside.

**Go files:**
- `internal/kfx/storyline.go` `renderNode` (line 262): calls `wrapNodeLink` which handles `$179` on the node
- `internal/kfx/storyline.go` `renderTableCell` (line 792): uses `renderContentChild` which calls `renderNode`
- The issue: in `renderTableCell`, the $271 child goes through `renderContentChild` → `renderNode`. `renderNode` falls through to `renderImageNode` which returns the image. Then `renderNode` calls `wrapNodeLink`. BUT `renderContentChild` calls `renderNode(child, depth)` — the child is the raw interface{} value, and `renderNode` first does `asMap(raw)` — does it have `$179`?

**Key question:** Is `$179` on the $271 image node itself, or on the $269 parent cell node? If on the $271, `wrapNodeLink` should catch it. If it's being consumed/lost during `prepareRenderableNode`, that's the bug. Trace exactly what Python's `content.pop("$179")` does vs what Go does with `$179`.

### Diff D: Hunger Games `c76J.xhtml` — Extra `<div>` wrapper around table cell images

```
Python: <td class="class_s772-0"><a href="..."><img .../></a></td>
Go:     <td class="class_s772-0"><div><img .../></div></td>
```

**Context:** Python's $271 handler creates `<img>`, then wraps in `<a>` (if $179), then wraps in `<div>` (if non-inline render, via `create_container` with `BLOCK_CONTAINER_PROPERTIES`). But for these images, the `<div>` wrapper's style gets merged with the `<td>` style via COMBINE_NESTED_DIVS, so the `<div>` disappears.

**Python path to trace:**
1. `yj_to_epub_content.py` lines 1324-1330: For non-inline images, `create_container(content_elem, content_style, "div", BLOCK_CONTAINER_PROPERTIES)` wraps `<img>` in `<div>`.
2. Then COMBINE_NESTED_DIVS at lines 1408-1448 may merge the `<div>` wrapper with the parent `<div>` that gets retagged to `<td>`.
3. If no CSS overlap → merge → wrapper disappears. If overlap → stays.

**Go files:**
- `internal/kfx/storyline.go` `renderImageNode` (line 908): always wraps in `<div>` when `imageClasses` returns a wrapperClass.
- `internal/kfx/storyline.go` `renderTableCell` (line 792): has COMBINE_NESTED_DIVS logic via `tableCellCombineNestedDivs` and `tableCellClass`.

**Key question:** For the HG table cell images, does Python merge the image wrapper `<div>` with the cell `<td>`? If so, the `<div>` should disappear. Go's `renderTableCell` handles COMBINE_NESTED_DIVS for $269 children with $145 text, but NOT for $271 image children with wrapper `<div>`s. The COMBINE_NESTED_DIVS logic needs to also handle the image wrapper case.

### Diff E: Hunger Games `c791.xhtml` — Body structure: images not in `<p>`, wrong body class

```
Python: <body class="class-1"><p class="class_s79F"><img .../> <img .../></p></body>
Go:     <body class="class_s79F"><img .../><img .../></body>
```

**Context:** This section has a storyline with a single top-level $269 node containing two $271 images. Python promotes this to body: the outer `<div>` becomes `<body>`, and simplify_styles converts the `<div>` to `<p>` (contains text+images, no block children → `<p>`).

**Python path to trace:**
1. `yj_to_epub_content.py` `process_content` for `$269` (line 428): creates `<div>`, calls `add_content`. The $146 children are two $271 images → `<img>` elements.
2. `yj_to_epub_content.py` `process_content` top-level (line 1445): `content_elem.tag = "body"`. The `<div>` becomes `<body>`.
3. Then the $269's `<div>` had `class="class_s79F"` style. After body promotion, this style becomes the body style. But Python's body class is `class-1`, not `class_s79F`. So something strips or replaces the style.
4. `yj_to_epub_properties.py` simplify_styles: the `<body>` tag with `class_s79F` properties — check what happens. The body tag itself doesn't go through simplify_styles in the same way.
5. Actually: Python promotes the SINGLE child div to body. The child's content (two images) stays inside. Then simplify_styles converts the child `<div>` (now inside `<body>`) that has images but no block children → `<p>`. Wait, the `<body>` IS the promoted `<div>`. The images are direct children of `<body>`.
6. Trace exactly: Python creates `<div class="s79F"><img/><img/></div>`. Then line 1450: `content_elem.tag = "body"`. So `<body class="s79F"><img/><img/></body>`. Then simplify_styles processes this: `<body>` contains images, has text=false, contains_image=true. But body doesn't get converted to `<p>`. 
7. Wait — Python's output has `<body class="class-1"><p class="class_s79F">...`. So the body has `class-1` (a body-level class) and there's a `<p>` inside. This means Python DIDN'T promote the div to body. Instead, the body was created separately and the div became `<p>` via simplify_styles.

**Re-trace more carefully:**
1. `process_section` line 205: `self.process_content(page_templates[-1], book_part.html, book_part, ...)` — processes the page template, not the section directly.
2. The page template is a `$270` (fitted container) that wraps the $269 with the two images.
3. The $270 handler creates `<div>`, calls `add_content` which processes $146 → $269 → creates `<div class="s79F"><img/><img/></div>`.
4. The $269 div is inside the $270 div. COMBINE_NESTED_DIVS may merge them.
5. Then `is_top_level = parent.tag == HTML` → True. Line 1447: `content_elem.tag = "body"`.
6. So the result is `<body class="s79F"><img/><img/></body>`.
7. Then simplify_styles on `<body>`: body contains images, no block elem. But body doesn't get `<p>` promotion.
8. But Python output has `<body class="class-1"><p class="class_s79F">...`. So something wraps the images in `<p>`.
9. This must be happening during simplify_styles: the body element's children (two `<img>` elements) — does simplify_styles wrap them? No, simplify_styles only modifies existing elements.
10. Actually, re-read Python's `process_content` for `$269`: it creates `<div>`, calls `add_content`. The $146 list has two $271 children. Each $271 creates `<img>` and goes through the container/link/render processing. For non-inline images (no $601=$283), Python wraps each `<img>` in a `<div>` via `create_container`. So the $269 div has two `<div><img/></div>` children.
11. Then COMBINE_NESTED_DIVS: the $269 div has two children, so it doesn't merge (requires len == 1).
12. The $269 div becomes body. Two `<div><img/></div>` inside body.
13. simplify_styles on each inner `<div>`: contains_image=true, contains_text=false, contains_block=false. The `<div>` doesn't match `contains_text and not contains_block` → stays as `<div>`.
14. But Python output has `<p>` not `<div>`. So something converts `<div>` → `<p>` for image-only divs.
15. WAIT — re-check: Python simplify_styles line 1920: `if "heading" in kfx_layout_hints and not contains_block_elem:` → figure/heading checks. Line 1935: `elif "figure" in kfx_layout_hints and contains_image and not self.epub2_desired:` → `<figure>`. Line 1941: `elif contains_text and not contains_block_elem:` → `<p>`.
16. The inner divs have contains_text=false (images have no text). So they DON'T get promoted to `<p>`.
17. BUT Python output has `<p>`. So my analysis is wrong somewhere. TRACE THE ACTUAL PYTHON CODE for c791 section.

**Go files:**
- `internal/kfx/storyline.go` `promotedBodyContainer` (line 2095): requires `len(nodes) == 1` — if there are multiple top-level nodes, no promotion.
- `internal/kfx/storyline.go` `renderStoryline` (line 43): body promotion logic.
- `internal/kfx/yj_to_epub_properties.go` `simplifyStylesElementFull` (line 1377): div→p promotion.

**Key question:** Trace c791 through Python step by step. What is the KFX storyline structure? How does Python render it to get `<body class="class-1"><p class="class_s79F"><img/><img/></p></body>`? Then match that in Go.

### Diff F: Hunger Games `content.opf` — Different image resource references

This is a CONSEQUENCE of Diff B (image resource selection). Fix Diff B and this resolves automatically.

## How To Work

1. **Pick one diff at a time.** Start with the simplest (Diff A or C).
2. **Trace the Python code FIRST.** Read the exact Python lines. Understand every branch. Document: "Python line X does Y because Z."
3. **Find the Go code that should match.** Identify where Go diverges.
4. **Implement the fix.** Match Python's behavior exactly.
5. **Test:** `go test ./internal/kfx/ -count=1 -timeout 120s` must pass.
6. **Verify:** Run `bash /tmp/compare_all.sh` to confirm the diff is resolved and no regressions.
7. **Commit.** Every step gets its own commit.

## Suggested Order

1. **Diff A** (Elvis stylesheet `font-size: 1em`) — Most isolated, pure simplify_styles issue.
2. **Diff C** (HG missing `<a>` links) — Clear Python path, `$179` handling.
3. **Diff D** (HG extra `<div>` wrapper) — Related to image container + COMBINE_NESTED_DIVS in table cells.
4. **Diff B** (HG image resource selection) — Requires implementing `$635` variant handling.
5. **Diff E** (HG c791 body structure) — Most complex, body promotion + simplify_styles interaction.

## Key Python Files

| File | Purpose |
|------|---------|
| `REFERENCE/Calibre_KFX_Input/kfxlib/yj_to_epub_content.py` | Content rendering (process_content, add_content, $269/$271/$279 handlers, COMBINE_NESTED_DIVS, link wrapping, body promotion) |
| `REFERENCE/Calibre_KFX_Input/kfxlib/yj_to_epub_properties.py` | Style processing (simplify_styles, orig_sty/parent_sty, font-size normalization, stripping loop) |
| `REFERENCE/Calibre_KFX_Input/kfxlib/yj_to_epub_resources.py` | Resource handling ($635 variants, get_external_resource) |
| `REFERENCE/Calibre_KFX_Input/kfxlib/epub_output.py` | Post-processing (strip empty spans/divs, beautify_html) |

## Key Go Files

| File | Purpose |
|------|---------|
| `internal/kfx/storyline.go` | Rendering (renderNode, renderImageNode, renderTableCell, wrapNodeLink, renderStoryline, promotedBodyContainer, tableCellCombineNestedDivs, applyContainerStyleEvents) |
| `internal/kfx/yj_to_epub_properties.go` | Style processing (simplifyStylesElementFull, fixupStylesAndClasses, parentStyle propagation, origFontSize) |
| `internal/kfx/render.go` | Pipeline (renderBookState, processReadingOrder) |
| `internal/kfx/yj_to_epub_resources.go` | Resource handling |
| `internal/kfx/state.go` | Fragment loading and organization |

## Fixtures

| What | Path |
|------|------|
| Elvis decrypted KFX | `REFERENCE/kfx_new/decrypted/Elvis and the Underdogs_B009NG3090_decrypted.kfx-zip` |
| Elvis Python EPUB | `REFERENCE/kfx_new/calibre_epubs/Elvis and the Underdogs_B009NG3090_decrypted.epub` |
| Hunger Games decrypted KFX | `REFERENCE/kfx_new/decrypted/The Hunger Games Trilogy_B004XJRQUQ_decrypted.kfx-zip` |
| Hunger Games Python EPUB | `REFERENCE/kfx_new/calibre_epubs/The Hunger Games Trilogy_B004XJRQUQ_decrypted.epub` |
| Martyr (regression guard) | `REFERENCE/kfx_examples/Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx` |
| Martyr Python EPUB | `REFERENCE/martyr_calibre.epub` |
| Comparison script | `bash /tmp/compare_all.sh` |
