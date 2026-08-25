# Kindle Previewer 3 KFX/YJ/KAF reverse-engineering notes

Status: first-pass static analysis, 2026-08-25

This document records a reoriented investigation of Kindle Previewer 3 as a primary semantic source for KFX/YJ/KAF. The goal is not to treat the current Python KFX Input implementation as the specification. Instead, the working model is:

- Kindle Previewer exposes Amazon's intended producer-side semantics and a large amount of the internal YJ/KAF implementation.
- KFX Input remains valuable as evidence for consumer KFX variants, historical formats, malformed-but-tolerated inputs, and one particular KFX -> EPUB inverse implementation.
- Differences between the two are research targets rather than presumptive bugs in either one.

The findings below are based on the local Previewer bundle at:

`REFERENCE/Kindle Previewer 3.app`

The bundle reports version **3.106**.

## Executive findings

The most important result of this pass is that Kindle Previewer is much more than an opaque KFX generator.

Amazon ships, in the Previewer bundle:

1. a ~54 MB Java converter containing the EPUB -> YJ conversion pipeline;
2. the KAF/YJ object-model interfaces and JNI wrappers;
3. a ~71 MB native KAF implementation (`libshared.dylib`, installed as `libKAFJNI-shared.dylib`);
4. an 854-entry named KAF property enum whose ordinal is used as the property ID;
5. machine-readable Ion files describing HTML/CSS -> YJ style mappings, semantic mappings, style inheritance/merge rules, and template defaults;
6. a **bidirectional** HTML <-> YJ style mapper, not merely a forward HTML -> YJ mapper;
7. dedicated position-map and location-map generators;
8. error/resource strings for an Amazon `YJDECOMPILER` capable of generating EPUB, although a top-level decompiler entry point has not yet been located in this bundle;
9. a directly usable native KAF runtime: a standalone Java harness can load Previewer's bundled `libshared.dylib` and retrieve Amazon's live 854-entry property-name/index map without running the Previewer GUI.

This materially changes how KFX should be investigated. For a large class of format questions, we do not need to infer intent from arbitrary commercial KFX samples or from anonymous `$NNN` branches in kfxlib. We can inspect Amazon's named property model, inspect the producer logic, generate controlled inputs, and in some cases inspect Amazon's own reverse mapping.

It does **not** make arbitrary KFX -> EPUB conversion trivial. Current Previewer cannot be assumed to generate every historical or consumer-side KFX structure ever accepted by Kindles, and the forward transform can discard source-level distinctions. Those remain compatibility problems. But the canonical semantics are substantially less opaque than kfxlib alone makes them appear.

## Primary artifacts

### Java converter

`REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/lib/EpubToKFXConverter-4.0.jar`

Observed properties:

- size: approximately 54 MB;
- 31,833 Java class entries in the archive;
- roughly 6,457 `com.amazon.*` classes;
- relevant package families include approximately:
  - 505 `com.amazon.yj.*` classes;
  - 207 `com.amazon.kaf.*` classes;
  - 106 `com.amazon.yjhtmlmapper.*` classes;
  - 85 `com.amazon.kfxconverter.*` classes;
  - 56 `com.amazon.kcfpositionmapcreator.*` classes.

A full JADX decompilation was successfully produced during this investigation in a temporary directory. Reproduce it with:

```sh
out=$(mktemp -d /tmp/kp3-jadx.XXXXXX)
jadx -d "$out" --show-bad-code --comments-level warn \
  'REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/lib/EpubToKFXConverter-4.0.jar'
```

The decompiler output is an aid, not an authority. For classes affected by case-insensitive filename collisions, decompile the class directly from the JAR with `jadx --single-class` or inspect it with `javap`.

### Native KAF library

`REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/lib/libshared.dylib`

Observed properties:

- approximately 71 MB;
- universal Mach-O containing x86_64 and arm64;
- install name/dependency exposes `@rpath/libKAFJNI-shared.dylib`;
- the Java wrapper loads `KAFJNI-shared`, with Previewer configuring the library name as `shared` on this platform;
- hundreds of named JNI exports survive stripping;
- C++ RTTI/type strings retain many KAF/YJSDK class names.

An x86_64 slice already exists at:

`REFERENCE/ghidra_analysis/libshared/libshared.x86_64.dylib`

### Converter data

Important machine-readable data lives under:

`REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/data/`

Especially:

- `stylemap.ion`
- `stylelist.ion`
- `semantics.ion`
- `semanticmap.ion`
- `template-properties.ion`
- `mapping_ignorable_patterns.ion`

These files are gzip-compressed binary Ion despite their `.ion` extension.

## Top-level KFX generation pipeline

### `KFXGenApp`

The top-level converter is:

`com.amazon.kfxconverter.app.KFXGenApp`

The stage enum `com.amazon.kfxconverter.d.g` gives the pipeline vocabulary directly:

```text
WORDTOEPUB
STRIPSOURCE
CREATEMOBI
CREATEYJ
IMGOPTIMISATION
CREATEPOSMAP
CREATELOCMAP
CREATEPOSMAPLOCMAP
PROCESSEPUB
ENDPROCESS
```

`KFXGenApp` orchestrates worker classes under `com.amazon.kfxconverter.process.a.*`. The useful mapping established in this pass is:

| Worker | Stage | Role |
| --- | --- | --- |
| `process.a.c` | `PROCESSEPUB` | EPUB/source preprocessing |
| `process.a.b` | `CREATEMOBI` | master MOBI production |
| `process.a.d` | `CREATEYJ` | source -> YJ generation |
| `process.a.g` | `IMGOPTIMISATION` | post-conversion image optimization |
| `process.a.e` | `CREATEPOSMAPLOCMAP` | position map followed by location map |

The application-level options are also informative. They include:

- `-allowYJConversionForFL`
- `-allowYJConversionForCN`
- `-allowYJConversionForArabic`
- `-disableLocMapGeneration`
- `-disablePostConversionForYJ`
- `-generateLocMap`
- `-shouldStampSemantics`
- `-run-conversion-for-yjcoach`
- `-run-conversion-for-yjbackfiller`

The converter constants identify internal/intermediate outputs such as:

- `canonical_YJ`
- `book.kdf`
- `book.kcb`
- `conversionReport.ion`
- `PostConversionReport.ion`
- `epubProcessorReport.ion`

The same code contains Amazon-internal deployment vocabulary including:

- `/apollo/env/YJConversionTools/`
- `/apollo/env/EpubUtilities`
- `YJCONVERSION_ENV_ROOT`

These strings are not needed to infer the architecture; the converter actively configures and invokes the same components locally.

## The actual EPUB -> YJ generator is in the JAR

The `CREATEYJ` process wrapper eventually invokes:

`com.amazon.adapter.common.app.EpubAdapterApp`

`EpubAdapterApp.main()` is very small. It delegates to:

`ConversionEngine.getEngine().convertToYJ(args)`

`com.amazon.adapter.common.wrapper.ConversionEngine` initializes the environment, loads KAF JNI, initializes the property-name system, and invokes:

`new YJConverter().convertToYJ(args)`

`com.amazon.adapter.common.wrapper.YJConverter` chooses a converter implementation based on source type. In the paths inspected here:

- EPUB/ZIP uses `com.amazon.adapter.c.a.a`;
- HTML uses `com.amazon.adapter.e.a.a`.

Both are subclasses of:

`com.amazon.adapter.common.d.a`

This class is the main conversion orchestration layer, and it exposes the producer pipeline in unusually explicit form.

### Producer pipeline inside `com.amazon.adapter.common.d.a`

The final conversion method runs, in order, approximately the following phases. The labels below are Amazon's own instrumentation/log labels from the implementation:

1. `Preprocess Args`
2. `Initialize YJ Document Builder`
3. `Transform Metadata`
4. `Preprocess HTML`
5. `Validate and Fix image for PDF Conversion`
6. `Stamp Book Language`
7. `Transform To YJ`
8. `Create Dictionary Rules`
9. `Transform Navigation Information`
10. `Transform Semantic Information`
11. `Post Process YJ`
12. `Drop style handler`
13. `Optimise YJ Document`
14. `Create Cover Section`
15. `Detect And Inject Notes`
16. `Fix Up Anchors Linking To Ruby Text`
17. `Style Fixup`
18. `Override Kindle Font Property`
19. `Table Semantic Analysis`
20. `Unit Normalization`
21. `Post Normalization Fixup`
22. `Populate Content Features`
23. `Set Content Capabilities`
24. `Remove Dual Cover`
25. `Post Style Optimization`
26. `Create Fixed Regions`
27. `Validate YJ Document`
28. `Write YJ Document To Disk`

This is a central finding. The generator is not merely a call into an opaque native binary. A large fraction of the semantic conversion, fixup, optimization, validation, and feature-version logic is directly visible in the Java code.

Examples already visible in this class include:

- dictionary-rule generation;
- special handling for vertical writing;
- table semantic analysis and table/table-viewer feature versions;
- YJ optimizer selection;
- unit normalization;
- publisher-note/footnote injection;
- ruby-anchor fixup;
- style fixups and post-style optimization;
- content capability/feature stamping;
- fixed-region generation;
- final YJ validation.

### EPUB-specific HTML -> YJ traversal

`com.amazon.adapter.c.a.a.j()` iterates the EPUB spine HTML files. For each file it creates an adapter-side source object and selects a transformer using:

`com.amazon.adapter.common.i.a.h.a(...)`

That factory chooses different conversion strategies for:

- facing pages;
- page spreads;
- connected-page-spread content;
- ordinary reflowable content;
- another specialized path determined from book metadata.

The common transformer base is:

`com.amazon.adapter.common.i.a.a`

For each DOM node it:

1. preprocesses the document;
2. converts the source node to an internal HTML representation;
3. selects an element handler through `C3955t`;
4. invokes that handler to build the YJ representation.

This gives a concrete route for future source-to-semantics tracing: start at a specific HTML construct, follow the element handler selected by `C3955t`, then follow style conversion through `yjhtmlmapper`.

## `yjhtmlmapper` is explicitly bidirectional

This is one of the strongest findings in the bundle.

The abstract mapper class:

`com.amazon.yjhtmlmapper.f.c`

exposes both directions:

```text
HTML elements -> YJ properties
YJ properties -> HTML elements/CSS
```

The implementation is:

`com.amazon.yjhtmlmapper.e.k`

It builds two maps from `stylemap.ion`:

- HTML mapping key -> YJ property definition;
- YJ property definition -> HTML mapping key.

The reverse map is populated only when the record's `ignore_for_yj_to_html_mapping` flag is false.

