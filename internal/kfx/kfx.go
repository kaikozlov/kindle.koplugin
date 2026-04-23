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
	HasConditionalContent    bool // set during content rendering when conditional page templates are found
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
	// DefaultFontFamily is the resolved font-family from document metadata ($538),
	// used to resolve "default" font names in KFX data. Port of Python
	// KFX_EPUB_Properties.default_font_family (yj_to_epub_metadata.py L110).
	DefaultFontFamily string
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




