package kfx

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
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

// TestScribeNotebookPipelineMultiplePages verifies that page sections
// materialize in reading-order sequence (Python's book_parts are appended by
// new_book_part as process_reading_order walks the reading order,
// yj_to_epub_notebook.py:97-99) and that a page whose template fragment is
// missing still materializes (get_fragment logs an error and returns an empty
// fragment; the page keeps referencing only its own SVG).
func TestScribeNotebookPipelineMultiplePages(t *testing.T) {
	state := newScribeNotebookState(t)

	// A second page with no resolvable template reference.
	state.Fragments.SectionFragments["page-2"] = parseSectionFragment("page-2", map[string]interface{}{
		"section_name":        "page-2",
		"nmdl.canvas_width":   3906,
		"nmdl.canvas_height":  5208,
		"nmdl.normalized_ppi": 2520,
		"page_templates":      []interface{}{"pt-page"},
	})
	state.Fragments.SectionOrder = []string{"page-2", "page-1", "tpl-lined"}

	book, err := renderBookState(state, nil)
	if err != nil {
		t.Fatalf("renderBookState failed: %v", err)
	}

	if len(book.Sections) != 2 {
		t.Fatalf("expected 2 spine sections, got %d", len(book.Sections))
	}
	if book.Sections[0].Filename != "page-2.xhtml" || book.Sections[1].Filename != "page-1.xhtml" {
		t.Errorf("reading order not preserved: %s, %s",
			book.Sections[0].Filename, book.Sections[1].Filename)
	}
	if book.Sections[0].ViewportWidth != 3906 || book.Sections[0].ViewportHeight != 5208 {
		t.Errorf("page-2 viewport = %dx%d, want 3906x5208",
			book.Sections[0].ViewportWidth, book.Sections[0].ViewportHeight)
	}

	var sawPage2SVG, sawPage1SVG bool
	for _, res := range book.Resources {
		if res.Filename == "page-2.svg" {
			sawPage2SVG = true
		}
		if res.Filename == "page-1.svg" {
			sawPage1SVG = true
		}
	}
	if !sawPage1SVG || !sawPage2SVG {
		t.Errorf("missing per-page SVG resources (page-1=%v page-2=%v)", sawPage1SVG, sawPage2SVG)
	}
}

// TestScribeNotebookPipelineMissingTemplateFragment verifies the explicit
// behavior when a template section references a nonexistent $608 fragment:
// the template part produces no SVG resource and pages keep only the white
// rect + page image (Python: get_fragment error + "Failed to locate the SVG
// image within Scribe notebook template", yj_to_epub_notebook.py:201).
func TestScribeNotebookPipelineMissingTemplateFragment(t *testing.T) {
	state := newScribeNotebookState(t)
	delete(state.Fragments.RubyContents, "pt-tpl")

	book, err := renderBookState(state, nil)
	if err != nil {
		t.Fatalf("renderBookState failed: %v", err)
	}

	if len(book.Sections) != 1 {
		t.Fatalf("expected 1 spine section, got %d", len(book.Sections))
	}
	body := book.Sections[0].BodyHTML
	if strings.Contains(body, "lined.svg") {
		t.Errorf("page body must not reference a missing template SVG:\n%s", body)
	}
	for _, res := range book.Resources {
		if res.Filename == "lined.svg" {
			t.Errorf("lined.svg must not be produced from a missing template fragment")
		}
	}
}

// TestScribeResourceFilenameSanitization covers the resource_location_filename
// port (yj_to_epub_resources.py:249-285, kfxlib 20260822): sanitization,
// single-pass double-slash collapse, leading-slash prefixing, suffix-before-
// extension uniquification, and the (location, suffix) cache.
func TestScribeResourceFilenameSanitization(t *testing.T) {
	used := map[string]struct{}{}
	cache := map[string]string{}
	if got := scribeResourceFilename("page-1.svg", used, cache); got != "page-1.svg" {
		t.Errorf("plain name: got %q, want page-1.svg", got)
	}
	// scribeResourceFilename only READS oebps names; registration happens in
	// manifest_resource → add_oebps_file. Simulate that here.
	used["page-1.svg"] = struct{}{}
	if got := scribeResourceFilename("/res/page 2+.svg", used, cache); got != "_res/page_2_.svg" {
		t.Errorf("sanitized name: got %q", got)
	}
	// Python L281: the -N uniquification suffix goes between root and
	// extension (foo.svg → foo-0.svg, NOT foo.svg-0.svg); the candidate keeps
	// its original casing (only the collision check is case-insensitive).
	if got := scribeResourceFilename("PAGE-1.SVG", used, cache); got != "PAGE-1-0.SVG" {
		t.Errorf("duplicate name: got %q, want PAGE-1-0.SVG", got)
	}
	// A repeat of the original spelling hits the cache and returns the SAME
	// filename (Python caches (location, suffix) on first use), even though the
	// case-variant collision above took the -0 name.
	if got := scribeResourceFilename("page-1.svg", used, cache); got != "page-1.svg" {
		t.Errorf("cached repeat: got %q, want page-1.svg", got)
	}
}

