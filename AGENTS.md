# AGENTS.md — kindle.koplugin Project Instructions

> **Read this file at the start of every session.** It contains hard rules, architectural context, and reference material required to work on this project correctly.

- Commit after every step — success OR failure, there must always be a commit
- Never accumulate more than one step of uncommitted changes
- If a change introduces new UNEXPECTED test failures or *unexpected* diffs: revert immediately, commit the revert, then figure out why. Regressions are only allowed as temporary artifacts if they are part of a refactor that makes progress on the task.

---

## Project Overview

**kindle.koplugin** is a [KOReader](https://koreader.rocks/) plugin that lets Kindle device owners browse and read their Kindle-native library books inside KOReader. It does this by:

1. **Scanning** the Kindle's on-device document library (KFX files in `/mnt/us/documents/`)
2. **Decrypting** DRM-protected books using on-device key extraction
3. **Converting** KFX → EPUB via a Python helper binary (Nuitka-compiled for ARM)
4. **Caching** converted EPUBs for fast re-opening
5. **Presenting** a virtual library UI inside KOReader's file browser

The plugin is modeled after the reference implementation at `REFERENCE/kobo.koplugin/`, which does the same thing for Kobo devices.

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│  KOReader (Lua)                                      │
│  main.lua ← entry point                              │
│  lua/*.lua ← plugin modules                          │
│    helper_client.lua → shells out to Python binary    │
│    cache_manager.lua → EPUB cache lifecycle           │
│    virtual_library.lua → browse/search UI             │
│    library_index.lua → book scanning & indexing       │
│    document_ext.lua → DocumentRegistry patches        │
│    filechooser_ext.lua → file browser integration     │
│    docsettings_ext.lua → sidecar settings support     │
└──────────────┬──────────────────────────────────────┘
               │ JSON stdin/stdout (via io.popen)
┌──────────────▼──────────────────────────────────────┐
│  kindle-helper (Python — Nuitka standalone for ARM)  │
│  python/kindle_helper.py ← CLI entry point           │
│  Subcommands: scan, convert, cover, decrypt,         │
│               drm-init, position                     │
│                                                      │
│  python/kfxlib/  → KFX decode + YJ→EPUB conversion   │
│                    (from Calibre KFX Input plugin)    │
│  python/dedrm/   → DRMION decryption (DeDRM ion.py)  │
│                    + drm-init key extraction          │
└──────────────┬──────────────────────────────────────┘
               │ (for drm-init only) shells out to device cvm
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
  → Python binary detects DRMION wrapper
  → if DRM: decrypts using cached page keys from drm_keys.json
  → kfxlib converts KFX → EPUB (Calibre's conversion pipeline)
  → Lua feeds EPUB path to KOReader's crengine
  → Book renders
```

---

## Tech Stack

| Component | Language | Notes |
|-----------|----------|-------|
| KOReader plugin frontend | Lua | Runs inside KOReader's LuaJIT environment |
| KFX→EPUB conversion | Python | kfxlib from Calibre KFX Input plugin, Nuitka-compiled for ARM |
| DRMION decryption | Python | DeDRM ion.py + pycryptodome |
| DRM key extraction orchestration | Python | Shells out to device JVM with LD_PRELOAD hook |
| DRM voucher extraction | Java (tiny) | ~30 lines, runs on device's `cvm` JVM |
| AES key interception | C (tiny) | ~60 lines, LD_PRELOAD hook, pre-compiled as static asset |
| KOReader integration hooks | Lua | Monkey-patches on KOReader's DocumentRegistry, FileChooser, etc. |

---

## Directory Layout

```
/
├── AGENTS.md                  ← YOU ARE HERE
├── README.md
├── _meta.lua                  ← KOReader plugin metadata
├── main.lua                   ← Plugin entry point (loaded by KOReader)
├── python_build.sh            ← Docker ARM build + package script
│
├── python/
│   ├── kindle_helper.py       ← CLI entry point (scan, convert, cover, decrypt, drm-init, position)
│   ├── kfxlib/                ← KFX→EPUB conversion engine (from Calibre KFX Input plugin)
│   │   ├── calibre-plugin-modules/  ← pypdf, typing_extensions (pure Python)
│   │   └── *.py               ← 34 modules: ION parsing, YJ decode, EPUB generation, etc.
│   └── dedrm/
│       ├── ion.py             ← DeDRM ION parser + DRMION decryption
│       ├── kfxtables.py       ← ION symbol tables
│       └── drm_init.py        ← drm-init: voucher scanning, key extraction, page key derivation
│
├── lib/                       ← Pre-compiled DRM helper assets
│   ├── crypto_hook.so         ← LD_PRELOAD AES key interception hook
│   ├── crypto_hook.c          ← Source for the hook
│   ├── KFXVoucherExtractor.jar ← Java voucher extraction class
│   └── KFXVoucherExtractor.java ← Source for the jar
│
├── lua/                       ← Lua plugin modules
│   ├── helper_client.lua      ← Python binary client (scan, convert, drm-init)
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
├── spec/                      ← Busted test suite (126 specs)
├── patches/                   ← KOReader startup patches
│   └── 2-kindle-virtual-library-startup.lua ← ffi/util.realpath virtual path support
│
├── scripts/                   ← Dev/CI scripts
│   ├── test                   ← Busted runner under luajit
│   ├── convert_kfx_python.py  ← Local KFX→EPUB conversion using kfxlib
│   └── kfx_reference_snapshot.py ← Reference EPUB comparison tool
│
├── .github/
│   └── Dockerfile.arm         ← Nuitka ARM build pipeline
│
└── REFERENCE/                 ← NOT tracked in git — local reference only
    ├── kobo.koplugin/         ← Sister plugin (Kobo) — architectural reference
    ├── koreader/              ← KOReader source — for understanding KOReader APIs
    ├── Calibre_KFX_Input/     ← Calibre KFX Input plugin source (kfxlib origin)
    ├── DeDRM_tools/           ← DeDRM plugin source (ion.py origin)
    ├── KFX_DRM/               ← DRM research: hooks, extractors, scripts, vouchers
    ├── kindle_drm_classes/    ← Decompiled Kindle DRM Java classes
    ├── KFX_DRM_INTEGRATION.md ← Detailed DRM integration plan
    └── KFX_DRM_RESEARCH.md   ← Full DRM research notes (~1200 lines)
```

---

## Hard Rules

### 1. kfxlib is Source of Truth for Conversion

`python/kfxlib/` (from Calibre's KFX Input plugin by John Howell) is the **sole source of truth** for all KFX→EPUB conversion logic. We use it directly — no porting, no reimplementation. This guarantees byte-identical output with Calibre.

**kfxlib code is NEVER modified** except for debug logging.

### 2. Every Change Must Be Tested

- Python: `python3 python/kindle_helper.py convert --input <kfx> --output <epub>`
- Lua: `./scripts/test` (busted under luajit)
- ARM binary: Docker build via `./python_build.sh`
- Some tests require KFX fixture files not in the repo — these auto-skip
- New Lua modules **must** include a corresponding `spec/*_spec.lua`
- Spec structure and mocking patterns follow `REFERENCE/kobo.koplugin/spec/`

### 3. Commits Should Be Atomic

Each logical step gets its own commit. If something breaks, revert and fix before moving on.

---

## DRM Integration

The DRM approach uses **on-device key extraction via LD_PRELOAD** with **just-in-time key refresh**.

### How It Works

1. Device stores DRM vouchers in `*.sdr/assets/voucher` alongside each `.kfx` file
2. Device serial available at `/proc/usid`
3. Account secret (ACSR) stored at `/var/local/java/prefs/acsr`
4. The `drm-init` command runs the device's `cvm` JVM with an LD_PRELOAD hook that intercepts AES key usage
5. The hook logs keys to `/mnt/us/crypto_keys.log`
6. A tiny Java class (`KFXVoucherExtractor.jar`) exercises the DRM SDK, triggering key usage
7. Python code (`dedrm/drm_init.py`) parses the log, matches keys to vouchers, extracts 16-byte page keys
8. Page keys are cached in `drm_keys.json`
9. **JIT retry loop**: when conversion fails due to stale keys, the Lua layer auto-triggers
   key extraction for that specific book and retries — transparent to the user

### DRM File Signatures

| Format | Magic Bytes | Meaning |
|--------|------------|---------|
| DRMION | `\xeaDRMION\xee` | DRM-encrypted KFX |
| CONT | `\xeaCONT\xee` | Container KFX (unencrypted) |
| Voucher | `\xe0\x01\x00\xea` + contains `ProtectedData` | DRM voucher |

### Key Stability

**Keys are NOT deterministic across re-downloads.** Amazon's delivery service generates a
fresh voucher with new ciphertext on every delivery, even for the same content version.
A cached page key becomes invalid whenever the device re-downloads a book's assets.

The JIT approach handles this transparently — stale keys are detected and refreshed
automatically when the user opens a book.

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

---

## Testing

### Running Tests

```sh
# Lua tests (busted under luajit — matches KOReader's runtime)
./scripts/test

# Python local test
python3 python/kindle_helper.py convert --input <kfx> --output <epub>

# Run a single spec file
./scripts/test spec/virtual_library_spec.lua
```

**Always use `./scripts/test` for CI/validation** — KOReader runs LuaJIT on-device.

### Test Structure

Follows `REFERENCE/kobo.koplugin/spec/` patterns:

```
spec/
├── helper.lua                    # Mock setup (loaded before every spec)
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

---

## Build & Deploy

```sh
# Build ARM binary (Docker + Nuitka)
./python_build.sh

# Run all tests
./scripts/test          # Lua

# Deploy to device
# Copy the zip contents to /mnt/us/koreader/plugins/kindle.koplugin/ on the Kindle
```

The `python_build.sh` script:
1. Docker build using `.github/Dockerfile.arm` (Nuitka standalone for ARMv7)
2. Packages binary + Lua plugin files into a ZIP
3. Produces `build/kindle-koplugin-armv7.zip`

### Binary Structure

The ARM binary is a Nuitka standalone build (~55MB):
- `kindle-helper` — static C wrapper (entry point, 362K)
- `libsyscall_wrapper.so` — syscall compat shim (preadv2/pwritev2)
- `dist/main.bin` — Nuitka-compiled Python binary (28MB)
- `dist/ld-linux-armhf.so.3` — bundled dynamic linker
- `dist/*.so` — shared libs (lxml, Pillow, pycryptodome, etc.)
- `dist/calibre-plugin-modules/` — pypdf, typing_extensions
- `dist/Crypto/` — pycryptodome (AES for DRMION decryption)
- `dist/bs4/` — beautifulsoup4

---

## Key References (Read When Needed)

| Document | When to Read |
|----------|-------------|
| `REFERENCE/KFX_DRM_INTEGRATION.md` | Any DRM-related work |
| `REFERENCE/KFX_DRM_RESEARCH.md` | Deep DRM technical details, ION format, key derivation |
| `REFERENCE/kobo.koplugin/` | When implementing Lua UI, virtual library, or KOReader integration |
| `REFERENCE/kobo.koplugin/main.lua` | Plugin structure and menu registration pattern |
| `REFERENCE/kobo.koplugin/spec/` | Test patterns, mocking approach, spec structure reference |
| `REFERENCE/koreader/` | When you need to understand KOReader internals |
| `REFERENCE/DeDRM_tools/` | Python DRM removal reference (source of our ion.py) |
| `REFERENCE/Calibre_KFX_Input/` | Source of our kfxlib |

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

---

## Test Fixtures & Comparison Books

The project has 6 real books from a Kindle device. All conversions produce output identical to Calibre reference EPUBs (only `dcterms:modified` timestamp differs).

### Book Inventory

| Book | Format | Notes |
|------|--------|-------|
| **Hunger Games Trilogy** | DRMION | Largest, most complex |
| **Throne of Glass** | DRMION | Has JXR images |
| **Elvis and the Underdogs** | DRMION | Many images |
| **The Familiars** | DRMION | Moderate complexity |
| **Three Below (Floors #2)** | DRMION | Smaller book |
| **Martyr** | CONT (unencrypted) | Byte-identical output |

### Fixture Paths

| What | Path |
|------|------|
| Raw KFX (CONT) | `REFERENCE/kfx_examples/Martyr_*.kfx` |
| Raw DRMION files | `REFERENCE/kindle_device/Items01/*.kfx` |
| Voucher files | `REFERENCE/kindle_device/Items01/*.sdr/assets/voucher` |
| Decrypted KFX-zip | `REFERENCE/kfx_new/decrypted/*.kfx-zip` |
| Calibre reference EPUBs | `REFERENCE/kfx_new/calibre_epubs/*.epub` |
| DRM keys cache | `REFERENCE/kindle_device/cache/drm_keys.json` |

---

## Common Gotchas

- **OrbStack + armv7**: Don't run `multiarch/qemu-user-static` — it breaks OrbStack's built-in emulation. If broken, restart OrbStack to clear bad binfmt entries.
- **KOReader's Lua is LuaJIT** — use `util.shell_escape()` for shell commands, not raw string concat
- **JSON communication** — the Python binary writes JSON to stdout; Lua parses it. stderr is for debug logging only
- **KFX fixtures** — some tests need real KFX files not in the repo; they auto-skip if absent
- **DRM books have two files** — the `.kfx` (DRMION content) and `*.sdr/assets/voucher` (decryption voucher)
- **Cache invalidation** — cache is keyed on `source_mtime + source_size + converter_version`. Bumping `CONVERTER_VERSION` in `cache_manager.lua` forces re-conversion of all books
- **Lua module paths** — KOReader adds the plugin directory to `package.path`, so `require("lua/cache_manager")` resolves to `plugins/kindle.koplugin/lua/cache_manager.lua`
- **Shared modules from kobo.koplugin** — `lua/lib/pattern_utils.lua`, `lua/lib/session_flags.lua`, `lua/filesystem_ext.lua`, `lua/readerui_ext.lua`, `lua/pathchooser_ext.lua` are adapted from kobo.
- **Nuitka `--include-data-dir`** — doesn't reliably place files on disk. Use explicit `cp` in Dockerfile output stage instead.
- **pycryptodome Crypto module** — must not be over-stripped. pypdf imports ARC4 at module level, so all cipher .so files must be kept.
