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

	fontFixer := newFontNameFixer()
	currentFontFixer = fontFixer
	defer func() {
		currentFontFixer = nil
	}()
	book.Resources, book.CoverImageHref, book.Stylesheet, book.ResourceHrefByID = buildResources(book, resourceFragments, fontFragments, rawFragments, rawBlobOrder)
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
				fallbackAnchorURI[anchorID] = sectionFilename(sectionID)
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
		contentFragments:  contentFragments,
		rubyGroups:        rubyGroups,
		rubyContents:      rubyContents,
		resourceHrefByID:  book.ResourceHrefByID,
		resourceFragments: resourceFragments,
		directAnchorURI:   directAnchorURI,
		fallbackAnchorURI: fallbackAnchorURI,
		positionToSection: positionToSectionID,
		positionAnchors:   navState.positionAnchors,
		positionAnchorID:  buildPositionAnchorIDs(navState.positionAnchors),
		anchorNamesByID:   map[string][]string{},
		anchorHeadingLevel: navState.anchorHeadingLevel,
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    styleFragments,
		styles:            newStyleCatalog(),
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
	if navOrder := orderedSectionIDsFromNavigation(selectedNav, positionToSectionID); len(navOrder) > 0 {
		sectionOrder = mergeSectionOrder(navOrder, sectionOrder)
	}
	debugSectionMappings(sectionFragments, navTitles, sectionOrder)

	for index, sectionID := range sectionOrder {
		section, ok := sectionFragments[sectionID]
		if !ok {
			continue
		}
		rendered, paragraphs, ok := renderSectionFragments(sectionID, section, storylines, contentFragments, &renderer)
		if !ok {
			continue
		}
		if debugSection := os.Getenv("KFX_DEBUG_SECTION_CLASS"); debugSection != "" {
			for _, wanted := range strings.Split(debugSection, ",") {
				if strings.TrimSpace(wanted) == sectionID {
					fmt.Fprintf(os.Stderr, "section=%s bodyClass=%q properties=%q\n", sectionID, rendered.BodyClass, rendered.Properties)
				}
			}
		}
		if len(paragraphs) == 0 && rendered.BodyHTML == "" {
			continue
		}
		title := navTitles[sectionID]
		if title == "" {
			title = deriveSectionTitle(paragraphs, index+1)
		}
		book.RenderedSections = append(book.RenderedSections, renderedSection{
			Filename:   sectionFilename(sectionID),
			Title:      title,
			PageTitle:  sectionID,
			Language:   normalizeLanguage(book.Language),
			BodyClass:  rendered.BodyClass,
			Paragraphs: paragraphs,
			Properties: rendered.Properties,
			Root:       rendered.Root,
		})
	}
	cleanupRenderedSections(book.RenderedSections)

	for _, section := range book.RenderedSections {
		renderer.styles.markReferenced(section.BodyClass)
		renderer.styles.markReferenced(renderedSectionBodyHTML(section))
	}
	replacer := renderer.styles.replacer()
	for index := range book.RenderedSections {
		book.RenderedSections[index].BodyClass = replacer.Replace(book.RenderedSections[index].BodyClass)
		replaceSectionDOMClassTokens(&book.RenderedSections[index], replacer)
	}
	attachSectionAliasAnchors(book.RenderedSections, &renderer)
	resolvedAnchorURI := resolveRenderedAnchorURIs(book.RenderedSections, &renderer)
	replaceRenderedAnchorPlaceholders(book.RenderedSections, resolvedAnchorURI)
	if css := renderer.styles.String(); css != "" {
		if book.Stylesheet != "" {
			book.Stylesheet += "\n"
		}
		book.Stylesheet += css
	}
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
		return sectionFilename(sectionID)
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
		return sectionFilename(sectionID)
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
	book.Sections = materializeRenderedSections(book.RenderedSections)
	applyCoverSVGPromotion(book)
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

func renderSectionFragments(sectionID string, section sectionFragment, storylines map[string]map[string]interface{}, contentFragments map[string][]string, renderer *storylineRenderer) (renderedStoryline, []string, bool) {
	templates := section.PageTemplates
	if len(templates) == 0 {
		templates = []pageTemplateFragment{{
			PositionID:         section.PositionID,
			Storyline:          section.Storyline,
			PageTemplateStyle:  section.PageTemplateStyle,
			PageTemplateValues: section.PageTemplateValues,
		}}
	}
	if renderer != nil && renderer.conditionEvaluator.fixedLayout && pageTemplatesHaveConditions(templates) {
		active := make([]pageTemplateFragment, 0, len(templates))
		for _, template := range templates {
			if template.Condition == nil || renderer.conditionEvaluator.evaluateBinary(template.Condition) {
				active = append(active, template)
			}
		}
		if len(active) == 0 {
			return renderedStoryline{}, nil, false
		}
		templates = active
	}

	mainIndex := len(templates) - 1
	mainTemplate := templates[mainIndex]
	storyline := storylines[mainTemplate.Storyline]
	if storyline == nil {
		return renderedStoryline{}, nil, false
	}
	nodes, _ := asSlice(storyline["$146"])
	paragraphs := flattenParagraphs(nodes, contentFragments)
	debugStorylineNodes(sectionID, nodes, 0)
	if os.Getenv("KFX_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "render section=%s pageStyle=%s storyStyle=%s\n", sectionID, mainTemplate.PageTemplateStyle, asStringDefault(storyline["$157"]))
	}
	rendered := renderer.renderStoryline(mainTemplate.PositionID, mainTemplate.PageTemplateStyle, mainTemplate.PageTemplateValues, storyline, nodes)

	for _, template := range templates[:mainIndex] {
		overlayStoryline := storylines[template.Storyline]
		if overlayStoryline == nil {
			continue
		}
		overlayNodes, _ := asSlice(overlayStoryline["$146"])
		overlayParagraphs := flattenParagraphs(overlayNodes, contentFragments)
		paragraphs = append(paragraphs, overlayParagraphs...)
		debugStorylineNodes(sectionID, overlayNodes, 0)
		if os.Getenv("KFX_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "render overlay section=%s pageStyle=%s storyStyle=%s conditional=%v\n", sectionID, template.PageTemplateStyle, asStringDefault(overlayStoryline["$157"]), template.HasCondition)
		}
		overlayRendered := renderer.renderStoryline(template.PositionID, template.PageTemplateStyle, template.PageTemplateValues, overlayStoryline, overlayNodes)
		if rendered.BodyClass == "" {
			rendered.BodyClass = overlayRendered.BodyClass
		}
		if overlayRendered.Root != nil {
			if rendered.Root == nil {
				rendered.Root = &htmlElement{Attrs: map[string]string{}}
			}
			rendered.Root.Children = append(rendered.Root.Children, overlayRendered.Root.Children...)
			rendered.BodyHTML = renderHTMLParts(rendered.Root.Children, true)
		}
		rendered.Properties = mergeSectionProperties(rendered.Properties, overlayRendered.Properties)
	}

	return rendered, paragraphs, len(paragraphs) > 0 || rendered.BodyHTML != ""
}

func pageTemplatesHaveConditions(templates []pageTemplateFragment) bool {
	for _, template := range templates {
		if template.Condition != nil || template.HasCondition {
			return true
		}
	}
	return false
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

func mergeSectionProperties(left string, right string) string {
	seen := map[string]bool{}
	merged := make([]string, 0, 2)
	for _, raw := range strings.Fields(strings.TrimSpace(left + " " + right)) {
		if raw == "" || seen[raw] {
			continue
		}
		seen[raw] = true
		merged = append(merged, raw)
	}
	return strings.Join(merged, " ")
}
