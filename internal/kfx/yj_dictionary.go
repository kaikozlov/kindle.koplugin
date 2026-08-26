package kfx

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/amazon-ion/ion-go/ion"
)

var dictionaryRuleInstructionRE = regexp.MustCompile(`^([0-9]+)(.)(.+)$`)

// processDictionaryRules ports KFX_EPUB_Misc.process_dictionary_rules added in
// KFX Input 2.34.0. Dictionary rules are stored as Ion-text blobs inside $597
// auxiliary_data metadata entries named yj.dictionary.inflection_rules.
func processDictionaryRules(auxiliaryData map[string]map[string]interface{}, auxiliaryDataOrder []string) map[int]string {
	rules := map[int]string{}
	// Python yj_to_epub_misc.py:495 iterates book_data[$597].values(), whose
	// insertion order is the original fragment-list order established by
	// organize_fragments_by_type (yj_to_epub.py:201-219). Preserve that order
	// because later duplicate rule IDs overwrite earlier ones.
	for _, auxID := range auxiliaryDataOrder {
		aux := auxiliaryData[auxID]
		if aux == nil {
			continue
		}
		metadata, _ := asSlice(aux["metadata"])
		for _, rawMD := range metadata {
			md, ok := asMap(rawMD)
			if !ok || asStringDefault(md["key"]) != "yj.dictionary.inflection_rules" {
				continue
			}
			raw, ok := md["value"].([]byte)
			if !ok {
				log.Printf("kfx: error: dictionary rule metadata has unexpected value type %T", md["value"])
				continue
			}
			parsed, err := parseDictionaryRuleSet(raw)
			if err != nil {
				log.Printf("kfx: error: exception while parsing dictionary rules: %v", err)
				continue
			}
			for id, rule := range parsed {
				rules[id] = rule
			}
			log.Printf("kfx: info: Parsed %d dictionary rules", len(rules))
		}
	}
	return rules
}

func parseDictionaryRuleSet(data []byte) (map[int]string, error) {
	decoder := ion.NewDecoder(ion.NewReaderString(string(data)))
	value, err := decoder.Decode()
	if err != nil {
		return nil, err
	}
	value = normalizeIon(value, nil)
	items, ok := asSlice(value)
	if !ok {
		return nil, fmt.Errorf("dictionary rule set is %T, want list", value)
	}
	rules := make(map[int]string, len(items))
	for index, raw := range items {
		item, ok := asMap(raw)
		if !ok {
			return nil, fmt.Errorf("dictionary rule %d is %T, want struct", index, raw)
		}
		id, ok := asInt(item["id"])
		if !ok {
			return nil, fmt.Errorf("dictionary rule %d has invalid id %v", index, item["id"])
		}
		rule, ok := asString(item["rule"])
		if !ok {
			return nil, fmt.Errorf("dictionary rule %d has invalid rule %v", index, item["rule"])
		}
		rules[id] = rule
	}
	return rules, nil
}

// pythonDictionarySliceIndex normalizes one Python string-slice bound for a
// rune slice: negative bounds count from the end and every bound is clamped to
// [0, len]. Python KFX Input relies on this behavior in unapply_dictionary_rule.
func pythonDictionarySliceIndex(index, length int) int {
	if index < 0 {
		index += length
		if index < 0 {
			return 0
		}
	}
	if index > length {
		return length
	}
	return index
}

