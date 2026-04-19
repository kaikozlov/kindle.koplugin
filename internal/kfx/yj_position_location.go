// Position and location mapping ported from Calibre
// REFERENCE/Calibre_KFX_Input/kfxlib/yj_position_location.py (BookPosLoc, ~1325 lines).
package kfx

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Constants ported from yj_position_location.py lines 18–27.
const (
	KFX_POSITIONS_PER_LOCATION = 110
	TYPICAL_POSITIONS_PER_PAGE = 1850
	MIN_POSITIONS_PER_PAGE     = 1
	MAX_POSITIONS_PER_PAGE     = 50000
	GEN_COVER_PAGE_NUMBER      = true
	MAX_WHITE_SPACE_ADJUST     = 50

	MAX_REPORT_ERRORS = 0
)

// RANGE_OPERS lists operators that represent range operations (start_eid left nil).
// Port of Python RANGE_OPERS (yj_position_location.py line 27).
var RANGE_OPERS = map[string]bool{
	"$298": true,
	"$299": true,
}

// ContentChunk represents a unit of content position information.
// Port of Python ContentChunk class (yj_position_location.py lines 31–62).
type ContentChunk struct {
	PID           int    // position ID
	EID           interface{} // entity ID (int or string/IonSymbol)
	EIDOffset     int    // offset within entity
	Length        int    // length in positions
	SectionName   string // section this chunk belongs to
	MatchZeroLen  bool   // whether zero-length match is OK
	Text          string // text content (empty string means nil)
	ImageResource string // image resource name
}

// NewContentChunk creates a ContentChunk with validation.
// Port of Python ContentChunk.__init__ (yj_position_location.py lines 32–43).
func NewContentChunk(pid int, eid interface{}, eidOffset, length int, sectionName string,
	matchZeroLen bool, text string, imageResource string) *ContentChunk {

	cc := &ContentChunk{
		PID:           pid,
		EID:           eid,
		EIDOffset:     eidOffset,
		Length:        length,
		SectionName:   sectionName,
		MatchZeroLen:  matchZeroLen,
		Text:          text,
		ImageResource: imageResource,
	}

	if pid < 0 || length < 0 || eidOffset < 0 {
		log.Printf("kfx: bad ContentChunk: %s", cc.String())
	}
	if eidInt, ok := eid.(int); ok && eidInt <= 0 {
		log.Printf("kfx: bad ContentChunk: %s", cc.String())
	}
	if text != "" && utf8.RuneCountInString(text) != length {
		log.Printf("kfx: bad ContentChunk: text length mismatch: %s", cc.String())
	}

	return cc
}

// Equal compares two ContentChunks. Port of Python ContentChunk.__eq__ (yj_position_location.py lines 45–55).
func (cc *ContentChunk) Equal(other *ContentChunk, comparePIDs ...bool) bool {
	if other == nil {
		return false
	}

	// Check eid type mismatch
	if fmt.Sprintf("%T", cc.EID) != fmt.Sprintf("%T", other.EID) {
		return false
	}

	shouldComparePIDs := true
	if len(comparePIDs) > 0 {
		shouldComparePIDs = comparePIDs[0]
	}

	if cc.PID == other.PID || !shouldComparePIDs {
		if eidsEqual(cc.EID, other.EID) && cc.EIDOffset == other.EIDOffset && cc.SectionName == other.SectionName {
			if cc.Length == other.Length ||
				(cc.MatchZeroLen && other.Length == 0) ||
				(other.MatchZeroLen && cc.Length == 0) {
				return true
			}
		}
	}

	return false
}

// String returns a formatted representation. Port of Python ContentChunk.__repr__.
func (cc *ContentChunk) String() string {
	eidStr := fmt.Sprintf("%v", cc.EID)
	offsetStr := ""
	if cc.EIDOffset != 0 {
		offsetStr = fmt.Sprintf("+%d", cc.EIDOffset)
	}
	matchZero := ""
	if cc.MatchZeroLen {
		matchZero = "*"
	}
	textRepr := ""
	if cc.Text != "" {
		textRepr = fmt.Sprintf("%q", cc.Text)
	}
	imgRepr := ""
	if cc.ImageResource != "" {
		imgRepr = fmt.Sprintf("%q", cc.ImageResource)
	}
	return fmt.Sprintf("pid=%d eid=%s%s len=%d%s sect=%s text=%s img=%s",
		cc.PID, eidStr, offsetStr, cc.Length, matchZero, cc.SectionName, textRepr, imgRepr)
}

