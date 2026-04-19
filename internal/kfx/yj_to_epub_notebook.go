package kfx

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
)

// Notebook / scribe: Calibre KFX_EPUB_Notebook (yj_to_epub_notebook.py).
// Ported from yj_to_epub_notebook.py (703 lines).
//
// This file contains:
//   - Module-level constants (lines 16-68 of Python)
//   - Standalone functions: adjustColorForDensity (lines 615-622), decodeStrokeValues (lines 624-703)
//   - Stub methods: processScribeNotebookPageSection, processScribeNotebookTemplateSection
//
// The class methods (process_notebook_content, scribe_notebook_stroke, etc.) require full
// book context (KFX_EPUB receiver) and will be expanded when scribe fixture integration is needed.

// ---------------------------------------------------------------------------
// Module Constants (yj_to_epub_notebook.py lines 16-68)
// ---------------------------------------------------------------------------

// CREATE_SVG_FILES_IN_EPUB controls whether SVG files are created as separate resources.
const CREATE_SVG_FILES_IN_EPUB = true

// PNG_SCALE_FACTOR is the scale factor for density PNG generation.
const PNG_SCALE_FACTOR = 8

// PNG_DENSITY_GAMMA is the gamma correction factor applied to density values.
const PNG_DENSITY_GAMMA = 3.5

// PNG_EDGE_FEATHERING is the edge feathering threshold for density map generation.
const PNG_EDGE_FEATHERING = 0.75

// INCLUDE_PRIOR_LINE_SEGMENT includes prior line segment in SVG paths.
const INCLUDE_PRIOR_LINE_SEGMENT = true

// ROUND_LINE_ENDINGS enables round stroke line caps and joins.
const ROUND_LINE_ENDINGS = true

// QUANTIZE_THICKNESS quantizes thickness adjust factor to 10% steps.
const QUANTIZE_THICKNESS = true

// ANNOTATION_TEXT_OPACITY is the opacity for annotation text elements.
const ANNOTATION_TEXT_OPACITY = 0.0

// SVG_DOCTYPE is the DOCTYPE declaration for SVG documents.
// Ported from yj_to_epub_notebook.py line 25.
var SVG_DOCTYPE = []byte("<!DOCTYPE svg PUBLIC '-//W3C//DTD SVG 1.1//EN' 'http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd'>")

// MIN_TAF / MAX_TAF are the range for thickness_adjust_factor values.
const (
	MIN_TAF = 0
	MAX_TAF = 1000
)

// MIN_DAF / MAX_DAF are the range for density_adjust_factor values.
const (
	MIN_DAF = 0
	MAX_DAF = 300
)

// ---------------------------------------------------------------------------
// Brush Type Constants (yj_to_epub_notebook.py lines 27-36)
// ---------------------------------------------------------------------------

const (
	ERASER       = "eraser"
	FOUNTAIN_PEN = "fountain pen"
	HIGHLIGHTER  = "highlighter"
	MARKER       = "marker"
	ORIGINAL_PEN = "original pen"
	PEN          = "pen"
	PENCIL       = "pencil"
	SHADER       = "shader"
	UNKNOWN      = "unknown"
)

// ---------------------------------------------------------------------------
// THICKNESS_NAME and THICKNESS_CHOICES (yj_to_epub_notebook.py lines 38-49)
// ---------------------------------------------------------------------------

// THICKNESS_NAME maps thickness indices to human-readable labels.
var THICKNESS_NAME = []string{"fine", "thin", "medium", "thick", "heavy"}

// THICKNESS_CHOICES maps brush type names to their 5 thickness values.
// Ported from Python dict (8 entries: 7 brush types + UNKNOWN with empty slice).
// ERASER is intentionally absent.
var THICKNESS_CHOICES = map[string][]float64{
	FOUNTAIN_PEN: {23.625, 31.5, 47.25, 78.75, 126.0},
	HIGHLIGHTER:  {252.0, 315.0, 441.0, 567.0, 756.0},
	MARKER:       {31.5, 63.0, 94.5, 189.0, 315.0},
	PEN:          {23.625, 39.375, 55.125, 94.5, 126.0},
	ORIGINAL_PEN: {23.625, 31.5, 63.0, 94.5, 126.0},
	PENCIL:       {23.625, 39.375, 63.0, 110.25, 189.0},
	SHADER:       {94.5, 189.0, 315.0, 441.0, 567.0},
	UNKNOWN:      {},
}

