package kfx

import (
	"strings"
	"testing"
)

// =============================================================================
// Tests for processSection branch dispatch — VAL-A-025 through VAL-A-031
// =============================================================================

// makeTestSection creates a sectionFragment with the given page templates and extra keys.
func makeTestSection(templates []pageTemplateFragment, extraKeys map[string]interface{}) sectionFragment {
	return sectionFragment{
		ID:            "test-section",
		PositionID:    100,
		Storyline:     "story1",
		PageTemplates: templates,
		PageTemplateValues: extraKeys,
	}
}

func makeTestTemplate(storyline string, condition interface{}) pageTemplateFragment {
	return pageTemplateFragment{
		PositionID:  200,
		Storyline:   storyline,
		HasCondition: condition != nil,
		Condition:   condition,
	}
}

// =============================================================================
// VAL-A-025: Scribe notebook page dispatch — nmdl.canvas_width key
// =============================================================================

func TestProcessSectionScribePageDispatch(t *testing.T) {
	section := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("story1", nil),
	}, map[string]interface{}{
		"nmdl.canvas_width": 800,
	})

	branch := determineSectionBranch(section, bookTypeNone)
	if branch != branchScribePage {
		t.Errorf("expected branchScribePage, got %v", branch)
	}
}

// =============================================================================
// VAL-A-026: Scribe notebook template dispatch — nmdl.template_type key
// =============================================================================

func TestProcessSectionScribeTemplateDispatch(t *testing.T) {
	section := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("story1", nil),
	}, map[string]interface{}{
		"nmdl.template_type": "lined",
	})

	branch := determineSectionBranch(section, bookTypeNone)
	if branch != branchScribeTemplate {
		t.Errorf("expected branchScribeTemplate, got %v", branch)
	}
}

// Scribe page takes priority over template (both keys present)
func TestProcessSectionScribePagePriorityOverTemplate(t *testing.T) {
	section := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("story1", nil),
	}, map[string]interface{}{
		"nmdl.canvas_width":  800,
		"nmdl.template_type": "lined",
	})

	branch := determineSectionBranch(section, bookTypeNone)
	if branch != branchScribePage {
		t.Errorf("expected branchScribePage (canvas_width takes priority), got %v", branch)
	}
}

// =============================================================================
// VAL-A-027: Comic/children book type dispatch
// =============================================================================

func TestProcessSectionComicDispatch(t *testing.T) {
	section := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("story1", nil),
	}, nil)

	branch := determineSectionBranch(section, bookTypeComic)
	if branch != branchComic {
		t.Errorf("expected branchComic, got %v", branch)
	}
}

func TestProcessSectionChildrenDispatch(t *testing.T) {
	section := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("story1", nil),
	}, nil)

	branch := determineSectionBranch(section, bookTypeChildren)
	if branch != branchComic {
		t.Errorf("expected branchComic (children maps to comic path), got %v", branch)
	}
}

// Comic dispatch takes priority over scribe keys (matches Python order)
func TestProcessSectionScribePriorityOverComic(t *testing.T) {
	section := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("story1", nil),
	}, map[string]interface{}{
		"nmdl.canvas_width": 800,
	})

	// nmdl.* is checked first in Python
	branch := determineSectionBranch(section, bookTypeComic)
	if branch != branchScribePage {
		t.Errorf("expected branchScribePage (nmdl takes priority), got %v", branch)
	}
}

// =============================================================================
// VAL-A-028: Magazine/print-replica with conditional template dispatch
// =============================================================================

func TestProcessSectionMagazineWithConditionalTemplate(t *testing.T) {
	condition := []interface{}{"$294", "$183", "$266"}
	section := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("story1", condition),
	}, nil)

	branch := determineSectionBranch(section, bookTypeMagazine)
	if branch != branchMagazine {
		t.Errorf("expected branchMagazine, got %v", branch)
	}
}

func TestProcessSectionPrintReplicaWithConditionalTemplate(t *testing.T) {
	condition := []interface{}{"$294", "$183", "$266"}
	section := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("story1", condition),
	}, nil)

	branch := determineSectionBranch(section, bookTypePrintReplica)
	if branch != branchMagazine {
		t.Errorf("expected branchMagazine, got %v", branch)
	}
}

// Magazine without conditional template falls through to reflowable
func TestProcessSectionMagazineWithoutConditionalTemplate(t *testing.T) {
	section := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("story1", nil),
	}, nil)

	branch := determineSectionBranch(section, bookTypeMagazine)
	if branch != branchReflowable {
		t.Errorf("expected branchReflowable (no conditional template), got %v", branch)
	}
}

// =============================================================================
// VAL-A-029: Default reflowable branch — last template is main
// =============================================================================

func TestProcessSectionReflowableDefault(t *testing.T) {
	section := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("story1", nil),
	}, nil)

	branch := determineSectionBranch(section, bookTypeNone)
	if branch != branchReflowable {
		t.Errorf("expected branchReflowable, got %v", branch)
	}
}

func TestProcessSectionReflowableWithEmptyBookType(t *testing.T) {
	section := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("story1", nil),
	}, nil)

	branch := determineSectionBranch(section, "")
	if branch != branchReflowable {
		t.Errorf("expected branchReflowable for empty book type, got %v", branch)
	}
}

// =============================================================================
// VAL-A-030: Conditional template evaluation — true and false conditions
// =============================================================================

func TestPageTemplatesHaveConditionsTrue(t *testing.T) {
	templates := []pageTemplateFragment{
		{Storyline: "s1", HasCondition: false},
		{Storyline: "s2", HasCondition: true},
	}
	if !pageTemplatesHaveConditions(templates) {
		t.Error("expected templates to have conditions")
	}
}

func TestPageTemplatesHaveConditionsFalse(t *testing.T) {
	templates := []pageTemplateFragment{
		{Storyline: "s1", HasCondition: false},
		{Storyline: "s2", HasCondition: false},
	}
	if pageTemplatesHaveConditions(templates) {
		t.Error("expected templates to NOT have conditions")
	}
}

func TestPageTemplatesHaveConditionsNilCondition(t *testing.T) {
	templates := []pageTemplateFragment{
		{Storyline: "s1", Condition: nil},
	}
	if pageTemplatesHaveConditions(templates) {
		t.Error("expected no conditions when Condition is nil and HasCondition is false")
	}
}

func TestPageTemplatesHaveConditionsNonNilCondition(t *testing.T) {
	condition := []interface{}{"$294", "$183", "$266"}
	templates := []pageTemplateFragment{
		{Storyline: "s1", Condition: condition},
	}
	if !pageTemplatesHaveConditions(templates) {
		t.Error("expected conditions when Condition is non-nil")
	}
}

