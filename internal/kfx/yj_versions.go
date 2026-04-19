package kfx

import "strings"

// yj_versions.go ports yj_versions.py from the Calibre KFX Input plugin.
// Python source: REFERENCE/Calibre_KFX_Input/kfxlib/yj_versions.py (1124 lines)
//
// This file contains version constants, known generators, feature registries,
// metadata dictionaries, and validation functions for the KFX format.

// ---------------------------------------------------------------------------
// 1.1 Sentinel Constants (Python lines 6-7)
// ---------------------------------------------------------------------------

// Any is the Python ANY sentinel (True). It is used as a wildcard key in
// KNOWN_FEATURES, KNOWN_METADATA, KNOWN_AUXILIARY_METADATA, and KNOWN_KCB_DATA
// to indicate that any value is accepted.
const Any = true

// TF is the Python TF set {False, True}. In Go we represent it as a small
// lookup function so callers can test whether a value is in the set.
var TF = map[bool]bool{false: true, true: true}

// ---------------------------------------------------------------------------
// 1.2 String Constants — Feature Names (Python lines 9-100)
// ---------------------------------------------------------------------------

const (
	// General capabilities (lines 9-14)
	ArticleReaderV1         = "ARTICLE_READER_V1"
	DualReadingOrderV1      = "DUAL_READING_ORDER_V1"
	GuidedViewNativeV1      = "GUIDED_VIEW_NATIVE_V1"
	JPEGXREncodingSupported = "JPEGXR_ENCODING_SUPPORTED"
	KindleRecapsV1          = "KINDLE_RECAPS_V1"
	MOPSupported            = "MOP_SUPPORTED"

	// NMDL note variants (lines 15-18)
	NMDLNote   = "NMDL_NOTE"
	NMDLNoteV2 = "NMDL_NOTE_V2"
	NMDLNoteV3 = "NMDL_NOTE_V3"
	NMDLNoteV4 = "NMDL_NOTE_V4"

	// HD / HDV / Vella (lines 19-22)
	SupportsHDV1  = "SUPPORTS_HD_V1"
	SupportsHDVV1 = "SUPPORTS_HDV_V1"
	SupportsHDVV2 = "SUPPORTS_HDV_V2"
	VellaSupported = "VELLA_SUPPORTED"

	// Generic (line 23)
	YJ = "YJ"

	// JP Vertical (lines 24-31)
	YJJPVV1SimpleVertical = "YJJPV_V1_SIMPLEVERTICAL"
	YJJPVV2TrackB         = "YJJPV_V2_TRACKB"
	YJJPVV3TrackB         = "YJJPV_V3_TRACKB"
	YJJPVV4TrackDPart1    = "YJJPV_V4_TRACKD_PART1"
	YJJPVV5               = "YJJPV_V5"
	YJJPVV6               = "YJJPV_V6"
	YJJPVV7               = "YJJPV_V7"
	YJJPVV8               = "YJJPV_V8"

	// Audio (lines 32-34)
	YJAudioV1 = "YJ_AUDIO_V1"
	YJAudioV2 = "YJ_AUDIO_V2"
	YJAudioV3 = "YJ_AUDIO_V3"

	// Conditional structure, cover, dict (lines 35-41)
	YJConditionalStructureV1 = "YJ_CONDITIONAL_STRUCTURE_V1"
	YJCoverImageDeferV1      = "YJ_COVER_IMAGE_DEFER_V1"
	YJCoverImageSeparateV1   = "YJ_COVER_IMAGE_SEPARATE_V1"
	YJDictV1                 = "YJ_DICT_V1"
	YJDictV1Arabic           = "YJ_DICT_V1_ARABIC"

	// Fixed layout (lines 42-48)
	YJFixedLayoutV2       = "YJ_FIXED_LAYOUT_V2"
	YJFixedLayoutPDF      = "YJ_FIXED_LAYOUT_PDF"
	YJFixedLayoutPDFV2    = "YJ_FIXED_LAYOUT_PDF_V2"
	YJFixedLayoutPDFV3    = "YJ_FIXED_LAYOUT_PDF_V3"
	YJFixedLayoutPDocsPDF = "YJ_FIXED_LAYOUT_PDOCS_PDF"

	// Other capabilities (lines 49-56)
	YJForcedContinuousScrollV1 = "YJ_FORCED_CONTINUOUS_SCROLL_V1"
	YJInteractivityV1          = "YJ_INTERACTIVITY_V1"
	YJInteractivityV2          = "YJ_INTERACTIVITY_V2"
	YJMathmlV1                 = "YJ_MATHML_V1"
	YJMixedWritingModeV1       = "YJ_MIXED_WRITING_MODE_V1"
	YJMixedWritingModeV2       = "YJ_MIXED_WRITING_MODE_V2"
	YJNonPDFAudioVideoV1       = "YJ_NON_PDF_AUDIO_VIDEO_V1"

	// PDF-backed (lines 57-60)
	YJPDFBackedFixedLayoutV1     = "YJ_PDF_BACKED_FIXED_LAYOUT_V1"
	YJPDFBackedFixedLayoutV1Test = "YJ_PDF_BACKED_FIXED_LAYOUT_V1_TEST"
	YJPDFBackedFixedLayoutV2     = "YJ_PDF_BACKED_FIXED_LAYOUT_V2"
	YJPDFLinks                   = "YJ_PDF_LINKS"

	// Publisher panels (lines 61-64)
	YJPublisherPanelsV2 = "YJ_PUBLISHER_PANELS_V2"
	YJPublisherPanelsV3 = "YJ_PUBLISHER_PANELS_V3"
	YJPublisherPanelsV4 = "YJ_PUBLISHER_PANELS_V4"

	// Ruby, reflowable (lines 65-82)
	YJRubyV1          = "YJ_RUBY_V1"
	YJReflowable      = "YJ_REFLOWABLE"
	YJReflowableV2    = "YJ_REFLOWABLE_V2"
	YJReflowableV3    = "YJ_REFLOWABLE_V3"
	YJReflowableV4    = "YJ_REFLOWABLE_V4"
	YJReflowableV5    = "YJ_REFLOWABLE_V5"
	YJReflowableV6    = "YJ_REFLOWABLE_V6"
	YJReflowableV7    = "YJ_REFLOWABLE_V7"
	YJReflowableV8    = "YJ_REFLOWABLE_V8"
	YJReflowableV9    = "YJ_REFLOWABLE_V9"
	YJReflowableV10   = "YJ_REFLOWABLE_V10"
	YJReflowableV11   = "YJ_REFLOWABLE_V11"
	YJReflowableV12   = "YJ_REFLOWABLE_V12"
	YJReflowableV13   = "YJ_REFLOWABLE_V13"
	YJReflowableV14   = "YJ_REFLOWABLE_V14"

	// Language-specific reflowable (lines 83-91)
	YJReflowableARv1           = "YJ_REFLOWABLE_AR_v1"
	YJReflowableCNv1           = "YJ_REFLOWABLE_CN_v1"
	YJReflowableFAV1           = "YJ_REFLOWABLE_FA_V1"
	YJReflowableHEV1           = "YJ_REFLOWABLE_HE_V1"
	YJReflowableIndicV1        = "YJ_REFLOWABLE_INDIC_V1"
	YJReflowableJPv1           = "YJ_REFLOWABLE_JP_v1"
	YJReflowableLangExpansionV1 = "YJ_REFLOWABLE_LANG_EXPANSION_V1"
	YJReflowableLangExpansionV2 = "YJ_REFLOWABLE_LANG_EXPANSION_V2"
	YJReflowableLargeSection   = "YJ_REFLOWABLE_LARGESECTION"

	// Tables (lines 92-104)
	YJReflowableTablesv1  = "YJ_REFLOWABLE_TABLESv1"
	YJReflowableTablesv2  = "YJ_REFLOWABLE_TABLESv2"
	YJReflowableTablesv3  = "YJ_REFLOWABLE_TABLESv3"
	YJReflowableTablesv4  = "YJ_REFLOWABLE_TABLESv4"
	YJReflowableTablesv5  = "YJ_REFLOWABLE_TABLESv5"
	YJReflowableTablesv6  = "YJ_REFLOWABLE_TABLESv6"
	YJReflowableTablesv7  = "YJ_REFLOWABLE_TABLESv7"
	YJReflowableTablesv8  = "YJ_REFLOWABLE_TABLESv8"
	YJReflowableTablesv9  = "YJ_REFLOWABLE_TABLESv9"
	YJReflowableTablesv10 = "YJ_REFLOWABLE_TABLESv10"
	YJReflowableTablesv11 = "YJ_REFLOWABLE_TABLESv11"

	// Table viewer (lines 105-106)
	YJReflowableTableViewerv1 = "YJ_REFLOWABLE_TABLEVIEWERv1"
	YJReflowableTableViewerv2 = "YJ_REFLOWABLE_TABLEVIEWERv2"

	// TCN, text popups, vertical text shadow, video (lines 107-110)
	YJReflowableTCNv1        = "YJ_REFLOWABLE_TCN_v1"
	YJTextPopUpsV1           = "YJ_TEXT_POPUPS_V1"
	YJVerticalTextShadowV1   = "YJ_VERTICAL_TEXT_SHADOW_V1"
	YJVideoV1                = "YJ_VIDEO_V1"
	YJVideoV3                = "YJ_VIDEO_V3"
)

