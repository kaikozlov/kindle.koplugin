#!/usr/bin/env python3
"""Kindle Helper — Python replacement for the Go kindle-helper binary.

Exposes the same CLI interface and JSON-over-stdout protocol so the Lua
plugin layer doesn't need any changes.  Subcommands: scan, convert, cover,
decrypt, position.

The KFX→EPUB conversion uses kfxlib (John Howell's Calibre KFX Input plugin)
directly — no Calibre installation required.

DRM handling: DRMION books are decrypted using cached page keys before being
passed to kfxlib. Device-specific key extraction is exposed through drm-init.
"""

import argparse
import base64
import hashlib
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

from epub_position import (
    PositionTranslationError, native_position_percent, translate_native_position,
    translate_pair, translate_xpointer)
from annotation_position import normalize_annotation_ends
from kfx_position_adapter import position_metadata_conversion
from krds_position import KrdsError, read_position_file, write_position_file

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
# scan — walk the Kindle document library
# ---------------------------------------------------------------------------

SUPPORTED_EXTENSIONS = {".kfx", ".azw", ".azw3", ".mobi", ".prc", ".pdf"}

# Directories to skip during scanning.  These contain Kindle system files
# (dictionaries, active content, firmware assets) that are not user books.
EXCLUDED_DIRS = {"dictionaries", "system"}

TRAILING_ID_RE = re.compile(r"_(?:[A-Z0-9]{10}|[A-F0-9]{32})$")


def _derive_title(filename):
    name = os.path.splitext(filename)[0]
    name = TRAILING_ID_RE.sub("", name)
    name = name.replace("_", " ").strip()
    return name or "Untitled"


def _sha1_hex(text):
    return hashlib.sha1(text.encode("utf-8")).hexdigest()


def _classify_kfx(path):
    """Classify a KFX file: return (open_mode, block_reason)."""
    try:
        with open(path, "rb") as f:
            header = f.read(8)
    except OSError:
        return ("blocked", "cannot_read")

    if header.startswith(DRMION_SIGNATURE):
        return ("convert", "")
    if header.startswith(CONT_SIGNATURE):
        return ("convert", "")
    return ("blocked", "unknown_format")


def _extract_sidecar_metadata(sidecar_dir):
    """Try to extract title/authors from the .sdr sidecar metadata."""
    # TODO: implement metadata.kfx parsing if needed.
    # For now return None — titles come from filenames.
    return None



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

        # Write output
        os.makedirs(os.path.dirname(os.path.abspath(output_path)), exist_ok=True)
        with open(output_path, "wb") as f:
            f.write(epub_data)

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
# cover — extract cover JPEG from .sdr/assets/metadata.kfx
# ---------------------------------------------------------------------------

def cmd_cover(args):
    if not args.sdr_dir:
        print("cover: --sdr-dir is required", file=sys.stderr)
        sys.exit(2)

    sdr_dir = args.sdr_dir
    output = getattr(args, "output", "") or ""

    # Look for metadata.kfx in the sidecar assets
    cover_data = None
    for root, dirs, files in os.walk(sdr_dir):
        for fname in files:
            if fname == "metadata.kfx":
                fpath = os.path.join(root, fname)
                try:
                    with open(fpath, "rb") as f:
                        data = f.read()
                except OSError:
                    continue

                # Quick scan for JPEG in the metadata.kfx container
                # JPEG starts with FF D8 FF and ends with FF D9
                jpeg_start = data.find(b"\xff\xd8\xff")
                if jpeg_start >= 0:
                    jpeg_end = data.find(b"\xff\xd9", jpeg_start)
                    if jpeg_end >= 0:
                        cover_data = data[jpeg_start:jpeg_end + 2]
                        break
        if cover_data:
            break

    if cover_data is None:
        exit_json({
            "version": VERSION,
            "ok": False,
            "message": "no cover image found in metadata.kfx",
        })

    if output:
        os.makedirs(os.path.dirname(os.path.abspath(output)), exist_ok=True)
        with open(output, "wb") as f:
            f.write(cover_data)
        exit_json({
            "version": VERSION,
            "ok": True,
            "size": len(cover_data),
        })
    else:
        sys.stdout.buffer.write(cover_data)


# ---------------------------------------------------------------------------
# decrypt — DRMION → KFX-zip (for testing / pre-decryption)
# ---------------------------------------------------------------------------