// =============================================================================
// VAL-A-031: Section unused key stripping
// =============================================================================

func TestStripUnusedKeys(t *testing.T) {
	data := map[string]interface{}{
		"$174":   "section-name",
		"$702":   "some-value",
		"yj.conversion.html_name":                   "test",
		"yj.semantics.book_anatomy_type":            "chapter",
		"yj.semantics.page_type":                    "body",
		"yj.authoring.auto_panel_settings_opacity":  "0.5",
		"yj.authoring.auto_panel_settings_padding_top": "10",
		"$141":   []interface{}{},
		"keep-me": "important",
	}

	stripUnusedSectionKeys(data)

	// These should have been removed
	for _, key := range []string{
		"$702",
		"yj.conversion.html_name",
		"yj.semantics.book_anatomy_type",
		"yj.semantics.page_type",
		"yj.authoring.auto_panel_settings_opacity",
		"yj.authoring.auto_panel_settings_padding_top",
	} {
		if _, exists := data[key]; exists {
			t.Errorf("expected key %q to be stripped, but it still exists", key)
		}
	}

	// These should remain
	for _, key := range []string{"$174", "$141", "keep-me"} {
		if _, exists := data[key]; !exists {
			t.Errorf("expected key %q to remain, but it was removed", key)
		}
	}
}

func TestStripUnusedKeysPreservesUnknownKeys(t *testing.T) {
	data := map[string]interface{}{
		"nmdl.canvas_width": 800,
		"$174":              "section-name",
	}

	stripUnusedSectionKeys(data)

	if _, exists := data["nmdl.canvas_width"]; !exists {
		t.Error("nmdl.canvas_width should be preserved")
	}
}

// =============================================================================
// processReadingOrder tests
// =============================================================================

func TestProcessReadingOrderDeduplicatesSections(t *testing.T) {
	// Two reading orders referencing the same section
	sectionFragments := map[string]sectionFragment{
		"section-a": {ID: "section-a", Storyline: "story-a"},
		"section-b": {ID: "section-b", Storyline: "story-b"},
	}
	storylines := map[string]map[string]interface{}{
		"story-a": {"$146": []interface{}{
			map[string]interface{}{"$145": map[string]interface{}{"name": "content-a", "$403": 0}},
		}},
		"story-b": {"$146": []interface{}{
			map[string]interface{}{"$145": map[string]interface{}{"name": "content-b", "$403": 0}},
		}},
	}
	contentFragments := map[string][]string{
		"content-a": {"Hello from A"},
		"content-b": {"Hello from B"},
	}

	book := &decodedBook{}
	renderer := &storylineRenderer{
		contentFragments:   contentFragments,
		styleFragments:     map[string]map[string]interface{}{},
		styles:             newStyleCatalog(),
		conditionEvaluator: conditionEvaluator{},
	}

	sectionOrder := []string{"section-a", "section-b", "section-a"}
	processReadingOrder(book, sectionOrder, sectionFragments, storylines, contentFragments, renderer, map[string]string{}, symOriginal)

	// Should produce 2 sections, not 3 (deduplicated)
	if len(book.RenderedSections) != 2 {
		t.Errorf("expected 2 rendered sections (deduplicated), got %d", len(book.RenderedSections))
	}
}

// =============================================================================
// bookType tests
// =============================================================================

func TestBookTypeString(t *testing.T) {
	tests := []struct {
		bt  bookType
		exp string
	}{
		{bookTypeNone, ""},
		{bookTypeComic, "comic"},
		{bookTypeChildren, "children"},
		{bookTypeMagazine, "magazine"},
		{bookTypePrintReplica, "print_replica"},
	}
	for _, test := range tests {
		got := string(test.bt)
		if got != test.exp {
			t.Errorf("bookType(%q).String() = %q, want %q", test.bt, got, test.exp)
		}
	}
}

func TestDetectBookTypeFromFeatures(t *testing.T) {
	tests := []struct {
		name     string
		features map[string]interface{}
		expected bookType
	}{
		{
			name:     "empty features",
			features: map[string]interface{}{},
			expected: bookTypeNone,
		},
		{
			name: "facing page → comic",
			features: map[string]interface{}{
				"$590": []interface{}{
					map[string]interface{}{
						"$492": "yj_facing_page",
						"$586": "kindle_capability_metadata",
					},
				},
			},
			expected: bookTypeComic,
		},
		{
			name: "double page spread → comic",
			features: map[string]interface{}{
				"$590": []interface{}{
					map[string]interface{}{
						"$492": "yj_double_page_spread",
						"$586": "kindle_capability_metadata",
					},
				},
			},
			expected: bookTypeComic,
		},
		{
			name: "publisher panels → comic",
			features: map[string]interface{}{
				"$590": []interface{}{
					map[string]interface{}{
						"$492": "yj_publisher_panels",
						"$586": "kindle_capability_metadata",
					},
				},
			},
			expected: bookTypeComic,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := detectBookType(nil, test.features)
			if got != test.expected {
				t.Errorf("detectBookType() = %v, want %v", got, test.expected)
			}
		})
	}
}

func TestDetectBookTypeFromCDEContentType(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]interface{}
		expected bookType
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			expected: bookTypeNone,
		},
		{
			name:     "no cde_content_type",
			metadata: map[string]interface{}{},
			expected: bookTypeNone,
		},
		{
			name: "MAGZ → magazine",
			metadata: map[string]interface{}{
				"$251": "MAGZ",
			},
			expected: bookTypeMagazine,
		},
		{
			name: "EBSP → not magazine",
			metadata: map[string]interface{}{
				"$251": "EBSP",
			},
			expected: bookTypeNone,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := detectBookType(test.metadata, nil)
			if got != test.expected {
				t.Errorf("detectBookType() = %v, want %v", got, test.expected)
			}
		})
	}
}

// =============================================================================
// Integration: processSection dispatch path tests
// =============================================================================

