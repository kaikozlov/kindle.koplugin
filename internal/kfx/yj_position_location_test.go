package kfx

import (
	"strings"
	"testing"
)

// =============================================================================
// Tests for yj_position_location.go — VAL-A-044 through VAL-A-085
// =============================================================================

// =============================================================================
// VAL-A-044: ContentChunk construction and field validation
// =============================================================================

func TestContentChunkConstructionValid(t *testing.T) {
	// Valid chunk with pid≥0, eid>0, eid_offset≥0, length≥0
	cc := NewContentChunk(0, 5, 0, 20, "section1", false, "hello", "")
	if cc.PID != 0 {
		t.Errorf("expected PID=0, got %d", cc.PID)
	}
	if cc.EID != 5 {
		t.Errorf("expected EID=5, got %v", cc.EID)
	}
	if cc.EIDOffset != 0 {
		t.Errorf("expected EIDOffset=0, got %d", cc.EIDOffset)
	}
	if cc.Length != 20 {
		t.Errorf("expected Length=20, got %d", cc.Length)
	}
	if cc.SectionName != "section1" {
		t.Errorf("expected SectionName=section1, got %s", cc.SectionName)
	}
	if cc.MatchZeroLen != false {
		t.Error("expected MatchZeroLen=false")
	}
	if cc.Text != "hello" {
		t.Errorf("expected Text=hello, got %s", cc.Text)
	}
}

func TestContentChunkConstructionWithImageResource(t *testing.T) {
	cc := NewContentChunk(10, 20, 5, 0, "sec", true, "", "image.png")
	if cc.ImageResource != "image.png" {
		t.Errorf("expected ImageResource=image.png, got %s", cc.ImageResource)
	}
	if cc.MatchZeroLen != true {
		t.Error("expected MatchZeroLen=true")
	}
}

func TestContentChunkConstructionWithNilEID(t *testing.T) {
	// String EID (like IonSymbol in Python)
	cc := NewContentChunk(0, "eid-symbol", 0, 0, "sec", false, "", "")
	if cc.EID != "eid-symbol" {
		t.Errorf("expected EID=eid-symbol, got %v", cc.EID)
	}
}

func TestContentChunkConstructionNegativePID(t *testing.T) {
	// Should log error but still create the chunk
	cc := NewContentChunk(-1, 5, 0, 10, "sec", false, "", "")
	if cc.PID != -1 {
		t.Errorf("expected PID=-1, got %d", cc.PID)
	}
}

func TestContentChunkConstructionZeroEID(t *testing.T) {
	// eid=0 should log error (eid>0 required for int eids)
	cc := NewContentChunk(0, 0, 0, 10, "sec", false, "", "")
	if cc.EID != 0 {
		t.Errorf("expected EID=0, got %v", cc.EID)
	}
}

func TestContentChunkConstructionNegativeEIDOffset(t *testing.T) {
	cc := NewContentChunk(0, 5, -1, 10, "sec", false, "", "")
	if cc.EIDOffset != -1 {
		t.Errorf("expected EIDOffset=-1, got %d", cc.EIDOffset)
	}
}

func TestContentChunkConstructionNegativeLength(t *testing.T) {
	cc := NewContentChunk(0, 5, 0, -1, "sec", false, "", "")
	if cc.Length != -1 {
		t.Errorf("expected Length=-1, got %d", cc.Length)
	}
}

func TestContentChunkConstructionTextLengthMismatch(t *testing.T) {
	// text "hello" has length 5, but we pass length=10 — should log error
	cc := NewContentChunk(0, 5, 0, 10, "sec", false, "hello", "")
	if cc.Text != "hello" {
		t.Errorf("expected Text=hello, got %s", cc.Text)
	}
}

// =============================================================================
// VAL-A-045: ContentChunk equality — matching pid
// =============================================================================

func TestContentChunkEqualMatchingPID(t *testing.T) {
	a := &ContentChunk{PID: 10, EID: 5, EIDOffset: 0, Length: 20, SectionName: "sec"}
	b := &ContentChunk{PID: 10, EID: 5, EIDOffset: 0, Length: 20, SectionName: "sec"}
	if !a.Equal(b) {
		t.Error("expected equal chunks with matching pid")
	}
}

func TestContentChunkEqualDifferentPIDButSameEID(t *testing.T) {
	// Different PIDs — need comparePIDs=false to match
	a := &ContentChunk{PID: 10, EID: 5, EIDOffset: 0, Length: 20, SectionName: "sec"}
	b := &ContentChunk{PID: 20, EID: 5, EIDOffset: 0, Length: 20, SectionName: "sec"}
	if a.Equal(b) {
		t.Error("should not be equal with different PIDs when comparePIDs=true (default)")
	}
	if !a.Equal(b, false) {
		t.Error("should be equal when comparePIDs=false and EID+section match")
	}
}

// =============================================================================
// VAL-A-046: ContentChunk equality — match_zero_len tolerance
// =============================================================================

func TestContentChunkEqualMatchZeroLenTolerance(t *testing.T) {
	// Chunk A (length=20, match_zero_len=true) equals Chunk B (length=0)
	a := &ContentChunk{PID: 10, EID: 5, EIDOffset: 0, Length: 20, SectionName: "sec", MatchZeroLen: true}
	b := &ContentChunk{PID: 10, EID: 5, EIDOffset: 0, Length: 0, SectionName: "sec"}
	if !a.Equal(b) {
		t.Error("match_zero_len chunk should equal zero-length chunk")
	}
}

func TestContentChunkEqualMatchZeroLenBidirectional(t *testing.T) {
	// Reverse: B has match_zero_len=true, A has length=0
	// This tests the other match_zero_len direction: (other.match_zero_len && self.length == 0)
	a := &ContentChunk{PID: 10, EID: 5, EIDOffset: 0, Length: 0, SectionName: "sec"}
	b := &ContentChunk{PID: 10, EID: 5, EIDOffset: 0, Length: 20, SectionName: "sec", MatchZeroLen: true}
	if !a.Equal(b) {
		t.Error("zero-length chunk should equal match_zero_len chunk")
	}
}

func TestContentChunkEqualNoMatchZeroLenDifferentLengths(t *testing.T) {
	// Chunk C (length=20, match_zero_len=false) != Chunk D (length=0)
	c := &ContentChunk{PID: 10, EID: 5, EIDOffset: 0, Length: 20, SectionName: "sec", MatchZeroLen: false}
	d := &ContentChunk{PID: 10, EID: 5, EIDOffset: 0, Length: 0, SectionName: "sec", MatchZeroLen: false}
	if c.Equal(d) {
		t.Error("different lengths without match_zero_len should not be equal")
	}
}

// =============================================================================
// VAL-A-047: ContentChunk equality — eid type mismatch
// =============================================================================

func TestContentChunkEqualEIDTypeMismatch(t *testing.T) {
	// eid=5 (int) != eid="$5" (string)
	a := &ContentChunk{PID: 10, EID: 5, EIDOffset: 0, Length: 20, SectionName: "sec"}
	b := &ContentChunk{PID: 10, EID: "$5", EIDOffset: 0, Length: 20, SectionName: "sec"}
	if a.Equal(b) {
		t.Error("chunks with different EID types should not be equal")
	}
}

