package virtualmachine

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_virtualmachine_mutator(t *testing.T) {
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
			mutator := NewMutator(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings))
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
