package kfx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/jpeg"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
)

func renderBookState(state *bookState, trace *traceWriter) (*decodedBook, error) {
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
	fontFixer.registerFontFamilies(fontFragments)
	fontFixer.setDefaultFontFamily(book.DefaultFontFamily)
	currentFontFixer = fontFixer
	defer func() {
		currentFontFixer = nil
	}()

	// Stage: organize_fragments + book_symbol_format
	if trace != nil {
		trace.addStage("organize_fragments", captureOrganizeFragments(state))
		trace.addStage("book_symbol_format", captureBookSymbolFormat(state))
	}

	book.Resources, book.CoverImageHref, book.Stylesheet, book.ResourceHrefByID = buildResources(book, resourceFragments, fontFragments, rawFragments, rawBlobOrder, symFmt)
	book.Language = inferBookLanguage(book.Language, contentFragments, storylines, styleFragments)

	// Stage: metadata / document_data / content_features / fonts
	if trace != nil {
		trace.addStage("content_features", captureContentFeatures(book))
		trace.addStage("fonts", captureFonts(book))
		trace.addStage("document_data", captureDocumentData(book))
		trace.addStage("metadata", captureMetadata(book))
	}

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

	navState := processNavigation(navRoots, navContainers, book.OrientationLock)
	selectedNav := navState.toc

	// Stage: navigation (capture after processNavigation, before process_reading_order)
	// Note: we capture nav structure here; the hrefs are set later after anchor resolution.
	// For a complete trace of final navigation hrefs, see the trace point after fixupAnchors.
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
			registerNamedPositionAnchor(navState.positionAnchors, anchorID, navTarget{PositionID: anchor.PositionID, Offset: anchor.Offset})
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
	// Create resource resolver matching Python's self.process_external_resource
	// (yj_to_epub_properties.py:1272-1273). Resolves $479/$528 symbol values
	// (background-image) to CSS url() paths via the pre-built resource href map.
	renderer.resolveResource = func(symbol string) string {
		if href, ok := book.ResourceHrefByID[symbol]; ok {
			return href
		}
		return ""
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
	// Merge: include any navigation-referenced sections not already in the reading order.
	// The KFX reading order ($170) is the authoritative order — Python processes
	// sections strictly in reading order (yj_to_epub_content.py:105-112).
	// Navigation only adds missing sections; it does not reorder existing ones.
	if navOrder := orderedSectionIDsFromNavigation(selectedNav, positionToSectionID); len(navOrder) > 0 {
		sectionOrder = mergeSectionOrder(sectionOrder, navOrder)
	}
	// Port of epub_output.py identify_cover: if a cover guide entry points to a section,
	// ensure that section is first in the spine (Python expects cover to be first in reading order).
	sectionOrder = promoteCoverSectionFromGuide(sectionOrder, navState.guide, positionToSectionID)
	debugSectionMappings(sectionFragments, navTitles, sectionOrder)

	processReadingOrder(book, sectionOrder, sectionFragments, storylines, contentFragments, &renderer, navTitles, symFmt)
	cleanupRenderedSections(book.RenderedSections)

	// Stage: reading_order (capture rendered section HTML after processReadingOrder)
	if trace != nil {
		trace.addStage("reading_order", captureReadingOrder(book.RenderedSections))
	}
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

	// Stage: stylesheet (capture CSS after createCSSFiles)
	if trace != nil {
		trace.addStage("stylesheet", captureStylesheet(book.Stylesheet))
	}

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

	// Port of Python: process_external_resource(icon).filename maps icon resource to href.
	iconHref := func(resourceID string) string {
		if href, ok := book.ResourceHrefByID[resourceID]; ok {
			return href
		}
		return ""
	}
	book.Navigation = navigationToEPUB(selectedNav, navHref, iconHref)
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

	// Stage: navigation (final — with resolved hrefs)
	// Stage: final_sections (after prepareBookParts, before materialize)
	if trace != nil {
		trace.addStage("navigation", captureNavigation(book.Navigation, book.Guide, book.PageList))
	}
	usedAnchors := make(map[string]bool, len(resolvedAnchorURI))
	for name, href := range resolvedAnchorURI {
		if href != "" {
			usedAnchors[name] = true
		}
	}
	reportDuplicateAnchors(navState.anchorSites, usedAnchors)
	book.Sections = materializeRenderedSections(book.RenderedSections)

	// Stage: final_sections
	if trace != nil {
		trace.addStage("final_sections", captureFinalSections(book.Sections))
	}
	applyCoverSVGPromotion(book, resolvedDefaultFont)
	pruneUnusedResources(book)
	book.Stylesheet = pruneUnusedStylesheetRules(book.Stylesheet, collectReferencedClasses(book))
	book.Stylesheet = finalizeStylesheet(book.Stylesheet)
	book.Identifier = normalizeBookIdentifier(book.Identifier)
	book.Language = normalizeLanguage(book.Language)

	return book, nil
}

// buildBookState loads container sources from path and organizes fragments.
// Port of YJ_Book.decode_book → KFX_EPUB.organize_fragments_by_type pipeline.
func buildBookState(path string) (*bookState, error) {
	sources, err := loadBookSources(path)
	if err != nil {
		return nil, err
	}
	return organizeFragments(path, sources)
}

// buildBookStateFromData creates a bookState from in-memory CONT KFX data.
// This is used after DRMION decryption produces a valid CONT container.
func buildBookStateFromData(contData []byte) (*bookState, error) {
	if len(contData) < 18 || !bytes.HasPrefix(contData, contSignature) {
		return nil, &UnsupportedError{Message: "data is not a valid CONT KFX container"}
	}

	source, err := loadContainerSourceData("<decrypted>", contData)
	if err != nil {
		return nil, err
	}

	return organizeFragments("<decrypted>", []*containerSource{source})
}

// Port of KFX_EPUB.organize_fragments_by_type (yj_to_epub.py) adapted to the Go fragmentCatalog layout.
// replace_ion_data symbol collection is approximated by recording resolved fragment IDs during the index walk.
func organizeFragments(bookPath string, sources []*containerSource) (*bookState, error) {
	fragments := fragmentCatalog{
		TitleMetadata:     nil,
		ContentFeatures:   map[string]interface{}{},
		DocumentData:      map[string]interface{}{},
		ContentFragments:  map[string][]string{},
		Storylines:        map[string]map[string]interface{}{},
		StyleFragments:    map[string]map[string]interface{}{},
		RubyGroups:        map[string]map[string]interface{}{},
		RubyContents:      map[string]map[string]interface{}{},
		SectionFragments:  map[string]sectionFragment{},
		AnchorFragments:   map[string]anchorFragment{},
		NavContainers:     map[string]map[string]interface{}{},
		ResourceFragments: map[string]resourceFragment{},
		ResourceRawData:   map[string]map[string]interface{}{},
		FormatCapabilities: map[string]map[string]interface{}{},
		Generators:        map[string]map[string]interface{}{},
		FontFragments:     map[string]fontFragment{},
		RawFragments:      map[string][]byte{},
		PositionAliases:   map[int]string{},
		FragmentIDsByType: map[string][]string{},
	}
	book := &decodedBook{
		Identifier: bookPath,
		Title:      strings.TrimSuffix(filepath.Base(bookPath), filepath.Ext(bookPath)),
		Language:   "en",
	}

	// Two-pass approach matching Calibre's yj_book.decode_book():
	//   Pass 1: container.deserialize() → loads doc_symbols into shared symtab
	//   Pass 2: container.get_fragments() → decodes entities with accumulated symtab
	//
	// Calibre processes all containers in loop 1 (loading symbols), then all
	// containers in loop 2 (decoding fragments). This ensures ALL docSymbols
	// are accumulated before any entity is decoded.
	sharedSym := &sharedDocSymbols{}

	// Sort sources alphabetically by path, matching Calibre's sequential
	// processing order.
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Path < sources[j].Path
	})

	// Pass 1: Accumulate docSymbols from all sources (Calibre: deserialize loop).
	for _, source := range sources {
		sharedSym.update(source.DocSymbols)
	}

	// Build resolver from fully accumulated docSymbols.
	srcDocSymbols := sharedSym.get()
	if len(srcDocSymbols) == 0 {
		return nil, &UnsupportedError{Message: "no document symbol table found in any container"}
	}
	resolver, err := newSymbolResolver(srcDocSymbols)
	if err != nil {
		return nil, err
	}

	bookSymbols := map[string]struct{}{}
	fontCount := 0
	categorizedData := map[string]map[string]bool{}

	// Pass 2: Decode all fragments using the fully accumulated resolver
	// (Calibre: get_fragments loop).
	for _, source := range sources {
		lastContainerID := ""
		for offset := 0; offset+24 <= len(source.IndexData); offset += 24 {
			idID := binary.LittleEndian.Uint32(source.IndexData[offset : offset+4])
			typeID := binary.LittleEndian.Uint32(source.IndexData[offset+4 : offset+8])
			entityOffset := int(binary.LittleEndian.Uint64(source.IndexData[offset+8 : offset+16]))
			entityLength := int(binary.LittleEndian.Uint64(source.IndexData[offset+16 : offset+24]))
			start := source.HeaderLen + entityOffset
			end := start + entityLength
			if start < 0 || end > len(source.Data) || start >= end {
				return nil, &UnsupportedError{Message: "entity offset is out of range"}
			}

			entityData := source.Data[start:end]
			fragmentID := resolver.Resolve(idID)
			bookSymbols[fragmentID] = struct{}{}
			fragmentType := fmt.Sprintf("$%d", typeID)
			payload, err := entityPayload(entityData)
			if err != nil {
				return nil, err
			}

			summaryID := fragmentID
			switch fragmentType {
			case "$270":
				value, err := decodeIonMap(payload, srcDocSymbols, resolver)
				if err != nil {
					return nil, err
				}
				containerID := fmt.Sprintf("%s:%s", asStringDefault(value["$161"]), asStringDefault(value["$409"]))
				lastContainerID = containerID
				summaryID = containerID
			case "$593":
				summaryID = lastContainerID
			case "$262":
				summaryID = fmt.Sprintf("%s-font-%03d", fragmentID, fontCount)
				fontCount++
			case "$258":
				// Python has no special ID override for $258 (yj_to_epub.py L186: id = fragment.fid).
				// The value is decoded later in the fragment type switch below.
				summaryID = fragmentID
			case "$387":
				value, err := decodeIonMap(payload, srcDocSymbols, resolver)
				if err != nil {
					return nil, err
				}
				summaryID = fmt.Sprintf("%s:%s", fragmentID, asStringDefault(value["$215"]))
			}
			fragments.FragmentIDsByType[fragmentType] = append(fragments.FragmentIDsByType[fragmentType], summaryID)

			// Track categorized IDs for duplicate/null detection (Python organize_fragments_by_type L202-204).
			if categorizedData[fragmentType] == nil {
				categorizedData[fragmentType] = map[string]bool{}
			}
			if categorizedData[fragmentType][summaryID] {
				log.Printf("kfx: book contains multiple %s fragments with id %s", fragmentType, summaryID)
			}
			categorizedData[fragmentType][summaryID] = true

			switch fragmentType {
			case "$145", "$157", "$164", "$258", "$259", "$260", "$262", "$266", "$270", "$391", "$490", "$538", "$585", "$593", "$608", "$609", "$756":
				value, err := decodeIonMap(payload, srcDocSymbols, resolver)
				if err != nil {
					return nil, err
				}

				switch fragmentType {
				case "$145":
					name, _ := asString(value["name"])
					stringsValue := toStringSlice(value["$146"])
					if name != "" && len(stringsValue) > 0 {
						fragments.ContentFragments[name] = stringsValue
					}
				case "$157":
					id := chooseFragmentIdentity(fragmentID, value["$173"])
					if id != "" {
						fragments.StyleFragments[id] = value
					}
				case "$164":
					mergeIonReferencedStringSymbols(value, bookSymbols)
					resource := parseResourceFragment(fragmentID, value)
					if resource.Location != "" {
						fragments.ResourceFragments[resource.ID] = resource
					}
					fragments.ResourceRawData[resource.ID] = value
				case "$258":
					order := readSectionOrder(value)
					if len(order) > 0 {
						fragments.SectionOrder = order
					}
					// Store $258 for applyReadingOrderMetadata (Python process_metadata L103: book_data.pop("$258", {})).
					fragments.ReadingOrderMetadata = value
				case "$259":
					id := chooseFragmentIdentity(fragmentID, value["$176"])
					if id != "" {
						fragments.Storylines[id] = value
					}
				case "$260":
					section := parseSectionFragment(fragmentID, value)
					if section.ID != "" && section.Storyline != "" {
						fragments.SectionFragments[section.ID] = section
					}
				case "$262":
					font := parseFontFragment(value)
					if font.Location != "" {
						fragments.FontFragments[font.Location] = font
					}
				case "$266":
					mergeIonReferencedStringSymbols(value, bookSymbols)
					anchor := parseAnchorFragment(fragmentID, value)
					if anchor.ID != "" && (anchor.PositionID != 0 || anchor.URI != "") {
						fragments.AnchorFragments[anchor.ID] = anchor
					}
				case "$270":
					fragments.Generators[summaryID] = value
				case "$391":
					id := chooseFragmentIdentity(fragmentID, value["$239"])
					if id != "" {
						fragments.NavContainers[id] = value
					}
				case "$490":
					fragments.TitleMetadata = value
				case "$538":
					fragments.DocumentData = value
				case "$585":
					fragments.ContentFeatures = value
				case "$593":
					fragments.FormatCapabilities[summaryID] = value
				case "$608":
					id := chooseFragmentIdentity(fragmentID, value["$758"])
					if id == "" {
						id = fragmentID
					}
					fragments.RubyContents[id] = value
				case "$756":
					id := chooseFragmentIdentity(fragmentID, value["$757"])
					if id == "" {
						id = fragmentID
					}
					fragments.RubyGroups[id] = value
				case "$609":
					sectionID := parsePositionMapSectionID(fragmentID, value)
					for _, positionID := range readPositionMap(value) {
						if positionID != 0 && sectionID != "" {
							fragments.PositionAliases[positionID] = sectionID
						}
					}
				}
			case "$389":
				value, err := decodeIonValue(payload, srcDocSymbols, resolver)
				if err != nil {
					return nil, err
				}
				if rootList, ok := asSlice(value); ok {
					for _, entry := range rootList {
						entryMap, ok := asMap(entry)
						if ok {
							fragments.NavRoots = append(fragments.NavRoots, entryMap)
						}
					}
				}
			case "$417", "$418":
				if fragmentID != "" {
					dataCopy := append([]byte(nil), payload...)
					fragments.RawFragments[fragmentID] = dataCopy
					fragments.RawBlobOrder = append(fragments.RawBlobOrder, rawBlob{
						ID:   fragmentID,
						Data: dataCopy,
					})
				}
			}
		}
	}

	// Null ID detection (Python organize_fragments_by_type L214):
	// When a category has multiple entries including an empty/null ID, log an error.
	for category, ids := range categorizedData {
		if len(ids) > 1 {
			if ids[""] || ids["\x00"] {
				log.Printf("kfx: fragment list contains mixed null/non-null ids of type %q", category)
			}
		}
	}

	// Port of replace_ion_data string-symbol discovery (yj_to_epub.py): collect YJ field string values into bookSymbols.
	mergeIonReferencedStringSymbols(fragments.TitleMetadata, bookSymbols)
	mergeIonReferencedStringSymbols(fragments.DocumentData, bookSymbols)
	mergeIonReferencedStringSymbols(fragments.ContentFeatures, bookSymbols)
	for _, m := range fragments.StyleFragments {
		mergeIonReferencedStringSymbols(m, bookSymbols)
	}
	for _, m := range fragments.Storylines {
		mergeIonReferencedStringSymbols(m, bookSymbols)
	}
	for _, m := range fragments.NavContainers {
		mergeIonReferencedStringSymbols(m, bookSymbols)
	}
	for _, m := range fragments.NavRoots {
		mergeIonReferencedStringSymbols(m, bookSymbols)
	}
	mergeContentFragmentStringSymbols(fragments.ContentFragments, bookSymbols)
	for _, m := range fragments.RubyGroups {
		mergeIonReferencedStringSymbols(m, bookSymbols)
	}
	for _, m := range fragments.RubyContents {
		mergeIonReferencedStringSymbols(m, bookSymbols)
	}
	for _, sec := range fragments.SectionFragments {
		mergeIonReferencedStringSymbols(sec.PageTemplateValues, bookSymbols)
		for _, t := range sec.PageTemplates {
			mergeIonReferencedStringSymbols(t.PageTemplateValues, bookSymbols)
			mergeIonReferencedStringSymbols(t.Condition, bookSymbols)
		}
	}

	// Port of Python process_document_data reading_orders: $169 from $538 document data.
	if len(fragments.SectionOrder) == 0 {
		if docOrder := readSectionOrder(fragments.DocumentData); len(docOrder) > 0 {
			fragments.SectionOrder = docOrder
		}
	}
	if len(fragments.SectionOrder) == 0 {
		for sectionID := range fragments.SectionFragments {
			fragments.SectionOrder = append(fragments.SectionOrder, sectionID)
		}
		sort.Strings(fragments.SectionOrder)
	}
	for fragmentType := range fragments.FragmentIDsByType {
		sort.Strings(fragments.FragmentIDsByType[fragmentType])
	}

	var primarySource *containerSource
	if len(sources) > 0 {
		primarySource = sources[0]
	}

	symbolFormat := determineBookSymbolFormat(bookSymbols, fragments.DocumentData, resolver)
	// Python yj_to_epub.py L239: log.info for non-SHORT book symbol format.
	if symbolFormat != symShort {
		log.Printf("kfx: Book symbol format is %s", symbolFormat)
	}
	// KFX_EPUB.__init__ L77–80 after determine_book_symbol_format (L76).
	applyKFXEPUBInitMetadataAfterOrganize(book, &fragments)

	return &bookState{
		Path:             bookPath,
		Source:           primarySource,
		Sources:          sources,
		Book:             book,
		Fragments:        fragments,
		BookSymbols:      bookSymbols,
		BookSymbolFormat: symbolFormat,
	}, nil
}

