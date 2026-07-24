package addon

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubeovnv1 "github.com/harvester/harvester/pkg/generated/controllers/kubeovn.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctlloggingv1 "github.com/harvester/harvester/pkg/generated/controllers/logging.banzaicloud.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/logging"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	vClusterAddonName      = "rancher-vcluster"
	vClusterAddonNamespace = "rancher-vcluster"
	vCluster0190           = "v0.19.0"
	vCluster0300           = "v0.30.0"
	kubeOVNOperatorAddon   = util.KubeOVNOperatorName

	labelParentSRIOVGPUDevice = "harvesterhci.io/parentSRIOVGPUDevice"
)

var vgpuDeviceGVR = schema.GroupVersionResource{
	Group:    "devices.harvesterhci.io",
	Version:  "v1beta1",
	Resource: "vgpudevices",
}

var (
	storageClassGVK          = storagev1.SchemeGroupVersion.WithKind("StorageClass")
	pvcGVK                   = corev1.SchemeGroupVersion.WithKind("PersistentVolumeClaim")
	volumeSnapshotGVK        = snapshotv1.SchemeGroupVersion.WithKind("VolumeSnapshot")
	volumeSnapshotClassGVK   = snapshotv1.SchemeGroupVersion.WithKind("VolumeSnapshotClass")
	volumeSnapshotContentGVK = snapshotv1.SchemeGroupVersion.WithKind("VolumeSnapshotContent")
	blockDeviceGVK           = v1beta1.SchemeGroupVersion.WithKind("BlockDevice")
)

func NewValidator(
	addons ctlharvesterv1.AddonCache,
	flowCache ctlloggingv1.FlowCache,
	outputCache ctlloggingv1.OutputCache,
	clusterFlowCache ctlloggingv1.ClusterFlowCache,
	clusterOutputCache ctlloggingv1.ClusterOutputCache,
	upgradeLogCache ctlharvesterv1.UpgradeLogCache,
	nodeCache ctlcorev1.NodeCache,
	vmCache ctlkubevirtv1.VirtualMachineCache,
	kubeovnSubnet ctlkubeovnv1.SubnetCache,
	scCache ctlstoragev1.StorageClassCache,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	vsCache ctlsnapshotv1.VolumeSnapshotCache,
	vscCache ctlsnapshotv1.VolumeSnapshotContentCache,
	vsClassCache ctlsnapshotv1.VolumeSnapshotClassCache,
	k8sClient client.Client,
) types.Validator {
	return &addonValidator{
		addons:                     addons,
		flowCache:                  flowCache,
		outputCache:                outputCache,
		clusterFlowCache:           clusterFlowCache,
		clusterOutputCache:         clusterOutputCache,
		upgradeLogCache:            upgradeLogCache,
		nodeCache:                  nodeCache,
		vmCache:                    vmCache,
		kubeovnSubnet:              kubeovnSubnet,
		storageClassCache:          scCache,
		pvcCache:                   pvcCache,
		volumeSnapshotCache:        vsCache,
		volumeSnapshotContentCache: vscCache,
		volumeSnapshotClassCache:   vsClassCache,
		k8sClient:                  k8sClient,
	}
}

