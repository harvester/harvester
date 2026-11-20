package virtualmachinebackup

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	werror "github.com/harvester/harvester/pkg/webhook/error"
)

func TestBackupTargetOptions(t *testing.T) {
	for _, tc := range []struct{ name, field, setting, value, want string }{
		{name: "direct connection"},
		{name: "certificate", field: `,"cert":"test-ca"`, want: "backup-target.cert"},
		{name: "virtual host", field: `,"virtualHostedStyle":true`, want: "virtualHostedStyle=true"},
		{name: "additional CA", setting: settings.AdditionalCASettingName, value: "test-ca", want: "additional-ca"},
		{name: "HTTP proxy", setting: settings.HTTPProxySettingName, value: `{"httpProxy":"http://proxy"}`, want: "http-proxy"},
		{name: "HTTPS proxy", setting: settings.HTTPProxySettingName, value: `{"httpsProxy":"http://proxy"}`, want: "http-proxy"},
		{name: "no proxy only", setting: settings.HTTPProxySettingName, value: `{"noProxy":".svc"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := gateValidator(`{"type":"s3","bucketName":"backups","thirdPartyBackupEnabled":true` + tc.field + `}`)
			if tc.setting != "" {
				_, err := v.targets.Settings.Create(&harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{Name: tc.setting}, Value: tc.value,
				})
				require.NoError(t, err)
			}
			for _, kind := range []harvesterv1.BackupType{
				harvesterv1.Restic, harvesterv1.Kopia, harvesterv1.Backup, harvesterv1.Snapshot,
			} {
				vmb := newVMBackup("default", "test-vm", kind)
				require.NoError(t, v.Update(nil, vmb, vmb.DeepCopy()))
				err := v.Create(nil, vmb)
				if tc.want == "" || !kind.IsDataMover() {
					require.NoError(t, err, "%s", kind)
					continue
				}
				require.ErrorContains(t, err, tc.want)
				var admissionError werror.AdmitError
				require.ErrorAs(t, err, &admissionError)
				require.Equal(t, fieldTypeName, admissionError.AsResult().Details.Causes[0].Field)
			}
		})
	}
}
