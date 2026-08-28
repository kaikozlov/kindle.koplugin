# Property 853 `bcSequenceNumber` — focused audit (Kindle Previewer 3.106)

Status: complete static + live-runtime audit, 2026-08-26

This is a narrow, deep follow-up to open question 3 in
`docs/kindle-previewer-reverse-engineering.md` (not edited here; the summary there
remains authoritative for the broader investigation).

Question: does shared symbol / KAF property **853 `bcSequenceNumber`** have any
reader, writer, serializer, or storage behavior in Kindle Previewer 3.106, and is it ever
*emitted* (as opposed to merely declared) in the generated Ion/KDF data we can produce?

Artifacts covered:

- `Contents/lib/fc/lib/EpubToKFXConverter-4.0.jar` (25,587 extracted classes)
- `Contents/lib/fc/lib/libshared.dylib` (x86_64 + arm64 slices)
- `Contents/lib/fc/bin/KindleImageProcessor`
- `Contents/MacOS/Kindle Previewer 3` (main binary)
- KFX Input 2.34.0 (`20260822`) Python reference
- 304 Amazon-generated KDF books (12 semantic fixture families, incl. comic/CMX variants)
- live native KAF runtime via isolated JNI subprocesses

## Executive answer

**853 is a real KFX container sequence field used by Amazon's bundled KAF/YJSDK `BinaryStorage` to order competing containers.**
The initial string-xref audit was insufficient: the field is consumed by numeric ID after parsing. A
follow-up exact-immediate/data-flow audit recovered the parser, the corrected `BinaryContainer`
layout, a storage-level sequence watermark, highest-sequence selection, and propagation of the value
into `BinaryObjectStream` objects. The decisive code exists both in Previewer's main executable and
in the x86_64 slice of `libshared.dylib` loaded by the standalone KAF JNI runtime, so this is not only
a GUI-side behavior.

| Artifact | Occurrences of the string | Nature |
| --- | ---: | --- |
| EpubToKFXConverter-4.0.jar | 1 class file | KAF property enum constant (declaration only) |
| libshared.dylib | 1 string per arch slice | registration by name; x86_64 also contains numeric parser + `BinaryStorage` arbitration |
| KindleImageProcessor | 1 string | shared-symbol-table registration; exact `0x355` immediates audited separately |
| Kindle Previewer 3 (main) | 1 string | registration + numeric parser + sequence-order consumers |
| KFX Input 2.34.0 | 0 by name; `$853?` placeholder | numeric table extent only |

In the 304 generated KDF instances swept in this investigation, every book declared the shared
table through 853 but none used 853 in a **KDF fragment payload** (highest observed payload SID: 790).
That negative result is orthogonal to the wire location: `bcSequenceNumber` belongs to KFX
container-info, not the KDF application fragment graph. Previewer parses it into
`BinaryContainer+0x44`; `BinaryStorage` compares it against a zero-initialized sequence watermark and
uses strict unsigned `>` ordering once sequence tracking is active. The value is also copied into
`BinaryObjectStream` and exposed through a virtual accessor; equivalent non-binary streams report
zero. What remains unknown is the producer-side increment/scope/wrap policy. The live generic KAF
property API also accepts/read-backs 853 in memory. `nativeSave` crashes even on an unmodified
save-only control in the standalone harness, so KAF-save persistence remains untested, not negative.

## Evidence

### 1. Java: enum declaration only, no reader/writer

The full JAR was extracted (25,587 class files) and byte-searched. Exactly one
class contains `bcSequenceNumber`:

```text
com/amazon/kaf/c/b.class   (KAF property enum, JADX: com.amazon.kaf.c.EnumC4222b)
```

`javap` of the static initializer shows the terminal constant:

```text
ldc           #62    // String BcSequenceNumber
sipush        853
ldc_w         #929   // String bcSequenceNumber
invokespecial #2564  // "<init>":(Ljava/lang/String;ILjava/lang/String;)V
putstatic    #1753   // Field BcSequenceNumber:Lcom/amazon/kaf/c/b;
```

It is the **last** constant of the 854-entry enum (ordinals 0..853), immediately
after `PageRegions`(852, `page_regions`) and `VertexList`(851, `vertex_list`).

