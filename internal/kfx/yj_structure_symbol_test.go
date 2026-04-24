package kfx

import (
	"strings"
	"testing"
)

// =============================================================================
// VAL-A-011: classify_symbol — SHARED detection via resolver
// =============================================================================

func TestClassifySymbolSharedNumeric(t *testing.T) {
	r := &symbolResolver{localStart: 1000, locals: []string{"localSym"}}
	if g := classifySymbolWithResolver("margin_right", r); g != symShared {
		t.Fatalf("margin_right with localStart=1000 = %v want shared", g)
	}
	// $999 is beyond the catalog (842 real names, SID 10-851). It's a $N placeholder
	// in the extended table but not recognized by name lookup → unknown.
	if g := classifySymbolWithResolver("$999", r); g != symUnknown {
		t.Fatalf("$999 with localStart=1000 = %v want unknown (beyond catalog)", g)
	}
	if g := classifySymbolWithResolver("c73", r); g != symShort {
		t.Fatalf("c73 = %v want short", g)
	}
	// $0 is a special system symbol (not local, not shared in the symtab sense),
	// so it falls through to the non-resolver classifySymbol path → unknown.
	if g := classifySymbolWithResolver("$0", r); g != symUnknown {
		t.Fatalf("$0 with localStart=1000 = %v want unknown (special system symbol)", g)
	}
}

// =============================================================================
// VAL-A-012: classify_symbol — COMMON exact-name matches
// =============================================================================

func TestClassifySymbolCommonExactNames(t *testing.T) {
	commonNames := []string{
		"APPROXIMATE_PAGE_LIST",
		"crop_bleed",
		"dictionary_rules",
		"mkfx_id",
		"page_list_entry",
		"srl_created_by_stampler",
		"yj.dictionary.text",
		"note_template_collection",
	}
	for _, name := range commonNames {
		if g := classifySymbol(name); g != symCommon {
			t.Fatalf("classifySymbol(%q) = %v, want common", name, g)
		}
	}
}

// =============================================================================
// VAL-A-013: classify_symbol — COMMON prefix and regex matches
// =============================================================================

func TestClassifySymbolCommonPrefixAndRegex(t *testing.T) {
	tests := []struct {
		name string
		want symType
	}{
		// kfx_cover_image prefix
		{"kfx_cover_image_large", symCommon},
		{"kfx_cover_image", symCommon},
		// ^content_[0-9]+$
		{"content_42", symCommon},
		{"content_0", symCommon},
		{"content_", symUnknown},    // no digits
		{"content_abc", symUnknown}, // non-digits
		// ^eidbucket_[0-9]+$
		{"eidbucket_7", symCommon},
		{"eidbucket_", symUnknown},
		// ^PAGE_LIST_[0-9]{10,}$
		{"PAGE_LIST_1234567890", symCommon},
		{"PAGE_LIST_123", symUnknown}, // too short
		// UUID regex
		{"550e8400-e29b-41d4-a716-446655440000", symCommon},
		{"00000000-0000-0000-0000-000000000000", symCommon},
		{"not-a-uuid", symUnknown},
		// ^yj\.(authoring|conversion|print|semantics)\.
		{"yj.conversion.html_name", symCommon},
		{"yj.authoring.test", symCommon},
		{"yj.print.page", symCommon},
		{"yj.semantics.heading", symCommon},
		{"yj.other.thing", symUnknown},
		// ^nmdl\.
		{"nmdl.canvas_width", symCommon},
		{"nmdl.test", symCommon},
	}
	for _, tt := range tests {
		if g := classifySymbol(tt.name); g != tt.want {
			t.Fatalf("classifySymbol(%q) = %v, want %v", tt.name, g, tt.want)
		}
	}
}

// =============================================================================
// VAL-A-014: classify_symbol — DICTIONARY pattern group
// =============================================================================

func TestClassifySymbolDictionary(t *testing.T) {
	tests := []struct {
		name string
		want symType
	}{
		{"G42", symDictionary},
		{"G42-spm", symDictionary},
		{"G0", symDictionary},
		{"yj.dictionary.text", symCommon},    // COMMON exact match has priority
		{"yj.dictionary.other", symDictionary},
		{"G", symUnknown},        // no digits
		{"g42", symShort},        // lowercase 'g' matches SHORT pattern [a-z][A-Z0-9]{1,6}
		{"G42-extra", symUnknown}, // extra suffix
	}
	for _, tt := range tests {
		if g := classifySymbol(tt.name); g != tt.want {
			t.Fatalf("classifySymbol(%q) = %v, want %v", tt.name, g, tt.want)
		}
	}
}

