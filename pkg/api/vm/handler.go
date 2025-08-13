package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/gorilla/mux"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/apierror"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	wranglername "github.com/rancher/wrangler/v3/pkg/name"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/rancher/wrangler/v3/pkg/slice"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdicommon "kubevirt.io/containerized-data-importer/pkg/controller/common"
	kubevirtutil "kubevirt.io/kubevirt/pkg/virt-operator/util"

	apiutil "github.com/harvester/harvester/pkg/api/util"
	volumeapi "github.com/harvester/harvester/pkg/api/volume"
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/builder"
	nodecontroller "github.com/harvester/harvester/pkg/controller/master/node"
	ctlcdiv1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/drainhelper"
	"github.com/harvester/harvester/pkg/util/virtualmachineinstance"
)

const (
	vmResource    = "virtualmachines"
	vmiResource   = "virtualmachineinstances"
	sshAnnotation = "harvesterhci.io/sshNames"
)

var (
	cloneVMAnnotationKeys = []string{
		util.AnnotationReservedMemory,
		util.AnnotationEnableCPUAndMemoryHotplug,
	}
)

type vmActionHandler struct {
	namespace                 string
	datavolumeClient          ctlcdiv1.DataVolumeClient
	kubevirtCache             ctlkubevirtv1.KubeVirtCache
	vms                       ctlkubevirtv1.VirtualMachineClient
	vmis                      ctlkubevirtv1.VirtualMachineInstanceClient
	vmCache                   ctlkubevirtv1.VirtualMachineCache
	vmiCache                  ctlkubevirtv1.VirtualMachineInstanceCache
	vmims                     ctlkubevirtv1.VirtualMachineInstanceMigrationClient
	vmTemplateClient          ctlharvesterv1.VirtualMachineTemplateClient
	vmTemplateVersionClient   ctlharvesterv1.VirtualMachineTemplateVersionClient
	vmimCache                 ctlkubevirtv1.VirtualMachineInstanceMigrationCache
	backups                   ctlharvesterv1.VirtualMachineBackupClient
	backupCache               ctlharvesterv1.VirtualMachineBackupCache
	restores                  ctlharvesterv1.VirtualMachineRestoreClient
	settingCache              ctlharvesterv1.SettingCache
	nadCache                  ctlcniv1.NetworkAttachmentDefinitionCache
	nodeCache                 ctlcorev1.NodeCache
	pvcCache                  ctlcorev1.PersistentVolumeClaimCache
	pvCache                   ctlcorev1.PersistentVolumeCache
	secretClient              ctlcorev1.SecretClient
	secretCache               ctlcorev1.SecretCache
	virtSubresourceRestClient rest.Interface
	virtRestClient            rest.Interface
	vmImages                  ctlharvesterv1.VirtualMachineImageClient
	vmImageCache              ctlharvesterv1.VirtualMachineImageCache
	storageClassCache         ctlstoragev1.StorageClassCache
	resourceQuotaClient       ctlharvesterv1.ResourceQuotaClient
	clientSet                 kubernetes.Clientset
}

func (h vmActionHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if err := h.doAction(rw, req); err != nil {
		status := http.StatusInternalServerError
		if e, ok := err.(*apierror.APIError); ok {
			status = e.Code.Status
		}
		rw.WriteHeader(status)
		_, _ = rw.Write([]byte(err.Error()))
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

func (h *vmActionHandler) doAction(rw http.ResponseWriter, r *http.Request) error {
	vars := util.EncodeVars(mux.Vars(r))
	action := vars["action"]
	namespace := vars["namespace"]
	name := vars["name"]

	user, ok := request.UserFrom(r.Context())
	if !ok {
		return apierror.NewAPIError(validation.Unauthorized, "failed to get user from request")
	}

	switch action {
	case ejectCdRom:
		var input EjectCdRomActionInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}

		if len(input.DiskNames) == 0 {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter diskNames is empty")
		}

		return h.ejectCdRom(r.Context(), name, namespace, input.DiskNames)
	case migrate:
		var input MigrateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		return h.migrate(r.Context(), namespace, name, input.NodeName)
	case abortMigration:
		return h.abortMigration(namespace, name)
	case findMigratableNodes:
		return h.findMigratableNodes(rw, namespace, name)
	case startVM, restartVM:
		if err := h.subresourceOperate(r.Context(), vmResource, namespace, name, action); err != nil {
			return fmt.Errorf("%s virtual machine %s/%s failed, %v", action, namespace, name, err)
		}
	case stopVM:
		// To align behavior with kubevirt v1.1.1, we set runStrategy to Halted when stopping a VM.
		if err := h.stopVM(namespace, name); err != nil {
			return fmt.Errorf("%s virtual machine %s/%s failed, %v", action, namespace, name, err)
		}
	case pauseVM, unpauseVM, softReboot:
		if err := h.subresourceOperate(r.Context(), vmiResource, namespace, name, action); err != nil {
			return fmt.Errorf("%s virtual machine %s/%s failed, %v", action, namespace, name, err)
		}
	case backupVM:
		var input BackupInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}

		if input.Name == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter backup name is required")
		}

		if err := h.checkBackupTargetConfigured(); err != nil {
			return err
		}

		if err := h.createVMBackup(name, namespace, input); err != nil {
			return err
		}
		return nil
	case snapshotVM:
		// TODO: currently the snapshot CRD creation is handled by UI, we do nothing here.
		return nil
	case restoreVM:
		var input RestoreInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}

		if input.Name == "" || input.BackupName == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter name and backupName are required")
		}

		if err := h.checkBackupTargetConfigured(); err != nil {
			return err
		}

		if err := h.restoreBackup(name, namespace, input); err != nil {
			return err
		}
		return nil
	case createTemplate:
		var input CreateTemplateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}

		if input.Name == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Template name is required")
		}
		return h.createTemplate(namespace, name, input)
	case addVolume:
		var input AddVolumeInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		if input.DiskName == "" || input.VolumeSourceName == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `diskName` and `volumeName` are required")
		}
		return h.addVolume(r.Context(), namespace, name, input)
	case removeVolume:
		var input RemoveVolumeInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		if input.DiskName == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter `volumeName` are required")
		}
		return h.removeVolume(r.Context(), namespace, name, input)
	case cloneVM:
		var input CloneInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}

		if input.TargetVM == "" {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Parameter targetVm are required")
		}

		if err := h.cloneVM(name, namespace, input); err != nil {
			return err
		}
		return nil
	case forceStopVM:
		var gracePeriod int64
		stopOptions := &kubevirtv1.StopOptions{GracePeriod: &gracePeriod}
		body, err := json.Marshal(stopOptions)
		if err != nil {
			return fmt.Errorf("%s virtual machine %s/%s failed, %v", action, namespace, name, err)
		}
		// The request is equal to "virtctl stop my-vm --grace-period 0 --force"
		if err := h.virtSubresourceRestClient.Put().Namespace(namespace).Resource(vmResource).SubResource(stopVM).Name(name).Body(body).Do(r.Context()).Error(); err != nil {
			// Kubevirt returns "Halted does not support manual stop requests" error when VM runStrategy is Halted,
			// but the request will still forcely stop the VM.
			if strings.Contains(err.Error(), "Halted does not support manual stop requests") {
				return nil
			}
			return err
		}
	case dismissInsufficientResourceQuota:
		return h.dismissInsufficientResourceQuota(name, namespace)
	case updateResourceQuotaAction:
		if ok, err := apiutil.CanUpdateResourceQuota(h.clientSet, namespace, user.GetName()); err != nil {
			return apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to check permission: %v", err))
		} else if !ok {
			return apierror.NewAPIError(validation.PermissionDenied, "User does not have permission to update resource quota")
		}
		var updateResourceQuotaInput UpdateResourceQuotaInput
		if err := json.NewDecoder(r.Body).Decode(&updateResourceQuotaInput); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, fmt.Sprintf("Failed to decode request body: %v ", err))
		}
		return h.updateResourceQuota(namespace, name, updateResourceQuotaInput)
	case deleteResourceQuotaAction:
		if ok, err := apiutil.CanUpdateResourceQuota(h.clientSet, namespace, user.GetName()); err != nil {
			return apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to check permission: %v", err))
		} else if !ok {
			return apierror.NewAPIError(validation.PermissionDenied, "User does not have permission to update resource quota")
		}
		return h.deleteResourceQuota(namespace, name)
	case cpuAndMemoryHotplug:
		vm, err := h.vmCache.Get(namespace, name)
		if err != nil {
			return apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to get virtual machine %s/%s: %v", namespace, name, err))
		}
		if !canCPUAndMemoryHotplug(vm) {
			return apierror.NewAPIError(validation.InvalidAction, "CPU and memory hotplug is not supported for this VM")
		}
		var input CPUAndMemoryHotplugInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to decode request body: %v "+err.Error())
		}
		return h.cpuAndMemoryHotplug(namespace, name, input)
	default:
		return apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
	}
	return nil
}

