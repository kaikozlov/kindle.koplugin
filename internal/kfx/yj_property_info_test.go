package kfx

import "testing"

func TestYJPropertySimpleCSSCoreKeys(t *testing.T) {
	for _, key := range []string{"$11", "$16", "$19", "$21", "$479"} {
		if _, ok := YJPropertySimpleCSSName(key); !ok {
			t.Fatalf("missing simple CSS mapping for %s", key)
		}
	}
	if n := len(yjPropertySimpleCSS); n < 35 {
		t.Fatalf("yjPropertySimpleCSS size = %d want >= 40", n)
	}
}
