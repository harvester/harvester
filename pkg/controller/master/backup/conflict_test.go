package backup

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	backupengine "github.com/harvester/harvester/pkg/backup/engine"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	restoreengine "github.com/harvester/harvester/pkg/restore/engine"
)

func conflictError() error {
	return apierrors.NewConflict(schema.GroupResource{Resource: "jobs"}, "transfer", errors.New("updated"))
}

type reconcileCase struct {
	name   string
	err    error
	failed bool
}

func reconcileCases(pending error) []reconcileCase {
	failure := errors.New("transfer failed")
	conflict := conflictError()
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "jobs"}, "transfer", failure)
	return []reconcileCase{
		{name: "conflict", err: conflictError()},
		{name: "wrapped conflict", err: fmt.Errorf("resume: %w", conflictError())},
		{name: "wrapped pending", err: fmt.Errorf("prepare: %w", pending)},
		{name: "joined retries", err: errors.Join(pending, conflict)},
		{name: "wrapped joined retries", err: fmt.Errorf("resume: %w", errors.Join(pending, conflict))},
		{name: "failure", err: failure, failed: true},
		{name: "failure and conflict", err: errors.Join(failure, conflict), failed: true},
		{name: "conflict and failure", err: errors.Join(conflict, failure), failed: true},
		{name: "pending and failure", err: errors.Join(pending, failure), failed: true},
		{name: "multiple wrapped causes", err: fmt.Errorf("resume: %w: %w", pending, failure), failed: true},
		{name: "nested failure", err: fmt.Errorf("resume: %w", errors.Join(conflict, errors.Join(pending, failure))), failed: true},
		{name: "conflict and forbidden", err: errors.Join(conflict, forbidden), failed: true},
		{name: "forbidden and conflict", err: errors.Join(forbidden, conflict), failed: true},
	}
}

func TestBackupRequeuesConflicts(t *testing.T) {
	for _, backupType := range []harvesterv1.BackupType{
		harvesterv1.Restic, harvesterv1.Kopia, harvesterv1.Backup, harvesterv1.Snapshot,
	} {
		for _, tc := range reconcileCases(backupengine.ErrRetryLater) {
			t.Run(string(backupType)+"/"+tc.name, func(t *testing.T) {
				h, engine, vmb, client := completionFixture(t, backupType)
				vmb.Status.VolumeBackups = append(vmb.Status.VolumeBackups,
					harvesterv1.VolumeBackup{Name: ptr.To("data"), VolumeSize: 100})
				engine.reconcile = func(vmb *harvesterv1.VirtualMachineBackup, index int) error {
					if index == 0 {
						vmb.Status.VolumeBackups[index].Progress = 100
						vmb.Status.VolumeBackups[index].ReadyToUse = ptr.To(true)
						return nil
					}
					vmb.Status.VolumeBackups[index].Progress = 50
					return tc.err
				}
				before := vmb.DeepCopy()
				_, reconcileErr := h.OnBackupChange("", vmb)
				require.Equal(t, before, vmb)
				current, err := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).
					Get(context.Background(), vmb.Name, metav1.GetOptions{})
				require.NoError(t, err)
				queued := h.vmbController.(*requeueBackupController).enqueues
				if tc.failed {
					require.ErrorIs(t, reconcileErr, tc.err)
					require.Equal(t, tc.err.Error(), *current.Status.Error.Message)
					require.Empty(t, queued)
					return
				}
				require.NoError(t, reconcileErr)
				require.Nil(t, current.Status.Error)
				require.False(t, ptr.Deref(current.Status.ReadyToUse, false))
				require.Equal(t, 75, current.Status.Progress)
				require.True(t, *current.Status.VolumeBackups[0].ReadyToUse, "preserve completed uploads while retrying")
				require.Equal(t, []enqueueCall{{vmb.Namespace, vmb.Name, engineRetryDelay}}, queued)
			})
		}
	}
}

func TestBackupRequeuesUpdateConflicts(t *testing.T) {
	for _, stage := range []string{"preparation", "status"} {
		t.Run(stage, func(t *testing.T) {
			h, engine, vmb, client := completionFixture(t, harvesterv1.Restic)
			engine.reconcile = func(*harvesterv1.VirtualMachineBackup, int) error { return backupengine.ErrRetryLater }
			if stage == "preparation" {
				h.vmbo = completionOperator{VMBackupOperator: h.vmbo, buildErr: conflictError()}
			} else {
				client.PrependReactor("update", "virtualmachinebackups",
					func(clienttesting.Action) (bool, runtime.Object, error) {
						return true, nil, conflictError()
					})
			}
			_, err := h.OnBackupChange("", vmb)
			require.NoError(t, err)
			current, err := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).
				Get(context.Background(), vmb.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, vmb.Status, current.Status, "conflict must not persist an error condition")
			require.Len(t, h.vmbController.(*requeueBackupController).enqueues, 1)
		})
	}
}

