package kfx

import (
	"fmt"
	"os"
	"strings"
)

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
		return propertyValue(propName, yjValue)
	default:
		return propertyValue(propName, yjValue)
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
