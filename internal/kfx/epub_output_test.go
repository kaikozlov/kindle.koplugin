package kfx

import (
	"testing"
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
