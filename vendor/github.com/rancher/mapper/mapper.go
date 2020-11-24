package mapper

import (
	"github.com/rancher/mapper/convert"
	"github.com/rancher/mapper/definition"
)

type Mapper interface {
	FromInternal(data map[string]interface{})
	ToInternal(data map[string]interface{}) error
	ModifySchema(schema *Schema, schemas *Schemas) error
}

type Mappers []Mapper

func (m Mappers) FromInternal(data map[string]interface{}) {
	for _, mapper := range m {
		mapper.FromInternal(data)
	}
}

func (m Mappers) ToInternal(data map[string]interface{}) error {
	var errors []error
	for i := len(m) - 1; i >= 0; i-- {
		errors = append(errors, m[i].ToInternal(data))
	}
	return NewErrors(errors...)
}

func (m Mappers) ModifySchema(schema *Schema, schemas *Schemas) error {
	for _, mapper := range m {
		if err := mapper.ModifySchema(schema, schemas); err != nil {
			return err
		}
	}
	return nil
}

type typeMapper struct {
	Mappers         []Mapper
	root            bool
	typeName        string
	subSchemas      map[string]*Schema
	subArraySchemas map[string]*Schema
	subMapSchemas   map[string]*Schema
}

func (t *typeMapper) FromInternal(data map[string]interface{}) {
	for fieldName, schema := range t.subSchemas {
		if schema.Mapper == nil {
			continue
		}
		fieldData, _ := data[fieldName].(map[string]interface{})
		schema.Mapper.FromInternal(fieldData)
	}

	for fieldName, schema := range t.subMapSchemas {
		if schema.Mapper == nil {
			continue
		}
		datas, _ := data[fieldName].(map[string]interface{})
		for _, fieldData := range datas {
			mapFieldData, _ := fieldData.(map[string]interface{})
			schema.Mapper.FromInternal(mapFieldData)
		}
	}

	for fieldName, schema := range t.subArraySchemas {
		if schema.Mapper == nil {
			continue
		}
		datas, _ := data[fieldName].([]interface{})
		for _, fieldData := range datas {
			mapFieldData, _ := fieldData.(map[string]interface{})
			schema.Mapper.FromInternal(mapFieldData)
		}
	}

	Mappers(t.Mappers).FromInternal(data)
}

func (t *typeMapper) ToInternal(data map[string]interface{}) error {
	errors := Errors{}
	errors = append(errors, Mappers(t.Mappers).ToInternal(data))

	for fieldName, schema := range t.subArraySchemas {
		if schema.Mapper == nil {
			continue
		}
		datas, _ := data[fieldName].([]interface{})
		for _, fieldData := range datas {
			errors = append(errors, schema.Mapper.ToInternal(convert.ToMapInterface(fieldData)))
		}
	}

	for fieldName, schema := range t.subMapSchemas {
		if schema.Mapper == nil {
			continue
		}
		datas, _ := data[fieldName].(map[string]interface{})
		for _, fieldData := range datas {
			errors = append(errors, schema.Mapper.ToInternal(convert.ToMapInterface(fieldData)))
		}
	}

	for fieldName, schema := range t.subSchemas {
		if schema.Mapper == nil {
			continue
		}
		fieldData, _ := data[fieldName].(map[string]interface{})
		errors = append(errors, schema.Mapper.ToInternal(fieldData))
	}

	return errors.Err()
}

func (t *typeMapper) ModifySchema(schema *Schema, schemas *Schemas) error {
	t.subSchemas = map[string]*Schema{}
	t.subArraySchemas = map[string]*Schema{}
	t.subMapSchemas = map[string]*Schema{}
	t.typeName = schema.ID

	mapperSchema := schema
	if schema.InternalSchema != nil {
		mapperSchema = schema.InternalSchema
	}
	for name, field := range mapperSchema.ResourceFields {
		fieldType := field.Type
		targetMap := t.subSchemas
		if definition.IsArrayType(fieldType) {
			fieldType = definition.SubType(fieldType)
			targetMap = t.subArraySchemas
		} else if definition.IsMapType(fieldType) {
			fieldType = definition.SubType(fieldType)
			targetMap = t.subMapSchemas
		}

		schema := schemas.Schema(fieldType)
		if schema != nil {
			targetMap[name] = schema
		}
	}

	return Mappers(t.Mappers).ModifySchema(schema, schemas)
}
