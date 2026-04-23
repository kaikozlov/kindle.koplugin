package kfx

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/amazon-ion/ion-go/ion"
)

// ---------------------------------------------------------------------------
// Port of yj_symbol_catalog.py: YJ_SYMBOLS shared symbol table and prelude.
// See ion_symbol_table.go for the LocalSymbolTable / symbolResolver.
// ---------------------------------------------------------------------------

var (
	yjPreludeOnce sync.Once
	yjPreludeData []byte
	yjPreludeErr  error
)

func sharedCatalog() ion.Catalog {
	return ion.NewCatalog(sharedTable())
}

func sharedTable() ion.SharedSymbolTable {
	symbols := make([]string, 991)
	for sid := 10; sid <= 1000; sid++ {
		symbols[sid-10] = fmt.Sprintf("$%d", sid)
	}
	return ion.NewSharedSymbolTable("YJ_symbols", 10, symbols)
}

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