// =============================================================================
// VAL-A-015: classify_symbol — ORIGINAL pattern group
// =============================================================================

func TestClassifySymbolOriginal(t *testing.T) {
	tests := []struct {
		name string
		want symType
	}{
		// V_ prefix
		{"V_0_0-PARA-0_0_123456789012_ab", symOriginal},
		{"V_0_0-CHAR-0_0_12345678901234_abc", symOriginal},
		// resource prefix
		{"resource_0_0_12345678901234_1_", symOriginal},
		{"res_0_0_12345678901234_1_", symOriginal},
		{"fonts/resource_0_0_12345678901234_1_", symOriginal},
		{"images/res_0_0_12345678901234_1_", symOriginal},
		// anchor/section/story/style/nav prefix
		{"anchor-0_0_12345678901234_1_", symOriginal},
		{"section-0_0_12345678901234_1_", symOriginal},
		{"story-0_0_12345678901234_1_", symOriginal},
		{"style-0_0_12345678901234_1_", symOriginal},
		{"navContainer0_0_12345678901234_1_", symOriginal},
		{"navUnit0_0_12345678901234_1_", symOriginal},
		// anchor- dash long
		{"anchor-my-label-12345678901234567-1", symOriginal},
		// anchor underscore hex
		{"anchor-my_label_0_12345678901234_ab", symOriginal},
		// LANDMARKS_/TOC_
		{"LANDMARKS_123456789012", symOriginal},
		{"TOC_123456789012", symOriginal},
		// LazyLoad/Target
		{"LazyLoadStoryLineForPage-42", symOriginal},
		{"TargetSectionForPage-42", symOriginal},
		{"TargetStoryLineForPage-42", symOriginal},
		// slice pdf
		{"slice_42.pdf", symOriginal},
		{"slice_42.txt", symUnknown}, // not pdf
		// Target_pg
		{"Target_pg_42_g_2", symOriginal},
		// KFXConditionalNavGroupUnit UUID
		{"KFXConditionalNavGroupUnit_550e8400-e29b-41d4-a716-446655440000", symOriginal},
		// Exact matches
		{"order-1", symOriginal},
		{"TargetReadingOrder", symOriginal},
		{"PageLabels", symOriginal},
		{"toc_entry", symOriginal},
		// Non-matching
		{"V_0", symUnknown}, // too short
	}
	for _, tt := range tests {
		if g := classifySymbol(tt.name); g != tt.want {
			t.Fatalf("classifySymbol(%q) = %v, want %v", tt.name, g, tt.want)
		}
	}
}

// =============================================================================
// VAL-A-016: classify_symbol — BASE64 pattern group
// =============================================================================

func TestClassifySymbolBase64(t *testing.T) {
	tests := []struct {
		name string
		want symType
	}{
		// Exactly 22 chars of [A-Za-z0-9_-] → BASE64 match
		{"ABCDEFGHIJKLMNOPQRSTUV", symBase64},
		// 22 + up to 6 [A-Z0-9] chars: "aBCDEFGHIJKLMNOPQRSTUVWX" = 24 chars
		{"aBCDEFGHIJKLMNOPQRSTUVWX", symBase64},
		// 26 chars: 22 base + 4 [A-Z0-9]
		{"abcDEFGHIJKLMNOPQRSTUVWXYZ", symBase64},
		// 28 chars: 22 base + 6 [A-Z0-9] (max)
		{"abcdefghijklmnopqrstuvWXYZ01", symBase64},
		// resource/ prefix (9 chars) + 22 base64 chars = 31 total
		{"resource/ABCDEFGHIJKLMNOPQRSTUVWX", symBase64},
		// [ctliz] prefix: "t" + 22 base chars
		{"tBCDEFGHIJKLMNOPQRSTUVWX", symBase64},
		// With suffix
		{"ABCDEFGHIJKLMNOPQRSTUV-hd", symBase64},
		{"ABCDEFGHIJKLMNOPQRSTUV.jpg", symBase64},
		{"ABCDEFGHIJKLMNOPQRSTUV.ttf", symBase64},
		{"ABCDEFGHIJKLMNOPQRSTUV.otf", symBase64},
		{"ABCDEFGHIJKLMNOPQRSTUV.woff", symBase64},
		{"ABCDEFGHIJKLMNOPQRSTUV-ad", symBase64},
		{"ABCDEFGHIJKLMNOPQRSTUV-spm", symBase64},
		{"ABCDEFGHIJKLMNOPQRSTUV_thumbnail", symBase64},
		// Too short (21 chars)
		{"ABCDEFGHIJKLMNOPQRSTU", symUnknown},
		// Regular short names
		{"short", symUnknown},
		{"cShortID", symUnknown},
	}
	for _, tt := range tests {
		if g := classifySymbol(tt.name); g != tt.want {
			t.Fatalf("classifySymbol(%q) (len=%d) = %v, want %v", tt.name, len(tt.name), g, tt.want)
		}
	}
}

