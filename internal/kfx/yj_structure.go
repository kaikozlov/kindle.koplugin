package kfx

// Port of yj_structure.py fragment validation functions, constants, and helpers.
// Python reference: REFERENCE/Calibre_KFX_Input/kfxlib/yj_structure.py

import (
	"crypto/rand"
	"fmt"
	"log"
	"math"
	"sort"
)

// ---------------------------------------------------------------------------
// Module-level constants (yj_structure.py lines 18-28, 50-157)
// ---------------------------------------------------------------------------

// Control constants (yj_structure.py lines 18-28)
var (
	// ReportKnownProblems controls error vs warning for known issues.
	// Python: REPORT_KNOWN_PROBLEMS = None (yj_structure.py:18)
	ReportKnownProblems interface{} = nil

	// ReportNonJPEGJFIFCover enables JPEG cover format check.
	// Python: REPORT_NON_JPEG_JFIF_COVER = False (yj_structure.py:19)
	ReportNonJPEGJFIFCover = false

	// ReportJPEGVariants enables JPEG variant reporting.
	// Python: REPORT_JPEG_VARIANTS = False (yj_structure.py:20)
	ReportJPEGVariants = false

	// DebugPDFPageSize enables PDF page size debug.
	// Python: DEBUG_PDF_PAGE_SIZE = False (yj_structure.py:21)
	DebugPDFPageSize = false
)

const (
	// MaxContentFragmentSize is the max bytes for content fragments.
	// Python: MAX_CONTENT_FRAGMENT_SIZE = 8192 (yj_structure.py:23)
	MaxContentFragmentSize = 8192

	// ApproximatePageList is the page list constant.
	// Python: APPROXIMATE_PAGE_LIST = "APPROXIMATE_PAGE_LIST" (yj_structure.py:25)
	ApproximatePageList = "APPROXIMATE_PAGE_LIST"

	// KFXCoverResource is the cover resource prefix.
	// Python: KFX_COVER_RESOURCE = "kfx_cover_image" (yj_structure.py:26)
	KFXCoverResource = "kfx_cover_image"

	// DictionaryRulesSymbol is the dictionary rules symbol.
	// Python: DICTIONARY_RULES_SYMBOL = "dictionary_rules" (yj_structure.py:28)
	DictionaryRulesSymbol = "dictionary_rules"
)

// FragmentIDKeys maps fragment types to the key paths used to extract fragment IDs.
// Python: FRAGMENT_ID_KEYS (yj_structure.py:57-72)
var FragmentIDKeys = map[string][]string{
	"$266": {"$180"},
	"$597": {"$174", "$598"},
	"$418": {"$165"},
	"$417": {"$165"},
	"$394": {"$240"},
	"$145": {"name"},
	"$164": {"$175"},
	"$391": {"$239"},
	"$692": {"name"},
	"$387": {"$174"},
	"$756": {"$757"},
	"$260": {"$174"},
	"$267": {"$174"},
	"$609": {"$174"},
	"$259": {"$176"},
	"$608": {"$598"},
	"$157": {"$173"},
	"$610": {"$602"},
}

// CommonFragmentReferences maps container symbols to their fragment reference types.
// Python: COMMON_FRAGMENT_REFERENCES (yj_structure.py:75-99)
var CommonFragmentReferences = map[string]string{
	"$749": "$259",
	"$266": "$266",
	"$597": "$597",
	"$429": "$157",
	"$479": "$164",
	"$145": "$145",
	"$146": "$608",
	"$245": "$164",
	"$179": "$266",
	"$165": "$417",
	"$392": "$391",
	"name":  "$145",
	"$167": "$164",
	"$175": "$164",
	"$757": "$756",
	"$174": "$260",
	"$170": "$260",
	"$176": "$259",
	"$157": "$157",
	"$173": "$157",
	"$528": "$164",
	"$214": "$164",
	"$636": "$417",
	"$635": "$164",
}

// NestedFragmentReferences maps (parent, container) pairs to fragment reference types.
// Python: NESTED_FRAGMENT_REFERENCES (yj_structure.py:102-107)
var NestedFragmentReferences = map[[2]string]string{
	{"$597", "$351"}: "$597",
	{"$597", "$538"}: "$597",
	{"$597", "$613"}: "$597",
	{"$597", "$614"}: "$597",
}

// SpecialFragmentReferences maps fragment types to their type-specific container→frag_ref mappings.
// Python: SPECIAL_FRAGMENT_REFERENCES (yj_structure.py:110-118)
var SpecialFragmentReferences = map[string]map[string]string{
	"$391": {
		"$247": "$394",
	},
	"$387": {
		"$213": "$164",
		"$214": "$164",
		"$212": "$164",
	},
}

// SpecialParentFragmentReferences maps fragment types to container_parent→bool mappings.
// Python: SPECIAL_PARENT_FRAGMENT_REFERENCES (yj_structure.py:121-124)
var SpecialParentFragmentReferences = map[string]map[string]bool{
	"$538": {
		"yj.print.style": false,
	},
}

// SectionDataTypes lists the section data fragment types.
// Python: SECTION_DATA_TYPES (yj_structure.py:127-132)
var SectionDataTypes = map[string]bool{
	"$387": true,
	"$260": true,
	"$267": true,
	"$609": true,
}

// ExpectedAnnotations lists the expected IonAnnotation triplets.
// Python: EXPECTED_ANNOTATIONS (yj_structure.py:135-142)
var ExpectedAnnotations = map[[3]string]bool{
	{"$164", "$214", "$164"}: true,
	{"$389", "$247", "$393"}: true,
	{"$389", "$392", "$391"}: true,
	{"$259", "$429", "$157"}: true,
	{"$259", "$173", "$157"}: true,
}

