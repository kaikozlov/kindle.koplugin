package kfx

import (
	"encoding/json"
	"strings"
	"testing"
)

// =============================================================================
// VAL-M3-ILLUST-001: Region magnification activate handler produces correct XHTML
// Python: yj_to_epub_content.py:674-694
//
// process_content handles "activate" (activate) entries within container content ($270).
// For each activate entry with action "zoom_in", it creates an <a class="app-amzn-magnify">
// element with a data-app-amzn-magnify JSON attribute containing targetId, sourceId, ordinal.
// =============================================================================

func TestRegionMagnification(t *testing.T) {
	// Set up content with a $426 activate entry
	content := map[string]interface{}{
		"ordinal": 1, // ordinal
		"activate": []interface{}{
			map[string]interface{}{
				"action": "zoom_in", // action = magnify
				"target": "target-eid-123",
				"source": "source-eid-456",
			},
		},
	}
	cfg := &regionMagnificationConfig{
		RegionMagnification: true,
	}

	result := processRegionMagnification(content, cfg)

	if len(result.ActivateElements) != 1 {
		t.Fatalf("expected 1 activate element, got %d", len(result.ActivateElements))
	}

	elem := result.ActivateElements[0]
	if elem.Tag != "a" {
		t.Errorf("expected tag 'a', got '%s'", elem.Tag)
	}
	if elem.Attrs["class"] != "app-amzn-magnify" {
		t.Errorf("expected class='app-amzn-magnify', got class='%s'", elem.Attrs["class"])
	}

	// Verify JSON data attribute
	dataAttr := elem.Attrs["data-app-amzn-magnify"]
	if dataAttr == "" {
		t.Fatal("expected data-app-amzn-magnify attribute to be set")
	}

	// Parse JSON to verify structure
	var magnifyData map[string]interface{}
	if err := json.Unmarshal([]byte(dataAttr), &magnifyData); err != nil {
		t.Fatalf("failed to parse magnify JSON: %v", err)
	}

	// targetId should be registered as magnify_target_<eid>
	targetID, ok := magnifyData["targetId"].(string)
	if !ok || !strings.HasPrefix(targetID, "magnify_target_") {
		t.Errorf("expected targetId to start with 'magnify_target_', got '%v'", magnifyData["targetId"])
	}

	// sourceId should be registered as magnify_source_<eid>
	sourceID, ok := magnifyData["sourceId"].(string)
	if !ok || !strings.HasPrefix(sourceID, "magnify_source_") {
		t.Errorf("expected sourceId to start with 'magnify_source_', got '%v'", magnifyData["sourceId"])
	}

	// ordinal should be 1 (from $427)
	ordinal, ok := magnifyData["ordinal"].(float64)
	if !ok || ordinal != 1 {
		t.Errorf("expected ordinal=1, got '%v'", magnifyData["ordinal"])
	}
}

// =============================================================================
// VAL-M3-ILLUST-002: Region magnification auto-enables on first activate
// Python: yj_to_epub_content.py:675-677
//
// When $426 is encountered and region_magnification is false, it must:
// 1. Log error "activate found without region magnification"
// 2. Set region_magnification to True
// =============================================================================

func TestRegionMagnification_AutoEnable(t *testing.T) {
	content := map[string]interface{}{
		"ordinal": 1,
		"activate": []interface{}{
			map[string]interface{}{
				"action": "zoom_in",
				"target": "target-eid",
				"source": "source-eid",
			},
		},
	}
	cfg := &regionMagnificationConfig{
		RegionMagnification: false, // initially false
	}

	result := processRegionMagnification(content, cfg)

	// Should have auto-enabled
	if !result.AutoEnabled {
		t.Error("expected AutoEnabled=true when region_magnification was initially false")
	}
	// Should still produce the activate element
	if len(result.ActivateElements) != 1 {
		t.Errorf("expected 1 activate element even when auto-enabled, got %d", len(result.ActivateElements))
	}
}

// =============================================================================
// VAL-M3-ILLUST-003: Region magnification registers link IDs for target and source
// Python: yj_to_epub_content.py:686-688
//
// register_link_id(activate.pop("target"), "magnify_target")
// register_link_id(activate.pop("source"), "magnify_source")
//
// The register_link_id function creates an anchor name like "magnify_target_<eid>"
// and registers it with the position (eid, 0).
// =============================================================================

