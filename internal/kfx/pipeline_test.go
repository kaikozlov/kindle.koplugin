package kfx

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestBuildBookStateFragmentSummaryMatchesReference(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")

	state, err := buildBookState(input)
	if err != nil {
		t.Fatalf("buildBookState() error = %v", err)
	}
	got := state.fragmentSnapshot()
	want := referenceFragmentSnapshot(t, input)
	compareIDs := map[string]bool{
		"$145": true,
		"$157": true,
		"$164": true,
		"$259": true,
		"$260": true,
		"$266": true,
		"$391": true,
		"$609": true,
	}

	for _, fragmentType := range []string{
		"$145", "$157", "$164", "$258", "$259", "$260", "$262",
		"$266", "$389", "$391", "$417", "$490", "$538", "$585", "$609",
	} {
		gotType, gotOK := got.Types[fragmentType]
		wantType, wantOK := want.Types[fragmentType]
		if gotOK != wantOK {
			t.Fatalf("%s presence mismatch: got=%v want=%v", fragmentType, gotOK, wantOK)
		}
		if !gotOK {
			continue
		}
		if gotType.Count != wantType.Count {
			t.Fatalf("%s count = %d, want %d", fragmentType, gotType.Count, wantType.Count)
		}
		if compareIDs[fragmentType] && !equalStringSlices(gotType.IDs, wantType.IDs) {
			t.Fatalf("%s ids = %v, want %v", fragmentType, gotType.IDs, wantType.IDs)
		}
	}
}

func TestParseSectionFragmentKeepsAllPageTemplatesAndUsesLastAsPrimary(t *testing.T) {
	section := parseSectionFragment("ignored", map[string]interface{}{
		"$174": "cX",
		"$141": []interface{}{
			map[string]interface{}{
				"$155": 101,
				"$157": "s-first",
				"$176": "story-first",
			},
			map[string]interface{}{
				"$155": 202,
				"$157": "s-main",
				"$176": "story-main",
				"$171": []interface{}{"$659", "$826"},
			},
		},
	})

	if section.ID != "cX" {
		t.Fatalf("section.ID = %q", section.ID)
	}
	if len(section.PageTemplates) != 2 {
		t.Fatalf("len(section.PageTemplates) = %d", len(section.PageTemplates))
	}
	if section.PositionID != 202 {
		t.Fatalf("section.PositionID = %d, want 202", section.PositionID)
	}
	if section.Storyline != "story-main" {
		t.Fatalf("section.Storyline = %q, want story-main", section.Storyline)
	}
	if section.PageTemplateStyle != "s-main" {
		t.Fatalf("section.PageTemplateStyle = %q, want s-main", section.PageTemplateStyle)
	}
	if section.PageTemplates[1].Condition == nil {
		t.Fatalf("expected second page template to preserve its condition")
	}
	if section.PageTemplates[0].Storyline != "story-first" || section.PageTemplates[1].Storyline != "story-main" {
		t.Fatalf("section.PageTemplates storylines = %#v", section.PageTemplates)
	}
}

func TestRenderSectionFragmentsUsesLastTemplateAsMainAndPrependsOverlaysAfterMain(t *testing.T) {
	section := sectionFragment{
		ID: "cX",
		PageTemplates: []pageTemplateFragment{
			{PositionID: 11, Storyline: "story-overlay"},
			{PositionID: 22, Storyline: "story-main"},
		},
	}
	storylines := map[string]map[string]interface{}{
		"story-overlay": {
			"$146": []interface{}{
				map[string]interface{}{"$145": map[string]interface{}{"name": "content", "$403": 0}},
			},
		},
		"story-main": {
			"$146": []interface{}{
				map[string]interface{}{"$145": map[string]interface{}{"name": "content", "$403": 1}},
			},
		},
	}
	renderer := storylineRenderer{
		contentFragments: map[string][]string{"content": {"Overlay", "Main"}},
		resourceHrefByID: map[string]string{},
		anchorToFilename: map[string]string{},
		positionToSection: map[int]string{
			11: "cX",
			22: "cX",
		},
		positionAnchors:  map[int]map[int][]string{},
		positionAnchorID: map[int]map[int]string{},
		emittedAnchorIDs: map[string]bool{},
		styleFragments:   map[string]map[string]interface{}{},
		styles:           newStyleCatalog(),
	}

	rendered, paragraphs, ok := renderSectionFragments("cX", section, storylines, renderer.contentFragments, &renderer)
	if !ok {
		t.Fatalf("renderSectionFragments() returned ok=false")
	}
	if len(paragraphs) != 2 || paragraphs[0] != "Main" || paragraphs[1] != "Overlay" {
		t.Fatalf("paragraphs = %#v", paragraphs)
	}
	if !strings.Contains(rendered.BodyHTML, "<p>Main</p>") || !strings.Contains(rendered.BodyHTML, "<p>Overlay</p>") {
		t.Fatalf("rendered.BodyHTML = %q", rendered.BodyHTML)
	}
	if strings.Index(rendered.BodyHTML, "<p>Main</p>") > strings.Index(rendered.BodyHTML, "<p>Overlay</p>") {
		t.Fatalf("rendered.BodyHTML order = %q", rendered.BodyHTML)
	}
}

