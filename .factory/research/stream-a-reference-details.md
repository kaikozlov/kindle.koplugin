# Stream A Reference Details — Python Code Locations

> Source: `/Users/kai/dev/projects/kindle.koplugin/REFERENCE/Calibre_KFX_Input/kfxlib/`
> Generated for Go porting work (Stream A)

---

## A1: organizeFragments / replace_ion_data parity

### File: `yj_to_epub.py`

#### `organize_fragments_by_type` (lines 181–215)

```python
def organize_fragments_by_type(self, fragment_list):
```

**Signature**: `organize_fragments_by_type(self, fragment_list) -> dict`

**What it does** (step by step):
1. Initialize `font_count = 0`, `categorized_data = {}`, `last_container_id = None`
2. Iterate over each `fragment` in `fragment_list`:
   - `id = fragment.fid` — add `id` to `self.book_symbols`
   - **Special ID remapping**:
     - `ftype == "$270"` → `id = IonSymbol("{container_format}:{container_id}")` from fragment value keys `$161` and `$409`. Also updates `last_container_id`.
     - `ftype == "$593"` → `id = last_container_id` (reuse previous container ID)
     - `ftype == "$262"` → `id = IonSymbol("{id}-font-{font_count:03d}")`, increment font_count
     - `ftype == "$387"` → `id = IonSymbol("{id}:{fragment.value['$215']}")`
   - `dt = categorized_data.setdefault(fragment.ftype, {})`
   - If `id not in dt`: `dt[id] = self.replace_ion_data(fragment.value)`
   - Else: log error about duplicate fragment
3. **Singleton collapse** (lines 207-213): For each category in `categorized_data`, if it has exactly one entry and the key equals the category name, collapse: `categorized_data[category] = categorized_data[category][id]`
4. If `None` in ids, log error about mixed null/non-null ids
5. Return `categorized_data`

**Fragment type keys handled**: The function processes ALL fragment types from the input list. The types that get special ID treatment are: `$270`, `$593`, `$262`, `$387`. After organization, the following types are consumed in `__init__`:
- `$157` (kfx_styles), `$164` (external_resources), `$259` (storyline), `$260` (section), `$417` (raw_media)
- `$266` (anchors), `$391` (navigation containers), `$490`, `$538` (document_data), `$585`, `$608` (page_templates)
- `$609` (section_position_id_map), `$756` (ruby_content), `$389` (book_navigation), `$145` (content)
- Also popped but not used: `$270`, `$593`, `$ion_symbol_table`, `$419`, `$692`, `$550`, `$265`, `$264`, `$395`, `$609`, `$621`, `$597`, `$610`, `$611`, `$387`, `$267`

**`book_data` dict structure**: `dict[ftype_str -> dict[id_symbol -> fragment_value] | fragment_value]`
- If a ftype has only one entry where key==ftype, the value is unwrapped from the dict
- Example: `self.book_data["$157"]` is a dict of `{style_name -> IonStruct}`
- Example: `self.book_data["$538"]` might be a single IonStruct (singleton)

#### `replace_ion_data` (lines 270–291)

```python
def replace_ion_data(self, f):
```

**Signature**: `replace_ion_data(self, f) -> replaced_data`

**What it does**: Recursively walks Ion data and collects all `IonSymbol` values into `self.book_symbols`. Specifically:
1. If `IonAnnotation` → recurse into `f.value` (strip annotation)
2. If `IonList` → return `[self.replace_ion_data(fc) for fc in f]`
3. If `IonSExp` → return `IonSExp([self.replace_ion_data(fc) for fc in f])` (preserve SExp type)
4. If `IonStruct` → create new `IonStruct`, recurse into both keys and values: `newf[replace_ion_data(fk)] = replace_ion_data(fv)`
5. If `IonSymbol` → add to `self.book_symbols`, return as-is
6. Otherwise (IonString, IonInt, etc.) → return as-is

**Dependencies**: `ion.ion_type`, `ion.IonAnnotation`, `ion.IonList`, `ion.IonSExp`, `ion.IonStruct`, `ion.IonSymbol`

#### `determine_book_symbol_format` (lines 217–238)

```python
def determine_book_symbol_format(self):
```

