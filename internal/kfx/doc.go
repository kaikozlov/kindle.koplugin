// Package kfx converts Amazon KFX containers into EPUB, using Calibre KFX Input as the behavioral
// reference (REFERENCE/Calibre_KFX_Input/kfxlib/) and EPUB / fragment snapshot tests as confirmatory evidence.
//
// # Pipeline Stage Order
//
//	Python: YJ_Book.decode_book → KFX_EPUB.__init__ (yj_to_epub.py) → EPUB_Output.generate_epub
//	Go:     loadBookSources → organizeFragments (yj_book.go) → renderBookState (yj_to_epub.go) → epub.Write
//
// # Python ↔ Go File Map
//
// Every Python module maps to exactly one Go file. The mapping is strictly 1:1.
//
//	Python module (kfxlib/)              | Go file (internal/kfx/)
//	-------------------------------------|-----------------------------------------------
//	epub_output.py                       | epub_output.go
//	ion.py                               | values.go
//	ion_binary.py                        | ion_binary.go
//	ion_symbol_table.py                  | ion_symbol_table.go
//	kfx_container.py                     | kfx_container.go
//	resources.py                         | yj_to_epub_resources.go
//	version.py                           | yj_versions.go
//	yj_book.py                           | yj_book.go
//	yj_container.py                      | yj_container.go
//	yj_metadata.py                       | yj_metadata.go
//	yj_position_location.py              | yj_position_location.go
//	yj_structure.py                      | yj_structure.go
//	yj_symbol_catalog.py                 | yj_symbol_catalog.go
//	yj_to_epub.py                        | yj_to_epub.go
//	yj_to_epub_content.py                | yj_to_epub_content.go
//	yj_to_epub_illustrated_layout.py     | yj_to_epub_illustrated_layout.go
//	yj_to_epub_metadata.py               | yj_to_epub_metadata.go
//	yj_to_epub_misc.py                   | yj_to_epub_misc.go
//	yj_to_epub_navigation.py             | yj_to_epub_navigation.go
//	yj_to_epub_notebook.py               | yj_to_epub_notebook.go
//	yj_to_epub_properties.py             | yj_to_epub_properties.go
//	yj_to_epub_resources.py              | yj_to_epub_resources.go
//	yj_to_image_book.py                  | yj_to_image_book.go
//
// # Not Ported (outside Go scope)
//
//	__init__.py        — Calibre plugin re-exports; cmd/kindle-helper is the Go entry point
//	ion_text.py        — ION text format; Go only handles binary ION
//	jxr_*.py           — JXR decoding lives in internal/jxr/
//	kpf_book.py        — KPF book handling; Kindle-only, not a KOReader concern
//	kpf_container.py   — KPF container; same
//	message_logging.py — Logging; uses Go standard library (log.Printf)
//	original_source_epub.py — Original source EPUB extraction; not ported
//	unpack_container.py — Container unpacking subsumed by kfx_container.go
//	utilities.py       — General utilities; uses Go standard library
//
// # Go-Only Files
//
//	Go file     | Purpose
//	------------|----------------------------------------------------------
//	kfx.go      | Public types (decodedBook, error types), Classify entry point
//	drm.go      | DRM decryption (DRMION format; not in Calibre KFX Input)
//	sidecar.go  | Kindle .sdr sidecar directory metadata extraction
//	trace.go    | Conversion trace/debug output writer
//
// # YJ Symbol Catalog
//
// Go uses real human-readable symbol names (language, font_family, content, etc.)
// throughout the conversion pipeline. These come from catalog.ion, which is embedded
// at compile time and parsed at init time. The catalog contains 842 YJ shared symbol
// names extracted from Amazon's Kindle Previewer.
//
// Python uses $N placeholders ($10, $145, etc.) internally and only translates at the
// ION text boundary. Go uses real names directly — so node["content"] in Go corresponds
// to node["$145"] in Python.
//
// Key functions:
//   - sharedTable()        → ion.SharedSymbolTable with 842 real names + $N fallbacks
//   - isSharedSymbolName() → checks if a name is a YJ shared symbol
//   - resolveSharedSymbol() → resolves a SID to its real name
//
// To update the catalog: copy REFERENCE/kfx_symbol_catalog.ion → internal/kfx/catalog.ion
//
// # Golden-File Parity Tests
//
// These tests compare Go's static data tables against the Calibre Python reference.
// If the Python source changes, the golden files must be regenerated:
//
//	python3 scripts/export_yj_symbol_catalog.py > internal/kfx/testdata/yj_symbols_golden.json
//	python3 scripts/export_yj_versions.py > internal/kfx/testdata/yj_versions_golden.json
//
// A Calibre bump that edits yj_symbol_catalog.py or yj_versions.py without
// updating Go goldens will fail CI.
//
// # Confirmatory Testing
//
// Use scripts/diff_kfx_parity.sh and scripts/kfx_reference_snapshot.py (fragment-summary) per fixture:
// REFERENCE/books/<name>/input.kfx (CONT), input.kfx-zip (DRMION), original.kfx (DRMION), and
// REFERENCE/books/<name>/calibre.epub references.
package kfx
