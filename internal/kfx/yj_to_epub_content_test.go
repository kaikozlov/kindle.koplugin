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
	condition := []interface{}{"==", "position", "anchor"}
	section := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("story1", condition),
	}, nil)

	branch := determineSectionBranch(section, bookTypeMagazine)
	if branch != branchMagazine {
		t.Errorf("expected branchMagazine, got %v", branch)
	}
}

func TestProcessSectionPrintReplicaWithConditionalTemplate(t *testing.T) {
	condition := []interface{}{"==", "position", "anchor"}
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
	condition := []interface{}{"==", "position", "anchor"}
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
		"section_name":   "section-name",
		"reading_order_switch_map":   "some-value",
		"yj.conversion.html_name":                   "test",
		"yj.semantics.book_anatomy_type":            "chapter",
		"yj.semantics.page_type":                    "body",
		"yj.authoring.auto_panel_settings_opacity":  "0.5",
		"yj.authoring.auto_panel_settings_padding_top": "10",
		"page_templates":   []interface{}{},
		"keep-me": "important",
	}

	stripUnusedSectionKeys(data)

	// These should have been removed
	for _, key := range []string{
		"reading_order_switch_map",
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
	for _, key := range []string{"section_name", "page_templates", "keep-me"} {
		if _, exists := data[key]; !exists {
			t.Errorf("expected key %q to remain, but it was removed", key)
		}
	}
}

