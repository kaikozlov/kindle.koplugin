package kfx

import "testing"

func TestGetFragmentDeleteTrueSemantics(t *testing.T) {
	frag := map[string]interface{}{"story_name": "story-1", "content_list": []interface{}{"x"}}
	container := map[string]map[string]interface{}{"story-1": frag}
	book := &decodedBook{
		fragmentMaps:       map[string]map[string]map[string]interface{}{"storyline": container},
		usedFragmentAccess: map[string]bool{},
	}

	first := getFragment(book, "storyline", "story-1")
	if first == nil || first["story_name"] != "story-1" {
		t.Fatalf("first getFragment = %#v", first)
	}
	if _, stillPresent := container["story-1"]; stillPresent {
		t.Fatal("getFragment must pop the fragment from its active container on first use")
	}
	if !book.usedFragmentAccess["storyline\x00story-1"] {
		t.Fatal("first fragment use was not recorded")
	}

	// Mutating the first result is exactly what the Python consumers do. A
	// second get must not return that already-mutated object when
	// RETAIN_USED_FRAGMENTS is false; Python returns a fresh empty IonStruct.
	delete(first, "story_name")
	first["mutated"] = true
	second := getFragment(book, "storyline", "story-1")
	if second == nil {
		t.Fatal("second use should return an empty struct, not nil")
	}
	if len(second) != 0 {
		t.Fatalf("second use = %#v, want empty struct", second)
	}
	if _, leaked := second["mutated"]; leaked {
		t.Fatal("second use returned the already-mutated first fragment")
	}
}

func TestGetFragmentMissingReturnsEmptyStruct(t *testing.T) {
	book := &decodedBook{
		fragmentMaps:       map[string]map[string]map[string]interface{}{},
		usedFragmentAccess: map[string]bool{},
	}
	got := getFragment(book, "structure", "missing")
	if got == nil || len(got) != 0 {
		t.Fatalf("missing fragment = %#v, want empty struct", got)
	}
}
