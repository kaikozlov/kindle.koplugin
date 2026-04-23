package kfx

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// ---------------------------------------------------------------------------
// Helper: build a minimal fragmentCatalog with the fragment data needed for tests.
// ---------------------------------------------------------------------------

func makeTestCatalogForMetadata() *fragmentCatalog {
	return &fragmentCatalog{
		TitleMetadata:        nil,
		ContentFeatures:      map[string]interface{}{},
		DocumentData:         map[string]interface{}{},
		ReadingOrderMetadata: nil,
		ContentFragments:     map[string][]string{},
		Storylines:           map[string]map[string]interface{}{},
		StyleFragments:       map[string]map[string]interface{}{},
		SectionFragments:     map[string]sectionFragment{},
		AnchorFragments:      map[string]anchorFragment{},
		NavContainers:        map[string]map[string]interface{}{},
		NavRoots:             nil,
		ResourceFragments:    map[string]resourceFragment{},
		ResourceRawData:      map[string]map[string]interface{}{},
		FormatCapabilities:   map[string]map[string]interface{}{},
		Generators:           map[string]map[string]interface{}{},
		FontFragments:        map[string]fontFragment{},
		RawFragments:         map[string][]byte{},
		PositionAliases:      map[int]string{},
		FragmentIDsByType:    map[string][]string{},
	}
}

// ---------------------------------------------------------------------------
// VAL-B-041: getMetadataValue two-tier lookup — $490 first, $258 fallback
// ---------------------------------------------------------------------------

func TestGetMetadataValue_TwoTierLookup_FirstTierWins(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// Tier 1: $490 fragment with title = "Title A"
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_title_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "title",
						"value": "Title A",
					},
				},
			},
		},
	}

	// Tier 2: $258 fragment with title symbol = "Title B" (fallback)
	cat.ReadingOrderMetadata = map[string]interface{}{
		"title": "Title B",
	}

	result := getMetadataValue(cat, "title", "kindle_title_metadata", nil)
	if result != "Title A" {
		t.Errorf("expected 'Title A', got %v", result)
	}
}

func TestGetMetadataValue_TwoTierLookup_Fallback(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// No $490 fragment.
	cat.TitleMetadata = nil

	// Tier 2: $258 fragment with title symbol = "Title B"
	cat.ReadingOrderMetadata = map[string]interface{}{
		"title": "Title B",
	}

	result := getMetadataValue(cat, "title", "kindle_title_metadata", nil)
	if result != "Title B" {
		t.Errorf("expected 'Title B', got %v", result)
	}
}

func TestGetMetadataValue_Default(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	def := "default_val"
	result := getMetadataValue(cat, "nonexistent_key", "kindle_title_metadata", def)
	if result != "default_val" {
		t.Errorf("expected 'default_val', got %v", result)
	}
}

func TestGetMetadataValue_NilDefault(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	result := getMetadataValue(cat, "nonexistent_key", "kindle_title_metadata", nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

// ---------------------------------------------------------------------------
// VAL-B-042: get_metadata_value with non-default category
// ---------------------------------------------------------------------------

func TestGetMetadataValue_NonDefaultCategory(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_title_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "yj_fixed_layout",
						"value": "should_not_match",
					},
				},
			},
			map[string]interface{}{
				"category": "kindle_capability_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "yj_fixed_layout",
						"value": 2,
					},
				},
			},
		},
	}

	result := getMetadataValue(cat, "yj_fixed_layout", "kindle_capability_metadata", nil)
	if result != 2 {
		t.Errorf("expected 2, got %v", result)
	}
}

// ---------------------------------------------------------------------------
// VAL-B-043: getFeatureValue with com.amazon.yjconversion namespace
// ---------------------------------------------------------------------------

func TestGetFeatureValue_YJConversionNamespace_MajorOnly(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	cat.ContentFeatures = map[string]interface{}{
		"features": []interface{}{
			map[string]interface{}{
				"namespace": "com.amazon.yjconversion",
				"key": "yj.illustrated_layout",
				"version_info": map[string]interface{}{
					"version": map[string]interface{}{
						"major_version": 2,
						"minor_version": 0,
					},
				},
			},
		},
	}

	result := getFeatureValue(cat, "yj.illustrated_layout", "com.amazon.yjconversion", nil)
	if result != 2 {
		t.Errorf("expected 2, got %v", result)
	}
}

func TestGetFeatureValue_YJConversionNamespace_MajorMinor(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	cat.ContentFeatures = map[string]interface{}{
		"features": []interface{}{
			map[string]interface{}{
				"namespace": "com.amazon.yjconversion",
				"key": "yj.illustrated_layout",
				"version_info": map[string]interface{}{
					"version": map[string]interface{}{
						"major_version": 2,
						"minor_version": 3,
					},
				},
			},
		},
	}

	result := getFeatureValue(cat, "yj.illustrated_layout", "com.amazon.yjconversion", nil)
	tuple, ok := result.([2]int)
	if !ok {
		t.Fatalf("expected [2]int tuple, got %T: %v", result, result)
	}
	if tuple[0] != 2 || tuple[1] != 3 {
		t.Errorf("expected (2, 3), got (%d, %d)", tuple[0], tuple[1])
	}
}

// ---------------------------------------------------------------------------
// VAL-B-044: getFeatureValue with format_capabilities namespace
// ---------------------------------------------------------------------------

func TestGetFeatureValue_FormatCapabilities(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// $593 fragments stored by their fragment IDs.
	// Python iterates fragment.value directly as a list of IonStruct.
	// In Go, the decoded IonList is stored under "content_list" key.
	cat.FormatCapabilities = map[string]map[string]interface{}{
		"fc1": {
			"content_list": []interface{}{
				map[string]interface{}{
					"key":    "some_feat",
					"version": "1.2",
				},
			},
		},
	}

	result := getFeatureValue(cat, "some_feat", "format_capabilities", nil)
	if result != "1.2" {
		t.Errorf("expected '1.2', got %v", result)
	}
}

