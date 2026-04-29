// Navigation, TOC, guide, and page-list handling ported from Calibre
// REFERENCE/Calibre_KFX_Input/kfxlib/yj_to_epub_navigation.py (KFX_EPUB_Navigation).
package kfx

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/epub"
)

func resolveNavigationContainer(value interface{}, navContainers map[string]map[string]interface{}) map[string]interface{} {
	if id, ok := asString(value); ok && id != "" {
		return navContainers[id]
	}
	container, _ := asMap(value)
	return container
}

func resolveNavigationUnit(value interface{}) map[string]interface{} {
	unit, _ := asMap(value)
	return unit
}

func collectNavigationContainers(navRoots []map[string]interface{}, navContainers map[string]map[string]interface{}) []map[string]interface{} {
	var containers []map[string]interface{}
	for _, root := range navRoots {
		entries, ok := asSlice(root["nav_containers"])
		if !ok {
			continue
		}
		for _, entry := range entries {
			if container := resolveNavigationContainer(entry, navContainers); container != nil {
				containers = append(containers, container)
			}
		}
	}
	return containers
}

func navigationType(value map[string]interface{}) string {
	navType, _ := asString(value["nav_type"])
	return navType
}

// navTypeToSymbolID converts a human-readable nav type name back to its ION $N symbol ID.
// Python uses raw $N symbols as nav types; Go uses real names from the ION catalog.
// Anchor names must match Calibre's format exactly (e.g. "$798_0_Chapter", not "headings_0_Chapter").
func navTypeToSymbolID(navType string) string {
	switch navType {
	case "toc":
		return "$212"
	case "scrubbers":
		return "$213"
	case "thumbnails":
		return "$214"
	case "landmarks":
		return "$236"
	case "page_list":
		return "$237"
	case "headings":
		return "$798"
	default:
		return navType
	}
}

// parseNavRepresentation extracts (label, icon, description) from a nav unit.
// Port of Python get_representation (yj_to_epub_navigation.py L247-271).
// Python extracts: $245→icon+label, $146→description (content list → text), $244→label.
func parseNavRepresentation(entry map[string]interface{}) (label string, icon string, description string) {
	representation, ok := asMap(entry["representation"])
	if !ok {
		return "", "", ""
	}

	// Python: if "icon" in representation: icon = representation.pop("icon"); label = str(icon)
	if iconRaw, ok := asMap(representation["icon"]); ok {
		// Icon is a resource reference; the label is str(icon).
		// For now, we extract the resource ID for later resolution.
		if resourceID, ok := asString(iconRaw["resource_name"]); ok {
			icon = resourceID
		}
		if icon != "" {
			label = icon // Python: label = str(icon)
		}
	}

	// Python: if "content_list" in representation: process content list → text → description
	if descRaw, ok := asSlice(representation["content_list"]); ok && len(descRaw) > 0 {
		// Python (yj_to_epub_navigation.py L303-306) builds a div element, calls process_content_list,
		// then extracts text via itertext(). Here we extract string content directly.
		// Navigation descriptions are simple text strings in practice, so the full
		// content rendering pipeline is not needed for correct EPUB output.
		var textParts []string
		for _, item := range descRaw {
			if s, ok := asString(item); ok {
				textParts = append(textParts, s)
			}
		}
		if len(textParts) > 0 {
			description = strings.TrimSpace(strings.Join(textParts, ""))
		}
	}

	// Python: if "label" in representation: label = representation.pop("label")
	if textLabel, ok := asString(representation["label"]); ok {
		label = textLabel
	}

	if label != "" {
		label = strings.TrimSpace(label)
	}

	return label, icon, description
}

// parseNavTitle extracts just the title/label from a nav unit entry.
// This is a convenience wrapper around parseNavRepresentation for cases
// where only the label is needed.
func parseNavTitle(value map[string]interface{}) string {
	label, _, _ := parseNavRepresentation(value)
	return label
}

func parseNavTarget(value map[string]interface{}) navTarget {
	target, ok := asMap(value["target_position"])
	if !ok {
		return navTarget{}
	}
	positionID, _ := asInt(target["id"])
	if positionID == 0 {
		positionID, _ = asInt(target["kfx_id"])
	}
	offset, _ := asInt(target["offset"])
	return navTarget{PositionID: positionID, Offset: offset}
}

func countNavPoints(points []navPoint) int {
	count := 0
	for _, point := range points {
		count++
		count += countNavPoints(point.Children)
	}
	return count
}

func flattenNavigationTitles(points []navPoint, positionToSection map[int]string, titles map[string]string) {
	for _, point := range points {
		if sectionID, ok := positionToSection[point.Target.PositionID]; ok && titles[sectionID] == "" && point.Title != "" {
			titles[sectionID] = point.Title
		}
		flattenNavigationTitles(point.Children, positionToSection, titles)
	}
}

func orderedSectionIDsFromNavigation(points []navPoint, positionToSection map[int]string) []string {
	var ordered []string
	var walk func(items []navPoint)
	walk = func(items []navPoint) {
		for _, point := range items {
			if sectionID, ok := positionToSection[point.Target.PositionID]; ok {
				ordered = append(ordered, sectionID)
			}
			walk(point.Children)
		}
	}
	walk(points)
	return ordered
}

