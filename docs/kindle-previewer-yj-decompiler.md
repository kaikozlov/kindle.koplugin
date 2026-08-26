# Kindle Previewer YJ→EPUB decompiler: surviving components & reconstructability

Focused follow-up to `docs/kindle-previewer-reverse-engineering.md` (§ "YJ decompiler evidence"
and § "Exact recovered decompiler launch contract"). That doc recovered the *native launcher*;
this doc answers the complementary question: **which decompiler components remain callable in
the current Previewer bundle, and is the YJ→EPUB path reconstructable without shadowing all of
kfxlib?**

Investigation scope: current app bundle only (`REFERENCE/Kindle Previewer 3.app/`). No web
search, no production code changes.

## Method note (important for reproduction)

`EpubToKFXConverter-4.0.jar` contains obfuscated packages that differ **only by case**
(`com/amazon/q/**` vs `com/amazon/Q/**`, `f` vs `F`, `e` vs `E`, …). On a case-insensitive
APFS volume, extraction merges these trees and silently corrupts cross-reference searches.
All cross-class reference claims below were verified against **raw ZIP entry bytes** via
Python `zipfile`, not an extracted tree. Single-class inspection uses
`javap` on bytes pulled straight from the archive.

```sh
JAR="REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/lib/EpubToKFXConverter-4.0.jar"
python3 - <<'EOF'   # authoritative reference scan (case-sensitive)
import zipfile
jar = zipfile.ZipFile("REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/lib/EpubToKFXConverter-4.0.jar")
for n in jar.namelist():
    if n.endswith(".class") and b"<PATTERN>" in jar.read(n):
        print(n)
EOF
```

---

## 1. CONFIRMED: the entry point is missing, and only the entry point class name survives

- `com.amazon.yj.decompiler.app.DecompilerApp` appears **exactly once** in the whole bundle:
  as a C-string in the native Mach-O (`Contents/MacOS/Kindle Previewer 3`, file offset
  `0x2707865`). No jar under `Contents/` contains the class (raw-zip byte scan).
- The bundled `EpubToKFXConverter-4.0.jar` ships the decompiler's *message bundles*
  (`com/amazon/language/resources/yjdecompiler/{error,debug,info}_en.properties`) and its
  module registration (`PackageToResourceLocationMap.properties` maps
  `YJDecompiler = com.amazon.language.resources.yjdecompiler`), but zero driver classes
  under any `yj/decompiler/` or equivalent package.
- No class in the jar logs `YJDECOMPILER_INSIDE_TRANSFORMER`,
  `YJDECOMPILER_LINK_RESOLVER_DATA`, `YJDECOMPILER_MAPPING_STYLE_KEY_LIST`, etc. The only
  surviving uses of `YJDECOMPILER_*` constants are:
  - `com/amazon/I/b.class` — the error-code enum itself (all ~50 codes present);
  - `com/amazon/adapter/common/n/a/d.class` — uses `YJDECOMPILER_STACKTRACE` while invoking
    the style mapper on the *forward* path;
  - `com/amazon/q/a/g/b/a/c.class` — a workflow step that writes a metadata file and throws
    `YJDECOMPILER_FILE_IO_EXCEPTION` (see § 3.2).

## 2. CONFIRMED: no dynamic jar / classpath / download mechanism provisions the decompiler

- The native launcher constructs exactly one classpath jar: `/lib/EpubToKFXConverter-4.0.jar`
  (native strings, file offsets `0x2708a90` region). `constructEnvironmentVars`
  (`0x10005f120..0x10005fa80`) builds `LD_LIBRARY_PATH`/`DYLD_LIBRARY_PATH` style values from
  `<root>/lib` and `<root>/lib/shared_libs` — native library pathing, not class loading.
- `MERGED_JAR_FILE_PATH` (env var, seen in `ConversionEngine.initializeAppConfig`) is only
  forwarded as `--JARConfigFile=<path>` to the conversion engine and only on non-Linux —
  an Amazon Brazil build-config mechanism, not a payload fetcher.
- The app's download machinery (DOW/DOWCacher, presigned S3 artifact URLs) fetches **book
  artifacts for cloud previews**, not jars.
- Conclusion: the decompiler was never dynamically provisioned in this build. It was either
  stripped from the jar at build time or existed only in internal builds.

## 3. CONFIRMED: surviving decompiler-side components (the reusable inventory)