// ---------------------------------------------------------------------------
// 1.3 PACKAGE_VERSION_PLACEHOLDERS (Python lines 103-107)
// ---------------------------------------------------------------------------

var PackageVersionPlaceholders = map[string]bool{
	"PackageVersion:YJReaderSDK-1.0.x.x GitSHA:c805492 Month-Day:04-22":   true,
	"PackageVersion:YJReaderSDK-1.0.x.x GitSHA:[33mc805492[m Month-Day:04-22": true,
	"kfxlib-00000000": true,
}

// ---------------------------------------------------------------------------
// 1.4 KNOWN_KFX_GENERATORS (Python lines 110-211)
// ---------------------------------------------------------------------------

// GeneratorEntry represents a (version_string, package_version_string) tuple.
type GeneratorEntry struct {
	Version        string
	PackageVersion string
}

// KnownKFXGenerators is the Go equivalent of Python's KNOWN_KFX_GENERATORS set.
var KnownKFXGenerators = map[GeneratorEntry]bool{
	{"2.16", "PackageVersion:YJReaderSDK-1.0.824.0 Month-Day:04-09"}:         true,
	{"3.41.1.0", "PackageVersion:YJReaderSDK-1.0.1962.11 Month-Day:10-17"}:   true,
	{"3.42.1.0", "PackageVersion:YJReaderSDK-1.0.2044.4 Month-Day:10-28"}:    true,
	{"6.11.1.2", "PackageVersion:YJReaderSDK-1.0.2467.43 Month-Day:07-05"}:   true,
	{"6.11.1.2", "PackageVersion:YJReaderSDK-1.0.2467.8 Month-Day:07-14"}:    true,
	{"6.11.1.2", "PackageVersion:YJReaderSDK-1.0.2539.3 Month-Day:03-17"}:    true,
	{"6.20.1.0", "PackageVersion:YJReaderSDK-1.0.2685.4 Month-Day:05-19"}:    true,
	{"6.24.1.0", "PackageVersion:YJReaderSDK-1.1.67.2 Month-Day:06-18"}:      true,
	{"6.28.1.0", "PackageVersion:YJReaderSDK-1.1.67.4 Month-Day:07-14"}:      true,
	{"6.28.2.0", "PackageVersion:YJReaderSDK-1.1.147.0 Month-Day:09-10"}:     true,
	{"7.38.1.0", "PackageVersion:YJReaderSDK-1.2.173.0 Month-Day:09-20"}:     true,
	{"7.45.1.0", "PackageVersion:YJReaderSDK-1.4.23.0 Month-Day:11-23"}:      true,
	{"7.58.1.0", "PackageVersion:YJReaderSDK-1.5.116.0 Month-Day:02-25"}:     true,
	{"7.66.1.0", "PackageVersion:YJReaderSDK-1.5.185.0 Month-Day:04-13"}:     true,
	{"7.66.1.0", "PackageVersion:YJReaderSDK-1.5.195.0 Month-Day:04-20"}:     true,
	{"7.91.1.0", "PackageVersion:YJReaderSDK-1.5.566.6 Month-Day:11-03"}:     true,
	{"7.91.1.0", "PackageVersion:YJReaderSDK-1.5.595.1 Month-Day:11-30"}:     true,
	{"7.111.1.1", "PackageVersion:YJReaderSDK-1.6.444.0 Month-Day:02-27"}:    true,
	{"7.111.1.1", "PackageVersion:YJReaderSDK-1.6.444.5 Month-Day:03-20"}:    true,
	{"7.121.3.0", "PackageVersion:YJReaderSDK-1.6.444.18 Month-Day:05-02"}:   true,
	{"7.125.1.0", "PackageVersion:YJReaderSDK-1.6.444.24 Month-Day:06-01"}:   true,
	{"7.125.1.0", "PackageVersion:YJReaderSDK-1.6.444.33 Month-Day:06-16"}:   true,
	{"7.131.2.0", "PackageVersion:YJReaderSDK-1.6.444.36 Month-Day:07-10"}:   true,
	{"7.135.2.0", "PackageVersion:YJReaderSDK-1.6.1034.2 Month-Day:08-23"}:   true,
	{"7.135.2.0", "PackageVersion:YJReaderSDK-1.6.1034.13 Month-Day:10-09"}:  true,
	{"7.135.2.0", "PackageVersion:YJReaderSDK-1.6.1034.17 Month-Day:11-06"}:  true,
	{"7.149.1.0", "PackageVersion:YJReaderSDK-1.6.1034.59 Month-Day:12-06"}:  true,
	{"7.149.1.0", "PackageVersion:YJReaderSDK-1.6.1034.62 Month-Day:12-21"}:  true,
	{"7.149.1.0", "PackageVersion:YJReaderSDK-1.6.1034.72 Month-Day:01-04"}:  true,
	{"7.149.1.0", "PackageVersion:YJReaderSDK-1.6.1871.0 Month-Day:01-23"}:   true,
	{"7.149.1.0", "PackageVersion:YJReaderSDK-1.6.1938.0 Month-Day:01-29"}:   true,
	{"7.149.1.0", "PackageVersion:YJReaderSDK-1.6.2071.0 Month-Day:02-12"}:   true,
	{"7.149.1.0", "PackageVersion:YJReaderSDK-1.6.200363.0 Month-Day:03-19"}: true,
	{"7.153.1.0", ""}: true,
	{"7.165.1.1", ""}: true,
	{"7.168.1.0", ""}: true,
	{"7.169.1.0", ""}: true,
	{"7.171.1.0", ""}: true,
	{"7.174.1.0", ""}: true,
	{"7.177.1.0", ""}: true,
	{"7.180.1.0", ""}: true,
	{"7.182.1.0", ""}: true,
	{"7.188.1.0", ""}: true,
	{"7.191.1.0", ""}: true,
	{"7.213.1.0", ""}: true,
	{"7.220.2.0", ""}: true,
	{"7.228.1.0", ""}: true,
	{"7.232.1.0", ""}: true,
	{"7.236.1.0", ""}: true,
	{"20.12.238.0", ""}: true,
}

