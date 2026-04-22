#!/usr/bin/env python3
"""Kindle Helper — Python replacement for the Go kindle-helper binary.

Exposes the same CLI interface and JSON-over-stdout protocol so the Lua
plugin layer doesn't need any changes.  Subcommands: scan, convert, cover,
decrypt, position.

The KFX→EPUB conversion uses kfxlib (John Howell's Calibre KFX Input plugin)
directly — no Calibre installation required.

DRM handling: DRMION books are decrypted using cached page keys before being
passed to kfxlib.  The drm-init command (device-specific key extraction) is
not yet implemented in Python.
"""

import argparse
import base64
import hashlib
import json
import os
import re
import sys
import zipfile
from pathlib import Path

VERSION = 1

# ---------------------------------------------------------------------------
# kfxlib setup — ensure bundled plugin modules (pypdf, typing_extensions) are
# importable even when calibre is not installed.
#
# In Nuitka standalone mode, __file__ points to the binary's location (dist/).
# We ship calibre-plugin-modules/ inside dist/ so pypdf can be found via
# sys.path.  When running from source, it lives next to this script.
# ---------------------------------------------------------------------------
_THIS_DIR = os.path.dirname(os.path.abspath(__file__))
_PLUGIN_MODULES = os.path.join(_THIS_DIR, "kfxlib", "calibre-plugin-modules")
if not os.path.isdir(_PLUGIN_MODULES):
    # Nuitka standalone: data files are in the same directory as the binary
    _PLUGIN_MODULES = os.path.join(_THIS_DIR, "calibre-plugin-modules")
if os.path.isdir(_PLUGIN_MODULES) and _PLUGIN_MODULES not in sys.path:
    sys.path.insert(0, _PLUGIN_MODULES)

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

DRMION_SIGNATURE = b"\xeaDRMION\xee"
CONT_SIGNATURE = b"CONT"


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

    # Try matching by book ID extracted from filename
    # Keys are indexed by ASIN/book ID (e.g. "B009NG3090")
    basename = os.path.basename(kfx_path)
    for book_id, entry in books.items():
        if book_id in basename:
            key_str = entry.get("page_key_128", "") if isinstance(entry, dict) else ""
            if key_str:
                try:
                    return bytes.fromhex(key_str)
                except ValueError:
                    pass

    # Fallback: try absolute path match
    abs_path = os.path.abspath(kfx_path)
    entry = books.get(abs_path) or books.get(kfx_path)
    if not entry:
        return None

    key_str = entry.get("page_key_128", "") if isinstance(entry, dict) else ""
    if not key_str:
        return None
    try:
        return bytes.fromhex(key_str)
    except ValueError:
        pass
    try:
        return base64.b64decode(key_str)
    except Exception:
        return None


def _decrypt_drmion(data, page_key):
    """Decrypt a DRMION blob using DeDRM's DrmIon with proper ION parsing.

    Uses the DeDRM ion.py library which correctly parses the ION structure
    of DRMION envelopes (EncryptedPage, PlainText, LZMA compression, etc).
    Returns the decrypted CONT data.
    """
    from io import BytesIO as _BytesIO
    from dedrm.ion import DrmIon as _DrmIon

    if not data.startswith(DRMION_SIGNATURE):
        raise ValueError("Not a DRMION file")

    class _Voucher:
        def __init__(self, key):
            self.secretkey = key

    out = _BytesIO()
    drm = _DrmIon(_BytesIO(data[8:-8]), lambda name: _Voucher(page_key))
    drm.parse(out)
    result = out.getvalue()

    if not result.startswith(CONT_SIGNATURE):
        raise ValueError("Decrypted data is not a valid CONT container")

    return result


# ---------------------------------------------------------------------------
# scan — walk the Kindle document library
# ---------------------------------------------------------------------------

SUPPORTED_EXTENSIONS = {".kfx", ".azw", ".azw3", ".mobi", ".prc", ".pdf"}
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


def cmd_scan(args):
    if not args.root:
        print("scan: --root is required", file=sys.stderr)
        sys.exit(2)

    books = []
    root = args.root

    for dirpath, dirnames, filenames in os.walk(root):
        # Skip .sdr directories
        dirnames[:] = [d for d in dirnames if not d.lower().endswith(".sdr")]

        for fname in sorted(filenames):
            ext = os.path.splitext(fname)[1].lower()
            if ext not in SUPPORTED_EXTENSIONS:
                continue

            fpath = os.path.join(dirpath, fname)
            try:
                stat = os.stat(fpath)
            except OSError:
                continue

            rel_path = os.path.relpath(fpath, root)
            # Normalise to forward slashes (matches Go behaviour)
            rel_path_norm = rel_path.replace(os.sep, "/")

            book = {
                "id": "sha1:" + _sha1_hex(rel_path_norm),
                "source_path": fpath,
                "format": ext.lstrip("."),
                "logical_ext": ext.lstrip("."),
                "title": _derive_title(fname),
                "authors": [],
                "display_name": _derive_title(fname),
                "open_mode": "direct",
                "source_mtime": int(stat.st_mtime),
                "source_size": stat.st_size,
            }

            # Check for sidecar directory
            sidecar = os.path.splitext(fpath)[0] + ".sdr"
            if os.path.isdir(sidecar):
                book["sidecar_path"] = sidecar

            if ext == ".kfx":
                book["logical_ext"] = "epub"
                mode, reason = _classify_kfx(fpath)
                book["open_mode"] = mode
                if reason:
                    book["block_reason"] = reason

                meta = _extract_sidecar_metadata(sidecar)
                if meta:
                    if meta.get("title"):
                        book["title"] = meta["title"]
                        book["display_name"] = meta["title"]
                    if meta.get("authors"):
                        book["authors"] = meta["authors"]

            books.append(book)

    books.sort(key=lambda b: (b["display_name"].lower(), b["source_path"]))

    exit_json({
        "version": VERSION,
        "root": root,
        "books": books,
    })


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
            page_key = _find_page_key(input_path, cache_dir)
            if page_key is None:
                exit_json({
                    "version": VERSION,
                    "ok": False,
                    "code": "drm",
                    "message": "DRM-protected book: no cached page key found",
                })

            with open(input_path, "rb") as f:
                data = f.read()

            try:
                cont_data = _decrypt_drmion(data, page_key)
            except Exception as e:
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
        error_type = type(e).__name__
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
    if page_key is None:
        print("decrypt: no cached page key found", file=sys.stderr)
        sys.exit(1)

    try:
        cont_data = _decrypt_drmion(data, page_key)
    except Exception as e:
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

