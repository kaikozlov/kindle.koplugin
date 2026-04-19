package kfx

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"testing"
)

// ---------------------------------------------------------------------------
// D2-1: getOrderedImages returns 3 values (images, pids, contentPosInfo)
// Python: yj_to_image_book.py:101-155
// ---------------------------------------------------------------------------

func TestGetOrderedImagesReturnValues(t *testing.T) {
	// Create a simple JPEG image for test data
	imgData := createTestJPEG(t, 100, 200)

	frags := fragmentCatalog{
		ResourceRawData: map[string]map[string]interface{}{
			"img1": {
				"$161": "$285",
				"$422": 100,
				"$423": 200,
				"$165": "res/img1.jpg",
			},
			"img2": {
				"$161": "$285",
				"$422": 100,
				"$423": 200,
				"$165": "res/img2.jpg",
			},
			"img3": {
				"$161": "$285",
				"$422": 100,
				"$423": 200,
				"$165": "res/img3.jpg",
			},
		},
		RawFragments: map[string][]byte{
			"res/img1.jpg": imgData,
			"res/img2.jpg": imgData,
			"res/img3.jpg": imgData,
		},
	}

	book := NewKFXImageBook(frags, []string{"img1", "img2", "img3"})
	images, pids, contentPosInfo := book.getOrderedImagesV2(false, false, false, nil)

	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(images))
	}
	if len(pids) != 3 {
		t.Fatalf("expected 3 pids, got %d", len(pids))
	}
	// contentPosInfo is nil since we don't have full fragment data
	_ = contentPosInfo
}

func TestGetOrderedImagesWithPids(t *testing.T) {
	imgData := createTestJPEG(t, 100, 50)

	frags := fragmentCatalog{
		ResourceRawData: map[string]map[string]interface{}{
			"img1": {
				"$161": "$285",
				"$422": 100,
				"$423": 50,
				"$165": "res/img1.jpg",
			},
		},
		RawFragments: map[string][]byte{
			"res/img1.jpg": imgData,
		},
	}

	book := NewKFXImageBook(frags, []string{"img1"})
	// Provide ordered PIDs
	book.orderedImagePids = []int{100}
	images, pids, _ := book.getOrderedImagesV2(false, false, false, nil)

	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if len(pids) != 1 || pids[0] != 100 {
		t.Fatalf("expected pids=[100], got %v", pids)
	}
}

func TestGetOrderedImagesSplitLandscape(t *testing.T) {
	// Create a landscape image (width > height) for comic splitting
	imgData := createTestJPEG(t, 200, 100)

	frags := fragmentCatalog{
		ResourceRawData: map[string]map[string]interface{}{
			"img1": {
				"$161": "$285",
				"$422": 200,
				"$423": 100,
				"$165": "res/img1.jpg",
			},
		},
		RawFragments: map[string][]byte{
			"res/img1.jpg": imgData,
		},
	}

	book := NewKFXImageBook(frags, []string{"img1"})
	book.orderedImagePids = []int{100}

	// Split landscape = true, isComic = true
	images, pids, _ := book.getOrderedImagesV2(true, true, false, nil)

	if len(images) != 2 {
		t.Fatalf("expected 2 images after split, got %d", len(images))
	}
	// LTR: left first, then right
	if images[0].Location != "res/img1-L.jpg" {
		t.Errorf("expected left image first, got %s", images[0].Location)
	}
	if images[1].Location != "res/img1-R.jpg" {
		t.Errorf("expected right image second, got %s", images[1].Location)
	}
	// PIDs: split adds PID for first half, same PID for second
	if len(pids) != 2 || pids[0] != 100 || pids[1] != 100 {
		t.Errorf("expected pids=[100,100], got %v", pids)
	}
}

func TestGetOrderedImagesSplitLandscapeRTL(t *testing.T) {
	imgData := createTestJPEG(t, 200, 100)

	frags := fragmentCatalog{
		ResourceRawData: map[string]map[string]interface{}{
			"img1": {
				"$161": "$285",
				"$422": 200,
				"$423": 100,
				"$165": "res/img1.jpg",
			},
		},
		RawFragments: map[string][]byte{
			"res/img1.jpg": imgData,
		},
	}

	book := NewKFXImageBook(frags, []string{"img1"})
	book.orderedImagePids = []int{100}

	// RTL: right first, then left
	images, _, _ := book.getOrderedImagesV2(true, true, true, nil)

	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
	// RTL: right image first, then left
	if images[0].Location != "res/img1-R.jpg" {
		t.Errorf("expected right image first (RTL), got %s", images[0].Location)
	}
	if images[1].Location != "res/img1-L.jpg" {
		t.Errorf("expected left image second (RTL), got %s", images[1].Location)
	}
}