func TestGetFeatureValue_Default(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	def := "fallback"
	result := getFeatureValue(cat, "nonexistent", "com.amazon.yjconversion", def)
	if result != "fallback" {
		t.Errorf("expected 'fallback', got %v", result)
	}
}

// ---------------------------------------------------------------------------
// VAL-B-045: isMagazine returns true when cde_type is "MAGZ"
// ---------------------------------------------------------------------------

func TestIsMagazine_True(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_title_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "cde_content_type",
						"value": "MAGZ",
					},
				},
			},
		},
	}

	ci := newCacheInfo()
	if !isMagazine(cat, ci) {
		t.Error("expected isMagazine=true for MAGZ")
	}
}

func TestIsMagazine_False(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_title_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "cde_content_type",
						"value": "EBOK",
					},
				},
			},
		},
	}

	ci := newCacheInfo()
	if isMagazine(cat, ci) {
		t.Error("expected isMagazine=false for EBOK")
	}
}

func TestIsMagazine_Empty(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	ci := newCacheInfo()
	if isMagazine(cat, ci) {
		t.Error("expected isMagazine=false when no cde_content_type")
	}
}

// ---------------------------------------------------------------------------
// VAL-B-046: isSample returns true when cde_type is "EBSP"
// ---------------------------------------------------------------------------

func TestIsSample_True(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_title_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "cde_content_type",
						"value": "EBSP",
					},
				},
			},
		},
	}

	ci := newCacheInfo()
	if !isSample(cat, ci) {
		t.Error("expected isSample=true for EBSP")
	}
}

func TestIsSample_False(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_title_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "cde_content_type",
						"value": "EBOK",
					},
				},
			},
		},
	}

	ci := newCacheInfo()
	if isSample(cat, ci) {
		t.Error("expected isSample=false for EBOK")
	}
}

// ---------------------------------------------------------------------------
// VAL-B-047: isFixedLayout checks scribe_notebook OR yj_fixed_layout capability
// ---------------------------------------------------------------------------

func TestIsFixedLayout_WithCapability(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_capability_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "yj_fixed_layout",
						"value": 1,
					},
				},
			},
		},
	}

	ci := newCacheInfo()
	if !isFixedLayout(cat, false, ci) {
		t.Error("expected isFixedLayout=true with yj_fixed_layout capability")
	}
}

func TestIsFixedLayout_ScribeNotebook(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	ci := newCacheInfo()
	if !isFixedLayout(cat, true, ci) {
		t.Error("expected isFixedLayout=true for scribe notebook")
	}
}

func TestIsFixedLayout_False(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	ci := newCacheInfo()
	if isFixedLayout(cat, false, ci) {
		t.Error("expected isFixedLayout=false when no fixed layout capability")
	}
}

// ---------------------------------------------------------------------------
// VAL-B-048: isPrintReplica logic
// ---------------------------------------------------------------------------

func TestIsPrintReplica_FixedLayoutEquals2(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_capability_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "yj_fixed_layout",
						"value": 2,
					},
				},
			},
		},
	}

	ci := newCacheInfo()
	if !isPrintReplica(cat, ci) {
		t.Error("expected isPrintReplica=true when yj_fixed_layout==2")
	}
}

func TestIsPrintReplica_TextbookAndNot3(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_capability_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "yj_textbook",
						"value": 1,
					},
					map[string]interface{}{
						"key": "yj_fixed_layout",
						"value": 1,
					},
				},
			},
		},
	}

	ci := newCacheInfo()
	if !isPrintReplica(cat, ci) {
		t.Error("expected isPrintReplica=true when yj_textbook exists and yj_fixed_layout!=3")
	}
}

func TestIsPrintReplica_TextbookAnd3_False(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_capability_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "yj_textbook",
						"value": 1,
					},
					map[string]interface{}{
						"key": "yj_fixed_layout",
						"value": 3,
					},
				},
			},
		},
	}

	ci := newCacheInfo()
	if isPrintReplica(cat, ci) {
		t.Error("expected isPrintReplica=false when yj_fixed_layout==3")
	}
}

func TestIsPrintReplica_NoTextbookNoFixedLayout_False(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	ci := newCacheInfo()
	if isPrintReplica(cat, ci) {
		t.Error("expected isPrintReplica=false when no textbook and no fixed_layout")
	}
}

func TestIsPrintReplica_TextbookAndNil(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_capability_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "yj_textbook",
						"value": 1,
					},
				},
			},
		},
	}

	ci := newCacheInfo()
	// yj_textbook exists and yj_fixed_layout is nil (!=3), so true
	if !isPrintReplica(cat, ci) {
		t.Error("expected isPrintReplica=true when yj_textbook exists and yj_fixed_layout is nil")
	}
}

// ---------------------------------------------------------------------------
// VAL-B-049: isImageBasedFixedLayout (tests getOrderedImageResources succeeds)
// ---------------------------------------------------------------------------

func TestIsImageBasedFixedLayout_True(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	// Need to make getOrderedImageResources succeed.
	// That requires isFixedLayout=true AND at least one image resource in section content.
	// Since the test catalog has no fixed-layout metadata and no section content with images,
	// isImageBasedFixedLayout will return false.
	cat.ResourceFragments["img1"] = resourceFragment{ID: "img1", Location: "loc1"}
	ci := newCacheInfo()
	result := isImageBasedFixedLayout(cat, ci)
	// Without fixed-layout metadata and actual image content in sections,
	// isImageBasedFixedLayout returns false.
	if result {
		t.Error("expected isImageBasedFixedLayout=false — no fixed-layout metadata and no image content in sections")
	}
}

func TestIsImageBasedFixedLayout_False(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	ci := newCacheInfo()
	if isImageBasedFixedLayout(cat, ci) {
		t.Error("expected isImageBasedFixedLayout=false when no resources")
	}
}

