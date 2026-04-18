package kfx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"sync"

	"github.com/amazon-ion/ion-go/ion"
)

var (
	ionVersionMarker = []byte{0xe0, 0x01, 0x00, 0xea}
	yjPreludeOnce    sync.Once
	yjPreludeData    []byte
	yjPreludeErr     error
)

const ionSystemSymbolCount = 9

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

func entityPayload(data []byte) ([]byte, error) {
	if len(data) < 10 || string(data[:4]) != "ENTY" {
		return nil, &UnsupportedError{Message: "entity wrapper is invalid"}
	}
	headerLen := int(binary.LittleEndian.Uint32(data[6:10]))
	if headerLen < 10 || headerLen > len(data) {
		return nil, &UnsupportedError{Message: "entity header length is invalid"}
	}
	return data[headerLen:], nil
}

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

func decodeIonMap(data []byte, docSymbols []byte, resolver *symbolResolver) (map[string]interface{}, error) {
	value, err := decodeIonValue(data, docSymbols, resolver)
	if err != nil {
		return nil, err
	}

	mapped, ok := value.(map[string]interface{})
	if !ok {
		return nil, &UnsupportedError{Message: "decoded Ion fragment is not a struct"}
	}
	return mapped, nil
}

func decodeIonValue(data []byte, docSymbols []byte, resolver *symbolResolver) (interface{}, error) {
	stream := data
	prefix := docSymbols
	if len(prefix) == 0 {
		var err error
		prefix, err = yjPrelude()
		if err != nil {
			return nil, &UnsupportedError{Message: err.Error()}
		}
	}
	if len(prefix) > 0 {
		stream = make([]byte, 0, len(prefix)+len(data))
		stream = append(stream, prefix...)
		stream = append(stream, stripIVM(data)...)
	}

	decoder := ion.NewDecoder(ion.NewReaderCat(bytes.NewReader(stream), sharedCatalog()))
	value, err := decoder.Decode()
	if err != nil {
		return nil, &UnsupportedError{Message: err.Error()}
	}

	return normalizeIon(value, resolver), nil
}

func stripIVM(data []byte) []byte {
	if bytes.HasPrefix(data, ionVersionMarker) {
		return data[len(ionVersionMarker):]
	}
	return data
}

func normalizeIon(value interface{}, resolver *symbolResolver) interface{} {
	switch typed := value.(type) {
	case *string:
		if typed == nil {
			return ""
		}
		return *typed
	case *ion.SymbolToken:
		if typed == nil {
			return ""
		}
		if resolver != nil && resolver.isLocalSID(uint32(typed.LocalSID)) {
			return resolver.Resolve(uint32(typed.LocalSID))
		}
		if typed.Text != nil {
			return *typed.Text
		}
		if resolver != nil {
			return resolver.Resolve(uint32(typed.LocalSID))
		}
		return fmt.Sprintf("$%d", typed.LocalSID)
	case ion.SymbolToken:
		if resolver != nil && resolver.isLocalSID(uint32(typed.LocalSID)) {
			return resolver.Resolve(uint32(typed.LocalSID))
		}
		if typed.Text != nil {
			return *typed.Text
		}
		if resolver != nil {
			return resolver.Resolve(uint32(typed.LocalSID))
		}
		return fmt.Sprintf("$%d", typed.LocalSID)
	case *ion.Decimal:
		if typed == nil {
			return float64(0)
		}
		return ionDecimalToFloat64(typed)
	case map[string]interface{}:
		result := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			result[key] = normalizeIon(item, resolver)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(typed))
		for index, item := range typed {
			result[index] = normalizeIon(item, resolver)
		}
		return result
	default:
		return typed
	}
}

// ionDecimalToFloat64 converts an ion.Decimal to float64 using big.Float
// for maximum precision. CSS values don't require arbitrary precision,
// but we use big.Float to avoid unnecessary rounding artifacts.
func ionDecimalToFloat64(d *ion.Decimal) float64 {
	coeff, exp := d.CoEx()
	bf := new(big.Float).SetInt(coeff)
	if exp != 0 {
		pow := new(big.Float).SetFloat64(math.Pow10(int(exp)))
		bf.Mul(bf, pow)
	}
	result, _ := bf.Float64()
	return result
}
