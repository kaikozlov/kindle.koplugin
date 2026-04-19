package kfx

import (
	"bytes"
	"fmt"
	"image/jpeg"
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
		// Python iterates fragment.value directly as a list of IonStruct.
		// In Go, $593 fragment value is decoded as a map with "$146" key containing
		// the list of capability entries, or the map itself may be a list-like value.
		for _, fragment := range cat.FormatCapabilities {
			// Try $146 (children list) first — most common encoding
			var entries []interface{}
			if children, ok := asSlice(fragment["$146"]); ok {
				entries = children
			} else if direct, ok := asSlice(fragment["items"]); ok {
				entries = direct
			} else {
				// Fragment value may itself be the list (unwrapped IonList)
				continue
			}
			for _, rawFC := range entries {
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
// Python: try: self.get_ordered_image_resources()
//
//	except Exception: cached = False
//	else: cached = True
//
// Must return true only if getOrderedImageResources succeeds (fixed-layout AND
// valid ordered images with no text).
// ---------------------------------------------------------------------------
func isImageBasedFixedLayout(cat *fragmentCatalog, ci *cacheInfo) bool {
	if ci.isImageBasedFXL != nil {
		return *ci.isImageBasedFXL
	}
	// Python calls get_ordered_image_resources which checks is_fixed_layout first.
	// We pass the result of isFixedLayout to the function.
	isFXL := isFixedLayout(cat, false, ci)
	_, _, err := getOrderedImageResources(*cat, isFXL)
	result := err == nil
	ci.isImageBasedFXL = &result
	return result
}

// ---------------------------------------------------------------------------
// isKfxV1 — Python yj_metadata.py:338-341 (cached property)
//
// Checks the FIRST $270 fragment's version field == 1 (default 0).
// Python: fragment = self.fragments.get("$270", first=True)
//         fragment.value.get("version", 0) == 1 if fragment is not None else False
// ---------------------------------------------------------------------------
func isKfxV1(cat *fragmentCatalog, ci *cacheInfo) bool {
	if ci.isKfxV1 != nil {
		return *ci.isKfxV1
	}
	result := false
	// Get the first $270 fragment ID (Python: first=True)
	if ids, ok := cat.FragmentIDsByType["$270"]; ok && len(ids) > 0 {
		firstID := ids[0]
		if gen, exists := cat.Generators[firstID]; exists {
			// Python: fragment.value.get("version", 0) == 1
			versionVal := 0
			if v, ok := asInt(gen["version"]); ok {
				versionVal = v
			}
			result = versionVal == 1
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
// Re-encodes using Go's image/jpeg package (equivalent to Python's PIL).
// If re-encoding fails or still doesn't produce JFIF, returns original data.
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
			origData := data

			// Python: cover = Image.open(io.BytesIO(data)); cover.save(outfile, "JPEG", quality="keep")
			// Go equivalent: decode and re-encode as JPEG
			reencoded, err := reencodeJPEG(data)
			if err == nil {
				data = reencoded
			} else {
				// Python: except Exception: data = orig_data
				data = origData
			}

			if isJFIFJPEG(data) {
				log.Printf("kfx: info: Changed cover image from %s to JPEG/JFIF for Kindle lockscreen display", jpegType(origData))
			} else {
				log.Printf("kfx: error: Failed to change cover image from %s to JPEG/JFIF", jpegType(origData))
				data = origData
			}
		}
	}

	return &coverImageData{
		Format: coverData.Format,
		Data:   data,
	}
}

// reencodeJPEG decodes JPEG bytes and re-encodes them with a JFIF APP0 marker.
// Go's image/jpeg.Encode doesn't produce JFIF markers, so we prepend one manually.
// This matches Python's PIL behavior where cover.save(outfile, "JPEG", quality="keep")
// produces a JFIF-compatible JPEG.
func reencodeJPEG(data []byte) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95})
	if err != nil {
		return nil, err
	}

	// Go's encoder doesn't write JFIF APP0 marker. Prepend one.
	// JFIF APP0 marker: FF E0 [length_hi length_lo] 4A 46 49 46 00 [version_major version_minor] [density units] [x_density_hi x_density_lo] [y_density_hi y_density_lo] [thumbnail_w] [thumbnail_h]
	encoded := buf.Bytes()
	if len(encoded) >= 4 && encoded[0] == 0xFF && encoded[1] == 0xD8 {
		// Insert JFIF APP0 marker after SOI (FF D8)
		jfifAPP0 := []byte{
			0xFF, 0xE0, // APP0 marker
			0x00, 0x10, // Length (16 bytes including length itself)
			0x4A, 0x46, 0x49, 0x46, 0x00, // "JFIF\0"
			0x01, 0x01, // Version 1.1
			0x00, // Density units: no units
			0x00, 0x01, // X density: 1
			0x00, 0x01, // Y density: 1
			0x00, 0x00, // No thumbnail
		}
		result := make([]byte, 0, len(encoded)+len(jfifAPP0))
		result = append(result, encoded[:2]...) // SOI
		result = append(result, jfifAPP0...)    // JFIF APP0
		result = append(result, encoded[2:]...) // Rest of JPEG
		return result, nil
	}

	return encoded, nil
}