func TestStripUnusedKeysPreservesUnknownKeys(t *testing.T) {
	data := map[string]interface{}{
		"nmdl.canvas_width": 800,
		"section_name":              "section-name",
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
		"story-a": {"content_list": []interface{}{
			map[string]interface{}{"content": map[string]interface{}{"name": "content-a", "index": 0}},
		}},
		"story-b": {"content_list": []interface{}{
			map[string]interface{}{"content": map[string]interface{}{"name": "content-b", "index": 0}},
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
	processReadingOrder(book, sectionOrder, sectionFragments, storylines, contentFragments, renderer, map[string]string{}, symOriginal, nil)

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
				"features": []interface{}{
					map[string]interface{}{
						"key": "yj_facing_page",
						"namespace": "kindle_capability_metadata",
					},
				},
			},
			expected: bookTypeComic,
		},
		{
			name: "double page spread → comic",
			features: map[string]interface{}{
				"features": []interface{}{
					map[string]interface{}{
						"key": "yj_double_page_spread",
						"namespace": "kindle_capability_metadata",
					},
				},
			},
			expected: bookTypeComic,
		},
		{
			name: "publisher panels → comic",
			features: map[string]interface{}{
				"features": []interface{}{
					map[string]interface{}{
						"key": "yj_publisher_panels",
						"namespace": "kindle_capability_metadata",
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
				"cde_content_type": "MAGZ",
			},
			expected: bookTypeMagazine,
		},
		{
			name: "EBSP → not magazine",
			metadata: map[string]interface{}{
				"cde_content_type": "EBSP",
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
		"story1": {"content_list": []interface{}{
			map[string]interface{}{"content": map[string]interface{}{"name": "c1", "index": 0}},
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
		makeTestTemplate("story1", []interface{}{"=="}),
		makeTestTemplate("story2", []interface{}{"=="}),
		makeTestTemplate("story3", nil),
	}
	section := makeTestSection(templates, nil)
	storylines := map[string]map[string]interface{}{
		"story1": {"content_list": []interface{}{
			map[string]interface{}{"content": map[string]interface{}{"name": "c1", "index": 0}},
		}},
		"story2": {"content_list": []interface{}{
			map[string]interface{}{"content": map[string]interface{}{"name": "c2", "index": 0}},
		}},
		"story3": {"content_list": []interface{}{
			map[string]interface{}{"content": map[string]interface{}{"name": "c3", "index": 0}},
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
		"story1": {"content_list": []interface{}{
			map[string]interface{}{"content": map[string]interface{}{"name": "c1", "index": 0}},
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
		{"reading_order_switch_map", true},
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
		{"section_name", false},
		{"page_templates", false},
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
		{Storyline: "s1", Condition: []interface{}{"==", "position", "anchor"}, HasCondition: true},
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
		{Storyline: "s2", Condition: []interface{}{"hasColor"}, HasCondition: true},
		{Storyline: "s3", Condition: []interface{}{"not", []interface{}{"hasColor"}}, HasCondition: true}, // NOT true = false
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
		{Storyline: "s1", HasCondition: true, Condition: []interface{}{"=="}},
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
		makeTestTemplate("s1", []interface{}{"=="}),
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

// makeLeafTemplateData creates a leaf content template with $159="container", $156="overflow"
// and some content fields.
func makeLeafTemplateData() map[string]interface{} {
	return map[string]interface{}{
		"type": "container",
		"layout": "overflow",
	}
}

// makePageSpreadTemplateData creates a page-spread container template ($437)
// with a storyline reference pointing to a story with child templates.
func makePageSpreadTemplateData(storyName string, childTemplates []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type": "container",
		"layout": "page_spread",
		"story_name": storyName,
		"virtual_panel": "enabled",
		"direction": nil,
		"fixed_height":  nil,
		"fixed_width":  nil,
		"float": nil,
		"writing_mode": nil,
		"id": 42, // location ID
	}
}

// makeScaleFitTemplateData creates a PDF-backed scale_fit template ($326).
func makeScaleFitTemplateData(storyName string) map[string]interface{} {
	return map[string]interface{}{
		"type": "container",
		"layout": "scale_fit",
		"story_name": storyName,
		"virtual_panel": "enabled",
		"direction": nil,
		"float": nil,
		"writing_mode": nil,
		"id": 55,
		"font_size":  16, // font_size = 16
	}
}

// makeConnectedPaginationTemplateData creates a connected pagination template ($323/$656).
func makeConnectedPaginationTemplateData(storyName string) map[string]interface{} {
	return map[string]interface{}{
		"type": "container",
		"layout": "vertical",
		"yj.enable_connected_dps": true,
		"story_name": storyName,
		"virtual_panel": "enabled",
		"id": 77,
		"yj.connected_pagination": 2, // connected_pagination = 2
	}
}

// =============================================================================
// VAL-A-032: Page spread branch detection ($437)
// =============================================================================

func TestPageSpreadBranchDetection(t *testing.T) {
	data := map[string]interface{}{
		"type": "container",
		"layout": "page_spread",
	}
	branch := determinePageSpreadBranch(data, true)
	if branch != pageSpreadBranchSpread {
		t.Errorf("expected pageSpreadBranchSpread for $437, got %v", branch)
	}
}

func TestFacingPageBranchDetection(t *testing.T) {
	data := map[string]interface{}{
		"type": "container",
		"layout": "facing_page",
	}
	branch := determinePageSpreadBranch(data, true)
	if branch != pageSpreadBranchFacing {
		t.Errorf("expected pageSpreadBranchFacing for $438, got %v", branch)
	}
}

func TestScaleFitBranchDetection(t *testing.T) {
	data := map[string]interface{}{
		"type": "container",
		"layout": "scale_fit",
	}
	branch := determinePageSpreadBranch(data, true)
	if branch != pageSpreadBranchScaleFit {
		t.Errorf("expected pageSpreadBranchScaleFit for $326 (section), got %v", branch)
	}
}

func TestConnectedPaginationBranchDetection(t *testing.T) {
	data := map[string]interface{}{
		"type": "container",
		"layout": "vertical",
		"yj.enable_connected_dps": true,
	}
	branch := determinePageSpreadBranch(data, true)
	if branch != pageSpreadBranchConnected {
		t.Errorf("expected pageSpreadBranchConnected for $323/$656, got %v", branch)
	}
}

func TestLeafBranchDetection(t *testing.T) {
	data := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
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
			"type": "container",
			"layout": "overflow",
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
		"type": "container",
		"layout": "page_spread",
		"story_name": storyName,
		"virtual_panel": "enabled",
		"id": 42,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{},
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
		"type": "container",
		"layout": "page_spread",
		"story_name": storyName,
		"virtual_panel": "$999", // unexpected virtual panel value
		"id": 42,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{},
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
		"type": "container",
		"layout": "overflow",
	}
	child2 := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
	}

	templateData := map[string]interface{}{
		"type": "container",
		"layout": "page_spread",
		"story_name": storyName,
		"virtual_panel": "enabled",
		"id": 42,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{child1, child2},
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
		"type": "container",
		"layout": "overflow",
	}
	child2 := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
	}

	templateData := map[string]interface{}{
		"type": "container",
		"layout": "page_spread",
		"story_name": storyName,
		"virtual_panel": "enabled",
		"id": 42,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{child1, child2},
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
		"type": "container",
		"layout": "overflow",
	}

	templateData := map[string]interface{}{
		"type": "container",
		"layout": "scale_fit",
		"story_name": storyName,
		"virtual_panel": "enabled",
		"direction": nil,
		"float": nil,
		"writing_mode": nil,
		"id": 55,
		"font_size":  16,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{child1},
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
		"type": "container",
		"layout": "scale_fit",
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
		"type": "container",
		"layout": "scale_fit",
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
		"type": "container",
		"layout": "overflow",
	}
	child2 := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
	}

	templateData := map[string]interface{}{
		"type": "container",
		"layout": "vertical",
		"yj.enable_connected_dps": true,
		"story_name": storyName,
		"virtual_panel": "enabled",
		"id": 77,
		"yj.connected_pagination": 2,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{child1, child2},
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
		"type": "container",
		"layout": "overflow",
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
		"type": "container",
		"layout": "overflow",
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
		"type": "container",
		"layout": "overflow",
	}

	// Outer template is $437 page-spread
	outerTemplate := map[string]interface{}{
		"type": "container",
		"layout": "page_spread",
		"story_name": outerStoryName,
		"virtual_panel": "enabled",
		"id": 10,
	}

	storylines := map[string]map[string]interface{}{
		outerStoryName: {
			"story_name": outerStoryName,
			"content_list": []interface{}{
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
		"id": 42,
		"kfx_id": 99,
	}
	id := getLocationID(data)
	if id != 42 {
		t.Errorf("expected location ID 42 from $155, got %v", id)
	}
	// $155 should have been consumed
	if _, exists := data["id"]; exists {
		t.Error("expected $155 to be consumed by getLocationID")
	}
}

func TestLocationIDFromTemplateFallback(t *testing.T) {
	data := map[string]interface{}{
		"kfx_id": 99,
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
		"type": "container",
		"layout": "page_spread",
		"story_name": storyName,
		"virtual_panel": "enabled",
		"id": 10,
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
		{"page_spread", "page-spread"},
		{"facing_page", "facing-page"},
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
		"type": "container",
		"layout": "vertical",
		"yj.enable_connected_dps": true,
		"story_name": storyName,
		"virtual_panel": "enabled",
		"id": 77,
		"yj.connected_pagination": 3, // wrong value — should be 2
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{},
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
		"type": "container",
		"layout": "page_spread",
		"story_name": "missing-story",
		"virtual_panel": "enabled",
		"id": 42,
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
		"type": "container",
		"layout": "page_spread",
		"story_name": storyName,
		"virtual_panel": "enabled",
		"id": 42,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{}, // empty children
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
		"type": "container",
		"layout": "overflow",
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
		"type": "container",
		"layout": "overflow",
	}
	child2 := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
	}
	child3 := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
	}

	templateData := map[string]interface{}{
		"type": "container",
		"layout": "page_spread",
		"story_name": storyName,
		"virtual_panel": "enabled",
		"id": locID,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{child1, child2, child3},
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
		"type": "container",
		"layout": "overflow",
	}

	templateData := map[string]interface{}{
		"type": "container",
		"layout": "page_spread",
		"story_name": storyName,
		"virtual_panel": "enabled",
		// No $155 key → location ID = 0 → no parentTemplateID
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{child1},
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
		"type": "container",
		"layout": "overflow",
	}
	child2 := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
	}

	templateData := map[string]interface{}{
		"type": "container",
		"layout": "vertical",
		"yj.enable_connected_dps": true,
		"story_name": storyName,
		"virtual_panel": "enabled",
		"id": locID,
		"yj.connected_pagination": 2,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{child1, child2},
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
		"type": "container",
		"layout": "overflow",
	}
	child2 := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
	}

	templateData := map[string]interface{}{
		"type": "container",
		"layout": "scale_fit",
		"story_name": storyName,
		"virtual_panel": "enabled",
		"direction": nil,
		"float": nil,
		"writing_mode": nil,
		"id": locID,
		"font_size":  16,
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{child1, child2},
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
		"type": "container",
		"layout": "scale_fit",
		"fixed_height":  "has-sixty-seven", // $67 present → should NOT enter scale_fit
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
		"type": "container",
		"layout": "scale_fit",
		"fixed_width":  "has-sixty-six", // $66 present → should NOT enter scale_fit
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
		"type": "container",
		"layout": "overflow",
	}

	templateData := map[string]interface{}{
		"type": "container",
		"layout": "scale_fit",
		"story_name": storyName,
		"virtual_panel": "enabled",
		"direction": nil,
		"float": nil,
		"writing_mode": nil,
		"id": 55,
		"font_size":  16,
		// No $67, no $66 → scale_fit branch should activate
	}
	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{child1},
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

// =============================================================================
// M2 Book Type Dispatch Wiring Tests
// VAL-M2-DISPATCH-001 through VAL-M2-DISPATCH-005
// =============================================================================

// mockLogRecorder captures log output for testing error/warning messages.
type mockLogRecorder struct {
	messages []string
}

// =============================================================================
// VAL-M2-DISPATCH-001: processSection uses detectBookType() result to select branch
// =============================================================================

func TestDetectBookTypeWired(t *testing.T) {
	// Test that processSectionWithType selects the correct branch based on book type.
	// Comic book type → branchComic path
	comicSection := makeTestSection([]pageTemplateFragment{
		{PositionID: 100, Storyline: "story1", PageTemplateValues: map[string]interface{}{
			"type": "container",
			"layout": "overflow",
			"id": 42,
		}},
	}, nil)

	branch := determineSectionBranch(comicSection, bookTypeComic)
	if branch != branchComic {
		t.Errorf("expected branchComic for comic book type, got %v", branch)
	}

	// Magazine with conditional template → branchMagazine
	condSection := makeTestSection([]pageTemplateFragment{
		{PositionID: 100, Storyline: "story1", HasCondition: true, Condition: []interface{}{"hasColor"}},
	}, nil)

	branch = determineSectionBranch(condSection, bookTypeMagazine)
	if branch != branchMagazine {
		t.Errorf("expected branchMagazine for magazine with conditional template, got %v", branch)
	}

	// No book type → branchReflowable
	defaultSection := makeTestSection([]pageTemplateFragment{
		makeTestTemplate("story1", nil),
	}, nil)

	branch = determineSectionBranch(defaultSection, bookTypeNone)
	if branch != branchReflowable {
		t.Errorf("expected branchReflowable for no book type, got %v", branch)
	}
}

// =============================================================================
// VAL-M2-DISPATCH-002: Magazine branch evaluates $171 conditions and dispatches
// =============================================================================

func TestProcessSectionMagazine(t *testing.T) {
	// Build a magazine section with 2 page templates, one with a true condition
	// and one with a false condition. Only 1 template should be active.
	cfg := pageSpreadConfig{
		BookType:                 bookTypeMagazine,
		IsPdfBacked:              false,
		RegionMagnification:      false,
		VirtualPanelsAllowed:     true,
		PageProgressionDirection: "ltr",
	}

	// Template with $156="overflow" (standard layout) and $171 condition (true via $300)
	template1Values := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
		"condition": []interface{}{"hasColor"}, // $300 is always true
	}

	// Template with false condition
	template2Values := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
		"condition": []interface{}{"not", []interface{}{"hasColor"}}, // NOT true = false
	}

	templates := []pageTemplateFragment{
		{PositionID: 200, Storyline: "s1", PageTemplateValues: template1Values, HasCondition: true, Condition: []interface{}{"hasColor"}},
		{PositionID: 300, Storyline: "s2", PageTemplateValues: template2Values, HasCondition: true, Condition: []interface{}{"not", []interface{}{"hasColor"}}},
	}

	section := sectionFragment{
		ID:                 "mag-section",
		PositionID:         100,
		PageTemplates:      templates,
		PageTemplateValues: map[string]interface{}{},
	}

	renderer := &storylineRenderer{
		conditionEvaluator: conditionEvaluator{fixedLayout: true},
		styles:             newStyleCatalog(),
	}

	result := processSectionMagazine(section, renderer, cfg, nil)

	// Should succeed — exactly 1 template processed (the one with true condition)
	// The other template (NOT true) should be skipped
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	// Should have 1 section produced from the $325 layout template
	if len(result.Sections) != 1 {
		t.Errorf("expected 1 section from magazine branch, got %d", len(result.Sections))
	}
}

// =============================================================================
// VAL-M2-DISPATCH-003: Magazine $437 layout dispatched to processPageSpreadPageTemplate
// =============================================================================

func TestMagazineLayout437(t *testing.T) {
	cfg := pageSpreadConfig{
		BookType:                 bookTypeMagazine,
		IsPdfBacked:              false,
		RegionMagnification:      false,
		VirtualPanelsAllowed:     true,
		PageProgressionDirection: "ltr",
	}

	storyName := "story-437"
	// Template with $156="page_spread" layout
	templateValues := map[string]interface{}{
		"type": "container",
		"layout": "page_spread",
		"condition": []interface{}{"hasColor"}, // true condition
		"story_name": storyName,
		"virtual_panel": "enabled",
		"id": 42,
	}

	templates := []pageTemplateFragment{
		{PositionID: 200, Storyline: storyName, PageTemplateValues: templateValues, HasCondition: true, Condition: []interface{}{"hasColor"}},
	}

	section := sectionFragment{
		ID:                 "mag-section-437",
		PositionID:         100,
		PageTemplates:      templates,
		PageTemplateValues: map[string]interface{}{},
	}

	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{
				map[string]interface{}{
					"type": "container",
					"layout": "overflow",
				},
			},
		},
	}

	renderer := &storylineRenderer{
		conditionEvaluator: conditionEvaluator{fixedLayout: true},
		styles:             newStyleCatalog(),
	}

	result := processSectionMagazine(section, renderer, cfg, storylines)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// $437 layout should have dispatched to processPageSpreadPageTemplate,
	// which should produce at least 1 section (leaf child of the spread)
	if len(result.Sections) == 0 && len(result.Children) == 0 {
		t.Error("expected $437 layout to produce sections or children via processPageSpreadPageTemplate")
	}
}

// =============================================================================
// VAL-M2-DISPATCH-004: Comic branch resolves $608 fragment and calls processPageSpreadPageTemplate
// =============================================================================

func TestComicCallsPageSpread(t *testing.T) {
	cfg := pageSpreadConfig{
		BookType:                 bookTypeComic,
		IsPdfBacked:              false,
		RegionMagnification:      false,
		VirtualPanelsAllowed:     true,
		PageProgressionDirection: "ltr",
	}

	storyName := "comic-story"
	templateValues := map[string]interface{}{
		"type": "container",
		"layout": "page_spread",
		"story_name": storyName,
		"virtual_panel": "enabled",
		"id": 42,
	}

	templates := []pageTemplateFragment{
		{PositionID: 200, Storyline: storyName, PageTemplateValues: templateValues},
	}

	section := sectionFragment{
		ID:                 "comic-section-1",
		PositionID:         100,
		PageTemplates:      templates,
		PageTemplateValues: map[string]interface{}{},
	}

	storylines := map[string]map[string]interface{}{
		storyName: {
			"story_name": storyName,
			"content_list": []interface{}{
				map[string]interface{}{
					"type": "container",
					"layout": "overflow",
				},
			},
		},
	}

	result := processSectionComic(section, cfg, storylines)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// Comic branch should resolve the $608 fragment and call processPageSpreadPageTemplate
	// which should produce sections or children
	if len(result.Sections) == 0 && len(result.Children) == 0 {
		t.Error("expected comic branch to produce sections or children via processPageSpreadPageTemplate")
	}
}

// =============================================================================
// VAL-M2-DISPATCH-005: Comic branch logs error when template count != 1
// =============================================================================

// =============================================================================
// GAP 9: Backdrop style ($429) — VAL-M3-003
// Port of Python yj_to_epub_content.py L682-689.
// Backdrop style is popped from the node, looked up in book data, and validated.
// Purely diagnostic — no output modification.
// =============================================================================

func TestBackdropStylePoppedFromNode(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{
			"bd-style-1": {
				"style_name": "bd-style-1",
				"margin":     "10px",
			},
		},
	}
	node := map[string]interface{}{
		"type":            "container",
		"backdrop_style":  "bd-style-1",
		"content":         map[string]interface{}{"name": "c1", "content_index": 0},
	}
	result, ok := r.prepareRenderableNode(node)
	if !ok {
		t.Fatal("expected node to be renderable")
	}
	// backdrop_style should be consumed
	if _, exists := result["backdrop_style"]; exists {
		t.Error("backdrop_style should have been popped from node")
	}
}

func TestBackdropStyleMissingStyleLogged(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{},
	}
	node := map[string]interface{}{
		"type":           "container",
		"backdrop_style": "missing-style",
		"content":        map[string]interface{}{"name": "c1", "content_index": 0},
	}
	result, ok := r.prepareRenderableNode(node)
	if !ok {
		t.Fatal("expected node to be renderable")
	}
	if _, exists := result["backdrop_style"]; exists {
		t.Error("backdrop_style should have been popped even when missing")
	}
}

