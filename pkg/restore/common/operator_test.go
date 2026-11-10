package common

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

func TestIsVolumesStatusInitialized(t *testing.T) {
	tests := []struct {
		name        string
		status      harvesterv1.VirtualMachineRestoreStatus
		initialized bool
	}{
		{name: "no condition"},
		{
			name: "lists alone do not indicate initialization",
			status: harvesterv1.VirtualMachineRestoreStatus{
				VolumeRestores: []harvesterv1.VolumeRestore{{VolumeName: "disk-0"}},
				DeletedVolumes: []string{"old-pvc"},
			},
		},
		{
			name: "unrelated true condition",
			status: harvesterv1.VirtualMachineRestoreStatus{
				Conditions: []harvesterv1.Condition{{Type: harvesterv1.RestoreConditionReady, Status: corev1.ConditionTrue}},
			},
		},
		{
			name: "false condition",
			status: harvesterv1.VirtualMachineRestoreStatus{
				Conditions: []harvesterv1.Condition{{Type: harvesterv1.RestoreConditionVolumesStatusInitialized, Status: corev1.ConditionFalse}},
			},
		},
		{
			name: "unknown condition",
			status: harvesterv1.VirtualMachineRestoreStatus{
				Conditions: []harvesterv1.Condition{{Type: harvesterv1.RestoreConditionVolumesStatusInitialized, Status: corev1.ConditionUnknown}},
			},
		},
		{
			name: "true condition with empty lists",
			status: harvesterv1.VirtualMachineRestoreStatus{
				Conditions: []harvesterv1.Condition{{Type: harvesterv1.RestoreConditionVolumesStatusInitialized, Status: corev1.ConditionTrue}},
			},
			initialized: true,
		},
	}

	reader := NewVMRestoreReader()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.initialized, reader.IsVolumesStatusInitialized(&harvesterv1.VirtualMachineRestore{Status: tt.status}))
		})
	}
}

type volumeStatusTestClient struct {
	ctlharvesterv1.VirtualMachineRestoreClient
	update func(*harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error)
}

func (c *volumeStatusTestClient) Update(vmr *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
	return c.update(vmr)
}

type volumeStatusTestVMCache struct {
	ctlkubevirtv1.VirtualMachineCache
	get func(string, string) (*kubevirtv1.VirtualMachine, error)
}

func (c *volumeStatusTestVMCache) Get(namespace, name string) (*kubevirtv1.VirtualMachine, error) {
	return c.get(namespace, name)
}

