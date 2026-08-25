import os
import shutil
import subprocess
import tempfile
import textwrap
import unittest
import zipfile


REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
JAVA_SOURCE = os.path.join(REPO_ROOT, "lib", "KFXVoucherExtractor.java")
JAVA_JAR = os.path.join(REPO_ROOT, "lib", "KFXVoucherExtractor.jar")
JAVA_STUBS = os.path.join(REPO_ROOT, "lib", "java-stubs")

HARNESS_SOURCE = """
public class VoucherExtractorAccountSecretHarness {
    public static void main(String[] args) throws Exception {
        System.out.print(KFXVoucherExtractor.readAccountSecret(args[0]));
    }
}
"""

ARGUMENTS_HARNESS_SOURCE = """
public class VoucherExtractorArgumentsHarness {
    public static void main(String[] args) {
        KFXVoucherExtractor.Arguments parsed = KFXVoucherExtractor.parseArguments(args);
        if (parsed == null) {
            System.out.println("null");
            return;
        }
        System.out.println(parsed.serial);
        System.out.println(parsed.accountSecretOverride == null ? "-" : parsed.accountSecretOverride);
        System.out.println(String.join("|", parsed.voucherPaths));
    }
}
"""



@unittest.skipUnless(shutil.which("javac") and shutil.which("java"), "Java toolchain is required")
class VoucherExtractorAccountSecretTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tempdir = tempfile.mkdtemp(prefix="voucher-extractor-test-")
        cls.classes_dir = os.path.join(cls.tempdir, "classes")
        os.makedirs(cls.classes_dir)

        harness_path = os.path.join(cls.tempdir, "VoucherExtractorAccountSecretHarness.java")
        arguments_harness_path = os.path.join(cls.tempdir, "VoucherExtractorArgumentsHarness.java")
        with open(harness_path, "w") as harness_file:
            harness_file.write(textwrap.dedent(HARNESS_SOURCE))
        with open(arguments_harness_path, "w") as harness_file:
            harness_file.write(textwrap.dedent(ARGUMENTS_HARNESS_SOURCE))


        stub_sources = []
        for dirpath, _, filenames in os.walk(JAVA_STUBS):
            for filename in filenames:
                if filename.endswith(".java"):
                    stub_sources.append(os.path.join(dirpath, filename))

        subprocess.run(
            [
                "javac",
                "--release",
                "8",
                "-d",
                cls.classes_dir,
                *sorted(stub_sources),
                JAVA_SOURCE,
                harness_path,
                arguments_harness_path,

            ],
            check=True,
            capture_output=True,
            text=True,
        )

        cls.jar_classes_dir = os.path.join(cls.tempdir, "jar-classes")
        os.makedirs(cls.jar_classes_dir)
        subprocess.run(
            [
                "javac",
                "--release",
                "8",
                "-cp",
                JAVA_JAR,
                "-d",
                cls.jar_classes_dir,
                *sorted(stub_sources),
                harness_path,
                arguments_harness_path,

            ],
            check=True,
            capture_output=True,
            text=True,
        )

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.tempdir)

    def read_account_secret(self, path):
        return subprocess.run(
            ["java", "-cp", self.classes_dir, "VoucherExtractorAccountSecretHarness", path],
            check=True,
            capture_output=True,
            text=True,
        )

    def parse_arguments(self, args):
        result = subprocess.run(
            ["java", "-cp", self.classes_dir, "VoucherExtractorArgumentsHarness", *args],
            check=True,
            capture_output=True,
            text=True,
        )
        return result.stdout.splitlines()

    def test_acsr_override_and_vouchers_are_parsed(self):
        lines = self.parse_arguments(["SERIAL", "--acsr", "secret-one", "/v1", "/v2"])
        self.assertEqual(["SERIAL", "secret-one", "/v1|/v2"], lines)

    def test_missing_override_falls_back_to_device_file(self):
        lines = self.parse_arguments(["SERIAL", "/v1"])
        self.assertEqual(["SERIAL", "-", "/v1"], lines)

    def test_flag_position_is_independent(self):
        lines = self.parse_arguments(["SERIAL", "/v0", "--acsr", "s", "/v1"])
        self.assertEqual(["SERIAL", "s", "/v0|/v1"], lines)

    def test_dangling_acsr_flag_is_rejected(self):
        self.assertEqual(["null"], self.parse_arguments(["SERIAL", "--acsr"]))

    def test_bundled_jar_parses_acsr_override(self):
        classpath = os.pathsep.join([JAVA_JAR, self.jar_classes_dir])
        result = subprocess.run(
            [
                "java", "-cp", classpath, "VoucherExtractorArgumentsHarness",
                "SERIAL", "--acsr", "secret-one", "/v1",
            ],
            check=True,
            capture_output=True,
            text=True,
        )
        self.assertEqual(["SERIAL", "secret-one", "/v1"], result.stdout.splitlines())

    def test_bundled_jar_contains_nested_argument_class(self):
        with zipfile.ZipFile(JAVA_JAR) as jar_file:
            names = jar_file.namelist()
        self.assertIn("KFXVoucherExtractor$Arguments.class", names)

    def test_missing_account_secret_warns_and_returns_empty(self):
        path = os.path.join(self.tempdir, "missing-acsr")
        result = self.read_account_secret(path)

        self.assertEqual("", result.stdout)
        self.assertIn("account secret is missing or empty", result.stderr.lower())
        self.assertIn("device serial only", result.stderr.lower())

    def test_empty_account_secret_warns_and_returns_empty(self):
        path = os.path.join(self.tempdir, "empty-acsr")
        open(path, "wb").close()
        result = self.read_account_secret(path)

        self.assertEqual("", result.stdout)
        self.assertIn("account secret is missing or empty", result.stderr.lower())
        self.assertIn("device serial only", result.stderr.lower())

    def test_populated_account_secret_is_trimmed(self):
        path = os.path.join(self.tempdir, "populated-acsr")
        with open(path, "w") as acsr_file:
            acsr_file.write("  account-secret\n")
        result = self.read_account_secret(path)

        self.assertEqual("account-secret", result.stdout)
        self.assertEqual("", result.stderr)

    def test_bundled_jar_matches_source_and_supports_missing_account_secret(self):
        with open(JAVA_SOURCE, "r") as source_file:
            expected_source = source_file.read()
        with zipfile.ZipFile(JAVA_JAR) as jar_file:
            bundled_source = jar_file.read("KFXVoucherExtractor.java").decode("utf-8")
        self.assertEqual(expected_source, bundled_source)

        path = os.path.join(self.tempdir, "missing-acsr-for-jar")
        classpath = os.pathsep.join([JAVA_JAR, self.jar_classes_dir])
        result = subprocess.run(
            ["java", "-cp", classpath, "VoucherExtractorAccountSecretHarness", path],
            check=True,
            capture_output=True,
            text=True,
        )
        self.assertEqual("", result.stdout)
        self.assertIn("device serial only", result.stderr.lower())


if __name__ == "__main__":
    unittest.main()