- Classifies every symbol in `self.book_symbols` using `self.book.classify_symbol()`
- Counts each `SYM_TYPE` occurrence
- Determines quorum from dictionary+short+base64+original counts // 2
- Sets `self.book_symbol_format` based on which type reaches quorum
- Priority: SHORT > DICTIONARY > BASE64 > ORIGINAL

#### `FRAGMENT_NAME_SYMBOL` constant (lines 30–39)

```python
FRAGMENT_NAME_SYMBOL = {
    "$266": "$180",
    "$164": "$175",
    "$391": "$239",
    "$393": "$240",
    "$260": "$174",
    "$608": "$598",
    "$259": "$176",
    "$157": "$173",
}
```

#### `PROGRESS_FTYPES` constant (line 41)

```python
PROGRESS_FTYPES = {"$260", "$259", "$164"}
```

---

## A2: classify_symbol and symbol format wiring

### File: `yj_structure.py`

#### `classify_symbol` (lines 1042–1087)

```python
def classify_symbol(self, name):
```

**Signature**: `classify_symbol(self, name: str) -> str` (returns a SYM_TYPE constant)

**Logic** — returns the first match:

1. **SYM_TYPE.SHARED** — if `self.symtab.is_shared_symbol(IS(name))`

2. **SYM_TYPE.COMMON** — matches any of:
   - Exact name in set: `APPROXIMATE_PAGE_LIST`, `"crop_bleed"`, `"dictionary_rules"`, `"mkfx_id"`, `"page_list_entry"`, `"srl_created_by_stampler"`, `"yj.dictionary.text"`, `"note_template_collection"`
   - Starts with `KFX_COVER_RESOURCE` (= `"kfx_cover_image"`)
   - Regex: `^content_[0-9]+$`
   - Regex: `^eidbucket_[0-9]+$`
   - Regex: `^PAGE_LIST_[0-9]{10,}$`
   - Regex: `UUID_MATCH_RE`
   - Regex: `^yj\.(authoring|conversion|print|semantics)\.`
   - Regex: `^nmdl\.`

3. **SYM_TYPE.DICTIONARY** — matches:
   - Regex: `^G[0-9]+(-spm)?$`
   - Regex: `^yj\.dictionary\.`

4. **SYM_TYPE.ORIGINAL** — matches:
   - Regex: `^V_[0-9]_[0-9](-PARA|-CHAR)?-[0-9]_[0-9]_[0-9a-f]{12,16}_[0-9a-f]{1,5}`
   - Regex: `^(fonts/|images/|resource/)?(res|resource)_[0-9]_[0-9]_[0-9a-f]{12,16}_[0-9a-f]{1,5}_`
   - Regex: `^(anchor-|section-|story-|style-|navContainer|navUnit)[0-9]_[0-9]_[0-9a-f]{12,16}_[0-9a-f]{1,5}`
   - Regex: `^anchor-[a-z0-9_-]+-[0-9]{17,19}-[0-9]{1,2}$`
   - Regex: `^anchor-[a-z0-9_-]+_[0-9]_[0-9a-f]{12,16}_[0-9a-f]{1,5}$`
   - Regex: `^(LANDMARKS_|TOC_)[0-9]{10,}$`
   - Regex: `^(LazyLoadStoryLineForPage-|TargetSectionForPage-|TargetStoryLineForPage-)[0-9]+$`
   - Regex: `^slice_[0-9]+\.pdf$`
   - Regex: `^Target_pg_[0-9]+_g_2$`
   - Regex: `^KFXConditionalNavGroupUnit_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
   - Exact: `"order-1"`, `"TargetReadingOrder"`, `"PageLabels"`, `"toc_entry"`

5. **SYM_TYPE.BASE64** — matches:
   - Regex: `^(resource/|[ctliz])?[A-Za-z0-9_-]{22}[A-Z0-9]{0,6}((-hd|-first-frame|-thumb)?(-resized-[0-9]+-[0-9]+|-hd-tile-[0-9]+-[0-9]+)?|-ad|-spm|_thumbnail|-transcoded|(_thumb)?\.jpg|\.ttf|\.otf|\.woff|\.woff2|\.eot|\.dfont|\.bin)?$`

6. **SYM_TYPE.SHORT** — matches:
   - Regex: `^(resource/rsrc|resource/e|rsrc|[a-z])[A-Z0-9]{1,6}((-hd|-first-frame|-thumb)?(-resized-[0-9]+-[0-9]+|-hd-tile-[0-9]+-[0-9]+)?|-ad|-spm|_thumbnail|-transcoded|(_thumb)?\.jpg|\.ttf|\.otf|\.woff|\.woff2|\.eot|\.dfont|\.bin)?$`

7. **SYM_TYPE.UNKNOWN** — default fallback

#### `SYM_TYPE` class (lines 180–188)

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

#### `allowed_symbol_prefix` (lines 1089–1090)

```python
def allowed_symbol_prefix(self, symbol_prefix):
    return symbol_prefix in "abcdefilnpstz"
