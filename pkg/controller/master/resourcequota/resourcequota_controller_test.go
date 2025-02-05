//go:build linux && amd64

package resourcequota

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctlharvcorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

const (
	resourceQuotaNamespace = "test"
	resourceQuotaName      = "rq1"
	uid

	memory1Gi = 1 * 1024 * 1024 * 1024
	cpuCore1  = 1
	cpuCore2  = 2
	cpuCore3  = 3
)

func TestHandler_OnResourceQuotaChanged(t *testing.T) {

	type fields struct {
		rqs ctlharvcorev1.ResourceQuotaClient
	}
	type args struct {
		rq *corev1.ResourceQuota
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
		want    *corev1.ResourceQuota
	}{
		{
			name:   "Resourcequota has limits value zero, skip scalling ",
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
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(0, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(0, resource.BinarySI),
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name:   "No RancherNamespaceResourceQuota annotation, skip scalling ",
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
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.BinarySI),
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name:   "Invalid RancherNamespaceResourceQuota annotation, skip scalling ",
			fields: fields{},
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace:   resourceQuotaNamespace,
						Name:        resourceQuotaName,
						Labels:      map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{util.CattleAnnotationResourceQuota: "{\"limit\":{\"limitsCpu\":\"10000m invalid\"}}"},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.BinarySI),
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name:   "Invalid Harvester VM migration annotation, skip scalling",
			fields: fields{},
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.CattleAnnotationResourceQuota:     "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10240Mi\"}}",
							util.AnnotationMigratingPrefix + "vm1": `{"limits.cpu":"1","limits.memory":"2Gi invalid"}`,
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.DecimalSI),
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name:   "No pending scaling, RQ is based on Rancher annotation",
			fields: fields{},
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.CattleAnnotationResourceQuota: "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10240Mi\"}}",
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.DecimalSI),
						},
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota: "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10240Mi\"}}",
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.DecimalSI),
					},
				},
			},
		},
		{
			name:   "Scaling up per vmim resources annotations",
			fields: fields{},
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.CattleAnnotationResourceQuota:     "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10Gi\"}}",
							util.AnnotationMigratingPrefix + "vm1": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.BinarySI),
						},
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota:     "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10Gi\"}}",
						util.AnnotationMigratingPrefix + "vm1": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore2, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(12*memory1Gi, resource.BinarySI),
					},
				},
			},
		},
		{
			name:   "Scaling up further per vmim resources annotations",
			fields: fields{},
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.CattleAnnotationResourceQuota:     "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10Gi\"}}",
							util.AnnotationMigratingPrefix + "vm1": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
							util.AnnotationMigratingPrefix + "vm2": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore2, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(12*memory1Gi, resource.BinarySI),
						},
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota:     "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10Gi\"}}",
						util.AnnotationMigratingPrefix + "vm1": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						util.AnnotationMigratingPrefix + "vm2": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore3, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(14*memory1Gi, resource.BinarySI),
					},
				},
			},
		},
		{
			name:   "Scaling down per vmim resources annotations",
			fields: fields{},
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.CattleAnnotationResourceQuota: "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10Gi\"}}",
							// util.AnnotationMigratingPrefix + "vm1": `{"limits.cpu":"1","limits.memory":"2Gi"}`, // vm1 finished the migration
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(12*memory1Gi, resource.BinarySI),
						},
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota: "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10Gi\"}}",
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.BinarySI),
					},
				},
			},
		},
		{
			name:   "Scaling is finished per vmim resources annotations",
			fields: fields{},
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.CattleAnnotationResourceQuota:     "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10Gi\"}}",
							util.AnnotationMigratingPrefix + "vm1": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
							util.AnnotationMigratingPrefix + "vm2": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore3, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(14*memory1Gi, resource.BinarySI),
						},
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota:     "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10Gi\"}}",
						util.AnnotationMigratingPrefix + "vm1": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						util.AnnotationMigratingPrefix + "vm2": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore3, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(14*memory1Gi, resource.BinarySI),
					},
				},
			},
		},
		{
			name:   "Rancher resets RQ, scaling is rebased",
			fields: fields{},
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.CattleAnnotationResourceQuota:     "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"20Gi\"}}", // Rancher resets the base from 10Gi to 20Gi
							util.AnnotationMigratingPrefix + "vm1": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
							util.AnnotationMigratingPrefix + "vm2": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(20*memory1Gi, resource.BinarySI), // Rancher resets the base from 10Gi to 20Gi
						},
					},
				},
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.CattleAnnotationResourceQuota:     "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"20Gi\"}}",
						util.AnnotationMigratingPrefix + "vm1": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						util.AnnotationMigratingPrefix + "vm2": `{"limits.cpu":"1","limits.memory":"2Gi"}`,
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore3, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(24*memory1Gi, resource.BinarySI), // Harvester adds the 4Gi to new base
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if tt.args.rq == nil {
				return
			}

			_, err := scaleResourceOnDemand(tt.args.rq)
			if (err != nil) != tt.wantErr {
				t.Errorf("scaleResourceOnDemand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			assert.Equal(t, tt.want, tt.args.rq, "case %q", tt.name)
		})
	}
}
