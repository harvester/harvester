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

	"github.com/stretchr/testify/require"
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
