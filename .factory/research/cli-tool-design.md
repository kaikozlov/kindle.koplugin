# CLI Tool Design: `cmd/kindle-helper`

## Summary

The `kindle-helper` binary is a Go CLI tool cross-compiled for ARM (Kindle devices) and called by the Lua frontend via `io.popen`. It outputs JSON to stdout, which Lua decodes. The binary **does not exist yet** — the `cmd/` directory has not been created.

---

## 1. Commands Required

### `scan` — Filesystem scanner for Kindle documents

**Called by:** `HelperClient:scan(root)` in `src/helper_client.lua`
**Invoked as:**
```
kindle-helper scan --root <path>
```

**Behavior:**
- Walks the filesystem tree starting at `--root`
- Filters to supported file extensions: `.kfx`, `.azw`, `.azw3`, `.mobi`, `.prc`, `.pdf`
- Skips `.sdr` directories
- For `.kfx` files, calls `kfx.Classify(path)` to determine `open_mode` ("convert", "blocked") and `block_reason` ("drm", "unsupported_kfx_layout")
- Derives title from filename (strips trailing ID hash, replaces underscores)
- Computes SHA1 of relative path as book ID
- Detects sidecar `.sdr` directories
- Sorts books alphabetically by display name

**JSON output (stdout):**
```json
{
  "version": 1,
  "root": "/mnt/us/documents",
  "books": [
    {
      "id": "sha1:abcdef...",
      "source_path": "/mnt/us/documents/Book Title_ABCDEF1234.kfx",
      "sidecar_path": "/mnt/us/documents/Book Title_ABCDEF1234.sdr",
      "format": "kfx",
      "logical_ext": "epub",
      "title": "Book Title",
      "authors": [],
      "display_name": "Book Title",
      "open_mode": "convert",
      "block_reason": "",
      "source_mtime": 1710000000,
      "source_size": 1234567
    }
  ]
}
```

**Go implementation:** `internal/scan/scan.go` — `scan.Run(root)` already implements this. Returns `jsonout.ScanResult`.

---

### `convert` — KFX to EPUB converter

**Called by:** `HelperClient:convert(input_path, output_path)` in `src/helper_client.lua`
**Also called by:** `scripts/diff_kfx_parity.sh` and `scripts/diff_report.sh` for testing
**Invoked as:**
```
kindle-helper convert --input <path> --output <path>
```

**Behavior:**
- Calls `kfx.Classify(input_path)` first to check DRM/unsupported
- If blocked, returns error JSON
- If convertable, calls `kfx.ConvertFile(input_path, outputPath)` which:
  1. Decodes the KFX container
  2. Organizes fragments (symbols, storylines, sections)
  3. Renders book state to HTML sections
  4. Writes EPUB via `epub.Write()`
- Returns JSON result on stdout

**JSON output (stdout):**
```json
{
  "version": 1,
  "ok": true,
  "output_path": "/cache/path/output.epub"
}
```

On error:
```json
{
  "version": 1,
  "ok": false,
  "code": "drm",
  "message": "DRM-protected KFX is not supported"
}
```

**Go implementation:** `internal/kfx/kfx.go` — `kfx.ConvertFile(inputPath, outputPath)` already implements this. `internal/jsonout.ConvertResult` defines the output struct.

---

## 2. No Other Commands

Only `scan` and `convert` are called from the Lua frontend. The `classify` functionality is embedded within `scan` (for KFX files) and `convert` (as a pre-check). There is no standalone `classify` command needed by any caller.

---

## 3. CLI Interface Contract (Lua Side)

From `src/helper_client.lua`:

```lua
-- All output goes to stdout as JSON, stderr is captured but only stdout is decoded
-- The binary must:
-- 1. Parse subcommand (scan | convert)
-- 2. Parse flags specific to each subcommand
-- 3. Output exactly one JSON object to stdout
-- 4. Exit 0 on success, non-zero on failure
-- 5. Errors go to stderr (for logging), result JSON goes to stdout
```

The Lua client captures both stdout+stderr (`2>&1`) but only JSON-decodes stdout. The `popen` call blocks until the binary exits.

---

## 4. Build Process

From `arm_build.sh`:

```bash
# Cross-compiled for three ARM targets (no CGO):
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=5  go build -ldflags="-s -w" -o kindle-helper-arm-legacy ./cmd/kindle-helper
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7  go build -ldflags="-s -w" -o kindle-helper-armv7       ./cmd/kindle-helper
CGO_ENABLED=0 GOOS=linux GOARCH=arm64        go build -ldflags="-s -w" -o kindle-helper-arm64        ./cmd/kindle-helper
```

- The package path is `./cmd/kindle-helper` (relative to the project root)
- Static linking (CGO_ENABLED=0) — no C dependencies
- Stripped binaries (`-s -w` flags)
- The resulting binary is named `kindle-helper` and placed alongside the `.lua` files in the plugin zip

---

## 5. Go Public API Surface

### `internal/kfx` package

