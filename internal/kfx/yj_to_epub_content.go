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

// validateOverlayCondition validates the $171 condition for overlay templates.
// Port of Python yj_to_epub_content.py:491-504.
// In Python, when a $171 condition is found in content with a layout, the layout must be "$324"
// and the condition must match a specific structure: IonSExp of length 3, where
// fv[0] in CONDITION_OPERATOR_NAMES ($294/$299/$298), fv[1]=="$183",
// fv[2] is IonSExp of length 2 with fv[2][0]=="$266".
// Returns true if the condition is valid, false and logs an error if not.
func validateOverlayCondition(condition interface{}, layout string) bool {
	if condition == nil {
		return true
	}
	// Python L492: if layout != "$324": log.error("Conditional page template has unexpected layout")
	if layout != "$324" {
		log.Printf("kfx: error: Conditional page template has unexpected layout: %s", layout)
	}
	// Python L500-504: validate condition structure
	// ion_type(condition) is IonSExp and len(condition) == 3 and condition[1] == "$183" and
	// ion_type(condition[2]) is IonSExp and len(condition[2]) == 2 and condition[2][0] == "$266" and
	// condition[0] in self.CONDITION_OPERATOR_NAMES
	slice, ok := asSlice(condition)
	if !ok || len(slice) != 3 {
		log.Printf("kfx: error: Condition is not in expected format: %v", condition)
		return false
	}
	fv0, _ := asString(slice[0])
	fv1, _ := asString(slice[1])
	if fv1 != "$183" {
		log.Printf("kfx: error: Condition is not in expected format: %v", condition)
		return false
	}
	fv2, ok := asSlice(slice[2])
	if !ok || len(fv2) != 2 {
		log.Printf("kfx: error: Condition is not in expected format: %v", condition)
		return false
	}
	fv20, _ := asString(fv2[0])
	if fv20 != "$266" {
		log.Printf("kfx: error: Condition is not in expected format: %v", condition)
		return false
	}
	if _, valid := conditionOperatorNames[fv0]; !valid {
		log.Printf("kfx: error: Condition is not in expected format: %v", condition)
		return false
	}
	return true
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
	_ = seq

	return renderSectionFragments(sectionID, section, storylines, contentFragments, renderer)
}