// eidsEqual compares two EID values (which can be int or string).
func eidsEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	switch av := a.(type) {
	case int:
		if bv, ok := b.(int); ok {
			return av == bv
		}
		return false
	case string:
		if bv, ok := b.(string); ok {
			return av == bv
		}
		return false
	default:
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
}

// ConditionalTemplate tracks conditional template position information for illustrated layout.
// Port of Python ConditionalTemplate class (yj_position_location.py lines 65–85).
type ConditionalTemplate struct {
	EndEID         interface{}
	EndEIDOffset   int
	Oper           string
	PosInfo        []*ContentChunk
	UseNext        bool
	StartEID       interface{}
	StartEIDOffset int
}

// NewConditionalTemplate creates a ConditionalTemplate.
// Port of Python ConditionalTemplate.__init__ (yj_position_location.py lines 66–77).
func NewConditionalTemplate(endEID interface{}, endEIDOffset int, oper string, posInfo []*ContentChunk) *ConditionalTemplate {
	ct := &ConditionalTemplate{
		EndEID:       endEID,
		EndEIDOffset: endEIDOffset,
		Oper:         oper,
		PosInfo:       posInfo,
		UseNext:      false,
	}

	if !RANGE_OPERS[oper] {
		ct.StartEID = endEID
		ct.StartEIDOffset = endEIDOffset
	}
	// For RANGE_OPERS, StartEID and StartEIDOffset remain zero/nil

	return ct
}

// String returns a formatted representation. Port of Python ConditionalTemplate.__repr__.
func (ct *ConditionalTemplate) String() string {
	if !RANGE_OPERS[ct.Oper] {
		return fmt.Sprintf("%s%v+%d (%v)", ct.Oper, ct.EndEID, ct.EndEIDOffset, ct.posInfoEIDs())
	}
	return fmt.Sprintf("%v+%d%s%v+%d(%v)",
		ct.StartEID, ct.StartEIDOffset, ct.Oper, ct.EndEID, ct.EndEIDOffset, ct.posInfoEIDs())
}

func (ct *ConditionalTemplate) posInfoEIDs() string {
	eids := make([]string, len(ct.PosInfo))
	for i, cc := range ct.PosInfo {
		eids[i] = fmt.Sprintf("%v", cc.EID)
	}
	return strings.Join(eids, " ")
}

// MatchReport tracks error reports with a configurable limit.
// Port of Python MatchReport class (yj_position_location.py lines 87–101).
type MatchReport struct {
	Count int
	Limit int
}

// NewMatchReport creates a MatchReport. Port of Python MatchReport.__init__.
func NewMatchReport(noLimit bool) *MatchReport {
	mr := &MatchReport{
		Count: 0,
		Limit: 0,
	}
	if !noLimit {
		mr.Limit = MAX_REPORT_ERRORS
	}
	return mr
}

// Report logs a warning message if under the limit. Port of Python MatchReport.report.
func (mr *MatchReport) Report(msg string) {
	if mr.Limit == 0 || mr.Count < mr.Limit {
		log.Printf("kfx: warning: %s", msg)
	}
	mr.Count++
}

// Final reports if the limit was exceeded. Port of Python MatchReport.final.
func (mr *MatchReport) Final() {
	if mr.Limit != 0 && mr.Count > mr.Limit {
		log.Printf("kfx: Mismatch report limit exceeded, %d total errors", mr.Count)
	}
}

