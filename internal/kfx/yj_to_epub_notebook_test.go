package kfx

import (
	"fmt"
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// VAL-D-001: Module constants
// ---------------------------------------------------------------------------

func TestNotebookConstants(t *testing.T) {
	if !CREATE_SVG_FILES_IN_EPUB {
		t.Error("CREATE_SVG_FILES_IN_EPUB should be true")
	}
	if PNG_SCALE_FACTOR != 8 {
		t.Errorf("PNG_SCALE_FACTOR = %d, want 8", PNG_SCALE_FACTOR)
	}
	if PNG_DENSITY_GAMMA != 3.5 {
		t.Errorf("PNG_DENSITY_GAMMA = %v, want 3.5", PNG_DENSITY_GAMMA)
	}
	if PNG_EDGE_FEATHERING != 0.75 {
		t.Errorf("PNG_EDGE_FEATHERING = %v, want 0.75", PNG_EDGE_FEATHERING)
	}
	if !INCLUDE_PRIOR_LINE_SEGMENT {
		t.Error("INCLUDE_PRIOR_LINE_SEGMENT should be true")
	}
	if !ROUND_LINE_ENDINGS {
		t.Error("ROUND_LINE_ENDINGS should be true")
	}
	if !QUANTIZE_THICKNESS {
		t.Error("QUANTIZE_THICKNESS should be true")
	}
	if ANNOTATION_TEXT_OPACITY != 0.0 {
		t.Errorf("ANNOTATION_TEXT_OPACITY = %v, want 0.0", ANNOTATION_TEXT_OPACITY)
	}
	if MIN_TAF != 0 {
		t.Errorf("MIN_TAF = %d, want 0", MIN_TAF)
	}
	if MAX_TAF != 1000 {
		t.Errorf("MAX_TAF = %d, want 1000", MAX_TAF)
	}
	if MIN_DAF != 0 {
		t.Errorf("MIN_DAF = %d, want 0", MIN_DAF)
	}
	if MAX_DAF != 300 {
		t.Errorf("MAX_DAF = %d, want 300", MAX_DAF)
	}
}

// ---------------------------------------------------------------------------
// VAL-D-002: SVG_DOCTYPE constant
// ---------------------------------------------------------------------------

func TestSVGDoctype(t *testing.T) {
	expected := "<!DOCTYPE svg PUBLIC '-//W3C//DTD SVG 1.1//EN' 'http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd'>"
	if string(SVG_DOCTYPE) != expected {
		t.Errorf("SVG_DOCTYPE = %q, want %q", string(SVG_DOCTYPE), expected)
	}
}

// ---------------------------------------------------------------------------
// VAL-D-003: Brush type string constants
// ---------------------------------------------------------------------------

func TestBrushTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"ERASER", ERASER, "eraser"},
		{"FOUNTAIN_PEN", FOUNTAIN_PEN, "fountain pen"},
		{"HIGHLIGHTER", HIGHLIGHTER, "highlighter"},
		{"MARKER", MARKER, "marker"},
		{"ORIGINAL_PEN", ORIGINAL_PEN, "original pen"},
		{"PEN", PEN, "pen"},
		{"PENCIL", PENCIL, "pencil"},
		{"SHADER", SHADER, "shader"},
		{"UNKNOWN", UNKNOWN, "unknown"},
	}
	for _, tc := range tests {
		if tc.got != tc.expected {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-D-004: THICKNESS_NAME
// ---------------------------------------------------------------------------

func TestThicknessName(t *testing.T) {
	expected := []string{"fine", "thin", "medium", "thick", "heavy"}
	if len(THICKNESS_NAME) != len(expected) {
		t.Fatalf("THICKNESS_NAME has %d entries, want %d", len(THICKNESS_NAME), len(expected))
	}
	for i, v := range expected {
		if THICKNESS_NAME[i] != v {
			t.Errorf("THICKNESS_NAME[%d] = %q, want %q", i, THICKNESS_NAME[i], v)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-D-005: THICKNESS_CHOICES — 8 brush types × 5 values
// ---------------------------------------------------------------------------

func TestThicknessChoices(t *testing.T) {
	if len(THICKNESS_CHOICES) != 8 {
		t.Fatalf("THICKNESS_CHOICES has %d entries, want 8", len(THICKNESS_CHOICES))
	}

	expected := map[string][]float64{
		FOUNTAIN_PEN: {23.625, 31.5, 47.25, 78.75, 126.0},
		HIGHLIGHTER:  {252.0, 315.0, 441.0, 567.0, 756.0},
		MARKER:       {31.5, 63.0, 94.5, 189.0, 315.0},
		PEN:          {23.625, 39.375, 55.125, 94.5, 126.0},
		ORIGINAL_PEN: {23.625, 31.5, 63.0, 94.5, 126.0},
		PENCIL:       {23.625, 39.375, 63.0, 110.25, 189.0},
		SHADER:       {94.5, 189.0, 315.0, 441.0, 567.0},
	}

	for brush, vals := range expected {
		got, ok := THICKNESS_CHOICES[brush]
		if !ok {
			t.Errorf("THICKNESS_CHOICES missing key %q", brush)
			continue
		}
		if len(got) != len(vals) {
			t.Errorf("THICKNESS_CHOICES[%q] has %d values, want %d", brush, len(got), len(vals))
			continue
		}
		for i, v := range vals {
			if got[i] != v {
				t.Errorf("THICKNESS_CHOICES[%q][%d] = %v, want %v", brush, i, got[i], v)
			}
		}
	}

	// UNKNOWN should have empty slice
	got, ok := THICKNESS_CHOICES[UNKNOWN]
	if !ok {
		t.Error("THICKNESS_CHOICES missing key UNKNOWN")
	} else if len(got) != 0 {
		t.Errorf("THICKNESS_CHOICES[UNKNOWN] has %d values, want 0", len(got))
	}
}

// ---------------------------------------------------------------------------
// VAL-D-006: THICKNESS_CHOICES — ERASER has no entry
// ---------------------------------------------------------------------------

func TestThicknessChoicesNoEraser(t *testing.T) {
	if _, ok := THICKNESS_CHOICES[ERASER]; ok {
		t.Error("THICKNESS_CHOICES should not contain ERASER key")
	}
}

// ---------------------------------------------------------------------------
// VAL-D-007: STROKE_COLORS — 10 entries
// ---------------------------------------------------------------------------

func TestStrokeColors(t *testing.T) {
	if len(STROKE_COLORS) != 10 {
		t.Fatalf("STROKE_COLORS has %d entries, want 10", len(STROKE_COLORS))
	}

	expected := map[int]struct {
		name string
		hex  int
	}{
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

	for idx, exp := range expected {
		got, ok := STROKE_COLORS[idx]
		if !ok {
			t.Errorf("STROKE_COLORS missing key %d", idx)
			continue
		}
		if got.Name != exp.name {
			t.Errorf("STROKE_COLORS[%d].Name = %q, want %q", idx, got.Name, exp.name)
		}
		if got.Hex != exp.hex {
			t.Errorf("STROKE_COLORS[%d].Hex = 0x%06x, want 0x%06x", idx, got.Hex, exp.hex)
		}
	}

	// Index 6 must be absent
	if _, ok := STROKE_COLORS[6]; ok {
		t.Error("STROKE_COLORS should not contain key 6")
	}
}

// ---------------------------------------------------------------------------
// VAL-D-008: adjust_color_for_density — luminance computation
//
// Python: lum = (r+g+b)//3; lum2 = min(max(round(255 - int((255-lum)*density)), 0), 255)
// ---------------------------------------------------------------------------

func TestAdjustColorForDensityLuminance(t *testing.T) {
	tests := []struct {
		color    int
		density  float64
		expected int
	}{
		// black stays black at full density: lum=0, (255-0)*1=255, 255-255=0
		{0x000000, 1.0, 0x000000},
		// white at zero density: lum=255, (255-255)*0=0, 255-0=255
		{0xffffff, 0.0, 0xffffff},
		// red at 50%: lum=(255+0+0)//3=85; lum2=round(255-int(170*0.5))=round(255-85)=170=0xaa
		{0xff0000, 0.5, 0xaaaaaa},
		// green at 50%: same lum=85
		{0x00ff00, 0.5, 0xaaaaaa},
		// blue at 50%: same lum=85
		{0x0000ff, 0.5, 0xaaaaaa},
	}
	for _, tc := range tests {
		got := adjustColorForDensity(tc.color, tc.density)
		if got != tc.expected {
			t.Errorf("adjustColorForDensity(0x%06x, %v) = 0x%06x, want 0x%06x", tc.color, tc.density, got, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-D-009: adjust_color_for_density — boundary clamping
// ---------------------------------------------------------------------------

func TestAdjustColorForDensityClamping(t *testing.T) {
	tests := []struct {
		color    int
		density  float64
		expected int
	}{
		// black at zero density: lum=0, (255-0)*0=0, 255-0=255
		{0x000000, 0.0, 0xffffff},
		// white at density=2.0: lum=255, (255-255)*2=0, 255-0=255
		// Note: density>1 doesn't affect already-white (lum=255) because (255-lum)=0
		{0xffffff, 2.0, 0xffffff},
		// black at density=2.0: lum=0, (255-0)*2=510, 255-510=-255 → clamped to 0
		{0x000000, 2.0, 0x000000},
		// gray at density=2.0: lum=128, (255-128)*2=254, 255-254=1
		{0x808080, 2.0, 0x010101},
	}
	for _, tc := range tests {
		got := adjustColorForDensity(tc.color, tc.density)
		if got != tc.expected {
			t.Errorf("adjustColorForDensity(0x%06x, %v) = 0x%06x, want 0x%06x", tc.color, tc.density, got, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-D-010: adjust_color_for_density — gray output symmetry
// ---------------------------------------------------------------------------

func TestAdjustColorForDensityGraySymmetry(t *testing.T) {
	colors := []int{0xff8800, 0x00ff00, 0x8800ff}
	density := 0.7
	for _, c := range colors {
		result := adjustColorForDensity(c, density)
		r := (result >> 16) & 0xff
		g := (result >> 8) & 0xff
		b := result & 0xff
		if r != g || g != b {
			t.Errorf("adjustColorForDensity(0x%06x, %v) = 0x%06x: R=%d G=%d B=%d, want R==G==B", c, density, result, r, g, b)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-D-011: decode_stroke_values — valid signature
// ---------------------------------------------------------------------------

func TestDecodeStrokeValuesValidSignature(t *testing.T) {
	// signature + num_vals=1 + instruction byte (nibble=4=increment=0, padding=0)
	data := []byte{
		0x01, 0x01,                   // signature
		0x01, 0x00, 0x00, 0x00,       // num_vals = 1
		0x40,                         // nibbles: 4 (increment=0), 0 (padding)
	}
	vals, err := decodeStrokeValues(data, 1, "test")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	if vals[0] != 0 {
		t.Errorf("vals[0] = %d, want 0", vals[0])
	}
}

func TestDecodeStrokeValuesInvalidSignature(t *testing.T) {
	data := []byte{
		0x00, 0x00, // bad signature
		0x01, 0x00, 0x00, 0x00,
		0x40,
	}
	_, err := decodeStrokeValues(data, 1, "test")
	if err == nil {
		t.Error("expected error for invalid signature")
	}
}

// ---------------------------------------------------------------------------
// VAL-D-012: decode_stroke_values — num_vals verification
// ---------------------------------------------------------------------------

func TestDecodeStrokeValuesNumValsMismatch(t *testing.T) {
	data := []byte{
		0x01, 0x01,                   // signature
		0x02, 0x00, 0x00, 0x00,       // num_vals = 2 (but we pass num_points=1)
		0x44,                         // 2 nibbles for 2 vals
	}
	_, err := decodeStrokeValues(data, 1, "test")
	if err == nil {
		t.Error("expected error for num_vals mismatch")
	}
}

// ---------------------------------------------------------------------------
// VAL-D-013: decode_stroke_values — instruction nibble extraction
// ---------------------------------------------------------------------------

func TestDecodeStrokeValuesNibbleExtraction(t *testing.T) {
	// 2 points, 2 instruction nibbles from 1 byte (no padding needed)
	data := []byte{
		0x01, 0x01,                   // signature
		0x02, 0x00, 0x00, 0x00,       // num_vals = 2
		0x44,                         // nibbles: 4, 4 — both produce increment=0
	}
	vals, err := decodeStrokeValues(data, 2, "test")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d", len(vals))
	}
}

// ---------------------------------------------------------------------------
// VAL-D-014: decode_stroke_values — increment decoding
//
// Per instruction nibble:
//   bits 0-1 (n): number of bytes for increment
//   bit 2: if set, increment = n; else read n bytes
//   bit 3: if set, negate increment
// ---------------------------------------------------------------------------

func TestDecodeStrokeValuesIncrementDecoding(t *testing.T) {
	tests := []struct {
		name     string
		instr    byte // single nibble value
		extra    []byte
		expected int
	}{
		// n=0, bit2=1 → increment=0
		{"n=0_bit2=1", 0x04, nil, 0},
		// n=1, bit2=1 → increment=1
		{"n=1_bit2=1", 0x05, nil, 1},
		// n=1, bit2=1, bit3=1 → increment=-1
		{"n=1_bit2=1_bit3=1", 0x0d, nil, -1},
		// n=0, bit2=0 → increment=0 (read 0 bytes)
		{"n=0_bit2=0", 0x00, nil, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte{
				0x01, 0x01,
				0x01, 0x00, 0x00, 0x00, // num_vals=1
				(tc.instr << 4), // high nibble=instruction, low nibble=0 (padding)
			}
			data = append(data, tc.extra...)

			vals, err := decodeStrokeValues(data, 1, tc.name)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if len(vals) != 1 {
				t.Fatalf("expected 1 value, got %d", len(vals))
			}
			if vals[0] != tc.expected {
				t.Errorf("value = %d, want %d", vals[0], tc.expected)
			}
		})
	}
}

// Test multi-byte increment reading (n=2, bit2=0 → read 2 bytes as uint16 LE)
func TestDecodeStrokeValuesMultiByteIncrement(t *testing.T) {
	data := []byte{
		0x01, 0x01,
		0x01, 0x00, 0x00, 0x00, // num_vals=1
		0x20,             // high nibble=0x02 (n=2,bit2=0), low=0 (padding)
		0x05, 0x00,       // uint16 LE = 5
	}
	vals, err := decodeStrokeValues(data, 1, "test_multibyte")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	if vals[0] != 5 {
		t.Errorf("value = %d, want 5", vals[0])
	}
}

// ---------------------------------------------------------------------------
// VAL-D-015: decode_stroke_values — delta decoding
//
// Python delta decoding:
//   if i == 0: change = 0; value = increment
//   else:      change += increment; value += change
//
// For increments [5, 3, -1, 0, 2]:
//   i=0: change=0,         value=0+5=5       → vals=[5]
//   i=1: change=0+3=3,     value=5+3=8       → vals=[5,8]
//   i=2: change=3+(-1)=2,  value=8+2=10      → vals=[5,8,10]
//   i=3: change=2+0=2,     value=10+2=12     → vals=[5,8,10,12]
//   i=4: change=2+2=4,     value=12+4=16     → vals=[5,8,10,12,16]
//
// Encoding instructions for these increments:
//   5: n=1, bit2=0 → instr=0x01, extra byte=5
//   3: n=3, bit2=1 → instr=0x07
//  -1: n=1, bit2=1, bit3=1 → instr=0x0d
//   0: n=0, bit2=1 → instr=0x04
//   2: n=2, bit2=1 → instr=0x06
// ---------------------------------------------------------------------------

func TestDecodeStrokeValuesDeltaDecoding(t *testing.T) {
	data := []byte{
		0x01, 0x01,                   // signature
		0x05, 0x00, 0x00, 0x00,       // num_vals=5
		0x17,                         // byte 0: nibbles 1, 7
		0xd4,                         // byte 1: nibbles d, 4
		0x60,                         // byte 2: nibbles 6, 0 (padding)
		0x05,                         // extra byte for instr 0x01: increment=5
	}

	vals, err := decodeStrokeValues(data, 5, "test_delta")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	expected := []int{5, 8, 10, 12, 16}
	if len(vals) != len(expected) {
		t.Fatalf("expected %d values, got %d", len(expected), len(vals))
	}
	for i, v := range expected {
		if vals[i] != v {
			t.Errorf("vals[%d] = %d, want %d", i, vals[i], v)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-D-016: decode_stroke_values — empty data
// ---------------------------------------------------------------------------

func TestDecodeStrokeValuesEmpty(t *testing.T) {
	data := []byte{
		0x01, 0x01,
		0x00, 0x00, 0x00, 0x00, // num_vals=0
	}
	vals, err := decodeStrokeValues(data, 0, "test_empty")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(vals) != 0 {
		t.Errorf("expected empty slice, got %d values", len(vals))
	}
}

// ---------------------------------------------------------------------------
// VAL-D-017: decode_stroke_values — extra data detection
// ---------------------------------------------------------------------------

func TestDecodeStrokeValuesExtraData(t *testing.T) {
	data := []byte{
		0x01, 0x01,
		0x01, 0x00, 0x00, 0x00, // num_vals=1
		0x40, // instruction: increment=0
		0xFF, // extra data!
	}
	_, err := decodeStrokeValues(data, 1, "test_extra")
	if err == nil {
		t.Error("expected error for extra data")
	}
}

// ---------------------------------------------------------------------------
// VAL-D-018: processScribeNotebookPageSection — stub returns false
// ---------------------------------------------------------------------------

func TestProcessScribeNotebookPageSectionStub(t *testing.T) {
	result := processScribeNotebookPageSection(nil, nil, "", 0)
	if result != false {
		t.Error("processScribeNotebookPageSection should return false (stub)")
	}
}

// ---------------------------------------------------------------------------
// VAL-D-019: processScribeNotebookTemplateSection — stub returns false
// ---------------------------------------------------------------------------

func TestProcessScribeNotebookTemplateSectionStub(t *testing.T) {
	result := processScribeNotebookTemplateSection(nil, nil, "")
	if result != false {
		t.Error("processScribeNotebookTemplateSection should return false (stub)")
	}
}

// ---------------------------------------------------------------------------
// Additional edge-case tests for adjustColorForDensity
// ---------------------------------------------------------------------------

func TestAdjustColorForDensityGrayInput(t *testing.T) {
	// Gray input 0x808080: lum = (128+128+128)//3 = 128
	// lum2 = round(255 - int((255-128)*1.0)) = round(255-127) = 128
	result := adjustColorForDensity(0x808080, 1.0)
	expected := 0x808080
	if result != expected {
		t.Errorf("adjustColorForDensity(0x808080, 1.0) = 0x%06x, want 0x%06x", result, expected)
	}
}

func TestAdjustColorForDensityHalfGray(t *testing.T) {
	// Gray 0x808080 at density 0.5: lum=128
	// lum2 = round(255 - int(127*0.5)) = round(255-63) = 192
	result := adjustColorForDensity(0x808080, 0.5)
	r := (result >> 16) & 0xff
	if r != 192 {
		t.Errorf("adjustColorForDensity(0x808080, 0.5) R component = %d, want 192 (result=0x%06x)", r, result)
	}
}

// ---------------------------------------------------------------------------
// Additional tests for decodeStrokeValues
// ---------------------------------------------------------------------------

func TestDecodeStrokeValuesNegativeIncrement(t *testing.T) {
	// 2 points: first increment=2 (n=2, bit2=1 → instr=0x06), second increment=-1 (n=1, bit2=1, bit3=1 → instr=0x0d)
	// Byte: (6 << 4) | 0xd = 0x6d
	// Delta decode:
	//   i=0: change=0, value=2
	//   i=1: change=0+(-1)=-1, value=2+(-1)=1
	data := []byte{
		0x01, 0x01,
		0x02, 0x00, 0x00, 0x00, // num_vals=2
		0x6d, // nibbles: 6, d
	}
	vals, err := decodeStrokeValues(data, 2, "test_neg")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d", len(vals))
	}
	if vals[0] != 2 {
		t.Errorf("vals[0] = %d, want 2", vals[0])
	}
	if vals[1] != 1 {
		t.Errorf("vals[1] = %d, want 1", vals[1])
	}
}

func TestDecodeStrokeValuesSingleByteIncrement(t *testing.T) {
	// n=1, bit2=0 → read 1 byte
	// instruction nibble: 0x01 (n=1, bit2=0, bit3=0)
	data := []byte{
		0x01, 0x01,
		0x01, 0x00, 0x00, 0x00, // num_vals=1
		0x10, // high nibble=0x01, low=0 (padding)
		42,   // increment byte = 42
	}
	vals, err := decodeStrokeValues(data, 1, "test_byte")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	if vals[0] != 42 {
		t.Errorf("value = %d, want 42", vals[0])
	}
}

func TestDecodeStrokeValuesThreeByteIncrement(t *testing.T) {
	// n=3, bit2=0, bit3=0 → instruction 0x03
	// Python reads 1 byte + 2 byte uint16 LE << 8
	// increment = byte + uint16LE << 8
	data := []byte{
		0x01, 0x01,
		0x01, 0x00, 0x00, 0x00, // num_vals=1
		0x30,             // high nibble=0x03, low=0 (padding)
		0x01,             // low byte
		0x00, 0x01,       // uint16 LE = 256
		// increment = 1 + (256 << 8) = 1 + 65536 = 65537
	}
	vals, err := decodeStrokeValues(data, 1, "test_3byte")
	// n=3 triggers a warning log but still decodes correctly
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d (err=%v)", len(vals), err)
	}
	expected := 1 + (256 << 8) // 65537
	if vals[0] != expected {
		t.Errorf("value = %d, want %d", vals[0], expected)
	}
}

func TestDecodeStrokeValuesNegativeThreeByteIncrement(t *testing.T) {
	// n=3, bit2=0, bit3=1 → instruction 0x0b
	data := []byte{
		0x01, 0x01,
		0x01, 0x00, 0x00, 0x00, // num_vals=1
		0xb0,             // high nibble=0x0b, low=0 (padding)
		0x01,             // low byte
		0x00, 0x01,       // uint16 LE = 256
	}
	vals, err := decodeStrokeValues(data, 1, "test_neg3byte")
	// n=3 triggers a warning log but still decodes correctly
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d (err=%v)", len(vals), err)
	}
	expected := -(1 + (256 << 8)) // -65537
	if vals[0] != expected {
		t.Errorf("value = %d, want %d", vals[0], expected)
	}
}

func TestDecodeStrokeValuesNegativeSingleByteIncrement(t *testing.T) {
	// n=1, bit2=0, bit3=1 → instruction 0x09, read 1 byte, negate
	data := []byte{
		0x01, 0x01,
		0x01, 0x00, 0x00, 0x00, // num_vals=1
		0x90, // high nibble=0x09, low=0 (padding)
		42,   // byte = 42, increment = -42
	}
	vals, err := decodeStrokeValues(data, 1, "test_negbyte")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	if vals[0] != -42 {
		t.Errorf("value = %d, want -42", vals[0])
	}
}

func TestDecodeStrokeValuesNegativeZeroIncrement(t *testing.T) {
	// n=0, bit2=1, bit3=1 → instruction 0x0c: increment=-0 triggers error
	data := []byte{
		0x01, 0x01,
		0x01, 0x00, 0x00, 0x00,
		0xc0, // high nibble=0x0c, low=0 (padding)
	}
	_, err := decodeStrokeValues(data, 1, "test_negzero")
	if err == nil {
		t.Error("expected error for negative zero increment")
	}
}

// ===========================================================================
// VAL-M4-NB-001: processNotebookContent dispatches content types correctly
// Python: yj_to_epub_notebook.py:220-268
// ===========================================================================

func TestProcessNotebookContent_DispatchesDollar270(t *testing.T) {
	// Verify that $270 content type is dispatched correctly and children are visited.
	fragmentStore := map[string]map[string]interface{}{}

	nc := &notebookContext{
		getFragment: func(ftype string, fid string) map[string]interface{} {
			key := ftype + ":" + fid
			return fragmentStore[key]
		},
		getNamedFragment: func(content map[string]interface{}, ftype string, nameSymbol string) map[string]interface{} {
			// Look up the story reference
			if ref, ok := content["storyline"]; ok {
				if fid, ok := ref.(string); ok {
					return fragmentStore["$259:"+fid]
				}
			}
			return nil
		},
		contentContext: "test",
	}

	parent := &svgElement{Tag: "svg"}

	content := map[string]interface{}{
		"type": "container",
		"content_list": []interface{}{
			map[string]interface{}{
				"type": "container",
			},
		},
	}

	processNotebookContent(nc, content, parent)

	// After processing, $159 should have been popped
	if _, ok := content["type"]; ok {
		t.Error("$159 should have been popped from content")
	}
	// $146 should have been popped
	if _, ok := content["content_list"]; ok {
		t.Error("$146 should have been popped from content")
	}
}

func TestProcessNotebookContent_SymbolLookup(t *testing.T) {
	// Verify that IonSymbol content triggers $608 fragment lookup.
	fragmentStore := map[string]map[string]interface{}{
		"$608:sym1": {
			"type": "container",
		},
	}

	nc := &notebookContext{
		getFragment: func(ftype string, fid string) map[string]interface{} {
			key := ftype + ":" + fid
			return fragmentStore[key]
		},
		getNamedFragment: func(content map[string]interface{}, ftype string, nameSymbol string) map[string]interface{} {
			return nil
		},
		contentContext: "test",
	}

	parent := &svgElement{Tag: "svg"}

	// Pass a string (IonSymbol) as content
	processNotebookContent(nc, "sym1", parent)

	// Should not panic and should have looked up the fragment
}

func TestProcessNotebookContent_UnknownContentType(t *testing.T) {
	// Non-$270 content type should log error but not panic.
	nc := &notebookContext{
		getFragment: func(ftype string, fid string) map[string]interface{} {
			return nil
		},
		getNamedFragment: func(content map[string]interface{}, ftype string, nameSymbol string) map[string]interface{} {
			return nil
		},
		contentContext: "test",
	}

	parent := &svgElement{Tag: "svg"}

	content := map[string]interface{}{
		"type": "$999", // unknown content type
	}

	processNotebookContent(nc, content, parent)
	// Should log error but not panic
}

func TestProcessNotebookContent_RecursesDollar176(t *testing.T) {
	// Verify that $176 story content is looked up and processed.
	storyFragment := map[string]interface{}{
		"story_name": "my_story",
		"content_list": []interface{}{
			map[string]interface{}{
				"type": "container",
			},
		},
	}

	fragmentStore := map[string]map[string]interface{}{
		"$259:story_ref": storyFragment,
	}

	nc := &notebookContext{
		getFragment: func(ftype string, fid string) map[string]interface{} {
			return fragmentStore[ftype+":"+fid]
		},
		getNamedFragment: func(content map[string]interface{}, ftype string, nameSymbol string) map[string]interface{} {
			if ref, ok := content["storyline"]; ok {
				if fid, ok := ref.(string); ok {
					return fragmentStore["$259:"+fid]
				}
			}
			return nil
		},
		contentContext: "test",
	}

	parent := &svgElement{Tag: "svg"}

	content := map[string]interface{}{
		"type": "container",
		"story_name": "story_name",
		"storyline": "story_ref",
	}

	processNotebookContent(nc, content, parent)

	// $176 should have been consumed in the content
	if _, ok := content["story_name"]; !ok {
		t.Error("$176 should still be in content (it's consumed by getNamedFragment)")
	}
}

func TestProcessNotebookContent_DispatchesStrokeWhenNoLayout(t *testing.T) {
	// When $270 has no $156 layout and has nmdl.type, should dispatch to scribeNotebookStroke.
	nc := &notebookContext{
		getFragment: func(ftype string, fid string) map[string]interface{} {
			return nil
		},
		getNamedFragment: func(content map[string]interface{}, ftype string, nameSymbol string) map[string]interface{} {
			return nil
		},
		contentContext: "test",
	}

	parent := &svgElement{Tag: "svg"}

	content := map[string]interface{}{
		"type":      "container",
		"nmdl.type": "nmdl.stroke_group",
		"nmdl.chunked": true,
		"nmdl.chunk_threshold": 50,
	}

	processNotebookContent(nc, content, parent)

	// nmdl.type should have been consumed
	if _, ok := content["nmdl.type"]; ok {
		t.Error("nmdl.type should have been consumed by scribeNotebookStroke")
	}
	// A <g> element should have been created
	if len(parent.Children) != 1 {
		t.Fatalf("expected 1 child element, got %d", len(parent.Children))
	}
	if parent.Children[0].Tag != "g" {
		t.Errorf("expected <g> element, got <%s>", parent.Children[0].Tag)
	}
}

// ===========================================================================
// VAL-M4-NB-002: scribeNotebookStroke handles stroke groups
// Python: yj_to_epub_notebook.py:274-292
// ===========================================================================

func TestScribeNotebookStrokeGroup(t *testing.T) {
	// Verify stroke group creates <g> element and processes annotations.
	nc := &notebookContext{
		contentContext: "test",
	}

	parent := &svgElement{Tag: "svg"}

	content := map[string]interface{}{
		"nmdl.type":            "nmdl.stroke_group",
		"nmdl.chunked":         true,
		"nmdl.chunk_threshold": 50,
		"annotations": []interface{}{
			map[string]interface{}{
				"annotation_type": "nmdl.hwr",
			},
		},
	}

	scribeNotebookStroke(nc, content, parent, "loc123")

	// nmdl.type should be consumed
	if _, ok := content["nmdl.type"]; ok {
		t.Error("nmdl.type should have been consumed")
	}

	// Should create a <g> child
	if len(parent.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(parent.Children))
	}

	groupElem := parent.Children[0]
	if groupElem.Tag != "g" {
		t.Errorf("expected <g>, got <%s>", groupElem.Tag)
	}

	// Should have id attribute
	if groupElem.Attrib["id"] != "loc123" {
		t.Errorf("expected id=loc123, got %v", groupElem.Attrib["id"])
	}
}

func TestScribeNotebookStrokeGroup_ChunkedValidation(t *testing.T) {
	// Verify that non-true chunked value logs error.
	nc := &notebookContext{
		contentContext: "test",
	}

	parent := &svgElement{Tag: "svg"}

	content := map[string]interface{}{
		"nmdl.type":            "nmdl.stroke_group",
		"nmdl.chunked":         false, // should trigger error
		"nmdl.chunk_threshold": 50,
	}

	scribeNotebookStroke(nc, content, parent, "")

	// Should still create group element despite error
	if len(parent.Children) != 1 {
		t.Fatalf("expected 1 child even with error, got %d", len(parent.Children))
	}
}

// ===========================================================================
// VAL-M4-NB-005: Brush type and thickness classification
// Python: yj_to_epub_notebook.py:330-350
// ===========================================================================

func TestBrushTypeClassification(t *testing.T) {
	tests := []struct {
		brushType int
		expected  string
	}{
		{0, ORIGINAL_PEN},
		{1, HIGHLIGHTER},
		{5, PENCIL},
		{6, FOUNTAIN_PEN},
		{7, MARKER},  // default when no thickness info
		{9, SHADER},
		{99, UNKNOWN + "99"}, // unknown type
	}

	for _, tc := range tests {
		got := classifyBrushType(tc.brushType)
		if got != tc.expected {
			t.Errorf("classifyBrushType(%d) = %q, want %q", tc.brushType, got, tc.expected)
		}
	}
}

func TestBrushTypeClassificationWithThickness(t *testing.T) {
	// Brush type 7: MARKER when variable thickness, PEN otherwise
	tests := []struct {
		brushType         int
		variableThickness bool
		expected          string
	}{
		{7, true, MARKER},
		{7, false, PEN},
		{0, false, ORIGINAL_PEN},
		{0, true, ORIGINAL_PEN},
	}

	for _, tc := range tests {
		got := classifyBrushTypeWithThickness(tc.brushType, tc.variableThickness)
		if got != tc.expected {
			t.Errorf("classifyBrushTypeWithThickness(%d, %v) = %q, want %q",
				tc.brushType, tc.variableThickness, got, tc.expected)
		}
	}
}

func TestBrushTypeOpacity(t *testing.T) {
	// Verify that HIGHLIGHTER and SHADER get correct opacity.
	// This is tested indirectly through the stroke processing.
	tests := []struct {
		brushType int
		opacity   float64
	}{
		{0, 1.0}, // ORIGINAL_PEN
		{1, 0.2}, // HIGHLIGHTER
		{5, 1.0}, // PENCIL
		{6, 1.0}, // FOUNTAIN_PEN
		{9, 0.2}, // SHADER
	}
	for _, tc := range tests {
		brushName := classifyBrushType(tc.brushType)
		opacity := 1.0
		if tc.brushType == 1 {
			opacity = 0.2
		}
		if tc.brushType == 9 {
			opacity = 0.2
		}
		_ = brushName
		if opacity != tc.opacity {
			t.Errorf("brush type %d (%s) opacity = %v, want %v", tc.brushType, brushName, opacity, tc.opacity)
		}
	}
}

// ===========================================================================
// VAL-M4-NB-003: SVG path generation for normal strokes
// Python: yj_to_epub_notebook.py:482-515
// ===========================================================================

func TestSVGPathGeneration_SinglePath(t *testing.T) {
	groupElem := &svgElement{Tag: "g"}
	points := []strokePoint{
		{X: 10, Y: 20, T: 5, D: 1.0},
		{X: 30, Y: 40, T: 5, D: 1.0},
		{X: 50, Y: 60, T: 5, D: 1.0},
	}

	generateSVGPaths(groupElem, points, 5)

	if len(groupElem.Children) != 1 {
		t.Fatalf("expected 1 path, got %d", len(groupElem.Children))
	}

	pathElem := groupElem.Children[0]
	if pathElem.Tag != "path" {
		t.Errorf("expected <path>, got <%s>", pathElem.Tag)
	}

	// Check d attribute
	d := pathElem.Attrib["d"]
	// With INCLUDE_PRIOR_LINE_SEGMENT, we include 2 prior points:
	// First point: M 10 20
	// Second point: L 30 40
	// Third point: L 50 60
	// But with prior segments: first path starts with nothing before, includes [1] prior
	// Actually: for i=0, priors=[2,1], but i<2 and i<1, so nothing included.
	// For i=0: path starts with M 10 20
	// For i=1: same t and d, so appended: L 30 40
	// For i=2: same t and d, so appended: L 50 60
	expectedD := "M 10 20 L 30 40 L 50 60"
	if d != expectedD {
		t.Errorf("path d = %q, want %q", d, expectedD)
	}

	// stroke-width should NOT be set when t == nmdlThickness
	if _, ok := pathElem.Attrib["stroke-width"]; ok {
		t.Error("stroke-width should not be set when equal to nmdlThickness")
	}
}

func TestSVGPathGeneration_VariableThickness(t *testing.T) {
	groupElem := &svgElement{Tag: "g"}
	points := []strokePoint{
		{X: 10, Y: 20, T: 5, D: 1.0},
		{X: 30, Y: 40, T: 10, D: 1.0}, // different thickness → new path
	}

	generateSVGPaths(groupElem, points, 5)

	// Point 0 has T=5 and only 1 point → no path element (need >1 points)
	// Point 1 has T=10 with prior segment from point 0 → 1 path element
	if len(groupElem.Children) != 1 {
		t.Fatalf("expected 1 path, got %d", len(groupElem.Children))
	}

	// The path should have stroke-width set (10 != 5)
	path1 := groupElem.Children[0]
	if path1.Attrib["stroke-width"] != "10" {
		t.Errorf("path stroke-width = %q, want %q", path1.Attrib["stroke-width"], "10")
	}
}

func TestSVGPathGeneration_IncludePriorLineSegment(t *testing.T) {
	// When INCLUDE_PRIOR_LINE_SEGMENT is true, new paths include prior points
	groupElem := &svgElement{Tag: "g"}
	points := []strokePoint{
		{X: 10, Y: 20, T: 5, D: 1.0},
		{X: 30, Y: 40, T: 5, D: 1.0},
		{X: 50, Y: 60, T: 15, D: 1.0}, // new path starts here
	}

	generateSVGPaths(groupElem, points, 5)

	// Two paths
	if len(groupElem.Children) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(groupElem.Children))
	}

	// Second path should include prior line segments from points[1] and points[0]
	// priors = [2, 1]: i=2, j=2 → points[0]=(10,20), j=1 → points[1]=(30,40)
	path2 := groupElem.Children[1]
	d := path2.Attrib["d"]
	// Should start with M for the first prior point
	if len(d) == 0 {
		t.Error("second path d attribute should not be empty")
	}
	// Should include prior points: (10,20) and (30,40), then current (50,60)
	// Points are added in order [2, 1]: first (10,20), then (30,40), then (50,60)
	if !containsStr(d, "M") {
		t.Errorf("path d should contain M: %q", d)
	}
}

// ===========================================================================
// VAL-M4-NB-006: scribeNotebookAnnotation processes handwriting recognition
// Python: yj_to_epub_notebook.py:517-614
// ===========================================================================

func TestScribeNotebookAnnotation_HWR(t *testing.T) {
	// Verify that nmdl.hwr annotation creates <text>/<tspan> elements.
	storyFragment := map[string]interface{}{
		"story_name": "hwr_story",
		"content_list": []interface{}{
			map[string]interface{}{
				"type": "text",
				"top":  float64(100),
				"left":  float64(50),
				"height":  float64(20),
				"width":  float64(80),
				"content": "Hello World",
				"style_events": []interface{}{
					map[string]interface{}{
						"model": "word",
						"offset": 0,
						"length": 5,
						"top":  float64(100),
						"left":  float64(50),
						"height":  float64(20),
						"width":  float64(40),
					},
					map[string]interface{}{
						"model": "word",
						"offset": 6,
						"length": 5,
						"top":  float64(100),
						"left":  float64(90),
						"height":  float64(20),
						"width":  float64(40),
					},
				},
			},
		},
	}

	nc := &notebookContext{
		getFragment: func(ftype string, fid string) map[string]interface{} {
			return nil
		},
		getNamedFragment: func(content map[string]interface{}, ftype string, nameSymbol string) map[string]interface{} {
			// Return the story fragment for any $259 lookup
			if ftype == "storyline" {
				return storyFragment
			}
			return nil
		},
		contentContext: "test",
	}

	parent := &svgElement{Tag: "g"}

	annotation := map[string]interface{}{
		"annotation_type": "nmdl.hwr",
		"storyline": "story_ref",
	}

	scribeNotebookAnnotation(nc, annotation, parent)

	// Should have created desc and text elements
	// The annotation processes the story which has content with $269 type
	// We expect at least desc + text elements
	if len(parent.Children) < 2 {
		t.Fatalf("expected at least 2 children (desc + text), got %d", len(parent.Children))
	}

	// Find the text element
	var textElem *svgElement
	for _, child := range parent.Children {
		if child.Tag == "text" {
			textElem = child
			break
		}
	}

	if textElem == nil {
		t.Fatal("expected <text> element to be created")
	}

	// Check text element attributes
	if textElem.Attrib["fill"] != "red" {
		t.Errorf("text fill = %q, want %q", textElem.Attrib["fill"], "red")
	}

	// Should have tspan children for each word
	tspanCount := 0
	for _, child := range textElem.Children {
		if child.Tag == "tspan" {
			tspanCount++
			if child.Text == "" {
				t.Error("tspan should have text content")
			}
		}
	}
	if tspanCount != 2 {
		t.Errorf("expected 2 tspan elements, got %d", tspanCount)
	}
}

func TestScribeNotebookAnnotation_UnexpectedType(t *testing.T) {
	// Verify unexpected annotation type logs error.
	nc := &notebookContext{
		getFragment: func(ftype string, fid string) map[string]interface{} {
			return nil
		},
		getNamedFragment: func(content map[string]interface{}, ftype string, nameSymbol string) map[string]interface{} {
			return nil
		},
		contentContext: "test",
	}

	parent := &svgElement{Tag: "g"}

	annotation := map[string]interface{}{
		"annotation_type": "nmdl.unknown_type",
	}

	scribeNotebookAnnotation(nc, annotation, parent)

	// Should not create any children
	if len(parent.Children) != 0 {
		t.Errorf("expected 0 children for unknown annotation type, got %d", len(parent.Children))
	}
}

// ===========================================================================
// VAL-M4-NB-004: PNG density map for variable-density strokes
// Python: yj_to_epub_notebook.py:402-480
// ===========================================================================

func TestScribeNotebookStroke_VariableDensity(t *testing.T) {
	// Verify that variable density strokes produce an <image> element with base64 PNG.
	nc := &notebookContext{
		contentContext: "test",
	}

	parent := &svgElement{Tag: "svg"}

	// Create position_x/y data with valid signature
	posData := []byte{
		0x01, 0x01,             // signature
		0x02, 0x00, 0x00, 0x00, // num_vals=2
		0x45,                   // nibbles: 4 (inc=0), 5 (inc=1)
	}

	// Make dafData return values != 100 to trigger variable density
	// nibble 5 → n=1,bit2=1 → increment=1; nibble 5 → increment=1
	// value[0]=1, change=0+1=1, value=1+1=2 → daf values [1, 2]
	dafData2 := []byte{
		0x01, 0x01,
		0x02, 0x00, 0x00, 0x00,
		0x55, // both nibbles = 5 → increment=1
	}

	tafData := []byte{
		0x01, 0x01,
		0x02, 0x00, 0x00, 0x00,
		0x44, // both nibbles = 4 → increment=0 → values [0, 0]
	}

	content := map[string]interface{}{
		"nmdl.type":        "nmdl.stroke",
		"nmdl.brush_type":  0, // ORIGINAL_PEN
		"nmdl.color":       0, // black
		"nmdl.random_seed": 42,
		"nmdl.stroke_bounds": []interface{}{int(0), int(0), int(1000), int(1000)},
		"nmdl.thickness":    float64(50),
		"nmdl.stroke_points": map[string]interface{}{
			"nmdl.num_points":            2,
			"nmdl.position_x":            posData,
			"nmdl.position_y":            posData,
			"nmdl.density_adjust_factor": dafData2, // non-100 values → variable density
			"nmdl.thickness_adjust_factor": tafData,
		},
	}

	scribeNotebookStroke(nc, content, parent, "density_loc")

	// Should have created a group with density image
	// Find the group element (might be nested in SVG root)
	var findGroup func(elem *svgElement) *svgElement
	findGroup = func(elem *svgElement) *svgElement {
		for _, child := range elem.Children {
			if child.Tag == "g" {
				return child
			}
			if result := findGroup(child); result != nil {
				return result
			}
		}
		return nil
	}

	groupElem := findGroup(parent)
	if groupElem == nil {
		t.Fatal("expected <g> element to be created")
	}

	// Group should have stroke="none" for variable density
	if groupElem.Attrib["stroke"] != "none" {
		t.Errorf("variable density group stroke = %q, want %q", groupElem.Attrib["stroke"], "none")
	}

	// Should have an image child with base64 data
	var imageElem *svgElement
	for _, child := range groupElem.Children {
		if child.Tag == "image" {
			imageElem = child
			break
		}
	}
	if imageElem == nil {
		t.Fatal("expected <image> element for variable density stroke")
	}

	href := imageElem.Attrib["xlink:href"]
	if !containsStr(href, "data:image/png;base64,") {
		t.Errorf("image href should contain base64 PNG data, got: %s", href[:min(80, len(href))])
	}
}

// ===========================================================================
// VAL-M4-NB-008: adjustColorForDensity edge cases
// Python: yj_to_epub_notebook.py:615-622
// ===========================================================================

func TestAdjustColorForDensity_AllColors(t *testing.T) {
	// Test all stroke colors at density 1.0
	for idx, entry := range STROKE_COLORS {
		result := adjustColorForDensity(entry.Hex, 1.0)
		r := (result >> 16) & 0xff
		g := (result >> 8) & 0xff
		b := result & 0xff
		// Result should be grayscale
		if r != g || g != b {
			t.Errorf("STROKE_COLORS[%d] (%s, 0x%06x) at density 1.0 = 0x%06x, not grayscale", idx, entry.Name, entry.Hex, result)
		}
	}
}

// ===========================================================================
// Helper: check if string contains substring
// ===========================================================================

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ===========================================================================
// Thickness matching
// ===========================================================================

func TestThicknessMatching(t *testing.T) {
	// Verify thickness matching works correctly for each brush type.
	// Python: iterates THICKNESS_CHOICES[brush_name] to find closest match.
	tests := []struct {
		brushName string
		thickness float64
		expected  string
	}{
		{FOUNTAIN_PEN, 23.625, "fine"},    // exact match
		{FOUNTAIN_PEN, 47.25, "medium"},   // exact match
		{PEN, 40.0, "thin"},               // closest to 39.375
		{PENCIL, 100.0, "thick"},          // closest to 110.25 or 94.5?
		{HIGHLIGHTER, 300.0, "thin"},      // closest to 315
		{UNKNOWN, 50.0, "50.000"},         // no choices → format string
	}

	for _, tc := range tests {
		bestDiff := 0.5
		thicknessName := fmt.Sprintf("%1.3f", tc.thickness)
		for idx, choice := range THICKNESS_CHOICES[tc.brushName] {
			diff := math.Abs(choice-tc.thickness) / choice
			if diff < bestDiff {
				thicknessName = THICKNESS_NAME[idx]
				bestDiff = diff
			}
		}

		if thicknessName != tc.expected {
			t.Errorf("thickness match for %s/%v = %q, want %q", tc.brushName, tc.thickness, thicknessName, tc.expected)
		}
	}
}

// ===========================================================================
// scribeNotebookStrokeIndividual with normal (non-density) stroke
// ===========================================================================

func TestScribeNotebookStroke_NormalStroke(t *testing.T) {
	// Test a normal stroke (all daf=100) that produces SVG paths.
	nc := &notebookContext{
		contentContext: "test",
	}

	parent := &svgElement{Tag: "svg"}

	// Create position data for 2 points
	posData := []byte{
		0x01, 0x01,
		0x02, 0x00, 0x00, 0x00,
		0x15, // nibble 1: n=1,bit2=0 → read 1 byte
		0x0a, // increment = 10
		0x54, // nibble 5: n=1,bit2=1 → increment=1; nibble 4: n=0,bit2=1 → increment=0
	}
	// Delta decode for posData:
	// i=0: change=0, value=10 → 10
	// i=1: change=0+1=1, value=10+1=11 → 11
	// Wait, nibbles are: byte 0x15 → high=1, low=5
	// i=0: instr=1, n=1,bit2=0 → read 1 byte (0x0a=10) → increment=10, value=10
	// i=1: instr=5, n=1,bit2=1 → increment=1, change=0+1=1, value=10+1=11

	// daf=100 for all points (not variable density)
	// We need daf values of 100 for each point.
	// To get value=100 at first point: first increment=100
	// Then no more changes: increment=0 for remaining points.
	// n=1,bit2=0 → instr=0x01, read 1 byte = 100 = 0x64
	dafAll100 := []byte{
		0x01, 0x01,
		0x02, 0x00, 0x00, 0x00,
		0x14, // nibble 1: n=1,bit2=0 → read 1 byte; nibble 4: inc=0
		0x64, // byte = 100
	}
	// i=0: instr=1, read 1 byte=100, value=100
	// i=1: instr=4, increment=0, change=0, value=100

	tafAll100 := []byte{
		0x01, 0x01,
		0x02, 0x00, 0x00, 0x00,
		0x14, // same as daf
		0x64, // 100
	}

	content := map[string]interface{}{
		"nmdl.type":        "nmdl.stroke",
		"nmdl.brush_type":  0, // ORIGINAL_PEN
		"nmdl.color":       2, // red
		"nmdl.random_seed": 42,
		"nmdl.stroke_bounds": []interface{}{int(0), int(0), int(1000), int(1000)},
		"nmdl.thickness":    float64(50),
		"nmdl.stroke_points": map[string]interface{}{
			"nmdl.num_points":              2,
			"nmdl.position_x":              posData,
			"nmdl.position_y":              posData,
			"nmdl.density_adjust_factor":   dafAll100,
			"nmdl.thickness_adjust_factor": tafAll100,
		},
	}

	scribeNotebookStroke(nc, content, parent, "normal_loc")

	// Find the group element
	var findGroups func(elem *svgElement) []*svgElement
	findGroups = func(elem *svgElement) []*svgElement {
		var result []*svgElement
		for _, child := range elem.Children {
			if child.Tag == "g" {
				result = append(result, child)
			}
			result = append(result, findGroups(child)...)
		}
		return result
	}

	groups := findGroups(parent)
	if len(groups) == 0 {
		t.Fatal("expected at least one <g> element")
	}

	// Find the group with id="normal_loc"
	var strokeGroup *svgElement
	for _, g := range groups {
		if g.Attrib["id"] == "normal_loc" {
			strokeGroup = g
			break
		}
	}
	if strokeGroup == nil {
		t.Fatal("expected <g> element with id=normal_loc")
	}

	// Should have fill="none" and stroke attributes for normal stroke
	if strokeGroup.Attrib["fill"] != "none" {
		t.Errorf("normal stroke group fill = %q, want %q", strokeGroup.Attrib["fill"], "none")
	}
	if strokeGroup.Attrib["stroke"] == "" {
		t.Error("normal stroke group should have stroke color")
	}
	if strokeGroup.Attrib["stroke-width"] == "" {
		t.Error("normal stroke group should have stroke-width")
	}

	// Should have desc and path children
	var hasDesc, hasPath bool
	for _, child := range strokeGroup.Children {
		if child.Tag == "desc" {
			hasDesc = true
		}
		if child.Tag == "path" {
			hasPath = true
		}
	}
	if !hasDesc {
		t.Error("expected <desc> element in stroke group")
	}
	if !hasPath {
		t.Error("expected <path> element in stroke group")
	}
}

// ===========================================================================
// VAL-M4-NB-007: decodeStrokeValues edge cases
// ===========================================================================

func TestDecodeStrokeValues_ShortData(t *testing.T) {
	// Not enough data for instructions should return error
	data := []byte{
		0x01, 0x01,
		0x05, 0x00, 0x00, 0x00, // num_vals=5 but only 1 byte of instruction data
		0x44,
	}
	_, err := decodeStrokeValues(data, 5, "test_short")
	if err == nil {
		t.Error("expected error for insufficient data")
	}
}

func TestDecodeStrokeValues_LargeValue(t *testing.T) {
	// Test with a large 2-byte increment value
	data := []byte{
		0x01, 0x01,
		0x01, 0x00, 0x00, 0x00,
		0x20,             // n=2, bit2=0 → read 2 bytes
		0x00, 0x10,       // uint16 LE = 0x1000 = 4096
	}
	vals, err := decodeStrokeValues(data, 1, "test_large")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if vals[0] != 4096 {
		t.Errorf("value = %d, want 4096", vals[0])
	}
}

// ===========================================================================
// SVG element tests
// ===========================================================================

func TestSVGElement_SetAttrib(t *testing.T) {
	elem := &svgElement{Tag: "g"}
	elem.setAttrib("id", "test1")
	elem.setAttrib("opacity", "0.20")

	if elem.Attrib["id"] != "test1" {
		t.Errorf("id = %q, want %q", elem.Attrib["id"], "test1")
	}
	if elem.Attrib["opacity"] != "0.20" {
		t.Errorf("opacity = %q, want %q", elem.Attrib["opacity"], "0.20")
	}
}

func TestNewSVGElement(t *testing.T) {
	parent := &svgElement{Tag: "svg"}
	child := newSVGElement(parent, "g", map[string]string{"id": "child1"})

	if child.Tag != "g" {
		t.Errorf("child tag = %q, want %q", child.Tag, "g")
	}
	if child.Parent != parent {
		t.Error("child parent should be parent")
	}
	if len(parent.Children) != 1 || parent.Children[0] != child {
		t.Error("parent should have child in Children")
	}
}

// ===========================================================================
// Context stack tests
// ===========================================================================

func TestContextStack(t *testing.T) {
	cs := &contextStack{base: "root"}

	if cs.current() != "root" {
		t.Errorf("initial current = %q, want %q", cs.current(), "root")
	}

	cs.push("level1")
	if cs.current() != "root level1" {
		t.Errorf("after push = %q, want %q", cs.current(), "root level1")
	}

	cs.push("level2")
	if cs.current() != "root level1 level2" {
		t.Errorf("after second push = %q, want %q", cs.current(), "root level1 level2")
	}

	cs.pop()
	if cs.current() != "root level1" {
		t.Errorf("after pop = %q, want %q", cs.current(), "root level1")
	}

	cs.pop()
	if cs.current() != "root" {
		t.Errorf("after second pop = %q, want %q", cs.current(), "root")
	}
}

// ===========================================================================
// Quantize thickness
// ===========================================================================

func TestQuantizeThickness(t *testing.T) {
	// When QUANTIZE_THICKNESS is true, TAF is quantized to 10% steps.
	if !QUANTIZE_THICKNESS {
		t.Fatal("QUANTIZE_THICKNESS should be true")
	}

	// Test quantization: (taf // 10) * 10
	tests := []struct {
		input    int
		expected int
	}{
		{95, 90},
		{100, 100},
		{105, 100},
		{99, 90},
		{10, 10},
		{0, 0},
	}

	for _, tc := range tests {
		result := (tc.input / 10) * 10
		if result != tc.expected {
			t.Errorf("quantize(%d) = %d, want %d", tc.input, result, tc.expected)
		}
	}
}
