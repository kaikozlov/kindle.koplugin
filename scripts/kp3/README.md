# Kindle Previewer semantic-oracle harness

Research tooling for interrogating the Kindle Previewer reference implementation. Nothing in this directory is used by the KOReader plugin at runtime.

## What it does

`run_probe.py` builds or accepts an EPUB, runs Amazon's bundled `EpubAdapterApp`, inspects the resulting KDF at the SQLite layer, and opens the same KDF through Amazon's bundled native KAF implementation.

```text
EPUB -> Amazon EpubAdapterApp -> book.kdf
                                |-> unwrapped SQLite
                                `-> native KAF typed graph
```

This is intended for controlled, one-feature-at-a-time experiments. It complements real consumer KFX samples; it does not replace them for historical or malformed-format compatibility.

## Requirements

- `REFERENCE/Kindle Previewer 3.app` from this research checkout.
- A host `javac`; probes are compiled with `--release 11` to match Previewer's bundled JRE.
- macOS/x86_64 compatibility sufficient to execute the bundled Previewer Java/native stack.

## Examples

```sh
# Minimal reflowable book
./scripts/kp3/run_probe.py --fixture minimal --workdir /tmp/kp3-minimal

# Table structure/span probe
./scripts/kp3/run_probe.py --fixture table --workdir /tmp/kp3-table

# Fixed-layout normalization probe
./scripts/kp3/run_probe.py --fixture fixed-layout --workdir /tmp/kp3-fixed

# Vertical Japanese + ruby + emphasis
./scripts/kp3/run_probe.py --fixture vertical-ruby --workdir /tmp/kp3-ruby

# Also dump Amazon's live property ID/name catalog
./scripts/kp3/run_probe.py --fixture minimal --catalog --workdir /tmp/kp3-catalog

# Compare the historical Go catalog with the live Amazon KAF table
./scripts/kp3/compare_catalog.py

# Generate Amazon KDF/KFX fixtures, then compare Python and Go reverse output
./scripts/kp3/reverse_compare.py --all --workdir /tmp/kp3-reverse
./scripts/kp3/reverse_compare.py --fixture footnote --diff

# Probe an existing EPUB instead of a built-in fixture
./scripts/kp3/run_probe.py --epub /path/to/book.epub --workdir /tmp/kp3-custom
```

The work directory preserves Amazon's conversion log, preprocessed source, wrapped KDF, unwrapped SQLite copy, and compiled probe classes.

`reverse_compare.py` adds a differential reverse path. It packages the Amazon-generated KDF as KPF, asks current KFX Input to serialize the decoded fragment graph into a single unencrypted KFX `CONT` container, then feeds that exact KFX to both current Python KFX Input and the historical Go implementation. This makes controlled Amazon-generated fixtures usable as parity tests even though the Go decoder does not understand KDF directly.

The KFX Input serializer is only a storage bridge in this experiment; both reverse implementations receive the same serialized KFX bytes. A mismatch therefore identifies a KFX->EPUB behavioral difference rather than an Amazon-producer difference.

## Native KAF caution

`KafSemanticProbe` intentionally traverses only a conservative subset of KAF. Some JNI adapter APIs have native ownership/lifetime assumptions; exploratory calls outside the current subset have produced a native JVM crash. The probe is therefore a one-shot subprocess and exits without broad object teardown.

Add new getters incrementally and validate them in an isolated subprocess before making them part of a corpus run.

## Fixture philosophy

Built-in fixtures currently cover:

- `minimal`: heading + paragraph baseline;
- `footnote`: EPUB 3 noteref/footnote mapping;
- `table`: caption, header/body rows, rowspan, and colspan;
- `fixed-layout`: minimal pre-paginated image-backed page;
- `vertical-ruby`: vertical Japanese, ruby, and emphasis;
- `link`: ordinary internal anchor link;
- `bidi`: RTL paragraph with an isolated LTR range;
- `list`: ordered-list start offset plus nested unordered list;
- `svg`: simple inline SVG normalization/rasterization.

Prefer a matrix of small semantic specimens over a few large synthetic books. A useful fixture should make one question easy to answer, for example:

- table structure and spans;
- footnote/endnote representation;
- bidi/writing-mode behavior;
- image/fixed-layout normalization;
- navigation targets;
- page spreads and illustrated layout;
- conditional content;
- document/page regions;
- SVG/KVG/path structures.

For each fixture, compare at least the producer log, typed KAF graph, and raw KDF/Ion structure before drawing conclusions.