// ---------------------------------------------------------------------------
// 1.5 GENERIC_CREATOR_VERSIONS (Python lines 213-217)
// ---------------------------------------------------------------------------

var GenericCreatorVersions = map[GeneratorEntry]bool{
	{"YJConversionTools", "2.15.0"}: true,
	{"KTC", "1.0.11.1"}:             true,
	{"", ""}:                         true,
}

// ---------------------------------------------------------------------------
// 1.6 KNOWN_FEATURES (Python lines 220-466)
//
// Type: map[category] -> map[featureKey] -> map[versionKey] -> featureName
// versionKey can be int, [2]int (tuple), or the string "ANY" (sentinel).
// We represent the sentinel as a special key type.
// ---------------------------------------------------------------------------

// VersionKey is the key type for KNOWN_FEATURES version maps.
// It can represent an integer version, a pair of integers (tuple),
// or the ANY sentinel.
type VersionKey struct {
	IntVal  int
	Tuple   [2]int
	IsTuple bool
	IsAny   bool
}

// IntVersionKey creates an integer VersionKey.
func IntVersionKey(v int) VersionKey {
	return VersionKey{IntVal: v}
}

// TupleVersionKey creates a 2-tuple VersionKey.
func TupleVersionKey(a, b int) VersionKey {
	return VersionKey{Tuple: [2]int{a, b}, IsTuple: true}
}

// AnyVersionKey creates the ANY sentinel VersionKey.
func AnyVersionKey() VersionKey {
	return VersionKey{IsAny: true}
}

// FeatureVersionMap maps version keys to feature name strings.
type FeatureVersionMap map[VersionKey]string

// FeatureKeyMap maps feature key names to their version maps.
type FeatureKeyMap map[string]FeatureVersionMap

