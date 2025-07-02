package virtualmachineinstance

import (
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	schedulingcorev1 "k8s.io/component-helpers/scheduling/corev1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

func GetAllNonLiveMigratableVMINames(vmis []*kubevirtv1.VirtualMachineInstance, nodes []*corev1.Node) ([]string, error) {
	var nonLiveMigratableVMINames []string

	nonWitnessNodes := util.ExcludeWitnessNodes(nodes)

	// If there is only one node, all VMs are non-migratable
	if len(nonWitnessNodes) == 1 {
		for _, vmi := range vmis {
			vmiNamespacedName := fmt.Sprintf("%s/%s", vmi.Namespace, vmi.Name)
			nonLiveMigratableVMINames = append(nonLiveMigratableVMINames, vmiNamespacedName)
		}
		return nonLiveMigratableVMINames, nil
	}

	for _, vmi := range vmis {
		vmiNamespacedName := fmt.Sprintf("%s/%s", vmi.Namespace, vmi.Name)

		// Node selectors
		if vmi.Spec.NodeSelector != nil {
			logrus.Infof("%s considered non-live migratable due to node selectors", vmiNamespacedName)
			nonLiveMigratableVMINames = append(nonLiveMigratableVMINames, vmiNamespacedName)
			continue
		}

		// PCIe devices
		if vmi.Spec.Domain.Devices.HostDevices != nil {
			logrus.Infof("%s considered non-live migratable due to pcie or usb devices", vmiNamespacedName)
			nonLiveMigratableVMINames = append(nonLiveMigratableVMINames, vmiNamespacedName)
			continue
		}

		// Node affinities
		migratable, err := migratableByNodeAffinity(vmi, nonWitnessNodes)
		if err != nil {
			return nonLiveMigratableVMINames, err
		}
		if !migratable {
			logrus.Infof("%s considered non-live migratable due to node affinities", vmiNamespacedName)
			nonLiveMigratableVMINames = append(nonLiveMigratableVMINames, vmiNamespacedName)
			continue
		}

		// container-disk or cdrom device
		if VMContainsCDRomOrContainerDisk(vmi) {
			nonLiveMigratableVMINames = append(nonLiveMigratableVMINames, vmiNamespacedName)
			logrus.Infof("%s considered non-live migratable due to CD-ROM or container disk", vmiNamespacedName)
			continue
		}
	}

	return nonLiveMigratableVMINames, nil
}

func ValidateVMMigratable(vmi *kubevirtv1.VirtualMachineInstance) error {
	vmiNamespacedName := fmt.Sprintf("%s/%s", vmi.Namespace, vmi.Name)

	if !vmi.IsRunning() {
		return fmt.Errorf("VM %s is not live migratable as it is not running", vmiNamespacedName)
	}

	// The VM is already in migrating state
	if vmi.Annotations[util.AnnotationMigrationUID] != "" {
		return fmt.Errorf("VM %s is not live migratable as it is already in a migrating state", vmiNamespacedName)
	}

	// Node selectors
	if vmi.Spec.NodeSelector != nil && vmi.Spec.NodeSelector[corev1.LabelHostname] != "" {
		return fmt.Errorf("VM %s is not live migratable as node selector is set", vmiNamespacedName)
	}

	// PCIe devices
	if len(vmi.Spec.Domain.Devices.HostDevices) != 0 {
		return fmt.Errorf("VM %s is not live migratable as PCIe or USB devices are attached", vmiNamespacedName)
	}

	// vGPU devices
	if len(vmi.Spec.Domain.Devices.GPUs) != 0 {
		return fmt.Errorf("VM %s is not live migratable as vGPU devices are attached", vmiNamespacedName)
	}

	// container-disk or cdrom device
	if VMContainsCDRomOrContainerDisk(vmi) {
		return fmt.Errorf("VM %s is not live migratable as CD-ROM or container disk is set", vmiNamespacedName)
	}

	return nil
}

func migratableByNodeAffinity(vmi *kubevirtv1.VirtualMachineInstance, nodes []*corev1.Node) (bool, error) {
	migratabilityMap := make(map[string]bool, len(nodes)-1)
	for _, node := range nodes {
		// Skip the node the VM currently run on
		if vmi.Status.NodeName == node.Name {
			continue
		}

		migratabilityMap[node.Name] = true

		if node.Spec.Unschedulable {
			migratabilityMap[node.Name] = false
			continue
		}

		if vmi.Spec.Affinity != nil && vmi.Spec.Affinity.NodeAffinity != nil && vmi.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
			nodeSelectorTerms := vmi.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution

			var err error
			migratabilityMap[node.Name], err = schedulingcorev1.MatchNodeSelectorTerms(node, nodeSelectorTerms)
			if err != nil {
				return false, err
			}
		}
	}

	var migratable bool
	for _, isMigratable := range migratabilityMap {
		if isMigratable {
			migratable = true
			break
		}
	}

	return migratable, nil
}

// ListByNode Get a list of VMIs that are running on the specified node
// and that match the specified labels.
func ListByNode(node *corev1.Node, selector labels.Selector, cache ctlkubevirtv1.VirtualMachineInstanceCache) ([]*kubevirtv1.VirtualMachineInstance, error) {
	req, err := labels.NewRequirement(util.LabelNodeNameKey, selection.Equals, []string{node.Name})
	if err != nil {
		return nil, err
	}
	selector = selector.Add(*req)
	list, err := cache.List(corev1.NamespaceAll, selector)
	if err != nil {
		return nil, fmt.Errorf("failed to list VMIs on node %s: %w", node.Name, err)
	}
	return list, nil
}

func VMContainsCDRomOrContainerDisk(vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vmi == nil {
		return false
	}

	for _, disk := range vmi.Spec.Domain.Devices.Disks {
		if disk.CDRom != nil {
			return true
		}
	}

	for _, volume := range vmi.Spec.Volumes {
		if volume.VolumeSource.ContainerDisk != nil {
			return true
		}
	}
	return false
}
