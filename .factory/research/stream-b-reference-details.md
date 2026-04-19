# Stream B: Precise Python Reference Details

This document provides exhaustive line-by-line detail for every Python function being ported in Stream B.
Source: `/Users/kai/dev/projects/kindle.koplugin/REFERENCE/Calibre_KFX_Input/kfxlib/`

---

## B1: EPUB Packaging Parity — `epub_output.py`

File: `epub_output.py` (1388 lines)

### Key Constants / Classes (lines 1-233)

**Constants:**
- `GENERATE_EPUB2_NCX_DOCTYPE = False` (line 30)
- `CONSOLIDATE_HTML = True` (line 31)
- `BEAUTIFY_HTML = True` (line 32)
- `USE_HIDDEN_ATTRIBUTE = True` (line 33)
- `STANDARD_GUIDE_TYPE = {"srl": "text"}` (lines 36-38)
- `EPUB3_VOCABULARY_OF_GUIDE_TYPE = {"cover": "cover", "text": "bodymatter", "toc": "toc"}` (lines 40-43)
- `DEFAULT_LABEL_OF_GUIDE_TYPE = {"cover": "Cover", "text": "Beginning", "toc": "Table of Contents"}` (lines 45-49)
- `TOC_PRIORITY_OF_GUIDE_TYPE = {"toc": 1, "text": 2, "cover": 3}` (lines 51-55)
- `PERIODICAL_NCX_CLASSES = {0: "section", 1: "article"}` (lines 57-60)
- `MANIFEST_ITEM_PROPERTIES = {"cover-image", "mathml", "nav", "remote-resources", "scripted", "svg", "switch"}` (line 62)
- `SPINE_ITEMREF_PROPERTIES` (lines 64-70) — large set of EPUB3 spine properties including rendition:*, facing-page-*, layout-blank
- `OPF_PROPERTIES = MANIFEST_ITEM_PROPERTIES | SPINE_ITEMREF_PROPERTIES` (line 72)

**Namespace URIs:**
- `XML_NS_URI`, `DC_NS_URI`, `OPF_NS_URI`, `NCX_NS_URI`, `XHTML_NS_URI`, `EPUB_NS_URI`, `IDX_NS_URI`, `MBP_NS_URI` (lines 75-83)
- `SVG_NS_URI`, `XLINK_NS_URI`, `MATHML_NS_URI` (lines 95-100)
- `XHTML_NAMESPACES`, `SVG_NAMESPACES`, `MATHML_NAMESPACES` (lines 85-107)
- `RESERVED_OPF_VALUE_PREFIXES` (lines 111-120) — maps prefix strings to URIs (a11y, dcterms, marc, media, onix, rendition, schema, xsd)

**Data Classes:**
- `OPFProperties` (lines 123-149) — wraps a set of opf_properties, with property getters/setters: `is_fxl`, `is_nav`, `is_cover_image`
- `BookPart(OPFProperties)` (lines 152-178) — fields: filename, part_index, html, opf_properties, linear, omit, idref, is_cover_page, nmdl_template_id; methods: head(), body()
- `ManifestEntry(OPFProperties)` (lines 181-187) — fields: filename, opf_properties, linear, external, id, reference_count
- `TocEntry` (lines 190-195) — fields: title, target, children, description, icon, anchor
- `GuideEntry` (lines 198-202) — fields: guide_type, title, target, anchor
- `PageMapEntry` (lines 205-210) — fields: label, target, anchor
- `OutputFile` (lines 213-216) — fields: binary_data, mimetype, height, width

### `EPUB_Output` class (lines 234-1388)

**Class-level file path constants:**
- `OEBPS_DIR = "OEBPS"` (line 238)
- `OPF_FILEPATH = "/content.opf"` (line 240)
- `NCX_FILEPATH = "/toc.ncx"` (line 241)
- `TEXT_FILEPATH = "/part%04d.xhtml"` (line 242)
- `SECTION_TEXT_FILEPATH = "/%s.xhtml"` (line 243)
- `COVER_FILEPATH = "/cover.xhtml"` (line 244)
- `NAV_FILEPATH = "/nav%s.xhtml"` (line 245)
- `FONT_FILEPATH = "/%s"` (line 246)
- `IMAGE_FILEPATH = "/%s"` (line 247)
- `PDF_FILEPATH = "/%s"` (line 248)
- `STYLES_CSS_FILEPATH = "/stylesheet.css"` (line 249)
- `RESET_CSS_FILEPATH = "/reset.css"` (line 250)
- `LAYOUT_CSS_FILEPATH = "/layout%04d.css"` (line 251)
- Conditional subdirectory variants (lines 253-262) when `PLACE_FILES_IN_SUBDIRS = True`

**`__init__`** (lines 263-321):
- Initializes: `epub2_desired`, `generate_epub2`, `force_cover`, `will_output`
- Data structures: `oebps_files = {}`, `book_parts = []`, `ncx_toc = []`, `manifest = []`, `manifest_files = {}`, `manifest_ids = set()`, `guide = []`, `css_files = set()`, `pagemap = []`
- Metadata fields: asin, book_id, title, title_pronunciation, authors, author_pronunciations, publisher, pubdate, description, subject, rights, language, source_language, target_language
- Feature flags: is_sample, is_dictionary, override_kindle_font, ncx_location, toc_ncx_id, orientation_lock, min_aspect_ratio, max_aspect_ratio, original_width, original_height, fixed_layout, region_magnification, virtual_panels, virtual_panels_allowed, illustrated_layout, html_cover, guided_view_native, remove_html_cover, scrolled_continuous, book_type
- Calls `set_book_type(None)`, `set_primary_writing_mode("horizontal-lr")`

**`set_book_type`** (lines 323-338): Sets is_children/is_comic/is_magazine/is_notebook based on book_type string.

**`set_primary_writing_mode`** (lines 340-351): Maps "horizontal-lr" → writing_mode="horizontal-tb", page_progression_direction="ltr"; "horizontal-rl" → "horizontal-tb"/"rtl"; "vertical-rl" → "vertical-tb"/"rtl".

**`manifest_resource`** (lines 353-374): Adds a ManifestEntry; generates unique idref via `fix_html_id` + `make_unique_name`; increments reference_count; optionally adds oebps_file.

**`reference_resource`** / **`unreference_resource`** (lines 376-393): Increment/decrement reference_count; removes entry when count reaches 0.

**`add_guide_entry`** (lines 395-401): Maps guide_type through STANDARD_GUIDE_TYPE, creates GuideEntry.

**`add_pagemap_entry`** (lines 403-404): Creates PageMapEntry.

**`add_oebps_file`** / **`remove_oebps_file`** (lines 406-411): Dict operations on self.oebps_files.

---

### `generate_epub` — lines 414-530

