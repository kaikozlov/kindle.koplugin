package drm

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCapturedKeys(t *testing.T) {
	logContent := `EVP_256_KEY:ca6d4c0102030405060708090a0b0c0d0e0f1011121314151617181920212223 IV:aabbccdd0102030405060708090a0b0c0d0e0f10
EVP_256_KEY:6533356635000000000000000000000000000000000000000000000000000000 IV:00000000000000000000000000000000
EVP_256_KEY:bb2233ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433 IV:11111111111111111111111111111111
`
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "crypto_keys.log")
	if err := os.WriteFile(logPath, []byte(logContent), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	keys, err := parseCapturedKeys(logPath)
	if err != nil {
		t.Fatalf("parseCapturedKeys: %v", err)
	}

	// Should have 2 keys (shared metadata key skipped)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	// First key
	if keys[0].Key == nil || len(keys[0].Key) != 32 {
		t.Errorf("first key length = %d, want 32", len(keys[0].Key))
	}
	if hex.EncodeToString(keys[0].Key) != "ca6d4c0102030405060708090a0b0c0d0e0f1011121314151617181920212223" {
		t.Errorf("first key mismatch: %s", hex.EncodeToString(keys[0].Key))
	}

	// Second key
	if keys[1].Key == nil || len(keys[1].Key) != 32 {
		t.Errorf("second key length = %d, want 32", len(keys[1].Key))
	}
}

func TestParseCapturedKeys_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "crypto_keys.log")
	os.WriteFile(logPath, []byte(""), 0644)

	keys, err := parseCapturedKeys(logPath)
	if err != nil {
		t.Fatalf("parseCapturedKeys: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestDeriveBookID(t *testing.T) {
	tests := []struct {
		voucherPath string
		wantID      string
	}{
		{
			"/mnt/us/documents/The_Familiars_B003VIWNQW.sdr/assets/voucher",
			"B003VIWNQW",
		},
		{
			"/mnt/us/documents/Elvis_B009NG3090.sdr/assets/voucher",
			"B009NG3090",
		},
		{
			"/mnt/us/documents/MyBook.sdr/assets/voucher",
			"MyBook",
		},
	}

	for _, tc := range tests {
		got := deriveBookID(tc.voucherPath)
		if got != tc.wantID {
			t.Errorf("deriveBookID(%q) = %q, want %q", tc.voucherPath, got, tc.wantID)
		}
	}
}

func TestFindVouchers(t *testing.T) {
	tmpDir := t.TempDir()

	// Create fake voucher structure
	voucherDir := filepath.Join(tmpDir, "Book_B0012345678.sdr", "assets")
	os.MkdirAll(voucherDir, 0755)
	os.WriteFile(filepath.Join(voucherDir, "voucher"), []byte("fake"), 0644)

	// Create a non-voucher file
	os.WriteFile(filepath.Join(tmpDir, "other.txt"), []byte("nope"), 0644)

	// Create SDR without voucher
	otherSdr := filepath.Join(tmpDir, "Other_B0099988776.sdr", "assets")
	os.MkdirAll(otherSdr, 0755)
	os.WriteFile(filepath.Join(otherSdr, "metadata.kfx"), []byte("meta"), 0644)

	vouchers, err := findVouchers(tmpDir)
	if err != nil {
		t.Fatalf("findVouchers: %v", err)
	}

	if len(vouchers) != 1 {
		t.Fatalf("expected 1 voucher, got %d", len(vouchers))
	}

	if !strings.HasSuffix(vouchers[0], "Book_B0012345678.sdr/assets/voucher") {
		t.Errorf("unexpected voucher path: %s", vouchers[0])
	}
}

func TestBytesIndex(t *testing.T) {
	tests := []struct {
		data    []byte
		sub     []byte
		wantIdx int
	}{
		{[]byte("hello world"), []byte("world"), 6},
		{[]byte("hello world"), []byte("hello"), 0},
		{[]byte("hello world"), []byte("xyz"), -1},
		{[]byte("RAW"), []byte("RAW"), 0},
		{[]byte{0x01, 0x02, 0x03, 0x04}, []byte{0x03, 0x04}, 2},
	}

	for _, tc := range tests {
		got := bytesIndex(tc.data, tc.sub)
		if got != tc.wantIdx {
			t.Errorf("bytesIndex(%x, %x) = %d, want %d", tc.data, tc.sub, got, tc.wantIdx)
		}
	}
}

func TestIsAlphanumeric(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"B003VIWNQW", true},
		{"abc123XYZ", true},
		{"has space", false},
		{"has-dash", false},
		{"", true}, // empty string is trivially alphanumeric
	}

	for _, tc := range tests {
		got := isAlphanumeric(tc.s)
		if got != tc.want {
			t.Errorf("isAlphanumeric(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}
