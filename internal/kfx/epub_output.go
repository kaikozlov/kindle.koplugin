package kfx

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"math"
	"strconv"
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
)

func applyCoverSVGPromotion(book *decodedBook, resolvedDefaultFont string) {
	if book == nil || book.CoverImageHref == "" {
		return
	}
	width, height := coverImageDimensions(book.Resources, book.CoverImageHref)
	if width == 0 || height == 0 {
		return
	}
	coverFound := false
	for index := range book.Sections {
		section := &book.Sections[index]
		// Match cover section by either title or containing the cover image.
		// Calibre identifies cover in process_section via layout + image, not title alone.
		if !strings.Contains(section.BodyHTML, `src="`+book.CoverImageHref+`"`) {
			continue
		}
		// Only promote sections that are primarily a cover image (not mixed content).
		if section.Title != "Cover" && !isCoverImageSection(section.BodyHTML) {
			continue
		}
		coverFound = true
		section.Properties = "svg"
		section.BodyHTML = fmt.Sprintf(
			`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" version="1.1" preserveAspectRatio="xMidYMid meet" viewBox="0 0 %d %d" height="100%%" width="100%%"><image xlink:href="%s" height="%d" width="%d"/></svg>`,
			width, height, escapeHTML(book.CoverImageHref), height, width,
		)
		// Python adds class_s8 with font-family only when the resolved default font is
		// not "serif" (the CSS heritable default). When the default is just "serif",
		// Python's set_html_defaults skips cover pages and no font-family is emitted.
		// Match Python behavior: only add class_s8 when a non-generic font is used.
		if resolvedDefaultFont != "serif" {
			section.BodyClass = "class_s8"
		} else {
			section.BodyClass = ""
		}
		break
	}
	if !coverFound {
		return
	}
	// Add the class_s8 CSS rule only when using a non-generic default font.
	// Python's cover sections only get font-family when the resolved default is not "serif".
	if resolvedDefaultFont == "serif" {
		return
	}
	classS8Rule := ".class_s8 {font-family: " + resolvedDefaultFont + "}"
	if !strings.Contains(book.Stylesheet, ".class_s8 {") {
		if book.Stylesheet != "" {
			book.Stylesheet += "\n"
		}
		book.Stylesheet += classS8Rule
	} else {
		lines := strings.Split(book.Stylesheet, "\n")
		for index, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), ".class_s8 {") {
				lines[index] = classS8Rule
			}
		}
		book.Stylesheet = strings.Join(lines, "\n")
	}
}

func coverImageDimensions(resources []epub.Resource, href string) (int, int) {
	for _, resource := range resources {
		if resource.Filename != href {
			continue
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(resource.Data))
		if err != nil {
			return 0, 0
		}
		return cfg.Width, cfg.Height
	}
	return 0, 0
}

// isCoverImageSection returns true if the body HTML is primarily just an image
// (possibly wrapped in a div), suitable for SVG cover promotion.
func isCoverImageSection(bodyHTML string) bool {
	stripped := strings.TrimSpace(bodyHTML)
	// Remove opening/closing div wrapper
	stripped = strings.TrimPrefix(stripped, "<div>")
	stripped = strings.TrimSuffix(stripped, "</div>")
	stripped = strings.TrimSpace(stripped)
	return strings.HasPrefix(stripped, "<img") && !strings.Contains(stripped, "<p>") && !strings.Contains(stripped, "<h")
}

func normalizeBookIdentifier(identifier string) string {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return trimmed
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "urn:asin:") {
		return trimmed
	}
	return "urn:asin:" + trimmed
}

func normalizeLanguage(language string) string {
	trimmed := strings.TrimSpace(language)
	if trimmed == "" {
		return "en"
	}
	if len(trimmed) > 2 && trimmed[2] == '_' {
		trimmed = strings.ReplaceAll(trimmed, "_", "-")
	}
	prefix, suffix, found := strings.Cut(trimmed, "-")
	if !found {
		return strings.ToLower(trimmed)
	}
	prefix = strings.ToLower(prefix)
	if len(suffix) < 4 {
		suffix = strings.ToUpper(suffix)
	} else {
		suffix = strings.ToUpper(suffix[:1]) + suffix[1:]
	}
	return prefix + "-" + suffix
}

