# Testing Infrastructure — kindle.koplugin Go Codebase

**Generated**: 2026-04-22  
**Branch**: `go` (commit `371bb73`)

---

## 1. Go Test Structure

### Overview

The Go tests live almost entirely in `internal/kfx/` (the core conversion engine). Other packages have smaller test suites:

| Package | Tests (PASS) | Notes |
|---------|-------------|-------|
| `internal/kfx/...` | 748 pass, 1 fail | Core KFX→EPUB conversion tests |
| `cmd/kindle-helper/...` | 9 pass | CLI integration tests |
| `internal/drm/...` | 6 pass | DRM key/cache tests |
| `internal/epub/...` | 50 pass | EPUB packaging tests |
| `internal/jxr/...` | 0 (no test files) | JPEG XR decoder — no tests |
| `internal/scan/...` | 1 pass | Document scanner |

**Total: ~814 tests pass, 1 test fails.**

### Test File Inventory (`internal/kfx/`)

| Test File | Size | What It Tests |
|-----------|------|---------------|
| `kfx_test.go` | 831 lines | End-to-end `ConvertFile()`, `Classify()`, DRMION detection, EPUB structure validation |
| `pipeline_test.go` | 1482 lines | Fragment summary parity, section parsing, Go↔Python reference comparison |
| `yj_structure_test.go` | ~39K | YJ structure (sections, spines, ordering) |
| `yj_structure_symbol_test.go` | ~25K | Symbol name handling in YJ structures |
| `yj_position_location_test.go` | ~76K | Position mapping and location tracking |
| `yj_to_epub_content_test.go` | ~71K | Content conversion (HTML generation from KFX content) |
| `yj_to_epub_properties_test.go` | ~4.5K | CSS property processing, style simplification |
| `yj_to_epub_resources_test.go` | ~15K | Image/font resource handling, JXR conversion |
| `yj_to_epub_navigation_test.go` | ~16K | NCX/NAV navigation generation |
| `yj_to_epub_notebook_test.go` | ~50K | Notebook/special format handling |
| `yj_to_image_book_test.go` | ~23K | Image book conversion (comics, illustrated) |
| `yj_to_epub_illustrated_layout_test.go` | ~17K | Illustrated layout processing |
| `yj_to_epub_misc_test.go` | ~1.3K | Miscellaneous conversion helpers |
| `yj_metadata_test.go` | ~52K | Book metadata extraction |
| `yj_book_test.go` | ~20K | YJ book loading and decoding |
| `yj_versions_test.go` | ~33K | Version/feature handling |
| `yj_versions_golden_test.go` | ~4K | Golden-file parity for KNOWN_FEATURES |
| `yj_symbol_catalog_test.go` | ~3.7K | Golden-file parity for YJ symbol catalog |
| `drm_test.go` | ~3.9K | DRM path mapping, key cache, AES decryption |
| `box_align_test.go` | ~13K | CSS box-align property handling |
| `float_precision_test.go` | ~15K | CSS quantity formatting precision |
| `low_severity_test.go` | ~26K | Low-severity error handling (graceful degradation) |
| `resource_processing_test.go` | ~20K | Image processing, JXR→JPEG, CBZ, PDF |
| `stream_a_medium_test.go` | ~23K | Medium-stream ION binary decoding |
| `illustrated_layout_nav_test.go` | ~15K | Illustrated layout navigation |
| `version_fragment_validation_test.go` | ~22K | Version/fragment validation |

### Testing Patterns

1. **Table-driven tests**: Used extensively for CSS property conversion, DRM path mapping, box-align handling, float precision
2. **Fixture-based integration tests**: `TestConvertFile*` tests run the full `ConvertFile()` pipeline against real KFX files from `REFERENCE/kfx_examples/` and `REFERENCE/kfx_new/decrypted/`
3. **EPUB content assertion**: Tests unzip the output EPUB and check for specific strings in HTML/CSS/OPF/NCX files (e.g., `strings.Contains(opf, '<dc:identifier id="bookid">...')`)
4. **Golden-file tests**: Symbol catalog and yj_versions parity verified against JSON golden files generated from Python
5. **Graceful skip for missing fixtures**: `testutil.SkipIfMissing(t, path)` skips tests when KFX fixture files aren't present (not in repo, only local)

### Test Helpers

