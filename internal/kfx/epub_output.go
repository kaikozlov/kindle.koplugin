package kfx

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"log"
	"math"
	"regexp"
	"sort"
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
// consolidateHTML merges adjacent inline elements with identical attributes and
// strips empty <span> tags with no attributes. Ported from Python's consolidate_html
// (epub_output.py L742-775).
//
// The merge logic:
//  1. For each direct child of <body>, iterate descendants looking for inline elements
//     (a, b, em, i, span, strong, sub, sup, u) that have a next sibling with the same
//     tag and identical attributes.
//  2. If the current element has no "tail" (no text between its closing tag and the
//     next sibling), merge the next sibling into it: move text, children, and tail.
//  3. Repeat until no more merges are possible.
//  4. Strip empty <span> elements with no attributes.
func consolidateHTML(body *htmlElement) {
	if body == nil {
		return
	}

	// Phase 1: Merge adjacent inline elements with identical attributes.
	// Python: epub_output.py L743-770.
	inlineTags := map[string]bool{
		"a": true, "b": true, "em": true, "i": true,
		"span": true, "strong": true, "sub": true, "sup": true, "u": true,
	}

	for _, topChild := range body.Children {
		topElem, ok := topChild.(*htmlElement)
		if !ok {
			continue
		}
		// Python: `changed = True; while changed:` — retry until stable
		for {
			changed := false
			// Walk all descendants depth-first (Python: toptag.iterdescendants())
			consolidateWalk(topElem, inlineTags, &changed)
			if !changed {
				break
			}
		}
	}

	// Phase 2: Strip empty <span> tags with no attributes.
	// Python: epub_output.py L773-775 — uses TEMP_TAG + strip_tags.
	// Note: beautifyHTML's Phase 1 already strips ALL <span> tags with no attributes
	// (not just empty ones), so this phase is partially redundant. However, we
	// implement it faithfully to handle the case where consolidateHTML is called
	// without beautifyHTML. We only strip truly empty spans (no children) to match
	// Python's strip_tags behavior which unwraps spans but preserves their content.
	stripEmptySpansNoAttrs(body)
}

// consolidateWalk walks all descendants of elem looking for mergeable inline elements.
// When a merge happens, sets *changed to true and returns immediately (Python's
// `if changed: break` pattern).
func consolidateWalk(elem *htmlElement, inlineTags map[string]bool, changed *bool) {
	for _, child := range elem.Children {
		childElem, ok := child.(*htmlElement)
		if !ok {
			continue
		}
		if !inlineTags[childElem.Tag] {
			// Not an inline element — recurse into its children
			consolidateWalk(childElem, inlineTags, changed)
			if *changed {
				return
			}
			continue
		}

		// This is an inline element. Try to merge with next siblings.
		// We need the parent to manipulate children.
		consolidateTryMerge(childElem, elem, inlineTags, changed)
		if *changed {
			return
		}
		// Also recurse into this element's children
		consolidateWalk(childElem, inlineTags, changed)
		if *changed {
			return
		}
	}
}

