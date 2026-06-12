package virtualmachineinstance

import (
	"encoding/base32"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/kubevirt/pkg/apimachinery/patch"
	"kubevirt.io/kubevirt/pkg/network/namescheme"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewMutator(
	vm ctlkubevirtv1.VirtualMachineCache,
	nadCache ctlcniv1.NetworkAttachmentDefinitionCache,
) types.Mutator {
	return &vmiMutator{
		vm:       vm,
		nadCache: nadCache,
	}
}

type vmiMutator struct {
	types.DefaultMutator
	vm       ctlkubevirtv1.VirtualMachineCache
	nadCache ctlcniv1.NetworkAttachmentDefinitionCache
}

func (m *vmiMutator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"virtualmachineinstances"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   kubevirtv1.SchemeGroupVersion.Group,
		APIVersion: kubevirtv1.SchemeGroupVersion.Version,
		ObjectType: &kubevirtv1.VirtualMachineInstance{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}

func (m *vmiMutator) Create(_ *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	vmi := newObj.(*kubevirtv1.VirtualMachineInstance)

	logrus.Debugf("create VMI %s/%s", vmi.Namespace, vmi.Name)

	vm, err := m.vm.Get(vmi.Namespace, vmi.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	patchOps, err := m.patchMacAddress(vm, vmi)
	if err != nil {
		return nil, fmt.Errorf("error patching mac address for vmi %s/%s: %w", vmi.Namespace, vmi.Name, err)
	}

	if len(vmi.Spec.Domain.Devices.HostDevices) > 0 || len(vmi.Spec.Domain.Devices.GPUs) > 0 {
		devicesPatch, err := patchDeviceName(vmi)
		if err != nil {
			return nil, fmt.Errorf("error patching device name for vmi %s/%s: %w", vmi.Namespace, vmi.Name, err)
		}
		patchOps = append(patchOps, devicesPatch...)
	}

	// generate static ip annotation patch for kubeovn if applicable
	kubeOVNPatch, err := generateKubeOVNStaticIPPatch(vm, vmi, m.nadCache)
	if err != nil {
		return nil, fmt.Errorf("error generating kubeovn static ip patch for vmi %s/%s: %w", vmi.Namespace, vmi.Name, err)
	}
	patchOps = append(patchOps, kubeOVNPatch...)
	return patchOps, nil
}

func (m *vmiMutator) patchMacAddress(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) (types.PatchOps, error) {
	if vm.Annotations == nil || vm.Annotations[util.AnnotationMacAddressName] == "" {
		return nil, nil
	}

	vmiInterfaces := map[string]string{}
	if err := json.Unmarshal([]byte(vm.Annotations[util.AnnotationMacAddressName]), &vmiInterfaces); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"name":      vm.Name,
			"namespace": vm.Namespace,
			"macs":      vm.Annotations[util.AnnotationMacAddressName],
		}).Error("failed to unmarshal mac-address from vm annotation")
		return nil, nil
	}

	patchOps := types.PatchOps{}
	for i, iface := range vmi.Spec.Domain.Devices.Interfaces {
		if vm.Spec.Template.Spec.Domain.Devices.Interfaces[i].MacAddress != "" {
			continue
		}
		ifaceMac, ok := vmiInterfaces[iface.Name]
		if !ok || ifaceMac == "" {
			continue
		}
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "add", "path": "/spec/domain/devices/interfaces/%d/macAddress", "value": "%s"}`, i, ifaceMac))
	}
	return patchOps, nil
}

func generateEncodedAlias(aliasName string) string {
	matched := regexp.MustCompile("^[a-zA-Z0-9_-]+$").MatchString(aliasName)
	if matched {
		return aliasName
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(aliasName))
}

func patchDeviceName(vmi *kubevirtv1.VirtualMachineInstance) (types.PatchOps, error) {
	patchOps := types.PatchOps{}

	hostDevicePath := "/spec/domain/devices/hostDevices/%d/name"
	gpuDevicePath := "/spec/domain/devices/gpus/%d/name"
	for i, hostDevice := range vmi.Spec.Domain.Devices.HostDevices {
		encodedAlias := generateEncodedAlias(hostDevice.Name)
		if encodedAlias != hostDevice.Name {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "%s", "value": "%s"}`, fmt.Sprintf(hostDevicePath, i), encodedAlias))
		}
	}
	for i, gpu := range vmi.Spec.Domain.Devices.GPUs {
		encodedAlias := generateEncodedAlias(gpu.Name)
		if encodedAlias != gpu.Name {
			patchOps = append(patchOps, fmt.Sprintf(`{"op": "replace", "path": "%s", "value": "%s"}`, fmt.Sprintf(gpuDevicePath, i), encodedAlias))
		}
	}
	return patchOps, nil
}

