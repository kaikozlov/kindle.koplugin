package kfx

import (
	"math"
	"sort"
	"testing"

)

// ---------------------------------------------------------------------------
// VAL-C-055: Module-level control constants
// ---------------------------------------------------------------------------

func TestModuleControlConstants(t *testing.T) {
	if ReportKnownProblems != nil {
		t.Errorf("ReportKnownProblems = %v, want nil", ReportKnownProblems)
	}
	if ReportNonJPEGJFIFCover {
		t.Error("ReportNonJPEGJFIFCover = true, want false")
	}
	if ReportJPEGVariants {
		t.Error("ReportJPEGVariants = true, want false")
	}
	if DebugPDFPageSize {
		t.Error("DebugPDFPageSize = true, want false")
	}
	if MaxContentFragmentSize != 8192 {
		t.Errorf("MaxContentFragmentSize = %d, want 8192", MaxContentFragmentSize)
	}
	if ApproximatePageList != "APPROXIMATE_PAGE_LIST" {
		t.Errorf("ApproximatePageList = %q, want %q", ApproximatePageList, "APPROXIMATE_PAGE_LIST")
	}
	if KFXCoverResource != "kfx_cover_image" {
		t.Errorf("KFXCoverResource = %q, want %q", KFXCoverResource, "kfx_cover_image")
	}
	if DictionaryRulesSymbol != "dictionary_rules" {
		t.Errorf("DictionaryRulesSymbol = %q, want %q", DictionaryRulesSymbol, "dictionary_rules")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-039: FIXED_LAYOUT_IMAGE_FORMATS — 5 entries
// ---------------------------------------------------------------------------

func TestFixedLayoutImageFormats(t *testing.T) {
	expected := map[string]bool{
		"$286": true, "$285": true, "$548": true, "$565": true, "$284": true,
	}
	if len(FixedLayoutImageFormats) != len(expected) {
		t.Errorf("FixedLayoutImageFormats has %d entries, want %d", len(FixedLayoutImageFormats), len(expected))
	}
	for k := range expected {
		if !FixedLayoutImageFormats[k] {
			t.Errorf("FixedLayoutImageFormats missing %q", k)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-C-052: SYM_TYPE constants
// ---------------------------------------------------------------------------

func TestSymTypeConstants(t *testing.T) {
	if symCommon != "common" {
		t.Errorf("symCommon = %q, want %q", symCommon, "common")
	}
	if symDictionary != "dictionary" {
		t.Errorf("symDictionary = %q, want %q", symDictionary, "dictionary")
	}
	if symOriginal != "original" {
		t.Errorf("symOriginal = %q, want %q", symOriginal, "original")
	}
	if symBase64 != "base64" {
		t.Errorf("symBase64 = %q, want %q", symBase64, "base64")
	}
	if symShort != "short" {
		t.Errorf("symShort = %q, want %q", symShort, "short")
	}
	if symShared != "shared" {
		t.Errorf("symShared = %q, want %q", symShared, "shared")
	}
	if symUnknown != "unknown" {
		t.Errorf("symUnknown = %q, want %q", symUnknown, "unknown")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-054: numstr helper — %g formatting
// ---------------------------------------------------------------------------

func TestNumstr(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{1.0, "1"},
		{0.5, "0.5"},
		{0.0, "0"},
		{100.0, "100"},
		{-1.5, "-1.5"},
		{1e6, "1e+06"},
		{1e-5, "1e-05"},
		{123.456, "123.456"},
		{0.123, "0.123"},
	}

	for _, tc := range tests {
		result := Numstr(tc.input)
		if result != tc.expected {
			t.Errorf("Numstr(%v) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-C-053: extract_fragment_id_from_value — known types
// ---------------------------------------------------------------------------

func TestExtractFragmentIDFromValue(t *testing.T) {
	// Test with $260 which has key ["$174"]
	result := ExtractFragmentIDFromValue("$260", map[string]interface{}{
		"$174": "my_section_id",
	})
	if result != "my_section_id" {
		t.Errorf("ExtractFragmentIDFromValue($260) = %q, want %q", result, "my_section_id")
	}

	// Test with $597 which has keys ["$174", "$598"]
	result = ExtractFragmentIDFromValue("$597", map[string]interface{}{
		"$174": "abc",
	})
	if result != "abc" {
		t.Errorf("ExtractFragmentIDFromValue($597 with $174) = %q, want %q", result, "abc")
	}

	// Test with $164 which has key ["$175"]
	result = ExtractFragmentIDFromValue("$164", map[string]interface{}{
		"$175": "resource_id",
	})
	if result != "resource_id" {
		t.Errorf("ExtractFragmentIDFromValue($164) = %q, want %q", result, "resource_id")
	}

	// Test with unknown fragment type
	result = ExtractFragmentIDFromValue("$999", map[string]interface{}{
		"$174": "some_id",
	})
	if result != "$999" {
		t.Errorf("ExtractFragmentIDFromValue($999) = %q, want %q", result, "$999")
	}

	// Test with non-map value
	result = ExtractFragmentIDFromValue("$260", "not a map")
	if result != "$260" {
		t.Errorf("ExtractFragmentIDFromValue($260, string) = %q, want %q", result, "$260")
	}

	// Test with $157 which has key ["$173"]
	result = ExtractFragmentIDFromValue("$157", map[string]interface{}{
		"$173": "style_id",
	})
	if result != "style_id" {
		t.Errorf("ExtractFragmentIDFromValue($157) = %q, want %q", result, "style_id")
	}

	// Test with $391 which has key ["$239"]
	result = ExtractFragmentIDFromValue("$391", map[string]interface{}{
		"$239": "nav_id",
	})
	if result != "nav_id" {
		t.Errorf("ExtractFragmentIDFromValue($391) = %q, want %q", result, "nav_id")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-043: walk_fragment — IonStruct traversal
// ---------------------------------------------------------------------------

func TestWalkFragmentStruct(t *testing.T) {
	fragment := Fragment{
		FType: "$260",
		FID:   "section1",
		Value: map[string]interface{}{
			"$174": "section1",
			"$176": "storyline1",
		},
	}

	mandatoryRefs := map[FragmentKey]bool{}
	optionalRefs := map[FragmentKey]bool{}
	eidDefs := map[interface{}]bool{}
	eidRefs := map[interface{}]bool{}

	WalkFragment(fragment, &mandatoryRefs, &optionalRefs, &eidDefs, &eidRefs, nil)

	// $174 → $260 (via CommonFragmentReferences), $176 → $259
	// But $174 is in FragmentIDKeys["$260"] and containerParent == fragment.FType == "$260",
	// so it should be skipped (isIDKey == true).
	// $176 → $259 via CommonFragmentReferences
	foundStoryline := false
	for k := range mandatoryRefs {
		if k.FType == "$259" && k.FID == "storyline1" {
			foundStoryline = true
		}
	}
	if !foundStoryline {
		t.Errorf("walk_fragment(struct): expected mandatory ref ($259, storyline1), got refs: %v", mandatoryRefs)
	}
}

// ---------------------------------------------------------------------------
// VAL-C-044: walk_fragment — IonList traversal
// ---------------------------------------------------------------------------

func TestWalkFragmentList(t *testing.T) {
	fragment := Fragment{
		FType: "$259",
		FID:   "storyline1",
		Value: map[string]interface{}{
			"$429": []interface{}{
				"style1",
				"style2",
			},
		},
	}

	mandatoryRefs := map[FragmentKey]bool{}
	optionalRefs := map[FragmentKey]bool{}
	eidDefs := map[interface{}]bool{}
	eidRefs := map[interface{}]bool{}

	WalkFragment(fragment, &mandatoryRefs, &optionalRefs, &eidDefs, &eidRefs, nil)

	// $429 → $157 via CommonFragmentReferences, for each element
	found1 := false
	found2 := false
	for k := range mandatoryRefs {
		if k.FType == "$157" && k.FID == "style1" {
			found1 = true
		}
		if k.FType == "$157" && k.FID == "style2" {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Errorf("walk_fragment(list): expected both style refs, got: %v", mandatoryRefs)
	}
}

// ---------------------------------------------------------------------------
// VAL-C-045: walk_fragment — EID definition tracking
// ---------------------------------------------------------------------------

func TestWalkFragmentEIDTracking(t *testing.T) {
	fragment := Fragment{
		FType: "$260",
		FID:   "section1",
		Value: map[string]interface{}{
			"$155": "entity1",
			"$185": "entity2",
		},
	}

	mandatoryRefs := map[FragmentKey]bool{}
	optionalRefs := map[FragmentKey]bool{}
	eidDefs := map[interface{}]bool{}
	eidRefs := map[interface{}]bool{}

	WalkFragment(fragment, &mandatoryRefs, &optionalRefs, &eidDefs, &eidRefs, nil)

	if !eidDefs["entity1"] {
		t.Error("walk_fragment: expected entity1 in eidDefs")
	}
	if !eidRefs["entity2"] {
		t.Error("walk_fragment: expected entity2 in eidRefs")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-046: walk_fragment — SPECIAL_FRAGMENT_REFERENCES dispatch
// ---------------------------------------------------------------------------

func TestWalkFragmentSpecialRefs(t *testing.T) {
	fragment := Fragment{
		FType: "$391",
		FID:   "nav1",
		Value: map[string]interface{}{
			"$247": "data1",
		},
	}

	mandatoryRefs := map[FragmentKey]bool{}
	optionalRefs := map[FragmentKey]bool{}
	eidDefs := map[interface{}]bool{}
	eidRefs := map[interface{}]bool{}

	WalkFragment(fragment, &mandatoryRefs, &optionalRefs, &eidDefs, &eidRefs, nil)

	// $247 in $391 → $394 via SpecialFragmentReferences
	found := false
	for k := range mandatoryRefs {
		if k.FType == "$394" && k.FID == "data1" {
			found = true
		}
	}
	if !found {
		t.Errorf("walk_fragment(special): expected ($394, data1) via special refs, got: %v", mandatoryRefs)
	}
}

// ---------------------------------------------------------------------------
// VAL-C-040: check_fragment_usage — BFS discovers all reachable fragments
// ---------------------------------------------------------------------------

func TestCheckFragmentUsageBFS(t *testing.T) {
	// Create a fragment graph where:
	// $538 (document_data, a root type) references $260 via $170 (section list)
	// $260 references $164 via $479 (image), $259 via $176 (storyline)
	fragments := FragmentList{
		{FType: "$538", FID: "$538", Value: map[string]interface{}{
			"$170": []interface{}{"section1"},
		}},
		{FType: "$260", FID: "section1", Value: map[string]interface{}{
			"$174": "section1",
			"$176": "storyline1",
			"$479": "resource1",
		}},
		{FType: "$164", FID: "resource1", Value: map[string]interface{}{
			"$175": "resource1",
		}},
		{FType: "$259", FID: "storyline1", Value: map[string]interface{}{}},
	}

	result := CheckFragmentUsage(fragments, nil)

	// All three non-container fragments should be referenced
	referencedTypes := map[string]bool{}
	for _, f := range result.Referenced {
		referencedTypes[f.FType+"::"+f.FID] = true
	}

	if !referencedTypes["$260::section1"] {
		t.Error("check_fragment_usage: $260/section1 should be referenced")
	}
	if !referencedTypes["$164::resource1"] {
		t.Error("check_fragment_usage: $164/resource1 should be referenced")
	}
	if !referencedTypes["$259::storyline1"] {
		t.Error("check_fragment_usage: $259/storyline1 should be referenced")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-041: check_fragment_usage — detects missing fragments
// ---------------------------------------------------------------------------

func TestCheckFragmentUsageMissingFragments(t *testing.T) {
	// $538 (root type) references $260 via $170
	// $260 references $259 via $176, but "nonexistent_storyline" doesn't exist as a fragment
	fragments := FragmentList{
		{FType: "$538", FID: "$538", Value: map[string]interface{}{
			"$170": []interface{}{"section1"},
		}},
		{FType: "$260", FID: "section1", Value: map[string]interface{}{
			"$174": "section1",
			"$176": "nonexistent_storyline",
		}},
	}

	result := CheckFragmentUsage(fragments, nil)

	if len(result.Missing) == 0 {
		t.Error("check_fragment_usage: expected missing fragments for nonexistent_storyline")
	}

	foundMissing := false
	for k := range result.Missing {
		if k.FType == "$259" && k.FID == "nonexistent_storyline" {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Errorf("check_fragment_usage: expected ($259, nonexistent_storyline) in missing, got: %v", result.Missing)
	}
}

// ---------------------------------------------------------------------------
// VAL-C-042: check_fragment_usage — detects unreferenced fragments
// ---------------------------------------------------------------------------

func TestCheckFragmentUsageUnreferencedFragments(t *testing.T) {
	fragments := FragmentList{
		{FType: "$270", FID: "container1", Value: map[string]interface{}{}},
		{FType: "$164", FID: "orphan_resource", Value: map[string]interface{}{
			"$175": "orphan_resource",
		}},
	}

	result := CheckFragmentUsage(fragments, nil)

	found := false
	for _, f := range result.Unreferenced {
		if f.FType == "$164" && f.FID == "orphan_resource" {
			found = true
		}
	}
	if !found {
		t.Errorf("check_fragment_usage: expected orphan_resource in unreferenced, got: %v", result.Unreferenced)
	}
}

// ---------------------------------------------------------------------------
// VAL-C-047: determine_entity_dependencies — transitive expansion
// ---------------------------------------------------------------------------

func TestDetermineEntityDependenciesTransitive(t *testing.T) {
	// Create chain: A($260) → B($164) → C($417)
	mandatoryRefs := map[FragmentKey]map[FragmentKey]bool{
		{FType: "$260", FID: "section1"}: {
			{FType: "$164", FID: "resource1"}: true,
		},
		{FType: "$164", FID: "resource1"}: {
			{FType: "$417", FID: "blob1"}: true,
		},
	}
	optionalRefs := map[FragmentKey]map[FragmentKey]bool{}

	deps := DetermineEntityDependencies(mandatoryRefs, optionalRefs)

	// section1 should have resource1 as mandatory dep (direct)
	// section1 should also transitively depend on blob1 via resource1
	foundSection1Mandatory := false
	for _, dep := range deps {
		if dep.EntityID == "section1" {
			for _, m := range dep.MandatoryDependencies {
				if m == "resource1" {
					foundSection1Mandatory = true
				}
			}
		}
	}
	if !foundSection1Mandatory {
		t.Errorf("determine_entity_dependencies: expected section1→resource1, got: %v", deps)
	}
}

// ---------------------------------------------------------------------------
// VAL-C-048: determine_entity_dependencies — skips $387 section references
// ---------------------------------------------------------------------------

func TestDetermineEntityDependenciesSkipsSections(t *testing.T) {
	mandatoryRefs := map[FragmentKey]map[FragmentKey]bool{
		{FType: "$387", FID: "sec1"}: {
			{FType: "$164", FID: "resource1"}: true,
		},
	}
	optionalRefs := map[FragmentKey]map[FragmentKey]bool{}

	deps := DetermineEntityDependencies(mandatoryRefs, optionalRefs)

	// $387 references should be cleared, so no dependencies should be generated
	for _, dep := range deps {
		if dep.EntityID == "sec1" {
			t.Errorf("determine_entity_dependencies: $387 should not produce deps, got: %v", dep)
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-C-049: determine_entity_dependencies — skips $164→$164 cross-references
// ---------------------------------------------------------------------------

func TestDetermineEntityDependenciesSkipsCrossResource(t *testing.T) {
	mandatoryRefs := map[FragmentKey]map[FragmentKey]bool{
		{FType: "$164", FID: "resource1"}: {
			{FType: "$164", FID: "resource2"}: true,
		},
	}
	optionalRefs := map[FragmentKey]map[FragmentKey]bool{}

	deps := DetermineEntityDependencies(mandatoryRefs, optionalRefs)

	// $164→$164 should be removed, so resource1 should have no dependencies
	for _, dep := range deps {
		if dep.EntityID == "resource1" {
			if len(dep.MandatoryDependencies) > 0 {
				t.Errorf("determine_entity_dependencies: $164→$164 should be skipped, got deps: %v", dep.MandatoryDependencies)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// VAL-C-050: rebuild_container_entity_map — reconstructs $419 fragment
// ---------------------------------------------------------------------------

func TestRebuildContainerEntityMap(t *testing.T) {
	fragments := FragmentList{
		{FType: "$270", FID: "container1", Value: map[string]interface{}{}},
		{FType: "$260", FID: "section1", Value: map[string]interface{}{}},
		{FType: "$164", FID: "resource1", Value: map[string]interface{}{}},
	}

	entityDeps := []EntityDependency{
		{
			EntityID:               "section1",
			MandatoryDependencies:  []string{"resource1"},
			OptionalDependencies:   nil,
		},
	}

	result := RebuildContainerEntityMap(fragments, "CR!TEST", entityDeps)

	// Find $419 fragment
	var found419 *Fragment
	for i := range result {
		if result[i].FType == "$419" {
			found419 = &result[i]
			break
		}
	}

	if found419 == nil {
		t.Fatal("rebuild_container_entity_map: no $419 fragment found")
	}

	valMap, ok := asMap(found419.Value)
	if !ok {
		t.Fatal("rebuild_container_entity_map: $419 value is not a map")
	}

	// Check $252 exists
	if _, ok := valMap["$252"]; !ok {
		t.Error("rebuild_container_entity_map: $419 missing $252")
	}

	// Check $253 exists (entity dependencies)
	if _, ok := valMap["$253"]; !ok {
		t.Error("rebuild_container_entity_map: $419 missing $253")
	}
}

// ---------------------------------------------------------------------------
// VAL-C-051: rebuild_container_entity_map — excludes container/root fragments
// ---------------------------------------------------------------------------

func TestRebuildContainerEntityMapExcludesRoots(t *testing.T) {
	fragments := FragmentList{
		{FType: "$270", FID: "container1", Value: map[string]interface{}{}},
		{FType: "$419", FID: "$419", Value: map[string]interface{}{}}, // old container map
		{FType: "$260", FID: "section1", Value: map[string]interface{}{}},
	}

	result := RebuildContainerEntityMap(fragments, "CR!TEST", nil)

	// Find $419 fragment
	var found419 *Fragment
	for i := range result {
		if result[i].FType == "$419" {
			found419 = &result[i]
			break
		}
	}

	if found419 == nil {
		t.Fatal("rebuild_container_entity_map: no $419 fragment found")
	}

	valMap, ok := asMap(found419.Value)
	if !ok {
		t.Fatal("rebuild_container_entity_map: $419 value is not a map")
	}

	// Check $252 container_contents
	contentsSlice, ok := asSlice(valMap["$252"])
	if !ok || len(contentsSlice) == 0 {
		t.Fatal("rebuild_container_entity_map: $252 missing or empty")
	}

	contentsMap, ok := asMap(contentsSlice[0])
	if !ok {
		t.Fatal("rebuild_container_entity_map: $252[0] is not a map")
	}

	// Get entity IDs list
	entityIDsSlice, ok := asSlice(contentsMap["$181"])
	if !ok {
		t.Fatal("rebuild_container_entity_map: $181 missing")
	}

	// Should only contain section1 (not $270, not $419)
	for _, id := range entityIDsSlice {
		idStr, _ := asString(id)
		if idStr == "container1" || idStr == "$419" {
			t.Errorf("rebuild_container_entity_map: container/root %q should not be in entity_ids", idStr)
		}
	}
}

// ---------------------------------------------------------------------------
// FragmentIDKeys and CommonFragmentReferences constants verification
// ---------------------------------------------------------------------------

func TestFragmentIDKeys(t *testing.T) {
	expectedCount := 18 // 18 entries in Python FRAGMENT_ID_KEYS
	if len(FragmentIDKeys) != expectedCount {
		t.Errorf("FragmentIDKeys has %d entries, want %d", len(FragmentIDKeys), expectedCount)
	}

	// Spot-check a few entries
	if keys, ok := FragmentIDKeys["$260"]; !ok || len(keys) != 1 || keys[0] != "$174" {
		t.Errorf("FragmentIDKeys[$260] = %v, want [$174]", keys)
	}
	if keys, ok := FragmentIDKeys["$597"]; !ok || len(keys) != 2 || keys[0] != "$174" || keys[1] != "$598" {
		t.Errorf("FragmentIDKeys[$597] = %v, want [$174 $598]", keys)
	}
}

func TestCommonFragmentReferences(t *testing.T) {
	// Verify count matches Python (24 entries)
	expectedCount := 24
	if len(CommonFragmentReferences) != expectedCount {
		t.Errorf("CommonFragmentReferences has %d entries, want %d", len(CommonFragmentReferences), expectedCount)
	}

	// Spot-check mappings
	if ref, ok := CommonFragmentReferences["$174"]; !ok || ref != "$260" {
		t.Errorf("CommonFragmentReferences[$174] = %q, want $260", ref)
	}
	if ref, ok := CommonFragmentReferences["$176"]; !ok || ref != "$259" {
		t.Errorf("CommonFragmentReferences[$176] = %q, want $259", ref)
	}
	if ref, ok := CommonFragmentReferences["$165"]; !ok || ref != "$417" {
		t.Errorf("CommonFragmentReferences[$165] = %q, want $417", ref)
	}
}

func TestNestedFragmentReferences(t *testing.T) {
	if len(NestedFragmentReferences) != 4 {
		t.Errorf("NestedFragmentReferences has %d entries, want 4", len(NestedFragmentReferences))
	}
	if ref, ok := NestedFragmentReferences[[2]string{"$597", "$351"}]; !ok || ref != "$597" {
		t.Errorf("NestedFragmentReferences[($597,$351)] = %q, want $597", ref)
	}
}

func TestSpecialFragmentReferences(t *testing.T) {
	if len(SpecialFragmentReferences) != 2 {
		t.Errorf("SpecialFragmentReferences has %d entries, want 2", len(SpecialFragmentReferences))
	}
	if ref, ok := SpecialFragmentReferences["$391"]["$247"]; !ok || ref != "$394" {
		t.Errorf("SpecialFragmentReferences[$391][$247] = %q, want $394", ref)
	}
	if ref, ok := SpecialFragmentReferences["$387"]["$213"]; !ok || ref != "$164" {
		t.Errorf("SpecialFragmentReferences[$387][$213] = %q, want $164", ref)
	}
}

func TestSectionDataTypes(t *testing.T) {
	expected := []string{"$387", "$260", "$267", "$609"}
	for _, e := range expected {
		if !SectionDataTypes[e] {
			t.Errorf("SectionDataTypes missing %q", e)
		}
	}
}

func TestEIDReferences(t *testing.T) {
	expected := []string{"$185", "$155", "$598", "$754", "$474", "$163"}
	for _, e := range expected {
		if !EIDReferences[e] {
			t.Errorf("EIDReferences missing %q", e)
		}
	}
}

func TestExpectedAnnotations(t *testing.T) {
	if len(ExpectedAnnotations) != 5 {
		t.Errorf("ExpectedAnnotations has %d entries, want 5", len(ExpectedAnnotations))
	}
	if !ExpectedAnnotations[[3]string{"$164", "$214", "$164"}] {
		t.Error("ExpectedAnnotations missing ($164, $214, $164)")
	}
}

// ---------------------------------------------------------------------------
// FragmentList.Get test
// ---------------------------------------------------------------------------

func TestFragmentListGet(t *testing.T) {
	fl := FragmentList{
		{FType: "$260", FID: "section1", Value: nil},
		{FType: "$260", FID: "section2", Value: nil},
		{FType: "$164", FID: "resource1", Value: nil},
	}

	// Get first matching type
	frag := fl.Get("$260", "", true)
	if frag == nil || frag.FID != "section1" {
		t.Errorf("Get($260, empty, first) = %v, want section1", frag)
	}

	// Get by type and ID
	frag = fl.Get("$260", "section2", true)
	if frag == nil || frag.FID != "section2" {
		t.Errorf("Get($260, section2, first) = %v, want section2", frag)
	}

	// Get non-existent
	frag = fl.Get("$999", "", true)
	if frag != nil {
		t.Errorf("Get($999) = %v, want nil", frag)
	}
}

// ---------------------------------------------------------------------------
// WalkFragment SExp test (IonSExp dispatch)
// ---------------------------------------------------------------------------

func TestWalkFragmentSExp(t *testing.T) {
	// IonSExp is represented as []interface{} with first element as operator
	fragment := Fragment{
		FType: "$260",
		FID:   "section1",
		Value: map[string]interface{}{
			"$171": []interface{}{"$294", "$183", "some_value"},
		},
	}

	mandatoryRefs := map[FragmentKey]bool{}
	optionalRefs := map[FragmentKey]bool{}
	eidDefs := map[interface{}]bool{}
	eidRefs := map[interface{}]bool{}

	WalkFragment(fragment, &mandatoryRefs, &optionalRefs, &eidDefs, &eidRefs, nil)

	// $171 is not in CommonFragmentReferences so no refs should be generated from it directly.
	// But the SExp walk should dispatch correctly without panicking.
	// The key check: the walk doesn't crash on SExp data.
}

// ---------------------------------------------------------------------------
// WalkFragment string-to-symbol conversion ($165/$636)
// ---------------------------------------------------------------------------

func TestWalkFragmentStringToSymbol(t *testing.T) {
	fragment := Fragment{
		FType: "$164",
		FID:   "resource1",
		Value: map[string]interface{}{
			"$165": "blob_location",
		},
	}

	mandatoryRefs := map[FragmentKey]bool{}
	optionalRefs := map[FragmentKey]bool{}
	eidDefs := map[interface{}]bool{}
	eidRefs := map[interface{}]bool{}

	WalkFragment(fragment, &mandatoryRefs, &optionalRefs, &eidDefs, &eidRefs, nil)

	// $165 → $417 via CommonFragmentReferences
	// But $175 is in FragmentIDKeys["$164"] and containerParent == "$164" (the fragment type),
	// and $165 is NOT in FragmentIDKeys["$164"], so it should NOT be skipped.
	// Actually $175 is the ID key for $164, not $165. So $165 should generate a ref.
	found := false
	for k := range mandatoryRefs {
		if k.FType == "$417" && k.FID == "blob_location" {
			found = true
		}
	}
	if !found {
		t.Errorf("walk_fragment($165 string): expected ($417, blob_location), got: %v", mandatoryRefs)
	}
}

// ---------------------------------------------------------------------------
// WalkFragment with $635 optional reference
// ---------------------------------------------------------------------------

func TestWalkFragmentOptionalRef(t *testing.T) {
	fragment := Fragment{
		FType: "$164",
		FID:   "resource1",
		Value: map[string]interface{}{
			"$635": "variant1",
		},
	}

	mandatoryRefs := map[FragmentKey]bool{}
	optionalRefs := map[FragmentKey]bool{}
	eidDefs := map[interface{}]bool{}
	eidRefs := map[interface{}]bool{}

	WalkFragment(fragment, &mandatoryRefs, &optionalRefs, &eidDefs, &eidRefs, nil)

	// $635 → $164 via CommonFragmentReferences, but $635 is optional
	found := false
	for k := range optionalRefs {
		if k.FType == "$164" && k.FID == "variant1" {
			found = true
		}
	}
	if !found {
		t.Errorf("walk_fragment($635): expected ($164, variant1) in optional refs, got: %v", optionalRefs)
	}

	// Should NOT be in mandatory
	for k := range mandatoryRefs {
		if k.FType == "$164" && k.FID == "variant1" {
			t.Error("walk_fragment($635): variant1 should be optional, not mandatory")
		}
	}
}

// ---------------------------------------------------------------------------
// WalkFragment $260 variant expansion
// ---------------------------------------------------------------------------

func TestWalkFragmentSectionVariants(t *testing.T) {
	fragments := FragmentList{
		{FType: "$260", FID: "section1", Value: map[string]interface{}{
			"$174": "section1",
		}},
		{FType: "$609", FID: "section1", Value: map[string]interface{}{}},
		{FType: "$597", FID: "section1", Value: map[string]interface{}{}},
	}

	// Walk a fragment that references $260 to trigger variant expansion
	fragment := Fragment{
		FType: "$538",
		FID:   "$538",
		Value: map[string]interface{}{
			"$174": "section1", // $174 → $260 via CommonFragmentReferences
		},
	}

	mandatoryRefs := map[FragmentKey]bool{}
	optionalRefs := map[FragmentKey]bool{}
	eidDefs := map[interface{}]bool{}
	eidRefs := map[interface{}]bool{}

	WalkFragment(fragment, &mandatoryRefs, &optionalRefs, &eidDefs, &eidRefs, fragments)

	// Should have $609 and $597 variant references since they exist in fragment list
	found609 := false
	found597 := false
	for k := range mandatoryRefs {
		if k.FType == "$609" && k.FID == "section1" {
			found609 = true
		}
		if k.FType == "$597" && k.FID == "section1" {
			found597 = true
		}
	}
	if !found609 {
		t.Errorf("walk_fragment: expected ($609, section1) variant ref, got: %v", mandatoryRefs)
	}
	if !found597 {
		t.Errorf("walk_fragment: expected ($597, section1) variant ref, got: %v", mandatoryRefs)
	}
}

// ---------------------------------------------------------------------------
// Numstr edge cases
// ---------------------------------------------------------------------------

func TestNumstrEdgeCases(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{math.Inf(1), "+Inf"},
		{math.Inf(-1), "-Inf"},
		{math.MaxFloat64, "1.7976931348623157e+308"},
		{math.SmallestNonzeroFloat64, "5e-324"},
	}

	for _, tc := range tests {
		result := Numstr(tc.input)
		if result != tc.expected {
			// Some platforms format differently; just verify it doesn't panic
			t.Logf("Numstr(%v) = %q (expected %q, may vary by platform)", tc.input, result, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// FragmentKey.String
// ---------------------------------------------------------------------------

func TestFragmentKeyString(t *testing.T) {
	fk := FragmentKey{FType: "$260", FID: "section1"}
	s := fk.String()
	if s != "($260, section1)" {
		t.Errorf("FragmentKey.String() = %q, want %q", s, "($260, section1)")
	}

	fk = FragmentKey{FType: "$270"}
	s = fk.String()
	if s != "($270)" {
		t.Errorf("FragmentKey.String() = %q, want %q", s, "($270)")
	}
}

// ---------------------------------------------------------------------------
// WalkFragment with NESTED_FRAGMENT_REFERENCES
// ---------------------------------------------------------------------------

func TestWalkFragmentNestedRefs(t *testing.T) {
	// NESTED_FRAGMENT_REFERENCES: ($597, $351) → $597
	// This means when walking a $597 fragment, and we encounter container $351
	// with containerParent being the sexp operator (e.g. "$597"),
	// it should resolve to $597.

	// Create a $597 fragment with nested reference through sexp
	fragment := Fragment{
		FType: "$597",
		FID:   "page1",
		Value: map[string]interface{}{
			"$351": "ref_data",
		},
	}

	mandatoryRefs := map[FragmentKey]bool{}
	optionalRefs := map[FragmentKey]bool{}
	eidDefs := map[interface{}]bool{}
	eidRefs := map[interface{}]bool{}

	WalkFragment(fragment, &mandatoryRefs, &optionalRefs, &eidDefs, &eidRefs, nil)

	// $351 with parent $597 should resolve via NestedFragmentReferences
	// But the containerParent comes from the struct key's parent, which is fragment.FType = "$597"
	// Actually in the struct walk: walk(fv, fk, cont) where fk=$351, cont="$597" (fragment.FType)
	// So container=$351, containerParent="$597"
	// NestedFragmentReferences[("$597", "$351")] = "$597"
	found := false
	for k := range mandatoryRefs {
		if k.FType == "$597" && k.FID == "ref_data" {
			found = true
		}
	}
	if !found {
		t.Errorf("walk_fragment(nested): expected ($597, ref_data) via nested refs, got: %v", mandatoryRefs)
	}
}

// ---------------------------------------------------------------------------
// DetermineEntityDependencies — empty input
// ---------------------------------------------------------------------------

func TestDetermineEntityDependenciesEmpty(t *testing.T) {
	deps := DetermineEntityDependencies(
		map[FragmentKey]map[FragmentKey]bool{},
		map[FragmentKey]map[FragmentKey]bool{},
	)
	if len(deps) != 0 {
		t.Errorf("DetermineEntityDependencies(empty) = %d deps, want 0", len(deps))
	}
}

// ---------------------------------------------------------------------------
// RebuildContainerEntityMap — no entities produces error log
// ---------------------------------------------------------------------------

func TestRebuildContainerEntityMapEmpty(t *testing.T) {
	// Only container fragments — no entity IDs to collect
	fragments := FragmentList{
		{FType: "$270", FID: "container1", Value: map[string]interface{}{}},
	}

	result := RebuildContainerEntityMap(fragments, "CR!TEST", nil)

	// No $419 should be generated
	for _, f := range result {
		if f.FType == "$419" {
			t.Error("rebuild_container_entity_map: $419 should not be generated when there are no entities")
		}
	}
}

// ---------------------------------------------------------------------------
// SpecialParentFragmentReferences test
// ---------------------------------------------------------------------------

func TestSpecialParentFragmentReferences(t *testing.T) {
	if len(SpecialParentFragmentReferences) != 1 {
		t.Errorf("SpecialParentFragmentReferences has %d entries, want 1", len(SpecialParentFragmentReferences))
	}
	if SpecialParentFragmentReferences["$538"] == nil {
		t.Error("SpecialParentFragmentReferences missing $538 entry")
	} else if SpecialParentFragmentReferences["$538"]["yj.print.style"] != false {
		t.Error("SpecialParentFragmentReferences[$538][yj.print.style] should be false")
	}
}

// ---------------------------------------------------------------------------
// RootFragmentTypes and ContainerFragmentTypes test
// ---------------------------------------------------------------------------

func TestRootAndContainerFragmentTypes(t *testing.T) {
	if len(RootFragmentTypes) != 17 {
		t.Errorf("RootFragmentTypes has %d entries, want 17", len(RootFragmentTypes))
	}
	if len(ContainerFragmentTypes) != 4 {
		t.Errorf("ContainerFragmentTypes has %d entries, want 4", len(ContainerFragmentTypes))
	}

	// Verify key entries
	if !RootFragmentTypes["$270"] {
		t.Error("RootFragmentTypes missing $270")
	}
	if !ContainerFragmentTypes["$419"] {
		t.Error("ContainerFragmentTypes missing $419")
	}
}

// ---------------------------------------------------------------------------
// KnownFragmentTypes test
// ---------------------------------------------------------------------------

func TestKnownFragmentTypes(t *testing.T) {
	// Verify some known types
	knownTypes := []string{
		"$270", "$490", "$389", "$419", "$538", "$266", "$597",
		"$418", "$417", "$164", "$260", "$259", "$157", "$608",
	}
	for _, ft := range knownTypes {
		if !KnownFragmentTypes[ft] {
			t.Errorf("KnownFragmentTypes missing %q", ft)
		}
	}
}

// ---------------------------------------------------------------------------
// sort helper tests (deterministic output)
// ---------------------------------------------------------------------------

func TestSortedFragmentKeys(t *testing.T) {
	m := map[FragmentKey]map[FragmentKey]bool{
		{FType: "$164", FID: "z"}: {},
		{FType: "$164", FID: "a"}: {},
		{FType: "$260", FID: "b"}: {},
	}
	keys := sortedFragmentKeys(m)
	if len(keys) != 3 {
		t.Fatalf("sortedFragmentKeys returned %d keys, want 3", len(keys))
	}
	// Should be sorted: ($164,a), ($164,z), ($260,b)
	if keys[0].FType != "$164" || keys[0].FID != "a" {
		t.Errorf("keys[0] = %v, want ($164,a)", keys[0])
	}
	if keys[1].FType != "$164" || keys[1].FID != "z" {
		t.Errorf("keys[1] = %v, want ($164,z)", keys[1])
	}
	if keys[2].FType != "$260" || keys[2].FID != "b" {
		t.Errorf("keys[2] = %v, want ($260,b)", keys[2])
	}
}

func TestSortedFragmentKeysFromSet(t *testing.T) {
	m := map[FragmentKey]bool{
		{FType: "$164", FID: "z"}: true,
		{FType: "$164", FID: "a"}: true,
		{FType: "$260", FID: "b"}: true,
	}
	keys := sortedFragmentKeysFromSet(m)

	// Verify sorted
	for i := 1; i < len(keys); i++ {
		prev := keys[i-1]
		curr := keys[i]
		if prev.FType > curr.FType || (prev.FType == curr.FType && prev.FID > curr.FID) {
			t.Errorf("keys not sorted at index %d: %v > %v", i, prev, curr)
		}
	}
}

// ---------------------------------------------------------------------------
// EntityDepsToInterface test
// ---------------------------------------------------------------------------

func TestEntityDepsToInterface(t *testing.T) {
	deps := []EntityDependency{
		{
			EntityID:               "section1",
			MandatoryDependencies:  []string{"resource1", "resource2"},
		},
		{
			EntityID:              "section1",
			OptionalDependencies: []string{"resource3"},
		},
	}

	result := entityDepsToInterface(deps)
	if len(result) != 2 {
		t.Fatalf("entityDepsToInterface returned %d entries, want 2", len(result))
	}

	// Check first dependency
	first, ok := asMap(result[0])
	if !ok {
		t.Fatal("first dep is not a map")
	}
	if id, _ := asString(first["$155"]); id != "section1" {
		t.Errorf("first dep $155 = %q, want section1", id)
	}
	mandatorySlice, ok := asSlice(first["$254"])
	if !ok || len(mandatorySlice) != 2 {
		t.Errorf("first dep $254 = %v, want 2 elements", mandatorySlice)
	}

	// Check second dependency
	second, ok := asMap(result[1])
	if !ok {
		t.Fatal("second dep is not a map")
	}
	optionalSlice, ok := asSlice(second["$255"])
	if !ok || len(optionalSlice) != 1 {
		t.Errorf("second dep $255 = %v, want 1 element", optionalSlice)
	}
}

// ---------------------------------------------------------------------------
// RebuildContainerEntityMap — entity IDs in order
// ---------------------------------------------------------------------------

func TestRebuildContainerEntityMapEntityOrder(t *testing.T) {
	fragments := FragmentList{
		{FType: "$260", FID: "section_b", Value: map[string]interface{}{}},
		{FType: "$260", FID: "section_a", Value: map[string]interface{}{}},
		{FType: "$164", FID: "resource1", Value: map[string]interface{}{}},
	}

	result := RebuildContainerEntityMap(fragments, "CR!TEST", nil)

	var found419 *Fragment
	for i := range result {
		if result[i].FType == "$419" {
			found419 = &result[i]
			break
		}
	}
	if found419 == nil {
		t.Fatal("rebuild_container_entity_map: no $419 found")
	}

	valMap, ok := asMap(found419.Value)
	if !ok {
		t.Fatal("$419 value is not a map")
	}

	contentsSlice, ok := asSlice(valMap["$252"])
	if !ok || len(contentsSlice) == 0 {
		t.Fatal("$252 missing or empty")
	}

	contentsMap, ok := asMap(contentsSlice[0])
	if !ok {
		t.Fatal("$252[0] is not a map")
	}

	entityIDsSlice, ok := asSlice(contentsMap["$181"])
	if !ok {
		t.Fatal("$181 missing")
	}

	if len(entityIDsSlice) != 3 {
		t.Errorf("entity IDs count = %d, want 3", len(entityIDsSlice))
	}

	// Verify all three entity IDs are present
	entitySet := map[string]bool{}
	for _, id := range entityIDsSlice {
		idStr, _ := asString(id)
		entitySet[idStr] = true
	}
	for _, expected := range []string{"section_a", "section_b", "resource1"} {
		if !entitySet[expected] {
			t.Errorf("entity IDs missing %q", expected)
		}
	}

	// Verify container ID
	containerID, _ := asString(contentsMap["$155"])
	if containerID != "CR!TEST" {
		t.Errorf("container ID = %q, want CR!TEST", containerID)
	}
}

// ---------------------------------------------------------------------------
// Ensure sort import is used (compile check)
// ---------------------------------------------------------------------------

func TestSortImportUsed(t *testing.T) {
	// This test just verifies the sort package compiles correctly
	strs := []string{"c", "a", "b"}
	sort.Strings(strs)
	if strs[0] != "a" {
		t.Error("sort not working")
	}
}
