"""Tests for scripts/audit_parity.py — the honest parity auditor.

These tests pin the anti-cheat behavior:
  - a name-only Go stub must NEVER count as implemented
  - delegation wrappers count only with real transitive substance
  - exclusions must be explicit, categorized, reasoned, and (for
    architecture claims) backed by substantive Go evidence
  - metrics must add up and never hide excluded functions

Run:  python3 -m unittest discover -s scripts/tests -v
"""

import ast
import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
import audit_parity as ap  # noqa: E402

REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


def mkpy(src: str) -> list:
    """Extract PyFunc list from Python source text."""
    with tempfile.NamedTemporaryFile("w", suffix=".py", delete=False) as f:
        f.write(src)
        path = f.name
    try:
        return ap.extract_python_functions(path)
    finally:
        os.unlink(path)


def mkgo(name, file="fake.go", nstmt=10, nlit=0, empty=False, const_only=False,
         error_only=False, notimpl=False, calls=None, called_by=1, line=1,
         trivial_shape="", ident_calls=None):
    return {
        "file": file, "name": name, "line": line, "end_line": line + nstmt,
        "nstmt": nstmt, "nlit": nlit, "nchars": 100, "empty": empty,
        "const_only": const_only, "error_only": error_only, "notimpl": notimpl,
        "trivial_shape": trivial_shape, "nparams": 0,
        "calls": calls or [], "ident_calls": ident_calls if ident_calls is not None else (calls or []),
        "self_calls": 0, "called_by": called_by,
    }


def index(*funcs):
    # Exact-case index, matching audit_parity's gofuncinfo indexing.
    by_name = {}
    for f in funcs:
        by_name.setdefault(f["name"], []).append(f)
    return {"functions": list(funcs), "by_name": by_name, "call_counts": {}}


class TestClassify(unittest.TestCase):
    def pf(self, name="do_work", nstmt=50, nlit=0, cls=None, shape=""):
        return ap.PyFunc(name=name, class_name=cls, line_start=1, line_end=99,
                         args="", docstring_first_line=None, nstmt=nstmt, nlit=nlit,
                         trivial_shape=shape)

    def test_substantive_match_is_implemented(self):
        go = mkgo("doWork", nstmt=45)
        self.assertEqual(ap.classify(self.pf(), go, None), "implemented")

    def test_const_only_stub_is_silent_stub(self):
        go = mkgo("doWork", nstmt=1, const_only=True, trivial_shape="const:nil")
        self.assertEqual(ap.classify(self.pf(shape=""), go, None), "stub_silent")

    def test_empty_stub_is_silent_stub(self):
        go = mkgo("doWork", empty=True, nstmt=0, trivial_shape="void")
        self.assertEqual(ap.classify(self.pf(shape=""), go, None), "stub_silent")

    def test_error_only_stub_is_silent_stub(self):
        go = mkgo("doWork", nstmt=1, error_only=True)
        self.assertEqual(ap.classify(self.pf(shape=""), go, None), "stub_silent")

    def test_notimpl_is_admitted_stub(self):
        go = mkgo("doWork", nstmt=1, notimpl=True)
        self.assertEqual(ap.classify(self.pf(shape=""), go, None), "stub_admitted")

    def test_trivial_py_trivial_go_is_implemented_trivial(self):
        go = mkgo("noop", nstmt=1, const_only=True, trivial_shape="const:nil")
        self.assertEqual(ap.classify(self.pf("noop", shape="const:nil"), go, None),
                         "implemented_trivial")

    def test_py_true_vs_go_false_not_trivial(self):
        go = mkgo("flag", nstmt=1, const_only=True, trivial_shape="const:false")
        self.assertEqual(ap.classify(self.pf("flag", shape="const:true"), go, None),
                         "stub_silent")

    def test_py_zero_vs_go_one_not_trivial(self):
        go = mkgo("count", nstmt=1, const_only=True, trivial_shape="const:int:1")
        self.assertEqual(ap.classify(self.pf("count", shape="const:int:0"), go, None),
                         "stub_silent")

    def test_py_int_matches_go_equal_int(self):
        go = mkgo("count", nstmt=1, const_only=True, trivial_shape="const:int:3")
        self.assertEqual(ap.classify(self.pf("count", shape="const:int:3"), go, None),
                         "implemented_trivial")

    def test_py_int_vs_go_float_same_value_matches(self):
        go = mkgo("count", nstmt=1, const_only=True, trivial_shape="const:float:2.0")
        self.assertEqual(ap.classify(self.pf("count", shape="const:int:2"), go, None),
                         "implemented_trivial")

    def test_py_string_vs_go_string_value_equality(self):
        go = mkgo("name", nstmt=1, const_only=True, trivial_shape='const:string:"nav"')
        self.assertEqual(ap.classify(self.pf("name", shape='const:string:nav'), go, None),
                         "implemented_trivial")
        go2 = mkgo("name", nstmt=1, const_only=True, trivial_shape='const:string:"toc"')
        self.assertEqual(ap.classify(self.pf("name", shape='const:string:nav'), go2, None),
                         "stub_silent")

    def test_py_identity_vs_go_constant_not_trivial(self):
        go = mkgo("passThrough", nstmt=1, const_only=True, trivial_shape="const:nil")
        self.assertEqual(ap.classify(self.pf("pass_through", shape="arg:0"), go, None),
                         "stub_silent")

    def test_py_identity_vs_go_same_position_matches(self):
        go = mkgo("passThrough", nstmt=1, const_only=True, trivial_shape="arg:0")
        self.assertEqual(ap.classify(self.pf("pass_through", shape="arg:0"), go, None),
                         "implemented_trivial")

    def test_py_identity_wrong_position_not_trivial(self):
        go = mkgo("second", nstmt=1, const_only=True, trivial_shape="arg:1")
        self.assertEqual(ap.classify(self.pf("first", shape="arg:0"), go, None),
                         "stub_silent")

    def test_py_void_vs_go_void(self):
        go = mkgo("noop", nstmt=0, empty=True, trivial_shape="void")
        self.assertEqual(ap.classify(self.pf("noop", shape="void"), go, None),
                         "implemented_trivial")

    def test_thin_body_without_delegate_is_thin(self):
        go = mkgo("doWork", nstmt=5)
        self.assertEqual(ap.classify(self.pf(), go, None), "thin")

    def test_literal_heavy_constructor_is_not_thin(self):
        # Python __init__ assigns 40 fields; Go returns one 40-field struct.
        go = mkgo("newThing", nstmt=1, nlit=40)
        py = self.pf("__init__", nstmt=40, cls="Thing")
        self.assertEqual(ap.classify(py, go, None), "implemented")

    def test_exclusion_wins(self):
        go = mkgo("doWork", nstmt=1, const_only=True)
        self.assertEqual(ap.classify(self.pf(), go, {"category": "x"}),
                         "excluded")

    def test_missing(self):
        self.assertEqual(ap.classify(self.pf(), None, None), "missing")


