package kfx

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
)


// Port of LIST_STYLE_TYPES / list marker → HTML list tag (yj_to_epub_content.py top; used from storyline emit).
var listTagByMarker = map[string]string{
	"alpha_lower": "ol",
	"alpha_upper": "ol",
	"circle": "ul",
	"disc": "ul",
	"image": "ul",
	"none": "ul",
	"numeric": "ol",
	"roman_lower": "ol",
	"roman_upper": "ol",
	"square": "ul",
}

// Port of CLASSIFICATION_EPUB_TYPE (yj_to_epub_content.py) — semantic EPUB type for aside/div from YJ classification.
var classificationEPUBType = map[string]string{
	"yj.chapternote": "footnote",
	"yj.endnote": "endnote",
	"footnote": "footnote",
}

// Port of layout-hint / element name hints from yj_to_epub_content.py (subset used by emit path).
var layoutHintElementNames = map[string]string{
	"caption": "caption",
	"figure": "figure",
	"treat_as_title": "heading",
}

// unusedSectionKeys lists keys that process_section pops from section data before processing
// (yj_to_epub_content.py L124-136).
var unusedSectionKeys = map[string]bool{
	"reading_order_switch_map": true,
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
		featureList, ok := asSlice(features["features"])
		if ok {
			for _, feature := range featureList {
				featureMap, ok := asMap(feature)
				if !ok {
					continue
				}
				featureID, _ := asString(featureMap["key"])
				category, _ := asString(featureMap["namespace"])
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
		if cdeType, ok := asString(metadata["cde_content_type"]); ok {
			if cdeType == "MAGZ" {
				return bookTypeMagazine
			}
		}
	}

	return bookTypeNone
}

// detectBookTypeFromBook derives the book type from the decodedBook flags that were set
// by applyMetadata/applyContentFeatures/applyDocumentData. This is the Go equivalent of
// Python's set_book_type calls scattered across process_metadata_item (yj_to_epub_metadata.py)
// and process_document_data (yj_to_epub_metadata.py).
//
// Python sets book_type via set_book_type() at multiple points during metadata processing.
// Go stores the individual flags (IsPDFBacked, RegionMagnification, etc.) and derives the
// final book type here, matching the priority order in Python:
//
//   1. RegionMagnification (yj_has_text_popups) → children (Python L265)
//   2. CDEContentType MAGZ → magazine (Python L203-204)
//   3. FixedLayout + VirtualPanelsAllowed + !IsPrintReplica → comic (Python L171-173)
//   4. IsPrintReplica → print_replica (Python L255, L279)
//   5. IsPDFBacked (without PrintReplica) → none (book type not set in Python for this case)
//   6. default → none
//
// Note: Python also has comic detection from yj_facing_page, yj_double_page_spread,
// yj_publisher_panels, continuous_popup_progression (value 0), and comic_panel_view_mode ($665).
// These are handled by detectBookType (from features/metadata) which is called first
// in the full detection chain.
func detectBookTypeFromBook(book *decodedBook) bookType {
	if book == nil {
		return bookTypeNone
	}

	// Priority 1: RegionMagnification → children (Python L265: yj_has_text_popups → set_book_type("children"))
	if book.RegionMagnification {
		return bookTypeChildren
	}

	// Priority 2: CDEContentType MAGZ → magazine (Python L203-204)
	if book.CDEContentType == "MAGZ" {
		return bookTypeMagazine
	}

	// Priority 3: Fixed layout + virtual panels + not print_replica → comic
	// (Python L171-173: if book_type is None and fixed_layout and (virtual_panels_allowed or not is_print_replica))
	if book.FixedLayout && (book.VirtualPanelsAllowed || !book.IsPrintReplica) {
		// But only if it's actually a print replica — in that case, skip
		if !book.IsPrintReplica {
			return bookTypeComic
		}
	}

	// Priority 4: Print replica (Python L255, L279)
	if book.IsPrintReplica {
		return bookTypePrintReplica
	}

	return bookTypeNone
}

// detectBookTypeFull performs the complete book type detection chain:
// 1. Try detectBookType from content features and metadata (capability-based detection)
// 2. If none, try detectBookTypeFromBook (flag-based detection from metadata items)
// This matches Python's multi-source set_book_type calls.
func detectBookTypeFull(book *decodedBook, fragments *fragmentCatalog) bookType {
	// Step 1: Try feature/metadata-based detection first
	var metadata map[string]interface{}
	if fragments != nil {
		metadata = fragments.ReadingOrderMetadata
	}
	bt := detectBookType(metadata, fragments.ContentFeatures)
	if bt != bookTypeNone {
		return bt
	}

	// Step 2: Try flag-based detection from decodedBook
	return detectBookTypeFromBook(book)
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
// In Python, when a $171 condition is found in content with a layout, the layout must be "fixed"
// and the condition must match a specific structure: IonSExp of length 3, where
// fv[0] in CONDITION_OPERATOR_NAMES ($294/$299/$298), fv[1]=="position",
// fv[2] is IonSExp of length 2 with fv[2][0]=="anchor".
// Returns true if the condition is valid, false and logs an error if not.
func validateOverlayCondition(condition interface{}, layout string) bool {
	if condition == nil {
		return true
	}
	// Python L492: if layout != "fixed": log.error("Conditional page template has unexpected layout")
	if layout != "fixed" {
		log.Printf("kfx: error: Conditional page template has unexpected layout: %s", layout)
	}
	// Python L500-504: validate condition structure
	// ion_type(condition) is IonSExp and len(condition) == 3 and condition[1] == "position" and
	// ion_type(condition[2]) is IonSExp and len(condition[2]) == 2 and condition[2][0] == "anchor" and
	// condition[0] in self.CONDITION_OPERATOR_NAMES
	slice, ok := asSlice(condition)
	if !ok || len(slice) != 3 {
		log.Printf("kfx: error: Condition is not in expected format: %v", condition)
		return false
	}
	fv0, _ := asString(slice[0])
	fv1, _ := asString(slice[1])
	if fv1 != "position" {
		log.Printf("kfx: error: Condition is not in expected format: %v", condition)
		return false
	}
	fv2, ok := asSlice(slice[2])
	if !ok || len(fv2) != 2 {
		log.Printf("kfx: error: Condition is not in expected format: %v", condition)
		return false
	}
	fv20, _ := asString(fv2[0])
	if fv20 != "anchor" {
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
	cfg *pageSpreadConfig,
) {
	// Port of Python's used_sections set for deduplication (L107).
	usedSections := map[string]bool{}

	// Determine book type from config if provided, otherwise none.
	bt := bookTypeNone
	if cfg != nil {
		bt = cfg.BookType
	}

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

		var rendered renderedStoryline
		var paragraphs []string
		var sectionOK bool

		if cfg != nil {
			// Use full dispatch when book type config is available.
			rendered, paragraphs, sectionOK = processSectionWithType(sectionID, section, index, bt, *cfg, storylines, contentFragments, renderer)
		} else {
			// Fallback to simple processSection for backward compatibility.
			rendered, paragraphs, sectionOK = processSection(sectionID, section, index, storylines, contentFragments, renderer)
		}

		if !sectionOK {
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
			Filename:         sectionFilename(sectionID),
			Title:            title,
			PageTitle:        sectionID,
			Language:         normalizeLanguage(book.Language),
			BodyClass:        rendered.BodyClass,
			BodyStyle:        rendered.BodyStyle,
			BodyStyleInferred: rendered.BodyStyleInferred,
			Paragraphs:       paragraphs,
			Properties:       rendered.Properties,
			Root:             rendered.Root,
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

	// Simple reflowable-only path (no book type dispatch).
	// Book type detection IS fully wired: detectBookTypeFull → pageSpreadConfig →
	// processReadingOrder dispatches to processSectionWithType when config is present.
	// This function is the fallback when no book type config is available.
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
		return processSectionScribePage(section, seq, nil)

	case branchScribeTemplate:
		return processSectionScribeTemplate(section, nil)

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
	nodes, _ := asSlice(storyline["content_list"])
	paragraphs := flattenParagraphs(nodes, contentFragments)
	debugStorylineNodes(sectionID, nodes, 0)
	if os.Getenv("KFX_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "render section=%s pageStyle=%s storyStyle=%s tmplValues=%v\n", sectionID, mainTemplate.PageTemplateStyle, asStringDefault(storyline["style"]), mainTemplate.PageTemplateValues)
	}
	rendered := renderer.renderStoryline(mainTemplate.PositionID, mainTemplate.PageTemplateStyle, mainTemplate.PageTemplateValues, storyline, nodes)

	for _, template := range templates[:mainIndex] {
		overlayStoryline := storylines[template.Storyline]
		if overlayStoryline == nil {
			continue
		}
		overlayNodes, _ := asSlice(overlayStoryline["content_list"])
		overlayParagraphs := flattenParagraphs(overlayNodes, contentFragments)
		paragraphs = append(paragraphs, overlayParagraphs...)
		debugStorylineNodes(sectionID, overlayNodes, 0)
		if os.Getenv("KFX_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "render overlay section=%s pageStyle=%s storyStyle=%s conditional=%v\n", sectionID, template.PageTemplateStyle, asStringDefault(overlayStoryline["style"]), template.HasCondition)
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
func processSectionScribePage(section sectionFragment, seq int, scribeCtx *ScribeNotebookContext) (renderedStoryline, []string, bool) {
	templates := section.PageTemplates
	if len(templates) == 0 {
		return renderedStoryline{}, nil, false
	}
	template := templates[0]
	result := processScribeNotebookPageSection(scribeCtx, section.PageTemplateValues, template.PageTemplateValues, section.ID, seq)
	if !result {
		return renderedStoryline{}, nil, false
	}
	return renderedStoryline{}, nil, false // scribe sections don't produce standard rendered output yet
}

// processSectionScribeTemplate dispatches to the scribe notebook template section handler.
// Port of Python's nmdl.template_type branch in process_section (L139-140).
func processSectionScribeTemplate(section sectionFragment, scribeCtx *ScribeNotebookContext) (renderedStoryline, []string, bool) {
	templates := section.PageTemplates
	if len(templates) == 0 {
		return renderedStoryline{}, nil, false
	}
	template := templates[0]
	result := processScribeNotebookTemplateSection(scribeCtx, section.PageTemplateValues, template.PageTemplateValues, section.ID)
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
	//     self.get_fragment(ftype="structure", fid=page_templates[0]), section_name)
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
		// Python L162: if "condition" not in page_template or self.evaluate_binary_condition(page_template.pop("condition")):
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

		// Python L163: if page_template["type"] != "container":
		ptype, _ := asString(working["type"])
		if ptype != "container" {
			log.Printf("kfx: error: section %s unexpected page_template type %s", section.ID, ptype)
		}

		// Python L167: layout = page_template["layout"]
		layout, _ := asString(working["layout"])

		if layout == "overflow" || layout == "vertical" {
			// Python L169-178: inline content processing
			// page_template.pop("type"); page_template.pop("layout")
			delete(working, "type")
			delete(working, "layout")

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

		} else if layout == "page_spread" {
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
	ptype, _ := asString(pageTemplate["type"])
	layout, _ := asString(pageTemplate["layout"])

	// Branch A: page-spread ($437) / facing-page ($438)
	if ptype == "container" && (layout == "page_spread" || layout == "facing_page") {
		if layout == "facing_page" {
			return pageSpreadBranchFacing
		}
		return pageSpreadBranchSpread
	}

	// Branch B: PDF-backed scale_fit ($326) — only for section-level templates
	if ptype == "container" && layout == "scale_fit" && isSection {
		return pageSpreadBranchScaleFit
	}

	// Branch C: connected pagination ($323/$656)
	if ptype == "container" && layout == "vertical" {
		if hasBool, _ := asBool(pageTemplate["yj.enable_connected_dps"]); hasBool {
			return pageSpreadBranchConnected
		}
	}

	// Branch D: leaf content (default)
	return pageSpreadBranchLeaf
}

// getLocationID extracts the location ID from a template by popping $155, then $598.
// Port of Python's get_location_id (yj_to_epub_navigation.py:372-373).
func getLocationID(data map[string]interface{}) int {
	if id, ok := popInt(data, "id"); ok {
		return id
	}
	if id, ok := popInt(data, "kfx_id"); ok {
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
	case "page_spread":
		return "page-spread"
	case "facing_page":
		return "facing-page"
	default:
		return ""
	}
}

// processVirtualPanel handles the $434 (virtual_panel) key from a page template.
// Port of Python's virtual_panel handling at yj_to_epub_content.py:219-226, 265-272, 303-310.
// Returns true if virtual_panels should be activated.
func processVirtualPanel(data map[string]interface{}, cfg *pageSpreadConfig, sectionName string) bool {
	virtualPanel := popInterfaceDefault(data, "virtual_panel")
	if virtualPanel == nil {
		if cfg.BookType == bookTypeComic && !cfg.RegionMagnification {
			log.Printf("kfx: error: section %s has missing virtual panel in comic without region magnification", sectionName)
		}
		return false
	}
	vpStr, _ := asString(virtualPanel)
	if vpStr == "enabled" && cfg.VirtualPanelsAllowed {
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
	if ptStr, ok := asString(pageTemplate["type"]); !ok || ptStr == "" {
		// Template might be a symbol reference — in Go's pipeline this is already resolved
		// via pageTemplateFragment. For standalone calls, skip.
		if len(pageTemplate) == 0 {
			// Empty template → leaf branch
			return processPageSpreadLeaf(pageTemplate, sectionName, pageSpread, parentTemplateID, isSection, cfg)
		}
	}

	branch := determinePageSpreadBranch(pageTemplate, isSection)

	// Python condition for scale_fit branch:
	//   self.is_pdf_backed and "fixed_height" not in page_template and "fixed_width" not in page_template
	// If not PDF-backed, OR if $67/$66 are present in pageTemplate, fall to leaf branch.
	if branch == pageSpreadBranchScaleFit {
		if !cfg.IsPdfBacked {
			branch = pageSpreadBranchLeaf
		} else if _, has67 := pageTemplate["fixed_height"]; has67 {
			branch = pageSpreadBranchLeaf
		} else if _, has66 := pageTemplate["fixed_width"]; has66 {
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
	delete(pageTemplate, "type")
	layout, _ := popString(pageTemplate, "layout")

	// Handle virtual panel
	if vp := processVirtualPanel(pageTemplate, &cfg, sectionName); vp {
		result.VirtualPanels = true
	}

	// Pop unused keys
	delete(pageTemplate, "direction")
	delete(pageTemplate, "fixed_height")
	delete(pageTemplate, "fixed_width")
	delete(pageTemplate, "float")
	delete(pageTemplate, "writing_mode")

	// Get location ID for parent (passed as parentTemplateID to first child only).
	// Port of Python: parent_template_id = self.get_location_id(page_template)
	// then parent_template_id = None after first child.
	locID := getLocationID(pageTemplate)

	// Get story from storyline reference
	storyName, _ := popString(pageTemplate, "story_name")
	story, ok := storylines[storyName]
	if !ok {
		result.Err = fmt.Errorf("page spread template references missing storyline %q", storyName)
		return result
	}

	// Pop storyline name from story data
	delete(story, "story_name")

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
	children, _ := asSlice(popInterfaceDefault(story, "content_list"))
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
	delete(pageTemplate, "type")
	delete(pageTemplate, "layout")

	// Handle virtual panel
	if vp := processVirtualPanel(pageTemplate, &cfg, sectionName); vp {
		result.VirtualPanels = true
	}

	// Pop unused keys
	delete(pageTemplate, "direction")
	delete(pageTemplate, "float")
	delete(pageTemplate, "writing_mode")

	// Validate font_size == 16
	fontSize, _ := popInt(pageTemplate, "font_size")
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
	storyName, _ := popString(pageTemplate, "story_name")
	story, ok := storylines[storyName]
	if !ok {
		result.Err = fmt.Errorf("scale_fit template references missing storyline %q", storyName)
		return result
	}
	delete(story, "story_name")

	// Process children without page_spread alternation
	children, _ := asSlice(popInterfaceDefault(story, "content_list"))
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
	delete(pageTemplate, "type")
	delete(pageTemplate, "layout")
	delete(pageTemplate, "yj.enable_connected_dps")

	// Handle virtual panel
	if vp := processVirtualPanel(pageTemplate, &cfg, sectionName); vp {
		result.VirtualPanels = true
	}

	// Validate connected_pagination == 2
	connectedPagination, _ := popInt(pageTemplate, "yj.connected_pagination")
	if connectedPagination != 2 {
		log.Printf("kfx: error: unexpected connected_pagination: %d", connectedPagination)
	}

	// Get location ID for parent (passed as parentTemplateID to first child only).
	// Port of Python: parent_template_id = self.get_location_id(page_template)
	// then parent_template_id = None after first child.
	locID := getLocationID(pageTemplate)

	// Get story
	storyName, _ := popString(pageTemplate, "story_name")
	story, ok := storylines[storyName]
	if !ok {
		result.Err = fmt.Errorf("connected pagination template references missing storyline %q", storyName)
		return result
	}
	delete(story, "story_name")

	// Process children with "rendition:page-spread-center"
	children, _ := asSlice(popInterfaceDefault(story, "content_list"))
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

// =============================================================================
// Region magnification — VAL-M3-ILLUST-001/002/003
// Port of Python yj_to_epub_content.py:674-694 (process_content $426 handling)
// =============================================================================

// regionMagnificationConfig holds the current region magnification state.
// Port of self.region_magnification boolean from Python's KFX_EPUB mixin.
type regionMagnificationConfig struct {
	RegionMagnification bool // self.region_magnification
}

// linkRegistration represents a register_link_id call result.
// Port of Python: self.register_link_id(eid, kind) → anchor_name.
type linkRegistration struct {
	Kind string // "magnify_target" or "magnify_source"
	EID  string // the entity ID from $163 or $474
}

// regionMagnificationResult holds the output of processRegionMagnification.
type regionMagnificationResult struct {
	ActivateElements      []htmlElement       // <a class="app-amzn-magnify"> elements
	LinkRegistrations     []linkRegistration  // magnify_target/magnify_source registrations
	AutoEnabled           bool                // set when $426 found without prior region_magnification
	HasUnknownActionError bool                // set when $428 action is not "zoom_in"
}

// processRegionMagnification handles $426 (activate) entries within container content.
// Port of Python yj_to_epub_content.py:674-694.
//
// Python reference:
//
//	if "activate" in content:
//	    if not self.region_magnification:
//	        log.error("activate found without region magnification")
//	        self.region_magnification = True
//	    ordinal = content.pop("ordinal")
//	    for activate in content.pop("activate"):
//	        action = activate.pop("action")
//	        if action == "zoom_in":
//	            activate_elem = etree.SubElement(content_elem, "a")
//	            activate_elem.set("class", "app-amzn-magnify")
//	            activate_elem.set("data-app-amzn-magnify", json_serialize_compact(OD(
//	                "targetId", self.register_link_id(activate.pop("target"), "magnify_target"),
//	                "sourceId", self.register_link_id(activate.pop("source"), "magnify_source"),
//	                "ordinal", ordinal)))
//	            self.check_empty(activate, ...)
//	        else:
//	            log.error(...)
func processRegionMagnification(content map[string]interface{}, cfg *regionMagnificationConfig) regionMagnificationResult {
	result := regionMagnificationResult{}

	// Check if $426 (activate) is present
	activatesRaw, hasActivates := content["activate"]
	if !hasActivates {
		return result
	}

	// Auto-enable region magnification if not already enabled
	// Python: if not self.region_magnification: log.error("activate found without region magnification"); self.region_magnification = True
	if !cfg.RegionMagnification {
		log.Printf("kfx: error: activate found without region magnification")
		cfg.RegionMagnification = true
		result.AutoEnabled = true
	}

	// Pop ordinal ($427)
	// Python: ordinal = content.pop("ordinal")
	ordinalRaw := popInterfaceDefault(content, "ordinal")
	ordinal := 0
	if ordinalRaw != nil {
		if iv, ok := asInt(ordinalRaw); ok {
			ordinal = iv
		}
	}

	// Pop activate list ($426)
	// Python: for activate in content.pop("activate"):
	activates, ok := asSlice(activatesRaw)
	if !ok {
		return result
	}

	for _, actRaw := range activates {
		activate, ok := asMap(actRaw)
		if !ok {
			continue
		}

		// Pop action ($428)
		// Python: action = activate.pop("action")
		actionRaw := popInterfaceDefault(activate, "action")
		action, _ := asString(actionRaw)

		if action == "zoom_in" {
			// Pop target ($163) and source ($474)
			// Python:
			//   activate.pop("target") → register_link_id(eid, "magnify_target")
			//   activate.pop("source") → register_link_id(eid, "magnify_source")
			targetEIDRaw := popInterfaceDefault(activate, "target")
			targetEID, _ := asString(targetEIDRaw)
			sourceEIDRaw := popInterfaceDefault(activate, "source")
			sourceEID, _ := asString(sourceEIDRaw)

			// Register link IDs (Python: register_link_id creates "magnify_target_<eid>" anchor)
			targetAnchor := "magnify_target_" + targetEID
			sourceAnchor := "magnify_source_" + sourceEID

			result.LinkRegistrations = append(result.LinkRegistrations,
				linkRegistration{Kind: "magnify_target", EID: targetEID},
				linkRegistration{Kind: "magnify_source", EID: sourceEID},
			)

			// Build JSON data attribute (Python: json_serialize_compact(OD(...)))
			// OD creates an OrderedDict with keys in insertion order:
			//   targetId, sourceId, ordinal
			magnifyData := map[string]interface{}{
				"targetId": targetAnchor,
				"sourceId": sourceAnchor,
				"ordinal":  ordinal,
			}
			jsonBytes, err := json.Marshal(magnifyData)
			if err != nil {
				log.Printf("kfx: error: failed to serialize magnify data: %v", err)
				continue
			}
			// json.Marshal produces compact JSON with sorted keys by default.
			// Python's json_serialize_compact with OD preserves insertion order.
			// We need to produce ordered JSON matching Python: {"targetId":...,"sourceId":...,"ordinal":...}
			// Since Go's json.Marshal sorts keys alphabetically, we need manual ordering.
			jsonStr := `{"targetId":"` + targetAnchor + `","sourceId":"` + sourceAnchor + `","ordinal":` + fmt.Sprintf("%d", ordinal) + `}`

			_ = jsonBytes // used for debugging, jsonStr is manually ordered

			// Create <a class="app-amzn-magnify"> element
			elem := htmlElement{
				Tag: "a",
				Attrs: map[string]string{
					"class":                   "app-amzn-magnify",
					"data-app-amzn-magnify":   jsonStr,
				},
			}

			result.ActivateElements = append(result.ActivateElements, elem)
		} else {
			// Python: log.error("%s has unknown %s action: %s" % ...)
			log.Printf("kfx: error: activate has unknown action: %s", action)
			result.HasUnknownActionError = true
		}
	}

	return result
}

func isLinkContainerProperty(prop string) bool {
	if heritableProperties[prop] && !reverseHeritablePropertiesExcludes[prop] {
		return true
	}
	switch prop {
	case "-kfx-attrib-colspan", "-kfx-attrib-rowspan", "-kfx-table-vertical-align",
		"-kfx-box-align", "-kfx-heading-level", "-kfx-layout-hints",
		"-kfx-link-color", "-kfx-visited-color":
		return true
	}
	return false
}

func createContainer(contentElem *htmlElement, contentStyle map[string]string, tag string, isContainerProperty func(string) bool) (*htmlElement, map[string]string) {
	containerElem := &htmlElement{Tag: tag, Attrs: map[string]string{}}
	containerElem.Children = []htmlPart{contentElem}

	containerStyle := map[string]string{}
	for prop, val := range contentStyle {
		if isContainerProperty(prop) {
			containerStyle[prop] = val
			delete(contentStyle, prop)
		}
	}

	if styleName, ok := contentStyle["-kfx-style-name"]; ok {
		containerStyle["-kfx-style-name"] = styleName
	}

	return containerElem, containerStyle
}

func createSpanSubcontainer(contentElem *htmlElement, contentStyle map[string]string) *htmlElement {
	subcontainerElem := &htmlElement{Tag: "span", Attrs: map[string]string{}}

	subcontainerElem.Children = append(subcontainerElem.Children, contentElem.Children...)
	contentElem.Children = []htmlPart{subcontainerElem}

	if styleName, ok := contentStyle["-kfx-style-name"]; ok {
		if subcontainerElem.Attrs == nil {
			subcontainerElem.Attrs = map[string]string{}
		}
		subcontainerElem.Attrs["style"] = "-kfx-style-name: " + styleName
	}

	return subcontainerElem
}

func fixVerticalAlignProperties(contentElem *htmlElement, contentStyle map[string]string) map[string]string {
	for _, prop := range []string{"-kfx-baseline-shift", "-kfx-baseline-style", "-kfx-table-vertical-align"} {
		outerVerticalAlign, ok := contentStyle[prop]
		if !ok {
			continue
		}
		delete(contentStyle, prop)

		existingVA, hasVA := contentStyle["vertical-align"]
		if !hasVA {
			contentStyle["vertical-align"] = outerVerticalAlign
		} else if existingVA != outerVerticalAlign {
			subcontainerElem := createSpanSubcontainer(contentElem, contentStyle)
			if subcontainerElem.Attrs == nil {
				subcontainerElem.Attrs = map[string]string{}
			}
			subcontainerElem.Attrs["style"] = "vertical-align: " + existingVA
			delete(contentStyle, "vertical-align")
			contentStyle["vertical-align"] = outerVerticalAlign
		}
	}

	return contentStyle
}

// ---------------------------------------------------------------------------
// Merged from fragments.go (origin: yj_to_epub_content.py)
// ---------------------------------------------------------------------------


// Port of Python process_reading_order reading order iteration (yj_to_epub_content.py L105+).
// Python iterates all reading orders; Go merges all section lists.
func readSectionOrder(value map[string]interface{}) []string {
	entries, ok := asSlice(value["reading_orders"])
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var result []string
	for _, entry := range entries {
		entryMap, ok := asMap(entry)
		if !ok {
			continue
		}
		sections, ok := asSlice(entryMap["sections"])
		if !ok {
			continue
		}
		for _, item := range sections {
			if text, ok := asString(item); ok && text != "" && !seen[text] {
				seen[text] = true
				result = append(result, text)
			}
		}
	}
	return result
}

func parsePositionMapSectionID(fragmentID string, value map[string]interface{}) string {
	return chooseFragmentIdentity(fragmentID, value["section_name"])
}

func readPositionMap(value map[string]interface{}) []int {
	entries, ok := asSlice(value["contains"])
	if !ok {
		return nil
	}
	positions := make([]int, 0, len(entries))
	for _, entry := range entries {
		pair, ok := asSlice(entry)
		if !ok || len(pair) != 2 {
			continue
		}
		positionID, ok := asInt(pair[1])
		if !ok || positionID == 0 {
			continue
		}
		positions = append(positions, positionID)
	}
	return positions
}

func sectionStorylineID(section map[string]interface{}) string {
	containers, ok := asSlice(section["page_templates"])
	if !ok || len(containers) == 0 {
		return ""
	}
	first, ok := asMap(containers[0])
	if !ok {
		return ""
	}
	storylineID, _ := asString(first["story_name"])
	return storylineID
}

func parseSectionFragment(fragmentID string, value map[string]interface{}) sectionFragment {
	id := chooseFragmentIdentity(fragmentID, value["section_name"])
	containers, ok := asSlice(value["page_templates"])
	if !ok || len(containers) == 0 {
		return sectionFragment{ID: id, RawValue: value}
	}
	templates := make([]pageTemplateFragment, 0, len(containers))
	for _, raw := range containers {
		container, ok := asMap(raw)
		if !ok {
			continue
		}
		storylineID, _ := asString(container["story_name"])
		pageTemplateStyle, _ := asString(container["style"])
		positionID, _ := asInt(container["id"])
		templates = append(templates, pageTemplateFragment{
			PositionID:         positionID,
			Storyline:          storylineID,
			PageTemplateStyle:  pageTemplateStyle,
			PageTemplateValues: filterBodyStyleValues(container),
			HasCondition:       container["condition"] != nil,
			Condition:          container["condition"],
		})
	}
	if len(templates) == 0 {
		return sectionFragment{ID: id, RawValue: value}
	}
	mainTemplate := templates[len(templates)-1]
	return sectionFragment{
		ID:                 id,
		PositionID:         mainTemplate.PositionID,
		Storyline:          mainTemplate.Storyline,
		PageTemplateStyle:  mainTemplate.PageTemplateStyle,
		PageTemplateValues: mainTemplate.PageTemplateValues,
		PageTemplates:      templates,
		RawValue:           value,
	}
}

func collectStorylinePositions(nodes []interface{}, sectionID string, positions map[int]string) {
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		if positionID, ok := asInt(node["id"]); ok && positionID != 0 && positions[positionID] == "" {
			positions[positionID] = sectionID
		}
		if children, ok := asSlice(node["content_list"]); ok {
			collectStorylinePositions(children, sectionID, positions)
		}
		if cols, ok := asSlice(node["column_format"]); ok {
			collectStorylinePositions(cols, sectionID, positions)
		}
	}
}

func parseAnchorFragment(fragmentID string, value map[string]interface{}) anchorFragment {
	id := chooseFragmentIdentity(fragmentID, value["anchor_name"])
	if debugAnchors := os.Getenv("KFX_DEBUG_ANCHORS"); debugAnchors != "" {
		for _, wanted := range strings.Split(debugAnchors, ",") {
			if strings.TrimSpace(wanted) == id || strings.TrimSpace(wanted) == fragmentID {
				fmt.Fprintf(os.Stderr, "anchor fragment id=%s fragment=%s value=%#v\n", id, fragmentID, value)
			}
		}
	}
	if uri, ok := asString(value["uri"]); ok {
		if uri == "http://" || uri == "https://" {
			uri = ""
		}
		return anchorFragment{ID: id, URI: uri}
	}
	// Port of Python yj_to_epub_navigation.py:55-63 — anchor fragments with $183
	// reference a position. Python: register_anchor(name, get_position(anchor.pop("position")))
	// where get_position extracts (eid, offset) from $183.$155/$598 and $183.$143.
	target, ok := asMap(value["position"])
	if !ok {
		return anchorFragment{ID: id}
	}
	positionID, _ := asInt(target["id"])
	offset, _ := asInt(target["offset"])
	return anchorFragment{
		ID:         id,
		PositionID: positionID,
		Offset:     offset,
	}
}

func chooseFragmentIdentity(fragmentID string, rawValue interface{}) string {
	valueID, _ := asString(rawValue)
	if isResolvedIdentity(valueID) {
		return valueID
	}
	if isResolvedIdentity(fragmentID) {
		return fragmentID
	}
	if valueID != "" {
		return valueID
	}
	return fragmentID
}

func isResolvedIdentity(value string) bool {
	if value == "" {
		return false
	}
	// A "resolved" identity is one that is NOT a shared symbol name.
	// Shared symbols (like "section", "container") are used as fallback IDs
	// when no specific fragment ID is available — they are not resolved identities.
	return !isSharedSymbolName(value)
}

func isPlaceholderSymbol(value string) bool {
	// With real names, a "placeholder" is a shared symbol name.
	// These are names like "section", "container", etc. that come from the
	// YJ shared symbol table — they are placeholders in the sense that they
	// identify a fragment type, not a specific fragment instance.
	return isSharedSymbolName(value)
}

// ---------------------------------------------------------------------------
// Merged from html.go (origin: yj_to_epub_content.py)
// ---------------------------------------------------------------------------


type htmlPart interface{}

type htmlText struct {
	Text string
}

type htmlElement struct {
	Tag      string
	Attrs    map[string]string
	Children []htmlPart
}

func splitTextHTMLParts(text string) []htmlPart {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	parts := make([]htmlPart, 0, len(lines)*2)
	for index, line := range lines {
		if index > 0 {
			parts = append(parts, &htmlElement{Tag: "br", Attrs: map[string]string{}})
		}
		if line != "" {
			parts = append(parts, htmlText{Text: line})
		}
	}
	return parts
}

func appendTextHTMLParts(element *htmlElement, text string) {
	if element == nil || text == "" {
		return
	}
	// Merge with last htmlText child if possible, matching Python's etree text
	// concatenation behavior. Without merging, the annotation event loop creates
	// individual htmlText nodes per character, which breaks locateOffsetIn for
	// page markers when wrapped in spans.
	if len(element.Children) > 0 {
		switch last := element.Children[len(element.Children)-1].(type) {
		case htmlText:
			element.Children[len(element.Children)-1] = htmlText{Text: last.Text + text}
			return
		case *htmlText:
			element.Children[len(element.Children)-1] = &htmlText{Text: last.Text + text}
			return
		}
	}
	element.Children = append(element.Children, splitTextHTMLParts(text)...)
}

func locateOffset(root *htmlElement, offset int) *htmlElement {
	if root == nil || offset < 0 {
		return nil
	}
	if found, ok := locateOffsetIn(root, offset); ok {
		return found
	}
	return nil
}

func locateOffsetIn(elem *htmlElement, offset int) (*htmlElement, bool) {
	if elem == nil {
		return nil, false
	}
	if elem.Tag == "img" {
		return elem, offset == 0
	}
	for index := 0; index < len(elem.Children); index++ {
		switch child := elem.Children[index].(type) {
		case htmlText:
			length := len([]rune(child.Text))
			if offset == 0 {
				span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
				elem.Children = insertHTMLParts(elem.Children, index, []htmlPart{span})
				return span, true
			}
			if offset < length {
				runes := []rune(child.Text)
				parts := make([]htmlPart, 0, 3)
				if offset > 0 {
					parts = append(parts, htmlText{Text: string(runes[:offset])})
				}
				span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
				parts = append(parts, span)
				if offset < len(runes) {
					parts = append(parts, htmlText{Text: string(runes[offset:])})
				}
				elem.Children = replaceHTMLParts(elem.Children, index, parts)
				return span, true
			}
			offset -= length
		case *htmlElement:
			if offset == 0 {
				// Port of Python locate_offset_in (yj_to_epub_content.py:1588-1589):
				// When offset is 0, return the element directly.
				// However, Python processes position anchors BEFORE annotations/style events,
				// so the element is always a raw text span. Go processes style events first,
				// so we may find styled spans, <a> links, or other annotation-created elements.
				// We need to insert an empty <span/> BEFORE it as a sibling, matching
				// Python's behavior where the anchor is a separate element.
				if child.Tag == "span" && len(child.Attrs) > 0 {
					// Styled span at offset 0: insert empty span before it as sibling
					span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
					elem.Children = insertHTMLParts(elem.Children, index, []htmlPart{span})
					return span, true
				}
				if child.Tag == "span" && len(child.Attrs) == 0 && len(child.Children) >= 1 {
					// Unstyled wrapper span: insert empty span as first child
					firstIsText := false
					switch child.Children[0].(type) {
					case htmlText, *htmlText:
						firstIsText = true
					}
					if firstIsText {
						span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
						child.Children = insertHTMLParts(child.Children, 0, []htmlPart{span})
						return span, true
					}
				}
				// Annotation-created elements (<a>, styled <div>, etc.) at offset 0:
				// insert empty <span/> BEFORE them as a sibling, matching Python's
				// behavior where position anchors are placed before annotation elements.
				// Python processes position anchors first, then annotations, so the
				// anchor span exists as a separate element before the <a> is created.
				if child.Tag != "span" || len(child.Attrs) > 0 {
					span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
					elem.Children = insertHTMLParts(elem.Children, index, []htmlPart{span})
					return span, true
				}
				return child, true
			}
			length := htmlPartLength(child)
			if offset < length {
				return locateOffsetIn(child, offset)
			}
			offset -= length
		}
	}
	if offset == 0 {
		span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
		elem.Children = append(elem.Children, span)
		return span, true
	}
	return nil, false
}

func htmlPartLength(part htmlPart) int {
	switch typed := part.(type) {
	case htmlText:
		return len([]rune(typed.Text))
	case *htmlElement:
		if typed == nil {
			return 0
		}
		if typed.Tag == "img" {
			return 1
		}
		length := 0
		for _, child := range typed.Children {
			length += htmlPartLength(child)
		}
		return length
	default:
		return 0
	}
}

func replaceHTMLParts(parts []htmlPart, index int, replacement []htmlPart) []htmlPart {
	out := make([]htmlPart, 0, len(parts)-1+len(replacement))
	out = append(out, parts[:index]...)
	out = append(out, replacement...)
	out = append(out, parts[index+1:]...)
	return out
}

func insertHTMLParts(parts []htmlPart, index int, inserted []htmlPart) []htmlPart {
	out := make([]htmlPart, 0, len(parts)+len(inserted))
	out = append(out, parts[:index]...)
	out = append(out, inserted...)
	out = append(out, parts[index:]...)
	return out
}

func renderHTMLParts(parts []htmlPart, multiline bool) string {
	var out strings.Builder
	for index, part := range parts {
		if index > 0 && multiline {
			out.WriteByte('\n')
		}
		out.WriteString(renderHTMLPart(part))
	}
	return out.String()
}

func renderHTMLPart(part htmlPart) string {
	switch typed := part.(type) {
	case nil:
		return ""
	case htmlText:
		return escapeHTML(typed.Text)
	case *htmlText:
		return escapeHTML(typed.Text)
	case *htmlElement:
		return renderHTMLElement(typed)
	default:
		return ""
	}
}

type preformatState struct {
	firstInBlock     bool
	previousChar     rune
	previousReplaced bool
	priorText        *htmlText
}

func (s *preformatState) reset() {
	s.firstInBlock = true
	s.previousChar = 0
	s.previousReplaced = false
	s.priorText = nil
}

func (s *preformatState) setMediaBoundary() {
	s.firstInBlock = false
	s.previousChar = '?'
	s.previousReplaced = false
	s.priorText = nil
}

func normalizeHTMLWhitespace(root *htmlElement) {
	if root == nil {
		return
	}
	state := &preformatState{}
	state.reset()
	root.Children = normalizeHTMLChildren(root.Tag, root.Children, state)
}

func normalizeHTMLChildren(tag string, children []htmlPart, state *preformatState) []htmlPart {
	if state == nil {
		state = &preformatState{}
		state.reset()
	}
	switch {
	case isPreformatMediaTag(tag):
		state.setMediaBoundary()
	case isPreformatInlineTag(tag):
	default:
		state.reset()
	}
	normalized := make([]htmlPart, 0, len(children))
	for _, child := range children {
		switch typed := child.(type) {
		case nil:
			continue
		case htmlText:
			normalized = append(normalized, normalizeHTMLTextParts(typed.Text, state)...)
		case *htmlText:
			normalized = append(normalized, normalizeHTMLTextParts(typed.Text, state)...)
		case *htmlElement:
			typed.Children = normalizeHTMLChildren(typed.Tag, typed.Children, state)
			normalized = append(normalized, typed)
		default:
			normalized = append(normalized, child)
		}
	}
	return normalized
}

func normalizeHTMLTextParts(text string, state *preformatState) []htmlPart {
	if text == "" {
		return nil
	}
	parts := []htmlPart{}
	var segment []rune
	flushSegment := func() {
		if len(segment) == 0 {
			return
		}
		if part := preformatHTMLText(string(segment), state); part != nil {
			parts = append(parts, part)
		}
		segment = segment[:0]
	}
	for _, ch := range text {
		if isEOLRune(ch) {
			flushSegment()
			parts = append(parts, &htmlElement{Tag: "br", Attrs: map[string]string{}})
			state.reset()
			continue
		}
		segment = append(segment, ch)
	}
	flushSegment()
	return parts
}

func preformatHTMLText(text string, state *preformatState) htmlPart {
	if text == "" {
		return nil
	}
	runes := []rune(text)
	out := make([]rune, 0, len(runes))
	for _, ch := range runes {
		orig := ch
		didReplace := false
		if ch == ' ' && (state.firstInBlock || state.previousChar == ' ') {
			if state.previousChar == ' ' && !state.previousReplaced {
				if len(out) > 0 {
					out[len(out)-1] = '\u00a0'
				} else if state.priorText != nil {
					state.priorText.Text = replaceLastRune(state.priorText.Text, '\u00a0')
				}
			}
			ch = '\u00a0'
			didReplace = true
		}
		out = append(out, ch)
		state.firstInBlock = false
		state.previousChar = orig
		state.previousReplaced = didReplace
	}
	part := &htmlText{Text: string(out)}
	state.priorText = part
	return part
}

func replaceLastRune(text string, replacement rune) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return text
	}
	runes[len(runes)-1] = replacement
	return string(runes)
}

func isEOLRune(ch rune) bool {
	switch ch {
	case '\n', '\r', '\u2028', '\u2029':
		return true
	default:
		return false
	}
}

func isPreformatInlineTag(tag string) bool {
	switch tag {
	case "a", "b", "bdi", "bdo", "em", "i", "path", "rb", "rt", "ruby", "span", "strong", "sub", "sup", "u":
		return true
	default:
		return false
	}
}

func isPreformatMediaTag(tag string) bool {
	switch tag {
	case "audio", "iframe", "img", "object", "svg", "video":
		return true
	default:
		return false
	}
}

func renderHTMLElement(element *htmlElement) string {
	if element == nil {
		return ""
	}
	if element.Tag == "" {
		return renderHTMLParts(element.Children, false)
	}
	var out strings.Builder
	out.WriteByte('<')
	out.WriteString(element.Tag)
	attrOrder := []string{"id", "class", "href", "src", "alt"}
	switch element.Tag {
	case "a":
		attrOrder = []string{"id", "href", "epub:type", "class"}
	case "img":
		attrOrder = []string{"src", "alt", "id", "class"}
	case "col":
		attrOrder = []string{"span", "class"}
	case "td":
		attrOrder = []string{"id", "colspan", "rowspan", "class"}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		attrOrder = []string{"id", "class"}
	case "p":
		attrOrder = []string{"id", "class"}
	}
	for _, key := range attrOrder {
		value, ok := element.Attrs[key]
		if !ok || (value == "" && key != "alt") {
			continue
		}
		out.WriteString(` ` + key + `="` + escapeHTML(value) + `"`)
	}
	remaining := make([]string, 0, len(element.Attrs))
	seen := map[string]bool{}
	for _, key := range attrOrder {
		seen[key] = true
	}
	for key := range element.Attrs {
		if !seen[key] {
			remaining = append(remaining, key)
		}
	}
	sort.Strings(remaining)
	for _, key := range remaining {
		value := element.Attrs[key]
		if value == "" {
			continue
		}
		out.WriteString(` ` + key + `="` + escapeHTML(value) + `"`)
	}
	if len(element.Children) == 0 {
		out.WriteString(`/>`)
		return out.String()
	}
	out.WriteByte('>')
	for _, child := range element.Children {
		out.WriteString(renderHTMLPart(child))
	}
	out.WriteString(`</` + element.Tag + `>`)
	return out.String()
}

// unexpectedCharacters is the Go equivalent of Python's UNEXPECTED_CHARACTERS set
// (yj_to_epub_content.py L67-84). These are C0/C1 control characters and other invalid
// Unicode that must be filtered from text content before emitting XHTML, since they are
// not valid in XML 1.0 documents.
var unexpectedCharacters = map[rune]bool{
	// C0 control characters (L68-71)
	0x0000: true, 0x0001: true, 0x0002: true, 0x0003: true,
	0x0004: true, 0x0005: true, 0x0006: true, 0x0007: true,
	0x0008: true,
	0x000B: true, 0x000C: true,
	0x000E: true, 0x000F: true,
	0x0010: true, 0x0011: true, 0x0012: true, 0x0013: true,
	0x0014: true, 0x0015: true, 0x0016: true, 0x0017: true,
	0x0018: true, 0x0019: true, 0x001A: true, 0x001B: true,
	0x001C: true, 0x001D: true, 0x001E: true, 0x001F: true,
	// C1 control characters (L72-75)
	0x0080: true, 0x0081: true, 0x0082: true, 0x0083: true,
	0x0084: true, 0x0085: true, 0x0086: true, 0x0087: true,
	0x0088: true, 0x0089: true, 0x008A: true, 0x008B: true,
	0x008C: true, 0x008D: true, 0x008E: true, 0x008F: true,
	0x0090: true, 0x0091: true, 0x0092: true, 0x0093: true,
	0x0094: true, 0x0095: true, 0x0096: true, 0x0097: true,
	0x0098: true, 0x0099: true, 0x009A: true, 0x009B: true,
	0x009C: true, 0x009D: true, 0x009E: true, 0x009F: true,
	// Arabic Letter Mark (L76)
	0x061C: true,
	// Invisible Separator (L77)
	0x2063: true,
	// Interlinear Annotation characters (L78)
	0xFFF9: true, 0xFFFA: true, 0xFFFB: true,
	// Noncharacters (L79)
	0xFFFE: true, 0xFFFF: true,
}

// cleanTextForLXML replaces characters in the UNEXPECTED_CHARACTERS set with "?",
// matching Python's clean_text_for_lxml (yj_to_epub_content.py L1807-1816).
func cleanTextForLXML(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if unexpectedCharacters[r] {
			b.WriteByte('?')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func escapeHTML(text string) string {
	// Filter invalid XML characters before escaping (Python: clean_text_for_lxml, L1807-1816)
	cleaned := cleanTextForLXML(text)
	var replacer = strings.NewReplacer(
		"&", "&amp;",
		`"`, "&quot;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(cleaned)
}

// ---------------------------------------------------------------------------
// Merged from style_events.go (origin: yj_to_epub_content.py)
// ---------------------------------------------------------------------------


// hasTextCombineUprightAll checks whether elem or any of its ancestors up to root
// has a CSS "text-combine-upright: all" style declaration. This is the Go equivalent
// of Python's parent walk in locate_offset_in (yj_to_epub_content.py L1588-1594):
//
//	e = elem
//	while e is not None:
//	    if self.get_style(e).get("text-combine-upright") == "all":
//	        text_len = 1
//	        break
//	    e = e.getparent()
func hasTextCombineUprightAll(root, elem *htmlElement) bool {
	e := elem
	for e != nil {
		style := parseDeclarationString(e.Attrs["style"])
		if style["text-combine-upright"] == "all" {
			return true
		}
		if e == root {
			break
		}
		parent, _ := findParent(root, e)
		e = parent
	}
	return false
}

func locateOffsetFull(root *htmlElement, offset int, splitAfter bool, zeroLen bool, isDropcap bool, textCombineInUse bool) *htmlElement {
	if root == nil || offset < 0 {
		return nil
	}

	result, remaining := locateOffsetInFull(root, root, offset, splitAfter, zeroLen, isDropcap, textCombineInUse)
	if remaining < 0 {
		return result
	}

	if remaining == 0 && !splitAfter {
		span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
		root.Children = append(root.Children, span)
		return span
	}

	if isDropcap {
		span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
		root.Children = append(root.Children, span)
		return span
	}

	return nil
}

func locateOffsetInFull(root *htmlElement, elem *htmlElement, offset int, splitAfter bool, zeroLen bool, isDropcap bool, textCombineInUse bool) (*htmlElement, int) {
	if offset < 0 {
		return nil, offset
	}

	if elem.Tag == "span" {
		textLen := elementTextLen(elem)

		// Port of Python text_combine_in_use optimization (yj_to_epub_content.py L1587-1594):
		// When text_len > 1 and text-combine-upright has been used in this book,
		// walk up the ancestor chain checking for text-combine-upright: all.
		// If found, treat this span's text as a single character (text_len = 1)
		// for offset calculation. This affects CJK vertical-text (tate-chu-yoko) content.
		// Python: if text_len > 1 and self.text_combine_in_use:
		//             e = elem
		//             while e is not None:
		//                 if self.get_style(e).get("text-combine-upright") == "all":
		//                     text_len = 1
		//                     break
		//                 e = e.getparent()
		if textLen > 1 && textCombineInUse && hasTextCombineUprightAll(root, elem) {
			textLen = 1
		}

		if textLen > 0 {
			if !splitAfter {
				if offset == 0 {
					return elem, -1
				} else if offset < textLen {
					newSpan := splitSpan(root, elem, offset)
					if zeroLen {
						splitSpan(root, newSpan, 0)
					}
					return newSpan, -1
				}
			} else {
				if offset == textLen-1 {
					return elem, -1
				} else if offset < textLen {
					splitSpan(root, elem, offset+1)
					return elem, -1
				}
			}

			offset -= textLen
		}

		for _, child := range elem.Children {
			if ce, ok := child.(*htmlElement); ok {
				result, remaining := locateOffsetInFull(root, ce, offset, splitAfter, zeroLen, isDropcap, textCombineInUse)
				if remaining < 0 {
					return result, remaining
				}
				offset = remaining
			}
		}

		return nil, offset
	}

	if isDropcap {
		style := parseDeclarationString(elem.Attrs["style"])
		if style["float"] != "" {
			return nil, offset
		}
	}

	if elem.Tag == "img" || elem.Tag == "svg" || elem.Tag == "math" {
		if offset == 0 {
			return elem, -1
		}
		offset--
		return nil, offset
	}

	switch elem.Tag {
	case "a", "aside", "div", "figure", "h1", "h2", "h3", "h4", "h5", "h6", "li", "ruby", "rb":
		for _, child := range elem.Children {
			if ce, ok := child.(*htmlElement); ok {
				result, remaining := locateOffsetInFull(root, ce, offset, splitAfter, zeroLen, isDropcap, textCombineInUse)
				if remaining < 0 {
					return result, remaining
				}
				offset = remaining
			}
		}
		return nil, offset

	case "rt":
		return nil, offset

	default:
		return nil, offset
	}
}

func elementTextLen(elem *htmlElement) int {
	length := 0
	for _, child := range elem.Children {
		if t, ok := child.(htmlText); ok {
			length += utf8.RuneCountInString(t.Text)
		} else {
			break
		}
	}
	return length
}

func splitSpan(root *htmlElement, oldSpan *htmlElement, firstTextLen int) *htmlElement {
	var textRunes []rune
	textEnd := 0
	for i, child := range oldSpan.Children {
		if t, ok := child.(htmlText); ok {
			textRunes = append(textRunes, []rune(t.Text)...)
			textEnd = i + 1
		} else {
			break
		}
	}

	var firstText string
	var secondText string
	if firstTextLen <= 0 {
		firstText = ""
		secondText = string(textRunes)
	} else if firstTextLen >= len(textRunes) {
		firstText = string(textRunes)
		secondText = ""
	} else {
		firstText = string(textRunes[:firstTextLen])
		secondText = string(textRunes[firstTextLen:])
	}

	var firstParts []htmlPart
	if firstText != "" {
		firstParts = []htmlPart{htmlText{Text: firstText}}
	}

	remaining := oldSpan.Children[textEnd:]

	newChildren := make([]htmlPart, 0, len(firstParts)+len(remaining))
	newChildren = append(newChildren, firstParts...)
	newChildren = append(newChildren, remaining...)
	oldSpan.Children = newChildren

	newSpan := &htmlElement{Tag: "span", Attrs: cloneAttrs(oldSpan)}
	if secondText != "" {
		newSpan.Children = []htmlPart{htmlText{Text: secondText}}
	}

	parent, idx := findParent(root, oldSpan)
	if parent != nil {
		parent.Children = insertHTMLParts(parent.Children, idx+1, []htmlPart{newSpan})
	}

	return newSpan
}

func findParent(root, target *htmlElement) (parent *htmlElement, index int) {
	if root == nil || target == nil {
		return nil, -1
	}
	for i, child := range root.Children {
		if ce, ok := child.(*htmlElement); ok {
			if ce == target {
				return root, i
			}
			if p, idx := findParent(ce, target); p != nil {
				return p, idx
			}
		}
	}
	return nil, -1
}

func cloneAttrs(elem *htmlElement) map[string]string {
	if len(elem.Attrs) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(elem.Attrs))
	for k, v := range elem.Attrs {
		cloned[k] = v
	}
	return cloned
}

func hasNoLeadingText(elem *htmlElement) bool {
	for _, child := range elem.Children {
		if t, ok := child.(htmlText); ok {
			if utf8.RuneCountInString(t.Text) > 0 {
				return false
			}
			continue
		}
		break
	}
	return true
}

func isFirstElementChild(parent *htmlElement, child *htmlElement) bool {
	for _, c := range parent.Children {
		if ce, ok := c.(*htmlElement); ok {
			return ce == child
		}
	}
	return false
}

func isLastElementChild(parent *htmlElement, child *htmlElement) bool {
	for i := len(parent.Children) - 1; i >= 0; i-- {
		if ce, ok := parent.Children[i].(*htmlElement); ok {
			return ce == child
		}
	}
	return false
}

func elementTailIsEmpty(parent *htmlElement, childIndex int) bool {
	if childIndex+1 < len(parent.Children) {
		if t, ok := parent.Children[childIndex+1].(htmlText); ok {
			return utf8.RuneCountInString(t.Text) == 0
		}
	}
	return true
}

func findOrCreateStyleEventElement(contentElem *htmlElement, eventOffset int, eventLength int, isDropcap bool) *htmlElement {
	if eventLength <= 0 {
		panic(fmt.Sprintf("style event has length: %d", eventLength))
	}

	first := locateOffsetFull(contentElem, eventOffset, false, false, isDropcap, false)
	if first == nil {
		return nil
	}

	last := locateOffsetFull(contentElem, eventOffset+eventLength-1, true, false, isDropcap, false)

	if last == nil || first == last {
		return first
	}

	firstParent, _ := findParent(contentElem, first)
	lastParent, _ := findParent(contentElem, last)

	if firstParent != lastParent {
		tryFirst := first
		firsts := []*htmlElement{tryFirst}

		for firstParent != nil && hasNoLeadingText(firstParent) && isFirstElementChild(firstParent, tryFirst) {
			tryFirst = firstParent
			firstParent, _ = findParent(contentElem, tryFirst)
			if firstParent != nil {
				firsts = append(firsts, tryFirst)
			}
		}

		tryLast := last
		lasts := []*htmlElement{tryLast}

		_, origLastIdx := findParent(contentElem, last)
		origLastParent := lastParent

		for lastParent != nil && elementTailIsEmpty(origLastParent, origLastIdx) && isLastElementChild(lastParent, tryLast) {
			tryLast = lastParent
			lastParent, _ = findParent(contentElem, tryLast)
			if lastParent != nil {
				lasts = append(lasts, tryLast)
			}
		}

		found := false
		for _, tf := range firsts {
			for _, tl := range lasts {
				tfParent, _ := findParent(contentElem, tf)
				tlParent, _ := findParent(contentElem, tl)
				if tfParent == tlParent && tfParent != nil {
					first = tf
					last = tl
					found = true
					break
				}
			}
			if found {
				break
			}
		}

		if !found {
			return nil
		}
	}

	eventElem := &htmlElement{Tag: "span", Attrs: map[string]string{}}

	seParent, firstIndex := findParent(contentElem, first)
	_, lastIndex := findParent(contentElem, last)

	moved := make([]htmlPart, lastIndex-firstIndex+1)
	copy(moved, seParent.Children[firstIndex:lastIndex+1])

	eventElem.Children = moved

	newChildren := make([]htmlPart, 0, len(seParent.Children)-(lastIndex-firstIndex))
	newChildren = append(newChildren, seParent.Children[:firstIndex]...)
	newChildren = append(newChildren, eventElem)
	newChildren = append(newChildren, seParent.Children[lastIndex+1:]...)
	seParent.Children = newChildren

	return eventElem
}

// ---------------------------------------------------------------------------
// Merged from content_helpers.go (origin: yj_to_epub_content.py)
// ---------------------------------------------------------------------------


var (
	cssIdentPattern = regexp.MustCompile(`^[-_a-zA-Z0-9]*$`)
)

type fontNameFixer struct {
	fixedNames       map[string]string
	nameReplacements map[string]string
}

var currentFontFixer *fontNameFixer

var cssGenericFontNames = map[string]bool{
	"serif":      true,
	"sans-serif": true,
	"cursive":    true,
	"fantasy":    true,
	"monospace":  true,
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func effectiveStyle(base map[string]interface{}, values map[string]interface{}) map[string]interface{} {
	style := cloneMap(base)
	if style == nil {
		style = map[string]interface{}{}
	}
	for key, value := range values {
		style[key] = value
	}
	return style
}

func mergeStyleValues(dst map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = map[string]interface{}{}
	}
	for key, value := range src {
		if _, exists := dst[key]; !exists {
			dst[key] = value
		}
	}
	return dst
}

func filterBodyStyleValues(values map[string]interface{}) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	allowed := map[string]bool{
		"font_family":  true,
		"font_style":  true,
		"font_size":  true,
		"text_indent":  true,
		"text_transform":  true,
		"line_height":  true,
		"margin_top":  true,
		"margin_left":  true,
		"margin_bottom":  true,
		"margin_right":  true,
		"fill_color":  true,
		"fill_opacity":  true,
		"box_align": true,
		"glyph_transform": true,
	}
	filtered := map[string]interface{}{}
	for key, value := range values {
		if allowed[key] {
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func declarationSet(declarations []string) map[string]bool {
	if len(declarations) == 0 {
		return nil
	}
	result := make(map[string]bool, len(declarations))
	for _, declaration := range declarations {
		result[declaration] = true
	}
	return result
}

func inheritedDefaultSet(declarations []string) map[string]bool {
	result := declarationSet(declarations)
	if result == nil {
		result = map[string]bool{}
	}
	hasTextIndent := false
	for declaration := range result {
		if strings.HasPrefix(declaration, "text-indent: ") {
			hasTextIndent = true
			break
		}
	}
	if !hasTextIndent {
		result["text-indent: 0"] = true
	}
	return result
}

func defaultBodyDeclarations(bodyClass string) []string {
	switch bodyClass {
	case "class-0":
		return []string{"font-family: FreeFontSerif,serif", "text-align: center"}
	case "class-1":
		return []string{"font-family: FreeFontSerif,serif"}
	case "class-2":
		return []string{"font-family: FreeFontSerif,serif", "text-align: justify", "text-indent: 1.44em"}
	case "class-3":
		return []string{"font-family: FreeFontSerif,serif", "text-align: justify"}
	case "class-7":
		return []string{"font-family: FreeFontSerif,serif", "font-style: italic", "text-align: justify", "text-indent: 1.44em"}
	case "class-8":
		return []string{"font-family: Shift Light,Palatino,Palatino Linotype,Palatino LT Std,Book Antiqua,Georgia,serif"}
	default:
		return nil
	}
}

// defaultBodyDeclarationsWithFont returns the CSS declarations for a static body class,
// using the resolved default font family. When the resolved font is "serif" (the CSS default),
// font-family is omitted from the declarations since it would be stripped by simplify_styles.
// When the resolved font is something else (e.g. "FreeFontSerif,serif" for Martyr), it is included.
func defaultBodyDeclarationsWithFont(bodyClass string, resolvedFont string) []string {
	switch bodyClass {
	case "class-0":
		if resolvedFont != "" && resolvedFont != "serif" {
			return []string{"font-family: " + resolvedFont, "text-align: center"}
		}
		return []string{"text-align: center"}
	case "class-1":
		if resolvedFont != "" && resolvedFont != "serif" {
			return []string{"font-family: " + resolvedFont}
		}
		return []string{"font-family: serif"}
	case "class-2":
		if resolvedFont != "" && resolvedFont != "serif" {
			return []string{"font-family: " + resolvedFont, "text-align: justify", "text-indent: 1.44em"}
		}
		return []string{"text-align: justify", "text-indent: 1.44em"}
	case "class-3":
		if resolvedFont != "" && resolvedFont != "serif" {
			return []string{"font-family: " + resolvedFont, "text-align: justify"}
		}
		return []string{"text-align: justify"}
	case "class-7":
		if resolvedFont != "" && resolvedFont != "serif" {
			return []string{"font-family: " + resolvedFont, "font-style: italic", "text-align: justify", "text-indent: 1.44em"}
		}
		return []string{"font-style: italic", "text-align: justify", "text-indent: 1.44em"}
	case "class-8":
		return []string{"font-family: Shift Light,Palatino,Palatino Linotype,Palatino LT Std,Book Antiqua,Georgia,serif"}
	default:
		return nil
	}
}

// defaultBodyFontDeclarations returns additional font-family declarations for the body that
// should be used for inheritance filtering. Not needed since defaultBodyDeclarations already
// includes font-family.
func defaultBodyFontDeclarations(bodyClass string) []string {
	return nil
}

func isStaticBodyClass(bodyClass string) bool {
	switch bodyClass {
	case "class-0", "class-1", "class-2", "class-3", "class-7", "class-8":
		return true
	default:
		return false
	}
}

func staticBodyClassForDeclarations(declarations []string) string {
	// Alternates: declarations without font-family (for books where font-family is "serif"
	// and gets stripped by simplify_styles), plus any variant with a non-"serif" font.
	alternates := map[string][][]string{
		"class-0": {
			{"text-align: center"},
		},
		"class-2": {
			{"text-align: justify", "text-indent: 1.44em"},
		},
		"class-3": {
			{"text-align: justify"},
		},
		"class-7": {
			{"font-style: italic", "text-align: justify", "text-indent: 1.44em"},
		},
	}
	for _, bodyClass := range []string{"class-0", "class-1", "class-2", "class-3", "class-7", "class-8"} {
		expected := defaultBodyDeclarations(bodyClass)
		if len(expected) != len(declarations) {
			for _, alternate := range alternates[bodyClass] {
				if len(alternate) != len(declarations) {
					continue
				}
				match := true
				for index := range alternate {
					if alternate[index] != declarations[index] {
						match = false
						break
					}
				}
				if match {
					return bodyClass
				}
			}
			continue
		}
		match := true
		for index := range expected {
			if expected[index] != declarations[index] {
				match = false
				break
			}
		}
		if match {
			return bodyClass
		}
	}
	return ""
}

func flattenParagraphs(nodes []interface{}, contents map[string][]string) []string {
	result := make([]string, 0, 64)
	var walk func(items []interface{})
	walk = func(items []interface{}) {
		for _, item := range items {
			node, ok := asMap(item)
			if !ok {
				continue
			}
			if ref, ok := asMap(node["content"]); ok {
				name, _ := asString(ref["name"])
				index, ok := asInt(ref["index"])
				if ok {
					if values, found := contents[name]; found && index >= 0 && index < len(values) {
						text := strings.TrimSpace(values[index])
						if text != "" {
							result = append(result, text)
						}
					}
				}
			}
			if children, ok := asSlice(node["content_list"]); ok {
				walk(children)
			}
		}
	}
	walk(nodes)
	return result
}

func deriveSectionTitle(paragraphs []string, sectionNumber int) string {
	for _, paragraph := range paragraphs {
		trimmed := strings.TrimSpace(paragraph)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > 80 {
			break
		}
		return trimmed
	}
	return fmt.Sprintf("Section %d", sectionNumber)
}

func naturalSortKey(value string) string {
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

func mapField(value interface{}, key string) (interface{}, bool) {
	if mapped, ok := value.(map[string]interface{}); ok {
		result, found := mapped[key]
		return result, found
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || rv.Kind() != reflect.Map {
		return nil, false
	}
	for _, mapKey := range rv.MapKeys() {
		if mapKeyString(mapKey.Interface()) == key {
			return rv.MapIndex(mapKey).Interface(), true
		}
	}
	return nil, false
}

func mapKeyString(value interface{}) string {
	if text, ok := asString(value); ok {
		return text
	}
	return fmt.Sprint(value)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func debugSectionMappings(sections map[string]sectionFragment, navTitles map[string]string, order []string) {
	if os.Getenv("KFX_DEBUG") == "" {
		return
	}
	for _, sectionID := range order {
		section := sections[sectionID]
		fmt.Fprintf(os.Stderr, "section id=%s pos=%d storyline=%s title=%s\n", sectionID, section.PositionID, section.Storyline, navTitles[sectionID])
	}
}

func debugStorylineNodes(sectionID string, nodes []interface{}, depth int) {
	if os.Getenv("KFX_DEBUG") == "" {
		return
	}
	debugSections := os.Getenv("KFX_DEBUG_SECTIONS")
	if debugSections == "" {
		if sectionID != "c73" && sectionID != "c109" && sectionID != "c6P" {
			return
		}
	} else if !strings.Contains(","+debugSections+",", ","+sectionID+",") {
		return
	}
	prefix := strings.Repeat("  ", depth)
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		positionID, _ := asInt(node["id"])
		styleID, _ := asString(node["style"])
		text := ""
		if ref, ok := asMap(node["content"]); ok {
			text = truncateDebugText(ref)
		}
		fmt.Fprintf(os.Stderr, "story %s %spos=%d type=%s style=%s text=%q keys=%v\n", sectionID, prefix, positionID, asStringDefault(node["type"]), styleID, text, sortedMapKeys(node))
		if cols, ok := asSlice(node["column_format"]); ok {
			fmt.Fprintf(os.Stderr, "story %s %scols=%#v\n", sectionID, prefix, cols)
		}
		if children, ok := asSlice(node["content_list"]); ok {
			debugStorylineNodes(sectionID, children, depth+1)
		}
	}
}

func sortedMapKeys(value map[string]interface{}) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func truncateDebugText(ref map[string]interface{}) string {
	name, _ := asString(ref["name"])
	index, _ := asInt(ref["index"])
	return fmt.Sprintf("%s[%d]", name, index)
}

func asStringDefault(value interface{}) string {
	result, _ := asString(value)
	return result
}

func intPtr(value int) *int {
	return &value
}

// sectionFilename produces the XHTML filename for a section, matching Python's
// SECTION_TEXT_FILEPATH % section_name convention (yj_to_epub_content.py:171,191).
// Python uses the raw $174 symbol directly as the filename, e.g.
// "UYqzWVgySW_Gl4WQ-Od_xQ1.xhtml". Previously Go applied uniquePartOfLocalSymbol
// which stripped base64/UUID prefixes, producing numeric names like "1.xhtml".
// Port of Python: self.SECTION_TEXT_FILEPATH % section_name where section_name
// comes from section.pop("section_name") — used verbatim, no uniquePartOfLocalSymbol.
func sectionFilename(sectionID string) string {
	return sectionID + ".xhtml"
}

func cloneStyleMap(style map[string]string) map[string]string {
	if len(style) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(style))
	for key, value := range style {
		cloned[key] = value
	}
	return cloned
}

func resolveContentText(contentFragments map[string][]string, ref map[string]interface{}) string {
	name, _ := asString(ref["name"])
	index, ok := asInt(ref["index"])
	if !ok {
		return ""
	}
	values := contentFragments[name]
	if index < 0 || index >= len(values) {
		return ""
	}
	// Filter invalid XML characters from KFX text content (Python: clean_text_for_lxml,
	// yj_to_epub_content.py L67-84 + L1807-1816). Applied at the resolution layer
	// so all downstream text consumers get clean text.
	return cleanTextForLXML(values[index])
}

func inferBookLanguage(defaultLanguage string, contentFragments map[string][]string, storylines map[string]map[string]interface{}, styleFragments map[string]map[string]interface{}) string {
	defaultKey := languageKey(defaultLanguage)
	if defaultKey == "" {
		return defaultLanguage
	}
	merits := map[string]int{}
	for _, storyline := range storylines {
		nodes, _ := asSlice(storyline["content_list"])
		accumulateContentLanguageMerits(nodes, defaultKey, merits, contentFragments, styleFragments)
	}
	bestLanguage := defaultKey
	bestMerit := 0
	for language, merit := range merits {
		if merit <= bestMerit || !languageMatchesDefault(language, defaultKey) {
			continue
		}
		bestLanguage = language
		bestMerit = merit
	}
	if bestMerit == 0 {
		return defaultLanguage
	}
	return bestLanguage
}

func accumulateContentLanguageMerits(nodes []interface{}, currentLanguage string, merits map[string]int, contentFragments map[string][]string, styleFragments map[string]map[string]interface{}) {
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		language := currentLanguage
		styleID, _ := asString(node["style"])
		style := effectiveStyle(styleFragments[styleID], node)
		if rawLanguage, ok := asString(style["language"]); ok && rawLanguage != "" {
			language = languageKey(rawLanguage)
		}
		if ref, ok := asMap(node["content"]); ok && language != "" {
			merits[language] += len([]rune(resolveContentText(contentFragments, ref)))
		}
		if children, ok := asSlice(node["content_list"]); ok {
			accumulateContentLanguageMerits(children, language, merits, contentFragments, styleFragments)
		}
	}
}

func languageKey(language string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
}

func languageMatchesDefault(candidate string, defaultLanguage string) bool {
	if candidate == "" || defaultLanguage == "" {
		return false
	}
	return candidate == defaultLanguage || strings.HasPrefix(candidate, defaultLanguage+"-")
}

func bodyPromotionPresenceStyle(bodyClass string) map[string]interface{} {
	switch bodyClass {
	case "class-0":
		return map[string]interface{}{"font_family": true, "text_alignment": true}
	case "class-1":
		return map[string]interface{}{"font_family": true}
	case "class-2":
		return map[string]interface{}{"font_family": true, "text_alignment": true, "text_indent": true}
	case "class-3":
		return map[string]interface{}{"font_family": true, "text_alignment": true}
	default:
		return nil
	}
}

func storylineUsesJustifiedBody(nodes []interface{}) bool {
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		styleID, _ := asString(node["style"])
		if styleID == "s6E" || styleID == "s6G" {
			return true
		}
		if children, ok := asSlice(node["content_list"]); ok && storylineUsesJustifiedBody(children) {
			return true
		}
	}
	return false
}

func estimateBodyClass(nodes []interface{}) string {
	if storylineUsesJustifiedBody(nodes) {
		return "class-2"
	}
	if storylineIsCentered(nodes) {
		return "class-0"
	}
	return "class-1"
}

func storylineIsCentered(nodes []interface{}) bool {
	return !storylineContainsParagraph(nodes)
}

func storylineContainsParagraph(nodes []interface{}) bool {
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		if _, ok := asMap(node["content"]); ok && headingLevel(node) == 0 {
			return true
		}
		if children, ok := asSlice(node["content_list"]); ok && storylineContainsParagraph(children) {
			return true
		}
	}
	return false
}

func appendClassNames(existing string, classNames ...string) string {
	parts := []string{}
	seen := map[string]bool{}
	for _, raw := range append([]string{existing}, classNames...) {
		for _, className := range strings.Fields(strings.TrimSpace(raw)) {
			if className == "" || seen[className] {
				continue
			}
			seen[className] = true
			parts = append(parts, className)
		}
	}
	return strings.Join(parts, " ")
}

func newFontNameFixer() *fontNameFixer {
	return &fontNameFixer{
		fixedNames:       map[string]string{},
		nameReplacements: map[string]string{},
	}
}

// setDefaultFontFamily sets up the default font name replacement map, matching Python's
// process_document_data (yj_to_epub_metadata.py L100-116):
//   self.font_name_replacements["default"] = DEFAULT_DOCUMENT_FONT_FAMILY  # "serif"
//   for default_name in DEFAULT_FONT_NAMES:
//       for font_family in self.default_font_family.split(","):
//           self.font_name_replacements[default_name] = self.strip_font_name(font_family)
// This ensures that "default" and "$amzn_fixup_default_font$" in KFX font-family lists
// resolve to the book's actual default font (e.g., "serif") instead of being kept as "default".
// registerFontFamilies should be called first so that @font-face names are available
// for proper case resolution.
// defaultFontFamily is the raw $11 value from document data, which may contain font names
// like "akba_9780593537626_epub3_cvi_r1-freefontserif" that need prefix stripping and
// case resolution through registered font names.
func (f *fontNameFixer) setDefaultFontFamily(defaultFontFamily string) {
	if defaultFontFamily == "" {
		defaultFontFamily = "serif"
	}
	// Resolve the raw default font family through fixFontName to get proper case.
	// This handles cases like "akba_9780593537626_epub3_cvi_r1-freefontserif" → "FreeFontSerif"
	// when @font-face has registered the name with proper case.
	resolvedFamily := f.splitAndFixFontFamilyList(defaultFontFamily)
	if len(resolvedFamily) > 0 {
		defaultFontFamily = strings.Join(resolvedFamily, ",")
	}
	// Python: self.font_name_replacements["default"] = DEFAULT_DOCUMENT_FONT_FAMILY
	f.nameReplacements["default"] = "serif"
	// Python: for default_name in DEFAULT_FONT_NAMES:
	//   for font_family in self.default_font_family.split(","):
	//     self.font_name_replacements[default_name] = self.strip_font_name(font_family)
	for _, defaultName := range []string{"default", "$amzn_fixup_default_font$"} {
		for _, fontFamily := range strings.Split(defaultFontFamily, ",") {
			f.nameReplacements[strings.ToLower(defaultName)] = stripFontName(fontFamily)
		}
	}
}

// registerFontFamilies registers @font-face font names with add=true, matching Python's
// process_fonts (yj_to_epub_resources.py) which calls fix_font_name(font["font_family"], add=True).
// This registers each font with its proper case so that subsequent lookups (e.g., when
// resolving "default" from KFX metadata) find the properly-cased name.
// Must be called before setDefaultFontFamily to ensure proper case resolution.
func (f *fontNameFixer) registerFontFamilies(fonts map[string]fontFragment) {
	for _, font := range fonts {
		if font.Family == "" {
			continue
		}
		// Register the raw font name (may have prefix like "akba_...-freefontserif").
		// This handles prefix-stripped names with the "?-" key convention.
		resolved := f.fixFontName(font.Family, true, false)
		// Also ensure the resolved name is registered without the "?-" prefix.
		// This handles lookups of the resolved name directly (e.g., "FreeFontSerif").
		// When the raw name had a prefix, the resolved name was stored with "?-" key,
		// but subsequent lookups use the plain lowercase key.
		if resolved != "" && !cssGenericFontNames[strings.ToLower(resolved)] {
			key := strings.ToLower(resolved)
			if _, ok := f.nameReplacements[key]; !ok {
				f.nameReplacements[key] = resolved
			}
			if _, ok := f.fixedNames[key]; !ok {
				f.fixedNames[key] = resolved
			}
		}
	}
}

// resolvedDefaultFontFamily returns the resolved default font family for use in
// setHTMLDefaults. This is the properly-cased, quoted font family string that
// Python would use for self.default_font_family. For books where the document
// default is just "serif", this returns "serif". For books like Martyr where the
// document default resolves to "FreeFontSerif", this returns "FreeFontSerif,serif".
func (f *fontNameFixer) resolvedDefaultFontFamily() string {
	if replacement, ok := f.nameReplacements["default"]; ok && replacement != "serif" {
		return f.fixAndQuoteFontFamilyList(replacement + ",serif")
	}
	return "serif"
}

func (f *fontNameFixer) fixAndQuoteFontFamilyList(value string) string {
	families := f.splitAndFixFontFamilyList(value)
	if len(families) == 0 {
		return ""
	}
	seen := map[string]bool{}
	quoted := make([]string, 0, len(families))
	for _, family := range families {
		key := strings.ToLower(family)
		if seen[key] {
			continue
		}
		seen[key] = true
		quoted = append(quoted, quoteFontName(family))
	}
	return strings.Join(quoted, ",")
}

func (f *fontNameFixer) splitAndFixFontFamilyList(value string) []string {
	parts := strings.Split(value, ",")
	families := make([]string, 0, len(parts))
	for _, part := range parts {
		if family := f.fixFontName(part, false, false); family != "" {
			families = append(families, family)
		}
	}
	return families
}

func stripFontName(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > 0 && (name[0] == '\'' || name[0] == '"') {
		name = name[1:]
	}
	if len(name) > 0 && (name[len(name)-1] == '\'' || name[len(name)-1] == '"') {
		name = name[:len(name)-1]
	}
	return strings.TrimSpace(name)
}

func (f *fontNameFixer) fixFontName(name string, add bool, generic bool) string {
	name = stripFontName(name)
	if name == "" {
		return ""
	}
	origName := strings.ToLower(name)
	if fixed, ok := f.fixedNames[origName]; ok {
		return fixed
	}
	name = strings.ReplaceAll(name, `\`, "")
	lower := strings.ToLower(name)
	replacements := map[string]string{
		"san-serif": "sans-serif",
		"ariel":     "Arial",
	}
	if replacement, ok := replacements[lower]; ok {
		name = replacement
		lower = strings.ToLower(name)
	}
	for _, suffix := range []string{"-oblique", "-italic", "-bold", "-regular", "-roman", "-medium"} {
		if strings.HasSuffix(lower, suffix) {
			name = name[:len(name)-len(suffix)] + " " + strings.TrimPrefix(suffix, "-")
			break
		}
	}
	hasPrefix := strings.Contains(name, "-") && name != "sans-serif"
	if hasPrefix {
		name = strings.ReplaceAll(name, "sans-serif", "sans_serif")
		name = name[strings.LastIndex(name, "-")+1:]
		name = strings.ReplaceAll(name, "sans_serif", "sans-serif")
	}
	name = strings.TrimSpace(name)
	if add {
		key := strings.ToLower(name)
		if hasPrefix {
			key = "?-" + key
		}
		if replacement, ok := f.nameReplacements[key]; ok {
			name = replacement
		} else {
			f.nameReplacements[key] = name
		}
	} else {
		if replacement, ok := f.nameReplacements[strings.ToLower(name)]; ok {
			name = replacement
		} else if cssGenericFontNames[strings.ToLower(name)] {
			name = strings.ToLower(name)
		} else {
			name = capitalizeFontName(name)
		}
	}
	f.fixedNames[origName] = name
	return name
}

func capitalizeFontName(name string) string {
	words := strings.Fields(name)
	for i, word := range words {
		if len(word) > 2 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		} else {
			words[i] = strings.ToUpper(word)
		}
	}
	return strings.Join(words, " ")
}

func quoteFontName(value string) string {
	for _, ident := range strings.Split(value, " ") {
		if ident == "" {
			break
		}
		first := ident[0]
		if (first >= '0' && first <= '9') || (len(ident) >= 2 && ident[:2] == "--") || !cssIdentPattern.MatchString(ident) {
			return quoteCSSString(value)
		}
		if first == '-' && len(ident) > 1 && ident[1] >= '0' && ident[1] <= '9' {
			return quoteCSSString(value)
		}
	}
	return value
}

func canonicalDeclarations(declarations []string) []string {
	if len(declarations) == 0 {
		return declarations
	}
	out := make([]string, 0, len(declarations))
	seen := map[string]bool{}
	for _, declaration := range declarations {
		declaration = strings.TrimSpace(declaration)
		if declaration == "" || seen[declaration] {
			continue
		}
		seen[declaration] = true
		out = append(out, declaration)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ni := out[i]
		nj := out[j]
		pi := ni
		if idx := strings.IndexByte(ni, ':'); idx >= 0 {
			pi = ni[:idx]
		}
		pj := nj
		if idx := strings.IndexByte(nj, ':'); idx >= 0 {
			pj = nj[:idx]
		}
		if pi == pj {
			return ni < nj
		}
		return pi < pj
	})
	return out
}

func quoteCSSString(value string) string {
	if !strings.Contains(value, "'") && !strings.Contains(value, `\`) {
		return "'" + value + "'"
	}
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}

// processContentProps is a wrapper around processContentProperties that also
// tracks textCombineInUse. Port of Python's self.text_combine_in_use tracking
// in convert_yj_properties (yj_to_epub_properties.py L1127).
// The resolve parameter is kept for API compatibility but uses r.resolveResource internally.
func (r *storylineRenderer) processContentProps(content map[string]interface{}, resolveResource ResourceResolver) map[string]string {
	css, combineInUse := processContentPropertiesWithCombineFlag(content, r.resolveResource)
	if combineInUse {
		r.textCombineInUse = true
	}
	return css
}

// ---------------------------------------------------------------------------
// Merged from storyline.go (origin: yj_to_epub_content.py)
// ---------------------------------------------------------------------------


type storylineRenderer struct {
	contentFragments    map[string][]string
	rubyGroups          map[string]map[string]interface{}
	rubyContents        map[string]map[string]interface{}
	resourceHrefByID    map[string]string
	resourceFragments   map[string]resourceFragment
	anchorToFilename    map[string]string
	directAnchorURI     map[string]string
	fallbackAnchorURI   map[string]string
	positionToSection   map[int]string
	positionAnchors     map[int]map[int][]string
	positionAnchorID    map[int]map[int]string
	anchorNamesByID     map[string][]string
	anchorHeadingLevel  map[string]int
	emittedAnchorIDs    map[string]bool
	styleFragments      map[string]map[string]interface{}
	styles              *styleCatalog
	activeBodyClass     string
	activeBodyDefaults  map[string]bool
	firstVisibleSeen    bool
	lastKFXHeadingLevel int
	symFmt              symType
	conditionEvaluator  conditionEvaluator
	resolveResource     ResourceResolver
	storylines          map[string]map[string]interface{}
	// textCombineInUse is set to true when any text-combine-upright: all
	// declaration is encountered during style processing (Python: self.text_combine_in_use).
	// Port of Python yj_to_epub_properties.py L1127 and yj_to_epub_content.py L103.
	textCombineInUse bool
	// hasConditionalContent is set to true when any conditional page template content
	// is encountered during rendering (Python: self.has_conditional_content).
	// Port of Python yj_to_epub_content.py L508 and yj_to_epub_illustrated_layout.py L27.
	hasConditionalContent bool
	// regionMagnification tracks the region magnification state.
	// Port of Python self.region_magnification (yj_to_epub_content.py L674-694).
	regionMagnification regionMagnificationConfig
	// pendingActivateElements holds activate <a> elements created by processRegionMagnification
	// that need to be attached to container elements.
	pendingActivateElements []htmlElement
	// inPromotedBody is true when rendering content nodes of a promoted body.
	// In Python, promoted body content is rendered inline (no heading/paragraph wrappers)
	// because the promoted style goes on <body> and children are just inline content.
	inPromotedBody bool
}

type conditionEvaluator struct {
	orientationLock   string
	fixedLayout       bool
	illustratedLayout bool
}

func (r *storylineRenderer) renderStoryline(sectionPositionID int, bodyStyleID string, bodyStyleValues map[string]interface{}, storyline map[string]interface{}, nodes []interface{}) renderedStoryline {
	result := renderedStoryline{}
	contentNodes := nodes
	promotedBody := false
	promotedBodyInline := false // true when promoted body is a heading leaf node (render inline)
	inferredBody := false
	if bodyStyleID == "" {
		if promotedStyleID, promotedNodes, ok, inline := promotedBodyContainer(nodes); ok {
			bodyStyleID = promotedStyleID
			bodyStyleValues = nil
			contentNodes = promotedNodes
			promotedBody = true
			promotedBodyInline = inline
			if os.Getenv("KFX_DEBUG_PROMOTE") != "" {
				bs := effectiveStyle(r.styleFragments[promotedStyleID], nil)
				fmt.Fprintf(os.Stderr, "PROMOTED pos=%d styleID=%s hints=%v\n",
					sectionPositionID, promotedStyleID, extractLayoutHintsFromStyle(bs))
			}
		}
	}
	if promotedBody {
		bodyStyleValues = mergeStyleValues(bodyStyleValues, r.inferPromotedBodyStyle(contentNodes))
	}
	if bodyStyleID == "" && len(bodyStyleValues) == 0 {
		bodyStyleValues = r.inferBodyStyleValues(contentNodes, defaultInheritedBodyStyle())
		inferredBody = true
		if len(bodyStyleValues) == 0 {
			bodyStyleValues = map[string]interface{}{
				"font_family": defaultInheritedBodyStyle()["font_family"],
			}
		}
	}
	if os.Getenv("KFX_DEBUG_BODY") != "" {
		fmt.Fprintf(os.Stderr, "body infer styleID=%s values=%#v\n", bodyStyleID, bodyStyleValues)
	}
	r.activeBodyDefaults = nil
	r.firstVisibleSeen = false
	r.lastKFXHeadingLevel = 1
	if bodyStyleID == "" {
		bodyStyleID, _ = asString(storyline["style"])
	}
	bodyStyle := effectiveStyle(r.styleFragments[bodyStyleID], bodyStyleValues)
	// In Python, text-indent is NOT set on the body element during rendering. Instead,
	// it's promoted to the body by reverse inheritance during simplify_styles.
	// Go previously stripped $36 here, but that prevented text-indent from appearing
	// in child classes (the inherited default "0" matched children's "0", so simplify
	// stripped it). Now we keep text-indent in the body style and let simplify_styles'
	// reverse inheritance handle it — matching Python's approach.
	// delete(bodyStyle, "text_indent")
	bodyDeclarations := cssDeclarationsFromMap(r.processContentProps(bodyStyle, r.resolveResource))
	// Extract layout hints from the body style fragment to include in the body's
	// inline style string — but ONLY when the body was promoted from a container.
	// When the body style comes from the page template (not promoted), the layout
	// hints belong on child elements, not the body. Python's add_kfx_style copies
	// layout_hints into whatever element the style is applied to. For promoted bodies,
	// that element IS the body. For template bodies, it's a child container.
	var bodyLayoutHints []string
	if promotedBody {
		bodyLayoutHints = extractLayoutHintsFromStyle(bodyStyle)
	}
	if bodyStyleID == "" && len(bodyDeclarations) == 0 {
		bodyStyleValues = map[string]interface{}{
			"font_family": defaultInheritedBodyStyle()["font_family"],
		}
		bodyStyle = effectiveStyle(r.styleFragments[bodyStyleID], bodyStyleValues)
		bodyDeclarations = cssDeclarationsFromMap(r.processContentProps(bodyStyle, r.resolveResource))
		bodyLayoutHints = extractLayoutHintsFromStyle(bodyStyle)
	}
	if len(bodyDeclarations) > 0 {
		baseName := "class"
		if bodyStyleID != "" {
			baseName = r.styleBaseName(bodyStyleID)
		}
		result.BodyStyle = styleStringFromDeclarations(baseName, bodyLayoutHints, bodyDeclarations)
	}
	if os.Getenv("KFX_DEBUG_BODY") != "" {
		fmt.Fprintf(os.Stderr, "body resolved styleID=%s decls=%v style=%s inferred=%v\n", bodyStyleID, bodyDeclarations, result.BodyStyle, inferredBody)
	}
	result.BodyStyleInferred = inferredBody
	if len(bodyDeclarations) > 0 {
		r.activeBodyDefaults = inheritedDefaultSet(bodyDeclarations)
	}
	bodyParts := make([]htmlPart, 0, len(contentNodes))
	if promotedBodyInline {
		// Python's promoted body content is rendered INLINE for heading leaf nodes.
		// The promoted heading style goes on <body>, and children are just inline
		// elements (<a>, <span>, text). No heading/paragraph wrappers are created.
		r.inPromotedBody = true
		for _, rawNode := range contentNodes {
			node, ok := asMap(rawNode)
			if !ok {
				continue
			}
			node, ok = r.prepareRenderableNode(node)
			if !ok {
				continue
			}

			// For leaf text nodes: extract text content and render inline
			if ref, ok := asMap(node["content"]); ok {
				text := r.resolveText(ref)
				if text != "" {
					content := r.applyAnnotations(text, node)
					for _, c := range content {
						part := r.wrapNodeLink(node, c)
						bodyParts = append(bodyParts, part)
					}
					continue
				}
			}

			// For image nodes: render inline image
			if imageNode := r.renderImageNode(node); imageNode != nil {
				imageNode = r.wrapNodeLink(node, imageNode)
				bodyParts = append(bodyParts, imageNode)
				continue
			}

			// For container nodes: render children inline
			if children, ok := asSlice(node["content_list"]); ok {
				for _, child := range children {
					rendered := r.renderInlinePart(child, 0)
					if rendered != nil {
						childMap, _ := asMap(child)
						rendered = r.wrapNodeLink(childMap, rendered)
						bodyParts = append(bodyParts, rendered)
					}
				}
				continue
			}

			// Fallback: render as inline part
			rendered := r.renderInlinePart(rawNode, 0)
			if rendered != nil {
				rendered = r.wrapNodeLink(node, rendered)
				bodyParts = append(bodyParts, rendered)
			}
		}
		r.inPromotedBody = false
	} else {
		for _, node := range contentNodes {
			rendered := r.renderNode(node, 0)
			if rendered != nil {
				bodyParts = append(bodyParts, rendered)
			}
		}
	}
	root := &htmlElement{Attrs: map[string]string{}, Children: bodyParts}
	normalizeHTMLWhitespace(root)
	r.applyPositionAnchors(root, sectionPositionID, false)
	result.Root = root
	result.BodyHTML = renderHTMLParts(root.Children, true)
	if strings.Contains(result.BodyHTML, "<svg ") {
		result.Properties = "svg"
	}
	return result
}

func (r *storylineRenderer) promoteCommonChildStyles(element *htmlElement) {
	if element == nil {
		return
	}
	for _, child := range element.Children {
		if childElement, ok := child.(*htmlElement); ok {
			r.promoteCommonChildStyles(childElement)
		}
	}
	if element.Tag != "div" {
		return
	}
	baseName, parentStyle, ok := r.dynamicClassStyle(element.Attrs["class"])
	if !ok {
		return
	}
	children := make([]*htmlElement, 0, len(element.Children))
	for _, child := range element.Children {
		childElement, ok := child.(*htmlElement)
		if !ok {
			continue
		}
		children = append(children, childElement)
	}
	if len(children) == 0 {
		return
	}
	const reverseInheritanceFraction = 0.8
	keys := []string{"font-family", "font-style", "font-weight", "font-variant", "text-align", "text-indent", "text-transform"}
	valueCounts := map[string]map[string]int{}
	for _, child := range children {
		_, childStyle, ok := r.dynamicClassStyle(child.Attrs["class"])
		if !ok {
			continue
		}
		for _, key := range keys {
			value := childStyle[key]
			if value == "" {
				continue
			}
			if valueCounts[key] == nil {
				valueCounts[key] = map[string]int{}
			}
			valueCounts[key][value]++
		}
	}
	newHeritable := map[string]string{}
	for _, key := range keys {
		values := valueCounts[key]
		if len(values) == 0 {
			continue
		}
		total := 0
		mostCommonValue := ""
		mostCommonCount := 0
		for value, count := range values {
			total += count
			if count > mostCommonCount {
				mostCommonValue = value
				mostCommonCount = count
			}
		}
		if total < len(children) && parentStyle[key] == "" {
			continue
		}
		if float64(mostCommonCount) >= float64(len(children))*reverseInheritanceFraction {
			newHeritable[key] = mostCommonValue
		}
	}
	if len(newHeritable) == 0 {
		return
	}
	oldParentStyle := cloneStyleMap(parentStyle)
	for _, child := range children {
		childBaseName, childStyle, ok := r.dynamicClassStyle(child.Attrs["class"])
		if !ok {
			continue
		}
		for key, newValue := range newHeritable {
			if childStyle[key] == newValue {
				delete(childStyle, key)
			} else if childStyle[key] == "" && oldParentStyle[key] != "" && oldParentStyle[key] != newValue {
				childStyle[key] = oldParentStyle[key]
			}
		}
		r.setDynamicClassStyle(child, childBaseName, childStyle)
	}
	for key, value := range newHeritable {
		parentStyle[key] = value
	}
	r.setDynamicClassStyle(element, baseName, parentStyle)
}

func (r *storylineRenderer) dynamicClassStyle(className string) (string, map[string]string, bool) {
	if r == nil || className == "" || r.styles == nil {
		return "", nil, false
	}
	entry, ok := r.styles.byToken[className]
	if !ok {
		return "", nil, false
	}
	return entry.baseName, parseDeclarationString(entry.declarations), true
}

func (r *storylineRenderer) styleBaseName(styleID string) string {
	if styleID == "" {
		return "class"
	}
	simplified := uniquePartOfLocalSymbol(styleID, r.symFmt)
	if simplified == "" {
		return "class"
	}
	return "class_" + simplified
}

func (r *storylineRenderer) setDynamicClassStyle(element *htmlElement, baseName string, style map[string]string) {
	if element == nil {
		return
	}
	if len(style) == 0 {
		delete(element.Attrs, "class")
		return
	}
	declarations := declarationListFromStyleMap(style)
	if len(declarations) == 0 {
		delete(element.Attrs, "class")
		return
	}
	element.Attrs["class"] = r.styles.bind(baseName, declarations)
}

func (r *storylineRenderer) setDynamicStyle(element *htmlElement, baseName string, layoutHints []string, declarations []string) {
	if element == nil {
		return
	}
	setElementStyleString(element, mergeStyleStrings(element.Attrs["style"], styleStringFromDeclarations(baseName, layoutHints, declarations)))
}

func (r *storylineRenderer) renderNode(raw interface{}, depth int) htmlPart {
	node, ok := asMap(raw)
	if !ok {
		// IonString entries in $146 lists create text nodes.
		// Python process_content (yj_to_epub_content.py:397-399) wraps them in <span>.
		if text, ok := asString(raw); ok && text != "" {
			return &htmlElement{Tag: "span", Children: []htmlPart{htmlText{Text: cleanTextForLXML(text)}}}
		}
		return nil
	}
	node, ok = r.prepareRenderableNode(node)
	if !ok {
		return nil
	}
	switch asStringDefault(node["type"]) {
	case "list":
		if list := r.renderListNode(node, depth); list != nil {
			return r.wrapNodeLink(node, list)
		}
	case "listitem":
		if item := r.renderListItemNode(node, depth); item != nil {
			return r.wrapNodeLink(node, item)
		}
	case "horizontal_rule":
		if rule := r.renderRuleNode(node); rule != nil {
			return r.wrapNodeLink(node, rule)
		}
	case "zoom_target":
		if hidden := r.renderHiddenNode(node, depth); hidden != nil {
			return r.wrapNodeLink(node, hidden)
		}
	case "table":
		if table := r.renderTableNode(node, depth); table != nil {
			if elem, ok := table.(*htmlElement); ok {
				elem = r.processAnnotations(node, "table", elem)
				return r.wrapNodeLink(node, elem)
			}
			return r.wrapNodeLink(node, table)
		}
	case "container":
		if container := r.renderFittedContainer(node, depth); container != nil {
			if elem, ok := container.(*htmlElement); ok {
				elem = r.processAnnotations(node, "container", elem)
				return r.wrapNodeLink(node, elem)
			}
			return r.wrapNodeLink(node, container)
		}
	case "kvg":
		if svg := r.renderSVGNode(node); svg != nil {
			return r.wrapNodeLink(node, svg)
		}
	case "plugin":
		if plugin := r.renderPluginNode(node); plugin != nil {
			return r.wrapNodeLink(node, plugin)
		}
	case "body":
		if tbody := r.renderStructuredContainer(node, "tbody", depth); tbody != nil {
			return r.wrapNodeLink(node, tbody)
		}
	case "header":
		if thead := r.renderStructuredContainer(node, "thead", depth); thead != nil {
			return r.wrapNodeLink(node, thead)
		}
	case "footer":
		if tfoot := r.renderStructuredContainer(node, "tfoot", depth); tfoot != nil {
			return r.wrapNodeLink(node, tfoot)
		}
	case "table_row":
		if row := r.renderTableRow(node, depth); row != nil {
			return r.wrapNodeLink(node, row)
		}
	}

	if imageNode := r.renderImageNode(node); imageNode != nil {
		return r.wrapNodeLink(node, imageNode)
	}

	if textNode := r.renderTextNode(node, depth); textNode != nil {
		return r.wrapNodeLink(node, textNode)
	}

	children, ok := asSlice(node["content_list"])
	if !ok {
		if hasRenderableContainer(node) {
			element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
			if styleAttr := r.containerClass(node); styleAttr != "" {
				element.Attrs["style"] = styleAttr
			}
			r.applyStructuralNodeAttrs(element, node, "")
			return r.wrapNodeLink(node, element)
		}
		return nil
	}

	if inline := r.renderInlineRenderContainer(node, children, depth); inline != nil {
		return r.wrapNodeLink(node, inline)
	}
	// Python defers heading conversion to simplify_styles (yj_to_epub_properties.py:1922).
	// We create a <div> here and store the heading level as a data attribute.
	// simplify_styles will convert it to <h1>-<h6> after seeing all children.
	hl := r.layoutHintHeadingLevel(node, children)
	if hl > 0 {
		element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
		for _, child := range children {
			if inline := r.renderInlineContent(child, depth+1); inline != nil {
				element.Children = append(element.Children, inline)
			}
		}
		if len(element.Children) > 0 {
			if styleAttr := r.containerClass(node); styleAttr != "" {
				element.Attrs["style"] = styleAttr
			}
			// Store heading level for simplify_styles to read. Python reads this from
			// sty.pop("-kfx-heading-level", self.last_kfx_heading_level) (line 1858).
			element.Attrs["data-kfx-heading-level"] = fmt.Sprintf("%d", hl)
			r.applyStructuralNodeAttrs(element, node, "")
			if positionID, _ := asInt(node["id"]); positionID != 0 {
				// Python parity: image-only heading containers should NOT receive
				// position anchors. In Python, process_position is called on the
				// story's parent (body), not on the heading container. When the
				// heading div contains only images (possibly wrapped in divs),
				// skip anchor emission to match Calibre output.
				if !headingContainsOnlyImages(element) {
					r.applyPositionAnchors(element, positionID, false)
				}
			}
			return r.wrapNodeLink(node, element)
		}
	}
	if figure := r.renderFigureHintContainer(node, children, depth); figure != nil {
		return r.wrapNodeLink(node, figure)
	}
	if paragraph := r.renderInlineParagraphContainer(node, children, depth); paragraph != nil {
		return r.wrapNodeLink(node, paragraph)
	}

	container := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	for _, child := range children {
		rendered := r.renderContentChild(child, depth+1, node)
		if rendered != nil {
			container.Children = append(container.Children, rendered)
		}
	}
	// Python yj_to_epub_content.py:1112+: $142 style events are applied to the
	// content element after all children are added. For containers with both element
	// and text children (like the Elvis logo: <img/> + "FIRST EDITION"), the style events
	// wrap text ranges in styled spans. applyContainerStyleEvents implements this.
	r.applyContainerStyleEvents(node, container)
	if len(container.Children) == 0 {
		return nil
	}
	// Python's COMBINE_NESTED_DIVS: if the container wraps a single image wrapper div
	// (<div><img/></div>), merge them into one div. The image wrapper (from imageClasses)
	// already partitioned properties. containerClass includes properties promoted from
	// children via inferPromotedStyleValues.
	// Python: content_style.update(child_sty, replace=False) — parent keeps its values,
	// child only adds properties not already present. So container overwrites wrapper.
	if wrapper := singleImageWrapperChild(container); wrapper != nil {
		containerStyle := r.containerClass(node)
		wrapperStyle := ""
		if wrapper.Attrs != nil {
			wrapperStyle = wrapper.Attrs["style"]
		}
		// mergeStyleStrings processes in order: first arg's properties can be overwritten
		// by second arg. We want container (parent) to win, so wrapper goes first.
		mergedStyle := mergeStyleStrings(wrapperStyle, containerStyle)
		if mergedStyle != "" {
			wrapper.Attrs["style"] = mergedStyle
		} else {
			delete(wrapper.Attrs, "style")
		}
		r.applyStructuralNodeAttrs(wrapper, node, "")
		if positionID, _ := asInt(node["id"]); positionID != 0 {
			r.applyPositionAnchors(wrapper, positionID, false)
		}
		// Port of Python $683 annotation processing for container content type ($270).
		// Python processes annotations AFTER style events and structural attrs.
		if contentType := asStringDefault(node["type"]); contentType == "container" {
			wrapper = r.processAnnotations(node, contentType, wrapper)
		}
		return r.wrapNodeLink(node, wrapper)
	}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		container.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(container, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(container, positionID, false)
	}
	// Port of Python $683 annotation processing for container content type ($270).
	// Python processes annotations AFTER style events and structural attrs.
	if contentType := asStringDefault(node["type"]); contentType == "container" {
		container = r.processAnnotations(node, contentType, container)
	}
	return r.wrapNodeLink(node, container)
}

// processAnnotations handles $683 (annotations) processing for container and table content types.
// Port of Python yj_to_epub_content.py L871-935: annotation processing after content type rendering.
// Three annotation types are handled:
//   - $690 (mathml): adds MathML desc inside SVG element (container only)
//   - $584 (alt_text): sets aria-label on container element
//   - $749 (alt_content): evaluates condition for table alt content replacement
//
// Returns the (possibly modified or replaced) element.
func (r *storylineRenderer) processAnnotations(node map[string]interface{}, contentType string, element *htmlElement) *htmlElement {
	if element == nil {
		return nil
	}
	annotations, ok := asSlice(node["annotations"])
	if !ok || len(annotations) == 0 {
		return element
	}
	delete(node, "annotations")

	for _, raw := range annotations {
		annotation, ok := asMap(raw)
		if !ok {
			continue
		}
		annotationType, _ := asString(annotation["annotation_type"])
		delete(annotation, "annotation_type")

		switch {
		case annotationType == "mathml" && contentType == "container":
			element = r.processMathMLAnnotation(node, annotation, element)

		case annotationType == "alt_text" && contentType == "container":
			r.processAltTextAnnotation(annotation, element)

		case annotationType == "alt_content" && contentType == "table":
			element = r.processAltContentAnnotation(annotation, element)

		default:
			fmt.Fprintf(os.Stderr, "kfx: warning: content has unknown %s annotation type: %s\n", contentType, annotationType)
		}

		// Port of Python check_empty(annotation, "%s annotation" % self.content_context)
		// Log any remaining keys in the annotation as unexpected.
		for key := range annotation {
			if key == "annotation_type" || key == "content" || key == "include" || key == "alt_content" {
				continue
			}
			fmt.Fprintf(os.Stderr, "kfx: warning: annotation has unexpected key %q\n", key)
		}
	}

	return element
}

// processMathMLAnnotation handles $690 (mathml) annotation for container content type.
// Port of Python yj_to_epub_content.py L875-903.
// RESTORE_MATHML_FROM_ANNOTATION is False by default (matching Python), so we create
// an SVG <desc> element with the MathML text instead of replacing the SVG with MathML.
func (r *storylineRenderer) processMathMLAnnotation(node map[string]interface{}, annotation map[string]interface{}, element *htmlElement) *htmlElement {
	// annotation.pop("$145") → get content text
	contentRef, ok := asMap(annotation["content"])
	if !ok {
		return element
	}
	delete(annotation, "content")
	annotationText := r.resolveText(contentRef)

	// Find SVG child element: content_elem.find(".//%s" % SVG)
	svg := findFirstSVGElement(element)
	if svg != nil {
		// RESTORE_MATHML_FROM_ANNOTATION is False in Python (line 23).
		// Python creates an SVG <desc> element with cleaned MathML text.
		// desc.text = re.sub(" amzn-src-id=\"[0-9]+\"", "", annotation_text)
		cleanText := reAmznSrcID.ReplaceAllString(annotationText, "")

		desc := &htmlElement{
			Tag:  "desc",
			Attrs: map[string]string{"xmlns": "http://www.w3.org/2000/svg"},
		}
		// desc is an SVG element with text content
		desc.Children = []htmlPart{htmlText{Text: cleanText}}

		// svg.insert(0 if svg[0].tag != "title" else 1, desc)
		insertIdx := 0
		if len(svg.Children) > 0 {
			if first, ok := svg.Children[0].(*htmlElement); ok && first.Tag == "title" {
				insertIdx = 1
			}
		}
		if insertIdx <= len(svg.Children) {
			svg.Children = append(svg.Children[:insertIdx], append([]htmlPart{desc}, svg.Children[insertIdx:]...)...)
		} else {
			svg.Children = append(svg.Children, desc)
		}

		// Port of Python yj_to_epub_content.py L899-901:
		//   if ("$56" in content and
		//           self.property_value("$56", copy.deepcopy(content["$56"])) == self.get_style(svg).get("width")):
		//       content.pop("$56")
		// $56 → "width". Remove width property from the node if its resolved CSS value
		// matches the SVG element's current width style attribute.
		if widthVal, ok := node["width"]; ok {
			resolvedWidth := propertyValue("width", widthVal, r.resolveResource)
			if svgStyle, hasStyle := svg.Attrs["style"]; hasStyle && resolvedWidth != "" {
				styleMap := parseDeclarationString(svgStyle)
				if svgWidth := styleMap["width"]; svgWidth == resolvedWidth {
					delete(node, "width")
				}
			}
		}
	} else {
		// Python: log.error("Missing svg for mathml annotation in: %s" % etree.tostring(content_elem))
		fmt.Fprintf(os.Stderr, "kfx: error: missing svg for mathml annotation\n")
	}

	return element
}

// reAmznSrcID matches " amzn-src-id=\"<digits>\"" for cleaning MathML annotation text.
// Port of Python: re.sub(" amzn-src-id=\"[0-9]+\"", "", annotation_text)
var reAmznSrcID = regexp.MustCompile(` amzn-src-id="[0-9]+"`)

// processAltTextAnnotation handles $584 (alt_text) annotation for container content type.
// Port of Python yj_to_epub_content.py L905-908.
// Sets aria-label on the container element if the text is non-empty and not the default.
func (r *storylineRenderer) processAltTextAnnotation(annotation map[string]interface{}, element *htmlElement) {
	// annotation.pop("$145") → get content text
	contentRef, ok := asMap(annotation["content"])
	if !ok {
		return
	}
	delete(annotation, "content")
	annotationText := r.resolveText(contentRef)

	// Python: if annotation_text and annotation_text != "no accessible name found.":
	if annotationText != "" && annotationText != "no accessible name found." {
		if element.Attrs == nil {
			element.Attrs = map[string]string{}
		}
		element.Attrs["aria-label"] = annotationText
	}
}

// processAltContentAnnotation handles $749 (alt_content) annotation for table content type.
// Port of Python yj_to_epub_content.py L910-935.
// Evaluates a condition to determine if alternative table content should be used.
// When condition is true, the table is replaced with the alt content.
// When condition is false, the alt content is processed (side effects only, save_resources=False).
func (r *storylineRenderer) processAltContentAnnotation(annotation map[string]interface{}, element *htmlElement) *htmlElement {
	// Python: alt_content_story = self.get_named_fragment(annotation, ftype="$259", name_symbol="$749")
	// get_named_fragment pops annotation["alt_content"] (the storyline name) and looks up in storylines.
	altContentName, _ := asString(annotation["alt_content"])
	delete(annotation, "alt_content")

	if altContentName == "" || r.storylines == nil {
		return element
	}
	altContentStory, ok := r.storylines[altContentName]
	if !ok {
		fmt.Fprintf(os.Stderr, "kfx: warning: alt_content annotation references missing storyline %q\n", altContentName)
		return element
	}

	// condition = annotation.pop("$592")
	// $592 → "include"
	condition := annotation["include"]
	delete(annotation, "include")

	// Python validates the condition against two expected forms:
	//   ["and", ["not", ["yj.supports", "yj.large_tables"]], ["yj.layout_type", "yj.in_page"]]
	//   ["and", ["not", ["yj.supports", "yj.large_tables"]], ["yj.layout_type", "yj.table_viewer"]]
	if condition != nil && !isValidAltContentCondition(condition) {
		fmt.Fprintf(os.Stderr, "kfx: warning: alt_content contains unexpected include condition: %v\n", condition)
	}

	if r.conditionEvaluator.evaluateBinary(condition) {
		// Condition is true: process alt content and replace the table element
		// Python: alt_content_elem = etree.Element("div")
		//         self.process_story(alt_content_story, alt_content_elem, book_part, writing_mode)
		//         content_elem = alt_content_elem[0]
		altElem := &htmlElement{Tag: "div", Attrs: map[string]string{}}
		r.renderStoryIntoElement(altContentStory, altElem)
		if len(altElem.Children) > 0 {
			if first, ok := altElem.Children[0].(*htmlElement); ok {
				fmt.Fprintf(os.Stderr, "kfx: warning: table alt_content was included\n")
				return first
			}
		}
		fmt.Fprintf(os.Stderr, "kfx: warning: table alt_content was included but produced no content\n")
		return element
	}

	// Condition is false: process story with save_resources=False (side effects only)
	// Python: orig_save_resources = self.save_resources
	//         self.save_resources = False
	//         self.process_story(alt_content_story, etree.Element("div"), book_part, writing_mode)
	//         self.save_resources = orig_save_resources
	// In Go, resource saving is handled differently (resources are collected during rendering).
	// We process the story into a throwaway element to trigger any side effects.
	discard := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	r.renderStoryIntoElement(altContentStory, discard)

	return element
}

// isValidAltContentCondition checks if the condition matches one of the two expected forms
// for $749 alt_content annotations. Port of Python's condition validation (L916-925).
func isValidAltContentCondition(condition interface{}) bool {
	cond, ok := condition.([]interface{})
	if !ok || len(cond) != 3 {
		return false
	}
	// First element must be "and"
	if op, _ := asString(cond[0]); op != "and" {
		return false
	}
	// Second element must be ["not", ["yj.supports", "yj.large_tables"]]
	notArgs, ok := cond[1].([]interface{})
	if !ok || len(notArgs) != 2 {
		return false
	}
	if op, _ := asString(notArgs[0]); op != "not" {
		return false
	}
	supportsArgs, ok := notArgs[1].([]interface{})
	if !ok || len(supportsArgs) != 2 {
		return false
	}
	if op, _ := asString(supportsArgs[0]); op != "yj.supports" {
		return false
	}
	if feat, _ := asString(supportsArgs[1]); feat != "yj.large_tables" {
		return false
	}
	// Third element must be ["yj.layout_type", "yj.in_page"] or ["yj.layout_type", "yj.table_viewer"]
	layoutArgs, ok := cond[2].([]interface{})
	if !ok || len(layoutArgs) != 2 {
		return false
	}
	if op, _ := asString(layoutArgs[0]); op != "yj.layout_type" {
		return false
	}
	feature, _ := asString(layoutArgs[1])
	return feature == "yj.in_page" || feature == "yj.table_viewer"
}

// renderStoryIntoElement renders a storyline's content_list into a parent element.
// Equivalent to Python's process_story which pops story_name, processes position,
// then calls process_content_list.
func (r *storylineRenderer) renderStoryIntoElement(storyline map[string]interface{}, parent *htmlElement) {
	// Python: story.pop("$176") → pop story_name
	delete(storyline, "story_name")

	// Python: self.process_content_list(story.pop("$146", []), parent, ...)
	contentList, _ := asSlice(storyline["content_list"])
	for _, child := range contentList {
		rendered := r.renderContentChild(child, 0)
		if rendered != nil {
			parent.Children = append(parent.Children, rendered)
		}
	}
}

// findFirstSVGElement finds the first <svg> element in the HTML tree.
// Port of Python: content_elem.find(".//%s" % SVG) where SVG is the namespaced svg tag.
func findFirstSVGElement(element *htmlElement) *htmlElement {
	if element == nil {
		return nil
	}
	if element.Tag == "svg" {
		return element
	}
	for _, child := range element.Children {
		if childElem, ok := child.(*htmlElement); ok {
			if found := findFirstSVGElement(childElem); found != nil {
				return found
			}
		}
	}
	return nil
}

func singleImageWrapperChild(container *htmlElement) *htmlElement {
	if len(container.Children) != 1 {
		return nil
	}
	div, ok := container.Children[0].(*htmlElement)
	if !ok || div.Tag != "div" {
		return nil
	}
	if len(div.Children) != 1 {
		return nil
	}
	img, ok := div.Children[0].(*htmlElement)
	if !ok || img.Tag != "img" {
		return nil
	}
	return div
}

func (r *storylineRenderer) renderListNode(node map[string]interface{}, depth int) htmlPart {
	tag := listTagByMarker[asStringDefault(node["list_style"])]
	if tag == "" {
		tag = "ul"
	}
	list := &htmlElement{Tag: tag, Attrs: map[string]string{}}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		list.Attrs["style"] = styleAttr
	}
	if start, ok := asInt(node["list_start_offset"]); ok && start > 0 && tag == "ol" && start != 1 {
		list.Attrs["start"] = strconv.Itoa(start)
	}
	children, _ := asSlice(node["content_list"])
	for _, child := range children {
		if rendered := r.renderNode(child, depth+1); rendered != nil {
			list.Children = append(list.Children, rendered)
		}
	}
	if len(list.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(list, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(list, positionID, false)
	}
	return list
}

func (r *storylineRenderer) renderListItemNode(node map[string]interface{}, depth int) htmlPart {
	item := &htmlElement{Tag: "li", Attrs: map[string]string{}}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		item.Attrs["style"] = styleAttr
	}
	if value, ok := asInt(node["list_start_offset"]); ok && value > 0 {
		item.Attrs["value"] = strconv.Itoa(value)
	}
	if ref, ok := asMap(node["content"]); ok {
		text := r.resolveText(ref)
		if text != "" {
			item.Children = append(item.Children, r.applyAnnotations(text, node)...)
		}
	} else if children, ok := asSlice(node["content_list"]); ok {
		for _, child := range children {
			if rendered := r.renderNode(child, depth+1); rendered != nil {
				item.Children = append(item.Children, rendered)
				continue
			}
			if inline := r.renderInlinePart(child, depth+1); inline != nil {
				item.Children = append(item.Children, inline)
			}
		}
	}
	if len(item.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(item, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(item, positionID, false)
	}
	return item
}

func (r *storylineRenderer) renderRuleNode(node map[string]interface{}) htmlPart {
	rule := &htmlElement{Tag: "hr", Attrs: map[string]string{}}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		rule.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(rule, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(rule, positionID, false)
	}
	return rule
}

func (r *storylineRenderer) renderHiddenNode(node map[string]interface{}, depth int) htmlPart {
	element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		element.Attrs["style"] = styleAttr
	}
	if hiddenStyle := styleStringFromDeclarations("class", nil, []string{"display: none"}); hiddenStyle != "" {
		element.Attrs["style"] = mergeStyleStrings(element.Attrs["style"], hiddenStyle)
	}
	if children, ok := asSlice(node["content_list"]); ok {
		for _, child := range children {
			if rendered := r.renderNode(child, depth+1); rendered != nil {
				element.Children = append(element.Children, rendered)
				continue
			}
			if inline := r.renderInlinePart(child, depth+1); inline != nil {
				element.Children = append(element.Children, inline)
			}
		}
	}
	if len(element.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderFittedContainer(node map[string]interface{}, depth int) htmlPart {
	fitWidth, _ := asBool(node["fit_width"])
	if !fitWidth {
		return nil
	}
	outer := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	if styleAttr := r.fittedContainerClass(node); styleAttr != "" {
		outer.Attrs["style"] = styleAttr
	}
	inner := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	children, _ := asSlice(node["content_list"])
	for _, child := range children {
		rendered := r.renderNode(child, depth+1)
		if rendered != nil {
			inner.Children = append(inner.Children, rendered)
		}
	}
	if len(inner.Children) == 0 {
		return nil
	}
	styleID, _ := asString(node["style"])
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	if styleAttr := styleStringFromDeclarations(baseName, nil, []string{"display: inline-block"}); styleAttr != "" {
		inner.Attrs["style"] = styleAttr
	}
	outer.Children = []htmlPart{inner}
	r.applyStructuralNodeAttrs(outer, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(outer, positionID, false)
	}
	return outer
}

func (r *storylineRenderer) renderPluginNode(node map[string]interface{}) htmlPart {
	resourceID, _ := asString(node["resource_name"])
	if resourceID == "" {
		return nil
	}
	href := r.resourceHrefByID[resourceID]
	if href == "" {
		return nil
	}
	resource := r.resourceFragments[resourceID]
	alt, _ := asString(node["alt_text"])
	switch {
	case resource.MediaType == "plugin/kfx-html-article" || resource.MediaType == "text/html" || resource.MediaType == "application/xhtml+xml":
		element := &htmlElement{
			Tag:   "iframe",
			Attrs: map[string]string{"src": href},
		}
		if styleAttr := styleStringFromDeclarations("class", nil, []string{
			"border-bottom-style: none",
			"border-left-style: none",
			"border-right-style: none",
			"border-top-style: none",
			"height: 100%",
			"width: 100%",
		}); styleAttr != "" {
			element.Attrs["style"] = styleAttr
		}
		r.applyStructuralNodeAttrs(element, node, "")
		return element
	case strings.HasPrefix(resource.MediaType, "audio/"):
		element := &htmlElement{
			Tag:   "audio",
			Attrs: map[string]string{"src": href, "controls": "controls"},
		}
		r.applyStructuralNodeAttrs(element, node, "")
		return element
	case strings.HasPrefix(resource.MediaType, "video/"):
		element := &htmlElement{
			Tag:   "video",
			Attrs: map[string]string{"src": href, "controls": "controls"},
		}
		if alt != "" {
			element.Attrs["aria-label"] = alt
		}
		r.applyStructuralNodeAttrs(element, node, "")
		return element
	case strings.HasPrefix(resource.MediaType, "image/"):
		return r.renderImageNode(node)
	default:
		element := &htmlElement{
			Tag:   "object",
			Attrs: map[string]string{"data": href},
		}
		if resource.MediaType != "" {
			element.Attrs["type"] = resource.MediaType
		}
		if alt != "" {
			element.Children = []htmlPart{htmlText{Text: alt}}
		}
		r.applyStructuralNodeAttrs(element, node, "")
		return element
	}
}

func (r *storylineRenderer) renderSVGNode(node map[string]interface{}) htmlPart {
	width, hasWidth := asInt(node["fixed_width"])
	height, hasHeight := asInt(node["fixed_height"])
	attrs := map[string]string{
		"version":             "1.1",
		"preserveAspectRatio": "xMidYMid meet",
	}
	if hasWidth && hasHeight && width > 0 && height > 0 {
		attrs["viewBox"] = fmt.Sprintf("0 0 %d %d", width, height)
	}
	element := &htmlElement{
		Tag:   "svg",
		Attrs: attrs,
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}

	// Python yj_to_epub_content.py L856-859:
	//   content_list = content.pop("$146", [])
	//   for shape in content.pop("$250", []):
	//       self.process_kvg_shape(content_elem, shape, content_list, book_part, writing_mode)
	// $146 = content_list, $250 = shape_list.
	contentList, _ := asSlice(node["content_list"])
	if shapeList, ok := asSlice(node["shape_list"]); ok {
		for _, shape := range shapeList {
			if shapeMap, ok := asMap(shape); ok {
				r.processKVGShape(element, shapeMap, &contentList, "")
			}
		}
	}

	return element
}

func (r *storylineRenderer) renderTableNode(node map[string]interface{}, depth int) htmlPart {
	table := &htmlElement{Tag: "table", Attrs: map[string]string{}}
	if styleAttr := r.tableClass(node); styleAttr != "" {
		table.Attrs["style"] = styleAttr
	}
	if cols, ok := asSlice(node["column_format"]); ok && len(cols) > 0 {
		colgroup := &htmlElement{Tag: "colgroup", Attrs: map[string]string{}}
		for _, raw := range cols {
			colMap, ok := asMap(raw)
			if !ok {
				continue
			}
			col := &htmlElement{Tag: "col", Attrs: map[string]string{}}
			if span, ok := asInt(colMap["column_span"]); ok && span > 1 {
				col.Attrs["span"] = strconv.Itoa(span)
			}
			if styleAttr := r.tableColumnClass(colMap); styleAttr != "" {
				col.Attrs["style"] = styleAttr
			}
			colgroup.Children = append(colgroup.Children, col)
		}
		if len(colgroup.Children) > 0 {
			table.Children = append(table.Children, colgroup)
		}
	}
	if children, ok := asSlice(node["content_list"]); ok {
		for _, child := range children {
			rendered := r.renderNode(child, depth+1)
			if rendered != nil {
				if childNode, ok := asMap(child); ok {
					r.applyStructuralAttrsToPart(rendered, childNode, table.Tag)
				}
				table.Children = append(table.Children, rendered)
			}
		}
	}
	if len(table.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(table, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(table, positionID, false)
	}
	return table
}

func (r *storylineRenderer) renderStructuredContainer(node map[string]interface{}, tag string, depth int) htmlPart {
	element := &htmlElement{Tag: tag, Attrs: map[string]string{}}
	if styleAttr := r.structuredContainerClass(node); styleAttr != "" {
		element.Attrs["style"] = styleAttr
	}
	if children, ok := asSlice(node["content_list"]); ok {
		for _, child := range children {
			rendered := r.renderNode(child, depth+1)
			if rendered != nil {
				element.Children = append(element.Children, rendered)
			}
		}
	}
	if len(element.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderTableRow(node map[string]interface{}, depth int) htmlPart {
	row := &htmlElement{Tag: "tr", Attrs: map[string]string{}}
	if styleID, _ := asString(node["style"]); styleID != "" {
		if styleAttr := r.structuredContainerClass(node); styleAttr != "" {
			row.Attrs["style"] = styleAttr
		}
	}
	children, _ := asSlice(node["content_list"])
	for _, child := range children {
		cellNode, ok := asMap(child)
		if !ok {
			// Port of Python table_row child promotion (yj_to_epub_content.py L811-826):
			// Bare strings or unexpected types in content_list get wrapped in <td>.
			// Python: for idx, child_elem in enumerate(list(content_elem)):
			//   if child_elem.tag == "div": child_elem.tag = "td"
			//   elif child_elem.tag != "td": wrap in <td>
			if text, ok := asString(child); ok && text != "" {
				cell := &htmlElement{Tag: "td", Children: []htmlPart{htmlText{Text: text}}}
				row.Children = append(row.Children, cell)
			}
			continue
		}
		cell := r.renderTableCell(cellNode, depth+1)
		if cell != nil {
			row.Children = append(row.Children, cell)
		}
	}
	if len(row.Children) == 0 {
		return nil
	}
	r.applyStructuralNodeAttrs(row, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(row, positionID, false)
	}
	return row
}

func (r *storylineRenderer) renderTableCell(node map[string]interface{}, depth int) htmlPart {
	cell := &htmlElement{Tag: "td", Attrs: map[string]string{}}
	// Extract colspan/rowspan.
	// In KFX data, these can be either:
	// 1. Directly on the node as $148/$149 (some fixtures)
	// 2. In the style fragment as property $148/$149, converted to -kfx-attrib-colspan/rowspan
	// Python extracts them via -kfx-attrib-* partition in fixup_styles_and_classes.
	// Go's cssDeclarationsFromMap strips -kfx- prefixed properties, so we also extract
	// from the style fragment here.
	colspanSet := false
	rowspanSet := false
	if colspan, ok := asInt(node["table_column_span"]); ok && colspan > 1 {
		cell.Attrs["colspan"] = strconv.Itoa(colspan)
		colspanSet = true
	}
	if rowspan, ok := asInt(node["table_row_span"]); ok && rowspan > 1 {
		cell.Attrs["rowspan"] = strconv.Itoa(rowspan)
		rowspanSet = true
	}
	if !colspanSet || !rowspanSet {
		if styleID, _ := asString(node["style"]); styleID != "" {
			effective := effectiveStyle(r.styleFragments[styleID], node)
			cssMap := r.processContentProps(effective, r.resolveResource)
			if !colspanSet {
				if v := cssMap["-kfx-attrib-colspan"]; v != "" {
					if n, err := strconv.Atoi(v); err == nil && n > 1 {
						cell.Attrs["colspan"] = v
					}
				}
			}
			if !rowspanSet {
				if v := cssMap["-kfx-attrib-rowspan"]; v != "" {
					if n, err := strconv.Atoi(v); err == nil && n > 1 {
						cell.Attrs["rowspan"] = v
					}
			}
			}
		}
	}

	// Python COMBINE_NESTED_DIVS check: when the cell $269 has a single $269 child with $145
	// text, and the parent and child CSS have no overlapping properties, the nested divs
	// merge into one. The merged result becomes <td>text</td> after retag + span strip.
	// When there IS overlap, no merge happens: the child $269 keeps its own <div> which
	// simplify_styles later promotes to <p>, giving <td><p class="child">text</p></td>.
	merged := r.tableCellCombineNestedDivs(node)

	if styleAttr := r.tableCellClass(node); styleAttr != "" {
		cell.Attrs["style"] = styleAttr
	}
	if ref, ok := asMap(node["content"]); ok {
		text := r.resolveText(ref)
		if text != "" {
			cell.Children = append(cell.Children, r.applyAnnotations(text, node)...)
		}
	} else if children, ok := asSlice(node["content_list"]); ok {
		for _, child := range children {
			childNode, ok := asMap(child)
			if !ok {
				// Python process_content IonString case (line 397-399):
				// bare string → <span>text</span>
				if text, ok := asString(child); ok {
					s := &htmlElement{Tag: "span", Children: []htmlPart{htmlText{Text: text}}}
					cell.Children = append(cell.Children, s)
				}
				continue
			}
			if ref, ok := asMap(childNode["content"]); ok && merged {
				// COMBINE_NESTED_DIVS merged: extract text directly into <td>.
				// Python: merge removes inner div, leaving <span>text</span> which
				// epub_output.py later strips, giving bare text in <td>.
				text := r.resolveText(ref)
				if text != "" {
					cell.Children = append(cell.Children, r.applyAnnotations(text, childNode)...)
				}
				// When the merged child has its own position ID with anchors,
				// promote those anchors to the <td>. In Python, these anchors
				// end up on the <td> after simplify_styles and beautify_html.
				if childPosID, _ := asInt(childNode["id"]); childPosID != 0 {
					if len(r.positionAnchors[childPosID]) > 0 {
						r.applyPositionAnchors(cell, childPosID, false)
					}
				}
				continue
			}
			// No merge: render child as full node. For a $269 with $145,
			// renderTextNode produces <p class="child">text</p>, matching Python's
			// simplify_styles div→p promotion.
			if rendered := r.renderContentChild(child, depth+1); rendered != nil {
				cell.Children = append(cell.Children, rendered)
			}
		}
	}
	// Python's process_content for $279 (table_row) renames each child <div> to <td>
	// directly (yj_to_epub_content.py:813). Go creates a separate <td> and renders
	// the child inside, producing <td><div>...</div></td>. Unwrap single-child <div>
	// wrappers inside the cell. Keep the cell's own style (from tableCellClass) and
	// add child-only properties (matching Python's content_style.update(child_sty, replace=False)).
	if len(cell.Children) == 1 {
		if div, ok := cell.Children[0].(*htmlElement); ok && div.Tag == "div" {
			// Only unwrap if the <div> has only id/style attrs
			divHasOnlySafeAttrs := true
			for k := range div.Attrs {
				if k != "id" && k != "style" {
					divHasOnlySafeAttrs = false
					break
				}
			}
			if divHasOnlySafeAttrs {
				// Merge child's style properties that the cell doesn't already have
				// (matching Python's content_style.update(child_sty, replace=False))
				if divStyle := div.Attrs["style"]; divStyle != "" {
					cellProps := parseDeclarationString(cell.Attrs["style"])
					childProps := parseDeclarationString(divStyle)
					for k, v := range childProps {
						if _, exists := cellProps[k]; !exists {
							cellProps[k] = v
						}
					}
					if len(cellProps) > 0 {
						cell.Attrs["style"] = styleStringFromMap(cellProps)
					}
				}
				if divID, ok := div.Attrs["id"]; ok && cell.Attrs["id"] == "" {
					cell.Attrs["id"] = divID
				}
				cell.Children = div.Children
			}
		}
	}

	r.applyStructuralNodeAttrs(cell, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(cell, positionID, false)
	}
	return cell
}

// renderInlineContent dispatches a $146 list child the same way Python's process_content
// handles IonString vs IonStruct (yj_to_epub_content.py:395-405).
// - IonString (Go string): creates <span>text</span>  (Python line 397-399)
// - IonStruct (Go map): delegates to renderInlinePart
// applyContainerStyleEvents applies $142 style events to a container element's children,
// matching Python's post-add_content style event processing (yj_to_epub_content.py:1112+).
// Python's find_or_create_style_event_element locates text at character offsets within
// the content element and wraps ranges in styled spans. This implementation handles the
// common case where text children (from IonString $146 items) need annotation wrapping.
func (r *storylineRenderer) applyContainerStyleEvents(node map[string]interface{}, container *htmlElement) {
	annotations, ok := asSlice(node["style_events"])
	if !ok || len(annotations) == 0 {
		return
	}
	// Build text offset map
	type textRange struct {
		childIdx    int
		startOffset int
		length      int
	}
	var ranges []textRange
	offset := 0
	for i, child := range container.Children {
		elem, ok := child.(*htmlElement)
		if !ok {
			continue
		}
		switch elem.Tag {
		case "img", "svg":
			ranges = append(ranges, textRange{childIdx: i, startOffset: offset, length: 1})
			offset++
		case "span":
			text := htmlElementText(elem, r.textCombineInUse)
			ranges = append(ranges, textRange{childIdx: i, startOffset: offset, length: len([]rune(text))})
			offset += len([]rune(text))
		}
	}
	// Apply each annotation to the matching text range
	for _, ann := range annotations {
		annMap, ok := asMap(ann)
		if !ok {
			continue
		}
		eventStart, _ := asInt(annMap["offset"])
		eventLen, _ := asInt(annMap["length"])
		if eventLen <= 0 {
			continue
		}
		anchorID, _ := asString(annMap["link_to"])
		styleID, _ := asString(annMap["style"])

		// Phase 1: Handle non-span children (img, svg, div wrappers with img children) for $179 link wrapping.
		// Python: if "link_to" in style_event: event_elem = replace_element_with_container(event_elem, "a")
		// Python's locate_offset traverses the element tree and can find <img> nested inside
		// wrapper <div>s. Each img/svg counts as 1 character in the offset map.
		if anchorID != "" {
			href := r.anchorHref(anchorID)
			if href != "" {
				// Compute link CSS from the annotation's style fragment.
				// Python: self.add_style(event_elem, self.process_content_properties(style_event), replace=True)
				// This gives the <a> ALL the annotation's CSS (e.g., text-decoration: underline).
				// Note: Unlike content-level link_to (which uses create_container + LINK_CONTAINER_PROPERTIES
				// to PARTITION properties), the style-event path adds ALL properties to the <a>.
				var linkStyleAttr string
				if styleID != "" {
					style := effectiveStyle(r.styleFragments[styleID], annMap)
					linkStyle := r.processContentProps(style, r.resolveResource)
					delete(linkStyle, "-kfx-layout-hints")
					if len(linkStyle) > 0 {
						baseName := r.styleBaseName(styleID)
						linkStyleAttr = styleStringFromDeclarations(baseName, nil, cssDeclarationsFromMap(linkStyle))
					}
				}

				for _, tr := range ranges {
					elem := container.Children[tr.childIdx].(*htmlElement)
					if elem.Tag == "span" {
						continue
					}
					annEnd := eventStart + eventLen
					trEnd := tr.startOffset + tr.length
					if eventStart >= trEnd || annEnd <= tr.startOffset {
						continue
					}
					// Find the actual target element.
					// If the direct child is a <div> wrapping an <img>, Python's locate_offset
					// would traverse into the div and find the <img> at the same offset.
					target := elem
					if elem.Tag == "div" {
						if img := findFirstDescendantByTag(elem, "img"); img != nil {
							target = img
						} else if svg := findFirstDescendantByTag(elem, "svg"); svg != nil {
							target = svg
						}
					}
					if target != elem {
						// Target is nested: find target's parent and replace target with <a><target/></a>.
						var findParent func(*htmlElement) *htmlElement
						findParent = func(e *htmlElement) *htmlElement {
							for _, c := range e.Children {
								if ch, ok := c.(*htmlElement); ok {
									if ch == target {
										return e
									}
									if p := findParent(ch); p != nil {
										return p
									}
								}
							}
							return nil
						}
						if p := findParent(elem); p != nil {
							wrapChildInLink(p, target, href, linkStyleAttr)
						}
					} else {
						linkAttrs := map[string]string{"href": href}
						if epubType := epubTypeFromAnnotation(annMap); epubType != "" {
							linkAttrs["epub:type"] = epubType
						}
						if linkStyleAttr != "" {
							linkAttrs["style"] = linkStyleAttr
						}
						container.Children[tr.childIdx] = &htmlElement{
							Tag:      "a",
							Attrs:    linkAttrs,
							Children: []htmlPart{elem},
						}
					}
					break
				}
			}
		}

		// Phase 2: Handle span children for style application.
		if styleID == "" {
			continue
		}
		// Find which text range(s) this annotation covers
		for _, tr := range ranges {
			elem := container.Children[tr.childIdx].(*htmlElement)
			if elem.Tag != "span" {
				continue
			}
			text := htmlElementText(elem, r.textCombineInUse)
			runes := []rune(text)
			trEnd := tr.startOffset + tr.length
			annEnd := eventStart + eventLen

			// Check overlap
			if eventStart >= trEnd || annEnd <= tr.startOffset {
				continue
			}

			// Compute local offset within this span's text
			localStart := eventStart - tr.startOffset
			if localStart < 0 {
				localStart = 0
			}
			localEnd := annEnd - tr.startOffset
			if localEnd > len(runes) {
				localEnd = len(runes)
			}

			if localStart >= localEnd {
				continue
			}

			// Get the annotation style
			style := effectiveStyle(r.styleFragments[styleID], annMap)
			cssMap := r.processContentProps(style, r.resolveResource)
			declarations := cssDeclarationsFromMap(cssMap)
			if len(declarations) == 0 {
				continue
			}
			baseName := "class"
			if styleID != "" {
				baseName = r.styleBaseName(styleID)
			}
			styledSpanClass := styleStringFromDeclarations(baseName, nil, declarations)

			// Split the span: before + styled + after
			before := string(runes[:localStart])
			styled := string(runes[localStart:localEnd])
			after := string(runes[localEnd:])

			var newChildren []htmlPart
			if before != "" {
				newChildren = append(newChildren, &htmlElement{Tag: "span", Children: []htmlPart{htmlText{Text: before}}})
			}
			styledSpan := &htmlElement{
				Tag:      "span",
				Attrs:    map[string]string{"style": styledSpanClass},
				Children: []htmlPart{htmlText{Text: styled}},
			}

			// Handle $179 (link) wrapping for text spans.
			// Python: if "link_to" in style_event: event_elem.tag = "a"; event_elem.set("href", ...)
			if anchorID, ok := asString(annMap["link_to"]); ok && anchorID != "" {
				if href := r.anchorHref(anchorID); href != "" {
					linkAttrs := map[string]string{"href": href}
					if epubType := epubTypeFromAnnotation(annMap); epubType != "" {
						linkAttrs["epub:type"] = epubType
					}
					styledSpan = &htmlElement{
						Tag:      "a",
						Attrs:    linkAttrs,
						Children: []htmlPart{styledSpan},
					}
				}
			}
			newChildren = append(newChildren, styledSpan)
			if after != "" {
				newChildren = append(newChildren, &htmlElement{Tag: "span", Children: []htmlPart{htmlText{Text: after}}})
			}

			// Replace the single span child with the split children
			container.Children = append(container.Children[:tr.childIdx], append(newChildren, container.Children[tr.childIdx+1:]...)...)
			break // annotation applied, move to next
		}
	}
}

// findFirstDescendantByTag finds the first descendant element with the given tag.
func findFirstDescendantByTag(elem *htmlElement, tag string) *htmlElement {
	for _, child := range elem.Children {
		if ch, ok := child.(*htmlElement); ok {
			if ch.Tag == tag {
				return ch
			}
			if found := findFirstDescendantByTag(ch, tag); found != nil {
				return found
			}
		}
	}
	return nil
}

// headingContainsOnlyImages returns true if the heading element contains only images
// (possibly wrapped in <div> containers). These heading divs should NOT receive
// position anchors — matching Python's behavior where process_position runs on the
// story's parent (body), not the heading container, and the image content processes
// separately through process_content.
func headingContainsOnlyImages(elem *htmlElement) bool {
	if len(elem.Children) == 0 {
		return false
	}
	for _, c := range elem.Children {
		e, ok := c.(*htmlElement)
		if !ok {
			return false // text node
		}
		if e.Tag == "img" || e.Tag == "svg" {
			continue
		}
		if e.Tag == "div" {
			// Check if div only contains images
			if !headingContainsOnlyImages(e) {
				return false
			}
			continue
		}
		return false // non-image element (span, p, etc)
	}
	return true
}

// wrapChildInLink replaces a child element inside its parent's children list
// with <a href="..."><child/></a>.
func wrapChildInLink(parent *htmlElement, target *htmlElement, href string, linkStyleAttr string) {
	for i, child := range parent.Children {
		if ch, ok := child.(*htmlElement); ok && ch == target {
			attrs := map[string]string{"href": href}
			if linkStyleAttr != "" {
				attrs["style"] = linkStyleAttr
			}
			parent.Children[i] = &htmlElement{
				Tag:      "a",
				Attrs:    attrs,
				Children: []htmlPart{target},
			}
			return
		}
	}
}

// htmlElementText extracts the combined text content of an htmlElement by recursively
// descending into child elements, matching Python's combined_text (yj_to_epub_content.py L1534-1554).
// When textCombineInUse is true and the element has text-combine-upright: all,
// returns " " (single space) matching Python behavior for CJK vertical text.
// Port of Python combined_text:
//
//	if elem.tag in {"img", SVG, MATH}: return " "
//	if self.text_combine_in_use and self.get_style(elem).get("text-combine-upright") == "all": return " "
//	texts = []
//	if elem.text: texts.append(elem.text)
//	for e in elem.iterfind("*"): texts.append(self.combined_text(e))
//	if elem.tail: texts.append(elem.tail)
//	return "".join(texts)
func htmlElementText(elem *htmlElement, textCombineInUse bool) string {
	// Python L1536-1537: if elem.tag in {"img", SVG, MATH}: return " "
	// In Go, SVG and MATH tags are lowercase without namespace prefix.
	switch elem.Tag {
	case "img", "svg", "math":
		return " "
	}

	// Python L1539-1540: if self.text_combine_in_use and self.get_style(elem).get("text-combine-upright") == "all": return " "
	if textCombineInUse {
		style := parseDeclarationString(elem.Attrs["style"])
		if style["text-combine-upright"] == "all" {
			return " "
		}
	}

	// Python L1542-1551:
	//   texts = []
	//   if elem.text: texts.append(elem.text)
	//   for e in elem.iterfind("*"): texts.append(self.combined_text(e))  // recursive descent
	//   if elem.tail: texts.append(elem.tail)
	//   return "".join(texts)
	//
	// In Go's HTML model, elem.Text/tail and child elements are all in the Children slice:
	//   - htmlText parts represent both elem.text and child element tails
	//   - *htmlElement parts are child elements that need recursive descent
	var buf strings.Builder
	for _, child := range elem.Children {
		switch typed := child.(type) {
		case htmlText:
			buf.WriteString(typed.Text)
		case *htmlText:
			buf.WriteString(typed.Text)
		case *htmlElement:
			// Python L1547: texts.append(self.combined_text(e))
			buf.WriteString(htmlElementText(typed, textCombineInUse))
		}
	}
	return buf.String()
}

func (r *storylineRenderer) renderInlineContent(child interface{}, depth int) htmlPart {
	if text, ok := asString(child); ok {
		return &htmlElement{Tag: "span", Children: []htmlPart{htmlText{Text: text}}}
	}
	return r.renderInlinePart(child, depth)
}

// renderContentChild dispatches a $146 list child the same way Python's process_content
// handles IonString vs IonStruct for block-level container paths.
// - IonString (Go string): creates <span>text</span> with parent annotations applied
// - IonStruct (Go map): delegates to renderNode, falls back to renderInlinePart
func (r *storylineRenderer) renderContentChild(child interface{}, depth int, parentNode ...map[string]interface{}) htmlPart {
	if text, ok := asString(child); ok {
		return &htmlElement{Tag: "span", Children: []htmlPart{htmlText{Text: text}}}
	}
	if rendered := r.renderNode(child, depth); rendered != nil {
		return rendered
	}
	return r.renderInlinePart(child, depth)
}

func (r *storylineRenderer) renderInlinePart(raw interface{}, depth int) htmlPart {
	node, ok := asMap(raw)
	if !ok {
		return nil
	}
	node, ok = r.prepareRenderableNode(node)
	if !ok {
		return nil
	}
	if imageNode := r.renderImageNode(node); imageNode != nil {
		return imageNode
	}
	if ref, ok := asMap(node["content"]); ok {
		text := r.resolveText(ref)
		if text == "" {
			return nil
		}
		// Check if this text node should be a heading. In Python, process_content
		// always creates <div> for $269 content, and simplify_styles converts to <h>
		// based on layout hints. Go's renderInlinePart creates <span>, which can't
		// be converted to heading. Delegate to renderTextNode for heading nodes.
		positionID, _ := asInt(node["id"])
		styleID, _ := asString(node["style"])
		level := headingLevel(node)
		if level == 0 {
			level = r.headingLevelForPosition(positionID, 0)
		}
		isHeading := layoutHintsInclude(r.nodeLayoutHints(node), "heading")
		if level > 0 && isHeading && !r.inPromotedBody {
			// This node has heading properties. Delegate to renderTextNode
			// which will create the correct <h1>-<h6> element.
			// Skip when inPromotedBody: for promoted bodies, the heading style
			// goes on <body> and content stays inline.
			return r.renderTextNode(node, depth)
		}
		content := r.applyAnnotations(text, node)
		if styleID == "" && positionID == 0 && len(content) == 1 {
			return content[0]
		}
		element := &htmlElement{Tag: "div", Attrs: map[string]string{}, Children: content}
		if styleAttr := r.spanClass(styleID); styleAttr != "" {
			element.Attrs["style"] = styleAttr
		}
		r.applyStructuralNodeAttrs(element, node, "")
		if positionID != 0 {
			r.applyPositionAnchors(element, positionID, false)
		}
		return element
	}
	children, ok := asSlice(node["content_list"])
	if !ok {
		return nil
	}
	styleID, _ := asString(node["style"])
	container := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	for _, child := range children {
		if rendered := r.renderInlinePart(child, depth+1); rendered != nil {
			container.Children = append(container.Children, rendered)
		}
	}
	if len(container.Children) == 0 {
		return nil
	}
	if styleAttr := r.inlineContainerClass(styleID, node); styleAttr != "" {
		container.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(container, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(container, positionID, false)
	}
	return container
}

func (r *storylineRenderer) renderImageNode(node map[string]interface{}) htmlPart {
	node, ok := r.prepareRenderableNode(node)
	if !ok {
		return nil
	}
	resourceID, _ := asString(node["resource_name"])
	if resourceID == "" {
		return nil
	}
	href := r.resourceHrefByID[resourceID]
	if href == "" {
		return nil
	}
	alt, _ := asString(node["alt_text"])
	image := &htmlElement{
		Tag:   "img",
		Attrs: map[string]string{"src": href, "alt": alt},
	}
	wrapperClass, imageClass := r.imageClasses(node)
	if imageClass != "" {
		image.Attrs["style"] = imageClass
	}
	// Python process_content $283 (inline render) for <img> (yj_to_epub_content.py:1295-1298):
	// render=="inline" adds -kfx-render:inline to style but does NOT create a container wrapper.
	// The wrapper div is only created in the else branch (non-inline render, line 1324-1330).
	// Without this check, inline images get a wrapper <div> which causes containsBlock=true
	// in simplify_styles, preventing <div>→<p> promotion for containers with mixed image+text.
	renderMode, _ := asString(node["render"])
	isInlineRender := renderMode == "inline"
	if wrapperClass == "" || isInlineRender {
		firstVisible := r.consumeVisibleElement()
		r.applyStructuralNodeAttrs(image, node, "")
		if positionID, _ := asInt(node["id"]); positionID != 0 {
			r.applyPositionAnchors(image, positionID, firstVisible)
		}
		return image
	}
	wrapper := &htmlElement{
		Tag:      "div",
		Attrs:    map[string]string{"style": wrapperClass},
		Children: []htmlPart{image},
	}
	r.applyStructuralNodeAttrs(wrapper, node, "")
	firstVisible := r.consumeVisibleElement()
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(wrapper, positionID, firstVisible)
	}
	return wrapper
}

func (r *storylineRenderer) renderTextNode(node map[string]interface{}, depth int) htmlPart {
	_ = depth
	var ok bool
	node, ok = r.prepareRenderableNode(node)
	if !ok {
		return nil
	}
	ref, ok := asMap(node["content"])
	if !ok {
		return nil
	}
	text := r.resolveText(ref)
	if text == "" {
		return nil
	}
	positionID, _ := asInt(node["id"])
	if os.Getenv("KFX_DEBUG") != "" && (positionID == 1110 || positionID == 1111 || positionID == 1177 || positionID == 1178) {
		fmt.Fprintf(os.Stderr, "render text pos=%d text=%q style=%s\n", positionID, text[:minInt(len(text), 32)], asStringDefault(node["style"]))
	}
	content := r.applyAnnotations(text, node)
	annotationStyleID := fullParagraphAnnotationStyleID(node, text)

	styleID, _ := asString(node["style"])
	level := headingLevel(node)
	if level == 0 {
		level = r.headingLevelForPosition(positionID, 0)
	}
	// Port of Python simplify_styles heading tag selection (yj_to_epub_properties.py ~L1928):
	// Only promote to <h1>-<h6> when layout hints include "heading" AND not fixed/illustrated layout.
	// Python: "heading" in kfx_layout_hints and not contains_block_elem → elem.tag = "h" + level
	isHeading := layoutHintsInclude(r.nodeLayoutHints(node), "heading")
	if level > 0 {
		r.lastKFXHeadingLevel = level
		if !isHeading {
			// Heading level stored in CSS ($790) but layout hints don't confirm heading;
			// render as <p> like Python does (simplify_styles won't promote this <div>).
			level = 0
		}
	} else if isHeading {
		level = r.lastKFXHeadingLevel
	}
	if level > 0 {
		firstVisible := r.consumeVisibleElement()
		element := &htmlElement{
			Tag:      fmt.Sprintf("h%d", level),
			Attrs:    map[string]string{},
			Children: content,
		}
		if styleID != "" {
			if styleAttr := r.headingClass(styleID); styleAttr != "" {
				element.Attrs["style"] = styleAttr
			}
		}
		r.applyStructuralNodeAttrs(element, node, "")
		r.applyPositionAnchors(element, positionID, firstVisible)
		return element
	}

	firstVisible := r.consumeVisibleElement()
	element := &htmlElement{
		Tag:      "p",
		Attrs:    map[string]string{},
		Children: content,
	}
	if styleID != "" {
		if styleAttr := r.paragraphClass(styleID, annotationStyleID); styleAttr != "" {
			element.Attrs["style"] = styleAttr
		}
	}
	r.applyFirstLineStyle(element, node)
	r.applyStructuralNodeAttrs(element, node, "")
	r.applyPositionAnchors(element, positionID, firstVisible)
	return element
}

func removeSingleFullTextLinkClass(parts []htmlPart) {
	if len(parts) != 1 {
		return
	}
	link, ok := parts[0].(*htmlElement)
	if !ok || link == nil || link.Tag != "a" {
		return
	}
	delete(link.Attrs, "class")
	delete(link.Attrs, "style")
}

func (r *storylineRenderer) applyStructuralAttrsToPart(part htmlPart, node map[string]interface{}, parentTag string) {
	element, ok := part.(*htmlElement)
	if !ok {
		return
	}
	r.applyStructuralNodeAttrs(element, node, parentTag)
}

func (r *storylineRenderer) applyFirstLineStyle(element *htmlElement, node map[string]interface{}) {
	if r == nil || element == nil || node == nil {
		return
	}
	raw, ok := asMap(node["yj.first_line_style"])
	if !ok {
		return
	}
	style := cloneMap(raw)
	if styleID, _ := asString(style["style_name"]); styleID != "" {
		style = effectiveStyle(r.styleFragments[styleID], style)
	}
	delete(style, "style_name")
	delete(style, "yj.first_line_style_type")
	declarations := cssDeclarationsFromMap(r.processContentProps(style, r.resolveResource))
	if len(declarations) == 0 {
		return
	}
	className := r.styles.reserveClass("kfx-firstline")
	if className == "" {
		return
	}
	element.Attrs["class"] = appendClassNames(element.Attrs["class"], className)
	r.styles.addStatic("."+className+"::first-line", declarations)
}

func (r *storylineRenderer) wrapNodeLink(node map[string]interface{}, part htmlPart) htmlPart {
	if node == nil || part == nil {
		return part
	}
	anchorID, _ := asString(node["link_to"])
	if anchorID == "" {
		return part
	}
	href := r.anchorHref(anchorID)
	if href == "" {
		return part
	}
	if element, ok := part.(*htmlElement); ok && element != nil && element.Tag == "a" {
		if element.Attrs == nil {
			element.Attrs = map[string]string{}
		}
		if element.Attrs["href"] == "" {
			element.Attrs["href"] = href
		}
		return element
	}
	// Build link attributes including epub:type from $616 (Python: -kfx-attrib-epub-type)
	linkAttrs := map[string]string{"href": href}
	if epubType := r.epubTypeFromNode(node); epubType != "" {
		linkAttrs["epub:type"] = epubType
	}
	return &htmlElement{
		Tag:      "a",
		Attrs:    linkAttrs,
		Children: []htmlPart{part},
	}
}

// epubTypeFromAnnotation extracts epub:type value from a $142 style event annotation.
// Python: $616=$617 → -kfx-attrib-epub-type: noteref → epub:type="noteref"
func epubTypeFromAnnotation(annMap map[string]interface{}) string {
	raw, ok := annMap["yj.display"]
	if !ok {
		return ""
	}
	v, ok := asString(raw)
	if !ok {
		return ""
	}
	switch v {
	case "yj.note":
		return "noteref"
	default:
		return v
	}
}

// epubTypeFromNode extracts epub:type from $616 property on a node.
// Python: $616=$617 → -kfx-attrib-epub-type: noteref → epub:type="noteref"
func (r *storylineRenderer) epubTypeFromNode(node map[string]interface{}) string {
	raw, ok := node["yj.display"]
	if !ok {
		return ""
	}
	// $617 → "noteref" (Python yj_to_epub_properties.py line 564)
	if v, ok := asString(raw); ok {
		switch v {
		case "yj.note":
			return "noteref"
		default:
			return v
		}
	}
	return ""
}

func (r *storylineRenderer) anchorHref(anchorID string) string {
	if anchorID == "" {
		return ""
	}
	if href := r.directAnchorURI[anchorID]; href != "" {
		return href
	}
	if href := r.anchorToFilename[anchorID]; href != "" {
		return href
	}
	if r.anchorNameRegistered(anchorID) {
		return "anchor:" + anchorID
	}
	return anchorID
}

func (r *storylineRenderer) anchorNameRegistered(anchorID string) bool {
	if r == nil || anchorID == "" {
		return false
	}
	for _, offsets := range r.positionAnchors {
		for _, names := range offsets {
			for _, name := range names {
				if name == anchorID {
					return true
				}
			}
		}
	}
	return false
}

func (r *storylineRenderer) prepareRenderableNode(node map[string]interface{}) (map[string]interface{}, bool) {
	if node == nil {
		return nil, false
	}
	working := cloneMap(node)
	hadConditionalContent := working["include"] != nil || working["exclude"] != nil || working["yj.conditional_properties"] != nil
	if include := working["include"]; include != nil && !r.conditionEvaluator.evaluateBinary(include) {
		return nil, false
	}
	delete(working, "include")
	if exclude := working["exclude"]; exclude != nil && r.conditionEvaluator.evaluateBinary(exclude) {
		return nil, false
	}
	delete(working, "exclude")
	if rawConditional, ok := asSlice(working["yj.conditional_properties"]); ok {
		for _, raw := range rawConditional {
			props, ok := asMap(raw)
			if !ok {
				continue
			}
			if merged := r.mergeConditionalProperties(working, props); merged != nil {
				working = merged
			}
		}
	}
	delete(working, "yj.conditional_properties")
	if hadConditionalContent {
		working["__has_conditional_content__"] = true
		r.hasConditionalContent = true
	}
	// Port of Python $696 word_boundary_list validation
	// (yj_to_epub_content.py L940-979). Pops word_boundary_list from the node
	// and validates that boundary positions match the text content, logging warnings
	// for mismatches. Purely diagnostic — does not modify output.
	if wbl, ok := asSlice(working["word_boundary_list"]); ok {
		r.validateWordBoundaries(wbl, working)
	}
	delete(working, "word_boundary_list")

	// Port of Python $684 pan_zoom_viewer validation (yj_to_epub_content.py L670-672).
	// Pops pan_zoom_viewer from the node and validates that its value is either
	// nil (absent) or "enabled". Logs an error for unexpected values.
	// Purely diagnostic — does not modify output.
	if panZoomViewer, exists := working["pan_zoom_viewer"]; exists {
		if panZoomViewer != nil {
			if vpStr, ok := asString(panZoomViewer); !ok || vpStr != "enabled" {
				log.Printf("kfx: error: container has pan_zoom_viewer=%v", panZoomViewer)
			}
		}
	}
	delete(working, "pan_zoom_viewer")

	// Port of Python $436 selection validation (yj_to_epub_content.py L1049-1052).
	// Pops selection from the node and validates that its value is either
	// "disabled" or "enabled". Logs an error for unexpected values.
	// Purely diagnostic — does not modify output.
	if selection, exists := working["selection"]; exists {
		if selStr, ok := asString(selection); !ok || (selStr != "disabled" && selStr != "enabled") {
			log.Printf("kfx: error: unexpected selection: %v", selection)
		}
	}
	delete(working, "selection")

	// Port of Python $475 fit_text validation (yj_to_epub_content.py L663-668).
	// Pops fit_text ($475) from the node and validates its value is "force" ($472).
	// Purely diagnostic — does not modify output.
	if fitText, exists := working["fit_text"]; exists {
		if ftStr, ok := asString(fitText); ok && ftStr != "force" {
			log.Printf("kfx: error: container has unexpected fit_text=%s", ftStr)
		}
	}
	delete(working, "fit_text")

	// Port of Python $429 backdrop_style validation (yj_to_epub_content.py L697-704).
	// Pops backdrop_style ($429) from the node, looks up the style definition, and validates
	// that only expected keys remain after popping style_name ($173), fill_color ($70),
	// and fill_opacity ($72). Purely diagnostic — does not modify output.
	if bdStyleName, exists := working["backdrop_style"]; exists {
		if name, ok := asString(bdStyleName); ok && name != "" {
			bdStyleContent := map[string]interface{}{}
			if styleDef, found := r.styleFragments[name]; found {
				for k, v := range styleDef {
					bdStyleContent[k] = v
				}
			} else {
				log.Printf("kfx: error: No definition found for backdrop style: %s", name)
			}
			// Python pops $173 (style_name), $70 (fill_color), $72 (fill_opacity) — expected keys
			delete(bdStyleContent, "style_name")
			delete(bdStyleContent, "fill_color")
			delete(bdStyleContent, "fill_opacity")
			for k := range bdStyleContent {
				log.Printf("kfx: warning: backdrop style %s has unexpected key %q", name, k)
			}
		}
	}
	delete(working, "backdrop_style")

	// Port of Python $754 main_content_id registration (yj_to_epub_content.py L869-870).
	// Registers the main_content_id as an anchor so the element can be targeted
	// by internal links. Python: self.register_link_id(content.pop("$754"), "main_content")
	if mainContentID, exists := working["main_content_id"]; exists {
		if eid, ok := asString(mainContentID); ok && eid != "" {
			r.registerMainContentAnchor(eid)
		}
	}
	delete(working, "main_content_id")

	// Port of Python $426 activate/magnification processing (yj_to_epub_content.py L674-694).
	// Pops activate entries from the node and processes magnification regions.
	// The processRegionMagnification function handles creating <a class="app-amzn-magnify">
	// elements and registering link IDs.
	if activates, exists := working["activate"]; exists {
		result := processRegionMagnification(working, &r.regionMagnification)
		// Store activate elements for attachment to the rendered container element.
		if len(result.ActivateElements) > 0 {
			r.pendingActivateElements = append(r.pendingActivateElements, result.ActivateElements...)
		}
		// Register link IDs for magnification targets/sources.
		for _, reg := range result.LinkRegistrations {
			r.registerMagnifyLink(reg.EID, reg.Kind)
		}
		_ = activates
	}
	delete(working, "activate")
	delete(working, "ordinal")

	// Port of Python $69 ignore:true for fixed layout containers (yj_to_epub_content.py L614-622).
	// When layout is "fixed" ($324) and ignore ($69) is true, Python adds z-index:1 CSS
	// to the content element via self.add_style(content_elem, {"z-index": "1"}).
	// We store the marker so containerClass can add z-index:1 to the element's style.
	if ignoreVal, exists := working["ignore"]; exists {
		ignore, _ := asBool(ignoreVal)
		if ignore {
			layout := asStringDefault(working["layout"])
			if layout == "fixed" {
				working["__ignore_zindex__"] = true
			}
		}
	}
	delete(working, "ignore")

	return working, true
}

// registerMainContentAnchor registers a main_content_id as a position anchor target.
// Port of Python: self.register_link_id(content.pop("$754"), "main_content")
// (yj_to_epub_content.py L869-870).
func (r *storylineRenderer) registerMainContentAnchor(eid string) {
	if r == nil || eid == "" {
		return
	}
	// Register in anchorNamesByID so the anchor system knows about this link target.
	if r.anchorNamesByID == nil {
		r.anchorNamesByID = map[string][]string{}
	}
	// The main_content_id creates an anchor with the eid as its name
	r.anchorNamesByID[eid] = append(r.anchorNamesByID[eid], eid)
	// Also register in directAnchorURI so wrapNodeLink can create hrefs to it
	if r.directAnchorURI == nil {
		r.directAnchorURI = map[string]string{}
	}
}

// registerMagnifyLink registers a magnification link ID for activate elements.
// Port of Python: self.register_link_id(eid, kind) (yj_to_epub_content.py L685-687).
func (r *storylineRenderer) registerMagnifyLink(eid string, kind string) {
	if r == nil || eid == "" {
		return
	}
	anchorName := kind + "_" + eid
	if r.anchorNamesByID == nil {
		r.anchorNamesByID = map[string][]string{}
	}
	r.anchorNamesByID[anchorName] = append(r.anchorNamesByID[anchorName], anchorName)
}

// validateWordBoundaries validates $696 word_boundary_list against the node's text content.
// Port of Python yj_to_epub_content.py L940-979.
// The word_boundary_list is a list of [sep_len, word_len, sep_len, word_len, ...] pairs
// that should cover the entire text. Validation logs warnings for mismatches but does
// not modify the output.
func (r *storylineRenderer) validateWordBoundaries(wbl []interface{}, node map[string]interface{}) {
	if len(wbl)%2 != 0 {
		log.Printf("kfx: warning: Unexpected word_boundary_list length: %v", wbl)
		return
	}

	// Try to get text from the node's content reference (text nodes).
	// Python uses combined_text(content_elem) which requires a rendered element;
	// we validate against the raw text content from the KFX data instead.
	var txt string
	if ref, ok := asMap(node["content"]); ok {
		txt = r.resolveText(ref)
	}
	if txt == "" {
		return
	}

	txtLen := unicodeLen(txt)
	offset := 0

	sepRE := regexp.MustCompile(`^[ \n\u25a0\u25cf]*$`)

	for i := 0; i < len(wbl); i += 2 {
		sepLen, ok := asInt(wbl[i])
		if !ok {
			log.Printf("kfx: warning: Unexpected word_boundary_list separator at index %d: %v", i, wbl)
			break
		}
		if sepLen < 0 || txtLen-offset < sepLen {
			log.Printf("kfx: warning: Unexpected word_boundary_list separator len %d: %v (%d), '%s' (%d)", sepLen, wbl, i, txt, offset)
			break
		}

		sep := unicodeSlice(txt, offset, offset+sepLen)
		if !sepRE.MatchString(sep) {
			log.Printf("kfx: warning: Unexpected word_boundary_list separator '%s': %v (%d), '%s' (%d)", sep, wbl, i, txt, offset)
		}

		offset += sepLen

		wordLen, ok := asInt(wbl[i+1])
		if !ok {
			log.Printf("kfx: warning: Unexpected word_boundary_list word len at index %d: %v", i+1, wbl)
			break
		}
		if wordLen <= 0 || txtLen-offset < wordLen {
			log.Printf("kfx: warning: Unexpected word_boundary_list word len %d: %v (%d), '%s' (%d)", wordLen, wbl, i, txt, offset)
			break
		}

		offset += wordLen
	}

	if offset < txtLen {
		sep := unicodeSlice(txt, offset)
		if !sepRE.MatchString(sep) {
			log.Printf("kfx: warning: Unexpected word_boundary_list final separator '%s': %v (%d), '%s' (%d)", sep, wbl, len(wbl)-2, txt, offset)
		}
	}
}

// unicodeSlice returns the substring s[start:stop] using rune offsets (not byte offsets).
// Port of Python utilities.unicode_slice (utilities.py L724-726).
func unicodeSlice(s string, start int, stop ...int) string {
	runes := []rune(s)
	if len(stop) > 0 {
		end := stop[0]
		if end > len(runes) {
			end = len(runes)
		}
		if start >= end {
			return ""
		}
		return string(runes[start:end])
	}
	if start >= len(runes) {
		return ""
	}
	return string(runes[start:])
}

func (r *storylineRenderer) mergeConditionalProperties(node map[string]interface{}, conditional map[string]interface{}) map[string]interface{} {
	if node == nil || conditional == nil {
		return node
	}
	props := cloneMap(conditional)
	apply := false
	if include := props["include"]; include != nil {
		apply = r.conditionEvaluator.evaluateBinary(include)
		delete(props, "include")
	} else if exclude := props["exclude"]; exclude != nil {
		apply = !r.conditionEvaluator.evaluateBinary(exclude)
		delete(props, "exclude")
	}
	if !apply {
		return node
	}
	merged := cloneMap(node)
	for key, value := range props {
		merged[key] = value
	}
	return merged
}

func (r *storylineRenderer) applyStructuralNodeAttrs(element *htmlElement, node map[string]interface{}, parentTag string) {
	if element == nil || node == nil {
		return
	}
	if element.Tag == "div" {
		if r.shouldPromoteLayoutHints() && layoutHintsInclude(r.nodeLayoutHints(node), "figure") && htmlPartContainsImage(element) {
			element.Tag = "figure"
		}
	}
	classification, _ := asString(node["yj.classification"])
	switch {
	case classification == "caption" && parentTag == "table" && element.Tag == "div":
		element.Tag = "caption"
	case classificationEPUBType[classification] != "" && element.Tag == "div":
		element.Tag = "aside"
	}
	if epubType := classificationEPUBType[classification]; epubType != "" && element.Tag == "aside" {
		element.Attrs["epub:type"] = epubType
	}
	if classification == "math" {
		element.Attrs["role"] = "math"
	}
	switch asStringDefault(node["layout"]) {
	case "fixed", "overflow":
		if styleAttr := styleStringFromDeclarations("class", nil, []string{"position: fixed"}); styleAttr != "" {
			element.Attrs["style"] = mergeStyleStrings(element.Attrs["style"], styleAttr)
		}
	}
}

func (r *storylineRenderer) nodeLayoutHints(node map[string]interface{}) []string {
	if node == nil {
		return nil
	}
	styleID, _ := asString(node["style"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	switch typed := style["layout_hints"].(type) {
	case string:
		if typed == "" {
			return nil
		}
		if hint := layoutHintElementNames[typed]; hint != "" {
			return []string{hint}
		}
		return strings.Fields(typed)
	case []interface{}:
		hints := make([]string, 0, len(typed))
		for _, raw := range typed {
			value, ok := asString(raw)
			if !ok || value == "" {
				continue
			}
			if hint := layoutHintElementNames[value]; hint != "" {
				hints = append(hints, hint)
				continue
			}
			hints = append(hints, strings.Fields(value)...)
		}
		if len(hints) == 0 {
			return nil
		}
		return hints
	default:
		return nil
	}
}

// layoutHintHeadingLevel returns the heading level (1-6) if the node should become a heading,
// or 0 if it should not. Previously this returned an HTML tag like "h1" and the caller
// created that tag directly. Now we defer the tag decision to simplify_styles (matching
// Python yj_to_epub_properties.py:1922) and only communicate the heading level.
func (r *storylineRenderer) layoutHintHeadingLevel(node map[string]interface{}, children []interface{}) int {
	if !r.shouldPromoteStructuralContainer(node) {
		return 0
	}
	if !layoutHintsInclude(r.nodeLayoutHints(node), "heading") {
		return 0
	}
	level := headingLevel(node)
	if level <= 0 || level > 6 {
		return 0
	}
	for _, child := range children {
		if r.renderInlinePart(child, 0) == nil {
			return 0
		}
	}
	return level
}

// renderInlineParagraphContainer creates a <div> for inline-only containers with text.
// Python creates a <div> via process_content and simplify_styles later converts to <p>.
// Previously Go created <p> here directly — now deferred to simplify_styles (Python parity).
func (r *storylineRenderer) renderInlineParagraphContainer(node map[string]interface{}, children []interface{}, depth int) htmlPart {
	if !r.shouldPromoteStructuralContainer(node) || len(children) != 1 || !nodeContainsTextContent(children) {
		return nil
	}
	element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	for _, child := range children {
		if inline := r.renderInlinePart(child, depth+1); inline != nil {
			element.Children = append(element.Children, inline)
			continue
		}
		return nil
	}
	if len(element.Children) == 0 {
		return nil
	}
	styleID, _ := asString(node["style"])
	if styleAttr := r.paragraphClass(styleID, ""); styleAttr != "" {
		element.Attrs["style"] = styleAttr
	}
	r.applyFirstLineStyle(element, node)
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderInlineRenderContainer(node map[string]interface{}, children []interface{}, depth int) htmlPart {
	renderMode, _ := asString(node["render"])
	if renderMode != "inline" {
		return nil
	}
	styleID, _ := asString(node["style"])
	element := &htmlElement{Tag: "span", Attrs: map[string]string{}}
	for _, child := range children {
		if inline := r.renderInlinePart(child, depth+1); inline != nil {
			element.Children = append(element.Children, inline)
			continue
		}
		return nil
	}
	if len(element.Children) == 0 {
		return nil
	}

	// Python yj_to_epub_content.py L1299-1312: after retagging div→span, check is_inline_only.
	// If the element tree contains only inline tags, keep it as <span> and potentially
	// flatten a bare single <span> child. If NOT inline-only, revert to <div> with
	// display:inline-block (the fit_width path from Python L1309-1310).
	if isInlineOnly(element) {
		// Python L1304-1307: flatten single bare <span> child with no attributes.
		// if len(content_elem) == 1 and content_elem[0].tag == "span" and
		//    len(content_elem[0]) == 0 and len(content_elem[0].attrib) == 0:
		if len(element.Children) == 1 {
			if child, ok := element.Children[0].(*htmlElement); ok &&
				child.Tag == "span" && len(child.Children) == 0 && len(child.Attrs) == 0 {
				// Merge: element.text = (element.text or "") + (child.text or "") + (child.tail or "")
				// In our model, element has no Text field (text is in htmlText children),
				// and child is an empty span with no children. Nothing to merge.
				// Python removes the child element, leaving a <span> with combined text.
				element.Children = nil
			}
		}
	} else {
		// Python L1308-1310: else: content_elem.tag = "div"; fit_width = True
		// fit_width adds display:inline-block for divs (Python L1346-1347).
		element.Tag = "div"
		fitWidthStyle := styleStringFromDeclarations("class", nil, []string{"display: inline-block"})
		if fitWidthStyle != "" {
			element.Attrs["style"] = mergeStyleStrings(element.Attrs["style"], fitWidthStyle)
		}
	}

	if styleAttr := r.inlineContainerClass(styleID, node); styleAttr != "" {
		element.Attrs["style"] = mergeStyleStrings(element.Attrs["style"], styleAttr)
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) renderFigureHintContainer(node map[string]interface{}, children []interface{}, depth int) htmlPart {
	if !r.shouldPromoteStructuralContainer(node) || !layoutHintsInclude(r.nodeLayoutHints(node), "figure") {
		return nil
	}
	element := &htmlElement{Tag: "div", Attrs: map[string]string{}}
	for _, child := range children {
		if rendered := r.renderNode(child, depth+1); rendered != nil {
			element.Children = append(element.Children, rendered)
			continue
		}
		if inline := r.renderInlinePart(child, depth+1); inline != nil {
			element.Children = append(element.Children, inline)
			continue
		}
		return nil
	}
	if len(element.Children) == 0 || !htmlPartContainsImage(element) {
		return nil
	}
	// Python's COMBINE_NESTED_DIVS for figure containers.
	if wrapper := singleImageWrapperChild(element); wrapper != nil {
		containerStyle := r.containerClass(node)
		wrapperStyle := ""
		if wrapper.Attrs != nil {
			wrapperStyle = wrapper.Attrs["style"]
		}
		// Parent (container) wins on conflicts, matching Python's replace=False
		mergedStyle := mergeStyleStrings(wrapperStyle, containerStyle)
		if mergedStyle != "" {
			wrapper.Attrs["style"] = mergedStyle
		} else {
			delete(wrapper.Attrs, "style")
		}
		r.applyStructuralNodeAttrs(wrapper, node, "")
		if positionID, _ := asInt(node["id"]); positionID != 0 {
			r.applyPositionAnchors(wrapper, positionID, false)
		}
		return wrapper
	}
	if styleAttr := r.containerClass(node); styleAttr != "" {
		element.Attrs["style"] = styleAttr
	}
	r.applyStructuralNodeAttrs(element, node, "")
	if positionID, _ := asInt(node["id"]); positionID != 0 {
		r.applyPositionAnchors(element, positionID, false)
	}
	return element
}

func (r *storylineRenderer) shouldPromoteLayoutHints() bool {
	if r == nil {
		return true
	}
	return !r.conditionEvaluator.fixedLayout && !r.conditionEvaluator.illustratedLayout
}

func (r *storylineRenderer) shouldPromoteStructuralContainer(node map[string]interface{}) bool {
	if !r.shouldPromoteLayoutHints() || node == nil {
		return false
	}
	if node["__has_conditional_content__"] != nil || node["yj.classification"] != nil {
		return false
	}
	switch asStringDefault(node["layout"]) {
	case "fixed", "overflow":
		return false
	}
	return true
}

func nodeContainsTextContent(children []interface{}) bool {
	for _, raw := range children {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		if _, ok := asMap(node["content"]); ok {
			return true
		}
		if nested, ok := asSlice(node["content_list"]); ok && nodeContainsTextContent(nested) {
			return true
		}
	}
	return false
}

func layoutHintsInclude(hints []string, want string) bool {
	for _, hint := range hints {
		if hint == want {
			return true
		}
	}
	return false
}

// extractLayoutHintsFromStyle extracts layout hints from a YJ style map.
// Ported from Python's nodeLayoutHints logic: reads the "layout_hints" key
// from the style and converts to a []string suitable for styleStringFromDeclarations.
func extractLayoutHintsFromStyle(style map[string]interface{}) []string {
	if style == nil {
		return nil
	}
	switch typed := style["layout_hints"].(type) {
	case string:
		if typed == "" {
			return nil
		}
		if hint := layoutHintElementNames[typed]; hint != "" {
			return []string{hint}
		}
		return strings.Fields(typed)
	case []interface{}:
		hints := make([]string, 0, len(typed))
		for _, raw := range typed {
			value, ok := asString(raw)
			if !ok || value == "" {
				continue
			}
			if hint := layoutHintElementNames[value]; hint != "" {
				hints = append(hints, hint)
			} else {
				hints = append(hints, value)
			}
		}
		if len(hints) == 0 {
			return nil
		}
		return hints
	}
	return nil
}

func htmlPartContainsImage(part htmlPart) bool {
	switch typed := part.(type) {
	case *htmlElement:
		if typed == nil {
			return false
		}
		if typed.Tag == "img" {
			return true
		}
		for _, child := range typed.Children {
			if htmlPartContainsImage(child) {
				return true
			}
		}
	}
	return false
}

func (r *storylineRenderer) bodyClass(styleID string, values map[string]interface{}) string {
	style := effectiveStyle(r.styleFragments[styleID], values)
	if len(style) == 0 {
		return ""
	}
	declarations := cssDeclarationsFromMap(r.processContentProps(style, r.resolveResource))
	if len(declarations) == 0 {
		return ""
	}
	if bodyClass := staticBodyClassForDeclarations(declarations); bodyClass != "" {
		return bodyClass
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) containerClass(node map[string]interface{}) string {
	styleID, _ := asString(node["style"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = mergeStyleValues(style, r.inferPromotedStyleValues(node))
	if len(style) == 0 {
		return ""
	}
	cssMap := r.processContentProps(style, r.resolveResource)

	// Handle -kfx-box-align → margin auto conversion, matching Python
	// yj_to_epub_content.py:1390-1404. Container elements get margin-auto
	// only when they have a width property (or are tables).
	if boxAlign, ok := cssMap["-kfx-box-align"]; ok {
		delete(cssMap, "-kfx-box-align")
		if boxAlign == "center" || boxAlign == "left" || boxAlign == "right" {
			_, hasWidth := cssMap["width"]
			if hasWidth {
				if boxAlign != "left" {
					cssMap["margin-left"] = "auto"
				}
				if boxAlign != "right" {
					cssMap["margin-right"] = "auto"
				}
			}
		}
	}

	declarations := cssDeclarationsFromMap(cssMap)
	if mapFontStyle(style["font_style"]) == "normal" && bodyDefaultsInclude(r.activeBodyDefaults, "font-style: italic") {
		declarations = append(declarations, "font-style: normal")
	}
	// Port of Python $69 ignore:true z-index addition (yj_to_epub_content.py L617).
	// When layout is "fixed" and ignore is true, Python adds z-index:1 via
	// self.add_style(content_elem, {"z-index": "1"}).
	if _, hasIgnoreZIndex := node["__ignore_zindex__"]; hasIgnoreZIndex {
		declarations = append(declarations, "z-index: 1")
	}
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, r.nodeLayoutHints(node), declarations)
}

func (r *storylineRenderer) tableClass(node map[string]interface{}) string {
	styleID, _ := asString(node["style"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	cssMap := r.processContentProps(style, r.resolveResource)

	// Handle -kfx-box-align → margin auto conversion for tables.
	// Ported from Python yj_to_epub_content.py (~L1390-1404):
	// For tables with box-align left/right/center, set the appropriate
	// margin-left/margin-right to auto (replacing any explicit value).
	// Tables always have a known width, so auto margins are appropriate.
	if boxAlign, ok := cssMap["-kfx-box-align"]; ok {
		delete(cssMap, "-kfx-box-align")
		if boxAlign == "center" || boxAlign == "left" || boxAlign == "right" {
			if boxAlign != "left" {
				cssMap["margin-left"] = "auto"
			}
			if boxAlign != "right" {
				cssMap["margin-right"] = "auto"
			}
		}
	}

	declarations := cssDeclarationsFromMap(cssMap)
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) tableColumnClass(node map[string]interface{}) string {
	style := effectiveStyle(nil, node)
	declarations := cssDeclarationsFromMap(r.processContentProps(style, r.resolveResource))
	if len(declarations) == 0 {
		return ""
	}
	return styleStringFromDeclarations("class", nil, declarations)
}

// fittedContainerClass generates the style class for the outer wrapper of a fitted container.
// It handles -kfx-box-align by converting it to text-align on the outer wrapper (not margin-auto),
// matching Python yj_to_epub_content.py:1375-1381 where fitted (inline-block) elements get
// a wrapper with text-align from box-align so the inline-block is horizontally positioned.
func (r *storylineRenderer) fittedContainerClass(node map[string]interface{}) string {
	styleID, _ := asString(node["style"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = mergeStyleValues(style, r.inferPromotedStyleValues(node))
	if len(style) == 0 {
		return ""
	}
	cssMap := r.processContentProps(style, r.resolveResource)

	// Handle -kfx-box-align → text-align conversion for fitted containers.
	// Python yj_to_epub_content.py:1375-1381:
	//   if "-kfx-box-align" in content_style:
	//       container_elem, container_style = self.create_container(
	//           content_elem, content_style, "div", BLOCK_ALIGNED_CONTAINER_PROPERTIES)
	//       container_style["text-align"] = container_style.pop("-kfx-box-align")
	// The outer wrapper gets text-align, which positions the inline-block inner element.
	if boxAlign, ok := cssMap["-kfx-box-align"]; ok {
		delete(cssMap, "-kfx-box-align")
		if boxAlign == "center" || boxAlign == "left" || boxAlign == "right" {
			cssMap["text-align"] = boxAlign
		}
	}

	declarations := cssDeclarationsFromMap(cssMap)
	if mapFontStyle(style["font_style"]) == "normal" && bodyDefaultsInclude(r.activeBodyDefaults, "font-style: italic") {
		declarations = append(declarations, "font-style: normal")
	}
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, r.nodeLayoutHints(node), declarations)
}

func (r *storylineRenderer) structuredContainerClass(node map[string]interface{}) string {
	styleID, _ := asString(node["style"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = mergeStyleValues(style, r.inferPromotedStyleValues(node))
	declarations := cssDeclarationsFromMap(r.processContentProps(style, r.resolveResource))
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

// tableCellCombineNestedDivs returns true when Python's COMBINE_NESTED_DIVS
// (yj_to_epub_content.py:1408-1448) would merge the cell $269 with its single child $269.
// This happens when the parent and child CSS properties don't overlap (excluding -kfx-style-name).
func (r *storylineRenderer) tableCellCombineNestedDivs(node map[string]interface{}) bool {
	children, ok := asSlice(node["content_list"])
	if !ok || len(children) != 1 {
		return false
	}
	child, ok := asMap(children[0])
	if !ok {
		return false
	}
	childContentType, _ := asString(child["type"])
	if childContentType != "text" {
		return false
	}
	if _, has145 := asMap(child["content"]); !has145 {
		return false // child doesn't have text content to merge
	}
	styleID, _ := asString(node["style"])
	childStyleID, _ := asString(child["style"])
	if styleID == "" || childStyleID == "" {
		return true // no style conflict
	}
	ownStyle := effectiveStyle(r.styleFragments[styleID], node)
	parentCSS := r.processContentProps(ownStyle, r.resolveResource)
	childStyle := effectiveStyle(r.styleFragments[childStyleID], child)
	childCSS := r.processContentProps(childStyle, r.resolveResource)
	for prop := range parentCSS {
		if prop == "-kfx-style-name" {
			continue
		}
		if _, exists := childCSS[prop]; exists {
			return false // overlap → no merge
		}
	}
	return true // no overlap → merge
}

func (r *storylineRenderer) tableCellClass(node map[string]interface{}) string {
	styleID, _ := asString(node["style"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = mergeStyleValues(style, r.inferPromotedStyleValues(node))

	// Replicate Python's COMBINE_NESTED_DIVS for table cells.
	// In Python, process_content creates nested divs for cell + content, then
	// COMBINE_NESTED_DIVS merges them when parent has no text, single child div,
	// block display, static position, no float, no overlapping properties.
	// content_style.update(child_sty, replace=False) adds child-only properties.
	// We check overlap against the parent's own CSS properties (before inference),
	// since inferred properties are reverse-inherited from children.
	if children, ok := asSlice(node["content_list"]); ok && len(children) == 1 {
		if child, ok := asMap(children[0]); ok {
			childStyleID, _ := asString(child["style"])
			if childStyleID != "" {
				ownStyle := effectiveStyle(r.styleFragments[styleID], node)
				parentCSS := r.processContentProps(ownStyle, r.resolveResource)
				childStyle := effectiveStyle(r.styleFragments[childStyleID], child)
				childCSS := r.processContentProps(childStyle, r.resolveResource)
				// Python excludes -kfx-style-name from overlap check
				hasOverlap := false
				for prop := range parentCSS {
					if prop == "-kfx-style-name" {
						continue
					}
					if _, exists := childCSS[prop]; exists {
						hasOverlap = true
						break
					}
				}
				if !hasOverlap {
					style = mergeStyleValues(style, childStyle)
				}
			}
		}
	}

	cssMap := r.processContentProps(style, r.resolveResource)

	// Handle -kfx-box-align → text-align conversion for table cells.
	// In Python's process_content (yj_to_epub_content.py), -kfx-box-align is popped from
	// td element styles. Since cssDeclarationsFromMap strips -kfx- prefixed properties,
	// we convert it to text-align here to preserve alignment information, matching the old
	// tableCellStyleDeclarations which used mapBoxAlign to convert $580/$34 values to text-align.
	if boxAlign, ok := cssMap["-kfx-box-align"]; ok {
		if boxAlign == "center" || boxAlign == "left" || boxAlign == "right" || boxAlign == "justify" {
			if _, exists := cssMap["text-align"]; !exists {
				cssMap["text-align"] = boxAlign
			}
		}
		delete(cssMap, "-kfx-box-align")
	}

	// Strip -kfx-attrib-colspan/rowspan from style — already handled as direct HTML attributes.
	// cssDeclarationsFromMap now preserves -kfx-attrib-* properties, but for table cells
	// these are redundant (extracted from $148/$149 into colspan/rowspan attrs directly).
	delete(cssMap, "-kfx-attrib-colspan")
	delete(cssMap, "-kfx-attrib-rowspan")

	declarations := cssDeclarationsFromMap(cssMap)
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) inlineContainerClass(styleID string, node map[string]interface{}) string {
	style := effectiveStyle(r.styleFragments[styleID], node)
	declarations := cssDeclarationsFromMap(r.processContentProps(style, r.resolveResource))
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

var blockAlignedContainerProperties = map[string]bool{
	"-kfx-attrib-colspan": true, "-kfx-attrib-rowspan": true,
	"-kfx-box-align": true, "-kfx-heading-level": true, "-kfx-layout-hints": true,
	"-kfx-table-vertical-align": true,
	"box-sizing": true,
	"float": true,
	"margin-left": true, "margin-right": true, "margin-top": true, "margin-bottom": true,
	"overflow": true,
	"page-break-after": true, "page-break-before": true, "page-break-inside": true,
	"text-indent": true,
	"transform": true, "transform-origin": true,
}

var reverseHeritablePropertiesExcludes = map[string]bool{
	"-amzn-page-align":                   true,
	"-kfx-user-margin-bottom-percentage": true,
	"-kfx-user-margin-left-percentage":   true,
	"-kfx-user-margin-right-percentage":  true,
	"-kfx-user-margin-top-percentage":    true,
	"font-size":  true,
	"line-height": true,
}

func isBlockContainerProperty(prop string) bool {
	if blockAlignedContainerProperties[prop] || prop == "display" {
		return true
	}
	return heritableProperties[prop] && !reverseHeritablePropertiesExcludes[prop]
}

func (r *storylineRenderer) imageClasses(node map[string]interface{}) (string, string) {
	styleID, _ := asString(node["style"])
	style := effectiveStyle(r.styleFragments[styleID], node)
	style = r.adjustRenderableStyle(style, node)
	if len(style) == 0 {
		return "", ""
	}
	cssMap := r.processContentProps(style, r.resolveResource)
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}

	// Handle -kfx-box-align → text-align conversion.
	// The old imageWrapperStyleDeclarations used mapBoxAlign to convert $580/$34
	// directly to text-align. processContentProperties outputs -kfx-box-align instead.
	// Convert it to text-align for CSS output, matching the old behavior.
	if boxAlign, ok := cssMap["-kfx-box-align"]; ok {
		if boxAlign == "center" || boxAlign == "left" || boxAlign == "right" || boxAlign == "justify" {
			cssMap["text-align"] = boxAlign
		}
		delete(cssMap, "-kfx-box-align")
	}

	// Partition properties between wrapper div and image element, matching Python's
	// create_container(BLOCK_CONTAINER_PROPERTIES) in yj_to_epub_content.py:1324.
	// Container gets: REVERSE_HERITABLE_PROPERTIES | BLOCK_ALIGNED_CONTAINER_PROPERTIES | {"display"}.
	// Image keeps everything else. No properties are dropped.
	wrapperProps := map[string]string{}
	imageProps := map[string]string{}
	for prop, val := range cssMap {
		if isBlockContainerProperty(prop) {
			wrapperProps[prop] = val
		} else {
			imageProps[prop] = val
		}
	}

	// Python yj_to_epub_content.py:1328-1331: when container has float, move % width
	// from inner image to container and set image width to 100%.
	if _, hasFloat := wrapperProps["float"]; hasFloat {
		if w, ok := imageProps["width"]; ok && strings.HasSuffix(w, "%") {
			wrapperProps["width"] = w
			imageProps["width"] = "100%"
		}
	}

	wrapperDecls := cssDeclarationsFromMap(wrapperProps)
	imageDecls := cssDeclarationsFromMap(imageProps)

	switch {
	case len(wrapperDecls) > 0 && len(imageDecls) > 0:
		return styleStringFromDeclarations(baseName, nil, wrapperDecls), styleStringFromDeclarations(baseName, nil, imageDecls)
	case len(wrapperDecls) > 0:
		return styleStringFromDeclarations(baseName, nil, wrapperDecls), ""
	case len(imageDecls) > 0:
		return "", styleStringFromDeclarations(baseName, nil, imageDecls)
	default:
		return "", ""
	}
}

func (r *storylineRenderer) adjustRenderableStyle(style map[string]interface{}, node map[string]interface{}) map[string]interface{} {
	if len(style) == 0 {
		return style
	}
	if fitTight, _ := asBool(node["fit_tight"]); fitTight {
		if value := cssLengthProperty(style["width"], "width"); value == "100%" {
			style = cloneMap(style)
			delete(style, "width")
		}
	}
	return style
}

func (r *storylineRenderer) headingClass(styleID string) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	className := r.headingClassName(styleID, style)
	declarations := cssDeclarationsFromMap(r.processContentProps(style, r.resolveResource))
	if mapFontStyle(style["font_style"]) == "normal" && bodyDefaultsInclude(r.activeBodyDefaults, "font-style: italic") {
		declarations = append(declarations, "font-style: normal")
	}
	// Note: text-indent reset is NOT added here. Python does not add text-indent: 0
	// during rendering. Instead, simplify_styles handles it via nonHeritableDefaultProperties
	// (which includes text-indent: 0 for <div>/<p> elements). The stripping loop in
	// simplify_styles keeps text-indent: 0 when it differs from the inherited body text-indent.
	// Previously Go added text-indent: 0 here, but that caused false overlaps in
	// COMBINE_NESTED_DIVS and differed from Python's rendering pipeline.
	if len(declarations) == 0 {
		return ""
	}
	return styleStringFromDeclarations(className, []string{"heading"}, declarations)
}

func (r *storylineRenderer) paragraphClass(styleID string, annotationStyleID string) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	// Merge link style inheritance: when paragraph doesn't have certain properties,
	// inherit them from the link (annotation) style. This preserves the behavior of
	// the old paragraphStyleDeclarations link inheritance block (kfx.go:1164-1201).
	linkStyle := effectiveStyle(r.styleFragments[annotationStyleID], nil)
	if linkStyle != nil {
		// Merge link color properties ($576=visited-color, $577=link-color) for color resolution
		for _, yjProp := range []string{"link_visited_style", "link_unvisited_style"} {
			if _, ok := style[yjProp]; !ok {
				if val, ok := linkStyle[yjProp]; ok {
					style[yjProp] = val
				}
			}
		}
		// Merge link font/typographic properties when paragraph doesn't have them
		for _, yjProp := range []string{"font_family", "font_style", "font_weight", "glyph_transform", "text_transform"} {
			if _, ok := style[yjProp]; !ok {
				if val, ok := linkStyle[yjProp]; ok {
					style[yjProp] = val
				}
			}
		}
	}
	cssMap := r.processContentProps(style, r.resolveResource)
	// Resolve link color: if no explicit color but -kfx-link-color == -kfx-visited-color,
	// set color to that value. This preserves what colorDeclarations(style, linkStyle) did
	// in the old paragraphStyleDeclarations, and matches simplifyStylesElementFull's <a> tag logic.
	if _, hasColor := cssMap["color"]; !hasColor {
		linkColor, hasLink := cssMap["-kfx-link-color"]
		visitedColor, hasVisited := cssMap["-kfx-visited-color"]
		if hasLink && hasVisited && linkColor == visitedColor {
			cssMap["color"] = linkColor
		}
	}
	declarations := cssDeclarationsFromMap(cssMap)
	if mapFontStyle(style["font_style"]) == "normal" && bodyDefaultsInclude(r.activeBodyDefaults, "font-style: italic") {
		declarations = append(declarations, "font-style: normal")
	}
	// Note: text-indent reset is NOT added here (see comment in paragraphClass).
	if os.Getenv("KFX_DEBUG_PARAGRAPH_STYLE") != "" {
		fmt.Fprintf(os.Stderr, "paragraph style=%s body=%s decls=%v\n", styleID, r.activeBodyClass, declarations)
	}
	className := ""
	if len(declarations) > 0 {
		baseName := "class"
		if styleID != "" {
			baseName = r.styleBaseName(styleID)
		}
		className = styleStringFromDeclarations(baseName, nil, declarations)
	}
	if annotationStyleID != "" {
		_ = r.linkClass(annotationStyleID, true)
	}
	return className
}

func (r *storylineRenderer) linkClass(styleID string, suppressColor bool) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	cssMap := r.processContentProps(style, r.resolveResource)
	// Always resolve link color: if no explicit color but -kfx-link-color == -kfx-visited-color,
	// set color to that value. This matches simplifyStylesElementFull's <a> tag logic.
	// Previously we suppressed color when suppressColor was true (for paragraphs that
	// handle color via link style inheritance). But simplify_styles will strip the
	// redundant color from <a> if it matches inherited, so we don't need to suppress.
	if _, hasColor := cssMap["color"]; !hasColor {
		linkColor, hasLink := cssMap["-kfx-link-color"]
		visitedColor, hasVisited := cssMap["-kfx-visited-color"]
		if hasLink && hasVisited && linkColor == visitedColor {
			cssMap["color"] = linkColor
		}
	}
	// Always strip -kfx- properties (they're not real CSS and will appear in the
	// style catalog if not removed here).
	delete(cssMap, "-kfx-link-color")
	delete(cssMap, "-kfx-visited-color")
	declarations := cssDeclarationsFromMap(cssMap)
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) spanClass(styleID string) string {
	style := effectiveStyle(r.styleFragments[styleID], nil)
	if len(style) == 0 {
		return ""
	}
	declarations := cssDeclarationsFromMap(r.processContentProps(style, r.resolveResource))
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

// annotationSpanClass generates the CSS class for an annotation's styled span.
// Port of Python yj_to_epub_content.py:1142 + 1307:
//   self.add_kfx_style(style_event, style_event.pop("style", None))  → merges style fragment into event
//   self.add_style(event_elem, self.process_content_properties(style_event), replace=True)
// Python merges the style fragment properties INTO the annotation map, then processes all properties.
// Go must do the same: merge the style fragment with the annotation's own properties.
func (r *storylineRenderer) annotationSpanClass(styleID string, annotationMap map[string]interface{}) string {
	style := effectiveStyle(r.styleFragments[styleID], annotationMap)
	if len(style) == 0 {
		return ""
	}
	declarations := cssDeclarationsFromMap(r.processContentProps(style, r.resolveResource))
	if len(declarations) == 0 {
		return ""
	}
	baseName := "class"
	if styleID != "" {
		baseName = r.styleBaseName(styleID)
	}
	return styleStringFromDeclarations(baseName, nil, declarations)
}

func (r *storylineRenderer) resolveText(ref map[string]interface{}) string {
	return resolveContentText(r.contentFragments, ref)
}

func hasRenderableContainer(node map[string]interface{}) bool {
	_, hasStyle := asString(node["style"])
	children, hasChildren := asSlice(node["content_list"])
	_, hasImage := asString(node["resource_name"])
	_, hasText := asMap(node["content"])
	return hasStyle && !hasImage && !hasText && (!hasChildren || len(children) == 0)
}

func promotedBodyContainer(nodes []interface{}) (string, []interface{}, bool, bool) {
	if len(nodes) != 1 {
		return "", nil, false, false
	}
	node, ok := asMap(nodes[0])
	if !ok {
		return "", nil, false, false
	}
	styleID, _ := asString(node["style"])

	// Case 1: Container node with content_list children.
	// Python's process_content creates a <div> that is_top_level renames to <body>.
	if children, ok := asSlice(node["content_list"]); ok && len(children) > 0 && styleID != "" {
		if _, ok := asMap(node["content"]); !ok {
			if _, ok := asString(node["resource_name"]); !ok {
				return styleID, children, true, false // container: use renderNode
			}
		}
	}

	// Case 2: Leaf text node with heading properties.
	// Python's process_content creates a <div> for this node, then is_top_level
	// renames it to <body>. The heading style goes on <body> and the text
	// content is rendered inline inside <body>.
	if styleID != "" {
		if _, hasContent := asMap(node["content"]); hasContent {
			if headingLevel(node) > 0 {
				return styleID, nodes, true, true // leaf heading: render inline
			}
		}
	}

	return "", nil, false, false
}

func defaultInheritedBodyStyle() map[string]interface{} {
	zero := 0.0
	return map[string]interface{}{
		"font_family": "default,serif",
		"font_style": "normal",
		"font_weight": "normal",
		"text_indent": map[string]interface{}{
			"unit": "em",
			"value": &zero,
		},
	}
}

func (r *storylineRenderer) inferBodyStyleValues(nodes []interface{}, parentStyle map[string]interface{}) map[string]interface{} {
	return r.inferSharedHeritableStyle(parentStyle, nodes)
}

func (r *storylineRenderer) inferPromotedBodyStyle(nodes []interface{}) map[string]interface{} {
	return r.inferSharedHeritableStyle(nil, nodes)
}

func (r *storylineRenderer) inferPromotedStyleValues(node map[string]interface{}) map[string]interface{} {
	children, ok := asSlice(node["content_list"])
	if !ok || len(children) == 0 {
		return nil
	}
	styleID, _ := asString(node["style"])
	return r.inferSharedHeritableStyle(effectiveStyle(r.styleFragments[styleID], node), children)
}

func (r *storylineRenderer) inferSharedHeritableStyle(parentStyle map[string]interface{}, nodes []interface{}) map[string]interface{} {
	if len(nodes) == 0 {
		return nil
	}
	type valueCount struct {
		count int
		raw   interface{}
	}
	const reverseInheritanceFraction = 0.8
	keys := []string{"font_family", "font_style", "font_weight", "text_alignment", "text_indent", "text_transform", "glyph_transform"}
	valueCounts := map[string]map[string]*valueCount{}
	numChildren := 0
	debugInfer := os.Getenv("KFX_DEBUG_INFER_COUNTS") != ""
	debugStyleIDs := make([]string, 0, len(nodes))
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		styleID, _ := asString(node["style"])
		if debugInfer {
			debugStyleIDs = append(debugStyleIDs, styleID)
		}
		style := effectiveStyle(r.styleFragments[styleID], node)
		if childPromoted := r.inferPromotedStyleValues(node); len(childPromoted) > 0 {
			style = mergeStyleValues(style, childPromoted)
		}
		numChildren++
		if len(style) == 0 {
			continue
		}
		for _, key := range keys {
			rawValue, ok := style[key]
			if !ok {
				continue
			}
			valueKey := fmt.Sprintf("%#v", rawValue)
			if valueCounts[key] == nil {
				valueCounts[key] = map[string]*valueCount{}
			}
			entry := valueCounts[key][valueKey]
			if entry == nil {
				entry = &valueCount{raw: rawValue}
				valueCounts[key][valueKey] = entry
			}
			entry.count++
		}
	}
	if numChildren == 0 {
		return nil
	}
	values := map[string]interface{}{}
	for _, key := range keys {
		counts := valueCounts[key]
		if len(counts) == 0 {
			continue
		}
		var (
			bestKey   string
			bestValue interface{}
			bestCount int
			total     int
		)
		for valueKey, entry := range counts {
			total += entry.count
			if entry.count > bestCount {
				bestKey = valueKey
				bestValue = entry.raw
				bestCount = entry.count
			}
		}
		if bestKey == "" {
			continue
		}
		if total < numChildren && (parentStyle == nil || parentStyle[key] == nil) {
			continue
		}
		if float64(bestCount) >= float64(numChildren)*reverseInheritanceFraction {
			values[key] = bestValue
		}
	}
	if len(values) == 0 {
		if debugInfer {
			fmt.Fprintf(os.Stderr, "infer none numChildren=%d styles=%v counts=", numChildren, debugStyleIDs)
			for _, key := range keys {
				if len(valueCounts[key]) == 0 {
					continue
				}
				fmt.Fprintf(os.Stderr, " %s:{", key)
				first := true
				for valueKey, entry := range valueCounts[key] {
					if !first {
						fmt.Fprint(os.Stderr, ", ")
					}
					first = false
					fmt.Fprintf(os.Stderr, "%s=%d", valueKey, entry.count)
				}
				fmt.Fprint(os.Stderr, "}")
			}
			fmt.Fprintln(os.Stderr)
		}
		return nil
	}
	if debugInfer {
		fmt.Fprintf(os.Stderr, "infer values numChildren=%d styles=%v values=%#v\n", numChildren, debugStyleIDs, values)
	}
	return values
}

func headingLevel(node map[string]interface{}) int {
	value, ok := node["yj.semantics.heading_level"]
	if !ok {
		return 0
	}
	level, _ := asInt(value)
	return level
}

func fullParagraphAnnotationStyleID(node map[string]interface{}, text string) string {
	annotations, ok := asSlice(node["style_events"])
	if !ok || len(annotations) == 0 {
		return ""
	}
	runeCount := len([]rune(text))
	for _, raw := range annotations {
		annotationMap, ok := asMap(raw)
		if !ok || !annotationCoversWholeText(annotationMap, runeCount) {
			continue
		}
		styleID, _ := asString(annotationMap["style"])
		return styleID
	}
	return ""
}

func annotationCoversWholeText(annotationMap map[string]interface{}, runeCount int) bool {
	if annotationMap == nil || runeCount == 0 {
		return false
	}
	start, hasStart := asInt(annotationMap["offset"])
	length, hasLength := asInt(annotationMap["length"])
	_, hasAnchor := asString(annotationMap["link_to"])
	return hasAnchor && hasStart && hasLength && start == 0 && length >= runeCount
}

func (r *storylineRenderer) headingClassName(styleID string, style map[string]interface{}) string {
	simplified := uniquePartOfLocalSymbol(styleID, r.symFmt)
	if simplified != "" {
		return "heading_" + simplified
	}
	return "heading_" + styleID
}

func filterBodyDefaultDeclarations(declarations []string, bodyDefaults map[string]bool) []string {
	if len(declarations) == 0 {
		return declarations
	}
	filtered := make([]string, 0, len(declarations))
	for _, declaration := range declarations {
		if bodyDefaults != nil && bodyDefaults[declaration] {
			continue
		}
		filtered = append(filtered, declaration)
	}
	return filtered
}

func activeTextIndentNeedsReset(bodyDefaults map[string]bool) bool {
	if len(bodyDefaults) == 0 {
		return false
	}
	for declaration := range bodyDefaults {
		if strings.HasPrefix(declaration, "text-indent: ") {
			return declaration != "text-indent: 0"
		}
	}
	return false
}

func bodyDefaultsInclude(bodyDefaults map[string]bool, declaration string) bool {
	if len(bodyDefaults) == 0 {
		return false
	}
	return bodyDefaults[declaration]
}

func (r *storylineRenderer) applyAnnotations(text string, node map[string]interface{}) []htmlPart {
	annotations, ok := asSlice(node["style_events"])
	type event struct {
		start int
		end   int
		open  func(parent *htmlElement) *htmlElement
		close func(opened *htmlElement)
	}
	type activeEvent struct {
		event  event
		opened *htmlElement
	}
	runes := []rune(text)
	// Port of Python yj_to_epub_content.py:1117-1131:
	// Python's add_kfx_style merges the $157 style fragment properties INTO the content node,
	// so $125/$126 (dropcap_lines/dropcap_chars) become part of the node. Go doesn't merge —
	// it uses effectiveStyle() to compute the combined style lazily. So we must look up
	// $125/$126 from the effective style when not directly in the node.
	dropcapLinesVal, hasDropcapLines := asInt(node["dropcap_lines"])
	if !hasDropcapLines || dropcapLinesVal <= 0 {
		if styleID, _ := asString(node["style"]); styleID != "" {
			if frag := r.styleFragments[styleID]; frag != nil {
				dropcapLinesVal, hasDropcapLines = asInt(frag["dropcap_lines"])
			}
		}
	}
	dropcapCharsVal, hasDropcapChars := asInt(node["dropcap_chars"])
	if !hasDropcapChars || dropcapCharsVal <= 0 {
		if styleID, _ := asString(node["style"]); styleID != "" {
			if frag := r.styleFragments[styleID]; frag != nil {
				dropcapCharsVal, hasDropcapChars = asInt(frag["dropcap_chars"])
			}
		}
	}
	if hasDropcapLines && dropcapLinesVal > 0 {
		if hasDropcapChars && dropcapCharsVal > 0 {
			dropcap := map[string]interface{}{
				"offset": 0,
				"length": dropcapCharsVal,
				"dropcap_lines": dropcapLinesVal,
			}
			// Port of Python yj_to_epub_content.py:1129:
			//   if "style_name" in content: dropcap_style_event[IS("style_name")] = content["style_name"]
			// Copy $173 (kfx-style-name) from content node to dropcap event so
			// the CSS class name uses the style fragment's base name.
			if v := node["style_name"]; v != nil {
				dropcap["style_name"] = v
			} else if styleID, _ := asString(node["style"]); styleID != "" {
				if frag := r.styleFragments[styleID]; frag != nil {
					if v := frag["style_name"]; v != nil {
						dropcap["style_name"] = v
					}
				}
			}
			annotations = append([]interface{}{dropcap}, annotations...)
			ok = true
		}
	}
	events := make([]event, 0, len(annotations))
	if ok {
		for _, raw := range annotations {
			annotationMap, ok := asMap(raw)
			if !ok {
				continue
			}
			start, hasStart := asInt(annotationMap["offset"])
			length, hasLength := asInt(annotationMap["length"])
			if !hasStart || !hasLength || length <= 0 || start < 0 || start >= len(runes) {
				continue
			}
			end := start + length
			if end > len(runes) {
				end = len(runes)
			}
			anchorID, _ := asString(annotationMap["link_to"])
			styleID, _ := asString(annotationMap["style"])
			dropcapClass := ""
			dropcapLines := 0
			if l, ok := asInt(annotationMap["dropcap_lines"]); ok && l > 0 {
				dropcapLines = l
				dropcapClass = r.dropcapClass(l)
			}
			if debugAnchors := os.Getenv("KFX_DEBUG_ANCHORS"); debugAnchors != "" && anchorID != "" {
				for _, wanted := range strings.Split(debugAnchors, ",") {
					if strings.TrimSpace(wanted) == anchorID {
						fmt.Fprintf(os.Stderr, "annotation anchor=%s style=%s value=%#v\n", anchorID, styleID, annotationMap)
					}
				}
			}
			href := r.anchorHref(anchorID)
			rubyName, hasRubyName := asString(annotationMap["ruby_name"])
			if hasRubyName && rubyName != "" {
				rubyIDs := r.rubyAnnotationIDs(annotationMap, end-start)
				var rubyElement *htmlElement
				events = append(events, event{
					start: start,
					end:   end,
					open: func(parent *htmlElement) *htmlElement {
						rubyElement = &htmlElement{Tag: "ruby", Attrs: map[string]string{}}
						parent.Children = append(parent.Children, rubyElement)
						rb := &htmlElement{Tag: "rb", Attrs: map[string]string{}}
						rubyElement.Children = append(rubyElement.Children, rb)
						return rb
					},
					close: func(opened *htmlElement) {
						if opened == nil || rubyElement == nil {
							return
						}
						for _, rubyID := range rubyIDs {
							rt := &htmlElement{Tag: "rt", Attrs: map[string]string{}, Children: r.rubyContentParts(rubyName, rubyID)}
							rubyElement.Children = append(rubyElement.Children, rt)
						}
					},
				})
				continue
			}
			if href != "" {
				// Port of Python yj_to_epub_content.py:1142 — add_kfx_style merges style fragment
				// into annotation map, then process_content_properties uses the merged result.
				style := effectiveStyle(r.styleFragments[styleID], annotationMap)
				linkCSS := r.processContentProps(style, r.resolveResource)
				if _, hasColor := linkCSS["color"]; !hasColor {
					linkColor, hasLink := linkCSS["-kfx-link-color"]
					visitedColor, hasVisited := linkCSS["-kfx-visited-color"]
					if hasLink && hasVisited && linkColor == visitedColor {
						linkCSS["color"] = linkColor
					}
				}
				delete(linkCSS, "-kfx-link-color")
				delete(linkCSS, "-kfx-visited-color")
				linkDecls := cssDeclarationsFromMap(linkCSS)
				linkBaseName := "class"
				if styleID != "" {
					linkBaseName = r.styleBaseName(styleID)
				}
				styleAttr := mergeStyleStrings(
					styleStringFromDeclarations(linkBaseName, nil, linkDecls),
					dropcapClass,
				)
				// Port of Python yj_to_epub_content.py $616→epub:type on annotation links.
				// Python: $616=$617 → -kfx-attrib-epub-type: noteref → epub:type="noteref"
				epubType := epubTypeFromAnnotation(annotationMap)
				events = append(events, event{
					start: start,
					end:   end,
					open: func(parent *htmlElement) *htmlElement {
						attrs := map[string]string{"href": href}
						if styleAttr != "" {
							attrs["style"] = styleAttr
						}
						if epubType != "" {
							attrs["epub:type"] = epubType
						}
						element := &htmlElement{Tag: "a", Attrs: attrs}
						parent.Children = append(parent.Children, element)
						return element
					},
				})
				continue
			}
			// Port of Python yj_to_epub_content.py:1117-1131 — dropcap events are
			// separate from annotation style events. The dropcap wraps the first N
			// characters in an inner span with float/font-size/line-height/margin,
			// and the annotation wraps it in an outer span with annotation styles.
			// Python inserts the dropcap_style_event at position 0 in style_events,
			// then processes annotations separately. Here we create two nested events.
			annotationStyle := r.annotationSpanClass(styleID, annotationMap)
			if dropcapClass != "" {
				// Dropcap event: innermost span wrapping the first N characters.
				// Port of Python is_dropcap branch (yj_to_epub_content.py:1213-1230):
				//   1. add_style(event_elem, {float: left, font-size: Nem, ...})
				//   2. add_style(event_elem, process_content_properties(style_event), replace=True)
				// The dropcap element gets BOTH the float/font-size/margin AND the
				// processed content properties from the style fragment, using the
				// style fragment's base name for CSS class naming.
				//
				// Key: Python's add_kfx_style merges the $157 style fragment into the
				// style_event, so process_content_properties sees all properties. The
				// CSS class name comes from the $173 (-kfx-style-name) property which
				// is set from the parent node's $157 style fragment.
				// Since the synthetic dropcap event has no $157, we use the parent
				// node's styleID to compute the correct base name for CSS class naming.
				nodeStyleID, _ := asString(node["style"])
				dropcapBaseName := "class"
				if nodeStyleID != "" {
					dropcapBaseName = r.styleBaseName(nodeStyleID)
				}
				// Compute dropcap-specific styles with the correct base name
				dropcapDeclStyle := styleStringFromDeclarations(dropcapBaseName, nil, []string{
					"float: left",
					fmt.Sprintf("font-size: %dem", dropcapLines),
					"line-height: 100%",
					"margin-bottom: 0",
					"margin-right: 0.1em",
					"margin-top: 0",
				})
				dropcapSpanStyle := dropcapDeclStyle
				if annotationStyle != "" {
					// Merge annotation content properties with dropcap styles.
					// Python applies dropcap styles first, then process_content_properties
					// with replace=True (which can override). mergeStyleStrings handles this.
					dropcapSpanStyle = mergeStyleStrings(dropcapDeclStyle, annotationStyle)
				}
				events = append(events, event{
					start: start,
					end:   end,
					open: func(parent *htmlElement) *htmlElement {
						element := &htmlElement{Tag: "span", Attrs: map[string]string{"style": dropcapSpanStyle}}
						parent.Children = append(parent.Children, element)
						return element
					},
				})
			}
			if annotationStyle != "" {
				// Annotation event: outer wrapper with annotation CSS
				events = append(events, event{
					start: start,
					end:   end,
					open: func(parent *htmlElement) *htmlElement {
						element := &htmlElement{Tag: "span", Attrs: map[string]string{"style": annotationStyle}}
						parent.Children = append(parent.Children, element)
						return element
					},
				})
			}
		}
	}
	if len(events) == 0 {
		return splitTextHTMLParts(text)
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].start == events[j].start {
			return events[i].end > events[j].end
		}
		return events[i].start < events[j].start
	})
	root := &htmlElement{Attrs: map[string]string{}}
	stack := []*activeEvent{{opened: root}}
	last := 0
	for index, rch := range runes {
		if last < index {
			appendTextHTMLParts(stack[len(stack)-1].opened, string(runes[last:index]))
			last = index
		}
		for _, ev := range events {
			if ev.start == index {
				opened := ev.open(stack[len(stack)-1].opened)
				stack = append(stack, &activeEvent{event: ev, opened: opened})
			}
		}
		appendTextHTMLParts(stack[len(stack)-1].opened, string(rch))
		last = index + 1
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].end == index+1 {
				if len(stack) > 1 {
					active := stack[len(stack)-1]
					if active.event.close != nil {
						active.event.close(active.opened)
					}
					stack = stack[:len(stack)-1]
				}
			}
		}
	}
	if last < len(runes) {
		appendTextHTMLParts(stack[len(stack)-1].opened, string(runes[last:]))
	}

	// Port of Python add_content (yj_to_epub_content.py:364-370):
	// Python wraps ALL text in <span> elements via SubElement(parent, "span").
	// This ensures no bare text nodes exist in the tree, which is critical for
	// simplify_styles reverse inheritance (Python skips when elem.text or elem.tail
	// is set, line 1875-1876). During simplify_styles, empty spans are unwrapped
	// (matching etree.strip_tags in epub_output.py:783-789).
	for i, child := range root.Children {
		switch child.(type) {
		case htmlText, *htmlText:
			root.Children[i] = &htmlElement{
				Tag:      "span",
				Attrs:    map[string]string{},
				Children: []htmlPart{child},
			}
		}
	}

	return root.Children
}

func (r *storylineRenderer) rubyAnnotationIDs(annotationMap map[string]interface{}, eventLength int) []int {
	if annotationMap == nil {
		return nil
	}
	if rubyID, ok := asInt(annotationMap["ruby_id"]); ok {
		return []int{rubyID}
	}
	rawIDs, ok := asSlice(annotationMap["ruby_id_list"])
	if !ok {
		return nil
	}
	ids := make([]int, 0, len(rawIDs))
	for _, raw := range rawIDs {
		entry, ok := asMap(raw)
		if !ok {
			continue
		}
		if rubyID, ok := asInt(entry["ruby_id"]); ok {
			ids = append(ids, rubyID)
		}
	}
	return ids
}

func (r *storylineRenderer) rubyContentParts(rubyName string, rubyID int) []htmlPart {
	content := r.getRubyContent(rubyName, rubyID)
	if content == nil {
		return nil
	}
	if ref, ok := asMap(content["content"]); ok {
		if text := r.resolveText(ref); text != "" {
			return splitTextHTMLParts(text)
		}
	}
	if children, ok := asSlice(content["content_list"]); ok {
		parts := make([]htmlPart, 0, len(children))
		for _, child := range children {
			if rendered := r.renderInlinePart(child, 0); rendered != nil {
				parts = append(parts, rendered)
			}
		}
		return parts
	}
	return nil
}

func (r *storylineRenderer) getRubyContent(rubyName string, rubyID int) map[string]interface{} {
	group := r.rubyGroups[rubyName]
	if group == nil {
		return nil
	}
	children, _ := asSlice(group["content_list"])
	for _, raw := range children {
		switch typed := raw.(type) {
		case string:
			if content := r.rubyContents[typed]; content != nil {
				if id, ok := asInt(content["ruby_id"]); ok && id == rubyID {
					return cloneMap(content)
				}
			}
		default:
			entry, ok := asMap(raw)
			if !ok {
				continue
			}
			if id, ok := asInt(entry["ruby_id"]); ok && id == rubyID {
				return cloneMap(entry)
			}
		}
	}
	return nil
}

func (r *storylineRenderer) dropcapClass(lines int) string {
	if lines <= 0 {
		return ""
	}
	return styleStringFromDeclarations("class", nil, []string{
		"float: left",
		fmt.Sprintf("font-size: %dem", lines),
		"line-height: 100%",
		"margin-bottom: 0",
		"margin-right: 0.1em",
		"margin-top: 0",
	})
}

func (r *storylineRenderer) anchorIDForPosition(positionID int, offset int) string {
	offsets := r.positionAnchorID[positionID]
	if offsets == nil {
		return ""
	}
	return offsets[offset]
}

func (r *storylineRenderer) anchorOnlyMovable(positionID int, offset int) bool {
	offsets := r.positionAnchors[positionID]
	if offsets == nil {
		return false
	}
	names := offsets[offset]
	if len(names) == 0 {
		return false
	}
	for _, name := range names {
		if strings.HasPrefix(name, "$798_") {
			return false
		}
	}
	return true
}

func (r *storylineRenderer) applyPositionAnchors(element *htmlElement, positionID int, isFirstVisible bool) {
	if element == nil || positionID == 0 {
		return
	}
	if os.Getenv("KFX_DEBUG") != "" && (positionID == 1110 || positionID == 1111 || positionID == 1177 || positionID == 1178) {
		fmt.Fprintf(os.Stderr, "apply anchors pos=%d tag=%s first=%v raw=%v ids=%v\n", positionID, element.Tag, isFirstVisible, r.positionAnchors[positionID], r.positionAnchorID[positionID])
	}
	offsets := r.positionAnchors[positionID]
	if len(offsets) == 0 {
		return
	}
	if anchorID := r.anchorIDForPosition(positionID, 0); anchorID != "" {
		if !isFirstVisible && !strings.HasPrefix(anchorID, "id__212_") {
			element.Attrs["id"] = anchorID
			r.emittedAnchorIDs[anchorID] = true
			r.registerAnchorElementNames(positionID, 0, anchorID)
			if os.Getenv("KFX_DEBUG") != "" && (positionID == 1110 || positionID == 1111 || positionID == 1177 || positionID == 1178 || positionID == 1007 || positionID == 1053) {
				fmt.Fprintf(os.Stderr, "set id pos=%d tag=%s id=%s class=%s\n", positionID, element.Tag, anchorID, element.Attrs["class"])
			}
		}
	}
	ordered := make([]int, 0, len(offsets))
	for offset := range offsets {
		if offset > 0 {
			ordered = append(ordered, offset)
		}
	}
	sort.Ints(ordered)
	for _, offset := range ordered {
		anchorID := r.anchorIDForPosition(positionID, offset)
		if anchorID == "" {
			continue
		}
		target := locateOffset(element, offset)
		if target == nil {
			continue
		}
		// Port of Python position anchor behavior (yj_to_epub_navigation.py:375-400 +
		// yj_to_epub_content.py:1094-1098):
		// Python processes position anchors BEFORE style events/annotations, so the
		// anchor is placed on a raw text span. When annotations later wrap that span
		// via replace_element_with_container, the id stays on the inner element.
		// Go processes annotations FIRST (in applyAnnotations/processTextContent),
		// then position anchors. So when locateOffset finds an annotation-created
		// element (<a> with href, elements with epub:type), we must create a separate
		// empty <span id="X"/> as a sibling before it, matching Python's output.
		if needsSeparateAnchorSpan(target) {
			if span := insertAnchorSpanBefore(element, target); span != nil {
				span.Attrs = map[string]string{"id": anchorID}
				r.emittedAnchorIDs[anchorID] = true
				r.registerAnchorElementNames(positionID, offset, anchorID)
				continue
			}
		}
		if target.Attrs == nil {
			target.Attrs = map[string]string{}
		}
		if target.Attrs["id"] == "" {
			target.Attrs["id"] = anchorID
			r.emittedAnchorIDs[anchorID] = true
			r.registerAnchorElementNames(positionID, offset, anchorID)
		}
	}
}

// needsSeparateAnchorSpan returns true when the target element was created by
// annotation processing and should NOT receive the anchor id directly.
// Instead, a separate empty <span id="X"/> should be inserted before it.
// Port of Python's ordering: position anchors processed BEFORE annotations,
// so the anchor span exists as a separate element before <a> wrapping.
func needsSeparateAnchorSpan(target *htmlElement) bool {
	if target == nil {
		return false
	}
	// <a> elements with href are link annotations — always separate
	if target.Tag == "a" && target.Attrs["href"] != "" {
		return true
	}
	// Elements with epub:type attribute are annotation containers
	if target.Attrs["epub:type"] != "" {
		return true
	}
	return false
}

// insertAnchorSpanBefore inserts an empty <span/> as a sibling before target
// by searching the element tree starting from root. Returns the new span,
// or nil if target can't be found.
func insertAnchorSpanBefore(root *htmlElement, target *htmlElement) *htmlElement {
	if root == nil || target == nil {
		return nil
	}
	span := &htmlElement{Tag: "span", Attrs: map[string]string{}}
	if didInsert := insertSpanBeforeInTree(root, target, span); didInsert {
		return span
	}
	return nil
}

// insertSpanBeforeInTree recursively searches root's children for target,
// and inserts span as a sibling before it. Returns true if inserted.
func insertSpanBeforeInTree(parent *htmlElement, target *htmlElement, span *htmlElement) bool {
	for i, child := range parent.Children {
		if child == target {
			parent.Children = append(parent.Children[:i], append([]htmlPart{span}, parent.Children[i:]...)...)
			return true
		}
		if ce, ok := child.(*htmlElement); ok {
			if insertSpanBeforeInTree(ce, target, span) {
				return true
			}
		}
	}
	return false
}

func (r *storylineRenderer) registerAnchorElementNames(positionID int, offset int, anchorID string) {
	if r == nil || anchorID == "" {
		return
	}
	offsets := r.positionAnchors[positionID]
	if offsets == nil {
		return
	}
	names := offsets[offset]
	if len(names) == 0 {
		return
	}
	if r.anchorNamesByID == nil {
		r.anchorNamesByID = map[string][]string{}
	}
	seen := map[string]bool{}
	for _, existing := range r.anchorNamesByID[anchorID] {
		seen[existing] = true
	}
	for _, name := range names {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		r.anchorNamesByID[anchorID] = append(r.anchorNamesByID[anchorID], name)
	}
}

func (r *storylineRenderer) headingLevelForPosition(positionID int, offset int) int {
	if r == nil || positionID == 0 || r.anchorHeadingLevel == nil {
		return 0
	}
	offsets := r.positionAnchors[positionID]
	if offsets == nil {
		return 0
	}
	for _, name := range offsets[offset] {
		if level := r.anchorHeadingLevel[name]; level > 0 {
			return level
		}
	}
	return 0
}

func (r *storylineRenderer) consumeVisibleElement() bool {
	isFirst := !r.firstVisibleSeen
	r.firstVisibleSeen = true
	return isFirst
}


// ---------------------------------------------------------------------------
// Merged from render.go (origin: yj_to_epub_content.py / epub_output.py)
// ---------------------------------------------------------------------------

func renderedSectionBodyHTML(section renderedSection) string {
	if section.Root == nil {
		return ""
	}
	return renderHTMLParts(section.Root.Children, true)
}

func replaceSectionDOMClassTokens(section *renderedSection, replacer *strings.Replacer) {
	if section == nil || section.Root == nil || replacer == nil {
		return
	}
	replaceHTMLClassTokens(section.Root, replacer)
}

func replaceHTMLClassTokens(element *htmlElement, replacer *strings.Replacer) {
	if element == nil || replacer == nil {
		return
	}
	if element.Attrs != nil {
		if className := element.Attrs["class"]; className != "" {
			element.Attrs["class"] = replacer.Replace(className)
		}
	}
	for _, child := range element.Children {
		if childElement, ok := child.(*htmlElement); ok {
			replaceHTMLClassTokens(childElement, replacer)
		}
	}
}

func materializeRenderedSections(rendered []renderedSection) []epub.Section {
	sections := make([]epub.Section, 0, len(rendered))
	for _, section := range rendered {
		sections = append(sections, epub.Section{
			Filename:    section.Filename,
			Title:       section.Title,
			PageTitle:   section.PageTitle,
			Language:    section.Language,
			BodyLanguage: section.BodyLanguage,
			BodyClass:   section.BodyClass,
			Paragraphs:  append([]string(nil), section.Paragraphs...),
			BodyHTML:    renderedSectionBodyHTML(section),
			Properties:  section.Properties,
		})
	}
	return sections
}

func cleanupRenderedSections(sections []renderedSection) {
	for index := range sections {
		if sections[index].Root == nil {
			continue
		}
		sections[index].Root.Children = cleanupHTMLParts(sections[index].Root.Children)
	}
}

func cleanupHTMLParts(parts []htmlPart) []htmlPart {
	cleaned := make([]htmlPart, 0, len(parts))
	for _, part := range parts {
		switch typed := part.(type) {
		case *htmlElement:
			typed.Children = cleanupHTMLParts(typed.Children)
			if isEmptyWrapper(typed) {
				continue
			}
			if shouldCollapseNestedDiv(typed) {
				cleaned = append(cleaned, typed.Children[0])
				continue
			}
			cleaned = append(cleaned, typed)
		default:
			cleaned = append(cleaned, part)
		}
	}
	return cleaned
}

func isEmptyWrapper(element *htmlElement) bool {
	if element == nil {
		return true
	}
	if element.Tag != "span" || len(element.Attrs) > 0 {
		return false
	}
	return len(element.Children) == 0
}

// isInlineOnly checks if an element tree contains only inline elements.
// Ported from Python yj_to_epub_content.py L1922-1935 (is_inline_only).
// Inline elements: a, audio, img, rb, rt, ruby, span, svg, video.
// SVG returns true unconditionally (Python L1924: if elem.tag == SVG: return True).
// Used in render:inline path to decide whether to keep <span> or revert to <div>.
func isInlineOnly(element *htmlElement) bool {
	if element == nil {
		return true
	}
	// Python L1924: if elem.tag == SVG: return True
	if element.Tag == "svg" {
		return true
	}
	// Python L1927: if elem.tag not in {"a","audio","img","rb","rt","ruby","span",SVG,"video"}: return False
	switch element.Tag {
	case "a", "audio", "img", "rb", "rt", "ruby", "span", "video":
		// ok, inline
	default:
		return false
	}
	// Python L1930-1932: for e in elem: if not self.is_inline_only(e): return False
	for _, child := range element.Children {
		childElem, ok := child.(*htmlElement)
		if !ok {
			// htmlText nodes are inline
			continue
		}
		if !isInlineOnly(childElem) {
			return false
		}
	}
	// Python L1934: return True
	return true
}

func shouldCollapseNestedDiv(element *htmlElement) bool {
	if element == nil || element.Tag != "div" || len(element.Attrs) > 0 || len(element.Children) != 1 {
		return false
	}
	child, ok := element.Children[0].(*htmlElement)
	if !ok || child == nil || child.Tag != "div" || len(child.Attrs) > 0 {
		return false
	}
	return true
}