// ---------------------------------------------------------------------------
// STROKE_COLORS (yj_to_epub_notebook.py lines 51-61)
// ---------------------------------------------------------------------------

// StrokeColorEntry represents a named color with its hex value.
type StrokeColorEntry struct {
	Name string
	Hex  int
}

// STROKE_COLORS maps color indices to (name, hex) pairs.
// Index 6 is intentionally absent (Python has no entry for 6).
var STROKE_COLORS = map[int]StrokeColorEntry{
	0:  {"black", 0x000000},
	1:  {"gray", 0x3f3f3f},
	2:  {"red", 0xff0000},
	3:  {"orange", 0xff8800},
	4:  {"yellow", 0xffff00},
	5:  {"green", 0x00ff00},
	7:  {"aqua", 0x00ffff},
	8:  {"purple", 0x8800ff},
	9:  {"pink", 0xff00ff},
	10: {"blue", 0x0000ff},
}

// ---------------------------------------------------------------------------
// adjustColorForDensity (yj_to_epub_notebook.py lines 615-622)
// ---------------------------------------------------------------------------

// adjustColorForDensity converts a packed RGB color to grayscale and applies
// a density factor. The density maps luminance: 0.0 = original, 1.0 = black.
//
// Python:
//
//	r = (color >> 16) & 255
//	g = (color >> 8) & 255
//	b = color & 255
//	lum = (r + g + b) // 3
//	lum2 = min(max(round(255 - int((255 - lum) * density)), 0), 255)
//	return (lum2 << 16) + (lum2 << 8) + lum2
func adjustColorForDensity(color int, density float64) int {
	r := (color >> 16) & 255
	g := (color >> 8) & 255
	b := color & 255
	lum := (r + g + b) / 3
	lum2 := int(math.Round(float64(255) - float64(int(float64(255-lum)*density))))
	if lum2 < 0 {
		lum2 = 0
	}
	if lum2 > 255 {
		lum2 = 255
	}
	return (lum2 << 16) + (lum2 << 8) + lum2
}

// ---------------------------------------------------------------------------
// decodeStrokeValues (yj_to_epub_notebook.py lines 624-703)
// ---------------------------------------------------------------------------

