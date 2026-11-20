package common

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	fakerest "k8s.io/client-go/rest/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

type startCase struct {
	name         string
	strategy     kubevirtv1.VirtualMachineRunStrategy
	ready        bool
	pending      bool
	vmiPhase     kubevirtv1.VirtualMachineInstancePhase
	response     *apierrors.StatusError
	wantCalls    int
	wantInFlight bool
	wantError    bool
}

func TestGetRestorePVCNameUsesSafeName(t *testing.T) {
	short := &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{UID: "uid"},
		Spec: harvesterv1.VirtualMachineRestoreSpec{
			VirtualMachineBackupName: "backup",
		},
	}
	require.Equal(t, "restore-backup-uid-disk", getRestorePVCName(short, "disk"))

	long := short.DeepCopy()
	long.Spec.VirtualMachineBackupName = strings.Repeat("backup", 20)
	first := getRestorePVCName(long, "volume-a")
	second := getRestorePVCName(long, "volume-b")
	require.LessOrEqual(t, len(first), 63)
	require.NotEqual(t, first, second)
}

func TestBuildSecretRefNameUsesSafeName(t *testing.T) {
	vmro := NewVMRestoreOperatorBuilder().Build()
	require.Equal(t, "target-source", vmro.BuildSecretRefName("target", "source"))

	longName := strings.Repeat("secret", 20)
	first := vmro.BuildSecretRefName(longName, "source-a")
	second := vmro.BuildSecretRefName(longName, "source-b")
	require.LessOrEqual(t, len(first), 63)
	require.NotEqual(t, first, second)
}

func TestStartVM(t *testing.T) {
	tests := []startCase{
		{name: "ready", strategy: kubevirtv1.RunStrategyHalted, ready: true},
		{name: "start already queued", strategy: kubevirtv1.RunStrategyRerunOnFailure, pending: true, wantInFlight: true},
		{name: "always starts automatically", strategy: kubevirtv1.RunStrategyAlways},
		{name: "halted", strategy: kubevirtv1.RunStrategyHalted, wantCalls: 1},
		{name: "manual without VMI", strategy: kubevirtv1.RunStrategyManual, wantCalls: 1},
		{name: "rerun without VMI", strategy: kubevirtv1.RunStrategyRerunOnFailure, wantCalls: 1},
		{name: "manual VMI already starting", strategy: kubevirtv1.RunStrategyManual, vmiPhase: kubevirtv1.Pending},
		{name: "rerun VMI already starting", strategy: kubevirtv1.RunStrategyRerunOnFailure, vmiPhase: kubevirtv1.Pending},
		{
			name: "rerun completed VMI", strategy: kubevirtv1.RunStrategyRerunOnFailure,
			vmiPhase: kubevirtv1.Succeeded, wantCalls: 1,
		},
		{
			name: "stale cache duplicate start", strategy: kubevirtv1.RunStrategyRerunOnFailure,
			wantCalls: 1, wantInFlight: true,
			response: apierrors.NewInternalError(errors.New("unable to complete request: stop/start already underway")),
		},
		{
			name: "stale cache running VMI", strategy: kubevirtv1.RunStrategyRerunOnFailure,
			wantCalls: 1, wantInFlight: true,
			response: startConflict("VM is already running"),
		},
		{
			name: "concurrent state patch", strategy: kubevirtv1.RunStrategyHalted, wantCalls: 1, wantInFlight: true,
			response: startConflict("jsonpatch test operation does not apply"),
		},
		{
			name: "unrelated conflict remains an error", strategy: kubevirtv1.RunStrategyManual, wantCalls: 1, wantError: true,
			response: startConflict("VM recovery required"),
		},
		{
			name: "permission failure", strategy: kubevirtv1.RunStrategyHalted, wantCalls: 1, wantError: true,
			response: apierrors.NewForbidden(kubevirtv1.Resource("virtualmachine"), "target", errors.New("not authorized")),
		},
		{
			name: "unrelated internal failure", strategy: kubevirtv1.RunStrategyHalted, wantCalls: 1, wantError: true,
			response: apierrors.NewInternalError(errors.New("API unavailable")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { runStartCase(t, tt) })
	}
}

func startConflict(message string) *apierrors.StatusError {
	return apierrors.NewConflict(kubevirtv1.Resource("virtualmachine"), "target", errors.New(message))
}

func runStartCase(t *testing.T, tt startCase) {
	t.Helper()
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "default"},
		Spec:       kubevirtv1.VirtualMachineSpec{RunStrategy: ptr.To(tt.strategy)},
		Status:     kubevirtv1.VirtualMachineStatus{Ready: tt.ready},
	}
	if tt.pending {
		vm.Status.StateChangeRequests = []kubevirtv1.VirtualMachineStateChangeRequest{{
			Action: kubevirtv1.StartRequest,
		}}
	}
	clientset := fake.NewSimpleClientset()
	if tt.vmiPhase != "" {
		createVMI(t, clientset, vm, tt.vmiPhase)
	}
	var calls int
	vmro := NewVMRestoreOperatorBuilder().
		WithVMICache(fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances)).
		WithVirtSubresourceRestClient(startRESTClient(t, tt.response, &calls)).Build()
	err := vmro.StartVM(context.Background(), vm)
	require.Equal(t, tt.wantCalls, calls)
	switch {
	case tt.wantInFlight:
		require.ErrorIs(t, err, ErrVMStartInProgress)
	case tt.wantError:
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrVMStartInProgress)
	default:
		require.NoError(t, err)
	}
}

