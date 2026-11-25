package vm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/rancher/wrangler/v3/pkg/slice"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

// cleanupBackoff paces the attempts of setRemovedPVCs. It waits for controllers that act on a new
// VM, so how long they take depends on the load of the cluster. retry.DefaultBackoff (10 ms start,
// 310 ms in total) is too short for that.
//   - Start at 100 ms and double: a controller that is quick is noticed early, a slow one is given
//     more time with every attempt and without polling the API server in a tight loop.
//   - 6 attempts, about 3 s in total: enough room for a busy cluster. An HTTP client still waits for
//     the answer, and the request then fails with 409, so that the caller can try again.
//   - 10 % jitter, so that concurrent requests do not hit the API server at the same moment.
var cleanupBackoff = wait.Backoff{Duration: 100 * time.Millisecond, Factor: 2, Jitter: 0.1, Steps: 6}

var (
	errCleanupFinalizerMissing = errors.New("the VirtualMachine has no cleanup finalizer yet")
	errPVCNotFound             = errors.New("the PVC of the disk does not exist")
	errDeletionInProgress      = errors.New("the VirtualMachine is already being deleted")
)

type vmStore struct {
	types.Store

	vms      ctlkubevirtv1.VirtualMachineClient
	pvcs     v1.PersistentVolumeClaimClient
	pvcCache v1.PersistentVolumeClaimCache
}

func (s *vmStore) Delete(request *types.APIRequest, _ *types.APISchema, id string) (types.APIObject, error) {
	removedDisks := request.Query["removedDisks"]
	vmID := util.NamespacedName(request.Namespace, request.Name)

	// Set removed PVCs in annotations. VMController is in charge of the cleanup.
	if err := s.setRemovedPVCs(request.Context(), request.Namespace, request.Name, removedDisks); err != nil {
		if apierrors.IsNotFound(err) {
			return types.APIObject{}, apierror.NewAPIError(validation.NotFound, fmt.Sprintf("VirtualMachine %s not found", vmID))
		}
		if errors.Is(err, errDeletionInProgress) {
			return types.APIObject{}, apierror.NewAPIError(validation.Conflict, fmt.Sprintf("VirtualMachine %s is already being deleted, its volumes to remove cannot be selected anymore", vmID))
		}
		if apierrors.IsConflict(err) || errors.Is(err, errCleanupFinalizerMissing) || errors.Is(err, errPVCNotFound) {
			return types.APIObject{}, apierror.NewAPIError(validation.Conflict, fmt.Sprintf("VirtualMachine %s is not ready to be deleted yet, retry later", vmID))
		}
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to set removedPersistentVolumeClaims to VirtualMachine %s, %v", vmID, err))
	}

	apiObj, err := s.Store.Delete(request, request.Schema, id)
	if err != nil {
		// Steve answers a VM that is already gone with the 204 ErrorCode, that is a success.
		var ec validation.ErrorCode
		if errors.As(err, &ec) && ec.Status == http.StatusNoContent {
			return types.APIObject{}, ec
		}
		// The VM is gone between the annotation and the delete.
		var apiErr *apierror.APIError
		if apierrors.IsNotFound(err) || (errors.As(err, &apiErr) && apiErr.Code.Status == http.StatusNotFound) {
			return types.APIObject{}, apierror.NewAPIError(validation.NotFound, fmt.Sprintf("VirtualMachine %s not found", vmID))
		}
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to remove VirtualMachine %s, %v", vmID, err))
	}

	return apiObj, nil
}

