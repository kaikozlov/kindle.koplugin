// Condition evaluation used by page-template branches; aligns with KFX_EPUB_Misc / yj_to_epub_misc.py.
// set_condition_operators (yj_to_epub_misc.py L29–66): keys and nargs live in conditionOperatorArity;
// semantics are implemented in conditionOperatorDispatch (same as former evaluate switch).
package kfx

import (
	"fmt"
	"os"
	"sync"
)

// Keys of Python self.condition_operators after set_condition_operators (yj_to_epub_misc.py L37–66).
var pythonConditionOperatorSymbols = map[string]struct{}{
	"$305": {}, "$304": {}, "$300": {}, "$301": {}, "$183": {}, "$302": {}, "$303": {},
	"$525": {}, "$526": {}, "$660": {},
	"$293": {}, "$266": {}, "$750": {}, "$659": {},
	"$292": {}, "$291": {}, "$294": {}, "$295": {}, "$296": {}, "$297": {}, "$298": {}, "$299": {},
	"$516": {}, "$517": {}, "$518": {}, "$519": {},
}

// conditionOperatorArity mirrors Python nargs: 0=nullary, 1=unary, 2=binary, -1=$659 (special).
var conditionOperatorArity = map[string]int{
	"$305": 0, "$304": 0, "$300": 0, "$301": 0, "$183": 0, "$302": 0, "$303": 0,
	"$525": 0, "$526": 0, "$660": 0,
	"$293": 1, "$266": 1, "$750": 1, "$659": -1,
	"$292": 2, "$291": 2, "$294": 2, "$295": 2, "$296": 2, "$297": 2, "$298": 2, "$299": 2,
	"$516": 2, "$517": 2, "$518": 2, "$519": 2,
}

func (e conditionEvaluator) screenSize() (int, int) {
	if e.orientationLock == "landscape" {
		return 1920, 1200
	}
	return 1200, 1920
}

func firstArg(args []interface{}) interface{} {
	if len(args) > 0 {
		return args[0]
	}
	return nil
}

func secondArg(args []interface{}) interface{} {
	if len(args) > 1 {
		return args[1]
	}
	return nil
}

