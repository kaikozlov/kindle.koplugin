# Python Reference Code Summary

Source: `REFERENCE/Calibre_KFX_Input/kfxlib/`

## File Listing with Line Counts

### Core Conversion Files (porting targets)
| File | Lines | Purpose |
|------|-------|---------|
| `yj_to_epub.py` | 355 | Main conversion orchestrator |
| `yj_structure.py` | 1313 | Fragment structure, symbol classification, consistency checks |
| `yj_position_location.py` | 1324 | Position/location maps, content chunking |
| `yj_metadata.py` | 885 | Metadata reading/writing, cover image handling |
| `yj_versions.py` | 1124 | Feature/metadata constants, version mapping |
| `epub_output.py` | 1504 | EPUB generation, OPF/NCX creation, zip packaging |
| `yj_to_epub_content.py` | 1944 | Content processing (HTML/XHTML generation) |
| `yj_to_epub_notebook.py` | 703 | Scribe notebook SVG/stroke processing |
| `yj_to_image_book.py` | 353 | CBZ/PDF image book conversion |

### Supporting Files
| File | Lines | Purpose |
|------|-------|---------|
| `yj_to_epub_illustrated_layout.py` | 408 | Illustrated layout conditional processing |
| `yj_to_epub_metadata.py` | 290 | EPUB metadata conversion |
| `yj_to_epub_misc.py` | 611 | Misc helpers (conditions, plugins, fonts) |
| `yj_to_epub_navigation.py` | 541 | TOC/navigation generation |
| `yj_to_epub_properties.py` | 2486 | CSS property conversion (largest file) |
| `yj_to_epub_resources.py` | 374 | Resource/font processing |
| `yj_book.py` | 348 | Book container (YJ_Book class) |
| `yj_container.py` | 385 | Fragment containers, symbol tables |
| `yj_symbol_catalog.py` | 876 | Symbol catalog management |
| `kfx_container.py` | 449 | KFX container format |
| `kpf_book.py` | 551 | KPF book format |
| `kpf_container.py` | 445 | KPF container format |
| `original_source_epub.py` | 1367 | Original EPUB source handling |
| `resources.py` | 804 | Image resource handling, format conversion |
| `utilities.py` | 822 | Utility functions |
| `unpack_container.py` | 159 | Container unpacking |
| `ion.py` | 380 | Ion data type base |
| `ion_binary.py` | 623 | Ion binary encoding |
| `ion_symbol_table.py` | 353 | Ion symbol tables |
| `ion_text.py` | 1109 | Ion text encoding |
| `jxr_container.py` | 123 | JXR container |
| `jxr_image.py` | 2544 | JPEG XR image processing |
| `jxr_misc.py` | 107 | JXR misc helpers |
| `message_logging.py` | 66 | Logging utilities |
| `version.py` | 1 | Version string |
| `__init__.py` | 33 | Package init |

### Third-party (calibre-plugin-modules/)
Includes pypdf (~35k lines), typing_extensions (~4584 lines) - not porting targets.

---

## Detailed File Summaries

### 1. `yj_to_epub.py` (355 lines) — Main Conversion Orchestrator

**Key Class: `KFX_EPUB`** (multiple inheritance mixin)
- Inherits from: `KFX_EPUB_Content`, `KFX_EPUB_Illustrated_Layout`, `KFX_EPUB_Metadata`, `KFX_EPUB_Misc`, `KFX_EPUB_Navigation`, `KFX_EPUB_Notebook`, `KFX_EPUB_Properties`, `KFX_EPUB_Resources`, `EPUB_Output`
- Constructor `__init__(self, book, epub2_desired=False, force_cover=False, metadata_only=False, progress=None)`
  - Calls `organize_fragments_by_type()` to categorize book fragments
  - Calls pipeline: `determine_book_symbol_format()` → `process_content_features()` → `process_fonts()` → `process_document_data()` → `process_metadata()` → `set_condition_operators()` → `process_anchors()` → `process_navigation()` → `process_reading_order()` → ... → `prepare_book_parts()` → `report_missing_positions()`
  - Validates all fragments consumed via `check_empty()`

