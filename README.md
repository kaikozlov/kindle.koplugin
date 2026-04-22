# kindle.koplugin — Kindle Virtual Library for KOReader

A [KOReader](https://koreader.rocks/) plugin that browses Kindle-native books from a virtual library and converts KFX files (including DRM-protected) to EPUB format.

## What It Does

- Scans a Kindle device's document library and indexes metadata
- Presents a virtual library UI inside KOReader for browsing Kindle-native books
- Converts Amazon KFX ebook files to standard EPUB format using the Calibre KFX Input plugin (kfxlib)
- Decrypts DRM-protected (DRMION) books using on-device key extraction
- Caches converted files for fast re-opening

## Architecture

```
KOReader Plugin (Lua)  →  Python CLI (kindle-helper)  →  kfxlib + DeDRM ion.py
```

The Lua frontend (`main.lua` + `lua/`) integrates with KOReader's widget system and spawns the Python binary as a helper process. All heavy lifting — KFX parsing, YJ decode, EPUB assembly, DRM decryption — is done by Python libraries compiled with Nuitka for ARM.

## Project Structure

```
python/
  kindle_helper.py     CLI entry point — bridges Lua ↔ Python via JSON stdin/stdout
  kfxlib/              KFX→EPUB conversion engine (from Calibre KFX Input plugin)
  dedrm/               DRMION decryption (DeDRM ion.py) + drm-init key extraction
lua/                   KOReader Lua frontend (widgets, cache, library index)
lib/                   Pre-compiled DRM helpers (Java jar, C hook)
spec/                  Busted test suite
scripts/               Dev/CI scripts
.github/Dockerfile.arm Nuitka ARM build pipeline
python_build.sh        Build script (Docker + package)
_meta.lua              KOReader plugin metadata
main.lua               Plugin entry point
```

## Building

```sh
# Build ARM binary (Docker + Nuitka)
./python_build.sh
```

## Testing

```sh
# Lua tests (busted under luajit)
./scripts/test

# Python local test
python3 python/kindle_helper.py convert --input <kfx> --output <epub>
```

Some tests require KFX fixture files not in the repo — they auto-skip if absent.

## Conversion Pipeline

The plugin uses kfxlib (John Howell's Calibre KFX Input plugin) directly — no porting, no reimplementation. This guarantees byte-identical output with Calibre.

- **KFX parsing** — ION binary/text parsing, symbol table resolution, fragment organization
- **EPUB generation** — Full YJ→EPUB conversion (sections, CSS, navigation, resources)
- **DRM decryption** — DeDRM's DrmIon for proper ION-structured DRMION parsing
- **DRM key extraction** — On-device voucher decryption via LD_PRELOAD hook + device JVM
- **Resource handling** — Image conversion, JXR support, font embedding