func TestRenderSectionFragmentsSelectsActiveConditionalTemplateForFixedLayout(t *testing.T) {
	section := sectionFragment{
		ID: "cX",
		PageTemplates: []pageTemplateFragment{
			{PositionID: 11, Storyline: "story-inactive", Condition: []interface{}{"$750", "$753"}},
			{PositionID: 22, Storyline: "story-active", Condition: []interface{}{"$750", "$752"}},
		},
	}
	storylines := map[string]map[string]interface{}{
		"story-inactive": {
			"$146": []interface{}{
				map[string]interface{}{"$145": map[string]interface{}{"name": "content", "$403": 0}},
			},
		},
		"story-active": {
			"$146": []interface{}{
				map[string]interface{}{"$145": map[string]interface{}{"name": "content", "$403": 1}},
			},
		},
	}
	renderer := storylineRenderer{
		contentFragments: map[string][]string{"content": {"Inactive", "Active"}},
		resourceHrefByID: map[string]string{},
		anchorToFilename: map[string]string{},
		positionToSection: map[int]string{
			11: "cX",
			22: "cX",
		},
		positionAnchors:  map[int]map[int][]string{},
		positionAnchorID: map[int]map[int]string{},
		emittedAnchorIDs: map[string]bool{},
		styleFragments:   map[string]map[string]interface{}{},
		styles:           newStyleCatalog(),
		conditionEvaluator: conditionEvaluator{
			fixedLayout: true,
		},
	}

	rendered, paragraphs, ok := renderSectionFragments("cX", section, storylines, renderer.contentFragments, &renderer)
	if !ok {
		t.Fatalf("renderSectionFragments() returned ok=false")
	}
	if len(paragraphs) != 1 || paragraphs[0] != "Active" {
		t.Fatalf("paragraphs = %#v", paragraphs)
	}
	if strings.Contains(rendered.BodyHTML, "Inactive") || !strings.Contains(rendered.BodyHTML, "Active") {
		t.Fatalf("rendered.BodyHTML = %q", rendered.BodyHTML)
	}
}

func TestRenderNodeSupportsListsAndRules(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments: map[string][]string{"content": []string{"First", "Second"}},
		resourceHrefByID: map[string]string{},
		anchorToFilename: map[string]string{},
		positionToSection: map[int]string{
			1: "cX",
			2: "cX",
		},
		positionAnchors:  map[int]map[int][]string{},
		positionAnchorID: map[int]map[int]string{},
		emittedAnchorIDs: map[string]bool{},
		styleFragments:   map[string]map[string]interface{}{},
		styles:           newStyleCatalog(),
	}

	list := renderer.renderNode(map[string]interface{}{
		"$159": "$276",
		"$100": "$346",
		"$104": 3,
		"$146": []interface{}{
			map[string]interface{}{
				"$159": "$277",
				"$145": map[string]interface{}{"name": "content", "$403": 0},
			},
			map[string]interface{}{
				"$159": "$277",
				"$145": map[string]interface{}{"name": "content", "$403": 1},
			},
		},
	}, 0)
	listHTML := renderHTMLPart(list)
	if !strings.Contains(listHTML, "<ol start=\"3\">") {
		t.Fatalf("listHTML = %q", listHTML)
	}
	if !strings.Contains(listHTML, "<li>First</li>") || !strings.Contains(listHTML, "<li>Second</li>") {
		t.Fatalf("listHTML = %q", listHTML)
	}

	rule := renderer.renderNode(map[string]interface{}{"$159": "$596"}, 0)
	ruleHTML := renderHTMLPart(rule)
	if ruleHTML != "<hr/>" && ruleHTML != "<hr />" {
		t.Fatalf("ruleHTML = %q", ruleHTML)
	}
}

func TestRenderNodeSupportsHiddenContent(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments:  map[string][]string{"content": []string{"Hidden"}},
		resourceHrefByID:  map[string]string{},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
	}

	hidden := renderer.renderNode(map[string]interface{}{
		"$159": "$439",
		"$146": []interface{}{
			map[string]interface{}{
				"$145": map[string]interface{}{"name": "content", "$403": 0},
			},
		},
	}, 0)
	hiddenHTML := renderHTMLPart(hidden)
	renderer.styles.markReferenced(hiddenHTML)
	if !strings.Contains(hiddenHTML, "class=") || !strings.Contains(hiddenHTML, "<p>Hidden</p>") {
		t.Fatalf("hiddenHTML = %q", hiddenHTML)
	}
	if !strings.Contains(renderer.styles.String(), "display: none") {
		t.Fatalf("stylesheet missing hidden display rule: %q", renderer.styles.String())
	}
}

func TestNormalizeHTMLWhitespaceReplacesEOLKindsAndPreservesSpaces(t *testing.T) {
	root := &htmlElement{
		Attrs: map[string]string{},
		Children: []htmlPart{
			&htmlElement{
				Tag:   "p",
				Attrs: map[string]string{},
				Children: []htmlPart{
					htmlText{Text: " lead  one\rline\u2028break\u2029tail\nend"},
				},
			},
		},
	}

	normalizeHTMLWhitespace(root)
	got := renderHTMLParts(root.Children, false)

	if !strings.Contains(got, "<br/>line<br/>break<br/>tail<br/>end") {
		t.Fatalf("normalized html missing expected line breaks: %q", got)
	}
	if !strings.Contains(got, "<p>\u00a0lead\u00a0\u00a0one") {
		t.Fatalf("normalized html missing expected non-breaking spaces: %q", got)
	}
}

func TestNormalizeHTMLWhitespaceCarriesDoubleSpaceAcrossInlineBoundaries(t *testing.T) {
	root := &htmlElement{
		Attrs: map[string]string{},
		Children: []htmlPart{
			&htmlElement{
				Tag:   "p",
				Attrs: map[string]string{},
				Children: []htmlPart{
					htmlText{Text: "a "},
					&htmlElement{
						Tag:      "span",
						Attrs:    map[string]string{},
						Children: []htmlPart{htmlText{Text: " b"}},
					},
				},
			},
		},
	}

	normalizeHTMLWhitespace(root)
	got := renderHTMLParts(root.Children, false)

	if got != "<p>a\u00a0<span>\u00a0b</span></p>" {
		t.Fatalf("normalized html = %q", got)
	}
}

