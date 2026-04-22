package kfx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

	"github.com/amazon-ion/ion-go/ion"
)

var (
	ionVersionMarker = []byte{0xe0, 0x01, 0x00, 0xea}
)

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
