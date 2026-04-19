// Package epub implements EPUB3 zip emission for the Kindle helper. Calibre’s EPUB_Output
// (epub_output.py) remains the behavioral superset (manifest item order, NCX depth, guide/landmarks);
// extend here when driving diffs to zero vs calibre_epubs fixtures.
package epub

import (
	"archive/zip"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Book struct {
	Identifier          string
	Title               string
	Language            string
	Authors             []string
	Published           string
	Description         string
	Publisher           string
	Modified            string
	OverrideKindleFonts bool
	Stylesheet          string
	CoverImageHref      string
	Sections            []Section
	Resources           []Resource
	Navigation          []NavPoint
	Guide               []GuideEntry
	PageList            []PageTarget
}

type Section struct {
	Filename   string
	Title      string
	PageTitle  string
	Language   string
	BodyClass  string
	Paragraphs []string
	BodyHTML   string
	Properties string
}

type Resource struct {
	Filename   string
	MediaType  string
	Data       []byte
	Properties string
}

type NavPoint struct {
	Title    string
	Href     string
	Children []NavPoint
}

type GuideEntry struct {
	Type  string
	Title string
	Href  string
}

type PageTarget struct {
	Label string
	Href  string
}

const (
	xmlDecl       = "<?xml version='1.0' encoding='utf-8'?>\n"
	containerXML  = xmlDecl + `<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">` + "\n" + `  <rootfiles>` + "\n" + `    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>` + "\n" + `  </rootfiles>` + "\n" + `</container>` + "\n"
	navHiddenAttr = ` hidden=""`
)

func Write(path string, book Book) error {
	if book.Title == "" {
		book.Title = "Untitled"
	}
	if book.Language == "" {
		book.Language = "en"
	}
	if book.Modified == "" {
		book.Modified = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	}
	if len(book.Navigation) == 0 {
		book.Navigation = navigationFromSections(book.Sections)
	}

	files := map[string][]byte{
		"META-INF/container.xml": []byte(containerXML),
		"OEBPS/content.opf":      []byte(contentOPF(book)),
		"OEBPS/nav.xhtml":        []byte(navXHTML(book)),
		"OEBPS/toc.ncx":          []byte(tocNCX(book)),
	}
	if book.Stylesheet != "" {
		files["OEBPS/stylesheet.css"] = []byte(book.Stylesheet)
	}
	for _, resource := range book.Resources {
		if resource.Filename == "" {
			continue
		}
		files["OEBPS/"+resource.Filename] = resource.Data
	}
	for index, section := range book.Sections {
		filename := section.Filename
		if filename == "" {
			filename = fmt.Sprintf("section-%03d.xhtml", index+1)
		}
		section.Filename = filename
		files["OEBPS/"+filename] = []byte(sectionXHTML(book, section))
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	if err := writeStored(zw, "mimetype", []byte("application/epub+zip")); err != nil {
		_ = zw.Close()
		return err
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if err := writeDeflated(zw, name, files[name]); err != nil {
			_ = zw.Close()
			return err
		}
	}

	if err := zw.Close(); err != nil {
		return err
	}
	return file.Sync()
}

func writeStored(zw *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func writeDeflated(zw *zip.Writer, name string, data []byte) error {
	writer, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func navigationFromSections(sections []Section) []NavPoint {
	points := make([]NavPoint, 0, len(sections))
	for _, section := range sections {
		title := strings.TrimSpace(section.Title)
		if title == "" {
			title = sectionBaseTitle(section.Filename)
		}
		points = append(points, NavPoint{Title: title, Href: section.Filename})
	}
	return points
}

func navXHTML(book Book) string {
	var body strings.Builder
	body.WriteString(`<nav epub:type="toc"` + navHiddenAttr + `><h1>Table of contents</h1><ol>`)
	body.WriteString(navPointListHTML(book.Navigation))
	body.WriteString(`</ol></nav>`)
	if len(book.Guide) > 0 {
		body.WriteString("\n")
		body.WriteString(`<nav epub:type="landmarks"` + navHiddenAttr + `><h2>Guide</h2><ol>`)
		for _, entry := range sortedGuide(book.Guide) {
			body.WriteString(`<li><a`)
			if entry.Type != "" {
				body.WriteString(` epub:type="` + xmlEscape(epubTypeForGuide(entry.Type)) + `"`)
			}
			body.WriteString(` href="` + xmlEscape(entry.Href) + `">` + ncxTextEscape(entry.Title) + `</a></li>`)
		}
		body.WriteString(`</ol></nav>`)
	}
	if len(book.PageList) > 0 {
		body.WriteString("\n")
		body.WriteString(`<nav epub:type="page-list"` + navHiddenAttr + `><ol>`)
		for _, page := range book.PageList {
			body.WriteString(`<li><a href="` + xmlEscape(page.Href) + `">` + xmlEscape(page.Label) + `</a></li>`)
		}
		body.WriteString(`</ol></nav>`)
	}
	return xmlDecl +
		`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">` + "\n" +
		`<head>` + "\n" +
		`<title>nav</title>` + "\n" +
		`</head>` + "\n" +
		`<body>` + "\n" +
		body.String() + "\n" +
		`</body>` + "\n" +
		`</html>`
}

func navPointListHTML(points []NavPoint) string {
	var items strings.Builder
	for _, point := range points {
		items.WriteString(`<li><a href="` + xmlEscape(point.Href) + `">` + ncxTextEscape(point.Title) + `</a>`)
		if len(point.Children) > 0 {
			items.WriteString(`<ol>` + navPointListHTML(point.Children) + `</ol>`)
		}
		items.WriteString(`</li>`)
	}
	return items.String()
}

func tocNCX(book Book) string {
	var out strings.Builder
	out.WriteString(xmlDecl)
	out.WriteString(`<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">` + "\n")
	out.WriteString(`  <head>` + "\n")
	out.WriteString(`    <meta name="dtb:uid" content="` + xmlEscape(book.Identifier) + `"/>` + "\n")
	out.WriteString(`  </head>` + "\n")
	out.WriteString(`  <docTitle>` + "\n")
	out.WriteString(`    <text>` + xmlEscape(book.Title) + `</text>` + "\n")
	out.WriteString(`  </docTitle>` + "\n")
	for _, author := range book.Authors {
		out.WriteString(`  <docAuthor>` + "\n")
		out.WriteString(`    <text>` + xmlEscape(author) + `</text>` + "\n")
		out.WriteString(`  </docAuthor>` + "\n")
	}
	out.WriteString(`  <navMap>` + "\n")
	navID := 0
	appendNCXPoints(&out, book.Navigation, 2, &navID)
	out.WriteString(`  </navMap>` + "\n")
	if len(book.PageList) > 0 {
		out.WriteString(`  <pageList>` + "\n")
		out.WriteString(`    <navLabel>` + "\n")
		out.WriteString(`      <text>Pages</text>` + "\n")
		out.WriteString(`    </navLabel>` + "\n")
		pageIDs := map[string]bool{}
		for _, page := range book.PageList {
			pageID := makeUniqueName(fixHTMLID("page_"+page.Label), pageIDs, "_", false)
			pageIDs[pageID] = true
			value, pageType := ncxPageMetadata(page.Label)
			out.WriteString(`    <pageTarget id="` + xmlEscape(pageID) + `"`)
			if value != "" {
				out.WriteString(` value="` + xmlEscape(value) + `"`)
			}
			if pageType != "" {
				out.WriteString(` type="` + xmlEscape(pageType) + `"`)
			}
			out.WriteString(`>` + "\n")
			out.WriteString(`      <navLabel>` + "\n")
			out.WriteString(`        <text>` + xmlEscape(page.Label) + `</text>` + "\n")
			out.WriteString(`      </navLabel>` + "\n")
			out.WriteString(`      <content src="` + xmlEscape(page.Href) + `"/>` + "\n")
			out.WriteString(`    </pageTarget>` + "\n")
		}
		out.WriteString(`  </pageList>` + "\n")
	}
	out.WriteString(`</ncx>` + "\n")
	return out.String()
}

func appendNCXPoints(out *strings.Builder, points []NavPoint, indent int, navID *int) {
	prefix := strings.Repeat("  ", indent)
	for _, point := range points {
		id := fmt.Sprintf("nav%d", *navID)
		*navID = *navID + 1
		out.WriteString(prefix + `<navPoint id="` + id + `">` + "\n")
		out.WriteString(prefix + `  <navLabel>` + "\n")
		out.WriteString(prefix + `    <text>` + ncxTextEscape(point.Title) + `</text>` + "\n")
		out.WriteString(prefix + `  </navLabel>` + "\n")
		out.WriteString(prefix + `  <content src="` + xmlEscape(point.Href) + `"/>` + "\n")
		if len(point.Children) > 0 {
			appendNCXPoints(out, point.Children, indent+1, navID)
		}
		out.WriteString(prefix + `</navPoint>` + "\n")
	}
}

func contentOPF(book Book) string {
	var out strings.Builder
	out.WriteString(xmlDecl)
	out.WriteString(`<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/" version="3.0" unique-identifier="bookid" prefix="marc: http://id.loc.gov/vocabulary/">` + "\n")
	out.WriteString(`  <metadata>` + "\n")
	out.WriteString(`    <dc:identifier id="bookid">` + xmlEscape(book.Identifier) + `</dc:identifier>` + "\n")
	out.WriteString(`    <dc:title>` + xmlEscape(book.Title) + `</dc:title>` + "\n")
	for index, author := range book.Authors {
		id := fmt.Sprintf("creator%d", index)
		out.WriteString(`    <dc:creator id="` + id + `">` + xmlEscape(author) + `</dc:creator>` + "\n")
		out.WriteString(`    <meta refines="#` + id + `" property="role" scheme="marc:relators">aut</meta>` + "\n")
	}
	out.WriteString(`    <dc:language>` + xmlEscape(book.Language) + `</dc:language>` + "\n")
	if book.Publisher != "" {
		out.WriteString(`    <dc:publisher>` + xmlEscape(book.Publisher) + `</dc:publisher>` + "\n")
	}
	if book.Description != "" {
		out.WriteString(`    <dc:description>` + xmlEscape(book.Description) + `</dc:description>` + "\n")
	}
	if book.Published != "" {
		out.WriteString(`    <dc:date>` + xmlEscape(book.Published) + `</dc:date>` + "\n")
	}
	out.WriteString(`    <meta property="dcterms:modified">` + xmlEscape(book.Modified) + `</meta>` + "\n")
	if book.OverrideKindleFonts {
		out.WriteString(`    <meta name="Override-Kindle-Fonts" content="true"/>` + "\n")
	}
	if book.CoverImageHref != "" {
		// Use truncated manifest ID for cover meta (Python: fix_html_id basename[:64]).
		coverID := makeManifestID(book.CoverImageHref, map[string]bool{})
		out.WriteString(`    <meta name="cover" content="` + xmlEscape(coverID) + `"/>` + "\n")
	}
	out.WriteString(`  </metadata>` + "\n")
	out.WriteString(`  <manifest>` + "\n")
	// Calibre sorts all manifest items together by filename (natural sort).
	// Port of epub_output.py generate_opf manifest ordering.
	type manifestItem struct {
		href        string
		id          string
		mediaType   string
		properties  string
		sortKey     string
	}
	items := make([]manifestItem, 0, len(book.Sections)+len(book.Resources)+3)
	usedIDs := map[string]bool{}
	sectionIDs := map[string]string{} // filename → manifest ID for spine
	for _, section := range book.Sections {
		if section.Filename == "" {
			continue
		}
		id := makeManifestID(section.Filename, usedIDs)
		sectionIDs[section.Filename] = id
		items = append(items, manifestItem{
			href:       section.Filename,
			id:         id,
			mediaType:  "application/xhtml+xml",
			properties: section.Properties,
			sortKey:    naturalSortKeyEpub(section.Filename),
		})
	}
	for _, resource := range book.Resources {
		if resource.Filename == "" || resource.MediaType == "" {
			continue
		}
		id := makeManifestID(resource.Filename, usedIDs)
		items = append(items, manifestItem{
			href:       resource.Filename,
			id:         id,
			mediaType:  resource.MediaType,
			properties: resource.Properties,
			sortKey:    naturalSortKeyEpub(resource.Filename),
		})
	}
	// nav, stylesheet, toc.ncx added at the end
	items = append(items, manifestItem{href: "nav.xhtml", id: "nav.xhtml", mediaType: "application/xhtml+xml", properties: "nav", sortKey: naturalSortKeyEpub("nav.xhtml")})
	if book.Stylesheet != "" {
		items = append(items, manifestItem{href: "stylesheet.css", id: "stylesheet.css", mediaType: "text/css", sortKey: naturalSortKeyEpub("stylesheet.css")})
	}
	items = append(items, manifestItem{href: "toc.ncx", id: "toc.ncx", mediaType: "application/x-dtbncx+xml", sortKey: naturalSortKeyEpub("toc.ncx")})
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].href < items[j].href
	})
	for _, item := range items {
		out.WriteString(`    <item href="` + xmlEscape(item.href) + `" id="` + xmlEscape(item.id) + `" media-type="` + xmlEscape(item.mediaType) + `"`)
		if item.properties != "" {
			out.WriteString(` properties="` + xmlEscape(item.properties) + `"`)
		}
		out.WriteString(`/>` + "\n")
	}
	out.WriteString(`  </manifest>` + "\n")
	out.WriteString(`  <spine toc="toc.ncx">` + "\n")
	for _, section := range book.Sections {
		if section.Filename == "" {
			continue
		}
		id := sectionIDs[section.Filename]
		if id == "" {
			id = section.Filename
		}
		out.WriteString(`    <itemref idref="` + xmlEscape(id) + `"/>` + "\n")
	}
	out.WriteString(`  </spine>` + "\n")
	if len(book.Guide) > 0 {
		out.WriteString(`  <guide>` + "\n")
		for _, entry := range book.Guide {
			out.WriteString(`    <reference type="` + xmlEscape(entry.Type) + `" title="` + xmlEscape(entry.Title) + `" href="` + xmlEscape(entry.Href) + `"/>` + "\n")
		}
		out.WriteString(`  </guide>` + "\n")
	}
	out.WriteString(`</package>`)
	return out.String()
}

func sectionXHTML(book Book, section Section) string {
	var body strings.Builder
	if section.BodyHTML != "" {
		body.WriteString(section.BodyHTML)
	} else {
		body.WriteString(`<h1>` + html.EscapeString(section.Title) + `</h1>`)
		for _, paragraph := range section.Paragraphs {
			body.WriteString(`<p>` + html.EscapeString(paragraph) + `</p>`)
		}
	}

	pageLanguage := section.Language
	if pageLanguage == "" {
		pageLanguage = book.Language
	}
	pageTitle := section.PageTitle
	if pageTitle == "" {
		pageTitle = sectionBaseTitle(section.Filename)
	}
	if pageTitle == "" {
		pageTitle = section.Title
	}
	if pageTitle == "" {
		pageTitle = book.Title
	}

	var out strings.Builder
	out.WriteString(xmlDecl)
	out.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml"`)
	if pageLanguage != "" {
		out.WriteString(` xml:lang="` + xmlEscape(pageLanguage) + `"`)
	}
	out.WriteString(`>` + "\n")
	out.WriteString(`<head>` + "\n")
	if book.Stylesheet != "" {
		out.WriteString(`<link rel="stylesheet" type="text/css" href="stylesheet.css"/>` + "\n")
	}
	out.WriteString(`<title>` + html.EscapeString(pageTitle) + `</title>` + "\n")
	out.WriteString(`</head>` + "\n")
	out.WriteString(`<body`)
	if section.BodyClass != "" {
		out.WriteString(` class="` + xmlEscape(section.BodyClass) + `"`)
	}
	out.WriteString(`>` + "\n")
	if section.Properties == "svg" && !strings.Contains(body.String(), "\n") {
		out.WriteString(body.String())
		out.WriteString(`</body>` + "\n")
	} else {
		out.WriteString(body.String() + "\n")
		out.WriteString(`</body>` + "\n")
	}
	out.WriteString(`</html>`)
	return out.String()
}

func sortedSectionsByFilename(sections []Section) []Section {
	sorted := append([]Section(nil), sections...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Filename < sorted[j].Filename
	})
	return sorted
}

