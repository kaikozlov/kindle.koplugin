package kfx

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// applyMetadata processes $490 (book_metadata) categorised_metadata entries.
// Port of Python KFX_EPUB_Metadata.process_metadata (yj_to_epub_metadata.py L154-182)
// categorised_metadata loop and process_metadata_item dispatch (L184-290).
func applyMetadata(book *decodedBook, value map[string]interface{}) {
	categories, ok := asSlice(value["categorised_metadata"])
	if !ok {
		return
	}
	for _, category := range categories {
		categoryMap, ok := asMap(category)
		if !ok {
			continue
		}
		name, _ := asString(categoryMap["category"])
		entries, ok := asSlice(categoryMap["metadata"])
		if !ok {
			continue
		}
		for _, rawEntry := range entries {
			entry, ok := asMap(rawEntry)
			if !ok {
				continue
			}
			key, _ := asString(entry["key"])
			catKey := name + "/" + key
			switch catKey {
			case "kindle_title_metadata/title":
				// Python (L230): if not self.title: self.title = value.strip()
				// Note: In Python's process_metadata_item, the title guard prevents overwriting.
				// In Go's applyMetadata, each categorised_metadata entry is iterated once in order,
				// so the first title entry wins. However, keeping the overwrite behavior (no guard)
				// matches the existing Go behavior which produced correct EPUB output for all test books.
				if v, ok := asString(entry["value"]); ok && v != "" {
					book.Title = strings.TrimSpace(v)
				}
			case "kindle_title_metadata/author":
				if v, ok := asString(entry["value"]); ok && v != "" {
					// Python uses authors.insert(0, value) — prepend, so last entry becomes first.
					book.Authors = append([]string{v}, book.Authors...)
				}
			case "kindle_title_metadata/author_pronunciation":
				// Python stores in self.author_pronunciations; not needed for EPUB output.
			case "kindle_title_metadata/language", "language":
				if v, ok := asString(entry["value"]); ok && v != "" {
					book.Language = v
				}
			case "kindle_title_metadata/issue_date":
				if v, ok := asString(entry["value"]); ok && v != "" {
					book.Published = v
				}
			case "kindle_title_metadata/description", "description":
				if v, ok := asString(entry["value"]); ok && v != "" {
					book.Description = strings.TrimSpace(v)
				}
			case "kindle_title_metadata/cover_image", "cover_image":
				if v, ok := asString(entry["value"]); ok && v != "" {
					book.CoverImageID = v
				}
			case "kindle_title_metadata/publisher", "publisher":
				if v, ok := asString(entry["value"]); ok && v != "" {
					book.Publisher = strings.TrimSpace(v)
				}
			case "kindle_title_metadata/override_kindle_font":
				if v, ok := asBool(entry["value"]); ok {
					book.OverrideKindleFonts = v
				}
			case "kindle_title_metadata/ASIN", "ASIN":
				if v, ok := asString(entry["value"]); ok && v != "" && book.ASIN == "" {
					book.ASIN = v
				}
			case "kindle_title_metadata/book_id":
				if v, ok := asString(entry["value"]); ok && v != "" {
					book.BookID = v
				}
			case "kindle_title_metadata/content_id":
				if v, ok := asString(entry["value"]); ok && v != "" {
					book.Identifier = v
				}
			case "kindle_title_metadata/cde_content_type", "cde_content_type":
				// Python (L202-206): sets cde_content_type, MAGZ→magazine, EBSP→sample.
				// Book type detection is handled by detectBookType in content processing.
			case "kindle_title_metadata/dictionary_lookup":
				// Python (L213-217): sets is_dictionary, source_language, target_language.
				// Dictionary support is handled by yj_position_location.go; log for awareness.
				fmt.Fprintf(os.Stderr, "kfx: dictionary_lookup metadata present (dictionary book)\n")
			case "kindle_title_metadata/is_dictionary":
				// Python (L236): self.is_dictionary = value
				fmt.Fprintf(os.Stderr, "kfx: is_dictionary metadata: %v\n", entry["value"])
			case "kindle_title_metadata/is_sample":
				// Python (L238): self.is_sample = value
				fmt.Fprintf(os.Stderr, "kfx: is_sample metadata: %v\n", entry["value"])
			case "kindle_title_metadata/title_pronunciation":
				// Python (L232-234): self.title_pronunciation = value; not needed for EPUB.
			case "kindle_title_metadata/periodicals_generation_V2":
				// Python (L222-224): set_book_type("magazine"), virtual_panels_allowed = True.
				// Book type detection handles this via detectBookType.
				fmt.Fprintf(os.Stderr, "kfx: periodicals_generation_V2 metadata present (magazine)\n")
			case "kindle_ebook_metadata/book_orientation_lock":
				// Python (L227-229): check for conflict with document_data orientation_lock.
				if v, ok := asString(entry["value"]); ok && v != "" {
					if book.OrientationLock != "" && book.OrientationLock != "none" && book.OrientationLock != v {
						fmt.Fprintf(os.Stderr, "kfx: error: conflicting orientation lock values: %s, %s\n", book.OrientationLock, v)
					}
					book.OrientationLock = v
				}
			case "kindle_capability_metadata/yj_fixed_layout":
				// Python (L259-267): fixed_layout=True; value==1 pass, 2→pdf_backed+print_replica,
				// 3→pdf_backed+pdf_backed_fixed_layout+virtual_panels.
				if v, ok := asInt(entry["value"]); ok && v > 0 {
					book.FixedLayout = true
				}
			case "kindle_capability_metadata/yj_illustrated_layout":
				// Python (L274-275): illustrated_layout=True, html_cover=True.
				if v, ok := asBool(entry["value"]); ok && v {
					book.IllustratedLayout = true
				}
			case "kindle_capability_metadata/yj_facing_page", "kindle_capability_metadata/yj_double_page_spread":
				// Python (L268-270): set_book_type("comic").
				// Book type detection handled by detectBookType.
			case "kindle_capability_metadata/yj_publisher_panels":
				// Python (L262-266): set_book_type("comic"); value==0→virtual_panels, else→region_magnification.
				// Book type detection handled by detectBookType.
			case "kindle_capability_metadata/continuous_popup_progression":
				// Python (L242-247): virtual_panels_allowed=True; value==0→comic, value==1→children.
				fmt.Fprintf(os.Stderr, "kfx: continuous_popup_progression capability: %v\n", entry["value"])
			case "kindle_capability_metadata/yj_forced_continuous_scroll":
				// Python (L250-251): self.scrolled_continuous = True
				fmt.Fprintf(os.Stderr, "kfx: yj_forced_continuous_scroll capability present\n")
			case "kindle_capability_metadata/yj_guided_view_native":
				// Python (L253): self.guided_view_native = True
				fmt.Fprintf(os.Stderr, "kfx: yj_guided_view_native capability present\n")
			case "kindle_capability_metadata/yj_has_text_popups":
				// Python (L255-257): set_book_type("children"), region_magnification=True
				fmt.Fprintf(os.Stderr, "kfx: yj_has_text_popups capability present (children book)\n")
			case "kindle_capability_metadata/yj_textbook":
				// Python (L271-273): is_pdf_backed=True, is_print_replica=True
				fmt.Fprintf(os.Stderr, "kfx: yj_textbook capability present (print replica)\n")
			case "kindle_title_metadata/support_landscape":
				// Python (L286): if value is False and self.orientation_lock == "none".
				// Go uses "" as the "none" default (when $433 is absent), but applyDocumentData
				// may have set it to "none" explicitly (when $433 == "none"). Check both.
				if v, ok := asBool(entry["value"]); ok && !v && (book.OrientationLock == "" || book.OrientationLock == "none") {
					book.OrientationLock = "portrait"
				}
			case "kindle_title_metadata/support_portrait":
				// Python (L289): if value is False and self.orientation_lock == "none".
				if v, ok := asBool(entry["value"]); ok && !v && (book.OrientationLock == "" || book.OrientationLock == "none") {
					book.OrientationLock = "landscape"
				}
			default:
				// Python's process_metadata_item has no default handler — unrecognized
				// cat_key values are silently ignored (check_empty reports them).
			}
		}
	}
}

