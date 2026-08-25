#!/usr/bin/env python3
"""Self-contained Kindle helper for KFX conversion and per-book DRM access.

The KFX→EPUB conversion uses kfxlib (John Howell's Calibre KFX Input plugin)
directly — no Calibre installation required. DRMION books are decrypted using
cached page keys, with JIT device-specific extraction exposed through
``extract-key``.
"""

import argparse
import base64
import json
import os
import re
import sys
import zipfile

# ---------------------------------------------------------------------------
# kfxlib setup — ensure bundled plugin modules (pypdf, typing_extensions) are
# importable even when calibre is not installed. The release package ships the
# source tree next to this entry point inside its self-contained CPython runtime.
# ---------------------------------------------------------------------------
_THIS_DIR = os.path.dirname(os.path.abspath(__file__))
_PLUGIN_MODULES = os.path.join(_THIS_DIR, "kfxlib", "calibre-plugin-modules")
if not os.path.isdir(_PLUGIN_MODULES):
    # Compatibility with older package layouts.
    _PLUGIN_MODULES = os.path.join(_THIS_DIR, "calibre-plugin-modules")
if os.path.isdir(_PLUGIN_MODULES) and _PLUGIN_MODULES not in sys.path:
    sys.path.insert(0, _PLUGIN_MODULES)

from epub_position import PositionTranslationError, translate_native_position, translate_pair
from annotation_position import normalize_annotation_ends
from kfx_position_adapter import position_metadata_conversion
from position_map import write_position_map

from dedrm.drmion import (
    CONT_SIGNATURE,
    DRMION_SIGNATURE,
    decrypt as decrypt_drmion,
    encryption_key_ids,
)

VERSION = 1

# ---------------------------------------------------------------------------
# JSON output helpers (same protocol as the Go binary)
# ---------------------------------------------------------------------------

def write_json(obj):
    sys.stdout.write(json.dumps(obj, ensure_ascii=False) + "\n")
    sys.stdout.flush()


def exit_json(obj, code=0):
    write_json(obj)
    sys.exit(code)


# ---------------------------------------------------------------------------
# DRMION decryption
# ---------------------------------------------------------------------------


def _decode_page_key(entry):
    key_value = entry.get("page_key_128", "") if isinstance(entry, dict) else entry
    if not isinstance(key_value, str) or not key_value:
        return None
    try:
        return bytes.fromhex(key_value)
    except ValueError:
        try:
            return base64.b64decode(key_value)
        except Exception:
            return None


def _find_page_key(kfx_path, cache_dir):
    """Load the cached page key for *kfx_path* from drm_keys.json."""
    if not cache_dir:
        return None
    keys_file = os.path.join(cache_dir, "drm_keys.json")
    if not os.path.isfile(keys_file):
        return None

    with open(keys_file, "r") as f:
        keys_data = json.load(f)

    books = keys_data.get("books", {})

    # Prefer the stable identifier embedded in DRMION EnvelopeMetadata. This
    # remains valid when Amazon changes or relocates the on-disk filename.
    try:
        with open(kfx_path, "rb") as source_file:
            source_data = source_file.read()
        for key_id in encryption_key_ids(source_data):
            page_key = _decode_page_key(keys_data.get("keys", {}).get(key_id))
            if page_key is not None:
                return page_key
    except Exception:
        pass

    abs_path = os.path.abspath(kfx_path)

    # Next prefer an exact source path recorded by cache version 2.
    for entry in books.values():
        if isinstance(entry, dict) and entry.get("source_path") == abs_path:
            page_key = _decode_page_key(entry)
            if page_key is not None:
                return page_key

    # Legacy cache fallback: match ASIN/book ID embedded in the filename.
    basename = os.path.basename(kfx_path)
    for book_id, entry in books.items():
        if book_id in basename:
            page_key = _decode_page_key(entry)
            if page_key is not None:
                return page_key

    # Legacy caches may use a source path as the books-table key.
    return _decode_page_key(books.get(abs_path) or books.get(kfx_path))


def _decrypt_drmion(data, page_key):
    """Decrypt a DRMION blob using the shared DeDRM parser."""
    return decrypt_drmion(data, page_key)


