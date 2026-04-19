package kfx

import (
	"log"
)

// ---------------------------------------------------------------------------
// Port of Python yj_metadata.py: metadata getter/query functions.
// These are methods of BookMetadata in Python; here they operate on fragmentCatalog.
//
// Python reference: REFERENCE/Calibre_KFX_Input/kfxlib/yj_metadata.py
// Go file: internal/kfx/yj_metadata_getters.go
// ---------------------------------------------------------------------------

// metadataSymbols mirrors Python METADATA_SYMBOLS (yj_structure.py):
// metadata key name → YJ Ion symbol.
var metadataSymbols = map[string]string{
	"ASIN":              "$224",
	"asset_id":          "$466",
	"author":            "$222",
	"cde_content_type":  "$251",
	"cover_image":       "$424",
	"description":       "$154",
	"language":          "$10",
	"orientation":       "$215",
	"publisher":         "$232",
	"reading_orders":    "$169",
	"support_landscape": "$218",
	"support_portrait":  "$217",
	"title":             "$153",
}

// symbolFormats maps format symbol → format string.
// Port of Python SYMBOL_FORMATS (resources.py).
var symbolFormats = map[string]string{
	"$599": "bmp",
	"$286": "gif",
	"$285": "jpg",
	"$548": "jxr",
	"$420": "pbm",
	"$565": "pdf",
	"$284": "png",
	"$287": "pobject",
	"$600": "tiff",
	"$612": "bpg",
}

// coverImageData represents the result of getCoverImageData:
// (format_string, bytes_data) from Python.
type coverImageData struct {
	Format string
	Data   []byte
}

// generatorInfo represents a (name, version) tuple from getGenerators.
type generatorInfo struct {
	Name    string
	Version string
}

// cacheInfo holds cached boolean property values, mirroring Python's
// cached @property pattern (hasattr checks).
type cacheInfo struct {
	cdeType             *string
	isFixedLayout       *bool
	isPrintReplica      *bool
	isImageBasedFXL     *bool
	isPDFBackedFXL      *bool
	isIllustratedLayout *bool
	isKfxV1             *bool
	hasPDFResource      *bool
}

func newCacheInfo() *cacheInfo {
	return &cacheInfo{}
}

// ---------------------------------------------------------------------------
// getMetadataValue — Python yj_metadata.py:362-387
//
// Two-tier lookup:
//   1. Search $490 fragment (TitleMetadata) for matching category ($495) and key name ($492).
//   2. Fallback: search $258 fragment (ReadingOrderMetadata) using metadataSymbols mapping.
//
// Returns the default value if not found.
// ---------------------------------------------------------------------------
func getMetadataValue(cat *fragmentCatalog, name, category string, defaultVal interface{}) interface{} {
	// Tier 1: search $490 (TitleMetadata)
	if cat.TitleMetadata != nil {
		containers, ok := asSlice(cat.TitleMetadata["$491"])
		if ok {
			for _, rawCM := range containers {
				cm, ok := asMap(rawCM)
				if !ok {
					continue
				}
				if asStringDefault(cm["$495"]) != category {
					continue
				}
				entries, ok := asSlice(cm["$258"])
				if !ok {
					continue
				}
				for _, rawKV := range entries {
					kv, ok := asMap(rawKV)
					if !ok {
						continue
					}
					if asStringDefault(kv["$492"]) == name {
						return kv["$307"]
					}
				}
			}
		}
	}

	// Tier 2: search $258 (ReadingOrderMetadata) using symbol mapping
	metadataSymbol, mapped := metadataSymbols[name]
	if mapped {
		if cat.ReadingOrderMetadata != nil {
			if val, exists := cat.ReadingOrderMetadata[metadataSymbol]; exists {
				return val
			}
		}
	}

	return defaultVal
}

