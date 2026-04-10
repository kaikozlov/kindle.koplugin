package kfx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/kaikozlov/kindle-koplugin/internal/epub"
	"github.com/kaikozlov/kindle-koplugin/internal/jxr"
)

var (
	contSignature     = []byte("CONT")
	drmionSignature   = []byte{0xea, 'D', 'R', 'M', 'I', 'O', 'N', 0xee}
	ionVersionMarker  = []byte{0xe0, 0x01, 0x00, 0xea}
	yjPreludeOnce     sync.Once
	yjPreludeData     []byte
	yjPreludeErr      error
	cssIdentPattern   = regexp.MustCompile(`^[-_a-zA-Z0-9]*$`)
	styleTokenPattern = regexp.MustCompile(`__STYLE_\d+__`)
)

const ionSystemSymbolCount = 9

type DRMError struct {
	Message string
}

func (e *DRMError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "DRM is present"
}

type UnsupportedError struct {
	Message string
}

func (e *UnsupportedError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "unsupported KFX layout"
}

type decodedBook struct {
	Title               string
	Language            string
	Authors             []string
	Identifier          string
	Published           string
	OverrideKindleFonts bool
	CoverImageID        string
	CoverImageHref      string
	Stylesheet          string
	ResourceHrefByID    map[string]string
	Sections            []epub.Section
	Resources           []epub.Resource
	Navigation          []epub.NavPoint
	Guide               []epub.GuideEntry
	PageList            []epub.PageTarget
}

type renderedStoryline struct {
	BodyHTML   string
	BodyClass  string
	Properties string
}

type htmlPart interface{}

type htmlText struct {
	Text string
}

type htmlElement struct {
	Tag      string
	Attrs    map[string]string
	Children []htmlPart
}

type resourceFragment struct {
	ID        string
	Location  string
	MediaType string
}

type fontFragment struct {
	Location string
	Family   string
	Style    string
	Weight   string
	Stretch  string
}

type symbolResolver struct {
	localStart uint32
	locals     []string
}

type rawBlob struct {
	ID   string
	Data []byte
}

type sectionFragment struct {
	ID                 string
	PositionID         int
	Storyline          string
	PageTemplateStyle  string
	PageTemplateValues map[string]interface{}
}

type anchorFragment struct {
	ID         string
	PositionID int
	URI        string
}

type navTarget struct {
	PositionID int
	Offset     int
}

type navPoint struct {
	Title    string
	Target   navTarget
	Children []navPoint
}

type guideEntry struct {
	Type   string
	Title  string
	Target navTarget
}

type pageEntry struct {
	Label  string
	Target navTarget
}

type storylineRenderer struct {
	contentFragments   map[string][]string
	resourceHrefByID   map[string]string
	anchorToFilename   map[string]string
	positionToSection  map[int]string
	positionAnchors    map[int]map[int][]string
	positionAnchorID   map[int]map[int]string
	emittedAnchorIDs   map[string]bool
	styleFragments     map[string]map[string]interface{}
	styles             *styleCatalog
	activeBodyClass    string
	activeBodyDefaults map[string]bool
	firstVisibleSeen   bool
}

type styleCatalog struct {
	staticRules  map[string]string
	entries      []*styleEntry
	byKey        map[string]*styleEntry
	byToken      map[string]*styleEntry
	finalized    bool
	replacements []string
	css          string
}

type styleEntry struct {
	token        string
	baseName     string
	declarations string
	count        int
	order        int
	finalName    string
	referenced   bool
}

type fontNameFixer struct {
	fixedNames       map[string]string
	nameReplacements map[string]string
}

var currentFontFixer *fontNameFixer

var cssGenericFontNames = map[string]bool{
	"serif":      true,
	"sans-serif": true,
	"cursive":    true,
	"fantasy":    true,
	"monospace":  true,
}

func Classify(path string) (openMode string, blockReason string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}

	switch {
	case bytes.HasPrefix(data, drmionSignature):
		return "blocked", "drm", nil
	case !bytes.HasPrefix(data, contSignature):
		return "blocked", "unsupported_kfx_layout", nil
	}

	sidecarRoot := strings.TrimSuffix(path, filepath.Ext(path)) + ".sdr"
	if _, err := os.Stat(filepath.Join(sidecarRoot, "assets", "voucher")); err == nil {
		return "blocked", "drm", nil
	}

	attachablesDir := filepath.Join(sidecarRoot, "assets", "attachables")
	if entries, err := os.ReadDir(attachablesDir); err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(strings.ToLower(entry.Name()), ".kfx") {
				return "blocked", "unsupported_kfx_layout", nil
			}
		}
	}

	return "convert", "", nil
}

func ConvertFile(inputPath, outputPath string) error {
	mode, reason, err := Classify(inputPath)
	if err != nil {
		return err
	}
	if mode == "blocked" {
		if reason == "drm" {
			return &DRMError{Message: "DRM-protected KFX is not supported"}
		}
		return &UnsupportedError{Message: "KFX book layout is not supported by the proof-of-concept converter"}
	}

	book, err := decodeKFX(inputPath)
	if err != nil {
		return err
	}
	if len(book.Sections) == 0 {
		return &UnsupportedError{Message: "no readable sections were extracted from the KFX file"}
	}

	return epub.Write(outputPath, epub.Book{
		Identifier:          book.Identifier,
		Title:               book.Title,
		Language:            book.Language,
		Authors:             book.Authors,
		Published:           book.Published,
		OverrideKindleFonts: book.OverrideKindleFonts,
		CoverImageHref:      book.CoverImageHref,
		Stylesheet:          book.Stylesheet,
		Sections:            book.Sections,
		Resources:           book.Resources,
		Navigation:          book.Navigation,
		Guide:               book.Guide,
		PageList:            book.PageList,
	})
}

func decodeKFX(path string) (*decodedBook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 18 || !bytes.HasPrefix(data, contSignature) {
		return nil, &UnsupportedError{Message: "file is not a CONT KFX container"}
	}

	headerLen := int(binary.LittleEndian.Uint32(data[6:10]))
	containerInfoOffset := int(binary.LittleEndian.Uint32(data[10:14]))
	containerInfoLength := int(binary.LittleEndian.Uint32(data[14:18]))
	if headerLen <= 0 || containerInfoOffset+containerInfoLength > len(data) {
		return nil, &UnsupportedError{Message: "container header is invalid"}
	}

	containerInfo, err := decodeIonMap(data[containerInfoOffset:containerInfoOffset+containerInfoLength], nil, nil)
	if err != nil {
		return nil, err
	}

	docSymbolOffset, ok := asInt(containerInfo["$415"])
	if !ok {
		return nil, &UnsupportedError{Message: "KFX document symbol table is missing"}
	}
	docSymbolLength, ok := asInt(containerInfo["$416"])
	if !ok {
		return nil, &UnsupportedError{Message: "KFX document symbol table length is missing"}
	}
	indexOffset, ok := asInt(containerInfo["$413"])
	if !ok {
		return nil, &UnsupportedError{Message: "KFX index table is missing"}
	}
	indexLength, ok := asInt(containerInfo["$414"])
	if !ok {
		return nil, &UnsupportedError{Message: "KFX index table length is missing"}
	}

	if docSymbolOffset+docSymbolLength > len(data) || indexOffset+indexLength > len(data) {
		return nil, &UnsupportedError{Message: "KFX offsets are out of range"}
	}

	docSymbols := data[docSymbolOffset : docSymbolOffset+docSymbolLength]
	indexData := data[indexOffset : indexOffset+indexLength]
	if os.Getenv("KFX_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "docSymbols offset=%d length=%d first=% x\n", docSymbolOffset, docSymbolLength, docSymbols[:minInt(16, len(docSymbols))])
	}
	resolver, err := newSymbolResolver(docSymbols)
	if err != nil {
		return nil, err
	}

	contentFragments := map[string][]string{}
	storylines := map[string]map[string]interface{}{}
	styleFragments := map[string]map[string]interface{}{}
	sectionFragments := map[string]sectionFragment{}
	anchors := map[string]anchorFragment{}
	navContainers := map[string]map[string]interface{}{}
	var navRoots []map[string]interface{}
	resourceFragments := map[string]resourceFragment{}
	fontFragments := map[string]fontFragment{}
	rawFragments := map[string][]byte{}
	positionAliases := map[int]string{}
	var rawBlobOrder []rawBlob
	var sectionOrder []string
	book := &decodedBook{
		Identifier: path,
		Title:      strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Language:   "en",
	}

	for offset := 0; offset+24 <= len(indexData); offset += 24 {
		idID := binary.LittleEndian.Uint32(indexData[offset : offset+4])
		typeID := binary.LittleEndian.Uint32(indexData[offset+4 : offset+8])
		entityOffset := int(binary.LittleEndian.Uint64(indexData[offset+8 : offset+16]))
		entityLength := int(binary.LittleEndian.Uint64(indexData[offset+16 : offset+24]))
		start := headerLen + entityOffset
		end := start + entityLength
		if start < 0 || end > len(data) || start >= end {
			return nil, &UnsupportedError{Message: "entity offset is out of range"}
		}

		entityData := data[start:end]
		fragmentID := resolver.Resolve(idID)
		fragmentType := fmt.Sprintf("$%d", typeID)
		payload, err := entityPayload(entityData)
		if err != nil {
			return nil, err
		}

		switch fragmentType {
		case "$145", "$157", "$164", "$258", "$259", "$260", "$262", "$266", "$391", "$490", "$609":
			value, err := decodeIonMap(payload, docSymbols, resolver)
			if err != nil {
				return nil, err
			}

			switch fragmentType {
			case "$145":
				name, _ := asString(value["name"])
				stringsValue := toStringSlice(value["$146"])
				if name != "" && len(stringsValue) > 0 {
					contentFragments[name] = stringsValue
				}
			case "$164":
				resource := parseResourceFragment(fragmentID, value)
				if resource.Location != "" {
					resourceFragments[resource.ID] = resource
				}
			case "$258":
				order := readSectionOrder(value)
				if len(order) > 0 {
					sectionOrder = order
				}
			case "$157":
				id := chooseFragmentIdentity(fragmentID, value["$173"])
				if id != "" {
					styleFragments[id] = value
				}
			case "$259":
				id := chooseFragmentIdentity(fragmentID, value["$176"])
				if id != "" {
					storylines[id] = value
				}
			case "$260":
				section := parseSectionFragment(fragmentID, value)
				if section.ID != "" && section.Storyline != "" {
					sectionFragments[section.ID] = section
				}
			case "$262":
				font := parseFontFragment(value)
				if font.Location != "" {
					fontFragments[font.Location] = font
				}
			case "$266":
				anchor := parseAnchorFragment(fragmentID, value)
				if anchor.ID != "" && (anchor.PositionID != 0 || anchor.URI != "") {
					anchors[anchor.ID] = anchor
				}
			case "$391":
				id := chooseFragmentIdentity(fragmentID, value["$239"])
				if id != "" {
					navContainers[id] = value
				}
			case "$490":
				applyMetadata(book, value)
			case "$609":
				sectionID := parsePositionMapSectionID(fragmentID, value)
				for _, positionID := range readPositionMap(value) {
					if positionID != 0 && sectionID != "" {
						positionAliases[positionID] = sectionID
					}
				}
			}
		case "$389":
			value, err := decodeIonValue(payload, docSymbols, resolver)
			if err != nil {
				return nil, err
			}
			if rootList, ok := asSlice(value); ok {
				for _, entry := range rootList {
					entryMap, ok := asMap(entry)
					if ok {
						navRoots = append(navRoots, entryMap)
					}
				}
			}
		case "$417", "$418":
			if fragmentID != "" {
				dataCopy := append([]byte(nil), payload...)
				rawFragments[fragmentID] = dataCopy
				rawBlobOrder = append(rawBlobOrder, rawBlob{
					ID:   fragmentID,
					Data: dataCopy,
				})
			}
		}
	}

	fontFixer := newFontNameFixer()
	currentFontFixer = fontFixer
	defer func() {
		currentFontFixer = nil
	}()
	book.Resources, book.CoverImageHref, book.Stylesheet, book.ResourceHrefByID = buildResources(book, resourceFragments, fontFragments, rawFragments, rawBlobOrder)

	if len(sectionOrder) == 0 {
		for sectionID := range sectionFragments {
			sectionOrder = append(sectionOrder, sectionID)
		}
		sort.Strings(sectionOrder)
	}

	positionToSectionID := map[int]string{}
	for positionID, sectionID := range positionAliases {
		positionToSectionID[positionID] = sectionID
	}
	for _, section := range sectionFragments {
		positionToSectionID[section.PositionID] = section.ID
	}
	for _, sectionID := range sectionOrder {
		section := sectionFragments[sectionID]
		storyline := storylines[section.Storyline]
		if storyline == nil {
			continue
		}
		nodes, _ := asSlice(storyline["$146"])
		collectStorylinePositions(nodes, sectionID, positionToSectionID)
	}

	navState := processNavigation(navRoots, navContainers)
	selectedNav := navState.toc
	navTitles := map[string]string{}
	flattenNavigationTitles(selectedNav, positionToSectionID, navTitles)
	anchorToFilename := map[string]string{}
	for anchorID, anchor := range anchors {
		if anchor.URI != "" {
			anchorToFilename[anchorID] = anchor.URI
		} else if sectionID, ok := positionToSectionID[anchor.PositionID]; ok {
			anchorToFilename[anchorID] = sectionFilename(sectionID)
		}
		if debugAnchors := os.Getenv("KFX_DEBUG_ANCHORS"); debugAnchors != "" {
			for _, wanted := range strings.Split(debugAnchors, ",") {
				if strings.TrimSpace(wanted) == anchorID {
					fmt.Fprintf(os.Stderr, "anchor map[%s]=%q uri=%q pos=%d\n", anchorID, anchorToFilename[anchorID], anchor.URI, anchor.PositionID)
				}
			}
		}
	}
	renderer := storylineRenderer{
		contentFragments:  contentFragments,
		resourceHrefByID:  book.ResourceHrefByID,
		anchorToFilename:  anchorToFilename,
		positionToSection: positionToSectionID,
		positionAnchors:   navState.positionAnchors,
		positionAnchorID:  buildPositionAnchorIDs(navState.positionAnchors),
		emittedAnchorIDs:  map[string]bool{},
		styleFragments:    styleFragments,
		styles:            newStyleCatalog(),
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
		storyline := storylines[section.Storyline]
		if storyline == nil {
			continue
		}
		nodes, _ := asSlice(storyline["$146"])
		paragraphs := flattenParagraphs(nodes, contentFragments)
		debugStorylineNodes(sectionID, nodes, 0)
		if os.Getenv("KFX_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "render section=%s pageStyle=%s storyStyle=%s\n", sectionID, section.PageTemplateStyle, asStringDefault(storyline["$157"]))
		}
		rendered := renderer.renderStoryline(section.PositionID, section.PageTemplateStyle, section.PageTemplateValues, storyline, nodes)
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
		book.Sections = append(book.Sections, epub.Section{
			Filename:   sectionFilename(sectionID),
			Title:      title,
			PageTitle:  sectionID,
			Language:   normalizeLanguage(book.Language),
			BodyClass:  rendered.BodyClass,
			Paragraphs: paragraphs,
			BodyHTML:   rendered.BodyHTML,
			Properties: rendered.Properties,
		})
	}
	for _, section := range book.Sections {
		renderer.styles.markReferenced(section.BodyClass)
		renderer.styles.markReferenced(section.BodyHTML)
	}
	replacer := renderer.styles.replacer()
	for index := range book.Sections {
		book.Sections[index].BodyClass = replacer.Replace(book.Sections[index].BodyClass)
		book.Sections[index].BodyHTML = replacer.Replace(book.Sections[index].BodyHTML)
	}
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
		sectionID := positionToSectionID[target.PositionID]
		if sectionID == "" {
			return ""
		}
		filename := sectionFilename(sectionID)
		if anchorID := renderer.anchorIDForPosition(target.PositionID, target.Offset); anchorID != "" && renderer.emittedAnchorIDs[anchorID] {
			return filename + "#" + anchorID
		}
		return filename
	}
	navHref := func(target navTarget) string {
		if target.PositionID == 0 {
			return ""
		}
		sectionID := positionToSectionID[target.PositionID]
		if sectionID == "" {
			return ""
		}
		filename := sectionFilename(sectionID)
		if target.Offset == 0 {
			return filename
		}
		if anchorID := renderer.anchorIDForPosition(target.PositionID, target.Offset); anchorID != "" && renderer.emittedAnchorIDs[anchorID] {
			return filename + "#" + anchorID
		}
		return filename
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
	applyCoverSVGPromotion(book)
	book.Stylesheet = pruneUnusedStylesheetRules(book.Stylesheet, collectReferencedClasses(book))
	book.Stylesheet = finalizeStylesheet(book.Stylesheet)
	book.Identifier = normalizeBookIdentifier(book.Identifier)
	book.Language = normalizeLanguage(book.Language)

	return book, nil
}

