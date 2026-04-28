package kfx

import (
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
)


// Placeholders embedded in XHTML before finalize; KFX_EPUB_Properties style catalog (yj_to_epub_properties.py).
var styleTokenPattern = regexp.MustCompile(`__STYLE_\d+__`)

type styleCatalog struct {
	staticRules  map[string]string
	entries      []*styleEntry
	byKey        map[string]*styleEntry
	byToken      map[string]*styleEntry
	finalized    bool
	replacements []string
	css          string
}

type styleEntry struct {
	token        string
	baseName     string
	declarations string
	count        int
	order        int
	finalName    string
	referenced   bool
}

func newStyleCatalog() *styleCatalog {
	return &styleCatalog{
		staticRules: map[string]string{},
		byKey:       map[string]*styleEntry{},
		byToken:     map[string]*styleEntry{},
	}
}

func (c *styleCatalog) addStatic(selector string, declarations []string) {
	if c == nil || selector == "" || len(declarations) == 0 {
		return
	}
	if !strings.HasPrefix(selector, ".") {
		selector = "." + selector
	}
	if _, ok := c.staticRules[selector]; ok {
		return
	}
	c.staticRules[selector] = strings.Join(canonicalDeclarations(declarations), "; ")
}

func (c *styleCatalog) bind(baseName string, declarations []string) string {
	if c == nil || baseName == "" || len(declarations) == 0 {
		return ""
	}
	baseName = strings.TrimPrefix(baseName, ".")
	declarations = canonicalDeclarations(declarations)
	key := baseName + "\x00" + strings.Join(declarations, "; ")
	if entry, ok := c.byKey[key]; ok {
		entry.count++
		return entry.token
	}
	entry := &styleEntry{
		token:        fmt.Sprintf("__STYLE_%d__", len(c.entries)),
		baseName:     baseName,
		declarations: strings.Join(declarations, "; "),
		count:        1,
		order:        len(c.entries),
	}
	c.entries = append(c.entries, entry)
	c.byKey[key] = entry
	c.byToken[entry.token] = entry
	c.finalized = false
	c.css = ""
	c.replacements = nil
	return entry.token
}

func (c *styleCatalog) reserveClass(baseName string) string {
	if c == nil || baseName == "" {
		return ""
	}
	baseName = strings.TrimPrefix(baseName, ".")
	for index := 0; ; index++ {
		candidate := baseName
		if index > 0 {
			candidate = fmt.Sprintf("%s-%d", baseName, index)
		}
		if _, ok := c.staticRules["."+candidate]; ok {
			continue
		}
		conflict := false
		for _, entry := range c.entries {
			if entry.baseName == candidate || entry.finalName == candidate {
				conflict = true
				break
			}
		}
		if !conflict {
			return candidate
		}
	}
}

func (c *styleCatalog) finalize() {
	if c == nil || c.finalized {
		return
	}
	c.finalized = true
	if len(c.entries) == 0 && len(c.staticRules) == 0 {
		c.css = ""
		return
	}
	usedEntries := make([]*styleEntry, 0, len(c.entries))
	groupSizes := map[string]int{}
	for _, entry := range c.entries {
		if !entry.referenced {
			continue
		}
		usedEntries = append(usedEntries, entry)
		groupSizes[entry.baseName]++
	}
	sortedEntries := append([]*styleEntry(nil), usedEntries...)
	// Python sorts styles by frequency (most common first) before assigning class indices.
	// Ported from: sorted(style_counts.items(), key=lambda sc: -sc[1])
	// Within each baseName group, entries with higher count get lower index numbers.
	// Ties broken by insertion order (stable sort).
	sort.SliceStable(sortedEntries, func(i, j int) bool {
		if sortedEntries[i].baseName != sortedEntries[j].baseName {
			return sortedEntries[i].baseName < sortedEntries[j].baseName
		}
		// Same baseName: higher count (more frequent) → lower index
		if sortedEntries[i].count != sortedEntries[j].count {
			return sortedEntries[i].count > sortedEntries[j].count
		}
		// Same count: preserve insertion order
		return sortedEntries[i].order < sortedEntries[j].order
	})
	nextIndex := map[string]int{}
	usedNames := map[string]bool{}
	for selector := range c.staticRules {
		usedNames[strings.TrimPrefix(selector, ".")] = true
	}
	usedNames["class_s8"] = true
	for _, entry := range sortedEntries {
		finalName := entry.baseName
		if entry.baseName == "class" {
			for {
				finalName = fmt.Sprintf("%s-%d", entry.baseName, nextIndex[entry.baseName])
				nextIndex[entry.baseName]++
				if !usedNames[finalName] {
					break
				}
			}
		} else if groupSizes[entry.baseName] > 1 || usedNames[finalName] {
			for {
				finalName = fmt.Sprintf("%s-%d", entry.baseName, nextIndex[entry.baseName])
				nextIndex[entry.baseName]++
				if !usedNames[finalName] {
					break
				}
			}
		}
		entry.finalName = finalName
		usedNames[finalName] = true
		c.replacements = append(c.replacements, entry.token, finalName)
	}
	rules := map[string]string{}
	selectors := make([]string, 0, len(c.staticRules)+len(c.entries))
	for selector, declarations := range c.staticRules {
		rules[selector] = declarations
		selectors = append(selectors, selector)
	}
	for _, entry := range usedEntries {
		selector := "." + entry.finalName
		if _, ok := rules[selector]; ok {
			continue
		}
		rules[selector] = entry.declarations
		selectors = append(selectors, selector)
	}
	sort.Slice(selectors, func(i, j int) bool {
		return naturalSortKey(selectors[i]) < naturalSortKey(selectors[j])
	})
	lines := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		lines = append(lines, selector+" {"+rules[selector]+"}")
	}
	c.css = strings.Join(lines, "\n")
}

func (c *styleCatalog) replacer() *strings.Replacer {
	if c == nil {
		return strings.NewReplacer()
	}
	c.finalize()
	if len(c.replacements) == 0 {
		return strings.NewReplacer()
	}
	return strings.NewReplacer(c.replacements...)
}

func (c *styleCatalog) markReferenced(content string) {
	if c == nil || content == "" {
		return
	}
	for _, token := range styleTokenPattern.FindAllString(content, -1) {
		if entry, ok := c.byToken[token]; ok {
			entry.referenced = true
		}
	}
}

func (c *styleCatalog) String() string {
	if c == nil {
		return ""
	}
	c.finalize()
	return c.css
}

func finalizeStylesheet(stylesheet string) string {
	stylesheet = strings.TrimSpace(stylesheet)
	if stylesheet == "" {
		return ""
	}
	lines := strings.Split(stylesheet, "\n")
	fontFaces := make([]string, 0, len(lines))
	rules := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "@charset ") {
			continue
		}
		if strings.HasPrefix(line, "@font-face ") {
			fontFaces = append(fontFaces, line)
			continue
		}
		rules = append(rules, line)
	}
	sort.Strings(fontFaces)
	sort.SliceStable(rules, func(i, j int) bool {
		selectorI := rules[i]
		if idx := strings.Index(selectorI, " {"); idx >= 0 {
			selectorI = selectorI[:idx]
		}
		selectorJ := rules[j]
		if idx := strings.Index(selectorJ, " {"); idx >= 0 {
			selectorJ = selectorJ[:idx]
		}
		return naturalSortKey(selectorI) < naturalSortKey(selectorJ)
	})
	// Post-process: add margin-left: auto; margin-right: auto to figure_ rules
	// that have text-align but don't have margin-left/margin-right.
	// This matches Python's create_container behavior for box-align (yj_to_epub_content.py:1335-1336).
	for i, rule := range rules {
		if !strings.Contains(rule, ".figure_") {
			continue
		}
		if !strings.Contains(rule, "text-align") {
			continue
		}
		if !strings.Contains(rule, "width:") {
			continue
		}
		if strings.Contains(rule, "margin-left") || strings.Contains(rule, "margin-right") {
			continue
		}
		// Parse the rule, add margin auto, re-sort properties alphabetically
		openBrace := strings.Index(rule, "{")
		closingBrace := strings.LastIndex(rule, "}")
		if openBrace < 0 || closingBrace < 0 || closingBrace <= openBrace {
			continue
		}
		selector := rule[:openBrace+1]
		body := rule[openBrace+1 : closingBrace]
		// Parse existing declarations
		decls := strings.Split(body, ";")
		decls = append(decls, "margin-left: auto", "margin-right: auto")
		// Clean and sort
		sorted := make([]string, 0, len(decls))
		for _, d := range decls {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			sorted = append(sorted, d)
		}
		sort.SliceStable(sorted, func(i, j int) bool {
			pi := sorted[i]
			pj := sorted[j]
			if idx := strings.IndexByte(pi, ':'); idx >= 0 {
				pi = pi[:idx]
			}
			if idx := strings.IndexByte(pj, ':'); idx >= 0 {
				pj = pj[:idx]
			}
			return pi < pj
		})
		rules[i] = selector + strings.Join(sorted, "; ") + "}"
	}
	out := make([]string, 0, 1+len(fontFaces)+len(rules))
	out = append(out, `@charset "UTF-8";`)
	out = append(out, fontFaces...)
	out = append(out, rules...)
	return strings.Join(out, "\n")
}

func collectReferencedClasses(book *decodedBook) map[string]bool {
	used := map[string]bool{}
	if book == nil {
		return used
	}
	addClasses := func(value string) {
		for _, className := range strings.Fields(strings.TrimSpace(value)) {
			if className != "" {
				used[className] = true
			}
		}
	}
	for _, section := range book.Sections {
		addClasses(section.BodyClass)
		for _, match := range regexp.MustCompile(`class="([^"]+)"`).FindAllStringSubmatch(section.BodyHTML, -1) {
			if len(match) > 1 {
				addClasses(match[1])
			}
		}
	}
	return used
}

// pruneUnusedResources removes image resources not referenced in any section HTML.
// Port of Python's raw_media unused removal (yj_to_epub.py L117-120).
// Fonts are always kept (referenced from CSS @font-face), and the cover image is kept.
func pruneUnusedResources(book *decodedBook) {
	if book == nil || len(book.Resources) == 0 {
		return
	}
	// Collect all referenced hrefs from section HTML.
	referenced := map[string]bool{}
	for _, section := range book.Sections {
		for _, match := range regexp.MustCompile(`src="([^"]+)"`).FindAllStringSubmatch(section.BodyHTML, -1) {
			if len(match) > 1 {
				referenced[match[1]] = true
			}
		}
	}
	// Also check stylesheet for url() references (fonts, etc).
	for _, match := range regexp.MustCompile(`url\(([^)]+)\)`).FindAllStringSubmatch(book.Stylesheet, -1) {
		if len(match) > 1 {
			referenced[match[1]] = true
		}
	}
	// Also keep the cover image.
	if book.CoverImageHref != "" {
		referenced[book.CoverImageHref] = true
	}
	// Filter resources — always keep fonts (they're referenced from CSS not HTML).
	filtered := make([]epub.Resource, 0, len(book.Resources))
	for _, res := range book.Resources {
		if referenced[res.Filename] || strings.HasPrefix(res.MediaType, "font/") || strings.HasPrefix(res.MediaType, "application/font") || strings.HasPrefix(res.MediaType, "application/x-font") {
			filtered = append(filtered, res)
		}
	}
	book.Resources = filtered
}

