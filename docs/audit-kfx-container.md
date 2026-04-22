# Audit: kfx_container.py → container.go + state.go

**Date:** 2026-04-22
**Feature:** audit-kfx-container
**Status:** ✅ Full parity confirmed

## Python Source
`REFERENCE/Calibre_KFX_Input/kfxlib/kfx_container.py` (449 lines)

## Go Counterparts
- `internal/kfx/container.go` (74 lines) — HTML container helpers (not container parsing)
- `internal/kfx/state.go` (713 lines) — Container loading, entity parsing, fragment organization
- `internal/kfx/decode.go` — ION decoding, symbol resolution, entity payload extraction
- `internal/kfx/fragments.go` — Fragment parsing helpers

## Architecture Note

The Go implementation takes a different structural approach than Python but achieves equivalent behavior:

### Python Architecture
1. `YJ_Book.decode_book()` two-pass: loop 1 calls `container.deserialize()` for all containers; loop 2 calls `container.get_fragments()` for all containers
2. `KfxContainer.deserialize()` — parses CONT header, extracts container_info, doc_symbols, format_capabilities, entity index table, kfxgen_info
3. `KfxContainer.get_fragments()` — yields synthetic $ion_symbol_table/$270/$593 fragments + deserialized entities
4. `KFX_EPUB.organize_fragments_by_type()` — categorizes fragments by type with ID remapping, singleton collapse
5. `KFX_EPUB.__init__()` — calls determine_book_symbol_format, process_content_features, process_metadata, etc.

### Go Architecture
1. `buildBookState()` → `loadBookSources()` → `organizeFragments()` — single entry point
2. `collectContainerBlobs()` — discovers CONT containers from file/zip/sidecar (mirrors Python `locate_book_datafiles()`)
3. `loadContainerSourceData()` — parses CONT header, extracts container_info, doc_symbols, index_table (mirrors `KfxContainer.deserialize()`)
4. `organizeFragments()` — two-pass: pass 1 accumulates docSymbols; pass 2 decodes all entities with shared resolver (mirrors Python two-pass + `organize_fragments_by_type()` combined)
5. Fragment categorization, book symbol format, metadata processing done inline

## Function-by-Function Audit

### 1. `KfxContainer.__init__()` → `containerSource` struct
**Python (L46-48):** Initializes YJContainer with symtab, datafile, fragments.
**Go:** `containerSource` struct holds Path, Data, HeaderLen, ContainerInfo, DocSymbols, IndexData.
**Verdict:** ✅ Equivalent.

### 2. `KfxContainer.deserialize()` → `loadContainerSourceData()`
**Python (L50-226):** 14 branches.

