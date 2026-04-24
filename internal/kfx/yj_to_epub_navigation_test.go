package kfx

import (
	"bytes"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"testing"
)

// captureStderr redirects os.Stderr to a buffer, runs f, then restores stderr.
// Returns the captured output.
func captureStderr(f func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	// Also redirect log output since our functions use log.Printf
	logOld := log.Writer()
	logFlags := log.Flags()
	log.SetOutput(w)
	log.SetFlags(0)
	f()
	w.Close()
	os.Stderr = old
	log.SetOutput(logOld)
	log.SetFlags(logFlags)
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// ============================================================================
// VAL-B-019: reportMissingPositions
// ============================================================================

func TestReportMissingPositionsEmpty(t *testing.T) {
	// Empty positionAnchors → no output at all.
	got := captureStderr(func() {
		reportMissingPositions(map[int]map[int][]string{})
	})
	if got != "" {
		t.Errorf("expected no output for empty positionAnchors, got %q", got)
	}
}

func TestReportMissingPositionsNil(t *testing.T) {
	// Nil positionAnchors → no output at all.
	got := captureStderr(func() {
		reportMissingPositions(nil)
	})
	if got != "" {
		t.Errorf("expected no output for nil positionAnchors, got %q", got)
	}
}

func TestReportMissingPositionsFormatsAndSorts(t *testing.T) {
	// VAL-B-019: Must format as "EID.OFFSET", sort them (as strings), and log error with count and list.
	positionAnchors := map[int]map[int][]string{
		42: {
			0: {"anchor_a"},
			5: {"anchor_b"},
		},
		7: {
			3: {"anchor_c"},
		},
	}
	got := captureStderr(func() {
		reportMissingPositions(positionAnchors)
	})
	// Must contain count=3
	if !strings.Contains(got, "3") {
		t.Errorf("expected count 3 in output, got %q", got)
	}
	// Must contain all position strings
	if !strings.Contains(got, "7.3") {
		t.Errorf("expected position 7.3 in output, got %q", got)
	}
	if !strings.Contains(got, "42.0") {
		t.Errorf("expected position 42.0 in output, got %q", got)
	}
	if !strings.Contains(got, "42.5") {
		t.Errorf("expected position 42.5 in output, got %q", got)
	}
	// String sort order: "42.0" < "42.5" < "7.3" (because "4" < "7")
	idx420 := strings.Index(got, "42.0")
	idx425 := strings.Index(got, "42.5")
	idx73 := strings.Index(got, "7.3")
	if idx420 >= idx425 || idx425 >= idx73 {
		t.Errorf("positions must be string-sorted: 42.0 (idx=%d) < 42.5 (idx=%d) < 7.3 (idx=%d), got %q", idx420, idx425, idx73, got)
	}
	// Must say "error" or "Fail"
	if !strings.Contains(strings.ToLower(got), "fail") && !strings.Contains(strings.ToLower(got), "error") {
		t.Errorf("expected error/fail message in output, got %q", got)
	}
}

func TestReportMissingPositionsTruncatesLongList(t *testing.T) {
	// truncate_list in Python caps at 10 items, then adds "... (N total)".
	// Create 15 positions.
	positionAnchors := map[int]map[int][]string{}
	for i := 0; i < 15; i++ {
		positionAnchors[i] = map[int][]string{0: {"anchor"}}
	}
	got := captureStderr(func() {
		reportMissingPositions(positionAnchors)
	})
	if !strings.Contains(got, "15") {
		t.Errorf("expected total count 15 in output, got %q", got)
	}
	// Should contain the truncation marker
	if !strings.Contains(got, "...") {
		t.Errorf("expected truncation marker '...' in output for 15 positions, got %q", got)
	}
	if !strings.Contains(got, "total") {
		t.Errorf("expected 'total' in output for 15 positions, got %q", got)
	}
}

// ============================================================================
// VAL-B-020: reportDuplicateAnchors
// ============================================================================

func TestReportDuplicateAnchorsSkipsWhenUnresolved(t *testing.T) {
	// Anchor "x" has multiple sites but is not in usedAnchors.
	// Python: checks anchor_name in self.used_anchors.
	anchorPositions := map[string]map[string]struct{}{
		"x": {"1.0": {}, "2.0": {}},
	}
	usedAnchors := map[string]bool{}
	got := captureStderr(func() {
		reportDuplicateAnchors(anchorPositions, usedAnchors)
	})
	if got != "" {
		t.Errorf("expected no output for unresolved anchor, got %q", got)
	}
}

func TestReportDuplicateAnchorsSkipsSingleSite(t *testing.T) {
	// Anchor "y" has only one position — should not be reported.
	anchorPositions := map[string]map[string]struct{}{
		"y": {"1.0": {}},
	}
	usedAnchors := map[string]bool{"y": true}
	got := captureStderr(func() {
		reportDuplicateAnchors(anchorPositions, usedAnchors)
	})
	if got != "" {
		t.Errorf("expected no output for single-site anchor, got %q", got)
	}
}

func TestReportDuplicateAnchorsEmitsForMultiSiteResolved(t *testing.T) {
	// Anchor "z" has 2 positions and is used — must report.
	anchorPositions := map[string]map[string]struct{}{
		"z": {"10.0": {}, "20.1": {}},
	}
	usedAnchors := map[string]bool{"z": true}
	got := captureStderr(func() {
		reportDuplicateAnchors(anchorPositions, usedAnchors)
	})
	if !strings.Contains(got, "z") {
		t.Errorf("expected anchor name 'z' in output, got %q", got)
	}
	if !strings.Contains(got, "10.0") || !strings.Contains(got, "20.1") {
		t.Errorf("expected both positions 10.0 and 20.1 in output, got %q", got)
	}
}

func TestReportDuplicateAnchorsOnlyReportsUsedAnchorsWithMultiplePositions(t *testing.T) {
	// VAL-B-020: Only reports anchors that are BOTH used AND have >1 position.
	// "alpha" is used AND has 2 positions → reported
	// "beta" has 2 positions but is NOT used → not reported
	// "gamma" is used but has only 1 position → not reported
	anchorPositions := map[string]map[string]struct{}{
		"alpha": {"1.0": {}, "1.5": {}},  // 2 positions, used → report
		"beta":  {"2.0": {}, "2.1": {}},  // 2 positions, not used → skip
		"gamma": {"3.0": {}},             // 1 position, used → skip
	}
	usedAnchors := map[string]bool{"alpha": true, "gamma": true}
	got := captureStderr(func() {
		reportDuplicateAnchors(anchorPositions, usedAnchors)
	})
	if !strings.Contains(got, "alpha") {
		t.Error("expected anchor 'alpha' (used + multiple positions) to be reported")
	}
	if strings.Contains(got, "beta") {
		t.Error("anchor 'beta' should NOT be reported (not used)")
	}
	if strings.Contains(got, "gamma") {
		t.Error("anchor 'gamma' should NOT be reported (single position)")
	}
}

func TestReportDuplicateAnchorsPositionsSorted(t *testing.T) {
	// Positions in the error message must be sorted.
	anchorPositions := map[string]map[string]struct{}{
		"multi": {"20.0": {}, "10.5": {}, "15.3": {}},
	}
	usedAnchors := map[string]bool{"multi": true}
	got := captureStderr(func() {
		reportDuplicateAnchors(anchorPositions, usedAnchors)
	})
	// Sorted: 10.5, 15.3, 20.0
	idx1 := strings.Index(got, "10.5")
	idx2 := strings.Index(got, "15.3")
	idx3 := strings.Index(got, "20.0")
	if idx1 >= idx2 || idx2 >= idx3 {
		t.Errorf("positions must be sorted: 10.5 (idx=%d) < 15.3 (idx=%d) < 20.0 (idx=%d), got %q", idx1, idx2, idx3, got)
	}
}

// ============================================================================
// VAL-B-021: registerAnchor populates bidirectionally
// ============================================================================

func TestRegisterAnchorBidirectional(t *testing.T) {
	// Register anchor "x" at (eid=1, offset=10), then "y" at same position.
	// Both must appear in positionAnchors[1][10].
	state := navProcessor{
		usedAnchorNames:   map[string]bool{},
		positionAnchors:   map[int]map[int][]string{},
		anchorSites:       map[string]map[string]struct{}{},
		anchorHeadingLevel: map[string]int{},
	}

	state.registerAnchor("x", navTarget{PositionID: 1, Offset: 10}, nil)
	state.registerAnchor("y", navTarget{PositionID: 1, Offset: 10}, nil)

	// Check positionAnchors: eid=1 → offset=10 → ["x", "y"]
	if state.positionAnchors[1] == nil || state.positionAnchors[1][10] == nil {
		t.Fatal("positionAnchors[1][10] should exist")
	}
	names := state.positionAnchors[1][10]
	if len(names) != 2 || names[0] != "x" || names[1] != "y" {
		t.Errorf("expected positionAnchors[1][10] = [x, y], got %v", names)
	}

	// Check anchorSites: each anchor → set of site keys
	if len(state.anchorSites["x"]) != 1 || len(state.anchorSites["y"]) != 1 {
		t.Errorf("expected each anchor to have 1 site, got x=%d y=%d", len(state.anchorSites["x"]), len(state.anchorSites["y"]))
	}
	if _, ok := state.anchorSites["x"]["1.10"]; !ok {
		t.Error("expected anchorSites[x] to contain '1.10'")
	}
	if _, ok := state.anchorSites["y"]["1.10"]; !ok {
		t.Error("expected anchorSites[y] to contain '1.10'")
	}
}

func TestRegisterAnchorMultiplePositions(t *testing.T) {
	// Register anchor "a" at two different positions.
	state := navProcessor{
		usedAnchorNames:    map[string]bool{},
		positionAnchors:    map[int]map[int][]string{},
		anchorSites:        map[string]map[string]struct{}{},
		anchorHeadingLevel: map[string]int{},
	}

	state.registerAnchor("a", navTarget{PositionID: 10, Offset: 0}, nil)
	state.registerAnchor("a", navTarget{PositionID: 20, Offset: 5}, nil)

	// anchorSites["a"] should have 2 entries
	if len(state.anchorSites["a"]) != 2 {
		t.Errorf("expected 2 sites for anchor 'a', got %d", len(state.anchorSites["a"]))
	}
	if _, ok := state.anchorSites["a"]["10.0"]; !ok {
		t.Error("expected anchorSites[a] to contain '10.0'")
	}
	if _, ok := state.anchorSites["a"]["20.5"]; !ok {
		t.Error("expected anchorSites[a] to contain '20.5'")
	}

	// positionAnchors should have both entries
	if state.positionAnchors[10][0][0] != "a" {
		t.Error("expected positionAnchors[10][0] to contain 'a'")
	}
	if state.positionAnchors[20][5][0] != "a" {
		t.Error("expected positionAnchors[20][5] to contain 'a'")
	}
}

func TestRegisterAnchorStoresHeadingLevel(t *testing.T) {
	state := navProcessor{
		usedAnchorNames:    map[string]bool{},
		positionAnchors:    map[int]map[int][]string{},
		anchorSites:        map[string]map[string]struct{}{},
		anchorHeadingLevel: map[string]int{},
	}
	level := 3
	state.registerAnchor("h", navTarget{PositionID: 5, Offset: 0}, &level)
	if state.anchorHeadingLevel["h"] != 3 {
		t.Errorf("expected heading level 3, got %d", state.anchorHeadingLevel["h"])
	}
}

func TestRegisterAnchorSkipsEmptyName(t *testing.T) {
	state := navProcessor{
		usedAnchorNames:    map[string]bool{},
		positionAnchors:    map[int]map[int][]string{},
		anchorSites:        map[string]map[string]struct{}{},
		anchorHeadingLevel: map[string]int{},
	}
	state.registerAnchor("", navTarget{PositionID: 5, Offset: 0}, nil)
	if len(state.anchorSites) != 0 {
		t.Error("expected no anchorSites for empty name")
	}
	if len(state.positionAnchors) != 0 {
		t.Error("expected no positionAnchors for empty name")
	}
}

func TestRegisterAnchorSkipsZeroPosition(t *testing.T) {
	state := navProcessor{
		usedAnchorNames:    map[string]bool{},
		positionAnchors:    map[int]map[int][]string{},
		anchorSites:        map[string]map[string]struct{}{},
		anchorHeadingLevel: map[string]int{},
	}
	state.registerAnchor("a", navTarget{PositionID: 0, Offset: 0}, nil)
	if len(state.anchorSites) != 0 {
		t.Error("expected no anchorSites for zero PositionID")
	}
	if len(state.positionAnchors) != 0 {
		t.Error("expected no positionAnchors for zero PositionID")
	}
}

// ============================================================================
// VAL-B-022: processNavigation initializes all data structures
// ============================================================================

func TestProcessNavigationInitializesStructures(t *testing.T) {
	// Calling processNavigation with no nav roots should produce empty structures.
	state := processNavigation(nil, nil, "", nil, false)
	if state.positionAnchors == nil {
		t.Error("positionAnchors should be non-nil")
	}
	if state.anchorSites == nil {
		t.Error("anchorSites should be non-nil")
	}
	if state.usedAnchorNames == nil {
		t.Error("usedAnchorNames should be non-nil")
	}
	if state.anchorHeadingLevel == nil {
		t.Error("anchorHeadingLevel should be non-nil")
	}
	if state.pageLabelAnchorID == nil {
		t.Error("pageLabelAnchorID should be non-nil")
	}
	if len(state.positionAnchors) != 0 {
		t.Error("positionAnchors should be empty")
	}
	if len(state.anchorSites) != 0 {
		t.Error("anchorSites should be empty")
	}
}

// ============================================================================
// VAL-B-023: Navigation type matching for TOC, landmarks, page-list
// ============================================================================

func TestNavContainerTypeTOC(t *testing.T) {
	container := map[string]interface{}{
		"nav_type": "toc",
		"entries": []interface{}{
			map[string]interface{}{
				"representation": map[string]interface{}{"label": "Chapter 1"},
				"target_position": map[string]interface{}{"id": 10, "offset": 0},
			},
		},
	}
	navContainers := map[string]map[string]interface{}{}
	state := processNavigation([]map[string]interface{}{}, navContainers, "", nil, false)
	state.processContainer(container, false)
	if len(state.toc) != 1 {
		t.Fatalf("expected 1 TOC entry, got %d", len(state.toc))
	}
	if state.toc[0].Title != "Chapter 1" {
		t.Errorf("expected title 'Chapter 1', got %q", state.toc[0].Title)
	}
}

func TestNavContainerTypeLandmarks(t *testing.T) {
	container := map[string]interface{}{
		"nav_type": "landmarks",
		"entries": []interface{}{
			map[string]interface{}{
				"representation": map[string]interface{}{"label": "Cover"},
				"landmark_type": "cover_page",
				"target_position": map[string]interface{}{"id": 5, "offset": 0},
			},
		},
	}
	navContainers := map[string]map[string]interface{}{}
	state := processNavigation([]map[string]interface{}{}, navContainers, "", nil, false)
	state.processContainer(container, false)
	if len(state.guide) != 1 {
		t.Fatalf("expected 1 guide entry, got %d", len(state.guide))
	}
	if state.guide[0].Type != "cover" {
		t.Errorf("expected guide type 'cover', got %q", state.guide[0].Type)
	}
	if state.guide[0].Title != "Cover" {
		t.Errorf("expected guide title 'Cover', got %q", state.guide[0].Title)
	}
}

func TestNavContainerTypePageList(t *testing.T) {
	container := map[string]interface{}{
		"nav_type": "page_list",
		"entries": []interface{}{
			map[string]interface{}{
				"representation": map[string]interface{}{"label": "42"},
				"target_position": map[string]interface{}{"id": 10, "offset": 0},
			},
		},
	}
	navContainers := map[string]map[string]interface{}{}
	state := processNavigation([]map[string]interface{}{}, navContainers, "", nil, false)
	state.processContainer(container, false)
	if len(state.pages) != 1 {
		t.Fatalf("expected 1 page entry, got %d", len(state.pages))
	}
	if state.pages[0].Label != "42" {
		t.Errorf("expected page label '42', got %q", state.pages[0].Label)
	}
}

// ============================================================================
// Sorting helper tests
// ============================================================================

func TestSortPositionStrings(t *testing.T) {
	// Verify that sortPositionStrings produces correct string ordering.
	// Python sorts as plain strings: "42.0" < "7.3" because "4" < "7".
	input := []string{"7.3", "1.10", "42.0", "42.5"}
	expected := []string{"1.10", "42.0", "42.5", "7.3"}
	sort.Sort(sortablePositionStrings(input))
	for i, got := range input {
		if got != expected[i] {
			t.Errorf("position[%d]: expected %q, got %q", i, expected[i], got)
		}
	}
}

func TestTruncateListShort(t *testing.T) {
	// List of 3 items should be returned unchanged.
	input := []string{"a", "b", "c"}
	result := truncatePositionList(input)
	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}
}