- **`internal/testutil/fixtures.go`**: Just one function — `SkipIfMissing(t, path)` — skips test if fixture file doesn't exist
- **`readZipFile(t, file)`** (in `kfx_test.go`): Reads a file from a zip archive
- **`readFixtureDocSymbols(t, path)`** (in `kfx_test.go`): Extracts document symbol table from a KFX binary
- **`unzipFiles(t, path)`** (in `kfx_test.go`): Unzips EPUB to a map of filename→content
- **`comparableArchiveNames(files)`** (in `kfx_test.go`): Gets sorted file names from archive map

### Phased Testing Approach

The `kfx_test.go` uses a "phase" naming convention for integration tests, tracing the pipeline stages:

- Phase 1: `TestConvertFilePhase1PreservesCoverAndPackageResources` — fonts, images, OPF
- Phase 2: `TestConvertFilePhase2PreservesSectionIDsAndLinkedContents` — section HTML, TOC
- Phase 3: `TestConvertFilePhase3UsesCanonicalSectionFilesForNavigationAndSpine` — nav, spine
- Phase 4: `TestConvertFilePhase4EmitsStyleClassesForTitleAndChapterPages` — CSS classes
- Phase 5: `TestConvertFilePhase5ConvertsJPEGXRResourcesToJPEG` — JXR→JPEG
- Phase 6: `TestConvertFilePhase6TracksCalibrePackageAndNavigationSemantics` — metadata, NCX
- Phase 7: `TestConvertFilePhase7UsesPageTemplateStylesForSectionBodyClasses` — body classes
- Phase 8: `TestConvertFilePhase8MatchesInlineStyleEventsAndFitWidthContainers` — inline styles

---

## 2. Golden Files

### Location: `internal/kfx/testdata/`

| Golden File | Size | Purpose | Source Script |
|-------------|------|---------|---------------|
| `yj_symbols_golden.json` | 16KB | 842 YJ shared symbol names, name, version | `scripts/export_yj_symbol_catalog.py` |
| `yj_versions_golden.json` | 1.6KB | KNOWN_FEATURES category→key structure + SHA-256 hash | `scripts/export_yj_versions.py` |

### How Golden Tests Work

**Symbol catalog** (`yj_symbol_catalog_test.go`):
- Loads `testdata/yj_symbols_golden.json` 
- Compares against Go's `sharedTable()` output
- Checks: name match, version match, every SID 10+ matches golden, Go extensions are `$N` placeholders, no duplicates

**YJ versions** (`yj_versions_golden_test.go`):
- Loads `testdata/yj_versions_golden.json`
- Builds same structure from Go's `KnownFeatures` map
- Compares categories and feature keys
- Also checks SHA-256 hash of canonical JSON for quick diff detection

### Golden Regeneration

```bash
python3 scripts/export_yj_symbol_catalog.py > internal/kfx/testdata/yj_symbols_golden.json
python3 scripts/export_yj_versions.py > internal/kfx/testdata/yj_versions_golden.json
```

These should be regenerated when the Calibre Python reference changes.

---

## 3. Parity Testing Scripts

### `scripts/diff_kfx_parity.sh`

**Purpose**: End-to-end EPUB diff comparison for a single book.

**How it works**:
1. Converts a KFX file using `go run ./cmd/kindle-helper convert`
2. Unzips both the Go output EPUB and a reference (Calibre) EPUB
3. Runs an embedded Python script that:
   - Compares all files in `OEBPS/` between the two EPUBs
   - Reports missing/extra/different files
   - For text files (.css, .html, .ncx, .opf, .xhtml, .xml), shows unified diffs
   - Optionally ignores `dcterms:modified` differences in content.opf
4. Exit code 1 if any diffs found, 0 if identical

**Options**: `--input`, `--reference-epub`, `--summary-only`, `--keep-tmp`, `--ignore-modified`

**Defaults**: Runs on Martyr (CONT, unencrypted book).

### `scripts/kfx_reference_snapshot.py`

**Purpose**: Runs the Calibre Python pipeline on a KFX file and emits a structured JSON snapshot of the fragment types/counts/IDs.

**How it works**:
1. Configures Python paths to load Calibre KFX Input code from `REFERENCE/Calibre_KFX_Input/`
2. Loads the YJ symbol catalog to translate `$N` → real names
3. Calls `YJ_Book.decode_book()` → `organize_fragments_by_type()`
4. Outputs JSON with fragment type names, counts, and sorted IDs

**Usage**: `python3 scripts/kfx_reference_snapshot.py fragment-summary --input <kfx-path>`

