package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

func TestJobCleanupWaitsForFailure(t *testing.T) {
	vmro := NewVMRestoreOperatorBuilder().Build()
	for _, state := range []string{"running", "failed", "complete", "deleting"} {
		t.Run(state, func(t *testing.T) {
			vmr := &harvesterv1.VirtualMachineRestore{}
			if state != "running" {
				vmr = vmro.SetErrorCondition(vmr, errors.New("restore failed"))
			}
			if state == "complete" {
				vmr.Status.Complete = ptr.To(true)
			}
			if state == "deleting" {
				vmr.DeletionTimestamp = ptr.To(metav1.Now())
			}
			calls := 0
			cleanupErr := errors.New("cleanup unavailable")
			h := JobHandler{Reader: vmro, Delete: func(*harvesterv1.VirtualMachineRestore) error {
				calls++
				return cleanupErr
			}}
			_, err := h.OnRestoreChange("", vmr)
			if state == "failed" {
				require.ErrorIs(t, err, cleanupErr)
				require.Equal(t, 1, calls)
				return
			}
			require.NoError(t, err)
			require.Zero(t, calls)
		})
	}
}

type typeClient struct {
	ctlharvesterv1.VirtualMachineRestoreClient
	updated *harvesterv1.VirtualMachineRestore
	err     error
}

func (c *typeClient) Update(vmr *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
	c.updated = vmr
	return vmr, c.err
}

func TestRecordEngine(t *testing.T) {
	for _, backupType := range []harvesterv1.BackupType{
		harvesterv1.Backup, harvesterv1.Snapshot, harvesterv1.Restic, harvesterv1.Kopia,
	} {
		t.Run(string(backupType), func(t *testing.T) {
			client := &typeClient{}
			vmro := NewVMRestoreOperatorBuilder().WithClient(client).Build()
			vmr := &harvesterv1.VirtualMachineRestore{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{"existing": "keep"},
			}}
			require.Empty(t, vmro.GetType(vmr))
			require.NoError(t, vmro.UpdateType(vmr, backupType))
			require.Equal(t, backupType, vmro.GetType(client.updated))
			require.Equal(t, "keep", client.updated.Annotations["existing"])
			require.Empty(t, vmro.GetType(vmr), "do not mutate the informer object")
			recorded := client.updated
			client.updated = nil
			require.NoError(t, vmro.UpdateType(recorded, backupType))
			require.Nil(t, client.updated, "an unchanged engine needs no API update")
		})
	}
}

func TestRecordEngineErrors(t *testing.T) {
	apiErr := errors.New("API unavailable")
	client := &typeClient{err: apiErr}
	vmro := NewVMRestoreOperatorBuilder().WithClient(client).Build()
	vmr := &harvesterv1.VirtualMachineRestore{}
	require.ErrorIs(t, vmro.UpdateType(vmr, harvesterv1.Restic), apiErr)
	require.Empty(t, vmro.GetType(vmr))
	vmr.Annotations = map[string]string{BackupTypeAnnotation: string(harvesterv1.Kopia)}
	client.updated = nil
	require.ErrorContains(t, vmro.UpdateType(vmr, harvesterv1.Restic), "engine changed")
	require.ErrorContains(t, vmro.UpdateType(vmr, "unknown"), "unsupported")
	require.Nil(t, client.updated)
}