// =============================================================================
// VAL-A-017: classify_symbol — SHORT pattern group
// =============================================================================

func TestClassifySymbolShort(t *testing.T) {
	tests := []struct {
		name string
		want symType
	}{
		{"c73", symShort},
		{"rsrcAABB", symShort},
		{"resource/rsrcAABB", symShort},
		{"resource/eAABB", symShort},
		{"eAB12", symShort},
		// With suffix
		{"c73-hd", symShort},
		{"c73.jpg", symShort},
		{"c73.ttf", symShort},
		{"c73-ad", symShort},
		{"c73-spm", symShort},
		{"c73_thumbnail", symShort},
		{"c73-resized-100-200", symShort},
		// Too few uppercase
		{"c", symUnknown},    // needs ≥1 uppercase/digit
		// Too many chars (7 chars exceeds limit)
		{"c1234567", symUnknown},
	}
	for _, tt := range tests {
		if g := classifySymbol(tt.name); g != tt.want {
			t.Fatalf("classifySymbol(%q) = %v, want %v", tt.name, g, tt.want)
		}
	}
}

// =============================================================================
// VAL-A-018: classify_symbol — UNKNOWN fallback
// =============================================================================

func TestClassifySymbolUnknown(t *testing.T) {
	tests := []struct {
		name string
		want symType
	}{
		{"totally-random-identifier", symUnknown},
		{"", symUnknown},
		{"x", symUnknown},
		{"not_a_known_pattern_at_all", symUnknown},
	}
	for _, tt := range tests {
		if g := classifySymbol(tt.name); g != tt.want {
			t.Fatalf("classifySymbol(%q) = %v, want %v", tt.name, g, tt.want)
		}
	}
}

// =============================================================================
// VAL-A-019: classify_symbol — priority order
// =============================================================================

func TestClassifySymbolPriorityOrder(t *testing.T) {
	// SHARED > COMMON > DICTIONARY > ORIGINAL > BASE64 > SHORT > UNKNOWN
	r := &symbolResolver{localStart: 1000, locals: []string{}}

	// SHARED takes priority over everything
	if g := classifySymbolWithResolver("margin_right", r); g != symShared {
		t.Fatalf("shared should win: got %v", g)
	}

	// COMMON takes priority over DICTIONARY
	// "yj.dictionary.text" is both COMMON (exact) and matches DICTIONARY regex
	if g := classifySymbol("yj.dictionary.text"); g != symCommon {
		t.Fatalf("COMMON should win over DICTIONARY for yj.dictionary.text: got %v", g)
	}

	// content_42 is COMMON, not SHORT or UNKNOWN
	if g := classifySymbol("content_42"); g != symCommon {
		t.Fatalf("COMMON should win for content_42: got %v", g)
	}

	// kfx_cover_image_large is COMMON
	if g := classifySymbol("kfx_cover_image_large"); g != symCommon {
		t.Fatalf("COMMON should win for kfx_cover_image_large: got %v", g)
	}
}

// =============================================================================
// VAL-A-020: allowed_symbol_prefix validation
// =============================================================================

func TestAllowedSymbolPrefix(t *testing.T) {
	validChars := "abcdefilnpstz"
	for _, ch := range validChars {
		if !allowedSymbolPrefix(string(ch)) {
			t.Fatalf("allowedSymbolPrefix(%q) = false, want true", string(ch))
		}
	}
	// Invalid single chars
	for _, ch := range "ghjkmqruvwxy" {
		if allowedSymbolPrefix(string(ch)) {
			t.Fatalf("allowedSymbolPrefix(%q) = true, want false", string(ch))
		}
	}
	// Multi-char valid
	if !allowedSymbolPrefix("ab") {
		t.Fatalf("allowedSymbolPrefix(\"ab\") = false, want true")
	}
	if !allowedSymbolPrefix("cdefilnpstz") {
		t.Fatalf("allowedSymbolPrefix(\"cdefilnpstz\") = false, want true")
	}
	// Multi-char invalid
	if allowedSymbolPrefix("ax") {
		t.Fatalf("allowedSymbolPrefix(\"ax\") = true, want false (x not in set)")
	}
	if allowedSymbolPrefix("xa") {
		t.Fatalf("allowedSymbolPrefix(\"xa\") = true, want false (x not in set)")
	}
	// Edge cases
	if !allowedSymbolPrefix("") {
		t.Fatalf("allowedSymbolPrefix(\"\") = false, want true (empty string is in any string)")
	}
}

