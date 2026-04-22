package kfx

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var (
	cssIdentPattern = regexp.MustCompile(`^[-_a-zA-Z0-9]*$`)
)

type fontNameFixer struct {
	fixedNames       map[string]string
	nameReplacements map[string]string
}

var currentFontFixer *fontNameFixer

var cssGenericFontNames = map[string]bool{
	"serif":      true,
	"sans-serif": true,
	"cursive":    true,
	"fantasy":    true,
	"monospace":  true,
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func effectiveStyle(base map[string]interface{}, values map[string]interface{}) map[string]interface{} {
	style := cloneMap(base)
	if style == nil {
		style = map[string]interface{}{}
	}
	for key, value := range values {
		style[key] = value
	}
	return style
}

func mergeStyleValues(dst map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = map[string]interface{}{}
	}
	for key, value := range src {
		if _, exists := dst[key]; !exists {
			dst[key] = value
		}
	}
	return dst
}

func filterBodyStyleValues(values map[string]interface{}) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	allowed := map[string]bool{
		"$11":  true,
		"$12":  true,
		"$16":  true,
		"$36":  true,
		"$41":  true,
		"$42":  true,
		"$47":  true,
		"$48":  true,
		"$49":  true,
		"$50":  true,
		"$70":  true,
		"$72":  true,
		"$580": true,
		"$583": true,
	}
	filtered := map[string]interface{}{}
	for key, value := range values {
		if allowed[key] {
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func declarationSet(declarations []string) map[string]bool {
	if len(declarations) == 0 {
		return nil
	}
	result := make(map[string]bool, len(declarations))
	for _, declaration := range declarations {
		result[declaration] = true
	}
	return result
}

func inheritedDefaultSet(declarations []string) map[string]bool {
	result := declarationSet(declarations)
	if result == nil {
		result = map[string]bool{}
	}
	hasTextIndent := false
	for declaration := range result {
		if strings.HasPrefix(declaration, "text-indent: ") {
			hasTextIndent = true
			break
		}
	}
	if !hasTextIndent {
		result["text-indent: 0"] = true
	}
	return result
}

func defaultBodyDeclarations(bodyClass string) []string {
	switch bodyClass {
	case "class-0":
		return []string{"font-family: FreeFontSerif,serif", "text-align: center"}
	case "class-1":
		return []string{"font-family: FreeFontSerif,serif"}
	case "class-2":
		return []string{"font-family: FreeFontSerif,serif", "text-align: justify", "text-indent: 1.44em"}
	case "class-3":
		return []string{"font-family: FreeFontSerif,serif", "text-align: justify"}
	case "class-7":
		return []string{"font-family: FreeFontSerif,serif", "font-style: italic", "text-align: justify", "text-indent: 1.44em"}
	case "class-8":
		return []string{"font-family: Shift Light,Palatino,Palatino Linotype,Palatino LT Std,Book Antiqua,Georgia,serif"}
	default:
		return nil
	}
}

// defaultBodyDeclarationsWithFont returns the CSS declarations for a static body class,
// using the resolved default font family. When the resolved font is "serif" (the CSS default),
// font-family is omitted from the declarations since it would be stripped by simplify_styles.
// When the resolved font is something else (e.g. "FreeFontSerif,serif" for Martyr), it is included.
func defaultBodyDeclarationsWithFont(bodyClass string, resolvedFont string) []string {
	switch bodyClass {
	case "class-0":
		if resolvedFont != "" && resolvedFont != "serif" {
			return []string{"font-family: " + resolvedFont, "text-align: center"}
		}
		return []string{"text-align: center"}
	case "class-1":
		if resolvedFont != "" && resolvedFont != "serif" {
			return []string{"font-family: " + resolvedFont}
		}
		return []string{"font-family: serif"}
	case "class-2":
		if resolvedFont != "" && resolvedFont != "serif" {
			return []string{"font-family: " + resolvedFont, "text-align: justify", "text-indent: 1.44em"}
		}
		return []string{"text-align: justify", "text-indent: 1.44em"}
	case "class-3":
		if resolvedFont != "" && resolvedFont != "serif" {
			return []string{"font-family: " + resolvedFont, "text-align: justify"}
		}
		return []string{"text-align: justify"}
	case "class-7":
		if resolvedFont != "" && resolvedFont != "serif" {
			return []string{"font-family: " + resolvedFont, "font-style: italic", "text-align: justify", "text-indent: 1.44em"}
		}
		return []string{"font-style: italic", "text-align: justify", "text-indent: 1.44em"}
	case "class-8":
		return []string{"font-family: Shift Light,Palatino,Palatino Linotype,Palatino LT Std,Book Antiqua,Georgia,serif"}
	default:
		return nil
	}
}

// defaultBodyFontDeclarations returns additional font-family declarations for the body that
// should be used for inheritance filtering. Not needed since defaultBodyDeclarations already
// includes font-family.
func defaultBodyFontDeclarations(bodyClass string) []string {
	return nil
}

func isStaticBodyClass(bodyClass string) bool {
	switch bodyClass {
	case "class-0", "class-1", "class-2", "class-3", "class-7", "class-8":
		return true
	default:
		return false
	}
}

func staticBodyClassForDeclarations(declarations []string) string {
	// Alternates: declarations without font-family (for books where font-family is "serif"
	// and gets stripped by simplify_styles), plus any variant with a non-"serif" font.
	alternates := map[string][][]string{
		"class-0": {
			{"text-align: center"},
		},
		"class-2": {
			{"text-align: justify", "text-indent: 1.44em"},
		},
		"class-3": {
			{"text-align: justify"},
		},
		"class-7": {
			{"font-style: italic", "text-align: justify", "text-indent: 1.44em"},
		},
	}
	for _, bodyClass := range []string{"class-0", "class-1", "class-2", "class-3", "class-7", "class-8"} {
		expected := defaultBodyDeclarations(bodyClass)
		if len(expected) != len(declarations) {
			for _, alternate := range alternates[bodyClass] {
				if len(alternate) != len(declarations) {
					continue
				}
				match := true
				for index := range alternate {
					if alternate[index] != declarations[index] {
						match = false
						break
					}
				}
				if match {
					return bodyClass
				}
			}
			continue
		}
		match := true
		for index := range expected {
			if expected[index] != declarations[index] {
				match = false
				break
			}
		}
		if match {
			return bodyClass
		}
	}
	return ""
}

func flattenParagraphs(nodes []interface{}, contents map[string][]string) []string {
	result := make([]string, 0, 64)
	var walk func(items []interface{})
	walk = func(items []interface{}) {
		for _, item := range items {
			node, ok := asMap(item)
			if !ok {
				continue
			}
			if ref, ok := asMap(node["$145"]); ok {
				name, _ := asString(ref["name"])
				index, ok := asInt(ref["$403"])
				if ok {
					if values, found := contents[name]; found && index >= 0 && index < len(values) {
						text := strings.TrimSpace(values[index])
						if text != "" {
							result = append(result, text)
						}
					}
				}
			}
			if children, ok := asSlice(node["$146"]); ok {
				walk(children)
			}
		}
	}
	walk(nodes)
	return result
}

func deriveSectionTitle(paragraphs []string, sectionNumber int) string {
	for _, paragraph := range paragraphs {
		trimmed := strings.TrimSpace(paragraph)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > 80 {
			break
		}
		return trimmed
	}
	return fmt.Sprintf("Section %d", sectionNumber)
}

func naturalSortKey(value string) string {
	lower := strings.ToLower(value)
	var out strings.Builder
	for index := 0; index < len(lower); {
		if lower[index] < '0' || lower[index] > '9' {
			out.WriteByte(lower[index])
			index++
			continue
		}
		start := index
		for index < len(lower) && lower[index] >= '0' && lower[index] <= '9' {
			index++
		}
		digits := lower[start:index]
		if pad := 8 - len(digits); pad > 0 {
			out.WriteString(strings.Repeat("0", pad))
		}
		out.WriteString(digits)
	}
	return out.String()
}

func mapField(value interface{}, key string) (interface{}, bool) {
	if mapped, ok := value.(map[string]interface{}); ok {
		result, found := mapped[key]
		return result, found
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || rv.Kind() != reflect.Map {
		return nil, false
	}
	for _, mapKey := range rv.MapKeys() {
		if mapKeyString(mapKey.Interface()) == key {
			return rv.MapIndex(mapKey).Interface(), true
		}
	}
	return nil, false
}

func mapKeyString(value interface{}) string {
	if text, ok := asString(value); ok {
		return text
	}
	return fmt.Sprint(value)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func debugSectionMappings(sections map[string]sectionFragment, navTitles map[string]string, order []string) {
	if os.Getenv("KFX_DEBUG") == "" {
		return
	}
	for _, sectionID := range order {
		section := sections[sectionID]
		fmt.Fprintf(os.Stderr, "section id=%s pos=%d storyline=%s title=%s\n", sectionID, section.PositionID, section.Storyline, navTitles[sectionID])
	}
}

func debugStorylineNodes(sectionID string, nodes []interface{}, depth int) {
	if os.Getenv("KFX_DEBUG") == "" {
		return
	}
	debugSections := os.Getenv("KFX_DEBUG_SECTIONS")
	if debugSections == "" {
		if sectionID != "c73" && sectionID != "c109" && sectionID != "c6P" {
			return
		}
	} else if !strings.Contains(","+debugSections+",", ","+sectionID+",") {
		return
	}
	prefix := strings.Repeat("  ", depth)
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		positionID, _ := asInt(node["$155"])
		styleID, _ := asString(node["$157"])
		text := ""
		if ref, ok := asMap(node["$145"]); ok {
			text = truncateDebugText(ref)
		}
		fmt.Fprintf(os.Stderr, "story %s %spos=%d type=%s style=%s text=%q keys=%v\n", sectionID, prefix, positionID, asStringDefault(node["$159"]), styleID, text, sortedMapKeys(node))
		if cols, ok := asSlice(node["$152"]); ok {
			fmt.Fprintf(os.Stderr, "story %s %scols=%#v\n", sectionID, prefix, cols)
		}
		if children, ok := asSlice(node["$146"]); ok {
			debugStorylineNodes(sectionID, children, depth+1)
		}
	}
}

func sortedMapKeys(value map[string]interface{}) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func truncateDebugText(ref map[string]interface{}) string {
	name, _ := asString(ref["name"])
	index, _ := asInt(ref["$403"])
	return fmt.Sprintf("%s[%d]", name, index)
}

func asStringDefault(value interface{}) string {
	result, _ := asString(value)
	return result
}

func intPtr(value int) *int {
	return &value
}

// sectionFilename produces the XHTML filename for a section, matching Python's
// SECTION_TEXT_FILEPATH % section_name convention (yj_to_epub_content.py:171,191).
// Python uses the raw $174 symbol directly as the filename, e.g.
// "UYqzWVgySW_Gl4WQ-Od_xQ1.xhtml". Previously Go applied uniquePartOfLocalSymbol
// which stripped base64/UUID prefixes, producing numeric names like "1.xhtml".
// Port of Python: self.SECTION_TEXT_FILEPATH % section_name where section_name
// comes from section.pop("$174") — used verbatim, no uniquePartOfLocalSymbol.
func sectionFilename(sectionID string) string {
	return sectionID + ".xhtml"
}

func cloneStyleMap(style map[string]string) map[string]string {
	if len(style) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(style))
	for key, value := range style {
		cloned[key] = value
	}
	return cloned
}

func resolveContentText(contentFragments map[string][]string, ref map[string]interface{}) string {
	name, _ := asString(ref["name"])
	index, ok := asInt(ref["$403"])
	if !ok {
		return ""
	}
	values := contentFragments[name]
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func inferBookLanguage(defaultLanguage string, contentFragments map[string][]string, storylines map[string]map[string]interface{}, styleFragments map[string]map[string]interface{}) string {
	defaultKey := languageKey(defaultLanguage)
	if defaultKey == "" {
		return defaultLanguage
	}
	merits := map[string]int{}
	for _, storyline := range storylines {
		nodes, _ := asSlice(storyline["$146"])
		accumulateContentLanguageMerits(nodes, defaultKey, merits, contentFragments, styleFragments)
	}
	bestLanguage := defaultKey
	bestMerit := 0
	for language, merit := range merits {
		if merit <= bestMerit || !languageMatchesDefault(language, defaultKey) {
			continue
		}
		bestLanguage = language
		bestMerit = merit
	}
	if bestMerit == 0 {
		return defaultLanguage
	}
	return bestLanguage
}

func accumulateContentLanguageMerits(nodes []interface{}, currentLanguage string, merits map[string]int, contentFragments map[string][]string, styleFragments map[string]map[string]interface{}) {
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		language := currentLanguage
		styleID, _ := asString(node["$157"])
		style := effectiveStyle(styleFragments[styleID], node)
		if rawLanguage, ok := asString(style["$10"]); ok && rawLanguage != "" {
			language = languageKey(rawLanguage)
		}
		if ref, ok := asMap(node["$145"]); ok && language != "" {
			merits[language] += len([]rune(resolveContentText(contentFragments, ref)))
		}
		if children, ok := asSlice(node["$146"]); ok {
			accumulateContentLanguageMerits(children, language, merits, contentFragments, styleFragments)
		}
	}
}

func languageKey(language string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
}

func languageMatchesDefault(candidate string, defaultLanguage string) bool {
	if candidate == "" || defaultLanguage == "" {
		return false
	}
	return candidate == defaultLanguage || strings.HasPrefix(candidate, defaultLanguage+"-")
}

func bodyPromotionPresenceStyle(bodyClass string) map[string]interface{} {
	switch bodyClass {
	case "class-0":
		return map[string]interface{}{"$11": true, "$34": true}
	case "class-1":
		return map[string]interface{}{"$11": true}
	case "class-2":
		return map[string]interface{}{"$11": true, "$34": true, "$36": true}
	case "class-3":
		return map[string]interface{}{"$11": true, "$34": true}
	default:
		return nil
	}
}

func storylineUsesJustifiedBody(nodes []interface{}) bool {
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		styleID, _ := asString(node["$157"])
		if styleID == "s6E" || styleID == "s6G" {
			return true
		}
		if children, ok := asSlice(node["$146"]); ok && storylineUsesJustifiedBody(children) {
			return true
		}
	}
	return false
}

func estimateBodyClass(nodes []interface{}) string {
	if storylineUsesJustifiedBody(nodes) {
		return "class-2"
	}
	if storylineIsCentered(nodes) {
		return "class-0"
	}
	return "class-1"
}

func storylineIsCentered(nodes []interface{}) bool {
	return !storylineContainsParagraph(nodes)
}

func storylineContainsParagraph(nodes []interface{}) bool {
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		if _, ok := asMap(node["$145"]); ok && headingLevel(node) == 0 {
			return true
		}
		if children, ok := asSlice(node["$146"]); ok && storylineContainsParagraph(children) {
			return true
		}
	}
	return false
}