type addonValidator struct {
	types.DefaultValidator

	addons                     ctlharvesterv1.AddonCache
	flowCache                  ctlloggingv1.FlowCache
	outputCache                ctlloggingv1.OutputCache
	clusterFlowCache           ctlloggingv1.ClusterFlowCache
	clusterOutputCache         ctlloggingv1.ClusterOutputCache
	upgradeLogCache            ctlharvesterv1.UpgradeLogCache
	nodeCache                  ctlcorev1.NodeCache
	vmCache                    ctlkubevirtv1.VirtualMachineCache
	kubeovnSubnet              ctlkubeovnv1.SubnetCache
	storageClassCache          ctlstoragev1.StorageClassCache
	pvcCache                   ctlcorev1.PersistentVolumeClaimCache
	volumeSnapshotCache        ctlsnapshotv1.VolumeSnapshotCache
	volumeSnapshotContentCache ctlsnapshotv1.VolumeSnapshotContentCache
	volumeSnapshotClassCache   ctlsnapshotv1.VolumeSnapshotClassCache
	k8sClient                  client.Client
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
	// Validate common fields
	if newAddon.Spec.Chart != oldAddon.Spec.Chart {
		return werror.NewBadRequest("chart field cannot be changed.")
	}

	if v1beta1.AddonOperationInProgress.IsTrue(oldAddon) {
		return werror.NewBadRequest(fmt.Sprintf("cannot perform operation, as an existing operation is in progress on addon %s", oldAddon.Name))
	}

	switch newAddon.Name {
	case vClusterAddonName:
		return v.validateVClusterAddonUpdate(newAddon, oldAddon)
	case util.RancherLoggingName:
		return v.validateRancherLoggingAddonUpdate(newAddon, oldAddon)
	case util.DeschedulerName:
		return v.validateDeschedulerAddonUpdate(newAddon, oldAddon)
	case util.PCIDevicesControllerName:
		return v.validatePCIDevicesControllerAddonUpdate(newAddon, oldAddon)
	case util.NvidiaDriverToolkitName:
		return v.validateNvidiaDriverToolkitAddonUpdate(newAddon, oldAddon)
	case util.KubeOVNOperatorName:
		return v.validateKubeOVNAddonUpdate(newAddon, oldAddon)
	case util.HarvesterCSIDriverLVMName:
		return v.validateLVMAddonUpdate(newAddon, oldAddon)
	}

	return nil
}

func (v *addonValidator) validateVClusterAddonUpdate(newAddon *v1beta1.Addon, oldAddon *v1beta1.Addon) error {
	if newAddon.Namespace != vClusterAddonNamespace || !newAddon.Spec.Enabled {
		return nil
	}
	return validateVClusterAddon(newAddon)
}