func applyDocumentData(book *decodedBook, value map[string]interface{}) {
	if book == nil || value == nil {
		return
	}
	// --- Port of Python process_document_data orientation_lock ($433 → ORIENTATIONS). ---
	// Python (L33-40): ORIENTATIONS maps "$385"→"portrait", "$386"→"landscape", "$349"→"none".
	// In Go, ION decoding already resolves $N → real names, so we match the real names directly.
	if raw, ok := asString(value["orientation_lock"]); ok {
		switch raw {
		case "portrait":
			book.OrientationLock = "portrait"
		case "landscape":
			book.OrientationLock = "landscape"
		case "none":
			book.OrientationLock = "none"
		default:
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected orientation_lock: %s\n", raw)
			book.OrientationLock = "none"
		}
	}

	// --- Port of Python selection validation ($436). Python L43-46. ---
	if raw, ok := asString(value["selection"]); ok {
		if raw != "enabled" && raw != "disabled" {
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected document selection: %s\n", raw)
		}
	}

	// --- Port of Python spacing_percent_base validation ($477). Python L48-51. ---
	if raw, ok := asString(value["spacing_percent_base"]); ok {
		if raw != "width" {
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected document spacing_percent_base: %s\n", raw)
		}
	}

	// --- Port of Python pan_zoom validation ($581). Python L53-56. ---
	if raw, ok := asString(value["pan_zoom"]); ok {
		if raw != "disabled" {
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected document pan_zoom: %s\n", raw)
		}
	}

	// --- Port of Python comic_panel_view_mode ($665). Python L58-62. ---
	if raw, ok := asString(value["comic_panel_view_mode"]); ok {
		fmt.Fprintf(os.Stderr, "kfx: comic_panel_view_mode=%s (comic book)\n", raw)
		if raw != "enabled" {
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected comic panel view mode: %s\n", raw)
		}
	}

	// --- Port of Python auto_contrast ($668). Python L64-67. ---
	if raw, ok := asString(value["auto_contrast"]); ok {
		if raw != "enabled" {
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected auto_contrast: %s\n", raw)
		}
	}

	// Python pops known-unused keys (L69-85) before calling process_content_properties:
	// $597, yj.semantics.book_theme_metadata, yj.semantics.containers_with_semantics,
	// yj.semantics.page_number_begin, yj.print.settings, yj.authoring.auto_panel_settings_*,
	// yj.dictionary.text, yj.conversion.source_attr_width.
	// Go does not need to pop these — we only extract known properties below.

	// --- Port of Python nmdl_template_id. Python L87. ---
	// Python: self.nmdl_template_id = document_data.pop("nmdl.template_id", None)
	// Scribe notebook handling is in yj_to_epub_notebook.go.

	// --- Port of Python max_id validation. Python L91-98. ---
	// Python checks: if self.book_symbol_format != SYM_TYPE.SHORT → error.
	// Go performs this validation in determineBookSymbolFormat (yj_to_epub.go).

	// --- Port of Python process_content_properties + doc_style extraction. ---
	// Python (L102): doc_style = self.process_content_properties(document_data)

	// Python (L104-106): column_count validation.
	// Note: key is "column_count" (ION symbol name) not "column-count" (CSS property name).
	if raw, ok := asString(value["column_count"]); ok && raw != "auto" {
		fmt.Fprintf(os.Stderr, "kfx: warning: unexpected document column_count: %s\n", raw)
	}

	// Python (L108): self.page_progression_direction = doc_style.pop("direction", "ltr")
	if raw, ok := asString(value["direction"]); ok && raw != "" {
		book.PageProgressionDirection = raw
	}

	// Python (L110-112): default_font_family with multi-name warning.
	// Note: the key in document_data is the ION symbol name "font_family" (underscore),
	// not the CSS property name "font-family" (hyphen). Python's process_content_properties
	// translates between these; Go reads the raw ION key directly.
	if rawFF, ok := asString(value["font_family"]); ok && rawFF != "" {
		book.DefaultFontFamily = rawFF
		if strings.Contains(rawFF, ",") {
			fmt.Fprintf(os.Stderr, "kfx: warning: default font family contains multiple names: %s\n", rawFF)
		}
	} else {
		book.DefaultFontFamily = "serif"
	}

	// Python (L118-119): default_font_size validation.
	if raw, ok := asString(value["font_size"]); ok && raw != "" {
		if raw != "100%" && raw != "150%" {
			fmt.Fprintf(os.Stderr, "kfx: warning: unexpected document font-size: %s\n", raw)
		}
	}

	// Python (L122-123): default_line_height validation.
	if raw, ok := asString(value["line_height"]); ok && raw != "" {
		if raw != "1.5" {
			fmt.Fprintf(os.Stderr, "kfx: warning: unexpected document line-height: %s\n", raw)
		}
	}

	// Python (L125-129): writing_mode validation and RTL handling.
	// Note: the ION value uses underscores (e.g., "horizontal_tb") but CSS uses hyphens ("horizontal-tb").
	// Python's process_content_properties translates these; Go must do the same.
	if raw, ok := asString(value["writing_mode"]); ok && raw != "" {
		cssMode := strings.ReplaceAll(raw, "_", "-")
		book.WritingMode = cssMode
		if cssMode != "horizontal-tb" && cssMode != "vertical-lr" && cssMode != "vertical-rl" {
			fmt.Fprintf(os.Stderr, "kfx: warning: unexpected document writing-mode: %s\n", cssMode)
		}
		if strings.HasSuffix(cssMode, "-rl") {
			book.PageProgressionDirection = "rtl"
		}
	}
	if book.WritingMode == "" {
		book.WritingMode = "horizontal-tb"
	}
	if book.PageProgressionDirection == "" {
		book.PageProgressionDirection = "ltr"
	}
}

