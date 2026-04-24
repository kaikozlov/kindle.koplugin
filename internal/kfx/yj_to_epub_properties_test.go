package kfx

import (
	"strings"
	"testing"
)

func TestYJPropertyInfoCoreKeys(t *testing.T) {
	// Every property used in the hardcoded *StyleDeclarations functions must be in yjPropertyInfo.
	coreKeys := []string{
		"font_family",  // font-family
		"font_size",  // font-size
		"font_style",  // font-style
		"font_weight",  // font-weight
		"glyph_transform", // font-variant
		"line_height",  // line-height
		"margin_top",  // margin-top
		"margin_bottom",  // margin-bottom
		"margin_left",  // margin-left
		"margin_right",  // margin-right
		"text_alignment",  // text-align
		"text_indent",  // text-indent
		"text_transform",  // text-transform
		"text_color",  // color
		"text_background_color",  // background-color
		"height",  // height
		"width",  // width
		"border_color_top",  // border-top-color
		"border_style_top",  // border-top-style
		"border_weight_top",  // border-top-width
		"style_name", // -kfx-style-name
	}
	for _, key := range coreKeys {
		info, ok := yjPropertyInfo[key]
		if !ok {
			t.Errorf("missing yjPropertyInfo entry for %s", key)
			continue
		}
		if info.name == "" {
			t.Errorf("yjPropertyInfo[%s].name is empty", key)
		}
	}
}

func TestConvertYJPropertiesFontFamily(t *testing.T) {
	props := map[string]interface{}{
		"font_family":  "next-reads-shift light,palatino,serif",
		"style_name": "s366",
		"margin_top":  map[string]interface{}{"value": float64(1.04167), "unit": "lh"},
		"box_align": "center", // text-align: center
	}
	result, _ := convertYJProperties(props, nil)

	if result["font-family"] == "" {
		t.Error("expected font-family to be set")
	}
	if !strings.Contains(result["font-family"], "Palatino") {
		t.Errorf("expected font-family to contain Palatino, got %q", result["font-family"])
	}
	if result["margin-top"] == "" {
		t.Error("expected margin-top to be set")
	}
	// $580 is -kfx-box-align, not text-align
	if result["-kfx-box-align"] != "center" {
		t.Errorf("expected -kfx-box-align=center, got %q", result["-kfx-box-align"])
	}
}

func TestConvertYJPropertiesBorderTop(t *testing.T) {
	props := map[string]interface{}{
		"border_color_top": float64(4284703587),
		"border_style_top": "solid",
		"border_weight_top": map[string]interface{}{"value": float64(0.45), "unit": "pt"},
	}
	result, _ := convertYJProperties(props, nil)

	if result["border-top-color"] != "#636363" {
		t.Errorf("expected border-top-color=#636363, got %q", result["border-top-color"])
	}
	if result["border-top-style"] != "solid" {
		t.Errorf("expected border-top-style=solid, got %q", result["border-top-style"])
	}
	if result["border-top-width"] == "" {
		t.Error("expected border-top-width to be set")
	}
}

func TestProcessContentPropertiesExtractsKnownKeys(t *testing.T) {
	content := map[string]interface{}{
		"font_family":  "serif",
		"style_name": "test-style",
		"type": "text",  // content type — NOT a property
		"layout": "vertical",  // layout — NOT a property
	}
	result := processContentProperties(content, nil)

	if _, ok := result["font-family"]; !ok {
		t.Error("expected font-family from $11")
	}
	if _, ok := result["-kfx-style-name"]; !ok {
		t.Error("expected -kfx-style-name from $173")
	}
	if _, ok := result["type"]; ok {
		t.Error("$159 (content type) should not be in CSS properties")
	}
}

func TestConvertYJPropertiesPositionOebPageFootDropped(t *testing.T) {
	// Python (yj_to_epub_properties.py L1101-1102):
	//   if property == "position" and value in ["oeb-page-foot", "oeb-page-head"]:
	//       property = "display" if self.generate_epub2 and EMIT_OEB_PAGE_PROPS else None
	// Since Go generates EPUB3 (not EPUB2), the property should be dropped entirely —
	// it must NOT appear as "position" in the output declarations.
	props := map[string]interface{}{
		"position": "footer", // mapped to "oeb-page-foot" by yjPropertyInfo value map
	}
	result, _ := convertYJProperties(props, nil)

	if _, ok := result["position"]; ok {
		t.Error("position:oeb-page-foot should be dropped in EPUB3 mode, but 'position' key found in declarations")
	}
	if _, ok := result["display"]; ok {
		t.Error("position:oeb-page-foot should NOT be converted to display in EPUB3 mode, but 'display' key found in declarations")
	}
}