// decodeStrokeValues decodes binary-encoded stroke value data using delta compression.
//
// The data format is:
//  1. 2-byte signature: \x01\x01
//  2. uint32 LE: number of values (must match numPoints)
//  3. Instruction nibbles: 2 nibbles per byte (high first), each encoding an increment
//  4. Optional extra bytes consumed by the instructions
//
// Per instruction nibble:
//   - Bits 0-1 (n): number of bytes for increment data
//   - Bit 2: if set, increment = n directly; else read n bytes
//   - Bit 3: if set, negate the increment
//
// Delta decoding: change += increment; value += change; first value = increment
//
// Ported from Python yj_to_epub_notebook.py:624-703.
func decodeStrokeValues(data []byte, numPoints int, name string) ([]int, error) {
	hadError := false
	pos := 0

	// Helper to extract n bytes from the buffer
	extract := func(n int) []byte {
		if pos+n > len(data) {
			return nil
		}
		result := data[pos : pos+n]
		pos += n
		return result
	}

	// Helper to unpack a single byte
	unpackByte := func() (byte, bool) {
		if pos >= len(data) {
			return 0, false
		}
		b := data[pos]
		pos++
		return b, true
	}

	// 1. Verify signature
	sig := extract(2)
	if sig == nil || sig[0] != 0x01 || sig[1] != 0x01 {
		sigHex := "nil"
		if sig != nil {
			sigHex = fmt.Sprintf("%02x%02x", sig[0], sig[1])
		}
		log.Printf("kfx: error: %s signature is incorrect (%s)", name, sigHex)
		hadError = true
	}

	// 2. Verify num_vals
	if pos+4 > len(data) {
		log.Printf("kfx: error: %s not enough data for num_vals", name)
		return nil, fmt.Errorf("stroke decode: %s: not enough data", name)
	}
	numVals := int(binary.LittleEndian.Uint32(data[pos : pos+4]))
	pos += 4

	if numVals != numPoints {
		log.Printf("kfx: error: %s expected %d values, found %d", name, numPoints, numVals)
		hadError = true
	}

	// 3. Extract instruction nibbles
	remaining := len(data) - pos
	if remaining*2 < numVals {
		log.Printf("kfx: error: %s not enough data (%d bytes) to extract %d values", name, remaining, numVals)
		return nil, fmt.Errorf("stroke decode: %s: not enough data for instructions", name)
	}

	instrs := make([]int, 0, numVals+1)
	for len(instrs) < numVals {
		b, ok := unpackByte()
		if !ok {
			break
		}
		instrs = append(instrs, int(b>>4))
		instrs = append(instrs, int(b&0x0f))
	}

	// Remove trailing padding nibble if we have more than needed
	if len(instrs) > numVals {
		pad := instrs[len(instrs)-1]
		instrs = instrs[:len(instrs)-1]
		if pad != 0 {
			log.Printf("kfx: error: %s incorrect padding value %d", name, pad)
			hadError = true
		}
	}

	// 4. Decode increments and apply delta decoding
	vals := make([]int, 0, numVals)
	change := 0
	value := 0

	for i := 0; i < numVals; i++ {
		instr := instrs[i]
		n := instr & 3
		var increment int

		if instr&4 != 0 {
			// Direct: increment = n
			increment = n
		} else {
			// Read n bytes for increment
			if pos+n > len(data) {
				log.Printf("kfx: error: %s pos %d instr %d - out of data", name, i, instr)
				hadError = true
				break
			}

			switch n {
			case 0:
				increment = 0
			case 1:
				increment = int(data[pos])
				pos++
			case 2:
				increment = int(binary.LittleEndian.Uint16(data[pos : pos+2]))
				pos += 2
			default: // n == 3
				log.Printf("kfx: error: %s pos %d instr %d - check number of bytes", name, i, instr)
				hadError = true
				b1 := int(data[pos])
				pos++
				b23 := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
				pos += 2
				increment = b1 + (b23 << 8)
			}
		}

		if instr&8 != 0 {
			if increment == 0 {
				log.Printf("kfx: error: %s pos %d instr %d - negative zero increment", name, i, instr)
				hadError = true
			}
			increment = -increment
		}

		if i == 0 {
			change = 0
			value = increment
		} else {
			change += increment
			value += change
		}

		vals = append(vals, value)
	}

	// 5. Check for extra data
	if pos < len(data) {
		extra := data[pos:]
		log.Printf("kfx: error: %s has extra data: %x", name, extra)
		hadError = true
	}

	if hadError {
		log.Printf("kfx: info: %s raw: %x", name, data)
		log.Printf("kfx: info: %s values: %v", name, vals)
		return vals, fmt.Errorf("stroke decode: %s: errors during decoding", name)
	}

	return vals, nil
}

// ---------------------------------------------------------------------------
// processScribeNotebookPageSection — stub (yj_to_epub_notebook.py line 78)
// ---------------------------------------------------------------------------

// processScribeNotebookPageSection processes scribe page sections with SVG stroke generation.
// Currently returns false (stub) — full implementation requires book context (KFX_EPUB receiver).
// Port of KFX_EPUB_Notebook.process_scribe_notebook_page_section.
func processScribeNotebookPageSection(section map[string]interface{}, pageTemplate map[string]interface{}, sectionName string, seq int) bool {
	_, _, _, _ = section, pageTemplate, sectionName, seq
	return false
}

// ---------------------------------------------------------------------------
// processScribeNotebookTemplateSection — stub (yj_to_epub_notebook.py line 158)
// ---------------------------------------------------------------------------

// processScribeNotebookTemplateSection processes scribe notebook templates.
// Currently returns false (stub) — full implementation requires book context (KFX_EPUB receiver).
// Port of KFX_EPUB_Notebook.process_scribe_notebook_template_section.
func processScribeNotebookTemplateSection(section map[string]interface{}, pageTemplate map[string]interface{}, sectionName string) bool {
	_, _, _ = section, pageTemplate, sectionName
	return false
}
