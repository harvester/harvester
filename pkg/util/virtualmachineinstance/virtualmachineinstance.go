package virtualmachineinstance

import (
	corev1 "k8s.io/api/core/v1"
	schedulingcorev1 "k8s.io/component-helpers/scheduling/corev1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

func GetAllNonLiveMigratableVMINames(vmis []*kubevirtv1.VirtualMachineInstance, nodes []*corev1.Node) ([]string, error) {
	var nonLiveMigratableVMINames []string

	// Skip for single-node clusters
	if len(nodes) == 1 {
		return nonLiveMigratableVMINames, nil
	}

	for _, vmi := range vmis {
		// Node selectors
		if vmi.Spec.NodeSelector != nil {
			nonLiveMigratableVMINames = append(nonLiveMigratableVMINames, vmi.Namespace+"/"+vmi.Name)
			continue
		}

		// PCIe devices
		if vmi.Spec.Domain.Devices.HostDevices != nil {
			nonLiveMigratableVMINames = append(nonLiveMigratableVMINames, vmi.Namespace+"/"+vmi.Name)
			continue
		}

		// Node affinities
		migratable, err := migratableByNodeAffinity(vmi, nodes)
		if err != nil {
			return nonLiveMigratableVMINames, err
		}
		if !migratable {
			nonLiveMigratableVMINames = append(nonLiveMigratableVMINames, vmi.Namespace+"/"+vmi.Name)
		}
	}

	return nonLiveMigratableVMINames, nil
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
