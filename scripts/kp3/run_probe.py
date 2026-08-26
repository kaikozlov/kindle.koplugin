#!/usr/bin/env python3
"""Run a controlled EPUB -> Amazon KDF -> KAF semantic probe.

This is research tooling, not plugin runtime code. It invokes the exact converter and
native KAF implementation bundled with the checked-in Kindle Previewer reference.
"""

from __future__ import annotations

import argparse
import os
from pathlib import Path
import shutil
import sqlite3
import subprocess
import sys
import tempfile

from make_fixture import FIXTURE_NAMES, write_epub


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_PREVIEWER = ROOT / "REFERENCE" / "Kindle Previewer 3.app"
FINGERPRINT_SIGNATURE = b"\xfa\x50\x0a\x5f"
FINGERPRINT_OFFSET = 1024
FINGERPRINT_RECORD_LEN = 1024
DATA_RECORD_LEN = 1024
DATA_RECORD_COUNT = 1024


def run(command: list[str], *, env: dict[str, str] | None = None, capture: bool = False) -> subprocess.CompletedProcess[str]:
    print("+", " ".join(command), file=sys.stderr)
    return subprocess.run(
        command,
        env=env,
        check=True,
        text=True,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.PIPE if capture else None,
    )


def fc_paths(previewer: Path) -> tuple[Path, Path, Path]:
    fc = previewer / "Contents" / "lib" / "fc"
    jar = fc / "lib" / "EpubToKFXConverter-4.0.jar"
    java = fc / "jre" / "bin" / "java"
    javac = shutil.which("javac")
    if not fc.is_dir() or not jar.is_file() or not java.is_file():
        raise SystemExit(f"invalid Kindle Previewer reference: {previewer}")
    if javac is None:
        raise SystemExit("javac not found in PATH")
    return fc, jar, Path(javac)


def producer_env(fc: Path, jar: Path) -> dict[str, str]:
    env = os.environ.copy()
    # The Java orchestration code itself uses the Windows-style spelling "Path"
    # even on macOS in several places, so provide both spellings deliberately.
    augmented_path = os.pathsep.join([env.get("PATH", ""), str(fc / "lib")])
    env.update({
        "PATH": augmented_path,
        "Path": augmented_path,
        "phantomjs_home_dir": str(fc),
        "js_scripts_home_dir": str(fc),
        "semantic_mapping_dir": str(fc) + os.sep,
        "style_mapping_dir": str(fc),
        "style_merger_dir": str(fc) + os.sep,
        "yj_character_fixer_base_dir": str(fc),
        "yjhtmlcleaner_path": str(fc / "bin" / "htmlcleanerapp"),
        "CSS_HOME_DIR": str(fc / "rasterfonts"),
        "YJCONVERSION_ENV_ROOT": str(fc),
        "MERGED_JAR_FILE_PATH": str(jar),
        "DYLD_LIBRARY_PATH": str(fc / "lib"),
        "YJ_ASCII_UNICODE_CONVERTER_DATA_DIR": str(fc),
    })
    return env


def produce_kdf(fc: Path, jar: Path, epub: Path, out_dir: Path, temp_dir: Path) -> Path:
    java = fc / "jre" / "bin" / "java"
    out_dir.mkdir(parents=True, exist_ok=True)
    temp_dir.mkdir(parents=True, exist_ok=True)
    command = [
        str(java),
        "-Dfile.encoding=UTF-8",
        "-Djava.awt.headless=true",
        f"-Djava.library.path={fc / 'lib'}",
        "-Dklibname=shared",
        "-cp", str(fc / "lib" / "*"),
        "com.amazon.adapter.common.app.EpubAdapterApp",
        str(epub), str(out_dir), str(temp_dir),
        "--write-to-db", "--persist-yj", "--do-graceful-error-handling",
        "--log-level", "WARNING",
    ]
    run(command, env=producer_env(fc, jar))
    kdf = out_dir / "book" / "book.kdf"
    if not kdf.is_file():
        raise SystemExit(f"Amazon producer returned without {kdf}")
    return kdf


def unwrap_sqlite_fingerprints(data: bytes) -> tuple[bytes, int]:
    count = 0
    data_offset = FINGERPRINT_OFFSET
    while len(data) >= data_offset + FINGERPRINT_RECORD_LEN:
        if data[data_offset:data_offset + len(FINGERPRINT_SIGNATURE)] != FINGERPRINT_SIGNATURE:
            break
        data = data[:data_offset] + data[data_offset + FINGERPRINT_RECORD_LEN:]
        count += 1
        data_offset += DATA_RECORD_LEN * DATA_RECORD_COUNT
    return data, count


