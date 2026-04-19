package kfx

// ---------------------------------------------------------------------------
// Port of Python yj_to_image_book.py: CBZ/PDF image book output.
//
// Python reference: REFERENCE/Calibre_KFX_Input/kfxlib/yj_to_image_book.py (353 lines)
// Go file: internal/kfx/yj_to_image_book.go
//
// Key types:
//   - ImageResource: image data with format, location, dimensions
//   - OutlineEntry: recursive PDF bookmark structure
//   - KFXImageBook: book-level operations for CBZ/PDF conversion
//
// Key functions:
//   - combineImagesIntoCBZ: create CBZ ZIP archive from images
//   - combineImagesIntoPDF: create PDF from images
//   - addPDFOutline: add recursive bookmarks to PDF
//   - suffixLocation: insert suffix before file extension
// ---------------------------------------------------------------------------

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"strings"
)

// Note: USE_HIGHEST_RESOLUTION_IMAGE_VARIANT is declared in yj_to_epub_resources.go.

// ---------------------------------------------------------------------------
// ImageResource — port of Python ImageResource/PdfImageResource
// ---------------------------------------------------------------------------

// ImageResource represents an image to be included in a CBZ or PDF.
// Port of Python resources.ImageResource and PdfImageResource.
type ImageResource struct {
	Format     string // YJ format symbol (e.g. "$284" for PNG, "$285" for JPG)
	Location   string // resource location path
	RawMedia   []byte // raw image data
	Height     int    // image height in pixels
	Width      int    // image width in pixels
	PageNums   []int  // for PDF resources: page numbers to include
	TotalPages int    // total pages in PDF resource
}

// OutlineEntry represents a PDF bookmark/outline item with optional children.
// Port of Python's outline_entries used in add_pdf_outline (line 296-302).
type OutlineEntry struct {
	Title    string
	PageNum  int
	Children []OutlineEntry
}

// ---------------------------------------------------------------------------
// KFXImageBook — port of Python KFX_IMAGE_BOOK class
// ---------------------------------------------------------------------------

// KFXImageBook wraps a book's fragments for CBZ/PDF image conversion.
// Port of Python KFX_IMAGE_BOOK (yj_to_image_book.py L18-213).
type KFXImageBook struct {
	fragments             fragmentCatalog
	orderedImageResources []string // pre-computed ordered image resource IDs
}

// NewKFXImageBook creates a KFXImageBook from a fragment catalog and ordered image list.
func NewKFXImageBook(frags fragmentCatalog, orderedImageResources []string) *KFXImageBook {
	return &KFXImageBook{
		fragments:             frags,
		orderedImageResources: orderedImageResources,
	}
}

// getOrderedImages returns the book's images in reading order.
// Port of Python KFX_IMAGE_BOOK.get_ordered_images (L101-155).
// splitLandscape corresponds to Python's split_landscape_comic_images.
// isComic corresponds to Python's is_comic.
// isRTL corresponds to Python's is_rtl.
func (b *KFXImageBook) getOrderedImages(splitLandscape, isComic, isRTL bool) []ImageResource {
	var orderedImages []ImageResource
	splitImageCount := 0

	for _, fid := range b.orderedImageResources {
		imgRes := b.getResourceImage(fid, false)
		if imgRes == nil {
			continue
		}

		// Split landscape comic images
		if splitLandscape && isComic && imgRes.Format != "$565" && imgRes.Width > imgRes.Height {
			splitImageCount++
			newWidth := imgRes.Width / 2

			// Crop left half
			leftData := cropImage(imgRes.RawMedia, imgRes.Width, imgRes.Height, 0, newWidth, 0, 0)
			left := ImageResource{
				Format:   imgRes.Format,
				Location: suffixLocation(imgRes.Location, "-L"),
				RawMedia: leftData,
				Height:   imgRes.Height,
				Width:    newWidth,
			}

			// Crop right half
			rightData := cropImage(imgRes.RawMedia, imgRes.Width, imgRes.Height, newWidth, 0, 0, 0)
			right := ImageResource{
				Format:   imgRes.Format,
				Location: suffixLocation(imgRes.Location, "-R"),
				RawMedia: rightData,
				Height:   imgRes.Height,
				Width:    newWidth,
			}

			if !isRTL {
				orderedImages = append(orderedImages, left)
				imgRes = &right
			} else {
				orderedImages = append(orderedImages, right)
				imgRes = &left
			}
		}

		orderedImages = append(orderedImages, *imgRes)
	}

	if splitImageCount > 0 {
		log.Printf("kfx: warning: Split %d landscape comic images into left/right image pairs", splitImageCount)
	}

	return orderedImages
}