func TestRenderTableNodePreservesColumnAndCellSpanAttributes(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments: map[string][]string{"content": {"Cell"}},
		resourceHrefByID: map[string]string{},
		anchorToFilename: map[string]string{},
		positionToSection: map[int]string{
			1: "cX",
		},
		positionAnchors:  map[int]map[int][]string{},
		positionAnchorID: map[int]map[int]string{},
		emittedAnchorIDs: map[string]bool{},
		styleFragments:   map[string]map[string]interface{}{},
		styles:           newStyleCatalog(),
	}

	table := renderer.renderNode(map[string]interface{}{
		"$159": "$278",
		"$152": []interface{}{
			map[string]interface{}{"$118": 2},
		},
		"$146": []interface{}{
			map[string]interface{}{
				"$159": "$279",
				"$146": []interface{}{
					map[string]interface{}{
						"$148": 2,
						"$149": 3,
						"$145": map[string]interface{}{"name": "content", "$403": 0},
					},
				},
			},
		},
	}, 0)
	tableHTML := renderHTMLPart(table)

	if !strings.Contains(tableHTML, "<col span=\"2\"/>") && !strings.Contains(tableHTML, "<col span=\"2\" />") {
		t.Fatalf("tableHTML missing col span: %q", tableHTML)
	}
	if !strings.Contains(tableHTML, "<td colspan=\"2\" rowspan=\"3\">Cell</td>") {
		t.Fatalf("tableHTML missing cell spans: %q", tableHTML)
	}
}

func TestRenderNodePromotesClassifiedContainersToAside(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments: map[string][]string{"content": {"Note"}},
		resourceHrefByID: map[string]string{},
		anchorToFilename: map[string]string{},
		positionToSection: map[int]string{
			1: "cX",
		},
		positionAnchors:  map[int]map[int][]string{},
		positionAnchorID: map[int]map[int]string{},
		emittedAnchorIDs: map[string]bool{},
		styleFragments:   map[string]map[string]interface{}{},
		styles:           newStyleCatalog(),
	}

	node := renderer.renderNode(map[string]interface{}{
		"$615": "$618",
		"$146": []interface{}{
			map[string]interface{}{
				"$145": map[string]interface{}{"name": "content", "$403": 0},
			},
		},
	}, 0)
	got := renderHTMLPart(node)

	if got != "<aside epub:type=\"footnote\"><p>Note</p></aside>" {
		t.Fatalf("classified aside html = %q", got)
	}
}

func TestRenderTableNodePromotesCaptionClassification(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments: map[string][]string{"content": {"Caption"}},
		resourceHrefByID: map[string]string{},
		anchorToFilename: map[string]string{},
		positionToSection: map[int]string{
			1: "cX",
		},
		positionAnchors:  map[int]map[int][]string{},
		positionAnchorID: map[int]map[int]string{},
		emittedAnchorIDs: map[string]bool{},
		styleFragments:   map[string]map[string]interface{}{},
		styles:           newStyleCatalog(),
	}

	table := renderer.renderNode(map[string]interface{}{
		"$159": "$278",
		"$146": []interface{}{
			map[string]interface{}{
				"$615": "$453",
				"$146": []interface{}{
					map[string]interface{}{
						"$145": map[string]interface{}{"name": "content", "$403": 0},
					},
				},
			},
		},
	}, 0)
	got := renderHTMLPart(table)

	if !strings.Contains(got, "<caption><p>Caption</p></caption>") {
		t.Fatalf("table caption html = %q", got)
	}
}

func TestRenderNodeAddsFixedLayoutClassForFixedContainers(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments:  map[string][]string{"content": {"Fixed"}},
		resourceHrefByID:  map[string]string{},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
	}

	node := renderer.renderNode(map[string]interface{}{
		"$156": "$324",
		"$146": []interface{}{
			map[string]interface{}{
				"$145": map[string]interface{}{"name": "content", "$403": 0},
			},
		},
	}, 0)
	got := renderHTMLPart(node)
	renderer.styles.markReferenced(got)

	if !strings.Contains(got, "class=") || !strings.Contains(got, "<p>Fixed</p>") {
		t.Fatalf("fixed layout html = %q", got)
	}
	if !strings.Contains(renderer.styles.String(), "position: fixed") {
		t.Fatalf("stylesheet missing fixed-position rule: %q", renderer.styles.String())
	}
}

func TestRenderTextNodeSupportsDropCaps(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments:  map[string][]string{"content": {"Hello"}},
		resourceHrefByID:  map[string]string{},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
	}

	node := renderer.renderTextNode(map[string]interface{}{
		"$145": map[string]interface{}{"name": "content", "$403": 0},
		"$125": 2,
		"$126": 1,
	}, 0)
	got := renderHTMLPart(node)
	renderer.styles.markReferenced(got)

	if !strings.Contains(got, "<span") || !strings.Contains(got, ">H</span>ello</p>") {
		t.Fatalf("drop cap html = %q", got)
	}
	stylesheet := renderer.styles.String()
	if !strings.Contains(stylesheet, "float: left") || !strings.Contains(stylesheet, "font-size: 2em") {
		t.Fatalf("drop cap stylesheet = %q", stylesheet)
	}
}

func TestRenderNodePromotesFigureLayoutHints(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments: map[string][]string{},
		resourceHrefByID: map[string]string{"img1": "images/pic.png"},
		resourceFragments: map[string]resourceFragment{
			"img1": {ID: "img1", MediaType: "image/png"},
		},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments: map[string]map[string]interface{}{
			"sFigure": {"$761": "figure"},
		},
		styles: newStyleCatalog(),
	}

	node := renderer.renderNode(map[string]interface{}{
		"$157": "sFigure",
		"$146": []interface{}{
			map[string]interface{}{
				"$175": "img1",
			},
		},
	}, 0)
	got := renderHTMLPart(node)

	if !strings.Contains(got, "<figure>") || !strings.Contains(got, "<img src=\"images/pic.png\" alt=\"\"/>") && !strings.Contains(got, "<img src=\"images/pic.png\" alt=\"\" />") {
		t.Fatalf("figure html = %q", got)
	}
}

