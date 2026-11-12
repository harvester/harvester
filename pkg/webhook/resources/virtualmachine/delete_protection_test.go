package virtualmachine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	vmutil "github.com/harvester/harvester/pkg/util/virtualmachine"
	werror "github.com/harvester/harvester/pkg/webhook/error"
)

const (
	deleteProtectionLabelOp = `{"op": "add", "path": "/metadata/labels/kubevirt.io~1vm-delete-protection", "value": "true"}`
	emptyLabelsOp           = `{"op": "add", "path": "/metadata/labels", "value": {}}`
)

func newDeleteProtectionVM(namespace string, labels map[string]string) *kubevirtv1.VirtualMachine {
	return &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "vm-1",
			Labels:    labels,
		},
	}
}

func TestPatchDeleteProtection(t *testing.T) {
	tests := []struct {
		name         string
		settingValue string // "" = setting object absent
		vm           *kubevirtv1.VirtualMachine
		expectedOps  []string
		expectErr    bool
	}{
		{
			name:         "setting absent - no patch",
			settingValue: "",
			vm:           newDeleteProtectionVM("default", nil),
			expectedOps:  nil,
		},
		{
			name:         "setting false - no patch",
			settingValue: "false",
			vm:           newDeleteProtectionVM("default", map[string]string{"app": "web"}),
			expectedOps:  nil,
		},
		{
			name:         "setting true, VM has no labels - create map and add label",
			settingValue: "true",
			vm:           newDeleteProtectionVM("default", nil),
			expectedOps:  []string{emptyLabelsOp, deleteProtectionLabelOp},
		},
		{
			name:         "setting true, VM has other labels - add label",
			settingValue: "true",
			vm:           newDeleteProtectionVM("default", map[string]string{"app": "web"}),
			expectedOps:  []string{deleteProtectionLabelOp},
		},
		{
			name:         "setting true, explicit false on VM is respected",
			settingValue: "true",
			vm:           newDeleteProtectionVM("default", map[string]string{util.LabelVMDeleteProtection: "false"}),
			expectedOps:  nil,
		},
		{
			name:         "setting true, explicit true on VM is left alone",
			settingValue: "true",
			vm:           newDeleteProtectionVM("default", map[string]string{util.LabelVMDeleteProtection: "true"}),
			expectedOps:  nil,
		},
		{
			name:         "setting true, node driver / CAPHV VM is exempt",
			settingValue: "true",
			vm:           newDeleteProtectionVM("default", map[string]string{util.LabelVMCreator: util.VirtualMachineCreatorNodeDriver}),
			expectedOps:  nil,
		},
		{
			name:         "setting true, Harvester-created VM is exempt",
			settingValue: "true",
			vm:           newDeleteProtectionVM("default", map[string]string{util.LabelVMCreator: vmutil.CreatorHarvester}),
			expectedOps:  nil,
		},
		{
			name:         "setting true, harvester-system VM is exempt",
			settingValue: "true",
			vm:           newDeleteProtectionVM(util.HarvesterSystemNamespaceName, nil),
			expectedOps:  nil,
		},
		{
			name:         "invalid setting value - reject instead of skipping protection",
			settingValue: "true ",
			vm:           newDeleteProtectionVM("default", nil),
			expectErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			if tc.settingValue != "" {
				_, err := clientset.HarvesterhciV1beta1().Settings().Create(context.TODO(), &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{Name: settings.DefaultVMDeleteProtectionSettingName},
					Default:    "false",
					Value:      tc.settingValue,
				}, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			m := &vmMutator{setting: fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings)}
			ops, err := m.patchDeleteProtection(tc.vm, nil)
			if tc.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), settings.DefaultVMDeleteProtectionSettingName)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expectedOps, []string(ops))
		})
	}
}

func TestVMValidatorDeleteProtection(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		expectError bool
	}{
		{name: "no labels", labels: nil, expectError: false},
		{name: "label false", labels: map[string]string{util.LabelVMDeleteProtection: "false"}, expectError: false},
		{name: "label empty", labels: map[string]string{util.LabelVMDeleteProtection: ""}, expectError: false},
		{name: "label 0", labels: map[string]string{util.LabelVMDeleteProtection: "0"}, expectError: false},
		{name: "label not a bool", labels: map[string]string{util.LabelVMDeleteProtection: "yes"}, expectError: false},
		{name: "label with spaces", labels: map[string]string{util.LabelVMDeleteProtection: " true "}, expectError: false},
		// Only "true" and "True" enable protection, as in the KubeVirt SSP policy.
		{name: "label TRUE", labels: map[string]string{util.LabelVMDeleteProtection: "TRUE"}, expectError: false},
		{name: "label 1", labels: map[string]string{util.LabelVMDeleteProtection: "1"}, expectError: false},
		{name: "label t", labels: map[string]string{util.LabelVMDeleteProtection: "t"}, expectError: false},
		{name: "label true", labels: map[string]string{util.LabelVMDeleteProtection: "true"}, expectError: true},
		{name: "label True", labels: map[string]string{util.LabelVMDeleteProtection: "True"}, expectError: true},
	}

	v := &vmValidator{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Delete(nil, newDeleteProtectionVM("default", tc.labels))
			if !tc.expectError {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			admitErr, ok := err.(werror.AdmitError)
			require.True(t, ok)
			assert.Equal(t, int32(400), admitErr.AsResult().Code)
			assert.Contains(t, err.Error(), "default/vm-1 has delete protection enabled")
			assert.Contains(t, err.Error(), util.LabelVMDeleteProtection)
		})
	}
}