func (v *addonValidator) validateRancherLoggingAddonUpdate(newAddon *v1beta1.Addon, oldAddon *v1beta1.Addon) error {
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

func (v *addonValidator) validateDeschedulerAddonUpdate(newAddon *v1beta1.Addon, oldAddon *v1beta1.Addon) error {
	if !newAddon.Spec.Enabled {
		return nil
	}

	if newAddon.Annotations != nil && newAddon.Annotations[util.AnnotationSkipDeschedulerAddonWebhookCheck] == "true" {
		return nil
	}

	return v.validateDeschedulerAddon(newAddon)
}

func (v *addonValidator) validatePCIDevicesControllerAddonUpdate(newAddon *v1beta1.Addon, oldAddon *v1beta1.Addon) error {
	// not being disabled, no validation needed
	if !isAddonBeingDisabled(newAddon, oldAddon) {
		return nil
	}

	// check for skip annotation
	// 1. some users may accidentally disable the addon and start VMs without detaching devices
	// 2. some users may want to forcibly disable the addon to initialize their devices state when vm is in stopped state
	// in general, we hope users can detach devices from VMs before disabling the addon for safety and understanding what they are doing
	// We check VM for case 1 and provide skip annotation for case 2
	if newAddon.Annotations != nil && newAddon.Annotations[util.AnnotationSkipPCIDevicesControllerAddonWebhookCheck] == "true" {
		logrus.Warnf("%v addon is being disabled but webhook check is skipped", util.PCIDevicesControllerName)
		return nil
	}

	// perform validation when pcidevices-controller addon is being disabled
	return v.validatePCIDevicesControllerAddon()
}

func (v *addonValidator) validateNvidiaDriverToolkitAddonUpdate(newAddon *v1beta1.Addon, oldAddon *v1beta1.Addon) error {
	// not being disabled, no validation needed
	if !isAddonBeingDisabled(newAddon, oldAddon) {
		return nil
	}

	// check for skip annotation
	if newAddon.Annotations != nil && newAddon.Annotations[util.AnnotationSkipNvidiaDriverToolkitAddonWebhookCheck] == "true" {
		logrus.Warnf("%v addon is being disabled but webhook check is skipped", util.NvidiaDriverToolkitName)
		return nil
	}

	// perform validation when nvidia-driver-toolkit addon is being disabled
	return v.validateNvidiaDriverToolkitAddon()
}

func (v *addonValidator) validateLVMAddonUpdate(newAddon *v1beta1.Addon, oldAddon *v1beta1.Addon) error {
	// addon not being disabled, no validation needed
	if !isAddonBeingDisabled(newAddon, oldAddon) {
		return nil
	}

	return v.validateDisableLVMAddon()
}

func isAddonBeingDisabled(newAddon *v1beta1.Addon, oldAddon *v1beta1.Addon) bool {
	return oldAddon.Spec.Enabled && !newAddon.Spec.Enabled
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
	//  strictly protect rancher-monitoring and rancher-logging and kubeovn-operator
	if oldAddon.Name == util.KubeOVNOperatorName || oldAddon.Name == util.RancherLoggingName || oldAddon.Name == util.RancherMonitoringName || oldAddon.Labels[util.AddonExperimentalLabel] != "true" {
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

func (v *addonValidator) validatePCIDevicesControllerAddon() error {
	vms, err := v.vmCache.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("error listing virtual machines: %v", err.Error()))
	}

	var vmsWithDevices []string
	for _, vm := range vms {
		if vm.Spec.Template == nil {
			continue
		}
		if len(vm.Spec.Template.Spec.Domain.Devices.HostDevices) > 0 {
			vmsWithDevices = append(vmsWithDevices, fmt.Sprintf("%s/%s", vm.Namespace, vm.Name))
		}
	}

	if len(vmsWithDevices) > 0 {
		return werror.NewBadRequest(fmt.Sprintf("pcidevices-controller addon cannot be disabled as the following VMs are using passthrough devices: %v", vmsWithDevices))
	}

	return nil
}

func (v *addonValidator) validateNvidiaDriverToolkitAddon() error {
	vms, err := v.vmCache.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("error listing virtual machines: %v", err.Error()))
	}

	// Collect all HostDevice names across all VMs
	hostDeviceNames := map[string]string{} // device name -> vm namespace/name
	for _, vm := range vms {
		if vm.Spec.Template == nil {
			continue
		}
		for _, hd := range vm.Spec.Template.Spec.Domain.Devices.HostDevices {
			hostDeviceNames[hd.Name] = fmt.Sprintf("%s/%s", vm.Namespace, vm.Name)
		}
	}

	if len(hostDeviceNames) == 0 {
		return nil
	}

	// List all VGPUDevice resources that have the parentSRIOVGPUDevice label,
	// which indicates they are vGPU devices managed by nvidia-driver-toolkit.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vgpuList := &unstructured.UnstructuredList{}
	vgpuList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   vgpuDeviceGVR.Group,
		Version: vgpuDeviceGVR.Version,
		Kind:    "VGPUDeviceList",
	})
	if err := v.k8sClient.List(ctx, vgpuList, client.HasLabels{labelParentSRIOVGPUDevice}); err != nil {
		return werror.NewInternalError(fmt.Sprintf("error listing VGPUDevices: %v", err.Error()))
	}

	// Build a set of VGPUDevice names for fast lookup
	vgpuDeviceNames := make(map[string]struct{}, len(vgpuList.Items))
	for _, item := range vgpuList.Items {
		vgpuDeviceNames[item.GetName()] = struct{}{}
	}

	// Check if any VM HostDevice matches a VGPUDevice
	var vmsWithVGPU []string
	for devName, vmRef := range hostDeviceNames {
		if _, ok := vgpuDeviceNames[devName]; ok {
			vmsWithVGPU = append(vmsWithVGPU, vmRef)
		}
	}

	if len(vmsWithVGPU) > 0 {
		return werror.NewBadRequest(fmt.Sprintf("nvidia-driver-toolkit addon cannot be disabled as the following VMs are using vGPU devices: %v", vmsWithVGPU))
	}

	return nil
}

