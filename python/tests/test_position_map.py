import json
import os
import sys
import tempfile
import unittest
import zipfile


PYTHON_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if PYTHON_DIR not in sys.path:
    sys.path.insert(0, PYTHON_DIR)

from position_map import PositionMapError, build_position_map  # noqa: E402


CONTAINER = b'''<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles>
</container>'''
OPF = b'''<package xmlns="http://www.idpf.org/2007/opf">
  <manifest><item id="one" href="one.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="one"/></spine>
</package>'''
XHTML = b'''<html xmlns="http://www.w3.org/1999/xhtml"><body>
  <div data-kfx-eid="1138" data-kfx-pid="27100"><p data-kfx-eid="1139" data-kfx-pid="27115">Hello <em>brave</em> new world</p><p>orphan text</p></div>
  <div data-kfx-eid="1141" data-kfx-pid="27300"><span>a</span><span>b</span>after spans</div>
  <p data-kfx-eid="1140" data-kfx-pid="27200">Tail node target</p>
</body></html>'''


class PositionMapTests(unittest.TestCase):
    def make_epub(self):
        handle, path = tempfile.mkstemp(suffix=".epub")
        os.close(handle)
        with zipfile.ZipFile(path, "w") as epub:
            epub.writestr("META-INF/container.xml", CONTAINER)
            epub.writestr("OEBPS/content.opf", OPF)
            epub.writestr("OEBPS/one.xhtml", XHTML)
        self.addCleanup(os.remove, path)
        return path

    def test_records_anchors_elements_and_text_nodes(self):
        payload = build_position_map(self.make_epub())

        self.assertEqual(1, payload["version"])
        fragment = payload["fragments"][0]
        self.assertEqual("OEBPS/one.xhtml", fragment["path"])

        by_eid = {a["eid"]: a for a in fragment["anchors"]}
        self.assertIn(1138, by_eid)
        self.assertIn(1139, by_eid)
        self.assertIn(1140, by_eid)
        self.assertIn(1141, by_eid)
        anchor = by_eid[1139]
        self.assertEqual("div/p", anchor["p"])
        self.assertEqual(27115, anchor["pid"])
        # "Hello " + "brave" + " new world" = 21 chars inside the anchor.
        self.assertEqual(21, anchor["t"])
        # Document-order nodes: p.text, then the em child's subtree text
        # (recorded on the em element), then em's tail on p.
        self.assertEqual(
            [("div/p", 1, 0, 6), ("div/p/em", 1, 6, 5), ("div/p", 2, 11, 10)],
            [(n["p"], n["n"], n["c"], n["v"]) for n in anchor["nodes"]],
        )
        self.assertEqual(21 + 11, by_eid[1138]["t"])
        # The unanchored div/p[2] belongs to the div anchor (a=1); the
        # body-level <p> is its own anchor (a=3, creation order 1138/1139/1140).
        self.assertEqual(1, fragment["elements"]["div/p[2]"]["a"])
        # The element-text-less div[2] anchor carries its last child's tail as
        # text()[1] — no phantom text node before it.
        tail_anchor = by_eid[1141]
        self.assertEqual(
            [("div[2]/span", 1, 0, 1), ("div[2]/span[2]", 1, 1, 1), ("div[2]", 1, 2, 11)],
            [(n["p"], n["n"], n["c"], n["v"]) for n in tail_anchor["nodes"]],
        )
        # div[2] has no direct text; its own text-node list is only the last
        # child's tail (span texts belong to their own elements).
        self.assertEqual([11], fragment["elements"]["div[2]"]["l"])
        self.assertGreater(payload["max_pid"], 27200)

    def test_rejects_epubs_without_anchors(self):
        handle, path = tempfile.mkstemp(suffix=".epub")
        os.close(handle)
        with zipfile.ZipFile(path, "w") as epub:
            epub.writestr("META-INF/container.xml", CONTAINER)
            epub.writestr("OEBPS/content.opf", OPF)
            epub.writestr("OEBPS/one.xhtml",
                          b'<html><body><p>no anchors</p></body></html>')
        self.addCleanup(os.remove, path)

        with self.assertRaises(PositionMapError):
            build_position_map(path)

    def test_serializes_to_compact_json(self):
        epub = self.make_epub()
        handle, out = tempfile.mkstemp(suffix=".positions.json")
        os.close(handle)
        self.addCleanup(os.remove, out)

        from position_map import write_position_map
        write_position_map(epub, out)

        with open(out, "r", encoding="utf-8") as source:
            payload = json.load(source)
        self.assertEqual(1, payload["version"])
        self.assertTrue(payload["fragments"][0]["anchors"])


if __name__ == "__main__":
    unittest.main()
