/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha4

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func (src *Cluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.Cluster)

	if err := Convert_v1alpha4_Cluster_To_v1beta1_Cluster(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &clusterv1.Cluster{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	if restored.Spec.Topology != nil {
		if dst.Spec.Topology == nil {
			dst.Spec.Topology = &clusterv1.Topology{}
		}
		dst.Spec.Topology.Variables = restored.Spec.Topology.Variables
		if restored.Spec.Topology.Workers != nil {
			if dst.Spec.Topology.Workers == nil {
				dst.Spec.Topology.Workers = &clusterv1.WorkersTopology{}
			}
			for i := range restored.Spec.Topology.Workers.MachineDeployments {
				dst.Spec.Topology.Workers.MachineDeployments[i].FailureDomain = restored.Spec.Topology.Workers.MachineDeployments[i].FailureDomain
				dst.Spec.Topology.Workers.MachineDeployments[i].Variables = restored.Spec.Topology.Workers.MachineDeployments[i].Variables
			}
		}
	}

	return nil
}

func (dst *Cluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.Cluster)

	if err := Convert_v1beta1_Cluster_To_v1alpha4_Cluster(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *ClusterList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.ClusterList)

	return Convert_v1alpha4_ClusterList_To_v1beta1_ClusterList(src, dst, nil)
}

func (dst *ClusterList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.ClusterList)

	return Convert_v1beta1_ClusterList_To_v1alpha4_ClusterList(src, dst, nil)
}

func (src *ClusterClass) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.ClusterClass)

	if err := Convert_v1alpha4_ClusterClass_To_v1beta1_ClusterClass(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &clusterv1.ClusterClass{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Spec.Patches = restored.Spec.Patches
	dst.Spec.Variables = restored.Spec.Variables
	dst.Spec.ControlPlane.MachineHealthCheck = restored.Spec.ControlPlane.MachineHealthCheck

	for i := range restored.Spec.Workers.MachineDeployments {
		dst.Spec.Workers.MachineDeployments[i].MachineHealthCheck = restored.Spec.Workers.MachineDeployments[i].MachineHealthCheck
	}

	return nil
}

func (dst *ClusterClass) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.ClusterClass)

	if err := Convert_v1beta1_ClusterClass_To_v1alpha4_ClusterClass(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

func (src *ClusterClassList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.ClusterClassList)

	return Convert_v1alpha4_ClusterClassList_To_v1beta1_ClusterClassList(src, dst, nil)
}

func (dst *ClusterClassList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.ClusterClassList)

	return Convert_v1beta1_ClusterClassList_To_v1alpha4_ClusterClassList(src, dst, nil)
}

func (src *Machine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.Machine)

	return Convert_v1alpha4_Machine_To_v1beta1_Machine(src, dst, nil)
}

func (dst *Machine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.Machine)

	return Convert_v1beta1_Machine_To_v1alpha4_Machine(src, dst, nil)
}

func (src *MachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineList)

	return Convert_v1alpha4_MachineList_To_v1beta1_MachineList(src, dst, nil)
}

func (dst *MachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineList)

	return Convert_v1beta1_MachineList_To_v1alpha4_MachineList(src, dst, nil)
}

func (src *MachineSet) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineSet)

	return Convert_v1alpha4_MachineSet_To_v1beta1_MachineSet(src, dst, nil)
}

func (dst *MachineSet) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineSet)

	return Convert_v1beta1_MachineSet_To_v1alpha4_MachineSet(src, dst, nil)
}

func (src *MachineSetList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineSetList)

	return Convert_v1alpha4_MachineSetList_To_v1beta1_MachineSetList(src, dst, nil)
}

func (dst *MachineSetList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineSetList)

	return Convert_v1beta1_MachineSetList_To_v1alpha4_MachineSetList(src, dst, nil)
}

func (src *MachineDeployment) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineDeployment)

	return Convert_v1alpha4_MachineDeployment_To_v1beta1_MachineDeployment(src, dst, nil)
}

