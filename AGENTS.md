# AGENTS.md — kindle.koplugin Project Instructions

> **Read this file at the start of every session.** It contains hard rules, architectural context, and reference material required to work on this project correctly.

- Commit after every step — success OR failure, there must always be a commit
- Never accumulate more than one step of uncommitted changes
- If a change introduces new UNEXPECTED test failures or *unexpected* diffs: revert immediately, commit the revert, then figure out why. Regressions are only allowed as temporary artifacts if they are part of a refactor that makes progress on the task. If you cannot explicitly say that they are and why, you must revert it.
- When making plans, You must add specific locations in the python code that we are referencing for each thing we are implementing in go. We are explicitly using the python as the source of truth. File, line, implementation details. Map it 1:1 python to go

---

## YJ Symbol Catalog — Real Names

Go uses **human-readable symbol names** throughout the conversion pipeline. The 842 YJ shared
symbol names are embedded at compile time from `internal/kfx/catalog.ion` (ION text format,
sourced from `REFERENCE/kfx_symbol_catalog.ion`).

**Python uses `$N` placeholders internally** (`$145`, `$179`, etc.) and only translates to
real names at the ION text boundary. **Go uses real names directly** in all decoded ION data.

| SID | Go name | Python placeholder |
|-----|---------|-------------------|
| 145 | `content` | `$145` |
| 146 | `content_list` | `$146` |
| 179 | `link_to` | `$179` |
| 601 | `render` | `$601` |
| 260 | `section` | `$260` |

When reading Python source, translate `$N` → real name using the catalog. When the Python
checks `if "$145" in value`, the Go equivalent is `if "content" in value`.

### Wire-Format Identifiers — When to Use `$N` in Go

Although Go uses real names internally, some output formats require the raw `$N` symbol ID
to match Calibre's wire format. The canonical example is `navTypeToSymbolID` in
`yj_to_epub_navigation.go`, which converts human-readable nav type names back to `$N` form
for anchor names (e.g., `$798_0_Chapter` instead of `headings_0_Chapter`). When producing
output that Calibre consumers parse, check whether the format expects `$N` identifiers and
use the reverse mapping (`symbolNameToID` table or equivalent) rather than real names.

### Golden-File Parity Tests

Golden tests verify Go's embedded catalog matches the Calibre Python reference:

```sh
# Regenerate goldens (run after updating catalog.ion or yj_versions.py)
python3 scripts/export_yj_symbol_catalog.py > internal/kfx/testdata/yj_symbols_golden.json
python3 scripts/export_yj_versions.py > internal/kfx/testdata/yj_versions_golden.json
```

If a Calibre bump changes `yj_symbol_catalog.py` without updating Go goldens, CI fails.

### Reference Snapshot Translation

`scripts/kfx_reference_snapshot.py` loads the Calibre Python code with the symbol catalog
and translates `$N` output to real names so Go↔Python comparison tests work correctly.

---

## Python Porting Rules — READ BEFORE EVERY PORTING TASK

This project has a recurring failure mode: **partial Python ports that silently drop unhandled branches**. Every recent diff was caused by the same pattern — porting only the "happy path" from Python while skipping branches that seem irrelevant at the time.

### The Pattern (DO NOT REPEAT)

| Fix | What We Skipped |
|-----|----------------|
| Elvis `font-size: 1em` | Python uses one `inherited_properties` dict; Go split into `inherited` + `comparisonInherited` but didn't sync updates between them |
| HG `link_to` link wrapping | Python's `find_or_create_style_event_element` handles `img`, `svg`, `div`, `a`, `aside`, `figure`, headings, `li`, `ruby`, `rb`; Go reduced all of this to `if elem.Tag != "span" { continue }` |
| Elvis table cell `<p>` wrappers | Python's COMBINE_NESTED_DIVS operates on any child type; Go only checked `content` text children |
| Elvis logo FIRST EDITION | Python's `process_content` handles IonString, IonSymbol, and IonStruct children in `content_list`; Go only handled IonStruct (map) children |
| Elvis inline image wrapping | Python checks `render` mode to skip container `<div>` for inline images; Go wrapped all images unconditionally |

### Mandatory Process: Branch Audit

When porting a Python function to Go, you MUST:

1. **List every branch** in the Python function — every `if`, `elif`, `for`, `try`, ternary, and type dispatch.
2. **Map each branch** to specific Go code (file + line).
3. **Flag gaps** — any Python branch with no Go counterpart is a bug, not a "nice to have later."
4. **Implement ALL branches**, not just the one triggered by the current diff. If a branch seems unreachable, add a `log.Errorf` for it instead of silently dropping it.
5. **Do NOT simplify** Python's type dispatches. When Python handles `img`, `svg`, `div`, `a`, `aside`, `figure`, `h1`-`h6`, `li`, `ruby`, `rb` separately, port ALL of those cases. Do NOT replace them with `if tag == "span" { ... } else { continue }`.

### When Modifying Existing Go Code

Before adding or modifying a function that processes HTML elements or KFX content:

1. Find the Python function it was ported from.
2. Re-read it. Python may handle cases you previously skipped.
3. Check: does the Python function iterate over element types, content types, or property types that Go doesn't?
4. Check: does Python have error/log branches for unexpected cases that Go silently ignores?
5. Check: does Python modify data structures in-place (single dict) where Go uses separate copies that can drift apart?

### Red Flags

If you catch yourself writing any of these, STOP and re-read the Python:

- `if elem.Tag != X { continue }` — Python almost certainly handles other tags
- `if _, ok := asMap(child); !ok { continue }` — Python handles non-map children (strings, symbols)
- `if _, ok := asSlice(node["content_list"]); !ok { return nil }` — Python's `add_content` also checks `content` and `story_name`
- `// Skip for now` or `// Not needed` — it IS needed, you just haven't hit the book that triggers it yet
- Any code checking `strings.HasPrefix(name, "$")` to identify shared symbols — use `isSharedSymbolName(name)` instead

---

## Project Overview