// ---------------------------------------------------------------------------
// D2-2: convertBookToCBZ produces valid ZIP with ComicBookInfo metadata
// Python: yj_to_image_book.py:22-48
// ---------------------------------------------------------------------------

func TestConvertBookToCBZ(t *testing.T) {
	imgData := createTestJPEG(t, 100, 200)

	images := []ImageResource{
		{Format: "$285", Location: "img1.jpg", RawMedia: imgData, Width: 100, Height: 200},
		{Format: "$284", Location: "img2.png", RawMedia: createTestPNG(t, 50, 50), Width: 50, Height: 50},
	}

	cbzData := combineImagesIntoCBZ(images, map[string]interface{}{
		"ComicBookInfo/1.0": map[string]interface{}{
			"title": "Test Book",
		},
	})

	if cbzData == nil {
		t.Fatal("expected non-nil CBZ data")
	}

	// Verify it's a valid ZIP
	r, err := zip.NewReader(bytes.NewReader(cbzData), int64(len(cbzData)))
	if err != nil {
		t.Fatalf("failed to read CBZ ZIP: %v", err)
	}

	// Should have 2 image entries
	if len(r.File) != 2 {
		t.Fatalf("expected 2 files in CBZ, got %d", len(r.File))
	}

	// Check sequential naming
	if r.File[0].Name != "0001.jpg" {
		t.Errorf("expected first file '0001.jpg', got '%s'", r.File[0].Name)
	}
	if r.File[1].Name != "0002.png" {
		t.Errorf("expected second file '0002.png', got '%s'", r.File[1].Name)
	}

	// Check ComicBookInfo metadata in ZIP comment
	if len(r.Comment) == 0 {
		t.Error("expected ZIP comment with ComicBookInfo metadata")
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(r.Comment), &metadata); err != nil {
		t.Fatalf("failed to parse ComicBookInfo JSON: %v", err)
	}

	cbi, ok := metadata["ComicBookInfo/1.0"].(map[string]interface{})
	if !ok {
		t.Fatal("expected ComicBookInfo/1.0 key in metadata")
	}
	if cbi["title"] != "Test Book" {
		t.Errorf("expected title 'Test Book', got %v", cbi["title"])
	}
}

func TestConvertBookToCBZEmpty(t *testing.T) {
	result := combineImagesIntoCBZ(nil, nil)
	if result != nil {
		t.Error("expected nil for empty images")
	}
}

// ---------------------------------------------------------------------------
// D2-3: cropImage scales margins by orig_width/resource_width
// Python: resources.py:696-702
// ---------------------------------------------------------------------------

func TestCropImage_ResourceScaling(t *testing.T) {
	// Create a 1000x1000 image
	imgData := createTestJPEG(t, 1000, 1000)

	// Resource says 500x500 but actual image is 1000x1000
	// Margins are in resource-space: left=10, right=10, top=10, bottom=10
	// Scaled to pixel-space: left=10*1000/500=20, right=10*1000/500=20
	result, err := cropImage(imgData, "test", 500, 500, 10, 10, 10, 10)
	if err != nil {
		t.Fatalf("cropImage failed: %v", err)
	}

	// Verify the result is a valid image
	cfg, _, err := image.DecodeConfig(bytes.NewReader(result))
	if err != nil {
		t.Fatalf("failed to decode cropped image: %v", err)
	}

	// Expected: crop_left=20, crop_right=1000-20-1=979
	// Expected: crop_top=20, crop_bottom=1000-20-1=979
	// Result: 979-20+1 = 960 pixels (but SubImage uses Rect min..max exclusive for bounds)
	// Actually: SubImage(image.Rect(20, 20, 979, 979)) → bounds (20,20)-(979,979)
	// Dx() = 979-20 = 959, Dy() = 979-20 = 959
	if cfg.Width != 959 {
		t.Errorf("expected cropped width ~959, got %d", cfg.Width)
	}
	if cfg.Height != 959 {
		t.Errorf("expected cropped height ~959, got %d", cfg.Height)
	}
}