// getResourceImage retrieves an ImageResource from the fragment catalog.
// Port of Python KFX_IMAGE_BOOK.get_resource_image (L157-213).
func (b *KFXImageBook) getResourceImage(resourceID string, ignoreVariants bool) *ImageResource {
	resData, ok := b.fragments.ResourceRawData[resourceID]
	if !ok {
		return nil
	}

	resourceFormat, _ := asString(resData["$161"])
	resourceHeight := intFromVal(resData["$423"], resData["$67"])
	resourceWidth := intFromVal(resData["$422"], resData["$66"])

	var location string
	var rawMedia []byte

	// Check for tiles ($636)
	if _, hasTiles := resData["$636"]; hasTiles {
		// Tile handling: simplified version — extract location from first tile
		tilesSlice, ok := asSlice(resData["$636"])
		if !ok || len(tilesSlice) == 0 {
			return nil
		}
		firstRow, ok := asSlice(tilesSlice[0])
		if !ok || len(firstRow) == 0 {
			return nil
		}
		tileLoc, _ := asString(firstRow[0])
		if tileLoc == "" {
			return nil
		}
		// Strip "-tile" suffix for base location
		if idx := strings.Index(tileLoc, "-tile"); idx >= 0 {
			location = tileLoc[:idx]
		} else {
			location = tileLoc
		}
		// For simplicity, just use the raw media from the first tile location
		rawMedia = b.fragments.RawFragments[tileLoc]
	} else {
		// Direct resource: get location from $165
		locVal, _ := asString(resData["$165"])
		location = locVal
		if location != "" {
			rawMedia = b.fragments.RawFragments[location]
		}
	}

	// Variant handling: try higher resolution variants
	if resourceFormat != "$565" && !ignoreVariants {
		variants, _ := asSlice(resData["$635"])
		for _, vr := range variants {
			variantName, _ := asString(vr)
			if variantName == "" {
				continue
			}
			variant := b.getResourceImage(variantName, true)
			if USE_HIGHEST_RESOLUTION_IMAGE_VARIANT && variant != nil &&
				variant.Width > resourceWidth && variant.Height > resourceHeight {
				location = variant.Location
				resourceFormat = variant.Format
				rawMedia = variant.RawMedia
				resourceWidth = variant.Width
				resourceHeight = variant.Height
			}
		}
	}

	if rawMedia == nil {
		return nil
	}

	if resourceFormat == "$565" {
		pageIndex := intFromVal(resData["$564"])
		return &ImageResource{
			Format:   "$565",
			Location: location,
			RawMedia: rawMedia,
			PageNums: []int{pageIndex},
		}
	}

	return &ImageResource{
		Format:   resourceFormat,
		Location: location,
		RawMedia: rawMedia,
		Height:   resourceHeight,
		Width:    resourceWidth,
	}
}

// intFromVal returns the first non-zero int from a list of interface{} values.
func intFromVal(vals ...interface{}) int {
	for _, v := range vals {
		if i, ok := asInt(v); ok && i != 0 {
			return i
		}
	}
	return 0
}

// cropImage crops an image to the specified region.
// Parameters: offLeft, cropWidth (0=full width), offTop (0=from top), offBottom (pixels from bottom to remove).
// Port of Python resources.crop_image.
func cropImage(data []byte, width, height, offLeft, cropWidth, offTop, offBottom int) []byte {
	// Decode the image
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		log.Printf("kfx: error: cropImage failed to decode: %v", err)
		return data
	}

	// Compute crop rectangle
	cropRight := width
	if cropWidth > 0 {
		cropRight = offLeft + cropWidth
	}
	cropBottom := height - offBottom
	if offTop > 0 {
		cropBottom = offTop
	}

	// Clamp bounds
	bounds := image.Rect(offLeft, 0, cropRight, cropBottom)
	subImg := img.(interface {
		SubImage(r image.Rectangle) image.Image
	}).SubImage(bounds)

	// Re-encode in the same format
	var buf bytes.Buffer
	switch format {
	case "png":
		png.Encode(&buf, subImg)
	default:
		jpeg.Encode(&buf, subImg, &jpeg.Options{Quality: 95})
	}
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// suffixLocation — port of Python suffix_location (yj_to_image_book.py L349-353)
// ---------------------------------------------------------------------------