func navigationToEPUB(points []navPoint, targetHref func(navTarget) string, iconHref func(string) string) []epub.NavPoint {
	output := make([]epub.NavPoint, 0, len(points))
	for _, point := range points {
		href := targetHref(point.Target)
		if href == "" || point.Title == "" {
			continue
		}
		epubIcon := ""
		if point.Icon != "" && iconHref != nil {
			epubIcon = iconHref(point.Icon)
		}
		output = append(output, epub.NavPoint{
			Title:       point.Title,
			Href:        href,
			Children:    navigationToEPUB(point.Children, targetHref, iconHref),
			Description: point.Description,
			Icon:        epubIcon,
		})
	}
	return output
}

type navProcessor struct {
	tocEntryCount      int
	usedAnchorNames    map[string]bool
	positionAnchors    map[int]map[int][]string
	// anchorSites records distinct (positionID, offset) pairs per anchor name (Python anchor_positions).
	anchorSites        map[string]map[string]struct{}
	anchorHeadingLevel map[string]int
	navContainers      map[string]map[string]interface{}
	toc                []navPoint
	guide              []guideEntry
	pages              []pageEntry
	pageLabelAnchorID  map[string]string // label → anchor_id (Python page_label_anchor_id)
	orientationLock       string // Python: self.orientation_lock — used for $248 entry_set filtering
	approximatePagesRemoved bool  // Python: self.approximate_pages_removed (yj_to_epub_navigation.py L38)
}

func processNavigation(navRoots []map[string]interface{}, navContainers map[string]map[string]interface{}, orientationLock string, readingOrderNames []string, isScribeNotebook bool) navProcessor {
	state := navProcessor{
		usedAnchorNames:    map[string]bool{},
		positionAnchors:    map[int]map[int][]string{},
		anchorSites:        map[string]map[string]struct{}{},
		anchorHeadingLevel: map[string]int{},
		navContainers:      navContainers,
		pageLabelAnchorID:  map[string]string{},
		orientationLock:    orientationLock,
	}

	// Port of Python process_navigation L101-102 for-else pattern (N1 gap fix):
	// Warn when a reading order has no matching navigation data in the nav roots.
	warnUnmatchedReadingOrders(navRoots, readingOrderNames, isScribeNotebook)

	containers := collectNavigationContainers(navRoots, navContainers)
	hasNavHeadings := false
	for _, container := range containers {
		if navigationType(container) == "headings" {
			hasNavHeadings = true
			break
		}
	}
	for _, container := range containers {
		state.processContainer(container, hasNavHeadings)
	}
	return state
}

// warnUnmatchedReadingOrders logs a warning for any reading order that has no matching
// navigation data in the nav roots. Port of Python process_navigation (yj_to_epub_navigation.py L97-102):
//
//	for reading_order in self.reading_orders:
//	    for i, book_navigation in enumerate(book_navigations):
//	        if book_navigation.get("$178", "") == reading_order_name:
//	            break
//	    else:
//	        if not self.book.is_scribe_notebook:
//	            log.warning("Failed to locate navigation for reading order \"%s\"" % reading_order_name)
func warnUnmatchedReadingOrders(navRoots []map[string]interface{}, readingOrderNames []string, isScribeNotebook bool) {
	if isScribeNotebook || len(readingOrderNames) == 0 {
		return
	}

	// Build set of reading_order_name values present in nav roots.
	navRootNames := map[string]bool{}
	for _, root := range navRoots {
		if name, ok := asString(root["reading_order_name"]); ok && name != "" {
			navRootNames[name] = true
		}
	}

	for _, roName := range readingOrderNames {
		if roName == "" {
			continue
		}
		if !navRootNames[roName] {
			log.Printf("kfx: warning: Failed to locate navigation for reading order %q", roName)
		}
	}
}

func (p *navProcessor) processContainer(container map[string]interface{}, hasNavHeadings bool) {
	navType := navigationType(container)
	// Port of Python L125-126: log error for unknown nav types.
	switch navType {
	case "toc", "scrubbers", "thumbnails", "landmarks", "page_list", "headings":
		// known type
	default:
		log.Printf("kfx: error: nav_container has unknown type: %s", navType)
	}
	if imports, ok := asSlice(container["imports"]); ok {
		for _, raw := range imports {
			if imported := resolveNavigationContainer(raw, p.navContainers); imported != nil {
				p.processContainer(imported, hasNavHeadings)
			}
		}
	}
	entries, ok := asSlice(container["entries"])
	if !ok {
		return
	}
	for _, raw := range entries {
		entry := resolveNavigationUnit(raw)
		if entry == nil {
			continue
		}
		switch navType {
		case "toc", "scrubbers", "thumbnails", "headings":
			p.processNavUnit(navType, entry, &p.toc, navType == "toc" && !hasNavHeadings, nil)
		case "landmarks":
			p.processGuideUnit(entry)
		case "page_list":
			navContainerName, _ := asString(container["$239"])
			p.processPageUnit(entry, navContainerName)
		}
	}
}

