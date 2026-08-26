# Kindle Previewer position & location maps — RE findings

Status: focused investigation, 2026-08-26. Scope: Amazon's position/location map
implementation in Kindle Previewer 3.106 and its relation to KFX Input (Python) and
the Go port. This document is deliberately separate from
`docs/kindle-previewer-reverse-engineering.md`; merge pointers only after review.

Evidence grades used below:

- **[F]** fact — directly observed in artifacts in this session (KDF blobs, native
  runtime output, decompiled code);
- **[I]** inference — consistent with the evidence but not directly pinned;
- **[Q]** open question.

## Vocabulary

| Term | Meaning |
| --- | --- |
| EID | element ID: the (numeric) symbol ID of a container/structure fragment. Native KAF exposes it as a `long`; in Ion fragments eids appear as `$598 kfx_id` strings |
| KFXID | the string name of a fragment (`i3`, `i7`, `c0`...). For local fragments KFXID and EID name the same object |
| PID (position id) | 0-based global reading position counter over the whole book; one unit per text character, one per image |
| Location | 1-based coarse reading position ("Kindle location") derived from PIDs |
| `$264` position_map | book-level section → eids list |
| `$265` position_id_map | book-level flat pid → eid(+offset) list |
| `$609` section_position_id_map | per-section `<section>-spm` fragment: one-based pid → kfx_id |
| `$610` yj.eidhash_eid_section_map | `eidbucket_N` fragments: eid → section, bucketed by hash (`$602 block`) |
| `$611` yj.section_pid_count_map | section → pid count (`$144 length`) |
| `$550` location_map | `{$178: reading_order_name, $182: [{$155: eid, $143: offset}]}` |
| `$621` yj.location_pid_map | plain pid list of locations (textbook/print-replica feature, per Python) |

Symbol names are from `internal/kfx/catalog.ion` and Previewer's live native
resolver: `143 offset, 144 length, 155 id, 174 section_name, 178 reading_order_name,
181 contains, 182 locations, 184 pid, 185 eid, 264 position_map, 265 position_id_map,
550 location_map, 609 section_position_id_map, 610 yj.eidhash_eid_section_map,
611 yj.section_pid_count_map, 621 yj.location_pid_map, 598 kfx_id, 602 block`.
The native library also contains the literal strings
`position_map/position_id_map/section_position_id_map/location_map/reading_orders`
(strings in `libshared`), and the KDF `fragment_properties.element_type` values use
the same names, confirming these are Amazon's own terms. **[F]**

## Where the maps are produced (architecture)

Two distinct producers exist, and they produce **different artifacts**: **[F]**

1. **In-book maps (KDF/KFX fragments)** — `$609/$610/$611` are written natively during
   the ordinary EPUB→YJ conversion (`EpubAdapterApp`/`DigitalBook` save). No Java
   writer for them exists in the converter JAR; Java only knows the KAF entity enum
   (`com.amazon.E.b`: `SECTION_POSITION_ID_MAP("section_position_id_map", "section_name")`,
   `POSITION_ID_MAP`, `YJ_KFXID_EID_MAP("yj.kfxid_eid_map", "kfx_id")`).
   The `location_map` `$550` fragment is written by the **full KFXGenApp pipeline**
   when the CREATELOCMAP stage runs (see cadence section).
2. **Cross-format maps** — `CREATEPOSMAP`/`CREATELOCMAP` workers
   (`com.amazon.kfxconverter.process.k/h`) invoke standalone apps
   `com.amazon.kcfpositionmapcreator.core.KCFPositionMapCreatorApp` and
   `com.amazon.kcflocationmap.creator.KCFLocationMapCreatorApp`, which map **Canonical
   YJ ↔ Mobi8/Mobi7** positions into protobuf-based `PositionMap` artifacts
   (`com.amazon.digital.kcfpositionmapping.proto.Types`: `PositionMap`, `MapPartition`,
   `MappingEntry`, `MappingMetadataEntry`). These are side artifacts for
   position-translation services, not the in-book fragments. **[F]**