func newSymbolResolver(docSymbols []byte) (*symbolResolver, error) {
	var buf bytes.Buffer
	writer := ion.NewBinaryWriter(&buf)
	if err := writer.WriteInt(0); err != nil {
		return nil, err
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}

	stream := make([]byte, 0, len(docSymbols)+buf.Len())
	stream = append(stream, docSymbols...)
	stream = append(stream, stripIVM(buf.Bytes())...)

	reader := ion.NewReaderCat(bytes.NewReader(stream), sharedCatalog())
	for reader.Next() {
		break
	}
	if err := reader.Err(); err != nil {
		return nil, err
	}
	table := reader.SymbolTable()
	if table == nil {
		return nil, fmt.Errorf("KFX document symbol table is empty")
	}

	maxImportID := uint32(ionSystemSymbolCount)
	for _, imported := range table.Imports() {
		if imported == nil || imported.Name() == "$ion" {
			continue
		}
		maxID := imported.MaxID()
		if maxID > ionSystemSymbolCount {
			maxID -= ionSystemSymbolCount
		}
		maxImportID += uint32(maxID)
	}

	return &symbolResolver{
		localStart: maxImportID + 1,
		locals:     table.Symbols(),
	}, nil
}

func (r *symbolResolver) Resolve(sid uint32) string {
	if sid == 0 {
		return ""
	}
	if r.isLocalSID(sid) {
		return r.locals[sid-r.localStart]
	}
	return fmt.Sprintf("$%d", sid)
}

func (r *symbolResolver) isLocalSID(sid uint32) bool {
	if r == nil {
		return false
	}
	offset := sid - r.localStart
	return sid >= r.localStart && int(offset) < len(r.locals)
}

func entityPayload(data []byte) ([]byte, error) {
	if len(data) < 10 || string(data[:4]) != "ENTY" {
		return nil, &UnsupportedError{Message: "entity wrapper is invalid"}
	}
	headerLen := int(binary.LittleEndian.Uint32(data[6:10]))
	if headerLen < 10 || headerLen > len(data) {
		return nil, &UnsupportedError{Message: "entity header length is invalid"}
	}
	return data[headerLen:], nil
}

func sharedCatalog() ion.Catalog {
	return ion.NewCatalog(sharedTable())
}

func sharedTable() ion.SharedSymbolTable {
	symbols := make([]string, 991)
	for sid := 10; sid <= 1000; sid++ {
		symbols[sid-10] = fmt.Sprintf("$%d", sid)
	}
	return ion.NewSharedSymbolTable("YJ_symbols", 10, symbols)
}

func yjPrelude() ([]byte, error) {
	yjPreludeOnce.Do(func() {
		var buf bytes.Buffer
		writer := ion.NewBinaryWriter(&buf)
		lst := ion.NewLocalSymbolTable([]ion.SharedSymbolTable{sharedTable()}, nil)
		yjPreludeErr = lst.WriteTo(writer)
		if yjPreludeErr == nil {
			yjPreludeErr = writer.Finish()
		}
		if yjPreludeErr == nil {
			yjPreludeData = buf.Bytes()
		}
	})
	return yjPreludeData, yjPreludeErr
}

func decodeIonMap(data []byte, docSymbols []byte, resolver *symbolResolver) (map[string]interface{}, error) {
	value, err := decodeIonValue(data, docSymbols, resolver)
	if err != nil {
		return nil, err
	}

	mapped, ok := value.(map[string]interface{})
	if !ok {
		return nil, &UnsupportedError{Message: "decoded Ion fragment is not a struct"}
	}
	return mapped, nil
}

func decodeIonValue(data []byte, docSymbols []byte, resolver *symbolResolver) (interface{}, error) {
	stream := data
	prefix := docSymbols
	if len(prefix) == 0 {
		var err error
		prefix, err = yjPrelude()
		if err != nil {
			return nil, &UnsupportedError{Message: err.Error()}
		}
	}
	if len(prefix) > 0 {
		stream = make([]byte, 0, len(prefix)+len(data))
		stream = append(stream, prefix...)
		stream = append(stream, stripIVM(data)...)
	}

	decoder := ion.NewDecoder(ion.NewReaderCat(bytes.NewReader(stream), sharedCatalog()))
	value, err := decoder.Decode()
	if err != nil {
		return nil, &UnsupportedError{Message: err.Error()}
	}

	return normalizeIon(value, resolver), nil
}

func stripIVM(data []byte) []byte {
	if bytes.HasPrefix(data, ionVersionMarker) {
		return data[len(ionVersionMarker):]
	}
	return data
}

func normalizeIon(value interface{}, resolver *symbolResolver) interface{} {
	switch typed := value.(type) {
	case *string:
		if typed == nil {
			return ""
		}
		return *typed
	case *ion.SymbolToken:
		if typed == nil {
			return ""
		}
		if resolver != nil && resolver.isLocalSID(uint32(typed.LocalSID)) {
			return resolver.Resolve(uint32(typed.LocalSID))
		}
		if typed.Text != nil {
			return *typed.Text
		}
		if resolver != nil {
			return resolver.Resolve(uint32(typed.LocalSID))
		}
		return fmt.Sprintf("$%d", typed.LocalSID)
	case ion.SymbolToken:
		if resolver != nil && resolver.isLocalSID(uint32(typed.LocalSID)) {
			return resolver.Resolve(uint32(typed.LocalSID))
		}
		if typed.Text != nil {
			return *typed.Text
		}
		if resolver != nil {
			return resolver.Resolve(uint32(typed.LocalSID))
		}
		return fmt.Sprintf("$%d", typed.LocalSID)
	case map[string]interface{}:
		result := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			result[key] = normalizeIon(item, resolver)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(typed))
		for index, item := range typed {
			result[index] = normalizeIon(item, resolver)
		}
		return result
	default:
		return typed
	}
}

func readSectionOrder(value map[string]interface{}) []string {
	entries, ok := asSlice(value["$169"])
	if !ok {
		return nil
	}
	for _, entry := range entries {
		entryMap, ok := asMap(entry)
		if !ok {
			continue
		}
		sections, ok := asSlice(entryMap["$170"])
		if !ok {
			continue
		}
		result := make([]string, 0, len(sections))
		for _, item := range sections {
			if text, ok := asString(item); ok {
				result = append(result, text)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return nil
}

func parsePositionMapSectionID(fragmentID string, value map[string]interface{}) string {
	return chooseFragmentIdentity(fragmentID, value["$174"])
}

func readPositionMap(value map[string]interface{}) []int {
	entries, ok := asSlice(value["$181"])
	if !ok {
		return nil
	}
	positions := make([]int, 0, len(entries))
	for _, entry := range entries {
		pair, ok := asSlice(entry)
		if !ok || len(pair) != 2 {
			continue
		}
		positionID, ok := asInt(pair[1])
		if !ok || positionID == 0 {
			continue
		}
		positions = append(positions, positionID)
	}
	return positions
}

func sectionStorylineID(section map[string]interface{}) string {
	containers, ok := asSlice(section["$141"])
	if !ok || len(containers) == 0 {
		return ""
	}
	first, ok := asMap(containers[0])
	if !ok {
		return ""
	}
	storylineID, _ := asString(first["$176"])
	return storylineID
}

func parseSectionFragment(fragmentID string, value map[string]interface{}) sectionFragment {
	id := chooseFragmentIdentity(fragmentID, value["$174"])
	containers, ok := asSlice(value["$141"])
	if !ok || len(containers) == 0 {
		return sectionFragment{ID: id}
	}
	first, ok := asMap(containers[0])
	if !ok {
		return sectionFragment{ID: id}
	}
	storylineID, _ := asString(first["$176"])
	pageTemplateStyle, _ := asString(first["$157"])
	positionID, _ := asInt(first["$155"])
	return sectionFragment{
		ID:                 id,
		PositionID:         positionID,
		Storyline:          storylineID,
		PageTemplateStyle:  pageTemplateStyle,
		PageTemplateValues: filterBodyStyleValues(first),
	}
}

func collectStorylinePositions(nodes []interface{}, sectionID string, positions map[int]string) {
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		if positionID, ok := asInt(node["$155"]); ok && positionID != 0 && positions[positionID] == "" {
			positions[positionID] = sectionID
		}
		if children, ok := asSlice(node["$146"]); ok {
			collectStorylinePositions(children, sectionID, positions)
		}
		if cols, ok := asSlice(node["$152"]); ok {
			collectStorylinePositions(cols, sectionID, positions)
		}
	}
}

func parseAnchorFragment(fragmentID string, value map[string]interface{}) anchorFragment {
	id := chooseFragmentIdentity(fragmentID, value["$180"])
	if debugAnchors := os.Getenv("KFX_DEBUG_ANCHORS"); debugAnchors != "" {
		for _, wanted := range strings.Split(debugAnchors, ",") {
			if strings.TrimSpace(wanted) == id || strings.TrimSpace(wanted) == fragmentID {
				fmt.Fprintf(os.Stderr, "anchor fragment id=%s fragment=%s value=%#v\n", id, fragmentID, value)
			}
		}
	}
	if uri, ok := asString(value["$186"]); ok {
		if uri == "http://" || uri == "https://" {
			uri = ""
		}
		return anchorFragment{ID: id, URI: uri}
	}
	target, ok := asMap(value["$183"])
	if !ok {
		return anchorFragment{ID: id}
	}
	positionID, _ := asInt(target["$155"])
	return anchorFragment{
		ID:         id,
		PositionID: positionID,
	}
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func effectiveStyle(base map[string]interface{}, values map[string]interface{}) map[string]interface{} {
	style := cloneMap(base)
	if style == nil {
		style = map[string]interface{}{}
	}
	for key, value := range values {
		style[key] = value
	}
	return style
}

func mergeStyleValues(dst map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = map[string]interface{}{}
	}
	for key, value := range src {
		if _, exists := dst[key]; !exists {
			dst[key] = value
		}
	}
	return dst
}

func filterBodyStyleValues(values map[string]interface{}) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	allowed := map[string]bool{
		"$11":  true,
		"$12":  true,
		"$16":  true,
		"$36":  true,
		"$41":  true,
		"$42":  true,
		"$47":  true,
		"$48":  true,
		"$49":  true,
		"$50":  true,
		"$70":  true,
		"$72":  true,
		"$580": true,
		"$583": true,
	}
	filtered := map[string]interface{}{}
	for key, value := range values {
		if allowed[key] {
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func declarationSet(declarations []string) map[string]bool {
	if len(declarations) == 0 {
		return nil
	}
	result := make(map[string]bool, len(declarations))
	for _, declaration := range declarations {
		result[declaration] = true
	}
	return result
}

func inheritedDefaultSet(declarations []string) map[string]bool {
	result := declarationSet(declarations)
	if result == nil {
		result = map[string]bool{}
	}
	hasTextIndent := false
	for declaration := range result {
		if strings.HasPrefix(declaration, "text-indent: ") {
			hasTextIndent = true
			break
		}
	}
	if !hasTextIndent {
		result["text-indent: 0"] = true
	}
	return result
}

func defaultBodyDeclarations(bodyClass string) []string {
	switch bodyClass {
	case "class-0":
		return []string{"font-family: FreeFontSerif,serif", "text-align: center"}
	case "class-1":
		return []string{"font-family: FreeFontSerif,serif"}
	case "class-2":
		return []string{"font-family: FreeFontSerif,serif", "text-align: justify", "text-indent: 1.44em"}
	case "class-3":
		return []string{"font-family: FreeFontSerif,serif", "text-align: justify"}
	case "class-7":
		return []string{"font-family: FreeFontSerif,serif", "font-style: italic", "text-align: justify", "text-indent: 1.44em"}
	case "class-8":
		return []string{"font-family: Shift Light,Palatino,Palatino Linotype,Palatino LT Std,Book Antiqua,Georgia,serif"}
	default:
		return nil
	}
}

func isStaticBodyClass(bodyClass string) bool {
	switch bodyClass {
	case "class-0", "class-1", "class-2", "class-3", "class-7", "class-8":
		return true
	default:
		return false
	}
}

func staticBodyClassForDeclarations(declarations []string) string {
	alternates := map[string][][]string{
		"class-0": {
			{"text-align: center"},
		},
		"class-3": {
			{"text-align: justify"},
		},
	}
	for _, bodyClass := range []string{"class-0", "class-1", "class-2", "class-3", "class-7", "class-8"} {
		expected := defaultBodyDeclarations(bodyClass)
		if len(expected) != len(declarations) {
			for _, alternate := range alternates[bodyClass] {
				if len(alternate) != len(declarations) {
					continue
				}
				match := true
				for index := range alternate {
					if alternate[index] != declarations[index] {
						match = false
						break
					}
				}
				if match {
					return bodyClass
				}
			}
			continue
		}
		match := true
		for index := range expected {
			if expected[index] != declarations[index] {
				match = false
				break
			}
		}
		if match {
			return bodyClass
		}
	}
	return ""
}

func flattenParagraphs(nodes []interface{}, contents map[string][]string) []string {
	result := make([]string, 0, 64)
	var walk func(items []interface{})
	walk = func(items []interface{}) {
		for _, item := range items {
			node, ok := asMap(item)
			if !ok {
				continue
			}
			if ref, ok := asMap(node["$145"]); ok {
				name, _ := asString(ref["name"])
				index, ok := asInt(ref["$403"])
				if ok {
					if values, found := contents[name]; found && index >= 0 && index < len(values) {
						text := strings.TrimSpace(values[index])
						if text != "" {
							result = append(result, text)
						}
					}
				}
			}
			if children, ok := asSlice(node["$146"]); ok {
				walk(children)
			}
		}
	}
	walk(nodes)
	return result
}

func deriveSectionTitle(paragraphs []string, sectionNumber int) string {
	for _, paragraph := range paragraphs {
		trimmed := strings.TrimSpace(paragraph)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > 80 {
			break
		}
		return trimmed
	}
	return fmt.Sprintf("Section %d", sectionNumber)
}

func parseResourceFragment(fragmentID string, value map[string]interface{}) resourceFragment {
	resourceID, _ := asString(value["$175"])
	if resourceID == "" {
		resourceID = fragmentID
	}
	location, _ := asString(value["$165"])
	mediaType, _ := asString(value["$162"])

	return resourceFragment{
		ID:        resourceID,
		Location:  location,
		MediaType: mediaType,
	}
}

func parseFontFragment(value map[string]interface{}) fontFragment {
	location, _ := asString(value["$165"])
	family, _ := asString(value["$11"])

	return fontFragment{
		Location: location,
		Family:   family,
		Style:    mapFontStyle(value["$12"]),
		Weight:   mapFontWeight(value["$13"]),
		Stretch:  mapFontStretch(value["$15"]),
	}
}

func buildResources(book *decodedBook, resources map[string]resourceFragment, fonts map[string]fontFragment, raw map[string][]byte, rawOrder []rawBlob) ([]epub.Resource, string, string, map[string]string) {
	var output []epub.Resource
	imagePool, fontPool := partitionRawBlobs(rawOrder)
	imageCursor := 0
	fontCursor := 0

	resourceIDs := make([]string, 0, len(resources))
	for resourceID := range resources {
		resourceIDs = append(resourceIDs, resourceID)
	}
	sort.Strings(resourceIDs)

	resourceFilenameByID := map[string]string{}
	firstImageFilename := ""
	for _, resourceID := range resourceIDs {
		resource := resources[resourceID]
		data := raw[resource.Location]
		if !blobMatchesImageMediaType(data, resource.MediaType) {
			data = nil
		}
		if len(data) == 0 {
			data, imageCursor = nextMatchingBlob(imagePool, imageCursor, resource.MediaType)
		}
		if len(data) == 0 {
			continue
		}
		mediaType := resource.MediaType
		if strings.EqualFold(mediaType, "image/jpg") {
			mediaType = "image/jpeg"
		}
		if strings.EqualFold(mediaType, "image/jxr") {
			convertedData, convertedType, err := convertJXRResource(data)
			if err == nil {
				data = convertedData
				mediaType = convertedType
			}
		}
		filename := resourceFilename(resourceFragment{
			ID:        resource.ID,
			Location:  resource.Location,
			MediaType: mediaType,
		})
		output = append(output, epub.Resource{
			Filename:  filename,
			MediaType: mediaType,
			Data:      data,
		})
		resourceFilenameByID[resourceID] = filename
		if firstImageFilename == "" {
			firstImageFilename = filename
		}
	}

	fontLocations := make([]string, 0, len(fonts))
	for location := range fonts {
		fontLocations = append(fontLocations, location)
	}
	sort.Strings(fontLocations)

	var stylesheet strings.Builder
	fontFaceLines := make([]string, 0, len(fontLocations))
	for _, location := range fontLocations {
		font := fonts[location]
		data := raw[location]
		if detectFontExtension(data) == ".bin" {
			data = nil
		}
		if len(data) == 0 {
			data, fontCursor = nextFontBlob(fontPool, fontCursor)
		}
		if len(data) == 0 {
			continue
		}
		filename := fontFilename(location, data)
		output = append(output, epub.Resource{
			Filename:  filename,
			MediaType: fontMediaType(filename),
			Data:      data,
		})

		family := font.Family
		if currentFontFixer != nil {
			family = currentFontFixer.fixFontName(family, true, false)
		}
		declarations := []string{"font-family: " + quoteFontName(family)}
		if font.Style != "" && font.Style != "normal" {
			declarations = append(declarations, "font-style: "+font.Style)
		}
		if font.Weight != "" && font.Weight != "normal" {
			declarations = append(declarations, "font-weight: "+font.Weight)
		}
		if font.Stretch != "" && font.Stretch != "normal" {
			declarations = append(declarations, "font-stretch: "+font.Stretch)
		}
		declarations = append(declarations, "src: url("+filename+")")
		fontFaceLines = append(fontFaceLines, "@font-face {"+strings.Join(canonicalDeclarations(declarations), "; ")+"}")
	}
	sort.Strings(fontFaceLines)
	for index, line := range fontFaceLines {
		if index > 0 {
			stylesheet.WriteByte('\n')
		}
		stylesheet.WriteString(line)
	}

	coverImageHref := resourceFilenameByID[book.CoverImageID]
	if coverImageHref == "" && book.CoverImageID != "" {
		coverImageHref = firstImageFilename
	}
	for index := range output {
		if output[index].Filename == coverImageHref && coverImageHref != "" {
			output[index].Properties = "cover-image"
			break
		}
	}
	return output, coverImageHref, strings.TrimSpace(stylesheet.String()), resourceFilenameByID
}

func applyMetadata(book *decodedBook, value map[string]interface{}) {
	categories, ok := asSlice(value["$491"])
	if !ok {
		return
	}
	for _, category := range categories {
		categoryMap, ok := asMap(category)
		if !ok {
			continue
		}
		name, _ := asString(categoryMap["$495"])
		if name != "kindle_title_metadata" {
			continue
		}
		entries, ok := asSlice(categoryMap["$258"])
		if !ok {
			continue
		}
		for _, rawEntry := range entries {
			entry, ok := asMap(rawEntry)
			if !ok {
				continue
			}
			key, _ := asString(entry["$492"])
			switch key {
			case "title":
				if value, ok := asString(entry["$307"]); ok && value != "" {
					book.Title = value
				}
			case "author":
				if value, ok := asString(entry["$307"]); ok && value != "" {
					book.Authors = append(book.Authors, value)
				}
			case "language":
				if value, ok := asString(entry["$307"]); ok && value != "" {
					book.Language = value
				}
			case "issue_date":
				if value, ok := asString(entry["$307"]); ok && value != "" {
					book.Published = value
				}
			case "cover_image":
				if value, ok := asString(entry["$307"]); ok && value != "" {
					book.CoverImageID = value
				}
			case "override_kindle_font":
				if value, ok := asBool(entry["$307"]); ok {
					book.OverrideKindleFonts = value
				}
			case "content_id", "ASIN":
				if value, ok := asString(entry["$307"]); ok && value != "" {
					book.Identifier = value
				}
			}
		}
	}
}

func asMap(value interface{}) (map[string]interface{}, bool) {
	result, ok := value.(map[string]interface{})
	return result, ok
}

func asSlice(value interface{}) ([]interface{}, bool) {
	result, ok := value.([]interface{})
	return result, ok
}

func asString(value interface{}) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case ion.SymbolToken:
		if typed.Text != nil {
			return *typed.Text, true
		}
		return fmt.Sprintf("$%d", typed.LocalSID), true
	case *ion.SymbolToken:
		if typed == nil {
			return "", false
		}
		if typed.Text != nil {
			return *typed.Text, true
		}
		return fmt.Sprintf("$%d", typed.LocalSID), true
	default:
		return "", false
	}
}

func asInt(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

func asBool(value interface{}) (bool, bool) {
	typed, ok := value.(bool)
	return typed, ok
}

func toStringSlice(value interface{}) []string {
	items, ok := asSlice(value)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := asString(item); ok {
			result = append(result, text)
		}
	}
	return result
}

func normalizeFontFamily(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "serif"
	}
	families := splitAndNormalizeFontFamilies(value)
	if len(families) == 0 {
		return "serif"
	}
	return families[0]
}

func mapFontStyle(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$382":
		return "italic"
	case "$381":
		return "oblique"
	case "$350":
		return "normal"
	default:
		return ""
	}
}