### 3.1 `com.amazon.yjhtmlmapper` — dual-direction style mapper (106 classes, intact)

The package implements **both** mapping directions behind one abstract class:

```
com.amazon.yjhtmlmapper.f.c            (abstract style mapper)
  static c() / a(boolean)  ── factory, synchronized lazy init → new com.amazon.yjhtmlmapper.e.k(boolean)
  FORWARD (HTML→YJ):  List<f.d> a(List<f.a> htmlStyles, B.d.b doc, List<d.d> ctx)
                      └ sole surviving caller: com.amazon.adapter.common.n.a.d (ingestion path)
  REVERSE (YJ→HTML):  List<f.a> a(List<f.d> containers, f.f containerType, List<d.d> ctx,
                                   c.c docContext, B.d.b doc, B.d.e.b.f styleGroup, e.b flags)
                      └ implemented in e.k; declared in f.c; ZERO external callers
```

Verified on raw ZIP bytes: the reverse method's exact descriptor occurs in exactly two
entries — `f/c.class` (abstract + public wrapper) and `e/k.class` (implementation). The
consumer was the stripped decompiler driver. **The reverse engine is present, orphaned, and
callable.**

Argument contract for the reverse method (types confirmed by `javap`):

| Parameter | Type | Role (confirmed by structure) |
| --- | --- | --- |
| containers | `List<f.d>` | YJ element runs; `f.d` wraps `B.d.e.h.s` (native YJ element handle) + `List<B.d.f.e>` style entries |
| containerType | `f.f` enum | `TEXT_CONTAINER, BOX_CONTAINER, IMAGE_CONTAINER, HORIZONTAL_RULE_CONTAINER, LIST_CONTAINER, TABLE_CONTAINER, STYLE_EVENT_ONE_D, STYLE_EVENT_TWO_D, ANCHOR` (each with `name` + `displayType`) |
| ctx | `List<d.d>` | mapping context entries (`d.c` kind + `f.a` style + `f.d` container + id) |
| docContext | `c.c` | document context: `c.a` + `q.a.a.a` + section headings pair (`B.d.e.h.h`, two `B.d.e.h.x`) |
| doc | `B.d.b` | KCF DOM document interface (containers, styles, elements — see § 3.4) |
| styleGroup | `B.d.e.b.f` | style group handle from the doc |
| flags | `e.b` | boolean option bundle |
| returns | `List<f.a>` | HTML style objects (`f.a` = html tag + attr map + css map + template name + `ab.e.c` template ref) |

Supporting machinery, all shipped:

- `e.d` / `e.l` — style-map loaders. Primary source: `data/stylemap.ion` (gzipped binary
  ION, annotation `com.amazon.yj.style.map.entry@1.0`), fallback `data/yjhtml_mapping.txt`
  (plain text), overridable via system property **`style_mapping_dir`** (same property name
  is passed by the native launcher — § 5). `e.d` explicitly reads the ION field
  **`ignore_for_yj_to_html_mapping`** — direct evidence this data file drives the reverse
  direction, not just the forward one.
- `stylemap.ion` schema (from the decompressed header): `html_tag, html_attribute,
  html_attribute_value_unit, yj_property, yj_value_type, yj_unit, html_attribute_value,
  yj_value, css_styles, style_name, style_value, display, converter_classname,
  ignore_for_yj_to_html_mapping`.
- `e.e` — style-map entry; holds `Class<SpecialStyleTransformer>` decoded from
  `converter_classname`.
- `transformers/` — 22 **unobfuscated** transformer classes, uniform contract:
  `ctor(e.c styleKey, e.e mapEntry, B.d.b doc, e.b flags[, Set extras, c.c docContext])`,
  `a() → List<f.a>` (decompiled HTML styles), `b() → List<f.d>` (YJ containers).
  Includes `BGColor, BGRepeat, BorderRadius, DefaultStyle, ImageBorder, Language,
  LineHeight, LinkStyle, MarginAuto, MaxCropPercentage, NonBlockingBlockImage, PageBleed,
  ShapeOutside, TextCombine, TextDecoration, TextEmphasisStyle,
  TransformerForWebkitTransform, UserAgentStyleAdding, WidowsOrphans, WritingMode, XYStyle`.
- `i.*` — CSS value utilities (colors via `java.awt.Color`, units, `img.height`/`hr.width`
  style key tables).
