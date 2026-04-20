package kfx

import (
	"sort"
	"strings"
)

type htmlPart interface{}

type htmlText struct {
	Text string
}

type htmlElement struct {
	Tag      string
	Attrs    map[string]string
	Children []htmlPart
}

func splitTextHTMLParts(text string) []htmlPart {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	parts := make([]htmlPart, 0, len(lines)*2)
	for index, line := range lines {
		if index > 0 {
			parts = append(parts, &htmlElement{Tag: "br", Attrs: map[string]string{}})
		}
		if line != "" {
			parts = append(parts, htmlText{Text: line})
		}
	}
	return parts
}

func appendTextHTMLParts(element *htmlElement, text string) {
	if element == nil || text == "" {
		return
	}
	// Merge with last htmlText child if possible, matching Python's etree text
	// concatenation behavior. Without merging, the annotation event loop creates
	// individual htmlText nodes per character, which breaks locateOffsetIn for
	// page markers when wrapped in spans.
	if len(element.Children) > 0 {
		switch last := element.Children[len(element.Children)-1].(type) {
		case htmlText:
			element.Children[len(element.Children)-1] = htmlText{Text: last.Text + text}
			return
		case *htmlText:
			element.Children[len(element.Children)-1] = &htmlText{Text: last.Text + text}
			return
		}
	}
	element.Children = append(element.Children, splitTextHTMLParts(text)...)
}

func locateOffset(root *htmlElement, offset int) *htmlElement {
	if root == nil || offset < 0 {
		return nil
	}
	if found, ok := locateOffsetIn(root, offset); ok {
		return found
	}
	return nil
}

func locateOffsetIn(elem *htmlElement, offset int) (*htmlElement, bool) {
	if elem == nil {
		return nil, false
	}
	if elem.Tag == "img" {
		return elem, offset == 0
	}
	for index := 0; index < len(elem.Children); index++ {
		switch child := elem.Children[index].(type) {
		case htmlText:
			length := len([]rune(child.Text))
			if offset == 0 {
				span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
				elem.Children = insertHTMLParts(elem.Children, index, []htmlPart{span})
				return span, true
			}
			if offset < length {
				runes := []rune(child.Text)
				parts := make([]htmlPart, 0, 3)
				if offset > 0 {
					parts = append(parts, htmlText{Text: string(runes[:offset])})
				}
				span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
				parts = append(parts, span)
				if offset < len(runes) {
					parts = append(parts, htmlText{Text: string(runes[offset:])})
				}
				elem.Children = replaceHTMLParts(elem.Children, index, parts)
				return span, true
			}
			offset -= length
		case *htmlElement:
			if offset == 0 {
				// Port of Python locate_offset_in (yj_to_epub_content.py:1600-1601):
				// When offset is 0, return the element directly. However, Python processes
				// position anchors BEFORE annotations, so the element is always a raw text
				// span. Go processes annotations first, so we may find wrapper spans here.
				// If this is a wrapper span (no attrs) containing only text, split it to
				// create an empty span for the page marker, matching Python's zero_len split.
				if child.Tag == "span" && len(child.Attrs) == 0 && len(child.Children) >= 1 {
					// Check if first child is text — if so, insert empty span before it
					firstIsText := false
					switch child.Children[0].(type) {
					case htmlText, *htmlText:
						firstIsText = true
					}
					if firstIsText {
						span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
						child.Children = insertHTMLParts(child.Children, 0, []htmlPart{span})
						return span, true
					}
				}
				return child, true
			}
			length := htmlPartLength(child)
			if offset < length {
				return locateOffsetIn(child, offset)
			}
			offset -= length
		}
	}
	if offset == 0 {
		span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
		elem.Children = append(elem.Children, span)
		return span, true
	}
	return nil, false
}

func htmlPartLength(part htmlPart) int {
	switch typed := part.(type) {
	case htmlText:
		return len([]rune(typed.Text))
	case *htmlElement:
		if typed == nil {
			return 0
		}
		if typed.Tag == "img" {
			return 1
		}
		length := 0
		for _, child := range typed.Children {
			length += htmlPartLength(child)
		}
		return length
	default:
		return 0
	}
}

func replaceHTMLParts(parts []htmlPart, index int, replacement []htmlPart) []htmlPart {
	out := make([]htmlPart, 0, len(parts)-1+len(replacement))
	out = append(out, parts[:index]...)
	out = append(out, replacement...)
	out = append(out, parts[index+1:]...)
	return out
}

func insertHTMLParts(parts []htmlPart, index int, inserted []htmlPart) []htmlPart {
	out := make([]htmlPart, 0, len(parts)+len(inserted))
	out = append(out, parts[:index]...)
	out = append(out, inserted...)
	out = append(out, parts[index:]...)
	return out
}

func renderHTMLParts(parts []htmlPart, multiline bool) string {
	var out strings.Builder
	for index, part := range parts {
		if index > 0 && multiline {
			out.WriteByte('\n')
		}
		out.WriteString(renderHTMLPart(part))
	}
	return out.String()
}

func renderHTMLPart(part htmlPart) string {
	switch typed := part.(type) {
	case nil:
		return ""
	case htmlText:
		return escapeHTML(typed.Text)
	case *htmlText:
		return escapeHTML(typed.Text)
	case *htmlElement:
		return renderHTMLElement(typed)
	default:
		return ""
	}
}

type preformatState struct {
	firstInBlock     bool
	previousChar     rune
	previousReplaced bool
	priorText        *htmlText
}

