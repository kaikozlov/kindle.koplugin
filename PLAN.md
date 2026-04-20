# CSS Class Parity Plan

## Current Status (2026-04-20)

| Book | Match | Differ | Summary |
|------|-------|--------|---------|
| **Martyr** | 102 | 0 | ✅ Perfect match |
| **Three Below** | 23 | 0 | ✅ Perfect match |
| **Hunger Games** | 105 | 7 | Table wrappers, composite styles, text-indent |
| **Throne of Glass** | 67 | 4 | JXR images, heading font-weight/hyphens, figure properties |
| **Elvis** | 30 | 2 | Missing `<p class="tableright">` wrappers; extra `class_cpytxt2-3` |
| **Familiars** | 49 | 5 | Font-weight class splitting, composite styles |

**Total: 341 match, 25 differ** (up from 258 match / 105 differ at start)

### Commits Made
1. `6fb90c0` — Fix trailing newline in content.opf (D12) ✅
2. `2f9d6ec` — Enable reverse inheritance for text-indent and body style parity (D1) ✅ **THE BIG WIN: Hunger Games 25→105**
3. `674230c` — Resolve link color for heading `<a>` children (D4) ✅
4. `727ebe8` — Rename `class_` to `figure_` for figure elements (D8) ✅ **Throne of Glass 64→67**
5. `92a8532` — Strip vendor-prefixed alternate equivalents in simplify_styles ✅
6. `46c8217` — Update PLAN.md with session summary

### Key Architectural Finding: Body Style Inheritance

**Root cause of most remaining diffs.**

Through extensive Python debugging (monkey-patching `simplify_styles`, `set_style`, and `add_composite_and_equivalent_styles`), the true root cause was identified:

**Python's `<body>` element has NO inline `style` attribute when `simplify_styles` runs.** Go's body element has the full section-specific body style (font-weight, text-align, text-transform, hyphens, etc.) as its inline `style` attribute.

**Python's body flow** (confirmed by debugging):
1. Body inline style: empty (no `style` attribute, `class=""`)
2. `sty = inherited_properties.copy()` → heritable defaults (font-weight: normal, etc.)
3. All defaults stripped in stripping loop (match inherited)
4. Children processed with `parentStyle` = heritable defaults only
5. Reverse inheritance promotes shared properties from children → body gets font-weight: bold, text-align: center, etc.
6. Body's CSS class generated from final style (includes reverse-inherited properties)

**Go's body flow** (current):
1. Body inline style: full section style (font-weight: bold; text-align: center; text-transform: uppercase)
2. Children inherit font-weight: bold, text-align: center via `parentStyle`
3. Reverse inheritance sees body already has these properties → different promotion decisions
4. Body's CSS class includes properties from both rendering pipeline AND reverse inheritance

**Impact on Throne of Glass heading font-weight:**
- Go: body has font-weight: bold → headings inherit bold → heading conversion pops font-weight from comparisonInherited → bold ≠ "" → NOT stripped → heading CSS has font-weight: bold
- Python: body has no font-weight initially → headings inherit normal → heading's explicit bold ≠ inherited normal → NOT stripped → heading CSS has font-weight: bold → BUT body's reverse inheritance then promotes font-weight (4/5 children have bold = 80%) and REMOVES it from all children → heading CSS ends up with NO font-weight

**Python debug evidence** (c1U section containing heading_s20):
```
set_style #1: heading s20 fw='bold' (heading's own simplify_styles)
set_style #2: heading s20 fw='(not set)' (body's reverse inheritance, line 1915)
set_style #3: heading s20 fw='(not set)' (add_composite_and_equivalent_styles)
```

**Why not just remove body inline style?** Attempted. Martyr regresses from 102/0 to 10/92 because:
- Without body inline style, all body properties come from reverse inheritance
- Reverse inheritance produces different results because children's post-simplify styles differ between Go and Python
- Specifically: Go's paragraph rendering (paragraphClass) produces different styles than Python's YJ→EPUB content conversion
- This causes class numbering to change completely

**Go file**: `internal/kfx/yj_to_epub_properties.go` lines 800-830 (body element creation)
**Python reference**: `yj_to_epub_properties.py` line 1404 (`simplify_styles(book_part.body(), ...)`)

### Remaining Defects

#### D1-BODY: Body style inheritance (described above)
**Affected**: Throne of Glass (heading font-weight/hyphens), Hunger Games (text-indent on tables), Familiars (font-weight splitting)
**Fix**: Either (a) restructure body element creation to match Python (no inline style) + fix children's styles, or (b) add post-simplify_styles reverse inheritance pass that mimics Python's body reverse inheritance behavior

#### D2: Missing `<p class="tableright">` wrappers in table cells
**Affected**: Elvis (1 file + stylesheet)
**Root cause**: Python wraps `<td>` text content in `<p class="class_tableright">` elements. Go leaves text bare in `<td>`.
**Python reference**: `yj_to_epub_content.py` lines 816-830
**Go file**: `internal/kfx/storyline.go` table rendering

#### D3: Logo+text rendering
**Affected**: Elvis (1 file)
**Root cause**: Go splits image+text into separate elements. Python keeps as single `<p>`.
**Python reference**: `yj_to_epub_content.py` lines 1294-1310
**Go file**: `internal/kfx/storyline.go` content rendering

