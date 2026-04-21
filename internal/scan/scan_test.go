package scan

import (
	"path/filepath"
	"testing"
)

func TestRunClassifiesFixtureBooks(t *testing.T) {
	root := filepath.Join("..", "..", "REFERENCE", "kfx_examples")

	kfxFiles, _ := filepath.Glob(filepath.Join(root, "*.kfx"))
	if len(kfxFiles) == 0 {
		t.Skip("no KFX fixture files found in REFERENCE/kfx_examples")
	}

	result, err := Run(root)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result.Books) == 0 {
		t.Skip("no KFX fixture files found in REFERENCE/kfx_examples")
	}

	for _, book := range result.Books {
		switch book.Title {
		case "Martyr":
			if book.OpenMode != "convert" {
				t.Errorf("Martyr: expected convert, got %s", book.OpenMode)
			}
		case "The Familiars":
			if book.OpenMode != "drm" && (book.OpenMode != "blocked" || book.BlockReason != "drm") {
				t.Errorf("The Familiars: expected drm or blocked:drm, got %s:%s", book.OpenMode, book.BlockReason)
			}
		}
	}
}
