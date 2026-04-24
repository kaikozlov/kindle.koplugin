package kfx

import (
	"bytes"
	"fmt"

	"github.com/amazon-ion/ion-go/ion"
)

// ---------------------------------------------------------------------------
// Port of ion_symbol_table.py: LocalSymbolTable / symbol resolution.
// Uses yj_symbol_catalog.go's sharedCatalog for YJ symbol table access.
// ---------------------------------------------------------------------------

const ionSystemSymbolCount = 9

type symbolResolver struct {
	localStart uint32
	locals     []string
}

func newSymbolResolver(docSymbols []byte) (*symbolResolver, error) {
	var buf bytes.Buffer
	writer := ion.NewBinaryWriter(&buf)
	if err := writer.WriteInt(0); err != nil {
		return nil, err
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}

	stream := make([]byte, 0, len(docSymbols)+buf.Len())
	stream = append(stream, docSymbols...)
	stream = append(stream, stripIVM(buf.Bytes())...)

	reader := ion.NewReaderCat(bytes.NewReader(stream), sharedCatalog())
	for reader.Next() {
		break
	}
	if err := reader.Err(); err != nil {
		return nil, err
	}
	table := reader.SymbolTable()
	if table == nil {
		return nil, fmt.Errorf("KFX document symbol table is empty")
	}

	maxImportID := uint32(ionSystemSymbolCount)
	for _, imported := range table.Imports() {
		if imported == nil || imported.Name() == "$ion" {
			continue
		}
		maxID := imported.MaxID()
		if maxID > ionSystemSymbolCount {
			maxID -= ionSystemSymbolCount
		}
		maxImportID += uint32(maxID)
	}

	return &symbolResolver{
		localStart: maxImportID + 1,
		locals:     table.Symbols(),
	}, nil
}

func (r *symbolResolver) Resolve(sid uint32) string {
	if sid == 0 {
		return ""
	}
	if r.isLocalSID(sid) {
		return r.locals[sid-r.localStart]
	}
	return fmt.Sprintf("$%d", sid)
}

func (r *symbolResolver) isLocalSID(sid uint32) bool {
	if r == nil {
		return false
	}
	offset := sid - r.localStart
	return sid >= r.localStart && int(offset) < len(r.locals)
}

// isSharedSymbolText checks whether a name is a shared YJ symbol.
// With real names from the catalog, shared symbols have names like "content",
// "font_family", etc. Local symbols are strings from the document's own
// symbol table.
func (r *symbolResolver) isSharedSymbolText(name string) bool {
	if r == nil || name == "" {
		return false
	}
	return isSharedSymbolName(name)
}

// replaceLocalSymbols replaces the local symbol table's symbols with the given sorted list.
// Port of LocalSymbolTable.replace_local_symbols (ion_symbol_table.py L267-269).
// Python: self.discard_local_symbols() + self.import_symbols(new_symbols).
// In Go, the symbolResolver is simpler — just replace the locals slice.
func (r *symbolResolver) replaceLocalSymbols(newSymbols []string) {
	if r == nil {
		return
	}
	r.locals = newSymbols
}

// getLocalSymbols returns the current local symbols.
// Port of LocalSymbolTable.get_local_symbols (ion_symbol_table.py L271-272).
func (r *symbolResolver) getLocalSymbols() []string {
	if r == nil {
		return nil
	}
	return r.locals
}
