import json
import os
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


class KindleHelperPositionTests(unittest.TestCase):
    def make_epub(self):
        handle, path = tempfile.mkstemp(suffix=".epub")
        os.close(handle)
        with zipfile.ZipFile(path, "w") as epub:
            epub.writestr("META-INF/container.xml", CONTAINER)
            epub.writestr("OEBPS/content.opf", OPF)
            epub.writestr("OEBPS/one.xhtml", XHTML)
            epub.writestr("OEBPS/kindle-position-map.json", json.dumps({"version": 1}))
        self.addCleanup(os.remove, path)
        return path

    def run_batch(self, requests):
        request_file = tempfile.NamedTemporaryFile(mode="w", delete=False)
        json.dump(requests, request_file)
        request_file.close()
        self.addCleanup(os.remove, request_file.name)
        return subprocess.run(
            [
                sys.executable,
                HELPER,
                "translate-native-positions",
                "--epub",
                self.make_epub(),
                "--request",
                request_file.name,
            ],
            check=False,
            capture_output=True,
            text=True,
        )

    def test_reverse_translates_annotation_range_batch(self):
        result = self.run_batch([
            {"start": "AXMEAAAJAAAA", "end": "AXMEAAANAAAA"},
            {"start": "AXMEAAAAAAAA", "end": "AXMEAAARAAAA"},
        ])
        self.assertEqual(0, result.returncode, result.stderr)
        payload = json.loads(result.stdout)
        self.assertTrue(payload["ok"])
        self.assertEqual(2, len(payload["positions"]))
        self.assertEqual(
            "/body/DocFragment/body/p/em/text().3",
            payload["positions"][0]["start"]["xpointer"],
        )
        self.assertEqual(27124, payload["positions"][0]["start"]["pid"])
        self.assertEqual("AXMEAAANAAAA", payload["positions"][0]["end"]["long"])

    def test_rejects_incomplete_native_range(self):
        result = self.run_batch([{"start": "AXMEAAAJAAAA"}])
        self.assertNotEqual(0, result.returncode)
        payload = json.loads(result.stdout)
        self.assertFalse(payload["ok"])
        self.assertEqual("native annotation range is incomplete", payload["message"])

    def test_rejects_oversized_batch(self):
        result = self.run_batch([
            {"start": "AXMEAAAJAAAA", "end": "AXMEAAANAAAA"}
            for _ in range(1001)
        ])
        self.assertNotEqual(0, result.returncode)
        payload = json.loads(result.stdout)
        self.assertFalse(payload["ok"])
        self.assertEqual("invalid native position request list", payload["message"])


if __name__ == "__main__":
    unittest.main()
