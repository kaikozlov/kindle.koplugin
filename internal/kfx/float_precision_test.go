package kfx

import (
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// TestFormatCSSQuantity — port of Python value_str from epub_output.py:1373-1393
// ---------------------------------------------------------------------------

func TestFormatCSSQuantityZero(t *testing.T) {
	// Rule 1: value 0 → "0"
	got := formatCSSQuantity(0)
	want := "0"
	if got != want {
		t.Errorf("formatCSSQuantity(0) = %q, want %q", got, want)
	}
}

func TestFormatCSSQuantityNearZero(t *testing.T) {
	// Rule 2: near-zero (< 1e-6) → "0"
	cases := []struct {
		name  string
		value float64
	}{
		{"positive_near_zero", 1e-7},
		{"negative_near_zero", -1e-7},
		{"tiny_positive", 1e-10},
		{"tiny_negative", -1e-10},
		{"negative_abs_near_zero", -5e-7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatCSSQuantity(tc.value)
			if got != "0" {
				t.Errorf("formatCSSQuantity(%v) = %q, want %q", tc.value, got, "0")
			}
		})
	}
}

func TestFormatCSSQuantityPositiveIntegers(t *testing.T) {
	// Rule 3: positive integer values
	cases := []struct {
		value float64
		want  string
	}{
		{1, "1"},
		{10, "10"},
		{100, "100"},
		{3600, "3600"},
	}
	for _, tc := range cases {
		got := formatCSSQuantity(tc.value)
		if got != tc.want {
			t.Errorf("formatCSSQuantity(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestFormatCSSQuantityNegativeValues(t *testing.T) {
	// Rule 6: negative values
	cases := []struct {
		value float64
		want  string
	}{
		{-1, "-1"},
		{-10, "-10"},
		{-100, "-100"},
		{-0.5, "-0.5"},
	}
	for _, tc := range cases {
		got := formatCSSQuantity(tc.value)
		if got != tc.want {
			t.Errorf("formatCSSQuantity(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestFormatCSSQuantityFloats(t *testing.T) {
	// Rule 3: format with %g default
	cases := []struct {
		value float64
		want  string
	}{
		{1.5, "1.5"},
		{0.5, "0.5"},
		{12.375, "12.375"},
		{3.14159, "3.14159"},
	}
	for _, tc := range cases {
		got := formatCSSQuantity(tc.value)
		if got != tc.want {
			t.Errorf("formatCSSQuantity(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestFormatCSSQuantityScientificNotationFallback(t *testing.T) {
	// Rule 4: scientific notation triggers reformat with %.4f
	// Python: if "e" in q_str.lower() → q_str = "%.4f" % quantity
	//
	// In Python, %g uses scientific notation when exponent < -4.
	// Values in range [1e-6, 1e-4) produce sci notation in %g.
	// After %.4f reformat, these values have < 0.0001 magnitude,
	// which rounds to "0.0000" and strips to "0".
	cases := []struct {
		name  string
		value float64
		want  string
	}{
		// 0.000005 > 1e-6 (not caught by near-zero), but %g → "5e-06" → %.4f → "0.0000" → strip → "0"
		{"small_sci_pos", 0.000005, "0"},
		// 0.00001234 → %g → "1.234e-05" → %.4f → "0.0000" → strip → "0"
		{"small_sci_value", 0.00001234, "0"},
		// Negative: same behavior
		{"small_neg_sci", -0.00001234, "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatCSSQuantity(tc.value)
			if got != tc.want {
				t.Errorf("formatCSSQuantity(%v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestFormatCSSQuantityTrailingZeros(t *testing.T) {
	// Rule 5: strip trailing zeros after decimal then strip trailing dot
	cases := []struct {
		value float64
		want  string
	}{
		{1.50, "1.5"},   // trailing zero stripped
		{2.0, "2"},      // trailing dot stripped → "2"
		{100.0, "100"},  // trailing dot stripped → "100"
		{1.250, "1.25"}, // trailing zero stripped
	}
	for _, tc := range cases {
		got := formatCSSQuantity(tc.value)
		if got != tc.want {
			t.Errorf("formatCSSQuantity(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestFormatCSSQuantityLargeValues(t *testing.T) {
	// Large values should not produce scientific notation
	cases := []struct {
		value float64
		want  string
	}{
		{1000000, "1000000"},
		{999999, "999999"},
	}
	for _, tc := range cases {
		got := formatCSSQuantity(tc.value)
		if got != tc.want {
			t.Errorf("formatCSSQuantity(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TestValueStrWithUnit — port of Python value_str(unit, emit_zero_unit)
// ---------------------------------------------------------------------------

func TestValueStrWithUnitNone(t *testing.T) {
	// Python: if quantity is None → return unit
	got := valueStrWithUnit(nil, "px", false)
	want := "px"
	if got != want {
		t.Errorf("valueStrWithUnit(nil, px, false) = %q, want %q", got, want)
	}
}

func TestValueStrWithUnitZeroNoUnit(t *testing.T) {
	// Python: q_str == "0" and not emit_zero_unit → return "0"
	got := valueStrWithUnit(0.0, "px", false)
	want := "0"
	if got != want {
		t.Errorf("valueStrWithUnit(0, px, false) = %q, want %q", got, want)
	}
}

func TestValueStrWithUnitZeroWithUnit(t *testing.T) {
	// Python: q_str == "0" and emit_zero_unit → return "0px"
	got := valueStrWithUnit(0.0, "px", true)
	want := "0px"
	if got != want {
		t.Errorf("valueStrWithUnit(0, px, true) = %q, want %q", got, want)
	}
}

func TestValueStrWithUnitNormal(t *testing.T) {
	// Normal float with unit
	got := valueStrWithUnit(12.5, "em", false)
	want := "12.5em"
	if got != want {
		t.Errorf("valueStrWithUnit(12.5, em, false) = %q, want %q", got, want)
	}
}

func TestValueStrWithUnitNegative(t *testing.T) {
	got := valueStrWithUnit(-3.5, "px", false)
	want := "-3.5px"
	if got != want {
		t.Errorf("valueStrWithUnit(-3.5, px, false) = %q, want %q", got, want)
	}
}

func TestValueStrWithUnitNearZero(t *testing.T) {
	// Near-zero without emit_zero_unit → "0" (no unit)
	got := valueStrWithUnit(1e-7, "px", false)
	want := "0"
	if got != want {
		t.Errorf("valueStrWithUnit(1e-7, px, false) = %q, want %q", got, want)
	}
}

func TestValueStrWithUnitNearZeroWithUnit(t *testing.T) {
	// Near-zero with emit_zero_unit → "0px"
	got := valueStrWithUnit(1e-7, "px", true)
	want := "0px"
	if got != want {
		t.Errorf("valueStrWithUnit(1e-7, px, true) = %q, want %q", got, want)
	}
}

func TestValueStrWithUnitInteger(t *testing.T) {
	// integer input → "42px"
	got := valueStrWithUnit(42, "px", false)
	want := "42px"
	if got != want {
		t.Errorf("valueStrWithUnit(42, px, false) = %q, want %q", got, want)
	}
}

func TestValueStrWithUnitNoUnit(t *testing.T) {
	// quantity with empty unit → just the number string
	got := valueStrWithUnit(5.5, "", false)
	want := "5.5"
	if got != want {
		t.Errorf("valueStrWithUnit(5.5, empty, false) = %q, want %q", got, want)
	}
}

func TestValueStrWithUnitNilNoUnit(t *testing.T) {
	// None quantity with empty unit → ""
	got := valueStrWithUnit(nil, "", false)
	want := ""
	if got != want {
		t.Errorf("valueStrWithUnit(nil, empty, false) = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// TestColorStr — port of Python color_str from yj_to_epub_properties.py:2121-2134
// ---------------------------------------------------------------------------

func TestColorStrOpaqueBlack(t *testing.T) {
	// alpha=1.0, rgb=0x000000 → COLOR_NAME["#000000"] = "black"
	got := colorStr(0xFF000000, 1.0)
	want := "black"
	if got != want {
		t.Errorf("colorStr(0xFF000000, 1.0) = %q, want %q", got, want)
	}
}

func TestColorStrOpaqueWhite(t *testing.T) {
	// alpha=1.0, rgb=0xffffff → COLOR_NAME["#ffffff"] = "white"
	got := colorStr(0xFFFFFFFF, 1.0)
	want := "white"
	if got != want {
		t.Errorf("colorStr(0xFFFFFFFF, 1.0) = %q, want %q", got, want)
	}
}

func TestColorStrOpaqueNamedColor(t *testing.T) {
	// alpha=1.0, rgb=0x000080 → "navy" (named color)
	got := colorStr(0xFF000080, 1.0)
	want := "navy"
	if got != want {
		t.Errorf("colorStr(0xFF000080, 1.0) = %q, want %q", got, want)
	}
}

func TestColorStrOpaqueUnnamedColor(t *testing.T) {
	// alpha=1.0, rgb=0x123456 → "#123456" (6-char hex)
	got := colorStr(0xFF123456, 1.0)
	want := "#123456"
	if got != want {
		t.Errorf("colorStr(0xFF123456, 1.0) = %q, want %q", got, want)
	}
}

func TestColorStrTranslucent(t *testing.T) {
	// alpha < 1.0 → rgba(r,g,b,%0.3f)
	// Python: alpha=0.5 → "%0.3f" % 0.5 = "0.500"
	// output: "rgba(128,64,32,0.500)"
	got := colorStr(0xFF804020, 0.5)
	want := "rgba(128,64,32,0.500)"
	if got != want {
		t.Errorf("colorStr(0xFF804020, 0.5) = %q, want %q", got, want)
	}
}

func TestColorStrAlphaZero(t *testing.T) {
	// alpha=0.0 → "rgba(r,g,b,0)"
	got := colorStr(0xFF804020, 0.0)
	want := "rgba(128,64,32,0)"
	if got != want {
		t.Errorf("colorStr(0xFF804020, 0.0) = %q, want %q", got, want)
	}
}

func TestColorStrAlphaSmallValue(t *testing.T) {
	// alpha=0.123 → "%0.3f" % 0.123 = "0.123"
	// output: "rgba(0,0,0,0.123)"
	got := colorStr(0xFF000000, 0.123)
	want := "rgba(0,0,0,0.123)"
	if got != want {
		t.Errorf("colorStr(0xFF000000, 0.123) = %q, want %q", got, want)
	}
}

func TestColorStrOpaqueRed(t *testing.T) {
	// alpha=1.0, rgb=0xff0000 → "red" (named)
	got := colorStr(0xFFFF0000, 1.0)
	want := "red"
	if got != want {
		t.Errorf("colorStr(0xFFFF0000, 1.0) = %q, want %q", got, want)
	}
}

func TestColorStrOpaqueGreen(t *testing.T) {
	// alpha=1.0, rgb=0x008000 → "green" (named)
	got := colorStr(0xFF008000, 1.0)
	want := "green"
	if got != want {
		t.Errorf("colorStr(0xFF008000, 1.0) = %q, want %q", got, want)
	}
}

func TestColorStrOpaqueGray(t *testing.T) {
	// alpha=1.0, rgb=0x808080 → "gray" (named)
	got := colorStr(0xFF808080, 1.0)
	want := "gray"
	if got != want {
		t.Errorf("colorStr(0xFF808080, 1.0) = %q, want %q", got, want)
	}
}

func TestColorStrOpaqueLime(t *testing.T) {
	// alpha=1.0, rgb=0x00ff00 → "lime" (named)
	got := colorStr(0xFF00FF00, 1.0)
	want := "lime"
	if got != want {
		t.Errorf("colorStr(0xFF00FF00, 1.0) = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// TestFixColorValue — ensure fixColorValue produces correct CSS color strings
// ---------------------------------------------------------------------------

func TestFixColorValueOpaqueBlack(t *testing.T) {
	got := fixColorValue(float64(0xFF000000))
	want := "black"
	if got != want {
		t.Errorf("fixColorValue(0xFF000000) = %q, want %q", got, want)
	}
}

func TestFixColorValueOpaqueWhite(t *testing.T) {
	got := fixColorValue(float64(0xFFFFFFFF))
	want := "white"
	if got != want {
		t.Errorf("fixColorValue(0xFFFFFFFF) = %q, want %q", got, want)
	}
}

func TestFixColorValueTransparent(t *testing.T) {
	// alpha byte < 2 → alpha = 0.0 → rgba(r,g,b,0)
	got := fixColorValue(float64(0x00804020))
	want := "rgba(128,64,32,0)"
	if got != want {
		t.Errorf("fixColorValue(0x00804020) = %q, want %q", got, want)
	}
}

func TestFixColorValueSemiTransparent(t *testing.T) {
	// alpha byte = 127 → int_to_alpha(127) = (127+1)/256 = 0.5
	// color_str with alpha=0.5 → rgba(128,64,32,0.500) (Python %0.3f format)
	got := fixColorValue(float64(0x7F804020))
	want := "rgba(128,64,32,0.500)"
	if got != want {
		t.Errorf("fixColorValue(0x7F804020) = %q, want %q", got, want)
	}
}

func TestFixColorValueOpaqueNamedRed(t *testing.T) {
	got := fixColorValue(float64(0xFFFF0000))
	want := "red"
	if got != want {
		t.Errorf("fixColorValue(0xFFFF0000) = %q, want %q", got, want)
	}
}

func TestFixColorValueOpaqueUnnamed(t *testing.T) {
	got := fixColorValue(float64(0xFF123456))
	want := "#123456"
	if got != want {
		t.Errorf("fixColorValue(0xFF123456) = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// TestIntToAlpha / TestAlphaToInt — matching Python int_to_alpha/alpha_to_int
// ---------------------------------------------------------------------------

func TestIntToAlpha(t *testing.T) {
	cases := []struct {
		input int
		want  float64
	}{
		{0, 0.0},
		{1, 0.0},
		{2, math.Max(math.Min(float64(3)/256.0, 1.0), 0.0)},
		{127, 0.5},
		{253, math.Max(math.Min(float64(254)/256.0, 1.0), 0.0)},
		{254, 1.0},
		{255, 1.0},
	}
	for _, tc := range cases {
		got := intToAlpha(tc.input)
		if math.Abs(got-tc.want) > 1e-10 {
			t.Errorf("intToAlpha(%d) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestAlphaToInt(t *testing.T) {
	cases := []struct {
		input float64
		want  int
	}{
		{0.0, 0},
		{0.011, 0}, // < 0.012 → 0
		// 0.012: int(0.012*256.0+0.5)=int(3.572)=3, max(min(3-1,255),0)=2
		{0.012, 2},
		{1.0, 255},
		{0.997, 255}, // > 0.996 → 255
	}
	for _, tc := range cases {
		got := alphaToInt(tc.input)
		if got != tc.want {
			t.Errorf("alphaToInt(%v) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestAlphaRoundTrip(t *testing.T) {
	// Test that alpha values round-trip through alphaToInt → intToAlpha
	for _, alpha := range []float64{0.1, 0.25, 0.5, 0.75, 0.9} {
		ai := alphaToInt(alpha)
		a2 := intToAlpha(ai)
		if math.Abs(a2-alpha) > 0.01 {
			t.Errorf("alpha round-trip: %v → %d → %v (diff > 0.01)", alpha, ai, a2)
		}
	}
}

// ---------------------------------------------------------------------------
// TestFloatPrecisionNumstr — numstr parity (already has tests in yj_structure_test.go
// so this just verifies the function exists and works)
// ---------------------------------------------------------------------------

func TestFloatPrecisionNumstr(t *testing.T) {
	// Python: def numstr(x): return "%g" % x
	// Go: numstr is an unexported alias that calls Numstr
	cases := []struct {
		value float64
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{1.5, "1.5"},
	}
	for _, tc := range cases {
		got := numstr(tc.value)
		if got != tc.want {
			t.Errorf("numstr(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}
