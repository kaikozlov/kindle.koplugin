import struct
import unittest
import zipfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
AGENT_JAR = ROOT / "bin" / "native-reading-progress-agent-v6.jar"
AGENT_SOURCE = ROOT / "agent" / "src" / "KindlePluginReadingProgressAgentV6.java"
RUNNER = ROOT / "bin" / "sync-native-progress"


class ReadingProgressAgentBundleTests(unittest.TestCase):
    def test_runner_and_manifest_select_v6_agent(self):
        self.assertIn(
            'native-reading-progress-agent-v6.jar',
            RUNNER.read_text(encoding="utf-8"),
        )
        with zipfile.ZipFile(AGENT_JAR) as bundle:
            manifest = bundle.read("META-INF/MANIFEST.MF").decode("utf-8")
        self.assertIn("Agent-Class: KindlePluginReadingProgressAgentV6", manifest)

    def test_bundled_agent_targets_java_11_and_contains_durability_checks(self):
        with zipfile.ZipFile(AGENT_JAR) as bundle:
            bytecode = bundle.read("KindlePluginReadingProgressAgentV6.class")
        self.assertEqual(b"\xca\xfe\xba\xbe", bytecode[:4])
        self.assertEqual(55, struct.unpack(">H", bytecode[6:8])[0])
        for marker in (
            b"LprSidecarAdapter",
            b"verify_local_lpr",
            b"local_progress_verified",
            b"local LPR durability check failed",
            b"native_percent",
            b"native rendered percentage unavailable",
            b"catalog_progress_saved",
            b"ContentCatalogLprUtils",
        ):
            self.assertIn(marker, bytecode)

    def test_source_reinserts_lpr_before_saving(self):
        source = AGENT_SOURCE.read_text(encoding="utf-8")
        stage = source.index("adapter.a(lpr)")
        save = source.index("book.Ue()")
        close = source.index("book.close()", save)
        reopen = source.index("content.dt(nativePath)", close)
        verify = source.index("local_progress_verified=true", reopen)
        self.assertLess(stage, save)
        self.assertLess(save, close)
        self.assertLess(close, reopen)
        self.assertLess(reopen, verify)

    def test_native_percentage_uses_kindles_rendered_progress_fraction(self):
        source = AGENT_SOURCE.read_text(encoding="utf-8")
        self.assertIn("position.UG() * 100.0", source)
        self.assertNotIn("position.nR() * 100.0", source)

    def test_native_catalog_transaction_publishes_the_shelf_percentage(self):
        source = AGENT_SOURCE.read_text(encoding="utf-8")
        self.assertIn('item.setProperty("percentFinished"', source)
        self.assertIn("ContentCatalogLprUtils.a(sdk, catalog, item)", source)
        self.assertIn('catalog_progress_saved=" + catalogSaved', source)


if __name__ == "__main__":
    unittest.main()
