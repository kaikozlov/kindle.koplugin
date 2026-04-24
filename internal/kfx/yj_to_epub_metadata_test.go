package kfx

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Helper: build categorised metadata map for testing applyMetadata.
// ---------------------------------------------------------------------------

func makeCategorisedMetadata(entries []struct {
	category string
	key      string
	value    interface{}
}) map[string]interface{} {
	// Group entries by category
	categoryMap := map[string][]interface{}{}
	for _, e := range entries {
		categoryMap[e.category] = append(categoryMap[e.category], map[string]interface{}{
			"key":   e.key,
			"value": e.value,
		})
	}

	var categories []interface{}
	for cat, meta := range categoryMap {
		categories = append(categories, map[string]interface{}{
			"category": cat,
			"metadata": meta,
		})
	}

	return map[string]interface{}{
		"categorised_metadata": categories,
	}
}

// ---------------------------------------------------------------------------
// VAL-M5-001: IsDictionary flag from dictionary_lookup metadata
// Python yj_to_epub_metadata.py L213-217: self.is_dictionary = True
// ---------------------------------------------------------------------------

func TestApplyMetadata_IsDictionary_DictionaryLookup(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_title_metadata", "dictionary_lookup", map[string]interface{}{
			"source_language": "en",
			"target_language": "fr",
		}},
	})

	applyMetadata(book, meta)

	if !book.IsDictionary {
		t.Error("expected IsDictionary=true when dictionary_lookup metadata present")
	}
}

func TestApplyMetadata_IsDictionary_MetadataTrue(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_title_metadata", "is_dictionary", true},
	})

	applyMetadata(book, meta)

	if !book.IsDictionary {
		t.Error("expected IsDictionary=true when is_dictionary=true")
	}
}

func TestApplyMetadata_IsDictionary_MetadataFalse(t *testing.T) {
	book := &decodedBook{IsDictionary: true}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_title_metadata", "is_dictionary", false},
	})

	applyMetadata(book, meta)

	if book.IsDictionary {
		t.Error("expected IsDictionary=false when is_dictionary=false")
	}
}

// ---------------------------------------------------------------------------
// VAL-M5-002: IsSample flag from cde_content_type and is_sample metadata
// Python yj_to_epub_metadata.py L201-206, L238
// ---------------------------------------------------------------------------

func TestApplyMetadata_IsSample_EBSP(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_title_metadata", "cde_content_type", "EBSP"},
	})

	applyMetadata(book, meta)

	if !book.IsSample {
		t.Error("expected IsSample=true when cde_content_type=EBSP")
	}
	if book.CDEContentType != "EBSP" {
		t.Errorf("expected CDEContentType=EBSP, got %q", book.CDEContentType)
	}
}

func TestApplyMetadata_IsSample_MetadataTrue(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_title_metadata", "is_sample", true},
	})

	applyMetadata(book, meta)

	if !book.IsSample {
		t.Error("expected IsSample=true when is_sample=true")
	}
}

func TestApplyMetadata_IsSample_MetadataFalse(t *testing.T) {
	book := &decodedBook{IsSample: true}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_title_metadata", "is_sample", false},
	})

	applyMetadata(book, meta)

	if book.IsSample {
		t.Error("expected IsSample=false when is_sample=false explicitly sets it")
	}
}

func TestApplyMetadata_CDEContentType_MAGZ(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_title_metadata", "cde_content_type", "MAGZ"},
	})

	applyMetadata(book, meta)

	if book.CDEContentType != "MAGZ" {
		t.Errorf("expected CDEContentType=MAGZ, got %q", book.CDEContentType)
	}
	if book.IsSample {
		t.Error("expected IsSample=false for MAGZ (only EBSP sets sample)")
	}
}

func TestApplyMetadata_CDEContentType_EBOK(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_title_metadata", "cde_content_type", "EBOK"},
	})

	applyMetadata(book, meta)

	if book.CDEContentType != "EBOK" {
		t.Errorf("expected CDEContentType=EBOK, got %q", book.CDEContentType)
	}
}