func (h *vmActionHandler) ejectCdRom(ctx context.Context, name, namespace string, diskNames []string) error {
	vm, err := h.vmCache.Get(namespace, name)
	if err != nil {
		return err
	}

	vmCopy := vm.DeepCopy()
	if err := ejectCdRomFromVM(vmCopy, diskNames); err != nil {
		return err
	}

	if !reflect.DeepEqual(vm, vmCopy) {
		if _, err := h.vms.Update(vmCopy); err != nil {
			return err
		}
		return h.subresourceOperate(ctx, vmResource, namespace, name, restartVM)
	}

	return nil
}

func (h *vmActionHandler) startPreCheck(namespace, name string) error {
	vm, err := h.vmCache.Get(namespace, name)
	if err != nil {
		return err
	}

	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvcName := volume.PersistentVolumeClaim.PersistentVolumeClaimVolumeSource.ClaimName
			pvcNamespace := vm.Namespace
			pvc, err := h.pvcCache.Get(pvcNamespace, pvcName)
			if err != nil {
				return err
			}
			if volumeapi.IsResizing(pvc) {
				return fmt.Errorf("can not start the VM %s/%s which has a resizing volume %s/%s", vm.Namespace, vm.Name, pvcNamespace, pvcName)
			}
		}
	}

	return nil
}

func (h *vmActionHandler) subresourceOperate(ctx context.Context, resource, namespace, name, subresourece string) error {
	switch subresourece {
	case startVM:
		if err := h.startPreCheck(namespace, name); err != nil {
			return err
		}
	}

	return h.virtSubresourceRestClient.Put().Namespace(namespace).Resource(resource).SubResource(subresourece).Name(name).Do(ctx).Error()
}

func (h *vmActionHandler) stopVM(namespace, name string) error {
	vm, err := h.vmCache.Get(namespace, name)
	if err != nil {
		return fmt.Errorf("failed to get virtual machine %s/%s: %v", namespace, name, err)
	}

	vmCopy := vm.DeepCopy()
	runStrategy := kubevirtv1.RunStrategyHalted
	vmCopy.Spec.RunStrategy = &runStrategy
	if !reflect.DeepEqual(vm, vmCopy) {
		_, err = h.vms.Update(vmCopy)
		return err
	}
	return nil
}

func ejectCdRomFromVM(vm *kubevirtv1.VirtualMachine, diskNames []string) error {
	disks := make([]kubevirtv1.Disk, 0, len(vm.Spec.Template.Spec.Domain.Devices.Disks))
	for _, disk := range vm.Spec.Template.Spec.Domain.Devices.Disks {
		if slice.ContainsString(diskNames, disk.Name) {
			if disk.CDRom == nil {
				return errors.New("disk " + disk.Name + " isn't a CD-ROM disk")
			}
			continue
		}
		disks = append(disks, disk)
	}

	volumes := make([]kubevirtv1.Volume, 0, len(vm.Spec.Template.Spec.Volumes))
	toRemoveClaimNames := make([]string, 0, len(vm.Spec.Template.Spec.Volumes))
	for _, vol := range vm.Spec.Template.Spec.Volumes {
		if !slice.ContainsString(diskNames, vol.Name) {
			volumes = append(volumes, vol)
			continue
		}
		if vol.VolumeSource.PersistentVolumeClaim != nil {
			toRemoveClaimNames = append(toRemoveClaimNames, vol.VolumeSource.PersistentVolumeClaim.ClaimName)
		}
	}

	if err := removeVolumeClaimTemplatesFromVMAnnotation(vm, toRemoveClaimNames); err != nil {
		return err
	}
	vm.Spec.Template.Spec.Volumes = volumes
	vm.Spec.Template.Spec.Domain.Devices.Disks = disks
	return nil
}

func removeVolumeClaimTemplatesFromVMAnnotation(vm *kubevirtv1.VirtualMachine, toRemoveDiskNames []string) error {
	volumeClaimTemplatesStr, ok := vm.Annotations[util.AnnotationVolumeClaimTemplates]
	if !ok {
		return nil
	}
	var volumeClaimTemplates, toUpdateVolumeClaimTemplates []corev1.PersistentVolumeClaim
	if err := json.Unmarshal([]byte(volumeClaimTemplatesStr), &volumeClaimTemplates); err != nil {
		return err
	}
	for _, volumeClaimTemplate := range volumeClaimTemplates {
		if !slice.ContainsString(toRemoveDiskNames, volumeClaimTemplate.Name) {
			toUpdateVolumeClaimTemplates = append(toUpdateVolumeClaimTemplates, volumeClaimTemplate)
		}
	}
	toUpdateVolumeClaimTemplateBytes, err := json.Marshal(toUpdateVolumeClaimTemplates)
	if err != nil {
		return err
	}
	vm.Annotations[util.AnnotationVolumeClaimTemplates] = string(toUpdateVolumeClaimTemplateBytes)
	return nil
}

