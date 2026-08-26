"""Tests for the honest branch auditor (scripts/audit_branches.py).

Pins:
  - branches are matched only inside the resolved Go counterpart's body
  - hollow stubs and missing counterparts cannot produce "found" coverage
  - universal-pattern heuristics report "weak", never "found"

Run:  python3 -m unittest discover -s scripts/tests -v
"""

import os
import sys
import tempfile
import unittest
from unittest import mock

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)
import audit_branches as ab  # noqa: E402
import audit_parity as ap  # noqa: E402


def fake_index(*funcs):
    by_lower = {}
    for f in funcs:
        by_lower.setdefault(f["name"].lower(), []).append(f)
    return {"functions": list(funcs), "by_lower": by_lower, "call_counts": {}}


def mkgo(name, file, line, end_line, nstmt=10):
    return {"file": file, "name": name, "line": line, "end_line": end_line,
            "nstmt": nstmt, "nlit": 0, "empty": False, "const_only": False,
            "error_only": False, "notimpl": False, "calls": [],
            "self_calls": 0, "called_by": 1}


class TestCheckGoForBranch(unittest.TestCase):
    def body_branch(self, desc, body):
        return ab.check_go_for_branch(None, {"description": desc, "types": ""}, body)

    def test_symbol_strong_match(self):
        self.assertEqual(self.body_branch('if $145 in value', 'if v, ok := value["content"]; ok {'), "found")

    def test_string_constant_strong_match(self):
        self.assertEqual(self.body_branch('if x == "kfx_cover_image"', 'if name == "kfx_cover_image" {'), "found")

    def test_numeric_compare_is_weak(self):
        self.assertEqual(self.body_branch("if i == 0", "total := 0\nfor i := range x {"), "weak")

    def test_len_compare_is_weak(self):
        self.assertEqual(self.body_branch("if len(x) == 1", "if len(x) == 1 {"), "weak")

    def test_var_compare_is_weak(self):
        self.assertEqual(self.body_branch("if i >= j", "if i >= j {"), "weak")

    def test_dead_code_is_weak(self):
        self.assertEqual(self.body_branch("if true", "whatever"), "weak")

    def test_unmatched_returns_unknown(self):
        self.assertEqual(self.body_branch("if hero_image_mode", "total := 0"), "unknown")

    def test_no_body_returns_uncertain_status(self):
        # go_content None -> legacy 'no-go-file' status, counted as uncertain
        self.assertIn(self.body_branch("if $145 in value", None), ("no-go-file", "unknown"))

    # --- generic-shape heuristics must be WEAK, never found ---

    def test_weak_reason_resolves_to_weak_not_unknown(self):
        # isinstance with no condition-specific evidence must classify weak
        # (reported heuristic), not silently fall through to unknown.
        result = self.body_branch("isinstance(value, dict)", "x := m[k]")
        self.assertEqual(result, "weak")

    def test_strong_evidence_beats_weak_reason(self):
        # A branch carrying BOTH an isinstance and a distinctive string
        # constant still earns strong credit — weak is a fallback, not a gate.
        result = self.body_branch('if isinstance(v, dict) and v.get("mode") == "hero_image"',
                                  'if m["mode"] == "hero_image" {')
        self.assertEqual(result, "found")

    def test_for_range_is_weak(self):
        self.assertEqual(self.body_branch("for child in children", "for _, c := range children {"), "weak")

    def test_else_is_weak(self):
        self.assertEqual(self.body_branch("else", "if x { } else { }"), "weak")

    def test_try_is_weak(self):
        self.assertEqual(self.body_branch("try", "if err != nil {"), "weak")

    def test_except_is_weak(self):
        self.assertEqual(self.body_branch("except Exception", "return err"), "weak")

    def test_isinstance_is_weak(self):
        self.assertEqual(self.body_branch("isinstance(value, IonStruct)", "v := asMap(value)"), "weak")

    def test_type_check_is_weak(self):
        self.assertEqual(self.body_branch("if data_type is IonString", "s := string(x)"), "weak")

    def test_none_check_is_weak(self):
        self.assertEqual(self.body_branch("if x is not None", "if v, ok := m[k]; ok {"), "weak")

    def test_keyword_only_is_weak(self):
        self.assertEqual(self.body_branch("if generate_toc_entries", "generateTocEntries := true"), "weak")

    # --- condition-specific evidence stays strong ---

    def test_symbol_word_boundary(self):
        # 'content' must NOT credit via 'content_list' (falls through to
        # keyword-weak because 'value' occurs — never 'found')
        self.assertEqual(self.body_branch('if $145 in value', 'x := value["content_list"]'), "weak")

    def test_string_word_boundary(self):
        # 'hero' must NOT credit strongly via 'heroes' (keyword-weak at most)
        self.assertEqual(self.body_branch('if kind == "hero"', 'name := "heroes"'), "weak")

    def test_single_char_string_constant_is_uncertain_not_strong(self):
        # Single-char string constants ("-", ".", "/") are substring noise:
        # len>=2 rule excludes them from strong credit, and with no other
        # evidence the branch lands in uncertain — never found.
        result = self.body_branch('if sep == "-"', 'strings.Contains(s, "-")')
        self.assertNotEqual(result, "found")
        self.assertEqual(result, "unknown")

    def test_method_name_is_strong(self):
        self.assertEqual(self.body_branch("if self.do_full_render()", "if r.doFullRender() {"), "found")

    def test_method_name_word_boundary(self):
        # 'head' must not credit strongly via 'headline' (keyword-weak at most)
        self.assertEqual(self.body_branch("if self.head()", "x := headline"), "weak")

    def test_constant_is_strong(self):
        self.assertEqual(self.body_branch("if COMBINE_NESTED_DIVS", "if COMBINE_NESTED_DIVS {"), "found")