func (v *addonValidator) validateDisableLVMAddon() error {
	var blockers []string
	var storageClasses map[string]struct{}
	var volumeSnapshotClasses map[string]struct{}

	toBlocker := func(kind, listErr string, names []string, err error) (string, error) {
		if err != nil {
			return "", werror.NewInternalError(fmt.Sprintf("%s: %v", listErr, err))
		}
		if len(names) == 0 {
			return "", nil
		}
		return formatLVMBlocker(kind, names), nil
	}

	checks := []func() (string, error){
		func() (string, error) {
			var names []string
			var err error
			storageClasses, names, err = v.getLVMStorageClasses()
			return toBlocker(storageClassGVK.Kind, "failed to list storage classes", names, err)
		},
		func() (string, error) {
			names, err := v.getLVMPVCs(storageClasses)
			return toBlocker(pvcGVK.Kind, "failed to list persistent volume claims", names, err)
		},
		func() (string, error) {
			var names []string
			var err error
			volumeSnapshotClasses, names, err = v.getLVMVolumeSnapshotClasses()
			return toBlocker(volumeSnapshotClassGVK.Kind, "failed to list volume snapshot classes", names, err)
		},
		func() (string, error) {
			names, err := v.getLVMVolumeSnapshots(volumeSnapshotClasses)
			return toBlocker(volumeSnapshotGVK.Kind, "failed to list volume snapshots", names, err)
		},
		func() (string, error) {
			names, err := v.getLVMVolumeSnapshotContents()
			return toBlocker(volumeSnapshotContentGVK.Kind, "failed to list volume snapshot contents", names, err)
		},
		func() (string, error) {
			names, err := v.getLVMBlockDevices()
			return toBlocker(blockDeviceGVK.Kind, "failed to list block devices", names, err)
		},
	}

	for _, check := range checks {
		blocker, err := check()
		if err != nil {
			return err
		}
		if blocker != "" {
			blockers = append(blockers, blocker)
		}
	}
	if len(blockers) > 0 {
		return werror.NewBadRequest(formatLVMAddonBlockersError(blockers))
	}

	return nil
}

func (v *addonValidator) getLVMStorageClasses() (map[string]struct{}, []string, error) {
	storageClasses, err := v.storageClassCache.List(labels.Everything())
	if err != nil {
		return nil, nil, err
	}

	storageClassNames := map[string]struct{}{}
	var blockers []string
	for _, sc := range storageClasses {
		if sc.Provisioner != util.CSIProvisionerLVM {
			continue
		}

		storageClassNames[sc.Name] = struct{}{}
		if isLVMAddonHelmManaged(sc) {
			continue
		}
		blockers = append(blockers, sc.Name)
	}

	sort.Strings(blockers)
	return storageClassNames, blockers, nil
}

func (v *addonValidator) getLVMPVCs(lvmStorageClasses map[string]struct{}) ([]string, error) {
	pvcs, err := v.pvcCache.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return nil, err
	}

	var blockers []string
	for _, pvc := range pvcs {
		if isLVMPersistentVolumeClaim(pvc, lvmStorageClasses) {
			blockers = append(blockers, objectNamespacedName(pvc))
		}
	}

	sort.Strings(blockers)
	return blockers, nil
}

func (v *addonValidator) getLVMVolumeSnapshotClasses() (map[string]struct{}, []string, error) {
	volumeSnapshotClasses, err := v.volumeSnapshotClassCache.List(labels.Everything())
	if err != nil {
		return nil, nil, err
	}

	volumeSnapshotClassNames := map[string]struct{}{}
	var blockers []string
	for _, volumeSnapshotClass := range volumeSnapshotClasses {
		if volumeSnapshotClass.Driver != util.CSIProvisionerLVM {
			continue
		}

		volumeSnapshotClassNames[volumeSnapshotClass.Name] = struct{}{}
		if isLVMAddonHelmManaged(volumeSnapshotClass) {
			continue
		}
		blockers = append(blockers, volumeSnapshotClass.Name)
	}

	sort.Strings(blockers)
	return volumeSnapshotClassNames, blockers, nil
}

func (v *addonValidator) getLVMVolumeSnapshots(lvmVolumeSnapshotClasses map[string]struct{}) ([]string, error) {
	volumeSnapshots, err := v.volumeSnapshotCache.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return nil, err
	}

	var blockers []string
	for _, volumeSnapshot := range volumeSnapshots {
		if volumeSnapshot.Spec.VolumeSnapshotClassName == nil {
			continue
		}
		if _, ok := lvmVolumeSnapshotClasses[*volumeSnapshot.Spec.VolumeSnapshotClassName]; ok {
			blockers = append(blockers, objectNamespacedName(volumeSnapshot))
		}
	}

	sort.Strings(blockers)
	return blockers, nil
}

