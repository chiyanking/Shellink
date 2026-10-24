package protocol

import "sort"

func Object(fields ...any) []KV {
	if len(fields)%2 != 0 {
		panic("protocol.Object requires key/value pairs")
	}
	values := make([]KV, 0, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		values = append(values, KV{Key: fields[i].(string), Val: normalize(fields[i+1])})
	}
	return values
}

func normalize(value any) Value {
	switch item := value.(type) {
	case *string:
		if item == nil {
			return nil
		}
		return *item
	case *int:
		if item == nil {
			return nil
		}
		return int64(*item)
	case *int64:
		if item == nil {
			return nil
		}
		return *item
	case *bool:
		if item == nil {
			return nil
		}
		return *item
	case *float64:
		if item == nil {
			return nil
		}
		return *item
	default:
		return value
	}
}

func MapValue(v Value) map[string]Value {
	result := make(map[string]Value)
	fields, ok := v.([]KV)
	if !ok {
		return result
	}
	for _, field := range fields {
		result[field.Key] = field.Val
	}
	return result
}

func ToJSONValue(v Value) any {
	switch x := v.(type) {
	case []KV:
		result := make(map[string]any, len(x))
		for _, field := range x {
			result[field.Key] = ToJSONValue(field.Val)
		}
		return result
	case []Value:
		result := make([]any, len(x))
		for i, item := range x {
			result[i] = ToJSONValue(item)
		}
		return result
	case uint64:
		return x
	default:
		return x
	}
}

func JSONValue(v any) Value {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		fields := make([]KV, 0, len(keys))
		for _, key := range keys {
			fields = append(fields, KV{Key: key, Val: JSONValue(x[key])})
		}
		return fields
	case []any:
		values := make([]Value, len(x))
		for i, value := range x {
			values[i] = JSONValue(value)
		}
		return values
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case float32:
		return float64(x)
	default:
		return x
	}
}

func StringValue(v Value) (string, bool) {
	x, ok := v.(string)
	return x, ok
}

func IntValue(v Value) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int64:
		return x, true
	case uint64:
		if x <= uint64(^uint64(0)>>1) {
			return int64(x), true
		}
	case float64:
		if x == float64(int64(x)) {
			return int64(x), true
		}
	}
	return 0, false
}

func BoolValue(v Value) (bool, bool) {
	x, ok := v.(bool)
	return x, ok
}
