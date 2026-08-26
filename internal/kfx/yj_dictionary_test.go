package kfx

import (
	"strings"
	"testing"
)

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
	rules, err := processDictionaryRules(aux, []string{"rules"})
	if err != nil {
		t.Fatal(err)
	}
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
	rules, err := processDictionaryRules(aux, []string{"first", "second"})
	if err != nil {
		t.Fatal(err)
	}
	if got := rules[3]; got != "0+es" {
		t.Fatalf("rule 3 = %q, want later auxiliary definition %q", got, "0+es")
	}
}

func TestProcessDictionaryRulesOuterStructureErrorsAbort(t *testing.T) {
	// Python yj_to_epub_misc.py:495-497 accesses aux[$258] and md[$492]
	// outside the parsing try block. Those structural errors abort conversion.
	tests := []struct {
		name string
		aux  map[string]map[string]interface{}
	}{
		{"missing auxiliary", map[string]map[string]interface{}{}},
		{"missing metadata", map[string]map[string]interface{}{"rules": {}}},
		{"metadata not list", map[string]map[string]interface{}{"rules": {"metadata": "bad"}}},
		{"metadata item not struct", map[string]map[string]interface{}{"rules": {"metadata": []interface{}{42}}}},
		{"metadata item missing key", map[string]map[string]interface{}{"rules": {"metadata": []interface{}{map[string]interface{}{"value": []byte(`[]`)}}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := processDictionaryRules(tc.aux, []string{"rules"}); err == nil {
				t.Fatal("malformed outer dictionary rule structure did not abort")
			}
		})
	}
}

func TestProcessDictionaryRulesPayloadErrorsAreNonFatal(t *testing.T) {
	// Python yj_to_epub_misc.py:498-505 catches failures while reading/parsing
	// md[$307] and the rule payload. A bad matching payload is logged and skipped.
	aux := map[string]map[string]interface{}{
		"rules": {"metadata": []interface{}{
			map[string]interface{}{"key": "yj.dictionary.inflection_rules"},
			map[string]interface{}{"key": "yj.dictionary.inflection_rules", "value": []byte(`not ion`)},
			map[string]interface{}{"key": "yj.dictionary.inflection_rules", "value": []byte(`[{id:4,rule:"0+s"}]`)},
		}},
	}
	rules, err := processDictionaryRules(aux, []string{"rules"})
	if err != nil {
		t.Fatalf("payload parse errors must remain non-fatal: %v", err)
	}
	if got := rules[4]; got != "0+s" {
		t.Fatalf("valid payload after malformed entries was lost: rule 4 = %q", got)
	}
}

func TestUnapplyDictionaryRuleCommands(t *testing.T) {
	tests := []struct{ word, rule, want string }{
		{"cats", "0+s", "cat"},
		{"cat", "0-s", "cats"},
		{"redo", "0/re", "reredo"},
		{"reredo", "0*re", "redo"},
		{"cafés", "0+s", "café"},
		// Python slices clamp an absolute insertion beyond the end.
		{"cat", "99/s", "cats"},
		// Negative end_offset_pos counts from the end for the '-' command.
		{"cat", "4-s", "cast"},
	}
	for _, tc := range tests {
		if got := unapplyDictionaryRule(tc.word, tc.rule); got != tc.want {
			t.Errorf("unapplyDictionaryRule(%q, %q) = %q, want %q", tc.word, tc.rule, got, tc.want)
		}
	}
}

func TestDictionaryEntryFlatRulesRenderIDXMarkup(t *testing.T) {
	// Python yj_to_epub_content.py:542-588: a $270 container carrying
	// yj.dictionary.* becomes idx:entry; with scalar/list rule IDs the LAST
	// dictionary term is the orthography and generated inflections are added
	// under idx:orth before ordinary content.
	r := &storylineRenderer{
		dictionaryRules:     map[int]string{1: "0-s"},
		usedDictionaryRules: map[int]struct{}{},
		styles:              newStyleCatalog(),
	}
	node := map[string]interface{}{
		"type":                "container",
		"content_list":        []interface{}{"definition"},
		"yj.dictionary.term":  []interface{}{"cat"},
		"yj.dictionary.rules": []interface{}{1},
	}
	part := r.renderNode(node, 0)
	if r.renderError != nil {
		t.Fatalf("renderNode: %v", r.renderError)
	}
	got := renderHTMLPart(part)
	wantPrefix := `<idx:entry scriptable="yes"><idx:orth value="cat"><idx:infl><idx:iform value="cats"/></idx:infl></idx:orth>`
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("dictionary markup = %q, want prefix %q", got, wantPrefix)
	}
	if !strings.Contains(got, "definition") {
		t.Fatalf("dictionary entry lost ordinary content: %q", got)
	}
	if _, ok := r.usedDictionaryRules[1]; !ok {
		t.Fatal("dictionary rule 1 was not marked used")
	}
}