This snapshot is used by `pipeline_test.go::TestBuildBookStateFragmentSummaryMatchesReference` to verify Go's fragment parsing matches Python's.

### `scripts/parity_check.sh`

**Purpose**: Run Python trace → Go trace → compare for one or all fixture books.

**How it works**:
1. Defines 6 fixtures (Martyr, ThreeBelow, Elvis, Familiars, HungerGames, ThroneOfGlass)
2. For each: runs `trace_python.py` → runs `go run ./cmd/kindle-helper trace` → runs `compare_traces.py`
3. Reports pass/fail/skip counts
4. Can keep trace JSON files with `--keep`

**Usage**: `scripts/parity_check.sh [--book NAME] [--keep]`

### `scripts/compare_traces.py`

**Purpose**: Deep comparison of Python and Go pipeline trace JSON files.

**How it works**:
1. Loads two trace JSON files (Python and Go)
2. Performs deep comparison at each pipeline stage
3. Reports exactly which stages and values differ
4. Supports `--verbose` for detailed output

### `scripts/trace_python.py`

**Purpose**: Trace the Python KFX→EPUB pipeline intermediate state for parity comparison.

**How it works**:
1. Runs the full Calibre pipeline on a KFX file
2. Dumps structured snapshots at every major pipeline stage
3. Outputs a JSON file that the Go trace dumper must produce identically

### `scripts/diff_report.sh`

**Purpose**: Structured CSS diff analysis between Python (Calibre) and Go pipelines.

**How it works**:
1. For each test KFX file, runs both pipelines
2. Extracts CSS from output EPUBs
3. Produces a machine-readable markdown diff report
4. Output order: Summary → Per-File Analysis → Property Frequency Analysis

### `scripts/audit_branches.py`

**Purpose**: Static analysis tool for branch auditing (Python → Go parity).

**How it works**:
1. Parses a Python function's AST
2. Lists every branch (if/elif/for/try/isinstance/type dispatch)
3. Checks the corresponding Go file for equivalent code
4. Used for the mandatory branch audit process described in AGENTS.md

**Usage**: `python3 scripts/audit_branches.py --file yj_to_epub_content.py --function process_content`

### `scripts/convert_kfx_python.py`

**Purpose**: Wrapper to run the Calibre Python conversion pipeline standalone.

---

## 4. Fixture Files

### `REFERENCE/kfx_examples/` — Original 6 Books

These are the **primary test fixtures**. KFX files are symlinks to `REFERENCE/kindle_device/Items01/`. EPUBs are Calibre reference outputs.

| File | Size | Format |
|------|------|--------|
| `Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx` | (symlink) | CONT (unencrypted) |
| `Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.epub` | 3.0MB | Calibre reference |
| `Elvis and the Underdogs_B009NG3090.kfx` | (symlink) | DRMION |
| `Elvis and the Underdogs_B009NG3090.epub` | 974KB | Calibre reference |
| `The Familiars_B003VIWNQW.kfx` | (symlink) | DRMION |
| `The Familiars_B003VIWNQW.epub` | 2.0MB | Calibre reference |
| `The Hunger Games Trilogy_B004XJRQUQ.kfx` | (symlink) | DRMION |
| `The Hunger Games Trilogy_B004XJRQUQ.epub` | 3.2MB | Calibre reference |
| `Three Below (Floors #2)_B008PL1YQ0.kfx` | (symlink) | DRMION |
| `Three Below (Floors #2)_B008PL1YQ0.epub` | 823KB | Calibre reference |
| `Throne of Glass_B007N6JEII.kfx` | (symlink) | DRMION |
| `Throne of Glass_B007N6JEII.epub` | 1.1MB | Calibre reference |

**Key point**: Martyr is the only unencrypted (CONT) book — it produces byte-identical Go output and is the regression baseline. The other 5 are DRMION and require decrypted KFX-zip files for Go testing.

### `REFERENCE/kfx_new/` — Decrypted & Reference Files

**`decrypted/`** — Original 5 DRMION books (decrypted to KFX-zip format):
- Elvis, Familiars, Hunger Games, Three Below, Throne of Glass
- These are the primary inputs for Go conversion testing of DRM books

**`calibre_epubs/`** — Calibre reference EPUBs for original 5 DRMION books

