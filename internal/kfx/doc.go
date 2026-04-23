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
// Every non-trivial Python module in kfxlib/ is listed below with its Go counterpart.
// Files marked (many-to-one) or (split) contain logic from multiple Python modules.
//
//	Python module (kfxlib/)                  | Go file(s) in internal/kfx/                   | Notes
//	-----------------------------------------|-----------------------------------------------|-----------------------------------------------
//	__init__.py                              | —                                             | Re-exports; no Go counterpart (plugin entry is cmd/kindle-helper)
//	epub_output.py                           | internal/epub/epub.go                         | EPUB packaging (zip/OPF/nav); outside this package
//	ion.py                                   | values.go                                     | Ion type constants (IonBool, IonString, etc.) and ion_type dispatch
//	ion_binary.py                            | ion_binary.go                                 | ION binary decoding (decodeIonMap, decodeIonValue, normalizeIon, stripIVM)
//	ion_symbol_table.py                      | yj_symbol_catalog.go (many-to-one)            | SymbolTableCatalog → symbolResolver; combined with YJ symbol catalog into one file
//	ion_text.py                              | —                                             | ION text format; not ported (Go only handles binary ION)
//	jxr_container.py                         | internal/jxr/                                 | JXR container parsing; outside this package
//	jxr_image.py                             | internal/jxr/                                 | JXR image decoding; outside this package
//	jxr_misc.py                              | internal/jxr/                                 | JXR utilities (Deserializer, etc.); outside this package
//	kfx_container.py                         | kfx_container.go                              | CONT binary parsing (loadContainerSource*, collectContainer*Blobs, validateEntityOffsets, loadBookSources)
//	kpf_book.py                              | —                                             | KPF book handling; not ported (Kindle-only, not KOReader concern)
//	kpf_container.py                         | —                                             | KPF container; not ported (Kindle-only, not KOReader concern)
//	message_logging.py                       | log.Printf / fmt.Fprintf to stderr            | Logging; no dedicated Go file (uses standard library)
//	original_source_epub.py                  | —                                             | Original source EPUB extraction; not ported
//	resources.py                             | yj_to_epub_resources.go + symbol_format.go    | Split: image/font helpers → yj_to_epub_resources.go; filename helpers → symbol_format.go
//	unpack_container.py                      | kfx_container.go (split)                      | Container unpacking (IonTextContainer, ZipUnpackContainer) → kfx_container.go handles CONT/ZIP loading
//	utilities.py                             | —                                             | General utilities; no Go counterpart (uses Go standard library)
//	version.py                               | yj_versions.go                                | Version string constants
//	yj_book.py                               | yj_to_epub.go (many-to-one)                   | decode_book orchestration → yj_to_epub.go (buildBookState, organizeFragments)
//	yj_container.py                          | yj_container.go                               | Fragment data model (FragmentKey, Fragment, FragmentList, fragment type sets)
//	yj_metadata.py                           | yj_metadata.go                                | Book metadata getters and queries
//	yj_position_location.py                  | yj_position_location.go                       | Position/location handling
//	yj_structure.py                          | yj_structure.go                               | BookStructure validation, walking, and checking
//	yj_symbol_catalog.py                     | yj_symbol_catalog.go                          | Shared symbol table, catalog (sharedCatalog, sharedTable, yjPrelude), symbol resolver
//	yj_to_epub.py                            | yj_to_epub.go                                 | Pipeline orchestration (ConvertFile, ConvertFileWithTrace, renderBookState, organizeFragments, buildBookState, decodeKFX)
//	yj_to_epub_content.py                    | yj_to_epub_content.go + fragments.go          | Split: content processing → yj_to_epub_content.go; reading order iteration → fragments.go
//	yj_to_epub_illustrated_layout.py         | yj_to_epub_illustrated_layout.go              | Illustrated layout anchor fixups
//	yj_to_epub_metadata.py                   | yj_to_epub_metadata.go                        | EPUB metadata application (applyMetadata, applyDocumentData, applyContentFeatures)
//	yj_to_epub_misc.py                       | yj_to_epub_misc.go                            | Condition operator symbols and evaluation
//	yj_to_epub_navigation.py                 | yj_to_epub_navigation.go + render.go          | Split: navigation processing → yj_to_epub_navigation.go; anchor resolution → render.go
//	yj_to_epub_notebook.py                   | yj_to_epub_notebook.go                        | Scribe/notebook processing stubs
//	yj_to_epub_properties.py                 | yj_to_epub_properties.go + yj_property_info.go + css_values.go | Split: style catalog → yj_to_epub_properties.go; property→CSS map → yj_property_info.go; CSS value handling → css_values.go
//	yj_to_epub_resources.py                  | yj_to_epub_resources.go                       | Resource and font building
//	yj_to_image_book.py                      | yj_to_image_book.go                           | Image book conversion
//	yj_versions.py                           | yj_versions.go                                | YJ version constants and validation
//
// # Go Files with No Python Counterpart (Unique Go Concerns)
//
// These files exist only in Go, addressing Go-specific needs:
//
//	Go file                      | Purpose
//	-----------------------------|----------------------------------------------------------
//	kfx.go                       | Public types (decodedBook, error types), Classify entry point
//	state.go                     | Fragment/book data types (fragmentCatalog, bookState), symbol merging helpers, sharedDocSymbols
//	content_helpers.go           | HTML generation helper functions
//	css_values.go                | CSS value handling (enum props, unit conversion)
//	drm.go                       | DRM decryption (DRMION format; not in Calibre KFX Input)
//	html.go                      | HTML DOM type definitions (htmlElement, htmlPart, etc.)
//	render.go                    | Anchor resolution, section materialization, DOM cleanup
//	sidecar.go                   | Kindle .sdr sidecar directory metadata extraction
//	storyline.go                 | Storyline rendering engine
//	style_events.go              | Style event processing and CSS class generation
//	svg.go                       | KVG/SVG rendering
//	trace.go                     | Conversion trace/debug output writer
//	symbol_format.go             | Symbol classification and format determination
//	fragments.go                 | Reading order fragment iteration
//	yj_property_info.go          | Data-driven YJ property → CSS mapping table
//	values.go                    | Ion value type helpers and constants
//
// # Confirmatory Testing
//
// Use scripts/diff_kfx_parity.sh and scripts/kfx_reference_snapshot.py (fragment-summary) per fixture:
// REFERENCE/kfx_examples/*.kfx, REFERENCE/kfx_new/decrypted/*.kfx-zip, monolithic_kfx, and
// REFERENCE/kfx_new/calibre_epubs/*.epub references where present.
package kfx
