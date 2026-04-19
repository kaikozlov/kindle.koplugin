# A4: process_page_spread_page_template — Library Note

## What was ported

`process_page_spread_page_template` from `yj_to_epub_content.py:210-344`.

## Go implementation location

- **Implementation**: `internal/kfx/yj_to_epub_content.go` — functions `processPageSpreadPageTemplate`, `processPageSpreadStoryBranch`, `processPageSpreadScaleFitBranch`, `processPageSpreadConnectedBranch`, `processPageSpreadLeaf`, plus helpers.
- **Tests**: `internal/kfx/yj_to_epub_content_test.go` — 27 new tests covering all 4 branches, virtual panel handling, LTR/RTL alternation, section name suffix, recursive processing.
- **Helper added**: `internal/kfx/values.go` — `asIntDefault` function.

## Key types

- `pageSpreadConfig` — book-level config (BookType, IsPdfBacked, RegionMagnification, VirtualPanelsAllowed, VirtualPanels, PageProgressionDirection)
- `pageSpreadBranch` — enum for branch detection (Spread, Facing, ScaleFit, Connected, Leaf)
- `pageSpreadResult` — output struct with Children, Sections, VirtualPanels, Err
- `pageSpreadChild` — a recursively processed child template
- `pageSpreadSection` — a leaf content section produced

## Design decisions

1. **Pure function approach**: Unlike Python's mixin class with `self`, the Go port uses a `pageSpreadConfig` struct passed explicitly. This enables stateless testing without needing the full book pipeline.

2. **Branch detection separated**: `determinePageSpreadBranch` is a pure data-inspection function. The full condition (including PDF-backed check) is in `processPageSpreadPageTemplate` which can override the branch.

3. **Recursive processing**: Children from `$146` are processed recursively with the same function, matching Python's recursive call pattern. Results are merged back into the parent's result.

4. **Location ID extraction**: `getLocationID` pops `$155` then `$598` from template data, matching Python's `get_location_id` (yj_to_epub_navigation.py:372-373).

5. **Virtual panel state**: `VirtualPanels` is returned as part of the result, allowing the caller to propagate the state upward.

## Integration points

- `processSectionComic` currently returns a stub result. When wired, it should construct a `pageSpreadConfig` from `decodedBook` fields and call `processPageSpreadPageTemplate` with the section's page template data.
- `processSectionMagazine` similarly needs to use `processPageSpreadPageTemplate` for `$437` layout templates.
- The `$608` fragment resolution (Python's `self.get_fragment(ftype="$608", fid=page_templates[0])`) happens during `organizeFragments` in Go, so template data is already available in `pageTemplateFragment.PageTemplateValues`.

## What's NOT yet done

- **Wiring into `processSectionComic`/`processSectionMagazine`**: These functions still return stub results. The full wiring requires converting `pageSpreadSection` entries into `renderedSection` objects through the storyline renderer.
- **`process_content` integration**: The leaf branch creates section metadata but doesn't yet call the storyline renderer for content processing. This requires the renderer pipeline.
- **`process_position` integration**: When `parentTemplateID` is set, Python calls `self.process_position()` for anchor positioning. This is deferred to the A5 position/location port.
