import base64
import os
import stat
import struct
import tempfile
import time
from dataclasses import dataclass


SIGNATURE = b"\x00\x00\x00\x00\x00\x1a\xb1\x26"

BOOLEAN = 0
INT = 1
LONG = 2
UTF = 3
DOUBLE = 4
SHORT = 5
FLOAT = 6
BYTE = 7
CHAR = 9
OBJECT_BEGIN = -2
OBJECT_END = -1


class KrdsError(ValueError):
    pass


@dataclass
class Value:
    tag: int
    value: object
    utf_empty_sentinel: bool = False


@dataclass
class ObjectValue:
    name: str
    values: list


class Reader:
    def __init__(self, data):
        self.data = data
        self.offset = 0

    def read(self, size):
        end = self.offset + size
        if size < 0 or end > len(self.data):
            raise KrdsError("truncated KRDS value")
        result = self.data[self.offset:end]
        self.offset = end
        return result

    def unpack(self, fmt):
        size = struct.calcsize(fmt)
        return struct.unpack(fmt, self.read(size))[0]

    def read_utf_body(self):
        empty = self.unpack("b")
        if empty == 1:
            return "", True
        if empty != 0:
            raise KrdsError("invalid KRDS UTF empty marker")
        size = self.unpack(">H")
        try:
            return self.read(size).decode("utf-8"), False
        except UnicodeDecodeError as error:
            raise KrdsError("invalid KRDS UTF value") from error

    def read_value(self):
        tag = self.unpack("b")
        if tag == BOOLEAN:
            value = self.unpack("b")
            if value not in (0, 1):
                raise KrdsError("invalid KRDS boolean")
            return Value(tag, value == 1)
        if tag == INT:
            return Value(tag, self.unpack(">i"))
        if tag == LONG:
            return Value(tag, self.unpack(">q"))
        if tag == UTF:
            value, empty = self.read_utf_body()
            return Value(tag, value, empty)
        if tag == DOUBLE:
            return Value(tag, self.unpack(">d"))
        if tag == SHORT:
            return Value(tag, self.unpack(">h"))
        if tag == FLOAT:
            return Value(tag, self.unpack(">f"))
        if tag == BYTE:
            return Value(tag, self.unpack("b"))
        if tag == CHAR:
            try:
                return Value(tag, self.read(1).decode("utf-8"))
            except UnicodeDecodeError as error:
                raise KrdsError("invalid KRDS character") from error
        if tag == OBJECT_BEGIN:
            name, _empty = self.read_utf_body()
            values = []
            while True:
                if self.offset >= len(self.data):
                    raise KrdsError("unterminated KRDS object")
                next_tag = struct.unpack_from("b", self.data, self.offset)[0]
                if next_tag == OBJECT_END:
                    self.offset += 1
                    break
                values.append(self.read_value())
            return Value(tag, ObjectValue(name, values))
        raise KrdsError("unknown KRDS datatype %d" % tag)


def _utf_body(value, empty_sentinel=False):
    encoded = value.encode("utf-8")
    if empty_sentinel and not encoded:
        return b"\x01"
    if len(encoded) > 0xffff:
        raise KrdsError("KRDS UTF value is too long")
    return b"\x00" + struct.pack(">H", len(encoded)) + encoded


def encode_value(node):
    tag = node.tag
    if tag == BOOLEAN:
        body = struct.pack("b", 1 if node.value else 0)
    elif tag == INT:
        body = struct.pack(">i", node.value)
    elif tag == LONG:
        body = struct.pack(">q", node.value)
    elif tag == UTF:
        body = _utf_body(node.value, node.utf_empty_sentinel)
    elif tag == DOUBLE:
        body = struct.pack(">d", node.value)
    elif tag == SHORT:
        body = struct.pack(">h", node.value)
    elif tag == FLOAT:
        body = struct.pack(">f", node.value)
    elif tag == BYTE:
        body = struct.pack("b", node.value)
    elif tag == CHAR:
        encoded = node.value.encode("utf-8")
        if len(encoded) != 1:
            raise KrdsError("KRDS character must encode to one byte")
        body = encoded
    elif tag == OBJECT_BEGIN:
        obj = node.value
        body = _utf_body(obj.name)
        body += b"".join(encode_value(value) for value in obj.values)
        body += struct.pack("b", OBJECT_END)
    else:
        raise KrdsError("unknown KRDS datatype %d" % tag)
    return struct.pack("b", tag) + body