// ConvertFile converts a KFX file at inputPath to an EPUB at outputPath.
// Port of the convert pipeline from Python's KFX Input plugin.
func ConvertFile(inputPath, outputPath string, cacheDir string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}

	// Handle DRMION: decrypt to CONT first
	if bytes.HasPrefix(data, drmionSignature) {
		pageKey, err := FindPageKey(inputPath, cacheDir)
		if err != nil {
			return &DRMError{Message: err.Error()}
		}

		contData, err := decryptDRMION(data, pageKey)
		if err != nil {
			return &DRMError{Message: fmt.Sprintf("decryption failed: %s", err)}
		}

		return convertFromDRMIONData(contData, outputPath, inputPath, pageKey)
	}

	mode, reason, err := Classify(inputPath)
	if err != nil {
		return err
	}
	if mode == "blocked" {
		return &UnsupportedError{Message: "KFX book layout is not supported by the proof-of-concept converter: " + reason}
	}

	book, err := decodeKFX(inputPath)
	if err != nil {
		return err
	}
	if len(book.Sections) == 0 {
		return &UnsupportedError{Message: "no readable sections were extracted from the KFX file"}
	}

	return epub.Write(outputPath, epub.Book{
		Identifier:              book.Identifier,
		Title:                   book.Title,
		Language:                book.Language,
		Authors:                 book.Authors,
		Published:               book.Published,
		Description:             book.Description,
		Publisher:               book.Publisher,
		OverrideKindleFonts:     book.OverrideKindleFonts,
		CoverImageHref:          book.CoverImageHref,
		Stylesheet:              book.Stylesheet,
		Sections:                book.Sections,
		Resources:               book.Resources,
		Navigation:              book.Navigation,
		Guide:                   book.Guide,
		PageList:                book.PageList,
		GenerateEpub2Compatible: true, // Python: GENERATE_EPUB2_COMPATIBLE = True
	})
}

