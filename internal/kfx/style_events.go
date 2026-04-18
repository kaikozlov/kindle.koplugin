package kfx

import (
	"fmt"
	"unicode/utf8"
)

func locateOffsetFull(root *htmlElement, offset int, splitAfter bool, zeroLen bool, isDropcap bool) *htmlElement {
	if root == nil || offset < 0 {
		return nil
	}

	result, remaining := locateOffsetInFull(root, root, offset, splitAfter, zeroLen, isDropcap)
	if remaining < 0 {
		return result
	}

	if remaining == 0 && !splitAfter {
		span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
		root.Children = append(root.Children, span)
		return span
	}

	if isDropcap {
		span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
		root.Children = append(root.Children, span)
		return span
	}

	return nil
}

func locateOffsetInFull(root *htmlElement, elem *htmlElement, offset int, splitAfter bool, zeroLen bool, isDropcap bool) (*htmlElement, int) {
	if offset < 0 {
		return nil, offset
	}

	if elem.Tag == "span" {
		textLen := elementTextLen(elem)

		if textLen > 0 {
			if !splitAfter {
				if offset == 0 {
					return elem, -1
				} else if offset < textLen {
					newSpan := splitSpan(root, elem, offset)
					if zeroLen {
						splitSpan(root, newSpan, 0)
					}
					return newSpan, -1
				}
			} else {
				if offset == textLen-1 {
					return elem, -1
				} else if offset < textLen {
					splitSpan(root, elem, offset+1)
					return elem, -1
				}
			}

			offset -= textLen
		}

		for _, child := range elem.Children {
			if ce, ok := child.(*htmlElement); ok {
				result, remaining := locateOffsetInFull(root, ce, offset, splitAfter, zeroLen, isDropcap)
				if remaining < 0 {
					return result, remaining
				}
				offset = remaining
			}
		}

		return nil, offset
	}

	if isDropcap {
		style := parseDeclarationString(elem.Attrs["style"])
		if style["float"] != "" {
			return nil, offset
		}
	}

	if elem.Tag == "img" || elem.Tag == "svg" || elem.Tag == "math" {
		if offset == 0 {
			return elem, -1
		}
		offset--
		return nil, offset
	}

	switch elem.Tag {
	case "a", "aside", "div", "figure", "h1", "h2", "h3", "h4", "h5", "h6", "li", "ruby", "rb":
		for _, child := range elem.Children {
			if ce, ok := child.(*htmlElement); ok {
				result, remaining := locateOffsetInFull(root, ce, offset, splitAfter, zeroLen, isDropcap)
				if remaining < 0 {
					return result, remaining
				}
				offset = remaining
			}
		}
		return nil, offset

	case "rt":
		return nil, offset

	default:
		return nil, offset
	}
}

func elementTextLen(elem *htmlElement) int {
	length := 0
	for _, child := range elem.Children {
		if t, ok := child.(htmlText); ok {
			length += utf8.RuneCountInString(t.Text)
		} else {
			break
		}
	}
	return length
}

func splitSpan(root *htmlElement, oldSpan *htmlElement, firstTextLen int) *htmlElement {
	var textRunes []rune
	textEnd := 0
	for i, child := range oldSpan.Children {
		if t, ok := child.(htmlText); ok {
			textRunes = append(textRunes, []rune(t.Text)...)
			textEnd = i + 1
		} else {
			break
		}
	}

	var firstText string
	var secondText string
	if firstTextLen <= 0 {
		firstText = ""
		secondText = string(textRunes)
	} else if firstTextLen >= len(textRunes) {
		firstText = string(textRunes)
		secondText = ""
	} else {
		firstText = string(textRunes[:firstTextLen])
		secondText = string(textRunes[firstTextLen:])
	}

	var firstParts []htmlPart
	if firstText != "" {
		firstParts = []htmlPart{htmlText{Text: firstText}}
	}

	remaining := oldSpan.Children[textEnd:]

	newChildren := make([]htmlPart, 0, len(firstParts)+len(remaining))
	newChildren = append(newChildren, firstParts...)
	newChildren = append(newChildren, remaining...)
	oldSpan.Children = newChildren

	newSpan := &htmlElement{Tag: "span", Attrs: cloneAttrs(oldSpan)}
	if secondText != "" {
		newSpan.Children = []htmlPart{htmlText{Text: secondText}}
	}

	parent, idx := findParent(root, oldSpan)
	if parent != nil {
		parent.Children = insertHTMLParts(parent.Children, idx+1, []htmlPart{newSpan})
	}

	return newSpan
}