To rule out *numeric* use where the string is absent, all 25,587 classes were
scanned for Java bytecode carrying the integer literal 853 (`sipush 0x0355` /
constant-pool int `0x00000355`). Nine files matched; `javap -c` context showed
**eight are unrelated static array initializations** (error-code tables in
`com/amazon/I/b`, a char map, float/int lookup tables, SVG glyph-id tables), and
the ninth is the enum itself. Confirmed: **no Java code in the converter JAR
reads, writes, serializes, or stores property 853.**

All other JARs under `Contents/` (commons-*, jericho, jna, jsoup, antlr, etc.)
contain zero occurrences, raw or compressed.

### 2. libshared.dylib: string references are registration; numeric behavior lives elsewhere

`bcSequenceNumber` appears once per arch slice in `__TEXT,__cstring`
(x86_64 `0x633a5d`, arm64 `0x617579`), embedded in the shared-symbol name block
(`... vertex_list, page_regions, bcSequenceNumber ...`).

Ghidra (x86_64 slice, 23,074 functions after analysis) reports exactly two
references:

```text
0x00037e66  _Java_com_amazon_kaf_util_PropertyNameUtil_getPropertyNameIndexMap  (READ)
0x00139590  FUN_00134950                                                                (DATA)
```

- `PropertyNameUtil_getPropertyNameIndexMap` is the JNI method behind
  `PropertyNameUtil.a()`: it builds the live 854-entry property-name/index map.
  Registration, not consumption.
- `FUN_00134950` decompiled is the **YJ shared-symbol-table registration
  initializer**: a long sequential series of virtual calls at vtable slot +0x18,
  one per shared symbol name in ascending order, terminating:

  ```text
  ... "vertex_list" ... "page_regions" ... "bcSequenceNumber", 0 ...
  ```

Both **string** xrefs are declaration/registration. This does **not** imply that property 853 has
no native behavior: shared properties are commonly accessed by numeric ID. An exact-immediate audit
of this same x86_64 `libshared.dylib` found only three `0x355` sites: `FUN_00186710`, the actual
container-info parser for 853; `FUN_001cd6d0`, a YJSDK/shared-table extent construction path; and
`FUN_00394220`, an unrelated OpenSSL source-line constant. The first site is the `libshared` copy of
the parser described below, and the same library also contains the downstream `BinaryStorage`
arbitration. The numeric/data-flow audit is therefore the authoritative behavioral check.

**Naming family (contextual).** The same initializer reveals the neighbors
853 lives among — a contiguous `bc*` block:

```text
bcContId  bcComprType  bcDRMScheme  bcChunkSize  bcIndexTabOffset  bcIndexTabLength
bcDocSymbolOffset  bcDocSymbolLength  bcRawMedia  bcRawFont  bcFCapabilitiesOffset  bcFCapabilitiesLength
```

KFX Input decodes these as **KFX container storage fields** (`kfx_container.py`:
`$409` id, `$410` compression, `$411` DRM scheme, `$412` chunk size, `$413/$414`
index-table offset/length, `$415/$416` document-symbol offset/length, and `$594/$595`
format-capabilities offset/length). `$417/$418` are `bcRawMedia`/`bcRawFont`; `$422/$423`
are `resource_width`/`resource_height`, not capabilities offsets. `bcSequenceNumber`
is appended at the end of the `bc*` family.

### 3. Native numeric-ID audit: parse, storage layout, and sequence ordering

A follow-up disassembly audit searched the x86-64 native binaries for exact immediate `0x355`
(decimal 853), then classified every hit by function context. This was necessary because a
string-only xref audit misses property accessors that pass or compare numeric IDs. The YJSDK code is
present in two linked copies with matching structure: Previewer's main executable and
`libshared.x86_64.dylib`. The address pairs below are useful for reproducing the result:

```text
role                              Previewer main       libshared.x86_64
handler_FileMetadata parser       FUN_10144b490        FUN_00186710
metadata -> BinaryContainer       FUN_10144b6b0        FUN_00186930
BinaryContainer constructor       FUN_101449660-family FUN_001848d0-family
BinaryStorage constructor         FUN_10144c150        FUN_001873d0
BinaryStorage selection           FUN_10144d7d0        FUN_00188960
BinaryStorage open/arbitration    FUN_10144e210        FUN_001893a0
BinaryObjectStream seq accessor   FUN_10144c010        FUN_00187290
```

