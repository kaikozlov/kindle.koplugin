package kfx

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
)


// Keys of Python self.condition_operators after set_condition_operators (yj_to_epub_misc.py L37–66).
var pythonConditionOperatorSymbols = map[string]struct{}{
	"screenActualHeight": {}, "screenActualWidth": {}, "hasColor": {}, "hasVideo": {}, "position": {}, "screenPixelWidth": {}, "screenPixelHeight": {},
	"isLandscape": {}, "isPortrait": {}, "yj.illustrated_layout": {},
	"not": {}, "anchor": {}, "yj.layout_type": {}, "yj.supports": {},
	"and": {}, "or": {}, "==": {}, "!=": {}, ">": {}, ">=": {}, "<": {}, "<=": {},
	"+": {}, "-": {}, "*": {}, "/": {},
}

// conditionOperatorArity mirrors Python nargs: 0=nullary, 1=unary, 2=binary, -1=$659 (special).
var conditionOperatorArity = map[string]int{
	"screenActualHeight": 0, "screenActualWidth": 0, "hasColor": 0, "hasVideo": 0, "position": 0, "screenPixelWidth": 0, "screenPixelHeight": 0,
	"isLandscape": 0, "isPortrait": 0, "yj.illustrated_layout": 0,
	"not": 1, "anchor": 1, "yj.layout_type": 1, "yj.supports": -1,
	"and": 2, "or": 2, "==": 2, "!=": 2, ">": 2, ">=": 2, "<": 2, "<=": 2,
	"+": 2, "-": 2, "*": 2, "/": 2,
}

func (e conditionEvaluator) screenSize() (int, int) {
	if e.orientationLock == "landscape" {
		return 1920, 1200
	}
	return 1200, 1920
}

func firstArg(args []interface{}) interface{} {
	if len(args) > 0 {
		return args[0]
	}
	return nil
}

func secondArg(args []interface{}) interface{} {
	if len(args) > 1 {
		return args[1]
	}
	return nil
}

func numericConditionValue(value interface{}) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	case bool:
		if typed {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func compareConditionValues(left interface{}, right interface{}) int {
	switch l := left.(type) {
	case bool:
		lv, rv := 0, 0
		if l {
			lv = 1
		}
		if rb, ok := right.(bool); ok && rb {
			rv = 1
		}
		switch {
		case lv < rv:
			return -1
		case lv > rv:
			return 1
		default:
			return 0
		}
	case string:
		rs, _ := right.(string)
		switch {
		case l < rs:
			return -1
		case l > rs:
			return 1
		default:
			return 0
		}
	default:
		lf := numericConditionValue(left)
		rf := numericConditionValue(right)
		switch {
		case lf < rf:
			return -1
		case lf > rf:
			return 1
		default:
			return 0
		}
	}
}

type conditionOpFn func(*conditionEvaluator, []interface{}, int, int) interface{}

var (
	conditionOperatorDispatchOnce sync.Once
	conditionOperatorDispatch     map[string]conditionOpFn
)

func conditionOperatorDispatchTable() map[string]conditionOpFn {
	conditionOperatorDispatchOnce.Do(func() {
		conditionOperatorDispatch = map[string]conditionOpFn{
			"not": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return !e.evaluateBinary(firstArg(args))
			},
			"anchor": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} {
				_, _ = width, height
				return 0
			},
			"yj.layout_type": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				arg, _ := asString(firstArg(args))
				switch arg {
				case "yj.in_page":
					return true
				case "yj.table_viewer":
					return false
				default:
					return false
				}
			},
			"yj.supports": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return knownSupportedFeatures[featureKey(args)]
			},
			"and": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return e.evaluateBinary(firstArg(args)) && e.evaluateBinary(secondArg(args))
			},
			"or": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return e.evaluateBinary(firstArg(args)) || e.evaluateBinary(secondArg(args))
			},
			"==": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return compareConditionValues(e.evaluate(firstArg(args)), e.evaluate(secondArg(args))) == 0
			},
			"!=": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return compareConditionValues(e.evaluate(firstArg(args)), e.evaluate(secondArg(args))) != 0
			},
			">": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return compareConditionValues(e.evaluate(firstArg(args)), e.evaluate(secondArg(args))) > 0
			},
			">=": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return compareConditionValues(e.evaluate(firstArg(args)), e.evaluate(secondArg(args))) >= 0
			},
			"<": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return compareConditionValues(e.evaluate(firstArg(args)), e.evaluate(secondArg(args))) < 0
			},
			"<=": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return compareConditionValues(e.evaluate(firstArg(args)), e.evaluate(secondArg(args))) <= 0
			},
			"+": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return numericConditionValue(e.evaluate(firstArg(args))) + numericConditionValue(e.evaluate(secondArg(args)))
			},
			"-": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return numericConditionValue(e.evaluate(firstArg(args))) - numericConditionValue(e.evaluate(secondArg(args)))
			},
			"*": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return numericConditionValue(e.evaluate(firstArg(args))) * numericConditionValue(e.evaluate(secondArg(args)))
			},
			"/": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				divisor := numericConditionValue(e.evaluate(secondArg(args)))
				if divisor == 0 {
					return 0
				}
				return numericConditionValue(e.evaluate(firstArg(args))) / divisor
			},
			"screenActualHeight": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} { _ = width; return height },
			"screenPixelHeight": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} { _ = width; return height },
			"screenActualWidth": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} { _ = height; return width },
			"screenPixelWidth": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} { _ = height; return width },
			"hasColor": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} {
				_ = width
				_ = height
				return true
			},
			"hasVideo": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} {
				_ = width
				_ = height
				return true
			},
			"position": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} {
				_ = width
				_ = height
				return 0
			},
			"isLandscape": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} { return width > height },
			"isPortrait": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} { return width < height },
			"yj.illustrated_layout": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} {
				_ = width
				_ = height
				return true
			},
		}
	})
	return conditionOperatorDispatch
}