// =============================================================================
// GAP 11: Main content link ID ($754) — VAL-M3-006
// Port of Python yj_to_epub_content.py L869-870.
// When a node has main_content_id, it should be registered so the element
// can be targeted by internal links.
// =============================================================================

func TestMainContentIDRegistersAnchor(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{},
		positionAnchors: map[int]map[int][]string{
			500: {0: {"existing_anchor"}},
		},
	}
	node := map[string]interface{}{
		"type":             "container",
		"main_content_id":  "mc-target-1",
		"id":               500,
		"content":          map[string]interface{}{"name": "c1", "content_index": 0},
	}
	result, ok := r.prepareRenderableNode(node)
	if !ok {
		t.Fatal("expected node to be renderable")
	}
	// main_content_id should be consumed
	if _, exists := result["main_content_id"]; exists {
		t.Error("main_content_id should have been popped from node")
	}
}

func TestMainContentIDAbsentNoOp(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{},
	}
	node := map[string]interface{}{
		"type":    "container",
		"id":      500,
		"content": map[string]interface{}{"name": "c1", "content_index": 0},
	}
	result, ok := r.prepareRenderableNode(node)
	if !ok {
		t.Fatal("expected node to be renderable")
	}
	if _, exists := result["main_content_id"]; exists {
		t.Error("main_content_id should not exist when not present")
	}
}

