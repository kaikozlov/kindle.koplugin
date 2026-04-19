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