**Constants:**
- `FRAGMENT_NAME_SYMBOL` — dict mapping ftype → name key symbol (e.g., `"$266": "$180"`)
- `PROGRESS_FTYPES` — `{"$260", "$259", "$164"}`

**Key Methods:**
- `decompile_to_epub()` — calls `generate_epub()` (from EPUB_Output)
- `organize_fragments_by_type(fragment_list)` — categorizes fragments by ftype into `categorized_data` dict
- `determine_book_symbol_format()` — determines if symbols are SHORT, DICTIONARY, BASE64, or ORIGINAL
- `unique_part_of_local_symbol(symbol)` / `prefix_unique_part_of_symbol()`
- `replace_ion_data(f)` — recursively collects symbols from Ion data
- `get_fragment(ftype, fid, delete=True)` — retrieves fragment from book_data, tracks usage
- `get_named_fragment(structure, ftype, delete, name_symbol)`
- `check_fragment_name()`, `get_fragment_name()`, `get_structure_name()`
- `progress_countdown()`, `update_progress()`

**Dependencies:** All `yj_to_epub_*` modules, `epub_output`, `ion`, `yj_structure`

---

### 2. `yj_structure.py` (1313 lines) — Fragment Structure & Symbol Classification

**Key Constants:**
- `METADATA_SYMBOLS` — maps metadata names to Ion symbols (e.g., `"ASIN": "$224"`, `"title": "$153"`)
- `METADATA_NAMES` — reverse mapping
- `FRAGMENT_ID_KEYS` — maps ftype to list of ID key symbols
- `COMMON_FRAGMENT_REFERENCES` — maps container symbols to ftype for fragment walking
- `NESTED_FRAGMENT_REFERENCES`, `SPECIAL_FRAGMENT_REFERENCES`, `SPECIAL_PARENT_FRAGMENT_REFERENCES`
- `SECTION_DATA_TYPES` — `{"$387", "$260", "$267", "$609"}`
- `EID_REFERENCES` — `{"$185", "$155", "$598", "$754", "$474", "$163"}`
- `EXPECTED_ANNOTATIONS`, `EXPECTED_DICTIONARY_ANNOTATIONS`

**Key Class: `SYM_TYPE`**
- Constants: `COMMON`, `DICTIONARY`, `ORIGINAL`, `BASE64`, `SHORT`, `SHARED`, `UNKNOWN`

**Key Class: `BookStructure`** (mixin for Book)
- `check_consistency()` — ~350 line validation of fragment structure, resource formats, features
- `extract_fragment_id_from_value(ftype, value)` — extracts fragment ID from value struct
- `check_fragment_usage(rebuild, ignore_extra)` — walks all fragments to find references/missing
- **`classify_symbol(name)`** — classifies a symbol name into SYM_TYPE using regex patterns:
  - COMMON: UUIDs, page lists, content_N, eidbucket_N, yj.authoring/conversion/print/semantics.*, nmdl.*
  - DICTIONARY: G[0-9]+ patterns, yj.dictionary.*
  - ORIGINAL: V_ patterns, resource_ patterns, anchor- patterns
  - BASE64: 22-char base64-like strings with optional suffixes
  - SHORT: short uppercase codes (1-6 chars) with optional suffixes
- **`walk_fragment(fragment, mandatory_frag_refs, optional_frag_refs, eid_defs, eid_refs)`** — recursive Ion data walker that:
  - Tracks IonAnnotation validation
  - Resolves fragment references via COMMON/NESTED/SPECIAL maps
  - Collects EID definitions and references
