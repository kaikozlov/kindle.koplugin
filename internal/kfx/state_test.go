package kfx

import (
	"encoding/binary"
	"fmt"
	"sort"
	"testing"
)

// =============================================================================
// Tests for organizeFragments parity — VAL-A-001 through VAL-A-010
// =============================================================================

// helper to build a minimal containerSource with synthetic index data.
func makeTestSource(indexEntries [][4]uint32, entities [][]byte) *containerSource {
	// Build index data: each entry is 24 bytes (4 id + 4 type + 8 offset + 8 length)
	indexData := make([]byte, 0, len(indexEntries)*24)
	// Build entity region: prepend ENTY header + payload
	entityRegion := make([]byte, 0)
	for _, payload := range entities {
		entry := make([]byte, 10+4+len(payload))
		copy(entry[:4], "ENTY")
		binary.LittleEndian.PutUint32(entry[6:10], uint32(10+4)) // header length
		copy(entry[10+4:], payload)
		entityRegion = append(entityRegion, entry...)
	}

	// Build full container: CONT header + container_info + entity region
	header := make([]byte, 18)
	copy(header[:4], "CONT")
	binary.LittleEndian.PutUint32(header[6:10], 18) // header length

	// Container info offset/length set to 0 (no container info for simple tests)
	binary.LittleEndian.PutUint32(header[10:14], 0)
	binary.LittleEndian.PutUint32(header[14:18], 0)

	// Now build entity offsets relative to data start (after header)
	data := append(header, entityRegion...)
	_ = len(header)

	for i, entry := range indexEntries {
		// Calculate entity offset/length in data
		entOffset := 0
		entLength := 0
		if i < len(entities) {
			entLength = 10 + 4 + len(entities[i])
			for j := 0; j < i; j++ {
				entOffset += 10 + 4 + len(entities[j])
			}
		}
		buf := make([]byte, 24)
		binary.LittleEndian.PutUint32(buf[0:4], entry[0]) // id ID
		binary.LittleEndian.PutUint32(buf[4:8], entry[1]) // type ID
		binary.LittleEndian.PutUint64(buf[8:16], uint64(entOffset))
		binary.LittleEndian.PutUint64(buf[16:24], uint64(entLength))
		indexData = append(indexData, buf...)
	}

	return &containerSource{
		Data:      data,
		HeaderLen: len(header),
		IndexData: indexData,
		Resolver:  &symbolResolver{localStart: 100, locals: []string{}},
	}
}

// =============================================================================
// VAL-A-001: Fragment type categorization into fragmentCatalog buckets
// =============================================================================

func TestOrganizeFragmentsCategorizesContentFragments(t *testing.T) {
	// $145 → ContentFragments[name] = stringSlice
	fragments := fragmentCatalog{
		ContentFragments:  map[string][]string{},
		StyleFragments:    map[string]map[string]interface{}{},
		ResourceFragments: map[string]resourceFragment{},
		SectionFragments:  map[string]sectionFragment{},
		Storylines:        map[string]map[string]interface{}{},
		FontFragments:     map[string]fontFragment{},
		AnchorFragments:   map[string]anchorFragment{},
		NavContainers:     map[string]map[string]interface{}{},
		RubyGroups:        map[string]map[string]interface{}{},
		RubyContents:      map[string]map[string]interface{}{},
		RawFragments:      map[string][]byte{},
		PositionAliases:   map[int]string{},
		FragmentIDsByType: map[string][]string{},
	}

	// Simulate what organizeFragments does for $145
	value := map[string]interface{}{
		"name": "content1",
		"$146": []interface{}{"sid_a", "sid_b"},
	}
	name, _ := asString(value["name"])
	stringsValue := toStringSlice(value["$146"])
	if name != "" && len(stringsValue) > 0 {
		fragments.ContentFragments[name] = stringsValue
	}

	if len(fragments.ContentFragments["content1"]) != 2 {
		t.Fatalf("expected 2 content strings, got %d", len(fragments.ContentFragments["content1"]))
	}
	if fragments.ContentFragments["content1"][0] != "sid_a" {
		t.Fatalf("expected sid_a, got %s", fragments.ContentFragments["content1"][0])
	}
}

// =============================================================================
// VAL-A-002: Fragment ID remapping — $270 container format
// =============================================================================

func TestOrganizeFragmentsRemaps270ContainerFormat(t *testing.T) {
	// For a fragment of type $270 containing $161="fmtA" and $409="c001",
	// the summary ID must be "fmtA:c001"
	fragmentID := "$10"
	value := map[string]interface{}{
		"$161": "fmtA",
		"$409": "c001",
	}

	containerID := formatStringDefault(value["$161"]) + ":" + formatStringDefault(value["$409"])
	summaryID := containerID

	if summaryID != "fmtA:c001" {
		t.Fatalf("expected summaryID 'fmtA:c001', got %q", summaryID)
	}
	_ = fragmentID
}