func TestRenderNodePromotesHeadingLayoutHints(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments:  map[string][]string{"content": {"Heading"}},
		resourceHrefByID:  map[string]string{},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments: map[string]map[string]interface{}{
			"sHeading": {"$761": "heading"},
		},
		styles: newStyleCatalog(),
	}

	node := renderer.renderNode(map[string]interface{}{
		"$157": "sHeading",
		"$790": 3,
		"$146": []interface{}{
			map[string]interface{}{
				"$145": map[string]interface{}{"name": "content", "$403": 0},
			},
		},
	}, 0)
	got := renderHTMLPart(node)

	if got != "<h3>Heading</h3>" {
		t.Fatalf("heading layout hint html = %q", got)
	}
}

func TestRenderNodePromotesInlineOnlyContainersToParagraphs(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments:  map[string][]string{"content": {"Hello"}},
		resourceHrefByID:  map[string]string{},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
	}

	node := renderer.renderNode(map[string]interface{}{
		"$146": []interface{}{
			map[string]interface{}{
				"$145": map[string]interface{}{"name": "content", "$403": 0},
			},
		},
	}, 0)
	got := renderHTMLPart(node)

	if got != "<p>Hello</p>" {
		t.Fatalf("inline-only container html = %q", got)
	}
}

func TestRenderNodeDoesNotPromoteMultiParagraphContainersToParagraph(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments:  map[string][]string{"content": {"Contents", "Cover"}},
		resourceHrefByID:  map[string]string{},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
	}

	node := renderer.renderNode(map[string]interface{}{
		"$157": "s53",
		"$146": []interface{}{
			map[string]interface{}{
				"$157": "s1S",
				"$790": 1,
				"$145": map[string]interface{}{"name": "content", "$403": 0},
			},
			map[string]interface{}{
				"$157": "s1U",
				"$145": map[string]interface{}{"name": "content", "$403": 1},
			},
		},
	}, 0)
	got := renderHTMLPart(node)

	// Python parity: $790 alone (without "heading" in layout hints) does NOT promote to <h1>.
	// Calibre simplify_styles only promotes <div> to heading when layout hints include "heading".
	if got != "<div><p>Contents</p><p>Cover</p></div>" {
		t.Fatalf("multi-paragraph container html = %q", got)
	}
}

func TestRenderNodeDoesNotPromoteLayoutHintsInFixedLayout(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments: map[string][]string{"content": {"Heading"}},
		resourceHrefByID: map[string]string{"img1": "images/pic.png"},
		resourceFragments: map[string]resourceFragment{
			"img1": {ID: "img1", MediaType: "image/png"},
		},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments: map[string]map[string]interface{}{
			"sHeading": {"$761": "heading"},
			"sFigure":  {"$761": "figure"},
		},
		styles: newStyleCatalog(),
		conditionEvaluator: conditionEvaluator{
			fixedLayout: true,
		},
	}

	heading := renderHTMLPart(renderer.renderNode(map[string]interface{}{
		"$157": "sHeading",
		"$790": 3,
		"$146": []interface{}{
			map[string]interface{}{
				"$145": map[string]interface{}{"name": "content", "$403": 0},
			},
		},
	}, 0))
	figure := renderHTMLPart(renderer.renderNode(map[string]interface{}{
		"$157": "sFigure",
		"$146": []interface{}{
			map[string]interface{}{
				"$175": "img1",
			},
		},
	}, 0))

	if heading == "<h3>Heading</h3>" || strings.Contains(figure, "<figure>") {
		t.Fatalf("fixed layout should suppress layout-hint promotion, heading=%q figure=%q", heading, figure)
	}
}

func TestRenderNodeSupportsNodeLevelLinks(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments: map[string][]string{},
		resourceHrefByID: map[string]string{"img1": "images/pic.png"},
		resourceFragments: map[string]resourceFragment{
			"img1": {ID: "img1", MediaType: "image/png"},
		},
		anchorToFilename:  map[string]string{"dest": "chapter.xhtml#dest"},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
	}

	node := renderer.renderNode(map[string]interface{}{
		"$175": "img1",
		"$179": "dest",
	}, 0)
	got := renderHTMLPart(node)

	if !strings.Contains(got, "<a href=\"chapter.xhtml#dest\">") || !strings.Contains(got, "<img src=\"images/pic.png\" alt=\"\"/>") && !strings.Contains(got, "<img src=\"images/pic.png\" alt=\"\" />") {
		t.Fatalf("linked node html = %q", got)
	}
}

func TestRenderNodeSupportsInlineRenderContainers(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments:  map[string][]string{"content": {"Inline"}},
		resourceHrefByID:  map[string]string{},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
	}

	node := renderer.renderNode(map[string]interface{}{
		"$601": "$283",
		"$146": []interface{}{
			map[string]interface{}{
				"$145": map[string]interface{}{"name": "content", "$403": 0},
			},
		},
	}, 0)
	got := renderHTMLPart(node)

	if got != "<span>Inline</span>" {
		t.Fatalf("inline render html = %q", got)
	}
}

func TestRenderImageNodeFitTightDropsWidthHundredPercent(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments: map[string][]string{},
		resourceHrefByID: map[string]string{"img1": "images/pic.png"},
		resourceFragments: map[string]resourceFragment{
			"img1": {ID: "img1", MediaType: "image/png"},
		},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments: map[string]map[string]interface{}{
			"sImg": {
				"$56": map[string]interface{}{"$307": 100.0, "$306": "$314"},
				"$57": map[string]interface{}{"$307": 5.0, "$306": "$308"},
			},
		},
		styles: newStyleCatalog(),
	}

	node := renderer.renderNode(map[string]interface{}{
		"$157": "sImg",
		"$175": "img1",
		"$784": true,
	}, 0)
	got := renderHTMLPart(node)
	renderer.styles.markReferenced(got)
	stylesheet := renderer.styles.String()

	if !strings.Contains(got, "<img src=\"images/pic.png\" alt=\"\"") {
		t.Fatalf("fit tight image html = %q", got)
	}
	if strings.Contains(stylesheet, "width: 100%") {
		t.Fatalf("fit tight stylesheet should not keep width 100%%: %q", stylesheet)
	}
	if !strings.Contains(stylesheet, "height: 5em") {
		t.Fatalf("fit tight stylesheet lost non-width declarations: %q", stylesheet)
	}
}

