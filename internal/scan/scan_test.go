package scan

import (
	"path/filepath"
	"testing"
)

func TestRunClassifiesFixtureBooks(t *testing.T) {
	root := filepath.Join("..", "..", "REFERENCE", "kfx_examples")
	result, err := Run(root)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result.Books) != 6 {
		t.Fatalf("expected 6 books, got %d", len(result.Books))
	}

	got := map[string]string{}
	for _, book := range result.Books {
		got[book.Title] = book.OpenMode + ":" + book.BlockReason
	}

	if got["Martyr"] != "convert:" {
		t.Fatalf("Martyr classification = %q", got["Martyr"])
	}
	// DRMION books are classified with openMode "drm" (not "blocked")
	if got["The Familiars"] != "drm:" {
		t.Fatalf("The Familiars classification = %q", got["The Familiars"])
	}
	if got["The Hunger Games Trilogy"] != "drm:" {
		t.Fatalf("The Hunger Games Trilogy classification = %q", got["The Hunger Games Trilogy"])
	}
	if got["Elvis and the Underdogs"] != "drm:" {
		t.Fatalf("Elvis and the Underdogs classification = %q", got["Elvis and the Underdogs"])
	}
	if got["Three Below (Floors #2)"] != "drm:" {
		t.Fatalf("Three Below (Floors #2) classification = %q", got["Three Below (Floors #2)"])
	}
	if got["Throne of Glass"] != "drm:" {
		t.Fatalf("Throne of Glass classification = %q", got["Throne of Glass"])
	}
}