- `a.a` — dev-time generator: reads `./data/yjhtml_mapping.txt` → ION (proves the .ion is
  generated from the .txt and how to regenerate/extend it).
- `g.a` — container post-processing on `List<f.d>` (forward-side sibling kept for symmetry).

### 3.2 `com.amazon.q.a.g` — EPUB output subsystem (31 classes, self-contained, orphaned)

Raw-zip scan: 31 entries reference `com/amazon/q/a/g`; **30 are inside the package; the only
external referent is `com/amazon/F/d/h`** (§ 3.3). No code bridges `kaf` (native book) to
`q.a.g` (writers) — zero classes reference both. That bridge was the decompiler driver.

- `q.a.g.a.a` — `ResourceType` enum (with folder names): `HTML, CSS, FONT, IMAGE, NCX, OPF,
  EPUB_2_YJ_MAPPING, XML, MIMETYPE, NAVIGATION`. `EPUB_2_YJ_MAPPING` matches the error
  string "Epub2YJMapping is supported only for cssSuppression" verbatim.
- `q.a.g.b.*` — resource writers:
  - `b` — xhtml writer (`"xhtml"`, guards "NavStore is null" / "Opf Store is null")
  - `c`, `n` — XML writers / DOM construction
  - `d` — CSS rule writer ("CSS rule should not be NULL")
  - `e` — metadata-file writer ("Nothing to write in Metadata file")
  - `j` — **NCX** writer
  - `k` — **OPF + NAV** writer (manifest ids, spine id-refs, `cover-image`, `nav`
    properties; matches `YJDECOMPILER_ERROR_ADDING_COVER_IMAGE` / `..._SPINE_PROPERTY_TO_OPF`)
  - `l` — base resource writer ("Ref Resource cannot be null")
  - `m` — template-CSS resource modifier ("Cannot modify a template css resource")
  - `q.a.g.b.a.*` — workflow-step wrappers; `a.c` writes a metadata file via `FileWriter`
    (`p()` path, `v()` content) and throws `YJDECOMPILER_FILE_IO_EXCEPTION`
- `q.a.g.c/d/e` — stores: OpfStore, NavStore ("NavStore is already set"), BookMetadata
  ("Bookmetadata cannot be null", "Opf Store cannot be null")
- `q.a.f.h` — directory→EPUB **zipper** ("IOException while zipping the directory";
  matches `YJDECOMPILER_ZIP_FILE_CREATION_FAILURE`); also unpack helper for `input`
  directories
- `q.a.b.a` — Dublin-Core metadata enum (`METADATA_TITLE/AUTHORS/IDENTIFIER/…` → `dcmes`
  `Title/Creator/…`)
- `q.a.c.b` — decompiler exception type (thrown by `F.d.h`, `q.a.g.b.a.c`)

### 3.3 `com.amazon.F.d.*` — style-template subsystem + data-root resolution (intact)

- `F.d.h` — style-template manager: enumerates `data/Templates/*.dotx` (KTClassic,
  KTModern, KTCosmos, KTAmore, KTStranger, ArticleMaster), resolves fonts (Bookerly,
  Amazon Ember, Open Sans Light, Merriweather) from `data/Fonts`, emits CSS resources
  (`styles`, `text/css`, `@font-face` with `font-family`/`src`), throws `q.a.c.b` on
  failure — this is the `YJDECOMPILER_ERROR_LOADING_TEMPLATE` implementation.
- `F.a.a` — data-path constants: `data/semantics.ion`, `data/template-properties.ion`,
  `data/font-info.ion`, `data/Templates`, plus KTemplate semantic names.
- `F.d.j` — **root resolution**: `System.getenv("YJCONVERSION_ENV_ROOT")` with fallback
  `System.getProperty("YJCONVERSION_ENV_ROOT")`; when unset, paths resolve relative to CWD.

### 3.4 Native YJ book access without kfxlib — `kaf` JNI + KCF DOM (intact)

- `com.amazon.kaf.jni.adapters.c.a()` — singleton init:
  `System.loadLibrary(System.getProperty("klibname", "KAFJNI-shared"))`. The native
  launchers pass `-Dklibname=shared` → `libshared.dylib`, which exports **476**
  `Java_com_amazon_kaf_jni_*` symbols (verified `nm` export list), including
  `BookFactory.nativeGetBook(String)`, `nativeGetBook(long)`, element/style/property
  accessors, and a native ION reader pool.
