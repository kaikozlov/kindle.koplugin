# D2: CBZ/PDF Image Book

## Implementation Notes

- **Custom PDF writer**: The implementation uses a self-contained minimal PDF generator (~350 lines) in `yj_to_image_book.go` instead of an external PDF library. This creates valid PDF-1.4 with JPEG image embedding, outline/bookmarks, metadata, and RTL support. Future PDF-related work should extend this writer rather than importing a competing library.
- **Unported resources.py functions**: The Python `yj_to_image_book.py` imports from `resources.py`: `crop_image`, `convert_image_to_pdf`, `convert_jxr_to_jpeg_or_png`, `convert_pdf_page_to_image`, `combine_image_tiles`. These are NOT yet ported. The Go implementation inlines a simplified `cropImage` and skips PDF page extraction, JXR conversion, and tile combining.
- **Simplified areas acknowledged in handoff**:
  - PDF $565 format resource embedding uses a simplified skip approach
  - JXR-to-JPEG conversion in CBZ output is logged as a warning
  - Tile combining for $636 resources uses simplified first-tile extraction

## Known Limitations

- `getOrderedImages` returns only `[]ImageResource`, dropping `ordered_image_pids` and `content_pos_info` that Python returns. This means `convert_book_to_pdf` cannot fully implement TOC page number mapping without changing the return signature.
- CBZ creation silently skips unexpected image formats (Python raises an exception and aborts).
- Progress callback parameter is not implemented.
