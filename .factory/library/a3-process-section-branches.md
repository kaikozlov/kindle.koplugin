# A3: process_section Branch Dispatch

## What was implemented

Port of `process_section` comic/magazine/fixed-layout branch dispatch from `yj_to_epub_content.py:105-208`.

## Key types and functions

### `bookType` (string enum)
- `bookTypeNone` — default, standard reflowable
- `bookTypeComic` — detected from content features (facing_page, double_page_spread, publisher_panels)
- `bookTypeChildren` — same dispatch as comic
- `bookTypeMagazine` — detected from CDE content type "MAGZ"
- `bookTypePrintReplica` — same dispatch as magazine

### `detectBookType(metadata, features)` → `bookType`
Examines `$258` metadata for `$251` (cde_content_type) and `$585` content features for `$590` array entries.

### `sectionBranch` (int enum)
- `branchScribePage` — nmdl.canvas_width → `processScribeNotebookPageSection`
- `branchScribeTemplate` — nmdl.template_type → `processScribeNotebookTemplateSection`
- `branchComic` — comic/children → `process_page_spread_page_template` (A4)
- `branchMagazine` — magazine/print-replica with conditional templates
- `branchReflowable` — default, uses `renderSectionFragments`

### `determineSectionBranch(section, bookType)` → `sectionBranch`
Dispatch priority (matches Python's if/elif order):
1. nmdl.canvas_width → scribe page
2. nmdl.template_type → scribe template
3. comic/children book type → comic
4. magazine/print-replica + conditional templates → magazine
5. default → reflowable

### `stripUnusedSectionKeys(data)`
Removes: `$702`, `yj.conversion.html_name`, `yj.semantics.*`, `yj.authoring.auto_panel_settings_*`

### `filterActiveTemplates(templates, evaluator)`
In fixed-layout mode, filters out templates whose conditions evaluate to false.

## What's NOT yet wired

1. **Book type is not plumbed into `processSection`** — The dispatch currently always uses the reflowable path because `bookType` is not passed through the render pipeline. When B5 (metadata getters) is complete, the `detectBookType()` result needs to be stored on `decodedBook` or passed to `processSection`.

2. **Comic/magazine/scribe branches are stubs** — `processSectionComic`, `processSectionMagazine`, `processSectionScribePage`, `processSectionScribeTemplate` return false (no rendered output). They need:
   - A4: `process_page_spread_page_template` for comic
   - Magazine: `add_content`, `process_position`, layout dispatch
   - D1: Full scribe notebook implementation

3. **`processReadingOrder` deduplication was added** — Python uses a `used_sections` set; Go now logs and skips duplicate section IDs.

## Validation contract assertions covered

- VAL-A-025: Scribe notebook page dispatch (nmdl.canvas_width)
- VAL-A-026: Scribe notebook template dispatch (nmdl.template_type)
- VAL-A-027: Comic/children book type dispatch
- VAL-A-028: Magazine/print-replica with conditional template dispatch
- VAL-A-029: Default reflowable branch unchanged
- VAL-A-030: Conditional template evaluation
- VAL-A-031: Section unused key stripping
