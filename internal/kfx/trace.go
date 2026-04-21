package kfx

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
)

// symTypeNames maps symType values to their Python-equivalent string representations.
var symTypeNames = map[symType]string{
	"":         "none",
	symShared:  "shared",
	symCommon:  "common",
	symDictionary: "dictionary",
	symOriginal: "original",
	symBase64:  "base64",
	symShort:   "short",
	symUnknown: "unknown",
}

// traceWriter accumulates pipeline stage snapshots for parity comparison with Python.
// Enable with KFX_TRACE=1 or the --trace CLI flag.
type traceWriter struct {
	Input  string                 `json:"input"`
	Stages map[string]interface{} `json:"stages"`
}

func newTraceWriter(inputPath string) *traceWriter {
	return &traceWriter{
		Input:  inputPath,
		Stages: make(map[string]interface{}),
	}
}

// addStage records a pipeline stage snapshot.
func (tw *traceWriter) addStage(name string, data interface{}) {
	if tw == nil {
		return
	}
	tw.Stages[name] = data
}

// writeToFile writes the accumulated trace as JSON to the given path.
func (tw *traceWriter) writeToFile(path string) error {
	if tw == nil {
		return nil
	}
	data, err := json.MarshalIndent(tw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal trace: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// --- Stage data types ---

type traceFragmentType struct {
	Count int      `json:"count"`
	IDs   []string `json:"ids,omitempty"`
}

type traceOrganizeFragments struct {
	FragmentTypes map[string]traceFragmentType `json:"fragment_types"`
}

type traceBookSymbolFormat struct {
	Format string `json:"format"`
}

type traceContentFeatures struct {
	IsPrintReplica bool   `json:"is_print_replica"`
	IsPDFBacked    bool   `json:"is_pdf_backed"`
	CDEContentType string `json:"cde_content_type"`
}

type traceFonts struct {
	PresentFontNames []string `json:"present_font_names"`
	UsedFontNames    []string `json:"used_font_names"`
	FontFacesCount   int      `json:"font_faces_count"`
}

type traceDocumentData struct {
	OrientationLock    string `json:"orientation_lock"`
	FixedLayout        bool   `json:"fixed_layout"`
	IllustratedLayout  bool   `json:"illustrated_layout"`
	OriginalWidth      *int   `json:"original_width"`
	OriginalHeight     *int   `json:"original_height"`
	RegionMagnification bool  `json:"region_magnification"`
	VirtualPanels      bool   `json:"virtual_panels"`
	VirtualPanelsAllowed bool `json:"virtual_panels_allowed"`
	GuidedViewNative   bool   `json:"guided_view_native"`
	ScrolledContinuous bool   `json:"scrolled_continuous"`
}

type traceMetadata struct {
	Title            string   `json:"title"`
	Authors          []string `json:"authors"`
	Language         string   `json:"language"`
	Publisher        string   `json:"publisher"`
	PubDate          string   `json:"pubdate"`
	Description      string   `json:"description"`
	ASIN             string   `json:"asin"`
	BookID           string   `json:"book_id"`
	OrientationLock  string   `json:"orientation_lock"`
	FixedLayout      bool     `json:"fixed_layout"`
	IllustratedLayout bool    `json:"illustrated_layout"`
	OverrideKindleFont bool   `json:"override_kindle_font"`
	HTMLCover        bool     `json:"html_cover"`
	BookType         string   `json:"book_type"`
	SourceLanguage    string   `json:"source_language"`
	TargetLanguage    string   `json:"target_language"`
	CoverResource    string   `json:"cover_resource"`
	WritingMode      string   `json:"writing_mode"`
}

type traceNavPoint struct {
	Title    string           `json:"title"`
	Target   string           `json:"target,omitempty"`
	Anchor   string           `json:"anchor,omitempty"`
	Children []traceNavPoint  `json:"children"`
}

type traceGuideEntry struct {
	GuideType string `json:"guide_type"`
	Title     string `json:"title"`
	Target    string `json:"target,omitempty"`
	Anchor    string `json:"anchor,omitempty"`
}

type tracePageTarget struct {
	Label  string `json:"label"`
	Target string `json:"target,omitempty"`
	Anchor string `json:"anchor,omitempty"`
}

type traceNavigation struct {
	NCXToc  []traceNavPoint    `json:"ncx_toc"`
	Guide   []traceGuideEntry  `json:"guide"`
	Pagemap []tracePageTarget  `json:"pagemap"`
}

type traceSection struct {
	Filename     string   `json:"filename"`
	BodyHTML     string   `json:"body_html"`
	BodyClass    string   `json:"body_class"`
	OPFProperties []string `json:"opf_properties"`
	IsCoverPage  bool     `json:"is_cover_page"`
}

type traceStylesheet struct {
	CSSContent string   `json:"css_content"`
	CSSFiles   []string `json:"css_files"`
}

type traceFinalSections map[string]string

// --- Capture functions ---

func captureOrganizeFragments(state *bookState) traceOrganizeFragments {
	result := traceOrganizeFragments{
		FragmentTypes: make(map[string]traceFragmentType),
	}
	snapshot := state.fragmentSnapshot()
	for ft, ts := range snapshot.Types {
		result.FragmentTypes[ft] = traceFragmentType{
			Count: ts.Count,
			IDs:   ts.IDs,
		}
	}
	return result
}

func captureBookSymbolFormat(state *bookState) traceBookSymbolFormat {
	return traceBookSymbolFormat{
		Format: symTypeNames[state.BookSymbolFormat],
	}
}

func captureContentFeatures(book *decodedBook) traceContentFeatures {
	return traceContentFeatures{
		CDEContentType: book.BookID, // best approximation; features are internal
	}
}

func captureFonts(book *decodedBook) traceFonts {
	// Collect font names from resources
	present := map[string]bool{}
	used := map[string]bool{}
	for _, r := range book.Resources {
		if strings.HasPrefix(r.MediaType, "font/") || strings.HasSuffix(r.Filename, ".otf") || strings.HasSuffix(r.Filename, ".ttf") {
			present[r.Filename] = true
		}
	}
	return traceFonts{
		PresentFontNames: sortedTraceKeys(present),
		UsedFontNames:    sortedTraceKeys(used),
		FontFacesCount:   len(present),
	}
}

func captureDocumentData(book *decodedBook) traceDocumentData {
	return traceDocumentData{
		OrientationLock:    book.OrientationLock,
		FixedLayout:        book.FixedLayout,
		IllustratedLayout:  book.IllustratedLayout,
	}
}

func captureMetadata(book *decodedBook) traceMetadata {
	bookType := ""
	return traceMetadata{
		Title:             book.Title,
		Authors:           book.Authors,
		Language:          book.Language,
		Publisher:         book.Publisher,
		PubDate:           book.Published,
		Description:       book.Description,
		ASIN:              book.ASIN,
		BookID:            book.BookID,
		OrientationLock:   book.OrientationLock,
		FixedLayout:       book.FixedLayout,
		IllustratedLayout: book.IllustratedLayout,
		OverrideKindleFont: book.OverrideKindleFonts,
		BookType:          bookType,
		CoverResource:     book.CoverImageID,
		WritingMode:       book.WritingMode,
	}
}

func captureNavigation(nav []epub.NavPoint, guide []epub.GuideEntry, pages []epub.PageTarget) traceNavigation {
	return traceNavigation{
		NCXToc:  convertNavPoints(nav),
		Guide:   convertGuideEntries(guide),
		Pagemap: convertPageTargets(pages),
	}
}

func convertNavPoints(pts []epub.NavPoint) []traceNavPoint {
	result := make([]traceNavPoint, len(pts))
	for i, p := range pts {
		result[i] = traceNavPoint{
			Title:    p.Title,
			Target:   p.Href,
			Children: convertNavPoints(p.Children),
		}
	}
	return result
}

func convertGuideEntries(entries []epub.GuideEntry) []traceGuideEntry {
	result := make([]traceGuideEntry, len(entries))
	for i, e := range entries {
		result[i] = traceGuideEntry{
			GuideType: e.Type,
			Title:     e.Title,
			Target:    e.Href,
		}
	}
	return result
}

func convertPageTargets(pts []epub.PageTarget) []tracePageTarget {
	result := make([]tracePageTarget, len(pts))
	for i, p := range pts {
		result[i] = tracePageTarget{
			Label:  p.Label,
			Target: p.Href,
		}
	}
	return result
}

func captureReadingOrder(sections []renderedSection) map[string]traceSection {
	result := make(map[string]traceSection, len(sections))
	for _, s := range sections {
		result[s.Filename] = traceSection{
			Filename:      s.Filename,
			BodyHTML:      renderedSectionBodyHTML(s),
			BodyClass:     s.BodyClass,
			OPFProperties: []string{},
			IsCoverPage:   false,
		}
	}
	return result
}

func captureStylesheet(css string) traceStylesheet {
	return traceStylesheet{
		CSSContent: css,
		CSSFiles:   []string{"/stylesheet.css"},
	}
}

func captureFinalSections(sections []epub.Section) traceFinalSections {
	result := make(traceFinalSections, len(sections))
	for _, s := range sections {
		result[s.Filename] = s.BodyHTML
	}
	return result
}

func sortedTraceKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
