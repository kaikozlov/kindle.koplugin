package kfx

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
	"github.com/kaikozlov/kindle-koplugin/internal/jxr"
)

// ---------------------------------------------------------------------------
// Resource variant handling & deduplication
// Port of yj_to_epub_resources.py lines 20-245
// ---------------------------------------------------------------------------

// USE_HIGHEST_RESOLUTION_IMAGE_VARIANT mirrors the Python constant of the same name.
const USE_HIGHEST_RESOLUTION_IMAGE_VARIANT = true

// resourceObj mirrors Python's Obj class (yj_to_epub_resources.py line 25)
// and the resource object created in get_external_resource.
type resourceObj struct {
	rawMedia          []byte
	filename          string
	extension         string
	format            string // resource format symbol (e.g., "$285" for jpg)
	mime              string
	location          string
	width             int
	height            int
	referredResources []string
	manifestEntry     *manifestEntry
	isSaved           bool
}

// outputFile mirrors Python's OutputFile (epub_output.py line 213).
type outputFile struct {
	binaryData []byte
	mimetype   string
	height     int
	width      int
}

// manifestEntry mirrors Python's ManifestEntry (epub_output.py line 181).
type manifestEntry struct {
	filename      string
	id            string
	mimetype      string // set only for referred resources (Python line 219)
	referenceCount int
}

// resourceProcessor implements the KFX_EPUB_Resources mixin class from
// yj_to_epub_resources.py, handling resource caching, variant selection,
// and deduplication.
type resourceProcessor struct {
	resourceCache    map[string]*resourceObj    // cache of processed resources by name
	usedRawMedia     map[string]bool            // set of accessed raw media locations
	fragments        map[string]map[string]interface{} // synthetic fragment store: "$164:name" → fragment data
	rawMedia         map[string][]byte          // raw media data ($417 equivalent)
	oebpsFiles       map[string]*outputFile     // saved files (dedup target)
	manifestFiles    map[string]*manifestEntry  // filename → manifest entry
	manifestRefCount map[string]int             // reference counting by filename
	usedOEBPSNames   map[string]struct{}        // used OEBPS filenames for dedup
}

