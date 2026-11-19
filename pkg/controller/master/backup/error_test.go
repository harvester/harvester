package backup

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupengine "github.com/harvester/harvester/pkg/backup/engine"
)

func statusErrorCases() []struct {
	name string
	err  error
} {
	return []struct {
		name string
		err  error
	}{
		{name: "saved"},
		{name: "conflict", err: conflictError()},
		{name: "unavailable", err: apierrors.NewServiceUnavailable("status unavailable")},
	}
}

func TestBackupPreservesFailure(t *testing.T) {
	failure := errors.New("backup failed")
	for _, tc := range statusErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			h, engine, vmb, client := completionFixture(t, harvesterv1.Restic)
			engine.reconcile = func(*harvesterv1.VirtualMachineBackup, int) error { return failure }
			updateErr := tc.err
			client.PrependReactor("update", "virtualmachinebackups",
				func(clienttesting.Action) (bool, runtime.Object, error) {
					return updateErr != nil, nil, updateErr
				})
			before := vmb.DeepCopy()
			err := h.reconcileBackup(vmb)
			require.ErrorIs(t, err, failure)
			require.Equal(t, before, vmb)
			require.Empty(t, h.vmbController.(*requeueBackupController).enqueues)
			current, getErr := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).
				Get(context.Background(), vmb.Name, metav1.GetOptions{})
			require.NoError(t, getErr)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				require.Equal(t, before.Status, current.Status, "failed status writes must not mutate the stored backup")
			}

			updateErr = nil
			require.ErrorIs(t, h.reconcileBackup(current), failure)
			current, getErr = client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).
				Get(context.Background(), vmb.Name, metav1.GetOptions{})
			require.NoError(t, getErr)
			require.Equal(t, failure.Error(), *current.Status.Error.Message)
		})
	}
}

func TestBackupProgressWriteFailure(t *testing.T) {
	for _, retryErr := range []error{backupengine.ErrRetryLater, conflictError()} {
		t.Run(retryErr.Error(), func(t *testing.T) {
			h, engine, vmb, client := completionFixture(t, harvesterv1.Restic)
			engine.reconcile = func(vmb *harvesterv1.VirtualMachineBackup, index int) error {
				vmb.Status.VolumeBackups[index].Progress = 50
				return retryErr
			}
			var updateErr error = apierrors.NewServiceUnavailable("status unavailable")
			client.PrependReactor("update", "virtualmachinebackups",
				func(clienttesting.Action) (bool, runtime.Object, error) {
					return updateErr != nil, nil, updateErr
				})
			before := vmb.DeepCopy()
			err := h.reconcileBackup(vmb)
			require.ErrorIs(t, err, retryErr)
			require.ErrorIs(t, err, updateErr)
			require.Equal(t, before, vmb)
			require.Empty(t, h.vmbController.(*requeueBackupController).enqueues)
			current, err := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).
				Get(context.Background(), vmb.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, before.Status, current.Status)

			updateErr = nil
			require.NoError(t, h.reconcileBackup(current))
			current, err = client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).
				Get(context.Background(), vmb.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, 50, current.Status.Progress)
			require.Nil(t, current.Status.Error)
			require.Equal(t, []enqueueCall{{vmb.Namespace, vmb.Name, engineRetryDelay}},
				h.vmbController.(*requeueBackupController).enqueues)
		})
	}
}

func TestRestorePreservesFailure(t *testing.T) {
	failure := errors.New("restore failed")
	for _, stage := range []string{"lookup", "transfer", "start"} {
		for _, tc := range statusErrorCases() {
			t.Run(stage+"/"+tc.name, func(t *testing.T) {
				h, engine, client := retryRestoreFixture(harvesterv1.Restic)
				switch stage {
				case "lookup":
					h.vmro.(*retryRestoreOperator).lookupErr = failure
				case "transfer":
					engine.errors = []error{failure}
				case "start":
					h, client = restoreVMFixture()
					h.vmro.(*retryRestoreOperator).startErr = failure
				}
				client.updateErr = tc.err
				before := client.current.DeepCopy()
				err := h.reconcileRestore(client.current)
				require.ErrorIs(t, err, failure)
				require.Empty(t, h.vmrController.(*retryRestoreController).enqueues)
				if tc.err != nil {
					require.ErrorIs(t, err, tc.err)
					require.Equal(t, before, client.current)
					client.updateErr = nil
					require.ErrorIs(t, h.reconcileRestore(client.current), failure)
				}
				require.True(t, h.vmro.IsFailed(client.current))
				for _, condition := range client.current.Status.Conditions {
					require.Contains(t, condition.Message, failure.Error())
				}
			})
		}
	}
}

func TestRestoreCleanupFailureWithConflict(t *testing.T) {
	h, client := restoreVMFixture()
	op := h.vmro.(*retryRestoreOperator)
	op.vm.Status.Ready = true
	op.cleanupErr = errors.New("cleanup unavailable")
	client.updateErr = conflictError()
	before := client.current.DeepCopy()
	err := h.reconcileRestore(client.current)
	require.ErrorIs(t, err, op.cleanupErr)
	require.ErrorIs(t, err, client.updateErr)
	require.Equal(t, before, client.current)
	require.Empty(t, h.vmrController.(*retryRestoreController).enqueues)

	client.updateErr = nil
	require.ErrorIs(t, h.reconcileRestore(client.current), op.cleanupErr)
	require.True(t, h.vmro.IsProgressing(client.current))
	require.False(t, h.vmro.IsFailed(client.current), "cleanup failures must allow another reconciliation")

	op.cleanupErr = nil
	require.NoError(t, h.reconcileRestore(client.current))
	require.True(t, h.vmro.IsComplete(client.current))
}
