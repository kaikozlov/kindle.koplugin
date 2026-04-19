package drm

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/kaikozlov/kindle-koplugin/internal/kfx"
)

// drmSymbolNames for voucher parsing (same set as kfx/drm.go but duplicated
// here to avoid circular import; the voucher parser only needs the subset
// used by VoucherEnvelope/Voucher structures).
var voucherSymbolNames = []string{
	"com.amazon.drm.Envelope@1.0",
	"com.amazon.drm.EnvelopeMetadata@1.0", "size", "page_size",
	"encryption_key", "encryption_transformation",
	"encryption_voucher", "signing_key", "signing_algorithm",
	"signing_voucher", "com.amazon.drm.EncryptedPage@1.0",
	"cipher_text", "cipher_iv", "com.amazon.drm.Signature@1.0",
	"data", "com.amazon.drm.EnvelopeIndexTable@1.0", "length",
	"offset", "algorithm", "encoded", "encryption_algorithm",
	"hashing_algorithm", "expires", "format", "id",
	"lock_parameters", "strategy", "com.amazon.drm.Key@1.0",
	"com.amazon.drm.KeySet@1.0", "com.amazon.drm.PIDv3@1.0",
	"com.amazon.drm.PlainTextPage@1.0",
	"com.amazon.drm.PlainText@1.0", "com.amazon.drm.PrivateKey@1.0",
	"com.amazon.drm.PublicKey@1.0", "com.amazon.drm.SecretKey@1.0",
	"com.amazon.drm.Voucher@1.0", "public_key", "private_key",
	"com.amazon.drm.KeyPair@1.0", "com.amazon.drm.ProtectedData@1.0",
	"doctype", "com.amazon.drm.EnvelopeIndexTableOffset@1.0",
	"enddoc", "license_type", "license", "watermark", "key", "value",
	"com.amazon.drm.License@1.0", "category", "metadata",
	"categorized_metadata", "com.amazon.drm.CategorizedMetadata@1.0",
	"com.amazon.drm.VoucherEnvelope@1.0", "mac", "voucher",
	"com.amazon.drm.ProtectedData@2.0",
	"com.amazon.drm.Envelope@2.0",
	"com.amazon.drm.EnvelopeMetadata@2.0",
	"com.amazon.drm.EncryptedPage@2.0",
	"com.amazon.drm.PlainText@2.0", "compression_algorithm",
	"com.amazon.drm.Compressed@1.0", "page_index_table",
}

// Shared metadata key prefix — keys starting with this are the shared
// device metadata key, not per-book voucher keys.
const sharedMetadataKeyPrefix = "6533356635"

// aesKeyPattern matches lines like: EVP_256_KEY:ca6d4c... IV:abcdef...
var aesKeyPattern = regexp.MustCompile(`^EVP_256_KEY:([0-9a-f]+)\s+IV:([0-9a-f]+)`)

// InitResult holds the outcome of a drm-init run.
type InitResult struct {
	BooksFound int
	KeysFound  int
	KeyCache   *kfx.DRMKeyCache
}

