package mappers

import (
	"github.com/docker/go-units"
	types "github.com/rancher/mapper"
	"github.com/rancher/mapper/convert"
)

var abbrs = []string{"", "k", "m", "g", "t", "p"}

type Bytes struct {
	Field string
}

func (d Bytes) FromInternal(data map[string]interface{}) {
	v, ok := data[d.Field]
	if !ok {
		return
	}

	n, err := convert.ToNumber(v)
	if err != nil {
		return
	}

	data[d.Field] = units.CustomSize("%.4g%s", float64(n), 1024.0, abbrs)
}

func (d Bytes) ToInternal(data map[string]interface{}) error {
	v, ok := data[d.Field]
	if !ok {
		return nil
	}

	if str, ok := v.(string); ok {
		sec, err := units.RAMInBytes(str)
		if err != nil {
			return err
		}
		data[d.Field] = sec
	}

	return nil
}

func (d Bytes) ModifySchema(schema *types.Schema, schemas *types.Schemas) error {
	return ValidateField(d.Field, schema)
}