The independently decompiled `libshared` routines reproduce the same 853 parse, `+0x44` container
field, zero-initialized `+0xd8` storage watermark, and unsigned sequence comparisons described below.

#### 3.1 `handler_FileMetadata` parses 853, then copies it into `BinaryContainer`

The decisive parser branch is in `FUN_10144b490` (`0x10144b490..0x10144b66e`; compare at
`0x10144b51d`). RTTI identifies the receiving functor as
`yjsdk::handler_FileMetadata` (`N5yjsdk20handler_FileMetadataE`). The function visits the KFX
container-info properties and stores them in a compact metadata block:

```text
SID  name                      handler_FileMetadata
409  bcContId                  +0x10
410  bcComprType               +0x18
411  bcDRMScheme               +0x1c
412  bcChunkSize               +0x20
413  bcIndexTabOffset          +0x24
414  bcIndexTabLength          +0x28
415  bcDocSymbolOffset         +0x2c
416  bcDocSymbolLength         +0x30
594  bcFCapabilitiesOffset     +0x34
595  bcFCapabilitiesLength     +0x38
853  bcSequenceNumber          +0x3c
```

For 853 the branch accepts either the integer representation or converts a float-valued Ion number
to an integer:

```c
else if (property_id == 0x355) {
    if ((value->flags & 1) == 0)
        *(uint32_t *)(handler + 0x3c) = value->integer;
    else
        *(int32_t *)(handler + 0x3c) = (int)value->floating;
}
```

`FUN_10144b6b0` then copies 48 bytes from `handler_FileMetadata+0x10..+0x3f` into
`yjsdk::BinaryContainer+0x18..+0x47` (`0x10144b75f..0x10144b776`). The resulting
`BinaryContainer` layout is therefore shifted by eight bytes. All three observed `BinaryContainer`
constructor variants explicitly initialize `+0x44` to zero (`0x101449267`, `0x1014492ee`,
`0x10144933e`), so an absent 853 field has a concrete reader default of zero:

```text
BinaryContainer +0x18  bcContId
                +0x20  bcComprType
                +0x24  bcDRMScheme
                +0x28  bcChunkSize
                +0x2c  bcIndexTabOffset
                +0x30  bcIndexTabLength
                +0x34  bcDocSymbolOffset
                +0x38  bcDocSymbolLength
                +0x3c  bcFCapabilitiesOffset
                +0x40  bcFCapabilitiesLength
                +0x44  bcSequenceNumber
```

This correction matters: the earlier audit incorrectly described `BinaryContainer+0x3c` as the
sequence field. It is actually the **format-capabilities offset**. The surrounding code independently
confirms the corrected layout: `FUN_10144a900` loads document symbols from `+0x34/+0x38`, while
`FUN_10144a6a0` loads format capabilities from `+0x3c/+0x40` using a
`yjsdk::FormatCapabilitiesCreator`. `bcChunkSize` also lands at `BinaryContainer+0x28`, matching the
`0x1000` default initialized by the metadata handler and KFX Input's 4096-byte default.

#### 3.2 `BinaryStorage` uses 853 as a monotonic container-selection sequence

RTTI identifies the owning loader/manager as `yjsdk::BinaryStorage`
(`N5yjsdk13BinaryStorageE`). Its constructor at `0x10144c150` initializes a 32-bit sequence
watermark at `BinaryStorage+0xd8` to zero (`0x10144c1e4`). The container-open path at
`0x10144e210` then consumes `BinaryContainer+0x44` directly:

```text
0x10144e317  incoming = container->bcSequenceNumber
0x10144e31b  current  = storage->sequence_watermark
0x10144e323  current_is_zero = (current == 0)
0x10144e32a  compare incoming vs current
0x10144e32c  incoming_is_newer = (incoming > current)   # unsigned, strict
0x10144e349  newer = current_is_zero || incoming_is_newer
0x10144e344  doc_symbols_ok = (load_doc_symbols(container) == 0)
0x10144e356  install_doc_state = newer && doc_symbols_ok
...
0x10144e4de  if newer:
0x10144e4e1      storage->sequence_watermark = incoming
```

So the meaning is no longer merely inferred from the name: **the field orders competing KFX
containers, and larger unsigned sequence numbers supersede smaller ones once sequence tracking is
active**. Equality does not satisfy the `>` comparison.