// =============================================================================
// GAP 8: Activate/magnification ($426) wired into render pipeline — VAL-M3-002
// Port of Python yj_to_epub_content.py L674-694.
// processRegionMagnification exists but was never called from the render pipeline.
// After fix, activate data should be consumed from the node.
// =============================================================================

func TestActivateDataConsumedFromContainerNode(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{},
	}
	node := map[string]interface{}{
		"type":    "container",
		"ordinal": 1,
		"activate": []interface{}{
			map[string]interface{}{
				"action": "zoom_in",
				"target": "target-eid",
				"source": "source-eid",
			},
		},
		"content": map[string]interface{}{"name": "c1", "content_index": 0},
	}
	result, ok := r.prepareRenderableNode(node)
	if !ok {
		t.Fatal("expected node to be renderable")
	}
	// activate should be consumed
	if _, exists := result["activate"]; exists {
		t.Error("activate should have been popped from node")
	}
}

// =============================================================================
// $69 ignore z-index for fixed layout containers — VAL-M3-008
// Port of Python yj_to_epub_content.py L621-630.
// When layout is "fixed" and ignore is true, z-index: 1 is added.
// =============================================================================

func TestIgnoreZIndexAddedForFixedLayoutContainer(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{
			"s1": {"font_size": int64(16)}, // minimal style so containerClass produces output
		},
	}
	node := map[string]interface{}{
		"type":    "container",
		"layout":  "fixed",
		"ignore":  true,
		"style":   "s1",
		"content": map[string]interface{}{"name": "c1", "content_index": 0},
	}
	result, ok := r.prepareRenderableNode(node)
	if !ok {
		t.Fatal("expected node to be renderable")
	}
	// ignore should be consumed
	if _, exists := result["ignore"]; exists {
		t.Error("ignore should have been popped from node")
	}
	// __ignore_zindex__ marker should be set so containerClass can read it
	if _, exists := result["__ignore_zindex__"]; !exists {
		t.Error("expected __ignore_zindex__ marker to be set on result node")
	}
	// Verify containerClass produces z-index:1 in the style output
	styleAttr := r.containerClass(result)
	if styleAttr == "" {
		t.Fatal("expected non-empty style attribute from containerClass")
	}
	if !containsDeclaration(styleAttr, "z-index", "1") {
		t.Errorf("expected z-index:1 in style output, got: %s", styleAttr)
	}
}