// ---------------------------------------------------------------------------
// VAL-B-050: Boolean property caching pattern
// ---------------------------------------------------------------------------

func TestCachingPattern_CdeType(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_title_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "cde_content_type",
						"value": "MAGZ",
					},
				},
			},
		},
	}

	ci := newCacheInfo()

	// Call twice, second call should use cached value
	r1 := isMagazine(cat, ci)
	r2 := isMagazine(cat, ci)
	if !r1 || !r2 {
		t.Errorf("expected both calls true, got %v then %v", r1, r2)
	}
}

func TestCachingPattern_IsKfxV1(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.Generators = map[string]map[string]interface{}{
		"gen1": {"version": 1},
	}
	cat.FragmentIDsByType["container"] = []string{"gen1"}

	ci := newCacheInfo()
	r1 := isKfxV1(cat, ci)
	r2 := isKfxV1(cat, ci)
	if !r1 || !r2 {
		t.Errorf("expected both calls true, got %v then %v", r1, r2)
	}
}

// ---------------------------------------------------------------------------
// VAL-B-051: is_kfx_v1 checks $270 fragment version field
// ---------------------------------------------------------------------------

func TestIsKfxV1_True(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.Generators = map[string]map[string]interface{}{
		"gen1": {"version": 1},
	}
	cat.FragmentIDsByType["container"] = []string{"gen1"}
	ci := newCacheInfo()
	if !isKfxV1(cat, ci) {
		t.Error("expected isKfxV1=true when version=1")
	}
}

func TestIsKfxV1_Version2_False(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.Generators = map[string]map[string]interface{}{
		"gen1": {"version": 2},
	}
	ci := newCacheInfo()
	if isKfxV1(cat, ci) {
		t.Error("expected isKfxV1=false when version=2")
	}
}

func TestIsKfxV1_NoFragment_False(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	ci := newCacheInfo()
	if isKfxV1(cat, ci) {
		t.Error("expected isKfxV1=false when no $270 fragment")
	}
}

// ---------------------------------------------------------------------------
// VAL-B-052: getCoverImageData follows resource chain
// ---------------------------------------------------------------------------

func TestGetCoverImageData_JPEG(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// Set cover_image metadata
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_title_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "cover_image",
						"value": "cover_res_1",
					},
				},
			},
		},
	}

	// Resource fragment with jpg format
	cat.ResourceFragments["cover_res_1"] = resourceFragment{
		ID:        "cover_res_1",
		Location:  "cover_loc",
		MediaType: "image/jpeg",
	}
	// Store the format info in the raw fragment data
	cat.ResourceRawData["cover_res_1"] = map[string]interface{}{
		"format": "jpg", // jpg format symbol
		"location": "cover_loc",
	}

	// Raw media
	cat.RawFragments["cover_loc"] = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46}

	result := getCoverImageData(cat)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Format != "jpeg" {
		t.Errorf("expected format 'jpeg', got '%s'", result.Format)
	}
	if len(result.Data) != 8 {
		t.Errorf("expected 8 bytes, got %d", len(result.Data))
	}
}

func TestGetCoverImageData_NoCoverMetadata_Nil(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	result := getCoverImageData(cat)
	if result != nil {
		t.Error("expected nil when no cover_image metadata")
	}
}

func TestGetCoverImageData_NoRawMedia_Nil(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_title_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "cover_image",
						"value": "cover_res_1",
					},
				},
			},
		},
	}

	cat.ResourceFragments["cover_res_1"] = resourceFragment{
		ID:       "cover_res_1",
		Location: "cover_loc",
	}
	cat.ResourceRawData["cover_res_1"] = map[string]interface{}{
		"format": "jpg",
		"location": "cover_loc",
	}
	// No raw media for cover_loc

	result := getCoverImageData(cat)
	if result != nil {
		t.Error("expected nil when no raw media")
	}
}

// ---------------------------------------------------------------------------
// VAL-B-053: getAssetId delegates to getMetadataValue
// ---------------------------------------------------------------------------

func TestGetAssetId(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_title_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "asset_id",
						"value": "asset_123",
					},
				},
			},
		},
	}

	result := getAssetID(cat)
	if result != "asset_123" {
		t.Errorf("expected 'asset_123', got %v", result)
	}
}

