package kfx

import (
	"strings"
	"testing"
)

// =============================================================================
// Tests for illustrated layout anchor rewrite — VAL-B-024 through VAL-B-032
// =============================================================================

// =============================================================================
// VAL-B-024: -kfx-amzn-condition anchor-id rewrite to fragment ID
// =============================================================================

func TestRewriteAmznConditionAnchorIDToFragment(t *testing.T) {
	// anchor-id with same-file anchor: URI → fragment ID only
	// The URI anchor:part0001#myid should match section filename part0001.xhtml
	style := "-kfx-amzn-condition: anchor-id anchor:part0001#myid"
	got := rewriteAmznConditionStyle(style, "part0001.xhtml")
	want := "-kfx-amzn-condition: anchor-id myid"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestRewriteAmznConditionAnchorOpaque(t *testing.T) {
	const fn = "c73.xhtml"
	got := rewriteAmznConditionStyle("-kfx-amzn-condition: anchor-id anchor:c73#frag1", fn)
	want := "-kfx-amzn-condition: anchor-id frag1"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// =============================================================================
// VAL-B-025: range-id.le operator prefix stripping
// =============================================================================

func TestRewriteAmznConditionRangeIDLeOperator(t *testing.T) {
	// range-id.le → stripped to range-id + fragment
	style := "-kfx-amzn-condition: range-id.le anchor:name#fragId"
	got := rewriteAmznConditionStyle(style, "name.xhtml")
	want := "-kfx-amzn-condition: range-id fragId"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// =============================================================================
// VAL-B-026: range-id.lt operator prefix stripping
// =============================================================================

func TestRewriteAmznConditionRangeIDLtOperator(t *testing.T) {
	// range-id.lt → stripped to range-id + fragment
	style := "-kfx-amzn-condition: range-id.lt anchor:name#fragId"
	got := rewriteAmznConditionStyle(style, "name.xhtml")
	want := "-kfx-amzn-condition: range-id fragId"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// =============================================================================
// VAL-B-027: Cross-file condition anchors are not rewritten
// =============================================================================

func TestRewriteAmznConditionCrossFileNotRewritten(t *testing.T) {
	// Different file path → style unchanged
	style := "-kfx-amzn-condition: anchor-id other_file.xhtml#fragId"
	got := rewriteAmznConditionStyle(style, "this_file.xhtml")
	if got != style {
		t.Errorf("cross-file condition should not be rewritten: got %q", got)
	}
}

// =============================================================================
// VAL-B-028: Condition operator mapping constants
// =============================================================================

func TestConditionOperatorNames(t *testing.T) {
	// Must map $294 → "anchor-id", $299 → "range-id.le", $298 → "range-id.lt"
	if conditionOperatorNames["=="] != "anchor-id" {
		t.Errorf(`$294: got %q, want "anchor-id"`, conditionOperatorNames["=="])
	}
	if conditionOperatorNames["<="] != "range-id.le" {
		t.Errorf(`$299: got %q, want "range-id.le"`, conditionOperatorNames["<="])
	}
	if conditionOperatorNames["<"] != "range-id.lt" {
		t.Errorf(`$298: got %q, want "range-id.lt"`, conditionOperatorNames["<"])
	}
	if len(conditionOperatorNames) != 3 {
		t.Errorf("expected 3 condition operators, got %d", len(conditionOperatorNames))
	}
}

// =============================================================================
// VAL-B-029: fixupIllustratedLayoutAnchors skips when no conditional content
// =============================================================================

func TestFixupIllustratedLayoutAnchorsSkipsWhenNotIllustrated(t *testing.T) {
	book := &decodedBook{IllustratedLayout: false}
	div := &htmlElement{
		Tag: "div",
		Attrs: map[string]string{
			"style": "-kfx-amzn-condition: anchor-id anchor:foo#bar",
		},
	}
	sections := []renderedSection{
		{Filename: "test.xhtml", Root: &htmlElement{Tag: "body", Children: []htmlPart{div}}},
	}
	fixupIllustratedLayoutAnchors(book, sections)
	// Style should NOT be rewritten since IllustratedLayout is false
	if div.Attrs["style"] != "-kfx-amzn-condition: anchor-id anchor:foo#bar" {
		t.Errorf("style should not be rewritten when IllustratedLayout=false, got %q", div.Attrs["style"])
	}
}

func TestFixupIllustratedLayoutAnchorsSkipsNilBook(t *testing.T) {
	div := &htmlElement{
		Tag: "div",
		Attrs: map[string]string{
			"style": "-kfx-amzn-condition: anchor-id anchor:foo#bar",
		},
	}
	sections := []renderedSection{
		{Filename: "test.xhtml", Root: &htmlElement{Tag: "body", Children: []htmlPart{div}}},
	}
	fixupIllustratedLayoutAnchors(nil, sections)
	if div.Attrs["style"] != "-kfx-amzn-condition: anchor-id anchor:foo#bar" {
		t.Errorf("style should not be rewritten when book is nil, got %q", div.Attrs["style"])
	}
}

// =============================================================================
// VAL-B-030: fixupIllustratedLayoutAnchors iterates all sections and all div children
// =============================================================================

func TestFixupIllustratedLayoutAnchorsIteratesAllSections(t *testing.T) {
	book := &decodedBook{IllustratedLayout: true}

	// Create 3 sections each with a div containing a same-file condition
	sections := make([]renderedSection, 3)
	for i := range sections {
		div := &htmlElement{
			Tag: "div",
			Attrs: map[string]string{
				"style": "-kfx-amzn-condition: anchor-id anchor:sec" + strings.Repeat("0", i) + "#frag",
			},
		}
		fn := "sec" + strings.Repeat("0", i) + ".xhtml"
		sections[i] = renderedSection{
			Filename: fn,
			Root:     &htmlElement{Tag: "body", Children: []htmlPart{div}},
		}
	}

	fixupIllustratedLayoutAnchors(book, sections)

	for i := range sections {
		body := sections[i].Root
		div := body.Children[0].(*htmlElement)
		got := div.Attrs["style"]
		want := "-kfx-amzn-condition: anchor-id frag"
		if got != want {
			t.Errorf("section %d: got style %q, want %q", i, got, want)
		}
	}
}

// =============================================================================
// VAL-B-031: STYLE_REWRITE preserves other declarations
// =============================================================================

func TestRewriteAmznConditionPreservesOtherDeclarations(t *testing.T) {
	style := "color: red; -kfx-amzn-condition: anchor-id anchor:x#frag; font-size: 12px"
	got := rewriteAmznConditionStyle(style, "x.xhtml")
	if !strings.Contains(got, "color: red") {
		t.Errorf("missing color declaration in %q", got)
	}
	if !strings.Contains(got, "font-size: 12px") {
		t.Errorf("missing font-size declaration in %q", got)
	}
	if !strings.Contains(got, "-kfx-amzn-condition: anchor-id frag") {
		t.Errorf("missing rewritten condition in %q", got)
	}
}

// =============================================================================
// Additional edge case tests
// =============================================================================

func TestRewriteAmznConditionNoFragmentNotRewritten(t *testing.T) {
	// URI without fragment → not rewritten
	style := "-kfx-amzn-condition: anchor-id anchor:some_name"
	got := rewriteAmznConditionStyle(style, "some_name.xhtml")
	if got != style {
		t.Errorf("no-fragment URI should not be rewritten, got %q", got)
	}
}

func TestRewriteAmznConditionNoMatchOnEmptyPath(t *testing.T) {
	// When URI path is empty and section filename is not empty, no match
	style := "-kfx-amzn-condition: anchor-id anchor:#fragOnly"
	got := rewriteAmznConditionStyle(style, "something.xhtml")
	// Empty path should match since it's same-file (no explicit different file)
	if !strings.Contains(got, "anchor-id fragOnly") {
		t.Errorf("empty-path same-file condition should be rewritten, got %q", got)
	}
}

func TestRewriteAmznConditionBasenamesMatch(t *testing.T) {
	// Path basename without extension matches section filename without extension
	style := "-kfx-amzn-condition: anchor-id /path/to/part0001#myfrag"
	got := rewriteAmznConditionStyle(style, "part0001.xhtml")
	want := "-kfx-amzn-condition: anchor-id myfrag"
	if got != want {
		t.Errorf("basename match: got %q want %q", got, want)
	}
}

func TestRewriteAmznConditionBasenameNoExtension(t *testing.T) {
	// Full filename match (path == section filename)
	style := "-kfx-amzn-condition: anchor-id part0001.xhtml#myfrag"
	got := rewriteAmznConditionStyle(style, "part0001.xhtml")
	want := "-kfx-amzn-condition: anchor-id myfrag"
	if got != want {
		t.Errorf("exact filename match: got %q want %q", got, want)
	}
}

// =============================================================================
// VAL-B-032: create_conditional_page_templates is gated by has_conditional_content
// =============================================================================

func TestCreateConditionalPageTemplatesSkipsWhenNoConditionalContent(t *testing.T) {
	book := &decodedBook{IllustratedLayout: true, HasConditionalContent: false}
	sections := []renderedSection{
		{
			Filename: "test.xhtml",
			Root: &htmlElement{
				Tag: "body",
				Children: []htmlPart{
					&htmlElement{Tag: "div", Attrs: map[string]string{
						"style": "-kfx-amzn-condition: anchor-id frag1",
					}},
				},
			},
		},
	}
	// When HasConditionalContent is false, the function should return early
	createConditionalPageTemplates(book, sections)
}

// =============================================================================
// Tests for createConditionalPageTemplates — inline mode (EMIT_PAGE_TEMPLATES=false)
// =============================================================================

func TestCreateConditionalPageTemplatesBasicTemplate(t *testing.T) {
	// Build a template div with condition + child img
	templateDiv := &htmlElement{
		Tag: "div",
		Attrs: map[string]string{
			"style": "-kfx-amzn-condition: anchor-id target1",
		},
		Children: []htmlPart{
			&htmlElement{
				Tag: "img",
				Attrs: map[string]string{
					"src": "image.png",
				},
			},
		},
	}

	// Target element
	targetDiv := &htmlElement{
		Tag: "div",
		Attrs: map[string]string{
			"id": "target1",
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{templateDiv, targetDiv},
	}

	book := &decodedBook{IllustratedLayout: true, HasConditionalContent: true}
	sections := []renderedSection{
		{Filename: "test.xhtml", Root: body},
	}

	// HasConditionalContent=true should process the templates
	createConditionalPageTemplates(book, sections)
	// EMIT_PAGE_TEMPLATES=false: non-float img is removed from template
	// The template div should have been removed from body and inserted into target
	// Verify the img child was removed (template had 1 img child, now empty)
	// After processing, templateDiv.Children should be empty since the img was removed
	if len(templateDiv.Children) != 0 {
		t.Errorf("expected template to have no children after img removal, got %d", len(templateDiv.Children))
	}
}

func TestCreateConditionalPageTemplatesFloatShapeNotRemoved(t *testing.T) {
	// img with -amzn-shape-outside should NOT be removed in inline mode
	templateDiv := &htmlElement{
		Tag: "div",
		Attrs: map[string]string{
			"style": "-kfx-amzn-condition: anchor-id target1; float: left",
		},
		Children: []htmlPart{
			&htmlElement{
				Tag: "img",
				Attrs: map[string]string{
					"src":    "image.png",
					"style":  "float: left; -amzn-shape-outside: url(image.png)",
				},
			},
		},
	}

	targetDiv := &htmlElement{
		Tag: "div",
		Attrs: map[string]string{
			"id": "target1",
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{templateDiv, targetDiv},
	}

	book := &decodedBook{IllustratedLayout: true, HasConditionalContent: true}
	sections := []renderedSection{
		{Filename: "test.xhtml", Root: body},
	}

	createConditionalPageTemplates(book, sections)
	// The img should still exist (it has shape-outside)
	if len(templateDiv.Children) == 0 {
		t.Error("float shape img should not have been removed")
	}
}

func TestCreateConditionalPageTemplatesDivElementRemoved(t *testing.T) {
	// div child elements are removed in inline mode (unless they have shape-outside)
	templateDiv := &htmlElement{
		Tag: "div",
		Attrs: map[string]string{
			"style": "-kfx-amzn-condition: anchor-id target1",
		},
		Children: []htmlPart{
			&htmlElement{
				Tag:   "div",
				Attrs: map[string]string{"style": "background-color: red"},
			},
		},
	}

	targetDiv := &htmlElement{
		Tag: "div",
		Attrs: map[string]string{
			"id": "target1",
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{templateDiv, targetDiv},
	}

	book := &decodedBook{IllustratedLayout: true, HasConditionalContent: true}
	sections := []renderedSection{
		{Filename: "test.xhtml", Root: body},
	}

	createConditionalPageTemplates(book, sections)
	// In inline mode, div with background-color is removed
	if len(templateDiv.Children) != 0 {
		t.Error("div child without shape-outside should have been removed")
	}
}

func TestCreateConditionalPageTemplatesPageAlignNoneDivMoved(t *testing.T) {
	// Last child div with -amzn-page-align=none and position=fixed should have
	// its id moved to body start and be removed from template
	templateDiv := &htmlElement{
		Tag: "div",
		Attrs: map[string]string{
			"style": "-kfx-amzn-condition: anchor-id target1",
		},
		Children: []htmlPart{
			&htmlElement{
				Tag:   "img",
				Attrs: map[string]string{"src": "image.png"},
			},
			&htmlElement{
				Tag:   "div",
				Attrs: map[string]string{"id": "story123", "style": "-amzn-page-align: none; position: fixed"},
			},
		},
	}

	targetDiv := &htmlElement{
		Tag: "div",
		Attrs: map[string]string{
			"id": "target1",
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{templateDiv, targetDiv},
	}

	book := &decodedBook{IllustratedLayout: true, HasConditionalContent: true}
	sections := []renderedSection{
		{Filename: "test.xhtml", Root: body},
	}

	createConditionalPageTemplates(book, sections)
	// The story div should be removed from template and its id added to body start
	// Body should have an id-only div at the start
	if len(body.Children) < 1 {
		t.Fatal("body should have children")
	}
	firstChild := body.Children[0].(*htmlElement)
	if firstChild.Tag != "div" {
		t.Errorf("expected first body child to be div, got %s", firstChild.Tag)
	}
	if firstChild.Attrs["id"] != "story123" {
		t.Errorf("expected id div for story123, got id=%q", firstChild.Attrs["id"])
	}
}

// =============================================================================
// Tests for helper functions used by createConditionalPageTemplates
// =============================================================================

func TestParseStyleString(t *testing.T) {
	style := "color: red; font-size: 12px"
	m := parseStyleString(style)
	if m["color"] != "red" {
		t.Errorf("color: got %q, want red", m["color"])
	}
	if m["font-size"] != "12px" {
		t.Errorf("font-size: got %q, want 12px", m["font-size"])
	}
}

func TestSerializeStyleMap(t *testing.T) {
	m := map[string]string{"color": "red", "font-size": "12px"}
	s := serializeStyleMap(m)
	if !strings.Contains(s, "color: red") {
		t.Errorf("missing color in %q", s)
	}
	if !strings.Contains(s, "font-size: 12px") {
		t.Errorf("missing font-size in %q", s)
	}
}

func TestFindElementByID(t *testing.T) {
	child := &htmlElement{Tag: "span", Attrs: map[string]string{"id": "target1"}}
	parent := &htmlElement{
		Tag:      "div",
		Children: []htmlPart{htmlText{Text: "hello"}, child},
	}
	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{parent},
	}

	found := findElementByID(body, "target1")
	if found == nil {
		t.Fatal("expected to find element with id=target1")
	}
	if found != child {
		t.Error("found wrong element")
	}
}

func TestFindElementByIDNotFound(t *testing.T) {
	body := &htmlElement{Tag: "body"}
	found := findElementByID(body, "nonexistent")
	if found != nil {
		t.Error("expected nil for non-existent id")
	}
}

func TestStripOperatorSuffix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"anchor-id", "anchor-id"},
		{"range-id.le", "range-id"},
		{"range-id.lt", "range-id"},
		{"range-id.foo.bar", "range-id"},
	}
	for _, tc := range tests {
		got := stripOperatorSuffix(tc.input)
		if got != tc.want {
			t.Errorf("stripOperatorSuffix(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
