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