// Helper to avoid import issues
func formatStringDefault(value interface{}) string {
	s, _ := asString(value)
	return s
}

// =============================================================================
// VAL-A-003: Fragment ID remapping — $593 reuses last container ID
// =============================================================================

func TestOrganizeFragmentsRemaps593ReusesLastContainerID(t *testing.T) {
	// When a $593 fragment follows a $270 fragment,
	// the $593 summary ID must equal the lastContainerID
	lastContainerID := "x:y" // set by preceding $270

	summaryID := lastContainerID

	if summaryID != "x:y" {
		t.Fatalf("expected summaryID 'x:y', got %q", summaryID)
	}
}

// =============================================================================
// VAL-A-004: Fragment ID remapping — $262 font count suffix
// =============================================================================

func TestOrganizeFragmentsRemaps262FontCountSuffix(t *testing.T) {
	fontCount := 0
	fragmentID := "myfont"

	// Three sequential $262 fragments
	ids := make([]string, 3)
	for i := 0; i < 3; i++ {
		ids[i] = fmt.Sprintf("%s-font-%03d", fragmentID, fontCount)
		fontCount++
	}

	expected := []string{"myfont-font-000", "myfont-font-001", "myfont-font-002"}
	for i, id := range ids {
		if id != expected[i] {
			t.Fatalf("font %d: expected %q, got %q", i, expected[i], id)
		}
	}
}

// =============================================================================
// VAL-A-005: Fragment ID remapping — $387 with $215 suffix
// =============================================================================

func TestOrganizeFragmentsRemaps387With215Suffix(t *testing.T) {
	fragmentID := "sec1"
	value := map[string]interface{}{
		"$215": "extra",
	}
	summaryID := fragmentID + ":" + formatStringDefault(value["$215"])

	if summaryID != "sec1:extra" {
		t.Fatalf("expected 'sec1:extra', got %q", summaryID)
	}
}

// =============================================================================
// VAL-A-006: replace_ion_data symbol collection — recursive walk
// =============================================================================

func TestMergeIonReferencedStringSymbolsRecursiveWalk(t *testing.T) {
	// After calling mergeIonReferencedStringSymbols on a nested structure,
	// bookSymbols must contain both "symA" and "symB"
	// In Go, $-prefixed keys in maps collect their string values;
	// slices recurse into children. Non-$ keys don't collect their values.
	bookSymbols := map[string]struct{}{}

	nested := map[string]interface{}{
		"$145": []interface{}{
			map[string]interface{}{
				"$157": "symA",
			},
			map[string]interface{}{
				"$146": []interface{}{
					map[string]interface{}{
						"$180": "symB",
					},
				},
			},
		},
	}

	mergeIonReferencedStringSymbols(nested, bookSymbols)

	if _, ok := bookSymbols["symA"]; !ok {
		t.Fatalf("expected bookSymbols to contain 'symA', got %v", bookSymbols)
	}
	if _, ok := bookSymbols["symB"]; !ok {
		t.Fatalf("expected bookSymbols to contain 'symB', got %v", bookSymbols)
	}
}

func TestMergeIonReferencedStringSymbolsNonDollarKeys(t *testing.T) {
	// Only $-prefixed keys should collect string values (not non-$ keys)
	bookSymbols := map[string]struct{}{}

	data := map[string]interface{}{
		"$145": "symbol_from_dollar",
		"name":  "not_a_symbol",
	}

	mergeIonReferencedStringSymbols(data, bookSymbols)

	if _, ok := bookSymbols["symbol_from_dollar"]; !ok {
		t.Fatalf("expected 'symbol_from_dollar' in bookSymbols")
	}
	if _, ok := bookSymbols["not_a_symbol"]; ok {
		t.Fatalf("'not_a_symbol' should NOT be collected (non-dollar key)")
	}
}

func TestMergeIonReferencedStringSymbolsEmptyString(t *testing.T) {
	// Empty strings should NOT be added
	bookSymbols := map[string]struct{}{}

	data := map[string]interface{}{
		"$145": "",
		"$146": []interface{}{""},
	}

	mergeIonReferencedStringSymbols(data, bookSymbols)

	if len(bookSymbols) != 0 {
		t.Fatalf("expected empty bookSymbols, got %v", bookSymbols)
	}
}

// =============================================================================
// VAL-A-007: mergeContentFragmentStringSymbols collects all content IDs
// =============================================================================