// KnownFeatures is the Go equivalent of Python's KNOWN_FEATURES dict.
var KnownFeatures = map[string]FeatureKeyMap{
	"format_capabilities": {
		"kfxgen.pidMapWithOffset": {IntVersionKey(1): YJ},
		"kfxgen.positionMaps":     {IntVersionKey(2): YJ},
		"kfxgen.textBlock":        {IntVersionKey(1): YJ},
		"db.delta_update":         {IntVersionKey(1): NMDLNote},
		"db.schema":               {IntVersionKey(1): YJ},
	},
	"SDK.Marker": {
		"CanonicalFormat": {
			IntVersionKey(1): YJ,
			IntVersionKey(2): YJ,
		},
	},
	"com.amazon.kindle.nmdl": {
		"note": {
			IntVersionKey(2): NMDLNoteV2,
			IntVersionKey(3): NMDLNoteV3,
			IntVersionKey(4): NMDLNoteV4,
		},
	},
	"com.amazon.yjconversion": {
		"ar-reflow-language": {
			IntVersionKey(1): YJReflowableARv1,
		},
		"cn-reflow-language": {
			IntVersionKey(1): YJReflowableCNv1,
		},
		"fa-reflow-language": {
			IntVersionKey(1): YJReflowableFAV1,
		},
		"he-reflow-language": {
			IntVersionKey(1): YJReflowableHEV1,
		},
		"indic-reflow-language": {
			IntVersionKey(1): YJReflowableIndicV1,
		},
		"jp-reflow-language": {
			IntVersionKey(1): YJReflowableJPv1,
		},
		"jpvertical-reflow-language": {
			IntVersionKey(2): YJJPVV2TrackB,
			IntVersionKey(3): YJJPVV3TrackB,
			IntVersionKey(4): YJJPVV4TrackDPart1,
			IntVersionKey(5): YJJPVV5,
			IntVersionKey(6): YJJPVV6,
			IntVersionKey(7): YJJPVV7,
		},
		"reflow-language": {
			IntVersionKey(2): YJReflowableLangExpansionV1,
			IntVersionKey(3): YJReflowableIndicV1,
		},
		"reflow-language-expansion": {
			IntVersionKey(1): YJReflowableLangExpansionV1,
		},
		"tcn-reflow-language": {
			IntVersionKey(1): YJReflowableTCNv1,
		},
		"multiple_reading_orders-switchable": {
			IntVersionKey(1): DualReadingOrderV1,
		},
		"reflow-section-size": {
			AnyVersionKey(): YJReflowableLargeSection,
		},
		"reflow-style": {
			IntVersionKey(1):                     YJReflowable,
			IntVersionKey(2):                     YJReflowableV2,
			IntVersionKey(3):                     YJReflowableV3,
			IntVersionKey(4):                     YJReflowableV4,
			IntVersionKey(5):                     YJReflowableV5,
			IntVersionKey(6):                     YJReflowableV6,
			IntVersionKey(7):                     YJReflowableV7,
			IntVersionKey(8):                     YJReflowableV8,
			IntVersionKey(9):                     YJReflowableV9,
			IntVersionKey(10):                    YJReflowableV10,
			IntVersionKey(11):                    YJReflowableV11,
			IntVersionKey(12):                    YJReflowableV12,
			IntVersionKey(13):                    YJReflowableV13,
			IntVersionKey(14):                    YJReflowableV14,
			TupleVersionKey(2147483646, 2147483647): YJ,
			TupleVersionKey(2147483647, 2147483647): YJ,
		},
		"yj_arabic_fixed_format": {
			IntVersionKey(1): YJ,
		},
		"yj_audio": {
			IntVersionKey(1): YJAudioV1,
			IntVersionKey(2): YJAudioV2,
			IntVersionKey(3): YJAudioV3,
		},
		"yj_custom_word_iterator": {
			IntVersionKey(1): YJFixedLayoutPDF,
		},
		"yj_dictionary": {
			IntVersionKey(1): YJDictV1,
			IntVersionKey(2): YJDictV1Arabic,
		},
		"yj_direction_rtl": {
			IntVersionKey(1): YJ,
		},
		"yj_double_page_spread": {
			IntVersionKey(1): YJFixedLayoutV2,
		},
		"yj_facing_page": {
			IntVersionKey(1): YJFixedLayoutV2,
		},
		"yj_fixed_layout": {
			IntVersionKey(1): YJFixedLayoutPDF,
		},
		"yj_graphical_highlights": {
			IntVersionKey(1): YJFixedLayoutPDF,
		},
		"yj_has_text_popups": {
			IntVersionKey(1): YJTextPopUpsV1,
		},
		"yj_hdv": {
			IntVersionKey(1): SupportsHDVV1,
			IntVersionKey(2): SupportsHDVV2,
		},
		"yj_interactive_image": {
			IntVersionKey(1): YJInteractivityV1,
		},
		"yj_jpegxr_sd": {
			IntVersionKey(1): JPEGXREncodingSupported,
		},
		"yj_jpg_rst_marker_present": {
			IntVersionKey(1): YJ,
		},
		"yj_mathml": {
			IntVersionKey(1): YJMathmlV1,
		},
		"yj_mixed_writing_mode": {
			IntVersionKey(1): YJMixedWritingModeV1,
			IntVersionKey(2): YJMixedWritingModeV2,
		},
		"yj_non_pdf_fixed_layout": {
			IntVersionKey(2): YJFixedLayoutV2,
		},
		"yj_pdf_backed_fixed_layout": {
			IntVersionKey(2): YJPDFBackedFixedLayoutV2,
		},
		"yj_pdf_links": {
			IntVersionKey(1): YJPDFLinks,
		},
		"yj_pdf_support": {
			IntVersionKey(1): YJFixedLayoutPDF,
		},
		"yj_publisher_panels": {
			IntVersionKey(2): YJPublisherPanelsV2,
			IntVersionKey(3): YJPublisherPanelsV3,
		},
		"yj_rotated_pages": {
			IntVersionKey(1): YJFixedLayoutPDF,
		},
		"yj_ruby": {
			IntVersionKey(1): YJRubyV1,
		},
		"yj_table": {
			IntVersionKey(1):  YJReflowableTablesv1,
			IntVersionKey(2):  YJReflowableTablesv2,
			IntVersionKey(3):  YJReflowableTablesv3,
			IntVersionKey(4):  YJReflowableTablesv4,
			IntVersionKey(5):  YJReflowableTablesv5,
			IntVersionKey(6):  YJReflowableTablesv6,
			IntVersionKey(7):  YJReflowableTablesv7,
			IntVersionKey(8):  YJReflowableTablesv8,
			IntVersionKey(9):  YJReflowableTablesv9,
			IntVersionKey(10): YJReflowableTablesv10,
			IntVersionKey(11): YJReflowableTablesv11,
		},
		"yj_table_viewer": {
			IntVersionKey(1): YJReflowableTableViewerv1,
			IntVersionKey(2): YJReflowableTableViewerv2,
		},
		"yj_textbook": {
			IntVersionKey(1): YJFixedLayoutPDF,
		},
		"yj_thumbnails_present": {
			IntVersionKey(1): YJ,
			IntVersionKey(2): YJ,
		},
		"yj_vertical_text_shadow": {
			IntVersionKey(1): YJVerticalTextShadowV1,
		},
		"yj_video": {
			IntVersionKey(1): YJ,
			IntVersionKey(3): YJVideoV3,
		},
		"yj.conditional_structure": {
			IntVersionKey(1): YJConditionalStructureV1,
		},
		"yj.illustrated_layout": {
			IntVersionKey(1): YJReflowableV5,
		},
	},
}

// ---------------------------------------------------------------------------
// 1.7 KNOWN_SUPPORTED_FEATURES (Python lines 469-480)
// ---------------------------------------------------------------------------

// SupportedFeatureEntry represents a KNOWN_SUPPORTED_FEATURES tuple.
type SupportedFeatureEntry struct {
	Symbol   string
	Key      string // empty for single-element tuples
	Version  int    // 0 when not present
	HasTuple bool   // true for 3-element tuples
}

var KnownSupportedFeatures = []SupportedFeatureEntry{
	{Symbol: "$826"},
	{Symbol: "$827"},
	{Symbol: "$660"},
	{Symbol: "$751"},
	{Symbol: "$664", Key: "crop_bleed", Version: 1, HasTuple: true},
}