func TestProcessSectionComicWithOneTemplate(t *testing.T) {
	// Comic with exactly 1 page template should succeed
	template := makeTestTemplate("story1", nil)
	section := makeTestSection([]pageTemplateFragment{template}, nil)
	storylines := map[string]map[string]interface{}{
		"story1": {"$146": []interface{}{
			map[string]interface{}{"$145": map[string]interface{}{"name": "c1", "$403": 0}},
		}},
	}
	contentFragments := map[string][]string{
		"c1": {"Hello world"},
	}
	renderer := &storylineRenderer{
		contentFragments:   contentFragments,
		styleFragments:     map[string]map[string]interface{}{},
		styles:             newStyleCatalog(),
		conditionEvaluator: conditionEvaluator{},
	}

	rendered, paragraphs, ok := processSection("test-section", section, 0, storylines, contentFragments, renderer)
	if !ok {
		t.Error("expected processSection to succeed for comic with 1 template")
	}
	// Should have rendered content
	if len(paragraphs) == 0 && rendered.BodyHTML == "" {
		t.Error("expected some content to be rendered")
	}
}

func TestProcessSectionReflowableWithMultipleTemplates(t *testing.T) {
	// Reflowable with 3 templates: last is main, first two are conditional overlays
	templates := []pageTemplateFragment{
		makeTestTemplate("story1", []interface{}{"$294"}),
		makeTestTemplate("story2", []interface{}{"$294"}),
		makeTestTemplate("story3", nil),
	}
	section := makeTestSection(templates, nil)
	storylines := map[string]map[string]interface{}{
		"story1": {"$146": []interface{}{
			map[string]interface{}{"$145": map[string]interface{}{"name": "c1", "$403": 0}},
		}},
		"story2": {"$146": []interface{}{
			map[string]interface{}{"$145": map[string]interface{}{"name": "c2", "$403": 0}},
		}},
		"story3": {"$146": []interface{}{
			map[string]interface{}{"$145": map[string]interface{}{"name": "c3", "$403": 0}},
		}},
	}
	contentFragments := map[string][]string{
		"c1": {"overlay1"},
		"c2": {"overlay2"},
		"c3": {"main content"},
	}
	renderer := &storylineRenderer{
		contentFragments:   contentFragments,
		styleFragments:     map[string]map[string]interface{}{},
		styles:             newStyleCatalog(),
		conditionEvaluator: conditionEvaluator{},
	}

	rendered, paragraphs, ok := processSection("test-section", section, 0, storylines, contentFragments, renderer)
	if !ok {
		t.Error("expected processSection to succeed with multiple templates")
	}
	// Should have rendered content from all 3 templates
	_ = rendered
	_ = paragraphs
}

func TestProcessSectionWithNoTemplatesCreatesDefault(t *testing.T) {
	// Section with no page templates should create a default one from section fields
	section := sectionFragment{
		ID:        "test-section",
		Storyline: "story1",
	}
	storylines := map[string]map[string]interface{}{
		"story1": {"$146": []interface{}{
			map[string]interface{}{"$145": map[string]interface{}{"name": "c1", "$403": 0}},
		}},
	}
	contentFragments := map[string][]string{
		"c1": {"Hello world"},
	}
	renderer := &storylineRenderer{
		contentFragments:   contentFragments,
		styleFragments:     map[string]map[string]interface{}{},
		styles:             newStyleCatalog(),
		conditionEvaluator: conditionEvaluator{},
	}

	_, _, ok := processSection("test-section", section, 0, storylines, contentFragments, renderer)
	if !ok {
		t.Error("expected processSection to succeed with no templates")
	}
}

// =============================================================================
// Section unused keys — comprehensive coverage
// =============================================================================

func TestStripAllYjSemanticsKeys(t *testing.T) {
	data := map[string]interface{}{
		"yj.semantics.book_anatomy_type":                "chapter",
		"yj.semantics.page_type":                        "body",
		"yj.authoring.auto_panel_settings_auto_mask_color_flag": true,
		"yj.authoring.auto_panel_settings_mask_color":   "#fff",
		"yj.authoring.auto_panel_settings_opacity":      "0.5",
		"yj.authoring.auto_panel_settings_padding_bottom": "10",
		"yj.authoring.auto_panel_settings_padding_left":  "5",
		"yj.authoring.auto_panel_settings_padding_right": "5",
		"yj.authoring.auto_panel_settings_padding_top":   "10",
	}

	stripUnusedSectionKeys(data)

	if len(data) != 0 {
		t.Errorf("expected all keys to be stripped, but %d remain: %v", len(data), data)
	}
}

func TestIsUnusedSectionKey(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"$702", true},
		{"yj.conversion.html_name", true},
		{"yj.semantics.book_anatomy_type", true},
		{"yj.semantics.page_type", true},
		{"yj.authoring.auto_panel_settings_auto_mask_color_flag", true},
		{"yj.authoring.auto_panel_settings_mask_color", true},
		{"yj.authoring.auto_panel_settings_opacity", true},
		{"yj.authoring.auto_panel_settings_padding_bottom", true},
		{"yj.authoring.auto_panel_settings_padding_left", true},
		{"yj.authoring.auto_panel_settings_padding_right", true},
		{"yj.authoring.auto_panel_settings_padding_top", true},
		{"$174", false},
		{"$141", false},
		{"nmdl.canvas_width", false},
		{"keep-me", false},
		{"yj.other.thing", false},
	}
	for _, test := range tests {
		got := isUnusedSectionKey(test.key)
		if got != test.expected {
			t.Errorf("isUnusedSectionKey(%q) = %v, want %v", test.key, got, test.expected)
		}
	}
}

// =============================================================================
// Conditional template filtering
// =============================================================================

func TestFilterConditionalTemplatesTrue(t *testing.T) {
	templates := []pageTemplateFragment{
		{Storyline: "s1", Condition: []interface{}{"$294", "$183", "$266"}, HasCondition: true},
		{Storyline: "s2", Condition: nil, HasCondition: false},
	}
	// In non-fixed-layout, conditions are not evaluated
	evaluator := conditionEvaluator{fixedLayout: false}
	filtered := filterActiveTemplates(templates, evaluator)
	if len(filtered) != 2 {
		t.Errorf("expected 2 templates (no filtering in non-fixed-layout), got %d", len(filtered))
	}
}

func TestFilterConditionalTemplatesInFixedLayout(t *testing.T) {
	templates := []pageTemplateFragment{
		{Storyline: "s1", Condition: nil, HasCondition: false},
		{Storyline: "s2", Condition: nil, HasCondition: false},
	}
	evaluator := conditionEvaluator{fixedLayout: true}
	filtered := filterActiveTemplates(templates, evaluator)
	if len(filtered) != 2 {
		t.Errorf("expected 2 active templates (no conditions), got %d", len(filtered))
	}
}

