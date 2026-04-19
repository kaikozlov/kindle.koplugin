# D3: Float Precision / CSS Value Formatting

## Implementation Notes

- Ports Python's `value_str` (epub_output.py:1373-1393), `color_str` (yj_to_epub_properties.py:2121-2175), `int_to_alpha`, `alpha_to_int`, and `numstr` (yj_structure.py:1312-1313).
- Near-zero threshold is `< 1e-6` (matching Python), NOT `< 1e-10` as stated in the feature description.
- Format pipeline: near-zero check → %g → scientific notation fallback to %.4f → trailing zero stripping.

## Known Parity Deviations

- **'-0' normalization**: Go's `formatCSSQuantity` normalizes `-0` to `0`, while Python preserves `-0` for negative values in range [1e-6, ~1e-5). This is cosmetic — `-0` is not meaningful in CSS.
- **fixColorValue string guard**: Go's `fixColorValue` does not handle string-typed numeric inputs. This is unreachable in the current pipeline since it's only called with numeric types.