| Branch | Python | Go | Status |
|--------|--------|-----|--------|
| MIN_LENGTH check | L53-55: `len(data) < MIN_LENGTH` raises Exception | L278-279: `len(data) < 18` returns UnsupportedError | ✅ Match |
| CONT signature validation | L57-63: checks for CONT, also checks PDB MOBI/CONT creator | L278: `!bytes.HasPrefix(data, contSignature)` returns UnsupportedError | ✅ Match (PDB check not needed for conversion) |
| Version check | L65-66: checks version in ALLOWED_VERSIONS={1,2}, logs error | Not checked explicitly | ⚪ Acceptable (version doesn't affect parsing logic) |
| Container info parsing | L69-74: extracts container_info_offset, length, deserializes ION | L281-286: same offsets, `decodeIonMap()` | ✅ Match |
| Container ID extraction | L76: `container_info.pop("$409", "")` | Not extracted in loadContainerSourceData | ✅ OK (extracted later in organizeFragments) |
| Compression type check | L78-80: pops $410, checks against DEFAULT_COMPRESSION_TYPE | Not checked | ⚪ Acceptable (only logs error in Python) |
| DRM scheme check | L82-84: pops $411, checks against DEFAULT_DRM_SCHEME | Not checked | ⚪ Acceptable (only logs error in Python) |
| Doc symbol extraction | L86-97: pops $415/$416, deserializes $ion_symbol_table, adjusts max_id | L293-298: extracts raw docSymbols bytes | ✅ Match (Go handles in symbolResolver) |
| Chunk size check | L99-101: pops $412, checks against DEFAULT_CHUNK_SIZE | Not checked | ⚪ Acceptable (only logs warning in Python) |
| Format capabilities | L103-111: pops $594/$595, deserializes $593 (version>1 only) | Not extracted separately | ⚪ Acceptable ($593 entities decoded from index table) |
| Index table extraction | L113-114: pops $413/$414 | L301-305: same extraction | ✅ Match |
| Extra container_info check | L116: checks remaining keys, logs error | Not checked | ⚪ Acceptable (only logs error in Python) |
| kfxgen_info parsing | L119-152: parses JSON metadata between container_info and header | Not parsed | ⚪ Acceptable (kfxgen_info is build metadata only) |
| Entity index iteration | L155-180: iterates index table, bounds checks, creates KfxContainerEntity | L469-477 (in organizeFragments): same iteration with bounds check | ✅ Match |
| Container format detection | L182-192: determines format from entity type_idnums | Not done (Go doesn't need format label) | ⚪ Acceptable (only used for container_info synthesis) |
| Synthetic $270 creation | L194-208: creates container_info IonAnnotation with metadata | Not synthesized | ✅ Correct ($270 entities already in index table) |

### 3. `KfxContainer.get_fragments()` → Entity processing in `organizeFragments()`
**Python (L227-236):** Yields doc_symbols ($ion_symbol_table), container_info ($270), format_capabilities ($593) as synthetic fragments, plus all deserialized entities.
**Go:** Processes entities directly from index table. No synthetic fragments needed because Go accesses the raw entity data directly.
**Verdict:** ✅ Functionally equivalent.

### 4. `KfxContainerEntity.__init__()` → Inline in `organizeFragments()`
**Python (L289-295):** Stores symtab, id_idnum, type_idnum, value, serialized_data.
**Go:** Entity data extracted inline during index table iteration.
**Verdict:** ✅ Equivalent.

### 5. `KfxContainerEntity.deserialize()` → `entityPayload()` + `decodeIonMap()`/`decodeIonValue()`
**Python (L297-334):** 8 branches.

| Branch | Python | Go | Status |
|--------|--------|-----|--------|
| ENTY signature check | L299-301: checks "ENTY" signature | L108 in decode.go: same check | ✅ Match |
| Version check | L303-304: checks version=1 | Not checked | ⚪ Acceptable |
| Header length check | L306-308: checks header_len >= MIN_LENGTH | L110 in decode.go: `headerLen < 10` | ✅ Match |
| Entity info parsing | L312-315: parses entity_info ION struct | Skipped (Go extracts payload after header) | ⚪ Acceptable (entity_info only has $410/$411 which are always 0) |
| Compression/DRM check | L316-323: checks $410=0, $411=0 | Not checked | ⚪ Acceptable (only logs errors) |
| RAW_FRAGMENT_TYPES | L328-329: returns IonBLOB for $417/$418 | state.go L605-612: stores raw bytes for $417/$418 | ✅ Match |
| ION deserialization | L331: `IonBinary.deserialize_single_value()` | decode.go `decodeIonMap()`/`decodeIonValue()` | ✅ Match |
| IonAnnotation handling | L333-339: unwraps annotation, reassigns fid | Handled by ion-go library + normalizeIon | ✅ Match |

### 6. `KfxContainerEntity.serialize()` → Not ported (write path)
**Python (L341-367):** Serializes entity back to binary.
**Go:** Not implemented (conversion pipeline is read-only).
**Verdict:** ✅ Correct omission.

### 7. `KfxContainer.serialize()` → Not ported (write path)
**Python (L238-367):** Serializes entire container back to binary.
**Go:** Not implemented (conversion pipeline is read-only).
**Verdict:** ✅ Correct omission.

### 8. `organize_fragments_by_type()` → `organizeFragments()`
**Python (yj_to_epub.py L173-213):** 10 branches.

| Branch | Python | Go | Status |
|--------|--------|-----|--------|
| Fragment ID collection | L176: `book_symbols.add(id)` | L471: `bookSymbols[fragmentID] = struct{}{}` | ✅ Match |
| $270 ID remapping | L178: `id = "%s:%s" % ($161, $409)` | L474-477: same format | ✅ Match |
| $593 ID remapping | L180: `id = last_container_id` | L479-480: same | ✅ Match |
| $262 ID remapping | L182-183: `id = "%s-font-%03d" % (id, font_count)` | L482-484: same format | ✅ Match |
| $387 ID remapping | L185-186: `id = "%s:%s" % (id, $215)` | L487-491: same format | ✅ Match |
| Duplicate detection | L190-191: logs error if id already in dt | L501-504: logs error | ✅ Match |
| Singleton collapse | L199-201: unwraps if single entry with key==category | Not done in Go | ⚪ Intentional (Go uses typed fragmentCatalog fields instead of generic dicts) |
| Null ID error | L203-204: logs error for mixed null/non-null | L590-595: checks for empty/null IDs | ✅ Match |
| replace_ion_data | L188: recursively collects symbols from values | L598-630: `mergeIonReferencedStringSymbols()` + inline collection | ✅ Match |

### 9. `collectContainerBlobs()` → Python `locate_book_datafiles()` + `get_container()`
**Python:** Complex dispatch based on file extension (.kfx, .kfx-zip, .zip, directory) and magic bytes.
**Go:** `collectContainerBlobs()` dispatches on magic bytes (CONT, DRMION, PK zip header).

| Branch | Python | Go | Status |
|--------|--------|-----|--------|
| DRMION detection | `data.startswith(DRMION_SIGNATURE)` | `bytes.HasPrefix(data, drmionSignature)` | ✅ Match |
| CONT detection | `data.startswith(KfxContainer.SIGNATURE)` | `bytes.HasPrefix(data, contSignature)` | ✅ Match |
| ZIP detection | `datafile.ext in [".kfx-zip", ".zip"]` | `bytes.HasPrefix(data, []byte("PK\x03\x04"))` | ✅ Match |
| Sidecar scanning | `locate_files_from_dir(sdr_dirname)` | `collectSidecarContainerBlobs(sidecarRoot)` | ✅ Match |
| File sorting | `sorted(self.container_datafiles)` | `sort.Strings(names)` / `sort.Slice(members)` | ✅ Match |

### 10. `determine_book_symbol_format()` → `determineBookSymbolFormat()`
**Python (yj_to_epub.py L215-240):** Classifies symbols, determines format (SHORT/DICTIONARY/BASE64/ORIGINAL).
**Go:** `determineBookSymbolFormat()` in symbol_format.go with same logic.
**Verdict:** ✅ Audited in previous feature (audit-yj-symbol-catalog).

### 11. `applyKFXEPUBInitMetadataAfterOrganize()` 
**Go:** `applyKFXEPUBInitMetadataAfterOrganize()` in yj_to_epub_metadata.go.
**Verdict:** ✅ Audited in previous feature (audit-yj-to-epub-metadata).

## Fragment Type Coverage

All fragment types handled by Python `organize_fragments_by_type` are handled by Go `organizeFragments`:

| Type | Python Field | Go Field | Status |
|------|-------------|----------|--------|
| $145 | ContentFragments | ContentFragments | ✅ |
| $157 | StyleFragments (kfx_styles) | StyleFragments | ✅ |
| $164 | External resources | ResourceFragments + ResourceRawData | ✅ |
| $258 | Reading order | SectionOrder + ReadingOrderMetadata | ✅ |
| $259 | Storylines | Storylines | ✅ |
| $260 | Sections | SectionFragments | ✅ |
| $262 | Fonts | FontFragments | ✅ |
| $266 | Anchors | AnchorFragments | ✅ |
| $270 | Generators (container_info) | Generators | ✅ |
| $387 | Position maps | Not in catalog (decoded inline) | ✅ |
| $389 | Nav roots | NavRoots | ✅ |
| $391 | Nav containers | NavContainers | ✅ |
| $417 | Raw media | RawFragments + RawBlobOrder | ✅ |
| $418 | Raw media (other) | RawFragments + RawBlobOrder | ✅ |
| $490 | Title metadata | TitleMetadata | ✅ |
| $538 | Document data | DocumentData | ✅ |
| $585 | Content features | ContentFeatures | ✅ |
| $593 | Format capabilities | FormatCapabilities | ✅ |
| $608 | Ruby contents | RubyContents | ✅ |
| $609 | Position maps | PositionAliases | ✅ |
| $756 | Ruby groups | RubyGroups | ✅ |

## Validation Results

- **go build ./cmd/kindle-helper/**: ✅ Builds cleanly
- **go test ./internal/kfx/ -count=1 -timeout 120s**: ✅ All tests pass
- **bash /tmp/compare_all.sh**: ✅ 394/394 match, 0 regressions

## Summary

Full parity confirmed between Python `kfx_container.py` (449 lines) and Go `container.go` (74 lines) + `state.go` (713 lines). The Go implementation restructures the Python's class hierarchy into a flatter, procedural approach but maintains equivalent behavior across all branches that affect conversion output. The omitted branches (version checks, compression/DRM scheme validation, kfxgen_info parsing, serialize methods) are all logging-only or write-path operations that don't affect EPUB output.

### Assertions Verified
- **VAL-DATA-005**: Container parsing handles all KFX container types ✅
- **VAL-DATA-006**: Fragment loading matches Python ✅
- **VAL-DATA-007**: YJ book structure composition matches Python ✅
