import base64
import json
import posixpath
import re
import zipfile
from xml.etree import ElementTree


__license__ = "GPL v3"


_STEP_RE = re.compile(r"^(?P<name>[^\[]+?)(?:\[(?P<index>[1-9][0-9]*)\])?$")
_XPOINTER_RE = re.compile(r"^(?P<path>/.*)\.(?P<offset>[0-9]+)$")


class PositionTranslationError(ValueError):
    pass


def _local_name(tag):
    return tag.rpartition("}")[2]


def _children_named(element, name):
    return [child for child in list(element) if _local_name(child.tag) == name]


def _parse_step(step):
    match = _STEP_RE.match(step)
    if not match:
        raise PositionTranslationError("unsupported XPointer step")
    return match.group("name"), int(match.group("index") or "1")


def _read_spine(epub):
    container = ElementTree.fromstring(epub.read("META-INF/container.xml"))
    rootfiles = [node for node in container.iter() if _local_name(node.tag) == "rootfile"]
    if not rootfiles:
        raise PositionTranslationError("EPUB rootfile is missing")
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
        raise PositionTranslationError("EPUB spine is empty")
    return spine


def _text_nodes(element):
    nodes = []
    if element.text is not None:
        nodes.append(((element, "text"), element.text))
    for child in list(element):
        if child.tail is not None:
            nodes.append(((child, "tail"), child.tail))
    return nodes


def _descendant_text_length_before(ancestor, target_node):
    total = 0

    def visit(element):
        nonlocal total
        if element.text is not None:
            if target_node == (element, "text"):
                return True
            total += len(element.text)
        for child in list(element):
            if visit(child):
                return True
            if child.tail is not None:
                if target_node == (child, "tail"):
                    return True
                total += len(child.tail)
        return False

    if not visit(ancestor):
        raise PositionTranslationError("XPointer text node is outside KFX element")
    return total


def _decode_long_position(eid, offset):
    if eid < 0 or eid > 0xffffffff or offset < 0 or offset > 0xffffffff:
        raise PositionTranslationError("KFX coordinate is out of range")
    raw = bytes((1,)) + eid.to_bytes(4, "little") + offset.to_bytes(4, "little")
    return base64.b64encode(raw).decode("ascii")


def translate_xpointer(epub_path, xpointer):
    match = _XPOINTER_RE.match(xpointer or "")
    if not match:
        raise PositionTranslationError("invalid normalized XPointer")
    character_offset = int(match.group("offset"))
    steps = [step for step in match.group("path").split("/") if step]
    if len(steps) < 3 or steps[0] != "body":
        raise PositionTranslationError("unsupported normalized XPointer root")

    fragment_name, fragment_index = _parse_step(steps[1])
    if fragment_name != "DocFragment":
        raise PositionTranslationError("XPointer does not identify an EPUB document")

    with zipfile.ZipFile(epub_path) as epub:
        spine = _read_spine(epub)
        if fragment_index > len(spine):
            raise PositionTranslationError("XPointer document is outside EPUB spine")
        document = ElementTree.fromstring(epub.read(spine[fragment_index - 1]))
        bodies = [node for node in document.iter() if _local_name(node.tag) == "body"]
        if not bodies:
            raise PositionTranslationError("EPUB document body is missing")
        current = bodies[0]
        ancestors = [current]
        target_node = None
        target_text = None

        for raw_step in steps[2:]:
            name, index = _parse_step(raw_step)
            if name == "body" and current is bodies[0] and len(ancestors) == 1:
                continue
            if name == "text()":
                nodes = _text_nodes(current)
                if index > len(nodes):
                    raise PositionTranslationError("XPointer text node is missing")
                target_node, target_text = nodes[index - 1]
                continue
            matches = _children_named(current, name)
            if index > len(matches):
                raise PositionTranslationError("XPointer element is missing")
            current = matches[index - 1]
            ancestors.append(current)

        kfx_element = next(
            (element for element in reversed(ancestors) if element.get("data-kfx-eid") is not None),
            None,
        )
        if kfx_element is None:
            raise PositionTranslationError("XPointer has no KFX position anchor")

        if target_text is None:
            target_node = (current, "text")
            target_text = current.text or ""
        if character_offset > len(target_text):
            raise PositionTranslationError("XPointer character offset is outside text node")

        eid = int(kfx_element.get("data-kfx-eid"))
        base_pid_value = kfx_element.get("data-kfx-pid")
        if base_pid_value is None:
            raise PositionTranslationError("KFX element has an ambiguous page-id base")
        eid_offset = _descendant_text_length_before(kfx_element, target_node)
        eid_offset += character_offset
        base_pid = int(base_pid_value)
        return {
            "eid": eid,
            "eid_offset": eid_offset,
            "pid": base_pid + eid_offset,
            "long": _decode_long_position(eid, eid_offset),
        }