```python
def generate_epub(self):
    # 1. Determine UID (ASIN > book_id > UUID)
    if self.asin:
        self.uid = "urn:asin:" + self.asin
    elif self.book_id:
        self.uid = self.book_id
    else:
        self.uid = "urn:uuid:" + str(uuid.uuid4())

    # 2. Default authors/title
    if not self.authors:
        self.authors = ["Unknown"]
    if not self.title:
        self.title = "Unknown"
    if self.is_sample:
        self.title += " - Sample"

    # 3. Log format description
    desc = []  # dictionary/sample/fixed layout/illustrated layout/book_type
    if desc:
        log.info("Format is %s" % " ".join(desc))

    # 4. check_epub_version() — may downgrade to epub2
    self.check_epub_version()

    # 5. identify_cover() — marks cover book_part
    self.identify_cover()
    if self.remove_html_cover:
        self.do_remove_html_cover()

    # 6. Force cover page if needed
    if self.force_cover:
        self.add_generic_cover_page()

    # 7. Validate content exists
    if not self.book_parts:
        raise Exception("Book does not contain any content")

    # 8. Default TOC from guide if empty
    if len(self.ncx_toc) == 0:
        for g in sorted(self.guide, key=lambda g: TOC_PRIORITY_OF_GUIDE_TYPE.get(g.guide_type, 999)):
            self.ncx_toc.append(TocEntry(g.title, target=g.target))
            break
        else:
            self.ncx_toc.append(TocEntry("Content", target=self.book_parts[0].filename))

    # 9. Create EPUB3 nav document if needed
    if not self.generate_epub2:
        for book_part in self.book_parts:
            if book_part.is_nav:
                break
        else:
            self.create_epub3_nav()

    # 10. Compare fixed layout viewports for comics/children
    if self.fixed_layout and (self.original_height is None or self.original_width is None) and (self.is_comic or self.is_children):
        self.compare_fixed_layout_viewports()

    # 11. save_book_parts() — serialize all HTML, add to manifest
    self.save_book_parts()

    # 12. Create NCX if needed
    if self.ncx_location is None and (self.generate_epub2 or self.GENERATE_EPUB2_COMPATIBLE):
        self.create_ncx()

    # 13. Create OPF
    self.create_opf()

    # 14. Log version mismatch warning
    if self.generate_epub2 is not self.epub2_desired:
        log.warning(...)

    # 15. Return ZIP data
    return self.zip_epub()
```

**Step order**: UID → defaults → version check → cover → validate → TOC default → EPUB3 nav → viewport comparison → save book parts → NCX → OPF → zip

---

### `create_opf` — lines 836-1084

**Helper closures defined inside:**
- `add_metadata_meta_name_content(name, content)` — adds `<meta name="..." content="..."/>`
- `add_metadata_meta_property(prop, text)` — adds `<meta property="...">text</meta>`
- `add_metadata_meta_refines_property(refines, prop, text, scheme=None)` — adds `<meta refines="..." property="...">`
- `prefix(value)` — tracks used prefixes for package prefix attribute

**Step-by-step logic:**

1. **Package element** (lines 858-862): `version="2.0"` or `"3.0"`, `unique-identifier="bookid"`, nsmap with OPF, DC, opf (alt URI)

2. **Metadata section** (lines 864-985):
   - `<dc:identifier id="bookid">` = self.uid (with EPUB2 opf:scheme="uuid" if applicable)
   - `<dc:title>` = self.title (with EPUB3 refines alternate-script for title_pronunciation)
   - `<dc:creator>` for each author (with EPUB3 refines role="aut" marc:relators, alternate-script for pronunciations; EPUB2 opf:role="aut")
   - `<dc:language>` (defaults to "en")
   - `<dc:publisher>` if set
   - `<dc:date>` (with EPUB2 opf:event="publication"), truncated to first 10 chars
   - `<dc:description>`, `<dc:subject>`, `<dc:rights>` if set
   - EPUB3: `<meta property="dcterms:modified">` with UTC ISO timestamp
   - Fixed layout: `rendition:layout=pre-paginated` (EPUB3), `fixed-layout=true` (meta), `original-resolution=WxH` (meta)
   - Scrolled continuous: `rendition:flow=scrolled-continuous` (EPUB3)
   - Book type children/comic: `book-type` meta
   - Orientation lock: `rendition:orientation` (EPUB3), `orientation-lock` (meta)
   - Override kindle font: `Override-Kindle-Fonts` meta
   - Writing mode (non-horizontal-lr): `primary-writing-mode` meta
   - Region magnification: `RegionMagnification` meta
   - Illustrated layout: `amzn:kindle-illustrated` meta
   - HTML cover: `amzn:cover-as-html` meta
   - Guided view native: `amzn:guided-view-native` meta
   - Min/max aspect ratio: `amzn:min-aspect-ratio`, `amzn:max-aspect-ratio` meta
   - Dictionary: `<x-metadata>` with DictionaryInLanguage/DictionaryOutLanguage
   - Magazine: `<x-metadata>` with output content-type
   - Used prefixes: set on package element

3. **Manifest** (lines 987-1043):
   - **Ordering**: `sorted(self.manifest, key=lambda m: m.filename)` — alphabetical by filename
   - For each manifest entry:
     - Determine toc_idref if filename matches ncx_location or ends with .ncx
     - Determine mimetype: from oebps_files or mimetype_of_filename; EPUB2 alt mimetypes applied
     - Determine href: urlrelpath from OPF_FILEPATH
     - `<item href=... id=... media-type=.../>`
     - Cover image: `<meta name="cover" content="..."/>` (unless html_cover or comic)
     - Fixed layout: discard `rendition:layout-pre-paginated` from manifest items, add `rendition:layout-reflowable` to non-FXL items; track page-spread
     - EPUB3 properties: intersect with MANIFEST_ITEM_PROPERTIES, set as sorted string
     - Unknown properties check

4. **Spine** (lines 1045-1069):
   - **Ordering**: follows `self.manifest` order (same as sorted alphabetical)
   - toc attribute if EPUB2 or EPUB2-compatible
   - page-progression-direction if not "ltr" and not EPUB2
   - For each manifest entry with `linear is not None`:
     - `<itemref idref=...>`
     - Spine properties: intersect with SPINE_ITEMREF_PROPERTIES
     - Comic spread center: if tweak enabled + is_comic + has_page_spread + no existing properties → add rendition:page-spread-center
     - EPUB3: set properties as prefixed sorted string
     - `linear="no"` if linear is False

5. **Guide** (lines 1071-1079):
   - Only for EPUB2 or EPUB2-compatible
   - `<guide>` with `<reference type=... title=... href=...>` for each guide entry
   - href via urlrelpath from OPF_FILEPATH

6. **Finalize** (lines 1081-1084):
   - `cleanup_namespaces`
   - Serialize with `pretty_print=True, xml_declaration=True`
   - Replace ALT_OPF_NS_URI with real OPF_NS_URI in output
   - Add to oebps_files

---

### `container_xml` — lines 1086-1094

```python
def container_xml(self):
    NS_URI = "urn:oasis:names:tc:opendocument:xmlns:container"
    container = etree.Element(qname(NS_URI, "container"), nsmap={None: NS_URI}, attrib={"version": "1.0"})
    rootfiles = etree.SubElement(container, "rootfiles")
    etree.SubElement(rootfiles, "rootfile", attrib={
        "full-path": (self.OEBPS_DIR + self.OPF_FILEPATH),
        "media-type": "application/oebps-package+xml"})
    return etree.tostring(container, encoding="utf-8", pretty_print=True, xml_declaration=True)
```

