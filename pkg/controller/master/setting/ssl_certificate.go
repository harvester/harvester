package setting

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	rancherDeploymentName       = "rancher"
	tlsIngressSecretName        = "tls-ingress"
	traefikDefaultIngressSecret = "traefik-ingress"
	defaultTLSStoreName         = "default"
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
	return h.redeploySSLCertificateWorkload()
}

func (h *Handler) updateCertificates(sslCertificate *settings.SSLCertificate) error {
	if err := h.updateTLSSecret(sslCertificate.PublicCertificate, sslCertificate.PrivateKey); err != nil {
		return err
	}

	if err := h.updateIngressDefaultCertificate(util.CattleSystemNamespaceName, tlsIngressSecretName); err != nil {
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

// updateIngressDefaultCertificate updates default ssl certificate of nginx ingress controller
func (h *Handler) updateIngressDefaultCertificate(namespace, secretName string) error {
	// generate traefik secret
	traefikSecret, err := h.generateDefaultCertificate(namespace, secretName)
	if err != nil {
		return err
	}

	err = h.apply.WithDynamicLookup().WithSetID(util.HarvesterChartReleaseName).ApplyObjects(traefikSecret)
	if err != nil {
		return fmt.Errorf("error applying default traefik secret: %w", err)
	}

	tlsStore := generateTLSStore(traefikDefaultIngressSecret)
	return h.apply.WithDynamicLookup().WithSetID(util.HarvesterChartReleaseName).ApplyObjects(tlsStore)
}

func (h *Handler) redeploySSLCertificateWorkload() error {
	if err := h.redeployDaemonset(util.KubeSystemNamespace, util.Rke2TraefikControllerName); err != nil {
		return err
	}

	return h.redeployDeployment(util.CattleSystemNamespaceName, rancherDeploymentName)
}

func (h *Handler) generateDefaultCertificate(namespace, name string) (*corev1.Secret, error) {
	secretObj, err := h.secretCache.Get(namespace, name)
	if err != nil {
		return nil, fmt.Errorf("error fetching secret %s/%s: %w", namespace, name, err)
	}

	traefikSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      traefikDefaultIngressSecret,
			Namespace: util.KubeSystemNamespace,
		},
		Data: secretObj.Data,
	}
	return traefikSecret, nil
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
				"namespace": util.KubeSystemNamespace,
			},
			"spec": map[string]interface{}{
				"defaultCertificate": map[string]interface{}{
					"secretName": secretName,
				},
			},
		},
	}
}
