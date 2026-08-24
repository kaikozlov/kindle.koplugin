# AGENTS.md — kindle.koplugin Project Instructions

## Project Overview

**kindle.koplugin** is a [KOReader](https://koreader.rocks/) plugin that lets Kindle device owners browse and read their Kindle-native library books inside KOReader. It does this by:

1. **Scanning** the Kindle's on-device document library (KFX files in `/mnt/us/documents/`)
2. **Decrypting** DRM-protected books using on-device key extraction
3. **Converting** KFX → EPUB via a bundled ARM CPython runtime and Python helper
4. **Caching** converted EPUBs for fast re-opening
5. **Presenting** a native `BookList` Kindle Library launched from one synthetic file-browser entry

KOReader itself is the architectural source of truth for UI/lifecycle behavior. `REFERENCE/kobo.koplugin/` may provide ideas, but do not copy its virtualization shims when current KOReader has a native extension point.

---

## Architecture

KOReader only sees **real document paths**. The historical `KINDLE_VIRTUAL://` scheme is gone; the only remnant is a one-line settings repair for a legacy `home_dir` value.

```
┌─────────────────────────────────────────────────────┐
│ KOReader (Lua)                                       │
│ main.lua                                             │
│   FileManager plugin init                            │
│     └─ minimal FileChooser hook → Kindle Library/    │
│                            │                         │
│                            ▼                         │
│                    native BookList                   │
│                            │                         │
│                select book / explicit open           │
│                            ▼                         │
│ filemanagerutil.openFile(real source/cache path)     │
│     └─ narrow resolver refreshes derived EPUB only   │
│        when that known Kindle path is stale/missing  │
│                            │                         │
│                            ▼                         │
│   ├─ KOReader chooses DocumentRegistry provider      │
│   ├─ KOReader owns DocSettings/History/Collections   │
│   └─ Reader lifecycle events                         │
│       ├─ DocSettingsLoad → Kindle → KOReader pull    │
│       └─ CloseDocument captures identity;            │
│          final SaveSettings → KOReader → Kindle push │
└──────────────────────┬──────────────────────────────┘
                       │ JSON CLI
┌──────────────────────▼──────────────────────────────┐
│ kindle-helper (C launcher + bundled ARM CPython)     │
│ python/kindle_helper.py                              │
│ KFX conversion / DRM / exact position translation    │
└──────────────────────┬──────────────────────────────┘
                       │ DRM init / native progress
┌──────────────────────▼──────────────────────────────┐
│ Kindle firmware services                             │
│ KFXVoucherExtractor + crypto hook / ReaderSDK agent  │
└─────────────────────────────────────────────────────┘
```

### Data Flow

```
User opens Kindle Library
  → FileChooser synthetic folder launches KindleLibrary BookList
  → browsing uses cc.db/scan metadata only (NO conversion or DRM side effects)
User selects a book
  → virtual_library model resolves the Kindle entry
  → filemanagerutil.openFile(real source/cache path)
  → open_file_ext refreshes/prepares only a known stale/missing Kindle-derived EPUB
  → KOReader chooses the native provider (MuPDF/CREngine/etc.)
  → KOReader loads native DocSettings
  → onDocSettingsLoad may update position before ReadSettings
  → ReaderRolling/Paging restores position normally
  → CloseDocument captures the mapped Kindle book/path
  → ReaderRolling writes final position during SaveSettings
  → plugin onSaveSettings pushes that final state to Kindle
```

**Do not patch** `lfs.attributes`, `ffiUtil.realpath`, `DocumentRegistry`, `ReaderUI:showReader`, `ReaderUI:onClose`, or `DocSettings` to emulate files. If a feature appears to require that, first re-read current `REFERENCE/koreader/` for a native lifecycle/UI seam.

## Tech Stack

| Component | Language | Notes |
|-----------|----------|-------|
| KOReader plugin frontend | Lua | Runs inside KOReader's LuaJIT environment |
| KFX→EPUB conversion | Python | kfxlib from Calibre KFX Input plugin, run by bundled ARM CPython |
| DRMION decryption | Python | DeDRM ion.py + pycryptodome |
| DRM key extraction orchestration | Python | Shells out to device JVM with LD_PRELOAD hook |
| DRM voucher extraction | Java (tiny) | ~30 lines, runs on device's `cvm` JVM |
| AES key interception | C (tiny) | ~60 lines, LD_PRELOAD hook, pre-compiled as static asset |
| KOReader integration | Lua | Native `BookList` + `DocSettingsLoad` + close-capture/final-`SaveSettings`; narrow reversible FileChooser discovery + real-path open resolver hooks |

---

## Directory Layout

```
/
├── AGENTS.md
├── README.md
├── _meta.lua
├── main.lua                       ← plugin lifecycle + menus + sync events
├── KOREADER_TEST_COMMIT           ← pinned KOReader Lua contract for tests
├── python_build.sh
│
├── lua/
│   ├── kindle_library.lua         ← native BookList UI; opens only real paths
│   ├── filechooser_ext.lua        ← minimal synthetic-folder hook
│   ├── open_file_ext.lua          ← refresh stale/missing known cached EPUBs before native open
│   ├── virtual_library.lua        ← Kindle book model + real-path mappings
│   ├── cache_manager.lua
│   ├── library_index.lua
│   ├── ccdb_scanner.lua
│   ├── helper_client.lua
│   ├── reading_state_sync.lua
│   └── lib/
│       ├── kindle_state_reader.lua
│       ├── kindle_state_writer.lua
│       ├── status_converter.lua
│       └── sync_decision_maker.lua
│
├── python/
│   ├── kindle_helper.py
│   ├── kfx_position_adapter.py    ← plugin-owned position instrumentation
│   ├── epub_position.py
│   ├── annotation_position.py
│   ├── kfxlib/                    ← pristine Calibre KFX Input source
│   └── dedrm/
│
├── agent/                         ← reproducible Kindle ReaderSDK progress agent
├── lib/                           ← DRM/native helper assets
├── scripts/
│   ├── test                       ← current KOReader-contract Lua tests
│   ├── koreader-test-container    ← overlays pinned Lua source on native runtime
│   └── build_progress_agent
├── spec/
├── .github/
└── REFERENCE/                     ← local reference checkouts; not release payload
    ├── koreader/                  ← PRIMARY source for KOReader behavior
    ├── DeDRM_tools/
    ├── KFX_Input/
    ├── sidecar/
    └── ...
```

There is deliberately **no plugin-local `patches/` directory**. KOReader's user-patch loader scans the top-level KOReader patches directory, not nested plugin directories, and this plugin no longer requires startup patches.

## Hard Rules

### 1. kfxlib is Source of Truth for Conversion

`python/kfxlib/` (from Calibre's KFX Input plugin by John Howell) is the **sole source of truth** for all KFX→EPUB conversion logic. We use it directly — no porting, no reimplementation. This guarantees byte-identical output with Calibre.

**kfxlib code is NEVER modified** except for debug logging.

### 2. Every Change Must Be Tested

- Python: `python3 python/kindle_helper.py convert --input <kfx> --output <epub>`
- Lua: `make test` or `./scripts/test` (koplugin-dev Docker image with real headless KOReader)
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

KOReader plugins live in a `<name>.koplugin/` directory with `_meta.lua` and a `WidgetContainer` subclass returned from `main.lua`.

### Native integration rules

- **Real paths are the document identity.** History, Collections, provider selection, sidecars, BookList caches, and ReaderUI must see the actual source file or cached EPUB path.
- **Library UI is a `BookList`, not a fake directory.** The FileManager hook may add one synthetic folder entry, but must never assign a URI to `FileChooser.path`.
- **Pull sync in `onDocSettingsLoad`, acknowledge at live readback.** KOReader emits `DocSettingsLoad` after plugin instantiation and before `ReadSettings`; an exact pull may stage only the translated XPointer there. Do not copy Kindle's percentage into KOReader or advance the reconciliation receipt yet. Confirm the destination from the live renderer at `ReaderReady` (or immediately after an approved live prompt), then persist KOReader's own rendered percentage and advance the receipt.
- **Push sync after the final `SaveSettings`.** In normal ReaderUI teardown, `CloseDocument` occurs before the UIManager-driven final save. Capture Kindle identity in `onCloseDocument`, then push from plugin `onSaveSettings`, which is registered after ReaderRolling and therefore sees its final XPointer/percent. An exact push advances the receipt only after ReaderSDK confirms the requested native coordinate and the Kindle shelf update succeeds. The uncommon ReaderUI branch that saves before `CloseDocument` may push immediately.
- **Exact reconciliation has only two authorities and one receipt.** The Kindle ReaderSDK coordinate and KOReader's translated XPointer are authoritative; shelf percentages are display metadata. Compare both current exact coordinates to the single last-agreed receipt: propagate a one-sided change, recover an interrupted one-sided write, and acknowledge matching coordinates after readback. If both sides moved independently and disagree, always prompt with both renderer-specific percentages and explicit **Use Kindle / Use KOReader / Cancel** choices, regardless of ordinary newer/older sync policy. Cancel preserves both sides so the next sync attempt asks again. Do not introduce event journals, session histories, or display-only sources into exact-position authority without a demonstrated requirement.
- **Renderer percentages are local.** A converted EPUB and the Kindle renderer may report different percentages for the same exact KFX coordinate. Never use one renderer's percentage as the other's exact-position state.
- **DocSettings is KOReader-owned.** Preserve native `doc`/`dir`/`hash` probing and migration. Do not override `getSidecarDir` or `getSidecarFilename`.
- **Provider selection is KOReader-owned.** The open resolver may substitute a refreshed real cache path, then must delegate to `filemanagerutil.openFile()` without forcing CREngine; direct PDFs must naturally resolve to MuPDF.
- **PathChooser is untouched.** The synthetic Kindle Library entry is injected only when `FileChooser.name == "filemanager"`.
- **Browsing must be side-effect free.** Listing a book or rendering metadata must not run KFX conversion, DRM extraction, or key refresh. Preparation begins only on explicit open.
- **Runtime hooks must unwind.** `stopPlugin()` must restore both the FileChooser methods and `filemanagerutil.openFile` so live disable/delete works without a restart.
- **Current `REFERENCE/koreader/` wins over old plugin precedent.** If Kobo code and current KOReader disagree, follow current KOReader unless there is a Kindle-specific necessity with a behavioral test.

Key KOReader APIs:
- `BookList` — Kindle Library UI
- `filemanagerutil.openFile()` — native document-open boundary
- `DocSettingsLoad` / `CloseDocument` / `SaveSettings` — reading-state lifecycle
- `DocSettings` — sidecar storage and migration
- `DocumentRegistry` — provider resolution (do not patch)
- `UIManager` — dialogs/scheduling
- `G_reader_settings` — persistent settings
- `WidgetContainer` — plugin base class


## Testing

### Running Tests

```sh
# Lua tests: pinned current KOReader Lua contract over the koplugin-dev native runtime
make test
./scripts/test

# Python local test
python3 python/kindle_helper.py convert --input <kfx> --output <epub>

# Run a single spec file
./scripts/test spec/virtual_library_spec.lua
```

**Always use `make test` or `./scripts/test` for validation.** `scripts/koreader-test-container` verifies `REFERENCE/koreader` is exactly the commit in `KOREADER_TEST_COMMIT`, overlays the current FileManager/ReaderUI/DocSettings/BookList/CoverBrowser Lua surface onto the known-good koplugin-dev native runtime, and then runs Busted. Initialize `REFERENCE/koreader/base` before testing (`git -C REFERENCE/koreader submodule update --init base`).

### Test Structure

```
spec/
├── 00_koreader_native_lifecycle_spec.lua # Real PluginLoader + FileManager smoke test
├── test_helper.lua                       # Kindle-specific controls layered on commonrequire
├── virtual_library_spec.lua
├── cache_manager_spec.lua
├── library_index_spec.lua
├── helper_client_spec.lua
└── *_spec.lua                            # Module and integration coverage
```

Prefer real KOReader modules from `commonrequire.lua`. Mock only narrow filesystem, process,
or database boundaries that cannot be reproduced safely in the headless container. Keep the
native lifecycle smoke test free of `test_helper.lua` so whole-module stubs cannot mask plugin
construction failures.

---

## Build & Deploy

```sh
# Build the self-contained ARMv7 package
./python_build.sh

# Run all tests
./scripts/test          # Lua

# Deploy to device
# Copy the zip contents to /mnt/us/koreader/plugins/kindle.koplugin/ on the Kindle
```

The `python_build.sh` script:
1. Downloads a pinned ARMv7 CPython runtime plus pinned binary wheels.
2. Builds the tiny static launcher and compatibility shims in Docker.
3. Bundles the hard-float dynamic loader/runtime so the same package works on
   Kindle softfp firmware through 5.16.2.x and hardfp firmware 5.16.3+.
4. Runs the packaged helper in a scratch ARM rootfs to verify it does not rely
   on the firmware's `/lib/ld-linux-armhf.so.3` or native libraries.
5. Packages the runtime + Lua plugin files into `build/kindle-koplugin-armv7.zip`.

### Binary Structure

The ARM package contains:
- `kindle-helper` — static C launcher (entry point)
- `libsyscall_wrapper.so` — syscall compat shim (preadv2/pwritev2)
- `dist/bin/python3` — pinned ARMv7 hard-float CPython interpreter
- `dist/lib/runtime/ld-linux-armhf.so.3` — bundled hard-float dynamic loader
- `dist/lib/runtime/` — matching glibc/GCC runtime needed by the interpreter
- `dist/lib/external/` — native libraries required by Pillow
- `dist/lib/python3.11/site-packages/` — lxml, Pillow, pycryptodome, etc.
- `dist/kfxlib/` — KFX conversion engine
- `dist/dedrm/` — DRM helpers

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
- **pycryptodome Crypto module** — must not be over-stripped. pypdf imports ARC4 at module level, so all cipher .so files must be kept.
