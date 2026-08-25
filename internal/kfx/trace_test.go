package kfx

import "testing"

func TestCaptureContentFeaturesUsesDecodedBookFlags(t *testing.T) {
	book := &decodedBook{
		BookID:                 "book-id-is-not-cde-type",
		CDEContentType:         "EBOK",
		IsPrintReplica:         true,
		IsPDFBacked:            true,
		IsPDFBackedFixedLayout: true,
	}
	got := captureContentFeatures(book)
	if got.CDEContentType != "EBOK" {
		t.Fatalf("CDEContentType=%q, want EBOK", got.CDEContentType)
	}
	if !got.IsPrintReplica || !got.IsPDFBacked || !got.IsPDFBackedFixedLayout {
		t.Fatalf("feature flags not captured: %#v", got)
	}
}

func TestCaptureDocumentDataUsesDecodedBookFlags(t *testing.T) {
	book := &decodedBook{
		OrientationLock:      "portrait",
		FixedLayout:          true,
		IllustratedLayout:    true,
		OriginalWidth:        450,
		OriginalHeight:       600,
		RegionMagnification:  true,
		VirtualPanelsAllowed: true,
		GuidedViewNative:     true,
		ScrolledContinuous:   true,
	}
	got := captureDocumentData(book)
	if got.OriginalWidth == nil || *got.OriginalWidth != 450 || got.OriginalHeight == nil || *got.OriginalHeight != 600 {
		t.Fatalf("viewport not captured: %#v", got)
	}
	if got.OrientationLock != "portrait" || !got.FixedLayout || !got.IllustratedLayout ||
		!got.RegionMagnification || !got.VirtualPanelsAllowed || !got.GuidedViewNative || !got.ScrolledContinuous {
		t.Fatalf("document flags not captured: %#v", got)
	}
}

func TestCaptureMetadataReportsDetectedBookType(t *testing.T) {
	book := &decodedBook{FixedLayout: true, IsPDFBacked: true, IsPDFBackedFixedLayout: true, VirtualPanelsAllowed: true}
	got := captureMetadata(book, &fragmentCatalog{})
	if got.BookType != "comic" {
		t.Fatalf("BookType=%q, want comic", got.BookType)
	}
}