The relevant logic in `com.amazon.yjhtmlmapper.e.k` is conceptually:

```text
forward[html_mapping] = yj_mapping
if !html_mapping.ignore_for_yj_to_html_mapping:
    reverse[yj_mapping] = html_mapping
```

The same class then implements the reverse conversion from a list of YJ properties to HTML/CSS using `DefaultStyleTransformer` or the named special transformer class.

The abstract mapper even contains a helper whose error text says:

`Multiple CSS properties for yj property (...). Please use getHTMLEquivalentProperties()`

This is direct evidence that the mapping layer was designed to answer YJ -> CSS questions, not only CSS -> YJ questions.

### `stylemap.ion` reverse-mapping flag

`com.amazon.yjhtmlmapper.e.d` parses every `stylemap.ion` record and explicitly reads:

`ignore_for_yj_to_html_mapping`

That value is stored in the HTML mapping-key object and controls whether the record is admitted to the YJ -> HTML reverse table.

This substantially changes the status of many style-conversion questions. For supported style properties, Amazon has shipped not just a forward mapping but a machine-readable declaration of which mappings are intended to be usable in reverse.

### YJ decompiler evidence

The same JAR ships localized resource bundles under:

`com/amazon/language/resources/yjdecompiler/`

The error strings describe a component explicitly called `YJDECOMPILER`, with operations/errors including style normalization, KFX validation, resource writing, EPUB creation, HTML-link resolution, reverse style mapping, source-OPF parsing, metadata handling, document-data access, Ion parsing, cover/spine updates, and style-template loading. The info resource contains:

`YJDECOMPILER_EPUB_FILE_CREATION_SUCCESSFUL = Generated Epub successfully : {0}`

The Previewer GUI contains the corresponding native orchestration layer. Instead of relying only on strings, the Mach-O `LC_FUNCTION_STARTS` table was decoded and the relevant x86_64 functions were bounded/disassembled directly. The current binary contains this coherent `DecompilerProcess` cluster:

| Address range | Recovered role |
| --- | --- |
| `0x10005d140..0x10005d2d0` | constructor-like initialization |
| `0x10005d2e0..0x10005d490` | destructor-like cleanup |
| `0x10005d4a0..0x10005d9c0` | `performDecompilerProcess` |
| `0x10005d9c0..0x10005e0c0` | `invokeDecompilerProcess` |
| `0x10005e0c0..0x10005e7c0` | parse/check decompilation log |
| `0x10005e7c0..0x10005f120` | construct argument list |
| `0x10005f120..0x10005fa80` | construct environment variables |
| `0x10005fa80..0x10005fda0` | execute external process invoker |

The role names are grounded by unique log strings inside the corresponding functions, including:

- `Decompilation of Epub is triggered..`;
- `triggering DecompilerProcess`;
- `Decompiler Process is completed`;
- `External Process Invocation(OOP call to YJDecompiler) completed with ExitCode : %d`;
- `DecompilerProcess::constructArgumentList - outputEpub = %s`;
- `DecompilerProcess::constructEnvironmentVars ...`.

#### Exact recovered decompiler launch contract

The argument-construction routine at `0x10005e7c0` takes, in addition to `this`:

- a generator string;
- an output directory;
- a temporary directory;
- an `isComixologyGenerated` boolean.

It constructs `<output-dir>/book.epub`, pushes the Java class name

`com.amazon.yj.decompiler.app.DecompilerApp`

and then constructs this application argument vector:

```text
<input-book.kdf>
<output-dir>/book.epub
<temp-dir>
--format
epub3
--generator
<generator>
--isComixologyGenerated
<true|false>
[--title <title>]
[--author <author1> <author2> ...]
```

The short option strings are not inferred from nearby text. The compiler encoded several of them as libc++ short-string immediates in the function body, and the bytes decode exactly to `--format`, `epub3`, `--generator`, `--isComixologyGenerated`, `--title`, and `--author`.

The first positional parameter can now be identified as the KDF path rather than merely guessed from context:

1. `DecompilerProcess` copies a `BookState` string through helper `0x100031f50`, which returns the field at `BookState + 0x70`, into the string later inserted as argv[0].
2. `PostPreviewOrchestrator` uses that same `BookState + 0x70` getter immediately before deciding whether to launch decompilation.
3. It passes the string to the file-type detector at `0x10006f5c0` and only follows this reverse-decompiler path when the detected type is `15`.
4. The underlying extension detector at `0x10006d270` checks exactly, in order:

```text
0  xht      4  opf      8  prc       12 docx
1  xhtml    5  epub     9  azw       13 rtf
2  htm      6  zip     10  azw3      14 kpf
3  html     7  mobi    11  doc       15 kdf
```

The associated type table stores the identity values `0..15`, so type 15 is unambiguously KDF.

Two other `BookState` fields are now identified by their direct use in the command:

- helper `0x100031b40` returns the shared string at `BookState + 0x40`; `invokeDecompilerProcess` passes it directly after `--generator`;
- helper `0x1000336f0` reads the byte at `BookState + 0x26d`; it is used to select `true` or `false` directly after `--isComixologyGenerated`.

`PostPreviewOrchestrator` also creates/joins a `decompilerTemp` path before the KDF type check and stores it back in `BookState + 0x18`. This is consistent with the temporary directory passed into `DecompilerApp`.

The environment builder is only partly named so far. It definitely constructs `<root>/lib` and `<root>/lib/shared_libs`, and logs `javaLibraryPath`, `sharedLibraryPath`, separator, old path, and new path. Several environment-key strings are held as global C++ `std::string` objects initialized at runtime rather than plain cstrings. Cross-comparison with the Kindle image-processing environment builder establishes at least one of those globals as `YJCONVERSION_ENV_ROOT`; other keys should remain unlabeled until their initializers are recovered.

#### Missing payload status

The launcher contract is therefore no longer the mystery. The missing piece is the implementation it expects to launch.

A byte-level search across the entire current Previewer application finds `com.amazon.yj.decompiler.app.DecompilerApp` only in the main executable. The class is absent from every JAR under `Contents/`, and the original application ZIP contains no additional decompiler JAR. `EpubToKFXConverter-4.0.jar` contains only the `yj­decompiler` language-resource bundle, not the entry-point class.

So the current evidence is:

- the GUI retains a complete KDF -> EPUB subprocess launcher;
- its class name and application argument contract are recoverable exactly;
- the corresponding Java payload is not shipped in this Previewer installation.

This narrows the remaining historical question substantially: was `DecompilerApp` present in an older Previewer/Kindle Create package, dynamically provisioned in an Amazon-internal build, or removed while the launcher code was left behind?

## Named property model: `$NNN` is not Amazon's semantic representation

The class-file path:

`com/amazon/kaf/c/b.class`

is a named enum decompiled by JADX as `com.amazon.kaf.c.EnumC4222b`.

Important caveat: on a case-insensitive filesystem a full JADX extraction collides with `com/amazon/kaf/c/B.class`. Decompile the lowercase class directly from the archive:

```sh
jadx \
  --single-class com.amazon.kaf.c.b \
  --single-class-output /tmp/kaf-property-enum.java \
  --no-res \
  'REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/lib/EpubToKFXConverter-4.0.jar'
```

The enum contains **854 constants**, numbered by Java ordinal from 0 through 853.

This is not merely a list of friendly labels adjacent to the real IDs. `com.amazon.kaf.jni.adapters.PropertyName` directly uses `EnumC4222b.ordinal()` as its stored property ID when a known named property is constructed. Therefore, for enum-backed properties, the enum ordinal is the KAF property ID.

Selected examples:

| ID | Amazon enum name | Amazon property string |
| ---: | --- | --- |
| 10 | `Language` | `language` |
| 11 | `FontFamily` | `font_family` |
| 145 | `Content` | `content` |
| 146 | `ContentList` | `content_list` |
| 259 | `Storyline` | referenced named storyline constant |
| 260 | `Section` | referenced named section constant |
| 261 | `StyleGroup` | `style_group` |
| 264 | `PositionMap` | `position_map` |
| 265 | `PositionIDMap` | referenced named position-ID-map constant |
| 266 | `Anchor` | `anchor` |
| 270 | `Container` | referenced named container constant |
| 271 | `Image` | `image` |
| 276 | `List` | `list` |
| 278 | `Table` | `table` |
| 391 | `NavigationContainer` | `nav_container` |
| 394 | `ConditionalNavigationUnit` | `conditional_nav_group_unit` |
| 605 | `WordIterationType` | `word_iteration_type` |
| 663 | `ConditionalProperties` | `yj.conditional_properties` |
| 664 | `SDLVersion` | referenced named SDL-version constant |
| 697 | `Dictionary` | `yj.dictionary` |
| 705 | `SourcePosition` | `source_position` |
| 706 | `TextOrientation` | `text_orientation` |
| 756 | `RubyContent` | referenced named ruby-content constant |
| 761 | `LayoutHints` | `layout_hints` |
| 762 | `RubyPositionHorizontal` | `ruby_position_horizontal` |
| 763 | `RubyPositionVertical` | `ruby_position_vertical` |
| 821 | `TableMetadata` | `table_metadata` |
| 853 | `BcSequenceNumber` | `bcSequenceNumber` |

The current KFX Input `yj_symbol_catalog.py` contains 843 shared-symbol placeholders covering `$10` through `$852`. The Previewer 3.106 enum has one additional enum-backed property at **853**, `bcSequenceNumber`, and no current kfxlib reference to `$853` was found in this checkout.

This is a concrete example of Previewer providing format information independently of the current KFX Input catalog.

### Historical Go catalog versus the live Amazon table

The old Go branch contains `internal/kfx/catalog.ion`, whose symbol list covers property IDs 10 through 851. It can now be checked against Amazon directly rather than treated as historical reverse-engineering lore.

The live 854-entry table was dumped from Previewer 3.106 through `PropertyNameUtil.a()` and compared by numeric ID with `internal/kfx/catalog.ion`:

```text
historical Go catalog symbols: 842   (IDs 10..851)
exact ID/name matches:         842
mismatches:                      0
current live additions:          2
  852  page_regions
  853  bcSequenceNumber
```

That is an exact **842/842** match in both ordering and spelling. For the vocabulary covered by that branch, the old Go catalog was not merely approximately right; it agrees byte-for-byte at the semantic-name level with the current Amazon KAF table. The observed current drift is append-only across these two newer entries.

This does not prove Amazon can never renumber or revise the table, but it gives us a useful update strategy: compare the live KAF catalog on each Previewer revision and treat new suffix entries as explicit format deltas rather than rediscovering the whole symbol space.

