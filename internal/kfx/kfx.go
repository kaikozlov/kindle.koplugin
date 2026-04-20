package kfx

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"log"
	"os"
	"path/filepath"
	"strings"

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

// convertFromCONTData takes raw CONT KFX data and converts it to EPUB.
// This is used after DRMION decryption produces a valid CONT container.
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

	book, err := renderBookState(state)
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

	book, err := renderBookState(state)
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