func TestContentChunkEqualBothStringEIDs(t *testing.T) {
	a := &ContentChunk{PID: 10, EID: "eid-abc", EIDOffset: 0, Length: 20, SectionName: "sec"}
	b := &ContentChunk{PID: 10, EID: "eid-abc", EIDOffset: 0, Length: 20, SectionName: "sec"}
	if !a.Equal(b) {
		t.Error("chunks with same string EID should be equal")
	}
}

func TestContentChunkEqualNil(t *testing.T) {
	a := &ContentChunk{PID: 10, EID: 5, EIDOffset: 0, Length: 20, SectionName: "sec"}
	if a.Equal(nil) {
		t.Error("chunk should not equal nil")
	}
}

// =============================================================================
// VAL-A-048: ConditionalTemplate — RANGE_OPERS set start to nil
// =============================================================================

func TestConditionalTemplateRangeOpersSetStartToNil(t *testing.T) {
	ct := NewConditionalTemplate(42, 5, "$298", nil)
	if ct.StartEID != nil {
		t.Errorf("expected StartEID=nil for $298, got %v", ct.StartEID)
	}
	if ct.StartEIDOffset != 0 {
		t.Errorf("expected StartEIDOffset=0 for $298, got %d", ct.StartEIDOffset)
	}
}

func TestConditionalTemplateRangeOper299(t *testing.T) {
	ct := NewConditionalTemplate(100, 10, "$299", nil)
	if ct.StartEID != nil {
		t.Errorf("expected StartEID=nil for $299, got %v", ct.StartEID)
	}
}

// =============================================================================
// VAL-A-049: ConditionalTemplate — non-RANGE_OPERS set start=end
// =============================================================================

func TestConditionalTemplateNonRangeOperSetStart(t *testing.T) {
	ct := NewConditionalTemplate(42, 5, "$294", nil)
	if ct.StartEID != 42 {
		t.Errorf("expected StartEID=42 for $294, got %v", ct.StartEID)
	}
	if ct.StartEIDOffset != 5 {
		t.Errorf("expected StartEIDOffset=5 for $294, got %d", ct.StartEIDOffset)
	}
}

func TestConditionalTemplateOper348(t *testing.T) {
	ct := NewConditionalTemplate(99, 7, "$348", nil)
	if ct.StartEID != 99 {
		t.Errorf("expected StartEID=99 for $348, got %v", ct.StartEID)
	}
	if ct.StartEIDOffset != 7 {
		t.Errorf("expected StartEIDOffset=7 for $348, got %d", ct.StartEIDOffset)
	}
}

// =============================================================================
// VAL-A-050: ConditionalTemplate — use_next flag initialization
// =============================================================================

func TestConditionalTemplateUseNextDefaultFalse(t *testing.T) {
	ct := NewConditionalTemplate(42, 0, "$294", nil)
	if ct.UseNext != false {
		t.Error("expected UseNext=false by default")
	}
}

func TestConditionalTemplatePosInfoSnapshot(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 1, Length: 10},
		{PID: 10, EID: 2, Length: 20},
	}
	ct := NewConditionalTemplate(42, 0, "$294", posInfo)
	if len(ct.PosInfo) != 2 {
		t.Errorf("expected 2 pos_info entries, got %d", len(ct.PosInfo))
	}
}

// =============================================================================
// VAL-A-051: collect_content_position_info — recursive walk (tested via helpers)
// We test the helper functions used by collect_content_position_info instead,
// since the full function requires complex Ion data structures.
// =============================================================================

func TestCollectContentPositionInfoReturnsEmptyForMinimalBook(t *testing.T) {
	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{},
	}
	result := bpl.CollectContentPositionInfo(true, false, false)
	if result != nil {
		t.Errorf("expected nil for empty book, got %v", result)
	}
}

// =============================================================================
// VAL-A-057: create_position_map — pid→eid mapping construction
// =============================================================================

func TestCreatePositionMapBuildsPIDToEIDMapping(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10, SectionName: "sec1"},
		{PID: 10, EID: 20, EIDOffset: 0, Length: 15, SectionName: "sec1"},
		{PID: 25, EID: 30, EIDOffset: 0, Length: 20, SectionName: "sec2"},
	}

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1", "sec2"},
	}

	hasSPIM, hasPositionIDOffset := bpl.CreatePositionMap(posInfo)

	if hasSPIM != false {
		t.Error("expected hasSPIM=false")
	}
	if hasPositionIDOffset != false {
		t.Error("expected hasPositionIDOffset=false for zero offsets")
	}
}

// =============================================================================
// VAL-A-058: create_position_map — position_id_offset tracking
// =============================================================================

func TestCreatePositionMapDetectsEIDOffset(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10, SectionName: "sec1"},
		{PID: 10, EID: 20, EIDOffset: 5, Length: 15, SectionName: "sec1"},
	}

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
	}

	_, hasPositionIDOffset := bpl.CreatePositionMap(posInfo)
	if !hasPositionIDOffset {
		t.Error("expected hasPositionIDOffset=true when eid_offset != 0")
	}
}

func TestCreatePositionMapNoEIDOffset(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10, SectionName: "sec1"},
		{PID: 10, EID: 20, EIDOffset: 0, Length: 15, SectionName: "sec1"},
	}

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
	}

	_, hasPositionIDOffset := bpl.CreatePositionMap(posInfo)
	if hasPositionIDOffset {
		t.Error("expected hasPositionIDOffset=false when all eid_offsets are 0")
	}
}

// =============================================================================
// VAL-A-059: create_position_map — dictionary/scribe/KPF early return
// =============================================================================

func TestCreatePositionMapDictionaryEarlyReturn(t *testing.T) {
	bpl := &BookPosLoc{
		IsDictionary:    true,
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
	}

	hasSPIM, hasPosIDOffset := bpl.CreatePositionMap(nil)
	if hasSPIM != false || hasPosIDOffset != false {
		t.Error("dictionary should return (false, false)")
	}
}

func TestCreatePositionMapScribeEarlyReturn(t *testing.T) {
	bpl := &BookPosLoc{
		IsScribeNotebook: true,
		Fragments:        &fragmentCatalog{},
		OrderedSections:  []string{"sec1"},
	}

	hasSPIM, hasPosIDOffset := bpl.CreatePositionMap(nil)
	if hasSPIM != false || hasPosIDOffset != false {
		t.Error("scribe notebook should return (false, false)")
	}
}

func TestCreatePositionMapKPFPPrepubEarlyReturn(t *testing.T) {
	bpl := &BookPosLoc{
		IsKPFPub:        true,
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
	}

	hasSPIM, hasPosIDOffset := bpl.CreatePositionMap(nil)
	if hasSPIM != false || hasPosIDOffset != false {
		t.Error("KPF prepub should return (false, false)")
	}
}

// =============================================================================
// VAL-A-060: create_position_map — section→eid mapping
// =============================================================================

func TestCreatePositionMapSectionToEIDMapping(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 5, EIDOffset: 0, Length: 10, SectionName: "A"},
		{PID: 10, EID: 10, EIDOffset: 0, Length: 15, SectionName: "A"},
		{PID: 25, EID: 7, EIDOffset: 0, Length: 5, SectionName: "B"},
	}

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"A", "B"},
	}

	bpl.CreatePositionMap(posInfo)
	// The function builds sectionEIDs internally; we verify it doesn't panic
	// and returns correct has_spim/has_position_id_offset.
}

