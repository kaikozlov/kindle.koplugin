# Parity Auditor Honesty Audit

**Updated:** 2026-08-25 · **Reference:** KFX Input 2.34.0 / `20260822`

## Purpose

This repository has previously produced apparently excellent Python→Go parity numbers while still containing obvious missing behavior and dead one-line functions created solely to satisfy name-based audits. The audit tooling is therefore a tripwire, not a proof system. Its job is to make unsupported claims visible and difficult to hide.

The semantic source of truth remains the current vendored KFX Input Python implementation. Behavioral equivalence requires differential/golden output evidence in addition to static review.

## What was wrong with the old function metric

The historical function auditor matched names case-insensitively across the whole Go tree. Any same-named Go declaration could satisfy a Python function, even when the Go body was `return nil`, `return false`, or otherwise unrelated. Dunder aliases such as `__repr__ -> String` and broad cross-file matches created further false positives. Large groups of Go functions were explicitly labeled as Python-named stubs “for parity audit purposes.”

The denominator was also incomplete. The old report audited 537 Python definitions. Current upstream contains **961** function/method definitions across the 35 core `kfxlib/*.py` files. The missing denominator included important conversion code such as `resources.py`, the JXR implementation, and `utilities.py`.

A second denominator bug existed in the Python extractor itself: nested functions underneath `if`, `for`, `try`, and similar control-flow nodes were skipped. The extractor now recursively visits all descendants and a regression test pins the current upstream total at 961.

## Current function-audit rules

`audit_parity.py` now uses the Go AST scanner in `scripts/gofuncinfo` and applies conservative identity/substance rules:

- Go identifiers are exact-case. `decodeKFX` and `DecodeKFX` are different functions.
- Automatic matching is unique + exact-case + same-file only. Cross-file matches require an explicit reviewed identity override.
- Generic aliases (`String`, `Equal`, `Get`, `Len`, etc.) never auto-match Python dunders.
- A Go declaration must contain substance. Empty, constant-only, error-only, and admitted-not-implemented bodies do not count as substantive ports.
- One-line delegation only receives credit when an unqualified identifier call resolves unambiguously to substantive code. Selector calls such as `strings.TrimSpace` cannot accidentally resolve to a same-named corpus function.
- Python trivial functions are compared semantically: literal values and identity-argument positions must agree (`True != false`, `0 != 1`, `return arg0 != return arg1`).
- Reviewed exclusions are validated. Architecture/library-replacement exclusions require concrete substantive Go evidence; stale or ambiguous exclusions are rejected rather than applied.
- Reviewed identity mappings must point exact-case to substantive Go functions.

The metric is deliberately named **structural coverage**. A large but wrong Go function still counts structurally, so the number cannot establish semantic parity.

## Complete upstream scope

Every current upstream core Python file is classified in `SCOPE_MANIFEST`. A new upstream `.py` file that is not deliberately classified is a hard audit problem.

Current definition accounting is:

- **961 upstream core definitions total**
- **807 in-scope definitions audited structurally**
- **154 definitions in explicitly reviewed file-scope waivers**

The 154 waived definitions remain visible in the upstream denominator. File-scope waivers currently cover KPF/original-source/standalone-unpack/Calibre logging/ION-text directions that are outside the on-device KFX→EPUB converter contract. They are not claimed as implemented.

Important previously omitted files are no longer blanket-hidden:

- `resources.py` is audited against `internal/kfx/yj_to_epub_resources.go`.
- `jxr_image.py`, `jxr_container.py`, and `jxr_misc.py` are audited against the curated `internal/jxr` implementation rather than waived merely because the architecture differs.
- `utilities.py` remains in scope against a curated component set. Host glue may receive individual reviewed exclusions, but output-affecting path/sort/serialization behavior cannot disappear behind a file-level waiver.

## Current structural numbers

From `python3 scripts/audit_parity.py --metric`:

```text
upstream_total_defs=961
in_scope_defs=807
upstream_out_of_scope_defs=154

implemented=211
implemented_delegation=30
implemented_trivial=0
mapped=0
excluded=167

stub_silent=32
thin=8
missing=256
unresolved_match=103
gap_functions=399

in-scope structural coverage = 241 / 640 = 37.7%
strict in-scope coverage      = 241 / 807 = 29.9%
upstream structural coverage = 241 / 961 = 25.1%
```

The `640` denominator removes only validated per-definition exclusions from the 807 in-scope definitions. Reviewed identity mappings, when present, count as substantive structural mappings rather than disappearing from the denominator.

These numbers are much lower than the historical 96.5% because the current audit refuses to infer equivalence merely from a name and now includes the previously omitted upstream files.

## Branch audit

The branch auditor is also deliberately conservative:

- it searches only the uniquely resolved exact-case same-file Go counterpart body;
- cross-file name collisions are not branch evidence;
- generic identifier/shape heuristics are `weak`, never strong coverage;
- missing/hollow counterparts contribute uncertainty rather than borrowing evidence from another Go function or file.

Current core-conversion branch metric (`audit_missing_branches.py --metric`):

```text
found_branches=661
weak_branches=2186
missing_branches=0
uncertain_branches=1759
total_branches=4606
branch_coverage_pct=14.4
```

This branch metric intentionally covers the ten central conversion files listed by `audit_missing_branches.py`; it is not a full-961-definition behavioral measure.

## Dead audit-only shims

The source still contains dead Python-name-only Go functions, including explicit blocks whose comments say they exist “for parity audit purposes.” The hardened auditor classifies these as stubs/dead rather than implementations. They should be removed when confirmed to have no production/test/interface role; they are no longer necessary to keep the audit green.

`PARITY_REPORT.md` marks matched dead functions and lists the real gaps instead of allowing those names to satisfy the metric.

## Behavioral evidence is separate

No static percentage proves converter equivalence. The required hierarchy is:

1. exact current Python source as semantic reference;
2. focused tests for known behavior and edge cases;
3. branch/static audits as coverage tripwires;
4. differential/golden EPUB comparison on a representative corpus.

At present `scripts/parity_diff.py --metric` may report every historical book as `SKIP` when the private corpus is absent. A zero-difference result with `books_total=0` is **no behavioral evidence** and must never be presented as parity.

## Maintenance

After any Python reference or Go conversion change:

```sh
python3 -m unittest discover -s scripts/tests
python3 scripts/audit_parity.py --metric
python3 scripts/audit_parity.py --report PARITY_REPORT.md
python3 scripts/audit_missing_branches.py --metric
go test ./scripts/gofuncinfo
```

Then run the normal Go/Lua/build verification and, when a real corpus is available, the differential EPUB suite. Any new exclusion or identity mapping should be reviewed as evidence, not treated as a way to improve the percentage.