func TestRenderTextNodeSupportsFirstLineStyles(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments:  map[string][]string{"content": {"Hello world"}},
		resourceHrefByID:  map[string]string{},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
	}

	node := renderer.renderTextNode(map[string]interface{}{
		"$145": map[string]interface{}{"name": "content", "$403": 0},
		"$622": map[string]interface{}{
			"$16":  2.0,
			"$625": map[string]interface{}{"$623": 1},
		},
	}, 0)
	got := renderHTMLPart(node)
	renderer.styles.markReferenced(got)
	stylesheet := renderer.styles.String()

	if !strings.Contains(got, "class=") || !strings.Contains(got, "<p") {
		t.Fatalf("first-line html = %q", got)
	}
	if !strings.Contains(stylesheet, "::first-line") || !strings.Contains(stylesheet, "font-size: 2") {
		t.Fatalf("first-line stylesheet = %q", stylesheet)
	}
}

func TestPruneUnusedStylesheetRulesKeepsPseudoClassRulesForUsedBaseClass(t *testing.T) {
	stylesheet := strings.Join([]string{
		".kfx-firstline-0::first-line { font-size: 2em; }",
		".unused::first-line { font-size: 3em; }",
	}, "\n")
	used := map[string]bool{"kfx-firstline-0": true}

	pruned := pruneUnusedStylesheetRules(stylesheet, used)

	if !strings.Contains(pruned, ".kfx-firstline-0::first-line") {
		t.Fatalf("expected used first-line rule to remain: %q", pruned)
	}
	if strings.Contains(pruned, ".unused::first-line") {
		t.Fatalf("expected unused first-line rule to be pruned: %q", pruned)
	}
}

func TestSimplifyStylesFullUnwrapsSpanWhenStyleIsFullyInherited(t *testing.T) {
	catalog := newStyleCatalog()
	italicParent := catalog.bind("class_italic", []string{"font-style: italic"})
	italicSpan := catalog.bind("class_italic", []string{"font-style: italic"})
	book := &decodedBook{
		RenderedSections: []renderedSection{{
			Root: &htmlElement{
				Attrs: map[string]string{},
				Children: []htmlPart{
					&htmlElement{
						Tag:   "ul",
						Attrs: map[string]string{},
						Children: []htmlPart{
							&htmlElement{
								Tag:   "li",
								Attrs: map[string]string{"class": italicParent},
								Children: []htmlPart{
									&htmlElement{
										Tag:      "span",
										Attrs:    map[string]string{"class": italicSpan},
										Children: []htmlPart{htmlText{Text: "A tree"}},
									},
								},
							},
						},
					},
				},
			},
		}},
	}

	simplifyStylesFull(book, catalog, nil, "serif")
	got := renderedSectionBodyHTML(book.RenderedSections[0])

	if strings.Contains(got, "<span") {
		t.Fatalf("expected inherited span wrapper to be removed, got %q", got)
	}
	if !strings.Contains(got, "A tree</li>") {
		t.Fatalf("unexpected simplified html %q", got)
	}
}

func TestSimplifyStylesFullRunsBeforeCSSEmission(t *testing.T) {
	catalog := newStyleCatalog()
	italicParent := catalog.bind("class_italic", []string{"font-style: italic"})
	italicSpan := catalog.bind("class_italic", []string{"font-style: italic"})
	book := &decodedBook{
		RenderedSections: []renderedSection{{
			Root: &htmlElement{
				Attrs: map[string]string{},
				Children: []htmlPart{
					&htmlElement{
						Tag:   "div",
						Attrs: map[string]string{"class": italicParent},
						Children: []htmlPart{
							&htmlElement{Tag: "span", Attrs: map[string]string{"class": italicSpan}, Children: []htmlPart{htmlText{Text: "W"}}},
							&htmlElement{Tag: "span", Attrs: map[string]string{"class": italicSpan}, Children: []htmlPart{htmlText{Text: "H"}}},
							&htmlElement{Tag: "span", Attrs: map[string]string{"class": italicSpan}, Children: []htmlPart{htmlText{Text: "I"}}},
							&htmlElement{Tag: "span", Attrs: map[string]string{"class": italicSpan}, Children: []htmlPart{htmlText{Text: "P"}}},
							htmlText{Text: "plain"},
						},
					},
					&htmlElement{
						Tag:      "div",
						Attrs:    map[string]string{},
						Children: []htmlPart{htmlText{Text: "other"}},
					},
				},
			},
		}},
	}

	simplifyStylesFull(book, catalog, nil, "serif")
	for i := range book.RenderedSections {
		catalog.markReferenced(renderedSectionBodyHTML(book.RenderedSections[i]))
	}
	replacer := catalog.replacer()
	for i := range book.RenderedSections {
		replaceSectionDOMClassTokens(&book.RenderedSections[i], replacer)
	}
	createCSSFiles(book, catalog)

	gotHTML := renderedSectionBodyHTML(book.RenderedSections[0])
	if strings.Contains(gotHTML, "<span") {
		t.Fatalf("expected reverse inheritance to remove child span wrappers, got %q", gotHTML)
	}
	if !strings.Contains(gotHTML, "<p class=") {
		t.Fatalf("expected simplified visible paragraph class, got %q", gotHTML)
	}
	if strings.Contains(gotHTML, "__STYLE_") {
		t.Fatalf("unexpected unresolved style token in html %q", gotHTML)
	}
	if strings.Count(book.Stylesheet, "font-style: italic") != 1 {
		t.Fatalf("expected a single emitted italic rule after simplification, stylesheet = %q", book.Stylesheet)
	}
}