// processSectionWithType dispatches to the correct section processing path based on book type.
// Port of Python's process_section dispatch logic (yj_to_epub_content.py L136-203).
// bt is the detected book type; cfg provides page-spread configuration; storylines maps
// storyline names to their data (needed for page-spread processing).
func processSectionWithType(sectionID string, section sectionFragment, seq int, bt bookType, cfg pageSpreadConfig, storylines map[string]map[string]interface{}, contentFragments map[string][]string, renderer *storylineRenderer) (renderedStoryline, []string, bool) {
	// Strip unused section keys (Python L124-136).
	stripUnusedSectionKeys(section.PageTemplateValues)

	// Determine which branch to take
	branch := determineSectionBranch(section, bt)

	switch branch {
	case branchScribePage:
		return processSectionScribePage(section, seq)

	case branchScribeTemplate:
		return processSectionScribeTemplate(section)

	case branchComic:
		// Python L142-154: comic/children → resolve $608, call processPageSpreadPageTemplate
		result := processSectionComic(section, cfg, storylines)
		if result.Err != nil {
			log.Printf("kfx: error: comic section %s failed: %v", sectionID, result.Err)
			return renderedStoryline{}, nil, false
		}
		// Comic branch produces page-spread results, not standard rendered content.
		// For now, return empty (page-spread sections are handled separately).
		return renderedStoryline{}, nil, false

	case branchMagazine:
		// Python L150-184: magazine/print-replica with conditional templates
		result := processSectionMagazine(section, renderer, cfg, storylines)
		if result.Err != nil {
			log.Printf("kfx: error: magazine section %s failed: %v", sectionID, result.Err)
			return renderedStoryline{}, nil, false
		}
		// Magazine branch produces page-spread results for $437 layouts
		// and inline sections for $325/$323 layouts.
		return renderedStoryline{}, nil, false

	default:
		// Default reflowable path
		return renderSectionFragments(sectionID, section, storylines, contentFragments, renderer)
	}
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
// Port of Python's comic/children branch in process_section (yj_to_epub_content.py:142-149).
// Validates exactly 1 page template, resolves the $608 fragment, then calls
// process_page_spread_page_template.
func processSectionComic(section sectionFragment, cfg pageSpreadConfig, storylines map[string]map[string]interface{}) pageSpreadResult {
	templates := section.PageTemplates
	if len(templates) != 1 {
		log.Printf("kfx: error: Comic section %s has %d page templates", section.ID, len(templates))
		if len(templates) == 0 {
			return pageSpreadResult{}
		}
	}

	// Python L154: self.process_page_spread_page_template(
	//     self.get_fragment(ftype="$608", fid=page_templates[0]), section_name)
	// In Go, the page template's PageTemplateValues already holds the resolved $608 data
	// (organized during organizeFragments). Pass it directly to processPageSpreadPageTemplate.
	template := templates[0]
	templateData := template.PageTemplateValues
	if templateData == nil {
		templateData = map[string]interface{}{}
	}

	return processPageSpreadPageTemplate(
		templateData,
		section.ID,
		"",  // page_spread (empty for top-level)
		nil, // parent_template_id
		true, // is_section
		cfg,
		storylines,
	)
}

// processSectionMagazine handles the magazine/print-replica with conditional template dispatch.
// Port of Python's magazine branch in process_section (yj_to_epub_content.py:150-184).
// Iterates page templates, skipping those whose condition evaluates to false,
// then processes the active template based on its layout type ($325/$323 → inline,
// $437 → processPageSpreadPageTemplate, other → error).
func processSectionMagazine(section sectionFragment, renderer *storylineRenderer, cfg pageSpreadConfig, storylines map[string]map[string]interface{}) pageSpreadResult {
	templates := section.PageTemplates
	templatesProcessed := 0
	var result pageSpreadResult

	for i, template := range templates {
		// Python L162: if "$171" not in page_template or self.evaluate_binary_condition(page_template.pop("$171")):
		condition := template.Condition
		if condition != nil && !renderer.conditionEvaluator.evaluateBinary(condition) {
			continue
		}

		// Get template data. In Python, page_template is the actual template struct.
		// In Go, the template data is in PageTemplateValues (already resolved from $608).
		templateData := template.PageTemplateValues
		if templateData == nil {
			templateData = map[string]interface{}{}
		}

		// Make a working copy to pop from (matching Python's mutation of the template)
		working := cloneMap(templateData)

		// Python L163: if page_template["$159"] != "$270":
		ptype, _ := asString(working["$159"])
		if ptype != "$270" {
			log.Printf("kfx: error: section %s unexpected page_template type %s", section.ID, ptype)
		}

		// Python L167: layout = page_template["$156"]
		layout, _ := asString(working["$156"])

		if layout == "$325" || layout == "$323" {
			// Python L169-178: inline content processing
			// page_template.pop("$159"); page_template.pop("$156")
			delete(working, "$159")
			delete(working, "$156")

			// Python creates a book_part, adds content, links CSS, processes position.
			// In Go, we record this as a leaf section in the result.
			locID := getLocationID(working)
			section := pageSpreadSection{
				PageTitle:    section.ID,
				Properties:   "",
				PositionOffset: 0,
				TemplateData: working,
			}
			if locID != 0 {
				section.ParentPositionID = locID
			}
			result.Sections = append(result.Sections, section)

		} else if layout == "$437" {
			// Python L180: self.process_page_spread_page_template(page_template, section_name)
			spreadResult := processPageSpreadPageTemplate(working, section.ID, "", nil, true, cfg, storylines)
			if spreadResult.Err != nil {
				result.Err = spreadResult.Err
				return result
			}
			result.Children = append(result.Children, spreadResult.Children...)
			result.Sections = append(result.Sections, spreadResult.Sections...)
			if spreadResult.VirtualPanels {
				result.VirtualPanels = true
			}

		} else {
			// Python L182: log.error("... unexpected page_template layout %s" % layout)
			log.Printf("kfx: error: section %s unexpected page_template layout %s", section.ID, layout)
		}

		templatesProcessed++
		_ = i
	}

	// Python L184-185: if templates_processed != 1: log.error(...)
	if templatesProcessed != 1 {
		log.Printf("kfx: error: section %s has %d active conditional page templates", section.ID, templatesProcessed)
	}

	return result
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

// =============================================================================
// process_page_spread_page_template — Port of yj_to_epub_content.py:210-344
// =============================================================================

// pageSpreadBranch identifies which sub-branch of process_page_spread_page_template applies.
type pageSpreadBranch int

const (
	pageSpreadBranchSpread    pageSpreadBranch = iota // $437 page-spread / $438 facing-page
	pageSpreadBranchFacing                             // $438 facing-page (grouped with spread for alternation)
	pageSpreadBranchScaleFit                           // $326 PDF-backed scale_fit
	pageSpreadBranchConnected                          // $323/$656 connected pagination
	pageSpreadBranchLeaf                               // default leaf content page
)

// pageSpreadConfig holds book-level configuration needed for page spread processing.
// Port of Python's self.is_comic, self.is_pdf_backed, self.region_magnification, etc.
type pageSpreadConfig struct {
	BookType                 bookType
	IsPdfBacked              bool
	RegionMagnification      bool
	VirtualPanelsAllowed     bool
	VirtualPanels            bool // set to true when $434==$441 and VirtualPanelsAllowed
	PageProgressionDirection string // "ltr" or "rtl"
}

// pageSpreadChild represents a recursively processed child template from the spread/story.
type pageSpreadChild struct {
	PageSpread string                 // e.g. "page-spread-left", "page-spread-right", "rendition:page-spread-center"
	Data       map[string]interface{} // the child template data (consumed)
}

// pageSpreadSection represents a leaf content section produced by the leaf branch.
type pageSpreadSection struct {
	PageTitle         string // unique section name (with spread suffix if applicable)
	Properties        string // OPF properties (page spread direction)
	ParentPositionID  int    // set when parent_template_id is provided
	PositionOffset    int    // offset for position processing (always 0 per Python)
	TemplateData      map[string]interface{} // remaining template data
	HasCSSLink        bool   // true when CSS file should be linked (STYLES_CSS_FILEPATH)
	ContentProcessed  bool   // true when process_content was called (leaf XHTML content generated)
	HasPositionMarker bool   // true when parent_template_id was non-nil (inserts position marker)
}

// pageSpreadResult holds the output of processPageSpreadPageTemplate.
type pageSpreadResult struct {
	Children      []pageSpreadChild   // child templates processed from story $146
	Sections      []pageSpreadSection // leaf sections created
	VirtualPanels bool                // whether virtual panels were activated
	Err           error               // non-nil on fatal errors (e.g. missing storyline)
}

// determinePageSpreadBranch determines which sub-branch of process_page_spread_page_template
// applies based on the template's $159 (type) and $156 (layout) keys.
// Port of Python's conditional branching at yj_to_epub_content.py:213-338.
func determinePageSpreadBranch(pageTemplate map[string]interface{}, isSection bool) pageSpreadBranch {
	ptype, _ := asString(pageTemplate["$159"])
	layout, _ := asString(pageTemplate["$156"])

	// Branch A: page-spread ($437) / facing-page ($438)
	if ptype == "$270" && (layout == "$437" || layout == "$438") {
		if layout == "$438" {
			return pageSpreadBranchFacing
		}
		return pageSpreadBranchSpread
	}

	// Branch B: PDF-backed scale_fit ($326) — only for section-level templates
	if ptype == "$270" && layout == "$326" && isSection {
		return pageSpreadBranchScaleFit
	}

	// Branch C: connected pagination ($323/$656)
	if ptype == "$270" && layout == "$323" {
		if hasBool, _ := asBool(pageTemplate["$656"]); hasBool {
			return pageSpreadBranchConnected
		}
	}

	// Branch D: leaf content (default)
	return pageSpreadBranchLeaf
}

// getLocationID extracts the location ID from a template by popping $155, then $598.
// Port of Python's get_location_id (yj_to_epub_navigation.py:372-373).
func getLocationID(data map[string]interface{}) int {
	if id, ok := popInt(data, "$155"); ok {
		return id
	}
	if id, ok := popInt(data, "$598"); ok {
		return id
	}
	return 0
}

// popInt removes a key from a map and returns its int value.
func popInt(data map[string]interface{}, key string) (int, bool) {
	val, exists := data[key]
	if !exists {
		return 0, false
	}
	delete(data, key)
	return asIntDefault(val, 0), true
}

// popString removes a key from a map and returns its string value.
func popString(data map[string]interface{}, key string) (string, bool) {
	val, exists := data[key]
	if !exists {
		return "", false
	}
	delete(data, key)
	s, _ := asString(val)
	return s, true
}

// popInterface removes a key from a map and returns its value.
func popInterface(data map[string]interface{}, key string) (interface{}, bool) {
	val, exists := data[key]
	if !exists {
		return nil, false
	}
	delete(data, key)
	return val, true
}

// extractSpreadType extracts the spread type from a page_spread property string.
// E.g. "rendition:page-spread-left" → "left", "rendition:page-spread-right" → "right"
// Port of Python's spread_type = page_spread.replace("rendition:", "").replace("page-spread-", "")
func extractSpreadType(pageSpread string) string {
	s := strings.ReplaceAll(pageSpread, "rendition:", "")
	s = strings.ReplaceAll(s, "page-spread-", "")
	return s
}

// uniqueSectionName constructs a unique section name from the base name and spread type.
// Port of Python's unique_section_name = "%s-%s" % (section_name, spread_type) if spread_type else section_name
func uniqueSectionName(sectionName, spreadType string) string {
	if spreadType == "" {
		return sectionName
	}
	return sectionName + "-" + spreadType
}

// layoutSpreadBaseProperty maps layout symbols to their CSS base property name.
// Port of Python's LAYOUTS dict at yj_to_epub_content.py:246-249.
func layoutSpreadBaseProperty(layout string) string {
	switch layout {
	case "$437":
		return "page-spread"
	case "$438":
		return "facing-page"
	default:
		return ""
	}
}

// processVirtualPanel handles the $434 (virtual_panel) key from a page template.
// Port of Python's virtual_panel handling at yj_to_epub_content.py:219-226, 265-272, 303-310.
// Returns true if virtual_panels should be activated.
func processVirtualPanel(data map[string]interface{}, cfg *pageSpreadConfig, sectionName string) bool {
	virtualPanel := popInterfaceDefault(data, "$434")
	if virtualPanel == nil {
		if cfg.BookType == bookTypeComic && !cfg.RegionMagnification {
			log.Printf("kfx: error: section %s has missing virtual panel in comic without region magnification", sectionName)
		}
		return false
	}
	vpStr, _ := asString(virtualPanel)
	if vpStr == "$441" && cfg.VirtualPanelsAllowed {
		return true
	}
	if vpStr != "" {
		log.Printf("kfx: warning: unexpected %s page_template virtual_panel: %s", cfg.BookType, virtualPanel)
	}
	return false
}

// popInterfaceDefault removes a key and returns its value, or nil if absent.
func popInterfaceDefault(data map[string]interface{}, key string) interface{} {
	val, exists := data[key]
	if !exists {
		return nil
	}
	delete(data, key)
	return val
}

// processPageSpreadPageTemplate processes a page template within a page spread.
// Port of Python's process_page_spread_page_template (yj_to_epub_content.py:210-344).
//
// Parameters:
//   - pageTemplate: the template data (map with $159, $156, etc.)
//   - sectionName: the section name
//   - pageSpread: the current page spread direction (e.g. "rendition:page-spread-left")
//   - parentTemplateID: position ID of the parent template (nil if none)
//   - isSection: whether this is a top-level section template
//   - cfg: book-level page spread configuration
//   - storylines: map of storyline name → storyline data
func processPageSpreadPageTemplate(
	pageTemplate map[string]interface{},
	sectionName string,
	pageSpread string,
	parentTemplateID *int,
	isSection bool,
	cfg pageSpreadConfig,
	storylines map[string]map[string]interface{},
) pageSpreadResult {
	result := pageSpreadResult{}

	// Python L211-212: if ion_type(page_template) is IonSymbol → resolve to $608 fragment
	// In Go, this resolution already happened during organizeFragments, so pageTemplate
	// is already the resolved map. If it's a string (symbol reference), look it up.
	if ptStr, ok := asString(pageTemplate["$159"]); !ok || ptStr == "" {
		// Template might be a symbol reference — in Go's pipeline this is already resolved
		// via pageTemplateFragment. For standalone calls, skip.
		if len(pageTemplate) == 0 {
			// Empty template → leaf branch
			return processPageSpreadLeaf(pageTemplate, sectionName, pageSpread, parentTemplateID, isSection, cfg)
		}
	}

	branch := determinePageSpreadBranch(pageTemplate, isSection)

	// Python condition for scale_fit branch:
	//   self.is_pdf_backed and "$67" not in page_template and "$66" not in page_template
	// If not PDF-backed, OR if $67/$66 are present in pageTemplate, fall to leaf branch.
	if branch == pageSpreadBranchScaleFit {
		if !cfg.IsPdfBacked {
			branch = pageSpreadBranchLeaf
		} else if _, has67 := pageTemplate["$67"]; has67 {
			branch = pageSpreadBranchLeaf
		} else if _, has66 := pageTemplate["$66"]; has66 {
			branch = pageSpreadBranchLeaf
		}
	}

	switch branch {
	case pageSpreadBranchSpread, pageSpreadBranchFacing:
		result = processPageSpreadStoryBranch(pageTemplate, sectionName, parentTemplateID, isSection, cfg, storylines)

	case pageSpreadBranchScaleFit:
		result = processPageSpreadScaleFitBranch(pageTemplate, sectionName, parentTemplateID, isSection, cfg, storylines)

	case pageSpreadBranchConnected:
		result = processPageSpreadConnectedBranch(pageTemplate, sectionName, parentTemplateID, isSection, cfg, storylines)

	default:
		result = processPageSpreadLeaf(pageTemplate, sectionName, pageSpread, parentTemplateID, isSection, cfg)
	}

	return result
}

// processPageSpreadStoryBranch handles Branch A: page-spread ($437) / facing-page ($438).
// Port of yj_to_epub_content.py:213-258.
func processPageSpreadStoryBranch(
	pageTemplate map[string]interface{},
	sectionName string,
	parentTemplateID *int,
	isSection bool,
	cfg pageSpreadConfig,
	storylines map[string]map[string]interface{},
) pageSpreadResult {
	result := pageSpreadResult{}

	// Pop $159 and $156
	delete(pageTemplate, "$159")
	layout, _ := popString(pageTemplate, "$156")

	// Handle virtual panel
	if vp := processVirtualPanel(pageTemplate, &cfg, sectionName); vp {
		result.VirtualPanels = true
	}

	// Pop unused keys
	delete(pageTemplate, "$192")
	delete(pageTemplate, "$67")
	delete(pageTemplate, "$66")
	delete(pageTemplate, "$140")
	delete(pageTemplate, "$560")

	// Get location ID for parent (passed as parentTemplateID to first child only).
	// Port of Python: parent_template_id = self.get_location_id(page_template)
	// then parent_template_id = None after first child.
	locID := getLocationID(pageTemplate)

	// Get story from storyline reference
	storyName, _ := popString(pageTemplate, "$176")
	story, ok := storylines[storyName]
	if !ok {
		result.Err = fmt.Errorf("page spread template references missing storyline %q", storyName)
		return result
	}

	// Pop storyline name from story data
	delete(story, "$176")

	// Determine base property and initial page property
	baseProperty := layoutSpreadBaseProperty(layout)
	leftProperty := baseProperty + "-left"
	rightProperty := baseProperty + "-right"

	var pageProperty string
	if cfg.PageProgressionDirection == "ltr" {
		pageProperty = leftProperty
	} else {
		pageProperty = rightProperty
	}

	// Process children from $146 (content_list)
	children, _ := asSlice(popInterfaceDefault(story, "$146"))
	result.Children = make([]pageSpreadChild, 0, len(children))

	// Port of Python: parent_template_id passed to first child, then set to None.
	childParentID := &locID
	if locID == 0 {
		childParentID = nil
	}

	for _, child := range children {
		childData, ok := asMap(child)
		if !ok {
			continue
		}

		// Recursively process each child template
		childResult := processPageSpreadPageTemplate(
			childData, sectionName, pageProperty, childParentID, false, cfg, storylines,
		)
		if childResult.Err != nil {
			result.Err = childResult.Err
			return result
		}

		// Merge child results
		if childResult.VirtualPanels {
			result.VirtualPanels = true
		}
		result.Children = append(result.Children, pageSpreadChild{
			PageSpread: pageProperty,
			Data:       childData,
		})
		result.Sections = append(result.Sections, childResult.Sections...)
		result.Children = append(result.Children, childResult.Children...)

		// After first child, parentTemplateID is set to None (Python: parent_template_id = None)
		childParentID = nil

		// Alternate left/right
		if pageProperty == rightProperty {
			pageProperty = leftProperty
		} else {
			pageProperty = rightProperty
		}
	}

	return result
}

// processPageSpreadScaleFitBranch handles Branch B: PDF-backed scale_fit ($326).
// Port of yj_to_epub_content.py:260-297.
func processPageSpreadScaleFitBranch(
	pageTemplate map[string]interface{},
	sectionName string,
	parentTemplateID *int,
	isSection bool,
	cfg pageSpreadConfig,
	storylines map[string]map[string]interface{},
) pageSpreadResult {
	result := pageSpreadResult{}

	// Pop $159 and $156
	delete(pageTemplate, "$159")
	delete(pageTemplate, "$156")

	// Handle virtual panel
	if vp := processVirtualPanel(pageTemplate, &cfg, sectionName); vp {
		result.VirtualPanels = true
	}

	// Pop unused keys
	delete(pageTemplate, "$192")
	delete(pageTemplate, "$140")
	delete(pageTemplate, "$560")

	// Validate font_size == 16
	fontSize, _ := popInt(pageTemplate, "$16")
	if fontSize != 16 {
		log.Printf("kfx: warning: unexpected font size in PDF backed scale_fit page template: %d", fontSize)
	}

	// Get location ID — used as parentTemplateID for ALL children.
	// Port of Python: parent_template_id = self.get_location_id(page_template)
	// Note: unlike story/connected branches, scale_fit passes parent_template_id
	// to ALL children (Python does NOT set parent_template_id = None after first child).
	locID := getLocationID(pageTemplate)
	var childParentID *int
	if locID != 0 {
		childParentID = &locID
	}

	// Get story
	storyName, _ := popString(pageTemplate, "$176")
	story, ok := storylines[storyName]
	if !ok {
		result.Err = fmt.Errorf("scale_fit template references missing storyline %q", storyName)
		return result
	}
	delete(story, "$176")

	// Process children without page_spread alternation
	children, _ := asSlice(popInterfaceDefault(story, "$146"))
	result.Children = make([]pageSpreadChild, 0, len(children))

	for _, child := range children {
		childData, ok := asMap(child)
		if !ok {
			continue
		}

		childResult := processPageSpreadPageTemplate(
			childData, sectionName, "", childParentID, false, cfg, storylines,
		)
		if childResult.Err != nil {
			result.Err = childResult.Err
			return result
		}

		if childResult.VirtualPanels {
			result.VirtualPanels = true
		}
		result.Children = append(result.Children, pageSpreadChild{
			PageSpread: "",
			Data:       childData,
		})
		result.Sections = append(result.Sections, childResult.Sections...)
		result.Children = append(result.Children, childResult.Children...)
	}

	return result
}

// processPageSpreadConnectedBranch handles Branch C: connected pagination ($323/$656).
// Port of yj_to_epub_content.py:299-335.
func processPageSpreadConnectedBranch(
	pageTemplate map[string]interface{},
	sectionName string,
	parentTemplateID *int,
	isSection bool,
	cfg pageSpreadConfig,
	storylines map[string]map[string]interface{},
) pageSpreadResult {
	result := pageSpreadResult{}

	// Pop $159, $156, $656
	delete(pageTemplate, "$159")
	delete(pageTemplate, "$156")
	delete(pageTemplate, "$656")

	// Handle virtual panel
	if vp := processVirtualPanel(pageTemplate, &cfg, sectionName); vp {
		result.VirtualPanels = true
	}

	// Validate connected_pagination == 2
	connectedPagination, _ := popInt(pageTemplate, "$655")
	if connectedPagination != 2 {
		log.Printf("kfx: error: unexpected connected_pagination: %d", connectedPagination)
	}

	// Get location ID for parent (passed as parentTemplateID to first child only).
	// Port of Python: parent_template_id = self.get_location_id(page_template)
	// then parent_template_id = None after first child.
	locID := getLocationID(pageTemplate)

	// Get story
	storyName, _ := popString(pageTemplate, "$176")
	story, ok := storylines[storyName]
	if !ok {
		result.Err = fmt.Errorf("connected pagination template references missing storyline %q", storyName)
		return result
	}
	delete(story, "$176")

	// Process children with "rendition:page-spread-center"
	children, _ := asSlice(popInterfaceDefault(story, "$146"))
	result.Children = make([]pageSpreadChild, 0, len(children))

	// Port of Python: parent_template_id passed to first child, then set to None.
	childParentID := &locID
	if locID == 0 {
		childParentID = nil
	}

	for _, child := range children {
		childData, ok := asMap(child)
		if !ok {
			continue
		}

		childResult := processPageSpreadPageTemplate(
			childData, sectionName, "rendition:page-spread-center", childParentID, false, cfg, storylines,
		)
		if childResult.Err != nil {
			result.Err = childResult.Err
			return result
		}

		if childResult.VirtualPanels {
			result.VirtualPanels = true
		}
		result.Children = append(result.Children, pageSpreadChild{
			PageSpread: "rendition:page-spread-center",
			Data:       childData,
		})
		result.Sections = append(result.Sections, childResult.Sections...)
		result.Children = append(result.Children, childResult.Children...)

		// After first child, parentTemplateID is set to None (Python: parent_template_id = None)
		childParentID = nil
	}

	return result
}

// processPageSpreadLeaf handles Branch D: leaf content page (default).
// Port of yj_to_epub_content.py:337-343.
func processPageSpreadLeaf(
	pageTemplate map[string]interface{},
	sectionName string,
	pageSpread string,
	parentTemplateID *int,
	isSection bool,
	cfg pageSpreadConfig,
) pageSpreadResult {
	result := pageSpreadResult{}

	// Port of Python L337-338:
	//   spread_type = page_spread.replace("rendition:", "").replace("page-spread-", "")
	//   unique_section_name = "%s-%s" % (section_name, spread_type) if spread_type else section_name
	spreadType := extractSpreadType(pageSpread)
	uniqueName := uniqueSectionName(sectionName, spreadType)

	// Port of Python L339-341:
	//   book_part = self.new_book_part(
	//       filename=self.SECTION_TEXT_FILEPATH % unique_section_name if RETAIN_SECTION_FILENAMES else None,
	//       opf_properties=set(page_spread.split()))
	section := pageSpreadSection{
		PageTitle:        uniqueName,
		Properties:       pageSpread,
		PositionOffset:   0,
		TemplateData:     pageTemplate,
		HasCSSLink:       true, // self.link_css_file(book_part, self.STYLES_CSS_FILEPATH)
		ContentProcessed: true, // self.process_content(page_template, book_part.html, ...)
	}

	// Port of Python L342-343:
	//   self.process_content(page_template, book_part.html, book_part, self.writing_mode, is_section=is_section)
	//   self.link_css_file(book_part, self.STYLES_CSS_FILEPATH)
	// The content processing and CSS linking are recorded via HasCSSLink and ContentProcessed flags.

	// Port of Python L341-342:
	//   if parent_template_id is not None:
	//       self.process_position(parent_template_id, 0, book_part.body())
	if parentTemplateID != nil {
		section.ParentPositionID = *parentTemplateID
		section.HasPositionMarker = true
	}

	result.Sections = append(result.Sections, section)
	return result
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
