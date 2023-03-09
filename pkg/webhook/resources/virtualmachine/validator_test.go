package virtualmachine

import (
	"context"
	"testing"

	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corefake "k8s.io/client-go/kubernetes/fake"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	apiharvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	cliharvesterv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
	kubevirttype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	kubevirtctrl "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/resourcequota"
	"github.com/harvester/harvester/pkg/util"
)

type vmWithResources struct {
	name            string
	runStrategy     kubevirtv1.VirtualMachineRunStrategy
	limitsCPU       string
	LimitsMemory    string
	printableStatus kubevirtv1.VirtualMachinePrintableStatus
}

var testCalcNonStoppedVMs = []*vmWithResources{
	// Running
	{"vm1", kubevirtv1.RunStrategyRerunOnFailure, "1000m", "4Gi", kubevirtv1.VirtualMachineStatusRunning},
	{"vm2", kubevirtv1.RunStrategyRerunOnFailure, "1000m", "4Gi", kubevirtv1.VirtualMachineStatusStarting},
	{"vm3", kubevirtv1.RunStrategyRerunOnFailure, "1000m", "4Gi", kubevirtv1.VirtualMachineStatusPaused},
	{"vm4", kubevirtv1.RunStrategyHalted, "1000m", "4Gi", kubevirtv1.VirtualMachineStatusStopping},

	// Restarting
	{"vm5", kubevirtv1.RunStrategyRerunOnFailure, "2000m", "4Gi", kubevirtv1.VirtualMachineStatusStopped},

	// Restoring
	{"vm6", kubevirtv1.RunStrategyRerunOnFailure, "2000m", "4Gi", kubevirtv1.VirtualMachineStatusUnschedulable},

	// Shutdown
	{"vm7", kubevirtv1.RunStrategyHalted, "1000m", "1Gi", kubevirtv1.VirtualMachineStatusStopped},
}

func getTestCalcNonStoppedVMs() []*kubevirtv1.VirtualMachine {
	var vms = make([]*kubevirtv1.VirtualMachine, 0, len(testCalcNonStoppedVMs))
	for _, r := range testCalcNonStoppedVMs {
		vms = append(vms, &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: r.name,
			},
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy: &r.runStrategy,
				Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							Resources: kubevirtv1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(r.limitsCPU),
									corev1.ResourceMemory: resource.MustParse(r.LimitsMemory),
								},
							},
						},
					},
				},
			},
			Status: kubevirtv1.VirtualMachineStatus{
				PrintableStatus: r.printableStatus,
			},
		})
	}
	return vms
}

func Test_vmValidator_checkVMResource(t *testing.T) {

	var clientset = fake.NewSimpleClientset()
	var coreclientset = corefake.NewSimpleClientset()

	var fields = &vmValidator{
		nsCache:        fakeNamespaceCache(coreclientset.CoreV1().Namespaces),
		vmCache:        fakeVirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
		vmRestoreCache: fakeVirtualMachineRestoreCache(clientset.HarvesterhciV1beta1().VirtualMachineRestores),
	}
	type args struct {
		name         string
		namespace    string
		uid          string
		limitsCPU    string
		limitsMemory string
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "basic 1",
			args: args{
				name:         "vm8",
				namespace:    corev1.NamespaceDefault,
				uid:          "1234567890",
				limitsCPU:    "1000m",
				limitsMemory: "2Gi",
			},
			wantErr: nil,
		},
		{
			name: "basic 2: non vm available resource",
			args: args{
				name:         "vm8",
				namespace:    "non-vmavailble",
				uid:          "1234567890",
				limitsCPU:    "1000m",
				limitsMemory: "2Gi",
			},
			wantErr: nil,
		},
		{
			name: "basic 3: cpu available resource",
			args: args{
				name:         "vm8",
				namespace:    "cpu-vmavailble",
				uid:          "1234567890",
				limitsCPU:    "1000m",
				limitsMemory: "2Gi",
			},
			wantErr: nil,
		},
		{
			name: "error 1: exceeded cpu",
			args: args{
				name:         "vm8",
				namespace:    corev1.NamespaceDefault,
				uid:          "1234567890",
				limitsCPU:    "3000m",
				limitsMemory: "2Gi",
			},
			wantErr: resourcequota.ErrVMAvailableResourceNotEnoughCPU,
		},
		{
			name: "error 2: exceeded memory",
			args: args{
				name:         "vm8",
				namespace:    corev1.NamespaceDefault,
				uid:          "1234567890",
				limitsCPU:    "1000m",
				limitsMemory: "4Gi",
			},
			wantErr: resourcequota.ErrVMAvailableResourceNotEnoughMemory,
		},
		{
			name: "error 3: only exceeded cpu with cpu limit",
			args: args{
				name:         "vm8",
				namespace:    "cpu-vmavailble",
				uid:          "1234567890",
				limitsCPU:    "3000m",
				limitsMemory: "4Gi",
			},
			wantErr: resourcequota.ErrVMAvailableResourceNotEnoughCPU,
		},
		{
			name: "error 3: only exceeded memory with memory limit",
			args: args{
				name:         "vm8",
				namespace:    "memory-vmavailble",
				uid:          "1234567890",
				limitsCPU:    "1000m",
				limitsMemory: "4Gi",
			},
			wantErr: resourcequota.ErrVMAvailableResourceNotEnoughMemory,
		},
	}
	v := &vmValidator{
		nsCache: fields.nsCache,
		vmCache: fields.vmCache,
		arq: resourcequota.NewAvailableResourceQuota(fields.vmCache,
			nil,
			nil,
			fields.vmRestoreCache,
			fields.nsCache),
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			newvm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.args.name,
					Namespace: tt.args.namespace,
					UID:       "1234567890",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: kubevirtv1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse(tt.args.limitsCPU),
										corev1.ResourceMemory: resource.MustParse(tt.args.limitsMemory),
									},
								},
							},
						},
					},
				},
			}
			assert.Equalf(t, tt.wantErr, v.checkVMResource(newvm), "checkVMResource(%v)", newvm)
		})
	}
}