// BookPosLoc provides position and location mapping for a book.
// Port of Python BookPosLoc class (yj_position_location.py lines 103–1325).
// Fields mirror Python self.* attributes used across position/location methods.
type BookPosLoc struct {
	Fragments         *fragmentCatalog
	OrderedSections   []string
	IsDictionary      bool
	IsScribeNotebook  bool
	IsKPFPub          bool
	IsKFXV1           bool
	IsSample          bool
	IsFixedLayout     bool
	IsPrintReplica    bool
	IsPDFBacked       bool
	BookType          bookType
	CDEType           string
	HasIllustratedLayoutConditionalPageTemplate bool

	// Internal state for position info collection (Python self.cpi_*)
	cpiPID          int
	cpiPIDForOffset int
	cpiProcessingStory bool
	cpiFixed        bool
	lastPII         int

	// hasEIDOffset tracks whether any eid_offset != 0 was seen
	hasEIDOffset bool
}

// anchorEidOffset looks up an anchor fragment and returns its (eid, offset).
// Port of Python anchor_eid_offset (yj_position_location.py lines 579–586).
func (bpl *BookPosLoc) anchorEidOffset(anchor string) (interface{}, int, bool) {
	if bpl.Fragments == nil {
		return nil, 0, false
	}
	anchorFrag, ok := bpl.Fragments.AnchorFragments[anchor]
	if !ok {
		log.Printf("kfx: Failed to locate position for anchor: %s", anchor)
		return nil, 0, false
	}
	// The anchorFragment stores PositionID; we need to look at the raw fragment data
	// for $183 → {$155: eid, $143: offset}. Since our anchorFragment may not have
	// the full position data, we return what we can from stored position ID.
	if anchorFrag.PositionID != 0 {
		return anchorFrag.PositionID, 0, true
	}
	return nil, 0, false
}

// PidForEid performs a linear search (with wraparound) for a chunk matching eid+offset.
// Port of Python pid_for_eid (yj_position_location.py lines 960–982).
func (bpl *BookPosLoc) PidForEid(eid interface{}, eidOffset int, posInfo []*ContentChunk) *int {
	if len(posInfo) == 0 {
		return nil
	}

	startPII := bpl.lastPII
	if startPII >= len(posInfo) {
		startPII = 0
		bpl.lastPII = 0
	}

	pii := startPII
	for {
		pi := posInfo[pii]
		if eidsEqual(pi.EID, eid) && eidOffset >= pi.EIDOffset && eidOffset <= pi.EIDOffset+pi.Length {
			result := pi.PID + eidOffset - pi.EIDOffset
			bpl.lastPII = pii
			return &result
		}
		pii++
		if pii >= len(posInfo) {
			pii = 0
		}
		if pii == startPII {
			break
		}
	}

	return nil
}

// EidForPid performs a binary search over posInfo sorted by PID.
// Port of Python eid_for_pid (yj_position_location.py lines 984–999).
func EidForPid(pid int, posInfo []*ContentChunk) (interface{}, int, bool) {
	low := 0
	high := len(posInfo) - 1

	for low <= high {
		mid := ((high - low) / 2) + low
		pi := posInfo[mid]

		if pid < pi.PID {
			high = mid - 1
		} else if pid > pi.PID+pi.Length {
			low = mid + 1
		} else {
			return pi.EID, pi.EIDOffset + pid - pi.PID, true
		}
	}

	return nil, 0, false
}

// GenerateApproximateLocations generates location boundaries every KFX_POSITIONS_PER_LOCATION positions.
// Port of Python generate_approximate_locations (yj_position_location.py lines 1083–1114).
func GenerateApproximateLocations(posInfo []*ContentChunk) []*ContentChunk {
	pid := 0
	nextLocPosition := 0
	currentSectionName := ""
	var locInfo []*ContentChunk

	for _, chunk := range posInfo {
		eidLocOffset := 0
		locPID := pid

		if chunk.SectionName != currentSectionName {
			nextLocPosition = locPID
			currentSectionName = chunk.SectionName
		}

		for {
			if locPID == nextLocPosition {
				locInfo = append(locInfo, &ContentChunk{
					PID:       locPID,
					EID:       chunk.EID,
					EIDOffset: chunk.EIDOffset + eidLocOffset,
				})
				nextLocPosition += KFX_POSITIONS_PER_LOCATION
			}

			eidRemaining := chunk.Length - eidLocOffset
			locRemaining := nextLocPosition - locPID

			if eidRemaining <= locRemaining {
				break
			}

			eidLocOffset += locRemaining
			locPID = nextLocPosition
		}

		pid += chunk.Length
	}

	log.Printf("kfx: Built approximate location_map with %d locations", len(locInfo))
	return locInfo
}

