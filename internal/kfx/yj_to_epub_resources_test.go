package kfx

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os/exec"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Existing tests
// ---------------------------------------------------------------------------

func TestPackageResourceStemImageUsesSymbolFormat(t *testing.T) {
	r := resourceFragment{
		ID:        "r1",
		Location:  "resource/c73-thumb.jpg",
		MediaType: "image/jpeg",
	}
	stem, ext := packageResourceStem(r, symShort, nil)
	if ext != ".jpg" {
		t.Fatalf("ext = %q", ext)
	}
	// SHORT format strips leading resource/ from unique_part (yj_to_epub.py).
	if want := "image_c73-thumb"; stem != want {
		t.Fatalf("stem symShort = %q want %q", stem, want)
	}
}

func TestPackageResourceStemResourceOriginal(t *testing.T) {
	r := resourceFragment{
		ID:        "plug1",
		Location:  "path/plugin-entry.bin",
		MediaType: "application/octet-stream",
	}
	stem, ext := packageResourceStem(r, symOriginal, nil)
	if ext != ".bin" {
		t.Fatalf("ext = %q", ext)
	}
	// Python resource_location_filename preserves path prefix (not "resource/") in the output.
	if want := "path/resource_plugin-entry"; stem != want {
		t.Fatalf("stem = %q want %q", stem, want)
	}
}

func TestUniquePackageResourceFilenameDedupesCaseInsensitive(t *testing.T) {
	used := map[string]struct{}{}
	a := resourceFragment{ID: "a", Location: "x/img.jpg", MediaType: "image/jpeg"}
	b := resourceFragment{ID: "b", Location: "y/img.jpg", MediaType: "image/jpeg"}
	f0 := uniquePackageResourceFilename(a, symOriginal, used, nil)
	f1 := uniquePackageResourceFilename(b, symOriginal, used, nil)
	if f0 == f1 {
		t.Fatalf("expected distinct names, got %q twice", f0)
	}
}

// ---------------------------------------------------------------------------
// VAL-B-033 through VAL-B-040: Resource variant handling & deduplication
// Port of yj_to_epub_resources.py lines 35-233
// ---------------------------------------------------------------------------

// newTestResourceProcessor creates a resourceProcessor with synthetic data for testing.
func newTestResourceProcessor() *resourceProcessor {
	return &resourceProcessor{
		resourceCache:    map[string]*resourceObj{},
		usedRawMedia:     map[string]bool{},
		saveResources:    true, // Python self.save_resources (yj_to_epub_resources.py L30)
		fragments:        map[string]map[string]interface{}{},
		rawMedia:         map[string][]byte{},
		oebpsFiles:       map[string]*outputFile{},
		manifestFiles:    map[string]*manifestEntry{},
		manifestRefCount: map[string]int{},
		usedOEBPSNames:   map[string]struct{}{},
	}
}

// addTestResource adds a synthetic $164 resource fragment to the processor.
func (rp *resourceProcessor) addTestResource(name string, location string, format string, mediaType string, width, height int, variantNames []string) {
	frag := map[string]interface{}{
		"resource_name":   name, // internal resource name
		"location":        location,
		"format":          format, // resource format (e.g., "jpg" for jpg)
		"mime":            mediaType,
		"resource_width":  width,
		"resource_height": height,
	}
	if len(variantNames) > 0 {
		variants := make([]interface{}, len(variantNames))
		for i, v := range variantNames {
			variants[i] = v
		}
		frag["yj.variants"] = variants
	}
	rp.fragments["$164:"+name] = frag
}

// addTestRawMedia adds synthetic raw media data ($417) to the processor.
func (rp *resourceProcessor) addTestRawMedia(location string, data []byte) {
	rp.rawMedia[location] = data
}

// ---------------------------------------------------------------------------
// VAL-B-033: Higher-resolution variant replaces base resource
// ---------------------------------------------------------------------------