def cmd_decrypt(args):
    if not args.input:
        print("decrypt: --input is required", file=sys.stderr)
        sys.exit(2)
    if not args.output:
        print("decrypt: --output is required", file=sys.stderr)
        sys.exit(2)

    input_path = args.input
    output_path = args.output
    cache_dir = getattr(args, "cache_dir", "") or ""

    with open(input_path, "rb") as f:
        data = f.read()

    if not data.startswith(DRMION_SIGNATURE):
        print("decrypt: not a DRMION file", file=sys.stderr)
        sys.exit(1)

    page_key = _find_page_key(input_path, cache_dir)
    try:
        # PlainText-only DRMION envelopes are valid without a cached key.
        cont_data = _decrypt_drmion(data, page_key)
    except Exception as e:
        if page_key is None:
            print("decrypt: no cached page key found", file=sys.stderr)
        else:
            print(f"decrypt: {e}", file=sys.stderr)
        sys.exit(1)

    print(f"decrypted main container: {len(cont_data)} bytes", file=sys.stderr)

    # Collect sidecar blobs
    sidecar_root = os.path.splitext(input_path)[0] + ".sdr"
    entries = [("main.kfx", cont_data)]

    if os.path.isdir(sidecar_root):
        for dirpath, _, filenames in os.walk(sidecar_root):
            for fn in sorted(filenames):
                fpath = os.path.join(dirpath, fn)
                try:
                    blob = open(fpath, "rb").read()
                except OSError:
                    continue
                rel = os.path.relpath(fpath, sidecar_root)
                if blob.startswith(CONT_SIGNATURE):
                    entries.append((rel, blob))
                elif blob.startswith(DRMION_SIGNATURE):
                    try:
                        dec = _decrypt_drmion(blob, page_key)
                        entries.append((rel, dec))
                        print(f"decrypted sidecar {rel}: {len(dec)} bytes", file=sys.stderr)
                    except Exception as e:
                        print(f"skipping DRMION sidecar {rel}: {e}", file=sys.stderr)

    # Write KFX-zip
    os.makedirs(os.path.dirname(os.path.abspath(output_path)), exist_ok=True)
    with zipfile.ZipFile(output_path, "w") as zf:
        for name, blob in entries:
            zf.writestr(name, blob)

    print(f"wrote {output_path} with {len(entries)} entries", file=sys.stderr)


# ---------------------------------------------------------------------------
# position — update reading position in .yjr sidecar file
# ---------------------------------------------------------------------------

# ---------------------------------------------------------------------------
# drm-init — extract DRM keys from device
# ---------------------------------------------------------------------------

def cmd_drm_init(args):
    root = args.root
    cache_dir = args.cache_dir or ""
    plugin_dir = args.plugin_dir or ""

    if not plugin_dir:
        # Script is in dist/, which is the plugin_dir for DRM helpers
        plugin_dir = os.path.dirname(os.path.abspath(sys.argv[0]))

    try:
        from dedrm.drm_init import run as drm_init_run
        result = drm_init_run(root, plugin_dir, cache_dir)
        exit_json({
            "version": VERSION,
            "ok": True,
            "books_found": result["books_found"],
            "keys_found": result["keys_found"],
        })
    except Exception as e:
        exit_json({
            "version": VERSION,
            "ok": False,
            "code": "drm_init",
            "message": str(e),
        })


def cmd_read_native_sidecar(args):
    try:
        result = read_position_file(args.input)
        exit_json({
            "version": VERSION,
            "ok": True,
            **result,
        })
    except (OSError, KrdsError) as error:
        exit_json({
            "version": VERSION,
            "ok": False,
            "message": str(error),
        }, code=1)


def cmd_write_native_sidecar(args):
    try:
        result = write_position_file(
            args.input,
            args.long_position,
            args.pid,
            args.timestamp_ms,
        )
        exit_json({
            "version": VERSION,
            "ok": True,
            **result,
        })
    except (OSError, KrdsError) as error:
        exit_json({
            "version": VERSION,
            "ok": False,
            "message": str(error),
        }, code=1)