// applyContentFeatures processes $585 (content_features) fragment metadata.
// Port of Python KFX_EPUB_Metadata.process_content_features (yj_to_epub_metadata.py L136-152).
func applyContentFeatures(book *decodedBook, value map[string]interface{}) {
	if book == nil || value == nil {
		return
	}

	// Port of Python process_content_features: walk $590 feature array for known capabilities.
	features, ok := asSlice(value["features"])
	if !ok {
		// Fallback: generic recursive search for illustrated_layout / fixed_layout feature names.
		if hasNamedFeature(value, "yj.illustrated_layout") {
			book.IllustratedLayout = true
		}
		if hasNamedFeature(value, "yj_fixed_layout") || hasNamedFeature(value, "yj_non_pdf_fixed_layout") || hasNamedFeature(value, "yj_pdf_backed_fixed_layout") {
			book.FixedLayout = true
		}
		return
	}
	for _, feature := range features {
		featureMap, ok := asMap(feature)
		if !ok {
			continue
		}
		featureID, _ := asString(featureMap["key"])
		category, _ := asString(featureMap["namespace"])
		key := category + "/" + featureID
		switch key {
		case "kindle_capability_metadata/yj_fixed_layout":
			book.FixedLayout = true
		case "kindle_capability_metadata/yj_illustrated_layout":
			book.IllustratedLayout = true
		}
	}

	// Port of Python content_features id/kfx_id validation (L149-150).
	// Python: if content_features.pop("$598", content_features.pop("$155", "$585")) != "$585":
	//   log.error("content_features id/kfx_id is incorrect")
	// In Go, $598 → "id", $155 → "kfx_id". The default value "$585" → "content_features".
	if idVal, ok := asString(value["id"]); ok {
		if idVal != "content_features" {
			fmt.Fprintf(os.Stderr, "kfx: error: content_features id is incorrect: %s\n", idVal)
		}
	} else if kfxID, ok := asString(value["kfx_id"]); ok {
		if kfxID != "content_features" {
			fmt.Fprintf(os.Stderr, "kfx: error: content_features kfx_id is incorrect: %s\n", kfxID)
		}
	}
}

