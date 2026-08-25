import base64
import json
import os
import struct
import subprocess
import sys
import tempfile
import unittest
import zipfile


PYTHON_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
HELPER = os.path.join(PYTHON_DIR, "kindle_helper.py")

CONTAINER = b'''<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles>
</container>'''
OPF = b'''<package xmlns="http://www.idpf.org/2007/opf">
  <manifest><item id="one" href="one.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="one"/></spine>
</package>'''
XHTML = b'''<html xmlns="http://www.w3.org/1999/xhtml"><body>
  <p data-kfx-eid="1139" data-kfx-pid="27115">Hello <em>brave</em> world</p>
</body></html>'''

SIGNATURE = b"\x00\x00\x00\x00\x00\x1a\xb1\x26"


def _utf(value):
    encoded = value.encode("utf-8")
    return b"\x00" + struct.pack(">H", len(encoded)) + encoded


def _value(tag, payload):
    return struct.pack("b", tag) + payload


def _utf_value(text):
    return _value(3, _utf(text))


def _long_value(number):
    return _value(2, struct.pack(">q", number))


def _object(name, *children):
    return _value(-2, _utf(name) + b"".join(children) + struct.pack("b", -1))


def make_store(eid=1139, offset=0, pid=27115):
    raw = bytes((1,)) + eid.to_bytes(4, "little") + offset.to_bytes(4, "little")
    lpr = base64.b64encode(raw).decode("ascii") + ":" + str(pid)
    objects = [
        _object("updated_lpr", _utf_value(lpr), _long_value(1000), _long_value(-1),
                _utf_value(""), _utf_value("")),
        _object("lpr", _value(7, struct.pack("b", 2)), _utf_value(lpr), _long_value(1100)),
    ]
    return (
        SIGNATURE
        + _long_value(1)
        + _value(1, struct.pack(">i", len(objects)))
        + b"".join(objects)
    )


class ReadCloseStateTests(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.mkdtemp(prefix="kindle-close-state-")
        self.source = os.path.join(self.tmpdir, "book.kfx")
        sidecar_dir = os.path.join(self.tmpdir, "book.sdr")
        os.mkdir(sidecar_dir)
        open(self.source, "wb").close()
        with open(os.path.join(sidecar_dir, "book.yjf"), "wb") as sidecar:
            sidecar.write(make_store())
        self.epub = os.path.join(self.tmpdir, "book.epub")
        with zipfile.ZipFile(self.epub, "w") as epub:
            epub.writestr("META-INF/container.xml", CONTAINER)
            epub.writestr("OEBPS/content.opf", OPF)
            epub.writestr("OEBPS/one.xhtml", XHTML)

    def tearDown(self):
        import shutil
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def run_helper(self):
        return subprocess.run(
            [
                sys.executable, HELPER, "read-close-state",
                "--input", self.source,
                "--epub", self.epub,
                "--xpointer", "/body/DocFragment/body/p/em/text().3",
            ],
            check=False, capture_output=True, text=True,
        )

    def test_returns_both_authority_translations_in_one_invocation(self):
        result = self.run_helper()

        self.assertEqual(0, result.returncode, result.stderr)
        payload = json.loads(result.stdout)
        self.assertTrue(payload["ok"])
        self.assertEqual("AXMEAAAAAAAA", payload["native"]["long"])
        self.assertEqual(27115, payload["native"]["pid"])
        self.assertEqual(1100, payload["native"]["timestamp_ms"])
        self.assertIsInstance(payload["native_xpointer"], str)
        self.assertIsInstance(payload["koreader"]["pid"], int)
        self.assertIsInstance(payload["koreader"]["percent"], float)
        self.assertGreaterEqual(payload["koreader"]["percent"], 0)
        self.assertLessEqual(payload["koreader"]["percent"], 100)

    def test_keeps_koreader_translation_when_no_sidecar_exists(self):
        os.remove(os.path.join(self.tmpdir, "book.sdr", "book.yjf"))

        result = self.run_helper()

        self.assertEqual(0, result.returncode, result.stderr)
        payload = json.loads(result.stdout)
        self.assertTrue(payload["ok"])
        self.assertIsNone(payload.get("native"))
        self.assertIsInstance(payload["native_error"], str)
        self.assertIsInstance(payload["koreader"]["pid"], int)

    def test_fails_when_the_koreader_translation_fails(self):
        bad = os.path.join(self.tmpdir, "bad.epub")
        open(bad, "wb").close()

        result = subprocess.run(
            [
                sys.executable, HELPER, "read-close-state",
                "--input", self.source,
                "--epub", bad,
                "--xpointer", "/body/DocFragment/body/p/em/text().3",
            ],
            check=False, capture_output=True, text=True,
        )

        self.assertNotEqual(0, result.returncode)
        payload = json.loads(result.stdout)
        self.assertFalse(payload["ok"])
        self.assertIsInstance(payload["koreader_error"], str)


if __name__ == "__main__":
    unittest.main()
