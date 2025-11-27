package addon

import (
	"fmt"

	"gopkg.in/yaml.v2"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/sirupsen/logrus"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlloggingv1 "github.com/harvester/harvester/pkg/generated/controllers/logging.banzaicloud.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/logging"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
)

const (
	vClusterAddonName      = "rancher-vcluster"
	vClusterAddonNamespace = "rancher-vcluster"
	vCluster0190           = "v0.19.0"
	vCluster0300           = "v0.30.0"
)

func NewValidator(addons ctlharvesterv1.AddonCache, flowCache ctlloggingv1.FlowCache, outputCache ctlloggingv1.OutputCache, clusterFlowCache ctlloggingv1.ClusterFlowCache, clusterOutputCache ctlloggingv1.ClusterOutputCache, upgradeLogCache ctlharvesterv1.UpgradeLogCache, nodeCache ctlcorev1.NodeCache) types.Validator {
	return &addonValidator{
		addons:             addons,
		flowCache:          flowCache,
		outputCache:        outputCache,
		clusterFlowCache:   clusterFlowCache,
		clusterOutputCache: clusterOutputCache,
		upgradeLogCache:    upgradeLogCache,
		nodeCache:          nodeCache,
	}
}

type addonValidator struct {
	types.DefaultValidator

	addons             ctlharvesterv1.AddonCache
	flowCache          ctlloggingv1.FlowCache
	outputCache        ctlloggingv1.OutputCache
	clusterFlowCache   ctlloggingv1.ClusterFlowCache
	clusterOutputCache ctlloggingv1.ClusterOutputCache
	upgradeLogCache    ctlharvesterv1.UpgradeLogCache
	nodeCache          ctlcorev1.NodeCache
}

func (v *addonValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.AddonResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.Addon{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
			admissionregv1.Delete,
		},
	}
}

// Do not allow one addon to be created twice
func (v *addonValidator) Create(_ *types.Request, newObj runtime.Object) error {
	newAddon := newObj.(*v1beta1.Addon)

	return v.validateNewAddon(newAddon)
}

// Do not allow some fields to be changed, or set to non-existing values
func (v *addonValidator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	newAddon := newObj.(*v1beta1.Addon)
	oldAddon := oldObj.(*v1beta1.Addon)

	return v.validateUpdatedAddon(newAddon, oldAddon)
}

func (v *addonValidator) validateNewAddon(newAddon *v1beta1.Addon) error {
	addonList, err := v.addons.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("cannot list addons, err: %+v", err))
	}

	for _, addon := range addonList {
		if addon.Spec.Chart == newAddon.Spec.Chart {
			return werror.NewConflict(fmt.Sprintf("addon with Chart %q has been created, cannot create a new one", addon.Spec.Chart))
		}
	}

	return nil
}

func (v *addonValidator) validateUpdatedAddon(newAddon *v1beta1.Addon, oldAddon *v1beta1.Addon) error {
	if newAddon.Spec.Chart != oldAddon.Spec.Chart {
		return werror.NewBadRequest("chart field cannot be changed.")
	}

	if v1beta1.AddonOperationInProgress.IsTrue(oldAddon) {
		return werror.NewBadRequest(fmt.Sprintf("cannot perform operation, as an existing operation is in progress on addon %s", oldAddon.Name))
	}

	if newAddon.Name == vClusterAddonName && newAddon.Namespace == vClusterAddonNamespace && newAddon.Spec.Enabled {
		return validateVClusterAddon(newAddon)
	}

	if newAddon.Name == util.RancherLoggingName {
		if oldAddon.Spec.Enabled == newAddon.Spec.Enabled {
			// spec `enabled` is not changed
			return nil
		}
		skip := newAddon.Annotations[util.AnnotationSkipRancherLoggingAddonWebhookCheck] == "true"
		// check when addon is `enabled`
		//   block if upgradeLog has deployed a managedchart as logging-operator
		if newAddon.Spec.Enabled {
			if skip {
				logrus.Warnf("%v addon is enabled but webhook check is skipped", util.RancherLoggingName)
				return nil
			}
			return v.validateEnableRancherLoggingAddon(newAddon)
		}

		// check when addon is `disabled`
		//   block if upgradeLog sits on top of rancher-logging addon
		if skip {
			logrus.Warnf("%v addon is disabled but webhook check is skipped", util.RancherLoggingName)
			return nil
		}
		return v.validateDisableRancherLoggingAddon(newAddon)
	}

	if newAddon.Name == util.DeschedulerName && newAddon.Spec.Enabled {
		if newAddon.Annotations != nil && newAddon.Annotations[util.AnnotationSkipDeschedulerAddonWebhookCheck] == "true" {
			return nil
		}

		return v.validateDeschedulerAddon(newAddon)
	}

	return nil
}