// Port of KFX_EPUB_Misc.evaluate_condition (yj_to_epub_misc.py) for Ion sexp conditions.
func (e conditionEvaluator) evaluate(condition interface{}) interface{} {
	switch typed := condition.(type) {
	case []interface{}:
		if len(typed) == 0 {
			return false
		}
		op, _ := asString(typed[0])
		args := typed[1:]
		width, height := e.screenSize()
		ee := e
		dispatch := conditionOperatorDispatchTable()
		if fn, ok := dispatch[op]; ok {
			// Port of Python nargs validation (yj_to_epub_misc.py evaluate_condition):
			//   if nargs != num: log.error("Condition operator has wrong number of arguments")
			if arity, known := conditionOperatorArity[op]; known && arity >= 0 && arity != len(args) {
				fmt.Fprintf(os.Stderr, "kfx: error: condition operator %q has wrong number of arguments: %v (expected %d)\n", op, args, arity)
				return false
			}
			return fn(&ee, args, width, height)
		}
		fmt.Fprintf(os.Stderr, "kfx: error: condition operator is unknown: %v\n", typed)
		return false
	case string:
		return e.evaluate([]interface{}{typed})
	case bool:
		return typed
	case int, int64, float64:
		return typed
	default:
		return false
	}
}

// Port of KFX_EPUB_Misc.evaluate_binary_condition (yj_to_epub_misc.py):
// logs error for non-binary results, matching Python's behavior.
func (e conditionEvaluator) evaluateBinary(condition interface{}) bool {
	value := e.evaluate(condition)
	typed, ok := value.(bool)
	if !ok && value != nil {
		fmt.Fprintf(os.Stderr, "kfx: error: condition has non-binary result (%v): %v\n", value, condition)
	}
	return ok && typed
}

// ---------------------------------------------------------------------------
// Port of yj_to_epub_misc.py KFX_EPUB_Misc functions (L123–605)
// ---------------------------------------------------------------------------

// pxToInt parses a CSS pixel value string and returns its integer value.
// Port of Python px_to_int (yj_to_epub_misc.py L607–608).
func pxToInt(s string) int {
	return int(math.Round(pxToFloat(s)))
}

// pxToFloat parses a CSS pixel value string and returns its float value.
// Port of Python px_to_float (yj_to_epub_misc.py L610–612).
func pxToFloat(s string) float64 {
	m := pxToFloatRe.FindStringSubmatch(s)
	if m == nil {
		return 0.0
	}
	var f float64
	fmt.Sscanf(m[1], "%f", &f)
	return f
}

var pxToFloatRe = regexp.MustCompile(`^([0-9]+[.]?[0-9]*)(px)?$`)

// splitCSSValueUnit splits a CSS value string into numeric quantity and unit.
// Port of Python split_value (epub_output.py).
// Returns ("", "") if the value is empty.
func splitCSSValueUnit(val string) (float64, string) {
	if val == "" {
		return 0, ""
	}
	q, unit := splitCSSValue(val)
	if q == nil {
		return 0, val
	}
	return *q, unit
}

// addSVGWrapperToBlockImage replaces a div>img with an SVG wrapper element.
// Port of Python KFX_EPUB_Misc.add_svg_wrapper_to_block_image (yj_to_epub_misc.py L123–195).
//
// Python branch audit:
//   L125: if len(content_elem) != 1 → error return
//   L129: for image_div in content_elem.findall("*") → iterate children
//   L131: if image_div.tag == "div" and len(image_div) == 1 and image_div[0].tag == "img" → main path
//   L148: if img_file is None → error return
//   L158: if (int_height and fixed_height and ...) or (int_width and ...) → error log
//   L163: if int_height and int_width → aspect ratio check
//   L166: if abs(img_aspect - svg_aspect) > 0.01 → error log
//   L170: else → use img dimensions
//   L173: if not (div_style.pop("text-align","center")=="center" and ...) → error log (validation)
//   L195: else → error log (incorrect image content)
func addSVGWrapperToBlockImage(contentElem *htmlElement, fixedHeight, fixedWidth int, oebpsFiles map[string]*outputFile, sectionFilename string) {
	// Python L125: if len(content_elem) != 1
	children := contentElem.Children
	if len(children) != 1 {
		fmt.Fprintf(os.Stderr, "kfx: error: incorrect div content for SVG wrapper: expected 1 child, got %d\n", len(children))
		return
	}

	imageDiv, ok := children[0].(*htmlElement)
	if !ok {
		fmt.Fprintf(os.Stderr, "kfx: error: incorrect div content for SVG wrapper: child is not an element\n")
		return
	}

	// Python L131: if image_div.tag == "div" and len(image_div) == 1 and image_div[0].tag == "img"
	if imageDiv.Tag != "div" || len(imageDiv.Children) != 1 {
		fmt.Fprintf(os.Stderr, "kfx: error: incorrect image content for SVG wrapper: expected div>img\n")
		return
	}

	img, ok := imageDiv.Children[0].(*htmlElement)
	if !ok || img.Tag != "img" {
		fmt.Fprintf(os.Stderr, "kfx: error: incorrect image content for SVG wrapper: expected img element\n")
		return
	}

	// Extract style properties from div and img elements.
	// Python: div_style = self.get_style(image_div) → parse style attr
	divStyle := parseElementStyle(imageDiv)
	delete(divStyle, "-kfx-style-name")
	delete(divStyle, "font-size")
	delete(divStyle, "line-height")
	delete(divStyle, "-kfx-heading-level")
	delete(divStyle, "margin-top")

	imgStyle := parseElementStyle(img)
	delete(imgStyle, "-kfx-style-name")
	delete(imgStyle, "font-size")
	delete(imgStyle, "line-height")
	iheight := imgStyle["height"]
	delete(imgStyle, "height")
	iwidth := imgStyle["width"]
	delete(imgStyle, "width")

	// Python L148: img_file = self.oebps_files.get(get_url_filename(urlabspath(img.get("src"), ref_from=book_part.filename)))
	imgSrc := img.Attrs["src"]
	imgFile := lookupImageFile(oebpsFiles, imgSrc, sectionFilename)
	if imgFile == nil {
		fmt.Fprintf(os.Stderr, "kfx: error: missing image for SVG wrapper: %s\n", imgSrc)
		return
	}

	imgHeight := imgFile.height
	imgWidth := imgFile.width

	origIntHeight := pxToInt(iheight)
	origIntWidth := pxToInt(iwidth)
	intHeight := origIntHeight
	intWidth := origIntWidth

	// Python L158: mismatch check
	if (intHeight != 0 && fixedHeight != 0 && intHeight != fixedHeight) ||
		(intWidth != 0 && fixedWidth != 0 && intWidth != fixedWidth) {
		fmt.Fprintf(os.Stderr, "kfx: error: unexpected image style for SVG wrapper (fixed h=%d, w=%d)\n", fixedHeight, fixedWidth)
	}

	// Python L163: if int_height and int_width → aspect ratio check
	if intHeight != 0 && intWidth != 0 {
		imgAspect := float64(intHeight) / float64(intWidth)
		svgAspect := float64(imgHeight) / float64(imgWidth)
		if math.Abs(imgAspect-svgAspect) > 0.01 {
			fmt.Fprintf(os.Stderr, "kfx: error: image (h=%d, w=%d) aspect ratio %f does not match SVG wrapper (h=%d, w=%d) %f\n",
				imgHeight, imgWidth, imgAspect, intHeight, intWidth, svgAspect)
		}
	} else {
		// Python L170: else → use image file dimensions
		intHeight = imgHeight
		intWidth = imgWidth
	}

	// Python L173: validate expected style properties
	textAlign, _ := divStyle["text-align"]
	delete(divStyle, "text-align")
	if textAlign == "" {
		textAlign = "center"
	}
	textIndent, _ := divStyle["text-indent"]
	delete(divStyle, "text-indent")
	if textIndent == "" {
		textIndent = "0"
	}
	position, _ := imgStyle["position"]
	delete(imgStyle, "position")
	if position == "" {
		position = "absolute"
	}
	top, _ := imgStyle["top"]
	delete(imgStyle, "top")
	if top == "" {
		top = "0"
	}
	left, _ := imgStyle["left"]
	delete(imgStyle, "left")
	if left == "" {
		left = "0"
	}

	heightOK := iheight == "" || origIntHeight != 0
	widthOK := iwidth == "" || origIntWidth != 0 || matchPercent95to100(iwidth)

	if !(textAlign == "center" && textIndent == "0" &&
		position == "absolute" && top == "0" && left == "0" &&
		heightOK && widthOK &&
		len(imgStyle) == 0 && len(divStyle) == 0) {
		fmt.Fprintf(os.Stderr, "kfx: error: unexpected image style for SVG wrapper (img h=%d, w=%d)\n", imgHeight, imgWidth)
	}

	// Replace img with SVG wrapper.
	// Python L189: svg = etree.SubElement(image_div, SVG, ...)
	svg := &htmlElement{
		Tag: "svg",
		Attrs: map[string]string{
			"version":             "1.1",
			"preserveAspectRatio": "xMidYMid meet",
			"viewBox":             fmt.Sprintf("0 0 %d %d", intWidth, intHeight),
			"height":              "100%",
			"width":               "100%",
		},
	}

	// Python L192: self.move_anchors(img, svg)
	// Transfer id attributes from img to svg
	if id, ok := img.Attrs["id"]; ok && id != "" {
		svg.Attrs["id"] = id
	}

	// Python L194: etree.SubElement(svg, image, ...)
	svgImg := &htmlElement{
		Tag: "image",
		Attrs: map[string]string{
			"xlink:href": imgSrc,
			"height":     fmt.Sprintf("%d", intHeight),
			"width":      fmt.Sprintf("%d", intWidth),
		},
	}
	svg.Children = append(svg.Children, svgImg)

	// Replace the imageDiv's img child with the SVG
	imageDiv.Children = []htmlPart{svg}
}

