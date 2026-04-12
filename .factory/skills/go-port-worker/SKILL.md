---
name: go-port-worker
description: Ports CSS simplification features from Python to Go, with strict multi-file diff/test verification
---

# Go Port Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## When to Use This Skill

Use for all features in this mission: dead code cleanup, diff auditing, precision fixes, missing feature ports, and iterative diff reduction.

## Required Skills

None. This is a code-level Go porting task using file editing tools and shell commands.

## Work Procedure

### Step 1: Understand the Feature

Read the feature description carefully. If it references Python code, read it from `/home/kai/gitrepos/kobo.koplugin/REFERENCE/Calibre_KFX_Input/kfxlib/yj_to_epub_properties.py` (or other files in that directory).

Read the target Go files:
- `internal/kfx/yj_to_epub_properties.go` — `simplifyStylesFull`, `simplifyStylesElementFull`
- `internal/kfx/kfx.go` — rendering pipeline, *StyleDeclarations (dead code)
- `internal/kfx/yj_property_info.go` — `processContentProperties`, `convertYJProperties`, `convertStyleUnits`

### Step 2: Save Baseline Diffs

Before making ANY changes, run diff checks on ALL 4 test files and save the counts:

```bash
cd /home/kai/gitrepos/kobo.koplugin/kindle.koplugin && for pair in "../REFERENCE/kfx_examples/Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx /tmp/martyr_python_ref.epub" "../REFERENCE/KFX_DRM/decrypted/Elvis and the Underdogs_B009NG3090_decrypted.kfx-zip /tmp/Elvis and the Underdogs_B009NG3090_calibre.epub" "../REFERENCE/KFX_DRM/decrypted/The Hunger Games Trilogy_B004XJRQUQ_decrypted.kfx-zip /tmp/The Hunger Games Trilogy_B004XJRQUQ_calibre.epub" "../REFERENCE/KFX_DRM/decrypted/Three Below (Floors #2)_B008PL1YQ0_decrypted.kfx-zip /tmp/Three Below (Floors #2)_B008PL1YQ0_calibre.epub"; do
  input=$(echo "$pair" | cut -d' ' -f1)
  ref=$(echo "$pair" | cut -d' ' -f2-)
  bn=$(basename "$input" | sed 's/_decrypted.*//' | sed 's/\..*//')
  tmpdir=$(mktemp -d)
  count=$(go run ./cmd/kindle-helper convert --input "$input" --output "$tmpdir/out.epub" 2>/dev/null && diff -u <(unzip -p "$ref" OEBPS/stylesheet.css) <(unzip -p "$tmpdir/out.epub" OEBPS/stylesheet.css) | grep '^[+-]' | grep -v '^---\|^+++' | wc -l)
  echo "BASELINE $bn: $count diffs"
  rm -rf "$tmpdir"
done
```

### Step 3: Implement

Make the minimal changes needed. Key constraints:
- Do NOT add unit conversion to `propertyValue()`
- Do NOT change HTML structure for image wrappers
- Do NOT modify Python reference code

### Step 4: Verify Against ALL Test Files

1. **Build:** `go build ./internal/kfx/...`
2. **Test:** `go test ./internal/kfx/... -count=1 -timeout 120s` — must be 10 FAIL lines or fewer
3. **Multi-file diff check:** Run the diff count command from Step 2 on ALL 4 test files
   - Compare counts to baseline — diffs must NOT increase on any file
   - Diffs should decrease or stay the same on all files
   - **Cosmetic diffs are acceptable**: class renumbering, gray vs #808080, -webkit- prefix ordering, whitespace

### Step 5: Commit or Revert

**If clean or improved** (10 failures or fewer, no increased diffs on any file):
```bash
git add -A && git commit -m "<commit message>"
```

**If diffs increased on any file, or new test failures:**
```bash
git checkout -- .
git add -A && git commit -m "revert: <step name> attempt failed"
```
Then stop and report.

### Step 6: Use KFX_DUMP_STYLES if Ambiguous

```bash
KFX_DUMP_STYLES=1 go run ./cmd/kindle-helper convert --input <INPUT> --output /tmp/test.epub 2>/dev/null
```

## Reference EPUB Generation

If reference EPUBs are missing from /tmp/, regenerate them:
```bash
cd /home/kai/gitrepos/kobo.koplugin/kindle.koplugin
python3 scripts/convert_kfx_python.py --input <INPUT> --output <OUTPUT>
```

Test files:
- Martyr: `../REFERENCE/kfx_examples/Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx`
- Elvis: `../REFERENCE/KFX_DRM/decrypted/Elvis and the Underdogs_B009NG3090_decrypted.kfx-zip`
- Hunger Games: `../REFERENCE/KFX_DRM/decrypted/The Hunger Games Trilogy_B004XJRQUQ_decrypted.kfx-zip`
- Three Below: `../REFERENCE/KFX_DRM/decrypted/Three Below (Floors #2)_B008PL1YQ0_decrypted.kfx-zip`

## Example Handoff

```json
{
  "salientSummary": "Removed dead code (paragraphStyleDeclarations, linkStyleDeclarations, imageWrapperStyleDeclarations, imageStyleDeclarations, filterDefaultParagraphMargins). Verified all 4 test files unchanged.",
  "whatWasImplemented": "Deleted 5 unused functions from kfx.go. No functional changes. Build and tests pass with 12 baseline failures.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      {"command": "go build ./internal/kfx/...", "exitCode": 0, "observation": "Build succeeded"},
      {"command": "go test ./internal/kfx/... -count=1 -timeout 120s", "exitCode": 0, "observation": "10 failures (baseline)"},
      {"command": "diff count all 4 files", "exitCode": 0, "observation": "Martyr:1 Elvis:42 HG:38 3B:10 — unchanged from baseline"}
    ],
    "interactiveChecks": []
  },
  "tests": {"added": []},
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- New test failures (count > 10)
- Diffs increased on any test file
- Python feature cannot be cleanly ported
- Uncertainty about whether a diff is cosmetic or meaningful
- Step was reverted — report failure and await instructions
