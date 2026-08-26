# KFX Input 2.34 delta audit

**Date:** 2026-08-26  
**Upstream sync commit:** `cf23d6d` (`20260520` / 2.33 → `20260822` / 2.34)  
**Go baseline audited:** `go` after `1d705c0`

This is a narrow audit of the exact KFX Input 2.33 → 2.34 change set against the helper's supported **CONT/KFX → EPUB** path. It is not a general parity claim. KPF/KDF-only behavior, alternate CBZ/PDF output modes, diagnostics-only changes, and bundled third-party library updates are called out separately.

## Supported CONT/KFX → EPUB changes

| Upstream 2.34 change | Python source-of-truth | Go status |
| --- | --- | --- |
| EPUB 3 XHTML now emits `<!DOCTYPE html>`; EPUB 2 keeps XHTML 1.1 doctype | `epub_output.py`, `process_xhtml_files` | **Ported.** `internal/epub/epub.go` selects the EPUB3/EPUB2 doctype; covered by EPUB writer tests and the Previewer oracle. |
| Comic spine entries with no explicit itemref property default to `rendition:page-spread-center` when any spread is present | `epub_output.py`, OPF spine construction | **Ported.** `internal/epub/epub.go` precomputes `hasPageSpread` over the complete spine and applies the comic default. |
| Dictionary auxiliary `$597` inflection rules are parsed before content rendering | `yj_to_epub.py` + `yj_to_epub_misc.py: process_dictionary_rules` | **Ported.** `internal/kfx/yj_dictionary.go: processDictionaryRules`, called by `renderBookState` before reading-order rendering. |
| Dictionary content emits `idx:entry` / `idx:orth` / `idx:infl` / `idx:iform`, handles flat and structured rule lists, and tracks used rules | `yj_to_epub_content.py`, dictionary block added in 2.34 | **Ported.** `internal/kfx/yj_dictionary.go` + normal storyline renderer; production-path and malformed-input tests cover the branch. |
| Defined/used dictionary rules are reported after content rendering | `yj_to_epub.py` finalization | **Ported.** `storylineRenderer.reportDictionaryRuleUsage` runs after reading order. Sticky dictionary renderer errors are propagated by `renderBookState` (`069ed96`). |
| Shared YJ table extends through SID 859 and current creator/max-id metadata advances | `yj_symbol_catalog.py`, `yj_versions.py` | **Ported and independently verified.** Go catalog/current version tables are synced through 859; Previewer 3.106 native `DigitalBook` resolution independently identifies the 852..859 tail. |
| PDF page-image path reuses a cached `PdfReader` | `resources.py: get_pdf_reader`; callers in `yj_structure.py`, `yj_to_epub_resources.py` | **Output-neutral cache itself not ported.** Go's PDF implementation was audited against current 2.34/pypdf page-image semantics (render-first, text/Form checks, image count, crop/media box, filter/mask handling, image-match gate). Lack of a reader cache affects performance only. |
| Missing dictionary JPEG raw-media is downgraded from error to warning | `yj_to_epub_resources.py: locate_raw_media`; `yj_structure.py` missing-fragment reporting | **Diagnostics gap.** Go's production resource builder can still warn/skip missing image bytes, but does not reproduce this exact dictionary+JPEG severity distinction. No EPUB-byte behavior changes. |
| Scribe canvas-size message changes warning → info | `yj_to_epub_notebook.py` | **Diagnostics-only.** No output semantic change. |

### Result for the supported path

After the current integration, the exact 2.34 delta contains **no known unported user-visible semantic change in the supported CONT/KFX → EPUB path**. The remaining differences above are cache/performance or logging severity. This conclusion is deliberately narrower than "KFX parity": the structural auditor still reports hundreds of gaps against the full current upstream definition set.

The strongest behavioral check is the controlled Previewer 3.106 matrix. Twelve independently generated specimens (`minimal`, `footnote`, `table`, `fixed-layout`, `vertical-ruby`, `link`, `bidi`, `list`, `svg`, `dropcap`, `image-figure`, `first-line`) are serialized once and then reversed by KFX Input 2.34 and Go. At `1d705c0`, all twelve have zero structural, image, and other differences after excluding the nondeterministic OPF timestamp.

## KPF/KDF-only 2.34 changes

These are real upstream changes, but the Go helper does not currently ingest the KPF/KDF SQLite container format, so they are not reachable from its production input path:

- `KpfContainer` now centralizes the KDF schemas and detects DRM-free dictionaries before normal deserialization.
- KPF/KDF dictionary indices are checked against terms found in content.
- Prepublication dictionaries can synthesize `$597` dictionary auxiliary data from `DictionaryRules.ion` and wire it into document data.
- Dictionary KPF fix-up adds the dictionary-rule auxiliary reference into `$538` document data.
- `local_action_fragments` / `local_delta_fragments` remain the authoritative source of Scribe-notebook detection.
- Single-container DRM-free dictionaries can be serialized back to KFX without running the normal dictionary checks.

This is also why the Go Scribe renderer must not be described as end-to-end Scribe support: the rendering code is present and tested, but the production helper has no KPF/KDF ingestion path that can set `IsScribeNotebook` from those schemas.

## Alternate output modes

2.34 also changes image-book/PDF/CBZ behavior:

- PDF readers are cached across repeated page operations.
- `combine_images_into_pdf` returns the original PDF when `PdfWriter(clone_from=...)` fails instead of returning `None`.
- CBZ/PDF assembly passes the PDF cache through repeated page extraction.

The plugin's production `kindle-helper convert` path emits EPUB, not the image-book CBZ/PDF modes. These changes therefore do not block the supported conversion path. The Go image-book code has substantial ports of those modes, but they should be evaluated separately if they become product-facing.

## Third-party pypdf update

Most lines in `cf23d6d` are the bundled pypdf update. Go does **not** and should not attempt a file-for-file pypdf port; it uses pdfcpu plus explicit page-content/image validation. The relevant current pypdf behavior was reviewed specifically for KFX's PDF-page-to-image decision path during the PDF work:

- rendering happens before extraction and is the visual oracle/fallback;
- invoked Form XObjects participate in text detection;
- inline images affect image counting;
- DCT/Flate/CCITT and mask cases have different acceptance behavior;
- extracted images are accepted only after visual comparison with the rendered page.

That review does not imply parity with every pypdf API.

## On-device PDF rendering limitation

The static Go helper uses `pdftoppm` for the render-first oracle. Kindle firmware does not provide it. This is **not a regression introduced by the Go port**: the shipped standalone Python helper also runs without Calibre, leaves `calibre_numeric_version = None`, skips its `pdftoppm` branch, and follows the same honest failure path (log the conversion failure and keep the PDF resource).

KOReader itself ships MuPDF and exposes PDF rendering through its LuaJIT FFI stack, so a future helper↔Lua renderer bridge can improve PDF-backed/print-replica fidelity without CGO. That is an enhancement beyond 2.34 production parity, not a blocker for general reflowable KFX.

## Residual items worth tracking

1. Reproduce the exact 2.34 dictionary missing-raw-media warning severity if diagnostic parity becomes useful.
2. Add KPF/KDF SQLite ingestion only if real Scribe or KPF dictionary inputs become an actual product requirement.
3. Treat the MuPDF Lua renderer bridge as a PDF-backed fidelity enhancement rather than a prerequisite for merging general CONT/KFX support.
4. Continue expanding the Previewer oracle; clean controlled fixtures are behavioral evidence for exercised paths, not representative real-book coverage.