```

Simple string containment check. Valid single-character prefixes: a, b, c, d, e, f, i, l, n, p, s, t, z.

#### `check_symbol_table` (lines 1099–1149)

```python
def check_symbol_table(self, rebuild=False, ignore_unused=False):
```

**What it does**:
1. Walks all fragments to collect `used_symbols` via `find_symbol_references`
2. Collects `original_symbols` from `$ion_symbol_table` fragment
3. Computes `new_symbols` = used non-shared symbols
4. Checks `missing_symbols` = new - original (with rebuild exclusion)
5. Checks `unused_symbols` = original - new (filtering known expected unused)
6. If `rebuild`: rebuilds local symbols sorted naturally, replaces symbol table import

**Dependencies**: `find_symbol_references`, `symtab.is_shared_symbol`, `symtab.get_local_symbols`, `symtab.replace_local_symbols`, `symtab.create_local_symbol`

#### `get_reading_orders` (lines 1182–1189)

```python
def get_reading_orders(self):
```

**What it does**: Returns reading orders from `$538` (document_data) fragment, or falls back to `$258` (metadata) fragment. Returns `[]` if neither exists.

```python
document_data = self.fragments.get("$538", first=True)
if document_data is not None:
    return document_data.value.get("$169", [])
metadata = self.fragments.get("$258", first=True)
return [] if metadata is None else metadata.value.get("$169", [])
```

#### `reading_order_names` (lines 1191–1192)

```python
def reading_order_names(self):
    return [reading_order.get("$178", "") for reading_order in self.get_reading_orders()]
```

#### `ordered_section_names` (lines 1194–1201)

```python
def ordered_section_names(self):
```

Returns flat list of section names from all reading orders, preserving order, deduplicating.

```python
section_names = []
for reading_order in self.get_reading_orders():
    for section_name in reading_order.get("$170", []):
        if section_name not in section_names:
            section_names.append(section_name)
return section_names
```

#### `has_illustrated_layout_page_template_condition` (lines 1228–1256)

```python
def has_illustrated_layout_page_template_condition(self):
```

**What it does**: Scans all `$260` (section) fragments recursively looking for a specific conditional page template pattern:
- Key `$171` (condition) exists
- Value is `IonSExp` of length 3
- `fv[1] == "$183"` (position)
- `fv[2]` is `IonSExp` of length 2, `fv[2][0] == "$266"` (anchor)
- `fv[0]` in `["$294", "$299", "$298"]` (condition operators: equals, range_start, range_end)

Returns `True` if found in any section, `False` otherwise.

#### `get_ordered_image_resources` (lines 1258–1298)

```python
def get_ordered_image_resources(self):
```

**Signature**: `get_ordered_image_resources(self) -> tuple(list, list, list)`

**What it does**:
1. Validates book is fixed-layout
2. Calls `self.collect_content_position_info(skip_non_rendered_content=True, include_background_images=True)`
3. Iterates over content chunks collecting `ordered_image_resources`, `ordered_image_resource_pids`
4. Validates: no text chunks, at least one image, sections have ≤2 images, no mixed foreground/background
5. Returns `(ordered_image_resources, ordered_image_resource_pids, content_pos_info)`

**Dependencies**: `collect_content_position_info`, `is_fixed_layout`

---

## A3: process_section comic/magazine/fixed-layout branches

### File: `yj_to_epub_content.py`

#### `process_reading_order` (lines 105–113)

```python
def process_reading_order(self):
```

Iterates all reading orders, calls `self.process_section()` for each section (deduplicating).

#### `process_section` (lines 115–208)

```python
def process_section(self, section, seq):
```

**Step-by-step**:
1. Pops `section_name` from `$174`
2. Pops known unused keys: `$702`, `yj.conversion.html_name`, `yj.semantics.*`, `yj.authoring.*`
3. Gets `page_templates` from `$141`
4. Checks `has_conditional_template` = any template has `$171`
5. **Dispatch logic** (the key branching):

**Branch 1 — Scribe notebook page** (line ~136):
```python
if "nmdl.canvas_width" in section:
    self.process_scribe_notebook_page_section(section, page_templates[0], section_name, seq)