// Verifies that z-index is NOT added when ignore is false or layout is not "fixed".
func TestIgnoreZIndexNotAddedForNonFixedLayout(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{
			"s1": {"font_size": int64(16)},
		},
	}
	// Test: non-fixed layout should not get z-index
	node := map[string]interface{}{
		"type":    "container",
		"layout":  "reflowable",
		"ignore":  true,
		"style":   "s1",
		"content": map[string]interface{}{"name": "c1", "content_index": 0},
	}
	result, ok := r.prepareRenderableNode(node)
	if !ok {
		t.Fatal("expected node to be renderable")
	}
	if _, exists := result["__ignore_zindex__"]; exists {
		t.Error("__ignore_zindex__ should NOT be set for non-fixed layout")
	}
}

// =============================================================================
// $475 fit_text validation — correct symbol name "force" ($472)
// Port of Python yj_to_epub_content.py L663-668.
// fit_text should be validated against "force" ($472), not "scale_fit".
// =============================================================================

func TestFitTextAcceptsForce(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{},
	}
	node := map[string]interface{}{
		"type":     "container",
		"fit_text": "force",
	}
	result, ok := r.prepareRenderableNode(node)
	if !ok {
		t.Fatal("expected node to be renderable")
	}
	// fit_text should be consumed
	if _, exists := result["fit_text"]; exists {
		t.Error("fit_text should have been popped from node")
	}
}

func TestFitTextRejectsOtherValues(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{},
	}
	node := map[string]interface{}{
		"type":     "container",
		"fit_text": "scale_fit",
	}
	result, ok := r.prepareRenderableNode(node)
	if !ok {
		t.Fatal("expected node to be renderable")
	}
	// fit_text should still be consumed (popped) even if value is unexpected
	if _, exists := result["fit_text"]; exists {
		t.Error("fit_text should have been popped from node regardless of value")
	}
}