// getExternalResource implements Python get_external_resource (yj_to_epub_resources.py lines 35-183).
// It looks up a resource fragment ($164), resolves its raw media, handles variant selection,
// and caches the result.
//
// Python reference: yj_to_epub_resources.py:35-183 — handles $636 tiles, $165/$166 search path,
// $214 external refs, $548 JXR conversion, $565 PDF page extraction, variant selection.
func (rp *resourceProcessor) getExternalResource(resource_name string, ignore_variants bool) *resourceObj {
	// 1. Check cache
	if cached, ok := rp.resourceCache[resource_name]; ok && cached != nil {
		return cached
	}

	// 2. Get fragment $164
	resource, ok := rp.fragments["$164:"+resource_name]
	if !ok {
		log.Printf("kfx: error: resource fragment $164:%s not found", resource_name)
		return nil
	}

	// 3. Validate internal name
	intResourceName, _ := asString(resource["$175"])
	if intResourceName != resource_name {
		log.Printf("kfx: error: Name of resource %s is incorrect: %s", resource_name, intResourceName)
	}

	// 4. Extract dimensions
	resourceFormat, _ := asString(resource["$161"])
	fixedHeight := asIntDefault(resource["$67"], 0)
	fixedWidth := asIntDefault(resource["$66"], 0)
	resourceHeight := asIntDefault(resource["$423"], 0)
	if resourceHeight == 0 {
		resourceHeight = fixedHeight
	}
	resourceWidth := asIntDefault(resource["$422"], 0)
	if resourceWidth == 0 {
		resourceWidth = fixedWidth
	}

	// 5. Get location and raw media — with $636 tile support
	var location string
	var rawMedia []byte

	if _, hasTiles := resource["$636"]; hasTiles {
		// Python: yj_to_epub_resources.py:68-82 — tile reassembly path
		yjTilesRaw := resource["$636"]
		tileHeightVal := asIntDefault(resource["$638"], 0)
		tileWidthVal := asIntDefault(resource["$637"], 0)
		tilePadding := asIntDefault(resource["$797"], 0)

		yjTiles := parseTilesGrid(yjTilesRaw)
		location = tileBaseLocation(yjTiles)

		tilesRawMedia := make([][]byte, 0)
		for _, row := range yjTiles {
			for _, tileLocation := range row {
				tilesRawMedia = append(tilesRawMedia, rp.locateRawMedia(tileLocation, true))
			}
		}

		combinedMedia, combinedFormat := combineImageTiles(
			resource_name, resourceHeight, resourceWidth, resourceFormat,
			tileHeightVal, tileWidthVal, tilePadding,
			yjTiles, tilesRawMedia, ignore_variants,
		)
		rawMedia = combinedMedia
		if combinedFormat != "" {
			resourceFormat = combinedFormat
		}
	} else {
		// Python: yj_to_epub_resources.py:83-86 — direct resource path
		location, _ = asString(resource["$165"])
		searchPath, _ := asString(resource["$166"])
		if searchPath == "" {
			searchPath = location
		}
		if searchPath != location {
			log.Printf("kfx: error: Image resource %s has location %s != search_path %s", resource_name, location, searchPath)
		}
		rawMedia = rp.locateRawMedia(location, true)
	}

	// 6. Early return for ignore_variants with missing raw_media
	if ignore_variants && rawMedia == nil {
		return nil
	}

	// 7. Determine extension from format
	extension := extensionForFormatSymbol(resourceFormat)

	// 8. Get MIME type from fragment
	mime, _ := asString(resource["$162"])

	// 9. Determine filename from location
	locationFn := location
	if !strings.HasSuffix(locationFn, extension) {
		dotIdx := strings.Index(locationFn, ".")
		if dotIdx >= 0 {
			locationFn = locationFn[:dotIdx] + extension
		} else {
			locationFn = locationFn + extension
		}
	}

	// 10. Get referred resources
	referredResources := []string{}
	if rr, ok := asSlice(resource["$167"]); ok {
		for _, v := range rr {
			if s, ok := asString(v); ok {
				referredResources = append(referredResources, s)
			}
		}
	}

	// 11. Process $214 external resource references (Python: yj_to_epub_resources.py:86-87)
	if extRef, ok := asString(resource["$214"]); ok && extRef != "" {
		rp.processExternalResource(extRef, false, false, false, false, false)
	}

	// 12. JXR conversion (Python: yj_to_epub_resources.py:89-91)
	if FIX_JPEG_XR && resourceFormat == "$548" && rawMedia != nil {
		convertedData, convertedFormat := convertJXRToJpegOrPNG(rawMedia, locationFn)
		rawMedia = convertedData
		resourceFormat = convertedFormat
		extension = extensionForFormatSymbol(resourceFormat)
		locationFn = replaceExtension(locationFn, extension)
	}

	// 13. PDF page extraction (Python: yj_to_epub_resources.py:93-115)
	suffix := ""
	if resourceFormat == "$565" && rawMedia != nil {
		if pageNumVal, ok := asInt(resource["$564"]); ok {
			pageNum := pageNumVal + 1
			suffix = pdfPageSuffix(pageNum)
		} else {
			// no $564 → page_num = 1
		}

		if FIX_PDF {
			pageNum := 1
			if pn, ok := asInt(resource["$564"]); ok {
				pageNum = pn + 1
			}
			imgData, imgFmt := convertPDFPageToImage(location, rawMedia, pageNum, nil, false)
			rawMedia = imgData
			resourceFormat = imgFmt
			mime = ""
			extension = extensionForFormatSymbol(resourceFormat)
			locationFn = replaceExtension(locationFn, extension)
		}
	}

	// 14. Generate filename using existing helper
	filename := uniquePackageResourceFilename(resourceFragment{
		ID:        resource_name,
		Location:  locationFn,
		MediaType: "image/jpeg", // simplified; full parity uses SYMBOL_FORMATS
	}, symOriginal, rp.usedOEBPSNames)

	// Apply page suffix to filename
	if suffix != "" {
		dot := strings.LastIndex(filename, ".")
		if dot >= 0 {
			filename = filename[:dot] + suffix + filename[dot:]
		} else {
			filename = filename + suffix
		}
	}

	// 15. VARIANT SELECTION — key logic (Python lines 170-179)
	if !ignore_variants {
		variants, _ := asSlice(resource["$635"])
		for _, vr := range variants {
			variantName, _ := asString(vr)
			if variantName == "" {
				continue
			}
			variant := rp.getExternalResource(variantName, true) // ignore_variants=True to prevent infinite recursion
			if USE_HIGHEST_RESOLUTION_IMAGE_VARIANT && variant != nil &&
				variant.width > resourceWidth && variant.height > resourceHeight {
				// Replace with higher-resolution variant
				rawMedia = variant.rawMedia
				filename = variant.filename
				resourceWidth = variant.width
				resourceHeight = variant.height
			}
		}
	}

	// 16. Cache result
	result := &resourceObj{
		rawMedia:          rawMedia,
		filename:          filename,
		extension:         extension,
		format:            resourceFormat,
		mime:              mime,
		location:          location,
		width:             resourceWidth,
		height:            resourceHeight,
		referredResources: referredResources,
		manifestEntry:     nil,
	}
	rp.resourceCache[resource_name] = result
	return result
}

// parseTilesGrid converts the $636 value to a 2D string grid.
// Python: yj_tiles is a 2D array of IonSymbol location strings.
func parseTilesGrid(val interface{}) [][]string {
	rows, ok := asSlice(val)
	if !ok {
		return nil
	}
	result := make([][]string, len(rows))
	for i, row := range rows {
		cols, ok := asSlice(row)
		if !ok {
			result[i] = []string{}
			continue
		}
		result[i] = make([]string, len(cols))
		for j, col := range cols {
			result[i][j], _ = asString(col)
		}
	}
	return result
}

// tileBaseLocation extracts the base location from the first tile.
// Python: yj_tiles[0][0].partition("-tile")[0]
func tileBaseLocation(yjTiles [][]string) string {
	if len(yjTiles) == 0 || len(yjTiles[0]) == 0 {
		return ""
	}
	firstTile := yjTiles[0][0]
	if idx := strings.Index(firstTile, "-tile"); idx >= 0 {
		return firstTile[:idx]
	}
	return firstTile
}

// replaceExtension replaces the extension in a filename.
// Python: location_fn.rpartition(".")[0] + extension
func replaceExtension(filename string, newExt string) string {
	dot := strings.LastIndex(filename, ".")
	if dot >= 0 {
		return filename[:dot] + newExt
	}
	return filename + newExt
}

