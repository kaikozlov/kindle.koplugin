import os
import sys
import unittest
from collections import namedtuple
from xml.etree import ElementTree


PYTHON_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if PYTHON_DIR not in sys.path:
    sys.path.insert(0, PYTHON_DIR)

from kfx_position_map import tag_position_element, unique_eid_base_pids


ContentChunk = namedtuple(
    "ContentChunk",
    "pid eid eid_offset length section_name text",
    defaults=[None],
)


class KfxPositionMapTests(unittest.TestCase):
    def test_unique_eid_base_pid_uses_coordinate_data_only(self):
        chunks = [
            ContentChunk(100, 7, 0, 5, "section-1", text="hello"),
            ContentChunk(105, 7, 5, 4, "section-1", text="book"),
        ]

        unique_bases = unique_eid_base_pids(chunks)

        self.assertEqual({7: 100}, unique_bases)

    def test_ambiguous_eid_does_not_publish_a_misleading_base_pid(self):
        chunks = [
            ContentChunk(10, 3, 0, 1, "a", text="x"),
            ContentChunk(50, 3, 0, 1, "b", text="y"),
        ]

        unique_bases = unique_eid_base_pids(chunks)
        elem = ElementTree.Element("span")
        tag_position_element(elem, 3, unique_bases)

        self.assertEqual("3", elem.get("data-kfx-eid"))
        self.assertIsNone(elem.get("data-kfx-pid"))

    def test_unique_eid_tags_element_with_eid_and_base_pid(self):
        chunks = [ContentChunk(42, 9, 2, 3, "section", text="abc")]
        unique_bases = unique_eid_base_pids(chunks)
        elem = ElementTree.Element("span")

        tag_position_element(elem, 9, unique_bases)

        self.assertEqual("9", elem.get("data-kfx-eid"))
        self.assertEqual("40", elem.get("data-kfx-pid"))


if __name__ == "__main__":
    unittest.main()