- `determine_entity_dependencies()` — builds dependency graph between fragments
- `rebuild_container_entity_map()` — rebuilds entity map
- `check_symbol_table()`, `find_symbol_references()` — symbol table validation
- `get_reading_orders()`, `reading_order_names()`, `ordered_section_names()`
- `extract_section_story_names(section_name)` — extracts story names from section
- `has_illustrated_layout_page_template_condition()` — scans for conditional templates
- `get_ordered_image_resources()` — for fixed-layout books

**Dependencies:** `ion`, `kfx_container`, `resources`, `utilities`, `yj_container`, `yj_versions`

---

### 3. `yj_position_location.py` (1324 lines) — Position & Location Maps

**Key Constants:**
- `KFX_POSITIONS_PER_LOCATION = 110`
- `TYPICAL_POSITIONS_PER_PAGE = 1850`
- `RANGE_OPERS = ["$298", "$299"]`

**Key Class: `ContentChunk`** — represents a chunk of content at a position
- Fields: `pid`, `eid`, `eid_offset`, `length`, `section_name`, `match_zero_len`, `text`, `image_resource`
- `__eq__(other, compare_pids=True)` — compares chunks by eid/offset/section

**Key Class: `ConditionalTemplate`** — for illustrated layout conditional processing
- Fields: `end_eid`, `end_eid_offset`, `oper`, `pos_info`, `use_next`, `start_eid`, `start_eid_offset`

**Key Class: `MatchReport`** — error reporting with limits

**Key Class: `BookPosLoc`** (mixin for Book)
- `check_position_and_location_maps()` — main entry: collects content info, verifies against maps
- **`collect_content_position_info()`** — ~300 line method that:
  - Iterates sections in reading order
  - For each section, defines nested `collect_section_position_info()` with deeply nested `extract_position_data()`
  - Handles ContentChunks, ConditionalTemplates, story references
  - Processes EID tracking, position counting
  - Handles KIM (Kindle In-Magazine) templates with conditions
- `anchor_eid_offset(anchor)` — resolves anchor to (eid, offset)
- `collect_position_map_info()` — reads position_id_map fragments
- `verify_position_info()` — compares content vs map position info
- `create_position_map(pos_info)` — rebuilds position maps
- `pid_for_eid(eid, eid_offset, pos_info)` — linear search for pid
- `eid_for_pid(pid, pos_info)` — binary search for (eid, offset)
- `collect_location_map_info(pos_info)` — processes location_map ($550) and yj.location_pid_map ($621)
- `generate_approximate_locations(pos_info)` — generates locations at KFX_POSITIONS_PER_LOCATION intervals
- `create_location_map(loc_info)` — creates location map fragment
- `create_approximate_page_list(desired_num_pages)` — creates approximate page numbers
- `determine_approximate_pages()` — calculates page breaks based on positions_per_page

**Dependencies:** `ion`, `yj_container`, `yj_structure`

---

### 4. `yj_metadata.py` (885 lines) — Metadata & Cover Handling

**Key Class: `YJ_Metadata`** — simple data class
- Fields: `authors`, `author_sort_fn`, `title`, `cde_content_type`, `asin`, `cover_image_data`, `description`, `issue_date`, `language`, `publisher`, `book_id`, `features`, `asset_id`

**Key Class: `BookMetadata`** (mixin for Book)
- `get_yj_metadata_from_book()` — reads metadata from $490 (book_metadata) and $258 (metadata) fragments
- `set_yj_metadata_to_book(yj_metadata)` — writes metadata back to fragments
- `has_metadata()`, `has_cover_data()`
- **`get_metadata_value(name, category, default)`** — looks up metadata from:
  1. `$490` fragment (book_metadata) with category (default "kindle_title_metadata")
  2. `$258` fragment using METADATA_SYMBOLS reverse lookup
- **`get_feature_value(feature, namespace, default)`** — looks up features from:
  1. `format_capabilities` namespace → `$593` fragment
  2. `com.amazon.yjconversion` (default) → `$585` fragment