func validateVClusterAddon(newAddon *v1beta1.Addon) error {
	type contentValues struct {
		Hostname string `yaml:"hostname,omitempty"`
		Global   struct {
			Hostname string `yaml:"hostname,omitempty"`
		} `yaml:"global,omitempty"`
	}

	addonContent := &contentValues{}
	// valuesContent contains a yaml string
	if err := yaml.Unmarshal([]byte(newAddon.Spec.ValuesContent), addonContent); err != nil {
		return werror.NewInternalError(fmt.Sprintf("unable to parse contentValues: %v for %s addon", err, vClusterAddonName))
	}

	// currently we only support v0.19.0 and v0.30.0 of vcluster
	// the parsing is designed to handle only these two versions for now
	var hostname string
	if newAddon.Spec.Version == vCluster0190 {
		hostname = addonContent.Hostname
	} else {
		hostname = addonContent.Global.Hostname
	}
	// ip addresses are valid fqdns
	// this check will return error if hostname is fqdn
	// but an ip address
	if fqdnErrs := validationutil.IsFullyQualifiedDomainName(field.NewPath(""), hostname); len(fqdnErrs) == 0 {
		if ipErrs := validationutil.IsValidIP(field.NewPath(""), hostname); len(ipErrs) == 0 {
			return werror.NewBadRequest(fmt.Sprintf("%s is not a valid hostname", hostname))
		}
		return nil
	}

	return werror.NewBadRequest(fmt.Sprintf("invalid fqdn %s provided for %s addon", addonContent.Hostname, vClusterAddonName))
}

func (v *addonValidator) validateEnableRancherLoggingAddon(newAddon *v1beta1.Addon) error {
	loger := logging.NewLogging(v.flowCache, v.outputCache, v.clusterFlowCache, v.clusterOutputCache)

	if err := loger.FlowsDangling(newAddon.Namespace); err != nil {
		return werror.NewBadRequest(fmt.Sprintf("%s, fix or delete it before enabling addon", err.Error()))
	}

	if err := loger.ClusterFlowsDangling(newAddon.Namespace); err != nil {
		return werror.NewBadRequest(fmt.Sprintf("%s, fix or delete it before enabling addon", err.Error()))
	}

	// when rancher-logging is disabled, then upgradeLog deploys a managedchart as the logging operator
	// block the enabling until upgradeLog is gone to avoid issues during addon helm install
	upgradeLogRunning, namespacedName, err := v.isUpgradeLogRunning()
	if err != nil {
		return err
	}

	if upgradeLogRunning {
		return werror.NewBadRequest(fmt.Sprintf("%v addon cannot be enabled as upgradeLog %v exists in the cluster, wait until the Harvester upgrade is finished or removed", util.RancherLoggingName, namespacedName))
	}

	return nil
}

func (v *addonValidator) validateDisableRancherLoggingAddon(newAddon *v1beta1.Addon) error {
	// if rancher-logging is enabled, then upgradeLog utilizes it
	// block the disabling until upgradeLog is gone
	upgradeLogRunning, namespacedName, err := v.isUpgradeLogRunning()
	if err != nil {
		return err
	}

	if upgradeLogRunning {
		return werror.NewBadRequest(fmt.Sprintf("%v addon cannot be disabled as upgradeLog %v exists in the cluster, wait until the Harvester upgrade is finished or removed", util.RancherLoggingName, namespacedName))
	}

	return nil
}

func (v *addonValidator) isUpgradeLogRunning() (bool, string, error) {
	// validate no `upgradeLog` CRs exist as they deployed rancher-logging as a managedchart
	// so we need to block enablement to avoid issues during addon helm install
	upgradeLogList, err := v.upgradeLogCache.List(util.HarvesterSystemNamespaceName, labels.Everything())
	if err != nil {
		return false, "", werror.NewBadRequest(fmt.Sprintf("error list upgradeLog objects: %v", err.Error()))
	}

	if len(upgradeLogList) > 0 {
		return true, fmt.Sprintf("%s/%s", upgradeLogList[0].Namespace, upgradeLogList[0].Name), nil
	}

	return false, "", nil
}

func (v *addonValidator) Delete(_ *types.Request, oldObj runtime.Object) error {
	oldAddon := oldObj.(*v1beta1.Addon)
	if oldAddon == nil {
		return nil
	}
	// don't allow delete non-experimental addons
	//  strictly protect rancher-monitoring and rancher-logging
	if oldAddon.Name == util.RancherLoggingName || oldAddon.Name == util.RancherMonitoringName || oldAddon.Labels[util.AddonExperimentalLabel] != "true" {
		return werror.NewBadRequest(fmt.Sprintf("%v/%v addon cannot be deleted", oldAddon.Namespace, oldAddon.Name))
	}
	return nil
}

func (v *addonValidator) validateDeschedulerAddon(newAddon *v1beta1.Addon) error {
	nodes, err := v.nodeCache.List(labels.Everything())
	if err != nil {
		return werror.NewBadRequest(fmt.Sprintf("error listing nodes: %v", err.Error()))
	}

	if len(nodes) <= 1 {
		return werror.NewBadRequest("descheduler addon cannot be enabled as not enough nodes exist in the cluster")
	}
	return nil
}
