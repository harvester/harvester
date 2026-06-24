package setting

import (
	"encoding/json"
	"strings"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	defaultTLSOptionName = "default"
)

func (h *Handler) syncSSLParameters(setting *harvesterv1.Setting) error {
	sslParameter := &settings.SSLParameter{}
	value := setting.Value
	if value == "" {
		value = setting.Default
	}
	if err := json.Unmarshal([]byte(value), sslParameter); err != nil {
		return err
	}

	return h.updateSSLParameters(sslParameter)
}

func (h *Handler) updateSSLParameters(sslParameter *settings.SSLParameter) error {
	logrus.Infof("Update SSL Parameters: Ciphers: %s, Protocols: %s", sslParameter.Ciphers, sslParameter.Protocols)

	tlsOptionRes := schema.GroupVersionResource{Group: "traefik.io", Version: "v1alpha1", Resource: "tlsoptions"}
	if sslParameter.Ciphers == "" {
		return h.dynamicClient.Resource(tlsOptionRes).Namespace(util.KubeSystemNamespace).Delete(h.ctx, defaultTLSOptionName, metav1.DeleteOptions{})
	}
	ciphers := strings.Split(sslParameter.Ciphers, ":")
	obj := generateUnstructuredTLSOption(ciphers)
	return h.apply.WithDynamicLookup().WithSetID(util.HarvesterChartReleaseName).ApplyObjects(obj)
}

/*
# Generate a TLSOption object
# https://doc.traefik.io/traefik/reference/routing-configuration/kubernetes/crd/tls/tlsoption/
apiVersion: traefik.io/v1alpha1
kind: TLSOption
metadata:
  name: mytlsoption
  namespace: default

spec:
  minVersion: VersionTLS12
  sniStrict: true
  cipherSuites:
    - TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
    - TLS_RSA_WITH_AES_256_GCM_SHA384
  clientAuth:
    secretNames:
      - secret-ca1
      - secret-ca2
    clientAuthType: VerifyClientCertIfGiven
*/

func generateUnstructuredTLSOption(ciphers []string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "TLSOption",
			"metadata": map[string]interface{}{
				"name":      defaultTLSOptionName,
				"namespace": util.KubeSystemNamespace,
			},
			"spec": map[string]interface{}{
				"cipherSuites": ciphers,
			},
		},
	}
}