class TestNameMatching(unittest.TestCase):
    def test_dunder_init_does_not_match_go_init(self):
        pf = ap.PyFunc(name="__init__", class_name="KFX_EPUB", line_start=1,
                       line_end=9, args="", docstring_first_line=None)
        names = ap.expected_go_names(pf)
        lowered = [n.lower() for n in names]
        self.assertNotIn("init", lowered,
                         "camel('__init__') must not match Go init()")
        # constructor aliases for KFX_EPUB must be present (Go-style casing)
        self.assertTrue(any("kfxepub" in n.lower() for n in names), names)

    def test_dunder_repr_maps_to_string(self):
        pf = ap.PyFunc(name="__repr__", class_name=None, line_start=1,
                       line_end=9, args="", docstring_first_line=None)
        self.assertIn("String", ap.expected_go_names(pf))

    def test_snake_to_camel(self):
        pf = ap.PyFunc(name="process_content", class_name="KFX_EPUB_Content",
                       line_start=1, line_end=9, args="", docstring_first_line=None)
        names = ap.expected_go_names(pf)
        self.assertIn("processContent", names)
        self.assertIn("ProcessContent", names)


class TestPythonSubstance(unittest.TestCase):
    def test_dict_literal_counts(self):
        funcs = mkpy("""
TABLE = {"a": 1, "b": 2, "c": 3, "d": 4}

def get_table():
    return {"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
""")
        f = [x for x in funcs if x.name == "get_table"][0]
        self.assertEqual(f.nlit, 5)
        self.assertEqual(f.substance, 1 + 5)

    def test_docstring_not_counted(self):
        funcs = mkpy('''
def documented():
    """This docstring is not a statement.

    More words.
    """
    return 1
''')
        f = [x for x in funcs if x.name == "documented"][0]
        self.assertEqual(f.nstmt, 1)

    def test_nested_functions_excluded_from_parent(self):
        funcs = mkpy("""
def parent():
    def child():
        x = 1
        y = 2
        return x + y
    return child()
""")
        f = [x for x in funcs if x.name == "parent"][0]
        self.assertEqual(f.nstmt, 1)  # only `return child()`


