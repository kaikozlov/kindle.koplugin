# User Testing Guide — KFX→EPUB Parity

## Overview

This project is a pure Go library with no running services. The only testable surface is the `cmd/kindle-helper` CLI binary and `go test` unit tests.

## Services

No services required. Build with `go build ./cmd/kindle-helper`. Test with `go test ./cmd/kindle-helper/... -v -count=1`.

## Testing Surfaces

### CLI Surface (`cmd/kindle-helper`)

- **Tool**: Build binary, run as subprocess, inspect JSON output
- **Commands**: `scan --root <path>`, `convert --input <path> --output <path>`
- **Output**: JSON to stdout, errors to stderr
- **Exit codes**: 0 = success, 1 = operational error, 2 = usage error

### Go Test Surface

- **Tool**: `go test ./... -count=1`
- **Known failures**: Pre-existing fixture-dependent tests in `internal/kfx`, `internal/jxr`, `internal/scan` fail because KFX fixture files don't exist in the repo. These are EXPECTED and must be ignored.
- **Relevant tests**: Only `./cmd/kindle-helper/...` tests matter for milestone "infra".

## Validation Concurrency

| Surface | Max Concurrent Validators | Reason |
|---------|--------------------------|--------|
| CLI     | 5                        | Subprocess execution, lightweight |
| go test | 3                        | Builds can consume memory |

## Flow Validator Guidance: CLI

The CLI surface tests the `cmd/kindle-helper` binary as a black box.

**Isolation rules:**
- Each validator builds its own binary in a temp directory
- Each validator uses its own temp directory for scan roots and output files
- No shared mutable state between validators
- Binary is read-only after build; no interference possible

