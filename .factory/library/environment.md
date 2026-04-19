# Environment

Environment variables, external dependencies, and setup notes.

## What belongs here

Required env vars, external dependencies, platform-specific notes.
Service ports/commands are in `.factory/services.yaml`.

---

## Go Version

Go 1.23 (specified in go.mod)

## Dependencies

Single external dependency: `github.com/amazon-ion/ion-go v1.5.0`

## Build Targets

ARM cross-compilation via `arm_build.sh`:
- `GOOS=linux GOARCH=arm GOARM=5` (Kindle Paperwhite)
- `GOOS=linux GOARCH=arm GOARM=7` (Kindle Oasis)
- `GOOS=linux GOARCH=arm64` (newer devices)

CGO is disabled (`CGO_ENABLED=0`).

## Python

Python 3 is available for running reference scripts (`scripts/kfx_reference_snapshot.py`) but is NOT needed for Go tests.

## Test Fixtures

KFX test fixtures are NOT available in the repository. All tests must use synthetic Go data. The following fixture paths are referenced in tests but do NOT exist:
- `REFERENCE/kfx_examples/Martyr_*.kfx`
- `REFERENCE/kfx_examples/The Familiars_*.kfx`
- `REFERENCE/kfx_new/decrypted/*.kfx-zip`
- `REFERENCE/martyr_unpack.zip`

Do NOT attempt to create or depend on these files.
