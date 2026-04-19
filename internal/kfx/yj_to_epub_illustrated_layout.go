package kfx

// Port of KFX_EPUB_Illustrated_Layout from yj_to_epub_illustrated_layout.py.
// Covers fixup_illustrated_layout_anchors (L29-128) and
// create_conditional_page_templates (L130-419).

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
)

// conditionOperatorNames maps YJ condition operator symbols to their CSS names.
// Port of CONDITION_OPERATOR_NAMES (yj_to_epub_illustrated_layout.py L20-24).
var conditionOperatorNames = map[string]string{
	"$294": "anchor-id",
	"$299": "range-id.le",
	"$298": "range-id.lt",
}

// EMIT_PAGE_TEMPLATES controls whether to emit CSS @-amzn-page-element rules
// or inline content. Python default is False.
const emitPageTemplates = false

// ADD_FINAL_CONTENT adds a zero-width non-joiner div to prevent KPR failure.
const addFinalContent = true

// emitEmptyConditions controls whether to emit conditions even when empty.
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
	// Simple sort for now — Go doesn't guarantee map iteration order
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
// Port of find_by_id (yj_to_epub_illustrated_layout.py L394+).
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

// removeChildFromParent removes child from its parent element.
func removeChildFromParent(parent *htmlElement, child *htmlElement) {
	for i, c := range parent.Children {
		if c == child {
			parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
			return
		}
	}
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

// isBlockParent returns true if the tag is a valid block-level parent for
// conditional content insertion.
func isBlockParent(tag string) bool {
	switch tag {
	case "div", "li", "td":
		return true
	}
	return false
}

// isInlineParent returns true if the tag is a valid inline parent for
// inline conditional content insertion.
func isInlineParent(tag string) bool {
	switch tag {
	case "div", "li", "td", "span", "a":
		return true
	}
	return false
}

// =============================================================================
// fixupIllustratedLayoutAnchors — yj_to_epub_illustrated_layout.py L29-128
// =============================================================================

// fixupIllustratedLayoutAnchors rewrites -kfx-amzn-condition inline styles
// from anchor: URIs to same-file fragment ids when applicable.
// Port of fixup_illustrated_layout_anchors (L29-128).
func fixupIllustratedLayoutAnchors(book *decodedBook, sections []renderedSection) {
	if book == nil || !book.IllustratedLayout {
		return
	}
	for i := range sections {
		if sections[i].Root == nil {
			continue
		}
		fixupIllustratedLayoutParts(sections[i].Root.Children, sections[i].Filename)
	}
}

func fixupIllustratedLayoutParts(parts []htmlPart, sectionFilename string) {
	for _, p := range parts {
		el, ok := p.(*htmlElement)
		if !ok || el == nil {
			continue
		}
		if el.Tag == "div" {
			if style, ok := el.Attrs["style"]; ok && strings.Contains(style, "-kfx-amzn-condition") {
				if next := rewriteAmznConditionStyle(style, sectionFilename); next != style {
					el.Attrs["style"] = next
				}
			}
		}
		fixupIllustratedLayoutParts(el.Children, sectionFilename)
	}
}

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
// createConditionalPageTemplates — yj_to_epub_illustrated_layout.py L130-419
// =============================================================================

// cssFileInfo holds a CSS file path and its content for layout CSS generation.
type cssFileInfo struct {
	Filename string
	Content  string
}

// createConditionalPageTemplates processes conditional page template divs in
// rendered sections. In inline mode (emitPageTemplates=false, matching Python
// default), it processes template children and removes non-float decorative
// elements. Returns a list of CSS files to create (empty in inline mode).
//
// Port of create_conditional_page_templates (L130-419).
func createConditionalPageTemplates(book *decodedBook, sections []renderedSection, hasConditionalContent bool) []cssFileInfo {
	if !hasConditionalContent {
		return nil
	}

	var cssFiles []cssFileInfo

	for secIdx := range sections {
		section := &sections[secIdx]
		if section.Root == nil {
			continue
		}
		body := section.Root

		// Iterate body's direct children. We use an index since we may modify the slice.
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

			// Pop known unused keys from template style
			popStyle(templateStyle, "-kfx-style-name")
			popStyle(templateStyle, "-kfx-attrib-epub-type")

			// Handle 100% height/width
			if templateStyle["height"] == "100%" && templateStyle["width"] == "100%" {
				delete(templateStyle, "height")
				delete(templateStyle, "width")
			}

			// Partition base_style from template_style:
			// base_style gets everything except -amzn-page-align and -kfx-collision
			baseStyle := make(map[string]string)
			for k, v := range templateStyle {
				switch k {
				case "-amzn-page-align", "-kfx-collision":
					// keep in templateStyle for now
				default:
					baseStyle[k] = v
					delete(templateStyle, k)
				}
			}

			// Check for extra styles that shouldn't be there
			extraProps := map[string]bool{
				"-amzn-page-header": true, "-amzn-page-footer": true,
				"background-color": true, "color": true,
			}
			for k := range templateStyle {
				if extraProps[k] {
					fmt.Fprintf(os.Stderr, "kfx: error: Conditional file=%s cond=%s has extra style: %s\n",
						section.Filename, amznCondition, k)
				}
			}

			// Process template children
			inlineContent := false

			for ci := 0; ci < len(templateElem.Children); ci++ {
				templateChild, ok := templateElem.Children[ci].(*htmlElement)
				if !ok || templateChild == nil {
					continue
				}
				idx := ci + 1 // 1-based index for error messages

				// Handle single-child div containers: unwrap to inner element
				// Python: if template_child.tag == "div" and len(template_child) > 0
				if templateChild.Tag == "div" && len(templateChild.Children) > 0 {
					childStyle := parseStyleString(templateChild.Attrs["style"])
					popStyle(childStyle, "-kfx-style-name")
					popStyle(childStyle, "position")
					popStyle(childStyle, "width")
					popStyle(childStyle, "height")
					pageAlign, _ := popStyle(childStyle, "-amzn-page-align")
					if pageAlign != "" {
						baseStyle["-amzn-page-align"] = pageAlign
					}

					// Check if this container has only one child element
					if len(templateChild.Children) == 1 && templateChild.Attrs["style"] != "" {
						// Unwrap: replace container with its inner child
						innerChild := templateChild.Children[0]
						if innerEl, ok := innerChild.(*htmlElement); ok {
							templateElem.Children[ci] = innerEl
							templateChild = innerEl
						}
					}
				}

				// Get child's own style
				origCondStyle := parseStyleString(templateChild.Attrs["style"])

				// Merge base style with child's own style (child overrides base)
				mergedStyle := make(map[string]string)
				for k, v := range baseStyle {
					mergedStyle[k] = v
				}
				for k, v := range origCondStyle {
					mergedStyle[k] = v
				}
				popStyle(mergedStyle, "-kfx-style-name")

				// Check if this is an img/video or div with background-color
				isImgOrVideo := templateChild.Tag == "img" || templateChild.Tag == "video"
				_, hasBgColor := mergedStyle["background-color"]
				isConditionalElement := isImgOrVideo || (templateChild.Tag == "div" && hasBgColor)

				if isConditionalElement {
					removeChild := false

					// Handle position=fixed → convert to float
					position, hadPosition := popStyle(mergedStyle, "position")
					if hadPosition && position == "fixed" {
						type posMapping struct {
							cssPos  string
							prop    string
						}
						mappings := []posMapping{
							{"left", "float"},
							{"right", "float"},
							{"top", "-amzn-float"},
							{"bottom", "-amzn-float"},
						}
						for _, m := range mappings {
							if val, ok := mergedStyle[m.cssPos]; ok {
								delete(mergedStyle, m.cssPos)
								if val != "0" {
									fmt.Fprintf(os.Stderr, "kfx: error: Conditional element has non-zero position: %s=%s\n",
										m.cssPos, val)
								}
								if existing, ok := mergedStyle[m.prop]; ok {
									if m.prop == "-amzn-float" {
										mergedStyle[m.prop] = existing + "," + m.cssPos
									} else {
										fmt.Fprintf(os.Stderr, "kfx: error: conflicting position styles\n")
									}
								} else {
									mergedStyle[m.prop] = m.cssPos
								}
							}
						}
					}

					// Handle -amzn-page-align=none
					if mergedStyle["-amzn-page-align"] == "none" {
						delete(mergedStyle, "-amzn-page-align")
					}

					_ = mergedStyle["float"] != "" || mergedStyle["-amzn-float"] != "" // isFloat
					_, _ = popStyle(mergedStyle, "-kfx-collision")
					epubTypesStr, _ := popStyle(mergedStyle, "-kfx-attrib-epub-type")
					epubTypes := map[string]bool{}
					for _, et := range strings.Fields(epubTypesStr) {
						if et != "" {
							epubTypes[et] = true
						}
					}

					if emitPageTemplates {
						// EMIT_PAGE_TEMPLATES mode — not used by default
						_ = inlineContent
						_ = idx
						// This branch is not exercised since emitPageTemplates is false
					} else {
						// Inline mode (EMIT_PAGE_TEMPLATES=false) — Python default
						if templateChild.Tag == "div" ||
							(templateChild.Tag == "img" && mergedStyle["-amzn-shape-outside"] == "") {
							removeChild = true
						} else {
							// Keep float shapes, remove decorative/illustrated types
							popStyle(mergedStyle, "-amzn-shape-outside")
							popStyle(mergedStyle, "-amzn-float")
							delete(epubTypes, "amzn:decorative")
							delete(epubTypes, "amzn:kindle-illustrated")
						}

						// Rebuild epub types string
						var types []string
						for k := range epubTypes {
							types = append(types, k)
						}
						if len(types) > 0 {
							// Sort for determinism
							for i := 0; i < len(types); i++ {
								for j := i + 1; j < len(types); j++ {
									if types[i] > types[j] {
										types[i], types[j] = types[j], types[i]
									}
								}
							}
							mergedStyle["-kfx-attrib-epub-type"] = strings.Join(types, " ")
						}

						if templateChild.Tag == "div" {
							mergedStyle["display"] = "inline"
						}

						if removeChild {
							// Remove this child from the template element
							templateElem.Children = append(templateElem.Children[:ci], templateElem.Children[ci+1:]...)
							ci-- // adjust index since we removed an element
						} else {
							// Set updated style
							if s := serializeStyleMap(mergedStyle); s != "" {
								templateChild.Attrs["style"] = s
							}
						}
					}

				} else if idx == len(templateElem.Children) &&
					templateChild.Tag == "div" {

					// Last child div with -amzn-page-align=none, position=fixed,
					// and no other styles → move its id to body start, remove element
					childStyleMap := parseStyleString(templateChild.Attrs["style"])
					pageAlign, _ := popStyle(childStyleMap, "-amzn-page-align")
					popStyle(childStyleMap, "position")
					collision, _ := popStyle(childStyleMap, "-kfx-collision")

					if pageAlign == "none" && len(childStyleMap) == 0 &&
						(collision == "" || collision == "always queue" || collision == "queue") {

						storyID := templateChild.Attrs["id"]
						if storyID != "" {
							// Create id-only div at body start
							idDiv := &htmlElement{
								Tag:   "div",
								Attrs: map[string]string{"id": storyID},
							}
							// Insert at beginning of body
							body.Children = append([]htmlPart{idDiv}, body.Children...)
							ti++ // adjust for the new element we just inserted
						}

						// Remove this child from template
						templateElem.Children = append(templateElem.Children[:ci], templateElem.Children[ci+1:]...)
						ci--
					}
				}
				_ = condOper
			}

			// Find target element by id
			target := findElementByID(body, targetID)
			if target == nil {
				// Remove template element from body
				body.Children = append(body.Children[:ti], body.Children[ti+1:]...)
				ti--
				continue
			}

			// Remove template element from body
			body.Children = append(body.Children[:ti], body.Children[ti+1:]...)
			ti-- // adjust for removed element

			// In inline mode (EMIT_PAGE_TEMPLATES=false):
			if !emitPageTemplates {
				// Walk up to block parent
				// For our simplified DOM, we insert as first child of target
				if len(templateElem.Children) > 0 {
					if !inlineContent {
						// Set template style if present
						if ts := serializeStyleMap(templateStyle); ts != "" {
							templateElem.Attrs["style"] = ts
						} else {
							delete(templateElem.Attrs, "style")
						}

						// Insert as first child of target's block parent
						insertTarget := target
						// Walk up to a block parent
						for insertTarget != nil && !isBlockParent(insertTarget.Tag) {
							// In our flat DOM, we just use target directly
							break
						}
						if insertTarget != nil {
							insertChild(insertTarget, 0, templateElem)
						}
					}
				}
			}
		}
	}

	return cssFiles
}
