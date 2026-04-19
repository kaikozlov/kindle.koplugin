package kfx

// Port of Calibre KFX Input: YJ_PROPERTY_INFO, property_value, convert_yj_properties
// from yj_to_epub_properties.py (~L84-1170).
//
// This is the data-driven property → CSS mapping that replaces the hardcoded
// *StyleDeclarations functions in kfx.go.  The Python pipeline converts every
// KFX property through YJ_PROPERTY_INFO → property_value → convert_yj_properties,
// producing a flat CSS declaration map.  The Go *StyleDeclarations helpers will be
// replaced by calls to ConvertYJProperties in a follow-up change.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// -----------------------------------------------------------------------
// Prop / YJ_PROPERTY_INFO  (Python: class Prop + YJ_PROPERTY_INFO dict)
// -----------------------------------------------------------------------

// propInfo mirrors Python's Prop(name, values=None).
type propInfo struct {
	name   string            // CSS property name (or -kfx- internal name)
	values map[string]string // optional symbol → CSS value map; nil means "pass through"
}

// yjPropertyInfo is the Go equivalent of Python's YJ_PROPERTY_INFO dict.
// Keys are KFX property IDs (e.g. "$11", "$47").  The table is a direct port
// of the Python data at yj_to_epub_properties.py L84-626.
var yjPropertyInfo = map[string]propInfo{
	"$479": {"background-image", nil},
	"$480": {"-kfx-background-positionx", nil},
	"$481": {"-kfx-background-positiony", nil},
	"$547": {"background-origin", map[string]string{"$378": "border-box", "$377": "content-box", "$379": "padding-box"}},
	"$484": {"background-repeat", map[string]string{"$487": "no-repeat", "$485": "repeat-x", "$486": "repeat-y"}},
	"$482": {"-kfx-background-sizex", nil},
	"$483": {"-kfx-background-sizey", nil},
	"$31":  {"-kfx-baseline-shift", nil},
	"$44":  {"-kfx-baseline-style", map[string]string{"$60": "bottom", "$320": "middle", "$350": "baseline", "$371": "sub", "$370": "super", "$449": "text-bottom", "$447": "text-top", "$58": "top"}},
	"$682": {"direction", map[string]string{"$376": "ltr", "$375": "rtl"}},
	"$674": {"unicode-bidi", map[string]string{"$675": "embed", "$676": "isolate", "$678": "isolate-override", "$350": "normal", "$677": "bidi-override", "$679": "plaintext"}},

	"$83": {"border-color", nil},
	"$86": {"border-bottom-color", nil},
	"$85": {"border-left-color", nil},
	"$87": {"border-right-color", nil},
	"$84": {"border-top-color", nil},

	"$461": {"border-bottom-left-radius", nil},
	"$462": {"border-bottom-right-radius", nil},
	"$459": {"border-top-left-radius", nil},
	"$460": {"border-top-right-radius", nil},

	"$457": {"-webkit-border-horizontal-spacing", nil},
	"$456": {"-webkit-border-vertical-spacing", nil},

	"$88": {"border-style", borderStyles},
	"$91": {"border-bottom-style", borderStyles},
	"$90": {"border-left-style", borderStyles},
	"$92": {"border-right-style", borderStyles},
	"$89": {"border-top-style", borderStyles},

	"$93": {"border-width", nil},
	"$96": {"border-bottom-width", nil},
	"$95": {"border-left-width", nil},
	"$97": {"border-right-width", nil},
	"$94": {"border-top-width", nil},

	"$60":  {"bottom", nil},
	"$580": {"-kfx-box-align", map[string]string{"$320": "center", "$59": "left", "$61": "right"}},
	"$133": {"page-break-after", map[string]string{"$352": "always", "$383": "auto", "$353": "avoid"}},
	"$134": {"page-break-before", map[string]string{"$352": "always", "$383": "auto", "$353": "avoid"}},
	"$135": {"page-break-inside", map[string]string{"$383": "auto", "$353": "avoid"}},
	"$708": {"-kfx-character-width", map[string]string{"$383": ""}}, // None in Python → empty
	"$476": {"overflow", map[string]string{"false": "visible", "true": "hidden"}},
	"$112": {"column-count", map[string]string{"$383": "auto"}},
	"$116": {"column-rule-color", nil},
	"$192": {"direction", map[string]string{"$376": "ltr", "$375": "rtl"}},
	"$99":  {"box-decoration-break", map[string]string{"false": "slice", "true": "clone"}},
	"$73":  {"background-clip", map[string]string{"$378": "border-box", "$377": "content-box", "$379": "padding-box"}},
	"$70":  {"-kfx-fill-color", nil},
	"$72":  {"-kfx-fill-opacity", nil},
	"$140": {"float", map[string]string{"$59": "left", "$61": "right", "$786": "snap-block"}},

	"$11":  {"font-family", nil},
	"$16":  {"font-size", nil},
	"$15":  {"font-stretch", map[string]string{"$365": "condensed", "$368": "expanded", "$350": "normal", "$366": "semi-condensed", "$367": "semi-expanded"}},
	"$12":  {"font-style", map[string]string{"$382": "italic", "$350": "normal", "$381": "oblique"}},
	"$13":  {"font-weight", map[string]string{"$361": "bold", "$363": "900", "$357": "300", "$359": "500", "$350": "normal", "$360": "600", "$355": "100", "$362": "800", "$356": "200"}},
	"$583": {"font-variant", map[string]string{"$349": "normal", "$369": "small-caps"}},
	"$57":  {"height", nil},
	"$458": {"empty-cells", map[string]string{"false": "show", "true": "hide"}},
	"$127": {"hyphens", map[string]string{"$383": "auto", "$384": "manual", "$349": "none"}},
	"$785": {"-kfx-keep-lines-together", nil},
	"$10":  {"-kfx-attrib-xml-lang", nil},
	"$761": {"-kfx-layout-hints", nil},
	"$59":  {"left", nil},
	"$32":  {"letter-spacing", nil},
	"$780": {"line-break", map[string]string{"$783": "anywhere", "$383": "auto", "$781": "loose", "$350": "normal", "$782": "strict"}},
	"$42":  {"line-height", map[string]string{"$383": "normal"}},
	"$577": {"-kfx-link-color", nil},
	"$576": {"-kfx-visited-color", nil},

	"$100": {"list-style-type", map[string]string{
		"$346": "lower-alpha", "$347": "upper-alpha", "$342": "circle",
		"$737": "cjk-earthly-branch", "$738": "cjk-heavenly-stem", "$736": "cjk-ideographic",
		"$796": "decimal-leading-zero", "$340": "disc", "$795": "georgian",
		"$739": "hiragana", "$740": "hiragana-iroha", "$271": "",
		"$743": "japanese-formal", "$744": "japanese-informal",
		"$741": "katakana", "$742": "katakana-iroha",
		"$793": "lower-armenian", "$791": "lower-greek", "$349": "none",
		"$343": "decimal", "$344": "lower-roman", "$345": "upper-roman",
		"$746": "simp-chinese-formal", "$745": "simp-chinese-informal",
		"$341": "square", "$748": "trad-chinese-formal", "$747": "trad-chinese-informal",
		"$794": "upper-armenian", "$792": "upper-greek"}},
	"$503": {"list-style-image", nil},
	"$551": {"list-style-position", map[string]string{"$552": "inside", "$553": "outside"}},

	"$46": {"margin", nil},
	"$49": {"margin-bottom", nil},
	"$48": {"margin-left", nil},
	"$50": {"margin-right", nil},
	"$47": {"margin-top", nil},

	"$64": {"max-height", nil},
	"$65": {"max-width", nil},
	"$62": {"min-height", nil},
	"$63": {"min-width", nil},

	"$45": {"white-space", map[string]string{"false": "normal", "true": "nowrap"}},
	"$105": {"outline-color", nil},
	"$106": {"outline-offset", nil},
	"$107": {"outline-style", borderStyles},
	"$108": {"outline-width", nil},

	"$554": {"text-decoration", map[string]string{"$330": "overline dashed", "$331": "overline dotted", "$329": "overline double", "$349": "", "$328": "overline"}},
	"$555": {"text-decoration-color", nil},

	"$51": {"padding", nil},
	"$54": {"padding-bottom", nil},
	"$53": {"padding-left", nil},
	"$55": {"padding-right", nil},
	"$52": {"padding-top", nil},

	"$183": {"position", map[string]string{"$324": "absolute", "$455": "oeb-page-foot", "$151": "oeb-page-head", "$488": "relative", "$489": "fixed"}},
	"$61":  {"right", nil},

	"$766": {"ruby-align", map[string]string{"$320": "center", "$773": "space-around", "$774": "space-between", "$680": "start"}},
	"$764": {"ruby-merge", map[string]string{"$772": "collapse", "$771": "separate"}},
	"$762": {"ruby-position", map[string]string{"$60": "under", "$58": "over"}},
	"$763": {"ruby-position", map[string]string{"$59": "under", "$61": "over"}},
	"$765": {"ruby-align", map[string]string{"$320": "center", "$773": "space-around", "$774": "space-between", "$680": "start"}},

	"$496": {"box-shadow", nil},
	"$546": {"box-sizing", map[string]string{"$378": "border-box", "$377": "content-box", "$379": "padding-box"}},
	"src":  {"src", nil},

	"$27": {"text-decoration", map[string]string{"$330": "line-through dashed", "$331": "line-through dotted", "$329": "line-through double", "$349": "", "$328": "line-through"}},
	"$28": {"text-decoration-color", nil},

	"$75": {"-webkit-text-stroke-color", nil},

	"$531": {"-svg-stroke-dasharray", nil},
	"$532": {"-svg-stroke-dashoffset", nil},
	"$77":  {"-svg-stroke-linecap", map[string]string{"$534": "butt", "$533": "round", "$341": "square"}},
	"$529": {"-svg-stroke-linejoin", map[string]string{"$536": "bevel", "$535": "miter", "$533": "round"}},
	"$530": {"-svg-stroke-miterlimit", nil},
	"$76":  {"-webkit-text-stroke-width", nil},

	"$173": {"-kfx-style-name", nil},

	"$150": {"border-collapse", map[string]string{"false": "separate", "true": "collapse"}},
	"$148": {"-kfx-attrib-colspan", nil},
	"$149": {"-kfx-attrib-rowspan", nil},

	"$34": {"text-align", map[string]string{"$320": "center", "$321": "justify", "$59": "left", "$61": "right"}},
	"$35": {"text-align-last", map[string]string{"$383": "auto", "$320": "center", "$681": "end", "$321": "justify", "$59": "left", "$61": "right", "$680": "start"}},

	"$21":  {"background-color", nil},
	"$528": {"background-image", nil},
	"$19":  {"color", nil},

	"$707": {"text-combine-upright", map[string]string{"$573": "all"}},
	"$718": {"text-emphasis-color", nil},
	"$719": {"-kfx-text-emphasis-position-horizontal", map[string]string{"$58": "over", "$60": "under"}},
	"$720": {"-kfx-text-emphasis-position-vertical", map[string]string{"$59": "left", "$61": "right"}},
	"$717": {"text-emphasis-style", map[string]string{
		"$724": "filled", "$728": "filled circle", "$726": "filled dot",
		"$730": "filled double-circle", "$734": "filled sesame", "$732": "filled triangle",
		"$725": "open", "$729": "open circle", "$727": "open dot",
		"$731": "open double-circle", "$735": "open sesame", "$733": "open triangle"}},
	"$706": {"text-orientation", map[string]string{"$383": "mixed", "$778": "sideways", "$779": "upright"}},
	"$36":  {"text-indent", nil},
	"$41":  {"text-transform", map[string]string{"$373": "lowercase", "$349": "none", "$374": "capitalize", "$372": "uppercase"}},
	"$497": {"text-shadow", nil},
	"$58":  {"top", nil},
	"$98":  {"transform", nil},
	"$549": {"transform-origin", nil},

	"$23": {"text-decoration", map[string]string{"$330": "underline dashed", "$331": "underline dotted", "$329": "underline double", "$349": "", "$328": "underline"}},
	"$24": {"text-decoration-color", nil},

	"$68": {"visibility", map[string]string{"false": "hidden", "true": "visible"}},
	"$716": {"white-space", map[string]string{"$715": "nowrap"}},
	"$56":  {"width", nil},
	"$569": {"word-break", map[string]string{"$570": "break-all", "$350": "normal"}},
	"$33":  {"word-spacing", nil},
	"$560": {"writing-mode", map[string]string{"$557": "horizontal-tb", "$559": "vertical-rl", "$558": "vertical-lr"}},

	"$650": {"-amzn-shape-outside", nil},
	"$646": {"-kfx-collision", nil},
	"$616": {"-kfx-attrib-epub-type", map[string]string{"$617": "noteref"}},
	"$658": {"yj-float-align", map[string]string{"$58": ""}},
	"$672": {"yj-float-bias", map[string]string{"$671": ""}},
	"$628": {"clear", map[string]string{"$59": "left", "$61": "right", "$421": "both", "$349": "none"}},
	"$673": {"yj-float-to-block", map[string]string{"false": ""}},
	"$644": {"-amzn-page-footer", map[string]string{"$442": "disable", "$441": "overlay"}},
	"$643": {"-amzn-page-header", map[string]string{"$442": "disable", "$441": "overlay"}},
	"$645": {"-amzn-max-crop-percentage", nil},
	"$790": {"-kfx-heading-level", nil},
	"$640": {"-kfx-user-margin-bottom-percentage", nil},
	"$641": {"-kfx-user-margin-left-percentage", nil},
	"$642": {"-kfx-user-margin-right-percentage", nil},
	"$639": {"-kfx-user-margin-top-percentage", nil},
	"$633": {"-kfx-table-vertical-align", map[string]string{"$350": "baseline", "$60": "bottom", "$320": "middle", "$58": "top"}},
	"$649": {"-kfx-attrib-epub-type", map[string]string{"$442": "amzn:decorative", "$441": "amzn:not-decorative"}},
	"$788": {"page-break-after", map[string]string{"$352": "always", "$383": "auto", "$353": "avoid"}},
	"$789": {"page-break-before", map[string]string{"$352": "always", "$383": "auto", "$353": "avoid"}},
}

