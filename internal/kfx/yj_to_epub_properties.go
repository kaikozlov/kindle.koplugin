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
		if sortedEntries[i].count == sortedEntries[j].count {
			return sortedEntries[i].order < sortedEntries[j].order
		}
		return sortedEntries[i].count > sortedEntries[j].count
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
func setHTMLDefaults(book *decodedBook) {
	if book == nil {
		return
	}
	lang := normalizeLanguage(book.Language)
	for i := range book.RenderedSections {
		bodyStyle := parseDeclarationString(book.RenderedSections[i].BodyStyle)
		if bodyStyle["font-family"] == "" {
			bodyStyle["font-family"] = "FreeFontSerif,serif"
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
}

// Port of KFX_EPUB_Properties.fixup_styles_and_classes (yj_to_epub_properties.py ~L1388+).
func fixupStylesAndClasses(book *decodedBook, catalog *styleCatalog) {
	if book == nil || catalog == nil {
		return
	}

	addStaticBodyClasses(catalog)
	simplifyStylesFull(book, catalog)

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
		if book.RenderedSections[i].BodyStyle != "" {
			if bodyClass := staticBodyClassForDeclarations(declarationListFromStyleMap(stripStyleMetadata(parseDeclarationString(book.RenderedSections[i].BodyStyle)))); bodyClass != "" {
				book.RenderedSections[i].BodyClass = bodyClass
				book.RenderedSections[i].BodyStyle = ""
			} else {
				countStyle(book.RenderedSections[i].BodyStyle)
			}
		}
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

func addStaticBodyClasses(catalog *styleCatalog) {
	if catalog == nil {
		return
	}
	for _, bodyClass := range []string{"class-0", "class-1", "class-2", "class-3", "class-7", "class-8"} {
		catalog.addStatic(bodyClass, defaultBodyDeclarations(bodyClass))
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
func simplifyStylesFull(book *decodedBook, catalog *styleCatalog) {
	if book == nil {
		return
	}
	for i := range book.RenderedSections {
		if book.RenderedSections[i].Root == nil {
			continue
		}
		bodyStyle := parseDeclarationString(book.RenderedSections[i].BodyStyle)
		bodyInherited := cloneStyleMap(heritableDefaultProperties)
		for prop, val := range bodyStyle {
			if heritableProperties[prop] {
				bodyInherited[prop] = val
			}
		}
		simplifyStylesElementFull(book.RenderedSections[i].Root, catalog, bodyInherited)
		for prop, val := range bodyStyle {
			if prop == "-kfx-style-name" || prop == "-kfx-layout-hints" {
				continue
			}
			if heritableDefaultProperties[prop] == val {
				delete(bodyStyle, prop)
			}
		}
		book.RenderedSections[i].BodyStyle = styleStringFromMap(bodyStyle)
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
var heritableProperties = map[string]bool{
	"font-family": true, "font-size": true, "font-style": true, "font-weight": true,
	"font-variant": true, "line-height": true, "color": true,
	"text-align": true, "text-indent": true, "text-transform": true,
	"-webkit-hyphens": true, "-webkit-line-break": true, "-webkit-text-combine": true,
	"writing-mode": true,
}

// Comparison defaults for simplify_styles. These are intentionally not the same as the emitted
// body defaults set in setHTMLDefaults; Python compares body styles against generic document
// defaults, then keeps only residual body differences such as the chosen font family.
var heritableDefaultProperties = map[string]string{
	"font-family":    "serif",
	"font-size":      "1em",
	"font-style":     "normal",
	"font-weight":    "normal",
	"font-variant":   "normal",
	"line-height":    "normal",
	"text-indent":    "0",
	"text-transform": "none",
	"writing-mode":   "horizontal-tb",
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
	return heritableProperties[name] && name != "font-size" && name != "line-height"
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

	explicitStyle := parseDeclarationString(elem.Attrs["style"])
	sty := cloneStyleMap(inherited)
	for name, val := range explicitStyle {
		sty[name] = val
	}

	parentStyle := map[string]string{}
	for name, val := range sty {
		if heritableProperties[name] {
			parentStyle[name] = val
		}
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
	if tagChangedToParagraph || tagChangedToFigure || elem.Tag == "p" {
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
		delete(comparisonInherited, "font-size")
		delete(comparisonInherited, "font-weight")
	}
	for name, val := range explicitStyle {
		if name == "-kfx-style-name" || name == "-kfx-layout-hints" {
			continue
		}
		if comparisonInherited[name] == val {
			delete(explicitStyle, name)
		}
	}

	// Collapse composite side: when all 4 individual sides of margin/padding/border-width/etc
	// are equal, collapse them into the shorthand property.
	// Ported from Python add_composite_and_equivalent_styles COMPOSITE_SIDE_STYLES loop.
	for _, entry := range compositeSideStyles {
		combinedProp := entry[0]
		individualProps := strings.Fields(entry[1])
		val, ok := explicitStyle[individualProps[0]]
		if !ok {
			continue
		}
		allEqual := true
		for _, individualProp := range individualProps[1:] {
			if explicitStyle[individualProp] != val {
				allEqual = false
				break
			}
		}
		if allEqual {
			explicitStyle[combinedProp] = val
			for _, individualProp := range individualProps {
				delete(explicitStyle, individualProp)
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

	setElementStyleString(elem, styleStringFromMap(explicitStyle))

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
