package backup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func TestResolveRunStrategyFromRestoreAndBackup(t *testing.T) {
	tests := []struct {
		name     string
		restore  *harvesterv1.VirtualMachineRestore
		backup   *harvesterv1.VirtualMachineBackup
		expected kubevirtv1.VirtualMachineRunStrategy
	}{
		{
			name: "halt after restore overrides source run strategy",
			restore: &harvesterv1.VirtualMachineRestore{
				Spec: harvesterv1.VirtualMachineRestoreSpec{HaltAfterRestore: true},
			},
			backup: &harvesterv1.VirtualMachineBackup{
				Status: harvesterv1.VirtualMachineBackupStatus{
					SourceSpec: &harvesterv1.VirtualMachineSourceSpec{Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: ptr.To(kubevirtv1.RunStrategyManual)}},
				},
			},
			expected: kubevirtv1.RunStrategyHalted,
		},
		{
			name: "uses source run strategy manual when provided",
			restore: &harvesterv1.VirtualMachineRestore{
				Spec: harvesterv1.VirtualMachineRestoreSpec{HaltAfterRestore: false},
			},
			backup: &harvesterv1.VirtualMachineBackup{
				Status: harvesterv1.VirtualMachineBackupStatus{
					SourceSpec: &harvesterv1.VirtualMachineSourceSpec{Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: ptr.To(kubevirtv1.RunStrategyManual)}},
				},
			},
			expected: kubevirtv1.RunStrategyManual,
		},
		{
			name: "uses source run strategy always when provided",
			restore: &harvesterv1.VirtualMachineRestore{
				Spec: harvesterv1.VirtualMachineRestoreSpec{HaltAfterRestore: false},
			},
			backup: &harvesterv1.VirtualMachineBackup{
				Status: harvesterv1.VirtualMachineBackupStatus{
					SourceSpec: &harvesterv1.VirtualMachineSourceSpec{Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: ptr.To(kubevirtv1.RunStrategyAlways)}},
				},
			},
			expected: kubevirtv1.RunStrategyAlways,
		},
		{
			name: "uses source run strategy halted when provided",
			restore: &harvesterv1.VirtualMachineRestore{
				Spec: harvesterv1.VirtualMachineRestoreSpec{HaltAfterRestore: false},
			},
			backup: &harvesterv1.VirtualMachineBackup{
				Status: harvesterv1.VirtualMachineBackupStatus{
					SourceSpec: &harvesterv1.VirtualMachineSourceSpec{Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: ptr.To(kubevirtv1.RunStrategyHalted)}},
				},
			},
			expected: kubevirtv1.RunStrategyHalted,
		},
		{
			name: "falls back to rerun on failure when source run strategy is nil",
			restore: &harvesterv1.VirtualMachineRestore{
				Spec: harvesterv1.VirtualMachineRestoreSpec{HaltAfterRestore: false},
			},
			backup: &harvesterv1.VirtualMachineBackup{
				Status: harvesterv1.VirtualMachineBackupStatus{
					SourceSpec: &harvesterv1.VirtualMachineSourceSpec{Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: nil}},
				},
			},
			expected: kubevirtv1.RunStrategyRerunOnFailure,
		},
		{
			name: "falls back to rerun on failure when backup is nil",
			restore: &harvesterv1.VirtualMachineRestore{
				Spec: harvesterv1.VirtualMachineRestoreSpec{HaltAfterRestore: false},
			},
			backup:   nil,
			expected: kubevirtv1.RunStrategyRerunOnFailure,
		},
		{
			name: "falls back to rerun on failure when source spec is nil",
			restore: &harvesterv1.VirtualMachineRestore{
				Spec: harvesterv1.VirtualMachineRestoreSpec{HaltAfterRestore: false},
			},
			backup: &harvesterv1.VirtualMachineBackup{
				Status: harvesterv1.VirtualMachineBackupStatus{SourceSpec: nil},
			},
			expected: kubevirtv1.RunStrategyRerunOnFailure,
		},
		{
			name:     "falls back to rerun on failure when restore is nil",
			restore:  nil,
			backup:   nil,
			expected: kubevirtv1.RunStrategyRerunOnFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveRunStrategyFromRestoreAndBackup(tt.restore, tt.backup)
			assert.Equal(t, tt.expected, got)
		})
	}
}
