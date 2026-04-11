// Navigation, TOC, guide, and page-list handling ported from Calibre
// REFERENCE/Calibre_KFX_Input/kfxlib/yj_to_epub_navigation.py (KFX_EPUB_Navigation).
package kfx

import (
	"fmt"
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
		entries, ok := asSlice(root["$392"])
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
	navType, _ := asString(value["$235"])
	return navType
}

func parseNavTitle(value map[string]interface{}) string {
	label, ok := asMap(value["$241"])
	if !ok {
		return ""
	}
	title, _ := asString(label["$244"])
	return strings.TrimSpace(title)
}

func parseNavTarget(value map[string]interface{}) navTarget {
	target, ok := asMap(value["$246"])
	if !ok {
		return navTarget{}
	}
	positionID, _ := asInt(target["$155"])
	if positionID == 0 {
		positionID, _ = asInt(target["$598"])
	}
	offset, _ := asInt(target["$143"])
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

func navigationToEPUB(points []navPoint, targetHref func(navTarget) string) []epub.NavPoint {
	output := make([]epub.NavPoint, 0, len(points))
	for _, point := range points {
		href := targetHref(point.Target)
		if href == "" || point.Title == "" {
			continue
		}
		output = append(output, epub.NavPoint{
			Title:    point.Title,
			Href:     href,
			Children: navigationToEPUB(point.Children, targetHref),
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
}

func processNavigation(navRoots []map[string]interface{}, navContainers map[string]map[string]interface{}) navProcessor {
	state := navProcessor{
		usedAnchorNames:    map[string]bool{},
		positionAnchors:    map[int]map[int][]string{},
		anchorSites:        map[string]map[string]struct{}{},
		anchorHeadingLevel: map[string]int{},
		navContainers:      navContainers,
		pageLabelAnchorID:  map[string]string{},
	}
	containers := collectNavigationContainers(navRoots, navContainers)
	hasNavHeadings := false
	for _, container := range containers {
		if navigationType(container) == "$798" {
			hasNavHeadings = true
			break
		}
	}
	for _, container := range containers {
		state.processContainer(container, hasNavHeadings)
	}
	return state
}

func (p *navProcessor) processContainer(container map[string]interface{}, hasNavHeadings bool) {
	navType := navigationType(container)
	if imports, ok := asSlice(container["imports"]); ok {
		for _, raw := range imports {
			if imported := resolveNavigationContainer(raw, p.navContainers); imported != nil {
				p.processContainer(imported, hasNavHeadings)
			}
		}
	}
	entries, ok := asSlice(container["$247"])
	if !ok {
		return
	}
	for _, raw := range entries {
		entry := resolveNavigationUnit(raw)
		if entry == nil {
			continue
		}
		switch navType {
		case "$212", "$213", "$214", "$798":
			p.processNavUnit(navType, entry, &p.toc, navType == "$212" && !hasNavHeadings, nil)
		case "$236":
			p.processGuideUnit(entry)
		case "$237":
			p.processPageUnit(entry)
		}
	}
}

func (p *navProcessor) processGuideUnit(entry map[string]interface{}) {
	label := parseNavTitle(entry)
	target := parseNavTarget(entry)
	if target.PositionID == 0 {
		return
	}
	navUnitName, _ := asString(entry["$240"])
	if navUnitName == "" {
		navUnitName = label
	}
	guideType := guideTypeForLandmark(asStringDefault(entry["$238"]))
	anchorName := p.uniqueAnchorName(navUnitName)
	if anchorName == "" {
		anchorName = p.uniqueAnchorName(guideType)
	}
	p.registerAnchor(anchorName, target, nil)
	if label == "cover-nav-unit" {
		label = ""
	}
	p.guide = append(p.guide, guideEntry{Type: guideType, Title: label, Target: target})
}

func (p *navProcessor) processPageUnit(entry map[string]interface{}) {
	label := parseNavTitle(entry)
	if debug := os.Getenv("KFX_DEBUG_PAGES"); debug != "" {
		fmt.Fprintf(os.Stderr, "page unit label=%q entry=%#v\n", label, entry)
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
	label := parseNavTitle(entry)
	navUnitName, _ := asString(entry["$240"])
	if navUnitName == "" {
		navUnitName = label
	}
	nextHeading := (*int)(nil)
	if navType == "$798" {
		if level, ok := headingLevelForLandmark(asStringDefault(entry["$238"])); ok {
			headingLevel = intPtr(level)
			nextHeading = intPtr(level)
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

	childrenRaw, _ := asSlice(entry["$247"])
	children := make([]navPoint, 0, len(childrenRaw))
	for _, raw := range childrenRaw {
		child := resolveNavigationUnit(raw)
		if child == nil {
			continue
		}
		p.processNavUnit(navType, child, &children, false, nextHeading)
	}

	target := parseNavTarget(entry)
	hasTarget := target.PositionID != 0
	if hasTarget {
		anchorName := fmt.Sprintf("%s_%d_%s", navType, p.tocEntryCount, navUnitName)
		p.tocEntryCount++
		p.registerAnchor(anchorName, target, headingLevel)
	}
	if navType == "$798" {
		return
	}
	if label == "" && !hasTarget {
		*out = append(*out, children...)
		return
	}
	*out = append(*out, navPoint{Title: label, Target: target, Children: children})
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
	case "$233":
		return "cover"
	case "$396", "$269":
		return "text"
	case "$212":
		return "toc"
	default:
		return value
	}
}

func headingLevelForLandmark(value string) (int, bool) {
	switch value {
	case "$799":
		return 1, true
	case "$800":
		return 2, true
	case "$801":
		return 3, true
	case "$802":
		return 4, true
	case "$803":
		return 5, true
	case "$804":
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

// Port of KFX_EPUB_Navigation.report_missing_positions (yj_to_epub_navigation.py L353+).
func reportMissingPositions(positionAnchors map[int]map[int][]string) {
	if len(positionAnchors) == 0 {
		return
	}
	left := 0
	for _, offs := range positionAnchors {
		left += len(offs)
	}
	if left == 0 {
		return
	}
	if os.Getenv("KFX_VERBOSE") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "kfx: error: failed to locate %d referenced position anchor(s) after content (report_missing_positions)\n", left)
}

// Port of KFX_EPUB_Navigation.report_duplicate_anchors (yj_to_epub_navigation.py L431+).
// Logs when an anchor that received a resolved URI was registered at more than one (position, offset).
func reportDuplicateAnchors(state navProcessor, resolved map[string]string) {
	if len(state.anchorSites) == 0 {
		return
	}
	for name, sites := range state.anchorSites {
		if len(sites) <= 1 {
			continue
		}
		if resolved == nil || resolved[name] == "" {
			continue
		}
		keys := make([]string, 0, len(sites))
		for k := range sites {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintf(os.Stderr, "kfx: error: anchor %q has multiple positions: %s (report_duplicate_anchors)\n", name, strings.Join(keys, ", "))
	}
}
