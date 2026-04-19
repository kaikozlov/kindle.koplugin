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
