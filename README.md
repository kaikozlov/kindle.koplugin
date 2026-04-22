<div align="center">

# kindle.koplugin

A KOReader plugin that lets you browse and read your Kindle book library directly in KOReader.

</div>

## Features

- **Virtual Library** — Browse your full Kindle book collection from inside KOReader's file browser
- **Seamless Opening** — Tap any book to read it. Conversions happen automatically in the background
- **Cached for Speed** — Books are cached after the first open, so re-opening is instant

## Installation

1. Download the latest release for your device.
2. Extract `kindle.koplugin` to your KOReader plugins directory:
   - Kindle: `/mnt/us/koreader/plugins/`
3. Restart KOReader

## Usage

Once installed, your Kindle books appear alongside your other files in KOReader's file browser. Just tap to open — the first open of each book takes a moment while it's prepared for reading, and after that it opens instantly.

## Compatibility

Designed for Kindle devices running KOReader alongside stock firmware.

## License

MIT License

---

<details>
<summary>Building from source</summary>

```sh
# Build ARM binary (Docker + Nuitka)
./python_build.sh

# Run Lua tests
./scripts/test
```

See `AGENTS.md` for full architecture and development documentation.

</details>