func mapFontWeight(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$361":
		return "bold"
	case "$363":
		return "900"
	case "$357":
		return "300"
	case "$359":
		return "500"
	case "$350":
		return "normal"
	case "$360":
		return "600"
	case "$355":
		return "100"
	case "$362":
		return "800"
	case "$356":
		return "200"
	default:
		return ""
	}
}

func mapFontStretch(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$365":
		return "condensed"
	case "$368":
		return "expanded"
	case "$350":
		return "normal"
	case "$366":
		return "semi-condensed"
	case "$367":
		return "semi-expanded"
	default:
		return ""
	}
}

func mapHyphens(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$383":
		return "auto"
	case "$384":
		return "manual"
	case "$349":
		return "none"
	default:
		return ""
	}
}

func mapPageBreak(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$352":
		return "always"
	case "$383":
		return "auto"
	case "$353":
		return "avoid"
	default:
		return ""
	}
}

func mapBorderStyle(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$349":
		return "none"
	case "$328":
		return "solid"
	case "$331":
		return "dotted"
	case "$330":
		return "dashed"
	case "$329":
		return "double"
	case "$335":
		return "ridge"
	case "$334":
		return "groove"
	case "$336":
		return "inset"
	case "$337":
		return "outset"
	default:
		return ""
	}
}

func mapBoxAlign(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$320":
		return "center"
	case "$59":
		return "left"
	case "$61":
		return "right"
	case "$321":
		return "justify"
	default:
		return ""
	}
}

func mapTableVerticalAlign(value interface{}) string {
	switch asStringDefault(value) {
	case "$350":
		return "baseline"
	case "$60":
		return "bottom"
	case "$320":
		return "middle"
	case "$58":
		return "top"
	default:
		return ""
	}
}

func mapTextDecoration(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$328":
		return "underline"
	default:
		return ""
	}
}

func mapFontVariant(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$369":
		return "small-caps"
	case "$349":
		return "normal"
	default:
		return ""
	}
}

func mapTextTransform(value interface{}) string {
	switch text, _ := asString(value); text {
	case "$374":
		return "capitalize"
	case "$373":
		return "lowercase"
	case "$372":
		return "uppercase"
	case "$349":
		return "none"
	default:
		return ""
	}
}

func containerStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssFontFamily(style["$11"]); value != "" {
		declarations = append(declarations, "font-family: "+value)
	}
	if value := cssLengthProperty(style["$16"], "$16"); value != "" && value != "1em" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := mapFontStyle(style["$12"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-style: "+value)
	}
	if value := mapFontWeight(style["$13"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-weight: "+value)
	}
	if value := mapFontVariant(style["$583"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-variant: "+value)
	}
	if value := cssLineHeight(style["$42"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["$49"], "$49"); value != "" {
		declarations = append(declarations, "margin-bottom: "+value)
	}
	if value := cssLengthProperty(style["$48"], "$48"); value != "" {
		declarations = append(declarations, "margin-left: "+value)
	}
	if value := cssLengthProperty(style["$50"], "$50"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := cssLengthProperty(style["$47"], "$47"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	}
	if value := cssLengthProperty(style["$52"], "$52"); value != "" {
		declarations = append(declarations, "padding-top: "+value)
	}
	if value := mapBoxAlign(style["$34"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	if value := cssLengthProperty(style["$36"], "$36"); value != "" {
		declarations = append(declarations, "text-indent: "+value)
	}
	if value := mapPageBreak(style["$135"]); value != "" {
		declarations = append(declarations, "page-break-inside: "+value)
	}
	if color := cssColor(style["$84"]); color != "" {
		declarations = append(declarations, "border-top-color: "+color)
	}
	if value := mapBorderStyle(style["$89"]); value != "" {
		declarations = append(declarations, "border-top-style: "+value)
	}
	if value := cssLengthProperty(style["$94"], "$94"); value != "" {
		declarations = append(declarations, "border-top-width: "+value)
	}
	if value := fillColor(style); value != "" {
		declarations = append(declarations, "background-color: "+value)
	}
	if value := mapTextTransform(style["$41"]); value != "" && value != "none" {
		declarations = append(declarations, "text-transform: "+value)
	}
	return declarations
}

func bodyStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssFontFamily(style["$11"]); value != "" {
		declarations = append(declarations, "font-family: "+value)
	}
	if value := mapFontStyle(style["$12"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-style: "+value)
	}
	if value := mapFontVariant(style["$583"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-variant: "+value)
	}
	if value := cssLengthProperty(style["$16"], "$16"); value != "" && value != "1em" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := cssLineHeight(style["$42"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["$49"], "$49"); value != "" {
		declarations = append(declarations, "margin-bottom: "+value)
	}
	if value := cssLengthProperty(style["$48"], "$48"); value != "" {
		declarations = append(declarations, "margin-left: "+value)
	}
	if value := cssLengthProperty(style["$50"], "$50"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := cssLengthProperty(style["$47"], "$47"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	}
	if value := mapBoxAlign(style["$580"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	} else if value := mapBoxAlign(style["$34"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	if value := cssLengthProperty(style["$36"], "$36"); value != "" {
		if value == "0" {
			goto skipBodyIndent
		}
		declarations = append(declarations, "text-indent: "+value)
	}
skipBodyIndent:
	if value := fillColor(style); value != "" {
		declarations = append(declarations, "background-color: "+value)
	}
	if value := mapTextTransform(style["$41"]); value != "" && value != "none" {
		declarations = append(declarations, "text-transform: "+value)
	}
	return declarations
}

func paragraphStyleDeclarations(style map[string]interface{}, linkStyle map[string]interface{}) []string {
	var declarations []string
	if value := colorDeclarations(style, linkStyle); value != "" {
		declarations = append(declarations, "color: "+value)
	}
	if value := cssFontFamily(style["$11"]); value != "" {
		declarations = append(declarations, "font-family: "+value)
	}
	if value := mapFontStyle(style["$12"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-style: "+value)
	}
	if value := mapFontWeight(style["$13"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-weight: "+value)
	}
	if value := mapFontVariant(style["$583"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-variant: "+value)
	}
	if value := cssLengthProperty(style["$16"], "$16"); value != "" && value != "1em" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := cssLineHeight(style["$42"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["$49"], "$49"); value != "" {
		declarations = append(declarations, "margin-bottom: "+value)
	} else {
		declarations = append(declarations, "margin-bottom: 0")
	}
	if value := cssLengthProperty(style["$48"], "$48"); value != "" {
		declarations = append(declarations, "margin-left: "+value)
	}
	if value := cssLengthProperty(style["$50"], "$50"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := cssLengthProperty(style["$47"], "$47"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	} else {
		declarations = append(declarations, "margin-top: 0")
	}
	if value := mapBoxAlign(style["$34"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	if value := cssLengthProperty(style["$36"], "$36"); value != "" {
		declarations = append(declarations, "text-indent: "+value)
	}
	if value := fillColor(style); value != "" {
		declarations = append(declarations, "background-color: "+value)
	}
	if value := mapTextTransform(style["$41"]); value != "" && value != "none" {
		declarations = append(declarations, "text-transform: "+value)
	}
	if value := mapTextDecoration(style["$23"]); value != "" {
		declarations = append(declarations, "text-decoration: "+value)
	}
	if value := mapPageBreak(style["$135"]); value != "" {
		declarations = append(declarations, "page-break-inside: "+value)
	}
	if linkStyle != nil {
		if _, ok := style["$11"]; !ok {
			if value := cssFontFamily(linkStyle["$11"]); value != "" {
				declarations = append(declarations, "font-family: "+value)
			}
		}
		if _, ok := style["$12"]; !ok {
			if value := mapFontStyle(linkStyle["$12"]); value != "" && value != "normal" {
				declarations = append(declarations, "font-style: "+value)
			}
		}
		if _, ok := style["$13"]; !ok {
			if value := mapFontWeight(linkStyle["$13"]); value != "" && value != "normal" {
				declarations = append(declarations, "font-weight: "+value)
			}
		}
		if _, ok := style["$583"]; !ok {
			if value := mapFontVariant(linkStyle["$583"]); value != "" && value != "normal" {
				declarations = append(declarations, "font-variant: "+value)
			}
		}
		if _, ok := style["$41"]; !ok {
			if value := mapTextTransform(linkStyle["$41"]); value != "" && value != "none" {
				declarations = append(declarations, "text-transform: "+value)
			}
		}
	}
	return declarations
}

func linkStyleDeclarations(style map[string]interface{}, suppressColor bool) []string {
	var declarations []string
	if !suppressColor {
		if value := colorDeclarations(style, nil); value != "" {
			declarations = append(declarations, "color: "+value)
		}
	}
	if value := cssLengthProperty(style["$16"], "$16"); value != "" && value != "1em" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := mapTextDecoration(style["$23"]); value != "" {
		declarations = append(declarations, "text-decoration: "+value)
	}
	return declarations
}

func spanStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssLengthProperty(style["$16"], "$16"); value != "" && value != "1em" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := cssFontFamily(style["$11"]); value != "" {
		declarations = append(declarations, "font-family: "+value)
	}
	if value := mapFontStyle(style["$12"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-style: "+value)
	}
	if value := mapFontWeight(style["$13"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-weight: "+value)
	}
	if value := mapFontVariant(style["$583"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-variant: "+value)
	}
	if value := cssLineHeight(style["$42"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["$50"], "$50"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := mapTextTransform(style["$41"]); value != "" && value != "none" {
		declarations = append(declarations, "text-transform: "+value)
	}
	return declarations
}

func imageWrapperStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssLengthProperty(style["$47"], "$47"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	}
	if value := mapBoxAlign(style["$580"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	return declarations
}

func styleClassName(prefix string, styleID string) string {
	if strings.HasSuffix(prefix, "_") && strings.HasPrefix(styleID, "-") {
		return strings.TrimSuffix(prefix, "_") + styleID
	}
	return prefix + styleID
}

func imageStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssLineHeight(style["$42"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["$56"], "$56"); value != "" {
		declarations = append(declarations, "width: "+value)
	}
	if value := cssLengthProperty(style["$57"], "$57"); value != "" {
		declarations = append(declarations, "height: "+value)
	}
	return declarations
}

func tableStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssLengthProperty(style["$47"], "$47"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	}
	if value := cssLengthProperty(style["$48"], "$48"); value != "" {
		declarations = append(declarations, "margin-left: "+value)
	}
	align := mapBoxAlign(style["$580"])
	if value := cssLengthProperty(style["$50"], "$50"); value != "" && align != "left" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := cssLengthProperty(style["$65"], "$65"); value != "" {
		declarations = append(declarations, "max-width: "+value)
	}
	if align == "left" {
		declarations = append(declarations, "margin-right: auto")
	}
	if color := cssColor(style["$83"]); color != "" {
		declarations = append(declarations, "border-color: "+color)
	}
	return declarations
}

func tableColumnStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssLengthProperty(style["$56"], "$56"); value != "" {
		declarations = append(declarations, "width: "+value)
	}
	return declarations
}

func structuredContainerDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := mapFontWeight(style["$13"]); value != "" {
		declarations = append(declarations, "font-weight: "+value)
	}
	return declarations
}

func tableCellStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := cssLengthProperty(style["$50"], "$50"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := mapBoxAlign(style["$34"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	if value := mapTableVerticalAlign(style["$633"]); value != "" {
		declarations = append(declarations, "vertical-align: "+value)
	}
	return declarations
}

func headingStyleDeclarations(style map[string]interface{}) []string {
	var declarations []string
	if value := mapHyphens(style["$127"]); value != "" {
		declarations = append(declarations, "-webkit-hyphens: "+value)
	}
	if value := cssFontFamily(style["$11"]); value != "" {
		declarations = append(declarations, "font-family: "+value)
	}
	if value := mapFontStyle(style["$12"]); value != "" && value != "normal" {
		declarations = append(declarations, "font-style: "+value)
	}
	if value := cssLengthProperty(style["$16"], "$16"); value != "" {
		declarations = append(declarations, "font-size: "+value)
	}
	if value := mapFontWeight(style["$13"]); value != "" {
		declarations = append(declarations, "font-weight: "+value)
	}
	if value := mapFontVariant(style["$583"]); value != "" {
		declarations = append(declarations, "font-variant: "+value)
	}
	if value := mapHyphens(style["$127"]); value != "" {
		declarations = append(declarations, "hyphens: "+value)
	}
	if value := cssLineHeight(style["$42"]); value != "" && value != "1.2" {
		declarations = append(declarations, "line-height: "+value)
	}
	if value := cssLengthProperty(style["$49"], "$49"); value != "" {
		declarations = append(declarations, "margin-bottom: "+value)
	} else {
		declarations = append(declarations, "margin-bottom: 0")
	}
	if value := cssLengthProperty(style["$47"], "$47"); value != "" {
		declarations = append(declarations, "margin-top: "+value)
	} else {
		declarations = append(declarations, "margin-top: 0")
	}
	if value := mapPageBreak(style["$788"]); value != "" {
		declarations = append(declarations, "page-break-after: "+value)
	}
	if value := mapBoxAlign(style["$34"]); value != "" {
		declarations = append(declarations, "text-align: "+value)
	}
	if value := cssLengthProperty(style["$36"], "$36"); value != "" {
		declarations = append(declarations, "text-indent: "+value)
	}
	if value := cssLengthProperty(style["$48"], "$48"); value != "" {
		declarations = append(declarations, "margin-left: "+value)
	}
	if value := cssLengthProperty(style["$50"], "$50"); value != "" {
		declarations = append(declarations, "margin-right: "+value)
	}
	if value := mapTextDecoration(style["$23"]); value != "" {
		declarations = append(declarations, "text-decoration: "+value)
	}
	if value := mapTextTransform(style["$41"]); value != "" {
		declarations = append(declarations, "text-transform: "+value)
	}
	return declarations
}

func cssFontFamily(value interface{}) string {
	text, ok := asString(value)
	if !ok || text == "" {
		return ""
	}
	if text == "default,serif" {
		return "FreeFontSerif,serif"
	}
	fixer := currentFontFixer
	if fixer == nil {
		fixer = newFontNameFixer()
	}
	return fixer.fixAndQuoteFontFamilyList(text)
}

func splitAndNormalizeFontFamilies(value string) []string {
	fixer := currentFontFixer
	if fixer == nil {
		fixer = newFontNameFixer()
	}
	return fixer.splitAndFixFontFamilyList(value)
}

func normalizeFontFamilyNameCase(name string) string {
	if name == "" {
		return ""
	}
	if strings.EqualFold(name, "serif") || strings.EqualFold(name, "sans-serif") || strings.EqualFold(name, "monospace") {
		return strings.ToLower(name)
	}
	return capitalizeFontName(name)
}

func quoteFontFamilies(families []string) []string {
	quoted := make([]string, 0, len(families))
	for _, family := range families {
		if family == "" {
			continue
		}
		quoted = append(quoted, quoteFontName(family))
	}
	return quoted
}

func cssLineHeight(value interface{}) string {
	magnitude, unit, ok := numericStyleValue(value)
	if !ok {
		return ""
	}
	switch unit {
	case "$310":
		return formatStyleNumber(magnitude * 1.2)
	case "$308", "$505":
		return formatStyleNumber(magnitude)
	default:
		return formatStyleNumber(magnitude)
	}
}

func cssLengthProperty(value interface{}, property string) string {
	magnitude, unit, ok := numericStyleValue(value)
	if !ok {
		return ""
	}
	if magnitude == 0 {
		return "0"
	}
	switch unit {
	case "$310":
		return formatStyleNumber(magnitude*1.2) + "em"
	case "$308", "$505":
		return formatStyleNumber(magnitude) + "em"
	case "$314":
		return formatStyleNumber(magnitude) + "%"
	case "$318":
		if magnitude > 0 && int(magnitude*1000)%225 == 0 {
			return formatStyleNumber(float64(int(magnitude*1000))/450.0) + "px"
		}
		return formatStyleNumber(magnitude) + "pt"
	case "$319":
		return formatStyleNumber(magnitude) + "px"
	default:
		if property == "$56" || property == "$57" || property == "$47" || property == "$49" || property == "$16" {
			return formatStyleNumber(magnitude)
		}
		return ""
	}
}

func numericStyleValue(value interface{}) (float64, string, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, "", true
	case *float64:
		if typed == nil {
			return 0, "", false
		}
		return *typed, "", true
	case float32:
		return float64(typed), "", true
	case *float32:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), "", true
	case int:
		return float64(typed), "", true
	case *int:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), "", true
	case int32:
		return float64(typed), "", true
	case *int32:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), "", true
	case int64:
		return float64(typed), "", true
	case *int64:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), "", true
	case uint32:
		return float64(typed), "", true
	case *uint32:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), "", true
	case uint64:
		return float64(typed), "", true
	case *uint64:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), "", true
	}
	rawMagnitude, okMagnitude := mapField(value, "$307")
	rawUnit, okUnit := mapField(value, "$306")
	if !okMagnitude || !okUnit {
		return 0, "", false
	}
	unit, _ := asString(rawUnit)
	switch typed := rawMagnitude.(type) {
	case float64:
		return typed, unit, true
	case *float64:
		if typed == nil {
			return 0, "", false
		}
		return *typed, unit, true
	case float32:
		return float64(typed), unit, true
	case *float32:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), unit, true
	case int:
		return float64(typed), unit, true
	case *int:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), unit, true
	case int32:
		return float64(typed), unit, true
	case *int32:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), unit, true
	case int64:
		return float64(typed), unit, true
	case *int64:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), unit, true
	case uint32:
		return float64(typed), unit, true
	case *uint32:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), unit, true
	case uint64:
		return float64(typed), unit, true
	case *uint64:
		if typed == nil {
			return 0, "", false
		}
		return float64(*typed), unit, true
	default:
		if parsed, err := strconv.ParseFloat(fmt.Sprint(rawMagnitude), 64); err == nil {
			return parsed, unit, true
		}
		return 0, "", false
	}
}

func cssColor(value interface{}) string {
	colorInt, ok := colorIntValue(value)
	if !ok {
		return ""
	}
	a := byte(colorInt >> 24)
	r := byte(colorInt >> 16)
	g := byte(colorInt >> 8)
	b := byte(colorInt)
	if a == 255 {
		switch (uint32(r) << 16) | (uint32(g) << 8) | uint32(b) {
		case 0x808080:
			return "gray"
		case 0xffffff:
			return "#fff"
		case 0x000000:
			return "#000"
		default:
			return fmt.Sprintf("#%02x%02x%02x", r, g, b)
		}
	}
	return fmt.Sprintf("rgba(%d,%d,%d,%s)", r, g, b, trimFloat(float64(a)/255.0))
}

func fillColor(style map[string]interface{}) string {
	_, hasColor := style["$70"]
	_, hasOpacity := style["$72"]
	if !hasColor && !hasOpacity {
		return ""
	}
	color := cssColor(style["$70"])
	if color == "" {
		color = "#ffffff"
	}
	opacity, _, ok := numericStyleValue(style["$72"])
	if !ok {
		return color
	}
	return addColorOpacity(color, opacity)
}

func addColorOpacity(color string, opacity float64) string {
	if opacity >= 0.999 {
		return color
	}
	r, g, b, _, ok := parseColor(color)
	if !ok {
		return color
	}
	if opacity <= 0.001 {
		return fmt.Sprintf("rgba(%d,%d,%d,0)", r, g, b)
	}
	return fmt.Sprintf("rgba(%d,%d,%d,%s)", r, g, b, trimFloat(opacity))
}

func colorDeclarations(style map[string]interface{}, linkStyle map[string]interface{}) string {
	for _, source := range []map[string]interface{}{style, linkStyle} {
		if value := cssColor(source["$576"]); value != "" {
			return value
		}
		if value := cssColor(source["$577"]); value != "" {
			return value
		}
	}
	return ""
}

func parseColor(value string) (int, int, int, float64, bool) {
	if strings.HasPrefix(value, "#") && len(value) == 7 {
		r, err1 := strconv.ParseInt(value[1:3], 16, 0)
		g, err2 := strconv.ParseInt(value[3:5], 16, 0)
		b, err3 := strconv.ParseInt(value[5:7], 16, 0)
		if err1 == nil && err2 == nil && err3 == nil {
			return int(r), int(g), int(b), 1, true
		}
	}
	if strings.HasPrefix(value, "rgba(") && strings.HasSuffix(value, ")") {
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(value, "rgba("), ")"), ",")
		if len(parts) != 4 {
			return 0, 0, 0, 0, false
		}
		r, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		g, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		b, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
		a, err4 := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
		if err1 == nil && err2 == nil && err3 == nil && err4 == nil {
			return r, g, b, a, true
		}
	}
	return 0, 0, 0, 0, false
}

func colorIntValue(value interface{}) (uint32, bool) {
	switch typed := value.(type) {
	case float64:
		return uint32(typed), true
	case *float64:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case int:
		return uint32(typed), true
	case *int:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case int32:
		return uint32(typed), true
	case *int32:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case int64:
		return uint32(typed), true
	case *int64:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case uint32:
		return typed, true
	case *uint32:
		if typed == nil {
			return 0, false
		}
		return *typed, true
	case uint64:
		return uint32(typed), true
	case *uint64:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	}
	raw, ok := mapField(value, "$19")
	if !ok {
		return 0, false
	}
	switch typed := raw.(type) {
	case float64:
		return uint32(typed), true
	case *float64:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case int:
		return uint32(typed), true
	case int32:
		return uint32(typed), true
	case int64:
		return uint32(typed), true
	case uint32:
		return typed, true
	case uint64:
		return uint32(typed), true
	case *int:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case *int32:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case *int64:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	case *uint32:
		if typed == nil {
			return 0, false
		}
		return *typed, true
	case *uint64:
		if typed == nil {
			return 0, false
		}
		return uint32(*typed), true
	default:
		return 0, false
	}
}

func trimFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func naturalSortKey(value string) string {
	lower := strings.ToLower(value)
	var out strings.Builder
	for index := 0; index < len(lower); {
		if lower[index] < '0' || lower[index] > '9' {
			out.WriteByte(lower[index])
			index++
			continue
		}
		start := index
		for index < len(lower) && lower[index] >= '0' && lower[index] <= '9' {
			index++
		}
		digits := lower[start:index]
		if pad := 8 - len(digits); pad > 0 {
			out.WriteString(strings.Repeat("0", pad))
		}
		out.WriteString(digits)
	}
	return out.String()
}

func mapField(value interface{}, key string) (interface{}, bool) {
	if mapped, ok := value.(map[string]interface{}); ok {
		result, found := mapped[key]
		return result, found
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || rv.Kind() != reflect.Map {
		return nil, false
	}
	for _, mapKey := range rv.MapKeys() {
		if mapKeyString(mapKey.Interface()) == key {
			return rv.MapIndex(mapKey).Interface(), true
		}
	}
	return nil, false
}

func mapKeyString(value interface{}) string {
	if text, ok := asString(value); ok {
		return text
	}
	return fmt.Sprint(value)
}

func formatStyleNumber(value float64) string {
	return strconv.FormatFloat(value, 'g', 6, 64)
}

func resourceFilename(resource resourceFragment) string {
	base := filepath.Base(resource.Location)
	ext := extensionForMediaType(resource.MediaType)
	return "image_" + base + ext
}

func fontFilename(location string, data []byte) string {
	base := filepath.Base(location)
	return "font_" + base + detectFontExtension(data)
}

func extensionForMediaType(mediaType string) string {
	switch strings.ToLower(mediaType) {
	case "image/jpg", "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/jxr":
		return ".jxr"
	default:
		return ".bin"
	}
}

func detectFontExtension(data []byte) string {
	if len(data) >= 4 {
		switch string(data[:4]) {
		case "OTTO":
			return ".otf"
		case "\x00\x01\x00\x00":
			return ".ttf"
		}
	}
	return ".bin"
}

func detectImageExtension(data []byte) string {
	if len(data) >= 4 {
		if bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}) {
			return ".jpg"
		}
		if bytes.HasPrefix(data, []byte{0x49, 0x49, 0xbc, 0x01}) {
			return ".jxr"
		}
		if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4e, 0x47}) {
			return ".png"
		}
	}
	return ".bin"
}

func blobMatchesImageMediaType(data []byte, mediaType string) bool {
	if len(data) == 0 {
		return false
	}
	switch extensionForMediaType(mediaType) {
	case ".jpg":
		return detectImageExtension(data) == ".jpg"
	case ".jxr":
		return detectImageExtension(data) == ".jxr"
	case ".png":
		return detectImageExtension(data) == ".png"
	default:
		return detectImageExtension(data) != ".bin"
	}
}

func partitionRawBlobs(rawOrder []rawBlob) ([]rawBlob, []rawBlob) {
	imagePool := make([]rawBlob, 0, len(rawOrder))
	fontPool := make([]rawBlob, 0, len(rawOrder))
	for _, blob := range rawOrder {
		switch {
		case detectFontExtension(blob.Data) != ".bin":
			fontPool = append(fontPool, blob)
		case detectImageExtension(blob.Data) != ".bin":
			imagePool = append(imagePool, blob)
		}
	}
	return imagePool, fontPool
}

func nextMatchingBlob(blobs []rawBlob, start int, mediaType string) ([]byte, int) {
	for index := start; index < len(blobs); index++ {
		if blobMatchesImageMediaType(blobs[index].Data, mediaType) {
			return blobs[index].Data, index + 1
		}
	}
	return nil, start
}

func nextFontBlob(blobs []rawBlob, start int) ([]byte, int) {
	for index := start; index < len(blobs); index++ {
		if detectFontExtension(blobs[index].Data) != ".bin" {
			return blobs[index].Data, index + 1
		}
	}
	return nil, start
}

func fontMediaType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".otf":
		return "font/otf"
	case ".ttf":
		return "font/ttf"
	default:
		return "application/octet-stream"
	}
}

func convertJXRResource(data []byte) ([]byte, string, error) {
	img, err := jxr.DecodeGray8(data)
	if err != nil {
		return nil, "", err
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 95}); err != nil {
		return nil, "", err
	}
	return encoded.Bytes(), "image/jpeg", nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func debugSectionMappings(sections map[string]sectionFragment, navTitles map[string]string, order []string) {
	if os.Getenv("KFX_DEBUG") == "" {
		return
	}
	for _, sectionID := range order {
		section := sections[sectionID]
		fmt.Fprintf(os.Stderr, "section id=%s pos=%d storyline=%s title=%s\n", sectionID, section.PositionID, section.Storyline, navTitles[sectionID])
	}
}

func debugStorylineNodes(sectionID string, nodes []interface{}, depth int) {
	if os.Getenv("KFX_DEBUG") == "" {
		return
	}
	debugSections := os.Getenv("KFX_DEBUG_SECTIONS")
	if debugSections == "" {
		if sectionID != "c73" && sectionID != "c109" && sectionID != "c6P" {
			return
		}
	} else if !strings.Contains(","+debugSections+",", ","+sectionID+",") {
		return
	}
	prefix := strings.Repeat("  ", depth)
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		positionID, _ := asInt(node["$155"])
		styleID, _ := asString(node["$157"])
		text := ""
		if ref, ok := asMap(node["$145"]); ok {
			text = truncateDebugText(ref)
		}
		fmt.Fprintf(os.Stderr, "story %s %spos=%d type=%s style=%s text=%q keys=%v\n", sectionID, prefix, positionID, asStringDefault(node["$159"]), styleID, text, sortedMapKeys(node))
		if cols, ok := asSlice(node["$152"]); ok {
			fmt.Fprintf(os.Stderr, "story %s %scols=%#v\n", sectionID, prefix, cols)
		}
		if children, ok := asSlice(node["$146"]); ok {
			debugStorylineNodes(sectionID, children, depth+1)
		}
	}
}

func sortedMapKeys(value map[string]interface{}) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func truncateDebugText(ref map[string]interface{}) string {
	name, _ := asString(ref["name"])
	index, _ := asInt(ref["$403"])
	return fmt.Sprintf("%s[%d]", name, index)
}

func collectNavigationIDs(navRoots []map[string]interface{}) []string {
	var ids []string
	for _, root := range navRoots {
		entries, ok := asSlice(root["$392"])
		if !ok {
			continue
		}
		for _, entry := range entries {
			if id, ok := asString(entry); ok && id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func navigationType(value map[string]interface{}) string {
	navType, _ := asString(value["$235"])
	return navType
}

func parseNavTitle(value map[string]interface{}) string {
	label, ok := asMap(value["$241"])
	if !ok {
		return ""
	}
	title, _ := asString(label["$244"])
	return strings.TrimSpace(title)
}

func parseNavTarget(value map[string]interface{}) navTarget {
	target, ok := asMap(value["$246"])
	if !ok {
		return navTarget{}
	}
	positionID, _ := asInt(target["$155"])
	if positionID == 0 {
		positionID, _ = asInt(target["$598"])
	}
	offset, _ := asInt(target["$143"])
	return navTarget{PositionID: positionID, Offset: offset}
}

func countNavPoints(points []navPoint) int {
	count := 0
	for _, point := range points {
		count++
		count += countNavPoints(point.Children)
	}
	return count
}

func flattenNavigationTitles(points []navPoint, positionToSection map[int]string, titles map[string]string) {
	for _, point := range points {
		if sectionID, ok := positionToSection[point.Target.PositionID]; ok && titles[sectionID] == "" && point.Title != "" {
			titles[sectionID] = point.Title
		}
		flattenNavigationTitles(point.Children, positionToSection, titles)
	}
}

func orderedSectionIDsFromNavigation(points []navPoint, positionToSection map[int]string) []string {
	var ordered []string
	var walk func(items []navPoint)
	walk = func(items []navPoint) {
		for _, point := range items {
			if sectionID, ok := positionToSection[point.Target.PositionID]; ok {
				ordered = append(ordered, sectionID)
			}
			walk(point.Children)
		}
	}
	walk(points)
	return ordered
}

func navigationToEPUB(points []navPoint, targetHref func(navTarget) string) []epub.NavPoint {
	output := make([]epub.NavPoint, 0, len(points))
	for _, point := range points {
		href := targetHref(point.Target)
		if href == "" || point.Title == "" {
			continue
		}
		output = append(output, epub.NavPoint{
			Title:    point.Title,
			Href:     href,
			Children: navigationToEPUB(point.Children, targetHref),
		})
	}
	return output
}

type navProcessor struct {
	tocEntryCount   int
	usedAnchorNames map[string]bool
	positionAnchors map[int]map[int][]string
	navContainers   map[string]map[string]interface{}
	toc             []navPoint
	guide           []guideEntry
	pages           []pageEntry
}

func processNavigation(navRoots []map[string]interface{}, navContainers map[string]map[string]interface{}) navProcessor {
	state := navProcessor{
		usedAnchorNames: map[string]bool{},
		positionAnchors: map[int]map[int][]string{},
		navContainers:   navContainers,
	}
	navIDs := collectNavigationIDs(navRoots)
	hasNavHeadings := false
	for _, navID := range navIDs {
		if container := navContainers[navID]; container != nil && navigationType(container) == "$798" {
			hasNavHeadings = true
			break
		}
	}
	for _, navID := range navIDs {
		container := navContainers[navID]
		if container == nil {
			continue
		}
		state.processContainer(container, hasNavHeadings)
	}
	return state
}

func (p *navProcessor) processContainer(container map[string]interface{}, hasNavHeadings bool) {
	navType := navigationType(container)
	if imports, ok := asSlice(container["imports"]); ok {
		for _, raw := range imports {
			importName, ok := asString(raw)
			if !ok || importName == "" {
				continue
			}
			if imported := p.navContainers[importName]; imported != nil {
				p.processContainer(imported, hasNavHeadings)
			}
		}
	}
	entries, ok := asSlice(container["$247"])
	if !ok {
		return
	}
	for _, raw := range entries {
		entry, ok := asMap(raw)
		if !ok {
			continue
		}
		switch navType {
		case "$212", "$213", "$214", "$798":
			p.processNavUnit(navType, entry, &p.toc, navType == "$212" && !hasNavHeadings, nil)
		case "$236":
			p.processGuideUnit(entry)
		case "$237":
			p.processPageUnit(entry)
		}
	}
}

func (p *navProcessor) processGuideUnit(entry map[string]interface{}) {
	label := parseNavTitle(entry)
	target := parseNavTarget(entry)
	if target.PositionID == 0 {
		return
	}
	navUnitName, _ := asString(entry["$240"])
	if navUnitName == "" {
		navUnitName = label
	}
	guideType := guideTypeForLandmark(asStringDefault(entry["$238"]))
	anchorName := p.uniqueAnchorName(navUnitName)
	if anchorName == "" {
		anchorName = p.uniqueAnchorName(guideType)
	}
	p.registerAnchor(anchorName, target)
	if label == "cover-nav-unit" {
		label = ""
	}
	p.guide = append(p.guide, guideEntry{Type: guideType, Title: label, Target: target})
}

func (p *navProcessor) processPageUnit(entry map[string]interface{}) {
	label := parseNavTitle(entry)
	if debug := os.Getenv("KFX_DEBUG_PAGES"); debug != "" {
		fmt.Fprintf(os.Stderr, "page unit label=%q entry=%#v\n", label, entry)
	}
	if label == "" {
		return
	}
	target := parseNavTarget(entry)
	if target.PositionID == 0 {
		return
	}
	p.registerAnchor(p.uniqueAnchorName("page_"+label), target)
	p.pages = append(p.pages, pageEntry{Label: label, Target: target})
}

func (p *navProcessor) processNavUnit(navType string, entry map[string]interface{}, out *[]navPoint, defaultHeading bool, headingLevel *int) {
	label := parseNavTitle(entry)
	navUnitName, _ := asString(entry["$240"])
	if navUnitName == "" {
		navUnitName = label
	}
	nextHeading := (*int)(nil)
	if navType == "$798" {
		if level, ok := headingLevelForLandmark(asStringDefault(entry["$238"])); ok {
			headingLevel = intPtr(level)
			nextHeading = intPtr(level)
		}
		if label == "heading-nav-unit" {
			label = ""
		}
		if navUnitName == "heading-nav-unit" {
			navUnitName = ""
		}
	} else if defaultHeading {
		nextHeading = intPtr(2)
	} else if headingLevel != nil && *headingLevel < 6 {
		nextHeading = intPtr(*headingLevel + 1)
	}

	childrenRaw, _ := asSlice(entry["$247"])
	children := make([]navPoint, 0, len(childrenRaw))
	for _, raw := range childrenRaw {
		child, ok := asMap(raw)
		if !ok {
			continue
		}
		p.processNavUnit(navType, child, &children, false, nextHeading)
	}

	target := parseNavTarget(entry)
	hasTarget := target.PositionID != 0
	if hasTarget {
		anchorName := fmt.Sprintf("%s_%d_%s", navType, p.tocEntryCount, navUnitName)
		p.tocEntryCount++
		p.registerAnchor(anchorName, target)
	}
	if navType == "$798" {
		return
	}
	if label == "" && !hasTarget {
		*out = append(*out, children...)
		return
	}
	*out = append(*out, navPoint{Title: label, Target: target, Children: children})
}

func (p *navProcessor) uniqueAnchorName(name string) string {
	if name == "" {
		return ""
	}
	if !p.usedAnchorNames[name] {
		p.usedAnchorNames[name] = true
		return name
	}
	for index := 0; ; index++ {
		candidate := fmt.Sprintf("%s:%d", name, index)
		if !p.usedAnchorNames[candidate] {
			p.usedAnchorNames[candidate] = true
			return candidate
		}
	}
}

func (p *navProcessor) registerAnchor(name string, target navTarget) {
	if name == "" || target.PositionID == 0 {
		return
	}
	offsets := p.positionAnchors[target.PositionID]
	if offsets == nil {
		offsets = map[int][]string{}
		p.positionAnchors[target.PositionID] = offsets
	}
	offsets[target.Offset] = append(offsets[target.Offset], name)
}

func guideTypeForLandmark(value string) string {
	switch value {
	case "$233":
		return "cover"
	case "$396", "$269":
		return "text"
	case "$212":
		return "toc"
	default:
		return value
	}
}

func headingLevelForLandmark(value string) (int, bool) {
	switch value {
	case "$799":
		return 1, true
	case "$800":
		return 2, true
	case "$801":
		return 3, true
	case "$802":
		return 4, true
	case "$803":
		return 5, true
	case "$804":
		return 6, true
	default:
		return 0, false
	}
}

func guideToEPUB(entries []guideEntry, targetHref func(navTarget) string) []epub.GuideEntry {
	out := make([]epub.GuideEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Type != "cover" && entry.Type != "toc" {
			continue
		}
		href := targetHref(entry.Target)
		if href == "" {
			continue
		}
		title := entry.Title
		if title == "" {
			title = strings.Title(entry.Type)
		}
		out = append(out, epub.GuideEntry{Type: entry.Type, Title: title, Href: href})
	}
	return out
}

func pagesToEPUB(entries []pageEntry, targetHref func(navTarget) string) []epub.PageTarget {
	out := make([]epub.PageTarget, 0, len(entries))
	for _, entry := range entries {
		href := targetHref(entry.Target)
		if href == "" {
			continue
		}
		out = append(out, epub.PageTarget{Label: entry.Label, Href: href})
	}
	return out
}

func buildPositionAnchorIDs(positionAnchors map[int]map[int][]string) map[int]map[int]string {
	seen := map[string]bool{}
	result := map[int]map[int]string{}
	positionIDs := make([]int, 0, len(positionAnchors))
	for positionID := range positionAnchors {
		positionIDs = append(positionIDs, positionID)
	}
	sort.Ints(positionIDs)
	for _, positionID := range positionIDs {
		offsets := positionAnchors[positionID]
		offsetIDs := make([]int, 0, len(offsets))
		for offset := range offsets {
			offsetIDs = append(offsetIDs, offset)
		}
		sort.Ints(offsetIDs)
		result[positionID] = map[int]string{}
		for _, offset := range offsetIDs {
			names := offsets[offset]
			if len(names) == 0 {
				continue
			}
			id := makeUniqueHTMLID(names[0], seen)
			seen[id] = true
			result[positionID][offset] = id
		}
	}
	return result
}

func makeUniqueHTMLID(name string, seen map[string]bool) string {
	base := fixHTMLID(name)
	if !seen[base] {
		return base
	}
	for index := 0; ; index++ {
		candidate := fmt.Sprintf("%s%d", base, index)
		if !seen[candidate] {
			return candidate
		}
	}
}

func fixHTMLID(id string) string {
	var out strings.Builder
	for _, r := range id {
		switch {
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-':
			out.WriteRune(r)
		default:
			out.WriteByte('_')
		}
	}
	fixed := out.String()
	if fixed == "" || !((fixed[0] >= 'A' && fixed[0] <= 'Z') || (fixed[0] >= 'a' && fixed[0] <= 'z')) {
		fixed = "id_" + fixed
	}
	return fixed
}

func asStringDefault(value interface{}) string {
	result, _ := asString(value)
	return result
}

func intPtr(value int) *int {
	return &value
}

func mergeSectionOrder(primary []string, fallback []string) []string {
	seen := map[string]bool{}
	merged := make([]string, 0, len(primary)+len(fallback))
	for _, sectionID := range primary {
		if sectionID == "" || seen[sectionID] {
			continue
		}
		seen[sectionID] = true
		merged = append(merged, sectionID)
	}
	for _, sectionID := range fallback {
		if sectionID == "" || seen[sectionID] {
			continue
		}
		seen[sectionID] = true
		merged = append(merged, sectionID)
	}
	return merged
}

func sectionFilename(sectionID string) string {
	return sectionID + ".xhtml"
}

func chooseFragmentIdentity(fragmentID string, rawValue interface{}) string {
	valueID, _ := asString(rawValue)
	if isResolvedIdentity(valueID) {
		return valueID
	}
	if isResolvedIdentity(fragmentID) {
		return fragmentID
	}
	if valueID != "" {
		return valueID
	}
	return fragmentID
}

func isResolvedIdentity(value string) bool {
	if value == "" {
		return false
	}
	return !(strings.HasPrefix(value, "$") && len(value) > 1)
}

func isPlaceholderSymbol(value string) bool {
	if !strings.HasPrefix(value, "$") || len(value) == 1 {
		return false
	}
	for _, r := range value[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func newStyleCatalog() *styleCatalog {
	return &styleCatalog{
		staticRules: map[string]string{},
		byKey:       map[string]*styleEntry{},
		byToken:     map[string]*styleEntry{},
	}
}

func (c *styleCatalog) addStatic(selector string, declarations []string) {
	if c == nil || selector == "" || len(declarations) == 0 {
		return
	}
	if !strings.HasPrefix(selector, ".") {
		selector = "." + selector
	}
	if _, ok := c.staticRules[selector]; ok {
		return
	}
	c.staticRules[selector] = strings.Join(canonicalDeclarations(declarations), "; ")
}

func (c *styleCatalog) bind(baseName string, declarations []string) string {
	if c == nil || baseName == "" || len(declarations) == 0 {
		return ""
	}
	baseName = strings.TrimPrefix(baseName, ".")
	declarations = canonicalDeclarations(declarations)
	key := baseName + "\x00" + strings.Join(declarations, "; ")
	if entry, ok := c.byKey[key]; ok {
		entry.count++
		return entry.token
	}
	entry := &styleEntry{
		token:        fmt.Sprintf("__STYLE_%d__", len(c.entries)),
		baseName:     baseName,
		declarations: strings.Join(declarations, "; "),
		count:        1,
		order:        len(c.entries),
	}
	c.entries = append(c.entries, entry)
	c.byKey[key] = entry
	c.byToken[entry.token] = entry
	c.finalized = false
	c.css = ""
	c.replacements = nil
	return entry.token
}

func (c *styleCatalog) finalize() {
	if c == nil || c.finalized {
		return
	}
	c.finalized = true
	if len(c.entries) == 0 && len(c.staticRules) == 0 {
		c.css = ""
		return
	}
	usedEntries := make([]*styleEntry, 0, len(c.entries))
	groupSizes := map[string]int{}
	for _, entry := range c.entries {
		if !entry.referenced {
			continue
		}
		usedEntries = append(usedEntries, entry)
		groupSizes[entry.baseName]++
	}
	sortedEntries := append([]*styleEntry(nil), usedEntries...)
	sort.SliceStable(sortedEntries, func(i, j int) bool {
		if sortedEntries[i].count == sortedEntries[j].count {
			return sortedEntries[i].order < sortedEntries[j].order
		}
		return sortedEntries[i].count > sortedEntries[j].count
	})
	nextIndex := map[string]int{}
	usedNames := map[string]bool{}
	for selector := range c.staticRules {
		usedNames[strings.TrimPrefix(selector, ".")] = true
	}
	usedNames["class_s8"] = true
	for _, entry := range sortedEntries {
		finalName := entry.baseName
		if entry.baseName == "class" {
			for {
				finalName = fmt.Sprintf("%s-%d", entry.baseName, nextIndex[entry.baseName])
				nextIndex[entry.baseName]++
				if !usedNames[finalName] {
					break
				}
			}
		} else if groupSizes[entry.baseName] > 1 || usedNames[finalName] {
			for {
				finalName = fmt.Sprintf("%s-%d", entry.baseName, nextIndex[entry.baseName])
				nextIndex[entry.baseName]++
				if !usedNames[finalName] {
					break
				}
			}
		}
		entry.finalName = finalName
		usedNames[finalName] = true
		c.replacements = append(c.replacements, entry.token, finalName)
	}
	rules := map[string]string{}
	selectors := make([]string, 0, len(c.staticRules)+len(c.entries))
	for selector, declarations := range c.staticRules {
		rules[selector] = declarations
		selectors = append(selectors, selector)
	}
	for _, entry := range usedEntries {
		selector := "." + entry.finalName
		if _, ok := rules[selector]; ok {
			continue
		}
		rules[selector] = entry.declarations
		selectors = append(selectors, selector)
	}
	sort.Slice(selectors, func(i, j int) bool {
		return naturalSortKey(selectors[i]) < naturalSortKey(selectors[j])
	})
	lines := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		lines = append(lines, selector+" {"+rules[selector]+"}")
	}
	c.css = strings.Join(lines, "\n")
}

func (c *styleCatalog) replacer() *strings.Replacer {
	if c == nil {
		return strings.NewReplacer()
	}
	c.finalize()
	if len(c.replacements) == 0 {
		return strings.NewReplacer()
	}
	return strings.NewReplacer(c.replacements...)
}

func (c *styleCatalog) markReferenced(content string) {
	if c == nil || content == "" {
		return
	}
	for _, token := range styleTokenPattern.FindAllString(content, -1) {
		if entry, ok := c.byToken[token]; ok {
			entry.referenced = true
		}
	}
}

func (c *styleCatalog) String() string {
	if c == nil {
		return ""
	}
	c.finalize()
	return c.css
}

func (r *storylineRenderer) renderStoryline(sectionPositionID int, bodyStyleID string, bodyStyleValues map[string]interface{}, storyline map[string]interface{}, nodes []interface{}) renderedStoryline {
	result := renderedStoryline{}
	contentNodes := nodes
	promotedBody := false
	if bodyStyleID == "" {
		if promotedStyleID, promotedNodes, ok := promotedBodyContainer(nodes); ok {
			bodyStyleID = promotedStyleID
			bodyStyleValues = nil
			contentNodes = promotedNodes
			promotedBody = true
		}
	}
	if promotedBody {
		bodyStyleValues = mergeStyleValues(bodyStyleValues, r.inferPromotedBodyStyle(contentNodes))
	}
	if bodyStyleID == "" && len(bodyStyleValues) == 0 {
		bodyStyleValues = r.inferBodyStyleValues(contentNodes, defaultInheritedBodyStyle())
		if len(bodyStyleValues) == 0 {
			bodyStyleValues = map[string]interface{}{
				"$11": defaultInheritedBodyStyle()["$11"],
			}
		}
	}
	if os.Getenv("KFX_DEBUG_BODY") != "" {
		fmt.Fprintf(os.Stderr, "body infer styleID=%s values=%#v\n", bodyStyleID, bodyStyleValues)
	}
	r.activeBodyClass = ""
	r.activeBodyDefaults = nil
	r.firstVisibleSeen = false
	if bodyStyleID == "" {
		bodyStyleID, _ = asString(storyline["$157"])
	}
	bodyStyle := effectiveStyle(r.styleFragments[bodyStyleID], bodyStyleValues)
	bodyDeclarations := bodyStyleDeclarations(bodyStyle)
	if bodyStyleID == "" && len(bodyDeclarations) == 0 {
		bodyStyleValues = map[string]interface{}{
			"$11": defaultInheritedBodyStyle()["$11"],
		}
		bodyStyle = effectiveStyle(r.styleFragments[bodyStyleID], bodyStyleValues)
		bodyDeclarations = bodyStyleDeclarations(bodyStyle)
	}
	if len(bodyDeclarations) > 0 {
		if bodyClass := staticBodyClassForDeclarations(bodyDeclarations); bodyClass != "" {
			result.BodyClass = bodyClass
		} else if bodyStyleID != "" || len(bodyStyleValues) > 0 {
			if className := r.bodyClass(bodyStyleID, bodyStyleValues); className != "" {
				result.BodyClass = className
			}
		}
	}
	if os.Getenv("KFX_DEBUG_BODY") != "" {
		fmt.Fprintf(os.Stderr, "body resolved styleID=%s decls=%v class=%s\n", bodyStyleID, bodyDeclarations, result.BodyClass)
	}
	if len(bodyDeclarations) > 0 {
		if isStaticBodyClass(result.BodyClass) {
			r.activeBodyDefaults = inheritedDefaultSet(defaultBodyDeclarations(result.BodyClass))
		} else {
			r.activeBodyDefaults = inheritedDefaultSet(bodyDeclarations)
		}
	}
	if result.BodyClass != "" {
		r.activeBodyClass = result.BodyClass
	}
	if styleID, _ := asString(storyline["$157"]); styleID != "" && result.BodyClass == "" {
		if className := r.bodyClass(styleID, nil); className != "" {
			result.BodyClass = className
		}
	}
	bodyParts := make([]htmlPart, 0, len(contentNodes))
	for _, node := range contentNodes {
		rendered := r.renderNode(node, 0)
		if rendered != nil {
			bodyParts = append(bodyParts, rendered)
		}
	}
	root := &htmlElement{Attrs: map[string]string{}, Children: bodyParts}
	r.promoteCommonChildStyles(root)
	r.applyPositionAnchors(root, sectionPositionID, false)
	result.BodyHTML = renderHTMLParts(root.Children, true)
	if result.BodyClass == "" && len(bodyDeclarations) > 0 {
		if bodyClass := staticBodyClassForDeclarations(bodyDeclarations); bodyClass != "" {
			result.BodyClass = bodyClass
		} else {
			result.BodyClass = r.styles.bind("class", bodyDeclarations)
		}
	}
	switch result.BodyClass {
	case "class-0", "class-1", "class-2", "class-3", "class-7", "class-8":
		r.styles.addStatic(result.BodyClass, defaultBodyDeclarations(result.BodyClass))
	}
	r.activeBodyClass = result.BodyClass
	if isStaticBodyClass(result.BodyClass) {
		r.activeBodyDefaults = inheritedDefaultSet(defaultBodyDeclarations(result.BodyClass))
	}
	if strings.Contains(result.BodyHTML, "<svg ") {
		result.Properties = "svg"
	}
	return result
}

func (r *storylineRenderer) promoteCommonChildStyles(element *htmlElement) {
	if element == nil {
		return
	}
	for _, child := range element.Children {
		if childElement, ok := child.(*htmlElement); ok {
			r.promoteCommonChildStyles(childElement)
		}
	}
	if element.Tag != "div" {
		return
	}
	baseName, parentStyle, ok := r.dynamicClassStyle(element.Attrs["class"])
	if !ok {
		return
	}
	children := make([]*htmlElement, 0, len(element.Children))
	for _, child := range element.Children {
		childElement, ok := child.(*htmlElement)
		if !ok {
			continue
		}
		children = append(children, childElement)
	}
	if len(children) == 0 {
		return
	}
	const reverseInheritanceFraction = 0.8
	keys := []string{"font-family", "font-style", "font-weight", "font-variant", "text-align", "text-indent", "text-transform"}
	valueCounts := map[string]map[string]int{}
	for _, child := range children {
		_, childStyle, ok := r.dynamicClassStyle(child.Attrs["class"])
		if !ok {
			continue
		}
		for _, key := range keys {
			value := childStyle[key]
			if value == "" {
				continue
			}
			if valueCounts[key] == nil {
				valueCounts[key] = map[string]int{}
			}
			valueCounts[key][value]++
		}
	}
	newHeritable := map[string]string{}
	for _, key := range keys {
		values := valueCounts[key]
		if len(values) == 0 {
			continue
		}
		total := 0
		mostCommonValue := ""
		mostCommonCount := 0
		for value, count := range values {
			total += count
			if count > mostCommonCount {
				mostCommonValue = value
				mostCommonCount = count
			}
		}
		if total < len(children) && parentStyle[key] == "" {
			continue
		}
		if float64(mostCommonCount) >= float64(len(children))*reverseInheritanceFraction {
			newHeritable[key] = mostCommonValue
		}
	}
	if len(newHeritable) == 0 {
		return
	}
	oldParentStyle := cloneStyleMap(parentStyle)
	for _, child := range children {
		childBaseName, childStyle, ok := r.dynamicClassStyle(child.Attrs["class"])
		if !ok {
			continue
		}
		for key, newValue := range newHeritable {
			if childStyle[key] == newValue {
				delete(childStyle, key)
			} else if childStyle[key] == "" && oldParentStyle[key] != "" && oldParentStyle[key] != newValue {
				childStyle[key] = oldParentStyle[key]
			}
		}
		r.setDynamicClassStyle(child, childBaseName, childStyle)
	}
	for key, value := range newHeritable {
		parentStyle[key] = value
	}
	r.setDynamicClassStyle(element, baseName, parentStyle)
}

func cloneStyleMap(style map[string]string) map[string]string {
	if len(style) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(style))
	for key, value := range style {
		cloned[key] = value
	}
	return cloned
}

func (r *storylineRenderer) dynamicClassStyle(className string) (string, map[string]string, bool) {
	if r == nil || className == "" || r.styles == nil {
		return "", nil, false
	}
	entry, ok := r.styles.byToken[className]
	if !ok {
		return "", nil, false
	}
	return entry.baseName, parseDeclarationString(entry.declarations), true
}

func (r *storylineRenderer) setDynamicClassStyle(element *htmlElement, baseName string, style map[string]string) {
	if element == nil {
		return
	}
	if len(style) == 0 {
		delete(element.Attrs, "class")
		return
	}
	declarations := declarationListFromStyleMap(style)
	if len(declarations) == 0 {
		delete(element.Attrs, "class")
		return
	}
	element.Attrs["class"] = r.styles.bind(baseName, declarations)
}

func (r *storylineRenderer) renderNode(raw interface{}, depth int) htmlPart {
	node, ok := asMap(raw)
	if !ok {
		return nil
	}
	switch asStringDefault(node["$159"]) {
	case "$278":
		if table := r.renderTableNode(node, depth); table != nil {
			return table
		}
	case "$270":
		if container := r.renderFittedContainer(node, depth); container != nil {
			return container
		}
	case "$454":
		if tbody := r.renderStructuredContainer(node, "tbody", depth); tbody != nil {
			return tbody
		}
	case "$151":
		if thead := r.renderStructuredContainer(node, "thead", depth); thead != nil {
			return thead
		}
	case "$455":
		if tfoot := r.renderStructuredContainer(node, "tfoot", depth); tfoot != nil {
			return tfoot
		}
	case "$279":
		if row := r.renderTableRow(node, depth); row != nil {
			return row
		}
	}

	if imageNode := r.renderImageNode(node); imageNode != nil {
		return imageNode
	}

	if textNode := r.renderTextNode(node, depth); textNode != nil {
		return textNode
	}

	children, ok := asSlice(node["$146"])
	if !ok {
		if hasRenderableContainer(node) {
			element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
			if className := r.containerClass(node); className != "" {
				element.Attrs["class"] = className
			}
			return element
		}
		return nil
	}

	container := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	for _, child := range children {
		rendered := r.renderNode(child, depth+1)
		if rendered != nil {
			container.Children = append(container.Children, rendered)
		}
	}
	if len(container.Children) == 0 {
		return nil
	}
	if className := r.containerClass(node); className != "" {
		container.Attrs["class"] = className
	}
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(container, positionID, false)
	}
	return container
}

func (r *storylineRenderer) renderFittedContainer(node map[string]interface{}, depth int) htmlPart {
	fitWidth, _ := asBool(node["$478"])
	if !fitWidth {
		return nil
	}
	outer := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	if className := r.containerClass(node); className != "" {
		outer.Attrs["class"] = className
	}
	inner := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	children, _ := asSlice(node["$146"])
	for _, child := range children {
		rendered := r.renderNode(child, depth+1)
		if rendered != nil {
			inner.Children = append(inner.Children, rendered)
		}
	}
	if len(inner.Children) == 0 {
		return nil
	}
	styleID, _ := asString(node["$157"])
	baseName := "class"
	if styleID != "" {
		baseName = "class_" + styleID
	}
	if className := r.styles.bind(baseName, []string{"display: inline-block"}); className != "" {
		inner.Attrs["class"] = className
	}
	outer.Children = []htmlPart{inner}
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(outer, positionID, false)
	}
	return outer
}

func (r *storylineRenderer) renderTableNode(node map[string]interface{}, depth int) htmlPart {
	table := &htmlElement{Tag: "table", Attrs: map[string]string{}}
	if className := r.tableClass(node); className != "" {
		table.Attrs["class"] = className
	}
	if cols, ok := asSlice(node["$152"]); ok && len(cols) > 0 {
		colgroup := &htmlElement{Tag: "colgroup", Attrs: map[string]string{}}
		for _, raw := range cols {
			colMap, ok := asMap(raw)
			if !ok {
				continue
			}
			col := &htmlElement{Tag: "col", Attrs: map[string]string{}}
			if className := r.tableColumnClass(colMap); className != "" {
				col.Attrs["class"] = className
			}
			colgroup.Children = append(colgroup.Children, col)
		}
		if len(colgroup.Children) > 0 {
			table.Children = append(table.Children, colgroup)
		}
	}
	if children, ok := asSlice(node["$146"]); ok {
		for _, child := range children {
			rendered := r.renderNode(child, depth+1)
			if rendered != nil {
				table.Children = append(table.Children, rendered)
			}
		}
	}
	if len(table.Children) == 0 {
		return nil
	}
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(table, positionID, false)
	}
	return table
}

func (r *storylineRenderer) renderStructuredContainer(node map[string]interface{}, tag string, depth int) htmlPart {
	element := &htmlElement{Tag: tag, Attrs: map[string]string{}}
	if className := r.structuredContainerClass(node); className != "" {
		element.Attrs["class"] = className
	}
	if children, ok := asSlice(node["$146"]); ok {
		for _, child := range children {
			rendered := r.renderNode(child, depth+1)
			if rendered != nil {
				element.Children = append(element.Children, rendered)
			}
		}
	}
	if len(element.Children) == 0 {
		return nil
	}
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderTableRow(node map[string]interface{}, depth int) htmlPart {
	row := &htmlElement{Tag: "tr", Attrs: map[string]string{}}
	if styleID, _ := asString(node["$157"]); styleID != "" {
		if className := r.structuredContainerClass(node); className != "" {
			row.Attrs["class"] = className
		}
	}
	children, _ := asSlice(node["$146"])
	for _, child := range children {
		cellNode, ok := asMap(child)
		if !ok {
			continue
		}
		cell := r.renderTableCell(cellNode, depth+1)
		if cell != nil {
			row.Children = append(row.Children, cell)
		}
	}
	if len(row.Children) == 0 {
		return nil
	}
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(row, positionID, false)
	}
	return row
}

func (r *storylineRenderer) renderTableCell(node map[string]interface{}, depth int) htmlPart {
	cell := &htmlElement{Tag: "td", Attrs: map[string]string{}}
	if className := r.tableCellClass(node); className != "" {
		cell.Attrs["class"] = className
	}
	if ref, ok := asMap(node["$145"]); ok {
		text := r.resolveText(ref)
		if text != "" {
			cell.Children = append(cell.Children, r.applyAnnotations(text, node)...)
		}
	} else if children, ok := asSlice(node["$146"]); ok {
		for _, child := range children {
			childNode, ok := asMap(child)
			if !ok {
				continue
			}
			if ref, ok := asMap(childNode["$145"]); ok {
				text := r.resolveText(ref)
				if text != "" {
					cell.Children = append(cell.Children, r.applyAnnotations(text, childNode)...)
				}
				continue
			}
			if rendered := r.renderNode(childNode, depth+1); rendered != nil {
				cell.Children = append(cell.Children, rendered)
			} else if inline := r.renderInlinePart(childNode, depth+1); inline != nil {
				cell.Children = append(cell.Children, inline)
			}
		}
	}
	if len(cell.Children) == 0 {
		return nil
	}
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(cell, positionID, false)
	}
	return cell
}

func (r *storylineRenderer) renderInlinePart(raw interface{}, depth int) htmlPart {
	node, ok := asMap(raw)
	if !ok {
		return nil
	}
	if imageNode := r.renderImageNode(node); imageNode != nil {
		return imageNode
	}
	if ref, ok := asMap(node["$145"]); ok {
		text := r.resolveText(ref)
		if text == "" {
			return nil
		}
		content := r.applyAnnotations(text, node)
		styleID, _ := asString(node["$157"])
		positionID, _ := asInt(node["$155"])
		if styleID == "" && positionID == 0 && len(content) == 1 {
			return content[0]
		}
		element := &htmlElement{Tag: "span", Attrs: map[string]string{}, Children: content}
		if className := r.spanClass(styleID); className != "" {
			element.Attrs["class"] = className
		}
		if positionID != 0 {
			r.applyPositionAnchors(element, positionID, false)
		}
		return element
	}
	children, ok := asSlice(node["$146"])
	if !ok {
		return nil
	}
	styleID, _ := asString(node["$157"])
	container := &htmlElement{Tag: "span", Attrs: map[string]string{}}
	for _, child := range children {
		if rendered := r.renderInlinePart(child, depth+1); rendered != nil {
			container.Children = append(container.Children, rendered)
		}
	}
	if len(container.Children) == 0 {
		return nil
	}
	if className := r.inlineContainerClass(styleID, node); className != "" {
		container.Attrs["class"] = className
	}
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(container, positionID, false)
	}
	return container
}

func (r *storylineRenderer) renderImageNode(node map[string]interface{}) htmlPart {
	resourceID, _ := asString(node["$175"])
	if resourceID == "" {
		return nil
	}
	href := r.resourceHrefByID[resourceID]
	if href == "" {
		return nil
	}
	alt, _ := asString(node["$584"])
	image := &htmlElement{
		Tag:   "img",
		Attrs: map[string]string{"src": href, "alt": alt},
	}
	wrapperClass, imageClass := r.imageClasses(node)
	if imageClass != "" {
		image.Attrs["class"] = imageClass
	}
	if wrapperClass == "" {
		firstVisible := r.consumeVisibleElement()
		if positionID, _ := asInt(node["$155"]); positionID != 0 {
			r.applyPositionAnchors(image, positionID, firstVisible)
		}
		return image
	}
	wrapper := &htmlElement{
		Tag:      "div",
		Attrs:    map[string]string{"class": wrapperClass},
		Children: []htmlPart{image},
	}
	firstVisible := r.consumeVisibleElement()
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(wrapper, positionID, firstVisible)
	}
	return wrapper
}