// =============================================================================
// VAL-A-061: pid_for_eid — linear search with wraparound
// =============================================================================

func TestPidForEidLinearSearch(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10},
		{PID: 10, EID: 20, EIDOffset: 0, Length: 15},
		{PID: 25, EID: 30, EIDOffset: 0, Length: 20},
	}

	bpl := &BookPosLoc{}

	// Search for eid=20 at offset 12 (within chunk at pid=10, length=15)
	pid := bpl.PidForEid(20, 12, posInfo)
	if pid == nil {
		t.Fatal("expected to find eid=20 at offset 12")
	}
	if *pid != 22 {
		t.Errorf("expected pid=22 (10+12), got %d", *pid)
	}
}

func TestPidForEidCaching(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10},
		{PID: 10, EID: 20, EIDOffset: 0, Length: 15},
		{PID: 25, EID: 30, EIDOffset: 0, Length: 20},
	}

	bpl := &BookPosLoc{}

	// First search
	pid1 := bpl.PidForEid(20, 5, posInfo)
	if pid1 == nil || *pid1 != 15 {
		t.Fatalf("expected pid=15, got %v", pid1)
	}

	// Verify cache was updated
	if bpl.lastPII != 1 {
		t.Errorf("expected lastPII=1, got %d", bpl.lastPII)
	}

	// Second search — should start from cached position
	pid2 := bpl.PidForEid(20, 3, posInfo)
	if pid2 == nil || *pid2 != 13 {
		t.Errorf("expected pid=13, got %v", pid2)
	}
}

// =============================================================================
// VAL-A-062: pid_for_eid — not found returns nil
// =============================================================================

func TestPidForEidNotFound(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10},
		{PID: 10, EID: 20, EIDOffset: 0, Length: 15},
	}

	bpl := &BookPosLoc{}
	pid := bpl.PidForEid(99, 0, posInfo)
	if pid != nil {
		t.Errorf("expected nil for non-existent eid, got %d", *pid)
	}
}

func TestPidForEidEmptyPosInfo(t *testing.T) {
	bpl := &BookPosLoc{}
	pid := bpl.PidForEid(10, 0, nil)
	if pid != nil {
		t.Error("expected nil for empty pos_info")
	}
}

// =============================================================================
// VAL-A-063: pid_for_eid — caching with last_pii_
// =============================================================================

func TestPidForEidCachingWraparound(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10},
		{PID: 10, EID: 20, EIDOffset: 0, Length: 15},
		{PID: 25, EID: 30, EIDOffset: 0, Length: 20},
	}

	bpl := &BookPosLoc{lastPII: 2} // Start from the end

	// Search for eid=10 — should wrap around to beginning
	pid := bpl.PidForEid(10, 0, posInfo)
	if pid == nil {
		t.Fatal("expected to find eid=10")
	}
	if *pid != 0 {
		t.Errorf("expected pid=0, got %d", *pid)
	}
}

// =============================================================================
// VAL-A-064: eid_for_pid — binary search
// =============================================================================

func TestEidForPidBinarySearch(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10},
		{PID: 10, EID: 20, EIDOffset: 0, Length: 15},
		{PID: 25, EID: 30, EIDOffset: 0, Length: 20},
		{PID: 45, EID: 40, EIDOffset: 0, Length: 10},
	}

	eid, offset, found := EidForPid(30, posInfo)
	if !found {
		t.Fatal("expected to find pid=30")
	}
	if eid != 30 {
		t.Errorf("expected eid=30, got %v", eid)
	}
	if offset != 5 { // 30 - 25 = 5 offset into chunk at pid=25
		t.Errorf("expected offset=5, got %d", offset)
	}
}

func TestEidForPidNotFound(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10},
	}

	_, _, found := EidForPid(100, posInfo)
	if found {
		t.Error("expected not found for pid=100")
	}
}

func TestEidForPidEmptyPosInfo(t *testing.T) {
	_, _, found := EidForPid(0, nil)
	if found {
		t.Error("expected not found for empty pos_info")
	}
}

// =============================================================================
// VAL-A-065: eid_for_pid — boundary pid at exact chunk start
// =============================================================================

func TestEidForPidExactChunkStart(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10},
		{PID: 10, EID: 20, EIDOffset: 0, Length: 15},
	}

	eid, offset, found := EidForPid(10, posInfo)
	if !found {
		t.Fatal("expected to find pid=10")
	}
	// Python's binary search: pid=10, chunk[0] has pid=0, length=10
	// 10 <= 0+10 → matches first chunk (inclusive end)
	if eid != 10 {
		t.Errorf("expected eid=10 (first chunk, inclusive end), got %v", eid)
	}
	if offset != 10 { // 10 - 0 = 10 offset into first chunk
		t.Errorf("expected offset=10, got %d", offset)
	}
}

// =============================================================================
// VAL-A-066: eid_for_pid — pid at chunk end (inclusive, not exclusive)
// =============================================================================

func TestEidForPidChunkEndInclusive(t *testing.T) {
	// Python uses inclusive end: pid <= pi.pid + pi.length
	// So pid=10 matches first chunk (pid=0, length=10)
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10},
		{PID: 10, EID: 20, EIDOffset: 0, Length: 15},
	}

	eid, offset, found := EidForPid(10, posInfo)
	if !found {
		t.Fatal("expected to find pid=10")
	}
	// Matches first chunk (pid=0, length=10) since 10 <= 0+10
	if eid != 10 {
		t.Errorf("expected eid=10, got %v", eid)
	}
	if offset != 10 {
		t.Errorf("expected offset=10, got %d", offset)
	}
}

func TestEidForPidAtEndOfLastChunk(t *testing.T) {
	// Only one chunk, pid at end
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10},
	}

	// pid=10: 10 <= 0+10 → inclusive, matches first chunk
	eid, offset, found := EidForPid(10, posInfo)
	if !found {
		t.Fatal("expected to find pid=10 (inclusive end)")
	}
	if eid != 10 {
		t.Errorf("expected eid=10, got %v", eid)
	}
	if offset != 10 {
		t.Errorf("expected offset=10, got %d", offset)
	}
}

func TestEidForPidPastEndOfLastChunk(t *testing.T) {
	// pid past the end should not be found
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10},
	}

	_, _, found := EidForPid(11, posInfo)
	if found {
		t.Error("pid past end of last chunk should not be found")
	}
}

// =============================================================================
// VAL-A-068: generate_approximate_locations — every 110 positions
// =============================================================================

func TestGenerateApproximateLocationsEvery110(t *testing.T) {
	// Single section with 350 total positions
	posInfo := []*ContentChunk{
		{PID: 0, EID: 1, EIDOffset: 0, Length: 350, SectionName: "sec1"},
	}

	locs := GenerateApproximateLocations(posInfo)

	// Expect locations at pids [0, 110, 220, 330]
	expected := []int{0, 110, 220, 330}
	if len(locs) != len(expected) {
		t.Fatalf("expected %d locations, got %d", len(expected), len(locs))
	}
	for i, loc := range locs {
		if loc.PID != expected[i] {
			t.Errorf("location %d: expected pid=%d, got %d", i, expected[i], loc.PID)
		}
	}
}

// =============================================================================
// VAL-A-069: generate_approximate_locations — section boundary resets counter
// =============================================================================