// borderStyles mirrors Python BORDER_STYLES.
var borderStyles = map[string]string{
	"$349": "none", "$328": "solid", "$331": "dotted", "$330": "dashed",
	"$329": "double", "$335": "ridge", "$334": "groove", "$336": "inset", "$337": "outset",
}

// -----------------------------------------------------------------------
// Enum property mappers  (Python: Prop values switch/case tables)
//
// These functions map KFX symbol IDs to CSS enum values.  They were
// originally in css_values.go and yj_to_epub_resources.go (leftover
// from Phase 1 monolith split).  Moved here because they belong with
// the YJ_PROPERTY_INFO data-driven property system.
// -----------------------------------------------------------------------

func mapHyphens(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$383":
		return "auto"
	case "$384":
		return "manual"
	case "$349":
		return "none"
	default:
		return ""
	}
}

func mapPageBreak(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$352":
		return "always"
	case "$383":
		return "auto"
	case "$353":
		return "avoid"
	default:
		return ""
	}
}

func mapBorderStyle(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$349":
		return "none"
	case "$328":
		return "solid"
	case "$331":
		return "dotted"
	case "$330":
		return "dashed"
	case "$329":
		return "double"
	case "$335":
		return "ridge"
	case "$334":
		return "groove"
	case "$336":
		return "inset"
	case "$337":
		return "outset"
	default:
		return ""
	}
}

