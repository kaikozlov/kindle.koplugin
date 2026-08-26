package kfx

import (
	"strings"
	"testing"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
)

// Integration tests for Scribe notebook production wiring.
//
// Python reference: KFX_EPUB_Notebook (yj_to_epub_notebook.py) invoked from
// KFX_EPUB_Content.process_section (yj_to_epub_content.py:136-140) and
// process_reading_order (yj_to_epub_content.py:105-113). These tests push a
// synthetic Scribe notebook (one page section + one template section) through
// the real renderBookState pipeline — not just helper calls — and verify the
// page becomes an EPUB section referencing materialized SVG resources.

// encodeStrokeValuesFixture builds binary stroke value data: signature 0x0101,
// uint32 LE count, instruction nibbles, then increment bytes.
// Produces the values [0, delta] with the minimal instruction encoding.
func encodeStrokeValuesFixture(delta int) []byte {
	if delta < 0 || delta > 0xFF {
		panic("fixture only supports single-byte positive deltas")
	}
	return []byte{
		0x01, 0x01, // signature
		0x02, 0x00, 0x00, 0x00, // num_vals = 2
		0x01,        // instruction nibbles: [0 (n=0, zero increment), 1 (n=1, read 1 byte)]
		byte(delta), // increment for the second value
	}
}

// newScribeNotebookState builds a bookState for a synthetic Scribe notebook:
//
//   - section "page-1" (nmdl.canvas_width) whose $608 template resolves a
//     storyline containing one pen stroke
//   - section "tpl-lined" (nmdl.template_type) whose $608 template resolves a
//     storyline containing one KVG shape (rendered as SVG)
//   - two reading orders ($351 pages + note_template_collection templates),
//     matching yj_structure.py:368-373 validation shape
//   - document_data carrying nmdl.template_id (yj_to_epub_metadata.py:91)
func newScribeNotebookState(t *testing.T) *bookState {
	t.Helper()

	stroke := map[string]interface{}{
		"type":               "container",
		"nmdl.type":          "nmdl.stroke",
		"nmdl.brush_type":    0, // original pen
		"nmdl.color":         0, // black
		"nmdl.thickness":     23.625,
		"nmdl.stroke_bounds": []interface{}{0, 0, 100, 100},
		"nmdl.stroke_points": map[string]interface{}{
			"nmdl.num_points": 2,
			"nmdl.position_x": encodeStrokeValuesFixture(10),
			"nmdl.position_y": encodeStrokeValuesFixture(0),
		},
	}

	// $608 structure fragments referenced (as IonSymbols) from $141 page_templates.
	rubyContents := map[string]map[string]interface{}{
		"pt-page": {"type": "container", "story_name": "page-story"},
		"pt-tpl":  {"type": "container", "story_name": "tpl-story"},
	}

	storylines := map[string]map[string]interface{}{
		"page-story": {"story_name": "page-story", "content_list": []interface{}{stroke}},
		"tpl-story": {
			"story_name": "page-template-story",
			"content_list": []interface{}{
				map[string]interface{}{
					"type":         "kvg",
					"fixed_width":  15624,
					"fixed_height": 20832,
					"shape_list": []interface{}{
						map[string]interface{}{
							"type": "line",
							// KFX path instructions: 0=M(x,y), 1=L(x,y) (processPath).
							"path": []interface{}{float64(0), float64(0), float64(0), float64(1), float64(15624), float64(20832)},
						},
					},
				},
			},
		},
	}

	// Section fragments in production shape: $141 entries are IonSymbols, so
	// parseSectionFragment stores the raw section dict and no parsed templates.
	pageSection := parseSectionFragment("page-1", map[string]interface{}{
		"section_name":        "page-1",
		"nmdl.canvas_width":   15624,
		"nmdl.canvas_height":  20832,
		"nmdl.normalized_ppi": 2520,
		"nmdl.template_id":    "tpl-lined",
		"page_templates":      []interface{}{"pt-page"},
	})
	templateSection := parseSectionFragment("tpl-lined", map[string]interface{}{
		"section_name":       "tpl-lined",
		"nmdl.template_type": "lined",
		"page_templates":     []interface{}{"pt-tpl"},
	})

	fragments := fragmentCatalog{
		ContentFeatures: map[string]interface{}{},
		DocumentData: map[string]interface{}{
			"nmdl.template_id": "tpl-lined",
			"reading_orders": []interface{}{
				map[string]interface{}{"reading_order_name": "$351", "sections": []interface{}{"page-1"}},
				map[string]interface{}{"reading_order_name": "note_template_collection", "sections": []interface{}{"tpl-lined"}},
			},
		},
		ContentFragments: map[string][]string{},
		Storylines:       storylines,
		StyleFragments:   map[string]map[string]interface{}{},
		RubyGroups:       map[string]map[string]interface{}{},
		RubyContents:     rubyContents,
		SectionFragments: map[string]sectionFragment{
			"page-1":    pageSection,
			"tpl-lined": templateSection,
		},
		AnchorFragments:    map[string]anchorFragment{},
		NavContainers:      map[string]map[string]interface{}{},
		ResourceFragments:  map[string]resourceFragment{},
		ResourceRawData:    map[string]map[string]interface{}{},
		FormatCapabilities: map[string]map[string]interface{}{},
		Generators:         map[string]map[string]interface{}{},
		PathBundles:        map[string]map[string]interface{}{},
		FontFragments:      map[string]fontFragment{},
		RawFragments:       map[string][]byte{},
		PositionAliases:    map[int]string{},
		FragmentIDsByType:  map[string][]string{},
		SectionOrder:       []string{"page-1", "tpl-lined"},
	}

	book := &decodedBook{Identifier: "scribe-test", Language: "en"}
	// Mirror organizeFragments (yj_book.go:581): metadata application happens
	// before renderBookState in the production pipeline.
	applyKFXEPUBInitMetadataAfterOrganize(book, &fragments)

	return &bookState{
		Path:             "scribe-test",
		Book:             book,
		Fragments:        fragments,
		BookSymbolFormat: symOriginal,
	}
}

