---
name: go-refactor-worker
description: Moves Go code between files within package kfx to match Python file layout
---

# Go Refactor Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## When to Use This Skill

Features that involve moving functions, types, and variables between Go files within `internal/kfx/` to align file naming with the Calibre Python reference layout. All work stays in `package kfx` — no subpackages.

## Required Skills

None — this is pure Go code reorganization.

## Work Procedure

### 1. Read the feature description and understand the scope

Read the feature description carefully. Identify:
- Source file(s) — where code currently lives
- Target file(s) — where code needs to move
- Specific symbols (funcs, types, vars) to move
- Symbols that must stay in the source file

Read `REFERENCE/Calibre_KFX_Input/kfxlib/<python_module>.py` to understand what the target Go file should conceptually contain.

### 2. Read the source files

Read ALL source files mentioned in the feature. For each symbol being moved:
- Note its exact line range (including comments/doc above it)
- Note which imports it needs
- Note which other symbols it references (to ensure they're available)

### 3. Create the target file (if new)

If the target file doesn't exist:
1. Create it with `package kfx` header
2. Add only the imports needed by the moved code
3. Copy moved code exactly — no logic changes, no renaming, no reformatting

### 4. Move code to target

For each symbol to move:
1. Copy the exact code (including comments) to the target file
2. Delete it from the source file
3. Clean up any now-unused imports in the source file

**Critical rules:**
- Do NOT modify function bodies, signatures, or logic
- Do NOT change variable names or type names
- Do NOT add or remove comments within function bodies
- Do NOT reformat code beyond what's needed for the move
- Ensure moved code can access all symbols it references (same package, so this should be automatic)

### 5. Verify after EVERY move

After moving each group of related symbols:
1. Run `go build ./cmd/kindle-helper/` — must compile
2. Run `go test ./internal/kfx/ -count=1 -timeout 120s` — must pass with same test count
3. Run `go vet ./internal/kfx/...` — must be clean

If any check fails:
- **STOP. Do NOT continue moving code.**
- Fix the issue (usually missing import or accidentally deleted code)
- If you can't fix it in 2 attempts, revert to last known-good state and return to orchestrator

### 6. Commit after each successful file operation

After each group of moves where all verification passes:
```bash
git add -A
git commit -m "refactor: move <what> from <source> to <target> (aligns with <python_module>.py)"
```

### 7. Final verification

After all moves for the feature are complete:
1. `go build ./cmd/kindle-helper/`
2. `go test ./internal/kfx/ -count=1 -timeout 120s`
3. `go vet ./internal/kfx/...`
4. Verify source file no longer contains moved symbols: `grep -c '<symbol>' <source_file>` → 0
5. Verify target file contains all expected symbols: `grep -c '<symbol>' <target_file>` → expected count
6. Verify test count matches baseline (note the count in your handoff)

### 8. Update doc.go ONLY if the feature says to

Only update doc.go if the feature explicitly requires it. When updating:
- Read current doc.go
- Update the file mapping table to reflect new files
- Keep the package comment structure intact

## Example Handoff

```json
{
  "salientSummary": "Created yj_symbol_catalog.go by extracting 12 symbols from decode.go (8 functions/vars) and kfx.go (symbolResolver struct + 4 methods). Verified go test passes with same test count (94 tests).",
  "whatWasImplemented": "Moved sharedTable, sharedCatalog, ionSystemSymbolCount, yjPreludeOnce/yjPreludeData/yjPreludeErr, yjPrelude(), symbolResolver struct, newSymbolResolver(), Resolve(), isLocalSID(), isSharedSymbolText() from decode.go and kfx.go into new yj_symbol_catalog.go. Cleaned up unused imports in both source files.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      {"command": "go test ./internal/kfx/ -count=1 -timeout 120s", "exitCode": 0, "observation": "94 tests passed, 0 failures, 8.5s"},
      {"command": "go vet ./internal/kfx/...", "exitCode": 0, "observation": "clean"},
      {"command": "go build ./cmd/kindle-helper/", "exitCode": 0, "observation": "binary produced"},
      {"command": "grep -c 'sharedTable\\|sharedCatalog\\|symbolResolver' internal/kfx/decode.go", "exitCode": 0, "observation": "0 matches (all moved)"},
      {"command": "grep -c 'func sharedTable\\|func sharedCatalog\\|type symbolResolver' internal/kfx/yj_symbol_catalog.go", "exitCode": 0, "observation": "4 matches (all present)"}
    ],
    "interactiveChecks": []
  },
  "tests": {
    "added": []
  },
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- `go test` fails and you can't fix it in 2 attempts
- A symbol to move has dependencies that would require moving more code than specified
- You discover the feature description is inaccurate (wrong source file, missing symbols, etc.)
- A move would require changing function signatures or breaking the public API
- Import cycle would be created (shouldn't happen since everything is same package, but report if it does)
