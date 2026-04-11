package kfx

import "testing"

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
	if g := e.evaluate([]interface{}{"$660"}); g != true {
		t.Fatalf("$660 = %v", g)
	}
	if g := e.evaluate([]interface{}{"$293", []interface{}{"$660"}}); g != false {
		t.Fatalf("not true = %v", g)
	}
	if g := e.evaluate([]interface{}{"$516", 1, 2}); numericConditionValue(g) != 3 {
		t.Fatalf("$516 1+2 = %v", g)
	}
}