func TestRenderNodeAddsMathRoleForClassifiedContent(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments:  map[string][]string{"content": {"x+y"}},
		resourceHrefByID:  map[string]string{},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
	}

	node := renderer.renderNode(map[string]interface{}{
		"$615": "$688",
		"$145": map[string]interface{}{"name": "content", "$403": 0},
	}, 0)
	got := renderHTMLPart(node)

	if got != "<p role=\"math\">x+y</p>" {
		t.Fatalf("math role html = %q", got)
	}
}

func TestConditionEvaluatorMatchesKnownSupportedFeaturesAndLogic(t *testing.T) {
	eval := conditionEvaluator{orientationLock: "portrait"}

	if !eval.evaluateBinary([]interface{}{"$659", "$826"}) {
		t.Fatalf("expected known supported feature $826 to be true")
	}
	if !eval.evaluateBinary([]interface{}{"$292", []interface{}{"$659", "$826"}, []interface{}{"$293", []interface{}{"$750", "$753"}}}) {
		t.Fatalf("expected composed condition to be true")
	}
	if eval.evaluateBinary([]interface{}{"$294", []interface{}{"$304"}, []interface{}{"$305"}}) {
		t.Fatalf("expected screen width == screen height to be false in portrait mode")
	}
}

func TestRenderNodeHonorsIncludeExcludeConditions(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments:  map[string][]string{"content": {"Visible"}},
		resourceHrefByID:  map[string]string{},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
		conditionEvaluator: conditionEvaluator{
			orientationLock: "portrait",
		},
	}

	included := renderer.renderNode(map[string]interface{}{
		"$592": []interface{}{"$659", "$826"},
		"$145": map[string]interface{}{"name": "content", "$403": 0},
	}, 0)
	if got := renderHTMLPart(included); got != "<p>Visible</p>" {
		t.Fatalf("included html = %q", got)
	}

	excluded := renderer.renderNode(map[string]interface{}{
		"$591": []interface{}{"$659", "$826"},
		"$145": map[string]interface{}{"name": "content", "$403": 0},
	}, 0)
	if excluded != nil {
		t.Fatalf("excluded node should be nil, got %q", renderHTMLPart(excluded))
	}
}

func TestConditionalPropertiesMergeIntoRenderableNode(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments:  map[string][]string{"content": {"Conditional"}},
		resourceHrefByID:  map[string]string{},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
		conditionEvaluator: conditionEvaluator{
			orientationLock: "portrait",
		},
	}

	node := renderer.renderNode(map[string]interface{}{
		"$146": []interface{}{
			map[string]interface{}{
				"$145": map[string]interface{}{"name": "content", "$403": 0},
			},
		},
		"$663": []interface{}{
			map[string]interface{}{
				"$592": []interface{}{"$659", "$826"},
				"$16":  2.0,
			},
		},
	}, 0)
	got := renderHTMLPart(node)
	renderer.styles.markReferenced(got)

	if !strings.Contains(got, "class=") || !strings.Contains(got, "<p>Conditional</p>") {
		t.Fatalf("conditional node html = %q", got)
	}
	if !strings.Contains(renderer.styles.String(), "font-size: 2") {
		t.Fatalf("conditional properties did not affect stylesheet: %q", renderer.styles.String())
	}
}

func TestBuildResourcesKeepsNonImageResources(t *testing.T) {
	book := &decodedBook{}
	resources, _, _, hrefs := buildResources(
		book,
		map[string]resourceFragment{
			"plug1": {ID: "plug1", Location: "plugin-entry", MediaType: "plugin/kfx-html-article"},
		},
		nil,
		map[string][]byte{
			"plugin-entry": []byte("<html><body>plugin</body></html>"),
		},
		nil,
		symOriginal,
	)

	if len(resources) != 1 {
		t.Fatalf("resource count = %d", len(resources))
	}
	if resources[0].MediaType != "plugin/kfx-html-article" {
		t.Fatalf("resource media type = %q", resources[0].MediaType)
	}
	if !strings.HasSuffix(resources[0].Filename, ".html") {
		t.Fatalf("resource filename = %q", resources[0].Filename)
	}
	if hrefs["plug1"] == "" {
		t.Fatalf("resource href missing for plug1")
	}
}

func TestRenderNodeSupportsHTMLPluginResources(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments:  map[string][]string{},
		resourceHrefByID:  map[string]string{"plug1": "resource_plugin-entry.html"},
		resourceFragments: map[string]resourceFragment{"plug1": {ID: "plug1", MediaType: "plugin/kfx-html-article"}},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
	}

	node := renderer.renderNode(map[string]interface{}{
		"$159": "$274",
		"$175": "plug1",
	}, 0)
	got := renderHTMLPart(node)
	renderer.styles.markReferenced(got)

	if !strings.Contains(got, "<iframe") || !strings.Contains(got, "src=\"resource_plugin-entry.html\"") || !strings.Contains(got, "class=") {
		t.Fatalf("plugin html = %q", got)
	}
	if !strings.Contains(renderer.styles.String(), "height: 100%") || !strings.Contains(renderer.styles.String(), "width: 100%") {
		t.Fatalf("plugin stylesheet = %q", renderer.styles.String())
	}
}