// processExternalResource implements Python process_external_resource (yj_to_epub_resources.py lines 185-233).
// It handles resource deduplication via binary data comparison and manifest management.
func (rp *resourceProcessor) processExternalResource(resource_name string, save, process_referred, save_referred, is_plugin, is_referred bool) *resourceObj {
	resourceObj := rp.getExternalResource(resource_name, false)
	if resourceObj == nil {
		return nil
	}

	if save && resourceObj.rawMedia != nil {
		if resourceObj.manifestEntry == nil {
			// Determine filename: root_filename(location) if is_referred, else resource_obj.filename
			filename := resourceObj.filename
			if is_referred {
				filename = rootFilename(resourceObj.location)
			}

			// Handle fragment separator (#page=...)
			filename, fragmentSep, fragment := partitionOnHash(filename)
			baseFilename := filename

			// Handle duplicates: check if same data already exists in oebps_files
			// Python: while filename in self.oebps_files: ... else: ...
			cnt := 0
			for {
				existing, exists := rp.oebpsFiles[filename]
				if !exists {
					// Filename is free — exit loop (Python's "else" clause)
					break
				}
				if existing.binaryData != nil && bytes.Equal(existing.binaryData, resourceObj.rawMedia) {
					// Same binary data — reuse manifest entry (dedup)
					me, ok := rp.manifestFiles[filename]
					if ok && me != nil {
						resourceObj.manifestEntry = me
						me.referenceCount++
						break
					}
				}
				// Generate unique filename with _N suffix
				fn, ext := splitExt(baseFilename)
				filename = fmt.Sprintf("%s_%d%s", fn, cnt, ext)
				cnt++
			}

			if resourceObj.manifestEntry == nil {
				// New file: add to manifest
				me := &manifestEntry{
					filename:      filename,
					referenceCount: 1,
				}
				// Python line 219: mimetype set only for referred resources
				if is_referred {
					me.mimetype = resourceObj.mime
				}
				resourceObj.manifestEntry = me
				rp.manifestFiles[filename] = me
				rp.oebpsFiles[filename] = &outputFile{
					binaryData: resourceObj.rawMedia,
					height:     resourceObj.height,
					width:      resourceObj.width,
				}
			}

			resourceObj.filename = filename + fragmentSep + fragment
			resourceObj.isSaved = true
		} else {
			resourceObj.manifestEntry.referenceCount++
		}
	}

	// Process referred resources
	if process_referred || save_referred {
		for _, rr := range resourceObj.referredResources {
			rp.processExternalResource(rr, save_referred, false, false, false, true)
		}
	}

	return resourceObj
}

// locateRawMedia implements Python locate_raw_media (yj_to_epub_resources.py lines 235-245).
// It looks up raw binary data from the raw_media store ($417 equivalent) and records
// the location as used.
func (rp *resourceProcessor) locateRawMedia(location string, report_missing bool) []byte {
	data, ok := rp.rawMedia[location]
	if !ok {
		if report_missing {
			log.Printf("kfx: error: Missing bcRawMedia %s", location)
		}
		return nil
	}
	rp.usedRawMedia[location] = true
	return data
}

// extensionForFormatSymbol maps a YJ resource format symbol to a file extension.
// Mirrors Python's SYMBOL_FORMATS reverse lookup (resources.py).
func extensionForFormatSymbol(format string) string {
	switch format {
	case "$285":
		return ".jpg"
	case "$284":
		return ".png"
	case "$548":
		return ".jxr"
	case "$286":
		return ".gif"
	case "$565":
		return ".pdf"
	case "$287":
		return ".pobject"
	case "$420":
		return ".pbm"
	case "$600":
		return ".tiff"
	case "$612":
		return ".bpg"
	default:
		return ".bin"
	}
}

// rootFilename implements Python utilities.root_filename (utilities.py line 341).
// If name starts with "/" return name, else return "/" + name.
func rootFilename(name string) string {
	if strings.HasPrefix(name, "/") {
		return name
	}
	return "/" + name
}

// partitionOnHash splits a string at the first "#" character, returning
// (before, "#", after) or (s, "", "") if no "#" is found.
func partitionOnHash(s string) (before, sep, after string) {
	idx := strings.Index(s, "#")
	if idx < 0 {
		return s, "", ""
	}
	return s[:idx], "#", s[idx+1:]
}

// splitExt splits a filename into base and extension.
// Equivalent to Python's posixpath.splitext.
func splitExt(filename string) (string, string) {
	dot := strings.LastIndex(filename, ".")
	if dot < 0 {
		return filename, ""
	}
	return filename[:dot], filename[dot:]
}

// ---------------------------------------------------------------------------
// End resource variant handling
// ---------------------------------------------------------------------------

