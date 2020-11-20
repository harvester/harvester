package mappers

import (
	types "github.com/rancher/mapper"
)

type Namespaced struct {
	IfNot   bool
	Mappers []types.Mapper
	run     bool
}

func (s *Namespaced) FromInternal(data map[string]interface{}) {
	if s.run {
		types.Mappers(s.Mappers).FromInternal(data)
	}
}

func (s *Namespaced) ToInternal(data map[string]interface{}) error {
	if s.run {
		return types.Mappers(s.Mappers).ToInternal(data)
	}
	return nil
}

func (s *Namespaced) ModifySchema(schema *types.Schema, schemas *types.Schemas) error {
	if s.IfNot {
		if schema.NonNamespaced {
			s.run = true
		}
	} else {
		if !schema.NonNamespaced {
			s.run = true
		}
	}
	if s.run {
		return types.Mappers(s.Mappers).ModifySchema(schema, schemas)
	}

	return nil
}
