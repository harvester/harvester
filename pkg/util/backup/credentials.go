package backup

import (
	"fmt"
	"strings"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	RepositoryIDKey   = "REPOSITORY_ID"
	ResticPasswordKey = "RESTIC_PASSWORD"
	KopiaPasswordKey  = "KOPIA_PASSWORD"
)

// CheckCredentials validates a new Job without embedding credentials in its spec.
// The container reads and validates the shared Secret again at startup.
func CheckCredentials(
	client ctlcorev1.SecretClient, target *harvesterv1.BackupTarget, passwordKey string,
) error {
	if err := CheckTarget(target); err != nil {
		return err
	}
	source, err := client.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	_, err = CredentialEnv(source.Data, target.RepositoryID, passwordKey)
	return err
}

// CredentialEnv returns a process environment, never Kubernetes EnvVar values.
func CredentialEnv(data map[string][]byte, repositoryID, passwordKey string) ([]string, error) {
	if passwordKey != ResticPasswordKey && passwordKey != KopiaPasswordKey {
		return nil, fmt.Errorf("unsupported repository password key")
	}
	if repositoryID == "" || string(data[RepositoryIDKey]) != repositoryID {
		return nil, fmt.Errorf("backup target Secret repository ID does not match the VMBackup")
	}
	env := make([]string, 0, 3)
	for _, key := range []string{util.AWSAccessKey, util.AWSSecretKey, passwordKey} {
		if len(data[key]) == 0 {
			return nil, fmt.Errorf("backup target Secret is missing %s", key)
		}
		if strings.ContainsRune(string(data[key]), 0) {
			return nil, fmt.Errorf("backup target Secret has an invalid %s", key)
		}
		env = append(env, key+"="+string(data[key]))
	}
	return env, nil
}
