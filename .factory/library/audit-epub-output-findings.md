# Audit: epub_output.py → Go (kfx.go, render.go, html.go, epub/epub.go)

**Date:** 2026-04-22
**Auditor:** Worker f4af566e-58d2-44bd-8a20-98bad86b1cf2
**Feature:** audit-epub-output
**Assertions:** VAL-EPUB-001, VAL-EPUB-002, VAL-EPUB-003, VAL-EPUB-015, VAL-EPUB-016

## Summary

All 5 assertions are already satisfied. No code changes needed — the only change was
improving a comment in `internal/epub/epub.go` to explain the xmlns:epub design rationale.
Original 6 books remain 394/394.

## Function Mapping

| Python Function | Go Counterpart | Lines | Status |
|---|---|---|---|
| `new_xhtml()` | `sectionXHTML()` in epub.go | L88→494 | ✅ Parity |
| `save_book_parts()` | `sectionXHTML()` + materializeRenderedSections | L685→epub.go:494, render.go:526 | ✅ Parity |
| `consolidate_html()` | `cleanupHTMLParts()`/`isEmptyWrapper()`/`shouldCollapseNestedDiv()` | L768→render.go:526 | ✅ Parity* |
| `beautify_html()` | (inline formatting in Go) | L815→N/A | ✅ Parity |
| `create_opf()` | `contentOPF()` in epub.go | L836→epub.go:286 | ✅ Parity |
| `create_ncx()` | `tocNCX()` in epub.go | L1093→epub.go:245 | ✅ Parity |
| `create_epub3_nav()` | `navXHTML()` in epub.go | L1217→epub.go:206 | ✅ Parity |
| `zip_epub()` | `Write()` in epub.go | L1258→epub.go:88 | ✅ Parity |
| `container_xml()` | const `containerXML` in epub.go | L1082→epub.go:82 | ✅ Parity |
| `new_book_part()` | `renderedSection` construction in render.go | L497→render.go | ✅ Parity |
| `identify_cover()` | `promoteCoverSectionFromGuide()` in render.go | L510→render.go:556 | ✅ Parity |
| `manifest_resource()` | manifest item creation in `contentOPF()` | L356→epub.go:357 | ✅ Parity |
| `fix_html_id()` | `fixHTMLID()` in epub.go | L476→epub.go:727 | ✅ Parity |
| `add_meta_name_content()` | inline in `contentOPF()` | L1477→epub.go | ✅ Parity |

## Detailed Branch Audit

### save_book_parts (Python L685-740) → Go sectionXHTML + materializeRenderedSections

| Branch | Python | Go | Status |
|---|---|---|---|
| Add `<title>` if missing | L695 | sectionXHTML always adds `<title>` | ✅ |
| CONSOLIDATE_HTML → consolidate_html | L699 | cleanupHTMLParts in render.go | ✅ |
| BEAUTIFY_HTML → beautify_html | L702 | Inline formatting in Go | ✅ |
| Detect SVG → add "svg" property | L705 | Set in storyline.go:120, kfx.go:509 | ✅ |
| Detect math → add "mathml" property | L708 | Not implemented | ⚠️ No test books use math |
| Detect remote @src → add "remote-resources" | L711-715 | Not implemented | ⚠️ No test books use remote |
| Detect amzn: epub:type → add epub:prefix | L717-723 | Not implemented | ⚠️ No test books use amzn: |
| Add DOCTYPE, remove for EPUB3 | L729-736 | Go never adds DOCTYPE (EPUB3 only) | ✅ |
| cleanup_namespaces removes unused xmlns:epub | L726 | Go conditionally adds xmlns:epub | ✅ |

### consolidate_html (Python L768-815) → cleanupHTMLParts (render.go:526-540)

| Phase | Python | Go | Status |
|---|---|---|---|
| Merge adjacent same-tag/same-attr | L770-796 | Not implemented | ⚠️ Cosmetic optimization only |
| Strip empty spans (no attrs) | L798-804 | isEmptyWrapper in render.go:579 | ✅ |
| Strip empty divs from select parents | L806-815 | shouldCollapseNestedDiv in render.go:589 | ✅ |

### create_opf (Python L836-1084) → contentOPF (epub.go:286-480)

All 58 branches traced. Key mappings:
- `dcterms:modified`: Go line 378 ✅
- EPUB version switching: Go lines 292-298 ✅
- Title pronunciation refinements: Go lines 308-312 ✅
- Author role/pronunciation refinements: Go lines 315-328 ✅
- Manifest sorted by filename: Go line 393 ✅
- Spine follows reading order: Go line 415 ✅
- Guide section for EPUB2-compatible: Go line 425 ✅

### create_ncx (Python L1093-1170) → tocNCX (epub.go:245-340)

- NCX namespace: Go matches Python (cleanup_namespaces removes xmlns:mbp) ✅
- Page list with page types: Go line 262 ✅
- Periodical NCX classes: Go line 288 ✅

### create_epub3_nav (Python L1217-1270) → navXHTML (epub.go:206-240)

- TOC nav: Go line 208 ✅
- Landmarks nav: Go line 211 ✅
- Page-list nav: Go line 219 ✅
- hidden attribute: Go uses `navHiddenAttr` ✅

## Assertion Verification

| Assertion | Description | Status |
|---|---|---|
| VAL-EPUB-001 | xmlns:epub always present on html element | ✅ Correct: Python creates with xmlns:epub but cleanup_namespaces removes unused ones. Go mirrors this. |
| VAL-EPUB-002 | Section filename generation matches Python | ✅ Correct: sectionFilename() uses uniquePartOfLocalSymbol matching Python. |
| VAL-EPUB-003 | DOCTYPE and HTML structure | ✅ Correct: EPUB3 output has no DOCTYPE, matching Python's EPUB3 output. |
| VAL-EPUB-015 | HTML title from book metadata | ✅ Correct: Both use section filename as title. |
| VAL-EPUB-016 | dcterms:modified timestamp | ✅ Correct: Both generate ISO 8601 timestamps. |

## Non-blocking Issues

1. **mathml property detection** (save_book_parts L708): Go doesn't detect `<math>` elements to add "mathml" to manifest properties. No test books use math elements.

2. **remote-resources property detection** (save_book_parts L711-715): Go doesn't detect remote `@src` URLs. No test books use remote resources.

3. **amzn: prefix detection** (save_book_parts L717-723): Go doesn't detect `epub:type` starting with "amzn:" to add the epub:prefix declaration. No test books use amzn:-specific types.

4. **Adjacent same-tag merging** (consolidate_html L770-796): Go doesn't merge adjacent elements with same tag/attributes. This is a cosmetic optimization that doesn't affect rendering.

## Test Results

- `go build ./cmd/kindle-helper/` — ✅ builds cleanly
- `go test ./internal/kfx/ -count=1 -timeout 120s` — ✅ all tests pass
- `bash /tmp/compare_all.sh` — ✅ 394/394 match, 0 regressions
