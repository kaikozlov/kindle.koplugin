package kfx

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
	"github.com/kaikozlov/kindle-koplugin/internal/jxr"
)

func mapFontStyle(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$382":
		return "italic"
	case "$381":
		return "oblique"
	case "$350":
		return "normal"
	default:
		return ""
	}
}

func mapFontWeight(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$361":
		return "bold"
	case "$363":
		return "900"
	case "$357":
		return "300"
	case "$359":
		return "500"
	case "$350":
		return "normal"
	case "$360":
		return "600"
	case "$355":
		return "100"
	case "$362":
		return "800"
	case "$356":
		return "200"
	default:
		return ""
	}
}

func mapFontStretch(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$365":
		return "condensed"
	case "$368":
		return "expanded"
	case "$350":
		return "normal"
	case "$366":
		return "semi-condensed"
	case "$367":
		return "semi-expanded"
	default:
		return ""
	}
}
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