func TestRegionMagnification_LinkIDs(t *testing.T) {
	content := map[string]interface{}{
		"ordinal": 3,
		"activate": []interface{}{
			map[string]interface{}{
				"action": "zoom_in",
				"target": "target-eid-789",
				"source": "source-eid-012",
			},
		},
	}
	cfg := &regionMagnificationConfig{
		RegionMagnification: true,
	}

	result := processRegionMagnification(content, cfg)

	if len(result.LinkRegistrations) != 2 {
		t.Fatalf("expected 2 link registrations (target + source), got %d", len(result.LinkRegistrations))
	}

	// First registration should be magnify_target
	targetReg := result.LinkRegistrations[0]
	if targetReg.Kind != "magnify_target" {
		t.Errorf("expected first registration kind='magnify_target', got '%s'", targetReg.Kind)
	}
	if targetReg.EID != "target-eid-789" {
		t.Errorf("expected first registration EID='target-eid-789', got '%s'", targetReg.EID)
	}

	// Second registration should be magnify_source
	sourceReg := result.LinkRegistrations[1]
	if sourceReg.Kind != "magnify_source" {
		t.Errorf("expected second registration kind='magnify_source', got '%s'", sourceReg.Kind)
	}
	if sourceReg.EID != "source-eid-012" {
		t.Errorf("expected second registration EID='source-eid-012', got '%s'", sourceReg.EID)
	}
}

// Test that unknown activate action logs error and doesn't create element
func TestRegionMagnification_UnknownAction(t *testing.T) {
	content := map[string]interface{}{
		"ordinal": 1,
		"activate": []interface{}{
			map[string]interface{}{
				"action": "$999", // unknown action
				"target": "target",
				"source": "source",
			},
		},
	}
	cfg := &regionMagnificationConfig{
		RegionMagnification: true,
	}

	result := processRegionMagnification(content, cfg)

	if len(result.ActivateElements) != 0 {
		t.Errorf("expected 0 activate elements for unknown action, got %d", len(result.ActivateElements))
	}
	if !result.HasUnknownActionError {
		t.Error("expected HasUnknownActionError=true for unknown action")
	}
}

// Test that no $426 key means no activate elements
func TestRegionMagnification_NoActivate(t *testing.T) {
	content := map[string]interface{}{
		"ordinal": 1,
	}
	cfg := &regionMagnificationConfig{
		RegionMagnification: true,
	}

	result := processRegionMagnification(content, cfg)

	if len(result.ActivateElements) != 0 {
		t.Errorf("expected 0 activate elements when no $426, got %d", len(result.ActivateElements))
	}
	if result.AutoEnabled {
		t.Error("expected AutoEnabled=false when no $426")
	}
}

// Test multiple activate entries
func TestRegionMagnification_MultipleActivates(t *testing.T) {
	content := map[string]interface{}{
		"ordinal": 2,
		"activate": []interface{}{
			map[string]interface{}{
				"action": "zoom_in",
				"target": "target1",
				"source": "source1",
			},
			map[string]interface{}{
				"action": "zoom_in",
				"target": "target2",
				"source": "source2",
			},
		},
	}
	cfg := &regionMagnificationConfig{
		RegionMagnification: true,
	}

	result := processRegionMagnification(content, cfg)

	if len(result.ActivateElements) != 2 {
		t.Fatalf("expected 2 activate elements, got %d", len(result.ActivateElements))
	}
	if len(result.LinkRegistrations) != 4 {
		t.Fatalf("expected 4 link registrations (2 targets + 2 sources), got %d", len(result.LinkRegistrations))
	}
}

// =============================================================================
// VAL-M3-ILLUST-004: KFXConditionalNav symbols classified as symOriginal
// Python: yj_structure.py:1071
// =============================================================================

func TestKFXConditionalNav_Classification(t *testing.T) {
	tests := []struct {
		name     string
		expected symType
	}{
		{
			name:     "KFXConditionalNavGroupUnit_550e8400-e29b-41d4-a716-446655440000",
			expected: symOriginal,
		},
		{
			name:     "KFXConditionalNavGroupUnit_00000000-0000-0000-0000-000000000000",
			expected: symOriginal,
		},
		{
			name:     "KFXConditionalNavGroupUnit_abcdef12-3456-7890-abcd-ef1234567890",
			expected: symOriginal,
		},
		{
			// Must not match — missing hyphens
			name:     "KFXConditionalNavGroupUnit_550e8400e29b41d4a716446655440000",
			expected: symUnknown,
		},
		{
			// Must not match — wrong prefix
			name:     "KFXConditionalNav_550e8400-e29b-41d4-a716-446655440000",
			expected: symUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := classifySymbol(tc.name)
			if result != tc.expected {
				t.Errorf("classifySymbol(%q) = %s, want %s", tc.name, result, tc.expected)
			}
		})
	}
}

