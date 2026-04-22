## Kindle Virtual Library for KOReader

A KOReader plugin that lets you browse and read your Kindle book library directly in KOReader. Your purchased books appear alongside your other files — just tap to read.

### Installation

1. Download the release for your device's architecture:

   | Architecture | Devices |
   | ------------ | ------- |
   | **armv7** | Kindle Paperwhite, Kindle Basic, Kindle Oasis, Kindle Scribe |

   > **Not sure?** armv7 covers all modern Kindle models.

2. Extract `kindle.koplugin` to your KOReader plugins directory:
   - Kindle: `/mnt/us/koreader/plugins/`
3. Restart KOReader

### Usage

Once installed, your Kindle books appear in KOReader's file browser automatically.

**Opening a book:**
1. Browse your library in the KOReader file browser
2. Tap any book to open it
3. The first open takes a moment while the book is prepared — after that, it opens instantly

### Compatibility

> Designed for Kindle devices running KOReader alongside stock firmware.

### License

MIT License

---

## Building from source

```sh
# Build ARM binary (Docker + Nuitka)
./python_build.sh

# Run Lua tests
./scripts/test
```

See `AGENTS.md` for full architecture and development documentation.