A second `BinaryStorage` virtual method beginning at `0x10144d7d0` performs the same ordering while
choosing among matching containers. For each candidate it reads `candidate+0x44`; if a current
candidate exists, a strictly larger unsigned sequence replaces it (`0x10144db67..0x10144db72`).
When `BinaryStorage+0xd8` is still zero, the routine has a fallback path that may replace the current
candidate without requiring a larger sequence (`0x10144db74..0x10144db7f`). Thus zero is a special
**uninitialized/sequence-order-not-established** state, not simply an ordinary oldest revision.

#### 3.3 The sequence is propagated to every binary object stream

The entity/object-stream creation path at `0x101449660` copies `BinaryContainer+0x44` into a newly
allocated `yjsdk::BinaryObjectStream` at `+0x30` (`0x1014496ab`). RTTI confirms the class as
`N5yjsdk18BinaryObjectStreamE`. Its virtual method at `0x10144c010` is simply:

```c
uint32_t BinaryObjectStream::vslot5() const {
    return *(uint32_t *)(this + 0x30);
}
```

The corresponding virtual slot in `yjsdk::StorageStream` and `yjsdk::YJTextObjectStream` points to
`0x101467bf0`, which returns **0** unconditionally. That makes the role of the slot unusually clear:
binary object streams carry their source container's sequence number; stream types without this
container-revision concept report zero.

What remains unknown is the **producer policy**: which packaging event increments the number, whether
it is global to a book or scoped to a container family, and how wraparound is handled. The reader-side
ordering semantics themselves are now directly established.

#### 3.4 Standalone KAF `BookFactory` reaches this same `BinaryStorage`

The JNI entry point in `libshared`,
`BookFactory.nativeGetBook(String)` (`0x00005a90` in the analyzed x86_64 slice), converts the Java
path and calls `FUN_0005b580`. The regular-file path then builds the native storage stack through
`FUN_001e8ca0` / `FUN_00209c40`; `FUN_00209c40` calls `FUN_0018e290`, which constructs a
`BinaryStorage` through `FUN_00187540`. That factory allocates the `0xe0`-byte storage object, calls
`FUN_001873d0`, stores the backing provider at `BinaryStorage+0xd0`, and invokes the storage vtable
slot `+0x60` for the initial input. That slot resolves to `FUN_001893a0`, the sequence-aware
open/arbitration routine above.

This call chain explains why the synthetic one-file KAF probes exercise the same code recovered from
the Previewer executable. It does **not** yet explain how a retail/package storage provider supplies
additional sibling containers to an already-created `BinaryStorage`: standalone
`BookFactory.a("book.kfx")` opens only the explicit file in the controlled experiment. The mechanism
for repeated/additional `+0x60` opens remains the next reader-side RE target.

Other exact `0x355` sites were classified separately:

- `FUN_100dc5a64`: `< 0x355` threshold in symbol/index handling — table-boundary behavior, not a
  field-specific read.
- `FUN_100ea0fee` (`yj::InputParagraphAdapter` context): `< 0x356` range check with exclusions —
  again shared/property-range behavior rather than `bcSequenceNumber` semantics.
- `0x1011912be`: OpenSSL `crypto/pkcs7/pk7_doit.c` diagnostic/line context — unrelated.
- `0x1019dcce9`: FDRM assertion/diagnostic context (`fdrm_descriptors_imp.cpp`) — unrelated.
- `FUN_100ad6ed0`: codec/table-construction arithmetic — unrelated to YJ properties.
- `FUN_101495da0`: passes `0x355` into a YJSDK construction path; this is consistent with current
  shared-table extent and does not by itself identify field 853 semantics.

`KindleImageProcessor` also contains exact `0x355` immediates. The obvious sites at
`0x100051db6` and `0x1000b03f2` are ImageMagick `bmp.c` / `pcd.c` diagnostics (source-line
constants), `0x10035d829` is image-codec arithmetic, and `0x1005fe530` mirrors the YJSDK/table-extent
construction path. No second field-specific `bcSequenceNumber` parser was identified there.

The original string-reference result remains useful but narrower: `bcSequenceNumber` has one
registration string reference in the Previewer main binary and one in KindleImageProcessor. The
numeric-ID audit is what exposes the real reader behavior.

### 4. KFX Input 2.34.0 and the Go plugin