class TestDelegation(unittest.TestCase):
    def test_wrapper_delegating_to_substance_is_implemented(self):
        real = mkgo("doWorkFull", nstmt=60, calls=[])
        wrapper = mkgo("doWork", nstmt=1, calls=["doWorkFull"])
        idx = index(real, wrapper)
        pf = ap.PyFunc(name="do_work", class_name=None, line_start=1, line_end=99,
                       args="", docstring_first_line=None, nstmt=50, nlit=0)
        self.assertEqual(ap.classify(pf, wrapper, None,
                                     tsub=ap.transitive_substance(idx, wrapper)),
                         "implemented_delegation")

    def test_wrapper_delegating_to_library_only_stays_thin(self):
        # Sprintf is external: resolves to nothing in the index.
        wrapper = mkgo("doWork", nstmt=1, calls=["Sprintf"])
        idx = index(wrapper)
        pf = ap.PyFunc(name="do_work", class_name=None, line_start=1, line_end=99,
                       args="", docstring_first_line=None, nstmt=50, nlit=0)
        self.assertEqual(ap.classify(pf, wrapper, None,
                                     tsub=ap.transitive_substance(idx, wrapper)),
                         "thin")

    def test_stub_chain_cannot_credit_itself(self):
        a = mkgo("stubA", nstmt=1, const_only=True, calls=[])
        b = mkgo("stubB", nstmt=1, const_only=True, calls=["stubA"])
        idx = index(a, b)
        self.assertEqual(ap.transitive_substance(idx, b), 2)  # 1 + 1, no inflation

    def test_cycle_safe(self):
        a = mkgo("mutualA", nstmt=5, calls=["mutualB"])
        b = mkgo("mutualB", nstmt=5, calls=["mutualA"])
        idx = index(a, b)
        self.assertEqual(ap.transitive_substance(idx, a), 10)


class TestExclusions(unittest.TestCase):
    def setUp(self):
        self.pyfuncs = {"fake.py": mkpy("""
class Thing:
    def replaced_by_lib(self):
        for i in range(10):
            print(i)

    def out_of_scope(self):
        return 1
""")}

    def good_entry(self, **over):
        e = {
            "py_file": "fake.py", "py_class": "Thing",
            "py_name": "replaced_by_lib",
            "category": "library-replacement",
            "reason": "provided by amazon-ion-go decode path",
            "evidence": [{"go_file": "real.go", "go_func": "realImpl"}],
        }
        e.update(over)
        return e

    def test_valid_entry_passes(self):
        go_index = index(mkgo("realImpl", file="real.go", nstmt=50))
        problems, valid_entries = ap.validate_exclusions([self.good_entry()], self.pyfuncs, go_index)
        self.assertEqual(problems, [])

    def test_unknown_category_rejected(self):
        go_index = index(mkgo("realImpl", file="real.go", nstmt=50))
        problems, _valid = ap.validate_exclusions(
            [self.good_entry(category="because-i-said-so")], self.pyfuncs, go_index)
        self.assertTrue(any("category" in p for p in problems))

    def test_short_reason_rejected(self):
        go_index = index(mkgo("realImpl", file="real.go", nstmt=50))
        problems, _valid = ap.validate_exclusions(
            [self.good_entry(reason="skip")], self.pyfuncs, go_index)
        self.assertTrue(any("reason" in p for p in problems))

    def test_missing_evidence_rejected_for_architecture_categories(self):
        go_index = index(mkgo("realImpl", file="real.go", nstmt=50))
        problems, _valid = ap.validate_exclusions(
            [self.good_entry(evidence=[])], self.pyfuncs, go_index)
        self.assertTrue(any("requires evidence" in p for p in problems))

    def test_trivial_evidence_rejected(self):
        go_index = index(mkgo("realImpl", file="real.go", const_only=True))
        problems, valid_entries = ap.validate_exclusions([self.good_entry()], self.pyfuncs, go_index)
        self.assertTrue(any("itself trivial" in p for p in problems))

    def test_nonexistent_evidence_rejected(self):
        go_index = index(mkgo("realImpl", file="real.go", nstmt=50))
        problems, _valid = ap.validate_exclusions(
            [self.good_entry(evidence=[{"go_file": "real.go",
                                        "go_func": "nope"}])],
            self.pyfuncs, go_index)
        self.assertTrue(any("no Go function" in p for p in problems))

    def test_stale_exclusion_rejected(self):
        go_index = index(mkgo("realImpl", file="real.go", nstmt=50))
        problems, _valid = ap.validate_exclusions(
            [self.good_entry(py_name="does_not_exist")], self.pyfuncs, go_index)
        self.assertTrue(any("no audited Python function" in p for p in problems))

    def test_out_of_scope_category_does_not_need_evidence(self):
        e = self.good_entry(py_name="out_of_scope", category="output-mode-out-of-scope",
                            reason="Calibre output mode unused by KOReader",
                            evidence=[])
        problems, _valid = ap.validate_exclusions([e], self.pyfuncs, index())
        self.assertEqual(problems, [])