func TestGetAssetId_NotFound(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	result := getAssetID(cat)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

// ---------------------------------------------------------------------------
// VAL-B-056: hasMetadata checks for $490 or $258 fragments
// ---------------------------------------------------------------------------

func TestHasMetadata_WithTitleMetadata(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{"categorised_metadata": nil}
	if !hasMetadata(cat) {
		t.Error("expected true when $490 exists")
	}
}

func TestHasMetadata_WithReadingOrderMetadata(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.ReadingOrderMetadata = map[string]interface{}{"title": "test"}
	if !hasMetadata(cat) {
		t.Error("expected true when $258 exists")
	}
}

func TestHasMetadata_Both(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{}
	cat.ReadingOrderMetadata = map[string]interface{}{}
	if !hasMetadata(cat) {
		t.Error("expected true when both exist")
	}
}

func TestHasMetadata_Neither(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	if hasMetadata(cat) {
		t.Error("expected false when neither exists")
	}
}

// ---------------------------------------------------------------------------
// VAL-B-057: hasPDFResource scans $164 fragments for $565 format
// ---------------------------------------------------------------------------

func TestHasPDFResource_True(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.ResourceRawData["res1"] = map[string]interface{}{
		"format": "pdf", // PDF format
	}
	ci := newCacheInfo()
	if !hasPDFResource(cat, ci) {
		t.Error("expected true when $164 has $565 format")
	}
}

func TestHasPDFResource_False(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.ResourceRawData["res1"] = map[string]interface{}{
		"format": "jxr", // JPEG-XR format
	}
	ci := newCacheInfo()
	if hasPDFResource(cat, ci) {
		t.Error("expected false for non-PDF format")
	}
}

func TestHasPDFResource_NoResources(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	ci := newCacheInfo()
	if hasPDFResource(cat, ci) {
		t.Error("expected false when no resources")
	}
}

// ---------------------------------------------------------------------------
// VAL-B-058: getGenerators returns (name, version) tuples from $270
// ---------------------------------------------------------------------------

func TestGetGenerators(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.Generators = map[string]map[string]interface{}{
		"gen1": {
			"major_version":    "GeneratorA",
			"minor_version":    "1.0",
			"version": 1, // Python: if "version" in fragment.value
		},
		"gen2": {
			"major_version":    "GeneratorB",
			"minor_version":    "2.0",
			"version": 2,
		},
	}

	gens := getGenerators(cat)
	if len(gens) != 2 {
		t.Fatalf("expected 2 generators, got %d", len(gens))
	}

	genMap := map[string]string{}
	for _, g := range gens {
		genMap[g.Name] = g.Version
	}
	if genMap["GeneratorA"] != "1.0" {
		t.Errorf("expected GeneratorA/1.0, got %s", genMap["GeneratorA"])
	}
	if genMap["GeneratorB"] != "2.0" {
		t.Errorf("expected GeneratorB/2.0, got %s", genMap["GeneratorB"])
	}
}

func TestGetGenerators_Empty(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	gens := getGenerators(cat)
	if len(gens) != 0 {
		t.Errorf("expected 0 generators, got %d", len(gens))
	}
}

// ---------------------------------------------------------------------------
// isPDFBackedFixedLayout
// ---------------------------------------------------------------------------

func TestIsPDFBackedFixedLayout_True(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_capability_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "yj_fixed_layout",
						"value": 3,
					},
				},
			},
		},
	}

	ci := newCacheInfo()
	if !isPDFBackedFixedLayout(cat, ci) {
		t.Error("expected true when yj_fixed_layout==3")
	}
}

func TestIsPDFBackedFixedLayout_False(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_capability_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "yj_fixed_layout",
						"value": 1,
					},
				},
			},
		},
	}

	ci := newCacheInfo()
	if isPDFBackedFixedLayout(cat, ci) {
		t.Error("expected false when yj_fixed_layout==1")
	}
}

// ---------------------------------------------------------------------------
// isIllustratedLayout
// ---------------------------------------------------------------------------

func TestIsIllustratedLayout_True(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.ContentFeatures = map[string]interface{}{
		"features": []interface{}{
			map[string]interface{}{
				"namespace": "com.amazon.yjconversion",
				"key": "yj.illustrated_layout",
				"version_info": map[string]interface{}{
					"version": map[string]interface{}{
						"major_version": 1,
						"minor_version": 0,
					},
				},
			},
		},
	}

	ci := newCacheInfo()
	if !isIllustratedLayout(cat, ci) {
		t.Error("expected true when illustrated layout feature exists")
	}
}

func TestIsIllustratedLayout_False(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	ci := newCacheInfo()
	if isIllustratedLayout(cat, ci) {
		t.Error("expected false when no illustrated layout feature")
	}
}

// ---------------------------------------------------------------------------
// cdeType
// ---------------------------------------------------------------------------

func TestCdeType(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.TitleMetadata = map[string]interface{}{
		"categorised_metadata": []interface{}{
			map[string]interface{}{
				"category": "kindle_title_metadata",
				"metadata": []interface{}{
					map[string]interface{}{
						"key": "cde_content_type",
						"value": "EBOK",
					},
				},
			},
		},
	}

	ci := newCacheInfo()
	result := cdeType(cat, ci)
	if result != "EBOK" {
		t.Errorf("expected 'EBOK', got %v", result)
	}
}

// ---------------------------------------------------------------------------
// hasCoverData
// ---------------------------------------------------------------------------

func TestHasCoverData_False(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	if hasCoverData(cat) {
		t.Error("expected false when no cover data")
	}
}

// ---------------------------------------------------------------------------
// get_page_count
// ---------------------------------------------------------------------------

func TestGetPageCount(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.NavRoots = []map[string]interface{}{
		{
			"nav_containers": []interface{}{
				map[string]interface{}{
					"nav_type": "page_list",
					"entries": []interface{}{"p1", "p2", "p3"},
				},
			},
		},
	}

	count := getPageCount(cat)
	if count != 3 {
		t.Errorf("expected 3 pages, got %d", count)
	}
}

