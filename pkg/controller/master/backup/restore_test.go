package backup

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/restore/engine"
)

func TestReconcileVolumeRestores(t *testing.T) {
	errBoot := errors.New("boot restore failed")
	errData := errors.New("data restore failed")

	tests := []struct {
		name       string
		errors     []error
		wantReady  bool
		wantRetry  bool
		wantErrors []error
		wantCalls  int
	}{
		{
			name:      "all volumes ready",
			errors:    []error{nil, nil},
			wantReady: true,
			wantCalls: 2,
		},
		{
			name:      "pending volume",
			errors:    []error{engine.ErrRetryLater, nil},
			wantRetry: true,
			wantCalls: 2,
		},
		{
			name:      "wrapped pending volume",
			errors:    []error{fmt.Errorf("snapshot check: %w", engine.ErrRetryLater), nil},
			wantRetry: true,
			wantCalls: 2,
		},
		{
			name:       "hard error stops reconciliation",
			errors:     []error{errBoot, errData},
			wantErrors: []error{errBoot},
			wantCalls:  1,
		},
		{
			name:       "hard error takes precedence over pending",
			errors:     []error{engine.ErrRetryLater, errData},
			wantErrors: []error{errData},
			wantCalls:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := &fakeRestoreEngine{errors: tt.errors}
			h := &RestoreHandler{
				vmro: restorecommon.NewVMRestoreOperatorBuilder().Build(),
				engines: map[harvesterv1.BackupType]engine.RestoreEngine{
					harvesterv1.Restic: re,
				},
			}
			vmr := &harvesterv1.VirtualMachineRestore{
				Status: harvesterv1.VirtualMachineRestoreStatus{
					VolumeRestores: []harvesterv1.VolumeRestore{
						{VolumeName: "boot-disk"},
						{VolumeName: "data-disk"},
					},
				},
			}
			vmb := &harvesterv1.VirtualMachineBackup{
				Spec: harvesterv1.VirtualMachineBackupSpec{Type: harvesterv1.Restic},
			}

			ready, err := h.reconcileVolumeRestores(vmr, vmb)

			require.Equal(t, tt.wantReady, ready)
			require.Len(t, re.calls, tt.wantCalls)
			require.Equal(t, tt.wantRetry, errors.Is(err, engine.ErrRetryLater))
			for _, wantErr := range tt.wantErrors {
				require.ErrorIs(t, err, wantErr)
			}
		})
	}
}

type fakeRestoreEngine struct {
	errors []error
	calls  []int
}

func (f *fakeRestoreEngine) Reconcile(
	_ *harvesterv1.VirtualMachineRestore,
	_ *harvesterv1.VirtualMachineBackup,
	index int,
) error {
	f.calls = append(f.calls, index)
	return f.errors[index]
}

func (f *fakeRestoreEngine) UpdateProgress(*harvesterv1.VolumeRestore) (int64, error) {
	return 0, nil
}

func (f *fakeRestoreEngine) Delete(*harvesterv1.VirtualMachineRestore, int) error {
	return nil
}

func (f *fakeRestoreEngine) RegisterWatchers(
	context.Context,
	func(namespace, name string),
) {
}

func TestUpdateStatusWaitsForVMStartup(t *testing.T) {
	tests := []struct {
		name       string
		startErr   error
		ready      bool
		keepHalted bool
		wantFailed bool
		wantDone   bool
	}{
		{name: "accepted start waits for readiness"},
		{name: "duplicate start waits for readiness", startErr: fmt.Errorf("start: %w", restorecommon.ErrVMStartInProgress)},
		{name: "real failure does not finalize", startErr: errors.New("start forbidden"), wantFailed: true},
		{name: "ready VM completes", ready: true, wantDone: true},
		{name: "halt after restore completes without starting", keepHalted: true, wantDone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &startupRestoreOperator{
				VMRestoreOperator: restorecommon.NewVMRestoreOperatorBuilder().Build(),
				startErr:          tt.startErr,
			}
			h := &RestoreHandler{context: context.Background(), vmro: op}
			vmr := &harvesterv1.VirtualMachineRestore{
				ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "default"},
				Spec:       harvesterv1.VirtualMachineRestoreSpec{HaltAfterRestore: tt.keepHalted},
			}
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "default"},
				Status:     kubevirtv1.VirtualMachineStatus{Ready: tt.ready},
			}
			err := h.updateStatus(vmr, vmr.DeepCopy(), vm, true)
			if tt.wantDone {
				require.NoError(t, err)
				require.Zero(t, op.starts)
				require.Equal(t, 1, op.cleanups)
				require.True(t, op.IsComplete(op.updated))
			} else {
				require.Error(t, err)
				require.Equal(t, 1, op.starts)
				require.Zero(t, op.cleanups)
				require.False(t, op.IsComplete(op.updated))
				if tt.wantFailed {
					require.ErrorIs(t, err, tt.startErr)
				} else {
					require.ErrorIs(t, err, engine.ErrRetryLater)
					require.True(t, op.IsProgressing(op.updated))
				}
			}
			require.Equal(t, tt.wantFailed, op.IsFailed(op.updated))
			require.Empty(t, vmr.Status.Conditions, "original informer object must not be mutated")
		})
	}
}

type startupRestoreOperator struct {
	restorecommon.VMRestoreOperator
	startErr error
	starts   int
	cleanups int
	updated  *harvesterv1.VirtualMachineRestore
}

func (o *startupRestoreOperator) StartVM(context.Context, *kubevirtv1.VirtualMachine) error {
	o.starts++
	return o.startErr
}

func (o *startupRestoreOperator) Update(
	_ *harvesterv1.VirtualMachineRestore,
	newVMr *harvesterv1.VirtualMachineRestore,
) (*harvesterv1.VirtualMachineRestore, error) {
	o.updated = newVMr.DeepCopy()
	return o.updated, nil
}

func (o *startupRestoreOperator) UpdateError(old *harvesterv1.VirtualMachineRestore, err error) error {
	o.updated = o.SetErrorCondition(old.DeepCopy(), err)
	return nil
}

func (o *startupRestoreOperator) DeleteOldPVCs(*harvesterv1.VirtualMachineRestore, *kubevirtv1.VirtualMachine) error {
	o.cleanups++
	return nil
}