func parseResourceFragment(fragmentID string, value map[string]interface{}) resourceFragment {
	resourceID, _ := asString(value["$175"])
	if resourceID == "" {
		resourceID = fragmentID
	}
	location, _ := asString(value["$165"])
	mediaType, _ := asString(value["$162"])

	// Width/height from $422/$423 (or $66/$67 fallback)
	width, _ := asInt(value["$422"])
	if width == 0 {
		width, _ = asInt(value["$66"])
	}
	height, _ := asInt(value["$423"])
	if height == 0 {
		height, _ = asInt(value["$67"])
	}

	// $635 variant references
	var variants []string
	if v, ok := asSlice(value["$635"]); ok {
		for _, item := range v {
			if name, ok := asString(item); ok && name != "" {
				variants = append(variants, name)
			}
		}
	}

	return resourceFragment{
		ID:        resourceID,
		Location:  location,
		MediaType: mediaType,
		Width:     width,
		Height:    height,
		Variants:  variants,
	}
}

func parseFontFragment(value map[string]interface{}) fontFragment {
	location, _ := asString(value["$165"])
	family, _ := asString(value["$11"])

	return fontFragment{
		Location: location,
		Family:   family,
		Style:    mapFontStyle(value["$12"]),
		Weight:   mapFontWeight(value["$13"]),
		Stretch:  mapFontStretch(value["$15"]),
	}
}

func buildResources(book *decodedBook, resources map[string]resourceFragment, fonts map[string]fontFragment, raw map[string][]byte, rawOrder []rawBlob, symFmt symType) ([]epub.Resource, string, string, map[string]string) {
	var output []epub.Resource
	imagePool, fontPool := partitionRawBlobs(rawOrder)
	imageCursor := 0
	fontCursor := 0

	resourceIDs := make([]string, 0, len(resources))
	for resourceID := range resources {
		resourceIDs = append(resourceIDs, resourceID)
	}
	sort.Strings(resourceIDs)

	resourceFilenameByID := map[string]string{}
	usedOEBPSNames := map[string]struct{}{}
	firstImageFilename := ""
	for _, resourceID := range resourceIDs {
		resource := resources[resourceID]
		data := raw[resource.Location]
		isImage := strings.HasPrefix(strings.ToLower(resource.MediaType), "image/")

		// $635 variant selection (Python yj_to_epub_resources.py:162-172)
		// If USE_HIGHEST_RESOLUTION_IMAGE_VARIANT and a variant has higher resolution,
		// use the variant's data and filename instead. The base resourceID maps to
		// the variant's filename so that references to the base symbol resolve correctly.
		if USE_HIGHEST_RESOLUTION_IMAGE_VARIANT && isImage && len(resource.Variants) > 0 {
			for _, variantID := range resource.Variants {
				vr, ok := resources[variantID]
				if !ok {
					continue
				}
				if vr.Width > resource.Width && vr.Height > resource.Height {
					variantData := raw[vr.Location]
					if len(variantData) == 0 {
						variantData, _ = nextMatchingBlob(imagePool, 0, vr.MediaType)
					}
					if len(variantData) > 0 {
						data = variantData
						resource = vr // use variant's ID/location for filename
						break
					}
				}
			}
		}
		if resourceID == "eAV" {
		}
		if isImage && !blobMatchesImageMediaType(data, resource.MediaType) {
			data = nil
		}
		if len(data) == 0 && isImage {
			data, imageCursor = nextMatchingBlob(imagePool, imageCursor, resource.MediaType)
		}
		if len(data) == 0 {
			if isImage {
				fmt.Fprintf(os.Stderr, "kfx: warning: skipping resource %q (%s): no image bytes (Calibre log.warning class)\n", resource.ID, resource.Location)
			}
			continue
		}
		mediaType := resource.MediaType
		if strings.EqualFold(mediaType, "image/jpg") {
			mediaType = "image/jpeg"
		}
		if strings.EqualFold(mediaType, "image/jxr") {
			convertedData, convertedType, err := convertJXRResource(data)
			if err == nil {
				data = convertedData
				mediaType = convertedType
			}
		}
		filename := uniquePackageResourceFilename(resourceFragment{
			ID:        resource.ID,
			Location:  resource.Location,
			MediaType: mediaType,
		}, symFmt, usedOEBPSNames)
		output = append(output, epub.Resource{
			Filename:  filename,
			MediaType: mediaType,
			Data:      data,
		})
		resourceFilenameByID[resourceID] = filename
		if firstImageFilename == "" {
			firstImageFilename = filename
		}
	}

	fontLocations := make([]string, 0, len(fonts))
	for location := range fonts {
		fontLocations = append(fontLocations, location)
	}
	sort.Strings(fontLocations)

	var stylesheet strings.Builder
	fontFaceLines := make([]string, 0, len(fontLocations))
	for _, location := range fontLocations {
		font := fonts[location]
		data := raw[location]
		if detectFontExtension(data) == ".bin" {
			data = nil
		}
		if len(data) == 0 {
			data, fontCursor = nextFontBlob(fontPool, fontCursor)
		}
		if len(data) == 0 {
			fmt.Fprintf(os.Stderr, "kfx: warning: skipping font %q: no bytes (Calibre log.error class → continue)\n", location)
			continue
		}
		filename := uniqueFontPackageFilename(location, data, symFmt, usedOEBPSNames)
		output = append(output, epub.Resource{
			Filename:  filename,
			MediaType: fontMediaType(filename),
			Data:      data,
		})

		family := font.Family
		if currentFontFixer != nil {
			family = currentFontFixer.fixFontName(family, true, false)
		}
		declarations := []string{"font-family: " + quoteFontName(family)}
		if font.Style != "" && font.Style != "normal" {
			declarations = append(declarations, "font-style: "+font.Style)
		}
		if font.Weight != "" && font.Weight != "normal" {
			declarations = append(declarations, "font-weight: "+font.Weight)
		}
		if font.Stretch != "" && font.Stretch != "normal" {
			declarations = append(declarations, "font-stretch: "+font.Stretch)
		}
		declarations = append(declarations, "src: url("+filename+")")
		fontFaceLines = append(fontFaceLines, "@font-face {"+strings.Join(canonicalDeclarations(declarations), "; ")+"}")
	}
	sort.Strings(fontFaceLines)
	for index, line := range fontFaceLines {
		if index > 0 {
			stylesheet.WriteByte('\n')
		}
		stylesheet.WriteString(line)
	}

	coverImageHref := resourceFilenameByID[book.CoverImageID]
	if coverImageHref == "" && book.CoverImageID != "" {
		coverImageHref = firstImageFilename
	}
	for index := range output {
		if output[index].Filename == coverImageHref && coverImageHref != "" {
			output[index].Properties = "cover-image"
			break
		}
	}
	return output, coverImageHref, strings.TrimSpace(stylesheet.String()), resourceFilenameByID
}