func (h *vmActionHandler) migrate(ctx context.Context, namespace, vmName string, nodeName string) error {
	vmi, err := h.vmiCache.Get(namespace, vmName)
	if err != nil {
		return err
	}
	if !vmi.IsRunning() {
		return errors.New("The VM is not in running state")
	}
	if !isReady(vmi) {
		return errors.New("Can't migrate the VM, the VM is not in ready status")
	}

	if err := virtualmachineinstance.ValidateVMMigratable(vmi); err != nil {
		return err
	}

	if ok, err := h.isMigratableNode(nodeName, vmi); err != nil {
		return fmt.Errorf("can't migrate the VM to the node %s: %s", nodeName, err.Error())
	} else if !ok {
		return errors.New("The target node is non-migratable")
	}

	kubevirt, err := h.kubevirtCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	if err != nil {
		return err
	}
	isKubeVirtReady := false
	for _, condition := range kubevirt.Status.Conditions {
		if condition.Type == kubevirtv1.KubeVirtConditionAvailable {
			if condition.Reason == kubevirtutil.ConditionReasonDeploymentReady {
				isKubeVirtReady = condition.Status == corev1.ConditionTrue
			}
			break
		}
	}
	if !isKubeVirtReady {
		return errors.New("KubeVirt is not ready")
	}

	vmim := &kubevirtv1.VirtualMachineInstanceMigration{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: vmName + "-",
			Namespace:    namespace,
		},
		Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{
			VMIName: vmName,
		},
	}
	if nodeName != "" {
		// check node name is valid
		if nodeName == vmi.Status.NodeName {
			return apierror.NewAPIError(validation.InvalidBodyContent, "The VM is currently running on the target node")
		}

		// set vmi node selector before starting the migration
		toUpdateVmi := vmi.DeepCopy()
		if toUpdateVmi.Annotations == nil {
			toUpdateVmi.Annotations = make(map[string]string)
		}
		if toUpdateVmi.Spec.NodeSelector == nil {
			toUpdateVmi.Spec.NodeSelector = make(map[string]string)
		}
		toUpdateVmi.Annotations[util.AnnotationMigrationTarget] = nodeName

		if err := util.VirtClientUpdateVmi(ctx, h.virtRestClient, h.namespace, namespace, vmName, toUpdateVmi); err != nil {
			logrus.Errorf("failed to update vmi %s/%s with migration target %s: %v", namespace, vmName, nodeName, err)
			return err
		}

		vm, err := h.vmCache.Get(namespace, vmName)
		if err != nil {
			return fmt.Errorf("failed to get virtual machine %s/%s: %v", namespace, vmName, err)
		}
		toUpdateVM := vm.DeepCopy()
		if toUpdateVM.Spec.Template.Spec.NodeSelector == nil {
			toUpdateVM.Spec.Template.Spec.NodeSelector = make(map[string]string)
		}

		toUpdateVM.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] = nodeName
		if _, err := h.vms.Update(toUpdateVM); err != nil {
			logrus.Errorf("failed to update VM %s/%s with migration target %s: %v", namespace, vmName, nodeName, err)
			return err
		}
	}

	vmimc, err := h.vmims.Create(vmim)
	if err != nil {
		logrus.Infof("start migration of vm %s/%s to node %s but fail to create vmim %s", namespace, vmName, nodeName, err.Error())
		return err
	}
	logrus.Infof("start migration of vm %s/%s to node %s, vmim %s", namespace, vmName, nodeName, vmimc.Name)
	return nil
}

func (h *vmActionHandler) isMigratableNode(targetNode string, vmi *kubevirtv1.VirtualMachineInstance) (bool, error) {
	if targetNode == "" {
		return true, nil
	}

	nodes, err := h.findMigratableNodesByVMI(vmi)
	if err != nil {
		return false, err
	}

	if len(nodes) == 0 {
		return false, errors.New("no matching migratable nodes found")
	}

	return slice.ContainsString(nodes, targetNode), nil
}

func (h *vmActionHandler) abortMigration(namespace, name string) error {
	vmi, err := h.vmiCache.Get(namespace, name)
	if err != nil {
		return err
	}
	if !canAbortMigrate(vmi) {
		return errors.New("The VM is not in migrating state")
	}

	vmims, err := h.vmimCache.List(namespace, labels.Everything())
	if err != nil {
		return err
	}
	migrationUID := getMigrationUID(vmi)
	for _, vmim := range vmims {
		if migrationUID == string(vmim.UID) {
			if !vmim.IsRunning() {
				return fmt.Errorf("cannot abort the migration as it is in %q phase", vmim.Status.Phase)
			}
			// Migration is aborted by deleting the VMIM object
			logrus.Infof("abort migration of vm %s/%s, delete vmim %s", namespace, name, vmim.Name)
			if err := h.vmims.Delete(namespace, vmim.Name, &metav1.DeleteOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *vmActionHandler) findMigratableNodes(rw http.ResponseWriter, namespace, name string) error {
	vmi, err := h.vmiCache.Get(namespace, name)
	if err != nil {
		return err
	}

	if err := virtualmachineinstance.ValidateVMMigratable(vmi); err != nil {
		return err
	}

	nodes, err := h.findMigratableNodesByVMI(vmi)
	if err != nil {
		return err
	}
	resp := FindMigratableNodesOutput{
		Nodes: nodes,
	}

	util.ResponseOKWithBody(rw, resp)
	return nil
}

func (h *vmActionHandler) findMigratableNodesByVMI(vmi *kubevirtv1.VirtualMachineInstance) ([]string, error) {
	nodeSelector, err := h.getNodeSelectorRequirementFromVMI(vmi)
	if err != nil {
		return nil, err
	}

	nodes, err := h.nodeCache.List(nodeSelector)
	if err != nil || len(nodes) == 0 {
		return nil, err
	}

	// ignore the node where the VM is running
	migratableNodes := make([]string, 0, len(nodes)-1)
	for _, node := range nodes {
		if vmi.Status.NodeName == node.Name {
			continue
		}

		if isDrained(node) {
			continue
		}

		migratableNodes = append(migratableNodes, node.Name)
	}
	return migratableNodes, nil
}

func isDrained(node *corev1.Node) bool {
	if _, ok := node.Annotations[nodecontroller.MaintainStatusAnnotationKey]; ok {
		return ok
	}
	if _, ok := node.Annotations[drainhelper.DrainAnnotation]; ok {
		return ok
	}
	if node.Spec.Unschedulable {
		return true
	}
	if node.Spec.Taints != nil {
		for _, taint := range node.Spec.Taints {
			if taint.Key == corev1.TaintNodeUnreachable || taint.Key == corev1.TaintNodeUnschedulable {
				return true
			}
		}
	}

	return false
}

func (h *vmActionHandler) getNodeSelectorRequirementFromVMI(vmi *kubevirtv1.VirtualMachineInstance) (labels.Selector, error) {
	if vmi == nil || vmi.Spec.Affinity == nil || vmi.Spec.Affinity.NodeAffinity == nil || vmi.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return labels.Everything(), nil
	}

	nodeSelector := labels.NewSelector()
	terms := vmi.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	for _, term := range terms {
		for _, e := range term.MatchExpressions {
			r, err := convertNodeSelectorRequirementToSelector(e)
			if err != nil {
				return nil, err
			}
			nodeSelector = nodeSelector.Add(*r)
		}
	}

	return nodeSelector, nil
}

func (h *vmActionHandler) createVMBackup(vmName, vmNamespace string, input BackupInput) error {
	apiGroup := kubevirtv1.SchemeGroupVersion.Group
	backup := &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.Name,
			Namespace: vmNamespace,
		},
		Spec: harvesterv1.VirtualMachineBackupSpec{
			Source: corev1.TypedLocalObjectReference{
				APIGroup: &apiGroup,
				Kind:     kubevirtv1.VirtualMachineGroupVersionKind.Kind,
				Name:     vmName,
			},
			Type: harvesterv1.Backup,
		},
	}
	if _, err := h.backups.Create(backup); err != nil {
		return fmt.Errorf("failed to create VM backup, error: %s", err.Error())
	}
	return nil
}

