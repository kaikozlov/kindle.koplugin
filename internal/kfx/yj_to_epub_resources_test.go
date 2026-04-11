package kfx

import "testing"

func TestPackageResourceStemImageUsesSymbolFormat(t *testing.T) {
	r := resourceFragment{
		ID:        "r1",
		Location:  "resource/c73-thumb.jpg",
		MediaType: "image/jpeg",
	}
	stem, ext := packageResourceStem(r, symShort)
	if ext != ".jpg" {
		t.Fatalf("ext = %q", ext)
	}
	// SHORT format strips leading resource/ from unique_part (yj_to_epub.py).
	if want := "image_c73-thumb"; stem != want {
		t.Fatalf("stem symShort = %q want %q", stem, want)
	}
}

func TestPackageResourceStemResourceOriginal(t *testing.T) {
	r := resourceFragment{
		ID:        "plug1",
		Location:  "path/plugin-entry.bin",
		MediaType: "application/octet-stream",
	}
	stem, ext := packageResourceStem(r, symOriginal)
	if ext != ".bin" {
		t.Fatalf("ext = %q", ext)
	}
	// Python resource_location_filename preserves path prefix (not "resource/") in the output.
	if want := "path/resource_plugin-entry"; stem != want {
		t.Fatalf("stem = %q want %q", stem, want)
	}
}

func TestUniquePackageResourceFilenameDedupesCaseInsensitive(t *testing.T) {
	used := map[string]struct{}{}
	a := resourceFragment{ID: "a", Location: "x/img.jpg", MediaType: "image/jpeg"}
	b := resourceFragment{ID: "b", Location: "y/img.jpg", MediaType: "image/jpeg"}
	f0 := uniquePackageResourceFilename(a, symOriginal, used)
	f1 := uniquePackageResourceFilename(b, symOriginal, used)
	if f0 == f1 {
		t.Fatalf("expected distinct names, got %q twice", f0)
	}
}