// packageResourceStem mirrors KFX_EPUB.resource_location_filename name stem (yj_to_epub_resources.py):
// unique_part_of_local_symbol + prefix_unique_part_of_symbol with image vs resource type.
// Port of KFX_EPUB.resource_location_filename (yj_to_epub_resources.py L247+).
// Preserves path prefix from the resource location (e.g. "images/") matching Python's
// safe_location.rpartition("/") path extraction.
func packageResourceStem(resource resourceFragment, symFmt symType) (stem, ext string) {
	ext = extensionForMediaType(resource.MediaType)
	loc := resource.Location
	if loc == "" {
		loc = resource.ID
	}
	// Preserve path prefix from location (Python: safe_location.rpartition("/")).
	// Strip leading "/" → "_" per Python: if location.startswith("/"): location = "_" + location[1:]
	if strings.HasPrefix(loc, "/") {
		loc = "_" + loc[1:]
	}
	// Sanitize like Python: re.sub(r"[^A-Za-z0-9_/.-]", "_", location)
	safeLoc := sanitizeLocation(loc)
	dirPath, name := safeLoc, ""
	if idx := strings.LastIndex(safeLoc, "/"); idx >= 0 {
		dirPath = safeLoc[:idx+1]
		name = safeLoc[idx+1:]
	} else {
		dirPath = ""
		name = safeLoc
	}
	// Strip "resource/" prefix like Python: for prefix in ["resource/", ...]: if path.startswith(prefix): path = path[len(prefix):]
	if strings.HasPrefix(dirPath, "resource/") {
		dirPath = dirPath[len("resource/"):]
	}
	root := strings.TrimSuffix(name, filepath.Ext(name))
	if root == "" {
		root = resource.ID
	}
	rt := "resource"
	if strings.HasPrefix(strings.ToLower(resource.MediaType), "image/") {
		rt = "image"
	}
	u := uniquePartOfLocalSymbol(root, symFmt)
	prefixed := prefixUniquePartOfSymbol(u, rt)
	return dirPath + prefixed, ext
}

// sanitizeLocation mirrors Python re.sub(r"[^A-Za-z0-9_/.-]", "_", location).
func sanitizeLocation(loc string) string {
	var b strings.Builder
	b.Grow(len(loc))
	for _, r := range loc {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '/' || r == '.' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func uniquePackageResourceFilename(resource resourceFragment, symFmt symType, used map[string]struct{}) string {
	stem, ext := packageResourceStem(resource, symFmt)
	for n := 0; ; n++ {
		name := stem + ext
		if n > 0 {
			name = fmt.Sprintf("%s-%d%s", stem, n-1, ext)
		}
		key := strings.ToLower(name)
		if _, dup := used[key]; !dup {
			used[key] = struct{}{}
			return name
		}
	}
}

func fontStemFromLocation(location string, data []byte, symFmt symType) (stem, ext string) {
	ext = detectFontExtension(data)
	base := filepath.Base(location)
	if base == "" || base == "." {
		base = location
	}
	root := strings.TrimSuffix(base, filepath.Ext(base))
	if root == "" {
		root = base
	}
	u := uniquePartOfLocalSymbol(root, symFmt)
	return prefixUniquePartOfSymbol(u, "font"), ext
}

func uniqueFontPackageFilename(location string, data []byte, symFmt symType, used map[string]struct{}) string {
	stem, ext := fontStemFromLocation(location, data, symFmt)
	for n := 0; ; n++ {
		name := stem + ext
		if n > 0 {
			name = fmt.Sprintf("%s-%d%s", stem, n-1, ext)
		}
		key := strings.ToLower(name)
		if _, dup := used[key]; !dup {
			used[key] = struct{}{}
			return name
		}
	}
}

func extensionForMediaType(mediaType string) string {
	switch strings.ToLower(mediaType) {
	case "plugin/kfx-html-article", "text/html":
		return ".html"
	case "application/xhtml+xml":
		return ".xhtml"
	case "image/jpg", "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/jxr":
		return ".jxr"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4":
		return ".m4a"
	case "audio/ogg":
		return ".ogg"
	case "video/mp4":
		return ".mp4"
	case "application/azn-plugin-object":
		return ".pobject"
	default:
		return ".bin"
	}
}

func detectFontExtension(data []byte) string {
	if len(data) >= 4 {
		switch string(data[:4]) {
		case "OTTO":
			return ".otf"
		case "\x00\x01\x00\x00":
			return ".ttf"
		}
	}
	return ".bin"
}

func detectImageExtension(data []byte) string {
	if len(data) >= 4 {
		if bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}) {
			return ".jpg"
		}
		if bytes.HasPrefix(data, []byte{0x49, 0x49, 0xbc, 0x01}) {
			return ".jxr"
		}
		if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4e, 0x47}) {
			return ".png"
		}
	}
	return ".bin"
}