func TestGetPageCount_NoNavRoot(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	count := getPageCount(cat)
	if count != 0 {
		t.Errorf("expected 0 pages, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// FIX 1: getFeatureValue format_capabilities iterates $593 correctly
// Python yj_metadata.py:374 iterates fragment.value directly as a list.
// In Go, the decoded $593 fragment has its list entries under "content_list" key.
// ---------------------------------------------------------------------------

func TestGetFeatureValue_FormatCapabilities_MultipleFeatures(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	cat.FormatCapabilities = map[string]map[string]interface{}{
		"fc1": {
			"content_list": []interface{}{
				map[string]interface{}{
					"key":    "feat_a",
					"version": "1.0",
				},
				map[string]interface{}{
					"key":    "feat_b",
					"version": "2.0",
				},
			},
		},
	}

	result := getFeatureValue(cat, "feat_a", "format_capabilities", nil)
	if result != "1.0" {
		t.Errorf("expected '1.0' for feat_a, got %v", result)
	}

	result = getFeatureValue(cat, "feat_b", "format_capabilities", nil)
	if result != "2.0" {
		t.Errorf("expected '2.0' for feat_b, got %v", result)
	}
}

func TestGetFeatureValue_FormatCapabilities_FallbackToItems(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// Backward compat: also accept "items" key
	cat.FormatCapabilities = map[string]map[string]interface{}{
		"fc1": {
			"items": []interface{}{
				map[string]interface{}{
					"key":    "legacy_feat",
					"version": "0.9",
				},
			},
		},
	}

	result := getFeatureValue(cat, "legacy_feat", "format_capabilities", nil)
	if result != "0.9" {
		t.Errorf("expected '0.9' for legacy_feat, got %v", result)
	}
}

func TestGetFeatureValue_FormatCapabilities_NotFound(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	cat.FormatCapabilities = map[string]map[string]interface{}{
		"fc1": {
			"content_list": []interface{}{
				map[string]interface{}{
					"key":    "other_feat",
					"version": "3.0",
				},
			},
		},
	}

	result := getFeatureValue(cat, "nonexistent", "format_capabilities", "default")
	if result != "default" {
		t.Errorf("expected default, got %v", result)
	}
}

func TestGetFeatureValue_FormatCapabilities_MultipleFragments(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	cat.FormatCapabilities = map[string]map[string]interface{}{
		"fc1": {
			"content_list": []interface{}{
				map[string]interface{}{
					"key":    "feat_one",
					"version": "1.0",
				},
			},
		},
		"fc2": {
			"content_list": []interface{}{
				map[string]interface{}{
					"key":    "feat_two",
					"version": "2.0",
				},
			},
		},
	}

	result := getFeatureValue(cat, "feat_two", "format_capabilities", nil)
	if result != "2.0" {
		t.Errorf("expected '2.0' for feat_two in second fragment, got %v", result)
	}
}

func TestGetFeatureValue_FormatCapabilities_EmptyFragment(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// Fragment with no $146 or items — should skip gracefully
	cat.FormatCapabilities = map[string]map[string]interface{}{
		"fc1": {},
	}

	result := getFeatureValue(cat, "any_feat", "format_capabilities", "default")
	if result != "default" {
		t.Errorf("expected default for empty fragment, got %v", result)
	}
}

// ---------------------------------------------------------------------------
// FIX 2: updateCoverSectionAndStoryline processes $157 style fragments
// Python yj_metadata.py:813-814 recursively processes style fragments via:
//   if "style" in content:
//       process_content(self.fragments.get(ftype="style", fid=content.get("style")).value, desc)
// ---------------------------------------------------------------------------

func TestUpdateCoverSectionAndStoryline_StyleFragmentProcessing(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// Set up section with a page template that references a $157 style
	cat.SectionOrder = []string{"cover_section"}
	cat.SectionFragments["cover_section"] = sectionFragment{
		ID:   "cover_section",
		Storyline: "story1",
		PageTemplateValues: map[string]interface{}{
			"width":  800,
			"height":  600,
			"style": "style_123",
			"content_list": []interface{}{
				map[string]interface{}{
					"width":  800,
					"fixed_width":  800,
					"style": "style_child",
				},
			},
		},
	}

	// Set up style fragments
	cat.StyleFragments["style_123"] = map[string]interface{}{
		"width": 800,
		"height": 600,
	}
	cat.StyleFragments["style_child"] = map[string]interface{}{
		"fixed_width": 800,
	}

	// Set up storyline
	cat.Storylines["story1"] = map[string]interface{}{
		"content_list": []interface{}{
			map[string]interface{}{
				"width":  800,
				"style": "style_story",
			},
		},
	}
	cat.StyleFragments["style_story"] = map[string]interface{}{
		"width": 800,
	}

	// Update dimensions from 800x600 to 1024x768
	updateCoverSectionAndStoryline(cat, 800, 600, 1024, 768)

	// Check page template dimensions were updated
	ptv := cat.SectionFragments["cover_section"].PageTemplateValues
	if ptv["width"] != 1024 {
		t.Errorf("expected page template $56=1024, got %v", ptv["width"])
	}
	if ptv["height"] != 768 {
		t.Errorf("expected page template $57=768, got %v", ptv["height"])
	}

	// Check $157 style fragment was updated
	if cat.StyleFragments["style_123"]["width"] != 1024 {
		t.Errorf("expected style_123 $56=1024, got %v", cat.StyleFragments["style_123"]["width"])
	}
	if cat.StyleFragments["style_123"]["height"] != 768 {
		t.Errorf("expected style_123 $57=768, got %v", cat.StyleFragments["style_123"]["height"])
	}

	// Check child's $157 style fragment was also updated
	if cat.StyleFragments["style_child"]["fixed_width"] != 1024 {
		t.Errorf("expected style_child $66=1024, got %v", cat.StyleFragments["style_child"]["fixed_width"])
	}

	// Check storyline child's $157 style fragment was updated
	if cat.StyleFragments["style_story"]["width"] != 1024 {
		t.Errorf("expected style_story $56=1024, got %v", cat.StyleFragments["style_story"]["width"])
	}
}

func TestUpdateCoverSectionAndStoryline_NoStyleRef(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// Section without $157 references — should still update dimensions
	cat.SectionOrder = []string{"cover_section"}
	cat.SectionFragments["cover_section"] = sectionFragment{
		ID:        "cover_section",
		Storyline: "",
		PageTemplateValues: map[string]interface{}{
			"width": 800,
			"height": 600,
		},
	}

	updateCoverSectionAndStoryline(cat, 800, 600, 1024, 768)

	ptv := cat.SectionFragments["cover_section"].PageTemplateValues
	if ptv["width"] != 1024 {
		t.Errorf("expected $56=1024, got %v", ptv["width"])
	}
	if ptv["height"] != 768 {
		t.Errorf("expected $57=768, got %v", ptv["height"])
	}
}

func TestUpdateCoverSectionAndStoryline_NoSections(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	// No sections — should not panic
	updateCoverSectionAndStoryline(cat, 800, 600, 1024, 768)
}

func TestUpdateCoverSectionAndStoryline_MissingStyleFragment(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// Section references a $157 style that doesn't exist — should skip gracefully
	cat.SectionOrder = []string{"cover_section"}
	cat.SectionFragments["cover_section"] = sectionFragment{
		ID:        "cover_section",
		Storyline: "",
		PageTemplateValues: map[string]interface{}{
			"width":  800,
			"style": "nonexistent_style",
		},
	}

	updateCoverSectionAndStoryline(cat, 800, 600, 1024, 768)

	ptv := cat.SectionFragments["cover_section"].PageTemplateValues
	if ptv["width"] != 1024 {
		t.Errorf("expected $56=1024, got %v", ptv["width"])
	}
}

func TestUpdateCoverSectionAndStoryline_NestedStyleFragments(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// Style fragment that itself references another style fragment
	cat.SectionOrder = []string{"cover_section"}
	cat.SectionFragments["cover_section"] = sectionFragment{
		ID:        "cover_section",
		Storyline: "",
		PageTemplateValues: map[string]interface{}{
			"style": "style_outer",
		},
	}
	cat.StyleFragments["style_outer"] = map[string]interface{}{
		"width":  800,
		"style": "style_inner",
	}
	cat.StyleFragments["style_inner"] = map[string]interface{}{
		"height": 600,
	}

	updateCoverSectionAndStoryline(cat, 800, 600, 1024, 768)

	// Both style fragments should be updated recursively
	if cat.StyleFragments["style_outer"]["width"] != 1024 {
		t.Errorf("expected style_outer $56=1024, got %v", cat.StyleFragments["style_outer"]["width"])
	}
	if cat.StyleFragments["style_inner"]["height"] != 768 {
		t.Errorf("expected style_inner $57=768, got %v", cat.StyleFragments["style_inner"]["height"])
	}
}

// ===========================================================================
// NEW TESTS: Fix 5 metadata getter inconsistencies (VAL-M1-META-001 to 005)
// ===========================================================================

// ---------------------------------------------------------------------------
// VAL-M1-META-003: getGenerators filters PACKAGE_VERSION_PLACEHOLDERS
// Python yj_metadata.py:393-398
// ---------------------------------------------------------------------------

func TestGetGenerators_PlaceholderFiltering(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.Generators = map[string]map[string]interface{}{
		"gen1": {
			"major_version":    "NormalGen",
			"minor_version":    "1.0.0",
			"version": 1,
		},
		"gen2": {
			"major_version":    "PlaceholderGen",
			"minor_version":    "kfxlib-00000000", // PACKAGE_VERSION_PLACEHOLDER
			"version": 1,
		},
		"gen3": {
			"major_version":    "AnotherPlaceholder",
			"minor_version":    "PackageVersion:YJReaderSDK-1.0.x.x GitSHA:c805492 Month-Day:04-22",
			"version": 2,
		},
	}

	gens := getGenerators(cat)
	if len(gens) != 3 {
		t.Fatalf("expected 3 generators, got %d", len(gens))
	}

	genMap := map[string]string{}
	for _, g := range gens {
		genMap[g.Name] = g.Version
	}

	// Normal version should be preserved
	if genMap["NormalGen"] != "1.0.0" {
		t.Errorf("expected NormalGen version '1.0.0', got '%s'", genMap["NormalGen"])
	}

	// Placeholder versions should be filtered to ""
	if genMap["PlaceholderGen"] != "" {
		t.Errorf("expected PlaceholderGen version '' (placeholder filtered), got '%s'", genMap["PlaceholderGen"])
	}
	if genMap["AnotherPlaceholder"] != "" {
		t.Errorf("expected AnotherPlaceholder version '' (placeholder filtered), got '%s'", genMap["AnotherPlaceholder"])
	}
}

func TestGetGenerators_NoVersionKey_Skipped(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.Generators = map[string]map[string]interface{}{
		"gen1": {
			"major_version": "NoVersionGen",
			"minor_version": "1.0.0",
			// No "version" key — should be skipped
		},
	}

	gens := getGenerators(cat)
	if len(gens) != 0 {
		t.Errorf("expected 0 generators (no version key), got %d", len(gens))
	}
}

func TestGetGenerators_EmptyPkgVersion(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.Generators = map[string]map[string]interface{}{
		"gen1": {
			"major_version":    "GenNoPkg",
			"version": 1,
			// No $588 key — asString returns ""
		},
	}

	gens := getGenerators(cat)
	if len(gens) != 1 {
		t.Fatalf("expected 1 generator, got %d", len(gens))
	}
	if gens[0].Version != "" {
		t.Errorf("expected empty version for missing $588, got '%s'", gens[0].Version)
	}
}

// ---------------------------------------------------------------------------
// VAL-M1-META-004: isKfxV1 checks $270 fragment version field correctly
// Python yj_metadata.py:338-341: fragment = self.fragments.get("container", first=True)
//   fragment.value.get("version", 0) == 1 if fragment is not None else False
// ---------------------------------------------------------------------------

func TestIsKfxV1_FirstFragmentOnly(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// Two $270 fragments: first has version=2, second has version=1
	// Python uses first=True, so only the first fragment is checked
	cat.Generators = map[string]map[string]interface{}{
		"gen_first": {"version": 2},
		"gen_second": {"version": 1},
	}
	cat.FragmentIDsByType["container"] = []string{"gen_first", "gen_second"}

	ci := newCacheInfo()
	// Should check only the first fragment (version=2), result should be false
	if isKfxV1(cat, ci) {
		t.Error("expected false: first $270 has version=2, not 1")
	}
}

func TestIsKfxV1_DefaultVersionZero(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// Fragment without explicit "version" key → defaults to 0
	cat.Generators = map[string]map[string]interface{}{
		"gen1": {"major_version": "SomeGen"},
	}
	cat.FragmentIDsByType["container"] = []string{"gen1"}

	ci := newCacheInfo()
	// Python: fragment.value.get("version", 0) == 1 → 0 == 1 → false
	if isKfxV1(cat, ci) {
		t.Error("expected false when version key absent (default 0)")
	}
}

func TestIsKfxV1_VersionOneIsTrue(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.Generators = map[string]map[string]interface{}{
		"gen1": {"version": 1},
	}
	cat.FragmentIDsByType["container"] = []string{"gen1"}

	ci := newCacheInfo()
	if !isKfxV1(cat, ci) {
		t.Error("expected true when first $270 fragment version=1")
	}
}

// ---------------------------------------------------------------------------
// VAL-M1-META-002: isImageBasedFixedLayout calls getOrderedImageResources
// Python yj_metadata.py:286-296:
//   try: self.get_ordered_image_resources()
//   except Exception: cached = False
//   else: cached = True
//
// The function returns true ONLY if getOrderedImageResources succeeds.
// getOrderedImageResources now calls CollectContentPositionInfo to walk section content.
// This test verifies the correct calling pattern.
// ---------------------------------------------------------------------------

func TestIsImageBasedFixedLayout_NonFixedLayoutReturnsFalse(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	// Non-fixed-layout book with resources — should return false because
	// getOrderedImageResources checks is_fixed_layout first
	cat.ResourceFragments["img1"] = resourceFragment{ID: "img1", Location: "loc1"}
	ci := newCacheInfo()
	if isImageBasedFixedLayout(cat, ci) {
		t.Error("expected false: book is not fixed-layout (no yj_fixed_layout capability)")
	}
}

func TestIsImageBasedFixedLayout_FixedLayoutNoImagesReturnsFalse(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	// Fixed-layout book but no images — getOrderedImageResources will fail
	// because there are no image resources in section content
	ci := newCacheInfo()
	// getOrderedImageResources will fail because there are no image resources
	// and the book is not fixed layout (no yj_fixed_layout metadata)
	if isImageBasedFixedLayout(cat, ci) {
		t.Error("expected false: fixed-layout but no image resources")
	}
}

func TestIsImageBasedFixedLayout_DelegatesToGetOrderedImageResources(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	// Add resource fragments but no section structure for getOrderedImageResources to find images
	cat.ResourceFragments["img1"] = resourceFragment{ID: "img1", Location: "loc1"}

	ci := newCacheInfo()
	// getOrderedImageResources will fail because there are no image resources in section content,
	// so even with resource fragments present, isImageBasedFixedLayout returns false.
	// This verifies we delegate to gOIR instead of just checking len(ResourceFragments) > 0.
	result := isImageBasedFixedLayout(cat, ci)
	if result {
		t.Error("expected false: gOIR returns error for no image resources in section content")
	}
}

// ---------------------------------------------------------------------------
// VAL-M1-META-005: fixCoverImageData re-encodes JFIF JPEG covers
// Python yj_metadata.py:556-578
// ---------------------------------------------------------------------------

func TestFixCoverImageData_NilInput(t *testing.T) {
	result := fixCoverImageData(nil)
	if result != nil {
		t.Error("expected nil output for nil input")
	}
}

func TestFixCoverImageData_AlreadyJFIF(t *testing.T) {
	// JPEG data that already has JFIF marker (0xFF 0xD8 0xFF 0xE0)
	jfifData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00}
	coverData := &coverImageData{Format: "jpeg", Data: jfifData}

	result := fixCoverImageData(coverData)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Data should be unchanged (already JFIF)
	if len(result.Data) != len(jfifData) {
		t.Errorf("expected data unchanged for already-JFIF image")
	}
	for i := range jfifData {
		if result.Data[i] != jfifData[i] {
			t.Errorf("JFIF data should be unchanged")
			break
		}
	}
}

func TestFixCoverImageData_EXIFReencode(t *testing.T) {
	// Create a minimal valid JPEG using Go's encoder (which produces no JFIF marker)
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	validJPEG := buf.Bytes()

	// Go's encoder produces no JFIF marker, so this should trigger re-encoding
	if isJFIFJPEG(validJPEG) {
		t.Skip("Go JPEG encoder produces JFIF, test not applicable")
	}

	coverData := &coverImageData{Format: "jpeg", Data: validJPEG}
	result := fixCoverImageData(coverData)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// After re-encoding with prepended JFIF, the result should be JFIF
	if !isJFIFJPEG(result.Data) {
		t.Error("expected re-encoded JPEG to have JFIF marker after prepend")
	}
	// The result should still be a valid JPEG (starts with FF D8)
	if len(result.Data) < 2 || result.Data[0] != 0xFF || result.Data[1] != 0xD8 {
		t.Error("re-encoded data is not a valid JPEG")
	}
}

func TestFixCoverImageData_NonJPEG_Unchanged(t *testing.T) {
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	coverData := &coverImageData{Format: "png", Data: pngData}

	result := fixCoverImageData(coverData)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Data) != len(pngData) {
		t.Errorf("PNG data should be unchanged")
	}
	for i := range pngData {
		if result.Data[i] != pngData[i] {
			t.Errorf("PNG data should be unchanged")
			break
		}
	}
}

func TestFixCoverImageData_CorruptJPEG_ReturnsOriginal(t *testing.T) {
	// Corrupt JPEG data (has EXIF marker but can't be decoded)
	corruptData := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x05, 0x00, 0x00}
	coverData := &coverImageData{Format: "jpeg", Data: corruptData}

	result := fixCoverImageData(coverData)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Should return original data since re-encoding fails
	if len(result.Data) != len(corruptData) {
		t.Errorf("corrupt JPEG should return original data")
	}
}

