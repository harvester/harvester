package schedulevmbackup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	werror "github.com/harvester/harvester/pkg/webhook/error"
)

func TestCreate(t *testing.T) {
	tests := []struct {
		name       string
		backupType v1beta1.BackupType
		suspend    bool
		wantError  string
		field      string
	}{
		{
			name:       "rejects restic",
			backupType: v1beta1.Restic,
			wantError:  "restic is not supported for scheduled VM backups",
			field:      "spec.vmbackup.type",
		},
		{
			name:       "rejects kopia",
			backupType: v1beta1.Kopia,
			wantError:  "kopia is not supported for scheduled VM backups",
			field:      "spec.vmbackup.type",
		},
		{
			name:       "rejects suspended restic schedule",
			backupType: v1beta1.Restic,
			suspend:    true,
			wantError:  "restic is not supported for scheduled VM backups",
			field:      "spec.vmbackup.type",
		},
		{
			name:       "rejects suspended kopia schedule",
			backupType: v1beta1.Kopia,
			suspend:    true,
			wantError:  "kopia is not supported for scheduled VM backups",
			field:      "spec.vmbackup.type",
		},
		{
			name:       "allows local snapshot without backup target",
			backupType: v1beta1.Snapshot,
		},
		{
			name:       "native backup still requires backup target",
			backupType: v1beta1.Backup,
			wantError:  "setting backup-target is not set",
			field:      "spec.suspend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setting := &v1beta1.Setting{
				ObjectMeta: metav1.ObjectMeta{Name: settings.BackupTargetSettingName},
				Value:      `{}`,
			}
			clientset := fake.NewSimpleClientset(setting)
			validator := NewValidator(
				fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings),
				nil,
				fakeclients.SVMBackupCache(clientset.HarvesterhciV1beta1().ScheduleVMBackups),
			)
			schedule := &v1beta1.ScheduleVMBackup{
				ObjectMeta: metav1.ObjectMeta{Name: "test-schedule", Namespace: "default"},
				Spec: v1beta1.ScheduleVMBackupSpec{
					Cron:       "0 * * * *",
					Retain:     8,
					MaxFailure: 4,
					Suspend:    tt.suspend,
					VMBackupSpec: v1beta1.VirtualMachineBackupSpec{
						Type: tt.backupType,
					},
				},
			}

			err := validator.Create(nil, schedule)
			if tt.wantError == "" {
				require.NoError(t, err)
				return
			}

			require.EqualError(t, err, tt.wantError)
			var admitErr werror.AdmitError
			require.ErrorAs(t, err, &admitErr)
			status := admitErr.AsResult()
			assert.Equal(t, metav1.StatusReasonInvalid, status.Reason)
			require.NotNil(t, status.Details)
			require.Len(t, status.Details.Causes, 1)
			assert.Equal(t, tt.field, status.Details.Causes[0].Field)
		})
	}
}
