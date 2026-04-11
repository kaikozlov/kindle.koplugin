package kfx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/jpeg"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/kaikozlov/kindle-koplugin/internal/epub"
)

var (
	contSignature    = []byte("CONT")
	drmionSignature  = []byte{0xea, 'D', 'R', 'M', 'I', 'O', 'N', 0xee}
	ionVersionMarker = []byte{0xe0, 0x01, 0x00, 0xea}
	yjPreludeOnce    sync.Once
	yjPreludeData    []byte
	yjPreludeErr     error
	cssIdentPattern  = regexp.MustCompile(`^[-_a-zA-Z0-9]*$`)
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
	Title                    string
	Language                 string
	Authors                  []string
	Identifier               string
	Published                string
	Description              string
	Publisher                string
	BookID                   string
	ASIN                     string
	OrientationLock          string
	WritingMode              string
	PageProgressionDirection string
	FixedLayout              bool
	IllustratedLayout        bool
	OverrideKindleFonts      bool
	CoverImageID             string
	CoverImageHref           string
	Stylesheet               string
	ResourceHrefByID         map[string]string
	RenderedSections         []renderedSection
	Sections                 []epub.Section
	Resources                []epub.Resource
	Navigation               []epub.NavPoint
	Guide                    []epub.GuideEntry
	PageList                 []epub.PageTarget
}

type renderedStoryline struct {
	Root       *htmlElement
	BodyHTML   string
	BodyClass  string
	BodyStyle  string
	Properties string
}

type renderedSection struct {
	Filename   string
	Title      string
	PageTitle  string
	Language   string
	BodyClass  string
	BodyStyle  string
	Paragraphs []string
	Properties string
	Root       *htmlElement
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
	PageTemplates      []pageTemplateFragment
}

type pageTemplateFragment struct {
	PositionID         int
	Storyline          string
	PageTemplateStyle  string
	PageTemplateValues map[string]interface{}
	HasCondition       bool
	Condition          interface{}
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
	contentFragments    map[string][]string
	rubyGroups          map[string]map[string]interface{}
	rubyContents        map[string]map[string]interface{}
	resourceHrefByID    map[string]string
	resourceFragments   map[string]resourceFragment
	anchorToFilename    map[string]string
	directAnchorURI     map[string]string
	fallbackAnchorURI   map[string]string
	positionToSection   map[int]string
	positionAnchors     map[int]map[int][]string
	positionAnchorID    map[int]map[int]string
	anchorNamesByID     map[string][]string
	anchorHeadingLevel  map[string]int
	emittedAnchorIDs    map[string]bool
	styleFragments      map[string]map[string]interface{}
	styles              *styleCatalog
	activeBodyClass     string
	activeBodyDefaults  map[string]bool
	firstVisibleSeen    bool
	lastKFXHeadingLevel int
	symFmt              symType
	conditionEvaluator  conditionEvaluator
}