// suffixLocation inserts suffix before the first dot in location.
// If no dot exists, appends suffix.
func suffixLocation(location, suffix string) string {
	if strings.Contains(location, ".") {
		return strings.Replace(location, ".", suffix+".", 1)
	}
	return location + suffix
}

// ---------------------------------------------------------------------------
// combineImagesIntoCBZ — port of Python combine_images_into_cbz (L304-347)
// ---------------------------------------------------------------------------

// combineImagesIntoCBZ creates a CBZ (ZIP) archive from ordered images.
// Metadata is serialized as compact JSON in the ZIP comment (if ≤65535 bytes).
func combineImagesIntoCBZ(orderedImages []ImageResource, metadata interface{}) []byte {
	if len(orderedImages) == 0 {
		return nil
	}

	// Track image formats for logging
	imageResourceFormats := map[string]map[string]bool{}
	pageImages := []ImageResource{}

	for _, imgRes := range orderedImages {
		ext := extensionForFormat(imgRes.Format)
		fmtUpper := strings.ToUpper(strings.TrimPrefix(ext, "."))
		if imageResourceFormats[fmtUpper] == nil {
			imageResourceFormats[fmtUpper] = map[string]bool{}
		}
		imageResourceFormats[fmtUpper][imgRes.Location] = true

		switch imgRes.Format {
		case "$286", "$285", "$284": // GIF, JPG, PNG — direct
			pageImages = append(pageImages, imgRes)
		case "$565": // PDF — would need convert_pdf_page_to_image
			// For the port, we skip PDF pages in CBZ output (simplified)
			log.Printf("kfx: warning: PDF resource %s skipped in CBZ output", imgRes.Location)
		case "$548": // JXR — would need convert_jxr_to_jpeg_or_png
			log.Printf("kfx: warning: JXR resource %s skipped in CBZ output", imgRes.Location)
		default:
			log.Printf("kfx: error: Unexpected image format: %s", imgRes.Format)
		}
	}

	// Build ZIP
	cbzFile := &bytes.Buffer{}
	zw := zip.NewWriter(cbzFile)

	for i, imgRes := range pageImages {
		ext := extensionForFormat(imgRes.Format)
		name := fmt.Sprintf("%04d%s", i+1, ext)
		w, err := zw.Create(name)
		if err != nil {
			log.Printf("kfx: error: Failed to create CBZ entry %s: %v", name, err)
			continue
		}
		w.Write(imgRes.RawMedia)
	}

	// Store metadata as JSON in ZIP comment
	if metadata != nil {
		comment, err := json.Marshal(metadata)
		if err == nil && len(comment) <= 65535 {
			zw.SetComment(string(comment))
		} else if len(comment) > 65535 {
			log.Printf("kfx: warning: Discarding CBZ metadata -- too long for ZIP comment")
		}
	}

	if err := zw.Close(); err != nil {
		log.Printf("kfx: error: Failed to close CBZ: %v", err)
		return nil
	}

	// Build format summary for logging
	var formatParts []string
	for fmtName, locs := range imageResourceFormats {
		formatParts = append(formatParts, fmt.Sprintf("%d %s", len(locs), fmtName))
	}

	log.Printf("kfx: info: Combined %s resources into a %d page CBZ file",
		strings.Join(formatParts, ", "), len(orderedImages))

	return cbzFile.Bytes()
}

// extensionForFormat returns the file extension for a YJ format symbol.
// Port of Python's SYMBOL_FORMATS lookup.
func extensionForFormat(formatSymbol string) string {
	if ext, ok := symbolFormats[formatSymbol]; ok {
		return "." + ext
	}
	return ".bin"
}

// ---------------------------------------------------------------------------
// combineImagesIntoPDF — port of Python combine_images_into_pdf (L215-294)
// ---------------------------------------------------------------------------

