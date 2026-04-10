package kfx

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestConvertFileCreatesReadableEPUB(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
	output := filepath.Join(t.TempDir(), "martyr.epub")

	if err := ConvertFile(input, output); err != nil {
		t.Fatalf("ConvertFile() error = %v", err)
	}

	if _, err := os.Stat(output); err != nil {
		t.Fatalf("expected EPUB output to exist: %v", err)
	}

	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer archive.Close()

	var foundReadableText bool
	for _, file := range archive.File {
		if !strings.HasPrefix(file.Name, "OEBPS/") || !strings.HasSuffix(file.Name, ".xhtml") {
			continue
		}
		sectionData := readZipFile(t, file)
		if strings.Contains(sectionData, "Copyright") || strings.Contains(sectionData, "Contents") {
			foundReadableText = true
			break
		}
	}

	if !foundReadableText {
		t.Fatalf("converted EPUB did not contain expected readable section text")
	}
}

func TestClassifyRecognizesDRMFixtures(t *testing.T) {
	martyr := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
	mode, reason, err := Classify(martyr)
	if err != nil {
		t.Fatalf("Classify(Martyr) error = %v", err)
	}
	if mode != "convert" || reason != "" {
		t.Fatalf("Classify(Martyr) = %q %q", mode, reason)
	}

	familiars := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "The Familiars_B003VIWNQW.kfx")
	mode, reason, err = Classify(familiars)
	if err != nil {
		t.Fatalf("Classify(The Familiars) error = %v", err)
	}
	if mode != "blocked" || reason != "drm" {
		t.Fatalf("Classify(The Familiars) = %q %q", mode, reason)
	}
}

func TestConvertFilePhase1PreservesCoverAndPackageResources(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
	output := filepath.Join(t.TempDir(), "martyr.epub")

	if err := ConvertFile(input, output); err != nil {
		t.Fatalf("ConvertFile() error = %v", err)
	}

	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer archive.Close()

	files := map[string]*zip.File{}
	var fontFiles []string
	for _, file := range archive.File {
		files[file.Name] = file
		if strings.HasPrefix(file.Name, "OEBPS/font_") && strings.HasSuffix(file.Name, ".otf") {
			fontFiles = append(fontFiles, file.Name)
		}
	}
	sort.Strings(fontFiles)

	requiredFiles := []string{
		"OEBPS/content.opf",
		"OEBPS/nav.xhtml",
		"OEBPS/toc.ncx",
		"OEBPS/stylesheet.css",
		"OEBPS/image_rsrc3S0.jpg",
	}
	for _, name := range requiredFiles {
		if _, ok := files[name]; !ok {
			t.Fatalf("expected EPUB to contain %s", name)
		}
	}

	if len(fontFiles) != 7 {
		t.Fatalf("expected 7 extracted fonts, got %d (%v)", len(fontFiles), fontFiles)
	}

	opf := readZipFile(t, files["OEBPS/content.opf"])
	for _, snippet := range []string{
		`properties="cover-image"`,
		`name="Override-Kindle-Fonts" content="true"`,
		`href="stylesheet.css"`,
		`href="toc.ncx"`,
	} {
		if !strings.Contains(opf, snippet) {
			t.Fatalf("content.opf is missing %q", snippet)
		}
	}

	css := readZipFile(t, files["OEBPS/stylesheet.css"])
	for _, snippet := range []string{
		`@font-face`,
		`font-family: FreeFontSerif`,
		`font-style: italic`,
		`font-weight: bold`,
		`src: url(font_rsrc3R2.otf)`,
	} {
		if !strings.Contains(css, snippet) {
			t.Fatalf("stylesheet.css is missing %q", snippet)
		}
	}
}