```

**Branch 2 — Scribe notebook template** (line ~139):
```python
elif "nmdl.template_type" in section:
    self.process_scribe_notebook_template_section(section, page_templates[0], section_name)
```

**Branch 3 — Comic/Children** (line ~142):
```python
elif self.is_comic or self.is_children:
    if len(page_templates) != 1:
        log.error("Comic %s has %d page templates" % ...)
    self.process_page_spread_page_template(
        self.get_fragment(ftype="$608", fid=page_templates[0]), section_name)
```
- Validates exactly 1 page template
- Gets `$608` fragment for the template
- Calls `process_page_spread_page_template`

**Branch 4 — Magazine/Print-replica with conditional template** (line ~150):
```python
elif (self.is_magazine or self.is_print_replica) and has_conditional_template:
```
- Iterates all page templates
- Skips those with `$171` (condition) that evaluate false
- For matching template, checks `$159` (type) and `$156` (layout):
  - Layout `$325` or `$323`: creates book part, adds content, processes position
  - Layout `$437`: calls `process_page_spread_page_template`
  - Other layouts: error
- Validates exactly 1 template processed

**Branch 5 — Default (reflowable/standard)** (line ~186):
```python
else:
    book_part = self.new_book_part(...)
    self.process_content(page_templates[-1], book_part.html, book_part, self.writing_mode, is_section=True)
    self.link_css_file(book_part, self.STYLES_CSS_FILEPATH)
    self.check_empty(page_templates[-1], ...)
    
    if len(page_templates) > 1:
        body = book_part.body()
        for i, page_template in enumerate(page_templates[:-1]):
            # Must have condition ($171)
            self.process_content(page_template, body, book_part, self.writing_mode, is_section=True)
