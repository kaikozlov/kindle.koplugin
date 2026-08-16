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

    def process_content(self, eid, elem):
        return self.process_position(eid, 0, elem)

    def process_page_template(self, eid, elem):
        return self.process_position(eid, 0, elem)


class KfxPositionAdapterTests(unittest.TestCase):
    def test_runtime_adapter_tags_only_content_positions_and_restores_class(self):
        original_init = FakeEpub.__init__
        original_process = FakeEpub.process_position
        content_element = ElementTree.Element("span")
        template_element = ElementTree.Element("body")

        with position_metadata_conversion(FakeEpub):
            converter = FakeEpub(FakeBook())
            result = converter.process_content(7, content_element)
            converter.process_page_template(7, template_element)

            self.assertEqual("original-result", result)
            self.assertEqual("yes", content_element.get("original-called"))
            self.assertEqual("7", content_element.get("data-kfx-eid"))
            self.assertEqual("100", content_element.get("data-kfx-pid"))
            self.assertEqual("yes", template_element.get("original-called"))
            self.assertIsNone(template_element.get("data-kfx-eid"))
            self.assertIsNone(template_element.get("data-kfx-pid"))

        self.assertIs(FakeEpub.__init__, original_init)
        self.assertIs(FakeEpub.process_position, original_process)

    def test_nonzero_content_offsets_do_not_add_element_metadata(self):
        element = ElementTree.Element("span")

        class OffsetFakeEpub(FakeEpub):
            def process_content(self, eid, elem):
                return self.process_position(eid, 3, elem)

        with position_metadata_conversion(OffsetFakeEpub):
            converter = OffsetFakeEpub(FakeBook())
            converter.process_content(7, element)

        self.assertIsNone(element.get("data-kfx-eid"))
        self.assertIsNone(element.get("data-kfx-pid"))


if __name__ == "__main__":
    unittest.main()
