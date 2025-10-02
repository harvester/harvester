package virtualmachine

import (
	"encoding/json"
	"fmt"
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
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	replaceOP        = "replace"
	nodeAffinityPath = "/spec/template/spec/affinity"
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
				`{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`,
			},
			setting: "",
		},
		{
			name: "has memory limit and other requests 2",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 32)), resource.BinarySI), // 4Gi
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
			},
			memory: nil,
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "1Gi"}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"3996Mi"}}`, // 4Gi - 100Mi
				`{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`,
			},
			setting: "",
		},
		{
			name: "has memory limit and other requests 3",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 34)), resource.BinarySI), // 16Gi
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
			},
			memory: nil,
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "4Gi"}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"16284Mi"}}`, // 16Gi - 100Mi
				`{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`,
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
				`{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`,
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
				`{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`,
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
				`{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests", "value": {"cpu":"100m","memory":"102Mi"}}`,
			},
			setting: `{"cpu":1000,"memory":1000,"storage":800}`,
		},
	}

	tests2 := []struct {
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
			name: "has memory limit, customized reserved memory and other requests",
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
				`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"640Mi"}}`, // 1Gi - 384Mi
				`{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`,
			},
			setting: "",
		},
		{
			name: "has memory limit, customized reserved memory and other requests 2",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 32)), resource.BinarySI), // 4Gi
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
			},
			memory: nil,
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "1Gi"}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"3712Mi"}}`, // 4Gi - 384Mi
				`{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`,
			},
			setting: "",
		},
	}

	tests3 := []struct {
		name        string
		resourceReq kubevirtv1.ResourceRequirements
		memory      *kubevirtv1.Memory
		patchOps    []string
		setting     string
	}{
		{
			name: "wrong reserved memory seeting get captured",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 28)), resource.BinarySI), // 256Mi
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
			},
			memory:   nil,
			patchOps: []string{},
			setting:  "",
		},
	}

	tests4 := []struct {
		name        string
		resourceReq kubevirtv1.ResourceRequirements
		memory      *kubevirtv1.Memory
		patchOps    []string
		setting     string
	}{
		{
			name: "too small guest OS memory get captured",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewQuantity(int64(385<<20), resource.BinarySI), // 385Mi, after sub 384 Mi reserved memory, only 1 Mi is left for guest OS
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
			},
			memory:   nil,
			patchOps: []string{},
			setting:  "",
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

	for _, tc := range tests2 {
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
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"harvesterhci.io/reservedMemory": "384Mi",
					},
				},
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

	for _, tc := range tests3 {
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
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"harvesterhci.io/reservedMemory": "384Mi",
					},
				},
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

			_, err = mutator.(*vmMutator).patchResourceOvercommit(vm)
			assert.NotNil(t, err)
			assert.Contains(t, err.Error(), "reservedMemory can't be equal or greater than limits.memory")
		})
	}

	for _, tc := range tests4 {
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
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"harvesterhci.io/reservedMemory": "384Mi",
					},
					Namespace: "test",
					Name:      "NotEnoughMemory",
				},
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

			_, err = mutator.(*vmMutator).patchResourceOvercommit(vm)
			assert.NotNil(t, err)
			assert.Contains(t, err.Error(), "guest memory is under the minimum requirement")
		})
	}
}

func TestPatchResourceOvercommitWithAdditionalGuestMemoryOverheadRatio(t *testing.T) {
	type testStruct struct {
		name                 string
		resourceReq          kubevirtv1.ResourceRequirements
		memory               *kubevirtv1.Memory
		patchOps             []string
		overcommitSetting    string
		overheadRatio        string
		invalidOverheadRatio bool
	}

	tests1 := []testStruct{
		{
			name: "has invalid OvercommitWithAdditionalGuestMemoryOverheadRatio then use default reserved memory",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 32)), resource.BinarySI), // 4Gi
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
			},
			memory: nil,
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "1Gi"}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"3996Mi"}}`, // 4Gi - 100Mi
				`{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`,
			},
			overcommitSetting:    "",
			overheadRatio:        "invalid",
			invalidOverheadRatio: true,
		},
	}

	tests2 := []testStruct{
		{
			name: "has invalid OvercommitWithAdditionalGuestMemoryOverheadRatioand and reserved memory annotation then use reserved memory annotation",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 33)), resource.BinarySI), // 8Gi
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
			},
			memory: nil,
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "2Gi"}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"7Gi"}}`, // 8Gi - 1Gi
				`{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`,
			},
			overcommitSetting:    "",
			overheadRatio:        "invalid",
			invalidOverheadRatio: true,
		},
	}

	tests3 := []testStruct{
		{
			// has valid OvercommitWithAdditionalGuestMemoryOverheadRatio and no reserved memory annotation
			name: "has valid OvercommitWithAdditionalGuestMemoryOverheadRatio 1",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 32)), resource.BinarySI), // 4Gi
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
			},
			memory: nil,
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "1Gi"}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"4Gi"}}`, // 4Gi
				`{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`,
			},
			overcommitSetting:    "",
			overheadRatio:        settings.AdditionalGuestMemoryOverheadRatioDefault,
			invalidOverheadRatio: false,
		},
	}

	tests4 := []testStruct{
		{
			name: "has valid OvercommitWithAdditionalGuestMemoryOverheadRatio and reserved memory annotation then use reserved memory",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 32)), resource.BinarySI), // 4Gi
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
			},
			memory: nil,
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "1Gi"}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"3Gi"}}`, // 3Gi
				`{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`,
			},
			overcommitSetting:    "",
			overheadRatio:        "4.8",
			invalidOverheadRatio: false,
		},
	}

	tests5 := []testStruct{
		{
			name: "has valid but zero OvercommitWithAdditionalGuestMemoryOverheadRatio then use default reserved memory",
			resourceReq: kubevirtv1.ResourceRequirements{
				Limits: map[v1.ResourceName]resource.Quantity{
					v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 32)), resource.BinarySI), // 4Gi
				},
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
				},
			},
			memory: nil,
			patchOps: []string{
				`{"op": "replace", "path": "/spec/template/spec/domain/resources/requests/memory", "value": "1Gi"}`,
				`{"op": "replace", "path": "/spec/template/spec/domain/memory", "value": {"guest":"3996Mi"}}`, // 4Gi - 100Mi
				`{"op": "replace", "path": "/spec/template/spec/domain/cpu/maxSockets", "value": 1}`,
			},
			overcommitSetting:    "",
			overheadRatio:        "0.0", // means clear current setting
			invalidOverheadRatio: false,
		},
	}

	setConfig := func(c *fake.Clientset, tc *testStruct) {
		setting := &harvesterv1.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name: settings.OvercommitConfigSettingName,
			},
			Default: `{"cpu":200,"memory":400,"storage":800}`,
		}
		if tc.overcommitSetting != "" {
			setting.Value = tc.overcommitSetting
		}
		err := c.Tracker().Add(setting)
		assert.Nil(t, err, "Mock resource should add into fake controller tracker")

		// invalid AdditionalGuestMemoryOverheadRatioConfig
		if tc.invalidOverheadRatio {
			_, err := settings.NewAdditionalGuestMemoryOverheadRatioConfig(tc.overheadRatio)
			assert.NotNil(t, err)
		} else {
			setting := &harvesterv1.Setting{
				ObjectMeta: metav1.ObjectMeta{
					Name: settings.AdditionalGuestMemoryOverheadRatioName,
				},
				Default: settings.AdditionalGuestMemoryOverheadRatioDefault,
				Value:   tc.overheadRatio,
			}
			err := c.Tracker().Add(setting)
			assert.Nil(t, err, "Mock resource should add into fake controller tracker")
		}
	}

	// has invalid OvercommitWithAdditionalGuestMemoryOverheadRatio then use default reserved memory
	for _, tc := range tests1 {
		t.Run(tc.name, func(t *testing.T) {
			// arrage
			clientset := fake.NewSimpleClientset()
			setConfig(clientset, &tc) // #nosec G601
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

	// has invalid OvercommitWithAdditionalGuestMemoryOverheadRatioand and reserved memory annotation then use reserved memory annotation
	for _, tc := range tests2 {
		t.Run(tc.name, func(t *testing.T) {
			// arrage
			clientset := fake.NewSimpleClientset()
			setConfig(clientset, &tc) // #nosec G601
			mutator := NewMutator(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
				fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions))
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"harvesterhci.io/reservedMemory": "1Gi",
					},
				},
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

	// has valid OvercommitWithAdditionalGuestMemoryOverheadRatio and no reserved memory annotation
	for _, tc := range tests3 {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			setConfig(clientset, &tc) // #nosec G601
			mutator := NewMutator(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
				fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions))
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{},
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

			actual, err := mutator.(*vmMutator).patchResourceOvercommit(vm)
			assert.Nil(t, err, tc.name)
			assert.Equal(t, tc.patchOps, actual)
		})
	}

	// has valid OvercommitWithAdditionalGuestMemoryOverheadRatio and reserved memory annotation then use reserved memory
	for _, tc := range tests4 {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			setConfig(clientset, &tc) // #nosec G601
			mutator := NewMutator(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
				fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions))
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"harvesterhci.io/reservedMemory": "1Gi",
					},
				},
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

			actual, err := mutator.(*vmMutator).patchResourceOvercommit(vm)
			assert.Nil(t, err, tc.name)
			assert.Equal(t, tc.patchOps, actual)
		})
	}

	// has valid but zero OvercommitWithAdditionalGuestMemoryOverheadRatio then use default reserved memory
	for _, tc := range tests5 {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			setConfig(clientset, &tc) // #nosec G601
			mutator := NewMutator(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
				fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions))
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{},
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

			actual, err := mutator.(*vmMutator).patchResourceOvercommit(vm)
			assert.Nil(t, err, tc.name)
			assert.Equal(t, tc.patchOps, actual)
		})
	}
}

func TestPatchResourceOvercommitWithDedicatedCPUPlacement(t *testing.T) {
	vm := &kubevirtv1.VirtualMachine{
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Resources: kubevirtv1.ResourceRequirements{
							Limits: map[v1.ResourceName]resource.Quantity{
								v1.ResourceCPU:    *resource.NewQuantity(int64(8), resource.DecimalSI),
								v1.ResourceMemory: *resource.NewQuantity(int64(math.Pow(2, 30)), resource.BinarySI), // 1Gi
							},
						},
						CPU: &kubevirtv1.CPU{
							Cores:                 8,
							Sockets:               1,
							DedicatedCPUPlacement: true,
						},
					},
				},
			},
		},
	}

	setting := &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: "overcommit-config",
		},
		Default: `{"cpu":200,"memory":400,"storage":800}`,
	}
	clientset := fake.NewSimpleClientset()
	err := clientset.Tracker().Add(setting)
	assert.Nil(t, err)
	mutator := NewMutator(fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		fakeclients.NetworkAttachmentDefinitionCache(clientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions))
	actual, err := mutator.(*vmMutator).patchResourceOvercommit(vm)
	assert.Nil(t, err)
	assert.Equal(t,
		[]string{
			"{\"op\": \"replace\", \"path\": \"/spec/template/spec/domain/memory\", \"value\": {\"guest\":\"924Mi\"}}",
			"{\"op\": \"replace\", \"path\": \"/spec/template/spec/domain/cpu/maxSockets\", \"value\": 1}",
			"{\"op\": \"replace\", \"path\": \"/spec/template/spec/domain/resources/requests\", \"value\": {\"cpu\":\"8\",\"memory\":\"1Gi\"}}"},
		actual)
}

func TestPatchAffinity(t *testing.T) {
	vm1 := &kubevirtv1.VirtualMachine{
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

	vm2 := &kubevirtv1.VirtualMachine{
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
												Key:      "network.harvesterhci.io/to-delete-after-mutating",
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
									NetworkName: "default/net1",
								},
							},
						},
					},
				},
			},
		},
	}

	vm3 := &kubevirtv1.VirtualMachine{
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
												Key:      "network.harvesterhci.io/bbbb",
												Operator: v1.NodeSelectorOpIn,
												Values:   []string{"true"},
											},
										},
									},
									{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      "just.for.testing",
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
									NetworkName: "default/net1",
								},
							},
						},
					},
				},
			},
		},
	}

	vm4 := &kubevirtv1.VirtualMachine{
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
												Key:      "network.harvesterhci.io/bbbb",
												Operator: v1.NodeSelectorOpIn,
												Values:   []string{"true"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	vm5 := &kubevirtv1.VirtualMachine{
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
												Key:      fmt.Sprintf("%s/%s", networkGroup, "mgmt"),
												Operator: v1.NodeSelectorOpIn,
												Values:   []string{"true"},
											},
											{
												Key:      "just.for.testing",
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
									NetworkName: "default/net1",
								},
							},
						},
					},
				},
			},
		},
	}
	vm6 := &kubevirtv1.VirtualMachine{
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Affinity: &v1.Affinity{
						PodAffinity: &v1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									TopologyKey: "topology.kubernetes.io/zone",
								},
							},
						},
					},
					Networks: []kubevirtv1.Network{
						{
							Name: "default",
							NetworkSource: kubevirtv1.NetworkSource{
								Pod: &kubevirtv1.PodNetwork{},
							},
						},
					},
				},
			},
		},
	}
	vm7 := &kubevirtv1.VirtualMachine{
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Affinity: &v1.Affinity{
						NodeAffinity: &v1.NodeAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []v1.PreferredSchedulingTerm{
								{
									Weight: 1,
									Preference: v1.NodeSelectorTerm{
										MatchExpressions: []v1.NodeSelectorRequirement{
											{
												Key:      "topology.kubernetes.io/zone",
												Operator: v1.NodeSelectorOpIn,
												Values:   []string{"zone1"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	vm8 := &kubevirtv1.VirtualMachine{
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						CPU: &kubevirtv1.CPU{
							DedicatedCPUPlacement: true,
						},
					},
				},
			},
		},
	}

	vm9 := &kubevirtv1.VirtualMachine{
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
												Key:      kubevirtv1.CPUManager,
												Operator: v1.NodeSelectorOpIn,
												Values:   []string{"true"},
											},
										},
									},
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

	clientSet := fake.NewSimpleClientset()
	nadGvr := schema.GroupVersionResource{
		Group:    "k8s.cni.cncf.io",
		Version:  "v1",
		Resource: "network-attachment-definitions",
	}
	if err := clientSet.Tracker().Create(nadGvr, net1.DeepCopy(), net1.Namespace); err != nil {
		t.Fatalf("failed to add net1 %+v", net1)
	}

	tests := []struct {
		name     string
		vm       *kubevirtv1.VirtualMachine
		affinity *v1.Affinity
	}{
		{
			name: "net1",
			vm:   vm1,
			affinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      fmt.Sprintf("%s/%s", networkGroup, "mgmt"),
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"true"},
								},
							},
						},
					},
					},
				},
			},
		},
		{
			name: "clearOldAffinity",
			vm:   vm2,
			affinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      fmt.Sprintf("%s/%s", networkGroup, "mgmt"),
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"true"},
								},
							},
						},
					},
					},
				},
			},
		},
		{
			name: "addRequirementToExistingTerms",
			vm:   vm3,
			affinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "just.for.testing",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"true"},
								},
								{
									Key:      fmt.Sprintf("%s/%s", networkGroup, "mgmt"),
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"true"},
								},
							},
						},
					},
					},
				},
			},
		},
		{
			name:     "emptyAffinity",
			vm:       vm4,
			affinity: &v1.Affinity{},
		},
		{
			name: "supportMultiExpressions",
			vm:   vm5,
			affinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "just.for.testing",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"true"},
								},
								{
									Key:      fmt.Sprintf("%s/%s", networkGroup, "mgmt"),
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"true"},
								},
							},
						},
					},
					},
				},
			},
		},
		{
			name: "keep pod affinity",
			vm:   vm6,
			affinity: &v1.Affinity{
				PodAffinity: &v1.PodAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
						{
							TopologyKey: "topology.kubernetes.io/zone",
						},
					},
				},
			},
		},
		{
			name: "no empty required affinity",
			vm:   vm7,
			affinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []v1.PreferredSchedulingTerm{
						{
							Weight: 1,
							Preference: v1.NodeSelectorTerm{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      "topology.kubernetes.io/zone",
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"zone1"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "add cpu manager affinity",
			vm:   vm8,
			affinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      kubevirtv1.CPUManager,
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"true"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:     "remove cpu manager affinity",
			vm:       vm9,
			affinity: &v1.Affinity{},
		},
	}

	type patch struct {
		Op    string       `json:"op"`
		Path  string       `json:"path"`
		Value *v1.Affinity `json:"value"`
	}

	for _, tc := range tests {
		mutator := NewMutator(fakeclients.HarvesterSettingCache(clientSet.HarvesterhciV1beta1().Settings),
			fakeclients.NetworkAttachmentDefinitionCache(clientSet.K8sCniCncfIoV1().NetworkAttachmentDefinitions))
		patchOps, err := mutator.(*vmMutator).patchAffinity(tc.vm, nil)
		assert.Nil(t, err, tc.name)

		patch := &patch{
			Op:    replaceOP,
			Path:  nodeAffinityPath,
			Value: tc.affinity,
		}

		bytes, err := json.Marshal(patch)
		if err != nil {
			assert.Nil(t, err, tc.name)
		}
		assert.Equal(t, types.PatchOps{string(bytes)}, patchOps)
	}
}
