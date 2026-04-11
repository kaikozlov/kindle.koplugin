package kfx

import "testing"

func TestRewriteAmznConditionStyleAnchorOpaque(t *testing.T) {
	const fn = "c73.xhtml"
	got := rewriteAmznConditionStyle("-kfx-amzn-condition: anchor-id anchor:c73#frag1", fn)
	want := "-kfx-amzn-condition: anchor-id frag1"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
