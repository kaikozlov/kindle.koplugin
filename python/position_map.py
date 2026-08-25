"""Serialize the converted EPUB's exact-position layout at conversion time.

The runtime translators used to re-parse the generated EPUB on every sync to
recover the data-kfx anchors that conversion itself injected. This module
flattens that layout into one JSON sidecar so sync-time translation is a pure
table lookup (done in Lua on the device, with no interpreter spawn).

Layout produced per spine document (fragment), in spine order so fragment
indexes match KOReader's DocFragment[i] steps:

  elements: canonical CRE-style path -> { a: anchor index (0 = none),
             s: text chars inside the anchor before this element's text,
             l: [len(element.text), len(child.tail), ...] }
  anchors:  [{ p: path, eid, pid, t: subtree text total,
              nodes: [{ p: owning element path, n: text() index within it,
                        c: offset inside the anchor, v: node length }] }]

Forward translation resolves an XPointer's element steps against ``elements``
and adds the character offset inside the anchor; reverse translation searches
the anchor's ``nodes`` for the covering text node and rebuilds the XPointer.
"""

import json
import posixpath
import zipfile
from xml.etree import ElementTree


class PositionMapError(ValueError):
    pass


def _local_name(tag):
    return tag.rpartition("}")[2]


def _read_spine(epub):
    container = ElementTree.fromstring(epub.read("META-INF/container.xml"))
    rootfiles = [node for node in container.iter() if _local_name(node.tag) == "rootfile"]
    if not rootfiles:
        raise PositionMapError("EPUB rootfile is missing")
    opf_path = rootfiles[0].get("full-path")
    opf = ElementTree.fromstring(epub.read(opf_path))
    manifest = {
        node.get("id"): node.get("href")
        for node in opf.iter()
        if _local_name(node.tag) == "item" and node.get("id") and node.get("href")
    }
    spine = []
    opf_dir = posixpath.dirname(opf_path)
    for node in opf.iter():
        if _local_name(node.tag) != "itemref":
            continue
        href = manifest.get(node.get("idref"))
        if href:
            spine.append(posixpath.normpath(posixpath.join(opf_dir, href)))
    if not spine:
        raise PositionMapError("EPUB spine is empty")
    return spine


def _element_step(element, parent):
    name = _local_name(element.tag)
    index = 1
    for sibling in list(parent):
        if sibling is element:
            break
        if _local_name(sibling.tag) == name:
            index += 1
    return name if index == 1 else "%s[%d]" % (name, index)


def _walk(element, path, anchor_idx, base, elements, anchors):
    """Walk one subtree; returns its total text length.

    ``base`` is the offset of this element's own text within its anchor
    (0 for the anchor element itself, ignored when ``anchor_idx`` is 0).
    """
    eid_attr = element.get("data-kfx-eid")
    pid_attr = element.get("data-kfx-pid")
    is_anchor = eid_attr is not None and pid_attr is not None
    if is_anchor:
        anchor_idx = len(anchors) + 1
        anchors.append({
            "p": path,
            "eid": int(eid_attr),
            "pid": int(pid_attr),
            "nodes": [],
        })
        base = 0

    anchor = anchors[anchor_idx - 1] if anchor_idx else None

    # XPointer text() indexes count only the text nodes that exist; a missing
    # tail is not a node, but an empty-string text or tail is.
    node_lengths = []
    if element.text is not None:
        node_lengths.append(len(element.text))
    for child in list(element):
        if child.tail is not None:
            node_lengths.append(len(child.tail))

    entry = {"a": anchor_idx, "s": base, "l": node_lengths}
    elements[path] = entry
    total = len(element.text or "")
    if anchor is not None and element.text is not None:
        anchor["nodes"].append(
            {"p": path, "n": 1, "c": base, "v": len(element.text)})

    cursor = base + total
    child_position = 0
    for child in list(element):
        child_position += 1
        child_path = (path + "/" if path else "") + _element_step(child, element)
        subtree = _walk(child, child_path, anchor_idx, cursor, elements, anchors)
        total += subtree
        cursor += subtree
        if child.tail is not None:
            if anchor is not None:
                anchor["nodes"].append({
                    "p": path,
                    "n": _tail_index_for(element, child_position),
                    "c": cursor,
                    "v": len(child.tail),
                })
            cursor += len(child.tail)
            total += len(child.tail)

    if is_anchor:
        anchors[anchor_idx - 1]["t"] = cursor
    return total


def _tail_index_for(parent, child_position):
    # _text_nodes order: element.text (when present), then each preceding
    # child's existing tail, then this tail.
    index = 1 if parent.text is not None else 0
    children = list(parent)
    for position in range(1, child_position):
        if children[position - 1].tail is not None:
            index += 1
    return index + 1


def build_position_map(epub_path):
    """Return the flattened position map for one converted EPUB."""
    fragments = []
    any_anchor = False

    with zipfile.ZipFile(epub_path) as epub:
        for document_path in _read_spine(epub):
            document = ElementTree.fromstring(epub.read(document_path))
            bodies = [node for node in document.iter() if _local_name(node.tag) == "body"]
            fragment = {"path": document_path, "elements": {}, "anchors": []}
            if bodies:
                body = bodies[0]
                for child in list(body):
                    child_path = _element_step(child, body)
                    _walk(child, child_path, 0, 0,
                          fragment["elements"], fragment["anchors"])
                if fragment["anchors"]:
                    any_anchor = True
            fragments.append(fragment)

    if not any_anchor:
        raise PositionMapError("EPUB has no KFX position anchors")

    max_pid = max(
        anchor["pid"] + max(1, anchor["t"])
        for fragment in fragments
        for anchor in fragment["anchors"]
    )
    return {
        "version": 1,
        "max_pid": max_pid,
        "fragments": fragments,
    }


def write_position_map(epub_path, output_path):
    payload = build_position_map(epub_path)
    with open(output_path, "w", encoding="utf-8") as target:
        json.dump(payload, target, separators=(",", ":"))
    return output_path