### Property 852 `page_regions`: a real current renderer consumer

Property 852 is not merely a newly appended name in the symbol catalog. The current Previewer renderer contains a direct static consumer.

The cstring

`readPageRegions: resolved %zu regions (fixedW=%d, fixedH=%d, renderedW=%.1f, renderedH=%.1f)`

is referenced from the function at `0x100cf2f2c..0x100cf38aa`. In that function, the generic YJ property accessor is called with immediate property IDs that map through the live KAF table to:

```text
66   fixed_width
67   fixed_height
852  page_regions
247  entries
58   top
59   left
56   width
57   height
761  layout_hints
```

The type checks and nested iteration recover the shape of the data quite closely. `page_regions` is expected to be a property-list/structure-like value; its `entries` member is a list; each list item is another structure with rectangle fields and an optional list of layout hints. In named form the reader is effectively consuming:

```text
fixed_width:  <positive number>
fixed_height: <positive number>
page_regions: {
  entries: [
    {
      top:          <number>,
      left:         <number>,
      width:        <positive number>,
      height:       <positive number>,
      layout_hints: [<enum/value>, ...]   # optional
    },
    ...
  ]
}
```

The renderer then scales each region from the fixed-page coordinate system into the actual rendered dimensions. The arithmetic in the function is explicit:

```text
rendered_left   ~= left   / fixed_width  * rendered_width  + page_x_offset
rendered_top    ~= top    / fixed_height * rendered_height + page_y_offset
rendered_width  ~= width  / fixed_width  * rendered_width
rendered_height ~= height / fixed_height * rendered_height
```

It constructs a region-state object for each valid positive rectangle and preserves the `layout_hints` list into that state. A separate renderer path logs:

`Page::rerenderIfNeeded: Page:%p Model(%p) was valid but PageRegions was empty, this may result in blank pages`

so these regions affect actual page rendering rather than being inert metadata.

This is important for two reasons:

1. it is the first direct semantic use recovered for one of the new suffix properties that current KFX Input does not name; and
2. the current basic fixed-layout fixture does **not** emit `page_regions`, so the property is tied to a narrower producer/content mode than ordinary pre-paginated layout.

The trigger remains open. Likely targets for controlled fixtures are document-region/shape-heavy content, comics, guided-view/panel content, and newer fixed-layout authoring paths. Those should be tested rather than assuming the exact profile from the renderer alone.

Property 853 `bcSequenceNumber` remains different: it is present in Amazon's current property tables, but no comparable semantic consumer has yet been recovered. It should not be assigned a meaning beyond its Amazon-provided name until an actual read/write path is found.

### Direct comparison to current kfxlib

Current KFX Input semantic code uses the raw IDs directly. For example:

- `$145` appears throughout fragment/content handling; Amazon names ID 145 `Content`.
- `$146` is used as nested content; Amazon names ID 146 `ContentList`.
- `$259` is handled as storyline fragments; Amazon names it `Storyline`.
- `$260` is handled as section fragments; Amazon names it `Section`.
- `$264` is the position-map fragment; Amazon names it `PositionMap`.
- `$270` is a core container/content type; Amazon names it `Container`.
- `$391` is navigation-container data; Amazon names it `NavigationContainer`.
- `$394` is checked under the string `conditional_nav_group_unit`; Amazon's enum calls it `ConditionalNavigationUnit`.
- `$605` is assigned to `word_iteration_type` in kfxlib; Amazon names it `WordIterationType`.
- `$663` is assigned to conditional-properties processing; Amazon names it `ConditionalProperties`.
- `$756` is ruby content; Amazon names it `RubyContent`.
- `$761` is exposed by kfxlib as `-kfx-layout-hints`; Amazon names it `LayoutHints`.
- `$762/$763` are ruby-position properties; Amazon names the horizontal and vertical forms directly.
- `$821` is treated as table metadata; Amazon names it `TableMetadata`.

This is strong evidence that many anonymous `$NNN` branches can be renamed and understood without speculation.

A source-wide count makes that point much stronger. Excluding `yj_symbol_catalog.py` itself, the current `kfxlib` checkout contains:

- **604 unique numeric `$NNN` IDs** used by actual implementation code;
- **2,463 total numeric-ID occurrences**;
- minimum used ID 10, maximum used ID 851;
- **604 / 604 of those used IDs have names in Previewer 3.106's live KAF property map**.

In other words, for the current checkout there is not a single numeric shared-property ID used by kfxlib's implementation that the current Previewer KAF runtime cannot name. The most frequently occurring raw IDs include:

```text
$146  content_list
$164  external_resource
$270  container
$258  metadata
$259  storyline
$492  key
$159  type
$597  auxiliary_data
$417  bcRawMedia
$157  style
$155  id
$307  value
$260  section
$145  content
$608  structure
$176  story_name
$165  location
$156  layout
$175  resource_name
$391  nav_container
```

This does not automatically tell us the semantics of every *combination* or value, but it eliminates the need to leave the vocabulary anonymous throughout semantic code.

## Runtime KAF probe: the native property map is directly callable

Static analysis suggested that the enum ordinal and KAF property ID were the same. We then validated that conclusion against Amazon's own native runtime rather than relying on decompiler interpretation.

A standalone Java harness can load the KAF JNI implementation directly from the Previewer bundle:

```java
import com.amazon.kaf.jni.adapters.c;
import com.amazon.kaf.util.PropertyNameUtil;
import com.amazon.kaf.c.ab;
import java.util.*;

public class KafProbe {
    public static void main(String[] args) throws Exception {
        c.a();
        List<ab> props = PropertyNameUtil.a();
        props.sort(Comparator.comparingLong(ab::d));
        System.out.println("props=" + props.size());
        for (ab p : props) {
            System.out.println(p.d() + "\\t" + p.a());
        }
    }
}
```

Compile and run it against the bundled JAR/native library:

```sh
JAR='REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/lib/EpubToKFXConverter-4.0.jar'
LIB='REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/lib'

javac -cp "$JAR" /tmp/KafProbe.java
java \
  -Dklibname=shared \
  -Djava.library.path="$PWD/$LIB" \
  -cp "$JAR:/tmp" \
  KafProbe
```

Observed result:

```text
KAF loaded
props=854
0       null
1       $ion
2       $ion_1_0
3       $ion_symbol_table
4       name
5       version
6       imports
7       symbols
8       max_id
9       $ion_shared_symbol_table
10      language
11      font_family
...
145     content
146     content_list
259     storyline
260     section
264     position_map
270     container
391     nav_container
605     word_iteration_type
663     yj.conditional_properties
697     yj.dictionary
756     ruby_content
821     table_metadata
...
852     page_regions
853     bcSequenceNumber
```

This is stronger evidence than the enum alone:

- `PropertyNameUtil.a()` is backed by the native `getPropertyNameIndexMap()` JNI method;
- the native KAF implementation reports exactly 854 property names;
- the live name/index map agrees with the enum for the checked IDs;
- the runtime itself reports property 853 as `bcSequenceNumber`.

The KAF JNI runtime can therefore be used as an independent semantic oracle from a small standalone harness. The next step is to move from property enumeration to opening a generated book and traversing the typed object graph.

## KAF is a typed book object model

Amazon's Java KAF wrappers live primarily under:

`com.amazon.kaf.jni.adapters`

The exposed object vocabulary includes:

- `DigitalBook`
- `BookContent`
- `BookFactory`
- `ObjectFactory`
- `ReadingOrder`
- `Section`
- `Storyline`
- `Container`
- `ContentList`
- `Style`
- `StyleEvent`
- `Resource`
- `EmbeddedFont`
- `Anchor`
- `Position`
- `Offset`
- `BookPositionInfo`
- `NavigationContainer`
- `NavigationProvider`
- `NavigationUnit`
- `PageTemplate`
- `RubyContent`
- `PathBundle`
- `DocumentData`
- and several specialized media/interactive/container types.

This object model is important because it demonstrates how Amazon conceptualizes the document above the serialized Ion layer. The `$NNN` IDs are not the conceptual model; they are property/element identifiers used by the model and serialization system.

### JNI bootstrap

`com.amazon.kaf.jni.adapters.c` loads the native library with:

```text
System.loadLibrary("KAFJNI-shared")
```

unless the `klibname` system property overrides the library name. Previewer's process launcher configures the Mac build to load `shared`, corresponding to the bundled `libshared.dylib`.

`com.amazon.adapter.common.wrapper.ConversionEngine` initializes this KAF bridge before performing the conversion.

### Useful `DigitalBook` operations

`DigitalBook` exposes native operations for:

- reading orders;
- navigation providers;
- metadata;
- content features and versions;
- book position information;
- storage export/clone;
- save/archive;
- symbol lookup;
- string lookup.

Particularly useful for reverse engineering are the bidirectional symbol APIs:

```text
nativeGetSymbolID(String)
nativeGetSymbolName(long)
```

`BookContent` exposes typed retrieval of resources, sections, auxiliary data, storylines, ruby content, containers, styles, fonts, and path bundles.

`ObjectFactory` exposes creation of many of those same typed objects.

This makes KAF a potential **semantic introspection oracle** for generated or loadable YJ/KDF data, not merely a library that Previewer happens to use internally.

## Native KAF findings from Ghidra

The copied `REFERENCE/ghidra` CLI works against the local Ghidra installation and was used for this pass.

### Setup

```sh
./REFERENCE/ghidra doctor

./REFERENCE/ghidra project create kindle_previewer_kaf

./REFERENCE/ghidra import \
  REFERENCE/ghidra_analysis/libshared/libshared.x86_64.dylib \
  --project kindle_previewer_kaf \
  --program libshared_x86_64 \
  --detach
```

The analyzed program contains roughly 23,000 recognized functions after Ghidra analysis.

Use explicit project/program selection for queries:

```sh
./REFERENCE/ghidra \
  --project kindle_previewer_kaf \
  --program libshared.x86_64.dylib \
  query functions -f 'name~nativeGetSymbolName' \
  --fields name,address,size,signature --format table
```

### `DigitalBook_nativeGetSymbolName`

Ghidra identified:

`_Java_com_amazon_kaf_jni_adapters_DigitalBook_nativeGetSymbolName`

at approximately `0x0000c2f0` in the analyzed x86_64 slice.

The decompiled JNI wrapper:

1. resolves the underlying C++ KAF object from the Java handle;
2. calls a virtual method at a KAF-object vtable slot near offset `0x108`, passing the numeric symbol ID and a C++ string output;
3. reports `Failed to get Symbol Name` on error;
4. converts the resulting C++ string to a Java string.