func TestFixCoverImageData_ShortData_NoPanic(t *testing.T) {
	// Data too short for JPEG check (< 4 bytes)
	shortData := []byte{0xFF, 0xD8}
	coverData := &coverImageData{Format: "jpeg", Data: shortData}

	result := fixCoverImageData(coverData)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestJpegType(t *testing.T) {
	// Port of Python resources.py:725-747 jpeg_type
	jfifData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00}
	if jt := jpegType(jfifData); jt != "JPEG" {
		t.Errorf("jpegType(JFIF) = %q, want 'JPEG'", jt)
	}

	exifData := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x10, 0x45, 0x78, 0x69, 0x66, 0x00}
	if jt := jpegType(exifData); jt != "JPEG/Exif" {
		t.Errorf("jpegType(EXIF) = %q, want 'JPEG/Exif'", jt)
	}

	spiffData := []byte{0xFF, 0xD8, 0xFF, 0xE8, 0x00}
	if jt := jpegType(spiffData); jt != "JPEG/SPIFF" {
		t.Errorf("jpegType(SPIFF) = %q, want 'JPEG/SPIFF'", jt)
	}

	adobeData := []byte{0xFF, 0xD8, 0xFF, 0xED, 0x00}
	if jt := jpegType(adobeData); jt != "JPEG/Adobe" {
		t.Errorf("jpegType(Adobe) = %q, want 'JPEG/Adobe'", jt)
	}

	noAppData := []byte{0xFF, 0xD8, 0xFF, 0xDB, 0x00}
	if jt := jpegType(noAppData); jt != "JPEG/no-app-marker" {
		t.Errorf("jpegType(no-app) = %q, want 'JPEG/no-app-marker'", jt)
	}

	nonJPEG := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x00}
	if jt := jpegType(nonJPEG); jt != "UNKNOWN(89504e470d0a1a0a00000000)" {
		t.Errorf("jpegType(PNG) = %q, want 'UNKNOWN(89504e470d0a1a0a00000000)'", jt)
	}
}