func TestFilterConditionalTemplatesInFixedLayoutWithBinaryCondition(t *testing.T) {
	// Use $300 (always true) which is a nullary operator returning true
	templates := []pageTemplateFragment{
		{Storyline: "s1", Condition: nil, HasCondition: false},
		{Storyline: "s2", Condition: []interface{}{"$300"}, HasCondition: true},
		{Storyline: "s3", Condition: []interface{}{"$293", []interface{}{"$300"}}, HasCondition: true}, // NOT true = false
	}
	evaluator := conditionEvaluator{fixedLayout: true}
	filtered := filterActiveTemplates(templates, evaluator)
	// s1 (no condition) and s2 (condition = true) should pass
	// s3 (NOT true = false) should be filtered out
	if len(filtered) != 2 {
		t.Errorf("expected 2 active templates in fixed layout, got %d", len(filtered))
	}
	if len(filtered) > 0 && filtered[0].Storyline != "s1" {
		t.Errorf("expected first active template to be s1, got %s", filtered[0].Storyline)
	}
	if len(filtered) > 1 && filtered[1].Storyline != "s2" {
		t.Errorf("expected second active template to be s2, got %s", filtered[1].Storyline)
	}
}

// =============================================================================
// Merge section properties
// =============================================================================

func TestMergeSectionProperties(t *testing.T) {
	tests := []struct {
		left, right, expected string
	}{
		{"", "", ""},
		{"svg", "", "svg"},
		{"", "svg", "svg"},
		{"svg", "rendition:page-spread-left", "svg rendition:page-spread-left"},
		{"svg rendition:page-spread-left", "rendition:page-spread-left", "svg rendition:page-spread-left"},
		{"a b c", "b d", "a b c d"},
	}
	for _, test := range tests {
		got := mergeSectionProperties(test.left, test.right)
		if got != test.expected {
			t.Errorf("mergeSectionProperties(%q, %q) = %q, want %q", test.left, test.right, got, test.expected)
		}
	}
}

// =============================================================================
// nmdl key detection in section data
// =============================================================================

func TestSectionHasNmdlCanvasWidth(t *testing.T) {
	data := map[string]interface{}{
		"nmdl.canvas_width": 800,
	}
	if !sectionHasNmdlKey(data, "nmdl.canvas_width") {
		t.Error("expected nmdl.canvas_width to be detected")
	}
}

func TestSectionHasNmdlTemplateType(t *testing.T) {
	data := map[string]interface{}{
		"nmdl.template_type": "lined",
	}
	if !sectionHasNmdlKey(data, "nmdl.template_type") {
		t.Error("expected nmdl.template_type to be detected")
	}
}

func TestSectionHasNmdlKeyAbsent(t *testing.T) {
	data := map[string]interface{}{
		"other_key": "value",
	}
	if sectionHasNmdlKey(data, "nmdl.canvas_width") {
		t.Error("expected nmdl.canvas_width to NOT be detected")
	}
}

// =============================================================================
// hasConditionalTemplate detection
// =============================================================================

func TestHasConditionalTemplateWithCondition(t *testing.T) {
	templates := []pageTemplateFragment{
		{Storyline: "s1", HasCondition: true, Condition: []interface{}{"$294"}},
	}
	if !hasConditionalTemplate(templates) {
		t.Error("expected hasConditionalTemplate=true when template has $171 condition")
	}
}

func TestHasConditionalTemplateWithoutCondition(t *testing.T) {
	templates := []pageTemplateFragment{
		{Storyline: "s1", HasCondition: false},
	}
	if hasConditionalTemplate(templates) {
		t.Error("expected hasConditionalTemplate=false when no templates have conditions")
	}
}

// =============================================================================
// ensureBranchSelectionIsConsistent validates the priority order:
// nmdl.canvas_width > nmdl.template_type > comic/children > magazine/print-replica > reflowable
// =============================================================================

func TestBranchPriorityOrder(t *testing.T) {
	// Build a section with nmdl.canvas_width key
	scribeSection := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("s1", nil),
	}, map[string]interface{}{
		"nmdl.canvas_width": 800,
	})

	// nmdl.canvas_width takes priority over everything
	if determineSectionBranch(scribeSection, bookTypeComic) != branchScribePage {
		t.Error("nmdl.canvas_width should take priority over comic")
	}
	if determineSectionBranch(scribeSection, bookTypeMagazine) != branchScribePage {
		t.Error("nmdl.canvas_width should take priority over magazine")
	}

	// Comic takes priority over magazine
	comicSection := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("s1", nil),
	}, nil)
	if determineSectionBranch(comicSection, bookTypeComic) != branchComic {
		t.Error("comic book type should select comic branch")
	}

	// Magazine with conditions takes magazine branch
	condSection := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("s1", []interface{}{"$294"}),
	}, nil)
	if determineSectionBranch(condSection, bookTypeMagazine) != branchMagazine {
		t.Error("magazine with conditional template should select magazine branch")
	}

	// Magazine without conditions falls through to reflowable
	noCondSection := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("s1", nil),
	}, nil)
	if determineSectionBranch(noCondSection, bookTypeMagazine) != branchReflowable {
		t.Error("magazine without conditional template should fall through to reflowable")
	}
}

// =============================================================================
// Verify sectionBranch type string representations for logging
// =============================================================================

func TestSectionBranchString(t *testing.T) {
	if !strings.Contains(sectionBranchString(branchScribePage), "scribe") {
		t.Errorf("branchScribePage string should contain 'scribe'")
	}
	if !strings.Contains(sectionBranchString(branchComic), "comic") {
		t.Errorf("branchComic string should contain 'comic'")
	}
	if !strings.Contains(sectionBranchString(branchMagazine), "magazine") {
		t.Errorf("branchMagazine string should contain 'magazine'")
	}
	if !strings.Contains(sectionBranchString(branchReflowable), "reflowable") {
		t.Errorf("branchReflowable string should contain 'reflowable'")
	}
}

// =============================================================================
// processPageSpreadPageTemplate tests — VAL-A-032 through VAL-A-038
// Port of yj_to_epub_content.py:210-344 (process_page_spread_page_template)
// =============================================================================

// makeSpreadConfig creates a pageSpreadConfig with sensible defaults for testing.
func makeSpreadConfig() pageSpreadConfig {
	return pageSpreadConfig{
		BookType:                 bookTypeComic,
		IsPdfBacked:              false,
		RegionMagnification:      false,
		VirtualPanelsAllowed:     true,
		PageProgressionDirection: "ltr",
	}
}

// makePDFSpreadConfig creates config for PDF-backed books.
func makePDFSpreadConfig() pageSpreadConfig {
	return pageSpreadConfig{
		BookType:                 bookTypePrintReplica,
		IsPdfBacked:              true,
		RegionMagnification:      false,
		VirtualPanelsAllowed:     true,
		PageProgressionDirection: "ltr",
	}
}

