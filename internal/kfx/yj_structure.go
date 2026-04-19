package kfx

// Port of yj_structure.py fragment validation functions, constants, and helpers.
// Python reference: REFERENCE/Calibre_KFX_Input/kfxlib/yj_structure.py

import (
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

// RootFragmentTypes are the root-level fragment types.
// Python: ROOT_FRAGMENT_TYPES (yj_container.py:70-88)
var RootFragmentTypes = map[string]bool{
	"$ion_symbol_table": true,
	"$270":              true,
	"$490":              true,
	"$389":              true,
	"$419":              true,
	"$585":              true,
	"$538":              true,
	"$262":              true,
	"$593":              true,
	"$550":              true,
	"$258":              true,
	"$265":              true,
	"$264":              true,
	"$395":              true,
	"$390":              true,
	"$621":              true,
	"$611":              true,
}

// ContainerFragmentTypes are the container-level fragment types.
// Python: CONTAINER_FRAGMENT_TYPES (yj_container.py:145-151)
var ContainerFragmentTypes = map[string]bool{
	"$270":              true,
	"$593":              true,
	"$ion_symbol_table": true,
	"$419":              true,
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
// Fragment key and fragment data types (matching Python YJFragmentKey/YJFragment)
// ---------------------------------------------------------------------------

// FragmentKey identifies a fragment by type and ID.
// Python: YJFragmentKey (yj_container.py:165)
type FragmentKey struct {
	FType string
	FID   string
}

// Fragment represents a single fragment with type, ID, and value.
// Python: YJFragment (yj_container.py:225)
type Fragment struct {
	FType string
	FID   string
	Value interface{}
}

// FragmentList is a sortable list of fragments.
// Python: YJFragmentList (yj_container.py:273)
type FragmentList []Fragment

// Get returns the first fragment matching the given type and optional ID.
// Python: YJFragmentList.get (yj_container.py:297-327)
func (fl FragmentList) Get(ftype string, fid string, first bool) *Fragment {
	var match *Fragment
	for i := range fl {
		if fl[i].FType == ftype {
			if fid == "" || fl[i].FID == fid {
				if first {
					return &fl[i]
				}
				if match != nil {
					return nil // multiple matches
				}
				match = &fl[i]
			}
		}
	}
	return match
}

// GetAll returns all fragments matching the given type.
func (fl FragmentList) GetAll(ftype string) FragmentList {
	var result FragmentList
	for i := range fl {
		if fl[i].FType == ftype {
			result = append(result, fl[i])
		}
	}
	return result
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
				log.Printf("kfx: Found multiple annotations in %s of %s fragment", cont, fragment.FType)
			}
			for _, annot := range ann.Annotations {
				if !ExpectedAnnotations[[3]string{fragment.FType, cont, annot}] &&
					!ExpectedDictionaryAnnotations[[3]string{fragment.FType, cont, annot}] {
					log.Printf("kfx: Found unexpected IonAnnotation %s in %s of %s fragment", annot, cont, fragment.FType)
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
// checkFragmentUsage (yj_structure.py:718-852)
// ---------------------------------------------------------------------------

// CheckFragmentUsageResult holds the results of check_fragment_usage.
type CheckFragmentUsageResult struct {
	Referenced   FragmentList
	Unreferenced FragmentList
	Missing      map[FragmentKey]bool
}

// CheckFragmentUsage performs a BFS walk of the fragment reference graph.
// Python: BookStructure.check_fragment_usage (yj_structure.py:718-852)
func CheckFragmentUsage(fragments FragmentList, getCoverFID func() string) CheckFragmentUsageResult {
	discovered := map[FragmentKey]bool{}

	unreferencedFragmentTypes := make(map[string]bool)
	for k := range RootFragmentTypes {
		if k != "$419" {
			unreferencedFragmentTypes[k] = true
		}
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

			WalkFragment(frag, &mandatoryRefs, &optionalRefs, &eidDefs, &eidRefs, fragments)

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
	var referencedFragments FragmentList
	var unreferencedFragments FragmentList
	alreadyProcessed := map[FragmentKey]Fragment{}

	for _, frag := range fragments {
		if frag.FType != "$262" && frag.FType != "$387" {
			if existing, exists := alreadyProcessed[FragmentKey{FType: frag.FType, FID: frag.FID}]; exists {
				if frag.FType == "$270" || frag.FType == "$593" {
					continue
				}
				_ = existing
				continue
			}
			alreadyProcessed[FragmentKey{FType: frag.FType, FID: frag.FID}] = frag
		}

		fragKey := FragmentKey{FType: frag.FType, FID: frag.FID}
		if visited[fragKey] {
			referencedFragments = append(referencedFragments, frag)
		} else if ContainerFragmentTypes[frag.FType] || frag.FID == frag.FType {
			log.Printf("kfx: error: Unexpected root fragment: %s", fragKey)
		} else {
			unreferencedFragments = append(unreferencedFragments, frag)
		}
	}

	if len(unreferencedFragments) > 0 {
		names := make([]string, len(unreferencedFragments))
		for i, f := range unreferencedFragments {
			names[i] = fmt.Sprintf("%s", FragmentKey{FType: f.FType, FID: f.FID})
		}
		log.Printf("kfx: error: Unreferenced fragments: %s", names)
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
		Referenced:   referencedFragments,
		Unreferenced: unreferencedFragments,
		Missing:      missing,
	}
}

// KnownFragmentTypes is the union of required and allowed book fragment types.
// Python: KNOWN_FRAGMENT_TYPES (yj_container.py:142)
var KnownFragmentTypes = map[string]bool{
	// REQUIRED_BOOK_FRAGMENT_TYPES
	"$ion_symbol_table": true,
	"$270":              true,
	"$490":              true,
	"$389":              true,
	"$419":              true,
	"$538":              true,
	"$550":              true,
	"$258":              true,
	"$265":              true,
	"$264":              true,
	"$611":              true,
	// ALLOWED_BOOK_FRAGMENT_TYPES
	"$266": true,
	"$597": true,
	"$418": true,
	"$417": true,
	"$394": true,
	"$145": true,
	"$585": true,
	"$610": true,
	"$164": true,
	"$262": true,
	"$593": true,
	"$391": true,
	"$692": true,
	"$387": true,
	"$395": true,
	"$756": true,
	"$260": true,
	"$267": true,
	"$390": true,
	"$609": true,
	"$259": true,
	"$608": true,
	"$157": true,
	"$621": true,
}

// String returns a string representation of a FragmentKey.
func (fk FragmentKey) String() string {
	if fk.FID != "" {
		return fmt.Sprintf("(%s, %s)", fk.FType, fk.FID)
	}
	return fmt.Sprintf("(%s)", fk.FType)
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