func TestRenderNodeSupportsAudioAndVideoPluginResources(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments: map[string][]string{},
		resourceHrefByID: map[string]string{
			"audio1": "resource_sound.mp3",
			"video1": "resource_clip.mp4",
		},
		resourceFragments: map[string]resourceFragment{
			"audio1": {ID: "audio1", MediaType: "audio/mpeg"},
			"video1": {ID: "video1", MediaType: "video/mp4"},
		},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
	}

	audio := renderHTMLPart(renderer.renderNode(map[string]interface{}{
		"$159": "$274",
		"$175": "audio1",
	}, 0))
	video := renderHTMLPart(renderer.renderNode(map[string]interface{}{
		"$159": "$274",
		"$175": "video1",
		"$584": "Video alt",
	}, 0))

	if !strings.Contains(audio, "<audio") || !strings.Contains(audio, "src=\"resource_sound.mp3\"") || !strings.Contains(audio, "controls=\"controls\"") {
		t.Fatalf("audio plugin html = %q", audio)
	}
	if !strings.Contains(video, "<video") || !strings.Contains(video, "src=\"resource_clip.mp4\"") || !strings.Contains(video, "aria-label=\"Video alt\"") {
		t.Fatalf("video plugin html = %q", video)
	}
}

func TestRenderNodeSupportsBasicSVGContainers(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments:  map[string][]string{},
		resourceHrefByID:  map[string]string{},
		resourceFragments: map[string]resourceFragment{},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
	}

	node := renderer.renderNode(map[string]interface{}{
		"$159": "$272",
		"$66":  100,
		"$67":  200,
	}, 0)
	got := renderHTMLPart(node)

	if !strings.Contains(got, "<svg") || !strings.Contains(got, "viewBox=\"0 0 100 200\"") || !strings.Contains(got, "preserveAspectRatio=\"xMidYMid meet\"") {
		t.Fatalf("svg html = %q", got)
	}
}

func TestMaterializeRenderedSectionsUsesDOMAfterLateMutation(t *testing.T) {
	section := renderedSection{
		Filename:  "section.xhtml",
		Title:     "Section",
		Language:  "en",
		BodyClass: "__STYLE_0__",
		Root: &htmlElement{
			Attrs: map[string]string{},
			Children: []htmlPart{
				&htmlElement{
					Tag:      "p",
					Attrs:    map[string]string{"class": "__STYLE_1__"},
					Children: []htmlPart{htmlText{Text: "Hello"}},
				},
			},
		},
	}
	replacer := strings.NewReplacer("__STYLE_0__", "body-class", "__STYLE_1__", "paragraph-class")
	replaceSectionDOMClassTokens(&section, replacer)
	section.BodyClass = replacer.Replace(section.BodyClass)

	out := materializeRenderedSections([]renderedSection{section})
	if len(out) != 1 {
		t.Fatalf("section count = %d", len(out))
	}
	if out[0].BodyClass != "body-class" {
		t.Fatalf("body class = %q", out[0].BodyClass)
	}
	if out[0].BodyHTML != `<p class="paragraph-class">Hello</p>` {
		t.Fatalf("body html = %q", out[0].BodyHTML)
	}
}

func TestRenderTextNodeSupportsRubyAnnotations(t *testing.T) {
	renderer := storylineRenderer{
		contentFragments: map[string][]string{"content": {"漢字", "かんじ"}},
		rubyGroups: map[string]map[string]interface{}{
			"ruby-set": {
				"$146": []interface{}{
					map[string]interface{}{
						"$758": 1,
						"$145": map[string]interface{}{"name": "content", "$403": 1},
					},
				},
			},
		},
		resourceHrefByID:  map[string]string{},
		resourceFragments: map[string]resourceFragment{},
		anchorToFilename:  map[string]string{},
		positionToSection: map[int]string{},
		positionAnchors:   map[int]map[int][]string{},
		positionAnchorID:  map[int]map[int]string{},
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    map[string]map[string]interface{}{},
		styles:            newStyleCatalog(),
	}

	node := renderer.renderTextNode(map[string]interface{}{
		"$145": map[string]interface{}{"name": "content", "$403": 0},
		"$142": []interface{}{
			map[string]interface{}{
				"$143": 0,
				"$144": 2,
				"$757": "ruby-set",
				"$758": 1,
			},
		},
	}, 0)
	got := renderHTMLPart(node)

	if !strings.Contains(got, "<ruby><rb>漢字</rb><rt>かんじ</rt></ruby>") {
		t.Fatalf("ruby html = %q", got)
	}
}

