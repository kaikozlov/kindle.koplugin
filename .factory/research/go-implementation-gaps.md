# Go Implementation Gap Analysis

Generated: 2026-04-18

---

## 1. state.go — `organizeFragments`, `book_data` handling

**Status: Substantial, mostly complete.**

### What's implemented:
- `buildBookState()` — top-level entry point: loads sources → `organizeFragments`
- `organizeFragments()` — full port of `KFX_EPUB.organize_fragments_by_type`:
  - Index walk over all container sources (24-byte index entries)
  - Fragment catalog populated by type: `$145` (content), `$157` (styles), `$164` (resources), `$258` (reading order), `$259` (storylines), `$260` (sections), `$262` (fonts), `$266` (anchors), `$391` (nav containers), `$490` (title metadata), `$538` (document data), `$585` (content features), `$608` (ruby contents), `$609` (position maps), `$756` (ruby groups), `$389` (nav roots), `$417`/`$418` (raw blobs)
  - Symbol collection via `mergeIonReferencedStringSymbols` across all fragment types
  - `combineContainerDocSymbols` merges doc symbol tables from all sources
  - Section order fallback chain: `$258` reading_orders → `$538` reading_orders → alphabetical
  - `determineBookSymbolFormat` called after organize
  - `applyKFXEPUBInitMetadataAfterOrganize` called at end (matches Python L77–80 order)
- `collectContainerBlobs` — handles CONT, DRMION, ZIP, and sidecar `.sdr` directory
- `fragmentSnapshot()` for debugging
- `loadContainerSourceData` — header parsing, container info decode, symbol table + index extraction

