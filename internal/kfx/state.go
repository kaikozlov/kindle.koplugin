package kfx

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
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
	DocSymbols    []byte
	IndexData     []byte
	Resolver      *symbolResolver
}

type fragmentCatalog struct {
	ContentFeatures   map[string]interface{}
	DocumentData      map[string]interface{}
	ContentFragments  map[string][]string
	Storylines        map[string]map[string]interface{}
	StyleFragments    map[string]map[string]interface{}
	RubyGroups        map[string]map[string]interface{}
	RubyContents      map[string]map[string]interface{}
	SectionFragments  map[string]sectionFragment
	AnchorFragments   map[string]anchorFragment
	NavContainers     map[string]map[string]interface{}
	NavRoots          []map[string]interface{}
	ResourceFragments map[string]resourceFragment
	FontFragments     map[string]fontFragment
	RawFragments      map[string][]byte
	PositionAliases   map[int]string
	RawBlobOrder      []rawBlob
	SectionOrder      []string
	FragmentIDsByType map[string][]string
}

type bookState struct {
	Path      string
	Source    *containerSource
	Sources   []*containerSource
	Book      *decodedBook
	Fragments fragmentCatalog
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
		sidecarBlobs, sidecarDRM, err := collectSidecarContainerBlobs(sidecarRoot)
		if err != nil {
			return nil, false, err
		}
		blobs = append(blobs, sidecarBlobs...)
		hasDRM = hasDRM || sidecarDRM
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

func collectSidecarContainerBlobs(root string) ([]containerBlob, bool, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !info.IsDir() {
		return nil, false, nil
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
		return nil, false, err
	}
	sort.Strings(names)

	blobs := make([]containerBlob, 0, len(names))
	hasDRM := false
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, false, err
		}
		switch {
		case bytes.HasPrefix(data, contSignature):
			blobs = append(blobs, containerBlob{Path: name, Data: data})
		case bytes.HasPrefix(data, drmionSignature):
			hasDRM = true
		}
	}
	return blobs, hasDRM, nil
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
	resolver, err := newSymbolResolver(docSymbols)
	if err != nil {
		return nil, err
	}

	return &containerSource{
		Path:          path,
		Data:          data,
		HeaderLen:     headerLen,
		ContainerInfo: containerInfo,
		DocSymbols:    docSymbols,
		IndexData:     indexData,
		Resolver:      resolver,
	}, nil
}

func combineContainerDocSymbols(sources []*containerSource) []byte {
	var combined []byte
	for _, source := range sources {
		if len(source.DocSymbols) == 0 {
			continue
		}
		combined = append(combined, source.DocSymbols...)
	}
	return combined
}

func organizeFragments(bookPath string, sources []*containerSource) (*bookState, error) {
	fragments := fragmentCatalog{
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

	docSymbols := combineContainerDocSymbols(sources)
	resolver, err := newSymbolResolver(docSymbols)
	if err != nil {
		return nil, err
	}

	fontCount := 0
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
			fragmentType := fmt.Sprintf("$%d", typeID)
			payload, err := entityPayload(entityData)
			if err != nil {
				return nil, err
			}

			summaryID := fragmentID
			switch fragmentType {
			case "$270":
				value, err := decodeIonMap(payload, docSymbols, resolver)
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
				value, err := decodeIonMap(payload, docSymbols, resolver)
				if err != nil {
					return nil, err
				}
				summaryID = chooseFragmentIdentity(fragmentID, value["$169"])
			case "$387":
				value, err := decodeIonMap(payload, docSymbols, resolver)
				if err != nil {
					return nil, err
				}
				summaryID = fmt.Sprintf("%s:%s", fragmentID, asStringDefault(value["$215"]))
			}
			fragments.FragmentIDsByType[fragmentType] = append(fragments.FragmentIDsByType[fragmentType], summaryID)

			switch fragmentType {
			case "$145", "$157", "$164", "$258", "$259", "$260", "$262", "$266", "$391", "$490", "$538", "$585", "$608", "$609", "$756":
				value, err := decodeIonMap(payload, docSymbols, resolver)
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
					resource := parseResourceFragment(fragmentID, value)
					if resource.Location != "" {
						fragments.ResourceFragments[resource.ID] = resource
					}
				case "$258":
					order := readSectionOrder(value)
					if len(order) > 0 {
						fragments.SectionOrder = order
					}
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
					anchor := parseAnchorFragment(fragmentID, value)
					if anchor.ID != "" && (anchor.PositionID != 0 || anchor.URI != "") {
						fragments.AnchorFragments[anchor.ID] = anchor
					}
				case "$391":
					id := chooseFragmentIdentity(fragmentID, value["$239"])
					if id != "" {
						fragments.NavContainers[id] = value
					}
				case "$490":
					applyMetadata(book, value)
				case "$538":
					fragments.DocumentData = value
					applyDocumentData(book, value)
				case "$585":
					fragments.ContentFeatures = value
					applyContentFeatures(book, value)
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
				value, err := decodeIonValue(payload, docSymbols, resolver)
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

	return &bookState{
		Path:      bookPath,
		Source:    primarySource,
		Sources:   sources,
		Book:      book,
		Fragments: fragments,
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
