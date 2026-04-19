package kfx

import (
	"bytes"
	"fmt"
	"image/jpeg"
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

	// 5. Get location and raw media
	location, _ := asString(resource["$165"])
	rawMedia := rp.locateRawMedia(location, true)

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

	// 9. Get referred resources
	referredResources := []string{}
	if rr, ok := asSlice(resource["$167"]); ok {
		for _, v := range rr {
			if s, ok := asString(v); ok {
				referredResources = append(referredResources, s)
			}
		}
	}

	// 10. Generate filename using existing helper
	filename := uniquePackageResourceFilename(resourceFragment{
		ID:        resource_name,
		Location:  locationFn,
		MediaType: "image/jpeg", // simplified; full parity uses SYMBOL_FORMATS
	}, symOriginal, rp.usedOEBPSNames)

	// 11. VARIANT SELECTION — key logic (Python lines 170-179)
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

	// 12. Cache result
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

	return resourceFragment{
		ID:        resourceID,
		Location:  location,
		MediaType: mediaType,
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