// Run executes the drm-init workflow:
//  1. Scan for voucher files under the given documents root
//  2. Read device serial from /proc/usid
//  3. Shell out to device's cvm with LD_PRELOAD hook to capture AES keys
//  4. Parse captured keys, decrypt vouchers, extract page keys
//  5. Write drm_keys.json cache
//
// pluginDir is the kindle.koplugin directory containing lib/ helpers.
func Run(documentsRoot string, pluginDir string, cacheDir string) (*InitResult, error) {
	// Step 1: Find voucher files
	vouchers, err := findVouchers(documentsRoot)
	if err != nil {
		return nil, fmt.Errorf("scan vouchers: %w", err)
	}
	if len(vouchers) == 0 {
		return &InitResult{}, nil
	}

	// Step 2: Read device serial
	serial, err := readDeviceSerial()
	if err != nil {
		return nil, fmt.Errorf("read serial: %w", err)
	}

	// Step 3: Run the Java extractor with LD_PRELOAD hook to capture keys
	if err := extractKeysWithHook(serial, vouchers, pluginDir); err != nil {
		return nil, fmt.Errorf("key extraction: %w", err)
	}

	// Step 4: Parse captured AES keys from the log
	keys, err := parseCapturedKeys("/mnt/us/crypto_keys.log")
	if err != nil {
		return nil, fmt.Errorf("parse keys: %w", err)
	}

	// Step 5: Decrypt vouchers and extract page keys
	cache := &kfx.DRMKeyCache{
		Version:      1,
		DeviceSerial: serial,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Books:        make(map[string]kfx.DRMKeyEntry),
	}

	keysFound := 0
	for _, voucherPath := range vouchers {
		voucherKey, err := findVoucherKey(voucherPath, keys)
		if err != nil {
			fmt.Fprintf(os.Stderr, "drm-init: skipping %s: %v\n", voucherPath, err)
			continue
		}

		pageKey, err := extractPageKeyFromVoucher(voucherPath, voucherKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "drm-init: page key extraction failed for %s: %v\n", voucherPath, err)
			continue
		}

		// Derive book ID from the voucher's parent directory name
		bookID := deriveBookID(voucherPath)

		// Prefer non-tmp vouchers over tmp_ ones
		if existing, ok := cache.Books[bookID]; ok {
			newIsTmp := strings.Contains(voucherPath, "tmp_")
			existingIsTmp := strings.Contains(existing.VoucherPath, "tmp_")
			if existingIsTmp && !newIsTmp {
				// New is better, overwrite below
			} else if !existingIsTmp && newIsTmp {
				fmt.Fprintf(os.Stderr, "drm-init: skipping tmp voucher %s (already have %s)\n", voucherPath, existing.VoucherPath)
				continue
			}
		}

		cache.Books[bookID] = kfx.DRMKeyEntry{
			VoucherPath:   voucherPath,
			VoucherKey256: hex.EncodeToString(voucherKey),
			PageKey128:    hex.EncodeToString(pageKey),
		}
		keysFound++
	}

	// Step 6: Write the cache file next to the plugin binary
	cachePath := kfx.DRMKeyCachePath(cacheDir)
	cacheData, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal cache: %w", err)
	}
	if err := os.WriteFile(cachePath, cacheData, 0644); err != nil {
		return nil, fmt.Errorf("write cache: %w", err)
	}

	// Step 7: Clean up the key log
	os.Remove("/mnt/us/crypto_keys.log")

	return &InitResult{
		BooksFound: len(vouchers),
		KeysFound:  keysFound,
		KeyCache:   cache,
	}, nil
}

// findVouchers walks the documents root looking for voucher files.
func findVouchers(root string) ([]string, error) {
	var vouchers []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == "voucher" {
			// Verify it's under an assets/ directory
			if strings.Contains(filepath.Dir(path), "assets") {
				vouchers = append(vouchers, path)
			}
		}
		return nil
	})
	return vouchers, err
}

// readDeviceSerial reads the Kindle serial from /proc/usid.
func readDeviceSerial() (string, error) {
	data, err := os.ReadFile("/proc/usid")
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\x00\n"), nil
}

// extractKeysWithHook runs the device's cvm JVM with the LD_PRELOAD hook
// to capture AES keys used during voucher processing.
func extractKeysWithHook(serial string, vouchers []string, pluginDir string) error {
	hookPath := filepath.Join(pluginDir, "lib", "crypto_hook.so")
	jarPath := filepath.Join(pluginDir, "lib", "KFXVoucherExtractor.jar")

	// Verify helper files exist
	if _, err := os.Stat(hookPath); err != nil {
		return fmt.Errorf("crypto_hook.so not found: %w", err)
	}
	if _, err := os.Stat(jarPath); err != nil {
		return fmt.Errorf("KFXVoucherExtractor.jar not found: %w", err)
	}

	// Clear the key log
	os.Remove("/mnt/us/crypto_keys.log")

	// Build voucher arguments
	voucherArgs := make([]string, len(vouchers))
	copy(voucherArgs, vouchers)

	args := []string{serial}
	args = append(args, voucherArgs...)

	cmd := exec.Command("/usr/java/bin/cvm",
		"-Djava.library.path=/usr/lib:/usr/java/lib",
		"-cp", jarPath+":/opt/amazon/ebook/lib/YJReader-impl.jar",
		"KFXVoucherExtractor",
	)
	cmd.Args = append(cmd.Args, args...)
	cmd.Env = append(os.Environ(),
		"LD_PRELOAD="+hookPath+":/usr/java/lib/arm/libdlopen_global.so",
		"LD_LIBRARY_PATH=/usr/lib:/usr/java/lib",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cvm failed: %w\noutput: %s", err, string(output))
	}

	// Verify we got "All vouchers attached" in output
	if !strings.Contains(string(output), "All vouchers attached") {
		return fmt.Errorf("voucher extraction may have failed: %s", string(output))
	}

	return nil
}

// CapturedKey holds an AES key and optional IV captured from the hook log.
type CapturedKey struct {
	Key []byte
	IV  []byte
}