- `get_generators()` — extracts (application_version, package_version) from $270 fragments
- `get_features()` — collects all features from both $593 and $585 fragments
- `get_page_count()` — counts pages from $389 navigation
- `report_features_and_metadata()` — reports features with version compatibility
- `get_cover_image_data()` — returns (format, raw_bytes) for cover image
- `fix_cover_image_data(cover_image_data)` — converts to JFIF JPEG
- `set_cover_image_data(cover_image_data)` — replaces cover image
- `check_cover_section_and_storyline()` — validates cover structure
- `update_cover_section_and_storyline()` — updates cover dimensions
- `update_image_resource_and_media()` — updates resource fragment

**Cached Properties:** `cde_type`, `is_magazine`, `is_sample`, `is_fixed_layout`, `is_image_based_fixed_layout`, `is_print_replica`, `is_pdf_backed_fixed_layout`, `is_illustrated_layout`, `has_illustrated_layout_conditional_page_template`, `is_kfx_v1`, `has_pdf_resource`

**Standalone Functions:**
- `author_sort_name(author)` — "First Last" → "Last, First"
- `unsort_author_name(author)` — "Last, First" → "First Last"
- `fix_language_for_kfx(language)` — language code normalization

**Dependencies:** `ion`, `resources`, `utilities`, `yj_container`, `yj_structure`, `yj_versions`

---

### 5. `yj_versions.py` (1124 lines) — Constants & Feature Mapping

**Feature Constants (strings):**
- ~100+ feature name constants: `YJ`, `YJ_REFLOWABLE_V2`, `YJ_FIXED_LAYOUT_V2`, `YJ_DICT_V1`, etc.
- `PACKAGE_VERSION_PLACEHOLDERS` — set of placeholder version strings

**Major Data Structures:**
- **`KNOWN_KFX_GENERATORS`** — set of (version, package_version) tuples for valid generators
- **`GENERIC_CREATOR_VERSIONS`** — known creator tool versions
- **`KNOWN_FEATURES`** — nested dict: `{namespace: {feature_name: {value: capability_constant}}}`
  - Namespaces: `format_capabilities`, `SDK.Marker`, `com.amazon.kindle.nmdl`, `com.amazon.yjconversion`
  - Each feature maps version values to capability constants
- **`KNOWN_SUPPORTED_FEATURES`** — set of known feature tuples
- **`KNOWN_METADATA`** — nested dict of valid metadata values per category:
  - Categories: `book_navigation`, `book_requirements`, `kindle_audit_metadata`, `kindle_capability_metadata`, `kindle_ebook_metadata`, `kindle_title_metadata`, `metadata`, `symbols`
- **`KNOWN_AUXILIARY_METADATA`** — known auxiliary data keys/values
- **`KNOWN_KCB_DATA`** — known KPF container book data
- **`KINDLE_VERSION_CAPABILITIES`** — maps Kindle firmware versions to lists of capabilities
- **`KINDLE_CAPABILITY_VERSIONS`** — reverse mapping: capability → minimum Kindle version

**Key Functions:**
- `is_known_generator(kfxgen_application_version, kfxgen_package_version)` — validates generator
- `is_known_feature(cat, key, val)` — checks if feature is known
- `kindle_feature_version(cat, key, val)` — returns minimum Kindle version for feature
- `is_known_metadata(cat, key, val)` — validates metadata value
- `is_known_aux_metadata(key, val)` — validates auxiliary metadata
- `is_known_kcb_data(cat, key, val)` — validates KCB data

**Dependencies:** None (pure constants + functions)

---

### 6. `epub_output.py` (1504 lines) — EPUB Generation

**Constants:**
- XML namespace URIs: `XHTML_NS_URI`, `EPUB_NS_URI`, `OPF_NS_URI`, `NCX_NS_URI`, `SVG_NS_URI`, etc.
- `MANIFEST_ITEM_PROPERTIES`, `SPINE_ITEMREF_PROPERTIES`, `OPF_PROPERTIES`
- `STANDARD_GUIDE_TYPE`, `EPUB3_VOCABULARY_OF_GUIDE_TYPE`, `DEFAULT_LABEL_OF_GUIDE_TYPE`
- File path templates: `OPF_FILEPATH`, `NCX_FILEPATH`, `TEXT_FILEPATH`, etc.