// combineImagesIntoPDF creates a PDF document from ordered images.
// Supports JPEG and PNG image resources. Metadata and outline are optional.
func combineImagesIntoPDF(orderedImages []ImageResource, metadata map[string]string, isRTL bool, outline []OutlineEntry) []byte {
	if len(orderedImages) == 0 {
		return nil
	}

	// Convert each image to a PDF page
	var pages []pdfPage
	for _, imgRes := range orderedImages {
		if imgRes.Format == "$565" {
			// PDF resource — for now, skip (would need pypdf-like page extraction)
			log.Printf("kfx: warning: PDF resource %s skipped in PDF output", imgRes.Location)
			continue
		}

		// Decode image to get dimensions
		cfg, _, err := image.DecodeConfig(bytes.NewReader(imgRes.RawMedia))
		if err != nil {
			log.Printf("kfx: error: Failed to decode image %s: %v", imgRes.Location, err)
			continue
		}

		// Re-encode as JPEG for PDF embedding (or use raw if already JPEG)
		var jpegData []byte
		var isJPEG bool
		if imgRes.Format == "$285" {
			jpegData = imgRes.RawMedia
			isJPEG = true
		} else {
			// Convert to JPEG for PDF
			img, _, err := image.Decode(bytes.NewReader(imgRes.RawMedia))
			if err != nil {
				log.Printf("kfx: error: Failed to decode image for conversion %s: %v", imgRes.Location, err)
				continue
			}
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
				log.Printf("kfx: error: Failed to convert image to JPEG %s: %v", imgRes.Location, err)
				continue
			}
			jpegData = buf.Bytes()
			isJPEG = false
		}

		pages = append(pages, pdfPage{
			imageData: jpegData,
			isJPEG:    isJPEG,
			width:     float64(cfg.Width),
			height:    float64(cfg.Height),
		})
	}

	if len(pages) == 0 {
		return nil
	}

	return buildPDF(pages, metadata, isRTL, outline)
}

// ---------------------------------------------------------------------------
// Minimal PDF writer
// ---------------------------------------------------------------------------

type pdfPage struct {
	imageData []byte
	isJPEG    bool
	width     float64
	height    float64
}

// pdfObject represents a PDF indirect object.
type pdfObject struct {
	id      int
	content string
	stream  []byte
}

