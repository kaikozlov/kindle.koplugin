# Go Parity Audit Plan

## Context

The Python pipeline (`python/kfxlib/` on `master` branch) produces byte-identical EPUBs
to Calibre reference output. The Go port (`internal/kfx/` on `go` branch) has significant
parity gaps. The goal is file-by-file audit to bring Go output to match Python output.

## Python Reference Location

The Python source of truth is in the **master** branch worktree:
`/Users/kai/dev/projects/kindle.koplugin-py/python/kfxlib/`

Go worktree (where you are now):
`/Users/kai/dev/projects/kindle.koplugin/`

## Test Books & Expected Results

| Book | Go Diff | Key Issues |
|------|---------|------------|
| Martyr (CONT) | ✅ Byte-identical | None |
| Three Below (DRMION) | ✅ Near-perfect | Only xmlns:mbp |
| Familiars (DRMION) | ⚠️ 7 files | Heading `<a>` class, stylesheet, content.opf |
| Elvis (DRMION) | ❌ 21 files | Image CSS class naming/dedup, spine ordering |
| Hunger Games (DRMION) | ❌ 87 files | Heading `<a>` class, CSS class naming, spine, stylesheet |
| Throne of Glass (DRMION) | ❌ 65 files | Heading `<a>` class, CSS class naming, JXR images |

Decrypted KFX-zip files: `REFERENCE/kfx_new/decrypted/*.kfx-zip`
Calibre reference EPUBs: `REFERENCE/kfx_new/calibre_epubs/*.epub`

## Methodology: File-by-File Audit

For EACH Go file in `internal/kfx/`:

1. Find the matching Python file in the master branch worktree
2. List every function in the Python file
3. For each function, list every branch (if/elif/for/try/ternary/type dispatch)
4. Map each Python branch to specific Go code (file + line)
5. Flag any Python branch with NO Go counterpart — that's a bug
6. Fix ALL gaps before moving to next file

## Priority Order (by diff impact)

Start with the files that affect the most books:

### Tier 1: Affects all books
1. **yj_to_epub_content.go** ↔ `yj_to_epub_content.py` — heading `<a>` class, section HTML
2. **yj_structure.go** ↔ `yj_structure.py` — section organization, spine ordering
3. **style_events.go** ↔ `yj_to_epub*.py` (style event handling) — CSS class naming/dedup

### Tier 2: Affects many books
4. **yj_to_epub_properties.go** ↔ `yj_to_epub_properties.py` — CSS property handling
5. **yj_to_epub_misc.go** ↔ `yj_to_epub_misc.py` — shared helpers
6. **yj_to_epub_metadata.go** ↔ `yj_to_epub_metadata.py` — OPF/metadata
7. **yj_to_epub_navigation.go** ↔ `yj_to_epub_navigation.py` — NCX/NAV

### Tier 3: Resource handling
8. **yj_to_epub_resources.go** ↔ `yj_to_epub_resources.py` — images, fonts
9. **yj_to_image_book.go** ↔ `yj_to_image_book.py` — image books
10. **svg.go** ↔ relevant Python — SVG generation

### Tier 4: Core/data layer
11. **decode.go** ↔ `ion.py`, `kfx_container.py` — ION parsing
12. **values.go** ↔ `yj_sequential.py` — YJ value handling
13. **symbol_format.go** ↔ `symbol_format.py` — symbol formatting
14. **fragments.go** ↔ `yj_fragment.py` — fragment handling
15. **state.go** ↔ various — pipeline state
16. **yj_versions.go** ↔ `yj_versions.py` — version handling
17. **yj_property_info.go** ↔ `yj_property_info.py` — property definitions
18. **yj_metadata_getters.go** ↔ `yj_metadata_getters.py` — metadata accessors
19. **yj_position_location.go** ↔ `yj_position_location.py` — position mapping
20. **render.go** ↔ render-related Python — rendering logic
21. **html.go** ↔ HTML-related Python — HTML generation
22. **container.go** ↔ `kfx_container.py` — KFX container handling
23. **sidecar.go** ↔ sidecar-related Python — sidecar processing
24. **storyline.go** ↔ storyline Python — storyline tracking
25. **css_values.go** ↔ CSS-related Python — CSS value processing
26. **content_helpers.go** ↔ content helper Python — content utilities
27. **drm.go** ↔ DRM-related Python — DRM handling

### Tier 5: Specialized
28. **yj_to_epub_illustrated_layout.go** ↔ `yj_to_epub_illustrated_layout.py`
29. **yj_to_epub_notebook.go** ↔ `yj_to_epub_notebook.py`
30. **doc.go** — just documentation

## Known Specific Gaps

### 1. Heading `<a>` class attribute
- Python preserves `class` attribute on `<a>` elements inside headings
- Go omits it
- Affects: Familiars, Elvis, Hunger Games, Throne of Glass

### 2. CSS class naming / deduplication
- Python's style catalog assigns different indices than Go
- Image container classes especially affected
- Affects: Elvis, Hunger Games, Throne of Glass

### 3. Spine ordering
- Sections appear in different order in `<spine>` `<itemref>` sequence
- Python has specific ordering logic Go doesn't replicate
- Affects: Elvis, Hunger Games

### 4. JXR images
- Go has JXR decoder (`internal/jxr/`) but not wired into EPUB resource pipeline
- Affects: Throne of Glass

### 5. `xmlns:mbp` namespace
- Was previously fixed for toc.ncx
- May still differ in some files

## Red Flags (from AGENTS.md)

If you catch yourself writing any of these, STOP and re-read the Python:

- `if elem.Tag != X { continue }` — Python almost certainly handles other tags
- `if _, ok := asMap(child); !ok { continue }` — Python handles non-map children
- `if _, ok := asSlice(node["content_list"]); !ok { return nil }` — Python also checks `content` and `story_name`
- `// Skip for now` or `// Not needed` — it IS needed

## Quick Reference: Branch/Worktree Map

| Branch | Worktree | Content |
|--------|----------|---------|
| `master` | `/Users/kai/dev/projects/kindle.koplugin-py` | Python pipeline (source of truth) |
| `go` | `/Users/kai/dev/projects/kindle.koplugin` | Go port (where you fix) |
| `crypto` | `/Users/kai/dev/projects/kindle.koplugin-crypto` | DRM crypto work |
| `kobo-parity` | `/Users/kai/dev/projects/kindle.koplugin-kobo-parity` | Kobo plugin parity |

## How to Compare Output

```bash
# Run Go conversion
cd /Users/kai/dev/projects/kindle.koplugin
go run ./cmd/kindle-helper convert --input <kfx-zip> --output /tmp/go_out.epub

# Run Python conversion (from master worktree)
cd /Users/kai/dev/projects/kindle.koplugin-py
python3 python/kindle_helper.py convert --input <kfx-zip> --output /tmp/py_out.epub

# Diff the EPUBs (they're just ZIPs)
diff <(unzip -l /tmp/go_out.epub) <(unzip -l /tmp/py_out.epub)

# Diff specific files inside
mkdir -p /tmp/go_epub /tmp/py_epub
unzip -o /tmp/go_out.epub -d /tmp/go_epub
unzip -o /tmp/py_out.epub -d /tmp/py_epub
diff -r /tmp/go_epub/ /tmp/py_epub/
```
