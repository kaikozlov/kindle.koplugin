package kfx

import (
	"bytes"
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

func sharedCatalog() ion.Catalog {
	return ion.NewCatalog(sharedTable())
}

func sharedTable() ion.SharedSymbolTable {
	syms, err := yjSymbols()
	if err != nil || len(syms) == 0 {
		// Fallback: this should never happen with a valid embedded catalog
		panic("kfx: failed to parse embedded YJ symbol catalog: " + err.Error())
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
