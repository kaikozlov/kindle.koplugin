# CSS Diff Audit Report

**Generated:** 2026-04-11
**Baseline commit:** 147c47a (cleanup: remove dead *StyleDeclarations and filterDefaultParagraphMargins)
**Diff counts:** Martyr=273, Elvis=165, HungerGames=127, ThreeBelow=105

---

## Executive Summary

All 670 remaining diffs fall into exactly **3 categories**:

| Category | Description | Martyr | Elvis | HG | 3B | Total |
|----------|-------------|--------|-------|----|----|-------|
| **A. Missing feature: lh→em unit conversion** | Go outputs `lh`/`rem` units instead of converting to `em` | ~56 | ~56 | ~14 | ~48 | ~174 |
| **B. Missing feature: `margin-*: 0` stripping** | Python strips explicit `margin-top: 0` / `margin-bottom: 0` defaults | ~114 | ~37 | ~28 | ~25 | ~204 |
| **C. Cosmetic: floating-point precision** | `1.380192em` vs `1.38019em`, `0.999999em` vs `1em`, etc. | ~131 | ~56 | ~44 | ~21 | ~252 |
| **D. Bug: raw map[] in output** | `map[$306:$310 $307:1.]` instead of `1em` (incomplete conversion) | 0 | ~77 | 0 | ~53 | ~130 |
| **E. Cosmetic: gray vs #808080** | Color synonym naming | ~4 | 0 | ~2 | 0 | ~6 |
| **F. Missing feature: -webkit-hyphens** | Python adds `-webkit-hyphens: auto` alongside `hyphens: auto`; Go omits `-webkit-` prefix | ~34 | 0 | 0 | 0 | ~34 |

**No bugs with wrong values were found.** All diffs are either missing features, cosmetic precision artifacts, or one bug category (raw map output in Elvis/3B).

> **Note:** Many lines have overlapping categories (e.g., a line may have both a precision artifact and a missing lh conversion). The per-file counts above reflect the primary category for each diff line.

---

## Category A: Missing Feature — lh/rem→em Unit Conversion

### Description