func blobMatchesImageMediaType(data []byte, mediaType string) bool {
	if len(data) == 0 {
		return false
	}
	switch extensionForMediaType(mediaType) {
	case ".jpg":
		return detectImageExtension(data) == ".jpg"
	case ".jxr":
		return detectImageExtension(data) == ".jxr"
	case ".png":
		return detectImageExtension(data) == ".png"
	default:
		return detectImageExtension(data) != ".bin"
	}
}

func partitionRawBlobs(rawOrder []rawBlob) ([]rawBlob, []rawBlob) {
	imagePool := make([]rawBlob, 0, len(rawOrder))
	fontPool := make([]rawBlob, 0, len(rawOrder))
	for _, blob := range rawOrder {
		switch {
		case detectFontExtension(blob.Data) != ".bin":
			fontPool = append(fontPool, blob)
		case detectImageExtension(blob.Data) != ".bin":
			imagePool = append(imagePool, blob)
		}
	}
	return imagePool, fontPool
}

func nextMatchingBlob(blobs []rawBlob, start int, mediaType string) ([]byte, int) {
	for index := start; index < len(blobs); index++ {
		if blobMatchesImageMediaType(blobs[index].Data, mediaType) {
			return blobs[index].Data, index + 1
		}
	}
	return nil, start
}

func nextFontBlob(blobs []rawBlob, start int) ([]byte, int) {
	for index := start; index < len(blobs); index++ {
		if detectFontExtension(blobs[index].Data) != ".bin" {
			return blobs[index].Data, index + 1
		}
	}
	return nil, start
}

func fontMediaType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".otf":
		return "font/otf"
	case ".ttf":
		return "font/ttf"
	default:
		return "application/octet-stream"
	}
}

// convertJXRResource is a pragmatic subset of resources.convert_jxr_to_jpeg_or_png (resources.py);
// full color JXR decode parity is Phase E backlog.
func convertJXRResource(data []byte) ([]byte, string, error) {
	img, err := jxr.DecodeGray8(data)
	if err != nil {
		return nil, "", err
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 95}); err != nil {
		return nil, "", err
	}
	return encoded.Bytes(), "image/jpeg", nil
}

// ---------------------------------------------------------------------------
// Resource Processing — Tile Reassembly, JXR Conversion, PDF Extraction
// Port of resources.py and yj_to_epub_resources.py resource handling
// ---------------------------------------------------------------------------

// Python constants from resources.py
const (
	COMBINE_TILES_LOSSLESS    = true
	MIN_JPEG_QUALITY          = 90
	MAX_JPEG_QUALITY          = 100
	COMBINED_TILE_SIZE_FACTOR = 1.2
	CONVERT_JXR_LOSSLESS      = false
	FIX_JPEG_XR               = true
	FIX_PDF                   = true
)

// IMAGE_COLOR_MODES maps to Python's list (resources.py:50-55).
// Used in combineImageTiles to determine the highest-quality color mode.
var IMAGE_COLOR_MODES = []string{"1", "L", "P", "RGB"}

const IMAGE_OPACITY_MODE = "A"