func numericConditionValue(value interface{}) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	case bool:
		if typed {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func compareConditionValues(left interface{}, right interface{}) int {
	switch l := left.(type) {
	case bool:
		lv, rv := 0, 0
		if l {
			lv = 1
		}
		if rb, ok := right.(bool); ok && rb {
			rv = 1
		}
		switch {
		case lv < rv:
			return -1
		case lv > rv:
			return 1
		default:
			return 0
		}
	case string:
		rs, _ := right.(string)
		switch {
		case l < rs:
			return -1
		case l > rs:
			return 1
		default:
			return 0
		}
	default:
		lf := numericConditionValue(left)
		rf := numericConditionValue(right)
		switch {
		case lf < rf:
			return -1
		case lf > rf:
			return 1
		default:
			return 0
		}
	}
}

type conditionOpFn func(*conditionEvaluator, []interface{}, int, int) interface{}

var (
	conditionOperatorDispatchOnce sync.Once
	conditionOperatorDispatch     map[string]conditionOpFn
)

func conditionOperatorDispatchTable() map[string]conditionOpFn {
	conditionOperatorDispatchOnce.Do(func() {
		conditionOperatorDispatch = map[string]conditionOpFn{
			"$293": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return !e.evaluateBinary(firstArg(args))
			},
			"$266": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} {
				_, _ = width, height
				return 0
			},
			"$750": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				arg, _ := asString(firstArg(args))
				switch arg {
				case "$752":
					return true
				case "$753":
					return false
				default:
					return false
				}
			},
			"$659": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return knownSupportedFeatures[featureKey(args)]
			},
			"$292": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return e.evaluateBinary(firstArg(args)) && e.evaluateBinary(secondArg(args))
			},
			"$291": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return e.evaluateBinary(firstArg(args)) || e.evaluateBinary(secondArg(args))
			},
			"$294": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return compareConditionValues(e.evaluate(firstArg(args)), e.evaluate(secondArg(args))) == 0
			},
			"$295": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return compareConditionValues(e.evaluate(firstArg(args)), e.evaluate(secondArg(args))) != 0
			},
			"$296": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return compareConditionValues(e.evaluate(firstArg(args)), e.evaluate(secondArg(args))) > 0
			},
			"$297": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return compareConditionValues(e.evaluate(firstArg(args)), e.evaluate(secondArg(args))) >= 0
			},
			"$298": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return compareConditionValues(e.evaluate(firstArg(args)), e.evaluate(secondArg(args))) < 0
			},
			"$299": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return compareConditionValues(e.evaluate(firstArg(args)), e.evaluate(secondArg(args))) <= 0
			},
			"$516": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return numericConditionValue(e.evaluate(firstArg(args))) + numericConditionValue(e.evaluate(secondArg(args)))
			},
			"$517": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return numericConditionValue(e.evaluate(firstArg(args))) - numericConditionValue(e.evaluate(secondArg(args)))
			},
			"$518": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				return numericConditionValue(e.evaluate(firstArg(args))) * numericConditionValue(e.evaluate(secondArg(args)))
			},
			"$519": func(e *conditionEvaluator, args []interface{}, width, height int) interface{} {
				_, _ = width, height
				divisor := numericConditionValue(e.evaluate(secondArg(args)))
				if divisor == 0 {
					return 0
				}
				return numericConditionValue(e.evaluate(firstArg(args))) / divisor
			},
			"$305": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} { _ = width; return height },
			"$303": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} { _ = width; return height },
			"$304": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} { _ = height; return width },
			"$302": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} { _ = height; return width },
			"$300": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} {
				_ = width
				_ = height
				return true
			},
			"$301": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} {
				_ = width
				_ = height
				return true
			},
			"$183": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} {
				_ = width
				_ = height
				return 0
			},
			"$525": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} { return width > height },
			"$526": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} { return width < height },
			"$660": func(_ *conditionEvaluator, _ []interface{}, width, height int) interface{} {
				_ = width
				_ = height
				return true
			},
		}
	})
	return conditionOperatorDispatch
}

// Port of KFX_EPUB_Misc.evaluate_condition (yj_to_epub_misc.py) for Ion sexp conditions.
func (e conditionEvaluator) evaluate(condition interface{}) interface{} {
	switch typed := condition.(type) {
	case []interface{}:
		if len(typed) == 0 {
			return false
		}
		op, _ := asString(typed[0])
		args := typed[1:]
		width, height := e.screenSize()
		ee := e
		dispatch := conditionOperatorDispatchTable()
		if fn, ok := dispatch[op]; ok {
			// Port of Python nargs validation (yj_to_epub_misc.py evaluate_condition):
			//   if nargs != num: log.error("Condition operator has wrong number of arguments")
			if arity, known := conditionOperatorArity[op]; known && arity >= 0 && arity != len(args) {
				fmt.Fprintf(os.Stderr, "kfx: error: condition operator %q has wrong number of arguments: %v (expected %d)\n", op, args, arity)
				return false
			}
			return fn(&ee, args, width, height)
		}
		fmt.Fprintf(os.Stderr, "kfx: error: condition operator is unknown: %v\n", typed)
		return false
	case string:
		return e.evaluate([]interface{}{typed})
	case bool:
		return typed
	case int, int64, float64:
		return typed
	default:
		return false
	}
}

// Port of KFX_EPUB_Misc.evaluate_binary_condition (yj_to_epub_misc.py):
// logs error for non-binary results, matching Python's behavior.
func (e conditionEvaluator) evaluateBinary(condition interface{}) bool {
	value := e.evaluate(condition)
	typed, ok := value.(bool)
	if !ok && value != nil {
		fmt.Fprintf(os.Stderr, "kfx: error: condition has non-binary result (%v): %v\n", value, condition)
	}
	return ok && typed
}
