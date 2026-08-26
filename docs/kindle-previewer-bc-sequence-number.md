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

**853 is a real native KFX container-info field; the current controlled KDF corpus does not exercise its producer.**
The initial string-xref audit was insufficient: Previewer's native code often accesses YJ/KFX
properties by numeric ID rather than by name (as the known `page_regions` consumer does). A
follow-up exact-immediate audit found an explicit container-info parser branch for property 853.

| Artifact | Occurrences of the string | Nature |
| --- | ---: | --- |
| EpubToKFXConverter-4.0.jar | 1 class file | KAF property enum constant (declaration only) |
| libshared.dylib | 1 per arch slice | shared-symbol-table registration + `PropertyNameUtil` map builder |
| KindleImageProcessor | 1 string | shared-symbol-table registration; exact `0x355` immediates audited separately |
| Kindle Previewer 3 (main) | 1 string | shared-symbol registration **plus numeric property-853 reader** |
| KFX Input 2.34.0 | 0 by name; `$853?` placeholder | numeric table extent only |

In the 304 generated KDF instances swept in this investigation, every book declared the shared
table through 853 but none used 853 in a **KDF fragment payload** (highest observed payload SID: 790).
That negative result is now known to be orthogonal to the likely wire location: `bcSequenceNumber` belongs to
KFX container-info, not the KDF application fragment graph. The native Previewer reader **does** recognize 853
in the same container-info parser that consumes
`bcContId`, compression/DRM/chunk metadata, document-symbol offsets, and format-capability offsets.
It coerces the incoming numeric value to an integer and stores it in a dedicated 32-bit field.
The live generic KAF property API also accepts/read-backs 853 in memory. `nativeSave` crashes even
on an unmodified save-only control in the standalone harness, so KAF-save persistence remains
untested, not negative.

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
no native behavior: shared properties are commonly accessed by numeric ID. The follow-up numeric
immediate audit below is therefore the authoritative behavioral check.

**Naming family (contextual).** The same initializer reveals the neighbors
853 lives among — a contiguous `bc*` block:

```text
bcContId  bcComprType  bcDRMScheme  bcChunkSize  bcIndexTabOffset  bcIndexTabLength
bcDocSymbolOffset  bcDocSymbolLength  bcRawMedia  bcRawFont  bcFCapabilitiesOffset  bcFCapabilitiesLength
```

KFX Input decodes these as **KFX container storage fields** (`kfx_container.py`:
`$409` id, `$410` compression, `$411` DRM scheme, `$412` chunk size, `$415/$416`
document-symbol offset/length, `$417` raw media, `$418` raw font, `$422/$423`
capabilities offset/length). `bcSequenceNumber` is appended at the end of this
family. That is naming evidence for "book-container storage bookkeeping," not
proof of semantics — see *Inference* below.

### 3. Native numeric-ID audit: Previewer explicitly parses `bcSequenceNumber`

A follow-up disassembly audit searched the x86-64 native binaries for exact immediate `0x355`
(decimal 853), then classified every hit by function context. This was necessary because a
string-only xref audit misses property accessors that pass or compare numeric IDs.

The decisive hit is in the Previewer main binary at `FUN_10144b490`
(`0x10144b490..0x10144b66e`; compare at `0x10144b51d`). Ghidra reconstructs the routine as a
property-ID visitor over one container-info object. Its switch is not ambiguous: the handled IDs
map exactly to the KFX `bc*` container header family:

```text
409  bcContId                -> object +0x10
410  bcComprType             -> object +0x18
411  bcDRMScheme             -> object +0x1c
412  bcChunkSize             -> object +0x20
413  bcIndexTabOffset        -> object +0x24
414  bcIndexTabLength        -> object +0x28
415  bcDocSymbolOffset       -> object +0x2c
416  bcDocSymbolLength       -> object +0x30
594  bcFCapabilitiesOffset   -> object +0x34
595  bcFCapabilitiesLength   -> object +0x38
853  bcSequenceNumber        -> object +0x3c
```