// ---------------------------------------------------------------------------
// 1.8 KNOWN_METADATA (Python lines 483-935)
//
// In Python, values are sets of strings/ints/floats or the ANY sentinel.
// In Go we use map[string]bool for string sets, map[int]bool for int sets,
// and interface{} for mixed sets. The ANY sentinel is represented by the
// AnyMetadata sentinel value.
// ---------------------------------------------------------------------------

// metadataValueSet represents a set of known values for a metadata key.
// If IsAny is true, any value is accepted. Otherwise, the specific sets
// contain the allowed values.
type metadataValueSet struct {
	IsAny   bool
	Strings map[string]bool
	Ints    map[int]bool
	Floats  map[float64]bool
	Bools   map[bool]bool
}

// KnownMetadata is the Go equivalent of Python's KNOWN_METADATA dict.
// map[category] -> map[key] -> metadataValueSet
var KnownMetadata = map[string]map[string]metadataValueSet{
	"book_navigation": {
		"pages": {IsAny: true},
	},
	"book_requirements": {
		"min_kindle_version": {IsAny: true},
	},
	"kindle_audit_metadata": {
		"file_creator": newStringSet("YJConversionTools", "FLYP", "KTC", "KC", "KPR"),
		"creator_version": newStringSet(
			"2.15.0",
			"0.1.24.0", "0.1.26.0", "2.0.0.1",
			"1.0.11.1", "1.3.0.0", "1.5.14.0", "1.8.1.0", "1.9.2.0",
			"1.11.399.0", "1.11.539.0", "1.12.11.0", "1.13.7.0", "1.13.10.0",
			"0.93.187.0", "0.94.32.0", "0.95.8.0", "0.96.4.0", "0.96.40.0",
			"0.97.79.3", "0.98.260.0", "0.98.315.0", "0.99.28.0", "0.101.1.0",
			"0.102.0.0", "0.103.0.0", "1.0.319.0", "1.1.58.0", "1.2.83.0",
			"1.3.30.0", "1.4.200067.0", "1.5.60.0", "1.6.97.0", "1.7.223.0",
			"1.8.50.0", "1.9.52.0", "1.10.214.0", "1.11.576.0", "1.12.39.0",
			"1.14.112.0", "1.15.20.0", "1.16.2.0", "1.18.0.0", "1.20.1.0",
			"1.21.6.0", "1.22.13.0", "1.23.0.0", "1.24.33.0", "1.25.34.0",
			"1.26.14.0", "1.27.14.0", "1.28.12.0", "1.29.17.0", "1.30.4.0",
			"1.31.0.0", "1.32.1.0", "1.33.3.0", "1.34.20.0", "1.35.210.0",
			"1.35.618.0", "1.35.770.0", "1.36.1.0", "1.36.20.0", "1.37.2.0",
			"1.38.0.0", "1.38.37.0", "1.39.30.0", "1.40.6.0", "1.41.10.0",
			"1.42.2.0", "1.42.6.0", "1.43.0.0", "1.44.13.0", "1.45.20.0",
			"1.46.2.0", "1.47.1.0", "1.48.7.0", "1.49.0.0", "1.50.0.0",
			"1.51.1.0", "1.52.2.0", "1.52.4.0", "1.52.6.0", "1.53.1.0",
			"1.54.0.0", "1.55.0.0", "1.56.0.0", "1.57.0.0", "1.58.0.0",
			"1.59.0.0", "1.60.0.0", "1.60.1.0", "1.60.2.0", "1.61.0.0",
			"1.62.0.0", "1.62.1.0", "1.63.0.0", "1.64.0.0", "1.65.1.0",
			"1.66.0.0", "1.67.0.0", "1.68.0.0", "1.69.0.0", "1.70.0.0",
			"1.71.0.0", "1.72.0.0", "1.72.1.0", "1.73.0.0", "1.74.0.0",
			"1.75.0.0", "1.76.0.0", "1.76.1.0", "1.77.0.0", "1.77.1.0",
			"1.78.0.0", "1.79.0.0", "1.80.0.0", "1.81.0.0", "1.82.0.0",
			"1.83.0.0", "1.83.1.0", "1.84.0.0", "1.85.0.0", "1.86.0.0",
			"1.87.0.0", "1.88.0.0", "1.88.1.0", "1.89.0.0", "1.90.0.0",
			"1.91.0.0", "1.92.0.0", "1.93.0.0", "1.94.0.0", "1.95.0.0",
			"1.96.0.0", "1.97.0.0", "1.98.0.0", "1.99.0.0", "1.100.0.0",
			"1.101.0.0", "1.102.0.0", "1.103.0.0", "1.104.0.0", "1.105.0.0",
			"1.106.0.0", "1.107.0.0", "1.108.0.0", "1.109.0.0", "1.110.0.0",
			"3.0.0", "3.1.0", "3.2.0", "3.3.0", "3.4.0", "3.5.0", "3.6.0",
			"3.7.0", "3.7.1", "3.8.0", "3.9.0", "3.10.0", "3.10.1", "3.11.0",
			"3.12.0", "3.13.0", "3.14.0", "3.15.0", "3.16.0", "3.17.0",
			"3.17.1", "3.20.0", "3.20.1", "3.21.0", "3.22.0", "3.23.0",
			"3.24.0", "3.25.0", "3.26.0", "3.27.0", "3.28.0", "3.28.1",
			"3.29.0", "3.29.1", "3.29.2", "3.30.0", "3.31.0", "3.32.0",
			"3.33.0", "3.34.0", "3.35.0", "3.36.0", "3.36.1", "3.37.0",
			"3.38.0", "3.39.0", "3.39.1", "3.40.0", "3.41.0", "3.42.0",
			"3.43.0", "3.44.0", "3.45.0", "3.46.0", "3.47.0", "3.48.0",
			"3.49.0", "3.50.0", "3.51.0", "3.52.0", "3.52.1", "3.53.0",
			"3.54.0", "3.55.0", "3.56.0", "3.56.1", "3.57.0", "3.57.1",
			"3.58.0", "3.59.0", "3.59.1", "3.60.0", "3.61.0", "3.62.0",
			"3.63.1", "3.64.0", "3.65.0", "3.66.0", "3.67.0", "3.68.0",
			"3.69.0", "3.70.0", "3.70.1", "3.71.0", "3.71.1", "3.72.0",
			"3.73.0", "3.73.1", "3.74.0", "3.75.0", "3.76.0", "3.77.0",
			"3.77.1", "3.78.0", "3.79.0", "3.80.0", "3.81.0", "3.82.0",
			"3.83.0", "3.84.0", "3.85.0", "3.85.1", "3.86.0", "3.87.0",
			"3.88.0", "3.89.0", "3.90.0", "3.91.0", "3.92.0", "3.93.0",
			"3.94.0", "3.95.0", "3.96.0", "3.97.0", "3.98.0", "3.99.0",
			"3.100.0", "3.101.0", "3.102.0", "3.103.0",
		),
	},
	"kindle_capability_metadata": {
		"continuous_popup_progression":   newIntSet(0, 1),
		"graphical_highlights":           newIntSet(1),
		"yj_default_navigation_mode":     newStringSet("swipe_and_scroll"),
		"yj_double_page_spread":          newIntSet(1),
		"yj_facing_page":                 newIntSet(1),
		"yj_fixed_layout":                newIntSet(1, 2, 3),
		"yj_scroll_capability":           newStringSet("toc_scroll_v1"),
		"yj_has_animations":              newIntSet(1),
		"yj_has_text_popups":             newIntSet(1),
		"yj_illustrated_layout":          newIntSet(1),
		"yj_publisher_panels":            newIntSet(0, 1),
		"yj_textbook":                    newIntSet(1),
	},
	"kindle_ebook_metadata": {
		"book_orientation_lock": newStringSet("landscape", "portrait", "none"),
		"intended_audience":     newStringSet("children"),
		"multipage_selection":   newStringSet("disabled"),
		"nested_span":           newStringSet("enabled"),
		"selection":             newStringSet("enabled"),
		"user_visible_labeling": newStringSet("page_exclusive"),
	},
	"kindle_title_metadata": {
		"cde_content_type":      newStringSet("EBOK", "EBSP", "MAGZ", "PDOC"),
		"ASIN":                  {IsAny: true},
		"asset_id":              {IsAny: true},
		"author":                {IsAny: true},
		"author_pronunciation":  {IsAny: true},
		"book_id":               {IsAny: true},
		"content_id":            {IsAny: true},
		"cover_image":           {IsAny: true},
		"description":           {IsAny: true},
		"dictionary_lookup":     {IsAny: true},
		"editionVersion":        {IsAny: true},
		"imprint_pronunciation": {IsAny: true},
		"is_dictionary":         newBoolSet(true),
		"is_sample":             {Bools: TF},
		"issue_date":            {IsAny: true},
		"itemType":              newStringSet("MAGZ"),
		"language":              {IsAny: true},
		"override_kindle_font":  {Bools: TF},
		"parent_asin":           {IsAny: true},
		"periodicals_generation_V2": newStringSet("true"),
		"publisher":             {IsAny: true},
		"title":                 {IsAny: true},
		"title_pronunciation":   {IsAny: true},
		"updateTime":            {IsAny: true},
	},
	"metadata": {
		"ASIN":                    {IsAny: true},
		"asset_id":                {IsAny: true},
		"author":                  {IsAny: true},
		"binding_direction":       newStringSet("binding_direction_left"),
		"cde_content_type":        newStringSet("EBOK", "MAGZ", "PDOC"),
		"cover_image":             {IsAny: true},
		"cover_page":              {IsAny: true},
		"doc_sym_publication_id":  {IsAny: true},
		"description":             {IsAny: true},
		"issue_date":              {IsAny: true},
		"language":                {IsAny: true},
		"orientation":             newStringSet("portrait", "landscape"),
		"parent_asin":             {IsAny: true},
		"publisher":               {IsAny: true},
		"reading_orders":          {IsAny: true},
		"support_landscape":       {Bools: TF},
		"support_portrait":        {Bools: TF},
		"target_NarrowDimension":  {IsAny: true},
		"target_WideDimension":    {IsAny: true},
		"title":                   {IsAny: true},
		"version":                 newFloatSet(1.0),
		"volume_label":            {IsAny: true},
	},
	"symbols": {
		"max_id": newIntSet(
			489, 609, 620, 626, 627, 634, 652, 662, 667, 668,
			673, 681, 693, 695, 696, 697, 700, 701, 705, 716,
			748, 753, 754, 755, 759, 761, 777, 779, 783, 785,
			786, 787, 789, 797, 804, 825, 827, 831, 832, 833,
			834, 851,
		),
	},
}

