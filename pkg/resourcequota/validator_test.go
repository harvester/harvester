package resourcequota

import (
	"context"
	"fmt"
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
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	maintenanceQuotaJSONFmt = `{"limit":{"limitsCpuPercent": %d, "LimitsMemoryPercent": %d}}`
)

type vmWithResources struct {
	name            string
	runStrategy     kubevirtv1.VirtualMachineRunStrategy
	limitsCPU       string
	LimitsMemory    string
	printableStatus kubevirtv1.VirtualMachinePrintableStatus
}

type vmimWithResources struct {
	vmiName string
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

var testMigratingVMIMs = []*vmimWithResources{
	// Running
	{"vm1"},
	{"vm2"},
	{"vm3"},
	{"vm4"},
	{"vm5"},
	{"vm6"},
}

func getTestCalcNonStoppedVMs() []*kubevirtv1.VirtualMachine {
	var vms = make([]*kubevirtv1.VirtualMachine, 0, len(testCalcNonStoppedVMs))
	for _, r := range testCalcNonStoppedVMs {
		vms = append(vms, &kubevirtv1.VirtualMachine{
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

func getMigratingVMIMs() []*kubevirtv1.VirtualMachineInstanceMigration {
	var vms = make([]*kubevirtv1.VirtualMachineInstanceMigration, 0, len(testMigratingVMIMs))
	for _, r := range testMigratingVMIMs {
		vms = append(vms, &kubevirtv1.VirtualMachineInstanceMigration{
			Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{
				VMIName: r.vmiName,
			},
		})
	}
	return vms
}

func getVM(name string) *kubevirtv1.VirtualMachine {
	for _, r := range testCalcNonStoppedVMs {
		if r.name == name {
			return &kubevirtv1.VirtualMachine{
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
			}
		}
	}
	return nil
}

func TestAvailableResourceQuota_ValidateMaintenanceResourcesField(t *testing.T) {
	type args struct {
		cpu    int64
		memory int64
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "validate maintenance resources field",
			args: args{
				cpu:    50,
				memory: 50,
			},
			wantErr: false,
		},
		{
			name: "validate cpu field",
			args: args{
				cpu: 50,
			},
			wantErr: false,
		},
		{
			name: "validate memory field",
			args: args{
				memory: 50,
			},
			wantErr: false,
		},
		{
			name: "exceeds cpu range",
			args: args{
				memory: 101,
			},
			wantErr: true,
		},
		{
			name: "exceeds cpu range 2",
			args: args{
				cpu: -1,
			},
			wantErr: true,
		},
		{
			name: "exceeds memory range",
			args: args{
				memory: 101,
			},
			wantErr: true,
		},
		{
			name: "exceeds memory range 2",
			args: args{
				memory: -1,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &AvailableResourceQuota{}
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						util.AnnotationMaintenanceQuota: fmt.Sprintf(maintenanceQuotaJSONFmt, tt.args.cpu, tt.args.memory),
					},
				},
			}
			if err := q.ValidateMaintenanceResourcesField(ns); (err != nil) != tt.wantErr {
				t.Errorf("ValidateMaintenanceResourcesField() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAvailableResourceQuota_CheckVMAvailableResoruces(t *testing.T) {

	var clientset = fake.NewSimpleClientset()
	var coreclientset = corefake.NewSimpleClientset()

	type args struct {
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
				namespace:    corev1.NamespaceDefault,
				uid:          "1234567890",
				limitsCPU:    "1000m",
				limitsMemory: "4Gi",
			},
			wantErr: nil,
		},
		{
			name: "basic 2: non vm available resource",
			args: args{
				namespace:    "non-vmavailble",
				uid:          "1234567890",
				limitsCPU:    "1000m",
				limitsMemory: "4Gi",
			},
			wantErr: nil,
		},
		{
			name: "basic 3: cpu available resource",
			args: args{
				namespace:    "cpu-vmavailble",
				uid:          "1234567890",
				limitsCPU:    "1000m",
				limitsMemory: "4Gi",
			},
			wantErr: nil,
		},
		{
			name: "error 1: exceeded cpu",
			args: args{
				namespace:    corev1.NamespaceDefault,
				uid:          "1234567890",
				limitsCPU:    "3000m",
				limitsMemory: "4Gi",
			},
			wantErr: ErrVMAvailableResourceNotEnoughCPU,
		},
		{
			name: "error 2: exceeded memory",
			args: args{
				namespace:    corev1.NamespaceDefault,
				uid:          "1234567890",
				limitsCPU:    "1000m",
				limitsMemory: "8Gi",
			},
			wantErr: ErrVMAvailableResourceNotEnoughMemory,
		},
		{
			name: "error 3: only exceeded cpu with cpu limit",
			args: args{
				namespace:    "cpu-vmavailble",
				uid:          "1234567890",
				limitsCPU:    "3000m",
				limitsMemory: "4Gi",
			},
			wantErr: ErrVMAvailableResourceNotEnoughCPU,
		},
		{
			name: "error 3: only exceeded memory with memory limit",
			args: args{
				namespace:    "memory-vmavailble",
				uid:          "1234567890",
				limitsCPU:    "1000m",
				limitsMemory: "8Gi",
			},
			wantErr: ErrVMAvailableResourceNotEnoughMemory,
		},
	}
	arq := NewAvailableResourceQuota(
		fakeVirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
		fakeVirtualMachineInstanceMigrationCache(clientset.KubevirtV1().VirtualMachineInstanceMigrations),
		fakeVirtualMachineBackupCache(clientset.HarvesterhciV1beta1().VirtualMachineBackups),
		fakeVirtualMachineRestoreCache(clientset.HarvesterhciV1beta1().VirtualMachineRestores),
		fakeNamespaceCache(coreclientset.CoreV1().Namespaces),
	)
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			newvm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
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
			assert.Equalf(t, tt.wantErr, arq.CheckVMAvailableResoruces(newvm), "CheckVMAvailableResoruces(%v)", newvm)
		})
	}
}

func TestAvailableResourceQuota_CheckMaintenanceAvailableResoruces(t *testing.T) {

	var clientset = fake.NewSimpleClientset()
	var coreclientset = corefake.NewSimpleClientset()

	type args struct {
		namespace string
		vminame   string
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "basic 1",
			args: args{
				namespace: corev1.NamespaceDefault,
				vminame:   "vm1",
			},
			wantErr: nil,
		},
		{
			name: "basic 2: non-set resource quota",
			args: args{
				namespace: "non-vmavailble",
				vminame:   "vm1",
			},
			wantErr: nil,
		},
		{
			name: "basic 3: cpu available resource",
			args: args{
				namespace: "cpu-vmavailble",
				vminame:   "vm1",
			},
			wantErr: nil,
		},
		{
			name: "error 1: exceeded cpu",
			args: args{
				namespace: "exceed-cpu",
				vminame:   "vm1",
			},
			wantErr: ErrVMAvailableResourceNotEnoughCPU,
		},
		{
			name: "error 2: exceeded memory",
			args: args{
				namespace: "exceed-memory",
				vminame:   "vm1",
			},
			wantErr: ErrVMAvailableResourceNotEnoughMemory,
		},
	}
	arq := NewAvailableResourceQuota(
		fakeVirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
		fakeVirtualMachineInstanceMigrationCache(clientset.KubevirtV1().VirtualMachineInstanceMigrations),
		fakeVirtualMachineBackupCache(clientset.HarvesterhciV1beta1().VirtualMachineBackups),
		fakeVirtualMachineRestoreCache(clientset.HarvesterhciV1beta1().VirtualMachineRestores),
		fakeNamespaceCache(coreclientset.CoreV1().Namespaces),
	)
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			newvmim := &kubevirtv1.VirtualMachineInstanceMigration{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: tt.args.namespace,
					UID:       "1234567890",
				},
				Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{
					VMIName: tt.args.vminame,
				},
			}
			assert.Equalf(t, tt.wantErr, arq.CheckMaintenanceAvailableResoruces(newvmim), "CheckMaintenanceAvailableResoruces(%v)", newvmim)
		})
	}
}

