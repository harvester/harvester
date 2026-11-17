package setting

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const configInput = `{"restic":{"memoryLimit":"8Gi"}}`

func TestThirdPartyBusyBackups(t *testing.T) {
	for _, engine := range []harvesterv1.BackupType{harvesterv1.Restic, harvesterv1.Kopia, harvesterv1.Backup, harvesterv1.Snapshot} {
		for _, state := range []string{"pending", "running", "complete", "deleting"} {
			t.Run(string(engine)+"/"+state, func(t *testing.T) {
				backup := &harvesterv1.VirtualMachineBackup{
					ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "another-namespace"},
					Spec:       harvesterv1.VirtualMachineBackupSpec{Type: engine},
				}
				if state != "pending" {
					backup.Status.ReadyToUse = ptr.To(state != "running")
				}
				if state == "deleting" {
					backup.DeletionTimestamp = ptr.To(metav1.Now())
				}
				validator, _ := thirdPartyValidator(backup)
				oldSetting, newSetting := thirdPartySettings()
				err := validator.Update(nil, oldSetting, newSetting)
				createErr := validator.Create(nil, newSetting)
				deleteErr := validator.Delete(nil, oldSetting)
				if engine.IsDataMover() && state != "complete" {
					assert.ErrorContains(t, err, "VMBackup another-namespace/backup")
					assert.Error(t, createErr)
					assert.Error(t, deleteErr)
					return
				}
				assert.NoError(t, err)
				assert.NoError(t, createErr)
				assert.NoError(t, deleteErr)
			})
		}
	}
}

func TestThirdPartyBusyRestores(t *testing.T) {
	for _, engine := range []harvesterv1.BackupType{harvesterv1.Restic, harvesterv1.Kopia, harvesterv1.Backup, harvesterv1.Snapshot} {
		for _, annotated := range []bool{false, true} {
			for _, state := range []string{"pending", "running", "complete", "deleting"} {
				t.Run(fmt.Sprintf("%s/%t/%s", engine, annotated, state), func(t *testing.T) {
					backup := &harvesterv1.VirtualMachineBackup{
						ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "source"},
						Spec:       harvesterv1.VirtualMachineBackupSpec{Type: engine},
						Status:     harvesterv1.VirtualMachineBackupStatus{ReadyToUse: ptr.To(true)},
					}
					restore := &harvesterv1.VirtualMachineRestore{
						ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "destination"},
						Spec: harvesterv1.VirtualMachineRestoreSpec{
							VirtualMachineBackupNamespace: backup.Namespace, VirtualMachineBackupName: backup.Name,
						},
					}
					if annotated {
						restore.Annotations = map[string]string{restorecommon.BackupTypeAnnotation: string(engine)}
					}
					if state != "pending" {
						restore.Status.Complete = ptr.To(state != "running")
					}
					if state == "deleting" {
						restore.DeletionTimestamp = ptr.To(metav1.Now())
					}
					validator, _ := thirdPartyValidator(backup, restore)
					oldSetting, newSetting := thirdPartySettings()
					err := validator.Update(nil, oldSetting, newSetting)
					createErr := validator.Create(nil, newSetting)
					deleteErr := validator.Delete(nil, oldSetting)
					if engine.IsDataMover() && state != "complete" {
						assert.ErrorContains(t, err, "VMRestore destination/restore")
						assert.Error(t, createErr)
						assert.Error(t, deleteErr)
						return
					}
					assert.NoError(t, err)
					assert.NoError(t, createErr)
					assert.NoError(t, deleteErr)
				})
			}
		}
	}
}

func TestThirdPartyLookupErrors(t *testing.T) {
	for _, resource := range []string{"virtualmachinebackups", "virtualmachinerestores"} {
		t.Run(resource, func(t *testing.T) {
			validator, client := thirdPartyValidator()
			client.PrependReactor("*", resource, func(clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, fmt.Errorf("injected read failure")
			})
			oldSetting, newSetting := thirdPartySettings()
			assert.ErrorContains(t, validator.Update(nil, oldSetting, newSetting), "injected read failure")
		})
	}
}

func TestBackupConfigWithoutPasswords(t *testing.T) {
	validator, client := thirdPartyValidator()
	client.PrependReactor("get", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("resource settings must not read credentials")
	})
	old, updated := thirdPartySettings()
	assert.NoError(t, validator.Create(nil, updated))
	assert.NoError(t, validator.Update(nil, old, updated))
	for _, engine := range []string{"restic", "kopia"} {
		updated.Value = fmt.Sprintf(`{"%s":{"password":"unused"}}`, engine)
		assert.ErrorContains(t, validator.Update(nil, old, updated), "unknown field")
	}
}

func TestBackupConfigBootstrap(t *testing.T) {
	busy := &harvesterv1.VirtualMachineBackup{
		Spec: harvesterv1.VirtualMachineBackupSpec{Type: harvesterv1.Restic},
	}
	validator, _ := thirdPartyValidator(busy)
	setting := &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{Name: settings.ThirdPartyBackupConfigSettingName},
		Default:    settings.InitThirdPartyBackupConfig(),
	}
	assert.NoError(t, validator.Create(nil, setting), "allow startup to create the default Setting")
	setting.Default = `{"restic":{"memoryLimit":"invalid"}}`
	assert.Error(t, validator.Create(nil, setting), "validate defaults even when value is empty")
}

func thirdPartySettings() (*harvesterv1.Setting, *harvesterv1.Setting) {
	oldSetting := &harvesterv1.Setting{
		ObjectMeta: metav1.ObjectMeta{Name: settings.ThirdPartyBackupConfigSettingName}, Value: "{}",
	}
	newSetting := oldSetting.DeepCopy()
	newSetting.Value = configInput
	return oldSetting, newSetting
}

func thirdPartySecret(target *settings.BackupTarget) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName},
		Data: map[string][]byte{
			backuputil.RepositoryIDKey:   []byte(backuputil.RepositoryID(target)),
			backuputil.ResticPasswordKey: []byte("repo-password"), backuputil.KopiaPasswordKey: []byte("repo-password"),
		},
	}
}

func thirdPartyValidator(objects ...runtime.Object) (*settingValidator, *fake.Clientset) {
	client := fake.NewSimpleClientset(objects...)
	validator := NewValidator(nil, nil,
		fakeclients.VMBackupCache(client.HarvesterhciV1beta1().VirtualMachineBackups), nil,
		restoreListCache{client: client}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		fakeclients.SecretCache(client.CoreV1().Secrets), nil,
	)
	validator.(*settingValidator).settingCache = fakeclients.HarvesterSettingCache(client.HarvesterhciV1beta1().Settings)
	return validator.(*settingValidator), client
}

type restoreListCache struct {
	ctlharvesterv1.VirtualMachineRestoreCache
	client *fake.Clientset
}

func (c restoreListCache) List(namespace string, selector labels.Selector) ([]*harvesterv1.VirtualMachineRestore, error) {
	list, err := c.client.HarvesterhciV1beta1().VirtualMachineRestores(namespace).List(
		context.Background(), metav1.ListOptions{LabelSelector: selector.String()},
	)
	if err != nil {
		return nil, err
	}
	result := make([]*harvesterv1.VirtualMachineRestore, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, nil
}
