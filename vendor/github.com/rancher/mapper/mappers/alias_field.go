package mappers

import (
	"github.com/rancher/mapper"
)

type AliasField struct {
	Field string
	Names []string
}

func NewAlias(field string, names ...string) AliasField {
	return AliasField{
		Field: field,
		Names: names,
	}
}

func (d AliasField) FromInternal(data map[string]interface{}) {
}

func (d AliasField) ToInternal(data map[string]interface{}) error {
	for _, name := range d.Names {
		if v, ok := data[name]; ok {
			delete(data, name)
			data[d.Field] = v
		}
	}
	return nil
}

func (d AliasField) ModifySchema(schema *mapper.Schema, schemas *mapper.Schemas) error {
	for _, name := range d.Names {
		schema.ResourceFields[name] = mapper.Field{}
	}

	return ValidateField(d.Field, schema)
}
