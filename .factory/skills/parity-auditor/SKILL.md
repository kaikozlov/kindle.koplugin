---
name: parity-auditor
description: Audits a single Python file against its Go counterpart, identifies every missing branch, and fixes all gaps.
---

# Parity Auditor

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## When to Use This Skill

This worker handles features that audit a single Python source file against its Go counterpart(s) in the KFX→EPUB conversion pipeline. Each feature specifies exactly which Python file and which Go file(s) to audit.

## Required Skills

None. This is a code audit and implementation skill using standard Go tooling.

## Work Procedure

### Phase 1: Understand the Assignment

1. Read the feature description carefully. It specifies:
   - The Python file to audit (e.g., `yj_to_epub_properties.py`)
   - The Go file(s) to compare against (e.g., `yj_to_epub_properties.go`)
   - Specific functions or areas to focus on

2. Read `mission.md` and `AGENTS.md` for mission context and constraints.

### Phase 2: Branch Audit (THE CORE WORK)

For EVERY function in the Python file that has Go counterparts:

1. **Run the branch audit tool** as a starting point:
   ```bash
   python3 scripts/audit_branches.py --file PYTHON_FILE.py --function FUNCTION_NAME --verbose
   ```
   This gives you a preliminary branch list. Do NOT trust it completely — it is heuristic-based.

2. **Read the Python function completely.** For each function:
   - List EVERY branch: every `if`, `elif`, `else`, `for`, `try`, `except`, ternary, `isinstance`, type dispatch.
   - Record the Python line number and what each branch does.
   - Document this in your working notes.

3. **Read the corresponding Go function.** For each Python branch:
   - Find the specific Go code (file + line) that corresponds.
   - If no Go code exists for a Python branch, flag it as a GAP.
   - If Go code exists but produces different behavior, flag it as a MISMATCH.

4. **Pay special attention to RED FLAGS from AGENTS.md:**
   - `if elem.Tag != X { continue }` — Python handles other tags too
   - `if _, ok := asMap(child); !ok { continue }` — Python handles non-map children
   - `// Skip for now` — it IS needed

### Phase 3: Implement Fixes

For every GAP and MISMATCH found:

1. **Write the fix** — port the Python branch faithfully to Go.
   - Match the Python logic exactly, including error handling and edge cases.
   - Do NOT simplify type dispatches.
   - Do NOT skip branches that seem unreachable.
   - If a branch seems unreachable, add a `log.Errorf` for it.

2. **Verify the fix compiles:**
   ```bash
   go build ./cmd/kindle-helper/
   ```

3. **Run unit tests:**
   ```bash
   go test ./internal/kfx/ -count=1 -timeout 120s
   ```

4. **If tests fail or unexpected diffs appear:** REVERT immediately. Commit the revert. Report back to orchestrator.

5. **If tests pass:** Commit the fix with a descriptive message.

### Phase 4: Verify No Regressions

After all fixes for this feature are committed:

```bash
go build ./cmd/kindle-helper/ && bash /tmp/compare_all.sh
```

The original 6 books MUST stay at 394/394. If any regression:
- REVERT immediately
- Commit the revert
- Report to orchestrator with details

### Phase 5: Document Findings

In your handoff, include:
- Complete list of functions audited
- For each function: number of branches found, gaps found, fixes applied
- Any branches you believe are genuinely unreachable (with evidence)
- Any areas where Python behavior is ambiguous or seems buggy
- Test results and comparison results

## Example Handoff

```json
{
  "salientSummary": "Audited yj_to_epub_properties.py simplify_styles function (300+ lines, 45 branches). Found 3 missing branches in Go: text-decoration-color→none !important conversion, negative padding discard, and outline-width removal. Fixed all 3. Unit tests pass, original 6 books remain 394/394.",
  "whatWasImplemented": "Added text-decoration-color→text-decoration:none !important conversion in convertYJProperties (yj_to_epub_properties.go:285). Added negative padding value discard in simplifyStylesElementFull (line 1710). Added outline-width removal when outline-style is none (line 1801). All three fixes match Python line-by-line.",
  "whatWasLeftUndone": "Did not audit create_css_files function (lines 2239-2265) — that was not in scope for this feature. Did not investigate the 4-entry size difference in HERITABLE_PROPERTIES set.",
  "verification": {
    "commandsRun": [
      {"command": "go build ./cmd/kindle-helper/", "exitCode": 0, "observation": "Builds cleanly"},
      {"command": "go test ./internal/kfx/ -count=1 -timeout 120s", "exitCode": 0, "observation": "All tests pass"},
      {"command": "bash /tmp/compare_all.sh", "exitCode": 0, "observation": "394/394 match, 0 regressions"}
    ],
    "interactiveChecks": []
  },
  "tests": {
    "added": []
  },
  "discoveredIssues": [
    {"severity": "medium", "description": "HERITABLE_PROPERTIES set in Go has 4 fewer entries than Python — may cause incorrect inheritance stripping", "suggestedFix": "Compare Python HERITABLE_DEFAULT_PROPERTIES keys against Go heritableProperties map"}
  ]
}
```

## When to Return to Orchestrator

- Feature depends on a function that hasn't been audited yet (coordinate with orchestrator)
- Python behavior is genuinely ambiguous — cannot determine correct port without user input
- A fix causes regressions in the original 6 books that cannot be resolved within the feature scope
- The Python file is significantly larger than expected and the feature scope needs to be split