func mapBoxAlign(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$320":
		return "center"
	case "$59":
		return "left"
	case "$61":
		return "right"
	case "$321":
		return "justify"
	default:
		return ""
	}
}

func mapTableVerticalAlign(value interface{}) string {
	switch asStringDefault(value) {
	case "$350":
		return "baseline"
	case "$60":
		return "bottom"
	case "$320":
		return "middle"
	case "$58":
		return "top"
	default:
		return ""
	}
}

func mapTextDecoration(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$328":
		return "underline"
	default:
		return ""
	}
}

func mapFontVariant(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$369":
		return "small-caps"
	case "$349":
		return "normal"
	default:
		return ""
	}
}

func mapTextTransform(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$374":
		return "capitalize"
	case "$373":
		return "lowercase"
	case "$372":
		return "uppercase"
	case "$349":
		return "none"
	default:
		return ""
	}
}

func mapFontStyle(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$382":
		return "italic"
	case "$381":
		return "oblique"
	case "$350":
		return "normal"
	default:
		return ""
	}
}

func mapFontWeight(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$361":
		return "bold"
	case "$363":
		return "900"
	case "$357":
		return "300"
	case "$359":
		return "500"
	case "$350":
		return "normal"
	case "$360":
		return "600"
	case "$355":
		return "100"
	case "$362":
		return "800"
	case "$356":
		return "200"
	default:
		return ""
	}
}

func mapFontStretch(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$365":
		return "condensed"
	case "$368":
		return "expanded"
	case "$350":
		return "normal"
	case "$366":
		return "semi-condensed"
	case "$367":
		return "semi-expanded"
	default:
		return ""
	}
}

// yjPropertyNames is the set of recognized KFX property IDs (mirrors YJ_PROPERTY_NAMES).
var yjPropertyNames map[string]bool