type fakeNamespaceCache func() typedcorev1.NamespaceInterface

func (c fakeNamespaceCache) Get(name string) (*corev1.Namespace, error) {
	var resourceQuota, maintenanceQuota string
	if name == corev1.NamespaceDefault {
		resourceQuota = `{"limit":{"limitsCpu":"20000m","limitsMemory":"57344Mi"}}`
		maintenanceQuota = `{"limit":{"limitsCpuPercent":50,"limitsMemoryPercent":50}}`
	} else if name == "cpu-vmavailble" {
		resourceQuota = `{"limit":{"limitsCpu":"20000m"}}`
		maintenanceQuota = `{"limit":{"limitsCpuPercent":50}}`
	} else if name == "memory-vmavailble" {
		resourceQuota = `{"limit":{"limitsMemory":"57344Mi"}}`
		maintenanceQuota = `{"limit":{"limitsMemoryPercent":50}}`
	} else if name == "exceed-cpu" {
		resourceQuota = `{"limit":{"limitsCpu":"16000m"}}`
		maintenanceQuota = `{"limit":{"limitsCpuPercent":50}}`
	} else if name == "exceed-memory" {
		resourceQuota = `{"limit":{"limitsMemory":"49152Mi"}}`
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
	return getVM(name), nil
}

func (c fakeVirtualMachineCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1.VirtualMachine, error) {
	return getTestCalcNonStoppedVMs(), nil
}

func (c fakeVirtualMachineCache) AddIndexer(indexName string, indexer ctlkubevirtv1.VirtualMachineIndexer) {
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

type fakeVirtualMachineInstanceMigrationCache func(string) kubevirttype.VirtualMachineInstanceMigrationInterface

func (c fakeVirtualMachineInstanceMigrationCache) Get(namespace, name string) (*kubevirtv1.VirtualMachineInstanceMigration, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVirtualMachineInstanceMigrationCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1.VirtualMachineInstanceMigration, error) {
	return getMigratingVMIMs(), nil
}

func (c fakeVirtualMachineInstanceMigrationCache) AddIndexer(indexName string, indexer ctlkubevirtv1.VirtualMachineInstanceMigrationIndexer) {
	panic("implement me")
}

func (c fakeVirtualMachineInstanceMigrationCache) GetByIndex(indexName, key string) ([]*kubevirtv1.VirtualMachineInstanceMigration, error) {
	panic("implement me")
}

type fakeVirtualMachineBackupCache func(string) cliharvesterv1.VirtualMachineBackupInterface

func (c fakeVirtualMachineBackupCache) Get(namespace, name string) (*apiharvesterv1.VirtualMachineBackup, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVirtualMachineBackupCache) List(namespace string, selector labels.Selector) ([]*apiharvesterv1.VirtualMachineBackup, error) {
	return nil, nil
}

func (c fakeVirtualMachineBackupCache) AddIndexer(indexName string, indexer ctlharvesterv1.VirtualMachineBackupIndexer) {
	panic("implement me")
}

func (c fakeVirtualMachineBackupCache) GetByIndex(indexName, key string) ([]*apiharvesterv1.VirtualMachineBackup, error) {
	panic("implement me")
}
