package config

import (
	"fmt"

	"github.com/rancher/mapper"
	"github.com/rancher/mapper/convert"
	"github.com/rancher/mapper/mappers"
)

type Converter func(val interface{}) (interface{}, error)

type fieldConverter struct {
	mappers.DefaultMapper
	fieldName string
	converter Converter
}

func (f fieldConverter) ToInternal(data map[string]interface{}) error {
	val, ok := data[f.fieldName]
	if !ok {
		return nil
	}
	var err error
	data[f.fieldName], err = f.converter(val)
	if err != nil {
		return fmt.Errorf("failed to convert field %q: %w", f.fieldName, err)
	}
	return nil
}

type typeConverter struct {
	mappers.DefaultMapper
	converter Converter
	fieldType string
	mappers   mapper.Mappers
}

func (t *typeConverter) ToInternal(data map[string]interface{}) error {
	return t.mappers.ToInternal(data)
}

func (t *typeConverter) ModifySchema(schema *mapper.Schema, _ *mapper.Schemas) error {
	for name, field := range schema.ResourceFields {
		if field.Type == t.fieldType {
			t.mappers = append(t.mappers, fieldConverter{
				fieldName: name,
				converter: t.converter,
			})
		}
	}
	return nil
}

func NewTypeConverter(fieldType string, converter Converter) mapper.Mapper {
	return &typeConverter{
		fieldType: fieldType,
		converter: converter,
	}
}

func NewToMap() mapper.Mapper {
	return NewTypeConverter("map[string]", func(val interface{}) (interface{}, error) {
		if m, ok := val.(map[string]interface{}); ok {
			obj := make(map[string]string, len(m))
			for k, v := range m {
				obj[k] = convert.ToString(v)
			}
			return obj, nil
		}
		return val, nil
	})
}

func NewToSlice() mapper.Mapper {
	return NewTypeConverter("array[string]", func(val interface{}) (interface{}, error) {
		if str, ok := val.(string); ok {
			return []string{str}, nil
		}
		return convert.ToStringSlice(val), nil
	})
}

func NewToBool() mapper.Mapper {
	return NewTypeConverter("boolean", func(val interface{}) (interface{}, error) {
		return convert.ToBool(val), nil
	})
}

func NewToInt() mapper.Mapper {
	return NewTypeConverter("int", func(val interface{}) (interface{}, error) {
		if val == nil || val == "nil" { // Allow nil values to pass through unchanged.
			return nil, nil
		}
		return convert.ToNumber(val)
	})
}

func NewToFloat() mapper.Mapper {
	return NewTypeConverter("float", func(val interface{}) (interface{}, error) {
		return convert.ToFloat(val)
	})
}