The adjacent `nativeGetSymbolID` wrapper performs the reverse operation using the neighboring virtual slot near `0x100`.

This independently confirms that the native KAF object itself has bidirectional symbol lookup.

### `BookFactory_nativeGetBook(String)`

The JNI wrapper converts the Java path/string, invokes a native helper that returns a shared KAF book object, and returns its handle to Java as a `DigitalBook` when successful.

This is the path to investigate for using KAF as a loader/introspection oracle.

### `BookContent_getNativeStoryline`

The wrapper resolves a `BookContent` native object and calls a virtual method near vtable offset `0x58` to retrieve the requested storyline object.

This confirms that `Storyline` is not merely a Java-side naming convenience layered on raw Ion. It is part of the native KAF object interface.

### `Storyline_getNativeContentList`

The JNI wrapper obtains the native storyline object and then invokes a native virtual operation near `0x218` to obtain/copy the content-list handle.

Again, `ContentList` exists as an explicit KAF-native concept.

### `DBStorage_updateIonSymbolTableFragmentNative`

This JNI function:

1. receives a byte payload from Java;
2. wraps/copies it into a native buffer object;
3. invokes a KAF storage virtual method near offset `0x200`;
4. reports `Failed to update symbol table fragment!` on failure.

This ties the high-level storage API directly to Ion symbol-table fragment handling.

### `DigitalBook_nativeSave(boolean, boolean)`

The JNI wrapper invokes a native KAF virtual method near offset `0xa0` with the two boolean save options and reports `Failed to save the book` on failure.

The Java generator eventually invokes the corresponding save path when `Write YJ Document To Disk` runs.

### Native type information

Despite stripping, `libshared.dylib` retains C++ RTTI/type strings including classes equivalent to:

- `kaf::KAFDigitalBook`
- `kaf::KAFBookContent`
- `kaf::KAFStoryline`
- `kaf::KAFContainer`
- `kaf::KAFContentList`
- `kaf::KAFBookPositionInfo`
- `kaf::KAFRubyContent`
- `yjsdk::DigitalBook`
- `yjsdk::BookContent`
- `yjsdk::SymbolID`

Build-path strings also expose names such as `YJReaderSDK`, `YJCommonsIO`, and `Turboshaft`.

The important conclusion is not the exact internal class hierarchy yet. It is that the Java KAF vocabulary corresponds to a substantial native C++ KAF/YJSDK model rather than a cosmetic wrapper around anonymous records.

## Machine-readable semantic/style data

The `.ion` files were decoded by gunzipping them and using the current kfxlib Ion reader only as a generic binary-Ion parser. Their semantic meaning is then read from Amazon's own field names and from the Java code that consumes them.

A reproducible skeleton is:

```sh
PYTHONPATH=REFERENCE/KFX_Input python3 - <<'PY'
import gzip
from pathlib import Path
from kfxlib.ion_binary import IonBinary
from kfxlib.ion_symbol_table import LocalSymbolTable

base = Path('REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/data')
for name in ['semantics.ion', 'semanticmap.ion', 'stylemap.ion',
             'stylelist.ion', 'template-properties.ion']:
    raw = gzip.decompress((base / name).read_bytes())
    values = IonBinary(LocalSymbolTable()).deserialize_multiple_values(
        raw, import_symbols=True)
    print(name, len(values))
PY
```

### `stylemap.ion`

Observed:

- decompressed size: ~74 KB;
- 943 mapping records;
- 143 unique YJ property names in the mapping records;
- 22 distinct converter/transformer classes referenced;
- 50 records set `ignore_for_yj_to_html_mapping`.

Representative transformer classes named directly by the data include:

- `XYStyleTransformer`
- `UserAgentStyleAddingTransformer`
- `BorderRadiusTransformer`
- `TextDecorationTransformer`
- `ImageBorderTransformer`
- `BGColorTransformer`
- `MarginAutoTransformer`
- `BGRepeatTransformer`
- `BoxShadowTransformer`
- `TextShadowTransformer`
- `LanguageTransformer`
- `WidowsOrphansTransformer`
- `WritingModeTransformer`
- `TextCombineTransformer`
- `LineHeightTransformer`
- `ShapeOutsideTransformer`
- `MaxCropPercentageTransformer`
- `PageBleedTransformer`
- `TextEmphasisStyleTransformer`
- `LinkStyleTransformer`

The mappings include ordinary CSS properties and Amazon-specific properties, units, conversion factors, element/tag constraints, and optional special transformer classes.

Examples include explicit mappings for:

- heading elements to `yj.semantics.heading_level`;
- EPUB note semantics;
- ruby position into horizontal/vertical ruby YJ properties;
- text orientation;
- bidi properties;
- background/fill properties;
- border radii;
- shadows;
- text emphasis;
- page-margin/page-align behavior.

The Java loader for this file is `com.amazon.yjhtmlmapper.e.d`.

### `stylelist.ion`

This file configures style-merger behavior rather than individual CSS-property translation.

The decoded records select merger classes including:

- `YJCumulativeRuleMerger`
- `YJOverridingRuleMerger`
- `YJOverrideMaximumRuleMerger`
- `YJCumulativeInSameContainerRuleMerger`
- `YJRelativeRuleMerger`
- `YJBaselineStyleRuleMerger`
- `YJHorizontalPositionRuleMerger`

This is especially relevant to any attempt to understand or replace kfxlib's style simplification/inheritance behavior. Amazon has explicit property-family-specific rules for how YJ style state accumulates or overrides across the document tree.

### `semantics.ion`

The decoded file contains 29 semantic entries.

Examples include entries for concepts such as:

- `KBookTitle`
- `KChapterTitle`
- `KBlockQuote`
- `KHeroImage`
- `KBylineAuthor`
- `KBodyMatter`

The records include EPUB-semantic values and flags/attributes used by the converter, including navigation/NCX-related behavior for some semantics.

### `semanticmap.ion`

This contains a small set of explicit EPUB-semantic -> YJ-semantic mappings, including mappings for concepts such as:

- title;
- body matter/body text;
- byline/author;
- block quote;
- introduction;
- hero image;
- breadcrumb.

The mappings target the `yj.semantics` namespace.

### `template-properties.ion`

This file contains named template/style defaults and processing metadata, including references to:

- Bookerly;
- Amazon Ember;
- separator glyphs;
- drop-cap font families;
- template variants such as `KTClassic-1.0`, `KTModern-1.0`, etc.;
- processors including `resolveProductWidget` and `enhanceSemantic`.

### `mapping_ignorable_patterns.ion`

This is consumed by `com.amazon.yjhtmlmapper.e.a` and acts as a pattern database for source HTML/CSS attributes or styles that the mapper should ignore/tolerate. The loader supports wildcard pattern matching over tag/style/value/unit tuples.

This is useful when distinguishing an actually unsupported style from source noise that Amazon's own pipeline intentionally discards.

## Controlled end-to-end fixture: EPUB -> Amazon YJ/KDF -> typed KAF graph

The producer and KAF runtime are not merely statically inspectable. A controlled fixture was successfully converted and then reopened through Amazon's own KAF object model without launching the Previewer GUI.

### Minimal input

A tiny EPUB was created containing essentially:

```html
<h1>Hello</h1>
<p id="p1">KFX probe.</p>
```

with:

```css
h1 { color: red; }
p { margin-top: 1em; }
```

### Direct producer invocation

The useful entry point is `com.amazon.adapter.common.app.EpubAdapterApp`, with its three positional arguments followed by the normal converter switches. The environment below mirrors `com.amazon.kfxconverter.process.d` closely enough for a successful conversion:

```sh
FC="$PWD/REFERENCE/Kindle Previewer 3.app/Contents/lib/fc"

mkdir -p /tmp/yjout /tmp/yjtmp

env \
  "Path=${PATH}:$FC/lib" \
  "phantomjs_home_dir=$FC" \
  "js_scripts_home_dir=$FC" \
  "semantic_mapping_dir=$FC/" \
  "style_mapping_dir=$FC" \
  "style_merger_dir=$FC/" \
  "yj_character_fixer_base_dir=$FC" \
  "yjhtmlcleaner_path=$FC/bin/htmlcleanerapp" \
  "CSS_HOME_DIR=$FC/rasterfonts" \
  "YJCONVERSION_ENV_ROOT=$FC" \
  "MERGED_JAR_FILE_PATH=$FC/lib/EpubToKFXConverter-4.0.jar" \
  "DYLD_LIBRARY_PATH=$FC/lib" \
  "YJ_ASCII_UNICODE_CONVERTER_DATA_DIR=$FC" \
  "$FC/jre/bin/java" \
  -Dfile.encoding=UTF-8 \
  -Djava.awt.headless=true \
  -Djava.library.path="$FC/lib" \
  -Dklibname=shared \
  -cp "$FC/lib/*" \
  com.amazon.adapter.common.app.EpubAdapterApp \
  /tmp/kp3-probe.epub /tmp/yjout /tmp/yjtmp \
  --write-to-db --persist-yj --do-graceful-error-handling \
  --log-level WARNING
```

Observed stdout:

```text
Successfully converted the file: /tmp/kp3-probe.epub
```

The producer generated:

```text
/tmp/yjout/book/book.kdf
/tmp/yjout/book/book.kdf-journal
/tmp/yjout/book/ManifestFile
/tmp/yjout/misc/...
/tmp/yjtmp/conversion.log
/tmp/yjtmp/conversionReport.ion
/tmp/yjtmp/errorInfo.json
/tmp/yjtmp/featureInfo.json
/tmp/yjtmp/metrics.json
/tmp/yjtmp/preprocessed/...
```

This gives us a repeatable way to manufacture feature-focused YJ/KDF fixtures from source EPUBs.

### Opening the generated KDF through KAF

A standalone Java probe then loaded the same native `libshared.dylib`, called `BookFactory.nativeGetBook(String)` through the public wrapper, and opened:

`/tmp/yjout/book/book.kdf`

The KAF runtime reported one reading order and the following non-empty object categories:

```text
AuxiliaryData=[c0-ad, dC]
Structure=[i3, i5, i7, t1]
NavigationContainer=[n9]
Storyline=[l2, l4]
Section=[c0]
Anchor=[aA, aB]
Style=[s6, s8]
```

The document-data object exposed named, typed properties directly:

```text
writing_mode        kElemType   horizontal_tb
 direction           kElemType   ltr
column_count         kElemType   auto
font_size            kFloatEm    1.0
selection            kElemType   enabled
auxiliary_data       kPropList   {yj.conversion = symbol "dC"}
max_id               kInt        13
line_height          kFloatEm    1.2
language             kString     "en"
spacing_percent_base kElemType   width
```

