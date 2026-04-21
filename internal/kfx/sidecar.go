package kfx

import (
	"os"
	"path/filepath"
	"strings"
)

// SidecarMetadata holds metadata extracted from the .sdr sidecar directory.
type SidecarMetadata struct {
	Title   string
	Authors []string
}

// ExtractSidecarMetadata reads metadata.kfx from the book's .sdr/assets/
// directory and extracts title and authors without requiring DRM decryption.
// The metadata.kfx file is an unencrypted CONT KFX container.
func ExtractSidecarMetadata(sidecarDir string) *SidecarMetadata {
	if sidecarDir == "" {
		return nil
	}

	metadataPath := filepath.Join(sidecarDir, "assets", "metadata.kfx")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil
	}

	// Verify it's a CONT container
	if len(data) < 18 || !isCONT(data) {
		return nil
	}

	// Load it as a container source
	src, err := loadContainerSourceData(metadataPath, data)
	if err != nil {
		return nil
	}

	// Organize fragments to extract metadata
	state, err := organizeFragments(metadataPath, []*containerSource{src})
	if err != nil {
		return nil
	}

	meta := &SidecarMetadata{}
	if state.Book.Title != "" && state.Book.Title != metadataPath {
		meta.Title = state.Book.Title
	}
	if len(state.Book.Authors) > 0 {
		meta.Authors = state.Book.Authors
	}

	if meta.Title == "" && len(meta.Authors) == 0 {
		return nil
	}

	return meta
}

// isCONT checks if data starts with the CONT signature.
func isCONT(data []byte) bool {
	return len(data) >= 4 && string(data[:4]) == "CONT"
}

// SidecarDirForBook returns the expected .sdr directory path for a book file.
func SidecarDirForBook(bookPath string) string {
	ext := filepath.Ext(bookPath)
	return strings.TrimSuffix(bookPath, ext) + ".sdr"
}

// ExtractCoverJPEG extracts the cover JPEG image from the book's .sdr/assets/metadata.kfx
// sidecar file. The metadata.kfx is an unencrypted CONT KFX container for most books
// (even DRM-protected ones). It typically embeds cover/thumbnail JPEGs in the ION data.
// We scan for JPEG markers and return the largest one (usually the cover).
// Returns nil if no cover is found.
func ExtractCoverJPEG(sidecarDir string) []byte {
	if sidecarDir == "" {
		return nil
	}

	metadataPath := filepath.Join(sidecarDir, "assets", "metadata.kfx")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil
	}

	return extractLargestJPEG(data)
}

// extractLargestJPEG scans raw bytes for JPEG markers and returns the largest
// JPEG found. This is a fallback for DRMION-encrypted metadata where ION
// parsing is not possible. It works because some metadata.kfx files embed
// JPEG thumbnails even when the ION structure is encrypted.
func extractLargestJPEG(data []byte) []byte {
	var bestData []byte
	bestSize := 0

	i := 0
	for i < len(data)-2 {
		if data[i] == 0xFF && data[i+1] == 0xD8 && data[i+2] == 0xFF {
			end := findJPEGEnd(data, i)
			if end > i {
				jpeg := data[i:end]
				if len(jpeg) > bestSize {
					bestData = jpeg
					bestSize = len(jpeg)
				}
				i = end
				continue
			}
		}
		i++
	}

	return bestData
}

// findJPEGEnd finds the JPEG EOI marker (FFD9) after the given start position.
func findJPEGEnd(data []byte, start int) int {
	i := start + 2
	for i < len(data)-1 {
		if data[i] == 0xFF && data[i+1] == 0xD9 {
			return i + 2
		}
		i++
	}
	return -1
}
