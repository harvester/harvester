package vm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	k8stesting "k8s.io/client-go/testing"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

type recordingStore struct {
	types.Store
	deletedIDs []string
	deleteErr  error
}

func (s *recordingStore) Delete(_ *types.APIRequest, _ *types.APISchema, id string) (types.APIObject, error) {
	s.deletedIDs = append(s.deletedIDs, id)
	return types.APIObject{ID: id}, s.deleteErr
}

func newTestVM(namespace, name string, claimNames ...string) *kubevirtv1.VirtualMachine {
	volumes := make([]kubevirtv1.Volume, 0, len(claimNames))
	for _, claimName := range claimNames {
		volumes = append(volumes, kubevirtv1.Volume{
			Name: claimName,
			VolumeSource: kubevirtv1.VolumeSource{
				PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claimName,
					},
				},
			},
		})
	}

	return &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			UID:         "uid-1",
			Annotations: map[string]string{},
			Finalizers:  []string{util.GetWranglerFinalizerName(util.VMCleanupPVCAndSnapshotControllerName)},
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Volumes: volumes,
				},
			},
		},
	}
}

// newTestVMWithoutFinalizer returns a VM that Wrangler has not given the cleanup finalizer yet.
func newTestVMWithoutFinalizer(namespace, name string, claimNames ...string) *kubevirtv1.VirtualMachine {
	vm := newTestVM(namespace, name, claimNames...)
	vm.Finalizers = nil
	return vm
}

func newTestPVC(namespace, name string, annotations map[string]string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Annotations: annotations,
		},
	}
}

