#!/usr/bin/env python3

import argparse
import json
import os
import sys


def configure_paths():
    script_dir = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.dirname(script_dir)  # scripts/ -> repo root
    ref_root = os.path.join(repo_root, "REFERENCE", "Calibre_KFX_Input")
    sys.path.insert(0, ref_root)
    sys.path.insert(0, os.path.join(ref_root, "kfxlib", "calibre-plugin-modules"))


def fragment_summary(input_path):
    from kfxlib import YJ_Book
    from kfxlib.yj_to_epub import KFX_EPUB

    class Organizer(object):
        def __init__(self):
            self.book_symbols = set()

        replace_ion_data = KFX_EPUB.replace_ion_data
        organize_fragments_by_type = KFX_EPUB.organize_fragments_by_type

    book = YJ_Book(input_path)
    book.decode_book(retain_yj_locals=True)
    organizer = Organizer()
    organized = organizer.organize_fragments_by_type(book.fragments)

    summary = {
        "title": os.path.splitext(os.path.basename(input_path))[0],
        "types": {},
    }
    singleton_struct_types = {"$258", "$490", "$538", "$585"}
    for fragment_type, value in organized.items():
        if isinstance(value, dict):
            payload = {"count": 1 if fragment_type in singleton_struct_types else len(value)}
            if fragment_type not in singleton_struct_types:
                payload["ids"] = sorted(str(key) for key in value.keys())
            summary["types"][fragment_type] = payload
        elif isinstance(value, list):
            summary["types"][fragment_type] = {"count": len(value)}
        else:
            summary["types"][fragment_type] = {"count": 1}

    return summary


def main():
    configure_paths()

    parser = argparse.ArgumentParser(description="Emit reference KFX pipeline snapshots from the Calibre KFX Input code.")
    parser.add_argument("kind", choices=["fragment-summary"], help="Snapshot type to emit")
    parser.add_argument("--input", required=True, help="Input KFX path")
    args = parser.parse_args()

    if args.kind == "fragment-summary":
        payload = fragment_summary(args.input)
    else:
        raise SystemExit("unsupported snapshot kind")

    json.dump(payload, sys.stdout, indent=2, sort_keys=True)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
