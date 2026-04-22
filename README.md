## Kindle Virtual Library for KOReader

A KOReader plugin that lets you browse and read your Kindle book library directly in KOReader. Your books appear in a **Kindle Library** folder in the file browser — just tap to read.

**[Download latest release](https://github.com/kaikozlov/kindle.koplugin/releases/latest)**

### Features

- **Virtual Library** — Browse your Kindle books from a dedicated folder in KOReader's file browser
- **Reading State Sync** — Sync reading progress between KOReader and Kindle, so you can switch between them and pick up where you left off
- **Cached for Speed** — Books are prepared on first open and cached, so re-opening is instant

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

**Opening a book:**
1. Open the file browser and tap **Kindle Library**
2. Browse your books and tap to read
3. The first open takes a moment while the book is prepared — after that, it opens instantly

**Syncing reading progress:**
1. Go to **Menu → Kindle Library → Sync reading state with Kindle** to enable
2. Reading progress syncs automatically between KOReader and Kindle

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
