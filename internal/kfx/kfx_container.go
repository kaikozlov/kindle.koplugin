package kfx

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
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

type containerBlob struct {
	Path string
	Data []byte
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

	return &containerSource{
		Path:          path,
		Data:          data,
		HeaderLen:     headerLen,
		ContainerInfo: containerInfo,
		DocSymbols:    docSymbols,
		IndexData:     indexData,
	}, nil
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

func entityPayload(data []byte) ([]byte, error) {
	if len(data) < 10 || string(data[:4]) != "ENTY" {
		return nil, &UnsupportedError{Message: "entity wrapper is invalid"}
	}
	headerLen := int(binary.LittleEndian.Uint32(data[6:10]))
	if headerLen < 10 || headerLen > len(data) {
		return nil, &UnsupportedError{Message: "entity header length is invalid"}
	}
	return data[headerLen:], nil
}
