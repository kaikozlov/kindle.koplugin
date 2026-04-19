# User Testing Guide — KFX→EPUB Parity

## Overview

This project is a pure Go library with no running services. The only testable surface is the `cmd/kindle-helper` CLI binary and `go test` unit tests.

## Services

No services required. Build with `go build ./cmd/kindle-helper`. Test with `go test ./cmd/kindle-helper/... -v -count=1`.

## Testing Surfaces

### CLI Surface (`cmd/kindle-helper`)

- **Tool**: Build binary, run as subprocess, inspect JSON output
- **Commands**: `scan --root <path>`, `convert --input <path> --output <path>`
- **Output**: JSON to stdout, errors to stderr
- **Exit codes**: 0 = success, 1 = operational error, 2 = usage error

### Go Test Surface

- **Tool**: `go test ./... -count=1`
- **Known failures**: Pre-existing fixture-dependent tests in `internal/kfx`, `internal/jxr`, `internal/scan` fail because KFX fixture files don't exist in the repo. These are EXPECTED and must be ignored.
- **Relevant tests**: Only `./cmd/kindle-helper/...` tests matter for milestone "infra".

## Validation Concurrency

| Surface | Max Concurrent Validators | Reason |
|---------|--------------------------|--------|
| CLI     | 5                        | Subprocess execution, lightweight |
| go test | 3                        | Builds can consume memory |

## Flow Validator Guidance: CLI

The CLI surface tests the `cmd/kindle-helper` binary as a black box.

**Isolation rules:**
- Each validator builds its own binary in a temp directory
- Each validator uses its own temp directory for scan roots and output files
- No shared mutable state between validators
- Binary is read-only after build; no interference possible

**Boundaries:**
- Do NOT modify source files
- Do NOT test with real KFX files (they don't exist in the repo)
- Use temp directories for all file operations
- Clean up temp files after tests

**How to test:**
1. Build binary: `go build -o /tmp/test-helper github.com/kaikozlov/kindle-koplugin/cmd/kindle-helper`
2. Run commands and capture stdout/stderr
3. Parse JSON output and verify structure
4. Check exit codes

## Pre-existing Known Failures (IGNORE)

These test failures are expected because fixture files are missing:

- `internal/jxr` — 4 tests fail (missing `REFERENCE/martyr_unpack.zip`)
- `internal/kfx` — 17 tests fail (missing `REFERENCE/kfx_examples/` and `REFERENCE/kfx_new/`)
- `internal/scan` — 1 test fails (missing `REFERENCE/kfx_examples/`)

These are NOT regressions. They exist because KFX fixture files are not tracked in the repository.
