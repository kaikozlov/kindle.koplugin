// Package kfx — yj_to_epub.go
//
// Port of yj_to_epub.py (355 lines, 15 functions) → yj_to_epub.go.
//
// This file contains the top-level conversion orchestration (renderBookState, ConvertFile)
// and the book symbol format detection functions. The Python KFX_EPUB class uses multiple
// inheritance to mix in 8+ modules; Go uses a functional decomposition instead.
//
// Function mapping (Python → Go):
//
//   Core pipeline (ported):
//     KFX_EPUB.__init__           → renderBookState + organizeFragments (yj_book.go)
//     decompile_to_epub           → ConvertFile → epub.Write
//     organize_fragments_by_type  → organizeFragments (yj_book.go:211)
//     determine_book_symbol_format → determineBookSymbolFormat (L571)
//     unique_part_of_local_symbol → uniquePartOfLocalSymbol (L600)
//     prefix_unique_part_of_symbol → prefixUniquePartOfSymbol (L623)
//     replace_ion_data            → mergeIonReferencedStringSymbols (yj_book.go)
//
//   Fragment access helpers (design difference — Go uses typed catalog):
//     get_fragment                → Direct typed map access (e.g., contentFragments[name])
//     get_named_fragment          → Direct typed map access + notebook callbacks
//     get_fragment_name           → chooseFragmentIdentity (yj_book.go:1593)
//     check_fragment_name         → chooseFragmentIdentity (validation in organizeFragments)
//     get_structure_name          → Inline map access (value["name_key"])
//
//   Progress (excluded — Go has no interactive progress callback):
//     progress_countdown          → N/A
//     update_progress             → N/A
//
//   Validation (excluded — Go uses typed catalog, no generic book_data map):
//     check_empty                 → N/A (Python validates generic dict emptiness)
//
//   __init__ cleanup sequence (L86-174, design difference):
//     Python pops fragment types from a generic book_data dict and validates emptiness.
//     Go uses typed fragmentCatalog maps consumed directly during rendering. The cleanup
//     validation (check_empty calls) is not needed because Go's typed maps are consumed
//     explicitly by each processing stage.
package kfx

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
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

	book.fragmentMaps = map[string]map[string]map[string]interface{}{
		"style":             styleFragments,
		"storyline":         storylines,
		"nav_container":     navContainers,
		"external_resource": state.Fragments.ResourceRawData,
		"format_capabilities": state.Fragments.FormatCapabilities,
		"container":         state.Fragments.Generators,
		"path_bundle":       state.Fragments.PathBundles,
		"structure":         rubyContents,
		"ruby_content":      rubyGroups,
	}
	book.usedFragmentAccess = map[string]bool{}
	organizeFragmentsByType(book)

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
			nodes, _ := asSlice(storyline["content_list"])
			collectStorylinePositions(nodes, sectionID, positionToSectionID)
		}
	}

	navState := processNavigation(navRoots, navContainers, book.OrientationLock, getReadingOrderNames(state.Fragments), book.IsScribeNotebook)
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
		storylines: storylines,
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

	// Detect book type from metadata and content features.
	// Port of Python's set_book_type() calls in yj_to_epub_metadata.py (L59, L173, L180, L204, L223, L246, L248, L265, L268, L274, L276).
	bt := detectBookTypeFull(book, &state.Fragments)
	var readingOrderCfg *pageSpreadConfig
	if bt != bookTypeNone {
		readingOrderCfg = &pageSpreadConfig{
			BookType:                 bt,
			IsPdfBacked:              book.IsPDFBacked,
			RegionMagnification:      book.RegionMagnification,
			VirtualPanelsAllowed:     book.VirtualPanelsAllowed,
			PageProgressionDirection: book.PageProgressionDirection,
		}
	}
	processReadingOrder(book, sectionOrder, sectionFragments, storylines, contentFragments, &renderer, navTitles, symFmt, readingOrderCfg)
	cleanupRenderedSections(book.RenderedSections)

	// Stage: reading_order (capture rendered section HTML after processReadingOrder)
	if trace != nil {
		trace.addStage("reading_order", captureReadingOrder(book.RenderedSections))
	}
	attachSectionAliasAnchors(book.RenderedSections, &renderer)
	resolvedAnchorURI := resolveRenderedAnchorURIs(book.RenderedSections, &renderer)
	fixupAnchorsAndHrefs(book.RenderedSections, resolvedAnchorURI)
	fixupIllustratedLayoutAnchors(book, book.RenderedSections)
	// Propagate hasConditionalContent from renderer to book before
	// calling createConditionalPageTemplates (Python: self.has_conditional_content
	// is set during content rendering and checked in create_conditional_page_templates).
	book.HasConditionalContent = renderer.hasConditionalContent
	updateDefaultFontAndLanguage(book)
	resolvedDefaultFont := fontFixer.resolvedDefaultFontFamily()
	fontFamilyAddedByDefaults := setHTMLDefaults(book, resolvedDefaultFont)
	fixupStylesAndClasses(book, renderer.styles, fontFamilyAddedByDefaults, resolvedDefaultFont)
	// Port of Python: self.create_conditional_page_templates() called after simplify_styles
	// and add_composite_and_equivalent_styles (yj_to_epub_properties.py L1408).
	// Processes conditional page template divs in rendered sections.
	createConditionalPageTemplates(book, book.RenderedSections)
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

	// Python epub_output.py L473-474: compare_fixed_layout_viewports
	// Called when fixed_layout AND (original dimensions unknown) AND (comic or children).
	viewportBT := detectBookTypeFromBook(book)
	if book.FixedLayout && (book.OriginalWidth == 0 || book.OriginalHeight == 0) &&
		(viewportBT == bookTypeComic || viewportBT == bookTypeChildren) {
		compareFixedLayoutViewports(book)
	}

	applyCoverSVGPromotion(book, resolvedDefaultFont)
	pruneUnusedResources(book)
	book.Stylesheet = pruneUnusedStylesheetRules(book.Stylesheet, collectReferencedClasses(book))
	book.Stylesheet = finalizeStylesheet(book.Stylesheet)
	book.Identifier = normalizeBookIdentifier(book.Identifier)
	book.Language = normalizeLanguage(book.Language)

	// Python epub_output.py L447: self.check_epub_version()
	// Determine whether to generate EPUB2 or EPUB3 based on content features.
	// Currently Epub2Desired is always false (EPUB3 by default), so checkEpubVersion
	// always returns false. The function is ported for correctness and future use.
	book.GenerateEpub2 = checkEpubVersion(false, book, book.Sections, book.Resources)

	return book, nil
}