Structure: `<container version="1.0"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`

---

### `create_ncx` — lines 1096-1203

```python
def create_ncx(self):
    doctype = (None if not (self.generate_epub2 or GENERATE_EPUB2_NCX_DOCTYPE) else
        '<!DOCTYPE ncx PUBLIC "-//NISO//DTD ncx 2005-1//EN" "http://www.daisy.org/z3986/2005/ncx-2005-1.dtd">')
    emit_playorder = doctype is not None

    # Build NCX structure:
    # <ncx version="2005-1">
    #   <head> — dtb:uid meta
    #   <docTitle> — book title
    #   <docAuthor> — each author
    #   <navMap> — recursive TOC via create_navmap()
    #   <pageList> (if pagemap entries exist) — with page targets

    # Play order: tracks URI → sequential number; first occurrence gets new number
    # Page list: value/type based on label (numeric=normal, roman=front, other=special)

    # Final: manifest_resource with NCX_FILEPATH, mimetype="application/x-dtbncx+xml"
```

**`get_next_playorder`** (lines 1205-1208): Maps URI to sequential play order number; first seen gets next number.

**`create_navmap`** (lines 1210-1243): Recursive. Each navPoint gets id="nav%d" (incrementing), optional magazine class (section/article), playOrder, navLabel text, content src, optional mbp:meta description, optional mbp:meta-img mastheadImage.

---

### `create_epub3_nav` — lines 1245-1284

```python
def create_epub3_nav(self):
    # Find unique filename for nav document
    # Create new book_part with is_nav=True
    # <nav epub:type="toc"> with <h1>"Table of contents"</h1> + recursive <ol> via create_nav_list()
    # If guide exists: <nav epub:type="landmarks"> with <h2>"Guide"</h2> + <ol>
    #   - Each guide entry mapped through EPUB3_VOCABULARY_OF_GUIDE_TYPE for epub:type
    # If pagemap exists: <nav epub:type="page-list"> with <ol>
    # hide_element() for non-magazine
```

**`hide_element`** (lines 1277-1280): Sets `hidden=""` attribute or `display:none` style.

**`create_nav_list`** (lines 1282-1294): Recursive `<ol>/<li>/<a>` structure from ncx_toc.

---

### `zip_epub` — lines 1286-1297

```python
def zip_epub(self):
    file = io.BytesIO()
    with zipfile.ZipFile(file, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        # 1. mimetype file — stored (not compressed), ASCII encoded
        zf.writestr("mimetype", "application/epub+zip".encode("ascii"), compress_type=zipfile.ZIP_STORED)
        # 2. META-INF/container.xml
        zf.writestr("META-INF/container.xml", self.container_xml())
        # 3. All oebps_files, sorted by filename, prefixed with OEBPS_DIR
        for filename, oebps_file in sorted(self.oebps_files.items()):
            zf.writestr(self.OEBPS_DIR + filename, oebps_file.binary_data)
    data = file.getvalue()
    file.close()
    return data
```

**Critical ZIP details:**
- mimetype is ZIP_STORED (not deflated), ASCII encoded
- container.xml at `META-INF/container.xml`
- All content files under `OEBPS/` prefix
- Content files sorted alphabetically by filename
- Returns bytes

---

### Other important functions in epub_output.py

**`save_book_parts`** (lines 685-834): Serializes each book part's HTML:
- Sets tag to HTML (xhtml namespace)
- Adds `<title>` to head if missing
- Consolidates HTML (merges adjacent same-tag elements with same attrs)
- Beautifies HTML (adds newlines)
- Detects SVG, MathML, remote-resources
- Detects amzn: prefixed epub:type → adds epub prefix declaration
- Cleanup namespaces
- Serializes with doctype (EPUB2) or without (EPUB3)
- Calls manifest_resource for each

**`check_epub_version`** (lines 654-683): Scans for EPUB3-only features; downgrades to EPUB2 if needed.

**`identify_cover`** (lines 533-560): Finds cover page from guide entries or cover-image manifest entry.

**`compare_fixed_layout_viewports`** (lines 599-652): Counts viewport sizes; picks most common; warns on conflicting aspect ratios.

---

## B2: Navigation Reporting — `yj_to_epub_navigation.py`

File: `yj_to_epub_navigation.py` (561 lines)

### Key Constants (lines 24-33)
- `KEEP_APPROX_PG_NUMS = False`
- `REPORT_DUPLICATE_PAGES = False`
- `PREVENT_DUPLICATE_PAGE_LABELS = False`
- `PREVENT_DUPLICATE_PAGE_TARGETS = False`
- `GUIDE_TYPE_OF_LANDMARK_TYPE = {"$233": "cover", "$396": "text", "$269": "text", "$212": "toc"}`

### `KFX_EPUB_Navigation` class (line 35)

**`process_anchors`** (lines 40-67): Initializes anchor data structures:
- `self.anchor_uri = {}` — anchor_name → URI string
- `self.anchor_elem = {}` — anchor_name → lxml element
- `self.anchor_id = {}` — anchor_name → HTML id
- `self.anchor_ids = set()` — all used HTML ids
- `self.position_anchors = {}` — eid → {offset: [anchor_names]}
- `self.anchor_positions = {}` — anchor_name → set of (eid, offset) tuples
- `self.used_anchors = set()` — anchors referenced via get_anchor_uri
- `self.immovable_anchors = set()`
- `self.page_anchor_id_label = {}`
- `self.anchor_heading_level = {}`
- Processes `$266` fragment entries (anchors); registers URIs (`$186`) or positions (`$183`)

**`process_navigation`** (lines 69-116): Processes `$390` (section_navigation), `$389` (book_navigations), `$391` (nav_container):
- Matches book navigations to reading orders
- Processes nav containers via `process_nav_container`

**`process_nav_container`** (lines 118-197): Handles nav types: `$212` (TOC), `$236` (landmarks), `$237` (page-list), `$213`, `$214`, `$798` (headings):
- TOC types → `process_nav_unit`
- Landmarks → register anchors + add_guide_entry
- Page list → register anchors + add_pagemap_entry

**`process_nav_unit`** (lines 199-316): Recursive TOC building:
- Gets label/icon/description from representation
- Processes nested `$247` and `$248` (entry sets with orientation filtering)
- Creates TocEntry with anchor
- Heading level tracking for `$798` type

**`register_anchor`** (lines 318-351):
```python
def register_anchor(self, anchor_name, position, heading_level=None):
    # Adds to anchor_positions (multi-map: anchor_name → set of positions)
    # Adds to position_anchors (nested map: eid → offset → [anchor_names])
    # Optionally stores heading_level in anchor_heading_level
    # Returns get_anchor_id(first anchor name at that position)
```

---

### `report_missing_positions` — lines 353-361

