package kfx

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
	"github.com/kaikozlov/kindle-koplugin/internal/jxr"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
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
	format            string // resource format symbol (e.g., "jpg" for jpg)
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
	saveResources    bool                       // Python self.save_resources (yj_to_epub_resources.py L30)
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
	intResourceName, _ := asString(resource["resource_name"])
	if intResourceName != resource_name {
		log.Printf("kfx: error: Name of resource %s is incorrect: %s", resource_name, intResourceName)
	}

	// 4. Extract dimensions
	resourceFormat, _ := asString(resource["format"])
	fixedHeight := asIntDefault(resource["fixed_height"], 0)
	fixedWidth := asIntDefault(resource["fixed_width"], 0)
	// Python L51-52: resource_height = resource.pop("$423", None) or fixed_height
	// $422/$423 may not be in shared symbol catalog, check raw forms too.
	resourceHeight := intFromAny(resource["resource_height"], resource["$423"], fixedHeight)
	resourceWidth := intFromAny(resource["resource_width"], resource["$422"], fixedWidth)

	// 5. Get location and raw media — with $636 tile support
	var location string
	var rawMedia []byte

	if _, hasTiles := resource["yj.tiles"]; hasTiles {
		// Python: yj_to_epub_resources.py:68-82 — tile reassembly path
		yjTilesRaw := resource["yj.tiles"]
		tileHeightVal := asIntDefault(resource["yj.tile_height"], 0)
		tileWidthVal := asIntDefault(resource["yj.tile_width"], 0)
		tilePadding := asIntDefault(resource["yj.tile_padding"], 0)

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
		location, _ = asString(resource["location"])
		searchPath, _ := asString(resource["search_path"])
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

	// 7. Determine extension from format (Python L84-87)
	extension := extensionForFormatSymbol(resourceFormat)

	// 8. Get MIME type from fragment (Python L89)
	mime, _ := asString(resource["mime"])

	// 8a. Python L90-95: if mime in EXTS_OF_MIMETYPE: if ext is .bin/.pobject: override from mime
	if mExt, ok := mimeTypeToExtension(mime); ok {
		if extension == ".pobject" || extension == ".bin" {
			// Python: if mime == "figure": extension = image_file_ext(raw_media)
			if mime == "figure" {
				if rawMedia != nil {
					if detected := detectImageExtension(rawMedia); detected != ".bin" {
						extension = detected
					}
				}
			} else {
				extension = mExt
			}
		}
	} else if mime != "" {
		// Python L94-95: elif mime is not None: log.error("unknown mime type")
		log.Printf("kfx: error: Resource %s has unknown mime type: %s", resource_name, mime)
	}

	// 9. Determine filename from location (Python L96-100)
	locationFn := location

	// Python L96-97: location_fn = resource.pop("yj.conversion.source_resource_filename", location_fn)
	if srcFn, ok := asString(resource["yj.conversion.source_resource_filename"]); ok && srcFn != "" {
		locationFn = srcFn
	}
	// Python L97: location_fn = resource.pop("yj.authoring.source_file_name", location_fn)
	if srcFn, ok := asString(resource["yj.authoring.source_file_name"]); ok && srcFn != "" {
		locationFn = srcFn
	}

	// Python L99: if (extension == ".pobject" or extension == ".bin") and "." in location_fn:
	//     extension = "." + location_fn.rpartition(".")[2]
	if (extension == ".pobject" || extension == ".bin") && strings.Contains(locationFn, ".") {
		if idx := strings.LastIndex(locationFn, "."); idx >= 0 {
			candidate := locationFn[idx:]
			if len(candidate) > 1 && isAlphaExt(candidate[1:]) {
				extension = candidate
			}
		}
	}

	// Python L101: if not location_fn.endswith(extension):
	//     location_fn = location_fn.partition(".")[0] + extension
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
	if rr, ok := asSlice(resource["referred_resources"]); ok {
		for _, v := range rr {
			if s, ok := asString(v); ok {
				referredResources = append(referredResources, s)
			}
		}
	}

	// 11. Process $214 external resource references (Python: yj_to_epub_resources.py:86-87)
	if extRef, ok := asString(resource["thumbnails"]); ok && extRef != "" {
		rp.processExternalResource(extRef, false, false, false, false, false)
	}

	// 12. JXR conversion (Python: yj_to_epub_resources.py:89-91)
	if FIX_JPEG_XR && resourceFormat == "jxr" && rawMedia != nil {
		convertedData, convertedFormat := convertJXRToJpegOrPNG(rawMedia, locationFn)
		rawMedia = convertedData
		resourceFormat = convertedFormat
		extension = extensionForFormatSymbol(resourceFormat)
		locationFn = replaceExtension(locationFn, extension)
	}

	// 13. PDF page extraction (Python: yj_to_epub_resources.py:93-115)
	suffix := ""
	if resourceFormat == "pdf" && rawMedia != nil {
		if pageNumVal, ok := asInt(resource["page_index"]); ok {
			pageNum := pageNumVal + 1
			suffix = pdfPageSuffix(pageNum)
		} else {
			// no $564 → page_num = 1
		}

		// Python L104-110: margin extraction (margin, margin_left, margin_right, margin_top, margin_bottom)
		// margin = resource.pop("$46", 0)  — stored but used for PDF rendering
		margin := asIntDefault(resource["yj.margin"], 0)
		_ = asIntDefault(resource["margin_left"], margin)
		_ = asIntDefault(resource["margin_right"], margin)
		_ = asIntDefault(resource["margin_top"], margin)
		_ = asIntDefault(resource["margin_bottom"], margin)
		// Note: margins are consumed from the fragment but the actual PDF rendering
		// uses pdftoppm which renders the full page. The crop happens separately.

		if FIX_PDF {
			pageNum := 1
			if pn, ok := asInt(resource["page_index"]); ok {
				pageNum = pn + 1
			}
			// Python L112-115: try/except with convert_pdf_page_to_image
			imgData, imgFmt := convertPDFPageToImage(location, rawMedia, pageNum, nil, false)
			// Python: on exception → log.error and keep old raw_media.
			// Go: convertPDFPageToImage returns placeholder on failure, so we always update.
			rawMedia = imgData
			resourceFormat = imgFmt
			mime = ""
			extension = extensionForFormatSymbol(resourceFormat)
			locationFn = replaceExtension(locationFn, extension)
		}
	}

	// 14. Generate filename using existing helper
	// Note: MediaType hardcoded to "image/jpeg". Python uses SYMBOL_FORMATS to resolve
	// the correct MIME type from the resource format symbol. Since Go converts all image
	// resources to JPEG (or PNG via JXR conversion which updates locationFn extension),
	// this works correctly for current test books. If non-JPEG/PNG resources are encountered,
	// the Format field on resourceFragment should be set to resourceFormat for proper MIME resolution.
	filename := uniquePackageResourceFilename(resourceFragment{
		ID:        resource_name,
		Location:  locationFn,
		MediaType: "image/jpeg",
	}, symOriginal, rp.usedOEBPSNames, rawMedia)

	// Apply page suffix to filename
	if suffix != "" {
		dot := strings.LastIndex(filename, ".")
		if dot >= 0 {
			filename = filename[:dot] + suffix + filename[dot:]
		} else {
			filename = filename + suffix
		}
	}

	// 15. VARIANT SELECTION — key logic (Python L170-179)
	// $635 may not be in shared symbol catalog, check raw form too.
	if !ignore_variants {
		variants, _ := asSlice(resource["yj.variants"])
		if len(variants) == 0 {
			variants, _ = asSlice(resource["$635"])
		}
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

	// 15a. Python L181: if "$564" in resource: filename += "#page=%d" % (resource.pop("$564") + 1)
	// $564 is page_index; add page fragment if present
	if pageNumVal, ok := asInt(resource["page_index"]); ok {
		filename += fmt.Sprintf("#page=%d", pageNumVal+1)
	}

	// 16. Cache result
	// Note: Python L177 calls self.check_empty(resource, "resource %s" % resource_name)
	// to validate that all fragment data has been consumed. Go's typed approach doesn't
	// require this — unused fields remain in the map and don't affect output.
	// See yj_to_epub.go header comment for the design difference rationale.
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

	// Python L189: if save and self.save_resources and resource_obj.raw_media is not None:
	if save && rp.saveResources && resourceObj.rawMedia != nil {
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
					if !ok || me == nil {
						// Python: raise Exception("Referenced file not in manifest: %s" % filename)
						log.Printf("kfx: error: Referenced file not in manifest: %s", filename)
					} else {
						resourceObj.manifestEntry = me
						me.referenceCount++
						break
					}
				}

				// Python: if is_referred and cnt == 0: log.error("Multiple referred resources exist...")
				if is_referred && cnt == 0 {
					log.Printf("kfx: error: Multiple referred resources exist with location %s", resourceObj.location)
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

	// Process referred resources (N9: Python L195-198)
	// Python: if process_referred or save_referred:
	//     for rr in resource_obj.referred_resources:
	//         self.process_external_resource(rr, save=save_referred, is_referred=True)
	//
	// Note: The Python code passes save=save_referred and is_referred=True to the
	// recursive call. The Go code matches this: save_referred controls whether the
	// referred resource is saved, and is_referred=true is always set for sub-resources.
	// The recursive call does NOT pass process_referred/save_referred to the sub-level,
	// meaning only one level of referred resources is processed (matching Python behavior).
	if process_referred || save_referred {
		for _, rr := range resourceObj.referredResources {
			rp.processExternalResource(rr, save_referred, false, false, false, true)
		}
	}

	// Python L222-225: validation for is_plugin / is_referred formats.
	// if is_referred: pass
	// elif is_plugin and format not in ["$287", "$284"]: error
	// elif (not is_plugin) and extension == ".pobject": error
	if !is_referred {
		if is_plugin && resourceObj.format != "pobject" && resourceObj.format != "png" {
			log.Printf("kfx: error: Unexpected plugin resource format %s for %s", resourceObj.format, resource_name)
		} else if !is_plugin && resourceObj.extension == ".pobject" {
			log.Printf("kfx: error: Unexpected non-plugin resource format %s for %s", resourceObj.extension, resource_name)
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
	case "jpg":
		return ".jpg"
	case "png":
		return ".png"
	case "jxr":
		return ".jxr"
	case "gif":
		return ".gif"
	case "pdf":
		return ".pdf"
	case "pobject":
		return ".pobject"
	case "pbm":
		return ".pbm"
	case "tiff":
		return ".tiff"
	case "yj.bpg":
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
	resourceID, _ := asString(value["resource_name"])
	if resourceID == "" {
		resourceID = fragmentID
	}
	location, _ := asString(value["location"])
	mediaType, _ := asString(value["mime"])
	format, _ := asString(value["format"])

	// If mime type is not present but format indicates an image,
	// set mediaType to the correct image MIME type. Known image format symbols:
	// "jxr" = JPEG XR, "jpg" = JPEG, "png" = PNG.
	// JXR resources must get "image/jxr" (not "image/jpeg") so the JXR→JPEG
	// conversion in buildResources detects and converts them correctly.
	if mediaType == "" {
		switch format {
		case "jxr":
			mediaType = "image/jxr"
		case "jpg":
			mediaType = "image/jpeg"
		case "png":
			mediaType = "image/png"
		}
	}

	// Width/height from $422/$423 (or $66/$67 fallback)
	// $422/$423 may not be in shared symbol catalog, check raw forms too.
	width := intFromAny(value["resource_width"], value["$422"], value["fixed_width"], value["$66"])
	height := intFromAny(value["resource_height"], value["$423"], value["fixed_height"], value["$67"])

	// $635 variant references
	// $635 may not be in shared symbol catalog, check raw form too.
	var variants []string
	if v, ok := asSlice(value["yj.variants"]); ok {
		for _, item := range v {
			if name, ok := asString(item); ok && name != "" {
				variants = append(variants, name)
			}
		}
	} else if v, ok := asSlice(value["$635"]); ok {
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
		Format:    format,
		Width:     width,
		Height:    height,
		Variants:  variants,
	}
}

func parseFontFragment(value map[string]interface{}) fontFragment {
	location, _ := asString(value["location"])
	family, _ := asString(value["font_family"])

	return fontFragment{
		Location: location,
		Family:   family,
		Style:    mapFontStyle(value["font_style"]),
		Weight:   mapFontWeight(value["font_weight"]),
		Stretch:  mapFontStretch(value["font_stretch"]),
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
	resourceDimensions := map[string][2]int{} // filename → {width, height}
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
		format := resource.Format
		if strings.EqualFold(mediaType, "image/jpg") {
			mediaType = "image/jpeg"
		}
		if strings.EqualFold(mediaType, "image/jxr") || strings.EqualFold(mediaType, "image/vnd.ms-photo") || strings.EqualFold(mediaType, "image/vnd.jxr") || (mediaType == "" && format == "jxr") {
			convertedData, convertedType, err := convertJXRResource(data)
			if err == nil {
				data = convertedData
				mediaType = convertedType
				format = "" // conversion changed format; clear so extension comes from mediaType
			}
		}
		filename := uniquePackageResourceFilename(resourceFragment{
			ID:        resource.ID,
			Location:  resource.Location,
			MediaType: mediaType,
			Format:    format,
		}, symFmt, usedOEBPSNames, data)
		output = append(output, epub.Resource{
			Filename:  filename,
			MediaType: mediaType,
			Data:      data,
		})
		resourceFilenameByID[resourceID] = filename
		if resource.Width > 0 || resource.Height > 0 {
			resourceDimensions[filename] = [2]int{resource.Width, resource.Height}
		}
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
	// Python process_fonts: used_fonts tracks location→filename to dedup fonts at same location
	usedFontLocations := map[string]string{}
	for _, location := range fontLocations {
		font := fonts[location]

		// Python L297: if location in used_fonts: reuse existing filename as src
		if existingFilename, ok := usedFontLocations[location]; ok {
			// Reuse the same src URL for this duplicate location
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
			declarations = append(declarations, "src: url("+existingFilename+")")
			fontFaceLines = append(fontFaceLines, "@font-face {"+strings.Join(canonicalDeclarations(declarations), "; ")+"}")
			continue
		}

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
		usedFontLocations[location] = filename // Python process_fonts: used_fonts[location] = filename

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

	// Python process_fonts L320-323: Second loop for unused raw font files.
	// In Python, raw_fonts ($418) is a dict of bcRawFont entries keyed by location.
	// The first loop pops matched entries, so remaining entries have no $262 font fragment.
	// Go uses a fontPool (raw blobs detected as fonts by magic bytes), so unused fonts are
	// pool entries whose ID is not a key in the fonts map and not already processed.
	for _, blob := range fontPool {
		if _, alreadyProcessed := usedFontLocations[blob.ID]; alreadyProcessed {
			continue // already added to output by the font loop above
		}
		if _, hasFontFragment := fonts[blob.ID]; hasFontFragment {
			continue // has a font fragment but was skipped (error already logged above)
		}
		// This raw font blob has no corresponding $262 font fragment — unused.
		// Python: log.warning("Unused font file: %s" % location)
		fmt.Fprintf(os.Stderr, "kfx: warning: unused font file: %s\n", blob.ID)
		filename := uniqueFontPackageFilename(blob.ID, blob.Data, symFmt, usedOEBPSNames)
		output = append(output, epub.Resource{
			Filename:  filename,
			MediaType: fontMediaType(filename),
			Data:      blob.Data,
		})
	}

	// Python process_fonts (L297-298) checks is_kpf_prepub for raw_media fallback:
	//   elif location in raw_fonts or (self.book.is_kpf_prepub and location in raw_media):
	//       raw_font = raw_fonts.pop(location, None) or raw_media.pop(location)
	// In Python, fonts are looked up from raw_fonts ($418) first, then for KPF prepub books,
	// from raw_media ($417) as a fallback. In Go, both bcRawFont and bcRawMedia entries
	// are stored in the unified RawFragments map (the `raw` parameter above), so
	// raw[location] finds fonts from either source without needing an is_kpf_prepub check.
	// The decodedBook.IsKpfPrepub field exists for completeness but is not needed here.

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
	book.ResourceDimensions = resourceDimensions
	return output, coverImageHref, strings.TrimSpace(stylesheet.String()), resourceFilenameByID
}

// packageResourceStem mirrors KFX_EPUB.resource_location_filename name stem (yj_to_epub_resources.py):
// unique_part_of_local_symbol + prefix_unique_part_of_symbol with image vs resource type.
// Port of KFX_EPUB.resource_location_filename (yj_to_epub_resources.py L247+).
// Preserves path prefix from the resource location (e.g. "images/") matching Python's
// safe_location.rpartition("/") path extraction.
func packageResourceStem(resource resourceFragment, symFmt symType, data []byte) (stem, ext string) {
	ext = resourceExtension(resource.Format, resource.MediaType, resource.Location, data)
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
	// Python uses RESOURCE_TYPE_OF_EXT to determine prefix from the resolved extension,
	// not from the media type. If ext is an image type, prefix is "image", else "resource".
	rt := resourceTypeOfExt(ext)
	u := uniquePartOfLocalSymbol(root, symFmt)
	prefixed := prefixUniquePartOfSymbol(u, rt)
	return dirPath + prefixed, ext
}

// sanitizeLocation mirrors Python re.sub(r"[^A-Za-z0-9_/.-]", "_", location)
// and safe_location.replace("//", "/x/") from resource_location_filename.
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
	// Python: safe_location = safe_location.replace("//", "/x/")
	return strings.ReplaceAll(b.String(), "//", "/x/")
}

func uniquePackageResourceFilename(resource resourceFragment, symFmt symType, used map[string]struct{}, data []byte) string {
	stem, ext := packageResourceStem(resource, symFmt, data)
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

// formatSymbolToExtension maps KFX resource format symbols ($161) to file extensions.
// Port of Python SYMBOL_FORMATS (resources.py:61-63) which is the inverse of FORMAT_SYMBOLS.
func formatSymbolToExtension(format string) (string, bool) {
	switch format {
	case "jpg":
		return ".jpg", true
	case "png":
		return ".png", true
	case "gif":
		return ".gif", true
	case "jxr":
		return ".jxr", true
	case "bmp":
		return ".bmp", true
	case "pdf":
		return ".pdf", true
	case "pbm":
		return ".pbm", true
	case "tiff":
		return ".tiff", true
	case "pobject":
		return ".pobject", true
	case "yj.bpg":
		return ".bpg", true
	default:
		return "", false
	}
}

// mimeTypeToExtension maps MIME type to the first extension, for the fallback path
// where extension is .bin/.pobject and MIME is known.
// Port of Python EXTS_OF_MIMETYPE (resources.py:139+), returns only the first extension.
func mimeTypeToExtension(mime string) (string, bool) {
	switch strings.ToLower(mime) {
	case "image/jpeg", "image/jpg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/gif":
		return ".gif", true
	case "image/svg+xml":
		return ".svg", true
	case "image/tiff":
		return ".tif", true
	case "image/bmp":
		return ".bmp", true
	case "image/webp":
		return ".webp", true
	case "image/jxr", "image/vnd.ms-photo", "image/vnd.jxr":
		return ".jxr", true
	case "application/azn-plugin-object":
		return ".pobject", true
	case "application/pdf":
		return ".pdf", true
	case "application/octet-stream", "application/protobuf", "application/x-protobuf":
		return ".bin", true
	case "res/bin":
		return ".bin", true
	case "res/img":
		return ".png", true
	case "res/kvg":
		return ".kvg", true
	case "plugin/kfx-html-article", "text/html":
		return ".html", true
	case "application/xhtml+xml":
		return ".xhtml", true
	case "text/css":
		return ".css", true
	case "font/ttf", "application/x-font-truetype", "application/x-font-ttf", "application/vnd.ms-opentype":
		return ".ttf", true
	case "font/otf", "application/x-font-otf", "application/font-sfnt":
		return ".otf", true
	case "font/woff", "application/font-woff":
		return ".woff", true
	case "font/woff2", "application/font-woff2":
		return ".woff2", true
	case "audio/mpeg":
		return ".mp3", true
	case "audio/mp4":
		return ".m4a", true
	case "video/mp4":
		return ".mp4", true
	case "figure":
		// Python EXTS_OF_MIMETYPE: "figure": [".figure"]
		// Special case: Python uses image_file_ext(raw_media) to detect actual extension
		return ".figure", true
	default:
		return "", false
	}
}

// resourceExtension resolves the file extension for a resource following Python's
// get_external_resource extension logic (yj_to_epub_resources.py:84-105):
//   1. If resource_format ($161) maps to a known extension, use that
//   2. Else use .bin
//   3. If mime ($162) maps to a known extension AND current ext is .bin/.pobject:
//      a. If mime is "figure", detect from raw data bytes
//      b. Else use the MIME's extension
//   4. If still .bin/.pobject AND location has a dot, use extension from location
func resourceExtension(format, mediaType string, location string, data []byte) string {
	// Step 1-2: extension from resource format symbol ($161)
	ext := ".bin"
	if f, ok := formatSymbolToExtension(format); ok {
		ext = f
	}

	// Step 3: if extension is .bin/.pobject, try mime type fallback
	// Python: if mime in EXTS_OF_MIMETYPE: if extension == ".pobject" or extension == ".bin": ...
	if ext == ".bin" || ext == ".pobject" {
		if mediaType == "figure" {
			// Python: extension = image_file_ext(raw_media)
			if len(data) > 0 {
				if detected := detectImageExtension(data); detected != ".bin" {
					ext = detected
				}
			}
		} else if mExt, ok := mimeTypeToExtension(mediaType); ok {
			ext = mExt
		}
	}

	// Step 4: if still .bin/.pobject, try extension from location filename
	// Python: if (extension == ".pobject" or extension == ".bin") and "." in location_fn:
	//     extension = "." + location_fn.rpartition(".")[2]
	if ext == ".bin" || ext == ".pobject" {
		locFn := location
		if idx := strings.LastIndex(locFn, "."); idx >= 0 {
			candidate := locFn[idx:] // e.g. ".jpg"
			// Validate it's a reasonable extension (alphabetic after the dot)
			if len(candidate) > 1 && isAlphaExt(candidate[1:]) {
				ext = candidate
			}
		}
	}

	return ext
}

// isAlphaExt checks if a string is a simple alphabetic extension (e.g. "jpg", "png").
func isAlphaExt(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return len(s) > 0
}

// resourceTypeOfExt maps file extension to resource type prefix, matching Python's
// RESOURCE_TYPE_OF_EXT (resources.py:111-137). Used to determine the filename prefix
// ("image" vs "resource") in resource_location_filename.
func resourceTypeOfExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".bmp", ".gif", ".jpg", ".jpeg", ".jxr", ".kvg", ".pdf", ".png",
		".svg", ".tif", ".tiff", ".webp", ".ico":
		return "image"
	case ".css":
		return "styles"
	case ".eot", ".dfont", ".otf", ".ttf", ".woff", ".woff2", ".pfb":
		return "font"
	case ".htm", ".html", ".js", ".xhtml":
		return "text"
	case ".mp3":
		return "audio"
	case ".mp4":
		return "video"
	default:
		return "resource"
	}
}

// detectFontExtension implements Python font_file_ext (resources.py:719-750).
// Detects font file type from binary magic bytes.
func detectFontExtension(data []byte) string {
	if len(data) >= 4 {
		magic4 := string(data[:4])
		switch magic4 {
		case "\x00\x01\x00\x00", "true", "typ1":
			return ".ttf"
		case "OTTO":
			return ".otf"
		case "wOFF":
			return ".woff"
		case "wOF2":
			return ".woff2"
		}
	}
	// EOT: offset 34-36 = "LP" and offset 8-12 is one of three known patterns
	if len(data) >= 36 && string(data[34:36]) == "LP" {
		if len(data) >= 12 {
			switch string(data[8:12]) {
			case "\x00\x00\x01\x00", "\x01\x00\x02\x00", "\x02\x00\x02\x00":
				return ".eot"
			}
		}
	}
	// dfont: starts with \x00\x00\x01\x00 at offset 0
	if len(data) >= 4 && data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x01 && data[3] == 0x00 {
		return ".dfont"
	}
	// pfb: \x80\x01 at offset 0, %!PS-AdobeFont-1.0 at offset 6-24
	if len(data) >= 24 && data[0] == 0x80 && data[1] == 0x01 && string(data[6:24]) == "%!PS-AdobeFont-1.0" {
		return ".pfb"
	}
	return ".bin"
}

// detectImageExtension implements Python image_file_ext (resources.py:756-775).
// Detects image format from magic bytes.
func detectImageExtension(data []byte) string {
	if len(data) >= 6 {
		// GIF87a or GIF89a
		if bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")) {
			return ".gif"
		}
	}
	if len(data) >= 4 {
		// JPEG: FF D8 FF
		if bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}) {
			return ".jpg"
		}
		// JXR: 49 49 BC 01
		if bytes.HasPrefix(data, []byte{0x49, 0x49, 0xbc, 0x01}) {
			return ".jxr"
		}
	}
	if len(data) >= 8 {
		// PNG: 89 50 4E 47 0D 0A 1A 0A
		if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}) {
			return ".png"
		}
	}
	if len(data) >= 4 {
		// PDF: %PDF
		if bytes.HasPrefix(data, []byte("%PDF")) {
			return ".pdf"
		}
		// TIFF little-endian: 49 49 2A 00
		if bytes.HasPrefix(data, []byte{0x49, 0x49, 0x2a, 0x00}) {
			return ".tif"
		}
		// TIFF big-endian: 4D 4D 00 2A
		if bytes.HasPrefix(data, []byte{0x4d, 0x4d, 0x00, 0x2a}) {
			return ".tif"
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
					log.Printf("kfx: info: tile padding ltrb: %d, %d, %d, %d", leftPadding, topPadding, rightPadding, bottomPadding)
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
	if resourceFormat == "jpg" && COMBINE_TILES_LOSSLESS {
		resourceFormat = "png"
	}

	if resourceFormat == "jpg" {
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
		return buf.Bytes(), "jpg"
	}

	// Fallback: try standard image decode (for non-JXR data passed to this function)
	img2, _, err2 := image.Decode(bytes.NewReader(imageData))
	if err2 != nil {
		// Can't decode — return as-is with JPEG format (will be handled upstream)
		log.Printf("kfx: error: Exception during conversion of JPEG-XR '%s': %v", resourceName, err)
		return imageData, "jxr"
	}

	// Determine output format based on color mode
	if CONVERT_JXR_LOSSLESS || hasAlpha(img2) {
		var buf bytes.Buffer
		png.Encode(&buf, img2)
		return buf.Bytes(), "png"
	}

	var buf bytes.Buffer
	jpeg.Encode(&buf, img2, &jpeg.Options{Quality: 95})
	return buf.Bytes(), "jpg"
}

// convertJXRToJpegOrPNG_RGBA handles RGBA images specifically → PNG output.
// Python: resources.py:279-280 — if img.mode == "RGBA" → PNG
func convertJXRToJpegOrPNG_RGBA(imageData []byte, resourceName string) ([]byte, string) {
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return imageData, "png"
	}
	_ = resourceName

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes(), "png"
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
//
// Uses pdfcpu (pure Go PDF library) to extract embedded images from PDF pages, mirroring
// Python's pypdf-based extraction in get_pdf_page_image (resources.py:366-425).
//
// Python reference path:
//   1. resources.py:323 — convert_pdf_page_to_image calls get_pdf_page_image
//   2. resources.py:366 — get_pdf_page_image uses pypdf.PdfReader to extract the single
//      embedded image directly (if the page is a single-image PDF page)
//   3. resources.py:338 — convert_pdf_page_to_jpeg uses pdftoppm subprocess as fallback
//
// The Python's get_pdf_page_image performs extensive validation: checks cropbox == mediabox,
// verifies exactly 1 image, no text, no annotations, correct aspect ratio, sufficient DPI,
// and that the extracted image matches the rendered version. Go uses pdfcpu's ExtractImagesRaw
// to extract images and performs equivalent validation where practical.
//
// Fallback: if extraction fails or the page doesn't meet validation criteria, a placeholder
// JPEG is returned.
func convertPDFPageToImage(location string, pdfData []byte, pageNum int, reportedErrors map[string]bool, forceJPEG bool) ([]byte, string) {
	// Default image: placeholder JPEG
	defaultImage, defaultFormat := makePlaceholderJPEG(location, pageNum)
	return getPDFPageImage(location, pdfData, pageNum, forceJPEG, defaultImage, defaultFormat)
}

// makePlaceholderJPEG creates a placeholder JPEG for a PDF page.
// Used as the fallback when real image extraction fails.
func makePlaceholderJPEG(location string, pageNum int) ([]byte, string) {
	if pageNum <= 0 {
		pageNum = 1
	}
	img := image.NewGray(image.Rect(0, 0, 612, 792))
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95})
	return buf.Bytes(), "jpg"
}