// CreateLocationMap removes old fragments and builds a new $550 fragment.
// Port of Python create_location_map (yj_position_location.py lines 1116–1130).
// Returns hasYJLocationPidMap (always false in our implementation).
func (bpl *BookPosLoc) CreateLocationMap(locInfo []*ContentChunk) bool {
	// In the full Python port, this removes existing $550/$621 fragments and creates new ones.
	// Our simplified version returns false for has_yj_location_pid_map.
	_ = locInfo
	return false
}

// DetermineApproximatePages creates page breaks for position info.
// Port of Python determine_approximate_pages (yj_position_location.py lines 1260–1325).
func DetermineApproximatePages(posInfo []*ContentChunk, pageTemplateEIDs map[interface{}]bool,
	firstSectionName string, positionsPerPage int, fixedLayout bool) ([]map[string]interface{}, int) {

	var pages []map[string]interface{}
	newSectionPageCount := 0
	nextPagePID := -1
	prevSectionName := ""

	for _, chunk := range posInfo {
		if pageTemplateEIDs[chunk.EID] {
			continue
		}

		if chunk.SectionName == firstSectionName && !GEN_COVER_PAGE_NUMBER {
			continue
		}

		newSection := chunk.SectionName != prevSectionName
		prevSectionName = chunk.SectionName

		if fixedLayout {
			if newSection {
				newSectionPageCount++
				pages = append(pages, makePageEntry(chunk.EID, chunk.EIDOffset, len(pages)+1))
			}
		} else {
			if newSection {
				nextPagePID = chunk.PID
				newSectionPageCount++
			}

			minChunkOffset := 0
			for {
				chunkOffset := nextPagePID - chunk.PID
				if chunkOffset < 0 {
					chunkOffset = 0
				}
				if chunkOffset >= chunk.Length {
					break
				}

				// Whitespace lookback adjustment
				if chunk.Text != "" && len(chunk.Text) > chunkOffset {
					r := rune(chunk.Text[chunkOffset])
					if !unicode.IsSpace(r) {
						initChunkOffset := chunkOffset
						for {
							if chunkOffset == 0 {
								break
							}
							if chunkOffset <= minChunkOffset {
								chunkOffset = initChunkOffset
								break
							}
							prevRune := rune(chunk.Text[chunkOffset-1])
							if unicode.IsSpace(prevRune) {
								break
							}
							chunkOffset--
						}
					}
				}

				pages = append(pages, makePageEntry(chunk.EID, chunk.EIDOffset+chunkOffset, len(pages)+1))
				nextPagePID += positionsPerPage
				minChunkOffset = chunkOffset + maxInt(positionsPerPage-MAX_WHITE_SPACE_ADJUST, 1)
			}
		}
	}

	return pages, newSectionPageCount
}

// makePageEntry creates a page entry map matching Python's IonStruct format.
func makePageEntry(eid interface{}, eidOffset, pageNum int) map[string]interface{} {
	return map[string]interface{}{
		"$241": map[string]interface{}{
			"$244": fmt.Sprintf("%d", pageNum),
		},
		"$246": map[string]interface{}{
			"$155": eid,
			"$143": eidOffset,
		},
	}
}

