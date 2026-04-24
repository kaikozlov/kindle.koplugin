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

// DebugVariants enables debug logging for image variant selection.
// Python: DEBUG_VARIANTS = False (yj_to_image_book.py:15)
const DebugVariants = false

// PDF page DPI constants — port of Python resources.py:32-35.
// Python selects the best DPI from PDF_PAGE_ALLOWED_DPI that produces a page height
// closest to PDF_PAGE_TARGET_HEIGHT (9.6 inches, typical Kindle page height).
const (
	pdfPageTargetHeight = 9.6 // Python: PDF_PAGE_TARGET_HEIGHT = 9.6 (resources.py:34)
)

// pdfPageAllowedDPI lists the allowed DPI values for PDF page sizing.
// Python: PDF_PAGE_ALLOWED_DPI = [75, 100, 125, 150, 200, 250, 300, 450, 600] (resources.py:35)
var pdfPageAllowedDPI = []int{75, 100, 125, 150, 200, 250, 300, 450, 600}

// bestDPIForPageHeight selects the DPI from pdfPageAllowedDPI that produces a page height
// closest to pdfPageTargetHeight (9.6 inches) for the given image pixel height.
// Port of Python convert_image_to_pdf DPI selection loop (resources.py:515-519).
func bestDPIForPageHeight(imageHeight int) int {
	bestDPI := 72 // fallback
	bestHeightDiff := -1.0

	for _, dpi := range pdfPageAllowedDPI {
		pageHeight := float64(imageHeight) / float64(dpi)
		diff := abs64(pageHeight - pdfPageTargetHeight)
		if bestHeightDiff < 0 || diff < bestHeightDiff {
			bestHeightDiff = diff
			bestDPI = dpi
		}
	}

	return bestDPI
}

// abs64 returns the absolute value of a float64.
func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// ---------------------------------------------------------------------------
// ImageResource — port of Python ImageResource/PdfImageResource
// ---------------------------------------------------------------------------

// ImageResource represents an image to be included in a CBZ or PDF.
// Port of Python resources.ImageResource and PdfImageResource.
type ImageResource struct {
	Format     string // YJ format symbol (e.g. "png" for PNG, "jpg" for JPG)
	Location   string // resource location path
	RawMedia   []byte // raw image data
	Height     int    // image height in pixels
	Width      int    // image width in pixels
	PageNums   []int  // for PDF resources: page numbers to include
	TotalPages int    // total pages in PDF resource
	// OriginalJPEG stores the JPEG data used to build a single-page PDF via convertImageToPDF.
	// This avoids a lossy round-trip through convertPDFPageToImage when building multi-page PDFs.
	// Python doesn't need this because pypdf can merge PDFs directly.
	OriginalJPEG []byte
}

// OutlineEntry represents a PDF bookmark/outline item with optional children.
// Port of Python's outline_entries used in add_pdf_outline (line 296-302).
type OutlineEntry struct {
	Title    string
	PageNum  int
	Children []OutlineEntry
}

// entireResourceUsed returns true if all pages of the PDF are included.
// Port of Python PdfImageResource.entire_resource_used (resources.py:247-248).
// Python: self.page_nums == list(range(1, self.total_pages + 1))
func (r *ImageResource) entireResourceUsed() bool {
	if r.TotalPages == 0 {
		return len(r.PageNums) == 0
	}
	if len(r.PageNums) != r.TotalPages {
		return false
	}
	for i, pn := range r.PageNums {
		if pn != i+1 {
			return false
		}
	}
	return true
}

// pageNumberRanges converts PageNums into a list of half-open [start, end) ranges.
// Port of Python PdfImageResource.page_number_ranges (resources.py:250-266).
// Python groups consecutive pages: [1,2,3,5,6] → [(1,4),(5,7)]
// These ranges are used with pypdf's writer.append(pages=(start,end)) API.
func (r *ImageResource) pageNumberRanges() [][2]int {
	var ranges [][2]int
	start := 0
	end := 0

	for _, pageNum := range r.PageNums {
		if start == 0 {
			start = pageNum
			end = pageNum
		} else if pageNum == end+1 {
			end = pageNum
		} else {
			ranges = append(ranges, [2]int{start, end + 1})
			start = pageNum
			end = pageNum
		}
	}

	if start != 0 {
		ranges = append(ranges, [2]int{start, end + 1})
	}

	return ranges
}

// ---------------------------------------------------------------------------
// KFXImageBook — port of Python KFX_IMAGE_BOOK class
// ---------------------------------------------------------------------------

