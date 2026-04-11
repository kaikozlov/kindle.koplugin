# Environment

Environment variables, external dependencies, and setup notes.

**What belongs here:** Required env vars, external dependencies, dependency quirks, platform-specific notes.
**What does NOT belong here:** Service ports/commands (use `.factory/services.yaml`).

---

## Go Version

Go module at `~/gitrepos/kobo.koplugin/kindle.koplugin`. Uses Go modules (go.mod/go.sum).

## Environment Variables

- `KFX_DUMP_STYLES=1` — triggers intermediate style checkpoint dumps for debugging
- `KFX_DEBUG_PARAGRAPH_STYLE` — debug logging for paragraph style generation

## Reference Files

- Python reference: `~/gitrepos/kobo.koplugin/REFERENCE/Calibre_KFX_Input/kfxlib/`
- Reference EPUB: `~/gitrepos/kobo.koplugin/REFERENCE/martyr_calibre.epub`
- KFX sample: `~/gitrepos/kobo.koplugin/REFERENCE/kfx_examples/Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx`

## Dependencies

Go only — no external services, databases, or daemons required.