class TestDuplicateDefIdentity(unittest.TestCase):
    """A name defined twice (getter + setter) must not be waivable by one
    exclusion; the entry must pin py_line and apply to exactly one def."""

    PY_SRC = '''
class OPFProps:
    @property
    def is_fxl(self):
        return "rendition:layout-pre-paginated" in self.props

    @is_fxl.setter
    def is_fxl(self, value):
        if value:
            self.props.add("rendition:layout-pre-paginated")
        else:
            self.props.discard("rendition:layout-pre-paginated")

    def single_def(self):
        return 1
'''

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.py_file = "dup.py"
        path = os.path.join(self.tmp.name, self.py_file)
        with open(path, "w") as f:
            f.write(self.PY_SRC)
        self.funcs = ap.extract_python_functions(path)
        self.by_name = {}
        for pf in self.funcs:
            self.by_name.setdefault(pf.name, []).append(pf)
        self.go_index = index(mkgo("realImpl", file="real.go", nstmt=50))
        self.pyfuncs = {self.py_file: self.funcs}

    def tearDown(self):
        self.tmp.cleanup()

    def entry(self, **over):
        e = {"py_file": self.py_file, "py_class": "OPFProps", "py_name": "is_fxl",
             "category": "alternate-architecture",
             "reason": "property state is a struct field in Go",
             "evidence": [{"go_file": "real.go", "go_func": "realImpl"}]}
        e.update(over)
        return e

    def test_duplicate_name_without_py_line_rejected(self):
        problems, valid = ap.validate_exclusions([self.entry()], self.pyfuncs, self.go_index)
        self.assertTrue(any("cannot silently waive multiple defs" in p for p in problems))
        self.assertEqual(valid, [])

    def test_pinned_py_line_validates(self):
        getter_line = self.by_name["is_fxl"][0].line_start
        problems, valid = ap.validate_exclusions([self.entry(py_line=getter_line)],
                                                 self.pyfuncs, self.go_index)
        self.assertEqual(problems, [])
        self.assertEqual(len(valid), 1)

    def test_wrong_py_line_rejected(self):
        # decorator line or off-by-one must not pass
        problems, _ = ap.validate_exclusions([self.entry(py_line=1)],
                                             self.pyfuncs, self.go_index)
        self.assertTrue(any("does not match any def" in p for p in problems))

    def test_unique_name_needs_no_py_line(self):
        e = self.entry(py_name="single_def")
        problems, valid = ap.validate_exclusions([e], self.pyfuncs, self.go_index)
        self.assertEqual(problems, [])
        self.assertEqual(len(valid), 1)

    def test_pinned_exclusion_waives_exactly_one_def(self):
        # Audit-level: pinned entry must exclude the getter but leave the
        # setter classified (here: missing, since no Go match for a setter).
        getter = self.by_name["is_fxl"][0]
        setter = self.by_name["is_fxl"][1]
        pinned = self.entry(py_line=getter.line_start)
        self.assertTrue(ap.exclusion_matches(pinned, getter, self.py_file))
        self.assertFalse(ap.exclusion_matches(pinned, setter, self.py_file))

    def test_unpinned_exclusion_would_waive_both_defs(self):
        # Documents the pre-fix behavior the validator now forbids.
        unpinned = self.entry()
        getter = self.by_name["is_fxl"][0]
        setter = self.by_name["is_fxl"][1]
        self.assertTrue(ap.exclusion_matches(unpinned, getter, self.py_file))
        self.assertTrue(ap.exclusion_matches(unpinned, setter, self.py_file))