func (s *preformatState) reset() {
	s.firstInBlock = true
	s.previousChar = 0
	s.previousReplaced = false
	s.priorText = nil
}

func (s *preformatState) setMediaBoundary() {
	s.firstInBlock = false
	s.previousChar = '?'
	s.previousReplaced = false
	s.priorText = nil
}

func normalizeHTMLWhitespace(root *htmlElement) {
	if root == nil {
		return
	}
	state := &preformatState{}
	state.reset()
	root.Children = normalizeHTMLChildren(root.Tag, root.Children, state)
}

func normalizeHTMLChildren(tag string, children []htmlPart, state *preformatState) []htmlPart {
	if state == nil {
		state = &preformatState{}
		state.reset()
	}
	switch {
	case isPreformatMediaTag(tag):
		state.setMediaBoundary()
	case isPreformatInlineTag(tag):
	default:
		state.reset()
	}
	normalized := make([]htmlPart, 0, len(children))
	for _, child := range children {
		switch typed := child.(type) {
		case nil:
			continue
		case htmlText:
			normalized = append(normalized, normalizeHTMLTextParts(typed.Text, state)...)
		case *htmlText:
			normalized = append(normalized, normalizeHTMLTextParts(typed.Text, state)...)
		case *htmlElement:
			typed.Children = normalizeHTMLChildren(typed.Tag, typed.Children, state)
			normalized = append(normalized, typed)
		default:
			normalized = append(normalized, child)
		}
	}
	return normalized
}

func normalizeHTMLTextParts(text string, state *preformatState) []htmlPart {
	if text == "" {
		return nil
	}
	parts := []htmlPart{}
	var segment []rune
	flushSegment := func() {
		if len(segment) == 0 {
			return
		}
		if part := preformatHTMLText(string(segment), state); part != nil {
			parts = append(parts, part)
		}
		segment = segment[:0]
	}
	for _, ch := range text {
		if isEOLRune(ch) {
			flushSegment()
			parts = append(parts, &htmlElement{Tag: "br", Attrs: map[string]string{}})
			state.reset()
			continue
		}
		segment = append(segment, ch)
	}
	flushSegment()
	return parts
}

func preformatHTMLText(text string, state *preformatState) htmlPart {
	if text == "" {
		return nil
	}
	runes := []rune(text)
	out := make([]rune, 0, len(runes))
	for _, ch := range runes {
		orig := ch
		didReplace := false
		if ch == ' ' && (state.firstInBlock || state.previousChar == ' ') {
			if state.previousChar == ' ' && !state.previousReplaced {
				if len(out) > 0 {
					out[len(out)-1] = '\u00a0'
				} else if state.priorText != nil {
					state.priorText.Text = replaceLastRune(state.priorText.Text, '\u00a0')
				}
			}
			ch = '\u00a0'
			didReplace = true
		}
		out = append(out, ch)
		state.firstInBlock = false
		state.previousChar = orig
		state.previousReplaced = didReplace
	}
	part := &htmlText{Text: string(out)}
	state.priorText = part
	return part
}

func replaceLastRune(text string, replacement rune) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return text
	}
	runes[len(runes)-1] = replacement
	return string(runes)
}

func isEOLRune(ch rune) bool {
	switch ch {
	case '\n', '\r', '\u2028', '\u2029':
		return true
	default:
		return false
	}
}

func isPreformatInlineTag(tag string) bool {
	switch tag {
	case "a", "b", "bdi", "bdo", "em", "i", "path", "rb", "rt", "ruby", "span", "strong", "sub", "sup", "u":
		return true
	default:
		return false
	}
}

func isPreformatMediaTag(tag string) bool {
	switch tag {
	case "audio", "iframe", "img", "object", "svg", "video":
		return true
	default:
		return false
	}
}

func renderHTMLElement(element *htmlElement) string {
	if element == nil {
		return ""
	}
	if element.Tag == "" {
		return renderHTMLParts(element.Children, false)
	}
	var out strings.Builder
	out.WriteByte('<')
	out.WriteString(element.Tag)
	attrOrder := []string{"id", "class", "href", "src", "alt"}
	switch element.Tag {
	case "a":
		attrOrder = []string{"id", "href", "class"}
	case "img":
		attrOrder = []string{"id", "src", "alt", "class"}
	}
	for _, key := range attrOrder {
		value, ok := element.Attrs[key]
		if !ok || (value == "" && key != "alt") {
			continue
		}
		out.WriteString(` ` + key + `="` + escapeHTML(value) + `"`)
	}
	remaining := make([]string, 0, len(element.Attrs))
	seen := map[string]bool{}
	for _, key := range attrOrder {
		seen[key] = true
	}
	for key := range element.Attrs {
		if !seen[key] {
			remaining = append(remaining, key)
		}
	}
	sort.Strings(remaining)
	for _, key := range remaining {
		value := element.Attrs[key]
		if value == "" {
			continue
		}
		out.WriteString(` ` + key + `="` + escapeHTML(value) + `"`)
	}
	if len(element.Children) == 0 {
		out.WriteString(`/>`)
		return out.String()
	}
	out.WriteByte('>')
	for _, child := range element.Children {
		out.WriteString(renderHTMLPart(child))
	}
	out.WriteString(`</` + element.Tag + `>`)
	return out.String()
}

func escapeHTML(text string) string {
	var replacer = strings.NewReplacer(
		"&", "&amp;",
		`"`, "&quot;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(text)
}
