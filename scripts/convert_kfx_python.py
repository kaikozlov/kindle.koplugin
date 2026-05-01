#!/usr/bin/env python3
"""Convert KFX to EPUB using the Calibre KFX Input Python pipeline from REFERENCE/."""

import argparse
import os
import sys


def main():
    script_dir = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.dirname(script_dir)
    ref_root = os.path.join(repo_root, "REFERENCE", "Calibre_KFX_Input")
    sys.path.insert(0, ref_root)
    sys.path.insert(0, os.path.join(ref_root, "kfxlib", "calibre-plugin-modules"))

    from kfxlib import YJ_Book

    parser = argparse.ArgumentParser(description="Convert KFX to EPUB using Calibre KFX Input Python pipeline")
    parser.add_argument("--input", required=True, help="Input KFX path")
    parser.add_argument("--output", required=True, help="Output EPUB path")
    args = parser.parse_args()

    # Load symbol catalog for $N -> real name translation
    catalog_path = os.path.join(repo_root, "REFERENCE", "kfx_symbol_catalog.ion")

    book = YJ_Book(args.input, symbol_catalog_filename=catalog_path)
    epub_data = book.convert_to_epub()
    with open(args.output, "wb") as f:
        f.write(epub_data)
    print(f"Converted {args.input} -> {args.output} ({len(epub_data)} bytes)")


if __name__ == "__main__":
    main()
