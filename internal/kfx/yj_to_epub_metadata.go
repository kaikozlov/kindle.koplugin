package kfx

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

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
				if value, ok := asString(entry["value"]); ok && value != "" {
					book.Title = strings.TrimSpace(value)
				}
			case "kindle_title_metadata/author":
				if value, ok := asString(entry["value"]); ok && value != "" {
					// Python uses authors.insert(0, value) — prepend, so last entry becomes first.
					book.Authors = append([]string{value}, book.Authors...)
				}
			case "kindle_title_metadata/author_pronunciation":
				// Python stores in self.author_pronunciations; not needed for EPUB output.
			case "kindle_title_metadata/language":
				if value, ok := asString(entry["value"]); ok && value != "" {
					book.Language = value
				}
			case "kindle_title_metadata/issue_date":
				if value, ok := asString(entry["value"]); ok && value != "" {
					book.Published = value
				}
			case "kindle_title_metadata/description":
				if value, ok := asString(entry["value"]); ok && value != "" {
					book.Description = strings.TrimSpace(value)
				}
			case "kindle_title_metadata/cover_image":
				if value, ok := asString(entry["value"]); ok && value != "" {
					book.CoverImageID = value
				}
			case "kindle_title_metadata/publisher":
				if value, ok := asString(entry["value"]); ok && value != "" {
					book.Publisher = strings.TrimSpace(value)
				}
			case "kindle_title_metadata/override_kindle_font":
				if value, ok := asBool(entry["value"]); ok {
					book.OverrideKindleFonts = value
				}
			case "kindle_title_metadata/ASIN", "ASIN":
				if value, ok := asString(entry["value"]); ok && value != "" && book.ASIN == "" {
					book.ASIN = value
				}
			case "kindle_title_metadata/book_id":
				if value, ok := asString(entry["value"]); ok && value != "" {
					book.BookID = value
				}
			case "kindle_title_metadata/content_id":
				if value, ok := asString(entry["value"]); ok && value != "" {
					book.Identifier = value
				}
			case "kindle_title_metadata/cde_content_type", "cde_content_type":
				// Python sets cde_content_type; may affect book_type (MAGZ→magazine, EBSP→sample).
				// Not yet wired to Go book type.
			case "kindle_ebook_metadata/book_orientation_lock":
				if value, ok := asString(entry["value"]); ok && value != "" {
					book.OrientationLock = value
				}
			case "kindle_capability_metadata/yj_fixed_layout":
				if value, ok := asInt(entry["value"]); ok && value > 0 {
					book.FixedLayout = true
				}
			case "kindle_capability_metadata/yj_illustrated_layout":
				if value, ok := asBool(entry["value"]); ok && value {
					book.IllustratedLayout = true
				}
			case "kindle_capability_metadata/yj_facing_page", "kindle_capability_metadata/yj_double_page_spread":
				// Python sets book_type = "comic"; not yet wired.
			case "kindle_capability_metadata/yj_publisher_panels":
				// Python sets book_type = "comic" + virtual_panels/region_magnification; not yet wired.
			case "kindle_title_metadata/support_landscape":
				// Python (L286): if value is False and self.orientation_lock == "none".
				// Go uses "" as the "none" default (when $433 is absent), but applyDocumentData
				// may have set it to "none" explicitly (when $433 == "none"). Check both.
				if value, ok := asBool(entry["value"]); ok && !value && (book.OrientationLock == "" || book.OrientationLock == "none") {
					book.OrientationLock = "portrait"
				}
			case "kindle_title_metadata/support_portrait":
				// Python (L289): if value is False and self.orientation_lock == "none".
				if value, ok := asBool(entry["value"]); ok && !value && (book.OrientationLock == "" || book.OrientationLock == "none") {
					book.OrientationLock = "landscape"
				}
			}
		}
	}
}

func applyDocumentData(book *decodedBook, value map[string]interface{}) {
	if book == nil || value == nil {
		return
	}
	// Port of Python process_document_data orientation_lock ($433 → ORIENTATIONS).
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
	// Python process_content_properties extracts these into doc_style; simplified here.
	if raw, ok := asString(value["direction"]); ok && raw != "" {
		book.PageProgressionDirection = raw
	}
	if raw, ok := asString(value["writing-mode"]); ok && raw != "" {
		book.WritingMode = raw
		if strings.HasSuffix(raw, "-rl") {
			book.PageProgressionDirection = "rtl"
		}
	}
	if book.WritingMode == "" {
		book.WritingMode = "horizontal-tb"
	}
	if book.PageProgressionDirection == "" {
		book.PageProgressionDirection = "ltr"
	}
	// Port of Python process_document_data (yj_to_epub_metadata.py L110):
	//   doc_style = self.process_content_properties(document_data)
	//   self.default_font_family = doc_style.pop("font-family", DEFAULT_DOCUMENT_FONT_FAMILY)
	// Python also sets font_name_replacements["default"] to the first font family name.
	// We store the raw font-family value here (or fallback to "serif").
	// The actual resolution happens in renderBookState via the font name fixer,
	// because @font-face fonts must be registered first for proper case resolution.
	if rawFF, ok := asString(value["font_family"]); ok && rawFF != "" {
		book.DefaultFontFamily = rawFF
	} else {
		book.DefaultFontFamily = "serif"
	}
}

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
var metadataSymbolNames = map[string]string{
	"ASIN": "ASIN",
	"asset_id": "asset_id",
	"author": "author",
	"cde_content_type": "cde_content_type",
	"cover_image": "cover_image",
	"description": "description",
	"language":  "language",
	"orientation": "orientation",
	"publisher": "publisher",
	"reading_orders": "reading_orders",
	"title": "title",
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
func applyMetadataItem(book *decodedBook, key string, value interface{}) {
	switch key {
	case "ASIN":
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
		// Python: if not self.title: self.title = value — $258 title only fills if $490 didn't set one.
		if s, ok := asString(value); ok && s != "" && book.Title == "" {
			book.Title = strings.TrimSpace(s)
		}
	}
}