**`decrypted_new/`** — 4 new DRMION books (for expanded parity testing):
- `1984_B003JTHWKU_decrypted.kfx-zip` (1.3MB)
- `HeatedRivalry_B0BZZCSYXJ_decrypted.kfx-zip` (515KB)
- `SecretsCrown_B004S333MC_decrypted.kfx-zip` (21MB)
- `SunriseReaping_B0D6BRYLL1_decrypted.kfx-zip` (1.0MB)

**`calibre_epubs_new/`** — Calibre reference EPUBs for 4 new books

**`monolithic_kfx/`** — Monolithic KFX files extracted from KFX-zips (5 original DRMION books)

**`parity_traces/`** — Saved Python and Go pipeline trace JSONs for all 6 original books:
- `{Book}_python.json` and `{Book}_go.json` for each fixture

### Difference Between `kfx_examples/` and `kfx_new/`

- `kfx_examples/` contains **raw KFX files** (including DRMION originals as symlinks) and Calibre reference EPUBs — used for the Martyr end-to-end test
- `kfx_new/` contains **decrypted KFX-zip files** and their Calibre references — used for all DRMION book testing. The `go run ./cmd/kindle-helper convert` command handles both raw KFX (CONT) and KFX-zip formats

---

## 5. Current Test Status

### Go Test Results (2026-04-22)

```
ok    cmd/kindle-helper         3.992s   (9 tests)
ok    internal/drm              0.341s   (6 tests)
ok    internal/epub             cached   (50 tests)
ok    internal/jxr              cached   (0 tests)
ok    internal/scan             0.637s   (1 test)
FAIL  internal/kfx              9.766s   (748 pass, 1 fail)
```

**Failing test**: `TestConvertFileMatchesReferenceStructureIgnoringImages`
- Location: `pipeline_test.go:1296`
- Error: `OEBPS/c109.xhtml text mismatch` — "Failed to locate 349 referenced positions"
- This appears to be a known parity gap (position location resolution for a specific book)

### Go Vet Results

```
$ go vet ./internal/kfx/...
(no output — clean)
```

No vet issues.

### Lua Test Results

Lua tests are run via `./scripts/test` (busted under luajit). Not run as part of this investigation.

---

## 6. Key Findings from PARITY_AUDIT.md and PLAN.md

### PARITY_AUDIT.md

Describes a systematic Python→Go parity audit plan:
- Methodology: file-by-file audit of every Python function → every branch → map to Go → flag gaps
- Priority order: yj_to_epub_content.go → yj_structure.go → style events → properties → misc
- Lists 30 Go files to audit, tiered by impact
- Known gaps: heading `<a>` class, CSS class naming/dedup, spine ordering, JXR images

### PLAN.md

Documents a step-by-step plan to bring Go output to text-identical match with Python:
- **Original 6 books**: 394/394 matching (fully parity) ✓
- **New 4 books**: 113 text files differ
- Architecture gap identified: Go makes tag decisions during rendering; Python defers to `simplify_styles`
- Steps 1-3 completed (heading refactor, xml:lang, xmlns:epub)
- Steps 4-9 pending (noteref, anchor separation, superscript, body class, structural differences)
- Contains detailed Python↔Go file mapping reference with line counts and status

---

## 7. Summary

### Test Coverage Strengths
- **Extensive integration tests**: 8 phased integration tests tracing the full pipeline
- **Golden-file parity**: Symbol catalog and versions verified against Python reference
- **Multiple comparison tools**: diff_kfx_parity.sh, parity_check.sh, compare_traces.py, diff_report.sh
- **Branch auditing tool**: audit_branches.py for systematic Python→Go branch coverage
- **10 test books**: 6 original + 4 new covering diverse KFX formats

### Test Coverage Gaps
- **`internal/jxr/`**: No tests at all for JPEG XR decoder
- **No test files in other internal packages**: `jsonout/` has no tests
- **Single test failure**: `TestConvertFileMatchesReferenceStructureIgnoringImages` fails due to a position location parity gap
- **Fixture dependency**: Many tests skip if KFX fixtures aren't present (they're local-only, not in git)
- **No benchmark tests**: Performance not measured
- **No fuzz tests**: ION binary parser could benefit from fuzz testing

### Recommendations for Future Work
1. Fix the failing `TestConvertFileMatchesReferenceStructureIgnoringImages` test
2. Add at least basic tests for `internal/jxr/`
3. Consider checking in small synthesized KFX fixtures for tests that currently skip
4. Add the 4 new test books (1984, HeatedRivalry, SecretsCrown, SunriseReaping) to the parity_check.sh fixtures list
