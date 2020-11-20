package mappers

import "github.com/rancher/mapper"

type DefaultMapper struct {
	Field string
}

func (d DefaultMapper) FromInternal(data map[string]interface{}) {
}

func (d DefaultMapper) ToInternal(data map[string]interface{}) error {
	return nil
}

func (d DefaultMapper) ModifySchema(schema *mapper.Schema, schemas *mapper.Schemas) error {
	if d.Field != "" {
		return ValidateField(d.Field, schema)
	}
	return nil
}
