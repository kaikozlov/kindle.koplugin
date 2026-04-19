package kfx

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// VAL-D-020: USE_HIGHEST_RESOLUTION_IMAGE_VARIANT constant
// ---------------------------------------------------------------------------

func TestUseHighestResolutionImageVariant(t *testing.T) {
	if !USE_HIGHEST_RESOLUTION_IMAGE_VARIANT {
		t.Error("USE_HIGHEST_RESOLUTION_IMAGE_VARIANT should be true")
	}
}

// ---------------------------------------------------------------------------
// VAL-D-021: combine_images_into_cbz — produces valid ZIP
// ---------------------------------------------------------------------------

func TestCombineImagesIntoCBZProducesValidZIP(t *testing.T) {
	// Create a tiny 2x2 PNG image for testing
	imgData := createTestPNG(t, 2, 2)

	images := []ImageResource{
		{Format: "$284", Location: "page1.png", RawMedia: imgData, Height: 2, Width: 2},
		{Format: "$285", Location: "page2.jpg", RawMedia: imgData, Height: 2, Width: 2},
	}

	result := combineImagesIntoCBZ(images, nil)
	if result == nil {
		t.Fatal("combineImagesIntoCBZ returned nil for non-empty input")
	}

	// Verify it's a valid ZIP
	reader, err := zip.NewReader(bytes.NewReader(result), int64(len(result)))
	if err != nil {
		t.Fatalf("Result is not a valid ZIP: %v", err)
	}

	if len(reader.File) != 2 {
		t.Errorf("Expected 2 files in ZIP, got %d", len(reader.File))
	}

	// Verify filenames match NNNN.ext pattern
	expected := []string{"0001.png", "0002.jpg"}
	for i, f := range reader.File {
		if f.Name != expected[i] {
			t.Errorf("File %d: name=%q, want %q", i, f.Name, expected[i])
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-D-022: combine_images_into_cbz — empty input returns nil
// ---------------------------------------------------------------------------

func TestCombineImagesIntoCBZEmptyInput(t *testing.T) {
	result := combineImagesIntoCBZ(nil, nil)
	if result != nil {
		t.Error("Expected nil for empty input")
	}

	result = combineImagesIntoCBZ([]ImageResource{}, nil)
	if result != nil {
		t.Error("Expected nil for empty slice input")
	}
}

// ---------------------------------------------------------------------------
// VAL-D-023: combine_images_into_cbz — stores metadata in ZIP comment
// ---------------------------------------------------------------------------

func TestCombineImagesIntoCBZMetadataInComment(t *testing.T) {
	imgData := createTestPNG(t, 2, 2)

	images := []ImageResource{
		{Format: "$284", Location: "page1.png", RawMedia: imgData, Height: 2, Width: 2},
	}

	metadata := map[string]interface{}{
		"ComicBookInfo/1.0": map[string]interface{}{
			"title":  "Test Book",
			"author": "Test Author",
		},
	}

	result := combineImagesIntoCBZ(images, metadata)
	if result == nil {
		t.Fatal("combineImagesIntoCBZ returned nil")
	}

	reader, err := zip.NewReader(bytes.NewReader(result), int64(len(result)))
	if err != nil {
		t.Fatalf("Result is not a valid ZIP: %v", err)
	}

	if len(reader.Comment) == 0 {
		t.Error("ZIP comment is empty, expected metadata JSON")
	}

	// Parse the comment as JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(reader.Comment), &parsed); err != nil {
		t.Fatalf("ZIP comment is not valid JSON: %v", err)
	}

	cbi, ok := parsed["ComicBookInfo/1.0"]
	if !ok {
		t.Error("Metadata missing ComicBookInfo/1.0 key")
	}
	cbiMap, ok := cbi.(map[string]interface{})
	if !ok {
		t.Fatal("ComicBookInfo/1.0 is not a JSON object")
	}
	if cbiMap["title"] != "Test Book" {
		t.Errorf("title=%v, want 'Test Book'", cbiMap["title"])
	}
}

// ---------------------------------------------------------------------------
// VAL-D-024: combine_images_into_cbz — discards oversized metadata
// ---------------------------------------------------------------------------

func TestCombineImagesIntoCBZDiscardsOversizedMetadata(t *testing.T) {
	imgData := createTestPNG(t, 2, 2)

	images := []ImageResource{
		{Format: "$284", Location: "page1.png", RawMedia: imgData, Height: 2, Width: 2},
	}

	// Create metadata that exceeds 65535 bytes
	largeValue := strings.Repeat("x", 70000)
	metadata := map[string]interface{}{
		"large_field": largeValue,
	}

	result := combineImagesIntoCBZ(images, metadata)
	if result == nil {
		t.Fatal("combineImagesIntoCBZ returned nil")
	}

	reader, err := zip.NewReader(bytes.NewReader(result), int64(len(result)))
	if err != nil {
		t.Fatalf("Result is not a valid ZIP: %v", err)
	}

	if len(reader.Comment) != 0 {
		t.Errorf("ZIP comment should be empty for oversized metadata, got %d bytes", len(reader.Comment))
	}
}

// ---------------------------------------------------------------------------
// VAL-D-025: combine_images_into_pdf — produces valid PDF
// ---------------------------------------------------------------------------

func TestCombineImagesIntoPDFProducesValidPDF(t *testing.T) {
	imgData := createTestJPEG(t, 10, 10)

	images := []ImageResource{
		{Format: "$285", Location: "page1.jpg", RawMedia: imgData, Height: 10, Width: 10},
		{Format: "$285", Location: "page2.jpg", RawMedia: imgData, Height: 10, Width: 10},
	}

	result := combineImagesIntoPDF(images, nil, false, nil)
	if result == nil {
		t.Fatal("combineImagesIntoPDF returned nil for non-empty input")
	}

	// Check PDF magic bytes
	if !bytes.HasPrefix(result, []byte("%PDF-")) {
		t.Errorf("Result does not start with %%PDF- magic, got: %q", string(result[:min(20, len(result))]))
	}
}

func TestCombineImagesIntoPDFEmptyInput(t *testing.T) {
	result := combineImagesIntoPDF(nil, nil, false, nil)
	if result != nil {
		t.Error("Expected nil for empty input")
	}

	result = combineImagesIntoPDF([]ImageResource{}, nil, false, nil)
	if result != nil {
		t.Error("Expected nil for empty slice input")
	}
}

// ---------------------------------------------------------------------------
// VAL-D-026: add_pdf_outline — recursive outline nesting
// ---------------------------------------------------------------------------

func TestAddPDFOutlineRecursiveNesting(t *testing.T) {
	// Build outline structure:
	// Chapter 1 (page 0)
	//   Section 1.1 (page 1)
	//   Section 1.2 (page 2)
	// Chapter 2 (page 3)
	outline := []OutlineEntry{
		{
			Title:    "Chapter 1",
			PageNum:  0,
			Children: []OutlineEntry{
				{Title: "Section 1.1", PageNum: 1},
				{Title: "Section 1.2", PageNum: 2},
			},
		},
		{
			Title:   "Chapter 2",
			PageNum: 3,
		},
	}

	imgData := createTestJPEG(t, 10, 10)
	images := []ImageResource{
		{Format: "$285", Location: "page1.jpg", RawMedia: imgData, Height: 10, Width: 10},
		{Format: "$285", Location: "page2.jpg", RawMedia: imgData, Height: 10, Width: 10},
		{Format: "$285", Location: "page3.jpg", RawMedia: imgData, Height: 10, Width: 10},
		{Format: "$285", Location: "page4.jpg", RawMedia: imgData, Height: 10, Width: 10},
	}

	result := combineImagesIntoPDF(images, nil, false, outline)
	if result == nil {
		t.Fatal("combineImagesIntoPDF returned nil")
	}

	// The PDF should contain outline items. We verify this by checking
	// the output contains "/Outlines" which is the PDF outline root type.
	pdfStr := string(result)
	if !strings.Contains(pdfStr, "/Outlines") {
		t.Error("PDF does not contain /Outlines")
	}
	if !strings.Contains(pdfStr, "Chapter 1") {
		t.Error("PDF does not contain 'Chapter 1' outline entry")
	}
	if !strings.Contains(pdfStr, "Section 1.1") {
		t.Error("PDF does not contain 'Section 1.1' outline entry")
	}
	if !strings.Contains(pdfStr, "Chapter 2") {
		t.Error("PDF does not contain 'Chapter 2' outline entry")
	}
}

// ---------------------------------------------------------------------------
// VAL-D-027: suffix_location — dot handling
// ---------------------------------------------------------------------------

func TestSuffixLocationDotHandling(t *testing.T) {
	tests := []struct {
		location string
		suffix   string
		expected string
	}{
		{"image.jpg", "_right", "image_right.jpg"},
		{"image", "_right", "image_right"},
		{"path/to/file.png", "_b", "path/to/file_b.png"},
		{"no_extension", "_v2", "no_extension_v2"},
		{"archive.tar.gz", "_v2", "archive_v2.tar.gz"},
	}

	for _, tc := range tests {
		result := suffixLocation(tc.location, tc.suffix)
		if result != tc.expected {
			t.Errorf("suffixLocation(%q, %q) = %q, want %q", tc.location, tc.suffix, result, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-D-028: suffix_location — only first dot replaced
// ---------------------------------------------------------------------------

func TestSuffixLocationOnlyFirstDotReplaced(t *testing.T) {
	result := suffixLocation("archive.tar.gz", "_v2")
	if result != "archive_v2.tar.gz" {
		t.Errorf("suffixLocation('archive.tar.gz', '_v2') = %q, want 'archive_v2.tar.gz'", result)
	}
}

// ---------------------------------------------------------------------------
// VAL-D-029: get_ordered_images — returns images in reading order
// ---------------------------------------------------------------------------

func TestGetOrderedImagesReadingOrder(t *testing.T) {
	// Construct a fragment catalog with two image resources in a specific order
	frags := fragmentCatalog{
		ResourceRawData: map[string]map[string]interface{}{
			"img_first": {
				"$161": "$284", // format = png
				"$165": "/raw/first",
				"$422": 100, // width
				"$423": 200, // height
			},
			"img_second": {
				"$161": "$284",
				"$165": "/raw/second",
				"$422": 100,
				"$423": 200,
			},
		},
		RawFragments: map[string][]byte{
			"/raw/first":  {0x89, 0x50, 0x4E, 0x47}, // PNG header
			"/raw/second": {0x89, 0x50, 0x4E, 0x47},
		},
		ResourceFragments: map[string]resourceFragment{
			"img_first":  {ID: "img_first", Location: "/raw/first"},
			"img_second": {ID: "img_second", Location: "/raw/second"},
		},
	}

	book := &KFXImageBook{
		fragments: frags,
		// Provide ordered image resources (simulating get_ordered_image_resources output)
		orderedImageResources: []string{"img_first", "img_second"},
	}

	images := book.getOrderedImages(false, false, false)
	if len(images) != 2 {
		t.Fatalf("Expected 2 images, got %d", len(images))
	}
	if images[0].Location != "/raw/first" {
		t.Errorf("First image location=%q, want '/raw/first'", images[0].Location)
	}
	if images[1].Location != "/raw/second" {
		t.Errorf("Second image location=%q, want '/raw/second'", images[1].Location)
	}
}

// ---------------------------------------------------------------------------
// VAL-D-030: get_ordered_images — landscape splitting
// ---------------------------------------------------------------------------

func TestGetOrderedImagesLandscapeSplitting(t *testing.T) {
	// Create a landscape image (width > height) — should be split into left/right halves
	imgData := createTestPNG(t, 200, 100) // 200 wide, 100 tall = landscape

	frags := fragmentCatalog{
		ResourceRawData: map[string]map[string]interface{}{
			"landscape_img": {
				"$161": "$284",
				"$165": "/raw/landscape",
				"$422": 200,
				"$423": 100,
			},
		},
		RawFragments: map[string][]byte{
			"/raw/landscape": imgData,
		},
		ResourceFragments: map[string]resourceFragment{
			"landscape_img": {ID: "landscape_img", Location: "/raw/landscape"},
		},
	}

	book := &KFXImageBook{
		fragments:             frags,
		orderedImageResources: []string{"landscape_img"},
	}

	// With landscape splitting enabled for comics
	images := book.getOrderedImages(true, true, false)
	if len(images) != 2 {
		t.Fatalf("Expected 2 images after landscape split, got %d", len(images))
	}
}

// ---------------------------------------------------------------------------
// Helper functions for creating test images
// ---------------------------------------------------------------------------

func createTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 128, G: 128, B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("Failed to create test PNG: %v", err)
	}
	return buf.Bytes()
}

func createTestJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 128, G: 128, B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("Failed to create test JPEG: %v", err)
	}
	return buf.Bytes()
}
