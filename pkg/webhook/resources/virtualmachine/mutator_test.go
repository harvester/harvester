package virtualmachine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/client-go/api/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_virtualmachine_mutator(t *testing.T) {
	tests := []struct {
		name        string
		resourceReq kubevirtv1.ResourceRequirements
		patchOps    []string
	}{
		{
			name: "has no limit",
			resourceReq: kubevirtv1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewScaledQuantity(1, resource.Giga),
					v1.ResourceCPU:    *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
			},
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/cpu", "value": "500m"}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "250M"}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/limits", "value": {"cpu":"1","memory":"1G"}}`,
			},
		},
		{
			name: "has memory limit",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewScaledQuantity(1, resource.Giga),
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
			},
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/cpu", "value": "500m"}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/limits/cpu", "value": "1"}`,
			},
		},
		{
			name: "has cpu limit",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewScaledQuantity(1, resource.Giga),
				},
			},
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "250M"}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/limits/memory", "value": "1G"}`,
			},
		},
		{
			name: "has both cpu and memory limits",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU:    *resource.NewMilliQuantity(1000, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewScaledQuantity(1, resource.Giga),
				},
			},
			patchOps: nil,
		},
	}

	setting := &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: "overcommit-config",
		},
		Value: `{"cpu":200,"memory":400,"storage":800}`,
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrage
			clientset := fake.NewSimpleClientset()
			err := clientset.Tracker().Add(setting)
			mutator := NewMutator(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings))
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
			vm := &kubevirtv1.VirtualMachine{
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{},
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: tc.resourceReq,
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
