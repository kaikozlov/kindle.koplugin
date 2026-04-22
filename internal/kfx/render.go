package kfx

import (
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
)

func registerNamedPositionAnchor(positionAnchors map[int]map[int][]string, name string, target navTarget) {
	if name == "" || target.PositionID == 0 {
		return
	}
	offsets := positionAnchors[target.PositionID]
	if offsets == nil {
		offsets = map[int][]string{}
		positionAnchors[target.PositionID] = offsets
	}
	names := offsets[target.Offset]
	for _, existing := range names {
		if existing == name {
			return
		}
	}
	offsets[target.Offset] = append([]string{name}, names...)
}

func firstAnchorNameForPosition(positionAnchors map[int]map[int][]string, positionID int, offset int) string {
	offsets := positionAnchors[positionID]
	if offsets == nil {
		return ""
	}
	names := offsets[offset]
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func resolveRenderedAnchorURIs(sections []renderedSection, renderer *storylineRenderer) map[string]string {
	resolved := map[string]string{}
	if renderer != nil {
		for name, uri := range renderer.directAnchorURI {
			if uri != "" {
				resolved[name] = uri
			}
		}
		for name, uri := range renderer.fallbackAnchorURI {
			if uri != "" && resolved[name] == "" {
				resolved[name] = uri
			}
		}
	}
	for _, section := range sections {
		if section.Root == nil {
			continue
		}
		visibleSeen := false
		resolveAnchorURIsInParts(section.Root.Children, section.Filename, renderer, resolved, &visibleSeen)
	}
	return resolved
}

func resolveAnchorURIsInParts(parts []htmlPart, filename string, renderer *storylineRenderer, resolved map[string]string, visibleSeen *bool) {
	for _, part := range parts {
		switch typed := part.(type) {
		case htmlText:
			if strings.TrimSpace(typed.Text) != "" {
				*visibleSeen = true
			}
		case *htmlText:
			if typed != nil && strings.TrimSpace(typed.Text) != "" {
				*visibleSeen = true
			}
		case *htmlElement:
			if typed == nil {
				continue
			}
			resolveAnchorURIsForElement(typed, filename, renderer, resolved, *visibleSeen)
			if elementCountsAsVisible(typed) {
				*visibleSeen = true
			}
			resolveAnchorURIsInParts(typed.Children, filename, renderer, resolved, visibleSeen)
		}
	}
}

func resolveAnchorURIsForElement(element *htmlElement, filename string, renderer *storylineRenderer, resolved map[string]string, visibleBefore bool) {
	if element == nil || renderer == nil {
		return
	}
	anchorID := element.Attrs["id"]
	if anchorID == "" {
		return
	}
	names := renderer.anchorNamesByID[anchorID]
	if len(names) == 0 {
		return
	}
	defaultURI := filename + "#" + anchorID
	allMovable := true
	anyMovable := false
	for _, name := range names {
		movable := !strings.HasPrefix(name, "$798_")
		if movable {
			anyMovable = true
		} else {
			allMovable = false
		}
		resolved[name] = defaultURI
	}
	if !visibleBefore && anyMovable {
		for _, name := range names {
			if !strings.HasPrefix(name, "$798_") {
				resolved[name] = filename
			}
		}
		if allMovable {
			delete(element.Attrs, "id")
		}
	}
}

func elementCountsAsVisible(element *htmlElement) bool {
	if element == nil {
		return false
	}
	switch element.Tag {
	case "img", "svg", "audio", "video", "object", "iframe", "br":
		return true
	}
	return false
}

func replaceRenderedAnchorPlaceholders(sections []renderedSection, resolved map[string]string) {
	for index := range sections {
		if sections[index].Root == nil {
			continue
		}
		replaceAnchorPlaceholdersInParts(sections[index].Root.Children, resolved)
	}
}

func attachSectionAliasAnchors(sections []renderedSection, renderer *storylineRenderer) {
	if renderer == nil {
		return
	}
	for index := range sections {
		root := sections[index].Root
		if root == nil {
			continue
		}
		sectionID := strings.TrimSuffix(sections[index].Filename, ".xhtml")
		for positionID, mappedSectionID := range renderer.positionToSection {
			if mappedSectionID != sectionID {
				continue
			}
			offsets := renderer.positionAnchors[positionID]
			if len(offsets) == 0 {
				continue
			}
			if anchorID := renderer.anchorIDForPosition(positionID, 0); anchorID != "" && len(renderer.anchorNamesByID[anchorID]) == 0 {
				if root.Attrs == nil {
					root.Attrs = map[string]string{}
				}
				if root.Attrs["id"] == "" {
					root.Attrs["id"] = anchorID
				}
				renderer.emittedAnchorIDs[anchorID] = true
				renderer.registerAnchorElementNames(positionID, 0, anchorID)
			}
			for offset := range offsets {
				if offset <= 0 {
					continue
				}
				anchorID := renderer.anchorIDForPosition(positionID, offset)
				if anchorID == "" || len(renderer.anchorNamesByID[anchorID]) > 0 {
					continue
				}
				target := locateOffset(root, offset)
				if target == nil {
					continue
				}
				if target.Attrs == nil {
					target.Attrs = map[string]string{}
				}
				if target.Attrs["id"] == "" {
					target.Attrs["id"] = anchorID
				}
				renderer.emittedAnchorIDs[anchorID] = true
				renderer.registerAnchorElementNames(positionID, offset, anchorID)
			}
		}
	}
}

func replaceAnchorPlaceholdersInParts(parts []htmlPart, resolved map[string]string) {
	for _, part := range parts {
		element, ok := part.(*htmlElement)
		if !ok || element == nil {
			continue
		}
		if href := element.Attrs["href"]; strings.HasPrefix(href, "anchor:") {
			if resolvedHref := resolved[strings.TrimPrefix(href, "anchor:")]; resolvedHref != "" {
				element.Attrs["href"] = resolvedHref
			}
		}
		replaceAnchorPlaceholdersInParts(element.Children, resolved)
	}
}

func renderedSectionBodyHTML(section renderedSection) string {
	if section.Root == nil {
		return ""
	}
	return renderHTMLParts(section.Root.Children, true)
}

func replaceSectionDOMClassTokens(section *renderedSection, replacer *strings.Replacer) {
	if section == nil || section.Root == nil || replacer == nil {
		return
	}
	replaceHTMLClassTokens(section.Root, replacer)
}

func replaceHTMLClassTokens(element *htmlElement, replacer *strings.Replacer) {
	if element == nil || replacer == nil {
		return
	}
	if element.Attrs != nil {
		if className := element.Attrs["class"]; className != "" {
			element.Attrs["class"] = replacer.Replace(className)
		}
	}
	for _, child := range element.Children {
		if childElement, ok := child.(*htmlElement); ok {
			replaceHTMLClassTokens(childElement, replacer)
		}
	}
}

func materializeRenderedSections(rendered []renderedSection) []epub.Section {
	sections := make([]epub.Section, 0, len(rendered))
	for _, section := range rendered {
		sections = append(sections, epub.Section{
			Filename:    section.Filename,
			Title:       section.Title,
			PageTitle:   section.PageTitle,
			Language:    section.Language,
			BodyLanguage: section.BodyLanguage,
			BodyClass:   section.BodyClass,
			Paragraphs:  append([]string(nil), section.Paragraphs...),
			BodyHTML:    renderedSectionBodyHTML(section),
			Properties:  section.Properties,
		})
	}
	return sections
}

func cleanupRenderedSections(sections []renderedSection) {
	for index := range sections {
		if sections[index].Root == nil {
			continue
		}
		sections[index].Root.Children = cleanupHTMLParts(sections[index].Root.Children)
	}
}

func cleanupHTMLParts(parts []htmlPart) []htmlPart {
	cleaned := make([]htmlPart, 0, len(parts))
	for _, part := range parts {
		switch typed := part.(type) {
		case *htmlElement:
			typed.Children = cleanupHTMLParts(typed.Children)
			if isEmptyWrapper(typed) {
				continue
			}
			if shouldCollapseNestedDiv(typed) {
				cleaned = append(cleaned, typed.Children[0])
				continue
			}
			cleaned = append(cleaned, typed)
		default:
			cleaned = append(cleaned, part)
		}
	}
	return cleaned
}

func isEmptyWrapper(element *htmlElement) bool {
	if element == nil {
		return true
	}
	if element.Tag != "span" || len(element.Attrs) > 0 {
		return false
	}
	return len(element.Children) == 0
}

func shouldCollapseNestedDiv(element *htmlElement) bool {
	if element == nil || element.Tag != "div" || len(element.Attrs) > 0 || len(element.Children) != 1 {
		return false
	}
	child, ok := element.Children[0].(*htmlElement)
	if !ok || child == nil || child.Tag != "div" || len(child.Attrs) > 0 {
		return false
	}
	return true
}