class TestResolveGoBody(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.kfx_dir = os.path.join(self.tmp.name, "internal", "kfx")
        os.makedirs(self.kfx_dir)
        with open(os.path.join(self.kfx_dir, "fake.go"), "w") as f:
            f.write("package kfx\n\nfunc doWork(x int) int {\n\tif x > 0 {\n\t\treturn x\n\t}\n\treturn 0\n}\n\n"
                    "func hollowStub() error { return nil }\n")

    def tearDown(self):
        self.tmp.cleanup()

    def patch_index(self, idx):
        return mock.patch.object(ap, "gofuncinfo", lambda path=None: idx)

    def test_same_file_body_resolution(self):
        idx = fake_index(mkgo("doWork", "fake.go", 3, 7))
        with self.patch_index(idx):
            fn, body = ab.resolve_go_body("do_work", os.path.join(self.kfx_dir, "fake.go"))
        self.assertIsNotNone(fn)
        self.assertEqual(fn["name"], "doWork")
        self.assertIn("if x > 0", body)
        self.assertNotIn("hollowStub", body)  # body only, not the whole file

    def test_stub_body_is_just_the_stub(self):
        idx = fake_index(mkgo("hollowStub", "fake.go", 10, 10))
        with self.patch_index(idx):
            fn, body = ab.resolve_go_body("hollow_stub", os.path.join(self.kfx_dir, "fake.go"))
        self.assertEqual(fn["name"], "hollowStub")
        self.assertEqual(body.strip(), "func hollowStub() error { return nil }")

    def test_no_match_returns_none(self):
        idx = fake_index(mkgo("doWork", "fake.go", 3, 7))
        with self.patch_index(idx):
            fn, body = ab.resolve_go_body("unported", os.path.join(self.kfx_dir, "fake.go"))
        self.assertIsNone(fn)
        self.assertIsNone(body)

    def test_other_file_match_uses_its_own_file(self):
        idx = fake_index(mkgo("doWork", "other.go", 1, 1))
        # no other.go exists on disk -> body unresolvable but func found
        with self.patch_index(idx):
            fn, body = ab.resolve_go_body("do_work", os.path.join(self.kfx_dir, "fake.go"))
        self.assertIsNotNone(fn)


class TestStubCannotClaimBranchCoverage(unittest.TestCase):
    """End-to-end: a Python function matched only by a hollow Go stub must
    NOT report its branches as found."""

    PY_SRC = '''
def process_widget(w):
    if w.kind == "hero":
        return render_hero(w)
    elif w.kind == "blank":
        return ""
    for child in w.children:
        if child.visible:
            process_widget(child)
    return None
'''

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        with open(os.path.join(self.tmp.name, "fake.py"), "w") as f:
            f.write(self.PY_SRC)
        self.idx = fake_index(mkgo("processWidget", "fake.go", 1, 1, nstmt=1))

    def tearDown(self):
        self.tmp.cleanup()

    def audit(self):
        import contextlib, io, re
        out = io.StringIO()
        with mock.patch.object(ap, "gofuncinfo", lambda path=None: self.idx):
            with contextlib.redirect_stdout(out):
                ab.audit_function(os.path.join(self.tmp.name, "fake.py"), None,
                                  "process_widget")
        text = out.getvalue()
        found = int(re.search(r'Found in Go:\s*(\d+)', text).group(1))
        uncertain = int(re.search(r'Uncertain:\s*(\d+)', text).group(1))
        missing = int(re.search(r'Missing in Go:\s*(\d+)', text).group(1))
        return found, uncertain, missing, text

    def test_hollow_stub_gives_no_coverage(self):
        found, uncertain, missing, text = self.audit()
        self.assertEqual(found, 0)
        self.assertEqual(uncertain, 5)
        self.assertEqual(missing, 0)

    def test_substantive_body_gives_coverage(self):
        go_path = os.path.join(self.tmp.name, "fake.go")
        with open(go_path, "w") as f:
            f.write('package kfx\n\nfunc processWidget(w widget) string {\n'
                    '\tif w.kind == "hero" {\n\t\treturn renderHero(w)\n\t}\n'
                    '\tfor _, child := range w.children {\n\t\tprocessWidget(child)\n\t}\n'
                    '\treturn ""\n}\n')
        self.idx = fake_index(mkgo("processWidget", "fake.go", 3, 10, nstmt=6))
        import contextlib, io, re
        out = io.StringIO()
        with mock.patch.object(ap, "gofuncinfo", lambda path=None: self.idx):
            with contextlib.redirect_stdout(out):
                ab.audit_function(os.path.join(self.tmp.name, "fake.py"), go_path,
                                  "process_widget")
        text = out.getvalue()
        found = int(re.search(r'Found in Go:\s*(\d+)', text).group(1))
        self.assertGreater(found, 0)


if __name__ == "__main__":
    unittest.main()
