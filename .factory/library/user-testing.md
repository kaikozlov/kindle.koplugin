# User Testing

Testing surface, required tools, and resource cost classification.

## What belongs here

How validators test the mission output, what tools they use, and resource constraints.

---

## Validation Surface

### Surface: Go Unit Tests
- **Tool**: `go test`
- **Command**: `go test ./internal/kfx/... ./internal/epub/... ./internal/jxr/... ./internal/scan/... ./cmd/kindle-helper/... -v`
- **Setup**: None needed — `go mod download` in init.sh
- **What's tested**: All behavioral assertions in the validation contract via synthetic test data

### Surface: CLI Tool
- **Tool**: `tuistory`
- **Command**: Build and run `cmd/kindle-helper --help`
- **Setup**: `go build ./cmd/kindle-helper`
- **What's tested**: CLI entry point exists and responds correctly

## Validation Concurrency

- **Max concurrent validators**: 5
- **Rationale**: No running services. Each `go test` invocation uses ~100MB RAM. Machine has ample headroom.

## Notes for Validators

- Do NOT attempt to use KFX fixture files — they don't exist in the repo
- All tests are synthetic/unit tests that construct Go data structures
- The `go test` command may show some fixture-dependent tests failing — these are pre-existing and should be ignored
- Focus on validating newly added tests pass
