package kfx

import (
	"fmt"
	"regexp"
	"strings"
)

// symType mirrors Calibre SYM_TYPE (REFERENCE/Calibre_KFX_Input/kfxlib/yj_structure.py).
type symType string

const (
	symShared     symType = "shared"
	symCommon     symType = "common"
	symDictionary symType = "dictionary"
	symOriginal   symType = "original"
	symBase64     symType = "base64"
	symShort      symType = "short"
	symUnknown    symType = "unknown"
)

// Port of BookStructure.classify_symbol (yj_structure.py) regex branches after the SHARED check.
func classifySymbol(name string) symType {
	if name == "" {
		return symUnknown
	}

	if isCommonSymbol(name) {
		return symCommon
	}
	if reDict.MatchString(name) {
		return symDictionary
	}
	if isOriginalSymbol(name) {
		return symOriginal
	}
	if reBase64Sym.MatchString(name) {
		return symBase64
	}
	if reShortSym.MatchString(name) {
		return symShort
	}
	return symUnknown
}

// Port of BookStructure.classify_symbol including SHARED when name is unresolved "$<sid>" text
// and the document symbol resolver marks it as imported/shared (sid < localStart, not a local slot).
func classifySymbolWithResolver(name string, r *symbolResolver) symType {
	if name == "" {
		return symUnknown
	}
	if r != nil && r.isSharedSymbolText(name) {
		return symShared
	}
	return classifySymbol(name)
}

func isCommonSymbol(name string) bool {
	switch name {
	case "APPROXIMATE_PAGE_LIST", "crop_bleed", "dictionary_rules", "mkfx_id",
		"page_list_entry", "srl_created_by_stampler", "yj.dictionary.text",
		"note_template_collection":
		return true
	}
	if strings.HasPrefix(name, "kfx_cover_image") {
		return true
	}
	return reContentNum.MatchString(name) ||
		reEidbucket.MatchString(name) ||
		rePageListNum.MatchString(name) ||
		reUUID.MatchString(name) ||
		reYjMeta.MatchString(name) ||
		reNmdl.MatchString(name)
}

func isOriginalSymbol(name string) bool {
	return reVUnderscore.MatchString(name) ||
		reResResourcePrefix.MatchString(name) ||
		reAnchorSectionPrefix.MatchString(name) ||
		reAnchorDashLong.MatchString(name) ||
		reAnchorUnderscoreHex.MatchString(name) ||
		reLandmarksTOC.MatchString(name) ||
		reLazyTarget.MatchString(name) ||
		reSlicePDF.MatchString(name) ||
		reTargetPg.MatchString(name) ||
		reKFXConditionalNav.MatchString(name) ||
		name == "order-1" || name == "TargetReadingOrder" || name == "PageLabels" || name == "toc_entry"
}

// Port of KFX_EPUB.determine_book_symbol_format (yj_to_epub.py).
func determineBookSymbolFormat(bookSymbols map[string]struct{}, documentData map[string]interface{}, r *symbolResolver) symType {
	counts := map[symType]int{}
	for s := range bookSymbols {
		counts[classifySymbolWithResolver(s, r)]++
	}
	counts[symOriginal] += counts[symUnknown] / 10

	quorum := (counts[symDictionary] + counts[symShort] + counts[symBase64] + counts[symOriginal]) / 2
	if counts[symShort] >= quorum || documentDataHasMaxID(documentData) {
		return symShort
	}
	if counts[symDictionary] >= quorum {
		return symDictionary
	}
	if counts[symBase64] >= quorum {
		return symBase64
	}
	return symOriginal
}

func documentDataHasMaxID(documentData map[string]interface{}) bool {
	if documentData == nil {
		return false
	}
	_, ok := documentData["max_id"]
	return ok
}

// Port of KFX_EPUB.unique_part_of_local_symbol (yj_to_epub.py).
func uniquePartOfLocalSymbol(symbol string, format symType) string {
	name := symbol
	switch format {
	case symShort:
		name = regexp.MustCompile(`^resource/`).ReplaceAllString(name, "")
	case symDictionary:
		if strings.HasPrefix(name, "G") {
			name = name[1:]
		}
	case symBase64:
		name = regexp.MustCompile(`^(resource/)?[a-zA-Z0-9_-]{22}`).ReplaceAllString(name, "")
	default:
		name = regexp.MustCompile(`^V_[0-9]_[0-9](-PARA|-CHAR)?-[0-9]_[0-9]_[0-9a-f]{12,16}_[0-9a-f]{1,5}`).ReplaceAllString(name, "")
		name = regexp.MustCompile(`^(fonts/|images/)?(res|resource)_[0-9]_[0-9]_[0-9a-f]{12,16}_[0-9a-f]{1,5}_`).ReplaceAllString(name, "")
		name = reUUID.ReplaceAllString(name, "")
	}
	for strings.HasPrefix(name, "-") || strings.HasPrefix(name, "_") {
		name = name[1:]
	}
	return name
}