func (r *storylineRenderer) renderTextNode(node map[string]interface{}, depth int) htmlPart {
	_ = depth
	ref, ok := asMap(node["$145"])
	if !ok {
		return nil
	}
	text := r.resolveText(ref)
	if text == "" {
		return nil
	}
	positionID, _ := asInt(node["$155"])
	if os.Getenv("KFX_DEBUG") != "" && (positionID == 1110 || positionID == 1111 || positionID == 1177 || positionID == 1178) {
		fmt.Fprintf(os.Stderr, "render text pos=%d text=%q style=%s\n", positionID, text[:minInt(len(text), 32)], asStringDefault(node["$157"]))
	}
	content := r.applyAnnotations(text, node)

	styleID, _ := asString(node["$157"])
	if level := headingLevel(node); level > 0 {
		firstVisible := r.consumeVisibleElement()
		element := &htmlElement{
			Tag:      fmt.Sprintf("h%d", level),
			Attrs:    map[string]string{},
			Children: content,
		}
		if styleID != "" {
			if className := r.headingClass(styleID); className != "" {
				element.Attrs["class"] = className
			}
		}
		r.applyPositionAnchors(element, positionID, firstVisible)
		return element
	}

	firstVisible := r.consumeVisibleElement()
	element := &htmlElement{
		Tag:      "p",
		Attrs:    map[string]string{},
		Children: content,
	}
	if styleID != "" {
		if className := r.paragraphClass(styleID, fullParagraphAnnotationStyleID(node, text)); className != "" {
			element.Attrs["class"] = className
		}
	}
	r.applyPositionAnchors(element, positionID, firstVisible)
	return element
}