func init() {
	yjPropertyNames = make(map[string]bool, len(yjPropertyInfo))
	for k := range yjPropertyInfo {
		yjPropertyNames[k] = true
	}
}

// -----------------------------------------------------------------------
// YJ_LENGTH_UNITS  (Python: YJ_LENGTH_UNITS)
// -----------------------------------------------------------------------

var yjLengthUnits = map[string]string{
	"$308": "em",
	"$506": "ch",
	"$315": "cm",
	"$309": "ex",
	"$317": "in",
	"$310": "lh",
	"$316": "mm",
	"$314": "%",
	"$318": "pt",
	"$319": "px",
	"$505": "rem",
	"$312": "vh",
	"$507": "vmax",
	"$313": "vmin",
	"$311": "vw",
}

// -----------------------------------------------------------------------
// COLOR_YJ_PROPERTIES  (Python: COLOR_YJ_PROPERTIES)
// -----------------------------------------------------------------------

var colorYJProperties = map[string]bool{
	"$83": true, "$86": true, "$85": true, "$87": true, "$84": true, "$116": true,
	"$498": true, "$70": true, "$121": true, "$105": true, "$555": true, "$28": true, "$75": true,
	"$21": true, "$19": true, "$718": true, "$24": true,
}

// -----------------------------------------------------------------------
// propertyValue  (Python: property_value, ~L1175)
//
// Converts a single KFX property value to a CSS string.
// This is the core of the data-driven pipeline.
// -----------------------------------------------------------------------

func propertyValue(propName string, yjValue interface{}) string {
	if yjValue == nil {
		return ""
	}

	info, infoOK := yjPropertyInfo[propName]

	switch v := yjValue.(type) {

	// IonStruct — length, color, shadow, transform-origin, etc.
	case map[string]interface{}:
		return propertyValueStruct(propName, v, info, infoOK)

	// string — could be a raw string, an enum symbol, font-family, language, etc.
	case string:
		if propName == "$11" {
			return cssFontFamily(v)
		}
		if propName == "$10" {
			return v // language string, keep as-is
		}
		if propName == "$173" {
			// -kfx-style-name: pass through (Python uses unique_part_of_local_symbol here)
			return v
		}
		// Check if this is an enum symbol ("$328" etc.) that maps through propInfo.values
		if infoOK && info.values != nil {
			if mapped, ok := info.values[v]; ok {
				return mapped // may be "" for None-mapped values
			}
		}
		// Color properties: $349 means color(0) → transparent/black
		if colorYJProperties[propName] && v == "$349" {
			return fixColorValue(0)
		}
		return v

	// IonSymbol — enum values mapped via propInfo.values
	// In Go ION decode, symbols arrive as strings with "$" prefix.
	// bool maps use "true"/"false" keys.

	// int / *float64 / float64 — numeric or color values
	case int:
		return propertyValueNumeric(propName, float64(v), info, infoOK)
	case int64:
		return propertyValueNumeric(propName, float64(v), info, infoOK)
	case float64:
		return propertyValueNumeric(propName, v, info, infoOK)
	case *float64:
		if v == nil {
			return ""
		}
		return propertyValueNumeric(propName, *v, info, infoOK)

	// bool — mapped via propInfo.values
	case bool:
		key := "false"
		if v {
			key = "true"
		}
		if infoOK && info.values != nil {
			if mapped, ok := info.values[key]; ok {
				return mapped
			}
		}
		return fmt.Sprintf("%v", v)

	// IonList — layout hints, collisions, transforms, shadows
	case []interface{}:
		return propertyValueList(propName, v, info, infoOK)
	}

	return fmt.Sprintf("%v", yjValue)
}

// propertyValueStruct handles struct-type KFX property values (lengths, colors, shadows, etc.).
func propertyValueStruct(propName string, v map[string]interface{}, info propInfo, infoOK bool) string {
	// Length: {$307: magnitude, $306: unit}
	if mag, ok := asFloat64(v["$307"]); ok {
		unitSym, _ := asString(v["$306"])
		unit := yjLengthUnits[unitSym]
		if unit == "" {
			unit = unitSym
		}
		if mag == 0 {
			return "0"
		}
		// FIX_PT_TO_PX: convert pt → px when magnitude is divisible by 0.225
		// Python: property_value ~L1190
		if unitSym == "$318" && mag > 0 {
			if int(mag*1000)%225 == 0 {
				mag = float64(int(mag*1000)) / 450.0
				unit = "px"
			}
		}
		return formatCSSQuantity(mag) + unit
	}

	// Color: {$19: int}
	if colorVal, ok := v["$19"]; ok {
		return fixColorValue(colorVal)
	}

	// Shadow: {$499/$500/$501/$502/$498, optional $336 inset}
	if _, has499 := v["$499"]; has499 {
		if _, has500 := v["$500"]; has500 {
			// Simplified shadow handling — full port later if needed
			parts := []string{}
			for _, sub := range []string{"$499", "$500", "$501", "$502", "$498"} {
				if subVal, ok := v[sub]; ok {
					parts = append(parts, propertyValue(sub, subVal))
				}
			}
			if _, inset := v["$336"]; inset {
				parts = append(parts, "inset")
			}
			return strings.Join(parts, " ")
		}
	}

	// transform-origin: {$58/$59}
	if _, hasTop := v["$58"]; hasTop {
		if propName == "$549" {
			parts := []string{}
			for _, sub := range []string{"$59", "$58"} {
				if subVal, ok := v[sub]; ok {
					parts = append(parts, propertyValue(sub, subVal))
				} else {
					parts = append(parts, "50%")
				}
			}
			return strings.Join(parts, " ")
		}
		// Rect-style value: top/right/bottom/left
		parts := []string{}
		for _, sub := range []string{"$58", "$61", "$60", "$59"} {
			if subVal, ok := v[sub]; ok {
				parts = append(parts, propertyValue(sub, subVal))
			}
		}
		return strings.Join(parts, " ")
	}

	// keep-lines-together: {$131/$132}
	if _, has131 := v["$131"]; has131 {
		oVal := valueStr(v["$131"])
		wVal := valueStr(v["$132"])
		if oVal == "" || oVal == "0" {
			oVal = "inherit"
		}
		if wVal == "" || wVal == "0" {
			wVal = "inherit"
		}
		return oVal + " " + wVal
	}

	// Fallback: unknown struct
	return fmt.Sprintf("%v", v)
}