// processGuideUnit handles nav units from a landmarks ($236) nav container.
// Port of Python process_nav_container landmarks branch (yj_to_epub_navigation.py L143-165).
// Python only registers guide entries when landmark_type is set.
func (p *navProcessor) processGuideUnit(entry map[string]interface{}) {
	label := parseNavTitle(entry)
	target := parseNavTarget(entry)
	if target.PositionID == 0 {
		return
	}
	navUnitName, _ := asString(entry["nav_unit_name"])
	if navUnitName == "" {
		navUnitName = label
	}
	targetPosition := parseNavTarget(entry)
	landmarkType := asStringDefault(entry["landmark_type"])

	// Port of Python: if landmark_type: ... (yj_to_epub_navigation.py L148-165)
	// Python only processes guide entries when landmark_type is present.
	if landmarkType != "" {
		guideType := guideTypeForLandmark(landmarkType)
		// Port of Python: self.unique_anchor_name(str(nav_unit_name) or guide_type)
		// Python's "or" returns nav_unit_name if truthy, else guide_type.
		nameForAnchor := navUnitName
		if nameForAnchor == "" {
			nameForAnchor = guideType
		}
		anchorName := p.uniqueAnchorName(nameForAnchor)
		p.registerAnchor(anchorName, targetPosition, nil)
		if label == "cover-nav-unit" {
			label = ""
		}
		p.guide = append(p.guide, guideEntry{Type: guideType, Title: label, Target: target})
	}
}

// processPageUnit handles nav units from a page_list ($237) nav container.
// Port of Python process_nav_container page_list branch (yj_to_epub_navigation.py L167-198).
func (p *navProcessor) processPageUnit(entry map[string]interface{}, navContainerName string) {
	label := parseNavTitle(entry)
	if debug := os.Getenv("KFX_DEBUG_PAGES"); debug != "" {
		fmt.Fprintf(os.Stderr, "page unit label=%q entry=%#v\n", label, entry)
	}
	navUnitName, _ := asString(entry["nav_unit_name"])
	if navUnitName == "" {
		navUnitName = "page_list_entry"
	}
	// Port of Python L175-176: if nav_unit_name != "page_list_entry": log.warning(...)
	if navUnitName != "page_list_entry" {
		log.Printf("kfx: warning: Unexpected page_list nav_unit_name: %s", navUnitName)
	}
	// Port of Python approximate page list handling (yj_to_epub_navigation.py L168-171):
	// Skip page entries from APPROXIMATE_PAGE_LIST containers.
	// Python: if nav_container_name == APPROXIMATE_PAGE_LIST and not KEEP_APPROX_PG_NUMS:
	if navContainerName == "APPROXIMATE_PAGE_LIST" {
		if !p.approximatePagesRemoved {
			log.Printf("kfx: warning: Removing approximate page numbers previously produced by KFX Output")
			p.approximatePagesRemoved = true
		}
		return
	}

	if label == "" {
		return
	}
	target := parseNavTarget(entry)
	if target.PositionID == 0 {
		return
	}
	anchorName := p.uniqueAnchorName("page_" + label)
	p.registerAnchor(anchorName, target, nil)
	// Port of Python page deduplication (yj_to_epub_navigation.py L183-193):
	// anchor_id is position-based; if same label already maps to this anchor_id, skip.
	anchorID := fmt.Sprintf("%d.%d", target.PositionID, target.Offset)
	if p.pageLabelAnchorID[label] == anchorID {
		return
	}
	p.pageLabelAnchorID[label] = anchorID
	p.pages = append(p.pages, pageEntry{Label: label, Target: target})
}

