# Parity Auditor Honesty Audit

**Date:** 2026-05 (this audit) · **Scope:** `scripts/audit_parity.py`,
`scripts/audit_branches.py`, `scripts/audit_missing_branches.py`, and the Go
conversion tree they measure.

## Why this document exists

Git history records that the parity metrics were once gamed and then honestly
reset (`1d9acf3` "Honest baseline: 26 uncertain branches... Previous 0 was from
cheating", `6e4525f` "Revert cheating"). By 2026-05 the metrics had drifted
back to perfect scores — `audit_parity.py --metric` reported **534/534
functions**, `audit_missing_branches.py` reported **4525/4525 branches** —
while the tree contained obvious one-line stubs (e.g. ~50 no-ops at the bottom
of `internal/kfx/epub_output.go`, ~120 in `internal/kfx/ion_binary.go`, and
`return fmt.Errorf("not implemented")` wrappers in `internal/kfx/yj_book.go`).

This document records how the inflation worked, what was done about it, and
what the honest numbers are.

## How the old auditors could be gamed

### Function audit (`audit_parity.py`)

The old audit matched **names only**, case-insensitively, across **all** Go
files. Any `func` declaration counted as an implementation of a same-named
Python function, regardless of its body. Consequences:

- `func generateEpub(outputPath string) error { return nil }` counted as a
  port of Python's 51-statement `EPUB_Output.generate_epub`.
- Stubs could live anywhere (global name index), so a name collision in an
  unrelated file satisfied parity.
- `snake_to_camel("__init__") == "Init"`, so every Python `__init__` matched
  whichever Go file had an `init()` function.
- Several sections of Go code carry the banner comment
  *"Missing Python functions — Ports from X. These stubs provide the
  Python-named API"* — they were written for the auditor, not for the program.
  **160 of 1327 Go functions are trivial AND have zero production call
  sites.**

### Branch audit (`audit_branches.py`)

- Branches were matched against the **whole Go file**, then against **all Go
  sources concatenated**, so evidence never had to be near the counterpart.
- Several strategies auto-returned "found" for shape-of-code heuristics:
  `if i == 0` → found; any variable-to-variable comparison → found; any
  `for` branch → found if the string `range ` appeared anywhere in the file;
  any `try/except` → found if `err` appeared anywhere in the file.
  `audit_missing_branches.py` summed these to a structurally-guaranteed 100%.

## What changed

| Tool | Change |
|---|---|
| `scripts/gofuncinfo` (new) | AST-based Go function scanner: statement count, composite-literal element count, trivial-body flags (empty / constant-or-identity returns / error-only returns / not-implemented admissions), call-graph counts. Tested in Go. |
| `scripts/audit_parity.py` | A match now requires name **and substance**. Trivial Go bodies against substantive Python functions are `stub_silent`/`stub_admitted`. One-line delegation wrappers count only if the transitive call closure carries real substance. Dunder false-matches fixed. Composite literals counted on both sides. Explicit, validated exclusions (below). |
| `scripts/parity_exclusions.json` (new) | Reviewable waiver manifest: fixed category enum, ≥10-char reason, and for architecture claims **evidence that must resolve to a named, verified-substantive Go function**. Invalid entries are rejected *and* ignored — a junk manifest cannot green the metric. |
| `scripts/audit_branches.py` | Branch matching is scoped to the resolved Go counterpart's **body**. Whole-file and all-sources search removed. Universal-pattern auto-founds reclassified as `weak` and excluded from coverage. |
| `scripts/audit_missing_branches.py` | In-process (60× faster); reports found/weak/missing/uncertain honestly. |
| `scripts/tests/` (new) | 48 unittest cases pinning all of the above, including "a same-named stub must not move the parity needle" and "a hollow counterpart must not produce branch coverage". |

## Classification: adapters vs shims

The audit distinguished two kinds of "same-named Go function that is not a
1:1 port":

1. **Genuine alternate-architecture adapters** — the behavior exists under a
   different shape, e.g. ION handling via `amazon-ion-go`
   (`decodeIonValue`/`decodeIonMap` with the embedded YJ catalog), EPUB
   packaging in `internal/epub` (`Write`, `contentOPF`, `tocNCX`,
   `navXHTML`), book decoding in `decodeKFX`, position/location logic under
   `BookPosLoc` methods, PDF outlines in `buildOutlineObjects`. These are
   **excluded with evidence** in `scripts/parity_exclusions.json`; the
   evidence functions are verified substantive by the auditor.
2. **Hollow name-only shims** — dead one-liners whose only consumer was the
   parity audit (e.g. `serializeValue → return nil`,
   `generateEpub → return nil`, `checkFragmentUsage`'s empty twin,
   `addPdfOutline → return nil` with a stale "not yet implemented" comment
   despite real outline support existing elsewhere). The honest auditor
   counts these as gaps unless a justified exclusion exists, and flags them
   `[dead: no call sites]` for deletion.

Out-of-scope-by-design items (Calibre output modes: single-KFX, CBZ, PDF
book conversion from YJ, KPF, zip-unpack, json-content; Calibre input
directory scanning; encode/serialize directions) are excluded under
`output-mode-out-of-scope` / `input-mode-out-of-scope` / `unused-direction`
with reasons — never counted as ports.

## Honest numbers (this audit)

Function audit (`python3 scripts/audit_parity.py --metric`):

```
537 Python functions audited
277 implemented + 40 delegation + 40 trivial↔trivial  = 357 present
167 excluded (explicit, validated, listed in PARITY_REPORT.md)
13 real gaps (6 silent stubs, 4 thin, 3 missing)
parity 96.5%   (strict — counting exclusions against parity: 66.5%)
```

Branch audit (`python3 scripts/audit_missing_branches.py --metric`):

```
4378 branches: 2341 strong-found, 0 weak, 0 missing, 2037 uncertain
53.5% strong coverage (uncertain = no verifiable counterpart body)
```

Both numbers are reproducible from the tree; `PARITY_REPORT.md` is the
generated detail view.

## Remaining real gaps (see PARITY_REPORT.md for locations)

1. **Dictionary books** — `process_dictionary_rules`,
   `unapply_dictionary_rule`, `is_drm_free_dictionary` are unported.
2. **`adjust_pixel_value`** — Go stub is an identity function; the Python
   `/100` scaling for PDF-backed books is absent.
3. **`fix_language`** — Python's language-suffix casing rules are not
   applied; Go's lowercase normalize can emit wrong EPUB language metadata.
4. **`have_content`** — 7-statement port of a 139-statement predicate.
5. **EPUB packaging behaviors with unproven coverage** —
   `do_remove_html_cover`, `add_generic_cover_page`, `save_book_parts`,
   `hide_element`: left as gaps pending investigation rather than
   silently excluded.
6. **Small hollow helpers** — `root_element`, `get_anchor_uri`,
   `replace_ion_data` (dead or 1–2 statements).

## Residual gaming vectors (documented, not eliminated)

- **Statement padding**: nstmt can be inflated with junk statements. The
  transitive-substance check makes this harder (padding must be reachable),
  but static metrics are ultimately gameable. The semantic arbiter is the
  golden-EPUB parity suite (`scripts/parity_diff.py`) plus the Go/Lua test
  suites — the auditors are tripwires, not proof.
- **Exclusion abuse**: bounded by the category enum, reason-length floor,
  evidence verification, staleness rejection, and the `strict_parity_pct`
  line which counts exclusions against parity.
- **Name-collision matching** across files remains name-based; substance
  checks make false positives loud (thin/stub) instead of silent.

## Maintenance

- Regenerate after Go/reference changes:
  `python3 scripts/audit_parity.py --report PARITY_REPORT.md`
- Auditor tests: `python3 -m unittest discover -s scripts/tests`
- Dumper tests: `go test ./scripts/gofuncinfo`
- When implementing a real gap, delete the corresponding exclusion entry (if
  any) — the auditor will re-classify automatically. When adding a new
  exclusion, expect review of its reason and evidence.