**Key Classes:**
- **`OPFProperties`** — manages OPF property sets; properties: `is_fxl`, `is_nav`, `is_cover_image`
- **`BookPart(OPFProperties)`** — represents an XHTML file in the EPUB
  - Fields: `filename`, `part_index`, `html`, `opf_properties`, `linear`, `omit`, `idref`, `is_cover_page`, `nmdl_template_id`
- **`ManifestEntry(OPFProperties)`** — represents a manifest item
  - Fields: `filename`, `opf_properties`, `linear`, `external`, `id`, `reference_count`
- **`TocEntry`** — TOC entry with title, target, children, description, icon, anchor
- **`GuideEntry`** — guide reference with guide_type, title, target, anchor
- **`PageMapEntry`** — page mapping with label, target, anchor
- **`OutputFile`** — output file data with binary_data, mimetype, height, width

**Key Class: `EPUB_Output`**
- `__init__(epub2_desired, force_cover, will_output)` — initializes EPUB metadata fields
- `set_book_type(book_type)` — sets children/comic/magazine/notebook flags
- `set_primary_writing_mode(mode)` — sets writing_mode and page_progression_direction
- **`generate_epub()`** — main EPUB generation pipeline:
  1. Set UID (ASIN/UUID)
  2. `check_epub_version()` — may upgrade to EPUB3
  3. `identify_cover()` — find cover page
  4. Save HTML parts → `save_book_parts()` (serializes XHTML, adds to manifest)
  5. Create NCX → `create_ncx()` if EPUB2 compatible
  6. Create EPUB3 nav → `create_epub3_nav()` if EPUB3
  7. **`create_opf()`** — generates content.opf XML with metadata, manifest, spine, guide
  8. `zip_epub()` — packages everything into ZIP
- `manifest_resource(filename, ...)` — adds file to manifest
- `new_book_part(filename, ...)` — creates new BookPart
- `link_css_file(book_part, css_file)` — adds CSS link to book part
- `save_book_parts()` — serializes all book parts to XHTML, applies consolidation/beautification
- **`create_opf()`** — generates OPF XML with: dc:identifier, dc:title, dc:creator, dc:language, dc:publisher, dc:date, rendition metadata, manifest items, spine itemrefs, guide references
- `create_ncx()` — generates NCX (EPUB2 TOC)
- `create_epub3_nav()` — generates EPUB3 navigation document
- `zip_epub()` — creates final EPUB ZIP file

**Utility Functions:**
- `qname(ns, name)`, `xhtmlns(name)`, `new_xhtml()` — namespace helpers
- `add_meta_name_content()`, `add_attribs()` — XML element helpers
- `value_str(quantity, unit)` — formats CSS values
- `split_value(val)` — parses CSS value strings
- `roman_to_int(input)` — Roman numeral conversion

**Dependencies:** `lxml`, `PIL`, `zipfile`, `uuid`, `datetime`, `resources`, `utilities`

---

### 7. `yj_to_epub_content.py` (1944 lines) — Content Processing

**Key Constants:**
- `LIST_STYLE_TYPES` — maps list type symbols to HTML tags (ol/ul)
- `CLASSIFICATION_EPUB_TYPE` — maps classification symbols to epub types (footnote/endnote)
- `BLOCK_ALIGNED_CONTAINER_PROPERTIES`, `BLOCK_CONTAINER_PROPERTIES`, `LINK_CONTAINER_PROPERTIES`
- `UNEXPECTED_CHARACTERS` — set of Unicode code points to filter
- `NBSP = "\u00a0"`