func TestConvertYJPropertiesPositionOebPageHeadDropped(t *testing.T) {
	// Same as above but for oeb-page-head (header).
	props := map[string]interface{}{
		"position": "header", // mapped to "oeb-page-head" by yjPropertyInfo value map
	}
	result, _ := convertYJProperties(props, nil)

	if _, ok := result["position"]; ok {
		t.Error("position:oeb-page-head should be dropped in EPUB3 mode, but 'position' key found in declarations")
	}
	if _, ok := result["display"]; ok {
		t.Error("position:oeb-page-head should NOT be converted to display in EPUB3 mode, but 'display' key found in declarations")
	}
}

func TestConvertYJPropertiesPositionRelativePreserved(t *testing.T) {
	// Ensure normal position values (like "relative") are NOT dropped.
	props := map[string]interface{}{
		"position": "relative",
	}
	result, _ := convertYJProperties(props, nil)

	if result["position"] != "relative" {
		t.Errorf("expected position=relative, got %q", result["position"])
	}
}

// ---------------------------------------------------------------------------
// Tests for vh/vw viewport unit cross-conversion
// (Python yj_to_epub_properties.py L1753-1785, VAL-FIX-013)
// ---------------------------------------------------------------------------

// TestConvertViewportUnits_DirectConversion converts vh on height directly to %.
// This is the simple case that already works.
func TestConvertViewportUnits_DirectConversion(t *testing.T) {
	sty := map[string]string{
		"height":           "50vh",
		"-amzn-page-align": "all",
	}
	convertViewportUnits(sty, nil, nil, "")
	if sty["height"] != "50%" {
		t.Errorf("expected height=50%%, got %q", sty["height"])
	}
}

// TestConvertViewportUnits_CrossConversionWidthVH converts width in vh units to height in %
// using image aspect ratio. Python L1756-1779: when name[0] != unit[1], the property
// name is swapped and the value is scaled by the image's aspect ratio.
func TestConvertViewportUnits_CrossConversionWidthVH(t *testing.T) {
	// width:50vh on a 200x100 image → height:(50*100/200)% = height:25%
	sty := map[string]string{
		"width":            "50vh",
		"-amzn-page-align": "all",
	}
	imgDims := map[string][2]int{
		"image.jpg": {200, 100}, // width=200, height=100
	}
	elem := &htmlElement{
		Tag:   "img",
		Attrs: map[string]string{"src": "image.jpg"},
	}
	convertViewportUnits(sty, elem, imgDims, "section.xhtml")

	// Python pops "width" and sets "height"
	if _, hasWidth := sty["width"]; hasWidth {
		t.Error("expected width to be removed after cross-conversion")
	}
	if sty["height"] != "25%" {
		t.Errorf("expected height=25%% after cross-conversion, got %q", sty["height"])
	}
}

// TestConvertViewportUnits_CrossConversionHeightVW converts height in vw units to width in %
// using image aspect ratio.
func TestConvertViewportUnits_CrossConversionHeightVW(t *testing.T) {
	// height:50vw on a 100x200 image → width:(50*100/200)% = width:25%
	sty := map[string]string{
		"height":           "50vw",
		"-amzn-page-align": "all",
	}
	imgDims := map[string][2]int{
		"image.jpg": {100, 200}, // width=100, height=200
	}
	elem := &htmlElement{
		Tag:   "img",
		Attrs: map[string]string{"src": "image.jpg"},
	}
	convertViewportUnits(sty, elem, imgDims, "section.xhtml")

	// Python pops "height" and sets "width"
	if _, hasHeight := sty["height"]; hasHeight {
		t.Error("expected height to be removed after cross-conversion")
	}
	if sty["width"] != "25%" {
		t.Errorf("expected width=25%% after cross-conversion, got %q", sty["width"])
	}
}

