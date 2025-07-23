// Package data contains functions for working with unstructured values like []interface or map[string]interface{}.
// It allows reading/writing to these values without having to convert to structured items.
package data

import (
	"strconv"
)

// RemoveValue removes a value from data. Keys should be in order denoting the path to the value in the nested
// structure of the map. For example, passing []string{"metadata", "annotations"} will make the function remove the
// "annotations" key from the "metadata" sub-map. Returns the removed value (if any) and a bool indicating if the value
// was found.
func RemoveValue(data map[string]interface{}, keys ...string) (interface{}, bool) {
	for i, key := range keys {
		if i == len(keys)-1 {
			val, ok := data[key]
			delete(data, key)
			return val, ok
		}
		data, _ = data[key].(map[string]interface{})
	}

	return nil, false
}

func GetValueN(data map[string]interface{}, keys ...string) interface{} {
	val, _ := GetValue(data, keys...)
	return val
}

// GetValue works similar to GetValueFromAny, but can only process maps. Kept this way to avoid breaking changes with
// the previous interface, GetValueFromAny should be used in most cases since that can handle slices as well.
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

// GetValueFromAny retrieves a value from the provided collection, which must be a map[string]interface, []interface, or []string (as a final value)
// Keys are always strings.
// For a map, a key denotes the key in the map whose value we want to retrieve.
// For the slice, it denotes the index (starting at 0) of the value we want to retrieve.
// Returns the retrieved value (if any) and a bool indicating if the value was found.
func GetValueFromAny(data interface{}, keys ...string) (interface{}, bool) {
	if len(keys) == 0 {
		return nil, false
	}
	for _, key := range keys {
		if d2, ok := data.(map[string]interface{}); ok {
			data, ok = d2[key]
			if !ok {
				return nil, false
			}
			continue
		}
		// So it must be an array. Verify the index and then continue
		// with a type-assertion switch block for the two different types of arrays we expect
		keyInt, err := strconv.Atoi(key)
		if err != nil || keyInt < 0 {
			return nil, false
		}
		switch node := data.(type) {
		case []interface{}:
			if keyInt >= len(node) {
				return nil, false
			}
			data = node[keyInt]
		case []string:
			if keyInt >= len(node) {
				return nil, false
			}
			data = node[keyInt]
			// If we're at the end of the keys, we'll return the value at the end of this function
			// Otherwise we'll try to index into the string and hit the default case,
			// and return <nil, false>
			// See the "keys nested too far on a string array" test.
		default:
			return nil, false
		}
	}

	return data, true
}

// PutValue updates the value of a given map at the index specified by keys that denote the path to the value in the
// nested structure of the map. If there is no current entry at a key, a new map is created for that value.
func PutValue(data map[string]interface{}, val interface{}, keys ...string) {
	if data == nil {
		return
	}

	// This is so ugly
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