func (dst *MachineDeployment) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineDeployment)

	return Convert_v1beta1_MachineDeployment_To_v1alpha4_MachineDeployment(src, dst, nil)
}

func (src *MachineDeploymentList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineDeploymentList)

	return Convert_v1alpha4_MachineDeploymentList_To_v1beta1_MachineDeploymentList(src, dst, nil)
}

func (dst *MachineDeploymentList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineDeploymentList)

	return Convert_v1beta1_MachineDeploymentList_To_v1alpha4_MachineDeploymentList(src, dst, nil)
}

func (src *MachineHealthCheck) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineHealthCheck)

	return Convert_v1alpha4_MachineHealthCheck_To_v1beta1_MachineHealthCheck(src, dst, nil)
}

func (dst *MachineHealthCheck) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineHealthCheck)

	return Convert_v1beta1_MachineHealthCheck_To_v1alpha4_MachineHealthCheck(src, dst, nil)
}

func (src *MachineHealthCheckList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*clusterv1.MachineHealthCheckList)

	return Convert_v1alpha4_MachineHealthCheckList_To_v1beta1_MachineHealthCheckList(src, dst, nil)
}

func (dst *MachineHealthCheckList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*clusterv1.MachineHealthCheckList)

	return Convert_v1beta1_MachineHealthCheckList_To_v1alpha4_MachineHealthCheckList(src, dst, nil)
}

func Convert_v1alpha4_MachineStatus_To_v1beta1_MachineStatus(in *MachineStatus, out *clusterv1.MachineStatus, s apiconversion.Scope) error {
	// Status.version has been removed in v1beta1, thus requiring custom conversion function. the information will be dropped.
	return autoConvert_v1alpha4_MachineStatus_To_v1beta1_MachineStatus(in, out, s)
}

func Convert_v1beta1_ClusterClassSpec_To_v1alpha4_ClusterClassSpec(in *clusterv1.ClusterClassSpec, out *ClusterClassSpec, s apiconversion.Scope) error {
	// spec.{variables,patches} has been added with v1beta1.
	return autoConvert_v1beta1_ClusterClassSpec_To_v1alpha4_ClusterClassSpec(in, out, s)
}

func Convert_v1beta1_Topology_To_v1alpha4_Topology(in *clusterv1.Topology, out *Topology, s apiconversion.Scope) error {
	// spec.topology.variables has been added with v1beta1.
	return autoConvert_v1beta1_Topology_To_v1alpha4_Topology(in, out, s)
}

// Convert_v1beta1_MachineDeploymentTopology_To_v1alpha4_MachineDeploymentTopology is an autogenerated conversion function.
func Convert_v1beta1_MachineDeploymentTopology_To_v1alpha4_MachineDeploymentTopology(in *clusterv1.MachineDeploymentTopology, out *MachineDeploymentTopology, s apiconversion.Scope) error {
	// MachineDeploymentTopology.FailureDomain has been added with v1beta1.
	return autoConvert_v1beta1_MachineDeploymentTopology_To_v1alpha4_MachineDeploymentTopology(in, out, s)
}

func Convert_v1beta1_MachineDeploymentClass_To_v1alpha4_MachineDeploymentClass(in *clusterv1.MachineDeploymentClass, out *MachineDeploymentClass, s apiconversion.Scope) error {
	// machineDeploymentClass.machineHealthCheck has been added with v1beta1.
	return autoConvert_v1beta1_MachineDeploymentClass_To_v1alpha4_MachineDeploymentClass(in, out, s)
}

func Convert_v1beta1_ControlPlaneClass_To_v1alpha4_ControlPlaneClass(in *clusterv1.ControlPlaneClass, out *ControlPlaneClass, s apiconversion.Scope) error {
	// controlPlaneClass.machineHealthCheck has been added with v1beta1.
	return autoConvert_v1beta1_ControlPlaneClass_To_v1alpha4_ControlPlaneClass(in, out, s)
}