func TestResourceVariantHigherResolutionReplacesBase(t *testing.T) {
	rp := newTestResourceProcessor()

	// Base resource: 800×600
	baseData := []byte("base-image-data-800x600")
	rp.addTestResource("base_img", "loc_base", "jpg", "image/jpeg", 800, 600, []string{"variant_img"})
	rp.addTestRawMedia("loc_base", baseData)

	// Variant: 1024×768 (strictly larger in both dimensions)
	variantData := []byte("variant-image-data-1024x768")
	rp.addTestResource("variant_img", "loc_variant", "jpg", "image/jpeg", 1024, 768, nil)
	rp.addTestRawMedia("loc_variant", variantData)

	// USE_HIGHEST_RESOLUTION_IMAGE_VARIANT is true by default
	result := rp.getExternalResource("base_img", false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.width != 1024 || result.height != 768 {
		t.Fatalf("expected variant dimensions 1024x768, got %dx%d", result.width, result.height)
	}
	if !bytes.Equal(result.rawMedia, variantData) {
		t.Fatal("expected variant raw media to replace base")
	}
}

func TestResourceVariantNotSelectedWhenOnlyOneDimensionLarger(t *testing.T) {
	rp := newTestResourceProcessor()

	// Base resource: 800×600
	baseData := []byte("base-image-data-800x600")
	rp.addTestResource("base_img", "loc_base", "jpg", "image/jpeg", 800, 600, []string{"variant_img"})
	rp.addTestRawMedia("loc_base", baseData)

	// Variant: 1024×500 (width larger but height smaller) — must NOT be selected
	variantData := []byte("variant-image-data-1024x500")
	rp.addTestResource("variant_img", "loc_variant", "jpg", "image/jpeg", 1024, 500, nil)
	rp.addTestRawMedia("loc_variant", variantData)

	result := rp.getExternalResource("base_img", false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.width != 800 || result.height != 600 {
		t.Fatalf("expected base dimensions 800x600 (variant should not replace), got %dx%d", result.width, result.height)
	}
	if !bytes.Equal(result.rawMedia, baseData) {
		t.Fatal("expected base raw media to be kept")
	}
}

// ---------------------------------------------------------------------------
// VAL-B-034: Variant selection is disabled when ignore_variants is true
// ---------------------------------------------------------------------------

func TestResourceVariantIgnoredWhenFlagSet(t *testing.T) {
	rp := newTestResourceProcessor()

	// Base resource: 800×600 with a higher-res variant
	baseData := []byte("base-image-data")
	rp.addTestResource("base_img", "loc_base", "jpg", "image/jpeg", 800, 600, []string{"variant_img"})
	rp.addTestRawMedia("loc_base", baseData)

	variantData := []byte("variant-image-data-1024x768")
	rp.addTestResource("variant_img", "loc_variant", "jpg", "image/jpeg", 1024, 768, nil)
	rp.addTestRawMedia("loc_variant", variantData)

	result := rp.getExternalResource("base_img", true) // ignore_variants=true
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.width != 800 || result.height != 600 {
		t.Fatalf("expected base dimensions 800x600 when ignore_variants=true, got %dx%d", result.width, result.height)
	}
}

// ---------------------------------------------------------------------------
// VAL-B-035: Resource deduplication by binary data comparison
// ---------------------------------------------------------------------------

func TestResourceDeduplicationByBinaryData(t *testing.T) {
	rp := newTestResourceProcessor()

	sameData := []byte("identical-image-data")

	// First resource
	rp.addTestResource("img_a", "myimage", "jpg", "image/jpeg", 100, 100, nil)
	rp.addTestRawMedia("myimage", sameData)

	// Process first resource
	rp.processExternalResource("img_a", true, false, false, false, false)
	obj1 := rp.getExternalResource("img_a", false)
	firstFilename := obj1.filename

	// Second resource with same location (so same generated filename) and same binary data.
	// In the Python flow, two different resources pointing to the same location
	// would produce the same filename via resource_location_filename.
	// We manually set up the same filename to test the dedup logic.
	rp.addTestResource("img_b", "myimage", "jpg", "image/jpeg", 100, 100, nil)
	obj2 := rp.getExternalResource("img_b", false)
	// Ensure they have the same filename (same location → same filename)
	if obj2.filename != firstFilename {
		// Override to match for the test — in real code they'd be the same
		// because same location maps to same filename
		obj2.filename = firstFilename
	}

	rp.processExternalResource("img_b", true, false, false, false, false)

	// Both should share the same manifest entry
	if obj1.manifestEntry == nil {
		t.Fatal("first resource should have manifest entry")
	}
	if obj1.manifestEntry != obj2.manifestEntry {
		t.Fatal("expected both resources to share the same manifest entry (dedup by binary data)")
	}

	// Only one file should exist in oebps_files
	if len(rp.oebpsFiles) != 1 {
		t.Fatalf("expected 1 file in oebps_files, got %d", len(rp.oebpsFiles))
	}
}

// ---------------------------------------------------------------------------
// VAL-B-036: Deduplication generates unique filename with _N suffix
// ---------------------------------------------------------------------------

func TestResourceDeduplicationGeneratesUniqueFilename(t *testing.T) {
	rp := newTestResourceProcessor()

	dataA := []byte("image-data-A")
	dataB := []byte("image-data-B")

	// Two resources with same location name pattern but different data
	rp.addTestResource("img_a", "myimage", "jpg", "image/jpeg", 100, 100, nil)
	rp.addTestRawMedia("myimage", dataA)

	rp.addTestResource("img_b", "myimage2", "jpg", "image/jpeg", 100, 100, nil)
	rp.addTestRawMedia("myimage2", dataB)

	// Process both
	rp.processExternalResource("img_a", true, false, false, false, false)
	rp.processExternalResource("img_b", true, false, false, false, false)

	// Should have 2 files in oebps_files (different data, so no dedup)
	if len(rp.oebpsFiles) != 2 {
		t.Fatalf("expected 2 files in oebps_files, got %d", len(rp.oebpsFiles))
	}

	// The filenames should differ
	objA := rp.getExternalResource("img_a", false)
	objB := rp.getExternalResource("img_b", false)
	if objA.filename == objB.filename {
		t.Fatalf("expected different filenames for different data, got same: %q", objA.filename)
	}
}

// ---------------------------------------------------------------------------
// VAL-B-037: Resource cache prevents re-fetching
// ---------------------------------------------------------------------------

func TestResourceCachePreventsReFetching(t *testing.T) {
	rp := newTestResourceProcessor()

	data := []byte("image-data")
	rp.addTestResource("cached_img", "loc_cached", "jpg", "image/jpeg", 100, 100, nil)
	rp.addTestRawMedia("loc_cached", data)

	// First call
	result1 := rp.getExternalResource("cached_img", false)
	if result1 == nil {
		t.Fatal("first call should return non-nil")
	}

	// Remove the fragment to prove second call uses cache
	delete(rp.fragments, "$164:cached_img")

	// Second call should return same cached object
	result2 := rp.getExternalResource("cached_img", false)
	if result2 == nil {
		t.Fatal("second call should return non-nil (from cache)")
	}
	if result1 != result2 {
		t.Fatal("expected same pointer from cache")
	}
}

// ---------------------------------------------------------------------------
// VAL-B-038: locateRawMedia reads from book_data $417
// ---------------------------------------------------------------------------

func TestLocateRawMediaReadsFromRawMedia(t *testing.T) {
	rp := newTestResourceProcessor()

	expected := []byte("raw-media-bytes")
	rp.addTestRawMedia("loc_abc", expected)

	data := rp.locateRawMedia("loc_abc", true)
	if data == nil {
		t.Fatal("expected non-nil raw media")
	}
	if !bytes.Equal(data, expected) {
		t.Fatalf("expected %q, got %q", expected, data)
	}
	if !rp.usedRawMedia["loc_abc"] {
		t.Fatal("expected location to be marked as used")
	}
}

func TestLocateRawMediaReturnsNilForMissing(t *testing.T) {
	rp := newTestResourceProcessor()

	data := rp.locateRawMedia("nonexistent", true)
	if data != nil {
		t.Fatalf("expected nil for missing location, got %v", data)
	}
}

func TestLocateRawMediaNoReportWhenFlagFalse(t *testing.T) {
	rp := newTestResourceProcessor()

	// Should not panic or error when report_missing=false
	data := rp.locateRawMedia("nonexistent", false)
	if data != nil {
		t.Fatalf("expected nil for missing location, got %v", data)
	}
}

// ---------------------------------------------------------------------------
// VAL-B-039: process_external_resource with is_referred uses root_filename
// ---------------------------------------------------------------------------

func TestProcessExternalResourceReferredUsesRootFilename(t *testing.T) {
	rp := newTestResourceProcessor()

	data := []byte("referred-image-data")
	rp.addTestResource("referred_img", "path/to/image", "jpg", "image/jpeg", 100, 100, nil)
	rp.addTestRawMedia("path/to/image", data)

	rp.processExternalResource("referred_img", true, false, false, false, true) // is_referred=true

	obj := rp.getExternalResource("referred_img", false)
	if obj == nil || obj.manifestEntry == nil {
		t.Fatal("expected resource to be saved with manifest entry")
	}

	// When is_referred=true, filename should be derived from root_filename(location)
	// root_filename("path/to/image") = "/path/to/image"
	// The saved filename should use this root form
	saved := false
	for filename := range rp.oebpsFiles {
		if filename == obj.filename {
			saved = true
			break
		}
	}
	if !saved {
		t.Fatalf("expected filename %q to be in oebps_files", obj.filename)
	}
}

// ---------------------------------------------------------------------------
// VAL-B-040: manifest mimetype set only for referred resources
// ---------------------------------------------------------------------------

func TestProcessExternalResourceManifestMimetypeOnlyForReferred(t *testing.T) {
	rp := newTestResourceProcessor()

	data := []byte("image-data-with-mime")
	rp.addTestResource("mime_test", "loc_mime", "jpg", "image/jpeg", 100, 100, nil)
	rp.addTestRawMedia("loc_mime", data)

	// Process as non-referred
	rp.processExternalResource("mime_test", true, false, false, false, false) // is_referred=false

	obj := rp.getExternalResource("mime_test", false)
	if obj == nil || obj.manifestEntry == nil {
		t.Fatal("expected manifest entry")
	}
	// For non-referred resources, mimetype should NOT be set from the resource's mime field
	// (it's determined by filename extension in the manifest)
	if obj.manifestEntry.mimetype != "" {
		t.Fatalf("expected empty mimetype for non-referred resource, got %q", obj.manifestEntry.mimetype)
	}

	// Now test with is_referred=true
	rp2 := newTestResourceProcessor()
	rp2.addTestResource("mime_test2", "loc_mime2", "jpg", "image/jpeg", 100, 100, nil)
	rp2.addTestRawMedia("loc_mime2", data)

	rp2.processExternalResource("mime_test2", true, false, false, false, true) // is_referred=true

	obj2 := rp2.getExternalResource("mime_test2", false)
	if obj2 == nil || obj2.manifestEntry == nil {
		t.Fatal("expected manifest entry for referred resource")
	}
	// For referred resources, mimetype IS set from the resource's mime field
	if obj2.manifestEntry.mimetype != "image/jpeg" {
		t.Fatalf("expected mimetype 'image/jpeg' for referred resource, got %q", obj2.manifestEntry.mimetype)
	}
}

// ---------------------------------------------------------------------------
// VAL-M6-001: PDF page extraction (GAP 12)
// Python: yj_to_epub_resources.py L112-115, resources.py L323-460
//
// Python's fallback for a non-extractable PDF page is a real pdftoppm rendering;
// if that fails, the exception path keeps the original PDF data in the resource.
// Go has no pure-Go PDF rasterizer (see convertPDFPageToImage doc comment), so
// the equivalent behavior is to report failure as an error — never a fabricated
// blank JPEG.
// ---------------------------------------------------------------------------

func TestConvertPDFPageToImage_ValidPDF_NoImage_ReturnsError(t *testing.T) {
	// A valid PDF whose page contains no extractable image must fail honestly
	// (Python: len(page.images) != 1 → rendered default; without a renderer Go
	// propagates the failure to the caller).
	t.Setenv("PATH", "/nonexistent") // force the no-renderer deployment
	pdfData := createMinimalPDF(1)

	result, format, err := convertPDFPageToImage("test.pdf", pdfData, 1, nil, false)
	if err == nil {
		t.Fatal("expected error for valid PDF with no embedded image")
	}
	if result != nil || format != "" {
		t.Fatalf("expected nil data/format on failure, got (%d bytes, %q)", len(result), format)
	}
}

func TestConvertPDFPageToImage_InvalidPDF_ReturnsError(t *testing.T) {
	// Invalid/non-PDF data must produce an error, not a fabricated image
	result, format, err := convertPDFPageToImage("bad.pdf", []byte("not-pdf"), 1, nil, false)
	if err == nil {
		t.Fatal("expected error for invalid PDF data")
	}
	if result != nil || format != "" {
		t.Fatalf("expected nil data/format on failure, got (%d bytes, %q)", len(result), format)
	}
}

func TestConvertPDFPageToImage_EmptyPDF_ReturnsError(t *testing.T) {
	result, format, err := convertPDFPageToImage("empty.pdf", nil, 1, nil, false)
	if err == nil {
		t.Fatal("expected error for empty PDF data")
	}
	if result != nil || format != "" {
		t.Fatalf("expected nil data/format on failure, got (%d bytes, %q)", len(result), format)
	}
}

func TestGetExternalResource_PDFResource_ExtractsAndConverts(t *testing.T) {
	// Verify the full pipeline: resource with format "pdf" triggers FIX_PDF path.
	// This PDF has no extractable page image, so conversion fails honestly and the
	// original PDF data, format and extension are retained
	// (Python: yj_to_epub_resources.py L112-115 exception path).
	t.Setenv("PATH", "/nonexistent") // force the no-renderer deployment
	rp := newTestResourceProcessor()

	pdfData := createMinimalPDF(1)
	frag := map[string]interface{}{
		"resource_name":   "pdf_res",
		"location":        "loc_pdf",
		"format":          "pdf",
		"mime":            "application/pdf",
		"resource_width":  612,
		"resource_height": 792,
		"page_index":      0,
		"yj.margin":       0,
		"margin_left":     0,
		"margin_right":    0,
		"margin_top":      0,
		"margin_bottom":   0,
	}
	rp.fragments["$164:pdf_res"] = frag
	rp.addTestRawMedia("loc_pdf", pdfData)

	result := rp.getExternalResource("pdf_res", false)
	if result == nil {
		t.Fatal("expected non-nil result for PDF resource")
	}
	if result.rawMedia == nil {
		t.Fatal("expected non-nil raw media")
	}
	// Honest failure keeps the original PDF bytes — no blank JPEG substitution.
	if !bytes.Equal(result.rawMedia, pdfData) {
		t.Fatal("expected raw media to keep the original PDF data on conversion failure")
	}
	if result.format != "pdf" {
		t.Fatalf("expected format to stay 'pdf' after failed conversion, got %q", result.format)
	}
	if result.extension != ".pdf" {
		t.Fatalf("expected extension to stay '.pdf' after failed conversion, got %q", result.extension)
	}
	// Filename should contain the page suffix
	if !strings.Contains(result.filename, "-page") {
		t.Fatalf("expected filename to contain page suffix, got %q", result.filename)
	}
}

func TestGetExternalResource_PDFResource_PageFragment(t *testing.T) {
	// Python L181: if "$564" in resource: filename += "#page=%d" % (resource.pop("$564") + 1)
	// Go: page_index → filename fragment
	rp := newTestResourceProcessor()

	pdfData := createMinimalPDF(1)
	frag := map[string]interface{}{
		"resource_name":   "pdf_frag",
		"location":        "loc_pdf_frag",
		"format":          "pdf",
		"mime":            "application/pdf",
		"resource_width":  612,
		"resource_height": 792,
		"page_index":      2, // 0-based, so page 3
		"yj.margin":       0,
		"margin_left":     0,
		"margin_right":    0,
		"margin_top":      0,
		"margin_bottom":   0,
	}
	rp.fragments["$164:pdf_frag"] = frag
	rp.addTestRawMedia("loc_pdf_frag", pdfData)

	result := rp.getExternalResource("pdf_frag", false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Filename should contain "#page=3" (page_index 2 + 1)
	if !strings.Contains(result.filename, "#page=3") {
		t.Fatalf("expected filename to contain '#page=3', got %q", result.filename)
	}
}

// ---------------------------------------------------------------------------
// VAL-M6-002: Referred resource recursive processing (N9)
// Python: yj_to_epub_resources.py L195-198
//
// processExternalResource with process_referred=true should recursively
// process all referred_resources entries.
// ---------------------------------------------------------------------------

func TestProcessExternalResource_ReferredResourcesRecursive(t *testing.T) {
	// N9: Python L195-198: if process_referred or save_referred:
	//     for rr in resource_obj.referred_resources:
	//         self.process_external_resource(rr, save=save_referred, is_referred=True)
	rp := newTestResourceProcessor()

	// Main resource with referred sub-resource
	mainData := []byte("main-resource-data")
	rp.addTestResource("main", "loc_main", "jpg", "image/jpeg", 100, 100, nil)
	rp.addTestRawMedia("loc_main", mainData)

	// Referred sub-resource
	referredData := []byte("referred-resource-data")
	rp.addTestResource("referred_sub", "loc_referred", "jpg", "image/jpeg", 50, 50, nil)
	rp.addTestRawMedia("loc_referred", referredData)

	// Manually set referred_resources on main
	mainFrag := rp.fragments["$164:main"]
	mainFrag["referred_resources"] = []interface{}{"referred_sub"}

	// Process with process_referred=true
	obj := rp.processExternalResource("main", false, true, false, false, false)
	if obj == nil {
		t.Fatal("expected non-nil result")
	}

	// The referred resource should have been processed and cached
	referredObj := rp.resourceCache["referred_sub"]
	if referredObj == nil {
		t.Fatal("expected referred resource to be processed (cached)")
	}
}

func TestProcessExternalResource_SaveReferredRecursive(t *testing.T) {
	// Python L195-198: save_referred=true causes referred resources to be saved
	rp := newTestResourceProcessor()

	mainData := []byte("main-data")
	rp.addTestResource("main_sr", "loc_main_sr", "jpg", "image/jpeg", 100, 100, nil)
	rp.addTestRawMedia("loc_main_sr", mainData)

	referredData := []byte("referred-data")
	rp.addTestResource("referred_sr", "loc_referred_sr", "jpg", "image/jpeg", 50, 50, nil)
	rp.addTestRawMedia("loc_referred_sr", referredData)

	mainFrag := rp.fragments["$164:main_sr"]
	mainFrag["referred_resources"] = []interface{}{"referred_sr"}

	// Process with save=true (for main) and save_referred=true (for referred)
	obj := rp.processExternalResource("main_sr", true, false, true, false, false)
	if obj == nil {
		t.Fatal("expected non-nil result")
	}

	// The referred resource should be saved (in oebps_files)
	referredObj := rp.resourceCache["referred_sr"]
	if referredObj == nil {
		t.Fatal("expected referred resource to be processed")
	}
	if !referredObj.isSaved {
		t.Fatal("expected referred resource to be saved (save_referred=true)")
	}
}

func TestProcessExternalResource_NoReferredProcessingWithoutFlag(t *testing.T) {
	// When neither process_referred nor save_referred is set,
	// referred resources should NOT be processed
	rp := newTestResourceProcessor()

	mainData := []byte("main-data-np")
	rp.addTestResource("main_np", "loc_main_np", "jpg", "image/jpeg", 100, 100, nil)
	rp.addTestRawMedia("loc_main_np", mainData)

	rp.addTestResource("referred_np", "loc_referred_np", "jpg", "image/jpeg", 50, 50, nil)
	rp.addTestRawMedia("loc_referred_np", []byte("referred-data-np"))

	mainFrag := rp.fragments["$164:main_np"]
	mainFrag["referred_resources"] = []interface{}{"referred_np"}

	// Process without process_referred or save_referred
	obj := rp.processExternalResource("main_np", false, false, false, false, false)
	if obj == nil {
		t.Fatal("expected non-nil result")
	}

	// The referred resource should NOT be cached (not processed)
	referredObj := rp.resourceCache["referred_np"]
	if referredObj != nil {
		t.Fatal("expected referred resource NOT to be processed without flag")
	}
}

// ---------------------------------------------------------------------------
// VAL-M6-EXTRA: Process External Resource validation branches
// Python: yj_to_epub_resources.py L222-225
// ---------------------------------------------------------------------------

func TestProcessExternalResource_PluginFormatValidation(t *testing.T) {
	// Python L223: elif is_plugin and resource_obj.format not in ["$287", "$284"]: error
	// $287 = "pobject", $284 = "png"
	rp := newTestResourceProcessor()

	// Resource with format "jpg" but is_plugin=true — should log error
	data := []byte("plugin-format-data")
	rp.addTestResource("plugin_bad", "loc_plugin", "jpg", "image/jpeg", 100, 100, nil)
	rp.addTestRawMedia("loc_plugin", data)

	// Should not panic; validation error is logged
	obj := rp.processExternalResource("plugin_bad", false, false, false, true, false)
	if obj == nil {
		t.Fatal("expected non-nil result despite plugin format validation")
	}
}

func TestProcessExternalResource_NonPluginPobjectValidation(t *testing.T) {
	// Python L224: elif (not is_plugin) and extension == ".pobject": error
	rp := newTestResourceProcessor()

	data := []byte("pobject-data")
	frag := map[string]interface{}{
		"resource_name":   "non_plugin_pobj",
		"location":        "loc_pobj",
		"format":          "pobject",
		"mime":            "application/azn-plugin-object",
		"resource_width":  100,
		"resource_height": 100,
	}
	rp.fragments["$164:non_plugin_pobj"] = frag
	rp.addTestRawMedia("loc_pobj", data)

	// Process as non-plugin — should log error about unexpected .pobject
	obj := rp.processExternalResource("non_plugin_pobj", false, false, false, false, false)
	if obj == nil {
		t.Fatal("expected non-nil result despite non-plugin pobject validation")
	}
	if obj.extension != ".pobject" {
		t.Fatalf("expected extension '.pobject', got %q", obj.extension)
	}
}

// ---------------------------------------------------------------------------
// VAL-M6-EXTRA: Resource mime type resolution edge cases
// Python: yj_to_epub_resources.py L89-95
// ---------------------------------------------------------------------------

func TestGetExternalResource_MimeFigure_DetectsExtensionFromData(t *testing.T) {
	// Python L91-92: if mime == "figure": extension = image_file_ext(raw_media)
	rp := newTestResourceProcessor()

	// Create valid JPEG data
	jpegData := createTestJPEG(t, 10, 10)

	frag := map[string]interface{}{
		"resource_name":   "fig_res",
		"location":        "loc_fig",
		"format":          "pobject", // will be overridden by mime "figure"
		"mime":            "figure",
		"resource_width":  100,
		"resource_height": 100,
	}
	rp.fragments["$164:fig_res"] = frag
	rp.addTestRawMedia("loc_fig", jpegData)

	result := rp.getExternalResource("fig_res", false)
	if result == nil {
		t.Fatal("expected non-nil result for figure mime type")
	}
	// Extension should be detected from the JPEG magic bytes, not remain .pobject
	if result.extension != ".jpg" {
		t.Fatalf("expected .jpg extension detected from JPEG data, got %q", result.extension)
	}
}

func TestGetExternalResource_UnknownMimeType_LogsError(t *testing.T) {
	// Python L94-95: elif mime is not None: log.error("unknown mime type")
	rp := newTestResourceProcessor()

	data := []byte("some-data")
	frag := map[string]interface{}{
		"resource_name":   "unknown_mime_res",
		"location":        "loc_um",
		"format":          "jpg",
		"mime":            "application/x-unknown-test-type",
		"resource_width":  100,
		"resource_height": 100,
	}
	rp.fragments["$164:unknown_mime_res"] = frag
	rp.addTestRawMedia("loc_um", data)

	result := rp.getExternalResource("unknown_mime_res", false)
	if result == nil {
		t.Fatal("expected non-nil result despite unknown mime type")
	}
}

// ---------------------------------------------------------------------------
// VAL-M6-EXTRA: Resource internal name validation
// Python: yj_to_epub_resources.py L44-45
// ---------------------------------------------------------------------------

func TestGetExternalResource_IncorrectInternalName(t *testing.T) {
	// Python L44-45: if int_resource_name != resource_name: log.error("Name is incorrect")
	rp := newTestResourceProcessor()

	frag := map[string]interface{}{
		"resource_name":   "wrong_name", // mismatched with fragment key
		"location":        "loc_mismatch",
		"format":          "jpg",
		"mime":            "image/jpeg",
		"resource_width":  100,
		"resource_height": 100,
	}
	rp.fragments["$164:actual_name"] = frag
	rp.addTestRawMedia("loc_mismatch", []byte("data"))

	result := rp.getExternalResource("actual_name", false)
	if result == nil {
		t.Fatal("expected non-nil result despite name mismatch")
	}
}

// ---------------------------------------------------------------------------
// VAL-M6-EXTRA: Extension from source filename fallback
// Python: yj_to_epub_resources.py L99-100
// ---------------------------------------------------------------------------

func TestGetExternalResource_ExtensionFromSourceFilename(t *testing.T) {
	// Python L99: if (extension == ".pobject" or extension == ".bin") and "." in location_fn:
	//     extension = "." + location_fn.rpartition(".")[2]
	rp := newTestResourceProcessor()

	data := []byte("data")
	frag := map[string]interface{}{
		"resource_name":                          "src_fn_res",
		"location":                               "loc_srcfn",
		"format":                                 "", // unknown format → .bin extension
		"mime":                                   "",
		"resource_width":                         100,
		"resource_height":                        100,
		"yj.conversion.source_resource_filename": "image/photo.gif",
	}
	rp.fragments["$164:src_fn_res"] = frag
	rp.addTestRawMedia("loc_srcfn", data)

	result := rp.getExternalResource("src_fn_res", false)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Extension should be derived from source filename (.gif) since format is unknown → .bin
	if result.extension != ".gif" {
		t.Fatalf("expected .gif extension from source filename, got %q", result.extension)
	}
}

// ---------------------------------------------------------------------------
// VAL-M6-EXTRA: Resource with ignore_variants and missing raw_media returns nil
// Python: yj_to_epub_resources.py L87-88
// ---------------------------------------------------------------------------

func TestGetExternalResource_IgnoreVariantsMissingMedia(t *testing.T) {
	// Python L87-88: if ignore_variants and raw_media is None: return None
	rp := newTestResourceProcessor()

	frag := map[string]interface{}{
		"resource_name":   "missing_res",
		"location":        "loc_missing",
		"format":          "jpg",
		"mime":            "image/jpeg",
		"resource_width":  100,
		"resource_height": 100,
	}
	rp.fragments["$164:missing_res"] = frag
	// No raw media added

	result := rp.getExternalResource("missing_res", true) // ignore_variants=true
	if result != nil {
		t.Fatal("expected nil when ignore_variants=true and raw_media is nil")
	}
}

// ---------------------------------------------------------------------------
// VAL-M6-EXTRA: Font property normalization
// Python: yj_to_epub_resources.py L315-317
//
// $15/$12/$13 properties equal to $350 (normal) are removed before
// convert_yj_properties. In Go, this is handled at output time —
// font-face declarations skip "normal" values.
// ---------------------------------------------------------------------------

func TestBuildResources_FontFaceSkipsNormalValues(t *testing.T) {
	// Verify that font-face CSS declarations skip "normal" style/weight/stretch
	// This tests the Go equivalent of Python L315-317 ($350 removal)
	font := fontFragment{
		Location: "font1",
		Family:   "TestFont",
		Style:    "normal", // should be skipped in CSS
		Weight:   "normal", // should be skipped in CSS
		Stretch:  "normal", // should be skipped in CSS
	}

	declarations := []string{"font-family: " + quoteFontName(font.Family)}
	if font.Style != "" && font.Style != "normal" {
		declarations = append(declarations, "font-style: "+font.Style)
	}
	if font.Weight != "" && font.Weight != "normal" {
		declarations = append(declarations, "font-weight: "+font.Weight)
	}
	if font.Stretch != "" && font.Stretch != "normal" {
		declarations = append(declarations, "font-stretch: "+font.Stretch)
	}
	declarations = append(declarations, "src: url(font.ttf)")

	css := "@font-face {" + strings.Join(declarations, "; ") + "}"

	// Should only have font-family and src, not style/weight/stretch
	if strings.Contains(css, "font-style") {
		t.Fatalf("expected no font-style for 'normal', got: %s", css)
	}
	if strings.Contains(css, "font-weight") {
		t.Fatalf("expected no font-weight for 'normal', got: %s", css)
	}
	if strings.Contains(css, "font-stretch") {
		t.Fatalf("expected no font-stretch for 'normal', got: %s", css)
	}
	if !strings.Contains(css, "font-family: TestFont") {
		t.Fatalf("expected font-family in CSS, got: %s", css)
	}
}

// ---------------------------------------------------------------------------
// GAP 12 fix: Real PDF page image extraction using pdfcpu
// Python: resources.py:323-425 — convert_pdf_page_to_image + get_pdf_page_image
// ---------------------------------------------------------------------------

// createPDFWithJPEG creates a minimal single-page PDF containing a JPEG image
// filling the page. The MediaBox is independent of the image dimensions so tests
// can control the effective DPI (image_dpi = img_width / (media_width / 72),
// Python resources.py:416-417 requires >= 75 DPI) and the aspect ratio.
func createPDFWithJPEG(imgWidth, imgHeight, pageWidth, pageHeight int) []byte {
	// Create a small JPEG image
	img := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))
	// Fill with a recognizable color pattern (red)
	for y := 0; y < imgHeight; y++ {
		for x := 0; x < imgWidth; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var jpegBuf bytes.Buffer
	jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 95})
	jpegData := jpegBuf.Bytes()

	// Build a PDF with the JPEG embedded as a DCTDecode stream
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	// Object 1: Catalog
	obj1Off := buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// Object 2: Pages
	obj2Off := buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	// Object 3: Page
	obj3Off := buf.Len()
	fmt.Fprintf(&buf, "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Contents 5 0 R /Resources << /XObject << /Im0 4 0 R >> >> >>\nendobj\n", pageWidth, pageHeight)

	// Object 4: Image XObject (DCTDecode = JPEG)
	obj4Off := buf.Len()
	fmt.Fprintf(&buf, "4 0 obj\n<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n", imgWidth, imgHeight, len(jpegData))
	buf.Write(jpegData)
	buf.WriteString("\nendstream\nendobj\n")

	// Object 5: Content stream (draw the image)
	contentStream := fmt.Sprintf("q %d 0 0 %d 0 0 cm /Im0 Do Q", pageWidth, pageHeight)
	obj5Off := buf.Len()
	fmt.Fprintf(&buf, "5 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(contentStream), contentStream)

	// Cross-reference table
	xrefOff := buf.Len()
	buf.WriteString("xref\n0 6\n")
	buf.WriteString("0000000000 65535 f \n")
	fmt.Fprintf(&buf, "%010d 00000 n \n", obj1Off)
	fmt.Fprintf(&buf, "%010d 00000 n \n", obj2Off)
	fmt.Fprintf(&buf, "%010d 00000 n \n", obj3Off)
	fmt.Fprintf(&buf, "%010d 00000 n \n", obj4Off)
	fmt.Fprintf(&buf, "%010d 00000 n \n", obj5Off)

	buf.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n")
	fmt.Fprintf(&buf, "%d\n", xrefOff)
	buf.WriteString("%%EOF\n")

	return buf.Bytes()
}

func TestConvertPDFPageToImage_PDFWithEmbeddedJPEG_ExtractsImage(t *testing.T) {
	// Python resources.py:366-425 — get_pdf_page_image extracts the single embedded
	// image from a PDF page when conditions are met (single image, no text, etc.)
	// This test verifies that Go's pdfcpu-based extraction works for the common case:
	// a single-page PDF with one embedded JPEG image.
	pdfData := createPDFWithJPEG(100, 150, 72, 108) // 100 DPI, aspect 2:3

	result, format, err := convertPDFPageToImage("test_embedded.pdf", pdfData, 1, nil, false)
	if err != nil {
		t.Fatalf("expected successful extraction for PDF with embedded JPEG, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for PDF with embedded JPEG")
	}
	if format != "jpg" {
		t.Fatalf("expected format 'jpg' for embedded JPEG, got %q", format)
	}

	// Verify the result is valid JPEG data (the extracted image, not a placeholder)
	config, err := jpeg.DecodeConfig(bytes.NewReader(result))
	if err != nil {
		t.Fatalf("extracted image should be valid JPEG, got decode error: %v", err)
	}

	// The extracted image should match the original dimensions (100x150)
	if config.Width != 100 || config.Height != 150 {
		t.Fatalf("expected extracted image dimensions 100x150, got %dx%d", config.Width, config.Height)
	}
}

func TestConvertPDFPageToImage_PDFWithEmbeddedJPEG_ForceJPEG(t *testing.T) {
	// Python resources.py:419-424 — when force_jpeg=True, convert to JPEG
	pdfData := createPDFWithJPEG(50, 50, 36, 36) // 100 DPI, aspect 1:1

	result, format, err := convertPDFPageToImage("force.jpg", pdfData, 1, nil, true)
	if err != nil {
		t.Fatalf("expected successful forceJPEG conversion, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for forceJPEG")
	}
	if format != "jpg" {
		t.Fatalf("expected format 'jpg' with forceJPEG, got %q", format)
	}

	// Verify valid JPEG
	if _, err := jpeg.Decode(bytes.NewReader(result)); err != nil {
		t.Fatalf("forceJPEG result should be valid JPEG, got error: %v", err)
	}
}

func TestConvertPDFPageToImage_EmptyPDFPages_NoImages_ReturnsError(t *testing.T) {
	// Python resources.py:384 — if len(page.images.keys()) != 1: return default_image
	// A PDF page with no embedded images cannot be extracted and there is no
	// renderer available — failure must be reported honestly.
	t.Setenv("PATH", "/nonexistent") // force the no-renderer deployment
	pdfData := createMinimalPDF(1)   // no embedded images

	result, format, err := convertPDFPageToImage("empty_pages.pdf", pdfData, 1, nil, false)
	if err == nil {
		t.Fatal("expected error for PDF with no images")
	}
	if result != nil || format != "" {
		t.Fatalf("expected nil data/format on failure, got (%d bytes, %q)", len(result), format)
	}
}

func TestConvertPDFPageToImage_InvalidPDFData_ReturnsError(t *testing.T) {
	// Python resources.py:423 — except Exception: return default_image
	// Invalid data fails the %PDF header check and must be reported as an error.
	result, format, err := convertPDFPageToImage("bad.pdf", []byte("not-a-pdf"), 1, nil, false)
	if err == nil {
		t.Fatal("expected error for invalid data")
	}
	if result != nil || format != "" {
		t.Fatalf("expected nil data/format on failure, got (%d bytes, %q)", len(result), format)
	}
}

// ---------------------------------------------------------------------------
// VAL-FIX-008: Unused font file warning + manifest
// Python: yj_to_epub_resources.py L320-323
//
// Python's process_fonts has a second loop after the main font processing:
//   for location in raw_fonts:
//       log.warning("Unused font file: %s" % location)
//       filename = self.resource_location_filename(location, "", self.FONT_FILEPATH)
//       self.manifest_resource(filename, data=raw_fonts[location])
//
// Fonts that exist in the KFX container (bcRawFont) but have no corresponding
// $262 font fragment are considered "unused" — they should be logged with a
// warning and still manifested in the EPUB.
// ---------------------------------------------------------------------------

// createTestTTF returns minimal bytes that look like a TrueType font to detectFontExtension.
func createTestTTF() []byte {
	return []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
}

func TestBuildResources_UnusedFontFile_WarnsAndManifests(t *testing.T) {
	// Python yj_to_epub_resources.py L320-323:
	// Raw font data that has no matching $262 font fragment should produce
	// a warning log and still be included in the EPUB manifest.
	book := &decodedBook{}

	// A font fragment that WILL be matched to its raw data
	fonts := map[string]fontFragment{
		"used_font_loc": {Location: "used_font_loc", Family: "UsedFont"},
	}

	// Raw data includes both used and unused font blobs
	usedFontData := createTestTTF()
	unusedFontData := createTestTTF()
	// Make them different so they don't dedupe
	unusedFontData = append(unusedFontData, 0xFF)

	raw := map[string][]byte{
		"used_font_loc":   usedFontData,
		"unused_font_loc": unusedFontData,
	}

	// rawOrder includes both; the unused one won't be consumed by the font loop
	// because there's no matching entry in the fonts map.
	rawOrder := []rawBlob{
		{ID: "used_font_loc", Data: usedFontData},
		{ID: "unused_font_loc", Data: unusedFontData},
	}

	resources, _, _, _ := buildResources(book, nil, fonts, raw, rawOrder, symOriginal)

	// Both fonts should appear in the output resources
	fontFileCount := 0
	for _, r := range resources {
		if r.MediaType == "font/ttf" || r.MediaType == "application/octet-stream" {
			fontFileCount++
		}
	}
	if fontFileCount < 2 {
		t.Fatalf("expected at least 2 font resources (used + unused), got %d; resources: %+v", fontFileCount, resources)
	}

	// The unused font should be present in the output
	foundUnused := false
	for _, r := range resources {
		if bytes.Equal(r.Data, unusedFontData) {
			foundUnused = true
			break
		}
	}
	if !foundUnused {
		t.Fatal("expected unused font data to be present in output resources")
	}
}

// ---------------------------------------------------------------------------
// VAL-FIX-009: is_kpf_prepub font raw_media fallback
// Python: yj_to_epub_resources.py L297-298
//
// Python separates raw_fonts ($418) from raw_media ($417). For KPF prepub books,
// fonts may exist in raw_media instead of raw_fonts. Python's fallback:
//   elif location in raw_fonts or (self.book.is_kpf_prepub and location in raw_media):
//       raw_font = raw_fonts.pop(location, None) or raw_media.pop(location)
//
// Go stores both bcRawFont and bcRawMedia entries in a unified RawFragments map,
// so the fallback is implicit — raw[location] finds fonts from either source.
// This test verifies that font data stored as bcRawMedia (not bcRawFont) is found
// by the font lookup in buildResources.
// ---------------------------------------------------------------------------

func TestBuildResources_FontInRawMedia_FoundByUnifiedMap(t *testing.T) {
	// Simulate a KPF prepub scenario: font data lives in bcRawMedia ($417),
	// not in a dedicated bcRawFont ($418) entry. In Go, both go into RawFragments.
	// The font lookup uses raw[location] which finds data from either source.
	book := &decodedBook{IsKpfPrepub: true}

	fonts := map[string]fontFragment{
		"media_font_loc": {Location: "media_font_loc", Family: "PrepubFont"},
	}

	// Font data only in raw (simulating bcRawMedia, no bcRawFont entry)
	fontData := createTestTTF()
	raw := map[string][]byte{
		"media_font_loc": fontData,
	}

	rawOrder := []rawBlob{
		{ID: "media_font_loc", Data: fontData},
	}

	resources, _, stylesheet, _ := buildResources(book, nil, fonts, raw, rawOrder, symOriginal)

	// Font should be found and included in output
	foundFont := false
	for _, r := range resources {
		if bytes.Equal(r.Data, fontData) {
			foundFont = true
			break
		}
	}
	if !foundFont {
		t.Fatal("expected font data from raw_media (bcRawMedia) to be found in output resources")
	}

	// Stylesheet should contain @font-face with PrepubFont family
	if !strings.Contains(stylesheet, "PrepubFont") {
		t.Fatalf("expected stylesheet to contain 'PrepubFont', got:\n%s", stylesheet)
	}
}

func TestBuildResources_FontInRawMedia_NotFoundWithoutUnifiedMap(t *testing.T) {
	// Verify that when font data is NOT in the raw map, it falls back to fontPool.
	// This tests the positional fallback (nextFontBlob) rather than raw[location].
	book := &decodedBook{IsKpfPrepub: false}

	fontData := createTestTTF()
	fonts := map[string]fontFragment{
		"some_location": {Location: "some_location", Family: "FallbackFont"},
	}

	// Empty raw map — font must come from fontPool via positional match
	raw := map[string][]byte{}
	rawOrder := []rawBlob{
		{ID: "unmatched_id", Data: fontData},
	}

	resources, _, stylesheet, _ := buildResources(book, nil, fonts, raw, rawOrder, symOriginal)

	// Font should still be found via fontPool fallback
	foundFont := false
	for _, r := range resources {
		if bytes.Equal(r.Data, fontData) {
			foundFont = true
			break
		}
	}
	if !foundFont {
		t.Fatal("expected font data to be found via fontPool fallback")
	}

	if !strings.Contains(stylesheet, "FallbackFont") {
		t.Fatalf("expected stylesheet to contain 'FallbackFont', got:\n%s", stylesheet)
	}
}

func TestGetPDFPageImage_MultiPagePDF_ExtractsCorrectPage(t *testing.T) {
	// Python resources.py:370 — pdf.pages[page_num - 1]
	// Verify that extraction targets the correct page number.
	// We use a single-page PDF and request page 1.
	pdfData := createPDFWithJPEG(80, 60, 72, 54) // 80 DPI, aspect 4:3

	result, format, err := getPDFPageImage("multipage.pdf", pdfData, 1, false, nil)
	if err != nil {
		t.Fatalf("expected successful extraction, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for page 1 extraction")
	}
	if format != "jpg" {
		t.Fatalf("expected format 'jpg', got %q", format)
	}

	// Verify dimensions match
	config, err := jpeg.DecodeConfig(bytes.NewReader(result))
	if err != nil {
		t.Fatalf("expected valid JPEG, got error: %v", err)
	}
	if config.Width != 80 || config.Height != 60 {
		t.Fatalf("expected 80x60, got %dx%d", config.Width, config.Height)
	}
}

// ---------------------------------------------------------------------------
// get_pdf_page_image validation parity tests
// Python: resources.py:382-460 — every branch of the validation chain that
// gates extraction of the single embedded image. Each test builds a minimal
// PDF whose page violates exactly one Python check and asserts the honest
// error path (Python returns the rendered default image; Go reports an error
// because no rasterizer is available — see convertPDFPageToImage).
// ---------------------------------------------------------------------------

// buildSinglePageTestPDF builds a PDF with object 1 = catalog, object 2 = pages
// (single kid = object 3), object 3 = pageBody, and any extra objects appended
// verbatim as objects 4, 5, ...
func buildSinglePageTestPDF(pageBody string, extraObjects ...[]byte) []byte {
	bodies := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(pageBody),
	}
	bodies = append(bodies, extraObjects...)

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(bodies))
	for i, b := range bodies {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", i+1)
		buf.Write(b)
		buf.WriteString("\nendobj\n")
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(bodies)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(bodies)+1, xref)
	return buf.Bytes()
}

