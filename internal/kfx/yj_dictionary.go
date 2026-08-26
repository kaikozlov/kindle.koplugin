package kfx

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/amazon-ion/ion-go/ion"
)

var dictionaryRuleInstructionRE = regexp.MustCompile(`^([0-9]+)(.)(.+)$`)

// processDictionaryRules ports KFX_EPUB_Misc.process_dictionary_rules added in
// KFX Input 2.34.0. Dictionary rules are stored as Ion-text blobs inside $597
// auxiliary_data metadata entries named yj.dictionary.inflection_rules.
func processDictionaryRules(auxiliaryData map[string]map[string]interface{}) map[int]string {
	rules := map[int]string{}
	for _, aux := range auxiliaryData {
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

		switch cmd {
		case "-":
			if endOffsetPos < 0 || endOffsetPos > len(runes) {
				log.Printf("kfx: error: Dictionary instruction %q has invalid offset for word %q in %q", instr, string(runes), rule)
				return string(runes)
			}
			runes = append(append(append([]rune{}, runes[:endOffsetPos]...), text...), runes[endOffsetPos:]...)
		case "+":
			start := endOffsetPos - len(text)
			if start < 0 || endOffsetPos < 0 || endOffsetPos > len(runes) || string(runes[start:endOffsetPos]) != string(text) {
				log.Printf("kfx: error: Dictionary instruction %q did not match word %q in %q", instr, string(runes), rule)
				return string(runes)
			}
			runes = append(append([]rune{}, runes[:start]...), runes[endOffsetPos:]...)
		case "/":
			if offset < 0 || offset > len(runes) {
				log.Printf("kfx: error: Dictionary instruction %q has invalid offset for word %q in %q", instr, string(runes), rule)
				return string(runes)
			}
			runes = append(append(append([]rune{}, runes[:offset]...), text...), runes[offset:]...)
		case "*":
			end := offset + len(text)
			if offset < 0 || end > len(runes) || string(runes[offset:end]) != string(text) {
				log.Printf("kfx: error: Dictionary instruction %q did not match word %q in %q", instr, string(runes), rule)
				return string(runes)
			}
			runes = append(append([]rune{}, runes[:offset]...), runes[end:]...)
		default:
			log.Printf("kfx: error: Unexpected dictionary rule command %q in %q", cmd, rule)
			return string(runes)
		}
	}
	return string(runes)
}
