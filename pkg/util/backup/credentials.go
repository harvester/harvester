package backup

import (
	"fmt"
	"strings"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	RepositoryIDKey   = "REPOSITORY_ID"
	ResticPasswordKey = "RESTIC_PASSWORD"
	KopiaPasswordKey  = "KOPIA_PASSWORD"
)

func CheckRepositoryPasswords(data map[string][]byte) error {
	for _, key := range []string{ResticPasswordKey, KopiaPasswordKey} {
		if strings.TrimSpace(string(data[key])) == "" || strings.ContainsRune(string(data[key]), 0) {
			return fmt.Errorf("backup target Secret is missing a valid %s", key)
		}
	}
	if string(data[ResticPasswordKey]) != string(data[KopiaPasswordKey]) {
		return fmt.Errorf("backup target Secret has inconsistent repository passwords")
	}
	return nil
}

// CheckPasswordInput validates the password submitted for an enabled target.
// Masked reconciliation and disabling third-party backups do not use this check.
func CheckPasswordInput(password string) error {
	switch {
	case strings.TrimSpace(password) == "":
		return fmt.Errorf("thirdPartyRepositoryPassword is required when third-party backups are enabled")
	case strings.ContainsRune(password, 0):
		return fmt.Errorf("thirdPartyRepositoryPassword must not contain a NUL character")
	default:
		return nil
	}
}

// CheckPasswordChange compares a validated password input with the stored repository:
//   - No stored ID or a different ID: allow a new password.
//   - Same ID, both passwords absent: allow initialization, including re-enabling.
//   - Same ID, either password present: require both valid, equal and matching input.
//     Reject a replacement, a missing password or inconsistent stored passwords.
//
// Updating the Secret does not change the password protecting the remote repository.
func CheckPasswordChange(data map[string][]byte, repositoryID, password string) error {
	if string(data[RepositoryIDKey]) != repositoryID {
		return nil
	}
	if len(data[ResticPasswordKey]) == 0 && len(data[KopiaPasswordKey]) == 0 {
		return nil
	}
	if err := CheckRepositoryPasswords(data); err != nil {
		return err
	}
	if string(data[ResticPasswordKey]) != password {
		return fmt.Errorf("thirdPartyRepositoryPassword must match the password of the configured repository; " +
			"changing the Secret does not change the repository password")
	}
	return nil
}

// CredentialData copies only the credentials needed by one engine. The shared
// Secret is read live when creating or repairing a per-Job Secret.
func CredentialData(
	client ctlcorev1.SecretClient, target *harvesterv1.BackupTarget, passwordKey string,
) (map[string][]byte, error) {
	if err := CheckTarget(target); err != nil {
		return nil, err
	}
	source, err := client.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return credentials(source.Data, target.RepositoryID, passwordKey)
}

func credentials(data map[string][]byte, repositoryID, passwordKey string) (map[string][]byte, error) {
	if passwordKey != ResticPasswordKey && passwordKey != KopiaPasswordKey {
		return nil, fmt.Errorf("unsupported repository password key")
	}
	if repositoryID == "" || string(data[RepositoryIDKey]) != repositoryID {
		return nil, fmt.Errorf("backup target Secret repository ID does not match the VMBackup")
	}
	result := make(map[string][]byte, 3)
	for _, key := range []string{util.AWSAccessKey, util.AWSSecretKey, passwordKey} {
		if len(data[key]) == 0 {
			return nil, fmt.Errorf("backup target Secret is missing %s", key)
		}
		if strings.ContainsRune(string(data[key]), 0) {
			return nil, fmt.Errorf("backup target Secret has an invalid %s", key)
		}
		result[key] = append([]byte(nil), data[key]...)
	}
	return result, nil
}

// SecretEnv is shared by per-Job Secrets and system workloads using the target Secret.
func SecretEnv(secretName, passwordKey string) []corev1.EnvVar {
	env := make([]corev1.EnvVar, 0, 3)
	for _, key := range []string{util.AWSAccessKey, util.AWSSecretKey, passwordKey} {
		env = append(env, corev1.EnvVar{Name: key, ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secretName}, Key: key},
		}})
	}
	return env
}
