# D1: Notebook/Scribe Support (yj_to_epub_notebook.py)

## What Was Ported

All module-level constants and standalone functions from `yj_to_epub_notebook.py` (703 lines):

- **Constants** (lines 16-68): `CREATE_SVG_FILES_IN_EPUB`, `PNG_SCALE_FACTOR`, `PNG_DENSITY_GAMMA`, `PNG_EDGE_FEATHERING`, `INCLUDE_PRIOR_LINE_SEGMENT`, `ROUND_LINE_ENDINGS`, `QUANTIZE_THICKNESS`, `ANNOTATION_TEXT_OPACITY`, `SVG_DOCTYPE`, `MIN_TAF`, `MAX_TAF`, `MIN_DAF`, `MAX_DAF`
- **Brush types** (lines 27-36): `ERASER`, `FOUNTAIN_PEN`, `HIGHLIGHTER`, `MARKER`, `ORIGINAL_PEN`, `PEN`, `PENCIL`, `SHADER`, `UNKNOWN`
- **Thickness data** (lines 38-49): `THICKNESS_NAME` (5 entries), `THICKNESS_CHOICES` (8 brush types × 5 values)
- **Stroke colors** (lines 51-61): `STROKE_COLORS` (10 entries as `StrokeColorEntry{Name, Hex}` structs)
- **adjustColorForDensity** (lines 615-622): Converts color to grayscale and applies density
- **decodeStrokeValues** (lines 624-703): Decodes binary stroke data with delta compression

## What Remains Stub

The following methods remain as stubs (return false) because they require full book context (KFX_EPUB receiver):
- `processScribeNotebookPageSection` (Python line 78)
- `processScribeNotebookTemplateSection` (Python line 158)

The Python class methods (`process_notebook_content`, `scribe_notebook_stroke`, `scribe_notebook_annotation`, `scribe_annotation_content`) also require book context and will need a Go struct receiver pattern.

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