**Key Class: `KFX_EPUB_Content`**
- `__init__()` — initializes context stack, style tracking
- **`process_reading_order()`** — iterates reading orders, calls `process_section()` for each
- **`process_section(section, seq)`** — main section processing dispatcher:
  - `"nmdl.canvas_width"` → `process_scribe_notebook_page_section()`
  - `"nmdl.template_type"` → `process_scribe_notebook_template_section()`
  - Comic/children → `process_page_spread_page_template()`
  - Magazine/print-replica with conditional → conditional template processing
  - Default → `process_content()` for page templates
- **`process_page_spread_page_template()`** — handles page spreads (facing pages, comics)
- `process_story(story, parent, book_part, writing_mode)` — processes a story fragment
- `add_content(content, parent, book_part, writing_mode)` — dispatches content by type
- `process_content_list(content_list, ...)` — iterates content list
- **`process_content(content, parent, book_part, writing_mode, ...)`** — ~500 line method, the core content converter:
  - `IonString` → `<span>`
  - `IonSymbol` → resolve fragment and recurse
  - `$269` (text) → `<div>` with text content
  - `$271` (image) → `<img>` with resource
  - `$274` (plugin) → plugin processing
  - `$439` (non-rendered) → hidden `<div>`
  - `$270` (container) → `<div>` with layout processing (scale_fit, inline, fixed, etc.)
  - `$276` (list) → `<ol>/<ul>`
  - `$277` (list item) → `<li>`
  - `$278` (table) → `<table>` with colgroup
  - `$454/$151/$455` → `<tbody>/<thead>/<tfoot>`
  - `$279` (table row) → `<tr>`
  - `$596` (horizontal rule) → `<hr>`
  - `$272` (SVG) → `<svg>` with KVG shape processing
  - Handles: annotations ($683), word boundaries ($696), conditions ($591/$592), conditional properties ($663), style events ($142), dropcaps, ruby text
- `create_container()`, `create_span_subcontainer()` — element wrapping
- `fix_vertical_align_properties()` — vertical-align CSS handling
- `content_text(content)` — extracts text from content
- `combined_text(elem)` — concatenates all text in element tree
- `locate_offset()`, `locate_offset_in()`, `split_span()` — text offset finding/splitting for style events
- `find_or_create_style_event_element()` — creates span for style events at offsets
- `get_ruby_content(ruby_name, ruby_id)` — retrieves ruby text content
- `prepare_book_parts()` — post-processing: replaces EOL with `<br>`, handles whitespace
- `preformat_spaces(elem)` — handles non-breaking spaces
- `add_kfx_style(content, kfx_style_name)` — applies KFX style to content
- `clean_text_for_lxml(text)` — filters unexpected characters

**Dependencies:** `epub_output`, `ion`, `ion_text`, `yj_to_epub_properties`

---

### 8. `yj_to_epub_notebook.py` (703 lines) — Scribe Notebook Processing

**Key Constants:**
- Brush types: `FOUNTAIN_PEN`, `HIGHLIGHTER`, `MARKER`, `PEN`, `PENCIL`, `SHADER`, etc.
- `THICKNESS_CHOICES` — dict mapping brush type to thickness values
- `STROKE_COLORS` — maps color index to (name, hex) tuples
- `PNG_SCALE_FACTOR = 8`, `PNG_DENSITY_GAMMA = 3.5`

**Key Class: `KFX_EPUB_Notebook`**
- `process_scribe_notebook_page_section(section, page_template, section_name, seq)` — creates fixed-layout SVG page with notebook strokes
- `process_scribe_notebook_template_section(section, page_template, section_name)` — processes template sections
- `process_notebook_content(content, parent)` — recursive notebook content processor
- **`scribe_notebook_stroke(content, parent, location_id)`** — processes individual strokes:
  - `nmdl.stroke_group` → SVG `<g>` group with annotations
  - `nmdl.stroke` → SVG stroke rendering:
    - Decodes brush type, color, thickness
    - Creates SVG path elements or density-based PNG images
    - Handles variable density/thickness via decode_stroke_values()
