package mappers

import (
	"fmt"

	types "github.com/rancher/mapper"
)

type MapToSlice struct {
	Field string
	Sep   string
}

func (d MapToSlice) FromInternal(data map[string]interface{}) {
}

func (d MapToSlice) ToInternal(data map[string]interface{}) error {
	v, ok := data[d.Field]
	if !ok {
		return nil
	}

	if m, ok := v.(map[string]interface{}); ok {
		var result []interface{}
		for k, v := range m {
			result = append(result, fmt.Sprintf("%s%s%v", k, d.Sep, v))
		}
		data[d.Field] = result
	}

	return nil
}

func (d MapToSlice) ModifySchema(schema *types.Schema, schemas *types.Schemas) error {
	return ValidateField(d.Field, schema)
}
