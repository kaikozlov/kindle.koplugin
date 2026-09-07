"""Adapter for the optional native libYJSDK KUAL/scriptlet key extractor.

The plugin does not bundle ABI-specific native binaries. If Satsuoni's tested
kfxdedrm tooling is installed — either the original KUAL extension or the
newer kfx-dedrm scriptlet packaging — this module can use it as a fallback
when the primary cvm/Java extraction route fails, import its key-ID mappings,
and remove the temporary plaintext keyfile afterward.
"""

import os
import subprocess


DEFAULT_NATIVE_DIRS = (
    # Original KUAL extension layout.
    "/mnt/us/extensions/kfxdedrm/bin",
    # Jadehawk kfx-dedrm scriptlet layout (same binary names, newer install).
    "/mnt/us/extensions/kfxdedrm-scriptlet/bin",
)
DEFAULT_KEY_FILE = "/mnt/us/dedrm/keyfile.txt"
CANDIDATE_NAMES = (
    "kfxdedrmhf_c11",
    "kfxdedrmhf_old",
    "kfxdedrm_old",
    "kfxdedrm_c11",
)


class NativeExtractorUnavailable(RuntimeError):
    pass


def _candidate_paths(plugin_dir=None, native_dir=None):
    directories = []
    if plugin_dir:
        directories.append(os.path.join(plugin_dir, "lib"))
    if native_dir:
        directories.append(native_dir)
    else:
        directories.extend(DEFAULT_NATIVE_DIRS)

    for directory in directories:
        for name in CANDIDATE_NAMES:
            yield os.path.join(directory, name)


def _should_probe_native_binary(path):
    """Return True when a candidate path should be runtime-probed.

    Kindle firmware mounts /mnt/us over FUSE. Permission and file-type metadata
    such as os.access(X_OK) or os.path.isfile() can be wrong for shipped
    kfxdedrm binaries even though execve succeeds. Only skip obvious
    directories; treat the extractor's own ``test`` command as authoritative.
    """
    if os.path.isdir(path):
        return False
    return os.path.isfile(path) or os.path.lexists(path)


def _probe_native_binary(path):
    """Run the native extractor compatibility self-test."""
    return subprocess.run(
        [path, "test"],
        capture_output=True,
        text=True,
        timeout=15,
    )


def find_executable(plugin_dir=None, native_dir=None):
    """Return the first ABI-compatible native extractor executable."""
    failures = []
    for path in _candidate_paths(plugin_dir, native_dir):
        if not _should_probe_native_binary(path):
            continue
        try:
            result = _probe_native_binary(path)
        except (OSError, subprocess.TimeoutExpired) as error:
            failures.append(f"{path}: {error}")
            continue
        if result.returncode == 0:
            return path
        failures.append(f"{path}: exit {result.returncode}")

    detail = "; ".join(failures) if failures else "no native extractor binaries found"
    raise NativeExtractorUnavailable(detail)


def parse_key_file(key_file):
    """Parse DeDRM key-ID$secret_key mappings from a native keyfile."""
    page_keys = {}
    with open(key_file, "r", encoding="utf-8", errors="replace") as source:
        for raw_line in source:
            line = raw_line.strip()
            if "$secret_key:" not in line:
                continue
            key_id, key_hex = line.rsplit("$secret_key:", 1)
            key_id = key_id.strip()
            key_hex = key_hex.strip()
            try:
                page_key = bytes.fromhex(key_hex)
            except ValueError:
                continue
            if key_id and len(page_key) == 16:
                page_keys[key_id] = page_key
    return page_keys


def extract_page_keys(plugin_dir=None, native_dir=None, key_file=None):
    """Run the optional native extractor and return key-ID to page-key mappings."""
    executable = find_executable(plugin_dir, native_dir)
    key_file = key_file or DEFAULT_KEY_FILE
    key_parent = os.path.dirname(key_file)
    if key_parent:
        os.makedirs(key_parent, exist_ok=True)

    try:
        try:
            os.remove(key_file)
        except OSError:
            pass

        result = subprocess.run(
            [executable, "keyfile"],
            capture_output=True,
            text=True,
            timeout=300,
        )
        if result.returncode != 0:
            # The native tool's verbose output may include the device serial,
            # account secret, and captured keys. Never propagate it into logs.
            raise RuntimeError(f"native extractor failed (exit {result.returncode})")
        if not os.path.isfile(key_file):
            raise RuntimeError("native extractor did not produce a keyfile")

        page_keys = parse_key_file(key_file)
        if not page_keys:
            raise RuntimeError("native extractor produced no usable page keys")
        return page_keys
    finally:
        try:
            os.remove(key_file)
        except OSError:
            pass
