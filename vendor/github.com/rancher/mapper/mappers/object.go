package mappers

import (
	types "github.com/rancher/mapper"
)

type Object struct {
	types.Mappers
}

func NewObject(mappers ...types.Mapper) Object {
	return Object{
		Mappers: append([]types.Mapper{
			&Embed{Field: "metadata"},
			&Embed{Field: "spec", Optional: true},
			&ReadOnly{Field: "status", Optional: true, SubFields: true},
			Drop{Field: "kind"},
			Drop{Field: "apiVersion"},
			Move{From: "selfLink", To: ".selfLink", DestDefined: true},
			&Namespaced{
				IfNot: true,
				Mappers: []types.Mapper{
					&Drop{Field: "namespace"},
				},
			},
			Drop{Field: "finalizers", IgnoreDefinition: true},
		}, mappers...),
	}
}