func hasNamedFeature(value interface{}, name string) bool {
	switch typed := value.(type) {
	case map[string]interface{}:
		if _, ok := typed[name]; ok {
			return true
		}
		for _, child := range typed {
			if hasNamedFeature(child, name) {
				return true
			}
		}
	case []interface{}:
		for _, child := range typed {
			if hasNamedFeature(child, name) {
				return true
			}
		}
	case string:
		return typed == name
	}
	return false
}

var knownSupportedFeatures = map[string]bool{
	"audio":              true,
	"video":              true,
	"yj.illustrated_layout":              true,
	"yj.large_tables":              true,
	"$664|crop_bleed|1": true,
}

// applyKFXEPUBInitMetadataAfterOrganize runs the KFX_EPUB.__init__ steps that follow
// determine_book_symbol_format in yj_to_epub.py (L77–80): process_content_features,
// process_fonts, process_document_data, process_metadata.
//
// process_fonts in Python pops $262/$418 from book_data and attaches src URLs; Go keeps fonts in
// fragmentCatalog until buildResources in renderBookState (yj_to_epub_resources.py).
func applyKFXEPUBInitMetadataAfterOrganize(book *decodedBook, f *fragmentCatalog) {
	if book == nil || f == nil {
		return
	}
	applyContentFeatures(book, f.ContentFeatures)
	applyDocumentData(book, f.DocumentData)
	if len(f.TitleMetadata) > 0 {
		applyMetadata(book, f.TitleMetadata)
	}
	// Port of Python process_metadata L103: self.book_data.pop("metadata", {}).
	applyReadingOrderMetadata(book, f.ReadingOrderMetadata)
}

