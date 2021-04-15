package migration

import (
	"context"

	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	v1 "kubevirt.io/client-go/api/v1"

	ctlv1 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/rancher/harvester/pkg/util"
)

// Handler resets vmi annotations and nodeSelector when a migration completes
type Handler struct {
	namespace  string
	vmiCache   ctlv1.VirtualMachineInstanceCache
	vms        ctlv1.VirtualMachineClient
	vmCache    ctlv1.VirtualMachineCache
	podCache   ctlcorev1.PodCache
	pods       ctlcorev1.PodClient
	restClient rest.Interface
}

func (h *Handler) OnVmiChanged(_ string, vmi *v1.VirtualMachineInstance) (*v1.VirtualMachineInstance, error) {
	if vmi == nil || vmi.DeletionTimestamp != nil ||
		vmi.Annotations == nil || vmi.Status.MigrationState == nil {
		return vmi, nil
	}

	if vmi.Annotations[util.AnnotationMigrationUID] == string(vmi.Status.MigrationState.MigrationUID) &&
		vmi.Status.MigrationState.Completed {
		toUpdate := vmi.DeepCopy()
		delete(toUpdate.Annotations, util.AnnotationMigrationUID)
		delete(toUpdate.Annotations, util.AnnotationMigrationState)
		if vmi.Annotations[util.AnnotationMigrationTarget] != "" {
			delete(toUpdate.Annotations, util.AnnotationMigrationTarget)
			delete(toUpdate.Spec.NodeSelector, corev1.LabelHostname)
		}

		if err := util.VirtClientUpdateVmi(context.Background(), h.restClient, h.namespace, vmi.Namespace, vmi.Name, toUpdate); err != nil {
			return vmi, err
		}
		if err := h.syncVM(vmi); err != nil {
			return vmi, err
		}
	}

	if vmi.Status.MigrationState.Completed && vmi.Status.MigrationState.AbortStatus == v1.MigrationAbortSucceeded {
		// clean up leftover pod on abortion success
		// https://github.com/kubevirt/kubevirt/issues/5373
		sets := labels.Set{
			v1.MigrationJobLabel: string(vmi.Status.MigrationState.MigrationUID),
		}
		pods, err := h.podCache.List(vmi.Namespace, sets.AsSelector())
		if err != nil {
			return vmi, err
		}
		if len(pods) > 0 {
			if err := h.pods.Delete(vmi.Namespace, pods[0].Name, &metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return vmi, err
			}
		}
	}

	return vmi, nil
}
