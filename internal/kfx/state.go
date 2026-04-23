package kfx

import (
	"strings"
)

type fragmentCatalog struct {
	TitleMetadata         map[string]interface{} // $490; applied in applyKFXEPUBInitMetadataAfterOrganize (yj_to_epub.py L77–80 order).
	ContentFeatures       map[string]interface{} // $585; content features with $590 capability list.
	DocumentData          map[string]interface{} // $538; document-level data.
	ReadingOrderMetadata  map[string]interface{} // $258 top-level; applied in applyKFXEPUBInitMetadataAfterOrganize.
	ContentFragments      map[string][]string
	Storylines            map[string]map[string]interface{}
	StyleFragments        map[string]map[string]interface{}
	RubyGroups            map[string]map[string]interface{}
	RubyContents          map[string]map[string]interface{}
	SectionFragments      map[string]sectionFragment
	AnchorFragments       map[string]anchorFragment
	NavContainers         map[string]map[string]interface{}
	NavRoots              []map[string]interface{}
	ResourceFragments     map[string]resourceFragment
	ResourceRawData       map[string]map[string]interface{} // $164 raw fragment data keyed by resource ID (for format/location lookup).
	FormatCapabilities    map[string]map[string]interface{} // $593 fragments keyed by fragment ID.
	Generators            map[string]map[string]interface{} // $270 fragments keyed by fragment ID.
	FontFragments         map[string]fontFragment
	RawFragments          map[string][]byte
	PositionAliases       map[int]string
	RawBlobOrder          []rawBlob
	SectionOrder          []string
	FragmentIDsByType     map[string][]string
}

type bookState struct {
	Path             string
	Source           *containerSource
	Sources          []*containerSource
	Book             *decodedBook
	Fragments        fragmentCatalog
	BookSymbols      map[string]struct{}
	BookSymbolFormat symType
}

type fragmentTypeSnapshot struct {
	Count int      `json:"count"`
	IDs   []string `json:"ids,omitempty"`
}

type fragmentSnapshot struct {
	Title string                          `json:"title"`
	Types map[string]fragmentTypeSnapshot `json:"types"`
}





// mergeContentFragmentStringSymbols records string IDs from $145 content bundles into bookSymbols
// (Calibre replace_ion_data walks Ion; Go content fragments are already resolved strings).
func mergeContentFragmentStringSymbols(frag map[string][]string, bookSymbols map[string]struct{}) {
	for _, ids := range frag {
		for _, id := range ids {
			if id != "" {
				bookSymbols[id] = struct{}{}
			}
		}
	}
}

func mergeIonReferencedStringSymbols(value interface{}, bookSymbols map[string]struct{}) {
	switch t := value.(type) {
	case map[string]interface{}:
		for k, v := range t {
			if strings.HasPrefix(k, "$") {
				if s, ok := v.(string); ok && s != "" {
					bookSymbols[s] = struct{}{}
				}
			}
			mergeIonReferencedStringSymbols(v, bookSymbols)
		}
	case []interface{}:
		for _, v := range t {
			mergeIonReferencedStringSymbols(v, bookSymbols)
		}
	}
}

// sharedDocSymbols maintains the shared symbol table state as containers are
// processed sequentially, matching Calibre's LocalSymbolTable pattern.
// The first container with docSymbols populates the shared state; subsequent
// containers with empty docSymbols (symLen=0) inherit the shared state.
type sharedDocSymbols struct {
	current []byte // accumulated docSymbols from all processed containers
}

// update sets the shared docSymbols from a container's docSymbols.
// If the container has docSymbols, they become the new shared state.
// If the container has no docSymbols, the shared state is unchanged.
func (s *sharedDocSymbols) update(containerDocSymbols []byte) {
	if len(containerDocSymbols) > 0 {
		s.current = containerDocSymbols
	}
}

// get returns the current shared docSymbols for decoding a container's fragments.
func (s *sharedDocSymbols) get() []byte {
	return s.current
}



func (s *bookState) fragmentSnapshot() fragmentSnapshot {
	snapshot := fragmentSnapshot{
		Title: s.Book.Title,
		Types: map[string]fragmentTypeSnapshot{},
	}
	for fragmentType, ids := range s.Fragments.FragmentIDsByType {
		snapshot.Types[fragmentType] = fragmentTypeSnapshot{
			Count: len(ids),
			IDs:   append([]string(nil), ids...),
		}
	}
	return snapshot
}