// ---------------------------------------------------------------------------
// D2-5/D2-6: CBZ converts PDF pages and JXR images
// Python: yj_to_image_book.py:326-333
// ---------------------------------------------------------------------------

func TestCBZ_ConvertPDFPages(t *testing.T) {
	// Create a minimal PDF
	pdfData := []byte("%PDF-1.4\n%test\n1 0 obj\n<< /Type /Catalog >>\nendobj\nxref\n0 1\n0000000000 65535 f \ntrailer\n<< /Root 1 0 R /Size 1 >>\nstartxref\n0\n%%EOF")

	imgData := createTestJPEG(t, 100, 200)

	images := []ImageResource{
		{Format: "$565", Location: "page.pdf", RawMedia: pdfData, PageNums: []int{1}},
		{Format: "$285", Location: "img1.jpg", RawMedia: imgData, Width: 100, Height: 200},
	}

	cbzData := combineImagesIntoCBZ(images, nil)
	if cbzData == nil {
		t.Fatal("expected non-nil CBZ data with PDF conversion")
	}

	r, err := zip.NewReader(bytes.NewReader(cbzData), int64(len(cbzData)))
	if err != nil {
		t.Fatalf("failed to read CBZ: %v", err)
	}

	// Should have 2 entries: 1 from PDF conversion + 1 direct image
	if len(r.File) != 2 {
		t.Fatalf("expected 2 files in CBZ (PDF converted + direct), got %d", len(r.File))
	}
}

func TestCBZ_ConvertJXR(t *testing.T) {
	imgData := createTestJPEG(t, 100, 200)

	images := []ImageResource{
		{Format: "$548", Location: "image.jxr", RawMedia: imgData, Width: 100, Height: 200},
	}

	cbzData := combineImagesIntoCBZ(images, nil)
	if cbzData == nil {
		t.Fatal("expected non-nil CBZ data with JXR conversion")
	}

	r, err := zip.NewReader(bytes.NewReader(cbzData), int64(len(cbzData)))
	if err != nil {
		t.Fatalf("failed to read CBZ: %v", err)
	}

	if len(r.File) != 1 {
		t.Fatalf("expected 1 file in CBZ (JXR converted), got %d", len(r.File))
	}
}

// ---------------------------------------------------------------------------
// D2-7: PDF writer merges sources with metadata and outline
// Python: yj_to_image_book.py:215-294
// ---------------------------------------------------------------------------

func TestCombineImagesIntoPDF(t *testing.T) {
	imgData := createTestJPEG(t, 100, 200)

	images := []ImageResource{
		{Format: "$285", Location: "img1.jpg", RawMedia: imgData, Width: 100, Height: 200},
		{Format: "$285", Location: "img2.jpg", RawMedia: createTestJPEG(t, 200, 300), Width: 200, Height: 300},
		{Format: "$285", Location: "img3.jpg", RawMedia: createTestJPEG(t, 300, 400), Width: 300, Height: 400},
	}

	metadata := map[string]string{
		"/Title":       "Test Book",
		"/Author":      "Test Author",
		"/CreationDate": "D:20240101000000",
	}

	outline := []OutlineEntry{
		{Title: "Chapter 1", PageNum: 0},
		{Title: "Chapter 2", PageNum: 1, Children: []OutlineEntry{
			{Title: "Section 2.1", PageNum: 2},
		}},
	}

	pdfData := combineImagesIntoPDF(images, metadata, false, outline)
	if pdfData == nil {
		t.Fatal("expected non-nil PDF data")
	}

	// Verify PDF header
	if !bytes.HasPrefix(pdfData, []byte("%PDF")) {
		t.Error("expected PDF header")
	}

	// Verify it has metadata
	if !bytes.Contains(pdfData, []byte("Test Book")) {
		t.Error("expected PDF to contain title metadata")
	}

	// Verify it has outline
	if !bytes.Contains(pdfData, []byte("Chapter 1")) {
		t.Error("expected PDF to contain outline entry 'Chapter 1'")
	}
}

