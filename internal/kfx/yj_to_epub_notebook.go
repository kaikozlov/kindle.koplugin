package kfx

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"math/rand"
	"sort"
	"strings"
)

// Notebook / scribe: Calibre KFX_EPUB_Notebook (yj_to_epub_notebook.py).
// Ported from yj_to_epub_notebook.py (703 lines).
//
// This file contains:
//   - Module-level constants (lines 16-68 of Python)
//   - Standalone functions: adjustColorForDensity (lines 615-622), decodeStrokeValues (lines 624-703)
//   - Notebook section processors: processScribeNotebookPageSection, processScribeNotebookTemplateSection
//   - Notebook content processors: processNotebookContent, scribeNotebookStroke, scribeNotebookAnnotation
//
// processScribeNotebookPageSection and processScribeNotebookTemplateSection use ScribeNotebookContext
// to carry book state (new_book_part, reading_orders, manifest_resource, etc.). These are called
// from processSectionWithType when a section has nmdl.canvas_width or nmdl.template_type.

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
// Notebook SVG element types
// ---------------------------------------------------------------------------

// svgElement represents an SVG element with tag, attributes, children, and text.
type svgElement struct {
	Tag        string
	Attrib     map[string]string
	Children   []*svgElement
	Text       string
	Parent     *svgElement
}

// newSVGElement creates a new SVG element as a child of parent.
// Port of Python's etree.SubElement(parent, tag, attrib=...).
func newSVGElement(parent *svgElement, tag string, attrib map[string]string) *svgElement {
	elem := &svgElement{Tag: tag, Attrib: attrib, Parent: parent}
	if parent != nil {
		parent.Children = append(parent.Children, elem)
	}
	return elem
}

// setAttrib sets an attribute on the element.
func (e *svgElement) setAttrib(key, value string) {
	if e.Attrib == nil {
		e.Attrib = make(map[string]string)
	}
	e.Attrib[key] = value
}

// ---------------------------------------------------------------------------
// notebookContext holds the shared state for notebook processing.
// Port of the self (KFX_EPUB) context used in Python's notebook methods.
// ---------------------------------------------------------------------------

// notebookContext holds the fragment lookup function and content context stack
// needed by the notebook processing methods.
// Port of Python's KFX_EPUB_Notebook mixin class context.
type notebookContext struct {
	// getFragment looks up a fragment by type and ID, returning its value as map[string]interface{}.
	// Port of Python's self.get_fragment(ftype, fid).
	getFragment func(ftype string, fid string) map[string]interface{}

	// getNamedFragment looks up a fragment via $259 name mapping.
	// Port of Python's self.get_named_fragment(content, ftype, name_symbol).
	getNamedFragment func(content map[string]interface{}, ftype string, nameSymbol string) map[string]interface{}

	// contentContext is a human-readable context string for error messages.
	// Port of Python's self.content_context.
	contentContext string

	// debug enables debug logging.
	debug bool
}

// pushContext appends context info.
func (nc *notebookContext) pushContext(ctx string) {
	nc.contentContext += " " + ctx
}

// popContext removes the last pushed context.
func (nc *notebookContext) popContext() {
	// Simple approach: track depth with a stack
	// For now, just trim last segment
	parts := strings.Fields(nc.contentContext)
	if len(parts) > 0 {
		nc.contentContext = strings.Join(parts[:len(parts)-1], " ")
	}
}

// ---------------------------------------------------------------------------
// contextStack tracks push/pop for content context
// ---------------------------------------------------------------------------

// contextStack manages a stack of context strings for push/pop operations.
type contextStack struct {
	base   string
	stack  []string
}

func (cs *contextStack) current() string {
	if len(cs.stack) == 0 {
		return cs.base
	}
	return cs.base + " " + strings.Join(cs.stack, " ")
}

func (cs *contextStack) push(ctx string) {
	cs.stack = append(cs.stack, ctx)
}

func (cs *contextStack) pop() {
	if len(cs.stack) > 0 {
		cs.stack = cs.stack[:len(cs.stack)-1]
	}
}

// ---------------------------------------------------------------------------
// processNotebookContent (yj_to_epub_notebook.py:220-268)
// ---------------------------------------------------------------------------

// processNotebookContent walks IonStruct content and dispatches to stroke/annotation handlers.
// Port of Python KFX_EPUB_Notebook.process_notebook_content (yj_to_epub_notebook.py:220-268).
//
// Parameters:
//   - nc: notebook context with fragment lookup
//   - content: the content to process (IonStruct, IonSymbol, etc.)
//   - parent: SVG parent element to append results to
func processNotebookContent(nc *notebookContext, content interface{}, parent *svgElement) {
	if nc.debug {
		log.Printf("kfx: debug: process notebook content: %v", content)
	}

	dataType := detectIonType(content)

	if dataType == ionTypeSymbol {
		// IonSymbol: resolve to fragment via $608 lookup
		fid, _ := content.(string)
		var fragment map[string]interface{}
		if nc.getFragment != nil {
			fragment = nc.getFragment("structure", fid)
		}
		if fragment != nil {
			processNotebookContent(nc, fragment, parent)
		}
		return
	}

	if dataType != ionTypeStruct {
		log.Printf("kfx: info: content: %v", content)
		log.Printf("kfx: error: %s has unknown content data type", nc.contentContext)
		return
	}

	contentMap, _ := content.(map[string]interface{})

	// Pop $159 (content type) and get location_id
	var contentType interface{}
	if v, ok := contentMap["type"]; ok {
		contentType = v
		delete(contentMap, "type")
	}

	locationID := getLocationIDString(contentMap)
	ctx := &contextStack{base: nc.contentContext}
	ctx.push(fmt.Sprintf("%v %s", contentType, locationID))
	nc.contentContext = ctx.current()

	if contentType == "container" {
		var layout interface{}
		if v, ok := contentMap["layout"]; ok {
			layout = v
			delete(contentMap, "layout")
		}

		if listContent, ok := contentMap["content_list"]; ok {
			// $146 list: iterate children
			delete(contentMap, "content_list")
			list, ok := listContent.([]interface{})
			if ok {
				for _, child := range list {
					processNotebookContent(nc, child, parent)
				}
			}
		} else if _, ok := contentMap["story_name"]; ok {
			// $176 story: look up named fragment via $259
			var story map[string]interface{}
			if nc.getNamedFragment != nil {
				story = nc.getNamedFragment(contentMap, "storyline", "story_name")
			}
			if story != nil {
				storyName := story["story_name"]
				delete(story, "story_name")
				if nc.debug {
					log.Printf("kfx: debug: Processing story %v", storyName)
				}

				ctx.push(fmt.Sprintf("story %v", storyName))
				nc.contentContext = ctx.current()

				if storyContent, ok := story["content_list"]; ok {
					delete(story, "content_list")
					list, ok := storyContent.([]interface{})
					if ok {
						for _, child := range list {
							processNotebookContent(nc, child, parent)
						}
					}
				}

				ctx.pop()
				nc.contentContext = ctx.current()

				// Python: self.check_empty(story, self.content_context)
				checkEmptyNotebook(story, nc.contentContext)
			}
		}

		if layout == nil {
			if _, hasNmdlType := contentMap["nmdl.type"]; hasNmdlType {
				scribeNotebookStroke(nc, contentMap, parent, locationID)
			}
		} else if layout != "vertical" {
			log.Printf("kfx: error: %s has unknown $270 layout: %v", nc.contentContext, layout)
		}
	} else {
		log.Printf("kfx: error: %s has unknown content type: %v", nc.contentContext, contentType)
	}

	// Python: self.check_empty(content, "%s content type %s" % (self.content_context, content_type))
	checkEmptyNotebook(contentMap, fmt.Sprintf("%s content type %v", nc.contentContext, contentType))

	ctx.pop()
	nc.contentContext = ctx.base
}