func (h *vmActionHandler) restoreBackup(vmName, vmNamespace string, input RestoreInput) error {
	if _, err := h.backupCache.Get(vmNamespace, input.BackupName); err != nil {
		return err
	}
	apiGroup := kubevirtv1.SchemeGroupVersion.Group
	restore := &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.Name,
			Namespace: vmNamespace,
		},
		Spec: harvesterv1.VirtualMachineRestoreSpec{
			Target: corev1.TypedLocalObjectReference{
				APIGroup: &apiGroup,
				Kind:     kubevirtv1.VirtualMachineGroupVersionKind.Kind,
				Name:     vmName,
			},
			VirtualMachineBackupNamespace: vmNamespace,
			VirtualMachineBackupName:      input.BackupName,
			NewVM:                         false,
		},
	}
	_, err := h.restores.Create(restore)
	if err != nil {
		return fmt.Errorf("failed to create restore, error: %s", err.Error())
	}

	return nil
}

func (h *vmActionHandler) checkBackupTargetConfigured() error {
	targetSetting, err := h.settingCache.Get(settings.BackupTargetSettingName)
	if err == nil && harvesterv1.SettingConfigured.IsTrue(targetSetting) {
		// backup target may be reset to initial/default, the SettingConfigured.IsTrue meets
		target, err := settings.DecodeBackupTarget(targetSetting.Value)
		if err != nil {
			return err
		}
		if !target.IsDefaultBackupTarget() {
			return nil
		}
	}
	return fmt.Errorf("backup target is invalid")
}

func getMigrationUID(vmi *kubevirtv1.VirtualMachineInstance) string {
	if vmi.Annotations[util.AnnotationMigrationUID] != "" {
		return vmi.Annotations[util.AnnotationMigrationUID]
	} else if vmi.Status.MigrationState != nil {
		return string(vmi.Status.MigrationState.MigrationUID)
	}
	return ""
}

// createTemplate creates a template and version that are derived from the given virtual machine.
func (h *vmActionHandler) createTemplate(namespace, name string, input CreateTemplateInput) error {
	vm, err := h.vmCache.Get(namespace, name)
	if err != nil {
		return err
	}

	keyPairIDs, err := getSSHKeysFromVMITemplateSpec(vm.Spec.Template)
	if err != nil {
		return err
	}

	vmtvName := fmt.Sprintf("%s-%s", input.Name, rand.String(5))
	vmSourceSpec, err := h.sanitizeVirtualMachineForTemplateVersion(vmtvName, vm, input.WithData)
	if err != nil {
		return err
	}

	var pvcStorageClassMap map[string]string
	if input.WithData {
		pvcStorageClassMap, err = h.getPVCStorageClassMap(vm)
		if err != nil {
			return err
		}
	}

	vmt, err := h.vmTemplateClient.Create(
		&harvesterv1.VirtualMachineTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      input.Name,
				Namespace: namespace,
			},
			Spec: harvesterv1.VirtualMachineTemplateSpec{
				Description: input.Description,
			},
		})
	if err != nil {
		return err
	}

	vmID := fmt.Sprintf("%s/%s", vmt.Namespace, vmt.Name)

	vmtv, err := h.vmTemplateVersionClient.Create(
		&harvesterv1.VirtualMachineTemplateVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmtvName,
				Namespace: namespace,
			},
			Spec: harvesterv1.VirtualMachineTemplateVersionSpec{
				TemplateID:  vmID,
				Description: fmt.Sprintf("Template drived from virtual machine [%s]", vmID),
				VM:          vmSourceSpec,
				KeyPairIDs:  keyPairIDs,
			},
		})
	if err != nil {
		return err
	}

	if input.WithData {
		if err := h.createVMImages(vmtv, vm, pvcStorageClassMap); err != nil {
			return err
		}
	}

	return h.createSecrets(vmtv, vm)
}

func (h *vmActionHandler) createSecrets(templateVersion *harvesterv1.VirtualMachineTemplateVersion, vm *kubevirtv1.VirtualMachine) error {
	for index, credential := range vm.Spec.Template.Spec.AccessCredentials {
		if sshPublicKey := credential.SSHPublicKey; sshPublicKey != nil && sshPublicKey.Source.Secret != nil {
			toCreateSecretName := getTemplateVersionSSHPublicKeySecretName(templateVersion.Name, index)
			if err := h.copySecret(sshPublicKey.Source.Secret.SecretName, toCreateSecretName, templateVersion); err != nil {
				return err
			}
		}
		if userPassword := credential.UserPassword; userPassword != nil && userPassword.Source.Secret != nil {
			toCreateSecretName := getTemplateVersionUserPasswordSecretName(templateVersion.Name, index)
			if err := h.copySecret(userPassword.Source.Secret.SecretName, toCreateSecretName, templateVersion); err != nil {
				return err
			}
		}
	}
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.CloudInitNoCloud == nil {
			continue
		}
		if volume.CloudInitNoCloud.UserDataSecretRef != nil {
			toCreateSecretName := getTemplateVersionUserDataSecretName(templateVersion.Name, volume.Name)
			if err := h.copySecret(volume.CloudInitNoCloud.UserDataSecretRef.Name, toCreateSecretName, templateVersion); err != nil {
				return err
			}
		}
		if volume.CloudInitNoCloud.NetworkDataSecretRef != nil {
			toCreateSecretName := getTemplateVersionNetworkDataSecretName(templateVersion.Name, volume.Name)
			if err := h.copySecret(volume.CloudInitNoCloud.NetworkDataSecretRef.Name, toCreateSecretName, templateVersion); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *vmActionHandler) copySecret(sourceName, targetName string, templateVersion *harvesterv1.VirtualMachineTemplateVersion) error {
	secret, err := h.secretCache.Get(templateVersion.Namespace, sourceName)
	if err != nil {
		return err
	}
	toCreate := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetName,
			Namespace: secret.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: templateVersion.APIVersion,
					Kind:       templateVersion.Kind,
					Name:       templateVersion.Name,
					UID:        templateVersion.UID,
				},
			},
		},
		Data: secret.Data,
	}
	_, err = h.secretClient.Create(toCreate)
	return err

}

func (h *vmActionHandler) updateVMVolumeClaimTemplate(vm *kubevirtv1.VirtualMachine, updateVolumeClaimTemplate func([]corev1.PersistentVolumeClaim) ([]corev1.PersistentVolumeClaim, bool)) error {
	var volumeClaimTemplates []corev1.PersistentVolumeClaim
	vmCopy := vm.DeepCopy()
	anno := vmCopy.GetAnnotations()
	if volumeClaimTemplatesJSON, ok := anno[util.AnnotationVolumeClaimTemplates]; ok {
		if err := json.Unmarshal([]byte(volumeClaimTemplatesJSON), &volumeClaimTemplates); err != nil {
			return fmt.Errorf("failed to unserialize %s, error: %v", util.AnnotationVolumeClaimTemplates, err)
		}
	}

	var changed bool
	volumeClaimTemplates, changed = updateVolumeClaimTemplate(volumeClaimTemplates)
	if !changed {
		return nil
	}

	volumeClaimTemplatesJSON, err := json.Marshal(volumeClaimTemplates)
	if err != nil {
		return fmt.Errorf("failed to serialize payload %v, error: %v", volumeClaimTemplates, err)
	}
	anno[util.AnnotationVolumeClaimTemplates] = string(volumeClaimTemplatesJSON)
	vmCopy.SetAnnotations(anno)
	if !reflect.DeepEqual(vm, vmCopy) {
		if _, err = h.vms.Update(vmCopy); err != nil {
			return fmt.Errorf("failed to update vm %s/%s, error: %v", vm.Namespace, vm.Name, err)
		}
	}
	return nil
}

