package setting

import (
	"net/url"
	"strings"

	v1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
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

func (h *Handler) OnChanged(key string, setting *v1alpha1.Setting) (*v1alpha1.Setting, error) {
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
