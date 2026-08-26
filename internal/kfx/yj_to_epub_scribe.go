package kfx

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
)

// Scribe notebook production wiring.
//
// Python reference: the notebook processors (yj_to_epub_notebook.py) are mixin
// methods on KFX_EPUB and reach book-level state directly (self.new_book_part,
// self.manifest_resource, self.reading_orders, ...). The Go port keeps the
// processors callback-driven via ScribeNotebookContext; this file builds that
// context from the real pipeline state and materializes the produced Scribe
// book parts into decodedBook sections/resources:
//
//   - self.nmdl_template_id         ← document_data nmdl.template_id
//     (yj_to_epub_metadata.py:28,91)
//   - self.reading_orders           ← document_data/metadata reading_orders
//     (yj_structure.py:346-355, get_reading_orders L1182-1189)
//   - self.new_book_part            (epub_output.py:502-523)
//   - self.manifest_resource        (epub_output.py:353-376)
//   - self.resource_location_filename (yj_to_epub_resources.py:244-285)
//   - self.process_content_properties (yj_to_epub_properties.py:1081-1087)
//   - self.add_style                (yj_to_epub_properties.py:2225-2237)
//   - self.get_fragment / get_named_fragment (yj_to_epub.py:294-331)
//   - template rendering via process_content (yj_to_epub_notebook.py:165)
//   - book_parts → spine/OEBPS files (epub_output.py generate_epub; omit=True
//     template parts excluded per yj_to_epub_notebook.py:179)

// scribeResourceFilename sanitizes a notebook SVG resource location and
// uniquifies it against the names already in use.
// Port of KFX_EPUB.resource_location_filename (yj_to_epub_resources.py:244-285)
// restricted to the notebook call shape:
// resource_location_filename("%s.svg" % name, "", self.IMAGE_FILEPATH, is_symbol=False).
// with IMAGE_FILEPATH = "/%s" and PLACE_FILES_IN_SUBDIRS = False (epub_output.py:249,238).
// Deviation: Go EPUB filenames omit the leading "/" (resources are written as
// OEBPS/<filename>), so "/page-1.svg" becomes "page-1.svg".
func scribeResourceFilename(location string, used map[string]struct{}) string {
	if location == "" {
		return ""
	}

	// Python L249-250: if location.startswith("/"): location = "_" + location[1:]
	if strings.HasPrefix(location, "/") {
		location = "_" + location[1:]
	}

	// Python L252-253: sanitize and collapse double slashes.
	safeLocation := sanitizeLocation(location)
	safeLocation = strings.ReplaceAll(safeLocation, "//", "/x/")

	// Python L255-256: path, sep, name = safe_location.rpartition("/")
	path, name := "", safeLocation
	if idx := strings.LastIndex(safeLocation, "/"); idx >= 0 {
		path = safeLocation[:idx+1]
		name = safeLocation[idx+1:]
	}

	// Python L258-260: root, sep, ext = name.rpartition(".")
	root, ext := name, ""
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		root = name[:idx]
		ext = name[idx:]
	}

	// Python L262-269: is_symbol is False for notebook SVGs, so no unique-part
	// prefixing is applied.

	// Python L271-273: strip "resource/" and IMAGE_FILEPATH directory prefixes.
	// IMAGE_FILEPATH "/%s" has no directory component, so only "resource/" applies.
	if strings.HasPrefix(path, "resource/") {
		path = path[len("resource/"):]
	}

	// Python L275: safe_filename = filepath_template % (path + root + suffix + ext)
	// (suffix is always "" for notebook SVGs).
	safeFilename := path + root + ext

	// Python L277-283: uniquify against existing oebps files (case-insensitive).
	for n := 0; ; n++ {
		candidate := safeFilename
		if n > 0 {
			candidate = fmt.Sprintf("%s-%d%s", safeFilename, n, ext)
		}
		key := strings.ToLower(candidate)
		if _, dup := used[key]; !dup {
			used[key] = struct{}{}
			return candidate
		}
	}
}

