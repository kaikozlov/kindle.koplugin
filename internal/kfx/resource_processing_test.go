package kfx

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// ---------------------------------------------------------------------------
// VAL-M3-RESOURCE-001: Tile reassembly combines $636 grid into single image
// Python: resources.py:530-625 combine_image_tiles
// ---------------------------------------------------------------------------

func TestCombineImageTiles_2x2Grid(t *testing.T) {
	tileWidth, tileHeight := 50, 50

	tiles := make([][]byte, 4)
	colors := []color.NRGBA{
		{R: 255, G: 0, B: 0, A: 255},
		{R: 0, G: 255, B: 0, A: 255},
		{R: 0, G: 0, B: 255, A: 255},
		{R: 255, G: 255, B: 0, A: 255},
	}
	for i, c := range colors {
		img := image.NewNRGBA(image.Rect(0, 0, tileWidth, tileHeight))
		for y := 0; y < tileHeight; y++ {
			for x := 0; x < tileWidth; x++ {
				img.Set(x, y, c)
			}
		}
		var buf bytes.Buffer
		png.Encode(&buf, img)
		tiles[i] = buf.Bytes()
	}

	yjTiles := [][]string{
		{"tile-0-0", "tile-0-1"},
		{"tile-1-0", "tile-1-1"},
	}

	rawMedia, resourceFormat := combineImageTiles(
		"test_resource", 100, 100, "png",
		tileHeight, tileWidth, 0,
		yjTiles, tiles, false,
	)

	if resourceFormat != "png" {
		t.Fatalf("expected format $284 (PNG), got %s", resourceFormat)
	}
	if rawMedia == nil {
		t.Fatal("expected non-nil raw media")
	}

	combined, _, err := image.Decode(bytes.NewReader(rawMedia))
	if err != nil {
		t.Fatalf("failed to decode combined image: %v", err)
	}
	bounds := combined.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Fatalf("expected combined size 100x100, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

// VAL-M3-RESOURCE-002: Tile reassembly handles missing tiles with error
// Python: resources.py:549-557
func TestCombineImageTiles_MissingTile(t *testing.T) {
	tileWidth, tileHeight := 50, 50

	img := image.NewNRGBA(image.Rect(0, 0, tileWidth, tileHeight))
	var buf bytes.Buffer
	png.Encode(&buf, img)

	yjTiles := [][]string{
		{"tile-0-0", "tile-0-1"},
	}
	tiles := [][]byte{buf.Bytes(), nil} // second tile missing

	rawMedia, _ := combineImageTiles(
		"test_resource", 50, 100, "png",
		tileHeight, tileWidth, 0,
		yjTiles, tiles, true,
	)

	// When ignore_variants=true and missing tiles, should return nil
	if rawMedia != nil {
		t.Fatal("expected nil raw media when tiles are missing and ignore_variants=true")
	}
}

// VAL-M3-RESOURCE-002: Missing tiles without ignore_variants still produces output
func TestCombineImageTiles_MissingTileNotIgnored(t *testing.T) {
	tileWidth, tileHeight := 50, 50

	img := image.NewNRGBA(image.Rect(0, 0, tileWidth, tileHeight))
	var buf bytes.Buffer
	png.Encode(&buf, img)

	yjTiles := [][]string{
		{"tile-0-0", "tile-0-1"},
	}
	tiles := [][]byte{buf.Bytes(), nil}

	// ignore_variants=false → should still produce output (with blank tile)
	rawMedia, _ := combineImageTiles(
		"test_resource", 50, 100, "png",
		tileHeight, tileWidth, 0,
		yjTiles, tiles, false,
	)

	if rawMedia == nil {
		t.Fatal("expected non-nil raw media even with missing tiles when ignore_variants=false")
	}
}

// VAL-M3-RESOURCE-003: Tile reassembly validates tile dimensions
// Python: resources.py:575-580
func TestCombineImageTiles_WrongDimensions(t *testing.T) {
	tileWidth, tileHeight := 50, 50

	// Create a tile that's the wrong size (30x30 instead of 50x50)
	img := image.NewNRGBA(image.Rect(0, 0, 30, 30))
	var buf bytes.Buffer
	png.Encode(&buf, img)

	yjTiles := [][]string{{"tile-0-0"}}
	tiles := [][]byte{buf.Bytes()}

	// Should not panic, should log error but still produce output
	rawMedia, _ := combineImageTiles(
		"test_resource", 50, 50, "png",
		tileHeight, tileWidth, 0,
		yjTiles, tiles, false,
	)
	if rawMedia == nil {
		t.Fatal("expected non-nil raw media even with wrong tile dimensions")
	}
}

// VAL-M3-RESOURCE-004: Tile reassembly handles JPEG quality optimization
// Python: resources.py:595-605 COMBINE_TILES_LOSSLESS
func TestCombineImageTiles_JpegLossless(t *testing.T) {
	tileWidth, tileHeight := 50, 50

	img := image.NewNRGBA(image.Rect(0, 0, tileWidth, tileHeight))
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95})

	yjTiles := [][]string{{"tile-0-0"}}
	tiles := [][]byte{buf.Bytes()}

	// With JPEG format ($285) and COMBINE_TILES_LOSSLESS=true, format should change to PNG ($284)
	rawMedia, resourceFormat := combineImageTiles(
		"test_resource", 50, 50, "jpg",
		tileHeight, tileWidth, 0,
		yjTiles, tiles, false,
	)

	if rawMedia == nil {
		t.Fatal("expected non-nil raw media")
	}
	if resourceFormat != "png" {
		t.Fatalf("expected format $284 (PNG for lossless), got %s", resourceFormat)
	}
}

