package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// PositionResult is the JSON output of the position command.
type PositionResult struct {
	Version int    `json:"version"`
	OK      bool   `json:"ok"`
	ERL     string `json:"erl,omitempty"`
	Message string `json:"message,omitempty"`
}

func cmdPosition(args []string) {
	fs := flag.NewFlagSet("position", flag.ExitOnError)
	yjrPath := fs.String("yjr", "", "path to .yjr sidecar file")
	oldPercent := fs.Float64("old-percent", -1, "old reading percentage from cc.db (0-100)")
	newPercent := fs.Float64("new-percent", -1, "new reading percentage from KOReader (0-100)")
	fs.Parse(args)

	if *yjrPath == "" {
		fmt.Fprintf(os.Stderr, "position: --yjr is required\n")
		os.Exit(2)
	}
	if *oldPercent < 0 || *newPercent < 0 {
		fmt.Fprintf(os.Stderr, "position: --old-percent and --new-percent are required\n")
		os.Exit(2)
	}

	data, err := os.ReadFile(*yjrPath)
	if err != nil {
		writeJSON(PositionResult{Version: version, OK: false, Message: err.Error()})
		os.Exit(1)
	}

	// Find the LAST erl value (the reading position, not bookmarks)
	erlKey := []byte{0xff, 0xfe, 0x00, 0x00, 0x03, 'e', 'r', 'l'}
	erlIdx := -1
	for i := 0; i <= len(data)-len(erlKey); i++ {
		match := true
		for j := 0; j < len(erlKey); j++ {
			if data[i+j] != erlKey[j] {
				match = false
				break
			}
		}
		if match {
			erlIdx = i
		}
	}
	if erlIdx < 0 {
		writeJSON(PositionResult{Version: version, OK: false, Message: "erl key not found in .yjr file"})
		os.Exit(1)
	}

	// Parse the erl value header: 03 XX XX XX <string_bytes>
	valStart := erlIdx + len(erlKey)
	if valStart+4 >= len(data) || data[valStart] != 0x03 {
		writeJSON(PositionResult{Version: version, OK: false, Message: "erl value has unexpected format"})
		os.Exit(1)
	}
	oldLen := int(data[valStart+1])<<16 | int(data[valStart+2])<<8 | int(data[valStart+3])
	if valStart+4+oldLen > len(data) {
		writeJSON(PositionResult{Version: version, OK: false, Message: "erl value truncated"})
		os.Exit(1)
	}
	oldErl := string(data[valStart+4 : valStart+4+oldLen])
	fmt.Fprintf(os.Stderr, "KindlePlugin: existing erl: %s\n", oldErl)

	// Parse the existing erl: base64part:position
	parts := strings.SplitN(oldErl, ":", 2)
	if len(parts) != 2 {
		writeJSON(PositionResult{Version: version, OK: false, Message: "erl value has no colon separator"})
		os.Exit(1)
	}

	decoded, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil || len(decoded) != 9 || decoded[0] != 0x01 {
		writeJSON(PositionResult{Version: version, OK: false, Message: "erl base64 decode failed"})
		os.Exit(1)
	}

	oldPosition, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		writeJSON(PositionResult{Version: version, OK: false, Message: fmt.Sprintf("erl position parse error: %v", err)})
		os.Exit(1)
	}

	// Calculate new position by scaling
	var newPosition int64
	if *oldPercent == 0 {
		// Book was at 0%, can't scale. Use the new percent directly with a rough estimate.
		// This shouldn't happen in practice — we only sync for books that have been read.
		writeJSON(PositionResult{Version: version, OK: true, ERL: oldErl, Message: "old percent is 0, cannot scale"})
		return
	}

	// Scale: new_pos = old_pos * (new_pct / old_pct)
	ratio := *newPercent / *oldPercent
	newPosition = int64(float64(oldPosition) * ratio)
	if newPosition < 0 {
		newPosition = 0
	}

	// Build new erl: same EID+offset (base64 part), new position
	newErl := parts[0] + ":" + strconv.FormatInt(newPosition, 10)
	fmt.Fprintf(os.Stderr, "KindlePlugin: new erl: %s (scaled %.2f%% → %.2f%%)\n", newErl, *oldPercent, *newPercent)

	// Build replacement bytes
	newErlBytes := []byte(newErl)
	newLen := len(newErlBytes)
	erlValueHeader := []byte{0x03, byte(newLen >> 16), byte(newLen >> 8), byte(newLen)}
	oldEnd := valStart + 4 + oldLen

	// Replace in data
	var result []byte
	result = append(result, data[:valStart]...)
	result = append(result, erlValueHeader...)
	result = append(result, newErlBytes...)
	result = append(result, data[oldEnd:]...)

	// Ensure sync_lpr is set to 1 (enable last position sync)
	syncKey := []byte{0xff, 0xfe, 0x00, 0x00, 0x08, 's', 'y', 'n', 'c', '_', 'l', 'p', 'r'}
	syncIdx := findBytes(result, syncKey)
	if syncIdx >= 0 {
		valOff := syncIdx + len(syncKey)
		if valOff+1 < len(result) {
			result[valOff] = 0x00
			result[valOff+1] = 0x01
		}
	}

	// Write back
	err = os.WriteFile(*yjrPath, result, 0644)
	if err != nil {
		writeJSON(PositionResult{Version: version, OK: false, Message: err.Error()})
		os.Exit(1)
	}

	writeJSON(PositionResult{
		Version: version,
		OK:      true,
		ERL:     newErl,
	})
}

func findBytes(data, pattern []byte) int {
	for i := 0; i <= len(data)-len(pattern); i++ {
		match := true
		for j := 0; j < len(pattern); j++ {
			if data[i+j] != pattern[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
