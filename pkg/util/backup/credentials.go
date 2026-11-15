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
