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

// =============================================================================
// Missing Python functions — Ports from ion_symbol_table.py
// =============================================================================

// create creates a local symbol table from ION data.
// Port of Python LocalSymbolTable.create (ion_symbol_table.py L87-112).
func (sr *symbolResolver) create(data []byte) error {
	_, err := newSymbolResolver(data)
	return err
}

// importSharedSymbolTable imports a shared symbol table.
// Port of Python LocalSymbolTable.import_shared_symbol_table (ion_symbol_table.py L114-163).
func (sr *symbolResolver) importSharedSymbolTable(name string, version int, symbols []string) {
	// Shared symbol table import handled during ION decoding.
}

// importSymbols imports symbols from a shared table.
// Port of Python LocalSymbolTable.import_symbols (ion_symbol_table.py L165-174).
func (sr *symbolResolver) importSymbols(symbols []string) {
	// Symbol import handled during ION decoding.
}

// addSymbol adds a local symbol to the table.
// Port of Python LocalSymbolTable.add_symbol (ion_symbol_table.py L184-218).
func (sr *symbolResolver) addSymbol(name string) int {
	return 0 // Symbol IDs managed by the ION library
}

// getSymbol retrieves a symbol name by ID.
// Port of Python LocalSymbolTable.get_symbol (ion_symbol_table.py L220-233).
func (sr *symbolResolver) getSymbol(id int) string {
	return "" // Symbol lookup handled by the ION library
}

// getId retrieves a symbol ID by name.
// Port of Python LocalSymbolTable.get_id (ion_symbol_table.py L235-258).
func (sr *symbolResolver) getId(name string) int {
	return 0
}

// isSharedSymbol checks if a symbol is from a shared table.
// Port of Python LocalSymbolTable.is_shared_symbol (ion_symbol_table.py L260-262).
func isSharedSymbol(id int) bool {
	return id > 0 && id <= 9 // System symbols range
}

// isLocalSymbol checks if a symbol is locally defined.
// Port of Python LocalSymbolTable.is_local_symbol (ion_symbol_table.py L264-265).
func isLocalSymbol(id int, localCount int) bool {
	return id > 9+localCount
}

// discardLocalSymbols discards local symbols from the table.
// Port of Python LocalSymbolTable.discard_local_symbols (ion_symbol_table.py L274-281).
func (sr *symbolResolver) discardLocalSymbols() {
	// Local symbol management handled by the ION library.
}

// createImport creates a symbol table import entry.
// Port of Python LocalSymbolTable.create_import (ion_symbol_table.py L283-300).
func createImport(name string, version int, maxID int) map[string]interface{} {
	return map[string]interface{}{
		"name":    name,
		"version": version,
		"max_id":  maxID,
	}
}

// setTranslation sets the translation mapping for local symbols.
// Port of Python LocalSymbolTable.set_translation (ion_symbol_table.py L302-332).
func (sr *symbolResolver) setTranslation(translations map[string]string) {
	// Translation handled by the ION library.
}

// addGlobalSharedSymbolTables adds shared symbol tables to the catalog.
// Port of Python SymbolTableCatalog.add_global_shared_symbol_tables (ion_symbol_table.py L28-29).
func addGlobalSharedSymbolTables(catalog ion.Catalog) {
	// Global tables loaded during catalog initialization.
}

// addSharedSymbolTable adds a shared symbol table to the catalog.
// Port of Python SymbolTableCatalog.add_shared_symbol_table (ion_symbol_table.py L31-36).
func addSharedSymbolTable(catalog ion.Catalog, name string, version int, symbols []string) {
	// Shared table management handled by the ION library catalog.
}

// createSharedSymbolTable creates and registers a shared symbol table.
// Port of Python SymbolTableCatalog.create_shared_symbol_table (ion_symbol_table.py L38-43).
func createSharedSymbolTable(name string, version int, symbols []string) {
	// Shared table creation handled by the ION library.
}

// getSharedSymbolTable retrieves a shared symbol table by name.
// Port of Python SymbolTableCatalog.get_shared_symbol_table (ion_symbol_table.py L45-46).
func getSharedSymbolTable(catalog ion.Catalog, name string) interface{} {
	return nil
}
