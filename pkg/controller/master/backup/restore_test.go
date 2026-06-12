package backup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
)

func TestGetDefaultRunStrategy(t *testing.T) {
	tests := []struct {
		name       string
		vmr        *harvesterv1.VirtualMachineRestore
		sourceSpec *harvesterv1.VirtualMachineSourceSpec
		expected   kubevirtv1.VirtualMachineRunStrategy
	}{
		{
			name: "halt after restore overrides source run strategy",
			vmr: &harvesterv1.VirtualMachineRestore{
				Spec: harvesterv1.VirtualMachineRestoreSpec{HaltAfterRestore: true},
			},
			sourceSpec: &harvesterv1.VirtualMachineSourceSpec{Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: ptr.To(kubevirtv1.RunStrategyManual)}},
			expected:   kubevirtv1.RunStrategyHalted,
		},
		{
			name:       "uses source run strategy manual when provided",
			vmr:        &harvesterv1.VirtualMachineRestore{},
			sourceSpec: &harvesterv1.VirtualMachineSourceSpec{Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: ptr.To(kubevirtv1.RunStrategyManual)}},
			expected:   kubevirtv1.RunStrategyManual,
		},
		{
			name:       "uses source run strategy always when provided",
			vmr:        &harvesterv1.VirtualMachineRestore{},
			sourceSpec: &harvesterv1.VirtualMachineSourceSpec{Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: ptr.To(kubevirtv1.RunStrategyAlways)}},
			expected:   kubevirtv1.RunStrategyAlways,
		},
		{
			name:       "uses source run strategy halted when provided",
			vmr:        &harvesterv1.VirtualMachineRestore{},
			sourceSpec: &harvesterv1.VirtualMachineSourceSpec{Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: ptr.To(kubevirtv1.RunStrategyHalted)}},
			expected:   kubevirtv1.RunStrategyHalted,
		},
		{
			name:       "falls back to rerun on failure when source run strategy is nil",
			vmr:        &harvesterv1.VirtualMachineRestore{},
			sourceSpec: &harvesterv1.VirtualMachineSourceSpec{Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: nil}},
			expected:   kubevirtv1.RunStrategyRerunOnFailure,
		},
		{
			name:       "falls back to rerun on failure when source spec is nil",
			vmr:        &harvesterv1.VirtualMachineRestore{},
			sourceSpec: nil,
			expected:   kubevirtv1.RunStrategyRerunOnFailure,
		},
	}

	h := &RestoreHandler{
		vmro: restorecommon.NewVMRestoreOperatorBuilder().Build(),
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.getDefaultRunStrategy(tt.vmr, tt.sourceSpec)
			assert.Equal(t, tt.expected, got)
		})
	}
}