// VerifyPositionInfo compares content-derived and map-derived position info.
// Port of Python verify_position_info (yj_position_location.py lines 836–904).
func VerifyPositionInfo(contentPosInfo, mapPosInfo []*ContentChunk, hasNonImageRenderInline bool) *MatchReport {
	report := NewMatchReport(true)

	contentIdx := 0
	mapIdx := 0
	contentNextPID := 0
	mapNextPID := 0

	contentAdvance := func(extra bool) {
		if contentIdx >= len(contentPosInfo) {
			return
		}
		chunk := contentPosInfo[contentIdx]
		if chunk.PID != contentNextPID {
			if hasNonImageRenderInline {
				if chunk.PID > contentNextPID {
					report.Report(fmt.Sprintf("position_id content expected pid %d <= idx=%d, chunk: %s",
						contentNextPID, contentIdx, chunk.String()))
				}
			} else {
				report.Report(fmt.Sprintf("position_id content expected pid %d at idx=%d, chunk: %s",
					contentNextPID, contentIdx, chunk.String()))
			}
		}
		if extra {
			report.Report(fmt.Sprintf("position_id content extra at idx=%d, chunk: %s",
				contentIdx, chunk.String()))
		}
		contentNextPID = chunk.PID + chunk.Length
		contentIdx++
	}

	mapAdvance := func(extra bool) {
		if mapIdx >= len(mapPosInfo) {
			return
		}
		chunk := mapPosInfo[mapIdx]
		if chunk.PID != mapNextPID {
			if hasNonImageRenderInline {
				if chunk.PID > mapNextPID {
					report.Report(fmt.Sprintf("position_id map expected pid %d <= idx=%d, chunk: %s",
						mapNextPID, mapIdx, chunk.String()))
				}
			} else {
				report.Report(fmt.Sprintf("position_id map expected pid %d at idx=%d, chunk: %s",
					mapNextPID, mapIdx, chunk.String()))
			}
		}
		if extra {
			report.Report(fmt.Sprintf("position_id map extra at idx=%d, chunk: %s",
				mapIdx, chunk.String()))
		}
		mapNextPID = chunk.PID + chunk.Length
		mapIdx++
	}

	comparePIDs := true

	for mapIdx < len(mapPosInfo) || contentIdx < len(contentPosInfo) {
		if mapIdx >= len(mapPosInfo) {
			contentAdvance(true)
			continue
		}
		if contentIdx >= len(contentPosInfo) {
			mapAdvance(true)
			continue
		}

		mapChunk := mapPosInfo[mapIdx]
		contentChunk := contentPosInfo[contentIdx]

		if contentChunk.Equal(mapChunk, comparePIDs) {
			mapAdvance(false)
			contentAdvance(false)
			continue
		}

		found := false
		for n := 1; n < 10; n++ {
			if mapIdx+n < len(mapPosInfo) && mapPosInfo[mapIdx+n].Equal(contentChunk) {
				for nn := 0; nn < n; nn++ {
					mapAdvance(true)
				}
				found = true
				break
			}
			if contentIdx+n < len(contentPosInfo) && mapChunk.Equal(contentPosInfo[contentIdx+n]) {
				for nn := 0; nn < n; nn++ {
					contentAdvance(true)
				}
				found = true
				break
			}
		}
		if !found {
			mapAdvance(true)
			contentAdvance(true)
		}
	}

	report.Final()
	return report
}

// CreatePositionMap builds pid→eid and section→eid mappings.
// Port of Python create_position_map (yj_position_location.py lines 906–958).
func (bpl *BookPosLoc) CreatePositionMap(posInfo []*ContentChunk) (bool, bool) {
	if bpl.IsDictionary || bpl.IsScribeNotebook || bpl.IsKPFPub {
		log.Printf("kfx: warning: Position map creation for KPF or dictionary not supported")
		return false, false
	}

	// Build section_eids map
	sectionEIDs := map[string]map[interface{}]bool{}
	for _, chunk := range posInfo {
		if sectionEIDs[chunk.SectionName] == nil {
			sectionEIDs[chunk.SectionName] = map[interface{}]bool{}
		}
		sectionEIDs[chunk.SectionName][chunk.EID] = true
	}

	// Build position_map ($264): section → list of eids
	positionMap := make([]map[string]interface{}, 0)
	for _, sectionName := range bpl.OrderedSections {
		eidSet := sectionEIDs[sectionName]
		eidList := make([]interface{}, 0, len(eidSet))
		for eid := range eidSet {
			eidList = append(eidList, eid)
		}
		positionMap = append(positionMap, map[string]interface{}{
			"$181": eidList,
			"$174": sectionName,
		})
	}

	_ = positionMap // In full port, would be stored as $264 fragment

	// Build position_id_map ($265): flat list of {pid, eid, offset}
	positionIDMap := make([]map[string]interface{}, 0)
	hasSPIM := false
	hasPositionIDOffset := false
	pid := 0

	for _, chunk := range posInfo {
		entry := map[string]interface{}{
			"$184": pid,
			"$185": chunk.EID,
		}
		if chunk.EIDOffset != 0 {
			entry["$143"] = chunk.EIDOffset
			hasPositionIDOffset = true
		}
		positionIDMap = append(positionIDMap, entry)
		pid += chunk.Length
	}

	// Terminal entry
	positionIDMap = append(positionIDMap, map[string]interface{}{
		"$184": pid,
		"$185": 0,
	})

	_ = positionIDMap // In full port, would be stored as $265 fragment

	return hasSPIM, hasPositionIDOffset
}