// =============================================================================
// VAL-A-021: get_reading_orders — $538 then $258 fallback
// =============================================================================

func TestGetReadingOrdersFrom538(t *testing.T) {
	// Only $538 has $169
	frags := fragmentCatalog{
		DocumentData: map[string]interface{}{
			"reading_orders": []interface{}{
				map[string]interface{}{"reading_order_name": "main"},
			},
		},
		ReadingOrderMetadata: nil,
	}
	orders := getReadingOrders(frags)
	if len(orders) != 1 {
		t.Fatalf("expected 1 reading order, got %d", len(orders))
	}
	name, _ := asString(orders[0].(map[string]interface{})["reading_order_name"])
	if name != "main" {
		t.Fatalf("expected reading order name 'main', got %q", name)
	}
}

func TestGetReadingOrdersFallsBackTo258(t *testing.T) {
	// Only $258 has $169
	frags := fragmentCatalog{
		DocumentData: nil,
		ReadingOrderMetadata: map[string]interface{}{
			"reading_orders": []interface{}{
				map[string]interface{}{"reading_order_name": "fallback"},
			},
		},
	}
	orders := getReadingOrders(frags)
	if len(orders) != 1 {
		t.Fatalf("expected 1 reading order, got %d", len(orders))
	}
	name, _ := asString(orders[0].(map[string]interface{})["reading_order_name"])
	if name != "fallback" {
		t.Fatalf("expected reading order name 'fallback', got %q", name)
	}
}

func TestGetReadingOrdersEmpty(t *testing.T) {
	// Neither has $169
	frags := fragmentCatalog{
		DocumentData:         nil,
		ReadingOrderMetadata: nil,
	}
	orders := getReadingOrders(frags)
	if len(orders) != 0 {
		t.Fatalf("expected 0 reading orders, got %d", len(orders))
	}
}

func TestGetReadingOrdersPrefers538Over258(t *testing.T) {
	// Both have $169 — $538 should be preferred
	frags := fragmentCatalog{
		DocumentData: map[string]interface{}{
			"reading_orders": []interface{}{
				map[string]interface{}{"reading_order_name": "from_538"},
			},
		},
		ReadingOrderMetadata: map[string]interface{}{
			"reading_orders": []interface{}{
				map[string]interface{}{"reading_order_name": "from_258"},
			},
		},
	}
	orders := getReadingOrders(frags)
	if len(orders) != 1 {
		t.Fatalf("expected 1 reading order, got %d", len(orders))
	}
	name, _ := asString(orders[0].(map[string]interface{})["reading_order_name"])
	if name != "from_538" {
		t.Fatalf("expected 'from_538', got %q", name)
	}
}

// =============================================================================
// VAL-A-022: ordered_section_names — deduplication and order preservation
// =============================================================================

func TestOrderedSectionNamesDeduplication(t *testing.T) {
	frags := fragmentCatalog{
		DocumentData: map[string]interface{}{
			"reading_orders": []interface{}{
				map[string]interface{}{
					"sections": []interface{}{"A", "B"},
				},
				map[string]interface{}{
					"sections": []interface{}{"B", "C"},
				},
			},
		},
	}
	names := orderedSectionNames(frags)
	expected := []string{"A", "B", "C"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d: %v", len(expected), len(names), names)
	}
	for i, n := range names {
		if n != expected[i] {
			t.Fatalf("position %d: expected %q, got %q", i, expected[i], n)
		}
	}
}

func TestOrderedSectionNamesEmpty(t *testing.T) {
	frags := fragmentCatalog{}
	names := orderedSectionNames(frags)
	if len(names) != 0 {
		t.Fatalf("expected empty, got %v", names)
	}
}