func TestTruncateListLong(t *testing.T) {
	// List of 15 items should be truncated to 10 + "... (15 total)".
	input := make([]string, 15)
	for i := range input {
		input[i] = "item"
	}
	result := truncatePositionList(input)
	if len(result) != 11 {
		t.Errorf("expected 11 items (10 + 1 summary), got %d: %v", len(result), result)
	}
	if !strings.Contains(result[10], "15") || !strings.Contains(result[10], "total") {
		t.Errorf("expected summary with count, got %q", result[10])
	}
}

// ============================================================================
// VAL-M11-001: Missing nav warning for reading orders (N1 gap)
// Port of Python process_navigation L101-102 for-else pattern:
//   for reading_order in self.reading_orders:
//       ...
//   else:
//       if not self.book.is_scribe_notebook:
//           log.warning("Failed to locate navigation for reading order \"%s\"" % reading_order_name)
// ============================================================================

func TestWarnUnmatchedReadingOrdersNoWarningWhenMatched(t *testing.T) {
	// When every reading order has a matching nav root, no warning should be emitted.
	navRoots := []map[string]interface{}{
		{"reading_order_name": "main", "nav_containers": []interface{}{}},
	}
	readingOrderNames := []string{"main"}
	got := captureStderr(func() {
		warnUnmatchedReadingOrders(navRoots, readingOrderNames, false)
	})
	if strings.Contains(got, "Failed to locate navigation") {
		t.Errorf("expected no warning when reading order is matched, got %q", got)
	}
}

