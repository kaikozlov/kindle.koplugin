---
name: infra-worker
description: Creates infrastructure features like CLI tools, build setup, and refactoring that doesn't involve Python porting.
---

# Infrastructure Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## When to Use This Skill

This worker handles infrastructure features: creating the `cmd/kindle-helper` CLI tool, moving code between files (refactoring), and other non-porting work.

## Required Skills

None.

## Work Procedure

### Step 1: Read Mission Context

1. Read `AGENTS.md` in missionDir
2. Read `.factory/library/architecture.md`
3. Read the feature description carefully

### Step 2: Study Existing Code

For CLI tools:
1. Read `arm_build.sh` to understand build expectations
2. Read `internal/kfx/kfx.go` for public API surface
3. Read `internal/scan/scan.go` for scan API
4. Read `main.lua` and `src/helper_client.lua` to understand how Lua calls the binary

For refactoring:
1. Read the source and destination files
2. Identify all references to moved code
3. Ensure all existing tests reference the correct location

### Step 3: Write Tests First (RED)

1. For CLI: test that the binary compiles and responds to `--help`
2. For refactoring: verify existing tests still reference the correct symbols
3. Run and confirm RED

### Step 4: Implement (GREEN)

1. Implement the infrastructure change
2. Run tests and confirm GREEN
3. Run `go test ./... -v` — no regressions

### Step 5: Commit

1. Review `git diff --cached`
2. Commit with descriptive message

## Example Handoff

```json
{
  "salientSummary": "Created cmd/kindle-helper CLI with scan and convert subcommands. Binary compiles for all ARM targets via arm_build.sh.",
  "whatWasImplemented": "cmd/kindle-helper/main.go with flag-based subcommand dispatch. scan --root calls scan.Run(). convert --input --output calls kfx.ConvertFile(). JSON output to stdout matching jsonout package format.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      { "command": "go build ./cmd/kindle-helper", "exitCode": 0, "observation": "Binary created" },
      { "command": "go test ./cmd/kindle-helper/... -v", "exitCode": 0, "observation": "2 tests passing" },
      { "command": "go test ./... -v", "exitCode": 0, "observation": "No regressions" }
    ],
    "interactiveChecks": []
  },
  "tests": {
    "added": [
      {
        "file": "cmd/kindle-helper/main_test.go",
        "cases": [
          { "name": "TestConvertFlagParsing", "verifies": "--input and --output flags parsed correctly" },
          { "name": "TestScanFlagParsing", "verifies": "--root flag parsed correctly" }
        ]
      }
    ]
  },
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- Build system or arm_build.sh requirements are unclear
- Existing API surface doesn't match what the CLI needs
- Lua frontend expectations are ambiguous
