package setting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_validatePersistentReservationEnabled(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "empty value", value: "", wantErr: false},
		{name: "true", value: "true", wantErr: false},
		{name: "false", value: "false", wantErr: false},
		{name: "invalid value", value: "enabled", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setting := &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.PersistentReservationEnabledSettingName},
				Default:    "false",
				Value:      tc.value,
			}
			err := validatePersistentReservationEnabled(setting)
			assert.Equal(t, tc.wantErr, err != nil, err)
		})
	}
}

func Test_settingValidator_validateUpdatePersistentReservationEnabled(t *testing.T) {
	newVMI := func(name string, reservation bool) *kubevirtv1.VirtualMachineInstance {
		return &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name},
			Spec: kubevirtv1.VirtualMachineInstanceSpec{
				Domain: kubevirtv1.DomainSpec{
					Devices: kubevirtv1.Devices{
						Disks: []kubevirtv1.Disk{
							{
								Name: "shared",
								DiskDevice: kubevirtv1.DiskDevice{
									LUN: &kubevirtv1.LunTarget{Bus: kubevirtv1.DiskBusSCSI, Reservation: reservation},
								},
							},
						},
					},
				},
			},
		}
	}
	newSetting := func(value string) *v1beta1.Setting {
		return &v1beta1.Setting{
			ObjectMeta: metav1.ObjectMeta{Name: settings.PersistentReservationEnabledSettingName},
			Default:    "false",
			Value:      value,
		}
	}

	tests := []struct {
		name     string
		vmis     []*kubevirtv1.VirtualMachineInstance
		oldValue string
		newValue string
		errMsg   string
	}{
		{
			name:     "enable",
			oldValue: "",
			newValue: "true",
		},
		{
			name:     "disable without VMs using reservation",
			vmis:     []*kubevirtv1.VirtualMachineInstance{newVMI("plain-lun", false)},
			oldValue: "true",
			newValue: "false",
		},
		{
			name:     "reset to default without VMs using reservation",
			oldValue: "true",
			newValue: "",
		},
		{
			name:     "disable while a VM uses reservation",
			vmis:     []*kubevirtv1.VirtualMachineInstance{newVMI("plain-lun", false), newVMI("wsfc-node1", true)},
			oldValue: "true",
			newValue: "false",
			errMsg:   "default/wsfc-node1",
		},
		{
			name:     "reset to default while a VM uses reservation",
			vmis:     []*kubevirtv1.VirtualMachineInstance{newVMI("wsfc-node1", true)},
			oldValue: "true",
			newValue: "",
			errMsg:   "default/wsfc-node1",
		},
		{
			name:     "keep enabled while a VM uses reservation",
			vmis:     []*kubevirtv1.VirtualMachineInstance{newVMI("wsfc-node1", true)},
			oldValue: "true",
			newValue: "true",
		},
		{
			name:     "invalid value",
			oldValue: "true",
			newValue: "off-ish",
			errMsg:   "must be true or false",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			for _, vmi := range tc.vmis {
				assert.NoError(t, clientset.Tracker().Add(vmi))
			}
			v := &settingValidator{
				vmiCache: fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances),
			}

			err := v.validateUpdatePersistentReservationEnabled(nil, newSetting(tc.oldValue), newSetting(tc.newValue))
			if tc.errMsg == "" {
				assert.NoError(t, err)
				return
			}
			assert.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), tc.errMsg), err.Error())
		})
	}
}