# ---------------------------------------------------------------------------
# convert — KFX → EPUB using kfxlib
# ---------------------------------------------------------------------------

def cmd_convert(args):
    if not args.input:
        print("convert: --input is required", file=sys.stderr)
        sys.exit(2)
    if not args.output:
        print("convert: --output is required", file=sys.stderr)
        sys.exit(2)

    input_path = args.input
    output_path = args.output
    cache_dir = getattr(args, "cache_dir", "") or ""

    try:
        # Handle DRMION: decrypt first, write to temp KFX-zip, then convert
        with open(input_path, "rb") as f:
            header = f.read(8)

        convert_path = input_path
        if header.startswith(DRMION_SIGNATURE):
            with open(input_path, "rb") as f:
                data = f.read()

            page_key = _find_page_key(input_path, cache_dir)
            try:
                # A DRMION envelope may contain only PlainText pages and need
                # no voucher or key. DeDRM requests the key lazily only when it
                # encounters an EncryptedPage.
                cont_data = _decrypt_drmion(data, page_key)
            except Exception as e:
                if page_key is None:
                    exit_json({
                        "version": VERSION,
                        "ok": False,
                        "code": "drm",
                        "message": "DRM-protected book: no cached page key found",
                    })
                exit_json({
                    "version": VERSION,
                    "ok": False,
                    "code": "drm",
                    "message": f"DRM decryption failed: {e}",
                })

            # Write decrypted CONT as a KFX-zip for kfxlib
            import tempfile
            tmp = tempfile.NamedTemporaryFile(suffix=".kfx-zip", delete=False)
            try:
                with zipfile.ZipFile(tmp, "w") as zf:
                    zf.writestr("main.kfx", cont_data)

                    # Collect sidecar blobs (CONT + DRMION containers from .sdr)
                    sidecar_root = os.path.splitext(input_path)[0] + ".sdr"
                    if os.path.isdir(sidecar_root):
                        for dirpath, _, filenames in os.walk(sidecar_root):
                            for fn in filenames:
                                fpath = os.path.join(dirpath, fn)
                                try:
                                    blob = open(fpath, "rb").read()
                                except OSError:
                                    continue
                                rel = os.path.relpath(fpath, sidecar_root)
                                if blob.startswith(DRMION_SIGNATURE):
                                    try:
                                        blob = _decrypt_drmion(blob, page_key)
                                    except Exception:
                                        continue
                                if blob.startswith(CONT_SIGNATURE):
                                    zf.writestr(rel, blob)
                convert_path = tmp.name
            finally:
                tmp.close()

        # Use kfxlib to convert
        from kfxlib import YJ_Book

        book = YJ_Book(convert_path)
        # Keep vendored kfxlib pristine while layering the text-free KFX
        # coordinate metadata needed for exact Kindle/KOReader position sync.
        with position_metadata_conversion():
            epub_data = book.convert_to_epub(epub2_desired=False)

        # Write output plus the conversion-time position map used by the
        # plugin's in-process exact-position sync.
        os.makedirs(os.path.dirname(os.path.abspath(output_path)), exist_ok=True)
        with open(output_path, "wb") as f:
            f.write(epub_data)
        map_path = re.sub(r"\.epub$", ".positions.json", output_path)
        try:
            write_position_map(output_path, map_path)
        except Exception as map_error:
            print(f"position map failed: {map_error}", file=sys.stderr)

        # Cleanup temp file if we created one
        if convert_path != input_path:
            try:
                os.unlink(convert_path)
            except OSError:
                pass

        exit_json({
            "version": VERSION,
            "ok": True,
            "output_path": output_path,
        })

    except Exception as e:
        code = "error"
        message = str(e)

        # Check for DRM error from kfxlib
        try:
            from kfxlib import KFXDRMError
            if isinstance(e, KFXDRMError):
                code = "drm"
        except ImportError:
            pass

        exit_json({
            "version": VERSION,
            "ok": False,
            "code": code,
            "message": message,
        })


# ---------------------------------------------------------------------------
# CLI argument parsing
# ---------------------------------------------------------------------------

