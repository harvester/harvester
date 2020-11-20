package mappers

import (
	types "github.com/rancher/mapper"
	"github.com/rancher/mapper/convert"
)

type SingleSlice struct {
	Field           string
	DontForceString bool
}

func (d SingleSlice) FromInternal(data map[string]interface{}) {
	if d.DontForceString {
		return
	}

	v, ok := data[d.Field]
	if !ok {
		return
	}

	ss := convert.ToInterfaceSlice(v)
	if len(ss) == 1 {
		if _, ok := ss[0].(string); ok {
			data[d.Field] = ss[0]
		}
	}
}

func (d SingleSlice) ToInternal(data map[string]interface{}) error {
	v, ok := data[d.Field]
	if !ok {
		return nil
	}

	if str, ok := v.(string); ok {
		data[d.Field] = []interface{}{str}
	}

	return nil
}

func (d SingleSlice) ModifySchema(schema *types.Schema, schemas *types.Schemas) error {
	return ValidateField(d.Field, schema)
}
