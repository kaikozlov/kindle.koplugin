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