// jpegType returns a description of the JPEG header type for logging.
// Port of Python resources.py:725-747 (jpeg_type).
func jpegType(data []byte) string {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		if len(data) >= 12 {
			return fmt.Sprintf("UNKNOWN(%x)", data[:12])
		}
		return "UNKNOWN"
	}

	if len(data) >= 10 && data[2] == 0xFF && data[3] == 0xE0 && string(data[6:10]) == "JFIF" {
		return "JPEG"
	}

	if len(data) >= 10 && data[2] == 0xFF && data[3] == 0xE1 && string(data[6:10]) == "Exif" {
		return "JPEG/Exif"
	}

	if len(data) >= 4 && data[2] == 0xFF && data[3] == 0xE8 {
		return "JPEG/SPIFF"
	}

	if len(data) >= 4 && data[2] == 0xFF && (data[3] == 0xED || data[3] == 0xEE) {
		return "JPEG/Adobe"
	}

	if len(data) >= 4 && data[2] == 0xFF && (data[3] == 0xDB || data[3] == 0xDE) {
		return "JPEG/no-app-marker"
	}

	if len(data) >= 12 {
		return fmt.Sprintf("JPEG/UNKNOWN(%x)", data[:12])
	}
	return "JPEG/UNKNOWN"
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
// getGenerators — Python yj_metadata.py:393-398 (get_generators)
//
// Returns all (generator_name, package_version) pairs from $270 fragments.
// Filters out PACKAGE_VERSION_PLACEHOLDERS, returning "" for placeholder versions.
// Only includes generators that have a "version" key in their fragment value.
// ---------------------------------------------------------------------------
func getGenerators(cat *fragmentCatalog) []generatorInfo {
	var result []generatorInfo
	for _, gen := range cat.Generators {
		// Python: if "version" in fragment.value
		if _, hasVersion := gen["version"]; !hasVersion {
			continue
		}
		name, _ := asString(gen["$587"])
		pkgVersion, _ := asString(gen["$588"])
		// Python: package_version if package_version not in PACKAGE_VERSION_PLACEHOLDERS else ""
		if PackageVersionPlaceholders[pkgVersion] {
			pkgVersion = ""
		}
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
// Also recursively processes $157 style fragments referenced via content keys.
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

	// Get page template: first from $141 list, or empty map if multiple
	pageTemplate := section.PageTemplateValues

	// Process the page template
	processContentDimensions(cat, pageTemplate, "section", origWidth, origHeight, width, height)

	// Process the storyline
	if section.Storyline != "" {
		if storyline, ok := cat.Storylines[section.Storyline]; ok {
			processStorylineDimensions(cat, storyline, "storyline", origWidth, origHeight, width, height)
		}
	}
}

func processContentDimensions(cat *fragmentCatalog, content map[string]interface{}, desc string, origWidth, origHeight, width, height int) {
	if content == nil {
		return
	}

	type dimUpdate struct {
		prop    string
		origVal int
		newVal  int
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
					log.Printf("kfx: warning: Unexpected cover %s %s %d (expected %d)", desc, u.prop, currentVal, u.origVal)
				}
				content[u.prop] = u.newVal
			}
		}
	}

	// Python yj_metadata.py:813-814: recursively process $157 style fragments
	//   if "$157" in content:
	//       process_content(self.fragments.get(ftype="$157", fid=content.get("$157")).value, desc)
	if styleRef, ok := asString(content["$157"]); ok && styleRef != "" {
		if styleFragment, exists := cat.StyleFragments[styleRef]; exists {
			processContentDimensions(cat, styleFragment, desc, origWidth, origHeight, width, height)
		}
	}

	// Process sub-children ($146)
	if children, ok := asSlice(content["$146"]); ok {
		for _, rawChild := range children {
			child, ok := asMap(rawChild)
			if ok {
				processContentDimensions(cat, child, desc, origWidth, origHeight, width, height)
			}
		}
	}
}

func processStorylineDimensions(cat *fragmentCatalog, storyline map[string]interface{}, desc string, origWidth, origHeight, width, height int) {
	if children, ok := asSlice(storyline["$146"]); ok {
		for _, rawChild := range children {
			child, ok := asMap(rawChild)
			if ok {
				processContentDimensions(cat, child, desc, origWidth, origHeight, width, height)
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