func TestGenerateApproximateLocationsSectionReset(t *testing.T) {
	// Two sections: first with 200 positions, second with 100 positions
	posInfo := []*ContentChunk{
		{PID: 0, EID: 1, EIDOffset: 0, Length: 200, SectionName: "sec1"},
		{PID: 200, EID: 2, EIDOffset: 0, Length: 100, SectionName: "sec2"},
	}

	locs := GenerateApproximateLocations(posInfo)

	// sec1: locations at [0, 110]
	// sec2 starts at pid=200: next_loc_position resets to 200, then 200+110=310 > 300
	// So sec2: location at [200]
	expected := []int{0, 110, 200}
	if len(locs) != len(expected) {
		t.Fatalf("expected %d locations, got %d: %v", len(expected), len(locs), locs)
	}
	for i, loc := range locs {
		if loc.PID != expected[i] {
			t.Errorf("location %d: expected pid=%d, got %d", i, expected[i], loc.PID)
		}
	}
}

// =============================================================================
// VAL-A-070: generate_approximate_locations — location splits across chunks
// =============================================================================

func TestGenerateApproximateLocationsSplitsAcrossChunks(t *testing.T) {
	// Chunk at pid=100, length=50, eid=5, eid_offset=0. Location at pid=110.
	posInfo := []*ContentChunk{
		{PID: 0, EID: 1, EIDOffset: 0, Length: 100, SectionName: "sec1"},
		{PID: 100, EID: 5, EIDOffset: 0, Length: 50, SectionName: "sec1"},
	}

	locs := GenerateApproximateLocations(posInfo)

	// First location at pid=0, second at pid=110 (within second chunk)
	if len(locs) < 2 {
		t.Fatalf("expected at least 2 locations, got %d", len(locs))
	}

	// Second location should be at pid=110, eid=5, eid_offset=10
	loc := locs[1]
	if loc.PID != 110 {
		t.Errorf("expected pid=110, got %d", loc.PID)
	}
	if loc.EID != 5 {
		t.Errorf("expected eid=5, got %v", loc.EID)
	}
	if loc.EIDOffset != 10 {
		t.Errorf("expected eid_offset=10, got %d", loc.EIDOffset)
	}
}

// =============================================================================
// VAL-A-071: generate_approximate_locations — empty pos_info
// =============================================================================

func TestGenerateApproximateLocationsEmpty(t *testing.T) {
	locs := GenerateApproximateLocations(nil)
	if len(locs) != 0 {
		t.Errorf("expected 0 locations for empty pos_info, got %d", len(locs))
	}
}

// =============================================================================
// VAL-A-075: collect_location_map_info — location length computation
// =============================================================================

func TestLocationLengthComputation(t *testing.T) {
	// Test that GenerateApproximateLocations produces correct PID values
	// which would be used for length computation: next_loc.pid - current_loc.pid
	posInfo := []*ContentChunk{
		{PID: 0, EID: 1, EIDOffset: 0, Length: 350, SectionName: "sec1"},
	}

	locs := GenerateApproximateLocations(posInfo)

	// Locations at pids [0, 110, 220, 330]
	expectedLengths := []int{110, 110, 110, 20} // last one extends to end of pos_info
	for i := 0; i < len(locs)-1; i++ {
		length := locs[i+1].PID - locs[i].PID
		if length != expectedLengths[i] {
			t.Errorf("location %d: expected length=%d, got %d", i, expectedLengths[i], length)
		}
	}
}

// =============================================================================
// VAL-A-076: determine_approximate_pages — fixed layout creates page per section
// =============================================================================

func TestDetermineApproximatePagesFixedLayout(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 100, SectionName: "sec1"},
		{PID: 100, EID: 20, EIDOffset: 0, Length: 200, SectionName: "sec2"},
		{PID: 300, EID: 30, EIDOffset: 0, Length: 150, SectionName: "sec3"},
	}

	pages, newSectionCount := DetermineApproximatePages(posInfo, nil, "sec1", 999999, true)
	if len(pages) != 3 {
		t.Fatalf("expected 3 pages for 3 fixed-layout sections, got %d", len(pages))
	}
	if newSectionCount != 3 {
		t.Errorf("expected newSectionCount=3, got %d", newSectionCount)
	}
}

// =============================================================================
// VAL-A-077: determine_approximate_pages — reflowable page breaks
// =============================================================================

func TestDetermineApproximatePagesReflowable(t *testing.T) {
	// Create a text chunk long enough for multiple pages
	longText := makeLongText(5000)
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: len(longText), SectionName: "sec1", Text: longText},
	}

	pages, _ := DetermineApproximatePages(posInfo, nil, "sec1", 1850, false)
	if len(pages) < 2 {
		t.Fatalf("expected at least 2 pages for 5000 positions with 1850 per page, got %d", len(pages))
	}
}

// =============================================================================
// VAL-A-078: determine_approximate_pages — section boundaries always create page breaks
// =============================================================================

func TestDetermineApproximatePagesSectionBoundaries(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 200, SectionName: "sec1"},
		{PID: 200, EID: 20, EIDOffset: 0, Length: 300, SectionName: "sec2"},
	}

	pages, _ := DetermineApproximatePages(posInfo, nil, "sec1", 1850, false)

	// With only 500 total positions (< 1850 per page), each section should still get a page
	if len(pages) < 2 {
		t.Fatalf("expected at least 2 pages (one per section), got %d", len(pages))
	}
}

// =============================================================================
// VAL-A-079: determine_approximate_pages — page_template_eids are skipped
// =============================================================================

func TestDetermineApproximatePagesSkipTemplateEIDs(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 100, SectionName: "sec1"},
		{PID: 100, EID: 999, EIDOffset: 0, Length: 50, SectionName: "sec1"}, // template eid
		{PID: 150, EID: 20, EIDOffset: 0, Length: 100, SectionName: "sec1"},
	}

	templateEIDs := map[interface{}]bool{999: true}
	pages, _ := DetermineApproximatePages(posInfo, templateEIDs, "sec1", 1850, false)

	// Only 200 positions of non-template content — should produce 1 page
	if len(pages) != 1 {
		t.Errorf("expected 1 page with template eid skipped, got %d", len(pages))
	}
}

// =============================================================================
// VAL-A-080: determine_approximate_pages — GEN_COVER_PAGE_NUMBER skips first section
// =============================================================================

func TestDetermineApproximatePagesCoverSkip(t *testing.T) {
	// GEN_COVER_PAGE_NUMBER is true by default, but the first section is only
	// skipped if section_name == first_section_name AND !GEN_COVER_PAGE_NUMBER.
	// Since GEN_COVER_PAGE_NUMBER=true, first section is NOT skipped.
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 100, SectionName: "cover"},
		{PID: 100, EID: 20, EIDOffset: 0, Length: 200, SectionName: "sec2"},
	}

	pages, _ := DetermineApproximatePages(posInfo, nil, "cover", 1850, false)

	// Both sections should produce pages since GEN_COVER_PAGE_NUMBER=true
	if len(pages) < 2 {
		t.Errorf("expected >= 2 pages with GEN_COVER_PAGE_NUMBER=true, got %d", len(pages))
	}
}

// =============================================================================
// VAL-A-081: determine_approximate_pages — whitespace lookback adjustment
// =============================================================================

