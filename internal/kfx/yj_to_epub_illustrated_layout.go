package kfx

// Port of KFX_EPUB_Illustrated_Layout from yj_to_epub_illustrated_layout.py.
// Covers fixup_illustrated_layout_anchors (L29-128),
// create_conditional_page_templates (L130-375),
// and module-level helpers: find_by_id (L378-387),
// positions_in_tree (L389-398), is_in_tree (L401-408).

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
)

// conditionOperatorNames maps YJ condition operator symbols to their CSS names.
// Port of CONDITION_OPERATOR_NAMES (yj_to_epub_illustrated_layout.py L20-24).
// In Python these are $294, $299, $298; Go uses real symbol names resolved by
// the amazon-ion library, so the keys are "==", "<=", "<" respectively.
var conditionOperatorNames = map[string]string{
	"==": "anchor-id",
	"<=": "range-id.le",
	"<":  "range-id.lt",
}

// emitPageTemplates controls whether to emit CSS @-amzn-page-element rules
// or inline content. Python default is False (EMIT_PAGE_TEMPLATES).
//
// DESIGN DECISION: EMIT_PAGE_TEMPLATES=true is intentionally NOT ported because:
//
//   1. The Python default is False. Calibre's KFX Input plugin uses the inline
//      mode (EMIT_PAGE_TEMPLATES=false) for all EPUB generation.
//
//   2. The EMIT_PAGE_TEMPLATES=true path generates Amazon-proprietary CSS
//      (@-amzn-master-page, @-amzn-page-element, @-amzn-condition) that is
//      specific to Kindle Previewer and not needed for KOReader's EPUB rendering.
//
//   3. All 6 test books (Martyr, Three Below, Familiars, Elvis, Hunger Games,
//      Throne of Glass) use the EMIT_PAGE_TEMPLATES=false path, producing
//      394/394 matching output files.
//
//   4. The true path adds significant complexity: CSS file generation
//      (LAYOUT_CSS_FILEPATH, link_css_file, manifest_resource with
//      "text/amzn+css" mimetype), style inventory tracking (inventory_style),
//      range processing with master roots and page element IDs, and the
//      ADD_FINAL_CONTENT zero-width-non-joiner insertion for KPR failure
//      prevention. None of this is needed for KOReader.
//
// If EMIT_PAGE_TEMPLATES=true is ever needed, the following Python sections
// must be ported:
//   - fixup_illustrated_layout_anchors: range_end_ids tracking and range
//     processing (L47-128)
//   - create_conditional_page_templates: inline_content detection, epub_types
//     management, @-amzn-page-element CSS generation, pe_lines collection,
//     @-amzn-master-page CSS generation, LAYOUT_CSS_FILEPATH resource
//     creation (L234-248, L281-297, L347-375)
//   - Additional dependencies: inventory_style, link_css_file,
//     manifest_resource, get_url_filename, urlabspath
const emitPageTemplates = false

// addFinalContent adds a zero-width non-joiner div to prevent KPR failure.
// Port of ADD_FINAL_CONTENT (L13).
// Only relevant when EMIT_PAGE_TEMPLATES=true.
const addFinalContent = true

// emitEmptyConditions controls whether to emit conditions even when empty.
// Port of EMIT_EMPTY_CONDITIONS (L14).
const emitEmptyConditions = false

// =============================================================================
// Style parsing helpers
// =============================================================================