// matchPercent95to100 matches Python's re.match(r"^(100|9[5-9].*)%$", iwidth)
func matchPercent95to100(s string) bool {
	if !strings.HasSuffix(s, "%") {
		return false
	}
	num := strings.TrimSuffix(s, "%")
	if num == "100" {
		return true
	}
	if len(num) >= 2 && num[0] == '9' && num[1] >= '5' && num[1] <= '9' {
		return true
	}
	return false
}

// lookupImageFile resolves an image src path to an outputFile entry.
// Mirrors Python's get_url_filename(urlabspath(img.get("src"), ref_from=book_part.filename))
// followed by self.oebps_files.get(...).
func lookupImageFile(oebpsFiles map[string]*outputFile, src, refFrom string) *outputFile {
	// Resolve relative path: extract filename from src
	filename := src
	if idx := strings.LastIndex(src, "/"); idx >= 0 {
		filename = src[idx+1:]
	}
	// Also try the full src path
	if f, ok := oebpsFiles[filename]; ok {
		return f
	}
	if f, ok := oebpsFiles[src]; ok {
		return f
	}
	return nil
}

// parseElementStyle parses the "style" attribute of an htmlElement into a map.
// Returns an empty map if no style attribute exists.
func parseElementStyle(elem *htmlElement) map[string]string {
	styleStr := elem.Attrs["style"]
	if styleStr == "" {
		return map[string]string{}
	}
	return parseInlineStyle(styleStr)
}

