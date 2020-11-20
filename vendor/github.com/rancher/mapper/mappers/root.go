package mappers

import (
	types "github.com/rancher/mapper"
)

type Root struct {
	enabled bool
	Mapper  types.Mapper
}

func (m *Root) FromInternal(data map[string]interface{}) {
	if m.enabled {
		m.Mapper.FromInternal(data)
	}
}

func (m *Root) ToInternal(data map[string]interface{}) error {
	if m.enabled {
		return m.Mapper.ToInternal(data)
	}
	return nil
}

func (m *Root) ModifySchema(s *types.Schema, schemas *types.Schemas) error {
	if s.Object {
		m.enabled = true
		return m.Mapper.ModifySchema(s, schemas)
	}
	return nil
}
