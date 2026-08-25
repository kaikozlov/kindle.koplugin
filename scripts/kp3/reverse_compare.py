#!/usr/bin/env python3
"""Differentially reverse Amazon-generated KDF through Python KFX Input and the Go port.

Pipeline:

    controlled EPUB -> Amazon EpubAdapterApp -> KDF/KPF
                                             -> Python KFX serializer -> single KFX
                                                                       |-> Python -> EPUB
                                                                       `-> Go     -> EPUB

The Python KFX serializer is used only to bridge Amazon's KDF storage format to the
single CONT container format understood by both reverse implementations. The semantic
comparison is between the Python and Go KFX->EPUB outputs from the same serialized KFX.
"""

from __future__ import annotations

import argparse
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(ROOT / "scripts"))

from make_fixture import FIXTURE_NAMES, write_epub  # noqa: E402
from run_probe import DEFAULT_PREVIEWER, fc_paths, produce_kdf  # noqa: E402
from parity_diff import diff_epubs, print_text_diff  # noqa: E402


def python_paths() -> None:
    ref = ROOT / "REFERENCE" / "KFX_Input"
    modules = ref / "kfxlib" / "calibre-plugin-modules"
    if not ref.is_dir():
        raise SystemExit(f"KFX Input reference not found: {ref}")
    sys.path.insert(0, str(ref))
    sys.path.insert(0, str(modules))


def package_kpf(book_dir: Path, destination: Path) -> None:
    """Package Amazon's generated book directory as a minimal KPF ZIP."""
    kdf = book_dir / "book.kdf"
    if not kdf.is_file():
        raise RuntimeError(f"missing generated KDF: {kdf}")
    with zipfile.ZipFile(destination, "w", zipfile.ZIP_STORED) as zf:
        for path in sorted(book_dir.rglob("*")):
            if path.is_file() and path.name != "book.kdf-journal":
                zf.write(path, path.relative_to(book_dir).as_posix())


def kpf_to_single_kfx(kpf: Path, destination: Path) -> None:
    """Decode KPF/KDF with current KFX Input and serialize its fragments as one KFX CONT."""
    python_paths()
    from kfxlib import YJ_Book
    from kfxlib.kfx_container import KfxContainer

    book = YJ_Book(str(kpf))
    book.decode_book()
    # convert_to_single_kfx intentionally rejects prepublication KPF. For this research
    # bridge we need exactly its serializer step after KPF decode, without changing the
    # decoded fragment graph.
    data = KfxContainer(book.symtab, fragments=book.fragments).serialize()
    destination.write_bytes(data)


def convert_python(kfx: Path, destination: Path) -> None:
    python_paths()
    from kfxlib import YJ_Book

    destination.write_bytes(YJ_Book(str(kfx)).convert_to_epub())


def convert_go(kfx: Path, destination: Path) -> tuple[int, str, str]:
    result = subprocess.run(
        ["go", "run", "./cmd/kindle-helper", "convert", "-input", str(kfx), "-output", str(destination)],
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    return result.returncode, result.stdout, result.stderr


def compare_fixture(name: str, root: Path, previewer: Path, show_diff: bool) -> int:
    workdir = root / name
    if workdir.exists():
        shutil.rmtree(workdir)
    workdir.mkdir(parents=True)

    source = workdir / f"{name}.epub"
    write_epub(name, source)
    fc, jar, _ = fc_paths(previewer)
    kdf = produce_kdf(fc, jar, source, workdir / "out", workdir / "tmp")

    kpf = workdir / f"{name}.kpf"
    kfx = workdir / f"{name}.kfx"
    python_epub = workdir / "python.epub"
    go_epub = workdir / "go.epub"
    package_kpf(kdf.parent, kpf)
    kpf_to_single_kfx(kpf, kfx)
    convert_python(kfx, python_epub)
    go_rc, go_stdout, go_stderr = convert_go(kfx, go_epub)

    print(f"\n== {name} ==")
    print(f"source={source}")
    print(f"kdf={kdf.stat().st_size} bytes kfx={kfx.stat().st_size} bytes")
    if go_rc != 0 or not go_epub.is_file():
        print(f"go=FAILED rc={go_rc}")
        if go_stdout.strip():
            print(go_stdout.rstrip())
        if go_stderr.strip():
            print(go_stderr.rstrip())
        return 1

    diffs = diff_epubs(str(python_epub), str(go_epub))
    structural = [d for d in diffs if d.category == "structural" and d.kind != "timestamp_only"]
    images = [d for d in diffs if d.category == "image"]
    other = [d for d in diffs if d.category == "other"]
    timestamp = [d for d in diffs if d.kind == "timestamp_only"]
    print(
        f"diffs={len(diffs)} structural={len(structural)} image={len(images)} "
        f"other={len(other)} timestamp_only={len(timestamp)}"
    )
    if go_stderr.strip():
        for line in go_stderr.splitlines():
            if "Failed to locate" in line or "error:" in line.lower() or "warning" in line.lower():
                print(f"go-log: {line}")
    for d in diffs:
        print(f"  {d.category:10s} {d.kind:14s} {d.fname}")
        if show_diff and d.category == "structural" and d.kind == "content_diff":
            print_text_diff(d)
    return 0


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--fixture", choices=FIXTURE_NAMES)
    group.add_argument("--all", action="store_true")
    parser.add_argument("--previewer", type=Path, default=DEFAULT_PREVIEWER)
    parser.add_argument("--workdir", type=Path)
    parser.add_argument("--diff", action="store_true", help="print structural unified diffs")
    args = parser.parse_args()

    names = FIXTURE_NAMES if args.all or args.fixture is None else [args.fixture]
    if args.workdir:
        root = args.workdir.resolve()
        root.mkdir(parents=True, exist_ok=True)
        cleanup = None
    else:
        cleanup = tempfile.TemporaryDirectory(prefix="kp3-reverse-")
        root = Path(cleanup.name)

    try:
        failures = sum(compare_fixture(name, root, args.previewer.resolve(), args.diff) for name in names)
        print(f"\nworkdir={root}")
        if failures:
            raise SystemExit(1)
    finally:
        if cleanup is not None:
            cleanup.cleanup()


if __name__ == "__main__":
    main()