def sqlite_summary(kdf: Path, destination: Path) -> None:
    data, count = unwrap_sqlite_fingerprints(kdf.read_bytes())
    destination.write_bytes(data)
    print(f"\n-- KDF SQLite --\nfingerprints_removed={count} wrapped_bytes={kdf.stat().st_size} sqlite_bytes={len(data)}")
    connection = sqlite3.connect(destination)
    try:
        cursor = connection.cursor()
        tables = [row[0] for row in cursor.execute("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")]
        print("tables=" + ",".join(tables))
        if "capabilities" in tables:
            print("capabilities:")
            for row in cursor.execute("SELECT * FROM capabilities ORDER BY key, version"):
                print("  " + "\t".join(map(str, row)))
        if "fragment_properties" in tables:
            print("fragment_properties:")
            for row in cursor.execute("SELECT id,key,value FROM fragment_properties ORDER BY id,key,value"):
                print("  " + "\t".join(map(str, row)))
        if "fragments" in tables:
            print("fragments:")
            for row in cursor.execute("SELECT id,payload_type,length(payload_value) FROM fragments ORDER BY id"):
                print("  " + "\t".join(map(str, row)))
    finally:
        connection.close()


def compile_probes(jar: Path, javac: Path, classes: Path) -> None:
    sources = sorted((Path(__file__).parent / "com" / "amazon" / "kaf" / "jni" / "adapters").glob("*.java"))
    classes.mkdir(parents=True, exist_ok=True)
    run([str(javac), "--release", "11", "-cp", str(jar), "-d", str(classes), *map(str, sources)])


def kaf_command(fc: Path, jar: Path, classes: Path, klass: str, *args: str) -> list[str]:
    return [
        str(fc / "jre" / "bin" / "java"),
        "-Dklibname=shared",
        f"-Djava.library.path={fc / 'lib'}",
        "-cp", os.pathsep.join([str(jar), str(classes)]),
        f"com.amazon.kaf.jni.adapters.{klass}",
        *args,
    ]


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group(required=False)
    source.add_argument("--epub", type=Path, help="use an existing EPUB")
    source.add_argument("--fixture", choices=FIXTURE_NAMES, default="minimal")
    parser.add_argument("--previewer", type=Path, default=DEFAULT_PREVIEWER)
    parser.add_argument("--workdir", type=Path, help="retain all producer output here")
    parser.add_argument("--catalog", action="store_true", help="also dump the live KAF property catalog")
    parser.add_argument(
        "--symbol-range", metavar="START:END",
        help="also dump DigitalBook native symbol names for an inclusive ID range",
    )
    parser.add_argument(
        "--positions", action="store_true",
        help="also dump the native BookPositionInfo view (pid/location/eid/kfxid/sections)",
    )
    parser.add_argument("--no-sqlite", action="store_true", help="skip raw SQLite summary")
    args = parser.parse_args()

    fc, jar, javac = fc_paths(args.previewer)
    if args.workdir:
        workdir = args.workdir.resolve()
        workdir.mkdir(parents=True, exist_ok=True)
        cleanup = None
    else:
        cleanup = tempfile.TemporaryDirectory(prefix="kp3-probe-")
        workdir = Path(cleanup.name)

    try:
        if args.epub:
            epub = args.epub.resolve()
        else:
            epub = workdir / f"{args.fixture}.epub"
            write_epub(args.fixture, epub)

        out_dir = workdir / "out"
        temp_dir = workdir / "tmp"
        classes = workdir / "classes"
        kdf = produce_kdf(fc, jar, epub, out_dir, temp_dir)
        print(f"\nKDF={kdf}")

        if not args.no_sqlite:
            sqlite_summary(kdf, workdir / "book.unwrapped.kdf")

        compile_probes(jar, javac, classes)
        print("\n-- KAF semantic graph --")
        run(kaf_command(fc, jar, classes, "KafSemanticProbe", str(kdf)))

        if args.catalog:
            print("\n-- live KAF property catalog --")
            run(kaf_command(fc, jar, classes, "KafPropertyCatalog"))

        if args.symbol_range:
            try:
                start, end = (int(part) for part in args.symbol_range.split(":", 1))
            except (ValueError, TypeError):
                raise SystemExit("--symbol-range must be START:END")
            if end < start:
                raise SystemExit("--symbol-range END must be >= START")
            print(f"\n-- native DigitalBook symbols {start}..{end} --")
            run(kaf_command(fc, jar, classes, "KafSymbolCatalog", str(kdf), str(start), str(end)))

        if args.positions:
            print("\n-- native BookPositionInfo --")
            run(kaf_command(fc, jar, classes, "KafPositionProbe", str(kdf)))

        if args.workdir:
            print(f"\nworkdir={workdir}")
    finally:
        if cleanup is not None:
            cleanup.cleanup()


if __name__ == "__main__":
    main()