(The leading space before `direction` above is only presentation; the actual property name is `direction`.)

### Source HTML becomes a typed semantic graph

The populated storyline `l4` contained two `Structure` objects with KAF container type `TEXT`.

For the source `<h1>Hello</h1>` object, KAF reported:

```text
kfx_id                     -> symbol "i5"
style                      -> symbol "s6"
yj.semantics.heading_level -> 1
type                       -> text
text                       -> "Hello" (YJ_UTF8, 5 bytes, 5 characters)
```

Style `s6` contained:

```text
font_size    -> 2.0 rem
layout_hints -> [treat_as_title]
line_height  -> 1.0 lh
text_color   -> 4293787648
style_name   -> symbol "s6"
font_weight  -> bold
```

For the source paragraph, KAF reported:

```text
kfx_id -> symbol "i7"
style  -> symbol "s8"
type   -> text
text   -> "KFX probe." (YJ_UTF8, 10 bytes, 10 characters)
```

Style `s8` contained normalized paragraph properties including:

```text
font_size     -> 1.0 rem
margin_bottom -> 0.8333333 lh
line_height   -> 1.0 lh
margin_top    -> 1.1166667 lh
```

The exact normalized values are interesting in their own right, but the larger result is more important: **we can now observe Amazon's typed semantic result for a controlled source feature directly**.

That gives us a practical experimental loop:

```text
minimal EPUB feature
        |
        v
EpubAdapterApp
        |
        v
book.kdf
        |
        +--> raw Ion/SQLite/storage inspection
        |
        +--> KAF typed graph inspection
        |
        +--> Previewer rendering
        |
        v
KFX Input inverse comparison
```

This is substantially better than relying on a small collection of arbitrary books for core semantic questions. A real-world corpus is still needed for historical/consumer variants, but canonical feature behavior can now be tested one variable at a time.

## Second controlled fixture: vertical Japanese text, ruby, and text emphasis

A second synthetic EPUB exercised three areas that are traditionally difficult to infer from arbitrary books:

- Japanese vertical writing;
- ruby annotation;
- text emphasis.

The source contained:

```html
<h1>見出し</h1>
<p><ruby><rb>漢</rb><rt>かん</rt></ruby>字と<span class="emph">強調</span>。</p>
```

with CSS equivalent to:

```css
html, body { writing-mode: vertical-rl; }
ruby { ruby-position: over; }
.emph { text-emphasis-style: filled dot; }
```

and Amazon's EPUB metadata form:

```html
<meta name="primary-writing-mode" content="vertical-rl"/>
```

### The metadata really matters

An initial fixture omitted the `primary-writing-mode` meta element. Even though the source CSS used `writing-mode: vertical-rl`, the generated KAF `DocumentData` reported:

```text
writing_mode = horizontal_tb
```

and the converter emitted a warning that `primary-writing-mode` was not available.

After adding:

```html
<meta name="primary-writing-mode" content="vertical-rl"/>
```

the warning disappeared and the generated document reported:

```text
writing_mode [kElemType] = vertical_rl
language     [kString]   = "ja"
```

This gives a controlled proof of the distinction between page/document writing-mode metadata and ordinary element CSS in Amazon's producer.

### Amazon's ruby parser requires explicit `rb`

The first ruby attempt used the common HTML5 shorthand:

```html
<ruby>漢<rt>かん</rt></ruby>
```

Amazon's converter emitted:

```text
Unsupported ruby child tag: Ruby tag has an unsupported child tag:
```

The reason is visible directly in:

`com.amazon.adapter.common.l.a.b.b`

Its ruby-child parser walks every non-comment DOM child and accepts only element names `rb` and `rt`. A raw text node becomes an unsupported child. With explicit markup:

```html
<ruby><rb>漢</rb><rt>かん</rt></ruby>
```

conversion succeeds without that error.

This is a useful example of why Previewer source-adjacent logic is better than guessing from observed output: the exact accepted ruby grammar is directly visible.

### Produced KAF styles

The generated KAF styles included:

```text
ruby_position_horizontal = top
ruby_position_vertical   = right
text_emphasis_style      = filled_dot
```

These agree with `stylemap.ion`, whose relevant records map:

```text
ruby-position: over/before -> top,right
ruby-position: under/after -> bottom,left
```

and map text-emphasis properties through `TextEmphasisStyleTransformer`.

### Exact generated ruby fragments

For this fixture the generated KDF can also be decoded at the raw-Ion layer. The relevant fragments are:

```text
style sF:
  ruby_position_horizontal = top
  ruby_position_vertical   = right

style sG:
  text_emphasis_style = filled_dot

structure iA:
  kfx_id  = iA
  style   = sB
  ruby_id = 1
  type    = text
  content = "かん"

ruby_content b9:
  ruby_name    = b9
  content_list = [iA]

structure i7:
  kfx_id = i7
  style  = s8
  type   = text
  content = "漢字と強調。"
  style_events = [
    {
      offset: 0,
      length: 1,
      style: sF,
      ruby_name: b9,
      ruby_id: 1
    },
    {
      offset: 3,
      length: 2,
      style: sG
    }
  ]
```

The raw numeric form of the same data uses:

```text
$142 style_events
$143 offset
$144 length
$145 content
$146 content_list
$157 style
$159 type
$269 text
$717 text_emphasis_style
$726 filled_dot
$756 ruby_content
$757 ruby_name
$758 ruby_id
$762 ruby_position_horizontal
$763 ruby_position_vertical
```

Every one of those names comes from the live Previewer KAF property map rather than from a hand-assigned local glossary.

The structure is consequently quite comprehensible when named:

- base text lives in the parent text structure;
- ruby pronunciation text is a separate `ruby_content` object containing text structure(s);
- the base-character range is linked to the ruby object by a style event carrying `ruby_name` + `ruby_id`;
- emphasis is another range style event.

That is a much cleaner semantic explanation than reading the corresponding raw `$142/$757/$758/...` structures in isolation.

### Current KFX Input is already one symbol behind current Previewer

Wrapping this generated KDF in a simple ZIP and asking the current KFX Input decoder to read it produces this warning before any semantic conversion:

```text
Import symbol table YJ_symbols version 10 max_id 844(+9=853)
exceeds known table size 843(+9=852)
```

It later reports:

```text
Unknown symbols: max_id=853
```

This is not a hypothetical version skew. Previewer 3.106's live KAF map contains property 853 (`bcSequenceNumber`), while the current `REFERENCE/KFX_Input/kfxlib/yj_symbol_catalog.py` stops at `$852`.

The synthetic fixture does not itself appear to use property 853 in its semantic fragments, so this mismatch does not imply a visible conversion bug in this fixture. It does establish that **current Amazon producer vocabulary is already ahead of the current KFX Input built-in catalog**.

That is exactly the sort of change a Previewer-derived compatibility audit can surface immediately instead of waiting for a user book to fail.

## Controlled table fixture: hierarchy, spans, and table policy

A third reflowable specimen was added specifically to test table semantics. Its source contains:

```html
<table>
  <caption>Probe table</caption>
  <thead><tr><th>H1</th><th>H2</th></tr></thead>
  <tbody>
    <tr><td rowspan="2">A</td><td>B</td></tr>
    <tr><td>C</td></tr>
    <tr><td colspan="2">D</td></tr>
  </tbody>
</table>
```

Amazon's typed KAF graph makes the structural interpretation explicit. The table root is a `TABLE` container with:

```text
type                    = table
yj.table_features       = [pan_zoom, scale_fit]
yj.table_selection_mode = yj.regional
table_border_collapse   = false
border_spacing_vertical   = 0.9 pt
border_spacing_horizontal = 0.9 pt
```

The source hierarchy is retained semantically:

```text
TABLE
├── caption-classified text
├── HEADER
│   └── TABLE_ROW
│       ├── H1 cell/text
│       └── H2 cell/text
└── BODY
    ├── TABLE_ROW
    │   ├── A cell/text
    │   └── B cell/text
    ├── TABLE_ROW
    │   └── C cell/text
    └── TABLE_ROW
        └── D cell/text
```

The caption gets both `yj.classification = caption` on its outer text structure and `layout_hints = [caption]` on the corresponding style.

An especially useful detail is where cell spans live. They are not direct properties on the row container. The producer places them on the cell's generated style:

```text
A-cell style:
  table_row_span = 2

D-cell style:
  table_column_span = 2
```

The raw-Ion decode agrees exactly:

```text
$149 table_row_span    = 2
$148 table_column_span = 2
```

This is another concrete warning against deriving a cleaner model solely by looking at source HTML: some semantics that appear structurally attached to an element in EPUB are serialized as style properties in YJ. A native decoder can normalize them into a cleaner internal table model, but the wire/model boundary needs to preserve where Amazon actually stores them.

The simple table does **not** emit `$821 table_metadata`; that property is therefore not required for an ordinary HTML table with spans. Its trigger should be investigated separately, likely with large-table/viewer-specific or richer table fixtures rather than assuming every table has it.

## Controlled footnote fixture: note link, classification, and anchor target

A fourth reflowable specimen isolates EPUB 3 footnote semantics:

```html
<p>Main text<a epub:type="noteref" href="#fn1">1</a>.</p>
<aside epub:type="footnote" id="fn1"><p>Footnote text.</p></aside>
```

Amazon turns the visible noteref character into a range style event rather than a dedicated structural child. In the typed KAF graph the main text is `Main text1.` and the one-character reference range carries:

```text
offset     = 9
length     = 1
yj.display = yj.note
link_to    = aA
style      = s9
```

The note body is a separate text structure with:

```text
yj.classification = footnote
position           = footer
type               = text
content            = "Footnote text."
```

The generated target anchor is explicit in raw Ion:

```text
anchor aA:
  anchor_name = aA
  position = {
    id     = i7
    offset = 0
  }
```

The same representation in numeric shared-symbol form is:

```text
$142 style_events
$143 offset
$144 length
$157 style
$179 link_to
$183 position
$266 anchor
$281 footnote
$455 footer
$615 yj.classification
$616 yj.display
$617 yj.note
```

This gives a clean canonical model for an ordinary EPUB 3 footnote: the source noteref becomes a styled/linking text range, the note itself becomes footer-classified footnote content, and the link resolves through a normal YJ anchor to the note structure. It is now possible to compare kfxlib's note reconstruction against Amazon's producer semantics feature-by-feature instead of inferring the whole note model from consumer books.

## Reusable controlled-oracle harness

The one-off producer/KAF experiments have been turned into repo-local research tooling under `scripts/kp3/`. This is intentionally separate from plugin runtime code.

Current pieces:

