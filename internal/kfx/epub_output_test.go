package kfx

import (
	"testing"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
)

// ---------------------------------------------------------------------------
// Tests for consolidateHTML — Python epub_output.py L742-775
// ---------------------------------------------------------------------------

// TestConsolidateHTMLMergeAdjacentSpans verifies that adjacent <span> elements
// with identical attributes are merged into a single element.
// Python: epub_output.py L742-770 — consolidate_html merges adjacent inline elements
// with matching tags and identical attributes.
func TestConsolidateHTMLMergeAdjacentSpans(t *testing.T) {
	// <div><span class="x">hello</span><span class="x"> world</span></div>
	// should become <div><span class="x">hello world</span></div>
	parent := &htmlElement{
		Tag: "div",
		Children: []htmlPart{
			&htmlElement{
				Tag:   "span",
				Attrs: map[string]string{"class": "x"},
				Children: []htmlPart{
					htmlText{Text: "hello"},
				},
			},
			&htmlElement{
				Tag:   "span",
				Attrs: map[string]string{"class": "x"},
				Children: []htmlPart{
					htmlText{Text: " world"},
				},
			},
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{parent},
	}

	consolidateHTML(body)

	// Should have exactly one span now
	spans := collectElements(parent, "span")
	if len(spans) != 1 {
		t.Fatalf("expected 1 span after consolidation, got %d", len(spans))
	}

	text := elementText(spans[0])
	if text != "hello world" {
		t.Fatalf("expected merged text 'hello world', got %q", text)
	}
}

// TestConsolidateHTMLMergeAdjacentLinks verifies that adjacent <a> elements
// with identical href are merged.
func TestConsolidateHTMLMergeAdjacentLinks(t *testing.T) {
	// <div><a href="#x">part1</a><a href="#x">part2</a></div>
	parent := &htmlElement{
		Tag: "div",
		Children: []htmlPart{
			&htmlElement{
				Tag:   "a",
				Attrs: map[string]string{"href": "#x"},
				Children: []htmlPart{
					htmlText{Text: "part1"},
				},
			},
			&htmlElement{
				Tag:   "a",
				Attrs: map[string]string{"href": "#x"},
				Children: []htmlPart{
					htmlText{Text: "part2"},
				},
			},
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{parent},
	}

	consolidateHTML(body)

	links := collectElements(parent, "a")
	if len(links) != 1 {
		t.Fatalf("expected 1 <a> after consolidation, got %d", len(links))
	}

	text := elementText(links[0])
	if text != "part1part2" {
		t.Fatalf("expected merged text 'part1part2', got %q", text)
	}
}

// TestConsolidateHTMLNoMergeDifferentAttrs verifies that elements with different
// attributes are NOT merged.
func TestConsolidateHTMLNoMergeDifferentAttrs(t *testing.T) {
	parent := &htmlElement{
		Tag: "div",
		Children: []htmlPart{
			&htmlElement{
				Tag:   "span",
				Attrs: map[string]string{"class": "x"},
				Children: []htmlPart{
					htmlText{Text: "hello"},
				},
			},
			&htmlElement{
				Tag:   "span",
				Attrs: map[string]string{"class": "y"},
				Children: []htmlPart{
					htmlText{Text: " world"},
				},
			},
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{parent},
	}

	consolidateHTML(body)

	spans := collectElements(parent, "span")
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans (different classes), got %d", len(spans))
	}
}

// TestConsolidateHTMLMergeWithChildren verifies that merging works when elements
// have child elements (not just text). Children from the next sibling are moved
// into the first element.
// Python: epub_output.py L759-762 — moves all children from n to e.
func TestConsolidateHTMLMergeWithChildren(t *testing.T) {
	// <div><b><em>A</em></b><b><em>B</em></b></div>
	// After merge: <b><em>A</em><em>B</em></b> — but then consolidateHTML's
	// recursive pass also merges the two adjacent <em> elements (same tag, no attrs).
	// Final result: <b><em>AB</em></b>
	parent := &htmlElement{
		Tag: "div",
		Children: []htmlPart{
			&htmlElement{
				Tag:   "b",
				Attrs: map[string]string{},
				Children: []htmlPart{
					&htmlElement{
						Tag:      "em",
						Children: []htmlPart{htmlText{Text: "A"}},
					},
				},
			},
			&htmlElement{
				Tag:   "b",
				Attrs: map[string]string{},
				Children: []htmlPart{
					&htmlElement{
						Tag:      "em",
						Children: []htmlPart{htmlText{Text: "B"}},
					},
				},
			},
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{parent},
	}

	consolidateHTML(body)

	bolds := collectElements(parent, "b")
	if len(bolds) != 1 {
		t.Fatalf("expected 1 <b> after consolidation, got %d", len(bolds))
	}

	// The two <em> elements inside <b> also get merged (same tag, no attrs)
	ems := collectElements(bolds[0], "em")
	if len(ems) != 1 {
		t.Fatalf("expected 1 <em> (merged recursively), got %d", len(ems))
	}
	emText := elementText(ems[0])
	if emText != "AB" {
		t.Fatalf("expected merged em text 'AB', got %q", emText)
	}
}

// TestConsolidateHTMLMergeThreeAdjacent verifies that 3+ adjacent matching elements
// are all merged. The outer while loop retries until no more merges are possible.
func TestConsolidateHTMLMergeThreeAdjacent(t *testing.T) {
	parent := &htmlElement{
		Tag: "div",
		Children: []htmlPart{
			&htmlElement{
				Tag:   "i",
				Attrs: map[string]string{},
				Children: []htmlPart{
					htmlText{Text: "A"},
				},
			},
			&htmlElement{
				Tag:   "i",
				Attrs: map[string]string{},
				Children: []htmlPart{
					htmlText{Text: "B"},
				},
			},
			&htmlElement{
				Tag:   "i",
				Attrs: map[string]string{},
				Children: []htmlPart{
					htmlText{Text: "C"},
				},
			},
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{parent},
	}

	consolidateHTML(body)

 italics := collectElements(parent, "i")
	if len(italics) != 1 {
		t.Fatalf("expected 1 <i> after consolidation, got %d", len(italics))
	}

	text := elementText(italics[0])
	if text != "ABC" {
		t.Fatalf("expected merged text 'ABC', got %q", text)
	}
}

// TestConsolidateHTMLStripEmptySpanNoAttrs verifies that empty <span> elements
// with no attributes are stripped.
// Python: epub_output.py L773-775 — strips spans with no attrs using TEMP_TAG.
func TestConsolidateHTMLStripEmptySpanNoAttrs(t *testing.T) {
	parent := &htmlElement{
		Tag: "div",
		Children: []htmlPart{
			htmlText{Text: "before"},
			&htmlElement{
				Tag:      "span",
				Attrs:    map[string]string{},
				Children: []htmlPart{},
			},
			htmlText{Text: "after"},
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{parent},
	}

	consolidateHTML(body)

	// The empty span should be removed; only text nodes remain
	for _, child := range parent.Children {
		if el, ok := child.(*htmlElement); ok {
			if el.Tag == "span" && len(el.Attrs) == 0 && len(el.Children) == 0 {
				t.Fatal("empty span with no attrs should have been stripped")
			}
		}
	}
}

// TestConsolidateHTMLNoMergeTailText verifies that elements with tail text
// (text after the closing tag) are NOT merged with their next sibling.
// Python: epub_output.py L750 — condition `not e.tail` prevents merge.
func TestConsolidateHTMLNoMergeTailText(t *testing.T) {
	// In our model, tail text is represented as a text node after the element.
	// <div><span class="x">A</span>"gap"<span class="x">B</span></div>
	// The two spans should NOT be merged because there's text between them.
	parent := &htmlElement{
		Tag: "div",
		Children: []htmlPart{
			&htmlElement{
				Tag:   "span",
				Attrs: map[string]string{"class": "x"},
				Children: []htmlPart{
					htmlText{Text: "A"},
				},
			},
			htmlText{Text: "gap"},
			&htmlElement{
				Tag:   "span",
				Attrs: map[string]string{"class": "x"},
				Children: []htmlPart{
					htmlText{Text: "B"},
				},
			},
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{parent},
	}

	consolidateHTML(body)

	spans := collectElements(parent, "span")
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans (tail text between them), got %d", len(spans))
	}
}

// TestConsolidateHTMLNoMergeDifferentTags verifies that elements with different
// tags are NOT merged even if they have identical attributes.
func TestConsolidateHTMLNoMergeDifferentTags(t *testing.T) {
	parent := &htmlElement{
		Tag: "div",
		Children: []htmlPart{
			&htmlElement{
				Tag:   "b",
				Attrs: map[string]string{"class": "x"},
				Children: []htmlPart{
					htmlText{Text: "A"},
				},
			},
			&htmlElement{
				Tag:   "i",
				Attrs: map[string]string{"class": "x"},
				Children: []htmlPart{
					htmlText{Text: "B"},
				},
			},
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{parent},
	}

	consolidateHTML(body)

	bolds := collectElements(parent, "b")
	italics := collectElements(parent, "i")
	if len(bolds) != 1 || len(italics) != 1 {
		t.Fatalf("expected 1 <b> and 1 <i>, got %d <b> and %d <i>", len(bolds), len(italics))
	}
}

// TestConsolidateHTMLMergePreservesLastChildTail verifies that when merging
// elements where the first element has child elements, the next element's text
// is appended after the last child of the first element (as "tail" text).
// Python: epub_output.py L754-757 — `tt = e[-1]; tt.tail = ...`
//
// In Go's model, Python's "tail" of e[-1] becomes a text node after that child.
// So <b><em>A</em></b><b>tail</b> becomes <b><em>A</em>tail</b>
// (the text "tail" is a sibling of <em> inside <b>, not inside <em>).
func TestConsolidateHTMLMergePreservesLastChildTail(t *testing.T) {
	parent := &htmlElement{
		Tag: "div",
		Children: []htmlPart{
			&htmlElement{
				Tag:   "b",
				Attrs: map[string]string{},
				Children: []htmlPart{
					&htmlElement{
						Tag:      "em",
						Children: []htmlPart{htmlText{Text: "A"}},
					},
				},
			},
			&htmlElement{
				Tag:   "b",
				Attrs: map[string]string{},
				Children: []htmlPart{
					htmlText{Text: "tail"},
				},
			},
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{parent},
	}

	consolidateHTML(body)

	bolds := collectElements(parent, "b")
	if len(bolds) != 1 {
		t.Fatalf("expected 1 <b>, got %d", len(bolds))
	}

	// <b> should have: <em>A</em>, "tail" (text node after em, not inside em)
	if len(bolds[0].Children) != 2 {
		t.Fatalf("expected 2 children in <b> (<em> + text), got %d", len(bolds[0].Children))
	}

	ems := collectElements(bolds[0], "em")
	if len(ems) != 1 {
		t.Fatalf("expected 1 <em>, got %d", len(ems))
	}
	emText := elementText(ems[0])
	if emText != "A" {
		t.Fatalf("expected em text 'A', got %q", emText)
	}

	// The tail text should be the second child
	if txt, ok := bolds[0].Children[1].(htmlText); !ok || txt.Text != "tail" {
		t.Fatalf("expected tail text 'tail' after <em>, got %v", bolds[0].Children[1])
	}
}

// TestConsolidateHTMLMergePreservesTailAfterMerge verifies that after merging
// two adjacent elements, the original tail of the second element is preserved
// on the merged result.
// Python: epub_output.py L764 — `if n.tail: e.tail = n.tail`
func TestConsolidateHTMLMergePreservesTailAfterMerge(t *testing.T) {
	// <div><span class="x">A</span><span class="x">B</span>tailText</div>
	// After merge: <div><span class="x">AB</span>tailText</div>
	parent := &htmlElement{
		Tag: "div",
		Children: []htmlPart{
			&htmlElement{
				Tag:   "span",
				Attrs: map[string]string{"class": "x"},
				Children: []htmlPart{
					htmlText{Text: "A"},
				},
			},
			&htmlElement{
				Tag:   "span",
				Attrs: map[string]string{"class": "x"},
				Children: []htmlPart{
					htmlText{Text: "B"},
				},
			},
			htmlText{Text: "tailText"},
		},
	}

	body := &htmlElement{
		Tag:      "body",
		Children: []htmlPart{parent},
	}

	consolidateHTML(body)

	spans := collectElements(parent, "span")
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// The tail text should still be present after the span
	text := elementText(spans[0])
	if text != "AB" {
		t.Fatalf("expected 'AB', got %q", text)
	}

	// Check that the tail text is preserved after the merged span
	if len(parent.Children) != 2 {
		t.Fatalf("expected 2 children (span + tail), got %d", len(parent.Children))
	}
	if txt, ok := parent.Children[1].(htmlText); !ok || txt.Text != "tailText" {
		t.Fatalf("expected tail text 'tailText', got %v", parent.Children[1])
	}
}

// ---------------------------------------------------------------------------
// Helper functions for tests
// ---------------------------------------------------------------------------

// collectElements collects all descendant elements with the given tag.
func collectElements(parent *htmlElement, tag string) []*htmlElement {
	var result []*htmlElement
	var walk func(elem *htmlElement)
	walk = func(elem *htmlElement) {
		for _, child := range elem.Children {
			if el, ok := child.(*htmlElement); ok {
				if el.Tag == tag {
					result = append(result, el)
				}
				walk(el)
			}
		}
	}
	walk(parent)
	return result
}

// elementText returns the concatenated text content of an element (depth-first).
func elementText(elem *htmlElement) string {
	var out string
	for _, child := range elem.Children {
		switch c := child.(type) {
		case htmlText:
			out += c.Text
		case *htmlText:
			out += c.Text
		case *htmlElement:
			out += elementText(c)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Tests for compareFixedLayoutViewports — Python epub_output.py L614-653
// ---------------------------------------------------------------------------

// TestCompareFixedLayoutViewports_NonFixedLayout is a no-op when FixedLayout is false.
func TestCompareFixedLayoutViewports_NonFixedLayout(t *testing.T) {
	book := &decodedBook{
		FixedLayout: false,
		RenderedSections: []renderedSection{
			{Filename: "test.xhtml", Root: &htmlElement{
				Tag: "html",
				Children: []htmlPart{
					&htmlElement{Tag: "meta", Attrs: map[string]string{"name": "viewport", "content": "width=800, height=600"}},
				},
			}},
		},
	}
	compareFixedLayoutViewports(book)
	if book.OriginalWidth != 0 || book.OriginalHeight != 0 {
		t.Errorf("expected no viewport detection for non-fixed-layout, got %dx%d", book.OriginalWidth, book.OriginalHeight)
	}
}

// TestCompareFixedLayoutViewports_SingleViewport sets OriginalWidth/Height from the most common viewport.
func TestCompareFixedLayoutViewports_SingleViewport(t *testing.T) {
	book := &decodedBook{
		FixedLayout: true,
		RenderedSections: []renderedSection{
			{Filename: "sec1.xhtml", Root: &htmlElement{
				Tag: "html",
				Children: []htmlPart{
					&htmlElement{Tag: "head", Children: []htmlPart{
						&htmlElement{Tag: "meta", Attrs: map[string]string{"name": "viewport", "content": "width=1024, height=768"}},
					}},
				},
			}},
			{Filename: "sec2.xhtml", Root: &htmlElement{
				Tag: "html",
				Children: []htmlPart{
					&htmlElement{Tag: "head", Children: []htmlPart{
						&htmlElement{Tag: "meta", Attrs: map[string]string{"name": "viewport", "content": "width=1024, height=768"}},
					}},
				},
			}},
		},
	}
	compareFixedLayoutViewports(book)
	if book.OriginalWidth != 1024 || book.OriginalHeight != 768 {
		t.Errorf("expected 1024x768, got %dx%d", book.OriginalWidth, book.OriginalHeight)
	}
}

// TestCompareFixedLayoutViewports_MultipleViewports selects the most common viewport.
func TestCompareFixedLayoutViewports_MultipleViewports(t *testing.T) {
	book := &decodedBook{
		FixedLayout: true,
		RenderedSections: []renderedSection{
			{Filename: "sec1.xhtml", Root: &htmlElement{
				Tag: "html",
				Children: []htmlPart{
					&htmlElement{Tag: "meta", Attrs: map[string]string{"name": "viewport", "content": "width=800, height=600"}},
				},
			}},
			{Filename: "sec2.xhtml", Root: &htmlElement{
				Tag: "html",
				Children: []htmlPart{
					&htmlElement{Tag: "meta", Attrs: map[string]string{"name": "viewport", "content": "width=1024, height=768"}},
				},
			}},
			{Filename: "sec3.xhtml", Root: &htmlElement{
				Tag: "html",
				Children: []htmlPart{
					&htmlElement{Tag: "meta", Attrs: map[string]string{"name": "viewport", "content": "width=1024, height=768"}},
				},
			}},
		},
	}
	compareFixedLayoutViewports(book)
	// 1024x768 appears twice, 800x600 appears once → 1024x768 wins
	if book.OriginalWidth != 1024 || book.OriginalHeight != 768 {
		t.Errorf("expected 1024x768 (most common), got %dx%d", book.OriginalWidth, book.OriginalHeight)
	}
}

// TestCompareFixedLayoutViewports_SmallViewportWarns logs a warning for viewports < 100px.
func TestCompareFixedLayoutViewports_SmallViewportWarns(t *testing.T) {
	book := &decodedBook{
		FixedLayout: true,
		RenderedSections: []renderedSection{
			{Filename: "tiny.xhtml", Root: &htmlElement{
				Tag: "html",
				Children: []htmlPart{
					&htmlElement{Tag: "meta", Attrs: map[string]string{"name": "viewport", "content": "width=50, height=50"}},
				},
			}},
		},
	}
	compareFixedLayoutViewports(book)
	// Should still set dimensions even for small viewports
	if book.OriginalWidth != 50 || book.OriginalHeight != 50 {
		t.Errorf("expected 50x50, got %dx%d", book.OriginalWidth, book.OriginalHeight)
	}
}

// TestCompareFixedLayoutViewports_NoViewports does nothing when no viewport metas found.
func TestCompareFixedLayoutViewports_NoViewports(t *testing.T) {
	book := &decodedBook{
		FixedLayout: true,
		RenderedSections: []renderedSection{
			{Filename: "sec1.xhtml", Root: &htmlElement{
				Tag: "html",
				Children: []htmlPart{
					&htmlElement{Tag: "p", Children: []htmlPart{htmlText{Text: "Hello"}}},
				},
			}},
		},
	}
	compareFixedLayoutViewports(book)
	if book.OriginalWidth != 0 || book.OriginalHeight != 0 {
		t.Errorf("expected no viewport detection, got %dx%d", book.OriginalWidth, book.OriginalHeight)
	}
}

// TestCompareFixedLayoutViewports_NilBook does not panic on nil book.
func TestCompareFixedLayoutViewports_NilBook(t *testing.T) {
	compareFixedLayoutViewports(nil)
}

// TestCompareFixedLayoutViewports_NilRoot skips sections with nil Root.
func TestCompareFixedLayoutViewports_NilRoot(t *testing.T) {
	book := &decodedBook{
		FixedLayout: true,
		RenderedSections: []renderedSection{
			{Filename: "sec1.xhtml"},
			{Filename: "sec2.xhtml", Root: &htmlElement{
				Tag: "html",
				Children: []htmlPart{
					&htmlElement{Tag: "meta", Attrs: map[string]string{"name": "viewport", "content": "width=800, height=1200"}},
				},
			}},
		},
	}
	compareFixedLayoutViewports(book)
	if book.OriginalWidth != 800 || book.OriginalHeight != 1200 {
		t.Errorf("expected 800x1200, got %dx%d", book.OriginalWidth, book.OriginalHeight)
	}
}

// TestAspectRatioMatch verifies the 1.5% tolerance.
func TestAspectRatioMatch(t *testing.T) {
	// Exact match
	if !aspectRatioMatch(1.5, 1.5) {
		t.Error("expected exact match")
	}
	// Within 1.5%
	if !aspectRatioMatch(1.0, 1.01) {
		t.Error("expected 1% difference to match")
	}
	// Outside 1.5%
	if aspectRatioMatch(1.0, 1.02) {
		t.Error("expected 2% difference NOT to match")
	}
	// Zero ratio
	if !aspectRatioMatch(0, 0) {
		t.Error("expected 0/0 to match")
	}
	if aspectRatioMatch(0, 1) {
		t.Error("expected 0/1 NOT to match")
	}
}

// ---------------------------------------------------------------------------
// Tests for checkEpubVersion — Python epub_output.py L654-684
// ---------------------------------------------------------------------------

// TestCheckEpubVersion_NotDesired returns false immediately when EPUB2 not desired.
// Python L655: if not self.generate_epub2: return
func TestCheckEpubVersion_NotDesired(t *testing.T) {
	book := &decodedBook{FixedLayout: true}
	result := checkEpubVersion(false, book, nil, nil)
	if result {
		t.Error("expected false (EPUB3) when epub2Desired is false")
	}
}

// TestCheckEpubVersion_FixedLayoutRequiresEPUB3 verifies that fixed-layout
// books require EPUB3.
// Python L657: if self.fixed_layout: self.generate_epub2 = False
func TestCheckEpubVersion_FixedLayoutRequiresEPUB3(t *testing.T) {
	book := &decodedBook{FixedLayout: true}
	result := checkEpubVersion(true, book, nil, nil)
	if result {
		t.Error("expected false (EPUB3 required) for fixed-layout book")
	}
}

// TestCheckEpubVersion_AuthorPronunciationsRequiresEPUB3 verifies that books
// with author pronunciations require EPUB3.
// Python L657: if self.author_pronunciations: self.generate_epub2 = False
func TestCheckEpubVersion_AuthorPronunciationsRequiresEPUB3(t *testing.T) {
	book := &decodedBook{AuthorPronunciations: []string{"pron.mp3"}}
	result := checkEpubVersion(true, book, nil, nil)
	if result {
		t.Error("expected false (EPUB3 required) for book with author pronunciations")
	}
}

// TestCheckEpubVersion_TitlePronunciationRequiresEPUB3 verifies that books
// with title pronunciation require EPUB3.
// Python L657: if self.title_pronunciation: self.generate_epub2 = False
func TestCheckEpubVersion_TitlePronunciationRequiresEPUB3(t *testing.T) {
	book := &decodedBook{TitlePronunciation: "title_pron.mp3"}
	result := checkEpubVersion(true, book, nil, nil)
	if result {
		t.Error("expected false (EPUB3 required) for book with title pronunciation")
	}
}

// TestCheckEpubVersion_EPUB3MimetypeRequiresEPUB3 verifies that resources with
// EPUB3-only mimetypes force EPUB3.
// Python L661-663: if oebps_file.mimetype in ["application/octet-stream", ...]
func TestCheckEpubVersion_EPUB3MimetypeRequiresEPUB3(t *testing.T) {
	for _, mt := range []string{
		"application/octet-stream",
		"application/xml",
		"text/javascript",
		"text/html",
	} {
		book := &decodedBook{}
		resources := []epub.Resource{{Filename: "test", MediaType: mt}}
		result := checkEpubVersion(true, book, nil, resources)
		if result {
			t.Errorf("expected false (EPUB3 required) for mimetype %q", mt)
		}
	}
}

// TestCheckEpubVersion_NavSectionRequiresEPUB3 verifies that nav sections
// require EPUB3.
// Python L668: if book_part.is_nav: self.generate_epub2 = False
func TestCheckEpubVersion_NavSectionRequiresEPUB3(t *testing.T) {
	book := &decodedBook{}
	sections := []epub.Section{
		{Filename: "nav.xhtml", Properties: "nav"},
	}
	result := checkEpubVersion(true, book, sections, nil)
	if result {
		t.Error("expected false (EPUB3 required) for nav section")
	}
}

// TestCheckEpubVersion_FXLSectionRequiresEPUB3 verifies that fixed-layout
// sections require EPUB3.
// Python L668: if book_part.is_fxl: self.generate_epub2 = False
func TestCheckEpubVersion_FXLSectionRequiresEPUB3(t *testing.T) {
	book := &decodedBook{}
	sections := []epub.Section{
		{Filename: "page.xhtml", Properties: "rendition:layout-pre-paginated"},
	}
	result := checkEpubVersion(true, book, sections, nil)
	if result {
		t.Error("expected false (EPUB3 required) for FXL section")
	}
}

// TestCheckEpubVersion_EPUB3TagRequiresEPUB3 verifies that sections containing
// EPUB3-only HTML tags require EPUB3.
// Python L674-677: if elem.tag in {article, aside, audio, ...}: self.generate_epub2 = False
func TestCheckEpubVersion_EPUB3TagRequiresEPUB3(t *testing.T) {
	for _, tag := range []string{"article", "aside", "audio", "nav", "section", "ruby", "video"} {
		book := &decodedBook{
			RenderedSections: []renderedSection{
				{
					Filename: "test.xhtml",
					Root: &htmlElement{
						Tag: "html",
						Children: []htmlPart{
							&htmlElement{Tag: tag},
						},
					},
				},
			},
		}
		result := checkEpubVersion(true, book, nil, nil)
		if result {
			t.Errorf("expected false (EPUB3 required) for tag %q", tag)
		}
	}
}

// TestCheckEpubVersion_DataAttributeRequiresEPUB3 verifies that elements with
// data-* attributes require EPUB3.
// Python L681: if attrib.startswith("data-"): self.generate_epub2 = False
func TestCheckEpubVersion_DataAttributeRequiresEPUB3(t *testing.T) {
	book := &decodedBook{
		RenderedSections: []renderedSection{
			{
				Filename: "test.xhtml",
				Root: &htmlElement{
					Tag: "div",
					Attrs: map[string]string{"data-custom": "value"},
					Children: []htmlPart{
						&htmlElement{Tag: "p", Children: []htmlPart{htmlText{Text: "hello"}}},
					},
				},
			},
		},
	}
	result := checkEpubVersion(true, book, nil, nil)
	if result {
		t.Error("expected false (EPUB3 required) for data-* attribute")
	}
}

// TestCheckEpubVersion_EpubTypeAttributeRequiresEPUB3 verifies that elements
// with epub:type attributes require EPUB3.
// Python L681: if attrib in [EPUB_PREFIX, EPUB_TYPE]: self.generate_epub2 = False
func TestCheckEpubVersion_EpubTypeAttributeRequiresEPUB3(t *testing.T) {
	book := &decodedBook{
		RenderedSections: []renderedSection{
			{
				Filename: "test.xhtml",
				Root: &htmlElement{
					Tag: "div",
					Attrs: map[string]string{"epub:type": "chapter"},
					Children: []htmlPart{
						&htmlElement{Tag: "p", Children: []htmlPart{htmlText{Text: "hello"}}},
					},
				},
			},
		},
	}
	result := checkEpubVersion(true, book, nil, nil)
	if result {
		t.Error("expected false (EPUB3 required) for epub:type attribute")
	}
}

// TestCheckEpubVersion_PlainReflowableReturnsTrue verifies that a plain
// reflowable book with no EPUB3 features allows EPUB2.
// Python: all checks pass without setting generate_epub2 to False.
func TestCheckEpubVersion_PlainReflowableReturnsTrue(t *testing.T) {
	book := &decodedBook{
		RenderedSections: []renderedSection{
			{
				Filename: "test.xhtml",
				Root: &htmlElement{
					Tag: "html",
					Children: []htmlPart{
						&htmlElement{Tag: "head"},
						&htmlElement{Tag: "body", Children: []htmlPart{
							&htmlElement{Tag: "p", Children: []htmlPart{htmlText{Text: "hello"}}},
						}},
					},
				},
			},
		},
	}
	sections := []epub.Section{
		{Filename: "test.xhtml"},
	}
	resources := []epub.Resource{
		{Filename: "img.jpg", MediaType: "image/jpeg"},
	}
	result := checkEpubVersion(true, book, sections, resources)
	if !result {
		t.Error("expected true (EPUB2 OK) for plain reflowable book")
	}
}