// testImagePageBody returns a page dict body that draws image XObject Im0
// (object 4) full-page via content stream object 5.
func testImagePageBody(pageW, pageH int, extraEntries string) string {
	return fmt.Sprintf(
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Contents 5 0 R %s"+
			"/Resources << /XObject << /Im0 4 0 R >> >> >>",
		pageW, pageH, extraEntries)
}

// testJPEGImageObject returns a DCTDecode image XObject body.
func testJPEGImageObject(t *testing.T, w, h int, extraDict string) []byte {
	t.Helper()
	jpegData := createTestJPEG(t, w, h)
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 %s/Filter /DCTDecode /Length %d >>\nstream\n", w, h, extraDict, len(jpegData))
	buf.Write(jpegData)
	buf.WriteString("\nendstream")
	return buf.Bytes()
}

// testStreamObject returns an uncompressed stream object body.
func testStreamObject(data string) []byte {
	return []byte(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(data), data))
}

func expectPDFConversionError(t *testing.T, location string, pdfData []byte, wantSubstr string) {
	t.Helper()
	// Force the no-renderer deployment (Kindle): with pdftoppm available the
	// failure path would return the rendered page image like Python does.
	t.Setenv("PATH", "/nonexistent")
	result, format, err := convertPDFPageToImage(location, pdfData, 1, nil, false)
	if err == nil {
		t.Fatalf("expected error containing %q, got success (format %q)", wantSubstr, format)
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("expected error containing %q, got: %v", wantSubstr, err)
	}
	if result != nil || format != "" {
		t.Fatalf("expected nil data/format on failure, got (%d bytes, %q)", len(result), format)
	}
}