func TestInitVolumesStatus(t *testing.T) {
	tests := []struct {
		name           string
		attachedPVC    string
		emptyBackup    bool
		newVM          bool
		deletionPolicy harvesterv1.DeletionPolicy
		failure        string
	}{
		{name: "all old disks detached", deletionPolicy: harvesterv1.VirtualMachineRestoreDelete},
		{name: "old disk still attached", attachedPVC: "current-pvc", deletionPolicy: harvesterv1.VirtualMachineRestoreDelete},
		{name: "empty backup", emptyBackup: true, deletionPolicy: harvesterv1.VirtualMachineRestoreDelete},
		{name: "retain old disks", deletionPolicy: harvesterv1.VirtualMachineRestoreRetain},
		{name: "new VM", newVM: true},
		{name: "volume restore construction fails", failure: "build", deletionPolicy: harvesterv1.VirtualMachineRestoreDelete},
		{name: "VM lookup fails", failure: "lookup", deletionPolicy: harvesterv1.VirtualMachineRestoreDelete},
		{name: "status update fails", failure: "update", deletionPolicy: harvesterv1.VirtualMachineRestoreDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vmr := &harvesterv1.VirtualMachineRestore{
				ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "default", UID: "restore-uid"},
				Spec: harvesterv1.VirtualMachineRestoreSpec{
					VirtualMachineBackupName: "backup",
					DeletionPolicy:           tt.deletionPolicy,
					NewVM:                    tt.newVM,
				},
				Status: harvesterv1.VirtualMachineRestoreStatus{
					Complete: ptr.To(false),
					Conditions: []harvesterv1.Condition{
						{Type: harvesterv1.RestoreConditionProgressing, Status: corev1.ConditionTrue},
					},
				},
			}
			original := vmr.DeepCopy()
			vmb := &harvesterv1.VirtualMachineBackup{
				ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default"},
				Spec: harvesterv1.VirtualMachineBackupSpec{
					Source: corev1.TypedLocalObjectReference{Kind: "VirtualMachine", Name: "source-vm"},
				},
			}
			if !tt.emptyBackup {
				vmb.Status.VolumeBackups = []harvesterv1.VolumeBackup{{
					Name:       ptr.To("snapshot"),
					VolumeName: "disk-0",
					PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
						ObjectMeta: metav1.ObjectMeta{Name: "backup-pvc"},
					},
				}}
			}
			failure := tt.failure
			if failure == "build" {
				vmb.Status.VolumeBackups[0].Name = nil
			}
			reader := NewVMRestoreReader()
			var persisted *harvesterv1.VirtualMachineRestore
			updates := 0
			op := NewVMRestoreOperatorBuilder().
				WithClient(&volumeStatusTestClient{
					update: func(obj *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
						updates++
						require.True(t, reader.IsVolumesStatusInitialized(obj), "save the condition together with the lists")
						if failure == "update" {
							return nil, errors.New("status update failed")
						}
						// Model API serialization, which omits empty volume lists.
						data, err := json.Marshal(obj)
						require.NoError(t, err)
						persisted = &harvesterv1.VirtualMachineRestore{}
						require.NoError(t, json.Unmarshal(data, persisted))
						return persisted, nil
					},
				}).
				WithVMCache(&volumeStatusTestVMCache{
					get: func(namespace, name string) (*kubevirtv1.VirtualMachine, error) {
						assert.Equal(t, "default", namespace)
						assert.Equal(t, "source-vm", name)
						require.False(t, tt.newVM)
						require.NotEqual(t, harvesterv1.VirtualMachineRestoreRetain, tt.deletionPolicy)
						if failure == "lookup" {
							return nil, errors.New("VM lookup failed")
						}
						vm := &kubevirtv1.VirtualMachine{
							Spec: kubevirtv1.VirtualMachineSpec{Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{}},
						}
						if tt.attachedPVC != "" {
							vm.Spec.Template.Spec.Volumes = []kubevirtv1.Volume{{
								Name: "disk-0",
								VolumeSource: kubevirtv1.VolumeSource{
									PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: tt.attachedPVC},
									},
								},
							}}
						}
						return vm, nil
					},
				}).
				WithVMBackupOperator(backupcommon.NewVMBackupOperatorBuilder().Build()).
				Build()

			err := op.InitVolumesStatus(vmr, vmb)
			assert.Equal(t, original, vmr, "initialization must not mutate the cached restore")
			if tt.failure != "" {
				require.Error(t, err)
				assert.Nil(t, persisted, "failed initialization must not persist success")
				assert.False(t, op.IsVolumesStatusInitialized(vmr))
				if failure != "update" {
					assert.Zero(t, updates, "do not persist partially initialized status")
				} else {
					assert.Equal(t, 1, updates)
				}
				// Once the failure is resolved, initialization can be retried.
				failure = ""
				vmb.Status.VolumeBackups[0].Name = ptr.To("snapshot")
				updates = 0
				require.NoError(t, op.InitVolumesStatus(vmr, vmb))
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, updates, "persist both lists and the condition in a single update")
			require.NotNil(t, persisted)
			assert.True(t, op.IsVolumesStatusInitialized(persisted), "the next reconcile must advance")
			if tt.emptyBackup {
				assert.Nil(t, persisted.Status.VolumeRestores)
			} else {
				require.Len(t, persisted.Status.VolumeRestores, 1)
				assert.Equal(t, "restore-backup-restore-uid-disk-0", persisted.Status.VolumeRestores[0].PersistentVolumeClaim.ObjectMeta.Name)
			}
			if tt.attachedPVC == "" {
				assert.Nil(t, persisted.Status.DeletedVolumes)
			} else {
				assert.Equal(t, []string{tt.attachedPVC}, persisted.Status.DeletedVolumes)
			}
			require.Len(t, persisted.Status.Conditions, 2)
			assert.Equal(t, original.Status.Conditions[0], persisted.Status.Conditions[0])
			cond := persisted.Status.Conditions[1]
			assert.Equal(t, harvesterv1.RestoreConditionVolumesStatusInitialized, cond.Type)
			assert.Equal(t, "InitializationComplete", cond.Reason)
			assert.NotEmpty(t, cond.LastUpdateTime)
			assert.Equal(t, cond.LastUpdateTime, cond.LastTransitionTime)
			assert.Equal(t, original, vmr)
		})
	}
}
