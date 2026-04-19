package kfx

import (
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_title_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "title",
						"$307": "Title A",
					},
				},
			},
		},
	}

	// Tier 2: $258 fragment with title symbol = "Title B" (fallback)
	cat.ReadingOrderMetadata = map[string]interface{}{
		"$153": "Title B",
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
		"$153": "Title B",
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_title_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "yj_fixed_layout",
						"$307": "should_not_match",
					},
				},
			},
			map[string]interface{}{
				"$495": "kindle_capability_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "yj_fixed_layout",
						"$307": 2,
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
		"$590": []interface{}{
			map[string]interface{}{
				"$586": "com.amazon.yjconversion",
				"$492": "yj.illustrated_layout",
				"$589": map[string]interface{}{
					"version": map[string]interface{}{
						"$587": 2,
						"$588": 0,
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
		"$590": []interface{}{
			map[string]interface{}{
				"$586": "com.amazon.yjconversion",
				"$492": "yj.illustrated_layout",
				"$589": map[string]interface{}{
					"version": map[string]interface{}{
						"$587": 2,
						"$588": 3,
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

	// $593 fragments stored by their fragment IDs
	cat.FormatCapabilities = map[string]map[string]interface{}{
		"fc1": {
			"items": []interface{}{
				map[string]interface{}{
					"$492":    "some_feat",
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_title_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "cde_content_type",
						"$307": "MAGZ",
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_title_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "cde_content_type",
						"$307": "EBOK",
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_title_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "cde_content_type",
						"$307": "EBSP",
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_title_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "cde_content_type",
						"$307": "EBOK",
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_capability_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "yj_fixed_layout",
						"$307": 1,
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_capability_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "yj_fixed_layout",
						"$307": 2,
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_capability_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "yj_textbook",
						"$307": 1,
					},
					map[string]interface{}{
						"$492": "yj_fixed_layout",
						"$307": 1,
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_capability_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "yj_textbook",
						"$307": 1,
					},
					map[string]interface{}{
						"$492": "yj_fixed_layout",
						"$307": 3,
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_capability_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "yj_textbook",
						"$307": 1,
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
	// Add an image resource that allows getOrderedImageResources to succeed
	cat.ResourceFragments["img1"] = resourceFragment{ID: "img1", Location: "loc1"}
	ci := newCacheInfo()
	if !isImageBasedFixedLayout(cat, ci) {
		t.Error("expected isImageBasedFixedLayout=true when resource fragments exist")
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_title_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "cde_content_type",
						"$307": "MAGZ",
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
	// $270 fragments are stored by ID in FragmentIDsByType + we need raw values
	cat.Generators = map[string]map[string]interface{}{
		"gen1": {"version": 1},
	}

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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_title_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "cover_image",
						"$307": "cover_res_1",
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
		"$161": "$285", // jpg format symbol
		"$165": "cover_loc",
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_title_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "cover_image",
						"$307": "cover_res_1",
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
		"$161": "$285",
		"$165": "cover_loc",
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_title_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "asset_id",
						"$307": "asset_123",
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
	cat.TitleMetadata = map[string]interface{}{"$491": nil}
	if !hasMetadata(cat) {
		t.Error("expected true when $490 exists")
	}
}

func TestHasMetadata_WithReadingOrderMetadata(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.ReadingOrderMetadata = map[string]interface{}{"$153": "test"}
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
		"$161": "$565", // PDF format
	}
	ci := newCacheInfo()
	if !hasPDFResource(cat, ci) {
		t.Error("expected true when $164 has $565 format")
	}
}

func TestHasPDFResource_False(t *testing.T) {
	cat := makeTestCatalogForMetadata()
	cat.ResourceRawData["res1"] = map[string]interface{}{
		"$161": "$548", // JPEG-XR format
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
			"$587": "GeneratorA",
			"$588": "1.0",
		},
		"gen2": {
			"$587": "GeneratorB",
			"$588": "2.0",
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_capability_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "yj_fixed_layout",
						"$307": 3,
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_capability_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "yj_fixed_layout",
						"$307": 1,
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
		"$590": []interface{}{
			map[string]interface{}{
				"$586": "com.amazon.yjconversion",
				"$492": "yj.illustrated_layout",
				"$589": map[string]interface{}{
					"version": map[string]interface{}{
						"$587": 1,
						"$588": 0,
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
		"$491": []interface{}{
			map[string]interface{}{
				"$495": "kindle_title_metadata",
				"$258": []interface{}{
					map[string]interface{}{
						"$492": "cde_content_type",
						"$307": "EBOK",
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
			"$392": []interface{}{
				map[string]interface{}{
					"$235": "$237",
					"$247": []interface{}{"p1", "p2", "p3"},
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
