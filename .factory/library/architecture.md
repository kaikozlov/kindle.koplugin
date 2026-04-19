# Architecture

How the KFX→EPUB conversion system works.

## What belongs here

High-level system description: components, relationships, data flows, invariants. Not implementation details.

---

## System Overview

This project is a Go port of Calibre's KFX Input plugin that converts Amazon KFX ebook files into EPUB format. It runs as a KOReader plugin on Kindle e-readers.

### Layers

| Layer | Language | Purpose |
|-------|----------|---------|
| Frontend | Lua (`main.lua`, `src/*.lua`) | KOReader UI integration, file scanning |
| CLI | Go (`cmd/kindle-helper/`) | Binary entry point called by Lua via `io.popen` |
| Core library | Go (`internal/kfx/`) | KFX decoding → EPUB conversion pipeline |
| EPUB packaging | Go (`internal/epub/`) | ZIP/OPF/NCX generation |
| Image handling | Go (`internal/jxr/`) | JPEG XR → JPEG conversion |
| File scanning | Go (`internal/scan/`) | Directory scanning and KFX classification |
| JSON output | Go (`internal/jsonout/`) | Structured JSON output for Lua consumption |

### Data Flow

```
KFX file → decodeKFX() → buildBookState() → renderBookState() → epub.Write() → EPUB file
```

1. **decodeKFX**: Reads KFX container (ZIP/DRMION/sidecar), extracts ION-encoded fragments
2. **buildBookState**: Organizes fragments by type, resolves symbols, builds metadata
3. **renderBookState**: Processes sections through storyline renderer, generates HTML
4. **epub.Write**: Packages HTML + resources + CSS into EPUB ZIP

### Key Data Structures

- **fragmentCatalog**: Central organized structure mapping fragment types → IDs → data
- **symbolResolver**: Resolves ION symbol table references
- **storylineRenderer**: Converts storyline events to HTML DOM
- **Book (epub)**: EPUB structure with sections, resources, navigation, CSS

### Python Reference Mapping

The Python code uses a mixin class pattern where `KFX_EPUB` inherits from multiple modules:
- `YJ_Structure` → `symbol_format.go`, `state.go`, `fragments.go`
- `YJ_Metadata` → `yj_to_epub_metadata.go`
- `YJ_PositionLocation` → `yj_to_epub_navigation.go` (partial)
- `YJ_ToEPUB` → `kfx.go`, `state.go`, `render.go`
- `YJ_ToEPUBContent` → `yj_to_epub_content.go`, `storyline.go`, `container.go`
- `YJ_ToEPUBProperties` → `yj_to_epub_properties.go`, `yj_property_info.go`
- `YJ_ToEPUBNavigation` → `yj_to_epub_navigation.go`
- `YJ_ToEPUBMisc` → `yj_to_epub_misc.go`
- `YJ_ToEPUBMetadata` → `yj_to_epub_metadata.go`
- `YJ_ToEPUBResources` → `yj_to_epub_resources.go`
- `YJ_ToEPUBIllustratedLayout` → `yj_to_epub_illustrated_layout.go`
- `YJ_ToEPUBNotebook` → `yj_to_epub_notebook.go`
- `EPUB_Output` → `internal/epub/epub.go`