// ---------------------------------------------------------------------------
// getLocationIDString extracts location ID from content map as a string.
// For notebook processing, the location ID is used as SVG element id attributes.
// Python: self.get_location_id(content) which returns location_id used as string.
// ---------------------------------------------------------------------------

func getLocationIDString(content map[string]interface{}) string {
	// In Python, get_location_id reads $183 and resolves the location.
	// For notebook processing, it's used primarily as an SVG element id attribute.
	if loc, ok := content["position"]; ok {
		switch v := loc.(type) {
		case string:
			return v
		case map[string]interface{}:
			if id, ok := v["id"]; ok {
				return fmt.Sprintf("%v", id)
			}
		}
	}
	// Fall back to int-based location ID
	if id := getLocationID(content); id != 0 {
		return fmt.Sprintf("%d", id)
	}
	return ""
}

// ---------------------------------------------------------------------------
// scribeNotebookStroke (yj_to_epub_notebook.py:270-515)
// ---------------------------------------------------------------------------

// scribeNotebookStroke processes stroke content, generating SVG elements.
// Port of Python KFX_EPUB_Notebook.scribe_notebook_stroke (yj_to_epub_notebook.py:270-515).
func scribeNotebookStroke(nc *notebookContext, content map[string]interface{}, parent *svgElement, locationID string) {
	nmdlType := content["nmdl.type"]
	delete(content, "nmdl.type")

	if nmdlType == "nmdl.stroke_group" {
		scribeNotebookStrokeGroup(nc, content, parent, locationID)
	} else if nmdlType == "nmdl.stroke" {
		scribeNotebookStrokeIndividual(nc, content, parent, locationID)
	} else {
		log.Printf("kfx: error: %s has unknown nmdl.type: %v", nc.contentContext, nmdlType)
	}
}

