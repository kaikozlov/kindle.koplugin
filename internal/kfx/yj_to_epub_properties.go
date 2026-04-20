// Stylesheet finalization and class reference scanning aligned with Calibre
// REFERENCE/Calibre_KFX_Input/kfxlib/yj_to_epub_properties.py (subset near create_css_files / OPF-adjacent helpers).
package kfx

import (
	"fmt"
	"os"
	"regexp"
	"sort"
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
	sort.SliceStable(sortedEntries, func(i, j int) bool {
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

	// Ported from Python beautify_html (epub_output.py:783-789):
	// Strip spans with no attributes (left over after REMOVE_EMPTY_NAMED_CLASSES).
	for i := range book.RenderedSections {
		beautifyHTML(book.RenderedSections[i].Root)
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
func beautifyHTML(root *htmlElement) {
	if root == nil {
		return
	}
	var walk func(elem *htmlElement)
	walk = func(elem *htmlElement) {
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
				walk(el)
			}
		}
	}
	walk(root)
}

// Port of KFX_EPUB_Properties.create_css_files (yj_to_epub_properties.py L2239+).
// Python writes STYLES_CSS_FILEPATH from font_faces + css_rules + media_queries; Go appends the style catalog
// string (dynamic rules from render) onto book.Stylesheet before finalizeStylesheet (render.go).
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

		simplifyStylesElementFull(bodyElem, catalog, bodyInherited)

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

func simplifyStylesElementFull(elem *htmlElement, catalog *styleCatalog, inherited map[string]string) (containsBlock, containsText, containsImage bool) {
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
	// Sync explicitStyle with the converted values of properties that were explicitly set.
	for name := range explicitStyle {
		if val, ok := sty[name]; ok {
			explicitStyle[name] = val
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
			childBlock, childText, childImage := simplifyStylesElementFull(ch, catalog, parentStyle)
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
	if elem.Tag == "div" {
		if strings.Contains(sty["-kfx-layout-hints"], "figure") && containsImage {
			elem.Tag = "figure"
			tagChangedToFigure = true
		} else if containsText && !containsBlock && !containsImage {
			elem.Tag = "p"
			tagChangedToParagraph = true
		}
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
	if elem.Tag == "h1" || elem.Tag == "h2" || elem.Tag == "h3" || elem.Tag == "h4" || elem.Tag == "h5" || elem.Tag == "h6" {
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
	if fsQty, fsUnit := splitCSSValue(sty["font-size"]); fsQty != nil && fsUnit == "em" {
		inherited["font-size"] = "1em"
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
	if tagChangedToFigure || elem.Tag == "figure" {
		if sname := sty["-kfx-style-name"]; sname != "" && !strings.HasPrefix(sname, "figure_") {
			sty["-kfx-style-name"] = "figure_" + sname
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
