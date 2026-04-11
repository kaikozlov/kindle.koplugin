package kfx

import (
	"strings"
	"testing"
)

func TestYJPropertyInfoCoreKeys(t *testing.T) {
	// Every property used in the hardcoded *StyleDeclarations functions must be in yjPropertyInfo.
	coreKeys := []string{
		"$11",  // font-family
		"$16",  // font-size
		"$12",  // font-style
		"$13",  // font-weight
		"$583", // font-variant
		"$42",  // line-height
		"$47",  // margin-top
		"$49",  // margin-bottom
		"$48",  // margin-left
		"$50",  // margin-right
		"$34",  // text-align
		"$36",  // text-indent
		"$41",  // text-transform
		"$19",  // color
		"$21",  // background-color
		"$57",  // height
		"$56",  // width
		"$84",  // border-top-color
		"$89",  // border-top-style
		"$94",  // border-top-width
		"$173", // -kfx-style-name
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
		"$11":  "next-reads-shift light,palatino,serif",
		"$173": "s366",
		"$47":  map[string]interface{}{"$307": float64(1.04167), "$306": "$310"},
		"$580": "$320", // text-align: center
	}
	result := convertYJProperties(props)

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
		"$84": float64(4284703587),
		"$89": "$328",
		"$94": map[string]interface{}{"$307": float64(0.45), "$306": "$318"},
	}
	result := convertYJProperties(props)

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
		"$11":  "serif",
		"$173": "test-style",
		"$159": "$269",  // content type — NOT a property
		"$156": "$323",  // layout — NOT a property
	}
	result := processContentProperties(content)

	if _, ok := result["font-family"]; !ok {
		t.Error("expected font-family from $11")
	}
	if _, ok := result["-kfx-style-name"]; !ok {
		t.Error("expected -kfx-style-name from $173")
	}
	if _, ok := result["$159"]; ok {
		t.Error("$159 (content type) should not be in CSS properties")
	}
}

func TestConvertYJPropertiesNoFontFamily(t *testing.T) {
	// s36C style fragment — has border properties but no $11
	props := map[string]interface{}{
		"$173": "s36C",
		"$36":  map[string]interface{}{"$307": float64(0.0), "$306": "$314"},
		"$42":  map[string]interface{}{"$307": float64(0.96), "$306": "$310"},
		"$47":  map[string]interface{}{"$307": float64(1.04167), "$306": "$310"},
		"$48":  map[string]interface{}{"$307": float64(5.0), "$306": "$314"},
		"$50":  map[string]interface{}{"$307": float64(5.0), "$306": "$314"},
		"$52":  map[string]interface{}{"$307": float64(1.04167), "$306": "$310"},
		"$84":  float64(4284703587),
		"$89":  "$328",
		"$94":  map[string]interface{}{"$307": float64(0.45), "$306": "$318"},
	}
	result := convertYJProperties(props)

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