// buildPDF creates a minimal PDF from pages with optional metadata, RTL, and outline.
func buildPDF(pages []pdfPage, metadata map[string]string, isRTL bool, outline []OutlineEntry) []byte {
	var objects []pdfObject
	objID := 1
	nextID := func() int { id := objID; objID++; return id }

	// Catalog
	catalogID := nextID()
	// Pages
	pagesID := nextID()

	// Outline root (if needed)
	var outlineRootID int
	if len(outline) > 0 {
		outlineRootID = nextID()
	}

	// Create page objects
	var pageIDs []int
	var imageObjIDs []int

	for i, page := range pages {
		pageID := nextID()
		imageObjID := nextID()

		// Image resource
		colorSpace := "/DeviceRGB"
		filter := "/DCTDecode" // JPEG
		if !page.isJPEG {
			// We re-encode non-JPEG as JPEG above, so this is always JPEG
			filter = "/DCTDecode"
		}

		imageObj := pdfObject{
			id: imageObjID,
			content: fmt.Sprintf(
				"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent 8 /Filter %s /Length %d >>",
				int(page.width), int(page.height), colorSpace, filter, len(page.imageData),
			),
			stream: page.imageData,
		}
		objects = append(objects, imageObj)
		imageObjIDs = append(imageObjIDs, imageObjID)

		// Content stream: draw image filling the page
		contentStream := fmt.Sprintf("q %.2f 0 0 %.2f 0 0 cm /Img%d Do Q", page.width, page.height, i+1)
		contentID := nextID()
		objects = append(objects, pdfObject{
			id:      contentID,
			content: fmt.Sprintf("<< /Length %d >>", len(contentStream)),
			stream:  []byte(contentStream),
		})

		// Page resources
		resources := fmt.Sprintf("<< /XObject << /Img%d %d 0 R >> >>", i+1, imageObjID)

		// Page object (will be finalized after outline is built)
		_ = pageID
		_ = resources
		_ = contentID
		pageIDs = append(pageIDs, pageID)
	}

	// Now build the actual objects in order
	// Reset and rebuild properly
	objID = 1
	objects = objects[:0]

	// 1. Catalog
	catalogObj := "<< /Type /Catalog /Pages " + fmt.Sprintf("%d 0 R", pagesID)
	if outlineRootID > 0 {
		catalogObj += fmt.Sprintf(" /Outlines %d 0 R /PageMode /UseOutlines", outlineRootID)
	}
	if isRTL {
		catalogObj += " /ViewerPreferences << /Direction /R2L >>"
	}
	catalogObj += " >>"
	objects = append(objects, pdfObject{id: catalogID, content: catalogObj})

	// 2. Pages (placeholder — will be updated with page refs)
	pagesObj := pdfObject{id: pagesID}
	objects = append(objects, pagesObj)

	// 3. Outline root (placeholder)
	if outlineRootID > 0 {
		objects = append(objects, pdfObject{id: outlineRootID, content: "<< /Type /Outlines >>"})
	}

	// 4. Per-page objects
	var contentObjIDs []int
	for i, page := range pages {
		imageObjID := nextID()
		colorSpace := "/DeviceRGB"
		filter := "/DCTDecode"

		objects = append(objects, pdfObject{
			id: imageObjID,
			content: fmt.Sprintf(
				"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent 8 /Filter %s /Length %d >>",
				int(page.width), int(page.height), colorSpace, filter, len(page.imageData),
			),
			stream: page.imageData,
		})

		contentStream := fmt.Sprintf("q %.2f 0 0 %.2f 0 0 cm /Img%d Do Q", page.width, page.height, i+1)
		contentID := nextID()
		objects = append(objects, pdfObject{
			id:      contentID,
			content: fmt.Sprintf("<< /Length %d >>", len(contentStream)),
			stream:  []byte(contentStream),
		})
		contentObjIDs = append(contentObjIDs, contentID)

		resources := fmt.Sprintf("<< /XObject << /Img%d %d 0 R >> >>", i+1, imageObjID)
		pageObj := fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %.2f %.2f] /Contents %d 0 R /Resources %s >>",
			pagesID, page.width, page.height, contentID, resources)

		objects = append(objects, pdfObject{id: pageIDs[i], content: pageObj})
	}

	// 5. Build outline entries if provided
	if outlineRootID > 0 {
		outlineObjs := buildOutlineObjects(outline, outlineRootID, pageIDs, &objID, &objects)
		// Update outline root
		firstID := 0
		lastID := 0
		if len(outlineObjs) > 0 {
			firstID = outlineObjs[0]
			lastID = outlineObjs[len(outlineObjs)-1]
			// Actually, first and last are the first/last top-level entries
			firstID = outlineObjs[0]
			lastID = outlineObjs[0]
			// Find last top-level entry
			count := 0
			for _, oe := range outline {
				_ = oe
				count++
			}
			if len(outlineObjs) >= count {
				lastID = outlineObjs[count-1]
			}
		}
		objects[2] = pdfObject{
			id: outlineRootID,
			content: fmt.Sprintf("<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>",
				firstID, lastID, countTotalOutlineEntries(outline)),
		}
	}

	// Update Pages object with all page refs
	var pageRefs []string
	for _, pid := range pageIDs {
		pageRefs = append(pageRefs, fmt.Sprintf("%d 0 R", pid))
	}
	objects[1] = pdfObject{
		id:      pagesID,
		content: fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(pageRefs, " "), len(pageIDs)),
	}

	// Build metadata info dict if provided
	var infoID int
	if len(metadata) > 0 {
		infoID = nextID()
		var metaParts []string
		for k, v := range metadata {
			if v != "" {
				// Escape PDF string
				escaped := strings.ReplaceAll(v, "\\", "\\\\")
				escaped = strings.ReplaceAll(escaped, "(", "\\(")
				escaped = strings.ReplaceAll(escaped, ")", "\\)")
				metaParts = append(metaParts, fmt.Sprintf("%s (%s)", k, escaped))
			}
		}
		objects = append(objects, pdfObject{
			id:      infoID,
			content: fmt.Sprintf("<< %s >>", strings.Join(metaParts, " ")),
		})
	}

	// Serialize PDF
	return serializePDF(objects, catalogID, infoID)
}

