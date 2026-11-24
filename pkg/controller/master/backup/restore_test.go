package backup

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
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
		{
			name:       "joined failure is not treated as pending",
			errors:     []error{errors.Join(engine.ErrRetryLater, errBoot), nil},
			wantErrors: []error{engine.ErrRetryLater, errBoot},
			wantCalls:  1,
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
			require.Equal(t, tt.wantRetry, isRestoreRetry(err))
			for _, wantErr := range tt.wantErrors {
				require.ErrorIs(t, err, wantErr)
			}
		})
	}
}

type fakeRestoreEngine struct {
	errors         []error
	progress       int64
	volumeProgress map[string]int64
	calls          []int
	deletes        int
	deleteErr      error
}

func (f *fakeRestoreEngine) Reconcile(
	_ *harvesterv1.VirtualMachineRestore,
	_ *harvesterv1.VirtualMachineBackup,
	index int,
) error {
	f.calls = append(f.calls, index)
	return f.errors[index]
}

func (f *fakeRestoreEngine) Delete(*harvesterv1.VirtualMachineRestore) error {
	f.deletes++
	return f.deleteErr
}

func (f *fakeRestoreEngine) UpdateProgress(_ *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore) (int64, error) {
	if progress, ok := f.volumeProgress[vr.VolumeName]; ok {
		return progress, nil
	}
	return f.progress, nil
}

func (f *fakeRestoreEngine) RegisterWatchers(
	context.Context,
	func(namespace, name string),
) {
}

func TestUpdateStatusWaitsForVolumes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		retry bool
	}{
		{name: "saved", retry: true},
		{name: "conflict", err: conflictError(), retry: true},
		{name: "failure", err: errors.New("status unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, client := restoreVMFixture()
			client.updateErr = tc.err
			op := h.vmro.(*retryRestoreOperator)
			op.vm.Status.Ready = true
			err := h.updateStatus(client.current, client.current.DeepCopy(), op.vm, false)
			require.ErrorIs(t, err, engine.ErrRetryLater)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			}
			require.Equal(t, tc.retry, isRestoreRetry(err))
			require.Zero(t, op.starts)
			require.Zero(t, op.cleanups, "unfinished volumes must block completion even when the VM is ready")
			require.False(t, h.vmro.IsComplete(client.current))
			require.False(t, h.vmro.IsFailed(client.current))
		})
	}
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
		{name: "conflict waits for readiness", startErr: conflictError()},
		{name: "wrapped conflict waits for readiness", startErr: fmt.Errorf("start: %w", conflictError())},
		{name: "wrapped retry waits for readiness", startErr: fmt.Errorf("start: %w", engine.ErrRetryLater)},
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
				wantErr := tt.startErr
				if wantErr == nil {
					wantErr = engine.ErrRetryLater
				}
				require.ErrorIs(t, err, wantErr)
				if !tt.wantFailed {
					require.True(t, isRestoreRetry(err))
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
	startErr   error
	cleanupErr error
	updateErr  error
	starts     int
	cleanups   int
	updated    *harvesterv1.VirtualMachineRestore
}

func (o *startupRestoreOperator) StartVM(context.Context, *kubevirtv1.VirtualMachine) error {
	o.starts++
	return o.startErr
}

func (o *startupRestoreOperator) Update(
	_ *harvesterv1.VirtualMachineRestore,
	newVMr *harvesterv1.VirtualMachineRestore,
) (*harvesterv1.VirtualMachineRestore, error) {
	if o.updateErr != nil {
		return nil, o.updateErr
	}
	o.updated = newVMr.DeepCopy()
	return o.updated, nil
}

func (o *startupRestoreOperator) UpdateError(old *harvesterv1.VirtualMachineRestore, err error) error {
	_, updateErr := o.Update(old, o.SetErrorCondition(old.DeepCopy(), err))
	return errors.Join(err, updateErr)
}

func (o *startupRestoreOperator) DeleteOldPVCs(*harvesterv1.VirtualMachineRestore, *kubevirtv1.VirtualMachine) error {
	o.cleanups++
	return o.cleanupErr
}

