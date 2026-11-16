package resourcequota

import (
	"encoding/json"
	"testing"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corefake "k8s.io/client-go/kubernetes/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

var (
	bothExist = &v3.NamespaceResourceQuota{
		Limit: v3.ResourceQuotaLimit{
			LimitsCPU:    "2000m",
			LimitsMemory: "3000Mi",
		},
	}
	cpuExist = &v3.NamespaceResourceQuota{
		Limit: v3.ResourceQuotaLimit{
			LimitsCPU: "2000m",
		},
	}
	memoryExist = &v3.NamespaceResourceQuota{
		Limit: v3.ResourceQuotaLimit{
			LimitsMemory: "3000Mi",
		},
	}
	noneExist *v3.NamespaceResourceQuota
)

const (
	namespaceBothExist   = "both-exist"
	namespaceCPUExist    = "cpu-exist"
	namespaceMemoryExist = "memory-exist"
	namespaceNoneExist   = "none-exist"

	uid1 = "1afcf4d9-b8a7-464a-a4e9-abe81fc7eacd"
	uid2 = "2afcf4d9-b8a7-464a-a4e9-abe81fc7eacd"

	memory1Gi = 1 * 1024 * 1024 * 1024
	testNS    = "default"
)

func Test_vmValidator_checkResourceQuota(t *testing.T) {
	var coreclientset = corefake.NewSimpleClientset()

	type args struct {
		vm *kubevirtv1.VirtualMachine
	}
	tests := []struct {
		name    string
		args    args
		want    *v3.NamespaceResourceQuota
		wantErr error
	}{
		{
			name: "both exist",
			args: args{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespaceBothExist,
					},
				},
			},
			want:    bothExist,
			wantErr: nil,
		}, {
			name: "cpu exist",
			args: args{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespaceCPUExist,
					},
				},
			},
			want:    cpuExist,
			wantErr: nil,
		}, {
			name: "memory exist",
			args: args{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespaceMemoryExist,
					},
				},
			},
			want:    memoryExist,
			wantErr: nil,
		}, {
			name: "none exist",
			args: args{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespaceNoneExist,
					},
				},
			},
			want:    noneExist,
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if tt.args.vm != nil {
				namespace := "default"
				if tt.args.vm.Namespace != "" {
					namespace = tt.args.vm.Namespace
				}
				ns := getNamespace(namespace)
				err := coreclientset.Tracker().Add(ns)
				assert.Nil(t, err, "Mock resource should add into fake controller tracker")
			}

			c := NewCalculator(
				fakeclients.NamespaceCache(coreclientset.CoreV1().Namespaces),
				nil,
				nil,
				nil,
				nil)

			got, err := c.getNamespaceResourceQuota(tt.args.vm)
			assert.Equalf(t, tt.wantErr, err, "getNamespaceResourceQuota(%v)", tt.args.vm)
			assert.Equalf(t, tt.want, got, "getNamespaceResourceQuota(%v)", tt.args.vm)
		})
	}
}

func getNamespace(name string) *corev1.Namespace {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	if name == namespaceBothExist {
		str, _ := serializeNamespaceResourceQuota(bothExist)
		ns.Annotations = map[string]string{util.CattleAnnotationResourceQuota: str}
	} else if name == namespaceCPUExist {
		str, _ := serializeNamespaceResourceQuota(cpuExist)
		ns.Annotations = map[string]string{util.CattleAnnotationResourceQuota: str}
	} else if name == namespaceMemoryExist {
		str, _ := serializeNamespaceResourceQuota(memoryExist)
		ns.Annotations = map[string]string{util.CattleAnnotationResourceQuota: str}
	}

	return ns
}