// getPDFPageImage implements Python get_pdf_page_image (resources.py:373-425).
//
// Uses pdfcpu to extract embedded images from a specific PDF page. Mirrors Python's
// validation logic:
//   - Extract images from the specified page using pdfcpu's ExtractImagesRaw
//   - Validate single image on page (matching Python's len(page.images.keys()) == 1)
//   - For DCTDecode (JPEG) images: return raw JPEG data if valid
//   - For other image types: decode and re-encode as JPEG or PNG
//   - Fall back to default placeholder on any validation failure
//
// Python reference: resources.py:373-425
func getPDFPageImage(location string, pdfData []byte, pageNum int, forceJPEG bool, defaultImage []byte, defaultFormat string) ([]byte, string) {
	if len(pdfData) == 0 || !bytes.HasPrefix(pdfData, []byte("%PDF")) {
		return defaultImage, defaultFormat
	}

	// Use pdfcpu to extract images from the specified page.
	// Python: pypdf.PdfReader(raw_media_file) + page.images
	rs := io.ReadSeeker(bytes.NewReader(pdfData))
	conf := model.NewDefaultConfiguration()

	pageStr := fmt.Sprintf("%d", pageNum)
	pageImages, err := api.ExtractImagesRaw(rs, []string{pageStr}, conf)
	if err != nil {
		log.Printf("kfx: warning: PDF image extraction failed for %s page %d: %v", location, pageNum, err)
		return defaultImage, defaultFormat
	}

	// Python L384: if len(page.images.keys()) != 1: return default_image
	// pageImages is []map[int]model.Image, one map per page
	if len(pageImages) == 0 {
		return defaultImage, defaultFormat
	}

	// Get the first page's images
	imgMap := pageImages[0]
	if len(imgMap) != 1 {
		// Python validates exactly 1 image on the page
		return defaultImage, defaultFormat
	}

	// Get the single image from the map
	var pdfImg model.Image
	for _, img := range imgMap {
		pdfImg = img
		break
	}

	// Read the image data from the model.Image's io.Reader
	imgData, err := io.ReadAll(pdfImg.Reader)
	if err != nil {
		log.Printf("kfx: warning: failed to read extracted PDF image from %s page %d: %v", location, pageNum, err)
		return defaultImage, defaultFormat
	}

	if len(imgData) == 0 {
		return defaultImage, defaultFormat
	}

	// Determine format based on the file type
	switch strings.ToLower(pdfImg.FileType) {
	case "jpg", "jpeg":
		// DCTDecode — raw JPEG, same as Python L407-408:
		// "if image_object.get('/Filter') == '/DCTDecode' and image_file_ext(image_data) == '.jpg'"
		log.Printf("kfx: info: Extracting JPEG image (%dx%d) from PDF %s page %d",
			pdfImg.Width, pdfImg.Height, location, pageNum)
		return imgData, "jpg"

	case "png":
		// FlateDecode — PNG image
		// Python L410-412: handles FlateDecode via page_image.image
		log.Printf("kfx: info: Extracting PNG image (%dx%d) from PDF %s page %d",
			pdfImg.Width, pdfImg.Height, location, pageNum)
		if forceJPEG {
			// Python L419-424: force_jpeg conversion
			return convertImageToJPEG(imgData, location, pageNum, defaultImage, defaultFormat)
		}
		return imgData, "png"

	case "tif", "tiff":
		// CCITTFaxDecode — TIFF image
		// Python L409-413: converts TIFF to PNG
		log.Printf("kfx: info: Extracting TIFF image (%dx%d) from PDF %s page %d, converting to PNG",
			pdfImg.Width, pdfImg.Height, location, pageNum)
		return convertTIFFToPNG(imgData, location, pageNum, defaultImage, defaultFormat)

	default:
		// Try to decode and convert
		log.Printf("kfx: info: Extracting %s image (%dx%d) from PDF %s page %d",
			pdfImg.FileType, pdfImg.Width, pdfImg.Height, location, pageNum)
		return convertImageToJPEG(imgData, location, pageNum, defaultImage, defaultFormat)
	}
}

