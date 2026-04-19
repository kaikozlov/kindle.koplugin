package kfx

import (
	"fmt"
	"log"
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

// unusedSectionKeys lists keys that process_section pops from section data before processing
// (yj_to_epub_content.py L124-136).
var unusedSectionKeys = map[string]bool{
	"$702": true,
	"yj.conversion.html_name": true,
	"yj.semantics.book_anatomy_type": true,
	"yj.semantics.page_type": true,
	"yj.authoring.auto_panel_settings_auto_mask_color_flag": true,
	"yj.authoring.auto_panel_settings_mask_color": true,
	"yj.authoring.auto_panel_settings_opacity": true,
	"yj.authoring.auto_panel_settings_padding_bottom": true,
	"yj.authoring.auto_panel_settings_padding_left": true,
	"yj.authoring.auto_panel_settings_padding_right": true,
	"yj.authoring.auto_panel_settings_padding_top": true,
}

// isUnusedSectionKey returns true if the given key should be stripped from section data.
func isUnusedSectionKey(key string) bool {
	return unusedSectionKeys[key]
}

// stripUnusedSectionKeys removes known unused keys from section data, matching Python's
// process_section (yj_to_epub_content.py L124-136).
func stripUnusedSectionKeys(data map[string]interface{}) {
	for key := range data {
		if isUnusedSectionKey(key) {
			delete(data, key)
		}
	}
}

// bookType represents the detected book type for section processing dispatch.
// Port of Python self.is_comic, self.is_children, self.is_magazine, self.is_print_replica.
type bookType string

const (
	bookTypeNone         bookType = ""
	bookTypeComic        bookType = "comic"
	bookTypeChildren     bookType = "children"
	bookTypeMagazine     bookType = "magazine"
	bookTypePrintReplica bookType = "print_replica"
)

// sectionBranch represents which processing path a section takes.
type sectionBranch int

const (
	branchReflowable    sectionBranch = iota // default reflowable/standard processing
	branchScribePage                         // nmdl.canvas_width → process_scribe_notebook_page_section
	branchScribeTemplate                     // nmdl.template_type → process_scribe_notebook_template_section
	branchComic                              // comic/children → process_page_spread_page_template
	branchMagazine                           // magazine/print-replica with conditional templates
)

func sectionBranchString(b sectionBranch) string {
	switch b {
	case branchScribePage:
		return "scribe-page"
	case branchScribeTemplate:
		return "scribe-template"
	case branchComic:
		return "comic"
	case branchMagazine:
		return "magazine"
	default:
		return "reflowable"
	}
}

// detectBookType determines the book type from metadata ($258) and content features ($585).
// Port of Python's book_type detection in yj_to_epub_metadata.py.
func detectBookType(metadata map[string]interface{}, features map[string]interface{}) bookType {
	// Check content features for comic-type capabilities
	if features != nil {
		featureList, ok := asSlice(features["$590"])
		if ok {
			for _, feature := range featureList {
				featureMap, ok := asMap(feature)
				if !ok {
					continue
				}
				featureID, _ := asString(featureMap["$492"])
				category, _ := asString(featureMap["$586"])
				key := category + "/" + featureID
				switch key {
				case "kindle_capability_metadata/yj_facing_page", "kindle_capability_metadata/yj_double_page_spread", "kindle_capability_metadata/yj_publisher_panels":
					return bookTypeComic
				}
			}
		}
	}

	// Check metadata for CDE content type
	if metadata != nil {
		if cdeType, ok := asString(metadata["$251"]); ok {
			if cdeType == "MAGZ" {
				return bookTypeMagazine
			}
		}
	}

	return bookTypeNone
}

// sectionHasNmdlKey checks if the section's PageTemplateValues contains the given nmdl key.
// In Python, these are checked as keys of the section (IonStruct) directly.
func sectionHasNmdlKey(values map[string]interface{}, key string) bool {
	if values == nil {
		return false
	}
	_, exists := values[key]
	return exists
}

// hasConditionalTemplate returns true if any page template has a condition ($171).
func hasConditionalTemplate(templates []pageTemplateFragment) bool {
	for _, template := range templates {
		if template.Condition != nil || template.HasCondition {
			return true
		}
	}
	return false
}

// determineSectionBranch determines which processing branch to use for a section,
// matching Python's process_section dispatch logic (yj_to_epub_content.py L136-186).
func determineSectionBranch(section sectionFragment, bt bookType) sectionBranch {
	values := section.PageTemplateValues

	// Branch 1: Scribe notebook page (nmdl.canvas_width)
	if sectionHasNmdlKey(values, "nmdl.canvas_width") {
		return branchScribePage
	}

	// Branch 2: Scribe notebook template (nmdl.template_type)
	if sectionHasNmdlKey(values, "nmdl.template_type") {
		return branchScribeTemplate
	}

	// Branch 3: Comic/children
	if bt == bookTypeComic || bt == bookTypeChildren {
		return branchComic
	}

	// Branch 4: Magazine/print-replica with conditional template
	if (bt == bookTypeMagazine || bt == bookTypePrintReplica) && hasConditionalTemplate(section.PageTemplates) {
		return branchMagazine
	}

	// Branch 5: Default reflowable
	return branchReflowable
}

// filterActiveTemplates returns templates that are active based on condition evaluation.
// In fixed-layout mode, templates with false conditions are filtered out.
// Port of Python's conditional template filtering in process_section magazine branch.
func filterActiveTemplates(templates []pageTemplateFragment, evaluator conditionEvaluator) []pageTemplateFragment {
	if !evaluator.fixedLayout {
		return templates
	}
	active := make([]pageTemplateFragment, 0, len(templates))
	for _, template := range templates {
		if template.Condition == nil || evaluator.evaluateBinary(template.Condition) {
			active = append(active, template)
		}
	}
	return active
}

// Port of KFX_EPUB_Content.process_reading_order (yj_to_epub_content.py L105-113):
// emit one XHTML body per section ID, deduplicating sections across reading orders.
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
	// Port of Python's used_sections set for deduplication (L107).
	usedSections := map[string]bool{}

	for index, sectionID := range sectionOrder {
		if usedSections[sectionID] {
			// Python: log.error("Duplicate section %s found in reading order" % section_name)
			log.Printf("kfx: duplicate section %s found in reading order", sectionID)
			continue
		}
		usedSections[sectionID] = true

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

// Port of KFX_EPUB_Content.process_section (yj_to_epub_content.py L115-208).
// seq is the reading-order index (Python enumerate).
// The function dispatches to different processing paths based on book type and section content:
//  1. nmdl.canvas_width → scribe notebook page
//  2. nmdl.template_type → scribe notebook template
//  3. comic/children → process_page_spread_page_template
//  4. magazine/print-replica with conditional templates → conditional template selection
//  5. default → reflowable processing (renderSectionFragments)
func processSection(sectionID string, section sectionFragment, seq int, storylines map[string]map[string]interface{}, contentFragments map[string][]string, renderer *storylineRenderer) (renderedStoryline, []string, bool) {
	// Strip unused section keys (Python L124-136).
	// In Python, section is a mutable dict and keys are popped. In Go, section.PageTemplateValues
	// holds the equivalent data. We strip unused keys there.
	stripUnusedSectionKeys(section.PageTemplateValues)

	// Determine processing branch based on section content and book type.
	// Note: book type detection is not yet fully wired (depends on B5 metadata getters),
	// so currently this always falls through to the reflowable path.
	// When book type is available, it will be passed through the renderer or section context.
	// For now, the reflowable path handles all existing behavior unchanged.
	_ = seq

	return renderSectionFragments(sectionID, section, storylines, contentFragments, renderer)
}

// renderSectionFragments is the reflowable / fixed-layout subset invoked from process_section.
// Port of Python's default (else) branch in process_section (yj_to_epub_content.py L186-203).
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
		active := filterActiveTemplates(templates, renderer.conditionEvaluator)
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

// processSectionScribePage dispatches to the scribe notebook page section handler.
// Port of Python's nmdl.canvas_width branch in process_section (L136-137).
func processSectionScribePage(section sectionFragment, seq int) (renderedStoryline, []string, bool) {
	templates := section.PageTemplates
	if len(templates) == 0 {
		return renderedStoryline{}, nil, false
	}
	template := templates[0]
	result := processScribeNotebookPageSection(section.PageTemplateValues, template.PageTemplateValues, section.ID, seq)
	if !result {
		return renderedStoryline{}, nil, false
	}
	return renderedStoryline{}, nil, false // scribe sections don't produce standard rendered output yet
}

// processSectionScribeTemplate dispatches to the scribe notebook template section handler.
// Port of Python's nmdl.template_type branch in process_section (L139-140).
func processSectionScribeTemplate(section sectionFragment) (renderedStoryline, []string, bool) {
	templates := section.PageTemplates
	if len(templates) == 0 {
		return renderedStoryline{}, nil, false
	}
	template := templates[0]
	result := processScribeNotebookTemplateSection(section.PageTemplateValues, template.PageTemplateValues, section.ID)
	if !result {
		return renderedStoryline{}, nil, false
	}
	return renderedStoryline{}, nil, false // scribe sections don't produce standard rendered output yet
}

// processSectionComic handles the comic/children book type dispatch.
// Port of Python's comic/children branch in process_section (L142-149).
// Validates exactly 1 page template, then calls process_page_spread_page_template.
// Note: full page_spread_page_template implementation is feature A4; this function
// provides the dispatch skeleton that A4 will complete.
func processSectionComic(section sectionFragment) (renderedStoryline, []string, bool) {
	templates := section.PageTemplates
	if len(templates) != 1 {
		log.Printf("kfx: error: comic section %s has %d page templates", section.ID, len(templates))
		if len(templates) == 0 {
			return renderedStoryline{}, nil, false
		}
	}
	// In Python, this calls self.process_page_spread_page_template with the $608 fragment.
	// Full implementation deferred to feature A4 (process_page_spread_page_template port).
	// For now, log that we recognized the comic branch.
	if os.Getenv("KFX_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "comic section=%s templates=%d (process_page_spread_page_template not yet ported)\n", section.ID, len(templates))
	}
	return renderedStoryline{}, nil, false
}

// processSectionMagazine handles the magazine/print-replica with conditional template dispatch.
// Port of Python's magazine branch in process_section (L150-184).
// Iterates page templates, skipping those whose condition evaluates to false,
// then processes the active template based on its layout type.
func processSectionMagazine(section sectionFragment, renderer *storylineRenderer) (renderedStoryline, []string, bool) {
	templates := section.PageTemplates
	templatesProcessed := 0

	for i, template := range templates {
		// Skip templates with false conditions
		if template.Condition != nil && !renderer.conditionEvaluator.evaluateBinary(template.Condition) {
			continue
		}

		// Python checks page_template["$159"] != "$270" → error
		// and page_template["$156"] for layout dispatch.
		// Full implementation deferred to feature A4 (process_page_spread_page_template port).
		_ = i
		templatesProcessed++
	}

	if templatesProcessed != 1 {
		log.Printf("kfx: error: section %s has %d active conditional page templates", section.ID, templatesProcessed)
	}

	return renderedStoryline{}, nil, false
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
	return hasConditionalTemplate(templates)
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