func (h *vmActionHandler) getPVCStorageClassMap(vm *kubevirtv1.VirtualMachine) (map[string]string, error) {
	pvcStorageClassMap := map[string]string{}
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}

		pvc, err := h.pvcCache.Get(vm.Namespace, volume.PersistentVolumeClaim.ClaimName)
		if err != nil {
			return pvcStorageClassMap, err
		}
		pvcStorageClass, err := h.storageClassCache.Get(*pvc.Spec.StorageClassName)
		if err != nil {
			return pvcStorageClassMap, err
		}
		var sc *storagev1.StorageClass
		if _, ok := pvcStorageClass.Parameters["backingImage"]; ok {
			// If "backingImage" is set, the storage class is from a VM image.
			// We can't use it directly. We need to find storageClassName annotation.
			if imageID, ok := pvc.Annotations[util.AnnotationImageID]; ok {
				imageIDSplit := strings.Split(imageID, "/")
				if len(imageIDSplit) == 2 {
					vmImage, err := h.vmImageCache.Get(imageIDSplit[0], imageIDSplit[1])
					if err != nil {
						return pvcStorageClassMap, err
					}
					if storageClassName, ok := vmImage.Annotations[util.AnnotationStorageClassName]; ok {
						sc, err = h.storageClassCache.Get(storageClassName)
						if err != nil {
							return pvcStorageClassMap, err
						}
					}
				}
			}
		} else {
			sc = pvcStorageClass
		}

		if sc == nil {
			pvcStorageClassMap[pvc.Name] = ""
			continue
		}
		pvcStorageClassMap[pvc.Name] = sc.Name
	}
	return pvcStorageClassMap, nil
}

func (h *vmActionHandler) createVMImages(templateVersion *harvesterv1.VirtualMachineTemplateVersion, vm *kubevirtv1.VirtualMachine, pvcStorageClassMap map[string]string) error {
	for index, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}
		targetSCName := pvcStorageClassMap[volume.PersistentVolumeClaim.ClaimName]
		// 3 cases here:
		// Root volume with backingImage, check SC name starts with "longhorn-templateversion-", use backingImage backend
		// volume with non longhorn provisioner, use CDI backend
		// volume with longhorn provisioner, use backingImage backend
		// we use CDI as default, so we need to check other two cases
		vmImageBackend := harvesterv1.VMIBackendCDI
		if strings.HasPrefix(targetSCName, "longhorn-templateversion-") {
			vmImageBackend = harvesterv1.VMIBackendBackingImage
		} else {
			// checking SC
			targetSC, err := h.storageClassCache.Get(targetSCName)
			if err != nil {
				return fmt.Errorf("failed to get storage class %s, error: %v", targetSCName, err)
			}
			if targetSC.Provisioner == util.CSIProvisionerLonghorn {
				if dataEngineVers, find := targetSC.Parameters["dataEngine"]; find {
					if dataEngineVers == string(longhorn.DataEngineTypeV1) {
						vmImageBackend = harvesterv1.VMIBackendBackingImage
					}
				} else {
					vmImageBackend = harvesterv1.VMIBackendBackingImage
				}
			}
		}
		if vmImageBackend == "" {
			return fmt.Errorf("failed to configure vm image backend for volume %s", volume.Name)
		}
		vmImageName := getTemplateVersionVMImageName(templateVersion.Name, index)
		vmImage := &harvesterv1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmImageName,
				Namespace: vm.Namespace,
				Annotations: map[string]string{
					util.AnnotationStorageClassName: targetSCName,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: templateVersion.APIVersion,
						Kind:       templateVersion.Kind,
						Name:       templateVersion.Name,
						UID:        templateVersion.UID,
					},
				},
			},
			Spec: harvesterv1.VirtualMachineImageSpec{
				Backend:                vmImageBackend,
				DisplayName:            vmImageName,
				SourceType:             harvesterv1.VirtualMachineImageSourceTypeExportVolume,
				PVCName:                volume.PersistentVolumeClaim.ClaimName,
				PVCNamespace:           vm.Namespace,
				TargetStorageClassName: targetSCName,
			},
		}

		if _, err := h.vmImages.Create(vmImage); err != nil {
			return err
		}
	}
	return nil
}

// Since the LH volume creation and replica scheduling are asynchronous
// Even the CreateVolume csi call success, the replica scheduling may still fail
// However for the sc with VolumeBindingMode is Immediate
// we could check annotation for corresponding error if it exists
func (h *vmActionHandler) checkAttachable(pvc *corev1.PersistentVolumeClaim) error {
	// Even Harvester specify default StorageClass,
	// we check the sc existence for robustness
	if pvc.Spec.StorageClassName == nil {
		return fmt.Errorf("volme %v with empty StorageClass is not permitted", pvc.Name)
	}

	sc, err := h.storageClassCache.Get(*pvc.Spec.StorageClassName)
	if err != nil {
		return err
	}

	if sc.VolumeBindingMode == nil {
		logrus.Infof("volme %v not specify VolumeBindingMode", pvc.Name)
		return nil
	}

	if *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
		return nil
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		return fmt.Errorf("volme %v not bound yet", pvc.Name)
	}

	pv, err := h.pvCache.Get(pvc.Spec.VolumeName)
	if err != nil {
		return err
	}

	//Even the volume is in degrade mode, it still could be attached
	scheduleErrAnno := pv.Annotations[longhorntypes.PVAnnotationLonghornVolumeSchedulingError]
	if scheduleErrAnno != "" && scheduleErrAnno != longhorn.ErrorReplicaScheduleSchedulingFailed {
		return fmt.Errorf("volme %v with error %v", pvc.Name, scheduleErrAnno)
	}

	return nil
}

// addVolume add a hotplug volume with given volume source and disk name.
func (h *vmActionHandler) addVolume(ctx context.Context, namespace, name string, input AddVolumeInput) error {
	// We only permit volume source from existing PersistentVolumeClaim at this moment.
	// KubeVirt won't check PVC existence so we validate it on our own.
	pvc, err := h.pvcCache.Get(namespace, input.VolumeSourceName)
	if err != nil {
		return err
	}

	if err := h.checkAttachable(pvc); err != nil {
		return err
	}

	// Restrict the flexibility of disk options here but future extension may be possible.
	body, err := json.Marshal(kubevirtv1.AddVolumeOptions{
		Name: input.DiskName,
		Disk: &kubevirtv1.Disk{
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{
					// KubeVirt only support SCSI for hotplug volume.
					Bus: "scsi",
				},
			},
		},
		VolumeSource: &kubevirtv1.HotplugVolumeSource{
			PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: input.VolumeSourceName,
				},
				Hotpluggable: true,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to serialize payload,: %v", err)
	}

	// Ref: https://kubevirt.io/api-reference/v0.44.0/operations.html#_v1vm-addvolume
	if err = h.virtSubresourceRestClient.
		Put().
		Namespace(namespace).
		Resource(vmResource).
		Name(name).
		SubResource(strings.ToLower(addVolume)).
		Body(body).
		Do(ctx).
		Error(); err != nil {
		return err
	}

	addVolumeClaimTemplate := func(volumeClaimTemplates []corev1.PersistentVolumeClaim) ([]corev1.PersistentVolumeClaim, bool) {
		for _, volumeClaimTemplate := range volumeClaimTemplates {
			if volumeClaimTemplate.Name == input.VolumeSourceName {
				return volumeClaimTemplates, false
			}
		}
		volumeClaimTemplates = append(volumeClaimTemplates, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: pvc.Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      pvc.Spec.AccessModes,
				Resources:        pvc.Spec.Resources,
				VolumeMode:       pvc.Spec.VolumeMode,
				StorageClassName: pvc.Spec.StorageClassName,
			},
		})
		return volumeClaimTemplates, true
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// We just updated the VM in the last step, so we get the VM from api-server directly, not local cache.
		vm, err := h.vms.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get vm %s/%s, error: %v", namespace, name, err)
		}

		return h.updateVMVolumeClaimTemplate(vm, addVolumeClaimTemplate)
	})
}