func appendClassNames(existing string, classNames ...string) string {
	parts := []string{}
	seen := map[string]bool{}
	for _, raw := range append([]string{existing}, classNames...) {
		for _, className := range strings.Fields(strings.TrimSpace(raw)) {
			if className == "" || seen[className] {
				continue
			}
			seen[className] = true
			parts = append(parts, className)
		}
	}
	return strings.Join(parts, " ")
}

func newFontNameFixer() *fontNameFixer {
	return &fontNameFixer{
		fixedNames:       map[string]string{},
		nameReplacements: map[string]string{},
	}
}

// setDefaultFontFamily sets up the default font name replacement map, matching Python's
// process_document_data (yj_to_epub_metadata.py L100-116):
//   self.font_name_replacements["default"] = DEFAULT_DOCUMENT_FONT_FAMILY  # "serif"
//   for default_name in DEFAULT_FONT_NAMES:
//       for font_family in self.default_font_family.split(","):
//           self.font_name_replacements[default_name] = self.strip_font_name(font_family)
// This ensures that "default" and "$amzn_fixup_default_font$" in KFX font-family lists
// resolve to the book's actual default font (e.g., "serif") instead of being kept as "default".
// registerFontFamilies should be called first so that @font-face names are available
// for proper case resolution.
// defaultFontFamily is the raw $11 value from document data, which may contain font names
// like "akba_9780593537626_epub3_cvi_r1-freefontserif" that need prefix stripping and
// case resolution through registered font names.
func (f *fontNameFixer) setDefaultFontFamily(defaultFontFamily string) {
	if defaultFontFamily == "" {
		defaultFontFamily = "serif"
	}
	// Resolve the raw default font family through fixFontName to get proper case.
	// This handles cases like "akba_9780593537626_epub3_cvi_r1-freefontserif" → "FreeFontSerif"
	// when @font-face has registered the name with proper case.
	resolvedFamily := f.splitAndFixFontFamilyList(defaultFontFamily)
	if len(resolvedFamily) > 0 {
		defaultFontFamily = strings.Join(resolvedFamily, ",")
	}
	// Python: self.font_name_replacements["default"] = DEFAULT_DOCUMENT_FONT_FAMILY
	f.nameReplacements["default"] = "serif"
	// Python: for default_name in DEFAULT_FONT_NAMES:
	//   for font_family in self.default_font_family.split(","):
	//     self.font_name_replacements[default_name] = self.strip_font_name(font_family)
	for _, defaultName := range []string{"default", "$amzn_fixup_default_font$"} {
		for _, fontFamily := range strings.Split(defaultFontFamily, ",") {
			f.nameReplacements[strings.ToLower(defaultName)] = stripFontName(fontFamily)
		}
	}
}