```python
def report_missing_positions(self):
    if self.position_anchors:
        pos = []
        for id in self.position_anchors:
            for offset in self.position_anchors[id]:
                pos.append("%s.%s" % (id, offset))

        log.error("Failed to locate %d referenced positions: %s" % (len(pos), ", ".join(truncate_list(sorted(pos)))))
```

**Data structure operated on:** `self.position_anchors` — dict mapping eid → dict mapping offset → list of anchor_names. Any remaining entries in this structure after processing represent positions that were registered but never located (never consumed by `process_position`).

**Returns:** Nothing (void). Logs error with count and list of missing position strings.

**Dependencies:** `truncate_list` from utilities, `log` from message_logging.

---

### `report_duplicate_anchors` — lines 431-440

```python
def report_duplicate_anchors(self):
    for anchor_name, positions in self.anchor_positions.items():
        if (anchor_name in self.used_anchors) and (len(positions) > 1):
            log.error("Anchor %s has multiple positions: %s" % (
                    anchor_name, ", ".join([self.position_str(p) for p in sorted(positions)])))
```

**Data structure operated on:** `self.anchor_positions` — dict mapping anchor_name → set of (eid, offset) tuples. Also checks `self.used_anchors` set (populated by `get_anchor_uri`).

**Logic:** Only reports anchors that were actually used (in `self.used_anchors`) AND have multiple positions mapped to them.

**Returns:** Nothing (void). Logs error for each duplicate.

**Dependencies:** `self.position_str(p)` — formats as `"%s.%d" % position`.

---

### `fixup_anchors_and_hrefs` — lines 452-542

Resolves all anchor references to URIs:
1. Maps each anchor_elem to book_part filename + id → sets anchor_uri
2. Moves IDs from non-visible-first elements to parent
3. Converts `anchor:` URIs in `<a href>` to relative paths
4. Resolves guide entries, pagemap entries, and TOC entries

---

## B3: Illustrated Layout — `yj_to_epub_illustrated_layout.py`

File: `yj_to_epub_illustrated_layout.py` (426 lines)

### Key Constants (lines 13-17)
- `ADD_FINAL_CONTENT = True`
- `EMIT_EMPTY_CONDITIONS = False`
- `EMIT_PAGE_TEMPLATES = False` — controls whether to emit CSS @-amzn-page-element or inline content

### `KFX_EPUB_Illustrated_Layout` class (line 19)

**`CONDITION_OPERATOR_NAMES`** (lines 20-24):
```python
CONDITION_OPERATOR_NAMES = {
    "$294": "anchor-id",
    "$299": "range-id.le",
    "$298": "range-id.lt",
}
```

**`__init__`** (lines 26-27): `self.has_conditional_content = False`

---

### `fixup_illustrated_layout_anchors` — lines 29-128

```python
def fixup_illustrated_layout_anchors(self):
    if not self.has_conditional_content:
        return

    for book_part in self.book_parts:
        body = book_part.body()
        anchor_ids = []
        range_end_ids = []

        # PHASE 1: Find all -kfx-amzn-condition styles
        for e in body.findall("div"):
            if "style" in e.attrib:
                style = self.get_style(e)
                if "-kfx-amzn-condition" in style:
                    oper, anchor = style["-kfx-amzn-condition"].split()
                    url = self.get_anchor_uri(anchor)
                    purl = urllib.parse.urlparse(url)
                    id = purl.fragment

                    if purl.path == book_part.filename and id:
                        # Rewrite condition to use just the fragment id
                        style["-kfx-amzn-condition"] = "%s %s" % (oper.partition(".")[0], id)
                        self.set_style(e, style)

                        if oper == "anchor-id":
                            anchor_ids.append(id)
                        else:
                            range_end_ids.append((id, e, oper, anchor))
                    else:
                        log.error("-kfx-amzn-condition anchor %s in file %s links to %s" % (anchor, book_part.filename, url))

        # PHASE 2 (only if EMIT_PAGE_TEMPLATES): Create master range divs
        if EMIT_PAGE_TEMPLATES:
            start_idx = 0
            ranges = []

            for id, cond_elem, oper, anchor in range_end_ids:
                # Find the element with the target id, walk up to body top-level
                # Calculate end_idx (body child index)
                # range-id.lt adjustment: if no visible elements before, end_idx -= 1
                # Validate characters_after <= 1
                ranges.append((start_idx, end_idx, id, cond_elem, oper, anchor))
                start_idx = end_idx + 1

            final_range = True
            master_roots = []

            # Process ranges in reverse order
            for range_idx, (start_idx, end_idx, id, cond_elem, oper, anchor) in reversed(list(enumerate(ranges))):
                # Rename condition to use amzn_master_range_N id
                range_id = "amzn_master_range_%d" % range_idx
                self.add_style(cond_elem, {"-kfx-amzn-condition": "%s %s" % (oper.partition(".")[0], range_id)}, replace=True)

                # Wrap body children [start_idx..end_idx] in range_div
                range_div = etree.Element("div")
                range_div.set("id", range_id)

                if end_idx < start_idx:
                    # Empty range: remove condition element
                    cond_elem.getparent().remove(cond_elem)
                else:
                    # Move children into range_div
                    for i in range(end_idx - start_idx + 1):
                        e = body[start_idx]
                        body.remove(e)
                        range_div.append(e)

                    body.insert(start_idx, range_div)
                    master_roots.append(range_div)

                    # Add final content div with zero-width non-joiner
                    if final_range and EMIT_PAGE_TEMPLATES and ADD_FINAL_CONTENT:
                        temp_div = etree.Element("div")
                        temp_div.text = "\u200c"
                        self.add_style(temp_div, {
                            "margin-top": "0", "margin-bottom": "0",
                            "margin-left": "0", "margin-right": "0",
                            "padding-top": "0", "padding-bottom": "0",
                            "padding-left": "0", "padding-right": "0"})
                        temp_div.set("id", "extra-content-to-prevent-kpr-failure")
                        body.insert(start_idx + 1, temp_div)
                        final_range = False

            # Validate anchor_ids are within master ranges
            for id in anchor_ids:
                e = find_by_id(body, id)
                for root in master_roots:
                    if is_in_tree(root, e):
                        break
                else:
                    log.error("Conditional anchor id %s not within a master range in %s" % (id, book_part.filename))
```

**Key -kfx-amzn-condition handling:**
1. Phase 1: Rewrites conditions from anchor names to HTML fragment IDs
2. Phase 2 (EMIT_PAGE_TEMPLATES only): Creates master range wrapper divs (`amzn_master_range_N`), wrapping body children into conditional ranges
3. Validates anchor IDs are within master ranges
4. Operators: `anchor-id` (point condition), `range-id.le` (range inclusive), `range-id.lt` (range exclusive)

---

### `create_conditional_page_templates` — lines 130-419

This is a very large function (~290 lines). Key logic:

```python
def create_conditional_page_templates(self):
    if not self.has_conditional_content:
        return

    for book_part in self.book_parts:
        css_lines = []
        body = book_part.body()

        for template_elem in body.findall("div"):
            if "style" in template_elem.attrib:
                template_style = self.get_style(template_elem)
                amzn_condition = template_style.pop("-kfx-amzn-condition", None)

                if amzn_condition:
                    # Parse condition: oper + target_id
                    cond_oper, target_id = amzn_condition.split()
                    template_style.pop("-kfx-style-name", None)
                    template_style.pop("-kfx-attrib-epub-type", None)

                    # Handle 100% height/width
                    # Partition into base_style and check for extra styles
                    # Process template children:
                    #   - Single-child div containers: unwrap to get inner element
                    #   - img/video/div with background-color: handle positioning, float, collision
                    #   - epub type handling: amzn:full-page, amzn:kindle-illustrated, amzn:decorative, amzn:non-decorative
                    #   - EMIT_PAGE_TEMPLATES=true: generate @-amzn-page-element CSS rules
                    #   - EMIT_PAGE_TEMPLATES=false: inline content, remove decorative elements, keep float shapes
                    #   - Final div with page-align=none: move id to body start, remove element

                    # Locate target element by id
                    target = find_by_id(body, target_id)

                    # Based on mode:
                    # inline_content: wrap in span, insert as first child of target's block parent
                    # !EMIT_PAGE_TEMPLATES: keep as div, insert as first child of target's block parent
                    # EMIT_PAGE_TEMPLATES: generate CSS @-amzn-master-page { @-amzn-condition(...) { ... } }

        # If css_lines generated, create layout CSS file
        if css_lines:
            css_file = self.LAYOUT_CSS_FILEPATH % book_part.part_index
            self.link_css_file(book_part, css_file, css_type="text/amzn+css")
            self.manifest_resource(css_file, data="\n".join(css_lines).encode("utf-8"), mimetype="text/amzn+css")
```

**CSS output format (when EMIT_PAGE_TEMPLATES=true):**
```
@-amzn-master-page {
  @-amzn-condition(anchor-id target_id) {
    base_style;
    @-amzn-page-element element_id {
      element_style
    }
  }
}
```

---

### `fixup_illustrated_layout_region_magnification` and `fixup_illustrated_layout_condition_classes`

**Not found in this file.** These functions do not exist in `yj_to_epub_illustrated_layout.py`. The class only has:
1. `__init__` (line 26)
2. `fixup_illustrated_layout_anchors` (line 29)
3. `create_conditional_page_templates` (line 130)

The region magnification and condition class logic may be handled in other files (e.g., `yj_to_epub_properties.py`).

---

### Module-level helper functions (lines 394-426)

```python
def find_by_id(root, search_id, required=True):
    # XPath search: .//*[@id="search_id"]
    # Returns first match or None (if not required) or raises Exception

def positions_in_tree(root):
    # Counts text characters + 1 for each img/video in tree

def is_in_tree(root, elem):
    # Walks up parent chain to check if elem is descendant of root
```

---

## B4: Resource Variant Handling — `yj_to_epub_resources.py`

File: `yj_to_epub_resources.py` (365 lines)

### Key Constants (lines 20-23)
- `USE_HIGHEST_RESOLUTION_IMAGE_VARIANT = True`
- `FIX_PDF = True`
- `FIX_JPEG_XR = True`
- `REPORT_PDF_MARGINS = False`

### `Obj` helper class (lines 25-27): Simple attribute holder using `__dict__.update(kwargs)`.

### `KFX_EPUB_Resources` class (line 27)

**`__init__`** (lines 28-33):
- `self.resource_cache = {}`
- `self.used_raw_media = set()`
- `self.save_resources = True`
- `self.location_filenames = {}`
- `self.reported_pdf_errors = set()`

---

### `get_external_resource` — lines 35-183

**Signature:** `def get_external_resource(self, resource_name, ignore_variants=False)`

**Full logic:**

```python
def get_external_resource(self, resource_name, ignore_variants=False):
    # 1. Check cache
    resource_obj = self.resource_cache.get(resource_name)
    if resource_obj is not None:
        return resource_obj

    # 2. Get fragment $164
    resource = self.get_fragment(ftype="$164", fid=resource_name)

    # 3. Validate internal name
    int_resource_name = resource.pop("$175", "")
    if int_resource_name != resource_name:
        log.error("Name of resource %s is incorrect: %s" % (resource_name, int_resource_name))

    # 4. Extract dimensions
    resource_format = resource.pop("$161", None)
    fixed_height = resource.pop("$67", None)
    fixed_width = resource.pop("$66", None)
    resource_height = resource.pop("$423", None) or fixed_height
    resource_width = resource.pop("$422", None) or fixed_width

    # 5. Handle tiles ($636) vs single image ($165)
    if "$636" in resource:
        yj_tiles = resource.pop("$636")
        tile_height = resource.pop("$638")
        tile_width = resource.pop("$637")
        tile_padding = resource.pop("$797", 0)
        location = yj_tiles[0][0].partition("-tile")[0]
        # Combine tiles
        tiles_raw_media = [self.locate_raw_media(tile) for row in yj_tiles for tile in row]
        raw_media, resource_format = combine_image_tiles(...)
    else:
        location = resource.pop("$165")
        search_path = resource.pop("$166", location)
        raw_media = self.locate_raw_media(location)

    # 6. Early return for ignore_variants with missing raw_media
    if ignore_variants and raw_media is None:
        return None

    # 7. Determine extension
    if resource_format in SYMBOL_FORMATS:
        extension = "." + SYMBOL_FORMATS[resource_format]
    else:
        if resource_format is not None:
            log.error("...")
        extension = ".bin"

    # 8. Handle MIME type
    mime = resource.pop("$162", None)
    if mime in EXTS_OF_MIMETYPE:
        if extension in (".pobject", ".bin"):
            if mime == "figure":
                extension = image_file_ext(raw_media)
            else:
                extension = EXTS_OF_MIMETYPE[mime][0]
    elif mime is not None:
        log.error("...")

    # 9. Determine filename from location
    location_fn = location
    location_fn = resource.pop("yj.conversion.source_resource_filename", location_fn)
    location_fn = resource.pop("yj.authoring.source_file_name", location_fn)
    if (extension in (".pobject", ".bin")) and "." in location_fn:
        extension = "." + location_fn.rpartition(".")[2]
    if not location_fn.endswith(extension):
        location_fn = location_fn.partition(".")[0] + extension

    # 10. Pop remaining fields
    resource.pop("$597", None)  # and many more...
    referred_resources = resource.pop("$167", [])

    # 11. Process referred resource ($214)
    if "$214" in resource:
        self.process_external_resource(resource.pop("$214"), save=False)

    # 12. JPEG-XR conversion
    if FIX_JPEG_XR and resource_format == "$548" and raw_media is not None:
        raw_media, resource_format = convert_jxr_to_jpeg_or_png(raw_media, location_fn)
        extension = "." + SYMBOL_FORMATS[resource_format]
        location_fn = location_fn.rpartition(".")[0] + extension

    # 13. PDF handling ($565 format)
    suffix = ""
    if resource_format == "$565" and raw_media is not None:
        page_num = resource.pop("$564") + 1  # 1-based
        suffix = "-page%d" % page_num
        # Extract margins...
        if FIX_PDF:
            img_data, img_fmt = convert_pdf_page_to_image(...)
            raw_media = img_data
            resource_format = img_fmt
            # Update extension and location_fn

    # 14. Generate filename
    filename = self.resource_location_filename(location_fn, suffix, self.IMAGE_FILEPATH)

    # 15. **VARIANT SELECTION** — key logic
    if not ignore_variants:
        for rr in resource.pop("$635", []):
            variant = self.get_external_resource(rr, ignore_variants=True)
            if (USE_HIGHEST_RESOLUTION_IMAGE_VARIANT and variant is not None and
                    variant.width > resource_width and variant.height > resource_height):
                # Replace with higher-resolution variant
                raw_media, filename, resource_width, resource_height = (
                    variant.raw_media, variant.filename, variant.width, variant.height)

    # 16. Handle page fragment for PDF
    if "$564" in resource:
        filename += "#page=%d" % (resource.pop("$564") + 1)

    # 17. Validate empty and cache result
    self.check_empty(resource, "resource %s" % resource_name)
    resource_obj = self.resource_cache[resource_name] = Obj(
        raw_media=raw_media, filename=filename, extension=extension,
        format=resource_format, mime=mime, location=location,
        width=resource_width, height=resource_height,
        referred_resources=referred_resources, manifest_entry=None)
    return resource_obj
```