// ---------------------------------------------------------------------------
// getFeatureValue — Python yj_metadata.py:389-417
//
// Two namespace paths:
//   1. "format_capabilities": search $593 fragments for matching feature name.
//   2. "com.amazon.yjconversion" (default): search $585 fragment's $590 list.
// ---------------------------------------------------------------------------
func getFeatureValue(cat *fragmentCatalog, feature, namespace string, defaultVal interface{}) interface{} {
	if namespace == "format_capabilities" {
		// Search $593 fragments (stored in FormatCapabilities map)
		for _, fragment := range cat.FormatCapabilities {
			items, ok := asSlice(fragment["items"])
			if !ok {
				// Try the value directly as a list (some formats store differently)
				continue
			}
			for _, rawFC := range items {
				fc, ok := asMap(rawFC)
				if !ok {
					continue
				}
				if asStringDefault(fc["$492"]) == feature {
					return fc["version"]
				}
			}
		}
	} else {
		// Search $585 fragment's $590 list (ContentFeatures)
		features, ok := asSlice(cat.ContentFeatures["$590"])
		if !ok {
			return defaultVal
		}
		for _, rawCF := range features {
			cf, ok := asMap(rawCF)
			if !ok {
				continue
			}
			if asStringDefault(cf["$586"]) != namespace {
				continue
			}
			if asStringDefault(cf["$492"]) != feature {
				continue
			}
			versionInfo, _ := asMap(cf["$589"])
			if versionInfo == nil {
				return defaultVal
			}
			versionMap, _ := asMap(versionInfo["version"])
			if versionMap == nil {
				return defaultVal
			}
			majorVersion := asIntDefault(versionMap["$587"], 0)
			minorVersion := asIntDefault(versionMap["$588"], 0)
			if minorVersion == 0 {
				return majorVersion
			}
			return [2]int{majorVersion, minorVersion}
		}
	}

	return defaultVal
}

// ---------------------------------------------------------------------------
// cdeType — Python yj_metadata.py:262-267 (cached property)
//
// Returns the raw cde_content_type string (e.g. "MAGZ", "EBSP", "EBOK").
// ---------------------------------------------------------------------------
func cdeType(cat *fragmentCatalog, ci *cacheInfo) string {
	if ci.cdeType != nil {
		return *ci.cdeType
	}
	val := getMetadataValue(cat, "cde_content_type", "kindle_title_metadata", nil)
	if val == nil {
		empty := ""
		ci.cdeType = &empty
		return ""
	}
	s, ok := asString(val)
	result := ""
	if ok {
		result = s
	}
	ci.cdeType = &result
	return result
}

// ---------------------------------------------------------------------------
// isMagazine — Python yj_metadata.py:269-271
// ---------------------------------------------------------------------------
func isMagazine(cat *fragmentCatalog, ci *cacheInfo) bool {
	return cdeType(cat, ci) == "MAGZ"
}

// ---------------------------------------------------------------------------
// isSample — Python yj_metadata.py:273-275
// ---------------------------------------------------------------------------
func isSample(cat *fragmentCatalog, ci *cacheInfo) bool {
	return cdeType(cat, ci) == "EBSP"
}

// ---------------------------------------------------------------------------
// isFixedLayout — Python yj_metadata.py:277-284 (cached property)
//
// True when scribe notebook OR yj_fixed_layout capability exists.
// ---------------------------------------------------------------------------
func isFixedLayout(cat *fragmentCatalog, isScribeNotebook bool, ci *cacheInfo) bool {
	if ci.isFixedLayout != nil {
		return *ci.isFixedLayout
	}
	result := isScribeNotebook ||
		getMetadataValue(cat, "yj_fixed_layout", "kindle_capability_metadata", nil) != nil
	ci.isFixedLayout = &result
	return result
}

