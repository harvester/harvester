package backup

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	restoreengine "github.com/harvester/harvester/pkg/restore/engine"
)

func TestRestoreLookupErrors(t *testing.T) {
	for _, tc := range reconcileCases(restoreengine.ErrRetryLater) {
		t.Run(tc.name, func(t *testing.T) {
			h, engine, client := retryRestoreFixture(harvesterv1.Restic)
			h.vmro.(*retryRestoreOperator).lookupErr = tc.err
			before := client.current.DeepCopy()
			err := h.reconcileRestore(client.current)
			require.Empty(t, engine.calls, "do not start resources without the source backup")
			require.Equal(t, tc.failed, h.vmro.IsFailed(client.current))
			queued := h.vmrController.(*retryRestoreController).enqueues
			if tc.failed {
				require.ErrorIs(t, err, tc.err)
				require.Empty(t, queued)
				return
			}
			require.NoError(t, err)
			require.Equal(t, before, client.current)
			require.Equal(t, []enqueueCall{{before.Namespace, before.Name, restoreProgressPoll}}, queued)
		})
	}
}

func TestRestoreSetupConflicts(t *testing.T) {
	for _, stage := range []string{"initialization", "type", "volumes", "owner"} {
		t.Run(stage, func(t *testing.T) {
			h, client := restoreVMFixture()
			client.updateErr = conflictError()
			switch stage {
			case "initialization":
				client.current.Status.Complete = nil
			case "type":
				delete(client.current.Annotations, restorecommon.BackupTypeAnnotation)
			case "volumes":
				client.current.Status.VolumeRestores = nil
				h.vmro.(*retryRestoreOperator).volumesErr = conflictError()
			case "owner":
				client.current.OwnerReferences = nil
			}
			before := client.current.DeepCopy()
			require.NoError(t, h.reconcileRestore(client.current))
			require.Equal(t, before, client.current, "conflict must not persist a failure")
			require.Equal(t, []enqueueCall{{before.Namespace, before.Name, restoreProgressPoll}},
				h.vmrController.(*retryRestoreController).enqueues)
		})
	}
}

func TestRestoreProgressWriteFailure(t *testing.T) {
	for _, retryErr := range []error{restoreengine.ErrRetryLater, conflictError()} {
		t.Run(retryErr.Error(), func(t *testing.T) {
			h, engine, client := retryRestoreFixture(harvesterv1.Restic)
			engine.errors = []error{retryErr}
			client.updateErr = errors.New("status write failed")
			before := client.current.DeepCopy()

			err := h.reconcileRestore(client.current)
			require.ErrorIs(t, err, retryErr)
			require.ErrorIs(t, err, client.updateErr)
			require.Equal(t, before, client.current)
			require.Empty(t, h.vmrController.(*retryRestoreController).enqueues,
				"a real progress write failure must reach Wrangler instead of becoming a timed retry")

			client.updateErr = nil
			require.NoError(t, h.reconcileRestore(client.current))
			require.Equal(t, 50, client.current.Status.Progress)
			require.Equal(t, []enqueueCall{{before.Namespace, before.Name, restoreProgressPoll}},
				h.vmrController.(*retryRestoreController).enqueues)
		})
	}
}

func TestRestoreStartErrors(t *testing.T) {
	cases := append(reconcileCases(restoreengine.ErrRetryLater),
		reconcileCase{name: "accepted start"},
		reconcileCase{name: "start in progress", err: restorecommon.ErrVMStartInProgress},
		reconcileCase{name: "wrapped start in progress", err: fmt.Errorf("start: %w", restorecommon.ErrVMStartInProgress)},
		reconcileCase{name: "start in progress and conflict", err: errors.Join(restorecommon.ErrVMStartInProgress, conflictError())},
		reconcileCase{
			name: "start in progress and failure", failed: true,
			err: errors.Join(restorecommon.ErrVMStartInProgress, errors.New("start failed")),
		})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, client := restoreVMFixture()
			op := h.vmro.(*retryRestoreOperator)
			op.startErr = tc.err
			before := client.current
			original := before.DeepCopy()
			err := h.reconcileRestore(before)
			require.Equal(t, original, before, "informer objects must not be mutated")
			require.Equal(t, tc.failed, h.vmro.IsFailed(client.current))
			require.False(t, h.vmro.IsComplete(client.current))
			require.Zero(t, op.cleanups, "do not finalize until the VM is ready")
			queued := h.vmrController.(*retryRestoreController).enqueues
			if tc.failed {
				require.ErrorIs(t, err, tc.err)
				require.Empty(t, queued)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 100, client.current.Status.VolumeRestores[0].Progress,
				"save transfer progress while waiting for the VM")
			require.True(t, h.vmro.IsProgressing(client.current))
			require.Equal(t, []enqueueCall{{before.Namespace, before.Name, restoreProgressPoll}}, queued)

			op.vm.Status.Ready = true
			h.vmrController.(*retryRestoreController).enqueues = nil
			require.NoError(t, h.reconcileRestore(client.current))
			require.True(t, h.vmro.IsComplete(client.current))
			require.Empty(t, h.vmrController.(*retryRestoreController).enqueues,
				"completion must not schedule another progress poll")
			require.Equal(t, 1, op.starts)
			require.Equal(t, 1, op.cleanups)
		})
	}
}