func TestCombineImagesIntoPDF_RTL(t *testing.T) {
	imgData := createTestJPEG(t, 100, 200)

	images := []ImageResource{
		{Format: "$285", Location: "img1.jpg", RawMedia: imgData, Width: 100, Height: 200},
	}

	pdfData := combineImagesIntoPDF(images, nil, true, nil)
	if pdfData == nil {
		t.Fatal("expected non-nil PDF data")
	}

	// Verify RTL direction
	if !bytes.Contains(pdfData, []byte("/R2L")) {
		t.Error("expected PDF to contain R2L direction for RTL")
	}
}

func TestCombineImagesIntoPDF_Empty(t *testing.T) {
	result := combineImagesIntoPDF(nil, nil, false, nil)
	if result != nil {
		t.Error("expected nil for empty images")
	}
}

// ---------------------------------------------------------------------------
// D2-9: TOC page resolution via position_of_anchor and pid_for_eid binary search
// Python: yj_to_image_book.py:67-85
// ---------------------------------------------------------------------------

func TestTOCPageResolution(t *testing.T) {
	// Simulate binary search: pid 350 in [100, 200, 300, 400, 500] → page 3
	orderedPids := []int{100, 200, 300, 400, 500}

	testCases := []struct {
		tocPid   int
		expected int
	}{
		{50, 0},   // Before all → page 0 (loop never breaks, pageNum stays at 0)
		{100, 0},  // 100 <= 100 → break at page 0
		{250, 2},  // 250 > 100 → pageNum=0; 250 > 200 → pageNum=1; 250 <= 300 → break at page 2
		{300, 2},  // 300 > 100 → pageNum=0; 300 > 200 → pageNum=1; 300 <= 300 → break at page 2
		{350, 3},  // 350 > 100 → pageNum=0; 350 > 200 → pageNum=1; 350 > 300 → pageNum=2; 350 <= 400 → break at page 3
		{500, 4},  // 500 > 100→0; 500>200→1; 500>300→2; 500>400→3; 500<=500 → break at page 4
		{600, 4},  // 600 > all → loop finishes with pageNum=4
	}

	for _, tc := range testCases {
		result := tocPageForPID(tc.tocPid, orderedPids)
		if result != tc.expected {
			t.Errorf("tocPageForPID(%d, %v) = %d, want %d", tc.tocPid, orderedPids, result, tc.expected)
		}
	}
}

func TestTOCPageResolutionEmpty(t *testing.T) {
	result := tocPageForPID(100, nil)
	if result != -1 {
		t.Errorf("expected -1 for empty pids, got %d", result)
	}
}

// ---------------------------------------------------------------------------
// D2-11: getResourceImage reassembles tiled images from $636 grid
// Python: yj_to_image_book.py:157-213
// ---------------------------------------------------------------------------

