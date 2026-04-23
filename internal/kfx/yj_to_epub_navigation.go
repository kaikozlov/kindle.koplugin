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

// parseNavRepresentation extracts (label, icon, description) from a nav unit.
// Port of Python get_representation (yj_to_epub_navigation.py L247-271).
// Python extracts: $245→icon+label, $146→description (content list → text), $244→label.
func parseNavRepresentation(entry map[string]interface{}) (label string, icon string, description string) {
	representation, ok := asMap(entry["$241"])
	if !ok {
		return "", "", ""
	}

	// Python: if "$245" in representation: icon = representation.pop("$245"); label = str(icon)
	if iconRaw, ok := asMap(representation["$245"]); ok {
		// Icon is a resource reference; the label is str(icon).
		// For now, we extract the resource ID for later resolution.
		if resourceID, ok := asString(iconRaw["$175"]); ok {
			icon = resourceID
		}
		if icon != "" {
			label = icon // Python: label = str(icon)
		}
	}

	// Python: if "$146" in representation: process content list → text → description
	if descRaw, ok := asSlice(representation["$146"]); ok && len(descRaw) > 0 {
		// Python builds a div element, processes content list, then extracts text.
		// Simplified: just extract text content from the representation.
		// In practice, navigation descriptions are simple text strings.
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

	// Python: if "$244" in representation: label = representation.pop("$244")
	if textLabel, ok := asString(representation["$244"]); ok {
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
	orientationLock    string            // Python: self.orientation_lock — used for $248 entry_set filtering
}

func processNavigation(navRoots []map[string]interface{}, navContainers map[string]map[string]interface{}, orientationLock string) navProcessor {
	state := navProcessor{
		usedAnchorNames:    map[string]bool{},
		positionAnchors:    map[int]map[int][]string{},
		anchorSites:        map[string]map[string]struct{}{},
		anchorHeadingLevel: map[string]int{},
		navContainers:      navContainers,
		pageLabelAnchorID:  map[string]string{},
		orientationLock:    orientationLock,
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
	// Port of Python process_nav_unit (yj_to_epub_navigation.py L176-227).
	// Extract label, icon, and description from representation.
	label, icon, description := parseNavRepresentation(entry)
	if label != "" {
		label = strings.TrimSpace(label)
	}

	// Python: desc = nav_unit.pop("$154", None); if desc: description = desc.strip()
	if desc, ok := asString(entry["$154"]); ok && desc != "" {
		description = strings.TrimSpace(desc)
	}

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

	// Process child nav units from $247
	childrenRaw, _ := asSlice(entry["$247"])
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
	entrySets, _ := asSlice(entry["$248"])
	for _, esRaw := range entrySets {
		entrySet, ok := asMap(esRaw)
		if !ok {
			continue
		}
		// Process children within entry set
		esChildren, _ := asSlice(entrySet["$247"])
		for _, raw := range esChildren {
			child := resolveNavigationUnit(raw)
			if child == nil {
				continue
			}
			p.processNavUnit(navType, child, &children, false, nextHeading)
		}

		// Orientation filtering (Python L218-224)
		orientation, _ := asString(entrySet["$215"])
		switch orientation {
		case "$386":
			// Portrait: clear children unless locked to landscape
			// Python: if self.orientation_lock != "landscape": nested_toc = []
			if p.orientationLock != "landscape" {
				children = nil
			}
		case "$385":
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