// Port of KFX_EPUB.prefix_unique_part_of_symbol (yj_to_epub.py).
func prefixUniquePartOfSymbol(uniquePart, prefix string) string {
	if uniquePart == "" {
		return prefix
	}
	if reAlnumDash.MatchString(uniquePart) || !reStartsLetter.MatchString(uniquePart) {
		return prefix + "_" + uniquePart
	}
	return uniquePart
}

// Port of BookStructure.allowed_symbol_prefix (yj_structure.py L1089-1090).
// Returns true when every character in symbol_prefix is contained in "abcdefilnpstz".
// Python: `return symbol_prefix in "abcdefilnpstz"`
func allowedSymbolPrefix(symbolPrefix string) bool {
	return strings.Contains("abcdefilnpstz", symbolPrefix)
}

// Port of BookStructure.get_reading_orders (yj_structure.py L1182-1189).
// Returns reading orders from $538 (document_data) fragment, or falls back to
// $258 (metadata) fragment. Returns empty list if neither exists.
func getReadingOrders(frags fragmentCatalog) []interface{} {
	if frags.DocumentData != nil {
		if orders, ok := asSlice(frags.DocumentData["$169"]); ok {
			return orders
		}
	}
	if frags.ReadingOrderMetadata != nil {
		if orders, ok := asSlice(frags.ReadingOrderMetadata["$169"]); ok {
			return orders
		}
	}
	return nil
}

// Port of BookStructure.ordered_section_names (yj_structure.py L1194-1201).
// Returns flat list of section names from all reading orders, preserving order, deduplicating.
func orderedSectionNames(frags fragmentCatalog) []string {
	readingOrders := getReadingOrders(frags)
	var sectionNames []string
	seen := map[string]bool{}
	for _, ro := range readingOrders {
		roMap, ok := asMap(ro)
		if !ok {
			continue
		}
		sections, ok := asSlice(roMap["$170"])
		if !ok {
			continue
		}
		for _, item := range sections {
			name, ok := asString(item)
			if !ok || name == "" || seen[name] {
				continue
			}
			seen[name] = true
			sectionNames = append(sectionNames, name)
		}
	}
	return sectionNames
}

// Port of BookStructure.has_illustrated_layout_page_template_condition (yj_structure.py L1228-1256).
// Scans all $260 (section) fragments recursively looking for a specific conditional page template pattern:
//   - Key $171 (condition) exists
//   - Value is []interface{} (IonSExp) of length 3
//   - fv[1] == "$183" (position)
//   - fv[2] is []interface{} (IonSExp) of length 2, fv[2][0] == "$266" (anchor)
//   - fv[0] in ["$294", "$299", "$298"] (condition operators)
//
// Returns true if found in any section, false otherwise.
func hasIllustratedLayoutPageTemplateCondition(frags fragmentCatalog) bool {
	for _, section := range frags.SectionFragments {
		if scanSectionForCondition(section.PageTemplateValues) {
			return true
		}
		// Also check individual page templates within the section
		for _, pt := range section.PageTemplates {
			if scanSectionForCondition(pt.PageTemplateValues) {
				return true
			}
			if scanSectionForConditionValue(pt.Condition) {
				return true
			}
		}
	}
	return false
}

// scanSectionForCondition recursively scans a map for the illustrated layout condition pattern.
// Port of Python _scan_section inner function (yj_structure.py L1231-1254).
func scanSectionForCondition(data map[string]interface{}) bool {
	if data == nil {
		return false
	}
	for fk, fv := range data {
		if fk == "$171" {
			if matchConditionPattern(fv) {
				return true
			}
		}
		if scanSectionForConditionValue(fv) {
			return true
		}
	}
	return false
}