func TestWarnUnmatchedReadingOrdersWarningWhenMissing(t *testing.T) {
	// When a reading order has no matching nav root, a warning should be emitted.
	navRoots := []map[string]interface{}{
		{"reading_order_name": "other", "nav_containers": []interface{}{}},
	}
	readingOrderNames := []string{"main"}
	got := captureStderr(func() {
		warnUnmatchedReadingOrders(navRoots, readingOrderNames, false)
	})
	if !strings.Contains(got, "Failed to locate navigation") {
		t.Errorf("expected warning about missing navigation for reading order 'main', got %q", got)
	}
	if !strings.Contains(got, "main") {
		t.Errorf("expected warning to mention reading order name 'main', got %q", got)
	}
}

func TestWarnUnmatchedReadingOrdersNoWarningForScribeNotebook(t *testing.T) {
	// Python: if not self.book.is_scribe_notebook: log.warning(...)
	// When isScribeNotebook=true, no warning should be emitted even if no match.
	navRoots := []map[string]interface{}{}
	readingOrderNames := []string{"main"}
	got := captureStderr(func() {
		warnUnmatchedReadingOrders(navRoots, readingOrderNames, true)
	})
	if strings.Contains(got, "Failed to locate navigation") {
		t.Errorf("expected no warning for scribe notebook, got %q", got)
	}
}

