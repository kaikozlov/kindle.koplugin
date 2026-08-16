"""Annotation-only normalization layered on kfxlib position translation."""

import base64
import posixpath
import zipfile
from xml.etree import ElementTree


def _local_name(tag):
    return tag.rpartition("}")[2]


def _spine_paths(epub):
    container = ElementTree.fromstring(epub.read("META-INF/container.xml"))
    rootfile = next(
        node for node in container.iter() if _local_name(node.tag) == "rootfile")
    opf_path = rootfile.get("full-path")
    opf = ElementTree.fromstring(epub.read(opf_path))
    manifest = {
        node.get("id"): node.get("href")
        for node in opf.iter()
        if _local_name(node.tag) == "item" and node.get("id") and node.get("href")
    }
    opf_dir = posixpath.dirname(opf_path)
    return [
        posixpath.normpath(posixpath.join(opf_dir, manifest[node.get("idref")]))
        for node in opf.iter()
        if _local_name(node.tag) == "itemref" and node.get("idref") in manifest
    ]


def _long_position(eid, offset):
    raw = bytes((1,)) + int(eid).to_bytes(4, "little") + int(offset).to_bytes(4, "little")
    return base64.b64encode(raw).decode("ascii")


def _terminal_element_lengths(epub_path):
    """Index generated KFX anchors in one EPUB pass.

    Batch annotation sync can contain hundreds of ranges. Opening and walking
    the complete spine once per range made close-time sync needlessly slow on
    Kindle hardware, so callers share this text-free length index.
    """
    lengths = {}
    try:
        with zipfile.ZipFile(epub_path) as epub:
            for path in _spine_paths(epub):
                document = ElementTree.fromstring(epub.read(path))
                for element in document.iter():
                    eid = element.get("data-kfx-eid")
                    base_pid = element.get("data-kfx-pid")
                    if eid is None or base_pid is None:
                        continue
                    try:
                        key = (int(eid), int(base_pid))
                    except ValueError:
                        continue
                    text_length = len("".join(element.itertext()))
                    if key in lengths and lengths[key] != text_length:
                        lengths[key] = None
                    else:
                        lengths[key] = text_length
    except (ElementTree.ParseError, KeyError, StopIteration, TypeError) as error:
        raise ValueError("invalid EPUB position metadata") from error
    return lengths


def _normalize_with_lengths(start, end, lengths):
    if not isinstance(start, dict) or not isinstance(end, dict):
        return end
    eid = end.get("eid")
    offset = end.get("eid_offset")
    pid = end.get("pid")
    if not all(isinstance(value, int) for value in (eid, offset, pid)) or offset <= 0:
        return end
    if isinstance(start.get("pid"), int) and pid <= start["pid"]:
        return end

    base_pid = pid - offset
    if lengths.get((eid, base_pid)) != offset:
        return end
    normalized = dict(end)
    normalized["eid_offset"] = offset - 1
    normalized["pid"] = pid - 1
    normalized["long"] = _long_position(eid, offset - 1)
    return normalized


def normalize_annotation_ends(epub_path, positions):
    """Normalize all terminal exclusive ends with one EPUB spine scan."""
    if not positions:
        return []
    lengths = _terminal_element_lengths(epub_path)
    normalized = []
    for position in positions:
        item = dict(position)
        item["end"] = _normalize_with_lengths(
            item.get("start"), item.get("end"), lengths)
        normalized.append(item)
    return normalized


def normalize_annotation_end(epub_path, start, end):
    """Convert a terminal exclusive KOReader end into Kindle's last character.

    The native factory resolves the boundary after a KFX element with short PID
    zero, and KPP renders such a highlight as ``[Image]``. Other end positions
    retain their exact kfxlib translation.
    """
    return _normalize_with_lengths(
        start, end, _terminal_element_lengths(epub_path))