def cmd_extract_key(args):
    """Extract the decryption key for a single book."""
    if not args.input:
        print("extract-key: --input is required", file=sys.stderr)
        sys.exit(2)

    input_path = args.input
    cache_dir = getattr(args, "cache_dir", "") or ""
    plugin_dir = getattr(args, "plugin_dir", "") or ""

    if not plugin_dir:
        plugin_dir = os.path.dirname(os.path.abspath(sys.argv[0]))
        if not os.path.isdir(os.path.join(plugin_dir, "lib")):
            parent = os.path.dirname(plugin_dir)
            if os.path.isdir(os.path.join(parent, "lib")):
                plugin_dir = parent

    try:
        from dedrm.drm_init import extract_book_key
        result = extract_book_key(input_path, plugin_dir, cache_dir)
        exit_json({
            "version": VERSION,
            **result,
        })
    except Exception as e:
        exit_json({
            "version": VERSION,
            "ok": False,
            "message": str(e),
        })


def cmd_translate_positions(args):
    try:
        with open(args.request, "r", encoding="utf-8") as request_file:
            requests = json.load(request_file)
        if not isinstance(requests, list) or len(requests) > 1000:
            raise ValueError("invalid position request list")
        translated = []
        for request in requests:
            if not isinstance(request, dict):
                raise ValueError("invalid position request")
            translated.append(translate_pair(
                args.epub,
                request.get("start"),
                request.get("end"),
            ))
        translated = normalize_annotation_ends(args.epub, translated)
        exit_json({
            "version": VERSION,
            "ok": True,
            "positions": translated,
        })
    except (OSError, zipfile.BadZipFile, PositionTranslationError, ValueError, json.JSONDecodeError) as error:
        exit_json({
            "version": VERSION,
            "ok": False,
            "message": str(error),
        }, code=1)


def cmd_translate_native_positions(args):
    """Reverse-translate a bounded annotation range batch into XPointers."""
    try:
        with open(args.request, "r", encoding="utf-8") as request_file:
            requests = json.load(request_file)
        if not isinstance(requests, list) or len(requests) > 1000:
            raise ValueError("invalid native position request list")
        translated = []
        for request in requests:
            if not isinstance(request, dict):
                raise ValueError("invalid native position request")
            start = request.get("start")
            end = request.get("end")
            if not isinstance(start, str) or not isinstance(end, str):
                raise ValueError("native annotation range is incomplete")
            translated.append({
                "start": translate_native_position(args.epub, start),
                "end": translate_native_position(args.epub, end),
            })
        exit_json({
            "version": VERSION,
            "ok": True,
            "positions": translated,
        })
    except (OSError, zipfile.BadZipFile, PositionTranslationError, ValueError,
            json.JSONDecodeError) as error:
        exit_json({
            "version": VERSION,
            "ok": False,
            "message": str(error),
        }, code=1)


def main():
    parser = argparse.ArgumentParser(prog="kindle-helper")
    sub = parser.add_subparsers(dest="command")

    # convert
    p_convert = sub.add_parser("convert")
    p_convert.add_argument("--input", required=True)
    p_convert.add_argument("--output", required=True)
    p_convert.add_argument("--cache-dir", default="")

    p_translates = sub.add_parser("translate-positions")
    p_translates.add_argument("--epub", required=True)
    p_translates.add_argument("--request", required=True)

    p_translate_natives = sub.add_parser("translate-native-positions")
    p_translate_natives.add_argument("--epub", required=True)
    p_translate_natives.add_argument("--request", required=True)

    # extract-key
    p_extract = sub.add_parser("extract-key")
    p_extract.add_argument("--input", required=True,
                           help="path to the KFX file")
    p_extract.add_argument("--cache-dir", default="",
                           help="cache directory for drm_keys.json")
    p_extract.add_argument("--plugin-dir", default="",
                           help="plugin directory containing lib/ helpers")

    args = parser.parse_args()

    if args.command is None:
        parser.print_help(sys.stderr)
        sys.exit(2)

    dispatch = {
        "convert": cmd_convert,
        "translate-positions": cmd_translate_positions,
        "translate-native-positions": cmd_translate_native_positions,
        "extract-key": cmd_extract_key,
    }

    dispatch[args.command](args)


if __name__ == "__main__":
    main()
