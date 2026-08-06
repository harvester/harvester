package setting

import (
	"encoding/json"
	"fmt"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	defaultTLSOptionName = "default"
)

func (h *Handler) syncTLSOption(setting *harvesterv1.Setting) error {
	tlsOptionSpec := &settings.TLSOptionSpec{}
	value := setting.Value
	if value == "" {
		value = setting.Default
	}

	if err := json.Unmarshal([]byte(value), tlsOptionSpec); err != nil {
		return err
	}

	return h.updateTLSOption(tlsOptionSpec)
}

func (h *Handler) updateTLSOption(tlsOptionSpec *settings.TLSOptionSpec) error {
	obj, err := generateUnstructuredTLSOption(tlsOptionSpec)
	if err != nil {
		return err
	}
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

func generateUnstructuredTLSOption(tlsOptionSpec *settings.TLSOptionSpec) (*unstructured.Unstructured, error) {
	// to avoid deepcopy errors when dealing with unstructured objects, we need to convert the slice of string to slice of interface{}
	cipherSuitesInterface := make([]interface{}, 0, len(tlsOptionSpec.CipherSuites))
	for _, cipherSuite := range tlsOptionSpec.CipherSuites {
		cipherSuitesInterface = append(cipherSuitesInterface, cipherSuite)
	}

	curvePreferencesInterface := make([]interface{}, 0, len(tlsOptionSpec.CurvePreferences))
	for _, curvePreference := range tlsOptionSpec.CurvePreferences {
		curvePreferencesInterface = append(curvePreferencesInterface, curvePreference)
	}

	alpnProtocolsInterface := make([]interface{}, 0, len(tlsOptionSpec.ALPNProtocols))
	for _, alpnProtocol := range tlsOptionSpec.ALPNProtocols {
		alpnProtocolsInterface = append(alpnProtocolsInterface, alpnProtocol)
	}
	clientAuthSecretNamesInterface := make([]interface{}, 0, len(tlsOptionSpec.ClientAuth.SecretNames))
	for _, secretName := range tlsOptionSpec.ClientAuth.SecretNames {
		clientAuthSecretNamesInterface = append(clientAuthSecretNamesInterface, secretName)
	}

	traefikOptionsObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "TLSOption",
			"metadata": map[string]interface{}{
				"name":      defaultTLSOptionName,
				"namespace": util.KubeSystemNamespace,
			},
			"spec": map[string]interface{}{
				"cipherSuites":          cipherSuitesInterface,
				"minVersion":            tlsOptionSpec.MinVersion,
				"maxVersion":            tlsOptionSpec.MaxVersion,
				"sniStrict":             tlsOptionSpec.SniStrict,
				"curvePreferences":      curvePreferencesInterface,
				"alpnProtocols":         alpnProtocolsInterface,
				"disableSessionTickets": tlsOptionSpec.DisableSessionTickets,
			},
		},
	}
	if tlsOptionSpec.ClientAuth.ClientAuthType != "" {
		clientAuthMap := map[string]interface{}{
			"secretNames":    clientAuthSecretNamesInterface,
			"clientAuthType": tlsOptionSpec.ClientAuth.ClientAuthType,
		}
		if err := unstructured.SetNestedMap(traefikOptionsObj.Object, clientAuthMap, "spec", "clientAuth"); err != nil {
			return nil, fmt.Errorf("failed to set clientAuth in TLSOption object: %w", err)
		}
	}

	return traefikOptionsObj, nil
}