```text
scripts/kp3/
├── compare_catalog.py
├── make_fixture.py
├── run_probe.py
└── com/amazon/kaf/jni/adapters/
    ├── KafPropertyCatalog.java
    └── KafSemanticProbe.java
```

`make_fixture.py` currently provides nine deliberately small source fixtures:

- `minimal`: one H1 and one paragraph;
- `footnote`: EPUB 3 `noteref` + `footnote` semantics and generated anchor targeting;
- `table`: caption, header/body rows, `rowspan`, and `colspan`;
- `fixed-layout`: a minimal pre-paginated page used to probe current fixed-layout producer behavior;
- `vertical-ruby`: Japanese vertical-writing metadata, explicit `rb`/`rt` ruby, ruby-position styling, and text emphasis.
- `link`: a same-document anchor link and target heading;
- `bidi`: RTL paragraph direction plus an isolated LTR range;
- `list`: ordered-list start offset plus a nested unordered list;
- `svg`: simple inline SVG used to observe current producer normalization.

`run_probe.py` performs the entire experiment:

```text
controlled EPUB
     |
     v
Amazon EpubAdapterApp
     |
     +--> wrapped book.kdf
     |       |
     |       +--> fingerprint removal --> stock SQLite schema/fragment dump
     |       |
     |       +--> bundled native KAF --> typed semantic graph
     |
     +--> Amazon conversion logs / preprocessed source
```

It uses the exact JAR, bundled JRE, and native `libshared` from `REFERENCE/Kindle Previewer 3.app`; compiles the small JNI probes with `javac --release 11`; and deliberately runs KAF as a one-shot subprocess because exploratory native getters have shown lifetime/ownership hazards.

Example:

```sh
./scripts/kp3/run_probe.py \
  --fixture vertical-ruby \
  --workdir /tmp/kp3-vertical-ruby
```

The current vertical-ruby probe deterministically reports, among other values:

```text
writing_mode = vertical_rl
ruby_position_horizontal = top
ruby_position_vertical = right
text_emphasis_style = filled_dot

styleEvent offset=kScalar:0 length=kScalar:1 props=3
  style = sF
  ruby_name = b9
  ruby_id = 1

styleEvent offset=kScalar:3 length=kScalar:2 props=1
  style = sG
```

The `minimal`, `footnote`, `table`, `fixed-layout`, `vertical-ruby`, `link`, `bidi`, `list`, and `svg` fixtures have all been run through the checked-in harness successfully. The minimal KDF also reproduces the expected single fingerprint record and three-table SQLite schema. The current simple fixed-layout specimen is normalized to a scale-fit container containing an image and does **not** emit `page_regions`; that negative result is useful because it shows property 852 is not a generic fixed-layout requirement.

The harness can additionally dump the current live Amazon property catalog:

```sh
./scripts/kp3/run_probe.py --fixture minimal --catalog
```

`compare_catalog.py` makes the historical-Go/live-Amazon symbol check reproducible; on Previewer 3.106 it reports 842 shared IDs, 842 exact matches, zero mismatches, plus live IDs 852 and 853.

This changes the practical corpus problem. A random corpus is still needed for historical, malformed, DRM-adjacent, publisher-specific, and consumer-delivered variants, but it is no longer the only way to learn canonical semantics. For current producer behavior we can manufacture a one-feature specimen, ask Amazon to compile it, and inspect both its raw and typed representations.

The next useful corpus should therefore be a **semantic fixture matrix**, not merely a larger pile of arbitrary books: one controlled input for tables, footnotes, fixed layout, page spread, conditional content, images, SVG/KVG, navigation, links, drop caps, bidi, writing modes, and so on. Real books then become compatibility/adversarial cases layered on top of an explicit canonical baseline.

### Differential reverse harness: Amazon-generated fixtures expose gaps the ten-book corpus missed

The KDF fixtures can also be turned into inputs for the historical Go reverse implementation. `scripts/kp3/reverse_compare.py` now automates this path:

```text
controlled EPUB
    |
    v
Amazon EpubAdapterApp
    |
    v
book.kdf + resources
    |
    v
minimal KPF ZIP
    |
    |  current KFX Input decodes KDF and serializes fragments only
    v
single unencrypted CONT KFX
    |                         |
    v                         v
current Python KFX Input      historical Go port
    |                         |
    v                         v
python.epub                 go.epub
    \_________________________/
                 |
                 v
          structural diff
```

The Python serializer is deliberately only the storage bridge. Both reverse implementations receive the **same KFX bytes**, so differences after that point are reverse-conversion parity differences.

This immediately found behavior outside the old ten-book corpus, despite that corpus having reached zero structural differences before the branch was abandoned.

Current Previewer 3.106 results for the first nine fixtures are:

| Fixture | Go reverse result | Non-timestamp structural diffs | Important semantic difference |
| --- | --- | ---: | --- |
| `minimal` | converts | 3 | fallback metadata/TOC behavior differs |
| `footnote` | converts | 5 | Go loses the footnote `<aside epub:type="footnote">` wrapper/style |
| `table` | converts | 4 | Go loses the `<table>` wrapper and leaves `thead`/`tbody` directly under `body` |
| `fixed-layout` | **fails** | n/a | Go reports no readable sections for this current Amazon fixed-layout form |
| `vertical-ruby` | converts | 5 | Go emits an empty `<rt/>`; pronunciation text is lost |
| `link` | converts | 3 | content XHTML matches; only fallback metadata/TOC behavior differs |
| `bidi` | converts | 5 | Go loses the paragraph/nested LTR range structure and leaves bidi CSS on `body` |
| `list` | converts | 5 | Go drops the top-level `<ol start="3">` wrapper; nested list survives |
| `svg` | converts | 4 | rasterized image survives; Go promotes the containing style to `body` and drops the wrapper `<div>` |

The meaningful content differences are concrete.

For the footnote specimen Python emits:

```html
<p class="class_s6">Main text<a href="c0.xhtml#aA" epub:type="noteref">1</a>.</p>
<aside id="aA" epub:type="footnote" class="class_s8">Footnote text.</aside>
```

while Go emits:

```html
<p class="class_s6">Main text<a href="c0.xhtml#aA" epub:type="noteref">1</a>.</p>
<p id="aA">Footnote text.</p>
```

So link reconstruction works, but the target's `footnote` classification and footer semantics are not being converted into the EPUB note element/style.

For the table specimen Python reconstructs a normal table tree:

```html
<table class="class_s11">
  <caption ...>...</caption>
  <thead ...>...</thead>
  <tbody ...>...</tbody>
</table>
```

while Go puts the table's style on `<body>`, emits the caption as a `<div>`, and places `<thead>` / `<tbody>` directly under `<body>`. The row/column spans themselves survive. This isolates the missing behavior to table/container reconstruction rather than span decoding.

For vertical ruby, Python emits:

```html
<ruby><rb>漢</rb><rt>かん</rt></ruby>
```

while Go emits:

```html
<ruby><rb>漢</rb><rt/></ruby>
```

The KAF/raw-Ion probe already showed that the pronunciation is present in the KFX as a separate `ruby_content` object, so this is unambiguously a Go reverse-path loss rather than missing source data.

Four additional fixtures make the pattern clearer:

- **Internal link:** Amazon compiles the linked text as a range style event with `link_to = aE`, and Go reconstructs the content XHTML identically to Python. This is a useful negative control: the differential harness does not manufacture a content mismatch for every fixture.
- **Bidi:** Amazon stores the RTL paragraph direction on one style and the embedded LTR range as a separate style event. Python reconstructs `<body dir="rtl"><p ...>...<span dir="ltr">ABC 123</span></p></body>`, while Go promotes the paragraph style/content into `body`, loses the nested span, and leaves `direction`/`unicode-bidi` in CSS.
- **List:** Amazon emits a typed `LIST` with `list_start_offset = 3`, `list_style = numeric`, nested `LIST_ITEM` objects, and a nested `LIST` with `list_style = circle`. Python reconstructs `<ol start="3">...`, while Go emits the top-level `<li>` children directly under `body`; the nested `<ul>` remains.
- **SVG:** this simple inline SVG is not retained as KVG/SVG by the current producer; Amazon rasterizes/normalizes it to an image resource inside a `CONTAINER`. Python keeps the containing `<div>` around the `<img>`, while Go promotes the container style to `body` and places the image directly there.

These are not four unrelated mysteries. Source comparison identifies a common architectural divergence in Go's top-level handling. `internal/kfx/yj_to_epub_content.go::promotedBodyContainer` promotes a single styled raw YJ node into the XHTML `body` based largely on the raw node's shape: a styled node with `content_list`, a styled leaf text node, or a styled resource node. That shortcut was introduced to approximate Python's later DOM simplification, but it makes the HTML tag decision before the node has been rendered.

Python does the opposite in `REFERENCE/KFX_Input/kfxlib/yj_to_epub_content.py::process_content`. It first constructs the semantic HTML element (`table`, `ol`, `div`, text container, and so on), applies the detailed `COMBINE_NESTED_DIVS` gates, and only then handles `is_top_level`. If the resulting top-level tag is not `aside`, `div`, or `figure`, Python wraps it and renames the **outer** element to `body`; the original `table`, `ol`, etc. remains a child. Only an actual top-level `aside`/`div`/`figure` can itself become `body`.

That distinction directly explains the table and list failures and plausibly accounts for much of the bidi/SVG wrapper behavior. It is a strong example of a parity shortcut that looked valid against the ten-book corpus but encoded the wrong abstraction. The correct invariant is not “one styled top-level YJ node can be promoted”; it is “perform Python's rendered-element merge and top-level-tag rules in the same order.”

Two other gaps can already be localized precisely rather than attributed to the broad promotion issue:

- **Ruby:** current Amazon's `ruby_content` group contains a child structure whose `content` is the direct Ion string `"かん"`. Go's `rubyContentParts` only accepts `content` when it is a map/reference or accepts a `content_list`; it never handles a direct string. Python's generic `process_content` does. The empty `<rt/>` is therefore a concrete omitted IonString branch.
- **Footnote:** Python initially builds ordinary text content as a `div`, applies `yj.classification = footnote` while it is still a `div`, changes it to `aside epub:type="footnote"`, and only later simplifies ordinary unclassified divs toward paragraphs. Go's `renderTextNode` eagerly constructs a `<p>` and then calls `applyStructuralNodeAttrs`; that helper only changes a classified element to `aside` when its tag is `div`. The semantic transition is already impossible by the time classification is applied.

#### Fixed-layout failure localized: the decoded data is present, the Go page-spread result is never rendered