// ExpectedDictionaryAnnotations lists the dictionary-specific annotations.
// Python: EXPECTED_DICTIONARY_ANNOTATIONS (yj_structure.py:145-148)
var ExpectedDictionaryAnnotations = map[[3]string]bool{
	{"$260", "$141", "$608"}: true,
	{"$259", "$146", "$608"}: true,
}

// EIDReferences lists the EID reference container symbols.
// Python: EID_REFERENCES (yj_structure.py:151-157)
var EIDReferences = map[string]bool{
	"$185": true,
	"$155": true,
	"$598": true,
	"$754": true,
	"$474": true,
	"$163": true,
}

// FixedLayoutImageFormats lists the fixed layout image format symbols.
// Python: FIXED_LAYOUT_IMAGE_FORMATS (yj_structure.py:54)
var FixedLayoutImageFormats = map[string]bool{
	"$286": true,
	"$285": true,
	"$548": true,
	"$565": true,
	"$284": true,
}

// ---------------------------------------------------------------------------
// Ion type constants used in walk_fragment dispatch
// ---------------------------------------------------------------------------

// ionType represents the kind of Ion value, matching Python's ion_type() dispatch.
type ionType int

const (
	ionTypeStruct     ionType = iota // IonStruct → map[string]interface{}
	ionTypeList                      // IonList → []interface{}
	ionTypeSExp                      // IonSExp → []interface{} (but tagged differently)
	ionTypeSymbol                    // IonSymbol → string
	ionTypeString                    // IonString → string
	ionTypeInt                       // IonInt → int
	ionTypeAnnotation                // IonAnnotation → annotated wrapper
)

// ionAnnotationData represents an Ion annotation wrapping a value.
// Python: IonAnnotation has .annotations and .value
type ionAnnotationData struct {
	Annotations []string
	Value       interface{}
}

// ---------------------------------------------------------------------------
// numstr helper (yj_structure.py line 1313)
// ---------------------------------------------------------------------------

// Numstr formats a number using Python's %g format.
// Python: `def numstr(x): return "%g" % x` (yj_structure.py:1313)
func Numstr(x float64) string {
	// Python's %g removes trailing zeros and uses scientific notation for large/small numbers.
	// Go's %g behaves the same way.
	return fmt.Sprintf("%g", x)
}

// ---------------------------------------------------------------------------
// logErrorOnce — C3-10: Error deduplication (yj_structure.py:1306-1310)
// ---------------------------------------------------------------------------

// reportedErrors tracks which error messages have already been logged,
// so that logErrorOnce can suppress duplicates.
// Python: self.reported_errors = set() on BookStructure
var reportedErrors = map[string]bool{}

// LogErrorOnce logs an error message only once per message string.
// Python: BookStructure.log_error_once (yj_structure.py:1306-1310)
//
//	def log_error_once(self, msg):
//	    if msg not in self.reported_errors:
//	        log.error(msg)
//	        self.reported_errors.add(msg)
func LogErrorOnce(msg string) {
	if !reportedErrors[msg] {
		log.Printf("kfx: error: %s", msg)
		reportedErrors[msg] = true
	}
}

// ResetReportedErrors clears the error deduplication set.
// Call between books to reset state.
func ResetReportedErrors() {
	reportedErrors = map[string]bool{}
}

// ---------------------------------------------------------------------------
// extractFragmentIDFromValue (yj_structure.py:703-716)
// ---------------------------------------------------------------------------

// ExtractFragmentIDFromValue extracts a fragment ID from a value struct based on the fragment type.
// Python: BookStructure.extract_fragment_id_from_value (yj_structure.py:703-716)
func ExtractFragmentIDFromValue(ftype string, value interface{}) string {
	valMap, ok := asMap(value)
	if !ok {
		return ftype
	}
	idKeys, hasKeys := FragmentIDKeys[ftype]
	if !hasKeys {
		return ftype
	}
	for _, idKey := range idKeys {
		if idVal, exists := valMap[idKey]; exists {
			fid, ok := asString(idVal)
			if ok {
				return fid
			}
		}
	}
	return ftype
}

// ---------------------------------------------------------------------------
// walkFragment (yj_structure.py:857-950)
// ---------------------------------------------------------------------------

// WalkFragment recursively walks fragment data to discover all fragment references and EIDs.
// Python: BookStructure.walk_fragment (yj_structure.py:857-950)
func WalkFragment(
	fragment Fragment,
	mandatoryFragRefs *map[FragmentKey]bool,
	optionalFragRefs *map[FragmentKey]bool,
	eidDefs *map[interface{}]bool,
	eidRefs *map[interface{}]bool,
	fragmentList FragmentList,
) {
	walkInternal(
		fragment.Value, nil, nil, true,
		fragment, mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList,
	)
}

