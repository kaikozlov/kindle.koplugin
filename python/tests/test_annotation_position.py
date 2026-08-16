import os
import sys
import tempfile
import unittest
import zipfile


PYTHON_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if PYTHON_DIR not in sys.path:
    sys.path.insert(0, PYTHON_DIR)

from annotation_position import normalize_annotation_end, normalize_annotation_ends


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
  <p data-kfx-eid="1140" data-kfx-pid="27132">Second</p>
</body></html>'''


class AnnotationPositionTests(unittest.TestCase):
    def make_epub(self):
        handle, path = tempfile.mkstemp(suffix=".epub")
        os.close(handle)
        with zipfile.ZipFile(path, "w") as epub:
            epub.writestr("META-INF/container.xml", CONTAINER)
            epub.writestr("OEBPS/content.opf", OPF)
            epub.writestr("OEBPS/one.xhtml", XHTML)
        self.addCleanup(os.remove, path)
        return path

    def test_moves_terminal_exclusive_end_to_last_selected_character(self):
        end = {"eid": 1139, "eid_offset": 17, "pid": 27132, "long": "old"}
        result = normalize_annotation_end(
            self.make_epub(), {"pid": 27116}, end)
        self.assertEqual(16, result["eid_offset"])
        self.assertEqual(27131, result["pid"])
        self.assertEqual("AXMEAAAQAAAA", result["long"])
        self.assertEqual(17, end["eid_offset"])

    def test_preserves_nonterminal_end(self):
        end = {"eid": 1139, "eid_offset": 11, "pid": 27126, "long": "same"}
        self.assertIs(
            end,
            normalize_annotation_end(self.make_epub(), {"pid": 27116}, end),
        )

    def test_does_not_move_zero_length_range_backwards(self):
        end = {"eid": 1139, "eid_offset": 17, "pid": 27132, "long": "same"}
        self.assertIs(
            end,
            normalize_annotation_end(self.make_epub(), {"pid": 27132}, end),
        )

    def test_normalizes_a_batch_without_mutating_translated_positions(self):
        positions = [
            {
                "start": {"pid": 27116},
                "end": {"eid": 1139, "eid_offset": 17, "pid": 27132, "long": "old"},
            },
            {
                "start": {"pid": 27133},
                "end": {"eid": 1140, "eid_offset": 6, "pid": 27138, "long": "old"},
            },
        ]
        result = normalize_annotation_ends(self.make_epub(), positions)
        self.assertEqual([27131, 27137], [item["end"]["pid"] for item in result])
        self.assertEqual([27132, 27138], [item["end"]["pid"] for item in positions])

    def test_empty_batch_does_not_open_the_epub(self):
        self.assertEqual([], normalize_annotation_ends("/missing.epub", []))

    def test_invalid_epub_metadata_has_a_stable_error(self):
        handle, path = tempfile.mkstemp(suffix=".epub")
        os.close(handle)
        with zipfile.ZipFile(path, "w") as epub:
            epub.writestr("META-INF/container.xml", b"not xml")
        self.addCleanup(os.remove, path)
        with self.assertRaisesRegex(ValueError, "invalid EPUB position metadata"):
            normalize_annotation_ends(path, [{"start": {}, "end": {}}])


if __name__ == "__main__":
    unittest.main()
