package virtualmachinerestore

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	werror "github.com/harvester/harvester/pkg/webhook/error"
)

func TestRestoreTargetOptions(t *testing.T) {
	for _, tc := range []struct {
		name, cert, setting, value, want string
		virtual                          bool
	}{
		{name: "direct connection"},
		{name: "certificate", cert: "test-ca", want: "backup-target.cert"},
		{name: "virtual host", virtual: true, want: "virtualHostedStyle=true"},
		{name: "additional CA", setting: settings.AdditionalCASettingName, value: "test-ca", want: "additional-ca"},
		{name: "HTTP proxy", setting: settings.HTTPProxySettingName, value: `{"httpProxy":"http://proxy"}`, want: "http-proxy"},
		{name: "HTTPS proxy", setting: settings.HTTPProxySettingName, value: `{"httpsProxy":"http://proxy"}`, want: "http-proxy"},
		{name: "no proxy only", setting: settings.HTTPProxySettingName, value: `{"noProxy":".svc"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := &settings.BackupTarget{
				Type: settings.S3BackupType, BucketName: "backups", ThirdPartyBackupEnabled: true,
				Cert: tc.cert, VirtualHostedStyle: tc.virtual,
			}
			v, vmb := restoreTargetFixture(t, target)
			if tc.setting != "" {
				_, err := v.targets.Settings.Create(&harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{Name: tc.setting}, Value: tc.value,
				})
				require.NoError(t, err)
			}
			vmr := &harvesterv1.VirtualMachineRestore{}
			vmr.Spec.NewVM = true
			for _, kind := range []harvesterv1.BackupType{
				harvesterv1.Restic, harvesterv1.Kopia, harvesterv1.Backup, harvesterv1.Snapshot,
			} {
				vmb.Spec.Type = kind
				require.NoError(t, v.Update(nil, vmr, vmr.DeepCopy()))
				err := v.checkVMBackupType(vmr, vmb)
				if tc.want == "" || !kind.IsDataMover() {
					require.NoError(t, err, "%s", kind)
					continue
				}
				require.ErrorContains(t, err, tc.want)
				var admissionError werror.AdmitError
				require.ErrorAs(t, err, &admissionError)
				require.Equal(t, fieldVirtualMachineBackupName, admissionError.AsResult().Details.Causes[0].Field)
			}
		})
	}
}

func restoreTargetFixture(t *testing.T, target *settings.BackupTarget) (*restoreValidator, *harvesterv1.VirtualMachineBackup) {
	t.Helper()
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName, UID: "source", ResourceVersion: "1",
	}, Data: map[string][]byte{
		util.AWSAccessKey: []byte("access"), util.AWSSecretKey: []byte("secret"),
		backuputil.ResticPasswordKey: []byte("password"), backuputil.KopiaPasswordKey: []byte("password"),
		backuputil.RepositoryIDKey: []byte(backuputil.RepositoryID(target)),
	}}
	record, err := backuputil.RepoReadyRecord(target, secret)
	require.NoError(t, err)
	value, err := json.Marshal(target)
	require.NoError(t, err)
	setting := &harvesterv1.Setting{ObjectMeta: metav1.ObjectMeta{
		Name: settings.BackupTargetSettingName, Annotations: map[string]string{backuputil.ReadyAnnotation: record},
	}, Value: string(value)}
	harvesterv1.SettingConfigured.True(setting)
	client := fake.NewSimpleClientset(setting, secret)
	v := &restoreValidator{
		vmbr: backupcommon.NewVMBackupReader(), vmrr: restorecommon.NewVMRestoreReader(),
		setting: fakeclients.HarvesterSettingCache(client.HarvesterhciV1beta1().Settings),
		targets: backuputil.TargetClient{
			Settings: fakeclients.HarvesterSettingClient(client.HarvesterhciV1beta1().Settings),
			Secrets:  fakeclients.SecretClient(client.CoreV1().Secrets),
		},
	}
	vmb := &harvesterv1.VirtualMachineBackup{
		Status: harvesterv1.VirtualMachineBackupStatus{BackupTarget: backuputil.BackupTargetStatus(target)},
	}
	return v, vmb
}