// ---------------------------------------------------------------------------
// VAL-M5-003: ScrolledContinuous flag from yj_forced_continuous_scroll
// Python yj_to_epub_metadata.py L250-251
// ---------------------------------------------------------------------------

func TestApplyMetadata_ScrolledContinuous(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_capability_metadata", "yj_forced_continuous_scroll", true},
	})

	applyMetadata(book, meta)

	if !book.ScrolledContinuous {
		t.Error("expected ScrolledContinuous=true when yj_forced_continuous_scroll present")
	}
}

// ---------------------------------------------------------------------------
// VAL-M5-004: GuidedViewNative flag from yj_guided_view_native
// Python yj_to_epub_metadata.py L253
// ---------------------------------------------------------------------------

func TestApplyMetadata_GuidedViewNative(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_capability_metadata", "yj_guided_view_native", true},
	})

	applyMetadata(book, meta)

	if !book.GuidedViewNative {
		t.Error("expected GuidedViewNative=true when yj_guided_view_native present")
	}
}

// ---------------------------------------------------------------------------
// VAL-M5-005: RegionMagnification flag from yj_has_text_popups and yj_publisher_panels
// Python yj_to_epub_metadata.py L255-257, L262-266
// ---------------------------------------------------------------------------

func TestApplyMetadata_RegionMagnification_TextPopups(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_capability_metadata", "yj_has_text_popups", true},
	})

	applyMetadata(book, meta)

	if !book.RegionMagnification {
		t.Error("expected RegionMagnification=true when yj_has_text_popups present")
	}
}

func TestApplyMetadata_RegionMagnification_PublisherPanelsNonZero(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_capability_metadata", "yj_publisher_panels", 1},
	})

	applyMetadata(book, meta)

	if !book.RegionMagnification {
		t.Error("expected RegionMagnification=true when yj_publisher_panels=1")
	}
}

func TestApplyMetadata_VirtualPanelsAllowed_PublisherPanelsZero(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_capability_metadata", "yj_publisher_panels", 0},
	})

	applyMetadata(book, meta)

	if book.RegionMagnification {
		t.Error("expected RegionMagnification=false when yj_publisher_panels=0")
	}
	if !book.VirtualPanelsAllowed {
		t.Error("expected VirtualPanelsAllowed=true when yj_publisher_panels=0")
	}
}

// ---------------------------------------------------------------------------
// VAL-M5-006: IsPDFBacked and IsPrintReplica flags from yj_fixed_layout and yj_textbook
// Python yj_to_epub_metadata.py L259-267, L271-273
// ---------------------------------------------------------------------------

func TestApplyMetadata_FixedLayoutValue1(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_capability_metadata", "yj_fixed_layout", 1},
	})

	applyMetadata(book, meta)

	if !book.FixedLayout {
		t.Error("expected FixedLayout=true when yj_fixed_layout=1")
	}
	if book.IsPDFBacked {
		t.Error("expected IsPDFBacked=false when yj_fixed_layout=1")
	}
	if book.IsPrintReplica {
		t.Error("expected IsPrintReplica=false when yj_fixed_layout=1")
	}
}

func TestApplyMetadata_FixedLayoutValue2(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_capability_metadata", "yj_fixed_layout", 2},
	})

	applyMetadata(book, meta)

	if !book.FixedLayout {
		t.Error("expected FixedLayout=true when yj_fixed_layout=2")
	}
	if !book.IsPDFBacked {
		t.Error("expected IsPDFBacked=true when yj_fixed_layout=2")
	}
	if !book.IsPrintReplica {
		t.Error("expected IsPrintReplica=true when yj_fixed_layout=2")
	}
}