// ---------------------------------------------------------------------------
// isPrintReplica — Python yj_metadata.py:298-305 (cached property)
//
// True if yj_fixed_layout==2 OR (yj_textbook exists AND yj_fixed_layout != 3).
// ---------------------------------------------------------------------------
func isPrintReplica(cat *fragmentCatalog, ci *cacheInfo) bool {
	if ci.isPrintReplica != nil {
		return *ci.isPrintReplica
	}
	yjTextbook := getMetadataValue(cat, "yj_textbook", "kindle_capability_metadata", nil)
	yjFixedLayout := getMetadataValue(cat, "yj_fixed_layout", "kindle_capability_metadata", nil)

	result := false
	if yjFixedLayout != nil {
		if fixedLayoutVal, ok := asInt(yjFixedLayout); ok && fixedLayoutVal == 2 {
			result = true
		}
	}
	if !result && yjTextbook != nil {
		if yjFixedLayout == nil {
			// yj_fixed_layout is nil, which is != 3
			result = true
		} else if fixedLayoutVal, ok := asInt(yjFixedLayout); ok && fixedLayoutVal != 3 {
			result = true
		}
	}

	ci.isPrintReplica = &result
	return result
}

// ---------------------------------------------------------------------------
// isPDFBackedFixedLayout — Python yj_metadata.py:307-312 (cached property)
// ---------------------------------------------------------------------------
func isPDFBackedFixedLayout(cat *fragmentCatalog, ci *cacheInfo) bool {
	if ci.isPDFBackedFXL != nil {
		return *ci.isPDFBackedFXL
	}
	yjFixedLayout := getMetadataValue(cat, "yj_fixed_layout", "kindle_capability_metadata", nil)
	result := false
	if yjFixedLayout != nil {
		if val, ok := asInt(yjFixedLayout); ok && val == 3 {
			result = true
		}
	}
	ci.isPDFBackedFXL = &result
	return result
}

// ---------------------------------------------------------------------------
// isIllustratedLayout — Python yj_metadata.py:314-319 (cached property)
// ---------------------------------------------------------------------------
func isIllustratedLayout(cat *fragmentCatalog, ci *cacheInfo) bool {
	if ci.isIllustratedLayout != nil {
		return *ci.isIllustratedLayout
	}
	result := getFeatureValue(cat, "yj.illustrated_layout", "com.amazon.yjconversion", nil) != nil
	ci.isIllustratedLayout = &result
	return result
}

// ---------------------------------------------------------------------------
// isImageBasedFixedLayout — Python yj_metadata.py:286-296 (cached property)
//
// Tests if getOrderedImageResources() would succeed.
// Simplified: returns true if there are resource fragments with locations.
// ---------------------------------------------------------------------------
func isImageBasedFixedLayout(cat *fragmentCatalog, ci *cacheInfo) bool {
	if ci.isImageBasedFXL != nil {
		return *ci.isImageBasedFXL
	}
	result := len(cat.ResourceFragments) > 0
	ci.isImageBasedFXL = &result
	return result
}

// ---------------------------------------------------------------------------
// isKfxV1 — Python yj_metadata.py:338-347 (cached property)
//
// Checks $270 fragment's version field == 1.
// $270 fragments are stored in Generators map (keyed by fragment ID).
// ---------------------------------------------------------------------------
func isKfxV1(cat *fragmentCatalog, ci *cacheInfo) bool {
	if ci.isKfxV1 != nil {
		return *ci.isKfxV1
	}
	result := false
	for _, gen := range cat.Generators {
		if versionVal, ok := asInt(gen["version"]); ok && versionVal == 1 {
			result = true
			break
		}
	}
	ci.isKfxV1 = &result
	return result
}

// ---------------------------------------------------------------------------
// hasPDFResource — Python yj_metadata.py:349-360 (cached property)
//
// Scans all $164 fragments for $565 (PDF) format.
// ---------------------------------------------------------------------------
func hasPDFResource(cat *fragmentCatalog, ci *cacheInfo) bool {
	if ci.hasPDFResource != nil {
		return *ci.hasPDFResource
	}
	result := false
	for _, rawData := range cat.ResourceRawData {
		if formatSym, ok := asString(rawData["$161"]); ok && formatSym == "$565" {
			result = true
			break
		}
	}
	ci.hasPDFResource = &result
	return result
}

