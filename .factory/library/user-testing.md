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