func TestApplyMetadata_FixedLayoutValue3(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_capability_metadata", "yj_fixed_layout", 3},
	})

	applyMetadata(book, meta)

	if !book.FixedLayout {
		t.Error("expected FixedLayout=true when yj_fixed_layout=3")
	}
	if !book.IsPDFBacked {
		t.Error("expected IsPDFBacked=true when yj_fixed_layout=3")
	}
	if !book.IsPDFBackedFixedLayout {
		t.Error("expected IsPDFBackedFixedLayout=true when yj_fixed_layout=3")
	}
	if !book.VirtualPanelsAllowed {
		t.Error("expected VirtualPanelsAllowed=true when yj_fixed_layout=3")
	}
	if book.IsPrintReplica {
		t.Error("expected IsPrintReplica=false when yj_fixed_layout=3")
	}
}

func TestApplyMetadata_Textbook(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_capability_metadata", "yj_textbook", 1},
	})

	applyMetadata(book, meta)

	if !book.IsPDFBacked {
		t.Error("expected IsPDFBacked=true when yj_textbook present")
	}
	if !book.IsPrintReplica {
		t.Error("expected IsPrintReplica=true when yj_textbook present")
	}
}

// ---------------------------------------------------------------------------
// VAL-M5-007: HTMLCover flag from yj_illustrated_layout
// Python yj_to_epub_metadata.py L274-275: self.illustrated_layout = self.html_cover = True
// ---------------------------------------------------------------------------

func TestApplyMetadata_IllustratedLayout_HTMLCover(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_capability_metadata", "yj_illustrated_layout", true},
	})

	applyMetadata(book, meta)

	if !book.IllustratedLayout {
		t.Error("expected IllustratedLayout=true when yj_illustrated_layout=true")
	}
	if !book.HTMLCover {
		t.Error("expected HTMLCover=true when yj_illustrated_layout=true (Python L274-275)")
	}
}

func TestApplyMetadata_IllustratedLayout_False(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_capability_metadata", "yj_illustrated_layout", false},
	})

	applyMetadata(book, meta)

	if book.IllustratedLayout {
		t.Error("expected IllustratedLayout=false when yj_illustrated_layout=false")
	}
	if book.HTMLCover {
		t.Error("expected HTMLCover=false when yj_illustrated_layout=false")
	}
}

// ---------------------------------------------------------------------------
// VAL-M5-008: Title guard — first title wins
// Python yj_to_epub_metadata.py L230: if not self.title: self.title = value.strip()
// ---------------------------------------------------------------------------

func TestApplyMetadata_TitleGuard_FirstWins(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_title_metadata", "title", "First Title"},
		{"kindle_title_metadata", "title", "Second Title"},
	})

	applyMetadata(book, meta)

	if book.Title != "First Title" {
		t.Errorf("expected Title='First Title' (guard should keep first), got %q", book.Title)
	}
}

func TestApplyMetadata_TitleGuard_EmptyBook(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_title_metadata", "title", "My Book"},
	})

	applyMetadata(book, meta)

	if book.Title != "My Book" {
		t.Errorf("expected Title='My Book', got %q", book.Title)
	}
}

func TestApplyMetadata_TitleGuard_PreserveExisting(t *testing.T) {
	book := &decodedBook{Title: "Existing Title"}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_title_metadata", "title", "New Title"},
	})

	applyMetadata(book, meta)

	if book.Title != "Existing Title" {
		t.Errorf("expected Title='Existing Title' (guard should preserve), got %q", book.Title)
	}
}

// ---------------------------------------------------------------------------
// VirtualPanelsAllowed from continuous_popup_progression and periodicals_generation_V2
// Python yj_to_epub_metadata.py L242-247, L222-224
// ---------------------------------------------------------------------------

func TestApplyMetadata_VirtualPanelsAllowed_ContinuousPopupProgression(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_capability_metadata", "continuous_popup_progression", 0},
	})

	applyMetadata(book, meta)

	if !book.VirtualPanelsAllowed {
		t.Error("expected VirtualPanelsAllowed=true when continuous_popup_progression present")
	}
}