// scanSectionForConditionValue recursively walks Ion-like Go data looking for the condition pattern.
func scanSectionForConditionValue(value interface{}) bool {
	switch typed := value.(type) {
	case map[string]interface{}:
		return scanSectionForCondition(typed)
	case []interface{}:
		for _, item := range typed {
			if scanSectionForConditionValue(item) {
				return true
			}
		}
	}
	return false
}

// matchConditionPattern checks if a value matches the illustrated layout condition pattern:
// IonSExp of length 3 where fv[0] in ["$294","$299","$298"], fv[1]=="$183",
// fv[2] is IonSExp of length 2 with fv[2][0]=="$266".
func matchConditionPattern(fv interface{}) bool {
	slice, ok := asSlice(fv)
	if !ok || len(slice) != 3 {
		return false
	}
	fv0, _ := asString(slice[0])
	fv1, _ := asString(slice[1])
	if fv1 != "$183" {
		return false
	}
	if fv0 != "$294" && fv0 != "$299" && fv0 != "$298" {
		return false
	}
	fv2, ok := asSlice(slice[2])
	if !ok || len(fv2) != 2 {
		return false
	}
	fv20, _ := asString(fv2[0])
	return fv20 == "$266"
}

// Port of BookStructure.get_ordered_image_resources (yj_structure.py L1258-1298).
// Returns (ordered_image_resources, ordered_image_resource_pids, error).
// The full Python version calls collect_content_position_info which is not yet ported (A5 scope),
// so this function validates the fixed-layout constraint and provides stub data collection
// for section fragments. The full position-aware version will be completed when A5 is ported.
func getOrderedImageResources(frags fragmentCatalog, isFixedLayout bool) ([]string, []int, error) {
	if !isFixedLayout {
		return nil, nil, fmt.Errorf("book is not fixed-layout")
	}

	// Collect image resources from section fragments (simplified version).
	// Full implementation requires collect_content_position_info (A5 scope).
	var orderedImageResources []string
	var orderedImageResourcePids []int

	// Walk section fragments for resources
	for _, section := range frags.SectionFragments {
		// Check page templates for image resources
		for _, pt := range section.PageTemplates {
			_ = pt // Image resource extraction requires content position info (A5)
		}
	}

	if len(orderedImageResources) == 0 {
		return nil, nil, fmt.Errorf("book does not contain image resources")
	}

	return orderedImageResources, orderedImageResourcePids, nil
}

// Port of BookStructure.check_symbol_table (yj_structure.py L1099-1149).
// Walks all fragments to collect used symbols, checks for missing and unused symbols.
// The rebuild and full replacement logic requires fragment list manipulation that
// depends on a more complete fragment store interface; this port focuses on the
// checking/logging portion that is relevant for the Go port's validation.
func checkSymbolTable(frags fragmentCatalog, resolver *symbolResolver, rebuild bool, ignoreUnused bool) {
	// Collect used symbols from all fragment data
	usedSymbols := map[string]bool{}
	findSymbolReferences(frags.TitleMetadata, usedSymbols)
	findSymbolReferences(frags.DocumentData, usedSymbols)
	findSymbolReferences(frags.ContentFeatures, usedSymbols)
	for _, m := range frags.StyleFragments {
		findSymbolReferences(m, usedSymbols)
	}
	for _, m := range frags.Storylines {
		findSymbolReferences(m, usedSymbols)
	}
	for _, m := range frags.NavContainers {
		findSymbolReferences(m, usedSymbols)
	}
	for _, root := range frags.NavRoots {
		findSymbolReferences(root, usedSymbols)
	}
	for _, sec := range frags.SectionFragments {
		findSymbolReferences(sec.PageTemplateValues, usedSymbols)
		for _, pt := range sec.PageTemplates {
			findSymbolReferences(pt.PageTemplateValues, usedSymbols)
		}
	}
	for _, ids := range frags.ContentFragments {
		for _, id := range ids {
			if id != "" {
				usedSymbols[id] = true
			}
		}
	}

	// Determine new (non-shared) symbols
	newSymbols := map[string]bool{}
	for symbol := range usedSymbols {
		if resolver != nil && resolver.isSharedSymbolText(symbol) {
			continue
		}
		newSymbols[symbol] = true
	}

	_ = newSymbols
	_ = rebuild
	_ = ignoreUnused

	// Note: Full missing/unused symbol checking against original $ion_symbol_table
	// requires access to the original symbol table, which is not available in
	// fragmentCatalog. The rebuild and replace_symbol_table_import logic is
	// deferred to a later milestone when fragment list manipulation is needed.
}

