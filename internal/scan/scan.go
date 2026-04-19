package scan

import (
	"crypto/sha1"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/jsonout"
	"github.com/kaikozlov/kindle-koplugin/internal/kfx"
)

var supportedExtensions = map[string]bool{
	".kfx":  true,
	".azw":  true,
	".azw3": true,
	".mobi": true,
	".prc":  true,
	".pdf":  true,
}

var trailingIDPattern = regexp.MustCompile(`_(?:[A-Z0-9]{10}|[A-F0-9]{32})$`)

func Run(root string) (jsonout.ScanResult, error) {
	books := make([]jsonout.ScanBook, 0, 32)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".sdr") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !supportedExtensions[ext] {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		book := jsonout.ScanBook{
			ID:          "sha1:" + sha1Hex(filepath.ToSlash(relativePath)),
			SourcePath:  path,
			Format:      strings.TrimPrefix(ext, "."),
			LogicalExt:  strings.TrimPrefix(ext, "."),
			Title:       deriveTitle(d.Name()),
			Authors:     []string{},
			DisplayName: deriveTitle(d.Name()),
			OpenMode:    "direct",
			SourceMtime: info.ModTime().Unix(),
			SourceSize:  info.Size(),
		}

		sidecarPath := strings.TrimSuffix(path, ext) + ".sdr"
		if stat, err := os.Stat(sidecarPath); err == nil && stat.IsDir() {
			book.SidecarPath = sidecarPath
		}

		if ext == ".kfx" {
			book.LogicalExt = "epub"
			mode, reason, err := kfx.Classify(path)
			if err != nil {
				return err
			}
			book.OpenMode = mode
			book.BlockReason = reason

			// Extract metadata from unencrypted sidecar (works for DRM books too)
			sidecarDir := ""
			if stat, err := os.Stat(strings.TrimSuffix(path, ext) + ".sdr"); err == nil && stat.IsDir() {
				sidecarDir = strings.TrimSuffix(path, ext) + ".sdr"
			}
			if sidecarDir == "" {
				sidecarDir = kfx.SidecarDirForBook(path)
			}
			if meta := kfx.ExtractSidecarMetadata(sidecarDir); meta != nil {
				if meta.Title != "" {
					book.Title = meta.Title
					book.DisplayName = meta.Title
				}
				if len(meta.Authors) > 0 {
					book.Authors = meta.Authors
				}
			}
		}

		books = append(books, book)
		return nil
	})
	if err != nil {
		return jsonout.ScanResult{}, err
	}

	sort.Slice(books, func(i, j int) bool {
		left := strings.ToLower(books[i].DisplayName)
		right := strings.ToLower(books[j].DisplayName)
		if left == right {
			return books[i].SourcePath < books[j].SourcePath
		}
		return left < right
	})

	return jsonout.ScanResult{
		Version: 1,
		Root:    root,
		Books:   books,
	}, nil
}

func deriveTitle(filename string) string {
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)
	name = trailingIDPattern.ReplaceAllString(name, "")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.TrimSpace(name)
	if name == "" {
		return "Untitled"
	}
	return name
}

func sha1Hex(text string) string {
	sum := sha1.Sum([]byte(text))
	return hex.EncodeToString(sum[:])
}
