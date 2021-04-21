package setting

import (
	"net/url"
	"strings"

	v1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

const (
	CertNamespace = "kube-system"
	CNPrefix      = "listener.cattle.io/cn-"

	certName = "serving-cert"
)

// Handler updates harvester certificate SANs on server-url setting changes
type Handler struct {
	v1.SecretClient
	v1.SecretCache
}

func (h *Handler) ServerURLOnChanged(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil || setting.Name != "server-url" || setting.Value == "" {
		return setting, nil
	}
	u, err := url.Parse(setting.Value)
	if err != nil {
		return setting, err
	}

	secret, err := h.SecretCache.Get(CertNamespace, certName)
	if errors.IsNotFound(err) {
		return setting, nil
	} else if err != nil {
		return setting, err
	}
	cn := u.Hostname()
	if secret.Annotations[CNPrefix+cn] == cn {
		return setting, nil
	}
	toUpdate := secret.DeepCopy()
	if toUpdate.Annotations == nil {
		toUpdate.Annotations = make(map[string]string)
	}

	//clean up other cns
	newAnnotations := map[string]string{}
	for k, v := range toUpdate.Annotations {
		if !strings.Contains(k, CNPrefix) {
			newAnnotations[k] = v
		}
	}
	toUpdate.Annotations = newAnnotations
	toUpdate.Annotations[CNPrefix+cn] = cn
	_, err = h.SecretClient.Update(toUpdate)
	return setting, err
}

func (h *Handler) LogLevelOnChanged(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil || setting.Name != "log-level" || setting.Value == "" {
		return setting, nil
	}

	level, err := logrus.ParseLevel(setting.Value)
	if err != nil {
		return setting, err
	}

	logrus.Infof("set log level to %s", level)
	logrus.SetLevel(level)
	return setting, nil
}