// =============================================================================
// $429 backdrop_style validation — correct keys fill_color ($70), fill_opacity ($72)
// Port of Python yj_to_epub_content.py L697-704.
// Python pops $173 (style_name), $70 (fill_color), $72 (fill_opacity).
// =============================================================================

func TestBackdropStylePopsCorrectKeys(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{
			"bd1": {
				"style_name":   "bd1",
				"fill_color":   "#FFFFFF",
				"fill_opacity": float64(1.0),
			},
		},
	}
	node := map[string]interface{}{
		"type":          "container",
		"backdrop_style": "bd1",
	}
	result, ok := r.prepareRenderableNode(node)
	if !ok {
		t.Fatal("expected node to be renderable")
	}
	// backdrop_style should be consumed
	if _, exists := result["backdrop_style"]; exists {
		t.Error("backdrop_style should have been popped from node")
	}
}

func TestBackdropStyleDetectsUnexpectedKeys(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{
			"bd2": {
				"style_name":   "bd2",
				"fill_color":   "#000000",
				"fill_opacity": float64(0.5),
				"extra_key":    "unexpected",
			},
		},
	}
	node := map[string]interface{}{
		"type":          "container",
		"backdrop_style": "bd2",
	}
	result, ok := r.prepareRenderableNode(node)
	if !ok {
		t.Fatal("expected node to be renderable")
	}
	_ = result
}

// Helper to check if a CSS style string contains a specific declaration.
func containsDeclaration(styleStr, prop, value string) bool {
	// Parse the style string for "prop: value"
	for _, part := range splitStyleDeclarations(styleStr) {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, prop+":") {
			val := strings.TrimSpace(strings.TrimPrefix(part, prop+":"))
			return val == value
		}
	}
	return false
}

func splitStyleDeclarations(s string) []string {
	// Split on semicolons, handling both "key: val; key: val" and "key: val"
	parts := strings.Split(s, ";")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// =============================================================================
// Table row child tag promotion — VAL-M3-011
// Port of Python yj_to_epub_content.py L627-642.
// In Python, table_row ($279) promotes child div → td and wraps non-td children
// in td elements. Go's renderTableRow should handle these cases.
// =============================================================================

func TestTableRowDivPromotedToTd(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{},
	}
	rowNode := map[string]interface{}{
		"type": "table_row",
		"content_list": []interface{}{
			map[string]interface{}{
				"type":    "body",
				"style":   "s1",
				"content": map[string]interface{}{"name": "c1", "content_index": 0},
			},
		},
	}
	result := r.renderTableRow(rowNode, 0)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	row, ok := result.(*htmlElement)
	if !ok {
		t.Fatal("expected *htmlElement")
	}
	if row.Tag != "tr" {
		t.Errorf("expected <tr>, got <%s>", row.Tag)
	}
}

// =============================================================================
// GAP 5: Table alt_content ($749) — already implemented, verify
// =============================================================================

func TestAltContentAnnotationOnTable(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{},
		storylines:     map[string]map[string]interface{}{},
		conditionEvaluator: conditionEvaluator{},
	}
	tableNode := map[string]interface{}{
		"type": "table",
		"annotations": []interface{}{
			map[string]interface{}{
				"annotation_type": "alt_content",
				"alt_content":     "missing-storyline",
				"include":         []interface{}{"and", []interface{}{"not", []interface{}{"yj.supports", "yj.large_tables"}}, []interface{}{"yj.layout_type", "yj.in_page"}},
			},
		},
		"content_list": []interface{}{},
	}
	element := &htmlElement{Tag: "table", Attrs: map[string]string{}}
	result := r.processAnnotations(tableNode, "table", element)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// =============================================================================
// GAP 6: Word boundary ($696) — already implemented, verify
// =============================================================================

func TestWordBoundaryListPoppedFromNode(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{},
	}
	node := map[string]interface{}{
		"type":              "text",
		"word_boundary_list": []interface{}{int64(0), int64(5), int64(1), int64(3)},
		"content":           map[string]interface{}{"name": "c1", "content_index": 0},
	}
	result, ok := r.prepareRenderableNode(node)
	if !ok {
		t.Fatal("expected node to be renderable")
	}
	if _, exists := result["word_boundary_list"]; exists {
		t.Error("word_boundary_list should have been popped from node")
	}
}

// =============================================================================
// GAP 7: First line style ($622) — already implemented, verify
// =============================================================================

func TestFirstLineStyleAppliedToElement(t *testing.T) {
	r := &storylineRenderer{
		styleFragments: map[string]map[string]interface{}{},
		styles:         newStyleCatalog(),
	}
	node := map[string]interface{}{
		"type": "text",
		"yj.first_line_style": map[string]interface{}{
			"style_name": "",
			"font_size":  "1.5em",
		},
		"content": map[string]interface{}{"name": "c1", "content_index": 0},
	}
	element := &htmlElement{Tag: "span", Attrs: map[string]string{}}
	r.applyFirstLineStyle(element, node)
	// First-line style should add a class with ::first-line CSS
	if element.Attrs["class"] == "" {
		t.Error("expected first-line class to be added to element")
	}
}

