package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/rancher/k3os/pkg/system"
	"github.com/rancher/mapper"
	"github.com/rancher/mapper/convert"
	merge2 "github.com/rancher/mapper/convert/merge"
	"github.com/rancher/mapper/values"
)

var (
	// SystemConfig is the default system configuration
	SystemConfig = system.RootPath("config.yaml")
	// LocalConfig is the local system configuration
	LocalConfig  = system.LocalPath("config.yaml")
	localConfigs = system.LocalPath("config.d")
)

var (
	schemas = mapper.NewSchemas().Init(func(s *mapper.Schemas) *mapper.Schemas {
		s.DefaultMappers = func() []mapper.Mapper {
			return []mapper.Mapper{
				NewToMap(),
				NewToSlice(),
				NewToBool(),
				&FuzzyNames{},
			}
		}
		return s
	}).MustImport(CloudConfig{})
	schema  = schemas.Schema("cloudConfig")
	readers = []reader{
		readSystemConfig,
		readCmdline,
		readLocalConfig,
		readCloudConfig,
		readUserData,
	}
)

func ToEnv(cfg CloudConfig) ([]string, error) {
	data, err := convert.EncodeToMap(&cfg)
	if err != nil {
		return nil, err
	}

	return mapToEnv("", data), nil
}

func mapToEnv(prefix string, data map[string]interface{}) []string {
	var result []string
	for k, v := range data {
		keyName := strings.ToUpper(prefix + convert.ToYAMLKey(k))
		if data, ok := v.(map[string]interface{}); ok {
			subResult := mapToEnv(keyName+"_", data)
			result = append(result, subResult...)
		} else {
			result = append(result, fmt.Sprintf("%s=%v", keyName, v))
		}
	}
	return result
}

func ReadConfig() (CloudConfig, error) {
	return readersToObject(append(readers, readLocalConfigs()...)...)
}

func readersToObject(readers ...reader) (CloudConfig, error) {
	result := CloudConfig{
		K3OS: K3OS{
			Install: &Install{},
		},
	}

	data, err := merge(readers...)
	if err != nil {
		return result, err
	}

	return result, convert.ToObj(data, &result)
}

type reader func() (map[string]interface{}, error)

func merge(readers ...reader) (map[string]interface{}, error) {
	data := map[string]interface{}{}
	for _, r := range readers {
		newData, err := r()
		if err != nil {
			return nil, err
		}
		if err := schema.Mapper.ToInternal(newData); err != nil {
			return nil, err
		}
		data = merge2.UpdateMerge(schema, schemas, data, newData, false)
	}
	return data, nil
}

func readSystemConfig() (map[string]interface{}, error) {
	return readFile(SystemConfig)
}

func readLocalConfig() (map[string]interface{}, error) {
	return readFile(LocalConfig)
}

func readLocalConfigs() []reader {
	var result []reader

	files, err := ioutil.ReadDir(localConfigs)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return []reader{
			func() (map[string]interface{}, error) {
				return nil, err
			},
		}
	}

	for _, f := range files {
		p := filepath.Join(localConfigs, f.Name())
		result = append(result, func() (map[string]interface{}, error) {
			return readFile(p)
		})
	}

	return result
}

func readFile(path string) (map[string]interface{}, error) {
	f, err := ioutil.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	data := map[string]interface{}{}
	if err := yaml.Unmarshal(f, &data); err != nil {
		return nil, err
	}

	return data, nil
}

func readCmdline() (map[string]interface{}, error) {
	//supporting regex https://regexr.com/4mq0s
	parser, err := regexp.Compile(`(\"[^\"]+\")|([^\s]+=(\"[^\"]+\")|([^\s]+))`)
	if err != nil {
		return nil, nil
	}

	bytes, err := ioutil.ReadFile("/proc/cmdline")
	if os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	data := map[string]interface{}{}
	for _, item := range parser.FindAllString(string(bytes), -1) {
		parts := strings.SplitN(item, "=", 2)
		value := "true"
		if len(parts) > 1 {
			value = strings.Trim(parts[1], `"`)
		}
		keys := strings.Split(strings.Trim(parts[0], `"`), ".")
		existing, ok := values.GetValue(data, keys...)
		if ok {
			switch v := existing.(type) {
			case string:
				values.PutValue(data, []string{v, value}, keys...)
			case []string:
				values.PutValue(data, append(v, value), keys...)
			}
		} else {
			values.PutValue(data, value, keys...)
		}
	}

	return data, nil
}
