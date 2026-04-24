package kfx

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"log"
	"strings"
)

// ---------------------------------------------------------------------------
// Port of Python yj_metadata.py: metadata getter/query functions.
// These are methods of BookMetadata in Python; here they operate on fragmentCatalog.
//
// Python reference: REFERENCE/Calibre_KFX_Input/kfxlib/yj_metadata.py
// Go file: internal/kfx/yj_metadata.go
// ---------------------------------------------------------------------------

// metadataSymbols mirrors Python METADATA_SYMBOLS (yj_structure.py):
// metadata key name → YJ Ion symbol.
var metadataSymbols = map[string]string{
	"ASIN":              "ASIN",
	"asset_id":          "asset_id",
	"author":            "author",
	"cde_content_type":  "cde_content_type",
	"cover_image":       "cover_image",
	"description":       "description",
	"language":          "language",
	"orientation":       "orientation",
	"publisher":         "publisher",
	"reading_orders":    "reading_orders",
	"support_landscape": "support_landscape",
	"support_portrait":  "support_portrait",
	"title":             "title",
}

// symbolFormats maps format symbol → format string.
// Port of Python SYMBOL_FORMATS (resources.py).
var symbolFormats = map[string]string{
	"bmp": "bmp",
	"gif": "gif",
	"jpg": "jpg",
	"jxr": "jxr",
	"pbm": "pbm",
	"pdf": "pdf",
	"png": "png",
	"pobject": "pobject",
	"tiff": "tiff",
	"yj.bpg": "bpg",
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
		containers, ok := asSlice(cat.TitleMetadata["categorised_metadata"])
		if ok {
			for _, rawCM := range containers {
				cm, ok := asMap(rawCM)
				if !ok {
					continue
				}
				if asStringDefault(cm["category"]) != category {
					continue
				}
				entries, ok := asSlice(cm["metadata"])
				if !ok {
					continue
				}
				for _, rawKV := range entries {
					kv, ok := asMap(rawKV)
					if !ok {
						continue
					}
					if asStringDefault(kv["key"]) == name {
						return kv["value"]
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
		// In Go, $593 fragment value is decoded as a map with "content_list" key containing
		// the list of capability entries, or the map itself may be a list-like value.
		for _, fragment := range cat.FormatCapabilities {
			// Try $146 (children list) first — most common encoding
			var entries []interface{}
			if children, ok := asSlice(fragment["content_list"]); ok {
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
				if asStringDefault(fc["key"]) == feature {
					return fc["version"]
				}
			}
		}
	} else {
		// Search $585 fragment's $590 list (ContentFeatures)
		features, ok := asSlice(cat.ContentFeatures["features"])
		if !ok {
			return defaultVal
		}
		for _, rawCF := range features {
			cf, ok := asMap(rawCF)
			if !ok {
				continue
			}
			if asStringDefault(cf["namespace"]) != namespace {
				continue
			}
			if asStringDefault(cf["key"]) != feature {
				continue
			}
			versionInfo, _ := asMap(cf["version_info"])
			if versionInfo == nil {
				return defaultVal
			}
			versionMap, _ := asMap(versionInfo["version"])
			if versionMap == nil {
				return defaultVal
			}
			majorVersion := asIntDefault(versionMap["major_version"], 0)
			minorVersion := asIntDefault(versionMap["minor_version"], 0)
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
	_, _, _, err := getOrderedImageResources(cat, isFXL)
	result := err == nil
	ci.isImageBasedFXL = &result
	return result
}

// ---------------------------------------------------------------------------
// isKfxV1 — Python yj_metadata.py:338-341 (cached property)
//
// Checks the FIRST $270 fragment's version field == 1 (default 0).
// Python: fragment = self.fragments.get("container", first=True)
//         fragment.value.get("version", 0) == 1 if fragment is not None else False
// ---------------------------------------------------------------------------
func isKfxV1(cat *fragmentCatalog, ci *cacheInfo) bool {
	if ci.isKfxV1 != nil {
		return *ci.isKfxV1
	}
	result := false
	// Get the first $270 fragment ID (Python: first=True)
	if ids, ok := cat.FragmentIDsByType["container"]; ok && len(ids) > 0 {
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
		if formatSym, ok := asString(rawData["format"]); ok && formatSym == "pdf" {
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
	coverFmt, ok := asString(coverResourceRaw["format"])
	if !ok {
		return nil
	}
	// If the format is a symbol, resolve to string
	if resolved, mapped := symbolFormats[coverFmt]; mapped {
		coverFmt = resolved
	}

	// Get location
	coverLocation, ok := asString(coverResourceRaw["location"])
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
		name, _ := asString(gen["major_version"])
		pkgVersion, _ := asString(gen["minor_version"])
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
		navContainers, ok := asSlice(navRoot["nav_containers"])
		if !ok {
			continue
		}
		for _, rawNavContainer := range navContainers {
			navContainer, ok := asMap(rawNavContainer)
			if !ok {
				continue
			}
			navType, _ := asString(navContainer["nav_type"])
			if navType == "page_list" {
				children, ok := asSlice(navContainer["entries"])
				if ok {
					return len(children)
				}
			}
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// checkCoverSectionAndStoryline — Python yj_metadata.py:648-791
//
// Validates cover section structure. Checks section template types, resolves
// cover storyline images, validates page template layouts, and can modify the
// cover resource. Returns (resourceName, coverEID) on success, or an error
// describing what went wrong.
//
// Parameters:
//   - expectedResource: if non-empty, verify the cover uses this resource
//   - expectedOrigWidth/expectedOrigHeight: if non-zero, verify cover dimensions
//   - allowPDF: if true, allow PDF cover images ($565 format)
//
// Python symbol mapping (Go uses real names):
//
//	$141 → page_templates    $159 → type           $156 → layout
//	$140 → float             $326 → solid          $323 → overflow
//	$325 → radial            $320 → horizontal     $68  → visibility
//	$66  → fixed_width       $67  → fixed_height   $176 → story_name
//	$155 → id                $171 → condition       $259 → storyline
//	$146 → content_list      $271 → image           $270 → container
//	$175 → resource_name     $157 → style           $56  → width
//	$57  → height            $306 → em              $314 → mm
//	$307 → ex                $16  → font_size       $42  → line_height
//	$173 → style_name        $546 → jxr             $377 → padding_bounds
// ---------------------------------------------------------------------------
// authorSortName — Python yj_metadata.py L849-867
//
// Converts author name from "First [Middle] Last" to "Last, First [Middle]".
// Handles suffixes (Jr, Sr, PhD, MD, II, III, IV, etc.) by combining them
// with the last name token. Returns the name unchanged if it already contains
// a comma or has fewer than 2 tokens.
//
// PERSON_SUFFIXES = {"phd", "md", "ba", "ma", "dds", "msts", "sr", "senior",
//
//	"jr", "junior", "ii", "iii", "iv"}
//
// Branch audit:
//
//	Python L853: al = author.split()                     → Go: strings.Fields
//	Python L856: if len(al) < 2: return author           → Go: len check
//	Python L858: if len(al) > 2 and suffix match         → Go: suffix check
//	Python L860:   if al[-2].endswith(","): strip comma  → Go: HasSuffix check
//	Python L862:   al = al[0:-2] + [combined last two]   → Go: slice rebuild
//	Python L864: if "," in "".join(al): return author     → Go: strings.Contains
//	Python L867: return al[-1] + ", " + join(al[:-1])    → Go: last + ", " + rest
// ---------------------------------------------------------------------------
func authorSortName(author string) string {
	// Python L851: PERSON_SUFFIXES set
	personSuffixes := map[string]bool{
		"phd": true, "md": true, "ba": true, "ma": true, "dds": true,
		"msts": true, "sr": true, "senior": true, "jr": true,
		"junior": true, "ii": true, "iii": true, "iv": true,
	}

	// Python L853: al = author.split()
	al := strings.Fields(author)

	// Python L856: if len(al) < 2: return author
	if len(al) < 2 {
		return author
	}

	// Python L858: if len(al) > 2 and al[-1].replace(".", "").lower() in PERSON_SUFFIXES
	if len(al) > 2 {
		// Normalize last token: remove dots, lowercase
		normalized := strings.ToLower(strings.ReplaceAll(al[len(al)-1], ".", ""))
		if personSuffixes[normalized] {
			// Python L860: if al[-2].endswith(","): al[-2] = al[-2][:-1]
			if strings.HasSuffix(al[len(al)-2], ",") {
				al[len(al)-2] = al[len(al)-2][:len(al[len(al)-2])-1]
			}
			// Python L862: al = al[0:-2] + ["%s %s" % (al[-2], al[-1])]
			combined := al[len(al)-2] + " " + al[len(al)-1]
			al = append(al[:len(al)-2], combined)
		}
	}

	// Python L864: if "," in "".join(al): return author
	if strings.Contains(strings.Join(al, ""), ",") {
		return author
	}

	// Python L867: return al[-1] + ", " + " ".join(al[:-1])
	return al[len(al)-1] + ", " + strings.Join(al[:len(al)-1], " ")
}

// ---------------------------------------------------------------------------
// unsortAuthorName — Python yj_metadata.py L870-875
//
// Reverses the authorSortName transformation: "Last, First" → "First Last".
// If the name doesn't contain ", ", returns it unchanged.
//
// Branch audit:
//
//	Python L871: if ", " in author                     → Go: strings.Contains
//	Python L872:   last, sep, first = author.partition → Go: strings.SplitN
//	Python L873:   author = first + " " + last         → Go: recombine
//	Python L875: return author                          → Go: return
// ---------------------------------------------------------------------------
func unsortAuthorName(author string) string {
	// Python L871: if ", " in author
	if strings.Contains(author, ", ") {
		// Python L872: last, sep, first = author.partition(", ")
		parts := strings.SplitN(author, ", ", 2)
		if len(parts) == 2 {
			// Python L873: author = first + " " + last
			return parts[1] + " " + parts[0]
		}
	}
	// Python L875: return author
	return author
}

// ---------------------------------------------------------------------------
// checkCoverSectionAndStoryline — Python yj_metadata.py:648-791
//
// Validates cover section structure. Checks section template types, resolves
// cover storyline images, validates page template layouts, and can modify the
// cover resource. Returns (resourceName, coverEID) on success, or an error
// describing what went wrong.
//
// Parameters:
//   - expectedResource: if non-empty, verify the cover uses this resource
//   - expectedOrigWidth/expectedOrigHeight: if non-zero, verify cover dimensions
//   - allowPDF: if true, allow PDF cover images ($565 format)
//
// Python symbol mapping (Go uses real names):
//
//	$141 → page_templates    $159 → type           $156 → layout
//	$140 → float             $326 → solid          $323 → overflow
//	$325 → radial            $320 → horizontal     $68  → visibility
//	$66  → fixed_width       $67  → fixed_height   $176 → story_name
//	$155 → id                $171 → condition       $259 → storyline
//	$146 → content_list      $271 → image           $270 → container
//	$175 → resource_name     $157 → style           $56  → width
//	$57  → height            $306 → em              $314 → mm
//	$307 → ex                $16  → font_size       $42  → line_height
//	$173 → style_name        $546 → jxr             $377 → padding_bounds
//	$183 → position          $324 → scale_fit       $475 → spacing_percent_base
//	$69  → ignore            $164 → external_resource $161 → format
//	$565 → ellipsis (=PDF in format context)   $214 → thumbnails
// ---------------------------------------------------------------------------
func checkCoverSectionAndStoryline(
	cat *fragmentCatalog,
	expectedResource string,
	expectedOrigWidth, expectedOrigHeight int,
	allowPDF bool,
) (resourceName string, coverEID string, err error) {

	// L654: cover_section = self.fragments.get(ftype="$260", fid=self.ordered_section_names()[0]).value
	if len(cat.SectionOrder) == 0 {
		return "", "", fmt.Errorf("no sections in book")
	}
	firstSectionID := cat.SectionOrder[0]
	section, exists := cat.SectionFragments[firstSectionID]
	if !exists {
		return "", "", fmt.Errorf("first section %q not found", firstSectionID)
	}
	coverSection := section.RawValue
	if coverSection == nil {
		return "", "", fmt.Errorf("section %q has no raw value", firstSectionID)
	}

	// L655: page_templates = cover_section["$141"]
	pageTemplates, ok := asSlice(coverSection["page_templates"])
	if !ok || len(pageTemplates) == 0 {
		return "", "", fmt.Errorf("cover section has no page_templates")
	}

	// L658-663: Validate first page template
	// Python: if (page_template.get("$159") != "$270" or
	//           page_template.get("$156") not in ["$326", "$323"] or
	//           page_template.get("$140") not in [None, "$320", "$68"]):
	pageTemplate0, ok := asMap(pageTemplates[0])
	if !ok {
		return "", "", fmt.Errorf("page_template[0] is not a map")
	}
	if asStringDefault(pageTemplate0["type"]) != "container" {
		return "", "", fmt.Errorf("unexpected section template 0: type=%q, expected container", asStringDefault(pageTemplate0["type"]))
	}
	layout0 := asStringDefault(pageTemplate0["layout"])
	if layout0 != "solid" && layout0 != "overflow" {
		return "", "", fmt.Errorf("unexpected section template 0: layout=%q, expected solid or overflow", layout0)
	}
	float0, hasFloat0 := pageTemplate0["float"]
	if hasFloat0 && float0 != nil {
		floatStr := asStringDefault(float0)
		if floatStr != "horizontal" && floatStr != "visibility" {
			return "", "", fmt.Errorf("unexpected section template 0: float=%q", floatStr)
		}
	}

	// L666-670: Extract template properties
	templateLayout := layout0
	origWidth := pageTemplate0["fixed_width"]
	origHeight := pageTemplate0["fixed_height"]
	storyName := asStringDefault(pageTemplate0["story_name"])
	coverEID = asStringDefault(pageTemplate0["id"])

	// L672-678: Validate second page template if present
	if len(pageTemplates) > 1 {
		pageTemplate1, ok := asMap(pageTemplates[1])
		if !ok {
			return "", "", fmt.Errorf("page_template[1] is not a map")
		}
		if asStringDefault(pageTemplate1["type"]) != "container" {
			return "", "", fmt.Errorf("unexpected section template 1: type=%q", asStringDefault(pageTemplate1["type"]))
		}
		if asStringDefault(pageTemplate1["layout"]) != "radial" {
			return "", "", fmt.Errorf("unexpected section template 1: layout=%q, expected radial", asStringDefault(pageTemplate1["layout"]))
		}
		if pageTemplate1["condition"] == nil {
			return "", "", fmt.Errorf("unexpected section template 1: missing condition")
		}
		if asStringDefault(pageTemplate1["story_name"]) != storyName {
			return "", "", fmt.Errorf("unexpected section template 1: story_name=%q, expected %q", asStringDefault(pageTemplate1["story_name"]), storyName)
		}
	}

	// L680-681: Error if more than 2 page templates
	if len(pageTemplates) > 2 {
		return "", "", fmt.Errorf("found %d page_templates", len(pageTemplates))
	}

	// L683: Get cover storyline
	coverStoryline, exists := cat.Storylines[storyName]
	if !exists {
		return "", "", fmt.Errorf("storyline %q not found", storyName)
	}

	// L684-686: Validate content_list length
	contentList, ok := asSlice(coverStoryline["content_list"])
	if !ok || len(contentList) != 1 {
		return "", "", fmt.Errorf("unexpected storyline content_list len %d", len(contentList))
	}

	// L688: content = content_list[0]
	content, ok := asMap(contentList[0])
	if !ok {
		return "", "", fmt.Errorf("storyline content_list[0] is not a map")
	}

	// L688: Branch on template_layout
	if templateLayout == "solid" {
		// L689-736: template_layout == $326 (solid)

		// L689: Sub-branch on content type
		if asStringDefault(content["type"]) == "image" {
			// L690-704: Direct image content
			// L691: resource_name = content.get("$175")
			resourceName = asStringDefault(content["resource_name"])

			// L694-703: Validate style fragment if present
			if styleRef, ok := asString(content["style"]); ok && styleRef != "" {
				style, exists := cat.StyleFragments[styleRef]
				if !exists {
					return "", "", fmt.Errorf("style fragment %q not found", styleRef)
				}

				// L697-700: Validate style width
				styleWidth := style["width"]
				if styleWidth != nil {
					styleWidthMap, isMap := asMap(styleWidth)
					if !isMap || asStringDefault(styleWidthMap["unit"]) != "mm" {
						return "", "", fmt.Errorf("unexpected cover storyline style width %v", styleWidth)
					}
					if pctVal, ok := asInt(styleWidthMap["value"]); !ok || pctVal < 95 {
						return "", "", fmt.Errorf("unexpected cover storyline style width value %v", styleWidthMap["value"])
					}
				}

				// L702-703: Validate style has only allowed keys
				for key := range style {
					if key != "font_size" && key != "line_height" && key != "style_name" && key != "width" {
						return "", "", fmt.Errorf("unexpected cover storyline style property %s", key)
					}
				}
			}

			// L705-706: Validate content has only allowed keys
			for key := range content {
				if key != "id" && key != "style" && key != "resource_name" && key != "type" {
					return "", "", fmt.Errorf("unexpected cover storyline content %s", key)
				}
			}
		} else {
			// L707-736: Container/overflow layout path
			// Python: if (content.get("$159") != "$270" or content.get("$156") != "$323" or content.get("$546") != "$377"):
			if asStringDefault(content["type"]) != "container" || asStringDefault(content["layout"]) != "overflow" || asStringDefault(content["jxr"]) != "padding_bounds" {
				return "", "", fmt.Errorf("unexpected cover storyline content")
			}

			// L711-712: Get orig dimensions from content
			origWidth2Raw := content["width"]
			origHeight2Raw := content["height"]
			origWidth2, _ := asInt(origWidth2Raw)
			origHeight2, _ := asInt(origHeight2Raw)
			if origWidth2Raw == nil {
				origWidth2 = -1
			}
			if origHeight2Raw == nil {
				origHeight2 = -1
			}

			// L714-716: Get content_list2
			contentList2, ok := asSlice(content["content_list"])
			if !ok || len(contentList2) == 0 {
				return "", "", fmt.Errorf("missing cover storyline content_list2")
			}

			// L718-719: content2 = content_list2[0]
			content2, ok := asMap(contentList2[0])
			if !ok {
				return "", "", fmt.Errorf("content2 is not a map")
			}
			resourceName = asStringDefault(content2["resource_name"])

			// L721-724: Validate content2
			if asStringDefault(content2["type"]) != "image" || asStringDefault(content2["jxr"]) != "padding_bounds" || asStringDefault(content2["position"]) != "scale_fit" {
				return "", "", fmt.Errorf("unexpected cover storyline content2")
			}
			content2Width, _ := asInt(content2["width"])
			content2Height, _ := asInt(content2["height"])
			if content2Width != origWidth2 || content2Height != origHeight2 {
				return "", "", fmt.Errorf("unexpected cover storyline content2 dimensions")
			}

			// L726-728: Validate content2 keys
			for key := range content2 {
				if key != "id" && key != "width" && key != "height" && key != "jxr" && key != "resource_name" && key != "type" && key != "position" {
					return "", "", fmt.Errorf("unexpected cover storyline content2 %s", key)
				}
			}

			// L730-732: Validate content keys
			for key := range content {
				if key != "id" && key != "width" && key != "height" && key != "jxr" && key != "layout" && key != "type" && key != "content_list" {
					return "", "", fmt.Errorf("unexpected cover storyline content %s", key)
				}
			}
		}
	} else {
		// L737-768: template_layout != $326 (e.g., overflow)
		// Python: if (content.get("$159") != "$270" or content.get("$156") != "$326" or content.get("$140") != "$320"):
		if asStringDefault(content["type"]) != "container" || asStringDefault(content["layout"]) != "solid" || asStringDefault(content["float"]) != "horizontal" {
			return "", "", fmt.Errorf("unexpected cover storyline content")
		}

		// L747-748: Override orig dimensions from page_template
		origWidth = pageTemplate0["fixed_width"]
		origHeight = pageTemplate0["fixed_height"]

		// L749-751: Validate content keys
		for key := range content {
			if key != "id" && key != "fixed_width" && key != "fixed_height" && key != "spacing_percent_base" && key != "layout" && key != "float" && key != "type" && key != "content_list" {
				return "", "", fmt.Errorf("unexpected cover storyline content %s", key)
			}
		}

		// L753-755: Get content_list2
		contentList2, ok := asSlice(content["content_list"])
		if !ok || len(contentList2) == 0 {
			return "", "", fmt.Errorf("missing cover storyline content_list2")
		}

		// L759: content2 = content_list2[0]
		content2, ok := asMap(contentList2[0])
		if !ok {
			return "", "", fmt.Errorf("content2 is not a map")
		}

		// L761-762: Validate content2 type
		if asStringDefault(content2["type"]) != "image" {
			return "", "", fmt.Errorf("unexpected cover storyline content2")
		}

		// L764-766: Validate content2 width
		width2 := content2["width"]
		if width2 != nil {
			width2Map, isMap := asMap(width2)
			if !isMap || asStringDefault(width2Map["unit"]) != "mm" {
				return "", "", fmt.Errorf("unexpected cover storyline width %v", width2)
			}
			if pctVal, ok := asInt(width2Map["value"]); !ok || pctVal < 95 {
				return "", "", fmt.Errorf("unexpected cover storyline width value %v", width2Map["value"])
			}
		}

		// L768-770: Validate content2 height
		height2 := content2["height"]
		if height2 != nil {
			height2Map, isMap := asMap(height2)
			if !isMap || asStringDefault(height2Map["unit"]) != "mm" {
				return "", "", fmt.Errorf("unexpected cover storyline height %v", height2)
			}
			if pctVal, ok := asInt(height2Map["value"]); !ok || pctVal < 95 {
				return "", "", fmt.Errorf("unexpected cover storyline height value %v", height2Map["value"])
			}
		}

		// L772: resource_name = content2.get("$175")
		resourceName = asStringDefault(content2["resource_name"])

		// L774-776: Validate content2 keys
		for key := range content2 {
			if key != "id" && key != "width" && key != "height" && key != "type" && key != "resource_name" {
				return "", "", fmt.Errorf("unexpected cover storyline content2 %s", key)
			}
		}

		// L778-785: Check optional content3
		if len(contentList2) >= 2 {
			content3, ok := asMap(contentList2[1])
			if !ok {
				return "", "", fmt.Errorf("content3 is not a map")
			}
			if asStringDefault(content3["type"]) != "container" || asStringDefault(content3["layout"]) != "scale_fit" {
				return "", "", fmt.Errorf("unexpected cover storyline content3")
			}
			// Python: content3.get("$69") is not True
			if ignoreVal, ok := asBool(content3["ignore"]); !ok || !ignoreVal {
				return "", "", fmt.Errorf("unexpected cover storyline content3: ignore not true")
			}
			for key := range content3 {
				if key != "id" && key != "story_name" && key != "layout" && key != "ignore" && key != "type" {
					return "", "", fmt.Errorf("unexpected cover storyline content3 %s", key)
				}
			}
		}
	}

	// L770-773: Validate cover resource
	// Python: cover_resource = self.fragments.get(ftype="$164", fid=resource_name).value
	coverResource, exists := cat.ResourceRawData[resourceName]
	if !exists {
		return "", "", fmt.Errorf("cover resource %q not found", resourceName)
	}
	// Python: if cover_resource[IS("$161")] == "$565" and not allow_pdf:
	// Go: $161 → format, $565 → ellipsis (but in Python context, $565 = PDF format)
	// Note: In the Python code, IS("$161") checks the "$161" key, and "$565" is "pdf" format.
	// But actually looking at the catalog mapping: $565 maps to "ellipsis" (not pdf).
	// $563 = "pdf". Let me re-check...
	// Actually: $565 = pdf in the FORMAT context (from resources.py).
	// Looking at symbolFormats: "pdf" → "pdf". And in the Python code:
	//   SYMBOL_FORMATS = {"$284": "png", "$285": "jpg", "$286": "gif", ...}
	//   $565 in this context is checked against $161 which is format.
	//   In the Go catalog, $565 = "ellipsis", but that's wrong for format context.
	//   Wait - looking at the Python IS() function and how $565 is used:
	//   cover_resource[IS("$161")] gets the format field. Then checks if it equals IS("$565").
	//   In the Python symbol catalog, $565 = "pdf" (it's after "page_index"=$562).
	//   Actually the catalog shows $565 = "ellipsis" but that might be wrong.
	//   Let me check Python's actual value...
	// Actually, $565 in the FORMAT context is pdf. Let me check the actual mapping.
	// From the Python yj_metadata.py code: IS("$565") creates a symbol $565.
	// From the symbol catalog: $565 = "ellipsis"
	// But wait - looking at resources.py:
	//   SYMBOL_FORMATS = {"$284": "png", "$285": "jpg", "$286": "gif", "$287": "pobject", "$565": "pdf", ...}
	// So in the format context, $565 maps to "pdf".
	// In the Go code, the resource format is already resolved to a string like "pdf".
	// So we check if the format is "pdf".
	resourceFormat, _ := asString(coverResource["format"])
	if resourceFormat == "pdf" && !allowPDF {
		return "", "", fmt.Errorf("first page uses a PDF image")
	}

	// L774-776: Validate expected resource
	if expectedResource != "" && resourceName != expectedResource {
		return "", "", fmt.Errorf("first page does not use expected cover image")
	}

	// L778-780: Validate expected dimensions
	// Python uses direct comparison on the orig_width/orig_height values.
	// In Go, origWidth and origHeight may be int or other types.
	if expectedOrigWidth != 0 || expectedOrigHeight != 0 {
		origWidthInt, widthOK := asInt(origWidth)
		origHeightInt, heightOK := asInt(origHeight)
		if expectedOrigWidth != 0 && (!widthOK || origWidthInt != expectedOrigWidth) {
			return "", "", fmt.Errorf("first page does not use expected cover dimensions")
		}
		if expectedOrigHeight != 0 && (!heightOK || origHeightInt != expectedOrigHeight) {
			return "", "", fmt.Errorf("first page does not use expected cover dimensions")
		}
	}

	return resourceName, coverEID, nil
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
		{"width", origWidth, width},
		{"height", origHeight, height},
		{"fixed_width", origWidth, width},
		{"fixed_height", origHeight, height},
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
	//   if "style" in content:
	//       process_content(self.fragments.get(ftype="style", fid=content.get("style")).value, desc)
	if styleRef, ok := asString(content["style"]); ok && styleRef != "" {
		if styleFragment, exists := cat.StyleFragments[styleRef]; exists {
			processContentDimensions(cat, styleFragment, desc, origWidth, origHeight, width, height)
		}
	}

	// Process sub-children ($146)
	if children, ok := asSlice(content["content_list"]); ok {
		for _, rawChild := range children {
			child, ok := asMap(rawChild)
			if ok {
				processContentDimensions(cat, child, desc, origWidth, origHeight, width, height)
			}
		}
	}
}

func processStorylineDimensions(cat *fragmentCatalog, storyline map[string]interface{}, desc string, origWidth, origHeight, width, height int) {
	if children, ok := asSlice(storyline["content_list"]); ok {
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
	features, ok := asSlice(cat.ContentFeatures["features"])
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