`KCFPositionMapCreatorApp` argument contract (decompiled): args 0..2 = yj file, temp
dir, language; options `707d…` mobi8+yjM8PosMap pair, `fd9e…` m7M8 map, `88ce…`
sample flag, `4b0f…` mobi7+yjM7 pair; distinguishes `fixed-layout` vs `reflowable`
(`com.amazon.kcfpositionmapcreator.d.d.b`). Its YJ walker (`…positionmapcreator.d.b`)
yields entry kinds TEXT, IMAGE, SVG, PAGEBREAK, BACKGROUND_IMAGE, DISPLAY_NONE and
consumes `MODIFIED_CONTENT_INFO` incremental-edit metadata — i.e. Amazon's canonical
"what occupies a position" model. **[F]** (semantics of each kind: TEXT=per character,
IMAGE/SVG=one entry each, observed below; PAGEBREAK/BACKGROUND_IMAGE observed only in
code, not fixtures **[I]**).

## Observed fragment structures (raw KDF, own symbol table) **[F]**

From `scripts/kp3/dump_kdf_maps.py` on controlled fixtures (minimal, footnote,
vertical-ruby, link, fixed-layout, image-figure, long-text):

```text
c0-spm   $609:: {$174: 'c0', $181: [[1,'i3'], [2,'i5'], [7,'i7']]}
yj.section_pid_count_map  $611:: {$181: [{$174:'c0', $144: 16}]}
eidbucket_22  $610:: {$602: 22, $181: [{$185:'i3', $174:'c0'}]}
```

- `$609` pids are **one-based** (first entry `[1, …]`); Python's reader confirms the
  one-based convention (`one_based_pid=True` on the prepub path). **[F]**
- `$610` `$602` is a hash bucket number (e.g. `i3`→22, `i5`→24, `i7`→26, `i8`→27,
  `i9`→28, `iB`→37, `iD`→39; stable across books, so eid-string-hash bucketing). **[F]** (hash
  function itself not recovered **[Q]**)
- `max_id` fragment = highest shared symbol id in use (854 in these fixtures, i.e.
  shared tail `yj.conversion.*`). **[F]**
- A second `auxiliary_data` set (`$597`, `$258 metadata`, `$492 key`, `$307 value`,
  `$351 default`) carries book-level flags, e.g. `IS_TARGET_SECTION`, and is unrelated
  to positions. **[F]**

The full-pipeline KDF (after CREATELOCMAP) additionally contains a fragment with
`element_type = location_map`:

```text
location_map  $550:: [{$178:'default', $182:[
  {$155:'i3',$143:0}, {$155:'i7',$143:89}, {$155:'i9',$143:108}, {$155:'iB',$143:123},
  {$155:'iD',$143:15}, … {$155:'iP',$143:36}]}]   # 13 entries
```

**[F]** (`$351 default` is the default reading-order name.)

`$264`/`$265` were **not** present in any Previewer-generated KDF inspected (neither
canonical_YJ nor post-locmap). They are book-level fragments of delivered/retail KFX
(Python handles them first and falls back to `$609/$611` on the prepub path).
At which exact pipeline stage `$264/$265` are written — KFX CONT serialization for
delivery vs. earlier — is **[Q]** (not observable with the current fixture set).

## Native `BookPositionInfo` (KAF) semantics **[F]**

`DigitalBook.h()` → `BookPositionInfo` (interface `com.amazon.kaf.c.o`), JNI
`getNative*` methods each dispatch one native vtable call on the position-info object:

| Java | Native vtable | Meaning (verified) |
| --- | --- | --- |
| `b(long)` getNativePositionforID | +0x18 `convertToPosition` | PID (0-based) → `Position` |
| `b(Y)` getNativePositionId | +0x20 `convertToPositionID` | `Position` → global PID |
| `a(Y)` getNativeLocation | — | `Position` → 1-based location |
| `a(long)` getNativePosition | — | 1-based location → `Position` |
| `a()` / `b()` | — | maxLocation / maxPositionId (0-based last PID) |
| `c(Y)` findNativeSectionIDForPosition | — | `Position` → section EID |
| `c(String)` / `e(long)` | — | KFXID string ↔ EID (identity for local names here) |
| `a(long[])` setNativeLocationMap; `b/a(String)` serialize/deserialize | — | location map replace/persist |

- **`Position.a()` is NOT the global PID** (independent Ghidra review + probe
  round-trips): it is the position object's own ID field, which on these fixtures is
  the **EID** of the owning container. **[F]**