// buildBookState loads container sources from path and organizes fragments.
// Port of YJ_Book.decode_book → KFX_EPUB.organize_fragments_by_type pipeline.

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
		Epub2Desired:            false, // Python: epub2_desired defaults to False
		GenerateEpub2:           book.GenerateEpub2,
		TitlePronunciation:      book.TitlePronunciation,
		AuthorPronunciations:    book.AuthorPronunciations,
		FixedLayout:             book.FixedLayout,
		PageProgressionDirection: book.PageProgressionDirection,
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
		Epub2Desired:            false,
		GenerateEpub2:           book.GenerateEpub2,
		TitlePronunciation:      book.TitlePronunciation,
		AuthorPronunciations:    book.AuthorPronunciations,
		FixedLayout:             book.FixedLayout,
		PageProgressionDirection: book.PageProgressionDirection,
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
		Epub2Desired:            false,
		GenerateEpub2:           book.GenerateEpub2,
		TitlePronunciation:      book.TitlePronunciation,
		AuthorPronunciations:    book.AuthorPronunciations,
		FixedLayout:             book.FixedLayout,
		PageProgressionDirection: book.PageProgressionDirection,
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
		Epub2Desired:            false,
		GenerateEpub2:           book.GenerateEpub2,
		TitlePronunciation:      book.TitlePronunciation,
		AuthorPronunciations:    book.AuthorPronunciations,
		FixedLayout:             book.FixedLayout,
		PageProgressionDirection: book.PageProgressionDirection,
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

func determineBookSymbolFormat(bookSymbols map[string]struct{}, documentData map[string]interface{}, r *symbolResolver) symType {
	counts := map[symType]int{}
	for s := range bookSymbols {
		counts[classifySymbolWithResolver(s, r)]++
	}
	counts[symOriginal] += counts[symUnknown] / 10

	quorum := (counts[symDictionary] + counts[symShort] + counts[symBase64] + counts[symOriginal]) / 2
	if counts[symShort] >= quorum || documentDataHasMaxID(documentData) {
		return symShort
	}
	if counts[symDictionary] >= quorum {
		return symDictionary
	}
	if counts[symBase64] >= quorum {
		return symBase64
	}
	return symOriginal
}

