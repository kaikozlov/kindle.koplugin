import contextlib
import io
import json
import os
import sys
import tempfile
import types
import unittest
from unittest import mock


PYTHON_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if PYTHON_DIR not in sys.path:
    sys.path.insert(0, PYTHON_DIR)

import kindle_helper


class PlaintextDrmIonTests(unittest.TestCase):
    def make_args(self, input_path, output_path):
        return types.SimpleNamespace(
            input=input_path,
            output=output_path,
            cache_dir="",
        )

    def test_convert_attempts_plaintext_drmion_without_cached_key(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            input_path = os.path.join(tmpdir, "book.kfx")
            output_path = os.path.join(tmpdir, "book.epub")
            drmion_data = kindle_helper.DRMION_SIGNATURE + b"plaintext-envelope"
            with open(input_path, "wb") as input_file:
                input_file.write(drmion_data)

            stdout = io.StringIO()
            with mock.patch.object(kindle_helper, "_find_page_key", return_value=None), \
                    mock.patch.object(
                        kindle_helper,
                        "_decrypt_drmion",
                        return_value=b"CONT plaintext book",
                    ) as decrypt, \
                    mock.patch("kfxlib.YJ_Book") as yj_book, \
                    contextlib.redirect_stdout(stdout), \
                    self.assertRaises(SystemExit) as exited:
                yj_book.return_value.convert_to_epub.return_value = b"epub-data"
                kindle_helper.cmd_convert(self.make_args(input_path, output_path))

            self.assertEqual(0, exited.exception.code)
            decrypt.assert_called_once_with(drmion_data, None)
            with open(output_path, "rb") as output_file:
                self.assertEqual(b"epub-data", output_file.read())
            self.assertTrue(json.loads(stdout.getvalue())["ok"])

    def test_convert_requests_key_when_plaintext_attempt_reaches_encryption(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            input_path = os.path.join(tmpdir, "book.kfx")
            output_path = os.path.join(tmpdir, "book.epub")
            with open(input_path, "wb") as input_file:
                input_file.write(kindle_helper.DRMION_SIGNATURE + b"encrypted-envelope")

            stdout = io.StringIO()
            with mock.patch.object(kindle_helper, "_find_page_key", return_value=None), \
                    mock.patch.object(
                        kindle_helper,
                        "_decrypt_drmion",
                        side_effect=Exception("Unable to obtain secret key"),
                    ), contextlib.redirect_stdout(stdout), \
                    self.assertRaises(SystemExit) as exited:
                kindle_helper.cmd_convert(self.make_args(input_path, output_path))

            self.assertEqual(0, exited.exception.code)
            result = json.loads(stdout.getvalue())
            self.assertFalse(result["ok"])
            self.assertEqual("drm", result["code"])
            self.assertIn("no cached page key", result["message"])

    def test_decrypt_accepts_plaintext_drmion_without_cached_key(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            input_path = os.path.join(tmpdir, "book.kfx")
            output_path = os.path.join(tmpdir, "book.kfx-zip")
            drmion_data = kindle_helper.DRMION_SIGNATURE + b"plaintext-envelope"
            with open(input_path, "wb") as input_file:
                input_file.write(drmion_data)

            with mock.patch.object(kindle_helper, "_find_page_key", return_value=None), \
                    mock.patch.object(
                        kindle_helper,
                        "_decrypt_drmion",
                        return_value=b"CONT plaintext book",
                    ) as decrypt:
                kindle_helper.cmd_decrypt(self.make_args(input_path, output_path))

            decrypt.assert_called_once_with(drmion_data, None)
            self.assertTrue(os.path.isfile(output_path))


if __name__ == "__main__":
    unittest.main()
