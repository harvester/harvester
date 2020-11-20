package mappers

import (
	types "github.com/rancher/mapper"
)

type SetValue struct {
	Field         string
	InternalValue interface{}
	ExternalValue interface{}
}

func (d SetValue) FromInternal(data map[string]interface{}) {
	if d.ExternalValue != nil {
		data[d.Field] = d.ExternalValue
	}
}

func (d SetValue) ToInternal(data map[string]interface{}) error {
	if d.InternalValue != nil {
		data[d.Field] = d.InternalValue
	}
	return nil
}

func (d SetValue) ModifySchema(schema *types.Schema, schemas *types.Schemas) error {
	return ValidateField(d.Field, schema)
}
