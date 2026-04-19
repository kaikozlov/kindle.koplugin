# Stream C+D Python Reference Details

> Auto-generated from `REFERENCE/Calibre_KFX_Input/kfxlib/` source files.
> File line numbers are 1-based. All paths relative to `kfxlib/`.

---

## C1: yj_versions.py — Constants & Validation Functions

**File**: `yj_versions.py` — 1124 lines total

### 1.1 Sentinel Constants

| Constant | Line | Value |
|----------|------|-------|
| `ANY` | 6 | `True` |
| `TF` | 7 | `{False, True}` |

### 1.2 String Constants (Feature Names)

Lines 9–100. Each is a simple string assignment `NAME = "NAME"`.

| Lines | Count | Category |
|-------|-------|----------|
| 9–14 | 6 | General capability names (ARTICLE_READER_V1, DUAL_READING_ORDER_V1, etc.) |
| 15–18 | 4 | NMDL_NOTE variants (V1–V4) |
| 19–22 | 4 | HD/HDV/Vella support |
| 23 | 1 | `YJ = "YJ"` (generic) |
| 24–31 | 8 | JP Vertical (YJJPV_V1–V8) |
| 32–34 | 3 | Audio (V1–V3) |
| 35–100 | ~65 | Reflowable, fixed layout, tables, video, etc. |

### 1.3 PACKAGE_VERSION_PLACEHOLDERS

**Lines**: 103–107
**Type**: `set` of 3 strings
```python
PACKAGE_VERSION_PLACEHOLDERS = {
    "PackageVersion:YJReaderSDK-1.0.x.x GitSHA:c805492 Month-Day:04-22",
    "PackageVersion:YJReaderSDK-1.0.x.x GitSHA:[33mc805492[m Month-Day:04-22",
    "kfxlib-00000000"
}
```

### 1.4 KNOWN_KFX_GENERATORS

**Lines**: 110–211
**Type**: `set` of tuples `(version_string, package_version_string)`
**Count**: ~49 entries — tuples of `("7.X.Y.Z", "PackageVersion:YJReaderSDK-...")`
- Early entries have both version and package string
- Later entries (from 7.153 onward) have empty string for package version: `("7.153.1.0", "")`
- Includes `("20.12.238.0", "")` as the last entry

### 1.5 GENERIC_CREATOR_VERSIONS

**Lines**: 213–217
**Type**: `set` of 3 tuples
```python
GENERIC_CREATOR_VERSIONS = {
    ("YJConversionTools", "2.15.0"),
    ("KTC", "1.0.11.1"),
    ("", ""),
}
```

### 1.6 KNOWN_FEATURES

**Lines**: 220–466
**Type**: Nested `dict[str, dict[str, dict[version_key, feature_name]]]`

**Structure**: `{category: {feature_key: {version_or_range: FEATURE_CONSTANT}}}`

Top-level categories:
1. `"format_capabilities"` — 4 feature keys (pidMapWithOffset, positionMaps, textBlock, delta_update, schema)
2. `"SDK.Marker"` — 1 key: `"CanonicalFormat"` with versions 1, 2
3. `"com.amazon.kindle.nmdl"` — 1 key: `"note"` with versions 2, 3, 4
4. `"com.amazon.yjconversion"` — ~38 feature keys including:
   - Language-specific reflow (ar, cn, fa, he, indic, jp, jpvertical, tcn)
   - `reflow-style` with versions 1–14 + `(2147483646, 2147483647)` and `(2147483647, 2147483647)` tuples
   - `yj_audio` (V1–V3), `yj_dictionary` (V1, V1_ARABIC)
   - `yj_table` (v1–v11), `yj_table_viewer` (v1, v2)
   - `yj_hdv` (V1, V2), `yj_publisher_panels` (V2, V3)
   - Various fixed layout, video, mathml, etc.

Special key types in version maps:
- Integer keys: `{1: YJ, 2: YJ_V2, ...}`
- `ANY` key: `{ANY: YJ_REFLOWABLE_LARGESECTION}` — matches any version
- Tuple keys: `{(2147483646, 2147483647): YJ}` — range match

### 1.7 KNOWN_SUPPORTED_FEATURES