class TestFileModeExclusionValidation(unittest.TestCase):
    """--file must apply the same exclusion validation as the full audit."""

    def test_file_mode_reports_and_ignores_invalid_exclusions(self):
        import shutil
        import subprocess
        repo = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
        py_dir = os.path.join(repo, "REFERENCE/KFX_Input/kfxlib")
        if not os.path.isdir(py_dir):
            self.skipTest("REFERENCE/KFX_Input/kfxlib not present")
        if not shutil.which("go"):
            self.skipTest("go toolchain not available")
        bad = {"exclusions": [{
            "py_file": "epub_output.py", "py_class": "EPUB_Output",
            "py_name": "generate_epub", "category": "because-i-said-so",
            "reason": "skip",
        }]}
        with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False) as f:
            json.dump(bad, f)
            manifest = f.name
        try:
            proc = subprocess.run(
                [sys.executable, "scripts/audit_parity.py", "--file", "epub_output",
                 "--exclusions", manifest],
                capture_output=True, text=True, cwd=repo)
        finally:
            os.unlink(manifest)
        self.assertIn("VALIDATION PROBLEMS", proc.stdout)
        self.assertIn("unknown category", proc.stdout)
        self.assertEqual(proc.returncode, 1)


class TestEndToEndHonestMetric(unittest.TestCase):
    """The anti-cheat scenario: a same-named one-line Go stub must not
    move the parity needle."""

    PY_SRC = '''
def do_real_work(items):
    total = 0
    for i in items:
        if i > 0:
            total += i
        elif i < -10:
            total -= 1
        else:
            total += 2
    return total

def do_stubbed_work(items):
    total = 0
    for i in items:
        if i > 0:
            total += i
        elif i < -10:
            total -= 1
        else:
            total += 2
    return total

def do_unported_work(items):
    return sorted(items)

def tiny():
    return None
'''

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        with open(os.path.join(self.tmp.name, "fake.py"), "w") as f:
            f.write(self.PY_SRC)
        self._old_py_dir = ap.PY_DIR
        ap.PY_DIR = self.tmp.name

    def tearDown(self):
        ap.PY_DIR = self._old_py_dir
        self.tmp.cleanup()

    def go_index(self):
        return index(
            mkgo("doRealWork", file="fake.go", nstmt=10),
            mkgo("doStubbedWork", file="fake.go", nstmt=1, const_only=True),
            mkgo("tiny", file="fake.go", nstmt=1, const_only=True, trivial_shape="const:nil"),
        )

    def audit(self, exclusions=None):
        return ap.audit_file("fake.py", go_index=self.go_index(),
                             exclusions=exclusions or [])

    def test_stub_not_counted_as_implemented(self):
        result = self.audit()
        by_name = {e["py_name"]: e["status"] for e in result["entries"]}
        self.assertEqual(by_name["do_real_work"], "implemented")
        self.assertEqual(by_name["do_stubbed_work"], "stub_silent")
        self.assertEqual(by_name["do_unported_work"], "missing")
        self.assertEqual(by_name["tiny"], "implemented_trivial")

        counts = result["counts"]
        total = sum(counts.values())
        self.assertEqual(total, result["python_function_count"])
        implemented = counts.get("implemented", 0) + \
            counts.get("implemented_trivial", 0) + \
            counts.get("implemented_delegation", 0)
        self.assertEqual(implemented, 2)  # NOT 3 — the stub must not count

    def test_exclusion_removes_from_denominator_but_is_reported(self):
        excl = [{
            "py_file": "fake.py", "py_class": None, "py_name": "do_stubbed_work",
            "category": "unused-direction",
            "reason": "serialization path never needed, decode-only tool",
        }]
        result = self.audit(exclusions=excl)
        counts = result["counts"]
        self.assertEqual(counts.get("excluded"), 1)
        self.assertEqual(counts.get("stub_silent", 0), 0)
        # total accounting still adds up
        self.assertEqual(sum(counts.values()), result["python_function_count"])

    def test_exclusion_cannot_upgrade_a_missing_function_to_implemented(self):
        excl = [{
            "py_file": "fake.py", "py_class": None, "py_name": "do_unported_work",
            "category": "unused-direction",
            "reason": "never called by the conversion pipeline",
        }]
        result = self.audit(exclusions=excl)
        by_name = {e["py_name"]: e["status"] for e in result["entries"]}
        self.assertEqual(by_name["do_unported_work"], "excluded")
        counts = result["counts"]
        implemented = counts.get("implemented", 0) + \
            counts.get("implemented_trivial", 0) + \
            counts.get("implemented_delegation", 0)
        self.assertEqual(implemented, 2)