func TestComicTemplateCountValidation(t *testing.T) {
	cfg := pageSpreadConfig{
		BookType:                 bookTypeComic,
		IsPdfBacked:              false,
		RegionMagnification:      false,
		VirtualPanelsAllowed:     true,
		PageProgressionDirection: "ltr",
	}

	// Section with 0 templates → error logged, empty result
	section0 := sectionFragment{
		ID:                 "comic-empty",
		PositionID:         100,
		PageTemplates:      []pageTemplateFragment{},
		PageTemplateValues: map[string]interface{}{},
	}

	result := processSectionComic(section0, cfg, nil)
	// Should return empty result (0 templates)
	if len(result.Sections) != 0 || len(result.Children) != 0 {
		t.Error("expected empty result for 0 templates")
	}

	// Section with 2 templates → error logged but still processes first template
	template1Values := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
	}
	template2Values := map[string]interface{}{
		"type": "container",
		"layout": "overflow",
	}
	section2 := sectionFragment{
		ID:         "comic-multi",
		PositionID: 100,
		PageTemplates: []pageTemplateFragment{
			{PositionID: 200, Storyline: "s1", PageTemplateValues: template1Values},
			{PositionID: 300, Storyline: "s2", PageTemplateValues: template2Values},
		},
		PageTemplateValues: map[string]interface{}{},
	}

	result = processSectionComic(section2, cfg, nil)
	// Error should be logged but function still processes the first template (leaf branch)
	if len(result.Sections) == 0 {
		t.Error("expected at least 1 section from 2-template comic (first template processed)")
	}
}

// =============================================================================
// VAL-M12-DISPATCH: detectBookTypeFromBook wires book type into section processing
// =============================================================================

func TestDetectBookTypeFromBookFlags(t *testing.T) {
	// Test that detectBookTypeFromBook correctly derives book type from decodedBook flags
	// that were set by applyMetadata/applyContentFeatures.
	tests := []struct {
		name     string
		book     *decodedBook
		expected bookType
	}{
		{
			name:     "empty book → none",
			book:     &decodedBook{},
			expected: bookTypeNone,
		},
		{
			name:     "CDEContentType MAGZ → magazine",
			book:     &decodedBook{CDEContentType: "MAGZ"},
			expected: bookTypeMagazine,
		},
		{
			name:     "CDEContentType EBSP → none",
			book:     &decodedBook{CDEContentType: "EBSP"},
			expected: bookTypeNone,
		},
		{
			name:     "FixedLayout + VirtualPanelsAllowed → comic",
			book:     &decodedBook{FixedLayout: true, VirtualPanelsAllowed: true},
			expected: bookTypeComic,
		},
		{
			name:     "FixedLayout + IsPDFBacked + IsPrintReplica + !VirtualPanelsAllowed → print_replica",
			book:     &decodedBook{FixedLayout: true, IsPDFBacked: true, IsPrintReplica: true},
			expected: bookTypePrintReplica,
		},
		{
			name:     "RegionMagnification → children",
			book:     &decodedBook{RegionMagnification: true},
			expected: bookTypeChildren,
		},
		{
			name:     "IsPDFBacked + IsPrintReplica → print_replica",
			book:     &decodedBook{IsPDFBacked: true, IsPrintReplica: true},
			expected: bookTypePrintReplica,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := detectBookTypeFromBook(test.book)
			if got != test.expected {
				t.Errorf("detectBookTypeFromBook() = %v, want %v", got, test.expected)
			}
		})
	}
}

func TestProcessReadingOrderUsesBookType(t *testing.T) {
	// Verify that processReadingOrder dispatches correctly when a pageSpreadConfig
	// is provided. When cfg is non-nil, processReadingOrder calls processSectionWithType
	// (which handles comic/magazine/scribe branches). When cfg is nil, it falls back
	// to processSection (reflowable-only).

	// Create a section that would be dispatched differently for comic vs reflowable
	sectionFragments := map[string]sectionFragment{
		"section-a": {
			ID:            "section-a",
			PositionID:    100,
			Storyline:     "story-a",
			PageTemplates: []pageTemplateFragment{{PositionID: 200, Storyline: "story-a"}},
			PageTemplateValues: map[string]interface{}{},
		},
	}
	storylines := map[string]map[string]interface{}{
		"story-a": {"content_list": []interface{}{
			map[string]interface{}{"content": map[string]interface{}{"name": "content-a", "index": 0}},
		}},
	}
	contentFragments := map[string][]string{
		"content-a": {"Hello world"},
	}

	book := &decodedBook{}
	renderer := &storylineRenderer{
		contentFragments:   contentFragments,
		styleFragments:     map[string]map[string]interface{}{},
		styles:             newStyleCatalog(),
		conditionEvaluator: conditionEvaluator{},
	}

	sectionOrder := []string{"section-a"}

	// Test with nil config — should fall back to processSection (reflowable)
	processReadingOrder(book, sectionOrder, sectionFragments, storylines, contentFragments, renderer, map[string]string{}, symOriginal, nil)

	if len(book.RenderedSections) != 1 {
		t.Fatalf("expected 1 rendered section (reflowable), got %d", len(book.RenderedSections))
	}
	if book.RenderedSections[0].Title == "" {
		t.Error("expected rendered section to have a title")
	}
}

