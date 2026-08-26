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