**Lines**: 469–480
**Type**: `set` of 5 tuples
```python
KNOWN_SUPPORTED_FEATURES = {
    ("$826",), ("$827",), ("$660",), ("$751",),
    ("$664", "crop_bleed", 1),
}
```

### 1.8 KNOWN_METADATA

**Lines**: 483–935
**Type**: Nested `dict[str, dict[str, set_or_ANY]]`

Top-level categories (~7):
1. `"book_navigation"` — `{"pages": ANY}`
2. `"book_requirements"` — `{"min_kindle_version": ANY}`
3. `"kindle_audit_metadata"` — `{"file_creator": {5 names}, "creator_version": {~150+ version strings}}`
4. `"kindle_capability_metadata"` — ~15 keys (continuous_popup_progression, graphical_highlights, etc.)
5. `"kindle_ebook_metadata"` — ~7 keys (book_orientation_lock, intended_audience, etc.)
6. `"kindle_title_metadata"` — ~25 keys (ASIN, author, title, language, etc.) — most use `ANY` or `TF`
7. `"metadata"` — ~20 keys (ASIN, cde_content_type, cover_image, etc.)
8. `"symbols"` — `{"max_id": {~42 integer values}}`

### 1.9 KNOWN_AUXILIARY_METADATA

**Lines**: 938–975
**Type**: `dict[str, set_or_ANY]`
**Count**: ~38 keys including: `ANCHOR_REFERRED_BY_CONTAINERS`, `auxData_resource_list`, `has_large_data_table`, `mime`, `namespace`, `page_rotation`, etc.

### 1.10 KNOWN_KCB_DATA

**Lines**: 978–1043
**Type**: Nested `dict[str, dict[str, list_or_set]]`
3 categories: `"book_state"`, `"content_hash"`, `"metadata"`, `"tool_data"`

### 1.11 UNSUPPORTED

**Line**: 1046
```python
UNSUPPORTED = "Unsupported"
```

### 1.12 KINDLE_VERSION_CAPABILITIES

**Lines**: 1049–1073
**Type**: `dict[version_string, list[FEATURE_CONSTANT]]`
**Count**: ~20 version entries from `"5.6.5"` to `"5.18.5"`
- Note: `"5.14.3.2"` maps to a single value (not a list): `YJ_PDF_BACKED_FIXED_LAYOUT_V1_TEST`

### 1.13 KINDLE_CAPABILITY_VERSIONS

**Lines**: 1076–1079
**Type**: `dict[FEATURE_CONSTANT, version_string]` — computed from `KINDLE_VERSION_CAPABILITIES`
```python
KINDLE_CAPABILITY_VERSIONS = {}
for version, capabilities in KINDLE_VERSION_CAPABILITIES.items():
    for capability in capabilities:
        KINDLE_CAPABILITY_VERSIONS[capability] = version
```

### 1.14 Functions

#### `is_known_generator(kfxgen_application_version, kfxgen_package_version)` — Lines 1082–1093
```python
def is_known_generator(kfxgen_application_version, kfxgen_package_version):
    if (kfxgen_application_version == "" or
            kfxgen_application_version.startswith("kfxlib") or
            kfxgen_application_version.startswith("KC") or
            kfxgen_application_version.startswith("KPR")):
        return True
    if kfxgen_package_version in PACKAGE_VERSION_PLACEHOLDERS:
        kfxgen_package_version = ""
    return (kfxgen_application_version, kfxgen_package_version) in KNOWN_KFX_GENERATORS
```
**Dependencies**: `PACKAGE_VERSION_PLACEHOLDERS`, `KNOWN_KFX_GENERATORS`

#### `is_known_feature(cat, key, val)` — Lines 1095–1098
```python
def is_known_feature(cat, key, val):
    vals = KNOWN_FEATURES.get(cat, {}).get(key, {})
    return val in vals or ANY in vals
```
**Dependencies**: `KNOWN_FEATURES`, `ANY`

#### `kindle_feature_version(cat, key, val)` — Lines 1100–1105
```python
def kindle_feature_version(cat, key, val):
    vals = KNOWN_FEATURES.get(cat, {}).get(key, {})
    feature = vals[val] if val in vals else vals.get(ANY)
    return KINDLE_CAPABILITY_VERSIONS.get(feature, UNSUPPORTED) if feature is not None else UNSUPPORTED
```
**Dependencies**: `KNOWN_FEATURES`, `KINDLE_CAPABILITY_VERSIONS`, `UNSUPPORTED`, `ANY`