- Python (`REFERENCE/KFX_Input`): `$853?` appears only as an anonymous placeholder in the shared-table tail.
  `kfx_container.py` explicitly pops the older `bc*` container-info fields (409..416, 594/595) but
  does **not** consume `$853?`; if a delivered KFX container actually carries it today, it will remain
  in `container_info` and be reported as extra data. No `bcSequenceNumber` semantic handler exists.
- Go plugin: `internal/kfx/catalog.ion` / goldens carry the real name
  `853 -> "bcSequenceNumber"` (sourced earlier from the live native resolver),
  and `yj_symbol_catalog_test.go` pins it. No Go code reads or writes the
  property.

This exposes a concrete **reader-arbitration gap** in both reverse implementations. Current KFX Input
sorts its discovered container datafiles by name, deserializes every container, and appends every
container's fragments. With conflicting duplicates, its normal decode path fails consistency checks
before EPUB generation (for example, duplicate singleton fragments and a duplicate section produce a
`YJFragmentList get has multiple matches` exception); if fragments reach
`organize_fragments_by_type`, that routine keeps the first duplicate ID and logs an error. Current Go
sorts `containerSource` values by path and processes every source, but its typed fragment maps
generally assign later values, so many duplicate IDs are effectively **last-write-wins** after
logging. Neither implementation consults 853 or reproduces `BinaryStorage`'s highest-sequence
selection.

A controlled two-container experiment makes the Go difference observable rather than merely static.
Two otherwise equivalent CONT v2 containers were built from the same long-text fixture, with the
first content string changed to `LOW! text` / `HIGH text` and container-info sequence values 4096 /
8192. In case A, the primary `book.kfx` carried **HIGH/8192** and alphabetically later
`book.sdr/low.kfx` carried **LOW/4096**; Go emitted `LOW! text`. Reversing the pair (LOW primary,
HIGH sidecar) emitted `HIGH text`. The result therefore follows source/path processing order, not
`bcSequenceNumber`.

The same synthetic pair does **not** yet provide an end-to-end native arbitration test. In the
standalone KAF harness, `BookFactory.a(".../book.kfx")` successfully reads the synthetic CONT and its
853 field, but it does not automatically discover the sibling `.sdr` container: case A returns the
HIGH primary and case B returns the LOW primary. Passing the containing directory fails with KAF
error 26, and a generic ZIP containing both CONT files fails with error 29. The §3 sequence-selection
semantics are therefore directly established from native code, while the exact external packaging /
storage-provider route that feeds multiple binary containers into one `BinaryStorage` remains open.
Practical behavior on a real delivered book is still unmeasured because no retail multi-container
sample carrying nonzero 853 values is present in the local corpus.

### 5. KDF declaration sweep: useful catalog evidence, not a container-info producer test

Every generated `book.kdf` instance swept under `/tmp` from the controlled fixture runs
(304 instances across repeated runs of the semantic families, including comic/CMX/region variants
and full `KFXGenApp` pipeline outputs) was decoded with KFX Input 2.34 and
its fragment graphs walked for raw shared-SID usage (`$N` / `$N?` forms,
keys and values).

Result — all 304 books, no exception:

```text
shared import declared through SID 853  (raw max_id 844 + 9 system symbols)
highest SID actually used in payloads:  790
occurrences of SID 853 in any payload:  0
```

Distribution of highest-used SID: 790 (106 books), 590 (55), 598 (47), 617 (33),
761 (27), 682 (27), 625 (9).

The `max_id` semantics matter and were mis-stated in earlier notes: the import's
raw `max_id` counts shared-table entries only; the last declared SID is
`max_id + 9` (system symbols). A book "declaring 853" is therefore just
Previewer stamping the current shared-table extent — the earlier KFX Input 2.33
warning (`exceeds known table size ... =853`) was a pure table-extent mismatch,
**not** evidence of 853 usage.

After the §3 reader classification, this KDF scan is explicitly **not a producer test for
`bcSequenceNumber`**. The native field belongs to KFX container-info, while the sweep above walked
KDF application-fragment Ion. The controlled Previewer pipeline available here stops at KDF rather
than a delivered CONT/KFX container. Testing emission therefore requires a delivery-container writer
or a retail KFX/CONT sample, not more KDF fragment sweeps.

### 6. Live KAF runtime: generic set/get works; native container reader supplies the schema clue

Isolated one-shot JNI subprocesses (bundled JRE 11 + `libshared.dylib`,
`System.exit(0)` before teardown per the known lifetime hazards):

