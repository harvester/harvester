package migration

import (
	"context"

	ctlv1 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/rancher/harvester/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	v1 "kubevirt.io/client-go/api/v1"
)

// Handler resets the node target in vmi annotations and nodeSelector when a migration completes
type Handler struct {
	namespace    string
	vmiCache     ctlv1.VirtualMachineInstanceCache
	vmController ctlv1.VirtualMachineController
	restClient   rest.Interface
}

func (h *Handler) OnVmiChanged(_ string, vmi *v1.VirtualMachineInstance) (*v1.VirtualMachineInstance, error) {
	if vmi == nil || vmi.DeletionTimestamp != nil ||
		vmi.Annotations == nil || vmi.Annotations[util.AnnotationMigrationTarget] == "" ||
		vmi.Status.MigrationState == nil || !vmi.Status.MigrationState.Completed ||
		vmi.Status.MigrationState.TargetNode != vmi.Annotations[util.AnnotationMigrationTarget] {
		return vmi, nil
	}

	toUpdate := vmi.DeepCopy()
	delete(toUpdate.Annotations, util.AnnotationMigrationTarget)
	delete(toUpdate.Spec.NodeSelector, corev1.LabelHostname)
	return vmi, util.VirtClientUpdateVmi(context.Background(), h.restClient, h.namespace, vmi.Namespace, vmi.Name, toUpdate)
}
