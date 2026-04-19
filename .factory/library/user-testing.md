# User Testing Guide

## Project Overview

This is a Go library for KFX→EPUB conversion. There are no services to start — all testing is via `go test`.

## Testing Surface

### Go Test (Unit Tests)

**Tool:** `go test` (direct shell execution)
**Command:** `go test ./internal/kfx/... -count=1 -run "^Test[^CBS]" -v`

The `-run "^Test[^CBS]"` filter excludes fixture-dependent tests (TestCBS*).

**No external services needed.** This is a pure Go library with synthetic test data only.

## Resource Cost Assessment

### Validation Concurrency

| Surface | Max Concurrent Validators | Reasoning |
|---------|--------------------------|-----------|
| go-test | 3 | Tests are CPU-bound, pure computation. No shared state between test runs. `go test` itself parallelizes within a single run. Multiple concurrent `go test` processes share no mutable state. |

## Flow Validator Guidance: go-test

### Isolation Rules

- Each flow validator runs `go test` with specific `-run` patterns
- No shared mutable state between validators — each test run is independent
- Tests create synthetic data in-memory and tear down automatically
- No file system writes outside of test cache

### Test Patterns by Assertion Group

**Book Type Dispatch (VAL-M2-DISPATCH-001 through VAL-M2-DISPATCH-005):**
```
go test ./internal/kfx/... -count=1 -run "TestDetectBookType|TestProcessSectionComic|TestProcessSectionMagazine|TestProcessSectionReflowable|TestProcessSectionPrintReplica|TestProcessSectionScribe|TestProcessSectionChildren|TestProcessSectionWith|TestProcessReadingOrder" -v
```

**Position/Location Core (VAL-M2-POSITION-001 through VAL-M2-POSITION-011):**
```
go test ./internal/kfx/... -count=1 -run "TestCollectContentPositionInfo|TestCollectPositionMapInfo|TestCreateLocationMap|TestCreateApproximatePageList|TestCreatePositionMap|TestCheckPositionAndLocationMaps|TestPidForEid|TestPositionOfAnchor" -v
```

**Edge Fixes (VAL-M2-EDGE-001 through VAL-M2-EDGE-010):**
```
go test ./internal/kfx/... -count=1 -run "TestAnchorEidOffset|TestDetermineApproximatePages_Rune|TestContentChunk_NilVsEmpty|TestHasNonImageRenderInline|TestCollectLocationMapInfo|TestCreateApproximatePageList_Double|TestCollectPositionInfo_Story|TestProcessPageSpreadLeaf|TestContentChunkConstruction|TestContentChunkEqual" -v
```

**Notebook/Scribe (VAL-M4-NB-001 through VAL-M4-NB-008):**
```
go test ./internal/kfx/... -count=1 -run "TestProcessNotebookContent|TestScribeNotebookStroke|TestBrushType|TestScribeNotebookAnnotation|TestDecodeStrokeValues|TestAdjustColorForDensity" -v
```

**Image Book (VAL-M4-IB-001 through VAL-M4-IB-008):**
```
go test ./internal/kfx/... -count=1 -run "TestGetOrderedImages|TestConvertBookToCBZ|TestConvertBookToPDF|TestCropImage|TestCBZ|TestCombineImages|TestTOCPage|TestGetResourceImage" -v
```

**M5 Cleanup - Ion/Symbol/Section (VAL-M5-ION-001, VAL-M5-SYM-001, VAL-M5-SEC-001):**
```
go test ./internal/kfx/... -count=1 -run "TestIonTypeParity|TestSymbolTableWritePathNotNeeded|TestSectionProcessingLogging" -v
```

**M5 Cleanup - Context/Position/Version/Prop/Frag (VAL-M5-CTX-001, VAL-M5-POS-001, VAL-M5-VER-001, VAL-M5-PROP-001, VAL-M5-FRAG-001):**
```
go test ./internal/kfx/... -count=1 -run "TestConnectedPaginationDefault|TestPositionLocationInternals|TestVersionConstantsParity|TestMapBoxAlign|TestRebuildCallsAllSteps" -v
```

**M5 Cleanup - Image Book/Format/Cross (VAL-M5-IB-001, VAL-M5-FMT-001, VAL-M5-CROSS-001):**
```
go test ./internal/kfx/... -count=1 -run "TestSuffixLocationParity|TestValueStrNilHandling|TestLowSeverityNonRegressive" -v
```

### Boundaries

- Do NOT modify any source files — only read and run tests
- Do NOT run tests that require real KFX fixtures (TestCBS*)
- Evidence: capture test output (pass/fail status per test name)