// removeVolume remove a hotplug volume by its disk name
func (h *vmActionHandler) removeVolume(ctx context.Context, namespace, name string, input RemoveVolumeInput) error {
	vm, err := h.vmCache.Get(namespace, name)
	if err != nil {
		return err
	}

	// Ensure the existence of the disk. KubeVirt will take care of other cases
	// such as trying to remove a non-hotplug volume.
	var pvcName string
	found := false
	for _, vol := range vm.Spec.Template.Spec.Volumes {
		if vol.Name == input.DiskName {
			found = true
			if vol.PersistentVolumeClaim != nil {
				pvcName = vol.PersistentVolumeClaim.ClaimName
			}
		}
	}
	if !found {
		return fmt.Errorf("disk `%s` not found in virtual machine `%s/%s`", input.DiskName, namespace, name)
	}

	body, err := json.Marshal(kubevirtv1.RemoveVolumeOptions{
		Name: input.DiskName,
	})

	if err != nil {
		return fmt.Errorf("failed to serialize payload,: %v", err)
	}
	// Ref: https://kubevirt.io/api-reference/v0.44.0/operations.html#_v1vm-removevolume
	if err = h.virtSubresourceRestClient.
		Put().
		Namespace(namespace).
		Resource(vmResource).
		Name(name).
		SubResource(strings.ToLower(removeVolume)).
		Body(body).
		Do(ctx).
		Error(); err != nil {
		return err
	}

	if pvcName == "" {
		return nil
	}

	removeVolumeClaimTemplate := func(volumeClaimTemplates []corev1.PersistentVolumeClaim) ([]corev1.PersistentVolumeClaim, bool) {
		for i, volumeClaimTemplate := range volumeClaimTemplates {
			if volumeClaimTemplate.Name == pvcName {
				volumeClaimTemplates[i], volumeClaimTemplates[len(volumeClaimTemplates)-1] = volumeClaimTemplates[len(volumeClaimTemplates)-1], volumeClaimTemplates[i]
				volumeClaimTemplates = volumeClaimTemplates[:len(volumeClaimTemplates)-1]
				return volumeClaimTemplates, true
			}
		}
		return volumeClaimTemplates, false
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// We just updated the VM in the last step, so we get the VM from api-server directly, not local cache.
		vm, err = h.vms.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		return h.updateVMVolumeClaimTemplate(vm, removeVolumeClaimTemplate)
	})
}

// cloneVM creates a VM which uses volume cloning from the source VM.
func (h *vmActionHandler) cloneVM(name string, namespace string, input CloneInput) error {
	vm, err := h.vmCache.Get(namespace, name)
	if err != nil {
		return fmt.Errorf("cannot get vm %s/%s, err: %w", namespace, name, err)
	}
	newVM := getClonedVMYamlFromSourceVM(input, vm)

	newPVCs, secretNameMap, err := h.cloneVolumes(newVM)
	if err != nil {
		return fmt.Errorf("clone volumes error for new vm %s/%s, err %w", newVM.Namespace, newVM.Name, err)
	}
	newPVCsString, err := json.Marshal(newPVCs)
	if err != nil {
		return fmt.Errorf("cannot marshal value %+v, err: %w", newPVCs, err)
	}

	newVM.ObjectMeta.Annotations[util.AnnotationVolumeClaimTemplates] = string(newPVCsString)
	if newVM, err = h.vms.Create(newVM); err != nil {
		return fmt.Errorf("cannot create new VM %s/%s, err: %w", newVM.Namespace, newVM.Name, err)
	}

	for oldSecretName, newSecretName := range secretNameMap {
		secret, err := h.secretCache.Get(namespace, oldSecretName)
		if err != nil {
			return fmt.Errorf("cannot get secret %s/%s, err: %w", namespace, oldSecretName, err)
		}

		newSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      newSecretName,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: newVM.APIVersion,
						Kind:       newVM.Kind,
						Name:       newVM.Name,
						UID:        newVM.UID,
					},
				},
			},
			Data:       secret.Data,
			StringData: secret.StringData,
			Type:       secret.Type,
		}
		if _, err = h.secretClient.Create(&newSecret); err != nil {
			return fmt.Errorf("cannot create a new secret from %s/%s, err: %w", namespace, oldSecretName, err)
		}
	}
	return nil
}

func (h *vmActionHandler) cloneVolumes(newVM *kubevirtv1.VirtualMachine) ([]corev1.PersistentVolumeClaim, map[string]string, error) {
	var (
		err           error
		newPVCs       []corev1.PersistentVolumeClaim
		secretNameMap = map[string]string{} // sourceVM secret name to newVM secret name
	)

	for i, volume := range newVM.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			var pvc *corev1.PersistentVolumeClaim
			pvc, err = h.pvcCache.Get(newVM.Namespace, volume.PersistentVolumeClaim.ClaimName)
			if err != nil {
				return nil, nil, fmt.Errorf("cannot get pvc %s, err: %w", volume.PersistentVolumeClaim.ClaimName, err)
			}

			annotations := map[string]string{}
			if imageID, ok := pvc.Annotations[util.AnnotationImageID]; ok {
				annotations[util.AnnotationImageID] = imageID
			}
			newPVC := corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   newVM.Namespace,
					Name:        names.SimpleNameGenerator.GenerateName(fmt.Sprintf("%s-%s-", newVM.Name, volume.Name)),
					Annotations: annotations,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: pvc.Spec.AccessModes,
					DataSource: &corev1.TypedLocalObjectReference{
						Kind: "PersistentVolumeClaim",
						Name: pvc.Name,
					},
					Resources:        pvc.Spec.Resources,
					StorageClassName: pvc.Spec.StorageClassName,
					VolumeMode:       pvc.Spec.VolumeMode,
				},
			}
			newPVCs = append(newPVCs, newPVC)
			volume.PersistentVolumeClaim.ClaimName = newPVC.Name
		} else if volume.CloudInitNoCloud != nil {
			if volume.CloudInitNoCloud.UserDataSecretRef != nil {
				if _, ok := secretNameMap[volume.CloudInitNoCloud.UserDataSecretRef.Name]; !ok {
					secretNameMap[volume.CloudInitNoCloud.UserDataSecretRef.Name] = names.SimpleNameGenerator.GenerateName(fmt.Sprintf("%s-", newVM.Name))
				}
				volume.CloudInitNoCloud.UserDataSecretRef.Name = secretNameMap[volume.CloudInitNoCloud.UserDataSecretRef.Name]
			}
			if volume.CloudInitNoCloud.NetworkDataSecretRef != nil {
				if _, ok := secretNameMap[volume.CloudInitNoCloud.NetworkDataSecretRef.Name]; !ok {
					secretNameMap[volume.CloudInitNoCloud.NetworkDataSecretRef.Name] = names.SimpleNameGenerator.GenerateName(fmt.Sprintf("%s-", newVM.Name))
				}
				volume.CloudInitNoCloud.NetworkDataSecretRef.Name = secretNameMap[volume.CloudInitNoCloud.NetworkDataSecretRef.Name]
			}
		} else if volume.ContainerDisk != nil {
			continue
		} else {
			return nil, nil, fmt.Errorf("invalid volume %s, only support PersistentVolumeClaim, CloudInitNoCloud, and ContainerDisk", volume.Name)
		}
		newVM.Spec.Template.Spec.Volumes[i] = volume
	}
	return newPVCs, secretNameMap, nil
}