func findParent(root, target *htmlElement) (parent *htmlElement, index int) {
	if root == nil || target == nil {
		return nil, -1
	}
	for i, child := range root.Children {
		if ce, ok := child.(*htmlElement); ok {
			if ce == target {
				return root, i
			}
			if p, idx := findParent(ce, target); p != nil {
				return p, idx
			}
		}
	}
	return nil, -1
}

func cloneAttrs(elem *htmlElement) map[string]string {
	if len(elem.Attrs) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(elem.Attrs))
	for k, v := range elem.Attrs {
		cloned[k] = v
	}
	return cloned
}

func hasNoLeadingText(elem *htmlElement) bool {
	for _, child := range elem.Children {
		if t, ok := child.(htmlText); ok {
			if utf8.RuneCountInString(t.Text) > 0 {
				return false
			}
			continue
		}
		break
	}
	return true
}

func isFirstElementChild(parent *htmlElement, child *htmlElement) bool {
	for _, c := range parent.Children {
		if ce, ok := c.(*htmlElement); ok {
			return ce == child
		}
	}
	return false
}

func isLastElementChild(parent *htmlElement, child *htmlElement) bool {
	for i := len(parent.Children) - 1; i >= 0; i-- {
		if ce, ok := parent.Children[i].(*htmlElement); ok {
			return ce == child
		}
	}
	return false
}

func elementTailIsEmpty(parent *htmlElement, childIndex int) bool {
	if childIndex+1 < len(parent.Children) {
		if t, ok := parent.Children[childIndex+1].(htmlText); ok {
			return utf8.RuneCountInString(t.Text) == 0
		}
	}
	return true
}

func findOrCreateStyleEventElement(contentElem *htmlElement, eventOffset int, eventLength int, isDropcap bool) *htmlElement {
	if eventLength <= 0 {
		panic(fmt.Sprintf("style event has length: %d", eventLength))
	}

	first := locateOffsetFull(contentElem, eventOffset, false, false, isDropcap)
	if first == nil {
		return nil
	}

	last := locateOffsetFull(contentElem, eventOffset+eventLength-1, true, false, isDropcap)

	if last == nil || first == last {
		return first
	}

	firstParent, _ := findParent(contentElem, first)
	lastParent, _ := findParent(contentElem, last)

	if firstParent != lastParent {
		tryFirst := first
		firsts := []*htmlElement{tryFirst}

		for firstParent != nil && hasNoLeadingText(firstParent) && isFirstElementChild(firstParent, tryFirst) {
			tryFirst = firstParent
			firstParent, _ = findParent(contentElem, tryFirst)
			if firstParent != nil {
				firsts = append(firsts, tryFirst)
			}
		}

		tryLast := last
		lasts := []*htmlElement{tryLast}

		_, origLastIdx := findParent(contentElem, last)
		origLastParent := lastParent

		for lastParent != nil && elementTailIsEmpty(origLastParent, origLastIdx) && isLastElementChild(lastParent, tryLast) {
			tryLast = lastParent
			lastParent, _ = findParent(contentElem, tryLast)
			if lastParent != nil {
				lasts = append(lasts, tryLast)
			}
		}

		found := false
		for _, tf := range firsts {
			for _, tl := range lasts {
				tfParent, _ := findParent(contentElem, tf)
				tlParent, _ := findParent(contentElem, tl)
				if tfParent == tlParent && tfParent != nil {
					first = tf
					last = tl
					found = true
					break
				}
			}
			if found {
				break
			}
		}

		if !found {
			return nil
		}
	}

	eventElem := &htmlElement{Tag: "span", Attrs: map[string]string{}}

	seParent, firstIndex := findParent(contentElem, first)
	_, lastIndex := findParent(contentElem, last)

	moved := make([]htmlPart, lastIndex-firstIndex+1)
	copy(moved, seParent.Children[firstIndex:lastIndex+1])

	eventElem.Children = moved

	newChildren := make([]htmlPart, 0, len(seParent.Children)-(lastIndex-firstIndex))
	newChildren = append(newChildren, seParent.Children[:firstIndex]...)
	newChildren = append(newChildren, eventElem)
	newChildren = append(newChildren, seParent.Children[lastIndex+1:]...)
	seParent.Children = newChildren

	return eventElem
}