// ConvertFileWithTrace converts a KFX file and writes a debug trace.
func ConvertFileWithTrace(inputPath string, outputPath string, tracePath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}

	// Handle DRMION same as ConvertFile
	if bytes.HasPrefix(data, drmionSignature) {
		return fmt.Errorf("trace mode does not support DRMION files; use decrypted KFX-zip instead")
	}

	state, err := buildBookState(inputPath)
	if err != nil {
		return err
	}
	trace := newTraceWriter(inputPath)
	book, err := renderBookState(state, trace)
	if err != nil {
		return err
	}
	if err := trace.writeToFile(tracePath); err != nil {
		return fmt.Errorf("write trace: %w", err)
	}
	return epub.Write(outputPath, epub.Book{
		Identifier:              book.Identifier,
		Title:                   book.Title,
		Language:                book.Language,
		Authors:                 book.Authors,
		Published:               book.Published,
		Description:             book.Description,
		Publisher:               book.Publisher,
		OverrideKindleFonts:     book.OverrideKindleFonts,
		CoverImageHref:          book.CoverImageHref,
		Stylesheet:              book.Stylesheet,
		Sections:                book.Sections,
		Resources:               book.Resources,
		Navigation:              book.Navigation,
		Guide:                   book.Guide,
		PageList:                book.PageList,
		GenerateEpub2Compatible: true,
	})
}