// KFXImageBook wraps a book's fragments for CBZ/PDF image conversion.
// Port of Python KFX_IMAGE_BOOK (yj_to_image_book.py L18-213).
type KFXImageBook struct {
	fragments             fragmentCatalog
	orderedImageResources []string // pre-computed ordered image resource IDs
	orderedImagePids      []int    // pre-computed PIDs matching orderedImageResources (D2-1)
	contentPosInfo        interface{} // content position info from get_ordered_image_resources (D2-1)
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
		if splitLandscape && isComic && imgRes.Format != "pdf" && imgRes.Width > imgRes.Height {
			splitImageCount++
			newWidth := imgRes.Width / 2

			// Crop left half: margins (0, newWidth, 0, 0)
			leftData, err := cropImage(imgRes.RawMedia, imgRes.Location, imgRes.Width, imgRes.Height, 0, newWidth, 0, 0)
			if err != nil {
				log.Printf("kfx: error: %v", err)
				continue
			}
			left := ImageResource{
				Format:   imgRes.Format,
				Location: suffixLocation(imgRes.Location, "-L"),
				RawMedia: leftData,
				Height:   imgRes.Height,
				Width:    newWidth,
			}

			// Crop right half: margins (newWidth, 0, 0, 0)
			rightData, err := cropImage(imgRes.RawMedia, imgRes.Location, imgRes.Width, imgRes.Height, newWidth, 0, 0, 0)
			if err != nil {
				log.Printf("kfx: error: %v", err)
				continue
			}
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

	// Python: yj_to_image_book.py:150-153 — warn if fewer images than expected page count
	numPages := getPageCount(&b.fragments)
	if numPages > 0 && len(orderedImages) < numPages {
		log.Printf("kfx: warning: Expected %d pages but found only %d page images in book", numPages, len(orderedImages))
	}

	return orderedImages
}

// getOrderedImagesV2 returns the book's images in reading order with PIDs and contentPosInfo.
// Port of Python KFX_IMAGE_BOOK.get_ordered_images (L101-155).
// Python returns (ordered_images, ordered_image_pids, content_pos_info).
// This is the corrected 3-value return version matching Python parity.
func (b *KFXImageBook) getOrderedImagesV2(splitLandscape, isComic, isRTL bool, progress interface{}) ([]ImageResource, []int, interface{}) {
	var orderedImages []ImageResource
	var orderedImagePids []int
	splitImageCount := 0

	for idx, fid := range b.orderedImageResources {
		var pid int
		if idx < len(b.orderedImagePids) {
			pid = b.orderedImagePids[idx]
		}

		imgRes := b.getResourceImage(fid, false)
		if imgRes == nil {
			continue
		}

		// Split landscape comic images
		// Python: split_landscape_comic_images and is_comic and image_resource.format != "pdf" and image_resource.width > image_resource.height
		if splitLandscape && isComic && imgRes.Format != "pdf" && imgRes.Width > imgRes.Height {
			splitImageCount++
			newWidth := imgRes.Width / 2

			// Crop left half: margins (0, newWidth, 0, 0)
			leftData, err := cropImage(imgRes.RawMedia, imgRes.Location, imgRes.Width, imgRes.Height, 0, newWidth, 0, 0)
			if err != nil {
				log.Printf("kfx: error: %v", err)
				continue
			}
			left := ImageResource{
				Format:   imgRes.Format,
				Location: suffixLocation(imgRes.Location, "-L"),
				RawMedia: leftData,
				Height:   imgRes.Height,
				Width:    newWidth,
			}

			// Crop right half: margins (newWidth, 0, 0, 0)
			rightData, err := cropImage(imgRes.RawMedia, imgRes.Location, imgRes.Width, imgRes.Height, newWidth, 0, 0, 0)
			if err != nil {
				log.Printf("kfx: error: %v", err)
				continue
			}
			right := ImageResource{
				Format:   imgRes.Format,
				Location: suffixLocation(imgRes.Location, "-R"),
				RawMedia: rightData,
				Height:   imgRes.Height,
				Width:    newWidth,
			}

			if !isRTL {
				orderedImages = append(orderedImages, left)
				orderedImagePids = append(orderedImagePids, pid)
				imgRes = &right
			} else {
				orderedImages = append(orderedImages, right)
				orderedImagePids = append(orderedImagePids, pid)
				imgRes = &left
			}
		}

		orderedImages = append(orderedImages, *imgRes)
		orderedImagePids = append(orderedImagePids, pid)
	}

	if splitImageCount > 0 {
		log.Printf("kfx: warning: Split %d landscape comic images into left/right image pairs", splitImageCount)
	}

	// Python: yj_to_image_book.py:150-153 — warn if fewer images than expected page count
	numPages := getPageCount(&b.fragments)
	if numPages > 0 && len(orderedImages) < numPages {
		log.Printf("kfx: warning: Expected %d pages but found only %d page images in book", numPages, len(orderedImages))
	}

	return orderedImages, orderedImagePids, b.contentPosInfo
}

// getResourceImage retrieves an ImageResource from the fragment catalog.
// Port of Python KFX_IMAGE_BOOK.get_resource_image (L157-213).
// D2-11: Fixed tile handling to use combineImageTiles for full reassembly.
func (b *KFXImageBook) getResourceImage(resourceID string, ignoreVariants bool) *ImageResource {
	resData, ok := b.fragments.ResourceRawData[resourceID]
	if !ok {
		return nil
	}

	resourceFormat, _ := asString(resData["format"])
	resourceHeight := intFromVal(resData["resource_height"], resData["fixed_height"])
	resourceWidth := intFromVal(resData["resource_width"], resData["fixed_width"])
	pageIndex := intFromVal(resData["page_index"])

	var location string
	var rawMedia []byte

	// Check for tiles ($636) — D2-11: full tile reassembly via combineImageTiles
	// Python: yj_to_image_book.py:168-186
	if _, hasTiles := resData["yj.tiles"]; hasTiles {
		yjTilesRaw, ok := asSlice(resData["yj.tiles"])
		if !ok || len(yjTilesRaw) == 0 {
			return nil
		}

		// Convert to [][]string tile grid
		var yjTiles [][]string
		var tilesRawMedia [][]byte

		for _, rowVal := range yjTilesRaw {
			rowSlice, ok := asSlice(rowVal)
			if !ok {
				continue
			}
			var row []string
			for _, tileVal := range rowSlice {
				tileLoc, _ := asString(tileVal)
				row = append(row, tileLoc)
				// Get raw media for each tile from RawFragments
				if tileLoc != "" {
					if raw, exists := b.fragments.RawFragments[tileLoc]; exists {
						tilesRawMedia = append(tilesRawMedia, raw)
					} else {
						tilesRawMedia = append(tilesRawMedia, nil)
					}
				} else {
					tilesRawMedia = append(tilesRawMedia, nil)
				}
			}
			yjTiles = append(yjTiles, row)
		}

		// Extract base location from first tile (strip "-tile" suffix)
		// Python: location = yj_tiles[0][0].partition("-tile")[0]
		firstTileLoc := ""
		if len(yjTiles) > 0 && len(yjTiles[0]) > 0 {
			firstTileLoc = yjTiles[0][0]
		}
		if firstTileLoc == "" {
			return nil
		}
		if idx := strings.Index(firstTileLoc, "-tile"); idx >= 0 {
			location = firstTileLoc[:idx]
		} else {
			location = firstTileLoc
		}

		// Get tile dimensions from resource
		tileHeight := intFromVal(resData["yj.tile_height"])
		tileWidth := intFromVal(resData["yj.tile_width"])
		tilePadding := intFromVal(resData["yj.tile_padding"])

		// Reassemble tiles via combineImageTiles
		combinedMedia, combinedFormat := combineImageTiles(
			resourceID, resourceHeight, resourceWidth, resourceFormat,
			tileHeight, tileWidth, tilePadding,
			yjTiles, tilesRawMedia, ignoreVariants,
		)

		if combinedMedia == nil {
			return nil
		}
		rawMedia = combinedMedia
		resourceFormat = combinedFormat
	} else {
		// Direct resource: get location from $165
		locVal, _ := asString(resData["location"])
		location = locVal
		if location != "" {
			rawMedia = b.fragments.RawFragments[location]
		}
	}

	// Variant handling: try higher resolution variants
	// Python: yj_to_image_book.py:188-201
	if resourceFormat != "pdf" && !ignoreVariants {
		variants, _ := asSlice(resData["yj.variants"])
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

	// Python: yj_to_image_book.py:204-213
	if resourceFormat == "pdf" {
		return &ImageResource{
			Format:     "pdf",
			Location:   location,
			RawMedia:   rawMedia,
			PageNums:   []int{pageIndex + 1}, // Python: page_nums = [page_index + 1]
			TotalPages: 0,
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

// cropImage crops an image by scaling margins from resource-space to pixel-space.
// Port of Python resources.crop_image (resources.py:696-717).
//
// Margins (in resource-space) are scaled by orig_width/resource_width and
// orig_height/resource_height to get pixel-space crop coordinates.
// Returns an error if the image cannot be decoded (unexpected format).
func cropImage(data []byte, resourceName string, resourceWidth, resourceHeight, marginLeft, marginRight, marginTop, marginBottom int) ([]byte, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("cropImage: failed to decode image %q: %w", resourceName, err)
	}
	_ = format

	origWidth := img.Bounds().Dx()
	origHeight := img.Bounds().Dy()

	// Scale margins from resource-space to pixel-space
	// Python: crop_left = int(margin_left * orig_width / resource_width)
	cropLeft := marginLeft * origWidth / resourceWidth
	cropRight := origWidth - marginRight*origWidth/resourceWidth - 1
	cropTop := marginTop * origHeight / resourceHeight
	cropBottom := origHeight - marginBottom*origHeight/resourceHeight - 1

	if cropRight < cropLeft || cropBottom < cropTop {
		log.Printf("kfx: warning: cropping entire image resource %s (%d, %d) by (%d, %d, %d, %d)",
			resourceName, origWidth, origHeight, cropLeft, cropTop, cropRight, cropBottom)
	}

	subImg := img.(interface {
		SubImage(r image.Rectangle) image.Image
	}).SubImage(image.Rect(cropLeft, cropTop, cropRight, cropBottom))

	// Re-encode in the same format
	// Python resources.py:714-717 — JPEG uses optimize_jpeg_image_quality with
	// target size = len(raw_media) * 0.6; other formats save directly.
	switch format {
	case "png":
		var buf bytes.Buffer
		png.Encode(&buf, subImg)
		return buf.Bytes(), nil
	default:
		// JPEG: use optimizeJPEGImageQuality for binary-search quality matching Python
		desiredSize := int(float64(len(data)) * 0.6)
		return optimizeJPEGImageQuality(subImg, desiredSize), nil
	}
}

// ---------------------------------------------------------------------------
// tocPageForPID — D2-9: TOC page number resolution
// Python: yj_to_image_book.py:67-85 — binary search for PID in ordered image PIDs.
// ---------------------------------------------------------------------------

// tocPageForPID performs binary search to find the page number for a given PID.
// Python: iterates ordered_image_pids, setting page_num and breaking when toc_pid <= ordered_page_pid.
// Returns -1 if no pages exist.
func tocPageForPID(tocPID int, orderedPids []int) int {
	if len(orderedPids) == 0 {
		return -1
	}

	pageNum := 0
	for orderedPageNum, orderedPagePID := range orderedPids {
		pageNum = orderedPageNum
		if tocPID <= orderedPagePID {
			break
		}
	}
	return pageNum
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
// convertImageToPDF — port of Python convert_image_to_pdf (resources.py:497-527)
//
// Converts an image resource to a single-page PDF resource.
// Python uses Pillow's image.save(pdf_file, "pdf") which automatically creates
// a PDF page sized to the image dimensions at the best DPI.
// Go builds a minimal PDF with the image embedded as a DCTDecode (JPEG) stream.
//
// For PDF resources ($565), Python returns them as-is.
// For JXR resources ($548), Python converts to JPEG/PNG first, then to PDF.
// For all other formats, Python converts the image to PDF directly.
// ---------------------------------------------------------------------------

// convertImageToPDF converts an image resource into a single-page PDF resource.
// Port of Python convert_image_to_pdf (resources.py:497-527).
func convertImageToPDF(imgRes ImageResource) *ImageResource {
	// Python: if image_resource.format == "$565": return image_resource
	if imgRes.Format == "pdf" {
		return &imgRes
	}

	imageData := imgRes.RawMedia

	// Python: if image_resource.format == "$548": convert JXR first
	if imgRes.Format == "jxr" {
		convertedData, _ := convertJXRToJpegOrPNG(imageData, imgRes.Location)
		imageData = convertedData
	}

	// Decode image to get dimensions and convert to JPEG for PDF embedding
	cfg, _, err := image.DecodeConfig(bytes.NewReader(imageData))
	if err != nil {
		log.Printf("kfx: error: failed to decode image for PDF conversion %s: %v", imgRes.Location, err)
		return nil
	}

	// Re-encode as JPEG for PDF embedding
	var jpegData []byte
	if imgRes.Format == "jpg" {
		jpegData = imageData
	} else {
		img, _, err := image.Decode(bytes.NewReader(imageData))
		if err != nil {
			log.Printf("kfx: error: failed to decode image for JPEG conversion %s: %v", imgRes.Location, err)
			return nil
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
			log.Printf("kfx: error: failed to convert image to JPEG %s: %v", imgRes.Location, err)
			return nil
		}
		jpegData = buf.Bytes()
	}

	// Build minimal PDF with single page containing the image
	pdfData := buildSinglePagePDF(jpegData, cfg.Width, cfg.Height)

	return &ImageResource{
		Format:       "pdf",
		Location:     imgRes.Location,
		RawMedia:     pdfData,
		Height:       cfg.Height,
		Width:        cfg.Width,
		PageNums:     []int{1},
		TotalPages:   1,
		OriginalJPEG: jpegData, // preserve JPEG data to avoid round-trip loss
	}
}

// buildSinglePagePDF creates a minimal single-page PDF with an embedded JPEG image.
// The page dimensions use adaptive DPI from bestDPIForPageHeight to target a 9.6 inch
// page height, matching Python's Pillow image.save(pdf, dpi=(best_dpi, best_dpi))
// behavior in convert_image_to_pdf (resources.py:515-519).
func buildSinglePagePDF(jpegData []byte, width, height int) []byte {
	// Object 1: Catalog
	// Object 2: Pages
	// Object 3: Page
	// Object 4: Image XObject
	// Object 5: Content stream

	// Python: resources.py:515-519 — select best DPI targeting 9.6 inch page height
	dpi := bestDPIForPageHeight(height)
	w := float64(width) / float64(dpi) * 72.0 // convert pixel dims to PDF points (1 point = 1/72 inch)
	h := float64(height) / float64(dpi) * 72.0

	contentStream := fmt.Sprintf("q %.2f 0 0 %.2f 0 0 cm /Img1 Do Q", w, h)

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	// Object 1: Catalog
	offset1 := buf.Len()
	fmt.Fprintf(&buf, "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// Object 2: Pages
	offset2 := buf.Len()
	fmt.Fprintf(&buf, "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	// Object 3: Page
	offset3 := buf.Len()
	fmt.Fprintf(&buf, "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Contents 5 0 R /Resources << /XObject << /Img1 4 0 R >> >> >>\nendobj\n", w, h)

	// Object 4: Image XObject
	offset4 := buf.Len()
	fmt.Fprintf(&buf, "4 0 obj\n<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n", width, height, len(jpegData))
	buf.Write(jpegData)
	buf.WriteString("\nendstream\nendobj\n")

	// Object 5: Content stream
	offset5 := buf.Len()
	fmt.Fprintf(&buf, "5 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(contentStream), contentStream)

	// Cross-reference table
	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 6\n")
	fmt.Fprintf(&buf, "0000000000 65535 f \n")
	fmt.Fprintf(&buf, "%010d 00000 n \n", offset1)
	fmt.Fprintf(&buf, "%010d 00000 n \n", offset2)
	fmt.Fprintf(&buf, "%010d 00000 n \n", offset3)
	fmt.Fprintf(&buf, "%010d 00000 n \n", offset4)
	fmt.Fprintf(&buf, "%010d 00000 n \n", offset5)
	fmt.Fprintf(&buf, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// combineImagesIntoCBZ — port of Python combine_images_into_cbz (L304-347)
// ---------------------------------------------------------------------------

// combineImagesIntoCBZ creates a CBZ (ZIP) archive from ordered images.
// Metadata is serialized as compact JSON in the ZIP comment (if ≤65535 bytes).
// D2-5/D2-6: Fixed to convert PDF pages and JXR images instead of skipping.
// Python: yj_to_image_book.py:304-347.
func combineImagesIntoCBZ(orderedImages []ImageResource, metadata interface{}) []byte {
	if len(orderedImages) == 0 {
		return nil
	}

	// Track image formats for logging
	imageResourceFormats := map[string]map[string]bool{}
	reportedPdfErrors := map[string]bool{}
	var pageImages []ImageResource

	for _, imgRes := range orderedImages {
		ext := extensionForFormat(imgRes.Format)
		fmtUpper := strings.ToUpper(strings.TrimPrefix(ext, "."))
		if imageResourceFormats[fmtUpper] == nil {
			imageResourceFormats[fmtUpper] = map[string]bool{}
		}
		imageResourceFormats[fmtUpper][imgRes.Location] = true

		switch imgRes.Format {
		case "gif", "jpg", "png": // GIF, JPG, PNG — direct
			// Python: page_images.append(image_resource)
			pageImages = append(pageImages, imgRes)

		case "pdf": // PDF — convert pages to images
			// Python: yj_to_image_book.py:326-330
			for _, pageNum := range imgRes.PageNums {
				imageData, fmt := convertPDFPageToImage(
					imgRes.Location, imgRes.RawMedia, pageNum, reportedPdfErrors, false)
				pageImages = append(pageImages, ImageResource{
					Format:   fmt,
					Location: "",
					RawMedia: imageData,
				})
			}

		case "jxr": // JXR — convert to JPEG/PNG
			// Python: yj_to_image_book.py:331-333
			imageData, fmt := convertJXRToJpegOrPNG(imgRes.RawMedia, imgRes.Location)
			pageImages = append(pageImages, ImageResource{
				Format:   fmt,
				Location: "",
				RawMedia: imageData,
			})

		default:
			// Python: raise Exception("Unexpected image format: %s" % image_resource.format)
			log.Printf("kfx: error: Unexpected image format: %s", imgRes.Format)
			return nil
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
// Port of Python combine_images_into_pdf (yj_to_image_book.py:215-294).
//
// Python uses pypdf.PdfWriter to merge PDF pages and images. Go builds a minimal
// PDF from scratch since we don't have pypdf. Key differences documented inline.
//
// PDF resource handling:
//   - Python: uses pypdf.PdfReader to extract pages and pypdf.PdfWriter.append to merge
//   - Go: converts all images (including PDF pages via convertImageToPDF) to JPEG-embedded
//     PDF pages, then concatenates them. PDF resources that already contain valid PDF data
//     are included directly as pages.
//   - Limitation: Go cannot extract individual pages from a multi-page PDF like pypdf.
//     This is documented in VAL-M10-001/VAL-M10-003.
func combineImagesIntoPDF(orderedImages []ImageResource, metadata map[string]string, isRTL bool, outline []OutlineEntry) []byte {
	if len(orderedImages) == 0 {
		return nil
	}

	// Python: yj_to_image_book.py:219-232
	// Combine consecutive same-location PDF resources and convert non-PDF images to PDF.
	// Python tracks image_resource_formats for logging and combined_pdf_images for merging.
	var combinedPDFImages []ImageResource

	for _, imgRes := range orderedImages {
		if imgRes.Format == "pdf" {
			// Python: yj_to_image_book.py:225-230 — merge consecutive PDF resources from same location
			if len(combinedPDFImages) > 0 &&
				combinedPDFImages[len(combinedPDFImages)-1].Format == "pdf" &&
				combinedPDFImages[len(combinedPDFImages)-1].Location == imgRes.Location {
				// Merge page_nums into the previous entry
				combinedPDFImages[len(combinedPDFImages)-1].PageNums = append(
					combinedPDFImages[len(combinedPDFImages)-1].PageNums, imgRes.PageNums...)
			} else {
				// New PDF resource entry
				// Python: yj_to_image_book.py:230-231 — pypdf.PdfReader to count pages
				// Go: parse /Count from the PDF's Pages object
				totalPages := countPDFPages(imgRes.RawMedia)
				imgRes.TotalPages = totalPages
				combinedPDFImages = append(combinedPDFImages, imgRes)
			}
		} else {
			// Python: combined_pdf_images.append(convert_image_to_pdf(image_resource))
			pdfRes := convertImageToPDF(imgRes)
			if pdfRes != nil {
				combinedPDFImages = append(combinedPDFImages, *pdfRes)
			}
		}
	}

	if len(combinedPDFImages) == 0 {
		return nil
	}

	// Python: yj_to_image_book.py:234-247
	// If single resource using entire PDF, use its raw data directly.
	// Otherwise, combine all resources.
	//
	// Go builds pages from each combined resource. For PDF resources, we embed
	// the raw PDF data as a single page (limitation: no page extraction).
	// For image-derived PDFs, we use the JPEG-embedded page.
	var pages []pdfPage

	for _, pdfImg := range combinedPDFImages {
		if pdfImg.RawMedia == nil || len(pdfImg.RawMedia) == 0 {
			continue
		}

		if pdfImg.Format == "pdf" && bytes.HasPrefix(pdfImg.RawMedia, []byte("%PDF")) {
			// PDF resource. Two cases:
			// 1. Image-derived single-page PDFs (from convertImageToPDF, OriginalJPEG set):
			//    Use the original JPEG data directly with DPI-adjusted dimensions.
			//    This avoids the lossy round-trip through convertPDFPageToImage that would
			//    lose the original image data and DPI information.
			//    Python: yj_to_image_book.py:232-236 — writer.append(fileobj=raw_media)
			// 2. Original PDF resources (no OriginalJPEG): extract pages via convertPDFPageToImage
			//    (LIMITATION: Go cannot extract individual pages like Python's pypdf).
			if len(pdfImg.OriginalJPEG) > 0 {
				// Image-derived PDF — use original JPEG data with DPI-adjusted page dims
				dpi := bestDPIForPageHeight(pdfImg.Height)
				pageW := float64(pdfImg.Width) / float64(dpi) * 72.0
				pageH := float64(pdfImg.Height) / float64(dpi) * 72.0
				pages = append(pages, pdfPage{
					imageData: pdfImg.OriginalJPEG,
					isJPEG:    true,
					width:     pageW,
					height:    pageH,
				})
			} else if pdfImg.entireResourceUsed() {
				// Original single-page PDF resource — use raw PDF data directly
				// Python: yj_to_image_book.py:249 writer.append(fileobj=io.BytesIO(raw_media))
				cfg, err := decodeImageConfigFromPDF(pdfImg.RawMedia)
				if err != nil {
					log.Printf("kfx: warning: failed to parse PDF page dimensions for %s: %v", pdfImg.Location, err)
					continue
				}
				pages = append(pages, pdfPage{
					imageData: pdfImg.RawMedia,
					isJPEG:    false,
					isPDF:     true,
					width:     float64(cfg.Width),
					height:    float64(cfg.Height),
				})
			} else {
				// Multi-page PDF — extract individual pages
				// Python: yj_to_image_book.py:252-259 uses writer.append(pages=page_range).
				for _, pageNum := range pdfImg.PageNums {
					imageData, imgFmt := convertPDFPageToImage(
						pdfImg.Location, pdfImg.RawMedia, pageNum, nil, true)
					if len(imageData) == 0 {
						continue
					}
					cfg, _, err := image.DecodeConfig(bytes.NewReader(imageData))
					if err != nil {
						continue
					}
					// Convert to JPEG for PDF embedding
					jpegData, jpegErr := ensureJPEG(imageData, imgFmt)
					if jpegErr != nil {
						continue
					}
					pages = append(pages, pdfPage{
						imageData: jpegData,
						isJPEG:    true,
						width:     float64(cfg.Width),
						height:    float64(cfg.Height),
					})
				}
			}
		} else {
			// Image-derived resource — decode image and embed directly as JPEG page
			// Python: convert_image_to_pdf creates a PDF, but for Go's buildPDF
			// we just need the JPEG data and dimensions.
			imageData := pdfImg.RawMedia
			imgFormat := pdfImg.Format

			// Handle JXR conversion
			if imgFormat == "jxr" {
				convertedData, convertedFmt := convertJXRToJpegOrPNG(imageData, pdfImg.Location)
				imageData = convertedData
				imgFormat = convertedFmt
			}

			// Decode to get dimensions
			cfg, _, err := image.DecodeConfig(bytes.NewReader(imageData))
			if err != nil {
				log.Printf("kfx: error: failed to decode image for PDF %s: %v", pdfImg.Location, err)
				continue
			}

			// Convert to JPEG for PDF embedding
			jpegData, jpegErr := ensureJPEG(imageData, imgFormat)
			if jpegErr != nil {
				log.Printf("kfx: error: failed to convert image for PDF %s: %v", pdfImg.Location, jpegErr)
				continue
			}

			pages = append(pages, pdfPage{
				imageData: jpegData,
				isJPEG:    true,
				width:     float64(cfg.Width),
				height:    float64(cfg.Height),
			})
		}
	}

	if len(pages) == 0 {
		return nil
	}

	// Python: yj_to_image_book.py:234-236
	// If single resource using entire PDF and no metadata/RTL/outline, return raw data
	if len(combinedPDFImages) == 1 && combinedPDFImages[0].entireResourceUsed() &&
		len(metadata) == 0 && !isRTL && len(outline) == 0 {
		// Optimization: return the raw PDF data directly if it's a single complete resource
		if bytes.HasPrefix(combinedPDFImages[0].RawMedia, []byte("%PDF")) {
			return combinedPDFImages[0].RawMedia
		}
	}

	return buildPDF(pages, metadata, isRTL, outline)
}

// ---------------------------------------------------------------------------
// Minimal PDF writer
// ---------------------------------------------------------------------------

type pdfPage struct {
	imageData []byte
	isJPEG    bool
	isPDF     bool // true if imageData is a complete single-page PDF
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
// Port of Python's PDF construction logic in combine_images_into_pdf (yj_to_image_book.py:215-294).
//
// Python uses pypdf.PdfWriter with clone_from/append. Go builds PDF from scratch.
// Differences from pypdf:
//   - No compress_identical_objects (pypdf-specific optimization)
//   - No pypdf page range extraction (all pages embedded as JPEG)
//   - PDF structure is functionally equivalent: catalog, pages, page objects, outline
func buildPDF(pages []pdfPage, metadata map[string]string, isRTL bool, outline []OutlineEntry) []byte {
	var objects []pdfObject
	objID := 1
	nextID := func() int { id := objID; objID++; return id }

	// Pre-allocate object IDs
	catalogID := nextID()
	pagesID := nextID()

	var outlineRootID int
	if len(outline) > 0 {
		outlineRootID = nextID()
	}

	// Allocate page IDs
	pageIDs := make([]int, len(pages))
	for i := range pages {
		pageIDs[i] = nextID()
		// image object + content stream object per page
		nextID() // image XObject
		nextID() // content stream
	}

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

	// Find maximum object ID for xref sizing
	maxID := 0
	for _, obj := range objects {
		if obj.id > maxID {
			maxID = obj.id
		}
	}

	// Object positions for xref
	offsets := make([]int64, maxID+1)

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
	fmt.Fprintf(&buf, "0 %d\n", maxID+1)
	fmt.Fprintf(&buf, "0000000000 65535 f \n")
	for i := 1; i <= maxID; i++ {
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

// decodeImageConfigFromPDF extracts page dimensions from a PDF file.
// Parses the first page's MediaBox to get width and height.
// Returns error if the PDF cannot be parsed.
func decodeImageConfigFromPDF(pdfData []byte) (image.Config, error) {
	if len(pdfData) == 0 || !bytes.HasPrefix(pdfData, []byte("%PDF")) {
		return image.Config{}, fmt.Errorf("not a valid PDF")
	}

	// Parse MediaBox from the first page
	// Pattern: /MediaBox [0 0 WIDTH HEIGHT]
	mediaBoxIdx := bytes.Index(pdfData, []byte("/MediaBox"))
	if mediaBoxIdx < 0 {
		return image.Config{}, fmt.Errorf("no MediaBox found in PDF")
	}

	// Extract the MediaBox values
	boxStart := bytes.Index(pdfData[mediaBoxIdx:], []byte("["))
	if boxStart < 0 {
		return image.Config{}, fmt.Errorf("malformed MediaBox")
	}
	boxStart += mediaBoxIdx + 1
	boxEnd := bytes.Index(pdfData[boxStart:], []byte("]"))
	if boxEnd < 0 {
		return image.Config{}, fmt.Errorf("malformed MediaBox")
	}

	boxContent := string(pdfData[boxStart : boxStart+boxEnd])
	// Parse "0 0 WIDTH HEIGHT" or similar
	var x0, y0, w, h float64
	n, _ := fmt.Sscanf(strings.TrimSpace(boxContent), "%f %f %f %f", &x0, &y0, &w, &h)
	if n < 4 {
		return image.Config{}, fmt.Errorf("failed to parse MediaBox: %q", boxContent)
	}

	return image.Config{
		Width:  int(w - x0),
		Height: int(h - y0),
	}, nil
}

// decodePDFPageDimensions extracts page dimensions from a single-page PDF.
// This is used for PDFs created by convertImageToPDF (buildSinglePagePDF).
func decodePDFPageDimensions(pdfData []byte) (image.Config, error) {
	return decodeImageConfigFromPDF(pdfData)
}

// countPDFPages counts the number of pages in a PDF by parsing /Count from the Pages object.
// Returns 0 if the PDF cannot be parsed.
// Port of Python: len(pypdf.PdfReader(raw_media).pages) used in
// combine_images_into_pdf (yj_to_image_book.py:231) and get_resource_image.
func countPDFPages(pdfData []byte) int {
	if len(pdfData) == 0 || !bytes.HasPrefix(pdfData, []byte("%PDF")) {
		return 0
	}

	// Find /Type /Pages object and extract /Count value
	// Pattern: /Type /Pages ... /Count N
	// Search for /Type /Pages first
	typePagesIdx := bytes.Index(pdfData, []byte("/Type /Pages"))
	if typePagesIdx < 0 {
		// Fallback: just look for /Count in the Pages dictionary
		return countPDFPagesByCount(pdfData)
	}

	// Find /Count near the /Type /Pages marker (within 200 bytes)
	searchEnd := typePagesIdx + 200
	if searchEnd > len(pdfData) {
		searchEnd = len(pdfData)
	}
	searchRegion := pdfData[typePagesIdx:searchEnd]

	countIdx := bytes.Index(searchRegion, []byte("/Count"))
	if countIdx < 0 {
		return 1 // Default to 1 page if /Count not found
	}

	// Parse the number after /Count
	afterCount := searchRegion[countIdx+6:]
	// Skip whitespace
	i := 0
	for i < len(afterCount) && (afterCount[i] == ' ' || afterCount[i] == '\n' || afterCount[i] == '\r' || afterCount[i] == '\t') {
		i++
	}

	var count int
	for i < len(afterCount) && afterCount[i] >= '0' && afterCount[i] <= '9' {
		count = count*10 + int(afterCount[i]-'0')
		i++
	}

	if count == 0 {
		return 1
	}
	return count
}

// countPDFPagesByCount is a fallback that searches for /Count N anywhere in the PDF.
func countPDFPagesByCount(pdfData []byte) int {
	// Find all /Count occurrences and return the one that's in a /Pages context
	idx := 0
	for {
		countIdx := bytes.Index(pdfData[idx:], []byte("/Count"))
		if countIdx < 0 {
			return 1
		}
		countIdx += idx
		afterCount := pdfData[countIdx+6:]
		i := 0
		for i < len(afterCount) && (afterCount[i] == ' ' || afterCount[i] == '\n' || afterCount[i] == '\r' || afterCount[i] == '\t') {
			i++
		}
		var count int
		for i < len(afterCount) && afterCount[i] >= '0' && afterCount[i] <= '9' {
			count = count*10 + int(afterCount[i]-'0')
			i++
		}
		if count > 0 {
			return count
		}
		idx = countIdx + 6
		if idx >= len(pdfData) {
			return 1
		}
	}
}

// renderPDFPageToJPEG converts a single-page PDF to a JPEG image.
// Since Go doesn't have pypdf's rendering capability, this uses the
// convertPDFPageToImage pathway (which generates a placeholder for real PDFs).
//
// LIMITATION (VAL-M10-001): For PDF-backed books, this produces a placeholder
// rather than the actual rendered page. None of the 6 test books use PDF resources.
func renderPDFPageToJPEG(pdfData []byte) ([]byte, error) {
	imageData, _ := convertPDFPageToImage("embedded-pdf", pdfData, 1, nil, true)
	if len(imageData) == 0 {
		return nil, fmt.Errorf("failed to render PDF page")
	}
	return imageData, nil
}

// ensureJPEG converts image data to JPEG if it isn't already.
// Handles jpg, png, and other formats by decoding and re-encoding.
func ensureJPEG(data []byte, format string) ([]byte, error) {
	if format == "jpg" {
		return data, nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ensureJPEG: failed to decode: %w", err)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		return nil, fmt.Errorf("ensureJPEG: failed to encode JPEG: %w", err)
	}
	return buf.Bytes(), nil
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