// parseCapturedKeys reads the crypto_keys.log and extracts AES-256 keys.
func parseCapturedKeys(logPath string) ([]CapturedKey, error) {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil, err
	}

	var keys []CapturedKey
	for _, line := range strings.Split(string(data), "\n") {
		m := aesKeyPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		keyHex := m[1]
		ivHex := m[2]

		// Skip the shared metadata key
		if strings.HasPrefix(keyHex, sharedMetadataKeyPrefix) {
			continue
		}

		key, err := hex.DecodeString(keyHex)
		if err != nil {
			continue
		}

		var iv []byte
		if ivHex != "none" {
			iv, _ = hex.DecodeString(ivHex)
		}

		keys = append(keys, CapturedKey{Key: key, IV: iv})
	}

	return keys, nil
}

// findVoucherKey finds the non-shared AES-256 key that decrypts the given voucher.
// Since the hook captures keys for all vouchers processed, we try each key.
func findVoucherKey(voucherPath string, keys []CapturedKey) ([]byte, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("no captured keys available")
	}

	// For simplicity, return the first non-shared key.
	// In practice, the hook captures keys in order of voucher processing,
	// and each voucher produces exactly one unique AES-256 key.
	// If there's only one voucher, there's one key.
	// If there are multiple, we need to try each key against the voucher.
	if len(keys) == 1 {
		return keys[0].Key, nil
	}

	// Try each key to find which one successfully decrypts the voucher
	voucherData, err := os.ReadFile(voucherPath)
	if err != nil {
		return nil, fmt.Errorf("read voucher: %w", err)
	}

	for _, captured := range keys {
		if len(captured.Key) != 32 {
			continue
		}
		if tryDecryptVoucher(voucherData, captured.Key) {
			return captured.Key, nil
		}
	}

	return nil, fmt.Errorf("no matching key found for %s", voucherPath)
}

// tryDecryptVoucher attempts to decrypt a voucher with the given AES-256 key
// and returns true if it succeeds (finds a KeySet in the result).
func tryDecryptVoucher(voucherData []byte, aes256Key []byte) bool {
	_, err := extractPageKeyFromVoucherData(voucherData, aes256Key)
	return err == nil
}

// extractPageKeyFromVoucher reads a voucher file, decrypts it with the AES-256 key,
// and extracts the 16-byte page key.
func extractPageKeyFromVoucher(voucherPath string, aes256Key []byte) ([]byte, error) {
	voucherData, err := os.ReadFile(voucherPath)
	if err != nil {
		return nil, fmt.Errorf("read voucher: %w", err)
	}

	return extractPageKeyFromVoucherData(voucherData, aes256Key)
}

// extractPageKeyFromVoucherData decrypts voucher data and extracts the 16-byte page key.
func extractPageKeyFromVoucherData(voucherData []byte, aes256Key []byte) ([]byte, error) {
	// Parse the VoucherEnvelope ION structure
	cat := ion.NewCatalog(ion.NewSharedSymbolTable("ProtectedData", 1, voucherSymbolNames))
	sys := ion.System{Catalog: cat}
	r := sys.NewReaderBytes(voucherData)

	// Find ciphertext and IV from the voucher
	ciphertext, cipherIV, err := parseVoucherEnvelope(r)
	if err != nil {
		return nil, err
	}

	// Decrypt with AES-256-CBC
	block, err := aes.NewCipher(aes256Key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext not aligned to block size")
	}

	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, cipherIV[:16])
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS7 padding
	if len(plaintext) == 0 {
		return nil, fmt.Errorf("empty plaintext")
	}
	pad := int(plaintext[len(plaintext)-1])
	if pad == 0 || pad > aes.BlockSize {
		return nil, fmt.Errorf("invalid PKCS7 padding")
	}
	plaintext = plaintext[:len(plaintext)-pad]

	// Find page key: look for "RAW" marker
	// After "RAW": 9d ae 90 <16-byte key> (ION annotation header + blob)
	rawIdx := bytesIndex(plaintext, []byte("RAW"))
	if rawIdx < 0 {
		return nil, fmt.Errorf("no RAW marker found in decrypted voucher")
	}

	// Skip past "RAW" (3 bytes) + ION annotation header (typically 3-6 bytes)
	// The Python code does: key_offset = raw_pos + 6
	// This accounts for "RAW" + 3 bytes of ION type descriptor
	keyOffset := rawIdx + 6
	if keyOffset+16 > len(plaintext) {
		return nil, fmt.Errorf("not enough data after RAW marker for page key")
	}

	pageKey := plaintext[keyOffset : keyOffset+16]
	return pageKey, nil
}

