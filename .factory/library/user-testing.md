# User Testing Guide: kindle.koplugin Content Pipeline

## Validation Surface

All validation is performed via bash commands against the Go conversion binary. No web services, no databases, no browser UI.

**Surfaces:**
| Surface | Tool | Description |
|---------|------|-------------|
| Go test suite | `bash` | `go test ./internal/kfx/ -count=1 -timeout 120s` |
| Go vet | `bash` | `go vet ./...` |
| EPUB diff | `bash` | `bash scripts/diff_kfx_parity.sh` |
| Trace comparison | `bash` | `bash scripts/parity_check.sh` |
| Code search | `bash` | `rg 'pattern' internal/kfx/` |
| Branch audit | `bash` | `python3 scripts/audit_branches.py` |

## Validation Concurrency

| Surface | Max Concurrent | Rationale |
|---------|---------------|-----------|
| Go build + test | 1 | Go compilation is fast (~5s) but shared build cache |
| EPUB comparison | 1 | Uses shared /tmp directories |
| Code search (rg) | 5 | Read-only, lightweight |
| Branch audit | 3 | Read-only, CPU-light |

## Isolation Rules

- `go build` and `go test` must be serial across all validators
- EPUB comparison scripts use shared `/tmp/go_d` and `/tmp/ref_d` directories — MUST be serial
- Code searches via `rg` can run concurrently (up to 5)
- The Go binary (`./kindle-helper`) is a shared artifact — build once before any conversions
- NOTE: `diff_kfx_parity.sh` calls `python` but only `python3` is available — validators should use explicit `python3` or set up alias

## Assertion Testing Strategy

Assertions are verified by:
1. **Running Go tests** and checking pass/fail
2. **Running comparison scripts** and checking matching/differing file counts
3. **Searching Go source code** with `rg` for specific patterns (function existence, branch handling)
4. **Running branch audit** to verify all Python branches have Go counterparts

## Fixture Paths

| What | Path |
|------|------|
| Original KFX files (6 books) | `REFERENCE/kfx_examples/*.kfx` |
| Decrypted DRMION KFX | `REFERENCE/kfx_new/decrypted/*.kfx-zip` |
| Calibre reference EPUBs (original) | `REFERENCE/kfx_examples/*.epub` |
| Calibre reference EPUBs (DRMION) | `REFERENCE/kfx_new/calibre_epubs/*.epub` |
| New test books | `REFERENCE/kfx_new/decrypted_new/*.kfx-zip` |
| New Calibre references | `REFERENCE/kfx_new/calibre_epubs_new/*.epub` |
| DRM keys cache | `REFERENCE/kindle_device/cache/drm_keys.json` |
| Parity traces | `REFERENCE/kfx_new/parity_traces/` |

## Key Commands

```bash
# Build (~5 seconds)
go build ./cmd/kindle-helper/

# Run Go tests
go test ./internal/kfx/ -count=1 -timeout 120s

# Static analysis
go vet ./...

# EPUB diff against Calibre reference
bash scripts/diff_kfx_parity.sh --input <kfx> --reference-epub <epub> --summary-only --ignore-modified

# Full trace comparison
bash scripts/parity_check.sh

# Branch audit tool
python3 scripts/audit_branches.py --file <file>.py --function <func> --verbose

# Python reference snapshot
python3 scripts/kfx_reference_snapshot.py fragment-summary --input <kfx>
```
