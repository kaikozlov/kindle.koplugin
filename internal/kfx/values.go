package kfx

import (
	"fmt"

	"github.com/amazon-ion/ion-go/ion"
)

func asMap(value interface{}) (map[string]interface{}, bool) {
	result, ok := value.(map[string]interface{})
	return result, ok
}

func asSlice(value interface{}) ([]interface{}, bool) {
	result, ok := value.([]interface{})
	return result, ok
}

func asString(value interface{}) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case ion.SymbolToken:
		if typed.Text != nil {
			return *typed.Text, true
		}
		return fmt.Sprintf("$%d", typed.LocalSID), true
	case *ion.SymbolToken:
		if typed == nil {
			return "", false
		}
		if typed.Text != nil {
			return *typed.Text, true
		}
		return fmt.Sprintf("$%d", typed.LocalSID), true
	default:
		return "", false
	}
}

func asInt(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

func asBool(value interface{}) (bool, bool) {
	typed, ok := value.(bool)
	return typed, ok
}

func asIntDefault(value interface{}, defaultVal int) int {
	if v, ok := asInt(value); ok {
		return v
	}
	return defaultVal
}

func toStringSlice(value interface{}) []string {
	items, ok := asSlice(value)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := asString(item); ok {
			result = append(result, text)
		}
	}
	return result
}