// propertyValueNumeric handles int/float KFX property values (colors, px values, raw numbers).
func propertyValueNumeric(propName string, v float64, info propInfo, infoOK bool) string {
	// Color property
	if colorYJProperties[propName] {
		return fixColorValue(v)
	}

	// Properties that stay as raw numbers (no px suffix)
	rawNumberProps := map[string]bool{
		"$112": true, "$13": true, "$148": true, "$149": true,
		"$645": true, "$647": true, "$648": true, "$790": true,
		"$640": true, "$641": true, "$642": true, "$639": true,
		"$72": true, "$126": true, "$125": true, "$42": true,
	}

	if rawNumberProps[propName] || v == 0 {
		return valueStr(v)
	}

	return valueStr(v) + "px"
}

// propertyValueList handles list-type KFX property values.
func propertyValueList(propName string, v []interface{}, info propInfo, infoOK bool) string {
	switch propName {
	case "$761": // layout hints
		hints := []string{}
		for _, item := range v {
			if s, ok := item.(string); ok {
				hints = append(hints, s)
			}
		}
		return strings.Join(hints, " ")

	case "$646": // collision
		vals := []string{}
		for _, item := range v {
			if s, ok := item.(string); ok {
				if mapped, ok := collisions[s]; ok {
					vals = append(vals, mapped)
				}
			}
		}
		return strings.Join(vals, " ")

	case "$98": // transform — simplified
		return fmt.Sprintf("%v", v)

	case "$497": // text-shadow list
		vals := make([]string, 0, len(v))
		for _, item := range v {
			vals = append(vals, propertyValue(propName, item))
		}
		return strings.Join(vals, ", ")

	case "$531": // stroke-dasharray
		vals := make([]string, 0, len(v))
		for _, item := range v {
			vals = append(vals, propertyValue(propName, item))
		}
		return strings.Join(vals, " ")
	}

	return fmt.Sprintf("%v", v)
}

// collisions mirrors Python COLLISIONS.
var collisions = map[string]string{
	"$352": "always",
	"$652": "queue",
}

// -----------------------------------------------------------------------
// convertYJProperties  (Python: convert_yj_properties, ~L1088)
//
// Takes a map of KFX property IDs → raw values and returns a flat
// CSS property → value map, exactly as Python's convert_yj_properties.
// -----------------------------------------------------------------------

func convertYJProperties(yjProperties map[string]interface{}) map[string]string {
	declarations := map[string]string{}

	for yjPropName, yjValue := range yjProperties {
		value := propertyValue(yjPropName, yjValue)
		if value == "" || value == "?" {
			continue
		}

		var cssName string
		if info, ok := yjPropertyInfo[yjPropName]; ok {
			cssName = info.name
		} else {
			// Unknown property — use the ID with hyphens
			cssName = yjPropName
		}

		// position: oeb-page-foot/oeb-page-head → display (EPUB2 handling)
		if cssName == "position" && (value == "oeb-page-foot" || value == "oeb-page-head") {
			continue // skip for now (EPUB3 path)
		}

		if existing, ok := declarations[cssName]; ok && existing != value {
			// text-decoration merges
			if cssName == "text-decoration" {
				declarations[cssName] = mergeTextDecoration(existing, value)
				continue
			}
			// -kfx-attrib-epub-type merges
			if cssName == "-kfx-attrib-epub-type" {
				declarations[cssName] = mergeEpubType(existing, value)
				continue
			}
			// Otherwise last-write-wins (Python logs error)
		}

		declarations[cssName] = value
	}

	// Post-processing: background-position, background-size, fill-color, etc.
	if _, ok := declarations["-kfx-background-positionx"]; ok {
		x := popMap(declarations, "-kfx-background-positionx", "50%")
		y := popMap(declarations, "-kfx-background-positiony", "50%")
		declarations["background-position"] = x + " " + y
	}
	if _, ok := declarations["-kfx-background-sizex"]; ok {
		x := popMap(declarations, "-kfx-background-sizex", "auto")
		y := popMap(declarations, "-kfx-background-sizey", "auto")
		declarations["background-size"] = x + " " + y
	}
	if _, okFC := declarations["-kfx-fill-color"]; okFC {
		fillColor := popMap(declarations, "-kfx-fill-color", "#ffffff")
		fillOpacity := popMap(declarations, "-kfx-fill-opacity", "")
		declarations["background-color"] = addColorOpacityStr(fillColor, fillOpacity)
	} else if _, okFO := declarations["-kfx-fill-opacity"]; okFO {
		// Ported from Python: also triggers on fill-opacity alone (using default #ffffff fill color).
		fillOpacity := popMap(declarations, "-kfx-fill-opacity", "")
		declarations["background-color"] = addColorOpacityStr("#ffffff", fillOpacity)
	}
	if _, ok := declarations["-kfx-text-emphasis-position-horizontal"]; ok {
		h := popMap(declarations, "-kfx-text-emphasis-position-horizontal", "")
		v := popMap(declarations, "-kfx-text-emphasis-position-vertical", "")
		parts := []string{}
		for _, s := range []string{h, v} {
			if s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			declarations["text-emphasis-position"] = strings.Join(parts, " ")
		}
	}
	if _, ok := declarations["-kfx-keep-lines-together"]; ok {
		kt := popMap(declarations, "-kfx-keep-lines-together", "")
		if kt != "" {
			parts := strings.Fields(kt)
			if len(parts) >= 2 {
				if parts[0] != "inherit" {
					declarations["orphans"] = parts[0]
				}
				if parts[1] != "inherit" {
					declarations["widows"] = parts[1]
				}
			}
		}
	}

	// Post-processing: -kfx-table-vertical-align → vertical-align
	// Ported from Python fix_vertical_align_properties (yj_to_epub_content.py ~L1497).
	// In Python, -kfx-table-vertical-align is renamed to vertical-align at content rendering time.
	// Since Go emits -kfx-table-vertical-align from YJ property mapping, we convert it here.
	if val := popMap(declarations, "-kfx-table-vertical-align", ""); val != "" {
		if _, exists := declarations["vertical-align"]; !exists {
			declarations["vertical-align"] = val
		}
	}

	// Post-processing: -kfx-baseline-shift → vertical-align
	// Part of Python's fix_vertical_align_properties handling for baseline-shift.
	if val := popMap(declarations, "-kfx-baseline-shift", ""); val != "" {
		if _, exists := declarations["vertical-align"]; !exists {
			declarations["vertical-align"] = val
		}
	}

	return declarations
}