// TestScribeNotebookPipelineEndToEnd drives the full conversion pipeline with a
// synthetic Scribe notebook and verifies Python KFX_EPUB_Notebook behavior:
//
//   - process_reading_order (yj_to_epub_content.py:105-113) walks BOTH reading
//     orders, so the page section and the template section are processed.
//   - process_scribe_notebook_page_section (yj_to_epub_notebook.py:78-156)
//     materializes the page as an FXL XHTML book part whose body SVG references
//     a serialized page-1.svg manifest resource.
//   - process_scribe_notebook_template_section (yj_to_epub_notebook.py:158-218)
//     serializes the template SVG as lined.svg, omits its own book part from
//     the spine, and inserts a template image into the page SVG (position 1).
func TestScribeNotebookPipelineEndToEnd(t *testing.T) {
	state := newScribeNotebookState(t)
	book, err := renderBookState(state, nil)
	if err != nil {
		t.Fatalf("renderBookState failed: %v", err)
	}

	if len(book.Sections) != 1 {
		t.Fatalf("expected exactly 1 spine section (page only; template omitted), got %d: %#v",
			len(book.Sections), book.Sections)
	}
	page := book.Sections[0]
	if page.Filename != "page-1.xhtml" {
		t.Errorf("page section filename = %q, want %q", page.Filename, "page-1.xhtml")
	}
	if page.ViewportWidth != 15624 || page.ViewportHeight != 20832 {
		t.Errorf("page viewport = %dx%d, want 15624x20832 (Python add_meta_name_content viewport)",
			page.ViewportWidth, page.ViewportHeight)
	}
	if !strings.Contains(page.Properties, "svg") {
		t.Errorf("page section properties = %q, want it to contain \"svg\" (epub_output.py:705-706)", page.Properties)
	}

	body := page.BodyHTML
	for _, want := range []string{
		`<svg`,
		`viewBox="0 0 15624 20832"`,
		`fill="white"`,
		`xlink:href="lined.svg"`,
		`xlink:href="page-1.svg"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page body missing %q\nbody: %s", want, body)
		}
	}
	// Python yj_to_epub_notebook.py:191-195 inserts the template image at
	// position 1 (between the white rect and the page image).
	if strings.Index(body, `xlink:href="lined.svg"`) > strings.Index(body, `xlink:href="page-1.svg"`) {
		t.Errorf("template image must precede page image in page SVG:\n%s", body)
	}

	resources := map[string]epubResourceView{}
	for _, res := range book.Resources {
		resources[res.Filename] = epubResourceView{mediaType: res.MediaType, data: string(res.Data)}
	}

	pageSVG, ok := resources["page-1.svg"]
	if !ok {
		t.Fatalf("page-1.svg resource missing; resources: %v", resourceNames(book.Resources))
	}
	if pageSVG.mediaType != "image/svg+xml" {
		t.Errorf("page-1.svg media type = %q, want image/svg+xml", pageSVG.mediaType)
	}
	for _, want := range []string{
		`<!DOCTYPE svg`,
		`viewBox="0 0 15624 20832"`,
		`stroke-width="24"`,
		`d="M 0 0 L 10 0"`,
	} {
		if !strings.Contains(pageSVG.data, want) {
			t.Errorf("page-1.svg missing %q:\n%s", want, pageSVG.data)
		}
	}

	templateSVG, ok := resources["lined.svg"]
	if !ok {
		t.Fatalf("lined.svg resource missing; resources: %v", resourceNames(book.Resources))
	}
	for _, want := range []string{
		`<!DOCTYPE svg`,
		`viewBox="0 0 15624 20832"`,
		`<path`,
		`L 15624 20832`,
	} {
		if !strings.Contains(templateSVG.data, want) {
			t.Errorf("lined.svg missing %q:\n%s", want, templateSVG.data)
		}
	}

	// Python add_style on the html svg element (yj_to_epub_notebook.py:152-153)
	// sets height/width 100% on the SVG inside the body.
	if !strings.Contains(body, `style="height: 100%; width: 100%"`) {
		t.Errorf("page body svg missing height/width 100%% style:\n%s", body)
	}
}

type epubResourceView struct {
	mediaType string
	data      string
}

func resourceNames(resources []epub.Resource) []string {
	names := make([]string, 0, len(resources))
	for _, res := range resources {
		names = append(names, res.Filename)
	}
	return names
}