class TestRealRepoSmoke(unittest.TestCase):
    """Runs against the real reference tree. Skips when REFERENCE/ is absent."""

    @classmethod
    def setUpClass(cls):
        cls.py_dir = os.path.join(REPO, "REFERENCE/KFX_Input/kfxlib")
        if not os.path.isdir(cls.py_dir):
            raise unittest.SkipTest("REFERENCE/KFX_Input/kfxlib not present")
        json_path = os.environ.get("GOFUNCINFO_JSON")
        if json_path and os.path.exists(json_path):
            with open(json_path) as f:
                data = json.load(f)
            cls.go_index = {"functions": data["functions"],
                            "call_counts": data.get("call_counts", {})}
            cls.go_index["by_lower"] = {}
            for fn in cls.go_index["functions"]:
                cls.go_index["by_lower"].setdefault(fn["name"].lower(), []).append(fn)
        else:
            try:
                cls.go_index = ap.gofuncinfo()
            except Exception as e:  # pragma: no cover
                raise unittest.SkipTest(f"gofuncinfo unavailable: {e}")

    def test_status_counts_add_up(self):
        results, exclusions, problems = ap.audit_all(ap.DEFAULT_EXCLUSIONS, None)
        # gofuncinfo() cached result already has by_lower; audit_all reuses it
        for r in results:
            self.assertEqual(sum(r["counts"].values()),
                             r["python_function_count"], r["python_file"])

    def test_known_stubs_are_not_implemented(self):
        results, exclusions, problems = ap.audit_all(ap.DEFAULT_EXCLUSIONS, None)
        by_key = {}
        for r in results:
            for e in r["entries"]:
                by_key[(r["python_file"], e["py_class"], e["py_name"])] = e["status"]
        # ion_binary serialize stubs: dead, const-only returns
        self.assertIn(by_key.get(("ion_binary.py", "IonBinary", "serialize_value")),
                      ("stub_silent", "excluded"))
        # output-mode wrapper admitting not-implemented
        self.assertIn(by_key.get(("yj_book.py", "YJ_Book", "convert_to_pdf")),
                      ("stub_admitted", "excluded"))

    def test_exclusions_validate(self):
        results, exclusions, problems = ap.audit_all(ap.DEFAULT_EXCLUSIONS, None)
        for p in problems:
            self.fail(f"exclusion validation problem: {p}")


if __name__ == "__main__":
    unittest.main()


class TestExactCaseIdentity(unittest.TestCase):
    """Go identifiers are case-sensitive; evidence and identity must be too."""

    def setUp(self):
        self.idx = index(
            mkgo("decodeKFX", file="yj_to_epub.go", nstmt=30),
            mkgo("DecodeKFX", file="yj_to_epub.go", nstmt=30),
        )

    def test_lowercase_evidence_does_not_match_exported(self):
        fn = ap.find_go_evidence(self.idx, {"go_file": "yj_to_epub.go",
                                            "go_func": "decodekfx"})
        self.assertIsNone(fn)  # exact-case only

    def test_evidence_case_mismatch_rejected(self):
        fn = ap.find_go_evidence(self.idx, {"go_file": "yj_to_epub.go",
                                            "go_func": "DecodeKFX"})
        self.assertIsNotNone(fn)
        self.assertEqual(fn["name"], "DecodeKFX")
        fn2 = ap.find_go_evidence(self.idx, {"go_file": "yj_to_epub.go",
                                             "go_func": "decodeKFX"})
        self.assertIsNotNone(fn2)
        self.assertEqual(fn2["name"], "decodeKFX")

    def test_go_index_lookup_exact_case(self):
        cands = ap.go_index_lookup(self.idx, "DecodeKFX", "yj_to_epub.go")
        self.assertEqual(len(cands), 1)
        self.assertEqual(cands[0]["name"], "DecodeKFX")
        self.assertEqual(ap.go_index_lookup(self.idx, "decodekfx"), [])


