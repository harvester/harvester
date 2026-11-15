package backup

import (
	"testing"

	"github.com/stretchr/testify/require"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
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

func TestRepositoryPassword(t *testing.T) {
	for backupType, expected := range map[harvesterv1.BackupType]string{
		harvesterv1.Restic: "3c30c4edf2cdac81cb00e566aee0b042559a00d3ed6c0a630c0effc035635c33",
		harvesterv1.Kopia:  "006dbf985027f8ec512287867bcb75e00d0ec76243c7c991734ce10ffa6d0dcc",
	} {
		t.Run(string(backupType), func(t *testing.T) {
			credentials := map[string][]byte{
				util.AWSAccessKey: []byte("access-a"),
				util.AWSSecretKey: []byte("secret-a"),
			}
			password, err := RepositoryPassword("sha256:repository", backupType, credentials)
			require.NoError(t, err)
			require.Equal(t, expected, password)

			for _, key := range []string{util.AWSAccessKey, util.AWSSecretKey} {
				original := credentials[key]
				credentials[key] = []byte("rotated")
				rotated, err := RepositoryPassword("sha256:repository", backupType, credentials)
				require.NoError(t, err)
				require.NotEqual(t, password, rotated, "rotating %s must change the password", key)
				credentials[key] = original
			}

			other, err := RepositoryPassword("sha256:other", backupType, credentials)
			require.NoError(t, err)
			require.NotEqual(t, password, other)
		})
	}
}

func TestRepositoryPasswordRejectsMissingInputs(t *testing.T) {
	for _, test := range []struct {
		name         string
		repositoryID string
		backupType   harvesterv1.BackupType
		accessKey    string
		secretKey    string
		want         string
	}{
		{
			name: "repository ID", backupType: harvesterv1.Restic,
			accessKey: "access", secretKey: "secret", want: "repository ID is empty",
		},
		{
			name: "engine", repositoryID: "repo", backupType: harvesterv1.Backup,
			accessKey: "access", secretKey: "secret", want: `unsupported data-mover backup type "backup"`,
		},
		{
			name: "access key", repositoryID: "repo", backupType: harvesterv1.Restic,
			secretKey: "secret", want: "S3 access key ID is empty",
		},
		{
			name: "secret key", repositoryID: "repo", backupType: harvesterv1.Kopia,
			accessKey: "access", want: "S3 secret access key is empty",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			password, err := RepositoryPassword(test.repositoryID, test.backupType, map[string][]byte{
				util.AWSAccessKey: []byte(test.accessKey),
				util.AWSSecretKey: []byte(test.secretKey),
			})
			require.EqualError(t, err, test.want)
			require.Empty(t, password)
		})
	}

	password, err := RepositoryPassword("repo", harvesterv1.Restic, nil)
	require.EqualError(t, err, "S3 access key ID is empty")
	require.Empty(t, password)
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