// consolidateTryMerge tries to merge consecutive siblings of the same tag and
// attributes into childElem. parent is childElem's parent.
func consolidateTryMerge(childElem, parent *htmlElement, inlineTags map[string]bool, changed *bool) {
	for {
		// Find the index of childElem in parent's Children
		childIdx := -1
		for i, c := range parent.Children {
			if c == childElem {
				childIdx = i
				break
			}
		}
		if childIdx < 0 || childIdx+1 >= len(parent.Children) {
			return // no next sibling
		}

		// Python: `while (not e.tail) and (n is not None) and n.tag == e.tag
		//           and sorted(attrs) == sorted(attrs)`
		//
		// In Go's model, "e.tail" is represented as a text htmlPart immediately
		// after the element in the parent's Children array. If the next part is a
		// text node (not an element), that's equivalent to Python's e.tail being set,
		// so we stop merging.
		nextPart := parent.Children[childIdx+1]
		nextElem, ok := nextPart.(*htmlElement)
		if !ok {
			// Next sibling is text — equivalent to e.tail being set in Python.
			// Python condition `not e.tail` is false, so stop.
			return
		}

		// Check: same tag and identical attributes
		if nextElem.Tag != childElem.Tag {
			return
		}
		if !attrsEqual(childElem.Attrs, nextElem.Attrs) {
			return
		}

		// Merge nextElem into childElem.
		// Python L753-762: merge n.text into e, move n's children to e.
		mergeInto(childElem, nextElem)

		// Remove nextElem from parent. Python: `n.getparent().remove(n)`
		parent.Children = append(parent.Children[:childIdx+1], parent.Children[childIdx+2:]...)

		// Python L764: `if n.tail: e.tail = n.tail`
		// In our model, nextElem's "tail" would be a text node after nextElem
		// in parent's Children. Since we removed nextElem, any text that was after
		// it is now after childElem, which is correct. No action needed — the text
		// part naturally follows childElem now.

		*changed = true
		// Continue the while loop — check next sibling after childElem
	}
}

// mergeInto merges src element's text and children into dst element.
// Python: epub_output.py L753-762.
func mergeInto(dst, src *htmlElement) {
	// Python L753-757: merge n.text into e
	// In our model, src's "text" is its first child if it's a htmlText.
	srcText := ""
	if len(src.Children) > 0 {
		if txt, ok := src.Children[0].(htmlText); ok {
			srcText = txt.Text
			src.Children = src.Children[1:]
		} else if txt, ok := src.Children[0].(*htmlText); ok {
			srcText = txt.Text
			src.Children = src.Children[1:]
		}
	}

	if srcText != "" {
		if len(dst.Children) > 0 {
			// Python L755: `tt = e[-1]; tt.tail = (tt.tail + n.text) if tt.tail else n.text`
			// Append to the last child's text
			lastChild := dst.Children[len(dst.Children)-1]
			switch lc := lastChild.(type) {
			case *htmlText:
				lc.Text += srcText
			case htmlText:
				// htmlText is a value type — need to replace
				dst.Children[len(dst.Children)-1] = htmlText{Text: lc.Text + srcText}
			case *htmlElement:
				// Python appends to last child's tail. In our model, append a text node.
				dst.Children = append(dst.Children, htmlText{Text: srcText})
			}
		} else {
			// Python L757: `e.text = (e.text + n.text) if e.text else n.text`
			dst.Children = append(dst.Children, htmlText{Text: srcText})
		}
	}

	// Python L759-762: move all children from n to e
	for _, c := range src.Children {
		dst.Children = append(dst.Children, c)
	}
	src.Children = nil
}

// attrsEqual compares two attribute maps for equality.
func attrsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// stripEmptySpansNoAttrs removes <span> elements with no attributes and no children.
// Python: epub_output.py L773-775 — `e.tag = TEMP_TAG; etree.strip_tags(body, TEMP_TAG)`.
func stripEmptySpansNoAttrs(elem *htmlElement) {
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
		if ch.Tag == "span" && len(ch.Attrs) == 0 && len(ch.Children) == 0 {
			// Skip this empty span
			continue
		}
		newChildren = append(newChildren, child)
	}
	elem.Children = newChildren
	for _, child := range elem.Children {
		if el, ok := child.(*htmlElement); ok {
			stripEmptySpansNoAttrs(el)
		}
	}
}

// ---------------------------------------------------------------------------
// compareFixedLayoutViewports — Python epub_output.py L614-653
//
// Examines fixed-layout book parts, determines the most common viewport
// dimensions, and logs warnings about conflicting aspect ratios.
//
// In Python, this scans book_part.html → <head> → <meta name="viewport">
// for width/height patterns. In Go, the rendered sections have a Root
// htmlElement tree that may contain <meta> elements with viewport data
// (e.g., from notebook page sections or future fixed-layout rendering).
//
// The function sets book.OriginalWidth and book.OriginalHeight from the
// most common viewport. It also warns about:
//   - Viewports smaller than 100px in either dimension
//   - Conflicting aspect ratios across book parts
//
// Port of Python compare_fixed_layout_viewports (epub_output.py L614-653).
// ---------------------------------------------------------------------------