// TestScribeResourceFilenameCacheRepeats verifies the location_filenames
// cache with EXACT upstream semantics: lookup uses the ORIGINAL location
// (yj_to_epub_resources.py:251-252) while the store uses the location AFTER
// leading-slash normalization (L254-255 rebind, L284 store). For a leading-
// "/" input a repeat therefore MISSES the cache and re-uniquifies — a real
// upstream asymmetry that is unreachable for notebook SVGs (names are
// "%s.svg" % section_name and never start with "/"). This test pins the
// source behavior rather than "fixing" it.
func TestScribeResourceFilenameCacheRepeats(t *testing.T) {
	used := map[string]struct{}{}
	cache := map[string]string{}

	// Plain (non-slash) locations: repeated requests hit the cache and return
	// the same name — even after the name has been registered in oebps
	// (Python consults the cache before the uniquify loop).
	first := scribeResourceFilename("note.svg", used, cache)
	if first != "note.svg" {
		t.Fatalf("first call: got %q, want note.svg", first)
	}
	used[strings.ToLower("note.svg")] = struct{}{} // add_oebps_file
	if second := scribeResourceFilename("note.svg", used, cache); second != "note.svg" {
		t.Errorf("cached repeat re-uniquified: got %q, want note.svg", second)
	}

	// Leading-slash locations: the store key is the NORMALIZED location, so
	// repeating the original spelling misses the cache. With the first result
	// registered in oebps, the repeat re-uniquifies to ...-0.svg — exactly what
	// Python produces (see test header for the line-level walkthrough).
	slashFirst := scribeResourceFilename("/tpl/lined.svg", used, cache)
	if slashFirst != "_tpl/lined.svg" {
		t.Fatalf("leading-slash first call: got %q, want _tpl/lined.svg", slashFirst)
	}
	used[strings.ToLower("_tpl/lined.svg")] = struct{}{}
	slashSecond := scribeResourceFilename("/tpl/lined.svg", used, cache)
	if slashSecond != "_tpl/lined-0.svg" {
		t.Errorf("leading-slash repeat: got %q, want _tpl/lined-0.svg (upstream asymmetry)", slashSecond)
	}
}

// TestScribeResourceFilenameTripleSlash pins Python's SINGLE-PASS
// ".replace('//', '/x/')" semantics: for 'a///b.svg' the residual '//' after
// the first replacement intentionally survives one pass, so the path becomes
// a/x// and the filename a/x//b.svg (verified against Python 3:
// 'a///b.svg'.replace('//', '/x/') == 'a/x//b.svg').
func TestScribeResourceFilenameTripleSlash(t *testing.T) {
	used := map[string]struct{}{}
	cache := map[string]string{}
	if got := scribeResourceFilename("a///b.svg", used, cache); got != "a/x//b.svg" {
		t.Errorf("triple slash: got %q, want a/x//b.svg", got)
	}
	if got := scribeResourceFilename("a//b//c.svg", used, cache); got != "a/x/b/x/c.svg" {
		t.Errorf("multiple double slashes: got %q, want a/x/b/x/c.svg", got)
	}
}