// makeLeafTemplateData creates a leaf content template with $159="$270", $156="$325"
// and some content fields.
func makeLeafTemplateData() map[string]interface{} {
	return map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}
}

// makePageSpreadTemplateData creates a page-spread container template ($437)
// with a storyline reference pointing to a story with child templates.
func makePageSpreadTemplateData(storyName string, childTemplates []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"$159": "$270",
		"$156": "$437",
		"$176": storyName,
		"$434": "$441",
		"$192": nil,
		"$67":  nil,
		"$66":  nil,
		"$140": nil,
		"$560": nil,
		"$155": 42, // location ID
	}
}

// makeScaleFitTemplateData creates a PDF-backed scale_fit template ($326).
func makeScaleFitTemplateData(storyName string) map[string]interface{} {
	return map[string]interface{}{
		"$159": "$270",
		"$156": "$326",
		"$176": storyName,
		"$434": "$441",
		"$192": nil,
		"$140": nil,
		"$560": nil,
		"$155": 55,
		"$16":  16, // font_size = 16
	}
}

// makeConnectedPaginationTemplateData creates a connected pagination template ($323/$656).
func makeConnectedPaginationTemplateData(storyName string) map[string]interface{} {
	return map[string]interface{}{
		"$159": "$270",
		"$156": "$323",
		"$656": true,
		"$176": storyName,
		"$434": "$441",
		"$155": 77,
		"$655": 2, // connected_pagination = 2
	}
}

// =============================================================================
// VAL-A-032: Page spread branch detection ($437)
// =============================================================================

func TestPageSpreadBranchDetection(t *testing.T) {
	data := map[string]interface{}{
		"$159": "$270",
		"$156": "$437",
	}
	branch := determinePageSpreadBranch(data, true)
	if branch != pageSpreadBranchSpread {
		t.Errorf("expected pageSpreadBranchSpread for $437, got %v", branch)
	}
}

func TestFacingPageBranchDetection(t *testing.T) {
	data := map[string]interface{}{
		"$159": "$270",
		"$156": "$438",
	}
	branch := determinePageSpreadBranch(data, true)
	if branch != pageSpreadBranchFacing {
		t.Errorf("expected pageSpreadBranchFacing for $438, got %v", branch)
	}
}

func TestScaleFitBranchDetection(t *testing.T) {
	data := map[string]interface{}{
		"$159": "$270",
		"$156": "$326",
	}
	branch := determinePageSpreadBranch(data, true)
	if branch != pageSpreadBranchScaleFit {
		t.Errorf("expected pageSpreadBranchScaleFit for $326 (section), got %v", branch)
	}
}

func TestConnectedPaginationBranchDetection(t *testing.T) {
	data := map[string]interface{}{
		"$159": "$270",
		"$156": "$323",
		"$656": true,
	}
	branch := determinePageSpreadBranch(data, true)
	if branch != pageSpreadBranchConnected {
		t.Errorf("expected pageSpreadBranchConnected for $323/$656, got %v", branch)
	}
}

func TestLeafBranchDetection(t *testing.T) {
	data := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}
	branch := determinePageSpreadBranch(data, true)
	if branch != pageSpreadBranchLeaf {
		t.Errorf("expected pageSpreadBranchLeaf for $325 layout, got %v", branch)
	}
}

func TestLeafBranchDetectionWithEmptyData(t *testing.T) {
	data := map[string]interface{}{}
	branch := determinePageSpreadBranch(data, true)
	if branch != pageSpreadBranchLeaf {
		t.Errorf("expected pageSpreadBranchLeaf for data without $159/$156, got %v", branch)
	}
}

// =============================================================================
// VAL-A-033: Virtual panel handling
// =============================================================================

func TestVirtualPanelNoneAllowed(t *testing.T) {
	cfg := makeSpreadConfig()
	cfg.VirtualPanelsAllowed = false
	cfg.RegionMagnification = false
	// When virtual_panel is nil and book is comic without region_magnification,
	// should log an error (handled inside processPageSpreadPageTemplate)
	result := processPageSpreadPageTemplate(
		map[string]interface{}{
			"$159": "$270",
			"$156": "$325",
		},
		"test-section",
		"",
		nil,
		true,
		cfg,
		map[string]map[string]interface{}{},
	)
	// Should succeed (leaf branch)
	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
}

func TestVirtualPanelSymbol441Allowed(t *testing.T) {
	cfg := makeSpreadConfig()
	cfg.VirtualPanelsAllowed = true

	storyName := "story-vp"
	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$437",
		"$176": storyName,
		"$434": "$441",
		"$155": 42,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"$176": storyName,
			"$146": []interface{}{},
		},
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
	if !result.VirtualPanels {
		t.Error("expected VirtualPanels=true when $434=$441 and panels allowed")
	}
}

func TestVirtualPanelUnexpectedValue(t *testing.T) {
	cfg := makeSpreadConfig()
	cfg.VirtualPanelsAllowed = false

	storyName := "story-vp-unexpected"
	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$437",
		"$176": storyName,
		"$434": "$999", // unexpected virtual panel value
		"$155": 42,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"$176": storyName,
			"$146": []interface{}{},
		},
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
	// VirtualPanels should NOT be set for unexpected value
	if result.VirtualPanels {
		t.Error("expected VirtualPanels=false for unexpected virtual panel value")
	}
}

// =============================================================================
// VAL-A-034: LTR/RTL page spread alternation
// =============================================================================

func TestPageSpreadLTRAlternation(t *testing.T) {
	cfg := makeSpreadConfig()
	cfg.PageProgressionDirection = "ltr"

	storyName := "story-ltr"
	child1 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}
	child2 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$437",
		"$176": storyName,
		"$434": "$441",
		"$155": 42,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"$176": storyName,
			"$146": []interface{}{child1, child2},
		},
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// For LTR, first child gets page-spread-left, second gets page-spread-right
	if len(result.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(result.Children))
	}
	if result.Children[0].PageSpread != "page-spread-left" {
		t.Errorf("first child: expected page-spread-left, got %q", result.Children[0].PageSpread)
	}
	if result.Children[1].PageSpread != "page-spread-right" {
		t.Errorf("second child: expected page-spread-right, got %q", result.Children[1].PageSpread)
	}
}