class TestMatchingConservatism(unittest.TestCase):
    """No automatic credit for ambiguous, generic, or cross-file matches."""

    PY_SRC = '''
class Style:
    def __len__(self):
        return len(self.props)

    def tostring(self):
        return "x"

def process_widget(w):
    return render(w)

def elsewhere_helper(x):
    return x + 1
'''

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.py_file = "match.py"
        with open(os.path.join(self.tmp.name, self.py_file), "w") as f:
            f.write(self.PY_SRC)
        self._old = ap.PY_DIR
        ap.PY_DIR = self.tmp.name

    def tearDown(self):
        ap.PY_DIR = self._old
        self.tmp.cleanup()

    def idx_with(self, *extra):
        # Baseline: an unrelated same-file String/Len pair (generic names),
        # a substantive processWidget in ANOTHER file (cross-file match),
        # and a unique same-file renderer.
        return index(
            mkgo("String", file="match.go", nstmt=5),
            mkgo("Len", file="match.go", nstmt=5),
            mkgo("renderer", file="match.go", nstmt=5),
            mkgo("processWidget", file="other.go", nstmt=40),
            *extra,
        )

    def audit(self, idx, overrides=None, exclusions=None):
        return ap.audit_file(self.py_file, go_index=idx,
                             exclusions=exclusions or [],
                             overrides=overrides or [])

    def test_generic_name_never_auto_matches(self):
        result = self.audit(self.idx_with())
        by = {e["py_name"]: e["status"] for e in result["entries"]}
        self.assertEqual(by["__len__"], "unresolved_match")  # Len is generic

    def test_two_same_file_string_methods_ambiguous(self):
        idx = self.idx_with(mkgo("String", file="match.go", nstmt=5, line=9))
        result = self.audit(idx)
        by = {e["py_name"]: e["status"] for e in result["entries"]}
        self.assertEqual(by["tostring"], "unresolved_match")

    def test_cross_file_unique_match_is_not_counted(self):
        result = self.audit(self.idx_with())
        e = next(e for e in result["entries"] if e["py_name"] == "process_widget")
        self.assertEqual(e["status"], "unresolved_match")
        self.assertIn("cross-file name-only match", e["unresolved_reason"])
        self.assertIn("requires explicit identity override", e["unresolved_reason"])

    def test_valid_override_maps_it(self):
        result = self.audit(self.idx_with(), overrides=[{
            "py_file": self.py_file, "py_class": None, "py_name": "process_widget",
            "mapping": {"go_file": "other.go", "go_func": "processWidget"},
            "reason": "widget rendering moved into other.go during refactor",
        }])
        e = next(e for e in result["entries"] if e["py_name"] == "process_widget")
        self.assertEqual(e["status"], "mapped")
        self.assertEqual(e["go_file"], "other.go")

    def test_override_with_trivial_target_rejected(self):
        idx = self.idx_with(mkgo("processWidget", file="stub.go", nstmt=1,
                                 const_only=True, trivial_shape="const:nil"))
        overrides = [{
            "py_file": self.py_file, "py_class": None, "py_name": "process_widget",
            "mapping": {"go_file": "stub.go", "go_func": "processWidget"},
            "reason": "mapping to a trivial function must be rejected",
        }]
        problems, valid = ap.validate_overrides(
            overrides, {self.py_file: ap.extract_python_functions(
                os.path.join(self.tmp.name, self.py_file))}, idx)
        self.assertTrue(any("trivial" in p for p in problems))
        self.assertEqual(valid, [])
        result = self.audit(idx, overrides=valid)
        e = next(e for e in result["entries"] if e["py_name"] == "process_widget")
        self.assertEqual(e["status"], "unresolved_match")

    def test_override_missing_reason_rejected(self):
        overrides = [{
            "py_file": self.py_file, "py_class": None, "py_name": "process_widget",
            "mapping": {"go_file": "other.go", "go_func": "processWidget"},
            "reason": "short",
        }]
        problems, valid = ap.validate_overrides(
            overrides, {self.py_file: ap.extract_python_functions(
                os.path.join(self.tmp.name, self.py_file))}, self.idx_with())
        self.assertTrue(any("reason" in p for p in problems))

    def test_same_file_unique_substantive_still_implements(self):
        idx = self.idx_with(mkgo("elsewhereHelper", file="match.go", nstmt=20))
        result = self.audit(idx)
        by = {e["py_name"]: e["status"] for e in result["entries"]}
        self.assertEqual(by["elsewhere_helper"], "implemented")


