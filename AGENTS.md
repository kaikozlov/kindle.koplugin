# AGENTS.md — kindle.koplugin Project Instructions

> **Read this file at the start of every session.** It contains hard rules, architectural context, and reference material required to work on this project correctly.

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
│   ├── kfx/                   ← Core KFX→EPUB conversion engine (~40 files)
│   ├── epub/                  ← EPUB packaging
│   ├── jxr/                   ← JPEG XR decoding
│   ├── jsonout/               ← JSON output types for CLI responses
│   └── scan/                  ← Document scanning & classification
│
├── src/                       ← Lua plugin modules
│   ├── helper_client.lua      ← Go binary client (scan, convert, drm-init)
│   ├── cache_manager.lua      ← EPUB cache lifecycle (freshness, cleanup)
│   ├── virtual_library.lua    ← Virtual library UI widget
│   ├── library_index.lua      ← Book scanning & metadata indexing
│   ├── document_ext.lua       ← DocumentRegistry monkey-patches
│   ├── filechooser_ext.lua    ← File browser integration patches
│   └── docsettings_ext.lua    ← Sidecar settings support
│
├── scripts/                   ← Dev/CI scripts (parity diffs, Python reference)
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
- Lua: follow patterns from `REFERENCE/kobo.koplugin/spec/` (busted framework)
- Some tests require KFX fixture files not in the repo — these auto-skip

### 5. Commits Should Be Atomic

Each logical step gets its own commit. If something breaks, revert and fix before moving on.

---

## DRM Integration (In Progress)

The DRM approach uses **on-device key extraction** — no offline key derivation needed.

### How It Works

1. Device stores DRM vouchers in `*.sdr/assets/voucher` alongside each `.kfx` file
2. Device serial available at `/proc/usid`
3. Account secret (ACSR) stored at `/var/local/java/prefs/acsr`
4. The `drm-init` command runs the device's `cvm` JVM with an LD_PRELOAD hook that intercepts AES key usage
5. The hook logs keys to `/mnt/us/crypto_keys.log`
6. A tiny Java class (`KFXVoucherExtractor.jar`) exercises the DRM SDK, triggering key usage
7. Go code parses the log, matches keys to vouchers, extracts 16-byte page keys
8. Page keys are cached in `drm_keys.json` — deterministic, only needs refresh on new book downloads

### DRM File Signatures

| Format | Magic Bytes | Meaning |
|--------|------------|---------|
| DRMION | `\xeaDRMION\xee` | DRM-encrypted KFX |
| CONT | `\xeaCONT\xee` | Container KFX (unencrypted) |
| Voucher | `\xe0\x01\x00\xea` + contains `ProtectedData` | DRM voucher |

### Key Stability

Keys are **deterministic**: same ACSR + serial + voucher → same key every time. Key cache only needs refreshing when:
- A new DRM book is downloaded (new voucher)
- The ACSR changes (account re-registration — rare)
- The device is factory reset

### Planned Go Code for DRM

| File | Purpose |
|------|---------|
| `internal/kfx/drm.go` | `DecryptDRMION()`, `ExtractPageKey()`, `ParseVoucherIon()` |
| `internal/kfx/drmion.go` | DRMION page decryption (ION binary → encrypted sections → decrypt → concatenate to CONT KFX) |
| `cmd/kindle-helper/main.go` | Add `drm-init` subcommand |

### Planned Lua Code for DRM

| File | Changes |
|------|---------|
| `src/helper_client.lua` | Add `drmInit()` method |
| `src/cache_manager.lua` | Handle `open_mode == "drm"` — check key cache before converting |
| `src/virtual_library.lua` | Show DRM-specific status text |
| `main.lua` | Add "Setup DRM decryption" menu item |

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

## Build & Deploy

```sh
# Native build (for development/testing)
go build ./cmd/kindle-helper

# Run tests
go test ./...

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
| `REFERENCE/koreader/` | When you need to understand KOReader internals |
| `REFERENCE/DeDRM_tools/` | Python DRM removal reference (original algorithms) |
| `REFERENCE/kindle_drm_classes/` | Decompiled Kindle Java DRM classes |
| `REFERENCE/KFX_DRM/` | Working DRM extraction code (hooks, extractors, scripts) |
| `PARITY_PROMPT.md` | Detailed parity audit instructions and rules |
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

## Common Gotchas

- **File paths on device are Linux ARM** — always cross-compile, never use dynamic linking
- **KOReader's Lua is LuaJIT** — use `util.shell_escape()` for shell commands, not raw string concat
- **JSON communication** — the Go binary writes JSON to stdout; Lua parses it. stderr is for debug logging only
- **KFX fixtures** — some tests need real KFX files not in the repo; they auto-skip if absent
- **DRM books have two files** — the `.kfx` (DRMION content) and `*.sdr/assets/voucher` (decryption voucher)
- **Cache invalidation** — cache is keyed on `source_mtime + source_size + converter_version`. Bumping `CONVERTER_VERSION` in `cache_manager.lua` forces re-conversion of all books