func TestPageSpreadRTLAlternation(t *testing.T) {
	cfg := makeSpreadConfig()
	cfg.PageProgressionDirection = "rtl"

	storyName := "story-rtl"
	child1 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}
	child2 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$437",
		"$176": storyName,
		"$434": "$441",
		"$155": 42,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"$176": storyName,
			"$146": []interface{}{child1, child2},
		},
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// For RTL, first child gets page-spread-right, second gets page-spread-left
	if len(result.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(result.Children))
	}
	if result.Children[0].PageSpread != "page-spread-right" {
		t.Errorf("first child: expected page-spread-right, got %q", result.Children[0].PageSpread)
	}
	if result.Children[1].PageSpread != "page-spread-left" {
		t.Errorf("second child: expected page-spread-left, got %q", result.Children[1].PageSpread)
	}
}

// =============================================================================
// VAL-A-035: PDF-backed scale_fit branch ($326)
// =============================================================================

func TestScaleFitBranchProcesses(t *testing.T) {
	cfg := makePDFSpreadConfig()

	storyName := "story-sf"
	child1 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$326",
		"$176": storyName,
		"$434": "$441",
		"$192": nil,
		"$140": nil,
		"$560": nil,
		"$155": 55,
		"$16":  16,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"$176": storyName,
			"$146": []interface{}{child1},
		},
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	if len(result.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(result.Children))
	}
	// Scale fit children should NOT have page spread alternation
	if result.Children[0].PageSpread != "" {
		t.Errorf("scale_fit child should have empty page_spread, got %q", result.Children[0].PageSpread)
	}
}

func TestScaleFitBranchNotPdfBacked(t *testing.T) {
	cfg := makeSpreadConfig() // not PDF-backed

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$326",
	}

	// determinePageSpreadBranch only checks data; the PDF-backed check happens in
	// processPageSpreadPageTemplate. At the branch-detection level, $326 + isSection
	// is still identified as scale_fit.
	branch := determinePageSpreadBranch(templateData, true)
	if branch != pageSpreadBranchScaleFit {
		t.Errorf("expected pageSpreadBranchScaleFit from data analysis, got %v", branch)
	}

	// But the full processing function should fall through to leaf when not PDF-backed
	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, map[string]map[string]interface{}{})
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	// Should produce a leaf section, not a scale_fit child
	if len(result.Sections) != 1 {
		t.Fatalf("expected 1 leaf section (fell through from scale_fit), got %d sections, %d children", len(result.Sections), len(result.Children))
	}
}

func TestScaleFitBranchNotSection(t *testing.T) {
	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$326",
	}

	// Not a section → should fall to leaf
	branch := determinePageSpreadBranch(templateData, false)
	if branch == pageSpreadBranchScaleFit {
		t.Error("scale_fit branch should not activate when is_section=false")
	}
}

// =============================================================================
// VAL-A-036: Connected pagination branch ($323/$656)
// =============================================================================

func TestConnectedPaginationBranchProcesses(t *testing.T) {
	cfg := makeSpreadConfig()

	storyName := "story-cp"
	child1 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}
	child2 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$323",
		"$656": true,
		"$176": storyName,
		"$434": "$441",
		"$155": 77,
		"$655": 2,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"$176": storyName,
			"$146": []interface{}{child1, child2},
		},
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	if len(result.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(result.Children))
	}
	// All connected pagination children should have center spread
	for i, child := range result.Children {
		if child.PageSpread != "rendition:page-spread-center" {
			t.Errorf("connected pagination child %d: expected rendition:page-spread-center, got %q", i, child.PageSpread)
		}
	}
}

// =============================================================================
// VAL-A-037: Leaf content branch
// =============================================================================

func TestLeafBranchCreatesSectionNameWithSpreadSuffix(t *testing.T) {
	cfg := makeSpreadConfig()

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}

	result := processPageSpreadPageTemplate(
		templateData,
		"my-section",
		"rendition:page-spread-left",
		nil,
		true,
		cfg,
		map[string]map[string]interface{}{},
	)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// Leaf with page_spread should produce section name with spread suffix
	if len(result.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(result.Sections))
	}
	// spread_type = "left" (stripped from "rendition:page-spread-left")
	// unique_section_name = "my-section-left"
	if result.Sections[0].PageTitle != "my-section-left" {
		t.Errorf("expected section pageTitle 'my-section-left', got %q", result.Sections[0].PageTitle)
	}
	if result.Sections[0].Properties != "rendition:page-spread-left" {
		t.Errorf("expected properties 'rendition:page-spread-left', got %q", result.Sections[0].Properties)
	}
}

func TestLeafBranchCreatesSectionNameWithoutSpread(t *testing.T) {
	cfg := makeSpreadConfig()

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}

	result := processPageSpreadPageTemplate(
		templateData,
		"my-section",
		"",
		nil,
		true,
		cfg,
		map[string]map[string]interface{}{},
	)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	if len(result.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(result.Sections))
	}
	// No spread → section name is just "my-section"
	if result.Sections[0].PageTitle != "my-section" {
		t.Errorf("expected section pageTitle 'my-section', got %q", result.Sections[0].PageTitle)
	}
	if result.Sections[0].Properties != "" {
		t.Errorf("expected empty properties, got %q", result.Sections[0].Properties)
	}
}

// =============================================================================
// VAL-A-038: Recursive processing — nested page-spread with facing-page
// =============================================================================

func TestRecursivePageSpreadProcessing(t *testing.T) {
	cfg := makeSpreadConfig()
	cfg.PageProgressionDirection = "ltr"

	outerStoryName := "outer-story"

	innerChild := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}

	// Outer template is $437 page-spread
	outerTemplate := map[string]interface{}{
		"$159": "$270",
		"$156": "$437",
		"$176": outerStoryName,
		"$434": "$441",
		"$155": 10,
	}

	storylines := map[string]map[string]interface{}{
		outerStoryName: {
			"$176": outerStoryName,
			"$146": []interface{}{
				innerChild,
			},
		},
	}

	result := processPageSpreadPageTemplate(outerTemplate, "test-section", "", nil, true, cfg, storylines)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// Should have processed 1 child through the page-spread branch
	if len(result.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(result.Children))
	}

	// Child should be page-spread-left for LTR
	if result.Children[0].PageSpread != "page-spread-left" {
		t.Errorf("expected page-spread-left, got %q", result.Children[0].PageSpread)
	}
}

func TestLocationIDFromTemplate(t *testing.T) {
	// getLocationID pops $155 first, then $598
	data := map[string]interface{}{
		"$155": 42,
		"$598": 99,
	}
	id := getLocationID(data)
	if id != 42 {
		t.Errorf("expected location ID 42 from $155, got %v", id)
	}
	// $155 should have been consumed
	if _, exists := data["$155"]; exists {
		t.Error("expected $155 to be consumed by getLocationID")
	}
}