**Boundaries:**
- Do NOT modify source files
- Do NOT test with real KFX files (they don't exist in the repo)
- Use temp directories for all file operations
- Clean up temp files after tests

**How to test:**
1. Build binary: `go build -o /tmp/test-helper github.com/kaikozlov/kindle-koplugin/cmd/kindle-helper`
2. Run commands and capture stdout/stderr
3. Parse JSON output and verify structure
4. Check exit codes

## Flow Validator Guidance: Go Test (Milestone A)

The Go test surface validates milestone A features by running `go test ./internal/kfx/... -count=1 -v` and verifying that specific tests pass.

**Test tool**: `go test ./internal/kfx/... -count=1 -v` (direct shell execution)

**Isolation rules:**
- Each validator runs `go test` with specific `-run` flags targeting its assigned assertions
- Tests are independent — they construct their own synthetic data
- No shared mutable state between tests
- Validators can run concurrently since `go test` builds and caches independently

**Boundaries:**
- Do NOT modify source files
- Only inspect test results (pass/fail/output)
- Ignore the 18 pre-existing fixture-dependent failures listed below
- Focus ONLY on the assigned assertion IDs

**How to test:**
1. Run `go test ./internal/kfx/... -count=1 -v -run "<TestPattern>"` for each assertion group
2. Check exit code: 0 = all tests pass, 1 = some tests fail
3. Parse output for specific test names matching the assertion
4. Report pass/fail per assertion based on whether the corresponding test passes

**Known test name mappings (milestone A):**
- A1 (organizeFragments): TestOrganizeFragments*, TestReplaceIonData*, TestFragmentIDRemap*, TestBookSymbols*, TestSectionOrder*, TestMergeIonReferencedStringSymbols*
- A2 (classifySymbol): TestClassifySymbol*, TestAllowedSymbolPrefix*, TestGetReadingOrders*, TestOrderedSectionNames*, TestDetermineBookSymbolFormat*, TestHasIllustratedLayout*
- A3 (processSection): TestProcessSection*, TestScribeNotebook*, TestComicDispatch*, TestMagazineDispatch*, TestSectionUnused*, TestBranchPriorityOrder*
- A4 (pageSpread): TestPageSpread*, TestScaleFit*, TestConnectedPagination*, TestLeafBranch*, TestVirtualPanel*, TestFacingPage*, TestRecursivePageSpread*, TestExtractSpreadType*
- A5 (position/location): TestContentChunk*, TestConditionalTemplate*, TestPositionMap*, TestLocationMap*, TestApproximatePages*, TestMatchReport*, TestPidForEid*, TestEidForPid*, TestCollectContent*

## Flow Validator Guidance: Go Test (Milestone B)

The Go test surface validates milestone B features by running `go test` on specific test patterns in `./internal/epub/...` and `./internal/kfx/...`.

**Test tool**: `go test ./internal/epub/... ./internal/kfx/... -count=1 -v` (direct shell execution)

**Isolation rules:**
- Each validator runs `go test` with specific `-run` flags targeting its assigned assertions
- Tests are independent — they construct their own synthetic data
- No shared mutable state between tests
- Validators can run concurrently since `go test` builds and caches independently

**Boundaries:**
- Do NOT modify source files
- Only inspect test results (pass/fail/output)
- Ignore the 18 pre-existing fixture-dependent failures (see below)
- Focus ONLY on the assigned assertion IDs

**How to test:**
1. Run `go test ./internal/epub/... -count=1 -v -run "<TestPattern>"` for EPUB packaging assertions
2. Run `go test ./internal/kfx/... -count=1 -v -run "<TestPattern>"` for KFX library assertions
3. Check exit code: 0 = all tests pass, 1 = some tests fail
4. Parse output for specific test names matching the assertion
5. Report pass/fail per assertion based on whether the corresponding test passes

**Known test name mappings (milestone B):**
- B1 (EPUB Packaging): TestManifest*, TestSpine*, TestOPF*, TestCover*, TestOverride*, TestNCX*, TestEpub3Nav*, TestGuide*, TestContainerXML*, TestZIP*, TestMakeManifestID*, TestSectionXHTML*, TestDefaultNav*, TestDefaultTitle*
- B2 (Navigation Reporting): TestReportMissing*, TestReportDuplicate*, TestRegisterAnchor*, TestProcessNavigation*, TestNavContainer*, TestStripOperator*, TestPythonCondition*
- B3 (Illustrated Layout): TestRewrite*, TestConditionOper*, TestFixup*, TestStyleRewrite*, TestCreateConditional*
- B4 (Resource Variant): TestResourceVariant*, TestResourceDedup*, TestResourceCache*, TestLocateRaw*, TestProcessExternal*, TestPackageResource*
- B5 (Metadata Getters): TestGetMetadata*, TestGetFeature*, TestIsMag*, TestIsSample*, TestIsFixed*, TestIsPrint*, TestIsImage*, TestIsKfx*, TestHasPdf*, TestHasMetadata*, TestGetCover*, TestGetAsset*, TestGetGenerators*, TestCaching*, TestCdeType*, TestUpdateCover*, TestHasCover*

## Flow Validator Guidance: Go Test (Milestone C)

The Go test surface validates milestone C features by running `go test` on specific test patterns in `./internal/kfx/...`.

**Test tool**: `go test ./internal/kfx/... -count=1 -v` (direct shell execution)

**Isolation rules:**
- Each validator runs `go test` with specific `-run` flags targeting its assigned assertions
- Tests are independent — they construct their own synthetic data
- No shared mutable state between tests
- Validators can run concurrently since `go test` builds and caches independently

**Boundaries:**
- Do NOT modify source files
- Only inspect test results (pass/fail/output)
- Ignore the pre-existing fixture-dependent failures (see below)
- Focus ONLY on the assigned assertion IDs

**How to test:**
1. Run `go test ./internal/kfx/... -count=1 -v -run "<TestPattern>"` for each assertion group
2. Check exit code: 0 = all tests pass, 1 = some tests fail
3. Parse output for specific test names matching the assertion
4. Report pass/fail per assertion based on whether the corresponding test passes

**Known test name mappings (milestone C):**
- C1 (yj_versions): TestSentinelConstants*, TestFeatureNameConstants*, TestPackageVersion*, TestKnownKFXGenerators, TestGenericCreatorVersions, TestKnownFeatures*, TestKnownSupportedFeatures, TestKnownMetadata*, TestKnownAuxiliary*, TestKnownKCBData*, TestUnsupported, TestKindleVersion*, TestKindleCapability*, TestIsKnownGenerator*, TestIsKnownFeature*, TestKindleFeatureVersion*, TestIsKnownMetadata*, TestIsKnownAux*, TestIsKnownKCBData*, TestAuxMetadata*
- C2 (enum props): TestYJPropertyInfoCoreKeys, TestConvertYJProperties*, TestProcessContentProperties*
- C3 (fragment validation): TestModuleControlConstants, TestFixedLayoutImageFormats, TestSymType*, TestNumstr*, TestExtractFragmentID*, TestWalkFragment*, TestCheckFragmentUsage*, TestDetermineEntityDependencies*, TestRebuildContainerEntityMap*, TestFragmentIDKeys, TestCommonFragmentReferences, TestNestedFragmentReferences, TestSpecialFragmentReferences, TestSectionDataTypes, TestEIDReferences, TestExpectedAnnotations, TestFragmentListGet, TestSortImportUsed, TestSpecialParentFragment*, TestRootAndContainer*, TestKnownFragment*, TestSortedFragment*, TestEntityDeps*

## Flow Validator Guidance: Go Test (Milestone D)

The Go test surface validates milestone D features by running `go test` on specific test patterns in `./internal/kfx/...`.

**Test tool**: `go test ./internal/kfx/... -count=1 -v` (direct shell execution)

**Isolation rules:**
- Each validator runs `go test` with specific `-run` flags targeting its assigned assertions
- Tests are independent — they construct their own synthetic data
- No shared mutable state between tests
- Validators can run concurrently since `go test` builds and caches independently

**Boundaries:**
- Do NOT modify source files
- Only inspect test results (pass/fail/output)
- Ignore the pre-existing fixture-dependent failures (see below)
- Focus ONLY on the assigned assertion IDs

**How to test:**
1. Run `go test ./internal/kfx/... -count=1 -v -run "<TestPattern>"` for each assertion group
2. Check exit code: 0 = all tests pass, 1 = some tests fail
3. Parse output for specific test names matching the assertion
4. Report pass/fail per assertion based on whether the corresponding test passes

**Known test name mappings (milestone D):**
- D1 (notebook/scribe): TestNotebookConstants, TestSVGDoctype, TestBrushTypeConstants, TestThicknessName, TestThicknessChoices, TestThicknessChoicesNoEraser, TestStrokeColors, TestAdjustColorForDensity*, TestDecodeStrokeValues*, TestProcessScribeNotebook*
- D2 (image book): TestCombineImagesIntoCBZ*, TestCombineImagesIntoPDF*, TestAddPDFOutline*, TestSuffixLocation*, TestGetOrderedImages*, TestLandscapeSplitting*, TestUseHighestResolutionImageVariant, TestGetOrderedImageResources*
- D3 (float precision): TestFormatCSSQuantity*, TestValueStrWithUnit*, TestColorStr*, TestIntToAlpha, TestAlphaToInt, TestFloatPrecisionNumstr, TestNumstr*

**Known validation contract discrepancies:**
- VAL-D-008: Contract claims adjustColorForDensity(0xffffff, 1.0) returns 0x000000, but Python returns 0xffffff. Our Go follows Python (source of truth).
- VAL-D-008: Contract claims adjustColorForDensity(0xffffff, 2.0) returns 0x000000, but Python returns 0xffffff.
- VAL-D-015: Contract claims delta decoding produces [5,13,20,27,36] but Python's algorithm produces [5,8,10,12,16].

## Flow Validator Guidance: Go Test (Milestone M1)

The Go test surface validates milestone M1 (foundation fixes) features by running `go test` on specific test patterns in `./internal/epub/...` and `./internal/kfx/...`.

**Test tool**: `go test ./internal/epub/... ./internal/kfx/... -count=1 -v` (direct shell execution)

**Isolation rules:**
- Each validator runs `go test` with specific `-run` flags targeting its assigned assertions
- Tests are independent — they construct their own synthetic data
- No shared mutable state between tests
- Validators can run concurrently since `go test` builds and caches independently

**Boundaries:**
- Do NOT modify source files
- Only inspect test results (pass/fail/output)
- Ignore the pre-existing fixture-dependent failures
- Focus ONLY on the assigned assertion IDs

**How to test:**
1. Run `go test ./internal/epub/... -count=1 -v -run "<TestPattern>"` for EPUB packaging assertions
2. Run `go test ./internal/kfx/... -count=1 -v -run "<TestPattern>"` for KFX library assertions
3. Check exit code: 0 = all tests pass, 1 = some tests fail
4. Parse output for specific test names matching the assertion
5. Report pass/fail per assertion based on whether the corresponding test passes

**Known test name mappings (milestone M1):**
- M1-EPUB (10 assertions): TestDefaultTitle_Unknown, TestFixHTMLID_ArabicIndic, TestFixHTMLID_IllustratedLayoutDots, TestEPUBVersionSwitching, TestGuideSectionConditional, TestNCX_MBPNamespace, TestSpinePageProgressionDirection, TestOPFMetadataRefinements, TestOPFManifestOrdering_MatchPython, TestFontFaceOrdering
- M1-META (5 assertions): TestUpdateCoverSection_Recursive157, TestIsImageBasedFixedLayout_FullValidation, TestGetGenerators_PlaceholderFiltering, TestIsKfxV1_VersionField, TestFixCoverImageData_JFIFReencode
- M1-VERS (9 assertions): TestKindleFeatureVersion_TrueEq1, TestKindleFeatureVersion_BoolIntEquivalence, TestIsKnownFeature_BoolIntEquivalence, TestColorAlphaThresholdConsistency, TestColorAlphaConsistency_ColorStrVsCssColor, TestFragmentValidation_KpfPrepub610, TestIonDataEq_*, TestFragmentValidation_DuplicateDetection, TestFragmentValidation_KpfPrepubCleanup, TestFragmentValidation_SampleDict597, TestDictionaryAnnotations_OnlyForDicts, TestRebuild_SkipsDictionary
- M1-FLOAT (4 assertions): TestValueStr_NilInterface, TestValueStr_NilFloat64Ptr, TestCropImage_ResourceScaling, TestCropImage_UnsupportedFormat, TestColorStr_*, TestFixColorValue*
- M1-STRM (6 assertions): TestMetadataProcessing258NoOverride, TestSymbolFormatNonShortLog*, TestCheckSymbolTable*, TestHasIllustratedLayoutCondition*, TestHasIllustratedLayoutPageTemplateCondition*, TestOverlayTemplate171*, TestFontSizeRem*

## Pre-existing Known Failures (IGNORE)

These test failures are expected because fixture files are missing:

- `internal/jxr` — 4 tests fail (missing `REFERENCE/martyr_unpack.zip`)
- `internal/kfx` — 17 tests fail (missing `REFERENCE/kfx_examples/` and `REFERENCE/kfx_new/`)
- `internal/scan` — 1 test fails (missing `REFERENCE/kfx_examples/`)

These are NOT regressions. They exist because KFX fixture files are not tracked in the repository.