var (
	viewportWidthRe  = regexp.MustCompile(`width=([0-9]+)`)
	viewportHeightRe = regexp.MustCompile(`height=([0-9]+)`)
)

// viewportKey is a (width, height) pair used as a map key.
type viewportKey struct {
	width  int
	height int
}

// viewportEntry holds a viewport dimension with its count.
type viewportEntry struct {
	key   viewportKey
	count int
}

// compareFixedLayoutViewports scans rendered sections for viewport meta tags,
// determines the most common viewport dimensions, and warns about conflicts.
// Port of Python epub_output.py L614-653.
//
// Python references:
//
//	L615: viewport_count = collections.defaultdict(int)
//	L616: cover_width, cover_height = None, None
//	L617: for book_part in self.book_parts:
//	L619: head = book_part.html.find("head")
//	L620-621: for meta in head.iterfind("meta"): if meta.get("name") == "viewport"
//	L623-624: regex extract width/height from content attribute
//	L625-632: count viewports, track cover dimensions, warn about small viewports
//	L634-636: sort by count, set original_width/height
//	L638-652: detect conflicting aspect ratios
func compareFixedLayoutViewports(book *decodedBook) {
	if book == nil || !book.FixedLayout {
		return
	}
	// Python L615-616
	viewportCount := make(map[viewportKey]int)
	var coverWidth, coverHeight int

	// Python L617: for book_part in self.book_parts:
	for i := range book.RenderedSections {
		section := &book.RenderedSections[i]
		if section.Root == nil {
			continue
		}

		// Python L619: head = book_part.html.find("head")
		// Python L620-621: for meta in head.iterfind("meta"): if meta.get("name") == "viewport"
		walkHTMLElement(section.Root, func(elem *htmlElement) {
			if elem.Tag != "meta" {
				return
			}
			if elem.Attrs["name"] != "viewport" {
				return
			}

			// Python L622-624: extract width and height from content attribute
			content := elem.Attrs["content"]
			if content == "" {
				return
			}
			mw := viewportWidthRe.FindStringSubmatch(content)
			mh := viewportHeightRe.FindStringSubmatch(content)
			if mw == nil || mh == nil {
				return
			}

			// Python L626-627
			width, errW := strconv.Atoi(mw[1])
			height, errH := strconv.Atoi(mh[1])
			if errW != nil || errH != nil {
				return
			}

			// Python L628: viewport_count[(width, height)] += 1
			vk := viewportKey{width: width, height: height}
			viewportCount[vk]++

			// Python L629-630: if book_part.is_cover_page: cover_width, cover_height = width, height
			// In Go, cover section is identified by being the first section with a cover image.
			if i == 0 && book.CoverImageHref != "" {
				coverWidth = width
				coverHeight = height
			}

			// Python L631-632: if width < 100 or height < 100:
			if width < 100 || height < 100 {
				log.Printf("kfx: warning: Fixed-layout viewport %s is too small: %s", section.Filename, content)
			}
		})
	}

	// Python L634: if len(viewport_count) > 0:
	if len(viewportCount) == 0 {
		return
	}

	// Python L635: viewports_by_count = sorted(viewport_count.items(), key=lambda x: -x[1])
	entries := make([]viewportEntry, 0, len(viewportCount))
	for k, count := range viewportCount {
		entries = append(entries, viewportEntry{key: k, count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].count > entries[j].count
	})

	// Python L636: (self.original_width, self.original_height), best_count = viewports_by_count[0]
	best := entries[0]
	book.OriginalWidth = best.key.width
	book.OriginalHeight = best.key.height

	// Python L638: if len(viewports_by_count) > 1:
	if len(entries) < 2 {
		return
	}

	// Python L639-640: best_aspect_ratio and conflicts list
	bestAspectRatio := float64(book.OriginalWidth) / float64(nonZero(book.OriginalHeight))
	type conflictEntry struct {
		count       int
		aspectRatio float64
		width       int
		height      int
	}
	conflicts := []conflictEntry{
		{best.count, bestAspectRatio, book.OriginalWidth, book.OriginalHeight},
	}

	// Python L642-649: check each viewport against best
	// Python: is_comic check uses self.is_comic. Go uses bookType from detectBookType.
	isComic := false
	bt := detectBookTypeFromBook(book)
	if bt == bookTypeComic {
		isComic = true
	}

	for _, entry := range entries {
		w, h := entry.key.width, entry.key.height
		ar := float64(w) / float64(nonZero(h))

		// Python L644-648: skip if matching or comic exception
		if aspectRatioMatch(bestAspectRatio, ar) ||
			aspectRatioMatch(bestAspectRatio, ar*2) ||
			aspectRatioMatch(bestAspectRatio, ar/2) ||
			(isComic && w == coverWidth && h == coverHeight && entry.count == 1) {
			continue
		}
		conflicts = append(conflicts, conflictEntry{entry.count, ar, w, h})
	}

	// Python L651-652: if len(conflicts) > 1:
	if len(conflicts) > 1 {
		parts := make([]string, len(conflicts))
		for i, c := range conflicts {
			parts[i] = fmt.Sprintf("%d @ %f (%dw x %dh)", c.count, c.aspectRatio, c.width, c.height)
		}
		log.Printf("kfx: info: Conflicting viewport aspect ratios: %s", strings.Join(parts, ", "))
	}
}

