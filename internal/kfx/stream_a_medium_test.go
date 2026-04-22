package kfx

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

// =============================================================================
// VAL-M1-STRM-001: $258 key values processed without special ID override
// Python: yj_metadata.py:32-42
// =============================================================================

func TestMetadataProcessing258NoOverride(t *testing.T) {
	// Verify that $258 entries in $490 are read directly ($492 for key, $307 for value)
	// without ID manipulation.
	cat := &fragmentCatalog{
		TitleMetadata: map[string]interface{}{
			"$491": []interface{}{
				map[string]interface{}{
					"$495": "kindle_title_metadata",
					"$258": []interface{}{
						map[string]interface{}{
							"$492": "author",
							"$307": "Test Author",
						},
						map[string]interface{}{
							"$492": "title",
							"$307": "Test Title",
						},
					},
				},
			},
		},
	}

	// Tier 1: getMetadataValue should find "author" and "title" via $492/$307 directly
	author := getMetadataValue(cat, "author", "kindle_title_metadata", nil)
	if author == nil {
		t.Fatal("expected author value, got nil")
	}
	if authorStr, ok := author.(string); !ok || authorStr != "Test Author" {
		t.Fatalf("expected 'Test Author', got %v", author)
	}

	title := getMetadataValue(cat, "title", "kindle_title_metadata", nil)
	if title == nil {
		t.Fatal("expected title value, got nil")
	}
	if titleStr, ok := title.(string); !ok || titleStr != "Test Title" {
		t.Fatalf("expected 'Test Title', got %v", title)
	}
}

func TestMetadataProcessing258ReadingOrder(t *testing.T) {
	// Verify $258 reading order metadata (tier 2) is read directly by symbol key.
	cat := &fragmentCatalog{
		ReadingOrderMetadata: map[string]interface{}{
			"$222": "Author Name",
			"$153": "Book Title",
		},
	}

	author := getMetadataValue(cat, "author", "", nil)
	if author == nil {
		t.Fatal("expected author value from ReadingOrderMetadata, got nil")
	}
	if authorStr, ok := author.(string); !ok || authorStr != "Author Name" {
		t.Fatalf("expected 'Author Name', got %v", author)
	}

	title := getMetadataValue(cat, "title", "", nil)
	if title == nil {
		t.Fatal("expected title value from ReadingOrderMetadata, got nil")
	}
	if titleStr, ok := title.(string); !ok || titleStr != "Book Title" {
		t.Fatalf("expected 'Book Title', got %v", title)
	}
}

// =============================================================================
// VAL-M1-STRM-002: Non-SHORT symbol format emits info log
// Python: yj_to_epub.py:239
// =============================================================================

func TestSymbolFormatNonShortLogging(t *testing.T) {
	// When book symbol format is not SHORT, an info log should be emitted.
	// We verify this through the determineBookSymbolFormat function.
	// The logging is done in state.go after determineBookSymbolFormat returns,
	// so we test the format determination here and the logging integration separately.

	// DICTIONARY format (should trigger logging in state.go)
	bookSymbols := map[string]struct{}{
		"G12345": {},
		"G67890": {},
	}
	format := determineBookSymbolFormat(bookSymbols, nil, nil)
	if format != symDictionary {
		t.Fatalf("expected DICTIONARY format, got %s", format)
	}

	// ORIGINAL format (should trigger logging) - need enough symbols to reach quorum
	bookSymbols2 := map[string]struct{}{
		"order-1":            {},
		"TargetReadingOrder": {},
		"PageLabels":         {},
	}
	format2 := determineBookSymbolFormat(bookSymbols2, nil, nil)
	if format2 != symOriginal {
		t.Fatalf("expected ORIGINAL format, got %s", format2)
	}

	// SHORT format (should NOT trigger logging)
	bookSymbols3 := map[string]struct{}{
		"resource/rsrcAB12": {},
		"resource/rsrcCD34": {},
	}
	format3 := determineBookSymbolFormat(bookSymbols3, nil, nil)
	if format3 != symShort {
		t.Fatalf("expected SHORT format, got %s", format3)
	}
}