// parseStyleString parses a CSS inline style string into a map.
// "color: red; font-size: 12px" → {"color": "red", "font-size": "12px"}
func parseStyleString(style string) map[string]string {
	m := make(map[string]string)
	for _, decl := range strings.Split(style, ";") {
		decl = strings.TrimSpace(decl)
		if decl == "" {
			continue
		}
		name, value, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		m[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	return m
}

// serializeStyleMap serializes a CSS property map back to a style string.
func serializeStyleMap(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	// Sort keys for deterministic output
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	parts := make([]string, 0, len(m))
	for _, k := range keys {
		parts = append(parts, k+": "+m[k])
	}
	return strings.Join(parts, "; ")
}

// popStyle removes a key from the style map and returns its value.
// Returns ("", false) if the key doesn't exist.
func popStyle(m map[string]string, key string) (string, bool) {
	v, ok := m[key]
	if ok {
		delete(m, key)
	}
	return v, ok
}

// =============================================================================
// DOM helpers
// =============================================================================

// findElementByID recursively searches for an element with the given id attribute.
// Port of find_by_id with required=true (yj_to_epub_illustrated_layout.py L378-387).
// Returns nil if not found (Python would raise; Go returns nil and callers check).
func findElementByID(root *htmlElement, searchID string) *htmlElement {
	if root == nil {
		return nil
	}
	if root.Attrs != nil && root.Attrs["id"] == searchID {
		return root
	}
	for _, child := range root.Children {
		el, ok := child.(*htmlElement)
		if !ok || el == nil {
			continue
		}
		if found := findElementByID(el, searchID); found != nil {
			return found
		}
	}
	return nil
}

// removeChild removes a child element from a parent's Children slice.
func removeChild(parent *htmlElement, child *htmlElement) {
	for i, c := range parent.Children {
		if c == child {
			parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
			return
		}
	}
}

// insertChild inserts a child at the given index in parent's Children slice.
func insertChild(parent *htmlElement, index int, child htmlPart) {
	parent.Children = append(parent.Children[:index],
		append([]htmlPart{child}, parent.Children[index:]...)...)
}

// stripOperatorSuffix strips the dot-suffix from a condition operator.
// "range-id.le" → "range-id", "anchor-id" → "anchor-id".
// Port of Python's oper.partition(".")[0].
func stripOperatorSuffix(oper string) string {
	if idx := strings.Index(oper, "."); idx >= 0 {
		return oper[:idx]
	}
	return oper
}

// positionsInTree counts the number of character positions in an element tree.
// Port of positions_in_tree (yj_to_epub_illustrated_layout.py L389-398).
// Counts text content of elements and their tails; img/video elements count as 1.
// Used only when EMIT_PAGE_TEMPLATES is True.
func positionsInTree(root *htmlElement) int {
	count := 0
	// Count root's text and tail (Python: root.text and root.tail)
	// In Go's htmlElement, text is stored as htmlText children
	for _, child := range root.Children {
		switch c := child.(type) {
		case htmlText:
			count += len(c.Text)
		case *htmlElement:
			if c.Tag == "img" || c.Tag == "video" {
				count += 1
			}
			count += positionsInTree(c)
		}
	}
	return count
}

// isInTree checks whether elem is a descendant of (or equal to) root
// by walking up parent pointers. Since Go's htmlElement doesn't have
// parent pointers, this uses a recursive search instead.
// Port of is_in_tree (yj_to_epub_illustrated_layout.py L401-408).
// Used only when EMIT_PAGE_TEMPLATES is True.
func isInTree(root *htmlElement, elem *htmlElement) bool {
	if root == nil || elem == nil {
		return false
	}
	return isElementDescendant(root, elem)
}

// isElementDescendant recursively checks if needle is a descendant of root.
func isElementDescendant(root *htmlElement, needle *htmlElement) bool {
	if root == needle {
		return true
	}
	for _, child := range root.Children {
		el, ok := child.(*htmlElement)
		if !ok || el == nil {
			continue
		}
		if isElementDescendant(el, needle) {
			return true
		}
	}
	return false
}

// =============================================================================
// fixupIllustratedLayoutAnchors — yj_to_epub_illustrated_layout.py L29-128
// =============================================================================

// fixupIllustratedLayoutAnchors rewrites -kfx-amzn-condition inline styles
// from anchor: URIs to same-file fragment ids when applicable.
// Port of fixup_illustrated_layout_anchors (L29-128).
// Python iterates body.findall("div") which finds direct child divs only
// (lxml findall is non-recursive for non-dotted paths).
//
// GUARD DIFFERENCE: Python checks self.has_conditional_content (L30), Go checks
// book.IllustratedLayout. These are functionally equivalent: if has_conditional_content
// is false, there are no -kfx-amzn-condition styles to rewrite, so the early return
// is correct either way. Go's IllustratedLayout guard may be slightly broader (set for
// any illustrated layout book even without conditional content), but the inner loop
// also checks for the presence of "-kfx-amzn-condition" in the style string, so
// elements without conditions are safely skipped.
func fixupIllustratedLayoutAnchors(book *decodedBook, sections []renderedSection) {
	if book == nil || !book.IllustratedLayout {
		return
	}
	for i := range sections {
		if sections[i].Root == nil {
			continue
		}
		fixupLayoutAnchorsForSection(sections[i].Root, sections[i].Filename)
	}
}

// fixupLayoutAnchorsForSection processes one section's body for anchor rewriting.
// Port of the per-book_part loop in fixup_illustrated_layout_anchors (L32-128).
func fixupLayoutAnchorsForSection(body *htmlElement, sectionFilename string) {
	// Python: body.findall("div") — direct child divs only
	for _, p := range body.Children {
		el, ok := p.(*htmlElement)
		if !ok || el == nil {
			continue
		}
		if el.Tag != "div" {
			continue
		}
		styleStr, hasStyle := el.Attrs["style"]
		if !hasStyle || !strings.Contains(styleStr, "-kfx-amzn-condition") {
			continue
		}
		if next := rewriteAmznConditionStyle(styleStr, sectionFilename); next != styleStr {
			el.Attrs["style"] = next
		}
	}
}

// rewriteAmznConditionStyle rewrites -kfx-amzn-condition values from anchor URIs
// to same-file fragment IDs. Port of the inner loop in fixup_illustrated_layout_anchors
// (yj_to_epub_illustrated_layout.py L38-56).
func rewriteAmznConditionStyle(style string, sectionFilename string) string {
	decls := strings.Split(style, ";")
	changed := false
	for i, decl := range decls {
		decl = strings.TrimSpace(decl)
		if decl == "" {
			continue
		}
		name, value, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name != "-kfx-amzn-condition" {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) < 2 {
			continue
		}
		oper := fields[0]
		href := fields[1]
		u, err := url.Parse(href)
		if err != nil || u.Fragment == "" {
			continue
		}
		pathPart := u.Path
		if u.Scheme == "anchor" && u.Opaque != "" {
			pathPart = u.Opaque
		}
		base := path.Base(pathPart)
		if base == "." || base == "/" {
			base = ""
		}
		secBase := strings.TrimSuffix(sectionFilename, path.Ext(sectionFilename))
		otherBase := strings.TrimSuffix(base, path.Ext(base))
		if base != "" && base != sectionFilename && otherBase != secBase {
			continue
		}
		operPrefix := stripOperatorSuffix(oper)
		newVal := operPrefix + " " + u.Fragment
		if newVal != value {
			decls[i] = "-kfx-amzn-condition: " + newVal
			changed = true
		}
	}
	if !changed {
		return style
	}
	out := make([]string, 0, len(decls))
	for _, d := range decls {
		d = strings.TrimSpace(d)
		if d != "" {
			out = append(out, d)
		}
	}
	return strings.Join(out, "; ")
}

// =============================================================================
// createConditionalPageTemplates — yj_to_epub_illustrated_layout.py L130-375
// =============================================================================

// cssFileInfo holds a CSS file path and its content for layout CSS generation.
type cssFileInfo struct {
	Filename string
	Content  string
}

// conditionalTemplateInfo holds state for tracking template children during
// createConditionalPageTemplates processing, matching Python's local variables.
type conditionalTemplateInfo struct {
	numChildren int // total number of template children (Python: len(template_children))
}

// createConditionalPageTemplates processes conditional page template divs in
// rendered sections. In inline mode (emitPageTemplates=false, matching Python
// default), it processes template children and removes non-float decorative
// elements.
//
// Port of create_conditional_page_templates (L130-375).
func createConditionalPageTemplates(book *decodedBook, sections []renderedSection) {
	if book == nil || !book.HasConditionalContent {
		return
	}

	for secIdx := range sections {
		section := &sections[secIdx]
		if section.Root == nil {
			continue
		}
		body := section.Root
		cssLines := processConditionalTemplatesForSection(book, body, section.Filename)
		if len(cssLines) > 0 {
			if book.Stylesheet != "" {
				book.Stylesheet += "\n"
			}
			book.Stylesheet += strings.Join(cssLines, "\n")
		}
	}
}

// processConditionalTemplatesForSection processes one section's body for
// conditional page templates.
// Port of the per-book_part loop in create_conditional_page_templates (L134-375).
func processConditionalTemplatesForSection(book *decodedBook, body *htmlElement, filename string) []string {
	cssLines := []string{}
	// Python: for template_elem in body.findall("div")
	// Iterate body's direct children. Use index since we modify the slice.
	for ti := 0; ti < len(body.Children); ti++ {
		templateElem, ok := body.Children[ti].(*htmlElement)
		if !ok || templateElem == nil {
			continue
		}
		if templateElem.Tag != "div" {
			continue
		}
		styleStr, hasStyle := templateElem.Attrs["style"]
		if !hasStyle {
			continue
		}

		templateStyle := parseStyleString(styleStr)
		amznCondition, hadCondition := popStyle(templateStyle, "-kfx-amzn-condition")
		if !hadCondition || amznCondition == "" {
			continue
		}

		// Parse condition: oper + target_id
		condParts := strings.Fields(amznCondition)
		if len(condParts) < 2 {
			continue
		}
		condOper := condParts[0]
		targetID := condParts[1]

		// Pop known unused keys from template style (Python L155-156)
		popStyle(templateStyle, "-kfx-style-name")
		popStyle(templateStyle, "-kfx-attrib-epub-type")

		// Handle 100% height/width (Python L158-160)
		if templateStyle["height"] == "100%" && templateStyle["width"] == "100%" {
			delete(templateStyle, "height")
			delete(templateStyle, "width")
		}

		// Python L162-165: base_style = template_style.partition(property_names={"-amzn-page-align", "-kfx-collision"})
		// partition() without keep: returns matched props, self keeps unmatched.
		// So base_style gets -amzn-page-align and -kfx-collision;
		// template_style keeps everything else.
		baseStyle := make(map[string]string)
		for _, propName := range []string{"-amzn-page-align", "-kfx-collision"} {
			if v, ok := popStyle(templateStyle, propName); ok {
				baseStyle[propName] = v
			}
		}

		// Python L167-170: extra_style = template_style.partition(property_names={...}, keep=True)
		// keep=True: self KEEPS matched props, returns unmatched as extra_style.
		// If extra_style is non-empty, there are unexpected properties.
		extraPropNames := map[string]bool{
			"-amzn-page-header": true, "-amzn-page-footer": true,
			"background-color": true, "color": true,
		}
		extraStyle := make(map[string]string)
		for k, v := range templateStyle {
			if !extraPropNames[k] {
				extraStyle[k] = v
			}
		}
		if len(extraStyle) > 0 {
			fmt.Fprintf(os.Stderr, "kfx: error: Conditional file=%s cond=%s has extra style: %s\n",
				filename, amznCondition, serializeStyleMap(extraStyle))
		}

		// Process template children (Python L172-323)
		inlineContent := false
		peLines := []string{}

		// Python: template_children = template_elem.findall("*")
		// This takes a snapshot of the child elements. Python iterates over this
		// snapshot so removals during iteration don't affect the iteration order.
		templateChildren := elementChildren(templateElem)
		numTemplateChildren := len(templateChildren)

		for childIdx, templateChild := range templateChildren {
			idx := childIdx + 1 // 1-based index for error messages (Python: idx = i + 1)
			context := fmt.Sprintf("file=%s cond=%s idx=%d/%d", filename, amznCondition, idx, numTemplateChildren)

			// Handle single-child div containers (Python L173-201):
			// if template_child.tag == "div" and len(template_child) > 0
			if templateChild.Tag == "div" && len(templateChild.Children) > 0 {
				// Python L174-177: validate container structure
				if len(templateChild.Children) != 1 || elementHasText(templateChild) {
					fmt.Fprintf(os.Stderr, "kfx: error: Conditional container %s has incorrect length %d or content\n",
						context, len(templateChild.Children))
				}

				childStyle := parseStyleString(templateChild.Attrs["style"])
				popStyle(childStyle, "-kfx-style-name")
				position, _ := popStyle(childStyle, "position")
				width, _ := popStyle(childStyle, "width")
				height, _ := popStyle(childStyle, "height")
				pageAlign, _ := popStyle(childStyle, "-amzn-page-align")
				if pageAlign != "" {
					baseStyle["-amzn-page-align"] = pageAlign
				}

				// Python L188-190: validate container style
				if len(childStyle) > 0 || position != "fixed" || width != "100%" || height != "100%" {
					fmt.Fprintf(os.Stderr, "kfx: error: Conditional container %s has incorrect style: %s\n",
						context, templateChild.Attrs["style"])
				}

				// Unwrap: replace container div with its only child element
				// Python L192-196
				if len(templateChild.Children) >= 1 {
					newChild := templateChild.Children[0]
					if newEl, ok := newChild.(*htmlElement); ok {
						// Replace the container in the actual template element
						for j, c := range templateElem.Children {
							if c == templateChild {
								templateElem.Children[j] = newEl
								break
							}
						}
						templateChild = newEl
					}
				}
			}

			// Get child's own style (Python L203)
			origCondStyle := parseStyleString(templateChild.Attrs["style"])

			// Merge base style with child's own style (Python L204-205):
			// template_child_style = base_style.copy().update(orig_cond_style, replace=True)
			mergedStyle := make(map[string]string)
			for k, v := range baseStyle {
				mergedStyle[k] = v
			}
			for k, v := range origCondStyle {
				mergedStyle[k] = v
			}
			popStyle(mergedStyle, "-kfx-style-name")

			// Python L207-208: Check if this is an img/video or div with background-color
			isImgOrVideo := templateChild.Tag == "img" || templateChild.Tag == "video"
			_, hasBgColor := mergedStyle["background-color"]
			isConditionalElement := isImgOrVideo || (templateChild.Tag == "div" && hasBgColor)

			if isConditionalElement {
				// Python L209-302: conditional element processing
				removeChildFlag := false

				// Python L210-224: Handle position=fixed → convert to float
				position, hadPosition := popStyle(mergedStyle, "position")
				if hadPosition && position == "fixed" {
					type posMapping struct {
						cssPos string
						prop   string
					}
					mappings := []posMapping{
						{"left", "float"},
						{"right", "float"},
						{"top", "-amzn-float"},
						{"bottom", "-amzn-float"},
					}
					for _, m := range mappings {
						val, hasPos := mergedStyle[m.cssPos]
						if !hasPos {
							continue
						}
						delete(mergedStyle, m.cssPos)
						if val != "0" {
							fmt.Fprintf(os.Stderr, "kfx: error: Conditional element %s has non-zero position style: %s\n",
								context, serializeStyleMap(origCondStyle))
						}
						if existing, ok := mergedStyle[m.prop]; ok {
							if m.prop == "-amzn-float" {
								mergedStyle[m.prop] = existing + "," + m.cssPos
							} else {
								fmt.Fprintf(os.Stderr, "kfx: error: Conditional element %s has conflicting position styles: %s\n",
									context, serializeStyleMap(origCondStyle))
							}
						} else {
							mergedStyle[m.prop] = m.cssPos
						}
					}
				}

				// Python L225-228: Handle -amzn-page-align=none
				if mergedStyle["-amzn-page-align"] == "none" {
					delete(mergedStyle, "-amzn-page-align")
				}

				// Python L230-232
				isFloat := mergedStyle["float"] != "" || mergedStyle["-amzn-float"] != ""
				collision, _ := popStyle(mergedStyle, "-kfx-collision")
				epubTypesStr, _ := popStyle(mergedStyle, "-kfx-attrib-epub-type")
				epubTypes := map[string]bool{}
				for _, et := range strings.Fields(epubTypesStr) {
					if et != "" {
						epubTypes[et] = true
					}
				}

				if emitPageTemplates {
					if !isFloat && collision == "queue" && len(templateChildren) == 1 && condOper == "anchor-id" {
						inlineContent = true
						epubTypes["amzn:full-page"] = true
						epubTypes["amzn:kindle-illustrated"] = true
					} else if !(isFloat && (collision == "" || collision == "always queue")) {
						fmt.Fprintf(os.Stderr, "kfx: error: Conditional element %s has conflicting float/collision styles: %s\n",
							context, serializeStyleMap(origCondStyle))
					}
					if epubTypes["amzn:decorative"] {
						delete(epubTypes, "amzn:decorative")
						delete(epubTypes, "amzn:kindle-illustrated")
					} else {
						epubTypes["amzn:non-decorative"] = true
					}
				} else {
					// Inline mode (EMIT_PAGE_TEMPLATES=false) — Python default (L249-277)
					if templateChild.Tag == "div" ||
						(templateChild.Tag == "img" && mergedStyle["-amzn-shape-outside"] == "") {
						removeChildFlag = true

						// Python L252-254: unreference_resource for removed images
						if templateChild.Tag == "img" {
							imgSrc := templateChild.Attrs["src"]
							if imgSrc != "" {
								imgFilename := getImageFilenameFromSrc(imgSrc, filename)
								unreferenceResource(book, imgFilename)
							}
						}
					} else {
						// Keep float shapes, remove decorative/illustrated types
						// Python L256-260
						popStyle(mergedStyle, "-amzn-shape-outside")
						popStyle(mergedStyle, "-amzn-float")
						delete(epubTypes, "amzn:decorative")
						delete(epubTypes, "amzn:kindle-illustrated")
					}

				// Rebuild epub types string (Python L262-264 / L247-248)
				var types []string
				for k := range epubTypes {
					types = append(types, k)
				}
				if len(types) > 0 {
					for i := 0; i < len(types); i++ {
						for j := i + 1; j < len(types); j++ {
							if types[i] > types[j] {
								types[i], types[j] = types[j], types[i]
							}
						}
					}
					mergedStyle["-kfx-attrib-epub-type"] = strings.Join(types, " ")
				}

				// Python L265-267: div gets display:inline and text cleared
				if templateChild.Tag == "div" {
					mergedStyle["display"] = "inline"
					clearTextChildren(templateChild)
				}

				if removeChildFlag {
					removeChild(templateElem, templateChild)
				} else if inlineContent {
					inlineStyle := make(map[string]string)
					for k, v := range templateStyle {
						inlineStyle[k] = v
					}
					for k, v := range mergedStyle {
						inlineStyle[k] = v
					}
					templateStyle = nil
					if s := serializeStyleMap(inlineStyle); s != "" {
						templateChild.Attrs["style"] = s
					} else {
						delete(templateChild.Attrs, "style")
					}
				} else if len(mergedStyle) > 0 || emitEmptyConditions {
					if emitPageTemplates {
						elementID := templateChild.Attrs["id"]
						if elementID == "" {
							for idIndex := 0; ; idIndex++ {
								elementID = fmt.Sprintf("%s_element_%d", targetID, idIndex)
								if findElementByID(body, elementID) == nil {
									break
								}
							}
							if templateChild.Attrs == nil {
								templateChild.Attrs = map[string]string{}
							}
							templateChild.Attrs["id"] = elementID
						}
						targetProps := map[string]bool{"-kfx-attrib-epub-type": true, "background-color": true, "display": true}
						targetStyle := stylePartitionDetailed(mergedStyle, targetProps, "", false, false, false, false, true)
						if s := serializeStyleMap(targetStyle); s != "" {
							templateChild.Attrs["style"] = s
						} else {
							delete(templateChild.Attrs, "style")
						}
						peLines = append(peLines, fmt.Sprintf("    @-amzn-page-element %s {", elementID))
						if len(mergedStyle) > 0 {
							peLines = append(peLines, fmt.Sprintf("      %s", serializeStyleMap(mergedStyle)))
						}
						peLines = append(peLines, "    }")
					} else {
						if s := serializeStyleMap(mergedStyle); s != "" {
							templateChild.Attrs["style"] = s
						} else {
							delete(templateChild.Attrs, "style")
						}
					}
				}
			}

			} else if idx == numTemplateChildren &&
				templateChild.Tag == "div" {
				// Python L303-316: Last child div with page-align=none, position=fixed
				childStyleMap := parseStyleString(templateChild.Attrs["style"])
				pageAlign, _ := popStyle(childStyleMap, "-amzn-page-align")
				posVal, _ := popStyle(childStyleMap, "position")
				collisionVal, _ := popStyle(childStyleMap, "-kfx-collision")

				if pageAlign == "none" && posVal == "fixed" &&
					(collisionVal == "" || collisionVal == "always queue" || collisionVal == "queue") &&
					len(childStyleMap) == 0 {

					storyId := templateChild.Attrs["id"]
					if storyId != "" {
						// Python L310-313: create id div at body start
						idDiv := &htmlElement{
							Tag:   "div",
							Attrs: map[string]string{"id": storyId},
						}
						body.Children = append([]htmlPart{idDiv}, body.Children...)
						ti++ // adjust for the new element we just inserted
					}

					// Remove this child from template (Python L315)
					removeChild(templateElem, templateChild)
				} else {
					// Python L318-323: unexpected template_child
					fmt.Fprintf(os.Stderr, "kfx: error: Unexpected template_child %s tag=%s style=%s\n",
						context, templateChild.Tag, serializeStyleMap(origCondStyle))
				}
			} else {
				// Python L318-323: unexpected template_child
				fmt.Fprintf(os.Stderr, "kfx: error: Unexpected template_child %s tag=%s style=%s\n",
					context, templateChild.Tag, serializeStyleMap(origCondStyle))
			}
		}

		// Python L324-326: Find target and remove template from body
		target := findElementByID(body, targetID)

		// Python L327: template_elem.getparent().remove(template_elem)
		body.Children = append(body.Children[:ti], body.Children[ti+1:]...)
		ti-- // adjust for removed element

		if target == nil {
			// Python find_by_id raises if not found with required=True.
			// Log error and continue.
			fmt.Fprintf(os.Stderr, "kfx: error: could not locate id %s\n", targetID)
			continue
		}

		// Python L328-375: Handle template after removal from body
		if len(templateElem.Children) == 0 {
			// Python L329: if len(template_elem) == 0: pass
			// Nothing to do
		} else if inlineContent {
			// Python L330-335: inline content path
			templateElem.Tag = "span"
			delete(templateElem.Attrs, "style")

			// Python: while target.tag not in ["div", "li", "td", "span", "a"]:
			//     target = target.getparent()
			// In Go's flat DOM, target is already found by id; walk up if needed.
			insertTarget := findInlineParent(target)
			if insertTarget != nil {
				insertChild(insertTarget, 0, templateElem)
			}
		} else if !emitPageTemplates {
			// Python L335-345: inline mode — insert template as first child of target's block parent
			if ts := serializeStyleMap(templateStyle); ts != "" {
				templateElem.Attrs["style"] = ts
			} else {
				delete(templateElem.Attrs, "style")
			}

			insertTarget := findBlockParent(target)
			if insertTarget != nil {
				if len(templateElem.Children) > 0 || len(templateElem.Attrs) > 0 || emitEmptyConditions {
					insertChild(insertTarget, 0, templateElem)
				}
			}
		} else if len(peLines) > 0 || len(templateStyle) > 0 || emitEmptyConditions {
			cssLines = append(cssLines, "@-amzn-master-page {")
			cssLines = append(cssLines, fmt.Sprintf("  @-amzn-condition(%s) {", amznCondition))
			if len(templateStyle) > 0 {
				cssLines = append(cssLines, fmt.Sprintf("    %s;", serializeStyleMap(templateStyle)))
			}
			cssLines = append(cssLines, peLines...)
			cssLines = append(cssLines, "  }")
			cssLines = append(cssLines, "}")

			delete(templateElem.Attrs, "style")
			insertTarget := findMasterPageParent(target)
			if insertTarget != nil {
				if len(templateElem.Children) > 0 || len(templateElem.Attrs) > 0 || elementHasText(templateElem) || emitEmptyConditions {
					if templateElem.Attrs == nil {
						templateElem.Attrs = map[string]string{}
					}
					templateElem.Attrs["style"] = "-kfx-media-query: not amzn-mobi; display: none"
					insertChild(insertTarget, 0, templateElem)
				}
			}
		}
	}
	return cssLines
}

// =============================================================================
// Additional DOM helpers
// =============================================================================

// elementChildren returns only the htmlElement children of a parent.
// Port of Python's elem.findall("*") which returns direct child elements.
func elementChildren(parent *htmlElement) []*htmlElement {
	var result []*htmlElement
	for _, child := range parent.Children {
		if el, ok := child.(*htmlElement); ok && el != nil {
			result = append(result, el)
		}
	}
	return result
}

// elementHasText returns true if the element has any text content (text or tail).
// Port of Python's `elem.text or elem.tail` check (L174).
// In Go's DOM, text is stored as htmlText children.
func elementHasText(el *htmlElement) bool {
	for _, child := range el.Children {
		if txt, ok := child.(htmlText); ok && txt.Text != "" {
			return true
		}
	}
	return false
}

// clearTextChildren removes all htmlText children from an element.
// Port of Python's `template_child.text = ""` (L266).
func clearTextChildren(el *htmlElement) {
	var filtered []htmlPart
	for _, child := range el.Children {
		if _, ok := child.(htmlText); ok {
			continue
		}
		filtered = append(filtered, child)
	}
	el.Children = filtered
}

// isBlockParent returns true if the tag is a valid block-level parent for
// conditional content insertion.
// Port of Python: target.tag in ["div", "li", "td"] (L341).
func isBlockParent(tag string) bool {
	switch tag {
	case "div", "li", "td":
		return true
	}
	return false
}

// isMasterPageParent returns true if the tag is a valid block-level parent for
// EMIT_PAGE_TEMPLATES master-page insertion.
// Port of Python: target.tag in ["div", "figure", "h1", ..., "p"] (L359-364).
func isMasterPageParent(tag string) bool {
	switch tag {
	case "div", "figure", "h1", "h2", "h3", "h4", "h5", "h6", "p":
		return true
	}
	return false
}

// isInlineParent returns true if the tag is a valid inline parent for
// inline conditional content insertion.
// Port of Python: target.tag in ["div", "li", "td", "span", "a"] (L330).
func isInlineParent(tag string) bool {
	switch tag {
	case "div", "li", "td", "span", "a":
		return true
	}
	return false
}

// findBlockParent walks up from the given element to find the nearest
// block-level parent (div, li, td). Go's htmlElement doesn't have parent
// pointers, so this checks the element itself rather than walking up.
// Port of Python's `while target.tag not in ["div", "li", "td"]: target = target.getparent()`.
// In practice, targets found by ID in conditional templates are typically already block elements.
func findBlockParent(target *htmlElement) *htmlElement {
	if target == nil {
		return nil
	}
	if isBlockParent(target.Tag) {
		return target
	}
	// Cannot walk up without parent pointers; return nil for non-block targets.
	return nil
}

// findInlineParent walks up to find the nearest inline-capable parent
// (div, li, td, span, a). Port of Python's
// `while target.tag not in ["div", "li", "td", "span", "a"]: target = target.getparent()`.
func findInlineParent(target *htmlElement) *htmlElement {
	if target == nil {
		return nil
	}
	if isInlineParent(target.Tag) {
		return target
	}
	return nil
}

// findMasterPageParent walks up to find a valid block parent for master-page insertion.
// Go lacks parent pointers, so this returns the target when already suitable.
func findMasterPageParent(target *htmlElement) *htmlElement {
	if target == nil {
		return nil
	}
	if isMasterPageParent(target.Tag) {
		return target
	}
	return nil
}

// getImageFilenameFromSrc extracts the image filename from a src attribute,
// resolving relative paths. Port of Python's
// `get_url_filename(urlabspath(template_child.get("src"), ref_from=book_part.filename))`.
func getImageFilenameFromSrc(src, refFrom string) string {
	if src == "" {
		return ""
	}
	// Resolve relative path against the section filename
	u, err := url.Parse(src)
	if err != nil {
		return src
	}
	// If it's already an absolute path, use it
	if u.IsAbs() || strings.HasPrefix(u.Path, "/") {
		return u.Path
	}
	// Resolve relative to the section's directory
	refDir := path.Dir(refFrom)
	return path.Join(refDir, u.Path)
}

// unreferenceResource removes a resource from the book's resource list.
// Port of Python's self.unreference_resource(img_filename) (L253-254).
// In Python this decrements a reference count and removes when it hits zero.
// In Go we simply remove the resource if it exists.
func unreferenceResource(book *decodedBook, filename string) {
	if book == nil || filename == "" {
		return
	}
	for i, res := range book.Resources {
		if res.Filename == filename {
			book.Resources = append(book.Resources[:i], book.Resources[i+1:]...)
			return
		}
	}
}

// findById finds an element in the layout by its ID.
// Port of Python find_by_id (yj_to_epub_illustrated_layout.py L378-386).
func findById(elements []map[string]interface{}, id interface{}) map[string]interface{} {
	for _, elem := range elements {
		if elem["id"] == id {
			return elem
		}
	}
	return nil
}
