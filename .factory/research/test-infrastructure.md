# Test Infrastructure — kindle.koplugin

## 1. How Go Tests Are Run

```bash
go test ./...
```

This recursively tests all packages under the module `github.com/kaikozlov/kindle-koplugin`. Individual packages can be targeted:

```bash
go test ./internal/kfx/...
go test ./internal/jxr/...
go test ./internal/scan/...
```

There is no Makefile, no test runner script, and no custom test flags beyond what `go test` provides natively.

---

## 2. Test Files — Complete List

All test files reside under `internal/`. There are **10 test files** across 3 packages:

### `internal/kfx/` (8 test files)
| File | Lines | Purpose |
|---|---|---|
| `pipeline_test.go` | 1461 | End-to-end pipeline tests: buildBookState, ConvertFile, snapshot comparison against Python reference |
| `kfx_test.go` | 781 | Integration tests: ConvertFile, Classify, DRM detection, KFX-ZIP handling, phased conversion |
| `box_align_test.go` | 302 | Unit tests for CSS box-align rendering logic using synthetic style fragments |
| `symbol_format_test.go` | 57 | Unit tests for symbol classification (common/short/original/shared) |
| `yj_property_info_test.go` | 134 | Unit tests for YJ property info registry and CSS property conversion |
| `yj_to_epub_illustrated_layout_test.go` | 14 | Unit test for amzn-condition style anchor rewriting |
| `yj_to_epub_misc_test.go` | 34 | Unit tests for condition operator symbol count and evaluation |
| `yj_to_epub_resources_test.go` | 39 | Unit tests for resource filename generation and deduplication |
| `yj_to_epub_navigation_test.go` | 28 | Unit tests for duplicate anchor reporting |

### `internal/jxr/` (1 test file)
| File | Lines | Purpose |
|---|---|---|
| `jxr_test.go` | 205 | JPEG XR parsing tests using binary fixture data from zip archives |

### `internal/scan/` (1 test file)
| File | Lines | Purpose |
|---|---|---|
| `scan_test.go` | 43 | Directory scanning and book classification test |

### Packages with **no tests**:
- `internal/epub/` — no test files
- `internal/jsonout/` — no test files

---

## 3. Current Test Results

**ALL TESTS FAIL** — every test fails with the same root cause: missing fixture files.

```
FAIL  github.com/kaikozlov/kindle-koplugin/internal/jxr   0.237s
FAIL  github.com/kaikozlov/kindle-koplugin/internal/kfx    0.393s
FAIL  github.com/kaikozlov/kindle-koplugin/internal/scan   0.524s
```

### Failure Root Cause

The tests reference fixture files in `REFERENCE/` that are **not present in the repository** (likely git-ignored or stored externally):

- `REFERENCE/kfx_examples/Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx` — primary test fixture
- `REFERENCE/kfx_examples/The Familiars_B003VIWNQW.kfx` — DRM test fixture
- `REFERENCE/kfx_new/decrypted/*.kfx-zip` — decrypted KFX-ZIP fixtures
- `REFERENCE/kfx_new/monolithic_kfx/*.kfx` — monolithic KFX fixtures
- `REFERENCE/martyr_unpack.zip` — unpacked KFX archive for JXR image tests

**Note:** One test (`TestReportDuplicateAnchorsEmitsForMultiSiteResolved`) actually runs and prints a warning to stderr (`anchor "z" has multiple positions: 10.0, 20.1`) but the package still fails overall.

### Test Count Breakdown (from error output)

| Package | Failing Tests | Notes |
|---|---|---|
| `internal/jxr` | 4 | All need `REFERENCE/martyr_unpack.zip` |
| `internal/kfx` | 18 | All need `REFERENCE/kfx_examples/` and `REFERENCE/kfx_new/` |
| `internal/scan` | 1 | Needs `REFERENCE/kfx_examples/` |
| **Total** | **23** | |

Additionally, `internal/kfx` contains unit tests that **do NOT depend on fixtures** and would pass if fixtures were present (the test runner stops at the first `t.Fatalf` in each test):
- `box_align_test.go` tests (synthetic data)
- `symbol_format_test.go` tests (pure logic)
- `yj_property_info_test.go` tests (pure logic)
- `yj_to_epub_illustrated_layout_test.go` tests (pure logic)
- `yj_to_epub_misc_test.go` tests (pure logic)
- `yj_to_epub_navigation_test.go` tests (pure logic)
- `yj_to_epub_resources_test.go` tests (pure logic)

---

## 4. Testing Patterns Used

### Pattern 1: Fixture-Based Integration Tests (majority)
- Load real KFX files from `REFERENCE/kfx_examples/` and `REFERENCE/kfx_new/`
- Run full conversion pipeline (`ConvertFile()`, `buildBookState()`)
- Compare output against expected results
- Tests are named by conversion phase (`TestConvertFilePhase1...`, `TestConvertFilePhase2...`, etc.)

### Pattern 2: Snapshot/Parity Comparison
- `pipeline_test.go` uses `referenceFragmentSnapshot()` to compare Go output against Python reference output
- Fragment types and IDs are compared field-by-field
- Uses `compareIDs` map to selectively compare specific fragment types