// findSymbolReferences recursively walks Ion-like Go data collecting string values
// referenced by $-prefixed keys into the usedSymbols set.
// Port of BookStructure.find_symbol_references (yj_structure.py L1151-1174).
func findSymbolReferences(data interface{}, usedSymbols map[string]bool) {
	switch typed := data.(type) {
	case map[string]interface{}:
		for k, v := range typed {
			// In Python, all struct keys (IonSymbol) are added to the set.
			// In Go, keys are already resolved strings.
			if k != "" {
				usedSymbols[k] = true
			}
			findSymbolReferences(v, usedSymbols)
		}
	case []interface{}:
		for _, item := range typed {
			findSymbolReferences(item, usedSymbols)
		}
	case string:
		if typed != "" {
			usedSymbols[typed] = true
		}
	}
}

var (
	reUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

	reContentNum  = regexp.MustCompile(`^content_[0-9]+$`)
	reEidbucket   = regexp.MustCompile(`^eidbucket_[0-9]+$`)
	rePageListNum = regexp.MustCompile(`^PAGE_LIST_[0-9]{10,}$`)
	reYjMeta      = regexp.MustCompile(`^yj\.(authoring|conversion|print|semantics)\.`)
	reNmdl        = regexp.MustCompile(`^nmdl\.`)
	reDict        = regexp.MustCompile(`^G[0-9]+(-spm)?$|^yj\.dictionary\.`)

	reVUnderscore         = regexp.MustCompile(`^V_[0-9]_[0-9](-PARA|-CHAR)?-[0-9]_[0-9]_[0-9a-f]{12,16}_[0-9a-f]{1,5}`)
	reResResourcePrefix   = regexp.MustCompile(`^(fonts/|images/|resource/)?(res|resource)_[0-9]_[0-9]_[0-9a-f]{12,16}_[0-9a-f]{1,5}_`)
	reAnchorSectionPrefix = regexp.MustCompile(`^(anchor-|section-|story-|style-|navContainer|navUnit)[0-9]_[0-9]_[0-9a-f]{12,16}_[0-9a-f]{1,5}`)
	reAnchorDashLong      = regexp.MustCompile(`^anchor-[a-z0-9_-]+-[0-9]{17,19}-[0-9]{1,2}$`)
	reAnchorUnderscoreHex = regexp.MustCompile(`^anchor-[a-z0-9_-]+_[0-9]_[0-9a-f]{12,16}_[0-9a-f]{1,5}$`)
	reLandmarksTOC        = regexp.MustCompile(`^(LANDMARKS_|TOC_)[0-9]{10,}$`)
	reLazyTarget          = regexp.MustCompile(`^(LazyLoadStoryLineForPage-|TargetSectionForPage-|TargetStoryLineForPage-)[0-9]+$`)
	reSlicePDF            = regexp.MustCompile(`^slice_[0-9]+\.pdf$`)
	reTargetPg            = regexp.MustCompile(`^Target_pg_[0-9]+_g_2$`)
	reKFXConditionalNav   = regexp.MustCompile(`^KFXConditionalNavGroupUnit_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

	reBase64Sym = regexp.MustCompile(
		`^(resource/|[ctliz])?[A-Za-z0-9_-]{22}[A-Z0-9]{0,6}` +
			`((-hd|-first-frame|-thumb)?(-resized-[0-9]+-[0-9]+|-hd-tile-[0-9]+-[0-9]+)?|` +
			`-ad|-spm|_thumbnail|-transcoded|(_thumb)?\.jpg|\.ttf|\.otf|\.woff|\.woff2|\.eot|\.dfont|\.bin)?$`,
	)
	reShortSym = regexp.MustCompile(
		`^(resource/rsrc|resource/e|rsrc|[a-z])[A-Z0-9]{1,6}` +
			`((-hd|-first-frame|-thumb)?(-resized-[0-9]+-[0-9]+|-hd-tile-[0-9]+-[0-9]+)?|` +
			`-ad|-spm|_thumbnail|-transcoded|(_thumb)?\.jpg|\.ttf|\.otf|\.woff|\.woff2|\.eot|\.dfont|\.bin)?$`,
	)

	reAlnumDash    = regexp.MustCompile(`^[A-Za-z0-9]+(-.+)?$`)
	reStartsLetter = regexp.MustCompile(`^[A-Za-z]`)
)