// scribeResourceMediaType maps a resource filename extension to its manifest
// media type. Port of EPUB_Output.mimetype_of_filename for the notebook SVG case.
func scribeResourceMediaType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}

// addSVGStyle applies CSS declarations to an SVG element's style attribute.
// Mirror of the htmlElement addStyle port (yj_to_epub_properties.go) of Python
// KFX_EPUB_Properties.add_style (yj_to_epub_properties.py:2225-2237).
func addSVGStyle(elem *svgElement, style map[string]string, replace bool) {
	if len(style) == 0 {
		return
	}
	if orig := elem.Attrib["style"]; orig != "" {
		existing := parseDeclarationString(orig)
		for k, v := range style {
			if replace {
				existing[k] = v
			} else if _, has := existing[k]; !has {
				existing[k] = v
			}
		}
		elem.setAttrib("style", styleStringFromMap(existing))
		return
	}
	elem.setAttrib("style", styleStringFromMap(styleCopy(style)))
}

// buildScribeNotebookContext assembles the production ScribeNotebookContext
// from the real book state. Returns nil when the book has no Scribe notebook
// sections, leaving the non-scribe pipeline untouched.
func buildScribeNotebookContext(
	book *decodedBook,
	frags fragmentCatalog,
	renderer *storylineRenderer,
	storylines map[string]map[string]interface{},
	sectionFragments map[string]sectionFragment,
) *ScribeNotebookContext {
	if book == nil {
		return nil
	}
	if !book.IsScribeNotebook && !bookHasScribeSections(sectionFragments) {
		return nil
	}

	// Reading orders as map views (Python self.reading_orders).
	var readingOrders []map[string]interface{}
	for _, raw := range getReadingOrders(frags) {
		if ro, ok := asMap(raw); ok {
			readingOrders = append(readingOrders, ro)
		}
	}

	// Manifest resources: Python self.manifest_resource (epub_output.py:353-376)
	// deduplicates against manifest_files, while resource_location_filename
	// uniquifies against oebps_files — two distinct registries in Python, kept
	// distinct here as well.
	oebpsSeen := map[string]struct{}{}
	for _, res := range book.Resources {
		oebpsSeen[strings.ToLower(res.Filename)] = struct{}{}
	}
	manifestSeen := map[string]struct{}{}
	manifestResource := func(filename string, data []byte) {
		filename = strings.TrimPrefix(filename, "/")
		key := strings.ToLower(filename)
		if _, dup := manifestSeen[key]; dup {
			// Python manifest_resource L359-362: report and skip duplicates.
			log.Printf("kfx: error: Duplicate file name in manifest: %s", filename)
			return
		}
		manifestSeen[key] = struct{}{}
		// Python manifest_resource → add_oebps_file(filename, data, mimetype).
		oebpsSeen[key] = struct{}{}
		book.Resources = append(book.Resources, epub.Resource{
			Filename:  filename,
			MediaType: scribeResourceMediaType(filename),
			Data:      append([]byte(nil), data...),
		})
	}

	resourceLocationFilename := func(name string, subdir string, filepathTemplate string, isSymbol bool) string {
		_ = subdir
		_ = filepathTemplate
		_ = isSymbol
		return scribeResourceFilename(name, oebpsSeen)
	}

	var resolveResource ResourceResolver
	if renderer != nil {
		resolveResource = renderer.resolveResource
	}

	ctx := &ScribeNotebookContext{
		NmdlTemplateID:           asStringDefault(frags.DocumentData["nmdl.template_id"]),
		WritingMode:              book.WritingMode,
		NewBookPart:              func(filename string) *ScribeBookPart { return NewScribeBookPart(filename) },
		ManifestResource:         manifestResource,
		ResourceLocationFilename: resourceLocationFilename,
		ProcessContentProperties: func(section map[string]interface{}) map[string]string {
			return processContentProperties(section, resolveResource)
		},
		AddStyle:            addSVGStyle,
		SectionTextFilepath: "%s.xhtml",
		ImageFilepath:       "%s",
		// Python self.get_fragment / self.get_named_fragment (yj_to_epub.py:294-331).
		GetFragment: func(ftype string, fid string) map[string]interface{} {
			return getFragment(book, ftype, fid)
		},
		GetNamedFragment: func(content map[string]interface{}, ftype string, nameSymbol string) map[string]interface{} {
			if content == nil {
				return nil
			}
			name, _ := asString(content[nameSymbol])
			delete(content, nameSymbol)
			if name == "" {
				return nil
			}
			return storylines[name]
		},
		ReadingOrders: readingOrders,
		notebookContext: &notebookContext{
			getFragment: func(ftype string, fid string) map[string]interface{} {
				return getFragment(book, ftype, fid)
			},
			getNamedFragment: func(content map[string]interface{}, ftype string, nameSymbol string) map[string]interface{} {
				// Python get_named_fragment (yj_to_epub.py:328-329):
				// get_fragment(ftype, fid=structure.pop(name_symbol or FRAGMENT_NAME_SYMBOL[ftype]))
				if content == nil {
					return nil
				}
				name, _ := asString(content[nameSymbol])
				delete(content, nameSymbol)
				if name == "" {
					return nil
				}
				return storylines[name]
			},
			pathBundles: func() map[string]map[string]interface{} {
				if renderer != nil {
					return renderer.pathBundles
				}
				return nil
			}(),
		},
	}

	// Template section rendering: Python yj_to_epub_notebook.py:165 runs the
	// standard process_content pipeline and then locates the resulting SVG
	// (L170-171). See renderScribeTemplateSVG.
	if renderer != nil {
		ctx.RenderTemplateContent = func(pageTemplate map[string]interface{}) *svgElement {
			return renderScribeTemplateSVG(renderer, storylines, pageTemplate)
		}
	}

	return ctx
}