type fakeNamespaceCache func() typedcorev1.NamespaceInterface

func (c fakeNamespaceCache) Get(name string) (*corev1.Namespace, error) {
	var resourceQuota, maintenanceQuota string
	if name == corev1.NamespaceDefault {
		resourceQuota = `{"limit":{"limitsCpu":"20000m","limitsMemory":"53248Mi"}}`
		maintenanceQuota = `{"limit":{"limitsCpuPercent":50,"limitsMemoryPercent":50}}`
	} else if name == "cpu-vmavailble" {
		resourceQuota = `{"limit":{"limitsCpu":"20000m"}}`
		maintenanceQuota = `{"limit":{"limitsCpuPercent":50}}`
	} else if name == "memory-vmavailble" {
		resourceQuota = `{"limit":{"limitsMemory":"53248Mi"}}`
		maintenanceQuota = `{"limit":{"limitsMemoryPercent":50}}`
	} else if name == "non-vmavailble" {
		resourceQuota = ``
		maintenanceQuota = ``
	}
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				util.AnnotationResourceQuota:    resourceQuota,
				util.AnnotationMaintenanceQuota: maintenanceQuota,
			},
		},
	}, nil
}

func (c fakeNamespaceCache) List(selector labels.Selector) ([]*corev1.Namespace, error) {
	panic("implement me")
}

func (c fakeNamespaceCache) AddIndexer(indexName string, indexer ctlcorev1.NamespaceIndexer) {
	panic("implement me")
}

func (c fakeNamespaceCache) GetByIndex(indexName, key string) ([]*corev1.Namespace, error) {
	panic("implement me")
}

type fakeVirtualMachineCache func(string) kubevirttype.VirtualMachineInterface

func (c fakeVirtualMachineCache) Get(namespace, name string) (*kubevirtv1.VirtualMachine, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVirtualMachineCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1.VirtualMachine, error) {
	return getTestCalcNonStoppedVMs(), nil
}

func (c fakeVirtualMachineCache) AddIndexer(indexName string, indexer kubevirtctrl.VirtualMachineIndexer) {
	panic("implement me")
}

func (c fakeVirtualMachineCache) GetByIndex(indexName, key string) ([]*kubevirtv1.VirtualMachine, error) {
	panic("implement me")
}

type fakeVirtualMachineRestoreCache func(string) cliharvesterv1.VirtualMachineRestoreInterface

func (c fakeVirtualMachineRestoreCache) Get(namespace, name string) (*apiharvesterv1.VirtualMachineRestore, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVirtualMachineRestoreCache) List(namespace string, selector labels.Selector) ([]*apiharvesterv1.VirtualMachineRestore, error) {
	return nil, nil
}
func (c fakeVirtualMachineRestoreCache) AddIndexer(indexName string, indexer ctlharvesterv1.VirtualMachineRestoreIndexer) {
	panic("implement me")
}

func (c fakeVirtualMachineRestoreCache) GetByIndex(indexName, key string) ([]*apiharvesterv1.VirtualMachineRestore, error) {
	panic("implement me")
}