// unapplyDictionaryRule reverses an Amazon dictionary inflection rule. Slicing
// is rune-based to preserve Python's Unicode string indexing semantics.
func unapplyDictionaryRule(word, rule string) string {
	runes := []rune(word)
	instructions := strings.Split(rule, ";")
	for i := len(instructions) - 1; i >= 0; i-- {
		instr := instructions[i]
		if instr == "" {
			continue
		}
		match := dictionaryRuleInstructionRE.FindStringSubmatch(instr)
		if match == nil {
			log.Printf("kfx: error: Unexpected dictionary rule instruction %q in %q", instr, rule)
			break
		}
		offset, _ := strconv.Atoi(match[1])
		cmd := match[2]
		text := []rune(match[3])
		endOffsetPos := len(runes) - offset

		// Python yj_to_epub_misc.py:524-541 uses ordinary string slices.
		// Preserve their clamping/negative-index behavior rather than rejecting
		// offsets that Go cannot use directly as slice indices.
		switch cmd {
		case "-":
			end := pythonDictionarySliceIndex(endOffsetPos, len(runes))
			runes = append(append(append([]rune{}, runes[:end]...), text...), runes[end:]...)
		case "+":
			start := pythonDictionarySliceIndex(endOffsetPos-len(text), len(runes))
			end := pythonDictionarySliceIndex(endOffsetPos, len(runes))
			if start > end || string(runes[start:end]) != string(text) {
				log.Printf("kfx: error: Dictionary instruction %q did not match word %q in %q", instr, string(runes), rule)
				return string(runes)
			}
			runes = append(append([]rune{}, runes[:start]...), runes[end:]...)
		case "/":
			pos := pythonDictionarySliceIndex(offset, len(runes))
			runes = append(append(append([]rune{}, runes[:pos]...), text...), runes[pos:]...)
		case "*":
			start := pythonDictionarySliceIndex(offset, len(runes))
			end := pythonDictionarySliceIndex(offset+len(text), len(runes))
			if start > end || string(runes[start:end]) != string(text) {
				log.Printf("kfx: error: Dictionary instruction %q did not match word %q in %q", instr, string(runes), rule)
				return string(runes)
			}
			runes = append(append([]rune{}, runes[:start]...), runes[end:]...)
		default:
			log.Printf("kfx: error: Unexpected dictionary rule command %q in %q", cmd, rule)
			return string(runes)
		}
	}
	return string(runes)
}
type dictionaryEntrySpec struct {
	orthography string
	inflections []string
}

func hasDictionaryEntryData(content map[string]interface{}) bool {
	if asStringDefault(content["type"]) != "container" {
		return false
	}
	_, hasTerm := content["yj.dictionary.term"]
	_, hasUnnormalized := content["yj.dictionary.unnormalized_term"]
	_, hasRules := content["yj.dictionary.rules"]
	return hasTerm || hasUnnormalized || hasRules
}

func dictionaryStringList(value interface{}, field string) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	if s, ok := asString(value); ok {
		return []string{s}, nil
	}
	values, ok := asSlice(value)
	if !ok {
		return nil, &UnsupportedError{Message: fmt.Sprintf("dictionary %s is %T, want string or list", field, value)}
	}
	result := make([]string, 0, len(values))
	for _, raw := range values {
		s, ok := asString(raw)
		if !ok {
			return nil, &UnsupportedError{Message: fmt.Sprintf("dictionary %s contains %T, want string", field, raw)}
		}
		result = append(result, s)
	}
	return result, nil
}

func dictionaryRuleIDList(value interface{}) ([]int, error) {
	if value == nil {
		return nil, nil
	}
	values, ok := asSlice(value)
	if !ok {
		return nil, &UnsupportedError{Message: fmt.Sprintf("dictionary rules are %T, want list", value)}
	}
	result := make([]int, 0, len(values))
	for _, raw := range values {
		id, ok := asInt(raw)
		if !ok {
			return nil, &UnsupportedError{Message: fmt.Sprintf("dictionary rule id is %T, want integer", raw)}
		}
		result = append(result, id)
	}
	return result, nil
}

