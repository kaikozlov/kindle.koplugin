import base64
import os
import stat
import struct
import sys
import tempfile
import time
import unittest


PYTHON_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if PYTHON_DIR not in sys.path:
    sys.path.insert(0, PYTHON_DIR)

import krds_position  # noqa: E402


def utf_body(value):
    encoded = value.encode("utf-8")
    return b"\x00" + struct.pack(">H", len(encoded)) + encoded


def value(tag, payload):
    return struct.pack("b", tag) + payload


def utf(value_text):
    return value(krds_position.UTF, utf_body(value_text))


def long(value_number):
    return value(krds_position.LONG, struct.pack(">q", value_number))


def byte(value_number):
    return value(krds_position.BYTE, struct.pack("b", value_number))


def boolean(value_bool):
    return value(krds_position.BOOLEAN, struct.pack("b", 1 if value_bool else 0))


def object_value(name, *children):
    return value(
        krds_position.OBJECT_BEGIN,
        utf_body(name) + b"".join(children) + struct.pack("b", krds_position.OBJECT_END),
    )


def make_store(lpr="AScEAAAAAAAA:3", fpr="ASUKAAAJAgAA:252650"):
    objects = [
        object_value("updated_lpr", utf(lpr), long(1000), long(-1), utf(""), utf("")),
        object_value("fpr", utf(fpr), long(900), long(-1), utf(""), utf("")),
        object_value("sync_lpr", boolean(False)),
        object_value("lpr", byte(2), utf(lpr), long(1100)),
        object_value("unknown.future.object", byte(7), utf("preserve-me")),
    ]
    return (
        krds_position.SIGNATURE
        + long(1)
        + value(krds_position.INT, struct.pack(">i", len(objects)))
        + b"".join(objects)
    )


class KrdsPositionTests(unittest.TestCase):
    def test_round_trips_unknown_values_byte_for_byte(self):
        data = make_store()
        store = krds_position.Store.parse(data)

        self.assertEqual(data, store.encode())
        self.assertEqual(
            {"long": "AScEAAAAAAAA", "pid": 3, "timestamp_ms": 1100},
            krds_position.read_position_data(data),
        )

    def test_updates_exact_lpr_and_advances_furthest_position(self):
        data = make_store()
        updated, readback = krds_position.update_position_data(
            data, "ATwFAACbAAAA", 442741, timestamp_ms=2000)

        self.assertEqual("ATwFAACbAAAA", readback["long"])
        self.assertEqual(442741, readback["pid"])
        self.assertEqual(2000, readback["timestamp_ms"])
        store = krds_position.Store.parse(updated)
        for name in ("lpr", "updated_lpr", "fpr"):
            position, timestamp_ms = krds_position._position_from_object(store.objects(name)[0])
            self.assertEqual("ATwFAACbAAAA:442741", position)
            self.assertEqual(2000, timestamp_ms)
        self.assertTrue(store.objects("sync_lpr")[0].values[0].value)
        self.assertEqual(data[-16:], updated[-16:])

    def test_does_not_move_furthest_position_backwards(self):
        data = make_store(fpr="ATwFAACbAAAA:442741")
        updated, _readback = krds_position.update_position_data(
            data, "AScEAAAAAAAA", 3, timestamp_ms=2000)

        store = krds_position.Store.parse(updated)
        position, timestamp_ms = krds_position._position_from_object(store.objects("fpr")[0])
        self.assertEqual("ATwFAACbAAAA:442741", position)
        self.assertEqual(900, timestamp_ms)

    def test_writes_every_position_sidecar_atomically_without_losing_mode(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            source_path = os.path.join(tmpdir, "book.kfx")
            sidecar_dir = os.path.join(tmpdir, "book.sdr")
            os.mkdir(sidecar_dir)
            open(source_path, "wb").close()
            yjr_path = os.path.join(sidecar_dir, "book.yjr")
            yjf_path = os.path.join(sidecar_dir, "book.yjf")
            with open(yjr_path, "wb") as target:
                target.write(make_store(lpr="AScEAAAAAAAA:2"))
            with open(yjf_path, "wb") as target:
                target.write(make_store())
            os.chmod(yjf_path, 0o640)

            result = krds_position.write_position_file(
                source_path, "ATwFAACbAAAA", 442741, timestamp_ms=2000)

            self.assertEqual(yjf_path, result["sidecar_path"])
            self.assertEqual(0o640, stat.S_IMODE(os.stat(yjf_path).st_mode))
            with open(yjf_path, "rb") as source:
                self.assertEqual(442741, krds_position.read_position_data(source.read())["pid"])
            with open(yjr_path, "rb") as source:
                self.assertEqual(442741, krds_position.read_position_data(source.read())["pid"])

    def test_reads_the_most_recently_updated_sidecar(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            source_path = os.path.join(tmpdir, "book.kfx")
            sidecar_dir = os.path.join(tmpdir, "book.sdr")
            os.mkdir(sidecar_dir)
            open(source_path, "wb").close()
            yjf_path = os.path.join(sidecar_dir, "book.yjf")
            yjr_path = os.path.join(sidecar_dir, "book.yjr")
            with open(yjf_path, "wb") as target:
                target.write(make_store())
            with open(yjr_path, "wb") as target:
                target.write(make_store(lpr="ATwFAACbAAAA:442741"))
            stamp = time.time()
            os.utime(yjf_path, (stamp, stamp))
            os.utime(yjr_path, (stamp + 10, stamp + 10))

            result = krds_position.read_position_file(source_path)

            self.assertEqual(yjr_path, result["sidecar_path"])
            self.assertEqual(442741, result["pid"])

    def test_rejects_malformed_native_position(self):
        invalid_long = base64.b64encode(b"wrong").decode("ascii")
        with self.assertRaises(krds_position.KrdsError):
            krds_position.update_position_data(make_store(), invalid_long, 1)


if __name__ == "__main__":
    unittest.main()
