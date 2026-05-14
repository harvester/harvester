package config

import (
	"fmt"

	"github.com/rancher/mapper"
	"github.com/rancher/mapper/convert"
	"gopkg.in/yaml.v3"
)

var (
	schemas = mapper.NewSchemas().Init(func(s *mapper.Schemas) *mapper.Schemas {
		s.DefaultMappers = func() []mapper.Mapper {
			return []mapper.Mapper{
				NewToMap(),
				NewToSlice(),
				NewToBool(),
				NewToInt(),
				NewToFloat(),
				&FuzzyNames{},
			}
		}
		return s
	}).MustImport(HarvesterConfig{})
	schema = schemas.Schema("harvesterConfig")
)

func LoadHarvesterConfig(yamlBytes []byte) (*HarvesterConfig, error) {
	result := NewHarvesterConfig()
	data := map[string]interface{}{}
	if err := yaml.Unmarshal(yamlBytes, &data); err != nil {
		return result, fmt.Errorf("failed to unmarshal yaml: %v", err)
	}
	if err := schema.Mapper.ToInternal(data); err != nil {
		return result, err
	}
	if err := convert.ToObj(data, result); err != nil {
		return result, fmt.Errorf("failed to convert to HarvesterConfig: %v", err)
	}
	if err := result.ExternalStorage.ParseMultiPathConfig(); err != nil {
		return result, fmt.Errorf("failed to parse external storage multi-path config: %v", err)
	}

	return result, nil
}