The fixed-layout failure is now localized well enough that it should not be described as an input-decoding failure. The generated KFX contains:

```text
kindle_capability_metadata/yj_fixed_layout = 3

section c0 page template:
  id            = 863
  story_name    = l4
  writing_mode  = horizontal_tb
  direction     = ltr
  font_size     = 16
  fixed_width   = 450
  fixed_height  = 600
  virtual_panel = enabled
  layout        = scale_fit
  type          = container
```

Go decodes the metadata correctly: `FixedLayout=true`, `IsPDFBacked=true`, `IsPDFBackedFixedLayout=true`, `VirtualPanelsAllowed=true`, and book type `comic`. An earlier Go trace incorrectly printed `is_pdf_backed=false` and put the book ID in `cde_content_type`; that was a trace-capture bug, not a decoder bug. The trace helpers have now been corrected to report the actual decoded-book flags.

Python's behavior for this exact template is also clear. Although the book is dispatched through the comic/page-spread path, the special PDF-backed `scale_fit` branch only applies when `fixed_width` and `fixed_height` are absent. Here both are present, so `process_page_spread_page_template` takes its ordinary leaf path and calls `process_content` on the complete page-template object. Current KFX Input consequently produces `/c0.xhtml` with:

```text
viewport: width=450, height=600
OPF property: rendition:layout-pre-paginated
body style: font-size: 0.16px
content: <div><img ... height="48px" width="48px"/></div>
```

The historical Go path loses this in three separate integration steps:

1. `parseSectionFragment` stores `PageTemplateValues` through `filterBodyStyleValues`. For this template that leaves only `font_size=16`; `type`, `layout`, `story_name`, dimensions, direction, writing mode, virtual-panel state, and ID are not carried into `processSectionComic`.
2. `processSectionComic` does call `processPageSpreadPageTemplate`, but `processSectionWithType` explicitly discards the returned `pageSpreadResult` and returns `(renderedStoryline{}, nil, false)`. `processReadingOrder` therefore adds no section. Existing Go tests explicitly codify zero `RenderedSections` for comic dispatch as the expected behavior.
3. Even when the full raw page-template map is supplied experimentally, this fixture correctly lands in `processPageSpreadLeaf`, but that Go function only records a `pageSpreadSection` plan (`TemplateData`, CSS-link flag, position marker, etc.). It does not invoke the actual storyline/content renderer or append a rendered XHTML section.

An isolated call confirmed both forms: the currently parsed template produces a leaf result containing only `{font_size:16}`, while supplying the complete raw template produces a leaf result containing all of the data above. Neither result can reach EPUB output because the page-spread result is not integrated into `RenderedSections`.

So the fixed-layout failure is not “Go cannot parse current Amazon fixed layout.” It is a partially implemented page-spread architecture whose intermediate result type never became an output path, compounded by prematurely filtering the page-template structure. This is another case where branch-level unit tests can be green while end-to-end semantics are absent.

There are also lower-severity systematic differences in these synthetic books: Python generates an opaque fallback identifier and `Unknown` author where Go derives an identifier from the input path; Python uses `Content` as the synthesized navigation label while Go derives text such as `Hello`, `Probe table`, or the first paragraph. The vertical fixture additionally differs in how document writing mode/default margins are emitted. Those need source-level comparison before deciding which are functional bugs versus output-normalization choices.

This experiment is important for the maintenance question. A green arbitrary-book corpus did not mean the old port had captured the semantic space. The Amazon producer can now generate small canonical cases that exercise branches absent from those books, and the first few such cases already found several real gaps. That makes a systematic generated corpus much more valuable than another hand-picked pile of books, while still leaving historical/consumer compatibility to real samples.

## Generated KDF storage format: SQLite plus Amazon fingerprint records

The synthetic `book.kdf` is recognizable as SQLite 3, but opening the file directly with stock `sqlite3` reports a malformed schema. The reason is visible at file offset 1024:

```text
fa 50 0a 5f ...
```

That is the same fingerprint wrapper already handled by KFX Input's `SQLiteFingerprintWrapper`:

```python
FINGERPRINT_OFFSET = 1024
FINGERPRINT_RECORD_LEN = 1024
DATA_RECORD_LEN = 1024
DATA_RECORD_COUNT = 1024
FINGERPRINT_SIGNATURE = b"\xfa\x50\x0a\x5f"
```

For this small generated fixture there is one 1024-byte fingerprint record. Removing bytes `[1024:2048]` changes the file from 21,504 bytes to 20,480 bytes, after which stock SQLite opens it normally.

This validates that particular piece of kfxlib behavior directly against current Amazon-generated output rather than against an old observed fixture.

### Minimal KDF schema

After removing the fingerprint record, the synthetic fixture has exactly three tables:

```sql
CREATE TABLE capabilities(
    key char(20),
    version smallint,
    primary key (key, version)
) without rowid;

CREATE TABLE fragments(
    id char(40),
    payload_type char(10),
    payload_value blob,
    primary key (id)
);

CREATE TABLE fragment_properties(
    id char(40),
    key char(40),
    value char(40),
    primary key (id, key, value)
) without rowid;
```

The only capability in this fixture is:

```text
db.schema -> 1
```

`fragment_properties` separately records graph/type information in plain strings. Examples from the controlled fixture:

```text
c0    child         c0-ad
c0    child         l4
c0    element_type  section
l4    child         i5
l4    child         i7
l4    child         l4
l4    element_type  storyline
i5    child         s6
i5    element_type  structure
i7    child         s8
i7    element_type  structure
s6    element_type  style
s8    element_type  style
n9    element_type  nav_container
aA    element_type  anchor
aB    element_type  anchor
```

The `fragments` table then stores the actual Ion payloads as `blob` values. The minimal fixture includes entries such as:

```text
$ion_symbol_table
max_id
content_features
book_metadata
book_navigation
document_data
c0
l2
l4
i3
i5
i7
s6
s8
n9
aA
aB
c0-ad
dC
c0-spm
yj.section_pid_count_map
```

This reveals a useful storage-layer split:

- SQLite `fragment_properties` exposes a plain-string object graph and element classification;
- `fragments` contains the serialized Ion object/property payloads;
- KAF reconstructs the typed object model on top of the two.

For future fixtures this means a three-way comparison is possible:

```text
SQLite graph metadata <-> raw Ion fragment <-> KAF typed object
```

That should make it much easier to determine where a semantic distinction is encoded and whether a kfxlib behavior belongs to storage decoding, YJ semantics, or EPUB reconstruction.

## Position and location maps

`process.a.e` runs position-map generation first and then location-map generation.

### Position map

The process wrapper `com.amazon.kfxconverter.process.k` invokes:

`com.amazon.kcfpositionmapcreator.core.KCFPositionMapCreatorApp`

The context class is `com.amazon.kfxconverter.c.m`.

The wrapper also configures `MobiContentDumper` and consumes the generated position-map artifacts/reports.

### Location map

The subsequent wrapper `com.amazon.kfxconverter.process.h` generates the location map, using the position-map result and a dedicated KCF location-map path. The bundle includes:

`com.amazon.kcflocationmap.creator.KCFLocationMapCreatorApp`

The result is tracked separately and may produce a `.loc` file depending on flags.

These two dedicated components are important for future work on Kindle/KOReader position translation: position/location semantics are not merely incidental fields in the main converter. Amazon has standalone map creators with their own object model and logic.

## What this means for KFX Input analysis

The current KFX Input implementation is still important, but its role should change in this investigation.

### What Previewer can answer directly or experimentally

Previewer is a strong source for:

- canonical property names/IDs;
- intended element/object categories;
- HTML/CSS -> YJ translation;
- YJ -> HTML/CSS reverse style mapping for supported properties;
- style inheritance/merge rules;
- semantic tagging;
- writing-mode and bidi handling;
- ruby semantics;
- table semantics and feature versions;
- navigation construction;
- position/location-map construction;
- document validation/fixup order;
- content feature/capability stamping;
- current-format serialization/storage behavior through KAF.

For those topics, the preferred method is now:

```text
minimal source EPUB/HTML
        |
        v
Amazon producer implementation
        |
        v
YJ/KDF/KFX structure
        |
        +----> KAF typed object model / native introspection
        |
        +----> Previewer rendering
        |
        v
compare with KFX Input inverse behavior
```

### What KFX Input remains uniquely useful for

KFX Input remains valuable evidence for:

- historical generator variants no longer emitted by current Previewer;
- actual consumer-delivered KFX containers;
- split-container and DRM-adjacent packaging behavior;
- malformed or publisher-specific structures tolerated in the field;
- old dictionary/comic/fixed-layout variants;
- jhowell's chosen normalization when multiple EPUB representations are semantically equivalent;
- format quirks learned from years of user-supplied books.

Those should be treated as compatibility extensions around the canonical model, not as the only available definition of the model.

## Concrete architectural implication for any future native implementation

A cleaner independent implementation need not reproduce kfxlib's pervasive raw-ID architecture.

A more defensible shape is:

```text
KFX/KDF/container bytes
        |
        v
Ion + symbol decoding
        |
        v
named/typed YJ/KAF model
        |
        v
semantic normalization
        |
        v
EPUB document model
        |
        v
serialization
```

Raw `$NNN` identifiers should ideally be confined to the serialization/symbol boundary. Amazon's own implementation already supplies the vocabulary and many of the structural concepts needed for such a model.

This would not remove the need for historical compatibility handling, but it would make those paths explicit exceptions instead of making the entire core implementation look like a wire-format dump.

## Facts, strong inferences, and open questions

### Facts established in this pass