func TestDictionaryEntryStructuredRules(t *testing.T) {
	// Python yj_to_epub_content.py:561-578: struct rule form takes the FIRST
	// term as orthography, keeps remaining terms as inflections, then applies
	// each word-specific rule.
	r := &storylineRenderer{
		dictionaryRules:     map[int]string{7: "0-ed"},
		usedDictionaryRules: map[int]struct{}{},
		styles:              newStyleCatalog(),
	}
	working := map[string]interface{}{
		"type":               "container",
		"yj.dictionary.term": []interface{}{"walk", "walks"},
		"yj.dictionary.rules": []interface{}{map[string]interface{}{
			"yj.dictionary.word":  "walk",
			"yj.dictionary.rules": []interface{}{7},
		}},
	}
	spec, err := r.consumeDictionaryEntry(working)
	if err != nil {
		t.Fatal(err)
	}
	if spec.orthography != "walk" {
		t.Fatalf("orthography = %q, want walk", spec.orthography)
	}
	if got, want := strings.Join(spec.inflections, ","), "walks,walked"; got != want {
		t.Fatalf("inflections = %q, want %q", got, want)
	}
	if _, remains := working["yj.dictionary.rules"]; remains {
		t.Fatal("dictionary rules were not consumed from working content")
	}
}

func TestDictionaryEntryMissingTermFailsConversion(t *testing.T) {
	// Python yj_to_epub_content.py:563/572 indexes dictionary_terms[0]/[-1];
	// malformed dictionary metadata without a term raises instead of rendering.
	r := &storylineRenderer{
		dictionaryRules:     map[int]string{},
		usedDictionaryRules: map[int]struct{}{},
		styles:              newStyleCatalog(),
	}
	part := r.renderNode(map[string]interface{}{
		"type":                "container",
		"yj.dictionary.rules": []interface{}{1},
	}, 0)
	if part != nil {
		t.Fatalf("malformed dictionary entry rendered as %q", renderHTMLPart(part))
	}
	if r.renderError == nil {
		t.Fatal("malformed dictionary entry did not fail rendering")
	}
}


func TestDictionaryRulesReachRenderBookState(t *testing.T) {
	// Production-path proof for Python yj_to_epub.py:79 ->
	// yj_to_epub_content.py:542-588: renderBookState must parse $597 rules and
	// make them available to the normal reading-order renderer.
	state := &bookState{
		Book: &decodedBook{Title: "Dictionary", Language: "en", IsDictionary: true},
		BookSymbolFormat: symOriginal,
		Fragments: fragmentCatalog{
			ContentFragments:  map[string][]string{},
			Storylines: map[string]map[string]interface{}{
				"story-1": {
					"story_name": "story-1",
					"content_list": []interface{}{map[string]interface{}{
						"type":                "container",
						"content_list":        []interface{}{"definition"},
						"yj.dictionary.term":  []interface{}{"cat"},
						"yj.dictionary.rules": []interface{}{3},
					}},
				},
			},
			StyleFragments:   map[string]map[string]interface{}{},
			RubyGroups:       map[string]map[string]interface{}{},
			RubyContents:     map[string]map[string]interface{}{},
			SectionFragments: map[string]sectionFragment{
				"section-1": {ID: "section-1", Storyline: "story-1", PageTemplateValues: map[string]interface{}{}},
			},
			AnchorFragments:       map[string]anchorFragment{},
			NavContainers:         map[string]map[string]interface{}{},
			ResourceFragments:     map[string]resourceFragment{},
			ResourceRawData:       map[string]map[string]interface{}{},
			FormatCapabilities:    map[string]map[string]interface{}{},
			Generators:            map[string]map[string]interface{}{},
			PathBundles:           map[string]map[string]interface{}{},
			AuxiliaryData: map[string]map[string]interface{}{
				"dictionary_rules": {"metadata": []interface{}{map[string]interface{}{
					"key": "yj.dictionary.inflection_rules",
					"value": []byte(`[{id:3,rule:"0-s"}]`),
				}}},
			},
			AuxiliaryDataOrder: []string{"dictionary_rules"},
			FontFragments:      map[string]fontFragment{},
			RawFragments:       map[string][]byte{},
			PositionAliases:    map[int]string{},
			SectionOrder:       []string{"section-1"},
			FragmentIDsByType:  map[string][]string{},
		},
	}
	book, err := renderBookState(state, nil)
	if err != nil {
		t.Fatalf("renderBookState: %v", err)
	}
	if len(book.RenderedSections) != 1 {
		t.Fatalf("rendered sections = %d, want 1", len(book.RenderedSections))
	}
	body := renderHTMLParts(book.RenderedSections[0].Root.Children, true)
	if !strings.Contains(body, `<idx:iform value="cats"/>`) {
		t.Fatalf("production dictionary rule did not reach rendered markup: %q", body)
	}
}