- `BookFactory.a(String path)` → `com.amazon.kaf.c.y` (native book interface: sections,
  positions, anchors, metadata, resource reads via `kaf.c.x`).
- **Surviving minimal loader recipe** — `com.amazon.kcflocationmap.c.a`:
  `public static kaf.c.y a(String path)` validates a `.yj`/`.kdf` file, logs "KFX sdk is
  being initialized for …", initializes the JNI singleton, and returns the book. This is a
  complete, shipped example of file→native-book loading from a plain `main`.
- Standalone shipped mains that already drive this stack: `KCFLocationMapCreatorApp`,
  `kcfpositionmapcreator.*`, `kfxconverter.app.KFXGenApp` (forward direction).
- `com.amazon.B.c.*` — KCF DOM SDK layer over the JNI objects; `B.c.a.a.b.a()` constructs a
  default `B.d.b` document factory (`B.d.b` = the document interface the style mapper's
  reverse method consumes).
- `com.amazon.B.b.a` — SVG/CSS transform parsing helpers (translate/scale/matrix) used by
  the mapper.
- `com.amazon.N.f.*` — jsoup-based HTML/CSS DOM utilities (element tracking,
  `CSSStyleDeclaration` caches, link/anchor bookkeeping) — the closest surviving relative
  of the decompiler's "HTML context"; same libraries the driver would have used.

### 3.5 Bundled data files (all under `Contents/lib/fc/data/`)

| File | Purpose | Loader |
| --- | --- | --- |
| `stylemap.ion` (gzip ION) | YJ↔HTML style mapping incl. reverse-direction flag | `yjhtmlmapper.e.d`/`e.l` |
| `stylelist.ion` (gzip ION) | style-merger rules (`com.amazon.yj.htmlstylemerger.yjstylelist.properties@1.0`) | `yj.style.merger.d`/`e.a` |
| `Templates/*.dotx`, `template-properties.ion`, `font-info.ion`, `Fonts/` | style templates | `F.d.h`, `F.a.a`, `F.d.j` |
| `semantics.ion`, `mapping_ignorable_patterns.ion`, `puaMapper.ion` | semantic/ignorable mappings | various |

## 4. CONFIRMED: what is *missing* (the actual gaps)

1. `com.amazon.yj.decompiler.app.DecompilerApp` — arg parsing + orchestration.
2. **Entry-point container transformers** — the structural YJ-container→HTML walk.
   `error_en.properties` names `BlockImageTransformer.getHeightOrWidthValueInPixel`;
   raw-zip scan finds `BlockImageTransformer` in exactly one entry:
   `yjhtmlmapper/transformers/NonBlockingBlockImageTransformer` (the *forward* counterpart,
   different class). No entry-point transformer classes survive.
