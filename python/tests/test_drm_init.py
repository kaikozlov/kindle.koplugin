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
                drm_init._preflight_check(drm_init._read_account_secrets())
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


class AccountSecretSplittingTests(unittest.TestCase):
    def read_secrets(self, content=None):
        if content is None:
            with tempfile.TemporaryDirectory() as tmpdir:
                missing = os.path.join(tmpdir, "missing-acsr")
                with mock.patch.object(drm_init, "_ACSR_PATH", missing):
                    return drm_init._read_account_secrets()
        with tempfile.NamedTemporaryFile(mode="w") as acsr_file:
            acsr_file.write(content)
            acsr_file.flush()
            with mock.patch.object(drm_init, "_ACSR_PATH", acsr_file.name):
                return drm_init._read_account_secrets()
    def test_comma_separated_secrets_are_split(self):
        self.assertEqual(
            ["secret-one", "secret-two", "secret-three"],
            self.read_secrets("secret-one, secret-two,secret-three\n"),
        )


    def test_empty_parts_are_dropped(self):
        self.assertEqual(
            ["secret-one"], self.read_secrets(", ,secret-one,,")
        )

    def test_missing_file_yields_empty_list(self):
        self.assertEqual([], self.read_secrets())


class MultiSecretHookIterationTests(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmpdir.cleanup)
        self.key_log = os.path.join(self.tmpdir.name, "crypto_keys.log")
        patcher = mock.patch.object(drm_init, "_KEY_LOG_PATH", self.key_log)
        patcher.start()
        self.addCleanup(patcher.stop)

    def run_hook(self, secrets, probe=None, results=None):
        if results is None:
            results = [mock.Mock(returncode=0, stdout="All vouchers attached", stderr="")]
        patches = [
            mock.patch.object(drm_init.os.path, "isfile", return_value=True),
            mock.patch.object(
                drm_init.subprocess, "run", side_effect=results
            ),
        ]
        for patch in patches:
            patch.start()
            self.addCleanup(patch.stop)
        drm_init._extract_keys_with_hook(
            "SERIAL", ["/book.sdr/assets/voucher"], "/plugin", secrets, probe=probe
        )
        return drm_init.subprocess.run.call_args_list

    def cvm_secrets(self, calls):
        secrets = []
        for call in calls:
            cmd = call.args[0]
            self.assertEqual("KFXVoucherExtractor", cmd[4])
            self.assertEqual("SERIAL", cmd[5])
            if "--acsr" in cmd:
                secrets.append(cmd[cmd.index("--acsr") + 1])
            else:
                secrets.append(None)
        return secrets

    def test_single_secret_runs_once_with_override(self):
        calls = self.run_hook(["secret-one"])
        self.assertEqual(["secret-one"], self.cvm_secrets(calls))

    def test_missing_acsr_runs_once_without_override(self):
        calls = self.run_hook([])
        self.assertEqual([None], self.cvm_secrets(calls))

    def test_probe_stops_iteration_once_satisfied(self):
        probe_state = {"calls": 0}

        def probe():
            probe_state["calls"] += 1
            return probe_state["calls"] >= 2
        calls = self.run_hook(
            ["secret-one", "secret-two"],
            probe=probe,
            results=[
                mock.Mock(returncode=0, stdout="All vouchers attached", stderr=""),
                mock.Mock(returncode=0, stdout="All vouchers attached", stderr=""),
            ],
        )
        # Probe returns False after run one, True after run two: both ran.
        self.assertEqual(["secret-one", "secret-two"], self.cvm_secrets(calls))


    def test_probe_short_circuits_remaining_secrets(self):
        calls = self.run_hook(
            ["secret-one", "secret-two", "secret-three"],
            probe=lambda: True,
        )
        self.assertEqual(["secret-one"], self.cvm_secrets(calls))

    def test_failed_run_falls_through_to_next_secret(self):
        results = [
            mock.Mock(returncode=1, stdout="", stderr="boom"),
            mock.Mock(returncode=0, stdout="All vouchers attached", stderr=""),
        ]
        calls = self.run_hook(["secret-one", "secret-two"], results=results)
        self.assertEqual(["secret-one", "secret-two"], self.cvm_secrets(calls))

    def test_all_runs_failing_raises_last_error(self):
        results = [
            mock.Mock(returncode=1, stdout="", stderr="boom-one"),
            mock.Mock(returncode=1, stdout="", stderr="boom-two"),
        ]
        with self.assertRaisesRegex(RuntimeError, "boom-two"):
            self.run_hook(["secret-one", "secret-two"], results=results)

    def test_unregistered_device_error_raises_immediately(self):
        results = [
            mock.Mock(
                returncode=1,
                stdout="",
                stderr="java.nio.file.NoSuchFileException: /var/local/java/prefs/acsr",
            ),
            mock.Mock(returncode=0, stdout="All vouchers attached", stderr=""),
        ]
        with mock.patch.object(drm_init.os.path, "isfile", return_value=True), \
                mock.patch.object(
                    drm_init.subprocess, "run", side_effect=results
                ) as run:
            with self.assertRaisesRegex(RuntimeError, "not registered"):
                drm_init._extract_keys_with_hook(
                    "SERIAL", ["/book.sdr/assets/voucher"], "/plugin",
                    ["secret-one", "secret-two"],
                )
        self.assertEqual(1, run.call_count)