func (p *navProcessor) processNavUnit(navType string, entry map[string]interface{}, out *[]navPoint, defaultHeading bool, headingLevel *int) {
	// Port of Python process_nav_unit (yj_to_epub_navigation.py L176-227).
	// Extract label, icon, and description from representation.
	label, icon, description := parseNavRepresentation(entry)
	if label != "" {
		label = strings.TrimSpace(label)
	}

	// Python: desc = nav_unit.pop("description", None); if desc: description = desc.strip()
	if desc, ok := asString(entry["description"]); ok && desc != "" {
		description = strings.TrimSpace(desc)
	}

	navUnitName, _ := asString(entry["nav_unit_name"])
	if navUnitName == "" {
		navUnitName = label
	}
	nextHeading := (*int)(nil)
	if navType == "headings" {
		landmarkType := asStringDefault(entry["landmark_type"])
		if landmarkType != "" {
			if level, ok := headingLevelForLandmark(landmarkType); ok {
				headingLevel = intPtr(level)
				nextHeading = intPtr(level)
			} else {
				// Port of Python L214-215: log.error and set heading_level = None
				log.Printf("kfx: error: Unexpected headings landmark_type: %s", landmarkType)
				headingLevel = nil
				nextHeading = nil
			}
		}
		if label == "heading-nav-unit" {
			label = ""
		}
		if navUnitName == "heading-nav-unit" {
			navUnitName = ""
		}
	} else if defaultHeading {
		if headingLevel == nil {
			headingLevel = intPtr(1)
		}
		nextHeading = intPtr(2)
	} else if headingLevel != nil && *headingLevel < 6 {
		nextHeading = intPtr(*headingLevel + 1)
	}

	// Process child nav units from $247
	childrenRaw, _ := asSlice(entry["entries"])
	children := make([]navPoint, 0, len(childrenRaw))
	for _, raw := range childrenRaw {
		child := resolveNavigationUnit(raw)
		if child == nil {
			continue
		}
		p.processNavUnit(navType, child, &children, false, nextHeading)
	}

	// Port of Python $248 entry_set handling (yj_to_epub_navigation.py L209-225).
	// Each entry_set contains $247 children and an $215 orientation filter.
	// Orientation $386 = portrait-only (clear if not landscape lock)
	// Orientation $385 = landscape-only (clear if landscape lock)
	entrySets, _ := asSlice(entry["entry_set"])
	for _, esRaw := range entrySets {
		entrySet, ok := asMap(esRaw)
		if !ok {
			continue
		}
		// Process children within entry set
		esChildren, _ := asSlice(entrySet["entries"])
		for _, raw := range esChildren {
			child := resolveNavigationUnit(raw)
			if child == nil {
				continue
			}
			p.processNavUnit(navType, child, &children, false, nextHeading)
		}

		// Orientation filtering (Python L218-224)
		orientation, _ := asString(entrySet["orientation"])
		switch orientation {
		case "landscape":
			// Portrait: clear children unless locked to landscape
			// Python: if self.orientation_lock != "landscape": nested_toc = []
			if p.orientationLock != "landscape" {
				children = nil
			}
		case "portrait":
			// Landscape: clear children when locked to landscape
			// Python: if self.orientation_lock == "landscape": nested_toc = []
			if p.orientationLock == "landscape" {
				children = nil
			}
		default:
			if orientation != "" {
				log.Printf("kfx: error: Unknown entry set orientation: %s", orientation)
			}
		}
	}

	target := parseNavTarget(entry)
	hasTarget := target.PositionID != 0
	if hasTarget {
		// Python uses the raw $N symbol ID as the anchor prefix (e.g. "$798_0_Chapter").
		// Go uses real names from the ION catalog, but anchor names must match Calibre exactly.
		anchorName := fmt.Sprintf("%s_%d_%s", navTypeToSymbolID(navType), p.tocEntryCount, navUnitName)
		p.tocEntryCount++
		p.registerAnchor(anchorName, target, headingLevel)
	}
	if navType == "headings" {
		return
	}
	if label == "" && !hasTarget {
		*out = append(*out, children...)
		return
	}
	*out = append(*out, navPoint{
		Title:       label,
		Target:      target,
		Children:    children,
		Description: description,
		Icon:        icon,
	})
}

func (p *navProcessor) uniqueAnchorName(name string) string {
	if name == "" {
		return ""
	}
	if !p.usedAnchorNames[name] {
		p.usedAnchorNames[name] = true
		return name
	}
	for index := 0; ; index++ {
		candidate := fmt.Sprintf("%s:%d", name, index)
		if !p.usedAnchorNames[candidate] {
			p.usedAnchorNames[candidate] = true
			return candidate
		}
	}
}

func (p *navProcessor) registerAnchor(name string, target navTarget, headingLevel *int) {
	if name == "" || target.PositionID == 0 {
		return
	}
	siteKey := fmt.Sprintf("%d.%d", target.PositionID, target.Offset)
	if p.anchorSites[name] == nil {
		p.anchorSites[name] = map[string]struct{}{}
	}
	p.anchorSites[name][siteKey] = struct{}{}

	offsets := p.positionAnchors[target.PositionID]
	if offsets == nil {
		offsets = map[int][]string{}
		p.positionAnchors[target.PositionID] = offsets
	}
	offsets[target.Offset] = append(offsets[target.Offset], name)
	if headingLevel != nil && *headingLevel > 0 {
		p.anchorHeadingLevel[name] = *headingLevel
	}
}

func guideTypeForLandmark(value string) string {
	switch value {
	case "cover_page":
		return "cover"
	case "srl", "text":
		return "text"
	case "toc":
		return "toc"
	default:
		return value
	}
}

func headingLevelForLandmark(value string) (int, bool) {
	switch value {
	case "h1":
		return 1, true
	case "h2":
		return 2, true
	case "h3":
		return 3, true
	case "h4":
		return 4, true
	case "h5":
		return 5, true
	case "h6":
		return 6, true
	default:
		return 0, false
	}
}

func guideToEPUB(entries []guideEntry, targetHref func(navTarget) string) []epub.GuideEntry {
	out := make([]epub.GuideEntry, 0, len(entries))
	for _, entry := range entries {
		// Port of Python epub_output.py guide filtering: cover, text, toc are standard guide types.
		if entry.Type != "cover" && entry.Type != "text" && entry.Type != "toc" {
			continue
		}
		href := targetHref(entry.Target)
		if href == "" {
			continue
		}
		title := entry.Title
		if title == "" {
			// Port of Python DEFAULT_LABEL_OF_GUIDE_TYPE.
			switch entry.Type {
			case "cover":
				title = "Cover"
			case "text":
				title = "Beginning"
			case "toc":
				title = "Table of Contents"
			default:
				title = strings.Title(entry.Type)
			}
		}
		out = append(out, epub.GuideEntry{Type: entry.Type, Title: title, Href: href})
	}
	return out
}