// setRemovedPVCs reads the VM from the API server, not from the cache: in the first seconds after
// its creation the controllers keep changing the VM, so the cached copy is stale and the update
// conflicts. The PVCs of removedDisks are resolved from the VM that is updated, in every attempt,
// because the cleanup handler deletes every PVC of the annotation without looking at the VM again.
// It also waits until Wrangler has added the finalizer of the cleanup handler: a VM deleted before
// that is gone at once, and the PVCs of the annotation would be left behind. The controller creates
// the PVCs of a new VM too, so a PVC that does not exist yet is retried as well. The UID of the VM is
// pinned: a VM of the same name that replaces it between two attempts is not the one of the request.
// A VM that is being deleted already is not waited for and its selection is not replaced: Wrangler adds
// no finalizer to it anymore, and the selection of an earlier request may not be cleaned up yet. A
// request that is cancelled stops before the next read and before the write.
func (s *vmStore) setRemovedPVCs(ctx context.Context, namespace, name string, removedDisks []string) error {
	retriable := func(err error) bool {
		return apierrors.IsConflict(err) || errors.Is(err, errCleanupFinalizerMissing) || errors.Is(err, errPVCNotFound)
	}
	var uid string
	return retry.OnError(cleanupBackoff, retriable, func() error {
		// A request that is cancelled is not retried and writes nothing. No %w: retry.OnError takes
		// context.Canceled for an interrupted wait and answers the last error, which can be nil.
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("the request is cancelled: %v", err)
		}
		vm, err := s.vms.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if uid != "" && uid != string(vm.UID) {
			return apierrors.NewNotFound(kubevirtv1.Resource("virtualmachines"), name)
		}
		uid = string(vm.UID)

		removedPVCs, err := s.getRemovedPVCs(vm, removedDisks)
		if errors.Is(err, errPVCNotFound) && vm.DeletionTimestamp != nil {
			// The cleanup removed it or the controller creates no PVC anymore: nothing to wait for.
			return errDeletionInProgress
		}
		if err != nil {
			return err
		}
		hasFinalizer := util.ContainsFinalizer(vm, util.GetWranglerFinalizerName(util.VMCleanupPVCAndSnapshotControllerName))
		selection := strings.Join(removedPVCs, ",")
		if vm.DeletionTimestamp != nil {
			// An empty selection that is set is a choice too: keep the PVCs.
			current, selected := vm.Annotations[util.RemovedPVCsAnnotationKey]
			if (len(removedPVCs) > 0 && !hasFinalizer) || (selected && current != selection) {
				return errDeletionInProgress
			}
		} else if len(removedPVCs) > 0 && !hasFinalizer {
			return errCleanupFinalizerMissing
		}

		if err := ctx.Err(); err != nil {
			return fmt.Errorf("the request is cancelled: %v", err)
		}
		vmCopy := vm.DeepCopy()
		if vmCopy.Annotations == nil {
			vmCopy.Annotations = map[string]string{}
		}
		vmCopy.Annotations[util.RemovedPVCsAnnotationKey] = selection
		_, err = s.vms.Update(vmCopy)
		return err
	})
}

// getRemovedPVCs returns the PVCs behind the removed disks of the VM, without the golden images.
func (s *vmStore) getRemovedPVCs(vm *kubevirtv1.VirtualMachine, removedDisks []string) ([]string, error) {
	var removedPVCs []string
	if vm.Spec.Template == nil {
		return removedPVCs, nil
	}

	for _, vol := range vm.Spec.Template.Spec.Volumes {
		if vol.PersistentVolumeClaim == nil || !slice.ContainsString(removedDisks, vol.Name) {
			continue
		}

		ok, err := s.isPVCGoldenImage(vm.Namespace, vol.PersistentVolumeClaim.ClaimName)
		if err != nil {
			// A missing PVC must not be taken for a missing VM by the caller, so no %w of the NotFound error.
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("%w: disk %s of VirtualMachine %s, %v", errPVCNotFound, vol.Name, util.NamespacedName(vm.Namespace, vm.Name), err)
			}
			return nil, fmt.Errorf("failed to look up the PVC of disk %s of VirtualMachine %s: %v", vol.Name, util.NamespacedName(vm.Namespace, vm.Name), err)
		}
		if !ok {
			removedPVCs = append(removedPVCs, vol.PersistentVolumeClaim.ClaimName)
		}
	}
	return removedPVCs, nil
}

// vm-import-controller allows golden images to be used natively as pvc with a vm
// during a delete operation we need to skip such pvc's as they are managed by the
// vmimage, and deletion of the vmimage will remove the pvc
func (s *vmStore) isPVCGoldenImage(namespace, name string) (bool, error) {
	pvcObj, err := s.pvcCache.Get(namespace, name)
	if apierrors.IsNotFound(err) {
		// The controller creates the PVCs of a new VM, the cache can lag behind.
		pvcObj, err = s.pvcs.Get(namespace, name, metav1.GetOptions{})
	}
	if err != nil {
		return false, fmt.Errorf("error looking up pvc %s: %w", util.NamespacedName(namespace, name), err)
	}

	if pvcObj.Annotations == nil {
		return false, nil
	}
	if pvcObj.Annotations[util.AnnotationGoldenImage] == "true" {
		return true, nil
	}

	return false, nil
}