func TestOrderedSectionNamesPreservesOrder(t *testing.T) {
	frags := fragmentCatalog{
		DocumentData: map[string]interface{}{
			"reading_orders": []interface{}{
				map[string]interface{}{
					"sections": []interface{}{"Z", "A", "M"},
				},
			},
		},
	}
	names := orderedSectionNames(frags)
	expected := []string{"Z", "A", "M"}
	for i, n := range names {
		if n != expected[i] {
			t.Fatalf("position %d: expected %q, got %q", i, expected[i], n)
		}
	}
}

// =============================================================================
// VAL-A-023: has_illustrated_layout_page_template_condition
// =============================================================================

func TestHasIllustratedLayoutPageTemplateConditionTrue(t *testing.T) {
	// Matching condition: $171 -> SExp[0]="==", [1]="position", [2]=SExp["anchor", ...]
	condition := []interface{}{"==", "position", []interface{}{"anchor", "anchor_name"}}
	section := map[string]interface{}{
		"page_templates": []interface{}{
			map[string]interface{}{
				"condition": condition,
			},
		},
	}
	frags := fragmentCatalog{
		SectionFragments: map[string]sectionFragment{
			"sec1": {ID: "sec1", PageTemplateValues: section},
		},
	}
	if !hasIllustratedLayoutPageTemplateCondition(frags) {
		t.Fatalf("expected true for matching condition")
	}
}

func TestHasIllustratedLayoutPageTemplateConditionWithWrongOperator(t *testing.T) {
	// Condition with fv[0] not in ["==", "<=", "<"]
	condition := []interface{}{"$999", "position", []interface{}{"anchor", "anchor_name"}}
	section := map[string]interface{}{
		"page_templates": []interface{}{
			map[string]interface{}{
				"condition": condition,
			},
		},
	}
	frags := fragmentCatalog{
		SectionFragments: map[string]sectionFragment{
			"sec1": {ID: "sec1", PageTemplateValues: section},
		},
	}
	if hasIllustratedLayoutPageTemplateCondition(frags) {
		t.Fatalf("expected false for wrong operator")
	}
}

func TestHasIllustratedLayoutPageTemplateConditionNoCondition(t *testing.T) {
	// Section without $171
	section := map[string]interface{}{
		"page_templates": []interface{}{
			map[string]interface{}{
				"style": "some_style",
			},
		},
	}
	frags := fragmentCatalog{
		SectionFragments: map[string]sectionFragment{
			"sec1": {ID: "sec1", PageTemplateValues: section},
		},
	}
	if hasIllustratedLayoutPageTemplateCondition(frags) {
		t.Fatalf("expected false when no $171")
	}
}

func TestHasIllustratedLayoutPageTemplateConditionEmpty(t *testing.T) {
	frags := fragmentCatalog{
		SectionFragments: map[string]sectionFragment{},
	}
	if hasIllustratedLayoutPageTemplateCondition(frags) {
		t.Fatalf("expected false for empty sections")
	}
}

// Test all three valid operators
func TestHasIllustratedLayoutPageTemplateConditionAllOperators(t *testing.T) {
	for _, op := range []string{"==", "<=", "<"} {
		condition := []interface{}{op, "position", []interface{}{"anchor", "anchor_name"}}
		section := map[string]interface{}{
			"page_templates": []interface{}{
				map[string]interface{}{
					"condition": condition,
				},
			},
		}
		frags := fragmentCatalog{
			SectionFragments: map[string]sectionFragment{
				"sec1": {ID: "sec1", PageTemplateValues: section},
			},
		}
		if !hasIllustratedLayoutPageTemplateCondition(frags) {
			t.Fatalf("expected true for operator %q", op)
		}
	}
}

