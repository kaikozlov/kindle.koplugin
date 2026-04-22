package kfx

import (
	"fmt"
	"os"
	"strings"
)

// Port of Python process_reading_order reading order iteration (yj_to_epub_content.py L105+).
// Python iterates all reading orders; Go merges all section lists.
func readSectionOrder(value map[string]interface{}) []string {
	entries, ok := asSlice(value["$169"])
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
		sections, ok := asSlice(entryMap["$170"])
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
	return chooseFragmentIdentity(fragmentID, value["$174"])
}

func readPositionMap(value map[string]interface{}) []int {
	entries, ok := asSlice(value["$181"])
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
	containers, ok := asSlice(section["$141"])
	if !ok || len(containers) == 0 {
		return ""
	}
	first, ok := asMap(containers[0])
	if !ok {
		return ""
	}
	storylineID, _ := asString(first["$176"])
	return storylineID
}

func parseSectionFragment(fragmentID string, value map[string]interface{}) sectionFragment {
	id := chooseFragmentIdentity(fragmentID, value["$174"])
	containers, ok := asSlice(value["$141"])
	if !ok || len(containers) == 0 {
		return sectionFragment{ID: id}
	}
	templates := make([]pageTemplateFragment, 0, len(containers))
	for _, raw := range containers {
		container, ok := asMap(raw)
		if !ok {
			continue
		}
		storylineID, _ := asString(container["$176"])
		pageTemplateStyle, _ := asString(container["$157"])
		positionID, _ := asInt(container["$155"])
		templates = append(templates, pageTemplateFragment{
			PositionID:         positionID,
			Storyline:          storylineID,
			PageTemplateStyle:  pageTemplateStyle,
			PageTemplateValues: filterBodyStyleValues(container),
			HasCondition:       container["$171"] != nil,
			Condition:          container["$171"],
		})
	}
	if len(templates) == 0 {
		return sectionFragment{ID: id}
	}
	mainTemplate := templates[len(templates)-1]
	return sectionFragment{
		ID:                 id,
		PositionID:         mainTemplate.PositionID,
		Storyline:          mainTemplate.Storyline,
		PageTemplateStyle:  mainTemplate.PageTemplateStyle,
		PageTemplateValues: mainTemplate.PageTemplateValues,
		PageTemplates:      templates,
	}
}

func collectStorylinePositions(nodes []interface{}, sectionID string, positions map[int]string) {
	for _, raw := range nodes {
		node, ok := asMap(raw)
		if !ok {
			continue
		}
		if positionID, ok := asInt(node["$155"]); ok && positionID != 0 && positions[positionID] == "" {
			positions[positionID] = sectionID
		}
		if children, ok := asSlice(node["$146"]); ok {
			collectStorylinePositions(children, sectionID, positions)
		}
		if cols, ok := asSlice(node["$152"]); ok {
			collectStorylinePositions(cols, sectionID, positions)
		}
	}
}

func parseAnchorFragment(fragmentID string, value map[string]interface{}) anchorFragment {
	id := chooseFragmentIdentity(fragmentID, value["$180"])
	if debugAnchors := os.Getenv("KFX_DEBUG_ANCHORS"); debugAnchors != "" {
		for _, wanted := range strings.Split(debugAnchors, ",") {
			if strings.TrimSpace(wanted) == id || strings.TrimSpace(wanted) == fragmentID {
				fmt.Fprintf(os.Stderr, "anchor fragment id=%s fragment=%s value=%#v\n", id, fragmentID, value)
			}
		}
	}
	if uri, ok := asString(value["$186"]); ok {
		if uri == "http://" || uri == "https://" {
			uri = ""
		}
		return anchorFragment{ID: id, URI: uri}
	}
	// Port of Python yj_to_epub_navigation.py:55-63 — anchor fragments with $183
	// reference a position. Python: register_anchor(name, get_position(anchor.pop("$183")))
	// where get_position extracts (eid, offset) from $183.$155/$598 and $183.$143.
	target, ok := asMap(value["$183"])
	if !ok {
		return anchorFragment{ID: id}
	}
	positionID, _ := asInt(target["$155"])
	offset, _ := asInt(target["$143"])
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
	return !(strings.HasPrefix(value, "$") && len(value) > 1)
}

func isPlaceholderSymbol(value string) bool {
	if !strings.HasPrefix(value, "$") || len(value) == 1 {
		return false
	}
	for _, r := range value[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
