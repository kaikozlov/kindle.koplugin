# kindle.koplugin — Kindle Virtual Library for KOReader

A [KOReader](https://koreader.rocks/) plugin that browses Kindle-native books from a virtual library and converts DRM-free KFX files to EPUB format. The core conversion engine is a Go port of the [Calibre KFX Input plugin](https://www.mobileread.com/forums/showthread.php?t=291290).

## What It Does

- Scans a Kindle device's document library and indexes metadata
- Presents a virtual library UI inside KOReader for browsing Kindle-native books
- Converts Amazon KFX ebook files to standard EPUB format using a Go conversion library
- Caches converted files for fast re-opening

## Architecture

```
KOReader Plugin (Lua)  →  Go CLI (kindle-helper)  →  Go conversion library (internal/)
```

The Lua frontend (`main.lua` + `src/`) integrates with KOReader's widget system and spawns the Go binary as a helper process. All heavy lifting — KFX parsing, YJ decode, EPUB assembly — lives in Go under `internal/`.

## Project Structure

```
cmd/kindle-helper/   CLI entry point — bridges Lua ↔ Go via JSON stdin/stdout
src/                 KOReader Lua frontend (widgets, cache, library index)
internal/
  kfx/               Core KFX→EPUB conversion (parity port of Calibre kfxlib/)
  epub/              EPUB packaging and NCX/NAV generation
  jxr/               JPEG XR decoder
  jsonout/           JSON output helpers for CLI responses
  scan/              Kindle document scanning
_meta.lua           KOReader plugin metadata
main.lua            Plugin entry point
arm_build.sh        Cross-compile script for Kindle ARM targets
scripts/            Development and CI scripts
REFERENCE/          Calibre KFX Input plugin (Python) — not tracked in repo
```

## Building

```sh
# Native build
go build ./cmd/kindle-helper

# Cross-compile for Kindle (ARM)
./arm_build.sh
```

## Testing

```sh
go test ./...
```

Some tests require KFX fixture files that are not included in the repository. Those tests will be skipped automatically if fixtures are absent.

## Python Reference

`REFERENCE/Calibre_KFX_Input/` contains the original Calibre KFX Input plugin written in Python. This is the **source of truth** for all conversion logic. Every function in the Go port maps 1:1 to its Python counterpart.

## Parity Approach

The port maintains strict three-fold parity with the Python reference:

1. **Structural** — Matching file names and directory layout
2. **Function-level** — Matching function names, signatures, and purposes
3. **Logic-level** — Matching control flow, return values, and edge-case behavior

All three layers are validated by tests that compare Go output against expected results derived from the Python implementation.

## Conversion Coverage

The Go port covers the full KFX→EPUB conversion pipeline:

- **Fragment organization** — Ion fragment parsing, symbol table resolution, metadata extraction
- **Section processing** — Book type dispatch (reflowable, comic, magazine), page spread templates, conditional layouts
- **Position/location mapping** — Content position info, position maps, location maps, approximate page lists
- **EPUB packaging** — OPF/NCX/NAV generation, EPUB2/3 version switching, RTL support, mbp: namespace
- **Resource handling** — Tile reassembly, JXR-to-JPEG conversion, PDF page extraction, variant selection
- **Illustrated layout** — Region magnification, conditional page templates, KFXConditionalNav
- **Notebook/scribe** — Stroke SVG paths, PNG density maps, handwriting recognition annotations
- **Image book output** — CBZ with ComicBookInfo metadata, PDF with outline bookmarks
- **Fragment validation** — ion_data_eq duplicate detection, kpf_prepub handling, dictionary/sample exceptions

## Mission History

- **Phase 2 (Function-Level Parity):** 16 work items across 4 streams — 247/247 assertions passed
- **Inconsistency Fix Mission:** 121 parity fixes across 5 milestones — 104/104 assertions passed