func sortedResourcesByFilename(resources []Resource) []Resource {
	sorted := append([]Resource(nil), resources...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Filename < sorted[j].Filename
	})
	return sorted
}

func sortedGuide(entries []GuideEntry) []GuideEntry {
	sorted := append([]GuideEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Type == sorted[j].Type {
			return sorted[i].Title < sorted[j].Title
		}
		return sorted[i].Type < sorted[j].Type
	})
	return sorted
}

func sectionBaseTitle(filename string) string {
	if filename == "" {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
}

func xmlEscape(text string) string {
	return html.EscapeString(text)
}

// ncxTextEscape escapes XML text content but preserves double quotes (Calibre behavior).
// In XML text nodes, " doesn't need escaping; html.EscapeString converts it to &#34; unnecessarily.
func ncxTextEscape(text string) string {
	escaped := html.EscapeString(text)
	escaped = strings.ReplaceAll(escaped, "&#34;", `"`)
	return escaped
}

// naturalSortKeyEpub returns a sort key that sorts filenames naturally (numbers by value).
func naturalSortKeyEpub(value string) string {
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

// makeManifestID produces a unique manifest ID from a filename.
// Port of Python epub_output.py manifest_resource: fix_html_id(filename.rpartition("/")[2][:64])
// + make_unique_name deduplication.
func makeManifestID(filename string, used map[string]bool) string {
	// Get basename, truncate to 64 chars like Python.
	base := filename
	if idx := strings.LastIndex(filename, "/"); idx >= 0 {
		base = filename[idx+1:]
	}
	if len(base) > 64 {
		base = base[:64]
	}
	// Sanitize: replace non-alphanumeric (except _ . -) with _
	id := sanitizeManifestID(base)
	// Ensure starts with letter
	if len(id) == 0 || !((id[0] >= 'A' && id[0] <= 'Z') || (id[0] >= 'a' && id[0] <= 'z')) {
		id = "id_" + id
	}
	// Deduplicate
	if !used[id] {
		used[id] = true
		return id
	}
	for i := 0; ; i++ {
		candidate := fmt.Sprintf("%s_%d", id, i)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

func sanitizeManifestID(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func EnsureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func epubTypeForGuide(guideType string) string {
	switch guideType {
	case "cover":
		return "cover"
	case "text":
		return "bodymatter"
	case "toc":
		return "toc"
	default:
		return guideType
	}
}

func ncxPageMetadata(label string) (string, string) {
	if label == "" {
		return "", ""
	}
	if isDecimal(label) {
		return label, "normal"
	}
	if value, ok := romanToInt(label); ok {
		return fmt.Sprintf("%d", value), "front"
	}
	return "", "special"
}

func isDecimal(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func romanToInt(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	numerals := map[rune]int{'i': 1, 'v': 5, 'x': 10, 'l': 50, 'c': 100, 'd': 500, 'm': 1000}
	value = strings.ToLower(value)
	total := 0
	prev := 0
	for i := len(value) - 1; i >= 0; i-- {
		n := numerals[rune(value[i])]
		if n == 0 {
			return 0, false
		}
		if n < prev {
			total -= n
		} else {
			total += n
			prev = n
		}
	}
	return total, true
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

func makeUniqueName(root string, seen map[string]bool, sep string, alwaysSuffix bool) string {
	if !alwaysSuffix && root != "" && !seen[root] {
		return root
	}
	for index := 0; ; index++ {
		candidate := fmt.Sprintf("%s%s%d", root, sep, index)
		if !seen[candidate] {
			return candidate
		}
	}
}