func TestConvertFilePhase2PreservesSectionIDsAndLinkedContents(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
	output := filepath.Join(t.TempDir(), "martyr.epub")

	if err := ConvertFile(input, output); err != nil {
		t.Fatalf("ConvertFile() error = %v", err)
	}

	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer archive.Close()

	files := map[string]*zip.File{}
	for _, file := range archive.File {
		files[file.Name] = file
	}

	for _, name := range []string{
		"OEBPS/c0.xhtml",
		"OEBPS/c9.xhtml",
		"OEBPS/cM.xhtml",
		"OEBPS/c1J.xhtml",
	} {
		if _, ok := files[name]; !ok {
			t.Fatalf("expected EPUB to contain %s", name)
		}
	}

	coverPage := readZipFile(t, files["OEBPS/c0.xhtml"])
	if !strings.Contains(coverPage, `image_rsrc3S0.jpg`) {
		t.Fatalf("c0.xhtml did not preserve the cover image page")
	}

	titlePage := readZipFile(t, files["OEBPS/c9.xhtml"])
	for _, snippet := range []string{
		`image_rsrc3S1`,
		`Book Title, Martyr!`,
	} {
		if !strings.Contains(titlePage, snippet) {
			t.Fatalf("c9.xhtml is missing %q", snippet)
		}
	}

	contentsPage := readZipFile(t, files["OEBPS/c1J.xhtml"])
	for _, snippet := range []string{
		`Contents`,
		`<a href="c0.xhtml"`,
		`<a href="c9.xhtml"`,
		`<a href="cM.xhtml"`,
		`<a href="c6P.xhtml"`,
		`Chapter One`,
	} {
		if !strings.Contains(contentsPage, snippet) {
			t.Fatalf("c1J.xhtml is missing %q", snippet)
		}
	}

	nav := readZipFile(t, files["OEBPS/nav.xhtml"])
	for _, snippet := range []string{
		`<a href="c0.xhtml">Cover</a>`,
		`<a href="c9.xhtml">Title Page</a>`,
		`<a href="cM.xhtml">Copyright</a>`,
		`<a href="c1J.xhtml">Contents</a>`,
		`<a href="c6P.xhtml">Chapter One</a>`,
	} {
		if !strings.Contains(nav, snippet) {
			t.Fatalf("nav.xhtml is missing %q", snippet)
		}
	}
}

func TestConvertFilePhase3UsesCanonicalSectionFilesForNavigationAndSpine(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
	output := filepath.Join(t.TempDir(), "martyr.epub")

	if err := ConvertFile(input, output); err != nil {
		t.Fatalf("ConvertFile() error = %v", err)
	}

	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer archive.Close()

	files := map[string]*zip.File{}
	for _, file := range archive.File {
		files[file.Name] = file
	}

	nav := readZipFile(t, files["OEBPS/nav.xhtml"])
	if strings.Contains(nav, `href="cover.xhtml"`) {
		t.Fatalf("nav.xhtml should not include synthetic cover.xhtml once c0.xhtml exists")
	}

	toc := readZipFile(t, files["OEBPS/toc.ncx"])
	if strings.Contains(toc, `src="cover.xhtml"`) {
		t.Fatalf("toc.ncx should not include synthetic cover.xhtml once c0.xhtml exists")
	}

	opf := readZipFile(t, files["OEBPS/content.opf"])
	if strings.Contains(opf, `<itemref idref="cover-page"/>`) {
		t.Fatalf("content.opf spine should not include synthetic cover.xhtml once c0.xhtml exists")
	}
}

func TestConvertFilePhase4EmitsStyleClassesForTitleAndChapterPages(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
	output := filepath.Join(t.TempDir(), "martyr.epub")

	if err := ConvertFile(input, output); err != nil {
		t.Fatalf("ConvertFile() error = %v", err)
	}

	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer archive.Close()

	files := map[string]*zip.File{}
	for _, file := range archive.File {
		files[file.Name] = file
	}

	c9 := readZipFile(t, files["OEBPS/c9.xhtml"])
	for _, snippet := range []string{
		`class="class_sK-0"`,
		`class="class_sK-1"`,
	} {
		if !strings.Contains(c9, snippet) {
			t.Fatalf("c9.xhtml is missing %q", snippet)
		}
	}

	c6P := readZipFile(t, files["OEBPS/c6P.xhtml"])
	for _, snippet := range []string{
		`class="heading_s6W-0"`,
		`class="class_s72"`,
		`class="class_s71"`,
	} {
		if !strings.Contains(c6P, snippet) {
			t.Fatalf("c6P.xhtml is missing %q", snippet)
		}
	}

	css := readZipFile(t, files["OEBPS/stylesheet.css"])
	for _, snippet := range []string{
		`.class_sK-0 {text-align: center}`,
		`.class_sK-1 {height: 100%}`,
		`.class_s72 {font-size: 1.86em; line-height: 1.22592; margin-bottom: 0.978857em; margin-top: 0.978857em; page-break-inside: avoid}`,
		`.class_s71 {width: 5.514%}`,
		`.heading_s6W-0 {-webkit-hyphens: auto; font-family: 'Trajan Pro 3'; font-size: 1.53em; font-weight: normal; hyphens: auto; line-height: 1.21536; margin-bottom: 0; margin-top: 2.06507em; page-break-after: avoid}`,
	} {
		if !strings.Contains(css, snippet) {
			t.Fatalf("stylesheet.css is missing %q", snippet)
		}
	}
}