func serializeNamespaceResourceQuota(quota *v3.NamespaceResourceQuota) (string, error) {
	if quota == nil {
		return "", nil
	}

	data, err := json.Marshal(quota)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func Test_vmValidator_containsEnoughResourceQuotaToStartVM(t *testing.T) {
	generateRQ := func(cpu, mem int64) kubevirtv1.ResourceRequirements {
		return kubevirtv1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: *resource.NewQuantity(mem*memory1Gi, resource.DecimalSI),
				corev1.ResourceCPU:    *resource.NewQuantity(cpu, resource.DecimalSI),
			},
		}
	}

	nsrq := v3.NamespaceResourceQuota{
		Limit: v3.ResourceQuotaLimit{
			LimitsCPU:    "4000m",
			LimitsMemory: "10240Mi",
		},
	}

	tests := []struct {
		name    string
		vm      *kubevirtv1.VirtualMachine
		nsrq    *v3.NamespaceResourceQuota
		rq      *corev1.ResourceQuota
		wantErr bool
	}{
		{
			name: "VM can start as quota is enough",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: testNS,
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: generateRQ(1, 1),
							},
						},
					},
				},
			},
			nsrq: &nsrq,
			rq: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNS,
					Name:      "rq1",
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota: "{\"limit\":{\"limitsCpu\":\"4\", \"limitsMemory\":\"10240Mi\"}}",
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(4, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.DecimalSI),
					},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(1, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(2*memory1Gi, resource.DecimalSI),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM can't start due to insufficient memory",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: testNS,
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: generateRQ(1, 1), // with overhead, the VM requires > 1Gi memory
							},
						},
					},
				},
			},
			nsrq: &nsrq,
			rq: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNS,
					Name:      "rq1",
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota: "{\"limit\":{\"limitsCpu\":\"4\", \"limitsMemory\":\"10240Mi\"}}",
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(4, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.DecimalSI),
					},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(2, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(9*memory1Gi, resource.DecimalSI), // only 1Gi memory is available
					},
				},
			},
			wantErr: true,
		},
		{
			name: "VM can't start due to insufficient cpu",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: testNS,
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: generateRQ(1, 1),
							},
						},
					},
				},
			},
			nsrq: &nsrq,
			rq: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNS,
					Name:      "rq1",
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota: "{\"limit\":{\"limitsCpu\":\"4\", \"limitsMemory\":\"10240Mi\"}}",
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(4, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.DecimalSI),
					},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(4, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(2*memory1Gi, resource.DecimalSI),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "VM can start and RQ is also scaled",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: testNS,
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: generateRQ(1, 1),
							},
						},
					},
				},
			},
			nsrq: &nsrq,
			rq: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNS,
					Name:      "rq1",
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota:             "{\"limit\":{\"limitsCpu\":\"4\", \"limitsMemory\":\"10240Mi\"}}",
						util.GenerateAnnotationKeyMigratingVMUID(uid1): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						util.GenerateAnnotationKeyMigratingVMUID(uid2): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(6, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(14.5*memory1Gi, resource.DecimalSI), // already scaled
					},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(4, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(8*memory1Gi, resource.DecimalSI),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM can't start and RQ is also scaled",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: testNS,
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: generateRQ(1, 1),
							},
						},
					},
				},
			},
			nsrq: &nsrq,
			rq: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNS,
					Name:      "rq1",
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota:             "{\"limit\":{\"limitsCpu\":\"4\", \"limitsMemory\":\"10240Mi\"}}",
						util.GenerateAnnotationKeyMigratingVMUID(uid1): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						util.GenerateAnnotationKeyMigratingVMUID(uid2): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(6, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(14.5*memory1Gi, resource.DecimalSI),
					},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(3, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(13*memory1Gi, resource.DecimalSI), // 10 - ( 13 - 2 -2 ) = 1, does not meet the VM's requirement
					},
				},
			},
			wantErr: true,
		},
		{
			name: "VM can start and RQ is also scaled",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: testNS,
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: generateRQ(1, 1),
							},
						},
					},
				},
			},
			nsrq: &nsrq,
			rq: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNS,
					Name:      "rq1",
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota:                          "{\"limit\":{\"limitsCpu\":\"4\", \"limitsMemory\":\"10240Mi\"}}",
						util.GenerateAnnotationKeyMigratingVMName("vminmigration1"): `{"limits.cpu":"1","limits.memory":"2Gi"}`, // name based key still works
						util.GenerateAnnotationKeyMigratingVMName("vminmigration2"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(6, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(14.5*memory1Gi, resource.DecimalSI),
					},
				},
				Status: corev1.ResourceQuotaStatus{
					Used: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(3, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(12*memory1Gi, resource.DecimalSI), // 10 - ( 12 - 2 -2 ) = 2, meets the VM's requirement
					},
				},
			},
			wantErr: false,
		},
		{
			name: "VM without resource limits",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm-nolimits",
					Namespace: testNS,
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Memory: &kubevirtv1.Memory{Guest: resource.NewQuantity(memory1Gi, resource.BinarySI)},
							},
						},
					},
				},
			},
			nsrq: &nsrq,
			rq: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNS,
					Name:      "rq1",
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
				},
			},
			wantErr: false,
		},
		{
			name: "VM without template",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm-notemplate",
					Namespace: testNS,
				},
			},
			nsrq: &nsrq,
			rq: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNS,
					Name:      "rq1",
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCalculator(
				nil,
				nil,
				nil,
				nil,
				nil)

			err := c.containsEnoughResourceQuotaToStartVM(tt.vm, tt.nsrq, tt.rq)
			assert.Equalf(t, tt.wantErr, err != nil, "case %s containsEnoughResourceQuotaToStartVM(%v) failed", tt.name, tt.vm.Name)
		})
	}
}

