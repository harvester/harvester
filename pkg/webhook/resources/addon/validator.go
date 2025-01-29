package addon

import (
	"fmt"

	yaml "gopkg.in/yaml.v2"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlloggingv1 "github.com/harvester/harvester/pkg/generated/controllers/logging.banzaicloud.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/logging"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	vClusterAddonName      = "rancher-vcluster"
	vClusterAddonNamespace = "rancher-vcluster"
)

func NewValidator(addons ctlharvesterv1.AddonCache, flowCache ctlloggingv1.FlowCache, outputCache ctlloggingv1.OutputCache, clusterFlowCache ctlloggingv1.ClusterFlowCache, clusterOutputCache ctlloggingv1.ClusterOutputCache) types.Validator {
	return &addonValidator{
		addons:             addons,
		flowCache:          flowCache,
		outputCache:        outputCache,
		clusterFlowCache:   clusterFlowCache,
		clusterOutputCache: clusterOutputCache,
	}
}

type addonValidator struct {
	types.DefaultValidator

	addons             ctlharvesterv1.AddonCache
	flowCache          ctlloggingv1.FlowCache
	outputCache        ctlloggingv1.OutputCache
	clusterFlowCache   ctlloggingv1.ClusterFlowCache
	clusterOutputCache ctlloggingv1.ClusterOutputCache
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

	if newAddon.Name == util.RancherLoggingName && newAddon.Spec.Enabled {
		if newAddon.Annotations != nil && newAddon.Annotations[util.AnnotationSkipRancherLoggingAddonWebhookCheck] == "true" {
			return nil
		}

		return v.validateRancherLoggingAddon(newAddon)
	}

	return nil
}

func validateVClusterAddon(newAddon *v1beta1.Addon) error {
	type contentValues struct {
		Hostname string `yaml:"hostname"`
	}

	addonContent := &contentValues{}

	// valuesContent contains a yaml string
	if err := yaml.Unmarshal([]byte(newAddon.Spec.ValuesContent), addonContent); err != nil {
		return werror.NewInternalError(fmt.Sprintf("unable to parse contentValues: %v for %s addon", err, vClusterAddonName))
	}

	// ip addresses are valid fqdns
	// this check will return error if hostname is fqdn
	// but an ip address
	if fqdnErrs := validationutil.IsFullyQualifiedDomainName(field.NewPath(""), addonContent.Hostname); len(fqdnErrs) == 0 {
		if ipErrs := validationutil.IsValidIP(field.NewPath(""), addonContent.Hostname); len(ipErrs) == 0 {
			return werror.NewBadRequest(fmt.Sprintf("%s is not a valid hostname", addonContent.Hostname))
		}
		return nil
	}

	return werror.NewBadRequest(fmt.Sprintf("invalid fqdn %s provided for %s addon", addonContent.Hostname, vClusterAddonName))
}

func (v *addonValidator) validateRancherLoggingAddon(newAddon *v1beta1.Addon) error {
	loger := logging.NewLogging(v.flowCache, v.outputCache, v.clusterFlowCache, v.clusterOutputCache)

	if err := loger.FlowsDangling(newAddon.Namespace); err != nil {
		return werror.NewBadRequest(fmt.Sprintf("%s, fix or delete it before enabling addon", err.Error()))
	}

	if err := loger.ClusterFlowsDangling(newAddon.Namespace); err != nil {
		return werror.NewBadRequest(fmt.Sprintf("%s, fix or delete it before enabling addon", err.Error()))
	}

	return nil
}