func TestConvertFilePhase5ConvertsJPEGXRResourcesToJPEG(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
	output := filepath.Join(t.TempDir(), "martyr.epub")

	if err := ConvertFile(input, output); err != nil {
		t.Fatalf("ConvertFile() error = %v", err)
	}

	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer archive.Close()

	files := map[string]*zip.File{}
	for _, file := range archive.File {
		files[file.Name] = file
		if strings.HasSuffix(file.Name, ".jxr") {
			t.Fatalf("expected converted EPUB to contain no .jxr resources, found %s", file.Name)
		}
	}

	expectedJPEGs := map[string][2]int{
		"OEBPS/image_rsrc3S1.jpg": {1094, 1920},
		"OEBPS/image_rsrc3S2.jpg": {331, 337},
		"OEBPS/image_rsrc3S3.jpg": {1875, 202},
		"OEBPS/image_rsrc3S4.jpg": {921, 522},
		"OEBPS/image_rsrc3S5.jpg": {1117, 894},
	}
	for name, wantSize := range expectedJPEGs {
		file := files[name]
		if file == nil {
			t.Fatalf("expected EPUB to contain %s", name)
		}
		data := []byte(readZipFile(t, file))
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s is not a valid JPEG: %v", name, err)
		}
		if cfg.Width != wantSize[0] || cfg.Height != wantSize[1] {
			t.Fatalf("%s dimensions = %dx%d, want %dx%d", name, cfg.Width, cfg.Height, wantSize[0], wantSize[1])
		}
	}

	c9 := readZipFile(t, files["OEBPS/c9.xhtml"])
	for _, snippet := range []string{
		`image_rsrc3S1.jpg`,
		`Book Title, Martyr!`,
	} {
		if !strings.Contains(c9, snippet) {
			t.Fatalf("c9.xhtml is missing %q", snippet)
		}
	}
	if strings.Contains(c9, `.jxr`) {
		t.Fatalf("c9.xhtml should not reference .jxr resources after conversion")
	}

	c6P := readZipFile(t, files["OEBPS/c6P.xhtml"])
	if !strings.Contains(c6P, `image_rsrc3S4.jpg`) {
		t.Fatalf("c6P.xhtml should reference the converted JPEG resource")
	}
	if strings.Contains(c6P, `.jxr`) {
		t.Fatalf("c6P.xhtml should not reference .jxr resources after conversion")
	}
}