// ---------------------------------------------------------------------------
// getCoverImageData — Python yj_metadata.py:536-554
//
// Follows chain: metadata_value("cover_image") → $164 fragment → $161 format + $165 location → $417 raw bytes.
// Returns nil if any step fails.
// ---------------------------------------------------------------------------
func getCoverImageData(cat *fragmentCatalog) *coverImageData {
	coverImageResource := getMetadataValue(cat, "cover_image", "kindle_title_metadata", nil)
	if coverImageResource == nil {
		return nil
	}
	coverImageName, ok := asString(coverImageResource)
	if !ok || coverImageName == "" {
		return nil
	}

	// Look up $164 resource fragment raw data
	coverResourceRaw, exists := cat.ResourceRawData[coverImageName]
	if !exists {
		return nil
	}

	// Get format
	coverFmt, ok := asString(coverResourceRaw["$161"])
	if !ok {
		return nil
	}
	// If the format is a symbol, resolve to string
	if resolved, mapped := symbolFormats[coverFmt]; mapped {
		coverFmt = resolved
	}

	// Get location
	coverLocation, ok := asString(coverResourceRaw["$165"])
	if !ok || coverLocation == "" {
		return nil
	}

	// Get raw media from $417
	coverRawMedia, exists := cat.RawFragments[coverLocation]
	if !exists {
		return nil
	}

	// Normalize format: "jpg" → "jpeg" (matches Python behavior)
	formatStr := coverFmt
	if formatStr == "jpg" {
		formatStr = "jpeg"
	}

	return &coverImageData{
		Format: formatStr,
		Data:   coverRawMedia,
	}
}

// ---------------------------------------------------------------------------
// fixCoverImageData — Python yj_metadata.py:556-578
//
// Ensures JPEG cover images start with JFIF marker.
// In Go, we do minimal validation (no PIL Image re-encoding available).
// ---------------------------------------------------------------------------
func fixCoverImageData(coverData *coverImageData) *coverImageData {
	if coverData == nil {
		return nil
	}

	fmt := coverData.Format
	data := coverData.Data

	// For JPEG images, check if they have the JFIF marker
	if (fmt == "jpg" || fmt == "jpeg") && len(data) >= 4 {
		if !isJFIFJPEG(data) {
			log.Printf("kfx: warning: cover image is JPEG but not JFIF format")
			// Python re-encodes with PIL; in Go we accept the data as-is
			// but could add image re-encoding if needed
		}
	}

	return coverData
}

func isJFIFJPEG(data []byte) bool {
	return len(data) >= 4 &&
		data[0] == 0xFF && data[1] == 0xD8 &&
		data[2] == 0xFF && data[3] == 0xE0
}

// ---------------------------------------------------------------------------
// getAssetID — Python yj_metadata.py:258-259
// ---------------------------------------------------------------------------
func getAssetID(cat *fragmentCatalog) interface{} {
	return getMetadataValue(cat, "asset_id", "kindle_title_metadata", nil)
}

// ---------------------------------------------------------------------------
// hasMetadata — Python yj_metadata.py:251-253
// ---------------------------------------------------------------------------
func hasMetadata(cat *fragmentCatalog) bool {
	return cat.TitleMetadata != nil || cat.ReadingOrderMetadata != nil
}

// ---------------------------------------------------------------------------
// hasCoverData — Python yj_metadata.py:255-256
// ---------------------------------------------------------------------------
func hasCoverData(cat *fragmentCatalog) bool {
	return getCoverImageData(cat) != nil
}

// ---------------------------------------------------------------------------
// getGenerators — Python yj_metadata.py (get_generators)
//
// Returns all (generator_name, package_version) pairs from $270 fragments.
// ---------------------------------------------------------------------------
func getGenerators(cat *fragmentCatalog) []generatorInfo {
	var result []generatorInfo
	for _, gen := range cat.Generators {
		name, _ := asString(gen["$587"])
		pkgVersion, _ := asString(gen["$588"])
		result = append(result, generatorInfo{
			Name:    name,
			Version: pkgVersion,
		})
	}
	return result
}

