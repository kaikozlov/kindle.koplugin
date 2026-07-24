import contextlib
import io
import json
import os
import sys
import tempfile
import unittest
from unittest import mock


PYTHON_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
if PYTHON_DIR not in sys.path:
    sys.path.insert(0, PYTHON_DIR)

from dedrm import drm_init  # noqa: E402


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


class KeyLogCleanupTests(unittest.TestCase):
    def test_key_log_is_removed_after_failure(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            key_log = os.path.join(tmpdir, "crypto_keys.log")
            with mock.patch.object(drm_init, "_KEY_LOG_PATH", key_log):
                with self.assertRaisesRegex(RuntimeError, "extraction failed"):
                    with drm_init._temporary_key_log():
                        with open(key_log, "w") as log_file:
                            log_file.write("sensitive key material")
                        raise RuntimeError("extraction failed")

            self.assertFalse(os.path.exists(key_log))

    def test_key_log_is_removed_after_success(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            key_log = os.path.join(tmpdir, "crypto_keys.log")
            with mock.patch.object(drm_init, "_KEY_LOG_PATH", key_log):
                with drm_init._temporary_key_log():
                    with open(key_log, "w") as log_file:
                        log_file.write("sensitive key material")

            self.assertFalse(os.path.exists(key_log))


class PageKeyValidationTests(unittest.TestCase):
    def test_validation_checks_main_and_sidecar_drmion(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            kfx_path = os.path.join(tmpdir, "book.kfx")
            sidecar_assets = os.path.join(tmpdir, "book.sdr", "assets")
            os.makedirs(sidecar_assets)
            sidecar_path = os.path.join(sidecar_assets, "resource.kfx")
            for path in (kfx_path, sidecar_path):
                with open(path, "wb") as drmion_file:
                    drmion_file.write(drm_init.drmion.DRMION_SIGNATURE + b"content")

            with mock.patch.object(
                drm_init.drmion,
                "decrypt",
                return_value=b"CONT validated",
            ) as decrypt:
                valid, error = drm_init._validate_page_key(kfx_path, b"k" * 16)

            self.assertTrue(valid)
            self.assertIsNone(error)
            self.assertEqual(2, decrypt.call_count)

    def test_validation_rejects_a_key_that_cannot_decrypt_content(self):
        with tempfile.NamedTemporaryFile(suffix=".kfx") as kfx_file:
            kfx_file.write(drm_init.drmion.DRMION_SIGNATURE + b"content")
            kfx_file.flush()
            with mock.patch.object(
                drm_init.drmion,
                "decrypt",
                side_effect=ValueError("bad padding"),
            ):
                valid, error = drm_init._validate_page_key(kfx_file.name, b"x" * 16)

        self.assertFalse(valid)
        self.assertIn("bad padding", error)

    def test_per_book_extraction_does_not_cache_rejected_key(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            kfx_path = os.path.join(tmpdir, "book.kfx")
            voucher_path = os.path.join(tmpdir, "book.sdr", "assets", "voucher")
            with mock.patch.object(drm_init, "_find_voucher_for_kfx", return_value=voucher_path), \
                    mock.patch.object(drm_init, "_read_device_serial", return_value="SERIAL"), \
                    mock.patch.object(drm_init, "_extract_keys_with_hook"), \
                    mock.patch.object(drm_init, "_parse_captured_keys", return_value=[{"key": b"v" * 32}]), \
                    mock.patch.object(drm_init, "_find_voucher_key", return_value=b"v" * 32), \
                    mock.patch.object(drm_init, "_extract_page_key", return_value=b"p" * 16), \
                    mock.patch.object(
                        drm_init,
                        "_validate_page_key",
                        return_value=(False, "page key rejected"),
                    ):
                result = drm_init.extract_book_key(kfx_path, tmpdir, tmpdir)

            self.assertFalse(result["ok"])
            self.assertIn("rejected", result["message"])
            self.assertFalse(os.path.exists(os.path.join(tmpdir, "drm_keys.json")))


class EncryptionKeyCacheTests(unittest.TestCase):
    def test_store_indexes_page_key_by_drm_identifier(self):
        cache = drm_init._new_key_cache("SERIAL")
        with mock.patch.object(
            drm_init,
            "_encryption_key_ids_for_book",
            return_value=["key-id-one", "key-id-two"],
        ):
            key_ids = drm_init._store_page_key(
                cache,
                "BOOK",
                "/book.sdr/assets/voucher",
                b"v" * 32,
                b"p" * 16,
                "/book.kfx",
            )

        self.assertEqual(["key-id-one", "key-id-two"], key_ids)
        self.assertEqual("70" * 16, cache["keys"]["key-id-one"]["page_key_128"])
        self.assertEqual(key_ids, cache["books"]["BOOK"]["encryption_key_ids"])
        self.assertEqual(2, cache["version"])

    def test_upgrade_preserves_legacy_book_entries(self):
        legacy_entry = {"page_key_128": "aa" * 16}
        cache = drm_init._upgrade_key_cache({
            "version": 1,
            "books": {"BOOK": legacy_entry},
        }, "SERIAL")

        self.assertEqual(2, cache["version"])
        self.assertEqual(legacy_entry, cache["books"]["BOOK"])
        self.assertEqual({}, cache["keys"])


class NativeFallbackTests(unittest.TestCase):
    def test_native_fallback_caches_validated_page_key(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            kfx_path = os.path.join(tmpdir, "book.kfx")
            voucher_path = os.path.join(tmpdir, "book.sdr", "assets", "voucher")
            with mock.patch.object(
                drm_init.native_extractor,
                "extract_page_keys",
                return_value={"key-id": b"p" * 16},
            ), mock.patch.object(
                drm_init,
                "_select_native_page_key",
                return_value=b"p" * 16,
            ), mock.patch.object(
                drm_init,
                "_encryption_key_ids_for_book",
                return_value=["key-id"],
            ):
                result = drm_init._native_book_fallback(
                    kfx_path,
                    voucher_path,
                    tmpdir,
                    tmpdir,
                    "SERIAL",
                    "cvm failed",
                )

            self.assertTrue(result["ok"])
            self.assertEqual("native", result["extractor"])
            with open(os.path.join(tmpdir, "drm_keys.json")) as cache_file:
                cache = json.load(cache_file)
            self.assertEqual("70" * 16, cache["keys"]["key-id"]["page_key_128"])
            self.assertEqual("", cache["books"][result["book_id"]]["voucher_key_256"])

    def test_bulk_native_fallback_writes_matching_keys(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            voucher_path = os.path.join(tmpdir, "Book_B001234567.sdr", "assets", "voucher")
            kfx_path = os.path.join(tmpdir, "Book_B001234567.kfx")
            with mock.patch.object(
                drm_init.native_extractor,
                "extract_page_keys",
                return_value={"key-id": b"p" * 16},
            ), mock.patch.object(
                drm_init,
                "_find_kfx_for_voucher",
                return_value=kfx_path,
            ), mock.patch.object(
                drm_init,
                "_select_native_page_key",
                return_value=b"p" * 16,
            ), mock.patch.object(
                drm_init,
                "_encryption_key_ids_for_book",
                return_value=["key-id"],
            ):
                result = drm_init._run_native_fallback(
                    [voucher_path], tmpdir, tmpdir, "SERIAL", "cvm failed"
                )

            self.assertEqual(1, result["keys_found"])
            self.assertEqual("native", result["extractor"])
            self.assertTrue(os.path.isfile(os.path.join(tmpdir, "drm_keys.json")))

    def test_primary_extraction_failure_invokes_native_fallback(self):
        native_result = {"ok": True, "book_id": "BOOK", "extractor": "native"}
        with mock.patch.object(drm_init, "_preflight_check"), \
                mock.patch.object(
                    drm_init,
                    "_find_voucher_for_kfx",
                    return_value="/book.sdr/assets/voucher",
                ), mock.patch.object(drm_init, "_read_device_serial", return_value="SERIAL"), \
                mock.patch.object(
                    drm_init,
                    "_extract_keys_with_hook",
                    side_effect=RuntimeError("cvm failed"),
                ), mock.patch.object(
                    drm_init,
                    "_native_book_fallback",
                    return_value=native_result,
                ) as fallback:
            result = drm_init.extract_book_key("/book.kfx", "/plugin", "/cache")

        self.assertEqual(native_result, result)
        fallback.assert_called_once()


class DeviceSerialTests(unittest.TestCase):
    def test_serial_removes_firmware_artifacts(self):
        serial_file = mock.mock_open(read_data="  G090G10512345678\r\n\x00é")
        with mock.patch("builtins.open", serial_file):
            serial = drm_init._read_device_serial()

        self.assertEqual("G090G10512345678", serial)

    def test_invalid_serial_is_rejected(self):
        serial_file = mock.mock_open(read_data="\x00\r\n ")
        with mock.patch("builtins.open", serial_file):
            with self.assertRaisesRegex(RuntimeError, "empty or invalid"):
                drm_init._read_device_serial()


if __name__ == "__main__":
    unittest.main()
