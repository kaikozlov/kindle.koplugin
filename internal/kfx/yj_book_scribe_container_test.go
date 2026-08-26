package kfx

import (
	"bytes"
	"encoding/binary"
	"sort"
	"testing"

	"github.com/amazon-ion/ion-go/ion"
)

// buildTestCONT assembles a genuine CONT KFX container that flows through the
// production loadContainerSourceData → organizeFragments path:
//
//	header (18 bytes) | container_info ION | doc symbols ION | ENTY entities | index table
//
// The shared YJ symbol table is imported; locals carry document-specific
// symbols (fragment ids, nmdl.* names — not shared symbols). Index entity
// offsets are stored relative to the end of the header because
// organizeFragments resolves each entity at HeaderLen + offset.
func buildTestCONT(t *testing.T, locals []string, entities map[string]map[string]interface{}, order []string, typeSIDs map[string]uint32) []byte {
	t.Helper()

	lst := ion.NewLocalSymbolTable([]ion.SharedSymbolTable{sharedTable()}, locals)

	writeION := func(buf *bytes.Buffer, write func(w ion.Writer)) {
		w := ion.NewBinaryWriterLST(buf, lst)
		write(w)
		if err := w.Finish(); err != nil {
			t.Fatal(err)
		}
	}

	fieldName := func(w ion.Writer, name string) {
		tok, err := ion.NewSymbolToken(lst, name)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.FieldName(tok); err != nil {
			t.Fatal(err)
		}
	}

	writeValue := func(w ion.Writer, v interface{}) {
		switch tv := v.(type) {
		case string:
			if err := w.WriteString(tv); err != nil {
				t.Fatal(err)
			}
		case int:
			if err := w.WriteInt(int64(tv)); err != nil {
				t.Fatal(err)
			}
		case float64:
			if err := w.WriteFloat(tv); err != nil {
				t.Fatal(err)
			}
		case []interface{}:
			if err := w.BeginList(); err != nil {
				t.Fatal(err)
			}
			for _, item := range tv {
				if s, ok := item.(string); ok {
					if err := w.WriteSymbolFromString(s); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := w.EndList(); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Document symbol table blob. ion-go emits the LST lazily when the first
	// application value is written, so write a one-byte int-zero sentinel and
	// remove that application value, leaving exactly IVM + local symbol table.
	// decodeIonValue prefixes this blob to each entity and must see the entity
	// struct as the first application value (not our sentinel).
	var docSym bytes.Buffer
	writeION(&docSym, func(w ion.Writer) {
		if err := w.WriteInt(0); err != nil {
			t.Fatal(err)
		}
	})
	docSymBytes := docSym.Bytes()
	if len(docSymBytes) == 0 || docSymBytes[len(docSymBytes)-1] != 0x20 {
		t.Fatalf("unexpected ION int-zero sentinel encoding: %x", docSymBytes)
	}
	docSym.Reset()
	docSym.Write(docSymBytes[:len(docSymBytes)-1])

	// ENTY-wrapped entities, written in the given order.
	var entBuf bytes.Buffer
	entOffsets := map[string]int{}
	entLengths := map[string]int{}
	for _, id := range order {
		enty := make([]byte, 14)
		copy(enty, "ENTY")
		binary.LittleEndian.PutUint32(enty[6:10], 14) // header length
		start := entBuf.Len()
		entBuf.Write(enty)
		writeION(&entBuf, func(w ion.Writer) {
			if err := w.BeginStruct(); err != nil {
				t.Fatal(err)
			}
			for _, k := range sortedStringMapKeys(entities[id]) {
				fieldName(w, k)
				writeValue(w, entities[id][k])
			}
			if err := w.EndStruct(); err != nil {
				t.Fatal(err)
			}
		})
		entOffsets[id] = start
		entLengths[id] = entBuf.Len() - start
	}

	const headerLen = 18
	ciOffset := headerLen

	// container_info only uses shared symbols; measure its stable size with
	// placeholder values, then write it with the real offsets.
	ciValues := func(symOff, symLen, idxOff, idxLen int) map[string]int {
		return map[string]int{
			"bcDocSymbolOffset": symOff,
			"bcDocSymbolLength": symLen,
			"bcIndexTabOffset":  idxOff,
			"bcIndexTabLength":  idxLen,
		}
	}
	writeCIStruct := func(buf *bytes.Buffer, values map[string]int) {
		w := ion.NewBinaryWriter(buf, sharedTable())
		if err := w.BeginStruct(); err != nil {
			t.Fatal(err)
		}
		for _, k := range sortedIntMapKeys(values) {
			tok, err := ion.NewSymbolToken(lst, k)
			if err != nil {
				t.Fatal(err)
			}
			if err := w.FieldName(tok); err != nil {
				t.Fatal(err)
			}
			if err := w.WriteInt(int64(values[k])); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.EndStruct(); err != nil {
			t.Fatal(err)
		}
		if err := w.Finish(); err != nil {
			t.Fatal(err)
		}
	}

	// container_info encodes its four offsets/lengths as ION ints, so its
	// encoded size depends on the values themselves. Iterate to a fixed
	// point: guess ciLen, derive the dependent offsets, encode the real CI,
	// and repeat until the encoded length stabilizes.
	ciLen := 0
	var ciData []byte
	var index bytes.Buffer
	docSymOff, entityRegionStart, indexOffset := 0, 0, 0
	for iter := 0; iter < 16; iter++ {
		docSymOff = headerLen + ciLen
		entityRegionStart = docSymOff + docSym.Len()
		indexOffset = entityRegionStart + entBuf.Len()

		// Index rows depend on entityRegionStart (offsets are relative to
		// HeaderLen: organizeFragments resolves at HeaderLen + offset), so
		// rebuild them each iteration too.
		index.Reset()
		for _, id := range order {
			var row [24]byte
			binary.LittleEndian.PutUint32(row[0:4], localSIDOf(t, lst, id))
			binary.LittleEndian.PutUint32(row[4:8], typeSIDs[id])
			binary.LittleEndian.PutUint64(row[8:16], uint64(entityRegionStart-headerLen+entOffsets[id]))
			binary.LittleEndian.PutUint64(row[16:24], uint64(entLengths[id]))
			index.Write(row[:])
		}

		var ciBuf bytes.Buffer
		writeCIStruct(&ciBuf, ciValues(docSymOff, docSym.Len(), indexOffset, index.Len()))
		encoded := ciBuf.Len()
		if encoded == ciLen {
			ciData = ciBuf.Bytes()
			break
		}
		ciLen = encoded
	}
	if ciData == nil {
		t.Fatal("container_info size did not converge")
	}

	var final bytes.Buffer
	final.Write([]byte("CONT"))
	final.Write([]byte{0, 0})
	binary.Write(&final, binary.LittleEndian, uint32(headerLen))
	binary.Write(&final, binary.LittleEndian, uint32(ciOffset))
	binary.Write(&final, binary.LittleEndian, uint32(ciLen))
	if len(ciData) != ciLen {
		t.Fatalf("container_info length drifted after convergence: %d != %d", len(ciData), ciLen)
	}
	final.Write(ciData)
	final.Write(docSym.Bytes())
	final.Write(entBuf.Bytes())
	final.Write(index.Bytes())
	return final.Bytes()
}

func localSIDOf(t *testing.T, lst ion.SymbolTable, name string) uint32 {
	t.Helper()
	sid, ok := lst.FindByName(name)
	if !ok || sid <= 0 {
		t.Fatalf("symbol %q not resolvable as a local symbol (sid=%d ok=%v)", name, sid, ok)
	}
	return uint32(sid)
}

func sortedStringMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedIntMapKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestOrganizeFragmentsRetainsStorylessSections drives a genuine CONT
// container through loadContainerSourceData → organizeFragments and verifies
// (per Python yj_to_epub.py:196-228, which retains EVERY $260 fragment by id
// with no storyline filter) that sections whose $141 page_templates are
// IonSymbol references are retained with their raw dict, whether or not they
// carry Scribe keys.
func TestOrganizeFragmentsRetainsStorylessSections(t *testing.T) {
	locals := []string{
		"sect-scribe", "sect-plain", "pt-ref",
		"section_name", "page_templates",
		"nmdl.canvas_width", "nmdl.canvas_height", "nmdl.normalized_ppi",
		"nmdl.template_id", "nmdl.template_type",
	}
	entities := map[string]map[string]interface{}{
		// Scribe page: symbol page template → no parsed storyline.
		"sect-scribe": {
			"section_name":        "sect-scribe",
			"nmdl.canvas_width":   15624,
			"nmdl.canvas_height":  20832,
			"nmdl.normalized_ppi": 2520,
			"page_templates":      []interface{}{"pt-ref"},
		},
		// NON-Scribe section, also with a symbol page template and no other
		// keys — the regression that a storyline predicate would false-drop.
		"sect-plain": {
			"section_name":   "sect-plain",
			"page_templates": []interface{}{"pt-ref"},
		},
	}
	order := []string{"sect-scribe", "sect-plain"}
	typeSIDs := map[string]uint32{"sect-scribe": 260, "sect-plain": 260} // $260 section type SID

	data := buildTestCONT(t, locals, entities, order, typeSIDs)
	src, err := loadContainerSourceData("scribe-cont", data)
	if err != nil {
		t.Fatalf("loadContainerSourceData: %v", err)
	}
	state, err := organizeFragments("scribe-cont", []*containerSource{src})
	if err != nil {
		t.Fatalf("organizeFragments: %v", err)
	}

	for _, id := range order {
		section, ok := state.Fragments.SectionFragments[id]
		if !ok {
			t.Errorf("section %q was dropped by the organizer (Python retains all $260 fragments)", id)
			continue
		}
		if section.Storyline != "" {
			t.Errorf("section %q unexpectedly parsed a storyline %q", id, section.Storyline)
		}
		if section.RawValue == nil {
			t.Errorf("section %q lost its raw dict", id)
		}
	}

	// Sanity: the scribe branch keys survive in the raw dict for dispatch.
	scribe := state.Fragments.SectionFragments["sect-scribe"]
	if _, ok := scribe.RawValue["nmdl.canvas_width"]; !ok {
		t.Error("scribe section raw dict lost nmdl.canvas_width")
	}
	if determineSectionBranch(scribe, bookTypeNotebook) != branchScribePage {
		t.Errorf("scribe section branch = %v, want branchScribePage", determineSectionBranch(scribe, bookTypeNotebook))
	}
}

// TestNmdlTemplateIDDoesNotMakeNotebook proves through the real
// organizeFragments → renderBookState path that a document_data
// nmdl.template_id alone does NOT mark the book as a Scribe notebook
// (upstream sets is_scribe_notebook only from the KPF/KDF schema,
// kpf_container.py:148-163). A plain CONT book with scribe-shaped sections
// keeps its normal (non-notebook) pipeline: no forced fixed layout, no
// fallback notebook title.
func TestNmdlTemplateIDDoesNotMakeNotebook(t *testing.T) {
	locals := []string{
		"sect-a", "pt-ref", "section_name", "page_templates",
		"nmdl.template_id", "reading_orders", "reading_order_name", "sections",
		"doc-data",
	}
	entities := map[string]map[string]interface{}{
		"sect-a": {
			"section_name":   "sect-a",
			"page_templates": []interface{}{"pt-ref"},
		},
		// document_data fragment ($538 = SID 538).
		"doc-data": {
			"nmdl.template_id": "some-template",
		},
	}
	order := []string{"sect-a", "doc-data"}
	typeSIDs := map[string]uint32{"sect-a": 260, "doc-data": 538}

	data := buildTestCONT(t, locals, entities, order, typeSIDs)
	src, err := loadContainerSourceData("plain-cont", data)
	if err != nil {
		t.Fatalf("loadContainerSourceData: %v", err)
	}
	state, err := organizeFragments("plain-cont", []*containerSource{src})
	if err != nil {
		t.Fatalf("organizeFragments: %v", err)
	}

	applyKFXEPUBInitMetadataAfterOrganize(state.Book, &state.Fragments)
	if state.Book.IsScribeNotebook {
		t.Error("nmdl.template_id in document_data must NOT set IsScribeNotebook")
	}
	if state.Book.FixedLayout {
		t.Error("plain CONT book must not be forced into fixed layout")
	}
	if strings := state.Book.Title; len(strings) > 0 && strings[:1] == "N" && len(strings) > 8 && strings[:9] == "Notebook " {
		t.Errorf("plain CONT book got the notebook fallback title %q", strings)
	}
}
