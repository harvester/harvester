package setting

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	rancherDeploymentName = "rancher"
	tlsIngressSecretName  = "tls-ingress"
	defaultTLSStoreName   = "default"
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
	if err := h.updateTraefikTLSStore(util.InternalTLSSecretName); err != nil {
		return err
	}
	return h.redeploySSLCertificateWorkload()
}

func (h *Handler) updateCertificates(sslCertificate *settings.SSLCertificate) error {
	if err := h.updateTLSSecret(sslCertificate.PublicCertificate, sslCertificate.PrivateKey); err != nil {
		return err
	}

	if err := h.updateTraefikTLSStore(tlsIngressSecretName); err != nil {
		return err
	}
	return h.redeploySSLCertificateWorkload()
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

// updateTraefikTLSStore updates default ssl certificate of traefik ingress controller
func (h *Handler) updateTraefikTLSStore(secretName string) error {
	tlsStore := generateTLSStore(secretName)
	return h.apply.WithDynamicLookup().WithSetID(util.HarvesterChartReleaseName).ApplyObjects(tlsStore)
}

func (h *Handler) redeploySSLCertificateWorkload() error {
	return h.redeployDeployment(util.CattleSystemNamespaceName, rancherDeploymentName)
}

/*
# Generate TLSStore object
# https://doc.traefik.io/traefik/reference/routing-configuration/kubernetes/crd/tls/tlsstore/
apiVersion: traefik.io/v1alpha1
kind: TLSStore
metadata:

	name: default

spec:

	defaultCertificate:
	  secretName:  supersecret
*/
func generateTLSStore(secretName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "TLSStore",
			"metadata": map[string]interface{}{
				"name":      defaultTLSStoreName,
				"namespace": util.CattleSystemNamespaceName,
			},
			"spec": map[string]interface{}{
				"defaultCertificate": map[string]interface{}{
					"secretName": secretName,
				},
			},
		},
	}
}