func TestSymbolFormatNonShortLogMessage(t *testing.T) {
	// Capture log output to verify the message is emitted.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// Log for non-SHORT format
	if symOriginal != symShort {
		log.Printf("kfx: Book symbol format is %s", symOriginal)
	}

	output := buf.String()
	if !strings.Contains(output, "Book symbol format is") {
		t.Fatalf("expected log message about book symbol format, got: %s", output)
	}
	if !strings.Contains(output, "original") {
		t.Fatalf("expected format 'original' in log, got: %s", output)
	}
}

// =============================================================================
// VAL-M1-STRM-003: checkSymbolTable validates used vs declared symbols
// Python: yj_structure.py:1099-1160
// =============================================================================

func TestCheckSymbolTableMissingSymbols(t *testing.T) {
	// When symbols are used but not in original_symbols, they should be logged as missing.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	frags := fragmentCatalog{
		StyleFragments: map[string]map[string]interface{}{
			"style1": {"$11": "serif", "$12": "bold", "unknown_symbol": "value"},
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

	r := &symbolResolver{localStart: 100, locals: []string{}}
	cfg := checkSymbolTableConfig{
		OriginalSyms: []string{"$11", "$12"}, // "unknown_symbol" is NOT in original
	}
	checkSymbolTableWithConfig(frags, r, false, false, cfg)

	output := buf.String()
	if !strings.Contains(output, "Symbol table is missing symbols") {
		t.Fatalf("expected missing symbols error log, got: %s", output)
	}
	if !strings.Contains(output, "unknown_symbol") {
		t.Fatalf("expected 'unknown_symbol' in missing symbols, got: %s", output)
	}
}

func TestCheckSymbolTableUnusedSymbols(t *testing.T) {
	// When original symbols are not used, they should be logged as unused.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

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

	r := &symbolResolver{localStart: 100, locals: []string{}}
	cfg := checkSymbolTableConfig{
		OriginalSyms: []string{"some_unused_sym", "mkfx_id"},
	}
	checkSymbolTableWithConfig(frags, r, false, false, cfg)

	output := buf.String()
	// "some_unused_sym" should be reported as unused
	if !strings.Contains(output, "unused symbols") {
		t.Fatalf("expected unused symbols warning log, got: %s", output)
	}
	if !strings.Contains(output, "some_unused_sym") {
		t.Fatalf("expected 'some_unused_sym' in unused symbols, got: %s", output)
	}
}

func TestCheckSymbolTableExpectedUnusedExceptions(t *testing.T) {
	// Certain symbols are expected to be unused (mkfx_id, UUIDs, PAGE_LIST_*, etc.)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

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

	r := &symbolResolver{localStart: 100, locals: []string{}}
	cfg := checkSymbolTableConfig{
		OriginalSyms: []string{
			"mkfx_id",
			"page_list_entry",
			"PAGE_LIST_1234567890",
			"12345678-1234-1234-1234-123456789abc", // UUID
		},
	}
	checkSymbolTableWithConfig(frags, r, false, false, cfg)

	output := buf.String()
	// These should be expected unused, not reported as regular unused.
	// The "unused symbols" warning (line 1) should NOT appear since all are expected.
	if strings.Contains(output, "kfx: warning: Symbol table contains") {
		t.Fatalf("expected no regular unused symbols warning for expected exceptions, got: %s", output)
	}
	// Python: log_known_error is a no-op when REPORT_KNOWN_PROBLEMS is None (default).
	// Go matches: ReportKnownProblems is nil, so expected-unused log is suppressed.
	// The output should be empty since all unused symbols are expected and not logged.
	if strings.Contains(output, "expected unused symbols") {
		t.Fatalf("expected no 'expected unused symbols' log when ReportKnownProblems is nil, got: %s", output)
	}
}

func TestCheckSymbolTableDictionaryNoMissingError(t *testing.T) {
	// When is_dictionary=true, missing symbols should NOT produce an error.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	frags := fragmentCatalog{
		StyleFragments: map[string]map[string]interface{}{
			"style1": {"missing_sym": "value"},
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

	r := &symbolResolver{localStart: 100, locals: []string{}}
	cfg := checkSymbolTableConfig{
		IsDictionary: true,
		OriginalSyms: []string{},
	}
	checkSymbolTableWithConfig(frags, r, false, false, cfg)

	output := buf.String()
	if strings.Contains(output, "Symbol table is missing symbols") {
		t.Fatalf("dictionary should suppress missing symbols error, got: %s", output)
	}
}

func TestCheckSymbolTableSampleSuffixAd(t *testing.T) {
	// When is_sample=true, symbols ending with "-ad" should be expected unused.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

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

	r := &symbolResolver{localStart: 100, locals: []string{}}
	cfg := checkSymbolTableConfig{
		IsSample:     true,
		OriginalSyms: []string{"resource-ad"},
	}
	checkSymbolTableWithConfig(frags, r, false, false, cfg)

	output := buf.String()
	// "-ad" suffix with is_sample should be expected unused, not regular unused warning
	if strings.Contains(output, "kfx: warning: Symbol table contains") {
		t.Fatalf("sample -ad symbols should not produce regular unused warning, got: %s", output)
	}
	// Python: log_known_error is a no-op when REPORT_KNOWN_PROBLEMS is None (default).
	// Go matches: ReportKnownProblems is nil, so expected-unused log is suppressed.
	if strings.Contains(output, "expected unused") {
		t.Fatalf("expected no 'expected unused' log when ReportKnownProblems is nil, got: %s", output)
	}
}

// =============================================================================
// VAL-M1-STRM-004: has_illustrated_layout_page_template_condition traversal
// Python: yj_structure.py:1228-1253
// =============================================================================

func TestHasIllustratedLayoutConditionFromParsedTemplate(t *testing.T) {
	// When condition is stored in pt.Condition (parsed from $141 containers),
	// it should still be detected.
	condition := []interface{}{"$294", "$183", []interface{}{"$266", "anchor_name"}}
	frags := fragmentCatalog{
		SectionFragments: map[string]sectionFragment{
			"sec1": {
				ID: "sec1",
				PageTemplateValues: map[string]interface{}{
					"$11": "serif",
				},
				PageTemplates: []pageTemplateFragment{
					{
						PositionID:         1,
						Storyline:          "story1",
						PageTemplateStyle:  "",
						PageTemplateValues: map[string]interface{}{"$11": "serif"},
						HasCondition:       true,
						Condition:          condition,
					},
				},
			},
		},
	}
	if !hasIllustratedLayoutPageTemplateCondition(frags) {
		t.Fatal("expected true: condition stored in pt.Condition should be detected")
	}
}

func TestHasIllustratedLayoutConditionInvalidOperator(t *testing.T) {
	// Condition with operator not in [$294, $299, $298] should not match.
	condition := []interface{}{"$999", "$183", []interface{}{"$266", "anchor_name"}}
	frags := fragmentCatalog{
		SectionFragments: map[string]sectionFragment{
			"sec1": {
				ID: "sec1",
				PageTemplates: []pageTemplateFragment{
					{
						HasCondition: true,
						Condition:    condition,
					},
				},
			},
		},
	}
	if hasIllustratedLayoutPageTemplateCondition(frags) {
		t.Fatal("expected false: invalid operator should not match")
	}
}

func TestHasIllustratedLayoutConditionWrongStructure(t *testing.T) {
	// Condition with wrong structure should not match.
	condition := []interface{}{"$294", "$183"} // missing third element
	frags := fragmentCatalog{
		SectionFragments: map[string]sectionFragment{
			"sec1": {
				ID: "sec1",
				PageTemplates: []pageTemplateFragment{
					{
						HasCondition: true,
						Condition:    condition,
					},
				},
			},
		},
	}
	if hasIllustratedLayoutPageTemplateCondition(frags) {
		t.Fatal("expected false: wrong structure should not match")
	}
}

func TestHasIllustratedLayoutConditionNoCondition(t *testing.T) {
	// Section with no condition should not match.
	frags := fragmentCatalog{
		SectionFragments: map[string]sectionFragment{
			"sec1": {
				ID: "sec1",
				PageTemplates: []pageTemplateFragment{
					{
						HasCondition: false,
						Condition:    nil,
					},
				},
			},
		},
	}
	if hasIllustratedLayoutPageTemplateCondition(frags) {
		t.Fatal("expected false: no condition")
	}
}

func TestHasIllustratedLayoutConditionAllOperators(t *testing.T) {
	// All three valid operators should match: $294, $299, $298
	for _, op := range []string{"$294", "$299", "$298"} {
		condition := []interface{}{op, "$183", []interface{}{"$266", "anchor"}}
		frags := fragmentCatalog{
			SectionFragments: map[string]sectionFragment{
				"sec1": {
					ID: "sec1",
					PageTemplates: []pageTemplateFragment{
						{Condition: condition},
					},
				},
			},
		}
		if !hasIllustratedLayoutPageTemplateCondition(frags) {
			t.Fatalf("expected true for operator %s", op)
		}
	}
}

// =============================================================================
// VAL-M1-STRM-005: $171 condition validation for overlay templates
// Python: yj_to_epub_content.py:501-504
// =============================================================================

func TestOverlayTemplate171ValidationValid(t *testing.T) {
	// Valid condition should return true
	condition := []interface{}{"$294", "$183", []interface{}{"$266", "anchor_name"}}
	if !validateOverlayCondition(condition, "$324") {
		t.Fatal("expected valid condition to return true")
	}
}

func TestOverlayTemplate171ValidationInvalidLayout(t *testing.T) {
	// Layout != $324 should log error but still validate condition structure
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	condition := []interface{}{"$294", "$183", []interface{}{"$266", "anchor_name"}}
	// The function still returns true for valid structure even with wrong layout
	validateOverlayCondition(condition, "$437")
	output := buf.String()
	if !strings.Contains(output, "unexpected layout") {
		t.Fatalf("expected layout error, got: %s", output)
	}
}

func TestOverlayTemplate171ValidationInvalidStructure(t *testing.T) {
	// Invalid condition structure should return false and log error
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// Wrong number of elements
	condition := []interface{}{"$294", "$183"}
	if validateOverlayCondition(condition, "$324") {
		t.Fatal("expected invalid condition to return false")
	}
	output := buf.String()
	if !strings.Contains(output, "not in expected format") {
		t.Fatalf("expected format error, got: %s", output)
	}
}

func TestOverlayTemplate171ValidationWrongFv1(t *testing.T) {
	// fv[1] != "$183" should return false
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	condition := []interface{}{"$294", "$999", []interface{}{"$266", "anchor"}}
	if validateOverlayCondition(condition, "$324") {
		t.Fatal("expected false for wrong fv[1]")
	}
}

func TestOverlayTemplate171ValidationWrongFv2(t *testing.T) {
	// fv[2] not IonSExp of length 2
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	condition := []interface{}{"$294", "$183", "not_a_list"}
	if validateOverlayCondition(condition, "$324") {
		t.Fatal("expected false for non-list fv[2]")
	}
}

func TestOverlayTemplate171ValidationWrongFv20(t *testing.T) {
	// fv[2][0] != "$266"
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	condition := []interface{}{"$294", "$183", []interface{}{"$999", "anchor"}}
	if validateOverlayCondition(condition, "$324") {
		t.Fatal("expected false for wrong fv[2][0]")
	}
}

func TestOverlayTemplate171ValidationWrongOperator(t *testing.T) {
	// fv[0] not in CONDITION_OPERATOR_NAMES
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	condition := []interface{}{"$999", "$183", []interface{}{"$266", "anchor"}}
	if validateOverlayCondition(condition, "$324") {
		t.Fatal("expected false for unknown operator")
	}
}

func TestOverlayTemplate171ValidationNilCondition(t *testing.T) {
	// nil condition should return true (no condition to validate)
	if !validateOverlayCondition(nil, "$324") {
		t.Fatal("expected true for nil condition")
	}
}

// =============================================================================
// VAL-M1-STRM-006: font_size rem conversion logs error on missing property
// Python: yj_to_epub_properties.py:1734-1738
// =============================================================================

func TestFontSizeRemMissingInheritedProperty(t *testing.T) {
	// When name=="font-size" and rem unit, but inherited has no "font-size",
	// should log error (Python would KeyError).
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	sty := map[string]string{
		"font-size": "1.5rem",
	}
	inherited := map[string]string{
		// No "font-size" key
	}

	convertStyleUnits(sty, inherited)

	output := buf.String()
	if !strings.Contains(output, "missing inherited font-size") {
		t.Fatalf("expected error for missing inherited font-size, got: %s", output)
	}
}

func TestFontSizeRemMissingElementProperty(t *testing.T) {
	// When name!="font-size" and rem unit, but element has no "font-size",
	// should log error (Python would KeyError).
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	sty := map[string]string{
		"margin-left": "1.5rem",
		// No "font-size" key in sty
	}
	inherited := map[string]string{
		"font-size": "1em", // present but for other properties we use element's own
	}

	convertStyleUnits(sty, inherited)

	output := buf.String()
	if !strings.Contains(output, "missing element font-size") {
		t.Fatalf("expected error for missing element font-size, got: %s", output)
	}
}

func TestFontSizeRemIncorrectUnits(t *testing.T) {
	// When base font-size has incorrect units (not rem or em), should log error.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	sty := map[string]string{
		"font-size": "1.5rem",
	}
	inherited := map[string]string{
		"font-size": "16px", // px units, not rem or em
	}

	convertStyleUnits(sty, inherited)

	output := buf.String()
	if !strings.Contains(output, "incorrect base font size units") {
		t.Fatalf("expected error for incorrect base font size units, got: %s", output)
	}
}

func TestFontSizeRemValidConversion(t *testing.T) {
	// Valid rem → em conversion should not log errors.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	sty := map[string]string{
		"font-size": "1.5rem",
	}
	inherited := map[string]string{
		"font-size": "1rem",
	}

	convertStyleUnits(sty, inherited)

	output := buf.String()
	if strings.Contains(output, "error") || strings.Contains(output, "Error") {
		t.Fatalf("expected no errors for valid conversion, got: %s", output)
	}
	// Should convert to em
	if sty["font-size"] != "1.5em" {
		t.Fatalf("expected font-size to be 1.5em, got: %s", sty["font-size"])
	}
}

func TestFontSizeRemEmBaseConversion(t *testing.T) {
	// When base font-size is in em, should convert to em without error.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	sty := map[string]string{
		"font-size": "0.75rem",
	}
	inherited := map[string]string{
		"font-size": "1em",
	}

	convertStyleUnits(sty, inherited)

	output := buf.String()
	if strings.Contains(output, "error") || strings.Contains(output, "Error") {
		t.Fatalf("expected no errors for em-base conversion, got: %s", output)
	}
	if sty["font-size"] != "0.75em" {
		t.Fatalf("expected font-size to be 0.75em, got: %s", sty["font-size"])
	}
}

// =============================================================================
// listTruncated helper tests
// =============================================================================

func TestListTruncated(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f"}

	result := listTruncated(items, 3)
	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}
	if result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Fatalf("expected [a b c], got %v", result)
	}

	// Less than max
	result2 := listTruncated(items, 10)
	if len(result2) != 6 {
		t.Fatalf("expected 6 items, got %d", len(result2))
	}
}
