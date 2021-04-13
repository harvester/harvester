package rancher

import (
	"context"
	"net/url"
	"strings"
	"time"

	rancherv3api "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	rancherv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	corev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sett "github.com/rancher/harvester/pkg/controller/master/setting"
	ctlharvesterv1 "github.com/rancher/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

const (
	CAName               = "serving-ca"
	ServerURLSettingName = "server-url"

	RancherNamespace        = "cattle-system"
	RancherInternalCAName   = "tls-rancher-internal-ca"
	RancherInternalCertName = "tls-rancher-internal"
	RancherPrivateCAName    = "tls-ca"

	tlsCertName = "tls.crt"
	tlsKeyName  = "tls.key"
	tlsCAName   = "cacerts.pem"
)

var (
	syncProgressInterval = 5 * time.Second
)

type Handler struct {
	Secrets         corev1.SecretClient
	SecretCache     corev1.SecretCache
	RancherSettings rancherv3.SettingClient
	Settings        ctlharvesterv1.SettingClient
	NodeDrivers     rancherv3.NodeDriverClient
}

func (h *Handler) SettingOnChange(key string, setting *rancherv3api.Setting) (*rancherv3api.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil || setting.Name != ServerURLSettingName || setting.Value == "" {
		return nil, nil
	}

	u, err := url.Parse(setting.Value)
	if err != nil {
		return setting, err
	}

	secret, err := h.SecretCache.Get(RancherNamespace, RancherInternalCertName)
	if err != nil {
		return setting, nil
	}

	cn := u.Hostname()
	if secret.Annotations[sett.CNPrefix+cn] == cn {
		return setting, nil
	}

	toUpdate := secret.DeepCopy()
	if toUpdate.Annotations == nil {
		toUpdate.Annotations = make(map[string]string)
	}

	//clean up other cns
	newAnnotations := map[string]string{}
	for k, v := range toUpdate.Annotations {
		if !strings.Contains(k, sett.CNPrefix) {
			newAnnotations[k] = v
		}
	}
	toUpdate.Annotations = newAnnotations
	toUpdate.Annotations[sett.CNPrefix+cn] = cn
	if _, err = h.Secrets.Update(toUpdate); err != nil {
		return setting, err
	}

	return setting, nil
}

func (h *Handler) registerPrivateCA(ctx context.Context) {
	for {
		err := h.createDefaultRancherPrivateCA()
		if err != nil {
			logrus.Errorf("Failed to create rancher default CA, error:%s", err.Error())
		} else {
			logrus.Infof("Done register rancher private CA")
			return
		}

		select {
		case <-time.After(syncProgressInterval):
			continue
		case <-ctx.Done():
			return
		}
	}
}

func (h *Handler) createDefaultRancherPrivateCA() error {
	secret, err := h.Secrets.Get(sett.CertNamespace, CAName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	_, err = h.SecretCache.Get(RancherNamespace, RancherInternalCAName)
	if apierrors.IsNotFound(err) {
		logrus.Infof("creating default rancher internal CA %s", RancherInternalCAName)
		secretInternalCA := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      RancherInternalCAName,
				Namespace: RancherNamespace,
			},
			StringData: map[string]string{
				tlsCertName: string(secret.Data[tlsCertName]),
				tlsKeyName:  string(secret.Data[tlsKeyName]),
			},
		}
		if _, err := h.Secrets.Create(secretInternalCA); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	_, err = h.SecretCache.Get(RancherNamespace, RancherPrivateCAName)
	if apierrors.IsNotFound(err) {
		logrus.Infof("creating default rancher private CA %s", RancherPrivateCAName)
		secretCA := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      RancherPrivateCAName,
				Namespace: RancherNamespace,
			},
			StringData: map[string]string{
				tlsCAName: string(secret.Data[tlsCertName]),
			},
		}
		if _, err := h.Secrets.Create(secretCA); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return nil
}