// scribeNotebookStrokeGroup handles nmdl.stroke_group content.
// Port of Python nmdl.stroke_group branch (yj_to_epub_notebook.py:274-292).
func scribeNotebookStrokeGroup(nc *notebookContext, content map[string]interface{}, parent *svgElement, locationID string) {
	nmdlChunked := content["nmdl.chunked"]
	delete(content, "nmdl.chunked")
	nmdlChunkThreshold := content["nmdl.chunk_threshold"]
	delete(content, "nmdl.chunk_threshold")

	if nmdlChunked != true {
		log.Printf("kfx: error: %s has unexpected nmdl.chunked: %v", nc.contentContext, nmdlChunked)
	}

	// Python: nmdl_chunk_threshold = content.pop("nmdl.chunk_threshold", None)
	// Python: if nmdl_chunk_threshold != 50: → logs error for both None and wrong values.
	// Match Python: nil (absent) also triggers the error since nil != 50.
	nmdlChunkThresholdInt := -1 // sentinel for nil
	if nmdlChunkThreshold != nil {
		switch v := nmdlChunkThreshold.(type) {
		case int:
			nmdlChunkThresholdInt = v
		case int64:
			nmdlChunkThresholdInt = int(v)
		case float64:
			nmdlChunkThresholdInt = int(v)
		default:
			nmdlChunkThresholdInt = -1
		}
	}
	if nmdlChunkThresholdInt != 50 {
		log.Printf("kfx: error: %s has unexpected nmdl.chunk_threshold: %v", nc.contentContext, nmdlChunkThreshold)
	}

	groupElem := newSVGElement(parent, "g", nil)

	if locationID != "" {
		groupElem.setAttrib("id", locationID)
	}

	annotations, ok := content["annotations"].([]interface{})
	if ok {
		delete(content, "annotations")
		for _, annotation := range annotations {
			annotationMap, ok := annotation.(map[string]interface{})
			if ok {
				scribeNotebookAnnotation(nc, annotationMap, groupElem)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// scribeNotebookStrokeIndividual handles nmdl.stroke content.
// Port of Python nmdl.stroke branch (yj_to_epub_notebook.py:293-515).
// ---------------------------------------------------------------------------

// strokePoint represents a single stroke point with position, thickness, and density.
type strokePoint struct {
	X int
	Y int
	T int // thickness at this point (quantized)
	D float64 // density adjust factor (0-1)
}

func scribeNotebookStrokeIndividual(nc *notebookContext, content map[string]interface{}, parent *svgElement, locationID string) {
	// Extract stroke properties
	nmdlBrushType := content["nmdl.brush_type"]
	delete(content, "nmdl.brush_type")
	nmdlColor := content["nmdl.color"]
	delete(content, "nmdl.color")
	nmdlRandomSeed := content["nmdl.random_seed"]
	delete(content, "nmdl.random_seed")
	nmdlStrokeBounds := content["nmdl.stroke_bounds"]
	delete(content, "nmdl.stroke_bounds")

	var nmdlThickness float64
	if v, ok := content["nmdl.thickness"]; ok {
		delete(content, "nmdl.thickness")
		switch val := v.(type) {
		case float64:
			nmdlThickness = val
		case int:
			nmdlThickness = float64(val)
		case int64:
			nmdlThickness = float64(val)
		}
	}

	// Extract stroke points data
	var nmdlStrokePoints map[string]interface{}
	if v, ok := content["nmdl.stroke_points"]; ok {
		delete(content, "nmdl.stroke_points")
		nmdlStrokePoints, _ = v.(map[string]interface{})
	}
	if nmdlStrokePoints == nil {
		nmdlStrokePoints = make(map[string]interface{})
	}

	var nmdlNumPoints int
	if v, ok := nmdlStrokePoints["nmdl.num_points"]; ok {
		delete(nmdlStrokePoints, "nmdl.num_points")
		switch val := v.(type) {
		case int:
			nmdlNumPoints = val
		case int64:
			nmdlNumPoints = int(val)
		case float64:
			nmdlNumPoints = int(val)
		}
	}

	// Delete origin_stroke_id (unused in Python)
	delete(content, "nmdl.origin_stroke_id")

	// Decode stroke values
	nmdlStrokeValues := make(map[string][]int)
	strokeNames := []string{
		"nmdl.position_x", "nmdl.position_y", "nmdl.density_adjust_factor",
		"nmdl.thickness_adjust_factor", "nmdl.tilt_x", "nmdl.tilt_y", "nmdl.pressure",
	}
	for _, name := range strokeNames {
		if v, ok := nmdlStrokePoints[name]; ok {
			delete(nmdlStrokePoints, name)
			data, ok := v.([]byte)
			if ok {
				vals, _ := decodeStrokeValues(data, nmdlNumPoints, name)
				nmdlStrokeValues[name] = vals
			}
		}
	}

	// Python: self.check_empty(nmdl_stroke_points, "%s nmdl_stroke_points" % self.content_context)
	checkEmptyNotebook(nmdlStrokePoints, nc.contentContext+" nmdl_stroke_points")

	// Parse stroke bounds
	var bounds [4]int
	if nmdlStrokeBounds != nil {
		switch v := nmdlStrokeBounds.(type) {
		case []interface{}:
			for i := 0; i < 4 && i < len(v); i++ {
				switch val := v[i].(type) {
				case int:
					bounds[i] = val
				case int64:
					bounds[i] = int(val)
				case float64:
					bounds[i] = int(val)
				}
			}
		}
	}

	boundWidth := bounds[2] - bounds[0]
	boundHeight := bounds[3] - bounds[1]

	// Determine stroke color
	var strokeColorName string
	var strokeColor int
	if nmdlColor != nil {
		colorIdx, _ := nmdlColor.(int)
		if entry, ok := STROKE_COLORS[colorIdx]; ok {
			strokeColorName = entry.Name
			strokeColor = entry.Hex
		} else {
			log.Printf("kfx: error: Unexpected color %d", colorIdx)
			strokeColorName = "unknown"
			strokeColor = 0
		}
	}

	// Check for variable density and thickness
	// Must be computed before brush type classification since
	// brush type 7 depends on variableThickness (Python: nmdl.brush_type == 7 → MARKER if variable_thickness else PEN).
	variableDensity := false
	if dafVals, ok := nmdlStrokeValues["nmdl.density_adjust_factor"]; ok {
		for _, daf := range dafVals {
			if daf != 100 {
				variableDensity = true
				break
			}
		}
	}

	variableThickness := false
	if tafVals, ok := nmdlStrokeValues["nmdl.thickness_adjust_factor"]; ok {
		for _, taf := range tafVals {
			if taf != 100 {
				variableThickness = true
				break
			}
		}
	}

	// Determine brush type — must come after variableThickness computation.
	// Python (yj_to_epub_notebook.py:330-350): brush_type 7 → MARKER if variable_thickness else PEN.
	opacity := 1.0
	additiveOpacity := false
	var brushName string

	if nmdlBrushType != nil {
		brushTypeInt, _ := nmdlBrushType.(int)
		brushName = classifyBrushTypeWithThickness(brushTypeInt, variableThickness)
		switch brushTypeInt {
		case 1: // HIGHLIGHTER
			opacity = 0.2
		case 9: // SHADER
			opacity = 0.2
			additiveOpacity = true
		}
	}

	// Determine thickness name
	thicknessName := fmt.Sprintf("%1.3f", nmdlThickness)
	choices := THICKNESS_CHOICES[brushName]
	bestThicknessDiff := 0.5
	for thicknessIdx, thicknessChoice := range choices {
		thicknessDiff := math.Abs(thicknessChoice-nmdlThickness) / thicknessChoice
		if thicknessDiff < bestThicknessDiff {
			thicknessName = THICKNESS_NAME[thicknessIdx]
			bestThicknessDiff = thicknessDiff
		}
	}

	thickness := int(math.Round(nmdlThickness))

	// Build points list
	posXVals := nmdlStrokeValues["nmdl.position_x"]
	posYVals := nmdlStrokeValues["nmdl.position_y"]
	tafVals := nmdlStrokeValues["nmdl.thickness_adjust_factor"]
	dafVals := nmdlStrokeValues["nmdl.density_adjust_factor"]

	points := make([]strokePoint, 0, nmdlNumPoints)
	lastX, lastY := -1, -1 // sentinel for "no previous point"

	for i := 0; i < nmdlNumPoints; i++ {
		x := bounds[0]
		y := bounds[1]
		if i < len(posXVals) {
			x += posXVals[i]
		}
		if i < len(posYVals) {
			y += posYVals[i]
		}

		taf := 100
		if variableThickness && i < len(tafVals) {
			taf = tafVals[i]
		}
		daf := 100
		if variableDensity && i < len(dafVals) {
			daf = dafVals[i]
		}

		// Range checks
		if x < bounds[0] || x > bounds[2] || y < bounds[1] || y > bounds[3] {
			log.Printf("kfx: error: point %d position out of range: (%d, %d) with bounds %v", i, x, y, bounds)
		}
		if taf < MIN_TAF || taf > MAX_TAF {
			log.Printf("kfx: error: point %d thickness_adjust_factor out of range: %d", i, taf)
		}
		if daf < MIN_DAF || daf > MAX_DAF {
			log.Printf("kfx: error: point %d density_adjust_factor out of range: %d", i, daf)
		}

		if QUANTIZE_THICKNESS {
			taf = (taf / 10) * 10
		}

		t := int(math.Round(nmdlThickness * float64(taf) / 100.0))
		d := float64(daf) / 100.0

		if x != lastX || y != lastY {
			points = append(points, strokePoint{X: x, Y: y, T: t, D: d})
		}

		lastX, lastY = x, y
	}

	opacityStr := fmt.Sprintf("%1.2f", opacity)

	// Handle opacity group for non-additive translucent strokes
	actualParent := parent
	if opacity < 1.0 && !additiveOpacity {
		// Walk up to find the SVG root element (tag == "svg")
		svgRoot := parent
		for svgRoot.Parent != nil {
			svgRoot = svgRoot.Parent
		}

		// Look for existing opacity group
		var opacityGroup *svgElement
		for _, child := range svgRoot.Children {
			if child.Tag == "g" && child.Attrib != nil && child.Attrib["opacity"] == opacityStr {
				opacityGroup = child
				break
			}
		}
		if opacityGroup == nil {
			opacityGroup = newSVGElement(svgRoot, "g", map[string]string{"opacity": opacityStr})
		}
		actualParent = opacityGroup
	}

	groupElem := newSVGElement(actualParent, "g", nil)

	if opacity < 1.0 && additiveOpacity {
		groupElem.setAttrib("opacity", opacityStr)
	}

	if locationID != "" {
		groupElem.setAttrib("id", locationID)
	}

	// Add description element
	descElem := newSVGElement(groupElem, "desc", nil)
	descElem.Text = fmt.Sprintf("%s %s %s", thicknessName, strokeColorName, brushName)

	// Set stroke/fill attributes based on density type
	if variableDensity {
		groupElem.setAttrib("stroke", "none")
		groupElem.setAttrib("fill", colorStr(strokeColor, 1.0))
	} else {
		groupElem.setAttrib("fill", "none")
		groupElem.setAttrib("stroke", colorStr(strokeColor, 1.0))
		groupElem.setAttrib("stroke-width", fmt.Sprintf("%d", thickness))

		if ROUND_LINE_ENDINGS {
			groupElem.setAttrib("stroke-linejoin", "round")
			groupElem.setAttrib("stroke-linecap", "round")
		}
	}

	// Handle transform
	if v, ok := content["transform"]; ok {
		delete(content, "transform")
		if vals, ok := v.([]interface{}); ok {
			transform := processTransform(vals, true)
			groupElem.setAttrib("transform", transform)
		}
	}

	// Generate SVG content
	if variableDensity {
		generateDensityPNG(groupElem, points, bounds, boundWidth, boundHeight, nmdlRandomSeed, strokeColor)
	} else {
		generateSVGPaths(groupElem, points, thickness)
	}
}

// ---------------------------------------------------------------------------
// classifyBrushType maps brush type int to name.
// Port of Python brush type switch in scribe_notebook_stroke (yj_to_epub_notebook.py:330-350).
// ---------------------------------------------------------------------------

func classifyBrushType(brushType int) string {
	switch brushType {
	case 0:
		return ORIGINAL_PEN
	case 1:
		return HIGHLIGHTER
	case 5:
		return PENCIL
	case 6:
		return FOUNTAIN_PEN
	case 7:
		// Python checks variable_thickness; if not variable, returns PEN.
		// We can't determine that here, so we return MARKER as the default.
		// The caller should override to PEN if needed.
		return MARKER
	case 9:
		return SHADER
	default:
		log.Printf("kfx: error: Unexpected brush type %d", brushType)
		return UNKNOWN + fmt.Sprintf("%d", brushType)
	}
}

// ---------------------------------------------------------------------------
// classifyBrushTypeWithThickness maps brush type int to name with thickness info.
// Port of Python brush type 7 logic: MARKER if variable_thickness else PEN.
// ---------------------------------------------------------------------------

func classifyBrushTypeWithThickness(brushType int, variableThickness bool) string {
	if brushType == 7 {
		if variableThickness {
			return MARKER
		}
		return PEN
	}
	return classifyBrushType(brushType)
}

// ---------------------------------------------------------------------------
// generateSVGPaths produces SVG <path> elements for normal (non-density) strokes.
// Port of Python's "else" branch in scribe_notebook_stroke (yj_to_epub_notebook.py:482-515).
// ---------------------------------------------------------------------------

func generateSVGPaths(groupElem *svgElement, points []strokePoint, nmdlThickness int) {
	prevT := -1
	prevD := -1.0
	type pathGroup struct {
		points []struct{ X, Y int }
		t      int
		d      float64
	}
	var paths []pathGroup
	var currentPath *pathGroup

	for i, pt := range points {
		if i == 0 || pt.T != prevT || pt.D != prevD {
			pg := pathGroup{t: pt.T, d: pt.D}
			paths = append(paths, pg)
			currentPath = &paths[len(paths)-1]

			// Include prior line segments
			priors := []int{2, 1}
			if !INCLUDE_PRIOR_LINE_SEGMENT {
				priors = []int{1}
			}
			for _, j := range priors {
				if i >= j {
					currentPath.points = append(currentPath.points, struct{ X, Y int }{points[i-j].X, points[i-j].Y})
				}
			}
		}

		currentPath.points = append(currentPath.points, struct{ X, Y int }{pt.X, pt.Y})
		prevT = pt.T
		prevD = pt.D
	}

	for _, pg := range paths {
		if len(pg.points) > 1 {
			pathElem := newSVGElement(groupElem, "path", nil)

			if pg.t != nmdlThickness {
				pathElem.setAttrib("stroke-width", fmt.Sprintf("%d", int(math.Round(float64(pg.t)))))
			}

			var z []string
			for _, pt := range pg.points {
				cmd := "L"
				if len(z) == 0 {
					cmd = "M"
				}
				z = append(z, fmt.Sprintf("%s %d %d", cmd, pt.X, pt.Y))
			}
			pathElem.setAttrib("d", strings.Join(z, " "))
		}
	}
}

// ---------------------------------------------------------------------------
// generateDensityPNG produces a density PNG as base64 for variable-density strokes.
// Port of Python's variable_density branch (yj_to_epub_notebook.py:402-480).
// ---------------------------------------------------------------------------

func generateDensityPNG(groupElem *svgElement, points []strokePoint, bounds [4]int, boundWidth, boundHeight int, nmdlRandomSeed interface{}, strokeColor int) {
	// Build interpolated points with midpoints for gaps
	type densityPoint struct {
		X, Y int
		R    float64 // radius
		D    float64 // density
	}

	var addPointsIfNeeded func(pts *[]densityPoint, x1, y1 int, r1, d1 float64, x2, y2 int, r2, d2 float64)
	addPointsIfNeeded = func(pts *[]densityPoint, x1, y1 int, r1, d1 float64, x2, y2 int, r2, d2 float64) {
		distance := math.Sqrt(float64((x2-x1)*(x2-x1) + (y2-y1)*(y2-y1)))
		if distance > math.Max(math.Max(r1, r2), 2) {
			x3 := (x1 + x2) / 2
			y3 := (y1 + y2) / 2
			r3 := (r1 + r2) / 2
			d3 := (d1 + d2) / 2
			addPointsIfNeeded(pts, x1, y1, r1, d1, x3, y3, r3, d3)
			addPointsIfNeeded(pts, x3, y3, r3, d3, x2, y2, r2, d2)
			*pts = append(*pts, densityPoint{X: x3, Y: y3, R: r3, D: d3})
		}
	}

	pts := make([]densityPoint, 0)
	var lastX, lastY int
	var lastR, lastD float64
	hasLast := false

	for _, pt := range points {
		x0 := (pt.X - bounds[0]) / PNG_SCALE_FACTOR
		y0 := (pt.Y - bounds[1]) / PNG_SCALE_FACTOR
		r0 := float64(pt.T) / float64(PNG_SCALE_FACTOR*2)

		if hasLast {
			addPointsIfNeeded(&pts, lastX, lastY, lastR, float64(lastD), x0, y0, r0, pt.D)
		}

		pts = append(pts, densityPoint{X: x0, Y: y0, R: r0, D: pt.D})
		lastX, lastY = x0, y0
		lastR = r0
		lastD = pt.D
		hasLast = true
	}

	// Create PRNG
	prng := rand.New(rand.NewSource(0))
	if nmdlRandomSeed != nil {
		switch v := nmdlRandomSeed.(type) {
		case int:
			prng.Seed(int64(v))
		case int64:
			prng.Seed(v)
		case float64:
			prng.Seed(int64(v))
		}
	}

	pngWidth := boundWidth / PNG_SCALE_FACTOR
	pngHeight := boundHeight / PNG_SCALE_FACTOR
	if pngWidth <= 0 || pngHeight <= 0 {
		return
	}

	densityMap := make([]float64, pngWidth*pngHeight)

	for _, pt := range pts {
		adjustedD := 1.0 - math.Pow(1.0-math.Min(math.Max(pt.D, 0.0), 1.0), PNG_DENSITY_GAMMA)

		intRadius := int(math.Ceil(pt.R * 1.5))
		for xx := pt.X - intRadius; xx <= pt.X+intRadius; xx++ {
			for yy := pt.Y - intRadius; yy <= pt.Y+intRadius; yy++ {
				if xx >= 0 && yy >= 0 && xx < pngWidth && yy < pngHeight {
					idx := xx + (yy * pngWidth)
					relDistance := math.Sqrt(float64((xx-pt.X)*(xx-pt.X)+(yy-pt.Y)*(yy-pt.Y))) / pt.R
					if relDistance <= PNG_EDGE_FEATHERING {
						if densityMap[idx] < adjustedD {
							densityMap[idx] = adjustedD
						}
					} else if relDistance <= prng.Float64()*(2.0-PNG_EDGE_FEATHERING) {
						reducedD := adjustedD * ((2.0 - PNG_EDGE_FEATHERING) - relDistance)
						if densityMap[idx] < reducedD {
							densityMap[idx] = reducedD
						}
					}
				}
			}
		}
	}

	// Generate PNG data
	pngData := make([]byte, len(densityMap))
	for i, adjustedD := range densityMap {
		if prng.Float64() < adjustedD {
			pngData[i] = 0
		} else {
			pngData[i] = 1
		}
	}

	// Create image
	if strokeColor == 0 {
		// Mode "1" (binary) - black and white
		img := image.NewRGBA(image.Rect(0, 0, pngWidth, pngHeight))
		for y := 0; y < pngHeight; y++ {
			for x := 0; x < pngWidth; x++ {
				idx := x + y*pngWidth
				if pngData[idx] == 0 {
					img.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
				} else {
					img.SetRGBA(x, y, color.RGBA{0, 0, 0, 0}) // transparent
				}
			}
		}
		writePNGImage(groupElem, img, bounds, boundWidth, boundHeight)
	} else {
		// Mode "P" (palette) - color
		r := byte((strokeColor >> 16) & 255)
		g := byte((strokeColor >> 8) & 255)
		b := byte(strokeColor & 255)

		img := image.NewRGBA(image.Rect(0, 0, pngWidth, pngHeight))
		for y := 0; y < pngHeight; y++ {
			for x := 0; x < pngWidth; x++ {
				idx := x + y*pngWidth
				if pngData[idx] == 0 {
					img.SetRGBA(x, y, color.RGBA{r, g, b, 255})
				} else {
					img.SetRGBA(x, y, color.RGBA{0, 0, 0, 0}) // transparent
				}
			}
		}
		writePNGImage(groupElem, img, bounds, boundWidth, boundHeight)
	}
}

// writePNGImage encodes an image as PNG and adds it as a base64 data URI.
func writePNGImage(groupElem *svgElement, img image.Image, bounds [4]int, boundWidth, boundHeight int) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Printf("kfx: error: failed to encode density PNG: %v", err)
		return
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	newSVGElement(groupElem, "image", map[string]string{
		"x":                   fmt.Sprintf("%d", bounds[0]),
		"y":                   fmt.Sprintf("%d", bounds[1]),
		"width":               fmt.Sprintf("%d", boundWidth),
		"height":              fmt.Sprintf("%d", boundHeight),
		"xlink:href":          fmt.Sprintf("data:image/png;base64,%s", encoded),
	})
}

// ---------------------------------------------------------------------------
// scribeNotebookAnnotation (yj_to_epub_notebook.py:517-555)
// ---------------------------------------------------------------------------

// scribeNotebookAnnotation processes annotation content within stroke groups.
// Port of Python KFX_EPUB_Notebook.scribe_notebook_annotation (yj_to_epub_notebook.py:517-555).
func scribeNotebookAnnotation(nc *notebookContext, annotation map[string]interface{}, elem *svgElement) {
	annotationType := annotation["annotation_type"]
	delete(annotation, "annotation_type")

	if annotationType == "nmdl.hwr" {
		var story map[string]interface{}
		if nc.getNamedFragment != nil {
			story = nc.getNamedFragment(annotation, "storyline", "alt_content")
		}

		if story != nil {
			storyName := story["story_name"]
			delete(story, "story_name")
			nc.pushContext(fmt.Sprintf("story %v", storyName))

			_ = getLocationIDString(story)

			if content, ok := story["content_list"]; ok {
				delete(story, "content_list")
				list, ok := content.([]interface{})
				if ok {
					for _, child := range list {
						childMap, ok := child.(map[string]interface{})
						if ok {
							scribeAnnotationContent(nc, childMap, elem)
						}
					}
				}
			}

			// Python: self.check_empty(story, self.content_context)
			checkEmptyNotebook(story, nc.contentContext)

			nc.popContext()
		}

		// Python: self.check_empty(annotation, "%s annotation" % self.content_context)
		checkEmptyNotebook(annotation, nc.contentContext+" annotation")
	} else {
		log.Printf("kfx: error: %s has unexpected annotation_type: %v", nc.contentContext, annotationType)
	}
}

// ---------------------------------------------------------------------------
// scribeAnnotationContent (yj_to_epub_notebook.py:557-614)
// ---------------------------------------------------------------------------

// scribeAnnotationContent processes handwriting recognition content.
// Port of Python KFX_EPUB_Notebook.scribe_annotation_content (yj_to_epub_notebook.py:557-614).
func scribeAnnotationContent(nc *notebookContext, content interface{}, elem *svgElement) {
	dataType := detectIonType(content)

	if dataType == ionTypeSymbol {
		// IonSymbol: resolve to fragment via $608 lookup
		fid, _ := content.(string)
		fragment := nc.getFragment("structure", fid)
		if fragment != nil {
			// Python calls self.process_content() here, which is a different pipeline.
			// For notebook context, this is typically a no-op or placeholder.
		}
		return
	}

	if dataType != ionTypeStruct {
		log.Printf("kfx: error: %s has unknown content data type in annotation", nc.contentContext)
		return
	}

	contentMap, _ := content.(map[string]interface{})

	locationID := getLocationIDString(contentMap)
	nc.pushContext(locationID)

	contentType := contentMap["type"]
	delete(contentMap, "type")

	if contentType == "text" {
		wordIterType := contentMap["word_iteration_type"]
		delete(contentMap, "word_iteration_type")
		if wordIterType != nil && wordIterType != "model" {
			log.Printf("kfx: warning: %s has text word_iteration_type=%v", nc.contentContext, wordIterType)
		}

		var top, left float64
		if v, ok := contentMap["top"]; ok {
			delete(contentMap, "top")
			top = toFloat64(v)
		}
		if v, ok := contentMap["left"]; ok {
			delete(contentMap, "left")
			left = toFloat64(v)
		}
		delete(contentMap, "height")
		delete(contentMap, "width")

		text := ""
		if v, ok := contentMap["content"]; ok {
			delete(contentMap, "content")
			text, _ = v.(string)
		}

		// Add desc element
		descElem := newSVGElement(elem, "desc", nil)
		descElem.Text = text

		// Add text element
		textElem := newSVGElement(elem, "text", map[string]string{
			"x":        fmt.Sprintf("%d", int(left)),
			"y":        fmt.Sprintf("%d", int(top)),
			"stroke":   "none",
			"fill":     "red",
			"opacity":  fmt.Sprintf("%0.2f", ANNOTATION_TEXT_OPACITY),
		})

		// Process style events ($142)
		if styleEvents, ok := contentMap["style_events"]; ok {
			delete(contentMap, "style_events")
			events, ok := styleEvents.([]interface{})
			if ok {
				for _, event := range events {
					eventMap, ok := event.(map[string]interface{})
					if !ok {
						continue
					}

					model := eventMap["model"]
					delete(eventMap, "model")
					if model != nil && model != "word" {
						log.Printf("kfx: warning: %s has text model=%v", nc.contentContext, model)
					}

					var offset, length int
					var wordTop, wordLeft, wordHeight, wordWidth float64

					if v, ok := eventMap["offset"]; ok {
						delete(eventMap, "offset")
						offset = toInt(v)
					}
					if v, ok := eventMap["length"]; ok {
						delete(eventMap, "length")
						length = toInt(v)
					}
					if v, ok := eventMap["top"]; ok {
						delete(eventMap, "top")
						wordTop = toFloat64(v)
					}
					if v, ok := eventMap["left"]; ok {
						delete(eventMap, "left")
						wordLeft = toFloat64(v)
					}
					if v, ok := eventMap["height"]; ok {
						delete(eventMap, "height")
						wordHeight = toFloat64(v)
					}
					if v, ok := eventMap["width"]; ok {
						delete(eventMap, "width")
						wordWidth = toFloat64(v)
					}
					delete(eventMap, "alt_content")

					// Python: self.check_empty(style_event, "%s style_event" % self.content_context)
					checkEmptyNotebook(eventMap, nc.contentContext+" style_event")

					word := ""
					if offset >= 0 && offset+length <= len(text) {
						word = text[offset : offset+length]
					}

					if word != "" {
						tspanElem := newSVGElement(textElem, "tspan", map[string]string{
							"x":               fmt.Sprintf("%d", int(wordLeft)),
							"y":               fmt.Sprintf("%d", int(wordTop+(wordHeight/2))),
							"textLength":      fmt.Sprintf("%d", int(wordWidth)),
							"font-size":       fmt.Sprintf("%d", int((wordWidth*2)/float64(len(word)))),
							"dominant-baseline": "middle",
						})
						tspanElem.Text = word
					}
				}
			}
		}
	} else {
		log.Printf("kfx: error: %s unknown annotation content type: %v", nc.contentContext, contentType)
	}

	// Python: self.check_empty(content, "%s content" % self.content_context)
	checkEmptyNotebook(contentMap, nc.contentContext+" content")

	nc.popContext()
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// toFloat64 converts interface{} to float64.
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	default:
		return 0
	}
}

// toInt converts interface{} to int.
func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case int32:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}

// checkEmptyNotebook logs a warning if the content map has unconsumed keys.
// Port of Python's self.check_empty(content, context) used throughout notebook processing.
func checkEmptyNotebook(content map[string]interface{}, context string) {
	for key := range content {
		log.Printf("kfx: warning: %s has unconsumed key: %s", context, key)
		return // only report once
	}
}

// ---------------------------------------------------------------------------
// ScribeNotebookContext (architectural refactor)
// ---------------------------------------------------------------------------

// ScribeNotebookContext carries the book-level state needed by
// processScribeNotebookPageSection and processScribeNotebookTemplateSection.
// In Python, these are methods on KFX_EPUB (self), accessing self.new_book_part,
// self.reading_orders, self.manifest_resource, etc.
//
// Port of Python's self (KFX_EPUB) context used by:
//   - process_scribe_notebook_page_section (yj_to_epub_notebook.py:78-156)
//   - process_scribe_notebook_template_section (yj_to_epub_notebook.py:158-218)
type ScribeNotebookContext struct {
	// notebookContext provides fragment lookup and content context stack.
	notebookContext *notebookContext

	// NmdlTemplateID is the default template ID (self.nmdl_template_id in Python).
	NmdlTemplateID string

	// WritingMode is the current writing mode (self.writing_mode in Python).
	WritingMode string

	// NewBookPart creates a new book part with the given filename.
	// Returns a *ScribeBookPart.
	// Port of Python: self.new_book_part(filename=...)
	NewBookPart func(filename string) *ScribeBookPart

	// ManifestResource registers a resource in the EPUB manifest.
	// Port of Python: self.manifest_resource(filename, data=...)
	ManifestResource func(filename string, data []byte)

	// ResourceLocationFilename generates a resource filename.
	// Port of Python: self.resource_location_filename(name, "", self.IMAGE_FILEPATH, is_symbol=False)
	ResourceLocationFilename func(name string, subdir string, filepath string, isSymbol bool) string

	// ProcessContentProperties extracts CSS properties from content data.
	// Port of Python: self.process_content_properties(section)
	ProcessContentProperties func(section map[string]interface{}) map[string]string

	// AddStyle applies CSS style properties to an SVG element.
	// Port of Python: self.add_style(elem, props, replace=...)
	AddStyle func(elem *svgElement, props map[string]string, replace bool)

	// SectionTextFilepath is the format string for section filenames.
	// Port of Python: self.SECTION_TEXT_FILEPATH (e.g. "%s.xhtml")
	SectionTextFilepath string

	// ImageFilepath is the path prefix for image resources.
	// Port of Python: self.IMAGE_FILEPATH
	ImageFilepath string

	// GetFragment looks up a fragment by type and ID.
	GetFragment func(ftype string, fid string) map[string]interface{}

	// GetNamedFragment looks up a fragment via storyline name mapping.
	GetNamedFragment func(content map[string]interface{}, ftype string, nameSymbol string) map[string]interface{}

	// ReadingOrders contains the book's reading order data.
	// Port of Python: self.reading_orders
	ReadingOrders []map[string]interface{}

	// BookParts is the list of all book parts created so far.
	// Port of Python: self.book_parts
	BookParts []*ScribeBookPart
}

// ScribeBookPart represents a book part created during scribe notebook processing.
// Port of Python's book_part object created by self.new_book_part().
type ScribeBookPart struct {
	Filename      string
	IsFXL         bool   // book_part.is_fxl
	Omit          bool   // book_part.omit (template sections set this to True)
	NmdlTemplateID string // book_part.nmdl_template_id
	HTML          *svgElement // book_part.html (root HTML element)
	Head          *svgElement // book_part.head()
	Body          *svgElement // book_part.body()
}

// NewScribeBookPart creates a new ScribeBookPart with the given filename.
func NewScribeBookPart(filename string) *ScribeBookPart {
	html := &svgElement{Tag: "html", Attrib: map[string]string{}}
	head := newSVGElement(html, "head", nil)
	body := newSVGElement(html, "body", nil)
	return &ScribeBookPart{
		Filename: filename,
		HTML:     html,
		Head:     head,
		Body:     body,
	}
}

// ---------------------------------------------------------------------------
// processScribeNotebookPageSection (yj_to_epub_notebook.py:78-156)
// ---------------------------------------------------------------------------

// processScribeNotebookPageSection processes scribe page sections with SVG stroke generation.
// Port of KFX_EPUB_Notebook.process_scribe_notebook_page_section (yj_to_epub_notebook.py:78-156).
//
// Returns true if the section was successfully processed, false otherwise.
func processScribeNotebookPageSection(ctx *ScribeNotebookContext, section map[string]interface{}, pageTemplate map[string]interface{}, sectionName string, seq int) bool {
	_ = seq

	if ctx == nil {
		return false
	}

	// Python L79-80: nmdl_canvas_width = section.pop("nmdl.canvas_width")
	var canvasWidth, canvasHeight int
	if v, ok := section["nmdl.canvas_width"]; ok {
		delete(section, "nmdl.canvas_width")
		canvasWidth = toInt(v)
	}
	if v, ok := section["nmdl.canvas_height"]; ok {
		delete(section, "nmdl.canvas_height")
		canvasHeight = toInt(v)
	}

	// Python L82-91: Validate canvas dimensions
	if !((canvasWidth == 15624 && canvasHeight == 20832) ||
		(canvasWidth == 13726 && canvasHeight == 7350) ||
		canvasWidth == 3906 || canvasWidth == 13734 ||
		canvasWidth == 3066 || canvasWidth == 6132 || canvasWidth == 12264 ||
		canvasHeight > 15000) {
		log.Printf("kfx: warning: Unexpected nmdl.canvas width=%d height=%d", canvasWidth, canvasHeight)
	}

	// Python L93-95: nmdl_normalized_ppi validation
	if v, ok := section["nmdl.normalized_ppi"]; ok {
		delete(section, "nmdl.normalized_ppi")
		ppi := toInt(v)
		if ppi != 2520 {
			log.Printf("kfx: error: Unexpected nmdl.normalized_ppi %d", ppi)
		}
	}

	// Python L97-98: book_part = self.new_book_part(filename=self.SECTION_TEXT_FILEPATH % section_name)
	sectionFilename := sectionName + ".xhtml"
	if ctx.SectionTextFilepath != "" {
		sectionFilename = fmt.Sprintf(ctx.SectionTextFilepath, sectionName)
	}

	var bookPart *ScribeBookPart
	if ctx.NewBookPart != nil {
		bookPart = ctx.NewBookPart(sectionFilename)
	} else {
		bookPart = NewScribeBookPart(sectionFilename)
	}
	bookPart.IsFXL = true

	// Python L101-107: nmdl_template_id handling
	if v, ok := section["nmdl.template_id"]; ok {
		delete(section, "nmdl.template_id")
		bookPart.NmdlTemplateID, _ = v.(string)
	} else {
		bookPart.NmdlTemplateID = ctx.NmdlTemplateID
	}

	// Python L102-107: Validate template_id is in reading_orders[1]
	if bookPart.NmdlTemplateID != "" && bookPart.NmdlTemplateID != "$349" {
		if len(ctx.ReadingOrders) != 2 ||
			getReadingOrderCategory(ctx.ReadingOrders[1]) != "note_template_collection" ||
			!readingOrderContains(ctx.ReadingOrders[1], bookPart.NmdlTemplateID) {
			log.Printf("kfx: error: note_template_collection reading order does not contain nmdl.template_id %s used in section %s",
				bookPart.NmdlTemplateID, sectionName)
		}
	}

	// Python L109-110: add_meta_name_content(book_part.head(), "viewport", ...)
	viewport := fmt.Sprintf("width=%d, height=%d", canvasWidth, canvasHeight)
	metaElem := newSVGElement(bookPart.Head, "meta", map[string]string{
		"name":    "viewport",
		"content": viewport,
	})
	_ = metaElem

	// Python L112-116: Create page SVG element
	pageSvgElem := &svgElement{
		Tag: "svg",
		Attrib: map[string]string{
			"version":             "1.1",
			"preserveAspectRatio": "xMidYMid meet",
			"viewBox":             fmt.Sprintf("0 0 %d %d", canvasWidth, canvasHeight),
		},
	}
	// Set XML namespace attributes (Python uses nsmap=SVG_NAMESPACES)
	pageSvgElem.Attrib["xmlns"] = "http://www.w3.org/2000/svg"
	pageSvgElem.Attrib["xmlns:xlink"] = "http://www.w3.org/1999/xlink"

	// Python L118-119: self.process_notebook_content(page_template, page_svg_elem)
	if ctx.notebookContext != nil {
		processNotebookContent(ctx.notebookContext, pageTemplate, pageSvgElem)
	}
	checkEmptyNotebookSafe(pageTemplate, fmt.Sprintf("Section %s page_template", sectionName))

	// Python L121-155: CREATE_SVG_FILES_IN_EPUB path
	if CREATE_SVG_FILES_IN_EPUB {
		// Python L123-126: Generate SVG filename and serialize
		pageSvgFilename := sectionName + ".svg"
		if ctx.ResourceLocationFilename != nil {
			pageSvgFilename = ctx.ResourceLocationFilename(sectionName+".svg", "", ctx.ImageFilepath, false)
		}

		svgData := serializeSVGDocument(pageSvgElem)
		if ctx.ManifestResource != nil {
			ctx.ManifestResource(pageSvgFilename, svgData)
		}

		// Python L131-136: Create HTML SVG element with white rect + image reference
		htmlSvgElem := newSVGElement(bookPart.Body, "svg", map[string]string{
			"version":             "1.1",
			"preserveAspectRatio": "xMidYMid meet",
			"viewBox":             fmt.Sprintf("0 0 %d %d", canvasWidth, canvasHeight),
			"xmlns":               "http://www.w3.org/2000/svg",
			"xmlns:xlink":         "http://www.w3.org/1999/xlink",
		})

		newSVGElement(htmlSvgElem, "rect", map[string]string{
			"x": "0", "y": "0", "width": "100%", "height": "100%", "fill": "white",
		})

		// Python L140-143: Add image reference to SVG file
		relPath := pageSvgFilename
		// urlrelpath computes the relative path from book_part.filename to page_svg_filename
		// For now, use the filename directly (both are in the same directory typically)
		newSVGElement(htmlSvgElem, "image", map[string]string{
			"x": "0", "y": "0", "width": "100%", "height": "100%",
			"xlink:href": relPath,
		})

		// Python L146-155: Handle inline_placement_type and positioning
		scribePageSectionPlacement(ctx, section, htmlSvgElem, sectionName)
	} else {
		// Python L145-146: else: body.append(page_svg_elem); html_svg_elem = page_svg_elem
		bookPart.Body.Children = append(bookPart.Body.Children, pageSvgElem)
		pageSvgElem.Parent = bookPart.Body
		htmlSvgElem := pageSvgElem

		scribePageSectionPlacement(ctx, section, htmlSvgElem, sectionName)
	}

	// Store the book part in context
	ctx.BookParts = append(ctx.BookParts, bookPart)

	return true
}

// scribePageSectionPlacement handles inline_placement_type and style positioning
// for page sections. Port of Python L146-156 in process_scribe_notebook_page_section.
func scribePageSectionPlacement(ctx *ScribeNotebookContext, section map[string]interface{}, htmlSvgElem *svgElement, sectionName string) {
	// Python L146-150: nmdl.inline_placement_type handling
	if v, ok := section["nmdl.inline_placement_type"]; ok {
		delete(section, "nmdl.inline_placement_type")
		placementType, _ := v.(string)

		if placementType != "$670" && placementType != "$669" {
			log.Printf("kfx: error: Unexpected nmdl.inline_placement_type: %s", placementType)
		}

		if ctx.ProcessContentProperties != nil {
			contentProps := ctx.ProcessContentProperties(section)
			if ctx.AddStyle != nil {
				ctx.AddStyle(htmlSvgElem, contentProps, true)
			}
		}
	} else {
		// Python L152-153: self.add_style(html_svg_elem, {"height": "100%", "width": "100%"})
		if ctx.AddStyle != nil {
			ctx.AddStyle(htmlSvgElem, map[string]string{"height": "100%", "width": "100%"}, false)
		}

		// Python L155-156: if "$58" in section or "$59" in section → fixed layout positioning
		if _, hasTop := section["top"]; hasTop || sectionHasPositionKey(section) {
			section["position"] = "fixed"
			if ctx.ProcessContentProperties != nil {
				contentProps := ctx.ProcessContentProperties(section)
				if ctx.AddStyle != nil {
					ctx.AddStyle(htmlSvgElem, contentProps, true)
				}
			}
		}
	}
}

// sectionHasPositionKey checks if section has positioning keys ($58 or $59).
// Port of Python: if "$58" in section or "$59" in section
func sectionHasPositionKey(section map[string]interface{}) bool {
	if _, ok := section["top"]; ok {
		return true
	}
	if _, ok := section["left"]; ok {
		return true
	}
	return false
}

// getReadingOrderCategory extracts the $178 category from a reading order.
// Port of Python: self.reading_orders[1].get("$178", "")
func getReadingOrderCategory(ro map[string]interface{}) string {
	if v, ok := ro["category"]; ok {
		s, _ := v.(string)
		return s
	}
	return ""
}

// readingOrderContains checks if a reading order contains the given template ID in its $170 list.
// Port of Python: book_part.nmdl_template_id not in self.reading_orders[1]["$170"]
func readingOrderContains(ro map[string]interface{}, templateID string) bool {
	if sections, ok := ro["sections"]; ok {
		if list, ok := sections.([]interface{}); ok {
			for _, item := range list {
				if s, ok := item.(string); ok && s == templateID {
					return true
				}
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// processScribeNotebookTemplateSection (yj_to_epub_notebook.py:158-218)
// ---------------------------------------------------------------------------

// processScribeNotebookTemplateSection processes scribe notebook templates.
// Port of KFX_EPUB_Notebook.process_scribe_notebook_template_section (yj_to_epub_notebook.py:158-218).
//
// Returns true if the section was successfully processed, false otherwise.
func processScribeNotebookTemplateSection(ctx *ScribeNotebookContext, section map[string]interface{}, pageTemplate map[string]interface{}, sectionName string) bool {
	if ctx == nil {
		return false
	}

	// Python L159-160: nmdl_template_type = section.pop("nmdl.template_type")
	var nmdlTemplateType string
	if v, ok := section["nmdl.template_type"]; ok {
		delete(section, "nmdl.template_type")
		nmdlTemplateType, _ = v.(string)
	}
	log.Printf("kfx: info: Notebook template: %s", nmdlTemplateType)

	// Python L162-163: book_part = self.new_book_part(filename=...)
	sectionFilename := sectionName + ".xhtml"
	if ctx.SectionTextFilepath != "" {
		sectionFilename = fmt.Sprintf(ctx.SectionTextFilepath, sectionName)
	}

	var bookPart *ScribeBookPart
	if ctx.NewBookPart != nil {
		bookPart = ctx.NewBookPart(sectionFilename)
	} else {
		bookPart = NewScribeBookPart(sectionFilename)
	}
	bookPart.IsFXL = true

	// Python L165-167: self.process_content(page_template, top_level_elem, book_part, self.writing_mode, is_section=True)
	// In Go, process_content is the main content rendering pipeline. For notebook templates,
	// we use the notebook content processing path instead.
	topLevelElem := bookPart.HTML
	if ctx.notebookContext != nil {
		processNotebookContent(ctx.notebookContext, pageTemplate, topLevelElem)
	}
	checkEmptyNotebookSafe(pageTemplate, fmt.Sprintf("Section %s page_template", sectionName))

	// Python L169-217: CREATE_SVG_FILES_IN_EPUB path for templates
	if CREATE_SVG_FILES_IN_EPUB {
		// Python L170-171: Find SVG element in body
		svgElem := findSVGElement(bookPart.Body)
		if svgElem != nil {
			// Python L172-175: Generate template SVG filename and serialize
			templateSvgFilename := nmdlTemplateType + ".svg"
			if ctx.ResourceLocationFilename != nil {
				templateSvgFilename = ctx.ResourceLocationFilename(nmdlTemplateType+".svg", "", ctx.ImageFilepath, false)
			}

			// Python L176-178: Clean up SVG element for extraction
			delete(svgElem.Attrib, "class")
			delete(svgElem.Attrib, "style")

			svgData := serializeSVGDocument(svgElem)
			if ctx.ManifestResource != nil {
				ctx.ManifestResource(templateSvgFilename, svgData)
			}

			// Python L179: book_part.omit = True
			bookPart.Omit = true

			// Python L181-216: Update page book parts that reference this template
			for _, pageBookPart := range ctx.BookParts {
				if pageBookPart.NmdlTemplateID == sectionName {
					htmlSvgElem := findSVGElement(pageBookPart.Body)
					if htmlSvgElem != nil {
						// Python L184-189: Check for existing template image and validate
						if len(htmlSvgElem.Children) == 3 {
							log.Printf("kfx: error: SVG image already has template in Scribe notebook page: %s", pageBookPart.Filename)
							// Remove the middle element (index 1) which is the old template image
							htmlSvgElem.Children = append(htmlSvgElem.Children[:1], htmlSvgElem.Children[2:]...)
						}

						// Python L191-195: Insert template image reference at position 1
						templateImage := &svgElement{
							Tag: "image",
							Attrib: map[string]string{
								"x":      "0",
								"y":      "0",
								"width":  "100%",
								"height": "100%",
								"xlink:href": templateSvgFilename,
							},
						}

						// Insert at position 1
						if len(htmlSvgElem.Children) <= 1 {
							htmlSvgElem.Children = append(htmlSvgElem.Children, templateImage)
						} else {
							htmlSvgElem.Children = append(
								htmlSvgElem.Children[:1],
								append([]*svgElement{templateImage}, htmlSvgElem.Children[1:]...)...,
							)
						}
					} else {
						log.Printf("kfx: error: Failed to locate the SVG image within Scribe notebook page: %s", pageBookPart.Filename)
					}
				}
			}
		} else {
			log.Printf("kfx: error: Failed to locate the SVG image within Scribe notebook template: %s", bookPart.Filename)
		}
	}

	// Store the book part
	ctx.BookParts = append(ctx.BookParts, bookPart)

	return true
}

// findSVGElement finds the first <svg> child element.
func findSVGElement(parent *svgElement) *svgElement {
	for _, child := range parent.Children {
		if child.Tag == "svg" {
			return child
		}
	}
	return nil
}

// serializeSVGDocument serializes an SVG element tree to bytes with DOCTYPE.
// Port of Python: etree.tostring(svg_document, encoding="utf-8", doctype=SVG_DOCTYPE, xml_declaration=True)
func serializeSVGDocument(root *svgElement) []byte {
	var buf bytes.Buffer
	buf.WriteString("<?xml version='1.0' encoding='utf-8'?>\n")
	buf.Write(SVG_DOCTYPE)
	buf.WriteByte('\n')
	serializeSVGElement(&buf, root, 0)
	return buf.Bytes()
}

// serializeSVGElement recursively serializes an SVG element to XML.
func serializeSVGElement(buf *bytes.Buffer, elem *svgElement, indent int) {
	prefix := ""
	for i := 0; i < indent; i++ {
		prefix += "  "
	}

	buf.WriteString(prefix)
	buf.WriteByte('<')
	buf.WriteString(elem.Tag)

	// Write attributes in a deterministic order
	keys := make([]string, 0, len(elem.Attrib))
	for k := range elem.Attrib {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		buf.WriteByte(' ')
		buf.WriteString(k)
		buf.WriteString("=\"")
		buf.WriteString(elem.Attrib[k])
		buf.WriteByte('"')
	}

	if len(elem.Children) == 0 && elem.Text == "" {
		buf.WriteString("/>\n")
		return
	}

	buf.WriteByte('>')

	if elem.Text != "" {
		buf.WriteString(elem.Text)
	}

	if len(elem.Children) > 0 {
		buf.WriteByte('\n')
		for _, child := range elem.Children {
			serializeSVGElement(buf, child, indent+1)
		}
		buf.WriteString(prefix)
	}

	buf.WriteString("</")
	buf.WriteString(elem.Tag)
	buf.WriteString(">\n")
}

// checkEmptyNotebookSafe is a nil-safe version of checkEmptyNotebook.
func checkEmptyNotebookSafe(content map[string]interface{}, context string) {
	if content == nil {
		return
	}
	checkEmptyNotebook(content, context)
}