func pagesToEPUB(entries []pageEntry, targetHref func(navTarget) string) []epub.PageTarget {
	out := make([]epub.PageTarget, 0, len(entries))
	for _, entry := range entries {
		href := targetHref(entry.Target)
		if href == "" {
			continue
		}
		out = append(out, epub.PageTarget{Label: entry.Label, Href: href})
	}
	return out
}

func buildPositionAnchorIDs(positionAnchors map[int]map[int][]string) map[int]map[int]string {
	seen := map[string]bool{}
	result := map[int]map[int]string{}
	positionIDs := make([]int, 0, len(positionAnchors))
	for positionID := range positionAnchors {
		positionIDs = append(positionIDs, positionID)
	}
	sort.Ints(positionIDs)
	for _, positionID := range positionIDs {
		offsets := positionAnchors[positionID]
		offsetIDs := make([]int, 0, len(offsets))
		for offset := range offsets {
			offsetIDs = append(offsetIDs, offset)
		}
		sort.Ints(offsetIDs)
		result[positionID] = map[int]string{}
		for _, offset := range offsetIDs {
			names := offsets[offset]
			if len(names) == 0 {
				continue
			}
			id := makeUniqueHTMLID(names[0], seen)
			seen[id] = true
			result[positionID][offset] = id
		}
	}
	return result
}