#### `is_known_metadata(cat, key, val)` — Lines 1106–1115
```python
def is_known_metadata(cat, key, val):
    if isinstance(val, list):
        for v in val:
            if not is_known_metadata(cat, key, v):
                return False
        return True
    else:
        vals = KNOWN_METADATA.get(cat, {}).get(key, {})
        return vals is ANY or val in vals
```
**Dependencies**: `KNOWN_METADATA`, `ANY` — **recursive** (calls itself for list values)

#### `is_known_aux_metadata(key, val)` — Lines 1117–1120
```python
def is_known_aux_metadata(key, val):
    vals = KNOWN_AUXILIARY_METADATA.get(key, {})
    return vals is ANY or val in vals
```
**Dependencies**: `KNOWN_AUXILIARY_METADATA`, `ANY`

#### `is_known_kcb_data(cat, key, val)` — Lines 1122–1124
```python
def is_known_kcb_data(cat, key, val):
    vals = KNOWN_KCB_DATA.get(cat, {}).get(key, {})
    return vals is ANY or val in vals
```
**Dependencies**: `KNOWN_KCB_DATA`, `ANY`

---

## C3: yj_structure.py — Fragment Validation Functions

**File**: `yj_structure.py` — 1313 lines total

### 3.1 Module-level Constants

| Name | Lines | Type | Description |
|------|-------|------|-------------|
| `REPORT_KNOWN_PROBLEMS` | 18 | `None` | Controls error vs warning for known issues |
| `REPORT_NON_JPEG_JFIF_COVER` | 19 | `False` | JPEG cover format check |
| `REPORT_JPEG_VARIANTS` | 20 | `False` | JPEG variant reporting |
| `DEBUG_PDF_PAGE_SIZE` | 21 | `False` | PDF page size debug |
| `MAX_CONTENT_FRAGMENT_SIZE` | 23 | `8192` | Max bytes for content fragments |
| `APPROXIMATE_PAGE_LIST` | 25 | `"APPROXIMATE_PAGE_LIST"` | Page list constant |
| `KFX_COVER_RESOURCE` | 26 | `"kfx_cover_image"` | Cover resource prefix |
| `DICTIONARY_RULES_SYMBOL` | 28 | `"dictionary_rules"` | Dictionary rules symbol |
| `METADATA_SYMBOLS` | 31–43 | `dict[str, str]` | 14 metadata name→symbol mappings |
| `METADATA_NAMES` | 46–47 | `dict[str, str]` | Reverse of METADATA_SYMBOLS |
| `CHECKED_IMAGE_FMTS` | 50 | `set` of 11 | Image formats that PIL checks |
| `UNCHECKED_IMAGE_FMTS` | 51 | `{"jxr", "kvg"}` | Unchecked image formats |
| `ALL_IMAGE_FMTS` | 52 | `CHECKED \| UNCHECKED` | All image formats |
| `FIXED_LAYOUT_IMAGE_FORMATS` | 54 | `set` of 5 symbols | Fixed layout image format symbols |
| `FRAGMENT_ID_KEYS` | 57–72 | `dict[str, list[str]]` | 13 fragment type→id key mappings |
| `COMMON_FRAGMENT_REFERENCES` | 75–99 | `dict[str, str]` | 20 container→fragment_type ref mappings |
| `NESTED_FRAGMENT_REFERENCES` | 102–107 | `dict[tuple, str]` | 4 (parent, container)→frag_ref mappings |
| `SPECIAL_FRAGMENT_REFERENCES` | 110–118 | `dict[str, dict[str, str]]` | 2 ftype→{container→frag_ref} |
| `SPECIAL_PARENT_FRAGMENT_REFERENCES` | 121–124 | `dict[str, dict[str, bool]]` | 1 ftype→{container_parent→False} |
| `SECTION_DATA_TYPES` | 127–132 | `set` of 4 | Section data fragment types |
| `EXPECTED_ANNOTATIONS` | 135–142 | `set` of 5 tuples | Expected IonAnnotation triplets |
| `EXPECTED_DICTIONARY_ANNOTATIONS` | 145–148 | `set` of 2 tuples | Dictionary-specific annotations |
| `EID_REFERENCES` | 151–157 | `set` of 6 | EID reference container symbols |