// buildOutlineObjects creates PDF outline entry objects and returns their IDs.
// Returns a flat list of all outline object IDs created.
func buildOutlineObjects(entries []OutlineEntry, parentID int, pageIDs []int, nextObjID *int, objects *[]pdfObject) []int {
	var allIDs []int
	var entryObjIDs []int

	for i, entry := range entries {
		objID := *nextObjID
		*nextObjID++

		// Determine page reference
		var dest string
		if entry.PageNum >= 0 && entry.PageNum < len(pageIDs) {
			dest = fmt.Sprintf("[%d 0 R /Fit]", pageIDs[entry.PageNum])
		} else if entry.PageNum == 0 && len(pageIDs) > 0 {
			dest = fmt.Sprintf("[%d 0 R /Fit]", pageIDs[0])
		}

		// Process children first
		var childIDs []int
		if len(entry.Children) > 0 {
			childIDs = buildOutlineObjects(entry.Children, objID, pageIDs, nextObjID, objects)
		}

		// Build object content
		parts := []string{
			fmt.Sprintf("/Title (%s)", escapePDFString(entry.Title)),
			fmt.Sprintf("/Parent %d 0 R", parentID),
		}

		if len(dest) > 0 {
			parts = append(parts, fmt.Sprintf("/Dest %s", dest))
		}

		if len(childIDs) > 0 {
			parts = append(parts, fmt.Sprintf("/First %d 0 R", childIDs[0]))
			parts = append(parts, fmt.Sprintf("/Last %d 0 R", childIDs[len(childIDs)-1]))
			parts = append(parts, fmt.Sprintf("/Count %d", len(childIDs)))
		}

		// Sibling links
		if i > 0 {
			parts = append(parts, fmt.Sprintf("/Prev %d 0 R", entryObjIDs[len(entryObjIDs)-1]))
		}
		if i < len(entries)-1 {
			// Next sibling will be the next iteration's object
			// We need to know its ID. Since we're creating sequentially, it's objID+1+childCount
			// This is tricky — let's use a two-pass approach
			// For now, we'll set Prev/Next after all objects are created
		}

		obj := pdfObject{
			id:      objID,
			content: fmt.Sprintf("<< %s >>", strings.Join(parts, " ")),
		}
		*objects = append(*objects, obj)
		entryObjIDs = append(entryObjIDs, objID)
		allIDs = append(allIDs, objID)
		allIDs = append(allIDs, childIDs...)
	}

	// Set sibling links
	for i := 1; i < len(entryObjIDs); i++ {
		// Find the object with entryObjIDs[i] and set Prev
		setOutlineField(*objects, entryObjIDs[i], "/Prev", fmt.Sprintf("%d 0 R", entryObjIDs[i-1]))
	}
	for i := 0; i < len(entryObjIDs)-1; i++ {
		setOutlineField(*objects, entryObjIDs[i], "/Next", fmt.Sprintf("%d 0 R", entryObjIDs[i+1]))
	}

	return entryObjIDs
}

// setOutlineField modifies an outline object's content to add a field.
func setOutlineField(objects []pdfObject, objID int, field, value string) {
	for i, obj := range objects {
		if obj.id == objID {
			// Insert the field before the closing >>
			content := objects[i].content
			content = strings.TrimSuffix(content, " >>")
			content += fmt.Sprintf(" %s %s >>", field, value)
			objects[i].content = content
			return
		}
	}
}

// escapePDFString escapes special characters in PDF string literals.
func escapePDFString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	return s
}

// countTotalOutlineEntries counts all outline entries recursively.
func countTotalOutlineEntries(entries []OutlineEntry) int {
	count := 0
	for _, e := range entries {
		count++
		count += countTotalOutlineEntries(e.Children)
	}
	return count
}

// serializePDF writes a minimal PDF from objects.
func serializePDF(objects []pdfObject, catalogID, infoID int) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	// Binary comment to mark as binary PDF
	buf.WriteString("%\xe2\xe3\xcf\xd3\n")

	// Object positions for xref
	offsets := make([]int64, len(objects)+1)

	for _, obj := range objects {
		offsets[obj.id] = int64(buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\n", obj.id, obj.content)
		if len(obj.stream) > 0 {
			fmt.Fprintf(&buf, "stream\n")
			buf.Write(obj.stream)
			fmt.Fprintf(&buf, "\nendstream\n")
		}
		buf.WriteString("endobj\n")
	}

	// Cross-reference table
	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n")
	fmt.Fprintf(&buf, "0 %d\n", len(objects)+1)
	fmt.Fprintf(&buf, "0000000000 65535 f \n")
	for i := 1; i <= len(objects); i++ {
		if offsets[i] > 0 {
			fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
		} else {
			fmt.Fprintf(&buf, "0000000000 00000 f \n")
		}
	}

	// Trailer
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root %d 0 R", len(objects)+1, catalogID)
	if infoID > 0 {
		fmt.Fprintf(&buf, " /Info %d 0 R", infoID)
	}
	fmt.Fprintf(&buf, " >>\n")
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	return buf.Bytes()
}

// decodeImageConfig extracts width and height from image data.
func decodeImageConfig(data []byte) (width, height int, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

// writePNG encodes an image.Image as PNG.
func writePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	return buf.Bytes(), err
}

// writeJPEG encodes an image.Image as JPEG.
func writeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	return buf.Bytes(), err
}