// ---------------------------------------------------------------------------
// 1.9 KNOWN_AUXILIARY_METADATA (Python lines 938-975)
// ---------------------------------------------------------------------------

var KnownAuxiliaryMetadata = map[string]metadataValueSet{
	"ANCHOR_REFERRED_BY_CONTAINERS": {IsAny: true},
	"auxData_resource_list":         {IsAny: true},
	"base_line":                     {IsAny: true},
	"button_type":                   newIntSet(1),
	"checkbox_state":                {IsAny: true},
	"dropDown_count":                {IsAny: true},
	"filename.opf":                  {IsAny: true},
	"has_large_data_table":          {Bools: TF},
	"IsSymNameBased":                {Bools: TF},
	"IS_TARGET_SECTION":             newBoolSet(true),
	"jpeg_resource_stream":          {IsAny: true},
	"jpeg_resource_stream_aux_id":   {IsAny: true},
	"jpeg_resource_stream_height":   {IsAny: true},
	"jpeg_resource_stream_width":    {IsAny: true},
	"kSectionContainsAVI":           newBoolSet(true),
	"links_extracted":               newBoolSet(true),
	"link_from_text":                {Bools: TF},
	"location":                      {IsAny: true},
	"mime":                          newStringSet("Audio", "Figure", "Video"),
	"ModifiedContentInfo":           {IsAny: true},
	"modified_time":                 {IsAny: true},
	"most-common-computed-style":    {IsAny: true},
	"namespace":                     newStringSet("KindleConversion"),
	"num-dual-covers-removed":       newIntSet(1),
	"page_rotation":                 newIntSet(0, 1),
	"plugin_group_list":             {IsAny: true},
	"resizable_plugin":              {Bools: TF},
	"resource_stream":               {IsAny: true},
	"size":                          {IsAny: true},
	"SourceIdContentInfo":           {IsAny: true},
	"target":                        {IsAny: true},
	"text_baseline":                 {IsAny: true},
	"text_ext":                      newIntSet(1),
	"type":                          newStringSet("resource"),
	"yj.dictionary.first_head_word": {IsAny: true},
	"yj.dictionary.inflection_rules": {IsAny: true},
}