type startupBackupOperator struct {
	backupcommon.VMBackupOperator
	lookupErr error
	writeErr  error
}

func (o startupBackupOperator) ResolveSourceVM(*harvesterv1.VirtualMachineBackup) (*kubevirtv1.VirtualMachine, error) {
	return &kubevirtv1.VirtualMachine{}, o.lookupErr
}

func (o startupBackupOperator) InitVMBackup(*harvesterv1.VirtualMachineBackup, *kubevirtv1.VirtualMachine) error {
	return o.writeErr
}

func (o startupBackupOperator) ConfigureCSISnapClasses(
	*harvesterv1.VirtualMachineBackup,
) (*harvesterv1.VirtualMachineBackup, error) {
	return nil, o.writeErr
}

func TestBackupSetupErrors(t *testing.T) {
	for _, stage := range []string{"source lookup", "initialization", "preparation"} {
		for _, tc := range reconcileCases(backupengine.ErrRetryLater) {
			t.Run(stage+"/"+tc.name, func(t *testing.T) {
				h, _, vmb, client := completionFixture(t, harvesterv1.Restic)
				storedStatus := vmb.DeepCopy().Status
				failBackupSetup(h, vmb, stage, tc.err)
				before := vmb.DeepCopy()
				reconcileErr := h.reconcileBackup(vmb)
				require.Equal(t, before, vmb, "informer objects must not be mutated")
				current, err := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).
					Get(context.Background(), vmb.Name, metav1.GetOptions{})
				require.NoError(t, err)
				queued := h.vmbController.(*requeueBackupController).enqueues
				if tc.failed {
					require.ErrorIs(t, reconcileErr, tc.err)
					require.Equal(t, tc.err.Error(), *current.Status.Error.Message)
					require.Empty(t, queued)
					return
				}
				require.NoError(t, reconcileErr)
				require.Equal(t, storedStatus, current.Status, "retryable errors must not be persisted as failures")
				require.Equal(t, []enqueueCall{{vmb.Namespace, vmb.Name, engineRetryDelay}}, queued)
			})
		}
	}
}

func failBackupSetup(h *Handler, vmb *harvesterv1.VirtualMachineBackup, stage string, err error) {
	if stage == "preparation" {
		h.vmbo = completionOperator{VMBackupOperator: h.vmbo, buildErr: err}
		return
	}
	vmb.Status.SourceSpec = nil
	op := startupBackupOperator{VMBackupOperator: h.vmbo, writeErr: err}
	if stage == "source lookup" {
		op.lookupErr = err
	}
	h.vmbo = op
}

func TestReadyBackupRequeuesConflict(t *testing.T) {
	h, _, vmb, client := completionFixture(t, harvesterv1.Restic)
	h.vmbo = startupBackupOperator{VMBackupOperator: h.vmbo, writeErr: conflictError()}
	storedStatus := vmb.DeepCopy().Status
	vmb.Status.ReadyToUse = ptr.To(true)
	require.NoError(t, h.reconcileBackup(vmb))
	current, err := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).
		Get(context.Background(), vmb.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, storedStatus, current.Status, "conflict must not persist an error condition")
	require.Equal(t, []enqueueCall{{vmb.Namespace, vmb.Name, engineRetryDelay}},
		h.vmbController.(*requeueBackupController).enqueues)
}

type retryRestoreClient struct {
	ctlharvesterv1.VirtualMachineRestoreClient
	current   *harvesterv1.VirtualMachineRestore
	updateErr error
}

func (c *retryRestoreClient) Update(vmr *harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineRestore, error) {
	if c.updateErr != nil {
		return nil, c.updateErr
	}
	c.current = vmr.DeepCopy()
	return c.current, nil
}

type retryRestoreOperator struct {
	restorecommon.VMRestoreOperator
	vmb        *harvesterv1.VirtualMachineBackup
	vm         *kubevirtv1.VirtualMachine
	lookupErr  error
	volumesErr error
	startErr   error
	cleanupErr error
	starts     int
	cleanups   int
}

func (o *retryRestoreOperator) ResolveVMBackup(*harvesterv1.VirtualMachineRestore) (*harvesterv1.VirtualMachineBackup, error) {
	return o.vmb, o.lookupErr
}

func (o *retryRestoreOperator) ResolveTargetVM(*harvesterv1.VirtualMachineRestore) (*kubevirtv1.VirtualMachine, error) {
	return o.vm, nil
}

func (o *retryRestoreOperator) InitVolumesStatus(
	*harvesterv1.VirtualMachineRestore, *harvesterv1.VirtualMachineBackup,
) error {
	return o.volumesErr
}

func (o *retryRestoreOperator) RestoreSecrets(
	*harvesterv1.VirtualMachineRestore, *harvesterv1.VirtualMachineBackup, *kubevirtv1.VirtualMachine,
) error {
	return nil
}

