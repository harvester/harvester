package backup

import (
	"testing"

	"github.com/stretchr/testify/require"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

func TestRepositoryIDIgnoresCredentials(t *testing.T) {
	target := &settings.BackupTarget{
		Type:               settings.S3BackupType,
		Endpoint:           "https://s3.example.com/",
		AccessKeyID:        "access-a",
		SecretAccessKey:    "secret-a",
		BucketName:         "backups",
		BucketRegion:       "region-a",
		Cert:               "certificate-a",
		VirtualHostedStyle: true,
	}
	id := RepositoryID(target)

	rotated := *target
	rotated.AccessKeyID = "access-b"
	rotated.SecretAccessKey = "secret-b"
	rotated.Cert = "certificate-b"
	rotated.ThirdPartyBackupEnabled = true
	rotated.RepoPassword = "repo-password"
	require.Equal(t, id, RepositoryID(&rotated))

	otherRepository := *target
	otherRepository.BucketName = "other"
	require.NotEqual(t, id, RepositoryID(&otherRepository))
}

func TestCheckTarget(t *testing.T) {
	oldTarget := settings.BackupTargetSet.Get()
	t.Cleanup(func() { require.NoError(t, settings.BackupTargetSet.Set(oldTarget)) })
	for _, test := range []struct {
		name    string
		current string
		want    string
	}{
		{name: "same", current: `{"type":"s3","bucketName":"backups"}`},
		{
			name: "rotated credentials",
			current: `{"type":"s3","bucketName":"backups",` +
				`"accessKeyId":"new-access","secretAccessKey":"new-secret"}`,
		},
		{
			name: "different target", current: `{"type":"s3","bucketName":"other"}`,
			want: "does not match the configured backup target",
		},
		{
			name: "disconnected", current: `{}`,
			want: "does not match the configured backup target",
		},
		{name: "invalid setting", current: "invalid", want: "invalid"},
		{name: "missing target", current: `{}`, want: "does not record a backup target"},
		{name: "invalid identity", current: `{}`, want: "does not match its recorded backup target"},
	} {
		t.Run(test.name, func(t *testing.T) {
			status := BackupTargetStatus(&settings.BackupTarget{
				Type: settings.S3BackupType, BucketName: "backups",
			})
			if test.name == "missing target" {
				status = nil
			}
			if test.name == "invalid identity" {
				status.RepositoryID = "sha256:invalid"
			}
			require.NoError(t, settings.BackupTargetSet.Set(test.current))
			err := CheckTarget(status)
			if test.want != "" {
				require.ErrorContains(t, err, test.want)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestTargetFromStatus(t *testing.T) {
	target := &settings.BackupTarget{
		Type:         settings.S3BackupType,
		Endpoint:     "https://s3.example.com",
		BucketName:   "backups",
		BucketRegion: "region-a",
	}
	status := BackupTargetStatus(target)

	recorded, err := TargetFromStatus(status)
	require.NoError(t, err)
	require.Equal(t, target.Type, recorded.Type)
	require.Equal(t, target.Endpoint, recorded.Endpoint)
	require.True(t, IsBackupTargetSame(status, target))

	status.BucketName = "tampered"
	_, err = TargetFromStatus(status)
	require.EqualError(t, err, "VMBackup repository ID does not match its recorded backup target")
}

func TestTargetFromStatusRequiresRepositoryIdentity(t *testing.T) {
	_, err := TargetFromStatus(&harvesterv1.BackupTarget{
		RepositoryID: "sha256:repository",
	})
	require.EqualError(t, err, "VMBackup does not record a backup target type")

	_, err = TargetFromStatus(&harvesterv1.BackupTarget{
		Type: string(settings.S3BackupType),
	})
	require.EqualError(t, err, "VMBackup does not record a repository ID")
}