3. **HTML link resolver** — `YJDECOMPILER_LINK_RESOLVER_DATA` ("Overwriting with anchors on
   html … resolved/unresolved anchor count") and `YJDECOMPILER_HTML_LINK_RESOLVING_FAILURE`
   have no surviving logger; `N.f` utilities exist but the resolver itself is gone.
4. **HTML context initialization** (`YJDECOMPILER_HTML_CONTEXT_INITIALISATION_FAILURE`) —
   the builder that creates per-file HTML contexts is gone.
5. **KFX validation step** (`YJDECOMPILER_KFX_VALIDATION_FAILURE`) — gone.
6. The **bridge** from `kaf` book → `q.a.g` writers (zero classes reference both).

## 5. CONFIRMED: launcher environment contract (extends the existing doc)

Newly recovered native strings (file offsets `0x2708900..0x2708b00`, adjacent to the JVM
launch strings) give the complete env/property name table the native side understands:

| Name | Kind | Role |
| --- | --- | --- |
| `YJCONVERSION_ENV_ROOT` | env (Java reads getenv→getProperty fallback) | root for `data/`, templates, fonts (`F.d.j`) |
| `LD_LIBRARY_PATH` / `DYLD_LIBRARY_PATH` | env | built from `<root>/lib`, `<root>/lib/shared_libs` |
| `-Djava.library.path=` | JVM prop | same dirs |
| `-Dklibname=shared` | JVM prop | selects `libshared.dylib` for `System.loadLibrary` |
| `style_mapping_dir` | JVM prop/system prop | overrides style-map location (`yjhtmlmapper.e.l`/`e.d`) |
| `style_merger_dir` | JVM prop | style-merger data location |
| `yj_character_fixer_base_dir` | JVM prop | character-fixer data |
| `CSS_HOME_DIR` | env | CSS data home |
| `MERGED_JAR_FILE_PATH` | env (non-Linux) | forwarded as `--JARConfigFile=` |
| `yjhtmlcleaner_path`, `phantomjs_home_dir`, `js_scripts_home_dir` | env/dir | tool locations |
| `conversionReport.ion`, `preserve_conv_log` | filenames | conversion report artifacts |

The subprocess argv itself (main class, KDF input, `book.epub` output, temp dir,
`--format epub3 --generator <g> --isComixologyGenerated <b> [--title …] [--author …]`) is
documented in `docs/kindle-previewer-reverse-engineering.md` and was not re-derived here
beyond confirming the immediate-encoded fragments (`--format`, `epub3`, `--generator`,
`--isComixologyGenerated`, `--title`) at `0x10005ea0d..0x10005ed0c`.

---

## 6. INFERENCE (clearly labeled — not byte-level proven)

- **Reconstructability: high for everything except the structural walk.** The surviving set
  covers style mapping (both directions), all EPUB resource writing (xhtml/css/ncx/opf/nav/
  mimetype/metadata), the final zip, style templates, and native YJ book access. The pieces
  to re-implement are: the driver `main` (arg contract already known from the native
  launcher), the entry-point container→HTML transformers, the link resolver, and HTML
  context assembly. This is a structural walk over `B.d.b`/`kaf.c.y` sections emitting
  xhtml via `q.a.g.b.b` — comparable in size to the yj_to_epub work already done in this
  repo's Go port, not to kfxlib as a whole.
- **No kfxlib shadowing needed for object-model access.** `libshared.dylib` + `kaf` JNI +
  `B.c` DOM give a complete native YJ/KDF reader — the same one the original decompiler
  used. What would still be "shadowed" is only the *emitter* logic, which we already
  independently implement in Go.
- **The strip was selective, not complete.** The presence of message bundles, the module
  registration, the orphaned writer package, and the reverse-mapper implementation suggests
  the decompiler jar was pruned by package (removing `com/amazon/yj/decompiler/**`-style
  entry points and entry-point transformers) rather than rebuilt from a decompiler-free
  source tree. Older Previewer/Kindle Create builds plausibly shipped the full set
  (testable only with an older installer, out of scope here).
- **JNI lifecycle hazards are real.** The parallel in-repo probe
  (`scripts/kp3/com/amazon/kaf/jni/adapters/KafPositionProbe.java`, crash log
  `hs_err_pid16862.log`) shows `libshared.dylib` can abort a plain JVM when native
  ownership rules are violated (crash inside `jni_ThrowNew` from `libshared+0x1b7e`).
  Any reconstruction should follow the shipped `kcflocationmap`/`kcfpositionmapcreator`
  call ordering and treat probes as one-shot subprocesses.
- The decompiler's `--format epub3` and generator flag imply the driver selected OPF/NAV
  (EPUB3) output by default; `q.a.g.b.k` writing both `opf` and `nav` plus `j` writing
  `ncx` suggests both EPUB2 and EPUB3 outputs were supported, selected by that flag.

## 7. Bottom line

The missing YJ→EPUB decompiler is **not** recoverable as a runnable whole from the current
bundle: the driver, entry-point transformers, link resolver, HTML context, and validator are
absent, and nothing dynamically provisions them. However, the bundle retains every
*leaf* subsystem the driver called — reverse style mapper + `stylemap.ion`, the complete
EPUB resource-writer package, the template manager, the zipper, and the native YJ book
reader — all orphaned but callable under `YJCONVERSION_ENV_ROOT`/`-Dklibname=shared`. A
reconstruction therefore reduces to writing one driver plus the structural container→HTML
walk, and does not require shadowing kfxlib.

Appendix: key evidence offsets (Mach-O `Contents/MacOS/Kindle Previewer 3`, `__cstring`
at vmaddr `0x102701ba0`, file off `40901536`): `DecompilerApp` `0x2707865`; env/prop table
`0x27089a9..0x2708a90`; `/lib/EpubToKFXConverter-4.0.jar` `0x2708a90`.
