//go:build linux && amd64

package migration

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakecore "k8s.io/client-go/kubernetes/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"
	kubevirtservices "kubevirt.io/kubevirt/pkg/virt-controller/services"

	fakegenerated "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const (
	resourceQuotaNamespace = "rs"
	resourceQuotaName      = "rq1"
	uid                    = "6afcf4d9-b8a7-464a-a4e9-abe81fc7eacd"
	vmimUID                = "8afcf4d9-b8a7-464a-a4e9-abe81fc7eacd"

	longVMIName = "a-very-long-name-exceeds-the-63-length-and-it-reports-error-in-old-version"

	memory1Gi = 1 * 1024 * 1024 * 1024

	// the vm limits.memory is dynamically computed, avoid to use hard coded number
	vmResourceLimitStr = "{\"limits.cpu\":\"1\",\"limits.memory\":\"%v\"}"

	errMockTrackerAdd = "mock resource should add into fake controller tracker"
	errMockTrackerGet = "mock resource should get into fake controller tracker"
)

func TestHandler_OnVmimChanged_WithResourceQuota(t *testing.T) {
	// compute the dynamic overhead per kubevirt
	getMemWithOverhead := func(memLimit int64) int64 {
		vmi := kubevirtv1.VirtualMachineInstance{
			ObjectMeta: v1.ObjectMeta{
				Name:      "vm1",
				Namespace: resourceQuotaNamespace,
			},
			Spec: kubevirtv1.VirtualMachineInstanceSpec{
				Domain: kubevirtv1.DomainSpec{
					Resources: kubevirtv1.ResourceRequirements{
						Limits: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceMemory: *resource.NewQuantity(memLimit, resource.BinarySI),
						},
					},
				},
			},
		}
		overhead := kubevirtservices.GetMemoryOverhead(&vmi, runtime.GOARCH, util.GetAdditionalGuestMemoryOverheadRatioWithoutError(nil)) // use default ratio 1.5
		return overhead.Value() + memLimit
	}

	type args struct {
		rq   *corev1.ResourceQuota
		vmi  *kubevirtv1.VirtualMachineInstance
		vmim *kubevirtv1.VirtualMachineInstanceMigration
	}

	tests := []struct {
		name    string
		args    args
		wantErr bool
		want    *corev1.ResourceQuota
	}{
		{
			name: "In Pending",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2, resource.BinarySI),
						},
					},
				},
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vm1",
						Namespace: resourceQuotaNamespace,
						UID:       uid,
						Annotations: map[string]string{
							util.AnnotationMigrationUID:   vmimUID, // bypass vmi migration state updating
							util.AnnotationMigrationState: StatePending,
						},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							Resources: kubevirtv1.ResourceRequirements{
								Limits: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
									corev1.ResourceMemory: *resource.NewQuantity(memory1Gi, resource.BinarySI),
								},
							},
						},
					},
				},
				vmim: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vmim1",
						Namespace: resourceQuotaNamespace,
						UID:       vmimUID,
					},
					Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{VMIName: "vm1"},
					Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtv1.MigrationPending,
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						util.GenerateAnnotationKeyMigratingVMUID(uid): fmt.Sprintf(vmResourceLimitStr, getMemWithOverhead(memory1Gi)),
					},
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(1, resource.DecimalSI), // will be updated by rq controller later
						corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2, resource.BinarySI),
					},
				},
			},
		},
		{
			name: "In Pending with a long VMI name, the UID based key is used in newer version",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2, resource.BinarySI),
						},
					},
				},
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:      longVMIName,
						Namespace: resourceQuotaNamespace,
						UID:       uid,
						Annotations: map[string]string{
							util.AnnotationMigrationUID:   vmimUID, // bypass vmi migration state updating
							util.AnnotationMigrationState: StatePending,
						},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							Resources: kubevirtv1.ResourceRequirements{
								Limits: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
									corev1.ResourceMemory: *resource.NewQuantity(memory1Gi, resource.BinarySI),
								},
							},
						},
					},
				},
				vmim: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vmim1",
						Namespace: resourceQuotaNamespace,
						UID:       vmimUID,
					},
					Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{VMIName: longVMIName},
					Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtv1.MigrationPending,
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						util.GenerateAnnotationKeyMigratingVMUID(uid): fmt.Sprintf(vmResourceLimitStr, getMemWithOverhead(memory1Gi)),
					},
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(1, resource.DecimalSI), // will be updated by rq controller later
						corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2, resource.BinarySI),
					},
				},
			},
		},
		{
			name: "Succeeded, name based annotation key is deleted from ResourceQuota",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Annotations: map[string]string{
							util.GenerateAnnotationKeyMigratingVMName("vm1"): fmt.Sprintf(vmResourceLimitStr, getMemWithOverhead(memory1Gi)),
						},
						Labels: map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(2, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2+getMemWithOverhead(memory1Gi), resource.BinarySI),
						},
					},
				},
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vm1",
						Namespace: resourceQuotaNamespace,
						Annotations: map[string]string{
							util.AnnotationMigrationUID:   vmimUID, // bypass vmi migration state updating
							util.AnnotationMigrationState: StatePending,
						},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							Resources: kubevirtv1.ResourceRequirements{
								Limits: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
									corev1.ResourceMemory: *resource.NewQuantity(memory1Gi, resource.BinarySI),
								},
							},
						},
					},
				},
				vmim: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vmim1",
						Namespace: resourceQuotaNamespace,
						UID:       vmimUID,
					},
					Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{VMIName: "vm1"},
					Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtv1.MigrationSucceeded,
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace:   resourceQuotaNamespace,
					Name:        resourceQuotaName,
					Annotations: map[string]string{},
					Labels:      map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(2, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2+getMemWithOverhead(memory1Gi), resource.BinarySI),
					},
				},
			},
		},
		{
			name: "Succeeded, UID based annotation key is deleted from ResourceQuota",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Annotations: map[string]string{
							util.GenerateAnnotationKeyMigratingVMUID(uid): fmt.Sprintf(vmResourceLimitStr, getMemWithOverhead(memory1Gi)),
						},
						Labels: map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(2, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2+getMemWithOverhead(memory1Gi), resource.BinarySI),
						},
					},
				},
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vm1",
						Namespace: resourceQuotaNamespace,
						UID:       uid,
						Annotations: map[string]string{
							util.AnnotationMigrationUID:   vmimUID, // bypass vmi migration state updating
							util.AnnotationMigrationState: StatePending,
						},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							Resources: kubevirtv1.ResourceRequirements{
								Limits: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
									corev1.ResourceMemory: *resource.NewQuantity(memory1Gi, resource.BinarySI),
								},
							},
						},
					},
				},
				vmim: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vmim1",
						Namespace: resourceQuotaNamespace,
						UID:       vmimUID,
					},
					Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{VMIName: "vm1"},
					Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtv1.MigrationSucceeded,
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace:   resourceQuotaNamespace,
					Name:        resourceQuotaName,
					Annotations: map[string]string{},
					Labels:      map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(2, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2+getMemWithOverhead(memory1Gi), resource.BinarySI),
					},
				},
			},
		},
		{
			name: "RQ is not in the target namespace, skip scaling",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "random",
						Name:      resourceQuotaName,
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2, resource.BinarySI),
						},
					},
				},
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vm1",
						Namespace: resourceQuotaNamespace,
						Annotations: map[string]string{
							util.AnnotationMigrationUID:   vmimUID, // bypass vmi migration state updating
							util.AnnotationMigrationState: StatePending,
						},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							Resources: kubevirtv1.ResourceRequirements{
								Limits: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
									corev1.ResourceMemory: *resource.NewQuantity(memory1Gi, resource.BinarySI),
								},
							},
						},
					},
				},
				vmim: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vmim1",
						Namespace: resourceQuotaNamespace,
						UID:       vmimUID,
					},
					Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{VMIName: "vm1"},
					Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtv1.MigrationPending,
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "random",
					Name:      resourceQuotaName,
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(1, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2, resource.BinarySI),
					},
				},
			},
		},
		{
			name: "RQ is not in the target namespace, skip scaling",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "random",
						Name:      resourceQuotaName,
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2, resource.BinarySI),
						},
					},
				},
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vm1",
						Namespace: resourceQuotaNamespace,
						Annotations: map[string]string{
							util.AnnotationMigrationUID:   vmimUID, // bypass vmi migration state updating
							util.AnnotationMigrationState: StatePending,
						},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							Resources: kubevirtv1.ResourceRequirements{
								Limits: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
									corev1.ResourceMemory: *resource.NewQuantity(memory1Gi, resource.BinarySI),
								},
							},
						},
					},
				},
				vmim: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vmim1",
						Namespace: resourceQuotaNamespace,
						UID:       vmimUID,
					},
					Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{VMIName: "vm1"},
					Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtv1.MigrationPending,
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "random",
					Name:      resourceQuotaName,
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(1, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2, resource.BinarySI),
					},
				},
			},
		},
		{
			name: "Not the default RQ, skip scaling",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2, resource.BinarySI),
						},
					},
				},
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vm1",
						Namespace: resourceQuotaNamespace,
						Annotations: map[string]string{
							util.AnnotationMigrationUID:   vmimUID, // bypass vmi migration state updating
							util.AnnotationMigrationState: StatePending,
						},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							Resources: kubevirtv1.ResourceRequirements{
								Limits: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
									corev1.ResourceMemory: *resource.NewQuantity(memory1Gi, resource.BinarySI),
								},
							},
						},
					},
				},
				vmim: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vmim1",
						Namespace: resourceQuotaNamespace,
						UID:       vmimUID,
					},
					Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{VMIName: "vm1"},
					Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtv1.MigrationPending,
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(1, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(memory1Gi*2, resource.BinarySI),
					},
				},
			},
		},
		{
			name: "RQ has zero quantity, skip scaling",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{},
					},
				},
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vm1",
						Namespace: resourceQuotaNamespace,
						Annotations: map[string]string{
							util.AnnotationMigrationUID:   vmimUID, // bypass vmi migration state updating
							util.AnnotationMigrationState: StatePending,
						},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							Resources: kubevirtv1.ResourceRequirements{
								Limits: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
									corev1.ResourceMemory: *resource.NewQuantity(memory1Gi, resource.BinarySI),
								},
							},
						},
					},
				},
				vmim: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vmim1",
						Namespace: resourceQuotaNamespace,
						UID:       vmimUID,
					},
					Spec: kubevirtv1.VirtualMachineInstanceMigrationSpec{VMIName: "vm1"},
					Status: kubevirtv1.VirtualMachineInstanceMigrationStatus{
						Phase: kubevirtv1.MigrationPending,
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var clientset = fakecore.NewSimpleClientset()
			var harvFakeClient = fakegenerated.NewSimpleClientset()

			if tt.args.rq != nil {
				err := clientset.Tracker().Add(tt.args.rq)
				assert.Nil(t, err, errMockTrackerAdd)
			}
			if tt.args.vmi != nil {
				err := harvFakeClient.Tracker().Add(tt.args.vmi)
				assert.Nil(t, err, errMockTrackerAdd)
			}
			if tt.args.vmim != nil {
				err := harvFakeClient.Tracker().Add(tt.args.vmim)
				assert.Nil(t, err, errMockTrackerAdd)
			}

			h := &Handler{
				rqs:      fakeclients.ResourceQuotaClient(clientset.CoreV1().ResourceQuotas),
				rqCache:  fakeclients.ResourceQuotaCache(clientset.CoreV1().ResourceQuotas),
				vmiCache: fakeclients.VirtualMachineInstanceCache(harvFakeClient.KubevirtV1().VirtualMachineInstances),
			}

			_, err := h.OnVmimChanged("", tt.args.vmim)
			if (err != nil) != tt.wantErr {
				t.Errorf("OnVmimChanged() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			rq, err := clientset.Tracker().Get(corev1.SchemeGroupVersion.WithResource("resourcequotas"), tt.args.rq.Namespace, tt.args.rq.Name)
			assert.Nil(t, err, errMockTrackerGet)
			assert.Equal(t, tt.want, rq, "case %q", tt.name)
		})
	}
}