- `Position.b()` → `Offset{type=kScalar, value=<character offset within the eid>, point=null}`.
  So `Position` ≈ (eid, char-offset) and global PID = section/chunk start + offset,
  exactly matching `$265`/`$609` arithmetic. **[F]**
- Locations are **1-based**; `a(0L)` dereferences null in native code and segfaults
  the bundled JVM (observed). **[F]**
- `getNativeAnchor` (`c(eid)`) aborts the JVM after the probe's read stages even as an
  existence-only call; anchor→position must be read from fragment data. **[F]**
- `serializeNativeLocationMap(path)` writes a standalone Ion-binary file annotated
  `$550` (same structure as the fragment; minimal fixture:
  `$550:: [{$178:'default', $182:[{$155:'i3',$143:0}]}]`). **[F]**

## Measured position semantics (controlled fixtures) **[F]**

| Fixture | `$609` chunks (`one-based pid → kfx_id`) | Length (`$611`) | Interpretation |
| --- | --- | --- | --- |
| minimal | 1→i3, 2→i5, 7→i7 | 16 | i3 = leading **empty text structure** (storyline-level `$608`, `type=$269 text`, no `$145`) = **exactly 1 PID**; i5 `Hello` = 5; i7 `KFX probe.` = 10; native maxPositionId=15 ✓ |
| footnote | 1→i3, 2→i5, 13→i7 | 26 | i5 `Main text1.` = 11 (noteref anchor text counts); i7 `Footnote text.` = 14 — **footnote aside text is counted** |
| vertical-ruby | 1→i3, 2→i5, 5→i7 | 10 | i7 len 6 = base text `漢字と強調。` — **ruby `rt` pronunciation text is NOT counted** |
| link | 1→i3, 2→i5, 17→i7, 31→i9 | 42 | link text counts normally |
| fixed-layout | 1→i3, 2→i8, 3→i5 | 3 | **page image = exactly 1 PID** |
| image-figure | 1→i3, 2→i5, 13→i7, 14→iB, 35→iD | 56 | **reflowable figure image = exactly 1 PID** (i7), caption text counts (iB), nested container (iD) holds remaining text |

Rules: every text character (post-normalization, pre-HTML-escaping) is one position;
each image (raster or SVG/KVG) is one position; an empty leading text structure is
one position. The native `Position` offsets observed for every PID are exactly
`pid − chunk_start`, i.e. character offsets within the eid. **[F]**

## Location cadence — three distinct behaviors

This is the investigation's central result. On the `long-text` fixture
(1746 total PIDs, `max_id` 856):

1. **Native fallback (canonical_YJ KDF, no locmap stage): locations every 128 PIDs.**
   `maxLocation=14`; location N starts at global PID `128·(N−1)`: 0, 128, 256, …,
   1664. Measured via location→Position→global PID round-trip. **[F]**
2. **Full KFXGenApp pipeline (CREATELOCMAP with `-generateLocMap`, CPL/CM/IMGOP=1):
   a Mobi-derived location map replaces the fallback.** Final KDF has
   `maxLocation=13` with irregular boundaries 0, 99, 242, 381, 521, 664, 807, 950,
   1093, 1236, 1373, 1515, 1658 — and a `location_map` `$550` fragment whose
   entries land exactly on those PIDs (e.g. i7@89 → 10+89=99, i9@108 → 134+108=242).
   The boundaries follow Mobi location rules, not a fixed PID stride. **[F]**
   (Full-pipeline KDF produced by a parallel run from the FC cwd so `bin/kindlegen`
   resolves; boundaries re-verified first-party with the probe on that KDF.)
3. **Python KFX Input / Go port when no `$550` exists: 110 PIDs per location**
   (`KFX_POSITIONS_PER_LOCATION = 110`, `generate_approximate_locations`). On the
   same fixture that yields 16 locations at 0, 110, 220, … — matching neither
   Amazon behavior. **[F]** (code) — this is jhowell's approximation, not an Amazon
   constant.

So "Kindle location" for a Previewer-produced book depends on which producer stage
ran; consumers translating KFX positions ↔ locations must prefer `$550`/`$621`
fragments when present. **[I]** (consumer precedence is Python-observed; Amazon
device behavior not tested **[Q]**).