func (r *storylineRenderer) bodyClass(styleID string, values map[string]interface{}) string {
	style := effectiveStyle(r.styleFragments[styleID], values)
	if len(style) == 0 {
		return ""
	}
	declarations := bodyStyleDeclarations(style)
	if len(declarations) == 0 {
		return ""
	}
	if bodyClass := staticBodyClassForDeclarations(declarations); bodyClass != "" {
		return bodyClass
	}
	baseName := "class"
	if styleID != "" {
		baseName = "class_" + styleID
	}
	return r.styles.bind(baseName, declarations)
}

func (r *storylineRenderer) containerClass(node map[string]interface{}) string {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = mergeStyleValues(style, r.inferPromotedStyleValues(node))
	if hasRenderableContainer(node) && style["$11"] == nil && style["$42"] != nil &&
		(style["$84"] != nil || style["$89"] != nil || style["$94"] != nil || style["$52"] != nil) {
		style = mergeStyleValues(style, map[string]interface{}{"$11": "default,serif"})
	}
	if len(style) == 0 {
		return ""
	}
	declarations := filterBodyDefaultDeclarations(containerStyleDeclarations(style), r.activeBodyDefaults)
	if mapFontStyle(style["$12"]) == "normal" && bodyDefaultsInclude(r.activeBodyDefaults, "font-style: italic") {
		declarations = append(declarations, "font-style: normal")
	}
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = "class_" + styleID
	}
	return r.styles.bind(baseName, declarations)
}

