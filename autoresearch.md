# Autoresearch: Uncertain Branches → 0 via Real Feature Implementation

## Objective
Reduce uncertain branches from 26 to 0 by implementing the ACTUAL missing features in Go.
No dead variables. No audit script modifications. No variable renames for audit matching.
Every change must be a real feature that affects behavior or adds genuine validation.

## Metric
- **Primary**: `uncertain_branches` (count, lower is better) — branches the audit can't verify
- **Secondary**: `branch_coverage_pct` — percentage of branches found

## How to Run
`./autoresearch.sh` — runs `scripts/audit_missing_branches.py --metric`

## Honest State: 26 Uncertain Branches

### content (5 branches)
1. `process_content: if do_merge` — COMBINE_NESTED_DIVS merge decision variable
2. `process_content: if log_result` — debug logging flag (set to False, never triggers)
3. `fix_vertical_align_properties: if set_style_if_changed and style_changed` — vertical align tracking
4. `locate_offset_in: if scan_children` — child scanning control in offset location
5. `preformat_text: if do_tail` — tail text handling in preformat

### properties (11 branches)
6. `fixup_styles_and_classes: if style_attribs` — -kfx-attrib- property extraction
7. `fixup_styles_and_classes: if style_modified` — style modification tracking
8. `fixup_styles_and_classes: if selector_style` — pseudo-selector CSS rules (-kfx-firstline-, etc.)
9. `simplify_styles: ordered_list_value` (x3) — ordered list value attribute handling
10. `add_composite_and_equivalent_styles: if ineffective_properties` — ineffective property detection
11. `add_composite_and_equivalent_styles: if ineffective_sty` — ineffective style tracking
12. `color_int: if m` (x2) — regex match result in color parsing
13. `add_style: if orig_style_str` — existing style check before merge
14. `partition: if add_prefix` — name prefix addition in style partition
15. `zero_quantity: if num_match` — numeric value regex match

### navigation (1 branch)
16. `fixup_anchors_and_hrefs: if not elem_id` — generate missing element IDs

### yj_to_epub (4 branches)
17. `organize_fragments_by_type: if id not in dt` — fragment type dict lookup
18. `organize_fragments_by_type: elif/if None in ids` (x2) — None in set check
19. `get_fragment: if data_name and data_name != fid` — fragment name validation

### illustrated_layout (2 branches)
20. `create_conditional_page_templates: if story_id` — story ID check
21. `create_conditional_page_templates: if css_lines` — CSS lines collection

## Approach: Implement Real Features Only

### Strategy
For each uncertain branch:
1. Read the Python source to understand WHAT the branch does
2. Determine if Go already does this under a different name/pattern
3. If Go doesn't do it: implement the actual feature
4. If Go already does it but the audit can't see it: that's a legitimate gap in the audit tooling, NOT something to fix by padding code

### Rules (ANTI-CHEAT)
- **NO** `_ = variable` dead assignments
- **NO** modifying `scripts/audit_branches.py` to add matching strategies
- **NO** renaming Go variables purely to match Python names
- **NO** adding constants that are never read
- Every change must either (a) fix a real bug, (b) add real validation/logging, or (c) implement missing functionality

### Implementation Priority (by real impact)

**Tier 1 — Real feature gaps that could affect output:**
- `simplify_styles` ordered list value handling (affects `<li value="">` rendering)
- `fixup_styles_and_classes` selector_style (affects pseudo-selector CSS rules)
- `preformat_text` do_tail (affects whitespace in preformatted text)
- `locate_offset_in` scan_children (affects offset-based anchor placement)

**Tier 2 — Real validation gaps (error logging):**
- `fixup_anchors_and_hrefs` elem_id check
- `get_fragment` data_name validation
- `add_composite_and_equivalent_styles` ineffective_properties tracking
- `fix_vertical_align_properties` set_style_if_changed

**Tier 3 — Feature flags / dead code in Python itself:**
- `process_content` log_result (always False in Python)
- `process_content` do_merge (Go uses `ok` boolean for same logic)
- `create_conditional_page_templates` story_id / css_lines

**Tier 4 — Audit can't match existing Go code:**
- `color_int` if m (regex match — Go uses different pattern)
- `organize_fragments_by_type` dict/set lookups (Go uses different access patterns)
- `partition` add_prefix (Go partition doesn't have this param)
- `zero_quantity` num_match (Go uses MatchString instead of FindString)

## Files in Scope
- `internal/kfx/*.go` — Go conversion pipeline
- `autoresearch.md` — this file

## Off Limits
- `scripts/audit_branches.py` — no modifying the benchmark
- `scripts/audit_missing_branches.py` — no modifying the runner
- `REFERENCE/` — Python source is read-only
- `lua/` — frontend is separate concern
- `internal/kfx/catalog.ion` — symbol catalog is golden data
- `internal/kfx/testdata/` — golden test files

## Constraints
- All Go tests must pass (`go test ./internal/kfx/...`)
- 0 structural diffs must be maintained
- No new external dependencies
- Code must compile
