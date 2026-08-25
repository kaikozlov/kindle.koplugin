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

# Probe an existing EPUB instead of a built-in fixture
./scripts/kp3/run_probe.py --epub /path/to/book.epub --workdir /tmp/kp3-custom
```

The work directory preserves Amazon's conversion log, preprocessed source, wrapped KDF, unwrapped SQLite copy, and compiled probe classes.

## Native KAF caution

`KafSemanticProbe` intentionally traverses only a conservative subset of KAF. Some JNI adapter APIs have native ownership/lifetime assumptions; exploratory calls outside the current subset have produced a native JVM crash. The probe is therefore a one-shot subprocess and exits without broad object teardown.

Add new getters incrementally and validate them in an isolated subprocess before making them part of a corpus run.

## Fixture philosophy

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