class Store:
    def __init__(self, first, count, values):
        self.first = first
        self.count = count
        self.values = values

    @classmethod
    def parse(cls, data):
        reader = Reader(data)
        if reader.read(len(SIGNATURE)) != SIGNATURE:
            raise KrdsError("invalid KRDS signature")
        first = reader.read_value()
        count = reader.read_value()
        if first.value != 1 or count.tag != INT or count.value < 0:
            raise KrdsError("invalid KRDS header")
        values = [reader.read_value() for _ in range(count.value)]
        if reader.offset != len(data):
            raise KrdsError("extra data after KRDS store")
        return cls(first, count, values)

    def encode(self):
        if self.count.value != len(self.values):
            raise KrdsError("KRDS object count mismatch")
        return SIGNATURE + encode_value(self.first) + encode_value(self.count) + b"".join(
            encode_value(value) for value in self.values)

    def objects(self, name=None):
        result = []

        def visit(node):
            if node.tag != OBJECT_BEGIN:
                return
            obj = node.value
            if name is None or obj.name == name:
                result.append(obj)
            for child in obj.values:
                visit(child)

        for value in self.values:
            visit(value)
        return result


def parse_position(value):
    if not isinstance(value, str) or value.count(":") != 1:
        raise KrdsError("invalid Kindle position string")
    long_position, pid_text = value.split(":", 1)
    try:
        raw = base64.b64decode(long_position, validate=True)
        pid = int(pid_text)
    except (ValueError, TypeError) as error:
        raise KrdsError("invalid Kindle position string") from error
    if len(raw) != 9 or raw[0] != 1 or pid < 0:
        raise KrdsError("unsupported Kindle position string")
    return {"long": long_position, "pid": pid}


def format_position(long_position, pid):
    parsed = parse_position("%s:%s" % (long_position, pid))
    return "%s:%d" % (parsed["long"], parsed["pid"])


def _position_from_object(obj):
    values = obj.values
    if obj.name == "lpr":
        if values and values[0].tag == UTF:
            return values[0].value, None
        if len(values) >= 3 and values[0].tag == BYTE and values[0].value <= 2:
            if values[1].tag == UTF and values[2].tag == LONG:
                return values[1].value, values[2].value
    elif obj.name in ("updated_lpr", "fpr"):
        if len(values) >= 2 and values[0].tag == UTF and values[1].tag == LONG:
            return values[0].value, values[1].value
    elif obj.name == "erl" and values and values[0].tag == UTF:
        return values[0].value, None
    raise KrdsError("unsupported %s object" % obj.name)


def read_position_data(data):
    store = Store.parse(data)
    candidates = []
    for name in ("lpr", "updated_lpr", "erl"):
        for obj in store.objects(name):
            position, timestamp_ms = _position_from_object(obj)
            parsed = parse_position(position)
            parsed["timestamp_ms"] = timestamp_ms
            candidates.append((name, parsed))
    if not candidates:
        raise KrdsError("KRDS store has no readable last-page position")
    candidates.sort(key=lambda item: (
        1 if item[0] == "lpr" else 0,
        item[1]["timestamp_ms"] if item[1]["timestamp_ms"] is not None else -1,
    ), reverse=True)
    return candidates[0][1]


def _set_object_position(obj, position, timestamp_ms):
    values = obj.values
    if obj.name == "lpr":
        if values and values[0].tag == UTF:
            values[0] = Value(UTF, position)
            return
        if len(values) >= 3 and values[0].tag == BYTE and values[0].value <= 2:
            values[1] = Value(UTF, position)
            values[2] = Value(LONG, timestamp_ms)
            return
    elif obj.name in ("updated_lpr", "fpr"):
        if len(values) >= 2 and values[0].tag == UTF and values[1].tag == LONG:
            values[0] = Value(UTF, position)
            values[1] = Value(LONG, timestamp_ms)
            return
    elif obj.name == "erl" and values and values[0].tag == UTF:
        values[0] = Value(UTF, position)
        return
    raise KrdsError("unsupported %s object" % obj.name)


