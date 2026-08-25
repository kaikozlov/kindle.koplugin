import os
import re
import sys
import unittest


PYTHON_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if PYTHON_DIR not in sys.path:
    sys.path.insert(0, PYTHON_DIR)


def _source():
    with open(os.path.join(PYTHON_DIR, "kindle_helper.py")) as handle:
        return handle.read()


def _dispatch_commands():
    block = re.search(r"dispatch = \{(.*?)\n    \}", _source(), re.S).group(1)
    return set(re.findall(r'"([^"]+)":', block))


class DispatchWiringTests(unittest.TestCase):
    def test_every_registered_subcommand_has_a_dispatch_entry(self):
        source = _source()
        registered = set(re.findall(r'sub\.add_parser\("([^"]+)"\)', source))
        dispatched = _dispatch_commands()

        self.assertTrue(registered, "no subcommands found")
        self.assertEqual(
            set(), registered - dispatched,
            "subcommands registered but never dispatched",
        )
        self.assertEqual(
            set(), dispatched - registered,
            "dispatch entries without a registered subcommand",
        )

    def test_sidecar_and_batch_commands_are_wired(self):
        dispatched = _dispatch_commands()
        for command in ("read-native-sidecar", "write-native-sidecar", "read-close-state"):
            self.assertIn(command, dispatched, command + " must stay dispatchable")


if __name__ == "__main__":
    unittest.main()