func generateKubeOVNStaticIPPatch(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance, nadCache ctlcniv1.NetworkAttachmentDefinitionCache) (types.PatchOps, error) {
	patchOps := types.PatchOps{}
	extraAnnotations, err := generateKubeOVNAnnotations(vm, vmi, nadCache)
	if err != nil {
		return nil, err
	}
	// if no field is present for annotations in metadata and we have annotations to add, we need to add the annotations field before adding any annotations
	if vmi.ObjectMeta.Annotations == nil && len(extraAnnotations) > 0 {
		patchOps = append(patchOps, `{"op": "add", "path": "/metadata/annotations", "value": {}}`)
	}

	for key, val := range extraAnnotations {
		patchOps = append(patchOps, fmt.Sprintf(`{"op": "add", "path": "/metadata/annotations/%s", "value": "%s"}`, patch.EscapeJSONPointer(key), val))
	}
	return patchOps, nil
}

func generateKubeOVNAnnotations(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance, nadCache ctlcniv1.NetworkAttachmentDefinitionCache) (map[string]string, error) {
	extraAnnotations := make(map[string]string)
	// generate a map of annotation keys for each network in vmi.Spec.Networks for kubeovn
	// for kubeovn to allocation static ip, each vmi object will need annotations of the form
	// vm-network.default.ovn.net1.kubernetes.io/logical_switch: vm-network
	// vm-network.default.kubernetes.io/ip_address.net1: 192.168.0.12
	// this needs to be repeated for each network interface that may be needing a static address

	// kubeovnNetworkMap will contain the prefix value for both switch and static ip annotation
	// eg
	// networks:
	// - multus:
	//     networkName: default/vm-network
	//   name: default
	// gets mapped to
	// default: vm-network.default in the kubeovnNetworkMap and used to generate switch and static ip annotations
	kubeovnNetworkMap := make(map[string]string)
	for _, network := range vmi.Spec.Networks {
		if network.Multus == nil {
			continue

		}
		elements := strings.Split(network.Multus.NetworkName, "/")
		var networkName, networkNamespace string
		if len(elements) == 1 {
			// no namespace provided for multus net-attach-def, so we use the namespace of vmi object
			networkName = elements[0]
			networkNamespace = vmi.Namespace
		} else if len(elements) == 2 {
			networkNamespace = elements[0]
			networkName = elements[1]
		} else {
			return nil, fmt.Errorf("invalid network name %s for network interface %s in vmi %s/%s", network.Multus.NetworkName, network.Name, vmi.Namespace, vmi.Name)
		}
		// verify if vm network is overlay, if not skip it as we will not be generating annotations for non-overlay network
		nad, err := nadCache.Get(networkNamespace, networkName)
		if err != nil {
			return nil, fmt.Errorf("error getting nad %s/%s for vmi %s/%s: %w", networkNamespace, networkName, vmi.Namespace, vmi.Name, err)
		}
		if !util.IsNetworkTypeOverlay(nad) {
			// net-attach-def is not of type overlay, skipping as this cannot be used for annotation generation
			continue
		}

		kubeovnNetworkMap[network.Name] = fmt.Sprintf("%s.%s", networkName, networkNamespace)
	}

	hashedNetworkNameMap := namescheme.CreateHashedNetworkNameScheme(vmi.Spec.Networks)
	// for each interface in the vmi, check if a static ip allocation annotation exists
	// on vm object and use the same to generate kubeovn annotations for each interface
	for _, interfaceObj := range vmi.Spec.Domain.Devices.Interfaces {
		// check if interface is even using overlay network as kubeovnNetworkMap only contains networks of type overlay, if interface is not using overlay network, skip it as we will not be generating annotations for non-overlay network
		if _, ok := kubeovnNetworkMap[interfaceObj.Name]; !ok {
			continue
		}
		// check static ip annotations, else skip and move to next interface as static ip annotation is required for generating kubeovn annotations
		staticIPAnnotationKey := fmt.Sprintf("%s/%s", util.StaticIPAnnotationKeyPrefix, interfaceObj.Name)
		val, ok := vm.Annotations[staticIPAnnotationKey]
		// no annotation found, nothing needs to be done for this interface
		if !ok {
			continue
		}
		// annotation found, generate ip annotation string
		podIfName := hashedNetworkNameMap[interfaceObj.Name]
		kubeovnPrefix := kubeovnNetworkMap[interfaceObj.Name]
		ipAnnotationKey := fmt.Sprintf("%s.kubernetes.io/ip_address.%s", kubeovnPrefix, podIfName)
		switchName := strings.Split(kubeovnPrefix, ".")[0]
		switchAnnotationKey := fmt.Sprintf("%s.ovn.%s.kubernetes.io/logical_switch", kubeovnPrefix, podIfName)
		extraAnnotations[ipAnnotationKey] = val
		extraAnnotations[switchAnnotationKey] = switchName

	}
	return extraAnnotations, nil
}
