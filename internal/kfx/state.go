package kfx

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type containerSource struct {
	Path          string
	Data          []byte
	HeaderLen     int
	ContainerInfo map[string]interface{}
	DocSymbols    []byte // raw ION symbol table data from this container (may be empty)
	IndexData     []byte
}

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

type containerBlob struct {
	Path string
	Data []byte
}

type fragmentTypeSnapshot struct {
	Count int      `json:"count"`
	IDs   []string `json:"ids,omitempty"`
}

type fragmentSnapshot struct {
	Title string                          `json:"title"`
	Types map[string]fragmentTypeSnapshot `json:"types"`
}

func buildBookState(path string) (*bookState, error) {
	sources, err := loadBookSources(path)
	if err != nil {
		return nil, err
	}
	return organizeFragments(path, sources)
}

// buildBookStateFromData creates a bookState from in-memory CONT KFX data.
// This is used after DRMION decryption produces a valid CONT container.
func buildBookStateFromData(contData []byte) (*bookState, error) {
	if len(contData) < 18 || !bytes.HasPrefix(contData, contSignature) {
		return nil, &UnsupportedError{Message: "data is not a valid CONT KFX container"}
	}

	source, err := loadContainerSourceData("<decrypted>", contData)
	if err != nil {
		return nil, err
	}

	return organizeFragments("<decrypted>", []*containerSource{source})
}

func loadBookSources(path string) ([]*containerSource, error) {
	blobs, hasDRM, err := collectContainerBlobs(path)
	if err != nil {
		return nil, err
	}
	if hasDRM {
		return nil, &DRMError{Message: "DRM-protected KFX is not supported"}
	}
	if len(blobs) == 0 {
		return nil, &UnsupportedError{Message: "file does not contain any readable CONT KFX containers"}
	}

	sources := make([]*containerSource, 0, len(blobs))
	for _, blob := range blobs {
		source, err := loadContainerSourceData(blob.Path, blob.Data)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func collectContainerBlobs(path string) ([]containerBlob, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}

	var blobs []containerBlob
	var hasDRM bool

	switch {
	case bytes.HasPrefix(data, drmionSignature):
		hasDRM = true
	case bytes.HasPrefix(data, contSignature):
		blobs = append(blobs, containerBlob{Path: path, Data: data})

		sidecarRoot := strings.TrimSuffix(path, filepath.Ext(path)) + ".sdr"
		sidecarBlobs, sidecarDRMionBlobs, err := collectSidecarContainerBlobs(sidecarRoot)
		if err != nil {
			return nil, false, err
		}
		blobs = append(blobs, sidecarBlobs...)
		hasDRM = hasDRM || len(sidecarDRMionBlobs) > 0
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		zipBlobs, zipDRM, err := collectZipContainerBlobs(path, data)
		if err != nil {
			return nil, false, err
		}
		blobs = append(blobs, zipBlobs...)
		hasDRM = hasDRM || zipDRM
	default:
		return nil, false, nil
	}

	return blobs, hasDRM, nil
}

func collectSidecarContainerBlobs(root string) ([]containerBlob, []containerBlob, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, nil
	}

	var names []string
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		names = append(names, path)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	sort.Strings(names)

	contBlobs := make([]containerBlob, 0, len(names))
	drmionBlobs := make([]containerBlob, 0)
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, nil, err
		}
		switch {
		case bytes.HasPrefix(data, contSignature):
			contBlobs = append(contBlobs, containerBlob{Path: name, Data: data})
		case bytes.HasPrefix(data, drmionSignature):
			drmionBlobs = append(drmionBlobs, containerBlob{Path: name, Data: data})
		}
	}
	return contBlobs, drmionBlobs, nil
}

