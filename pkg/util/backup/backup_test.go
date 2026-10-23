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
	require.Equal(t, id, RepositoryID(&rotated))

	otherRepository := *target
	otherRepository.BucketName = "other"
	require.NotEqual(t, id, RepositoryID(&otherRepository))
}

func TestRepositoryPassword(t *testing.T) {
	repositoryID := "sha256:repository"
	resticPassword, err := RepositoryPassword(repositoryID, harvesterv1.Restic)
	require.NoError(t, err)

	samePassword, err := RepositoryPassword(repositoryID, harvesterv1.Restic)
	require.NoError(t, err)
	require.Equal(t, resticPassword, samePassword)

	kopiaPassword, err := RepositoryPassword(repositoryID, harvesterv1.Kopia)
	require.NoError(t, err)
	require.NotEqual(t, resticPassword, kopiaPassword)

	_, err = RepositoryPassword("", harvesterv1.Restic)
	require.EqualError(t, err, "repository ID is empty")
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