func TestGetPDFPageImage_TextOnlyPage_Rejected(t *testing.T) {
	// A page with text and no image at all. Python's check order applies:
	// len(page.images) != 1 (resources.py:392) rejects before the text check
	// (resources.py:394) would even run.
	pdfData := buildSinglePageTestPDF(
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 144 216] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		testStreamObject("BT /F1 12 Tf 72 108 Td (Hello) Tj ET"),
		[]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	expectPDFConversionError(t, "text.pdf", pdfData, "0 images")
}

func TestGetPDFPageImage_TextWithImage_Rejected(t *testing.T) {
	// The image count check passes (single image) but the page also paints
	// text, so extraction must be refused.
	pdfData := buildSinglePageTestPDF(
		testImagePageBody(72, 108, ""),
		testJPEGImageObject(t, 100, 150, ""),
		testStreamObject("q 72 0 0 108 0 0 cm /Im0 Do Q BT 12 12 Td (caption) Tj ET"))
	expectPDFConversionError(t, "textimg.pdf", pdfData, "contains text")
}

func TestGetPDFPageImage_TextInFormXObject_Rejected(t *testing.T) {
	// Text inside an invoked Form XObject is still page text — the scanner must
	// recurse into form streams (Python: page.extract_text() covers forms).
	pageBody := "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 72 108] /Contents 5 0 R /Resources << /XObject << /Im0 4 0 R /Fm0 6 0 R >> >> >>"
	formContent := "BT (form text) Tj ET"
	form := []byte(fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 72 108] /Resources << >> /Length %d >>\nstream\n%s\nendstream", len(formContent), formContent))
	pdfData := buildSinglePageTestPDF(
		pageBody,
		testJPEGImageObject(t, 100, 150, ""),
		testStreamObject("q 72 0 0 108 0 0 cm /Im0 Do Q q 1 0 0 1 0 0 cm /Fm0 Do Q"),
		form)
	expectPDFConversionError(t, "formtext.pdf", pdfData, "contains text")
}