// consumeDictionaryEntry ports the dictionary-specific block of
// KFX_EPUB_Content.process_content (yj_to_epub_content.py:542-588). It mutates
// the working content map just as Python pops the three yj.dictionary fields.
func (r *storylineRenderer) consumeDictionaryEntry(content map[string]interface{}) (*dictionaryEntrySpec, error) {
	dictionaryTerms, err := dictionaryStringList(content["yj.dictionary.term"], "term")
	if err != nil {
		return nil, err
	}
	delete(content, "yj.dictionary.term")

	unnormalizedTerms, err := dictionaryStringList(content["yj.dictionary.unnormalized_term"], "unnormalized_term")
	if err != nil {
		return nil, err
	}
	delete(content, "yj.dictionary.unnormalized_term")
	if len(unnormalizedTerms) != len(dictionaryTerms) {
		log.Printf("kfx: warning: Unexpected number of unnormalized dictionary terms (%d vs %d): %v vs %v", len(unnormalizedTerms), len(dictionaryTerms), unnormalizedTerms, dictionaryTerms)
	}

	rawRules := content["yj.dictionary.rules"]
	delete(content, "yj.dictionary.rules")
	if len(dictionaryTerms) == 0 {
		// Python indexes dictionary_terms[0] or [-1] below and raises IndexError.
		return nil, &UnsupportedError{Message: "dictionary entry has no yj.dictionary.term"}
	}

	var orthography string
	inflections := []string{}
	wordRules := []struct {
		word  string
		rules []int
	}{}

	rulesList, ok := asSlice(rawRules)
	if rawRules != nil && !ok {
		return nil, &UnsupportedError{Message: fmt.Sprintf("dictionary rules are %T, want list", rawRules)}
	}
	structuredRules := len(rulesList) > 0
	if structuredRules {
		_, structuredRules = asMap(rulesList[0])
	}

	if structuredRules {
		orthography = dictionaryTerms[0]
		inflections = append(inflections, dictionaryTerms[1:]...)
		for _, rawRule := range rulesList {
			ruleMap, ok := asMap(rawRule)
			if !ok {
				return nil, &UnsupportedError{Message: fmt.Sprintf("dictionary structured rule is %T, want struct", rawRule)}
			}
			working := cloneMap(ruleMap)
			word, ok := asString(working["yj.dictionary.word"])
			if !ok {
				return nil, &UnsupportedError{Message: "dictionary structured rule is missing yj.dictionary.word"}
			}
			delete(working, "yj.dictionary.word")
			ids, err := dictionaryRuleIDList(working["yj.dictionary.rules"])
			if err != nil {
				return nil, err
			}
			if _, exists := working["yj.dictionary.rules"]; !exists {
				return nil, &UnsupportedError{Message: "dictionary structured rule is missing yj.dictionary.rules"}
			}
			delete(working, "yj.dictionary.rules")
			for key := range working {
				log.Printf("kfx: error: dictionary rule for %q has extra data key %q", word, key)
			}
			wordRules = append(wordRules, struct {
				word  string
				rules []int
			}{word: word, rules: ids})
		}
	} else {
		orthography = dictionaryTerms[len(dictionaryTerms)-1]
		inflections = append(inflections, dictionaryTerms[:len(dictionaryTerms)-1]...)
		ids, err := dictionaryRuleIDList(rawRules)
		if err != nil {
			return nil, err
		}
		wordRules = append(wordRules, struct {
			word  string
			rules []int
		}{word: orthography, rules: ids})
	}

	if r.usedDictionaryRules == nil {
		r.usedDictionaryRules = map[int]struct{}{}
	}
	for _, wr := range wordRules {
		for _, ruleID := range wr.rules {
			r.usedDictionaryRules[ruleID] = struct{}{}
			if rule, exists := r.dictionaryRules[ruleID]; exists {
				inflections = append(inflections, unapplyDictionaryRule(wr.word, rule))
			}
		}
	}

	return &dictionaryEntrySpec{orthography: orthography, inflections: inflections}, nil
}

func decorateDictionaryEntry(element *htmlElement, spec *dictionaryEntrySpec) *htmlElement {
	if element == nil || spec == nil {
		return element
	}
	if element.Attrs == nil {
		element.Attrs = map[string]string{}
	}
	element.Tag = "idx:entry"
	element.Attrs["scriptable"] = "yes"
	orth := &htmlElement{Tag: "idx:orth", Attrs: map[string]string{"value": spec.orthography}}
	for _, inflection := range spec.inflections {
		orth.Children = append(orth.Children, &htmlElement{
			Tag: "idx:infl",
			Children: []htmlPart{&htmlElement{Tag: "idx:iform", Attrs: map[string]string{"value": inflection}}},
		})
	}
	element.Children = append([]htmlPart{orth}, element.Children...)
	return element
}
func dictionaryContentElement(part htmlPart) *htmlElement {
	element, ok := part.(*htmlElement)
	if !ok || element == nil {
		return nil
	}
	// wrapNodeLink may place the rendered content element inside a single <a>.
	if element.Tag == "a" && len(element.Children) == 1 {
		if child, ok := element.Children[0].(*htmlElement); ok {
			return child
		}
	}
	return element
}

func (r *storylineRenderer) setRenderError(err error) {
	if err != nil && r.renderError == nil {
		r.renderError = err
	}
}

func (r *storylineRenderer) reportDictionaryRuleUsage() {
	defined := map[int]struct{}{}
	for id := range r.dictionaryRules {
		defined[id] = struct{}{}
	}
	unused := []int{}
	for id := range defined {
		if _, used := r.usedDictionaryRules[id]; !used {
			unused = append(unused, id)
		}
	}
	undefined := []int{}
	for id := range r.usedDictionaryRules {
		if _, exists := defined[id]; !exists {
			undefined = append(undefined, id)
		}
	}
	sort.Ints(unused)
	sort.Ints(undefined)
	if len(unused) > 0 {
		log.Printf("kfx: warning: Unused dictionary rules: %v", unused)
	}
	if len(undefined) > 0 {
		log.Printf("kfx: warning: Undefined dictionary rules: %v", undefined)
	}
}