- `scribe_notebook_annotation(annotation, elem)` — processes handwriting recognition annotations
- `scribe_annotation_content(content, elem)` — renders HWR text as SVG text/tspan

**Standalone Functions:**
- `decode_stroke_values(data, num_points, name)` — decodes binary stroke point data using delta encoding
- `adjust_color_for_density(color, density)` — adjusts color for density rendering

**Dependencies:** `epub_output`, `ion`, `utilities`

---

### 9. `yj_to_image_book.py` (353 lines) — Image Book Conversion

**Key Class: `KFX_IMAGE_BOOK`**
- `__init__(book)` — stores book reference
- `convert_book_to_cbz(split_landscape_comic_images, progress)` — converts to CBZ comic archive
- `convert_book_to_pdf(split_landscape_comic_images, progress)` — converts to PDF with TOC
- `get_ordered_images(split_landscape_comic_images, is_comic, is_rtl, progress)` — extracts ordered image resources, optionally splitting landscape images
- `get_resource_image(resource_name)` — retrieves image resource, handles tiles, variants, PDF pages

**Standalone Functions:**
- `combine_images_into_pdf(ordered_images, metadata, is_rtl, outline)` — combines images into PDF using pypdf
- `combine_images_into_cbz(ordered_images, metadata)` — combines images into CBZ ZIP
- `add_pdf_outline(pdf_writer, outline_entries, parent)` — recursive PDF outline builder
- `suffix_location(location, suffix)` — adds suffix to location filename

**Dependencies:** `KFX_EPUB` (from yj_to_epub), `resources`, `utilities`, `pypdf`, `zipfile`

---

## Cross-File Dependencies Graph

```
yj_to_epub.py ──→ (all yj_to_epub_* modules, epub_output, ion, yj_structure)
yj_structure.py ──→ (ion, kfx_container, resources, utilities, yj_container, yj_versions)
yj_position_location.py ──→ (ion, utilities, yj_container, yj_structure)
yj_metadata.py ──→ (ion, resources, utilities, yj_container, yj_structure, yj_versions)
yj_versions.py ──→ (none - pure constants)
epub_output.py ──→ (resources, utilities, lxml, PIL, zipfile)
yj_to_epub_content.py ──→ (epub_output, ion, ion_text, yj_to_epub_properties)
yj_to_epub_notebook.py ──→ (epub_output, ion, utilities)
yj_to_image_book.py ──→ (yj_to_epub, resources, utilities)
```

## Fragment Type Symbol Reference (most commonly used)

| Symbol | Name | Description |
|--------|------|-------------|
| `$260` | section | Book section content |
| `$259` | storyline | Story line content |
| `$164` | external_resource | External resource (images, fonts) |
| `$417` | raw_media | Raw binary media data |
| `$157` | kfx_style | KFX style definition |
| `$145` | content_fragment | Content fragment |
| `$266` | anchor | Position anchor |
| `$391` | nav_container | Navigation container |
| `$393` | nav_entry | Navigation entry |
| `$389` | book_navigation | Book navigation |
| `$270` | container | KFX container |
| `$593` | format_capabilities | Format capabilities |
| `$538` | document_data | Document data |
| `$258` | metadata | Book metadata |
| `$490` | book_metadata | Extended book metadata |
| `$585` | feature_data | Feature data |
| `$550` | location_map | Location map |
| `$608` | page_template | Page template |
| `$609` | section_position_id_map | Section position ID map |
| `$610` | eidhash_eid_section_map | EID hash section map |
| `$611` | dictionary_position_id_map | Dictionary position map |
| `$265` | position_id_map | Position ID map |
| `$264` | position_map | Position map |
| `$621` | yj_location_pid_map | Location PID map |
| `$756` | ruby_content | Ruby text content |
| `$419` | container_entity_map | Container entity map |
| `$262` | font | Font data |
| `$387` | media_overlay | Media overlay |
| `$597` | auxiliary_data | Auxiliary data |
