package kfx

import (
	"bytes"
	"fmt"
	_ "embed"
	"sync"

	"github.com/amazon-ion/ion-go/ion"
)

// ---------------------------------------------------------------------------
// Port of yj_symbol_catalog.py: YJ_SYMBOLS shared symbol table and prelude.
// See ion_symbol_table.go for the LocalSymbolTable / symbolResolver.
//
// The real symbol names are parsed at init time from catalog.ion (ION text
// format), which contains the YJ_symbols shared symbol table definition with
// human-readable names extracted from Amazon's Kindle Previewer.
//
// Source: REFERENCE/kfx_symbol_catalog.ion
// ---------------------------------------------------------------------------

//go:embed catalog.ion
var catalogION []byte

var (
	yjSymbolsOnce sync.Once
	yjSymbolsData []string
	yjSymbolsErr  error
)

// parseYJSymbols reads the embedded ION text catalog and extracts the YJ_symbols list.
func parseYJSymbols() ([]string, error) {
	reader := ion.NewReaderString(string(catalogION))
	for reader.Next() {
		// The catalog contains a single $ion_shared_symbol_table annotated struct
		if reader.Type() != ion.StructType {
			continue
		}
		reader.StepIn()
		for reader.Next() {
			fieldName, err := reader.FieldName()
			if err != nil || fieldName == nil || fieldName.Text == nil {
				continue
			}
			if *fieldName.Text != "symbols" || reader.Type() != ion.ListType {
				continue
			}
			var symbols []string
			reader.StepIn()
			for reader.Next() {
				if reader.Type() == ion.StringType {
					s, err := reader.StringValue()
					if err == nil && s != nil {
						symbols = append(symbols, *s)
					}
				}
			}
			reader.StepOut()
			return symbols, nil
		}
		reader.StepOut()
	}
	if err := reader.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

func yjSymbols() ([]string, error) {
	yjSymbolsOnce.Do(func() {
		yjSymbolsData, yjSymbolsErr = parseYJSymbols()
	})
	return yjSymbolsData, yjSymbolsErr
}

// sharedSymbolSet returns a set of all YJ shared symbol names for quick lookup.
// Used by isSharedSymbolName() to identify shared symbols without the $N prefix.
var sharedSymbolSetOnce sync.Once
var sharedSymbolSetData map[string]bool

func sharedSymbolSet() map[string]bool {
	sharedSymbolSetOnce.Do(func() {
		syms, err := yjSymbols()
		if err != nil || len(syms) == 0 {
			panic("kfx: failed to parse embedded YJ symbol catalog: " + err.Error())
		}
		sharedSymbolSetData = make(map[string]bool, len(syms))
		for _, s := range syms {
			sharedSymbolSetData[s] = true
		}
	})
	return sharedSymbolSetData
}

// isSharedSymbolName checks whether a name is a YJ shared symbol.
// Replaces the old strings.HasPrefix(name, "$") heuristic.
func isSharedSymbolName(name string) bool {
	return sharedSymbolSet()[name]
}

func sharedCatalog() ion.Catalog {
	return ion.NewCatalog(sharedTable())
}

func sharedTable() ion.SharedSymbolTable {
	syms, err := yjSymbols()
	if err != nil || len(syms) == 0 {
		// Fallback: this should never happen with a valid embedded catalog
		panic("kfx: failed to parse embedded YJ symbol catalog: " + err.Error())
	}
	// Extend to SID 1000 (991 entries total) to match the original $N range.
	// SIDs beyond the catalog (852-1000) get $N placeholder names.
	for len(syms) < 991 {
		syms = append(syms, fmt.Sprintf("$%d", len(syms)+10))
	}
	return ion.NewSharedSymbolTable("YJ_symbols", 10, syms)
}

var (
	yjPreludeOnce sync.Once
	yjPreludeData []byte
	yjPreludeErr  error
)

func yjPrelude() ([]byte, error) {
	yjPreludeOnce.Do(func() {
		var buf bytes.Buffer
		writer := ion.NewBinaryWriter(&buf)
		lst := ion.NewLocalSymbolTable([]ion.SharedSymbolTable{sharedTable()}, nil)
		yjPreludeErr = lst.WriteTo(writer)
		if yjPreludeErr == nil {
			yjPreludeErr = writer.Finish()
		}
		if yjPreludeErr == nil {
			yjPreludeData = buf.Bytes()
		}
	})
	return yjPreludeData, yjPreludeErr
}