### Pattern 3: Synthetic Unit Tests
- `box_align_test.go` constructs `storylineRenderer` structs with hand-crafted style fragments
- Tests individual rendering paths in isolation
- No file I/O required

### Pattern 4: Pure Logic Unit Tests
- `symbol_format_test.go`, `yj_to_epub_misc_test.go`, etc.
- Test individual functions with hardcoded inputs
- No external dependencies

### Pattern 5: ZIP Archive Verification
- `kfx_test.go` opens the output EPUB as a ZIP and inspects contents
- Verifies file existence, readable text, and archive structure
- Uses `unzipFiles()` and `comparableArchiveNames()` helpers

### Pattern 6: Table-Driven Tests (minimal)
- Most tests use individual test functions rather than table-driven subtests
- Some tests iterate over a hardcoded list (e.g., fragment types in `pipeline_test.go`)

### Notable: No Golden File Pattern
- Tests compare against Python reference output or hardcoded expectations
- No `.golden` files or snapshot files checked into the repo

---

## 5. KFX Test Fixtures

### Expected Fixture Locations (referenced by tests but NOT present)

| Path | Used By | Description |
|---|---|---|
| `REFERENCE/kfx_examples/Martyr_*.kfx` | `kfx_test.go`, `pipeline_test.go` | Primary conversion test book |
| `REFERENCE/kfx_examples/The Familiars_*.kfx` | `kfx_test.go` | DRM detection test |
| `REFERENCE/kfx_new/decrypted/*.kfx-zip` | `kfx_test.go` | Decrypted KFX-ZIP archives |
| `REFERENCE/kfx_new/monolithic_kfx/*.kfx` | `kfx_test.go` | Monolithic KFX for comparison |
| `REFERENCE/martyr_unpack.zip` | `jxr_test.go` | Unpacked KFX for JXR image testing |

### Reference Material That DOES Exist

| Path | Description |
|---|---|
| `REFERENCE/Calibre_KFX_Input/` | Complete Calibre KFX Input plugin source (Python reference implementation) |
| `REFERENCE/Calibre_KFX_Input/kfxlib/` | 38 sub-items — the kfxlib Python library |
| `REFERENCE/Calibre_KFX_Input/291290.zip` | 2.5 MB zip archive (likely a KFX sample) |

The `.gitignore` only contains `kindle-helper`, so the fixture absence is NOT due to gitignore — they are simply not committed to the repository.

---

## 6. CLI Tool / cmd/ Directory

**There is no `cmd/` directory and no `main.go` file.** This is a library-only Go module with no executable entry point. The project is a KOReader plugin where:

- **Lua** (`main.lua`, `src/*.lua`) is the runtime entry point on the e-reader
- **Go** (`internal/`) is the conversion library compiled to a binary
- The build script `arm_build.sh` handles cross-compilation for ARM targets

Helper scripts exist in `scripts/`:
- `diff_kfx_parity.sh` — compares Go vs Python output for parity
- `diff_report.sh` — generates diff reports
- `kfx_reference_snapshot.py` — captures Python reference output for comparison
- `convert_kfx_python.py` — Python conversion for parity testing

---

## 7. .gitignore Contents

```
kindle-helper
```

Only one entry: `kindle-helper` (the compiled Go binary). All fixture files in `REFERENCE/` are tracked or absent, but not git-ignored.

---

## 8. Test Pattern Analysis — Key Files

### `pipeline_test.go` (first 100 lines)
- Imports: `archive/zip`, `encoding/json`, `os/exec`, `regexp`, `sort`, `strings`
- `TestBuildBookStateFragmentSummaryMatchesReference`: Loads `Martyr_*.kfx`, builds book state, compares fragment snapshot against Python reference using `referenceFragmentSnapshot()`
- `TestParseSectionFragmentKeepsAllPageTemplatesAndUsesLastAsPrimary`: Pure unit test with hardcoded map data
- `TestRenderSectionFragmentsUsesLastTemplateAsMainAndPrependsOverlaysAfterMain`: Unit test with synthetic `sectionFragment` structs
- Pattern: Mix of integration (fixture-dependent) and unit (synthetic data) tests

### `kfx_test.go` (first 100 lines)
- Imports: `archive/zip`, `encoding/binary`, `image/jpeg`, `io`, `os`, `path/filepath`
- `TestConvertFileCreatesReadableEPUB`: End-to-end test — converts KFX to EPUB, opens result as ZIP, checks for readable text ("Copyright", "Contents")
- `TestClassifyRecognizesDRMFixtures`: Tests `Classify()` returns "convert" for DRM-free and "blocked"/"drm" for DRM-protected files
- `TestClassifyRecognizesDecryptedKFXZipFixtures`: Tests `Classify()` for KFX-ZIP format
- `TestConvertFileFromKFXZipMatchesMonolithicConversion`: Compares KFX-ZIP and monolithic KFX conversion produce same output
- Pattern: All integration tests requiring fixture files, testing the full pipeline

---

## Summary

The test infrastructure is functional but entirely dependent on **external fixture files** that are not present in the repository. The 23 failing tests would pass once fixtures are placed in the expected `REFERENCE/` locations. Approximately 10+ unit tests (box_align, symbol_format, yj_property_info, etc.) use synthetic data and would pass independently if the fixture-dependent tests didn't cause the package to fail first.
