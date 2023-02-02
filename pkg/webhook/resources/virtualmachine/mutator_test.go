package virtualmachine

import (
	"math"
	"testing"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func TestPatchResourceOvercommit(t *testing.T) {
	tests := []struct {
		name        string
		resourceReq kubevirtv1.ResourceRequirements
		memory      *kubevirtv1.Memory
		patchOps    []string
		setting     string
	}{
		{
			name:        "has no limits",
			resourceReq: kubevirtv1.ResourceRequirements{},
			patchOps:    nil,
			setting:     "",
		},
		{
			name: "has memory limit and other requests",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 30)), resource.BinarySI), // 1Gi
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
			},
			memory: nil,
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "256Mi"}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"924Mi"}}`, // 1Gi - 100Mi
			},
			setting: "",
		},
		{
			name: "has cpu limit and other requests",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 30)), resource.BinarySI), // 1Gi
				},
			},
			memory: nil,
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/cpu", "value": "500m"}`,
			},
			setting: "",
		},
		{
			name: "has both cpu and memory limits but not requests",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU:    *resource.NewMilliQuantity(1000, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 30)), resource.BinarySI), // 1Gi
				},
			},
			memory: nil,
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"924Mi"}}`, // 1Gi - 100Mi
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests", "value": {"cpu":"500m","memory":"256Mi"}}`,
			},
		},
		{
			name: "use value instead of default setting",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU:    *resource.NewMilliQuantity(1000, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 30)), resource.BinarySI), // 1Gi
				},
			},
			memory: nil,
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"924Mi"}}`, // 1Gi - 100Mi
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests", "value": {"cpu":"100m","memory":"102Mi"}}`,
			},
			setting: `{"cpu":1000,"memory":1000,"storage":800}`,
		},
		{
			name: "replace old guest memory",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU:    *resource.NewMilliQuantity(1000, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 30)), resource.BinarySI), // 1Gi
				},
			},
			memory: &kubevirtv1.Memory{
				Guest: resource.NewQuantity(int64(math.Pow(2, 40)), resource.BinarySI), // 1Ti
			},
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/memory/guest", "value": "924Mi"}`, // 1Gi - 100Mi
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests", "value": {"cpu":"100m","memory":"102Mi"}}`,
			},
			setting: `{"cpu":1000,"memory":1000,"storage":800}`,
		},
	}

	setting := &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: "overcommit-config",
		},
		Default: `{"cpu":200,"memory":400,"storage":800}`,
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrage
			clientset := fake.NewSimpleClientset()
			settingCpy := setting.DeepCopy()
			if tc.setting != "" {
				settingCpy.Value = tc.setting
			}
			err := clientset.Tracker().Add(settingCpy)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
			mutator := NewMutator(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
				fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions))
			vm := &kubevirtv1.VirtualMachine{
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{},
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: tc.resourceReq,
								Memory:    tc.memory,
							},
						},
					},
				},
			}

			// act
			actual, err := mutator.(*vmMutator).patchResourceOvercommit(vm)

			// assert
			assert.Nil(t, err, tc.name)
			assert.Equal(t, tc.patchOps, actual)
		})
	}
}

func TestPatchAffinity(t *testing.T) {
	oldVm := &kubevirtv1.VirtualMachine{
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Affinity: &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
								NodeSelectorTerms: []v1.NodeSelectorTerm{
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      "network.harvesterhci.io/test",
												Operator: v1.NodeSelectorOpIn,
												Values:   []string{"true"},
											},
										},
									},
								},
							},
						},
					},
					Networks: []kubevirtv1.Network{
						{
							Name: "default",
							NetworkSource: kubevirtv1.NetworkSource{
								Multus: &kubevirtv1.MultusNetwork{
									NetworkName: "default/net2",
								},
							},
						},
					},
				},
			},
		},
	}

	vm := &kubevirtv1.VirtualMachine{
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{
							Name: "default",
							NetworkSource: kubevirtv1.NetworkSource{
								Multus: &kubevirtv1.MultusNetwork{
									NetworkName: "default/net1",
								},
							},
						},
					},
				},
			},
		},
	}

	net1 := &cniv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "net1",
			Namespace: "default",
			Labels: map[string]string{
				"network.harvesterhci.io/clusternetwork": "mgmt",
				"network.harvesterhci.io/type":           "L2VlanNetwork",
				"network.harvesterhci.io/vlan-id":        "178",
			},
		},
		Spec: cniv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","name":"vlan178","type":"bridge","bridge":"mgmt-br","promiscMode":true,"vlan":178,"ipam":{}}`,
		},
	}

	net2 := &cniv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "net2",
			Namespace: "default",
			Labels: map[string]string{
				"network.harvesterhci.io/clusternetwork": "test",
				"network.harvesterhci.io/type":           "L2VlanNetwork",
				"network.harvesterhci.io/vlan-id":        "178",
			},
		},
		Spec: cniv1.NetworkAttachmentDefinitionSpec{
			Config: `{"cniVersion":"0.3.1","name":"vlan178","type":"bridge","bridge":"test-br","promiscMode":true,"vlan":178,"ipam":{}}`,
		},
	}

	clientSet := fake.NewSimpleClientset()
	nadGvr := schema.GroupVersionResource{
		Group:    "k8s.cni.cncf.io",
		Version:  "v1",
		Resource: "network-attachment-definitions",
	}
	if err := clientSet.Tracker().Create(nadGvr, net1.DeepCopy(), net1.Namespace); err != nil {
		t.Fatalf("failed to add net1 %+v", net1)
	}
	if err := clientSet.Tracker().Create(nadGvr, net2.DeepCopy(), net2.Namespace); err != nil {
		t.Fatalf("failed to add net1 %+v", net2)
	}

	tests := []struct {
		name     string
		oldVm    *kubevirtv1.VirtualMachine
		newVm    *kubevirtv1.VirtualMachine
		patchOps types.PatchOps
	}{
		{
			name:  "net1",
			newVm: vm,
			patchOps: types.PatchOps{
				`{"op": "replace", "path": "/spec/template/spec/affinity", "value": {"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"network.harvesterhci.io/mgmt","operator":"In","values":["true"]}]}]}}}}`,
			},
		},
		{
			name:  "net2ToNet1",
			oldVm: oldVm,
			newVm: vm,
			patchOps: types.PatchOps{
				`{"op": "replace", "path": "/spec/template/spec/affinity", "value": {"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"network.harvesterhci.io/mgmt","operator":"In","values":["true"]}]}]}}}}`,
			},
		},
	}

	for _, tc := range tests {
		mutator := NewMutator(fakeclients.HarvesterSettingCache(clientSet.HarvesterhciV1beta1().Settings),
			fakeclients.NetworkAttachmentDefinitionCache(clientSet.K8sCniCncfIoV1().NetworkAttachmentDefinitions))
		patchOps, err := mutator.(*vmMutator).patchAffinity(tc.oldVm, tc.newVm, nil)
		assert.Nil(t, err, tc.name)
		assert.Equal(t, tc.patchOps, patchOps)
	}
}
