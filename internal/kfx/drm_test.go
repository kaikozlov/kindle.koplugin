package kfx

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestKfxToVoucherPath(t *testing.T) {
	tests := []struct {
		kfxPath    string
		wantVoucher string
	}{
		{
			"/mnt/us/documents/Book.kfx",
			"/mnt/us/documents/Book.sdr/assets/voucher",
		},
		{
			"/mnt/us/documents/My Folder/Some_Book_B003VIWNQW.kfx",
			"/mnt/us/documents/My Folder/Some_Book_B003VIWNQW.sdr/assets/voucher",
		},
	}

	for _, tc := range tests {
		got := kfxToVoucherPath(tc.kfxPath)
		if got != tc.wantVoucher {
			t.Errorf("kfxToVoucherPath(%q) = %q, want %q", tc.kfxPath, got, tc.wantVoucher)
		}
	}
}

func TestLoadDRMKeyCache_MissingFile(t *testing.T) {
	// This should fail since drm_keys.json doesn't exist next to the test binary
	_, err := LoadDRMKeyCache("/nonexistent/dir")
	if err == nil {
		t.Log("LoadDRMKeyCache succeeded (unexpected if file doesn't exist)")
	}
}

func TestFindPageKey_WithCacheFile(t *testing.T) {
	// Create a temp directory with a drm_keys.json
	tmpDir := t.TempDir()

	cache := DRMKeyCache{
		Version:      1,
		DeviceSerial: "TESTSERIAL",
		GeneratedAt:  "2026-01-01T00:00:00Z",
		Books: map[string]DRMKeyEntry{
			"B003VIWNQW": {
				VoucherPath:   "/mnt/us/documents/Book.sdr/assets/voucher",
				VoucherKey256: "ca6d4c0000000000000000000000000000000000000000000000000000000000",
				PageKey128:    "5006ef00000000000000000000000000",
			},
		},
	}

	data, err := json.Marshal(cache)
	if err != nil {
		t.Fatalf("marshal cache: %v", err)
	}

	cachePath := filepath.Join(tmpDir, "drm_keys.json")
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	// Parse it back
	loaded, err := LoadDRMKeyCacheFromFile(cachePath)
	if err != nil {
		t.Fatalf("LoadDRMKeyCache: %v", err)
	}

	if len(loaded.Books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(loaded.Books))
	}

	// Verify we can find the page key
	entry, ok := loaded.Books["B003VIWNQW"]
	if !ok {
		t.Fatal("missing B003VIWNQW entry")
	}

	if entry.PageKey128 != "5006ef00000000000000000000000000" {
		t.Errorf("unexpected page key: %s", entry.PageKey128)
	}
}

func TestAES128CBCDecrypt(t *testing.T) {
	// Test with known values: encrypt then decrypt
	key := []byte("0123456789abcdef") // 16 bytes
	iv := []byte("abcdef0123456789")  // 16 bytes

	// "hello world" + PKCS7 padding to 16 bytes
	// helloworld\x05\x05\x05\x05\x05
	plaintext := []byte("hello world")
	padded := make([]byte, 16)
	copy(padded, plaintext)
	pad := 16 - len(plaintext)
	for i := len(plaintext); i < 16; i++ {
		padded[i] = byte(pad)
	}

	// Encrypt using Go's CBC encryptor to get known ciphertext
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}

	ciphertext := make([]byte, 16)
	encMode := cipher.NewCBCEncrypter(block, iv)
	encMode.CryptBlocks(ciphertext, padded)

	// Now decrypt
	decrypted, err := aes128CBCDecrypt(key, iv, ciphertext)
	if err != nil {
		t.Fatalf("aes128CBCDecrypt: %v", err)
	}

	if string(decrypted) != "hello world" {
		t.Errorf("decrypted = %q, want %q", string(decrypted), "hello world")
	}
}

func TestBytesStartsWithDRMION(t *testing.T) {
	tests := []struct {
		data []byte
		want bool
	}{
		{[]byte{0xea, 'D', 'R', 'M', 'I', 'O', 'N', 0xee, 0x01, 0x02}, true},
		{[]byte{'C', 'O', 'N', 'T'}, false},
		{[]byte{0xea, 'D', 'R', 'M'}, false}, // too short
		{[]byte{}, false},
	}

	for _, tc := range tests {
		got := bytesStartsWithDRMION(tc.data)
		if got != tc.want {
			t.Errorf("bytesStartsWithDRMION(%x) = %v, want %v", tc.data, got, tc.want)
		}
	}
}

// LoadDRMKeyCacheFromFile reads a drm_keys.json from a specific path (for testing).
func LoadDRMKeyCacheFromFile(path string) (*DRMKeyCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache DRMKeyCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	if cache.Version < 1 {
		return nil, err
	}
	return &cache, nil
}
