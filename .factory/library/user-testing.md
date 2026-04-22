# User Testing Guide: kindle.koplugin Content Pipeline

## Overview

This project is a Go binary that converts KFX → EPUB. There are no web services, no databases, no browser UI. All user testing is performed via **bash commands**:

- `go build ./cmd/kindle-helper/` — Build verification
- `go test ./internal/kfx/ -count=1 -timeout 120s` — Unit test suite
- `bash /tmp/compare_all.sh` — EPUB diff comparison for original 6 books (Martyr, ThreeBelow, Elvis, Familiars, HungerGames, TOG)
- `bash /tmp/compare_new.sh` — EPUB diff comparison for new 4 books (1984, HeatedRivalry, SecretsCrown, SunriseReaping)
- `rg` / `grep` — Code-level verification for specific patterns

## Validation Concurrency

### Testing Surfaces and Resource Costs

| Surface | Tool | Max Concurrent | Notes |
|---------|------|---------------|-------|
| `bash` (go build + test) | Execute | 1 | CPU-intensive, shared build cache |
| `bash` (EPUB comparison) | Execute | 1 | Disk-intensive, shared /tmp dirs |
| `bash` (code grep) | Execute | 3-5 | Lightweight, read-only code search |

### Concurrency Decision

- **go build + go test**: Serial (1 at a time) — shared Go build cache, CPU-bound
- **EPUB comparisons**: Serial (1 at a time) — shared /tmp/go_d and /tmp/ref_d directories, disk-bound
- **Code grep/rg**: Can run 3-5 concurrently — read-only, lightweight

### Recommended Partitioning

Since most assertions require running the full EPUB comparison pipeline (which is serial due to shared temp directories), the most efficient approach is to:

1. Run the full comparison suite once
2. Partition assertions by area for code-level verification (rg/grep)
3. Each group of code-level assertions can run in parallel

## Flow Validator Guidance: bash

### Isolation Rules
- `go build` and `go test` must be serial across all validators
- EPUB comparison scripts use shared `/tmp/go_d` and `/tmp/ref_d` directories — MUST be serial
- Code searches via `rg` can run concurrently (up to 5)
- The Go binary (`./kindle-helper`) is a shared artifact — build once before any conversions

### Assertion Testing Strategy

For this project, assertions are verified by:

1. **Running the comparison scripts** and checking the output for matching/differing file counts
2. **Searching Go source code** with `rg` for specific patterns
3. **Examining generated EPUB content** (unzipped in /tmp) for specific CSS/HTML patterns

### Key Commands

```bash
# Build
go build ./cmd/kindle-helper/

# Run tests
go test ./internal/kfx/ -count=1 -timeout 120s

# Compare original 6 books
bash /tmp/compare_all.sh

# Compare new 4 books
bash /tmp/compare_new.sh

# Search Go code
rg 'pattern' internal/kfx/

# Search generated EPUB content
# After comparison, files are in /tmp/go_d/ and /tmp/ref_d/
```

## Fixture Paths

| What | Path |
|------|------|
| Original KFX files | `REFERENCE/kfx_examples/*.kfx` |
| Decrypted DRMION KFX | `REFERENCE/kfx_new/decrypted/*.kfx-zip` |
| Calibre reference EPUBs (original) | `REFERENCE/kfx_examples/*.epub` and `REFERENCE/martyr_calibre.epub` |
| Calibre reference EPUBs (DRMION) | `REFERENCE/kfx_new/calibre_epubs/*.epub` |
| New device KFX files | `REFERENCE/kindle_device_new/*.kfx` |
| New Calibre reference EPUBs | `REFERENCE/kfx_new/calibre_epubs_new/*.epub` |
| DRM keys cache | `REFERENCE/kindle_device/cache/drm_keys.json` |
