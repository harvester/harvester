package util

// GetValue retrieves a value from a nested map[string]interface{} by a sequence of keys.
// example: GetValue(map[string]interface{}{"a": map[string]interface{}{"b": "c"}}, "a", "b") -> "c", true
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

// PutValue sets a value in a nested map[string]interface{} by a sequence of keys,
// creating intermediate maps as needed. If an intermediate key exists but is not a map, the call is a no-op.
// example: PutValue(data, "c", "a", "b") sets data["a"]["b"] = "c"
func PutValue(data map[string]interface{}, val interface{}, keys ...string) {
	if data == nil {
		return
	}

	for i, key := range keys {
		if i == len(keys)-1 {
			data[key] = val
		} else {
			newData, ok := data[key]
			if ok {
				newMap, ok := newData.(map[string]interface{})
				if ok {
					data = newMap
				} else {
					return
				}
			} else {
				newMap := map[string]interface{}{}
				data[key] = newMap
				data = newMap
			}
		}
	}
}