// ---------------------------------------------------------------------------
// 1.10 KNOWN_KCB_DATA (Python lines 978-1043)
// ---------------------------------------------------------------------------

// kcbValueSet represents a set of allowed values for KCB data.
// When IsAny is true, any value is accepted. Otherwise, check the typed sets.
type kcbValueSet struct {
	IsAny   bool
	Strings map[string]bool
	Ints    map[int]bool
	Bools   map[bool]bool
}

var KnownKCBData = map[string]map[string]kcbValueSet{
	"book_state": {
		"book_input_type":           newKCBIntSet(0, 1, 2, 3, 4, 6, 7),
		"book_fl_type":              newKCBIntSet(0, 1, 2),
		"book_manga_comic":          newKCBBoolSet(false),
		"book_reading_direction":    newKCBIntSet(0, 1, 2),
		"book_reading_option":       newKCBIntSet(0, 1, 2),
		"book_target_type":          newKCBIntSet(1, 2, 3),
		"book_virtual_panelmovement": newKCBIntSet(0, 1, 2),
	},
	"content_hash": {},
	"metadata": {
		"book_path":           {IsAny: true},
		"edited_tool_versions": {}, // populated in init()
		"format":              newKCBStringSet("yj"),
		"global_styling":      {Bools: TF},
		"id":                  {IsAny: true},
		"log_path":            {IsAny: true},
		"platform":            newKCBStringSet("mac", "win"),
		"quality_report":      {IsAny: true},
		"source_path":         {IsAny: true},
		"tool_name":           newKCBStringSet("KC", "KPR", "KTC", "Kindle Previewer 3"),
		"tool_version":        {}, // populated in init()
	},
	"tool_data": {
		"cache_path":                  {IsAny: true},
		"created_on":                  {IsAny: true},
		"last_modified_time":          {IsAny: true},
		"link_extract_choice":         {Bools: TF},
		"link_notification_preference": {Bools: TF},
	},
}

// ---------------------------------------------------------------------------
// 1.11 UNSUPPORTED (Python line 1046)
// ---------------------------------------------------------------------------

const Unsupported = "Unsupported"

// ---------------------------------------------------------------------------
// 1.12 KINDLE_VERSION_CAPABILITIES (Python lines 1049-1073)
// ---------------------------------------------------------------------------

// KindleVersionCapabilities maps Kindle firmware version strings to the list
// of feature constants supported by that version. Note that "5.14.3.2" maps
// to a single feature (not a slice), matching the Python code.
var KindleVersionCapabilities = map[string]interface{}{
	"5.6.5":      []string{JPEGXREncodingSupported, YJ, YJReflowable, YJReflowableV2, YJReflowableLargeSection},
	"5.7.2":      []string{YJReflowableV3, YJReflowableV4, YJReflowableTablesv1},
	"5.7.4":      []string{SupportsHDVV1},
	"5.8.2":      []string{YJReflowableTableViewerv1},
	"5.8.5":      []string{YJFixedLayoutV2, YJReflowableIndicV1},
	"5.8.7":      []string{YJReflowableV6, YJReflowableTablesv2, YJReflowableTablesv3, YJReflowableTableViewerv2},
	"5.8.8":      []string{YJReflowableV7, YJReflowableTablesv4, YJReflowableTablesv5},
	"5.8.10":     []string{YJReflowableV8, YJReflowableV9, YJReflowableCNv1},
	"5.9.4":      []string{YJDictV1, YJReflowableV11, YJReflowableJPv1, YJReflowableTablesv6, YJReflowableTablesv7},
	"5.9.6":      []string{YJDictV1Arabic, YJFixedLayoutPDFV3, YJReflowableV10, YJReflowableARv1},
	"5.10.1.1":   []string{YJReflowableLangExpansionV1, YJReflowableTCNv1},
	"5.11.1.1":   []string{YJReflowableV12},
	"5.12.2":     []string{YJJPVV2TrackB},
	"5.13.2":     []string{YJCoverImageDeferV1},
	"5.13.4":     []string{YJJPVV1SimpleVertical, YJJPVV3TrackB, YJJPVV4TrackDPart1, YJJPVV5, YJJPVV6, YJJPVV7, YJJPVV8},
	"5.13.5":     []string{YJMixedWritingModeV1, YJRubyV1},
	"5.14.1":     []string{SupportsHDVV2},
	"5.14.3":     []string{YJMixedWritingModeV2, YJReflowableV13, YJReflowableV14, YJVerticalTextShadowV1},
	"5.14.3.2":   YJPDFBackedFixedLayoutV1Test, // single value, not a slice
	"5.16.6":     []string{YJAudioV3, YJConditionalStructureV1, YJPublisherPanelsV3, YJVideoV3},
	"5.18.1":     []string{YJPublisherPanelsV4, YJReflowableTablesv10, YJReflowableTablesv8, YJReflowableTablesv9, YJTextPopUpsV1},
	"5.18.2":     []string{YJMathmlV1},
	"5.18.5":     []string{YJReflowableFAV1, YJReflowableHEV1, YJReflowableLangExpansionV2},
}

// ---------------------------------------------------------------------------
// 1.13 KINDLE_CAPABILITY_VERSIONS (Python lines 1076-1079)
// Computed from KINDLE_VERSION_CAPABILITIES: maps feature name → version string
// ---------------------------------------------------------------------------

var KindleCapabilityVersions map[string]string

func init() {
	// Compute KINDLE_CAPABILITY_VERSIONS from KINDLE_VERSION_CAPABILITIES
	KindleCapabilityVersions = make(map[string]string)
	for version, capabilities := range KindleVersionCapabilities {
		switch caps := capabilities.(type) {
		case string:
			KindleCapabilityVersions[caps] = version
		case []string:
			for _, cap := range caps {
				KindleCapabilityVersions[cap] = version
			}
		}
	}

	// Populate KNOWN_KCB_DATA references to creator_version sets
	creatorVersions := KnownMetadata["kindle_audit_metadata"]["creator_version"]
	kcbCreatorVersions := kcbValueSet{Strings: creatorVersions.Strings}
	KnownKCBData["metadata"]["edited_tool_versions"] = kcbCreatorVersions
	KnownKCBData["metadata"]["tool_version"] = kcbCreatorVersions
}

// ---------------------------------------------------------------------------
// 1.14 Validation Functions (Python lines 1082-1124)
// ---------------------------------------------------------------------------