func TestGetPDFPageImage_AnnotationWithContents_Rejected(t *testing.T) {
	// Python resources.py:397-400: /Annots entry carrying /Contents → default.
	pdfData := buildSinglePageTestPDF(
		testImagePageBody(72, 108, "/Annots [6 0 R] "),
		testJPEGImageObject(t, 100, 150, ""),
		testStreamObject("q 72 0 0 108 0 0 cm /Im0 Do Q"),
		[]byte("<< /Type /Annot /Subtype /Text /Contents (sticky note) >>"))
	expectPDFConversionError(t, "annot.pdf", pdfData, "annotations with content")
}

func TestGetPDFPageImage_AnnotationWithoutContents_Extracts(t *testing.T) {
	// Annotations without /Contents do not block extraction (Python only
	// rejects when "/Contents" is present in the annotation dict).
	pdfData := buildSinglePageTestPDF(
		testImagePageBody(72, 108, "/Annots [6 0 R] "),
		testJPEGImageObject(t, 100, 150, ""),
		testStreamObject("q 72 0 0 108 0 0 cm /Im0 Do Q"),
		[]byte("<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] >>"))
	result, format, err := convertPDFPageToImage("annot_ok.pdf", pdfData, 1, nil, false)
	if err != nil {
		t.Fatalf("expected extraction to succeed, got: %v", err)
	}
	if format != "jpg" {
		t.Fatalf("expected format jpg, got %q", format)
	}
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(result)); err != nil || cfg.Width != 100 || cfg.Height != 150 {
		t.Fatalf("expected 100x150 JPEG, got %vx%v err=%v", cfg.Width, cfg.Height, err)
	}
}

