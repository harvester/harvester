package mappers

import (
	"github.com/rancher/wrangler/v3/pkg/data"
	types "github.com/rancher/wrangler/v3/pkg/schemas"
)

type Exists struct {
	Field   string
	Mapper  types.Mapper
	enabled bool
}

func (m *Exists) FromInternal(data data.Object) {
	if m.enabled {
		m.Mapper.FromInternal(data)
	}
}

func (m *Exists) ToInternal(data data.Object) error {
	if m.enabled {
		return m.Mapper.ToInternal(data)
	}
	return nil
}

func (m *Exists) ModifySchema(s *types.Schema, schemas *types.Schemas) error {
	if _, ok := s.ResourceFields[m.Field]; ok {
		m.enabled = true
		return m.Mapper.ModifySchema(s, schemas)
	}
	return nil
}
