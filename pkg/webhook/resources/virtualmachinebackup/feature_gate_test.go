package virtualmachinebackup

import (
	"context"
	"fmt"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	corev1 "k8s.io/api/core/v1"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	werror "github.com/harvester/harvester/pkg/webhook/error"
)

func TestCreateBackupGate(t *testing.T) {
	for _, backupType := range []v1beta1.BackupType{
		v1beta1.Restic, v1beta1.Kopia, v1beta1.Backup, v1beta1.Snapshot,
	} {
		for _, recovered := range []bool{false, true} {
			for _, tt := range []struct {
				name    string
				field   string
				enabled bool
			}{
				{name: "omitted"},
				{name: "disabled", field: `,"thirdPartyBackupEnabled":false`},
				{name: "null", field: `,"thirdPartyBackupEnabled":null`},
				{name: "enabled", field: `,"thirdPartyBackupEnabled":true`, enabled: true},
			} {
				name := fmt.Sprintf("%s/recovered=%t/%s", backupType, recovered, tt.name)
				t.Run(name, func(t *testing.T) {
					validator := gateValidator(`{"type":"s3","bucketName":"backups"` + tt.field + `}`)
					vmb := newVMBackup("default", "test-vm", backupType)
					if recovered {
						vmb.Status.SourceSpec = &v1beta1.VirtualMachineSourceSpec{}
						vmb.Status.VolumeBackups = []v1beta1.VolumeBackup{}
					}

					// Disabling the gate must not prevent updates to an existing backup.
					require.NoError(t, validator.Update(nil, vmb, vmb.DeepCopy()))
					err := validator.Create(nil, vmb)
					if !backupType.IsDataMover() || tt.enabled {
						require.NoError(t, err)
						return
					}
					require.ErrorContains(t, err, "thirdPartyBackupEnabled")
					var admissionError werror.AdmitError
					require.ErrorAs(t, err, &admissionError)
					require.Equal(t, "spec.type", admissionError.AsResult().Details.Causes[0].Field)
				})
			}
		}
	}
}

func TestBackupGateInvalidTarget(t *testing.T) {
	for _, value := range []string{"", `{}`, `{"type":"s3","thirdPartyBackupEnabled":"true"}`} {
		t.Run(value, func(t *testing.T) {
			validator := gateValidator(value)
			for _, backupType := range []v1beta1.BackupType{v1beta1.Restic, v1beta1.Kopia} {
				require.Error(t, validator.Create(nil, newVMBackup("default", "test-vm", backupType)))
			}
			// Local snapshots do not require a backup target.
			require.NoError(t, validator.Create(nil, newVMBackup("default", "test-vm", v1beta1.Snapshot)))
		})
	}
}

func gateValidator(target string) *virtualMachineBackupValidator {
	objects := []runtime.Object{newVM("default", "test-vm"), newCSIDriverConfigSetting()}
	if target != "" {
		objects = append(objects, &v1beta1.Setting{
			ObjectMeta: metav1.ObjectMeta{Name: settings.BackupTargetSettingName},
			Value:      target,
		})
	}
	clientset := fake.NewSimpleClientset(objects...)
	setting, _ := clientset.HarvesterhciV1beta1().Settings().Get(context.Background(), settings.BackupTargetSettingName, metav1.GetOptions{})
	config, err := settings.DecodeBackupTarget(target)
	if setting != nil && err == nil && config.ThirdPartyBackupEnabled {
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName,
			UID: "credentials", ResourceVersion: "1",
		}}
		record, _ := backuputil.RepoReadyRecord(config, secret)
		setting.Annotations = map[string]string{backuputil.ReadyAnnotation: record}
		v1beta1.SettingConfigured.True(setting)
		_, _ = clientset.CoreV1().Secrets(secret.Namespace).Create(context.Background(), secret, metav1.CreateOptions{})
		_, _ = clientset.HarvesterhciV1beta1().Settings().Update(context.Background(), setting, metav1.UpdateOptions{})
	}
	return &virtualMachineBackupValidator{
		targets:            backuputil.TargetClient{Settings: fakeclients.HarvesterSettingClient(clientset.HarvesterhciV1beta1().Settings), Secrets: fakeclients.SecretClient(clientset.CoreV1().Secrets)},
		vms:                fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
		setting:            fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
		resourceQuotaCache: absentQuotaCache{},
		vmimCache:          emptyMigrationCache{},
		vmbr:               common.NewVMBackupReader(),
	}
}

type absentQuotaCache struct {
	ctlharvesterv1.ResourceQuotaCache
}

func (absentQuotaCache) Get(_, name string) (*v1beta1.ResourceQuota, error) {
	return nil, apierrors.NewNotFound(v1beta1.SchemeGroupVersion.WithResource("resourcequotas").GroupResource(), name)
}

type emptyMigrationCache struct {
	ctlkubevirtv1.VirtualMachineInstanceMigrationCache
}

func (emptyMigrationCache) GetByIndex(_, _ string) ([]*kubevirtv1.VirtualMachineInstanceMigration, error) {
	return nil, nil
}