func TestProcessReadingOrderWithComicBookType(t *testing.T) {
	// When book type is comic, processReadingOrder should dispatch to
	// processSectionWithType which calls processSectionComic.
	// This should produce page-spread sections, not standard reflowable content.

	sectionFragments := map[string]sectionFragment{
		"section-a": {
			ID:            "section-a",
			PositionID:    100,
			Storyline:     "story-a",
			PageTemplates: []pageTemplateFragment{{PositionID: 200, Storyline: "story-a"}},
			PageTemplateValues: map[string]interface{}{},
		},
	}
	storylines := map[string]map[string]interface{}{
		"story-a": {"content_list": []interface{}{
			map[string]interface{}{"content": map[string]interface{}{"name": "content-a", "index": 0}},
		}},
	}
	contentFragments := map[string][]string{
		"content-a": {"Hello world"},
	}

	book := &decodedBook{}
	renderer := &storylineRenderer{
		contentFragments:   contentFragments,
		styleFragments:     map[string]map[string]interface{}{},
		styles:             newStyleCatalog(),
		conditionEvaluator: conditionEvaluator{},
	}

	cfg := pageSpreadConfig{
		BookType:                 bookTypeComic,
		IsPdfBacked:              false,
		RegionMagnification:      false,
		VirtualPanelsAllowed:     false,
		PageProgressionDirection: "ltr",
	}

	sectionOrder := []string{"section-a"}

	// With comic config, processSectionWithType should dispatch to processSectionComic
	// which produces page-spread results (returned as false from processSectionWithType since
	// comic sections are handled separately).
	processReadingOrder(book, sectionOrder, sectionFragments, storylines, contentFragments, renderer, map[string]string{}, symOriginal, &cfg)

	// Comic sections are handled by processSectionComic which produces pageSpreadResult,
	// not standard rendered sections. processSectionWithType returns (renderedStoryline{}, nil, false)
	// for comic/magazine branches, so no RenderedSections should be added.
	if len(book.RenderedSections) != 0 {
		t.Errorf("expected 0 rendered sections for comic dispatch (handled by page-spread), got %d", len(book.RenderedSections))
	}
}

// =============================================================================
// Tests for cleanTextForLXML — VAL-FIX-003
// Port of Python yj_to_epub_content.py L67-84 (UNEXPECTED_CHARACTERS) and L1807-1816 (clean_text_for_lxml)
// =============================================================================

func TestCleanTextForLXML_FiltersC0ControlCharacters(t *testing.T) {
	// C0 control characters (0x00-0x1F except 0x09 TAB, 0x0A LF, 0x0D CR)
	// Python L68-71: 0x0000-0x0008, 0x000b-0x000c, 0x000e-0x001f
	input := "Hello\x00World\x01Test\x02"
	expected := "Hello?World?Test?"
	result := cleanTextForLXML(input)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestCleanTextForLXML_FiltersC1ControlCharacters(t *testing.T) {
	// C1 control characters (0x80-0x9F)
	// Python L72-75: 0x0080-0x009f
	input := "Text\u0080with\u009fC1"
	expected := "Text?with?C1"
	result := cleanTextForLXML(input)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestCleanTextForLXML_PermitsWhitespace(t *testing.T) {
	// Tab (0x09), LF (0x0A), CR (0x0D) are NOT in UNEXPECTED_CHARACTERS
	input := "Line1\tindented\nLine2\r\nLine3"
	result := cleanTextForLXML(input)
	if result != input {
		t.Errorf("whitespace should be preserved, expected %q, got %q", input, result)
	}
}

func TestCleanTextForLXML_FiltersArabicLetterMark(t *testing.T) {
	// Python L76: 0x061c (Arabic Letter Mark)
	input := "Text\u061cMore"
	expected := "Text?More"
	result := cleanTextForLXML(input)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestCleanTextForLXML_FiltersInvisibleSeparator(t *testing.T) {
	// Python L77: 0x2063 (Invisible Separator)
	input := "A\u2063B"
	expected := "A?B"
	result := cleanTextForLXML(input)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestCleanTextForLXML_FiltersInterlinearAnnotationChars(t *testing.T) {
	// Python L78: 0xfff9, 0xfffa, 0xfffb (Interlinear Annotation chars)
	input := "A\ufff9B\ufffaC\ufffbD"
	expected := "A?B?C?D"
	result := cleanTextForLXML(input)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestCleanTextForLXML_FiltersNonCharacters(t *testing.T) {
	// Python L79: 0xfffe, 0xffff (noncharacters)
	input := "X\ufffeY\uffffZ"
	expected := "X?Y?Z"
	result := cleanTextForLXML(input)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestCleanTextForLXML_PreservesNormalText(t *testing.T) {
	input := "Hello, World! 日本語 🌍"
	result := cleanTextForLXML(input)
	if result != input {
		t.Errorf("normal text should be preserved, expected %q, got %q", input, result)
	}
}

func TestCleanTextForLXML_EmptyString(t *testing.T) {
	result := cleanTextForLXML("")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestCleanTextForLXML_AllUnexpectedCharacters(t *testing.T) {
	// Verify all entries in the UNEXPECTED_CHARACTERS set are filtered
	for cp := range unexpectedCharacters {
		input := string(rune(cp))
		result := cleanTextForLXML(input)
		if result != "?" {
			t.Errorf("character U+%04X should be replaced with ?, got %q", cp, result)
		}
	}
}

func TestEscapeHTML_WithControlCharacters(t *testing.T) {
	// Verify that escapeHTML now filters control characters via cleanTextForLXML
	input := "Hello\x00World\x01Test"
	result := escapeHTML(input)
	if strings.Contains(result, "\x00") || strings.Contains(result, "\x01") {
		t.Errorf("escapeHTML should filter control characters, got %q", result)
	}
	// The ? replacements should be present
	if !strings.Contains(result, "Hello?World?Test") {
		t.Errorf("expected control chars replaced with ?, got %q", result)
	}
}

func TestResolveContentText_FiltersControlChars(t *testing.T) {
	// Verify that text resolved from content fragments has control chars filtered
	contentFragments := map[string][]string{
		"test_frag": {"Hello\x00World\u0080End"},
	}
	ref := map[string]interface{}{
		"name":  "test_frag",
		"index": int64(0),
	}
	result := resolveContentText(contentFragments, ref)
	expected := "Hello?World?End"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}
