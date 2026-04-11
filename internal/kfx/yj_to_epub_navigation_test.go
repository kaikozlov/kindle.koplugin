package kfx

import "testing"

func TestReportDuplicateAnchorsSkipsWhenUnresolved(t *testing.T) {
	state := navProcessor{
		anchorSites: map[string]map[string]struct{}{
			"x": {"1.0": {}, "2.0": {}},
		},
	}
	reportDuplicateAnchors(state, map[string]string{}) // no resolved entry — Python skips unused
}

func TestReportDuplicateAnchorsSkipsSingleSite(t *testing.T) {
	state := navProcessor{
		anchorSites: map[string]map[string]struct{}{
			"y": {"1.0": {}},
		},
	}
	reportDuplicateAnchors(state, map[string]string{"y": "s.xhtml#y"})
}

func TestReportDuplicateAnchorsEmitsForMultiSiteResolved(t *testing.T) {
	state := navProcessor{
		anchorSites: map[string]map[string]struct{}{
			"z": {"10.0": {}, "20.1": {}},
		},
	}
	// Should not panic; stderr emission is expected when resolved.
	reportDuplicateAnchors(state, map[string]string{"z": "s.xhtml#z"})
}
