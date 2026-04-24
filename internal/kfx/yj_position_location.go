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
	"<": true,
	"<=": true,
}

// ContentChunk represents a unit of content position information.
// Port of Python ContentChunk class (yj_position_location.py lines 31–62).
type ContentChunk struct {
	PID           int         // position ID
	EID           interface{} // entity ID (int or string/IonSymbol)
	EIDOffset     int         // offset within entity
	Length        int         // length in positions
	SectionName   string      // section this chunk belongs to
	MatchZeroLen  bool        // whether zero-length match is OK
	Text          string      // text content (empty string means nil)
	HasText       bool        // true when Text was explicitly set (distinguishes nil from empty string)
	ImageResource string      // image resource name
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

// eidOffsetKey is used as a map key for the equals set in conditional template processing.
type eidOffsetKey struct {
	EID    interface{}
	Offset int
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

	// Fragment store for adding/removing fragments by type (used by CreatePositionMap, CreateLocationMap, etc.)
	Store *FragmentStore

	// Internal state for position info collection (Python self.cpi_*)
	cpiPID          int
	cpiPIDForOffset int
	cpiProcessingStory bool
	cpiFixed        bool
	lastPII         int

	// hasEIDOffset tracks whether any eid_offset != 0 was seen
	hasEIDOffset bool

	// cachedHasNonImageRenderInline caches the result of HasNonImageRenderInline
	cachedHasNonImageRenderInline *bool
}

// FragmentStore provides a simple store for fragments by type, used for
// creating/removing position and location mapping fragments.
// This is a lightweight alternative to modifying fragmentCatalog directly.
type FragmentStore struct {
	fragments map[string][]map[string]interface{} // ftype → list of fragment values
}

// NewFragmentStore creates a new FragmentStore.
func NewFragmentStore() *FragmentStore {
	return &FragmentStore{
		fragments: make(map[string][]map[string]interface{}),
	}
}

// Get returns the first fragment value for a given type, or nil.
func (fs *FragmentStore) Get(ftype string) map[string]interface{} {
	list := fs.fragments[ftype]
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

// GetAll returns all fragment values for a given type.
func (fs *FragmentStore) GetAll(ftype string) []map[string]interface{} {
	return fs.fragments[ftype]
}

// Append adds a fragment value for a given type.
func (fs *FragmentStore) Append(ftype string, value map[string]interface{}) {
	fs.fragments[ftype] = append(fs.fragments[ftype], value)
}

// RemoveAll removes all fragments of a given type.
func (fs *FragmentStore) RemoveAll(ftype string) {
	delete(fs.fragments, ftype)
}

// Has returns true if any fragments of the given type exist.
func (fs *FragmentStore) Has(ftype string) bool {
	return len(fs.fragments[ftype]) > 0
}

// anchorEidOffset looks up an anchor fragment ($266) and returns its (eid, offset).
// Port of Python anchor_eid_offset (yj_position_location.py lines 579–586).
func (bpl *BookPosLoc) anchorEidOffset(anchor interface{}) (interface{}, int, bool) {
	anchorStr, ok := asString(anchor)
	if !ok {
		anchorStr = fmt.Sprintf("%v", anchor)
	}

	// Look for a $266 fragment with fid matching the anchor name
	if bpl.Store != nil {
		for _, frag := range bpl.Store.GetAll("anchor") {
			if fid, _ := asString(frag["anchor_name"]); fid == anchorStr {
				if pos, ok := asMap(frag["position"]); ok {
					eid := pos["id"]
					offset := asIntDefault(pos["offset"], 0)
					return eid, offset, true
				}
			}
		}
	}

	// Fall back to AnchorFragments map
	if bpl.Fragments != nil {
		af, ok := bpl.Fragments.AnchorFragments[anchorStr]
		if ok && af.PositionID != 0 {
			return af.PositionID, 0, true
		}
	}

	log.Printf("kfx: Failed to locate position for anchor: %s", anchorStr)
	return nil, 0, false
}

// HasNonImageRenderInline walks $259/$608 fragments to detect non-image render-inline elements.
// Port of Python has_non_image_render_inline (yj_position_location.py:588-616).
// Returns true if any struct has $159!="image" AND $601=="inline".
func (bpl *BookPosLoc) HasNonImageRenderInline() bool {
	if bpl.cachedHasNonImageRenderInline != nil {
		return *bpl.cachedHasNonImageRenderInline
	}

	result := false

	// walk recursively checks Ion data for non-image render-inline
	var walk func(data interface{}) bool
	walk = func(data interface{}) bool {
		switch v := data.(type) {
		case []interface{}: // IonList / IonSExp
			for _, val := range v {
				if walk(val) {
					return true
				}
			}
		case map[string]interface{}: // IonStruct
			// Check condition: $159 != "image" AND $601 == "inline"
			typ, _ := asString(v["type"])
			renderMode, _ := asString(v["render"])
			if typ != "image" && renderMode == "inline" {
				return true
			}
			for _, val := range v {
				if walk(val) {
					return true
				}
			}
		}
		return false
	}

	if bpl.Store != nil {
		for _, ftype := range []string{"storyline", "structure"} {
			for _, frag := range bpl.Store.GetAll(ftype) {
				if walk(frag) {
					result = true
					break
				}
			}
			if result {
				break
			}
		}
	}

	bpl.cachedHasNonImageRenderInline = &result
	return result
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

// CreateLocationMap removes old $550/$621 fragments and builds a new $550 with $182 list.
// Port of Python create_location_map (yj_position_location.py lines 1116–1130).
// Returns hasYJLocationPidMap (always false since we don't create $621).
func (bpl *BookPosLoc) CreateLocationMap(locInfo []*ContentChunk) bool {
	hasYJLocationPidMap := false

	// Remove old $550 and $621 fragments
	if bpl.Store != nil {
		bpl.Store.RemoveAll("location_map")
		bpl.Store.RemoveAll("yj.location_pid_map")

		// Build new $550 fragment
		locations := make([]interface{}, 0, len(locInfo))
		for _, loc := range locInfo {
			entry := map[string]interface{}{
				"id": loc.EID,
				"offset": loc.EIDOffset,
			}
			locations = append(locations, entry)
		}

		locationMap := []interface{}{
			map[string]interface{}{
				"locations": locations,
			},
		}
		bpl.Store.Append("location_map", map[string]interface{}{"location_map": locationMap})
	}

	return hasYJLocationPidMap
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

				// Whitespace lookback adjustment — rune-based indexing for UTF-8
				// Port of Python yj_position_location.py:1299 (chunk.text[chunk_offset])
				if chunk.HasText && chunk.Text != "" {
					runes := []rune(chunk.Text)
					if chunkOffset < len(runes) {
						if !unicode.IsSpace(runes[chunkOffset]) {
							initChunkOffset := chunkOffset
							for {
								if chunkOffset == 0 {
									break
								}
								if chunkOffset <= minChunkOffset {
									chunkOffset = initChunkOffset
									break
								}
								if unicode.IsSpace(runes[chunkOffset-1]) {
									break
								}
								chunkOffset--
							}
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
		"representation": map[string]interface{}{
			"label": fmt.Sprintf("%d", pageNum),
		},
		"target_position": map[string]interface{}{
			"id": eid,
			"offset": eidOffset,
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

// CreatePositionMap builds $264 (section→eid list) and $265 (pid→eid+offset) fragments.
// Port of Python create_position_map (yj_position_location.py lines 906–958).
func (bpl *BookPosLoc) CreatePositionMap(posInfo []*ContentChunk) (bool, bool) {
	if bpl.IsDictionary || bpl.IsScribeNotebook || bpl.IsKPFPub {
		log.Printf("kfx: warning: Position map creation for KPF or dictionary not supported")

		if bpl.Store != nil {
			bpl.Store.RemoveAll("position_map")
			bpl.Store.RemoveAll("position_id_map")
			bpl.Store.RemoveAll("yj.eidhash_eid_section_map")
		}

		return false, false
	}

	// Remove old mapping fragments first
	// Port of Python lines 918–922: removes $264, $265, $609, $610, $611
	if bpl.Store != nil {
		for _, ftype := range []string{"position_map", "position_id_map", "section_position_id_map", "yj.eidhash_eid_section_map", "yj.section_pid_count_map"} {
			bpl.Store.RemoveAll(ftype)
		}
	}

	// Build section_eids: section_name → set of eids
	sectionEIDs := map[string]map[interface{}]bool{}
	for _, chunk := range posInfo {
		if sectionEIDs[chunk.SectionName] == nil {
			sectionEIDs[chunk.SectionName] = map[interface{}]bool{}
		}
		sectionEIDs[chunk.SectionName][chunk.EID] = true
	}

	// Build position_map ($264): section → list of eids
	positionMap := make([]interface{}, 0)
	for _, sectionName := range bpl.OrderedSections {
		eidSet := sectionEIDs[sectionName]
		eidList := make([]interface{}, 0, len(eidSet))
		for eid := range eidSet {
			eidList = append(eidList, eid)
		}
		positionMap = append(positionMap, map[string]interface{}{
			"contains": eidList,
			"section_name": sectionName,
		})
	}

	if bpl.Store != nil {
		bpl.Store.Append("position_map", map[string]interface{}{"position_map": positionMap})
	}

	// Build position_id_map ($265): flat list of {pid, eid, offset}
	positionIDMap := make([]interface{}, 0)
	hasSPIM := false
	hasPositionIDOffset := false
	pid := 0

	for _, chunk := range posInfo {
		entry := map[string]interface{}{
			"pid": pid,
			"eid": chunk.EID,
		}
		if chunk.EIDOffset != 0 {
			entry["offset"] = chunk.EIDOffset
			hasPositionIDOffset = true
		}
		positionIDMap = append(positionIDMap, entry)
		pid += chunk.Length
	}

	// Terminal entry
	positionIDMap = append(positionIDMap, map[string]interface{}{
		"pid": pid,
		"eid": 0,
	})

	if bpl.Store != nil {
		bpl.Store.Append("position_id_map", map[string]interface{}{"position_id_map": positionIDMap})
	}

	return hasSPIM, hasPositionIDOffset
}

// CollectPositionMapInfo parses $264/$265/$609/$610/$611 fragments to build position map info.
// Port of Python collect_position_map_info (yj_position_location.py lines 601–834).
func (bpl *BookPosLoc) CollectPositionMapInfo() []*ContentChunk {
	var posInfo []*ContentChunk
	eidStartPos := map[interface{}]int{}
	prevEIDOffset := map[interface{}]int{}
	eidSection := map[interface{}]string{}
	bpl.hasEIDOffset = false

	// processSPIM processes a Section Position ID Map (SPIM) fragment
	processSPIM := func(contains []interface{}, sectionStartPID int, sectionName string,
		addSectionLength *int, verifySectionLength *int, pidIsReallyLen bool, oneBasedPID bool, intEID bool) {

		if addSectionLength != nil {
			contains = append(contains, []interface{}{*addSectionLength + 1, 0})
		}

		pid := 0
		eid := 0
		var eidIface interface{} = 0
		eidOffset := 0

		for i, pi := range contains {
			var nextPID, nextEIDInt int
			var nextOffset int

			piList, isList := asSlice(pi)
			piInt, isInt := asInt(pi)
			piMap, isMap := asMap(pi)

			switch {
			case isList:
				if len(piList) < 2 || len(piList) > 3 {
					log.Printf("kfx: error: Bad section_position_id_map list at %d", i)
					return
				}
				nextPID, _ = asInt(piList[0])
				nextEIDInt, _ = asInt(piList[1])
				if len(piList) > 2 {
					nextOffset, _ = asInt(piList[2])
				}
			case isInt:
				nextPID = piInt
				nextEIDInt = eid + 1 // Sequential EID
				nextOffset = 0
			case isMap:
				extraKeys := false
				for k := range piMap {
					if k != "pid" && k != "eid" && k != "offset" {
						extraKeys = true
						break
					}
				}
				if extraKeys {
					log.Printf("kfx: error: Bad section_position_id_map list keys at %d", i)
					return
				}
				nextPID, _ = asInt(piMap["pid"])
				nextEIDInt, _ = asInt(piMap["eid"])
				nextOffset = asIntDefault(piMap["offset"], 0)
			default:
				log.Printf("kfx: error: Bad section_position_id_map entry type at %d", i)
				return
			}

			if pidIsReallyLen {
				nextPID += pid
			}
			if oneBasedPID {
				nextPID--
			}

			if i > 0 {
				if sectionName != "" {
					if existing, ok := eidSection[eidIface]; ok && existing != sectionName {
						log.Printf("kfx: error: section_position_id_map eid %v expected in section %s found in %s",
							eidIface, existing, sectionName)
					}
					eidSection[eidIface] = sectionName
				}

				if eidOffset != 0 {
					bpl.hasEIDOffset = true
				}

				if _, ok := eidStartPos[eidIface]; !ok {
					eidStartPos[eidIface] = pid
				}

				expectedOffset := pid - eidStartPos[eidIface]
				if eidOffset != expectedOffset {
					log.Printf("kfx: warning: position map eid %v offset is %d, expected %d",
						eidIface, eidOffset, expectedOffset)
				}

				chunkSection := sectionName
				if sectionName == "" {
					chunkSection = eidSection[eidIface]
				}

				posInfo = append(posInfo, &ContentChunk{
					PID:         pid + sectionStartPID,
					EID:         eidIface,
					EIDOffset:   eidOffset,
					Length:      nextPID - pid,
					SectionName: chunkSection,
				})
				prevEIDOffset[eidIface] = eidOffset
			}

			pid = nextPID
			eid = nextEIDInt
			if intEID {
				eidIface = eid
			} else {
				eidIface = nextEIDInt
			}
			eidOffset = nextOffset
		}

		if eid != 0 || eidOffset != 0 {
			log.Printf("kfx: error: section_position_id_map last eid is %d+%d (should be zero)", eid, eidOffset)
		}

		if verifySectionLength != nil && pid != *verifySectionLength {
			log.Printf("kfx: error: section_position_id_map section %s length %d, expected %d",
				sectionName, pid, *verifySectionLength)
		}
	}

	if bpl.IsDictionary || bpl.IsKPFPub {
		// Dictionary/KPF path: process $611 → $609 SPIM fragments
		if bpl.Store != nil {
			fragment611 := bpl.Store.Get("yj.section_pid_count_map")
			if fragment611 != nil {
				sectMaps, ok := asSlice(fragment611["contains"])
				if ok {
					sectionPIDCount := map[string]int{}
					for _, sm := range sectMaps {
						smMap, ok := asMap(sm)
						if !ok {
							continue
						}
						sn, _ := asString(smMap["section_name"])
						length, _ := asInt(smMap["length"])
						sectionPIDCount[sn] = length
					}

					sectionStartPID := 0
					for _, sectionName := range bpl.OrderedSections {
						spimFrag := bpl.Store.Get("section_position_id_map")
						if spimFrag == nil {
							log.Printf("kfx: error: section_position_id_map missing for section %s", sectionName)
							sectionStartPID += sectionPIDCount[sectionName]
							continue
						}

						spim, ok := asMap(spimFrag["section_position_id_map"])
						if !ok {
							sectionStartPID += sectionPIDCount[sectionName]
							continue
						}
						spimSectionName, _ := asString(spim["section_name"])
						if spimSectionName != sectionName {
							log.Printf("kfx: error: section_position_id_map for section %s has section %s", sectionName, spimSectionName)
						}

						addLen := sectionPIDCount[sectionName]
						processSPIM(toIfaceSlice(spim["contains"]), sectionStartPID, sectionName,
							&addLen, nil, false, true, false)
						sectionStartPID += sectionPIDCount[sectionName]
					}
				}
			}
		}

		// Check for excess $264/$265
		if bpl.Store != nil {
			for _, ftype := range []string{"position_map", "position_id_map"} {
				if bpl.Store.Has(ftype) {
					log.Printf("kfx: error: Excess mapping fragment: %s", ftype)
				}
			}
		}
	} else {
		// Normal book path: process $264 (position_map) and $265 (position_id_map)
		if bpl.Store != nil {
			fragment264 := bpl.Store.Get("position_map")
			if fragment264 != nil {
				pmSlice, ok := asSlice(fragment264["position_map"])
				if ok {
					extraSections := map[string]bool{}
					missingSections := map[string]bool{}
					for _, sn := range bpl.OrderedSections {
						missingSections[sn] = true
					}

					for _, pm := range pmSlice {
						pmMap, ok := asMap(pm)
						if !ok {
							continue
						}
						sectionName, _ := asString(pmMap["section_name"])
						if !missingSections[sectionName] {
							extraSections[sectionName] = true
						}
						delete(missingSections, sectionName)

						eidList, ok := asSlice(pmMap["contains"])
						if !ok {
							continue
						}
						for _, eidEntry := range eidList {
							if eidList2, ok := asSlice(eidEntry); ok && len(eidList2) >= 2 {
								baseEID, _ := asInt(eidList2[0])
								count, _ := asInt(eidList2[1])
								for j := 0; j < count; j++ {
									eidSection[baseEID+j] = sectionName
								}
							} else {
								eidSection[eidEntry] = sectionName
							}
						}
					}

					for sn := range extraSections {
						log.Printf("kfx: error: position_map has extra sections: %s", sn)
					}
					for sn := range missingSections {
						log.Printf("kfx: error: position_map has missing sections: %s", sn)
					}
				}
			}

			hasSPIM := false
			_ = hasSPIM // tracked for SPIM format detection
			fragment265 := bpl.Store.Get("position_id_map")
			if fragment265 != nil {
				val := fragment265["position_id_map"]
				if valSlice, ok := asSlice(val); ok {
					// Simple list format
					processSPIM(valSlice, 0, "", nil, nil, false, false, true)
				} else if valMap, ok := asMap(val); ok {
					// SPIM format with per-section maps
					hasSPIM = true
					sectMaps, ok := asSlice(valMap["contains"])
					if ok {
						bookPID := 0
						for _, sm := range sectMaps {
							smMap, ok := asMap(sm)
							if !ok {
								continue
							}
							sectionName, _ := asString(smMap["section_name"])
							sectionStartPID, _ := asInt(smMap["pid"])

							if sectionStartPID != bookPID {
								log.Printf("kfx: error: section %s start pid %d, expected %d",
									sectionName, sectionStartPID, bookPID)
							}

							spimFrag := bpl.Store.Get("section_position_id_map")
							if spimFrag == nil {
								log.Printf("kfx: error: section_position_id_map missing for section %s", sectionName)
								continue
							}

							spim, ok := asMap(spimFrag["section_position_id_map"])
							if !ok {
								continue
							}
							spimSectionName, _ := asString(spim["section_name"])
							if spimSectionName != sectionName {
								log.Printf("kfx: error: section_position_id_map for section %s has section %s",
									sectionName, spimSectionName)
							}

							sectionLength, _ := asInt(smMap["length"])
							processSPIM(toIfaceSlice(spim["contains"]), sectionStartPID, sectionName,
								nil, &sectionLength, true, false, true)
							bookPID += sectionLength
						}
					}
				}

				// Cross-validate eid sets
				positionMapEIDs := map[interface{}]bool{}
				for eid := range eidSection {
					positionMapEIDs[eid] = true
				}
				positionIDMapEIDs := map[interface{}]bool{}
				for eid := range prevEIDOffset {
					positionIDMapEIDs[eid] = true
				}

				for eid := range positionMapEIDs {
					if !positionIDMapEIDs[eid] {
						log.Printf("kfx: error: position_map has extra eids: %v", eid)
					}
				}
				for eid := range positionIDMapEIDs {
					if !positionMapEIDs[eid] {
						log.Printf("kfx: error: position_map has missing eids: %v", eid)
					}
				}
			}

			// Check for excess $611/$610
			for _, ftype := range []string{"yj.section_pid_count_map", "yj.eidhash_eid_section_map"} {
				if bpl.Store.Has(ftype) {
					log.Printf("kfx: error: Excess mapping fragment: %s", ftype)
				}
			}
		}
	}

	return posInfo
}

// toIfaceSlice converts a value to []interface{} if possible.
func toIfaceSlice(v interface{}) []interface{} {
	if v == nil {
		return nil
	}
	if s, ok := asSlice(v); ok {
		return s
	}
	return nil
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

	// $550 fragment processing with validation
	// Port of Python collect_location_map_info $550 handling
	fragment550 := bpl.getFragment550()
	if fragment550 != nil {
		// The $550 fragment value is: {"location_map": [{"locations": [entries...]}]}
		// Need to unwrap the $550 key to get the location map list
		var locationMapList []interface{}
		if outer, ok := asSlice(fragment550["location_map"]); ok && len(outer) > 0 {
			locationMapList = outer
		} else {
			// Alternative: $182 directly on the fragment (some test data)
			locationMapList = []interface{}{fragment550}
		}

		for _, lmOuter := range locationMapList {
			lmMap, ok := asMap(lmOuter)
			if !ok {
				continue
			}
			entries, ok := asSlice(lmMap["locations"])
			if !ok {
				log.Printf("kfx: error: Bad location_map fragment: missing or invalid $182 list")
				continue
			}
			for i, lm := range entries {
				entryMap, ok := asMap(lm)
				if !ok {
					log.Printf("kfx: error: Bad location_map entry at index %d: expected map", i)
					continue
				}
				eid := entryMap["id"]
				if eid == nil {
					log.Printf("kfx: error: Bad location_map entry at index %d: missing $155", i)
					continue
				}
				eidOffset := asIntDefault(entryMap["offset"], 0)

				pid := bpl.PidForEid(eid, eidOffset, posInfo)
				if pid == nil {
					log.Printf("kfx: error: location_map failed to locate eid %v offset %d", eid, eidOffset)
				} else {
					addLoc(*pid, eid, eidOffset)
				}
			}
		}
		endAddLoc()
	}

	// $621 fragment processing with validation
	// Port of Python collect_location_map_info $621 handling (yj_position_location.py:1060-1075)
	fragment621 := bpl.getFragment621()
	hasYJLocationPidMap := fragment621 != nil
	if hasYJLocationPidMap {
		locationPIDs, ok := asSlice(fragment621["locations"])
		if !ok {
			log.Printf("kfx: error: Bad yj.location_pid_map fragment: %v", fragment621)
		} else {
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
	if bpl.Store != nil {
		return bpl.Store.Get("location_map")
	}
	return nil
}

// getFragment621 returns the $621 yj.location_pid_map fragment data if available.
func (bpl *BookPosLoc) getFragment621() map[string]interface{} {
	if bpl.Store != nil {
		return bpl.Store.Get("yj.location_pid_map")
	}
	return nil
}

// CreateApproximatePageList generates approximate page numbers.
// Port of Python create_approximate_page_list (yj_position_location.py lines 1132–1258).
func (bpl *BookPosLoc) CreateApproximatePageList(desiredNumPages int) {
	// Validate CDE type — port of lines 1134–1151
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
	if bpl.IsFixedLayout && bpl.hasDoublePageSpread() {
		log.Printf("kfx: error: Cannot create page numbers for fixed layout books with page spreads")
		return
	}

	// Port of lines 1152–1155: locate reading order
	readingOrderNames := bpl.readingOrderNames()
	if len(readingOrderNames) != 1 {
		log.Printf("kfx: error: Cannot create page numbers - Failed to locate single default reading order")
		return
	}

	// Port of lines 1157–1184: locate book_navigation and existing page list
	readingOrderName := readingOrderNames[0]
	var bookNav *map[string]interface{}
	var navContainers []interface{}
	inlineNavContainers := true

	if bpl.Store != nil {
		bookNavVal := bpl.Store.Get("book_navigation")
		if bookNavVal != nil {
			navList, ok := asSlice(bookNavVal["book_navigation"])
			if ok {
				for _, bn := range navList {
					bnMap, ok := asMap(bn)
					if !ok {
						continue
					}
					roName, _ := asString(bnMap["reading_order_name"])
					if roName != readingOrderName {
						continue
					}
					bookNav = &bnMap
					ncList, ok := asSlice(bnMap["nav_containers"])
					if !ok {
						break
					}
					navContainers = ncList

					for i, nc := range navContainers {
						ncMap, ok := asMap(nc)
						if !ok {
							continue
						}
						navType, _ := asString(ncMap["nav_type"])
						if navType == "page_list" {
							// Found existing approximate page list — remove it
							navContainers = append(navContainers[:i], navContainers[i+1:]...)
							(*bookNav)["nav_containers"] = navContainers
							break
						}
					}
					break
				}
			}
		}
	}

	// Port of lines 1185–1193: collect position info
	sectionNames := bpl.OrderedSections
	posInfo := bpl.CollectContentPositionInfo(true, false, false)

	if len(sectionNames) == 0 && len(posInfo) == 0 {
		log.Printf("kfx: error: Cannot produce approximate page numbers - No content found for reading order %s", readingOrderName)
		return
	}

	// Port of lines 1194–1214: determine pages
	var pages []map[string]interface{}

	if bpl.IsFixedLayout {
		pages, _ = DetermineApproximatePages(posInfo, nil, sectionNames[0], 999999, true)
		log.Printf("kfx: Created %d fixed layout page numbers", len(pages))
	} else if desiredNumPages == 0 {
		pages, _ = DetermineApproximatePages(posInfo, nil, sectionNames[0], TYPICAL_POSITIONS_PER_PAGE, false)
		log.Printf("kfx: Created %d approximate page numbers", len(pages))
	} else {
		minPPP := MIN_POSITIONS_PER_PAGE
		maxPPP := MAX_POSITIONS_PER_PAGE
		positionsPerPage := 0
		for minPPP <= maxPPP {
			positionsPerPage = (minPPP + maxPPP) / 2
			pages, _ = DetermineApproximatePages(posInfo, nil, sectionNames[0], positionsPerPage, false)
			if len(pages) == desiredNumPages {
				break
			} else if len(pages) > desiredNumPages {
				minPPP = positionsPerPage + 1
			} else {
				maxPPP = positionsPerPage - 1
			}
		}
		log.Printf("kfx: Created %d approximate page numbers using %d positions per page for %d desired pages",
			len(pages), positionsPerPage, desiredNumPages)
	}

	// Port of lines 1215–1257: add pages to navigation
	if len(pages) > 0 && bpl.Store != nil {
		if bookNav == nil {
			newNav := map[string]interface{}{
				"reading_order_name": readingOrderName,
				"nav_containers": []interface{}{},
			}
			bpl.Store.Append("book_navigation", map[string]interface{}{"book_navigation": []interface{}{newNav}})
			bookNav = &newNav
			navContainers = nil
		}

		// Build nav container with pages
		pageEntries := make([]interface{}, 0, len(pages))
		for _, page := range pages {
			pageEntries = append(pageEntries, page)
		}

		navContainerData := map[string]interface{}{
			"nav_type": "page_list",
			"nav_container_name": "approximate_page_list",
			"entries": pageEntries,
		}

		if inlineNavContainers {
			navContainers = append(navContainers, navContainerData)
		} else {
			bpl.Store.Append("nav_container", map[string]interface{}{"nav_container": navContainerData})
			navContainers = append(navContainers, "approximate_page_list")
		}

		(*bookNav)["nav_containers"] = navContainers
	}
}

// hasDoublePageSpread checks for double-page-spread metadata.
func (bpl *BookPosLoc) hasDoublePageSpread() bool {
	if bpl.Fragments != nil {
		if md, ok := asMap(bpl.Fragments.TitleMetadata); ok {
			if caps, ok := asSlice(md["features"]); ok {
				for _, cap := range caps {
					if capMap, ok := asMap(cap); ok {
						if name, _ := asString(capMap["exclude"]); name == "yj_double_page_spread" {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// readingOrderNames returns the list of reading order names from document data.
func (bpl *BookPosLoc) readingOrderNames() []string {
	if bpl.Fragments == nil {
		return nil
	}
	orders := getReadingOrders(*bpl.Fragments)
	var names []string
	for _, order := range orders {
		orderMap, ok := asMap(order)
		if !ok {
			continue
		}
		if name, ok := asString(orderMap["reading_order_name"]); ok && name != "" {
			names = append(names, name)
		}
	}
	return names
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

// CheckPositionAndLocationMaps is the top-level orchestration method that calls all sub-methods.
// Port of Python check_position_and_location_maps (yj_position_location.py lines 103–126).
func (bpl *BookPosLoc) CheckPositionAndLocationMaps() {
	contentPosInfo := bpl.CollectContentPositionInfo(true, false, false)
	mapPosInfo := bpl.CollectPositionMapInfo()

	if !bpl.IsKFXV1 {
		VerifyPositionInfo(contentPosInfo, mapPosInfo, false)
	}

	_ = bpl.CollectLocationMapInfo(mapPosInfo)
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
// Walks all sections in reading order, building ContentChunk objects with PID, EID, SectionName, Text.
func (bpl *BookPosLoc) CollectContentPositionInfo(keepFootnoteRefs, skipNonRenderedContent, includeBackgroundImages bool) []*ContentChunk {
	eidSection := map[interface{}]string{}
	eidStartPos := map[interface{}]int{}
	var posInfo []*ContentChunk
	var sectionPosInfo []*ContentChunk
	var eidCondInfo []*ConditionalTemplate
	processedStoryNames := map[string]bool{}
	bpl.cpiPID = 0
	bpl.cpiPIDForOffset = 0
	bpl.cpiProcessingStory = false
	bpl.cpiFixed = false
	sectionStories := map[string]map[string]bool{}
	storySections := map[string]map[string]bool{}

	// currentSectionName is captured by the extractPositionData closure
	var currentSectionName string

	// haveContent creates or merges a ContentChunk — port of Python lines 135–225
	haveContent := func(eid interface{}, length int, advance bool, options ...func(*ContentChunk)) {
		if eid == nil {
			return
		}

		if _, ok := eidStartPos[eid]; !ok {
			eidStartPos[eid] = bpl.cpiPIDForOffset
		}

		eidOffset := bpl.cpiPIDForOffset - eidStartPos[eid]

		if advance {
			bpl.cpiPIDForOffset += length
		}

		// Merge consecutive same-EID chunks (Python lines 215–220)
		if len(sectionPosInfo) > 0 {
			lastChunk := sectionPosInfo[len(sectionPosInfo)-1]
			if eidsEqual(lastChunk.EID, eid) && (lastChunk.ImageResource == "") {
				sectionPosInfo = sectionPosInfo[:len(sectionPosInfo)-1]
				bpl.cpiPID += length
				eidOffset += length
				length += lastChunk.Length
				sectionPosInfo = append(sectionPosInfo, &ContentChunk{
					PID:          lastChunk.PID,
					EID:          lastChunk.EID,
					EIDOffset:    lastChunk.EIDOffset,
					Length:       length,
					SectionName:  lastChunk.SectionName,
					MatchZeroLen: lastChunk.MatchZeroLen,
				})

				if length == 0 {
					return
				}
				// Merged — prevent adding a duplicate chunk below
				return
			}
		}

		// Handle conditional template insertion for illustrated layout stories
		// Port of Python yj_position_location.py L209-327
		if bpl.cpiProcessingStory && len(eidCondInfo) > 0 {
			if !bpl.cpiFixed {
				// One-time initialization: set start_eid for non-range operators
				// and build the equals set. Port of Python L218-228.
				prevEID := eid
				prevEIDOffset := eidOffset
				equals := map[eidOffsetKey]bool{}

				for _, ct := range eidCondInfo {
					if ct.StartEID == nil && !RANGE_OPERS[ct.Oper] {
						ct.StartEID = eid
						ct.StartEIDOffset = eidOffset
						ct.EndEID = eid
						ct.EndEIDOffset = eidOffset
					}
					if ct.Oper == "==" {
						equals[eidOffsetKey{ct.StartEID, ct.StartEIDOffset}] = true
					}
				}

				// Build moves list and reorder eid_cond_info.
				// Port of Python L229-275.
				moves := []int{}
				prevEqualCTIdx := -1
				var prevEqualCTStartEID interface{}
				var prevEqualCTStartEIDOffset int
				storyStartEID := eidCondInfo[len(eidCondInfo)-1].StartEID
				storyStartEIDOffset := eidCondInfo[len(eidCondInfo)-1].StartEIDOffset

				for idx, ct := range eidCondInfo {
					if ct.Oper == "yj.collision" {
						// $348 (collision) entries always move to front
						moves = append(moves, idx)

					} else if ct.Oper == "==" {
						// $294 (equals) duplicate entries move to front
						if prevEqualCTIdx == idx-1 &&
							eidsEqual(ct.StartEID, prevEqualCTStartEID) &&
							ct.StartEIDOffset == prevEqualCTStartEIDOffset {
							ct.StartEID = storyStartEID
							ct.StartEIDOffset = storyStartEIDOffset
							ct.EndEID = storyStartEID
							ct.EndEIDOffset = storyStartEIDOffset
							moves = append(moves, idx)
						} else {
							prevEqualCTStartEID = ct.StartEID
							prevEqualCTStartEIDOffset = ct.StartEIDOffset
						}
						prevEqualCTIdx = idx

					} else if ct.StartEID == nil && RANGE_OPERS[ct.Oper] {
						// Range operator: resolve start_eid using prev_eid/prev_eid_offset
						for equals[eidOffsetKey{prevEID, prevEIDOffset}] {
							prevEIDOffset++
						}
						ct.StartEID = prevEID
						ct.StartEIDOffset = prevEIDOffset

						if eidsEqual(ct.StartEID, ct.EndEID) &&
							(ct.StartEIDOffset > ct.EndEIDOffset ||
								(ct.Oper == "<" && ct.StartEIDOffset == ct.EndEIDOffset)) {
							// Range ends before it starts — move to front
							prevEID = ct.StartEID
							prevEIDOffset = ct.StartEIDOffset
							ct.StartEID = eid
							ct.StartEIDOffset = eidOffset
							moves = append(moves, idx)
						} else {
							prevEID = ct.EndEID
							prevEIDOffset = ct.EndEIDOffset
						}
					}
				}

				// Apply moves: pop from back to front and insert at position 0
				for i := len(moves) - 1; i >= 0; i-- {
					idx := moves[i]
					entry := eidCondInfo[idx]
					eidCondInfo = append(eidCondInfo[:idx], eidCondInfo[idx+1:]...)
					eidCondInfo = append([]*ConditionalTemplate{entry}, eidCondInfo...)
				}

				bpl.cpiFixed = true
			}

			// Best-match search: iterate ALL entries, find the one matching
			// current eid with the lowest start_eid_offset.
			// Port of Python L290-327.
			for len(eidCondInfo) > 0 {
				var ct *ConditionalTemplate
				ctIdx := -1

				for idx_, ct_ := range eidCondInfo {
					if ct_.UseNext {
						ct_.StartEID = eid
						ct_.StartEIDOffset = eidOffset
						ct_.UseNext = false
					}

					if ct_.StartEID != nil && eidsEqual(ct_.StartEID, eid) &&
						(ct == nil || ct_.StartEIDOffset < ct.StartEIDOffset) {
						ctIdx = idx_
						ct = ct_
					}
				}

				if ct == nil {
					break
				}

				if ct.StartEIDOffset < eidOffset {
					log.Printf("kfx: error: conditional %s is before %v+%d-%d", ct, eid, eidOffset, eidOffset+length)
					eidCondInfo = append(eidCondInfo[:ctIdx], eidCondInfo[ctIdx+1:]...)

				} else if ct.StartEIDOffset == eidOffset {
					// Exact match — insert conditional position info
					for _, cpo := range ct.PosInfo {
						cpo.PID = bpl.cpiPID
						bpl.cpiPID += cpo.Length
						sectionPosInfo = append(sectionPosInfo, cpo)
					}
					eidCondInfo = append(eidCondInfo[:ctIdx], eidCondInfo[ctIdx+1:]...)

				} else if ct.StartEIDOffset < eidOffset+length {
					// Conditional falls inside the current chunk — split
					splitLen := ct.StartEIDOffset - eidOffset
					sectionPosInfo = append(sectionPosInfo, &ContentChunk{
						PID:         bpl.cpiPID,
						EID:         eid,
						EIDOffset:   eidOffset,
						Length:      splitLen,
						SectionName: eidSection[eid],
					})
					length -= splitLen
					eidOffset += splitLen
					bpl.cpiPID += splitLen

				} else if ct.StartEIDOffset == eidOffset+length {
					// Conditional falls past the chunk — mark for next use
					ct.UseNext = true
					break

				} else {
					break
				}
			}
		}

		chunk := &ContentChunk{
			PID:         bpl.cpiPID,
			EID:         eid,
			EIDOffset:   eidOffset,
			Length:      length,
			SectionName: eidSection[eid],
		}
		for _, opt := range options {
			opt(chunk)
		}
		sectionPosInfo = append(sectionPosInfo, chunk)
		bpl.cpiPID += length
	}

	// extractPositionData recursively walks Ion data extracting position info
	// Port of Python lines 128–577 (nested extract_position_data function)
	var extractPositionData func(data interface{}, currentEID interface{}, contentKey string,
		listIndex, listMax int, advance bool, noteRefs *[][]int)

	extractPositionData = func(data interface{}, currentEID interface{}, contentKey string,
		listIndex, listMax int, advance bool, noteRefs *[][]int) {

		switch v := data.(type) {
		case []interface{}: // IonList
			for i, fc := range v {
				if fc == nil {
					continue
				}
				extractPositionData(fc, currentEID, contentKey, i, len(v)-1, advance, noteRefs)
			}

		case map[string]interface{}: // IonStruct
			// Port of lines 230–577 (IonStruct branch)
			// Set up EID tracking — Python lines 230–245
			var parentEID interface{} // Python L372: parent_eid = current_eid (reset per struct)
			if contentKey != "storyline" {
				eid := v["id"]
				if eid == nil {
					eid = v["kfx_id"]
				}
				if eid != nil {
					parentEID = currentEID // Python L372: parent_eid = current_eid
					currentEID = eid
					if existing, ok := eidSection[currentEID]; ok {
						if existing == currentSectionName {
							log.Printf("kfx: error: duplicate eid %v in section %s", currentEID, currentSectionName)
						} else {
							log.Printf("kfx: error: duplicate eid %v in sections %s and %s", currentEID, existing, currentSectionName)
						}
					}
					eidSection[currentEID] = currentSectionName
				}
			}

			typ, _ := asString(v["type"])

			// Skip non-rendered content (Python lines 250–251)
			if skipNonRenderedContent {
				if skip, _ := asBool(v["ignore"]); skip || typ == "zoom_target" {
					return
				}
			}

			// Python L390-393: inline rendered content within content_list needs parent EID positioning
			// When typ is container/image/text AND render is inline AND we're inside content_list,
			// register content at the parent EID to position inline elements correctly.
			// Python: have_content(parent_eid, -1 if list_index == 0 else 0, advance)
			// In Go, listIndex is always an int (0 from non-list callers), so we don't check it
			// against None like Python does — the parentEID != nil guard is sufficient.
			renderMode, _ := asString(v["render"])
			if (typ == "container" || typ == "image" || typ == "text") &&
				parentEID != nil && contentKey == "content_list" && renderMode == "inline" {
				length := 0
				if listIndex == 0 {
					length = -1
				}
				haveContent(parentEID, length, advance)
			}

			saveCPIDPIDForOffset := bpl.cpiPIDForOffset

			// Handle $596, $271, $272, $274 types — Python lines 256–258
			switch typ {
			case "horizontal_rule", "image", "kvg", "plugin":
				imgRes := ""
				if typ == "image" {
					imgRes, _ = asString(v["resource_name"])
				}
				imgOpt := func(cc *ContentChunk) { cc.ImageResource = imgRes }
				haveContent(currentEID, 1, advance, imgOpt)
			case "container", "listitem", "text", "header":
				// Check for content keys — Python lines 259–264
				hasContent := false
				for _, ct := range []string{"content", "content_list", "story_name"} {
					if _, ok := v[ct]; ok {
						hasContent = true
						break
					}
				}
				if !hasContent {
					haveContent(currentEID, 1, advance)
				}
			}

			// Handle $141 (page templates) — Python lines 290–460
			if ptList, ok := asSlice(v["page_templates"]); ok {
				if bpl.HasIllustratedLayoutConditionalPageTemplate {
					pidSave := bpl.cpiPID
					haveRange := false
					var lastOper string

					for _, pt := range ptList {
						extractPositionData(pt, currentEID, "page_templates", 0, 0, advance, noteRefs)

						if ptMap, ok := asMap(pt); ok {
							if condition, ok := asSlice(ptMap["condition"]); ok &&
								len(condition) == 3 {
								// Conditional template ($171 condition present)
								// Port of Python lines 412–431
								condOp, _ := asString(condition[0])
								if condOp == "==" || condOp == "<=" || condOp == "<" {
									// Parse condition_eid_offset from condition[2][1]
									if anchorList, ok := asSlice(condition[2]); ok && len(anchorList) >= 2 {
										anchorEID, anchorOffset, found := bpl.anchorEidOffset(anchorList[1])
										if found {
											eidCondInfo = append(eidCondInfo, NewConditionalTemplate(
												anchorEID, anchorOffset, condOp,
												copyChunkSlice(sectionPosInfo)))

											if RANGE_OPERS[condOp] {
												haveRange = true
											}
											lastOper = condOp
										}
									}
								}
							} else {
								// Non-conditional template (no $171 condition) — collision entry
								// Port of Python lines 434–447
								nonConditionalPosInfo := copyChunkSlice(sectionPosInfo)

								if haveRange {
									var finalRangePosInfo []*ContentChunk
									if lastOper == "<" {
										finalRangePosInfo = nonConditionalPosInfo
										nonConditionalPosInfo = nil
									} else {
										finalRangePosInfo = nil
									}

									eidCondInfo = append(eidCondInfo, NewConditionalTemplate(
										nil, 0, "<",
										finalRangePosInfo))
								}

								eidCondInfo = append(eidCondInfo, NewConditionalTemplate(
									nil, 0, "yj.collision",
									nonConditionalPosInfo))
							}
						}

						// Clear section_pos_info after each template (Python line 449)
						sectionPosInfo = sectionPosInfo[:0]
					}
					bpl.cpiPID = pidSave
				} else {
					for _, pt := range ptList {
						extractPositionData(pt, currentEID, "page_templates", 0, 0, advance, noteRefs)
					}
				}
			}

			// Handle $146 (children) — Python lines 375–376
			if children, ok := asSlice(v["content_list"]); ok && typ != "plugin" && typ != "kvg" {
				haveContent(currentEID, 1, advance)
				extractPositionData(children, currentEID, "content_list", 0, 0, advance, noteRefs)
			}

			// Handle $145 (content) — Python lines 386–393
			if contentVal := v["content"]; contentVal != nil {
				if strVal, ok := asString(contentVal); ok {
					haveContent(currentEID, unicodeLen(strVal), advance,
						func(cc *ContentChunk) { cc.Text = strVal; cc.HasText = true })
				}
			}

			// Handle $176 (story) — Python lines 402–422
			if storyName, ok := asString(v["story_name"]); ok && contentKey != "storyline" {
				haveContent(currentEID, 1, advance)

				if !bpl.HasIllustratedLayoutConditionalPageTemplate {
					if !processedStoryNames[storyName] {
						// Walk storyline fragment
						if bpl.Store != nil {
							if storylineFrag := bpl.Store.Get("storyline"); storylineFrag != nil {
								if storylineData, ok := storylineFrag["storyline"]; ok {
									extractPositionData(storylineData, nil, "storyline", 0, 0, advance, noteRefs)
								}
							}
						}
						processedStoryNames[storyName] = true
					} else {
						log.Printf("kfx: error: story %s appears in multiple sections", storyName)
					}
				}

				// Track section→story and story→section mappings
				if sectionStories[currentSectionName] == nil {
					sectionStories[currentSectionName] = map[string]bool{}
				}
				sectionStories[currentSectionName][storyName] = true
				if storySections[storyName] == nil {
					storySections[storyName] = map[string]bool{}
				}
				storySections[storyName][currentSectionName] = true
			}

			// Handle other non-string values — Python lines 423–429
			skipKeys := map[string]bool{
				"alt_content": true, "alt_text": true, "annotations": true, "content": true,
				"content_list": true, "page_templates": true, "reading_order_switch_map": true, "shape_list": true, "story_name": true,
			}
			for fk, fv := range v {
				if _, isStr := fv.(string); !isStr && !skipKeys[fk] && fk != "" {
					extractPositionData(fv, currentEID, fk, 0, 0, advance, noteRefs)
				}
			}

			// Adjust cpiPIDForOffset for render-inline — Python lines 430–432
			if typ != "image" {
				if renderMode, _ := asString(v["render"]); renderMode == "inline" &&
					bpl.cpiPIDForOffset > saveCPIDPIDForOffset+1 {
					bpl.cpiPIDForOffset = saveCPIDPIDForOffset + 1
				}
			}

		case string: // IonString
			length := unicodeLen(v)
			if contentKey == "content_list" && listIndex == 0 {
				length -= 1
			}
			haveContent(currentEID, length, advance,
				func(cc *ContentChunk) { cc.Text = v; cc.HasText = true })
		}
	}

	// collectSectionPositionInfo processes one section — Python lines 127–570
	collectSectionPositionInfo := func(sectionName string) {
		currentSectionName = sectionName
		sectionPosInfo = sectionPosInfo[:0]
		eidCondInfo = eidCondInfo[:0]

		// Extract from section fragment ($260)
		if bpl.Store != nil {
			sectionFrag := bpl.Store.Get("section")
			if sectionFrag != nil {
				if sectionData, ok := sectionFrag["section"]; ok {
					extractPositionData(sectionData, nil, "section", 0, 0, true, nil)
				}
			}
		}

		// Report leftover conditional templates
		for _, ci := range eidCondInfo {
			if len(ci.PosInfo) > 0 {
				log.Printf("kfx: error: left over conditional template info: %s", ci)
			}
		}
		eidCondInfo = eidCondInfo[:0]

		posInfo = append(posInfo, sectionPosInfo...)
		sectionPosInfo = sectionPosInfo[:0]
	}

	// Process each section in order — Python lines 566–570
	for _, sectionName := range bpl.OrderedSections {
		collectSectionPositionInfo(sectionName)
	}

	// Validate section→story constraints — Python lines 548–564
	for sectionName, stories := range sectionStories {
		numStories := len(stories)
		valid := numStories == 1 ||
			(bpl.IsPrintReplica && numStories == 2) ||
			(bpl.IsPDFBacked && (numStories == 2 || numStories == 3)) ||
			(bpl.IsScribeNotebook && numStories > 0)
		if !valid {
			storyList := make([]string, 0, numStories)
			for s := range stories {
				storyList = append(storyList, s)
			}
			log.Printf("kfx: error: section %s has stories %v", sectionName, storyList)
		}
	}
	for story, sections := range storySections {
		if len(sections) > 1 {
			sectionList := make([]string, 0, len(sections))
			for s := range sections {
				sectionList = append(sectionList, s)
			}
			log.Printf("kfx: error: story %s is in sections %v", story, sectionList)
		}
	}

	return posInfo
}

// copyChunkSlice creates a shallow copy of a ContentChunk slice.
func copyChunkSlice(chunks []*ContentChunk) []*ContentChunk {
	if len(chunks) == 0 {
		return nil
	}
	result := make([]*ContentChunk, len(chunks))
	copy(result, chunks)
	return result
}