func TestConvertFilePhase6TracksCalibrePackageAndNavigationSemantics(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
	output := filepath.Join(t.TempDir(), "martyr.epub")

	if err := ConvertFile(input, output); err != nil {
		t.Fatalf("ConvertFile() error = %v", err)
	}

	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer archive.Close()

	files := map[string]*zip.File{}
	for _, file := range archive.File {
		files[file.Name] = file
	}

	if _, ok := files["OEBPS/cover.xhtml"]; ok {
		t.Fatalf("EPUB should not contain synthetic cover.xhtml once canonical cover section exists")
	}

	nav := readZipFile(t, files["OEBPS/nav.xhtml"])
	for _, snippet := range []string{
		`<title>nav</title>`,
		`<nav epub:type="toc" hidden="">`,
		`<h1>Table of contents</h1>`,
		`<li><a href="c6P.xhtml">Chapter One</a><ol><li><a href="c73.xhtml">Monday</a></li></ol></li>`,
	} {
		if !strings.Contains(nav, snippet) {
			t.Fatalf("nav.xhtml is missing %q", snippet)
		}
	}

	toc := readZipFile(t, files["OEBPS/toc.ncx"])
	for _, snippet := range []string{
		`content="urn:asin:5AFAFAA13FFE43ECBE78F0FF3761814C"`,
		`<docAuthor>`,
		`<text>Kaveh Akbar</text>`,
		`<navPoint id="nav7">`,
		`<text>Chapter One</text>`,
		`<content src="c6P.xhtml"/>`,
		`<navPoint id="nav8">`,
		`<text>Monday</text>`,
		`<content src="c73.xhtml"/>`,
	} {
		if !strings.Contains(toc, snippet) {
			t.Fatalf("toc.ncx is missing %q", snippet)
		}
	}

	opf := readZipFile(t, files["OEBPS/content.opf"])
	for _, snippet := range []string{
		`prefix="marc: http://id.loc.gov/vocabulary/"`,
		`<dc:identifier id="bookid">urn:asin:5AFAFAA13FFE43ECBE78F0FF3761814C</dc:identifier>`,
		`<dc:creator id="creator0">Kaveh Akbar</dc:creator>`,
		`<meta refines="#creator0" property="role" scheme="marc:relators">aut</meta>`,
		`<meta property="dcterms:modified">`,
		`<meta name="cover" content="image_rsrc3S0.jpg"/>`,
		`<item href="c0.xhtml" id="c0.xhtml" media-type="application/xhtml+xml" properties="svg"/>`,
		`<item href="image_rsrc3S0.jpg" id="image_rsrc3S0.jpg" media-type="image/jpeg" properties="cover-image"/>`,
		`<item href="font_rsrc3R2.otf" id="font_rsrc3R2.otf" media-type="font/otf"/>`,
		`<item href="nav.xhtml" id="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>`,
		`<spine toc="toc.ncx">`,
		`<itemref idref="c0.xhtml"/>`,
		`<itemref idref="c9.xhtml"/>`,
	} {
		if !strings.Contains(opf, snippet) {
			t.Fatalf("content.opf is missing %q", snippet)
		}
	}
	if strings.Contains(opf, `cover.xhtml`) {
		t.Fatalf("content.opf should not reference synthetic cover.xhtml")
	}

	c0 := readZipFile(t, files["OEBPS/c0.xhtml"])
	for _, snippet := range []string{
		`xml:lang=`,
		`<title>c0</title>`,
		`<body class="class_s8">`,
		`<svg `,
		`xlink:href="image_rsrc3S0.jpg"`,
	} {
		if !strings.Contains(c0, snippet) {
			t.Fatalf("c0.xhtml is missing %q", snippet)
		}
	}

	c1J := readZipFile(t, files["OEBPS/c1J.xhtml"])
	for _, snippet := range []string{
		`<title>c1J</title>`,
		`<body class="class-1">`,
	} {
		if !strings.Contains(c1J, snippet) {
			t.Fatalf("c1J.xhtml is missing %q", snippet)
		}
	}

	c6P := readZipFile(t, files["OEBPS/c6P.xhtml"])
	for _, snippet := range []string{
		`<title>c6P</title>`,
		`<body class="class-0">`,
		`<div class="class_s72"><img src="image_rsrc3S4.jpg" alt="" class="class_s71"/></div>`,
	} {
		if !strings.Contains(c6P, snippet) {
			t.Fatalf("c6P.xhtml is missing %q", snippet)
		}
	}
	if strings.Contains(c6P, `<div class="class_s72"><div>`) {
		t.Fatalf("c6P.xhtml should not include an extra wrapper div around the chapter image")
	}
}

func TestConvertFilePhase7UsesPageTemplateStylesForSectionBodyClasses(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
	output := filepath.Join(t.TempDir(), "martyr.epub")

	if err := ConvertFile(input, output); err != nil {
		t.Fatalf("ConvertFile() error = %v", err)
	}

	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer archive.Close()

	files := map[string]*zip.File{}
	for _, file := range archive.File {
		files[file.Name] = file
	}

	wantBodyClass := map[string]string{
		"OEBPS/cM.xhtml":   `class="class_s1H-1"`,
		"OEBPS/c2GS.xhtml": `class="class_s2H4"`,
		"OEBPS/c2NP.xhtml": `class="class_s2P2"`,
		"OEBPS/cGY.xhtml":  `class="class_sHA"`,
		"OEBPS/cR2.xhtml":  `class="class_sRD"`,
		"OEBPS/c35Y.xhtml": `class="class-8"`,
	}
	for name, snippet := range wantBodyClass {
		file := files[name]
		if file == nil {
			t.Fatalf("expected EPUB to contain %s", name)
		}
		data := readZipFile(t, file)
		if !strings.Contains(data, snippet) {
			t.Fatalf("%s is missing %q", name, snippet)
		}
	}
}