func decodeKFX(path string) (*decodedBook, error) {
	state, err := buildBookState(path)
	if err != nil {
		return nil, err
	}
	if os.Getenv("KFX_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "docSymbols length=%d first=% x\n", len(state.Source.DocSymbols), state.Source.DocSymbols[:minInt(16, len(state.Source.DocSymbols))])
	}
	return renderBookState(state, nil)
}

// DecodeKFX decodes a KFX file and returns the decoded book.
func DecodeKFX(path string) (*decodedBook, error) {
	return decodeKFX(path)
}

func convertFromDRMIONData(contData []byte, outputPath string, originalPath string, pageKey []byte) error {
	if !bytes.HasPrefix(contData, contSignature) {
		return &UnsupportedError{Message: "decrypted data is not a valid CONT KFX container"}
	}

	// Build the primary source from decrypted CONT data
	primarySource, err := loadContainerSourceData(originalPath, contData)
	if err != nil {
		return fmt.Errorf("parsing decrypted CONT: %w", err)
	}

	// Collect additional blobs from the .sdr sidecar directory.
	// DRM books store metadata, cover images, and other fragments in
	// the sidecar (e.g. assets/metadata.kfx, assets/attachables/*.kfx).
	// Some books (e.g. The Familiars) have DRMION-encrypted metadata.kfx
	// that must also be decrypted.
	sources := []*containerSource{primarySource}
	sidecarRoot := strings.TrimSuffix(originalPath, filepath.Ext(originalPath)) + ".sdr"
	contBlobs, drmionBlobs, err := collectSidecarContainerBlobs(sidecarRoot)
	if err != nil {
		log.Printf("DRM sidecar collection failed for %s: %v", sidecarRoot, err)
	}
	for _, blob := range contBlobs {
		src, err := loadContainerSourceData(blob.Path, blob.Data)
		if err != nil {
			log.Printf("skipping sidecar blob %s: %v", blob.Path, err)
			continue
		}
		sources = append(sources, src)
	}

	// Decrypt DRMION sidecar blobs using the same page key.
	// These may contain the document symbol table needed for the main content.
	for _, blob := range drmionBlobs {
		decrypted, decErr := decryptDRMION(blob.Data, pageKey)
		if decErr != nil {
			log.Printf("skipping DRMION sidecar %s: %v", blob.Path, decErr)
			continue
		}

		// Try LZMA decompression if the decrypted data doesn't start with CONT
		if !bytes.HasPrefix(decrypted, contSignature) && len(decrypted) > 1 && decrypted[0] == 0x00 {
			decompressed, lzmaErr := lzmaDecompress(decrypted[1:])
			if lzmaErr == nil && bytes.HasPrefix(decompressed, contSignature) {
				decrypted = decompressed
			}
		}

		if !bytes.HasPrefix(decrypted, contSignature) {
			log.Printf("DRMION sidecar %s: not CONT after decryption, skipping", blob.Path)
			continue
		}

		src, err := loadContainerSourceData(blob.Path, decrypted)
		if err != nil {
			log.Printf("skipping decrypted sidecar %s: %v", blob.Path, err)
			continue
		}

		// Validate entity offsets — decrypted metadata.kfx may have mismatched
		// offsets. If any entity is out of range, only use its docSymbols,
		// not its fragments.
		if !validateEntityOffsets(src) {
			log.Printf("DRM: decrypted sidecar %s has invalid entity offsets, using docSymbols only (%d bytes)", blob.Path, len(src.DocSymbols))
			// Create a minimal source with docSymbols but empty index
			// so the two-pass symbol accumulation picks up the symbols.
			sources = append(sources, &containerSource{
				Path:       src.Path,
				DocSymbols: src.DocSymbols,
			})
			continue
		}

		sources = append(sources, src)
		log.Printf("DRM: decrypted sidecar %s (%d bytes)", blob.Path, len(decrypted))
	}

	if len(sources) > 1 {
		log.Printf("DRM conversion using %d sources (1 decrypted + %d sidecar)", len(sources), len(sources)-1)
	}

	// Use the original path as book identifier (for title fallback)
	bookPath := originalPath
	state, err := organizeFragments(bookPath, sources)
	if err != nil {
		return err
	}

	book, err := renderBookState(state, nil)
	if err != nil {
		return err
	}
	if len(book.Sections) == 0 {
		return &UnsupportedError{Message: "no readable sections were extracted from the decrypted KFX file"}
	}

	return epub.Write(outputPath, epub.Book{
		Identifier:          book.Identifier,
		Title:               book.Title,
		Language:            book.Language,
		Authors:             book.Authors,
		Published:           book.Published,
		Description:         book.Description,
		Publisher:           book.Publisher,
		OverrideKindleFonts: book.OverrideKindleFonts,
		CoverImageHref:      book.CoverImageHref,
		Stylesheet:          book.Stylesheet,
		Sections:            book.Sections,
		Resources:           book.Resources,
		Navigation:          book.Navigation,
		Guide:               book.Guide,
		PageList:                book.PageList,
		GenerateEpub2Compatible: true, // Python: GENERATE_EPUB2_COMPATIBLE = True
	})
}