### 3.2 SYM_TYPE Class

**Lines**: 180–188
```python
class SYM_TYPE:
    COMMON = "common"
    DICTIONARY = "dictionary"
    ORIGINAL = "original"
    BASE64 = "base64"
    SHORT = "short"
    SHARED = "shared"
    UNKNOWN = "unknown"
```

### 3.3 BookStructure Class

**Lines**: 190–1313

Methods (all are instance methods on `BookStructure`):

| Method | Lines | Signature | Purpose |
|--------|-------|-----------|---------|
| `check_consistency` | 192–701 | `def check_consistency(self)` | Full book consistency validation (very large: 510 lines) |
| `extract_fragment_id_from_value` | 703–716 | `def extract_fragment_id_from_value(self, ftype, value)` | Extract fragment ID from value struct |
| `check_fragment_usage` | 718–852 | `def check_fragment_usage(self, rebuild=False, ignore_extra=False)` | Fragment reference graph walk |
| `create_container_id` | 854–855 | `def create_container_id(self)` | Generate random CR! container ID |
| `walk_fragment` | 857–950 | `def walk_fragment(self, fragment, mandatory_frag_refs, optional_frag_refs, eid_defs, eid_refs)` | Recursively walk fragment data |
| `determine_entity_dependencies` | 952–1004 | `def determine_entity_dependencies(self, mandatory_references, optional_references)` | Build entity dependency graph |
| `rebuild_container_entity_map` | 1006–1040 | `def rebuild_container_entity_map(self, container_id, entity_dependencies=None)` | Reconstruct container entity map |
| `classify_symbol` | 1042–1087 | `def classify_symbol(self, name)` | Classify symbol name into SYM_TYPE |
| `allowed_symbol_prefix` | 1089–1090 | `def allowed_symbol_prefix(self, symbol_prefix)` | Check allowed single-char prefix |
| `create_local_symbol` | 1092–1097 | `def create_local_symbol(self, name)` | Create validated local symbol |
| `check_symbol_table` | 1099–1149 | `def check_symbol_table(self, rebuild=False, ignore_unused=False)` | Symbol table validation |
| `replace_symbol_table_import` | 1151–1159 | `def replace_symbol_table_import(self)` | Replace symbol table import fragment |
| `find_symbol_references` | 1161–1180 | `def find_symbol_references(self, data, s)` | Collect all symbol references |
| `get_reading_orders` | 1182–1189 | `def get_reading_orders(self)` | Get reading orders from document_data or metadata |
| `reading_order_names` | 1191–1192 | `def reading_order_names(self)` | List reading order names |
| `ordered_section_names` | 1194–1202 | `def ordered_section_names(self)` | Get deduplicated section names in order |
| `extract_section_story_names` | 1204–1226 | `def extract_section_story_names(self, section_name)` | Extract story names from section |
| `has_illustrated_layout_page_template_condition` | 1228–1256 | `def has_illustrated_layout_page_template_condition(self)` | Check for layout page template conditions |
| `get_ordered_image_resources` | 1258–1298 | `def get_ordered_image_resources(self)` | Get fixed-layout ordered images |
| `log_known_error` | 1300–1304 | `def log_known_error(self, msg)` | Log known errors per REPORT_KNOWN_PROBLEMS |
| `log_error_once` | 1306–1310 | `def log_error_once(self, msg)` | Log error only once per message |

### 3.4 `numstr` Helper Function

**Line**: 1313 (last line of file)
```python
def numstr(x):
    return "%g" % x
```
**Used in** `check_consistency` for comparing PDF dimensions with resource dimensions.

### 3.5 Key Function Details

#### `check_fragment_usage(self, rebuild=False, ignore_extra=False)` — Lines 718–852
**Purpose**: Build complete fragment reference graph, detect missing/unreferenced fragments, optionally rebuild.

