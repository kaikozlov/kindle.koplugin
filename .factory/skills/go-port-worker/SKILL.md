---
name: go-port-worker
description: Ports Python functions from the Calibre KFX Input reference to Go, maintaining 1:1 parity.
---

# Go Port Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## When to Use This Skill

This worker handles all features that involve porting Python functions from `REFERENCE/Calibre_KFX_Input/kfxlib/` to Go in `internal/kfx/` (and occasionally `internal/epub/`, `internal/jxr/`, etc.). Every feature follows the same pattern: read Python source, port to Go with tests, verify parity.

## Required Skills

None. This is a pure Go code porting worker using `go test` for verification.

## Work Procedure

### Step 0: Read the Mission Context

Read these files BEFORE starting any work:
1. `AGENTS.md` (in missionDir) — mission boundaries, constraints, coding conventions
2. `.factory/library/architecture.md` — how the system works
3. `.factory/library/python-porting-guide.md` — the porting methodology
4. `.factory/research/stream-{a,b,cd}-reference-details.md` — detailed Python reference for YOUR feature's stream

### Step 1: Study the Python Reference

For the assigned feature, locate the exact Python source using the feature description's `Python Reference` section:
1. Open the exact file in `REFERENCE/Calibre_KFX_Input/kfxlib/`
2. Go to the exact line range specified in the feature description
3. Read the COMPLETE function/class body — do NOT skim
4. Identify:
   - All input parameters and their types
   - All return values
   - All data structures (classes, dicts, sets)
   - All conditional branches
   - All dependencies on other kfxlib functions

### Step 2: Study the Existing Go Code

1. Read the Go file(s) that will be modified or that the new code will be added to
2. Understand the existing Go data structures and naming conventions
3. Check if any helper functions already exist that can be reused
4. Note the existing test patterns in `*_test.go` files in the same package

### Step 3: Write Tests First (RED)

Write failing tests BEFORE any implementation:

1. Create a test file (or add to existing `*_test.go` in the same package)
2. Use synthetic Go test data — construct Ion structs, maps, slices manually
3. Test the PORTED behavior, not the Python syntax:
   - If Python returns a dict, Go should return a map with equivalent structure
   - If Python uses class methods, Go should use methods on a struct
   - If Python logs warnings, Go should log warnings (test observable side effects)
4. Cover at minimum:
   - Happy path with typical data
   - Edge cases (empty input, nil values, zero-length data)
   - Each conditional branch in the Python code
5. Run `go test ./internal/kfx/... -run TestYourFeature -v` and confirm tests FAIL (RED)

### Step 4: Implement (GREEN)

Port the Python code to Go:

1. Follow Go naming conventions (CamelCase, not snake_case)
2. Match Python function names closely: `organize_fragments_by_type` → `organizeFragmentsByType` or into existing `organizeFragments`
3. Use Go idioms for error handling (not Python exceptions)
4. Preserve the EXACT logic flow — same conditionals, same order of operations
5. For data tables/constants: use Go maps or slices, preserving all key/value pairs
6. For regex patterns: translate to Go regexp syntax carefully
7. Run `go test ./internal/kfx/... -run TestYourFeature -v` and confirm tests PASS (GREEN)

### Step 5: Verify No Regressions

1. Run `go test ./internal/kfx/... -v` — ALL tests must pass (not just yours)
2. Run `go vet ./internal/kfx/...` — no issues
3. If ANY existing test breaks:
   - REVERT immediately with `git checkout -- .`
   - Commit the revert
   - Investigate why the test broke before retrying

### Step 6: Commit

1. `git add` the changed files
2. `git diff --cached` — review for accidental changes, debug prints, etc.
3. `git commit` with a message following the pattern:
   `feat: port [function_name] from [python_file]:[line_range]`
   or
   `fix: complete [function_name] parity with [python_file]:[line_range]`

### Step 7: Manual Verification

1. Read through the ported Go code side-by-side with the Python source
2. Verify every conditional branch has a Go equivalent
3. Verify every data value is preserved
4. Note any INTENTIONAL deviations (Go error handling, type system differences) in the handoff

## Example Handoff

```json
{
  "salientSummary": "Ported classify_symbol from yj_structure.py:1042-1087 to symbol_format.go with full regex parity. All 7 symbol type groups (SHARED, COMMON, ORIGINAL, SHORT, BASE64, DICTIONARY, UNKNOWN) now classify correctly. 12 new unit tests added covering all branches.",
  "whatWasImplemented": "Completed classifySymbolWithResolver in symbol_format.go to match Python yj_structure.py:1042-1087. Added allowed_symbol_prefix check (line 1089). Added check_symbol_table equivalent (lines 1099-1149). All 7 regex pattern groups ported with identical matching logic.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      { "command": "go test ./internal/kfx/... -run TestClassifySymbol -v", "exitCode": 0, "observation": "12 tests passing" },
      { "command": "go test ./internal/kfx/... -v", "exitCode": 0, "observation": "All 35 tests passing, no regressions" },
      { "command": "go vet ./internal/kfx/...", "exitCode": 0, "observation": "No issues" }
    ],
    "interactiveChecks": []
  },
  "tests": {
    "added": [
      {
        "file": "internal/kfx/symbol_format_test.go",
        "cases": [
          { "name": "TestClassifySymbolShared", "verifies": "Shared symbols classified via isSharedSymbolText" },
          { "name": "TestClassifySymbolCommon", "verifies": "Common symbols matched by exact string" },
          { "name": "TestClassifySymbolOriginal", "verifies": "12 regex patterns for V_, res_, anchor- prefixes" },
          { "name": "TestClassifySymbolShort", "verifies": "Short resource names matched" },
          { "name": "TestClassifySymbolBase64", "verifies": "22+ char base64 names matched" },
          { "name": "TestClassifySymbolDictionary", "verifies": "G[0-9]+ and yj.dictionary.* patterns" },
          { "name": "TestClassifySymbolUnknown", "verifies": "Unmatched symbols return UNKNOWN" },
          { "name": "TestClassifySymbolPriority", "verifies": "Priority order: SHARED > COMMON > ORIGINAL > SHORT > BASE64 > DICTIONARY > UNKNOWN" },
          { "name": "TestAllowedSymbolPrefix", "verifies": "Valid prefixes accepted, invalid rejected" },
          { "name": "TestCheckSymbolTable", "verifies": "Unknown symbols logged as warnings" },
          { "name": "TestDetermineBookSymbolFormatQuorum", "verifies": "Quorum calculation matches Python" },
          { "name": "TestDetermineBookSymbolFormatPriority", "verifies": "SHORT > DICTIONARY > BASE64 > ORIGINAL" }
        ]
      }
    ]
  },
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- Feature depends on a function that doesn't exist yet in Go (and isn't part of this feature's scope)
- Python reference code is ambiguous and the correct behavior can't be determined
- A pre-existing test fails and the cause is outside this feature's scope
- The feature's scope is larger than described in the feature description
- Go type system prevents exact Python parity (e.g., Python's dynamic typing) — describe the adaptation needed