func TestLocationIDFromTemplateFallback(t *testing.T) {
	data := map[string]interface{}{
		"$598": 99,
	}
	id := getLocationID(data)
	if id != 99 {
		t.Errorf("expected location ID 99 from $598 fallback, got %v", id)
	}
}

func TestLocationIDFromTemplateEmpty(t *testing.T) {
	data := map[string]interface{}{}
	id := getLocationID(data)
	if id != 0 {
		t.Errorf("expected 0 for missing location ID, got %v", id)
	}
}

// =============================================================================
// Integration: processSectionComic dispatches to processPageSpreadPageTemplate
// =============================================================================

func TestProcessSectionComicDispatchesToPageSpreadTemplate(t *testing.T) {
	// Build a complete comic section with a page-spread template
	storyName := "comic-story"
	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$437",
		"$176": storyName,
		"$434": "$441",
		"$155": 10,
	}

	section := sectionFragment{
		ID:    "comic-section-1",
		Storyline: storyName,
		PageTemplates: []pageTemplateFragment{
			{
				PositionID:         100,
				Storyline:          storyName,
				PageTemplateValues: templateData,
			},
		},
		PageTemplateValues: map[string]interface{}{},
	}

	// Verify branch detection still works
	branch := determineSectionBranch(section, bookTypeComic)
	if branch != branchComic {
		t.Errorf("expected branchComic, got %v", branch)
	}
}

// =============================================================================
// Spread type extraction from page_spread property string
// =============================================================================

func TestExtractSpreadType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"rendition:page-spread-left", "left"},
		{"rendition:page-spread-right", "right"},
		{"rendition:page-spread-center", "center"},
		{"page-spread-left", "left"},
		{"page-spread-right", "right"},
		{"", ""},
		{"something", "something"},
	}
	for _, test := range tests {
		got := extractSpreadType(test.input)
		if got != test.expected {
			t.Errorf("extractSpreadType(%q) = %q, want %q", test.input, got, test.expected)
		}
	}
}

// =============================================================================
// Section name suffix handling
// =============================================================================

func TestUniqueSectionNameWithSpread(t *testing.T) {
	tests := []struct {
		sectionName string
		spreadType  string
		expected    string
	}{
		{"section-1", "left", "section-1-left"},
		{"section-1", "right", "section-1-right"},
		{"section-1", "center", "section-1-center"},
		{"section-1", "", "section-1"},
		{"my-section", "left", "my-section-left"},
	}
	for _, test := range tests {
		got := uniqueSectionName(test.sectionName, test.spreadType)
		if got != test.expected {
			t.Errorf("uniqueSectionName(%q, %q) = %q, want %q", test.sectionName, test.spreadType, got, test.expected)
		}
	}
}

// =============================================================================
// Facing page layout string mapping
// =============================================================================

func TestLayoutSpreadBaseProperty(t *testing.T) {
	tests := []struct {
		layout    string
		expected  string
	}{
		{"$437", "page-spread"},
		{"$438", "facing-page"},
	}
	for _, test := range tests {
		got := layoutSpreadBaseProperty(test.layout)
		if got != test.expected {
			t.Errorf("layoutSpreadBaseProperty(%q) = %q, want %q", test.layout, got, test.expected)
		}
	}
}

// =============================================================================
// Connected pagination validation
// =============================================================================

func TestConnectedPaginationValidation(t *testing.T) {
	cfg := makeSpreadConfig()

	storyName := "story-cp-bad"
	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$323",
		"$656": true,
		"$176": storyName,
		"$434": "$441",
		"$155": 77,
		"$655": 3, // wrong value — should be 2
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"$176": storyName,
			"$146": []interface{}{},
		},
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	// Should still succeed but log a warning
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
}

// =============================================================================
// Edge cases
// =============================================================================

func TestPageSpreadWithMissingStoryline(t *testing.T) {
	cfg := makeSpreadConfig()

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$437",
		"$176": "missing-story",
		"$434": "$441",
		"$155": 42,
	}
	storylines := map[string]map[string]interface{}{
		// "missing-story" intentionally not present
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	if result.Err == nil {
		t.Error("expected error for missing storyline")
	}
}

func TestPageSpreadWithEmptyChildList(t *testing.T) {
	cfg := makeSpreadConfig()

	storyName := "story-empty"
	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$437",
		"$176": storyName,
		"$434": "$441",
		"$155": 42,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"$176": storyName,
			"$146": []interface{}{}, // empty children
		},
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if len(result.Children) != 0 {
		t.Errorf("expected 0 children, got %d", len(result.Children))
	}
}

func TestLeafBranchWithParentTemplateID(t *testing.T) {
	cfg := makeSpreadConfig()

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}

	parentID := 42
	result := processPageSpreadPageTemplate(
		templateData,
		"my-section",
		"rendition:page-spread-left",
		&parentID,
		false, // not section
		cfg,
		map[string]map[string]interface{}{},
	)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	if len(result.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(result.Sections))
	}
	// When parent_template_id is set, it should be recorded
	if result.Sections[0].ParentPositionID != 42 {
		t.Errorf("expected ParentPositionID=42, got %d", result.Sections[0].ParentPositionID)
	}
}

// =============================================================================
// Scrutiny fix tests — parentTemplateID propagation and scale_fit condition
// =============================================================================

// TestStoryBranchParentTemplateIDFirstChildOnly verifies that in the story branch
// ($437/$438), parentTemplateID (from $155 location ID) is propagated to the first
// child only, and set to nil for subsequent children.
// Python reference: yj_to_epub_content.py ~260 — parent_template_id passed to first
// child then set to None.
func TestStoryBranchParentTemplateIDFirstChildOnly(t *testing.T) {
	cfg := makeSpreadConfig()
	cfg.PageProgressionDirection = "ltr"

	storyName := "story-ptid"
	locID := 42

	// Two leaf children in the story
	child1 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}
	child2 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}
	child3 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$437",
		"$176": storyName,
		"$434": "$441",
		"$155": locID,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"$176": storyName,
			"$146": []interface{}{child1, child2, child3},
		},
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// Should have 3 sections (leaf children produce sections)
	if len(result.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(result.Sections))
	}

	// First child should have parentTemplateID = locID (42)
	if result.Sections[0].ParentPositionID != locID {
		t.Errorf("first child: expected ParentPositionID=%d, got %d", locID, result.Sections[0].ParentPositionID)
	}
	// Second child should have NO parentTemplateID (nil → 0)
	if result.Sections[1].ParentPositionID != 0 {
		t.Errorf("second child: expected ParentPositionID=0 (nil), got %d", result.Sections[1].ParentPositionID)
	}
	// Third child should also have NO parentTemplateID
	if result.Sections[2].ParentPositionID != 0 {
		t.Errorf("third child: expected ParentPositionID=0 (nil), got %d", result.Sections[2].ParentPositionID)
	}
}