The `convertStyleUnits` function in `yj_property_info.go` converts `lh` and `rem` units to `em`, but **only inside `simplifyStylesElementFull`**. When elements are rendered through `processContentProperties` but do NOT pass through `simplifyStylesElementFull` (because they aren't part of the inline content flow — e.g., block-level headings, standalone elements), their `lh` and `rem` values remain unconverted in the output.

### Evidence

**Martyr** (7 classes with unconverted lh):
- `.class_s1H-1`, `.class_s1H-2`: `margin-top: 2.66667lh`, `line-height: 1lh`
- `.class_s2H4`, `.class_s2P2`: `margin-top: 2.66667lh`, `line-height: 1lh`
- `.class_s5X`, `.class_sHA`, `.class_sRD`: `margin-top: 2.66667lh`, `margin-bottom: Xlh`, `line-height: 1lh`

**Hunger Games** (3 classes with unconverted lh/rem):
- `.class_s2T`: `margin-top: 5.33333lh` (Python: `margin-top: 6.4em`)
- `.class_s79F`: `font-size: 0.75rem`, `line-height: 0.625lh` (Python: `font-size: 0.75em`, `line-height: 1`)
- `.class_s790`: `line-height: 1lh` (Python: plain `text-indent: 1.65em; width: 100%`)

These elements also gain `font-family: FreeFontSerif,serif` and `line-height: 1lh` properties that Python doesn't emit — suggesting the Go renderer adds inherited properties that Python's simplification pipeline would strip.

### Python Reference

Python's `simplify_styles()` in `yj_to_epub_properties.py` handles unit conversion during the `simplify_styles_element` pass. The Go port (`convertStyleUnits` in `yj_property_info.go`) works correctly for elements processed through `simplifyStylesElementFull`, but is not applied to all code paths.

### Fix Strategy

Ensure `convertStyleUnits` is applied to **all** CSS output paths, not just `simplifyStylesElementFull`. The issue is likely that block-level elements (headings, standalone blocks) are styled through a different code path that bypasses simplification.

---

## Category B: Missing Feature — `margin-top: 0` / `margin-bottom: 0` Stripping

### Description

Python's `simplify_styles()` strips explicit `margin-top: 0` and `margin-bottom: 0` when they match the default value. The Go converter emits these explicit zero margins, while Python omits them.

### Evidence

This is the **largest single source of diffs** across all files:

- **Martyr**: ~114 removed lines are classes where Python has no `margin-bottom: 0` / `margin-top: 0` but Go includes them
- **Elvis**: ~37 removed lines
- **Hunger Games**: ~28 removed lines
- **Three Below**: ~25 removed lines

Example (Martyr):
```
-Python: .class_s6E-0 {margin-bottom: 0; margin-top: 0; text-indent: 0}
+Go:     .class_s6E-0 {text-indent: 0}
```

When Python strips `margin-bottom: 0; margin-top: 0`, the resulting shorter declaration changes the diff. Combined with precision artifacts (e.g., `margin-top: 0.999999em` instead of `margin-top: 1em` which rounds and then gets stripped), this accounts for a significant portion of all diffs.

### Python Reference

Python's `simplify_styles_element()` calls `strip_default_property_values()` which removes properties matching heritable defaults. When `margin-top` and `margin-bottom` are zero (the default), they are stripped.

### Fix Strategy

The `simplifyStylesElementFull` function should strip `margin-top: 0` and `margin-bottom: 0` when they match the inherited default. This is likely already handled in some paths (since some classes don't have the issue) but missing in others.

---

## Category C: Cosmetic — Floating-Point Precision Artifacts

### Description

Go's `float64` arithmetic produces values like `1.380192em`, `0.999999em`, `3.200004em` where Python's `Decimal` produces `1.38019em`, `1em`, `3.2em`.

### Evidence

Across all files, precision artifacts account for:
- **Martyr**: ~131 diff lines (48% of total)
- **Elvis**: ~56 diff lines
- **Hunger Games**: ~44 diff lines
- **Three Below**: ~21 diff lines

Common patterns:
- `0.999999em` vs `1em` (should be `1em`)
- `1.380192em` vs `1.38019em` (extra trailing digit)
- `3.200004em` vs `3.2em` (should be `3.2em`)
- `0.249999em` vs `0.25em` (should be `0.25em`)
- `1.310004em` vs `1.31em` (should be `1.31em`)
- `2.909088em` vs `2.90909em` (trailing digit differs)
- `1.179999em` vs `1.18em` (should be `1.18em`)

### Python Reference

Python uses `Decimal` arithmetic throughout `property_value()` which avoids floating-point accumulation errors.

### Fix Strategy

Add rounding in `formatCSSQuantity()` (in `yj_property_info.go`) to match Python's 6-significant-digit Decimal output. For example:
```go
func formatCSSQuantity(q float64) string {
    // Round to 6 significant digits to match Python Decimal
    if q == 0 {
        return "0"
    }
    magnitude := math.Floor(math.Log10(math.Abs(q)))
    roundFactor := math.Pow(10, magnitude - 5)
    q = math.Round(q / roundFactor) * roundFactor
    // ... format with appropriate precision
}
```

---

## Category D: Bug — Raw `map[]` Values in CSS Output

### Description

The Go converter outputs raw Go map string representations like `map[$306:$310 $307:1.]` instead of resolved CSS values like `1em`. This only affects Elvis and Three Below.

### Evidence

- **Elvis**: 77 instances of `map[$306:...]` in the Go CSS output
- **Three Below**: 53 instances

Common patterns:
- `font-size: map[$306:$505 $307:1.]` → should be `font-size: 1em`
- `line-height: map[$306:$310 $307:1.]` → should be `line-height: 1`
- `text-indent: map[$306:$308 $307:0.]` → should be `text-indent: 0`
- `margin-top: map[$306:$310 $307:8d-1]` → should be a numeric value
- `width: map[$306:$314 $307:40.195]` → should be `width: 40.195%`

These affect properties like `font-size`, `line-height`, `text-indent`, `margin-*`, `width`, `padding-*`.

### Root Cause

The `propertyValue()` or `formatCSSQuantity()` function is receiving a Go `map[string]interface{}` value where it expects a numeric or string value, and `fmt.Sprintf` is formatting it as a raw map instead of extracting and converting the value.

### Python Reference

Python's `property_value()` handles structured YJ values by extracting the numeric component from the value structure before formatting.

### Fix Strategy

Investigate why structured YJ values (maps with `$306`, `$307`, etc. keys) are being passed to the CSS output without being resolved. This likely happens in `propertyValue()` or `formatCSSQuantity()` where the code doesn't handle the structured value format that certain KFX files use.

---

## Category E: Cosmetic — `gray` vs `#808080`

### Description

Python outputs `border-color: gray` while Go outputs `border-color: #808080`. These are equivalent CSS color values.

### Evidence

- **Martyr**: 2 classes (`.class_s21A`, `.class_sAW`)
- **Hunger Games**: 1 class (`.class_s78C`)
- **Elvis**: 0
- **Three Below**: 0

### Fix Strategy

Optional. Convert `#808080` to `gray` in the output to match Python exactly. This is purely cosmetic.

---

## Category F: Missing Feature — `-webkit-hyphens` Prefix

### Description

Python adds `-webkit-hyphens: auto` alongside `hyphens: auto` for browser compatibility. Go only outputs `hyphens: auto` without the `-webkit-` prefix.

### Evidence

- **Martyr only**: All 17 heading classes have `-webkit-hyphens: auto` in Python but not in Go
- This accounts for 34 diff lines (one `-webkit-hyphens: auto; hyphens: auto` → `hyphens: auto` per heading class, plus the `margin-bottom: 0` stripping)

### Python Reference

Python's `add_composite_and_equivalent_styles()` adds `-webkit-` prefixed equivalents for certain properties.

### Fix Strategy

Port the `-webkit-` prefix equivalent generation from Python's `add_composite_and_equivalent_styles()` to Go's `simplifyStylesFull()`.

---

## Additional Observations

### 1. `vertical-align` Stripping (Martyr: 4, HG: 10, Elvis: 3)

Python strips `vertical-align` from non-table-cell elements during simplification. Go retains it. Example:
```
-Python: .class_s206 {text-align: left; vertical-align: top}
+Go:     .class_s206 {text-align: left}
```
This is part of the ineffective property stripping in `simplify_styles_element()`.

### 2. `max-width` + `width` Simplification (HG: 3 classes)

Python collapses `max-width: 100%; width: 100%` to just `width: 100%` when they're equivalent. Go retains both:
```
-Python: .class_s76Z {max-width: 100%; width: 100%}
+Go:     .class_s76Z {width: 100%}
```

### 3. Class Renumbering

Many diffs are caused by class ordering differences. This is cosmetic and expected — the class numbering depends on processing order.

### 4. `font-family: FreeFontSerif,serif` Addition (Martyr: 7, HG: 8)

Go adds `font-family: FreeFontSerif,serif` to some elements that Python doesn't. This appears related to the lh conversion issue — elements that should have their properties simplified are retaining inherited font-family values.

### 5. `box-sizing: content-box` Addition (HG: 1)

Go adds `box-sizing: content-box` to `.class_s790` where Python doesn't. This is likely an explicit default being emitted unnecessarily.

### 6. `font-size` Value Discrepancy (HG: `.class_s7E6`)

```
-Python: font-size: 0.75em
+Go:     font-size: 0.5625em
```
This is `0.75 * 0.75 = 0.5625` — suggests a double font-size scaling in the Go path. **This is a potential bug** but may be caused by the lh/rem conversion issue (the element may have a `rem` font-size that gets an extra em multiplication).

---

## Recommended Fix Priority

1. **Fix D (map[] bug)** — Most impactful for Elvis/3B. Raw map values in CSS output is a real rendering bug.
2. **Fix C (precision)** — Largest cosmetic improvement. Rounding in `formatCSSQuantity()` would reduce diffs by ~40%.
3. **Fix B (margin stripping)** — Second largest improvement. Strip `margin-top/bottom: 0` defaults.
4. **Fix A (lh/rem conversion)** — Ensure all code paths apply `convertStyleUnits`.
5. **Fix F (-webkit-hyphens)** — Small, targeted fix. Only affects Martyr headings.
6. **Fix E (gray→#808080)** — Purely cosmetic. Lowest priority.

---

## Per-File Summary

### Martyr (273 diffs)
- **Primary cause**: `margin-*: 0` stripping (114) + precision artifacts (131)
- **Secondary**: lh unit conversion (7 classes, ~56 diff lines), `-webkit-hyphens` (34)
- **Minor**: `gray` vs `#808080` (2), `vertical-align` stripping (4), `font-family` addition (7)
- **No map[] bugs**

### Elvis (165 diffs)
- **Primary cause**: Raw `map[]` values (77) + `margin-*: 0` stripping (37) + precision (56)
- **Secondary**: `vertical-align` stripping (3), `-webkit-border-spacing` (1)
- **No lh issues, no gray issues, no -webkit-hyphens issues**

### Hunger Games (127 diffs)
- **Primary cause**: `margin-*: 0` stripping (28) + precision (44) + lh/rem conversion (3 classes, ~14 lines)
- **Secondary**: `vertical-align` stripping (10), `max-width` simplification (3), `gray` vs `#808080` (1), `font-family` addition (8), `box-sizing` (1)
- **Potential bug**: `font-size: 0.5625em` vs `0.75em` in `.class_s7E6` (double scaling?)
- **No map[] bugs**

### Three Below (105 diffs)
- **Primary cause**: Raw `map[]` values (53) + `margin-*: 0` stripping (25) + precision (21)
- **No lh issues, no gray issues, no -webkit-hyphens issues, no vertical-align issues**

---

## Diff Verification Commands

Regenerate this report:
```bash
cd ~/gitrepos/kobo.koplugin/kindle.koplugin
for pair in "../REFERENCE/kfx_examples/Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx /tmp/martyr_python_ref.epub" "../REFERENCE/KFX_DRM/decrypted/Elvis and the Underdogs_B009NG3090_decrypted.kfx-zip /tmp/Elvis and the Underdogs_B009NG3090_calibre.epub" "../REFERENCE/KFX_DRM/decrypted/The Hunger Games Trilogy_B004XJRQUQ_decrypted.kfx-zip /tmp/The Hunger Games Trilogy_B004XJRQUQ_calibre.epub" "../REFERENCE/KFX_DRM/decrypted/Three Below (Floors #2)_B008PL1YQ0_decrypted.kfx-zip /tmp/Three Below (Floors #2)_B008PL1YQ0_calibre.epub"; do
  input=$(echo "$pair" | cut -d' ' -f1)
  ref=$(echo "$pair" | cut -d' ' -f2-)
  bn=$(basename "$input" | sed 's/_decrypted.*//' | sed 's/\..*//')
  tmpdir=$(mktemp -d)
  count=$(go run ./cmd/kindle-helper convert --input "$input" --output "$tmpdir/out.epub" 2>/dev/null && diff -u <(unzip -p "$ref" OEBPS/stylesheet.css) <(unzip -p "$tmpdir/out.epub" OEBPS/stylesheet.css) | grep '^[+-]' | grep -v '^---\|^+++' | wc -l)
  echo "$bn: $count diffs"
  rm -rf "$tmpdir"
done
```