**Variant selection logic (lines 170-179):**
- Iterates through `$635` (variants list) in the resource
- For each variant, recursively calls `get_external_resource(rr, ignore_variants=True)` to avoid infinite recursion
- If `USE_HIGHEST_RESOLUTION_IMAGE_VARIANT` is True AND variant has both width > resource_width AND height > resource_height: replaces raw_media, filename, dimensions
- This means variant must be strictly larger in BOTH dimensions to be selected

---

### `process_external_resource` — lines 185-233

**Signature:** `def process_external_resource(self, resource_name, save=True, process_referred=False, save_referred=False, is_plugin=False, is_referred=False)`

```python
def process_external_resource(self, resource_name, save=True, process_referred=False, save_referred=False,
                              is_plugin=False, is_referred=False):
    resource_obj = self.get_external_resource(resource_name)

    if save and self.save_resources and resource_obj.raw_media is not None:
        if resource_obj.manifest_entry is None:
            # Determine filename: root_filename(location) if is_referred, else resource_obj.filename
            filename = root_filename(resource_obj.location) if is_referred else resource_obj.filename
            filename, fragment_sep, fragment = filename.partition("#")
            base_filename = filename

            # Handle duplicates: check if same data already exists in oebps_files
            cnt = 0
            while filename in self.oebps_files:
                if self.oebps_files[filename].binary_data == resource_obj.raw_media:
                    manifest_entry = self.manifest_files.get(filename, None)
                    if manifest_entry is not None:
                        resource_obj.manifest_entry = manifest_entry
                        self.reference_resource(manifest_entry)
                        break
                # Generate unique filename with _N suffix
                fn, ext = posixpath.splitext(base_filename)
                filename = "%s_%d%s" % (fn, cnt, ext)
                cnt += 1
            else:
                # New file: add to manifest
                resource_obj.manifest_entry = self.manifest_resource(
                    filename, data=resource_obj.raw_media,
                    height=resource_obj.height, width=resource_obj.width,
                    mimetype=resource_obj.mime if is_referred else None)

            resource_obj.filename = filename + fragment_sep + fragment
            resource_obj.is_saved = True
        else:
            self.reference_resource(resource_obj.manifest_entry)

    # Process referred resources
    if process_referred or save_referred:
        for rr in resource_obj.referred_resources:
            self.process_external_resource(rr, save=save_referred, is_referred=True)

    # Validation
    if is_referred:
        pass
    elif is_plugin and resource_obj.format not in ["$287", "$284"]:
        log.error("Unexpected plugin resource format...")
    elif (not is_plugin) and resource_obj.extension == ".pobject":
        log.error("Unexpected non-plugin resource format...")

    return resource_obj
```

**Key difference from simple implementation:** Deduplication by comparing binary_data; is_referred uses root_filename; manifest mimetype only set for referred resources.

---

### `locate_raw_media` — lines 235-245

```python
def locate_raw_media(self, location, report_missing=True):
    try:
        raw_media = self.book_data["$417"][location]
        self.used_raw_media.add(location)
    except Exception:
        if report_missing:
            log.error("Missing bcRawMedia %s" % location)
        raw_media = None
    return raw_media
```

### `resource_location_filename` — lines 247-283

Generates safe, unique filenames from resource locations. Handles path sanitization, symbol unique parts, deduplication with case-insensitive checking.

### `process_fonts` — lines 285-329

Processes font resources from `$262` and `$418` fragments. Handles font deduplication, format detection, and CSS @font-face generation.

### `uri_reference` — lines 331-364

Resolves URIs with schemes: `kfx:` → process_external_resource, `navto:`/`navt:` → anchor lookup, `http:`/`https:` → external URL, `mailto:` → passthrough.

---

## B5: Metadata Getter/Query Functions — `yj_metadata.py`

File: `yj_metadata.py` (831 lines)

### `YJ_Metadata` class (lines 25-32)

```python
class YJ_Metadata(object):
    def __init__(self, author_sort_fn=None, replace_existing_authors_with_sort=False):
        self.authors = []
        self.author_sort_fn = author_sort_name if author_sort_fn is None else author_sort_fn
        self.replace_existing_authors_with_sort = replace_existing_authors_with_sort
        self.title = self.cde_content_type = self.asin = self.cover_image_data = self.description = None
        self.issue_date = self.language = self.publisher = self.book_id = self.features = self.asset_id = None
```

**Fields:** authors, author_sort_fn, replace_existing_authors_with_sort, title, cde_content_type, asin, cover_image_data, description, issue_date, language, publisher, book_id, features, asset_id

---

### `BookMetadata` class (line 34)

All getter/query functions are methods of this class.

---

### `get_yj_metadata_from_book` — lines 35-115

**Signature:** `def get_yj_metadata_from_book(self)`

Extracts metadata from two fragment sources:
1. `$490` (book_metadata) — iterates `$491` containers, finds `kindle_title_metadata` category, extracts key-value pairs from `$258`
2. `$258` (metadata) — fallback, uses METADATA_NAMES symbol mapping

Extracted fields: author, title, cde_content_type, ASIN, description, issue_date, language, publisher, book_id, asset_id

Author parsing: handles `&`, `and`, comma-separated formats. Calls `unsort_author_name` to convert "Last, First" back to "First Last".

Returns YJ_Metadata object.

---

### `set_yj_metadata_to_book` — lines 117-249

**Signature:** `def set_yj_metadata_to_book(self, yj_metadata)`