```text
DigitalBook.nativeGetSymbolID("bcSequenceNumber") = 853
DigitalBook.nativeGetSymbolName(853)             = "bcSequenceNumber"
PropertyNameUtil.a(853)                          -> id=853, enum=BcSequenceNumber
PropertyManager.a("bcSequenceNumber")            -> same property object
```

Bidirectional resolution is confirmed live, not just static.

**Write/read-back in memory** (probe sets 853 on the first Structure container
of the content storyline of the minimal fixture):

```text
target container type=Structure kfx_id=864
property count before=4
container.a(prop, kInt 12345) returned true
read back: kInt 12345
property count after=5
```

So KAF's generic property layer (`Container_setNativeProperty` / `getNativeProperty`) accepts and
stores the property on a Structure with no validation error. This generic set/get experiment does
not establish where the property is valid semantically; the native container-info parser in §3 is
the stronger evidence for its actual format family.

One soft observation: `PropertyName.h()` (default value) reports type `kElemType`
with int value 853, i.e. the property's own symbol — a placeholder rather than a
typed default. Interpretation (flagged as such): 853 has no declared default
type, unlike properties with real consumer schemas.

**Persistence honestly untested.** `DigitalBook.nativeSave(true)` SIGSEGVs in
this standalone harness (`RawAccessBarrier::load_internal`, GC frame). The
control matters: **`save-only` — open the unmodified KDF and save — crashes
identically**. The crash is a harness/ownership limitation of `nativeSave`,
not something caused by property 853. Consequently no claim is made about
whether 853 survives KAF serialization. Holding strong references to the
`PropertyManager`/`PropertyValue` did not change the outcome.