**Algorithm**:
1. Initialize `discovered` set with root fragment types
2. Seed with cover_image metadata fragment
3. BFS loop: for each discovered fragment, call `walk_fragment` to find references
4. Track `mandatory_references` and `optional_references` per fragment
5. Report missing fragments
6. Separate referenced vs unreferenced fragments
7. If `rebuild=True`: rebuild container with `rebuild_container_entity_map`

**Calls**: `walk_fragment`, `determine_entity_dependencies`, `rebuild_container_entity_map`

#### `walk_fragment(self, fragment, mandatory_frag_refs, optional_frag_refs, eid_defs, eid_refs)` — Lines 857–950
**Purpose**: Recursively walk fragment data to discover all fragment references and EIDs.

**Inner function `walk(data, container, container_parent, top_level)`**:
- Dispatches on `ion_type(data)`:
  - `IonAnnotation`: validate annotations, recurse on value
  - `IonList`: iterate and recurse
  - `IonStruct`: recurse on each key/value
  - `IonSExp`: recurse on children (data[1:])
  - `IonString`: if container is `"$165"` or `"$636"`, convert to symbol and recurse
  - `IonSymbol`: complex reference resolution:
    - Track EID definitions (`$155`, `$598`, etc.)
    - Look up in `SPECIAL_FRAGMENT_REFERENCES`, `SPECIAL_PARENT_FRAGMENT_REFERENCES`, `NESTED_FRAGMENT_REFERENCES`, `COMMON_FRAGMENT_REFERENCES`
    - Add to `mandatory_frag_refs` or `optional_frag_refs`
    - For `$260` references, also check related `$609`, `$597`, `$267`, `$387` variants

**Dependencies**: `COMMON_FRAGMENT_REFERENCES`, `NESTED_FRAGMENT_REFERENCES`, `SPECIAL_FRAGMENT_REFERENCES`, `SPECIAL_PARENT_FRAGMENT_REFERENCES`, `FRAGMENT_ID_KEYS`, `EXPECTED_ANNOTATIONS`, `EXPECTED_DICTIONARY_ANNOTATIONS`, `EID_REFERENCES`

#### `determine_entity_dependencies(self, mandatory_references, optional_references)` — Lines 952–1004
**Purpose**: Compute deep/transitive dependencies between fragments.

