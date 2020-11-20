package mappers

import (
	"strings"

	types "github.com/rancher/mapper"
	"github.com/rancher/mapper/convert"
)

type AliasValue struct {
	Field string
	Alias map[string][]string
}

func (d AliasValue) FromInternal(data map[string]interface{}) {
}

func (d AliasValue) ToInternal(data map[string]interface{}) error {
	v, ok := data[d.Field]
	if !ok {
		return nil
	}

	for name, values := range d.Alias {
		for _, value := range values {
			if strings.EqualFold(value, convert.ToString(v)) {
				data[d.Field] = name
			}
		}
	}

	return nil
}

func (d AliasValue) ModifySchema(schema *types.Schema, schemas *types.Schemas) error {
	return ValidateField(d.Field, schema)
}