For 853 the decompiled branch is:

```c
else if (param_3 == 0x355) {
    if ((*(byte *)(param_4 + 1) & 1) == 0)
        *(uint32_t *)(param_1 + 0x3c) = *(uint32_t *)(param_4 + 8);
    else
        *(int32_t *)(param_1 + 0x3c) = (int)*(float *)(param_4 + 8);
}
```

So **`bcSequenceNumber` is not declaration-only**. Previewer's native container reader recognizes
it as a numeric container-info field and stores a 32-bit integer representation in a dedicated
slot. What that integer sequences (container generations, chunks, revisions, etc.) is still not
established by this path alone.

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

For anyone resuming that mutation: binary-Ion field SID 853 encodes as varuint
`0xDA 0x55`, and each `fragments.payload_value` blob is the raw struct with a
3-byte `\xe0\x01\x00\xea` IVM prefix inside the fingerprint-wrapped SQLite.

## Confirmed facts

1. Java converter code declares property 853 in the KAF enum but has no Java reader/writer for it;
   the other Java literal-853 hits are unrelated tables.
2. `libshared.dylib`'s **string** references are property-table registration, not field consumption.
3. Previewer's main native binary has an explicit numeric property visitor that handles the exact
   `bc*` container-info family and stores **853 `bcSequenceNumber`** as a 32-bit integer at object
   offset `+0x3c`.
4. Other exact native `0x355` sites are table-boundary/YJSDK-extent behavior or unrelated third-party
   library constants; no second field-specific reader was identified in KindleImageProcessor.
5. KFX Input 2.34 knows 853 only as `$853?` and does not pop/interpret it from `container_info`; Go
   carries the real name but no behavior.
6. All 304 generated KDF instances in the local sweep declared the shared table through 853, while
   none used 853 in KDF fragment payload data (highest observed payload SID 790). Because 853 is a KFX
   container-info field, this does **not** test the delivery-container writer.
7. The live runtime resolves 853 ↔ `bcSequenceNumber` bidirectionally, and KAF's generic property
   layer accepts an integer value in memory.
8. `nativeSave` crashes identically with no modification (control), so persistence through that
   standalone save harness is unknown.

## Inference (clearly separated)

- **Format family is now high confidence:** the native parser handles 853 in the same routine and
  object layout as `bcContId`, compression/DRM/chunk metadata, index/document-symbol offsets, and
  format-capability offsets. `bcSequenceNumber` is therefore a KFX **container-info** field, not an
  EPUB/YJ content property in any meaningful application-level sense.
- **Exact semantics remain open:** the name and 32-bit storage strongly indicate a sequence number,
  but the reader path alone does not establish what is sequenced or how the value changes.
- **Producer remains unknown:** the controlled Previewer outputs stop at KDF and therefore do not exercise the
  delivery-container writer where this field belongs. A packaging/delivery serializer is the primary place to inspect next;
  current evidence does not establish whether Previewer 3.106 can emit it.
- **Reader behavior is proven only for Previewer's native container parser.** Device readers may use
  the field too, but that is not established by this bundle.

## Practical guidance

- Keep Go's `853 -> bcSequenceNumber` name (correct, Amazon-provided).
- Treat 853 as a **container-info integer field** in documentation/model naming. Do not invent its
  sequencing semantics until a producer sample or downstream use is recovered.
- KFX Input currently leaves `$853?` as extra `container_info`; if a real retail sample carries it,
  capture the value and container version before deciding whether to silently accept/store it.
- A real-world delivered KFX/CONT container carrying 853 would settle the producer/version question immediately;
  preserve its container version and `container_info` as a fixture.
- On the next Previewer bump, re-run the two cheap checks first: the
  `PropertyNameUtil` table extent and the `DigitalBook.nativeGetSymbolName`
  tail (`scripts/kp3/run_probe.py --catalog` / `--symbol-range`) — a new entry
  after `bcSequenceNumber` or a change in its enum position is the earliest
  signal that the vocabulary grew again.

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