// parseInlineStyle parses a CSS inline style string into a property map.
func parseInlineStyle(s string) map[string]string {
	result := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ":", 2)
		if len(kv) == 2 {
			result[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return result
}

// horizontalFXLBlockImages positions fixed-layout images horizontally.
// Port of Python KFX_EPUB_Misc.horizontal_fxl_block_images (yj_to_epub_misc.py L197–233).
//
// Python branch audit:
//   L199: for image_div in content_elem.findall("*") → iterate children
//   L201: if image_div.tag == "div" and len(image_div) == 1 and image_div[0].tag == "img"
//   L204: if img_file is None → error return
//   L210: if position != "absolute" or left unit not in ["","px"] or width unit not in ["","px"] → error log
//   L216: else → set styles
//   L218: if "top" not in img_style → set to 0px
//   L221: if "left" not in img_style → set to left_px
//   L224: if "height" not in img_style → set to img_file.height px
//   L227: if "width" not in img_style → set to img_file.width px
//   L231: left_px = round(left + width)
//   L233: else → error log (incorrect image content)
func horizontalFXLBlockImages(contentElem *htmlElement, oebpsFiles map[string]*outputFile, sectionFilename string) {
	leftPx := 0.0

	for _, child := range contentElem.Children {
		imageDiv, ok := child.(*htmlElement)
		if !ok {
			fmt.Fprintf(os.Stderr, "kfx: error: incorrect image content for horizontal fxl: not an element\n")
			continue
		}

		if imageDiv.Tag != "div" || len(imageDiv.Children) != 1 {
			fmt.Fprintf(os.Stderr, "kfx: error: incorrect image content for horizontal fxl: expected div>img\n")
			continue
		}

		img, ok := imageDiv.Children[0].(*htmlElement)
		if !ok || img.Tag != "img" {
			fmt.Fprintf(os.Stderr, "kfx: error: incorrect image content for horizontal fxl: expected img element\n")
			continue
		}

		// Python L204: img_file = self.oebps_files.get(...)
		imgSrc := img.Attrs["src"]
		imgFile := lookupImageFile(oebpsFiles, imgSrc, sectionFilename)
		if imgFile == nil {
			fmt.Fprintf(os.Stderr, "kfx: error: missing image for horizontal fxl: %s\n", imgSrc)
			return
		}

		imgStyle := parseElementStyle(img)

		// Python L210: validate position and units
		position := imgStyle["position"]
		if position == "" {
			position = "absolute"
		}

		_, leftUnit := splitCSSValueUnit(imgStyle["left"])
		if imgStyle["left"] == "" {
			leftUnit = ""
		}
		_, widthUnit := splitCSSValueUnit(imgStyle["width"])
		if imgStyle["width"] == "" {
			widthUnit = ""
		}

		if position != "absolute" ||
			(leftUnit != "" && leftUnit != "px") ||
			(widthUnit != "" && widthUnit != "px") {
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected image style for horizontal fxl (h=%d w=%d)\n", imgFile.height, imgFile.width)
		} else {
			// Python L217: img_style["position"] = "absolute"
			imgStyle["position"] = "absolute"

			// Python L218: if "top" not in img_style
			if _, hasTop := imgStyle["top"]; !hasTop {
				imgStyle["top"] = "0px"
			}

			// Python L221: if "left" not in img_style
			if _, hasLeft := imgStyle["left"]; !hasLeft {
				imgStyle["left"] = valueStrWithUnit(leftPx, "px", true)
			}

			// Python L224: if "height" not in img_style
			if _, hasHeight := imgStyle["height"]; !hasHeight {
				imgStyle["height"] = valueStrWithUnit(float64(imgFile.height), "px", true)
			}

			// Python L227: if "width" not in img_style
			if _, hasWidth := imgStyle["width"]; !hasWidth {
				imgStyle["width"] = valueStrWithUnit(float64(imgFile.width), "px", true)
			}

			// Apply updated style to img element
			setElementStyleFromMap(img, imgStyle)

			// Python L231: left_px = round(float(split_value(img_style["left"])[0]) + float(split_value(img_style["width"])[0]))
			finalLeft, _ := splitCSSValueUnit(imgStyle["left"])
			finalWidth, _ := splitCSSValueUnit(imgStyle["width"])
			leftPx = math.Round(finalLeft + finalWidth)
		}
	}
}

// setElementStyleFromMap updates an htmlElement's style attribute from a map.
func setElementStyleFromMap(elem *htmlElement, style map[string]string) {
	if len(style) == 0 {
		delete(elem.Attrs, "style")
		return
	}
	parts := make([]string, 0, len(style))
	for k, v := range style {
		parts = append(parts, k+": "+v)
	}
	// Sort for deterministic output
	// Simple insertion sort for small maps
	for i := 1; i < len(parts); i++ {
		for j := i; j > 0 && parts[j] < parts[j-1]; j-- {
			parts[j], parts[j-1] = parts[j-1], parts[j]
		}
	}
	elem.Attrs["style"] = strings.Join(parts, "; ")
}

// processBounds applies bound data as CSS positioning to an element.
// Port of Python KFX_EPUB_Misc.process_bounds (yj_to_epub_misc.py L590–605).
//
// Python branch audit:
//   L591: for bound, property_name in [("x","left"), ("y","top"), ("h","height"), ("w","width")]
//   L592: if bound in bounds → process
//   L594: if ion_type(bound_value) is IonStruct → process structured bound
//   L595: unit = bound_value.pop("unit")
//   L596: value = value_str(...)
//   L598: self.add_style(elem, {property_name: value}, replace=True)
//   L601: if bound in ["x", "y"] → add position: absolute
//   L604: else → error (unexpected bound data type)
func processBounds(elem *htmlElement, bounds map[string]interface{}) {
	type boundEntry struct {
		boundKey     string
		propertyName string
	}
	entries := []boundEntry{
		{"x", "left"},
		{"y", "top"},
		{"h", "height"},
		{"w", "width"},
	}

	style := parseElementStyle(elem)

	for _, entry := range entries {
		boundValue, ok := bounds[entry.boundKey]
		if !ok {
			continue
		}

		// Python L594: if ion_type(bound_value) is IonStruct
		bv, ok := asMap(boundValue)
		if !ok {
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected bound data type for %s: %T\n", entry.propertyName, boundValue)
			continue
		}

		// Python L595: unit = bound_value.pop("unit")
		unit, _ := asString(bv["unit"])
		delete(bv, "unit")

		// Python L596: value = value_str(bound_value.pop("value"), "%" if unit == "percent" else unit)
		value, _ := asFloat64(bv["value"])
		delete(bv, "value")

		cssUnit := unit
		if unit == "percent" {
			cssUnit = "%"
		}

		style[entry.propertyName] = valueStrWithUnit(value, cssUnit, true)

		// Python L601: if bound in ["x", "y"] → add position: absolute
		if entry.boundKey == "x" || entry.boundKey == "y" {
			style["position"] = "absolute"
		}
	}

	if len(style) > 0 {
		setElementStyleFromMap(elem, style)
	}
}

// processPluginURI processes a plugin URI, creating a child element in contentElem.
// Port of Python KFX_EPUB_Misc.process_plugin_uri (yj_to_epub_misc.py L562–588).
//
// Python branch audit:
//   L563: purl = urllib.parse.urlparse(uri)
//   L565: if purl.scheme == "kfx" → process plugin
//   L567: child_elem = etree.SubElement(content_elem, "plugin-temp")
//   L568: self.process_plugin(...)
//   L569: self.process_bounds(child_elem, bounds)
//   L570: else → error (unexpected scheme)
func processPluginURI(uri string, bounds map[string]interface{}, contentElem *htmlElement, rp *resourceProcessor, sectionFilename string) {
	purl, err := url.Parse(uri)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kfx: error: cannot parse plugin URI: %s\n", uri)
		return
	}

	if purl.Scheme == "kfx" {
		// Python L567: child_elem = etree.SubElement(content_elem, "plugin-temp")
		childElem := &htmlElement{Tag: "div", Attrs: map[string]string{}}

		// Python L568: self.process_plugin(urllib.parse.unquote(purl.netloc + purl.path), ...)
		resourceName, _ := url.PathUnescape(purl.Host + purl.Path)
		processPlugin(resourceName, "", childElem, rp, sectionFilename, false)

		// Python L569: self.process_bounds(child_elem, bounds)
		if bounds != nil {
			processBounds(childElem, bounds)
		}

		contentElem.Children = append(contentElem.Children, childElem)
	} else {
		fmt.Fprintf(os.Stderr, "kfx: error: unexpected plugin URI scheme: %s\n", uri)
	}
}

