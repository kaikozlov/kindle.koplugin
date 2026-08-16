import os
import struct
import subprocess
import tempfile
import unittest
import zipfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
AGENT_JAR = ROOT / "bin" / "native-reading-progress-agent-v6.jar"
ATTACH_CLASS = ROOT / "bin" / "classes" / "AttachLauncher.class"
RUNNER = ROOT / "bin" / "sync-native-progress"
BUILD_SCRIPT = ROOT / "scripts" / "build_progress_agent"


class ReadingProgressAgentBundleTests(unittest.TestCase):
    def test_agent_bundle_is_reproducible_from_source(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            subprocess.run(
                [str(BUILD_SCRIPT), tmpdir],
                cwd=ROOT,
                check=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                env={**os.environ, "LC_ALL": "C"},
            )
            rebuilt_jar = Path(tmpdir) / AGENT_JAR.name
            rebuilt_attach = Path(tmpdir) / "classes" / ATTACH_CLASS.name

            self.assertEqual(AGENT_JAR.read_bytes(), rebuilt_jar.read_bytes())
            self.assertEqual(ATTACH_CLASS.read_bytes(), rebuilt_attach.read_bytes())

    def test_agent_targets_java_11_and_manifest_selects_v6(self):
        with zipfile.ZipFile(AGENT_JAR) as bundle:
            manifest = bundle.read("META-INF/MANIFEST.MF").decode("utf-8")
            bytecode = bundle.read("KindlePluginReadingProgressAgentV6.class")

        self.assertIn("Agent-Class: KindlePluginReadingProgressAgentV6", manifest)
        self.assertEqual(b"\xca\xfe\xba\xbe", bytecode[:4])
        self.assertEqual(55, struct.unpack(">H", bytecode[6:8])[0])

    def test_runner_uses_the_rebuilt_v6_agent(self):
        runner = RUNNER.read_text(encoding="utf-8")
        self.assertIn("native-reading-progress-agent-v6.jar", runner)
        self.assertIn("AttachLauncher", runner)


if __name__ == "__main__":
    unittest.main()
