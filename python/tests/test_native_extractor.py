import os
import stat
import sys
import tempfile
import unittest
from unittest import mock


PYTHON_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if PYTHON_DIR not in sys.path:
    sys.path.insert(0, PYTHON_DIR)

from dedrm import native_extractor  # noqa: E402


class NativeExtractorTests(unittest.TestCase):
    def test_parse_key_file_accepts_only_16_byte_secret_keys(self):
        with tempfile.NamedTemporaryFile(mode="w") as key_file:
            key_file.write("key-one$secret_key:" + "11" * 16 + "\n")
            key_file.write("invalid$secret_key:abcd\n")
            key_file.write("not a key line\n")
            key_file.flush()

            keys = native_extractor.parse_key_file(key_file.name)

        self.assertEqual({"key-one": bytes.fromhex("11" * 16)}, keys)

    def test_find_executable_uses_first_compatible_binary(self):
        with tempfile.TemporaryDirectory() as native_dir:
            first = os.path.join(native_dir, native_extractor.CANDIDATE_NAMES[0])
            second = os.path.join(native_dir, native_extractor.CANDIDATE_NAMES[1])
            for path in (first, second):
                open(path, "wb").close()
                os.chmod(path, os.stat(path).st_mode | stat.S_IXUSR)

            results = [
                mock.Mock(returncode=1),
                mock.Mock(returncode=0),
            ]
            with mock.patch.object(native_extractor.subprocess, "run", side_effect=results):
                executable = native_extractor.find_executable(native_dir=native_dir)

        self.assertEqual(second, executable)

    def test_extract_page_keys_removes_generated_keyfile(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            key_file = os.path.join(tmpdir, "keyfile.txt")

            def run_extractor(args, **kwargs):
                with open(key_file, "w") as output:
                    output.write("key-one$secret_key:" + "22" * 16 + "\n")
                return mock.Mock(returncode=0, stdout="", stderr="")

            with mock.patch.object(native_extractor, "find_executable", return_value="/native"), \
                    mock.patch.object(
                        native_extractor.subprocess,
                        "run",
                        side_effect=run_extractor,
                    ):
                keys = native_extractor.extract_page_keys(key_file=key_file)

            self.assertEqual(bytes.fromhex("22" * 16), keys["key-one"])
            self.assertFalse(os.path.exists(key_file))

    def test_extract_page_keys_removes_keyfile_after_failure(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            key_file = os.path.join(tmpdir, "keyfile.txt")

            def fail_extractor(args, **kwargs):
                with open(key_file, "w") as output:
                    output.write("sensitive")
                return mock.Mock(returncode=2, stdout="failed", stderr="")

            with mock.patch.object(native_extractor, "find_executable", return_value="/native"), \
                    mock.patch.object(
                        native_extractor.subprocess,
                        "run",
                        side_effect=fail_extractor,
                    ), self.assertRaisesRegex(RuntimeError, "exit 2"):
                native_extractor.extract_page_keys(key_file=key_file)

            self.assertFalse(os.path.exists(key_file))


if __name__ == "__main__":
    unittest.main()
