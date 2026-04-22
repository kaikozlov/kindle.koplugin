"""DRM key extraction for Kindle KFX books.

Ports the Go drm-init command to Python. The workflow:
1. Scan for voucher files under the documents root
2. Read device serial from /proc/usid
3. Run the device's cvm JVM with LD_PRELOAD hook to capture AES keys
4. Parse captured keys from the log
5. Decrypt vouchers and extract 16-byte page keys
6. Write drm_keys.json cache
"""

import json
import os
import re
import subprocess
import sys
import struct
import time

from io import BytesIO

# Shared metadata key prefix — keys starting with this are the shared
# device metadata key, not per-book voucher keys.
_SHARED_METADATA_KEY_PREFIX = "6533356635"

# Pattern for captured keys from crypto_hook.so
_AES_KEY_RE = re.compile(r"^EVP_256_KEY:([0-9a-f]+)\s+IV:([0-9a-f]+)")


def run(documents_root, plugin_dir, cache_dir):
    """Execute the drm-init workflow. Returns dict with books_found, keys_found."""
    # Step 1: Find voucher files
    vouchers = _find_vouchers(documents_root)
    if not vouchers:
        return {"books_found": 0, "keys_found": 0}

    # Step 2: Read device serial
    serial = _read_device_serial()

    # Step 3: Run the Java extractor with LD_PRELOAD hook
    _extract_keys_with_hook(serial, vouchers, plugin_dir)

    # Step 4: Parse captured AES keys from the log
    keys = _parse_captured_keys("/mnt/us/crypto_keys.log")
    if not keys:
        raise RuntimeError("no AES keys captured from device")

    # Step 5: Decrypt vouchers and extract page keys
    cache = {
        "version": 1,
        "device_serial": serial,
        "generated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "books": {},
    }

    keys_found = 0
    for voucher_path in vouchers:
        voucher_key = _find_voucher_key(voucher_path, keys)
        if voucher_key is None:
            print(f"drm-init: skipping {voucher_path}: no matching key", file=sys.stderr, flush=True)
            continue

        try:
            page_key = _extract_page_key(voucher_path, voucher_key)
        except Exception as e:
            print(f"drm-init: page key extraction failed for {voucher_path}: {e}", file=sys.stderr, flush=True)
            continue

        book_id = _derive_book_id(voucher_path)

        # Prefer non-tmp vouchers over tmp_ ones
        if book_id in cache["books"]:
            new_is_tmp = "tmp_" in voucher_path
            existing_is_tmp = "tmp_" in cache["books"][book_id].get("voucher_path", "")
            if not existing_is_tmp and new_is_tmp:
                print(f"drm-init: skipping tmp voucher {voucher_path}", file=sys.stderr, flush=True)
                continue

        cache["books"][book_id] = {
            "voucher_path": voucher_path,
            "voucher_key_256": voucher_key.hex(),
            "page_key_128": page_key.hex(),
        }
        keys_found += 1

    # Step 6: Write the cache file
    cache_path = os.path.join(cache_dir, "drm_keys.json")
    with open(cache_path, "w") as f:
        json.dump(cache, f, indent=2)

    # Step 7: Clean up the key log
    try:
        os.remove("/mnt/us/crypto_keys.log")
    except OSError:
        pass

    return {"books_found": len(vouchers), "keys_found": keys_found}


def _find_vouchers(root):
    """Walk the documents root looking for voucher files under assets/ dirs."""
    vouchers = []
    for dirpath, _, filenames in os.walk(root):
        for fname in filenames:
            if fname == "voucher" and "assets" in dirpath:
                vouchers.append(os.path.join(dirpath, fname))
    vouchers.sort()
    return vouchers


def _read_device_serial():
    """Read the Kindle serial from /proc/usid."""
    try:
        with open("/proc/usid", "r") as f:
            return f.read().strip("\x00\n")
    except FileNotFoundError:
        raise RuntimeError("/proc/usid not found — not running on a Kindle device?")


def _extract_keys_with_hook(serial, vouchers, plugin_dir):
    """Run the device's cvm JVM with LD_PRELOAD hook to capture AES keys."""
    hook_path = os.path.join(plugin_dir, "lib", "crypto_hook.so")
    jar_path = os.path.join(plugin_dir, "lib", "KFXVoucherExtractor.jar")

    if not os.path.isfile(hook_path):
        raise FileNotFoundError(f"crypto_hook.so not found: {hook_path}")
    if not os.path.isfile(jar_path):
        raise FileNotFoundError(f"KFXVoucherExtractor.jar not found: {jar_path}")

    # Clear the key log
    try:
        os.remove("/mnt/us/crypto_keys.log")
    except OSError:
        pass

    args = [serial] + vouchers

    cmd = [
        "/usr/java/bin/cvm",
        "-Djava.library.path=/usr/lib:/usr/java/lib",
        "-cp", jar_path + ":/opt/amazon/ebook/lib/YJReader-impl.jar",
        "KFXVoucherExtractor",
    ] + args

    env = os.environ.copy()
    env["LD_PRELOAD"] = hook_path + ":/usr/java/lib/arm/libdlopen_global.so"
    env["LD_LIBRARY_PATH"] = "/usr/lib:/usr/java/lib"

    result = subprocess.run(cmd, env=env, capture_output=True, text=True, timeout=60)

    if result.returncode != 0:
        raise RuntimeError(f"cvm failed (exit {result.returncode}): {result.stderr}\n{result.stdout}")

    if "All vouchers attached" not in result.stdout:
        raise RuntimeError(f"voucher extraction may have failed: {result.stdout}")