// registerFontFamilies registers @font-face font names with add=true, matching Python's
// process_fonts (yj_to_epub_resources.py) which calls fix_font_name(font["$11"], add=True).
// This registers each font with its proper case so that subsequent lookups (e.g., when
// resolving "default" from KFX metadata) find the properly-cased name.
// Must be called before setDefaultFontFamily to ensure proper case resolution.
func (f *fontNameFixer) registerFontFamilies(fonts map[string]fontFragment) {
	for _, font := range fonts {
		if font.Family == "" {
			continue
		}
		// Register the raw font name (may have prefix like "akba_...-freefontserif").
		// This handles prefix-stripped names with the "?-" key convention.
		resolved := f.fixFontName(font.Family, true, false)
		// Also ensure the resolved name is registered without the "?-" prefix.
		// This handles lookups of the resolved name directly (e.g., "FreeFontSerif").
		// When the raw name had a prefix, the resolved name was stored with "?-" key,
		// but subsequent lookups use the plain lowercase key.
		if resolved != "" && !cssGenericFontNames[strings.ToLower(resolved)] {
			key := strings.ToLower(resolved)
			if _, ok := f.nameReplacements[key]; !ok {
				f.nameReplacements[key] = resolved
			}
			if _, ok := f.fixedNames[key]; !ok {
				f.fixedNames[key] = resolved
			}
		}
	}
}