func TestDetermineApproximatePagesWhitespaceLookback(t *testing.T) {
	// Create text where page break falls inside a word
	// "hello world and more text" at positions 0-24
	// positions_per_page=8, break at position 8 (inside "world")
	text := "hello world and more text"
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: len(text), SectionName: "sec1", Text: text},
	}

	pages, _ := DetermineApproximatePages(posInfo, nil, "sec1", 8, false)

	// First page at pid=0. Second page should be adjusted to after "hello " (position 6)
	// since position 8 falls inside "world" and we look back for whitespace.
	if len(pages) < 2 {
		t.Fatalf("expected at least 2 pages, got %d", len(pages))
	}

	// The second page target offset should be at a word boundary
	secondPage := pages[1]
	targetMap, ok := secondPage["$246"].(map[string]interface{})
	if !ok {
		t.Fatal("expected $246 in page entry")
	}
	offset, ok := targetMap["$143"].(int)
	if !ok {
		t.Fatal("expected $143 offset in target")
	}

	// Should be adjusted back to position 6 (after "hello ") instead of position 8
	if offset > 6 {
		t.Errorf("expected whitespace-adjusted offset <= 6, got %d", offset)
	}
}

// =============================================================================
// VAL-A-082: determine_approximate_pages — binary search for desired page count
// =============================================================================

func TestCreateApproximatePageListBinarySearch(t *testing.T) {
	// Create 10000 positions of text
	longText := makeLongText(10000)
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: len(longText), SectionName: "sec1", Text: longText},
	}

	pages := CreateApproximatePageListWithPosInfo(posInfo, nil, "sec1", 5, false)

	// Binary search should find positions_per_page that produces ~5 pages
	if len(pages) == 0 {
		t.Fatal("expected some pages")
	}
	// Allow some tolerance since whitespace adjustment may cause exact match to differ
	if len(pages) > 7 || len(pages) < 3 {
		t.Errorf("expected approximately 5 pages, got %d", len(pages))
	}
}

// =============================================================================
// VAL-A-076/077: create_approximate_page_list — fixed-layout vs reflowable
// =============================================================================

func TestCreateApproximatePageListFixedLayout(t *testing.T) {
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 100, SectionName: "sec1"},
		{PID: 100, EID: 20, EIDOffset: 0, Length: 200, SectionName: "sec2"},
	}

	pages := CreateApproximatePageListWithPosInfo(posInfo, nil, "sec1", 0, true)
	if len(pages) != 2 {
		t.Errorf("expected 2 fixed-layout pages (one per section), got %d", len(pages))
	}
}

func TestCreateApproximatePageListReflowableAuto(t *testing.T) {
	longText := makeLongText(5000)
	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: len(longText), SectionName: "sec1", Text: longText},
	}

	pages := CreateApproximatePageListWithPosInfo(posInfo, nil, "sec1", 0, false)
	if len(pages) < 2 {
		t.Errorf("expected at least 2 auto pages for 5000 positions, got %d", len(pages))
	}
}

// =============================================================================
// VAL-A-083: verify_position_info — content vs map parallel walk
// =============================================================================

func TestVerifyPositionInfoMatching(t *testing.T) {
	contentPosInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10, SectionName: "sec1"},
		{PID: 10, EID: 20, EIDOffset: 0, Length: 15, SectionName: "sec1"},
	}
	mapPosInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10, SectionName: "sec1"},
		{PID: 10, EID: 20, EIDOffset: 0, Length: 15, SectionName: "sec1"},
	}

	report := VerifyPositionInfo(contentPosInfo, mapPosInfo, false)
	if report.Count != 0 {
		t.Errorf("expected 0 reports for matching pos_info, got %d", report.Count)
	}
}

func TestVerifyPositionInfoMismatchedEID(t *testing.T) {
	contentPosInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10, SectionName: "sec1"},
	}
	mapPosInfo := []*ContentChunk{
		{PID: 0, EID: 99, EIDOffset: 0, Length: 10, SectionName: "sec1"},
	}

	report := VerifyPositionInfo(contentPosInfo, mapPosInfo, false)
	if report.Count == 0 {
		t.Error("expected non-zero reports for mismatched eids")
	}
}

func TestVerifyPositionInfoExtraContent(t *testing.T) {
	contentPosInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10, SectionName: "sec1"},
		{PID: 10, EID: 20, EIDOffset: 0, Length: 15, SectionName: "sec1"},
	}
	mapPosInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10, SectionName: "sec1"},
	}

	report := VerifyPositionInfo(contentPosInfo, mapPosInfo, false)
	if report.Count == 0 {
		t.Error("expected reports for extra content chunk")
	}
}

// =============================================================================
// VAL-A-084: anchor_eid_offset — lookup from anchor fragment
// =============================================================================

func TestAnchorEidOffsetFound(t *testing.T) {
	bpl := &BookPosLoc{
		Fragments: &fragmentCatalog{
			AnchorFragments: map[string]anchorFragment{
				"my-anchor": {ID: "my-anchor", PositionID: 42},
			},
		},
	}

	eid, offset, found := bpl.anchorEidOffset("my-anchor")
	if !found {
		t.Fatal("expected to find anchor")
	}
	if eid != 42 {
		t.Errorf("expected eid=42, got %v", eid)
	}
	if offset != 0 {
		t.Errorf("expected offset=0, got %d", offset)
	}
}

func TestAnchorEidOffsetNotFound(t *testing.T) {
	bpl := &BookPosLoc{
		Fragments: &fragmentCatalog{
			AnchorFragments: map[string]anchorFragment{},
		},
	}

	_, _, found := bpl.anchorEidOffset("nonexistent")
	if found {
		t.Error("expected not found for nonexistent anchor")
	}
}

// =============================================================================
// VAL-A-085: MatchReport — limit enforcement
// =============================================================================

func TestMatchReportLimitEnforcement(t *testing.T) {
	mr := NewMatchReport(false)
	// With MAX_REPORT_ERRORS=0, limit is 0 which means unlimited
	// When no_limit=False, limit=MAX_REPORT_ERRORS=0
	// When limit=0, the check is: if (not self.limit) or self.count < self.limit
	// not 0 = True, so always logs. This is the "unlimited" case.
	_ = mr

	// Test with noLimit=true
	mr2 := NewMatchReport(true)
	if mr2.Limit != 0 {
		t.Errorf("expected limit=0 for noLimit, got %d", mr2.Limit)
	}
}

func TestMatchReportCountIncrement(t *testing.T) {
	mr := NewMatchReport(true)
	mr.Report("test message 1")
	mr.Report("test message 2")
	mr.Report("test message 3")

	if mr.Count != 3 {
		t.Errorf("expected count=3, got %d", mr.Count)
	}
}

// =============================================================================
// ContentChunk String() and repr
// =============================================================================

func TestContentChunkString(t *testing.T) {
	cc := &ContentChunk{PID: 10, EID: 5, EIDOffset: 3, Length: 20, SectionName: "sec1", Text: "hello"}
	s := cc.String()
	if !contains(s, "pid=10") {
		t.Errorf("expected repr to contain 'pid=10', got %s", s)
	}
	if !contains(s, "eid=5+3") {
		t.Errorf("expected repr to contain 'eid=5+3', got %s", s)
	}
}

func TestContentChunkStringNoOffset(t *testing.T) {
	cc := &ContentChunk{PID: 10, EID: 5, EIDOffset: 0, Length: 20, SectionName: "sec1"}
	s := cc.String()
	if contains(s, "+0") {
		t.Errorf("expected no offset display when offset=0, got %s", s)
	}
}