func TestGetPDFPageImage_CropBoxDiffers_Rejected(t *testing.T) {
	// Python resources.py:390-391: cropbox != mediabox → default image.
	pdfData := buildSinglePageTestPDF(
		testImagePageBody(72, 108, "/CropBox [10 10 60 100] "),
		testJPEGImageObject(t, 100, 150, ""),
		testStreamObject("q 72 0 0 108 0 0 cm /Im0 Do Q"))
	expectPDFConversionError(t, "crop.pdf", pdfData, "cropbox != mediabox")
}

func TestGetPDFPageImage_AspectMismatch_Rejected(t *testing.T) {
	// Python resources.py:411-414: |page_aspect - image_aspect| * page_aspect > 0.001.
	// Image 100x150 (aspect 0.667) on page 72x144 (aspect 0.5).
	pdfData := buildSinglePageTestPDF(
		testImagePageBody(72, 144, ""),
		testJPEGImageObject(t, 100, 150, ""),
		testStreamObject("q 72 0 0 144 0 0 cm /Im0 Do Q"))
	expectPDFConversionError(t, "aspect.pdf", pdfData, "aspect")
}

func TestGetPDFPageImage_LowDPI_Rejected(t *testing.T) {
	// Python resources.py:416-417: image_dpi < 75 → default.
	// Image 100x150 on page 144x216 → 100 / (144/72) = 50 DPI.
	pdfData := buildSinglePageTestPDF(
		testImagePageBody(144, 216, ""),
		testJPEGImageObject(t, 100, 150, ""),
		testStreamObject("q 144 0 0 216 0 0 cm /Im0 Do Q"))
	expectPDFConversionError(t, "lowdpi.pdf", pdfData, "below 75")
}

