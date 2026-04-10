package scan

import (
	"path/filepath"
	"testing"
)

func TestRunClassifiesFixtureBooks(t *testing.T) {
	root := filepath.Join("..", "..", "..", "REFERENCE", "kfx_examples")
	result, err := Run(root)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result.Books) != 3 {
		t.Fatalf("expected 3 books, got %d", len(result.Books))
	}

	got := map[string]string{}
	for _, book := range result.Books {
		got[book.Title] = book.OpenMode + ":" + book.BlockReason
	}

	if got["Martyr"] != "convert:" {
		t.Fatalf("Martyr classification = %q", got["Martyr"])
	}
	if got["The Familiars"] != "blocked:drm" {
		t.Fatalf("The Familiars classification = %q", got["The Familiars"])
	}
	if got["The Hunger Games Trilogy"] != "blocked:drm" {
		t.Fatalf("The Hunger Games Trilogy classification = %q", got["The Hunger Games Trilogy"])
	}
}