def update_position_data(data, long_position, pid, timestamp_ms=None):
    position = format_position(long_position, pid)
    timestamp_ms = int(time.time() * 1000) if timestamp_ms is None else int(timestamp_ms)
    if timestamp_ms < 0:
        raise KrdsError("invalid Kindle position timestamp")
    store = Store.parse(data)
    if store.encode() != data:
        raise KrdsError("KRDS round-trip changed unmodified data")

    updated = False
    for name in ("lpr", "updated_lpr", "erl"):
        for obj in store.objects(name):
            _set_object_position(obj, position, timestamp_ms)
            updated = True

    for obj in store.objects("fpr"):
        current, _current_time = _position_from_object(obj)
        if parse_position(current)["pid"] < pid:
            _set_object_position(obj, position, timestamp_ms)

    for obj in store.objects("sync_lpr"):
        if obj.values and obj.values[0].tag == BOOLEAN:
            obj.values[0] = Value(BOOLEAN, True)

    if not updated:
        raise KrdsError("KRDS store has no writable last-page position")
    encoded = store.encode()
    readback = read_position_data(encoded)
    if readback["long"] != long_position or readback["pid"] != pid:
        raise KrdsError("KRDS position readback mismatch")
    return encoded, readback


def _position_sidecar_candidates(source_path):
    stem, _extension = os.path.splitext(source_path)
    sidecar_dir = stem + ".sdr"
    if not os.path.isdir(sidecar_dir):
        raise KrdsError("Kindle sidecar directory is missing")
    candidates = []
    for name in os.listdir(sidecar_dir):
        extension = os.path.splitext(name)[1].lower()
        if extension not in (".yjf", ".yjr", ".azw3f", ".azw3r"):
            continue
        path = os.path.join(sidecar_dir, name)
        if not os.path.isfile(path):
            continue
        try:
            with open(path, "rb") as source:
                read_position_data(source.read())
        except (OSError, KrdsError):
            continue
        candidates.append((-os.path.getmtime(path), 0 if extension in (".yjf", ".azw3f") else 1, path))
    if not candidates:
        raise KrdsError("Kindle position sidecar is unavailable")
    candidates.sort()
    return [entry[2] for entry in candidates]


def find_position_sidecar(source_path):
    return _position_sidecar_candidates(source_path)[0]


def read_position_file(source_path):
    sidecar_path = find_position_sidecar(source_path)
    with open(sidecar_path, "rb") as source:
        result = read_position_data(source.read())
    result["sidecar_path"] = sidecar_path
    return result


def _rewrite_position_sidecar(sidecar_path, long_position, pid, timestamp_ms):
    with open(sidecar_path, "rb") as source:
        original = source.read()
    updated, result = update_position_data(original, long_position, pid, timestamp_ms)
    original_mode = stat.S_IMODE(os.stat(sidecar_path).st_mode)
    parent = os.path.dirname(sidecar_path)
    temp_fd, temp_path = tempfile.mkstemp(prefix=".kindle-position-", dir=parent)
    try:
        with os.fdopen(temp_fd, "wb") as target:
            target.write(updated)
            target.flush()
            os.fsync(target.fileno())
        os.chmod(temp_path, original_mode)
        with open(temp_path, "rb") as check:
            verified = read_position_data(check.read())
        if verified["long"] != long_position or verified["pid"] != pid:
            raise KrdsError("temporary KRDS position readback mismatch")
        os.replace(temp_path, sidecar_path)
    finally:
        try:
            os.remove(temp_path)
        except FileNotFoundError:
            pass
    return result


def write_position_file(source_path, long_position, pid, timestamp_ms=None):
    sidecar_paths = _position_sidecar_candidates(source_path)
    # The stock reader may read either sidecar flavor; keep every position
    # store consistent so the next native open lands on the exact coordinate.
    result = None
    for sidecar_path in sidecar_paths:
        result = _rewrite_position_sidecar(sidecar_path, long_position, pid, timestamp_ms)
    result["sidecar_path"] = sidecar_paths[0]
    return result