func (h *vmActionHandler) sanitizeVirtualMachineForTemplateVersion(templateVersionName string, vm *kubevirtv1.VirtualMachine, withData bool) (harvesterv1.VirtualMachineSourceSpec, error) {
	var err error
	sanitizedVM := removeMacAddresses(vm)
	sanitizedVM = replaceSecrets(templateVersionName, sanitizedVM)
	if withData {
		sanitizedVM, err = h.replaceVolumes(templateVersionName, sanitizedVM)
		if err != nil {
			return harvesterv1.VirtualMachineSourceSpec{}, err
		}
	}

	return harvesterv1.VirtualMachineSourceSpec{
		ObjectMeta: sanitizedVM.ObjectMeta,
		Spec:       sanitizedVM.Spec,
	}, nil
}

func replaceSecrets(templateVersionName string, vm *kubevirtv1.VirtualMachine) *kubevirtv1.VirtualMachine {
	sanitizedVM := vm.DeepCopy()
	for index, credential := range sanitizedVM.Spec.Template.Spec.AccessCredentials {
		if sshPublicKey := credential.SSHPublicKey; sshPublicKey != nil && sshPublicKey.Source.Secret != nil {
			sanitizedVM.Spec.Template.Spec.AccessCredentials[index].SSHPublicKey.Source.Secret.SecretName = getTemplateVersionSSHPublicKeySecretName(templateVersionName, index)
		}
		if userPassword := credential.UserPassword; userPassword != nil && userPassword.Source.Secret != nil {
			sanitizedVM.Spec.Template.Spec.AccessCredentials[index].UserPassword.Source.Secret.SecretName = getTemplateVersionUserPasswordSecretName(templateVersionName, index)
		}
	}
	for index, volume := range sanitizedVM.Spec.Template.Spec.Volumes {
		if volume.CloudInitNoCloud == nil {
			continue
		}
		if volume.CloudInitNoCloud.UserDataSecretRef != nil {
			sanitizedVM.Spec.Template.Spec.Volumes[index].CloudInitNoCloud.UserDataSecretRef.Name = getTemplateVersionUserDataSecretName(templateVersionName, volume.Name)
		}
		if volume.CloudInitNoCloud.NetworkDataSecretRef != nil {
			sanitizedVM.Spec.Template.Spec.Volumes[index].CloudInitNoCloud.NetworkDataSecretRef.Name = getTemplateVersionNetworkDataSecretName(templateVersionName, volume.Name)
		}
	}
	return sanitizedVM
}

func (h *vmActionHandler) replaceVolumes(templateVersionName string, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
	sanitizedVM := vm.DeepCopy()
	volumeClaimTemplates := []corev1.PersistentVolumeClaim{}
	for index, volume := range sanitizedVM.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}

		pvc, err := h.pvcCache.Get(vm.Namespace, volume.PersistentVolumeClaim.ClaimName)
		if err != nil {
			return nil, err
		}

		// generate new volume template
		// - longhorn v1, we need to use the new storageclass Name (for backingImage)
		// - others, use the pvc StorageClassName
		vmImageName := getTemplateVersionVMImageName(templateVersionName, index)
		targetStorageClassName := fmt.Sprintf("longhorn-%s", vmImageName)
		if _, find := pvc.Annotations[cdicommon.AnnCreatedForDataVolume]; find {
			targetStorageClassName = *pvc.Spec.StorageClassName
		}
		annoImageID := fmt.Sprintf("%s/%s", vm.Namespace, vmImageName)
		pvcName := getTemplateVersionPvcName(templateVersionName, index)
		volumeClaimTemplates = append(volumeClaimTemplates, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: pvcName,
				Annotations: map[string]string{
					util.AnnotationImageID: annoImageID,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      pvc.Spec.AccessModes,
				Resources:        pvc.Spec.Resources,
				VolumeMode:       pvc.Spec.VolumeMode,
				StorageClassName: &targetStorageClassName,
			},
		})
		sanitizedVM.Spec.Template.Spec.Volumes[index].PersistentVolumeClaim.ClaimName = pvcName
	}

	volumeCliamTemplatesJSON, err := json.Marshal(volumeClaimTemplates)
	if err != nil {
		return nil, err
	}

	if sanitizedVM.Annotations == nil {
		sanitizedVM.Annotations = map[string]string{}
	}
	sanitizedVM.Annotations[util.AnnotationVolumeClaimTemplates] = string(volumeCliamTemplatesJSON)
	return sanitizedVM, nil
}

func (h *vmActionHandler) dismissInsufficientResourceQuota(name, namespace string) error {
	vm, err := h.vmCache.Get(namespace, name)
	if err != nil {
		return fmt.Errorf("cannot get vm %s/%s, err: %w", namespace, name, err)
	}

	if !canDismissInsufficientResourceQuota(vm) {
		return errors.New("cannot dismiss insufficient resource quota message")
	}

	vmCpy := vm.DeepCopy()
	delete(vmCpy.Annotations, util.AnnotationInsufficientResourceQuota)
	if _, err = h.vms.Update(vmCpy); err != nil {
		return fmt.Errorf("failed to update vm %s/%s, error: %v", vm.Namespace, vm.Name, err)
	}
	return nil
}

// removeMacAddresses replaces the mac address of each device interface with an empty string.
// This is because macAddresses are unique, and should not reuse the original's.
func removeMacAddresses(vm *kubevirtv1.VirtualMachine) *kubevirtv1.VirtualMachine {
	sanitizedVM := vm.DeepCopy()
	for index := range sanitizedVM.Spec.Template.Spec.Domain.Devices.Interfaces {
		sanitizedVM.Spec.Template.Spec.Domain.Devices.Interfaces[index].MacAddress = ""
	}
	return sanitizedVM
}

// getSSHKeysFromVMITemplateSpec first checks the given VirtualMachineInstanceTemplateSpec
// for ssh key annotation. If found, it attempts to parse it into a string slice and return
// it.
func getSSHKeysFromVMITemplateSpec(vmitSpec *kubevirtv1.VirtualMachineInstanceTemplateSpec) ([]string, error) {
	if vmitSpec == nil {
		return nil, nil
	}
	annos := vmitSpec.ObjectMeta.Annotations
	if annos == nil {
		return nil, nil
	}
	var sshKeys []string
	if err := json.Unmarshal([]byte(annos[sshAnnotation]), &sshKeys); err != nil {
		return nil, err
	}
	return sshKeys, nil
}