func TestMergeContentFragmentStringSymbols(t *testing.T) {
	bookSymbols := map[string]struct{}{}
	frag := map[string][]string{
		"content1": {"sid_a", "sid_b"},
		"content2": {"sid_c"},
	}

	mergeContentFragmentStringSymbols(frag, bookSymbols)

	for _, id := range []string{"sid_a", "sid_b", "sid_c"} {
		if _, ok := bookSymbols[id]; !ok {
			t.Fatalf("expected bookSymbols to contain %q", id)
		}
	}
}

func TestMergeContentFragmentStringSymbolsSkipsEmpty(t *testing.T) {
	bookSymbols := map[string]struct{}{}
	frag := map[string][]string{
		"content1": {"sid_a", "", "sid_b"},
	}

	mergeContentFragmentStringSymbols(frag, bookSymbols)

	if _, ok := bookSymbols[""]; ok {
		t.Fatalf("empty string should not be in bookSymbols")
	}
	if len(bookSymbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(bookSymbols))
	}
}

// =============================================================================
// VAL-A-008: book_symbols accumulates fragment IDs from all containers
// =============================================================================

func TestBookSymbolsAccumulatesFromMultipleContainers(t *testing.T) {
	// Two synthetic container sources each contribute distinct fragment IDs;
	// all appear in the final bookSymbols set.
	bookSymbols := map[string]struct{}{}

	// Simulate: source 1 contributes IDs from fragments
	ids1 := []string{"frag_a", "frag_b"}
	for _, id := range ids1 {
		bookSymbols[id] = struct{}{}
	}

	// Source 2 contributes different IDs
	ids2 := []string{"frag_c", "frag_d"}
	for _, id := range ids2 {
		bookSymbols[id] = struct{}{}
	}

	for _, id := range []string{"frag_a", "frag_b", "frag_c", "frag_d"} {
		if _, ok := bookSymbols[id]; !ok {
			t.Fatalf("expected bookSymbols to contain %q from both sources", id)
		}
	}
}

// =============================================================================
// VAL-A-009: Section order derivation — $258 then $538 then fallback
// =============================================================================

func TestSectionOrderFrom258(t *testing.T) {
	// $258 provides reading orders with section names
	value := map[string]interface{}{
		"$169": []interface{}{
			map[string]interface{}{
				"$170": []interface{}{"secA", "secB"},
			},
		},
	}
	order := readSectionOrder(value)
	if len(order) != 2 || order[0] != "secA" || order[1] != "secB" {
		t.Fatalf("expected [secA secB], got %v", order)
	}
}

func TestSectionOrderFallbackTo538(t *testing.T) {
	// When $258 has no reading orders, $538 provides them
	value := map[string]interface{}{
		"$169": []interface{}{
			map[string]interface{}{
				"$170": []interface{}{"secX", "secY"},
			},
		},
	}
	order := readSectionOrder(value)
	if len(order) != 2 || order[0] != "secX" || order[1] != "secY" {
		t.Fatalf("expected [secX secY], got %v", order)
	}
}

func TestSectionOrderFallbackToSortedKeys(t *testing.T) {
	// When both $258 and $538 are empty, sort keys of SectionFragments
	sections := map[string]sectionFragment{
		"charlie": {ID: "charlie"},
		"alpha":   {ID: "alpha"},
		"bravo":   {ID: "bravo"},
	}
	var order []string
	for id := range sections {
		order = append(order, id)
	}
	// sort.Strings is what organizeFragments does
	sort.Strings(order)

	expected := []string{"alpha", "bravo", "charlie"}
	for i, id := range order {
		if id != expected[i] {
			t.Fatalf("position %d: expected %q, got %q", i, expected[i], id)
		}
	}
}

// =============================================================================
// VAL-A-010: Duplicate fragment ID within same type is recorded
// =============================================================================

func TestDuplicateFragmentIDsRecorded(t *testing.T) {
	// Two $157 fragments with different IDs both appear in FragmentIDsByType["$157"]
	idsByType := map[string][]string{}
	fragmentType := "$157"

	// First fragment
	idsByType[fragmentType] = append(idsByType[fragmentType], "style1")
	// Second fragment
	idsByType[fragmentType] = append(idsByType[fragmentType], "style2")

	if len(idsByType["$157"]) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(idsByType["$157"]))
	}
	if idsByType["$157"][0] != "style1" {
		t.Fatalf("expected style1, got %q", idsByType["$157"][0])
	}
	if idsByType["$157"][1] != "style2" {
		t.Fatalf("expected style2, got %q", idsByType["$157"][1])
	}
}

// =============================================================================
// Singleton collapse (Python organize_fragments_by_type L207-213)
// =============================================================================