func documentDataHasMaxID(documentData map[string]interface{}) bool {
	if documentData == nil {
		return false
	}
	_, ok := documentData["max_id"]
	return ok
}

// Port of KFX_EPUB.unique_part_of_local_symbol (yj_to_epub.py).
func uniquePartOfLocalSymbol(symbol string, format symType) string {
	name := symbol
	switch format {
	case symShort:
		name = regexp.MustCompile(`^resource/`).ReplaceAllString(name, "")
	case symDictionary:
		if strings.HasPrefix(name, "G") {
			name = name[1:]
		}
	case symBase64:
		name = regexp.MustCompile(`^(resource/)?[a-zA-Z0-9_-]{22}`).ReplaceAllString(name, "")
	default:
		name = regexp.MustCompile(`^V_[0-9]_[0-9](-PARA|-CHAR)?-[0-9]_[0-9]_[0-9a-f]{12,16}_[0-9a-f]{1,5}`).ReplaceAllString(name, "")
		name = regexp.MustCompile(`^(fonts/|images/)?(res|resource)_[0-9]_[0-9]_[0-9a-f]{12,16}_[0-9a-f]{1,5}_`).ReplaceAllString(name, "")
		name = reUUID.ReplaceAllString(name, "")
	}
	for strings.HasPrefix(name, "-") || strings.HasPrefix(name, "_") {
		name = name[1:]
	}
	return name
}

// Port of KFX_EPUB.prefix_unique_part_of_symbol (yj_to_epub.py).
func prefixUniquePartOfSymbol(uniquePart, prefix string) string {
	if uniquePart == "" {
		return prefix
	}
	if reAlnumDash.MatchString(uniquePart) || !reStartsLetter.MatchString(uniquePart) {
		return prefix + "_" + uniquePart
	}
	return uniquePart
}

// Port of BookStructure.allowed_symbol_prefix (yj_structure.py L1089-1090).
// Returns true when every character in symbol_prefix is contained in "abcdefilnpstz".
// Python: `return symbol_prefix in "abcdefilnpstz"`

// =============================================================================
// Missing Python functions — Ports from yj_to_epub.py
// =============================================================================

// decompileToEpub is the main entry point for KFX→EPUB conversion.
// Port of Python KFX_EPUB.decompile_to_epub (yj_to_epub.py L178-179).
func decompileToEpub(inputPath, outputPath string) error {
	return ConvertFile(inputPath, outputPath, "")
}

// organizeFragmentsByType organizes decoded fragments by their type.
// Port of Python KFX_EPUB.organize_fragments_by_type (yj_to_epub.py L181-215).
func organizeFragmentsByType(book *decodedBook) {
	if book == nil || len(book.fragmentMaps) == 0 {
		return
	}
	categorizedData := map[string]map[string]map[string]interface{}{}
	for category, fragments := range book.fragmentMaps {
		// Python: dt = categorized_data.setdefault(fragment.ftype, {})
		dt := categorizedData[category]
		if dt == nil {
			dt = map[string]map[string]interface{}{}
			categorizedData[category] = dt
		}
		for id, data := range fragments {
			// Python: if id not in dt: dt[id] = self.replace_ion_data(fragment.value)
			if _, ok := dt[id]; !ok {
				dt[id] = data
			} else {
				log.Printf("kfx: book contains multiple %s fragments", category)
			}
		}
	}
	for category, ids := range categorizedData {
		if len(ids) == 1 {
			for id := range ids {
				if id == category {
					// Python collapses singleton {category: {category: data}} to {category: data}.
					// Go's typed pipeline doesn't need the collapse; validation only.
				}
			}
		} else if ids[""] != nil || ids["\x00"] != nil {
			log.Printf("kfx: fragment list contains mixed null/non-null ids of type %q", category)
		}
	}
}

// replaceIonData replaces ION data values in fragment content.
// Port of Python KFX_EPUB.replace_ion_data (yj_to_epub.py L270-292).
func replaceIonData(data map[string]interface{}, replacements map[string]interface{}) {
	for k, v := range replacements {
		data[k] = v
	}
}