// processPlugin handles all plugin types.
// Port of Python KFX_EPUB_Misc.process_plugin (yj_to_epub_misc.py L235–560).
//
// Python branch audit (all branches):
//   L236: res = self.process_external_resource(resource_name, save=False, is_plugin=True)
//   L241: if is_html or res.mime == "plugin/kfx-html-article" → HTML plugin path
//     L243: src = urlrelpath(self.process_external_resource(..., save_referred=True).filename, ...)
//     L246: if RENDER_HTML_PLUGIN_AS == "iframe" → iframe
//     L254: elif RENDER_HTML_PLUGIN_AS == "object" → object
//     L263: else → anchor link
//   L277: elif res.format == "$284" → image plugin (PNG format)
//     L278: content_elem.tag = "img"
//   L282: else → manifest-based plugin processing
//     L284: manifest_raw_media = res.raw_media.decode("utf-8")
//     L286: manifest_symtab = LocalSymbolTable(...)
//     L288-290: try/except to parse ION manifest
//     L294: plugin_type = manifest_.get_annotation()
//     L295: manifest = manifest_.value
//     L298: if plugin_type == "audio" → audio plugin (L300–315)
//     L318: elif plugin_type == "button" → button plugin (L320–367)
//     L370: elif plugin_type == "hyperlink" → hyperlink plugin (L372–380)
//     L383: elif plugin_type == "image_sequence" → image_sequence (L385–391)
//     L394: elif plugin_type in ["scrollable", "slideshow"] → (L396–430)
//       L400: if manifest["properties"].get("initial_visibility") == "hide" → add visibility:hidden
//       L403: if "alt_text" in manifest["properties"] → update alt_text
//       L406: for child in manifest["facets"]["children"] → process_plugin_uri
//       L411: if plugin_type == "scrollable" → process_referred
//     L417: elif plugin_type == "video" → video plugin (L419–446)
//       L421: if user_interaction == "enabled" → controls
//       L424: if enter_view.name == "start" → autoplay
//       L427: if loop_count < 0 → loop
//       L430: if "poster" in facets → poster
//       L433: if "first_frame" in facets → save ref
//       L438: alt_text fallback
//       L441: src = uri_reference
//       L445: move anchors
//     L451: elif plugin_type == "webview" → webview (L453–462)
//       L455: save_referred
//       L457: if purl.scheme == "kfx" → recursive process_plugin with is_html=True
//       L461: else → error
//     L464: elif plugin_type == "zoomable" → zoomable (L466–471)
//       L467: img with uri
//     L473: else → unknown plugin type (L475–486)
//       L477: object element
//       L483: if len(content_elem) == 0 → alt text
func processPlugin(resourceName string, altText string, contentElem *htmlElement, rp *resourceProcessor, sectionFilename string, isHTML bool) {
	if rp == nil {
		fmt.Fprintf(os.Stderr, "kfx: error: resource processor is nil for plugin %s\n", resourceName)
		return
	}

	// Python L236: res = self.process_external_resource(resource_name, save=False, is_plugin=True)
	res := rp.processExternalResource(resourceName, false, false, false, true, false)
	if res == nil {
		fmt.Fprintf(os.Stderr, "kfx: error: plugin resource not found: %s\n", resourceName)
		return
	}

	// Python L241: if is_html or res.mime == "plugin/kfx-html-article"
	if isHTML || res.mime == "plugin/kfx-html-article" {
		// Python L243: save the resource and get relative path
		savedRes := rp.processExternalResource(resourceName, true, false, true, true, false)
		if savedRes == nil {
			return
		}
		src := urlRelPath(savedRes.filename, sectionFilename)

		// Python L246: RENDER_HTML_PLUGIN_AS == "iframe" (constant in Python)
		contentElem.Tag = "iframe"
		contentElem.Attrs["src"] = src
		setElementStyleFromMap(contentElem, map[string]string{
			"height":              "100%",
			"width":               "100%",
			"border-bottom-style": "none",
			"border-left-style":   "none",
			"border-right-style":  "none",
			"border-top-style":    "none",
		})
		return
	}

	// Python L277: elif res.format == "$284" (PNG format)
	if res.format == "png" {
		// Python L278-281: simple image
		savedRes := rp.processExternalResource(resourceName, true, false, false, false, false)
		if savedRes == nil {
			return
		}
		contentElem.Tag = "img"
		contentElem.Attrs["src"] = urlRelPath(savedRes.filename, sectionFilename)
		if altText != "" {
			contentElem.Attrs["alt"] = altText
		}
		return
	}

	// Python L282-292: manifest-based plugin processing
	pluginType, manifest := parsePluginManifest(resourceName, res.rawMedia)
	if pluginType == "" {
		return
	}

	switch pluginType {
	case "audio":
		// Python L298-315: audio plugin
		rp.processExternalResource(resourceName, false, true, false, true, false)

		contentElem.Tag = "audio"
		contentElem.Attrs["controls"] = ""

		facets, _ := asMap(manifest["facets"])
		media, _ := asMap(facets["media"])
		uri, _ := asString(media["uri"])
		if uri != "" {
			src := resolvePluginURI(uri, rp, true)
			contentElem.Attrs["src"] = urlRelPath(src, sectionFilename)
		}

		// Python L311-313: save play/pause image URIs
		player, _ := asMap(facets["player"])
		for _, imageRef := range []string{"play_images", "pause_images"} {
			if uris, ok := asSlice(player[imageRef]); ok {
				for _, u := range uris {
					uriMap, _ := asMap(u)
					if uriVal, ok := asString(uriMap["uri"]); ok {
						rp.processExternalResource(uriVal, false, false, false, false, false)
					}
				}
			}
		}

	case "button":
		// Python L318-367: button plugin
		contentElem.Tag = "div"

		facets, _ := asMap(manifest["facets"])
		images, _ := asSlice(facets["images"])
		for _, img := range images {
			imgMap, _ := asMap(img)
			role, _ := asString(imgMap["role"])
			if role != "upstate" {
				fmt.Fprintf(os.Stderr, "kfx: warning: unknown button image role %s in %s\n", role, resourceName)
			}

			// Python L335-342: RENDER_BUTTON_PLUGIN = True → always render button images
			uri, _ := asString(imgMap["uri"])
			if uri != "" {
				imgRef := resolvePluginURI(uri, rp, false)
				imgElem := &htmlElement{
					Tag:   "img",
					Attrs: map[string]string{"src": urlRelPath(imgRef, sectionFilename)},
				}
				if altText != "" {
					imgElem.Attrs["alt"] = altText
				}
				style := map[string]string{"max-width": "100%"}
				setElementStyleFromMap(imgElem, style)
				contentElem.Children = append(contentElem.Children, imgElem)
			}
		}

		// Python L347-351: validate click events
		events, _ := asMap(manifest["events"])
		clicks, _ := asSlice(events["click"])
		if clicks == nil {
			// Python L348: clicks if isinstance(clicks, list) else [clicks]
			if c, ok := asMap(events["click"]); ok {
				clicks = []interface{}{c}
			}
		}
		for _, click := range clicks {
			clickMap, _ := asMap(click)
			name, _ := asString(clickMap["name"])
			if name != "change_state" {
				fmt.Fprintf(os.Stderr, "kfx: warning: unknown button event click name %s in %s\n", name, resourceName)
			}
		}

		rp.processExternalResource(resourceName, false, true, false, true, false)

	case "hyperlink":
		// Python L370-380: hyperlink plugin
		contentElem.Tag = "a"
		setElementStyleFromMap(contentElem, map[string]string{
			"height": "100%",
			"width":  "100%",
		})

		facets, _ := asMap(manifest["facets"])
		uri, _ := asMap(facets["uri"])
		if uri != nil {
			uriStr, _ := asString(uri["uri"])
			if uriStr == "" {
				// facets["uri"] may be the URI string directly
				uriStr, _ = asString(facets["uri"])
			}
			if uriStr != "" {
				href := resolvePluginURI(uriStr, rp, false)
				contentElem.Attrs["href"] = urlRelPath(href, sectionFilename)
			}
		}

	case "image_sequence":
		// Python L383-391: image_sequence plugin
		contentElem.Tag = "div"

		facets, _ := asMap(manifest["facets"])
		images, _ := asSlice(facets["images"])
		for _, img := range images {
			imgMap, _ := asMap(img)
			uri, _ := asString(imgMap["uri"])
			if uri != "" {
				imgRef := resolvePluginURI(uri, rp, false)
				imgElem := &htmlElement{
					Tag:   "img",
					Attrs: map[string]string{
						"src": urlRelPath(imgRef, sectionFilename),
					},
				}
				if altText != "" {
					imgElem.Attrs["alt"] = altText
				}
				contentElem.Children = append(contentElem.Children, imgElem)
			}
		}

	case "scrollable", "slideshow":
		// Python L394-415: scrollable/slideshow plugins
		contentElem.Tag = "div"

		properties, _ := asMap(manifest["properties"])
		if properties != nil {
			// Python L400: if manifest["properties"].get("initial_visibility") == "hide"
			if iv, _ := asString(properties["initial_visibility"]); iv == "hide" {
				style := parseElementStyle(contentElem)
				style["visibility"] = "hidden"
				setElementStyleFromMap(contentElem, style)
			}

			// Python L403: if "alt_text" in manifest["properties"]
			if at, ok := asString(properties["alt_text"]); ok && at != "" {
				altText = at
			}
		}

		// Python L406: for child in manifest["facets"]["children"]
		facets, _ := asMap(manifest["facets"])
		children, _ := asSlice(facets["children"])
		for _, child := range children {
			childMap, _ := asMap(child)
			childURI, _ := asString(childMap["uri"])
			childBounds, _ := asMap(childMap["bounds"])
			if childURI != "" {
				processPluginURI(childURI, childBounds, contentElem, rp, sectionFilename)
			}
		}

		// Python L411: if plugin_type == "scrollable"
		if pluginType == "scrollable" {
			rp.processExternalResource(resourceName, false, true, false, true, false)
		}

	case "video":
		// Python L417-449: video plugin
		contentElem.Tag = "video"

		properties, _ := asMap(manifest["properties"])
		if properties != nil {
			// Python L421: if user_interaction == "enabled" → controls
			if ui, _ := asString(properties["user_interaction"]); ui == "enabled" {
				contentElem.Attrs["controls"] = ""
			}
		}

		// Python L424: if enter_view.name == "start" → autoplay
		events, _ := asMap(manifest["events"])
		enterView, _ := asMap(events["enter_view"])
		if enterView != nil {
			if name, _ := asString(enterView["name"]); name == "start" {
				contentElem.Attrs["autoplay"] = ""
			}
		}

		// Python L427: if loop_count < 0 → loop
		if properties != nil {
			playCtx, _ := asMap(properties["play_context"])
			if playCtx != nil {
				loopCount, _ := asInt(playCtx["loop_count"])
				if loopCount < 0 {
					contentElem.Attrs["loop"] = ""
				}
			}
		}

		facets, _ := asMap(manifest["facets"])

		// Python L430: if "poster" in facets
		poster, _ := asMap(facets["poster"])
		if poster != nil {
			posterURI, _ := asString(poster["uri"])
			if posterURI != "" {
				posterRef := resolvePluginURI(posterURI, rp, false)
				contentElem.Attrs["poster"] = urlRelPath(posterRef, sectionFilename)
			}
		}

		// Python L433: if "first_frame" in facets
		firstFrame, _ := asMap(facets["first_frame"])
		if firstFrame != nil {
			ffURI, _ := asString(firstFrame["uri"])
			if ffURI != "" {
				rp.processExternalResource(ffURI, false, false, false, false, false)
			}
		}

		// Python L438: alt_text fallback
		if altText == "" {
			altText = fmt.Sprintf("Cannot display %s content", pluginType)
		}

		// Python L441: src = uri_reference
		media, _ := asMap(facets["media"])
		if media != nil {
			mediaURI, _ := asString(media["uri"])
			if mediaURI != "" {
				src := resolvePluginURI(mediaURI, rp, true)
				contentElem.Attrs["src"] = urlRelPath(src, sectionFilename)
			}
		}

		// Python L445-448: move existing children out and move anchors back
		// This removes all existing children (placeholder content) and preserves anchor IDs
		var anchors []*htmlElement
		for _, child := range contentElem.Children {
			if elem, ok := child.(*htmlElement); ok {
				if _, hasID := elem.Attrs["id"]; hasID {
					anchors = append(anchors, elem)
				}
			}
		}
		contentElem.Children = nil
		for _, a := range anchors {
			contentElem.Children = append(contentElem.Children, a)
		}

	case "webview":
		// Python L451-462: webview plugin
		rp.processExternalResource(resourceName, false, false, true, true, false)

		facets, _ := asMap(manifest["facets"])
		uri, _ := asString(facets["uri"])
		if uri != "" {
			purl, err := url.Parse(uri)
			if err == nil && purl.Scheme == "kfx" {
				// Python L459: recursive process_plugin with is_html=True
				kfxResourceName, _ := url.PathUnescape(purl.Host + purl.Path)
				processPlugin(kfxResourceName, altText, contentElem, rp, sectionFilename, true)
			} else {
				fmt.Fprintf(os.Stderr, "kfx: error: unexpected webview plugin URI scheme: %s\n", uri)
			}
		}

	case "zoomable":
		// Python L464-471: zoomable plugin
		contentElem.Tag = "img"

		facets, _ := asMap(manifest["facets"])
		media, _ := asMap(facets["media"])
		if media != nil {
			mediaURI, _ := asString(media["uri"])
			if mediaURI != "" {
				imgRef := resolvePluginURI(mediaURI, rp, false)
				contentElem.Attrs["src"] = urlRelPath(imgRef, sectionFilename)
			}
		}
		if altText != "" {
			contentElem.Attrs["alt"] = altText
		}

	default:
		// Python L473-486: unknown plugin type
		fmt.Fprintf(os.Stderr, "kfx: error: unknown plugin type %s in resource %s\n", pluginType, resourceName)

		contentElem.Tag = "object"
		savedRes := rp.processExternalResource(resourceName, true, false, true, true, false)
		if savedRes != nil {
			contentElem.Attrs["data"] = urlRelPath(savedRes.filename, sectionFilename)
			if savedRes.mime != "" {
				contentElem.Attrs["type"] = savedRes.mime
			}
		}

		if len(contentElem.Children) == 0 {
			text := altText
			if text == "" {
				text = fmt.Sprintf("Cannot display %s content", pluginType)
			}
			contentElem.Children = []htmlPart{htmlText{Text: text}}
		}
	}
}

