package mappers

import (
	"github.com/mattn/go-shellwords"
	types "github.com/rancher/mapper"
	"github.com/rancher/mapper/convert"
)

type Shlex struct {
	Field string
}

func (d Shlex) FromInternal(data map[string]interface{}) {
	v, ok := data[d.Field]
	if !ok {
		return
	}

	parts := convert.ToStringSlice(v)
	if len(parts) == 1 {
		data[d.Field] = parts[0]
	}
}

func (d Shlex) ToInternal(data map[string]interface{}) error {
	v, ok := data[d.Field]
	if !ok {
		return nil
	}

	if str, ok := v.(string); ok {
		parts, err := shellwords.Parse(str)
		if err != nil {
			return err
		}
		data[d.Field] = parts
	}

	return nil
}

func (d Shlex) ModifySchema(schema *types.Schema, schemas *types.Schemas) error {
	return ValidateField(d.Field, schema)
}