func TestGetResourceImageTiles(t *testing.T) {
	// Create 4 tile images (2x2 grid)
	tileW, tileH := 100, 100
	tile1 := createTestPNG(t, tileW, tileH)
	tile2 := createTestPNG(t, tileW, tileH)
	tile3 := createTestPNG(t, tileW, tileH)
	tile4 := createTestPNG(t, tileW, tileH)

	frags := fragmentCatalog{
		ResourceRawData: map[string]map[string]interface{}{
			"tiled-img": {
				"$161": "$284",
				"$422": 200,
				"$423": 200,
				"$636": []interface{}{
					[]interface{}{"loc-tile-0-0", "loc-tile-0-1"},
					[]interface{}{"loc-tile-1-0", "loc-tile-1-1"},
				},
				"$637": tileW,
				"$638": tileH,
				"$797": 0,
			},
		},
		RawFragments: map[string][]byte{
			"loc-tile-0-0": tile1,
			"loc-tile-0-1": tile2,
			"loc-tile-1-0": tile3,
			"loc-tile-1-1": tile4,
		},
	}

	book := NewKFXImageBook(frags, []string{"tiled-img"})
	imgRes := book.getResourceImage("tiled-img", false)

	if imgRes == nil {
		t.Fatal("expected non-nil image resource for tiled image")
	}

	// Should have full 200x200 image
	if imgRes.Width != 200 || imgRes.Height != 200 {
		t.Errorf("expected 200x200, got %dx%d", imgRes.Width, imgRes.Height)
	}

	// Verify the image is valid
	cfg, _, err := image.DecodeConfig(bytes.NewReader(imgRes.RawMedia))
	if err != nil {
		t.Fatalf("failed to decode combined tile image: %v", err)
	}
	if cfg.Width != 200 || cfg.Height != 200 {
		t.Errorf("expected decoded 200x200, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestGetResourceImageTilesWithPadding(t *testing.T) {
	// Create tiles with padding (tile is 102x102, but tile area is 100x100 with 0 padding at edges)
	tileW, tileH := 100, 100
	padding := 2
	// Interior tiles have padding on sides
	tileWithPadding := createTestPNG(t, tileW+padding, tileH+padding)

	frags := fragmentCatalog{
		ResourceRawData: map[string]map[string]interface{}{
			"tiled-img": {
				"$161": "$284",
				"$422": 200,
				"$423": 200,
				"$636": []interface{}{
					[]interface{}{"tile-0-0", "tile-0-1"},
					[]interface{}{"tile-1-0", "tile-1-1"},
				},
				"$637": tileW,
				"$638": tileH,
				"$797": padding,
			},
		},
		RawFragments: map[string][]byte{
			"tile-0-0": createTestPNG(t, tileW, tileH), // edge tile, no left/top padding
			"tile-0-1": tileWithPadding,                 // edge tile, no top padding, right padding
			"tile-1-0": tileWithPadding,                 // edge tile, no left padding, bottom padding
			"tile-1-1": tileWithPadding,                 // interior tile, all padding
		},
	}

	book := NewKFXImageBook(frags, []string{"tiled-img"})
	imgRes := book.getResourceImage("tiled-img", false)

	if imgRes == nil {
		t.Fatal("expected non-nil image resource for tiled image with padding")
	}

	// Verify combined dimensions
	cfg, _, err := image.DecodeConfig(bytes.NewReader(imgRes.RawMedia))
	if err != nil {
		t.Fatalf("failed to decode combined tile image: %v", err)
	}
	if cfg.Width != 200 || cfg.Height != 200 {
		t.Errorf("expected decoded 200x200, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestGetResourceImageNoTiles(t *testing.T) {
	imgData := createTestJPEG(t, 100, 200)

	frags := fragmentCatalog{
		ResourceRawData: map[string]map[string]interface{}{
			"img1": {
				"$161": "$285",
				"$422": 100,
				"$423": 200,
				"$165": "res/img1.jpg",
			},
		},
		RawFragments: map[string][]byte{
			"res/img1.jpg": imgData,
		},
	}

	book := NewKFXImageBook(frags, []string{"img1"})
	imgRes := book.getResourceImage("img1", false)

	if imgRes == nil {
		t.Fatal("expected non-nil image resource")
	}
	if imgRes.Format != "$285" {
		t.Errorf("expected format $285, got %s", imgRes.Format)
	}
	if imgRes.Width != 100 || imgRes.Height != 200 {
		t.Errorf("expected 100x200, got %dx%d", imgRes.Width, imgRes.Height)
	}
}

func TestGetResourceImageVariantSelection(t *testing.T) {
	smallData := createTestJPEG(t, 100, 200)
	bigData := createTestJPEG(t, 200, 400)

	frags := fragmentCatalog{
		ResourceRawData: map[string]map[string]interface{}{
			"img1": {
				"$161": "$285",
				"$422": 100,
				"$423": 200,
				"$165": "res/img1.jpg",
				"$635": []interface{}{"img1-hd"},
			},
			"img1-hd": {
				"$161": "$285",
				"$422": 200,
				"$423": 400,
				"$165": "res/img1-hd.jpg",
			},
		},
		RawFragments: map[string][]byte{
			"res/img1.jpg":    smallData,
			"res/img1-hd.jpg": bigData,
		},
	}

	book := NewKFXImageBook(frags, []string{"img1"})
	imgRes := book.getResourceImage("img1", false)

	if imgRes == nil {
		t.Fatal("expected non-nil image resource")
	}

	// Should select the higher resolution variant
	if imgRes.Width != 200 || imgRes.Height != 400 {
		t.Errorf("expected variant 200x400, got %dx%d", imgRes.Width, imgRes.Height)
	}
}

func TestGetResourceImageMissing(t *testing.T) {
	frags := fragmentCatalog{
		ResourceRawData: map[string]map[string]interface{}{},
	}

	book := NewKFXImageBook(frags, []string{})
	imgRes := book.getResourceImage("nonexistent", false)

	if imgRes != nil {
		t.Error("expected nil for missing resource")
	}
}

func TestGetResourceImagePDFResource(t *testing.T) {
	pdfData := []byte("%PDF-1.4 test")

	frags := fragmentCatalog{
		ResourceRawData: map[string]map[string]interface{}{
			"pdf1": {
				"$161": "$565",
				"$165": "res/doc.pdf",
				"$564": 2, // page_index = 2, page_nums = [3]
			},
		},
		RawFragments: map[string][]byte{
			"res/doc.pdf": pdfData,
		},
	}

	book := NewKFXImageBook(frags, []string{"pdf1"})
	imgRes := book.getResourceImage("pdf1", false)

	if imgRes == nil {
		t.Fatal("expected non-nil PDF resource")
	}
	if imgRes.Format != "$565" {
		t.Errorf("expected format $565, got %s", imgRes.Format)
	}
	// Python: page_nums = [page_index + 1] — page_index 2 → page_nums [3]
	if len(imgRes.PageNums) != 1 || imgRes.PageNums[0] != 3 {
		t.Errorf("expected PageNums=[3], got %v", imgRes.PageNums)
	}
}

// ---------------------------------------------------------------------------
// suffixLocation tests
// ---------------------------------------------------------------------------

func TestSuffixLocation(t *testing.T) {
	tests := []struct {
		location, suffix, expected string
	}{
		{"image.jpg", "-L", "image-L.jpg"},
		{"image.png", "-R", "image-R.png"},
		{"image", "-L", "image-L"},
		{"path/to/image.jpeg", "-tile", "path/to/image-tile.jpeg"},
	}

	for _, tc := range tests {
		result := suffixLocation(tc.location, tc.suffix)
		if result != tc.expected {
			t.Errorf("suffixLocation(%q, %q) = %q, want %q", tc.location, tc.suffix, result, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// Helper functions for tests
// ---------------------------------------------------------------------------

func createTestJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Fill with a pattern so the image is not all zeros
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, image.Rect(0, 0, w, h).At(x%256, y%256))
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("failed to create test JPEG: %v", err)
	}
	return buf.Bytes()
}

func createTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to create test PNG: %v", err)
	}
	return buf.Bytes()
}

// isValidPDF checks if data starts with PDF header
func isValidPDF(data []byte) bool {
	return bytes.HasPrefix(data, []byte("%PDF"))
}

// isValidJPEG checks if data starts with JPEG magic bytes
func isValidJPEG(data []byte) bool {
	return len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8
}

// readPDFObject reads a PDF object at the given offset
func readPDFObject(data []byte, offset int) string {
	end := offset + 100
	if end > len(data) {
		end = len(data)
	}
	return string(data[offset:end])
}

// parsePDFXRef finds and parses the xref table
func parsePDFXRef(data []byte) map[int]int64 {
	xrefIdx := bytes.LastIndex(data, []byte("startxref\n"))
	if xrefIdx < 0 {
		return nil
	}

	// Read the offset after startxref
	line := string(data[xrefIdx+len("startxref\n"):])
	var offset int64
	fmt.Sscanf(line, "%d", &offset)

	xref := map[int]int64{}
	pos := int(offset)
	if pos <= 0 || pos >= len(data) {
		return xref
	}

	// Skip "xref\n"
	if !bytes.HasPrefix(data[pos:], []byte("xref")) {
		return xref
	}
	pos += 5 // "xref\n"

	// Read subsection header: start count
	var start, count int
	n, _ := fmt.Sscanf(string(data[pos:]), "%d %d", &start, &count)
	if n != 2 {
		return xref
	}

	// Skip to entries
	newline := bytes.Index(data[pos:], []byte("\n"))
	if newline < 0 {
		return xref
	}
	pos += newline + 1

	for i := 0; i < count && pos < len(data); i++ {
		var off int64
		var gen int
		var f string
		n, _ := fmt.Sscanf(string(data[pos:]), "%010d %05d %s", &off, &gen, &f)
		if n == 3 && f == "n" {
			xref[start+i] = off
		}
		pos += 20 // each xref entry is 20 bytes
	}

	return xref
}

// Suppress unused import warning
var _ = io.EOF