// IsKnownGenerator checks if a given KFX generator version is known.
// Port of Python is_known_generator (lines 1082-1093).
func IsKnownGenerator(kfxgenApplicationVersion, kfxgenPackageVersion string) bool {
	if kfxgenApplicationVersion == "" ||
		strings.HasPrefix(kfxgenApplicationVersion, "kfxlib") ||
		strings.HasPrefix(kfxgenApplicationVersion, "KC") ||
		strings.HasPrefix(kfxgenApplicationVersion, "KPR") {
		return true
	}

	if PackageVersionPlaceholders[kfxgenPackageVersion] {
		kfxgenPackageVersion = ""
	}

	return KnownKFXGenerators[GeneratorEntry{
		Version:        kfxgenApplicationVersion,
		PackageVersion: kfxgenPackageVersion,
	}]
}

// IsKnownFeature checks if a feature value is known for a given category and key.
// Port of Python is_known_feature (lines 1095-1098).
// val should be an int, [2]int (tuple), or string matching a VersionKey.
//
// Python's implementation is: return val in vals or ANY in vals
// Since Python's ANY=True and True==1 (bool is subclass of int), ANY in vals
// matches when integer key 1 is present OR when True is an explicit key.
// We replicate this by checking both AnyVersionKey() and IntVersionKey(1).
func IsKnownFeature(cat, key string, val interface{}) bool {
	vals := KnownFeatures[cat][key]
	if vals == nil {
		return false
	}
	vk := toVersionKey(val)
	if _, ok := vals[vk]; ok {
		return true
	}
	// Check for ANY sentinel (Python: ANY in vals)
	// Python's ANY=True equals 1, so check both explicit ANY key and int key 1
	_, hasAny := vals[AnyVersionKey()]
	if hasAny {
		return true
	}
	_, hasOne := vals[IntVersionKey(1)]
	return hasOne
}

// KindleFeatureVersion returns the Kindle firmware version that first supported
// the given feature, or UNSUPPORTED if not found.
// Port of Python kindle_feature_version (lines 1100-1105).
func KindleFeatureVersion(cat, key string, val interface{}) string {
	vals := KnownFeatures[cat][key]
	if vals == nil {
		return Unsupported
	}

	vk := toVersionKey(val)
	var feature string
	if f, ok := vals[vk]; ok {
		feature = f
	} else if f, ok := vals[AnyVersionKey()]; ok {
		feature = f
	} else {
		return Unsupported
	}

	if v, ok := KindleCapabilityVersions[feature]; ok {
		return v
	}
	return Unsupported
}

// IsKnownMetadata checks if a metadata value is known for a given category and key.
// Port of Python is_known_metadata (lines 1106-1115).
// val can be a string, int, float64, bool, or []interface{} (list).
func IsKnownMetadata(cat, key string, val interface{}) bool {
	// Handle list values recursively
	if list, ok := val.([]interface{}); ok {
		for _, v := range list {
			if !IsKnownMetadata(cat, key, v) {
				return false
			}
		}
		return true
	}

	vals, ok := KnownMetadata[cat][key]
	if !ok {
		return false
	}
	if vals.IsAny {
		return true
	}
	return metadataValueContains(vals, val)
}

// IsKnownAuxMetadata checks if an auxiliary metadata value is known.
// Port of Python is_known_aux_metadata (lines 1117-1120).
func IsKnownAuxMetadata(key string, val interface{}) bool {
	vals, ok := KnownAuxiliaryMetadata[key]
	if !ok {
		return false
	}
	if vals.IsAny {
		return true
	}
	return metadataValueContains(vals, val)
}

// IsKnownKCBData checks if a KCB data value is known for a given category and key.
// Port of Python is_known_kcb_data (lines 1122-1124).
func IsKnownKCBData(cat, key string, val interface{}) bool {
	vals, ok := KnownKCBData[cat][key]
	if !ok {
		return false
	}
	if vals.IsAny {
		return true
	}
	return kcbValueContains(vals, val)
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// toVersionKey converts a Go value to a VersionKey for lookup in KnownFeatures.
// In Python, True==1 (bool is a subclass of int), so we treat bool true as int 1.
func toVersionKey(val interface{}) VersionKey {
	switch v := val.(type) {
	case int:
		return IntVersionKey(v)
	case bool:
		// Python: True==1, False==0
		if v {
			return IntVersionKey(1)
		}
		return IntVersionKey(0)
	case [2]int:
		return TupleVersionKey(v[0], v[1])
	case string:
		// Handle string version numbers — but feature keys are always int/tuple/ANY
		return VersionKey{}
	default:
		return VersionKey{}
	}
}

// metadataValueContains checks if a value is in a metadataValueSet.
func metadataValueContains(vs metadataValueSet, val interface{}) bool {
	switch v := val.(type) {
	case string:
		return vs.Strings[v]
	case int:
		return vs.Ints[v]
	case float64:
		return vs.Floats[v]
	case bool:
		return vs.Bools[v]
	}
	return false
}

// kcbValueContains checks if a value is in a kcbValueSet.
func kcbValueContains(vs kcbValueSet, val interface{}) bool {
	switch v := val.(type) {
	case string:
		return vs.Strings[v]
	case int:
		return vs.Ints[v]
	case bool:
		return vs.Bools[v]
	}
	return false
}

// newStringSet creates a metadataValueSet from string values.
func newStringSet(vals ...string) metadataValueSet {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return metadataValueSet{Strings: m}
}

// newIntSet creates a metadataValueSet from int values.
func newIntSet(vals ...int) metadataValueSet {
	m := make(map[int]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return metadataValueSet{Ints: m}
}

// newFloatSet creates a metadataValueSet from float64 values.
func newFloatSet(vals ...float64) metadataValueSet {
	m := make(map[float64]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return metadataValueSet{Floats: m}
}

// newBoolSet creates a metadataValueSet from bool values.
func newBoolSet(vals ...bool) metadataValueSet {
	m := make(map[bool]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return metadataValueSet{Bools: m}
}

// newKCBIntSet creates a kcbValueSet from int values.
func newKCBIntSet(vals ...int) kcbValueSet {
	m := make(map[int]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return kcbValueSet{Ints: m}
}

// newKCBBoolSet creates a kcbValueSet from bool values.
func newKCBBoolSet(vals ...bool) kcbValueSet {
	m := make(map[bool]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return kcbValueSet{Bools: m}
}

// newKCBStringSet creates a kcbValueSet from string values.
func newKCBStringSet(vals ...string) kcbValueSet {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return kcbValueSet{Strings: m}
}
