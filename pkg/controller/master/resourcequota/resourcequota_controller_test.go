//go:build linux && amd64

package resourcequota

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakecore "k8s.io/client-go/kubernetes/fake"

	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const (
	resourceQuotaNamespace = "test"
	resourceQuotaName      = "rq1"
	uid                    = "8afcf4d9-b8a7-464a-a4e9-abe81fc7eacf"

	memory1Gi = 1 * 1024 * 1024 * 1024
	cpuCore1  = 1
	cpuCore2  = 2
	cpuCore3  = 3

	errMockTrackerAdd = "mock resource should add into fake controller tracker"
	errMockTrackerGet = "mock resource should get into fake controller tracker"
)

// this test only covers the function scaleResourceQuotaOnDemand which handles the errors while processing quota related annotations
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
			name: "ResourceQuota has all zero CPU and memory limits value, skip scaling",
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
			name: "ResourceQuota has no or empty CattleAnnotationResourceQuota on namespace annotation, skip scaling",
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
			name: "ResourceQuota has invalid CattleAnnotationResourceQuota on namespace annotation, skip scaling",
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
			name: "ResourceQuota has invalid Harvester VM migration annotation, skip scaling",
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

// this test covers the full process of OnResourceQuotaChanged by controller
func TestHandler_OnResourceQuotaChanged_ByController(t *testing.T) {

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
			name: "ResourceQuota is scaled up per Harvester VM migration annotation, flag is cleared",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.GenerateAnnotationKeyMigratingVMUID(uid): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
							util.AnnotationMigratingScalingResyncNeeded:   "true",
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
						util.AnnotationMigratingScalingResyncNeeded:   "false",
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
			name: "ResourceQuota is scaled up per Harvester VM migration annotation, no flag is set by legacy controller",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.GenerateAnnotationKeyMigratingVMUID(uid): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
							// util.AnnotationMigratingScalingResyncNeeded:   "true", // legacy controller did not set this flag
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
						// util.AnnotationMigratingScalingResyncNeeded:   "false", // without this annotation also means sync is done
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
			name: "ResourceQuota has sync requests per Harvester VM migration annotation, but no real changes, the flag is cleared when it is set",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.GenerateAnnotationKeyMigratingVMUID(uid): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
							util.AnnotationMigratingScalingResyncNeeded:   "true", // flag is set, but no real changes
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore1, resource.DecimalSI),
							corev1.ResourceLimitsMemory: *resource.NewQuantity(12*memory1Gi, resource.BinarySI), // already scaled
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
						util.AnnotationMigratingScalingResyncNeeded:   "false", // flag is cleared
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
			name: "ResourceQuota is processed, no quota annotation change , no sync flag; then no change",
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
							corev1.ResourceLimitsMemory: *resource.NewQuantity(12*memory1Gi, resource.BinarySI), // already scaled
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
						corev1.ResourceLimitsMemory: *resource.NewQuantity(12*memory1Gi, resource.BinarySI),
					},
				},
			},
		},
		{
			name: "ResourceQuota auto scaling is disabled",
			args: args{
				rq: &corev1.ResourceQuota{
					ObjectMeta: v1.ObjectMeta{
						Namespace: resourceQuotaNamespace,
						Name:      resourceQuotaName,
						Labels:    map[string]string{util.LabelManagementDefaultResourceQuota: "true"},
						Annotations: map[string]string{
							util.GenerateAnnotationKeyMigratingVMName("vm1"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
							util.GenerateAnnotationKeyMigratingVMName("vm2"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
							util.AnnotationMigratingScalingResyncNeeded:      "true",
							util.AnnotationSkipResourceQuotaAutoScaling:      "true",
						},
					},
					Spec: corev1.ResourceQuotaSpec{
						Hard: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore3, resource.DecimalSI),
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
						util.GenerateAnnotationKeyMigratingVMName("vm1"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						util.GenerateAnnotationKeyMigratingVMName("vm2"): `{"limits.cpu":"1","limits.memory":"2Gi"}`,
						util.AnnotationMigratingScalingResyncNeeded:      "true",
						util.AnnotationSkipResourceQuotaAutoScaling:      "true",
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceLimitsCPU:    *resource.NewQuantity(cpuCore3, resource.DecimalSI),
						corev1.ResourceLimitsMemory: *resource.NewQuantity(10*memory1Gi, resource.BinarySI), // no change
					},
				},
			},
		},
	}

	getNamespace := func(nrqAnnotation string) *corev1.Namespace {
		return &corev1.Namespace{
			ObjectMeta: v1.ObjectMeta{
				Name: resourceQuotaNamespace,
				Annotations: map[string]string{
					util.CattleAnnotationResourceQuota: nrqAnnotation,
				},
			},
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.rq == nil {
				return
			}

			var clientset = fakecore.NewSimpleClientset()
			if tt.args.rq != nil {
				err := clientset.Tracker().Add(tt.args.rq)
				assert.Nil(t, err, errMockTrackerAdd)

				err = clientset.Tracker().Add(getNamespace(tt.args.nsrqAnnotation))
				assert.Nil(t, err, errMockTrackerAdd)
			}

			h := &Handler{
				nsCache: fakeclients.NamespaceCache(clientset.CoreV1().Namespaces),
				rqs:     fakeclients.ResourceQuotaClient(clientset.CoreV1().ResourceQuotas),
				rqCache: fakeclients.ResourceQuotaCache(clientset.CoreV1().ResourceQuotas),
			}
			_, err := h.OnResourceQuotaChanged(tt.args.rq.Name, tt.args.rq)
			if (err != nil) != tt.wantErr {
				t.Errorf("OnResourceQuotaChanged() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			rq, err := h.rqCache.Get(tt.args.rq.Namespace, tt.args.rq.Name)
			assert.Nil(t, err, errMockTrackerGet)
			assert.Equal(t, tt.want, rq, "case %q", tt.name)
		})
	}
}