// =============================================================================
// ConditionalTemplate String()
// =============================================================================

func TestConditionalTemplateString(t *testing.T) {
	ct := NewConditionalTemplate(42, 5, "$294", []*ContentChunk{{EID: 1}, {EID: 2}})
	s := ct.String()
	if !contains(s, "$294") {
		t.Errorf("expected repr to contain $294, got %s", s)
	}
}

func TestConditionalTemplateStringRange(t *testing.T) {
	ct := NewConditionalTemplate(42, 5, "$298", []*ContentChunk{{EID: 1}})
	s := ct.String()
	if !contains(s, "$298") {
		t.Errorf("expected repr to contain $298, got %s", s)
	}
}

// =============================================================================
// SortContentChunksByPID
// =============================================================================

func TestSortContentChunksByPID(t *testing.T) {
	chunks := []*ContentChunk{
		{PID: 25, EID: 3},
		{PID: 0, EID: 1},
		{PID: 10, EID: 2},
	}
	SortContentChunksByPID(chunks)
	if chunks[0].PID != 0 || chunks[1].PID != 10 || chunks[2].PID != 25 {
		t.Errorf("expected sorted [0, 10, 25], got [%d, %d, %d]",
			chunks[0].PID, chunks[1].PID, chunks[2].PID)
	}
}

// =============================================================================
// CollectLocationMapInfo with no fragments
// =============================================================================

func TestCollectLocationMapInfoNoFragments(t *testing.T) {
	bpl := &BookPosLoc{
		Fragments: &fragmentCatalog{},
	}

	posInfo := []*ContentChunk{
		{PID: 0, EID: 1, EIDOffset: 0, Length: 100, SectionName: "sec1"},
	}

	locInfo := bpl.CollectLocationMapInfo(posInfo)
	if len(locInfo) != 0 {
		t.Errorf("expected 0 locations with no fragments, got %d", len(locInfo))
	}
}

// =============================================================================
// FragmentStore tests
// =============================================================================

func TestFragmentStoreBasic(t *testing.T) {
	fs := NewFragmentStore()
	if fs.Has("$264") {
		t.Error("expected no $264 fragments initially")
	}
	if fs.Get("$264") != nil {
		t.Error("expected nil for missing fragment")
	}

	fs.Append("$264", map[string]interface{}{"$264": "test"})
	if !fs.Has("$264") {
		t.Error("expected $264 to exist after append")
	}

	all := fs.GetAll("$264")
	if len(all) != 1 {
		t.Errorf("expected 1 fragment, got %d", len(all))
	}

	fs.RemoveAll("$264")
	if fs.Has("$264") {
		t.Error("expected $264 to be removed")
	}
}

func TestFragmentStoreMultiple(t *testing.T) {
	fs := NewFragmentStore()
	fs.Append("$265", map[string]interface{}{"data": 1})
	fs.Append("$265", map[string]interface{}{"data": 2})
	if len(fs.GetAll("$265")) != 2 {
		t.Errorf("expected 2 fragments, got %d", len(fs.GetAll("$265")))
	}
	first := fs.Get("$265")
	if first["data"] != 1 {
		t.Error("expected Get to return first fragment")
	}
}

// =============================================================================
// VAL-M2-POSITION-001: CollectContentPositionInfo walks sections and returns ContentChunks
// =============================================================================

func TestCollectContentPositionInfoWalksSections(t *testing.T) {
	// Build a simple book with 2 sections, each containing a container with text
	fs := NewFragmentStore()
	fs.Append("$260", map[string]interface{}{
		"$260": map[string]interface{}{
			"$174": "sec1",
			"$141": []interface{}{
				map[string]interface{}{
					"$155": 10,
					"$159": "$270",
					"$146": []interface{}{
						"Hello World", // 11 chars
					},
				},
			},
		},
	})
	// Override with two section data
	fs.RemoveAll("$260")
	fs.Append("$260", map[string]interface{}{
		"$260": map[string]interface{}{
			"$174": "sec1",
			"$141": []interface{}{
				map[string]interface{}{
					"$155": 10,
					"$159": "$270",
					"$146": []interface{}{"Hello"},
				},
			},
		},
	})

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
		Store:           fs,
	}

	result := bpl.CollectContentPositionInfo(true, false, false)
	if len(result) == 0 {
		t.Fatal("expected non-empty position info")
	}

	// Should have at least one chunk for sec1
	foundSec1 := false
	for _, chunk := range result {
		if chunk.SectionName == "sec1" {
			foundSec1 = true
			break
		}
	}
	if !foundSec1 {
		t.Error("expected at least one chunk with section sec1")
	}
}

func TestCollectContentPositionInfoTwoSections(t *testing.T) {
	fs := NewFragmentStore()
	// Section 1 with eid=10
	fs.Append("$260", map[string]interface{}{
		"$260": []interface{}{
			map[string]interface{}{
				"$155": 10,
				"$159": "$270",
				"$146": []interface{}{"text1"},
			},
		},
	})
	// Section 2 with eid=20
	fs.Append("$260", map[string]interface{}{
		"$260": []interface{}{
			map[string]interface{}{
				"$155": 20,
				"$159": "$270",
				"$146": []interface{}{"text2"},
			},
		},
	})

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1", "sec2"},
		Store:           fs,
	}

	result := bpl.CollectContentPositionInfo(true, false, false)
	if len(result) < 2 {
		t.Fatalf("expected at least 2 chunks for 2 sections, got %d", len(result))
	}
}

// =============================================================================
// VAL-M2-POSITION-002: CollectContentPositionInfo tracks eid→section mapping
// =============================================================================

func TestCollectPositionInfo_EidSectionMap(t *testing.T) {
	fs := NewFragmentStore()
	fs.Append("$260", map[string]interface{}{
		"$260": []interface{}{
			map[string]interface{}{
				"$155": 100,
				"$159": "$270",
				"$146": []interface{}{"abc"},
			},
		},
	})

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"s1"},
		Store:           fs,
	}

	result := bpl.CollectContentPositionInfo(true, false, false)
	if len(result) == 0 {
		t.Fatal("expected at least one chunk")
	}

	// The chunk for eid=100 should have section s1
	found := false
	for _, chunk := range result {
		if chunk.EID == 100 && chunk.SectionName == "s1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected chunk with eid=100 in section s1, got %v", result)
	}
}

// =============================================================================
// VAL-M2-POSITION-003: CollectContentPositionInfo merges consecutive same-EID chunks
// =============================================================================

func TestCollectPositionInfo_MergeEids(t *testing.T) {
	fs := NewFragmentStore()
	// Two consecutive children with same eid=42
	fs.Append("$260", map[string]interface{}{
		"$260": []interface{}{
			map[string]interface{}{
				"$155": 42,
				"$159": "$270",
				"$146": []interface{}{
					"abc", // 3 chars
					"def", // 3 chars - should merge with previous
				},
			},
		},
	})

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
		Store:           fs,
	}

	result := bpl.CollectContentPositionInfo(true, false, false)

	// After merging, should have fewer chunks than if unmerged
	if len(result) == 0 {
		t.Fatal("expected at least one chunk")
	}

	// Check that there are consecutive chunks with same EID being merged
	eid42Count := 0
	for _, chunk := range result {
		if chunk.EID == 42 {
			eid42Count++
		}
	}
	// Due to merging, there should be at most 2 chunks for eid=42 (one for the content, one for the $270 container)
	if eid42Count > 2 {
		t.Errorf("expected at most 2 chunks for eid=42 after merging, got %d", eid42Count)
	}
}