class VoucherKeyMatchingTests(unittest.TestCase):
    def test_lone_candidate_is_still_trial_decrypted(self):
        """A single captured key may come from a wrong account secret."""
        with tempfile.NamedTemporaryFile() as voucher_file:
            with mock.patch.object(
                drm_init,
                "_extract_page_key_from_data",
                side_effect=ValueError("bad padding"),
            ):
                matched = drm_init._find_voucher_key(
                    voucher_file.name, [{"key": b"v" * 32, "iv": None}]
                )

        self.assertIsNone(matched)

    def test_matching_key_is_returned(self):
        with tempfile.NamedTemporaryFile() as voucher_file:
            with mock.patch.object(
                drm_init,
                "_extract_page_key_from_data",
                side_effect=[ValueError("bad padding"), None],
            ):
                matched = drm_init._find_voucher_key(
                    voucher_file.name,
                    [
                        {"key": b"w" * 32, "iv": None},
                        {"key": b"v" * 32, "iv": None},
                    ],
                )

        self.assertEqual(b"v" * 32, matched)


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
            self.assertEqual("drm_key_extraction_failed", result["code"])
            self.assertIn("rejected", result["detail"])
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

    def test_missing_cvm_and_native_extractor_returns_actionable_code(self):
        primary_error = FileNotFoundError(2, "No such file or directory", "/usr/java/bin/cvm")
        with tempfile.TemporaryDirectory() as tmpdir, mock.patch.object(
            drm_init.native_extractor,
            "extract_page_keys",
            side_effect=drm_init.native_extractor.NativeExtractorUnavailable(
                "no native extractor binaries found"
            ),
        ):
            result = drm_init._native_book_fallback(
                "/book.kfx",
                "/book.sdr/assets/voucher",
                tmpdir,
                tmpdir,
                "SERIAL",
                primary_error,
            )

        self.assertFalse(result["ok"])
        self.assertEqual("drm_extractor_unavailable", result["code"])
        self.assertIn("does not provide the Java DRM runtime", result["message"])
        self.assertIn("/usr/java/bin/cvm", result["detail"])
        self.assertIn("no native extractor binaries found", result["detail"])

    def test_other_native_fallback_failure_is_sanitized(self):
        with tempfile.TemporaryDirectory() as tmpdir, mock.patch.object(
            drm_init.native_extractor,
            "extract_page_keys",
            side_effect=RuntimeError("native secret diagnostic"),
        ):
            result = drm_init._native_book_fallback(
                "/book.kfx",
                "/book.sdr/assets/voucher",
                tmpdir,
                tmpdir,
                "SERIAL",
                RuntimeError("java secret diagnostic"),
            )

        self.assertFalse(result["ok"])
        self.assertEqual("drm_key_extraction_failed", result["code"])
        self.assertEqual("Book access key extraction failed.", result["message"])
        self.assertNotIn("secret diagnostic", result["message"])
        self.assertIn("java secret diagnostic", result["detail"])
        self.assertIn("native secret diagnostic", result["detail"])

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


class ParseCapturedKeysTests(unittest.TestCase):
    """Direct coverage of the crypto_hook.so wire format consumer.

    The exact log line asserted here is the one produced by the ARMv7 harness
    in .github/Dockerfile.crypto_hook (key 00..1f, IV a0..af), so the C hook,
    its wire format, and this parser are kept in lockstep without mocks.
    """

    HOOK_LINE = (
        "EVP_256_KEY:000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f "
        "IV:a0a1a2a3a4a5a6a7a8a9aaabacadaeaf"
    )

    def parse_log(self, *lines):
        with tempfile.NamedTemporaryFile("w", suffix=".log", delete=False) as log_file:
            log_file.write("\n".join(lines) + "\n")
            path = log_file.name
        try:
            return drm_init._parse_captured_keys(path)
        finally:
            os.unlink(path)

    def test_exact_hook_line_is_decoded(self):
        keys = self.parse_log(self.HOOK_LINE)

        self.assertEqual(
            [{"key": bytes(range(32)), "iv": bytes(0xA0 + i for i in range(16))}],
            keys,
        )

    def test_malformed_lines_are_ignored(self):
        keys = self.parse_log(
            "garbage without any format",
            "EVP_128_KEY:000102030405060708090a0b0c0d0e0f IV:a0a1a2a3a4a5a6a7a8a9aaabacadaeaf",
            "EVP_256_KEY:zz02030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f IV:a0a1a2a3a4a5a6a7a8a9aaabacadaeaf",
            "EVP_256_KEY:000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f IV:none",
            "AES_DEC_256_KEY:000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
        )
        self.assertEqual([], keys)

    def test_multiple_valid_lines_are_kept_in_order(self):
        second = (
            "EVP_256_KEY:ff0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f "
            "IV:00000000000000000000000000000000"
        )
        keys = self.parse_log(self.HOOK_LINE, second)

        self.assertEqual(2, len(keys))
        self.assertEqual(bytes([0xFF]) + bytes(range(1, 32)), keys[1]["key"])
        self.assertEqual(bytes(16), keys[1]["iv"])

    def test_shared_metadata_key_is_rejected(self):
        keys = self.parse_log(
            "EVP_256_KEY:6533356635"
            "000102030405060708090a0b0c0d0e0f101112131415161718"
            " IV:a0a1a2a3a4a5a6a7a8a9aaabacadaeaf"
        )

        self.assertEqual([], keys)

    def test_missing_log_yields_no_keys(self):
        self.assertEqual([], drm_init._parse_captured_keys("/nonexistent/crypto_keys.log"))


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