// resolvedDefaultFontFamily returns the resolved default font family for use in
// setHTMLDefaults. This is the properly-cased, quoted font family string that
// Python would use for self.default_font_family. For books where the document
// default is just "serif", this returns "serif". For books like Martyr where the
// document default resolves to "FreeFontSerif", this returns "FreeFontSerif,serif".
func (f *fontNameFixer) resolvedDefaultFontFamily() string {
	if replacement, ok := f.nameReplacements["default"]; ok && replacement != "serif" {
		return f.fixAndQuoteFontFamilyList(replacement + ",serif")
	}
	return "serif"
}

func (f *fontNameFixer) fixAndQuoteFontFamilyList(value string) string {
	families := f.splitAndFixFontFamilyList(value)
	if len(families) == 0 {
		return ""
	}
	seen := map[string]bool{}
	quoted := make([]string, 0, len(families))
	for _, family := range families {
		key := strings.ToLower(family)
		if seen[key] {
			continue
		}
		seen[key] = true
		quoted = append(quoted, quoteFontName(family))
	}
	return strings.Join(quoted, ",")
}

func (f *fontNameFixer) splitAndFixFontFamilyList(value string) []string {
	parts := strings.Split(value, ",")
	families := make([]string, 0, len(parts))
	for _, part := range parts {
		if family := f.fixFontName(part, false, false); family != "" {
			families = append(families, family)
		}
	}
	return families
}