func pruneUnusedStylesheetRules(stylesheet string, used map[string]bool) string {
	if stylesheet == "" || len(used) == 0 {
		return stylesheet
	}
	lines := strings.Split(stylesheet, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ".") {
			selector := trimmed[1:]
			if idx := strings.Index(selector, " {"); idx >= 0 {
				selector = selector[:idx]
			}
			baseSelector := selector
			for _, sep := range []string{"::", ":", " ", "["} {
				if idx := strings.Index(baseSelector, sep); idx >= 0 {
					baseSelector = baseSelector[:idx]
				}
			}
			if baseSelector != "" && !used[baseSelector] {
				continue
			}
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func parseDeclarationString(value string) map[string]string {
	style := map[string]string{}
	for _, declaration := range strings.Split(value, ";") {
		declaration = strings.TrimSpace(declaration)
		if declaration == "" {
			continue
		}
		name, rawValue, ok := strings.Cut(declaration, ":")
		if !ok {
			continue
		}
		style[strings.TrimSpace(name)] = strings.TrimSpace(rawValue)
	}
	return style
}

func styleStringFromMap(style map[string]string) string {
	declarations := declarationListFromStyleMap(style)
	if len(declarations) == 0 {
		return ""
	}
	return strings.Join(declarations, "; ")
}

func declarationListFromStyleMap(style map[string]string) []string {
	if len(style) == 0 {
		return nil
	}

	// Apply alternate equivalent properties (-webkit- prefixed equivalents).
	// Ported from Python ALTERNATE_EQUIVALENT_PROPERTIES in add_composite_and_equivalent_styles.
	for name, altName := range alternateEquivalentProperties {
		if val, ok := style[name]; ok && val != "" {
			if _, exists := style[altName]; !exists {
				style[altName] = val
			}
		}
	}

	declarations := make([]string, 0, len(style))
	for name, value := range style {
		if strings.TrimSpace(value) == "" {
			continue
		}
		declarations = append(declarations, name+": "+value)
	}
	return canonicalDeclarations(declarations)
}

func styleMetadataForBaseName(baseName string, layoutHints []string) map[string]string {
	meta := map[string]string{}
	switch {
	case strings.HasPrefix(baseName, "heading_"):
		meta["-kfx-style-name"] = strings.TrimPrefix(baseName, "heading_")
		layoutHints = append(layoutHints, "heading")
	case strings.HasPrefix(baseName, "class_"):
		meta["-kfx-style-name"] = strings.TrimPrefix(baseName, "class_")
	}
	if len(layoutHints) > 0 {
		uniqHints := make([]string, 0, len(layoutHints))
		seen := map[string]bool{}
		for _, hint := range layoutHints {
			if hint == "" || seen[hint] {
				continue
			}
			seen[hint] = true
			uniqHints = append(uniqHints, hint)
		}
		sortedHints := append([]string(nil), uniqHints...)
		sort.Strings(sortedHints)
		meta["-kfx-layout-hints"] = strings.Join(sortedHints, " ")
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

func styleStringFromDeclarations(baseName string, layoutHints []string, declarations []string) string {
	style := parseDeclarationString(strings.Join(canonicalDeclarations(declarations), "; "))
	for name, value := range styleMetadataForBaseName(baseName, layoutHints) {
		style[name] = value
	}
	return styleStringFromMap(style)
}

func mergeStyleStrings(values ...string) string {
	style := map[string]string{}
	for _, value := range values {
		for name, raw := range parseDeclarationString(value) {
			style[name] = raw
		}
	}
	return styleStringFromMap(style)
}

func stripStyleMetadata(style map[string]string) map[string]string {
	if len(style) == 0 {
		return map[string]string{}
	}
	out := map[string]string{}
	for name, value := range style {
		if name == "-kfx-style-name" || name == "-kfx-layout-hints" {
			continue
		}
		out[name] = value
	}
	return out
}

func setElementStyleString(elem *htmlElement, value string) {
	if elem == nil {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		if elem.Attrs != nil {
			delete(elem.Attrs, "style")
			if len(elem.Attrs) == 0 {
				elem.Attrs = nil
			}
		}
		return
	}
	if elem.Attrs == nil {
		elem.Attrs = map[string]string{}
	}
	elem.Attrs["style"] = value
}

func prependClassName(existing string, className string) string {
	className = strings.TrimSpace(className)
	if className == "" {
		return strings.TrimSpace(existing)
	}
	parts := []string{}
	seen := map[string]bool{}
	for _, raw := range append([]string{className}, strings.Fields(strings.TrimSpace(existing))...) {
		if raw == "" || seen[raw] {
			continue
		}
		seen[raw] = true
		parts = append(parts, raw)
	}
	return strings.Join(parts, " ")
}

// Port of KFX_EPUB_Properties.update_default_font_and_language (yj_to_epub_properties.py ~L1604+).
func updateDefaultFontAndLanguage(book *decodedBook) {
	if book == nil {
		return
	}
	book.Language = normalizeLanguage(book.Language)
}

// Port of KFX_EPUB_Properties.set_html_defaults (yj_to_epub_properties.py ~L1652+).
// Go emits per-section Language in epub.sectionXHTML; ensure each rendered section inherits book language when unset.
func setHTMLDefaults(book *decodedBook, defaultFontFamily string) map[int]bool {
	if book == nil {
		return nil
	}
	fontFamilyAddedByDefaults := map[int]bool{}
	lang := normalizeLanguage(book.Language)
	for i := range book.RenderedSections {
		bodyStyle := parseDeclarationString(book.RenderedSections[i].BodyStyle)
		if bodyStyle["font-family"] == "" {
			bodyStyle["font-family"] = defaultFontFamily
			fontFamilyAddedByDefaults[i] = true
		}
		if bodyStyle["font-size"] == "" {
			bodyStyle["font-size"] = "1em"
		}
		if bodyStyle["line-height"] == "" {
			bodyStyle["line-height"] = "normal"
		}
		if bodyStyle["writing-mode"] == "" {
			if strings.TrimSpace(book.WritingMode) != "" {
				bodyStyle["writing-mode"] = book.WritingMode
			} else {
				bodyStyle["writing-mode"] = "horizontal-tb"
			}
		}
		book.RenderedSections[i].BodyStyle = styleStringFromMap(bodyStyle)
		if strings.TrimSpace(book.RenderedSections[i].Language) == "" {
			book.RenderedSections[i].Language = lang
		}
	}
	return fontFamilyAddedByDefaults
}

// Port of KFX_EPUB_Properties.fixup_styles_and_classes (yj_to_epub_properties.py ~L1388+).
func fixupStylesAndClasses(book *decodedBook, catalog *styleCatalog, fontFamilyAddedByDefaults map[int]bool, resolvedDefaultFont string) {
	if book == nil || catalog == nil {
		return
	}

	simplifyStylesFull(book, catalog, fontFamilyAddedByDefaults, resolvedDefaultFont)

	// Ported from Python fixup_styles_and_classes (yj_to_epub_properties.py:1427-1446):
	// Extract -kfx-attrib-* properties from inline styles and convert to element attributes.
	// Python: style.partition(name_prefix="-kfx-attrib-", remove_prefix=True)
	for i := range book.RenderedSections {
		walkHTMLElement(book.RenderedSections[i].Root, func(elem *htmlElement) {
			if elem == nil || elem.Attrs == nil || elem.Attrs["style"] == "" {
				return
			}
			style := parseDeclarationString(elem.Attrs["style"])
			modified := false
			for name, value := range style {
				if !strings.HasPrefix(name, "-kfx-attrib-") {
					continue
				}
				// Strip prefix: -kfx-attrib-xml-lang → xml-lang
				attrName := name[len("-kfx-attrib-"):]
				var xmlAttr string
				if strings.HasPrefix(attrName, "epub-") {
					// -kfx-attrib-epub-type → {http://www.idpf.org/2007/ops}type
					// Stored as "epub:type" in the attribute map.
					xmlAttr = "epub:type"
				} else if strings.HasPrefix(attrName, "xml-") {
					// -kfx-attrib-xml-lang → xml:lang
					xmlAttr = "xml:lang"
				} else {
					// colspan, rowspan, valign, etc. — use as-is
					xmlAttr = attrName
				}
				if elem.Attrs == nil {
					elem.Attrs = map[string]string{}
				}
				elem.Attrs[xmlAttr] = value
				delete(style, name)
				modified = true
			}
			if modified {
				if len(style) == 0 {
					delete(elem.Attrs, "style")
				} else {
					elem.Attrs["style"] = styleStringFromMap(style)
				}
				if len(elem.Attrs) == 0 {
					elem.Attrs = nil
				}
			}
		})
	}

	// Ported from Python fixup_styles_and_classes direction/unicode-bidi conversion
	// (yj_to_epub_properties.py L1448-1500).
	// Convert CSS direction property to HTML dir attribute and unicode-bidi to bdi/bdo elements.
	// Go always generates EPUB3 so the condition is always true (Python:
	// CVT_DIRECTION_PROPERTY_TO_MARKUP or not self.generate_epub2).
	for i := range book.RenderedSections {
		walkHTMLElement(book.RenderedSections[i].Root, func(elem *htmlElement) {
			if elem == nil || elem.Attrs == nil || elem.Attrs["style"] == "" {
				return
			}
			style := parseDeclarationString(elem.Attrs["style"])
			if _, hasDir := style["direction"]; !hasDir {
				if _, hasBidi := style["unicode-bidi"]; !hasBidi {
					return
				}
			}

			unicodeBidi := style["unicode-bidi"]
			if unicodeBidi == "" {
				unicodeBidi = "normal"
			}

			// Determine has_block and has_content by iterating all descendants.
			// Python: for ex in e.iterfind(".//*") — iterates ONLY descendants, not e itself.
			hasBlock := false
			hasContent := elemHasDirectText(elem) // Python: has_content = e.text
			walkDescendantElements(elem, func(desc *htmlElement) {
				if desc == nil {
					return
				}
				if !hasBlock {
					switch desc.Tag {
					case "aside", "div", "figure", "h1", "h2", "h3", "h4", "h5", "h6",
						"iframe", "li", "ol", "p", "table", "td", "ul":
						hasBlock = true
					}
				}
				if !hasContent {
					if elemHasDirectText(desc) {
						hasContent = true
					} else if desc.Tag == "audio" || desc.Tag == "img" || desc.Tag == "li" ||
						desc.Tag == "math" || desc.Tag == "object" || desc.Tag == "svg" || desc.Tag == "video" {
						hasContent = true
					}
					// Python checks ex.tail — in Go's DOM, tail text is represented
					// as htmlText siblings after the element within its parent's Children.
					if !hasContent && elemHasTailText(desc, elem) {
						hasContent = true
					}
				}
			})

			if !hasContent {
				// No content: just remove the direction/unicode-bidi properties
				delete(style, "direction")
				delete(style, "unicode-bidi")
				setElemStyle(elem, style)

			} else if unicodeBidi == "embed" || unicodeBidi == "normal" || hasBlock {
				// embed/normal or has block children: convert direction to dir attribute
				if dir, ok := style["direction"]; ok {
					elem.Attrs["dir"] = dir
					delete(style, "direction")
				}
				delete(style, "unicode-bidi")
				setElemStyle(elem, style)

			} else if unicodeBidi == "isolate" || unicodeBidi == "bidi-override" ||
				unicodeBidi == "isolate-override" {
				// isolate/bidi-override/isolate-override: wrap content in bdi/bdo element
				bdx := &htmlElement{}
				if strings.Contains(unicodeBidi, "override") {
					bdx.Tag = "bdo"
				} else {
					bdx.Tag = "bdi"
				}
				if dir, ok := style["direction"]; ok {
					bdx.Attrs = map[string]string{"dir": dir}
					delete(style, "direction")
				}

				if elem.Tag != "img" {
					// Move element's children to bdx (Python: bdx.text = e.text, move children)
					bdx.Children = elem.Children
					elem.Children = []htmlPart{bdx}
				}

				delete(style, "unicode-bidi")
				setElemStyle(elem, style)

			} else {
				log.Printf("Cannot produce EPUB3 equivalent for: unicode-bidi:%s direction:%s",
					unicodeBidi, style["direction"])
			}
		})
	}

	// Ported from Python REMOVE_EMPTY_NAMED_CLASSES (yj_to_epub_properties.py:1499-1505):
	// If an element's style has ONLY -kfx-style-name and/or -kfx-layout-hints
	// (no real CSS properties), remove the style attr entirely so no class is assigned.
	for i := range book.RenderedSections {
		walkHTMLElement(book.RenderedSections[i].Root, func(elem *htmlElement) {
			if elem == nil || elem.Attrs == nil || elem.Attrs["style"] == "" {
				return
			}
			style := parseDeclarationString(elem.Attrs["style"])
			hasOther := false
			for name := range style {
				if name != "-kfx-style-name" && name != "-kfx-layout-hints" {
					hasOther = true
					break
				}
			}
			if !hasOther {
				delete(elem.Attrs, "style")
				if len(elem.Attrs) == 0 {
					elem.Attrs = nil
				}
			}
		})
	}

	// Ported from Python consolidate_html (epub_output.py:742-775):
	// Merge adjacent inline elements with identical attributes, then strip empty
	// <span> tags with no attributes. Must run before beautifyHTML (Python calls
	// consolidate_html at L700 before beautify_html at L703).
	for i := range book.RenderedSections {
		consolidateHTML(book.RenderedSections[i].Root)
	}

	// Ported from Python beautify_html (epub_output.py:783-789):
	// Strip spans with no attributes (left over after REMOVE_EMPTY_NAMED_CLASSES).
	for i := range book.RenderedSections {
		beautifyHTML(book.RenderedSections[i].Root)
	}

	// Ported from Python consolidate_html div stripping (epub_output.py:781-804):
	// Strip bare <div> wrappers with no attributes after style catalog has converted
	// inline styles to classes. Must run after consolidateHTML and beautifyHTML so
	// that style catalog has already removed redundant class attributes.
	for i := range book.RenderedSections {
		stripBareDivs(book.RenderedSections[i].Root)
	}

	type countedStyle struct {
		style string
		count int
		order int
	}
	styleCounts := map[string]*countedStyle{}
	nextOrder := 0
	countStyle := func(style string) {
		style = strings.TrimSpace(style)
		if style == "" {
			return
		}
		if existing, ok := styleCounts[style]; ok {
			existing.count++
			return
		}
		styleCounts[style] = &countedStyle{style: style, count: 1, order: nextOrder}
		nextOrder++
	}

	for i := range book.RenderedSections {
		// Count body style first (matching Python's body.iter("*") which encounters body before children).
		// Python has no static body classes — all body styles go through the normal counting pipeline.
		if book.RenderedSections[i].BodyStyle != "" {
			countStyle(book.RenderedSections[i].BodyStyle)
		}
		// Walk all child elements. Python counts via body.iter("*") which yields body first,
		// then children depth-first.
		walkHTMLElement(book.RenderedSections[i].Root, func(elem *htmlElement) {
			if elem != nil && elem.Attrs != nil && elem.Attrs["style"] != "" {
				countStyle(elem.Attrs["style"])
			}
		})
	}

	sortedStyles := make([]*countedStyle, 0, len(styleCounts))
	for _, entry := range styleCounts {
		sortedStyles = append(sortedStyles, entry)
	}
	sort.SliceStable(sortedStyles, func(i, j int) bool {
		if sortedStyles[i].count == sortedStyles[j].count {
			return sortedStyles[i].order < sortedStyles[j].order
		}
		return sortedStyles[i].count > sortedStyles[j].count
	})

	knownClassCounts := map[string]int{}
	for selector := range catalog.staticRules {
		knownClassCounts[strings.TrimPrefix(selector, ".")] = 2
	}
	type styleCandidate struct {
		styleStr  string
		style     map[string]string
		className string
	}
	candidates := make([]styleCandidate, 0, len(sortedStyles))
	for _, entry := range sortedStyles {
		style := parseDeclarationString(entry.style)
		styleName := sanitizeCSSClassComponent(style["-kfx-style-name"])
		delete(style, "-kfx-style-name")
		classPrefix := classPrefixFromStyle(style)
		delete(style, "-kfx-layout-hints")

		className := prefixUniquePartOfSymbol(styleName, classPrefix)
		knownClassCounts[classPrefix]++
		knownClassCounts[className]++
		candidates = append(candidates, styleCandidate{styleStr: entry.style, style: style, className: className})
	}

	styleClassNames := map[string]string{}
	classStyles := map[string]map[string]string{}
	usedClassCounts := map[string]int{}
	referencedClasses := map[string]bool{}

	for _, entry := range candidates {
		className := entry.className
		if knownClassCounts[className] > 1 || classStyles[className] != nil {
			for {
				unique := usedClassCounts[className]
				usedClassCounts[className]++
				candidate := fmt.Sprintf("%s-%d", className, unique)
				if knownClassCounts[candidate] == 0 && classStyles[candidate] == nil {
					className = candidate
					break
				}
			}
		}
		styleClassNames[entry.styleStr] = className
		classStyles[className] = entry.style
	}

	for i := range book.RenderedSections {
		if style := strings.TrimSpace(book.RenderedSections[i].BodyStyle); style != "" {
			if className := styleClassNames[style]; className != "" {
				book.RenderedSections[i].BodyClass = prependClassName(book.RenderedSections[i].BodyClass, className)
				book.RenderedSections[i].BodyStyle = ""
				referencedClasses[className] = true
			}
		}
		walkHTMLElement(book.RenderedSections[i].Root, func(elem *htmlElement) {
			if elem == nil || elem.Attrs == nil {
				return
			}
			style := strings.TrimSpace(elem.Attrs["style"])
			if style == "" {
				return
			}
			className := styleClassNames[style]
			if className != "" {
				elem.Attrs["class"] = prependClassName(elem.Attrs["class"], className)
				delete(elem.Attrs, "style")
				referencedClasses[className] = true
			}
		})
	}

	for className, classStyle := range classStyles {
		if !referencedClasses[className] || len(classStyle) == 0 {
			continue
		}
		catalog.addStatic(className, declarationListFromStyleMap(classStyle))
	}

	// Ported from Python inventory_style validation (yj_to_epub_properties.py L1593-1602).
	// Validate all CSS property values in static rules against KNOWN_STYLES for
	// development-time regression detection. Python also validates font_faces but Go
	// processes font faces as pre-formatted CSS strings in book.Stylesheet, not as
	// property maps, so we only validate the staticRules here.
	for _, declarations := range catalog.staticRules {
		style := parseDeclarationString(declarations)
		inventoryStyle(style)
	}

	for i := range book.RenderedSections {
		fixupEmptyClassAttributes(book.RenderedSections[i].Root)
	}
}

func fixupEmptyClassAttributes(root *htmlElement) {
	if root == nil {
		return
	}
	if root.Attrs != nil {
		if c, ok := root.Attrs["class"]; ok && strings.TrimSpace(c) == "" {
			delete(root.Attrs, "class")
		}
		if len(root.Attrs) == 0 {
			root.Attrs = nil
		}
	}
	for _, ch := range root.Children {
		if el, ok := ch.(*htmlElement); ok {
			fixupEmptyClassAttributes(el)
		}
	}
}

// beautifyHTML strips spans with no attributes, matching Python's epub_output.py
// beautify_html (lines 783-789). Called after REMOVE_EMPTY_NAMED_CLASSES.
func createCSSFiles(book *decodedBook, catalog *styleCatalog) {
	if book == nil || catalog == nil {
		return
	}
	if css := catalog.String(); css != "" {
		if book.Stylesheet != "" {
			book.Stylesheet += "\n"
		}
		book.Stylesheet += css
	}
}

// Port of KFX_EPUB_Properties.simplify_styles post-rendering transformations (yj_to_epub_properties.py ~L1675+).
// Performs structural element conversions and CSS style inheritance/simplification on rendered DOM:
//   - <div> → <p>/<figure> conversion based on content analysis
//   - Remove unnecessary <span> wrappers with empty class
//   - Strip classes whose declarations are fully inherited from parent
//
// Note: Full Python simplify_styles has additional features (reverse inheritance, composite styles,
// ineffective property stripping, rem unit conversion) that require the style catalog to be mutable.
// This implementation covers the most impactful transformations that can be done post-rendering.
// combineNestedDivsPass implements Python's COMBINE_NESTED_DIVS (yj_to_epub_content.py:1409-1443).
// Bottom-up tree walk that merges parent <div> with single child <div>/<p> when their
// style properties don't overlap (except -kfx-style-name). Must run before simplify_styles.
func combineNestedDivsPass(elem *htmlElement) {
	if elem == nil {
		return
	}
	for _, child := range elem.Children {
		if ch, ok := child.(*htmlElement); ok {
			combineNestedDivsPass(ch)
		}
	}
	combineNestedDivs(elem)
}

func combineNestedDivs(elem *htmlElement) {
	if elem == nil || elem.Tag != "div" || len(elem.Children) != 1 {
		return
	}
	childHE, ok := elem.Children[0].(*htmlElement)
	if !ok {
		return
	}
	if childHE.Tag != "div" && childHE.Tag != "p" {
		return
	}
	for k := range elem.Attrs {
		if k != "id" && k != "style" {
			return
		}
	}
	for k := range childHE.Attrs {
		if k != "id" && k != "style" {
			return
		}
	}
	parentStyle := parseDeclarationString(elem.Attrs["style"])
	childStyle := parseDeclarationString(childHE.Attrs["style"])
	if parentStyle["display"] != "" && parentStyle["display"] != "block" {
		return
	}
	if parentStyle["position"] != "" && parentStyle["position"] != "static" {
		return
	}
	if parentStyle["float"] != "" && parentStyle["float"] != "none" {
		return
	}
	if childStyle["display"] != "" && childStyle["display"] != "block" {
		return
	}
	if childStyle["position"] != "" && childStyle["position"] != "static" {
		return
	}
	if childStyle["float"] != "" && childStyle["float"] != "none" {
		return
	}
	for k := range parentStyle {
		if k == "-kfx-style-name" {
			continue
		}
		if _, ok := childStyle[k]; ok {
			return
		}
	}
	_, parentHasID := elem.Attrs["id"]
	_, childHasID := childHE.Attrs["id"]
	if parentHasID && childHasID {
		return
	}
	// Merge: parent keeps its values, child adds non-overlapping properties
	for k, v := range childStyle {
		if _, ok := parentStyle[k]; !ok {
			parentStyle[k] = v
		}
	}
	if len(parentStyle) > 0 {
		elem.Attrs["style"] = styleStringFromMap(parentStyle)
	} else {
		delete(elem.Attrs, "style")
	}
	elem.Children = childHE.Children
}

func simplifyStylesFull(book *decodedBook, catalog *styleCatalog, fontFamilyAddedByDefaults map[int]bool, resolvedDefaultFont string) {
	if book == nil {
		return
	}
	for i := range book.RenderedSections {
		if book.RenderedSections[i].Root == nil {
			continue
		}

		// Build inherited properties matching Python's heritable_default_properties.
		// Python passes HERITABLE_DEFAULTS_FILTERED to simplify_styles(body_element).
		// The body's own style is on the body element itself (not merged into inherited).
		// The body's font-family (set by set_html_defaults) is on the element,
		// so it survives stripping against the default "serif" inherited value.
		// Children inherit the body's actual font-family via parent_sty (built from sty).
		bodyInherited := cloneStyleMap(heritableDefaultProperties)

		// Ported from Python (yj_to_epub_properties.py:1399-1401):
		// lang = book_part.html.get(XML_LANG)
		// if lang: heritable_default_properties.update({"-kfx-attrib-xml-lang": lang})
		// This ensures xml:lang propagates through simplify_styles inheritance.
		sectionLang := book.RenderedSections[i].Language
		if sectionLang != "" {
			bodyInherited["-kfx-attrib-xml-lang"] = sectionLang
		}

		// Convert lh/rem units in body style before putting it on the virtual body element.
		// Python's simplify_styles is called on the body element and converts units as part
		// of its processing. Since the body's style string comes from the rendering pipeline
		// (not from simplifyStylesElementFull), we need to convert here.
		bodyStyleMap := parseDeclarationString(book.RenderedSections[i].BodyStyle)
		convertStyleUnits(bodyStyleMap, heritableDefaultProperties)

		// Create a virtual body element matching Python's architecture where
		// simplify_styles is called on the <body> element.
		// Python: self.simplify_styles(book_part.body(), book_part, heritable_default_properties)
		//
		// IMPORTANT: Python's <body> has paragraphs, headings, and divs as DIRECT children.
		// This allows reverse inheritance to detect shared heritable properties across all
		// children (e.g., text-indent: 1.6em from most paragraphs). Go wraps body parts
		// in a Root element; we must flatten Root's children here so the virtual body has
		// the same child structure as Python. Without this, reverse inheritance on body
		// only sees the single Root wrapper, preventing detection of shared properties.
		var bodyChildren []htmlPart
		if book.RenderedSections[i].Root != nil {
			bodyChildren = book.RenderedSections[i].Root.Children
		}

		// Determine the body element's initial style for simplify_styles.
		// Python's body gets style from two sources:
		//   1. Content rendering (add_kfx_style) — when the page template has a style ref
		//   2. set_html_defaults — adds font-family, font-size, line-height, writing-mode if missing
		//
		// When the body has NO content-rendered style (inferred body), Python's body only
		// gets set_html_defaults properties. Go's inferBodyStyleValues produces a FULLER
		// style (including font-weight, text-align, etc.) that Python's body doesn't have.
		// Using the full inferred style as the body's inline style causes children to inherit
		// extra properties via parentStyle, changing stripping behavior.
		//
		// Fix: for inferred bodies, use minimal style (set_html_defaults properties only).
		// Reverse inheritance will promote shared properties from children.
		bodyStyleForSimplify := bodyStyleMap
		if book.RenderedSections[i].BodyStyleInferred {
			minimalBodyStyle := map[string]string{}
			for _, prop := range []string{"font-family", "font-size", "line-height", "writing-mode"} {
				if val, ok := bodyStyleMap[prop]; ok {
					minimalBodyStyle[prop] = val
				}
			}
			bodyStyleForSimplify = minimalBodyStyle
		}

		bodyElem := &htmlElement{
			Tag:      "body",
			Attrs:    map[string]string{"style": styleStringFromMap(bodyStyleForSimplify)},
			Children: bodyChildren,
		}

		// NOTE: Python's process_content creates a <div> child inside <body> that
		// simplify_styles later converts to <p> for inline-only content. Go's
		// promotedBodyContainer unwraps the body container, putting children
		// directly in <body>. We do NOT wrap in <p> here because Python does NOT
		// create a <p> wrapper either — the <div>→<body> renaming happens before
		// simplify_styles, and <body> is never converted to <p>. The inline
		// children stay directly inside <body>, matching Python's output.

		if os.Getenv("KFX_DEBUG_PRESIMPLIFY") != "" {
			fn := book.RenderedSections[i].Filename
			if fn == "c9.xhtml" || fn == "c1K4.xhtml" {
				var dumpPart func(htmlPart, int) string
				dumpPart = func(p htmlPart, depth int) string {
					indent := strings.Repeat("  ", depth)
					switch v := p.(type) {
					case *htmlElement:
						s := fmt.Sprintf("%s<%s", indent, v.Tag)
						keys := make([]string, 0, len(v.Attrs))
						for k := range v.Attrs {
							keys = append(keys, k)
						}
						sort.Strings(keys)
						for _, k := range keys {
							s += fmt.Sprintf(" %s=\"%s\"", k, v.Attrs[k])
						}
						if len(v.Children) == 0 {
							s += ">"
						} else {
							s += ">\n"
							for _, ch := range v.Children {
								s += dumpPart(ch, depth+1)
							}
							s += fmt.Sprintf("%s</%s>", indent, v.Tag)
						}
						return s + "\n"
					case htmlText:
						return fmt.Sprintf("%sTEXT:%q\n", indent, v.Text)
					case string:
						return fmt.Sprintf("%sSTR:%q\n", indent, v)
					default:
						return fmt.Sprintf("%s???:%T\n", indent, p)
					}
				}
				fmt.Fprintf(os.Stderr, "PRESIMPLIFY %s:\n%s\n", fn, dumpPart(bodyElem, 0))
			}
		}

		// Python's COMBINE_NESTED_DIVS runs during process_content (per-node) before
		// simplify_styles. Run it here as a pre-pass on the body element tree.
		combineNestedDivsPass(bodyElem)

		simplifyStylesElementFull(bodyElem, catalog, bodyInherited, &simplifyState{
			resourceDims:    book.ResourceDimensions,
			sectionFilename: book.RenderedSections[i].Filename,
		})

		if os.Getenv("KFX_DEBUG_POSTSIMPLIFY") != "" {
			fn := book.RenderedSections[i].Filename
			if fn == "c9.xhtml" || fn == "c1K4.xhtml" {
				var dumpPart func(htmlPart, int) string
				dumpPart = func(p htmlPart, depth int) string {
					indent := strings.Repeat("  ", depth)
					switch v := p.(type) {
					case *htmlElement:
						s := fmt.Sprintf("%s<%s", indent, v.Tag)
						keys := make([]string, 0, len(v.Attrs))
						for k := range v.Attrs {
							keys = append(keys, k)
						}
						sort.Strings(keys)
						for _, k := range keys {
							s += fmt.Sprintf(" %s=\"%s\"", k, v.Attrs[k])
						}
						if len(v.Children) == 0 {
							s += ">"
						} else {
							s += ">\n"
							for _, ch := range v.Children {
								s += dumpPart(ch, depth+1)
							}
							s += fmt.Sprintf("%s</%s>", indent, v.Tag)
						}
						return s + "\n"
					case htmlText:
						return fmt.Sprintf("%sTEXT:%q\n", indent, v.Text)
					case string:
						return fmt.Sprintf("%sSTR:%q\n", indent, v)
					default:
						return fmt.Sprintf("%s???:%T\n", indent, p)
					}
				}
				fmt.Fprintf(os.Stderr, "POSTSIMPLIFY %s:\n%s\n", fn, dumpPart(bodyElem, 0))
			}
		}

		// Extract -kfx-attrib-xml-lang from body style to set xml:lang on <body>.
		// Python does this in fixup_styles_and_classes via style.partition("-kfx-attrib-", ...)
		// which extracts -kfx-attrib-* properties and sets them as element attributes.
		if bodyElem.Attrs != nil {
			if bodyStyle := bodyElem.Attrs["style"]; bodyStyle != "" {
				parsed := parseDeclarationString(bodyStyle)
				if lang, ok := parsed["-kfx-attrib-xml-lang"]; ok && lang != "" {
					book.RenderedSections[i].BodyLanguage = lang
					delete(parsed, "-kfx-attrib-xml-lang")
					if len(parsed) == 0 {
						delete(bodyElem.Attrs, "style")
					} else {
						bodyElem.Attrs["style"] = styleStringFromMap(parsed)
					}
				}
			}
		}

		// Extract the updated children back into the Root wrapper.
		// Reverse inheritance on the body may have modified children's styles
		// (e.g., adding text-indent: 0 to image wrapper divs).
		book.RenderedSections[i].Root.Children = bodyElem.Children
		book.RenderedSections[i].BodyStyle = ""
		if bodyElem.Attrs != nil {
			book.RenderedSections[i].BodyStyle = bodyElem.Attrs["style"]
		}

		// Add resolved default font-family to empty structural containers.
		// In Python, the rendering pipeline preserves font-family from KFX style fragments
		// on all elements. Python's simplify_styles then runs reverse inheritance which promotes
		// font-family to the body. Elements without font-family (like empty structural containers)
		// get the body's pre-reverse-inheritance value from set_html_defaults.
		// In Go, filterBodyDefaultDeclarations strips font-family from children during rendering,
		// so reverse inheritance has no font-family to promote. Add it explicitly.
		if resolvedDefaultFont != "" && resolvedDefaultFont != "serif" && !fontFamilyAddedByDefaults[i] {
			overrideFFForEmptyContainers(book.RenderedSections[i].Root, resolvedDefaultFont)
		}
	}
}

// compositeSideStyles lists CSS shorthand properties and their corresponding individual side properties.
// When all individual sides have the same value, they can be collapsed into the shorthand.
// Ported from Python COMPOSITE_SIDE_STYLES in yj_to_epub_properties.py.
var compositeSideStyles = [][2]string{
	{"border-color", "border-bottom-color border-left-color border-right-color border-top-color"},
	{"border-style", "border-bottom-style border-left-style border-right-style border-top-style"},
	{"border-width", "border-bottom-width border-left-width border-right-width border-top-width"},
	{"margin", "margin-bottom margin-left margin-right margin-top"},
	{"padding", "padding-bottom padding-left padding-right padding-top"},
}

// alternateEquivalentProperties maps CSS properties to their vendor-prefixed equivalents.
// When a property has a value, its alternate equivalent is also added with the same value.
// Ported from Python ALTERNATE_EQUIVALENT_PROPERTIES in yj_to_epub_properties.py.
var alternateEquivalentProperties = map[string]string{
	"box-decoration-break":   "-webkit-box-decoration-break",
	"hyphens":                "-webkit-hyphens",
	"line-break":             "-webkit-line-break",
	"ruby-position":          "-webkit-ruby-position",
	"text-combine-upright":   "-webkit-text-combine",
	"text-emphasis-color":    "-webkit-text-emphasis-color",
	"text-emphasis-position": "-webkit-text-emphasis-position",
	"text-emphasis-style":    "-webkit-text-emphasis-style",
	"text-orientation":       "-webkit-text-orientation",
	"transform":              "-webkit-transform",
	"transform-origin":       "-webkit-transform-origin",
	"writing-mode":           "-webkit-writing-mode",
}

// Block-level element tags per Python simplify_styles contains_block_elem check.
var blockLevelTags = map[string]bool{
	"aside": true, "caption": true, "div": true, "figure": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"li": true, "p": true, "td": true,
	"table": true, "tr": true, "th": true, "thead": true, "tbody": true, "tfoot": true,
	"ul": true, "ol": true, "blockquote": true, "pre": true, "hr": true,
	"section": true, "article": true, "header": true, "footer": true, "nav": true,
}

// Inline element tags per Python INLINE_ELEMENTS in yj_to_epub_properties.py.
var inlineElementTags = map[string]bool{
	"a": true, "bdo": true, "br": true, "img": true,
	"object": true, "rp": true, "ruby": true, "span": true,
}

// Heritable CSS properties per Python (yj_to_epub_properties.py HERITABLE_PROPERTIES).
// Heritable CSS properties — the set of all properties that inherit from parent to child in CSS.
// Ported from Python HERITABLE_PROPERTIES (derived from HERITABLE_DEFAULT_PROPERTIES keys) in yj_to_epub_properties.py.
var heritableProperties = map[string]bool{
	"-amzn-page-align": true,
	"-kfx-attrib-xml-lang": true,
	"-kfx-link-color": true,
	"-kfx-user-margin-bottom-percentage": true,
	"-kfx-user-margin-left-percentage": true,
	"-kfx-user-margin-right-percentage": true,
	"-kfx-user-margin-top-percentage": true,
	"-kfx-visited-color": true,
	"-webkit-border-horizontal-spacing": true,
	"-webkit-border-vertical-spacing": true,
	"-webkit-text-stroke-color": true,
	"-webkit-text-stroke-width": true,
	"border-collapse": true,
	"border-spacing": true,
	"caption-side": true,
	"color": true,
	"cursor": true,
	"direction": true,
	"empty-cells": true,
	"font": true,
	"font-family": true,
	"font-feature-settings": true,
	"font-language-override": true,
	"font-kerning": true,
	"font-size": true,
	"font-size-adjust": true,
	"font-stretch": true,
	"font-style": true,
	"font-synthesis": true,
	"font-variant": true,
	"font-weight": true,
	"hanging-punctuation": true,
	"hyphens": true,
	"letter-spacing": true,
	"line-break": true,
	"line-height": true,
	"list-style": true,
	"list-style-image": true,
	"list-style-position": true,
	"list-style-type": true,
	"orphans": true,
	"overflow-wrap": true,
	"quotes": true,
	"ruby-align": true,
	"ruby-merge": true,
	"ruby-position": true,
	"tab-size": true,
	"text-align": true,
	"text-align-last": true,
	"text-combine-upright": true,
	"text-indent": true,
	"text-justify": true,
	"text-orientation": true,
	"text-shadow": true,
	"text-transform": true,
	"text-underline-position": true,
	"unicode-bidi": true,
	"visibility": true,
	"white-space": true,
	"widows": true,
	"word-break": true,
	"word-spacing": true,
	"word-wrap": true,
	"writing-mode": true,
}

// Comparison defaults for simplify_styles. These are intentionally not the same as the emitted
// body defaults set in setHTMLDefaults; Python compares body styles against generic document
// defaults, then keeps only residual body differences such as the chosen font family.
// Ported from Python HERITABLE_DEFAULT_PROPERTIES in yj_to_epub_properties.py.
// Only non-None entries are included (matching Python's HERITABLE_DEFAULTS_FILTERED).
var heritableDefaultProperties = map[string]string{
	"-amzn-page-align":                "none",
	"-kfx-user-margin-bottom-percentage": "100",
	"-kfx-user-margin-left-percentage":   "100",
	"-kfx-user-margin-right-percentage":  "100",
	"-kfx-user-margin-top-percentage":    "100",
	"-webkit-border-horizontal-spacing":  "2px",
	"-webkit-border-vertical-spacing":    "2px",
	"-webkit-text-stroke-width":          "0",
	"border-collapse":                   "separate",
	"border-spacing":                    "2px 2px",
	"caption-side":                      "top",
	"cursor":                            "auto",
	"direction":                         "ltr",
	"empty-cells":                       "show",
	"font-family":                       "serif",
	"font-kerning":                      "auto",
	"font-size":                         "1em",
	"font-size-adjust":                  "none",
	"font-stretch":                      "normal",
	"font-style":                        "normal",
	"font-synthesis":                    "weight style",
	"font-variant":                      "normal",
	"font-weight":                       "normal",
	"hanging-punctuation":               "none",
	"letter-spacing":                    "normal",
	"line-break":                        "auto",
	"line-height":                       "normal",
	"list-style-image":                  "none",
	"list-style-position":               "outside",
	"list-style-type":                   "disc",
	"orphans":                           "2",
	"overflow-wrap":                     "normal",
	"ruby-align":                        "space-around",
	"ruby-merge":                        "separate",
	"ruby-position":                     "over",
	"tab-size":                          "8",
	"text-align-last":                   "auto",
	"text-combine-upright":              "none",
	"text-indent":                       "0",
	"text-justify":                      "auto",
	"text-orientation":                  "mixed",
	"text-shadow":                       "none",
	"text-transform":                    "none",
	"text-underline-position":           "auto",
	"unicode-bidi":                      "normal",
	"visibility":                        "visible",
	"white-space":                       "normal",
	"widows":                            "2",
	"word-break":                        "normal",
	"word-spacing":                      "normal",
	"word-wrap":                         "normal",
	"writing-mode":                      "horizontal-tb",
	"-webkit-writing-mode":              "horizontal-tb",
}

// Non-heritable CSS properties with their default values, used in simplify_styles for comparison.
// When an element's non-heritable property matches its default value, it can be stripped.
// Ported from Python NON_HERITABLE_DEFAULT_PROPERTIES in yj_to_epub_properties.py.
var nonHeritableDefaultProperties = map[string]string{
	"background-clip":     "border-box",
	"background-color":    "transparent",
	"background-image":    "none",
	"background-origin":   "padding-box",
	"background-position": "0% 0%",
	"background-repeat":   "repeat",
	"background-size":     "auto auto",
	"box-decoration-break": "slice",
	"box-sizing":          "content-box",
	"column-count":        "auto",
	"float":               "none",
	"margin-bottom":       "0",
	"margin-left":         "0",
	"margin-right":        "0",
	"margin-top":          "0",
	"overflow":            "visible",
	"padding-bottom":      "0",
	"padding-left":        "0",
	"padding-right":       "0",
	"padding-top":         "0",
	"page-break-after":    "auto",
	"page-break-before":   "auto",
	"page-break-inside":   "auto",
	"position":            "static",
	"text-decoration":     "none",
	"text-emphasis-position": "over right",
	"transform":           "none",
	"transform-origin":    "50% 50% 0",
	"vertical-align":      "baseline",
	"z-index":             "auto",
}

// knownStyles maps CSS property names to the set of valid values for validation purposes.
// Ported from Python KNOWN_STYLES (yj_to_epub_properties.py L724-865).
// Used by inventoryStyle to detect unexpected CSS property values at development time.
// The special value "*" matches any value.
var knownStyles = map[string]map[string]bool{
	"-amzn-float":                 setOf("bottom", "top", "top,bottom"),
	"-amzn-max-crop-percentage":   setOf("0 0 0 0"),
	"-amzn-page-align":            setOf("all", "bottom", "bottom,left", "bottom,left,right", "bottom,right", "left", "left,right", "right", "top", "top,bottom,left", "top,bottom,right", "top,left", "top,left,right", "top,right"),
	"-amzn-page-footer":           setOf("disable"),
	"-amzn-page-header":           setOf("disable"),
	"-amzn-shape-outside":         setOf("*"),
	"-webkit-border-horizontal-spacing": setOf("0"),
	"-webkit-border-vertical-spacing":   setOf("0"),
	"-webkit-box-decoration-break":      setOf("clone", "slice"),
	"-webkit-hyphens":             setOf("auto", "manual", "none"),
	"-webkit-line-break":          setOf("anywhere", "auto", "loose", "normal", "strict"),
	"-webkit-ruby-position":       setOf("over", "under"),
	"-webkit-text-combine":        setOf("horizontal", "none"),
	"-webkit-text-emphasis-color": setOf("#0"),
	"-webkit-text-emphasis-position": setOf("over right", "under left"),
	"-webkit-text-emphasis-style": setOf(
		"filled", "filled circle", "filled dot", "filled double-circle", "filled sesame", "filled triangle",
		"open", "open circle", "open dot", "open double-circle", "open sesame", "open triangle"),
	"-webkit-text-orientation":    setOf("mixed", "sideways", "upright"),
	"-webkit-text-stroke-color":   setOf("#0"),
	"-webkit-text-stroke-width":   setOf("0"),
	"-webkit-transform":           setOf("*"),
	"-webkit-transform-origin":    setOf("0 0"),
	"-webkit-writing-mode":        setOf("horizontal-tb", "vertical-lr", "vertical-rl"),
	"background-clip":             setOf("border-box", "content-box", "padding-box"),
	"background-color":            setOf("#0"),
	"background-image":            setOf("*"),
	"background-origin":           setOf("border-box", "content-box"),
	"background-position":         setOf("0 0"),
	"background-size":             setOf("0 0", "auto 0", "0 auto", "contain", "cover"),
	"background-repeat":           setOf("no-repeat", "repeat-x", "repeat-y"),
	"border-bottom-color":         setOf("#0"),
	"border-bottom-left-radius":   setOf("0"),
	"border-bottom-right-radius":  setOf("0"),
	"border-bottom-style":         setOf("dashed", "dotted", "double", "groove", "inset", "none", "outset", "ridge", "solid"),
	"border-bottom-width":         setOf("0"),
	"border-collapse":             setOf("collapse", "separate"),
	"border-color":                setOf("#0"),
	"border-left-color":           setOf("#0"),
	"border-left-style":           setOf("dashed", "dotted", "double", "groove", "inset", "none", "outset", "ridge", "solid"),
	"border-left-width":           setOf("0"),
	"border-right-color":          setOf("#0"),
	"border-right-style":          setOf("dashed", "dotted", "double", "groove", "inset", "none", "outset", "ridge", "solid"),
	"border-right-width":          setOf("0"),
	"border-spacing":              setOf("0 0"),
	"border-style":                setOf("dashed", "dotted", "double", "groove", "inset", "none", "outset", "ridge", "solid"),
	"border-top-color":            setOf("#0"),
	"border-top-left-radius":      setOf("0"),
	"border-top-right-radius":     setOf("0"),
	"border-top-style":            setOf("dashed", "dotted", "double", "groove", "inset", "none", "outset", "ridge", "solid"),
	"border-top-width":            setOf("0"),
	"border-width":                setOf("0"),
	"bottom":                      setOf("0"),
	"box-decoration-break":        setOf("clone", "slice"),
	"box-shadow":                  setOf("0 0 #0", "0 0 #0 inset", "0 0 0 #0", "0 0 0 #0 inset", "0 0 0 0 #0", "0 0 0 0 #0 inset"),
	"box-sizing":                  setOf("border-box", "content-box"),
	"clear":                       setOf("both", "left", "none", "right"),
	"color":                       setOf("#0"),
	"column-count":                setOf("0", "auto"),
	"direction":                   setOf("ltr", "rtl"),
	"display":                     setOf("block", "inline", "inline-block", "none", "oeb-page-foot", "oeb-page-head", "inline-table"),
	"float":                       setOf("left", "right", "snap-block"),
	"font-family":                 setOf("*"),
	"font-size":                   setOf("0"),
	"font-style":                  setOf("italic", "normal", "oblique"),
	"font-variant":                setOf("normal", "small-caps"),
	"font-weight":                 setOf("0", "bold", "normal"),
	"height":                      setOf("0"),
	"hyphens":                     setOf("auto", "manual", "none"),
	"left":                        setOf("0"),
	"letter-spacing":              setOf("0", "normal"),
	"line-break":                  setOf("anywhere", "auto", "loose", "normal", "strict"),
	"line-height":                 setOf("0", "normal"),
	"list-style-image":            setOf("*"),
	"list-style-position":         setOf("inside", "outside"),
	"list-style-type":             setOf(
		"circle", "cjk-ideographic", "decimal", "decimal-leading-zero", "disc", "georgian", "katakana", "katakana-iroha",
		"lower-alpha", "lower-armenian", "lower-greek", "lower-roman", "none", "square", "upper-alpha", "upper-armenian",
		"upper-greek", "upper-roman"),
	"margin":                      setOf("0"),
	"margin-bottom":               setOf("0"),
	"margin-left":                 setOf("0", "auto"),
	"margin-right":                setOf("0", "auto"),
	"margin-top":                  setOf("0"),
	"max-height":                  setOf("0"),
	"max-width":                   setOf("0"),
	"min-height":                  setOf("0"),
	"min-width":                   setOf("0"),
	"orphans":                     setOf("0"),
	"outline-color":               setOf("#0"),
	"outline-offset":              setOf("0"),
	"outline-style":               setOf("dashed", "dotted", "double", "groove", "inset", "none", "outset", "ridge", "solid"),
	"outline-width":               setOf("0"),
	"overflow":                    setOf("hidden"),
	"padding":                     setOf("0"),
	"padding-bottom":              setOf("0"),
	"padding-left":                setOf("0"),
	"padding-right":               setOf("0"),
	"padding-top":                 setOf("0"),
	"page-break-after":            setOf("always", "auto", "avoid"),
	"page-break-before":           setOf("always", "auto", "avoid"),
	"page-break-inside":           setOf("auto", "avoid"),
	"position":                    setOf("absolute", "fixed", "relative"),
	"right":                       setOf("0"),
	"ruby-align":                  setOf("center", "space-around", "space-between", "start"),
	"ruby-position":               setOf("over", "under"),
	"src":                         setOf("*"),
	"text-align":                  setOf("center", "justify", "left", "right"),
	"text-align-last":             setOf("auto", "center", "end", "justify", "left", "right", "start"),
	"text-combine-upright":        setOf("all", "none"),
	"text-decoration":             setOf(
		"line-through", "none !important", "overline", "underline",
		"overline dashed", "overline dotted", "overline double",
		"underline dashed", "underline dotted", "underline double",
		"line-through dashed", "line-through dotted", "line-through double",
		"overline underline", "line-through overline", "line-through underline",
		"line-through overline underline"),
	"text-decoration-color":       setOf("#0"),
	"text-emphasis-color":         setOf("#0"),
	"text-emphasis-position":      setOf("over right", "under left"),
	"text-emphasis-style":         setOf(
		"filled", "filled circle", "filled dot", "filled double-circle", "filled sesame", "filled triangle",
		"open", "open circle", "open dot", "open double-circle", "open sesame", "open triangle"),
	"text-indent":                 setOf("0"),
	"text-orientation":            setOf("mixed", "sideways", "upright"),
	"text-shadow":                 setOf("*"),
	"text-transform":              setOf("capitalize", "none", "lowercase", "uppercase"),
	"top":                         setOf("0"),
	"transform":                   setOf("*"),
	"transform-origin":            setOf("0 0"),
	"unicode-bidi":                setOf("bidi-override", "embed", "isolate", "isolate-override", "normal", "plaintext"),
	"vertical-align":              setOf("0", "baseline", "bottom", "middle", "sub", "super", "text-bottom", "text-top", "top"),
	"visibility":                  setOf("hidden", "visible"),
	"white-space":                 setOf("normal", "nowrap"),
	"widows":                      setOf("0"),
	"width":                       setOf("0"),
	"word-break":                  setOf("break-all", "normal"),
	"word-spacing":                setOf("0", "normal"),
	"writing-mode":                setOf("horizontal-tb", "vertical-lr", "vertical-rl"),
	"z-index":                     setOf("0"),
}

// setOf is a helper to build map[string]bool sets from string arguments.
// Used for initializing knownStyles and similar validation maps.
func setOf(values ...string) map[string]bool {
	m := make(map[string]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}

func isReverseHeritableProperty(name string) bool {
	if !heritableProperties[name] {
		return false
	}
	// Ported from Python REVERSE_HERITABLE_PROPERTIES exclusions.
	switch name {
	case "-amzn-page-align",
		"-kfx-user-margin-bottom-percentage",
		"-kfx-user-margin-left-percentage",
		"-kfx-user-margin-right-percentage",
		"-kfx-user-margin-top-percentage",
		"font-size",
		"line-height":
		return false
	}
	return true
}

func allowsReverseInheritance(tag string) bool {
	return false
}

func mergeStyleMaps(dst map[string]string, src map[string]string) map[string]string {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = map[string]string{}
	}
	for name, value := range src {
		dst[name] = value
	}
	return dst
}

func equalStyleMaps(left map[string]string, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for name, value := range left {
		if right[name] != value {
			return false
		}
	}
	return true
}

func equalStringLists(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// overrideFFForEmptyContainers adds the resolved default font-family to empty structural
// container elements (divs with no text and no children) that don't already have font-family.
// This matches Python's reverse inheritance behavior where elements like s36C (a self-closing
// separator div) receive font-family from the body's pre-reverse-inheritance style when other
// children's font-family was promoted to the parent.
func overrideFFForEmptyContainers(root *htmlElement, resolvedDefaultFont string) {
	if root == nil {
		return
	}
	for _, child := range root.Children {
		elem, ok := child.(*htmlElement)
		if !ok || elem == nil {
			continue
		}
		// Only process empty structural containers: div/p with no text and no child elements
		if elem.Tag != "div" && elem.Tag != "p" {
			continue
		}
		hasChildren := false
		hasText := false
		for _, c := range elem.Children {
			switch ch := c.(type) {
			case *htmlElement:
				hasChildren = true
			case htmlText:
				if strings.TrimSpace(ch.Text) != "" {
					hasText = true
				}
			case *htmlText:
				if strings.TrimSpace(ch.Text) != "" {
					hasText = true
				}
			}
		}
		if hasChildren || hasText {
			continue
		}
		// Check if element doesn't have font-family in its style
		style := parseDeclarationString(elem.Attrs["style"])
		if _, hasFF := style["font-family"]; hasFF {
			continue
		}
		// Only add font-family to elements that already have non-trivial style properties
		// (border, margin, padding, etc.). Skip elements with only metadata properties
		// like -kfx-style-name or completely unstyled elements.
		if len(style) == 0 {
			continue
		}
		hasRealProperty := false
		for name := range style {
			if !strings.HasPrefix(name, "-kfx-") {
				hasRealProperty = true
				break
			}
		}
		if !hasRealProperty {
			continue
		}
		// Add the resolved default font-family
		style["font-family"] = resolvedDefaultFont
		setElementStyleString(elem, styleStringFromMap(style))
	}
}

func directChildElements(elem *htmlElement) []*htmlElement {
	if elem == nil || len(elem.Children) == 0 {
		return nil
	}
	children := make([]*htmlElement, 0, len(elem.Children))
	for _, child := range elem.Children {
		if el, ok := child.(*htmlElement); ok {
			children = append(children, el)
		}
	}
	return children
}

func rootHasDirectText(elem *htmlElement) bool {
	if elem == nil {
		return false
	}
	for _, child := range elem.Children {
		switch ch := child.(type) {
		case htmlText:
			if strings.TrimSpace(ch.Text) != "" {
				return true
			}
		case *htmlText:
			if strings.TrimSpace(ch.Text) != "" {
				return true
			}
		}
	}
	return false
}

func applyReverseInheritance(childElements []*htmlElement, currentStyle map[string]string) map[string]string {
	if len(childElements) == 0 {
		return nil
	}
	childProps := map[string]map[string]int{}
	for _, child := range childElements {
		for name, val := range parseDeclarationString(child.Attrs["style"]) {
			if !isReverseHeritableProperty(name) || val == "" {
				continue
			}
			if childProps[name] == nil {
				childProps[name] = map[string]int{}
			}
			childProps[name][val]++
		}
	}
	newHeritable := map[string]string{}
	for name, vals := range childProps {
		total := 0
		mostCommon := ""
		mostCommonCount := 0
		for val, count := range vals {
			total += count
			if count > mostCommonCount {
				mostCommon = val
				mostCommonCount = count
			}
		}
		if total < len(childElements) && currentStyle[name] == "" {
			continue
		}
		if float64(mostCommonCount) >= float64(len(childElements))*0.8 {
			newHeritable[name] = mostCommon
		}
	}
	if len(newHeritable) == 0 {
		return nil
	}
	for _, child := range childElements {
		childStyle := parseDeclarationString(child.Attrs["style"])
		for name, newHeritableVal := range newHeritable {
			if val, ok := childStyle[name]; ok {
				if val == newHeritableVal {
					delete(childStyle, name)
					// Also strip vendor-prefixed alternate equivalent
					if altName, ok := alternateEquivalentProperties[name]; ok {
						delete(childStyle, altName)
					}
				}
			} else if currentStyle[name] != "" && currentStyle[name] != newHeritableVal {
				childStyle[name] = currentStyle[name]
			}
		}
		setElementStyleString(child, styleStringFromMap(childStyle))
	}
	return newHeritable
}

func styleEntryForClassName(className string, catalog *styleCatalog) *styleEntry {
	if className == "" || catalog == nil {
		return nil
	}
	if entry, ok := catalog.byToken[className]; ok {
		return entry
	}
	for _, entry := range catalog.entries {
		if entry.finalName == className {
			return entry
		}
	}
	return nil
}

// styleMapFromClass parses CSS declarations from a single class name via the style catalog.
func styleMapFromClass(className string, catalog *styleCatalog) map[string]string {
	if className == "" || catalog == nil {
		return nil
	}
	if entry := styleEntryForClassName(className, catalog); entry != nil {
		return parseDeclarationString(entry.declarations)
	}
	if decls, ok := catalog.staticRules["."+className]; ok {
		return parseDeclarationString(decls)
	}
	return nil
}

func styleInfoFromClasses(classAttr string, catalog *styleCatalog) (baseName string, style map[string]string, preserved []string) {
	baseName = "class"
	for _, className := range strings.Fields(strings.TrimSpace(classAttr)) {
		if entry := styleEntryForClassName(className, catalog); entry != nil {
			if baseName == "class" && entry.baseName != "" {
				baseName = entry.baseName
			}
			style = mergeStyleMaps(style, parseDeclarationString(entry.declarations))
			continue
		}
		if decls, ok := catalog.staticRules["."+className]; ok {
			if baseName == "class" {
				baseName = className
			}
			style = mergeStyleMaps(style, parseDeclarationString(decls))
			continue
		}
		preserved = append(preserved, className)
	}
	if style == nil {
		style = map[string]string{}
	}
	return baseName, style, preserved
}

func applyStyleToElementClasses(elem *htmlElement, catalog *styleCatalog, baseName string, style map[string]string, preserved []string) {
	if elem == nil {
		return
	}
	className := strings.Join(preserved, " ")
	if len(style) > 0 && catalog != nil {
		if baseName == "" {
			baseName = "class"
		}
		if token := catalog.bind(baseName, declarationListFromStyleMap(style)); token != "" {
			className = appendClassNames(className, token)
		}
	}
	if className != "" {
		if elem.Attrs == nil {
			elem.Attrs = map[string]string{}
		}
		elem.Attrs["class"] = className
	} else if elem.Attrs != nil {
		delete(elem.Attrs, "class")
		if len(elem.Attrs) == 0 {
			elem.Attrs = nil
		}
	}
}

// simplifyState holds mutable state passed through the simplify_styles recursion.
// Python uses self.last_kfx_heading_level (instance variable) but Go needs explicit state.
type simplifyState struct {
	lastKfxHeadingLevel string // tracks last seen heading level (Python: self.last_kfx_heading_level)
	resourceDims        map[string][2]int // image dimensions by filename (for vh/vw cross-conversion)
	sectionFilename     string            // current section filename (for resolving img src paths)
}

func simplifyStylesElementFull(elem *htmlElement, catalog *styleCatalog, inherited map[string]string, state *simplifyState, parentKnownWidth ...bool) (containsBlock, containsText, containsImage bool) {
	if elem == nil {
		return false, false, false
	}

	// Ported from Python simplify_styles (yj_to_epub_properties.py:1677):
	// Clone inherited to prevent mutations from corrupting the caller's parentStyle map.
	inherited = cloneStyleMap(inherited)

	explicitStyle := parseDeclarationString(elem.Attrs["style"])
	sty := cloneStyleMap(inherited)

	// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1678-1679):
	// If inherited font-size is in em units, normalize it to "1em" so that relative
	// font-size values are not double-scaled. Without this, a nested element with
	// font-size: 0.75rem inside a parent with font-size: 0.75em would incorrectly
	// compute 0.75*0.75 instead of 0.75.
	if fsQty, fsUnit := splitCSSValue(inherited["font-size"]); fsQty != nil && fsUnit == "em" {
		inherited["font-size"] = "1em"
		sty["font-size"] = "1em"
	}

	for name, val := range explicitStyle {
		sty[name] = val
	}

	// Save pre-conversion font-size for parentStyle (Python: orig_sty["font-size"]).
	// Python passes the pre-conversion font-size to children via parent_sty so that
	// children doing rem→em conversion can use it as the base (in rem units).
	origFontSize, hasOrigFontSize := sty["font-size"]

	// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1690-1697):
	// Strip positioning properties (top/bottom/left/right) when position is static
	// and the value is "0". Non-zero values with static position are logged as warnings.
	if sty["position"] == "static" || sty["position"] == "" {
		for _, positioning := range []string{"top", "bottom", "left", "right"} {
			if val, ok := sty[positioning]; ok {
				if val == "0" {
					delete(sty, positioning)
				}
			}
		}
	}

	// Ported from Python simplify_styles (yj_to_epub_properties.py line 1699):
	// Synthesize border-spacing from -webkit-border-horizontal-spacing and
	// -webkit-border-vertical-spacing.
	hSpacing := sty["-webkit-border-horizontal-spacing"]
	vSpacing := sty["-webkit-border-vertical-spacing"]
	if hSpacing != "" && vSpacing != "" {
		sty["border-spacing"] = hSpacing + " " + vSpacing
	}

	// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1701-1706):
	// Convert -kfx-user-margin-*-percentage to -amzn-page-align.
	var pageAlignSides []string
	for _, side := range []string{"top", "bottom", "left", "right"} {
		key := "-kfx-user-margin-" + side + "-percentage"
		if sty[key] == "-100" {
			pageAlignSides = append(pageAlignSides, side)
		}
	}
	pageAlign := "none"
	if len(pageAlignSides) > 0 {
		if len(pageAlignSides) >= 4 {
			pageAlign = "all"
		} else {
			pageAlign = strings.Join(pageAlignSides, ",")
		}
	}
	sty["-amzn-page-align"] = pageAlign

	// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1708-1711):
	// Discard invalid negative padding values.
	for _, name := range []string{"padding", "padding-top", "padding-bottom", "padding-left", "padding-right"} {
		if val, ok := sty[name]; ok && strings.HasPrefix(val, "-") {
			delete(sty, name)
		}
	}

	// Convert lh/rem units to em before anything else.
	// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1713-1752).
	// Must happen before parentStyle computation so children receive converted units.
	// Convert the full sty map (inherited + explicit) so that inherited lh/rem values
	// are also normalized before being passed to children.
	convertStyleUnits(sty, inherited)
	// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1753-1785, VAL-FIX-013):
	// Convert vh/vw viewport units to percentages, including cross-conversion for images
	// with wrong-axis units (e.g. width:50vh → height:100% using image aspect ratio).
	convertViewportUnits(sty, elem, state.resourceDims, state.sectionFilename)
	// Sync explicitStyle with the converted values of properties that were explicitly set.
	for name := range explicitStyle {
		if val, ok := sty[name]; ok {
			explicitStyle[name] = val
		}
	}

	// Ported from Python simplify_styles (yj_to_epub_properties.py L1675, L1788-1798):
	// Convert percentage margins to pixel values for elements without a known width.
	// Python passes known_width as a parameter (default=True) through recursion.
	// - "width" in sty → known_width = True (explicit width means % margins can be resolved)
	// - display == "inline-block" → known_width = False (no width context for % margins)
	// - Otherwise → inherit parent's known_width value
	// When known_width is False, % margins are converted to px using PX_PER_PERCENT (8.534).
	knownWidth := true // default parameter value from Python (L1675)
	if len(parentKnownWidth) > 0 {
		knownWidth = parentKnownWidth[0]
	}
	if _, hasWidth := sty["width"]; hasWidth {
		knownWidth = true
	} else if sty["display"] == "inline-block" {
		knownWidth = false
	}
	if !knownWidth {
		// Choose horizontal or vertical margins based on writing-mode.
		marginNames := []string{"margin-left", "margin-right"}
		if sty["writing-mode"] != "horizontal-tb" {
			marginNames = []string{"margin-bottom", "margin-top"}
		}
		for _, name := range marginNames {
			if val, ok := sty[name]; ok {
				quantity, unit := splitCSSValue(val)
				if unit == "%" && quantity != nil {
					sty[name] = formatCSSQuantity(math.Round(*quantity*pxPerPercent)) + "px"
				}
			}
		}
	}

	// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1801-1802):
	// Remove outline-width when outline-style is none.
	if _, hasOutlineWidth := sty["outline-width"]; hasOutlineWidth {
		if sty["outline-style"] == "none" || sty["outline-style"] == "" {
			delete(sty, "outline-width")
		}
	}

	// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1804-1827):
	// OL/UL start attribute management.
	if elem.Tag == "ol" {
		if startStr, ok := elem.Attrs["start"]; ok {
			if startStr == "1" {
				delete(elem.Attrs, "start")
			}
		}
	} else if elem.Tag == "ul" {
		if _, ok := elem.Attrs["start"]; ok {
			delete(elem.Attrs, "start")
		}
	}

	// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1832-1838):
	// Convert background-image + -amzn-max-crop-percentage to background-size.
	if sty["background-image"] != "" && sty["background-image"] != "none" {
		cropVal := sty["-amzn-max-crop-percentage"]
		if cropVal == "0 0 0 0" {
			delete(sty, "-amzn-max-crop-percentage")
			sty["background-size"] = "contain"
		} else if cropVal == "0 100 100 0" {
			delete(sty, "-amzn-max-crop-percentage")
			sty["background-size"] = "cover"
		}
	}

	// Link color handling — ported from Python simplify_styles (yj_to_epub_properties.py lines 1828-1831).
	// When an <a> tag has no color set but has -kfx-link-color == -kfx-visited-color, set color to that value.
	if elem.Tag == "a" {
		if _, hasColor := sty["color"]; !hasColor {
			linkColor, hasLink := sty["-kfx-link-color"]
			visitedColor, hasVisited := sty["-kfx-visited-color"]
			if hasLink && hasVisited && linkColor == visitedColor {
				sty["color"] = linkColor
				explicitStyle["color"] = linkColor
			}
		}
	}

	parentStyle := map[string]string{}
	for name, val := range sty {
		if heritableProperties[name] {
			parentStyle[name] = val
		}
	}
	// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1847-1849):
	// Override font-size in parentStyle with the pre-conversion value so that children
	// doing rem→em conversion use the correct base. If we pass the converted value (e.g.,
	// "0.75em"), children's rem conversion would use that as the base and produce wrong
	// results (e.g., 0.75*0.75=0.5625 instead of 0.75).
	if hasOrigFontSize {
		parentStyle["font-size"] = origFontSize
	}

	childElements := []*htmlElement{}
	hasDirectText := false
	for _, child := range elem.Children {
		switch ch := child.(type) {
		case *htmlElement:
			childBlock, childText, childImage := simplifyStylesElementFull(ch, catalog, parentStyle, state, knownWidth)
			containsBlock = containsBlock || childBlock
			containsText = containsText || childText
			containsImage = containsImage || childImage
			childElements = append(childElements, ch)
		case htmlText:
			if strings.TrimSpace(ch.Text) != "" {
				containsText = true
				hasDirectText = true
			}
		case *htmlText:
			if strings.TrimSpace(ch.Text) != "" {
				containsText = true
				hasDirectText = true
			}
		}
	}

	if len(childElements) > 0 && !hasDirectText {
		for name, val := range applyReverseInheritance(childElements, sty) {
			sty[name] = val
			explicitStyle[name] = val
		}
	}

	newChildren := make([]htmlPart, 0, len(elem.Children))
	for _, child := range elem.Children {
		ch, ok := child.(*htmlElement)
		if !ok {
			newChildren = append(newChildren, child)
			continue
		}
		chClass := ""
		chStyle := ""
		if ch.Attrs != nil {
			chClass = ch.Attrs["class"]
			chStyle = ch.Attrs["style"]
		}
		if ch.Tag == "span" && strings.TrimSpace(chClass) == "" && strings.TrimSpace(chStyle) == "" && len(ch.Attrs) == 0 {
			newChildren = append(newChildren, ch.Children...)
			continue
		}
		newChildren = append(newChildren, ch)
	}
	elem.Children = newChildren

	// Merge non-heritable defaults into inherited before comparison/stripping.
	// Ported from Python: inherited_properties.update(self.non_heritable_default_properties)
	// This ensures the stripping loop at the end also removes non-heritable properties
	// that match their default values (e.g. margin: 0, padding: 0, box-sizing: content-box).
	for name, val := range nonHeritableDefaultProperties {
		inherited[name] = val
	}

	// For div elements (and elements that would have been divs in Python but are
	// already p/heading/figure in Go), add non-heritable defaults to the explicit style
	// when not already present. Ported from Python simplify_styles:
	//   if elem.tag == "div": sty.update(self.non_heritable_default_properties, replace=False)
	// In Python, headings and paragraphs start as divs and get these defaults added before
	// being converted. In Go, they start as their final tag, so we need to add the defaults
	// for these tags too. This ensures margin-top: 0 and margin-bottom: 0 are present in
	// the style for comparison against paragraph (1em) or heading (popped) inherited values.
	needsNonHeritableDefaults := elem.Tag == "div" || elem.Tag == "p" ||
		elem.Tag == "h1" || elem.Tag == "h2" || elem.Tag == "h3" ||
		elem.Tag == "h4" || elem.Tag == "h5" || elem.Tag == "h6" ||
		elem.Tag == "figure"
	if needsNonHeritableDefaults {
		for name, val := range nonHeritableDefaultProperties {
			if _, ok := explicitStyle[name]; !ok {
				explicitStyle[name] = val
				sty[name] = val
			}
		}
	}

	tagChangedToParagraph := false
	tagChangedToFigure := false
	tagChangedToHeading := false

	// Python simplify_styles (yj_to_epub_properties.py:1919-1945):
	//   if elem.tag == "div" and not (fixed_layout or illustrated_layout or has_conditional_content):
	if elem.Tag == "div" {
		// Extract heading level. Python (line 1858):
		//   kfx_heading_level = sty.pop("-kfx-heading-level", self.last_kfx_heading_level)
		// Go stores it as a data attribute on the <div> from rendering.
		kfxHeadingLevel := ""
		if dataLevel, ok := elem.Attrs["data-kfx-heading-level"]; ok {
			kfxHeadingLevel = dataLevel
			delete(elem.Attrs, "data-kfx-heading-level")
		}
		if kfxHeadingLevel == "" {
			kfxHeadingLevel = state.lastKfxHeadingLevel
		}

		kfxLayoutHints := sty["-kfx-layout-hints"]

		// Branch A: heading (Python line 1922)
		//   if "heading" in kfx_layout_hints and not contains_block_elem:
		if strings.Contains(kfxLayoutHints, "heading") && !containsBlock && kfxHeadingLevel != "" {
			elem.Tag = "h" + kfxHeadingLevel
			tagChangedToHeading = true
			// Python pops font-size, font-weight from inherited (line 1926-1927)
			delete(inherited, "font-size")
			delete(inherited, "font-weight")
			// Python pops margins based on writing-mode (line 1929-1932)
			if sty["writing-mode"] == "horizontal-tb" || sty["writing-mode"] == "" {
				delete(inherited, "margin-top")
				delete(inherited, "margin-bottom")
			} else {
				delete(inherited, "margin-left")
				delete(inherited, "margin-right")
			}
		} else if strings.Contains(kfxLayoutHints, "figure") && containsImage {
			// Branch B: figure (Python line 1934)
			//   elif "figure" in kfx_layout_hints and contains_image and not self.epub2_desired:
			elem.Tag = "figure"
			tagChangedToFigure = true
		} else if containsText && !containsBlock {
			// Branch C: paragraph (Python line 1941)
			//   elif contains_text and not contains_block_elem:
			elem.Tag = "p"
			tagChangedToParagraph = true
		}

		// Update last heading level state (Python: self.last_kfx_heading_level = ...)
		state.lastKfxHeadingLevel = kfxHeadingLevel
	}

	comparisonInherited := inherited
	if tagChangedToParagraph || tagChangedToFigure || elem.Tag == "p" || elem.Tag == "figure" {
		comparisonInherited = cloneStyleMap(inherited)
		if sty["writing-mode"] == "horizontal-tb" || sty["writing-mode"] == "" {
			comparisonInherited["margin-top"] = "1em"
			comparisonInherited["margin-bottom"] = "1em"
		} else {
			comparisonInherited["margin-left"] = "1em"
			comparisonInherited["margin-right"] = "1em"
		}
	}
	if tagChangedToHeading || elem.Tag == "h1" || elem.Tag == "h2" || elem.Tag == "h3" || elem.Tag == "h4" || elem.Tag == "h5" || elem.Tag == "h6" {
		comparisonInherited = cloneStyleMap(inherited)
		// Python pops font-size from inherited (line 1928) but then re-adds it as "1em" (line 1956).
		// The net effect: font-size in sty matches inherited → stripped. Don't delete font-size.
		delete(comparisonInherited, "font-weight")
		// Ported from Python simplify_styles: for heading conversion, pop margin-top/margin-bottom
		// (or margin-left/margin-right for vertical writing mode) from inherited so that
		// explicit zero margins are NOT stripped (they don't match the empty/nonexistent inherited value).
		if sty["writing-mode"] == "horizontal-tb" || sty["writing-mode"] == "" {
			delete(comparisonInherited, "margin-top")
			delete(comparisonInherited, "margin-bottom")
		} else {
			delete(comparisonInherited, "margin-left")
			delete(comparisonInherited, "margin-right")
		}
	}

	// Ported from Python simplify_styles (yj_to_epub_properties.py line 1956):
	// After unit conversion and all processing, if sty's font-size is in em units,
	// re-normalize inherited font-size to "1em" so the stripping loop will correctly
	// strip font-size when it matches (e.g. inherited "1.125rem" → converted to "1em"
	// in sty, but inherited still has "1.125rem" → mismatch → not stripped → wrong).
	//
	// Python modifies inherited_properties in-place and uses it directly in the stripping
	// loop (line 1966). Go uses a separate comparisonInherited clone (captured earlier for
	// paragraph/figure margin overrides). We must normalize BOTH so the stripping loop sees
	// the correct font-size regardless of which variable it reads.
	if fsQty, fsUnit := splitCSSValue(sty["font-size"]); fsQty != nil && fsUnit == "em" {
		inherited["font-size"] = "1em"
		// comparisonInherited may be a separate clone — sync font-size into it too.
		// If it aliases inherited (no clone was needed), this is a no-op.
		comparisonInherited["font-size"] = "1em"
	}

	// Python stripping loop (yj_to_epub_properties.py lines 1966-1968):
	//   for name, val in list(sty.items()):
	//       if val == inherited_properties.get(name, ""): sty.pop(name)
	// This iterates the FULL merged sty (inherited + explicit + defaults),
	// removing any property that matches the inherited value.
	// Then self.set_style(elem, sty) stores the remaining properties.
	//
	// Additionally, when a property is stripped, also strip its vendor-prefixed
	// alternate equivalent (e.g., strip hyphens → also strip -webkit-hyphens).
	// In Python, add_composite_and_equivalent_styles adds the prefix BEFORE
	// simplify_styles, so both get stripped together. In Go, the prefix is baked
	// into the rendering style string, so stripping the original leaves the prefix.
	for name, val := range sty {
		if name == "-kfx-style-name" || name == "-kfx-layout-hints" {
			continue
		}
		if comparisonInherited[name] == val {
			delete(sty, name)
			// Also strip the vendor-prefixed alternate equivalent.
			if altName, ok := alternateEquivalentProperties[name]; ok {
				delete(sty, altName)
			}
		}
		// Check if this is a vendor-prefixed alternate whose original was stripped.
		for origName, altName := range alternateEquivalentProperties {
			if name == altName && comparisonInherited[origName] == val {
				delete(sty, name)
				delete(sty, origName)
				break
			}
		}
	}

	// After stripping, store the remaining sty as the element's style string.
	// Python does self.set_style(elem, sty) which stores the full remaining style.
	// Keep all properties including -kfx-style-name and -kfx-layout-hints for class naming.
	// Python's convert_styles_to_classes later pops these to derive the class name.

	// When a div was converted to a figure during simplify_styles, rename the class
	// prefix from "class_" to "figure_" to match Python's convert_styles_to_classes.
	// Python: fixup_styles_and_classes derives class_name_prefix from -kfx-layout-hints.
	// If hints contain "figure", the prefix becomes "figure". In Go, the layout hints
	// are stored as metadata in the style string, but after tag conversion the element's
	// tag determines the prefix via styleMetadataForBaseName. For elements that start as
	// div but become figure during simplify_styles, we must also update the style name
	// prefix so the class name matches Python's.
	if tagChangedToFigure || elem.Tag == "figure" {
		if sname := sty["-kfx-style-name"]; sname != "" && !strings.HasPrefix(sname, "figure_") {
			sty["-kfx-style-name"] = "figure_" + sname
		}
	}

	// Ported from Python fixup_styles_and_classes class naming (yj_to_epub_properties.py L1521-1540):
	// When a div was converted to a heading during simplify_styles, update the style name
	// prefix to "heading_" so that classPrefixFromStyle produces the correct "heading" prefix.
	// Python: class_name_prefix = "-".join(sorted(list(style.pop("-kfx-layout-hints").split())))
	// When hints contain "heading", prefix becomes "heading". The style name itself may
	// already have "heading_" from the rendering pipeline, but for divs that were converted
	// to headings during simplify_styles (which Python does here but Go does in rendering),
	// we need to ensure the prefix is correct.
	if tagChangedToHeading {
		if sname := sty["-kfx-style-name"]; sname != "" && !strings.HasPrefix(sname, "heading_") {
			sty["-kfx-style-name"] = "heading_" + sname
		}
	}

	setElementStyleString(elem, styleStringFromMap(sty))

	// Strip background-* properties when background-color is transparent and background-image is none.
	// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1951-1953).
	if (sty["background-color"] == "transparent" || sty["background-color"] == "") &&
		(sty["background-image"] == "none" || sty["background-image"] == "") {
		for _, name := range []string{
			"background-clip", "background-origin", "background-position",
			"background-repeat", "background-size",
		} {
			delete(sty, name)
		}
	}

	// Collapse composite side: when all 4 individual sides of margin/padding/border-width/etc
	// are equal, collapse them into the shorthand property.
	// Ported from Python add_composite_and_equivalent_styles COMPOSITE_SIDE_STYLES loop.
	// Operates on sty (post-stripping) matching Python which runs this after simplify_styles.
	for _, entry := range compositeSideStyles {
		combinedProp := entry[0]
		individualProps := strings.Fields(entry[1])
		val, ok := sty[individualProps[0]]
		if !ok {
			continue
		}
		allEqual := true
		for _, individualProp := range individualProps[1:] {
			if sty[individualProp] != val {
				allEqual = false
				break
			}
		}
		if allEqual {
			sty[combinedProp] = val
			for _, individualProp := range individualProps {
				delete(sty, individualProp)
			}
		}
	}

	// Ineffective property warning logging for inline elements.
	// Ported from Python add_composite_and_equivalent_styles (yj_to_epub_properties.py lines 1995-2016).
	// modify=False — log only, never remove properties.
	display := sty["display"]
	isInline := display == "inline" || (inlineElementTags[elem.Tag] && display != "block" && display != "inline-block")
	if isInline {
		var ineffectiveProps []string
		for _, name := range []string{
			"list-style-image", "list-style-position", "list-style-type",
			"column-count", "text-align", "text-align-last", "text-indent",
		} {
			if _, ok := sty[name]; ok {
				ineffectiveProps = append(ineffectiveProps, name)
			}
		}
		if elem.Tag != "img" {
			for _, name := range []string{
				"height", "width", "max-height", "max-width",
				"-amzn-page-align", "-amzn-page-footer", "-amzn-page-header",
			} {
				if _, ok := sty[name]; ok {
					ineffectiveProps = append(ineffectiveProps, name)
				}
			}
			if sty["white-space"] != "nowrap" {
				if _, ok := sty["overflow"]; ok {
					ineffectiveProps = append(ineffectiveProps, "overflow")
				}
			}
		}
		if len(ineffectiveProps) > 0 {
			styleName := sty["-kfx-style-name"]
			if styleName == "" {
				styleName = "?"
			}
			var propParts []string
			for _, name := range ineffectiveProps {
				propParts = append(propParts, name+": "+sty[name])
			}
			fmt.Fprintf(os.Stderr, "kfx: warning: ineffective properties in %s element for kfx-style %s: %s\n",
				elem.Tag, styleName, strings.Join(propParts, "; "))
		}
	}

	// Remove -kfx-link-color and -kfx-visited-color from final styles.
	// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1962-1964).
	// For <a> tags, remove from inherited (so children don't inherit them).
	// For other tags, remove from sty (so they don't appear in the output).
	if elem.Tag == "a" {
		delete(inherited, "-kfx-link-color")
		delete(inherited, "-kfx-visited-color")
	}
	delete(sty, "-kfx-link-color")
	delete(sty, "-kfx-visited-color")

	// If a span has no remaining style after stripping (all properties were inherited/stripped),
	// clear its class attribute so the parent's span unwrapping pass will remove it.
	// This matches Python behavior where simplify_styles strips inherited properties and
	// the resulting empty-styled spans are effectively no-ops.
	// Check for any non-metadata properties remaining in sty.
	hasCSSProps := false
	for name := range sty {
		if !strings.HasPrefix(name, "-kfx-") {
			hasCSSProps = true
			break
		}
	}
	if elem.Tag == "span" && !hasCSSProps {
		if elem.Attrs != nil {
			delete(elem.Attrs, "class")
			if len(elem.Attrs) == 0 {
				elem.Attrs = nil
			}
		}
	}

	containsBlock = containsBlock || blockLevelTags[elem.Tag]
	containsImage = containsImage || elem.Tag == "img" || elem.Tag == "svg"
	return containsBlock, containsText, containsImage
}

func walkHTMLElement(root *htmlElement, visit func(*htmlElement)) {
	if root == nil || visit == nil {
		return
	}
	visit(root)
	for _, child := range root.Children {
		if elem, ok := child.(*htmlElement); ok {
			walkHTMLElement(elem, visit)
		}
	}
}

// walkDescendantElements visits only the descendant elements of root (not root itself).
// Equivalent to Python's e.iterfind(".//*") which yields descendants only.
func walkDescendantElements(root *htmlElement, visit func(*htmlElement)) {
	if root == nil || visit == nil {
		return
	}
	for _, child := range root.Children {
		if elem, ok := child.(*htmlElement); ok {
			walkHTMLElement(elem, visit)
		}
	}
}

// elemHasDirectText returns true if the element has any text content directly
// inside it (equivalent to Python lxml's e.text being non-empty/non-nil).
// In Go's DOM, direct text is represented by htmlText nodes in Children.
func elemHasDirectText(elem *htmlElement) bool {
	if elem == nil {
		return false
	}
	for _, child := range elem.Children {
		if txt, ok := child.(htmlText); ok && txt.Text != "" {
			return true
		}
	}
	return false
}

// elemHasTailText returns true if the element has tail text within the context
// of the root element. In Python lxml, ex.tail is text that follows an element
// within its parent. In Go's DOM, we need to check if the element appears as a
// child and the next sibling is a text node.
func elemHasTailText(elem *htmlElement, root *htmlElement) bool {
	if elem == nil || root == nil {
		return false
	}
	// Search for elem in root's descendants and check for tail text
	var findTail func(parent *htmlElement) bool
	findTail = func(parent *htmlElement) bool {
		for i, child := range parent.Children {
			if childElem, ok := child.(*htmlElement); ok {
				if childElem == elem {
					// Found the element; check if next sibling is text
					if i+1 < len(parent.Children) {
						if txt, ok := parent.Children[i+1].(htmlText); ok && txt.Text != "" {
							return true
						}
					}
					return false
				}
				if findTail(childElem) {
					return true
				}
			}
		}
		return false
	}
	return findTail(root)
}

// setElemStyle updates or removes the style attribute on an element from a style map.
// If the map is empty, the style attribute is removed entirely.
func setElemStyle(elem *htmlElement, style map[string]string) {
	if len(style) == 0 {
		delete(elem.Attrs, "style")
		if len(elem.Attrs) == 0 {
			elem.Attrs = nil
		}
	} else {
		elem.Attrs["style"] = styleStringFromMap(style)
	}
}

// rootHasBlockChildren returns true if the root element has any direct children
// that are block-level elements (div, table, p, h1-h6, figure, aside, li, td).
// Used to determine whether a promoted body should get paragraph-level margins:
// bodies with block children stay as <div> in Python (no margins), while bodies
// with only inline children get converted to <p> (margins preserved).
func rootHasBlockChildren(root *htmlElement) bool {
	if root == nil {
		return false
	}
	blockTags := map[string]bool{
		"div": true, "table": true, "p": true,
		"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
		"figure": true, "aside": true, "li": true, "td": true,
	}
	for _, child := range root.Children {
		if elem, ok := child.(*htmlElement); ok {
			if blockTags[elem.Tag] {
				return true
			}
		}
	}
	return false
}

func sanitizeCSSClassComponent(value string) string {
	value = regexp.MustCompile(`[^A-Za-z0-9_-]`).ReplaceAllString(value, "_")
	return strings.TrimSpace(value)
}

func classPrefixFromStyle(style map[string]string) string {
	hints := strings.Fields(strings.TrimSpace(style["-kfx-layout-hints"]))
	if len(hints) == 0 {
		return "class"
	}
	sort.Strings(hints)
	return strings.Join(hints, "-")
}

// ---------------------------------------------------------------------------
// Merged from css_values.go (origin: yj_to_epub_properties.py)
// ---------------------------------------------------------------------------


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
	if value := cssFontFamily(style["font_family"]); value != "" {
		declarations = append(declarations, "font-family: "+value)
	}
	if value := cssLengthProperty(style["font_size"], "font_size"); value != "" && value != "1em" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := mapFontStyle(style["font_style"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-style: "+value)
	}
	if value := mapFontWeight(style["font_weight"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-weight: "+value)
	}
	if value := mapFontVariant(style["glyph_transform"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-variant: "+value)
	}
	if value := cssLineHeight(style["line_height"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["margin_bottom"], "margin_bottom"); value != "" {
		declarations = append(declarations, "margin-bottom: "+value)
	}
	if value := cssLengthProperty(style["margin_left"], "margin_left"); value != "" {
		declarations = append(declarations, "margin-left: "+value)
	}
	if value := cssLengthProperty(style["margin_right"], "margin_right"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := cssLengthProperty(style["margin_top"], "margin_top"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	}
	if value := cssLengthProperty(style["padding_top"], "padding_top"); value != "" {
		declarations = append(declarations, "padding-top: "+value)
	}
	if value := mapBoxAlign(style["text_alignment"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	if value := cssLengthProperty(style["text_indent"], "text_indent"); value != "" {
		declarations = append(declarations, "text-indent: "+value)
	}
	if value := mapPageBreak(style["break_inside"]); value != "" {
		declarations = append(declarations, "page-break-inside: "+value)
	}
	if color := cssColor(style["border_color_top"]); color != "" {
		declarations = append(declarations, "border-top-color: "+color)
	}
	if value := mapBorderStyle(style["border_style_top"]); value != "" {
		declarations = append(declarations, "border-top-style: "+value)
	}
	if value := cssLengthProperty(style["border_weight_top"], "border_weight_top"); value != "" {
		declarations = append(declarations, "border-top-width: "+value)
	}
	if value := fillColor(style); value != "" {
		declarations = append(declarations, "background-color: "+value)
	}
	if value := mapTextTransform(style["text_transform"]); value != "" && value != "none" {
		declarations = append(declarations, "text-transform: "+value)
	}
	return declarations
}

func bodyStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssFontFamily(style["font_family"]); value != "" {
		declarations = append(declarations, "font-family: "+value)
	}
	if value := mapFontStyle(style["font_style"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-style: "+value)
	}
	if value := mapFontVariant(style["glyph_transform"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-variant: "+value)
	}
	if value := cssLengthProperty(style["font_size"], "font_size"); value != "" && value != "1em" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := cssLineHeight(style["line_height"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["margin_bottom"], "margin_bottom"); value != "" {
		declarations = append(declarations, "margin-bottom: "+value)
	}
	if value := cssLengthProperty(style["margin_left"], "margin_left"); value != "" {
		declarations = append(declarations, "margin-left: "+value)
	}
	if value := cssLengthProperty(style["margin_right"], "margin_right"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := cssLengthProperty(style["margin_top"], "margin_top"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	}
	if value := mapBoxAlign(style["box_align"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	} else if value := mapBoxAlign(style["text_alignment"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	if value := cssLengthProperty(style["text_indent"], "text_indent"); value != "" {
		if value == "0" {
			goto skipBodyIndent
		}
		declarations = append(declarations, "text-indent: "+value)
	}
skipBodyIndent:
	if value := fillColor(style); value != "" {
		declarations = append(declarations, "background-color: "+value)
	}
	if value := mapTextTransform(style["text_transform"]); value != "" && value != "none" {
		declarations = append(declarations, "text-transform: "+value)
	}
	return declarations
}

func spanStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssLengthProperty(style["font_size"], "font_size"); value != "" && value != "1em" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := cssFontFamily(style["font_family"]); value != "" {
		declarations = append(declarations, "font-family: "+value)
	}
	if value := mapFontStyle(style["font_style"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-style: "+value)
	}
	if value := mapFontWeight(style["font_weight"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-weight: "+value)
	}
	if value := mapFontVariant(style["glyph_transform"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-variant: "+value)
	}
	if value := cssLineHeight(style["line_height"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["margin_right"], "margin_right"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := mapTextTransform(style["text_transform"]); value != "" && value != "none" {
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
	if value := cssLengthProperty(style["margin_top"], "margin_top"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	}
	if value := cssLengthProperty(style["margin_left"], "margin_left"); value != "" {
		declarations = append(declarations, "margin-left: "+value)
	}
	align := mapBoxAlign(style["box_align"])
	if value := cssLengthProperty(style["margin_right"], "margin_right"); value != "" && align != "left" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := cssLengthProperty(style["max_width"], "max_width"); value != "" {
		declarations = append(declarations, "max-width: "+value)
	}
	if align == "left" {
		declarations = append(declarations, "margin-right: auto")
	}
	if color := cssColor(style["border_color"]); color != "" {
		declarations = append(declarations, "border-color: "+color)
	}
	return declarations
}

func tableColumnStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssLengthProperty(style["width"], "width"); value != "" {
		declarations = append(declarations, "width: "+value)
	}
	return declarations
}

func structuredContainerDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := mapFontWeight(style["font_weight"]); value != "" {
		declarations = append(declarations, "font-weight: "+value)
	}
	return declarations
}

func tableCellStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssLengthProperty(style["margin_right"], "margin_right"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := mapBoxAlign(style["text_alignment"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	if value := mapTableVerticalAlign(style["yj.vertical_align"]); value != "" {
		declarations = append(declarations, "vertical-align: "+value)
	}
	return declarations
}

func headingStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := mapHyphens(style["hyphens"]); value != "" {
		declarations = append(declarations, "-webkit-hyphens: "+value)
	}
	if value := cssFontFamily(style["font_family"]); value != "" {
		declarations = append(declarations, "font-family: "+value)
	}
	if value := mapFontStyle(style["font_style"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-style: "+value)
	}
	if value := cssLengthProperty(style["font_size"], "font_size"); value != "" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := mapFontWeight(style["font_weight"]); value != "" {
		declarations = append(declarations, "font-weight: "+value)
	} else {
		declarations = append(declarations, "font-weight: normal")
	}
	if value := mapFontVariant(style["glyph_transform"]); value != "" {
		declarations = append(declarations, "font-variant: "+value)
	}
	if value := mapHyphens(style["hyphens"]); value != "" {
		declarations = append(declarations, "hyphens: "+value)
	}
	if value := cssLineHeight(style["line_height"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["margin_bottom"], "margin_bottom"); value != "" {
		declarations = append(declarations, "margin-bottom: "+value)
	} else {
		declarations = append(declarations, "margin-bottom: 0")
	}
	if value := cssLengthProperty(style["margin_top"], "margin_top"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	} else {
		declarations = append(declarations, "margin-top: 0")
	}
	if value := mapPageBreak(style["yj_break_after"]); value != "" {
		declarations = append(declarations, "page-break-after: "+value)
	}
	if value := mapBoxAlign(style["text_alignment"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	if value := cssLengthProperty(style["text_indent"], "text_indent"); value != "" {
		declarations = append(declarations, "text-indent: "+value)
	}
	if value := cssLengthProperty(style["margin_left"], "margin_left"); value != "" {
		declarations = append(declarations, "margin-left: "+value)
	}
	if value := cssLengthProperty(style["margin_right"], "margin_right"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := mapTextDecoration(style["underline"]); value != "" {
		declarations = append(declarations, "text-decoration: "+value)
	}
	if value := mapTextTransform(style["text_transform"]); value != "" {
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
	case "lh":
		return formatStyleNumber(magnitude * 1.2)
	case "em", "rem":
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
	case "lh":
		return formatStyleNumber(magnitude*1.2) + "em"
	case "em", "rem":
		return formatStyleNumber(magnitude) + "em"
	case "percent":
		return formatStyleNumber(magnitude) + "%"
	case "pt":
		if magnitude > 0 && int(magnitude*1000)%225 == 0 {
			return formatStyleNumber(float64(int(magnitude*1000))/450.0) + "px"
		}
		return formatStyleNumber(magnitude) + "pt"
	case "px":
		return formatStyleNumber(magnitude) + "px"
	default:
		if property == "width" || property == "height" || property == "margin_top" || property == "margin_bottom" || property == "font_size" {
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
	rawMagnitude, okMagnitude := mapField(value, "value")
	rawUnit, okUnit := mapField(value, "unit")
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
	_, hasColor := style["fill_color"]
	_, hasOpacity := style["fill_opacity"]
	if !hasColor && !hasOpacity {
		return ""
	}
	color := cssColor(style["fill_color"])
	if color == "" {
		color = "#ffffff"
	}
	opacity, _, ok := numericStyleValue(style["fill_opacity"])
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
		if value := cssColor(source["link_visited_style"]); value != "" {
			return value
		}
		if value := cssColor(source["link_unvisited_style"]); value != "" {
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
	raw, ok := mapField(value, "text_color")
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

// ---------------------------------------------------------------------------
// Merged from yj_property_info.go (origin: yj_to_epub_properties.py)
// ---------------------------------------------------------------------------


// -----------------------------------------------------------------------
// Prop / YJ_PROPERTY_INFO  (Python: class Prop + YJ_PROPERTY_INFO dict)
// -----------------------------------------------------------------------

// propInfo mirrors Python's Prop(name, values=None).
type propInfo struct {
	name   string            // CSS property name (or -kfx- internal name)
	values map[string]string // optional symbol → CSS value map; nil means "pass through"
}

// yjPropertyInfo is the Go equivalent of Python's YJ_PROPERTY_INFO dict.
// Keys are KFX property IDs (e.g. "font_family", "margin_top").  The table is a direct port
// of the Python data at yj_to_epub_properties.py L84-626.
var yjPropertyInfo = map[string]propInfo{
	"background_image": {"background-image", nil},
	"background_positionx": {"-kfx-background-positionx", nil},
	"background_positiony": {"-kfx-background-positiony", nil},
	"background_origin": {"background-origin", map[string]string{"border_bounds": "border-box", "content_bounds": "content-box", "padding_bounds": "padding-box"}},
	"background_repeat": {"background-repeat", map[string]string{"no_repeat": "no-repeat", "repeat_x": "repeat-x", "repeat_y": "repeat-y"}},
	"background_sizex": {"-kfx-background-sizex", nil},
	"background_sizey": {"-kfx-background-sizey", nil},
	"baseline_shift":  {"-kfx-baseline-shift", nil},
	"baseline_style":  {"-kfx-baseline-style", map[string]string{"bottom": "bottom", "center": "middle", "normal": "baseline", "subscript": "sub", "superscript": "super", "text_bottom": "text-bottom", "text_top": "text-top", "top": "top"}},
	"bidi_direction": {"direction", map[string]string{"ltr": "ltr", "rtl": "rtl"}},
	"bidi_unicode": {"unicode-bidi", map[string]string{"bidi_embed": "embed", "isolate": "isolate", "isolate_override": "isolate-override", "normal": "normal", "override": "bidi-override", "plaintext": "plaintext"}},

	"border_color": {"border-color", nil},
	"border_color_bottom": {"border-bottom-color", nil},
	"border_color_left": {"border-left-color", nil},
	"border_color_right": {"border-right-color", nil},
	"border_color_top": {"border-top-color", nil},

	"border_radius_bottom_left": {"border-bottom-left-radius", nil},
	"border_radius_bottom_right": {"border-bottom-right-radius", nil},
	"border_radius_top_left": {"border-top-left-radius", nil},
	"border_radius_top_right": {"border-top-right-radius", nil},

	"border_spacing_horizontal": {"-webkit-border-horizontal-spacing", nil},
	"border_spacing_vertical": {"-webkit-border-vertical-spacing", nil},

	"border_style": {"border-style", borderStyles},
	"border_style_bottom": {"border-bottom-style", borderStyles},
	"border_style_left": {"border-left-style", borderStyles},
	"border_style_right": {"border-right-style", borderStyles},
	"border_style_top": {"border-top-style", borderStyles},

	"border_weight": {"border-width", nil},
	"border_weight_bottom": {"border-bottom-width", nil},
	"border_weight_left": {"border-left-width", nil},
	"border_weight_right": {"border-right-width", nil},
	"border_weight_top": {"border-top-width", nil},

	"bottom":  {"bottom", nil},
	"box_align": {"-kfx-box-align", map[string]string{"center": "center", "left": "left", "right": "right"}},
	"break_after": {"page-break-after", map[string]string{"always": "always", "auto": "auto", "avoid": "avoid"}},
	"break_before": {"page-break-before", map[string]string{"always": "always", "auto": "auto", "avoid": "avoid"}},
	"break_inside": {"page-break-inside", map[string]string{"auto": "auto", "avoid": "avoid"}},
	"character_width": {"-kfx-character-width", map[string]string{"auto": ""}}, // None in Python → empty
	"clip": {"overflow", map[string]string{"false": "visible", "true": "hidden"}},
	"column_count": {"column-count", map[string]string{"auto": "auto"}},
	"column_rule_color": {"column-rule-color", nil},
	"direction": {"direction", map[string]string{"ltr": "ltr", "rtl": "rtl"}},
	"draw_spanning_borders":  {"box-decoration-break", map[string]string{"false": "slice", "true": "clone"}},
	"fill_bounds":  {"background-clip", map[string]string{"border_bounds": "border-box", "content_bounds": "content-box", "padding_bounds": "padding-box"}},
	"fill_color":  {"-kfx-fill-color", nil},
	"fill_opacity":  {"-kfx-fill-opacity", nil},
	"float": {"float", map[string]string{"left": "left", "right": "right", "snap_block": "snap-block"}},

	"font_family":  {"font-family", nil},
	"font_size":  {"font-size", nil},
	"font_stretch":  {"font-stretch", map[string]string{"condensed": "condensed", "expanded": "expanded", "normal": "normal", "semi_condensed": "semi-condensed", "semi_expanded": "semi-expanded"}},
	"font_style":  {"font-style", map[string]string{"italic": "italic", "normal": "normal", "oblique": "oblique"}},
	"font_weight":  {"font-weight", map[string]string{"bold": "bold", "heavy": "900", "light": "300", "medium": "500", "normal": "normal", "semi_bold": "600", "thin": "100", "ultra_bold": "800", "ultra_light": "200"}},
	"glyph_transform": {"font-variant", map[string]string{"none": "normal", "small_caps": "small-caps"}},
	"height":  {"height", nil},
	"hide_empty_cells": {"empty-cells", map[string]string{"false": "show", "true": "hide"}},
	"hyphens": {"hyphens", map[string]string{"auto": "auto", "manual": "manual", "none": "none"}},
	"keep_lines_together": {"-kfx-keep-lines-together", nil},
	"language":  {"-kfx-attrib-xml-lang", nil},
	"layout_hints": {"-kfx-layout-hints", nil},
	"left":  {"left", nil},
	"letterspacing":  {"letter-spacing", nil},
	"line_break": {"line-break", map[string]string{"anywhere": "anywhere", "auto": "auto", "loose": "loose", "normal": "normal", "strict": "strict"}},
	"line_height":  {"line-height", map[string]string{"auto": "normal"}},
	"link_unvisited_style": {"-kfx-link-color", nil},
	"link_visited_style": {"-kfx-visited-color", nil},

	"list_style": {"list-style-type", map[string]string{
		"alpha_lower": "lower-alpha", "alpha_upper": "upper-alpha", "circle": "circle",
		"cjk_earthly_branch": "cjk-earthly-branch", "cjk_heavenly_stem": "cjk-heavenly-stem", "cjk_ideographic": "cjk-ideographic",
		"decimal_leading_zero": "decimal-leading-zero", "disc": "disc", "georgian": "georgian",
		"hiragana": "hiragana", "hiragana_iroha": "hiragana-iroha", "image": "",
		"japanese_formal": "japanese-formal", "japanese_informal": "japanese-informal",
		"katakana": "katakana", "katakana_iroha": "katakana-iroha",
		"lower_armenian": "lower-armenian", "lower_greek": "lower-greek", "none": "none",
		"numeric": "decimal", "roman_lower": "lower-roman", "roman_upper": "upper-roman",
		"simp_chinese_formal": "simp-chinese-formal", "simp_chinese_informal": "simp-chinese-informal",
		"square": "square", "trad_chinese_formal": "trad-chinese-formal", "trad_chinese_informal": "trad-chinese-informal",
		"upper_armenian": "upper-armenian", "upper_greek": "upper-greek"}},
	"list_style_image": {"list-style-image", nil},
	"list_style_position": {"list-style-position", map[string]string{"inside": "inside", "outside": "outside"}},

	"margin": {"margin", nil},
	"margin_bottom": {"margin-bottom", nil},
	"margin_left": {"margin-left", nil},
	"margin_right": {"margin-right", nil},
	"margin_top": {"margin-top", nil},

	"max_height": {"max-height", nil},
	"max_width": {"max-width", nil},
	"min_height": {"min-height", nil},
	"min_width": {"min-width", nil},

	"nobreak": {"white-space", map[string]string{"false": "normal", "true": "nowrap"}},
	"outline_color": {"outline-color", nil},
	"outline_offset": {"outline-offset", nil},
	"outline_style": {"outline-style", borderStyles},
	"outline_weight": {"outline-width", nil},

	"overline": {"text-decoration", map[string]string{"dashed": "overline dashed", "dotted": "overline dotted", "double": "overline double", "none": "", "solid": "overline"}},
	"overline_color": {"text-decoration-color", nil},

	"padding": {"padding", nil},
	"padding_bottom": {"padding-bottom", nil},
	"padding_left": {"padding-left", nil},
	"padding_right": {"padding-right", nil},
	"padding_top": {"padding-top", nil},

	"position": {"position", map[string]string{"fixed": "absolute", "footer": "oeb-page-foot", "header": "oeb-page-head", "relative": "relative", "viewport": "fixed"}},
	"right":  {"right", nil},

	"ruby_base_align": {"ruby-align", map[string]string{"center": "center", "space_around": "space-around", "space_between": "space-between", "start": "start"}},
	"ruby_merge": {"ruby-merge", map[string]string{"collapse": "collapse", "separate": "separate"}},
	"ruby_position_horizontal": {"ruby-position", map[string]string{"bottom": "under", "top": "over"}},
	"ruby_position_vertical": {"ruby-position", map[string]string{"left": "under", "right": "over"}},
	"ruby_text_align": {"ruby-align", map[string]string{"center": "center", "space_around": "space-around", "space_between": "space-between", "start": "start"}},

	"shadows": {"box-shadow", nil},
	"sizing_bounds": {"box-sizing", map[string]string{"border_bounds": "border-box", "content_bounds": "content-box", "padding_bounds": "padding-box"}},
	"src":  {"src", nil},

	"strikethrough": {"text-decoration", map[string]string{"dashed": "line-through dashed", "dotted": "line-through dotted", "double": "line-through double", "none": "", "solid": "line-through"}},
	"strikethrough_color": {"text-decoration-color", nil},

	"stroke_color": {"-webkit-text-stroke-color", nil},

	"stroke_dasharray": {"-svg-stroke-dasharray", nil},
	"stroke_dashoffset": {"-svg-stroke-dashoffset", nil},
	"stroke_linecap":  {"-svg-stroke-linecap", map[string]string{"butt": "butt", "round": "round", "square": "square"}},
	"stroke_linejoin": {"-svg-stroke-linejoin", map[string]string{"bevel": "bevel", "miter": "miter", "round": "round"}},
	"stroke_miterlimit": {"-svg-stroke-miterlimit", nil},
	"stroke_width":  {"-webkit-text-stroke-width", nil},

	"style_name": {"-kfx-style-name", nil},

	"table_border_collapse": {"border-collapse", map[string]string{"false": "separate", "true": "collapse"}},
	"table_column_span": {"-kfx-attrib-colspan", nil},
	"table_row_span": {"-kfx-attrib-rowspan", nil},

	"text_alignment": {"text-align", map[string]string{"center": "center", "justify": "justify", "left": "left", "right": "right"}},
	"text_alignment_last": {"text-align-last", map[string]string{"auto": "auto", "center": "center", "end": "end", "justify": "justify", "left": "left", "right": "right", "start": "start"}},

	"text_background_color":  {"background-color", nil},
	"text_background_image": {"background-image", nil},
	"text_color":  {"color", nil},

	"text_combine": {"text-combine-upright", map[string]string{"all": "all"}},
	"text_emphasis_color": {"text-emphasis-color", nil},
	"text_emphasis_position_horizontal": {"-kfx-text-emphasis-position-horizontal", map[string]string{"top": "over", "bottom": "under"}},
	"text_emphasis_position_vertical": {"-kfx-text-emphasis-position-vertical", map[string]string{"left": "left", "right": "right"}},
	"text_emphasis_style": {"text-emphasis-style", map[string]string{
		"filled": "filled", "filled_circle": "filled circle", "filled_dot": "filled dot",
		"filled_double_circle": "filled double-circle", "filled_sesame": "filled sesame", "filled_triangle": "filled triangle",
		"open": "open", "open_circle": "open circle", "open_dot": "open dot",
		"open_double_circle": "open double-circle", "open_sesame": "open sesame", "open_triangle": "open triangle"}},
	"text_orientation": {"text-orientation", map[string]string{"auto": "mixed", "sideways": "sideways", "upright": "upright"}},
	"text_indent":  {"text-indent", nil},
	"text_transform":  {"text-transform", map[string]string{"lowercase": "lowercase", "none": "none", "titlecase": "capitalize", "uppercase": "uppercase"}},
	"text_shadows": {"text-shadow", nil},
	"top":  {"top", nil},
	"transform":  {"transform", nil},
	"transform_origin": {"transform-origin", nil},

	"underline": {"text-decoration", map[string]string{"dashed": "underline dashed", "dotted": "underline dotted", "double": "underline double", "none": "", "solid": "underline"}},
	"underline_color": {"text-decoration-color", nil},

	"visibility": {"visibility", map[string]string{"false": "hidden", "true": "visible"}},
	"white_space": {"white-space", map[string]string{"nowrap": "nowrap"}},
	"width":  {"width", nil},
	"word_break": {"word-break", map[string]string{"break_all": "break-all", "normal": "normal"}},
	"wordspacing":  {"word-spacing", nil},
	"writing_mode": {"writing-mode", map[string]string{"horizontal_tb": "horizontal-tb", "vertical_rl": "vertical-rl", "vertical_lr": "vertical-lr"}},

	"yj.border_path": {"-amzn-shape-outside", nil},
	"yj.collision": {"-kfx-collision", nil},
	"yj.display": {"-kfx-attrib-epub-type", map[string]string{"yj.note": "noteref"}},
	"yj.float_align": {"yj-float-align", map[string]string{"top": ""}},
	"yj.float_bias": {"yj-float-bias", map[string]string{"yj.at": ""}},
	"yj.float_clear": {"clear", map[string]string{"left": "left", "right": "right", "both": "both", "none": "none"}},
	"yj.float_to_block": {"yj-float-to-block", map[string]string{"false": ""}},
	"yj.footer_overlay": {"-amzn-page-footer", map[string]string{"disabled": "disable", "enabled": "overlay"}},
	"yj.header_overlay": {"-amzn-page-header", map[string]string{"disabled": "disable", "enabled": "overlay"}},
	"yj.max_crop": {"-amzn-max-crop-percentage", nil},
	"yj.semantics.heading_level": {"-kfx-heading-level", nil},
	"yj.user_margin_bottom_percentage": {"-kfx-user-margin-bottom-percentage", nil},
	"yj.user_margin_left_percentage": {"-kfx-user-margin-left-percentage", nil},
	"yj.user_margin_right_percentage": {"-kfx-user-margin-right-percentage", nil},
	"yj.user_margin_top_percentage": {"-kfx-user-margin-top-percentage", nil},
	"yj.vertical_align": {"-kfx-table-vertical-align", map[string]string{"normal": "baseline", "bottom": "bottom", "center": "middle", "top": "top"}},
	"yj.viewer": {"-kfx-attrib-epub-type", map[string]string{"disabled": "amzn:decorative", "enabled": "amzn:not-decorative"}},
	"yj_break_after": {"page-break-after", map[string]string{"always": "always", "auto": "auto", "avoid": "avoid"}},
	"yj_break_before": {"page-break-before", map[string]string{"always": "always", "auto": "auto", "avoid": "avoid"}},
}

// borderStyles mirrors Python BORDER_STYLES.
var borderStyles = map[string]string{
	"none": "none", "solid": "solid", "dotted": "dotted", "dashed": "dashed",
	"double": "double", "ridge": "ridge", "groove": "groove", "inset": "inset", "outset": "outset",
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
	case "auto":
		return "auto"
	case "manual":
		return "manual"
	case "none":
		return "none"
	default:
		return ""
	}
}

func mapPageBreak(value interface{}) string {
	switch text, _ := asString(value); text {
	case "always":
		return "always"
	case "auto":
		return "auto"
	case "avoid":
		return "avoid"
	default:
		return ""
	}
}

func mapBorderStyle(value interface{}) string {
	switch text, _ := asString(value); text {
	case "none":
		return "none"
	case "solid":
		return "solid"
	case "dotted":
		return "dotted"
	case "dashed":
		return "dashed"
	case "double":
		return "double"
	case "ridge":
		return "ridge"
	case "groove":
		return "groove"
	case "inset":
		return "inset"
	case "outset":
		return "outset"
	default:
		return ""
	}
}

func mapBoxAlign(value interface{}) string {
	switch text, _ := asString(value); text {
	case "center":
		return "center"
	case "left":
		return "left"
	case "right":
		return "right"
	case "justify":
		return "justify"
	default:
		return ""
	}
}

func mapTableVerticalAlign(value interface{}) string {
	switch asStringDefault(value) {
	case "normal":
		return "baseline"
	case "bottom":
		return "bottom"
	case "center":
		return "middle"
	case "top":
		return "top"
	default:
		return ""
	}
}

func mapTextDecoration(value interface{}) string {
	switch text, _ := asString(value); text {
	case "solid":
		return "underline"
	default:
		return ""
	}
}

func mapFontVariant(value interface{}) string {
	switch text, _ := asString(value); text {
	case "small_caps":
		return "small-caps"
	case "none":
		return "normal"
	default:
		return ""
	}
}

func mapTextTransform(value interface{}) string {
	switch text, _ := asString(value); text {
	case "titlecase":
		return "capitalize"
	case "lowercase":
		return "lowercase"
	case "uppercase":
		return "uppercase"
	case "none":
		return "none"
	default:
		return ""
	}
}

func mapFontStyle(value interface{}) string {
	switch text, _ := asString(value); text {
	case "italic":
		return "italic"
	case "oblique":
		return "oblique"
	case "normal":
		return "normal"
	default:
		return ""
	}
}

func mapFontWeight(value interface{}) string {
	switch text, _ := asString(value); text {
	case "bold":
		return "bold"
	case "heavy":
		return "900"
	case "light":
		return "300"
	case "medium":
		return "500"
	case "normal":
		return "normal"
	case "semi_bold":
		return "600"
	case "thin":
		return "100"
	case "ultra_bold":
		return "800"
	case "ultra_light":
		return "200"
	default:
		return ""
	}
}

func mapFontStretch(value interface{}) string {
	switch text, _ := asString(value); text {
	case "condensed":
		return "condensed"
	case "expanded":
		return "expanded"
	case "normal":
		return "normal"
	case "semi_condensed":
		return "semi-condensed"
	case "semi_expanded":
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
	"em": "em",
	"ch": "ch",
	"cm": "cm",
	"ex": "ex",
	"in": "in",
	"lh": "lh",
	"mm": "mm",
	"percent": "%",
	"pt": "pt",
	"px": "px",
	"rem": "rem",
	"vh": "vh",
	"vmax": "vmax",
	"vmin": "vmin",
	"vw": "vw",
}

// -----------------------------------------------------------------------
// COLOR_YJ_PROPERTIES  (Python: COLOR_YJ_PROPERTIES)
// -----------------------------------------------------------------------

var colorYJProperties = map[string]bool{
	"border_color": true, "border_color_bottom": true, "border_color_left": true, "border_color_right": true, "border_color_top": true, "column_rule_color": true,
	"color": true, "fill_color": true, "footnote_line_color": true, "outline_color": true, "overline_color": true, "strikethrough_color": true, "stroke_color": true,
	"text_background_color": true, "text_color": true, "text_emphasis_color": true, "underline_color": true,
}

// -----------------------------------------------------------------------
// propertyValue  (Python: property_value, ~L1175)
//
// Converts a single KFX property value to a CSS string.
// This is the core of the data-driven pipeline.
// -----------------------------------------------------------------------

// ResourceResolver resolves a KFX resource symbol (e.g. "eAV") to a relative URL
// suitable for use in CSS url() values. Port of Python's self.process_external_resource
// call in property_value for $479/$175/$528 (yj_to_epub_properties.py:1272-1273).
// Returns empty string if the resource cannot be resolved.
type ResourceResolver func(symbol string) string

func propertyValue(propName string, yjValue interface{}, resolveResource ResourceResolver) string {
	if yjValue == nil {
		return ""
	}

	info, infoOK := yjPropertyInfo[propName]

	switch v := yjValue.(type) {

	// IonStruct — length, color, shadow, transform-origin, etc.
	case map[string]interface{}:
		return propertyValueStruct(propName, v, info, infoOK, resolveResource)

	// string — could be a raw string, an enum symbol, font-family, language, etc.
	case string:
		if propName == "font_family" {
			return cssFontFamily(v)
		}
		if propName == "language" {
			return v // language string, keep as-is
		}
		if propName == "style_name" {
			// -kfx-style-name: pass through (Python uses unique_part_of_local_symbol here)
			return v
		}
		// Python: property_value (yj_to_epub_properties.py:1272-1273):
		//   elif yj_property_name in {"background_image", "resource_name", "text_background_image"}:
		//       value = self.css_url(urlrelpath(
		//           self.process_external_resource(yj_value).filename, ref_from=self.STYLES_CSS_FILEPATH))
		// $479 = background-image, $528 = background-image (alt), $175 = resource name
		if (propName == "background_image" || propName == "resource_name" || propName == "text_background_image") && resolveResource != nil {
			if resolved := resolveResource(v); resolved != "" {
				return "url(" + resolved + ")"
			}
		}
		// Check if this is an enum symbol ("solid" etc.) that maps through propInfo.values
		if infoOK && info.values != nil {
			if mapped, ok := info.values[v]; ok {
				return mapped // may be "" for None-mapped values
			}
		}
		// Color properties: $349 means color(0) → transparent/black
		if colorYJProperties[propName] && v == "none" {
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
		return propertyValueList(propName, v, info, infoOK, resolveResource)
	}

	return fmt.Sprintf("%v", yjValue)
}

// propertyValueStruct handles struct-type KFX property values (lengths, colors, shadows, etc.).
func propertyValueStruct(propName string, v map[string]interface{}, info propInfo, infoOK bool, resolveResource ResourceResolver) string {
	// Length: {$307: magnitude, $306: unit}
	if mag, ok := asFloat64(v["value"]); ok {
		unitSym, _ := asString(v["unit"])
		unit := yjLengthUnits[unitSym]
		if unit == "" {
			unit = unitSym
		}
		if mag == 0 {
			return "0"
		}
		// FIX_PT_TO_PX: convert pt → px when magnitude is divisible by 0.225
		// Python: property_value ~L1190
		if unitSym == "pt" && mag > 0 {
			if int(mag*1000)%225 == 0 {
				mag = float64(int(mag*1000)) / 450.0
				unit = "px"
			}
		}
		return formatCSSQuantity(mag) + unit
	}

	// Color: {$19: int}
	if colorVal, ok := v["text_color"]; ok {
		return fixColorValue(colorVal)
	}

	// Shadow: {$499/$500/$501/$502/$498, optional $336 inset}
	// Port of Python yj_to_epub_properties.py L1207-1216: iterates sub-properties
	// (horizontal_offset, vertical_offset, blur, spread, color) and appends optional "inset".
	if _, has499 := v["horizontal_offset"]; has499 {
		if _, has500 := v["vertical_offset"]; has500 {
			parts := []string{}
			for _, sub := range []string{"horizontal_offset", "vertical_offset", "blur", "spread", "color"} {
				if subVal, ok := v[sub]; ok {
					parts = append(parts, propertyValue(sub, subVal, resolveResource))
				}
			}
			if _, inset := v["inset"]; inset {
				parts = append(parts, "inset")
			}
			return strings.Join(parts, " ")
		}
	}

	// transform-origin: {$58/$59}
	if _, hasTop := v["top"]; hasTop {
		if propName == "transform_origin" {
			parts := []string{}
			for _, sub := range []string{"left", "top"} {
				if subVal, ok := v[sub]; ok {
					parts = append(parts, propertyValue(sub, subVal, resolveResource))
				} else {
					parts = append(parts, "50%")
				}
			}
			return strings.Join(parts, " ")
		}
		// Rect-style value: top/right/bottom/left
		parts := []string{}
		for _, sub := range []string{"top", "right", "bottom", "left"} {
			if subVal, ok := v[sub]; ok {
				parts = append(parts, propertyValue(sub, subVal, resolveResource))
			}
		}
		return strings.Join(parts, " ")
	}

	// keep-lines-together: {$131/$132}
	if _, has131 := v["first"]; has131 {
		oVal := valueStr(v["first"])
		wVal := valueStr(v["last"])
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
const alphaMask = 0xff000000

func propertyValueNumeric(propName string, v float64, info propInfo, infoOK bool) string {
	// Color property
	if colorYJProperties[propName] {
		// Ported from Python property_value (yj_to_epub_properties.py L1290-1291):
		// When $70 (fill-color) has zero alpha bits, set alpha to fully opaque.
		// Python: if yj_property_name == "fill_color" and int(yj_value) & ALPHA_MASK == 0:
		//             value = int(yj_value) | ALPHA_MASK
		if propName == "fill_color" {
			i := int(v)
			if i&alphaMask == 0 {
				v = float64(i | alphaMask)
			}
		}
		return fixColorValue(v)
	}

	// Properties that stay as raw numbers (no px suffix)
	rawNumberProps := map[string]bool{
		"column_count": true, "font_weight": true, "table_column_span": true, "table_row_span": true,
		"yj.max_crop": true, "yj.min_aspect_ratio": true, "yj.max_aspect_ratio": true, "yj.semantics.heading_level": true,
		"yj.user_margin_bottom_percentage": true, "yj.user_margin_left_percentage": true, "yj.user_margin_right_percentage": true, "yj.user_margin_top_percentage": true,
		"fill_opacity": true, "dropcap_chars": true, "dropcap_lines": true, "line_height": true,
	}

	if rawNumberProps[propName] || v == 0 {
		return valueStr(v)
	}

	return valueStr(v) + "px"
}

// propertyValueList handles list-type KFX property values.
func propertyValueList(propName string, v []interface{}, info propInfo, infoOK bool, resolveResource ResourceResolver) string {
	switch propName {
	case "layout_hints": // layout hints
		// Ported from Python property_value (yj_to_epub_properties.py L1354-1363):
		//   for layout_hint in yj_value:
		//       element_name = LAYOUT_HINT_ELEMENT_NAMES.get(layout_hint)
		//       if element_name: values.append(element_name)
		//       else: log.warning(...)
		// LAYOUT_HINT_ELEMENT_NAMES maps $453→caption, $282→figure, $760→heading.
		hints := []string{}
		for _, item := range v {
			if s, ok := item.(string); ok {
				if mapped, ok := layoutHintElementNames[s]; ok {
					hints = append(hints, mapped)
				}
			}
		}
		return strings.Join(hints, " ")

	case "yj.collision": // collision
		vals := []string{}
		for _, item := range v {
			if s, ok := item.(string); ok {
				if mapped, ok := collisions[s]; ok {
					vals = append(vals, mapped)
				}
			}
		}
		// Ported from Python property_value (yj_to_epub_properties.py L1352):
		//   value = " ".join(sorted(values))
		sort.Strings(vals)
		return strings.Join(vals, " ")

	case "transform": // transform
		// Python yj_to_epub_properties.py L1345: value = self.process_transform(yj_value, svg)
		// In propertyValue (non-SVG) context, svg=False so process_transform uses "px" and "," separator.
		return processTransform(v, false)

	case "yj.border_path": // shape-outside (-amzn-shape-outside)
		// Python yj_to_epub_properties.py L1331-1332: value = self.process_polygon(yj_value)
		return processPolygon(v)

	case "text_shadows": // text-shadow list
		vals := make([]string, 0, len(v))
		for _, item := range v {
			vals = append(vals, propertyValue(propName, item, resolveResource))
		}
		return strings.Join(vals, ", ")

	case "stroke_dasharray": // stroke-dasharray
		vals := make([]string, 0, len(v))
		for _, item := range v {
			vals = append(vals, propertyValue(propName, item, resolveResource))
		}
		return strings.Join(vals, " ")
	}

	return fmt.Sprintf("%v", v)
}

// collisions mirrors Python COLLISIONS.
var collisions = map[string]string{
	"always": "always",
	"yj.queue": "queue",
}

// -----------------------------------------------------------------------
// convertYJProperties  (Python: convert_yj_properties, ~L1088)
//
// Takes a map of KFX property IDs → raw values and returns a flat
// CSS property → value map, exactly as Python's convert_yj_properties.
// -----------------------------------------------------------------------

func convertYJProperties(yjProperties map[string]interface{}, resolveResource ResourceResolver) (map[string]string, bool) {
	declarations := map[string]string{}
	textCombineInUse := false

	for yjPropName, yjValue := range yjProperties {
		value := propertyValue(yjPropName, yjValue, resolveResource)
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

		// Python (yj_to_epub_properties.py L1101-1102):
		//   if property == "position" and value in ["oeb-page-foot", "oeb-page-head"]:
		//       property = "display" if self.generate_epub2 and EMIT_OEB_PAGE_PROPS else None
		// Go generates EPUB3 (not EPUB2), and EMIT_OEB_PAGE_PROPS defaults to False,
		// so the property is always dropped (set to None in Python). The continue
		// skips adding this property to declarations, matching Python's behavior.
		if cssName == "position" && (value == "oeb-page-foot" || value == "oeb-page-head") {
			continue
		}

		// Python fix_language (yj_to_epub_properties.py:2089): normalize language tag.
		// "en-us" → "en-US", "pt-br" → "pt-BR".
		if cssName == "-kfx-attrib-xml-lang" {
			value = normalizeLanguageTag(value)
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

	// Ported from Python convert_yj_properties (yj_to_epub_properties.py L276-278):
	// When text-combine-upright is "all", remove writing-mode: horizontal-tb if present.
	// Python: if declarations.get("text-combine-upright") == "all": ...
	// Also set textCombineInUse flag (Python: self.text_combine_in_use = True, L1127).
	if declarations["text-combine-upright"] == "all" {
		textCombineInUse = true
		if declarations["writing-mode"] == "horizontal-tb" {
			delete(declarations, "writing-mode")
		}
	}

	// Ported from Python convert_yj_properties (yj_to_epub_properties.py L286-289):
	// When text-decoration-color is "rgba(255,255,255,0)" and text-decoration is absent,
	// remove the color and set text-decoration: none !important.
	// Python: if ("text-decoration-color" in declarations and "text-decoration" not in declarations and
	//             declarations["text-decoration-color"] == "rgba(255,255,255,0)"):
	//             declarations.pop("text-decoration-color")
	//             declarations["text-decoration"] = "none !important"
	if _, hasTDC := declarations["text-decoration-color"]; hasTDC {
		if _, hasTD := declarations["text-decoration"]; !hasTD {
			if declarations["text-decoration-color"] == "rgba(255,255,255,0)" {
				delete(declarations, "text-decoration-color")
				declarations["text-decoration"] = "none !important"
			}
		}
	}

	// Post-processing: -kfx-baseline-shift / -kfx-baseline-style / -kfx-table-vertical-align → vertical-align
	// Ported from Python fix_vertical_align_properties (yj_to_epub_content.py ~L1497-1514).
	// Python iterates over all three properties. For each found:
	//   - If no vertical-align exists → set it to the popped value
	//   - If vertical-align exists and differs → would create subcontainer span (handled at render level)
	//   - If vertical-align exists and matches → no-op
	// In Go's class-based system, we convert all three to vertical-align here.
	// The conflict case (different values) is rare; we use last-write-wins with the outermost property.
	for _, prop := range []string{"-kfx-baseline-shift", "-kfx-baseline-style", "-kfx-table-vertical-align"} {
		if val := popMap(declarations, prop, ""); val != "" {
			if _, exists := declarations["vertical-align"]; !exists || declarations["vertical-align"] != val {
				declarations["vertical-align"] = val
			}
		}
	}

	return declarations, textCombineInUse
}

// -----------------------------------------------------------------------
// processContentProperties  (Python: process_content_properties, ~L1081)
//
// Extracts KFX properties from a content dict, converts them, returns
// CSS declaration map.
// -----------------------------------------------------------------------

func processContentProperties(content map[string]interface{}, resolveResource ResourceResolver) map[string]string {
	contentProperties := map[string]interface{}{}
	for k := range content {
		if yjPropertyNames[k] {
			contentProperties[k] = content[k]
		}
	}
	css, _ := convertYJProperties(contentProperties, resolveResource)
	return css
}

// processContentPropertiesWithCombineFlag is like processContentProperties but also
// returns whether text-combine-upright: all was encountered, matching Python's
// self.text_combine_in_use flag (yj_to_epub_properties.py L1127).
func processContentPropertiesWithCombineFlag(content map[string]interface{}, resolveResource ResourceResolver) (map[string]string, bool) {
	contentProperties := map[string]interface{}{}
	for k := range content {
		if yjPropertyNames[k] {
			contentProperties[k] = content[k]
		}
	}
	return convertYJProperties(contentProperties, resolveResource)
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
		if seen[prop] || val == "" {
			continue
		}
		// Skip internal -kfx- properties EXCEPT -kfx-attrib-* which need to survive
		// into inline styles for later extraction as XML attributes (Python yj_to_epub_properties.py:1427).
		if strings.HasPrefix(prop, "-kfx-") && !strings.HasPrefix(prop, "-kfx-attrib-") {
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
	// pxPerPercent is PX_PER_PERCENT = 8.534 (Python yj_to_epub_properties.py:34).
	// Used to convert percentage margins to pixel values for elements without a known width.
	pxPerPercent = 8.534
)

// splitCSSValue splits a CSS value string into its numeric quantity and unit parts.
// Ported from Python split_value in epub_output.py.
// Returns (nil, val) if the value is not numeric.
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
				// Python L1734: base_font_size = inherited_properties["font-size"]
				// KeyError if missing; in Go we log error.
				var ok bool
				baseFontSize, ok = inherited["font-size"]
				if !ok || baseFontSize == "" {
					log.Printf("kfx: error: Cannot convert %s with missing inherited font-size", name)
					continue
				}
			} else {
				// Python L1737: base_font_size = orig_sty["font-size"]
				// KeyError if missing; in Go we log error.
				if hasOrigFontSize {
					baseFontSize = origFontSize
				} else {
					var ok bool
					baseFontSize, ok = sty["font-size"]
					if !ok || baseFontSize == "" {
						log.Printf("kfx: error: Cannot convert %s with missing element font-size", name)
						continue
					}
				}
			}

			baseQuantity, baseUnit := splitCSSValue(baseFontSize)
			if baseQuantity != nil {
				if baseUnit == "rem" {
					q = q / *baseQuantity
					unit = "em"
				} else if baseUnit == "em" {
					unit = "em"
				} else {
					// Python L1745: log.error("Cannot convert %s:%s with incorrect base font size units %s")
					log.Printf("kfx: error: Cannot convert %s:%s with incorrect base font size units %s", name, val, baseFontSize)
				}
			} else {
				log.Printf("kfx: error: Cannot convert %s:%s with invalid base font size %s", name, val, baseFontSize)
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
	}
}

// convertViewportUnits converts vh/vw viewport units to percentages for page-aligned content.
// Ported from Python simplify_styles (yj_to_epub_properties.py lines 1753-1785, VAL-FIX-013).
//
// Direct conversion: height in vh → height in %, width in vw → width in %
// Cross-conversion: width in vh → height in % (using image aspect ratio),
//
//	height in vw → width in % (using image aspect ratio)
//
// Cross-conversion requires image dimensions from resourceDimensions, looked up
// by resolving the img element's src attribute. Python uses self.oebps_files[filename]
// for this; Go uses the ResourceDimensions map populated during buildResources.
func convertViewportUnits(sty map[string]string, elem *htmlElement, resourceDimensions map[string][2]int, sectionFilename string) {
	pageAlign := sty["-amzn-page-align"]
	if pageAlign == "none" || pageAlign == "" {
		// Python L1783: log.error("viewport-based units with wrong property or without page-align")
		return
	}

	for name, val := range sty {
		quantity, unit := splitCSSValue(val)
		if quantity == nil {
			continue
		}
		if unit != "vh" && unit != "vw" {
			continue
		}
		if name != "height" && name != "width" {
			// Python L1783: log.error("viewport-based units with wrong property or without page-align")
			continue
		}

		// Python L1756: if name[0] != unit[1]
		// "width"[0]='w', "vh"[1]='h' → 'w' != 'h' → CROSS
		// "height"[0]='h', "vw"[1]='w' → 'h' != 'w' → CROSS
		// "height"[0]='h', "vh"[1]='h' → 'h' == 'h' → DIRECT
		// "width"[0]='w', "vw"[1]='w' → 'w' == 'w' → DIRECT
		if name[0] != unit[1] {
			// Cross-conversion: wrong-axis unit
			// Python L1757: if not ("height" in sty and "width" in sty)
			_, hasHeight := sty["height"]
			_, hasWidth := sty["width"]
			if hasHeight && hasWidth {
				// Python L1779: log.error("viewport-based units with wrong property: %s:%s")
				fmt.Fprintf(os.Stderr, "kfx: error: viewport-based units with wrong property: %s:%s\n", name, val)
				continue
			}

			if elem == nil || elem.Tag != "img" {
				// Python L1780: log.error("viewport-based units with wrong property on non-image: %s:%s")
				fmt.Fprintf(os.Stderr, "kfx: error: viewport-based units with wrong property on non-image: %s:%s\n", name, val)
				continue
			}

			// Look up image dimensions
			imgSrc := elem.Attrs["src"]
			imgWidth, imgHeight := lookupImageDimensions(resourceDimensions, imgSrc, sectionFilename)
			if imgWidth == 0 || imgHeight == 0 {
				fmt.Fprintf(os.Stderr, "kfx: error: cannot resolve image dimensions for cross-conversion: %s\n", imgSrc)
				continue
			}

			origProp := name
			delete(sty, origProp)

			q := *quantity
			if name == "width" {
				// Python L1769: quantity = (quantity * img_file.height) / img_file.width
				q = (q * float64(imgHeight)) / float64(imgWidth)
				name = "height"
			} else {
				// Python L1772: quantity = (quantity * img_file.width) / img_file.height
				q = (q * float64(imgWidth)) / float64(imgHeight)
				name = "width"
			}

			// Python L1774-1775: snap 99-101 to 100
			if q > 99.0 && q < 101.0 {
				q = 100.0
			} else {
				fmt.Fprintf(os.Stderr, "kfx: warning: converted %s:%s for img %dw x %dh to %s:%f%%\n",
					origProp, val, imgWidth, imgHeight, name, q)
			}

			sty[name] = formatCSSQuantity(q) + "%"
		} else {
			// Direct conversion: same-axis unit (height+vh or width+vw)
			sty[name] = formatCSSQuantity(*quantity) + "%"
		}
	}
}

// lookupImageDimensions resolves an img src to resource dimensions.
// Mirrors Python's get_url_filename(urlabspath(elem.get("src"), ref_from=book_part.filename))
// followed by self.oebps_files[filename].height/width.
func lookupImageDimensions(resourceDimensions map[string][2]int, src, refFrom string) (width, height int) {
	if resourceDimensions == nil || src == "" {
		return 0, 0
	}
	// Try direct filename match (mirrors lookupImageFile in yj_to_epub_misc.go)
	filename := src
	if idx := strings.LastIndex(src, "/"); idx >= 0 {
		filename = src[idx+1:]
	}
	if dims, ok := resourceDimensions[filename]; ok {
		return dims[0], dims[1]
	}
	if dims, ok := resourceDimensions[src]; ok {
		return dims[0], dims[1]
	}
	return 0, 0
}

// normalizeLanguageTag normalizes a BCP 47 language tag to match Python's fix_language
// (yj_to_epub_properties.py:2089). Converts underscores to hyphens, lowercases the primary
// subtag, and uppercases the secondary subtag (if less than 4 chars) or title-cases it.
func normalizeLanguageTag(lang string) string {
	if len(lang) > 2 && lang[2] == '_' {
		lang = strings.ReplaceAll(lang, "_", "-")
	}
	prefix, suffix, found := strings.Cut(lang, "-")
	if !found {
		return strings.ToLower(lang)
	}
	prefix = strings.ToLower(prefix)
	if len(suffix) < 4 {
		suffix = strings.ToUpper(suffix)
	} else {
		suffix = strings.ToUpper(suffix[:1]) + suffix[1:]
	}
	return prefix + "-" + suffix
}

// cssColorNames is the set of CSS named color keywords, used by zeroQuantity.
// Ported from Python COLOR_NAMES (yj_to_epub_properties.py L678).
var cssColorNames = map[string]bool{
	"black": true, "navy": true, "blue": true, "green": true, "teal": true,
	"lime": true, "cyan": true, "maroon": true, "purple": true, "olive": true,
	"gray": true, "red": true, "magenta": true, "yellow": true, "white": true,
}

// zeroQuantity normalizes CSS values for validation comparison.
// Ported from Python zero_quantity (yj_to_epub_properties.py L2469-2479).
// Returns "#0" for colors, "0" for numeric values, and the original value otherwise.
func zeroQuantity(val string) string {
	// Check for hex color
	if isHexColor.MatchString(val) {
		return "#0"
	}
	// Check for rgba() color
	if isRGBAColor.MatchString(val) {
		return "#0"
	}
	// Check for named color
	if cssColorNames[val] {
		return "#0"
	}
	// Check for numeric value with optional unit
	if isNumericValue.MatchString(val) {
		return "0"
	}
	return val
}

// Pre-compiled regexes for zeroQuantity (ported from Python zero_quantity regexes).
var (
	isHexColor   = regexp.MustCompile(`^#[0-9a-f]+$`)
	isRGBAColor  = regexp.MustCompile(`^rgba\([0-9]+,[0-9]+,[0-9]+,[0-9.]+\)$`)
	isNumericValue = regexp.MustCompile(`^[+-]?[0-9]+\.?[0-9]*(|em|ex|ch|rem|vw|vh|vmin|vmax|%|cm|mm|in|px|pt|pc)$`)
)

// inventoryStyle validates CSS property values against known valid values.
// Ported from Python inventory_style (yj_to_epub_properties.py L1594-1602).
// Logs unexpected style definitions for development-time regression detection.
func inventoryStyle(style map[string]string) {
	reported := map[[2]string]bool{}
	for key, value := range style {
		parts := strings.Split(value, " ")
		normalizedParts := make([]string, len(parts))
		for i, p := range parts {
			normalizedParts[i] = zeroQuantity(p)
		}
		simpleValue := strings.Join(normalizedParts, " ")
		validValues := knownStyles[key]
		if validValues == nil {
			continue
		}
		if validValues[simpleValue] || validValues["*"] {
			continue
		}
		pair := [2]string{key, value}
		if !reported[pair] {
			log.Printf("Unexpected style definition: %s: %s", key, value)
			reported[pair] = true
		}
	}
}
