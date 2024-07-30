package setting

import (
	"encoding/json"
	"fmt"

	"github.com/rancher/wrangler/v3/pkg/data"
	"gopkg.in/yaml.v3"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	rancherDeploymentName = "rancher"
	tlsIngressSecretName  = "tls-ingress"
)

func (h *Handler) syncSSLCertificate(setting *harvesterv1.Setting) error {
	sslCertificate := &settings.SSLCertificate{}
	value := setting.Value
	if value == "" {
		value = setting.Default
	}
	if err := json.Unmarshal([]byte(value), sslCertificate); err != nil {
		return err
	}
	if sslCertificate.CA == "" && sslCertificate.PublicCertificate == "" && sslCertificate.PrivateKey == "" {
		return h.resetCertificates()
	}

	return h.updateCertificates(sslCertificate)
}

func (h *Handler) resetCertificates() error {
	if err := h.updateIngressDefaultCertificate(util.CattleSystemNamespaceName, util.InternalTLSSecretName); err != nil {
		return err
	}

	return h.redeployDeployment(util.CattleSystemNamespaceName, rancherDeploymentName)
}

func (h *Handler) updateCertificates(sslCertificate *settings.SSLCertificate) error {
	if err := h.updateTLSSecret(sslCertificate.PublicCertificate, sslCertificate.PrivateKey); err != nil {
		return err
	}

	if err := h.updateIngressDefaultCertificate(util.CattleSystemNamespaceName, tlsIngressSecretName); err != nil {
		return err
	}

	return h.redeployDeployment(util.CattleSystemNamespaceName, rancherDeploymentName)
}

func (h *Handler) updateTLSSecret(publicCertificate, privateKey string) error {
	tlsSecret, err := h.secretCache.Get(util.CattleSystemNamespaceName, tlsIngressSecretName)
	if err != nil {
		return err
	}
	toUpdateSecret := tlsSecret.DeepCopy()
	toUpdateSecret.Data["tls.crt"] = []byte(publicCertificate)
	toUpdateSecret.Data["tls.key"] = []byte(privateKey)
	_, err = h.secrets.Update(toUpdateSecret)
	return err
}

// updateIngressDefaultCertificate updates default ssl certificate of nginx ingress controller
func (h *Handler) updateIngressDefaultCertificate(namespace, secretName string) error {
	secretRef := fmt.Sprintf("%s/%s", namespace, secretName)
	helmChartConfig, err := h.helmChartConfigCache.Get(util.KubeSystemNamespace, util.Rke2IngressNginxAppName)
	if err != nil {
		return err
	}
	toUpdateHelmChartConfig := helmChartConfig.DeepCopy()
	var values = make(map[string]interface{})

	if err := yaml.Unmarshal([]byte(helmChartConfig.Spec.ValuesContent), &values); err != nil {
		return err
	}
	data.PutValue(values, secretRef, "controller", "extraArgs", "default-ssl-certificate")
	newValuesContent, err := yaml.Marshal(values)
	if err != nil {
		return err
	}
	toUpdateHelmChartConfig.Spec.ValuesContent = string(newValuesContent)
	if _, err := h.helmChartConfigs.Update(toUpdateHelmChartConfig); err != nil {
		return err
	}
	return nil
}
