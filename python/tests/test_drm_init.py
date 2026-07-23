import contextlib
import io
import os
import sys
import tempfile
import unittest
from unittest import mock


PYTHON_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if PYTHON_DIR not in sys.path:
    sys.path.insert(0, PYTHON_DIR)

from dedrm import drm_init


class AccountSecretPreflightTests(unittest.TestCase):
    def run_preflight(self, acsr_path):
        stderr = io.StringIO()
        with mock.patch.object(drm_init, "_ACSR_PATH", acsr_path):
            with contextlib.redirect_stderr(stderr):
                drm_init._preflight_check()
        return stderr.getvalue()

    def test_missing_account_secret_warns_and_continues(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            warning = self.run_preflight(os.path.join(tmpdir, "missing-acsr"))

        self.assertIn("account secret is missing or empty", warning.lower())
        self.assertIn("device serial only", warning.lower())

    def test_empty_account_secret_warns_and_continues(self):
        with tempfile.NamedTemporaryFile() as acsr_file:
            warning = self.run_preflight(acsr_file.name)

        self.assertIn("account secret is missing or empty", warning.lower())
        self.assertIn("device serial only", warning.lower())

    def test_populated_account_secret_does_not_warn(self):
        with tempfile.NamedTemporaryFile(mode="w") as acsr_file:
            acsr_file.write("account-secret\n")
            acsr_file.flush()
            warning = self.run_preflight(acsr_file.name)

        self.assertEqual("", warning)


if __name__ == "__main__":
    unittest.main()