func TestRestoreCleanupRetries(t *testing.T) {
	cleanupErr := errors.New("PVC deletion unavailable")
	updateErr := errors.New("status update unavailable")
	tests := []struct {
		name       string
		keepHalted bool
		updateErr  error
	}{
		{name: "ready VM"},
		{name: "halted VM", keepHalted: true},
		{name: "status update fails", updateErr: updateErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &startupRestoreOperator{
				VMRestoreOperator: restorecommon.NewVMRestoreOperatorBuilder().Build(),
				cleanupErr:        cleanupErr,
				updateErr:         tt.updateErr,
			}
			h := &RestoreHandler{vmro: op}
			vmr := &harvesterv1.VirtualMachineRestore{
				Spec: harvesterv1.VirtualMachineRestoreSpec{HaltAfterRestore: tt.keepHalted},
			}
			vm := &kubevirtv1.VirtualMachine{
				Status: kubevirtv1.VirtualMachineStatus{Ready: !tt.keepHalted},
			}
			original := vmr.DeepCopy()

			err := h.updateStatus(vmr, vmr.DeepCopy(), vm, true)
			require.ErrorIs(t, err, cleanupErr)
			require.Equal(t, original, vmr, "informer object must not be mutated")
			if tt.updateErr != nil {
				require.ErrorIs(t, err, updateErr)
				require.Nil(t, op.updated)
			} else {
				vmr = op.updated
			}

			// A repeated failure must remain retryable after persisting its condition.
			op.updateErr = nil
			err = h.updateStatus(vmr, vmr.DeepCopy(), vm, true)
			require.ErrorIs(t, err, cleanupErr)
			vmr = op.updated
			require.True(t, op.IsProgressing(vmr))
			require.False(t, op.IsFailed(vmr), "the reconcile guard must allow cleanup retries")
			require.False(t, op.IsComplete(vmr))
			for _, condition := range vmr.Status.Conditions {
				require.Contains(t, condition.Message, cleanupErr.Error())
				if condition.Type == harvesterv1.RestoreConditionProgressing {
					require.Equal(t, corev1.ConditionTrue, condition.Status)
				}
			}

			op.cleanupErr = nil
			err = h.updateStatus(vmr, vmr.DeepCopy(), vm, true)
			require.NoError(t, err)
			require.True(t, op.IsComplete(op.updated))
			require.False(t, op.IsFailed(op.updated))
			require.Equal(t, 3, op.cleanups)
			require.Zero(t, op.starts)
		})
	}
}

func TestRestoreDeletionUsesSavedType(t *testing.T) {
	for _, backupType := range []harvesterv1.BackupType{
		"", harvesterv1.Backup, harvesterv1.Snapshot, harvesterv1.Restic, harvesterv1.Kopia, "unknown",
	} {
		t.Run(string(backupType), func(t *testing.T) {
			engines := map[harvesterv1.BackupType]engine.RestoreEngine{}
			for _, kind := range []harvesterv1.BackupType{harvesterv1.Backup, harvesterv1.Snapshot, harvesterv1.Restic, harvesterv1.Kopia} {
				engines[kind] = &fakeRestoreEngine{}
			}
			// The operator has no source cache: deletion must never resolve VMBackup.
			h := &RestoreHandler{vmro: restorecommon.NewVMRestoreOperatorBuilder().Build(), engines: engines}
			vmr := &harvesterv1.VirtualMachineRestore{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{restorecommon.BackupTypeAnnotation: string(backupType)},
			}}
			_, err := h.RestoreOnRemove("", vmr)
			if backupType == "unknown" {
				require.ErrorContains(t, err, "unsupported restore engine")
			} else {
				require.NoError(t, err)
			}
			wantCalls := map[harvesterv1.BackupType]int{backupType: 1}
			if backupType == "" {
				wantCalls = map[harvesterv1.BackupType]int{
					harvesterv1.Snapshot: 1,
					harvesterv1.Backup:   1,
				}
			}
			for kind, re := range engines {
				require.Equal(t, wantCalls[kind], re.(*fakeRestoreEngine).deletes,
					"unexpected cleanup calls for %s", kind)
			}
		})
	}
}

