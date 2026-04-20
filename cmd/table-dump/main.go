package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kaikozlov/kindle-koplugin/internal/kfx"
)

func main() {
	f, err := os.Open("REFERENCE/kfx_new/decrypted/Elvis and the Underdogs_B009NG3090_decrypted.kfx-zip")
	if err != nil {
		panic(err)
	}
	state, err := kfx.BuildBookState(f)
	f.Close()
	if err != nil {
		panic(err)
	}

	// Check RubyContents
	fmt.Println("=== RubyContents ===")
	for k, v := range state.Fragments.RubyContents {
		fmt.Printf("  key=%q val=%v\n", k, v)
	}
	
	// Check all raw fragments for "FIRST EDITION"
	fmt.Println("\n=== Raw fragments containing 'FIRST EDITION' ===")
	for name, data := range state.Fragments.RawFragments {
		if strings.Contains(string(data), "FIRST EDITION") {
			fmt.Printf("  name=%s data=%s\n", name, string(data)[:min(200, len(data))])
		}
	}

	// Check storylines for the logo section  
	fmt.Println("\n=== Storylines with cpytxt2 ===")
	_ = sort.Strings
	for name, storyline := range state.Fragments.RawFragments {
		_ = name
		_ = storyline
	}
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
