package kfx

import (
	"fmt"
	"os"
	"strings"
)

// Port of LIST_STYLE_TYPES / list marker → HTML list tag (yj_to_epub_content.py top; used from storyline emit).
var listTagByMarker = map[string]string{
	"$346": "ol",
	"$347": "ol",
	"$342": "ul",
	"$340": "ul",
	"$271": "ul",
	"$349": "ul",
	"$343": "ol",
	"$344": "ol",
	"$345": "ol",
	"$341": "ul",
}

// Port of CLASSIFICATION_EPUB_TYPE (yj_to_epub_content.py) — semantic EPUB type for aside/div from YJ classification.
var classificationEPUBType = map[string]string{
	"$618": "footnote",
	"$619": "endnote",
	"$281": "footnote",
}

// Port of layout-hint / element name hints from yj_to_epub_content.py (subset used by emit path).
var layoutHintElementNames = map[string]string{
	"$453": "caption",
	"$282": "figure",
	"$760": "heading",
}

// Port of KFX_EPUB_Content.process_reading_order (yj_to_epub_content.py L105+): emit one XHTML body per section ID.
func processReadingOrder(
	book *decodedBook,
	sectionOrder []string,
	sectionFragments map[string]sectionFragment,
	storylines map[string]map[string]interface{},
	contentFragments map[string][]string,
	renderer *storylineRenderer,
	navTitles map[string]string,
	symFmt symType,
) {
	for index, sectionID := range sectionOrder {
		section, ok := sectionFragments[sectionID]
		if !ok {
			continue
		}
		rendered, paragraphs, ok := processSection(sectionID, section, index, storylines, contentFragments, renderer)
		if !ok {
			continue
		}
		if debugSection := os.Getenv("KFX_DEBUG_SECTION_CLASS"); debugSection != "" {
			for _, wanted := range strings.Split(debugSection, ",") {
				if strings.TrimSpace(wanted) == sectionID {
					fmt.Fprintf(os.Stderr, "section=%s bodyClass=%q properties=%q\n", sectionID, rendered.BodyClass, rendered.Properties)
				}
			}
		}
		if len(paragraphs) == 0 && rendered.BodyHTML == "" {
			continue
		}
		title := navTitles[sectionID]
		if title == "" {
			title = deriveSectionTitle(paragraphs, index+1)
		}
		book.RenderedSections = append(book.RenderedSections, renderedSection{
			Filename:   sectionFilename(sectionID, symFmt),
			Title:      title,
			PageTitle:  sectionID,
			Language:   normalizeLanguage(book.Language),
			BodyClass:  rendered.BodyClass,
			BodyStyle:  rendered.BodyStyle,
			Paragraphs: paragraphs,
			Properties: rendered.Properties,
			Root:       rendered.Root,
		})
	}
}

// Port of KFX_EPUB_Content.process_section (yj_to_epub_content.py L115+). seq is the reading-order index (Python enumerate).
func processSection(sectionID string, section sectionFragment, seq int, storylines map[string]map[string]interface{}, contentFragments map[string][]string, renderer *storylineRenderer) (renderedStoryline, []string, bool) {
	_ = seq
	return renderSectionFragments(sectionID, section, storylines, contentFragments, renderer)
}

// renderSectionFragments is the reflowable / fixed-layout subset invoked from process_section.
func renderSectionFragments(sectionID string, section sectionFragment, storylines map[string]map[string]interface{}, contentFragments map[string][]string, renderer *storylineRenderer) (renderedStoryline, []string, bool) {
	templates := section.PageTemplates
	if len(templates) == 0 {
		templates = []pageTemplateFragment{{
			PositionID:         section.PositionID,
			Storyline:          section.Storyline,
			PageTemplateStyle:  section.PageTemplateStyle,
			PageTemplateValues: section.PageTemplateValues,
		}}
	}
	if renderer != nil && renderer.conditionEvaluator.fixedLayout && pageTemplatesHaveConditions(templates) {
		active := make([]pageTemplateFragment, 0, len(templates))
		for _, template := range templates {
			if template.Condition == nil || renderer.conditionEvaluator.evaluateBinary(template.Condition) {
				active = append(active, template)
			}
		}
		if len(active) == 0 {
			return renderedStoryline{}, nil, false
		}
		templates = active
	}

	mainIndex := len(templates) - 1
	mainTemplate := templates[mainIndex]
	storyline := storylines[mainTemplate.Storyline]
	if storyline == nil {
		return renderedStoryline{}, nil, false
	}
	nodes, _ := asSlice(storyline["$146"])
	paragraphs := flattenParagraphs(nodes, contentFragments)
	debugStorylineNodes(sectionID, nodes, 0)
	if os.Getenv("KFX_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "render section=%s pageStyle=%s storyStyle=%s\n", sectionID, mainTemplate.PageTemplateStyle, asStringDefault(storyline["$157"]))
	}
	rendered := renderer.renderStoryline(mainTemplate.PositionID, mainTemplate.PageTemplateStyle, mainTemplate.PageTemplateValues, storyline, nodes)

	for _, template := range templates[:mainIndex] {
		overlayStoryline := storylines[template.Storyline]
		if overlayStoryline == nil {
			continue
		}
		overlayNodes, _ := asSlice(overlayStoryline["$146"])
		overlayParagraphs := flattenParagraphs(overlayNodes, contentFragments)
		paragraphs = append(paragraphs, overlayParagraphs...)
		debugStorylineNodes(sectionID, overlayNodes, 0)
		if os.Getenv("KFX_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "render overlay section=%s pageStyle=%s storyStyle=%s conditional=%v\n", sectionID, template.PageTemplateStyle, asStringDefault(overlayStoryline["$157"]), template.HasCondition)
		}
		overlayRendered := renderer.renderStoryline(template.PositionID, template.PageTemplateStyle, template.PageTemplateValues, overlayStoryline, overlayNodes)
		if rendered.BodyClass == "" {
			rendered.BodyClass = overlayRendered.BodyClass
		}
		if overlayRendered.Root != nil {
			if rendered.Root == nil {
				rendered.Root = &htmlElement{Attrs: map[string]string{}}
			}
			rendered.Root.Children = append(rendered.Root.Children, overlayRendered.Root.Children...)
			rendered.BodyHTML = renderHTMLParts(rendered.Root.Children, true)
		}
		rendered.Properties = mergeSectionProperties(rendered.Properties, overlayRendered.Properties)
	}

	return rendered, paragraphs, len(paragraphs) > 0 || rendered.BodyHTML != ""
}

// Port of KFX_EPUB_Content.prepare_book_parts (yj_to_epub_content.py ~L1782+).
// Python runs replace_eol_with_br / reset_preformat / preformat_spaces on each book_part body;
// Go reapplies whitespace + EOL normalization on the htmlElement tree before XHTML materialization.
func prepareBookParts(book *decodedBook) {
	if book == nil {
		return
	}
	for i := range book.RenderedSections {
		if book.RenderedSections[i].Root != nil {
			normalizeHTMLWhitespace(book.RenderedSections[i].Root)
		}
	}
}

func pageTemplatesHaveConditions(templates []pageTemplateFragment) bool {
	for _, template := range templates {
		if template.Condition != nil || template.HasCondition {
			return true
		}
	}
	return false
}
func mergeSectionProperties(left string, right string) string {
	seen := map[string]bool{}
	merged := make([]string, 0, 2)
	for _, raw := range strings.Fields(strings.TrimSpace(left + " " + right)) {
		if raw == "" || seen[raw] {
			continue
		}
		seen[raw] = true
		merged = append(merged, raw)
	}
	return strings.Join(merged, " ")
}