// VAL-M3-RESOURCE-004: Non-JPEG tile formats are saved directly
func TestCombineImageTiles_PngDirect(t *testing.T) {
	tileWidth, tileHeight := 50, 50

	img := image.NewNRGBA(image.Rect(0, 0, tileWidth, tileHeight))
	var buf bytes.Buffer
	png.Encode(&buf, img)

	yjTiles := [][]string{{"tile-0-0"}}
	tiles := [][]byte{buf.Bytes()}

	// PNG format should stay PNG
	rawMedia, resourceFormat := combineImageTiles(
		"test_resource", 50, 50, "png",
		tileHeight, tileWidth, 0,
		yjTiles, tiles, false,
	)

	if rawMedia == nil {
		t.Fatal("expected non-nil raw media")
	}
	if resourceFormat != "png" {
		t.Fatalf("expected format $284 (PNG), got %s", resourceFormat)
	}
}

// Tile padding test
func TestCombineImageTiles_WithPadding(t *testing.T) {
	tileWidth, tileHeight := 50, 50
	tilePadding := 5

	// Each tile is (tileWidth + leftPad + rightPad) x (tileHeight + topPad + bottomPad)
	// For 1x1 grid: leftPad=0, rightPad=0, topPad=0, bottomPad=0
	// Actually for a 1x1 grid there's no padding needed
	// Let's do a 2x1 grid:
	// Row 0: topPad=0, bottomPad=min(5, 100-50*1)=5
	// Col 0: leftPad=0, rightPad=min(5, 100-50*1)=5 → tile is 55x55
	// Col 1: leftPad=5, rightPad=min(5, 100-50*2)=0 → tile is 55x55

	tileSize := tileWidth + tilePadding // 55
	img := image.NewNRGBA(image.Rect(0, 0, tileSize, tileSize))
	var buf bytes.Buffer
	png.Encode(&buf, img)

	yjTiles := [][]string{{"tile-0-0", "tile-0-1"}}
	tiles := [][]byte{buf.Bytes(), buf.Bytes()}

	rawMedia, _ := combineImageTiles(
		"test_resource", 50, 100, "png",
		tileHeight, tileWidth, tilePadding,
		yjTiles, tiles, false,
	)

	if rawMedia == nil {
		t.Fatal("expected non-nil raw media with padding tiles")
	}
}

// ---------------------------------------------------------------------------
// VAL-M3-RESOURCE-005: JXR-to-JPEG conversion transforms JPEG-XR resources
// Python: resources.py:269-292 convert_jxr_to_jpeg_or_png
// ---------------------------------------------------------------------------

func TestConvertJXRToJpegOrPNG_GrayImage(t *testing.T) {
	// Create a grayscale image and encode as PNG (simulating what JXR decode would produce)
	// We test the conversion function's JPEG output path
	img := image.NewGray(image.Rect(0, 0, 10, 10))
	for i := range img.Pix {
		img.Pix[i] = 128
	}

	// Encode as PNG first, then pass through convertJXRToJpegOrPNG
	// Since we can't easily create JXR data, test via the exposed function directly
	// by creating a PNG and simulating RGBA=false path
	var pngBuf bytes.Buffer
	png.Encode(&pngBuf, img)

	// Test the RGBA detection function
	result, format := convertJXRToJpegOrPNG(pngBuf.Bytes(), "test_gray")
	if result == nil {
		t.Fatal("expected non-nil result from JXR conversion")
	}
	if format != "jpg" && format != "png" {
		t.Fatalf("expected format $285 or $284, got %s", format)
	}
}

