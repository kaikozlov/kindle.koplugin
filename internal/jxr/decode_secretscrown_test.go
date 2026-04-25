package jxr

import (
	"fmt"
	"os"
	"testing"
)

func TestDecodeSecretsCrownJXR(t *testing.T) {
	path := "/tmp/OEBPS/image_16-resized-702-1027.jxr"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("file not found: %s", path)
	}
	img, err := DecodeGray8(data)
	if err != nil {
		t.Fatalf("DecodeGray8 error: %v", err)
	}
	if img == nil {
		t.Fatal("nil image")
	}
	fmt.Printf("Decoded: bounds=%v\n", img.Bounds())
}