// parseVoucherEnvelope extracts ciphertext and cipher_iv from a VoucherEnvelope ION structure.
func parseVoucherEnvelope(r ion.Reader) (ciphertext, cipherIV []byte, err error) {
	if !r.Next() {
		return nil, nil, fmt.Errorf("empty voucher envelope")
	}

	// Expect a struct with VoucherEnvelope annotation
	annot := getAnnotationName(r)
	if !strings.HasPrefix(annot, "com.amazon.drm.VoucherEnvelope@") {
		// Might not have the annotation; try to proceed anyway
	}

	if r.Type() != ion.StructType {
		return nil, nil, fmt.Errorf("expected struct, got %v", r.Type())
	}

	if err := r.StepIn(); err != nil {
		return nil, nil, fmt.Errorf("step in: %w", err)
	}

	var innerVoucherData []byte

	for r.Next() {
		field := getFieldNameText(r)
		switch field {
		case "voucher":
			bv, err := r.ByteValue()
			if err != nil {
				return nil, nil, fmt.Errorf("read voucher: %w", err)
			}
			innerVoucherData = bv
		case "strategy":
			// Skip strategy section
		}
	}

	if err := r.StepOut(); err != nil {
		return nil, nil, fmt.Errorf("step out: %w", err)
	}

	if len(innerVoucherData) == 0 {
		return nil, nil, fmt.Errorf("voucher envelope has no inner voucher")
	}

	// Parse the inner voucher to get ciphertext and cipher_iv
	return parseInnerVoucher(innerVoucherData)
}

// parseInnerVoucher parses the com.amazon.drm.Voucher ION structure to extract ciphertext and IV.
func parseInnerVoucher(data []byte) (ciphertext, cipherIV []byte, err error) {
	cat := ion.NewCatalog(ion.NewSharedSymbolTable("ProtectedData", 1, voucherSymbolNames))
	sys := ion.System{Catalog: cat}
	r := sys.NewReaderBytes(data)

	if !r.Next() {
		return nil, nil, fmt.Errorf("empty voucher")
	}

	if r.Type() != ion.StructType {
		return nil, nil, fmt.Errorf("expected struct, got %v", r.Type())
	}

	if err := r.StepIn(); err != nil {
		return nil, nil, fmt.Errorf("step in: %w", err)
	}

	for r.Next() {
		field := getFieldNameText(r)
		switch field {
		case "cipher_text":
			bv, err := r.ByteValue()
			if err != nil {
				return nil, nil, fmt.Errorf("read cipher_text: %w", err)
			}
			ciphertext = bv
		case "cipher_iv":
			bv, err := r.ByteValue()
			if err != nil {
				return nil, nil, fmt.Errorf("read cipher_iv: %w", err)
			}
			cipherIV = bv
		}
	}

	if err := r.StepOut(); err != nil {
		return nil, nil, fmt.Errorf("step out: %w", err)
	}

	if ciphertext == nil {
		return nil, nil, fmt.Errorf("voucher missing cipher_text")
	}
	if cipherIV == nil {
		return nil, nil, fmt.Errorf("voucher missing cipher_iv")
	}

	return ciphertext, cipherIV, nil
}

// getAnnotationName returns the first annotation text for the current value.
func getAnnotationName(r ion.Reader) string {
	annots, err := r.Annotations()
	if err != nil || len(annots) == 0 {
		return ""
	}
	if annots[0].Text != nil {
		return *annots[0].Text
	}
	return ""
}

// getFieldNameText returns the field name text for the current value.
func getFieldNameText(r ion.Reader) string {
	fn, err := r.FieldName()
	if err != nil || fn == nil {
		return ""
	}
	if fn.Text != nil {
		return *fn.Text
	}
	return ""
}

// deriveBookID extracts a book identifier from a voucher path.
// E.g., /mnt/us/documents/Book_B003VIWNQW.sdr/assets/voucher → B003VIWNQW
func deriveBookID(voucherPath string) string {
	// Walk up: voucher → assets → <name>.sdr
	dir := filepath.Dir(filepath.Dir(voucherPath))
	base := filepath.Base(dir)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// Try to extract ASIN-like ID from trailing pattern
	parts := strings.Split(name, "_")
	if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if len(last) == 10 && isAlphanumeric(last) {
			return last
		}
	}

	return name
}

func isAlphanumeric(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
			return false
		}
	}
	return true
}

// bytesIndex finds the index of sub in data, or -1.
func bytesIndex(data, sub []byte) int {
	for i := 0; i <= len(data)-len(sub); i++ {
		if equalBytes(data[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ReadStream is a helper for reading all data from an io.Reader.
func ReadStream(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
