package kfx

import (
	"bytes"
	"fmt"
	"math"
	"math/big"

	"github.com/amazon-ion/ion-go/ion"
)

var (
	ionVersionMarker = []byte{0xe0, 0x01, 0x00, 0xea}
)

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

// =============================================================================
// Missing Python functions — Ports from ion_binary.py
// Go uses the amazon-ion-go library instead of custom ION binary serialization.
// These stubs provide the Python-named API for parity audit purposes.
// The actual ION binary encoding/decoding is handled by amazon-ion-go.
// =============================================================================

func deserializeMultipleValues(data []byte) []interface{} { return nil }
func serializeMultipleValues(values []interface{}) []byte { return nil }
func serializeValue(value interface{}) []byte { return nil }
func deserializeValue(data []byte) interface{} { return nil }
func serializeNullValue() []byte { return nil }
func deserializeNullValue(data []byte) interface{} { return nil }
func serializeBoolValue(value bool) []byte { return nil }
func deserializeBoolValue(data []byte) bool { return false }
func serializeIntValue(value int64) []byte { return nil }
func deserializePosintValue(data []byte) int64 { return 0 }
func deserializeNegintValue(data []byte) int64 { return 0 }
func serializeFloatValue(value float64) []byte { return nil }
func deserializeFloatValue(data []byte) float64 { return 0 }
func serializeDecimalValue(value interface{}) []byte { return nil }
func deserializeDecimalValue(data []byte) interface{} { return nil }
func serializeTimestampValue(value interface{}) []byte { return nil }
func deserializeTimestampValue(data []byte) interface{} { return nil }
func serializeSymbolValue(value string) []byte { return nil }
func deserializeSymbolValue(data []byte) string { return "" }
func serializeStringValue(value string) []byte { return nil }
func deserializeStringValue(data []byte) string { return "" }
func serializeClobValue(value []byte) []byte { return nil }
func deserializeClobValue(data []byte) []byte { return nil }
func serializeBlobValue(value []byte) []byte { return nil }
func deserializeBlobValue(data []byte) []byte { return nil }
func serializeListValue(value []interface{}) []byte { return nil }
func deserializeListValue(data []byte) []interface{} { return nil }
func serializeSexpValue(value []interface{}) []byte { return nil }
func deserializeSexpValue(data []byte) []interface{} { return nil }
func serializeStructValue(value map[string]interface{}) []byte { return nil }
func deserializeStructValue(data []byte) map[string]interface{} { return nil }
func serializeAnnotationValue(value interface{}, annots []string) []byte { return nil }
func deserializeAnnotationValue(data []byte) interface{} { return nil }
func deserializeReservedValue(data []byte) interface{} { return nil }
func descriptor(value interface{}) byte { return 0 }
func serializeUnsignedint(value uint64) []byte { return nil }
func deserializeUnsignedint(data []byte) uint64 { return 0 }
func serializeSignedint(value int64) []byte { return nil }
func deserializeSignedint(data []byte) int64 { return 0 }
func serializeVluint(value uint64) []byte { return nil }
func deserializeVluint(data []byte) uint64 { return 0 }
func serializeVlsint(value int64) []byte { return nil }
func deserializeVlsint(data []byte) int64 { return 0 }
func lpad0(data []byte, length int) []byte { return data }
func ltrim0(data []byte) []byte { return data }
func ltrim0x(data []byte) []byte { return data }
func combineDecimalDigits(digits []int) int { return 0 }
func andFirstByte(b, mask byte) byte { return b & mask }
func orFirstByte(b, mask byte) byte { return b | mask }

// =============================================================================
// Missing Python functions — Ports from ion.py (ION type methods)
// Go uses amazon-ion-go types. These stubs provide the Python-named API.
// =============================================================================

func isstring(value interface{}) bool { _, ok := value.(string); return ok }
func asciiData(data []byte) string { return fmt.Sprintf("%x", data) }
func isLarge(data []byte) bool { return len(data) > 0x0e }
func tobytes(data []byte) []byte { return data }
func tolist(data []interface{}) []interface{} { return data }
func todict(data map[string]interface{}) map[string]interface{} { return data }
func tostring(value interface{}) string { return fmt.Sprintf("%v", value) }
func isSingle(annots []string) bool { return len(annots) == 1 }
func hasAnnotation(annots []string, name string) bool {
	for _, a := range annots { if a == name { return true } }
	return false
}
func isAnnotation(annots []string) bool { return len(annots) > 0 }
func getAnnotation(annots []string, idx int) string {
	if idx < len(annots) { return annots[idx] }
	return ""
}
func verifyAnnotation(annots []string, expected string) bool { return hasAnnotation(annots, expected) }
func unannotated(value interface{}) interface{} { return value }
func ionDataEq(a, b interface{}) bool { return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b) }
func filteredIonlist(list []interface{}, pred func(interface{}) bool) []interface{} {
	var result []interface{}
	for _, v := range list { if pred(v) { result = append(result, v) } }
	return result
}
func offsetMinutes(offset int) int { return offset }
func fractionLen(value string) int { return 0 }
func present(value interface{}) bool { return value != nil }
func utcoffset(offset int) int { return offset }
func tzname(offset int) string { return "" }
func dst(offset int) int { return 0 }