// Test with wrong SExp structure
func TestHasIllustratedLayoutPageTemplateConditionWrongStructure(t *testing.T) {
	tests := []struct {
		name      string
		condition interface{}
	}{
		{"wrong length 2", []interface{}{"==", "position"}},
		{"wrong length 4", []interface{}{"==", "position", []interface{}{"anchor", "x"}, "extra"}},
		{"fv[2] wrong type", []interface{}{"==", "position", "not_a_slice"}},
		{"fv[2] wrong length", []interface{}{"==", "position", []interface{}{"anchor"}}},
		{"fv[2][0] wrong", []interface{}{"==", "position", []interface{}{"$999", "x"}}},
		{"fv[1] wrong", []interface{}{"==", "$999", []interface{}{"anchor", "x"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			section := map[string]interface{}{
				"page_templates": []interface{}{
					map[string]interface{}{
						"condition": tt.condition,
					},
				},
			}
			frags := fragmentCatalog{
				SectionFragments: map[string]sectionFragment{
					"sec1": {ID: "sec1", PageTemplateValues: section},
				},
			}
			if hasIllustratedLayoutPageTemplateCondition(frags) {
				t.Fatalf("expected false for %s", tt.name)
			}
		})
	}
}

// =============================================================================
// VAL-A-024: determine_book_symbol_format — quorum and priority
// =============================================================================

func TestDetermineBookSymbolFormatQuorumAndPriority(t *testing.T) {
	tests := []struct {
		name       string
		symbols    map[string]struct{}
		docData    map[string]interface{}
		wantFormat symType
	}{
		{
			"empty favors SHORT",
			map[string]struct{}{},
			nil,
			symShort,
		},
		{
			"all SHORT",
			makeSymbolSet([]string{"c73", "c74", "c75", "eAABB"}),
			nil,
			symShort,
		},
		{
			"all DICTIONARY",
			makeSymbolSet([]string{"G1", "G2", "G3", "G4"}),
			nil,
			symDictionary,
		},
		{
			"mixed SHORT reaches quorum",
			makeSymbolSet([]string{
				"c01", "c02", "c03", "c04", "c05", "c06",
				"G1", "G2", "G3", "G4",
			}),
			nil,
			symShort,
		},
		{
			"documentData max_id forces SHORT",
			makeSymbolSet([]string{"G1", "G2", "G3", "G4", "G5", "G6"}),
			map[string]interface{}{"max_id": float64(100)},
			symShort,
		},
		{
			"BASE64 reaches quorum",
			makeSymbolSet([]string{
				"ABCDEFGHIJKLMNOPQRSTUV",
				"abcdefghijklmnopqrstuvWXYZ01",
				"tBCDEFGHIJKLMNOPQRSTUVWX",
				"abcDEFGHIJKLMNOPQRSTUVWXYZ",
			}),
			nil,
			symBase64,
		},
		{
			"ORIGINAL fallback",
			makeSymbolSet([]string{
				"order-1",
				"TargetReadingOrder",
				"PageLabels",
			}),
			nil,
			symOriginal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineBookSymbolFormat(tt.symbols, tt.docData, nil)
			if got != tt.wantFormat {
				t.Fatalf("determineBookSymbolFormat = %v, want %v", got, tt.wantFormat)
			}
		})
	}
}

// =============================================================================
// check_symbol_table tests (VAL-A-related: parity verification)
// =============================================================================

func TestCheckSymbolTableLogsUnknown(t *testing.T) {
	// checkSymbolTable should not panic with empty data
	frags := fragmentCatalog{
		StyleFragments:    map[string]map[string]interface{}{},
		Storylines:        map[string]map[string]interface{}{},
		NavContainers:     map[string]map[string]interface{}{},
		NavRoots:          nil,
		TitleMetadata:     nil,
		DocumentData:      nil,
		ContentFeatures:   nil,
		SectionFragments:  map[string]sectionFragment{},
		AnchorFragments:   map[string]anchorFragment{},
		ResourceFragments: map[string]resourceFragment{},
		ContentFragments:  map[string][]string{},
		RubyGroups:        map[string]map[string]interface{}{},
		RubyContents:      map[string]map[string]interface{}{},
	}
	// Should not panic
	r := &symbolResolver{localStart: 100, locals: []string{}}
	checkSymbolTable(frags, r, false, false)
}

func TestCheckSymbolTableWithSymbols(t *testing.T) {
	// Test with some known symbol data
	frags := fragmentCatalog{
		StyleFragments: map[string]map[string]interface{}{
			"style1": {"font_family": "serif", "font_style": "bold"},
		},
		Storylines:        map[string]map[string]interface{}{},
		NavContainers:     map[string]map[string]interface{}{},
		NavRoots:          nil,
		TitleMetadata:     nil,
		DocumentData:      nil,
		ContentFeatures:   nil,
		SectionFragments:  map[string]sectionFragment{},
		AnchorFragments:   map[string]anchorFragment{},
		ResourceFragments: map[string]resourceFragment{},
		ContentFragments:  map[string][]string{},
		RubyGroups:        map[string]map[string]interface{}{},
		RubyContents:      map[string]map[string]interface{}{},
	}
	r := &symbolResolver{localStart: 100, locals: []string{"style1"}}
	// Should not panic even with style data
	checkSymbolTable(frags, r, false, false)
}