// CollectLocationMapInfo parses location maps from fragments.
// Port of Python collect_location_map_info (yj_position_location.py lines 1001–1081).
func (bpl *BookPosLoc) CollectLocationMapInfo(posInfo []*ContentChunk) []*ContentChunk {
	var locInfo []*ContentChunk
	var prevLoc *ContentChunk
	report := NewMatchReport(false)

	addLoc := func(pid int, eid interface{}, eidOffset int) {
		loc := &ContentChunk{PID: pid, EID: eid, EIDOffset: eidOffset}
		locInfo = append(locInfo, loc)
		if prevLoc != nil {
			prevLoc.Length = pid - prevLoc.PID
		}
		prevLoc = loc
	}

	endAddLoc := func() {
		if prevLoc != nil && len(posInfo) > 0 {
			last := posInfo[len(posInfo)-1]
			prevLoc.Length = last.PID + last.Length - prevLoc.PID
		}
	}

	// $550 fragment processing
	fragment550 := bpl.getFragment550()
	if fragment550 != nil {
		entries, ok := asSlice(fragment550["$182"])
		if ok {
			for i, lm := range entries {
				lmMap, ok := asMap(lm)
				if !ok {
					continue
				}
				_ = i
				eid := lmMap["$155"]
				eidOffset := asIntDefault(lmMap["$143"], 0)

				pid := bpl.PidForEid(eid, eidOffset, posInfo)
				if pid == nil {
					log.Printf("kfx: error: location_map failed to locate eid %v offset %d", eid, eidOffset)
				} else {
					addLoc(*pid, eid, eidOffset)
				}
			}
			endAddLoc()
		}
	}

	// $621 fragment processing
	fragment621 := bpl.getFragment621()
	hasYJLocationPidMap := fragment621 != nil
	if hasYJLocationPidMap {
		locationPIDs, ok := asSlice(fragment621["$182"])
		if ok {
			if len(locInfo) > 0 {
				// Cross-validate
				for i, loc := range locInfo {
					if i < len(locationPIDs) {
						lpmPID, ok := asInt(locationPIDs[i])
						if ok && loc.PID != lpmPID {
							report.Report(fmt.Sprintf("location_map pid %d != yj.location_pid_map pid %d for location %d eid %v offset %d",
								loc.PID, lpmPID, i+1, loc.EID, loc.EIDOffset))
						}
					}
				}
				if len(locInfo) != len(locationPIDs) {
					log.Printf("kfx: error: location_map has %d locations but yj.location_pid_map has %d",
						len(locInfo), len(locationPIDs))
				}
			} else {
				for i, rawPID := range locationPIDs {
					pid, ok := asInt(rawPID)
					if !ok {
						continue
					}
					eid, eidOffset, found := EidForPid(pid, posInfo)
					if !found {
						log.Printf("kfx: error: yj.location_pid_map %d failed to locate eid for pid %d", i+1, pid)
					} else {
						addLoc(pid, eid, eidOffset)
					}
				}
				endAddLoc()
			}
		}
	}

	report.Final()
	return locInfo
}

// getFragment550 returns the $550 location_map fragment data if available.
func (bpl *BookPosLoc) getFragment550() map[string]interface{} {
	// In the full Python port, this looks up self.fragments.get("$550", first=True)
	// For our simplified version, we return nil (no location map fragment available)
	return nil
}

