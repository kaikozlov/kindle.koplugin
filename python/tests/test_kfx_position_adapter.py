import os
import sys
import unittest
from collections import namedtuple
from xml.etree import ElementTree


PYTHON_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if PYTHON_DIR not in sys.path:
    sys.path.insert(0, PYTHON_DIR)

from kfx_position_adapter import position_metadata_conversion


ContentChunk = namedtuple(
    "ContentChunk",
    "pid eid eid_offset length section_name text",
    defaults=[None],
)


class FakeBook:
    asin = "B012345678"

    def collect_content_position_info(self):
        return [ContentChunk(100, 7, 0, 5, "section", text="hello")]


class FakeEpub:
    def __init__(self, book):
        self.book = book

    def process_position(self, eid, offset, elem):
        elem.set("original-called", "yes")
        return "original-result"


class KfxPositionAdapterTests(unittest.TestCase):
    def test_runtime_adapter_tags_positions_and_restores_class(self):
        original_init = FakeEpub.__init__
        original_process = FakeEpub.process_position
        element = ElementTree.Element("span")

        with position_metadata_conversion(FakeEpub):
            converter = FakeEpub(FakeBook())
            result = converter.process_position(7, 0, element)

            self.assertEqual("original-result", result)
            self.assertEqual("yes", element.get("original-called"))
            self.assertEqual("7", element.get("data-kfx-eid"))
            self.assertEqual("100", element.get("data-kfx-pid"))

        self.assertIs(FakeEpub.__init__, original_init)
        self.assertIs(FakeEpub.process_position, original_process)

    def test_nonzero_offsets_do_not_add_element_metadata(self):
        element = ElementTree.Element("span")

        with position_metadata_conversion(FakeEpub):
            converter = FakeEpub(FakeBook())
            converter.process_position(7, 3, element)

        self.assertIsNone(element.get("data-kfx-eid"))
        self.assertIsNone(element.get("data-kfx-pid"))


if __name__ == "__main__":
    unittest.main()
