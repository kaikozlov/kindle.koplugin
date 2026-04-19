# D1: Notebook/Scribe Support (yj_to_epub_notebook.py)

## What Was Ported

All module-level constants and standalone functions from `yj_to_epub_notebook.py` (703 lines):

- **Constants** (lines 16-68): `CREATE_SVG_FILES_IN_EPUB`, `PNG_SCALE_FACTOR`, `PNG_DENSITY_GAMMA`, `PNG_EDGE_FEATHERING`, `INCLUDE_PRIOR_LINE_SEGMENT`, `ROUND_LINE_ENDINGS`, `QUANTIZE_THICKNESS`, `ANNOTATION_TEXT_OPACITY`, `SVG_DOCTYPE`, `MIN_TAF`, `MAX_TAF`, `MIN_DAF`, `MAX_DAF`
- **Brush types** (lines 27-36): `ERASER`, `FOUNTAIN_PEN`, `HIGHLIGHTER`, `MARKER`, `ORIGINAL_PEN`, `PEN`, `PENCIL`, `SHADER`, `UNKNOWN`
- **Thickness data** (lines 38-49): `THICKNESS_NAME` (5 entries), `THICKNESS_CHOICES` (8 brush types × 5 values)
- **Stroke colors** (lines 51-61): `STROKE_COLORS` (10 entries as `StrokeColorEntry{Name, Hex}` structs)
- **adjustColorForDensity** (lines 615-622): Converts color to grayscale and applies density
- **decodeStrokeValues** (lines 624-703): Decodes binary stroke data with delta compression
- **processNotebookContent** (lines 220-268): Dispatches $270 layouts, recurses $146/$176, delegates strokes
- **scribeNotebookStroke** (lines 270-515): Handles stroke groups and individual strokes with brush type classification, variable density PNG, SVG path generation
- **scribeNotebookAnnotation** (lines 517-555): Processes nmdl.hwr annotations
- **scribeAnnotationContent** (lines 557-614): Handles $269 content with word iteration and style events

## What Remains Stub

The following methods remain as stubs (return false) because they require full EPUB book context:
- `processScribeNotebookPageSection` (Python line 78) — needs new_book_part, viewport, SVG resource management
- `processScribeNotebookTemplateSection` (Python line 158) — needs content processing pipeline, SVG extraction

## Architecture: Notebook Processing Context

The Python class methods use `self` (KFX_EPUB) for fragment lookup, context tracking, and book part creation. In Go, we use:

- **`notebookContext`**: Holds `getFragment` and `getNamedFragment` function closures, `contentContext` string, and `debug` flag
- **`contextStack`**: Manages push/pop context strings for error messages
- **`svgElement`**: Represents SVG elements with tag, attributes, children, text, and parent pointer
- **`strokePoint`**: Holds X, Y, T (thickness), D (density) for each decoded stroke point

The `getLocationIDString` function extracts location IDs as strings (for SVG id attributes), falling back to the integer-based `getLocationID` from yj_to_epub_content.go.

## Brush Type Classification

Brush type 7 has dual behavior: `MARKER` when variable thickness, `PEN` when constant. This is handled by `classifyBrushTypeWithThickness`. The simple `classifyBrushType` always returns `MARKER` for type 7 (safe default).

## PNG Density Map

Variable-density strokes generate a PNG image using Go's `image/png` package:
- Points are scaled by `PNG_SCALE_FACTOR` (8)
- Midpoints are interpolated recursively for gaps > max(r1, r2, 2)
- Density is computed with gamma correction (`PNG_DENSITY_GAMMA` = 3.5)
- Edge feathering uses `PNG_EDGE_FEATHERING` = 0.75
- PRNG seeded from `nmdl.random_seed` for deterministic output
- Image is base64-encoded and embedded as data URI

## Validation Contract Discrepancies

The validation contract (VAL-D-008) contains incorrect expected values for `adjustColorForDensity`:

1. **`adjustColorForDensity(0xffffff, 1.0)`**: Contract says `0x000000`, but Python returns `0xffffff`. The Python formula: `lum=255, (255-255)*1.0=0, 255-0=255`.
2. **`adjustColorForDensity(0xffffff, 2.0)`**: Contract says `0x000000`, but Python returns `0xffffff`. Same reason: `(255-255)*2=0`.
3. **Delta decoding expected values** in VAL-D-015 are incorrect. The contract claims `[5,13,20,27,36]` but Python's actual delta algorithm produces `[5,8,10,12,16]` for increments `[5,3,-1,0,2]`.

Our Go implementation follows the Python code exactly (the source of truth per AGENTS.md).

## Key Implementation Notes

- `STROKE_COLORS` uses a Go struct `StrokeColorEntry{Name, Hex}` since Go doesn't have Python tuples
- `decodeStrokeValues` uses `encoding/binary` for little-endian reads instead of Python's `Deserializer`
- The `n=3` byte count case in `decodeStrokeValues` triggers a warning log (matching Python) but still decodes correctly
- `colorStr` from `yj_property_info.go` is reused for SVG stroke/fill color generation
- `processTransform` from `svg.go` is reused for $98 transform values
- `getLocationID` from `yj_to_epub_content.go` is reused, with `getLocationIDString` wrapper for SVG id attributes
- SVG elements use a custom tree structure (`svgElement`) since Go doesn't have lxml.etree