```
- Creates book part from last (main) template
- Processes conditional templates (all but last) inline

---

## A4: process_page_spread_page_template

### File: `yj_to_epub_content.py`

#### `process_page_spread_page_template` (lines 210–344)

```python
def process_page_spread_page_template(self, page_template, section_name, page_spread="", parent_template_id=None, is_section=True):
```

**Parameters**:
- `page_template`: IonStruct or IonSymbol (if symbol, resolves to `$608` fragment)
- `section_name`: string
- `page_spread`: string (default `""`) — one of `"rendition:page-spread-left"`, `"rendition:page-spread-right"`, or `""`
- `parent_template_id`: position ID of parent template
- `is_section`: bool, whether this is a top-level section template

**Branching logic**:

**Branch A — Page spread / Facing page** (lines 213–255):
Condition: `page_template["$159"] == "$270" and page_template["$156"] in ["$437", "$438"]`
- Pops `$159`, `$156`
- Pops `$434` (virtual_panel) — checks if comic/region_magnification
- Pops `$192`, `$67`, `$66`, `$140`, `$560`
- Gets parent_template_id from location_id
- Gets `$259` story fragment via `get_named_fragment`
- Pops `$176` (story_name)
- For layout `$437` → "page-spread", `$438` → "facing-page"
- Alternates left/right properties based on `page_progression_direction`
- Recursively calls itself for each child template in story's `$146`

**Branch B — PDF backed scale_fit** (lines 257–297):
Condition: `page_template["$159"] == "$270" and page_template["$156"] == "$326" and is_section and self.is_pdf_backed and no $67/$66`
- Similar to Branch A but for scale_fit layout in PDF-backed books
- Pops `$159`, `$156`, `$434`, `$192`, `$140`, `$560`
- Validates font_size == 16
- Recursively processes children without page_spread alternation

**Branch C — Connected pagination** (lines 299–335):
Condition: `page_template["$159"] == "$270" and page_template["$156"] == "$323" and page_template.get("$656", False)`
- Pops `$159`, `$156`, `$656`
- Validates `connected_pagination == 2`
- Uses `"rendition:page-spread-center"` for all children

**Branch D — Leaf page template** (lines 337–343):
Default fallback for actual content pages:
- Creates new book part with unique section name
- Processes content with `process_content`
- Links CSS
- If `parent_template_id` is set, processes position

**End**: checks page_template is empty.

---

## A5: yj_position_location.py — ALL classes and functions

### File: `yj_position_location.py`

#### Imports (lines 1–10)

```python
import collections
from .ion import (ion_type, IonAnnotation, IonInt, IonList, IonSExp, IonString, IonStruct, IonSymbol, IS, unannotated)
from .message_logging import log
from .utilities import (list_symbols, natural_sort_key, truncate_list, unicode_len)
from .yj_container import (YJFragment, YJFragmentKey)
from .yj_structure import APPROXIMATE_PAGE_LIST
```

**Module dependencies**: `ion`, `message_logging`, `utilities`, `yj_container`, `yj_structure`

#### Constants (lines 18–27)

```python
KFX_POSITIONS_PER_LOCATION = 110
TYPICAL_POSITIONS_PER_PAGE = 1850
MIN_POSITIONS_PER_PAGE = 1
MAX_POSITIONS_PER_PAGE = 50000
GEN_COVER_PAGE_NUMBER = True
MAX_WHITE_SPACE_ADJUST = 50
DEBUG_PAGES = False
DEBUG_KIM_TEMPLATES = False
RANGE_OPERS = ["$298", "$299"]
```

#### `ContentChunk` class (lines 31–62)

```python
class ContentChunk(object):
    def __init__(self, pid, eid, eid_offset, length=0, section_name=None, match_zero_len=False, text=None, image_resource=None):
```

**Fields**:
- `pid` (int): position ID
- `eid` (int/IonSymbol): entity ID
- `eid_offset` (int): offset within entity
- `length` (int): length in positions (default 0)
- `section_name` (str/None): section this chunk belongs to
- `match_zero_len` (bool): whether zero-length match is OK (default False)
- `text` (str/None): text content of this chunk
- `image_resource` (str/None): image resource name

**Methods**:
- `__init__` (line 32): initializes fields, validates pid≥0, eid>0 (if int), eid_offset≥0, length≥0, text length matches
- `__eq__` (line 45): compares by (pid OR eid+eid_offset+section_name+length), with match_zero_len tolerance
- `__repr__` (line 57): formatted string representation

#### `ConditionalTemplate` class (lines 65–85)

```python
class ConditionalTemplate(object):
    def __init__(self, end_eid, end_eid_offset, oper, pos_info):
```

**Fields**:
- `end_eid`: ending entity ID
- `end_eid_offset`: ending offset within entity
- `oper`: operation type (`"$298"`, `"$299"`, `"$294"`, `"$348"`)
- `pos_info`: list of ContentChunk (snapshot of position info at creation time)
- `use_next`: bool flag (default False)
- `start_eid`, `start_eid_offset`: set to end_eid/end_eid_offset if oper not in RANGE_OPERS, else None

**Methods**:
- `__init__` (line 66): initializes fields, sets start position based on oper
- `__repr__` (line 79): formatted string with conditional template details

#### `MatchReport` class (lines 87–101)

```python
class MatchReport(object):
    def __init__(self, no_limit=False):
