package setting

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/rancher/wrangler/v3/pkg/relatedresource"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	psaApi "k8s.io/pod-security-admission/api"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvSettings "github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

// default list of expect namespaces from rancher docs: https://ranchermanager.docs.rancher.com/how-to-guides/new-user-guides/authentication-permissions-and-global-configuration/psa-config-templates#exempting-required-rancher-namespaces and added additional harvester specific namespaces
func (h *Handler) syncPodSecuritySetting(setting *harvesterv1.Setting) error {
	pssSetting, err := harvSettings.GetPodSecuritySetting(setting)
	if err != nil {
		return err
	}
	// setting is not enabled, no further processing needed
	if !pssSetting.Enabled {
		return h.cleanupManagedPSSLabels()
	}

	restrictedNamespacesList := util.GetRestrictedNamespacesList(pssSetting)
	privilegedNamespacesList := util.GetPrivilegedNamespacesList(pssSetting)
	whiteListedNamespaces := util.GetWhitelistedNamespacesList(pssSetting)

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
		// if namespace has manually added PSS labels, we skip them
		// as it may be a user defined scenario
		// we will only clean up labels if the label was added by Harvester controller
		// and this is controller by the key util.HarvesterManagedPSSKey
		// which gets applied to namespace when a PSS is applied
		if _, ok := nsCopy.Labels[util.HarvesterManagedPSSKey]; ok {
			delete(nsCopy.Labels, psaApi.EnforceLevelLabel)
			delete(nsCopy.Labels, psaApi.EnforceVersionLabel)
		}
	} else if slices.Contains(restrictedNamespacesList, nsCopy.Name) {
		applyPSSLabel(nsCopy, psaApi.EnforceLevelLabel, string(psaApi.LevelRestricted))
	} else if slices.Contains(privilegedNamespacesList, nsCopy.Name) {
		applyPSSLabel(nsCopy, psaApi.EnforceLevelLabel, string(psaApi.LevelPrivileged))
	} else {
		applyPSSLabel(nsCopy, psaApi.EnforceLevelLabel, string(psaApi.LevelBaseline))
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
	ns.Labels[psaApi.EnforceVersionLabel] = psaApi.VersionLatest
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
		delete(nsCopy.Labels, psaApi.EnforceLevelLabel)
		delete(nsCopy.Labels, psaApi.EnforceVersionLabel)
		delete(nsCopy.Labels, util.HarvesterManagedPSSKey)
		if _, err := h.namespaces.Update(nsCopy); err != nil {
			return fmt.Errorf("error cleaning up pss labels from namespace %s: %v", ns.Name, err)
		}
	}

	return nil
}