func walkInternal(
	data interface{},
	container *string,
	containerParent *string,
	topLevel bool,
	fragment Fragment,
	mandatoryFragRefs *map[FragmentKey]bool,
	optionalFragRefs *map[FragmentKey]bool,
	eidDefs *map[interface{}]bool,
	eidRefs *map[interface{}]bool,
	fragmentList FragmentList,
) {
	// Default container to fragment type
	cont := ""
	if container != nil {
		cont = *container
	} else {
		cont = fragment.FType
	}

	cParent := ""
	if containerParent != nil {
		cParent = *containerParent
	}

	// Dispatch on Ion type
	dataType := detectIonType(data)

	switch dataType {
	case ionTypeAnnotation:
		ann, ok := data.(ionAnnotationData)
		if !ok {
			return
		}
		if !topLevel {
			if len(ann.Annotations) != 1 {
				LogErrorOnce(fmt.Sprintf("Found multiple annotations in %s of %s fragment", cont, fragment.FType))
			}
			for _, annot := range ann.Annotations {
				if !ExpectedAnnotations[[3]string{fragment.FType, cont, annot}] &&
					!ExpectedDictionaryAnnotations[[3]string{fragment.FType, cont, annot}] {
					LogErrorOnce(fmt.Sprintf("Found unexpected IonAnnotation %s in %s of %s fragment", annot, cont, fragment.FType))
				}
			}
		}
		walkInternal(ann.Value, container, containerParent, false,
			fragment, mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList)

	case ionTypeList:
		slice, ok := asSlice(data)
		if !ok {
			return
		}
		for _, fc := range slice {
			walkInternal(fc, &cont, &cParent, false,
				fragment, mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList)
		}

	case ionTypeStruct:
		m, ok := asMap(data)
		if !ok {
			return
		}
		for fk, fv := range m {
			walkInternal(fv, &fk, &cont, false,
				fragment, mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList)
		}

	case ionTypeSExp:
		slice, ok := asSlice(data)
		if !ok || len(slice) == 0 {
			return
		}
		first := ""
		if s, ok := asString(slice[0]); ok {
			first = s
		}
		for _, fc := range slice[1:] {
			walkInternal(fc, &first, &cont, false,
				fragment, mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList)
		}

	case ionTypeString:
		// In Python: if container in ["$165", "$636"]: walk(IS(data), container, container_parent)
		// This converts the string to a symbol and processes it as such.
		if cont == "$165" || cont == "$636" {
			processSymbolReference(
				fmt.Sprintf("%v", data), cont, cParent, fragment,
				mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList,
			)
		}

	case ionTypeSymbol:
		sym, _ := asString(data)
		processSymbolReference(
			sym, cont, cParent, fragment,
			mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList,
		)
	}

	// Handle EID defs/refs for int and symbol types (Python: data_type is IonInt or IonSymbol)
	if dataType == ionTypeInt || dataType == ionTypeSymbol {
		sym, _ := asString(data)
		if (cont == "$155" || cont == "$598") && fragment.FType != "$550" &&
			fragment.FType != "$265" && fragment.FType != "$264" &&
			fragment.FType != "$609" && fragment.FType != "$610" &&
			fragment.FType != "$621" && fragment.FType != "$611" {
			(*eidDefs)[data] = true
			if sym != "" {
				(*eidDefs)[sym] = true
			}
		} else if EIDReferences[cont] {
			// Skip IonInt 0 in $265 fragments (Python: not (data_type is IonInt and data == 0 and fragment.ftype == "$265"))
			if dataType == ionTypeInt {
				if intVal, ok := asInt(data); ok && intVal == 0 && fragment.FType == "$265" {
					// skip
				} else {
					(*eidRefs)[data] = true
					if sym != "" {
						(*eidRefs)[sym] = true
					}
				}
			} else {
				(*eidRefs)[data] = true
				if sym != "" {
					(*eidRefs)[sym] = true
				}
			}
		}
	}
}

// processSymbolReference handles IonSymbol reference resolution during fragment walking.
// This is the core of the walk_fragment symbol dispatch logic (yj_structure.py:918-948).
func processSymbolReference(
	data string,
	container string,
	containerParent string,
	fragment Fragment,
	mandatoryFragRefs *map[FragmentKey]bool,
	optionalFragRefs *map[FragmentKey]bool,
	eidDefs *map[interface{}]bool,
	eidRefs *map[interface{}]bool,
	fragmentList FragmentList,
) {
	// Track EID definitions (container == "$155")
	if container == "$155" {
		(*eidDefs)[data] = true
	}

	// Check if this is a fragment ID key within its own parent (skip in that case)
	idKeys := FragmentIDKeys[fragment.FType]
	isIDKey := false
	if containerParent == fragment.FType {
		for _, key := range idKeys {
			if key == container {
				isIDKey = true
				break
			}
		}
	}

	if isIDKey {
		return
	}

	// Look up fragment reference through priority chain:
	// SPECIAL_FRAGMENT_REFERENCES → SPECIAL_PARENT_FRAGMENT_REFERENCES →
	// NESTED_FRAGMENT_REFERENCES → COMMON_FRAGMENT_REFERENCES
	var fragRef string
	fragRefFound := false

	// 1. Check SPECIAL_FRAGMENT_REFERENCES
	if specialRefs, ok := SpecialFragmentReferences[fragment.FType]; ok {
		if ref, exists := specialRefs[container]; exists {
			fragRef = ref
			fragRefFound = true
		}
	}

	// 2. Check SPECIAL_PARENT_FRAGMENT_REFERENCES
	if !fragRefFound {
		if specialRefs, ok := SpecialParentFragmentReferences[fragment.FType]; ok {
			if skip, exists := specialRefs[containerParent]; exists {
				// In Python, this maps to False, meaning "skip this reference"
				if !skip {
					return
				}
			}
		}
	}

	// 3. Check NESTED_FRAGMENT_REFERENCES
	if !fragRefFound {
		if ref, exists := NestedFragmentReferences[[2]string{containerParent, container}]; exists {
			fragRef = ref
			fragRefFound = true
		}
	}

	// 4. Check COMMON_FRAGMENT_REFERENCES
	if !fragRefFound {
		if ref, exists := CommonFragmentReferences[container]; exists {
			fragRef = ref
			fragRefFound = true
		}
	}

	if !fragRefFound {
		return
	}

	// Handle the reference
	if container == "name" && (containerParent == "$249" || containerParent == "$692") {
		(*mandatoryFragRefs)[FragmentKey{FType: "$692", FID: data}] = true
	} else if container == "$165" && fragmentList != nil {
		if frag := fragmentList.Get("$418", data, true); frag != nil {
			(*mandatoryFragRefs)[FragmentKey{FType: "$418", FID: data}] = true
		} else {
			// Not found as $418, add as the resolved fragRef type
			(*mandatoryFragRefs)[FragmentKey{FType: fragRef, FID: data}] = true
		}
	} else if container == "$635" {
		(*optionalFragRefs)[FragmentKey{FType: fragRef, FID: data}] = true
	} else {
		(*mandatoryFragRefs)[FragmentKey{FType: fragRef, FID: data}] = true
	}

	// For $260 references, also check related variant fragments
	if fragRef == "$260" {
		variantKeys := []FragmentKey{
			{FType: "$609", FID: data},
			{FType: "$609", FID: data + "-spm"},
			{FType: "$597", FID: data + "-ad"},
			{FType: "$597", FID: data},
			{FType: "$267", FID: data},
			{FType: "$387", FID: data},
		}
		for _, refKey := range variantKeys {
			if fragmentList != nil && fragmentList.Get(refKey.FType, refKey.FID, true) != nil {
				(*mandatoryFragRefs)[refKey] = true
			}
		}
	}
}

