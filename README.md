## Kindle Virtual Library for KOReader

A KOReader plugin that lets you browse and read your Kindle book library directly in KOReader. Your books appear in a **Kindle Library** folder in the file browser — just tap to read.

**[Download latest release](https://github.com/kaikozlov/kindle.koplugin/releases/latest)**

### Features

- **Native Kindle Library** — A Kindle Library entry opens a KOReader `BookList`; documents themselves always use real source/cache paths
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
   KOReader Bookshelf, Collections, or History; persisted cached EPUB paths are
   mapped back to their native source book automatically. If a derived EPUB was
   cleared or became stale, it is rebuilt at that real path before KOReader opens it.

Automatic pull runs in KOReader's `DocSettingsLoad` lifecycle event, after the
reader plugin exists but before KOReader's normal `ReadSettings` pass. That
means ReaderRolling consumes the synchronized XPointer normally, including when
KOReader cold-starts directly into a cached Kindle EPUB from History,
Collections, or `lastfile`. On close, `CloseDocument` captures the mapped
Kindle identity and the actual push runs from the following `SaveSettings`
event, after ReaderRolling has stored the final XPointer and percentage.
**Ask me** is shown asynchronously after the reader is on-screen (for pulls) or
teardown completes (for pushes); **Always sync** applies silently, and **Never**
leaves the destination unchanged. A genuine two-sided exact-position conflict
always prompts regardless of those rules, because neither reader is a safe
automatic winner.

The plugin translates KOReader XPointers to Kindle KFX coordinates, persists
them through Kindle's ReaderSDK, and reverse-translates the native
last-page-read position when returning to KOReader. KOReader and Kindle
calculate percentages against different rendered content lengths, so exact
sync never copies one reader's percentage into the other. On a pull, only the
translated XPointer is staged before `ReadSettings`; after KOReader renders the
destination, its own rendered percentage is read back. On a push, the Kindle
shelf is written with the percentage returned by ReaderSDK.

The plugin stores one text-free reconciliation receipt: the last exact KFX
coordinate known to both readers. Each exact sync compares that receipt with
the current Kindle coordinate and the current KOReader coordinate. If only one
side moved, that side is propagated; if both already agree, the agreement is
confirmed. If both moved independently and disagree, the plugin shows both
renderer-specific percentages and requires an explicit **Use Kindle**, **Use
KOReader**, or **Cancel** choice. Cancel preserves both sides and the conflict is
asked again on the next sync attempt. This also recovers an interrupted close by
retrying the one unfinished KOReader-to-Kindle push. Pull receipts advance only
after KOReader confirms the rendered destination; push receipts advance only
after ReaderSDK confirms the exact coordinate and the Kindle shelf update
succeeds. Shelf-percentage drift is repaired separately and never selects an
exact reading position.

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
# Rebuild the Kindle ReaderSDK progress agent (JDK 11+)
./scripts/build_progress_agent

# Build the self-contained ARMv7 package
./python_build.sh

# Initialize the pinned KOReader reference contract once
# (REFERENCE/koreader must be checked out at KOREADER_TEST_COMMIT)
git -C REFERENCE/koreader submodule update --init base

# Run Lua tests against that KOReader Lua contract
./scripts/test
```