func createVMI(
	t *testing.T,
	clientset *fake.Clientset,
	vm *kubevirtv1.VirtualMachine,
	phase kubevirtv1.VirtualMachineInstancePhase,
) {
	t.Helper()
	vmi := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: vm.ObjectMeta,
		Status:     kubevirtv1.VirtualMachineInstanceStatus{Phase: phase},
	}
	_, err := clientset.KubevirtV1().VirtualMachineInstances(vm.Namespace).Create(
		context.Background(), vmi, metav1.CreateOptions{},
	)
	require.NoError(t, err)
}

func startRESTClient(t *testing.T, response *apierrors.StatusError, calls *int) *fakerest.RESTClient {
	t.Helper()
	return &fakerest.RESTClient{
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		GroupVersion: schema.GroupVersion{
			Group: kubevirtv1.SubresourceGroupName, Version: kubevirtv1.ApiLatestVersion,
		},
		Client: fakerest.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
			*calls++
			require.Equal(t, http.MethodPut, r.Method)
			require.Contains(t, r.URL.Path, "/namespaces/default/virtualmachines/target/start")
			return startResponse(t, response), nil
		}),
	}
}

func startResponse(t *testing.T, response *apierrors.StatusError) *http.Response {
	t.Helper()
	code := http.StatusAccepted
	var body []byte
	if response != nil {
		status := response.ErrStatus
		status.Kind, status.APIVersion = "Status", "v1"
		var err error
		body, err = json.Marshal(status)
		require.NoError(t, err)
		code = int(status.Code)
	}
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func TestStartVMPreservesVMICacheErrors(t *testing.T) {
	strategies := []kubevirtv1.VirtualMachineRunStrategy{
		kubevirtv1.RunStrategyManual,
		kubevirtv1.RunStrategyRerunOnFailure,
	}
	for _, strategy := range strategies {
		t.Run(string(strategy), func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			cacheErr := errors.New("cache unavailable")
			clientset.PrependReactor("get", "virtualmachineinstances", func(clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, cacheErr
			})
			vmro := NewVMRestoreOperatorBuilder().
				WithVMICache(fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances)).Build()
			err := vmro.StartVM(context.Background(), &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "default"},
				Spec:       kubevirtv1.VirtualMachineSpec{RunStrategy: ptr.To(strategy)},
			})
			require.ErrorIs(t, err, cacheErr)
		})
	}
}

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
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "source-vm", Namespace: "default"},
				Spec:       kubevirtv1.VirtualMachineSpec{Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{}},
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
			client := fake.NewSimpleClientset(vm)
			client.PrependReactor("get", "virtualmachines", func(action clienttesting.Action) (bool, runtime.Object, error) {
				assert.Equal(t, vm.Namespace, action.GetNamespace())
				assert.Equal(t, vm.Name, action.(clienttesting.GetAction).GetName())
				require.False(t, tt.newVM)
				require.NotEqual(t, harvesterv1.VirtualMachineRestoreRetain, tt.deletionPolicy)
				if failure == "lookup" {
					return true, nil, errors.New("VM lookup failed")
				}
				return false, nil, nil
			})
			client.PrependReactor("update", "virtualmachinerestores", func(action clienttesting.Action) (bool, runtime.Object, error) {
				updates++
				obj := action.(clienttesting.UpdateAction).GetObject().(*harvesterv1.VirtualMachineRestore)
				require.True(t, reader.IsVolumesStatusInitialized(obj), "save the condition together with the lists")
				if failure == "update" {
					return true, nil, errors.New("status update failed")
				}
				// Model API serialization, which omits empty volume lists.
				data, err := json.Marshal(obj)
				require.NoError(t, err)
				persisted = &harvesterv1.VirtualMachineRestore{}
				require.NoError(t, json.Unmarshal(data, persisted))
				return true, persisted, nil
			})
			op := NewVMRestoreOperatorBuilder().
				WithClient(fakeclients.VMRestoreClient(client.HarvesterhciV1beta1().VirtualMachineRestores)).
				WithVMCache(fakeclients.VirtualMachineCache(client.KubevirtV1().VirtualMachines)).
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