- Previewer 3.106 ships `EpubToKFXConverter-4.0.jar` and a native KAF library.
- The JAR contains the source-adjacent EPUB -> YJ conversion pipeline.
- `EpubAdapterApp -> ConversionEngine -> YJConverter` is the active Java route into YJ generation.
- `com.amazon.adapter.common.d.a` explicitly sequences metadata, HTML transform, semantic transform, YJ postprocessing, optimization, note/ruby/style/table/unit fixups, capability stamping, validation, and save.
- The KAF Java API uses a typed object model with books, sections, storylines, containers, content lists, navigation, styles, positions, ruby content, etc.
- KAF is backed by a native C++ implementation exposed through JNI.
- The KAF property enum has 854 entries and `PropertyName` uses the enum ordinal as the property ID.
- A standalone harness successfully loads the bundled native KAF implementation and `PropertyNameUtil.a()` returns the native 854-entry property map.
- The native property map agrees with the enum for the checked IDs and reports ID 853 as `bcSequenceNumber` in Previewer 3.106.
- Excluding the symbol-catalog declaration itself, current kfxlib uses 604 unique numeric `$NNN` IDs in implementation code, and all 604 are named by Previewer 3.106's native KAF property map.
- Current KFX Input in this checkout stops its built-in YJ shared-symbol placeholder list at `$852` and has no `$853` use.
- `stylemap.ion` is loaded by Amazon code and contains a reverse-mapping suppression field called `ignore_for_yj_to_html_mapping`.
- `yjhtmlmapper` implements both HTML -> YJ and YJ -> HTML/CSS mapping paths.
- `stylelist.ion` configures explicit style merge/inheritance strategy classes.
- Previewer ships YJDecompiler error/info resource bundles describing EPUB generation and reverse style mapping behavior.
- Dedicated KCF position-map and location-map applications are present.
- `EpubAdapterApp` was successfully invoked directly on a synthetic EPUB and generated a valid `book.kdf` without launching the Previewer GUI.
- `BookFactory` then successfully reopened that generated KDF through Amazon's native KAF runtime.
- The generated KDF is a SQLite database with Amazon's 1024-byte `fa 50 0a 5f` fingerprint record inserted at offset 1024; removing it makes the database readable by stock SQLite.
- The minimal generated KDF schema contains `capabilities`, `fragments`, and `fragment_properties`, with graph/type edges in plain strings and Ion payloads in fragment blobs.
- KAF exposed the fixture as typed `Storyline`, `Structure`, `Style`, text, document-data, navigation, anchor, and section objects; the source H1 was explicitly stamped with `yj.semantics.heading_level = 1`.
- The Previewer GUI binary names an out-of-process `com.amazon.yj.decompiler.app.DecompilerApp` and contains a complete-looking decompiler orchestration vocabulary, but that class is absent from every bundled JAR inspected.
- A second controlled fixture proved that Amazon's producer uses `<meta name="primary-writing-mode" content="vertical-rl"/>` to establish document-level vertical writing mode; CSS alone did not set KAF `DocumentData.writing_mode`.
- Amazon's current ruby parser requires explicit `rb` and `rt` children; the HTML5 shorthand with a raw text child is rejected by the producer.
- The generated ruby representation was recovered exactly: base-text ranges use style events carrying `ruby_name`/`ruby_id`, while pronunciation text lives in a separate `ruby_content` object.
- Feeding a current Previewer-generated KDF to the current KFX Input decoder immediately warns that YJ max ID 853 exceeds its known table ending at 852.
- The historical Go symbol catalog matches the live Previewer 3.106 KAF table exactly for all 842 IDs it contains (10..851); Previewer adds `page_regions` at 852 and `bcSequenceNumber` at 853.
- Previewer's current renderer directly consumes property 852 `page_regions` as fixed-page rectangles plus optional layout hints and scales them into rendered-page coordinates.
- A controlled EPUB 3 footnote compiles into a `yj.note` style event linking to an anchor whose target is a footer-classified `footnote` text structure.
- Bridging Amazon-generated KDF through current KFX Input's serializer into one shared KFX input exposes real historical-Go parity gaps: footnote target semantics are lost, table wrapping is malformed, ruby pronunciation is dropped, and the simple current fixed-layout fixture is not readable by the Go converter.
- The fixed-layout failure is now localized after decode: Go correctly recognizes `yj_fixed_layout=3` as PDF-backed fixed-layout/comic, but `parseSectionFragment` strips the structural page-template fields, `processSectionWithType` discards the resulting `pageSpreadResult`, and the leaf result type itself does not render XHTML.
- The Go trace implementation previously misreported PDF-backed/CDE state because `captureContentFeatures` populated only `CDEContentType: book.BookID`; trace capture now reports the actual decoded flags and detected book type.
- Additional controlled link/bidi/list/SVG fixtures show that ordinary internal-link content round-trips, while top-level bidi/list/container cases expose a common over-broad Go body-promotion heuristic that differs from Python's rendered-element-first top-level rules.
- The ruby pronunciation loss is specifically caused by Go `rubyContentParts` omitting direct IonString `content`; the footnote loss is specifically caused by Go choosing `<p>` before applying the classification that Python applies while the node is still a `<div>`.

### Strong inferences

- For enum-backed property IDs, the KAF enum provides the authoritative current Previewer name for many kfxlib `$NNN` symbols. This is stronger than name correlation because `PropertyName` directly stores the enum ordinal as the ID.
- Amazon's intended internal semantic model is KAF/YJ typed objects and named properties; raw numeric symbol IDs are a serialization/interface detail rather than the conceptual representation.
- The combination of producer code, bidirectional style mapping, KAF introspection, and rendering can serve as a substantially better oracle for current KFX semantics than a random-book corpus alone.
- The current Previewer contains enough infrastructure to construct a systematic feature-oriented test corpus from minimal EPUB inputs; this was demonstrated end-to-end by generating and reopening a synthetic KDF. It still cannot be assumed to cover all historical consumer KFX variants.

### Open questions

1. **Where did the `com.amazon.yj.decompiler.app.DecompilerApp` payload live?**
   - The Previewer GUI's KDF -> EPUB launcher and application argument contract are now recovered.
   - The class is absent from every JAR currently bundled in the inspected app and from the original current Previewer ZIP.
   - The remaining question is historical/deployment-specific: older Previewer or Kindle Create package, dynamically provisioned artifact, or Amazon-internal build dependency.

2. **Which KAF object/property APIs are safe and useful for exhaustive graph dumping?**
   - Loading `book.kdf`, enumerating object IDs, document data, storylines, styles, text content, and basic sections works.
   - One exploratory call to section property enumeration caused the bundled JRE/native stack to crash, so the harness should isolate APIs and establish ownership/lifetime rules rather than indiscriminately traversing every getter.

3. **What exactly is property 853 `bcSequenceNumber` used for?**
   - It is newer than the current KFX Input built-in catalog in this checkout.
   - Current Previewer-generated KDFs already advertise a shared-symbol max ID that includes 853, even when the tested semantic fragments do not appear to use it.
   - Static references plus targeted fixture generation should determine which producer/profile emits it and in which fragment/object family.

4. **How much of Amazon's YJ -> EPUB decompiler remains callable?**
   - The bidirectional style mapper is callable in principle.
   - The broader YJDecompiler resource set indicates a larger reverse pipeline existed or exists.

5. **Which Previewer-generated structures differ from real consumer KFX?**
   - This requires controlled generated fixtures plus representative real files.
   - The goal should be to classify differences by generator/version/profile instead of treating all KFX as one undifferentiated corpus.

6. **What are the native KAF serializer/storage formats and vtables?**
   - JNI wrappers are mapped well enough to begin renaming C++ virtual slots in Ghidra.
   - Recovering `KAFDigitalBook`, storage, property-value, container, and storyline vtables would make native decompilation much more readable.

## Recommended next investigation order

1. Extend the now-reusable KAF/KDF harness into a feature matrix:
   - add navigation, links, drop caps, bidi, conditional content, page spreads, comics/guided view, SVG/KVG, and richer fixed-layout specimens;
   - continue comparing each typed KAF graph with its raw KDF/Ion/storage representation;
   - add isolated subprocess probes for additional JNI APIs only when a concrete semantic question requires them.

2. Locate the missing YJ -> HTML/EPUB decompiler payload historically:
   - inspect older Previewer/Kindle Create distributions for `com.amazon.yj.decompiler.app.DecompilerApp`;
   - trace callers of the reverse `yjhtmlmapper` method and `YJDECOMPILER_*` resources for reusable reverse components that remain in the current JAR;
   - use the already-recovered KDF -> EPUB argument contract to recognize candidate payloads immediately.

3. Create a controlled feature fixture corpus:
   - one semantic or style feature per EPUB;
   - run Previewer;
   - dump produced KDF/YJ through both raw Ion and KAF;
   - render in Previewer;
   - compare with KFX Input's reverse output.

4. Recover and document the KAF native class/vtable layout in Ghidra, beginning with:
   - `KAFDigitalBook`;
   - `KAFBookContent`;
   - `KAFStoryline`;
   - `KAFContainer`;
   - `KAFContentList`;
   - `KAFBookPositionInfo`;
   - storage/symbol-table classes.

5. Build an ID/name/meaning cross-reference generated from Previewer's enum and mapping files, then use it when reading kfxlib so that `$NNN` occurrences are immediately presented with Amazon's current semantic names.

## Reproducibility notes

Useful commands from this pass:

```sh
# Previewer version
/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' \
  'REFERENCE/Kindle Previewer 3.app/Contents/Info.plist'

# Inspect JAR class list
jar tf \
  'REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/lib/EpubToKFXConverter-4.0.jar'

# Directly decompile a class that collides on case-insensitive filesystems
jadx --single-class com.amazon.kaf.c.b \
  --single-class-output /tmp/kaf-property-enum.java \
  --no-res \
  'REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/lib/EpubToKFXConverter-4.0.jar'

# Native KAF runtime probe (after writing /tmp/KafProbe.java as above)
JAR='REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/lib/EpubToKFXConverter-4.0.jar'
LIB='REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/lib'
javac -cp "$JAR" /tmp/KafProbe.java
java -Dklibname=shared -Djava.library.path="$PWD/$LIB" -cp "$JAR:/tmp" KafProbe

# Ghidra health
./REFERENCE/ghidra doctor

# Analyze the already-extracted x86_64 native slice
./REFERENCE/ghidra project create kindle_previewer_kaf
./REFERENCE/ghidra import \
  REFERENCE/ghidra_analysis/libshared/libshared.x86_64.dylib \
  --project kindle_previewer_kaf --program libshared_x86_64 --detach

# Find native symbol-name lookup
./REFERENCE/ghidra --project kindle_previewer_kaf \
  --program libshared.x86_64.dylib \
  query functions -f 'name~nativeGetSymbolName' \
  --fields name,address,size,signature --format table

# Decompile a JNI wrapper
./REFERENCE/ghidra --project kindle_previewer_kaf \
  --program libshared.x86_64.dylib \
  decompile _Java_com_amazon_kaf_jni_adapters_DigitalBook_nativeGetSymbolName
```

## Current conclusion

The central research assumption should be changed from:

> KFX Input is the only practical specification, so independent implementations must infer semantics from its behavior and from a limited book corpus.

To:

> Kindle Previewer exposes a large portion of Amazon's current KFX/YJ semantic implementation, including named property IDs, a typed KAF model, producer logic, bidirectional style mappings, validation/fixup phases, and native storage/object APIs. KFX Input should be used primarily to extend that canonical model with historical and field-observed compatibility behavior.

That is a much stronger basis for further KFX reverse engineering.