// when setting `additional-guest-memory-overhead-ratio` changes, the VM might reach the limit and can't start
// tune those two parameters accordingly
func Test_vmValidator_containsEnoughResourceQuotaToStartVM_WithChangedOverheadRatio(t *testing.T) {
	generateRQ := func(cpu, mem int64) kubevirtv1.ResourceRequirements {
		return kubevirtv1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: *resource.NewQuantity(mem*memory1Gi, resource.DecimalSI),
				corev1.ResourceCPU:    *resource.NewQuantity(cpu, resource.DecimalSI),
			},
		}
	}

	nsrq := v3.NamespaceResourceQuota{
		Limit: v3.ResourceQuotaLimit{
			LimitsCPU:    "5000m",
			LimitsMemory: "5120Mi", // 5Gi
		},
	}

	tests := []struct {
		name          string
		vm            *kubevirtv1.VirtualMachine
		nsrq          *v3.NamespaceResourceQuota
		rq            *corev1.ResourceQuota
		overheadRatio string
		wantErr       bool
	}{
		{
			name: "VM can start as quota is enough with default overhead-ratio 1.5",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: testNS,
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: generateRQ(4, 4), // 4core, 4Gi
							},
						},
					},
				},
			},
			nsrq: &nsrq,
			rq: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNS,
					Name:      "rq1",
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota: "{\"limit\":{\"limitsCpu\":\"4\", \"limitsMemory\":\"5120Mi\"}}",
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(4, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.DecimalSI),
					},
				},
			},
			overheadRatio: "1.5", // default
			wantErr:       false,
		},
		{
			name: "VM can start as quota is enough with customized overhead-ratio 2.5",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: testNS,
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: generateRQ(4, 4),
							},
						},
					},
				},
			},
			nsrq: &nsrq,
			rq: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNS,
					Name:      "rq1",
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota: "{\"limit\":{\"limitsCpu\":\"4\", \"limitsMemory\":\"5120Mi\"}}",
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(4, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.DecimalSI),
					},
				},
			},
			overheadRatio: "2.5",
			wantErr:       false,
		},
		{
			name: "VM can't start as quota is not enough with customized overhead-ratio 5.0",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: testNS,
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: generateRQ(4, 4),
							},
						},
					},
				},
			},
			nsrq: &nsrq,
			rq: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNS,
					Name:      "rq1",
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota: "{\"limit\":{\"limitsCpu\":\"4\", \"limitsMemory\":\"5120Mi\"}}",
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(4, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.DecimalSI),
					},
				},
			},
			overheadRatio: "5.0",
			wantErr:       true,
		},
		{
			name: "VM can start as quota is enough with customized overhead-ratio 5.0",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm1",
					Namespace: testNS,
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: generateRQ(4, 3), // RQ does not change, but VM requests less memory
							},
						},
					},
				},
			},
			nsrq: &nsrq,
			rq: &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNS,
					Name:      "rq1",
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota: "{\"limit\":{\"limitsCpu\":\"4\", \"limitsMemory\":\"5120Mi\"}}",
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(4, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.DecimalSI),
					},
				},
			},
			overheadRatio: "5.0",
			wantErr:       false,
		},
	}
	for _, tt := range tests {
		clientset := fake.NewSimpleClientset()
		overheadRatio := &harvesterv1.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name: "additional-guest-memory-overhead-ratio",
			},
			Value: tt.overheadRatio,
		}
		err := clientset.Tracker().Add(overheadRatio)
		assert.Nil(t, err, "mock resource should add into fake controller tracker")
		settingCache := fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings)
		t.Run(tt.name, func(t *testing.T) {
			c := NewCalculator(
				nil,
				nil,
				nil,
				nil,
				settingCache)
			err := c.containsEnoughResourceQuotaToStartVM(tt.vm, tt.nsrq, tt.rq)
			assert.Equalf(t, tt.wantErr, err != nil, "case %s Test_vmValidator_containsEnoughResourceQuotaToStartVM_WithChangedOverheadRatio(%v) failed", tt.name, tt.vm.Name)
		})
	}
}

func Test_vmValidator_checkIfVMCanStartByResourceQuota_SkipsDeletingVM(t *testing.T) {
	now := metav1.Now()
	runStrategy := kubevirtv1.RunStrategyRerunOnFailure
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "vm-deleting",
			Namespace:         namespaceBothExist,
			DeletionTimestamp: &now,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			RunStrategy: &runStrategy,
		},
		Status: kubevirtv1.VirtualMachineStatus{
			PrintableStatus: kubevirtv1.VirtualMachineStatusStopped,
		},
	}

	// All caches are nil; the deletion guard must return before accessing them.
	c := NewCalculator(nil, nil, nil, nil, nil)
	assert.NoError(t, c.CheckIfVMCanStartByResourceQuota(vm))
}

// noPodCache returns no pods for index lookups.
type noPodCache struct {
	fakeclients.PodCache
}

func (noPodCache) GetByIndex(_, _ string) ([]*corev1.Pod, error) {
	return nil, nil
}