func TestCleanupRenderedSectionsCombinesNestedDivsAndStripsEmptySpans(t *testing.T) {
	sections := []renderedSection{
		{
			Root: &htmlElement{
				Attrs: map[string]string{},
				Children: []htmlPart{
					&htmlElement{
						Tag:   "div",
						Attrs: map[string]string{},
						Children: []htmlPart{
							&htmlElement{Tag: "span", Attrs: map[string]string{}},
							&htmlElement{
								Tag:   "div",
								Attrs: map[string]string{},
								Children: []htmlPart{
									&htmlElement{
										Tag:      "p",
										Attrs:    map[string]string{},
										Children: []htmlPart{htmlText{Text: "Hello"}},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	cleanupRenderedSections(sections)
	got := renderedSectionBodyHTML(sections[0])
	if got != "<div><p>Hello</p></div>" {
		t.Fatalf("cleaned body html = %q", got)
	}
}

func TestConvertFileMatchesReferenceStructureIgnoringImages(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
	output := filepath.Join(t.TempDir(), "martyr.epub")
	reference := filepath.Join("..", "..", "..", "REFERENCE", "martyr_calibre.epub")

	if err := ConvertFile(input, output); err != nil {
		t.Fatalf("ConvertFile() error = %v", err)
	}

	gotFiles := unzipFiles(t, output)
	wantFiles := unzipFiles(t, reference)
	gotNames := comparableArchiveNames(gotFiles)
	wantNames := comparableArchiveNames(wantFiles)
	if !equalStringSlices(gotNames, wantNames) {
		t.Fatalf("comparable archive names = %v, want %v", gotNames, wantNames)
	}

	for _, name := range gotNames {
		gotData := gotFiles[name]
		wantData := wantFiles[name]
		if isTextArchiveFile(name) {
			gotText := normalizeReferenceText(name, string(gotData))
			wantText := normalizeReferenceText(name, string(wantData))
			if gotText != wantText {
				t.Fatalf("%s text mismatch", name)
			}
			continue
		}
		if !bytes.Equal(gotData, wantData) {
			t.Fatalf("%s binary mismatch", name)
		}
	}

	gotImages := imageArchiveNames(gotFiles)
	wantImages := imageArchiveNames(wantFiles)
	if !equalStringSlices(gotImages, wantImages) {
		t.Fatalf("image names = %v, want %v", gotImages, wantImages)
	}
}

// Second non-Martyr gate: same non-image path list as Calibre for Three Below (Floors #2).
// Full byte-for-byte text parity vs calibre_epubs is still Phase D work (manifest/spine ordering, OPF metadata).
func TestConvertFileThreeBelowKFXZipMatchesCalibreComparableArchivePaths(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_new", "decrypted", "Three Below (Floors #2)_B008PL1YQ0_decrypted.kfx-zip")
	output := filepath.Join(t.TempDir(), "three-below.epub")
	reference := filepath.Join("..", "..", "..", "REFERENCE", "kfx_new", "calibre_epubs", "Three Below (Floors #2)_B008PL1YQ0_decrypted.epub")

	if err := ConvertFile(input, output); err != nil {
		t.Fatalf("ConvertFile() error = %v", err)
	}

	gotFiles := unzipFiles(t, output)
	wantFiles := unzipFiles(t, reference)
	gotNames := comparableArchiveNames(gotFiles)
	wantNames := comparableArchiveNames(wantFiles)
	if !equalStringSlices(gotNames, wantNames) {
		t.Fatalf("comparable archive names = %v, want %v", gotNames, wantNames)
	}
}

// Path-list gate for additional titles (same tier as Three Below). Full Martyr-style text+OPF parity
// for these fixtures remains Phase D until manifests align with Calibre.
func testConvertFileMatchesCalibreComparableArchivePathsWhenPresent(t *testing.T, input, reference string) {
	t.Helper()
	if _, err := os.Stat(input); err != nil {
		t.Skip("input not found:", input)
	}
	if _, err := os.Stat(reference); err != nil {
		t.Skip("reference epub not found:", reference)
	}
	output := filepath.Join(t.TempDir(), filepath.Base(input)+".epub")
	if err := ConvertFile(input, output); err != nil {
		t.Fatalf("ConvertFile() error = %v", err)
	}
	gotFiles := unzipFiles(t, output)
	wantFiles := unzipFiles(t, reference)
	gotNames := comparableArchiveNames(gotFiles)
	wantNames := comparableArchiveNames(wantFiles)
	if !equalStringSlices(gotNames, wantNames) {
		t.Fatalf("comparable archive names = %v, want %v", gotNames, wantNames)
	}
}

func TestConvertFileElvisKFXZipMatchesCalibreComparableArchivePathsWhenPresent(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_new", "decrypted", "Elvis and the Underdogs_B009NG3090_decrypted.kfx-zip")
	reference := filepath.Join("..", "..", "..", "REFERENCE", "kfx_new", "calibre_epubs", "Elvis and the Underdogs_B009NG3090_decrypted.epub")
	testConvertFileMatchesCalibreComparableArchivePathsWhenPresent(t, input, reference)
}

func TestConvertFileHungerGamesKFXZipMatchesCalibreComparableArchivePathsWhenPresent(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_new", "decrypted", "The Hunger Games Trilogy_B004XJRQUQ_decrypted.kfx-zip")
	reference := filepath.Join("..", "..", "..", "REFERENCE", "kfx_new", "calibre_epubs", "The Hunger Games Trilogy_B004XJRQUQ_decrypted.epub")
	testConvertFileMatchesCalibreComparableArchivePathsWhenPresent(t, input, reference)
}

func TestConvertFileFamiliarsKFXZipMatchesCalibreComparableArchivePathsWhenPresent(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_new", "decrypted", "The Familiars_B003VIWNQW_decrypted.kfx-zip")
	reference := filepath.Join("..", "..", "..", "REFERENCE", "kfx_new", "calibre_epubs", "The Familiars_B003VIWNQW_decrypted.epub")
	testConvertFileMatchesCalibreComparableArchivePathsWhenPresent(t, input, reference)
}

func referenceFragmentSnapshot(t *testing.T, input string) fragmentSnapshot {
	t.Helper()

	script := filepath.Join("..", "..", "scripts", "kfx_reference_snapshot.py")
	cmd := exec.Command("python", script, "fragment-summary", "--input", input)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("reference snapshot command failed: %v", err)
	}

	var snapshot fragmentSnapshot
	if err := json.Unmarshal(output, &snapshot); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return snapshot
}

func unzipFiles(t *testing.T, path string) map[string][]byte {
	t.Helper()

	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("zip.OpenReader(%s) error = %v", path, err)
	}
	defer archive.Close()

	files := map[string][]byte{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatalf("%s Open() error = %v", file.Name, err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatalf("%s ReadAll() error = %v", file.Name, err)
		}
		files[file.Name] = data
	}
	return files
}

func comparableArchiveNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		if isImageArchiveFile(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func imageArchiveNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		if isImageArchiveFile(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func isImageArchiveFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") || strings.HasSuffix(lower, ".png")
}

func isTextArchiveFile(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range []string{".css", ".html", ".ncx", ".opf", ".xhtml", ".xml"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

var modifiedMetaPattern = regexp.MustCompile(`<meta property="dcterms:modified">.*?</meta>`)

func normalizeReferenceText(name string, text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if strings.HasSuffix(strings.ToLower(name), "content.opf") {
		text = modifiedMetaPattern.ReplaceAllString(text, "")
	}
	return strings.TrimSpace(text)
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