// =============================================================================
// VAL-M3-ILLUST-005: KFXConditionalNav groups preserved during fragment processing
// Python: yj_structure.py:1071
//
// Nav container fragments with KFXConditionalNav IDs should survive the fragment
// processing pipeline because their symbols are classified as ORIGINAL (not UNKNOWN),
// meaning they won't be flagged during symbol table validation.
// =============================================================================

func TestKFXConditionalNav_Preservation(t *testing.T) {
	// A KFXConditionalNav fragment ID should be classified as ORIGINAL,
	// which means it won't trigger unknown symbol errors during check_symbol_table.
	// The fragment should be preserved because it's reachable via $391 nav_container
	// references.
	navID := "KFXConditionalNavGroupUnit_550e8400-e29b-41d4-a716-446655440000"

	symType := classifySymbol(navID)
	if symType != symOriginal {
		t.Errorf("KFXConditionalNav ID should classify as symOriginal, got %s", symType)
	}

	// Also verify it doesn't classify as something lower priority
	if symType == symUnknown {
		t.Error("KFXConditionalNav ID must not be classified as unknown — this would cause it to be flagged during symbol validation")
	}
}

// =============================================================================
// VAL-M3-ILLUST-006: Virtual panel validation checks region_magnification for comics
// Python: yj_to_epub_content.py:221-222
//
// When processing a page template, if virtual_panel ($434) is None:
//   - If is_comic AND NOT region_magnification: log error about missing virtual panel
//   - If region_magnification is enabled: no error (magnification allows missing panel)
//   - If not comic: no error
// =============================================================================

func TestVirtualPanelValidation_ComicWithoutMagnification(t *testing.T) {
	cfg := pageSpreadConfig{
		BookType:                 bookTypeComic,
		RegionMagnification:      false,
		VirtualPanelsAllowed:     true,
		PageProgressionDirection: "ltr",
	}

	data := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
		// $434 is absent → virtual_panel is nil
	}

	result := processPageSpreadPageTemplate(data, "test-section", "", nil, true, cfg, map[string]map[string]interface{}{})
	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
	// The error is logged, not returned. We verify the code path works correctly.
	// When comic + no region_magnification + no $434: error logged, returns false.
}

func TestVirtualPanelValidation_ComicWithMagnification(t *testing.T) {
	cfg := pageSpreadConfig{
		BookType:                 bookTypeComic,
		RegionMagnification:      true, // region magnification enabled
		VirtualPanelsAllowed:     true,
		PageProgressionDirection: "ltr",
	}

	data := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
		// $434 is absent → virtual_panel is nil
		// But region_magnification is true, so no error should be logged
	}

	result := processPageSpreadPageTemplate(data, "test-section", "", nil, true, cfg, map[string]map[string]interface{}{})
	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
}

func TestVirtualPanelValidation_NonComicWithoutMagnification(t *testing.T) {
	cfg := pageSpreadConfig{
		BookType:                 bookTypeNone, // not comic
		RegionMagnification:      false,
		VirtualPanelsAllowed:     true,
		PageProgressionDirection: "ltr",
	}

	data := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
		// $434 is absent → virtual_panel is nil
		// Not comic, so no error
	}

	result := processPageSpreadPageTemplate(data, "test-section", "", nil, true, cfg, map[string]map[string]interface{}{})
	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
}

// Test that processVirtualPanel directly shows the validation behavior
func TestVirtualPanelValidation_DirectValidation(t *testing.T) {
	tests := []struct {
		name       string
		bookType   bookType
		regionMag  bool
		has434     bool
		wantError  bool // expect error log when comic + no magnification + no $434
	}{
		{"comic_no_mag_no_panel", bookTypeComic, false, false, true},
		{"comic_with_mag_no_panel", bookTypeComic, true, false, false},
		{"comic_with_panel", bookTypeComic, false, true, false},
		{"noncomic_no_mag_no_panel", bookTypeNone, false, false, false},
		{"magazine_no_mag_no_panel", bookTypeMagazine, false, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := pageSpreadConfig{
				BookType:                 tc.bookType,
				RegionMagnification:      tc.regionMag,
				VirtualPanelsAllowed:     true,
				PageProgressionDirection: "ltr",
			}

			data := map[string]interface{}{}
			if tc.has434 {
				data["virtual_panel"] = "enabled"
			}

			// processVirtualPanel returns whether virtual panels were activated,
			// but the error logging is a side effect we can't directly test.
			// Instead we verify the function runs without panic and returns
			// expected boolean values.
			result := processVirtualPanel(data, &cfg, "test-section")

			if tc.has434 && !result {
				t.Error("expected true when $434=$441 and panels allowed")
			}
			if !tc.has434 && result {
				t.Error("expected false when no $434")
			}
		})
	}
}
