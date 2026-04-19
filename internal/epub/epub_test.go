package epub

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ---------- helpers ----------------------------------------------------------

// writeBookToTemp writes a minimal EPUB to a temp file and returns its path.
func writeBookToTemp(t *testing.T, book Book) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.epub")
	if err := Write(path, book); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	return path
}

// readZIP reads an EPUB zip and returns a map of filename → content.
func readZIP(t *testing.T, path string) map[string][]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("stat zip: %v", err)
	}
	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		t.Fatalf("zip NewReader: %v", err)
	}
	m := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		m[f.Name] = data
	}
	return m
}

// readZIPHeaders reads the zip file headers to check compression method.
func readZIPHeaders(t *testing.T, path string) []zip.FileHeader {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("stat zip: %v", err)
	}
	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		t.Fatalf("zip NewReader: %v", err)
	}
	headers := make([]zip.FileHeader, len(zr.File))
	for i, f := range zr.File {
		headers[i] = f.FileHeader
	}
	return headers
}

// xmlName is a helper struct for parsing XML elements.
type xmlName struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
}

// parseXML is a minimal XML parser that returns the root element.
func parseXML(t *testing.T, data []byte) *xml.Name {
	t.Helper()
	var name xmlName
	if err := xml.Unmarshal(data, &name); err != nil {
		// Try wrapping in a root element for fragments
		wrapped := "<root>" + string(data) + "</root>"
		if err2 := xml.Unmarshal([]byte(wrapped), &name); err2 != nil {
			t.Fatalf("XML parse error: %v (original: %v)", err2, err)
		}
	}
	return &name.XMLName
}

// extractXMLAttr extracts an attribute value from an XML element string.
func extractXMLAttr(element, attrName string) string {
	// Find the element
	re := regexp.MustCompile(regexp.QuoteMeta(attrName) + `="([^"]*)"`)
	matches := re.FindStringSubmatch(element)
	if len(matches) > 1 {
		return matches[1]
	}
	// Also try single quotes
	re = regexp.MustCompile(regexp.QuoteMeta(attrName) + `='([^']*)'`)
	matches = re.FindStringSubmatch(element)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// extractAllMatches extracts all occurrences of a regex pattern.
func extractAllMatches(data, pattern string) []string {
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(data, -1)
	results := make([]string, len(matches))
	for i, m := range matches {
		results[i] = m[1]
	}
	return results
}

// extractFirstMatch extracts the first match of a regex group.
func extractFirstMatch(data, pattern string) string {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(data)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// sampleBook creates a sample book for testing.
func sampleBook() Book {
	return Book{
		Identifier: "urn:uuid:test-id-123",
		Title:      "Test Book",
		Language:   "en",
		Authors:    []string{"Author One", "Author Two"},
		Published:  "2024-01-15",
		Modified:   "2024-01-15T10:30:00Z",
		Sections: []Section{
			{Filename: "charlie.xhtml", Title: "Chapter C", BodyHTML: "<p>C</p>"},
			{Filename: "alpha.xhtml", Title: "Chapter A", BodyHTML: "<p>A</p>"},
			{Filename: "bravo.xhtml", Title: "Chapter B", BodyHTML: "<p>B</p>"},
		},
		Resources: []Resource{
			{Filename: "image.png", MediaType: "image/png", Data: []byte("fake-png")},
		},
		Navigation: []NavPoint{
			{Title: "Chapter C", Href: "charlie.xhtml"},
			{Title: "Chapter A", Href: "alpha.xhtml"},
			{Title: "Chapter B", Href: "bravo.xhtml"},
		},
	}
}

// =========================================================================== //
// VAL-B-001: Manifest items are sorted alphabetically by filename
// =========================================================================== //

func TestManifestOrdering(t *testing.T) {
	book := sampleBook()
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)

	opfData := string(files["OEBPS/content.opf"])
	// Extract manifest item hrefs in order
	hrefs := extractAllMatches(opfData, `<item[^>]*href="([^"]*)"[^>]*>`)

	// Verify alphabetical ordering
	sorted := make([]string, len(hrefs))
	copy(sorted, hrefs)
	sort.Strings(sorted)

	for i, got := range hrefs {
		want := sorted[i]
		if got != want {
			t.Errorf("manifest item %d: got href %q, want %q (alphabetical)", i, got, want)
		}
	}

	// Verify our specific test files appear in alphabetical order
	// alpha.xhtml < bravo.xhtml < charlie.xhtml < image.png < nav.xhtml < stylesheet... < toc.ncx
	var sectionFiles []string
	for _, h := range hrefs {
		if strings.HasSuffix(h, ".xhtml") && h != "nav.xhtml" {
			sectionFiles = append(sectionFiles, h)
		}
	}
	expected := []string{"alpha.xhtml", "bravo.xhtml", "charlie.xhtml"}
	if len(sectionFiles) != len(expected) {
		t.Fatalf("section files: got %v, want %v", sectionFiles, expected)
	}
	for i, got := range sectionFiles {
		if got != expected[i] {
			t.Errorf("section file %d: got %q, want %q", i, got, expected[i])
		}
	}
}

// =========================================================================== //
// VAL-B-002: Spine itemref order matches section input order
// =========================================================================== //

func TestSpineOrder(t *testing.T) {
	book := sampleBook()
	// Sections are in order: charlie, alpha, bravo (not alphabetical)
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)

	opfData := string(files["OEBPS/content.opf"])

	// Extract spine itemref idrefs in order
	idrefs := extractAllMatches(opfData, `<itemref[^>]*idref="([^"]*)"[^>]*/>`)

	// Must match input section order: charlie → alpha → bravo
	expected := []string{"charlie.xhtml", "alpha.xhtml", "bravo.xhtml"}
	if len(idrefs) != len(expected) {
		t.Fatalf("spine idrefs: got %v (count %d), want %v (count %d)", idrefs, len(idrefs), expected, len(expected))
	}
	for i, got := range idrefs {
		if got != expected[i] {
			t.Errorf("spine itemref %d: got %q, want %q", i, got, expected[i])
		}
	}
}

// =========================================================================== //
// VAL-B-003: OPF metadata — dc:identifier, dc:title, dc:language with defaults
// =========================================================================== //