// TestConvertViewportUnits_CrossConversionSnapsTo100 snaps values between 99 and 101 to 100.
// Python L1774-1775: if quantity > 99.0 and quantity < 101.0: quantity = 100.0
func TestConvertViewportUnits_CrossConversionSnapsTo100(t *testing.T) {
	// width:100vh on a 100x100 image → height:(100*100/100)% = 100% (exact, should snap)
	sty := map[string]string{
		"width":            "100vh",
		"-amzn-page-align": "all",
	}
	imgDims := map[string][2]int{
		"image.jpg": {100, 100},
	}
	elem := &htmlElement{
		Tag:   "img",
		Attrs: map[string]string{"src": "image.jpg"},
	}
	convertViewportUnits(sty, elem, imgDims, "section.xhtml")

	if sty["height"] != "100%" {
		t.Errorf("expected height=100%% (snapped), got %q", sty["height"])
	}
}

// TestConvertViewportUnits_CrossConversionNotImage logs error for non-img elements.
// Python L1780: log.error("viewport-based units with wrong property on non-image")
func TestConvertViewportUnits_CrossConversionNotImage(t *testing.T) {
	sty := map[string]string{
		"width":            "50vh",
		"-amzn-page-align": "all",
	}
	elem := &htmlElement{
		Tag:   "div",
		Attrs: map[string]string{},
	}
	convertViewportUnits(sty, elem, nil, "section.xhtml")

	// Non-image with wrong-axis should NOT convert — just log error
	if sty["width"] != "50vh" {
		t.Errorf("expected width to remain 50vh for non-image, got %q", sty["width"])
	}
}

// TestConvertViewportUnits_CrossConversionBothDimsSpecified logs error when both
// height and width are already in sty. Python L1779: log.error("viewport-based
// units with wrong property: ...") when both dimensions are present.
func TestConvertViewportUnits_CrossConversionBothDimsSpecified(t *testing.T) {
	sty := map[string]string{
		"width":            "50vh",
		"height":           "75%",
		"-amzn-page-align": "all",
	}
	elem := &htmlElement{
		Tag:   "img",
		Attrs: map[string]string{"src": "image.jpg"},
	}
	imgDims := map[string][2]int{
		"image.jpg": {200, 100},
	}
	convertViewportUnits(sty, elem, imgDims, "section.xhtml")

	// Should NOT cross-convert when both height and width already present
	if sty["width"] != "50vh" {
		t.Errorf("expected width to remain 50vh when both dims present, got %q", sty["width"])
	}
}

// TestConvertViewportUnits_NoPageAlign skips conversion when page-align is "none".
// Python L1755: if page_align != "none" and name in ["height", "width"]
func TestConvertViewportUnits_NoPageAlign(t *testing.T) {
	sty := map[string]string{
		"height":           "50vh",
		"-amzn-page-align": "none",
	}
	convertViewportUnits(sty, nil, nil, "")
	if sty["height"] != "50vh" {
		t.Errorf("expected height unchanged when page-align=none, got %q", sty["height"])
	}
}

func TestConvertYJPropertiesNoFontFamily(t *testing.T) {
	// s36C style fragment — has border properties but no $11
	props := map[string]interface{}{
		"style_name": "s36C",
		"text_indent":  map[string]interface{}{"value": float64(0.0), "unit": "percent"},
		"line_height":  map[string]interface{}{"value": float64(0.96), "unit": "lh"},
		"margin_top":  map[string]interface{}{"value": float64(1.04167), "unit": "lh"},
		"margin_left":  map[string]interface{}{"value": float64(5.0), "unit": "percent"},
		"margin_right":  map[string]interface{}{"value": float64(5.0), "unit": "percent"},
		"padding_top":  map[string]interface{}{"value": float64(1.04167), "unit": "lh"},
		"border_color_top":  float64(4284703587),
		"border_style_top":  "solid",
		"border_weight_top":  map[string]interface{}{"value": float64(0.45), "unit": "pt"},
	}
	result, _ := convertYJProperties(props, nil)

	if _, ok := result["font-family"]; ok {
		t.Error("s36C should not have font-family")
	}
	if result["-kfx-style-name"] != "s36C" {
		t.Errorf("expected style name s36C, got %q", result["-kfx-style-name"])
	}
	if result["border-top-style"] != "solid" {
		t.Errorf("expected border-top-style=solid, got %q", result["border-top-style"])
	}
}
