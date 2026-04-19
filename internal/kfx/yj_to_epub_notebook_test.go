package kfx

import (
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
