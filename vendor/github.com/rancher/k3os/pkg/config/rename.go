package config

import (
	"strings"

	"github.com/rancher/mapper"
	"github.com/rancher/mapper/convert"
	"github.com/rancher/mapper/mappers"
)

type FuzzyNames struct {
	mappers.DefaultMapper
	names map[string]string
}

func (f *FuzzyNames) ToInternal(data map[string]interface{}) error {
	for k, v := range data {
		if newK, ok := f.names[k]; ok && newK != k {
			data[newK] = v
		}
	}
	return nil
}

func (f *FuzzyNames) addName(name, toName string) {
	f.names[strings.ToLower(name)] = toName
	f.names[convert.ToYAMLKey(name)] = toName
	f.names[strings.ToLower(convert.ToYAMLKey(name))] = toName
}

func (f *FuzzyNames) ModifySchema(schema *mapper.Schema, schemas *mapper.Schemas) error {
	f.names = map[string]string{}

	for name := range schema.ResourceFields {
		if strings.HasSuffix(name, "s") && len(name) > 1 {
			f.addName(name[:len(name)-1], name)
		}
		if strings.HasSuffix(name, "es") && len(name) > 2 {
			f.addName(name[:len(name)-2], name)
		}
		f.addName(name, name)
	}

	f.names["pass"] = "passphrase"
	f.names["password"] = "passphrase"

	return nil
}