func (o *retryRestoreOperator) StartVM(context.Context, *kubevirtv1.VirtualMachine) error {
	o.starts++
	return o.startErr
}

func (o *retryRestoreOperator) DeleteOldPVCs(*harvesterv1.VirtualMachineRestore, *kubevirtv1.VirtualMachine) error {
	o.cleanups++
	return o.cleanupErr
}

type retryRestoreController struct {
	ctlharvesterv1.VirtualMachineRestoreController
	enqueues []enqueueCall
}

func (c *retryRestoreController) EnqueueAfter(namespace, name string, delay time.Duration) {
	c.enqueues = append(c.enqueues, enqueueCall{namespace, name, delay})
}

func TestRestoreRequeuesConflicts(t *testing.T) {
	for _, backupType := range []harvesterv1.BackupType{
		harvesterv1.Restic, harvesterv1.Kopia, harvesterv1.Backup, harvesterv1.Snapshot,
	} {
		for _, tc := range reconcileCases(restoreengine.ErrRetryLater) {
			t.Run(string(backupType)+"/"+tc.name, func(t *testing.T) {
				h, engine, client := retryRestoreFixture(backupType)
				engine.errors = []error{tc.err}
				before := client.current.DeepCopy()
				_, err := h.RestoreOnChanged("", client.current)
				queued := h.vmrController.(*retryRestoreController).enqueues
				if tc.failed {
					require.ErrorIs(t, err, tc.err)
					require.True(t, h.vmro.IsFailed(client.current))
					require.Empty(t, queued)
					return
				}
				require.NoError(t, err)
				require.False(t, h.vmro.IsFailed(client.current))
				require.False(t, h.vmro.IsComplete(client.current))
				require.Equal(t, []enqueueCall{{before.Namespace, before.Name, restoreProgressPoll}}, queued)
				require.Equal(t, 50, client.current.Status.Progress)
				assertRestoreResumes(t, h, engine, client.current)
			})
		}
	}
}

func TestRestoreRequeuesUpdateConflicts(t *testing.T) {
	for _, stage := range []string{"lookup", "status"} {
		t.Run(stage, func(t *testing.T) {
			h, engine, client := retryRestoreFixture(harvesterv1.Restic)
			engine.errors = []error{restoreengine.ErrRetryLater}
			if stage == "lookup" {
				h.vmro.(*retryRestoreOperator).lookupErr = conflictError()
			} else {
				client.updateErr = conflictError()
			}
			before := client.current.DeepCopy()
			_, err := h.RestoreOnChanged("", client.current)
			require.NoError(t, err)
			require.Equal(t, before, client.current, "conflict must not persist an error condition")
			require.Equal(t, []enqueueCall{{before.Namespace, before.Name, restoreProgressPoll}},
				h.vmrController.(*retryRestoreController).enqueues)

			h.vmro.(*retryRestoreOperator).lookupErr = nil
			client.updateErr = nil
			assertRestoreResumes(t, h, engine, client.current)
		})
	}
}

func assertRestoreResumes(
	t *testing.T,
	h *RestoreHandler,
	engine *fakeRestoreEngine,
	vmr *harvesterv1.VirtualMachineRestore,
) {
	t.Helper()
	// A retry must still reach the engine, rather than stop at IsFailed.
	engine.calls = nil
	engine.errors = []error{restoreengine.ErrRetryLater}
	_, err := h.RestoreOnChanged("", vmr)
	require.NoError(t, err)
	require.Equal(t, []int{0}, engine.calls)
}

func retryRestoreFixture(backupType harvesterv1.BackupType) (*RestoreHandler, *fakeRestoreEngine, *retryRestoreClient) {
	vmr := &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name: "restore", Namespace: "default",
			Finalizers:  []string{"wrangler.cattle.io/" + restoreControllerName},
			Annotations: map[string]string{restorecommon.BackupTypeAnnotation: string(backupType)},
		},
		Spec: harvesterv1.VirtualMachineRestoreSpec{NewVM: true},
		Status: harvesterv1.VirtualMachineRestoreStatus{
			Complete: ptr.To(false),
			VolumeRestores: []harvesterv1.VolumeRestore{{
				VolumeName: "disk", VolumeSize: 100,
			}},
		},
	}
	client := &retryRestoreClient{current: vmr}
	op := &retryRestoreOperator{
		VMRestoreOperator: restorecommon.NewVMRestoreOperatorBuilder().WithClient(client).Build(),
		vmb:               &harvesterv1.VirtualMachineBackup{Spec: harvesterv1.VirtualMachineBackupSpec{Type: backupType}},
	}
	engine := &fakeRestoreEngine{progress: 50}
	h := &RestoreHandler{
		vmro: op, vmbo: backupcommon.NewVMBackupOperatorBuilder().Build(),
		vmrController: &retryRestoreController{},
		engines:       map[harvesterv1.BackupType]restoreengine.RestoreEngine{backupType: engine},
	}
	return h, engine, client
}