// ---------------------------------------------------------------------------
// VAL-M1-META-001: updateCoverSectionAndStoryline recursively processes $157
// Python yj_metadata.py:813-814
// Already tested by TestUpdateCoverSectionAndStoryline_NestedStyleFragments
// above, but add an explicit test for deep nesting.
// ---------------------------------------------------------------------------

func TestUpdateCoverSectionAndStoryline_DeepNested157(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// Three levels of $157 nesting
	cat.SectionOrder = []string{"cover_section"}
	cat.SectionFragments["cover_section"] = sectionFragment{
		ID:        "cover_section",
		Storyline: "",
		PageTemplateValues: map[string]interface{}{
			"style": "style_l1",
		},
	}
	cat.StyleFragments["style_l1"] = map[string]interface{}{
		"width":  800,
		"style": "style_l2",
	}
	cat.StyleFragments["style_l2"] = map[string]interface{}{
		"height":  600,
		"style": "style_l3",
	}
	cat.StyleFragments["style_l3"] = map[string]interface{}{
		"fixed_width": 800,
	}

	updateCoverSectionAndStoryline(cat, 800, 600, 1024, 768)

	// All three levels should be updated
	if cat.StyleFragments["style_l1"]["width"] != 1024 {
		t.Errorf("expected style_l1 $56=1024, got %v", cat.StyleFragments["style_l1"]["width"])
	}
	if cat.StyleFragments["style_l2"]["height"] != 768 {
		t.Errorf("expected style_l2 $57=768, got %v", cat.StyleFragments["style_l2"]["height"])
	}
	if cat.StyleFragments["style_l3"]["fixed_width"] != 1024 {
		t.Errorf("expected style_l3 $66=1024, got %v", cat.StyleFragments["style_l3"]["fixed_width"])
	}
}