func collectZipContainerBlobs(path string, data []byte) ([]containerBlob, bool, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, false, err
	}

	type member struct {
		name string
		file *zip.File
	}
	members := make([]member, 0, len(archive.File))
	for _, file := range archive.File {
		if file.FileInfo().IsDir() {
			continue
		}
		members = append(members, member{name: file.Name, file: file})
	}
	sort.Slice(members, func(i, j int) bool {
		return members[i].name < members[j].name
	})

	blobs := make([]containerBlob, 0, len(members))
	hasDRM := false
	for _, member := range members {
		reader, err := member.file.Open()
		if err != nil {
			return nil, false, err
		}
		memberData, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return nil, false, readErr
		}
		if closeErr != nil {
			return nil, false, closeErr
		}

		memberPath := path + "#" + member.name
		switch {
		case bytes.HasPrefix(memberData, contSignature):
			blobs = append(blobs, containerBlob{Path: memberPath, Data: memberData})
		case bytes.HasPrefix(memberData, drmionSignature):
			hasDRM = true
		}
	}
	return blobs, hasDRM, nil
}

func loadContainerSource(path string) (*containerSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadContainerSourceData(path, data)
}

// validateEntityOffsets checks that all entity offsets in the container's
// index table are within the data bounds. Decrypted DRMION sidecars may
// have index entries referencing positions in the original encrypted data
// that don't match the decrypted CONT structure.
func validateEntityOffsets(src *containerSource) bool {
	for offset := 0; offset+24 <= len(src.IndexData); offset += 24 {
		entityOffset := int(binary.LittleEndian.Uint64(src.IndexData[offset+8 : offset+16]))
		entityLength := int(binary.LittleEndian.Uint64(src.IndexData[offset+16 : offset+24]))
		start := src.HeaderLen + entityOffset
		end := start + entityLength
		if start < 0 || end > len(src.Data) || start >= end {
			log.Printf("kfx: invalid entity in %s: headerLen=%d entityOffset=%d entityLength=%d dataLen=%d",
				src.Path, src.HeaderLen, entityOffset, entityLength, len(src.Data))
			return false
		}
	}
	return true
}

