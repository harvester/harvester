package resourcequota

import (
	"encoding/json"
	"testing"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corefake "k8s.io/client-go/kubernetes/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"

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
)

func Test_vmValidator_checkResourceQuota(t *testing.T) {
	var coreclientset = corefake.NewSimpleClientset()

	type fields struct {
		nsCache ctlcorev1.NamespaceCache
	}
	type args struct {
		vm *kubevirtv1.VirtualMachine
	}
	tests := []struct {
		name    string
		fields  fields
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