#### D5: Font-weight class splitting
**Affected**: Familiars (5 files)
**Root cause**: D1-BODY — body has font-weight in Go but not in Python during simplify_styles. Go's children inherit font-weight from body, changing how reverse inheritance counts and strips properties.
**Go file**: `internal/kfx/yj_to_epub_properties.go` body element creation

#### D7: Missing `margin-bottom: 0; margin-top: 0` on paragraphs
**Affected**: Hunger Games (class_s79F)
**Root cause**: Related to D1-BODY

#### D8: Figure class properties
**Affected**: Throne of Glass (figure_s2C missing font-style/font-weight/margin defaults)
**Root cause**: Figure elements don't get non-heritable defaults the same way Python does

#### D9: JXR images
**Affected**: Throne of Glass (4 files)
**Root cause**: JXR decoder exists in `internal/jxr/` but not wired into EPUB resource pipeline

#### D10: Heading font-weight/hyphens
**Affected**: Throne of Glass (headings s20/s22/s24 keep font-weight: bold, heading_sF keeps -webkit-hyphens)
**Root cause**: D1-BODY — body has these properties in Go, preventing reverse inheritance from stripping them from headings

---

## Implementation History

### Phase 1: Trivial fixes ✅
1. D11 (xmlns:mbp) — Already fixed before this plan
2. D12 (trailing newline) — Fixed in `6fb90c0`

### Phase 2: text-indent: 0 (D1) ✅
3. Removed `$36` (text-indent) from body style during rendering — `2f9d6ec`
4. Flattened body children for reverse inheritance — `2f9d6ec`
5. Removed `filterBodyDefaultDeclarations` from rendering pipeline — `2f9d6ec`
6. **Result**: Hunger Games 25→105 match (+80 files)

### Phase 3: Non-heritable defaults (D7) ✅
7. Already working for most cases. The remaining D7 cases are caused by D1-ORDERING.

### Phase 4: Composite style merging (D6, D5) ✅ (partial)
8. Added vendor-prefixed alternate equivalent stripping — `92a8532`
9. D5/D6 require D1-ORDERING fix to fully resolve

### Phase 5: Table and content structure (D2, D3) — Deferred
10. D2/D3 require deeper rendering pipeline investigation

### Phase 6: Heading properties (D4, D10) ✅ (partial)
11. D4 heading color — Fixed in `674230c`
12. D10 heading font-weight/hyphens — Requires D1-ORDERING fix

### Phase 7: Figure classes (D8) ✅ (partial)
13. Figure class naming (`class_` → `figure_`) — Fixed in `727ebe8`
14. Figure property defaults (margin, font-style, font-weight) — Still missing

### Phase 8: JXR images (D9) — Deferred
15. JXR decoder exists but not wired into EPUB pipeline

---

## Next Steps

### Critical: Fix body style inheritance (D1-BODY)
This is the single highest-impact fix. Two approaches:

**Approach A: Remove body inline style, fix children's styles**
1. Set body element's inline style to empty (matching Python)
2. Fix children's rendering styles to include properties currently inherited from body (e.g., paragraph s26 in TOG needs font-weight: bold)
3. Let reverse inheritance promote properties from children to body
4. Verify Martyr still works (may need to update test expectations)

**Approach B: Post-simplify reverse inheritance pass**
1. After simplify_styles on body, run a second reverse inheritance pass that only strips promoted properties from children (mimicking Python's behavior)
2. This avoids changing the body's initial style or children's rendering
3. May not fix all cases since the root cause is the body's parentStyle affecting children's stripping

**Key insight**: In Python's TOG c1U section, paragraph s26 has font-weight: bold (from its KFX style fragment). This makes 4/5 children bold (80%) → promoted. In Go, paragraph s26 doesn't have font-weight because `paragraphClass` doesn't include it. This is a rendering pipeline difference.

### Then: Figure property defaults (D8)
After body fix, check if figure properties are resolved. If not, investigate figure rendering.

### Then: JXR image wiring (D9)
Wire `internal/jxr/` decoder into EPUB resource pipeline.

### Then: Table p wrappers (D2) and logo rendering (D3)
These require deeper rendering pipeline changes.

---

## Key Python Files Reference

| Python File | Lines | Purpose |
|------------|-------|---------|
| `yj_to_epub_properties.py` | 1876-1920 | Reverse inheritance + simplify_styles order |
| `yj_to_epub_properties.py` | 1973-2021 | `add_composite_and_equivalent_styles` |
| `yj_to_epub_properties.py` | 1655-1670 | Body default injection |
| `yj_to_epub_content.py` | 42-68 | Body style inference |
| `yj_to_epub_content.py` | 816-830 | Table cell rendering |
| `yj_to_epub_content.py` | 1294-1310 | Inline render handling |
| `yj_to_epub_content.py:1324-1345` | `storyline.go` | Image wrapper property partition |
| `yj_to_epub_properties.py:1973-2021` | `yj_to_epub_properties.go` | `add_composite_and_equivalent_styles` |
| `yj_to_epub_properties.py:1655-1670` | `yj_to_epub_properties.go` | Body default font/line-height/text-indent injection |
| `yj_to_epub_properties.py:1830` | `yj_to_epub_properties.go` | Non-heritable defaults for divs |
| `yj_to_epub_properties.py:1919-1924` | `yj_to_epub_properties.go` | Heading inherited property popping |