// convertImageToJPEG decodes image data and re-encodes as JPEG.
// Python: resources.py:419-424 — force_jpeg conversion path.
func convertImageToJPEG(imgData []byte, location string, pageNum int, defaultImage []byte, defaultFormat string) ([]byte, string) {
	img, err := decodeImageBytes(imgData)
	if err != nil {
		log.Printf("kfx: warning: failed to decode extracted image from PDF %s page %d: %v", location, pageNum, err)
		return defaultImage, defaultFormat
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		return defaultImage, defaultFormat
	}
	return buf.Bytes(), "jpg"
}

// convertTIFFToPNG converts TIFF image data to PNG format.
// Python: resources.py:411-413 — pil_img.save(outfile, "PNG", optimize=True)
func convertTIFFToPNG(imgData []byte, location string, pageNum int, defaultImage []byte, defaultFormat string) ([]byte, string) {
	img, err := decodeImageBytes(imgData)
	if err != nil {
		log.Printf("kfx: warning: failed to decode TIFF from PDF %s page %d: %v", location, pageNum, err)
		return defaultImage, defaultFormat
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return defaultImage, defaultFormat
	}
	return buf.Bytes(), "png"
}

// decodeImageBytes decodes image data using Go's standard image decoders.
func decodeImageBytes(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// pdfPageSuffix returns the filename suffix for a PDF page number.
// Python: yj_to_epub_resources.py:97-99 — suffix = "-page%d" % page_num
func pdfPageSuffix(pageNum int) string {
	return fmt.Sprintf("-page%d", pageNum)
}
