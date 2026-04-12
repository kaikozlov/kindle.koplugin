# Architecture

How the Go KFX converter works — components, relationships, data flows, invariants.

**What belongs here:** System architecture, component relationships, data flows, invariants.
**What does NOT belong here:** Service ports/commands (use `.factory/services.yaml`), env vars (use `environment.md`).

---

## Overview

The Go KFX converter ports the calibre-kfx-input Python plugin to Go for static binary compilation on e-readers. It converts Amazon KFX files to EPUB format.

## Key Components

### 1. KFX Decoding (`kfx.go`)
- `decodedBook` — the core data structure holding decoded KFX content
- `storylineRenderer` — walks the decoded KFX tree and produces HTML with CSS class names
- `*StyleDeclarations` functions — per-element-type CSS property extractors (being replaced by unified `processContentProperties`)
- `*Class` methods (paragraphClass, linkClass, spanClass, etc.) — produce CSS class names with declarations

### 2. YJ Property Conversion (`yj_property_info.go`)
- `yjPropertyNames` — set of all known YJ property names
- `propertyValue(yjPropName, yjValue)` — converts a YJ property value to CSS value
- `convertYJProperties(yjProperties)` — converts all YJ properties to CSS declarations map
- `processContentProperties(content)` — extracts YJ properties from content dict and converts to CSS

### 3. CSS Simplification (`yj_to_epub_properties.go`)
- `simplifyStylesFull` — top-level post-rendering simplification pass
- `simplifyStylesElementFull` — recursive per-element simplification:
  - Builds merged style from inherited + explicit
  - Extracts heritable properties for children
  - Applies reverse inheritance (moves common child properties to parent)
  - Unwraps empty `<span>` elements
  - Converts `<div>` → `<p>`/`<figure>` based on content analysis
  - Strips inherited property values
- `setHTMLDefaults` — sets body-level CSS defaults

## Data Flow

```
KFX binary → decode → decodedBook → storylineRenderer → HTML + CSS classes
                     ↓
              processContentProperties (YJ → CSS map)
                     ↓
              *StyleDeclarations (per-element filtering) ← being replaced
                     ↓
              styleStringFromDeclarations → CSS class in catalog
                     ↓
              simplifyStylesFull (post-render simplification)
                     ↓
              Final EPUB with stylesheet.css
```

## Key Invariants

1. **Unit conversion location**: lh→em, rem→em, pt→px conversions happen in `simplifyStylesElementFull` via `convertStyleUnits()`, NOT in `propertyValue()`
2. **Two-phase styling**: Properties are set at render time, then simplified post-render in `simplifyStylesFull`
3. **All batches swapped**: ALL element types now use `processContentProperties` — the old per-element-type `*StyleDeclarations` functions have been removed (dead code cleanup in commit 147c47a)

## Current State (as of dd94a88)

- **Test failures**: 10 (improved from 12 baseline)
- **Diff counts**: Martyr=1, Elvis=42, HG=38, 3B=10 (total=91)
- **Of 91 diffs, ~84 are cosmetic**: class index renumbering due to different encounter ordering in Go vs Python during `fixupStylesAndClasses`. Both produce the same CSS property values; only the `.class-N` indices differ.
- **Root cause of cosmetic diffs**: Go and Python both sort styles by count descending, using stable sort that preserves encounter order as tiebreaker. Different rendering pipelines produce different encounter orders, causing equal-count styles to get different indices.
- **~7 structural diffs remain**: font-family handling edge cases (Martyr, HG), box-sizing extra property (HG), missing font-style/font-weight (HG), property redistribution on specific classes

## Python Reference Architecture

The Python code at `REFERENCE/Calibre_KFX_Input/kfxlib/` uses a single data-driven pipeline:
- `YJ_PROPERTY_INFO → property_value() → convert_yj_properties() → process_content_properties()`
- Post-processing: `simplify_styles()` → `add_composite_and_equivalent_styles()`
- `simplify_styles` handles: inheritance stripping, reverse inheritance, div→p conversion, unit conversions, position cleanup, negative padding removal, outline cleanup, ordered list optimization, background handling, link color resolution
- `add_composite_and_equivalent_styles` handles: COMPOSITE_SIDE_STYLES collapsing, -webkit- prefix equivalents, ineffective property warning logging