func TestRestoreDeletionRetries(t *testing.T) {
	cleanupErr := errors.New("cleanup unavailable")
	re := &fakeRestoreEngine{deleteErr: cleanupErr}
	h := &RestoreHandler{
		vmro:    restorecommon.NewVMRestoreOperatorBuilder().Build(),
		engines: map[harvesterv1.BackupType]engine.RestoreEngine{harvesterv1.Restic: re},
	}
	vmr := &harvesterv1.VirtualMachineRestore{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{restorecommon.BackupTypeAnnotation: string(harvesterv1.Restic)},
	}}
	updated, err := h.RestoreOnRemove("", vmr)
	require.ErrorIs(t, err, cleanupErr, "retain the VMRestore finalizer until cleanup succeeds")
	require.Same(t, vmr, updated)
	re.deleteErr = nil
	updated, err = h.RestoreOnRemove("", vmr)
	require.NoError(t, err)
	require.Equal(t, 2, re.deletes)
	require.Same(t, vmr, updated)
}

type recordingOperator struct {
	restorecommon.VMRestoreOperator
	vmb       *harvesterv1.VirtualMachineBackup
	recorded  harvesterv1.BackupType
	recordErr error
	inits     int
}

func (o *recordingOperator) ResolveVMBackup(*harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineBackup, error) {
	return o.vmb, nil
}

func (o *recordingOperator) UpdateType(_ *harvesterv1.VirtualMachineRestore, kind harvesterv1.BackupType) error {
	o.recorded = kind
	return o.recordErr
}

func (o *recordingOperator) InitVolumesStatus(*harvesterv1.VirtualMachineRestore, *harvesterv1.VirtualMachineBackup) error {
	o.inits++
	return nil
}

func TestRecordTypeBeforeResources(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("update-fails=%t", fail), func(t *testing.T) {
			op := &recordingOperator{
				VMRestoreOperator: restorecommon.NewVMRestoreOperatorBuilder().Build(),
				vmb:               &harvesterv1.VirtualMachineBackup{Spec: harvesterv1.VirtualMachineBackupSpec{Type: harvesterv1.Kopia}},
			}
			if fail {
				op.recordErr = errors.New("API unavailable")
			}
			h := &RestoreHandler{vmro: op, vmbo: backupcommon.NewVMBackupOperatorBuilder().Build()}
			vmr := &harvesterv1.VirtualMachineRestore{
				ObjectMeta: metav1.ObjectMeta{Finalizers: []string{"wrangler.cattle.io/" + restoreControllerName}},
				Status:     harvesterv1.VirtualMachineRestoreStatus{Complete: ptr.To(false)},
			}
			_, err := h.RestoreOnChanged("", vmr)
			require.Equal(t, op.recordErr, err)
			require.Equal(t, harvesterv1.Kopia, op.recorded)
			require.Zero(t, op.inits, "stop before initializing volumes or creating engine resources")
			vmr.Annotations = map[string]string{restorecommon.BackupTypeAnnotation: string(harvesterv1.Kopia)}
			_, err = h.RestoreOnChanged("", vmr)
			require.NoError(t, err)
			require.Equal(t, 1, op.inits, "proceed only after observing the persisted type")
		})
	}
}

func TestRestoreWaitsForFinalizer(t *testing.T) {
	// No source cache or resource clients are available: reconciliation must stop
	// until the removal handler has successfully persisted its finalizer.
	h := &RestoreHandler{vmro: restorecommon.NewVMRestoreOperatorBuilder().Build()}
	vmr := &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			restorecommon.BackupTypeAnnotation: string(harvesterv1.Restic),
		}},
		Status: harvesterv1.VirtualMachineRestoreStatus{Complete: ptr.To(false)},
	}
	_, err := h.RestoreOnChanged("", vmr)
	require.NoError(t, err)
}