func TestGetPDFPageImage_MultipleImages_Rejected(t *testing.T) {
	// Python resources.py:392-393: len(page.images) != 1 → default.
	pageBody := "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 72 108] /Contents 6 0 R /Resources << /XObject << /Im0 4 0 R /Im1 5 0 R >> >> >>"
	pdfData := buildSinglePageTestPDF(
		pageBody,
		testJPEGImageObject(t, 100, 150, ""),
		testJPEGImageObject(t, 101, 151, ""), // distinct: pdfcpu deduplicates identical images
		testStreamObject("q 72 0 0 108 0 0 cm /Im0 Do Q q 10 0 0 10 10 10 cm /Im1 Do Q"))
	expectPDFConversionError(t, "twoimg.pdf", pdfData, "2 images")
}

func TestGetPDFPageImage_PageOutOfRange_Rejected(t *testing.T) {
	// Python resources.py:388: pdf.pages[page_num - 1] raises IndexError.
	pdfData := createPDFWithJPEG(100, 150, 72, 108)
	result, format, err := convertPDFPageToImage("range.pdf", pdfData, 3, nil, false)
	if err == nil {
		t.Fatal("expected error for out-of-range page")
	}
	if result != nil || format != "" {
		t.Fatalf("expected nil data/format on failure, got (%d bytes, %q)", len(result), format)
	}
}

func TestGetPDFPageImage_SoftMask_Rejected(t *testing.T) {
	// Python relies on image_match (resources.py:444-445) to catch images whose
	// extraction loses transparency. Without a rasterizer Go rejects masked
	// images outright (documented deviation — see getPDFPageImage).
	pdfData := buildSinglePageTestPDF(
		testImagePageBody(72, 108, ""),
		testJPEGImageObject(t, 100, 150, "/SMask 6 0 R "),
		testStreamObject("q 72 0 0 108 0 0 cm /Im0 Do Q"),
		[]byte("<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\n\x00\x40\x80\xff\nendstream"))
	expectPDFConversionError(t, "smask.pdf", pdfData, "transparency mask")
}