// combineImageTiles implements Python combine_image_tiles (resources.py:530-625).
// It combines a 2D grid of image tiles into a single full-size image,
// handling padding, cropping, and JPEG quality optimization.
func combineImageTiles(
	resourceName string, resourceHeight, resourceWidth int, resourceFormat string,
	tileHeight, tileWidth, tilePadding int,
	yjTiles [][]string, tilesRawMedia [][]byte, ignoreVariants bool,
) ([]byte, string) {

	tileImages := make([]image.Image, len(tilesRawMedia))
	separateTilesSize := 0
	tileCount := 0
	fullImageColorMode := IMAGE_COLOR_MODES[0] // "1"
	fullImageOpacityMode := ""

	missingTiles := false
	tileNum := 0
	for y, row := range yjTiles {
		for x := range row {
			tileRawMedia := tilesRawMedia[tileNum]
			if tileRawMedia != nil {
				tileCount++
				separateTilesSize += len(tileRawMedia)
				tile, _, err := image.Decode(bytes.NewReader(tileRawMedia))
				if err != nil {
					log.Printf("kfx: error: Resource %s tile %s failed to decode: %v", resourceName, yjTiles[y][x], err)
					tileImages[tileNum] = nil
				} else {
					tileImages[tileNum] = tile

					// Determine color mode from tile
					tileMode := tileColorMode(tile)
					if tileMode == "" {
						log.Printf("kfx: error: Resource %s tile %s has unexpected image mode", resourceName, yjTiles[y][x])
					} else {
						tileIdx := colorModeIndex(tileMode)
						currentIdx := colorModeIndex(fullImageColorMode)
						if tileIdx > currentIdx {
							fullImageColorMode = tileMode
						}
					}

					// Check opacity mode
					mode := fullImageModeString(tile)
					if strings.HasSuffix(mode, IMAGE_OPACITY_MODE) {
						fullImageOpacityMode = IMAGE_OPACITY_MODE
					}
				}
			} else {
				tileImages[tileNum] = nil
				log.Printf("kfx: error: Resource %s is missing tile (%d, %d): %s", resourceName, x, y, yjTiles[y][x])
				missingTiles = true
			}
			tileNum++
		}
	}

	if missingTiles && ignoreVariants {
		return nil, ""
	}

	// Create full image
	_ = fullImageColorMode + fullImageOpacityMode // fullMode for potential future RGBA support
	fullImage := image.NewRGBA(image.Rect(0, 0, resourceWidth, resourceHeight))

	for y, row := range yjTiles {
		topPadding := 0
		if y != 0 {
			topPadding = tilePadding
		}
		bottomPadding := tilePadding
		if val := resourceHeight - tileHeight*(y+1); val < bottomPadding {
			bottomPadding = val
		}

		for x, tileLocation := range row {
			leftPadding := 0
			if x != 0 {
				leftPadding = tilePadding
			}
			rightPadding := tilePadding
			if val := resourceWidth - tileWidth*(x+1); val < rightPadding {
				rightPadding = val
			}

			tileIdx := y*len(row) + x
			tile := tileImages[tileIdx]
			if tile != nil {
				tWidth := tile.Bounds().Dx()
				tHeight := tile.Bounds().Dy()
				if tWidth != tileWidth+leftPadding+rightPadding || tHeight != tileHeight+topPadding+bottomPadding {
					log.Printf("kfx: error: Resource %s tile %d, %d size (%d, %d) does not have padding %d of expected size (%d, %d)",
						resourceName, x, y, tWidth, tHeight, tilePadding, tileWidth, tileHeight)
				}

				// Crop: remove padding
				cropRect := image.Rect(leftPadding, topPadding, tileWidth+leftPadding, tileHeight+topPadding)
				// Use sub-image if available, otherwise just paste
				type subImager interface {
					SubImage(r image.Rectangle) image.Image
				}
				if si, ok := tile.(subImager); ok {
					tile = si.SubImage(cropRect)
				}

				// Paste tile into full image
				drawTile(fullImage, tile, x*tileWidth, y*tileHeight)
			}
			_ = tileLocation
		}
	}

	if fullImage.Bounds().Dx() != resourceWidth || fullImage.Bounds().Dy() != resourceHeight {
		log.Printf("kfx: error: Resource %s combined tiled image size is (%d, %d) but should be (%d, %d)",
			resourceName, fullImage.Bounds().Dx(), fullImage.Bounds().Dy(), resourceWidth, resourceHeight)
	}

	// Determine output format
	if resourceFormat == "$285" && COMBINE_TILES_LOSSLESS {
		resourceFormat = "$284"
	}

	if resourceFormat == "$285" {
		// JPEG: optimize quality
		desiredCombinedSize := max(int(float64(separateTilesSize)*COMBINED_TILE_SIZE_FACTOR), 1024)
		rawMedia := optimizeJPEGImageQuality(fullImage, desiredCombinedSize)
		return rawMedia, resourceFormat
	}

	// PNG or other format
	var buf bytes.Buffer
	png.Encode(&buf, fullImage)
	return buf.Bytes(), resourceFormat
}

// drawTile draws a tile image onto the destination at the given position.
// This replaces Python's Image.paste.
func drawTile(dst *image.RGBA, tile image.Image, x, y int) {
	tileBounds := tile.Bounds()
	for ty := tileBounds.Min.Y; ty < tileBounds.Max.Y; ty++ {
		for tx := tileBounds.Min.X; tx < tileBounds.Max.X; tx++ {
			dst.Set(x+tx-tileBounds.Min.X, y+ty-tileBounds.Min.Y, tile.At(tx, ty))
		}
	}
}