func (r *storylineRenderer) tableClass(node map[string]interface{}) string {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	declarations := tableStyleDeclarations(style)
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = "class_" + styleID
	}
	return r.styles.bind(baseName, declarations)
}

func (r *storylineRenderer) tableColumnClass(node map[string]interface{}) string {
	style := effectiveStyle(nil, node)
	declarations := tableColumnStyleDeclarations(style)
	if len(declarations) == 0 {
		return ""
	}
	return r.styles.bind("class", declarations)
}

func (r *storylineRenderer) structuredContainerClass(node map[string]interface{}) string {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = mergeStyleValues(style, r.inferPromotedStyleValues(node))
	declarations := structuredContainerDeclarations(style)
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = "class_" + styleID
	}
	return r.styles.bind(baseName, declarations)
}

func (r *storylineRenderer) tableCellClass(node map[string]interface{}) string {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = mergeStyleValues(style, r.inferPromotedStyleValues(node))
	if children, ok := asSlice(node["$146"]); ok && len(children) == 1 {
		if child, ok := asMap(children[0]); ok {
			childStyleID, _ := asString(child["$157"])
			style = mergeStyleValues(style, effectiveStyle(r.styleFragments[childStyleID], child))
		}
	}
	declarations := tableCellStyleDeclarations(style)
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = "class_" + styleID
	}
	return r.styles.bind(baseName, declarations)
}

func (r *storylineRenderer) inlineContainerClass(styleID string, node map[string]interface{}) string {
	style := effectiveStyle(r.styleFragments[styleID], node)
	declarations := spanStyleDeclarations(style)
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = "class_" + styleID
	}
	return r.styles.bind(baseName, declarations)
}

func (r *storylineRenderer) imageClasses(node map[string]interface{}) (string, string) {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	if len(style) == 0 {
		return "", ""
	}
	wrapperDecls := imageWrapperStyleDeclarations(style)
	imageDecls := imageStyleDeclarations(style)
	baseName := "class"
	if styleID != "" {
		baseName = "class_" + styleID
	}
	switch {
	case len(wrapperDecls) > 0 && len(imageDecls) > 0:
		wrapperClass := r.styles.bind(baseName, wrapperDecls)
		imageClass := r.styles.bind(baseName, imageDecls)
		return wrapperClass, imageClass
	case len(wrapperDecls) > 0:
		return r.styles.bind(baseName, wrapperDecls), ""
	case len(imageDecls) > 0:
		return "", r.styles.bind(baseName, imageDecls)
	default:
		return "", ""
	}
}

func (r *storylineRenderer) headingClass(styleID string) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	className := headingClassName(styleID, style)
	declarations := filterBodyDefaultDeclarations(headingStyleDeclarations(style), r.activeBodyDefaults)
	if mapFontStyle(style["$12"]) == "normal" && bodyDefaultsInclude(r.activeBodyDefaults, "font-style: italic") {
		declarations = append(declarations, "font-style: normal")
	}
	if style["$36"] == nil && activeTextIndentNeedsReset(r.activeBodyDefaults) {
		declarations = append(declarations, "text-indent: 0")
	}
	if len(declarations) == 0 {
		return ""
	}
	return r.styles.bind(className, declarations)
}

func (r *storylineRenderer) paragraphClass(styleID string, annotationStyleID string) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	linkStyle := effectiveStyle(r.styleFragments[annotationStyleID], nil)
	declarations := filterBodyDefaultDeclarations(paragraphStyleDeclarations(style, linkStyle), r.activeBodyDefaults)
	declarations = filterDefaultParagraphMargins(declarations)
	if mapFontStyle(style["$12"]) == "normal" && bodyDefaultsInclude(r.activeBodyDefaults, "font-style: italic") {
		declarations = append(declarations, "font-style: normal")
	}
	if style["$36"] == nil && activeTextIndentNeedsReset(r.activeBodyDefaults) {
		declarations = append(declarations, "text-indent: 0")
	}
	if os.Getenv("KFX_DEBUG_PARAGRAPH_STYLE") != "" {
		fmt.Fprintf(os.Stderr, "paragraph style=%s body=%s decls=%v\n", styleID, r.activeBodyClass, declarations)
	}
	className := ""
	if len(declarations) > 0 {
		baseName := "class"
		if styleID != "" {
			baseName = "class_" + styleID
		}
		className = r.styles.bind(baseName, declarations)
	}
	if annotationStyleID != "" {
		_ = r.linkClass(annotationStyleID, true)
	}
	return className
}

func (r *storylineRenderer) linkClass(styleID string, suppressColor bool) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	declarations := linkStyleDeclarations(style, suppressColor)
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = "class_" + styleID
	}
	return r.styles.bind(baseName, declarations)
}

func (r *storylineRenderer) spanClass(styleID string) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	declarations := spanStyleDeclarations(style)
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = "class_" + styleID
	}
	return r.styles.bind(baseName, declarations)
}

func (r *storylineRenderer) resolveText(ref map[string]interface{}) string {
	name, _ := asString(ref["name"])
	index, ok := asInt(ref["$403"])
	if !ok {
		return ""
	}
	values := r.contentFragments[name]
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func hasRenderableContainer(node map[string]interface{}) bool {
	_, hasStyle := asString(node["$157"])
	children, hasChildren := asSlice(node["$146"])
	_, hasImage := asString(node["$175"])
	_, hasText := asMap(node["$145"])
	return hasStyle && !hasImage && !hasText && (!hasChildren || len(children) == 0)
}

func promotedBodyContainer(nodes []interface{}) (string, []interface{}, bool) {
	if len(nodes) != 1 {
		return "", nil, false
	}
	node, ok := asMap(nodes[0])
	if !ok {
		return "", nil, false
	}
	styleID, _ := asString(node["$157"])
	children, ok := asSlice(node["$146"])
	if !ok || len(children) == 0 || styleID == "" {
		return "", nil, false
	}
	if _, ok := asMap(node["$145"]); ok {
		return "", nil, false
	}
	if _, ok := asString(node["$175"]); ok {
		return "", nil, false
	}
	if headingLevel(node) > 0 {
		return "", nil, false
	}
	return styleID, children, true
}

func bodyPromotionPresenceStyle(bodyClass string) map[string]interface{} {
	switch bodyClass {
	case "class-0":
		return map[string]interface{}{"$11": true, "$34": true}
	case "class-1":
		return map[string]interface{}{"$11": true}
	case "class-2":
		return map[string]interface{}{"$11": true, "$34": true, "$36": true}
	case "class-3":
		return map[string]interface{}{"$11": true, "$34": true}
	default:
		return nil
	}
}

func defaultInheritedBodyStyle() map[string]interface{} {
	zero := 0.0
	return map[string]interface{}{
		"$11": "default,serif",
		"$12": "$350",
		"$13": "$350",
		"$36": map[string]interface{}{
			"$306": "$308",
			"$307": &zero,
		},
	}
}

func (r *storylineRenderer) inferBodyStyleValues(nodes []interface{}, parentStyle map[string]interface{}) map[string]interface{} {
	return r.inferSharedHeritableStyle(parentStyle, nodes)
}

func (r *storylineRenderer) inferPromotedBodyStyle(nodes []interface{}) map[string]interface{} {
	return r.inferSharedHeritableStyle(nil, nodes)
}

func (r *storylineRenderer) inferPromotedStyleValues(node map[string]interface{}) map[string]interface{} {
	children, ok := asSlice(node["$146"])
	if !ok || len(children) == 0 {
		return nil
	}
	styleID, _ := asString(node["$157"])
	return r.inferSharedHeritableStyle(effectiveStyle(r.styleFragments[styleID], node), children)
}

func (r *storylineRenderer) inferSharedHeritableStyle(parentStyle map[string]interface{}, nodes []interface{}) map[string]interface{} {
	if len(nodes) == 0 {
		return nil
	}
	type valueCount struct {
		count int
		raw   interface{}
	}
	const reverseInheritanceFraction = 0.8
	keys := []string{"$11", "$12", "$13", "$34", "$36", "$41", "$583"}
	valueCounts := map[string]map[string]*valueCount{}
	numChildren := 0
	debugInfer := os.Getenv("KFX_DEBUG_INFER_COUNTS") != ""
	debugStyleIDs := make([]string, 0, len(nodes))
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		styleID, _ := asString(node["$157"])
		if debugInfer {
			debugStyleIDs = append(debugStyleIDs, styleID)
		}
		style := effectiveStyle(r.styleFragments[styleID], node)
		if childPromoted := r.inferPromotedStyleValues(node); len(childPromoted) > 0 {
			style = mergeStyleValues(style, childPromoted)
		}
		numChildren++
		if len(style) == 0 {
			continue
		}
		for _, key := range keys {
			rawValue, ok := style[key]
			if !ok {
				continue
			}
			valueKey := fmt.Sprintf("%#v", rawValue)
			if valueCounts[key] == nil {
				valueCounts[key] = map[string]*valueCount{}
			}
			entry := valueCounts[key][valueKey]
			if entry == nil {
				entry = &valueCount{raw: rawValue}
				valueCounts[key][valueKey] = entry
			}
			entry.count++
		}
	}
	if numChildren == 0 {
		return nil
	}
	values := map[string]interface{}{}
	for _, key := range keys {
		counts := valueCounts[key]
		if len(counts) == 0 {
			continue
		}
		var (
			bestKey   string
			bestValue interface{}
			bestCount int
			total     int
		)
		for valueKey, entry := range counts {
			total += entry.count
			if entry.count > bestCount {
				bestKey = valueKey
				bestValue = entry.raw
				bestCount = entry.count
			}
		}
		if bestKey == "" {
			continue
		}
		if total < numChildren && (parentStyle == nil || parentStyle[key] == nil) {
			continue
		}
		if float64(bestCount) >= float64(numChildren)*reverseInheritanceFraction {
			values[key] = bestValue
		}
	}
	if len(values) == 0 {
		if debugInfer {
			fmt.Fprintf(os.Stderr, "infer none numChildren=%d styles=%v counts=", numChildren, debugStyleIDs)
			for _, key := range keys {
				if len(valueCounts[key]) == 0 {
					continue
				}
				fmt.Fprintf(os.Stderr, " %s:{", key)
				first := true
				for valueKey, entry := range valueCounts[key] {
					if !first {
						fmt.Fprint(os.Stderr, ", ")
					}
					first = false
					fmt.Fprintf(os.Stderr, "%s=%d", valueKey, entry.count)
				}
				fmt.Fprint(os.Stderr, "}")
			}
			fmt.Fprintln(os.Stderr)
		}
		return nil
	}
	if debugInfer {
		fmt.Fprintf(os.Stderr, "infer values numChildren=%d styles=%v values=%#v\n", numChildren, debugStyleIDs, values)
	}
	return values
}

func headingLevel(node map[string]interface{}) int {
	value, ok := node["$790"]
	if !ok {
		return 0
	}
	level, _ := asInt(value)
	return level
}

func fullParagraphAnnotationStyleID(node map[string]interface{}, text string) string {
	annotations, ok := asSlice(node["$142"])
	if !ok || len(annotations) == 0 {
		return ""
	}
	runeCount := len([]rune(text))
	for _, raw := range annotations {
		annotationMap, ok := asMap(raw)
		if !ok || !annotationCoversWholeText(annotationMap, runeCount) {
			continue
		}
		styleID, _ := asString(annotationMap["$157"])
		return styleID
	}
	return ""
}

func annotationCoversWholeText(annotationMap map[string]interface{}, runeCount int) bool {
	if annotationMap == nil || runeCount == 0 {
		return false
	}
	start, hasStart := asInt(annotationMap["$143"])
	length, hasLength := asInt(annotationMap["$144"])
	_, hasAnchor := asString(annotationMap["$179"])
	return hasAnchor && hasStart && hasLength && start == 0 && length >= runeCount
}

func storylineUsesJustifiedBody(nodes []interface{}) bool {
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		styleID, _ := asString(node["$157"])
		if styleID == "s6E" || styleID == "s6G" {
			return true
		}
		if children, ok := asSlice(node["$146"]); ok && storylineUsesJustifiedBody(children) {
			return true
		}
	}
	return false
}

func estimateBodyClass(nodes []interface{}) string {
	if storylineUsesJustifiedBody(nodes) {
		return "class-2"
	}
	if storylineIsCentered(nodes) {
		return "class-0"
	}
	return "class-1"
}

func storylineIsCentered(nodes []interface{}) bool {
	return !storylineContainsParagraph(nodes)
}

