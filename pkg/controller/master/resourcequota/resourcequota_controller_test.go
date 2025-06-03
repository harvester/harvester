//go:build linux && amd64

package resourcequota

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/util"
)

const (
	resourceQuotaNamespace = "test"
	resourceQuotaName      = "rq1"
	uid                    = "8afcf4d9-b8a7-464a-a4e9-abe81fc7eacf"

	memory1Gi = 1 * 1024 * 1024 * 1024
	cpuCore1  = 1
	cpuCore2  = 2
	cpuCore3  = 3
)

func TestHandler_OnResourceQuotaChanged(t *testing.T) {

	type args struct {
		rq             *corev1.ResourceQuota
		nsrqAnnotation string // util.CattleAnnotationResourceQuota on namespace object
	}

	tests := []struct {
		name    string
		args    args
		wantErr bool
		want    *corev1.ResourceQuota
	}{
		{
			name: "ResourceQuota has all zero CPU and memory limits value, skip scalling",
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
				nsrqAnnotation: "",
			},
			wantErr: true,
		},
		{
			name: "ResourceQuota has no or empty CattleAnnotationResourceQuota on namespace annotation, skip scalling",
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
				nsrqAnnotation: "",
			},
			wantErr: true,
		},
		{
			name: "ResourceQuota has invalid CattleAnnotationResourceQuota on namespace annotation, skip scalling",
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
				nsrqAnnotation: "{\"limit\":{\"limitsCpu\":\"10000m invalid\"}}", // invalid value
			},
			wantErr: true,
		},
		{
			name: "ResourceQuota has invalid invalid Harvester VM migration annotation, skip scalling",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.GenerateAnnotationKeyMigratingVMUID(uid): `{"limits.cpu":"1","limits.memory":"2Gi invalid"}`, // invalid value
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.DecimalSI),
						},
					},
				},
				nsrqAnnotation: "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10240Mi\"}}",
			},
			wantErr: true,
		},
		{
			name: "ResourceQuota has no Harvester VM migration annotation, ResourceQuota is based on CattleAnnotationResourceQuota annotation",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace:   resourceQuotaNamespace,
						Name:        resourceQuotaName,
						Labels:      map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.DecimalSI),
						},
					},
				},
				nsrqAnnotation: "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10240Mi\"}}",
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace:   resourceQuotaNamespace,
					Name:        resourceQuotaName,
					Labels:      map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{},
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
			name: "ResourceQuota is scaled up per Harvester VM migration annotation",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.GenerateAnnotationKeyMigratingVMUID(uid): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.BinarySI),
						},
					},
				},
				nsrqAnnotation: "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10Gi\"}}",
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.GenerateAnnotationKeyMigratingVMUID(uid): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore2, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(12*memory1Gi, resource.BinarySI), // scaled up
					},
				},
			},
		},
		{
			name: "ResourceQuota is scaled up only CPU limits per Harvester VM migration annotation",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.GenerateAnnotationKeyMigratingVMUID(uid): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU: *resource.NewQuantity(cpuCore1, resource.DecimalSI),
						},
					},
				},
				nsrqAnnotation: "{\"limit\":{\"limitsCpu\":\"1\"}}",
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.GenerateAnnotationKeyMigratingVMUID(uid): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU: *resource.NewQuantity(cpuCore2, resource.DecimalSI), // scaled up
					},
				},
			},
		},
		{
			name: "ResourceQuota is scaled up only memory limits per Harvester VM migration annotation",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.GenerateAnnotationKeyMigratingVMUID(uid): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.BinarySI),
						},
					},
				},
				nsrqAnnotation: "{\"limit\":{\"limitsMemory\":\"10Gi\"}}",
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.GenerateAnnotationKeyMigratingVMUID(uid): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsMemory: *resource.NewQuantity(12*memory1Gi, resource.BinarySI), // scaled up
					},
				},
			},
		},
		{
			name: "ResourceQuota is further scaled up only memory limits per Harvester VM migration annotation",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.GenerateAnnotationKeyMigratingVMName("vm1"): `{"limits.cpu":"1","limits.memory":"2Gi"}`, // name based key is still working
							util.GenerateAnnotationKeyMigratingVMName("vm2"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore2, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(12*memory1Gi, resource.BinarySI), // from 10 -> 12
						},
					},
				},
				nsrqAnnotation: "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10Gi\"}}",
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.GenerateAnnotationKeyMigratingVMName("vm1"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						util.GenerateAnnotationKeyMigratingVMName("vm2"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore3, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(14*memory1Gi, resource.BinarySI), // from 10 -> 14
					},
				},
			},
		},
		{
			name: "ResourceQuota is scaled down per Harvester VM migration annotation",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace:   resourceQuotaNamespace,
						Name:        resourceQuotaName,
						Labels:      map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							// util.AnnotationMigratingNamePrefix + "vm1": `{"limits.cpu":"1","limits.memory":"2Gi"}`, // vm1 finished the migration
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(12*memory1Gi, resource.BinarySI), // scaled
						},
					},
				},
				nsrqAnnotation: "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10Gi\"}}",
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace:   resourceQuotaNamespace,
					Name:        resourceQuotaName,
					Labels:      map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.BinarySI), // 12 -> 10
					},
				},
			},
		},
		{
			name: "ResourceQuota scaling is finished per Harvester VM migration annotation",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.GenerateAnnotationKeyMigratingVMName("vm1"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
							util.GenerateAnnotationKeyMigratingVMName("vm2"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore3, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(14*memory1Gi, resource.BinarySI), // already 10 -> 14
						},
					},
				},
				nsrqAnnotation: "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"10Gi\"}}",
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.GenerateAnnotationKeyMigratingVMName("vm1"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						util.GenerateAnnotationKeyMigratingVMName("vm2"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore3, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(14*memory1Gi, resource.BinarySI), // no change
					},
				},
			},
		},
		{
			name: "Rancher resets ResourceQuota per namespace annotation, Harvester scales it up accordingly",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.GenerateAnnotationKeyMigratingVMName("vm1"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
							util.GenerateAnnotationKeyMigratingVMName("vm2"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(20*memory1Gi, resource.BinarySI), // Rancher resets the base from 10Gi to 20Gi
						},
					},
				},
				nsrqAnnotation: "{\"limit\":{\"limitsCpu\":\"1\", \"limitsMemory\":\"20Gi\"}}", // Rancher resets ResourceQuota via this annotation on namespace
			},
			wantErr: false,
			want: &corev1.ResourceQuota{
				ObjectMeta: v1.ObjectMeta{
					Namespace: resourceQuotaNamespace,
					Name:      resourceQuotaName,
					Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
					Annotations: map[string]string{
						util.GenerateAnnotationKeyMigratingVMName("vm1"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						util.GenerateAnnotationKeyMigratingVMName("vm2"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore3, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(24*memory1Gi, resource.BinarySI), // 20 -> 24, Harvester scales up additional 4Gi
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
			_, err := scaleResourceQuotaOnDemand(tt.args.rq, tt.args.nsrqAnnotation)
			if (err != nil) != tt.wantErr {
				t.Errorf("scaleResourceQuotaOnDemand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			assert.Equal(t, tt.want, tt.args.rq, "case %q", tt.name)
		})
	}
}