func loadContainerSourceData(path string, data []byte) (*containerSource, error) {
	if len(data) < 18 || !bytes.HasPrefix(data, contSignature) {
		return nil, &UnsupportedError{Message: "file is not a CONT KFX container"}
	}

	headerLen := int(binary.LittleEndian.Uint32(data[6:10]))
	containerInfoOffset := int(binary.LittleEndian.Uint32(data[10:14]))
	containerInfoLength := int(binary.LittleEndian.Uint32(data[14:18]))
	if headerLen <= 0 || containerInfoOffset+containerInfoLength > len(data) {
		return nil, &UnsupportedError{Message: "container header is invalid"}
	}

	containerInfo, err := decodeIonMap(data[containerInfoOffset:containerInfoOffset+containerInfoLength], nil, nil)
	if err != nil {
		return nil, err
	}

	docSymbolOffset, ok := asInt(containerInfo["$415"])
	if !ok {
		return nil, &UnsupportedError{Message: "KFX document symbol table is missing"}
	}
	docSymbolLength, ok := asInt(containerInfo["$416"])
	if !ok {
		return nil, &UnsupportedError{Message: "KFX document symbol table length is missing"}
	}
	indexOffset, ok := asInt(containerInfo["$413"])
	if !ok {
		return nil, &UnsupportedError{Message: "KFX index table is missing"}
	}
	indexLength, ok := asInt(containerInfo["$414"])
	if !ok {
		return nil, &UnsupportedError{Message: "KFX index table length is missing"}
	}

	if docSymbolOffset+docSymbolLength > len(data) || indexOffset+indexLength > len(data) {
		return nil, &UnsupportedError{Message: "KFX offsets are out of range"}
	}

	docSymbols := data[docSymbolOffset : docSymbolOffset+docSymbolLength]
	indexData := data[indexOffset : indexOffset+indexLength]

	return &containerSource{
		Path:          path,
		Data:          data,
		HeaderLen:     headerLen,
		ContainerInfo: containerInfo,
		DocSymbols:    docSymbols,
		IndexData:     indexData,
	}, nil
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

// Port of KFX_EPUB.organize_fragments_by_type (yj_to_epub.py) adapted to the Go fragmentCatalog layout.
// replace_ion_data symbol collection is approximated by recording resolved fragment IDs during the index walk.
func organizeFragments(bookPath string, sources []*containerSource) (*bookState, error) {
	fragments := fragmentCatalog{
		TitleMetadata:     nil,
		ContentFeatures:   map[string]interface{}{},
		DocumentData:      map[string]interface{}{},
		ContentFragments:  map[string][]string{},
		Storylines:        map[string]map[string]interface{}{},
		StyleFragments:    map[string]map[string]interface{}{},
		RubyGroups:        map[string]map[string]interface{}{},
		RubyContents:      map[string]map[string]interface{}{},
		SectionFragments:  map[string]sectionFragment{},
		AnchorFragments:   map[string]anchorFragment{},
		NavContainers:     map[string]map[string]interface{}{},
		ResourceFragments: map[string]resourceFragment{},
		ResourceRawData:   map[string]map[string]interface{}{},
		FormatCapabilities: map[string]map[string]interface{}{},
		Generators:        map[string]map[string]interface{}{},
		FontFragments:     map[string]fontFragment{},
		RawFragments:      map[string][]byte{},
		PositionAliases:   map[int]string{},
		FragmentIDsByType: map[string][]string{},
	}
	book := &decodedBook{
		Identifier: bookPath,
		Title:      strings.TrimSuffix(filepath.Base(bookPath), filepath.Ext(bookPath)),
		Language:   "en",
	}

	// Two-pass approach matching Calibre's yj_book.decode_book():
	//   Pass 1: container.deserialize() → loads doc_symbols into shared symtab
	//   Pass 2: container.get_fragments() → decodes entities with accumulated symtab
	//
	// Calibre processes all containers in loop 1 (loading symbols), then all
	// containers in loop 2 (decoding fragments). This ensures ALL docSymbols
	// are accumulated before any entity is decoded.
	sharedSym := &sharedDocSymbols{}

	// Sort sources alphabetically by path, matching Calibre's sequential
	// processing order.
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Path < sources[j].Path
	})

	// Pass 1: Accumulate docSymbols from all sources (Calibre: deserialize loop).
	for _, source := range sources {
		sharedSym.update(source.DocSymbols)
	}

	// Build resolver from fully accumulated docSymbols.
	srcDocSymbols := sharedSym.get()
	if len(srcDocSymbols) == 0 {
		return nil, &UnsupportedError{Message: "no document symbol table found in any container"}
	}
	resolver, err := newSymbolResolver(srcDocSymbols)
	if err != nil {
		return nil, err
	}

	bookSymbols := map[string]struct{}{}
	fontCount := 0
	categorizedData := map[string]map[string]bool{}

	// Pass 2: Decode all fragments using the fully accumulated resolver
	// (Calibre: get_fragments loop).
	for _, source := range sources {
		lastContainerID := ""
		for offset := 0; offset+24 <= len(source.IndexData); offset += 24 {
			idID := binary.LittleEndian.Uint32(source.IndexData[offset : offset+4])
			typeID := binary.LittleEndian.Uint32(source.IndexData[offset+4 : offset+8])
			entityOffset := int(binary.LittleEndian.Uint64(source.IndexData[offset+8 : offset+16]))
			entityLength := int(binary.LittleEndian.Uint64(source.IndexData[offset+16 : offset+24]))
			start := source.HeaderLen + entityOffset
			end := start + entityLength
			if start < 0 || end > len(source.Data) || start >= end {
				return nil, &UnsupportedError{Message: "entity offset is out of range"}
			}

			entityData := source.Data[start:end]
			fragmentID := resolver.Resolve(idID)
			bookSymbols[fragmentID] = struct{}{}
			fragmentType := fmt.Sprintf("$%d", typeID)
			payload, err := entityPayload(entityData)
			if err != nil {
				return nil, err
			}

			summaryID := fragmentID
			switch fragmentType {
			case "$270":
				value, err := decodeIonMap(payload, srcDocSymbols, resolver)
				if err != nil {
					return nil, err
				}
				containerID := fmt.Sprintf("%s:%s", asStringDefault(value["$161"]), asStringDefault(value["$409"]))
				lastContainerID = containerID
				summaryID = containerID
			case "$593":
				summaryID = lastContainerID
			case "$262":
				summaryID = fmt.Sprintf("%s-font-%03d", fragmentID, fontCount)
				fontCount++
			case "$258":
				value, err := decodeIonMap(payload, srcDocSymbols, resolver)
				if err != nil {
					return nil, err
				}
				summaryID = chooseFragmentIdentity(fragmentID, value["$169"])
			case "$387":
				value, err := decodeIonMap(payload, srcDocSymbols, resolver)
				if err != nil {
					return nil, err
				}
				summaryID = fmt.Sprintf("%s:%s", fragmentID, asStringDefault(value["$215"]))
			}
			fragments.FragmentIDsByType[fragmentType] = append(fragments.FragmentIDsByType[fragmentType], summaryID)

			// Track categorized IDs for duplicate/null detection (Python organize_fragments_by_type L202-204).
			if categorizedData[fragmentType] == nil {
				categorizedData[fragmentType] = map[string]bool{}
			}
			if categorizedData[fragmentType][summaryID] {
				log.Printf("kfx: book contains multiple %s fragments with id %s", fragmentType, summaryID)
			}
			categorizedData[fragmentType][summaryID] = true

			switch fragmentType {
			case "$145", "$157", "$164", "$258", "$259", "$260", "$262", "$266", "$270", "$391", "$490", "$538", "$585", "$593", "$608", "$609", "$756":
				value, err := decodeIonMap(payload, srcDocSymbols, resolver)
				if err != nil {
					return nil, err
				}

				switch fragmentType {
				case "$145":
					name, _ := asString(value["name"])
					stringsValue := toStringSlice(value["$146"])
					if name != "" && len(stringsValue) > 0 {
						fragments.ContentFragments[name] = stringsValue
					}
				case "$157":
					id := chooseFragmentIdentity(fragmentID, value["$173"])
					if id != "" {
						fragments.StyleFragments[id] = value
					}
				case "$164":
					mergeIonReferencedStringSymbols(value, bookSymbols)
					resource := parseResourceFragment(fragmentID, value)
					if resource.Location != "" {
						fragments.ResourceFragments[resource.ID] = resource
					}
					fragments.ResourceRawData[resource.ID] = value
				case "$258":
					order := readSectionOrder(value)
					if len(order) > 0 {
						fragments.SectionOrder = order
					}
					// Store $258 for applyReadingOrderMetadata (Python process_metadata L103: book_data.pop("$258", {})).
					fragments.ReadingOrderMetadata = value
				case "$259":
					id := chooseFragmentIdentity(fragmentID, value["$176"])
					if id != "" {
						fragments.Storylines[id] = value
					}
				case "$260":
					section := parseSectionFragment(fragmentID, value)
					if section.ID != "" && section.Storyline != "" {
						fragments.SectionFragments[section.ID] = section
					}
				case "$262":
					font := parseFontFragment(value)
					if font.Location != "" {
						fragments.FontFragments[font.Location] = font
					}
				case "$266":
					mergeIonReferencedStringSymbols(value, bookSymbols)
					anchor := parseAnchorFragment(fragmentID, value)
					if anchor.ID != "" && (anchor.PositionID != 0 || anchor.URI != "") {
						fragments.AnchorFragments[anchor.ID] = anchor
					}
				case "$270":
					fragments.Generators[summaryID] = value
				case "$391":
					id := chooseFragmentIdentity(fragmentID, value["$239"])
					if id != "" {
						fragments.NavContainers[id] = value
					}
				case "$490":
					fragments.TitleMetadata = value
				case "$538":
					fragments.DocumentData = value
				case "$585":
					fragments.ContentFeatures = value
				case "$593":
					fragments.FormatCapabilities[summaryID] = value
				case "$608":
					id := chooseFragmentIdentity(fragmentID, value["$758"])
					if id == "" {
						id = fragmentID
					}
					fragments.RubyContents[id] = value
				case "$756":
					id := chooseFragmentIdentity(fragmentID, value["$757"])
					if id == "" {
						id = fragmentID
					}
					fragments.RubyGroups[id] = value
				case "$609":
					sectionID := parsePositionMapSectionID(fragmentID, value)
					for _, positionID := range readPositionMap(value) {
						if positionID != 0 && sectionID != "" {
							fragments.PositionAliases[positionID] = sectionID
						}
					}
				}
			case "$389":
				value, err := decodeIonValue(payload, srcDocSymbols, resolver)
				if err != nil {
					return nil, err
				}
				if rootList, ok := asSlice(value); ok {
					for _, entry := range rootList {
						entryMap, ok := asMap(entry)
						if ok {
							fragments.NavRoots = append(fragments.NavRoots, entryMap)
						}
					}
				}
			case "$417", "$418":
				if fragmentID != "" {
					dataCopy := append([]byte(nil), payload...)
					fragments.RawFragments[fragmentID] = dataCopy
					fragments.RawBlobOrder = append(fragments.RawBlobOrder, rawBlob{
						ID:   fragmentID,
						Data: dataCopy,
					})
				}
			}
		}
	}

	// Null ID detection (Python organize_fragments_by_type L214):
	// When a category has multiple entries including an empty/null ID, log an error.
	for category, ids := range categorizedData {
		if len(ids) > 1 {
			if ids[""] || ids["\x00"] {
				log.Printf("kfx: fragment list contains mixed null/non-null ids of type %q", category)
			}
		}
	}

	// Port of replace_ion_data string-symbol discovery (yj_to_epub.py): collect YJ field string values into bookSymbols.
	mergeIonReferencedStringSymbols(fragments.TitleMetadata, bookSymbols)
	mergeIonReferencedStringSymbols(fragments.DocumentData, bookSymbols)
	mergeIonReferencedStringSymbols(fragments.ContentFeatures, bookSymbols)
	for _, m := range fragments.StyleFragments {
		mergeIonReferencedStringSymbols(m, bookSymbols)
	}
	for _, m := range fragments.Storylines {
		mergeIonReferencedStringSymbols(m, bookSymbols)
	}
	for _, m := range fragments.NavContainers {
		mergeIonReferencedStringSymbols(m, bookSymbols)
	}
	for _, m := range fragments.NavRoots {
		mergeIonReferencedStringSymbols(m, bookSymbols)
	}
	mergeContentFragmentStringSymbols(fragments.ContentFragments, bookSymbols)
	for _, m := range fragments.RubyGroups {
		mergeIonReferencedStringSymbols(m, bookSymbols)
	}
	for _, m := range fragments.RubyContents {
		mergeIonReferencedStringSymbols(m, bookSymbols)
	}
	for _, sec := range fragments.SectionFragments {
		mergeIonReferencedStringSymbols(sec.PageTemplateValues, bookSymbols)
		for _, t := range sec.PageTemplates {
			mergeIonReferencedStringSymbols(t.PageTemplateValues, bookSymbols)
			mergeIonReferencedStringSymbols(t.Condition, bookSymbols)
		}
	}

	// Port of Python process_document_data reading_orders: $169 from $538 document data.
	if len(fragments.SectionOrder) == 0 {
		if docOrder := readSectionOrder(fragments.DocumentData); len(docOrder) > 0 {
			fragments.SectionOrder = docOrder
		}
	}
	if len(fragments.SectionOrder) == 0 {
		for sectionID := range fragments.SectionFragments {
			fragments.SectionOrder = append(fragments.SectionOrder, sectionID)
		}
		sort.Strings(fragments.SectionOrder)
	}
	for fragmentType := range fragments.FragmentIDsByType {
		sort.Strings(fragments.FragmentIDsByType[fragmentType])
	}

	var primarySource *containerSource
	if len(sources) > 0 {
		primarySource = sources[0]
	}

	symbolFormat := determineBookSymbolFormat(bookSymbols, fragments.DocumentData, resolver)
	// KFX_EPUB.__init__ L77–80 after determine_book_symbol_format (L76).
	applyKFXEPUBInitMetadataAfterOrganize(book, &fragments)

	return &bookState{
		Path:             bookPath,
		Source:           primarySource,
		Sources:          sources,
		Book:             book,
		Fragments:        fragments,
		BookSymbols:      bookSymbols,
		BookSymbolFormat: symbolFormat,
	}, nil
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
