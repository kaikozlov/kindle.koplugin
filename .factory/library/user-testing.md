# User Testing

Testing surface, required testing skills/tools, resource cost classification per surface.

**What belongs here:** Testing surface findings, validation tools, resource costs.
**What does NOT belong here:** Architecture details (use `architecture.md`).

---

## Validation Surface

This mission has a single testing surface: **Go test suite + CLI diff check**.

### Tools

- `go test` — unit test suite with 12 pre-existing failures
- `go run ./cmd/kindle-helper convert` — CLI converter for diff checking
- `diff` — compare output CSS against Python reference EPUB
- `KFX_DUMP_STYLES=1` — intermediate style checkpoint dumps

### No Browser/API Testing

This is a CLI tool and Go library. There are no web surfaces to test with agent-browser.

## Validation Concurrency

Max concurrent validators: **1** (sequential by design — each step depends on the previous)

## Validation Method

1. Run `go test ./internal/kfx/... -count=1 -timeout 120s` and count FAIL lines
2. Run diff check against reference EPUB's stylesheet.css
3. Both must show no regressions from baseline

## Flow Validator Guidance: Go test suite + CLI diff check

### Isolation Rules
- This is a single-user Go library — no concurrent user isolation needed
- All validation is sequential: build → test → convert → diff
- No shared mutable state between validators (each gets its own temp directory)

### Boundaries
- Do NOT modify any Go source files — only run build/test/convert/diff commands
- Do NOT modify Python reference code in REFERENCE/
- Each converter run must use a fresh temp directory (`mktemp -d`)
- Clean up temp directories after each diff check

### Cosmetic vs Structural Diffs
When analyzing diffs, categorize each as:
- **Cosmetic (ignore):** Class index renumbering (same properties, different .class-N name), color name synonyms (gray vs #808080), -webkit- prefix ordering, whitespace
- **Structural (report as failure):** Different CSS property values, missing/extra properties on same class name, different property sets

### Test KFX Files (ALL must be checked)
| File | Input | Reference |
|------|-------|-----------|
| Martyr | `../REFERENCE/kfx_examples/Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx` | `/tmp/martyr_python_ref.epub` |
| Elvis | `../REFERENCE/KFX_DRM/decrypted/Elvis and the Underdogs_B009NG3090_decrypted.kfx-zip` | `/tmp/Elvis and the Underdogs_B009NG3090_calibre.epub` |
| Hunger Games | `../REFERENCE/KFX_DRM/decrypted/The Hunger Games Trilogy_B004XJRQUQ_decrypted.kfx-zip` | `/tmp/The Hunger Games Trilogy_B004XJRQUQ_calibre.epub` |
| Three Below | `../REFERENCE/KFX_DRM/decrypted/Three Below (Floors #2)_B008PL1YQ0_decrypted.kfx-zip` | `/tmp/Three Below (Floors #2)_B008PL1YQ0_calibre.epub` |