// detectIonType determines the Ion type of a Go value.
func detectIonType(data interface{}) ionType {
	if data == nil {
		return ionTypeInt
	}
	switch data.(type) {
	case ionAnnotationData:
		return ionTypeAnnotation
	case map[string]interface{}:
		return ionTypeStruct
	case []interface{}:
		// Could be list or sexp; Go doesn't distinguish.
		// Default to list. Callers that need sexp behavior should wrap.
		return ionTypeList
	case string:
		return ionTypeSymbol
	case int, int32, int64, uint32, uint64, float64:
		return ionTypeInt
	case bool:
		return ionTypeInt
	}
	return ionTypeInt
}

// ---------------------------------------------------------------------------
// IonDataEq (ion.py:304-370)
// ---------------------------------------------------------------------------

// IonDataEq recursively compares two Ion values for structural equality.
// Port of Python ion_data_eq (ion.py:304-370).
// Returns true if the values are structurally identical.
func IonDataEq(f1, f2 interface{}) bool {
	return ionDataEqInternal(f1, f2)
}

// ionDataEqInternal is the recursive comparison engine.
// Port of Python ion_data_eq_ (ion.py:311-360).
func ionDataEqInternal(f1, f2 interface{}) bool {
	// Determine Ion types
	t1 := detectIonType(f1)
	t2 := detectIonType(f2)

	// Type mismatch
	if t2 != t1 {
		return false
	}

	switch t1 {
	case ionTypeAnnotation:
		ann1, ok1 := f1.(ionAnnotationData)
		ann2, ok2 := f2.(ionAnnotationData)
		if !ok1 || !ok2 {
			return false
		}
		// Compare annotations as lists
		if !ionDataEqInternal(toInterfaceSlice(ann1.Annotations), toInterfaceSlice(ann2.Annotations)) {
			return false
		}
		// Compare values
		return ionDataEqInternal(ann1.Value, ann2.Value)

	case ionTypeList, ionTypeSExp:
		s1, ok1 := asSlice(f1)
		s2, ok2 := asSlice(f2)
		if !ok1 || !ok2 {
			return false
		}
		if len(s1) != len(s2) {
			return false
		}
		for i := range s1 {
			if !ionDataEqInternal(s1[i], s2[i]) {
				return false
			}
		}
		return true

	case ionTypeStruct:
		m1, ok1 := asMap(f1)
		m2, ok2 := asMap(f2)
		if !ok1 || !ok2 {
			return false
		}
		if len(m1) != len(m2) {
			return false
		}
		for k, v1 := range m1 {
			v2, exists := m2[k]
			if !exists {
				return false
			}
			if !ionDataEqInternal(v1, v2) {
				return false
			}
		}
		return true

	case ionTypeSymbol, ionTypeString:
		s1, ok1 := asString(f1)
		s2, ok2 := asString(f2)
		if !ok1 || !ok2 {
			return false
		}
		return s1 == s2

	case ionTypeInt:
		// Handle float NaN comparison (Python: math.isnan check)
		f1Float, f1IsFloat := asFloat64(f1)
		f2Float, f2IsFloat := asFloat64(f2)
		if f1IsFloat && f2IsFloat && math.IsNaN(f1Float) && math.IsNaN(f2Float) {
			return true
		}
		// Standard comparison
		return fmt.Sprintf("%v", f1) == fmt.Sprintf("%v", f2)
	}

	return fmt.Sprintf("%v", f1) == fmt.Sprintf("%v", f2)
}

