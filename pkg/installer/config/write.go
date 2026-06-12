package config

import (
	"github.com/rancher/mapper/convert"
	"gopkg.in/yaml.v3"
)

func PrintInstall(cfg HarvesterConfig) ([]byte, error) {
	data, err := convert.EncodeToMap(cfg.Install)
	if err != nil {
		return nil, err
	}

	toYAMLKeys(data)
	return yaml.Marshal(data)
}

func toYAMLKeys(data map[string]interface{}) {
	for k, v := range data {
		if sub, ok := v.(map[string]interface{}); ok {
			toYAMLKeys(sub)
		}
		newK := convert.ToYAMLKey(k)
		if newK != k {
			delete(data, k)
			data[newK] = v
		}
	}
}
