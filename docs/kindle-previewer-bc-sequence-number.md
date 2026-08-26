# Property 853 `bcSequenceNumber` — focused audit (Kindle Previewer 3.106)

Status: complete static + live-runtime audit, 2026-08-26

This is a narrow, deep follow-up to open question 3 in
`docs/kindle-previewer-reverse-engineering.md` (not edited here; the summary there
remains authoritative for the broader investigation).

Question: does shared symbol / KAF property **853 `bcSequenceNumber`** have any
reader, writer, serializer, or storage reference anywhere in the Kindle Previewer
3.106 bundle, and is it ever *used* (as opposed to declared) in actual Ion data?

Artifacts covered:

- `Contents/lib/fc/lib/EpubToKFXConverter-4.0.jar` (25,587 extracted classes)
- `Contents/lib/fc/lib/libshared.dylib` (x86_64 + arm64 slices)
- `Contents/lib/fc/bin/KindleImageProcessor`
- `Contents/MacOS/Kindle Previewer 3` (main binary)
- KFX Input 2.34.0 (`20260822`) Python reference
- 304 Amazon-generated KDF books (12 semantic fixture families, incl. comic/CMX variants)
- live native KAF runtime via isolated JNI subprocesses

## Executive answer

**853 is vocabulary, not behavior — in this bundle.** Every occurrence in every
Amazon binary and the entire Java converter is declaration/registration only:

| Artifact | Occurrences of the string | Nature |
| --- | ---: | --- |
| EpubToKFXConverter-4.0.jar | 1 class file | KAF property enum constant (declaration only) |
| libshared.dylib | 1 per arch slice | shared-symbol-table registration + `PropertyNameUtil` map builder |
| KindleImageProcessor | 1 | shared-symbol-table registration call |
| Kindle Previewer 3 (main) | 1 | shared-symbol-table registration call |
| KFX Input 2.34.0 | 0 by name; `$853?` placeholder | numeric table extent only |

No producer path emits it (304/304 generated books declare the shared table through
853 but use no symbol above 790), no consumer reads it, and the live KAF runtime
accepts it as a generic property but has no type/default semantics beyond its name.
The `nativeSave` path of the standalone KAF harness crashes even without
modification, so serialization persistence is honestly untested, not negative.

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

### 2. libshared.dylib: exactly two references, both registration

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

Both xrefs are declaration/registration. There is no third reference; nothing in
the 23k-function native code reads or writes the property.

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

### 3. Main binary and KindleImageProcessor: one registration call each

These binaries use no classic absolute pointers (chained fixups), so an x86-64
RIP-relative `lea` scan was run over `__text` targeting each string's vaddr:

| Binary | String vaddr | Code refs | Site |
| --- | ---: | ---: | --- |
| Kindle Previewer 3 | `0x10278b0c6` | 1 | lea at `0x1013fa5b0` |
| KindleImageProcessor | `0x102525460` | 1 | lea at `0x10058c8c0` |

Both sites disassemble to the identical pattern (raw bytes, e.g. main binary
`... ff 50 18 | 48 8b 75 d0 | 48 8b 06 | 48 8d 15 <disp=string>` → `lea rdi,[rbp-0x14]` →
`xor ecx,ecx` → `call [rax+0x18]`):

```text
call [rax+0x18]        ; previous registration
mov rsi,[rbp-0x10]
mov rax,[rsi]
lea rdx,[rip+...]      ; "bcSequenceNumber"
lea rdi,[rbp-0x14]
xor ecx,ecx            ; third arg 0
call [rax+0x18]        ; register(name, 0) — same slot as libshared's initializer
```

Same vtable slot +0x18, same three-argument shape as the final entries of
libshared's initializer. Both binaries embed the YJ reader symbol table and
register the name once; neither contains any other reference.

The Previewer GUI's renderer *does* actively consume **852** `page_regions`
(established previously); by contrast 853 has no analogous consumer anywhere.

### 4. KFX Input 2.34.0 and the Go plugin

- Python (`REFERENCE/KFX_Input`): `$853?` appears exactly once, as an anonymous
  placeholder in `kfxlib/yj_symbol_catalog.py`'s shared-table tail
  (`$852? $853? $854? ... $859`). No `bcSequenceNumber` string exists anywhere
  in the reference tree; no semantic code path mentions `$853` or `$853?`.
- Go plugin: `internal/kfx/catalog.ion` / goldens carry the real name
  `853 -> "bcSequenceNumber"` (sourced earlier from the live native resolver),
  and `yj_symbol_catalog_test.go` pins it. No Go code reads or writes the
  property.

### 5. Declaration vs use in 304 generated KDFs

Every Amazon-generated `book.kdf` under `/tmp` from all fixture runs
(304 books across the 12 semantic families, including comic/CMX/region variants
and two full `KFXGenApp` pipeline outputs) was decoded with KFX Input 2.34 and
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

### 6. Live KAF runtime: accepted as a generic property, no semantics

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

So KAF's generic property layer (`Container_setNativeProperty` /
`getNativeProperty`) accepts and stores the property on a Structure with no
validation error — consistent with a generic property store that has no
schema constraint for 853.

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

1. The string occurs in the JAR only in the KAF property enum; all other Java
   `853` literals are unrelated static arrays (8 files, individually verified).
2. libshared references it from exactly two places: the property-name map JNI
   builder and the shared-symbol-table registration initializer.
3. Main binary and KindleImageProcessor each contain exactly one code reference,
   the same registration call shape as libshared's initializer.
4. No reader, writer, serializer, or storage consumer of 853 exists in any
   inspected Java or native code of the Previewer 3.106 bundle.
5. KFX Input 2.34 knows 853 only as `$853?`; the Go plugin catalog carries the
   real name; neither uses it semantically.
6. All 304 generated books declare the shared table through 853 and use nothing
   above 790; declaration extent ≠ payload use.
7. The live runtime resolves 853 ↔ `bcSequenceNumber` bidirectionally, and KAF's
   generic property layer accepts an int value on a Structure in memory.
8. `nativeSave` crashes identically with no modification (control), so
   persistence through KAF save is untestable in this harness — unknown, not
   negative.

## Inference (clearly separated)

- **Family**: `bcSequenceNumber` sits at the tail of the `bc*` block whose other
  members are all KFX *container* storage fields. Moderate-confidence inference:
  it names a new book-container bookkeeping field (the name suggests a sequence
  number of some kind) appended to the container-info/entity vocabulary.
- **Producer**: whatever writes it is not in this bundle — consistent with the
  established pattern that the consumer-visible `bc*` fields are written by
  KFX container packaging rather than the EPUB→YJ converter. Candidate writers
  would be Amazon packaging/delivery tooling or a newer converter version; no
  evidence exists in 3.106.
- **Consumer**: a Kindle-device reader or newer YJReaderSDK plausibly reads it;
  Previewer's reader registers the name but never reads it. Speculative.
- Any meaning beyond "book-container-associated sequence-number-like field" is
  **unsupported** by current evidence and should not be encoded into names,
  defaults, or behavior in KFX Input or the Go port beyond the raw name.

## Practical guidance

- Keep Go's `853 -> bcSequenceNumber` name (correct, Amazon-provided).
- Keep KFX Input-style tolerance for unknown shared SIDs past the known table
  (`$N?` placeholders, skip on decode) — that is exactly the right posture for a
  declared-but-unused symbol.
- If a real-world book ever presents SID 853 in a payload, its fragment family
  (expected: container info/entity, by naming family) would settle the producer
  question immediately; capture it as a fixture.
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