func convertFromCONTData(contData []byte, outputPath string) error {
	if !bytes.HasPrefix(contData, contSignature) {
		return &UnsupportedError{Message: "decrypted data is not a valid CONT KFX container"}
	}

	// Feed the CONT data through the normal decode pipeline
	state, err := buildBookStateFromData(contData)
	if err != nil {
		return err
	}

	book, err := renderBookState(state, nil)
	if err != nil {
		return err
	}
	if len(book.Sections) == 0 {
		return &UnsupportedError{Message: "no readable sections were extracted from the decrypted KFX file"}
	}

	return epub.Write(outputPath, epub.Book{
		Identifier:          book.Identifier,
		Title:               book.Title,
		Language:            book.Language,
		Authors:             book.Authors,
		Published:           book.Published,
		Description:         book.Description,
		Publisher:           book.Publisher,
		OverrideKindleFonts: book.OverrideKindleFonts,
		CoverImageHref:      book.CoverImageHref,
		Stylesheet:          book.Stylesheet,
		Sections:            book.Sections,
		Resources:           book.Resources,
		Navigation:          book.Navigation,
		Guide:               book.Guide,
		PageList:                book.PageList,
		GenerateEpub2Compatible: true, // Python: GENERATE_EPUB2_COMPATIBLE = True
	})
}