### What's incomplete/notes:
- **doc.go says phase-a-replace-ion-organize is "Partial"**: The index walk is "not Python book_data / replace_ion_data struct" — Go uses a custom fragment catalog (`fragmentCatalog`) instead of Python's `book_data` dict-of-dicts pattern. This is a structural difference but functionally equivalent.
- No explicit `$270` container fragment storage (only used for `lastContainerID` grouping in the index walk).
- The `$593` entries use `lastContainerID` as `summaryID` but are otherwise not stored — this matches Python behavior (they're grouped under their container).

---

## 2. symbol_format.go — `classifySymbolWithResolver`

**Status: Complete.**

### What's implemented:
- `classifySymbol()` — full regex-based classification matching Python `BookStructure.classify_symbol`:
  - `isCommonSymbol` — exact matches + `kfx_cover_image*`, content/eidbucket/pagelist numbers, UUID, yj.meta, nmdl
  - `reDict` — `G[0-9]+(-spm)?$` and `yj.dictionary.*`
  - `isOriginalSymbol` — 12 regex patterns for V_, res_, anchor-, LANDMARKS_, etc.
  - `reBase64Sym` — 22+ char base64 resource names
  - `reShortSym` — short resource names
- `classifySymbolWithResolver()` — adds SHARED check via `symbolResolver.isSharedSymbolText` for unresolved `$<sid>` text
- `determineBookSymbolFormat()` — voting logic: counts by type, quorum check, `symUnknown/10` added to original
- `documentDataHasMaxID()` — checks `$538` for `max_id` key
- `uniquePartOfLocalSymbol()` — strips format-specific prefixes (short, dictionary, base64, original)
- `prefixUniquePartOfSymbol()` — prefix attachment with alnum-dash and letter-start heuristics

### What's incomplete:
- Nothing significant. Full parity with Python's `classify_symbol` and `determine_book_symbol_format`.

---

## 3. yj_to_epub_content.go — `processSection` branches

**Status: Partial — reflowable/fixed-layout only.**

### What's implemented:
- `processReadingOrder()` — iterates section order, calls `processSection` per section
- `processSection()` → delegates to `renderSectionFragments()`
- `renderSectionFragments()`:
  - Handles page templates (single or multi-template with conditions)
  - Fixed-layout condition filtering (active template selection)
  - Main template rendering via `renderer.renderStoryline()`
  - Overlay template rendering (templates before main)
  - Merges body classes and section properties
  - Debug logging via `KFX_DEBUG` / `KFX_DEBUG_SECTION_CLASS`
- `prepareBookParts()` — post-processing: `normalizeHTMLWhitespace` on each rendered section Root
- Helper maps: `listTagByMarker`, `classificationEPUBType`, `layoutHintElementNames`
- `pageTemplatesHaveConditions()`, `mergeSectionProperties()`

### What's NOT implemented (branches that Python has but Go doesn't):
- **Scribe/Notebook branch**: Python `process_section` checks for `nmdl.*` keys and branches to `process_scribe_notebook_page_section` / `process_scribe_notebook_template_section` — Go has stubs in `yj_to_epub_notebook.go` but they're never called from `processSection`
- **Comic/Magazine branch**: Python checks book_type for "comic" or "magazine" and has special handling — Go doesn't wire `cde_content_type` to book type
- **`replace_eol_with_br`**: Python runs this on book_part bodies — Go uses `normalizeHTMLWhitespace` which is a different approach
- **`reset_preformat` / `preformat_spaces`**: Python preformat handling — not explicitly ported

---

## 4. yj_to_epub_illustrated_layout.go

**Status: Partial.**

### What's implemented:
- `fixupIllustratedLayoutAnchors()` — rewrites `-kfx-amzn-condition` inline styles from `anchor:` URIs to same-file fragment IDs
- `fixupIllustratedLayoutParts()` — recursive walk of HTML elements
- `rewriteAmznConditionStyle()` — parses CSS declarations, rewrites anchor: URIs to fragments
- Handles `anchor:` scheme URIs, path-based matching, base filename comparison

### What's incomplete:
- This is described as a "subset" in doc.go. Python's `yj_to_epub_illustrated_layout.py` has more:
  - `fixup_illustrated_layout_region_magnification` — region magnification handling
  - `fixup_illustrated_layout_condition_classes` — KFXConditionalNav class handling
  - Full SVG viewport / panel layout for illustrated layout mode

---

## 5. yj_to_epub_navigation.go — ported vs missing

**Status: Substantial, mostly complete.**

### What's implemented:
- `processNavigation()` — full nav processing: collects containers, processes each by type
- `processContainer()` — handles imports, dispatches by nav type (`$212`/`$213`/`$214`/`$798` → TOC, `$236` → guide, `$237` → pages)
- `processNavUnit()` — recursive TOC point building with heading levels, anchor registration
- `processGuideUnit()` — guide entries with landmark type mapping
- `processPageUnit()` — page list entries with deduplication
- `navigationToEPUB()` — converts nav points to epub.NavPoint
- `guideToEPUB()` — converts guide entries, filters to standard types (cover/text/toc)
- `pagesToEPUB()` — converts page entries
- `buildPositionAnchorIDs()` — builds position→offset→anchor_id map
- `fixupAnchorsAndHrefs()` — replaces anchor placeholders
- `reportMissingPositions()` — logs unresolved positions
- `reportDuplicateAnchors()` — logs duplicate anchor registrations
- `orderedSectionIDsFromNavigation()` — extracts section IDs from TOC order
- `mergeSectionOrder()` — merges nav order with reading order
- Helper functions: `parseNavTitle`, `parseNavTarget`, `resolveNavigationContainer`, `guideTypeForLandmark`, `headingLevelForLandmark`

### What's missing:
- Python has additional navigation processing around KFXConditionalNav groups — Go handles basic nav but not conditional navigation groups
- `collectNavigationContainers` is implemented but may not handle all Python edge cases around container import chains

---

## 6. yj_to_epub_metadata.go — functions present

**Status: Substantial.**

### Functions:
- `applyMetadata()` — processes `$490` title metadata: title, author, language, issue_date, description, cover_image, publisher, override_kindle_font, ASIN, book_id, content_id, cde_content_type, orientation_lock, fixed_layout, illustrated_layout, facing_page, publisher_panels, support_landscape, support_portrait
- `applyDocumentData()` — extracts orientation_lock ($433), direction, writing-mode, default font family ($11)
- `applyContentFeatures()` — processes `$590` feature array for fixed_layout and illustrated_layout
- `hasNamedFeature()` — recursive feature name search
- `applyKFXEPUBInitMetadataAfterOrganize()` — orchestration: content features → document data → title metadata → reading order metadata
- `applyReadingOrderMetadata()` — processes `$258` top-level metadata entries
- `applyMetadataItem()` — individual metadata key/value application
- `featureKey()` — builds feature key from args
- `metadataSymbolNames` — Ion key → metadata name map

### What's incomplete:
- `cde_content_type` handling — Python maps MAGZ→magazine, EBSP→sample book types; Go comment says "Not yet wired to Go book type"
- `facing_page` / `double_page_spread` / `publisher_panels` — Python sets `book_type = "comic"`; Go comment says "not yet wired"
- `author_pronunciation` — Python stores in `self.author_pronunciations`; Go comment says "not needed for EPUB output"
- No `font_name_replacements["default"]` wiring (noted in `applyDocumentData` comments)

---

## 7. yj_to_epub_misc.go — current state

**Status: Complete for condition evaluation.**

### What's implemented:
- Full condition operator dispatch table matching Python's `set_condition_operators`:
  - Nullary (0 args): `$305` (screen height), `$304` (screen width), `$300`/`$301` (true), `$302`/`$303` (screen dims), `$183` (zero), `$525` (landscape), `$526` (portrait), `$660` (supported feature)
  - Unary (1 arg): `$293` (NOT), `$266` (zero/null), `$750` (device type check), `$659` (supported feature check)
  - Binary (2 args): `$292` (AND), `$291` (OR), `$294`-`$299` (comparison operators), `$516`-`$519` (arithmetic)
- `evaluate()` — main evaluation entry point (list → operator dispatch)
- `evaluateBinary()` — wraps evaluate with bool type check and error logging
- `screenSize()` — returns screen dimensions based on orientation lock
- `conditionOperatorArity` — nargs validation with mismatch error logging
- Helper functions: `firstArg`, `secondArg`, `numericConditionValue`, `compareConditionValues`

### What's missing:
- Python `yj_to_epub_misc.py` is broader than just condition evaluation — it's the KFX_EPUB_Misc mixin class. The Go implementation focuses exclusively on condition evaluation. Other misc functionality (if any) is distributed across other Go files.

---

## 8. yj_to_epub_resources.go — current state

**Status: Substantial, with known gaps.**

### What's implemented:
- `buildResources()` — full resource/font assembly pipeline:
  - Sorts resource IDs alphabetically
  - Matches raw blobs to resources by media type
  - JXR → JPEG conversion
  - Unique filename generation with deduplication
  - `@font-face` CSS generation with font property declarations
  - Cover image identification and `cover-image` property tagging
- `mapFontStyle()`, `mapFontWeight()`, `mapFontStretch()` — Ion enum → CSS value
- `parseResourceFragment()`, `parseFontFragment()` — fragment parsing
- `packageResourceStem()` — resource filename stem generation with path preservation
- `uniquePackageResourceFilename()`, `uniqueFontPackageFilename()` — deduplication
- `extensionForMediaType()`, `detectFontExtension()`, `detectImageExtension()`
- `blobMatchesImageMediaType()` — blob type validation
- `partitionRawBlobs()` — separates image/font pools
- `convertJXRResource()` — JXR gray→JPEG conversion
- Font media type detection, CSS `@font-face` declaration generation

### What's incomplete:
- `convertJXRResource` comment says: "full color JXR decode parity is Phase E backlog" — currently only handles gray8
- `PLACE_FILES_IN_SUBDIRS` not implemented (doc.go says this is a configuration difference, not a parity gap)
- No sub-directory support for resources (Python can place in `images/`, `fonts/`, etc.)

---

## 9. yj_to_epub_notebook.go — current state

**Status: Stub only (as expected).**

### What's implemented:
- `processScribeNotebookPageSection()` — returns `false`, takes correct params
- `processScribeNotebookTemplateSection()` — returns `false`, takes correct params

### What's incomplete:
- Everything — these are pure stubs with no logic. Doc.go says "no fixture-driven logic".
- Not called from `processSection` — the scribe branch detection (`nmdl.*` keys) isn't wired up.
- Expected to remain stubs until scribe fixtures are available for testing.

---

## 10. kfx.go — what remains

**Status: Core types + entry points + various emitters.**

### What's in kfx.go:
- **Error types**: `DRMError`, `UnsupportedError`
- **Core data types**: `decodedBook`, `renderedStoryline`, `renderedSection`, `resourceFragment`, `fontFragment`, `symbolResolver`, `rawBlob`, `sectionFragment`, `pageTemplateFragment`, `anchorFragment`, `navTarget`, `navPoint`, `guideEntry`, `pageEntry`
- **Entry points**: `Classify()`, `ConvertFile()`, `decodeKFX()`
- **Cover SVG promotion**: `applyCoverSVGPromotion()`, `coverImageDimensions()`, `isCoverImageSection()`
- **Utilities**: `normalizeBookIdentifier()`, `normalizeLanguage()`
- Various commented-out declarations (styleBaseName, singleImageWrapperChild, blockAlignedContainerProperties, etc.)

### What's incomplete:
- Several function declarations are present as comments only (not implemented):
  - `styleBaseName` — style class base naming
  - `singleImageWrapperChild` — image wrapper detection
  - `blockAlignedContainerProperties` — block property detection
  - `reverseHeritablePropertiesExcludes` — heritable property partition
  - `isBlockContainerProperty` — block container property check
- These are likely implemented elsewhere (yj_to_epub_properties.go, content_helpers.go) but the comment stubs remain

---

## 11. render.go — current state

**Status: Core orchestration, substantial.**

### What's implemented:
- `renderBookState()` — main orchestration function (~200 lines):
  - Font name fixer setup and registration
  - Resource building (`buildResources`)
  - Language inference
  - Position-to-section mapping (from aliases, sections, page templates, storyline nodes)
  - Navigation processing and title extraction
  - Anchor URI resolution (direct and fallback)
  - `storylineRenderer` construction with all dependencies
  - Navigation order merging and cover promotion
  - `processReadingOrder` call
  - Post-processing: cleanup, alias anchors, anchor URI resolution, fixup, HTML defaults, styles, CSS, SVG promotion, resource pruning, stylesheet pruning
  - EPUB navigation/guide/page list generation
  - Identifier and language normalization
- `registerNamedPositionAnchor()` — anchor registration
- `resolveRenderedAnchorURIs()` — resolves anchor URIs across sections
- `attachSectionAliasAnchors()` — attaches anchor IDs to section elements
- `replaceRenderedAnchorPlaceholders()` — replaces `anchor:` hrefs
- `materializeRenderedSections()` — converts rendered sections to epub.Sections
- `cleanupRenderedSections()` — HTML cleanup (empty wrappers, nested div collapsing)
- `promoteCoverSectionFromGuide()` — moves cover section to front of spine
- Various helper functions for HTML manipulation

### What's incomplete:
- This is the primary orchestration point. Most gaps are in the functions it calls rather than here.
- `debugSectionMappings` is referenced but defined elsewhere

---

## 12. fragments.go — current state

**Status: Complete for fragment parsing.**

### What's implemented:
- `readSectionOrder()` — extracts `$169` reading orders from `$258` or `$538`
- `parsePositionMapSectionID()` — extracts section ID from position map fragments
- `readPositionMap()` — extracts position IDs from `$181` entries
- `sectionStorylineID()` — extracts storyline ID from section containers
- `parseSectionFragment()` — full section fragment parsing:
  - Multiple page template containers → `PageTemplates` slice
  - Last template used as primary (matching Python behavior)
  - Condition preservation
- `collectStorylinePositions()` — recursive position extraction from storyline nodes
- `parseAnchorFragment()` — anchor parsing with URI and position extraction, debug logging
- `chooseFragmentIdentity()` — identity resolution (resolved name > fragment ID)
- `isResolvedIdentity()`, `isPlaceholderSymbol()` — identity classification

### What's incomplete:
- Nothing significant. Full fragment parsing parity.

---

## internal/epub/epub.go — vs Python's epub_output.py

**Status: Substantial EPUB3 emission, with known gaps.**

### What's implemented:
- `Book` struct — all metadata fields, sections, resources, navigation, guide, page list
- `Write()` — full EPUB3 zip emission:
  - `mimetype` (stored, uncompressed)
  - `META-INF/container.xml`
  - `OEBPS/content.opf` — OPF with metadata, manifest, spine, guide
  - `OEBPS/nav.xhtml` — EPUB3 navigation with TOC, landmarks, page-list
  - `OEBPS/toc.ncx` — NCX for backward compatibility with page metadata
  - `OEBPS/stylesheet.css`
  - Sections and resources
- `contentOPF()` — manifest items sorted by filename (Calibre parity), unique manifest IDs
- `sectionXHTML()` — per-section XHTML generation with language, title, body class
- `navXHTML()` — EPUB3 nav document with TOC, landmarks, page-list
- `tocNCX()` — NCX with page targets and metadata
- Natural sort key for filenames
- Manifest ID generation (basename truncated to 64 chars, sanitized, deduplicated)
- Guide type mapping (cover/text/toc → epub:type)
- NCX page metadata (decimal → normal, roman → front)

### What's missing vs Python's `epub_output.py`:
- **Manifest ordering**: Python uses more sophisticated sorting; Go uses string sort (doc.go says "manifest items sorted together by filename string")
- **Spine page-progression-direction**: Not emitted in the OPF spine element (Python does when RTL)
- **Metadata completeness**: Python adds more dc: metadata elements; Go covers the core set
- **File-in-subdirectory support**: Python can place files in subdirectories; Go puts all in OEBPS/
- **OPF metadata refinements**: Python adds more refines entries (file-as, alternate-script, etc.)
- **NCX depth**: Python sets `dtb:depth` and `dtb:totalPageCount` in NCX head; Go only sets `dtb:uid`
- **Cover image manifest**: Python has specific cover image handling; Go uses `cover-image` property

---

## TODO/FIXME/HACK Comments

Found across the codebase:

| File | Line | Comment |
|------|------|---------|
| `yj_to_epub_resources.go` | 440 | `"full color JXR decode parity is Phase E backlog"` |
| `yj_to_epub_metadata.go` | 82 | `"Not yet wired to Go book type"` (cde_content_type) |
| `yj_to_epub_metadata.go` | 96 | `"Python sets book_type = 'comic'; not yet wired"` (facing_page) |
| `yj_to_epub_metadata.go` | 98 | `"Python sets book_type = 'comic' + virtual_panels/region_magnification; not yet wired"` |
| `svg.go` | 111 | `"path bundle lookup not yet implemented"` |
| `doc.go` | 39 | `"phase-e-images-resources — Partial: convertJXRResource (gray→JPEG); byte parity vs Calibre deferred"` |
| `doc.go` | 40 | `"phase-f-notebook-scribe — Partial: yj_to_epub_notebook.go processScribe* stubs (no fixture-driven logic)"` |
| `pipeline_test.go` | 1300 | `"Full byte-for-byte text parity vs calibre_epubs is still Phase D work"` |

---

## Summary by Phase (from doc.go checklist)

| Phase | Status | Key Gaps |
|-------|--------|----------|
| phase-a (organize/classify) | Partial | Index walk not Python book_data struct; structurally equivalent |
| phase-b (reading order/sections) | Partial | Scribe/comic/magazine branches unported |
| phase-c (property info/styles) | Partial | Some YJ property mappings incomplete |
| phase-d (anchors/nav/epub) | Partial | Illustrated layout subset; OPF metadata gaps |
| phase-e (images/resources) | Partial | JXR color decode; no subdirectory support |
| phase-f (notebook/scribe) | Stub | Pure stubs, no fixture-driven logic |
| condition-operators | Done | Full dispatch table with arity validation |
| post-organize-init-metadata | Done | Full $490/$538/$585 processing |
| error-handling | Partial | Missing image/font warnings, condition errors logged |