| Function | Signature | Purpose |
|----------|-----------|---------|
| `Classify` | `(path string) (openMode string, blockReason string, err error)` | Determine if a KFX file can be converted. Returns ("convert", "", nil) for open files, ("blocked", "drm"/"unsupported_kfx_layout", nil) for blocked files |
| `ConvertFile` | `(inputPath, outputPath string) error` | Full KFX→EPUB conversion. Internally calls Classify first, then decodeKFX, then epub.Write |

### `internal/scan` package

| Function | Signature | Purpose |
|----------|-----------|---------|
| `Run` | `(root string) (jsonout.ScanResult, error)` | Walk filesystem, classify KFX files, return structured scan result |

### `internal/jsonout` package

| Type | Fields | Purpose |
|------|--------|---------|
| `ScanResult` | `Version, Root, Books []ScanBook` | Top-level scan output |
| `ScanBook` | `ID, SourcePath, SidecarPath, Format, LogicalExt, Title, Authors, DisplayName, OpenMode, BlockReason, SourceMtime, SourceSize` | Per-book scan entry |
| `ConvertResult` | `Version, OK, Code, Message, OutputPath` | Convert command output |

### `internal/epub` package

| Function | Signature | Purpose |
|----------|-----------|---------|
| `Write` | `(outputPath string, book Book) error` | Write EPUB zip with OPF, NCX, nav.xhtml, sections, resources |

---

## 6. CLI Design Specification

The CLI should be implemented as a standard Go `main` package at `cmd/kindle-helper/main.go` using the `flag` package (or a minimal subcommand approach since Go's stdlib doesn't have subcommand support built into `flag`).

### Recommended structure:

```
cmd/kindle-helper/
└── main.go          # Entry point with subcommand dispatch
```

### Pseudocode:

```go
func main() {
    if len(os.Args) < 2 {
        fatal("usage: kindle-helper <scan|convert> [flags]")
    }

    switch os.Args[1] {
    case "scan":
        fs := flag.NewFlagSet("scan", flag.ExitOnError)
        root := fs.String("root", "", "root directory to scan")
        fs.Parse(os.Args[2:])
        // validate root
        result, err := scan.Run(*root)
        // json.Marshal to stdout

    case "convert":
        fs := flag.NewFlagSet("convert", flag.ExitOnError)
        input := fs.String("input", "", "input KFX path")
        output := fs.String("output", "", "output EPUB path")
        fs.Parse(os.Args[2:])
        // validate input/output
        err := kfx.ConvertFile(*input, *output)
        // build ConvertResult, json.Marshal to stdout

    default:
        fatal("unknown command: " + os.Args[1])
    }
}
```

### Key implementation notes:

1. **JSON to stdout only** — the Lua client JSON-decodes stdout. All diagnostic/logging output goes to stderr.
2. **Exit code 0** on success (even if conversion reports an error in the JSON). The Lua client checks the JSON `ok` field, not the exit code.
3. **No external CLI library needed** — the flag parsing is trivial (2 subcommands, each with 1-2 flags). Use `flag.NewFlagSet` for subcommands.
4. **Error handling pattern**: For `convert`, always output a `ConvertResult` JSON. If `kfx.ConvertFile` returns a `*kfx.DRMError`, set `code: "drm"`. If `*kfx.UnsupportedError`, set `code: "unsupported"`. Generic errors get `code: "error"`.
5. **No verbose/debug flags** — the Go library already checks `KFX_DEBUG` env var for debug output to stderr.

---

## 7. Testing Integration

### `scripts/diff_kfx_parity.sh`
```bash
go run ./cmd/kindle-helper convert --input "$input_path" --output "$output_epub"
```
Runs the convert command via `go run`, then unzips and diffs the output EPUB against the Calibre Python reference.

### `scripts/diff_report.sh`
```bash
(cd "$plugin_dir" && go run ./cmd/kindle-helper convert --input "$input" --output "$output")
```
Same convert invocation, runs for multiple test KFX files, compares CSS output.

### `scripts/convert_kfx_python.py`
The Python equivalent using `YJ_Book(args.input).convert_to_epub()`. Used to generate reference EPUBs for comparison.

---

## 8. File Reference Index

| File | Key Insight |
|------|-------------|
| `src/helper_client.lua` | Defines the CLI contract — `scan --root`, `convert --input --output`, JSON stdout |
| `scripts/diff_kfx_parity.sh` | Calls `go run ./cmd/kindle-helper convert --input ... --output ...` |
| `scripts/diff_report.sh` | Same convert invocation for CSS diff analysis |
| `arm_build.sh` | Build path is `./cmd/kindle-helper`, cross-compiled for ARM |
| `internal/kfx/kfx.go` | `ConvertFile()` and `Classify()` — the core API |
| `internal/scan/scan.go` | `Run()` — full scan implementation |
| `internal/jsonout/jsonout.go` | `ScanResult`, `ScanBook`, `ConvertResult` — output types |
| `main.lua` | Plugin entry point, creates `HelperClient` |
