# Parity Tooling Proposal — Making Python→Go Porting Verifiable

## The Problem

We're porting ~8,000 lines of Python across 9 files into Go. The current workflow is:

1. Read a Python function
2. Write the Go equivalent
3. Run `diff_kfx_parity.sh` (compares final EPUB output only)
4. Stare at the diff and try to figure out which function diverged

This is **end-to-end testing at the wrong level of granularity.** When the EPUB has 87 different files, tracing back to "which function in which file produced the wrong output" is painful and error-prone. The bugs keep happening in the same way: **branches get silently dropped** because the Python has 15 cases and the Go handles 3.

## What We Need

We need to compare **intermediate pipeline state** at each stage, not just the final EPUB. If we can answer "stage 10 (process_reading_order) produces matching HTML for section X but diverges at section Y, specifically in `process_content` when handling `$601` render mode" — that's a tractable bug. "87 files differ" is not.

## Proposal: Three Tools

### Tool 1: Python Trace Dumper (`scripts/trace_python.py`)

A script that runs the Python pipeline on a KFX file and dumps **structured snapshots at every pipeline stage** to a JSON file.

```
$ scripts/trace_python.py --input Martyr.kfx --output /tmp/parity_traces/Martyr.json
```

Output structure:
```json
{
  "input": "Martyr.kfx",
  "stages": {
    "organize_fragments": {
      "fragment_types": {"$145": 12, "$157": 8, ...},
      "fragment_ids": {"$260": ["s1", "s2", ...]},
      "book_symbol_format": "SHORT"
    },
    "metadata": {
      "title": "Martyr",
      "authors": ["..."],
      "language": "en",
      "orientation_lock": ...,
      "writing_mode": ...
    },
    "anchors": {
      "anchor_map": {"id1": {"uri": "...", "position": 100}, ...}
    },
    "navigation": {
      "toc": [{"label": "...", "href": "...", "children": [...]}],
      "guide": [...],
      "page_list": [...]
    },
    "reading_order": {
      "sections": {
        "s1": {
          "filename": "s1.xhtml",
          "body_html": "<div class=\"...\">...</div>",
          "body_class": "...",
          "used_styles": ["style_1", "style_2"]
        }
      }
    },
    "stylesheet": {
      "css": ".style_1 { ... }\n.style_2 { ... }\n",
      "class_count": 42,
      "classes": ["style_1", "style_2", ...]
    },
    "final_sections": {
      "s1": "<html>...complete XHTML...</html>"
    }
  }
}
```

**Implementation:** Monkey-patch the Python `KFX_EPUB.__init__` to capture state after each `self.process_*()` call. This is ~50 lines of Python — inject tracing hooks at known pipeline stages.

**Key design choice:** The trace captures data at the **same abstraction level** the Go code operates at. For `organize_fragments`, we capture the resulting dict structure. For `process_reading_order`, we capture the rendered HTML per section. For `create_css_files`, we capture the CSS string.

### Tool 2: Go Trace Dumper (`cmd/kindle-helper trace`)

Add a `trace` subcommand to the Go binary that does the same conversion but emits the same JSON structure:

```
$ go run ./cmd/kindle-helper trace --input Martyr.kfx --output /tmp/parity_traces/Martyr_go.json
```

**Implementation:** Add optional "trace hooks" at the same pipeline stages in `renderBookState`. When tracing is enabled (via `--trace` flag or `KFX_TRACE=1` env var), each stage dumps its state to a structured log. The JSON keys must match the Python trace exactly.

This requires:
1. A `traceWriter` that accumulates stage snapshots
2. Adding `trace.Stage("name", data)` calls at each pipeline checkpoint in `renderBookState`
3. A `trace` subcommand in `main.go` that runs the pipeline and writes the JSON

Estimated: ~200 lines of Go (mostly JSON struct definitions + hook points).

### Tool 3: Parity Comparator (`scripts/compare_traces.py`)

Takes two trace files and produces a structured diff:

```
$ scripts/compare_traces.py /tmp/parity_traces/Martyr.json /tmp/parity_traces/Martyr_go.json
```

Output:
```
STAGE: organize_fragments
  ✓ fragment_types: all counts match (15 types)
  ✓ fragment_ids: all IDs match
  ✓ book_symbol_format: SHORT == SHORT

STAGE: metadata
  ✓ title: "Martyr" == "Martyr"
  ✗ language: "en" != "en-US"  ← MISMATCH

STAGE: anchors
  ✓ anchor_map: 42/42 anchors match

STAGE: navigation
  ✓ toc: 8/8 entries match
  ✗ guide: extra entry in Go: {"type": "text", "href": "..."}
  ✓ page_list: 0 entries (both)

STAGE: reading_order
  ✓ section count: 12 == 12
  ✗ section "s3" body_html DIFFERS (first diff at char 447):
    Python: <div class="heading s3"><a class="s8"...
    Go:     <div class="heading"><a...
    ↑ Missing class on <a> inside heading

STAGE: stylesheet
  ✗ CSS class count: 42 != 38
  ✗ Missing classes (Python only): style_7, style_12, style_15
  ✗ Extra classes (Go only): class_x3, class_x9
  ✗ Differing classes:
    style_3: Python has "font-size: 1em", Go missing this property

SUMMARY: 5 stages match, 3 stages have differences
```