func (v *addonValidator) getLVMVolumeSnapshotContents() ([]string, error) {
	volumeSnapshotContents, err := v.volumeSnapshotContentCache.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var blockers []string
	for _, volumeSnapshotContent := range volumeSnapshotContents {
		if volumeSnapshotContent.Spec.Driver == util.CSIProvisionerLVM {
			blockers = append(blockers, volumeSnapshotContent.Name)
		}
	}

	sort.Strings(blockers)
	return blockers, nil
}

func (v *addonValidator) getLVMBlockDevices() ([]string, error) {
	blockDevices, err := v.listBlockDevices()
	if err != nil {
		return nil, err
	}

	var blockers []string
	for _, blockDevice := range blockDevices {
		provisioned, found, err := unstructured.NestedBool(blockDevice.Object, "spec", "provision")
		if err != nil {
			return nil, err
		}
		if !found || !provisioned {
			continue
		}

		if _, found, err = unstructured.NestedMap(blockDevice.Object, "spec", "provisioner", "lvm"); err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		blockers = append(blockers, objectNamespacedName(&blockDevice))
	}

	sort.Strings(blockers)
	return blockers, nil
}

func (v *addonValidator) listBlockDevices() ([]unstructured.Unstructured, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	blockDeviceList := &unstructured.UnstructuredList{}
	blockDeviceList.SetGroupVersionKind(blockDeviceGVK.GroupVersion().WithKind(blockDeviceGVK.Kind + "List"))
	if err := v.k8sClient.List(ctx, blockDeviceList); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil, nil
		}
		return nil, err
	}

	return blockDeviceList.Items, nil
}

func isLVMPersistentVolumeClaim(pvc *corev1.PersistentVolumeClaim, lvmStorageClasses map[string]struct{}) bool {
	if pvc.Spec.StorageClassName != nil {
		if _, ok := lvmStorageClasses[*pvc.Spec.StorageClassName]; ok {
			return true
		}
	}

	if pvc.Annotations[util.AnnStorageProvisioner] == util.CSIProvisionerLVM {
		return true
	}

	return pvc.Annotations[util.AnnBetaStorageProvisioner] == util.CSIProvisionerLVM
}

func isLVMAddonHelmManaged(obj metav1.Object) bool {
	annotations := obj.GetAnnotations()
	return annotations[util.HelmReleaseNameAnnotation] == util.HarvesterCSIDriverLVMName &&
		annotations[util.HelmReleaseNamespaceAnnotation] == util.HarvesterSystemNamespaceName
}

func objectNamespacedName(obj metav1.Object) string {
	if obj.GetNamespace() == "" {
		return obj.GetName()
	}
	return fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
}

func formatLVMBlocker(kind string, names []string) string {
	return fmt.Sprintf("%s %v", kind, names)
}

func formatLVMAddonBlockersError(blockers []string) string {
	return fmt.Sprintf(
		"%s addon cannot be disabled as LVM resources still exist: %s. Delete these resources before disabling the addon",
		util.HarvesterCSIDriverLVMName,
		strings.Join(blockers, "; "),
	)
}

// restrict disabling kubeovn-operator addon when VMs are using the overlay networks provided by kubeovn.
func (v *addonValidator) validateKubeOVNAddonUpdate(newAddon, oldAddon *v1beta1.Addon) error {
	// addon not being disabled, no validation needed
	if !isAddonBeingDisabled(newAddon, oldAddon) {
		return nil
	}

	//subnet crds already removed, return no-op
	if v.kubeovnSubnet == nil {
		return nil
	}

	subnets, err := v.kubeovnSubnet.List("", labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to retrieve subnets err=%v", err))
	}

	// V4UsingIPs and V6UsingIPs will be non-zero both when the VM is running and
	// when it is stopped, as long as it is still attached to an overlay network.
	for _, subnet := range subnets {
		if subnet.Status.V4UsingIPs != 0 || subnet.Status.V6UsingIPs != 0 {
			return werror.NewBadRequest(fmt.Sprintf("kubeovn-operator addon cannot be disabled as VMs attached to overlay network %s in subnet %s are still in use, delete the VMs before disabling the addon", subnet.Spec.Provider, subnet.Name))
		}
	}

	return nil
}
