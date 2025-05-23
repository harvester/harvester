package setting

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/rancher/wrangler/v3/pkg/relatedresource"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	harvSettings "github.com/harvester/harvester/pkg/settings"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

const (
	pssEnforcementKey          = "pod-security.kubernetes.io/enforce"
	pssEnforcementValue        = "baseline"
	pssEnforcementVersionKey   = "pod-security.kubernetes.io/enforce-version"
	pssEnforcementVersionValue = "latest"
)

// default list of whitelisted namspaces from https://ranchermanager.docs.rancher.com/how-to-guides/new-user-guides/authentication-permissions-and-global-configuration/psa-config-templates#exempting-required-rancher-namespaces and added additional harvester specific namespaces

var defaultHarvesterNamespaceWhilteList = []string{"calico-apiserver", "calico-system", "cattle-alerting", "cattle-csp-adapter-system", "cattle-elemental-system", "cattle-epinio-system", "cattle-externalip-system", "cattle-fleet-local-system", "cattle-fleet-system", "cattle-gatekeeper-system", "cattle-global-data", "cattle-global-nt", "cattle-impersonation-system", "cattle-istio", "cattle-istio-system", "cattle-logging", "cattle-logging-system", "cattle-monitoring-system", "cattle-neuvector-system", "cattle-prometheus", "cattle-provisioning-capi-system", "cattle-resources-system", "cattle-sriov-system", "cattle-system", "cattle-ui-plugin-system", "cattle-windows-gmsa-system", "cert-manager", "cis-operator-system", "fleet-default", "ingress-nginx", "istio-system", "kube-node-lease", "kube-public", "kube-system", "longhorn-system", "rancher-alerting-drivers", "security-scan", "tigera-operator", "harvester-system", "harvester-public", "rancher-vcluster", "cattle-dashboards", "fleet-local", "local"}

func (h *Handler) syncPodSecuritySetting(setting *harvesterv1.Setting) error {
	pssSetting := &harvSettings.PodSecuritySetting{}
	var value string
	if setting.Value != "" {
		value = setting.Value
	} else {
		value = setting.Default
	}
	if err := json.Unmarshal([]byte(value), pssSetting); err != nil {
		return fmt.Errorf("Invalid JSON `%s`: %s", setting.Value, err.Error())
	}

	// setting is not enabled, no further processing needed
	if !pssSetting.Enabled {
		return nil
	}

	// split comma separated whitelist
	additionalNamespaces := strings.Split(pssSetting.WhitelistedNamespacesList, ",")
	whiteListedNamespaces := append(defaultHarvesterNamespaceWhilteList, additionalNamespaces...)

	// fetch all namespaces
	nsList, err := h.namespacesCache.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("error fetching namespaces during reconcile of podsecuritystandardsetting: %v", err)
	}

	for _, v := range nsList {
		if err := h.reconcileNamespace(v, whiteListedNamespaces); err != nil {
			return fmt.Errorf("error reconciling namespace %s: %v", v.Name, err)
		}
	}
	return nil
}

// reconcileNamespaces will apply/remove pss labels as necessary
func (h *Handler) reconcileNamespace(ns *corev1.Namespace, whitelist []string) error {
	// ignore namespaces being deleted
	if !ns.DeletionTimestamp.IsZero() {
		return nil
	}

	nsCopy := ns.DeepCopy()
	// namespace is whitelist, remove the pss keys
	if slices.Contains(whitelist, nsCopy.Name) {
		delete(nsCopy.Labels, pssEnforcementKey)
		delete(nsCopy.Labels, pssEnforcementVersionKey)
	} else {
		if nsCopy.Labels == nil {
			nsCopy.Labels = make(map[string]string)
		}
		nsCopy.Labels[pssEnforcementKey] = pssEnforcementValue
		nsCopy.Labels[pssEnforcementVersionKey] = pssEnforcementVersionValue
	}

	// no further changes are needed
	if reflect.DeepEqual(ns, nsCopy) {
		return nil
	}
	_, err := h.namespaces.Update(nsCopy)
	return err
}

// enqueue ClusterPodSecurityStandardSetting if there is a change to a namespace
func (h *Handler) watchNamespaceChanges(_, _ string, obj runtime.Object) ([]relatedresource.Key, error) {
	if ns, ok := obj.(*corev1.Namespace); ok {
		// ignore namespaces being deleted
		if !ns.DeletionTimestamp.IsZero() {
			return nil, nil
		}
		return []relatedresource.Key{
			{
				Name: harvSettings.ClusterPodSecurityStandardSettingName,
			},
		}, nil
	}
	return nil, nil
}
