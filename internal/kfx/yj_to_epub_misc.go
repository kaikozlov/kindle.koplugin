package kfx

import (
	"fmt"
	"os"
	"strings"
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

// ---------------------------------------------------------------------------
// Merged from svg.go (origin: yj_to_epub_misc.py KVG/SVG rendering)
// ---------------------------------------------------------------------------


func adjustPixelValue(value float64) float64 {
	return value
}

func processKVGShape(parent *htmlElement, shape map[string]interface{}, writingMode string) {
	shapeType, _ := asString(shape["$159"])
	delete(shape, "$159")

	var elem *htmlElement

	switch shapeType {
	case "$273":
		pathData := shape["$249"]
		delete(shape, "$249")
		d := processPath(pathData)
		elem = &htmlElement{
			Tag:   "path",
			Attrs: map[string]string{"d": d},
		}
		parent.Children = append(parent.Children, elem)

	case "$270":
		source, _ := asString(shape["$474"])
		delete(shape, "$474")
		if source == "" {
			fmt.Fprintf(os.Stderr, "kfx: error: missing KVG container content source\n")
			return
		}
		elem = &htmlElement{
			Tag:   "text",
			Attrs: map[string]string{},
		}
		parent.Children = append(parent.Children, elem)

	default:
		if shapeType != "" {
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected shape type: %s\n", shapeType)
		}
		return
	}

	svgAttrs := [][2]string{
		{"$70", "fill"},
		{"$72", "fill-opacity"},
		{"$75", "stroke"},
		{"$531", "stroke-dasharray"},
		{"$532", "stroke-dashoffset"},
		{"$77", "stroke-linecap"},
		{"$529", "stroke-linejoin"},
		{"$530", "stroke-miterlimit"},
		{"$76", "stroke-width"},
		{"$98", "transform"},
	}

	for _, attr := range svgAttrs {
		yjPropName := attr[0]
		svgAttrib := attr[1]
		if val, ok := shape[yjPropName]; ok {
			delete(shape, yjPropName)
			elem.Attrs[svgAttrib] = propertyValueSVG(yjPropName, val)
		}
	}

	if _, hasStroke := elem.Attrs["stroke"]; hasStroke {
		if _, hasFill := elem.Attrs["fill"]; !hasFill {
			elem.Attrs["fill"] = "none"
		}
	}
}

func propertyValueSVG(propName string, yjValue interface{}) string {
	switch v := yjValue.(type) {
	case float64:
		if colorYJProperties[propName] {
			return fixColorValue(v)
		}
		return valueStr(v)
	case *float64:
		if v == nil {
			return ""
		}
		return propertyValueSVG(propName, *v)
	case int:
		return propertyValueSVG(propName, float64(v))
	case int64:
		return propertyValueSVG(propName, float64(v))
	case []interface{}:
		if propName == "$98" {
			return processTransform(v, true)
		}
		return propertyValue(propName, yjValue, nil)
	default:
		return propertyValue(propName, yjValue, nil)
	}
}

func processPath(path interface{}) string {
	if m, ok := asMap(path); ok {
		bundleName, _ := asString(m["name"])
		pathIndex, _ := asInt(m["$403"])
		_ = bundleName
		_ = pathIndex
		fmt.Fprintf(os.Stderr, "kfx: error: path bundle lookup not yet implemented: %s\n", bundleName)
		return ""
	}

	p, ok := asSlice(path)
	if !ok {
		return ""
	}

	d := []string{}
	remaining := make([]interface{}, len(p))
	copy(remaining, p)

	processInstruction := func(inst string, nArgs int) {
		d = append(d, inst)
		for j := 0; j < nArgs; j++ {
			if len(remaining) == 0 {
				fmt.Fprintf(os.Stderr, "kfx: error: incomplete path instruction in %v\n", p)
				return
			}
			v := remaining[0]
			remaining = remaining[1:]
			vf, ok := asFloat64(v)
			if ok {
				v = adjustPixelValue(vf)
			}
			d = append(d, valueStr(v))
		}
	}

	for len(remaining) > 0 {
		inst := remaining[0]
		remaining = remaining[1:]
		instInt, ok := asInt(inst)
		if !ok {
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected path instruction %v in %v\n", inst, p)
			break
		}
		switch instInt {
		case 0:
			processInstruction("M", 2)
		case 1:
			processInstruction("L", 2)
		case 2:
			processInstruction("Q", 4)
		case 3:
			processInstruction("C", 6)
		case 4:
			processInstruction("Z", 0)
		default:
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected path instruction %d in %v\n", instInt, p)
			break
		}
	}

	return strings.Join(d, " ")
}

func processPolygon(path []interface{}) string {
	percentValueStr := func(v interface{}) string {
		f, ok := asFloat64(v)
		if !ok {
			return valueStr(v) + "%"
		}
		return valueStr(f*100) + "%"
	}

	d := []string{}
	i := 0
	ln := len(path)
	for i < ln {
		inst, ok := asInt(path[i])
		if !ok {
			fmt.Fprintf(os.Stderr, "kfx: error: bad path instruction in %v\n", path)
			break
		}
		switch inst {
		case 0, 1:
			if i+3 > ln {
				fmt.Fprintf(os.Stderr, "kfx: error: bad path instruction in %v\n", path)
				return fmt.Sprintf("polygon(%s)", strings.Join(d, ", "))
			}
			d = append(d, fmt.Sprintf("%s %s", percentValueStr(path[i+1]), percentValueStr(path[i+2])))
			i += 3
		case 4:
			i++
		default:
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected path instruction %d in %v\n", inst, path)
			break
		}
	}

	return fmt.Sprintf("polygon(%s)", strings.Join(d, ", "))
}

func processTransform(vals []interface{}, svg bool) string {
	var px, sep string
	if svg {
		px = ""
		sep = " "
	} else {
		px = "px"
		sep = ","
	}

	if len(vals) != 6 {
		fmt.Fprintf(os.Stderr, "kfx: error: unexpected transform: %v\n", vals)
		return "?"
	}

	v := make([]float64, 6)
	for i, val := range vals {
		f, ok := asFloat64(val)
		if !ok {
			fmt.Fprintf(os.Stderr, "kfx: error: unexpected transform value: %v\n", vals)
			return "?"
		}
		v[i] = f
	}

	v[4] = adjustPixelValue(v[4])
	v[5] = adjustPixelValue(v[5])

	var translate string
	if v[4] == 0 && v[5] == 0 {
		translate = ""
	} else {
		translate = fmt.Sprintf("translate(%s%s%s) ", svgValueStr(v[4], px), sep, svgValueStr(v[5], px))
	}

	if v[0] == 1 && v[1] == 0 && v[2] == 0 && v[3] == 1 && translate != "" {
		return strings.TrimSpace(translate)
	}

	if v[1] == 0 && v[2] == 0 {
		if v[0] == v[3] {
			return translate + fmt.Sprintf("scale(%s)", valueStr(v[0]))
		}
		return translate + fmt.Sprintf("scale(%s%s%s)", valueStr(v[0]), sep, valueStr(v[3]))
	}

	if v[0] == 0 && v[1] == 1 && v[2] == -1 && v[3] == 0 {
		return translate + "rotate(-90deg)"
	}

	if v[0] == 0 && v[1] == -1 && v[2] == 1 && v[3] == 0 {
		return translate + "rotate(90deg)"
	}

	if v[0] == -1 && v[1] == 0 && v[2] == 0 && v[3] == -1 {
		return translate + "rotate(180deg)"
	}

	fmt.Fprintf(os.Stderr, "kfx: warning: unexpected transform matrix: %v\n", vals)
	strVals := make([]string, 6)
	for i, val := range v {
		strVals[i] = valueStr(val)
	}
	return fmt.Sprintf("matrix(%s)", strings.Join(strVals, sep))
}

func svgValueStr(v float64, unit string) string {
	s := valueStr(v)
	if unit != "" && s != "0" {
		return s + unit
	}
	return s
}