func TestOPFMetadata(t *testing.T) {
	book := sampleBook()
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	// dc:identifier
	identifier := extractFirstMatch(opfData, `<dc:identifier[^>]*id="bookid"[^>]*>([^<]+)</dc:identifier>`)
	if identifier != book.Identifier {
		t.Errorf("dc:identifier: got %q, want %q", identifier, book.Identifier)
	}

	// dc:title
	title := extractFirstMatch(opfData, `<dc:title>([^<]+)</dc:title>`)
	if title != book.Title {
		t.Errorf("dc:title: got %q, want %q", title, book.Title)
	}

	// dc:language
	lang := extractFirstMatch(opfData, `<dc:language>([^<]+)</dc:language>`)
	if lang != book.Language {
		t.Errorf("dc:language: got %q, want %q", lang, book.Language)
	}
}

func TestOPFMetadataDefaults(t *testing.T) {
	// Empty Title → "Unknown" (Python: epub_output.py:426-427)
	book := Book{
		Identifier: "urn:uuid:default-test",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	title := extractFirstMatch(opfData, `<dc:title>([^<]+)</dc:title>`)
	if title != "Unknown" {
		t.Errorf("default title: got %q, want %q", title, "Unknown")
	}

	lang := extractFirstMatch(opfData, `<dc:language>([^<]+)</dc:language>`)
	if lang != "en" {
		t.Errorf("default language: got %q, want %q", lang, "en")
	}
}

// =========================================================================== //
// VAL-B-004: OPF metadata — dc:creator with refines
// =========================================================================== //

func TestOPFCreatorWithRefines(t *testing.T) {
	book := sampleBook()
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	// Check both creators
	for i, author := range book.Authors {
		id := fmt.Sprintf("creator%d", i)
		// dc:creator element
		creatorPattern := fmt.Sprintf(`<dc:creator[^>]*id="%s"[^>]*>([^<]+)</dc:creator>`, id)
		creatorText := extractFirstMatch(opfData, creatorPattern)
		if creatorText != author {
			t.Errorf("creator %d: got %q, want %q", i, creatorText, author)
		}
		// refines meta
		refinesPattern := fmt.Sprintf(`<meta[^>]*refines="#%s"[^>]*property="role"[^>]*scheme="marc:relators"[^>]*>([^<]*)</meta>`, id)
		roleText := extractFirstMatch(opfData, refinesPattern)
		if roleText != "aut" {
			t.Errorf("creator %d role: got %q, want %q", i, roleText, "aut")
		}
	}
}

// =========================================================================== //
// VAL-B-005: OPF metadata — dc:publisher, dc:description, dc:date
// =========================================================================== //

func TestOPFOptionalMetadata(t *testing.T) {
	book := Book{
		Identifier: "urn:uuid:opt-test",
		Title:      "Optional Test",
		Language:   "en",
		Authors:    []string{"A"},
		Publisher:  "Test Publisher",
		Description: "A test description",
		Published:  "2024-06-15",
		Modified:   "2024-06-15T12:00:00Z",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	// dc:publisher
	pub := extractFirstMatch(opfData, `<dc:publisher>([^<]+)</dc:publisher>`)
	if pub != book.Publisher {
		t.Errorf("dc:publisher: got %q, want %q", pub, book.Publisher)
	}

	// dc:description
	desc := extractFirstMatch(opfData, `<dc:description>([^<]+)</dc:description>`)
	if desc != book.Description {
		t.Errorf("dc:description: got %q, want %q", desc, book.Description)
	}

	// dc:date
	date := extractFirstMatch(opfData, `<dc:date>([^<]+)</dc:date>`)
	if date != book.Published {
		t.Errorf("dc:date: got %q, want %q", date, book.Published)
	}
}

func TestOPFOptionalMetadataAbsentWhenEmpty(t *testing.T) {
	book := Book{
		Identifier: "urn:uuid:absent-test",
		Title:      "Absent Test",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	if strings.Contains(opfData, "<dc:publisher>") {
		t.Error("dc:publisher should be absent when Publisher is empty")
	}
	if strings.Contains(opfData, "<dc:description>") {
		t.Error("dc:description should be absent when Description is empty")
	}
	if strings.Contains(opfData, "<dc:date>") {
		t.Error("dc:date should be absent when Published is empty")
	}
}

// =========================================================================== //
// VAL-B-006: OPF metadata — dcterms:modified timestamp
// =========================================================================== //

func TestOPFModifiedTimestamp(t *testing.T) {
	// With explicit value
	book := Book{
		Identifier: "urn:uuid:mod-test",
		Title:      "T",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-03-20T14:30:00Z",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	modified := extractFirstMatch(opfData, `<meta[^>]*property="dcterms:modified"[^>]*>([^<]+)</meta>`)
	if modified != "2024-03-20T14:30:00Z" {
		t.Errorf("explicit modified: got %q, want %q", modified, "2024-03-20T14:30:00Z")
	}
}

func TestOPFModifiedTimestampAutoGenerated(t *testing.T) {
	// Without explicit value — should auto-generate UTC ISO 8601
	book := Book{
		Identifier: "urn:uuid:auto-mod-test",
		Title:      "T",
		Language:   "en",
		Authors:    []string{"A"},
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	modified := extractFirstMatch(opfData, `<meta[^>]*property="dcterms:modified"[^>]*>([^<]+)</meta>`)
	if modified == "" {
		t.Fatal("auto-generated modified should not be empty")
	}
	// Should match YYYY-MM-DDTHH:MM:SSZ pattern
	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)
	if !re.MatchString(modified) {
		t.Errorf("auto-generated modified %q doesn't match ISO 8601 pattern", modified)
	}
}

// =========================================================================== //
// VAL-B-007: Cover image manifest entry with cover meta
// =========================================================================== //

func TestCoverImageManifestAndMeta(t *testing.T) {
	book := Book{
		Identifier:     "urn:uuid:cover-test",
		Title:          "Cover Test",
		Language:       "en",
		Authors:        []string{"A"},
		Modified:       "2024-01-01T00:00:00Z",
		CoverImageHref: "cover.jpg",
		Resources: []Resource{
			{Filename: "cover.jpg", MediaType: "image/jpeg", Data: []byte("fake-jpeg"), Properties: "cover-image"},
		},
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	// Cover meta should exist
	coverMeta := extractFirstMatch(opfData, `<meta[^>]*name="cover"[^>]*content="([^"]*)"`)
	if coverMeta == "" {
		t.Fatal("cover meta element not found")
	}

	// The cover meta content should reference the manifest item for cover.jpg
	// Verify that the manifest item for cover.jpg exists with the same id
	coverItemID := extractFirstMatch(opfData, `<item[^>]*href="cover\.jpg"[^>]*id="([^"]*)"`)
	if coverItemID == "" {
		t.Fatal("manifest item for cover.jpg not found")
	}
	if coverMeta != coverItemID {
		t.Errorf("cover meta content %q doesn't match manifest item id %q", coverMeta, coverItemID)
	}

	// Cover resource should have cover-image properties
	coverProps := extractFirstMatch(opfData, `<item[^>]*href="cover\.jpg"[^>]*properties="([^"]*)"`)
	if !strings.Contains(coverProps, "cover-image") {
		t.Errorf("cover item properties %q should contain cover-image", coverProps)
	}
}

// =========================================================================== //
// VAL-B-008: Override-Kindle-Fonts meta
// =========================================================================== //

func TestOverrideKindleFontsMeta(t *testing.T) {
	book := Book{
		Identifier:          "urn:uuid:kf-test",
		Title:               "KF Test",
		Language:            "en",
		Authors:             []string{"A"},
		Modified:            "2024-01-01T00:00:00Z",
		OverrideKindleFonts: true,
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	if !strings.Contains(opfData, `name="Override-Kindle-Fonts"`) {
		t.Error("Override-Kindle-Fonts meta should be present when OverrideKindleFonts=true")
	}
	if !strings.Contains(opfData, `content="true"`) || !strings.Contains(opfData, `Override-Kindle-Fonts`) {
		t.Error("Override-Kindle-Fonts meta should have content=true")
	}
}

func TestOverrideKindleFontsMetaAbsent(t *testing.T) {
	book := Book{
		Identifier:          "urn:uuid:no-kf-test",
		Title:               "No KF Test",
		Language:            "en",
		Authors:             []string{"A"},
		Modified:            "2024-01-01T00:00:00Z",
		OverrideKindleFonts: false,
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	if strings.Contains(opfData, "Override-Kindle-Fonts") {
		t.Error("Override-Kindle-Fonts meta should be absent when OverrideKindleFonts=false")
	}
}

// =========================================================================== //
// VAL-B-009: NCX structure — navMap with sequential nav IDs
// =========================================================================== //

func TestNCXSequentialNavIDs(t *testing.T) {
	book := Book{
		Identifier: "urn:uuid:ncx-test",
		Title:      "NCX Test",
		Language:   "en",
		Authors:    []string{"Author"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "s1.xhtml", Title: "Ch1", BodyHTML: "<p>1</p>"},
			{Filename: "s2.xhtml", Title: "Ch2", BodyHTML: "<p>2</p>"},
		},
		Navigation: []NavPoint{
			{Title: "Ch1", Href: "s1.xhtml", Children: []NavPoint{
				{Title: "Sub1", Href: "s1.xhtml#sub1"},
			}},
			{Title: "Ch2", Href: "s2.xhtml"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)

	ncxData := string(files["OEBPS/toc.ncx"])

	// Check NCX version attribute
	if !strings.Contains(ncxData, `version="2005-1"`) {
		t.Error("NCX should have version='2005-1'")
	}

	// Check namespace
	if !strings.Contains(ncxData, `xmlns="http://www.daisy.org/z3986/2005/ncx/"`) {
		t.Error("NCX should have NCX namespace")
	}

	// Check dtb:uid
	uid := extractFirstMatch(ncxData, `<meta[^>]*name="dtb:uid"[^>]*content="([^"]*)"`)
	if uid != book.Identifier {
		t.Errorf("dtb:uid: got %q, want %q", uid, book.Identifier)
	}

	// Check docTitle
	docTitle := extractFirstMatch(ncxData, `<docTitle>\s*<text>([^<]+)</text>`)
	if docTitle != book.Title {
		t.Errorf("docTitle: got %q, want %q", docTitle, book.Title)
	}

	// Check docAuthor
	docAuthor := extractFirstMatch(ncxData, `<docAuthor>\s*<text>([^<]+)</text>`)
	if docAuthor != "Author" {
		t.Errorf("docAuthor: got %q, want %q", docAuthor, "Author")
	}

	// Check navPoint IDs are sequential (depth-first)
	navIDs := extractAllMatches(ncxData, `<navPoint[^>]*id="(nav\d+)"`)
	expectedIDs := []string{"nav0", "nav1", "nav2"}
	if len(navIDs) != len(expectedIDs) {
		t.Fatalf("nav IDs: got %v (count %d), want %v (count %d)", navIDs, len(navIDs), expectedIDs, len(expectedIDs))
	}
	for i, got := range navIDs {
		if got != expectedIDs[i] {
			t.Errorf("nav ID %d: got %q, want %q", i, got, expectedIDs[i])
		}
	}
}

// =========================================================================== //
// VAL-B-010: NCX page list with page types
// =========================================================================== //

func TestNCXPageList(t *testing.T) {
	book := Book{
		Identifier: "urn:uuid:pg-test",
		Title:      "Page Test",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
		PageList: []PageTarget{
			{Label: "i", Href: "sec.xhtml#pi"},
			{Label: "42", Href: "sec.xhtml#p42"},
			{Label: "A", Href: "sec.xhtml#pA"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	ncxData := string(files["OEBPS/toc.ncx"])

	// Check pageList exists
	if !strings.Contains(ncxData, "<pageList>") {
		t.Fatal("NCX should contain <pageList>")
	}

	// Extract pageTarget entries with their type and value
	types := extractAllMatches(ncxData, `<pageTarget[^>]*type="([^"]*)"`)
	values := extractAllMatches(ncxData, `<pageTarget[^>]*value="([^"]*)"`)

	// "i" → type="front", value="1"
	// "42" → type="normal", value="42"
	// "A" → type="special"
	expectedTypes := []string{"front", "normal", "special"}
	if len(types) != len(expectedTypes) {
		t.Fatalf("page types: got %v, want %v", types, expectedTypes)
	}
	for i, got := range types {
		if got != expectedTypes[i] {
			t.Errorf("page type %d: got %q, want %q", i, got, expectedTypes[i])
		}
	}

	// Values: "1" (roman i=1), "42"
	expectedValues := []string{"1", "42"}
	if len(values) != len(expectedValues) {
		t.Fatalf("page values: got %v, want %v", values, expectedValues)
	}
	for i, got := range values {
		if got != expectedValues[i] {
			t.Errorf("page value %d: got %q, want %q", i, got, expectedValues[i])
		}
	}
}

// =========================================================================== //
// VAL-B-011: EPUB3 nav document — toc, landmarks, page-list sections
// =========================================================================== //

func TestEpub3Nav(t *testing.T) {
	book := Book{
		Identifier: "urn:uuid:nav-test",
		Title:      "Nav Test",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
		Navigation: []NavPoint{
			{Title: "Chapter 1", Href: "sec.xhtml"},
		},
		Guide: []GuideEntry{
			{Type: "toc", Title: "TOC", Href: "sec.xhtml"},
		},
		PageList: []PageTarget{
			{Label: "1", Href: "sec.xhtml#p1"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	navData := string(files["OEBPS/nav.xhtml"])

	// Check toc nav
	if !strings.Contains(navData, `epub:type="toc"`) {
		t.Error("nav document should contain toc nav section")
	}
	if !strings.Contains(navData, "Table of contents") {
		t.Error("toc nav should have 'Table of contents' heading")
	}

	// Check landmarks nav (present because Guide is non-empty)
	if !strings.Contains(navData, `epub:type="landmarks"`) {
		t.Error("nav document should contain landmarks nav section")
	}

	// Check page-list nav (present because PageList is non-empty)
	if !strings.Contains(navData, `epub:type="page-list"`) {
		t.Error("nav document should contain page-list nav section")
	}
}

func TestEpub3NavNoLandmarksWhenNoGuide(t *testing.T) {
	book := Book{
		Identifier: "urn:uuid:nav-noguide",
		Title:      "Nav NoGuide",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
		Navigation: []NavPoint{
			{Title: "Ch1", Href: "sec.xhtml"},
		},
		// Guide and PageList are empty
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	navData := string(files["OEBPS/nav.xhtml"])

	if strings.Contains(navData, `epub:type="landmarks"`) {
		t.Error("landmarks should be absent when Guide is empty")
	}
	if strings.Contains(navData, `epub:type="page-list"`) {
		t.Error("page-list should be absent when PageList is empty")
	}
}

// =========================================================================== //
// VAL-B-012: Guide type mapping in EPUB3 nav landmarks
// =========================================================================== //

func TestGuideTypeMappingInNav(t *testing.T) {
	book := Book{
		Identifier: "urn:uuid:guide-map",
		Title:      "Guide Map",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
		Guide: []GuideEntry{
			{Type: "cover", Title: "Cover", Href: "sec.xhtml"},
			{Type: "text", Title: "Beginning", Href: "sec.xhtml"},
			{Type: "toc", Title: "Table of Contents", Href: "sec.xhtml"},
			{Type: "custom", Title: "Custom", Href: "sec.xhtml"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	navData := string(files["OEBPS/nav.xhtml"])

	// EPUB3_VOCABULARY_OF_GUIDE_TYPE mapping:
	// cover → cover, text → bodymatter, toc → toc
	expectedMappings := map[string]string{
		"cover":  "cover",
		"text":   "bodymatter",
		"toc":    "toc",
		"custom": "custom",
	}
	for guideType, epubType := range expectedMappings {
		// Find the entry with the guide type
		// The nav should have epub:type="mapped_type"
		// We need to find the anchor that corresponds to this guide entry
		pattern := fmt.Sprintf(`epub:type="%s"`, epubType)
		if !strings.Contains(navData, pattern) {
			t.Errorf("guide type %q should map to epub:type %q in nav", guideType, epubType)
		}
	}
}

// =========================================================================== //
// VAL-B-013: container.xml structure
// =========================================================================== //

func TestContainerXML(t *testing.T) {
	expected := xmlDecl +
		`<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">` + "\n" +
		`  <rootfiles>` + "\n" +
		`    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>` + "\n" +
		`  </rootfiles>` + "\n" +
		`</container>` + "\n"

	if containerXML != expected {
		t.Errorf("containerXML mismatch:\ngot:\n%s\nwant:\n%s", containerXML, expected)
	}
}

// =========================================================================== //
// VAL-B-014: ZIP packaging — mimetype first and stored (not deflated)
// =========================================================================== //

func TestZIPMimetypeFirstStored(t *testing.T) {
	book := sampleBook()
	path := writeBookToTemp(t, book)
	headers := readZIPHeaders(t, path)

	if len(headers) == 0 {
		t.Fatal("zip should have at least one file")
	}

	// First entry must be "mimetype"
	if headers[0].Name != "mimetype" {
		t.Errorf("first zip entry: got %q, want %q", headers[0].Name, "mimetype")
	}

	// mimetype must be ZIP_STORED (not deflated)
	if headers[0].Method != zip.Store {
		t.Errorf("mimetype compression: got method %d (deflated), want %d (stored)", headers[0].Method, zip.Store)
	}

	// Verify remaining entries are deflated and sorted alphabetically
	var remaining []string
	for i, h := range headers {
		if i == 0 {
			continue // skip mimetype
		}
		remaining = append(remaining, h.Name)
		if h.Method != zip.Deflate {
			t.Errorf("file %q should be deflated, got method %d", h.Name, h.Method)
		}
	}
	sorted := make([]string, len(remaining))
	copy(sorted, remaining)
	sort.Strings(sorted)
	for i, got := range remaining {
		if got != sorted[i] {
			t.Errorf("zip entry order %d: got %q, want %q (alphabetical)", i, got, sorted[i])
		}
	}
}

func TestZIPMimetypeContent(t *testing.T) {
	book := sampleBook()
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)

	content := string(files["mimetype"])
	if content != "application/epub+zip" {
		t.Errorf("mimetype content: got %q, want %q", content, "application/epub+zip")
	}
}

func TestZIPContainerXML(t *testing.T) {
	book := sampleBook()
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)

	if _, ok := files["META-INF/container.xml"]; !ok {
		t.Error("zip should contain META-INF/container.xml")
	}
}

func TestZIPOEBPSPrefix(t *testing.T) {
	book := sampleBook()
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)

	for name := range files {
		if name == "mimetype" || strings.HasPrefix(name, "META-INF/") {
			continue
		}
		if !strings.HasPrefix(name, "OEBPS/") {
			t.Errorf("content file %q should have OEBPS/ prefix", name)
		}
	}
}

// =========================================================================== //
// VAL-B-015: Manifest ID generation — basename truncation and sanitization
// =========================================================================== //

func TestMakeManifestID(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{"simple", "OEBPS/part0001.xhtml", "part0001.xhtml"},
		{"long_truncate", strings.Repeat("a", 100) + ".xhtml", strings.Repeat("a", 64)},
		{"numeric_prefix", "123start.xhtml", "id_123start.xhtml"},
		{"special_chars", "my file (1).xhtml", "my_file__1_.xhtml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			used := map[string]bool{}
			got := makeManifestID(tt.filename, used)
			if got != tt.want {
				t.Errorf("makeManifestID(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestMakeManifestIDDedup(t *testing.T) {
	used := map[string]bool{}
	id1 := makeManifestID("test.xhtml", used)
	id2 := makeManifestID("test.xhtml", used)
	if id1 == id2 {
		t.Errorf("duplicate IDs should be de-duplicated: got %q twice", id1)
	}
	// First should be "test.xhtml", second should have suffix
	if id1 != "test.xhtml" {
		t.Errorf("first ID: got %q, want %q", id1, "test.xhtml")
	}
	if !strings.HasPrefix(id2, "test.xhtml_") {
		t.Errorf("second ID: got %q, should have suffix", id2)
	}
}

// =========================================================================== //
// VAL-B-016: Section XHTML serialization with language and title
// =========================================================================== //

func TestSectionXHTMLSerialization(t *testing.T) {
	book := Book{
		Title:      "Book Title",
		Language:   "en",
		Stylesheet: "body { margin: 0; }",
	}
	section := Section{
		Filename:  "test.xhtml",
		Language:  "fr",
		PageTitle: "My Page",
		BodyHTML:  "<p>test</p>",
	}
	xhtml := sectionXHTML(book, section)

	// xml:lang from section language
	if !strings.Contains(xhtml, `xml:lang="fr"`) {
		t.Error("section XHTML should have xml:lang from section language")
	}

	// <title> from PageTitle
	if !strings.Contains(xhtml, "<title>My Page</title>") {
		t.Error("section XHTML should have title from PageTitle")
	}

	// Stylesheet link
	if !strings.Contains(xhtml, `<link rel="stylesheet"`) {
		t.Error("section XHTML should have stylesheet link when book has stylesheet")
	}

	// Body content
	if !strings.Contains(xhtml, "<p>test</p>") {
		t.Error("section XHTML should contain body HTML")
	}
}

func TestSectionXHTMLTitleFallback(t *testing.T) {
	book := Book{
		Title:    "Book Title",
		Language: "en",
	}
	// No PageTitle, no Title → should use basename of filename
	section := Section{
		Filename: "chapter1.xhtml",
		BodyHTML: "<p>content</p>",
	}
	xhtml := sectionXHTML(book, section)
	if !strings.Contains(xhtml, "<title>chapter1</title>") {
		t.Error("section XHTML should use filename base as title fallback")
	}
}

func TestSectionXHTMLLanguageFallback(t *testing.T) {
	book := Book{
		Language: "de",
	}
	section := Section{
		Filename: "sec.xhtml",
		BodyHTML: "<p>content</p>",
	}
	xhtml := sectionXHTML(book, section)
	if !strings.Contains(xhtml, `xml:lang="de"`) {
		t.Error("section XHTML should fall back to book language when section language is empty")
	}
}

// =========================================================================== //
// VAL-B-017: Default navigation from sections when Navigation is empty
// =========================================================================== //

func TestDefaultNavigationFromSections(t *testing.T) {
	book := Book{
		Identifier: "urn:uuid:default-nav",
		Title:      "Default Nav",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "ch1.xhtml", Title: "Chapter One", BodyHTML: "<p>1</p>"},
			{Filename: "ch2.xhtml", Title: "Chapter Two", BodyHTML: "<p>2</p>"},
			{Filename: "ch3.xhtml", Title: "Chapter Three", BodyHTML: "<p>3</p>"},
		},
		// Navigation is empty — should auto-generate
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)

	// Check NCX has 3 navPoints matching section titles
	ncxData := string(files["OEBPS/toc.ncx"])
	tocLabels := extractAllMatches(ncxData, `<navPoint[^>]*>\s*<navLabel>\s*<text>([^<]+)</text>`)

	expected := []string{"Chapter One", "Chapter Two", "Chapter Three"}
	if len(tocLabels) != len(expected) {
		t.Fatalf("nav labels: got %v (count %d), want %v", tocLabels, len(tocLabels), expected)
	}
	for i, got := range tocLabels {
		if got != expected[i] {
			t.Errorf("nav label %d: got %q, want %q", i, got, expected[i])
		}
	}
}

// =========================================================================== //
// VAL-B-018: Default title when empty
// =========================================================================== //

func TestDefaultTitleWhenEmpty(t *testing.T) {
	book := Book{
		Identifier: "urn:uuid:empty-title",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	title := extractFirstMatch(opfData, `<dc:title>([^<]+)</dc:title>`)
	if title != "Unknown" {
		t.Errorf("empty title: got %q, want %q", title, "Unknown")
	}
}

// =========================================================================== //
// Additional edge case / integration tests
// =========================================================================== //

func TestNavXHTMLStructure(t *testing.T) {
	book := sampleBook()
	navHTML := navXHTML(book)

	// Must be valid XML with xhtml namespace
	if !strings.Contains(navHTML, `xmlns="http://www.w3.org/1999/xhtml"`) {
		t.Error("nav should have XHTML namespace")
	}
	if !strings.Contains(navHTML, `xmlns:epub="http://www.idpf.org/2007/ops"`) {
		t.Error("nav should have EPUB namespace")
	}
}

func TestContentOPFHasEPUB3Version(t *testing.T) {
	book := sampleBook()
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	if !strings.Contains(opfData, `version="3.0"`) {
		t.Error("OPF should have version 3.0")
	}
	if !strings.Contains(opfData, `unique-identifier="bookid"`) {
		t.Error("OPF should have unique-identifier=bookid")
	}
}

func TestNavPointListHTMLNested(t *testing.T) {
	points := []NavPoint{
		{Title: "Parent", Href: "parent.xhtml", Children: []NavPoint{
			{Title: "Child1", Href: "parent.xhtml#c1"},
			{Title: "Child2", Href: "parent.xhtml#c2"},
		}},
	}
	html := navPointListHTML(points)

	if !strings.Contains(html, `<ol>`) {
		t.Error("nested nav should contain <ol>")
	}
	// Should have nested structure: <li>...<ol>...</ol>...</li>
	outerLi := strings.Count(html, "<li>")
	if outerLi != 3 {
		t.Errorf("expected 3 <li> elements (1 parent + 2 children), got %d", outerLi)
	}
}

func TestRomanToInt(t *testing.T) {
	tests := []struct {
		input string
		want  int
		ok    bool
	}{
		{"i", 1, true},
		{"ii", 2, true},
		{"iv", 4, true},
		{"v", 5, true},
		{"ix", 9, true},
		{"x", 10, true},
		{"xl", 40, true},
		{"l", 50, true},
		{"c", 100, true},
		{"d", 500, true},
		{"m", 1000, true},
		{"xx", 20, true},
		{"", 0, false},
		{"abc", 0, false},
		{"I", 1, true}, // uppercase
	}
	for _, tt := range tests {
		got, ok := romanToInt(tt.input)
		if ok != tt.ok {
			t.Errorf("romanToInt(%q): ok=%v, want %v", tt.input, ok, tt.ok)
		}
		if got != tt.want {
			t.Errorf("romanToInt(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestNcxPageMetadata(t *testing.T) {
	tests := []struct {
		label    string
		value    string
		pageType string
	}{
		{"42", "42", "normal"},
		{"1", "1", "normal"},
		{"i", "1", "front"},
		{"iv", "4", "front"},
		{"xii", "12", "front"},
		{"A", "", "special"},
		{"", "", ""},
	}
	for _, tt := range tests {
		value, pageType := ncxPageMetadata(tt.label)
		if value != tt.value || pageType != tt.pageType {
			t.Errorf("ncxPageMetadata(%q) = (%q, %q), want (%q, %q)",
				tt.label, value, pageType, tt.value, tt.pageType)
		}
	}
}

func TestEpubTypeForGuide(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"cover", "cover"},
		{"text", "bodymatter"},
		{"toc", "toc"},
		{"custom", "custom"},
		{"srl", "srl"},
	}
	for _, tt := range tests {
		got := epubTypeForGuide(tt.input)
		if got != tt.want {
			t.Errorf("epubTypeForGuide(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFixHTMLID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with spaces", "with_spaces"},
		{"special!@#chars", "special___chars"},
		{"123numeric", "id_123numeric"},
		{"", "id_"},
	}
	for _, tt := range tests {
		got := fixHTMLID(tt.input, false)
		if got != tt.want {
			t.Errorf("fixHTMLID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeManifestID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"part0001.xhtml", "part0001.xhtml"},
		{"my file.xhtml", "my_file.xhtml"},
		{"image(1).png", "image_1_.png"},
		{"test@2x.jpg", "test_2x.jpg"},
	}
	for _, tt := range tests {
		got := sanitizeManifestID(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeManifestID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMakeUniqueName(t *testing.T) {
	seen := map[string]bool{}
	// First call without alwaysSuffix should return root
	got := makeUniqueName("test", seen, "_", false)
	if got != "test" {
		t.Errorf("first unique name: got %q, want %q", got, "test")
	}
	seen["test"] = true
	// Second call with duplicate should add suffix
	got = makeUniqueName("test", seen, "_", false)
	if got == "test" {
		t.Errorf("duplicate should have suffix, got %q", got)
	}
}

func TestSectionBaseTitle(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"chapter1.xhtml", "chapter1"},
		{"path/to/section.xhtml", "section"},
		{"", ""},
	}
	for _, tt := range tests {
		got := sectionBaseTitle(tt.filename)
		if got != tt.want {
			t.Errorf("sectionBaseTitle(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestIsDecimal(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"42", true},
		{"0", true},
		{"123456", true},
		{"", false},
		{"12a", false},
		{"a12", false},
	}
	for _, tt := range tests {
		got := isDecimal(tt.input)
		if got != tt.want {
			t.Errorf("isDecimal(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// =========================================================================== //
// VAL-M1-EPUB-001: Default title for empty books is "Unknown"
// Python: epub_output.py:426-427
// =========================================================================== //

func TestDefaultTitle_Unknown(t *testing.T) {
	book := Book{
		Identifier: "urn:uuid:title-test",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	title := extractFirstMatch(opfData, `<dc:title>([^<]+)</dc:title>`)
	if title != "Unknown" {
		t.Errorf("empty title default: got %q, want %q", title, "Unknown")
	}

	// Also check NCX
	ncxData := string(files["OEBPS/toc.ncx"])
	docTitle := extractFirstMatch(ncxData, `<docTitle>\s*<text>([^<]+)</text>`)
	if docTitle != "Unknown" {
		t.Errorf("NCX docTitle: got %q, want %q", docTitle, "Unknown")
	}
}

// =========================================================================== //
// VAL-M1-EPUB-002: Arabic-Indic numerals converted to ASCII digits in HTML IDs
// Python: epub_output.py:481
// =========================================================================== //

func TestFixHTMLID_ArabicIndic(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"arabic_indic_1", "a\u0661b", "a1b"},
		{"arabic_indic_9", "\u0669", "id_9"},
		{"extended_arabic_indic_0", "\u06f0", "id_0"},
		{"extended_arabic_indic_9", "\u06f9", "id_9"},
		{"mixed_arabic_indic", "sec\u0661\u0662\u0663", "sec123"},
		{"all_zero", "\u0660\u06f0", "id_00"},
		{"preserve_ascii_digits", "abc123", "abc123"},
		{"arabic_after_alpha", "ch\u0667", "ch7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixHTMLID(tt.input, false)
			if got != tt.want {
				t.Errorf("fixHTMLID(%q, false) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// =========================================================================== //
// VAL-M1-EPUB-003: Dots replaced with underscores in illustrated layout HTML IDs
// Python: epub_output.py:479
// =========================================================================== //

func TestFixHTMLID_IllustratedLayoutDots(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		illustratedLayout  bool
		want               string
	}{
		{"dot_illustrated", "sec.1", true, "sec_1"},
		{"dot_not_illustrated", "sec.1", false, "sec.1"},
		{"multiple_dots_illustrated", "a.b.c", true, "a_b_c"},
		{"multiple_dots_not_illustrated", "a.b.c", false, "a.b.c"},
		{"no_dots_illustrated", "sec1", true, "sec1"},
		{"no_dots_not_illustrated", "sec1", false, "sec1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixHTMLID(tt.input, tt.illustratedLayout)
			if got != tt.want {
				t.Errorf("fixHTMLID(%q, %v) = %q, want %q", tt.input, tt.illustratedLayout, got, tt.want)
			}
		})
	}
}

// =========================================================================== //
// VAL-M1-EPUB-004: EPUB version switches from 3 to 2 when no EPUB3 features needed
// Python: epub_output.py:654-683
// =========================================================================== //

func TestEPUBVersionSwitching(t *testing.T) {
	baseBook := Book{
		Identifier: "urn:uuid:version-test",
		Title:      "Version Test",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}

	t.Run("default_epub3", func(t *testing.T) {
		book := baseBook
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])
		if !strings.Contains(opfData, `version="3.0"`) {
			t.Error("default should be EPUB 3.0")
		}
	})

	t.Run("epub2_desired_generates_epub2", func(t *testing.T) {
		book := baseBook
		book.GenerateEpub2 = true
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])
		if !strings.Contains(opfData, `version="2.0"`) {
			t.Error("GenerateEpub2 should produce EPUB 2.0")
		}
	})
}

// =========================================================================== //
// VAL-M1-EPUB-005: Guide section emitted only for EPUB2 or EPUB2-compatible
// Python: epub_output.py:1071
// =========================================================================== //

func TestGuideSectionConditional(t *testing.T) {
	baseBook := Book{
		Identifier: "urn:uuid:guide-test",
		Title:      "Guide Test",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
		Guide: []GuideEntry{
			{Type: "toc", Title: "TOC", Href: "sec.xhtml"},
		},
	}

	t.Run("epub3_no_guide", func(t *testing.T) {
		// Pure EPUB3 with no EPUB2 compatibility → no guide section
		book := baseBook
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])
		if strings.Contains(opfData, "<guide>") {
			t.Error("pure EPUB3 should not have guide section")
		}
	})

	t.Run("epub2_has_guide", func(t *testing.T) {
		book := baseBook
		book.GenerateEpub2 = true
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])
		if !strings.Contains(opfData, "<guide>") {
			t.Error("EPUB2 should have guide section")
		}
	})

	t.Run("epub2_compatible_has_guide", func(t *testing.T) {
		book := baseBook
		book.GenerateEpub2Compatible = true
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])
		if !strings.Contains(opfData, "<guide>") {
			t.Error("EPUB2-compatible should have guide section")
		}
	})

	t.Run("no_guide_entries_no_guide_section", func(t *testing.T) {
		book := baseBook
		book.Guide = nil
		book.GenerateEpub2 = true
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])
		if strings.Contains(opfData, "<guide>") {
			t.Error("no guide entries should not produce guide section even in EPUB2")
		}
	})
}

// =========================================================================== //
// VAL-M1-EPUB-006: NCX includes mbp: namespace for descriptions, masthead, periodical
// Python: epub_output.py:1096-1243
// =========================================================================== //

func TestNCX_MBPNamespace(t *testing.T) {
	t.Run("mbp_namespace_declaration", func(t *testing.T) {
		book := Book{
			Identifier: "urn:uuid:mbp-test",
			Title:      "MBP Test",
			Language:   "en",
			Authors:    []string{"A"},
			Modified:   "2024-01-01T00:00:00Z",
			Sections: []Section{
				{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
			},
			Navigation: []NavPoint{
				{Title: "Ch1", Href: "sec.xhtml"},
			},
		}
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		ncxData := string(files["OEBPS/toc.ncx"])

		// B1-13: NCX must declare mbp: namespace (epub_output.py:1105-1107)
		if !strings.Contains(ncxData, `xmlns:mbp="https://kindlegen.s3.amazonaws.com/AmazonKindlePublishingGuidelines.pdf"`) {
			t.Error("NCX should have mbp: namespace declaration")
		}
	})

	t.Run("mbp_meta_description", func(t *testing.T) {
		book := Book{
			Identifier: "urn:uuid:mbp-desc-test",
			Title:      "MBP Desc Test",
			Language:   "en",
			Authors:    []string{"A"},
			Modified:   "2024-01-01T00:00:00Z",
			Sections: []Section{
				{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
			},
			Navigation: []NavPoint{
				{Title: "Ch1", Href: "sec.xhtml", Description: "Chapter description"},
			},
		}
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		ncxData := string(files["OEBPS/toc.ncx"])

		if !strings.Contains(ncxData, `<mbp:meta name="description">Chapter description</mbp:meta>`) {
			t.Error("NCX should have mbp:meta description element")
		}
	})

	t.Run("mbp_meta_img_masthead", func(t *testing.T) {
		book := Book{
			Identifier: "urn:uuid:mbp-icon-test",
			Title:      "MBP Icon Test",
			Language:   "en",
			Authors:    []string{"A"},
			Modified:   "2024-01-01T00:00:00Z",
			Sections: []Section{
				{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
			},
			Navigation: []NavPoint{
				{Title: "Ch1", Href: "sec.xhtml", Icon: "masthead.jpg"},
			},
		}
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		ncxData := string(files["OEBPS/toc.ncx"])

		if !strings.Contains(ncxData, `<mbp:meta-img name="mastheadImage" src="masthead.jpg"/>`) {
			t.Error("NCX should have mbp:meta-img mastheadImage element")
		}
	})

	t.Run("periodical_ncx_classes", func(t *testing.T) {
		book := Book{
			Identifier: "urn:uuid:periodical-test",
			Title:      "Periodical Test",
			Language:   "en",
			Authors:    []string{"A"},
			Modified:   "2024-01-01T00:00:00Z",
			IsMagazine: true,
			Sections: []Section{
				{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
			},
			Navigation: []NavPoint{
				{Title: "Periodical", Href: "sec.xhtml", Children: []NavPoint{
					{Title: "Section", Href: "sec.xhtml", Children: []NavPoint{
						{Title: "Article", Href: "sec.xhtml"},
					}},
				}},
			},
		}
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		ncxData := string(files["OEBPS/toc.ncx"])

		// Periodical NCX class at depth 0
		if !strings.Contains(ncxData, `class="periodical"`) {
			t.Error("magazine NCX should have periodical class at depth 0")
		}
		// Section class at depth 1
		if !strings.Contains(ncxData, `class="section"`) {
			t.Error("magazine NCX should have section class at depth 1")
		}
		// Article class at depth 2
		if !strings.Contains(ncxData, `class="article"`) {
			t.Error("magazine NCX should have article class at depth 2")
		}
	})

	t.Run("no_periodical_classes_when_not_magazine", func(t *testing.T) {
		book := Book{
			Identifier: "urn:uuid:not-magazine",
			Title:      "Not Magazine",
			Language:   "en",
			Authors:    []string{"A"},
			Modified:   "2024-01-01T00:00:00Z",
			Sections: []Section{
				{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
			},
			Navigation: []NavPoint{
				{Title: "Ch1", Href: "sec.xhtml"},
			},
		}
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		ncxData := string(files["OEBPS/toc.ncx"])

		if strings.Contains(ncxData, `class="periodical"`) {
			t.Error("non-magazine NCX should not have periodical class")
		}
	})
}

// =========================================================================== //
// VAL-M1-EPUB-007: Spine includes page-progression-direction for RTL books
// Python: epub_output.py:1052-1053
// =========================================================================== //

func TestSpinePageProgressionDirection(t *testing.T) {
	baseBook := Book{
		Identifier: "urn:uuid:spine-test",
		Title:      "Spine Test",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}

	t.Run("rtl_spine", func(t *testing.T) {
		book := baseBook
		book.PageProgressionDirection = "rtl"
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])

		if !strings.Contains(opfData, `page-progression-direction="rtl"`) {
			t.Error("RTL book should have page-progression-direction attribute on spine")
		}
	})

	t.Run("ltr_no_attribute", func(t *testing.T) {
		book := baseBook
		book.PageProgressionDirection = "ltr"
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])

		if strings.Contains(opfData, "page-progression-direction") {
			t.Error("LTR book should not have page-progression-direction attribute")
		}
	})

	t.Run("empty_no_attribute", func(t *testing.T) {
		book := baseBook
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])

		if strings.Contains(opfData, "page-progression-direction") {
			t.Error("empty direction should not have page-progression-direction attribute")
		}
	})

	t.Run("rtl_epub2_no_attribute", func(t *testing.T) {
		book := baseBook
		book.PageProgressionDirection = "rtl"
		book.GenerateEpub2 = true
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])

		if strings.Contains(opfData, "page-progression-direction") {
			t.Error("EPUB2 should not have page-progression-direction attribute")
		}
	})
}

// =========================================================================== //
// VAL-M1-EPUB-008: OPF metadata refinements for alternate-script and file-as
// Python: epub_output.py:877-894
// =========================================================================== //

func TestOPFMetadataRefinements(t *testing.T) {
	baseBook := Book{
		Identifier: "urn:uuid:refines-test",
		Title:      "Refines Test",
		Language:   "en",
		Authors:    []string{"Author One", "Author Two"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}

	t.Run("title_pronunciation_refines", func(t *testing.T) {
		book := baseBook
		book.TitlePronunciation = "タイトル"
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])

		// Title should have id="title"
		if !strings.Contains(opfData, `<dc:title id="title">Refines Test</dc:title>`) {
			t.Error("title should have id when pronunciation is set")
		}
		// Should have refines meta
		if !strings.Contains(opfData, `<meta refines="#title" property="alternate-script">タイトル</meta>`) {
			t.Error("should have alternate-script refines for title pronunciation")
		}
	})

	t.Run("author_pronunciation_refines", func(t *testing.T) {
		book := baseBook
		book.AuthorPronunciations = []string{"オーターワン", "オーターツー"}
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])

		// First author pronunciation
		if !strings.Contains(opfData, `<meta refines="#creator0" property="alternate-script">オーターワン</meta>`) {
			t.Error("should have alternate-script refines for first author")
		}
		// Second author pronunciation
		if !strings.Contains(opfData, `<meta refines="#creator1" property="alternate-script">オーターツー</meta>`) {
			t.Error("should have alternate-script refines for second author")
		}
	})

	t.Run("epub2_no_refines", func(t *testing.T) {
		book := baseBook
		book.GenerateEpub2 = true
		book.TitlePronunciation = "タイトル"
		book.AuthorPronunciations = []string{"オーター"}
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])

		// EPUB2 should not have title id
		if strings.Contains(opfData, `<dc:title id="title"`) {
			t.Error("EPUB2 should not have title id attribute")
		}
		// EPUB2 should not have refines
		if strings.Contains(opfData, "refines=") {
			t.Error("EPUB2 should not have refines metadata")
		}
	})

	t.Run("author_role_refines", func(t *testing.T) {
		book := baseBook
		path := writeBookToTemp(t, book)
		files := readZIP(t, path)
		opfData := string(files["OEBPS/content.opf"])

		// Should have role refines for each author
		for i := range book.Authors {
			id := fmt.Sprintf("creator%d", i)
			pattern := fmt.Sprintf(`<meta refines="#%s" property="role" scheme="marc:relators">aut</meta>`, id)
			if !strings.Contains(opfData, pattern) {
				t.Errorf("author %d should have role refines", i)
			}
		}
	})
}

// =========================================================================== //
// VAL-M1-EPUB-009: OPF manifest item ordering matches Python
// Python: epub_output.py:1009 sorted(self.manifest, key=lambda m: m.filename)
// =========================================================================== //

func TestOPFManifestOrdering_MatchPython(t *testing.T) {
	book := Book{
		Identifier: "urn:uuid:manifest-order-test",
		Title:      "Manifest Order",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-01-01T00:00:00Z",
		Sections: []Section{
			{Filename: "chapter2.xhtml", Title: "Ch2", BodyHTML: "<p>2</p>"},
			{Filename: "chapter1.xhtml", Title: "Ch1", BodyHTML: "<p>1</p>"},
		},
		Resources: []Resource{
			{Filename: "image2.png", MediaType: "image/png", Data: []byte("png2")},
			{Filename: "image1.jpg", MediaType: "image/jpeg", Data: []byte("jpg1")},
			{Filename: "style.css", MediaType: "text/css", Data: []byte("body{}")},
		},
		Stylesheet: "body { margin: 0; }",
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)
	opfData := string(files["OEBPS/content.opf"])

	// Extract manifest items in order
	hrefs := extractAllMatches(opfData, `<item[^>]*href="([^"]*)"[^>]*>`)

	// All hrefs should be in alphabetical order (Python sorts by filename)
	sorted := make([]string, len(hrefs))
	copy(sorted, hrefs)
	sort.Strings(sorted)

	for i, got := range hrefs {
		if got != sorted[i] {
			t.Errorf("manifest item %d: got %q, want %q (alphabetical)", i, got, sorted[i])
		}
	}
}

// =========================================================================== //
// VAL-M1-EPUB-010: Font @font-face emission ordering matches Python
// Python: yj_to_epub_properties.py:2254 sorted(self.font_faces)
// Note: Font @font-face ordering is handled in internal/kfx package.
// This test validates that the epub package preserves stylesheet order.
// =========================================================================== //

func TestFontFaceOrdering(t *testing.T) {
	// The font @font-face ordering is done in the kfx package
	// (yj_to_epub_properties.go:239 sorts fontFaces).
	// Here we test that the stylesheet content is preserved as-is in the EPUB.
	fontCSS := "@font-face {font-family: \"Arial\"; src: url(\"a.ttf\")}\n@font-face {font-family: \"Bold\"; src: url(\"b.ttf\")}\nbody { font-family: Arial; }"

	book := Book{
		Identifier: "urn:uuid:font-test",
		Title:      "Font Test",
		Language:   "en",
		Authors:    []string{"A"},
		Modified:   "2024-01-01T00:00:00Z",
		Stylesheet: fontCSS,
		Sections: []Section{
			{Filename: "sec.xhtml", Title: "S", BodyHTML: "<p>x</p>"},
		},
	}
	path := writeBookToTemp(t, book)
	files := readZIP(t, path)

	cssData := string(files["OEBPS/stylesheet.css"])
	if cssData != fontCSS {
		t.Errorf("stylesheet content not preserved:\ngot: %q\nwant: %q", cssData, fontCSS)
	}
}