def cmd_position(args):
    if not args.yjr:
        print("position: --yjr is required", file=sys.stderr)
        sys.exit(2)
    if args.old_percent is None or args.new_percent is None:
        print("position: --old-percent and --new-percent are required", file=sys.stderr)
        sys.exit(2)

    yjr_path = args.yjr
    old_percent = args.old_percent
    new_percent = args.new_percent

    with open(yjr_path, "rb") as f:
        data = bytearray(f.read())

    # Find the LAST erl value (reading position, not bookmarks)
    erl_key = b"\xff\xfe\x00\x00\x03erl"
    erl_idx = -1
    pos = 0
    while True:
        idx = data.find(erl_key, pos)
        if idx < 0:
            break
        erl_idx = idx
        pos = idx + 1

    if erl_idx < 0:
        exit_json({
            "version": VERSION,
            "ok": False,
            "message": "erl key not found in .yjr file",
        })

    # Parse the erl value header: 03 XX XX XX <string_bytes>
    val_start = erl_idx + len(erl_key)
    if val_start + 4 >= len(data) or data[val_start] != 0x03:
        exit_json({
            "version": VERSION,
            "ok": False,
            "message": "erl value has unexpected format",
        })

    old_len = (data[val_start + 1] << 16) | (data[val_start + 2] << 8) | data[val_start + 3]
    if val_start + 4 + old_len > len(data):
        exit_json({
            "version": VERSION,
            "ok": False,
            "message": "erl value truncated",
        })

    old_erl = bytes(data[val_start + 4:val_start + 4 + old_len]).decode("utf-8", errors="replace")
    print(f"existing erl: {old_erl}", file=sys.stderr)

    # Parse existing erl: base64part:position
    parts = old_erl.split(":", 1)
    if len(parts) != 2:
        exit_json({
            "version": VERSION,
            "ok": False,
            "message": "erl value has no colon separator",
        })

    decoded = base64.b64decode(parts[0])
    if len(decoded) != 9 or decoded[0] != 0x01:
        exit_json({
            "version": VERSION,
            "ok": False,
            "message": "erl base64 decode failed",
        })

    try:
        old_position = int(parts[1])
    except ValueError:
        exit_json({
            "version": VERSION,
            "ok": False,
            "message": "erl position parse error",
        })

    if old_percent == 0:
        exit_json({
            "version": VERSION,
            "ok": True,
            "erl": old_erl,
            "message": "old percent is 0, cannot scale",
        })

    # Scale: new_pos = old_pos * (new_pct / old_pct)
    new_position = int(old_position * (new_percent / old_percent))
    if new_position < 0:
        new_position = 0

    new_erl = parts[0] + ":" + str(new_position)
    print(f"new erl: {new_erl} (scaled {old_percent:.2f}% -> {new_percent:.2f}%)", file=sys.stderr)

    # Build replacement bytes
    new_erl_bytes = new_erl.encode("utf-8")
    new_len = len(new_erl_bytes)
    erl_value_header = bytes([0x03, (new_len >> 16) & 0xFF, (new_len >> 8) & 0xFF, new_len & 0xFF])
    old_end = val_start + 4 + old_len

    # Replace in data
    result = data[:val_start] + bytearray(erl_value_header) + bytearray(new_erl_bytes) + data[old_end:]

    # Ensure sync_lpr is set to 1
    sync_key = b"\xff\xfe\x00\x00\x08sync_lpr"
    sync_idx = result.find(sync_key)
    if sync_idx >= 0:
        val_off = sync_idx + len(sync_key)
        if val_off + 1 < len(result):
            result[val_off] = 0x00
            result[val_off + 1] = 0x01

    with open(yjr_path, "wb") as f:
        f.write(result)

    exit_json({
        "version": VERSION,
        "ok": True,
        "erl": new_erl,
    })


# ---------------------------------------------------------------------------
# CLI argument parsing
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(prog="kindle-helper")
    sub = parser.add_subparsers(dest="command")

    # scan
    p_scan = sub.add_parser("scan")
    p_scan.add_argument("--root", required=True)

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

    # position
    p_pos = sub.add_parser("position")
    p_pos.add_argument("--yjr", required=True)
    p_pos.add_argument("--old-percent", type=float, required=True)
    p_pos.add_argument("--new-percent", type=float, required=True)

    args = parser.parse_args()

    if args.command is None:
        parser.print_help(sys.stderr)
        sys.exit(2)

    dispatch = {
        "scan": cmd_scan,
        "convert": cmd_convert,
        "cover": cmd_cover,
        "decrypt": cmd_decrypt,
        "position": cmd_position,
    }

    dispatch[args.command](args)


if __name__ == "__main__":
    main()