func TestUpdateCoverSectionAndStoryline_StyleFragmentWith146Children(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// $157 style fragment with $146 children that also have $157 refs
	cat.SectionOrder = []string{"cover_section"}
	cat.SectionFragments["cover_section"] = sectionFragment{
		ID:        "cover_section",
		Storyline: "",
		PageTemplateValues: map[string]interface{}{
			"style": "style_parent",
		},
	}
	cat.StyleFragments["style_parent"] = map[string]interface{}{
		"width": 800,
		"content_list": []interface{}{
			map[string]interface{}{
				"height":  600,
				"style": "style_child_a",
			},
			map[string]interface{}{
				"fixed_width": 800,
				"style": "style_child_b",
			},
		},
	}
	cat.StyleFragments["style_child_a"] = map[string]interface{}{
		"fixed_height": 600,
	}
	cat.StyleFragments["style_child_b"] = map[string]interface{}{
		"width": 800,
	}

	updateCoverSectionAndStoryline(cat, 800, 600, 1024, 768)

	if cat.StyleFragments["style_parent"]["width"] != 1024 {
		t.Errorf("expected style_parent $56=1024, got %v", cat.StyleFragments["style_parent"]["width"])
	}
	if cat.StyleFragments["style_child_a"]["fixed_height"] != 768 {
		t.Errorf("expected style_child_a $67=768, got %v", cat.StyleFragments["style_child_a"]["fixed_height"])
	}
	if cat.StyleFragments["style_child_b"]["width"] != 1024 {
		t.Errorf("expected style_child_b $56=1024, got %v", cat.StyleFragments["style_child_b"]["width"])
	}
}

func TestUpdateCoverSectionAndStoryline_StructValueNotUpdated(t *testing.T) {
	cat := makeTestCatalogForMetadata()

	// $56 value is a struct (IonStruct) — should NOT be updated
	cat.SectionOrder = []string{"cover_section"}
	cat.SectionFragments["cover_section"] = sectionFragment{
		ID:        "cover_section",
		Storyline: "",
		PageTemplateValues: map[string]interface{}{
			"width": map[string]interface{}{"nested": true}, // IonStruct — skip
			"height": 600,                                    // int — update
		},
	}

	updateCoverSectionAndStoryline(cat, 800, 600, 1024, 768)

	ptv := cat.SectionFragments["cover_section"].PageTemplateValues
	// $56 should remain unchanged (it's a struct)
	if m, ok := ptv["width"].(map[string]interface{}); !ok || !m["nested"].(bool) {
		t.Errorf("expected $56 to remain a struct, got %v", ptv["width"])
	}
	// $57 should be updated
	if ptv["height"] != 768 {
		t.Errorf("expected $57=768, got %v", ptv["height"])
	}
}
