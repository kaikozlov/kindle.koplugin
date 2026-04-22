package kfx

import (
	"bytes"
	"fmt"
	"strconv"
	"sync"

	"github.com/amazon-ion/ion-go/ion"
)

var (
	yjPreludeOnce sync.Once
	yjPreludeData []byte
	yjPreludeErr  error
)

const ionSystemSymbolCount = 9

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

// isSharedSymbolText mirrors symtab.is_shared_symbol for text form "$<sid>" (yj_structure.py / Ion locals).
func (r *symbolResolver) isSharedSymbolText(name string) bool {
	if r == nil || name == "" || name[0] != '$' {
		return false
	}
	for _, ch := range name[1:] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	sid64, err := strconv.ParseUint(name[1:], 10, 32)
	if err != nil || sid64 == 0 {
		return false
	}
	sid := uint32(sid64)
	if r.isLocalSID(sid) {
		return false
	}
	return sid < r.localStart
}