// styleBaseName returns a simplified class base name from a style ID, applying
// uniquePartOfLocalSymbol to strip the symbol-format prefix (ORIGINAL: V_N_N-PARA-…, etc.)
// matching Calibre's simplify_styles class naming behavior.

// singleImageWrapperChild returns the <div> wrapper if the container has exactly one child
// that is a <div> containing a single <img>. Returns nil otherwise.

// blockAlignedContainerProperties matches Python's BLOCK_ALIGNED_CONTAINER_PROPERTIES
// (yj_to_epub_content.py:49-55).

// reverseHeritablePropertiesExcludes are removed from heritableProperties to produce
// REVERSE_HERITABLE_PROPERTIES (yj_to_epub_properties.py:994).

// isBlockContainerProperty returns true if the CSS property belongs on the wrapper container
// rather than the image element. Matches Python's BLOCK_CONTAINER_PROPERTIES partition
// (yj_to_epub_content.py:57): REVERSE_HERITABLE_PROPERTIES | BLOCK_ALIGNED_CONTAINER_PROPERTIES | {"display"}.

func applyCoverSVGPromotion(book *decodedBook, resolvedDefaultFont string) {
	if book == nil || book.CoverImageHref == "" {
		return
	}
	width, height := coverImageDimensions(book.Resources, book.CoverImageHref)
	if width == 0 || height == 0 {
		return
	}
	coverFound := false
	for index := range book.Sections {
		section := &book.Sections[index]
		// Match cover section by either title or containing the cover image.
		// Calibre identifies cover in process_section via layout + image, not title alone.
		if !strings.Contains(section.BodyHTML, `src="`+book.CoverImageHref+`"`) {
			continue
		}
		// Only promote sections that are primarily a cover image (not mixed content).
		if section.Title != "Cover" && !isCoverImageSection(section.BodyHTML) {
			continue
		}
		coverFound = true
		section.Properties = "svg"
		section.BodyHTML = fmt.Sprintf(
			`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" version="1.1" preserveAspectRatio="xMidYMid meet" viewBox="0 0 %d %d" height="100%%" width="100%%"><image xlink:href="%s" height="%d" width="%d"/></svg>`,
			width, height, escapeHTML(book.CoverImageHref), height, width,
		)
		// Python adds class_s8 with font-family only when the resolved default font is
		// not "serif" (the CSS heritable default). When the default is just "serif",
		// Python's set_html_defaults skips cover pages and no font-family is emitted.
		// Match Python behavior: only add class_s8 when a non-generic font is used.
		if resolvedDefaultFont != "serif" {
			section.BodyClass = "class_s8"
		} else {
			section.BodyClass = ""
		}
		break
	}
	if !coverFound {
		return
	}
	// Add the class_s8 CSS rule only when using a non-generic default font.
	// Python's cover sections only get font-family when the resolved default is not "serif".
	if resolvedDefaultFont == "serif" {
		return
	}
	classS8Rule := ".class_s8 {font-family: " + resolvedDefaultFont + "}"
	if !strings.Contains(book.Stylesheet, ".class_s8 {") {
		if book.Stylesheet != "" {
			book.Stylesheet += "\n"
		}
		book.Stylesheet += classS8Rule
	} else {
		lines := strings.Split(book.Stylesheet, "\n")
		for index, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), ".class_s8 {") {
				lines[index] = classS8Rule
			}
		}
		book.Stylesheet = strings.Join(lines, "\n")
	}
}