**Algorithm**:
1. Skip `$387` (section) mandatory references (they're handled separately)
2. Skip `$164`→`$164` cross-references (resource→resource)
3. For each fragment, transitively expand mandatory references
4. Build `entity_dependencies` list of `IonStruct` with `$155` (entity_id), `$254` (mandatory deps), `$255` (optional deps)
5. Dependency types: `("$260", "$164")` and `("$164", "$417")`

#### `rebuild_container_entity_map(self, container_id, entity_dependencies=None)` — Lines 1006–1040
**Purpose**: Reconstruct the `$419` (container_entity_map) fragment.

**Algorithm**:
1. Remove existing `$419` fragment
2. Collect all entity IDs from non-container, non-root fragments
3. Build new `$419` IonStruct with `$252` (container_contents) and optionally `$253` (entity_dependencies)
4. Append new fragment to fragments list

---

## D1: yj_to_epub_notebook.py — Notebook/Scribe Support

**File**: `yj_to_epub_notebook.py` — 703 lines total

### 4.1 Imports (Lines 1–14)

```python
import base64, io, math, random
from lxml import etree
from PIL import Image

from .epub_output import (add_meta_name_content, SVG, SVG_NAMESPACES, XLINK_HREF)
from .ion import (ion_type, IonStruct, IonSymbol, IS)
from .message_logging import log
from .utilities import (Deserializer, disable_debug_log, type_name, urlrelpath)
```

### 4.2 Module Constants

| Name | Lines | Value | Description |
|------|-------|-------|-------------|
| `CREATE_SVG_FILES_IN_EPUB` | 16 | `True` | Whether to create separate SVG files |
| `PNG_SCALE_FACTOR` | 17 | `8` | Scale factor for density PNG |
| `PNG_DENSITY_GAMMA` | 18 | `3.5` | Gamma correction for density |
| `PNG_EDGE_FEATHERING` | 19 | `0.75` | Edge feathering threshold |
| `INCLUDE_PRIOR_LINE_SEGMENT` | 20 | `True` | Include prior line segment in paths |
| `ROUND_LINE_ENDINGS` | 21 | `True` | Round stroke line caps/joins |
| `QUANTIZE_THICKNESS` | 22 | `True` | Quantize thickness to 10% steps |
| `ANNOTATION_TEXT_OPACITY` | 23 | `0.0` | Text annotation opacity |
| `SVG_DOCTYPE` | 25 | `b"<!DOCTYPE svg ..."` | SVG DOCTYPE declaration |
| Brush type constants | 27–36 | 11 string constants | ERASER, FOUNTAIN_PEN, HIGHLIGHTER, etc. |
| `THICKNESS_NAME` | 38 | `["fine", "thin", "medium", "thick", "heavy"]` | Thickness name labels |
| `THICKNESS_CHOICES` | 40–49 | `dict[str, list[float]]` | 8 brush types with 5 thickness values each |
| `STROKE_COLORS` | 51–61 | `dict[int, tuple[str, int]]` | 10 color index→(name, hex) mappings |
| `MIN_TAF` | 64 | `0` | Min thickness adjust factor |
| `MAX_TAF` | 65 | `1000` | Max thickness adjust factor |
| `MIN_DAF` | 67 | `0` | Min density adjust factor |
| `MAX_DAF` | 68 | `300` | Max density adjust factor |

### 4.3 KFX_EPUB_Notebook Class

**Lines**: 73–613
**Methods**:

| Method | Lines | Signature |
|--------|-------|-----------|
| `__init__` | 75–76 | `def __init__(self)` |
| `process_scribe_notebook_page_section` | 78–156 | `def process_scribe_notebook_page_section(self, section, page_template, section_name, seq)` |
| `process_scribe_notebook_template_section` | 158–196 | `def process_scribe_notebook_template_section(self, section, page_template, section_name)` |
| `process_notebook_content` | 197–245 | `def process_notebook_content(self, content, parent)` |
| `scribe_notebook_stroke` | 247–534 | `def scribe_notebook_stroke(self, content, parent, location_id)` |
| `scribe_notebook_annotation` | 536–552 | `def scribe_notebook_annotation(self, annotation, elem)` |
| `scribe_annotation_content` | 554–613 | `def scribe_annotation_content(self, content, elem)` |

### 4.4 Standalone Functions

#### `adjust_color_for_density(color, density)` — Lines 615–622
```python
def adjust_color_for_density(color, density):
    r = (color >> 16) & 255
    g = (color >> 8) & 255
    b = color & 255
    lum = (r + g + b) // 3
    lum2 = min(max(round(255 - int((255 - lum) * density)), 0), 255)
    return (lum2 << 16) + (lum2 << 8) + lum2
```
**Purpose**: Adjust a color integer by density factor (luminance-based). Converts to grayscale, applies density, returns packed RGB int.

#### `decode_stroke_values(data, num_points, name)` — Lines 624–703
```python
def decode_stroke_values(data, num_points, name):
```
**Purpose**: Decode binary-encoded stroke value data (delta-compressed).

**Algorithm**:
1. Verify 2-byte signature `\x01\x01`
2. Unpack `num_vals` (uint32 LE) — must match `num_points`
3. Extract instruction nibbles (4-bit per byte, high nibble first)
4. Per instruction:
   - Bits 0-1 (`n`): number of bytes for increment
   - Bit 2: if set, increment = n directly; else read n bytes
   - Bit 3: if set, negate increment
5. Apply delta-decoding: `change += increment; value += change`
6. Return list of decoded integer values

**Dependencies**: `Deserializer` from utilities, `log`

---

## D2: yj_to_image_book.py — CBZ/PDF Image Book Output

**File**: `yj_to_image_book.py` — 353 lines total

### 5.1 Imports (Lines 1–12)

```python
import collections, datetime, io, re, zipfile
from .message_logging import log
from .resources import (
    combine_image_tiles, convert_image_to_pdf, convert_jxr_to_jpeg_or_png, convert_pdf_page_to_image,
    crop_image, ImageResource, PdfImageResource, pypdf, SYMBOL_FORMATS)
from .utilities import (json_serialize_compact, list_counts)
from .yj_to_epub import KFX_EPUB
```

### 5.2 Module Constants

| Name | Line | Value |
|------|------|-------|
| `USE_HIGHEST_RESOLUTION_IMAGE_VARIANT` | 14 | `True` |
| `DEBUG_VARIANTS` | 15 | `False` |

### 5.3 KFX_IMAGE_BOOK Class

**Lines**: 18–213

| Method | Lines | Signature | Purpose |
|--------|-------|-----------|---------|
| `__init__` | 22–23 | `def __init__(self, book)` | Store book reference |
| `convert_book_to_cbz` | 25–55 | `def convert_book_to_cbz(self, split_landscape_comic_images, progress)` | Convert to CBZ with ComicBookInfo metadata |
| `convert_book_to_pdf` | 57–99 | `def convert_book_to_pdf(self, split_landscape_comic_images, progress)` | Convert to PDF with TOC outline |
| `get_ordered_images` | 101–155 | `def get_ordered_images(self, split_landscape_comic_images=False, is_comic=False, is_rtl=False, progress=None)` | Get ordered image list with optional landscape splitting |
| `get_resource_image` | 157–213 | `def get_resource_image(self, resource_name, ignore_variants=False)` | Get ImageResource for a fragment, handling tiles, variants, formats |

### 5.4 Standalone Functions

#### `combine_images_into_pdf(ordered_images, metadata=None, is_rtl=False, outline=None)` — Lines 215–294
**Purpose**: Combine image resources into a single PDF file.
- Handles PDF pages, raster images converted to PDF
- Supports pypdf for combining with metadata, RTL, and outline
- Uses `pypdf.PdfWriter`, `pypdf.PdfReader`
- Calls `convert_image_to_pdf()` for raster images
- Calls `add_pdf_outline()` for TOC

#### `add_pdf_outline(pdf_writer, outline_entries, parent=None)` — Lines 296–302
**Purpose**: Recursively add PDF outline/bookmarks.
```python
def add_pdf_outline(pdf_writer, outline_entries, parent=None):
    for outline_entry in outline_entries:
        new_entry = pdf_writer.add_outline_item(outline_entry.title, outline_entry.page_num, parent=parent)
        if outline_entry.children:
            add_pdf_outline(pdf_writer, outline_entry.children, new_entry)
```

#### `combine_images_into_cbz(ordered_images, metadata=None)` — Lines 304–347
**Purpose**: Combine image resources into CBZ (ZIP) file.
- Handles PDF pages (converted to images), JXR (converted to JPEG/PNG), and direct image formats
- Names pages as `0001.ext`, `0002.ext`, etc.
- Stores ComicBookInfo metadata as JSON in ZIP comment
- Uses `SYMBOL_FORMATS` for format→extension mapping

#### `suffix_location(location, suffix)` — Lines 349–353
**Purpose**: Add suffix before file extension.
```python
def suffix_location(location, suffix):
    if "." in location:
        return re.sub("\\.", suffix + ".", location, count=1)
    return location + suffix
```

---

## D3: Floating-Point Precision — CSS Value Formatting

### 6.1 `value_str(quantity, unit="", emit_zero_unit=False)` — epub_output.py Lines 1373–1393

**This is the core CSS numeric formatting function used everywhere.**

```python
def value_str(quantity, unit="", emit_zero_unit=False):
    if quantity is None:
        return unit

    if abs(quantity) < 1e-6:
        q_str = "0"
    elif type(quantity) is float:
        q_str = "%g" % quantity
    else:
        q_str = str(quantity)

    if "e" in q_str.lower():
        q_str = "%.4f" % quantity

    if "." in q_str:
        q_str = q_str.rstrip("0").rstrip(".")

    if q_str == "0" and not emit_zero_unit:
        return q_str

    return q_str + unit
```

**Formatting Rules** (critical for Go port):
1. `None` → return just the unit string
2. Near-zero (`abs < 1e-6`) → `"0"` (no unit unless `emit_zero_unit=True`)
3. `float` → Python's `%g` format (which removes trailing zeros)
4. Non-float (int, Decimal) → `str()`
5. If scientific notation detected (`e` in result) → reformat as `%.4f`
6. Strip trailing zeros after decimal: `"1.500" → "1.5"`, `"2.0" → "2"`
7. `"0"` without `emit_zero_unit` returns just `"0"` (no unit suffix)
8. Otherwise: `q_str + unit`

### 6.2 `color_str(self, rgb_int, alpha=1.0)` — yj_to_epub_properties.py Lines ~2373–2388

```python
def color_str(self, rgb_int, alpha=1.0):
    if alpha == 1.0:
        hex_color = "#%06x" % (rgb_int & 0x00ffffff)
        if hex_color in COLOR_NAME:
            return COLOR_NAME[hex_color]
        return "#000" if hex_color == "#000000" else ("#fff" if hex_color == "#ffffff" else hex_color)

    red = (rgb_int & 0x00ff0000) >> 16
    green = (rgb_int & 0x0000ff00) >> 8
    blue = (rgb_int & 0x000000ff)
    alpha_str = "0" if alpha == 0.0 else "%0.3f" % alpha
    return "rgba(%d,%d,%d,%s)" % (red, green, blue, alpha_str)
```

**Rules**:
- alpha=1.0: Use named color if known, else 3-char hex for black/white, else 6-char hex
- alpha≠1.0: `rgba(r,g,b,alpha_str)` with alpha formatted as `%0.3f` (or `"0"` for exactly 0.0)

### 6.3 `int_to_alpha(self, alpha_int)` — yj_to_epub_properties.py

```python
def int_to_alpha(self, alpha_int):
    if alpha_int < 2:
        return 0.0
    if alpha_int > 253:
        return 1.0
    return max(min(float(alpha_int + 1) / 256.0, 1.0), 0.0)
```

### 6.4 `alpha_to_int(self, alpha)` — yj_to_epub_properties.py

```python
def alpha_to_int(self, alpha):
    if alpha < 0.012:
        return 0
    if alpha > 0.996:
        return 255
    return max(min(int(alpha * 256.0 + 0.5) - 1, 255), 0)
```

### 6.5 Where value_str is Used

`value_str` is imported and used in `yj_to_epub_properties.py` in the `property_value` method and `simplify_styles`:
- Formatting CSS length values with units (e.g., `value_str(magnitude, unit_string)`)
- Formatting pixel values (e.g., `value_str(self.adjust_pixel_value(yj_value))`)
- Formatting line-height (e.g., `value_str(quantity * LINE_HEIGHT_SCALE_FACTOR, "em")`)
- Formatting converted viewport units (e.g., `value_str(quantity, "%")`)

### 6.6 `numstr(x)` — yj_structure.py Line 1313

```python
def numstr(x):
    return "%g" % x
```
Used in `check_consistency` for comparing PDF image dimensions with resource metadata dimensions.

---

## Cross-File Dependency Summary

### yj_versions.py
- **Depends on**: Nothing (standalone data module)
- **Used by**: `yj_structure.py` (imports `is_known_aux_metadata`, `is_known_kcb_data`)

### yj_structure.py
- **Depends on**: `ion`, `kfx_container`, `message_logging`, `resources`, `utilities`, `version`, `yj_container`, `yj_versions`
- **Used by**: All conversion modules that need book structure

### yj_to_epub_notebook.py
- **Depends on**: `base64`, `io`, `math`, `random`, `lxml.etree`, `PIL.Image`, `epub_output`, `ion`, `message_logging`, `utilities`
- **Note**: `KFX_EPUB_Notebook` is a mixin class — it's used by composition in the main `KFX_EPUB` class

### yj_to_image_book.py
- **Depends on**: `collections`, `datetime`, `io`, `re`, `zipfile`, `message_logging`, `resources`, `utilities`, `yj_to_epub`
- **Note**: `KFX_IMAGE_BOOK` wraps `KFX_EPUB` for metadata extraction

### yj_to_epub_properties.py
- **Depends on**: `collections`, `decimal`, `functools`, `lxml.etree`, `re`, `epub_output` (imports `value_str`), `ion`, `message_logging`, `utilities`
- **Key type**: Uses `decimal.Decimal` for `LINE_HEIGHT_SCALE_FACTOR = decimal.Decimal("1.2")`

### epub_output.py
- **Defines**: `value_str()` at line 1373 — the universal CSS numeric formatting function