func TestWarnUnmatchedReadingOrdersMultipleReadingOrders(t *testing.T) {
	// Two reading orders, one matched and one unmatched.
	navRoots := []map[string]interface{}{
		{"reading_order_name": "main", "nav_containers": []interface{}{}},
	}
	readingOrderNames := []string{"main", "secondary"}
	got := captureStderr(func() {
		warnUnmatchedReadingOrders(navRoots, readingOrderNames, false)
	})
	if !strings.Contains(got, "secondary") {
		t.Errorf("expected warning for unmatched 'secondary' reading order, got %q", got)
	}
	if strings.Contains(got, "main") {
		t.Errorf("should not warn about matched 'main' reading order, got %q", got)
	}
}

func TestWarnUnmatchedReadingOrdersEmptyReadingOrders(t *testing.T) {
	// No reading orders → no warning.
	navRoots := []map[string]interface{}{}
	got := captureStderr(func() {
		warnUnmatchedReadingOrders(navRoots, nil, false)
	})
	if got != "" {
		t.Errorf("expected no output with no reading orders, got %q", got)
	}
}

func TestWarnUnmatchedReadingOrdersNavRootNoName(t *testing.T) {
	// Nav root with no reading_order_name should not match anything.
	navRoots := []map[string]interface{}{
		{"nav_containers": []interface{}{}},
	}
	readingOrderNames := []string{"main"}
	got := captureStderr(func() {
		warnUnmatchedReadingOrders(navRoots, readingOrderNames, false)
	})
	if !strings.Contains(got, "Failed to locate navigation") {
		t.Errorf("expected warning when nav root has no reading_order_name, got %q", got)
	}
}