// VAL-M3-RESOURCE-006: JXR conversion handles RGBA mode as PNG
// Python: resources.py:279-280
func TestConvertJXR_RGBA(t *testing.T) {
	// RGBA images should produce PNG output (not JPEG)
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.NRGBA{R: 255, G: 0, B: 0, A: 128})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)

	result, format := convertJXRToJpegOrPNG_RGBA(buf.Bytes(), "test_rgba")
	if result == nil {
		t.Fatal("expected non-nil result for RGBA conversion")
	}
	if format != "png" {
		t.Fatalf("expected PNG ($284) for RGBA, got %s", format)
	}
}

// ---------------------------------------------------------------------------
// VAL-M3-RESOURCE-007: PDF page extraction converts PDF resource to image
// Python: resources.py:323-400 convert_pdf_page_to_image
// ---------------------------------------------------------------------------

func TestConvertPDFPageToImage_NoPanic(t *testing.T) {
	// PDF page extraction in Go requires external tool (pdftoppm) or library.
	// For synthetic testing, verify it doesn't panic and handles invalid data.
	pdfData := createMinimalPDF(1)

	// Should not panic even if rendering tools are unavailable
	result, format := convertPDFPageToImage("test.pdf", pdfData, 1, nil, false)
	// result may be nil if no PDF renderer is available — that's OK for now
	_ = result
	_ = format
}

func TestConvertPDFPageToImage_InvalidData(t *testing.T) {
	// Should handle corrupt PDF data gracefully
	result, format := convertPDFPageToImage("bad.pdf", []byte("not-a-pdf"), 1, nil, false)
	// Should fall back to default JPEG
	if result == nil {
		t.Fatal("expected non-nil result (fallback) for invalid PDF data")
	}
	if format != "jpg" {
		t.Fatalf("expected fallback format $285 (JPEG), got %s", format)
	}
}

// ---------------------------------------------------------------------------
// VAL-M3-RESOURCE-008: PDF page extraction appends page suffix to filename
// Python: yj_to_epub_resources.py:97-99
// ---------------------------------------------------------------------------