func TestRestorePreservesStartRetry(t *testing.T) {
	for _, startErr := range []error{
		restoreengine.ErrRetryLater, conflictError(), restorecommon.ErrVMStartInProgress,
	} {
		for _, tc := range []struct {
			name    string
			err     error
			requeue bool
		}{
			{name: "saved", requeue: true},
			{name: "conflict", err: conflictError(), requeue: true},
			{name: "throttled", err: apierrors.NewTooManyRequests("API throttled", 1), requeue: true},
			{name: "unavailable", err: apierrors.NewServiceUnavailable("status unavailable"), requeue: true},
			{name: "failure", err: errors.New("status write failed")},
		} {
			t.Run(startErr.Error()+"/"+tc.name, func(t *testing.T) {
				h, client := restoreVMFixture()
				h.vmro.(*retryRestoreOperator).startErr = startErr
				client.updateErr = tc.err
				controller := h.vmrController.(*retryRestoreController)

				err := h.runRestore(client.current)
				require.ErrorIs(t, err, startErr, "preserve the start error until the enqueue boundary")
				if tc.err != nil {
					require.ErrorIs(t, err, tc.err)
				}
				require.Empty(t, controller.enqueues)
				require.False(t, h.vmro.IsFailed(client.current))

				err = h.reconcileRestore(client.current)
				if !tc.requeue {
					require.ErrorIs(t, err, startErr)
					require.ErrorIs(t, err, tc.err)
					require.Empty(t, controller.enqueues)
					return
				}
				require.NoError(t, err)
				require.Equal(t, []enqueueCall{{client.current.Namespace, client.current.Name, restoreProgressPoll}},
					controller.enqueues)
			})
		}
	}
}

func TestRestoreFinishConflicts(t *testing.T) {
	for _, stage := range []string{"waiting status", "cleanup", "complete status"} {
		t.Run(stage, func(t *testing.T) {
			h, client := restoreVMFixture()
			op := h.vmro.(*retryRestoreOperator)
			op.vm.Status.Ready = stage != "waiting status"
			if stage == "cleanup" {
				op.cleanupErr = conflictError()
			} else {
				client.updateErr = conflictError()
			}
			before := client.current.DeepCopy()
			require.NoError(t, h.reconcileRestore(client.current))
			require.False(t, h.vmro.IsFailed(client.current))
			require.False(t, h.vmro.IsComplete(client.current))
			require.Equal(t, []enqueueCall{{before.Namespace, before.Name, restoreProgressPoll}},
				h.vmrController.(*retryRestoreController).enqueues)

			client.updateErr = nil
			op.cleanupErr = nil
			op.vm.Status.Ready = true
			require.NoError(t, h.reconcileRestore(client.current))
			require.True(t, h.vmro.IsComplete(client.current))
		})
	}
}

func restoreVMFixture() (*RestoreHandler, *retryRestoreClient) {
	h, engine, client := retryRestoreFixture(harvesterv1.Restic)
	engine.errors = []error{nil}
	engine.progress = 100
	client.current.OwnerReferences = []metav1.OwnerReference{{Kind: "VirtualMachine", Name: "target", UID: "target-uid"}}
	h.vmro.(*retryRestoreOperator).vm = &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default", Name: "target", UID: "target-uid",
			Annotations: map[string]string{lastRestoreAnnotation: h.vmro.GetRestoreID(client.current)},
		},
	}
	return h, client
}