// bookHasScribeSections reports whether any section fragment carries notebook
// keys (Python yj_to_epub_content.py:144,147 checks these on the section).
func bookHasScribeSections(sectionFragments map[string]sectionFragment) bool {
	for _, section := range sectionFragments {
		if sectionHasNmdlKey(section.PageTemplateValues, "nmdl.canvas_width") ||
			sectionHasNmdlKey(section.RawValue, "nmdl.canvas_width") ||
			sectionHasNmdlKey(section.PageTemplateValues, "nmdl.template_type") ||
			sectionHasNmdlKey(section.RawValue, "nmdl.template_type") {
			return true
		}
	}
	return false
}

// renderScribeTemplateSVG renders a notebook template page template through the
// standard content pipeline and extracts the resulting SVG element.
// Port of yj_to_epub_notebook.py:165-171:
//
//	self.process_content(page_template, top_level_elem, book_part, self.writing_mode, is_section=True)
//	...
//	svg_elem = book_part.body().find(SVG)
//
// Go's storylineRenderer is the process_content equivalent (see
// renderSectionFragments for the same composition). Python searches direct
// children of <body> (process_content retags top-level content as body);
// Go's renderer nests content under the section root, so the first svg
// descendant is located instead.
func renderScribeTemplateSVG(renderer *storylineRenderer, storylines map[string]map[string]interface{}, pageTemplate map[string]interface{}) *svgElement {
	if renderer == nil || pageTemplate == nil {
		return nil
	}

	var storyline map[string]interface{}
	var nodes []interface{}
	// Python process_content pops the content type ($159), location id ($155)
	// and style ($157) from the page template, and the container branch pops the
	// story name ($176) via get_named_fragment before rendering (check_empty
	// bookkeeping at yj_to_epub_notebook.py:167).
	delete(pageTemplate, "type")
	positionID, _ := asInt(pageTemplate["id"])
	delete(pageTemplate, "id")
	styleID, _ := asString(pageTemplate["style"])
	delete(pageTemplate, "style")
	if storyName, ok := asString(pageTemplate["story_name"]); ok && storyName != "" {
		delete(pageTemplate, "story_name")
		storyline = storylines[storyName]
		if storyline == nil {
			return nil
		}
		nodes, _ = asSlice(storyline["content_list"])
	} else if contentList, ok := asSlice(pageTemplate["content_list"]); ok {
		storyline = map[string]interface{}{}
		nodes = contentList
	} else {
		// The template page template may itself be the content object.
		storyline = map[string]interface{}{}
		nodes = []interface{}{pageTemplate}
	}

	rendered := renderer.renderStoryline(positionID, styleID, nil, storyline, nodes)
	if rendered.Root == nil {
		return nil
	}
	svgHTML := findFirstDescendantByTag(rendered.Root, "svg")
	if svgHTML == nil {
		return nil
	}
	return htmlElementToSVG(svgHTML, true)
}