func TestApplyMetadata_VirtualPanelsAllowed_PeriodicalsV2(t *testing.T) {
	book := &decodedBook{}
	meta := makeCategorisedMetadata([]struct {
		category string
		key      string
		value    interface{}
	}{
		{"kindle_title_metadata", "periodicals_generation_V2", true},
	})

	applyMetadata(book, meta)

	if !book.VirtualPanelsAllowed {
		t.Error("expected VirtualPanelsAllowed=true when periodicals_generation_V2 present")
	}
}

// ---------------------------------------------------------------------------
// applyContentFeatures sets HTMLCover for illustrated layout
// ---------------------------------------------------------------------------

func TestApplyContentFeatures_IllustratedLayout_HTMLCover(t *testing.T) {
	book := &decodedBook{}
	// Content features uses kindle_capability_metadata namespace for flag setting
	features := map[string]interface{}{
		"features": []interface{}{
			map[string]interface{}{
				"namespace": "kindle_capability_metadata",
				"key":       "yj_illustrated_layout",
			},
		},
	}

	applyContentFeatures(book, features)

	if !book.IllustratedLayout {
		t.Error("expected IllustratedLayout=true from content features")
	}
	if !book.HTMLCover {
		t.Error("expected HTMLCover=true from content features (Python L274-275)")
	}
}

func TestApplyContentFeatures_FixedLayoutFromCapability(t *testing.T) {
	book := &decodedBook{}
	features := map[string]interface{}{
		"features": []interface{}{
			map[string]interface{}{
				"namespace": "kindle_capability_metadata",
				"key":       "yj_fixed_layout",
			},
		},
	}

	applyContentFeatures(book, features)

	if !book.FixedLayout {
		t.Error("expected FixedLayout=true from content features yj_fixed_layout")
	}
}

// ---------------------------------------------------------------------------
// applyMetadataItem: cde_content_type via reading order metadata
// ---------------------------------------------------------------------------

func TestApplyMetadataItem_CDEContentType(t *testing.T) {
	book := &decodedBook{}
	applyMetadataItem(book, "cde_content_type", "EBSP")

	if book.CDEContentType != "EBSP" {
		t.Errorf("expected CDEContentType=EBSP, got %q", book.CDEContentType)
	}
	if !book.IsSample {
		t.Error("expected IsSample=true for EBSP via applyMetadataItem")
	}
}

func TestApplyMetadataItem_TitleGuard(t *testing.T) {
	book := &decodedBook{}
	applyMetadataItem(book, "title", "First")
	applyMetadataItem(book, "title", "Second")

	if book.Title != "First" {
		t.Errorf("expected Title='First' (guard), got %q", book.Title)
	}
}

// ---------------------------------------------------------------------------
// VAL-M12-SCRIBE: IsScribeNotebook detection from document_data nmdl.template_id
// Python: kpf_container.py L150/163 sets is_scribe_notebook=True
// Go detects from nmdl.template_id in document_data (yj_to_epub_metadata.py L87)
// ---------------------------------------------------------------------------

func TestApplyDocumentData_ScribeNotebookDetection(t *testing.T) {
	// When nmdl.template_id is present in document_data, the book should be
	// detected as a scribe notebook. Python detects from ACTION/DELTA fragment
	// schemas; Go detects from nmdl.template_id presence.
	book := &decodedBook{}
	value := map[string]interface{}{
		"nmdl.template_id": "my_template",
	}
	applyDocumentData(book, value)
	if !book.IsScribeNotebook {
		t.Error("expected IsScribeNotebook=true when nmdl.template_id is present in document_data")
	}
}

func TestApplyDocumentData_NotScribeNotebook(t *testing.T) {
	// When nmdl.template_id is absent, the book should not be detected as scribe notebook.
	book := &decodedBook{}
	value := map[string]interface{}{}
	applyDocumentData(book, value)
	if book.IsScribeNotebook {
		t.Error("expected IsScribeNotebook=false when nmdl.template_id is absent")
	}
}
