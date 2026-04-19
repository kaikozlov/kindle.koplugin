// Package kfx converts Amazon KFX containers into EPUB, using Calibre KFX Input as the behavioral
// reference (REFERENCE/Calibre_KFX_Input/kfxlib/) and EPUB / fragment snapshot tests as confirmatory evidence.
//
// # Python ↔ Go file map (living)
//
// Calibre module (kfxlib) → Go file(s)
//
//	yj_to_epub.py (KFX_EPUB orchestration, organize_fragments_by_type, determine_book_symbol_format) → state.go (organizeFragments), kfx.go (ConvertFile, decodeKFX, ION), symbol_format.go (classify_symbol, determine_book_symbol_format, unique/prefix helpers); post-organize ordering remains render.go (renderBookState)
//	yj_to_epub_content.py → yj_to_epub_content.go (processReadingOrder, processSection→renderSectionFragments, prepareBookParts), render.go (renderBookState = __init__ glue)
//	yj_to_epub_properties.py → yj_to_epub_properties.go (styleCatalog, yj_property_info.go simple CSS map, fixupStylesAndClasses/createCSSFiles/updateDefaultFontAndLanguage/setHTMLDefaults hooks, finalizeStylesheet, …), kfx.go (emitters — remainder)
//	yj_to_epub_navigation.py → yj_to_epub_navigation.go (processNavigation, fixupAnchorsAndHrefs, reportMissingPositions, nav/guide/page EPUB helpers)
//	yj_to_epub_metadata.py → yj_to_epub_metadata.go (applyMetadata, applyDocumentData, applyContentFeatures)
//	yj_to_epub_resources.py → yj_to_epub_resources.go (buildResources, convertJXRResource, resource/font helpers)
//	yj_to_epub_misc.py → yj_to_epub_misc.go (pythonConditionOperatorSymbols registry, conditionEvaluator.evaluate = set_condition_operators semantics)
//	yj_to_epub_illustrated_layout.py → yj_to_epub_illustrated_layout.go (fixupIllustratedLayoutAnchors, -kfx-amzn-condition rewrite subset)
//	yj_to_epub_notebook.py → yj_to_epub_notebook.go (processScribe* stubs until fixtures)
//	epub_output.py → internal/epub/epub.go (zip/OPF/nav; package comment tracks epub_output.py gap)
//
// # KFX_EPUB.__init__ checklist (plan YAML todos → status)
//
//	phase-0-remainder-thin-kfx — Done: styleCatalog moved to yj_to_epub_properties.go; kfx.go still holds ION/storyline/CSS emitters.
//	phase-a-replace-ion-organize — Partial: mergeIonReferencedStringSymbols + mergeContentFragmentStringSymbols ($145) + TitleMetadata, $164/$266, sections, nav roots, + post-organize maps; index walk still not Python book_data / replace_ion_data struct.
//	phase-a-classify-shared-symtab — Partial: classifySymbolWithResolver + symbolResolver.isSharedSymbolText for "$<sid>" text.
//	phase-a-symbol-format-wire-naming — Partial: sectionFilename + buildResources resource/font stems use uniquePartOfLocalSymbol + prefixUniquePartOfSymbol (yj_to_epub_resources.resource_location_filename); style catalog / fragment keys may still use chooseFragmentIdentity.
//	phase-a-condition-operators — Done: pythonConditionOperatorSymbols + conditionOperatorArity + conditionOperatorDispatch table + arity mismatch logging + unknown operator error + evaluate_binary_condition non-binary result warning (yj_to_epub_misc.set_condition_operators / evaluate_condition / evaluate_binary_condition).
//	phase-a-post-organize-init-metadata — Done: applyKFXEPUBInitMetadataAfterOrganize after determineBookSymbolFormat (yj_to_epub.py L77–80); expanded applyMetadata with publisher, description, ASIN, book_id, cde_content_type, facing_page, double_page_spread; applyDocumentData now extracts writing_mode, page_progression_direction, orientation_lock with error logging; applyContentFeatures parses $590 feature array.
//	phase-b-process-reading-order — Done: processReadingOrder in yj_to_epub_content.go, called from renderBookState.
//	phase-b-process-section — Partial: processSection entry + renderSectionFragments reflowable/fixed subset; heading tag selection now checks layout hints (simplify_styles L1928 parity) — $790 alone no longer promotes to <h1>-<h6> without "heading" in $761 layout hints; scribe/comic/magazine branches unported.
//	phase-b-prepare-book-parts — Partial: prepareBookParts reapplies normalizeHTMLWhitespace per rendered section Root before materializeRenderedSections.
//	phase-c-yj-property-info — Partial: yj_property_info.go simple Ion→CSS map + YJPropertySimpleCSSName; enum Prop entries moved from css_values.go and yj_to_epub_resources.go.
//	phase-c-fixup-styles-classes — Partial: fixupStylesAndClasses strips empty class attrs; createCSSFiles merges styleCatalog; styleBaseName applies uniquePartOfLocalSymbol; simplifyStylesPostProcess converts div→p (text-only, no block children) and unwraps classless spans (Python simplify_styles div/p/figure conversion parity).
//	phase-c-simplify-styles-html-defaults — Partial: setHTMLDefaults fills per-section Language; updateDefaultFontAndLanguage normalizes book language; simplifyStylesPostProcess in yj_to_epub_properties.go handles div→p/figure conversion + span unwrapping (partial port of Python simplify_styles ~L1928).
//	phase-d-process-anchors-navigation-reporting — Partial: reportMissingPositions; reportDuplicateAnchors; processNavigation retained; guideToEPUB includes "text"→"bodymatter" landmark.
//	phase-d-fixup-anchors-hrefs — Partial: fixupAnchorsAndHrefs → replaceRenderedAnchorPlaceholders.
//	phase-d-illustrated-layout — Partial: -kfx-amzn-condition anchor: URI rewrite when IllustratedLayout.
//	phase-d-metadata-opf-remainder — Partial: OPF emission in internal/epub; dc:publisher/dc:description emitted; expanded decodedBook fields (Description, Publisher, ASIN, BookID, WritingMode, PageProgressionDirection); applyMetadata handles 20+ categorized keys from $490; applyDocumentData extracts writing_mode, page_progression_direction, orientation_lock; $258 title guarded with "if not already set" to prevent overriding $490 full title.
//	phase-d-epub-output-package — Partial: manifest items sorted together by filename string (Calibre epub_output.py ordering parity); spine ordering matches Calibre via promoteCoverSectionFromGuide (cover first in spine); cover SVG promotion uses content-based matching (not just title="Cover"); class_s8 body class only for titled Cover sections; nav.xhtml/toc.ncx structure matches; guide entries match Calibre types/titles/hrefs.
//	phase-d-e2e-calibre-structure-tests — Partial: Martyr full gate (0 text diffs); Three Below spine ordering now matches Calibre; Elvis/Hunger/Familiars path-list when refs exist.
//	phase-e-images-resources — Partial: convertJXRResource (gray→JPEG); byte parity vs Calibre deferred; PLACE_FILES_IN_SUBDIRS not implemented (Calibre default is False; Three Below reference used non-default True → images/ path differences are configuration, not parity gap).
//	phase-f-notebook-scribe — Partial: yj_to_epub_notebook.go processScribe* stubs (no fixture-driven logic).
//	error-handling-implement — Partial: missing image/font bytes → stderr warnings in buildResources; condition operator arity mismatch + unknown op → stderr errors in evaluate; non-binary condition results → stderr error in evaluateBinary; unexpected orientation_lock → stderr error in applyDocumentData; title override guard ($258 won't override $490 title).
//	doc-go-checklist-update — This block is the living checklist (update after substantive ports).
//
// # KFX_EPUB.__init__ stage order vs Go
//
// Python: YJ_Book.decode_book → KFX_EPUB.__init__ (yj_to_epub.py) → EPUB_Output.generate_epub.
// Go: loadBookSources → organizeFragments (state.go) → renderBookState (render.go) → epub.Write.
//
// # classify_symbol / SHARED
//
// classifySymbolWithResolver (symbol_format.go) applies symShared for unresolved "$<sid>" names when
// symbolResolver.isSharedSymbolText matches imported symbols (sid < localStart, not a local slot). Resolved
// fragment IDs from the index still go through classifySymbol regex branches.
//
// # Error-handling parity
//
// Structural/decode failures → return error. Calibre log.warning / check_empty class → prefer stderr and continue;
// buildResources logs when image bytes are missing for a declared image resource.
//
// # Confirmatory matrix
//
// Use scripts/diff_kfx_parity.sh and scripts/kfx_reference_snapshot.py (fragment-summary) per fixture:
// REFERENCE/kfx_examples/*.kfx, REFERENCE/kfx_new/decrypted/*.kfx-zip, monolithic_kfx, and
// REFERENCE/kfx_new/calibre_epubs/*.epub references where present.
package kfx