type conditionEvaluator struct {
	orientationLock   string
	fixedLayout       bool
	illustratedLayout bool
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
	blobs, hasDRM, err := collectContainerBlobs(path)
	if err != nil {
		return "", "", err
	}
	if hasDRM {
		return "blocked", "drm", nil
	}
	if len(blobs) == 0 {
		return "blocked", "unsupported_kfx_layout", nil
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
		Description:         book.Description,
		Publisher:           book.Publisher,
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
	state, err := buildBookState(path)
	if err != nil {
		return nil, err
	}
	if os.Getenv("KFX_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "docSymbols length=%d first=% x\n", len(state.Source.DocSymbols), state.Source.DocSymbols[:minInt(16, len(state.Source.DocSymbols))])
	}
	return renderBookState(state)
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

// isSharedSymbolText mirrors symtab.is_shared_symbol for text form "$<sid>" (yj_structure.py / Ion locals).
func (r *symbolResolver) isSharedSymbolText(name string) bool {
	if r == nil || name == "" || name[0] != '$' {
		return false
	}
	for _, ch := range name[1:] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	sid64, err := strconv.ParseUint(name[1:], 10, 32)
	if err != nil || sid64 == 0 {
		return false
	}
	sid := uint32(sid64)
	if r.isLocalSID(sid) {
		return false
	}
	return sid < r.localStart
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

// Port of Python process_reading_order reading order iteration (yj_to_epub_content.py L105+).
// Python iterates all reading orders; Go merges all section lists.
func readSectionOrder(value map[string]interface{}) []string {
	entries, ok := asSlice(value["$169"])
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var result []string
	for _, entry := range entries {
		entryMap, ok := asMap(entry)
		if !ok {
			continue
		}
		sections, ok := asSlice(entryMap["$170"])
		if !ok {
			continue
		}
		for _, item := range sections {
			if text, ok := asString(item); ok && text != "" && !seen[text] {
				seen[text] = true
				result = append(result, text)
			}
		}
	}
	return result
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
	templates := make([]pageTemplateFragment, 0, len(containers))
	for _, raw := range containers {
		container, ok := asMap(raw)
		if !ok {
			continue
		}
		storylineID, _ := asString(container["$176"])
		pageTemplateStyle, _ := asString(container["$157"])
		positionID, _ := asInt(container["$155"])
		templates = append(templates, pageTemplateFragment{
			PositionID:         positionID,
			Storyline:          storylineID,
			PageTemplateStyle:  pageTemplateStyle,
			PageTemplateValues: filterBodyStyleValues(container),
			HasCondition:       container["$171"] != nil,
			Condition:          container["$171"],
		})
	}
	if len(templates) == 0 {
		return sectionFragment{ID: id}
	}
	mainTemplate := templates[len(templates)-1]
	return sectionFragment{
		ID:                 id,
		PositionID:         mainTemplate.PositionID,
		Storyline:          mainTemplate.Storyline,
		PageTemplateStyle:  mainTemplate.PageTemplateStyle,
		PageTemplateValues: mainTemplate.PageTemplateValues,
		PageTemplates:      templates,
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

// defaultBodyFontDeclarations returns additional font-family declarations for the body that
// should be used for inheritance filtering. Not needed since defaultBodyDeclarations already
// includes font-family.
func defaultBodyFontDeclarations(bodyClass string) []string {
	return nil
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
	} else {
		declarations = append(declarations, "font-weight: normal")
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

// defaultFontNames mirrors Python DEFAULT_FONT_NAMES (yj_to_epub_properties.py).
var defaultFontNames = map[string]bool{
	"default":                   true,
	"$amzn_fixup_default_font$": true,
}

func cssFontFamily(value interface{}) string {
	text, ok := asString(value)
	if !ok || text == "" {
		return ""
	}
	if text == "default,serif" {
		return "FreeFontSerif,serif"
	}
	// Filter out default font names (Python simplify_styles strips these).
	parts := strings.Split(text, ",")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		trimmed = strings.Trim(trimmed, "'\"")
		if !defaultFontNames[strings.ToLower(trimmed)] {
			filtered = append(filtered, part)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	text = strings.Join(filtered, ",")
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

func asStringDefault(value interface{}) string {
	result, _ := asString(value)
	return result
}

func intPtr(value int) *int {
	return &value
}

func sectionFilename(sectionID string, format symType) string {
	u := uniquePartOfLocalSymbol(sectionID, format)
	if u == "" {
		u = sectionID
	}
	return u + ".xhtml"
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
	r.activeBodyDefaults = nil
	r.firstVisibleSeen = false
	r.lastKFXHeadingLevel = 1
	if bodyStyleID == "" {
		bodyStyleID, _ = asString(storyline["$157"])
	}
	bodyStyle := effectiveStyle(r.styleFragments[bodyStyleID], bodyStyleValues)
	bodyDeclarations := cssDeclarationsFromMap(processContentProperties(bodyStyle))
	if bodyStyleID == "" && len(bodyDeclarations) == 0 {
		bodyStyleValues = map[string]interface{}{
			"$11": defaultInheritedBodyStyle()["$11"],
		}
		bodyStyle = effectiveStyle(r.styleFragments[bodyStyleID], bodyStyleValues)
		bodyDeclarations = cssDeclarationsFromMap(processContentProperties(bodyStyle))
	}
	if len(bodyDeclarations) > 0 {
		baseName := "class"
		if bodyStyleID != "" {
			baseName = r.styleBaseName(bodyStyleID)
		}
		result.BodyStyle = styleStringFromDeclarations(baseName, nil, bodyDeclarations)
	}
	if os.Getenv("KFX_DEBUG_BODY") != "" {
		fmt.Fprintf(os.Stderr, "body resolved styleID=%s decls=%v style=%s\n", bodyStyleID, bodyDeclarations, result.BodyStyle)
	}
	if len(bodyDeclarations) > 0 {
		r.activeBodyDefaults = inheritedDefaultSet(bodyDeclarations)
	}
	bodyParts := make([]htmlPart, 0, len(contentNodes))
	for _, node := range contentNodes {
		rendered := r.renderNode(node, 0)
		if rendered != nil {
			bodyParts = append(bodyParts, rendered)
		}
	}
	root := &htmlElement{Attrs: map[string]string{}, Children: bodyParts}
	normalizeHTMLWhitespace(root)
	r.applyPositionAnchors(root, sectionPositionID, false)
	result.Root = root
	result.BodyHTML = renderHTMLParts(root.Children, true)
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

// styleBaseName returns a simplified class base name from a style ID, applying
// uniquePartOfLocalSymbol to strip the symbol-format prefix (ORIGINAL: V_N_N-PARA-…, etc.)
// matching Calibre's simplify_styles class naming behavior.
func (r *storylineRenderer) styleBaseName(styleID string) string {
	if styleID == "" {
		return "class"
	}
	simplified := uniquePartOfLocalSymbol(styleID, r.symFmt)
	if simplified == "" {
		return "class"
	}
	return "class_" + simplified
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

func (r *storylineRenderer) setDynamicStyle(element *htmlElement, baseName string, layoutHints []string, declarations []string) {
	if element == nil {
		return
	}
	setElementStyleString(element, mergeStyleStrings(element.Attrs["style"], styleStringFromDeclarations(baseName, layoutHints, declarations)))
}

func (r *storylineRenderer) renderNode(raw interface{}, depth int) htmlPart {
	node, ok := asMap(raw)
	if !ok {
		return nil
	}
	node, ok = r.prepareRenderableNode(node)
	if !ok {
		return nil
	}
	switch asStringDefault(node["$159"]) {
	case "$276":
		if list := r.renderListNode(node, depth); list != nil {
			return r.wrapNodeLink(node, list)
		}
	case "$277":
		if item := r.renderListItemNode(node, depth); item != nil {
			return r.wrapNodeLink(node, item)
		}
	case "$596":
		if rule := r.renderRuleNode(node); rule != nil {
			return r.wrapNodeLink(node, rule)
		}
	case "$439":
		if hidden := r.renderHiddenNode(node, depth); hidden != nil {
			return r.wrapNodeLink(node, hidden)
		}
	case "$278":
		if table := r.renderTableNode(node, depth); table != nil {
			return r.wrapNodeLink(node, table)
		}
	case "$270":
		if container := r.renderFittedContainer(node, depth); container != nil {
			return r.wrapNodeLink(node, container)
		}
	case "$272":
		if svg := r.renderSVGNode(node); svg != nil {
			return r.wrapNodeLink(node, svg)
		}
	case "$274":
		if plugin := r.renderPluginNode(node); plugin != nil {
			return r.wrapNodeLink(node, plugin)
		}
	case "$454":
		if tbody := r.renderStructuredContainer(node, "tbody", depth); tbody != nil {
			return r.wrapNodeLink(node, tbody)
		}
	case "$151":
		if thead := r.renderStructuredContainer(node, "thead", depth); thead != nil {
			return r.wrapNodeLink(node, thead)
		}
	case "$455":
		if tfoot := r.renderStructuredContainer(node, "tfoot", depth); tfoot != nil {
			return r.wrapNodeLink(node, tfoot)
		}
	case "$279":
		if row := r.renderTableRow(node, depth); row != nil {
			return r.wrapNodeLink(node, row)
		}
	}

	if imageNode := r.renderImageNode(node); imageNode != nil {
		return r.wrapNodeLink(node, imageNode)
	}

	if textNode := r.renderTextNode(node, depth); textNode != nil {
		return r.wrapNodeLink(node, textNode)
	}

	children, ok := asSlice(node["$146"])
	if !ok {
		if hasRenderableContainer(node) {
			element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
			if styleAttr := r.containerClass(node); styleAttr != "" {
				element.Attrs["style"] = styleAttr
			}
			r.applyStructuralNodeAttrs(element, node, "")
			return r.wrapNodeLink(node, element)
		}
		return nil
	}

	if inline := r.renderInlineRenderContainer(node, children, depth); inline != nil {
		return r.wrapNodeLink(node, inline)
	}
	if headingTag := r.layoutHintHeadingTag(node, children); headingTag != "" {
		element := &htmlElement{Tag: headingTag, Attrs: map[string]string{}}
		for _, child := range children {
			if inline := r.renderInlinePart(child, depth+1); inline != nil {
				element.Children = append(element.Children, inline)
			}
		}
		if len(element.Children) > 0 {
			if styleAttr := r.containerClass(node); styleAttr != "" {
				element.Attrs["style"] = styleAttr
			}
			r.applyStructuralNodeAttrs(element, node, "")
			if positionID, _ := asInt(node["$155"]); positionID != 0 {
				r.applyPositionAnchors(element, positionID, false)
			}
			return r.wrapNodeLink(node, element)
		}
	}
	if figure := r.renderFigureHintContainer(node, children, depth); figure != nil {
		return r.wrapNodeLink(node, figure)
	}
	if paragraph := r.renderInlineParagraphContainer(node, children, depth); paragraph != nil {
		return r.wrapNodeLink(node, paragraph)
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
	if styleAttr := r.containerClass(node); styleAttr != "" {
		container.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(container, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(container, positionID, false)
	}
	return r.wrapNodeLink(node, container)
}

func (r *storylineRenderer) renderListNode(node map[string]interface{}, depth int) htmlPart {
	tag := listTagByMarker[asStringDefault(node["$100"])]
	if tag == "" {
		tag = "ul"
	}
	list := &htmlElement{Tag: tag, Attrs: map[string]string{}}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		list.Attrs["style"] = styleAttr
	}
	if start, ok := asInt(node["$104"]); ok && start > 0 && tag == "ol" && start != 1 {
		list.Attrs["start"] = strconv.Itoa(start)
	}
	children, _ := asSlice(node["$146"])
	for _, child := range children {
		if rendered := r.renderNode(child, depth+1); rendered != nil {
			list.Children = append(list.Children, rendered)
		}
	}
	if len(list.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(list, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(list, positionID, false)
	}
	return list
}

func (r *storylineRenderer) renderListItemNode(node map[string]interface{}, depth int) htmlPart {
	item := &htmlElement{Tag: "li", Attrs: map[string]string{}}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		item.Attrs["style"] = styleAttr
	}
	if value, ok := asInt(node["$104"]); ok && value > 0 {
		item.Attrs["value"] = strconv.Itoa(value)
	}
	if ref, ok := asMap(node["$145"]); ok {
		text := r.resolveText(ref)
		if text != "" {
			item.Children = append(item.Children, r.applyAnnotations(text, node)...)
		}
	} else if children, ok := asSlice(node["$146"]); ok {
		for _, child := range children {
			if rendered := r.renderNode(child, depth+1); rendered != nil {
				item.Children = append(item.Children, rendered)
				continue
			}
			if inline := r.renderInlinePart(child, depth+1); inline != nil {
				item.Children = append(item.Children, inline)
			}
		}
	}
	if len(item.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(item, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(item, positionID, false)
	}
	return item
}

func (r *storylineRenderer) renderRuleNode(node map[string]interface{}) htmlPart {
	rule := &htmlElement{Tag: "hr", Attrs: map[string]string{}}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		rule.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(rule, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(rule, positionID, false)
	}
	return rule
}

func (r *storylineRenderer) renderHiddenNode(node map[string]interface{}, depth int) htmlPart {
	element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		element.Attrs["style"] = styleAttr
	}
	if hiddenStyle := styleStringFromDeclarations("class", nil, []string{"display: none"}); hiddenStyle != "" {
		element.Attrs["style"] = mergeStyleStrings(element.Attrs["style"], hiddenStyle)
	}
	if children, ok := asSlice(node["$146"]); ok {
		for _, child := range children {
			if rendered := r.renderNode(child, depth+1); rendered != nil {
				element.Children = append(element.Children, rendered)
				continue
			}
			if inline := r.renderInlinePart(child, depth+1); inline != nil {
				element.Children = append(element.Children, inline)
			}
		}
	}
	if len(element.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderFittedContainer(node map[string]interface{}, depth int) htmlPart {
	fitWidth, _ := asBool(node["$478"])
	if !fitWidth {
		return nil
	}
	outer := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		outer.Attrs["style"] = styleAttr
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
		baseName = r.styleBaseName(styleID)
	}
	if styleAttr := styleStringFromDeclarations(baseName, nil, []string{"display: inline-block"}); styleAttr != "" {
		inner.Attrs["style"] = styleAttr
	}
	outer.Children = []htmlPart{inner}
	r.applyStructuralNodeAttrs(outer, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(outer, positionID, false)
	}
	return outer
}

func (r *storylineRenderer) renderPluginNode(node map[string]interface{}) htmlPart {
	resourceID, _ := asString(node["$175"])
	if resourceID == "" {
		return nil
	}
	href := r.resourceHrefByID[resourceID]
	if href == "" {
		return nil
	}
	resource := r.resourceFragments[resourceID]
	alt, _ := asString(node["$584"])
	switch {
	case resource.MediaType == "plugin/kfx-html-article" || resource.MediaType == "text/html" || resource.MediaType == "application/xhtml+xml":
		element := &htmlElement{
			Tag:   "iframe",
			Attrs: map[string]string{"src": href},
		}
		if styleAttr := styleStringFromDeclarations("class", nil, []string{
			"border-bottom-style: none",
			"border-left-style: none",
			"border-right-style: none",
			"border-top-style: none",
			"height: 100%",
			"width: 100%",
		}); styleAttr != "" {
			element.Attrs["style"] = styleAttr
		}
		r.applyStructuralNodeAttrs(element, node, "")
		return element
	case strings.HasPrefix(resource.MediaType, "audio/"):
		element := &htmlElement{
			Tag:   "audio",
			Attrs: map[string]string{"src": href, "controls": "controls"},
		}
		r.applyStructuralNodeAttrs(element, node, "")
		return element
	case strings.HasPrefix(resource.MediaType, "video/"):
		element := &htmlElement{
			Tag:   "video",
			Attrs: map[string]string{"src": href, "controls": "controls"},
		}
		if alt != "" {
			element.Attrs["aria-label"] = alt
		}
		r.applyStructuralNodeAttrs(element, node, "")
		return element
	case strings.HasPrefix(resource.MediaType, "image/"):
		return r.renderImageNode(node)
	default:
		element := &htmlElement{
			Tag:   "object",
			Attrs: map[string]string{"data": href},
		}
		if resource.MediaType != "" {
			element.Attrs["type"] = resource.MediaType
		}
		if alt != "" {
			element.Children = []htmlPart{htmlText{Text: alt}}
		}
		r.applyStructuralNodeAttrs(element, node, "")
		return element
	}
}

func (r *storylineRenderer) renderSVGNode(node map[string]interface{}) htmlPart {
	width, hasWidth := asInt(node["$66"])
	height, hasHeight := asInt(node["$67"])
	attrs := map[string]string{
		"version":             "1.1",
		"preserveAspectRatio": "xMidYMid meet",
	}
	if hasWidth && hasHeight && width > 0 && height > 0 {
		attrs["viewBox"] = fmt.Sprintf("0 0 %d %d", width, height)
	}
	element := &htmlElement{
		Tag:   "svg",
		Attrs: attrs,
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderTableNode(node map[string]interface{}, depth int) htmlPart {
	table := &htmlElement{Tag: "table", Attrs: map[string]string{}}
	if styleAttr := r.tableClass(node); styleAttr != "" {
		table.Attrs["style"] = styleAttr
	}
	if cols, ok := asSlice(node["$152"]); ok && len(cols) > 0 {
		colgroup := &htmlElement{Tag: "colgroup", Attrs: map[string]string{}}
		for _, raw := range cols {
			colMap, ok := asMap(raw)
			if !ok {
				continue
			}
			col := &htmlElement{Tag: "col", Attrs: map[string]string{}}
			if span, ok := asInt(colMap["$118"]); ok && span > 1 {
				col.Attrs["span"] = strconv.Itoa(span)
			}
			if styleAttr := r.tableColumnClass(colMap); styleAttr != "" {
				col.Attrs["style"] = styleAttr
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
				if childNode, ok := asMap(child); ok {
					r.applyStructuralAttrsToPart(rendered, childNode, table.Tag)
				}
				table.Children = append(table.Children, rendered)
			}
		}
	}
	if len(table.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(table, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(table, positionID, false)
	}
	return table
}

func (r *storylineRenderer) renderStructuredContainer(node map[string]interface{}, tag string, depth int) htmlPart {
	element := &htmlElement{Tag: tag, Attrs: map[string]string{}}
	if styleAttr := r.structuredContainerClass(node); styleAttr != "" {
		element.Attrs["style"] = styleAttr
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
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderTableRow(node map[string]interface{}, depth int) htmlPart {
	row := &htmlElement{Tag: "tr", Attrs: map[string]string{}}
	if styleID, _ := asString(node["$157"]); styleID != "" {
		if styleAttr := r.structuredContainerClass(node); styleAttr != "" {
			row.Attrs["style"] = styleAttr
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
	r.applyStructuralNodeAttrs(row, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(row, positionID, false)
	}
	return row
}

func (r *storylineRenderer) renderTableCell(node map[string]interface{}, depth int) htmlPart {
	cell := &htmlElement{Tag: "td", Attrs: map[string]string{}}
	if colspan, ok := asInt(node["$148"]); ok && colspan > 1 {
		cell.Attrs["colspan"] = strconv.Itoa(colspan)
	}
	if rowspan, ok := asInt(node["$149"]); ok && rowspan > 1 {
		cell.Attrs["rowspan"] = strconv.Itoa(rowspan)
	}
	if styleAttr := r.tableCellClass(node); styleAttr != "" {
		cell.Attrs["style"] = styleAttr
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
	r.applyStructuralNodeAttrs(cell, node, "")
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
	node, ok = r.prepareRenderableNode(node)
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
		if styleAttr := r.spanClass(styleID); styleAttr != "" {
			element.Attrs["style"] = styleAttr
		}
		r.applyStructuralNodeAttrs(element, node, "")
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
	if styleAttr := r.inlineContainerClass(styleID, node); styleAttr != "" {
		container.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(container, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(container, positionID, false)
	}
	return container
}

func (r *storylineRenderer) renderImageNode(node map[string]interface{}) htmlPart {
	node, ok := r.prepareRenderableNode(node)
	if !ok {
		return nil
	}
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
		image.Attrs["style"] = imageClass
	}
	if wrapperClass == "" {
		firstVisible := r.consumeVisibleElement()
		r.applyStructuralNodeAttrs(image, node, "")
		if positionID, _ := asInt(node["$155"]); positionID != 0 {
			r.applyPositionAnchors(image, positionID, firstVisible)
		}
		return image
	}
	wrapper := &htmlElement{
		Tag:      "div",
		Attrs:    map[string]string{"style": wrapperClass},
		Children: []htmlPart{image},
	}
	r.applyStructuralNodeAttrs(wrapper, node, "")
	firstVisible := r.consumeVisibleElement()
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(wrapper, positionID, firstVisible)
	}
	return wrapper
}

func (r *storylineRenderer) renderTextNode(node map[string]interface{}, depth int) htmlPart {
	_ = depth
	var ok bool
	node, ok = r.prepareRenderableNode(node)
	if !ok {
		return nil
	}
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
	annotationStyleID := fullParagraphAnnotationStyleID(node, text)

	styleID, _ := asString(node["$157"])
	level := headingLevel(node)
	if level == 0 {
		level = r.headingLevelForPosition(positionID, 0)
	}
	// Port of Python simplify_styles heading tag selection (yj_to_epub_properties.py ~L1928):
	// Only promote to <h1>-<h6> when layout hints include "heading" AND not fixed/illustrated layout.
	// Python: "heading" in kfx_layout_hints and not contains_block_elem → elem.tag = "h" + level
	isHeading := layoutHintsInclude(r.nodeLayoutHints(node), "heading")
	if level > 0 {
		r.lastKFXHeadingLevel = level
		if !isHeading {
			// Heading level stored in CSS ($790) but layout hints don't confirm heading;
			// render as <p> like Python does (simplify_styles won't promote this <div>).
			level = 0
		}
	} else if isHeading {
		level = r.lastKFXHeadingLevel
	}
	if level > 0 {
		if annotationStyleID != "" {
			removeSingleFullTextLinkClass(content)
		}
		firstVisible := r.consumeVisibleElement()
		element := &htmlElement{
			Tag:      fmt.Sprintf("h%d", level),
			Attrs:    map[string]string{},
			Children: content,
		}
		if styleID != "" {
			if styleAttr := r.headingClass(styleID); styleAttr != "" {
				element.Attrs["style"] = styleAttr
			}
		}
		r.applyStructuralNodeAttrs(element, node, "")
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
		if styleAttr := r.paragraphClass(styleID, annotationStyleID); styleAttr != "" {
			element.Attrs["style"] = styleAttr
		}
	}
	r.applyFirstLineStyle(element, node)
	r.applyStructuralNodeAttrs(element, node, "")
	r.applyPositionAnchors(element, positionID, firstVisible)
	return element
}

func removeSingleFullTextLinkClass(parts []htmlPart) {
	if len(parts) != 1 {
		return
	}
	link, ok := parts[0].(*htmlElement)
	if !ok || link == nil || link.Tag != "a" {
		return
	}
	delete(link.Attrs, "class")
	delete(link.Attrs, "style")
}

func (r *storylineRenderer) applyStructuralAttrsToPart(part htmlPart, node map[string]interface{}, parentTag string) {
	element, ok := part.(*htmlElement)
	if !ok {
		return
	}
	r.applyStructuralNodeAttrs(element, node, parentTag)
}

func (r *storylineRenderer) applyFirstLineStyle(element *htmlElement, node map[string]interface{}) {
	if r == nil || element == nil || node == nil {
		return
	}
	raw, ok := asMap(node["$622"])
	if !ok {
		return
	}
	style := cloneMap(raw)
	if styleID, _ := asString(style["$173"]); styleID != "" {
		style = effectiveStyle(r.styleFragments[styleID], style)
	}
	delete(style, "$173")
	delete(style, "$625")
	declarations := cssDeclarationsFromMap(processContentProperties(style))
	if len(declarations) == 0 {
		return
	}
	className := r.styles.reserveClass("kfx-firstline")
	if className == "" {
		return
	}
	element.Attrs["class"] = appendClassNames(element.Attrs["class"], className)
	r.styles.addStatic("."+className+"::first-line", declarations)
}

func (r *storylineRenderer) wrapNodeLink(node map[string]interface{}, part htmlPart) htmlPart {
	if node == nil || part == nil {
		return part
	}
	anchorID, _ := asString(node["$179"])
	if anchorID == "" {
		return part
	}
	href := r.anchorHref(anchorID)
	if href == "" {
		return part
	}
	if element, ok := part.(*htmlElement); ok && element != nil && element.Tag == "a" {
		if element.Attrs == nil {
			element.Attrs = map[string]string{}
		}
		if element.Attrs["href"] == "" {
			element.Attrs["href"] = href
		}
		return element
	}
	return &htmlElement{
		Tag:      "a",
		Attrs:    map[string]string{"href": href},
		Children: []htmlPart{part},
	}
}

func (r *storylineRenderer) anchorHref(anchorID string) string {
	if anchorID == "" {
		return ""
	}
	if href := r.directAnchorURI[anchorID]; href != "" {
		return href
	}
	if href := r.anchorToFilename[anchorID]; href != "" {
		return href
	}
	if r.anchorNameRegistered(anchorID) {
		return "anchor:" + anchorID
	}
	return anchorID
}

func (r *storylineRenderer) anchorNameRegistered(anchorID string) bool {
	if r == nil || anchorID == "" {
		return false
	}
	for _, offsets := range r.positionAnchors {
		for _, names := range offsets {
			for _, name := range names {
				if name == anchorID {
					return true
				}
			}
		}
	}
	return false
}

func (r *storylineRenderer) prepareRenderableNode(node map[string]interface{}) (map[string]interface{}, bool) {
	if node == nil {
		return nil, false
	}
	working := cloneMap(node)
	hadConditionalContent := working["$592"] != nil || working["$591"] != nil || working["$663"] != nil
	if include := working["$592"]; include != nil && !r.conditionEvaluator.evaluateBinary(include) {
		return nil, false
	}
	delete(working, "$592")
	if exclude := working["$591"]; exclude != nil && r.conditionEvaluator.evaluateBinary(exclude) {
		return nil, false
	}
	delete(working, "$591")
	if rawConditional, ok := asSlice(working["$663"]); ok {
		for _, raw := range rawConditional {
			props, ok := asMap(raw)
			if !ok {
				continue
			}
			if merged := r.mergeConditionalProperties(working, props); merged != nil {
				working = merged
			}
		}
	}
	delete(working, "$663")
	if hadConditionalContent {
		working["__has_conditional_content__"] = true
	}
	return working, true
}

func (r *storylineRenderer) mergeConditionalProperties(node map[string]interface{}, conditional map[string]interface{}) map[string]interface{} {
	if node == nil || conditional == nil {
		return node
	}
	props := cloneMap(conditional)
	apply := false
	if include := props["$592"]; include != nil {
		apply = r.conditionEvaluator.evaluateBinary(include)
		delete(props, "$592")
	} else if exclude := props["$591"]; exclude != nil {
		apply = !r.conditionEvaluator.evaluateBinary(exclude)
		delete(props, "$591")
	}
	if !apply {
		return node
	}
	merged := cloneMap(node)
	for key, value := range props {
		merged[key] = value
	}
	return merged
}

func (r *storylineRenderer) applyStructuralNodeAttrs(element *htmlElement, node map[string]interface{}, parentTag string) {
	if element == nil || node == nil {
		return
	}
	if element.Tag == "div" {
		if r.shouldPromoteLayoutHints() && layoutHintsInclude(r.nodeLayoutHints(node), "figure") && htmlPartContainsImage(element) {
			element.Tag = "figure"
		}
	}
	classification, _ := asString(node["$615"])
	switch {
	case classification == "$453" && parentTag == "table" && element.Tag == "div":
		element.Tag = "caption"
	case classificationEPUBType[classification] != "" && element.Tag == "div":
		element.Tag = "aside"
	}
	if epubType := classificationEPUBType[classification]; epubType != "" && element.Tag == "aside" {
		element.Attrs["epub:type"] = epubType
	}
	if classification == "$688" {
		element.Attrs["role"] = "math"
	}
	switch asStringDefault(node["$156"]) {
	case "$324", "$325":
		if styleAttr := styleStringFromDeclarations("class", nil, []string{"position: fixed"}); styleAttr != "" {
			element.Attrs["style"] = mergeStyleStrings(element.Attrs["style"], styleAttr)
		}
	}
}

func (r *storylineRenderer) nodeLayoutHints(node map[string]interface{}) []string {
	if node == nil {
		return nil
	}
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	switch typed := style["$761"].(type) {
	case string:
		if typed == "" {
			return nil
		}
		if hint := layoutHintElementNames[typed]; hint != "" {
			return []string{hint}
		}
		return strings.Fields(typed)
	case []interface{}:
		hints := make([]string, 0, len(typed))
		for _, raw := range typed {
			value, ok := asString(raw)
			if !ok || value == "" {
				continue
			}
			if hint := layoutHintElementNames[value]; hint != "" {
				hints = append(hints, hint)
				continue
			}
			hints = append(hints, strings.Fields(value)...)
		}
		if len(hints) == 0 {
			return nil
		}
		return hints
	default:
		return nil
	}
}

func (r *storylineRenderer) layoutHintHeadingTag(node map[string]interface{}, children []interface{}) string {
	if !r.shouldPromoteStructuralContainer(node) {
		return ""
	}
	if !layoutHintsInclude(r.nodeLayoutHints(node), "heading") {
		return ""
	}
	level := headingLevel(node)
	if level <= 0 || level > 6 {
		return ""
	}
	for _, child := range children {
		if r.renderInlinePart(child, 0) == nil {
			return ""
		}
	}
	return fmt.Sprintf("h%d", level)
}

func (r *storylineRenderer) renderInlineParagraphContainer(node map[string]interface{}, children []interface{}, depth int) htmlPart {
	if !r.shouldPromoteStructuralContainer(node) || len(children) != 1 || !nodeContainsTextContent(children) {
		return nil
	}
	element := &htmlElement{Tag: "p", Attrs: map[string]string{}}
	for _, child := range children {
		if inline := r.renderInlinePart(child, depth+1); inline != nil {
			element.Children = append(element.Children, inline)
			continue
		}
		return nil
	}
	if len(element.Children) == 0 {
		return nil
	}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		element.Attrs["style"] = styleAttr
	}
	r.applyFirstLineStyle(element, node)
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderInlineRenderContainer(node map[string]interface{}, children []interface{}, depth int) htmlPart {
	renderMode, _ := asString(node["$601"])
	if renderMode != "$283" {
		return nil
	}
	styleID, _ := asString(node["$157"])
	element := &htmlElement{Tag: "span", Attrs: map[string]string{}}
	for _, child := range children {
		if inline := r.renderInlinePart(child, depth+1); inline != nil {
			element.Children = append(element.Children, inline)
			continue
		}
		return nil
	}
	if len(element.Children) == 0 {
		return nil
	}
	if styleAttr := r.inlineContainerClass(styleID, node); styleAttr != "" {
		element.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderFigureHintContainer(node map[string]interface{}, children []interface{}, depth int) htmlPart {
	if !r.shouldPromoteStructuralContainer(node) || !layoutHintsInclude(r.nodeLayoutHints(node), "figure") {
		return nil
	}
	element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	for _, child := range children {
		if rendered := r.renderNode(child, depth+1); rendered != nil {
			element.Children = append(element.Children, rendered)
			continue
		}
		if inline := r.renderInlinePart(child, depth+1); inline != nil {
			element.Children = append(element.Children, inline)
			continue
		}
		return nil
	}
	if len(element.Children) == 0 || !htmlPartContainsImage(element) {
		return nil
	}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		element.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["$155"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) shouldPromoteLayoutHints() bool {
	if r == nil {
		return true
	}
	return !r.conditionEvaluator.fixedLayout && !r.conditionEvaluator.illustratedLayout
}

func (r *storylineRenderer) shouldPromoteStructuralContainer(node map[string]interface{}) bool {
	if !r.shouldPromoteLayoutHints() || node == nil {
		return false
	}
	if node["__has_conditional_content__"] != nil || node["$615"] != nil {
		return false
	}
	switch asStringDefault(node["$156"]) {
	case "$324", "$325":
		return false
	}
	return true
}

func nodeContainsTextContent(children []interface{}) bool {
	for _, raw := range children {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		if _, ok := asMap(node["$145"]); ok {
			return true
		}
		if nested, ok := asSlice(node["$146"]); ok && nodeContainsTextContent(nested) {
			return true
		}
	}
	return false
}

func layoutHintsInclude(hints []string, want string) bool {
	for _, hint := range hints {
		if hint == want {
			return true
		}
	}
	return false
}

func htmlPartContainsImage(part htmlPart) bool {
	switch typed := part.(type) {
	case *htmlElement:
		if typed == nil {
			return false
		}
		if typed.Tag == "img" {
			return true
		}
		for _, child := range typed.Children {
			if htmlPartContainsImage(child) {
				return true
			}
		}
	}
	return false
}

func (r *storylineRenderer) bodyClass(styleID string, values map[string]interface{}) string {
	style := effectiveStyle(r.styleFragments[styleID], values)
	if len(style) == 0 {
		return ""
	}
	declarations := cssDeclarationsFromMap(processContentProperties(style))
	if len(declarations) == 0 {
		return ""
	}
	if bodyClass := staticBodyClassForDeclarations(declarations); bodyClass != "" {
		return bodyClass
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) containerClass(node map[string]interface{}) string {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = mergeStyleValues(style, r.inferPromotedStyleValues(node))
	if len(style) == 0 {
		return ""
	}
	declarations := filterBodyDefaultDeclarations(cssDeclarationsFromMap(processContentProperties(style)), r.activeBodyDefaults)
	if mapFontStyle(style["$12"]) == "normal" && bodyDefaultsInclude(r.activeBodyDefaults, "font-style: italic") {
		declarations = append(declarations, "font-style: normal")
	}
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, r.nodeLayoutHints(node), declarations)
}

func (r *storylineRenderer) tableClass(node map[string]interface{}) string {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	declarations := cssDeclarationsFromMap(processContentProperties(style))
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) tableColumnClass(node map[string]interface{}) string {
	style := effectiveStyle(nil, node)
	declarations := cssDeclarationsFromMap(processContentProperties(style))
	if len(declarations) == 0 {
		return ""
	}
	return styleStringFromDeclarations("class", nil, declarations)
}

func (r *storylineRenderer) structuredContainerClass(node map[string]interface{}) string {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = mergeStyleValues(style, r.inferPromotedStyleValues(node))
	declarations := cssDeclarationsFromMap(processContentProperties(style))
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
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
	declarations := cssDeclarationsFromMap(processContentProperties(style))
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) inlineContainerClass(styleID string, node map[string]interface{}) string {
	style := effectiveStyle(r.styleFragments[styleID], node)
	declarations := cssDeclarationsFromMap(processContentProperties(style))
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) imageClasses(node map[string]interface{}) (string, string) {
	styleID, _ := asString(node["$157"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = r.adjustRenderableStyle(style, node)
	if len(style) == 0 {
		return "", ""
	}
	wrapperDecls := imageWrapperStyleDeclarations(style)
	imageDecls := imageStyleDeclarations(style)
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	switch {
	case len(wrapperDecls) > 0 && len(imageDecls) > 0:
		return styleStringFromDeclarations(baseName, nil, wrapperDecls), styleStringFromDeclarations(baseName, nil, imageDecls)
	case len(wrapperDecls) > 0:
		return styleStringFromDeclarations(baseName, nil, wrapperDecls), ""
	case len(imageDecls) > 0:
		return "", styleStringFromDeclarations(baseName, nil, imageDecls)
	default:
		return "", ""
	}
}

func (r *storylineRenderer) adjustRenderableStyle(style map[string]interface{}, node map[string]interface{}) map[string]interface{} {
	if len(style) == 0 {
		return style
	}
	if fitTight, _ := asBool(node["$784"]); fitTight {
		if value := cssLengthProperty(style["$56"], "$56"); value == "100%" {
			style = cloneMap(style)
			delete(style, "$56")
		}
	}
	return style
}

func (r *storylineRenderer) headingClass(styleID string) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	className := r.headingClassName(styleID, style)
	declarations := filterBodyDefaultDeclarations(cssDeclarationsFromMap(processContentProperties(style)), r.activeBodyDefaults)
	if mapFontStyle(style["$12"]) == "normal" && bodyDefaultsInclude(r.activeBodyDefaults, "font-style: italic") {
		declarations = append(declarations, "font-style: normal")
	}
	if style["$36"] == nil && activeTextIndentNeedsReset(r.activeBodyDefaults) {
		declarations = append(declarations, "text-indent: 0")
	}
	if len(declarations) == 0 {
		return ""
	}
	return styleStringFromDeclarations(className, []string{"heading"}, declarations)
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
			baseName = r.styleBaseName(styleID)
		}
		className = styleStringFromDeclarations(baseName, nil, declarations)
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
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) spanClass(styleID string) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	declarations := cssDeclarationsFromMap(processContentProperties(style))
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) resolveText(ref map[string]interface{}) string {
	return resolveContentText(r.contentFragments, ref)
}

func resolveContentText(contentFragments map[string][]string, ref map[string]interface{}) string {
	name, _ := asString(ref["name"])
	index, ok := asInt(ref["$403"])
	if !ok {
		return ""
	}
	values := contentFragments[name]
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func inferBookLanguage(defaultLanguage string, contentFragments map[string][]string, storylines map[string]map[string]interface{}, styleFragments map[string]map[string]interface{}) string {
	defaultKey := languageKey(defaultLanguage)
	if defaultKey == "" {
		return defaultLanguage
	}
	merits := map[string]int{}
	for _, storyline := range storylines {
		nodes, _ := asSlice(storyline["$146"])
		accumulateContentLanguageMerits(nodes, defaultKey, merits, contentFragments, styleFragments)
	}
	bestLanguage := defaultKey
	bestMerit := 0
	for language, merit := range merits {
		if merit <= bestMerit || !languageMatchesDefault(language, defaultKey) {
			continue
		}
		bestLanguage = language
		bestMerit = merit
	}
	if bestMerit == 0 {
		return defaultLanguage
	}
	return bestLanguage
}

func accumulateContentLanguageMerits(nodes []interface{}, currentLanguage string, merits map[string]int, contentFragments map[string][]string, styleFragments map[string]map[string]interface{}) {
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		language := currentLanguage
		styleID, _ := asString(node["$157"])
		style := effectiveStyle(styleFragments[styleID], node)
		if rawLanguage, ok := asString(style["$10"]); ok && rawLanguage != "" {
			language = languageKey(rawLanguage)
		}
		if ref, ok := asMap(node["$145"]); ok && language != "" {
			merits[language] += len([]rune(resolveContentText(contentFragments, ref)))
		}
		if children, ok := asSlice(node["$146"]); ok {
			accumulateContentLanguageMerits(children, language, merits, contentFragments, styleFragments)
		}
	}
}

func languageKey(language string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
}

func languageMatchesDefault(candidate string, defaultLanguage string) bool {
	if candidate == "" || defaultLanguage == "" {
		return false
	}
	return candidate == defaultLanguage || strings.HasPrefix(candidate, defaultLanguage+"-")
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

func (r *storylineRenderer) headingClassName(styleID string, style map[string]interface{}) string {
	simplified := uniquePartOfLocalSymbol(styleID, r.symFmt)
	if simplified != "" {
		return "heading_" + simplified
	}
	return "heading_" + styleID
}

func appendClassNames(existing string, classNames ...string) string {
	parts := []string{}
	seen := map[string]bool{}
	for _, raw := range append([]string{existing}, classNames...) {
		for _, className := range strings.Fields(strings.TrimSpace(raw)) {
			if className == "" || seen[className] {
				continue
			}
			seen[className] = true
			parts = append(parts, className)
		}
	}
	return strings.Join(parts, " ")
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
		open  func(parent *htmlElement) *htmlElement
		close func(opened *htmlElement)
	}
	type activeEvent struct {
		event  event
		opened *htmlElement
	}
	runes := []rune(text)
	if dropcapLines, hasDropcapLines := asInt(node["$125"]); hasDropcapLines && dropcapLines > 0 {
		if dropcapChars, hasDropcapChars := asInt(node["$126"]); hasDropcapChars && dropcapChars > 0 {
			dropcap := map[string]interface{}{
				"$143": 0,
				"$144": dropcapChars,
				"$125": dropcapLines,
			}
			annotations = append([]interface{}{dropcap}, annotations...)
			ok = true
		}
	}
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
			dropcapClass := ""
			if lines, ok := asInt(annotationMap["$125"]); ok && lines > 0 {
				dropcapClass = r.dropcapClass(lines)
			}
			if debugAnchors := os.Getenv("KFX_DEBUG_ANCHORS"); debugAnchors != "" && anchorID != "" {
				for _, wanted := range strings.Split(debugAnchors, ",") {
					if strings.TrimSpace(wanted) == anchorID {
						fmt.Fprintf(os.Stderr, "annotation anchor=%s style=%s value=%#v\n", anchorID, styleID, annotationMap)
					}
				}
			}
			href := r.anchorHref(anchorID)
			rubyName, hasRubyName := asString(annotationMap["$757"])
			if hasRubyName && rubyName != "" {
				rubyIDs := r.rubyAnnotationIDs(annotationMap, end-start)
				var rubyElement *htmlElement
				events = append(events, event{
					start: start,
					end:   end,
					open: func(parent *htmlElement) *htmlElement {
						rubyElement = &htmlElement{Tag: "ruby", Attrs: map[string]string{}}
						parent.Children = append(parent.Children, rubyElement)
						rb := &htmlElement{Tag: "rb", Attrs: map[string]string{}}
						rubyElement.Children = append(rubyElement.Children, rb)
						return rb
					},
					close: func(opened *htmlElement) {
						if opened == nil || rubyElement == nil {
							return
						}
						for _, rubyID := range rubyIDs {
							rt := &htmlElement{Tag: "rt", Attrs: map[string]string{}, Children: r.rubyContentParts(rubyName, rubyID)}
							rubyElement.Children = append(rubyElement.Children, rt)
						}
					},
				})
				continue
			}
			if href != "" {
				styleAttr := mergeStyleStrings(
					r.linkClass(styleID, annotationCoversWholeText(annotationMap, len(runes))),
					dropcapClass,
				)
				events = append(events, event{
					start: start,
					end:   end,
					open: func(parent *htmlElement) *htmlElement {
						attrs := map[string]string{"href": href}
						if styleAttr != "" {
							attrs["style"] = styleAttr
						}
						element := &htmlElement{Tag: "a", Attrs: attrs}
						parent.Children = append(parent.Children, element)
						return element
					},
				})
				continue
			}
			if styleAttr := mergeStyleStrings(r.spanClass(styleID), dropcapClass); styleAttr != "" {
				events = append(events, event{
					start: start,
					end:   end,
					open: func(parent *htmlElement) *htmlElement {
						element := &htmlElement{Tag: "span", Attrs: map[string]string{"style": styleAttr}}
						parent.Children = append(parent.Children, element)
						return element
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
	stack := []*activeEvent{{opened: root}}
	last := 0
	for index, rch := range runes {
		if last < index {
			appendTextHTMLParts(stack[len(stack)-1].opened, string(runes[last:index]))
			last = index
		}
		for _, ev := range events {
			if ev.start == index {
				opened := ev.open(stack[len(stack)-1].opened)
				stack = append(stack, &activeEvent{event: ev, opened: opened})
			}
		}
		appendTextHTMLParts(stack[len(stack)-1].opened, string(rch))
		last = index + 1
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].end == index+1 {
				if len(stack) > 1 {
					active := stack[len(stack)-1]
					if active.event.close != nil {
						active.event.close(active.opened)
					}
					stack = stack[:len(stack)-1]
				}
			}
		}
	}
	if last < len(runes) {
		appendTextHTMLParts(stack[len(stack)-1].opened, string(runes[last:]))
	}
	return root.Children
}

func (r *storylineRenderer) rubyAnnotationIDs(annotationMap map[string]interface{}, eventLength int) []int {
	if annotationMap == nil {
		return nil
	}
	if rubyID, ok := asInt(annotationMap["$758"]); ok {
		return []int{rubyID}
	}
	rawIDs, ok := asSlice(annotationMap["$759"])
	if !ok {
		return nil
	}
	ids := make([]int, 0, len(rawIDs))
	for _, raw := range rawIDs {
		entry, ok := asMap(raw)
		if !ok {
			continue
		}
		if rubyID, ok := asInt(entry["$758"]); ok {
			ids = append(ids, rubyID)
		}
	}
	return ids
}

func (r *storylineRenderer) rubyContentParts(rubyName string, rubyID int) []htmlPart {
	content := r.getRubyContent(rubyName, rubyID)
	if content == nil {
		return nil
	}
	if ref, ok := asMap(content["$145"]); ok {
		if text := r.resolveText(ref); text != "" {
			return splitTextHTMLParts(text)
		}
	}
	if children, ok := asSlice(content["$146"]); ok {
		parts := make([]htmlPart, 0, len(children))
		for _, child := range children {
			if rendered := r.renderInlinePart(child, 0); rendered != nil {
				parts = append(parts, rendered)
			}
		}
		return parts
	}
	return nil
}

func (r *storylineRenderer) getRubyContent(rubyName string, rubyID int) map[string]interface{} {
	group := r.rubyGroups[rubyName]
	if group == nil {
		return nil
	}
	children, _ := asSlice(group["$146"])
	for _, raw := range children {
		switch typed := raw.(type) {
		case string:
			if content := r.rubyContents[typed]; content != nil {
				if id, ok := asInt(content["$758"]); ok && id == rubyID {
					return cloneMap(content)
				}
			}
		default:
			entry, ok := asMap(raw)
			if !ok {
				continue
			}
			if id, ok := asInt(entry["$758"]); ok && id == rubyID {
				return cloneMap(entry)
			}
		}
	}
	return nil
}

func (r *storylineRenderer) dropcapClass(lines int) string {
	if lines <= 0 {
		return ""
	}
	return styleStringFromDeclarations("class", nil, []string{
		"float: left",
		fmt.Sprintf("font-size: %dem", lines),
		"line-height: 100%",
		"margin-bottom: 0",
		"margin-right: 0.1em",
		"margin-top: 0",
	})
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
			r.registerAnchorElementNames(positionID, 0, anchorID)
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
			r.registerAnchorElementNames(positionID, offset, anchorID)
		}
	}
}

func (r *storylineRenderer) registerAnchorElementNames(positionID int, offset int, anchorID string) {
	if r == nil || anchorID == "" {
		return
	}
	offsets := r.positionAnchors[positionID]
	if offsets == nil {
		return
	}
	names := offsets[offset]
	if len(names) == 0 {
		return
	}
	if r.anchorNamesByID == nil {
		r.anchorNamesByID = map[string][]string{}
	}
	seen := map[string]bool{}
	for _, existing := range r.anchorNamesByID[anchorID] {
		seen[existing] = true
	}
	for _, name := range names {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		r.anchorNamesByID[anchorID] = append(r.anchorNamesByID[anchorID], name)
	}
}

func (r *storylineRenderer) headingLevelForPosition(positionID int, offset int) int {
	if r == nil || positionID == 0 || r.anchorHeadingLevel == nil {
		return 0
	}
	offsets := r.positionAnchors[positionID]
	if offsets == nil {
		return 0
	}
	for _, name := range offsets[offset] {
		if level := r.anchorHeadingLevel[name]; level > 0 {
			return level
		}
	}
	return 0
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
		// Match cover section by either title or containing the cover image.
		// Calibre identifies cover in process_section via layout + image, not title alone.
		if !strings.Contains(section.BodyHTML, `src="`+book.CoverImageHref+`"`) {
			continue
		}
		// Only promote sections that are primarily a cover image (not mixed content).
		if section.Title != "Cover" && !isCoverImageSection(section.BodyHTML) {
			continue
		}
		// Calibre adds class_s8 for SVG cover sections that are titled "Cover".
		if section.Title == "Cover" {
			section.BodyClass = "class_s8"
		} else {
			section.BodyClass = ""
		}
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
