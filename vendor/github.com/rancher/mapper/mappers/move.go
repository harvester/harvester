package mappers

import (
	"fmt"
	"strings"

	"github.com/rancher/mapper"
	"github.com/rancher/mapper/convert"
	"github.com/rancher/mapper/definition"
	"github.com/rancher/mapper/values"
)

type Move struct {
	From, To, CodeName string
	DestDefined        bool
	NoDeleteFromField  bool
}

func (m Move) FromInternal(data map[string]interface{}) {
	if v, ok := values.RemoveValue(data, strings.Split(m.From, "/")...); ok {
		values.PutValue(data, v, strings.Split(m.To, "/")...)
	}
}

func (m Move) ToInternal(data map[string]interface{}) error {
	if v, ok := values.RemoveValue(data, strings.Split(m.To, "/")...); ok {
		values.PutValue(data, v, strings.Split(m.From, "/")...)
	}
	return nil
}

func (m Move) ModifySchema(s *mapper.Schema, schemas *mapper.Schemas) error {
	fromSchema, _, fromField, ok, err := getField(s, schemas, m.From)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("failed to find field %s on schema %s", m.From, s.ID)
	}

	toSchema, toFieldName, _, ok, err := getField(s, schemas, m.To)
	if err != nil {
		return err
	}
	_, ok = toSchema.ResourceFields[toFieldName]
	if ok && !strings.Contains(m.To, "/") && !m.DestDefined {
		return fmt.Errorf("field %s already exists on schema %s", m.To, s.ID)
	}

	if !m.NoDeleteFromField {
		delete(fromSchema.ResourceFields, m.From)
	}

	if !m.DestDefined {
		if m.CodeName == "" {
			fromField.CodeName = convert.Capitalize(toFieldName)
		} else {
			fromField.CodeName = m.CodeName
		}
		toSchema.ResourceFields[toFieldName] = fromField
	}

	return nil
}

func getField(schema *mapper.Schema, schemas *mapper.Schemas, target string) (*mapper.Schema, string, mapper.Field, bool, error) {
	parts := strings.Split(target, "/")
	for i, part := range parts {
		if i == len(parts)-1 {
			continue
		}

		fieldType := schema.ResourceFields[part].Type
		if definition.IsArrayType(fieldType) {
			fieldType = definition.SubType(fieldType)
		}
		subSchema := schemas.Schema(fieldType)
		if subSchema == nil {
			return nil, "", mapper.Field{}, false, fmt.Errorf("failed to find field or schema for %s on %s", part, schema.ID)
		}

		schema = subSchema
	}

	name := parts[len(parts)-1]
	f, ok := schema.ResourceFields[name]
	return schema, name, f, ok, nil
}