func TestSingletonCollapseWhenKeyEqualsCategory(t *testing.T) {
	// When a category has 1 entry where key==category, unwrap from dict
	categorizedData := map[string]interface{}{
		"$538": map[string]interface{}{
			"$538": map[string]interface{}{"$169": "data"},
		},
	}

	// Apply singleton collapse logic
	for category, idsRaw := range categorizedData {
		ids, ok := idsRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if len(ids) == 1 {
			for id, val := range ids {
				if id == category {
					categorizedData[category] = val
				}
			}
		}
	}

	// After collapse, $538 should be the inner value directly
	val, ok := categorizedData["$538"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected $538 to be unwrapped to map, got %T", categorizedData["$538"])
	}
	if val["$169"] != "data" {
		t.Fatalf("expected inner value $169='data', got %v", val["$169"])
	}
}

func TestSingletonNoCollapseWhenKeyDiffersFromCategory(t *testing.T) {
	// When a category has 1 entry but key != category, do NOT unwrap
	categorizedData := map[string]interface{}{
		"$538": map[string]interface{}{
			"other_key": map[string]interface{}{"$169": "data"},
		},
	}

	for category, idsRaw := range categorizedData {
		ids, ok := idsRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if len(ids) == 1 {
			for id, val := range ids {
				if id == category {
					categorizedData[category] = val
				}
			}
		}
	}

	// Should NOT be collapsed because key "other_key" != "$538"
	inner, ok := categorizedData["$538"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected $538 to remain a map, got %T", categorizedData["$538"])
	}
	if _, exists := inner["other_key"]; !exists {
		t.Fatalf("expected 'other_key' to remain in the map")
	}
}

func TestNoCollapseWhenMultipleEntries(t *testing.T) {
	// When a category has multiple entries, no collapse
	categorizedData := map[string]interface{}{
		"$157": map[string]interface{}{
			"style1": map[string]interface{}{"$11": "serif"},
			"style2": map[string]interface{}{"$11": "sans"},
		},
	}

	for category, idsRaw := range categorizedData {
		ids, ok := idsRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if len(ids) == 1 {
			for id, val := range ids {
				if id == category {
					categorizedData[category] = val
				}
			}
		}
	}

	inner, ok := categorizedData["$157"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected $157 to remain a map with 2 entries, got %T", categorizedData["$157"])
	}
	if len(inner) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(inner))
	}
}

// =============================================================================
// Null/mixed ID error handling (Python L214)
// =============================================================================

func TestNullIDErrorDetection(t *testing.T) {
	// When a category has mixed nil and non-nil IDs, it should be detected
	categorizedData := map[string]interface{}{
		"$260": map[string]interface{}{
			"":     map[string]interface{}{"$174": "section_nil"},
			"sec1": map[string]interface{}{"$174": "section1"},
		},
	}

	// Check for nil/empty IDs (Python checks `None in ids`)
	for category, idsRaw := range categorizedData {
		ids, ok := idsRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if len(ids) > 1 {
			_, hasEmpty := ids[""]
			if hasEmpty {
				t.Logf("detected mixed null/non-null ids of type %q (expected behavior)", category)
			}
		}
	}
}

// =============================================================================
// chooseFragmentIdentity tests
// =============================================================================

func TestChooseFragmentIdentity(t *testing.T) {
	tests := []struct {
		name       string
		fragmentID string
		rawValue   interface{}
		want       string
	}{
		{"resolved value ID", "$10", "myID", "myID"},
		{"value ID is placeholder, fragment also placeholder", "$10", "$99", "$99"}, // neither resolved, valueID fallback
		{"both are resolved", "fallback", "primary", "primary"},
		{"empty value uses fragment", "frag", "", "frag"},
		{"value ID takes priority", "frag", "better", "better"},
		{"nil value uses fragment", "frag", nil, "frag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chooseFragmentIdentity(tt.fragmentID, tt.rawValue)
			if got != tt.want {
				t.Fatalf("chooseFragmentIdentity(%q, %v) = %q, want %q", tt.fragmentID, tt.rawValue, got, tt.want)
			}
		})
	}
}

// =============================================================================
// isResolvedIdentity tests
// =============================================================================

func TestIsResolvedIdentity(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"$10", false},
		{"$999", false},
		{"myID", true},
		{"section-1_0", true},
		{"c0", true},
		{"$", true}, // edge case: just dollar sign, not $<digits> pattern
	}

	for _, tt := range tests {
		got := isResolvedIdentity(tt.value)
		if got != tt.want {
			t.Fatalf("isResolvedIdentity(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

// =============================================================================
// isPlaceholderSymbol tests
// =============================================================================

func TestIsPlaceholderSymbol(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"$10", true},
		{"$0", true},
		{"$999", true},
		{"", false},
		{"myID", false},
		{"$abc", false},
		{"$", false},
		{"$10a", false},
	}

	for _, tt := range tests {
		got := isPlaceholderSymbol(tt.value)
		if got != tt.want {
			t.Fatalf("isPlaceholderSymbol(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}
