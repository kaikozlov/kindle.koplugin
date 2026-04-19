package kfx

import "testing"

// ---------------------------------------------------------------------------
// VAL-C-001: Sentinel constants defined with correct values
// ---------------------------------------------------------------------------

func TestSentinelConstantsAny(t *testing.T) {
	if !Any {
		t.Errorf("ANY should be true, got %v", Any)
	}
}

func TestSentinelConstantsTF(t *testing.T) {
	if len(TF) != 2 {
		t.Fatalf("TF should have 2 entries, got %d", len(TF))
	}
	if !TF[true] {
		t.Error("TF should contain true")
	}
	if !TF[false] {
		t.Error("TF should contain false")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-002: Feature name string constants — complete and identity-mapped
// ---------------------------------------------------------------------------

func TestFeatureNameConstantsComplete(t *testing.T) {
	expected := []struct {
		name     string
		constant string
	}{
		// General capabilities (6)
		{"ARTICLE_READER_V1", ArticleReaderV1},
		{"DUAL_READING_ORDER_V1", DualReadingOrderV1},
		{"GUIDED_VIEW_NATIVE_V1", GuidedViewNativeV1},
		{"JPEGXR_ENCODING_SUPPORTED", JPEGXREncodingSupported},
		{"KINDLE_RECAPS_V1", KindleRecapsV1},
		{"MOP_SUPPORTED", MOPSupported},
		// NMDL note (4)
		{"NMDL_NOTE", NMDLNote},
		{"NMDL_NOTE_V2", NMDLNoteV2},
		{"NMDL_NOTE_V3", NMDLNoteV3},
		{"NMDL_NOTE_V4", NMDLNoteV4},
		// HD/HDV/Vella (4)
		{"SUPPORTS_HD_V1", SupportsHDV1},
		{"SUPPORTS_HDV_V1", SupportsHDVV1},
		{"SUPPORTS_HDV_V2", SupportsHDVV2},
		{"VELLA_SUPPORTED", VellaSupported},
		// Generic
		{"YJ", YJ},
		// JP Vertical (8)
		{"YJJPV_V1_SIMPLEVERTICAL", YJJPVV1SimpleVertical},
		{"YJJPV_V2_TRACKB", YJJPVV2TrackB},
		{"YJJPV_V3_TRACKB", YJJPVV3TrackB},
		{"YJJPV_V4_TRACKD_PART1", YJJPVV4TrackDPart1},
		{"YJJPV_V5", YJJPVV5},
		{"YJJPV_V6", YJJPVV6},
		{"YJJPV_V7", YJJPVV7},
		{"YJJPV_V8", YJJPVV8},
		// Audio (3)
		{"YJ_AUDIO_V1", YJAudioV1},
		{"YJ_AUDIO_V2", YJAudioV2},
		{"YJ_AUDIO_V3", YJAudioV3},
		// Conditional structure, cover, dict
		{"YJ_CONDITIONAL_STRUCTURE_V1", YJConditionalStructureV1},
		{"YJ_COVER_IMAGE_DEFER_V1", YJCoverImageDeferV1},
		{"YJ_COVER_IMAGE_SEPARATE_V1", YJCoverImageSeparateV1},
		{"YJ_DICT_V1", YJDictV1},
		{"YJ_DICT_V1_ARABIC", YJDictV1Arabic},
		// Fixed layout
		{"YJ_FIXED_LAYOUT_V2", YJFixedLayoutV2},
		{"YJ_FIXED_LAYOUT_PDF", YJFixedLayoutPDF},
		{"YJ_FIXED_LAYOUT_PDF_V2", YJFixedLayoutPDFV2},
		{"YJ_FIXED_LAYOUT_PDF_V3", YJFixedLayoutPDFV3},
		{"YJ_FIXED_LAYOUT_PDOCS_PDF", YJFixedLayoutPDocsPDF},
		// Other capabilities
		{"YJ_FORCED_CONTINUOUS_SCROLL_V1", YJForcedContinuousScrollV1},
		{"YJ_INTERACTIVITY_V1", YJInteractivityV1},
		{"YJ_INTERACTIVITY_V2", YJInteractivityV2},
		{"YJ_MATHML_V1", YJMathmlV1},
		{"YJ_MIXED_WRITING_MODE_V1", YJMixedWritingModeV1},
		{"YJ_MIXED_WRITING_MODE_V2", YJMixedWritingModeV2},
		{"YJ_NON_PDF_AUDIO_VIDEO_V1", YJNonPDFAudioVideoV1},
		// PDF-backed
		{"YJ_PDF_BACKED_FIXED_LAYOUT_V1", YJPDFBackedFixedLayoutV1},
		{"YJ_PDF_BACKED_FIXED_LAYOUT_V1_TEST", YJPDFBackedFixedLayoutV1Test},
		{"YJ_PDF_BACKED_FIXED_LAYOUT_V2", YJPDFBackedFixedLayoutV2},
		{"YJ_PDF_LINKS", YJPDFLinks},
		// Publisher panels
		{"YJ_PUBLISHER_PANELS_V2", YJPublisherPanelsV2},
		{"YJ_PUBLISHER_PANELS_V3", YJPublisherPanelsV3},
		{"YJ_PUBLISHER_PANELS_V4", YJPublisherPanelsV4},
		// Ruby, reflowable
		{"YJ_RUBY_V1", YJRubyV1},
		{"YJ_REFLOWABLE", YJReflowable},
		{"YJ_REFLOWABLE_V2", YJReflowableV2},
		{"YJ_REFLOWABLE_V3", YJReflowableV3},
		{"YJ_REFLOWABLE_V4", YJReflowableV4},
		{"YJ_REFLOWABLE_V5", YJReflowableV5},
		{"YJ_REFLOWABLE_V6", YJReflowableV6},
		{"YJ_REFLOWABLE_V7", YJReflowableV7},
		{"YJ_REFLOWABLE_V8", YJReflowableV8},
		{"YJ_REFLOWABLE_V9", YJReflowableV9},
		{"YJ_REFLOWABLE_V10", YJReflowableV10},
		{"YJ_REFLOWABLE_V11", YJReflowableV11},
		{"YJ_REFLOWABLE_V12", YJReflowableV12},
		{"YJ_REFLOWABLE_V13", YJReflowableV13},
		{"YJ_REFLOWABLE_V14", YJReflowableV14},
		// Language-specific reflowable
		{"YJ_REFLOWABLE_AR_v1", YJReflowableARv1},
		{"YJ_REFLOWABLE_CN_v1", YJReflowableCNv1},
		{"YJ_REFLOWABLE_FA_V1", YJReflowableFAV1},
		{"YJ_REFLOWABLE_HE_V1", YJReflowableHEV1},
		{"YJ_REFLOWABLE_INDIC_V1", YJReflowableIndicV1},
		{"YJ_REFLOWABLE_JP_v1", YJReflowableJPv1},
		{"YJ_REFLOWABLE_LANG_EXPANSION_V1", YJReflowableLangExpansionV1},
		{"YJ_REFLOWABLE_LANG_EXPANSION_V2", YJReflowableLangExpansionV2},
		{"YJ_REFLOWABLE_LARGESECTION", YJReflowableLargeSection},
		// Tables
		{"YJ_REFLOWABLE_TABLESv1", YJReflowableTablesv1},
		{"YJ_REFLOWABLE_TABLESv2", YJReflowableTablesv2},
		{"YJ_REFLOWABLE_TABLESv3", YJReflowableTablesv3},
		{"YJ_REFLOWABLE_TABLESv4", YJReflowableTablesv4},
		{"YJ_REFLOWABLE_TABLESv5", YJReflowableTablesv5},
		{"YJ_REFLOWABLE_TABLESv6", YJReflowableTablesv6},
		{"YJ_REFLOWABLE_TABLESv7", YJReflowableTablesv7},
		{"YJ_REFLOWABLE_TABLESv8", YJReflowableTablesv8},
		{"YJ_REFLOWABLE_TABLESv9", YJReflowableTablesv9},
		{"YJ_REFLOWABLE_TABLESv10", YJReflowableTablesv10},
		{"YJ_REFLOWABLE_TABLESv11", YJReflowableTablesv11},
		// Table viewer
		{"YJ_REFLOWABLE_TABLEVIEWERv1", YJReflowableTableViewerv1},
		{"YJ_REFLOWABLE_TABLEVIEWERv2", YJReflowableTableViewerv2},
		// TCN, text popups, vertical text shadow, video
		{"YJ_REFLOWABLE_TCN_v1", YJReflowableTCNv1},
		{"YJ_TEXT_POPUPS_V1", YJTextPopUpsV1},
		{"YJ_VERTICAL_TEXT_SHADOW_V1", YJVerticalTextShadowV1},
		{"YJ_VIDEO_V1", YJVideoV1},
		{"YJ_VIDEO_V3", YJVideoV3},
	}

	for _, tc := range expected {
		if tc.constant != tc.name {
			t.Errorf("constant %s = %q, want %q", tc.name, tc.constant, tc.name)
		}
	}

	// Total should match Python: 73 string constants + NMDL_NOTE_V2/V3/V4 tuples ported as strings
	// The feature description says "~90" but Python actually has 73 string constants.
	// We include NMDL_NOTE_V2/V3/V4 which in Python are single-element tuples due to trailing commas.
	if len(expected) != 92 {
		t.Errorf("expected 92 feature name constants (73 Python strings + NMDL tuples + table/TCN/viewer variants), got %d", len(expected))
	}
}

// ---------------------------------------------------------------------------
// VAL-C-003: PACKAGE_VERSION_PLACEHOLDERS set — 3 entries
// ---------------------------------------------------------------------------

func TestPackageVersionPlaceholders(t *testing.T) {
	if len(PackageVersionPlaceholders) != 3 {
		t.Fatalf("PACKAGE_VERSION_PLACEHOLDERS should have 3 entries, got %d", len(PackageVersionPlaceholders))
	}

	expected := []string{
		"PackageVersion:YJReaderSDK-1.0.x.x GitSHA:c805492 Month-Day:04-22",
		"PackageVersion:YJReaderSDK-1.0.x.x GitSHA:[33mc805492[m Month-Day:04-22",
		"kfxlib-00000000",
	}
	for _, e := range expected {
		if !PackageVersionPlaceholders[e] {
			t.Errorf("PACKAGE_VERSION_PLACEHOLDERS missing %q", e)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-C-004: KNOWN_KFX_GENERATORS set — ~49 tuple entries
// ---------------------------------------------------------------------------

func TestKnownKFXGenerators(t *testing.T) {
	if len(KnownKFXGenerators) != 50 {
		t.Errorf("KNOWN_KFX_GENERATORS should have 50 entries, got %d", len(KnownKFXGenerators))
	}

	// Spot-check specific entries
	tests := []struct {
		version string
		pkg     string
		want    bool
	}{
		{"2.16", "PackageVersion:YJReaderSDK-1.0.824.0 Month-Day:04-09", true},
		{"7.153.1.0", "", true},
		{"20.12.238.0", "", true},
		{"99.99.99.0", "", false},
	}
	for _, tc := range tests {
		got := KnownKFXGenerators[GeneratorEntry{Version: tc.version, PackageVersion: tc.pkg}]
		if got != tc.want {
			t.Errorf("KNOWN_KFX_GENERATORS[%+v] = %v, want %v", tc, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-C-005: GENERIC_CREATOR_VERSIONS set — 3 entries
// ---------------------------------------------------------------------------

func TestGenericCreatorVersions(t *testing.T) {
	if len(GenericCreatorVersions) != 3 {
		t.Errorf("GENERIC_CREATOR_VERSIONS should have 3 entries, got %d", len(GenericCreatorVersions))
	}

	expected := []GeneratorEntry{
		{"YJConversionTools", "2.15.0"},
		{"KTC", "1.0.11.1"},
		{"", ""},
	}
	for _, e := range expected {
		if !GenericCreatorVersions[e] {
			t.Errorf("GENERIC_CREATOR_VERSIONS missing %+v", e)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-C-006: KNOWN_FEATURES nested dict — 4 categories
// ---------------------------------------------------------------------------

func TestKnownFeaturesCategories(t *testing.T) {
	// Must have exactly 4 top-level categories
	expectedCats := []string{
		"format_capabilities",
		"SDK.Marker",
		"com.amazon.kindle.nmdl",
		"com.amazon.yjconversion",
	}
	if len(KnownFeatures) != len(expectedCats) {
		t.Errorf("KNOWN_FEATURES should have %d categories, got %d", len(expectedCats), len(KnownFeatures))
	}
	for _, cat := range expectedCats {
		if _, ok := KnownFeatures[cat]; !ok {
			t.Errorf("KNOWN_FEATURES missing category %q", cat)
		}
	}

	// format_capabilities has ≥4 feature keys
	fc := KnownFeatures["format_capabilities"]
	if len(fc) < 4 {
		t.Errorf("format_capabilities should have ≥4 keys, got %d", len(fc))
	}

	// SDK.Marker has 1 key: CanonicalFormat
	sm := KnownFeatures["SDK.Marker"]
	if _, ok := sm["CanonicalFormat"]; !ok {
		t.Error("SDK.Marker missing CanonicalFormat key")
	}

	// com.amazon.kindle.nmdl has 1 key: note
	nmdl := KnownFeatures["com.amazon.kindle.nmdl"]
	if _, ok := nmdl["note"]; !ok {
		t.Error("com.amazon.kindle.nmdl missing note key")
	}

	// com.amazon.yjconversion has ~38 feature keys
	yjc := KnownFeatures["com.amazon.yjconversion"]
	if len(yjc) < 35 {
		t.Errorf("com.amazon.yjconversion should have ≥35 keys, got %d", len(yjc))
	}
}

func TestKnownFeaturesReflowStyle(t *testing.T) {
	// reflow-style has versions 1-14 plus tuple keys
	rs := KnownFeatures["com.amazon.yjconversion"]["reflow-style"]
	if rs[IntVersionKey(1)] != YJReflowable {
		t.Errorf("reflow-style[1] = %q, want %q", rs[IntVersionKey(1)], YJReflowable)
	}
	if rs[IntVersionKey(14)] != YJReflowableV14 {
		t.Errorf("reflow-style[14] = %q, want %q", rs[IntVersionKey(14)], YJReflowableV14)
	}
	tupleKey := TupleVersionKey(2147483646, 2147483647)
	if rs[tupleKey] != YJ {
		t.Errorf("reflow-style[(2147483646, 2147483647)] = %q, want %q", rs[tupleKey], YJ)
	}
}

func TestKnownFeaturesYjTable(t *testing.T) {
	yt := KnownFeatures["com.amazon.yjconversion"]["yj_table"]
	for i := 1; i <= 11; i++ {
		expected := YJReflowableTablesv1
		switch i {
		case 2:
			expected = YJReflowableTablesv2
		case 3:
			expected = YJReflowableTablesv3
		case 4:
			expected = YJReflowableTablesv4
		case 5:
			expected = YJReflowableTablesv5
		case 6:
			expected = YJReflowableTablesv6
		case 7:
			expected = YJReflowableTablesv7
		case 8:
			expected = YJReflowableTablesv8
		case 9:
			expected = YJReflowableTablesv9
		case 10:
			expected = YJReflowableTablesv10
		case 11:
			expected = YJReflowableTablesv11
		}
		if yt[IntVersionKey(i)] != expected {
			t.Errorf("yj_table[%d] = %q, want %q", i, yt[IntVersionKey(i)], expected)
		}
	}
}

func TestKnownFeaturesYjAudio(t *testing.T) {
	ya := KnownFeatures["com.amazon.yjconversion"]["yj_audio"]
	if ya[IntVersionKey(1)] != YJAudioV1 {
		t.Errorf("yj_audio[1] = %q, want %q", ya[IntVersionKey(1)], YJAudioV1)
	}
	if ya[IntVersionKey(2)] != YJAudioV2 {
		t.Errorf("yj_audio[2] = %q, want %q", ya[IntVersionKey(2)], YJAudioV2)
	}
	if ya[IntVersionKey(3)] != YJAudioV3 {
		t.Errorf("yj_audio[3] = %q, want %q", ya[IntVersionKey(3)], YJAudioV3)
	}
}

// ---------------------------------------------------------------------------
// VAL-C-007: KNOWN_FEATURES — ANY sentinel in version map
// (also tested via IsKnownFeature in VAL-C-018)
// ---------------------------------------------------------------------------

func TestKnownFeaturesAnySentinel(t *testing.T) {
	rss := KnownFeatures["com.amazon.yjconversion"]["reflow-section-size"]
	if _, ok := rss[AnyVersionKey()]; !ok {
		t.Error("reflow-section-size should have ANY sentinel key")
	}
	if rss[AnyVersionKey()] != YJReflowableLargeSection {
		t.Errorf("reflow-section-size[ANY] = %q, want %q", rss[AnyVersionKey()], YJReflowableLargeSection)
	}
}

// ---------------------------------------------------------------------------
// VAL-C-008: KNOWN_SUPPORTED_FEATURES set — 5 entries
// ---------------------------------------------------------------------------

func TestKnownSupportedFeatures(t *testing.T) {
	if len(KnownSupportedFeatures) != 5 {
		t.Errorf("KNOWN_SUPPORTED_FEATURES should have 5 entries, got %d", len(KnownSupportedFeatures))
	}

	// Check first entry (single-element)
	if KnownSupportedFeatures[0].Symbol != "$826" {
		t.Errorf("KNOWN_SUPPORTED_FEATURES[0].Symbol = %q, want $826", KnownSupportedFeatures[0].Symbol)
	}
	// Check last entry (3-element tuple)
	last := KnownSupportedFeatures[4]
	if last.Symbol != "$664" || last.Key != "crop_bleed" || last.Version != 1 || !last.HasTuple {
		t.Errorf("KNOWN_SUPPORTED_FEATURES[4] = %+v, want {$664 crop_bleed 1 true}", last)
	}
}

// ---------------------------------------------------------------------------
// VAL-C-009: KNOWN_METADATA nested dict — ~7-8 categories
// ---------------------------------------------------------------------------

func TestKnownMetadataCategories(t *testing.T) {
	expectedCats := []string{
		"book_navigation",
		"book_requirements",
		"kindle_audit_metadata",
		"kindle_capability_metadata",
		"kindle_ebook_metadata",
		"kindle_title_metadata",
		"metadata",
		"symbols",
	}
	if len(KnownMetadata) != len(expectedCats) {
		t.Errorf("KNOWN_METADATA should have %d categories, got %d", len(expectedCats), len(KnownMetadata))
	}
	for _, cat := range expectedCats {
		if _, ok := KnownMetadata[cat]; !ok {
			t.Errorf("KNOWN_METADATA missing category %q", cat)
		}
	}
}

func TestKnownMetadataAnySentinel(t *testing.T) {
	// book_navigation/pages is ANY
	if !KnownMetadata["book_navigation"]["pages"].IsAny {
		t.Error("KNOWN_METADATA[book_navigation][pages] should be ANY")
	}
	// book_requirements/min_kindle_version is ANY
	if !KnownMetadata["book_requirements"]["min_kindle_version"].IsAny {
		t.Error("KNOWN_METADATA[book_requirements][min_kindle_version] should be ANY")
	}
}

func TestKnownMetadataCreatorVersion(t *testing.T) {
	// kindle_audit_metadata/creator_version should have ~150+ entries
	cv := KnownMetadata["kindle_audit_metadata"]["creator_version"]
	if cv.IsAny {
		t.Error("creator_version should not be ANY")
	}
	if len(cv.Strings) < 150 {
		t.Errorf("creator_version should have ≥150 entries, got %d", len(cv.Strings))
	}
	// Spot-check a few values
	if !cv.Strings["2.15.0"] {
		t.Error("creator_version missing 2.15.0")
	}
	if !cv.Strings["3.103.0"] {
		t.Error("creator_version missing 3.103.0")
	}
}

func TestKnownMetadataSymbols(t *testing.T) {
	// symbols/max_id should have ~42 integer values
	mi := KnownMetadata["symbols"]["max_id"]
	if len(mi.Ints) < 42 {
		t.Errorf("symbols/max_id should have ≥42 entries, got %d", len(mi.Ints))
	}
	if !mi.Ints[489] {
		t.Error("symbols/max_id missing 489")
	}
	if !mi.Ints[851] {
		t.Error("symbols/max_id missing 851")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-010: KNOWN_AUXILIARY_METADATA — ~38 keys
// ---------------------------------------------------------------------------

func TestKnownAuxiliaryMetadata(t *testing.T) {
	if len(KnownAuxiliaryMetadata) < 36 {
		t.Errorf("KNOWN_AUXILIARY_METADATA should have ≥36 keys, got %d", len(KnownAuxiliaryMetadata))
	}

	// Spot-check required keys
	requiredKeys := []string{
		"ANCHOR_REFERRED_BY_CONTAINERS",
		"auxData_resource_list",
		"has_large_data_table",
		"mime",
		"namespace",
		"page_rotation",
	}
	for _, k := range requiredKeys {
		if _, ok := KnownAuxiliaryMetadata[k]; !ok {
			t.Errorf("KNOWN_AUXILIARY_METADATA missing key %q", k)
		}
	}

	// Check ANY sentinel keys
	if !KnownAuxiliaryMetadata["ANCHOR_REFERRED_BY_CONTAINERS"].IsAny {
		t.Error("ANCHOR_REFERRED_BY_CONTAINERS should be ANY")
	}
	// Check specific set keys
	if !KnownAuxiliaryMetadata["mime"].Strings["Audio"] {
		t.Error("mime should contain Audio")
	}
	if !KnownAuxiliaryMetadata["mime"].Strings["Video"] {
		t.Error("mime should contain Video")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-011: KNOWN_KCB_DATA — 4 categories
// ---------------------------------------------------------------------------

func TestKnownKCBData(t *testing.T) {
	expectedCats := []string{"book_state", "content_hash", "metadata", "tool_data"}
	if len(KnownKCBData) != len(expectedCats) {
		t.Errorf("KNOWN_KCB_DATA should have %d categories, got %d", len(expectedCats), len(KnownKCBData))
	}
	for _, cat := range expectedCats {
		if _, ok := KnownKCBData[cat]; !ok {
			t.Errorf("KNOWN_KCB_DATA missing category %q", cat)
		}
	}

	// Spot-check tool_data keys
	td := KnownKCBData["tool_data"]
	requiredKeys := []string{"cache_path", "created_on", "last_modified_time", "link_extract_choice", "link_notification_preference"}
	for _, k := range requiredKeys {
		if _, ok := td[k]; !ok {
			t.Errorf("KNOWN_KCB_DATA[tool_data] missing key %q", k)
		}
	}

	// tool_data/cache_path is ANY
	if !td["cache_path"].IsAny {
		t.Error("tool_data/cache_path should be ANY")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-012: UNSUPPORTED constant
// ---------------------------------------------------------------------------

func TestUnsupported(t *testing.T) {
	if Unsupported != "Unsupported" {
		t.Errorf("UNSUPPORTED = %q, want %q", Unsupported, "Unsupported")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-013: KINDLE_VERSION_CAPABILITIES — ~20 version entries
// ---------------------------------------------------------------------------

func TestKindleVersionCapabilities(t *testing.T) {
	if len(KindleVersionCapabilities) < 20 {
		t.Errorf("KINDLE_VERSION_CAPABILITIES should have ≥20 entries, got %d", len(KindleVersionCapabilities))
	}

	// First entry
	first := KindleVersionCapabilities["5.6.5"]
	firstSlice, ok := first.([]string)
	if !ok {
		t.Fatalf("5.6.5 should be []string")
	}
	expected := []string{JPEGXREncodingSupported, YJ, YJReflowable, YJReflowableV2, YJReflowableLargeSection}
	if len(firstSlice) != len(expected) {
		t.Errorf("5.6.5 should have %d entries, got %d", len(expected), len(firstSlice))
	}
	for i, v := range expected {
		if firstSlice[i] != v {
			t.Errorf("5.6.5[%d] = %q, want %q", i, firstSlice[i], v)
		}
	}

	// Special single-value entry
	single := KindleVersionCapabilities["5.14.3.2"]
	singleStr, ok := single.(string)
	if !ok {
		t.Fatalf("5.14.3.2 should be string, not []string")
	}
	if singleStr != YJPDFBackedFixedLayoutV1Test {
		t.Errorf("5.14.3.2 = %q, want %q", singleStr, YJPDFBackedFixedLayoutV1Test)
	}

	// Last entry
	last := KindleVersionCapabilities["5.18.5"]
	lastSlice, ok := last.([]string)
	if !ok {
		t.Fatalf("5.18.5 should be []string")
	}
	if len(lastSlice) != 3 {
		t.Errorf("5.18.5 should have 3 entries, got %d", len(lastSlice))
	}
}

// ---------------------------------------------------------------------------
// VAL-C-014: KINDLE_CAPABILITY_VERSIONS — computed inverse map
// ---------------------------------------------------------------------------

func TestKindleCapabilityVersions(t *testing.T) {
	// YJ → 5.6.5
	if KindleCapabilityVersions[YJ] != "5.6.5" {
		t.Errorf("KINDLE_CAPABILITY_VERSIONS[YJ] = %q, want 5.6.5", KindleCapabilityVersions[YJ])
	}
	// YJ_REFLOWABLE → 5.6.5
	if KindleCapabilityVersions[YJReflowable] != "5.6.5" {
		t.Errorf("KINDLE_CAPABILITY_VERSIONS[YJ_REFLOWABLE] = %q, want 5.6.5", KindleCapabilityVersions[YJReflowable])
	}
	// YJ_PDF_BACKED_FIXED_LAYOUT_V1_TEST → 5.14.3.2
	if KindleCapabilityVersions[YJPDFBackedFixedLayoutV1Test] != "5.14.3.2" {
		t.Errorf("KINDLE_CAPABILITY_VERSIONS[YJ_PDF_BACKED_FIXED_LAYOUT_V1_TEST] = %q, want 5.14.3.2", KindleCapabilityVersions[YJPDFBackedFixedLayoutV1Test])
	}
	// YJ_MATHML_V1 → 5.18.2
	if KindleCapabilityVersions[YJMathmlV1] != "5.18.2" {
		t.Errorf("KINDLE_CAPABILITY_VERSIONS[YJ_MATHML_V1] = %q, want 5.18.2", KindleCapabilityVersions[YJMathmlV1])
	}
}

// ---------------------------------------------------------------------------
// VAL-C-015: is_known_generator — prefix-based acceptance
// ---------------------------------------------------------------------------

func TestIsKnownGeneratorPrefix(t *testing.T) {
	tests := []struct {
		version string
		pkg     string
		want    bool
	}{
		{"", "anything", true},
		{"kfxlib-1.0", "", true},
		{"KC2.0", "", true},
		{"KPR3.0", "", true},
		{"UnknownTool", "", false},
	}
	for _, tc := range tests {
		got := IsKnownGenerator(tc.version, tc.pkg)
		if got != tc.want {
			t.Errorf("IsKnownGenerator(%q, %q) = %v, want %v", tc.version, tc.pkg, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-C-016: is_known_generator — KNOWN_KFX_GENERATORS membership
// ---------------------------------------------------------------------------

func TestIsKnownGeneratorMembership(t *testing.T) {
	// Known entry
	if !IsKnownGenerator("2.16", "PackageVersion:YJReaderSDK-1.0.824.0 Month-Day:04-09") {
		t.Error("IsKnownGenerator(2.16, ...) should be true")
	}

	// Placeholder normalization
	if !IsKnownGenerator("7.153.1.0", "PackageVersion:YJReaderSDK-1.0.x.x GitSHA:c805492 Month-Day:04-22") {
		t.Error("IsKnownGenerator(7.153.1.0, placeholder) should be true (placeholder → empty → 7.153.1.0 in set)")
	}

	// Unknown
	if IsKnownGenerator("99.99.99.0", "") {
		t.Error("IsKnownGenerator(99.99.99.0, '') should be false")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-017: is_known_feature — basic lookup
// ---------------------------------------------------------------------------

func TestIsKnownFeatureBasic(t *testing.T) {
	// Known version key (yj_audio accepts integer version keys 1, 2, 3)
	if !IsKnownFeature("com.amazon.yjconversion", "yj_audio", 1) {
		t.Error("IsKnownFeature(yj_audio, 1) should be true")
	}
}

func TestIsKnownFeatureLookup(t *testing.T) {
	// Known version key
	if !IsKnownFeature("com.amazon.yjconversion", "yj_audio", 1) {
		t.Error("IsKnownFeature(yj_audio, 1) should be true")
	}
	if !IsKnownFeature("com.amazon.yjconversion", "yj_audio", 2) {
		t.Error("IsKnownFeature(yj_audio, 2) should be true")
	}
	// Nonexistent version is still "known" because Python's ANY=True==1,
	// and key 1 exists in the map, so ANY in vals is True.
	if !IsKnownFeature("com.amazon.yjconversion", "yj_audio", "NONEXISTENT") {
		t.Error("IsKnownFeature(yj_audio, NONEXISTENT) should be true (Python ANY==1 matches key 1)")
	}
	// Nonexistent category
	if IsKnownFeature("nonexistent_cat", "key", "val") {
		t.Error("IsKnownFeature(nonexistent_cat, key, val) should be false")
	}
	// Nonexistent key in existing category
	if IsKnownFeature("com.amazon.yjconversion", "nonexistent_key", "val") {
		t.Error("IsKnownFeature(nonexistent_key) should be false")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-018: is_known_feature — ANY sentinel wildcard
// ---------------------------------------------------------------------------

func TestIsKnownFeatureAnySentinel(t *testing.T) {
	// reflow-section-size has explicit ANY sentinel key
	if !IsKnownFeature("com.amazon.yjconversion", "reflow-section-size", 99999) {
		t.Error("IsKnownFeature(reflow-section-size, 99999) should be true (ANY sentinel)")
	}
	// reflow-style does NOT have an explicit ANY sentinel, but Python's
	// ANY=True equals 1, so since key 1 exists, ANY in vals is True.
	// Our Go port replicates this by checking IntVersionKey(1).
	if !IsKnownFeature("com.amazon.yjconversion", "reflow-style", 99999) {
		t.Error("IsKnownFeature(reflow-style, 99999) should be true (Python ANY==True==1 matches key 1)")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-019: kindle_feature_version — version resolution
// ---------------------------------------------------------------------------

func TestKindleFeatureVersionResolution(t *testing.T) {
	// format_capabilities/db.schema maps 1→YJ, and YJ→5.6.5
	v := KindleFeatureVersion("format_capabilities", "db.schema", 1)
	if v != "5.6.5" {
		t.Errorf("kindle_feature_version(db.schema, 1) = %q, want 5.6.5", v)
	}

	// Nonexistent
	v = KindleFeatureVersion("nonexistent", "key", "val")
	if v != Unsupported {
		t.Errorf("kindle_feature_version(nonexistent) = %q, want %q", v, Unsupported)
	}

	// ANY fallback: reflow-section-size has only ANY→YJ_REFLOWABLE_LARGESECTION
	v = KindleFeatureVersion("com.amazon.yjconversion", "reflow-section-size", 99999)
	if v != "5.6.5" {
		t.Errorf("kindle_feature_version(reflow-section-size, 99999) = %q, want 5.6.5", v)
	}
}

// ---------------------------------------------------------------------------
// VAL-C-020: is_known_metadata — simple value lookup
// ---------------------------------------------------------------------------

func TestIsKnownMetadataSimple(t *testing.T) {
	// ANY sentinel
	if !IsKnownMetadata("book_navigation", "pages", "anything") {
		t.Error("IsKnownMetadata(book_navigation, pages, anything) should be true (ANY)")
	}
	if !IsKnownMetadata("book_requirements", "min_kindle_version", "5.6.5") {
		t.Error("IsKnownMetadata(book_requirements, min_kindle_version, 5.6.5) should be true (ANY)")
	}
	// Nonexistent category
	if IsKnownMetadata("nonexistent_cat", "key", "val") {
		t.Error("IsKnownMetadata(nonexistent_cat, key, val) should be false")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-021: is_known_metadata — list value recursion
// ---------------------------------------------------------------------------

func TestIsKnownMetadataList(t *testing.T) {
	// List of known string values (ANY)
	if !IsKnownMetadata("kindle_title_metadata", "author", []interface{}{"Author1", "Author2"}) {
		t.Error("IsKnownMetadata(author, [Author1, Author2]) should be true (ANY)")
	}

	// Empty list
	if !IsKnownMetadata("kindle_title_metadata", "author", []interface{}{}) {
		t.Error("IsKnownMetadata(author, []) should be true (empty list)")
	}

	// cde_content_type has specific values
	if !IsKnownMetadata("kindle_title_metadata", "cde_content_type", []interface{}{"EBOK", "MAGZ"}) {
		t.Error("IsKnownMetadata(cde_content_type, [EBOK, MAGZ]) should be true")
	}

	// List with unknown value in non-ANY set
	if IsKnownMetadata("kindle_title_metadata", "cde_content_type", []interface{}{"EBOK", "UNKNOWN"}) {
		t.Error("IsKnownMetadata(cde_content_type, [EBOK, UNKNOWN]) should be false")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-022: is_known_aux_metadata — lookup with ANY sentinel
// ---------------------------------------------------------------------------

func TestIsKnownAuxMetadata(t *testing.T) {
	// Known key/value
	if !IsKnownAuxMetadata("mime", "Audio") {
		t.Error("IsKnownAuxMetadata(mime, Audio) should be true")
	}
	if !IsKnownAuxMetadata("mime", "Figure") {
		t.Error("IsKnownAuxMetadata(mime, Figure) should be true")
	}
	// Unknown value in specific set
	if IsKnownAuxMetadata("mime", "unknown_mime") {
		t.Error("IsKnownAuxMetadata(mime, unknown_mime) should be false")
	}
	// Unknown key
	if IsKnownAuxMetadata("nonexistent_key", "val") {
		t.Error("IsKnownAuxMetadata(nonexistent_key, val) should be false")
	}
	// ANY sentinel key
	if !IsKnownAuxMetadata("location", "any_value_here") {
		t.Error("IsKnownAuxMetadata(location, any_value_here) should be true (ANY)")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-023: is_known_kcb_data — lookup with ANY sentinel
// ---------------------------------------------------------------------------

func TestIsKnownKCBData(t *testing.T) {
	// ANY sentinel
	if !IsKnownKCBData("tool_data", "cache_path", "/some/path") {
		t.Error("IsKnownKCBData(tool_data, cache_path, /some/path) should be true (ANY)")
	}
	// Nonexistent category
	if IsKnownKCBData("nonexistent", "key", "val") {
		t.Error("IsKnownKCBData(nonexistent, key, val) should be false")
	}
	// Known int value
	if !IsKnownKCBData("book_state", "book_input_type", 0) {
		t.Error("IsKnownKCBData(book_state, book_input_type, 0) should be true")
	}
	// Unknown int value
	if IsKnownKCBData("book_state", "book_input_type", 99) {
		t.Error("IsKnownKCBData(book_state, book_input_type, 99) should be false")
	}
}

// ---------------------------------------------------------------------------
// Additional edge case tests
// ---------------------------------------------------------------------------

func TestIsKnownMetadataIntValues(t *testing.T) {
	// kindle_capability_metadata/yj_fixed_layout accepts {1, 2, 3}
	if !IsKnownMetadata("kindle_capability_metadata", "yj_fixed_layout", 1) {
		t.Error("IsKnownMetadata(yj_fixed_layout, 1) should be true")
	}
	if !IsKnownMetadata("kindle_capability_metadata", "yj_fixed_layout", 3) {
		t.Error("IsKnownMetadata(yj_fixed_layout, 3) should be true")
	}
	if IsKnownMetadata("kindle_capability_metadata", "yj_fixed_layout", 5) {
		t.Error("IsKnownMetadata(yj_fixed_layout, 5) should be false")
	}
}

func TestIsKnownMetadataBoolValues(t *testing.T) {
	// kindle_title_metadata/is_sample uses TF {false, true}
	if !IsKnownMetadata("kindle_title_metadata", "is_sample", true) {
		t.Error("IsKnownMetadata(is_sample, true) should be true")
	}
	if !IsKnownMetadata("kindle_title_metadata", "is_sample", false) {
		t.Error("IsKnownMetadata(is_sample, false) should be true")
	}
}

func TestIsKnownMetadataFloatValues(t *testing.T) {
	// metadata/version has {1.0}
	if !IsKnownMetadata("metadata", "version", 1.0) {
		t.Error("IsKnownMetadata(metadata, version, 1.0) should be true")
	}
	if IsKnownMetadata("metadata", "version", 2.0) {
		t.Error("IsKnownMetadata(metadata, version, 2.0) should be false")
	}
}

func TestKindleFeatureVersionTupleKey(t *testing.T) {
	// reflow-style has tuple keys (2147483646, 2147483647) → YJ
	v := KindleFeatureVersion("com.amazon.yjconversion", "reflow-style", [2]int{2147483646, 2147483647})
	if v != "5.6.5" {
		t.Errorf("kindle_feature_version(reflow-style, (2147483646,2147483647)) = %q, want 5.6.5", v)
	}
}

func TestKnownKCBDataMetadataReferences(t *testing.T) {
	// After init(), metadata/edited_tool_versions should share creator_version set
	edv := KnownKCBData["metadata"]["edited_tool_versions"]
	if edv.Strings == nil || len(edv.Strings) == 0 {
		t.Error("metadata/edited_tool_versions should be populated from creator_version")
	}
	// Spot-check
	if !edv.Strings["3.103.0"] {
		t.Error("metadata/edited_tool_versions should contain 3.103.0")
	}

	tv := KnownKCBData["metadata"]["tool_version"]
	if tv.Strings == nil || len(tv.Strings) == 0 {
		t.Error("metadata/tool_version should be populated from creator_version")
	}
}

func TestIsKnownGeneratorAllPrefixes(t *testing.T) {
	// Test all prefix variants
	tests := []struct {
		version string
		want    bool
	}{
		{"", true},
		{"kfxlib-anything", true},
		{"KC2.0", true},
		{"KPR3.0", true},
		{"SomethingElse", false},
		{"kfxlib", true},    // starts with "kfxlib"
		{"KC", true},        // starts with "KC"
		{"KPR", true},       // starts with "KPR"
		{"KFXLIB-upper", false}, // case sensitive
	}
	for _, tc := range tests {
		got := IsKnownGenerator(tc.version, "")
		if got != tc.want {
			t.Errorf("IsKnownGenerator(%q, '') = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func TestKnownKCBDataBoolValues(t *testing.T) {
	// book_state/book_manga_comic has {false}
	if !IsKnownKCBData("book_state", "book_manga_comic", false) {
		t.Error("IsKnownKCBData(book_manga_comic, false) should be true")
	}
	if IsKnownKCBData("book_state", "book_manga_comic", true) {
		t.Error("IsKnownKCBData(book_manga_comic, true) should be false")
	}
}

func TestAuxMetadataTFSet(t *testing.T) {
	// has_large_data_table uses TF {false, true}
	if !IsKnownAuxMetadata("has_large_data_table", true) {
		t.Error("IsKnownAuxMetadata(has_large_data_table, true) should be true")
	}
	if !IsKnownAuxMetadata("has_large_data_table", false) {
		t.Error("IsKnownAuxMetadata(has_large_data_table, false) should be true")
	}
}
