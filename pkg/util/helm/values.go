package helm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/client-go/kubernetes"

	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

// FetchImageFromHelmValues fetches image information from helm chart values, the name points to a chart e.g. harvester
func FetchImageFromHelmValues(clientSet kubernetes.Interface, namespace, name string, keyNames []string) (settings.Image, error) {
	helmValues, err := FetchHelmValues(clientSet, name)
	if err != nil {
		return settings.Image{}, err
	}

	image, err := ImageFromHelmValues(helmValues, keyNames)
	if err != nil {
		return settings.Image{}, fmt.Errorf("%s/%s app: %w", namespace, name, err)
	}

	return image, nil
}

// FetchHelmValues fetches all values, including the chart defaults, of a helm release
func FetchHelmValues(clientSet kubernetes.Interface, name string) (map[string]interface{}, error) {
	restClientGetter := cli.New().RESTClientGetter()
	driverSecret := driver.NewSecrets(clientSet.CoreV1().Secrets(""))
	getValues := action.NewGetValues(&action.Configuration{
		RESTClientGetter: restClientGetter,
		Releases:         storage.Init(driverSecret),
		KubeClient:       kube.New(restClientGetter),
	})
	getValues.AllValues = true
	helmValues, err := getValues.Run(name)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"helm.name": name,
		}).Error("failed to get helm values")
		return nil, err
	}

	return helmValues, nil
}

// ImageFromHelmValues gets image information from the keyNames path of helm chart values
func ImageFromHelmValues(helmValues map[string]interface{}, keyNames []string) (settings.Image, error) {
	var (
		image settings.Image
		ok    bool
		bytes []byte
		value interface{}
		err   error
	)

	if value, ok = util.GetValue(helmValues, keyNames...); !ok {
		logrus.Debugf("fail to get value of path(%s)", strings.Join(keyNames, "."))
		return image, fmt.Errorf("cannot find path(%s)", strings.Join(keyNames, "."))
	}

	if bytes, err = json.Marshal(value); err != nil {
		return image, fmt.Errorf("cannot marshal image in path(%s): %w", strings.Join(keyNames, "."), err)
	}

	if err = json.Unmarshal(bytes, &image); err != nil {
		return image, fmt.Errorf("cannot unmarshal image in path(%s): %w", strings.Join(keyNames, "."), err)
	}

	return image, nil
}

// CattleRegistryFromHelmValues gets global.cattle.systemDefaultRegistry from helm chart values,
// it returns an empty string if the value is not set
func CattleRegistryFromHelmValues(helmValues map[string]interface{}) string {
	value, _ := util.GetValue(helmValues, "global", "cattle", "systemDefaultRegistry")
	registry, _ := value.(string)
	return registry
}