// promoteCoverSectionFromGuide moves the cover section to the front of the section order.
// Port of epub_output.py identify_cover which expects the cover page to be first in the book.
func promoteCoverSectionFromGuide(sections []string, guideEntries []guideEntry, positionToSection map[int]string) []string {
	if len(sections) == 0 || len(guideEntries) == 0 {
		return sections
	}
	// Find cover section from guide entry.
	var coverSectionID string
	for _, entry := range guideEntries {
		if entry.Type == "cover" && entry.Target.PositionID != 0 {
			coverSectionID = positionToSection[entry.Target.PositionID]
			break
		}
	}
	if coverSectionID == "" {
		return sections
	}
	// Check if already first.
	if len(sections) > 0 && sections[0] == coverSectionID {
		return sections
	}
	// Move cover section to front.
	result := make([]string, 0, len(sections))
	result = append(result, coverSectionID)
	for _, id := range sections {
		if id != coverSectionID {
			result = append(result, id)
		}
	}
	return result
}
func beautifyHTML(root *htmlElement) {
	if root == nil {
		return
	}

	// Phase 1: Strip empty <span> tags (Python beautify_html, epub_output.py)
	var walkSpans func(elem *htmlElement)
	walkSpans = func(elem *htmlElement) {
		if elem == nil {
			return
		}
		newChildren := make([]htmlPart, 0, len(elem.Children))
		for _, child := range elem.Children {
			ch, ok := child.(*htmlElement)
			if !ok {
				newChildren = append(newChildren, child)
				continue
			}
			if ch.Tag == "span" && len(ch.Attrs) == 0 {
				newChildren = append(newChildren, ch.Children...)
			} else {
				newChildren = append(newChildren, child)
			}
		}
		elem.Children = newChildren
		for _, child := range elem.Children {
			if el, ok := child.(*htmlElement); ok {
				walkSpans(el)
			}
		}
	}
	walkSpans(root)

	// Phase 2: Strip empty <div> wrappers (Python beautify_html, epub_output.py:796-812)
	// A <div> with no attributes is stripped when:
	// - Parent is in {aside, caption, div, figure, h1-h6, li, p, td} AND parent has 1 child AND parent has no text
	// - Parent is <body> AND body has no text AND all children are block-level elements without tails
	// This is a loop because stripping one div may enable stripping another.
	divStripParents := map[string]bool{
		"aside": true, "caption": true, "div": true, "figure": true,
		"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
		"li": true, "p": true, "td": true,
	}
	blockTags := map[string]bool{
		"aside": true, "div": true, "figure": true,
		"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
		"hr": true, "iframe": true, "ol": true, "p": true, "table": true, "ul": true,
	}

	for {
		changed := false
		var stripDivs func(elem *htmlElement)
		stripDivs = func(elem *htmlElement) {
			if elem == nil {
				return
			}
			for _, child := range elem.Children {
				ch, ok := child.(*htmlElement)
				if !ok {
					continue
				}
				stripDivs(ch)
			}

			// Look for empty <div> children to strip
			newChildren := make([]htmlPart, 0, len(elem.Children))
			for _, child := range elem.Children {
				ch, ok := child.(*htmlElement)
				if !ok {
					newChildren = append(newChildren, child)
					continue
				}
				if ch.Tag == "div" && len(ch.Attrs) == 0 && shouldStripDiv(elem, ch, divStripParents, blockTags) {
					// Strip: promote div's children into parent
					newChildren = append(newChildren, ch.Children...)
					changed = true
				} else {
					newChildren = append(newChildren, child)
				}
			}
			elem.Children = newChildren
		}
		stripDivs(root)
		if !changed {
			break
		}
	}
}

// shouldStripDiv checks if an empty-attribute <div> should be stripped from its parent,
// matching Python's beautify_html logic (epub_output.py:796-812).

func shouldStripDiv(parent, div *htmlElement, divStripParents, blockTags map[string]bool) bool {
	if divStripParents[parent.Tag] {
		// Parent must have exactly 1 child (the div itself) and no text content
		if len(parent.Children) != 1 {
			return false
		}
		return true
	}
	if parent.Tag == "body" {
		// Special body case: strip div if all div children are block-level elements
		// and none of them have tail text.
		for _, child := range div.Children {
			ch, ok := child.(*htmlElement)
			if !ok {
				return false
			}
			if !blockTags[ch.Tag] {
				return false
			}
		}
		// Check no tail text after div in parent's children
		for i, sib := range parent.Children {
			if sib == div {
				// Check for tail text after the div
				if i+1 < len(parent.Children) {
					if t, ok := parent.Children[i+1].(*htmlText); ok && strings.TrimSpace(t.Text) != "" {
						return false
					}
				}
				break
			}
		}
		return true
	}
	return false
}

// Port of KFX_EPUB_Properties.create_css_files (yj_to_epub_properties.py L2239+).
// Python writes STYLES_CSS_FILEPATH from font_faces + css_rules + media_queries; Go appends the style catalog
// string (dynamic rules from render) onto book.Stylesheet before finalizeStylesheet (render.go).

func trimFloat(value float64) string {
	// Same formatting as formatStyleNumber but without units.
	return formatStyleNumber(value)
}


func formatStyleNumber(value float64) string {
	// Port of Python epub_output.py value_str formatting logic.
	if math.Abs(value) < 1e-6 {
		return "0"
	}
	s := strconv.FormatFloat(value, 'g', 6, 64)
	// Fall back to fixed-point if scientific notation appeared.
	if strings.ContainsAny(s, "eE") {
		s = strconv.FormatFloat(value, 'f', 4, 64)
	}
	// Strip trailing zeros then trailing decimal point.
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}


func valueStr(v interface{}) string {
	// Python: if quantity is None → return "" (epub_output.py:1374-1375)
	if v == nil {
		return ""
	}
	switch n := v.(type) {
	case float64:
		return formatCSSQuantity(n)
	case *float64:
		if n == nil {
			return ""
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

	// Rule 2: format with %g (6 significant digits, matching Python "%g" % quantity)
	s := strconv.FormatFloat(q, 'g', 6, 64)

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