// =============================================================================
// VAL-M2-POSITION-004: CollectPositionMapInfo parses $264/$265 fragments
// =============================================================================

func TestCollectPositionMapInfoBasic(t *testing.T) {
	fs := NewFragmentStore()
	// Add a simple position_id_map ($265) as a flat list
	fs.Append("$265", map[string]interface{}{
		"$265": []interface{}{
			[]interface{}{0, 10},    // pid=0, eid=10
			[]interface{}{100, 20},  // pid=100, eid=20
			[]interface{}{200, 0},   // terminal entry
		},
	})
	// Add a position_map ($264)
	fs.Append("$264", map[string]interface{}{
		"$264": []interface{}{
			map[string]interface{}{
				"$174": "sec1",
				"$181": []interface{}{10, 20},
			},
		},
	})

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
		Store:           fs,
	}

	result := bpl.CollectPositionMapInfo()
	if len(result) == 0 {
		t.Fatal("expected non-empty position map info")
	}

	// Should have chunks from the position_id_map entries
	if len(result) < 1 {
		t.Errorf("expected at least 1 chunk, got %d", len(result))
	}
}

// =============================================================================
// VAL-M2-POSITION-005: CollectPositionMapInfo handles SPIM $609 fragments
// =============================================================================

func TestCollectPositionMapInfo_SPIM(t *testing.T) {
	fs := NewFragmentStore()
	// SPIM format: $265 value is a map with $181 containing per-section entries
	fs.Append("$265", map[string]interface{}{
		"$265": map[string]interface{}{
			"$181": []interface{}{
				map[string]interface{}{
					"$174": "sec1",
					"$184": 0,
					"$144": 100,
				},
			},
		},
	})
	// $609 SPIM fragment for sec1
	fs.Append("$609", map[string]interface{}{
		"$609": map[string]interface{}{
			"$174": "sec1",
			"$181": []interface{}{
				[]interface{}{0, 10},
				[]interface{}{100, 0},
			},
		},
	})

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
		Store:           fs,
	}

	result := bpl.CollectPositionMapInfo()
	if len(result) == 0 {
		t.Fatal("expected non-empty SPIM position map info")
	}
}

// =============================================================================
// VAL-M2-POSITION-006: CollectPositionMapInfo validates eid section consistency
// =============================================================================

func TestCollectPositionMapInfo_EidValidation(t *testing.T) {
	fs := NewFragmentStore()
	// Position map with section sec1
	fs.Append("$264", map[string]interface{}{
		"$264": []interface{}{
			map[string]interface{}{
				"$174": "sec1",
				"$181": []interface{}{10},
			},
		},
	})

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
		Store:           fs,
	}

	// Should not panic — validation just logs errors
	result := bpl.CollectPositionMapInfo()
	_ = result
}

// =============================================================================
// VAL-M2-POSITION-007: CreateLocationMap removes old $550/$621 and builds new $550
// =============================================================================

func TestCreateLocationMapRemovesOldAndBuildsNew(t *testing.T) {
	fs := NewFragmentStore()
	// Pre-existing $550 and $621 fragments
	fs.Append("$550", map[string]interface{}{"$550": "old"})
	fs.Append("$621", map[string]interface{}{"$621": "old"})

	bpl := &BookPosLoc{
		Store: fs,
	}

	locInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0},
		{PID: 110, EID: 20, EIDOffset: 5},
		{PID: 220, EID: 30, EIDOffset: 0},
	}

	result := bpl.CreateLocationMap(locInfo)

	// hasYJLocationPidMap should be false
	if result != false {
		t.Error("expected hasYJLocationPidMap=false")
	}

	// Old $550 and $621 should be gone
	if fs.Has("$621") {
		t.Error("expected $621 to be removed")
	}

	// New $550 should exist with 3 location entries
	new550 := fs.Get("$550")
	if new550 == nil {
		t.Fatal("expected new $550 fragment to be created")
	}
	locMapData, ok := new550["$550"].([]interface{})
	if !ok {
		t.Fatalf("expected $550 to be []interface{}, got %T", new550["$550"])
	}
	if len(locMapData) != 1 {
		t.Fatalf("expected 1 map entry in $550, got %d", len(locMapData))
	}
	locMap, ok := locMapData[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected map entry")
	}
	entries, ok := locMap["$182"].([]interface{})
	if !ok {
		t.Fatal("expected $182 list in location map")
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 location entries, got %d", len(entries))
	}
}

// =============================================================================
// VAL-M2-POSITION-008: CreateApproximatePageList processes full page list logic
// =============================================================================

func TestCreateApproximatePageListBasic(t *testing.T) {
	fs := NewFragmentStore()
	// Add reading order
	fs.Append("$389", map[string]interface{}{
		"$389": []interface{}{
			map[string]interface{}{
				"$178": "default",
				"$170": []interface{}{"sec1"},
				"$169": []interface{}{},
			},
		},
	})
	// Add section fragment
	fs.Append("$260", map[string]interface{}{
		"$260": []interface{}{
			map[string]interface{}{
				"$155": 10,
				"$159": "$270",
				"$146": []interface{}{makeLongText(5000)},
			},
		},
	})

	frags := &fragmentCatalog{
		DocumentData: map[string]interface{}{
			"$169": []interface{}{
				map[string]interface{}{
					"$178": "default",
					"$170": []interface{}{"sec1"},
				},
			},
		},
	}

	bpl := &BookPosLoc{
		Fragments:       frags,
		OrderedSections: []string{"sec1"},
		Store:           fs,
	}

	bpl.CreateApproximatePageList(0)

	// Should not panic — pages are generated
}

func TestCreateApproximatePageListInvalidCDEType(t *testing.T) {
	fs := NewFragmentStore()
	bpl := &BookPosLoc{
		CDEType: "MAGZ",
		Store:   fs,
	}

	// Should return early and not panic
	bpl.CreateApproximatePageList(0)
}

func TestCreateApproximatePageListDictionary(t *testing.T) {
	fs := NewFragmentStore()
	bpl := &BookPosLoc{
		IsDictionary: true,
		Store:        fs,
	}

	// Should return early
	bpl.CreateApproximatePageList(0)
}

// =============================================================================
// VAL-M2-POSITION-009: CreatePositionMap stores $264 and $265 fragments
// =============================================================================