// optimizeJPEGImageQuality implements Python optimize_jpeg_image_quality (resources.py:628-649).
// Uses binary search to find the JPEG quality that produces output closest to desiredSize.
func optimizeJPEGImageQuality(jpegImage image.Image, desiredSize int) []byte {
	minQuality := MIN_JPEG_QUALITY
	maxQuality := MAX_JPEG_QUALITY
	var bestSizeDiff int
	var bestRawMedia []byte
	bestSet := false

	for maxQuality >= minQuality {
		quality := (maxQuality + minQuality) / 2
		var buf bytes.Buffer
		jpeg.Encode(&buf, jpegImage, &jpeg.Options{Quality: quality})
		rawMedia := buf.Bytes()

		sizeDiff := len(rawMedia) - desiredSize

		if !bestSet || absInt(sizeDiff) < absInt(bestSizeDiff) {
			bestSizeDiff = sizeDiff
			bestRawMedia = rawMedia
			bestSet = true
		}

		if len(rawMedia) < desiredSize {
			minQuality = quality + 1
		} else {
			maxQuality = quality - 1
		}
	}

	return bestRawMedia
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// tileColorMode returns the color mode of an image (Python's tile.mode without alpha).
func tileColorMode(img image.Image) string {
	switch img.(type) {
	case *image.Gray, *image.Gray16:
		return "L"
	case *image.NRGBA, *image.NRGBA64, *image.RGBA, *image.RGBA64:
		return "RGB"
	case *image.Paletted:
		return "P"
	default:
		return "RGB"
	}
}

// fullImageModeString returns the full mode string including alpha.
func fullImageModeString(img image.Image) string {
	switch img.(type) {
	case *image.NRGBA, *image.NRGBA64:
		return "RGBA"
	case *image.Gray, *image.Gray16:
		return "L"
	case *image.Paletted:
		return "P"
	default:
		return "RGB"
	}
}

// colorModeIndex returns the index of a color mode in IMAGE_COLOR_MODES.
// Higher index = higher quality.
func colorModeIndex(mode string) int {
	for i, m := range IMAGE_COLOR_MODES {
		if m == mode {
			return i
		}
	}
	return 0
}

// convertJXRToJpegOrPNG implements Python convert_jxr_to_jpeg_or_png (resources.py:269-292).
// It converts JXR image data to JPEG or PNG format.
// For non-JXR data, it passes through the existing image decoder and converts
// based on RGBA/lossless flags.
func convertJXRToJpegOrPNG(imageData []byte, resourceName string) ([]byte, string) {
	// Try JXR decode first
	img, err := jxr.DecodeGray8(imageData)
	if err == nil && img != nil {
		// JXR decoded successfully — convert to JPEG
		var buf bytes.Buffer
		jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95})
		return buf.Bytes(), "$285"
	}

	// Fallback: try standard image decode (for non-JXR data passed to this function)
	img2, _, err2 := image.Decode(bytes.NewReader(imageData))
	if err2 != nil {
		// Can't decode — return as-is with JPEG format (will be handled upstream)
		log.Printf("kfx: error: Exception during conversion of JPEG-XR '%s': %v", resourceName, err)
		return imageData, "$548"
	}

	// Determine output format based on color mode
	if CONVERT_JXR_LOSSLESS || hasAlpha(img2) {
		var buf bytes.Buffer
		png.Encode(&buf, img2)
		return buf.Bytes(), "$284"
	}

	var buf bytes.Buffer
	jpeg.Encode(&buf, img2, &jpeg.Options{Quality: 95})
	return buf.Bytes(), "$285"
}

// convertJXRToJpegOrPNG_RGBA handles RGBA images specifically → PNG output.
// Python: resources.py:279-280 — if img.mode == "RGBA" → PNG
func convertJXRToJpegOrPNG_RGBA(imageData []byte, resourceName string) ([]byte, string) {
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return imageData, "$284"
	}
	_ = resourceName

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes(), "$284"
}

// hasAlpha returns true if the image has an alpha channel with non-opaque pixels.
func hasAlpha(img image.Image) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a != 0xffff {
				return true
			}
		}
	}
	return false
}

// convertPDFPageToImage implements Python convert_pdf_page_to_image (resources.py:323-400).
// In Go, PDF rendering requires external tools (pdftoppm) or libraries.
// For the port, we create a placeholder that returns a default JPEG for valid PDF data
// and logs when the conversion can't be done natively.
// When external PDF rendering is not available, returns a minimal JPEG placeholder.
func convertPDFPageToImage(location string, pdfData []byte, pageNum int, reportedErrors map[string]bool, forceJPEG bool) ([]byte, string) {
	// Default image: create a minimal JPEG from the PDF page number
	// In production, this would call pdftoppm or similar
	defaultImage, defaultFormat := convertPDFPageToJPEG(location, pdfData, pageNum)
	return getPDFPageImage(location, pdfData, pageNum, forceJPEG, defaultImage, defaultFormat)
}

// convertPDFPageToJPEG creates a JPEG rendering of a PDF page.
// Python: resources.py:338-370 — uses pdftoppm subprocess.
// In Go, we generate a placeholder since pdftoppm may not be available.
func convertPDFPageToJPEG(location string, pdfData []byte, pageNum int) ([]byte, string) {
	if len(pdfData) == 0 || !bytes.HasPrefix(pdfData, []byte("%PDF")) {
		// Not a valid PDF — return placeholder
		img := image.NewGray(image.Rect(0, 0, 1, 1))
		var buf bytes.Buffer
		jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95})
		return buf.Bytes(), "$285"
	}

	// PDF is valid — for now generate a placeholder JPEG
	// In production, this would use an actual PDF renderer
	log.Printf("kfx: info: PDF page extraction for %s page %d (rendering placeholder)", location, pageNum)
	img := image.NewGray(image.Rect(0, 0, 612, 792))
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95})
	return buf.Bytes(), "$285"
}

// getPDFPageImage implements Python get_pdf_page_image (resources.py:373-400).
// It attempts to extract the image directly from the PDF, falling back to the rendered version.
func getPDFPageImage(location string, pdfData []byte, pageNum int, forceJPEG bool, defaultImage []byte, defaultFormat string) ([]byte, string) {
	// For now, return the default rendered image
	// Full implementation would use pypdf-style extraction
	_ = location
	_ = forceJPEG
	return defaultImage, defaultFormat
}

// pdfPageSuffix returns the filename suffix for a PDF page number.
// Python: yj_to_epub_resources.py:97-99 — suffix = "-page%d" % page_num
func pdfPageSuffix(pageNum int) string {
	return fmt.Sprintf("-page%d", pageNum)
}