// parsePluginManifest parses the raw ION manifest from a plugin resource.
// Returns the plugin type annotation and the parsed manifest data.
// Port of Python yj_to_epub_misc.py L284-293.
//
// Python uses IonText to parse the manifest, but Go stores plugin data as binary ION
// (the pipeline uses amazon-ion binary decoder throughout). We decode the binary ION
// and extract the annotation (plugin type) and value (manifest data).
func parsePluginManifest(resourceName string, rawMedia []byte) (pluginType string, manifest map[string]interface{}) {
	if len(rawMedia) == 0 {
		fmt.Fprintf(os.Stderr, "kfx: error: empty plugin manifest for %s\n", resourceName)
		return "", nil
	}

	// Decode binary ION data
	parsed, err := decodeIonValue(rawMedia, nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kfx: error: exception processing plugin %s: %v\n", resourceName, err)
		return "", nil
	}

	// Python L294: plugin_type = manifest_.get_annotation()
	// Python L295: manifest = manifest_.value
	// In our ION representation, the annotation is stored in the parsed result.
	// For plugin manifests, the data is typically a struct (map).
	switch p := parsed.(type) {
	case map[string]interface{}:
		manifest = p
		// Check for plugin_type stored as a key in the struct
		if pt, ok := asString(p["plugin_type"]); ok {
			pluginType = pt
			delete(p, "plugin_type")
		} else if pt, ok := asString(p["type"]); ok {
			pluginType = pt
			delete(p, "type")
		}
	default:
		fmt.Fprintf(os.Stderr, "kfx: error: unexpected plugin manifest type for %s: %T\n", resourceName, parsed)
		return "", nil
	}

	return pluginType, manifest
}