```

**Fields**:
- `count` (int): number of reports made
- `limit` (int): max reports (0 = unlimited, else `MAX_REPORT_ERRORS`)

**Methods**:
- `__init__` (line 88): initializes count and limit
- `report(self, msg)` (line 92): logs warning if under limit, increments count
- `final(self)` (line 98): logs if limit was exceeded

#### `BookPosLoc` class (lines 103–1325)

Main class for position and location mapping.

##### `check_position_and_location_maps` (lines 104–126)

```python
def check_position_and_location_maps(self):
```

**What it does**:
1. Calls `collect_content_position_info()` → `content_pos_info`
2. Calls `collect_position_map_info()` → `map_pos_info`
3. If not KFX v1: calls `verify_position_info(content_pos_info, map_pos_info)`
4. Calls `collect_location_map_info(map_pos_info)` → `loc_info`
5. Computes section lengths from map_pos_info
6. Validates `reflow-section-size` feature against calculated value

**Dependencies**: `collect_content_position_info`, `collect_position_map_info`, `verify_position_info`, `collect_location_map_info`, `get_feature_value`, `get_metadata_value`, `is_kfx_v1`, `is_sample`

##### `collect_content_position_info` (lines 128–577)

```python
def collect_content_position_info(self, keep_footnote_refs=True, skip_non_rendered_content=False, include_background_images=False):
```

**This is the largest and most complex function.**

**Parameters**:
- `keep_footnote_refs`: whether to track footnote references (default True)
- `skip_non_rendered_content`: skip content with `$69=True` or type `$439` (default False)
- `include_background_images`: include background image resources (default False)

**Return**: `list[ContentChunk]`

**Internal state**:
- `eid_section`: dict mapping eid → section_name
- `eid_start_pos`: dict mapping eid → starting pid_for_offset
- `pos_info`: list of ContentChunk (accumulated result)
- `section_pos_info`: list of ContentChunk (per-section, merged into pos_info)
- `eid_cond_info`: list of ConditionalTemplate (for illustrated layout)
- `processed_story_names`: set of processed story names
- `self.cpi_pid_`: running position ID counter
- `self.cpi_pid_for_offset_`: running offset-based position counter

**Nested function `collect_section_position_info(section_name)`**:
- Contains deeply nested function `extract_position_data(data, current_eid, content_key, list_index, list_max, advance, note_refs)`
- Which contains nested `have_content(eid, length, advance_, ...)` — the core function that creates ContentChunks

**`extract_position_data` dispatches on `ion_type(data)`**:
- `IonAnnotation` → recurse into value
- `IonList` → iterate children
- `IonSExp` → iterate children
- `IonString` → compute length via `unicode_len`, call `have_content`
- `IonStruct` → complex handling:
  - Extract eid from `$155` or `$598`
  - Track eid→section mapping
  - Handle `$159` (type) specific logic
  - Process `$141` (page_templates) — conditional template logic
  - Process `$142` (style_events) for footnote refs
  - Process `$146` (content_list) recursively
  - Process `$145` (text content) — inline or struct content
  - Process `$176` (storyline reference)
  - Process `$683` (annotations)
  - Process `$749` (alt content)
  - Process `$479` and `$157` for background images

**Conditional template processing** (illustrated layout): When `self.has_illustrated_layout_conditional_page_template` is True:
- Page templates in `$141` are processed with conditional template tracking
- `ConditionalTemplate` objects track start/end eids and operators
- Complex logic for `$294` (equals), `$298`/`$299` (range), `$348` (move)
- Templates can split content chunks at conditional boundaries

**Story processing**: Stories referenced via `$176` are either processed inline or deferred (pending_story_names) for illustrated layout.

**Dependencies**: `ordered_section_names`, `anchor_eid_offset`, `has_illustrated_layout_conditional_page_template`, `unicode_len`, fragments access

##### `anchor_eid_offset` (lines 579–586)

```python
def anchor_eid_offset(self, anchor):
```

**What it does**: Looks up `$266` (anchor) fragment by name, extracts `$183` (position), returns `(eid, offset)` tuple or None.

```python
fragment = self.fragments.get(ftype="$266", fid=anchor)
if fragment is not None and "$183" in fragment.value:
    position = fragment.value["$183"]
    return (position["$155"], position.get("$143", 0))