This turns "87 files differ" into "section s3 is missing the class attribute on `<a>` inside headings, and the CSS has 4 fewer classes."

---

## Implementation Plan

### Phase 1: Python Trace Dumper (1 session)

Create `scripts/trace_python.py`:
- Monkey-patches `KFX_EPUB.__init__` to intercept state between stages
- Captures: organize_fragments output, metadata, anchor map, nav tree, rendered sections (HTML), CSS, final sections
- Outputs structured JSON
- Run once per fixture book to create "golden" trace files
- Store golden traces in `REFERENCE/kfx_new/parity_traces/`

### Phase 2: Go Trace Hooks (1 session)

Add trace infrastructure to Go pipeline:
- `internal/kfx/trace.go` — trace writer struct, stage capture methods
- Add `trace.Stage("name", data)` calls in `renderBookState` at each checkpoint
- Add `trace` subcommand to `cmd/kindle-helper/main.go`
- Run against same fixtures, produce Go trace JSONs

### Phase 3: Comparator (0.5 session)

Create `scripts/compare_traces.py`:
- Deep comparison of two trace JSON files
- Stage-by-stage reporting with specific mismatch locations
- For HTML diffs: show context around first divergence
- For CSS diffs: use existing CSS parsing from `diff_report.sh`
- Exit code 0 if all stages match, 1 otherwise

### Phase 4: Integration (0.5 session)

Wire it all together:
- `scripts/parity_check.sh` — runs Python trace, Go trace, compare, for one or all fixture books
- Add to CI: `go test ./internal/kfx/ -run TestParityTraces` that runs the comparator
- Update `AGENTS.md` with the new workflow

---

## Additional Tooling: Branch Audit Script

A separate utility (`scripts/audit_branches.py`) that statically analyzes a Python function and lists every branch:

```
$ scripts/audit_branches.py --file yj_to_epub_content.py --function process_content
```

Output:
```
Function: process_content (yj_to_epub_content.py:390)
Branches: 23

  L391: isinstance(content, IonStruct) → process_struct_content
  L393: isinstance(content, IonSymbol) → process_symbol_content  
  L395: isinstance(content, IonString) → process_string_content
  L401: if content_layout == "$601" → inline image branch
  L403: if "$146" in content → has children
  L407: if child is IonString → text handling
  L409: if child is IonSymbol → symbol handling
  L411: if child is IonStruct → struct handling
  ... (23 total branches)

Go mapping check (yj_to_epub_content.go):
  ✓ L391: IonStruct branch → renderContentStruct (L412)
  ✓ L393: IonSymbol branch → processSymbolContent (L456)  
  ✗ L395: IonString branch → NO GO EQUIVALENT FOUND ← GAP
  ✓ L401: $601 inline image → renderInlineImage (L501)
  ...
```

This directly addresses the "we keep dropping branches" problem by making every branch visible and checkable.

Implementation: Python AST parsing (~150 lines). The "Go mapping check" part can be a grep-based heuristic — look for the same symbol names and patterns in the corresponding Go file. It won't be perfect but will catch obvious gaps.

---

## Why This Works

The current problem is a **visibility gap**. We can see:
- ✅ The Python source (read it)
- ✅ The Go source (read it)  
- ✅ The final EPUB diff (diff_kfx_parity.sh)
- ❌ **What happens at each intermediate stage**
- ❌ **Which specific branch was taken for a specific section**
- ❌ **Where exactly the Go output first diverges from Python**

These tools close the visibility gap. With trace comparison:
- You port a function
- Run `scripts/parity_check.sh --book Elvis`
- See exactly which stage diverges and what the first difference is
- Fix it, re-run, see it pass
- Move to the next function

With branch auditing:
- Before porting a function, run `audit_branches.py`
- See every branch you need to handle
- Check them off as you port each one
- No more "we only handled IonStruct and forgot IonString/IonSymbol"

---

## Prioritized Impact

| Tool | Effort | Impact |
|------|--------|--------|
| Python Trace Dumper | 1 session | **High** — creates the golden reference |
| Go Trace Hooks | 1 session | **High** — enables comparison |
| Parity Comparator | 0.5 session | **High** — turns raw data into actionable diffs |
| Branch Audit Script | 1 session | **Medium** — prevents the root cause (dropped branches) |

Total: ~3-4 sessions to build all four tools. After that, every subsequent porting session becomes much faster and more reliable.
