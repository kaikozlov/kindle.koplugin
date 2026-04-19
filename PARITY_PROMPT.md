I'm working on the kindle.koplugin project — a Go port of Calibre's KFX Input plugin that converts Amazon KFX ebook files into EPUB format.

## The Rule

The Python code in `REFERENCE/Calibre_KFX_Input/` is the **sole source of truth**. Every line of our Go implementation must map to it. There is a three-fold parity requirement:

1. **Structural** — If Python has a file in one place, Go has a file with the same name in the corresponding place.
2. **Function-level** — If Python has a function in a particular file, Go has a function with the same name and purpose in the corresponding place.
3. **Logic-level** — If Python's function does X and returns Y, Go's function does the same thing in the same order and returns Y.

## Your Task

Investigate the current state of parity between the Go code (`internal/`) and the Python reference (`REFERENCE/Calibre_KFX_Input/kfxlib/`). Determine where gaps exist at each of the three levels, then produce a prioritized, stepwise plan for closing those gaps.

## What to Look At

- `REFERENCE/Calibre_KFX_Input/kfxlib/` — the Python source of truth (36 files)
- `internal/kfx/` — the Go implementation
- `internal/epub/`, `internal/jxr/`, `internal/scan/`, `internal/jsonout/` — supporting Go packages
- `AGENTS.md` — project work rules (read this first)
- `.factory/library/` — any existing analysis documents
- `scripts/` — parity testing and diff tools

## Constraints

- Read AGENTS.md before you start — it contains hard rules about commits, reverts, and the relationship between the Python and Go code.
- The Python code is NEVER modified except for debug logging. It is the instruction manual.
- Every change must be tested. Every step must be committed.
- The resolution order is Structure → Function → Logic — fix the foundation before the walls, fix the walls before the paint.
- `REFERENCE/Calibre_KFX_Input/*.py` (top-level files like `action.py`, `config.py`, `jobs.py`) are Calibre plugin infrastructure — they do NOT need Go ports. The Lua code in `src/` replaces them.
- The core conversion library is in `kfxlib/` — that's what we're porting.

## Deliverable

A markdown plan with:
1. A structural audit (file-by-file comparison)
2. A function-level gap list (which Python functions are missing/wrong in Go)
3. A logic-level gap list (which Go functions exist but behave differently from Python)
4. A prioritized, stepwise execution plan ordered Structure → Function → Logic
5. Size estimates for each step

Write the plan to `PLAN.md` in the project root.
