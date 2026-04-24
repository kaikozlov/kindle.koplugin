package kfx

import (
	"testing"
)

func TestPythonConditionOperatorSymbolCount(t *testing.T) {
	// yj_to_epub_misc.py registers 26 Ion operator symbols in set_condition_operators.
	if got := len(pythonConditionOperatorSymbols); got != 26 {
		t.Fatalf("operator symbol count = %d want 26", got)
	}
	if len(conditionOperatorArity) != len(pythonConditionOperatorSymbols) {
		t.Fatalf("arity map len %d != symbol set len %d", len(conditionOperatorArity), len(pythonConditionOperatorSymbols))
	}
	for sym := range pythonConditionOperatorSymbols {
		if _, ok := conditionOperatorArity[sym]; !ok {
			t.Fatalf("missing arity for %s", sym)
		}
	}
	for sym := range conditionOperatorArity {
		if _, ok := pythonConditionOperatorSymbols[sym]; !ok {
			t.Fatalf("extra arity entry %s", sym)
		}
	}
}

func TestEvaluateConditionDispatchMatchesLegacyCases(t *testing.T) {
	e := conditionEvaluator{orientationLock: "portrait", fixedLayout: false, illustratedLayout: false}
	if g := e.evaluate([]interface{}{"yj.illustrated_layout"}); g != true {
		t.Fatalf("$660 = %v", g)
	}
	if g := e.evaluate([]interface{}{"not", []interface{}{"yj.illustrated_layout"}}); g != false {
		t.Fatalf("not true = %v", g)
	}
	if g := e.evaluate([]interface{}{"+", 1, 2}); numericConditionValue(g) != 3 {
		t.Fatalf("$516 1+2 = %v", g)
	}
}

// ---------------------------------------------------------------------------
// GAP 1: processPath — path bundle lookup from $692 book data
// Python yj_to_epub_misc.py process_path L288-298
// ---------------------------------------------------------------------------

// TestProcessPath_DirectPath tests that processPath correctly renders SVG path
// instructions from a direct path list (non-bundle reference).
// Python yj_to_epub_misc.py L300-333 (the p = list(path) branch).
func TestProcessPath_DirectPath(t *testing.T) {
	// Path data: M(0) x y, L(1) x y, Z(4)
	path := []interface{}{
		float64(0), float64(10.0), float64(20.0), // M 10 20
		float64(1), float64(30.0), float64(40.0), // L 30 40
		float64(4), // Z
	}

	result := processPathWithBundles(path, nil)
	expected := "M 10 20 L 30 40 Z"
	if result != expected {
		t.Errorf("processPath(direct) = %q, want %q", result, expected)
	}
}

// TestProcessPath_BundleLookup tests that processPath correctly looks up a named
// path bundle from book data and renders the referenced path.
// Python yj_to_epub_misc.py L289-298:
//   path_bundle_name = path.pop("name")
//   path_index = path.pop("$403")
//   return self.process_path(self.book_data["$692"][path_bundle_name]["$693"][path_index])
//
// VAL-M7-001: Path bundle lookup from $692.
func TestProcessPath_BundleLookup(t *testing.T) {
	// Create a path bundle: {"my_bundle": {"path_list": [[M, L, Z], [M, L, L, Z]]}}
	bundles := map[string]map[string]interface{}{
		"my_bundle": {
			"path_list": []interface{}{
				// First path: M 10 20 L 30 40 Z
				[]interface{}{float64(0), float64(10.0), float64(20.0), float64(1), float64(30.0), float64(40.0), float64(4)},
				// Second path: M 5 5 L 10 10 L 15 15 Z
				[]interface{}{float64(0), float64(5.0), float64(5.0), float64(1), float64(10.0), float64(10.0), float64(1), float64(15.0), float64(15.0), float64(4)},
			},
		},
	}

	// Reference the second path (index=1) from bundle "my_bundle"
	pathRef := map[string]interface{}{
		"name":  "my_bundle",
		"index": float64(1),
	}

	result := processPathWithBundles(pathRef, bundles)
	expected := "M 5 5 L 10 10 L 15 15 Z"
	if result != expected {
		t.Errorf("processPath(bundle lookup) = %q, want %q", result, expected)
	}
}

// TestProcessPath_BundleLookup_FirstPath tests index 0 in a path bundle.
func TestProcessPath_BundleLookup_FirstPath(t *testing.T) {
	bundles := map[string]map[string]interface{}{
		"shapes": {
			"path_list": []interface{}{
				[]interface{}{float64(0), float64(100.0), float64(200.0), float64(4)},
			},
		},
	}

	pathRef := map[string]interface{}{
		"name":  "shapes",
		"index": float64(0),
	}

	result := processPathWithBundles(pathRef, bundles)
	expected := "M 100 200 Z"
	if result != expected {
		t.Errorf("processPath(bundle[0]) = %q, want %q", result, expected)
	}
}

// TestProcessPath_MissingBundle tests that a missing bundle name returns empty
// and logs an error, matching Python L294-296.
func TestProcessPath_MissingBundle(t *testing.T) {
	bundles := map[string]map[string]interface{}{
		"existing": {"path_list": []interface{}{}},
	}

	pathRef := map[string]interface{}{
		"name":  "nonexistent",
		"index": float64(0),
	}

	result := processPathWithBundles(pathRef, bundles)
	if result != "" {
		t.Errorf("processPath(missing bundle) = %q, want empty string", result)
	}
}

// TestProcessPath_PathWithCurves tests quadratic (Q) and cubic (C) Bezier curves.
// Python L316-317: inst==2 → Q with 4 args, inst==3 → C with 6 args.
func TestProcessPath_PathWithCurves(t *testing.T) {
	path := []interface{}{
		float64(0), float64(0.0), float64(0.0),                        // M 0 0
		float64(2), float64(10.0), float64(0.0), float64(10.0), float64(10.0), // Q 10 0 10 10
		float64(3), float64(20.0), float64(10.0), float64(20.0), float64(0.0), float64(30.0), float64(0.0), // C 20 10 20 0 30 0
		float64(4), // Z
	}

	result := processPathWithBundles(path, nil)
	expected := "M 0 0 Q 10 0 10 10 C 20 10 20 0 30 0 Z"
	if result != expected {
		t.Errorf("processPath(curves) = %q, want %q", result, expected)
	}
}