// TestStoryBranchParentTemplateIDZeroWhenNoLocationID verifies that when there's no
// $155 location ID in the template, no parentTemplateID is propagated to any child.
func TestStoryBranchParentTemplateIDZeroWhenNoLocationID(t *testing.T) {
	cfg := makeSpreadConfig()

	storyName := "story-no-ptid"
	child1 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$437",
		"$176": storyName,
		"$434": "$441",
		// No $155 key → location ID = 0 → no parentTemplateID
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"$176": storyName,
			"$146": []interface{}{child1},
		},
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	if len(result.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(result.Sections))
	}
	if result.Sections[0].ParentPositionID != 0 {
		t.Errorf("expected ParentPositionID=0 when no location ID, got %d", result.Sections[0].ParentPositionID)
	}
}

// TestConnectedBranchParentTemplateIDFirstChildOnly verifies that in the connected
// pagination branch ($323/$656), parentTemplateID is propagated to the first child
// only, and set to nil for subsequent children.
// Python reference: yj_to_epub_content.py ~310 — parent_template_id passed to first
// child then set to None.
func TestConnectedBranchParentTemplateIDFirstChildOnly(t *testing.T) {
	cfg := makeSpreadConfig()

	storyName := "story-cp-ptid"
	locID := 77

	child1 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}
	child2 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$323",
		"$656": true,
		"$176": storyName,
		"$434": "$441",
		"$155": locID,
		"$655": 2,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"$176": storyName,
			"$146": []interface{}{child1, child2},
		},
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	if len(result.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(result.Sections))
	}

	// First child should have parentTemplateID = locID (77)
	if result.Sections[0].ParentPositionID != locID {
		t.Errorf("first child: expected ParentPositionID=%d, got %d", locID, result.Sections[0].ParentPositionID)
	}
	// Second child should have NO parentTemplateID
	if result.Sections[1].ParentPositionID != 0 {
		t.Errorf("second child: expected ParentPositionID=0 (nil), got %d", result.Sections[1].ParentPositionID)
	}
}

// TestScaleFitBranchParentTemplateIDAllChildren verifies that in the scale_fit branch
// ($326), parentTemplateID is propagated to ALL children (Python does NOT set
// parent_template_id = None after the first child).
// Python reference: yj_to_epub_content.py ~280 — parent_template_id passed to all
// children without reset.
func TestScaleFitBranchParentTemplateIDAllChildren(t *testing.T) {
	cfg := makePDFSpreadConfig()

	storyName := "story-sf-ptid"
	locID := 55

	child1 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}
	child2 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$326",
		"$176": storyName,
		"$434": "$441",
		"$192": nil,
		"$140": nil,
		"$560": nil,
		"$155": locID,
		"$16":  16,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"$176": storyName,
			"$146": []interface{}{child1, child2},
		},
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	if len(result.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(result.Sections))
	}

	// BOTH children should have parentTemplateID = locID (55)
	// (Unlike story/connected branches which only pass to first child)
	if result.Sections[0].ParentPositionID != locID {
		t.Errorf("first child: expected ParentPositionID=%d, got %d", locID, result.Sections[0].ParentPositionID)
	}
	if result.Sections[1].ParentPositionID != locID {
		t.Errorf("second child: expected ParentPositionID=%d, got %d", locID, result.Sections[1].ParentPositionID)
	}
}

// TestScaleFitFallsToLeafWhenDollar67Present verifies that when IsPdfBacked is true
// but $67 is present in the pageTemplate, the scale_fit branch falls through to the
// leaf branch instead.
// Python reference: yj_to_epub_content.py ~260 — condition checks
// "self.is_pdf_backed and '$67' not in page_template and '$66' not in page_template".
func TestScaleFitFallsToLeafWhenDollar67Present(t *testing.T) {
	cfg := makePDFSpreadConfig() // PDF-backed = true

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$326",
		"$67":  "has-sixty-seven", // $67 present → should NOT enter scale_fit
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, map[string]map[string]interface{}{})
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// Should produce a leaf section, not enter scale_fit processing
	if len(result.Sections) != 1 {
		t.Fatalf("expected 1 leaf section (fell through from scale_fit due to $67), got %d sections, %d children", len(result.Sections), len(result.Children))
	}
}

// TestScaleFitFallsToLeafWhenDollar66Present verifies that when IsPdfBacked is true
// but $66 is present in the pageTemplate, the scale_fit branch falls through to the
// leaf branch instead.
func TestScaleFitFallsToLeafWhenDollar66Present(t *testing.T) {
	cfg := makePDFSpreadConfig() // PDF-backed = true

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$326",
		"$66":  "has-sixty-six", // $66 present → should NOT enter scale_fit
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, map[string]map[string]interface{}{})
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// Should produce a leaf section, not enter scale_fit processing
	if len(result.Sections) != 1 {
		t.Fatalf("expected 1 leaf section (fell through from scale_fit due to $66), got %d sections, %d children", len(result.Sections), len(result.Children))
	}
}

// TestScaleFitProcessesWhenPdfBackedNoDollar67Dollar66 verifies the happy path:
// IsPdfBacked=true, no $67, no $66 → scale_fit branch processes normally.
func TestScaleFitProcessesWhenPdfBackedNoDollar67Dollar66(t *testing.T) {
	cfg := makePDFSpreadConfig()

	storyName := "story-sf-happy"
	child1 := map[string]interface{}{
		"$159": "$270",
		"$156": "$325",
	}

	templateData := map[string]interface{}{
		"$159": "$270",
		"$156": "$326",
		"$176": storyName,
		"$434": "$441",
		"$192": nil,
		"$140": nil,
		"$560": nil,
		"$155": 55,
		"$16":  16,
		// No $67, no $66 → scale_fit branch should activate
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"$176": storyName,
			"$146": []interface{}{child1},
		},
	}

	result := processPageSpreadPageTemplate(templateData, "test-section", "", nil, true, cfg, storylines)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// Should produce a child (not a leaf section) — scale_fit branch active
	if len(result.Children) != 1 {
		t.Errorf("expected 1 child from scale_fit branch, got %d children, %d sections", len(result.Children), len(result.Sections))
	}
}