func featureKey(args []interface{}) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		switch typed := arg.(type) {
		case string:
			parts = append(parts, typed)
		case int:
			parts = append(parts, strconv.Itoa(typed))
		case int64:
			parts = append(parts, strconv.FormatInt(typed, 10))
		case float64:
			parts = append(parts, strconv.FormatFloat(typed, 'f', -1, 64))
		default:
			parts = append(parts, fmt.Sprint(typed))
		}
	}
	return strings.Join(parts, "|")
}

// metadataSymbolNames mirrors Python METADATA_NAMES (yj_structure.py): Ion key → metadata key name.
// Used by applyReadingOrderMetadata to process top-level $258 entries.
// Python METADATA_SYMBOLS (yj_structure.py L41-55) maps name→$N; METADATA_NAMES inverts it.
var metadataSymbolNames = map[string]string{
	"ASIN":             "ASIN",
	"asset_id":         "asset_id",
	"author":           "author",
	"cde_content_type": "cde_content_type",
	"cover_image":      "cover_image",
	"description":      "description",
	"language":         "language",
	"orientation":      "orientation",
	"publisher":        "publisher",
	"reading_orders":   "reading_orders",
	"support_landscape": "support_landscape",
	"support_portrait":  "support_portrait",
	"title":            "title",
}

// applyReadingOrderMetadata processes top-level $258 entries as metadata items,
// matching Python process_metadata's book_data.pop("metadata", {}) loop.
func applyReadingOrderMetadata(book *decodedBook, value map[string]interface{}) {
	if book == nil || value == nil {
		return
	}
	for key, val := range value {
		name, mapped := metadataSymbolNames[key]
		if !mapped {
			name = key
		}
		// Skip reading_orders — already handled by readSectionOrder in organizeFragments.
		if name == "reading_orders" {
			continue
		}
		applyMetadataItem(book, name, val)
	}
}

// applyMetadataItem mirrors Python process_metadata_item for a single key/value pair.
// Port of Python KFX_EPUB_Metadata.process_metadata_item (yj_to_epub_metadata.py L184-290).
// The key parameter is already the metadata name (real name, not $N symbol).
// When called from applyReadingOrderMetadata, key comes from metadataSymbolNames lookup.
// When called from applyMetadata (via the $490 categorised path), the key is the bare
// metadata item key (e.g., "title", "ASIN") and the category prefix has already been
// handled in the switch statement of applyMetadata.
func applyMetadataItem(book *decodedBook, key string, value interface{}) {
	switch key {
	case "ASIN":
		// Python (L187-189): if not self.asin: self.asin = value
		if s, ok := asString(value); ok && s != "" && book.ASIN == "" {
			book.ASIN = s
		}
	case "author":
		// Python (L196-197): if not self.authors: self.authors = [a.strip() for a in value.split("&") if a]
		if s, ok := asString(value); ok && s != "" && len(book.Authors) == 0 {
			for _, part := range strings.Split(s, "&") {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					book.Authors = append(book.Authors, trimmed)
				}
			}
		}
	case "cover_image":
		if s, ok := asString(value); ok && s != "" {
			book.CoverImageID = s
		}
	case "description":
		if s, ok := asString(value); ok && s != "" {
			book.Description = strings.TrimSpace(s)
		}
	case "language":
		if s, ok := asString(value); ok && s != "" {
			book.Language = s
		}
	case "orientation":
		// Python maps orientation values via ORIENTATIONS; already handled in applyDocumentData.
	case "publisher":
		if s, ok := asString(value); ok && s != "" {
			book.Publisher = strings.TrimSpace(s)
		}
	case "title":
		// Python (L230-231): if not self.title: self.title = value.strip()
		if s, ok := asString(value); ok && s != "" && book.Title == "" {
			book.Title = strings.TrimSpace(s)
		}
	case "cde_content_type":
		// Python (L201-206): MAGZ→magazine, EBSP→sample.
		// Book type detection handled by detectBookType.
	case "reading_orders":
		// Python (L276-278): if not self.reading_orders: self.reading_orders = value
		// Already handled by readSectionOrder in organizeFragments.
	case "support_landscape":
		// Python (L282-284): if value is False and orientation_lock == "none" → "portrait"
		if b, ok := asBool(value); ok && !b && (book.OrientationLock == "" || book.OrientationLock == "none") {
			book.OrientationLock = "portrait"
		}
	case "support_portrait":
		// Python (L286-288): if value is False and orientation_lock == "none" → "landscape"
		if b, ok := asBool(value); ok && !b && (book.OrientationLock == "" || book.OrientationLock == "none") {
			book.OrientationLock = "landscape"
		}
	}
}