func TestConvertFilePhase8MatchesInlineStyleEventsAndFitWidthContainers(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
	output := filepath.Join(t.TempDir(), "martyr.epub")

	if err := ConvertFile(input, output); err != nil {
		t.Fatalf("ConvertFile() error = %v", err)
	}

	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer archive.Close()

	files := map[string]*zip.File{}
	for _, file := range archive.File {
		files[file.Name] = file
	}

	c15V := readZipFile(t, files["OEBPS/c15V.xhtml"])
	for _, snippet := range []string{
		`11. <span class="class_s3PK">یازده</span>. Yahz-dah.`,
		`<p class="class_s87-1">—</p>`,
	} {
		if !strings.Contains(c15V, snippet) {
			t.Fatalf("c15V.xhtml is missing %q", snippet)
		}
	}

	c73 := readZipFile(t, files["OEBPS/c73.xhtml"])
	if !strings.Contains(c73, `<p class="class_s87-0">—</p>`) {
		t.Fatalf("c73.xhtml is missing centered separator paragraph class")
	}

	cA8 := readZipFile(t, files["OEBPS/cA8.xhtml"])
	for _, snippet := range []string{
		`<td class="class_sAP-1">IV.</td>`,
		`<div class="class_sB0-0"><div class="class_sB0-1"><p class="class_sAY">A. GENERAL</p></div></div>`,
		`<p class="class_sB2"><span class="class_s3PX">1. </span>The USS VINCENNES did not purposely shoot down an Iranian commercial airliner.</p>`,
	} {
		if !strings.Contains(cA8, snippet) {
			t.Fatalf("cA8.xhtml is missing %q", snippet)
		}
	}

	css := readZipFile(t, files["OEBPS/stylesheet.css"])
	for _, snippet := range []string{
		`.class_s87-0 {text-align: center; text-indent: 0}`,
		`.class_s87-1 {text-align: center}`,
		`.class_s3PK {font-size: 1.02em; line-height: 1.23552}`,
		`.class_s3PX {margin-right: 0.25em}`,
		`.class_sAP-1 {margin-right: 15.5%; text-align: right; vertical-align: top}`,
		`.class_sB0-0 {font-weight: bold; margin-right: 0.781%; margin-top: 1.18em; text-align: right}`,
		`.class_sB0-1 {display: inline-block}`,
	} {
		if !strings.Contains(css, snippet) {
			t.Fatalf("stylesheet.css is missing %q", snippet)
		}
	}
}

func TestSymbolResolverAccountsForIonSystemSymbols(t *testing.T) {
	input := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples", "Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
	docSymbols := readFixtureDocSymbols(t, input)

	resolver, err := newSymbolResolver(docSymbols)
	if err != nil {
		t.Fatalf("newSymbolResolver() error = %v", err)
	}

	for sid, want := range map[uint32]string{
		852: "c0",
		853: "c9",
		854: "cM",
		855: "c1J",
		954: "s8",
		955: "sF",
		956: "sK",
	} {
		if got := resolver.Resolve(sid); got != want {
			t.Fatalf("Resolve(%d) = %q, want %q", sid, got, want)
		}
	}
}

func readZipFile(t *testing.T, file *zip.File) string {
	t.Helper()

	reader, err := file.Open()
	if err != nil {
		t.Fatalf("file.Open() error = %v", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	return string(data)
}

func readFixtureDocSymbols(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(data) < 18 {
		t.Fatalf("fixture is too short")
	}

	containerInfoOffset := int(binary.LittleEndian.Uint32(data[10:14]))
	containerInfoLength := int(binary.LittleEndian.Uint32(data[14:18]))
	containerInfo, err := decodeIonMap(data[containerInfoOffset:containerInfoOffset+containerInfoLength], nil, nil)
	if err != nil {
		t.Fatalf("decodeIonMap(container_info) error = %v", err)
	}

	docSymbolOffset, ok := asInt(containerInfo["$415"])
	if !ok {
		t.Fatalf("container_info missing $415")
	}
	docSymbolLength, ok := asInt(containerInfo["$416"])
	if !ok {
		t.Fatalf("container_info missing $416")
	}
	if docSymbolOffset+docSymbolLength > len(data) {
		t.Fatalf("document symbol slice is out of range")
	}

	return data[docSymbolOffset : docSymbolOffset+docSymbolLength]
}
