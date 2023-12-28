package util

// GetValue retrieve value from map[string]interface{} by keys
// example: GetValue(map[string]interface{}{"a": map[string]interface{}{"b": "c"}}, "a", "b") -> "c"
func GetValue(data map[string]interface{}, keys ...string) (interface{}, bool) {
	for i, key := range keys {
		if i == len(keys)-1 {
			val, ok := data[key]
			return val, ok
		}
		data, _ = data[key].(map[string]interface{})
	}

	return nil, false
}
