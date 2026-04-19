package kfx

import (
	"fmt"
	"strconv"
	"strings"
)

func normalizeFontFamily(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "serif"
	}
	families := splitAndNormalizeFontFamilies(value)
	if len(families) == 0 {
		return "serif"
	}
	return families[0]
}

func containerStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssFontFamily(style["$11"]); value != "" {
		declarations = append(declarations, "font-family: "+value)
	}
	if value := cssLengthProperty(style["$16"], "$16"); value != "" && value != "1em" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := mapFontStyle(style["$12"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-style: "+value)
	}
	if value := mapFontWeight(style["$13"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-weight: "+value)
	}
	if value := mapFontVariant(style["$583"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-variant: "+value)
	}
	if value := cssLineHeight(style["$42"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["$49"], "$49"); value != "" {
		declarations = append(declarations, "margin-bottom: "+value)
	}
	if value := cssLengthProperty(style["$48"], "$48"); value != "" {
		declarations = append(declarations, "margin-left: "+value)
	}
	if value := cssLengthProperty(style["$50"], "$50"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := cssLengthProperty(style["$47"], "$47"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	}
	if value := cssLengthProperty(style["$52"], "$52"); value != "" {
		declarations = append(declarations, "padding-top: "+value)
	}
	if value := mapBoxAlign(style["$34"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	if value := cssLengthProperty(style["$36"], "$36"); value != "" {
		declarations = append(declarations, "text-indent: "+value)
	}
	if value := mapPageBreak(style["$135"]); value != "" {
		declarations = append(declarations, "page-break-inside: "+value)
	}
	if color := cssColor(style["$84"]); color != "" {
		declarations = append(declarations, "border-top-color: "+color)
	}
	if value := mapBorderStyle(style["$89"]); value != "" {
		declarations = append(declarations, "border-top-style: "+value)
	}
	if value := cssLengthProperty(style["$94"], "$94"); value != "" {
		declarations = append(declarations, "border-top-width: "+value)
	}
	if value := fillColor(style); value != "" {
		declarations = append(declarations, "background-color: "+value)
	}
	if value := mapTextTransform(style["$41"]); value != "" && value != "none" {
		declarations = append(declarations, "text-transform: "+value)
	}
	return declarations
}

func bodyStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssFontFamily(style["$11"]); value != "" {
		declarations = append(declarations, "font-family: "+value)
	}
	if value := mapFontStyle(style["$12"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-style: "+value)
	}
	if value := mapFontVariant(style["$583"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-variant: "+value)
	}
	if value := cssLengthProperty(style["$16"], "$16"); value != "" && value != "1em" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := cssLineHeight(style["$42"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["$49"], "$49"); value != "" {
		declarations = append(declarations, "margin-bottom: "+value)
	}
	if value := cssLengthProperty(style["$48"], "$48"); value != "" {
		declarations = append(declarations, "margin-left: "+value)
	}
	if value := cssLengthProperty(style["$50"], "$50"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := cssLengthProperty(style["$47"], "$47"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	}
	if value := mapBoxAlign(style["$580"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	} else if value := mapBoxAlign(style["$34"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	if value := cssLengthProperty(style["$36"], "$36"); value != "" {
		if value == "0" {
			goto skipBodyIndent
		}
		declarations = append(declarations, "text-indent: "+value)
	}
skipBodyIndent:
	if value := fillColor(style); value != "" {
		declarations = append(declarations, "background-color: "+value)
	}
	if value := mapTextTransform(style["$41"]); value != "" && value != "none" {
		declarations = append(declarations, "text-transform: "+value)
	}
	return declarations
}

func spanStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssLengthProperty(style["$16"], "$16"); value != "" && value != "1em" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := cssFontFamily(style["$11"]); value != "" {
		declarations = append(declarations, "font-family: "+value)
	}
	if value := mapFontStyle(style["$12"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-style: "+value)
	}
	if value := mapFontWeight(style["$13"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-weight: "+value)
	}
	if value := mapFontVariant(style["$583"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-variant: "+value)
	}
	if value := cssLineHeight(style["$42"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["$50"], "$50"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := mapTextTransform(style["$41"]); value != "" && value != "none" {
		declarations = append(declarations, "text-transform: "+value)
	}
	return declarations
}

func styleClassName(prefix string, styleID string) string {
	if strings.HasSuffix(prefix, "_") && strings.HasPrefix(styleID, "-") {
		return strings.TrimSuffix(prefix, "_") + styleID
	}
	return prefix + styleID
}

func tableStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssLengthProperty(style["$47"], "$47"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	}
	if value := cssLengthProperty(style["$48"], "$48"); value != "" {
		declarations = append(declarations, "margin-left: "+value)
	}
	align := mapBoxAlign(style["$580"])
	if value := cssLengthProperty(style["$50"], "$50"); value != "" && align != "left" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := cssLengthProperty(style["$65"], "$65"); value != "" {
		declarations = append(declarations, "max-width: "+value)
	}
	if align == "left" {
		declarations = append(declarations, "margin-right: auto")
	}
	if color := cssColor(style["$83"]); color != "" {
		declarations = append(declarations, "border-color: "+color)
	}
	return declarations
}

func tableColumnStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssLengthProperty(style["$56"], "$56"); value != "" {
		declarations = append(declarations, "width: "+value)
	}
	return declarations
}

func structuredContainerDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := mapFontWeight(style["$13"]); value != "" {
		declarations = append(declarations, "font-weight: "+value)
	}
	return declarations
}

func tableCellStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssLengthProperty(style["$50"], "$50"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := mapBoxAlign(style["$34"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	if value := mapTableVerticalAlign(style["$633"]); value != "" {
		declarations = append(declarations, "vertical-align: "+value)
	}
	return declarations
}

func headingStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := mapHyphens(style["$127"]); value != "" {
		declarations = append(declarations, "-webkit-hyphens: "+value)
	}
	if value := cssFontFamily(style["$11"]); value != "" {
		declarations = append(declarations, "font-family: "+value)
	}
	if value := mapFontStyle(style["$12"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-style: "+value)
	}
	if value := cssLengthProperty(style["$16"], "$16"); value != "" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := mapFontWeight(style["$13"]); value != "" {
		declarations = append(declarations, "font-weight: "+value)
	} else {
		declarations = append(declarations, "font-weight: normal")
	}
	if value := mapFontVariant(style["$583"]); value != "" {
		declarations = append(declarations, "font-variant: "+value)
	}
	if value := mapHyphens(style["$127"]); value != "" {
		declarations = append(declarations, "hyphens: "+value)
	}
	if value := cssLineHeight(style["$42"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["$49"], "$49"); value != "" {
		declarations = append(declarations, "margin-bottom: "+value)
	} else {
		declarations = append(declarations, "margin-bottom: 0")
	}
	if value := cssLengthProperty(style["$47"], "$47"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	} else {
		declarations = append(declarations, "margin-top: 0")
	}
	if value := mapPageBreak(style["$788"]); value != "" {
		declarations = append(declarations, "page-break-after: "+value)
	}
	if value := mapBoxAlign(style["$34"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	if value := cssLengthProperty(style["$36"], "$36"); value != "" {
		declarations = append(declarations, "text-indent: "+value)
	}
	if value := cssLengthProperty(style["$48"], "$48"); value != "" {
		declarations = append(declarations, "margin-left: "+value)
	}
	if value := cssLengthProperty(style["$50"], "$50"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := mapTextDecoration(style["$23"]); value != "" {
		declarations = append(declarations, "text-decoration: "+value)
	}
	if value := mapTextTransform(style["$41"]); value != "" {
		declarations = append(declarations, "text-transform: "+value)
	}
	return declarations
}

// defaultFontNames mirrors Python DEFAULT_FONT_NAMES (yj_to_epub_properties.py).
var defaultFontNames = map[string]bool{
	"default":                   true,
	"$amzn_fixup_default_font$": true,
}

func cssFontFamily(value interface{}) string {
	text, ok := asString(value)
	if !ok || text == "" {
		return ""
	}
	// Port of Python fix_and_quote_font_family_list: split by comma, fix each name via fixFontName.
	// Python's fixFontName resolves "default" through font_name_replacements, which maps
	// "default" → the book's actual default font family (from document metadata $538).
	// Go previously hardcoded "default,serif" → "FreeFontSerif,serif", which caused
	// font-family to appear in element classes where Python strips it.
	fixer := currentFontFixer
	if fixer == nil {
		fixer = newFontNameFixer()
	}
	return fixer.fixAndQuoteFontFamilyList(text)
}

func splitAndNormalizeFontFamilies(value string) []string {
	fixer := currentFontFixer
	if fixer == nil {
		fixer = newFontNameFixer()
	}
	return fixer.splitAndFixFontFamilyList(value)
}

func normalizeFontFamilyNameCase(name string) string {
	if name == "" {
		return ""
	}
	if strings.EqualFold(name, "serif") || strings.EqualFold(name, "sans-serif") || strings.EqualFold(name, "monospace") {
		return strings.ToLower(name)
	}
	return capitalizeFontName(name)
}

func quoteFontFamilies(families []string) []string {
	quoted := make([]string, 0, len(families))
	for _, family := range families {
		if family == "" {
			continue
		}
		quoted = append(quoted, quoteFontName(family))
	}
	return quoted
}

func cssLineHeight(value interface{}) string {
	magnitude, unit, ok := numericStyleValue(value)
	if !ok {
		return ""
	}
	switch unit {
	case "$310":
		return formatStyleNumber(magnitude * 1.2)
	case "$308", "$505":
		return formatStyleNumber(magnitude)
	default:
		return formatStyleNumber(magnitude)
	}
}

func cssLengthProperty(value interface{}, property string) string {
	magnitude, unit, ok := numericStyleValue(value)
	if !ok {
		return ""
	}
	if magnitude == 0 {
		return "0"
	}
	switch unit {
	case "$310":
		return formatStyleNumber(magnitude*1.2) + "em"
	case "$308", "$505":
		return formatStyleNumber(magnitude) + "em"
	case "$314":
		return formatStyleNumber(magnitude) + "%"
	case "$318":
		if magnitude > 0 && int(magnitude*1000)%225 == 0 {
			return formatStyleNumber(float64(int(magnitude*1000))/450.0) + "px"
		}
		return formatStyleNumber(magnitude) + "pt"
	case "$319":
		return formatStyleNumber(magnitude) + "px"
	default:
		if property == "$56" || property == "$57" || property == "$47" || property == "$49" || property == "$16" {
			return formatStyleNumber(magnitude)
		}
		return ""
	}
}

func numericStyleValue(value interface{}) (float64, string, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, "", true
	case *float64:
		if typed == nil {
			return 0, "", false
		}
		return *typed, "", true
	case float32:
		return float64(typed), "", true
	case *float32:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), "", true
	case int:
		return float64(typed), "", true
	case *int:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), "", true
	case int32:
		return float64(typed), "", true
	case *int32:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), "", true
	case int64:
		return float64(typed), "", true
	case *int64:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), "", true
	case uint32:
		return float64(typed), "", true
	case *uint32:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), "", true
	case uint64:
		return float64(typed), "", true
	case *uint64:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), "", true
	}
	rawMagnitude, okMagnitude := mapField(value, "$307")
	rawUnit, okUnit := mapField(value, "$306")
	if !okMagnitude || !okUnit {
		return 0, "", false
	}
	unit, _ := asString(rawUnit)
	switch typed := rawMagnitude.(type) {
	case float64:
		return typed, unit, true
	case *float64:
		if typed == nil {
			return 0, "", false
		}
		return *typed, unit, true
	case float32:
		return float64(typed), unit, true
	case *float32:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), unit, true
	case int:
		return float64(typed), unit, true
	case *int:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), unit, true
	case int32:
		return float64(typed), unit, true
	case *int32:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), unit, true
	case int64:
		return float64(typed), unit, true
	case *int64:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), unit, true
	case uint32:
		return float64(typed), unit, true
	case *uint32:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), unit, true
	case uint64:
		return float64(typed), unit, true
	case *uint64:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), unit, true
	default:
		if parsed, err := strconv.ParseFloat(fmt.Sprint(rawMagnitude), 64); err == nil {
			return parsed, unit, true
		}
		return 0, "", false
	}
}

func cssColor(value interface{}) string {
	colorInt, ok := colorIntValue(value)
	if !ok {
		return ""
	}
	a := byte(colorInt >> 24)
	r := byte(colorInt >> 16)
	g := byte(colorInt >> 8)
	b := byte(colorInt)
	if a == 255 {
		switch (uint32(r) << 16) | (uint32(g) << 8) | uint32(b) {
		case 0x808080:
			return "gray"
		case 0xffffff:
			return "#fff"
		case 0x000000:
			return "#000"
		default:
			return fmt.Sprintf("#%02x%02x%02x", r, g, b)
		}
	}
	return fmt.Sprintf("rgba(%d,%d,%d,%s)", r, g, b, trimFloat(float64(a)/255.0))
}

func fillColor(style map[string]interface{}) string {
	_, hasColor := style["$70"]
	_, hasOpacity := style["$72"]
	if !hasColor && !hasOpacity {
		return ""
	}
	color := cssColor(style["$70"])
	if color == "" {
		color = "#ffffff"
	}
	opacity, _, ok := numericStyleValue(style["$72"])
	if !ok {
		return color
	}
	return addColorOpacity(color, opacity)
}

func addColorOpacity(color string, opacity float64) string {
	if opacity >= 0.999 {
		return color
	}
	r, g, b, _, ok := parseColor(color)
	if !ok {
		return color
	}
	if opacity <= 0.001 {
		return fmt.Sprintf("rgba(%d,%d,%d,0)", r, g, b)
	}
	return fmt.Sprintf("rgba(%d,%d,%d,%s)", r, g, b, trimFloat(opacity))
}

func colorDeclarations(style map[string]interface{}, linkStyle map[string]interface{}) string {
	for _, source := range []map[string]interface{}{style, linkStyle} {
		if value := cssColor(source["$576"]); value != "" {
			return value
		}
		if value := cssColor(source["$577"]); value != "" {
			return value
		}
	}
	return ""
}

func parseColor(value string) (int, int, int, float64, bool) {
	if strings.HasPrefix(value, "#") && len(value) == 7 {
		r, err1 := strconv.ParseInt(value[1:3], 16, 0)
		g, err2 := strconv.ParseInt(value[3:5], 16, 0)
		b, err3 := strconv.ParseInt(value[5:7], 16, 0)
		if err1 == nil && err2 == nil && err3 == nil {
			return int(r), int(g), int(b), 1, true
		}
	}
	if strings.HasPrefix(value, "rgba(") && strings.HasSuffix(value, ")") {
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(value, "rgba("), ")"), ",")
		if len(parts) != 4 {
			return 0, 0, 0, 0, false
		}
		r, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		g, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		b, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
		a, err4 := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
		if err1 == nil && err2 == nil && err3 == nil && err4 == nil {
			return r, g, b, a, true
		}
	}
	return 0, 0, 0, 0, false
}

func colorIntValue(value interface{}) (uint32, bool) {
	switch typed := value.(type) {
	case float64:
		return uint32(typed), true
	case *float64:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case int:
		return uint32(typed), true
	case *int:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case int32:
		return uint32(typed), true
	case *int32:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case int64:
		return uint32(typed), true
	case *int64:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case uint32:
		return typed, true
	case *uint32:
		if typed == nil {
			return 0, false
		}
		return *typed, true
	case uint64:
		return uint32(typed), true
	case *uint64:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	}
	raw, ok := mapField(value, "$19")
	if !ok {
		return 0, false
	}
	switch typed := raw.(type) {
	case float64:
		return uint32(typed), true
	case *float64:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case int:
		return uint32(typed), true
	case int32:
		return uint32(typed), true
	case int64:
		return uint32(typed), true
	case uint32:
		return typed, true
	case uint64:
		return uint32(typed), true
	case *int:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case *int32:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case *int64:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case *uint32:
		if typed == nil {
			return 0, false
		}
		return *typed, true
	case *uint64:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	default:
		return 0, false
	}
}

func trimFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func formatStyleNumber(value float64) string {
	return strconv.FormatFloat(value, 'g', 6, 64)
}