```

##### `has_non_image_render_inline` (lines 588–621)

```python
def has_non_image_render_inline(self):
```

**What it does**: Cached check. Walks all `$259` and `$608` fragments looking for content with `$601 == "$283"` (render inline) that isn't type `$271` (image). Returns bool.

##### `collect_position_map_info` (lines 623–834)

```python
def collect_position_map_info(self):
```

**Return**: `list[ContentChunk]`

**What it does**:
1. If dictionary/KPF: processes `$611` (eid_section_map) fragment
   - Gets section_pid_count from `$611`
   - Processes each section's `$609` (section_position_id_map) via `process_spim`
   - Validates against `$610` (eidhash_eid_section_map)
2. Otherwise: processes `$264` (position_map) and `$265` (position_id_map) fragments
   - `$264`: maps eids to sections
   - `$265`: single list or per-section position ID maps
   - Per-section: gets `$609` for each section

**Internal `process_spim` function** has parameters: `contains, section_start_pid, section_name, add_section_length, verify_section_length, pid_is_really_len, one_based_pid, int_eid`

**Dependencies**: `ordered_section_names`, `symbol_id`, `has_illustrated_layout_conditional_page_template`, `has_non_image_render_inline`, `get_feature_value`

##### `verify_position_info` (lines 836–904)

```python
def verify_position_info(self, content_pos_info, map_pos_info):
```

**What it does**: Compares content-derived position info with map-derived position info:
1. Creates `PosData` helper objects for both lists
2. Walks both in parallel, matching chunks by eid/eid_offset/section
3. Allows up to 9 positions of slack (lookahead)
4. Reports mismatches via `MatchReport`
5. Uses `has_non_image_render_inline()` to determine strictness

**Internal `PosData` class**: Tracks index and next_pid, provides `advance`, `chunk`, `at_end` methods.

##### `create_position_map` (lines 906–958)

```python
def create_position_map(self, pos_info):
```

**Return**: `tuple(bool, bool)` — (has_spim, has_position_id_offset)

**What it does**:
1. If dictionary/scribe/KPF: remove `$264`, `$265`, `$610` fragments, return (False, False)
2. Remove all mapping fragments: `$264`, `$265`, `$609`, `$610`, `$611`
3. Build `position_map` (`$264`): section → list of eids
4. Build `position_id_map` (`$265`): flat list of {pid, eid, offset} entries
5. Append terminal entry with eid=0
6. Add new fragments to self.fragments
7. Returns (has_spim=False, has_position_id_offset)

**Dependencies**: `ordered_section_names`, `symbol_id`

##### `pid_for_eid` (lines 960–982)

```python
def pid_for_eid(self, eid, eid_offset, pos_info):
```

**What it does**: Linear search (with wraparound from last position) for a chunk matching `eid` with `eid_offset` in range `[chunk.eid_offset, chunk.eid_offset + chunk.length]`. Returns `pid + eid_offset - chunk.eid_offset` or None.

**Caching**: Uses `self.last_pii_` as starting index for faster lookups.

##### `eid_for_pid` (lines 984–999)

```python
def eid_for_pid(self, pid, pos_info):
```

**What it does**: Binary search over pos_info (sorted by pid). Returns `(eid, eid_offset + pid - chunk.eid_offset)` or `(None, None)`.

```python
low = 0
high = len(pos_info) - 1
while low <= high:
    mid = ((high - low) // 2) + low
    pi = pos_info[mid]
    if pid < pi.pid:
        high = mid - 1
    elif pid > pi.pid + pi.length:
        low = mid + 1
    else:
        return (pi.eid, pi.eid_offset + pid - pi.pid)
return (None, None)
```

##### `collect_location_map_info` (lines 1001–1081)

```python
def collect_location_map_info(self, pos_info):
```

**Return**: `list[ContentChunk]` (location info)

**What it does**:
1. Gets `$550` (location_map) fragment
2. For each location entry `{eid, offset}` in `$550.$182`: converts to pid via `pid_for_eid`
3. Gets `$621` (yj.location_pid_map) fragment if present
4. If both present: validates pids match between the two maps
5. If only `$621`: converts pids to eids via `eid_for_pid`
6. Returns list of ContentChunk representing locations

**Dependencies**: `pid_for_eid`, `eid_for_pid`, `get_feature_value`, `get_metadata_value`

##### `generate_approximate_locations` (lines 1083–1114)

```python
def generate_approximate_locations(self, pos_info):
```

**Return**: `list[ContentChunk]`

**What it does**: Generates location boundaries every `KFX_POSITIONS_PER_LOCATION` (110) positions:
1. Tracks running `pid`, `next_loc_position`, `current_section_name`
2. First location at start of each section
3. Subsequent locations every 110 positions
4. Splits across chunks as needed
5. Returns list of ContentChunk with pid, eid, eid_offset

##### `create_location_map` (lines 1116–1130)

```python
def create_location_map(self, loc_info):
```

**Return**: `bool` (has_yj_location_pid_map, always False)

**What it does**:
1. Removes existing `$550` and `$621` fragments
2. Builds new `$550` fragment: `[{eid, offset} for each location]`
3. Appends new fragment to self.fragments

##### `create_approximate_page_list` (lines 1132–1258)

```python
def create_approximate_page_list(self, desired_num_pages):
```

**What it does** (high-level):
1. Validates book type (not dictionary, not scribe, not certain fixed-layout)
2. Gets reading order name (must be exactly 1)
3. Looks for existing `$237` (page_list) navigation in `$389` (book_navigation)
4. If existing page list matches desired: return early
5. Collects `pos_info` via `collect_content_position_info()`
6. Collects `page_template_eids` via `walk_fragment`
7. If fixed-layout: calls `determine_approximate_pages` with `positions_per_page=999999`
8. If `desired_num_pages == 0`: uses `TYPICAL_POSITIONS_PER_PAGE` (1850)
9. If specific page count: binary searches for correct `positions_per_page`
10. Creates/updates `$389` navigation with page list entries
11. Each page: `{label: {value: page_number}, target: {eid, offset}}`

**Dependencies**: `reading_order_names`, `collect_content_position_info`, `determine_approximate_pages`, `walk_fragment`, `create_local_symbol`, `ordered_section_names`

##### `determine_approximate_pages` (lines 1260–1325)

```python
def determine_approximate_pages(self, pos_info, page_template_eids, first_section_name, positions_per_page, fixed_layout=False):
```

**Return**: `tuple(list, int)` — (pages, new_section_page_count)

**What it does**:
1. Iterates pos_info chunks
2. Skips chunks with eid in page_template_eids
3. Skips first_section chunks if `GEN_COVER_PAGE_NUMBER` is False
4. For **fixed_layout**: creates a page for each new section
5. For **reflowable**: creates page breaks:
   - At each new section boundary
   - At every `positions_per_page` positions
   - Adjusts break position to avoid splitting words (looks backward for whitespace)
   - Uses `MAX_WHITE_SPACE_ADJUST` (50) as maximum lookback
6. Each page is: `IonStruct(IS("$241"), IonStruct(IS("$244"), str(page_num)), IS("$246"), IonStruct(IS("$155"), eid, IS("$143"), offset))`

---

## Cross-reference: Fragment Type Symbols

| Symbol | Name | Used In |
|--------|------|---------|
| `$145` | content | yj_to_epub, yj_position_location |
| `$146` | content_list | yj_to_epub_content |
| `$157` | kfx_style | yj_to_epub, yj_to_epub_content |
| `$164` | external_resource | yj_to_epub |
| `$169` | reading_orders | yj_structure |
| `$174` | section_name | yj_to_epub_content |
| `$176` | storyline_name | yj_to_epub_content, yj_position_location |
| `$180` | anchor_name | yj_to_epub |
| `$258` | metadata | yj_structure |
| `$259` | storyline | yj_to_epub, yj_to_epub_content |
| `$260` | section | yj_to_epub, yj_to_epub_content, yj_position_location |
| `$262` | font | yj_to_epub |
| `$264` | position_map | yj_position_location |
| `$265` | position_id_map | yj_position_location |
| `$266` | anchor | yj_to_epub, yj_position_location |
| `$270` | container | yj_to_epub |
| `$387` | section_data | yj_structure |
| `$389` | book_navigation | yj_to_epub, yj_structure |
| `$391` | nav_container | yj_to_epub |
| `$417` | raw_media | yj_to_epub |
| `$538` | document_data | yj_to_epub, yj_structure |
| `$550` | location_map | yj_position_location |
| `$593` | format_capabilities | yj_structure |
| `$608` | page_template | yj_to_epub_content |
| `$609` | section_position_id_map | yj_position_location |
| `$610` | eidhash_eid_section_map | yj_position_location |
| `$611` | eid_section_map | yj_position_location |
| `$621` | yj_location_pid_map | yj_position_location |
| `$756` | ruby_content | yj_to_epub_content |
