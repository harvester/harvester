package mappers

import (
	types "github.com/rancher/mapper"
	"github.com/rancher/mapper/convert"
)

type JSONKeys struct {
}

func (d JSONKeys) FromInternal(data map[string]interface{}) {
}

func (d JSONKeys) ToInternal(data map[string]interface{}) error {
	for key, value := range data {
		newKey := convert.ToJSONKey(key)
		if newKey != key {
			data[newKey] = value
			delete(data, key)
		}
	}
	return nil
}

func (d JSONKeys) ModifySchema(schema *types.Schema, schemas *types.Schemas) error {
	return nil
}
