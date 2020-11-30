package util

import (
	"github.com/rancher/wrangler/pkg/data"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rancher/harvester/pkg/config"
)

const (
	defaultNamespace      = "default"
	certNoneConfigMapName = "importer-ca-none"
)

// SetHTTPSourceDataVolume sets the requiresScratch annotation, and certConfigMap with an existing empty configmap
// It changes the CDI's decision to convert image after the whole file is downloaded.
func SetHTTPSourceDataVolume(data data.Object) {
	logrus.Infof("%v+", data.Map("spec", "source", "http"))
	if data.Map("spec", "source", "http") != nil {
		data.SetNested("true", "metadata", "annotations", "cdi.kubevirt.io/storage.import.requiresScratch")
		data.SetNested(certNoneConfigMapName, "spec", "source", "http", "certConfigMap")
	}
}

func InitCertConfigMap(scaled *config.Scaled) error {
	configmaps := scaled.Management.CoreFactory.Core().V1().ConfigMap()
	certNoneConfigmap := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: certNoneConfigMapName,
			//DVs are created in the default namespace
			Namespace: defaultNamespace,
		},
	}
	_, err := configmaps.Create(certNoneConfigmap)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}