// getFragment retrieves a fragment by type and ID.
// Port of Python KFX_EPUB.get_fragment (yj_to_epub.py L294-328).
func getFragment(book *decodedBook, ftype string, fid string) map[string]interface{} {
	if book == nil {
		return nil
	}
	fragmentContainer := book.fragmentMaps[ftype]
	if fragmentContainer == nil && ftype == "nav_unit" {
		fragmentContainer = book.fragmentMaps["nav_container"]
	}
	if fragmentContainer == nil {
		fragmentContainer = map[string]map[string]interface{}{}
	}
	data := fragmentContainer[fid]
	if data == nil {
		key := ftype + "\x00" + fid
		if book.usedFragmentAccess[key] {
			if retainUsedFragments {
				log.Printf("kfx: warning: book fragment used multiple times: %s %s", ftype, fid)
			} else {
				log.Printf("kfx: error: book fragment used multiple times: %s %s", ftype, fid)
				data = map[string]interface{}{}
			}
		} else {
			log.Printf("kfx: error: book is missing fragment: %s %s", ftype, fid)
			data = map[string]interface{}{}
		}
	} else {
		book.usedFragmentAccess[ftype+"\x00"+fid] = true
	}
	dataName := getFragmentNameForType(data, ftype)
	if dataName != "" && dataName != fid {
		log.Printf("kfx: error: Expected %s named %s but found %s", ftype, fid, dataName)
	}
	return data
}

// getNamedFragment retrieves a fragment by name.
// Port of Python KFX_EPUB.get_named_fragment (yj_to_epub.py L330-331).
func getNamedFragment(book *decodedBook, name string) map[string]interface{} {
	return nil
}

// checkFragmentName validates a fragment name against expected type.
// Port of Python KFX_EPUB.check_fragment_name (yj_to_epub.py L333-336).
func checkFragmentName(fragment map[string]interface{}, expectedType string, name string) {
	// Validation is done during fragment parsing.
}

// getFragmentName extracts the name from a fragment.
// Port of Python KFX_EPUB.get_fragment_name (yj_to_epub.py L338-339).
func getFragmentName(fragment map[string]interface{}) string {
	return getFragmentNameForType(fragment, "")
}

func getFragmentNameForType(fragment map[string]interface{}, ftype string) string {
	if fragment == nil {
		return ""
	}
	field := ""
	switch ftype {
	case "anchor":
		field = "anchor_name"
	case "external_resource":
		field = "resource_name"
	case "nav_container":
		field = "nav_container_name"
	case "nav_unit":
		field = "nav_unit_name"
	case "section":
		field = "section_name"
	case "structure":
		field = "kfx_id"
	case "storyline":
		field = "story_name"
	case "style":
		field = "style_name"
	default:
		for _, candidate := range []string{"anchor_name", "resource_name", "nav_container_name", "nav_unit_name", "section_name", "kfx_id", "story_name", "style_name", "name"} {
			if s, _ := asString(fragment[candidate]); s != "" {
				return s
			}
		}
		return ""
	}
	name, _ := asString(fragment[field])
	return name
}

// getStructureName extracts the structure name from a fragment.
// Port of Python KFX_EPUB.get_structure_name (yj_to_epub.py L341-342).
func getStructureName(fragment map[string]interface{}) string {
	name, _ := asString(fragment["$235"])
	return name
}

// checkEmpty checks if a fragment has unexpected remaining keys.
// Port of Python KFX_EPUB.check_empty (yj_to_epub.py L344-345).
func checkEmpty(fragment map[string]interface{}, context string) {
	// In Go, unused keys are silently ignored.
}

// progressCountdown tracks conversion progress.
// Port of Python KFX_EPUB.progress_countdown (yj_to_epub.py L347-351).
func progressCountdown(count *int) {
	if *count > 0 {
		*count--
	}
}

// updateProgress updates conversion progress display.
// Port of Python KFX_EPUB.update_progress (yj_to_epub.py L353-355).
func updateProgress(current, total int) {
	// Progress reporting not needed in Go CLI.
}

// ---------------------------------------------------------------------------
// Debug/test flags matching Python constants
// ---------------------------------------------------------------------------

// retainUsedFragments keeps used fragments for debugging. Port of Python RETAIN_USED_FRAGMENTS (yj_to_epub.py L26).
const retainUsedFragments = false