func TestVMStoreDelete(t *testing.T) {
	backoff := cleanupBackoff
	cleanupBackoff = wait.Backoff{Duration: time.Millisecond, Factor: 1, Steps: 4}
	t.Cleanup(func() { cleanupBackoff = backoff })

	const (
		namespace = "default"
		vmName    = "vm1"
	)

	tests := []struct {
		name                     string
		vm                       *kubevirtv1.VirtualMachine
		pvcs                     []*corev1.PersistentVolumeClaim
		removedDisks             []string
		expectErr                bool
		expectDeleted            bool
		expectRemovedPVCs        string
		expectAnnotationSet      bool
		expectErrStatus          int
		updateConflicts          int
		finalizerAtGet           int
		pvcAtGet                 int
		claimAfterConflict       string
		uidAfterConflict         string
		expectGets               int
		expectKeptSelection      string
		expectKeptEmpty          bool
		expectNoSelection        bool
		expectUID                string
		cancelled                bool
		terminatingAfterConflict string
		goneAtGet                int
		deleteErr                error
		expectNoContent          bool
		expectUpdates            int
	}{
		{
			name:            "vm not found",
			expectErr:       true,
			expectErrStatus: http.StatusNotFound,
		},
		{
			name: "no removed disks",
			vm:   newTestVM(namespace, vmName, "disk1", "disk2"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
				newTestPVC(namespace, "disk2", nil),
			},
			expectDeleted:       true,
			expectAnnotationSet: true,
			expectRemovedPVCs:   "",
		},
		{
			name: "removed disk is added to the annotation",
			vm:   newTestVM(namespace, vmName, "disk1", "disk2"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
				newTestPVC(namespace, "disk2", nil),
			},
			removedDisks:        []string{"disk2"},
			expectDeleted:       true,
			expectAnnotationSet: true,
			expectRemovedPVCs:   "disk2",
		},
		{
			name: "update conflicts are retried",
			vm:   newTestVM(namespace, vmName, "disk1"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:        []string{"disk1"},
			updateConflicts:     2,
			expectUpdates:       3,
			expectDeleted:       true,
			expectAnnotationSet: true,
			expectRemovedPVCs:   "disk1",
		},
		{
			name: "update conflicts that do not stop answer 409",
			vm:   newTestVM(namespace, vmName, "disk1"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:    []string{"disk1"},
			updateConflicts: 4,
			expectErr:       true,
			expectErrStatus: http.StatusConflict,
		},
		{
			name: "the delete waits for the cleanup finalizer",
			vm:   newTestVMWithoutFinalizer(namespace, vmName, "disk1"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:        []string{"disk1"},
			finalizerAtGet:      3,
			expectDeleted:       true,
			expectAnnotationSet: true,
			expectRemovedPVCs:   "disk1",
		},
		{
			name: "a VM that is gone while waiting is not found",
			vm:   newTestVMWithoutFinalizer(namespace, vmName, "disk1"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:    []string{"disk1"},
			goneAtGet:       3,
			expectErr:       true,
			expectErrStatus: http.StatusNotFound,
		},
		{
			name: "a VM without the cleanup finalizer is not deleted with its volumes",
			vm:   newTestVMWithoutFinalizer(namespace, vmName, "disk1"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:    []string{"disk1"},
			expectErr:       true,
			expectErrStatus: http.StatusConflict,
		},
		{
			name: "a VM without the cleanup finalizer is deleted without volumes",
			vm:   newTestVMWithoutFinalizer(namespace, vmName, "disk1"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			expectDeleted:       true,
			expectAnnotationSet: true,
			expectRemovedPVCs:   "",
		},
		{
			name: "a VM that is being deleted without the cleanup finalizer is not waited for",
			vm: func() *kubevirtv1.VirtualMachine {
				vm := newTestVMWithoutFinalizer(namespace, vmName, "disk1")
				vm.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				return vm
			}(),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:    []string{"disk1"},
			expectErr:       true,
			expectErrStatus: http.StatusConflict,
			expectGets:      1,
		},
		{
			name: "a VM that is being deleted keeps the selection of an earlier request",
			vm: func() *kubevirtv1.VirtualMachine {
				vm := newTestVM(namespace, vmName, "disk1", "disk2")
				vm.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				vm.Annotations[util.RemovedPVCsAnnotationKey] = "disk2"
				return vm
			}(),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
				newTestPVC(namespace, "disk2", nil),
			},
			removedDisks:        []string{"disk1"},
			expectErr:           true,
			expectErrStatus:     http.StatusConflict,
			expectGets:          1,
			expectKeptSelection: "disk2",
		},
		{
			name: "a request that loses the race against the delete of another request keeps its selection",
			vm:   newTestVM(namespace, vmName, "disk1", "disk2"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
				newTestPVC(namespace, "disk2", nil),
			},
			removedDisks:             []string{"disk1"},
			updateConflicts:          1,
			terminatingAfterConflict: "disk2",
			expectErr:                true,
			expectErrStatus:          http.StatusConflict,
			expectKeptSelection:      "disk2",
		},
		{
			name: "a VM that is being deleted with the cleanup finalizer is deleted with its volumes",
			vm: func() *kubevirtv1.VirtualMachine {
				vm := newTestVM(namespace, vmName, "disk1")
				vm.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				return vm
			}(),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:        []string{"disk1"},
			expectDeleted:       true,
			expectAnnotationSet: true,
			expectRemovedPVCs:   "disk1",
		},
		{
			name: "a VM that is gone after the delete is a success",
			vm:   newTestVM(namespace, vmName, "disk1"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:        []string{"disk1"},
			deleteErr:           validation.ErrorCode{Status: 204},
			expectNoContent:     true,
			expectDeleted:       true,
			expectAnnotationSet: true,
			expectRemovedPVCs:   "disk1",
		},
		{
			name: "other errors of the delete fail the request",
			vm:   newTestVM(namespace, vmName, "disk1"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:    []string{"disk1"},
			deleteErr:       errors.New("boom"),
			expectErr:       true,
			expectErrStatus: http.StatusInternalServerError,
		},
		{
			name: "a VM that is gone before the delete is not found, as Steve answers it",
			vm:   newTestVM(namespace, vmName, "disk1"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:    []string{"disk1"},
			deleteErr:       apierror.NewAPIError(validation.NotFound, "gone"),
			expectErr:       true,
			expectErrStatus: http.StatusNotFound,
		},
		{
			name: "a VM that is gone before the delete is not found, as Kubernetes answers it",
			vm:   newTestVM(namespace, vmName, "disk1"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:    []string{"disk1"},
			deleteErr:       apierrors.NewNotFound(schema.GroupResource{Resource: "virtualmachines"}, vmName),
			expectErr:       true,
			expectErrStatus: http.StatusNotFound,
		},
		{
			name: "golden image PVC is skipped",
			vm:   newTestVM(namespace, vmName, "disk1", "disk2"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", map[string]string{util.AnnotationGoldenImage: "true"}),
				newTestPVC(namespace, "disk2", nil),
			},
			removedDisks:        []string{"disk1", "disk2"},
			expectDeleted:       true,
			expectAnnotationSet: true,
			expectRemovedPVCs:   "disk2",
		},
		{
			name:            "a PVC that does not appear is answered with retry later",
			vm:              newTestVM(namespace, vmName, "disk1"),
			removedDisks:    []string{"disk1"},
			expectErr:       true,
			expectErrStatus: http.StatusConflict,
		},
		{
			name: "a missing PVC of a VM that is being deleted is not waited for",
			vm: func() *kubevirtv1.VirtualMachine {
				vm := newTestVM(namespace, vmName, "disk1")
				vm.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				return vm
			}(),
			removedDisks:    []string{"disk1"},
			expectErr:       true,
			expectErrStatus: http.StatusConflict,
			expectGets:      1,
		},
		{
			name: "a VM that is being deleted keeps the empty selection of an earlier request",
			vm: func() *kubevirtv1.VirtualMachine {
				vm := newTestVM(namespace, vmName, "disk1")
				vm.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				vm.Annotations[util.RemovedPVCsAnnotationKey] = ""
				return vm
			}(),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:    []string{"disk1"},
			expectErr:       true,
			expectErrStatus: http.StatusConflict,
			expectGets:      1,
			expectKeptEmpty: true,
		},
		{
			name: "a cancelled request reads and writes nothing",
			vm:   newTestVM(namespace, vmName, "disk1"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:      []string{"disk1"},
			cancelled:         true,
			expectErr:         true,
			expectErrStatus:   http.StatusInternalServerError,
			expectNoSelection: true,
		},
		{
			name:                "a PVC that does not exist yet is waited for",
			vm:                  newTestVM(namespace, vmName, "disk1"),
			removedDisks:        []string{"disk1"},
			pvcAtGet:            3,
			expectDeleted:       true,
			expectAnnotationSet: true,
			expectRemovedPVCs:   "disk1",
		},
		{
			name: "a VM that replaces the VM of the request is neither annotated nor deleted",
			vm:   newTestVM(namespace, vmName, "disk1"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
			},
			removedDisks:      []string{"disk1"},
			updateConflicts:   1,
			uidAfterConflict:  "uid-2",
			expectErr:         true,
			expectErrStatus:   http.StatusNotFound,
			expectUpdates:     1,
			expectUID:         "uid-2",
			expectNoSelection: true,
		},
		{
			name: "the PVCs are resolved from the VM that is updated",
			vm:   newTestVM(namespace, vmName, "disk1"),
			pvcs: []*corev1.PersistentVolumeClaim{
				newTestPVC(namespace, "disk1", nil),
				newTestPVC(namespace, "disk1-migrated", nil),
			},
			removedDisks:        []string{"disk1"},
			updateConflicts:     1,
			claimAfterConflict:  "disk1-migrated",
			expectDeleted:       true,
			expectAnnotationSet: true,
			expectRemovedPVCs:   "disk1-migrated",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			if tc.vm != nil {
				assert.NoError(t, clientset.Tracker().Add(tc.vm))
			}
			for _, pvc := range tc.pvcs {
				assert.NoError(t, clientset.Tracker().Add(pvc))
			}

			updates := 0
			clientset.PrependReactor("update", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				updates++
				if updates <= tc.updateConflicts {
					if tc.uidAfterConflict != "" {
						vm := tc.vm.DeepCopy()
						vm.UID = k8stypes.UID(tc.uidAfterConflict)
						assert.NoError(t, clientset.Tracker().Update(kubevirtv1.SchemeGroupVersion.WithResource("virtualmachines"), vm, namespace))
					}
					if tc.terminatingAfterConflict != "" {
						vm := tc.vm.DeepCopy()
						vm.DeletionTimestamp = &metav1.Time{Time: time.Now()}
						vm.Annotations[util.RemovedPVCsAnnotationKey] = tc.terminatingAfterConflict
						assert.NoError(t, clientset.Tracker().Update(kubevirtv1.SchemeGroupVersion.WithResource("virtualmachines"), vm, namespace))
					}
					if tc.claimAfterConflict != "" {
						vm := tc.vm.DeepCopy()
						vm.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = tc.claimAfterConflict
						assert.NoError(t, clientset.Tracker().Update(kubevirtv1.SchemeGroupVersion.WithResource("virtualmachines"), vm, namespace))
					}
					return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "virtualmachines"}, vmName, errors.New("stale"))
				}
				return false, nil, nil
			})
			gets := 0
			clientset.PrependReactor("get", "virtualmachines", func(_ k8stesting.Action) (bool, runtime.Object, error) {
				gets++
				if gets == tc.goneAtGet {
					return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "virtualmachines"}, vmName)
				}
				if gets == tc.pvcAtGet {
					assert.NoError(t, clientset.Tracker().Add(newTestPVC(namespace, "disk1", nil)))
				}
				if gets == tc.finalizerAtGet {
					vm := tc.vm.DeepCopy()
					vm.Finalizers = []string{util.GetWranglerFinalizerName(util.VMCleanupPVCAndSnapshotControllerName)}
					assert.NoError(t, clientset.Tracker().Update(kubevirtv1.SchemeGroupVersion.WithResource("virtualmachines"), vm, namespace))
				}
				return false, nil, nil
			})

			inner := &recordingStore{deleteErr: tc.deleteErr}
			store := &vmStore{
				Store:    inner,
				vms:      fakeclients.VirtualMachineClient(clientset.KubevirtV1().VirtualMachines),
				pvcs:     fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims),
				pvcCache: fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
			}

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			req := &types.APIRequest{
				Name:      vmName,
				Namespace: namespace,
				Query:     map[string][]string{"removedDisks": tc.removedDisks},
				Request:   httptest.NewRequest(http.MethodDelete, "/", nil).WithContext(ctx),
			}
			_, err := store.Delete(req, nil, util.NamespacedName(namespace, vmName))

			if tc.expectErr {
				assert.Error(t, err)
				if tc.expectErrStatus != 0 {
					var apiErr *apierror.APIError
					if assert.ErrorAs(t, err, &apiErr) {
						assert.Equal(t, tc.expectErrStatus, apiErr.Code.Status)
					}
				}
				if tc.deleteErr == nil {
					assert.Empty(t, inner.deletedIDs)
				} else {
					assert.Equal(t, []string{util.NamespacedName(namespace, vmName)}, inner.deletedIDs)
				}
				if tc.expectGets != 0 {
					assert.Equal(t, tc.expectGets, gets)
				}
				if tc.cancelled {
					assert.Zero(t, gets)
				}
				if tc.expectUpdates != 0 {
					assert.Equal(t, tc.expectUpdates, updates)
				}
				if tc.vm == nil {
					return
				}
				vm, getErr := clientset.KubevirtV1().VirtualMachines(namespace).Get(t.Context(), vmName, metav1.GetOptions{})
				assert.NoError(t, getErr)
				selection, found := vm.Annotations[util.RemovedPVCsAnnotationKey]
				if tc.expectKeptSelection != "" {
					assert.Equal(t, tc.expectKeptSelection, selection)
				}
				if tc.expectKeptEmpty {
					assert.True(t, found)
					assert.Empty(t, selection)
				}
				if tc.expectNoSelection {
					assert.False(t, found)
				}
				if tc.expectUID != "" {
					assert.Equal(t, k8stypes.UID(tc.expectUID), vm.UID)
				}
				return
			}
			if tc.expectNoContent {
				assert.Equal(t, tc.deleteErr, err)
			} else {
				assert.NoError(t, err)
			}
			if tc.expectUpdates != 0 {
				assert.Equal(t, tc.expectUpdates, updates)
			}

			if tc.expectDeleted {
				assert.Equal(t, []string{util.NamespacedName(namespace, vmName)}, inner.deletedIDs)
			}

			vm, err := clientset.KubevirtV1().VirtualMachines(namespace).Get(t.Context(), vmName, metav1.GetOptions{})
			assert.NoError(t, err)
			value, found := vm.Annotations[util.RemovedPVCsAnnotationKey]
			assert.Equal(t, tc.expectAnnotationSet, found)
			assert.Equal(t, tc.expectRemovedPVCs, value)
		})
	}
}
