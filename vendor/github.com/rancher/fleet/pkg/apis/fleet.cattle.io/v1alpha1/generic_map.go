package v1alpha1

import (
	"encoding/json"
)

type GenericMap struct {
	Data map[string]interface{} `json:"-"`
}

func (in GenericMap) MarshalJSON() ([]byte, error) {
	return json.Marshal(in.Data)
}

func (in *GenericMap) UnmarshalJSON(data []byte) error {
	in.Data = map[string]interface{}{}
	return json.Unmarshal(data, &in.Data)
}

func (in *GenericMap) DeepCopyInto(out *GenericMap) {
	out.Data = make(map[string]interface{}, len(in.Data))
	deepCopyMap(in.Data, out.Data)
}

// deepCopyMap will recursively copy each entry from src into dest, ensuring that nested maps and slices are copied as well
//
// NOTE: The implementation relies on GenericMap's data being loaded from a JSON or YAML via Unmarshal, which produces map[string]interface{} and []interface{} types.
// The recursive copy won't work with other variants as pointers or specific value types.
func deepCopyMap(src, dest map[string]interface{}) {
	for key := range src {
		switch value := src[key].(type) {
		case map[string]interface{}:
			destValue := make(map[string]interface{}, len(value))
			deepCopyMap(value, destValue)
			dest[key] = destValue
		case []interface{}:
			destValue := make([]interface{}, len(value))
			deepCopySlice(value, destValue)
			dest[key] = destValue
		default:
			dest[key] = value
		}
	}
}

// deepCopySlice will recursively copy each entry from src into dest, ensuring that nested maps and slices are copied as well
// It assumes that both slices have the same capacity.
func deepCopySlice(src, dest []interface{}) {
	for i := range src {
		switch value := src[i].(type) {
		case map[string]interface{}:
			destValue := make(map[string]interface{}, len(value))
			deepCopyMap(value, destValue)
			dest[i] = destValue
		case []interface{}:
			destValue := make([]interface{}, len(value))
			deepCopySlice(value, destValue)
			dest[i] = destValue
		default:
			dest[i] = value
		}
	}
}
