package kfx

import "testing"

func TestParseDictionaryRuleSet(t *testing.T) {
	rules, err := parseDictionaryRuleSet([]byte(`[{id:1,rule:"0+s"},{id:7,rule:"1-ed;0+ing"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if rules[1] != "0+s" || rules[7] != "1-ed;0+ing" || len(rules) != 2 {
		t.Fatalf("dictionary rules = %#v", rules)
	}
}

func TestProcessDictionaryRulesFromAuxiliaryMetadata(t *testing.T) {
	aux := map[string]map[string]interface{}{
		"rules": {"metadata": []interface{}{
			map[string]interface{}{"key": "ignored", "value": []byte(`[]`)},
			map[string]interface{}{"key": "yj.dictionary.inflection_rules", "value": []byte(`[{id:3,rule:"0+s"}]`)},
		}},
	}
	rules := processDictionaryRules(aux, []string{"rules"})
	if len(rules) != 1 || rules[3] != "0+s" {
		t.Fatalf("dictionary rules = %#v", rules)
	}
}


func TestProcessDictionaryRulesPreservesAuxiliaryOrder(t *testing.T) {
	// Python yj_to_epub.py:201-219 + yj_to_epub_misc.py:495-503: the
	// categorized $597 dict preserves fragment insertion order and a later
	// duplicate rule id overwrites the earlier definition.
	aux := map[string]map[string]interface{}{
		"first": {"metadata": []interface{}{map[string]interface{}{
			"key": "yj.dictionary.inflection_rules", "value": []byte(`[{id:3,rule:"0+s"}]`),
		}}},
		"second": {"metadata": []interface{}{map[string]interface{}{
			"key": "yj.dictionary.inflection_rules", "value": []byte(`[{id:3,rule:"0+es"}]`),
		}}},
	}
	rules := processDictionaryRules(aux, []string{"first", "second"})
	if got := rules[3]; got != "0+es" {
		t.Fatalf("rule 3 = %q, want later auxiliary definition %q", got, "0+es")
	}
}

func TestUnapplyDictionaryRuleCommands(t *testing.T) {
	tests := []struct{ word, rule, want string }{
		{"cats", "0+s", "cat"},
		{"cat", "0-s", "cats"},
		{"redo", "0/re", "reredo"},
		{"reredo", "0*re", "redo"},
		{"cafés", "0+s", "café"},
	}
	for _, tc := range tests {
		if got := unapplyDictionaryRule(tc.word, tc.rule); got != tc.want {
			t.Errorf("unapplyDictionaryRule(%q, %q) = %q, want %q", tc.word, tc.rule, got, tc.want)
		}
	}
}