// toInterfaceSlice converts a string slice to []interface{}.
func toInterfaceSlice(ss []string) []interface{} {
	result := make([]interface{}, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}

// ---------------------------------------------------------------------------
// FragmentValidationOptions (yj_structure.py:718-852)
// ---------------------------------------------------------------------------

// FragmentValidationOptions holds options for CheckFragmentUsageWithOptions.
// These correspond to the Python BookStructure fields is_kpf_prepub,
// is_sample, is_dictionary, and is_scribe_notebook.
type FragmentValidationOptions struct {
	IsKpfPrepub     bool
	IsSample        bool
	IsDictionary    bool
	IsScribeNotebook bool
	IgnoreExtra     bool
}

// CheckFragmentUsageResult holds the results of check_fragment_usage.
type CheckFragmentUsageResult struct {
	Referenced        FragmentList
	Unreferenced      FragmentList
	Missing           map[FragmentKey]bool
	DiffDupeFragments bool
}

// CheckFragmentUsageWithOptions performs a BFS walk of the fragment reference graph
// with the given options for conditional behavior.
// Python: BookStructure.check_fragment_usage (yj_structure.py:718-852).
func CheckFragmentUsageWithOptions(fragments FragmentList, getCoverFID func() string, opts FragmentValidationOptions) CheckFragmentUsageResult {
	discovered := map[FragmentKey]bool{}

	unreferencedFragmentTypes := make(map[string]bool)
	for k := range RootFragmentTypes {
		if k != "$419" {
			unreferencedFragmentTypes[k] = true
		}
	}

	// C3-1: is_kpf_prepub adds $610 to unreferenced_fragment_types (yj_structure.py:721-724)
	if opts.IsKpfPrepub {
		unreferencedFragmentTypes["$610"] = true
	}

	// Seed with root fragment types
	for _, frag := range fragments {
		if unreferencedFragmentTypes[frag.FType] {
			discovered[FragmentKey{FType: frag.FType, FID: frag.FID}] = true
		}
		if !KnownFragmentTypes[frag.FType] {
			discovered[FragmentKey{FType: frag.FType, FID: frag.FID}] = true
		}
	}

	// Seed with cover_image metadata fragment
	if getCoverFID != nil {
		coverFID := getCoverFID()
		if coverFID != "" {
			discovered[FragmentKey{FType: "$164", FID: coverFID}] = true
		}
	}

	visited := map[FragmentKey]bool{}
	mandatoryReferences := map[FragmentKey]map[FragmentKey]bool{}
	optionalReferences := map[FragmentKey]map[FragmentKey]bool{}
	missing := map[FragmentKey]bool{}
	eidDefs := map[interface{}]bool{}
	eidRefs := map[interface{}]bool{}

	// Mark container types as already visited
	for ftype := range ContainerFragmentTypes {
		visited[FragmentKey{FType: ftype}] = true
		visited[FragmentKey{FType: ftype, FID: ftype}] = true
	}

	for len(discovered) > 0 {
		nextVisits := map[FragmentKey]bool{}
		for k := range discovered {
			if !visited[k] {
				nextVisits[k] = true
			}
		}
		discovered = map[FragmentKey]bool{}

		for i := range fragments {
			frag := fragments[i]
			fragKey := FragmentKey{FType: frag.FType, FID: frag.FID}
			if !nextVisits[fragKey] {
				continue
			}

			mandatoryRefs := map[FragmentKey]bool{}
			optionalRefs := map[FragmentKey]bool{}

			WalkFragmentWithOptions(frag, &mandatoryRefs, &optionalRefs, &eidDefs, &eidRefs, fragments, opts)

			visited[fragKey] = true
			mandatoryReferences[fragKey] = mandatoryRefs
			optionalReferences[fragKey] = optionalRefs

			for k := range mandatoryRefs {
				discovered[k] = true
			}
			for k := range optionalRefs {
				discovered[k] = true
			}
		}

		for k := range nextVisits {
			if !visited[k] {
				missing[k] = true
			}
		}
	}

	// Report missing fragments
	for key := range missing {
		if key.FType == "$597" {
			log.Printf("kfx: warning: Referenced fragment is missing from book: %s", key)
		} else {
			log.Printf("kfx: error: Referenced fragment is missing from book: %s", key)
		}
	}

	// Separate referenced vs unreferenced fragments
	// C3-5: ion_data_eq duplicate detection (yj_structure.py:783-791)
	var referencedFragments FragmentList
	var unreferencedFragments FragmentList
	alreadyProcessed := map[FragmentKey]Fragment{}
	diffDupeFragments := false

	for _, frag := range fragments {
		if frag.FType != "$262" && frag.FType != "$387" {
			fragKey := FragmentKey{FType: frag.FType, FID: frag.FID}
			if existing, exists := alreadyProcessed[fragKey]; exists {
				if frag.FType == "$270" || frag.FType == "$593" {
					continue
				}

				if IonDataEq(frag.Value, existing.Value) {
					// Identical duplicates
					if frag.FType == "$597" {
						// Python: self.log_known_error("Duplicate fragment: %s" % str(fragment))
						log.Printf("kfx: warning: Duplicate fragment (known error): %s", fragKey)
					} else {
						log.Printf("kfx: warning: Duplicate fragment: %s", fragKey)
					}
					continue
				} else {
					// Content-differing duplicate — fatal
					log.Printf("kfx: error: Duplicate fragment key with different content: %s", fragKey)
					diffDupeFragments = true
				}
			} else {
				alreadyProcessed[fragKey] = frag
			}
		}

		fragKey := FragmentKey{FType: frag.FType, FID: frag.FID}
		if visited[fragKey] {
			referencedFragments = append(referencedFragments, frag)
		} else if ContainerFragmentTypes[frag.FType] || frag.FID == frag.FType {
			log.Printf("kfx: error: Unexpected root fragment: %s", fragKey)
		} else if frag.FType == "$597" && (opts.IsSample || opts.IsDictionary) {
			// C3-3: is_sample/is_dictionary silently accepts unreferenced $597 (yj_structure.py:795)
		} else if !opts.IgnoreExtra {
			unreferencedFragments = append(unreferencedFragments, frag)
		}
	}

	// C3-4: is_kpf_prepub cleanup (yj_structure.py:807-809)
	if opts.IsKpfPrepub {
		filtered := make(FragmentList, 0, len(unreferencedFragments))
		prepubTypes := map[string]bool{
			"$391": true, "$266": true, "$259": true, "$260": true, "$608": true,
		}
		for _, f := range unreferencedFragments {
			if !prepubTypes[f.FType] {
				filtered = append(filtered, f)
			}
		}
		unreferencedFragments = filtered
	}

	if len(unreferencedFragments) > 0 {
		names := make([]string, len(unreferencedFragments))
		for i, f := range unreferencedFragments {
			names[i] = fmt.Sprintf("%s", FragmentKey{FType: f.FType, FID: f.FID})
		}
		log.Printf("kfx: error: Unreferenced fragments: %s", names)
	}

	if diffDupeFragments {
		log.Printf("kfx: fatal: Book appears to have KFX containers from multiple books. (duplicate fragments)")
	}

	undefinedEIDs := map[interface{}]bool{}
	for ref := range eidRefs {
		if !eidDefs[ref] {
			undefinedEIDs[ref] = true
		}
	}
	if len(undefinedEIDs) > 0 {
		names := make([]string, 0, len(undefinedEIDs))
		for ref := range undefinedEIDs {
			names = append(names, fmt.Sprintf("%v", ref))
		}
		sort.Strings(names)
		log.Printf("kfx: error: Undefined EIDs: %s", names)
	}

	return CheckFragmentUsageResult{
		Referenced:        referencedFragments,
		Unreferenced:      unreferencedFragments,
		Missing:           missing,
		DiffDupeFragments: diffDupeFragments,
	}
}

// WalkFragmentWithOptions is like WalkFragment but respects dictionary annotation checks.
// C3-9: EXPECTED_DICTIONARY_ANNOTATIONS only checked for dictionaries (yj_structure.py:871-872).
func WalkFragmentWithOptions(
	fragment Fragment,
	mandatoryFragRefs *map[FragmentKey]bool,
	optionalFragRefs *map[FragmentKey]bool,
	eidDefs *map[interface{}]bool,
	eidRefs *map[interface{}]bool,
	fragmentList FragmentList,
	opts FragmentValidationOptions,
) {
	walkInternalOpts(
		fragment.Value, nil, nil, true,
		fragment, mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList, opts,
	)
}

func walkInternalOpts(
	data interface{},
	container *string,
	containerParent *string,
	topLevel bool,
	fragment Fragment,
	mandatoryFragRefs *map[FragmentKey]bool,
	optionalFragRefs *map[FragmentKey]bool,
	eidDefs *map[interface{}]bool,
	eidRefs *map[interface{}]bool,
	fragmentList FragmentList,
	opts FragmentValidationOptions,
) {
	cont := ""
	if container != nil {
		cont = *container
	} else {
		cont = fragment.FType
	}

	cParent := ""
	if containerParent != nil {
		cParent = *containerParent
	}

	dataType := detectIonType(data)

	switch dataType {
	case ionTypeAnnotation:
		ann, ok := data.(ionAnnotationData)
		if !ok {
			return
		}
		if !topLevel {
			if len(ann.Annotations) != 1 {
				LogErrorOnce(fmt.Sprintf("Found multiple annotations in %s of %s fragment", cont, fragment.FType))
			}
			for _, annot := range ann.Annotations {
				// C3-9: EXPECTED_DICTIONARY_ANNOTATIONS only checked for dictionaries
				if !ExpectedAnnotations[[3]string{fragment.FType, cont, annot}] &&
					!(opts.IsDictionary && ExpectedDictionaryAnnotations[[3]string{fragment.FType, cont, annot}]) {
					LogErrorOnce(fmt.Sprintf("Found unexpected IonAnnotation %s in %s of %s fragment", annot, cont, fragment.FType))
				}
			}
		}
		walkInternalOpts(ann.Value, container, containerParent, false,
			fragment, mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList, opts)

	case ionTypeList:
		slice, ok := asSlice(data)
		if !ok {
			return
		}
		for _, fc := range slice {
			walkInternalOpts(fc, &cont, &cParent, false,
				fragment, mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList, opts)
		}

	case ionTypeStruct:
		m, ok := asMap(data)
		if !ok {
			return
		}
		for fk, fv := range m {
			walkInternalOpts(fv, &fk, &cont, false,
				fragment, mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList, opts)
		}

	case ionTypeSExp:
		slice, ok := asSlice(data)
		if !ok || len(slice) == 0 {
			return
		}
		first := ""
		if s, ok := asString(slice[0]); ok {
			first = s
		}
		for _, fc := range slice[1:] {
			walkInternalOpts(fc, &first, &cont, false,
				fragment, mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList, opts)
		}

	case ionTypeString:
		if cont == "$165" || cont == "$636" {
			processSymbolReference(
				fmt.Sprintf("%v", data), cont, cParent, fragment,
				mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList,
			)
		}

	case ionTypeSymbol:
		sym, _ := asString(data)
		processSymbolReference(
			sym, cont, cParent, fragment,
			mandatoryFragRefs, optionalFragRefs, eidDefs, eidRefs, fragmentList,
		)
	}

	// Handle EID defs/refs
	if dataType == ionTypeInt || dataType == ionTypeSymbol {
		sym, _ := asString(data)
		if (cont == "$155" || cont == "$598") && fragment.FType != "$550" &&
			fragment.FType != "$265" && fragment.FType != "$264" &&
			fragment.FType != "$609" && fragment.FType != "$610" &&
			fragment.FType != "$621" && fragment.FType != "$611" {
			(*eidDefs)[data] = true
			if sym != "" {
				(*eidDefs)[sym] = true
			}
		} else if EIDReferences[cont] {
			if dataType == ionTypeInt {
				if intVal, ok := asInt(data); ok && intVal == 0 && fragment.FType == "$265" {
					// skip
				} else {
					(*eidRefs)[data] = true
					if sym != "" {
						(*eidRefs)[sym] = true
					}
				}
			} else {
				(*eidRefs)[data] = true
				if sym != "" {
					(*eidRefs)[sym] = true
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// RebuildFragments (yj_structure.py:816-852)
// ---------------------------------------------------------------------------

// RebuildFragments performs the rebuild phase of check_fragment_usage.
// C3-7: is_dictionary/is_scribe_notebook skips $270 container regeneration.
// Python: BookStructure.check_fragment_usage rebuild block (yj_structure.py:816-852).
func RebuildFragments(fragments FragmentList, getAssetID func() string, isDictionary, isScribeNotebook bool) FragmentList {
	// C3-7: Skip rebuild container processing for dictionary/scribe notebooks
	if isDictionary || isScribeNotebook {
		// For dictionary/scribe, just sort referenced fragments
		return FragmentList(sortedByKey(fragments))
	}

	// Normal rebuild: regenerate $270 container
	containerIDs := map[string]bool{}
	var kfxgenApplicationVersion, kfxgenPackageVersion, version *string

	for _, frag := range fragments.GetAll("$270") {
		valMap, ok := asMap(frag.Value)
		if !ok {
			continue
		}
		if containerID, ok := asString(valMap["$409"]); ok && containerID != "" {
			containerIDs[containerID] = true
		}
		if v, ok := asString(valMap["$587"]); ok && v != "" {
			kfxgenApplicationVersion = &v
		}
		if v, ok := asString(valMap["$588"]); ok {
			kfxgenPackageVersion = &v
		}
		if v, ok := asString(valMap["version"]); ok && v != "" {
			version = &v
		}
	}

	var containerID string
	if len(containerIDs) == 1 {
		for id := range containerIDs {
			containerID = id
		}
	} else {
		if getAssetID != nil {
			containerID = getAssetID()
		}
		if containerID == "" {
			containerID = CreateContainerID()
		}
	}

	// Build new $270 container
	appVersion := "kfxlib-unknown"
	if kfxgenApplicationVersion != nil {
		appVersion = *kfxgenApplicationVersion
	}
	pkgVersion := ""
	if kfxgenPackageVersion != nil {
		pkgVersion = *kfxgenPackageVersion
	}
	versionStr := "0"
	if version != nil {
		versionStr = *version
	}

	newContainer := Fragment{
		FType: "$270",
		Value: map[string]interface{}{
			"$409":    containerID,
			"$161":    "KFX main",
			"$587":    appVersion,
			"$588":    pkgVersion,
			"version": versionStr,
		},
	}

	// Remove old $270 fragments and add new one
	var result FragmentList
	for _, frag := range fragments {
		if frag.FType == "$270" {
			continue
		}
		result = append(result, frag)
	}
	result = append(result, newContainer)

	return FragmentList(sortedByKey(result))
}

// CreateContainerID generates a random container ID using crypto/rand.
// Python: BookStructure.create_container_id (yj_structure.py:854-855).
// Python uses random.choice() which is non-deterministic — crypto/rand ensures uniqueness.
func CreateContainerID() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, 28)
	randBytes := make([]byte, 28)
	// crypto/rand.Read always fills the buffer and returns nil error on supported platforms
	_, _ = rand.Read(randBytes)
	for i := range result {
		result[i] = chars[int(randBytes[i])%len(chars)]
	}
	return "CR!" + string(result)
}

// sortedByKey returns fragments sorted by (FType, FID).
func sortedByKey(fl FragmentList) FragmentList {
	sorted := make(FragmentList, len(fl))
	copy(sorted, fl)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].FType != sorted[j].FType {
			return sorted[i].FType < sorted[j].FType
		}
		return sorted[i].FID < sorted[j].FID
	})
	return sorted
}

// CheckFragmentUsage is a backward-compatible wrapper for CheckFragmentUsageWithOptions.
// It calls CheckFragmentUsageWithOptions with default (zero) options.
func CheckFragmentUsage(fragments FragmentList, getCoverFID func() string) CheckFragmentUsageResult {
	return CheckFragmentUsageWithOptions(fragments, getCoverFID, FragmentValidationOptions{})
}

// ---------------------------------------------------------------------------
// determineEntityDependencies (yj_structure.py:952-1004)
// ---------------------------------------------------------------------------

// EntityDependency represents a single entity's dependencies.
type EntityDependency struct {
	EntityID                string
	MandatoryDependencies  []string
	OptionalDependencies   []string
}

// DetermineEntityDependencies computes deep/transitive dependencies between fragments.
// Python: BookStructure.determine_entity_dependencies (yj_structure.py:952-1004)
func DetermineEntityDependencies(
	mandatoryReferences map[FragmentKey]map[FragmentKey]bool,
	optionalReferences map[FragmentKey]map[FragmentKey]bool,
) []EntityDependency {
	// Step 1: Skip $387 (section) mandatory references
	for fragKey, refs := range mandatoryReferences {
		if fragKey.FType == "$387" {
			mandatoryReferences[fragKey] = map[FragmentKey]bool{}
			_ = refs // clear
		}
	}

	// Step 2: Transitively expand mandatory references
	deepReferences := map[FragmentKey]map[FragmentKey]bool{}

	for fragKey, refs := range mandatoryReferences {
		oldRefs := map[FragmentKey]bool{}
		newRefs := map[FragmentKey]bool{}
		for k := range refs {
			newRefs[k] = true
		}

		// Skip $164→$164 cross-references
		if fragKey.FType == "$164" {
			for nFrag := range newRefs {
				if nFrag.FType == "$164" {
					delete(newRefs, nFrag)
				}
			}
		}

		for len(setDiff(newRefs, oldRefs)) > 0 {
			oldRefs = union(oldRefs, newRefs)
			newRefs = map[FragmentKey]bool{}
			for k := range oldRefs {
				newRefs[k] = true
			}
			for ref := range oldRefs {
				if refRefs, ok := mandatoryReferences[ref]; ok {
					for k := range refRefs {
						newRefs[k] = true
					}
				}
			}
		}

		deepReferences[fragKey] = newRefs
	}

	// Step 3: Build entity dependencies
	var entityDependencies []EntityDependency

	// Sorted iteration for deterministic output
	sortedKeys := sortedFragmentKeys(deepReferences)

	// Dependency pairs: (depends, dependant)
	depPairs := [][2]string{
		{"$260", "$164"},
		{"$164", "$417"},
	}

	for _, fragKey := range sortedKeys {
		var mandatoryDeps []string
		var optionalDeps []string

		for _, pair := range depPairs {
			if fragKey.FType == pair[0] {
				sortedRefs := sortedFragmentKeysFromSet(deepReferences[fragKey])
				for _, refFrag := range sortedRefs {
					if refFrag.FType == pair[1] {
						mandatoryDeps = append(mandatoryDeps, refFrag.FID)

						// Check optional references
						if optRefs, ok := optionalReferences[refFrag]; ok {
							sortedOptRefs := sortedFragmentKeysFromSet(optRefs)
							for _, optRefFrag := range sortedOptRefs {
								if optRefFrag.FType == pair[1] {
									optionalDeps = append(optionalDeps, optRefFrag.FID)
								}
							}
						}
					}
				}
			}
		}

		if len(mandatoryDeps) > 0 {
			entityDependencies = append(entityDependencies, EntityDependency{
				EntityID:               fragKey.FID,
				MandatoryDependencies: mandatoryDeps,
			})
		}
		if len(optionalDeps) > 0 {
			entityDependencies = append(entityDependencies, EntityDependency{
				EntityID:              fragKey.FID,
				OptionalDependencies: optionalDeps,
			})
		}
	}

	return entityDependencies
}

// ---------------------------------------------------------------------------
// rebuildContainerEntityMap (yj_structure.py:1006-1040)
// ---------------------------------------------------------------------------

// ContainerEntity represents a $419 container entity map fragment.
type ContainerEntity struct {
	ContainerID        string
	EntityIDs          []string
	EntityDependencies []EntityDependency
}

// RebuildContainerEntityMap reconstructs the $419 container_entity_map fragment.
// Python: BookStructure.rebuild_container_entity_map (yj_structure.py:1006-1040)
func RebuildContainerEntityMap(fragments FragmentList, containerID string, entityDependencies []EntityDependency) FragmentList {
	var newFragments FragmentList
	var entityIDs []string
	var oldEntityDeps []EntityDependency

	for _, frag := range fragments {
		if frag.FType == "$419" {
			// Extract old entity dependencies if present
			if valMap, ok := asMap(frag.Value); ok {
				if depsVal, ok := valMap["$253"]; ok {
					_ = depsVal // keep for reference
				}
			}
			oldEntityDeps = entityDependencies // use provided if not nil
			continue
		}

		newFragments = append(newFragments, frag)

		// Collect entity IDs from non-container, non-root fragments
		if !ContainerFragmentTypes[frag.FType] && frag.FID != frag.FType {
			found := false
			for _, id := range entityIDs {
				if id == frag.FID {
					found = true
					break
				}
			}
			if !found {
				entityIDs = append(entityIDs, frag.FID)
			}
		}
	}

	if entityDependencies == nil {
		entityDependencies = oldEntityDeps
	}

	// Build container_contents: {$155: containerID, $181: entityIDs}
	containerContents := map[string]interface{}{
		"$155": containerID,
		"$181": entityIDsToInterface(entityIDs),
	}

	containerEntityMap := map[string]interface{}{
		"$252": []interface{}{containerContents},
	}

	if len(entityDependencies) > 0 {
		containerEntityMap["$253"] = entityDepsToInterface(entityDependencies)
	}

	if len(entityIDs) > 0 || len(entityDependencies) > 0 {
		newFragments = append(newFragments, Fragment{
			FType: "$419",
			Value: containerEntityMap,
		})
	} else {
		log.Printf("kfx: error: Omitting container_entity_map due to lack of content")
	}

	return newFragments
}

func entityIDsToInterface(ids []string) []interface{} {
	result := make([]interface{}, len(ids))
	for i, id := range ids {
		result[i] = id
	}
	return result
}

func entityDepsToInterface(deps []EntityDependency) []interface{} {
	result := make([]interface{}, len(deps))
	for i, dep := range deps {
		depMap := map[string]interface{}{
			"$155": dep.EntityID,
		}
		if len(dep.MandatoryDependencies) > 0 {
			depMap["$254"] = stringSliceToInterface(dep.MandatoryDependencies)
		}
		if len(dep.OptionalDependencies) > 0 {
			depMap["$255"] = stringSliceToInterface(dep.OptionalDependencies)
		}
		result[i] = depMap
	}
	return result
}

func stringSliceToInterface(ss []string) []interface{} {
	result := make([]interface{}, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}

// ---------------------------------------------------------------------------
// Set operations
// ---------------------------------------------------------------------------

func union(a, b map[FragmentKey]bool) map[FragmentKey]bool {
	result := map[FragmentKey]bool{}
	for k := range a {
		result[k] = true
	}
	for k := range b {
		result[k] = true
	}
	return result
}

func setDiff(a, b map[FragmentKey]bool) map[FragmentKey]bool {
	result := map[FragmentKey]bool{}
	for k := range a {
		if !b[k] {
			result[k] = true
		}
	}
	return result
}

func sortedFragmentKeys(m map[FragmentKey]map[FragmentKey]bool) []FragmentKey {
	keys := make([]FragmentKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].FType != keys[j].FType {
			return keys[i].FType < keys[j].FType
		}
		return keys[i].FID < keys[j].FID
	})
	return keys
}

func sortedFragmentKeysFromSet(m map[FragmentKey]bool) []FragmentKey {
	keys := make([]FragmentKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].FType != keys[j].FType {
			return keys[i].FType < keys[j].FType
		}
		return keys[i].FID < keys[j].FID
	})
	return keys
}

// numstrFloat formats a float using Python's %g format (same as Numstr but more explicit name).
func numstrFloat(x float64) string {
	if x == 0 {
		return "0"
	}
	if math.Abs(x) >= 1e6 || (math.Abs(x) < 1e-4 && x != 0) {
		return fmt.Sprintf("%g", x)
	}
	return fmt.Sprintf("%g", x)
}
