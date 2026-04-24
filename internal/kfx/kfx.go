package kfx

import (
	"bytes"
	"os"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
)

var (
	contSignature   = []byte("CONT")
	drmionSignature = []byte{0xea, 'D', 'R', 'M', 'I', 'O', 'N', 0xee}
)

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
	TitlePronunciation       string
	Language                 string
	Authors                  []string
	AuthorPronunciations     []string
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
	HasConditionalContent    bool // set during content rendering when conditional page templates are found

	// OriginalWidth and OriginalHeight are the most common viewport dimensions across
	// all fixed-layout book parts. Set by compareFixedLayoutViewports when the book is
	// fixed-layout and dimensions haven't been determined yet.
	// Port of Python self.original_width / self.original_height (epub_output.py L303, L636).
	OriginalWidth  int
	OriginalHeight int
	OverrideKindleFonts      bool
	CoverImageID             string
	CoverImageHref           string
	Stylesheet               string
	ResourceHrefByID         map[string]string
	// ResourceDimensions maps EPUB resource filenames to their pixel dimensions [width, height].
	// Populated during buildResources from resource_fragment metadata ($422/$66 width, $423/$67 height).
	// Used by simplify_styles for vh/vw cross-conversion (Python yj_to_epub_properties.py L1753-1785).
	ResourceDimensions       map[string][2]int
	RenderedSections         []renderedSection
	Sections                 []epub.Section
	Resources                []epub.Resource
	Navigation               []epub.NavPoint
	Guide                    []epub.GuideEntry
	PageList                 []epub.PageTarget
	// DefaultFontFamily is the resolved font-family from document metadata ($538),
	// used to resolve "default" font names in KFX data. Port of Python
	// KFX_EPUB_Properties.default_font_family (yj_to_epub_metadata.py L110).
	DefaultFontFamily string

	// --- Book type flags (set by applyMetadata, propagated to content processing) ---
	// Port of Python GAP 4 / N2-N7: book type flag propagation from yj_to_epub_metadata.py.

	// CDEContentType stores the raw cde_content_type value (e.g. "MAGZ", "EBSP", "EBOK").
	// Python L201-206: self.cde_content_type = value; MAGZ→magazine, EBSP→sample.
	CDEContentType string

	// IsDictionary is true for dictionary books (kindle_title_metadata/dictionary_lookup or is_dictionary).
	// Python L213-217, L236.
	IsDictionary bool

	// IsSample is true when cde_content_type is "EBSP" or is_sample metadata is true.
	// Python L206, L238.
	IsSample bool

	// ScrolledContinuous is true when yj_forced_continuous_scroll capability is present.
	// Python L250-251.
	ScrolledContinuous bool

	// GuidedViewNative is true when yj_guided_view_native capability is present.
	// Python L253.
	GuidedViewNative bool

	// RegionMagnification is true when yj_has_text_popups or yj_publisher_panels (non-zero)
	// capability is present. Controls magnification region processing.
	// Python L255-257, L265-266.
	RegionMagnification bool

	// IsPDFBacked is true when yj_fixed_layout==2 or ==3, or yj_textbook capability is present.
	// Python L260, L263, L272.
	IsPDFBacked bool

	// IsPrintReplica is true when yj_fixed_layout==2 or yj_textbook (and not fixed_layout==3).
	// Python L260, L272.
	IsPrintReplica bool

	// IsPDFBackedFixedLayout is true when yj_fixed_layout==3.
	// Python L263.
	IsPDFBackedFixedLayout bool

	// VirtualPanelsAllowed is true for comic books with continuous_popup_progression or
	// yj_publisher_panels==0 or yj_fixed_layout==3.
	// Python L243, L265, L264.
	VirtualPanelsAllowed bool

	// HTMLCover is true when yj_illustrated_layout is present.
	// Python L274-275: self.illustrated_layout = self.html_cover = True.
	HTMLCover bool

	// IsScribeNotebook is true when the book is a Kindle Scribe notebook.
	// Python (yj_book.py L38): self.is_scribe_notebook = False (set True by kpf_container.py L150/163
	// when ACTION_FRAGMENTS_SCHEMA or DELTA_FRAGMENTS_SCHEMA is found).
	// Go detects this from nmdl.template_id in document_data (yj_to_epub_metadata.py L87)
	// or from nmdl.* section keys during content processing.
	// Used by navigation to suppress warnings for missing reading order nav data
	// (yj_to_epub_navigation.py L107).
	IsScribeNotebook bool

	// IsKpfPrepub is true when the book was loaded from a KPF container (Kindle Previewer export)
	// that does not have final metadata (no ASIN, asset_id, cde_content_type, or content_id).
	// Python sets this in kpf_container.py L216, then clears it at L223 if those metadata keys exist.
	// Go processes KFX containers directly (not KPF), so this is always false in the normal pipeline.
	// Design difference: Python separates raw_fonts ($418) from raw_media ($417) and uses is_kpf_prepub
	// to fall back to raw_media for font lookup. Go stores both in a unified RawFragments map, so
	// the fallback is implicit — raw[location] finds fonts from either source.
	// Port of Python self.is_kpf_prepub (kpf_container.py L216-223, yj_to_epub_resources.py L297-298).
	IsKpfPrepub bool

	// GenerateEpub2 is true when the book content allows EPUB2 output.
	// Set by checkEpubVersion after rendering. When false, EPUB3 features are used.
	// Python: self.generate_epub2 (epub_output.py L268, L654-684).
	GenerateEpub2 bool
}

type renderedStoryline struct {
	Root             *htmlElement
	BodyHTML         string
	BodyClass        string
	BodyStyle        string
	BodyStyleInferred bool
	Properties       string
}

type renderedSection struct {
	Filename       string
	Title          string
	PageTitle      string
	Language       string
	BodyLanguage   string // xml:lang for <body> element (from -kfx-attrib-xml-lang, may differ from Language)
	BodyClass      string
	BodyStyle      string
	BodyStyleInferred bool // true if body style was inferred from children (not from content rendering)
	Paragraphs     []string
	Properties     string
	Root           *htmlElement
}

type resourceFragment struct {
	ID        string
	Location  string
	MediaType string
	Format    string   // $161 — resource format symbol (e.g. "jpg" for JPEG)
	Width     int      // $422 (or $66)
	Height    int      // $423 (or $67)
	Variants  []string // $635 — list of variant resource IDs
}

type fontFragment struct {
	Location string
	Family   string
	Style    string
	Weight   string
	Stretch  string
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
	RawValue           map[string]interface{} // Original section fragment data for check_cover_section_and_storyline (Python yj_metadata.py L648-791).
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
	Offset     int
	URI        string
}

type navTarget struct {
	PositionID int
	Offset     int
}

type navPoint struct {
	Title       string
	Target      navTarget
	Children    []navPoint
	Description string // NCX mbp:meta name="description" (Python: toc_entry.description)
	Icon        string // NCX mbp:meta-img name="mastheadImage" (Python: toc_entry.icon)
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

func Classify(path string) (openMode string, blockReason string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}

	if bytes.HasPrefix(data, drmionSignature) {
		return "drm", "", nil
	}

	if !bytes.HasPrefix(data, contSignature) && !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return "blocked", "unsupported_kfx_layout", nil
	}

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