// =============================================================================
// get_ordered_image_resources tests
// =============================================================================

func TestGetOrderedImageResourcesNotFixedLayout(t *testing.T) {
	frags := fragmentCatalog{
		SectionFragments: map[string]sectionFragment{},
	}
	_, _, _, err := getOrderedImageResources(&frags, false)
	if err == nil {
		t.Fatalf("expected error for non-fixed-layout book")
	}
	if !strings.Contains(err.Error(), "not fixed-layout") {
		t.Fatalf("expected 'not fixed-layout' error, got: %v", err)
	}
}

func TestGetOrderedImageResourcesNoImages(t *testing.T) {
	frags := fragmentCatalog{
		SectionFragments: map[string]sectionFragment{},
	}
	_, _, _, err := getOrderedImageResources(&frags, true)
	if err == nil {
		t.Fatalf("expected error for book with no image resources")
	}
	if !strings.Contains(err.Error(), "does not contain image resources") {
		t.Fatalf("expected 'does not contain image resources' error, got: %v", err)
	}
}

// =============================================================================
// Helper
// =============================================================================

func makeSymbolSet(names []string) map[string]struct{} {
	result := make(map[string]struct{}, len(names))
	for _, n := range names {
		result[n] = struct{}{}
	}
	return result
}

// =============================================================================
// VAL-FIX-002: checkSymbolTable rebuild — replace_local_symbols + replace_symbol_table_import
// Python: yj_structure.py L1143-1164 (check_symbol_table rebuild block + replace_symbol_table_import)
// =============================================================================

func TestCheckSymbolTableRebuildReplaceLocalSymbols(t *testing.T) {
	// Python L1143-1148: when rebuild=true, the function:
	// 1. Collects book_symbols (used local symbols where get_id >= local_min_id)
	// 2. Sorts them with natural_sort_key
	// 3. Calls replace_local_symbols(sorted_book_symbols)
	// 4. Calls replace_symbol_table_import()

	// Setup: fragmentCatalog with some local symbols used as struct keys in content
	// (Python's find_symbol_references adds IonSymbol struct keys, not string values)
	frags := fragmentCatalog{
		StyleFragments: map[string]map[string]interface{}{
			"zebra_style": {"local_sym_z": map[string]interface{}{}},
			"alpha_style": {"local_sym_a": map[string]interface{}{}},
		},
		Storylines:        map[string]map[string]interface{}{},
		NavContainers:     map[string]map[string]interface{}{},
		NavRoots:          nil,
		TitleMetadata:     nil,
		DocumentData:      nil,
		ContentFeatures:   nil,
		SectionFragments:  map[string]sectionFragment{},
		AnchorFragments:   map[string]anchorFragment{},
		ResourceFragments: map[string]resourceFragment{},
		ContentFragments:  map[string][]string{},
		RubyGroups:        map[string]map[string]interface{}{},
		RubyContents:      map[string]map[string]interface{}{},
		RawFragments:      map[string][]byte{},
	}

	// symbolResolver with localStart=100, some local symbols already present
	r := &symbolResolver{
		localStart: 100,
		locals:     []string{"old_local_1", "old_local_2"},
	}

	cfg := checkSymbolTableConfig{
		OriginalSyms: []string{"local_sym_z", "local_sym_a", "old_local_1", "old_local_2"},
	}

	// Before rebuild, the resolver should have old locals
	if len(r.locals) != 2 {
		t.Fatalf("expected 2 locals before rebuild, got %d", len(r.locals))
	}

	checkSymbolTableWithConfig(frags, r, true, false, cfg)

	// After rebuild:
	// 1. replace_local_symbols should replace old locals with book_symbols
	//    (the used local symbols, sorted by natural_sort_key)
	// 2. "local_sym_z" and "local_sym_a" are used (in StyleFragments),
	//    but "old_local_1" and "old_local_2" are NOT used.
	//    So book_symbols = ["local_sym_a", "local_sym_z"] (natural-sorted)
	// 3. resolver.locals should now be ["local_sym_a", "local_sym_z"]
	if len(r.locals) != 2 {
		t.Fatalf("expected 2 locals after rebuild, got %d: %v", len(r.locals), r.locals)
	}
	if r.locals[0] != "local_sym_a" {
		t.Errorf("expected locals[0] = 'local_sym_a', got %q", r.locals[0])
	}
	if r.locals[1] != "local_sym_z" {
		t.Errorf("expected locals[1] = 'local_sym_z', got %q", r.locals[1])
	}
}