func getTemplateVersionUserDataSecretName(templateVersionName, volumeName string) string {
	return wranglername.SafeConcatName("templateversion", templateVersionName, volumeName, "userdata")
}

func getTemplateVersionNetworkDataSecretName(templateVersionName, volumeName string) string {
	return wranglername.SafeConcatName("templateversion", templateVersionName, volumeName, "networkdata")
}

func getTemplateVersionPvcName(templateVersionName string, diskIndex int) string {
	return wranglername.SafeConcatName("templateversion", templateVersionName, fmt.Sprintf("disk-%d", diskIndex))
}

func getTemplateVersionVMImageName(templateVersionName string, imageIndex int) string {
	return wranglername.SafeConcatName("templateversion", templateVersionName, fmt.Sprintf("image-%d", imageIndex))
}

func getTemplateVersionSSHPublicKeySecretName(templateVersionName string, credentialIndex int) string {
	return wranglername.SafeConcatName("templateversion", templateVersionName, fmt.Sprintf("credential-%d", credentialIndex), "sshpublickey")
}

func getTemplateVersionUserPasswordSecretName(templateVersionName string, credentialIndex int) string {
	return wranglername.SafeConcatName("templateversion", templateVersionName, fmt.Sprintf("credential-%d", credentialIndex), "userpassword")
}

func getClonedVMYamlFromSourceVM(input CloneInput, sourceVM *kubevirtv1.VirtualMachine) *kubevirtv1.VirtualMachine {
	newVM := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        input.TargetVM,
			Namespace:   sourceVM.Namespace,
			Annotations: map[string]string{},
			Labels:      sourceVM.Labels,
		},
		Spec: *sourceVM.Spec.DeepCopy(),
	}
	for _, cloneVMAnnoKey := range cloneVMAnnotationKeys {
		if sourceAnnoValue, keyExist := sourceVM.Annotations[cloneVMAnnoKey]; keyExist {
			newVM.Annotations[cloneVMAnnoKey] = sourceAnnoValue
		}
	}

	runStrategy := kubevirtv1.VirtualMachineRunStrategy(input.RunStrategy)
	if runStrategy != kubevirtv1.RunStrategyUnknown {
		newVM.Spec.RunStrategy = &runStrategy
	}
	newVM.Spec.Template.Spec.Hostname = newVM.Name
	newVM.Spec.Template.ObjectMeta.Labels[builder.LabelKeyVirtualMachineName] = newVM.Name
	for i := range newVM.Spec.Template.Spec.Domain.Devices.Interfaces {
		newVM.Spec.Template.Spec.Domain.Devices.Interfaces[i].MacAddress = ""
	}
	return newVM
}

func convertNodeSelectorRequirementToSelector(req corev1.NodeSelectorRequirement) (*labels.Requirement, error) {
	switch req.Operator {
	case corev1.NodeSelectorOpIn:
		return labels.NewRequirement(req.Key, selection.In, req.Values)
	case corev1.NodeSelectorOpNotIn:
		return labels.NewRequirement(req.Key, selection.NotIn, req.Values)
	case corev1.NodeSelectorOpExists:
		return labels.NewRequirement(req.Key, selection.Exists, nil)
	case corev1.NodeSelectorOpDoesNotExist:
		return labels.NewRequirement(req.Key, selection.DoesNotExist, nil)
	case corev1.NodeSelectorOpGt:
		return labels.NewRequirement(req.Key, selection.GreaterThan, req.Values)
	case corev1.NodeSelectorOpLt:
		return labels.NewRequirement(req.Key, selection.LessThan, req.Values)
	default:
		return &labels.Requirement{}, fmt.Errorf("unsupported operator: %v", req.Operator)
	}
}

func (h *vmActionHandler) updateResourceQuota(namespace, name string, input UpdateResourceQuotaInput) error {
	totalSnapshotSizeQuotaQuantity, err := resource.ParseQuantity(input.TotalSnapshotSizeQuota)
	if err != nil {
		return apierror.NewAPIError(validation.InvalidBodyContent, fmt.Sprintf("Failed to parse totalSnapshotSizeQuota: %v", err))
	}
	totalSnapshotSizeQuotaInt64, ok := totalSnapshotSizeQuotaQuantity.AsInt64()
	if !ok {
		return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to parse totalSnapshotSizeQuota as int64")
	}

	resourceQuota, err := h.resourceQuotaClient.Get(namespace, util.DefaultResourceQuotaName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			if totalSnapshotSizeQuotaInt64 == 0 {
				return nil
			}
			resourceQuota = &harvesterv1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      util.DefaultResourceQuotaName,
				},
				Spec: harvesterv1.ResourceQuotaSpec{
					SnapshotLimit: harvesterv1.SnapshotLimit{
						VMTotalSnapshotSizeQuota: map[string]int64{
							name: totalSnapshotSizeQuotaInt64,
						},
					},
				},
			}
			_, err = h.resourceQuotaClient.Create(resourceQuota)
			return err
		}
		return err
	}

	resourceQuotaCopy := resourceQuota.DeepCopy()
	if resourceQuotaCopy.Spec.SnapshotLimit.VMTotalSnapshotSizeQuota == nil {
		resourceQuotaCopy.Spec.SnapshotLimit.VMTotalSnapshotSizeQuota = map[string]int64{}
	}
	resourceQuotaCopy.Spec.SnapshotLimit.VMTotalSnapshotSizeQuota[name] = totalSnapshotSizeQuotaInt64
	if !reflect.DeepEqual(resourceQuota, resourceQuotaCopy) {
		_, err = h.resourceQuotaClient.Update(resourceQuotaCopy)
		return err
	}
	return nil
}

func (h *vmActionHandler) deleteResourceQuota(namespace, name string) error {
	resourceQuota, err := h.resourceQuotaClient.Get(namespace, util.DefaultResourceQuotaName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	resourceQuotaCopy := resourceQuota.DeepCopy()
	if resourceQuotaCopy.Spec.SnapshotLimit.VMTotalSnapshotSizeQuota == nil {
		return nil
	}
	delete(resourceQuotaCopy.Spec.SnapshotLimit.VMTotalSnapshotSizeQuota, name)
	if !reflect.DeepEqual(resourceQuota, resourceQuotaCopy) {
		_, err = h.resourceQuotaClient.Update(resourceQuotaCopy)
		return err
	}
	return nil
}

func (h *vmActionHandler) cpuAndMemoryHotplug(namespace, name string, input CPUAndMemoryHotplugInput) error {
	if input.Sockets == 0 && input.Memory == "" {
		return nil
	}

	patchOps := []string{}
	if input.Sockets != 0 {
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/cpu/sockets", "value": %d}`, input.Sockets))
	}

	if input.Memory != "" {
		memory, err := resource.ParseQuantity(input.Memory)
		if err != nil {
			return fmt.Errorf("failed to parse memory quantity: %v", err)
		}

		patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "/spec/template/spec/domain/memory/guest", "value": "%s"}`, memory.String()))
	}

	if len(patchOps) == 0 {
		return nil
	}

	logrus.WithFields(logrus.Fields{
		"namespace": namespace,
		"name":      name,
		"patchOps":  patchOps,
	}).Info("patch cpu and memory hotplug")
	patchData := fmt.Sprintf("[%s]", strings.Join(patchOps, ","))
	_, err := h.vms.Patch(namespace, name, k8stypes.JSONPatchType, []byte(patchData))
	return err
}
