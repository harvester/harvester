package catalog

import (
	"encoding/json"
	"fmt"
	"strings"

	catalogv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	"github.com/sirupsen/logrus"

	"github.com/harvester/harvester/pkg/settings"
)

// FetchAppChartImage fetches image information from Spec.Values of Rancher RCD App.
func FetchAppChartImage(appCache catalogv1.AppCache, namespace, name string, keyNames []string) (settings.Image, error) {
	var image settings.Image

	harvesterApp, err := appCache.Get(namespace, name)
	if err != nil {
		return image, fmt.Errorf("cannot get %s/%s app: %v", namespace, name, err)
	}

	var (
		ok    bool
		bytes []byte
		value = harvesterApp.Spec.Chart.Values
	)

	for _, key := range keyNames {
		if value, ok = value[key].(map[string]interface{}); !ok {
			logrus.Debugf("fail to get value of path(%s)", strings.Join(keyNames, "."))
			return image, fmt.Errorf("cannot find %s in path(%s) %s/%s app", key, strings.Join(keyNames, "."), namespace, name)
		}
	}

	if bytes, err = json.Marshal(value); err != nil {
		return image, fmt.Errorf("cannot marshal image in path(%s) of %s/%s app: %v", strings.Join(keyNames, "."), namespace, name, err)
	}

	if err = json.Unmarshal(bytes, &image); err != nil {
		return image, fmt.Errorf("cannot unmarshal image in path(%s) of %s/%s app: %v", strings.Join(keyNames, "."), namespace, name, err)
	}

	return image, nil
}
