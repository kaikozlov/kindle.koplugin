package kfx

// Port of yj_container.py fragment data model types and operations.
// Python reference: REFERENCE/Calibre_KFX_Input/kfxlib/yj_container.py

import "fmt"

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

// String returns a string representation of a FragmentKey.
func (fk FragmentKey) String() string {
	if fk.FID != "" {
		return fmt.Sprintf("(%s, %s)", fk.FType, fk.FID)
	}
	return fmt.Sprintf("(%s)", fk.FType)
}

// ---------------------------------------------------------------------------
// Fragment type sets (yj_container.py constants)
// ---------------------------------------------------------------------------

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