// resolvePluginURI resolves a plugin URI to a resource path.
// When manifestExternalRefs is true, the resource is saved (process_referred).
// Port of Python self.uri_reference(uri, manifest_external_refs=True).
func resolvePluginURI(uri string, rp *resourceProcessor, manifestExternalRefs bool) string {
	// The uri is typically a kfx:// or resource reference
	// Try to resolve it as a resource name
	if rp != nil && manifestExternalRefs {
		res := rp.processExternalResource(uri, true, false, true, false, false)
		if res != nil && res.filename != "" {
			return res.filename
		}
	}
	if rp != nil && !manifestExternalRefs {
		res := rp.processExternalResource(uri, false, false, false, false, false)
		if res != nil && res.filename != "" {
			return res.filename
		}
	}
	return uri
}

// urlRelPath returns a relative path from refFrom to target.
// Port of Python urlrelpath(target, ref_from=book_part.filename).
func urlRelPath(target, refFrom string) string {
	if target == "" {
		return ""
	}
	// Simple relative path: if target is already relative, return as-is
	if !strings.HasPrefix(target, "/") {
		return target
	}
	// If refFrom has a directory component, compute relative path
	dir := ""
	if idx := strings.LastIndex(refFrom, "/"); idx >= 0 {
		dir = refFrom[:idx+1]
	}
	if strings.HasPrefix(target, dir) && dir != "" {
		return target[len(dir):]
	}
	return target
}

// ---------------------------------------------------------------------------
// Merged from svg.go (origin: yj_to_epub_misc.py KVG/SVG rendering)
// ---------------------------------------------------------------------------


func adjustPixelValue(value float64) float64 {
	return value
}

func processKVGShape(parent *htmlElement, shape map[string]interface{}, writingMode string) {
	shapeType, _ := asString(shape["type"])
	delete(shape, "type")

	var elem *htmlElement

	switch shapeType {
	case "shape":
		pathData := shape["path"]
		delete(shape, "path")
		d := processPath(pathData)
		elem = &htmlElement{
			Tag:   "path",
			Attrs: map[string]string{"d": d},
		}
		parent.Children = append(parent.Children, elem)

	case "container":
		source, _ := asString(shape["source"])
		delete(shape, "source")
		if source == "" {
			fmt.Fprintf(os.Stderr, "kfx: error: missing KVG container content source\n")
			return
		}
		elem = &htmlElement{
			Tag:   "text",
			Attrs: map[string]string{},
		}
		parent.Children = append(parent.Children, elem)

	default:
		if shapeType != "" {
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected shape type: %s\n", shapeType)
		}
		return
	}

	svgAttrs := [][2]string{
		{"fill_color", "fill"},
		{"fill_opacity", "fill-opacity"},
		{"stroke_color", "stroke"},
		{"stroke_dasharray", "stroke-dasharray"},
		{"stroke_dashoffset", "stroke-dashoffset"},
		{"stroke_linecap", "stroke-linecap"},
		{"stroke_linejoin", "stroke-linejoin"},
		{"stroke_miterlimit", "stroke-miterlimit"},
		{"stroke_width", "stroke-width"},
		{"transform", "transform"},
	}

	for _, attr := range svgAttrs {
		yjPropName := attr[0]
		svgAttrib := attr[1]
		if val, ok := shape[yjPropName]; ok {
			delete(shape, yjPropName)
			elem.Attrs[svgAttrib] = propertyValueSVG(yjPropName, val)
		}
	}

	if _, hasStroke := elem.Attrs["stroke"]; hasStroke {
		if _, hasFill := elem.Attrs["fill"]; !hasFill {
			elem.Attrs["fill"] = "none"
		}
	}
}

