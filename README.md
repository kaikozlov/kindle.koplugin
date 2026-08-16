## Kindle Virtual Library for KOReader

A KOReader plugin that lets you browse and read your Kindle book library directly in KOReader. Your books appear in a **Kindle Library** folder in the file browser — just tap to read.

**[Download latest release](https://github.com/soloipd/kindle.koplugin/releases/latest)**

### Features

- **Virtual Library** — Browse your Kindle books from a dedicated folder in KOReader's file browser
- **Exact Reading State Sync** — Switch between KOReader and the native Kindle reader at the same text position, in either direction
- **Cached for Speed** — Books are prepared on first open and cached, so re-opening is instant

### Installation

1. Download the release for your device's architecture:

   | Architecture | Devices |
   | ------------ | ------- |
   | **armv7** | Kindle Paperwhite, Kindle Basic, Kindle Oasis, Kindle Scribe |

   > **Not sure?** armv7 covers supported modern Kindle models. The package is
   > self-contained across both the softfp userspace used through firmware
   > 5.16.2.x and the hardfp userspace introduced in firmware 5.16.3.

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
2. Under **Sync behavior**, enable automatic open/close sync and choose the
   FROM/TO Kindle rules for newer and older progress
3. Exact reading position syncs whether the book is opened from Kindle Library,
   KOReader Bookshelf, or History; cached Kindle EPUBs are mapped back to their
   native source book automatically

Automatic sync runs before opening and after closing a book. **Ask me** waits
for your answer at that lifecycle boundary, **Always sync** applies silently,
and **Never** leaves the destination unchanged. The plugin translates KOReader
XPointers to Kindle KFX coordinates, persists them through Kindle's ReaderSDK,
and reverse-translates the native last-page-read position when returning to
KOReader. If KOReader cold-starts directly into a mapped Bookshelf or History
entry before plugins are loaded, a reader-ready catch-up applies the same exact
position to the live reader; normal Kindle Library opens are not reconciled
twice. The shelf percentage is updated only after the authoritative native save
succeeds. KOReader and Kindle calculate percentages against different rendered
content lengths, so the native shelf is written with Kindle's rendered
percentage rather than copying KOReader's percentage. This keeps the native
shelf and native reader consistent while the exact text position remains the
cross-reader source of truth.

The plugin also stores a text-free reconciliation receipt containing only the
last successfully synchronized KFX position. On the next open it compares the
native reader's exact last-page-read coordinate with that receipt. A changed
coordinate is pulled even if Kindle's catalog timestamp is stale; an unchanged
coordinate cannot overwrite newer KOReader progress. If the coordinate still
matches but a stale native process has overwritten only the shelf percentage,
the plugin repairs that display value from Kindle's verified rendered percent
without moving either reader. A receipt is written only after the exact native
save and shelf update both succeed.

For annotation integrations, the bundled helper also provides bounded batch
translation in both directions. `translate-positions` converts normalized
KOReader XPointer ranges to exact KFX coordinates, while
`translate-native-positions` reverse-translates up to 1,000 native ranges and
verifies every endpoint by round trip. The coordinate map contains no book or
annotation text.

### Compatibility

> Designed for Kindle devices running KOReader alongside stock firmware.

Book-key extraction normally uses the Kindle Java DRM SDK bundled with the
firmware. If that route fails and
[Satsuoni's `kfxdedrm` KUAL extension](https://github.com/Satsuoni/DeDRM_tools/tree/master/Other_Tools/KRFKeyExtractor/kindle_device)
is installed at `/mnt/us/extensions/kfxdedrm/`, the plugin can automatically
use its tested native `libYJSDK` extractor as a fallback. The external binaries
are optional and are not bundled with this plugin.

### License

MIT License

---

## Building from source

```sh
# Build the self-contained ARMv7 package
./python_build.sh

# Run Lua tests
./scripts/test
```