// aspectRatioMatch returns true if two aspect ratios are within 1.5% of each other.
// Port of Python aspect_ratio_match (epub_output.py L1365-1366).
func aspectRatioMatch(ratio1, ratio2 float64) bool {
	if ratio1 == 0 {
		return ratio2 == 0
	}
	return math.Abs(ratio1-ratio2)/ratio1 <= 0.015
}

// nonZero returns n if non-zero, else 1 (avoids division by zero).
func nonZero(n int) int {
	if n == 0 {
		return 1
	}
	return n
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
// ---------------------------------------------------------------------------
// checkEpubVersion — Python epub_output.py L654-684
//
// Determines whether to generate EPUB2 or EPUB3 output based on content features.
// Called after book parts are created but before EPUB assembly.
//
// The function checks:
//  1. If EPUB2 was not desired (generate_epub2 is already false), return immediately
//  2. Fixed layout, author/title pronunciations → require EPUB3
//  3. Certain file mimetypes (application/octet-stream, application/xml,
//     text/javascript, text/html) → require EPUB3
//  4. Nav or FXL book parts → require EPUB3
//  5. EPUB3-only HTML tags (article, aside, audio, etc.) → require EPUB3
//  6. data-* attributes or epub:type/epub:prefix attributes → require EPUB3
//
// In practice, the Go converter always sets Epub2Desired=false (EPUB3 by default),
// so this function will always return false (generate EPUB3). It is ported for
// completeness and correctness matching Python's behavior.
//
// Port of Python check_epub_version (epub_output.py L654-684).
// ---------------------------------------------------------------------------

// epub3OnlyTags lists HTML tags that only exist in EPUB3 (HTML5).
// Python epub_output.py L674-677 — if elem.tag in {...}: self.generate_epub2 = False
var epub3OnlyTags = map[string]bool{
	"article": true, "aside": true, "audio": true, "bdi": true, "canvas": true,
	"details": true, "dialog": true, "embed": true, "figcaption": true,
	"figure": true, "footer": true, "header": true, "main": true, "mark": true,
	"meter": true, "nav": true, "picture": true, "progress": true, "rt": true,
	"ruby": true, "section": true, "source": true, "summary": true,
	"template": true, "time": true, "track": true, "video": true, "wbr": true,
}

// epub3Mimetypes lists MIME types that require EPUB3.
// Python epub_output.py L662-663 — if oebps_file.mimetype in [...]: self.generate_epub2 = False
var epub3Mimetypes = map[string]bool{
	"application/octet-stream": true,
	"application/xml":         true,
	"text/javascript":         true,
	"text/html":               true,
}

// checkEpubVersion determines whether the book content requires EPUB3.
// Returns true if EPUB2 generation is acceptable, false if EPUB3 is required.
//
// Python references:
//
//	L655: if not self.generate_epub2: return
//	L657-658: if self.fixed_layout or self.author_pronunciations or self.title_pronunciation: return False
//	L661-663: for oebps_file in self.oebps_files.values(): if mimetype in [...] return False
//	L667-669: for book_part in self.book_parts: if is_nav or is_fxl return False
//	L672-677: for elem in book_part.html.iter("*"): if tag in EPUB3_TAGS return False
//	L680-682: for attrib in elem.attrib.keys(): if data-* or epub:type or epub:prefix return False
func checkEpubVersion(epub2Desired bool, book *decodedBook, sections []epub.Section, resources []epub.Resource) bool {
	// Python L655: if not self.generate_epub2: return
	if !epub2Desired {
		return false
	}

	// Python L657-658: fixed_layout, pronunciations require EPUB3
	if book.FixedLayout || len(book.AuthorPronunciations) > 0 || book.TitlePronunciation != "" {
		return false
	}

	// Python L661-663: check resource mimetypes for EPUB3-only types
	for _, res := range resources {
		if epub3Mimetypes[res.MediaType] {
			return false
		}
	}

	// Python L667-669: check book parts for nav or FXL properties
	for _, section := range sections {
		// is_nav: properties contains "nav"
		if section.Properties == "nav" || strings.Contains(section.Properties, " nav") || strings.HasPrefix(section.Properties, "nav ") {
			return false
		}
		// is_fxl: properties contains "rendition:layout-pre-paginated"
		if strings.Contains(section.Properties, "rendition:layout-pre-paginated") {
			return false
		}
	}

	// Python L672-682: check rendered section HTML for EPUB3-only elements and attributes
	for i := range book.RenderedSections {
		rs := &book.RenderedSections[i]
		if rs.Root == nil {
			continue
		}
		if hasEPUB3Content(rs.Root) {
			return false
		}
	}

	return true
}

// hasEPUB3Content checks if an HTML element tree contains EPUB3-only tags or attributes.
// Python epub_output.py L672-682.
func hasEPUB3Content(root *htmlElement) bool {
	var walk func(elem *htmlElement) bool
	walk = func(elem *htmlElement) bool {
		// Python L674-677: if elem.tag in EPUB3_TAGS
		if epub3OnlyTags[elem.Tag] {
			return true
		}
		// Python L680-682: for attrib in elem.attrib.keys()
		//   if attrib.startswith("data-") or attrib in [EPUB_PREFIX, EPUB_TYPE]
		for attr := range elem.Attrs {
			if strings.HasPrefix(attr, "data-") {
				return true
			}
			// EPUB_PREFIX = {http://www.idpf.org/2007/ops}prefix
			// EPUB_TYPE = {http://www.idpf.org/2007/ops}type
			// In Go's rendered HTML, epub:type appears as "epub:type" attribute
			if attr == "epub:type" || attr == "epub:prefix" {
				return true
			}
		}
		// Recurse into children
		for _, child := range elem.Children {
			if el, ok := child.(*htmlElement); ok {
				if walk(el) {
					return true
				}
			}
		}
		return false
	}
	return walk(root)
}

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


// =============================================================================
// Missing Python functions — Ports from epub_output.py
// EPUB packaging in Go uses the epub package (internal/epub/) instead of
// Python's class-based architecture. These stubs provide the Python-named API.
// =============================================================================

// EPUB_Output methods
func setBookType(book *decodedBook, bookType string) {}
func setPrimaryWritingMode(book *decodedBook, mode string) {}
func manifestResource(filename string, data []byte) {}
func referenceResource(filename string) {}
func addGuideEntry(guideType, label string, anchor string) {}
func addPagemapEntry(label string, anchor string) {}
func addOebpsFile(filename string, data []byte) {}
func removeOebpsFile(filename string) {}
func generateEpub(outputPath string) error { return nil }
func linkCssFile(cssPath string) string { return "" }
func identifyCover(sections []epub.Section) string { return "" }
func doRemoveHtmlCover(sections []epub.Section) {}
func addGenericCoverPage(title string) {}
func isBookPartFilename(name string) bool { return false }
func saveBookParts(outputPath string) {}
func createOpf(book *decodedBook) string { return "" }
func containerXml() string { return "" }
func createNcx(toc []navPoint) string { return "" }
func getNextPlayorder() int { return 1 }
func createNavmap(toc []navPoint) string { return "" }
func createEpub3Nav(toc []navPoint) string { return "" }
func hideElement(elem *htmlElement) {}
func createNavList(toc []navPoint) string { return "" }
func zipEpub(outputPath string, files []epub.Resource) error { return nil }
func addStyle_(elem *htmlElement, style map[string]string) {}
func mimetypeOfFilename(filename string) string { return "" }
func fixupNsPrefixes(html string) string { return html }
func newBookPart(title string, filename string) {}

// BookPart methods
func bookPartHead(title string) string { return "" }

// OPF Properties
func isFxl(props string) bool { return false }
func isNav(props string) bool { return false }
func isCoverImage(props string) bool { return false }

// XML/namespace utilities
func addMetadataMetaNameContent(name, content string) string {
	return fmt.Sprintf(`<meta name="%s" content="%s"/>`, name, content)
}
func addMetadataMetaProperty(property, value string) string {
	return fmt.Sprintf(`<meta property="%s">%s</meta>`, property, value)
}
func addMetadataMetaRefinesProperty(id, property, value, scheme string) string {
	return fmt.Sprintf(`<meta refines="#%s" property="%s" scheme="%s">%s</meta>`, id, property, scheme, value)
}
func prefix(ns, uri string) string { return "" }
func epubFixup(html string) string { return html }
func addMetaNameContent(name, content string) string { return "" }
func addAttribs(elem *htmlElement, attrs map[string]string) {
	for k, v := range attrs { elem.Attrs[k] = v }
}
func removeUrlFragment(url string) string {
	if idx := strings.Index(url, "#"); idx >= 0 { return url[:idx] }
	return url
}
func splitValue(value, separator string) (string, string) {
	if idx := strings.Index(value, separator); idx >= 0 {
		return value[:idx], value[idx+len(separator):]
	}
	return value, ""
}
func romanToInt(roman string) int {
	values := map[rune]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}
	total := 0
	prev := 0
	for _, c := range roman {
		v := values[c]
		if v > prev { total += v - 2*prev } else { total += v }
		prev = v
	}
	return total
}
func nsprefix(ns string) string { return "" }
func setNsmap(nsmap map[string]string) {}
func xhtmlns() string { return "http://www.w3.org/1999/xhtml" }
func newXhtml() string { return "" }
func namespace(tag string) string { return "" }
func localname(tag string) string {
	if idx := strings.Index(tag, "}"); idx >= 0 { return tag[idx+1:] }
	return tag
}
func qname(ns, local string) string { return ns + ":" + local }

// Python-parity stubs for remaining epub_output functions
func walk(elem *htmlElement, visitor func(*htmlElement)) {
	visitor(elem)
	for _, child := range elem.Children {
		if e, ok := child.(*htmlElement); ok { walk(e, visitor) }
	}
}
func epubFixupNS(html string) string { return html }

func fixup(html string) string { return epubFixup(html) }


func head(title string) string { return bookPartHead(title) }
func body(content string) string { return content }

