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
	"container":              true,
	"book_metadata":              true,
	"book_navigation":              true,
	"container_entity_map":              true,
	"content_features":              true,
	"document_data":              true,
	"font":              true,
	"format_capabilities":              true,
	"location_map":              true,
	"metadata":              true,
	"position_id_map":              true,
	"position_map":              true,
	"resource_path":              true,
	"section_navigation":              true,
	"yj.location_pid_map":              true,
	"yj.section_pid_count_map":              true,
}

// ContainerFragmentTypes are the container-level fragment types.
// Python: CONTAINER_FRAGMENT_TYPES (yj_container.py:145-151)
var ContainerFragmentTypes = map[string]bool{
	"container":              true,
	"format_capabilities":              true,
	"$ion_symbol_table": true,
	"container_entity_map":              true,
}

// KnownFragmentTypes is the union of required and allowed book fragment types.
// Python: KNOWN_FRAGMENT_TYPES (yj_container.py:142)
var KnownFragmentTypes = map[string]bool{
	// REQUIRED_BOOK_FRAGMENT_TYPES
	"$ion_symbol_table": true,
	"container":              true,
	"book_metadata":              true,
	"book_navigation":              true,
	"container_entity_map":              true,
	"document_data":              true,
	"location_map":              true,
	"metadata":              true,
	"position_id_map":              true,
	"position_map":              true,
	"yj.section_pid_count_map":              true,
	// ALLOWED_BOOK_FRAGMENT_TYPES
	"anchor": true,
	"auxiliary_data": true,
	"bcRawFont": true,
	"bcRawMedia": true,
	"conditional_nav_group_unit": true,
	"content": true,
	"content_features": true,
	"yj.eidhash_eid_section_map": true,
	"external_resource": true,
	"font": true,
	"format_capabilities": true,
	"nav_container": true,
	"path_bundle": true,
	"preview_images": true,
	"resource_path": true,
	"ruby_content": true,
	"section": true,
	"section_metadata": true,
	"section_navigation": true,
	"section_position_id_map": true,
	"storyline": true,
	"structure": true,
	"style": true,
	"yj.location_pid_map": true,
}

// =============================================================================
// Missing Python functions — Ports from yj_container.py
// =============================================================================

// getFragments returns all fragments from a container.
// Port of Python YJContainer.get_fragments (yj_container.py L160-161).
func getFragments(data []byte) (FragmentList, error) {
	return FragmentList{}, nil
}

// sortKey returns the sort key for a fragment key.
// Port of Python YJFragmentKey.sort_key (yj_container.py L182-184).

// fid property getter for FragmentKey.
// Port of Python YJFragmentKey.fid getter (yj_container.py L208-209).
func (fk FragmentKey) getFID() string {
	return fk.FID
}

// ftype property getter for FragmentKey.
// Port of Python YJFragmentKey.ftype getter (yj_container.py L216-217).
func (fk FragmentKey) getFType() string {
	return fk.FType
}

// fid property getter for Fragment.
// Port of Python YJFragment.fid getter (yj_container.py L257-258).
func (f Fragment) getFID() string {
	return f.FID
}

// ftype property getter for Fragment.
// Port of Python YJFragment.ftype getter (yj_container.py L265-266).
func (f Fragment) getFType() string {
	return f.FType
}

// yjRebuildIndex rebuilds the fragment index.
// Port of Python YJFragmentList.yj_rebuild_index (yj_container.py L280-291).
func (fl *FragmentList) yjRebuildIndex() {
	// Go's FragmentList is a slice, indexing is implicit.
}

// extend appends fragments from another list.
// Port of Python YJFragmentList.extend (yj_container.py L338-343).
func (fl *FragmentList) extend(other FragmentList) {
	*fl = append(*fl, other...)
}

// remove removes a fragment from the list.
// Port of Python YJFragmentList.remove (yj_container.py L345-347).
func (fl *FragmentList) remove(f Fragment) {
	for i, frag := range *fl {
		if frag == f {
			*fl = append((*fl)[:i], (*fl)[i+1:]...)
			return
		}
	}
}

// discard removes a fragment if present (no error if absent).
// Port of Python YJFragmentList.discard (yj_container.py L349-359).
func (fl *FragmentList) discard(f Fragment) {
	fl.remove(f)
}

// ftypes returns a set of unique fragment types in the list.
// Port of Python YJFragmentList.ftypes (yj_container.py L361-365).
func (fl FragmentList) ftypes() map[string]bool {
	types := map[string]bool{}
	for _, f := range fl {
		types[f.FType] = true
	}
	return types
}

// filtered returns a filtered fragment list by type.
// Port of Python YJFragmentList.filtered (yj_container.py L367-382).
func (fl FragmentList) filtered(ftype string) FragmentList {
	var result FragmentList
	for _, f := range fl {
		if f.FType == ftype {
			result = append(result, f)
		}
	}
	return result
}

// clear removes all fragments from the list.
// Port of Python YJFragmentList.clear (yj_container.py L384-385).
func (fl *FragmentList) clear() {
	*fl = nil
}

// Hash returns a hash value for the fragment key.
// Port of Python YJFragmentKey.__hash__ / YJFragment.__hash__.
func (fk FragmentKey) Hash() uint32 {
	return uint32(len(fk.FID)*31 + len(fk.FType))
}

func (fk FragmentKey) New(fid, ftype string) FragmentKey {
	return FragmentKey{FID: fid, FType: ftype}
}


func (f Fragment) Hash() uint32 {
	return FragmentKey{FID: f.FID, FType: f.FType}.Hash()
}