def _parse_captured_keys(log_path):
    """Read the crypto_keys.log and extract AES-256 keys."""
    try:
        with open(log_path, "r") as f:
            data = f.read()
    except FileNotFoundError:
        return []

    keys = []
    for line in data.splitlines():
        m = _AES_KEY_RE.match(line)
        if not m:
            continue

        key_hex = m.group(1)
        iv_hex = m.group(2)

        # Skip the shared metadata key
        if key_hex.startswith(_SHARED_METADATA_KEY_PREFIX):
            continue

        key = bytes.fromhex(key_hex)
        iv = bytes.fromhex(iv_hex) if iv_hex != "none" else None

        keys.append({"key": key, "iv": iv})

    return keys


def _find_voucher_key(voucher_path, captured_keys):
    """Find the AES-256 key that decrypts the given voucher."""
    if not captured_keys:
        return None

    if len(captured_keys) == 1:
        return captured_keys[0]["key"]

    # Try each key to find which one decrypts the voucher
    with open(voucher_path, "rb") as f:
        voucher_data = f.read()

    for captured in captured_keys:
        if len(captured["key"]) != 32:
            continue
        try:
            _extract_page_key_from_data(voucher_data, captured["key"])
            return captured["key"]
        except Exception:
            continue

    return None


def _extract_page_key(voucher_path, aes256_key):
    """Read a voucher file, decrypt it, and extract the 16-byte page key."""
    with open(voucher_path, "rb") as f:
        voucher_data = f.read()
    return _extract_page_key_from_data(voucher_data, aes256_key)


def _extract_page_key_from_data(voucher_data, aes256_key):
    """Decrypt voucher data and extract the 16-byte page key.

    The voucher is an ION structure (VoucherEnvelope → Voucher → cipher_text + cipher_iv).
    We decrypt the cipher_text with AES-256-CBC using the captured key, then find
    the page key by looking for the "RAW" marker in the plaintext.
    """
    from Crypto.Cipher import AES

    # Parse the voucher ION to get ciphertext and cipher_iv
    ciphertext, cipher_iv = _parse_voucher(voucher_data)

    if len(ciphertext) % AES.block_size != 0:
        raise ValueError("ciphertext not aligned to block size")

    # Decrypt with AES-256-CBC
    cipher = AES.new(aes256_key, AES.MODE_CBC, cipher_iv[:16])
    plaintext = cipher.decrypt(ciphertext)

    # Remove PKCS7 padding
    if not plaintext:
        raise ValueError("empty plaintext")
    pad = plaintext[-1]
    if pad == 0 or pad > AES.block_size:
        raise ValueError("invalid PKCS7 padding")
    plaintext = plaintext[:-pad]

    # Find page key: look for "RAW" marker
    # After "RAW": 9d ae 90 <16-byte key> (ION annotation header + blob)
    raw_idx = plaintext.find(b"RAW")
    if raw_idx < 0:
        raise ValueError("no RAW marker found in decrypted voucher")

    # The Go code: key_offset = raw_pos + 6
    # "RAW" (3 bytes) + 3 bytes of ION type descriptor
    key_offset = raw_idx + 6
    if key_offset + 16 > len(plaintext):
        raise ValueError("not enough data after RAW marker for page key")

    return plaintext[key_offset:key_offset + 16]


def _parse_voucher(voucher_data):
    """Parse a voucher file to extract ciphertext and cipher_iv.

    The voucher is a two-layer ION structure:
    - Outer: VoucherEnvelope with a "voucher" lob field
    - Inner: Voucher with "cipher_text" and "cipher_iv" fields

    We use a simple binary scan approach instead of a full ION parser,
    since we only need two specific fields.
    """
    from dedrm.ion import BinaryIonParser, addprottable

    # Parse outer envelope
    envelope = BinaryIonParser(BytesIO(voucher_data))
    addprottable(envelope)
    envelope.reset()

    if not envelope.hasnext():
        raise ValueError("empty voucher envelope")
    envelope.next()  # skip to struct

    inner_voucher_data = None
    envelope.stepin()
    while envelope.hasnext():
        envelope.next()
        if envelope.getfieldname() == "voucher":
            inner_voucher_data = envelope.lobvalue()
    envelope.stepout()

    if not inner_voucher_data:
        raise ValueError("voucher envelope has no inner voucher")

    # Parse inner voucher for cipher_text and cipher_iv
    inner = BinaryIonParser(BytesIO(inner_voucher_data))
    addprottable(inner)
    inner.reset()

    if not inner.hasnext():
        raise ValueError("empty inner voucher")
    inner.next()  # skip to struct

    ciphertext = None
    cipher_iv = None

    inner.stepin()
    while inner.hasnext():
        inner.next()
        fname = inner.getfieldname()
        if fname == "cipher_text":
            ciphertext = inner.lobvalue()
        elif fname == "cipher_iv":
            cipher_iv = inner.lobvalue()
    inner.stepout()

    if ciphertext is None:
        raise ValueError("voucher missing cipher_text")
    if cipher_iv is None:
        raise ValueError("voucher missing cipher_iv")

    return ciphertext, cipher_iv


def _derive_book_id(voucher_path):
    """Extract a book identifier from a voucher path.

    E.g., /mnt/us/documents/Book_B003VIWNQW.sdr/assets/voucher → B003VIWNQW
    """
    # Walk up: voucher → assets → <name>.sdr
    sdr_dir = os.path.dirname(os.path.dirname(voucher_path))
    base = os.path.basename(sdr_dir)
    name = os.path.splitext(base)[0]

    # Try to extract ASIN-like ID from trailing pattern
    parts = name.rsplit("_", 1)
    if len(parts) >= 2 and len(parts[-1]) == 10 and parts[-1].isalnum():
        return parts[-1]

    return name
