# State: organizeFragments & replace_ion_data Parity

## What was implemented (feature a1-organize-fragments-parity)

### Fragment ID Remapping (yj_to_epub.py:181-215)

All four special ID remapping cases are implemented in `organizeFragments()` (state.go):

- **$270** → `"{container_format}:{container_id}"` from `$161`/`$409` keys. Updates `lastContainerID`.
- **$593** → reuses `lastContainerID` from preceding `$270` (per-source scope).
- **$262** → `"{id}-font-{count:03d}"` with global font counter across all sources.
- **$387** → `"{id}:{value[$215]}"`.

### Duplicate & Null ID Detection

- `categorizedData` map tracks per-type ID sets during fragment processing.
- Duplicate IDs within same type: `log.Printf("kfx: book contains multiple %s fragments with id %s", ...)`.
- Null/empty IDs in categories with multiple entries: `log.Printf("kfx: fragment list contains mixed null/non-null ids of type %q", ...)`.

### Singleton Collapse

Python's singleton collapse (when category has 1 entry where key==category) is effectively handled by Go's typed `fragmentCatalog` fields — each fragment type routes directly to its typed field (TitleMetadata, DocumentData, etc.), so the dict→value unwiring is implicit.

### replace_ion_data / Symbol Collection (yj_to_epub.py:270-291)

`mergeIonReferencedStringSymbols()` recursively walks `map[string]interface{}` and `[]interface{}`, collecting string values under `$`-prefixed keys into `bookSymbols`. This parallels Python's `replace_ion_data` which recursively walks IonStruct/IonList/IonSExp collecting IonSymbol values. The Go approach differs because Ion symbols are already resolved to strings during decoding — only `$`-prefixed map keys contain symbol references.

`mergeContentFragmentStringSymbols()` iterates all string slices in ContentFragments and adds non-empty strings to bookSymbols.

### Section Order Derivation

Three-tier priority: `$258` reading orders → `$538` document data reading orders → sorted keys of SectionFragments.

## Key Design Decisions

1. **Per-source lastContainerID**: The `lastContainerID` for `$593` remapping is scoped per container source, matching Python's per-container processing.
2. **Global font counter**: The `$262` font count is global across all sources, matching Python.
3. **Typed fields vs categorized dict**: Go uses typed `fragmentCatalog` struct instead of Python's `categorized_data` dict, eliminating the need for explicit singleton collapse.

## Test Coverage

22 tests in `state_test.go` covering:
- Fragment ID remapping for all 4 special types
- Recursive symbol collection (nested maps and slices)
- Content fragment string collection
- Multi-source symbol accumulation
- Section order derivation (all 3 priority levels)
- Singleton collapse logic
- Null ID detection
- chooseFragmentIdentity, isResolvedIdentity, isPlaceholderSymbol helpers