// -----------------------------------------------------------------------
// processContentProperties  (Python: process_content_properties, ~L1081)
//
// Extracts KFX properties from a content dict, converts them, returns
// CSS declaration map.
// -----------------------------------------------------------------------

func processContentProperties(content map[string]interface{}) map[string]string {
	contentProperties := map[string]interface{}{}
	for k := range content {
		if yjPropertyNames[k] {
			contentProperties[k] = content[k]
		}
	}
	return convertYJProperties(contentProperties)
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

// cssDeclarationsFromMap converts a CSS property→value map to a sorted slice of "property: value" strings.
// It skips internal -kfx- properties that are not real CSS and skips empty values.
func cssDeclarationsFromMap(m map[string]string) []string {
	// Canonical order for common CSS properties (matches Python output order roughly)
	order := []string{
		"font-family", "font-size", "font-style", "font-weight", "font-variant",
		"line-height",
		"margin-top", "margin-bottom", "margin-left", "margin-right",
		"padding-top", "padding-bottom", "padding-left", "padding-right",
		"text-align", "text-indent", "text-transform",
		"color", "background-color",
		"border-top-color", "border-top-style", "border-top-width",
		"border-bottom-color", "border-bottom-style", "border-bottom-width",
		"border-left-color", "border-left-style", "border-left-width",
		"border-right-color", "border-right-style", "border-right-width",
		"page-break-inside",
	}
	ordered := make([]string, 0, len(m))
	seen := map[string]bool{}
	for _, prop := range order {
		if val, ok := m[prop]; ok && val != "" {
			ordered = append(ordered, prop+": "+val)
			seen[prop] = true
		}
	}
	// Remaining properties in sorted order
	remaining := make([]string, 0, len(m))
	for prop, val := range m {
		if seen[prop] || val == "" || strings.HasPrefix(prop, "-kfx-") {
			continue
		}
		remaining = append(remaining, prop)
	}
	sort.Strings(remaining)
	for _, prop := range remaining {
		ordered = append(ordered, prop+": "+m[prop])
	}
	return ordered
}

// popMap removes and returns a value from a map, or returns defaultVal.
func popMap(m map[string]string, key, defaultVal string) string {
	if v, ok := m[key]; ok {
		delete(m, key)
		return v
	}
	return defaultVal
}

func mergeTextDecoration(a, b string) string {
	set := map[string]bool{}
	for _, s := range strings.Fields(a) {
		set[s] = true
	}
	for _, s := range strings.Fields(b) {
		set[s] = true
	}
	parts := make([]string, 0, len(set))
	for s := range set {
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func mergeEpubType(a, b string) string {
	set := map[string]bool{}
	for _, s := range strings.Fields(a) {
		set[s] = true
	}
	for _, s := range strings.Fields(b) {
		set[s] = true
	}
	parts := make([]string, 0, len(set))
	for s := range set {
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

// valueStr formats a numeric value for CSS output, matching Python's value_str
// (epub_output.py:1373-1393). For float64 values it delegates to formatCSSQuantity
// which implements the full %g → %.4f fallback → trailing zero strip pipeline.
func valueStr(v interface{}) string {
	switch n := v.(type) {
	case float64:
		return formatCSSQuantity(n)
	case *float64:
		if n == nil {
			return "0"
		}
		return formatCSSQuantity(*n)
	case int:
		if n == 0 {
			return "0"
		}
		return fmt.Sprintf("%d", n)
	case int64:
		if n == 0 {
			return "0"
		}
		return fmt.Sprintf("%d", n)
	case string:
		return n
	}
	return fmt.Sprintf("%v", v)
}

// valueStrWithUnit is the full port of Python's value_str (epub_output.py:1373-1393),
// including unit suffix and emit_zero_unit flag.
//
// Python signature: def value_str(quantity, unit="", emit_zero_unit=False)
func valueStrWithUnit(quantity interface{}, unit string, emitZeroUnit bool) string {
	// Rule 1: None → return just the unit
	if quantity == nil {
		return unit
	}

	// Format the quantity as a string
	var qStr string
	switch n := quantity.(type) {
	case float64:
		qStr = formatCSSQuantity(n)
	case *float64:
		if n == nil {
			return unit
		}
		qStr = formatCSSQuantity(*n)
	case int:
		if n == 0 {
			qStr = "0"
		} else {
			qStr = fmt.Sprintf("%d", n)
		}
	case int64:
		if n == 0 {
			qStr = "0"
		} else {
			qStr = fmt.Sprintf("%d", n)
		}
	default:
		qStr = fmt.Sprintf("%v", quantity)
	}

	// Rule 7: "0" without emitZeroUnit → return just "0" (no unit)
	if qStr == "0" && !emitZeroUnit {
		return qStr
	}

	return qStr + unit
}

// colorStr converts an RGB integer and alpha to a CSS color string.
// Ported from Python color_str (yj_to_epub_properties.py:2121-2134).
//
// Rules:
//   - alpha == 1.0: named color if known, else 3-char hex for black/white, else 6-char hex
//   - alpha != 1.0: rgba(r,g,b,alpha_str) with alpha formatted as %0.3f (then trailing zeros stripped)
func colorStr(rgbInt int, alpha float64) string {
	if alpha == 1.0 {
		hexColor := fmt.Sprintf("#%06x", rgbInt&0x00ffffff)
		if name, ok := colorName[hexColor]; ok {
			return name
		}
		if hexColor == "#000000" {
			return "#000"
		}
		if hexColor == "#ffffff" {
			return "#fff"
		}
		return hexColor
	}

	red := (rgbInt & 0x00ff0000) >> 16
	green := (rgbInt & 0x0000ff00) >> 8
	blue := rgbInt & 0x000000ff

	var alphaStr string
	if alpha == 0.0 {
		alphaStr = "0"
	} else {
		// Python: "%0.3f" % alpha → 3 decimal places, then strip trailing zeros
		alphaStr = formatAlpha(alpha)
	}

	return fmt.Sprintf("rgba(%d,%d,%d,%s)", red, green, blue, alphaStr)
}

// formatAlpha formats an alpha value matching Python's "%0.3f" % alpha.
// Python's color_str uses this directly without trailing zero stripping,
// so 0.5 → "0.500", 0.123 → "0.123", etc.
func formatAlpha(alpha float64) string {
	return fmt.Sprintf("%.3f", alpha)
}

// intToAlpha converts an integer alpha byte (0-255) to a float64 alpha value.
// Ported from Python int_to_alpha (yj_to_epub_properties.py:2159-2166).
func intToAlpha(alphaInt int) float64 {
	if alphaInt < 2 {
		return 0.0
	}
	if alphaInt > 253 {
		return 1.0
	}
	return math.Max(math.Min(float64(alphaInt+1)/256.0, 1.0), 0.0)
}

// alphaToInt converts a float64 alpha value to an integer alpha byte (0-255).
// Ported from Python alpha_to_int (yj_to_epub_properties.py:2168-2175).
func alphaToInt(alpha float64) int {
	if alpha < 0.012 {
		return 0
	}
	if alpha > 0.996 {
		return 255
	}
	return int(math.Max(math.Min(alpha*256.0+0.5, 255), 0)) - 1
}

// numstr is an unexported alias for Numstr (yj_structure.py:1313: "%g" % x).
func numstr(x float64) string {
	return Numstr(x)
}
func asFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case *float64:
		if n == nil {
			return 0, false
		}
		return *n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// colorName maps hex colors to CSS named colors, ported from Python COLOR_NAME.
var colorName = map[string]string{
	"#000000": "black",
	"#000080": "navy",
	"#0000ff": "blue",
	"#008000": "green",
	"#008080": "teal",
	"#00ff00": "lime",
	"#00ffff": "cyan",
	"#800000": "maroon",
	"#800080": "purple",
	"#808000": "olive",
	"#808080": "gray",
	"#ff0000": "red",
	"#ff00ff": "magenta",
	"#ffff00": "yellow",
	"#ffffff": "white",
}

// fixColorValue converts a numeric color to a CSS color string.
// Ported from Python fix_color_value (yj_to_epub_properties.py:2102-2107).
// It extracts the ARGB packed integer, computes alpha via intToAlpha,
// then delegates to colorStr for the final formatting.
func fixColorValue(v interface{}) string {
	var n float64
	switch val := v.(type) {
	case float64:
		n = val
	case *float64:
		if val == nil {
			return "#000000"
		}
		n = *val
	case int:
		n = float64(val)
	case int64:
		n = float64(val)
	default:
		return fmt.Sprintf("%v", v)
	}
	// Convert to int for bit manipulation
	i := int(n)
	alpha := intToAlpha((i >> 24) & 0xFF)
	return colorStr(i, alpha)
}

// addColorOpacityStr is the string-argument version of addColorOpacity.
func addColorOpacityStr(color, opacity string) string {
	if opacity == "" || opacity == "1" {
		return color
	}
	// Ported from Python add_color_opacity: parse opacity, extract RGB from color,
	// produce rgba(r,g,b,alpha) string.
	op, err := strconv.ParseFloat(opacity, 64)
	if err != nil {
		return color
	}
	if op >= 0.999 {
		return color
	}
	// Parse the color to extract r, g, b.
	// Handle #rrggbb format (most common from fixColorValue).
	if strings.HasPrefix(color, "#") && len(color) == 7 {
		r, err1 := strconv.ParseUint(color[1:3], 16, 8)
		g, err2 := strconv.ParseUint(color[3:5], 16, 8)
		b, err3 := strconv.ParseUint(color[5:7], 16, 8)
		if err1 == nil && err2 == nil && err3 == nil {
			if op <= 0.001 {
				return fmt.Sprintf("rgba(%d,%d,%d,0)", r, g, b)
			}
			return fmt.Sprintf("rgba(%d,%d,%d,%s)", r, g, b, formatAlpha(op))
		}
	}
	// Fallback for other color formats: just return color (shouldn't happen for fill-color).
	return color
}

// CSS unit conversion constants, ported from Python yj_to_epub_properties.py.
const (
	// lineHeightScaleFactor is LINE_HEIGHT_SCALE_FACTOR = 1.2 (Python decimal.Decimal("1.2")).
	lineHeightScaleFactor = 1.2
	// minimumLineHeight is MINIMUM_LINE_HEIGHT = 1.0 (Python decimal.Decimal("1.0")).
	minimumLineHeight = 1.0
	// useNormalLineHeight is USE_NORMAL_LINE_HEIGHT = True in Python.
	useNormalLineHeight = true
)

// splitCSSValue splits a CSS value string into its numeric quantity and unit parts.
// Ported from Python split_value in epub_output.py.
// Returns (nil, val) if the value is not numeric.
func splitCSSValue(val string) (*float64, string) {
	if val == "" {
		return nil, val
	}
	// Match optional sign, then digits with optional decimal point
	i := 0
	if i < len(val) && (val[i] == '+' || val[i] == '-') {
		i++
	}
	digitStart := i
	hasDot := false
	hasDigit := false
	for i < len(val) {
		c := val[i]
		if c >= '0' && c <= '9' {
			hasDigit = true
			i++
		} else if c == '.' && !hasDot {
			hasDot = true
			i++
		} else {
			break
		}
	}
	if !hasDigit {
		return nil, val
	}
	numStr := val[digitStart:i]
	unit := val[i:]
	quantity, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return nil, val
	}
	// Handle leading sign
	if digitStart > 0 && val[0] == '-' {
		quantity = -quantity
	}
	return &quantity, unit
}

// formatCSSQuantity formats a float64 for CSS output, matching Python's
// value_str (epub_output.py:1373-1393) for the quantity portion.
//
// Rules ported from Python value_str:
//  1. Near-zero (abs < 1e-6) → "0"
//  2. Format with %g (Go: 6 significant digits via 'g' format)
//  3. If scientific notation in result → reformat with %.4f
//  4. Strip trailing zeros after decimal, then strip trailing decimal point
//  5. Handle negative values correctly
func formatCSSQuantity(q float64) string {
	// Rule 1: near-zero → "0"
	if math.Abs(q) < 1e-6 {
		return "0"
	}

	// Rule 2: format with %g (6 significant digits)
	s := strconv.FormatFloat(q, 'g', -1, 64)

	// Rule 3: if scientific notation detected → reformat with %.4f
	if strings.Contains(s, "e") || strings.Contains(s, "E") {
		s = strconv.FormatFloat(q, 'f', 4, 64)
	}

	// Rule 4: strip trailing zeros after decimal, then strip trailing dot
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}

	// Normalize -0 to 0 (can happen after stripping trailing zeros from negative small values)
	if s == "-0" {
		s = "0"
	}

	return s
}

// convertStyleUnits converts lh and rem CSS units to em/unitless in a style map.
// Ported from Python yj_to_epub_properties.py simplify_styles lines 1713-1752.
// This conversion happens in simplifyStylesElementFull BEFORE the comparison/stripping loop,
// so that lh/rem values are normalized to em before being compared against heritableDefaultProperties.
//
// lh conversion:
//   - For line-height: if USE_NORMAL_LINE_HEIGHT and value in [0.99, 1.01], set to "normal"
//   - Otherwise: multiply by LINE_HEIGHT_SCALE_FACTOR (1.2), clamp to MINIMUM_LINE_HEIGHT (1.0)
//   - For line-height: emit unitless value (e.g., "1.2")
//   - For other properties: emit em value (e.g., "1.2em")
//
// rem conversion:
//   - Convert rem to em based on the base font-size units
//   - If base font-size is in rem: divide quantity by base_font_size_quantity
//   - If base font-size is in em: keep quantity, change unit to em
//   - For line-height: also apply MINIMUM_LINE_HEIGHT clamping
func convertStyleUnits(sty map[string]string, inherited map[string]string) {
	// Save original font-size for rem conversion (need it before we modify sty).
	origFontSize, hasOrigFontSize := sty["font-size"]

	for name, val := range sty {
		quantity, unit := splitCSSValue(val)
		if quantity == nil {
			continue
		}

		if unit == "lh" {
			q := *quantity
			if name == "line-height" {
				if useNormalLineHeight && q >= 0.99 && q <= 1.01 {
					sty[name] = "normal"
				} else {
					q = q * lineHeightScaleFactor
					if q < minimumLineHeight {
						q = minimumLineHeight
					}
					sty[name] = formatCSSQuantity(q)
				}
			} else {
				q = q * lineHeightScaleFactor
				sty[name] = formatCSSQuantity(q) + "em"
			}
		}

		// Re-parse in case lh conversion changed the value
		quantity2, unit2 := splitCSSValue(sty[name])
		if quantity2 != nil {
			quantity = quantity2
			unit = unit2
		} else {
			// Value is "normal" or non-numeric; skip rem conversion
			continue
		}

		if unit == "rem" {
			q := *quantity
			var baseFontSize string
			if name == "font-size" {
				// Use inherited font-size
				baseFontSize = inherited["font-size"]
			} else {
				// Use the element's own original font-size
				if hasOrigFontSize {
					baseFontSize = origFontSize
				} else {
					baseFontSize = sty["font-size"]
				}
			}

			baseQuantity, baseUnit := splitCSSValue(baseFontSize)
			if baseQuantity != nil {
				if baseUnit == "rem" {
					q = q / *baseQuantity
					unit = "em"
				} else if baseUnit == "em" {
					unit = "em"
				}
				// else: log error in Python; we silently skip
			}

			if name == "line-height" && q < minimumLineHeight {
				q = minimumLineHeight
			}

			if unit == "em" {
				sty[name] = formatCSSQuantity(q) + "em"
			} else {
				sty[name] = formatCSSQuantity(q) + unit
			}
		}

		// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1753-1785):
		// Convert vh/vw viewport units to percentages for page-aligned content.
		// Python does full cross-conversion for images with wrong-axis units (e.g. height in vw),
		// which requires image dimension knowledge. We handle the simple case (direct conversion)
		// and skip the complex cross-conversion.
		if (unit == "vh" || unit == "vw") && quantity != nil {
			if pageAlignVal, hasAlign := sty["-amzn-page-align"]; hasAlign && pageAlignVal != "none" {
				if name == "height" || name == "width" {
					sty[name] = formatCSSQuantity(*quantity) + "%"
				}
			}
		}
	}
}