A raw storage-layer injection (adding a `$853?` field to fragment `i5` via
kfxlib's Ion round-trip) was started and deliberately abandoned: a fresh
`LocalSymbolTable` only resolves `$853?` to ID 853 after full book decode, and
per the audit's direction the in-memory KAF result above already answers the
"does KAF tolerate it" question without byte-level mutation.

For anyone resuming that mutation: binary-Ion field SID 853 encodes as VarUInt bytes
`0x06 0xD5` (verified with KFX Input 2.34's Ion serializer), and each
`fragments.payload_value` blob is the raw struct with a 3-byte
`\xe0\x01\x00\xea` IVM prefix inside the fingerprint-wrapped SQLite.

## Confirmed facts

1. Java converter code declares property 853 in the KAF enum but has no Java reader/writer for it;
   the other Java literal-853 hits are unrelated tables.
2. `libshared.dylib`'s **string** references are property-table registration, but its x86_64 code
   also contains the numeric 853 parser and the same sequence-aware `BinaryStorage` implementation as
   Previewer's main executable. `BookFactory.nativeGetBook(String)` reaches that storage stack through
   the native KAF file-open path.
3. `yjsdk::handler_FileMetadata` parses 853 as a 32-bit numeric container-info field at handler
   offset `+0x3c`; `FUN_10144b6b0` copies that metadata block into `yjsdk::BinaryContainer`, where
   **`bcSequenceNumber` is at `+0x44`**. `BinaryContainer+0x3c/+0x40` are instead the
   format-capabilities offset/length. `BinaryContainer` constructors initialize `+0x44` to zero.
4. `yjsdk::BinaryStorage` initializes its sequence watermark (`+0xd8`) to zero and, on container
   load, treats `(current == 0) || (incoming >u current)` as the sequence-newer condition. The
   doc-symbol state install additionally requires successful document-symbol parsing, but the stored
   sequence watermark follows the `newer` flag itself. Equality does not satisfy the strict comparison.
5. A separate `BinaryStorage` lookup/selection path compares `BinaryContainer+0x44` values and, once
   the watermark is nonzero, retains the candidate with the larger unsigned sequence. With a zero
   watermark it uses a fallback path that does not require a larger sequence.
6. Container sequence numbers are propagated into `yjsdk::BinaryObjectStream+0x30`; that class's
   corresponding virtual accessor returns the value. `StorageStream` and `YJTextObjectStream`
   implement the same slot by returning zero.
7. KFX Input 2.34 knows 853 only as `$853?` and does not pop/interpret it from `container_info`; Go
   carries the Amazon-provided name but no behavior. Both process multi-container inputs by sorted
   file/path order rather than sequence arbitration; Python keeps the first duplicate fragment ID,
   while many Go typed maps overwrite with the later duplicate.
8. All 304 generated KDF instances in the local sweep declared the shared table through 853, while
   none used 853 in KDF fragment payload data (highest observed payload SID 790). Because 853 is a
   KFX container-info field, this does **not** test the delivery-container writer.
9. The live runtime resolves 853 ↔ `bcSequenceNumber` bidirectionally, and KAF's generic property
   layer accepts an integer value in memory.
10. `nativeSave` crashes identically with no modification (control), so persistence through that
    standalone save harness is unknown.

## Inference / remaining unknowns

- **Reader-side role is high confidence:** `bcSequenceNumber` is a container revision/order
  discriminator. The bundled KAF/YJSDK uses it to choose which of multiple matching binary containers
  is authoritative and carries the selected container sequence into object streams.
- **Zero is special:** reader code treats a zero storage watermark as sequence ordering not yet
  established and enables fallback selection behavior. This does not prove that a serialized
  `bcSequenceNumber=0` has a single universal producer meaning.
- **Producer policy remains unknown:** current evidence does not establish which packaging event
  increments the sequence, whether counters are global to the book or scoped to a container family,
  whether values can skip, or how 32-bit wraparound is handled. The reader compares them as unsigned
  integers without any wrap-aware arithmetic in the recovered paths.
- **Producer location remains the next target:** controlled Previewer outputs stop at KDF and do not
  exercise the delivered KFX/CONT writer where this field belongs. A real delivered container or a
  recovered packaging serializer is needed to establish how 853 is emitted.
- **Device behavior is still unproven:** the recovered semantics are Previewer 3.106's bundled
  KAF/YJSDK behavior; a Kindle device may implement the same policy, but this bundle alone does not
  establish that.

## Practical guidance

- Keep Go's `853 -> bcSequenceNumber` name; it is Amazon-provided and the recovered semantics now
  support treating it explicitly as a container sequence field.
- Do **not** invent a generation algorithm or default-increment policy in the converter yet. Preserve
  the raw 32-bit value if support is added before a producer sample is available.
- KFX Input currently leaves `$853?` as extra `container_info`; if a real retail sample carries it,
  capture the container version, `bcContId`, sequence value, and sibling containers before deciding
  how a reader should expose or normalize it.
- A real book containing multiple same-purpose/same-id containers with differing 853 values is the
  strongest next fixture: it can validate the recovered highest-sequence arbitration end-to-end.
- On the next Previewer bump, re-run the `PropertyNameUtil`/native-symbol tail checks and this numeric
  consumer audit; a changed comparison or additional field after 853 would be materially relevant.

## Reproduction

```sh
# JAR: full extract + byte search + literal-853 audit (see §1)
JAR='REFERENCE/Kindle Previewer 3.app/Contents/lib/fc/lib/EpubToKFXConverter-4.0.jar'
unzip -q -o "$JAR" -d /tmp/kp3-853/jar
grep -rla bcSequenceNumber /tmp/kp3-853/jar            # -> com/amazon/kaf/c/b.class only
javap -c -p com/amazon/kaf/c/b | grep -B3 'String bcSequenceNumber'

# Native: one-slice lea scan (§3) + Ghidra xrefs (§2)
./REFERENCE/ghidra x-ref to --project kindle_previewer_kaf \
  --program libshared.x86_64.dylib 0x633a5d

# Live runtime (§6): compile the probe (scripts/kp3 pattern, package-private adapters)
FC="$PWD/REFERENCE/Kindle Previewer 3.app/Contents/lib/fc"
javac --release 11 -cp "$FC/lib/EpubToKFXConverter-4.0.jar" -d /tmp/classes Kaf853Probe.java
DYLD_LIBRARY_PATH="$FC/lib" "$FC/jre/bin/java" -Dklibname=shared \
  -Djava.library.path="$FC/lib" -cp "$FC/lib/*:/tmp/classes" \
  com.amazon.kaf.jni.adapters.Kaf853Probe resolve <book.kdf>
```

The KDF sweep (§5) packaged each `book/` directory as a minimal KPF, decoded with
`YJ_Book(...).decode_book(retain_yj_locals=True)`, and walked every fragment
value for `$N`/`$N?` symbol usage; 304 books, zero 853 hits.
