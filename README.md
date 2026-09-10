## Kindle Library for KOReader

A KOReader plugin that lets you read the books in your Kindle's native library directly in KOReader. Your books appear in a **Kindle Library** folder in the file browser — tap to read. Everything happens on-device.

**[Download latest release](https://github.com/kaikozlov/kindle.koplugin/releases/latest)**

### Features

- **Native Kindle Library** — browse your Kindle books in KOReader, with covers and metadata
- **Entirely on-device** — your library is prepared and cached locally; no computer or external services needed
- **Exact reading state sync** — switch between KOReader and the stock Kindle reader at the same position, in either direction
- **Cached for speed** — books are prepared on first open and cached, so re-opening is instant

### Installation

1. Download `kindle-koplugin-armv7.zip` from the [latest release](https://github.com/kaikozlov/kindle.koplugin/releases/latest)
2. Extract `kindle.koplugin/` to your KOReader plugins directory:
   - Kindle: `/mnt/us/koreader/plugins/`
3. Restart KOReader

### Usage

**Opening a book:**
1. Open the file browser and tap **Kindle Library**
2. Browse your books and tap to read
3. The first open takes a while — the book is prepared on-device. After that it opens instantly. A "Preparing…" message shows while this happens; large books can take several minutes on older devices.

The library can also be opened via **Menu → Tools → More tools → Kindle Library → Browse Kindle Library**, or bound to a gesture or profile through KOReader's Dispatcher ("Kindle Library" action).

**Syncing reading progress:**
1. Go to **Menu → Tools → More tools → Kindle Library → Sync reading state with Kindle** to enable sync (works in the file browser and inside a book)
2. Open **Sync behavior** and enable **Enable sync FROM Kindle TO KOReader** — this is off by default, and without it no sync from Kindle ever happens. Sync TO Kindle is on by default and applies silently.
3. Choose what to do when one side has newer or older progress under the **FROM/TO Kindle sync settings** entries. Each case can be set to **Always sync**, **Ask me**, or **Never**.

After that, positions sync automatically when a book opens or closes. Exact position sync works whether the book is opened from Kindle Library, KOReader Bookshelf, Collections, or History.

If both KOReader and Kindle moved independently since the last sync, you'll get a conflict prompt showing both percentages — pick **Use Kindle**, **Use KOReader**, or **Cancel** (cancel keeps both sides unchanged and asks again next time). One-sided changes follow your FROM/TO rules instead: **Always sync** applies silently, **Ask me** prompts, **Never** skips.

> **Note:** if you enable sync while a Kindle book is already open, that reading session won't write back on close. Reopen the book once so its opening state can be reconciled first.

### Settings

Menu locations: **Menu → Tools → More tools → Kindle Library**

| Setting | Description |
| ------- | ----------- |
| Sync reading state with Kindle | Master switch for reading-position sync (off by default) |
| Sync all books now | Bulk-sync all eligible prepared books; disabled while a book is open |
| Enable sync FROM Kindle TO KOReader | Pull Kindle's position into KOReader (off by default) |
| Enable sync FROM KOReader TO Kindle | Push KOReader's position to Kindle (on by default) |
| Automatic sync on book open and close | Run sync automatically instead of only manually (on by default) |
| FROM / TO Kindle sync settings | Per-direction rules for newer/older progress: Always sync, Ask me, or Never |
| Virtual Library Enabled | Show the Kindle Library folder in the file browser |
| Virtual Library Folder Cover | Set a custom cover image for the folder entry |
| Clear Book Keys | Clear cached per-book keys; re-extracted when needed |
| Clear Kindle Cache | Delete all prepared books; disabled while a book is open |
| Refresh Kindle Index | Rescan the on-device library for new/removed books |

### Troubleshooting

**"This Kindle firmware cannot extract this book's access key by itself"** means the plugin could not obtain the key needed to open that DRM-protected book.

v0.0.6 supports both the legacy Kindle Java runtime (`/usr/java/bin/cvm`) and the newer Java runtime (`/usr/java/bin/java`, including Java 21 firmware) directly. If the built-in Java extraction path is unavailable or fails, the plugin can automatically fall back to a compatible native extractor from Satsuoni/Jadehawk's `kfxdedrm` tooling when installed at either of these locations:

- `/mnt/us/extensions/kfxdedrm/bin/`
- `/mnt/us/extensions/kfxdedrm-scriptlet/bin/`

The fallback is optional and is not bundled with this plugin. If a book still fails to open, include KOReader's `crash.log` when reporting the issue.

### Compatibility

> Designed for Kindle devices running KOReader alongside stock firmware. Requires a jailbroken Kindle with KOReader installed.

- Firmware 5.16.2.x and earlier (softfp) and 5.16.3+ (hardfp) are both covered by the single armv7 package
- Book access uses the Kindle's own stored credentials and normally requires no external tools
- Java 21 firmware on newer Kindle devices is supported as of v0.0.6
- A compatible `kfxdedrm` native extractor is detected automatically as a fallback when the built-in Java extraction path is unavailable or fails

### Acknowledgments

- [KFX Input and kfxlib](https://www.mobileread.com/forums/showthread.php?t=291290) by John Howell
- [Satsuoni's DeDRM fork](https://github.com/Satsuoni/DeDRM_tools)

### License

MIT License

---

## How it works

KOReader only ever sees real file paths. Browsing the Kindle Library reads the device's `cc.db` metadata with no side effects; opening a book prepares a cached, KOReader-friendly copy of that book on-device and hands the real path to KOReader's normal open flow. History, Collections, sidecars, and provider selection all work natively.

Reading-state sync is exact, not percentage-based: when a book is prepared, the helper emits a text-free position map alongside it, and the plugin translates between KOReader XPointers and native Kindle coordinates in-process, reading and writing the Kindle reader's own state store. See [AGENTS.md](AGENTS.md) for the full architecture, data flow, and design constraints.

## Building from source

No local toolchain is required — just Docker and [just](https://github.com/casey/just).
All tests run against the real KOReader runtime from the pinned [koplugin-dev](https://github.com/kaikozlov/koplugin-dev) image.

```sh
just setup     # one-time: install git hooks and pull the development image

just verify    # canonical: formatting, lint, Lua specs on real KOReader,
               # Python/Java suite, and the ARMv7 DRM hook matrix

# Focused commands
just test                               # all non-e2e Lua tests
just test-file spec/virtual_library_spec.lua   # one exact spec file
just test-python                        # Python/Java tests (also runs Java voucher contract tests)
just test-drm-hook                      # shipped crypto_hook.so under ARMv7 OpenSSL 1.1 + 3

# Product builds
just build-voucher   # rebuild the DRM voucher extractor JAR (JDK 8+)
just build           # build the self-contained ARMv7 release package
```

`just --list` shows every recipe, including `shell` (drop into the container)
and `lua` (KOReader's LuaJIT REPL).
