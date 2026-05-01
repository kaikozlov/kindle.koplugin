//go:build ignore

package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/kaikozlov/kindle-koplugin/internal/kfx"
)

func main() {
	book, err := kfx.DecodeKFXFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	for _, frag := range book.Fragments {
		if len(frag.RawMedia) > 0 {
			if bytes.HasPrefix(frag.RawMedia, []byte{0xFF, 0xD8, 0xFF}) {
				fmt.Printf("JPEG: %s (size=%d, ftype=%s)\n", frag.ID, len(frag.RawMedia), frag.FType)
			} else if bytes.HasPrefix(frag.RawMedia, []byte{0x89, 0x50, 0x4E, 0x47}) {
				fmt.Printf("PNG: %s (size=%d, ftype=%s)\n", frag.ID, len(frag.RawMedia), frag.FType)
			} else if bytes.HasPrefix(frag.RawMedia, []byte{0x49, 0x49, 0xBC}) || bytes.HasPrefix(frag.RawMedia, []byte{0x4D, 0x4D, 0xBC}) {
				fmt.Printf("JXR: %s (size=%d, ftype=%s)\n", frag.ID, len(frag.RawMedia), frag.FType)
			} else {
				fmt.Printf("OTHER: %s (size=%d, ftype=%s, magic=%x)\n", frag.ID, len(frag.RawMedia), frag.FType, frag.RawMedia[:min(4, len(frag.RawMedia))])
			}
		}
	}
}
func min(a, b int) int {
	if a < b { return a }
	return b
}