func stripFontName(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > 0 && (name[0] == '\'' || name[0] == '"') {
		name = name[1:]
	}
	if len(name) > 0 && (name[len(name)-1] == '\'' || name[len(name)-1] == '"') {
		name = name[:len(name)-1]
	}
	return strings.TrimSpace(name)
}

func (f *fontNameFixer) fixFontName(name string, add bool, generic bool) string {
	name = stripFontName(name)
	if name == "" {
		return ""
	}
	origName := strings.ToLower(name)
	if fixed, ok := f.fixedNames[origName]; ok {
		return fixed
	}
	name = strings.ReplaceAll(name, `\`, "")
	lower := strings.ToLower(name)
	replacements := map[string]string{
		"san-serif": "sans-serif",
		"ariel":     "Arial",
	}
	if replacement, ok := replacements[lower]; ok {
		name = replacement
		lower = strings.ToLower(name)
	}
	for _, suffix := range []string{"-oblique", "-italic", "-bold", "-regular", "-roman", "-medium"} {
		if strings.HasSuffix(lower, suffix) {
			name = name[:len(name)-len(suffix)] + " " + strings.TrimPrefix(suffix, "-")
			break
		}
	}
	hasPrefix := strings.Contains(name, "-") && name != "sans-serif"
	if hasPrefix {
		name = strings.ReplaceAll(name, "sans-serif", "sans_serif")
		name = name[strings.LastIndex(name, "-")+1:]
		name = strings.ReplaceAll(name, "sans_serif", "sans-serif")
	}
	name = strings.TrimSpace(name)
	if add {
		key := strings.ToLower(name)
		if hasPrefix {
			key = "?-" + key
		}
		if replacement, ok := f.nameReplacements[key]; ok {
			name = replacement
		} else {
			f.nameReplacements[key] = name
		}
	} else {
		if replacement, ok := f.nameReplacements[strings.ToLower(name)]; ok {
			name = replacement
		} else if cssGenericFontNames[strings.ToLower(name)] {
			name = strings.ToLower(name)
		} else {
			name = capitalizeFontName(name)
		}
	}
	f.fixedNames[origName] = name
	return name
}

func capitalizeFontName(name string) string {
	words := strings.Fields(name)
	for i, word := range words {
		if len(word) > 2 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		} else {
			words[i] = strings.ToUpper(word)
		}
	}
	return strings.Join(words, " ")
}

func quoteFontName(value string) string {
	for _, ident := range strings.Split(value, " ") {
		if ident == "" {
			break
		}
		first := ident[0]
		if (first >= '0' && first <= '9') || (len(ident) >= 2 && ident[:2] == "--") || !cssIdentPattern.MatchString(ident) {
			return quoteCSSString(value)
		}
		if first == '-' && len(ident) > 1 && ident[1] >= '0' && ident[1] <= '9' {
			return quoteCSSString(value)
		}
	}
	return value
}

func canonicalDeclarations(declarations []string) []string {
	if len(declarations) == 0 {
		return declarations
	}
	out := make([]string, 0, len(declarations))
	seen := map[string]bool{}
	for _, declaration := range declarations {
		declaration = strings.TrimSpace(declaration)
		if declaration == "" || seen[declaration] {
			continue
		}
		seen[declaration] = true
		out = append(out, declaration)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ni := out[i]
		nj := out[j]
		pi := ni
		if idx := strings.IndexByte(ni, ':'); idx >= 0 {
			pi = ni[:idx]
		}
		pj := nj
		if idx := strings.IndexByte(nj, ':'); idx >= 0 {
			pj = nj[:idx]
		}
		if pi == pj {
			return ni < nj
		}
		return pi < pj
	})
	return out
}

func quoteCSSString(value string) string {
	if !strings.Contains(value, "'") && !strings.Contains(value, `\`) {
		return "'" + value + "'"
	}
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}
