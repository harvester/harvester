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
func FetchImageFromHelmValues(clientSet *kubernetes.Clientset, namespace, name string, keyNames []string) (settings.Image, error) {
	var image settings.Image
	kubeInterface := clientSet
	restClientGetter := cli.New().RESTClientGetter()
	driverSecret := driver.NewSecrets(kubeInterface.CoreV1().Secrets(""))
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
		return image, err
	}

	var (
		ok    bool
		bytes []byte
		value interface{}
	)

	if value, ok = util.GetValue(helmValues, keyNames...); !ok {
		logrus.Debugf("fail to get value of path(%s)", strings.Join(keyNames, "."))
		return image, fmt.Errorf("cannot find path(%s) %s/%s app", strings.Join(keyNames, "."), namespace, name)
	}

	if bytes, err = json.Marshal(value); err != nil {
		return image, fmt.Errorf("cannot marshal image in path(%s) of %s/%s app: %v", strings.Join(keyNames, "."), namespace, name, err)
	}

	if err = json.Unmarshal(bytes, &image); err != nil {
		return image, fmt.Errorf("cannot unmarshal image in path(%s) of %s/%s app: %v", strings.Join(keyNames, "."), namespace, name, err)
	}

	return image, nil
}
