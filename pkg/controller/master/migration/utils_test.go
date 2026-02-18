//go:build linux && amd64

package migration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

func Test_IsVmiResetHarvesterMigrationAnnotationRequired(t *testing.T) {
	type args struct {
		vmi *kubevirtv1.VirtualMachineInstance
	}

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "vmi migration is ongoing",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vm1",
						Namespace: resourceQuotaNamespace,
						UID:       uid,
						Annotations: map[string]string{
							util.AnnotationMigrationUID:   vmimUID,
							util.AnnotationMigrationState: StatePending,
						},
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						MigrationState: &kubevirtv1.VirtualMachineInstanceMigrationState{
							Completed:    false,
							MigrationUID: vmimUID,
						},
					},
				},
			},
			want: false,
		},
		{
			name: "vmi migration is completed, require clean",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vm1",
						Namespace: resourceQuotaNamespace,
						UID:       uid,
						Annotations: map[string]string{
							util.AnnotationMigrationUID:   vmimUID,
							util.AnnotationMigrationState: StatePending,
						},
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						MigrationState: &kubevirtv1.VirtualMachineInstanceMigrationState{
							Completed:    true,
							MigrationUID: vmimUID,
						},
					},
				},
			},
			want: true,
		},
		{
			name: "vmi migration is completed, require clean",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:      "vm1",
						Namespace: resourceQuotaNamespace,
						UID:       uid,
						Annotations: map[string]string{
							util.AnnotationMigrationUID:   vmimUID,
							util.AnnotationMigrationState: StatePending,
						},
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						MigrationState: &kubevirtv1.VirtualMachineInstanceMigrationState{
							Failed:       true,
							MigrationUID: vmimUID,
						},
					},
				},
			},
			want: true,
		},
		{
			name: "vmi migration is completed, already cleaned",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:        "vm1",
						Namespace:   resourceQuotaNamespace,
						UID:         uid,
						Annotations: map[string]string{
							// util.AnnotationMigrationUID:   vmimUID,
							// util.AnnotationMigrationState: StatePending,
						},
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						MigrationState: &kubevirtv1.VirtualMachineInstanceMigrationState{
							Completed:    true,
							MigrationUID: vmimUID,
						},
					},
				},
			},
			want: false,
		},
		{
			name: "vmi migration is failed, already cleaned",
			args: args{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: v1.ObjectMeta{
						Name:        "vm1",
						Namespace:   resourceQuotaNamespace,
						UID:         uid,
						Annotations: map[string]string{
							// util.AnnotationMigrationUID:   vmimUID,
							// util.AnnotationMigrationState: StatePending,
						},
					},
					Status: kubevirtv1.VirtualMachineInstanceStatus{
						MigrationState: &kubevirtv1.VirtualMachineInstanceMigrationState{
							Failed:       true,
							MigrationUID: vmimUID,
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetRequired := IsVmiResetHarvesterMigrationAnnotationRequired(tt.args.vmi)
			assert.Equal(t, tt.want, resetRequired, "case %q", tt.name)
		})
	}
}