func propertyValueSVG(propName string, yjValue interface{}) string {
	switch v := yjValue.(type) {
	case float64:
		if colorYJProperties[propName] {
			return fixColorValue(v)
		}
		return valueStr(v)
	case *float64:
		if v == nil {
			return ""
		}
		return propertyValueSVG(propName, *v)
	case int:
		return propertyValueSVG(propName, float64(v))
	case int64:
		return propertyValueSVG(propName, float64(v))
	case []interface{}:
		if propName == "transform" {
			return processTransform(v, true)
		}
		return propertyValue(propName, yjValue, nil)
	default:
		return propertyValue(propName, yjValue, nil)
	}
}

func processPath(path interface{}) string {
	if m, ok := asMap(path); ok {
		bundleName, _ := asString(m["name"])
		pathIndex, _ := asInt(m["index"])
		_ = bundleName
		_ = pathIndex
		fmt.Fprintf(os.Stderr, "kfx: error: path bundle lookup not yet implemented: %s\n", bundleName)
		return ""
	}

	p, ok := asSlice(path)
	if !ok {
		return ""
	}

	d := []string{}
	remaining := make([]interface{}, len(p))
	copy(remaining, p)

	processInstruction := func(inst string, nArgs int) {
		d = append(d, inst)
		for j := 0; j < nArgs; j++ {
			if len(remaining) == 0 {
				fmt.Fprintf(os.Stderr, "kfx: error: incomplete path instruction in %v\n", p)
				return
			}
			v := remaining[0]
			remaining = remaining[1:]
			vf, ok := asFloat64(v)
			if ok {
				v = adjustPixelValue(vf)
			}
			d = append(d, valueStr(v))
		}
	}

	for len(remaining) > 0 {
		inst := remaining[0]
		remaining = remaining[1:]
		instInt, ok := asInt(inst)
		if !ok {
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected path instruction %v in %v\n", inst, p)
			break
		}
		switch instInt {
		case 0:
			processInstruction("M", 2)
		case 1:
			processInstruction("L", 2)
		case 2:
			processInstruction("Q", 4)
		case 3:
			processInstruction("C", 6)
		case 4:
			processInstruction("Z", 0)
		default:
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected path instruction %d in %v\n", instInt, p)
			break
		}
	}

	return strings.Join(d, " ")
}

func processPolygon(path []interface{}) string {
	percentValueStr := func(v interface{}) string {
		f, ok := asFloat64(v)
		if !ok {
			return valueStr(v) + "%"
		}
		return valueStr(f*100) + "%"
	}

	d := []string{}
	i := 0
	ln := len(path)
	for i < ln {
		inst, ok := asInt(path[i])
		if !ok {
			fmt.Fprintf(os.Stderr, "kfx: error: bad path instruction in %v\n", path)
			break
		}
		switch inst {
		case 0, 1:
			if i+3 > ln {
				fmt.Fprintf(os.Stderr, "kfx: error: bad path instruction in %v\n", path)
				return fmt.Sprintf("polygon(%s)", strings.Join(d, ", "))
			}
			d = append(d, fmt.Sprintf("%s %s", percentValueStr(path[i+1]), percentValueStr(path[i+2])))
			i += 3
		case 4:
			i++
		default:
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected path instruction %d in %v\n", inst, path)
			break
		}
	}

	return fmt.Sprintf("polygon(%s)", strings.Join(d, ", "))
}

func processTransform(vals []interface{}, svg bool) string {
	var px, sep string
	if svg {
		px = ""
		sep = " "
	} else {
		px = "px"
		sep = ","
	}

	if len(vals) != 6 {
		fmt.Fprintf(os.Stderr, "kfx: error: unexpected transform: %v\n", vals)
		return "?"
	}

	v := make([]float64, 6)
	for i, val := range vals {
		f, ok := asFloat64(val)
		if !ok {
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected transform value: %v\n", vals)
			return "?"
		}
		v[i] = f
	}

	v[4] = adjustPixelValue(v[4])
	v[5] = adjustPixelValue(v[5])

	var translate string
	if v[4] == 0 && v[5] == 0 {
		translate = ""
	} else {
		translate = fmt.Sprintf("translate(%s%s%s) ", svgValueStr(v[4], px), sep, svgValueStr(v[5], px))
	}

	if v[0] == 1 && v[1] == 0 && v[2] == 0 && v[3] == 1 && translate != "" {
		return strings.TrimSpace(translate)
	}

	if v[1] == 0 && v[2] == 0 {
		if v[0] == v[3] {
			return translate + fmt.Sprintf("scale(%s)", valueStr(v[0]))
		}
		return translate + fmt.Sprintf("scale(%s%s%s)", valueStr(v[0]), sep, valueStr(v[3]))
	}

	if v[0] == 0 && v[1] == 1 && v[2] == -1 && v[3] == 0 {
		return translate + "rotate(-90deg)"
	}

	if v[0] == 0 && v[1] == -1 && v[2] == 1 && v[3] == 0 {
		return translate + "rotate(90deg)"
	}

	if v[0] == -1 && v[1] == 0 && v[2] == 0 && v[3] == -1 {
		return translate + "rotate(180deg)"
	}

	fmt.Fprintf(os.Stderr, "kfx: warning: unexpected transform matrix: %v\n", vals)
	strVals := make([]string, 6)
	for i, val := range v {
		strVals[i] = valueStr(val)
	}
	return fmt.Sprintf("matrix(%s)", strings.Join(strVals, sep))
}

func svgValueStr(v float64, unit string) string {
	s := valueStr(v)
	if unit != "" && s != "0" {
		return s + unit
	}
	return s
}