// getFragment621 returns the $621 yj.location_pid_map fragment data if available.
func (bpl *BookPosLoc) getFragment621() map[string]interface{} {
	// In the full Python port, this looks up self.fragments.get("$621", first=True)
	return nil
}

// CreateApproximatePageList generates approximate page numbers.
// Port of Python create_approximate_page_list (yj_position_location.py lines 1132–1258).
func (bpl *BookPosLoc) CreateApproximatePageList(desiredNumPages int) {
	if bpl.CDEType != "" && bpl.CDEType != "EBOK" && bpl.CDEType != "EBSP" && bpl.CDEType != "PDOC" {
		log.Printf("kfx: error: Cannot create page numbers for KFX %s", bpl.CDEType)
		return
	}
	if bpl.IsDictionary {
		log.Printf("kfx: error: Cannot create page numbers for KFX dictionary")
		return
	}
	if bpl.IsScribeNotebook {
		log.Printf("kfx: error: Cannot create page numbers for a Scribe notebook")
		return
	}
	if len(bpl.OrderedSections) == 0 {
		log.Printf("kfx: error: Cannot create page numbers - no sections")
		return
	}

	// Full implementation would collect pos_info via CollectContentPositionInfo,
	// gather page_template_eids, and call DetermineApproximatePages.
	// See CreateApproximatePageListWithPosInfo for the core logic.
}

// CreateApproximatePageListWithPosInfo generates page numbers using provided pos_info.
// This is a convenience function for testing.
func CreateApproximatePageListWithPosInfo(posInfo []*ContentChunk, pageTemplateEIDs map[interface{}]bool,
	firstSectionName string, desiredNumPages int, isFixedLayout bool) []map[string]interface{} {

	if len(posInfo) == 0 {
		return nil
	}

	var pages []map[string]interface{}

	if isFixedLayout {
		pages, _ = DetermineApproximatePages(posInfo, pageTemplateEIDs, firstSectionName, 999999, true)
	} else if desiredNumPages == 0 {
		pages, _ = DetermineApproximatePages(posInfo, pageTemplateEIDs, firstSectionName, TYPICAL_POSITIONS_PER_PAGE, false)
	} else {
		minPPP := MIN_POSITIONS_PER_PAGE
		maxPPP := MAX_POSITIONS_PER_PAGE

		for minPPP <= maxPPP {
			positionsPerPage := (minPPP + maxPPP) / 2
			pages, _ = DetermineApproximatePages(posInfo, pageTemplateEIDs, firstSectionName, positionsPerPage, false)

			if len(pages) == desiredNumPages {
				break
			} else if len(pages) > desiredNumPages {
				minPPP = positionsPerPage + 1
			} else {
				maxPPP = positionsPerPage - 1
			}
		}
	}

	return pages
}

// unicodeLen returns the number of runes (Unicode code points) in a string,
// matching Python's len() for Unicode strings and utilities.unicode_len.
func unicodeLen(s string) int {
	return utf8.RuneCountInString(s)
}

// maxInt returns the larger of two ints. (minInt is in content_helpers.go)
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Ensure math is available for unused imports guard.
var _ = math.Pi

// Sort interface for ContentChunk by PID
type byPID []*ContentChunk

func (a byPID) Len() int           { return len(a) }
func (a byPID) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byPID) Less(i, j int) bool { return a[i].PID < a[j].PID }

// SortContentChunksByPID sorts a slice of ContentChunk by PID.
func SortContentChunksByPID(chunks []*ContentChunk) {
	sort.Sort(byPID(chunks))
}

// CollectContentPositionInfo collects position info by walking section content.
// Port of Python collect_content_position_info (yj_position_location.py lines 128–577).
// This is a simplified version that handles the core logic.
func (bpl *BookPosLoc) CollectContentPositionInfo(keepFootnoteRefs, skipNonRenderedContent, includeBackgroundImages bool) []*ContentChunk {
	// Full implementation would recursively walk section content,
	// tracking eid→section mapping, building ContentChunks, merging
	// consecutive same-eid chunks, and handling conditional templates.
	//
	// The full Python function is ~450 lines with deeply nested closures.
	// This simplified version returns an empty list for testing the other
	// functions that consume pos_info.
	return nil
}