class TestDelegationEvidence(unittest.TestCase):
    """Only unambiguous unqualified Ident calls carry delegation credit."""

    def pf(self):
        return ap.PyFunc(name="do_work", class_name=None, line_start=1,
                         line_end=99, args="", docstring_first_line=None,
                         nstmt=50, nlit=0, trivial_shape="")

    def wrapper(self, ident_calls, file="w.go", calls=None):
        return mkgo("doWork", file=file, nstmt=1,
                    ident_calls=ident_calls, calls=calls if calls is not None else ident_calls)

    def test_selector_call_grants_no_credit(self):
        # A corpus func named TrimSpace exists, but the wrapper only calls
        # strings.TrimSpace (selector) — recorded in Calls, not IdentCalls.
        idx = index(mkgo("TrimSpace", file="s.go", nstmt=40),
                    self.wrapper(ident_calls=[], calls=["TrimSpace"]))
        w = idx["functions"][1]
        self.assertEqual(ap.transitive_substance(idx, w), 1)
        self.assertEqual(ap.classify(self.pf(), w, None,
                                     tsub=ap.transitive_substance(idx, w)), "thin")

    def test_ambiguous_ident_grants_no_credit(self):
        idx = index(mkgo("helper", file="a.go", nstmt=40),
                    mkgo("helper", file="b.go", nstmt=40),
                    self.wrapper(ident_calls=["helper"]))
        w = idx["functions"][2]
        self.assertEqual(ap.transitive_substance(idx, w), 1)  # wrapper only

    def test_unambiguous_ident_grants_credit(self):
        idx = index(mkgo("helper", file="w.go", nstmt=40),
                    self.wrapper(ident_calls=["helper"]))
        w = idx["functions"][1]
        self.assertGreaterEqual(ap.transitive_substance(idx, w), 40)
        self.assertEqual(ap.classify(self.pf(), w, None,
                                     tsub=ap.transitive_substance(idx, w)),
                         "implemented_delegation")


class TestSemanticPyTriviality(unittest.TestCase):
    def shapes(self, src):
        with tempfile.NamedTemporaryFile("w", suffix=".py", delete=False) as f:
            f.write(src)
            path = f.name
        try:
            return {pf.name: pf.trivial_shape
                    for pf in ap.extract_python_functions(path)}
        finally:
            os.unlink(path)

    def test_one_line_lookup_is_not_trivial(self):
        s = self.shapes("""
class C:
    def prop_lookup(self):
        return self.lookup[self.key]

    def call(self, x):
        return transform(x)

    def membership(self):
        return "nav" in self.props
""")
        self.assertEqual(s["prop_lookup"], "")   # index/attribute: substantive
        self.assertEqual(s["call"], "")          # call: substantive
        self.assertEqual(s["membership"], "")    # comparison: substantive

    def test_true_trivial_shapes(self):
        s = self.shapes('''
def f_none():
    return None

def f_true():
    return True

def f_int():
    return 3

def f_str():
    return "nav"

def f_empty():
    return ""

def f_identity(x):
    return x

def f_pass():
    pass

def f_docstring():
    """Docs."""
    pass
''')
        self.assertEqual(s["f_none"], "const:nil")
        self.assertEqual(s["f_true"], "const:true")
        self.assertEqual(s["f_int"], "const:int:3")
        self.assertEqual(s["f_str"], "const:string:nav")
        self.assertEqual(s["f_empty"], "const:empty-string")
        self.assertEqual(s["f_identity"], "arg:0")
        self.assertEqual(s["f_pass"], "void")
        self.assertEqual(s["f_docstring"], "void")

    def test_mixed_returns_not_trivial(self):
        s = self.shapes("""
def mixed(flag):
    if flag:
        return 1
    return 2
""")
        self.assertEqual(s["mixed"], "")


class TestCLIRegressions(unittest.TestCase):
    """--json must work for the full run; METRIC excluded printed once."""

    @classmethod
    def setUpClass(cls):
        repo = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
        cls.repo = repo
        if not os.path.isdir(os.path.join(repo, "REFERENCE/KFX_Input/kfxlib")):
            raise unittest.SkipTest("REFERENCE/KFX_Input/kfxlib not present")
        import shutil
        if not shutil.which("go"):
            raise unittest.SkipTest("go toolchain not available")

    def run_cli(self, *flags):
        import subprocess
        return subprocess.run(
            [sys.executable, "scripts/audit_parity.py", *flags],
            capture_output=True, text=True, cwd=self.repo)

    def test_full_run_json_outputs_json(self):
        proc = self.run_cli("--json")
        try:
            doc = json.loads(proc.stdout)
        except json.JSONDecodeError as e:
            self.fail(f"--json full run did not emit JSON: {e}\n{proc.stdout[:400]}")
        self.assertIn("results", doc)
        self.assertTrue(all("python_file" in r for r in doc["results"]))

    def test_metric_excluded_printed_once(self):
        proc = self.run_cli("--metric")
        self.assertEqual(proc.stdout.count("METRIC excluded="), 1, proc.stdout)

    def test_metric_renames_coverage(self):
        proc = self.run_cli("--metric")
        self.assertIn("METRIC structural_coverage_pct=", proc.stdout)
        # Backward-compatible alias retained
        self.assertIn("METRIC parity_pct=", proc.stdout)
