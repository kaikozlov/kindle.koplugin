---
name: go-port-worker
description: Ports a specific CSS simplification feature from Python to Go, with strict diff/test verification
---

# Go Port Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## When to Use This Skill

Use this worker for each of the 4 sequential porting steps:
1. Port filterDefaultParagraphMargins to simplifyStylesFull
2. Port COMPOSITE_SIDE_STYLES merging to simplifyStylesFull
3. Port ineffective property stripping to simplifyStylesFull
4. Batch 2 swap: replace *StyleDeclarations with processContentProperties

## Required Skills

None. This is a code-level Go porting task using file editing tools and shell commands.

## Work Procedure

### Step 1: Read the Python Reference

Read the specific Python function you're porting from `/home/kai/gitrepos/kobo.koplugin/REFERENCE/Calibre_KFX_Input/kfxlib/yj_to_epub_properties.py`. Understand every line. The Python code is the source of truth.

### Step 2: Read the Current Go Code

Read the target Go files:
- `internal/kfx/yj_to_epub_properties.go` — contains `simplifyStylesFull` and `simplifyStylesElementFull`
- `internal/kfx/kfx.go` — contains `*StyleDeclarations` functions and `filterDefaultParagraphMargins`
- `internal/kfx/yj_property_info.go` — contains `processContentProperties` and `convertYJProperties`

### Step 3: Save Baseline

Before making ANY changes, run the diff check and save the output:
```bash
cd /home/kai/gitrepos/kobo.koplugin/kindle.koplugin && go build ./internal/kfx/... && tmpdir=$(mktemp -d) && go run ./cmd/kindle-helper convert --input ../REFERENCE/kfx_examples/Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx --output "$tmpdir/out.epub" 2>/dev/null && diff -u <(unzip -p ../REFERENCE/martyr_calibre.epub OEBPS/stylesheet.css) <(unzip -p "$tmpdir/out.epub" OEBPS/stylesheet.css) | grep '^[+-]' | grep -v '^---\|^+++' | head -60
```

### Step 4: Implement

Make the minimal changes needed to port the Python feature. Key constraints:
- Do NOT add unit conversion to `propertyValue()`
- Do NOT add properties one-by-one to `*StyleDeclarations` functions
- Do NOT change HTML structure for image wrappers
- Do NOT modify Python reference code

### Step 5: Verify

Run BOTH verification gates:

1. **Build:** `cd /home/kai/gitrepos/kobo.koplugin/kindle.koplugin && go build ./internal/kfx/...`
2. **Test:** `cd /home/kai/gitrepos/kobo.koplugin/kindle.koplugin && go test ./internal/kfx/... -count=1 -timeout 120s`
   - Count FAIL lines: must be exactly 12
3. **Diff check:** Run the diff check command from Step 3
   - Compare output to baseline — functionally new diffs are NOT acceptable
   - **Cosmetic diffs ARE acceptable**: floating-point precision (3.200004em vs 3.2em), missing explicit margin defaults (margin-bottom: 0 absent vs present), class name renumbering, gray vs #808080, -webkit- prefix ordering, orphans/widows additions, properties matching defaults being absent

### Step 6: Commit or Revert

**If clean or only cosmetic diffs** (12 failures, no functionally new diffs):
```bash
git add -A && git commit -m "<step commit message from feature description>"
```

**If functionally new diffs exist** (wrong property values, missing properties that affect rendering):
```bash
git checkout -- .
git add -A && git commit -m "revert: <step name> attempt failed"
```
Then stop and report — do NOT try to fix it.

### Step 7: Use KFX_DUMP_STYLES if Ambiguous

If the diff check output is ambiguous (you're not sure if a diff is cosmetic or meaningful), use:
```bash
KFX_DUMP_STYLES=1 go run ./cmd/kindle-helper convert --input ../REFERENCE/kfx_examples/Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx --output /tmp/test.epub 2>/dev/null
```
This dumps intermediate style checkpoints for comparison.

## Example Handoff

```json
{
  "salientSummary": "Ported filterDefaultParagraphMargins from paragraphClass into simplifyStylesElementFull's inheritance-based approach. When div→p conversion happens, margin-top/margin-bottom: 1em are now recorded as inherited defaults and stripped by the existing comparison logic. Removed the explicit filterDefaultParagraphMargins call from paragraphClass.",
  "whatWasImplemented": "Modified simplifyStylesElementFull to record margin-top: 1em and margin-bottom: 1em as inherited when converting <div> to <p>. Removed filterDefaultParagraphMargins call from paragraphClass in kfx.go:3449. The filterDefaultParagraphMargins function is now dead code but left in place for reference.",
  "whatWasLeftUndone": "The filterDefaultParagraphMargins function in kfx.go:3886 is now unused but was not deleted to keep the diff minimal.",
  "verification": {
    "commandsRun": [
      {"command": "go build ./internal/kfx/...", "exitCode": 0, "observation": "Build succeeded"},
      {"command": "go test ./internal/kfx/... -count=1 -timeout 120s 2>&1 | grep -c '^--- FAIL'", "exitCode": 0, "observation": "12 failures (baseline maintained)"},
      {"command": "diff check vs reference martyr_calibre.epub", "exitCode": 0, "observation": "No new diff lines vs baseline"}
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

- New test failures introduced (count > 12)
- New CSS diffs that are NOT cosmetic
- The Python feature cannot be cleanly ported without architectural changes
- Uncertainty about whether a diff is cosmetic or meaningful — stop and ask
- The step was reverted — report the failure and await instructions