// TestSerializeSVGDocumentEscaping verifies XML escaping in
// serializeSVGDocument: lxml escapes &, <, > and " in attribute values and
// & < > in text nodes (quotes preserved). Raw output would be invalid XML for
// HWR text such as `A&B <x>`.
func TestSerializeSVGDocumentEscaping(t *testing.T) {
	root := &svgElement{
		Tag: "svg",
		Attrib: map[string]string{
			"xmlns":   "http://www.w3.org/2000/svg",
			"viewBox": "0 0 100 100",
		},
	}
	group := newSVGElement(root, "g", map[string]string{"id": `A&B <x> "q"`})
	desc := newSVGElement(group, "desc", nil)
	desc.Text = `A&B <x> "q"`

	got := string(serializeSVGDocument(root))
	for _, want := range []string{
		`id="A&amp;B &lt;x&gt; &quot;q&quot;"`,
		`A&amp;B &lt;x&gt; "q"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("serialized SVG missing %q:\n%s", want, got)
		}
	}
	// No raw specials may leak through unescaped.
	for _, bad := range []string{`id="A&B`, `>A&B`, `<x> "q"</desc>`} {
		if strings.Contains(got, bad) {
			t.Errorf("serialized SVG contains unescaped %q:\n%s", bad, got)
		}
	}
}

