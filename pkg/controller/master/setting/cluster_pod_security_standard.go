package setting

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/rancher/wrangler/v3/pkg/relatedresource"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	harvSettings "github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

// default list of whitelisted namspaces from https://ranchermanager.docs.rancher.com/how-to-guides/new-user-guides/authentication-permissions-and-global-configuration/psa-config-templates#exempting-required-rancher-namespaces and added additional harvester specific namespaces

var defaultHarvesterNamespaceWhilteList = []string{"calico-apiserver", "calico-system", "cattle-alerting", "cattle-csp-adapter-system", "cattle-elemental-system", "cattle-epinio-system", "cattle-externalip-system", "cattle-fleet-local-system", "cattle-fleet-system", "cattle-gatekeeper-system", "cattle-global-data", "cattle-global-nt", "cattle-impersonation-system", "cattle-istio", "cattle-istio-system", "cattle-logging", "cattle-logging-system", "cattle-monitoring-system", "cattle-neuvector-system", "cattle-prometheus", "cattle-provisioning-capi-system", "cattle-resources-system", "cattle-sriov-system", "cattle-system", "cattle-ui-plugin-system", "cattle-windows-gmsa-system", "cert-manager", "cis-operator-system", "fleet-default", "ingress-nginx", "istio-system", "kube-node-lease", "kube-public", "kube-system", "longhorn-system", "rancher-alerting-drivers", "security-scan", "tigera-operator", "harvester-system", "harvester-public", "rancher-vcluster", "cattle-dashboards", "fleet-local", "local"}

func (h *Handler) syncPodSecuritySetting(setting *harvesterv1.Setting) error {
	pssSetting, err := harvSettings.GetPodSecuritySetting(setting)
	if err != nil {
		return err
	}
	// setting is not enabled, no further processing needed
	if !pssSetting.Enabled {
		return h.cleanupManagedPSSLabels()
	}

	// split comma separated whitelist
	additionalNamespaces := strings.Split(pssSetting.WhitelistedNamespacesList, ",")
	restrictedNamespacesList := strings.Split(pssSetting.RestrictedNamespacesList, ",")
	privilegedNamespacesList := strings.Split(pssSetting.PrivilegedNamespacesList, ",")

	whiteListedNamespaces := append(defaultHarvesterNamespaceWhilteList, additionalNamespaces...)

	// fetch all namespaces
	nsList, err := h.namespacesCache.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("error fetching namespaces during reconcile of podsecuritystandardsetting: %v", err)
	}

	for _, v := range nsList {
		if err := h.reconcileNamespace(v, whiteListedNamespaces, restrictedNamespacesList, privilegedNamespacesList); err != nil {
			return fmt.Errorf("error reconciling namespace %s: %v", v.Name, err)
		}
	}
	return nil
}

// reconcileNamespaces will apply/remove pss labels as necessary
func (h *Handler) reconcileNamespace(ns *corev1.Namespace, whitelist, restrictedNamespacesList, privilegedNamespacesList []string) error {
	// ignore namespaces being deleted
	if !ns.DeletionTimestamp.IsZero() {
		return nil
	}

	nsCopy := ns.DeepCopy()
	// namespace is whitelist, remove the pss keys
	if slices.Contains(whitelist, nsCopy.Name) {
		delete(nsCopy.Labels, util.PSSEnforcementKey)
		delete(nsCopy.Labels, util.PSSEnforcementVersionKey)
	} else if slices.Contains(restrictedNamespacesList, nsCopy.Name) {
		applyPSSLabel(nsCopy, util.PSSEnforcementKey, util.RestrictedEnforcementValue)
	} else if slices.Contains(privilegedNamespacesList, nsCopy.Name) {
		applyPSSLabel(nsCopy, util.PSSEnforcementKey, util.PrivilegedEnforcementValue)
	} else {
		applyPSSLabel(nsCopy, util.PSSEnforcementKey, util.BaselineEnforcementValue)
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

// apply harvester managed pss labels to namespace
// harvesterManagedPSSKey is used to make it easier to filter our namespaces being managed by the harvester settings controller
func applyPSSLabel(ns *corev1.Namespace, key, value string) {
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}
	ns.Labels[key] = value
	ns.Labels[util.HarvesterManagedPSSKey] = util.HarvesterManagedPSSValue
	ns.Labels[util.PSSEnforcementVersionKey] = util.PSSEnforcementVersionValue
}

// cleanupManagedPSSLabels is invoked when PSS setting is disabled
// it will use the label pss.harvesterhci.io/managed=true to identify
// namespaces that were managed by harvester and remove the pss and harvester management labels
func (h *Handler) cleanupManagedPSSLabels() error {
	sets := labels.Set{
		util.HarvesterManagedPSSKey: util.HarvesterManagedPSSValue,
	}

	nsList, err := h.namespacesCache.List(sets.AsSelector())
	if err != nil {
		return fmt.Errorf("error listing namespaces: %v", err)
	}

	for _, ns := range nsList {
		nsCopy := ns.DeepCopy()
		delete(nsCopy.Labels, util.PSSEnforcementKey)
		delete(nsCopy.Labels, util.PSSEnforcementVersionKey)
		delete(nsCopy.Labels, util.HarvesterManagedPSSKey)
		if !reflect.DeepEqual(nsCopy.Labels, ns.Labels) {
			if _, err := h.namespaces.Update(nsCopy); err != nil {
				return fmt.Errorf("error cleaning up pss labels from namespace %s: %v", ns.Name, err)
			}
		}
	}

	return nil
}
