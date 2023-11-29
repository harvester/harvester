package migration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakecore "k8s.io/client-go/kubernetes/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"

	fakegenerated "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	ctlharvcorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	ctlvirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const (
	resourceQuotaNamespace = "rs"
	resourceQuotaName
	uid

	errMockTrackerAdd = "mock resource should add into fake controller tracker"
	errMockTrackerGet = "mock resource should get into fake controller tracker"
)

func TestHandler_OnVmimChanged_WithResourceQuota(t *testing.T) {

	type fields struct {
		rqs      ctlharvcorev1.ResourceQuotaClient
		rqCache  ctlharvcorev1.ResourceQuotaCache
		vmiCache ctlvirtv1.VirtualMachineInstanceCache
		vmCache  ctlvirtv1.VirtualMachineCache
	}
	type args struct {
		rq   *corev1.ResourceQuota
		vmi  *kubevirtv1.VirtualMachineInstance
		vmim *kubevirtv1.VirtualMachineInstanceMigration
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
		want    *corev1.ResourceQuota
	}{
		{
			name:   "In Pending",
			fields: fields{},
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
							corev1.ResourceLimitsMemory: *resource.NewQuantity(1297436672, resource.BinarySI),
						},
					},
				},
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vm1",
						Namespace: resourceQuotaNamespace,
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							Resources: kubevirtv1.ResourceRequirements{
								Limits: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
									corev1.ResourceMemory: *resource.NewQuantity(1073741824, resource.BinarySI),
								},
							},
						},
					},
				},
				vmim: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vmim1",
						Namespace: resourceQuotaNamespace,
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
						util.AnnotationMigratingPrefix + "vm1": `{"limits.cpu":"1","limits.memory":"1266Mi"}`,
					},
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(2, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(2624933888, resource.BinarySI),
					},
				},
			},
		},
		{
			name:   "Succeeded", // Equivalent to Failed
			fields: fields{},
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Annotations: map[string]string{
							util.AnnotationMigratingPrefix + "vm1": `{"limits.cpu":"1","limits.memory":"1364Mi"}`,
						},
						Labels: map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(2, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(2727694336, resource.BinarySI),
						},
					},
				},
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vm1",
						Namespace: resourceQuotaNamespace,
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{
						Domain: kubevirtv1.DomainSpec{
							Resources: kubevirtv1.ResourceRequirements{
								Limits: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU:    *resource.NewQuantity(1, resource.DecimalSI),
									corev1.ResourceMemory: *resource.NewQuantity(1073741824, resource.BinarySI),
								},
							},
						},
					},
				},
				vmim: &kubevirtv1.VirtualMachineInstanceMigration{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vmim1",
						Namespace: resourceQuotaNamespace,
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
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(1, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(1297436672, resource.BinarySI),
					},
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
