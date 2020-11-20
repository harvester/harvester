package config

import (
	"fmt"
	"io"

	"github.com/ghodss/yaml"
	"github.com/rancher/mapper/convert"
)

func PrintInstall(cfg CloudConfig) ([]byte, error) {
	data, err := convert.EncodeToMap(cfg.K3OS.Install)
	if err != nil {
		return nil, err
	}

	toYAMLKeys(data)
	return yaml.Marshal(data)
}

func Write(cfg CloudConfig, writer io.Writer) error {
	bytes, err := ToBytes(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal [%s]: %v", string(bytes), err)
	}
	_, err = writer.Write(bytes)
	return err
}

func ToBytes(cfg CloudConfig) ([]byte, error) {
	cfg.K3OS.Install = nil
	data, err := convert.EncodeToMap(cfg)
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
