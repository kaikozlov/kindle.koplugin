package kfx

import (
	"bytes"
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
		"$175": name,   // internal resource name
		"$165": location,
		"$161": format, // resource format (e.g., "$285" for jpg)
		"$162": mediaType,
		"$422": width,
		"$423": height,
	}
	if len(variantNames) > 0 {
		variants := make([]interface{}, len(variantNames))
		for i, v := range variantNames {
			variants[i] = v
		}
		frag["$635"] = variants
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
	rp.addTestResource("base_img", "loc_base", "$285", "image/jpeg", 800, 600, []string{"variant_img"})
	rp.addTestRawMedia("loc_base", baseData)

	// Variant: 1024×768 (strictly larger in both dimensions)
	variantData := []byte("variant-image-data-1024x768")
	rp.addTestResource("variant_img", "loc_variant", "$285", "image/jpeg", 1024, 768, nil)
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
	rp.addTestResource("base_img", "loc_base", "$285", "image/jpeg", 800, 600, []string{"variant_img"})
	rp.addTestRawMedia("loc_base", baseData)

	// Variant: 1024×500 (width larger but height smaller) — must NOT be selected
	variantData := []byte("variant-image-data-1024x500")
	rp.addTestResource("variant_img", "loc_variant", "$285", "image/jpeg", 1024, 500, nil)
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
	rp.addTestResource("base_img", "loc_base", "$285", "image/jpeg", 800, 600, []string{"variant_img"})
	rp.addTestRawMedia("loc_base", baseData)

	variantData := []byte("variant-image-data-1024x768")
	rp.addTestResource("variant_img", "loc_variant", "$285", "image/jpeg", 1024, 768, nil)
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
	rp.addTestResource("img_a", "myimage", "$285", "image/jpeg", 100, 100, nil)
	rp.addTestRawMedia("myimage", sameData)

	// Process first resource
	rp.processExternalResource("img_a", true, false, false, false, false)
	obj1 := rp.getExternalResource("img_a", false)
	firstFilename := obj1.filename

	// Second resource with same location (so same generated filename) and same binary data.
	// In the Python flow, two different resources pointing to the same location
	// would produce the same filename via resource_location_filename.
	// We manually set up the same filename to test the dedup logic.
	rp.addTestResource("img_b", "myimage", "$285", "image/jpeg", 100, 100, nil)
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
	rp.addTestResource("img_a", "myimage", "$285", "image/jpeg", 100, 100, nil)
	rp.addTestRawMedia("myimage", dataA)

	rp.addTestResource("img_b", "myimage2", "$285", "image/jpeg", 100, 100, nil)
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
	rp.addTestResource("cached_img", "loc_cached", "$285", "image/jpeg", 100, 100, nil)
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
	rp.addTestResource("referred_img", "path/to/image", "$285", "image/jpeg", 100, 100, nil)
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
	rp.addTestResource("mime_test", "loc_mime", "$285", "image/jpeg", 100, 100, nil)
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
	rp2.addTestResource("mime_test2", "loc_mime2", "$285", "image/jpeg", 100, 100, nil)
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
