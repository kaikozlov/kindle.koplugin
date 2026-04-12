package kfx

import (
	"fmt"
	"os"
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
)

func renderBookState(state *bookState) (*decodedBook, error) {
	book := state.Book
	contentFragments := state.Fragments.ContentFragments
	rubyGroups := state.Fragments.RubyGroups
	rubyContents := state.Fragments.RubyContents
	storylines := state.Fragments.Storylines
	styleFragments := state.Fragments.StyleFragments
	sectionFragments := state.Fragments.SectionFragments
	anchors := state.Fragments.AnchorFragments
	navContainers := state.Fragments.NavContainers
	navRoots := state.Fragments.NavRoots
	resourceFragments := state.Fragments.ResourceFragments
	fontFragments := state.Fragments.FontFragments
	rawFragments := state.Fragments.RawFragments
	positionAliases := state.Fragments.PositionAliases
	rawBlobOrder := state.Fragments.RawBlobOrder
	sectionOrder := append([]string(nil), state.Fragments.SectionOrder...)
	symFmt := state.BookSymbolFormat

	fontFixer := newFontNameFixer()
	// Register @font-face font names first (Python: process_fonts runs before process_document_data).
	// This ensures font names like "FreeFontSerif" are registered with proper case before
	// setDefaultFontFamily resolves "default" → the document's default font family.
	fontFixer.registerFontFamilies(fontFragments)
	fontFixer.setDefaultFontFamily(book.DefaultFontFamily)
	currentFontFixer = fontFixer
	defer func() {
		currentFontFixer = nil
	}()
	book.Resources, book.CoverImageHref, book.Stylesheet, book.ResourceHrefByID = buildResources(book, resourceFragments, fontFragments, rawFragments, rawBlobOrder, symFmt)
	book.Language = inferBookLanguage(book.Language, contentFragments, storylines, styleFragments)

	positionToSectionID := map[int]string{}
	for positionID, sectionID := range positionAliases {
		positionToSectionID[positionID] = sectionID
	}
	for _, section := range sectionFragments {
		if section.PositionID != 0 {
			positionToSectionID[section.PositionID] = section.ID
		}
		for _, template := range section.PageTemplates {
			if template.PositionID != 0 {
				positionToSectionID[template.PositionID] = section.ID
			}
		}
	}
	for _, sectionID := range sectionOrder {
		section := sectionFragments[sectionID]
		templates := section.PageTemplates
		if len(templates) == 0 {
			templates = []pageTemplateFragment{{
				PositionID:         section.PositionID,
				Storyline:          section.Storyline,
				PageTemplateStyle:  section.PageTemplateStyle,
				PageTemplateValues: section.PageTemplateValues,
			}}
		}
		for _, template := range templates {
			storyline := storylines[template.Storyline]
			if storyline == nil {
				continue
			}
			nodes, _ := asSlice(storyline["$146"])
			collectStorylinePositions(nodes, sectionID, positionToSectionID)
		}
	}

	navState := processNavigation(navRoots, navContainers)
	selectedNav := navState.toc
	navTitles := map[string]string{}
	flattenNavigationTitles(selectedNav, positionToSectionID, navTitles)
	directAnchorURI := map[string]string{}
	fallbackAnchorURI := map[string]string{}
	for anchorID, anchor := range anchors {
		if anchor.URI != "" {
			directAnchorURI[anchorID] = anchor.URI
		} else if anchor.PositionID != 0 {
			if sectionID := positionToSectionID[anchor.PositionID]; sectionID != "" {
				fallbackAnchorURI[anchorID] = sectionFilename(sectionID, symFmt)
			}
			registerNamedPositionAnchor(navState.positionAnchors, anchorID, navTarget{PositionID: anchor.PositionID})
		}
		if debugAnchors := os.Getenv("KFX_DEBUG_ANCHORS"); debugAnchors != "" {
			for _, wanted := range strings.Split(debugAnchors, ",") {
				if strings.TrimSpace(wanted) == anchorID {
					fmt.Fprintf(os.Stderr, "anchor map[%s]=%q uri=%q pos=%d\n", anchorID, directAnchorURI[anchorID], anchor.URI, anchor.PositionID)
				}
			}
		}
	}

	renderer := storylineRenderer{
		contentFragments:   contentFragments,
		rubyGroups:         rubyGroups,
		rubyContents:       rubyContents,
		resourceHrefByID:   book.ResourceHrefByID,
		resourceFragments:  resourceFragments,
		directAnchorURI:    directAnchorURI,
		fallbackAnchorURI:  fallbackAnchorURI,
		positionToSection:  positionToSectionID,
		positionAnchors:    navState.positionAnchors,
		positionAnchorID:   buildPositionAnchorIDs(navState.positionAnchors),
		anchorNamesByID:    map[string][]string{},
		anchorHeadingLevel: navState.anchorHeadingLevel,
		emittedAnchorIDs:   map[string]bool{},
		styleFragments:     styleFragments,
		styles:             newStyleCatalog(),
		symFmt:             symFmt,
		conditionEvaluator: conditionEvaluator{
			orientationLock:   book.OrientationLock,
			fixedLayout:       book.FixedLayout,
			illustratedLayout: book.IllustratedLayout,
		},
	}
	if os.Getenv("KFX_DEBUG_STYLES") != "" {
		for _, styleID := range strings.Split(os.Getenv("KFX_DEBUG_STYLES"), ",") {
			styleID = strings.TrimSpace(styleID)
			if styleID == "" {
				continue
			}
			fmt.Fprintf(os.Stderr, "style %s = %#v\n", styleID, styleFragments[styleID])
		}
	}
	if os.Getenv("KFX_DEBUG") != "" {
		for _, pos := range []int{1007, 1053, 1110, 1111, 1177, 1178} {
			fmt.Fprintf(os.Stderr, "anchor ids pos=%d offsets=%v raw=%v\n", pos, renderer.positionAnchorID[pos], renderer.positionAnchors[pos])
		}
	}
	// Merge navigation-referenced sections (guide entries, TOC entries) into the reading order.
	if navOrder := orderedSectionIDsFromNavigation(selectedNav, positionToSectionID); len(navOrder) > 0 {
		sectionOrder = mergeSectionOrder(navOrder, sectionOrder)
	}
	// Port of epub_output.py identify_cover: if a cover guide entry points to a section,
	// ensure that section is first in the spine (Python expects cover to be first in reading order).
	sectionOrder = promoteCoverSectionFromGuide(sectionOrder, navState.guide, positionToSectionID)
	debugSectionMappings(sectionFragments, navTitles, sectionOrder)

	processReadingOrder(book, sectionOrder, sectionFragments, storylines, contentFragments, &renderer, navTitles, symFmt)
	cleanupRenderedSections(book.RenderedSections)
	attachSectionAliasAnchors(book.RenderedSections, &renderer)
	resolvedAnchorURI := resolveRenderedAnchorURIs(book.RenderedSections, &renderer)
	fixupAnchorsAndHrefs(book.RenderedSections, resolvedAnchorURI)
	fixupIllustratedLayoutAnchors(book, book.RenderedSections)
	updateDefaultFontAndLanguage(book)
	resolvedDefaultFont := fontFixer.resolvedDefaultFontFamily()
	fontFamilyAddedByDefaults := setHTMLDefaults(book, resolvedDefaultFont)
	fixupStylesAndClasses(book, renderer.styles, fontFamilyAddedByDefaults, resolvedDefaultFont)
	createCSSFiles(book, renderer.styles)
	book.Stylesheet = finalizeStylesheet(book.Stylesheet)

	targetHref := func(target navTarget) string {
		if target.PositionID == 0 {
			return ""
		}
		if href := resolvedAnchorURI[firstAnchorNameForPosition(navState.positionAnchors, target.PositionID, target.Offset)]; href != "" {
			return href
		}
		sectionID := positionToSectionID[target.PositionID]
		if sectionID == "" {
			return ""
		}
		return sectionFilename(sectionID, symFmt)
	}
	navHref := func(target navTarget) string {
		if target.PositionID == 0 {
			return ""
		}
		if href := resolvedAnchorURI[firstAnchorNameForPosition(navState.positionAnchors, target.PositionID, target.Offset)]; href != "" {
			return href
		}
		sectionID := positionToSectionID[target.PositionID]
		if sectionID == "" {
			return ""
		}
		return sectionFilename(sectionID, symFmt)
	}

	book.Navigation = navigationToEPUB(selectedNav, navHref)
	book.Guide = guideToEPUB(navState.guide, navHref)
	if os.Getenv("KFX_DEBUG") != "" {
		for _, page := range navState.pages {
			if page.Label == "13" || page.Label == "14" || page.Label == "23" || page.Label == "26" || page.Label == "33" || page.Label == "35" || page.Label == "36" || page.Label == "38" || page.Label == "41" || page.Label == "50" || page.Label == "52" || page.Label == "59" || page.Label == "60" || page.Label == "61" || page.Label == "101" || page.Label == "102" {
				fmt.Fprintf(os.Stderr, "page label=%s pos=%d off=%d href=%s\n", page.Label, page.Target.PositionID, page.Target.Offset, targetHref(page.Target))
			}
		}
	}
	book.PageList = pagesToEPUB(navState.pages, targetHref)
	prepareBookParts(book)
	reportMissingPositions(navState.positionAnchors)
	reportDuplicateAnchors(navState, resolvedAnchorURI)
	book.Sections = materializeRenderedSections(book.RenderedSections)
	applyCoverSVGPromotion(book)
	pruneUnusedResources(book)
	book.Stylesheet = pruneUnusedStylesheetRules(book.Stylesheet, collectReferencedClasses(book))
	book.Stylesheet = finalizeStylesheet(book.Stylesheet)
	book.Identifier = normalizeBookIdentifier(book.Identifier)
	book.Language = normalizeLanguage(book.Language)

	return book, nil
}

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
			Filename:   section.Filename,
			Title:      section.Title,
			PageTitle:  section.PageTitle,
			Language:   section.Language,
			BodyClass:  section.BodyClass,
			Paragraphs: append([]string(nil), section.Paragraphs...),
			BodyHTML:   renderedSectionBodyHTML(section),
			Properties: section.Properties,
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

// promoteCoverSectionFromGuide moves the cover section to the front of the section order.
// Port of epub_output.py identify_cover which expects the cover page to be first in the book.
func promoteCoverSectionFromGuide(sections []string, guideEntries []guideEntry, positionToSection map[int]string) []string {
	if len(sections) == 0 || len(guideEntries) == 0 {
		return sections
	}
	// Find cover section from guide entry.
	var coverSectionID string
	for _, entry := range guideEntries {
		if entry.Type == "cover" && entry.Target.PositionID != 0 {
			coverSectionID = positionToSection[entry.Target.PositionID]
			break
		}
	}
	if coverSectionID == "" {
		return sections
	}
	// Check if already first.
	if len(sections) > 0 && sections[0] == coverSectionID {
		return sections
	}
	// Move cover section to front.
	result := make([]string, 0, len(sections))
	result = append(result, coverSectionID)
	for _, id := range sections {
		if id != coverSectionID {
			result = append(result, id)
		}
	}
	return result
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