**kindle.koplugin** is a [KOReader](https://koreader.rocks/) plugin that lets Kindle device owners browse and read their Kindle-native library books inside KOReader. It does this by:

1. **Scanning** the Kindle's on-device document library (KFX files in `/mnt/us/documents/`)
2. **Decrypting** DRM-protected books using on-device key extraction
3. **Converting** KFX → EPUB via a Go helper binary
4. **Caching** converted EPUBs for fast re-opening
5. **Presenting** a virtual library UI inside KOReader's file browser

The plugin is modeled after the reference implementation at `REFERENCE/kobo.koplugin/`, which does the same thing for Kobo devices.

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│  KOReader (Lua)                                      │
│  main.lua ← entry point                              │
│  src/*.lua ← plugin modules                          │
│    helper_client.lua → shells out to Go binary        │
│    cache_manager.lua → EPUB cache lifecycle           │
│    virtual_library.lua → browse/search UI             │
│    library_index.lua → book scanning & indexing       │
│    document_ext.lua → DocumentRegistry patches        │
│    filechooser_ext.lua → file browser integration     │
│    docsettings_ext.lua → sidecar settings support     │
└──────────────┬──────────────────────────────────────┘
               │ JSON stdin/stdout (via io.popen)
┌──────────────▼──────────────────────────────────────┐
│  kindle-helper (Go static binary — ARM cross-compile)│
│  cmd/kindle-helper/main.go                           │
│  Subcommands: scan, convert, drm-init (planned)      │
│                                                      │
│  internal/kfx/    → KFX decode + YJ→EPUB conversion  │
│  internal/epub/   → EPUB packaging (NCX/NAV)         │
│  internal/jxr/    → JPEG XR decoder                  │
│  internal/jsonout/ → JSON output types                │
│  internal/scan/   → Kindle document scanner           │
└──────────────────────────────────────────────────────┘
               │ (for DRM) shells out to device cvm
┌──────────────▼──────────────────────────────────────┐
│  DRM Helper Files (bundled in plugin)                 │
│  lib/KFXVoucherExtractor.jar → Java JNI → DRMSDK     │
│  lib/crypto_hook.so         → LD_PRELOAD AES hook    │
│  drm_keys.json              → cached decryption keys  │
└──────────────────────────────────────────────────────┘
```

### Data Flow

```
User opens book in KOReader
  → document_ext.lua intercepts open
  → cache_manager checks for cached EPUB
  → if not cached: helper_client:convert(kfx_path, epub_path)
  → Go binary detects DRMION wrapper
  → if DRM: decrypts using cached page keys from drm_keys.json
  → Go converts KFX → EPUB (existing conversion pipeline)
  → Lua feeds EPUB path to KOReader's crengine
  → Book renders
```

---

## Tech Stack

| Component | Language | Notes |
|-----------|----------|-------|
| KOReader plugin frontend | Lua | Runs inside KOReader's LuaJIT environment |
| KFX→EPUB conversion + DRM | Go | Statically compiled for ARM (armv5/armv7/arm64) |
| DRM voucher extraction | Java (tiny) | ~30 lines, runs on device's `cvm` JVM |
| AES key interception | C (tiny) | ~60 lines, LD_PRELOAD hook into device crypto lib |
| KOReader integration hooks | Lua | Monkey-patches on KOReader's DocumentRegistry, FileChooser, etc. |

---

## Directory Layout

```
/
├── AGENTS.md                  ← YOU ARE HERE
├── README.md
├── PARITY_PROMPT.md           ← Go↔Python parity rules
├── _meta.lua                  ← KOReader plugin metadata
├── main.lua                   ← Plugin entry point (loaded by KOReader)
├── go.mod / go.sum
├── arm_build.sh               ← Cross-compile + package script
│
├── cmd/kindle-helper/
│   ├── main.go                ← CLI: scan, convert, (planned: drm-init)
│   └── main_test.go
│
├── internal/
│   ├── kfx/                   ← Core KFX→EPUB conversion engine (1:1 Python↔Go file map)
│   │   ├── catalog.ion        ← Embedded YJ symbol names (842 human-readable names)
│   │   └── testdata/           ← Golden JSON files for parity tests
│   ├── epub/                  ← EPUB packaging
│   ├── jxr/                   ← JPEG XR decoding
│   ├── jsonout/               ← JSON output types for CLI responses
│   └── scan/                  ← Document scanning & classification
│
├── lua/                       ← Lua plugin modules
│   ├── helper_client.lua      ← Go binary client (scan, convert, drm-init)
│   ├── cache_manager.lua      ← EPUB cache lifecycle (freshness, cleanup)
│   ├── virtual_library.lua    ← Virtual library path management & book entries
│   ├── library_index.lua      ← Book scanning & metadata indexing
│   ├── document_ext.lua       ← DocumentRegistry monkey-patches
│   ├── filechooser_ext.lua    ← File browser integration patches
│   ├── docsettings_ext.lua    ← Sidecar settings support
│   ├── showreader_ext.lua     ← ReaderUI:showReader intercept
│   ├── filesystem_ext.lua     ← lfs.attributes virtual path patching
│   ├── readerui_ext.lua       ← ReaderUI close → virtual library navigation
│   ├── pathchooser_ext.lua    ← PathChooser bypass virtual library
│   ├── bookinfomanager_ext.lua← CoverBrowser metadata integration
│   └── lib/                   ← Shared utility modules (from kobo.koplugin)
│       ├── pattern_utils.lua  ← Lua pattern magic char escaping
│       └── session_flags.lua  ← Session-persistent flag files in /tmp
│
├── spec/                      ← Busted test suite (264 specs)
├── patches/                   ← KOReader startup patches
│   └── 2-kindle-virtual-library-startup.lua ← ffi/util.realpath virtual path support
│
├── scripts/                   ← Dev/CI scripts
│   ├── test                   ← Busted runner under luajit
│   ├── export_yj_symbol_catalog.py ← Export golden from ION catalog
│   ├── export_yj_versions.py  ← Export KNOWN_FEATURES golden from Python
│   ├── kfx_reference_snapshot.py ← Run Calibre Python, translate $N → real names
│   └── replace_symbol_names.py ← Mechanical $N → real name replacement tool
│
├── REFERENCE/                 ← NOT tracked in git — local reference only
│   ├── kobo.koplugin/         ← Sister plugin (Kobo) — architectural reference
│   ├── koreader/              ← KOReader source — for understanding KOReader APIs
│   ├── localsend.koplugin/    ← Another KOReader plugin reference
│   ├── KFX_DRM/               ← DRM research: hooks, extractors, scripts, vouchers
│   ├── kindle_drm_classes/    ← Decompiled Kindle DRM Java classes
│   ├── DeDRM_tools/           ← DeDRM plugin source (Python DRM removal)
│   ├── KFX_DRM_INTEGRATION.md ← Detailed DRM integration plan
│   ├── KFX_DRM_RESEARCH.md   ← Full DRM research notes (~1200 lines)
│   └── kobo_virtual_library_findings.md
│
└── .factory/                  ← Factory/Multi-agent framework (research, skills, validation)
```

---

## Hard Rules

### 1. Python Reference is Source of Truth

`REFERENCE/Calibre_KFX_Input/` (or equivalent Python reference) is the **sole source of truth** for all KFX→EPUB conversion logic. The Go port in `internal/kfx/` must maintain strict three-fold parity:

- **Structural** — Matching file names and directory layout
- **Function-level** — Matching function names, signatures, and purposes
- **Logic-level** — Matching control flow, return values, and edge-case behavior

**The Python code is NEVER modified** except for debug logging.

### 2. Resolution Order: Structure → Function → Logic

When fixing parity gaps, always fix the foundation first:
1. Ensure file structure matches
2. Ensure all functions exist with correct signatures
3. Ensure logic produces identical outputs

### 3. Top-level Python Files Do NOT Need Go Ports

Files like `action.py`, `config.py`, `jobs.py` in the Calibre reference are Calibre plugin infrastructure. The Lua code in `src/` replaces them.

### 4. Every Change Must Be Tested

- Go: `go test ./...`
- Lua: `./scripts/test` (busted under luajit)
- Some tests require KFX fixture files not in the repo — these auto-skip
- New Lua modules **must** include a corresponding `spec/*_spec.lua`
- Spec structure and mocking patterns follow `REFERENCE/kobo.koplugin/spec/`

### 5. Commits Should Be Atomic

Each logical step gets its own commit. If something breaks, revert and fix before moving on.

---

## DRM Integration (In Progress)

The DRM approach uses **on-device key extraction via LD_PRELOAD** with **just-in-time key refresh**.

### How It Works

1. Device stores DRM vouchers in `*.sdr/assets/voucher` alongside each `.kfx` file
2. Device serial available at `/proc/usid`
3. Account secret (ACSR) stored at `/var/local/java/prefs/acsr`
4. The `drm-init` command runs the device's `cvm` JVM with an LD_PRELOAD hook that intercepts AES key usage
5. The hook logs keys to `/mnt/us/crypto_keys.log`
6. A tiny Java class (`KFXVoucherExtractor.jar`) exercises the DRM SDK, triggering key usage
7. Go code parses the log, matches keys to vouchers, extracts 16-byte page keys
8. Page keys are cached in `drm_keys.json`
9. **JIT retry loop**: when conversion fails due to stale keys, the Lua layer auto-triggers
   key extraction for that specific book and retries — transparent to the user

### DRM File Signatures

| Format | Magic Bytes | Meaning |
|--------|------------|---------|
| DRMION | `\xeaDRMION\xee` | DRM-encrypted KFX |
| CONT | `\xeaCONT\xee` | Container KFX (unencrypted) |
| Voucher | `\xe0\x01\x00\xea` + contains `ProtectedData` | DRM voucher |

### Key Stability — CORRECTED

**Keys are NOT deterministic across re-downloads.** Amazon's delivery service generates a
fresh voucher with new ciphertext on every delivery, even for the same content version.
A cached page key becomes invalid whenever the device re-downloads a book's assets.

Re-download triggers include:
- Opening a book in the Kindle reader (triggers asset check)
- Background sync / storage scans (periodic, automated)
- Amazon pushing content updates (formatting, metadata changes)

Full evidence: see `REFERENCE/KFX_DRM_RESEARCH.md` — "Key Stability Investigation".

The JIT approach handles this transparently — stale keys are detected and refreshed
automatically when the user opens a book. No manual intervention needed.

### Planned Go Code for DRM

| File | Purpose |
|------|---------|
| `internal/kfx/drm.go` | `DecryptDRMION()`, `ExtractPageKey()`, `ParseVoucherIon()`, `LoadDRMKeys()`, `FindPageKey()` |
| `internal/kfx/drmion.go` | DRMION page decryption (ION binary → encrypted sections → decrypt → concatenate to CONT KFX) |
| `cmd/kindle-helper/main.go` | Add `drm-init` subcommand (targeted and batch modes) |
| `internal/kfx/yj_to_epub.go` | Modify `ConvertFile` to handle DRMION with stale-key detection |

### Planned Lua Code for DRM

| File | Changes |
|------|---------|
| `lua/helper_client.lua` | Add `drmInit(voucher_paths)` and `drmInitAll()` methods |
| `lua/cache_manager.lua` | JIT DRM retry loop — auto-extract key on `drm_key_stale` error |
| `lua/virtual_library.lua` | Show DRM-specific status text |
| `main.lua` | Add "Decrypt all DRM books" and "Refresh DRM keys" menu items |

Full plan: see `REFERENCE/KFX_DRM_INTEGRATION.md`.

---

## KOReader Plugin Conventions

KOReader plugins live in a `<name>.koplugin/` directory and must contain:
- `_meta.lua` — plugin metadata (name, description, version)
- `main.lua` — entry point, must return a `WidgetContainer` subclass

Key KOReader APIs used:
- `DocumentRegistry` — register/intercept document open handlers
- `FileChooser` — file browser widget (we patch for virtual library)
- `UIManager` — show dialogs, schedule tasks
- `InfoMessage` / `ConfirmBox` — standard dialog widgets
- `DataStorage` — paths for plugin data/cache
- `G_reader_settings` — persistent settings store
- `WidgetContainer` — base class for plugins
- `gettext` (`_()`) — localization

### Monkey-Patching Pattern

The plugin extends KOReader by monkey-patching core classes at runtime. Each `*_ext.lua` module follows this pattern:
1. `init(deps)` — receive references to virtual_library, cache_manager, etc.
2. `apply(TargetClass)` — replace or wrap methods on the target class
3. Store original methods for fallback/chaining

### KOReader Source Reference

`REFERENCE/koreader/` contains the full KOReader source. Use it to understand APIs, widget lifecycle, and rendering. Key directories:
- `frontend/document/documentregistry.lua`
- `frontend/ui/widget/filechooser.lua`
- `frontend/ui/widget/*.lua`
- `frontend/docsettings.lua`

---

## Testing

### Running Tests

```sh
# Lua tests (busted under luajit — matches KOReader's runtime)
./scripts/test

# Go tests
go test ./...

# Run a single spec file
./scripts/test spec/virtual_library_spec.lua

# Run with the system busted (lua5.5, for convenience)
busted --lua=lua
```

**Always use `./scripts/test` for CI/validation** — KOReader runs LuaJIT on-device, so we test against it. The `scripts/test` wrapper sets `LUA_PATH`/`LUA_CPATH` so busted's modules are findable under luajit.

### Golden-File Tests

Go has golden-file parity tests that compare embedded symbol data against the Calibre Python reference:

```sh
# Regenerate symbol catalog golden (after updating catalog.ion)
python3 scripts/export_yj_symbol_catalog.py > internal/kfx/testdata/yj_symbols_golden.json

# Regenerate yj_versions golden (after Calibre bump changes KNOWN_FEATURES)
python3 scripts/export_yj_versions.py > internal/kfx/testdata/yj_versions_golden.json
```

If these tests fail, it means the Go code has drifted from the Python source of truth. Either:
1. Update the golden by re-running the export script (if the Python changed), or
2. Fix the Go code (if the golden is correct but Go diverged)

### Test Structure

Follows `REFERENCE/kobo.koplugin/spec/` patterns:

```
spec/
├── helper.lua                    # Mock setup (loaded before every spec)
│                                 # Provides: logger, util, json, lfs, device,
│                                 # ui/uimanager, docsettings, datastorage, etc.
├── virtual_library_spec.lua      # 48 tests
├── cache_manager_spec.lua        # 16 tests
├── library_index_spec.lua        #  9 tests
├── helper_client_spec.lua        # 12 tests
├── pattern_utils_spec.lua        # 15 tests
├── session_flags_spec.lua        #  5 tests
├── filesystem_ext_spec.lua       # 10 tests
├── docsettings_ext_spec.lua      #  4 tests
└── filechooser_ext_spec.lua      #  7 tests
```

### Writing New Specs

1. Create `spec/<module_name>_spec.lua`
2. `require("spec/helper")` in `setup()` — this loads all mocks
3. Clear `package.loaded` for your module in `before_each()` to get fresh instances
4. Use `resetAllMocks()` in `before_each()` to reset G_reader_settings and UIManager state
5. Use `createIOOpenMocker()` for tests that need to control file I/O
6. Follow the `describe`/`it`/`assert` patterns from existing specs

### Key Mocking Conventions

- **`spec/helper.lua`** pre-registers mocks via `package.preload` for all KOReader APIs
- **`_G.G_reader_settings`** — global mock with `readSetting`/`saveSetting`/`isTrue`
- **`createIOOpenMocker()`** — scoped `io.open` mocking for cache/file tests
- **`libs/libkoreader-lfs`** — mock with `_setFileState`/`_setDirectoryContents` test helpers
- **Never mock the module under test** — only mock its dependencies

### When Adding New Lua Modules

Every new `lua/*.lua` or `lua/lib/*.lua` module should have a corresponding spec. At minimum:
- Initialization / constructor tests
- Each public method with success and failure cases
- Edge cases (nil inputs, empty strings, missing data)

---

## Build & Deploy

```sh
# Native build (for development/testing)
go build ./cmd/kindle-helper

# Run all tests
./scripts/test          # Lua
go test ./...           # Go

# Cross-compile for Kindle ARM targets
./arm_build.sh
# Produces build/kindle-koplugin-{arm-legacy,armv7,arm64}.zip

# Deploy to device
# Copy the zip contents to /mnt/us/koreader/plugins/kindle.koplugin/ on the Kindle
```

The `arm_build.sh` script:
1. Compiles Go binary for armv5 (legacy), armv7, and arm64
2. Packages each with Lua plugin files into a ZIP
3. Each ZIP contains `kindle.koplugin/` directory ready to extract to the device

---

## Key References (Read When Needed)

| Document | When to Read |
|----------|-------------|
| `REFERENCE/KFX_DRM_INTEGRATION.md` | Any DRM-related work |
| `REFERENCE/KFX_DRM_RESEARCH.md` | Deep DRM technical details, ION format, key derivation |
| `REFERENCE/kobo.koplugin/` | When implementing Lua UI, virtual library, or KOReader integration |
| `REFERENCE/kobo.koplugin/main.lua` | Plugin structure and menu registration pattern |
| `REFERENCE/kobo.koplugin/src/` | Lua module patterns (virtual library, metadata, extensions) |
| `REFERENCE/kobo.koplugin/src/virtual_library.lua` | Virtual library UI implementation reference |
| `REFERENCE/kobo.koplugin/spec/` | Test patterns, mocking approach, spec structure reference |
| `REFERENCE/GAPS.md` | Feature gap analysis vs kobo.koplugin |
| `REFERENCE/COPYABILITY.md` | Which kobo modules can be copied and the upstream sync strategy |
| `REFERENCE/koreader/` | When you need to understand KOReader internals |
| `REFERENCE/DeDRM_tools/` | Python DRM removal reference (original algorithms) |
| `REFERENCE/kindle_drm_classes/` | Decompiled Kindle Java DRM classes |
| `REFERENCE/KFX_DRM/` | Working DRM extraction code (hooks, extractors, scripts) |
| `PARITY_PROMPT.md` | Detailed parity audit instructions and rules |
| `internal/kfx/catalog.ion` | Embedded YJ symbol catalog (842 real names, source of truth for Go) |
| `REFERENCE/kfx_symbol_catalog.ion` | Master ION catalog file (copy to `internal/kfx/catalog.ion` to update) |
| `.factory/library/architecture.md` | Additional architecture notes |
| `.factory/library/python-porting-guide.md` | Python→Go porting guidelines |

---

## Device Context

The target is a Kindle e-reader (typically Paperwhite or similar) running KOReader alongside the stock Kindle firmware. Key device paths:

| Path | Purpose |
|------|---------|
| `/mnt/us/documents/` | Kindle document library root |
| `/mnt/us/koreader/` | KOReader installation |
| `/mnt/us/koreader/plugins/` | KOReader plugins directory |
| `/proc/usid` | Device serial number |
| `/var/local/java/prefs/acsr` | Account secret (ACSR) |
| `/usr/java/bin/cvm` | Device JVM (used for DRM key extraction) |
| `*/assets/voucher` | DRM voucher files (per-book, alongside `.kfx`) |

The Go binary is statically compiled (`CGO_ENABLED=0`) for Linux ARM. No shared libraries required at runtime except what's already on the device.

---

## Test Fixtures & Comparison Books

The project has 6 real books from a Kindle device. When comparing Go output against Calibre reference EPUBs, **always use the DRMION books as primary comparison targets**. Martyr is an unencrypted (CONT) book that produces byte-identical output — it is only useful for regression testing.

### Book Inventory

| Book | Format | Primary Use |
|------|--------|-------------|
| **1984** | DRMION | Structural diffs — `<a>` class, body class naming, table cell wrapping, CSS ordering |
| **Sunrise Reaping** | DRMION | Extra `id` attrs on image-heading divs (31 files) |
| **Secrets Crown** | DRMION | JXR images not decoded, CSS class ordering, class index swap |
| **Hunger Games Trilogy** | DRMION | Images only (JFIF re-encoding) |
| **Throne of Glass** | DRMION | Images only |
| **Elvis and the Underdogs** | DRMION | Images only |
| **The Familiars** | DRMION | Images only |
| **Three Below (Floors #2)** | DRMION | Images only |
| **Heated Rivalry** | DRMION | Images only |
| **Martyr** | CONT | Images only (byte-identical content, JFIF diff) |

### Fixture Paths

All test fixtures live under `REFERENCE/books/<name>/` with consistent naming:

| File | Meaning | Present For |
|------|---------|-------------|
| `input.kfx` | CONT (unencrypted) source | Martyr |
| `input.kfx-zip` | Decrypted DRMION source (what Go converts) | All DRMION books |
| `original.kfx` | DRMION original (for Classify tests) | Old DRMION books |
| `voucher` | DRM voucher from device | All DRMION books |
| `calibre.epub` | Calibre golden reference | All books |

Top-level: `REFERENCE/books/drm_keys.json` — merged page keys for all 9 DRMION books.

### Current Parity Status (May 2026)

**All 10 books are structurally clean.** Zero structural diffs across all fixtures.
All remaining differences are image-only (JPEG re-encoding: Go outputs plain JPEG without JFIF header,
Calibre adds JFIF metadata; dimensions and pixel content are identical).

| Book | Structural | Image | Notes |
|------|-----------|-------|-------|
| Martyr | 0 | 5 | Image re-encoding only |
| 1984 | 0 | 7 | Image re-encoding only |
| Elvis | 0 | 30 | Image re-encoding only |
| Familiars | 0 | 28 | Image re-encoding only |
| Heated Rivalry | 0 | 7 | Image re-encoding only |
| Hunger Games | 0 | 46 | Image re-encoding only |
| Secrets Crown | 0 | 11 | Image re-encoding only |
| Sunrise Reaping | 0 | 39 | Image re-encoding only |
| Three Below | 0 | 16 | Image re-encoding only |
| Throne of Glass | 0 | 7 | Image re-encoding only |

**Image diff nature:** Go's JPEG encoder writes plain JPEG without JFIF APP0 marker. Calibre's PIL/Pillow
encoder adds JFIF metadata (version 1.01, resolution). Image dimensions and pixel content are identical.
Go files are slightly larger due to different quality settings.

### Known Parity Gaps (all image-level, no structural)

1. **JFIF header missing** — Go outputs raw JPEG without JFIF APP0 marker; Calibre includes it. Cosmetic only — all EPUB readers handle both.
2. **JPEG quality/encoding** — Go uses different JPEG quality settings than Calibre's Pillow, producing slightly different file sizes. Pixel content is identical.

---

## Common Gotchas

- **File paths on device are Linux ARM** — always cross-compile, never use dynamic linking
- **KOReader's Lua is LuaJIT** — use `util.shell_escape()` for shell commands, not raw string concat
- **JSON communication** — the Go binary writes JSON to stdout; Lua parses it. stderr is for debug logging only
- **KFX fixtures** — some tests need real KFX files not in the repo; they auto-skip if absent
- **DRM books have two files** — the `.kfx` (DRMION content) and `*.sdr/assets/voucher` (decryption voucher)
- **Cache invalidation** — cache is keyed on `source_mtime + source_size + converter_version`. Bumping `CONVERTER_VERSION` in `cache_manager.lua` forces re-conversion of all books
- **Lua module paths** — KOReader adds the plugin directory to `package.path`, so `require("lua/cache_manager")` resolves to `plugins/kindle.koplugin/lua/cache_manager.lua`
- **Shared modules from kobo.koplugin** — `lua/lib/pattern_utils.lua`, `lua/lib/session_flags.lua`, `lua/filesystem_ext.lua`, `lua/readerui_ext.lua`, `lua/pathchooser_ext.lua` are adapted from kobo. See `REFERENCE/COPYABILITY.md` for the sync strategy