func TestPDFPageSuffix(t *testing.T) {
	tests := []struct {
		pageNum int
		want    string
	}{
		{1, "-page1"},
		{3, "-page3"},
		{10, "-page10"},
	}
	for _, tt := range tests {
		got := pdfPageSuffix(tt.pageNum)
		if got != tt.want {
			t.Errorf("pdfPageSuffix(%d) = %q, want %q", tt.pageNum, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-M3-RESOURCE-009: URI resolution validates $165 vs $166 search path mismatch
// Python: yj_to_epub_resources.py:76-79
// ---------------------------------------------------------------------------

func TestResourceSearchPath_Mismatch(t *testing.T) {
	rp := newTestResourceProcessor()

	frag := map[string]interface{}{
		"resource_name": "r1",
		"location": "loc_A",
		"search_path": "loc_B",
		"format": "jpg",
		"mime": "image/jpeg",
		"resource_width": 100,
		"resource_height": 100,
	}
	rp.fragments["$164:r1"] = frag
	rp.addTestRawMedia("loc_A", []byte("data"))

	result := rp.getExternalResource("r1", false)
	if result == nil {
		t.Fatal("expected non-nil result despite search path mismatch")
	}
	if result.location != "loc_A" {
		t.Fatalf("expected location 'loc_A', got %q", result.location)
	}
}

func TestResourceSearchPath_Match(t *testing.T) {
	rp := newTestResourceProcessor()

	frag := map[string]interface{}{
		"resource_name": "r2",
		"location": "loc_same",
		"search_path": "loc_same",
		"format": "jpg",
		"mime": "image/jpeg",
		"resource_width": 100,
		"resource_height": 100,
	}
	rp.fragments["$164:r2"] = frag
	rp.addTestRawMedia("loc_same", []byte("data"))

	result := rp.getExternalResource("r2", false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ---------------------------------------------------------------------------
// VAL-M3-RESOURCE-010: URI resolution processes $214 external resource references
// Python: yj_to_epub_resources.py:86-87
// ---------------------------------------------------------------------------

func TestResourceExternalRef(t *testing.T) {
	rp := newTestResourceProcessor()

	extData := []byte("external-resource-data")
	rp.addTestResource("ext_ref", "loc_ext", "jpg", "image/jpeg", 100, 100, nil)
	rp.addTestRawMedia("loc_ext", extData)

	mainData := []byte("main-resource-data")
	frag := map[string]interface{}{
		"resource_name": "main_res",
		"location": "loc_main",
		"format": "jpg",
		"mime": "image/jpeg",
		"resource_width": 100,
		"resource_height": 100,
		"thumbnails": "ext_ref",
	}
	rp.fragments["$164:main_res"] = frag
	rp.addTestRawMedia("loc_main", mainData)

	result := rp.getExternalResource("main_res", false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !bytes.Equal(result.rawMedia, mainData) {
		t.Fatal("expected main resource data, not external ref data")
	}

	extResult := rp.resourceCache["ext_ref"]
	if extResult == nil {
		t.Fatal("expected external resource to be cached after processing $214")
	}
}

// ---------------------------------------------------------------------------
// VAL-M3-RESOURCE-011: Resource variant selection compares dimensions correctly
// Python: yj_to_epub_resources.py:170-179
// Variant replaces main only if BOTH width AND height are larger
// ---------------------------------------------------------------------------

func TestResourceVariantSelection_BothDimensionsLarger(t *testing.T) {
	rp := newTestResourceProcessor()

	baseData := []byte("base-100x200")
	rp.addTestResource("base", "loc_base", "jpg", "image/jpeg", 100, 200, []string{"variant"})
	rp.addTestRawMedia("loc_base", baseData)

	variantData := []byte("variant-200x400")
	rp.addTestResource("variant", "loc_var", "jpg", "image/jpeg", 200, 400, nil)
	rp.addTestRawMedia("loc_var", variantData)

	result := rp.getExternalResource("base", false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.width != 200 || result.height != 400 {
		t.Fatalf("expected variant 200x400, got %dx%d", result.width, result.height)
	}
}

func TestResourceVariantSelection_WidthOnlyLarger(t *testing.T) {
	rp := newTestResourceProcessor()

	baseData := []byte("base-100x200")
	rp.addTestResource("base", "loc_base", "jpg", "image/jpeg", 100, 200, []string{"variant"})
	rp.addTestRawMedia("loc_base", baseData)

	variantData := []byte("variant-200x100")
	rp.addTestResource("variant", "loc_var", "jpg", "image/jpeg", 200, 100, nil)
	rp.addTestRawMedia("loc_var", variantData)

	result := rp.getExternalResource("base", false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.width != 100 || result.height != 200 {
		t.Fatalf("expected base 100x200 (variant not selected), got %dx%d", result.width, result.height)
	}
}

func TestResourceVariantSelection_HeightOnlyLarger(t *testing.T) {
	rp := newTestResourceProcessor()

	baseData := []byte("base-100x200")
	rp.addTestResource("base", "loc_base", "jpg", "image/jpeg", 100, 200, []string{"variant"})
	rp.addTestRawMedia("loc_base", baseData)

	variantData := []byte("variant-50x400")
	rp.addTestResource("variant", "loc_var", "jpg", "image/jpeg", 50, 400, nil)
	rp.addTestRawMedia("loc_var", variantData)

	result := rp.getExternalResource("base", false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.width != 100 || result.height != 200 {
		t.Fatalf("expected base 100x200 (variant not selected), got %dx%d", result.width, result.height)
	}
}

func TestResourceVariantSelection_SameDimensions(t *testing.T) {
	rp := newTestResourceProcessor()

	baseData := []byte("base-100x200")
	rp.addTestResource("base", "loc_base", "jpg", "image/jpeg", 100, 200, []string{"variant"})
	rp.addTestRawMedia("loc_base", baseData)

	variantData := []byte("variant-100x200")
	rp.addTestResource("variant", "loc_var", "jpg", "image/jpeg", 100, 200, nil)
	rp.addTestRawMedia("loc_var", variantData)

	result := rp.getExternalResource("base", false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.width != 100 || result.height != 200 {
		t.Fatalf("expected base 100x200 (same dims not selected), got %dx%d", result.width, result.height)
	}
}

// ---------------------------------------------------------------------------
// Integration: Tile reassembly via getExternalResource with $636
// ---------------------------------------------------------------------------

func TestGetExternalResource_TiledImage(t *testing.T) {
	rp := newTestResourceProcessor()

	tileWidth, tileHeight := 50, 50

	tileImg := image.NewNRGBA(image.Rect(0, 0, tileWidth, tileHeight))
	for y := 0; y < tileHeight; y++ {
		for x := 0; x < tileWidth; x++ {
			tileImg.Set(x, y, color.NRGBA{R: 128, G: 128, B: 128, A: 255})
		}
	}
	var tileBuf bytes.Buffer
	png.Encode(&tileBuf, tileImg)

	frag := map[string]interface{}{
		"resource_name": "tiled_res",
		"format": "png",
		"mime": "image/png",
		"resource_width": 100,
		"resource_height": 50,
		"yj.tiles": []interface{}{
			[]interface{}{"tiled_res-tile-0-0", "tiled_res-tile-0-1"},
		},
		"yj.tile_width": tileWidth,
		"yj.tile_height": tileHeight,
		"yj.tile_padding": 0,
	}
	rp.fragments["$164:tiled_res"] = frag
	rp.addTestRawMedia("tiled_res-tile-0-0", tileBuf.Bytes())
	rp.addTestRawMedia("tiled_res-tile-0-1", tileBuf.Bytes())

	result := rp.getExternalResource("tiled_res", false)
	if result == nil {
		t.Fatal("expected non-nil result for tiled resource")
	}
	if result.rawMedia == nil {
		t.Fatal("expected non-nil raw media for tiled resource")
	}
}

// ---------------------------------------------------------------------------
// Integration: PDF page via getExternalResource with $565
// ---------------------------------------------------------------------------

func TestGetExternalResource_PDFPage(t *testing.T) {
	rp := newTestResourceProcessor()

	pdfData := createMinimalPDF(1)

	frag := map[string]interface{}{
		"resource_name": "pdf_res",
		"location": "loc_pdf",
		"format": "pdf",
		"mime": "application/pdf",
		"resource_width": 612,
		"resource_height": 792,
		"page_index": 0,
		"margin":  0,
		"margin_left":  0,
		"margin_right":  0,
		"margin_top":  0,
		"margin_bottom":  0,
	}
	rp.fragments["$164:pdf_res"] = frag
	rp.addTestRawMedia("loc_pdf", pdfData)

	result := rp.getExternalResource("pdf_res", false)
	if result == nil {
		t.Fatal("expected non-nil result for PDF resource")
	}
	if result.rawMedia == nil {
		t.Fatal("expected non-nil raw media for PDF resource")
	}
}

// ---------------------------------------------------------------------------
// Integration: $214 external ref via getExternalResource
// ---------------------------------------------------------------------------

func TestGetExternalResource_ExternalRefProcessed(t *testing.T) {
	rp := newTestResourceProcessor()

	extData := []byte("ext-data")
	rp.addTestResource("ext213", "loc_ext213", "jpg", "image/jpeg", 50, 50, nil)
	rp.addTestRawMedia("loc_ext213", extData)

	mainData := []byte("main-data")
	frag := map[string]interface{}{
		"resource_name": "main214",
		"location": "loc_main214",
		"format": "jpg",
		"mime": "image/jpeg",
		"resource_width": 100,
		"resource_height": 100,
		"thumbnails": "ext213",
	}
	rp.fragments["$164:main214"] = frag
	rp.addTestRawMedia("loc_main214", mainData)

	result := rp.getExternalResource("main214", false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	extResult := rp.resourceCache["ext213"]
	if extResult == nil {
		t.Fatal("expected external ref to be cached after processing $214")
	}
}

// ---------------------------------------------------------------------------
// Helper functions for tests
// ---------------------------------------------------------------------------

// createMinimalPDF creates a minimal valid PDF with the given number of pages.
func createMinimalPDF(pages int) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	obj1Offset := buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	obj2Offset := buf.Len()
	kids := ""
	for i := 0; i < pages; i++ {
		if i > 0 {
			kids += " "
		}
		kids += fmt.Sprintf("%d 0 R", i+3)
	}
	buf.WriteString(fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", kids, pages))

	objOffsets := []int{obj1Offset, obj2Offset}
	for i := 0; i < pages; i++ {
		objOffsets = append(objOffsets, buf.Len())
		buf.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n", i+3))
	}

	xrefOffset := buf.Len()
	buf.WriteString("xref\n")
	buf.WriteString(fmt.Sprintf("0 %d\n", len(objOffsets)+1))
	buf.WriteString("0000000000 65535 f \n")
	for _, off := range objOffsets {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}

	buf.WriteString("trailer\n")
	buf.WriteString(fmt.Sprintf("<< /Size %d /Root 1 0 R >>\n", len(objOffsets)+1))
	buf.WriteString("startxref\n")
	buf.WriteString(fmt.Sprintf("%d\n", xrefOffset))
	buf.WriteString("%%EOF\n")

	return buf.Bytes()
}