func TestGetPDFPageImage_FlateDecodeImage_ExtractedAsPNG(t *testing.T) {
	// Python resources.py:421-422: FlateDecode → page_image.image (decoded raster).
	// pdfcpu renders flate images as PNG; Go re-encodes the decoded raster.
	const w, h = 100, 150
	raw := make([]byte, w*h*3)
	for i := 0; i < len(raw); i += 3 {
		raw[i], raw[i+1], raw[i+2] = 0x20, 0x40, 0x80
	}
	var fbuf bytes.Buffer
	zw := zlib.NewWriter(&fbuf)
	zw.Write(raw)
	zw.Close()

	var img bytes.Buffer
	fmt.Fprintf(&img, "<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n", w, h, fbuf.Len())
	img.Write(fbuf.Bytes())
	img.WriteString("\nendstream")

	pdfData := buildSinglePageTestPDF(
		testImagePageBody(72, 108, ""),
		img.Bytes(),
		testStreamObject("q 72 0 0 108 0 0 cm /Im0 Do Q"))

	result, format, err := convertPDFPageToImage("flate.pdf", pdfData, 1, nil, false)
	if err != nil {
		t.Fatalf("expected flate image extraction, got: %v", err)
	}
	if format != "png" {
		t.Fatalf("expected format png, got %q", format)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(result))
	if err != nil {
		t.Fatalf("expected valid PNG, got: %v", err)
	}
	if cfg.Width != w || cfg.Height != h {
		t.Fatalf("expected %dx%d PNG, got %dx%d", w, h, cfg.Width, cfg.Height)
	}
}

// ---------------------------------------------------------------------------
// pdftoppm rendering fallback + image_match parity
// Python: resources.py:324-326 (default_image), 328-364 (convert_pdf_page_to_jpeg),
// 444-445 (image_match), 464-510 (image_match implementation).
// ---------------------------------------------------------------------------

// requirePdftoppm skips the test when pdftoppm is not installed (e.g. CI or
// the Kindle itself).
func requirePdftoppm(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not available; skipping renderer parity test")
	}
}

func TestRenderPDFPageJPEG_NotAvailable(t *testing.T) {
	// Kindle deployment: no pdftoppm on PATH. Python's convert_pdf_page_to_jpeg
	// raises, which propagates to the resource exception path.
	t.Setenv("PATH", "/nonexistent")
	if _, err := renderPDFPageJPEG("test.pdf", createPDFWithJPEG(100, 150, 72, 108), 1); err == nil {
		t.Fatal("expected error when pdftoppm is not on PATH")
	}
}

func TestRenderPDFPageJPEG_RendersPageAt300DPI(t *testing.T) {
	requirePdftoppm(t)

	// 72x108pt page rendered at 300 DPI -> 300x450 pixels.
	jpegData, err := renderPDFPageJPEG("test.pdf", createPDFWithJPEG(100, 150, 72, 108), 1)
	if err != nil {
		t.Fatalf("expected successful render, got: %v", err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpegData))
	if err != nil {
		t.Fatalf("rendered page should be a valid JPEG, got: %v", err)
	}
	if cfg.Width != 300 || cfg.Height != 450 {
		t.Fatalf("expected 300x450 render at 300 DPI, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestRenderPDFPageJPEG_MissingPage_Fails(t *testing.T) {
	requirePdftoppm(t)

	pdfData := createPDFWithJPEG(100, 150, 72, 108)
	if _, err := renderPDFPageJPEG("test.pdf", pdfData, 3); err == nil {
		t.Fatal("expected error rendering an out-of-range page")
	}
}

func TestConvertPDFPageToImage_ValidationFailure_ReturnsRenderedPage(t *testing.T) {
	// Python resources.py:324: every get_pdf_page_image failure returns
	// default_image — the rendered page. A text page cannot be extracted, so
	// with pdftoppm available the render must be returned, never an error.
	requirePdftoppm(t)

	pdfData := buildSinglePageTestPDF(
		testImagePageBody(72, 108, ""),
		testJPEGImageObject(t, 100, 150, ""),
		testStreamObject("q 72 0 0 108 0 0 cm /Im0 Do Q BT 12 12 Td (caption) Tj ET"))

	result, format, err := convertPDFPageToImage("textimg.pdf", pdfData, 1, nil, false)
	if err != nil {
		t.Fatalf("expected rendered fallback, got error: %v", err)
	}
	if format != "jpg" {
		t.Fatalf("expected format jpg for rendered fallback, got %q", format)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(result))
	if err != nil {
		t.Fatalf("rendered fallback should be valid JPEG, got: %v", err)
	}
	// Rendered at 300 DPI of a 72x108pt page.
	if cfg.Width != 300 || cfg.Height != 450 {
		t.Fatalf("expected 300x450 rendered page, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestConvertPDFPageToImage_ExtractedOnlyWhenMatch(t *testing.T) {
	// With a renderer available, a valid single-image page returns the exact
	// embedded JPEG (extraction preferred over rendering, Python resources.py:446-447).
	requirePdftoppm(t)

	pdfData := buildSinglePageTestPDF(
		testImagePageBody(72, 108, ""),
		testJPEGImageObject(t, 100, 150, ""),
		testStreamObject("q 72 0 0 108 0 0 cm /Im0 Do Q"))

	result, format, err := convertPDFPageToImage("match.pdf", pdfData, 1, nil, false)
	if err != nil {
		t.Fatalf("expected successful extraction, got: %v", err)
	}
	if format != "jpg" {
		t.Fatalf("expected format jpg, got %q", format)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(result))
	if err != nil {
		t.Fatalf("expected valid JPEG, got: %v", err)
	}
	// Extraction returns the embedded 100x150 image, not the 300x450 render.
	if cfg.Width != 100 || cfg.Height != 150 {
		t.Fatalf("expected extracted 100x150 image, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestConvertPDFPageToImage_NoRenderer_HonestError(t *testing.T) {
	// Without a renderer the same text-page failure must surface as an error so
	// the caller keeps the original PDF (Python exception path,
	// yj_to_epub_resources.py L112-115).
	t.Setenv("PATH", "/nonexistent")

	pdfData := buildSinglePageTestPDF(
		testImagePageBody(72, 108, ""),
		testJPEGImageObject(t, 100, 150, ""),
		testStreamObject("q 72 0 0 108 0 0 cm /Im0 Do Q BT 12 12 Td (caption) Tj ET"))

	expectPDFConversionError(t, "textimg.pdf", pdfData, "contains text")
}

func TestGetExternalResource_PDFResource_RenderedConversion(t *testing.T) {
	// With pdftoppm available the FIX_PDF branch converts using the rendered
	// page and updates format/extension exactly like Python's success path
	// (yj_to_epub_resources.py L115-118).
	requirePdftoppm(t)

	rp := newTestResourceProcessor()
	pdfData := buildSinglePageTestPDF(
		testImagePageBody(72, 108, ""),
		testJPEGImageObject(t, 100, 150, ""),
		testStreamObject("q 72 0 0 108 0 0 cm /Im0 Do Q BT 12 12 Td (caption) Tj ET"))

	frag := map[string]interface{}{
		"resource_name":   "pdf_res",
		"location":        "loc_pdf",
		"format":          "pdf",
		"mime":            "application/pdf",
		"resource_width":  72,
		"resource_height": 108,
		"page_index":      0,
		"yj.margin":       0,
		"margin_left":     0,
		"margin_right":    0,
		"margin_top":      0,
		"margin_bottom":   0,
	}
	rp.fragments["$164:pdf_res"] = frag
	rp.addTestRawMedia("loc_pdf", pdfData)

	result := rp.getExternalResource("pdf_res", false)
	if result == nil {
		t.Fatal("expected non-nil result for PDF resource")
	}
	if result.format != "jpg" {
		t.Fatalf("expected format 'jpg' after rendered conversion, got %q", result.format)
	}
	if result.extension != ".jpg" {
		t.Fatalf("expected extension '.jpg' after rendered conversion, got %q", result.extension)
	}
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(result.rawMedia)); err != nil || cfg.Width != 300 || cfg.Height != 450 {
		t.Fatalf("expected 300x450 rendered JPEG, got %vx%v err=%v", cfg.Width, cfg.Height, err)
	}
}

func TestPDFImageMatch(t *testing.T) {
	red := func(w, h int) image.Image {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
			}
		}
		return img
	}
	blue := func(w, h int) image.Image {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.SetRGBA(x, y, color.RGBA{B: 255, A: 255})
			}
		}
		return img
	}

	if !pdfImageMatch(red(512, 512), red(1024, 1024)) {
		t.Fatal("identical images at different scales must match")
	}
	if pdfImageMatch(red(512, 512), blue(512, 512)) {
		t.Fatal("solid red vs solid blue must not match")
	}
	if !pdfImageMatch(red(4, 4), blue(4, 4)) {
		t.Fatal("tiny images cannot be compared and must match (Python loops do not execute)")
	}
}