func coverImageDimensions(resources []epub.Resource, href string) (int, int) {
	for _, resource := range resources {
		if resource.Filename != href {
			continue
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(resource.Data))
		if err != nil {
			return 0, 0
		}
		return cfg.Width, cfg.Height
	}
	return 0, 0
}

// isCoverImageSection returns true if the body HTML is primarily just an image
// (possibly wrapped in a div), suitable for SVG cover promotion.
func isCoverImageSection(bodyHTML string) bool {
	stripped := strings.TrimSpace(bodyHTML)
	// Remove opening/closing div wrapper
	stripped = strings.TrimPrefix(stripped, "<div>")
	stripped = strings.TrimSuffix(stripped, "</div>")
	stripped = strings.TrimSpace(stripped)
	return strings.HasPrefix(stripped, "<img") && !strings.Contains(stripped, "<p>") && !strings.Contains(stripped, "<h")
}

func normalizeBookIdentifier(identifier string) string {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return trimmed
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "urn:asin:") {
		return trimmed
	}
	return "urn:asin:" + trimmed
}

func normalizeLanguage(language string) string {
	trimmed := strings.TrimSpace(language)
	if trimmed == "" {
		return "en"
	}
	if len(trimmed) > 2 && trimmed[2] == '_' {
		trimmed = strings.ReplaceAll(trimmed, "_", "-")
	}
	prefix, suffix, found := strings.Cut(trimmed, "-")
	if !found {
		return strings.ToLower(trimmed)
	}
	prefix = strings.ToLower(prefix)
	if len(suffix) < 4 {
		suffix = strings.ToUpper(suffix)
	} else {
		suffix = strings.ToUpper(suffix[:1]) + suffix[1:]
	}
	return prefix + "-" + suffix
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