## Python (KFX Input 2.34) and Go handling — parity status

Python `yj_position_location.py` (Go port `internal/kfx/yj_position_location.go`,
audited 2026-04-22, 394/394 parity at the time):

- `collect_position_map_info` reads `$264` (section→eids), then `$265`; if `$265` is a
  struct it iterates sections with per-section `$609` (`pid_is_really_len`,
  `verify_section_length` against `$265`'s `$144`); dictionary/prepub path uses
  `$611` + `$609` with `one_based_pid` and cross-checks `$610` eidbuckets. **[F]** (code)
- `kpf_book.py` L360-383 (prepub path): if the map is missing/short it **synthesizes**
  `$264/$265` from `collect_content_position_info`, and if `$550` is absent it
  synthesizes an **approximate** `$550` at 110 PIDs/location (skipped for print
  replica/magazine). Consequence: the serialized single-KFX produced by our
  reverse-compare bridge contains kfxlib-synthesized `$264/$265/$550` even though the
  Amazon KDF only had `$609/$610/$611` (+ possibly native fallback locations). **[F]**
- `pid_for_eid` (linear scan w/ cursor) and `eid_for_pid` (binary search) implement
  (eid, offset) ↔ pid using chunk arithmetic identical to the native model. **[F]** (code)
- Go uses the same algorithms with real symbol names (`position_map`,
  `section_position_id_map`, …) instead of `$N`. **[F]**

**No parity bug was found.** Python and Go agree with each other, and their fragment
*reading* semantics agree with Amazon's producer. The only true divergences from
Amazon are (a) the 110/location approximation when `$550` is absent — present in
Python by design, faithfully ported — and (b) the synthesized `$264/$265` on the
prepub bridge path. Neither affects KFX→EPUB output correctness for reading
positions that come from real fragments. Per task policy, no production code was
changed.

## Anchors and ruby/images — summary

- Anchor objects exist natively for text eids (safe existence check succeeded on
  earlier stages) but position reads via `Anchor` crash the runtime; fragment-level
  anchor data (`$266 anchor` fragments carrying `$183 position {id, $143 offset}`
  as consumed by Python's `anchor_eid_offset`, `yj_position_location.py` L579-586)
  is the reliable source.
  **[F]**/**[I]** (native anchor-position equality not proven **[Q]**)
- Ruby: `rt` text excluded from positions; base text chars counted one each. **[F]**
- Images (fixed-layout page, reflowable figure): exactly one position each. **[F]**
- Footnote aside text **is** position-counted. **[F]**

## Reproduction

```sh
# native position view (pid/eid/globalPid/locations/sections), rc=0
./scripts/kp3/run_probe.py --fixture long-text --positions --workdir /tmp/kp3-pos/long-text

# raw KDF map fragments with the KDF's own symbol table
python3 scripts/kp3/dump_kdf_maps.py /tmp/kp3-pos/long-text/book.unwrapped.kdf

# reverse-parity through the kfxlib bridge (uses synthesized maps; see caveat above)
./scripts/kp3/reverse_compare.py --fixture long-text --workdir /tmp/kp3-rev-longtext
```

Tooling notes: `run_probe.py --positions` (optionally `--locmap-out PATH` to serialize
the native location map); probe stages 2/4 label the position
object's ID field `eid` and convert global PIDs explicitly through
`BookPositionInfo_getNativePositionId` (vtable +0x20); unsafe calls (location 0,
`getNativeAnchor`) are avoided/gated. JVM fatal-error files are routed to `/tmp`
(`-XX:ErrorFile`), including for crash-prone exploratory probes.

## Open questions

1. `$264`/`$265` writer stage: never observed in Previewer KDFs; presumably produced
   during retail KFX CONT serialization (device delivery). Needs a real retail KFX
   with known producer version, or the Scribe/KPF packaging path. **[Q]**
2. `$610` bucket hash function. **[Q]**
3. `$621 yj.location_pid_map` producer (textbook/print-replica only in Python's
   experience). **[Q]**
4. Native fallback constant 128: observed on one fixture; confirm on a second
   long book with different section structure when fixtures allow. **[Q]**
5. Whether device renderers re-derive locations when `$550` is absent (fallback 128)
   or require the fragment. **[Q]**