func storylineContainsParagraph(nodes []interface{}) bool {
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		if _, ok := asMap(node["$145"]); ok && headingLevel(node) == 0 {
			return true
		}
		if children, ok := asSlice(node["$146"]); ok && storylineContainsParagraph(children) {
			return true
		}
	}
	return false
}

func headingClassName(styleID string, style map[string]interface{}) string {
	return "heading_" + styleID
}

func filterBodyDefaultDeclarations(declarations []string, bodyDefaults map[string]bool) []string {
	if len(declarations) == 0 {
		return declarations
	}
	filtered := make([]string, 0, len(declarations))
	for _, declaration := range declarations {
		if bodyDefaults != nil && bodyDefaults[declaration] {
			continue
		}
		filtered = append(filtered, declaration)
	}
	return filtered
}

func filterDefaultParagraphMargins(declarations []string) []string {
	if len(declarations) == 0 {
		return declarations
	}
	filtered := make([]string, 0, len(declarations))
	for _, declaration := range declarations {
		if declaration == "margin-top: 1em" || declaration == "margin-bottom: 1em" {
			continue
		}
		filtered = append(filtered, declaration)
	}
	return filtered
}

func activeTextIndentNeedsReset(bodyDefaults map[string]bool) bool {
	if len(bodyDefaults) == 0 {
		return false
	}
	for declaration := range bodyDefaults {
		if strings.HasPrefix(declaration, "text-indent: ") {
			return declaration != "text-indent: 0"
		}
	}
	return false
}

func bodyDefaultsInclude(bodyDefaults map[string]bool, declaration string) bool {
	if len(bodyDefaults) == 0 {
		return false
	}
	return bodyDefaults[declaration]
}

func newFontNameFixer() *fontNameFixer {
	return &fontNameFixer{
		fixedNames:       map[string]string{},
		nameReplacements: map[string]string{},
	}
}

func (f *fontNameFixer) fixAndQuoteFontFamilyList(value string) string {
	families := f.splitAndFixFontFamilyList(value)
	if len(families) == 0 {
		return ""
	}
	seen := map[string]bool{}
	quoted := make([]string, 0, len(families))
	for _, family := range families {
		key := strings.ToLower(family)
		if seen[key] {
			continue
		}
		seen[key] = true
		quoted = append(quoted, quoteFontName(family))
	}
	return strings.Join(quoted, ",")
}

func (f *fontNameFixer) splitAndFixFontFamilyList(value string) []string {
	parts := strings.Split(value, ",")
	families := make([]string, 0, len(parts))
	for _, part := range parts {
		if family := f.fixFontName(part, false, false); family != "" {
			families = append(families, family)
		}
	}
	return families
}

func stripFontName(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > 0 && (name[0] == '\'' || name[0] == '"') {
		name = name[1:]
	}
	if len(name) > 0 && (name[len(name)-1] == '\'' || name[len(name)-1] == '"') {
		name = name[:len(name)-1]
	}
	return strings.TrimSpace(name)
}

func (f *fontNameFixer) fixFontName(name string, add bool, generic bool) string {
	name = stripFontName(name)
	if name == "" {
		return ""
	}
	origName := strings.ToLower(name)
	if fixed, ok := f.fixedNames[origName]; ok {
		return fixed
	}
	name = strings.ReplaceAll(name, `\`, "")
	lower := strings.ToLower(name)
	replacements := map[string]string{
		"san-serif": "sans-serif",
		"ariel":     "Arial",
	}
	if replacement, ok := replacements[lower]; ok {
		name = replacement
		lower = strings.ToLower(name)
	}
	for _, suffix := range []string{"-oblique", "-italic", "-bold", "-regular", "-roman", "-medium"} {
		if strings.HasSuffix(lower, suffix) {
			name = name[:len(name)-len(suffix)] + " " + strings.TrimPrefix(suffix, "-")
			break
		}
	}
	hasPrefix := strings.Contains(name, "-") && name != "sans-serif"
	if hasPrefix {
		name = strings.ReplaceAll(name, "sans-serif", "sans_serif")
		name = name[strings.LastIndex(name, "-")+1:]
		name = strings.ReplaceAll(name, "sans_serif", "sans-serif")
	}
	name = strings.TrimSpace(name)
	if add {
		key := strings.ToLower(name)
		if hasPrefix {
			key = "?-" + key
		}
		if replacement, ok := f.nameReplacements[key]; ok {
			name = replacement
		} else {
			f.nameReplacements[key] = name
		}
	} else {
		if replacement, ok := f.nameReplacements[strings.ToLower(name)]; ok {
			name = replacement
		} else if cssGenericFontNames[strings.ToLower(name)] {
			name = strings.ToLower(name)
		} else {
			name = capitalizeFontName(name)
		}
	}
	f.fixedNames[origName] = name
	return name
}

func capitalizeFontName(name string) string {
	words := strings.Fields(name)
	for i, word := range words {
		if len(word) > 2 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		} else {
			words[i] = strings.ToUpper(word)
		}
	}
	return strings.Join(words, " ")
}

func quoteFontName(value string) string {
	for _, ident := range strings.Split(value, " ") {
		if ident == "" {
			break
		}
		first := ident[0]
		if (first >= '0' && first <= '9') || (len(ident) >= 2 && ident[:2] == "--") || !cssIdentPattern.MatchString(ident) {
			return quoteCSSString(value)
		}
		if first == '-' && len(ident) > 1 && ident[1] >= '0' && ident[1] <= '9' {
			return quoteCSSString(value)
		}
	}
	return value
}

func finalizeStylesheet(stylesheet string) string {
	stylesheet = strings.TrimSpace(stylesheet)
	if stylesheet == "" {
		return ""
	}
	lines := strings.Split(stylesheet, "\n")
	fontFaces := make([]string, 0, len(lines))
	rules := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "@charset ") {
			continue
		}
		if strings.HasPrefix(line, "@font-face ") {
			fontFaces = append(fontFaces, line)
			continue
		}
		rules = append(rules, line)
	}
	sort.Strings(fontFaces)
	sort.SliceStable(rules, func(i, j int) bool {
		selectorI := rules[i]
		if idx := strings.Index(selectorI, " {"); idx >= 0 {
			selectorI = selectorI[:idx]
		}
		selectorJ := rules[j]
		if idx := strings.Index(selectorJ, " {"); idx >= 0 {
			selectorJ = selectorJ[:idx]
		}
		return naturalSortKey(selectorI) < naturalSortKey(selectorJ)
	})
	out := make([]string, 0, 1+len(fontFaces)+len(rules))
	out = append(out, `@charset "UTF-8";`)
	out = append(out, fontFaces...)
	out = append(out, rules...)
	return strings.Join(out, "\n")
}

func collectReferencedClasses(book *decodedBook) map[string]bool {
	used := map[string]bool{}
	if book == nil {
		return used
	}
	addClasses := func(value string) {
		for _, className := range strings.Fields(strings.TrimSpace(value)) {
			if className != "" {
				used[className] = true
			}
		}
	}
	for _, section := range book.Sections {
		addClasses(section.BodyClass)
		for _, match := range regexp.MustCompile(`class="([^"]+)"`).FindAllStringSubmatch(section.BodyHTML, -1) {
			if len(match) > 1 {
				addClasses(match[1])
			}
		}
	}
	return used
}

func pruneUnusedStylesheetRules(stylesheet string, used map[string]bool) string {
	if stylesheet == "" || len(used) == 0 {
		return stylesheet
	}
	lines := strings.Split(stylesheet, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ".") {
			selector := trimmed[1:]
			if idx := strings.Index(selector, " {"); idx >= 0 {
				selector = selector[:idx]
			}
			if selector != "" && !used[selector] {
				continue
			}
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func parseDeclarationString(value string) map[string]string {
	style := map[string]string{}
	for _, declaration := range strings.Split(value, ";") {
		declaration = strings.TrimSpace(declaration)
		if declaration == "" {
			continue
		}
		name, rawValue, ok := strings.Cut(declaration, ":")
		if !ok {
			continue
		}
		style[strings.TrimSpace(name)] = strings.TrimSpace(rawValue)
	}
	return style
}

func declarationListFromStyleMap(style map[string]string) []string {
	if len(style) == 0 {
		return nil
	}
	declarations := make([]string, 0, len(style))
	for name, value := range style {
		if strings.TrimSpace(value) == "" {
			continue
		}
		declarations = append(declarations, name+": "+value)
	}
	return canonicalDeclarations(declarations)
}

func canonicalDeclarations(declarations []string) []string {
	if len(declarations) == 0 {
		return declarations
	}
	out := make([]string, 0, len(declarations))
	seen := map[string]bool{}
	for _, declaration := range declarations {
		declaration = strings.TrimSpace(declaration)
		if declaration == "" || seen[declaration] {
			continue
		}
		seen[declaration] = true
		out = append(out, declaration)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ni := out[i]
		nj := out[j]
		pi := ni
		if idx := strings.IndexByte(ni, ':'); idx >= 0 {
			pi = ni[:idx]
		}
		pj := nj
		if idx := strings.IndexByte(nj, ':'); idx >= 0 {
			pj = nj[:idx]
		}
		if pi == pj {
			return ni < nj
		}
		return pi < pj
	})
	return out
}

func quoteCSSString(value string) string {
	if !strings.Contains(value, "'") && !strings.Contains(value, `\`) {
		return "'" + value + "'"
	}
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}

func (r *storylineRenderer) applyAnnotations(text string, node map[string]interface{}) []htmlPart {
	annotations, ok := asSlice(node["$142"])
	type event struct {
		start int
		end   int
		open  func() *htmlElement
	}
	runes := []rune(text)
	events := make([]event, 0, len(annotations))
	if ok {
		for _, raw := range annotations {
			annotationMap, ok := asMap(raw)
			if !ok {
				continue
			}
			start, hasStart := asInt(annotationMap["$143"])
			length, hasLength := asInt(annotationMap["$144"])
			if !hasStart || !hasLength || length <= 0 || start < 0 || start >= len(runes) {
				continue
			}
			end := start + length
			if end > len(runes) {
				end = len(runes)
			}
			anchorID, _ := asString(annotationMap["$179"])
			styleID, _ := asString(annotationMap["$157"])
			if debugAnchors := os.Getenv("KFX_DEBUG_ANCHORS"); debugAnchors != "" && anchorID != "" {
				for _, wanted := range strings.Split(debugAnchors, ",") {
					if strings.TrimSpace(wanted) == anchorID {
						fmt.Fprintf(os.Stderr, "annotation anchor=%s style=%s value=%#v\n", anchorID, styleID, annotationMap)
					}
				}
			}
			href := r.anchorToFilename[anchorID]
			if href == "" && anchorID != "" {
				href = anchorID
			}
			if href != "" {
				className := r.linkClass(styleID, annotationCoversWholeText(annotationMap, len(runes)))
				events = append(events, event{
					start: start,
					end:   end,
					open: func() *htmlElement {
						attrs := map[string]string{"href": href}
						if className != "" {
							attrs["class"] = className
						}
						return &htmlElement{Tag: "a", Attrs: attrs}
					},
				})
				continue
			}
			if className := r.spanClass(styleID); className != "" {
				events = append(events, event{
					start: start,
					end:   end,
					open: func() *htmlElement {
						return &htmlElement{Tag: "span", Attrs: map[string]string{"class": className}}
					},
				})
			}
		}
	}
	if len(events) == 0 {
		return splitTextHTMLParts(text)
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].start == events[j].start {
			return events[i].end > events[j].end
		}
		return events[i].start < events[j].start
	})
	root := &htmlElement{Attrs: map[string]string{}}
	stack := []*htmlElement{root}
	last := 0
	for index, rch := range runes {
		if last < index {
			appendTextHTMLParts(stack[len(stack)-1], string(runes[last:index]))
			last = index
		}
		for _, ev := range events {
			if ev.start == index {
				element := ev.open()
				stack[len(stack)-1].Children = append(stack[len(stack)-1].Children, element)
				stack = append(stack, element)
			}
		}
		appendTextHTMLParts(stack[len(stack)-1], string(rch))
		last = index + 1
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].end == index+1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if last < len(runes) {
		appendTextHTMLParts(stack[len(stack)-1], string(runes[last:]))
	}
	return root.Children
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
	element.Children = append(element.Children, splitTextHTMLParts(text)...)
}

func (r *storylineRenderer) anchorIDForPosition(positionID int, offset int) string {
	offsets := r.positionAnchorID[positionID]
	if offsets == nil {
		return ""
	}
	return offsets[offset]
}

func (r *storylineRenderer) anchorOnlyMovable(positionID int, offset int) bool {
	offsets := r.positionAnchors[positionID]
	if offsets == nil {
		return false
	}
	names := offsets[offset]
	if len(names) == 0 {
		return false
	}
	for _, name := range names {
		if strings.HasPrefix(name, "$798_") {
			return false
		}
	}
	return true
}

func (r *storylineRenderer) applyPositionAnchors(element *htmlElement, positionID int, isFirstVisible bool) {
	if element == nil || positionID == 0 {
		return
	}
	if os.Getenv("KFX_DEBUG") != "" && (positionID == 1110 || positionID == 1111 || positionID == 1177 || positionID == 1178) {
		fmt.Fprintf(os.Stderr, "apply anchors pos=%d tag=%s first=%v raw=%v ids=%v\n", positionID, element.Tag, isFirstVisible, r.positionAnchors[positionID], r.positionAnchorID[positionID])
	}
	offsets := r.positionAnchors[positionID]
	if len(offsets) == 0 {
		return
	}
	if anchorID := r.anchorIDForPosition(positionID, 0); anchorID != "" {
		if !isFirstVisible && !strings.HasPrefix(anchorID, "id__212_") {
			element.Attrs["id"] = anchorID
			r.emittedAnchorIDs[anchorID] = true
			if os.Getenv("KFX_DEBUG") != "" && (positionID == 1110 || positionID == 1111 || positionID == 1177 || positionID == 1178 || positionID == 1007 || positionID == 1053) {
				fmt.Fprintf(os.Stderr, "set id pos=%d tag=%s id=%s class=%s\n", positionID, element.Tag, anchorID, element.Attrs["class"])
			}
		}
	}
	ordered := make([]int, 0, len(offsets))
	for offset := range offsets {
		if offset > 0 {
			ordered = append(ordered, offset)
		}
	}
	sort.Ints(ordered)
	for _, offset := range ordered {
		anchorID := r.anchorIDForPosition(positionID, offset)
		if anchorID == "" {
			continue
		}
		target := locateOffset(element, offset)
		if target == nil {
			continue
		}
		if target.Attrs == nil {
			target.Attrs = map[string]string{}
		}
		if target.Attrs["id"] == "" {
			target.Attrs["id"] = anchorID
			r.emittedAnchorIDs[anchorID] = true
		}
	}
}

func (r *storylineRenderer) consumeVisibleElement() bool {
	isFirst := !r.firstVisibleSeen
	r.firstVisibleSeen = true
	return isFirst
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
	case *htmlElement:
		return renderHTMLElement(typed)
	default:
		return ""
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

func applyCoverSVGPromotion(book *decodedBook) {
	if book == nil || book.CoverImageHref == "" {
		return
	}
	width, height := coverImageDimensions(book.Resources, book.CoverImageHref)
	if width == 0 || height == 0 {
		return
	}
	for index := range book.Sections {
		section := &book.Sections[index]
		if section.Title != "Cover" || !strings.Contains(section.BodyHTML, `src="`+book.CoverImageHref+`"`) {
			continue
		}
		section.BodyClass = "class_s8"
		section.Properties = "svg"
		section.BodyHTML = fmt.Sprintf(
			`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" version="1.1" preserveAspectRatio="xMidYMid meet" viewBox="0 0 %d %d" height="100%%" width="100%%"><image xlink:href="%s" height="%d" width="%d"/></svg>`,
			width, height, escapeHTML(book.CoverImageHref), height, width,
		)
		break
	}
	if !strings.Contains(book.Stylesheet, ".class_s8 {") {
		if book.Stylesheet != "" {
			book.Stylesheet += "\n"
		}
		book.Stylesheet += ".class_s8 {font-family: FreeFontSerif,serif}"
	} else {
		lines := strings.Split(book.Stylesheet, "\n")
		for index, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), ".class_s8 {") {
				lines[index] = ".class_s8 {font-family: FreeFontSerif,serif}"
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
		if strings.EqualFold(trimmed, "en") {
			return "en-US"
		}
		return trimmed
	}
	if len(suffix) < 4 {
		suffix = strings.ToUpper(suffix)
	} else {
		suffix = strings.ToUpper(suffix[:1]) + suffix[1:]
	}
	return prefix + "-" + suffix
}
