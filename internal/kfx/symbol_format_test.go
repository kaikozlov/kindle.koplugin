package kfx

import "testing"

func TestClassifySymbolCommonAndOriginal(t *testing.T) {
	if g := classifySymbol("yj.conversion.html_name"); g != symCommon {
		t.Fatalf("yj meta = %v want common", g)
	}
	// Calibre SHORT pattern matches compact ids like "c73".
	if g := classifySymbol("c73"); g != symShort {
		t.Fatalf("compact id = %v want short", g)
	}
	if g := classifySymbol("00000000-0000-0000-0000-000000000000"); g != symCommon {
		t.Fatalf("uuid literal = %v want common", g)
	}
	if g := classifySymbol("V_0_0-PARA-0_0_1234567890123_ab"); g != symOriginal {
		t.Fatalf("V_ style = %v want original", g)
	}
}

func TestDetermineBookSymbolFormatEmpty(t *testing.T) {
	got := determineBookSymbolFormat(map[string]struct{}{}, nil, nil)
	if got != symShort {
		t.Fatalf("empty book = %v (Python quorum 0 favors SHORT)", got)
	}
}

func TestClassifySymbolSharedNumeric(t *testing.T) {
	r := &symbolResolver{localStart: 1000, locals: []string{"localSym"}}
	if g := classifySymbolWithResolver("$50", r); g != symShared {
		t.Fatalf("$50 with localStart=1000 = %v want shared", g)
	}
	if g := classifySymbolWithResolver("c73", r); g != symShort {
		t.Fatalf("c73 = %v want short", g)
	}
}