func TestCheckSymbolTableRebuildSymbolTableImport(t *testing.T) {
	// Python L1151-1160: replace_symbol_table_import creates a new $ion_symbol_table
	// fragment from the updated symbol table, removes the old one, inserts the new one.

	// Setup: include a $ion_symbol_table raw fragment
	frags := fragmentCatalog{
		StyleFragments: map[string]map[string]interface{}{
			"style1": {"my_local": map[string]interface{}{}},
		},
		Storylines:        map[string]map[string]interface{}{},
		NavContainers:     map[string]map[string]interface{}{},
		NavRoots:          nil,
		TitleMetadata:     nil,
		DocumentData:      nil,
		ContentFeatures:   nil,
		SectionFragments:  map[string]sectionFragment{},
		AnchorFragments:   map[string]anchorFragment{},
		ResourceFragments: map[string]resourceFragment{},
		ContentFragments:  map[string][]string{},
		RubyGroups:        map[string]map[string]interface{}{},
		RubyContents:      map[string]map[string]interface{}{},
		RawFragments: map[string][]byte{
			"$ion_symbol_table": []byte("old_symtab_data"),
		},
		RawBlobOrder: []rawBlob{
			{ID: "$ion_symbol_table", Data: []byte("old_symtab_data")},
		},
	}

	r := &symbolResolver{
		localStart: 100,
		locals:     []string{"my_local"},
	}

	cfg := checkSymbolTableConfig{
		OriginalSyms: []string{"my_local"},
	}

	checkSymbolTableWithConfig(frags, r, true, false, cfg)

	// After rebuild, the RawFragments should have a new $ion_symbol_table
	// that reflects the updated local symbols.
	newSymtab, exists := frags.RawFragments["$ion_symbol_table"]
	if !exists {
		t.Fatal("expected $ion_symbol_table in RawFragments after rebuild")
	}

	// The new symbol table should contain the sorted local symbols
	newSymtabStr := string(newSymtab)
	if newSymtabStr == "" {
		t.Fatal("expected non-empty $ion_symbol_table after rebuild")
	}

	// Verify the rebuild replaced the old data
	if newSymtabStr == "old_symtab_data" {
		t.Error("expected $ion_symbol_table to be rebuilt with new data, but got old data")
	}
}

func TestCheckSymbolTableRebuildSortsNatually(t *testing.T) {
	// Python L1148: sorted(book_symbols, key=natural_sort_key)
	// Verify that symbols are sorted naturally (e.g., "sym2" before "sym10").

	frags := fragmentCatalog{
		StyleFragments: map[string]map[string]interface{}{
			"s1": {"sym10": map[string]interface{}{}},
			"s2": {"sym2": map[string]interface{}{}},
			"s3": {"sym1": map[string]interface{}{}},
		},
		Storylines:        map[string]map[string]interface{}{},
		NavContainers:     map[string]map[string]interface{}{},
		NavRoots:          nil,
		TitleMetadata:     nil,
		DocumentData:      nil,
		ContentFeatures:   nil,
		SectionFragments:  map[string]sectionFragment{},
		AnchorFragments:   map[string]anchorFragment{},
		ResourceFragments: map[string]resourceFragment{},
		ContentFragments:  map[string][]string{},
		RubyGroups:        map[string]map[string]interface{}{},
		RubyContents:      map[string]map[string]interface{}{},
		RawFragments:      map[string][]byte{},
	}

	r := &symbolResolver{
		localStart: 100,
		locals:     []string{"old"},
	}

	cfg := checkSymbolTableConfig{
		OriginalSyms: []string{"sym10", "sym2", "sym1", "old"},
	}

	checkSymbolTableWithConfig(frags, r, true, false, cfg)

	// After rebuild, locals should be natural-sorted: sym1, sym2, sym10
	if len(r.locals) != 3 {
		t.Fatalf("expected 3 locals after rebuild, got %d: %v", len(r.locals), r.locals)
	}
	if r.locals[0] != "sym1" {
		t.Errorf("expected locals[0] = 'sym1', got %q", r.locals[0])
	}
	if r.locals[1] != "sym2" {
		t.Errorf("expected locals[1] = 'sym2', got %q", r.locals[1])
	}
	if r.locals[2] != "sym10" {
		t.Errorf("expected locals[2] = 'sym10', got %q", r.locals[2])
	}
}