Writes metadata back to both `$490` and `$258` fragments. Handles author sorting, language fixups, cover image updates, and conditional field updates (None means don't change, False means delete).

---

### `has_metadata` — lines 251-253

```python
def has_metadata(self):
    return (self.fragments.get(YJFragmentKey(ftype="$490")) is not None or
            self.fragments.get(YJFragmentKey(ftype="$258")) is not None)
```

### `has_cover_data` — lines 255-256

```python
def has_cover_data(self):
    return self.get_cover_image_data() is not None
```

---

### `get_asset_id` — lines 258-259

```python
def get_asset_id(self):
    return self.get_metadata_value("asset_id")
```

---

### `cde_type` — lines 262-267

```python
@property
def cde_type(self):
    if not hasattr(self, "_cached_cde_type"):
        self._cached_cde_type = self.get_metadata_value("cde_content_type")
    return self._cached_cde_type
```

**Cached property.** Returns the raw cde_content_type string (e.g., "MAGZ", "EBSP", "EBOK").

---

### `is_magazine` — lines 269-271

```python
@property
def is_magazine(self):
    return self.cde_type == "MAGZ"
```

---

### `is_sample` — lines 273-275

```python
@property
def is_sample(self):
    return self.cde_type == "EBSP"
```

---

### `is_fixed_layout` — lines 277-284

```python
@property
def is_fixed_layout(self):
    if not hasattr(self, "_cached_is_fixed_layout"):
        self._cached_is_fixed_layout = (
            self.is_scribe_notebook or
            self.get_metadata_value("yj_fixed_layout", "kindle_capability_metadata") is not None)
    return self._cached_is_fixed_layout
```

**Cached property.** Checks if scribe notebook OR if `yj_fixed_layout` exists in `kindle_capability_metadata` category.

---

### `is_image_based_fixed_layout` — lines 286-296

```python
@property
def is_image_based_fixed_layout(self):
    if not hasattr(self, "_cached_is_image_based_fixed_layout"):
        try:
            self.get_ordered_image_resources()
        except Exception:
            self._cached_is_image_based_fixed_layout = False
        else:
            self._cached_is_image_based_fixed_layout = True
    return self._cached_is_image_based_fixed_layout
```

**Cached property.** Tests if `get_ordered_image_resources()` succeeds without exception.

---

### `is_print_replica` — lines 298-305

```python
@property
def is_print_replica(self):
    if not hasattr(self, "_cached_is_print_replica"):
        yj_textbook = self.get_metadata_value("yj_textbook", category="kindle_capability_metadata")
        yj_fixed_layout = self.get_metadata_value("yj_fixed_layout", category="kindle_capability_metadata")
        self._cached_is_print_replica = yj_fixed_layout == 2 or (yj_textbook is not None and yj_fixed_layout != 3)
    return self._cached_is_print_replica
```

**Cached property.** True if yj_fixed_layout==2 (PDF print replica) OR (yj_textbook exists AND yj_fixed_layout != 3).

---

### `is_pdf_backed_fixed_layout` — lines 307-312

```python
@property
def is_pdf_backed_fixed_layout(self):
    if not hasattr(self, "_cached_is_pdf_backed_fixed_layout"):
        self._cached_is_pdf_backed_fixed_layout = self.get_metadata_value("yj_fixed_layout", "kindle_capability_metadata") == 3
    return self._cached_is_pdf_backed_fixed_layout
```

---

### `is_illustrated_layout` — lines 314-319

```python
@property
def is_illustrated_layout(self):
    if not hasattr(self, "_cached_is_illustrated_layout"):
        self._cached_is_illustrated_layout = self.get_feature_value("yj.illustrated_layout") is not None
    return self._cached_is_illustrated_layout
```

---

### `has_illustrated_layout_conditional_page_template` — lines 321-336

```python
@property
def has_illustrated_layout_conditional_page_template(self):
    if not hasattr(self, "_cached_is_illustrated_layout_conditional_structure"):
        self._cached_is_illustrated_layout_conditional_structure = (
            self.get_feature_value("yj.conditional_structure") is not None or
            (self.get_feature_value("reflow-style", default=0) == 5 and not self.is_magazine) or
            self.has_illustrated_layout_page_template_condition())
    return self._cached_is_illustrated_layout_conditional_structure
```

**Three conditions (OR):**
1. `yj.conditional_structure` feature exists
2. `reflow-style` feature has value 5 AND not magazine
3. `has_illustrated_layout_page_template_condition()` returns True

---

### `is_kfx_v1` — lines 338-347

```python
@property
def is_kfx_v1(self):
    if not hasattr(self, "_cached_is_kfx_v1"):
        fragment = self.fragments.get("$270", first=True)
        self._cached_is_kfx_v1 = fragment.value.get("version", 0) == 1 if fragment is not None else False
    return self._cached_is_kfx_v1
```

Checks `$270` fragment's `version` field == 1.

---

### `has_pdf_resource` — lines 349-360

```python
@property
def has_pdf_resource(self):
    if not hasattr(self, "_cached_has_pdf_resource"):
        for fragment in self.fragments.get_all("$164"):
            if fragment.value.get("$161") == "$565":
                self._cached_has_pdf_resource = True
                break
        else:
            self._cached_has_pdf_resource = False
    return self._cached_has_pdf_resource
```

Checks if any `$164` (resource) fragment has format `$565` (PDF).

---

### `get_metadata_value` — lines 362-387

**Signature:** `def get_metadata_value(self, name, category="kindle_title_metadata", default=None)`

```python
def get_metadata_value(self, name, category="kindle_title_metadata", default=None):
    try:
        # 1. Search book_metadata ($490) for matching category and key name
        fragment = self.fragments.get("$490")
        if fragment is not None:
            for cm in fragment.value["$491"]:
                if cm["$495"] == category:
                    for kv in cm["$258"]:
                        if kv["$492"] == name:
                            return kv["$307"]

        # 2. Fallback: search metadata ($258) using symbol mapping
        metadata_symbol = METADATA_SYMBOLS.get(name)
        if metadata_symbol is not None:
            fragment = self.fragments.get("$258")
            if fragment is not None and metadata_symbol in fragment.value:
                return fragment.value[metadata_symbol]
    except Exception:
        pass

    return default
```

**Two-tier lookup:**
1. `$490` fragment: search `$491` list for matching `category` in `$495`, then find `name` in `$258` key-value pairs (`$492`=key, `$307`=value)
2. `$258` fragment: use METADATA_SYMBOLS mapping (name → YJ symbol like `$153` for title)

---

### `get_feature_value` — lines 389-417

**Signature:** `def get_feature_value(self, feature, namespace="com.amazon.yjconversion", default=None)`

```python
def get_feature_value(self, feature, namespace="com.amazon.yjconversion", default=None):
    if namespace == "format_capabilities":
        # Search $593 fragment for matching feature name
        fragment = self.fragments.get("$593", first=True)
        if fragment is not None:
            for fc in fragment.value:
                if fc.get("$492", "") == feature:
                    return fc.get("version", "")
    else:
        # Search $585 fragment for matching namespace + feature
        fragment = self.fragments.get("$585", first=True)
        if fragment is not None:
            for cf in fragment.value.get("$590", []):
                if cf.get("$586", "") == namespace and cf.get("$492", "") == feature:
                    vi = cf.get("$589", {}).get("version", {})
                    major_version = vi.get("$587", 0)
                    minor_version = vi.get("$588", 0)
                    return major_version if minor_version == 0 else (major_version, minor_version)

    return default
```

**Two namespace paths:**
1. `format_capabilities`: searches `$593` fragment for `feature` match, returns `version` string
2. `com.amazon.yjconversion` (default): searches `$585` fragment's `$590` list for matching namespace+feature, returns major version or (major, minor) tuple

---

### `get_generators` — lines 389-399

Returns set of (generator_name, package_version) tuples from `$270` fragments.

### `get_features` — lines 401-418

Returns set of (namespace, key, version) tuples from both `$593` and `$585` fragments.

### `get_page_count` — lines 420-432

Searches `$389` (book_navigation) for a `$237` (page-list) nav container and returns its child count.

---

### `report_features_and_metadata` — lines 434-534

**Signature:** `def report_features_and_metadata(self, unknown_only=False)`

```python
def report_features_and_metadata(self, unknown_only=False):
    # 1. Collect all features via get_features()
    # 2. Check each against is_known_feature(); log unknown
    # 3. Track min_kindle_version via kindle_feature_version()
    # 4. Log features list

    # 5. Collect metadata from generators, symbols, book_metadata, metadata, page count
    # 6. For each metadata entry, check against is_known_generator/is_known_metadata
    # 7. Special handling for cover_image (dimensions + format), dictionary_lookup, description
    # 8. Log metadata list
```

---

### `get_cover_image_data` — lines 536-554

**Signature:** `def get_cover_image_data(self)`

```python
def get_cover_image_data(self):
    cover_image_resource = self.get_metadata_value("cover_image")
    if not cover_image_resource:
        return None

    cover_resource = self.fragments.get(ftype="$164", fid=cover_image_resource)
    if cover_resource is None:
        return None

    cover_fmt = cover_resource.value["$161"]
    if ion_type(cover_fmt) is IonSymbol:
        cover_fmt = SYMBOL_FORMATS[cover_fmt]

    cover_raw_media = self.fragments.get(ftype="$417", fid=cover_resource.value["$165"])
    if cover_raw_media is None:
        return None

    return ("jpeg" if cover_fmt == "jpg" else cover_fmt, cover_raw_media.value.tobytes())
```

**Returns:** `None` or `(format_string, bytes_data)` tuple. Format is "jpeg" for jpg, otherwise the raw format string.

**Data path:** metadata_value("cover_image") → `$164` fragment → `$161` format + `$165` location → `$417` fragment → raw bytes.

---

### `fix_cover_image_data` — lines 556-578

**Signature:** `def fix_cover_image_data(self, cover_image_data)`

```python
def fix_cover_image_data(self, cover_image_data):
    fmt = cover_image_data[0]
    data = orig_data = cover_image_data[1]

    # Convert to JFIF JPEG if needed for Kindle lockscreen
    if fmt.lower() in ["jpg", "jpeg"] and not data.startswith(b"\xff\xd8\xff\xe0"):
        try:
            cover = Image.open(io.BytesIO(data))
            outfile = io.BytesIO()
            cover.save(outfile, "JPEG", quality="keep")
            cover.close()
            data = outfile.getvalue()
        except Exception:
            data = orig_data

        if data.startswith(b"\xff\xd8\xff\xe0"):
            log.info("Changed cover image from %s to JPEG/JFIF..." % jpeg_type(orig_data))
        else:
            log.error("Failed to change cover image...")
            data = orig_data

    return (fmt, data)
```

**Logic:** Ensures JPEG cover images start with JFIF marker (`\xff\xd8\xff\xe0`). Re-encodes if needed.

---

### `set_cover_image_data` — lines 580-646

**Signature:** `def set_cover_image_data(self, cover_image_data)`

Validates format (must be JPEG), validates existing cover resource, calls `check_cover_section_and_storyline`, `update_cover_section_and_storyline`, `update_image_resource_and_media`. Also handles thumbnail generation.

### `check_cover_section_and_storyline` — lines 648-791

**Signature:** `def check_cover_section_and_storyline(self, expected_resource=None, expected_orig_width=None, expected_orig_height=None, allow_pdf=False)`

Validates the cover section structure: checks page templates, storyline content, layout types ($326 vs $323). Returns (resource_name, cover_eid).

### `update_cover_section_and_storyline` — lines 793-820

**Signature:** `def update_cover_section_and_storyline(self, orig_width, orig_height, width, height)`

Updates cover section and storyline dimensions when cover image size changes. Processes `$56`/`$57` (width/height) and `$66`/`$67` (fixed_width/fixed_height) properties.

---

### Module-level functions (lines 822-831)

**`author_sort_name(author)`** (lines 823-839): Converts "First Last" to "Last, First". Handles suffixes (Jr., III, etc.), comma-separated names.

**`unsort_author_name(author)`** (lines 841-845): Reverses "Last, First" back to "First Last".

**`fix_language_for_kfx(language)`** (lines 847-853): Applies LANGUAGE_FIXUPS mapping, converts zh-tw → zh-hant.

---

## Cross-file Dependencies Summary

### B1 (epub_output.py) depends on:
- `resources.py`: `EPUB2_ALT_MIMETYPES`, `MIMETYPE_OF_EXT`
- `utilities.py`: `make_unique_name`, `urlrelpath`
- `message_logging.py`: `log`

### B2 (yj_to_epub_navigation.py) depends on:
- `epub_output.py`: `TocEntry`
- `utilities.py`: `make_unique_name`, `truncate_list`, `urlrelpath`
- `yj_position_location.py`: `DEBUG_PAGES`
- `yj_structure.py`: `APPROXIMATE_PAGE_LIST`

### B3 (yj_to_epub_illustrated_layout.py) depends on:
- `utilities.py`: `get_url_filename`, `urlabspath`
- `yj_to_epub_navigation.py`: `visible_elements_before`

### B4 (yj_to_epub_resources.py) depends on:
- `resources.py`: `EXTS_OF_MIMETYPE`, `combine_image_tiles`, `convert_jxr_to_jpeg_or_png`, `convert_pdf_page_to_image`, `font_file_ext`, `image_file_ext`, `RESOURCE_TYPE_OF_EXT`, `SYMBOL_FORMATS`
- `utilities.py`: `root_filename`, `urlrelpath`

### B5 (yj_metadata.py) depends on:
- `ion.py`: `IS`, `IonBLOB`, `IonStruct`, `IonSymbol`, `ion_type`, `unannotated`
- `original_source_epub.py`: `LANGUAGE_FIXUPS`
- `resources.py`: `FORMAT_SYMBOLS`, `image_size`, `jpeg_type`, `SYMBOL_FORMATS`
- `utilities.py`: `disable_debug_log`, `list_symbols`, `list_symbols_unsorted`, `natural_sort_key`, `quote_name`
- `yj_container.py`: `YJFragment`, `YJFragmentKey`
- `yj_structure.py`: `METADATA_NAMES`, `METADATA_SYMBOLS`
- `yj_versions.py`: `kindle_feature_version`, `is_known_feature`, `is_known_generator`, `is_known_metadata`, `PACKAGE_VERSION_PLACEHOLDERS`, `UNSUPPORTED`
