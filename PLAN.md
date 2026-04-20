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

### Key Architectural Finding: Reverse Inheritance Ordering

**Root cause of most remaining diffs.**

Go's `simplifyStylesElementFull` (in `internal/kfx/yj_to_epub_properties.go`) processes children BEFORE reverse inheritance, while Python processes children AFTER:

**Go order (current):**
1. Build `parentStyle` from `sty` (before reverse inheritance)
2. Process children with `parentStyle` (they don't get reverse-promoted properties)
3. `applyReverseInheritance(childElements, sty)` — promotes shared heritable properties from children to parent

**Python order (correct):**
1. `apply_reverse_inheritance` — promotes shared heritable properties from children to parent
2. `compute_inherited(sty)` — builds inherited from the UPDATED sty (with reverse-promoted properties)
3. Process children with updated inherited

**Impact**: Children don't inherit reverse-promoted properties from the parent. This causes:
- Body-level `hyphens: none`, `font-weight: bold` not propagating to headings → headings keep properties that should be stripped
- Body-level `text-indent` not propagating to some elements → missing `text-indent: 0` on tables
- Font-weight not propagating from bold spans to their paragraph → font-weight class splitting

**Why not just reorder?** Attempted reordering in `simplifyStylesElementFull`. The fix works for Throne of Glass but breaks Martyr tests. The issue is that `applyReverseInheritance` reads children's style attributes. In the original order, children have been through simplify_styles (their styles are cleaned up). In the reordered version, children have original rendering styles (which include extra properties). This causes reverse inheritance to see different property values and produce different promotion results.

**Proper fix** (not yet implemented):
- Reorder: collect childElements → run reverse inheritance → compute parentStyle → process children
- Update Martyr test expectations to match Python's output (which is the correct behavior)
- OR: make reverse inheritance work correctly on original rendering styles (match Python's behavior exactly)

**Go file**: `internal/kfx/yj_to_epub_properties.go` lines 1486-1540 (the parentStyle/childElements/reverseInheritance block)

**Python reference**: `yj_to_epub_properties.py` lines 1876-1920

### Remaining Defects (all trace to ordering or structural rendering)

#### D1-ORDERING: Reverse inheritance ordering (described above)
**Affected**: All 4 remaining books (Throne of Glass headings, Hunger Games tables, Familiars font-weight)
**Fix**: Reorder parentStyle computation after reverse inheritance, update Martyr tests

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
**Root cause**: Rendering pipeline produces different HTML structure. Python promotes bold spans' content to paragraph level and wraps non-bold text in new spans. Go keeps original span structure.
**Go file**: `internal/kfx/storyline.go` text event rendering + `internal/kfx/yj_to_epub_properties.go` simplify_styles

#### D7: Missing `margin-bottom: 0; margin-top: 0` on paragraphs
**Affected**: Hunger Games (class_s79F)
**Root cause**: Related to D1-ORDERING — text-align is reverse-inherited but margins aren't

#### D8: Figure class properties
**Affected**: Throne of Glass (figure_s2C missing font-style/font-weight/margin defaults)
**Root cause**: Figure elements don't get non-heritable defaults the same way Python does

#### D9: JXR images
**Affected**: Throne of Glass (4 files)
**Root cause**: JXR decoder exists in `internal/jxr/` but not wired into EPUB resource pipeline

#### D10: Heading font-weight/hyphens
**Affected**: Throne of Glass (headings s20/s22/s24 keep font-weight: bold, heading_sF keeps -webkit-hyphens)
**Root cause**: D1-ORDERING — body has these properties via reverse inheritance but children don't inherit them

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

### Critical: Fix reverse inheritance ordering (D1-ORDERING)
This is the single highest-impact fix. It would resolve or improve:
- Throne of Glass heading diffs (font-weight, hyphens)
- Hunger Games text-indent on tables
- Familiars font-weight class splitting

**Implementation**:
1. In `simplifyStylesElementFull`, move the `applyReverseInheritance` call before `parentStyle` computation
2. Update `TestConvertFilePhase8MatchesInlineStyleEventsAndFitWidthContainers` and `TestConvertFileMatchesReferenceStructureIgnoringImages` expectations to match Python's output
3. Run full comparison against all 6 books
4. If Martyr no longer matches Python's output byte-for-byte, investigate whether the test expectations were correct

### Then: Figure property defaults (D8)
After ordering fix, check if figure properties are resolved. If not, investigate figure rendering.

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