def cmd_read_close_state(args):
    """One-spawn read of everything an exact open/close sync needs.

    Interpreter startup dominates each helper invocation on the device, so
    the Kindle sidecar, its reverse translation, and the forward translation
    of KOReader's XPointer are returned together.
    """
    result = {"version": VERSION, "ok": True}
    try:
        native = read_position_file(args.input)
    except (OSError, KrdsError) as error:
        result["native_error"] = str(error)
    else:
        result["native"] = {
            "long": native["long"],
            "pid": native["pid"],
            "timestamp_ms": native.get("timestamp_ms"),
        }
        try:
            translated = translate_native_position(args.epub, native["long"])
            result["native_xpointer"] = translated["xpointer"]
            result["native_pid"] = translated["pid"]
            result["native_percent"] = translated["percent"]
        except (OSError, zipfile.BadZipFile, PositionTranslationError, ValueError) as error:
            result["native_translate_error"] = str(error)
    try:
        koreader = translate_xpointer(args.epub, args.xpointer)
        result["koreader"] = dict(koreader)
        result["koreader"]["percent"] = native_position_percent(args.epub, koreader["pid"])
    except (OSError, zipfile.BadZipFile, PositionTranslationError, ValueError) as error:
        result["ok"] = False
        result["koreader_error"] = str(error)
    exit_json(result, code=0 if result["ok"] else 1)


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


def cmd_translate_position(args):
    try:
        translated = translate_pair(args.epub, args.start, args.end)
        exit_json({
            "version": VERSION,
            "ok": True,
            **translated,
        })
    except (OSError, zipfile.BadZipFile, PositionTranslationError, ValueError) as error:
        exit_json({
            "version": VERSION,
            "ok": False,
            "message": str(error),
        }, code=1)


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


def cmd_translate_native_position(args):
    try:
        translated = translate_native_position(args.epub, args.long_position)
        exit_json({
            "version": VERSION,
            "ok": True,
            **translated,
        })
    except (OSError, zipfile.BadZipFile, PositionTranslationError, ValueError) as error:
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

    # cover
    p_cover = sub.add_parser("cover")
    p_cover.add_argument("--sdr-dir", required=True)
    p_cover.add_argument("--output", default="")

    # decrypt
    p_decrypt = sub.add_parser("decrypt")
    p_decrypt.add_argument("--input", required=True)
    p_decrypt.add_argument("--output", required=True)
    p_decrypt.add_argument("--cache-dir", default="")

    # Exact native reading position sidecars
    p_read_native = sub.add_parser("read-native-sidecar")
    p_read_native.add_argument("--input", required=True)

    p_write_native = sub.add_parser("write-native-sidecar")
    p_write_native.add_argument("--input", required=True)
    p_write_native.add_argument("--long", dest="long_position", required=True)
    p_write_native.add_argument("--pid", type=int, required=True)
    p_write_native.add_argument("--timestamp-ms", type=int)

    # One-spawn batched read for exact open/close sync
    p_close_state = sub.add_parser("read-close-state")
    p_close_state.add_argument("--input", required=True)
    p_close_state.add_argument("--epub", required=True)
    p_close_state.add_argument("--xpointer", required=True)

    # Diagnostic bulk DRM extraction
    p_drm = sub.add_parser("drm-init")
    p_drm.add_argument("--root", default="/mnt/us/documents",
                      help="root directory to scan for DRM books")
    p_drm.add_argument("--cache-dir", default="",
                      help="cache directory for drm_keys.json")
    p_drm.add_argument("--plugin-dir", default="",
                      help="plugin directory containing lib/ helpers")

    p_translate = sub.add_parser("translate-position")
    p_translate.add_argument("--epub", required=True)
    p_translate.add_argument("--start", required=True)
    p_translate.add_argument("--end", required=True)

    p_translates = sub.add_parser("translate-positions")
    p_translates.add_argument("--epub", required=True)
    p_translates.add_argument("--request", required=True)

    p_translate_native = sub.add_parser("translate-native-position")
    p_translate_native.add_argument("--epub", required=True)
    p_translate_native.add_argument("--long", dest="long_position", required=True)

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
        "cover": cmd_cover,
        "decrypt": cmd_decrypt,
        "drm-init": cmd_drm_init,
        "read-native-sidecar": cmd_read_native_sidecar,
        "read-close-state": cmd_read_close_state,
        "write-native-sidecar": cmd_write_native_sidecar,
        "translate-position": cmd_translate_position,
        "translate-positions": cmd_translate_positions,
        "translate-native-position": cmd_translate_native_position,
        "translate-native-positions": cmd_translate_native_positions,
        "extract-key": cmd_extract_key,
    }

    dispatch[args.command](args)


if __name__ == "__main__":
    main()
