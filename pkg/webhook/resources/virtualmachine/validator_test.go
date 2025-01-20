package virtualmachine

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

func TestCheckMaintenanceModeStrategyIsValid(t *testing.T) {
	var testCases = []struct {
		name        string
		expectError bool
		oldVM       *kubevirtv1.VirtualMachine
		newVM       *kubevirtv1.VirtualMachine
	}{
		{
			name:        "reject new VM if maintenance mode strategy label is not set",
			expectError: true,
			oldVM:       nil,
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
		{
			name:        "reject new VM if maintenance mode strategy label is set to invalid value",
			expectError: true,
			oldVM:       nil,
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: "foobar",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
		{
			name:        "reject update to VM if maintenance mode strategy label is invalid for new VM",
			expectError: true,
			oldVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: util.MaintainModeStrategyMigrate,
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: "foobar",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
		{
			// This case is crucial, so Harvester can still operate existing VMs with
			// bogus maintenance-mode strategies (i.e. update their status, shut them
			// down, etc.)
			name:        "accept update to VM if maintenance mode strategy label was invalid on old VM",
			expectError: false,
			oldVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: "foobar",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: "foobar",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
		{
			// This case ensures that IF the maintenance-mode label is updated, it is
			// updated with a valid value
			name:        "reject update to VM with invalid maintenance-mode strategy label, even if maintenance mode strategy label was invalid on old VM",
			expectError: true,
			oldVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: "foobar",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: "barfoo",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
		{
			name:        "accept new VM maintenance mode strategy label is set to valid value",
			expectError: false,
			oldVM:       nil,
			newVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: util.MaintainModeStrategyMigrate,
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		err := checkMaintenanceModeStrategyIsValid(tc.newVM, tc.oldVM)
		if tc.expectError {
			assert.NotNil(t, err, tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}