// htmlElementToSVG converts a rendered htmlElement tree into the notebook
// module's svgElement tree. ensureNamespaces adds xmlns/xmlns:xlink
// declarations that Python's set_nsmap(SVG_NAMESPACES) attaches to SVG content
// elements (yj_to_epub_content.py:825) and that lxml preserves in the
// standalone template SVG document.
func htmlElementToSVG(elem *htmlElement, ensureNamespaces bool) *svgElement {
	if elem == nil {
		return nil
	}
	converted := &svgElement{Tag: elem.Tag, Attrib: map[string]string{}}
	for k, v := range elem.Attrs {
		converted.Attrib[k] = v
	}
	if ensureNamespaces && elem.Tag == "svg" {
		if _, has := converted.Attrib["xmlns"]; !has {
			converted.Attrib["xmlns"] = "http://www.w3.org/2000/svg"
		}
		if _, has := converted.Attrib["xmlns:xlink"]; !has {
			converted.Attrib["xmlns:xlink"] = "http://www.w3.org/1999/xlink"
		}
	}
	for _, child := range elem.Children {
		switch typed := child.(type) {
		case *htmlElement:
			converted.Children = append(converted.Children, htmlElementToSVG(typed, false))
		case htmlText:
			converted.Text = typed.Text
		case *htmlText:
			converted.Text = typed.Text
		}
	}
	return converted
}

// svgElementToHTML converts a Scribe book part body tree into htmlElement
// parts for materialization into renderedSection.Root.
func svgElementToHTML(elem *svgElement) *htmlElement {
	if elem == nil {
		return nil
	}
	converted := &htmlElement{Tag: elem.Tag, Attrs: map[string]string{}}
	for k, v := range elem.Attrib {
		converted.Attrs[k] = v
	}
	if elem.Text != "" {
		converted.Children = append(converted.Children, htmlText{Text: elem.Text})
	}
	for _, child := range elem.Children {
		converted.Children = append(converted.Children, svgElementToHTML(child))
	}
	return converted
}

// materializeScribeNotebookSections appends the Scribe page book parts to the
// rendered sections in creation (= reading) order, matching Python where
// new_book_part appends to self.book_parts and generate_epub emits each one
// except parts with omit=True (set for template sections at
// yj_to_epub_notebook.py:179). Materialization is deferred until every section
// has been processed because the template section mutates previously created
// page book parts (yj_to_epub_notebook.py:181-201).
func materializeScribeNotebookSections(book *decodedBook, scribeCtx *ScribeNotebookContext, navTitles map[string]string) {
	if book == nil || scribeCtx == nil {
		return
	}
	for _, part := range scribeCtx.BookParts {
		if part.Omit {
			continue
		}
		root := &htmlElement{}
		for _, child := range part.Body.Children {
			root.Children = append(root.Children, svgElementToHTML(child))
		}
		// Python set_html_defaults later fills body font defaults for every
		// book part (yj_to_epub_properties.py:1652-1573); Go's setHTMLDefaults
		// performs the same pass over RenderedSections after this point.
		book.RenderedSections = append(book.RenderedSections, renderedSection{
			Filename:       strings.TrimPrefix(part.Filename, "/"),
			Title:          navTitles[part.PageTitle],
			PageTitle:      part.PageTitle,
			Language:       normalizeLanguage(book.Language),
			IsFixedLayout:  part.IsFXL,
			ViewportWidth:  part.ViewportWidth,
			ViewportHeight: part.ViewportHeight,
			Properties:     "svg", // epub_output.py:705-706: body contains SVG
			Root:           root,
		})
	}
}
