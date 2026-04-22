# Architecture: KFX→EPUB Conversion Pipeline

## Overview

The pipeline converts Amazon KFX format books to standard EPUB files. The Python implementation (Calibre KFX Input plugin) is the source of truth; the Go port must produce byte-identical output.

## Pipeline Stages

```
KFX Binary → ION Decode → Fragment Organization → YJ Structure → 
Content Rendering → Style Processing → EPUB Packaging → Final EPUB
```

### 1. ION Decode (decode.go, symbol_format.go)
Parses Amazon's ION binary format into Go maps/slices. KFX uses ION with shared symbol tables (YJ symbol catalog).

### 2. Fragment Organization (state.go, fragments.go)
Organizes decoded ION fragments into a coherent book structure. Fragments are keyed by type ($608) and ID.

### 3. YJ Structure (yj_structure.go)
Builds the book's structural hierarchy: sections, headings, reading order. Determines spine ordering and section boundaries.

### 4. Content Rendering (storyline.go, yj_to_epub_content.go)
Renders KFX content into HTML elements. This is the largest and most complex stage, handling all KFX content types ($269 text, $271 image, $270 container, $276 list, $278 table, $272 SVG, etc.).

### 5. Style Processing (yj_to_epub_properties.go)
Converts KFX style properties to CSS. Key operations:
- Unit conversion (lh→em, rem→em, vh/vw→%, pt→px)
- Tag promotion (div→h1-h6, div→figure, div→p)
- Reverse inheritance (promote common child properties to parent)
- Style stripping (remove properties matching inherited)
- CSS class naming and deduplication

### 6. EPUB Packaging (kfx.go, render.go, yj_to_epub_navigation.go, yj_to_epub_metadata.go)
Packages HTML sections into EPUB format with:
- Navigation files (NCX for EPUB2, NAV for EPUB3)
- Metadata (OPF with manifest, spine, metadata, guide)
- Resources (images, fonts)
- Stylesheet

## Key Data Structures

- `PipelineState` (state.go): Main pipeline state, holds all fragment data and book metadata
- `Storyline` (storyline.go): Content rendering state for a single section
- `StyleMap` (css_values.go): CSS property map used throughout style processing
- `BookPart` (yj_structure.go): Represents a section/chapter of the book

## Critical Functions

| Function | File | Purpose |
|----------|------|---------|
| simplifyStylesElementFull | yj_to_epub_properties.go | THE most critical function — converts KFX styles to CSS |
| processContent | storyline.go | Renders KFX content tree to HTML |
| findOrCreateStyleEventElement | style_events.go | Creates styled elements at text offsets |
| combineNestedDivs | storyline.go | Merges qualifying nested divs |
| fixupStylesAndClasses | yj_to_epub_properties.go | Assigns CSS class names and builds stylesheet |
| ConvertFile | kfx.go | Top-level conversion entry point |

## Test Books

| Book | Format | Files | Current Status |
|------|--------|-------|---------------|
| Martyr | CONT | 102 | 102/102 match |
| Three Below | DRMION | 23 | 23/23 match |
| Elvis | DRMION | 32 | 32/32 match |
| Familiars | DRMION | 54 | 54/54 match |
| Hunger Games | DRMION | 112 | 112/112 match |
| TOG | DRMION | 71 | 71/71 match |
| HeatedRivalry | DRMION | 51 | 51/51 match |
| 1984 | DRMION | 47 | 32 match, 15 differ |
| SecretsCrown | DRMION | 5 | 1 match, 4 differ |
| SunriseReaping | DRMION | 48 | 16 match, 32 differ |

## Comparison Infrastructure

- `/tmp/compare_all.sh` — Compares original 6 books
- `/tmp/compare_new.sh` — Compares new 4 books  
- `scripts/diff_kfx_parity.sh` — Single book detailed diff
- `scripts/audit_branches.py` — Static branch analysis tool
- `scripts/compare_traces.py` — Stage-by-stage pipeline comparison
