# Autoresearch: Python→Go Structural Parity

## Objective
Achieve 1:1 structural parity between every Python function in `REFERENCE/Calibre_KFX_Input/kfxlib/` and its Go counterpart in `internal/kfx/`. Every Python `def` must exist as a separate, named Go `func` — no inlining.

## Metrics
- **Primary**: `missing_functions` (count, lower is better) — Python functions with no named Go counterpart
- **Secondary**: `total_functions` — total Python functions across all audited files
- **Secondary**: `matched_functions` — functions that have a named Go counterpart
- **Secondary**: `parity_pct` — percentage of functions matched

## How to Run
`./autoresearch.sh` — runs `scripts/audit_parity.py --metric`

## Approach

### Priority Order (by impact on conversion correctness)
1. **Core conversion files first** (most diffs originate here):
   - `yj_to_epub_content.py` (31 functions, 18 missing)
   - `yj_to_epub_properties.py` (58 functions, 45 missing)
   - `yj_to_epub_misc.py` (17 functions, 10 missing)
   - `yj_to_epub_navigation.py` (27 functions, 20 missing)
   - `yj_to_epub.py` (15 functions, 12 missing)
   - `yj_to_epub_metadata.py` (5 functions, 5 missing)
   - `yj_to_epub_resources.py` (9 functions, 6 missing)
2. **Supporting conversion files**:
   - `yj_to_epub_illustrated_layout.py` (6 functions, 2 missing)
   - `yj_to_epub_notebook.py` (9 functions, 1 missing)
   - `yj_to_image_book.py` (10 functions, 7 missing)
3. **Data structure files**:
   - `yj_container.py` (31 functions, 28 missing)
   - `yj_book.py` (17 functions, 17 missing)
   - `yj_metadata.py` (33 functions, 12 missing)
   - `yj_position_location.py` (32 functions, 13 missing)
   - `yj_structure.py` (25 functions, 11 missing)
4. **Infrastructure files**:
   - `epub_output.py` (70 functions, 66 missing)
   - `ion.py` (47 functions, 47 missing)
   - `ion_binary.py` (50 functions, 50 missing)
   - `ion_symbol_table.py` (25 functions, 23 missing)
   - `kfx_container.py` (8 functions, 8 missing)
   - `yj_symbol_catalog.py` (1 function, 1 missing)
   - `yj_versions.py` (6 functions, 1 missing)

### Process Per Function
1. Read the Python function source (`REFERENCE/Calibre_KFX_Input/kfxlib/<file>.py`)
2. Read the Pytago transpilation (`REFERENCE/pytago_test_new/go_output/<file>.go`) if available
3. Check if the logic already exists in Go under a different name or inlined into another function
4. If inlined: extract into a separate named function
5. If missing: implement as a new function following pytago output as reference
6. Every `if`, `elif`, `for`, `try`, ternary, and type dispatch must have a Go counterpart
7. Run `go test ./internal/kfx/...` to verify no regressions

### Function Naming Convention
- Python `snake_case` → Go `camelCase` (unexported)
- Python `Class.method` → Go standalone function (with receiver if appropriate)
- Python `__init__` → Go `NewClass()` or part of struct init
- Python `__repr__`/`__str__` → Go `String()` method
- Python dunder methods → Go idiomatic equivalents

## Files in Scope
- `internal/kfx/*.go` — all Go conversion pipeline files
- `scripts/audit_parity.py` — the parity audit tool itself

## Off Limits
- `REFERENCE/` — Python source is read-only
- `scripts/` (other than audit_parity.py) — tooling is stable
- `lua/` — frontend is separate concern
- `internal/kfx/catalog.ion` — symbol catalog is golden data
- `internal/kfx/testdata/` — golden test files

## Constraints
- All Go tests must pass (`go test ./internal/kfx/...`)
- No new external dependencies
- Code must compile
- Every change must map to specific Python source (file, line, branch)

## What's Been Tried
- Baseline: 403 missing functions out of 532 total (24.2% parity)