// ---------------------------------------------------------------------------
// getPageCount — Python yj_metadata.py (get_page_count)
//
// Searches $389 (book_navigation) for a $237 (page-list) nav container
// and returns its child count.
// ---------------------------------------------------------------------------
func getPageCount(cat *fragmentCatalog) int {
	for _, navRoot := range cat.NavRoots {
		navContainers, ok := asSlice(navRoot["$392"])
		if !ok {
			continue
		}
		for _, rawNavContainer := range navContainers {
			navContainer, ok := asMap(rawNavContainer)
			if !ok {
				continue
			}
			navType, _ := asString(navContainer["$235"])
			if navType == "$237" {
				children, ok := asSlice(navContainer["$247"])
				if ok {
					return len(children)
				}
			}
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// updateCoverSectionAndStoryline — Python yj_metadata.py:793-820
//
// Updates cover section and storyline dimensions when cover image size changes.
// Processes $56/$57 (width/height) and $66/$67 (fixed_width/fixed_height) properties.
// ---------------------------------------------------------------------------
func updateCoverSectionAndStoryline(cat *fragmentCatalog, origWidth, origHeight, width, height int) {
	// Get first section from section order
	if len(cat.SectionOrder) == 0 {
		return
	}
	firstSectionID := cat.SectionOrder[0]
	section, exists := cat.SectionFragments[firstSectionID]
	if !exists {
		return
	}

	// Process the section's page template values
	processContentDimensions(section.PageTemplateValues, origWidth, origHeight, width, height)

	// Process the storyline
	if section.Storyline != "" {
		if storyline, ok := cat.Storylines[section.Storyline]; ok {
			processStorylineDimensions(storyline, origWidth, origHeight, width, height)
		}
	}
}

func processContentDimensions(content map[string]interface{}, origWidth, origHeight, width, height int) {
	if content == nil {
		return
	}

	type dimUpdate struct {
		prop     string
		origVal  int
		newVal   int
	}
	updates := []dimUpdate{
		{"$56", origWidth, width},
		{"$57", origHeight, height},
		{"$66", origWidth, width},
		{"$67", origHeight, height},
	}

	for _, u := range updates {
		if val, exists := content[u.prop]; exists {
			// Only update if it's a simple integer, not a struct
			if _, ok := asMap(val); !ok {
				currentVal, ok := asInt(val)
				if ok && currentVal != u.origVal {
					log.Printf("kfx: warning: Unexpected cover section %s %d (expected %d)", u.prop, currentVal, u.origVal)
				}
				content[u.prop] = u.newVal
			}
		}
	}

	// Process sub-children ($146)
	if children, ok := asSlice(content["$146"]); ok {
		for _, rawChild := range children {
			child, ok := asMap(rawChild)
			if ok {
				processContentDimensions(child, origWidth, origHeight, width, height)
			}
		}
	}
}

func processStorylineDimensions(storyline map[string]interface{}, origWidth, origHeight, width, height int) {
	if children, ok := asSlice(storyline["$146"]); ok {
		for _, rawChild := range children {
			child, ok := asMap(rawChild)
			if ok {
				processContentDimensions(child, origWidth, origHeight, width, height)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// reportFeaturesAndMetadata — Python yj_metadata.py:434-534
//
// Debug reporting function. Logs features and metadata found in the book.
// Simplified from Python which does extensive known-feature checking.
// ---------------------------------------------------------------------------
func reportFeaturesAndMetadata(cat *fragmentCatalog) {
	// Report features from $585
	features, ok := asSlice(cat.ContentFeatures["$590"])
	if ok && len(features) > 0 {
		log.Printf("kfx: info: %d content features found", len(features))
	}

	// Report generators from $270
	if len(cat.Generators) > 0 {
		log.Printf("kfx: info: %d generators found", len(cat.Generators))
	}

	// Report page count
	if pageCount := getPageCount(cat); pageCount > 0 {
		log.Printf("kfx: info: %d pages", pageCount)
	}
}