// TestScribeNotebookPipelineInlinePlacement covers the
// nmdl.inline_placement_type branch (yj_to_epub_notebook.py:146-150): the
// section's content properties are processed and replace the svg element's
// style instead of the default height/width 100% (L152-153).
func TestScribeNotebookPipelineInlinePlacement(t *testing.T) {
	state := newScribeNotebookState(t)
	// Rebuild the page section with inline placement and a fixed position.
	state.Fragments.SectionFragments["page-1"] = parseSectionFragment("page-1", map[string]interface{}{
		"section_name":               "page-1",
		"nmdl.canvas_width":          15624,
		"nmdl.canvas_height":         20832,
		"nmdl.normalized_ppi":        2520,
		"nmdl.template_id":           "tpl-lined",
		"nmdl.inline_placement_type": "yj.after", // $670
		"position":                   "fixed",
		"top":                        float64(100),
		"left":                       float64(200),
		"page_templates":             []interface{}{"pt-page"},
	})

	book, err := renderBookState(state, nil)
	if err != nil {
		t.Fatalf("renderBookState failed: %v", err)
	}
	if len(book.Sections) != 1 {
		t.Fatalf("expected 1 spine section, got %d", len(book.Sections))
	}
	body := book.Sections[0].BodyHTML
	// process_content_properties converts the YJ properties into CSS
	// declarations applied to the svg element with replace=True. KFX fixed
	// positioning maps to CSS absolute positioning in convert_yj_properties.
	for _, want := range []string{"position: absolute", "top: 100px", "left: 200px"} {
		if !strings.Contains(body, want) {
			t.Errorf("inline placement style missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `style="height: 100%; width: 100%"`) {
		t.Errorf("inline placement must replace the default height/width style:\n%s", body)
	}
}

// TestScribeNotebookEPUBWriterSmoke runs the final EPUB packaging over the
// synthetic notebook to verify the writer accepts the materialized sections
// and SVG resources (spine properties, xlink:href serialization, manifest).
func TestScribeNotebookEPUBWriterSmoke(t *testing.T) {
	state := newScribeNotebookState(t)
	book, err := renderBookState(state, nil)
	if err != nil {
		t.Fatalf("renderBookState failed: %v", err)
	}
	if len(book.Sections) == 0 {
		t.Fatal("no readable sections were extracted from the notebook")
	}

	outputPath := filepath.Join(t.TempDir(), "notebook.epub")
	// Same field mapping as the production writers (yj_to_epub.go ConvertFile
	// and friends) for the fields a notebook exercises.
	if err := epub.Write(outputPath, epub.Book{
		Identifier: book.Identifier,
		Title:      book.Title,
		Language:   book.Language,
		Sections:   book.Sections,
		Resources:  book.Resources,
	}); err != nil {
		t.Fatalf("epub write failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read epub: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty EPUB output")
	}
	// The EPUB is a zip: local file headers must contain the SVG resources.
	for _, want := range []string{"OEBPS/page-1.svg", "OEBPS/lined.svg", "OEBPS/page-1.xhtml"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("EPUB missing %s", want)
		}
	}
}

// TestScribeNotebookEPUBItemrefFXL asserts the production OPF semantics for a
// Scribe notebook end to end through decodedBookToEPUB + epub.Write:
//
//   - book.is_scribe_notebook forces fixed_layout = True
//     (yj_to_epub_metadata.py:171-182), so the OPF carries the book-level
//     rendition:layout pre-paginated metadata.
//   - Because the BOOK is fixed-layout, epub_output.py:1031-1037 strips the
//     per-entry rendition:layout-pre-paginated from FXL sections (redundant
//     with the book-level metadata) — the page itemref is therefore BARE.
//   - "svg" remains a manifest item property (epub_output.py:1040-1042).
//   - The fallback notebook title "Notebook <book_id> <date>" is applied since
//     no metadata title exists.
func TestScribeNotebookEPUBItemrefFXL(t *testing.T) {
	state := newScribeNotebookState(t)
	book, err := renderBookState(state, nil)
	if err != nil {
		t.Fatalf("renderBookState failed: %v", err)
	}

	if !book.FixedLayout {
		t.Fatal("Scribe notebook must force FixedLayout (yj_to_epub_metadata.py:179)")
	}
	if !strings.HasPrefix(book.Title, "Notebook ") {
		t.Errorf("fallback notebook title missing, got %q", book.Title)
	}

	// Production mapping (same helper ConvertFile uses).
	epubBook := decodedBookToEPUB(book)
	if !epubBook.FixedLayout || epubBook.BookType != "notebook" {
		t.Fatalf("mapping lost scribe semantics: FixedLayout=%v BookType=%q", epubBook.FixedLayout, epubBook.BookType)
	}

	outputPath := filepath.Join(t.TempDir(), "notebook.epub")
	if err := epub.Write(outputPath, epubBook); err != nil {
		t.Fatalf("epub write failed: %v", err)
	}

	zipFile, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("open epub: %v", err)
	}
	defer zipFile.Close()

	var opfData string
	for _, f := range zipFile.File {
		if f.Name == "OEBPS/content.opf" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open opf: %v", err)
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			opfData = string(data)
		}
	}
	if opfData == "" {
		t.Fatal("OEBPS/content.opf missing from EPUB")
	}

	// Book-level FXL metadata is present...
	if !strings.Contains(opfData, `<meta property="rendition:layout">pre-paginated</meta>`) {
		t.Errorf("OPF missing book-level rendition:layout metadata:\n%s", opfData)
	}
	// ...so the FXL page entry's per-entry pre-paginated is stripped
	// (epub_output.py:1031-1033) and its itemref is bare.
	if !strings.Contains(opfData, `<itemref idref="page-1.xhtml"/>`) {
		t.Errorf("FXL page itemref must be bare under book fixed-layout:\n%s", opfData)
	}
	if strings.Contains(opfData, `properties="rendition:layout-pre-paginated"`) {
		t.Errorf("per-entry pre-paginated must be stripped for FXL sections:\n%s", opfData)
	}
	// "svg" stays a MANIFEST property; it must not leak onto the itemref, and
	// rendition:* must not leak onto the manifest item.
	if strings.Contains(opfData, `<itemref idref="page-1.xhtml" properties="svg`) {
		t.Errorf("itemref carries manifest-only svg property:\n%s", opfData)
	}
	manifestProps := extractItemProperties(opfData, "page-1.xhtml")
	if manifestProps != "svg" {
		t.Errorf("manifest item properties = %q, want svg", manifestProps)
	}
}

// extractItemProperties returns the properties attribute of a manifest <item>
// with the given href ("" when absent).
func extractItemProperties(opfData, href string) string {
	start := strings.Index(opfData, `<item href="`+href+`"`)
	if start < 0 {
		return ""
	}
	end := strings.Index(opfData[start:], `/>`)
	if end < 0 {
		return ""
	}
	item := opfData[start : start+end]
	idx := strings.Index(item, `properties="`)
	if idx < 0 {
		return ""
	}
	rest := item[idx+len(`properties="`):]
	if quote := strings.Index(rest, `"`); quote >= 0 {
		return rest[:quote]
	}
	return ""
}

func extractOPFAttribute(opfData, _, _ string) string {
	// Return the properties attribute of the page-1 manifest item.
	start := strings.Index(opfData, `<item href="page-1.xhtml"`)
	if start < 0 {
		return ""
	}
	end := strings.Index(opfData[start:], `/>`)
	if end < 0 {
		return ""
	}
	item := opfData[start : start+end]
	idx := strings.Index(item, `properties="`)
	if idx < 0 {
		return ""
	}
	rest := item[idx+len(`properties="`):]
	if quote := strings.Index(rest, `"`); quote >= 0 {
		return rest[:quote]
	}
	return ""
}
