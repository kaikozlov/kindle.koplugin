package kfx

import (
	"bytes"
	"github.com/ulikunitz/xz/lzma"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/amazon-ion/ion-go/ion"
)

// DRMKeyEntry holds the cached decryption data for a single DRM book.
type DRMKeyEntry struct {
	VoucherPath   string `json:"voucher_path"`
	VoucherKey256 string `json:"voucher_key_256"`
	PageKey128    string `json:"page_key_128"`
}

// DRMKeyCache is the on-disk cache of decrypted page keys.
type DRMKeyCache struct {
	Version      int                    `json:"version"`
	DeviceSerial string                 `json:"device_serial"`
	GeneratedAt  string                 `json:"generated_at"`
	Books        map[string]DRMKeyEntry `json:"books"`
}

// drmSymbolNames are the shared symbol table entries used by Amazon DRM ION structures.
// These match the SYM_NAMES list from DeDRM_tools/ion.py.
var drmSymbolNames = []string{
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

// newDRMCatalog creates an ion.Catalog pre-loaded with the DRM shared symbol table.
func newDRMCatalog() ion.Catalog {
	sst := ion.NewSharedSymbolTable("ProtectedData", 1, drmSymbolNames)
	return ion.NewCatalog(sst)
}

// FindPageKey looks up the 16-byte page key for a KFX file by reading the
// drm_keys.json cache and matching the book by its voucher path or ASIN.
func FindPageKey(kfxPath string, cacheDir string) ([]byte, error) {
	cache, err := LoadDRMKeyCache(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("drm key cache: %w", err)
	}

	voucherPath := kfxToVoucherPath(kfxPath)

	// First try exact match
	for _, entry := range cache.Books {
		if entry.VoucherPath == voucherPath {
			key, err := hex.DecodeString(entry.PageKey128)
			if err != nil {
				return nil, fmt.Errorf("invalid page key hex for %s: %w", voucherPath, err)
			}
			if len(key) != 16 {
				return nil, fmt.Errorf("page key is %d bytes, expected 16", len(key))
			}
			return key, nil
		}
	}

	// Fall back to ASIN-based matching (keys may be cached with different voucher paths)
	asin := extractASINFromPath(kfxPath)
	if asin != "" {
		for bookASIN, entry := range cache.Books {
			if bookASIN == asin {
				key, err := hex.DecodeString(entry.PageKey128)
				if err != nil {
					return nil, fmt.Errorf("invalid page key hex for ASIN %s: %w", asin, err)
				}
				if len(key) != 16 {
					return nil, fmt.Errorf("page key is %d bytes, expected 16", len(key))
				}
				return key, nil
			}
		}
	}

	return nil, fmt.Errorf("no page key cached for %s (voucher: %s)", kfxPath, voucherPath)
}

// extractASINFromPath extracts an Amazon ASIN (B-prefixed 10-char alphanumeric)
// from a file path like /mnt/us/documents/.../BookTitle_B003VIWNQW.kfx
func extractASINFromPath(path string) string {
	base := filepath.Base(path)
	re := regexp.MustCompile(`_(B[0-9A-Z]{9})[._]`)
	m := re.FindStringSubmatch(base)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// LoadDRMKeyCache reads the drm_keys.json file from the cache directory.
func LoadDRMKeyCache(cacheDir string) (*DRMKeyCache, error) {
	path := DRMKeyCachePath(cacheDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cache DRMKeyCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if cache.Version < 1 {
		return nil, fmt.Errorf("invalid drm_keys.json version %d", cache.Version)
	}

	return &cache, nil
}

// DRMKeyCachePath returns the path to the drm_keys.json cache file
// inside the given cache directory.
func DRMKeyCachePath(cacheDir string) string {
	if cacheDir == "" {
		return "drm_keys.json"
	}
	return filepath.Join(cacheDir, "drm_keys.json")
}

// kfxToVoucherPath converts a .kfx file path to the expected voucher path.
func kfxToVoucherPath(kfxPath string) string {
	ext := filepath.Ext(kfxPath)
	base := strings.TrimSuffix(kfxPath, ext)
	return base + ".sdr/assets/voucher"
}

// aes128CBCDecrypt decrypts data using AES-128-CBC with the given key and IV,
// then strips PKCS7 padding.
func aes128CBCDecrypt(key, iv, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length %d is not a multiple of block size", len(ciphertext))
	}

	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS7 padding
	if len(plaintext) == 0 {
		return plaintext, nil
	}
	pad := int(plaintext[len(plaintext)-1])
	if pad == 0 || pad > aes.BlockSize {
		return nil, fmt.Errorf("invalid PKCS7 padding value %d", pad)
	}
	for i := len(plaintext) - pad; i < len(plaintext); i++ {
		if int(plaintext[i]) != pad {
			return nil, fmt.Errorf("invalid PKCS7 padding")
		}
	}

	return plaintext[:len(plaintext)-pad], nil
}

// getAnnotationName returns the first annotation name for the current value,
// or empty string if none.
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

// getFieldNameText returns the field name text for the current value,
// or empty string if none.
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

// decryptDRMION takes raw DRMION data (with \xeaDRMION\xee wrapper),
// decrypts all pages using the page key, and returns plain CONT KFX.
//
// The DRMION format is:
//   - 8-byte header: \xeaDRMION\xee
//   - ION binary body containing Envelope → {EnvelopeMetadata, EncryptedPage/PlainText} sections
//   - 8-byte trailer: \xeaENDDOC\xee (may be absent)
//
// Each EncryptedPage has cipher_text + cipher_iv, decrypted with AES-128-CBC using the
// 16-byte page key. Decrypted pages are concatenated to form a valid CONT KFX file.
func decryptDRMION(data []byte, pageKey []byte) ([]byte, error) {
	// Strip DRMION header
	if !bytesStartsWithDRMION(data) {
		return nil, fmt.Errorf("not a DRMION file")
	}

	body := data[8:]
	// Strip last 8 bytes (Python does data[8:-8] unconditionally)
	if len(body) > 8 {
		body = body[:len(body)-8]
	}

	cat := newDRMCatalog()
	sys := ion.System{Catalog: cat}
	r := sys.NewReaderBytes(body)

	var output []byte

	for r.Next() {
		// Expect doctype symbol first
		if r.Type() == ion.SymbolType {
			annot := getAnnotationName(r)
			if annot == "doctype" {
				continue
			}
		}

		// Expect a list annotated as Envelope@1.0 or Envelope@2.0
		annot := getAnnotationName(r)
		if !strings.HasPrefix(annot, "com.amazon.drm.Envelope@") {
			continue
		}

		if r.Type() != ion.ListType {
			continue
		}

		if err := r.StepIn(); err != nil {
			return nil, fmt.Errorf("step into envelope: %w", err)
		}

		pages, err := processEnvelope(r, pageKey)
		if err != nil {
			return nil, err
		}
		output = append(output, pages...)

		if err := r.StepOut(); err != nil {
			return nil, fmt.Errorf("step out of envelope: %w", err)
		}
	}

	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("ion reader error: %w", err)
	}

	if len(output) == 0 {
		return nil, fmt.Errorf("DRMION envelope contained no decrypted data")
	}

	return output, nil
}

// processEnvelope iterates over the contents of a DRMION Envelope list,
// decrypting EncryptedPage entries and collecting PlainText entries.
func processEnvelope(r ion.Reader, pageKey []byte) ([]byte, error) {
	var output []byte

	for r.Next() {
		annot := getAnnotationName(r)

		switch {
		case strings.HasPrefix(annot, "com.amazon.drm.EnvelopeMetadata@"):
			// We don't need metadata for decryption, skip it.

		case strings.HasPrefix(annot, "com.amazon.drm.EncryptedPage@"):
			pageData, err := processEncryptedPage(r, pageKey)
			if err != nil {
				return nil, fmt.Errorf("encrypted page: %w", err)
			}
			output = append(output, pageData...)

		case strings.HasPrefix(annot, "com.amazon.drm.PlainText@"):
			pageData, err := processPlainTextPage(r)
			if err != nil {
				return nil, fmt.Errorf("plain text page: %w", err)
			}
			output = append(output, pageData...)

		case annot == "enddoc":
			// End of envelope sections.
			return output, nil
		}
	}

	return output, nil
}

// processEncryptedPage decrypts a single EncryptedPage struct.
func processEncryptedPage(r ion.Reader, pageKey []byte) ([]byte, error) {
	if r.Type() != ion.StructType {
		return nil, fmt.Errorf("encrypted page is not a struct")
	}

	if err := r.StepIn(); err != nil {
		return nil, fmt.Errorf("step into encrypted page: %w", err)
	}

	var ct, civ []byte
	decompress := false

	for r.Next() {
		field := getFieldNameText(r)
		switch field {
		case "cipher_text":
			bv, err := r.ByteValue()
			if err != nil {
				return nil, fmt.Errorf("read cipher_text: %w", err)
			}
			ct = bv
			// Check for Compressed annotation on this field
			if getAnnotationName(r) == "com.amazon.drm.Compressed@1.0" {
				decompress = true
			}
		case "cipher_iv":
			bv, err := r.ByteValue()
			if err != nil {
				return nil, fmt.Errorf("read cipher_iv: %w", err)
			}
			civ = bv
		default:
			// Check for Compressed annotation
			annot := getAnnotationName(r)
			if annot == "com.amazon.drm.Compressed@1.0" {
				decompress = true
			}
		}
	}

	if err := r.StepOut(); err != nil {
		return nil, fmt.Errorf("step out of encrypted page: %w", err)
	}

	if ct == nil || civ == nil {
		return nil, fmt.Errorf("encrypted page missing cipher_text or cipher_iv")
	}

	// Decrypt with AES-128-CBC using page key
	iv := civ[:16]
	decrypted, err := aes128CBCDecrypt(pageKey[:16], iv, ct)
	if err != nil {
		return nil, fmt.Errorf("aes decrypt: %w", err)
	}

	log.Printf("drm: encrypted page ct=%d iv=%d decrypted=%d decompress=%v", len(ct), len(civ), len(decrypted), decompress)

	if decompress {
		// LZMA decompression: first byte is a "use filter" flag (must be 0),
		// rest is LZMA1 data.
		if len(decrypted) == 0 || decrypted[0] != 0 {
			return nil, fmt.Errorf("LZMA use filter flag is %d, expected 0", decrypted[0])
		}
		decompressed, err := lzmaDecompress(decrypted[1:])
		if err != nil {
			return nil, fmt.Errorf("lzma decompress: %w", err)
		}
		log.Printf("drm: encrypted page decompressed %d -> %d", len(decrypted), len(decompressed))
		return decompressed, nil
	}

	// Check for LZMA signature even without Compressed annotation
	if len(decrypted) > 1 && decrypted[0] == 0x00 && decrypted[1] == 0x5d {
		decompressed, err := lzmaDecompress(decrypted[1:])
		if err == nil {
			return decompressed, nil
		}
	}

	return decrypted, nil
}

// processPlainTextPage reads a PlainText page (no decryption needed).
func processPlainTextPage(r ion.Reader) ([]byte, error) {
	if r.Type() != ion.StructType {
		return nil, fmt.Errorf("plain text page is not a struct")
	}

	if err := r.StepIn(); err != nil {
		return nil, fmt.Errorf("step into plain text: %w", err)
	}

	var plaintext []byte
	decompress := false

	for r.Next() {
		field := getFieldNameText(r)
		switch field {
		case "data":
			bv, err := r.ByteValue()
			if err != nil {
				return nil, fmt.Errorf("read data: %w", err)
			}
			plaintext = bv
		default:
			annot := getAnnotationName(r)
			if annot == "com.amazon.drm.Compressed@1.0" {
				decompress = true
			}
		}
	}

	if err := r.StepOut(); err != nil {
		return nil, fmt.Errorf("step out of plain text: %w", err)
	}

	if plaintext == nil {
		return nil, nil
	}

	// Some DRMION files use PlainText pages that are LZMA-compressed
	// but don't have the Compressed@1.0 annotation. Detect by checking
	// for LZMA signature (0x00 filter byte + 0x5d properties byte).
	if !decompress && len(plaintext) > 1 && plaintext[0] == 0x00 && plaintext[1] == 0x5d {
		decompress = true
		log.Printf("drm: plaintext page has LZMA signature without Compressed annotation")
	}

	if decompress {
		if len(plaintext) == 0 || plaintext[0] != 0 {
			return nil, fmt.Errorf("LZMA use filter flag is %d", plaintext[0])
		}
		decompressed, err := lzmaDecompress(plaintext[1:])
		if err != nil {
			return nil, fmt.Errorf("lzma decompress: %w", err)
		}
		return decompressed, nil
	}

	return plaintext, nil
}

// lzmaDecompress decompresses LZMA1 data (format_alone).
func lzmaDecompress(data []byte) ([]byte, error) {
	r, err := lzma.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("lzma reader: %w", err)
	}

	result, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("lzma read: %w", err)
	}
	return result, nil
}

// bytesStartsWithDRMION checks if data begins with the DRMION magic.
// DecryptDRMION is the exported version of decryptDRMION.
func DecryptDRMION(data []byte, pageKey []byte) ([]byte, error) {
	return decryptDRMION(data, pageKey)
}

// DRMIONSignature returns the 8-byte DRMION magic header.
func DRMIONSignature() []byte {
	return []byte{0xea, 'D', 'R', 'M', 'I', 'O', 'N', 0xee}
}

func bytesStartsWithDRMION(data []byte) bool {
	return len(data) >= 8 &&
		data[0] == 0xea &&
		data[1] == 'D' &&
		data[2] == 'R' &&
		data[3] == 'M' &&
		data[4] == 'I' &&
		data[5] == 'O' &&
		data[6] == 'N' &&
		data[7] == 0xee
}