def translate_pair(epub_path, start_xpointer, end_xpointer):
    start = translate_xpointer(epub_path, start_xpointer)
    end = translate_xpointer(epub_path, end_xpointer)
    return {"start": start, "end": end}


def _decode_native_long(long_position):
    try:
        raw = base64.b64decode(long_position, validate=True)
    except (ValueError, TypeError) as error:
        raise PositionTranslationError("invalid native long position") from error
    if len(raw) != 9 or raw[0] != 1:
        raise PositionTranslationError("unsupported native long position")
    return int.from_bytes(raw[1:5], "little"), int.from_bytes(raw[5:9], "little")


def _element_step(element, parent):
    name = _local_name(element.tag)
    siblings = _children_named(parent, name)
    index = siblings.index(element) + 1
    return name if index == 1 else "%s[%d]" % (name, index)


def _xpointer_for_text(body, target_parent, target_node, offset, fragment_index):
    parents = {child: parent for parent in body.iter() for child in list(parent)}
    ancestors = []
    current = target_parent
    while current is not body:
        parent = parents.get(current)
        if parent is None:
            raise PositionTranslationError("native position is outside EPUB body")
        ancestors.append(_element_step(current, parent))
        current = parent
    ancestors.reverse()
    text_nodes = [node for node, _text in _text_nodes(target_parent)]
    try:
        text_index = text_nodes.index(target_node) + 1
    except ValueError as error:
        raise PositionTranslationError("native text node is unavailable") from error
    fragment = "DocFragment" if fragment_index == 1 else "DocFragment[%d]" % fragment_index
    steps = ["body", fragment, "body"] + ancestors
    text_step = "text()" if text_index == 1 else "text()[%d]" % text_index
    steps.append(text_step)
    return "/" + "/".join(steps) + "." + str(offset)


def _find_text_at_offset(anchor, requested_offset):
    consumed = 0

    def visit(element):
        nonlocal consumed
        if element.text is not None:
            length = len(element.text)
            if requested_offset <= consumed + length:
                return element, (element, "text"), requested_offset - consumed
            consumed += length
        for child in list(element):
            found = visit(child)
            if found is not None:
                return found
            if child.tail is not None:
                length = len(child.tail)
                if requested_offset <= consumed + length:
                    return element, (child, "tail"), requested_offset - consumed
                consumed += length
        return None

    result = visit(anchor)
    if result is None:
        raise PositionTranslationError("native offset is outside KFX element")
    return result


def translate_native_position(epub_path, long_position):
    """Translate an authoritative Kindle long position back to KOReader XPointer."""
    eid, eid_offset = _decode_native_long(long_position)
    with zipfile.ZipFile(epub_path) as epub:
        spine = _read_spine(epub)
        for fragment_index, document_path in enumerate(spine, 1):
            document = ElementTree.fromstring(epub.read(document_path))
            bodies = [node for node in document.iter() if _local_name(node.tag) == "body"]
            if not bodies:
                continue
            body = bodies[0]
            for anchor in body.iter():
                if anchor.get("data-kfx-eid") != str(eid):
                    continue
                target_parent, target_node, text_offset = _find_text_at_offset(
                    anchor, eid_offset)
                xpointer = _xpointer_for_text(
                    body, target_parent, target_node, text_offset, fragment_index)
                verified = translate_xpointer(epub_path, xpointer)
                if verified["long"] != long_position:
                    raise PositionTranslationError("reverse position verification failed")
                return {
                    "xpointer": xpointer,
                    "eid": eid,
                    "eid_offset": eid_offset,
                    "pid": verified["pid"],
                    "long": long_position,
                }
    raise PositionTranslationError("native KFX element is missing from EPUB")