// VM with only domain.memory.guest configured, a non-empty printableStatus, a non-Halted runStrategy, and no
// virt-launcher pod in a namespace with a Rancher resource quota.
func Test_vmValidator_checkIfVMCanStartByResourceQuota_VMWithoutLimits(t *testing.T) {
	const ns = "repro-quota"

	coreclientset := corefake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
				Annotations: map[string]string{
					util.CattleAnnotationResourceQuota: `{"limit":{"limitsCpu":"5000m","limitsMemory":"10240Mi"}}`,
				},
			},
		},
		&corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default-repro",
				Namespace: ns,
				Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourceLimitsCPU:    resource.MustParse("5"),
					corev1.ResourceLimitsMemory: resource.MustParse("10Gi"),
				},
			},
		},
	)

	runStrategy := kubevirtv1.RunStrategyManual
	guestMemory := resource.MustParse("1Gi")
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "repro-nolimits",
			Namespace: ns,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			RunStrategy: &runStrategy,
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						CPU:    &kubevirtv1.CPU{Cores: 1},
						Memory: &kubevirtv1.Memory{Guest: &guestMemory},
					},
				},
			},
		},
		Status: kubevirtv1.VirtualMachineStatus{
			PrintableStatus: kubevirtv1.VirtualMachineStatusStopped,
		},
	}

	c := NewCalculator(
		fakeclients.NamespaceCache(coreclientset.CoreV1().Namespaces),
		noPodCache{},
		fakeclients.ResourceQuotaCache(coreclientset.CoreV1().ResourceQuotas),
		nil,
		nil)

	assert.NotPanics(t, func() {
		assert.NoError(t, c.CheckIfVMCanStartByResourceQuota(vm))
	})
}

// Only resources with an explicit limit are checked, an absent limit must not
// be treated as a zero-valued requirement, and must not be counted as overhead.
func Test_vmValidator_containsEnoughResourceQuotaToStartVM_PartialLimits(t *testing.T) {
	newVM := func(limits corev1.ResourceList) *kubevirtv1.VirtualMachine {
		guestMemory := resource.MustParse("1Gi")
		return &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm-partial", Namespace: testNS},
			Spec: kubevirtv1.VirtualMachineSpec{
				Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							CPU:       &kubevirtv1.CPU{Cores: 1},
							Memory:    &kubevirtv1.Memory{Guest: &guestMemory},
							Resources: kubevirtv1.ResourceRequirements{Limits: limits},
						},
					},
				},
			},
		}
	}
	nsrq := &v3.NamespaceResourceQuota{
		Limit: v3.ResourceQuotaLimit{LimitsCPU: "5000m", LimitsMemory: "10240Mi"},
	}
	newRQ := func(usedCPU, usedMem string) *corev1.ResourceQuota {
		return &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Namespace: testNS, Name: "rq1"},
			Status: corev1.ResourceQuotaStatus{
				Used: corev1.ResourceList{
					corev1.ResourceLimitsCPU:    resource.MustParse(usedCPU),
					corev1.ResourceLimitsMemory: resource.MustParse(usedMem),
				},
			},
		}
	}
	cpuOnly := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}
	memOnly := corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")}
	exhausted := newRQ("6", "11Gi") // both dimensions already over the quota

	tests := []struct {
		name    string
		limits  corev1.ResourceList
		rq      *corev1.ResourceQuota
		wantErr bool
	}{
		{
			name:    "no limits, quota exceeded: skip check",
			limits:  nil,
			rq:      exhausted,
			wantErr: false,
		},
		{
			name:    "empty limits, quota exceeded: skip check",
			limits:  corev1.ResourceList{},
			rq:      exhausted,
			wantErr: false,
		},
		{
			name:    "unrelated resource limit: skip check",
			limits:  corev1.ResourceList{"example.com/gpu": resource.MustParse("1")},
			rq:      exhausted,
			wantErr: false,
		},
		{
			name:    "CPU limit only, memory quota exceeded",
			limits:  cpuOnly,
			rq:      newRQ("0", "11Gi"),
			wantErr: false,
		},
		{
			name:    "CPU limit only, CPU quota exceeded",
			limits:  cpuOnly,
			rq:      newRQ("5", "0"),
			wantErr: true,
		},
		{
			name:    "memory limit only, CPU quota exceeded",
			limits:  memOnly,
			rq:      newRQ("6", "0"),
			wantErr: false,
		},
		{
			name:    "memory limit only, memory quota exceeded",
			limits:  memOnly,
			rq:      newRQ("0", "10Gi"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCalculator(nil, nil, nil, nil, nil)
			err := c.containsEnoughResourceQuotaToStartVM(newVM(tt.limits), nsrq, tt.rq)
			assert.Equalf(t, tt.wantErr, err != nil, "unexpected result: %v", err)
		})
	}
}
