// Package kfx converts Amazon KFX containers into EPUB, using Calibre KFX Input as the behavioral
// reference (REFERENCE/Calibre_KFX_Input/kfxlib/) and EPUB / fragment snapshot tests as confirmatory evidence.
//
// # Pipeline Stage Order
//
//	Python: YJ_Book.decode_book → KFX_EPUB.__init__ (yj_to_epub.py) → EPUB_Output.generate_epub
//	Go:     loadBookSources → organizeFragments (yj_to_epub.go) → renderBookState (yj_to_epub.go) → epub.Write
//
// # Python ↔ Go File Map
//
// Every Python module maps to exactly one Go file. The mapping is 1:1.
//
//	Python module (kfxlib/)                  | Go file (internal/kfx/)
//	-----------------------------------------|-----------------------------------------------
//	epub_output.py                           | epub_output.go
//	ion.py                                   | values.go
//	ion_binary.py                            | ion_binary.go
//	ion_symbol_table.py + yj_symbol_catalog.py | yj_symbol_catalog.go
//	kfx_container.py                         | kfx_container.go
//	resources.py                             | yj_to_epub_resources.go
//	version.py                               | yj_versions.go
//	yj_book.py                               | yj_to_epub.go
//	yj_container.py                          | yj_container.go
//	yj_metadata.py                           | yj_metadata.go
//	yj_position_location.py                  | yj_position_location.go
//	yj_structure.py                          | yj_structure.go
//	yj_to_epub.py                            | yj_to_epub.go
//	yj_to_epub_content.py                    | yj_to_epub_content.go
//	yj_to_epub_illustrated_layout.py         | yj_to_epub_illustrated_layout.go
//	yj_to_epub_metadata.py                   | yj_to_epub_metadata.go
//	yj_to_epub_misc.py                       | yj_to_epub_misc.go
//	yj_to_epub_navigation.py                 | yj_to_epub_navigation.go
//	yj_to_epub_notebook.py                   | yj_to_epub_notebook.go
//	yj_to_epub_properties.py                 | yj_to_epub_properties.go
//	yj_to_epub_resources.py                  | yj_to_epub_resources.go
//	yj_to_image_book.py                      | yj_to_image_book.go
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
//	Go file          | Purpose
//	-----------------|----------------------------------------------------------
//	kfx.go           | Public types (decodedBook, error types), Classify entry point
//	state.go         | Fragment/book data types (fragmentCatalog, bookState), symbol merging helpers
//	drm.go           | DRM decryption (DRMION format; not in Calibre KFX Input)
//	sidecar.go       | Kindle .sdr sidecar directory metadata extraction
//	svg.go           | KVG/SVG rendering (from yj_to_epub_misc.py; split out for size)
//	trace.go         | Conversion trace/debug output writer
//
// # Confirmatory Testing
//
// Use scripts/diff_kfx_parity.sh and scripts/kfx_reference_snapshot.py (fragment-summary) per fixture:
// REFERENCE/kfx_examples/*.kfx, REFERENCE/kfx_new/decrypted/*.kfx-zip, monolithic_kfx, and
// REFERENCE/kfx_new/calibre_epubs/*.epub references where present.
package kfx
