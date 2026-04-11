package kfx

import (
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
