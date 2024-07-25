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

// GetValueFromAny retrieves a value from the provided collection, which must be a map[string]interface or a []interface.
// Keys are always strings.
// For a map, a key denotes the key in the map whose value we want to retrieve.
// For the slice, it denotes the index (starting at 0) of the value we want to retrieve.
// Returns the retrieved value (if any) and a bool indicating if the value was found.
func GetValueFromAny(data interface{}, keys ...string) (interface{}, bool) {
	for i, key := range keys {
		if i == len(keys)-1 {
			if dataMap, ok := data.(map[string]interface{}); ok {
				val, ok := dataMap[key]
				return val, ok
			}
			if dataSlice, ok := data.([]interface{}); ok {
				return itemByIndex(dataSlice, key)
			}
		}
		if dataMap, ok := data.(map[string]interface{}); ok {
			data, _ = dataMap[key]
		} else if dataSlice, ok := data.([]interface{}); ok {
			data, _ = itemByIndex(dataSlice, key)
		}
	}

	return nil, false
}

func itemByIndex(dataSlice []interface{}, key string) (interface{}, bool) {
	keyInt, err := strconv.Atoi(key)
	if err != nil {
		return nil, false
	}
	if keyInt >= len(dataSlice) || keyInt < 0 {
		return nil, false
	}
	return dataSlice[keyInt], true
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