func makeUniqueHTMLID(name string, seen map[string]bool) string {
	base := fixHTMLID(name)
	if !seen[base] {
		return base
	}
	for index := 0; ; index++ {
		candidate := fmt.Sprintf("%s%d", base, index)
		if !seen[candidate] {
			return candidate
		}
	}
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

func mergeSectionOrder(primary []string, fallback []string) []string {
	seen := map[string]bool{}
	merged := make([]string, 0, len(primary)+len(fallback))
	for _, sectionID := range primary {
		if sectionID == "" || seen[sectionID] {
			continue
		}
		seen[sectionID] = true
		merged = append(merged, sectionID)
	}
	for _, sectionID := range fallback {
		if sectionID == "" || seen[sectionID] {
			continue
		}
		seen[sectionID] = true
		merged = append(merged, sectionID)
	}
	return merged
}

// Port of KFX_EPUB_Navigation.fixup_anchors_and_hrefs (yj_to_epub_navigation.py L452+).
func fixupAnchorsAndHrefs(sections []renderedSection, resolved map[string]string) {
	replaceRenderedAnchorPlaceholders(sections, resolved)
}

// Port of KFX_EPUB_Navigation.report_missing_positions (yj_to_epub_navigation.py L353-361).
// Logs error for each unresolved position (remaining entries in positionAnchors after processing).
// Formats as "EID.OFFSET", sorts them, and truncates list at 10 items (matching Python's truncate_list).
func reportMissingPositions(positionAnchors map[int]map[int][]string) {
	if len(positionAnchors) == 0 {
		return
	}
	var pos []string
	for eid, offsets := range positionAnchors {
		for offset := range offsets {
			pos = append(pos, fmt.Sprintf("%d.%d", eid, offset))
		}
	}
	if len(pos) == 0 {
		return
	}
	sort.Sort(sortablePositionStrings(pos))
	truncated := truncatePositionList(pos)
	log.Printf("kfx: error: Failed to locate %d referenced positions: %s", len(pos), strings.Join(truncated, ", "))
}

// Port of KFX_EPUB_Navigation.report_duplicate_anchors (yj_to_epub_navigation.py L431-440).
// Logs error for anchors that are BOTH used (present in usedAnchors) AND have multiple positions.
func reportDuplicateAnchors(anchorPositions map[string]map[string]struct{}, usedAnchors map[string]bool) {
	if len(anchorPositions) == 0 {
		return
	}
	for anchorName, positions := range anchorPositions {
		if !usedAnchors[anchorName] {
			continue
		}
		if len(positions) <= 1 {
			continue
		}
		keys := make([]string, 0, len(positions))
		for k := range positions {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		log.Printf("kfx: error: Anchor %s has multiple positions: %s", anchorName, strings.Join(keys, ", "))
	}
}

// truncatePositionList truncates a list to maxAllowed items, appending a "... (N total)" summary.
// Port of Python utilities.truncate_list (utilities.py L159-160).
func truncatePositionList(lst []string) []string {
	const maxAllowed = 10
	if len(lst) <= maxAllowed {
		return lst
	}
	result := make([]string, maxAllowed+1)
	copy(result[:maxAllowed], lst[:maxAllowed])
	result[maxAllowed] = fmt.Sprintf("... (%d total)", len(lst))
	return result
}

// sortablePositionStrings implements sort.Interface for position strings like "42.5".
// Python sorts these as plain strings (which is correct for formatted "EID.OFFSET" strings).
type sortablePositionStrings []string

func (s sortablePositionStrings) Len() int           { return len(s) }
func (s sortablePositionStrings) Less(i, j int) bool { return s[i] < s[j] }
func (s sortablePositionStrings) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// ---------------------------------------------------------------------------
// Merged from render.go (origin: yj_to_epub_navigation.py anchor handling)
// ---------------------------------------------------------------------------

func registerNamedPositionAnchor(positionAnchors map[int]map[int][]string, name string, target navTarget) {
	if name == "" || target.PositionID == 0 {
		return
	}
	offsets := positionAnchors[target.PositionID]
	if offsets == nil {
		offsets = map[int][]string{}
		positionAnchors[target.PositionID] = offsets
	}
	names := offsets[target.Offset]
	for _, existing := range names {
		if existing == name {
			return
		}
	}
	offsets[target.Offset] = append([]string{name}, names...)
}

func firstAnchorNameForPosition(positionAnchors map[int]map[int][]string, positionID int, offset int) string {
	offsets := positionAnchors[positionID]
	if offsets == nil {
		return ""
	}
	names := offsets[offset]
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func resolveRenderedAnchorURIs(sections []renderedSection, renderer *storylineRenderer) map[string]string {
	resolved := map[string]string{}
	if renderer != nil {
		for name, uri := range renderer.directAnchorURI {
			if uri != "" {
				resolved[name] = uri
			}
		}
		for name, uri := range renderer.fallbackAnchorURI {
			if uri != "" && resolved[name] == "" {
				resolved[name] = uri
			}
		}
	}
	for _, section := range sections {
		if section.Root == nil {
			continue
		}
		visibleSeen := false
		resolveAnchorURIsInParts(section.Root.Children, section.Filename, renderer, resolved, &visibleSeen)
	}
	return resolved
}

func resolveAnchorURIsInParts(parts []htmlPart, filename string, renderer *storylineRenderer, resolved map[string]string, visibleSeen *bool) {
	for _, part := range parts {
		switch typed := part.(type) {
		case htmlText:
			if strings.TrimSpace(typed.Text) != "" {
				*visibleSeen = true
			}
		case *htmlText:
			if typed != nil && strings.TrimSpace(typed.Text) != "" {
				*visibleSeen = true
			}
		case *htmlElement:
			if typed == nil {
				continue
			}
			resolveAnchorURIsForElement(typed, filename, renderer, resolved, *visibleSeen)
			if elementCountsAsVisible(typed) {
				*visibleSeen = true
			}
			resolveAnchorURIsInParts(typed.Children, filename, renderer, resolved, visibleSeen)
		}
	}
}

func resolveAnchorURIsForElement(element *htmlElement, filename string, renderer *storylineRenderer, resolved map[string]string, visibleBefore bool) {
	if element == nil || renderer == nil {
		return
	}
	anchorID := element.Attrs["id"]
	if anchorID == "" {
		return
	}
	names := renderer.anchorNamesByID[anchorID]
	if len(names) == 0 {
		return
	}
	defaultURI := filename + "#" + anchorID
	allMovable := true
	anyMovable := false
	for _, name := range names {
		movable := !strings.HasPrefix(name, "$798_")
		if movable {
			anyMovable = true
		} else {
			allMovable = false
		}
		resolved[name] = defaultURI
	}
	if !visibleBefore && anyMovable {
		for _, name := range names {
			if !strings.HasPrefix(name, "$798_") {
				resolved[name] = filename
			}
		}
		if allMovable {
			delete(element.Attrs, "id")
		}
	}
}

func elementCountsAsVisible(element *htmlElement) bool {
	if element == nil {
		return false
	}
	switch element.Tag {
	case "img", "svg", "audio", "video", "object", "iframe", "br":
		return true
	}
	return false
}

func replaceRenderedAnchorPlaceholders(sections []renderedSection, resolved map[string]string) {
	for index := range sections {
		if sections[index].Root == nil {
			continue
		}
		// Port of Python fixup_anchors_and_hrefs elem_id check (yj_to_epub_navigation.py L459-461):
		// If the section root (body) has no ID, generate one from the filename.
		// Python: elem_id = elem.get("id", ""); if not elem_id: elem.set("id", anchor_name)
		if sections[index].Root.Attrs["id"] == "" {
			// Use the filename (without .xhtml) as the ID
			elemID := strings.TrimSuffix(sections[index].Filename, ".xhtml")
			if sections[index].Root.Attrs == nil {
				sections[index].Root.Attrs = map[string]string{}
			}
			sections[index].Root.Attrs["id"] = elemID
		}
		replaceAnchorPlaceholdersInParts(sections[index].Root.Children, resolved)
	}
}

func attachSectionAliasAnchors(sections []renderedSection, renderer *storylineRenderer) {
	if renderer == nil {
		return
	}
	for index := range sections {
		root := sections[index].Root
		if root == nil {
			continue
		}
		sectionID := strings.TrimSuffix(sections[index].Filename, ".xhtml")
		for positionID, mappedSectionID := range renderer.positionToSection {
			if mappedSectionID != sectionID {
				continue
			}
			offsets := renderer.positionAnchors[positionID]
			if len(offsets) == 0 {
				continue
			}
			if anchorID := renderer.anchorIDForPosition(positionID, 0); anchorID != "" && len(renderer.anchorNamesByID[anchorID]) == 0 {
				if root.Attrs == nil {
					root.Attrs = map[string]string{}
				}
				if root.Attrs["id"] == "" {
					root.Attrs["id"] = anchorID
				}
				renderer.emittedAnchorIDs[anchorID] = true
				renderer.registerAnchorElementNames(positionID, 0, anchorID)
			}
			for offset := range offsets {
				if offset <= 0 {
					continue
				}
				anchorID := renderer.anchorIDForPosition(positionID, offset)
				if anchorID == "" || len(renderer.anchorNamesByID[anchorID]) > 0 {
					continue
				}
				target := locateOffset(root, offset)
				if target == nil {
					continue
				}
				if target.Attrs == nil {
					target.Attrs = map[string]string{}
				}
				if target.Attrs["id"] == "" {
					target.Attrs["id"] = anchorID
				}
				renderer.emittedAnchorIDs[anchorID] = true
				renderer.registerAnchorElementNames(positionID, offset, anchorID)
			}
		}
	}
}

func replaceAnchorPlaceholdersInParts(parts []htmlPart, resolved map[string]string) {
	for _, part := range parts {
		element, ok := part.(*htmlElement)
		if !ok || element == nil {
			continue
		}
		if href := element.Attrs["href"]; strings.HasPrefix(href, "anchor:") {
			if resolvedHref := resolved[strings.TrimPrefix(href, "anchor:")]; resolvedHref != "" {
				element.Attrs["href"] = resolvedHref
			}
		}
		replaceAnchorPlaceholdersInParts(element.Children, resolved)
	}
}

// =============================================================================
// Missing Python functions — Ports from yj_to_epub_navigation.py
// =============================================================================

// processAnchors initializes anchor tracking data structures and processes all anchors.
// Port of Python KFX_EPUB_Navigation.process_anchors (yj_to_epub_navigation.py L40-67).
func (p *navProcessor) processAnchors(anchors map[string]interface{}) {
	for _, anchorRaw := range anchors {
		anchor, ok := asMap(anchorRaw)
		if !ok {
			continue
		}
		if uri, ok := asString(anchor["$186"]); ok {
			if uri == "http://" || uri == "https://" {
				uri = ""
			}
			// Store external URI anchor (Python: self.anchor_uri[anchor_name] = uri)
			// In Go, external URI anchors are stored differently
			_ = uri
		}
	}
}

// processNavContainer processes a navigation container and its nav units.
// Port of Python KFX_EPUB_Navigation.process_nav_container (yj_to_epub_navigation.py L118-197).
func (p *navProcessor) processNavContainer(navContainer map[string]interface{}, navContainerName string, readingOrderName string, hasNavHeadings bool) {
	p.processContainer(navContainer, hasNavHeadings)
}

// getPosition extracts (locationID, offset) from a position map.
// Port of Python KFX_EPUB_Navigation.get_position (yj_to_epub_navigation.py L285-289).
func (p *navProcessor) getPosition(position map[string]interface{}) (int, int) {
	eid := getLocationID(position)
	offset, _ := asInt(position["$143"])
	return eid, offset
}

// getRepresentation extracts (label, icon, description) from a nav unit entry.
// Port of Python KFX_EPUB_Navigation.get_representation (yj_to_epub_navigation.py L291-313).
func (p *navProcessor) getRepresentation(entry map[string]interface{}) (string, string, string) {
	label := ""
	var icon, description string

	if repRaw, ok := asMap(entry["$241"]); ok {
		if iconRaw, ok := asString(repRaw["$245"]); ok {
			icon = iconRaw
			label = iconRaw
		}
		if labelRaw, ok := asString(repRaw["$244"]); ok {
			label = labelRaw
		}
	}

	return label, icon, description
}

// positionStr formats a (eid, offset) pair as a string.
// Port of Python KFX_EPUB_Navigation.position_str (yj_to_epub_navigation.py L315-316).
func positionStr(eid, offset int) string {
	return fmt.Sprintf("%d.%d", eid, offset)
}

// positionOfAnchor finds the (eid, offset) position for an anchor name.
// Port of Python KFX_EPUB_Navigation.position_of_anchor (yj_to_epub_navigation.py L345-351).
func (p *navProcessor) positionOfAnchor(anchorName string) (int, int) {
	for eid, offsets := range p.positionAnchors {
		for offset, names := range offsets {
			for _, name := range names {
				if name == anchorName {
					return eid, offset
				}
			}
		}
	}
	return 0, 0
}

// registerLinkID registers an anchor for a link element by eid.
// Port of Python KFX_EPUB_Navigation.register_link_id (yj_to_epub_navigation.py L362-363).
func (p *navProcessor) registerLinkID(eid int, kind string) string {
	name := fmt.Sprintf("%s_%d", kind, eid)
	p.registerAnchor(name, navTarget{PositionID: eid, Offset: 0}, nil)
	return name
}

// getAnchorID returns (or creates) a unique HTML id for an anchor name.
// Port of Python KFX_EPUB_Navigation.get_anchor_id (yj_to_epub_navigation.py L365-370).
func (p *navProcessor) getAnchorID(anchorName string) string {
	return p.uniqueAnchorName(anchorName)
}

// processPosition handles position anchor placement at a given (eid, offset).
// Port of Python KFX_EPUB_Navigation.process_position (yj_to_epub_navigation.py L375-402).
func (p *navProcessor) processPosition(eid, offset int, elem *htmlElement) []string {
	if offsets, ok := p.positionAnchors[eid]; ok {
		if names, ok := offsets[offset]; ok {
			anchorID := p.getAnchorID(names[0])
			if elem != nil {
				if elem.Attrs == nil {
					elem.Attrs = map[string]string{}
				}
				if _, has := elem.Attrs["id"]; !has {
					elem.Attrs["id"] = anchorID
				}
			}
			delete(offsets, offset)
			if len(offsets) == 0 {
				delete(p.positionAnchors, eid)
			}
			return names
		}
	}
	return nil
}

// moveAnchor moves an anchor's element reference from old to new.
// Port of Python KFX_EPUB_Navigation.move_anchor (yj_to_epub_navigation.py L404-410).
func moveAnchor(oldElem, newElem *htmlElement) {
	if oldElem.Attrs != nil {
		if id, has := oldElem.Attrs["id"]; has && newElem != nil {
			if newElem.Attrs == nil {
				newElem.Attrs = map[string]string{}
			}
			if _, has := newElem.Attrs["id"]; !has {
				newElem.Attrs["id"] = id
			}
			delete(oldElem.Attrs, "id")
		}
	}
}

// moveAnchors moves all anchors rooted in oldRoot to targetElem.
// Port of Python KFX_EPUB_Navigation.move_anchors (yj_to_epub_navigation.py L412-418).
func moveAnchors(oldRoot, targetElem *htmlElement) {
	moveAnchor(oldRoot, targetElem)
}

// getAnchorURI returns the URI for a named anchor.
// Port of Python KFX_EPUB_Navigation.get_anchor_uri (yj_to_epub_navigation.py L420-429).
func (p *navProcessor) getAnchorURI(anchorName string) string {
	// Try to resolve to a file#fragment URI.
	// Port of Python KFX_EPUB_Navigation.get_anchor_uri (yj_to_epub_navigation.py L420-429).
	// For now return the anchor name as a fragment reference.
	return "#" + anchorName
}

// anchorAsURI converts an anchor name to anchor: URI form.
// Port of Python KFX_EPUB_Navigation.anchor_as_uri (yj_to_epub_navigation.py L437-438).
func anchorAsURI(anchor string) string {
	return "anchor:" + anchor
}

// anchorFromURI extracts the anchor name from an anchor: URI.
// Port of Python KFX_EPUB_Navigation.anchor_from_uri (yj_to_epub_navigation.py L440-441).
func anchorFromURI(uri string) string {
	return strings.TrimPrefix(uri, "anchor:")
}

// idOfAnchor returns the HTML id attribute value for an anchor in a specific file.
// Port of Python KFX_EPUB_Navigation.id_of_anchor (yj_to_epub_navigation.py L443-450).
func (p *navProcessor) idOfAnchor(anchor string, filename string) string {
	url := p.getAnchorURI(anchor)
	parts := strings.SplitN(url, "#", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

// resolveTocTarget resolves anchor names to URIs in the TOC.
// Port of Python KFX_EPUB_Navigation.resolve_toc_target (yj_to_epub_navigation.py L507-513).
func (p *navProcessor) resolveTocTarget(toc *[]navPoint) {
	// Port of Python resolve_toc_target (yj_to_epub_navigation.py L507-513).
	// In Python, toc_entry.target is set to get_anchor_uri(toc_entry.anchor).
	// In Go, navPoint.Target is a navTarget{PositionID, Offset} resolved during
	// processNavUnit. The anchor-to-URI resolution happens at EPUB assembly time.
	for i := range *toc {
		if len((*toc)[i].Children) > 0 {
			p.resolveTocTarget(&(*toc)[i].Children)
		}
	}
}

// rootElement walks up to the root of the HTML tree.
// Port of Python root_element (yj_to_epub_navigation.py L518-522).
func rootElement(elem *htmlElement) *htmlElement {
	// In Go's htmlElement model, there's no parent pointer.
	// This function exists for API parity but the root must be tracked separately.
	return elem
}

// visibleElementsBefore checks if there are visible elements before the target.
// Port of Python visible_elements_before (yj_to_epub_navigation.py L525-541).
func visibleElementsBefore(elem *htmlElement, root *htmlElement) bool {
	if root == nil || elem == root {
		return false
	}
	// Walk root's children looking for elem
	found := false
	var walk func(e *htmlElement) bool
	walk = func(e *htmlElement) bool {
		if e == elem {
			found = true
			return true
		}
		if e.Tag == "img" || e.Tag == "br" || e.Tag == "hr" || e.Tag == "li" || e.Tag == "ol" || e.Tag == "ul" {
			return false // visible element before target
		}
		for _, child := range e.Children {
			if childElem, ok := child.(*htmlElement); ok {
				if walk(childElem) {
					return true
				}
			} else if txt, ok := child.(htmlText); ok && txt.Text != "" {
				return false // text before target
			}
		}
		return false
	}
	walk(root)
	return found
}