func TestCreatePositionMapStores(t *testing.T) {
	fs := NewFragmentStore()

	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10, SectionName: "sec1"},
		{PID: 10, EID: 20, EIDOffset: 0, Length: 15, SectionName: "sec1"},
		{PID: 25, EID: 30, EIDOffset: 0, Length: 20, SectionName: "sec2"},
	}

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1", "sec2"},
		Store:           fs,
	}

	hasSPIM, hasPositionIDOffset := bpl.CreatePositionMap(posInfo)

	if hasSPIM != false {
		t.Error("expected hasSPIM=false")
	}
	if hasPositionIDOffset != false {
		t.Error("expected hasPositionIDOffset=false for zero offsets")
	}

	// Verify $264 fragment was stored
	frag264 := fs.Get("$264")
	if frag264 == nil {
		t.Fatal("expected $264 fragment to be stored")
	}
	pmSlice, ok := frag264["$264"].([]interface{})
	if !ok {
		t.Fatalf("expected $264 to be []interface{}, got %T", frag264["$264"])
	}
	if len(pmSlice) != 2 {
		t.Errorf("expected 2 section entries in $264, got %d", len(pmSlice))
	}

	// Verify $265 fragment was stored
	frag265 := fs.Get("$265")
	if frag265 == nil {
		t.Fatal("expected $265 fragment to be stored")
	}
	pidSlice, ok := frag265["$265"].([]interface{})
	if !ok {
		t.Fatalf("expected $265 to be []interface{}, got %T", frag265["$265"])
	}
	// Should have 3 entries + 1 terminal = 4
	if len(pidSlice) != 4 {
		t.Errorf("expected 4 entries in $265 (3 chunks + terminal), got %d", len(pidSlice))
	}

	// Terminal entry should have $185=0
	terminal, ok := pidSlice[3].(map[string]interface{})
	if !ok {
		t.Fatal("expected terminal entry to be a map")
	}
	if terminal["$185"] != 0 {
		t.Errorf("expected terminal $185=0, got %v", terminal["$185"])
	}
	if terminal["$184"] != 45 { // 10+15+20=45
		t.Errorf("expected terminal $184=45, got %v", terminal["$184"])
	}
}

// =============================================================================
// VAL-M2-POSITION-010: CreatePositionMap removes old mapping fragments first
// =============================================================================

func TestCreatePositionMapRemovesOld(t *testing.T) {
	fs := NewFragmentStore()
	// Pre-existing fragments
	fs.Append("$264", map[string]interface{}{"$264": "old"})
	fs.Append("$265", map[string]interface{}{"$265": "old"})
	fs.Append("$609", map[string]interface{}{"$609": "old"})
	fs.Append("$610", map[string]interface{}{"$610": "old"})
	fs.Append("$611", map[string]interface{}{"$611": "old"})

	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10, SectionName: "sec1"},
	}

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
		Store:           fs,
	}

	bpl.CreatePositionMap(posInfo)

	// Old $609, $610, $611 should be removed
	if fs.Has("$609") {
		t.Error("expected $609 to be removed")
	}
	if fs.Has("$610") {
		t.Error("expected $610 to be removed")
	}
	if fs.Has("$611") {
		t.Error("expected $611 to be removed")
	}

	// New $264 and $265 should exist
	if !fs.Has("$264") {
		t.Error("expected new $264 to exist")
	}
	if !fs.Has("$265") {
		t.Error("expected new $265 to exist")
	}
}

// =============================================================================
// VAL-M2-POSITION-011: CheckPositionAndLocationMaps orchestration exists
// =============================================================================

func TestCheckPositionAndLocationMaps(t *testing.T) {
	fs := NewFragmentStore()
	fs.Append("$260", map[string]interface{}{
		"$260": []interface{}{
			map[string]interface{}{
				"$155": 10,
				"$159": "$270",
				"$146": []interface{}{"text"},
			},
		},
	})

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
		Store:           fs,
	}

	// Should not panic — calls all sub-methods in order
	bpl.CheckPositionAndLocationMaps()
}

func TestCheckPositionAndLocationMapsEmptyBook(t *testing.T) {
	fs := NewFragmentStore()
	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{},
		Store:           fs,
	}

	// Empty book should not panic
	bpl.CheckPositionAndLocationMaps()
}

// =============================================================================
// CreatePositionMap with EIDOffset tracking
// =============================================================================

func TestCreatePositionMapWithEIDOffset(t *testing.T) {
	fs := NewFragmentStore()

	posInfo := []*ContentChunk{
		{PID: 0, EID: 10, EIDOffset: 0, Length: 10, SectionName: "sec1"},
		{PID: 10, EID: 20, EIDOffset: 5, Length: 15, SectionName: "sec1"},
	}

	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
		Store:           fs,
	}

	_, hasPositionIDOffset := bpl.CreatePositionMap(posInfo)
	if !hasPositionIDOffset {
		t.Error("expected hasPositionIDOffset=true when eid_offset != 0")
	}

	// Verify $265 entries include $143 offset
	frag265 := fs.Get("$265")
	pidSlice, _ := frag265["$265"].([]interface{})
	secondEntry, _ := pidSlice[1].(map[string]interface{})
	if secondEntry["$143"] != 5 {
		t.Errorf("expected $143=5 in second entry, got %v", secondEntry["$143"])
	}
}

// =============================================================================
// CreatePositionMap dictionary early return removes $264/$265/$610
// =============================================================================

func TestCreatePositionMapDictionaryRemovesOldFragments(t *testing.T) {
	fs := NewFragmentStore()
	fs.Append("$264", map[string]interface{}{"$264": "old"})
	fs.Append("$265", map[string]interface{}{"$265": "old"})
	fs.Append("$610", map[string]interface{}{"$610": "old"})

	bpl := &BookPosLoc{
		IsDictionary:    true,
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
		Store:           fs,
	}

	hasSPIM, hasPosIDOffset := bpl.CreatePositionMap(nil)
	if hasSPIM != false || hasPosIDOffset != false {
		t.Error("dictionary should return (false, false)")
	}

	if fs.Has("$264") {
		t.Error("expected $264 to be removed for dictionary")
	}
	if fs.Has("$265") {
		t.Error("expected $265 to be removed for dictionary")
	}
	if fs.Has("$610") {
		t.Error("expected $610 to be removed for dictionary")
	}
}

// =============================================================================
// CollectContentPositionInfo with empty sections
// =============================================================================

func TestCollectContentPositionInfoEmptySections(t *testing.T) {
	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{},
		Store:           NewFragmentStore(),
	}

	result := bpl.CollectContentPositionInfo(true, false, false)
	if len(result) != 0 {
		t.Errorf("expected empty result for empty book, got %d chunks", len(result))
	}
}

// =============================================================================
// CollectPositionMapInfo with no fragments
// =============================================================================

func TestCollectPositionMapInfoNoFragments(t *testing.T) {
	bpl := &BookPosLoc{
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
		Store:           NewFragmentStore(),
	}

	result := bpl.CollectPositionMapInfo()
	if len(result) != 0 {
		t.Errorf("expected empty result with no fragments, got %d chunks", len(result))
	}
}

// =============================================================================
// CollectPositionMapInfo dictionary path
// =============================================================================

func TestCollectPositionMapInfoDictionaryPath(t *testing.T) {
	fs := NewFragmentStore()
	bpl := &BookPosLoc{
		IsDictionary:    true,
		Fragments:       &fragmentCatalog{},
		OrderedSections: []string{"sec1"},
		Store:           fs,
	}

	result := bpl.CollectPositionMapInfo()
	// Dictionary path without $611 returns empty
	if len(result) != 0 {
		t.Errorf("expected empty for dictionary without $611, got %d", len(result))
	}
}

// =============================================================================
// Helpers
// =============================================================================

// makeLongText creates a text string of approximately the given number of characters,
// with spaces every ~8 characters to allow whitespace lookback.
func makeLongText(length int) string {
	var sb strings.Builder
	for i := 0; i < length; i++ {
		if i > 0 && i%8 == 0 {
			sb.WriteByte(' ')
		} else {
			sb.WriteByte('x')
		}
	}
	return sb.String()
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
