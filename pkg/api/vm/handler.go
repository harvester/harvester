package vm

import (
	"net/http"

	ctlkubevirtv1alpha3 "github.com/rancher/vm/pkg/generated/controllers/kubevirt.io/v1alpha3"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type vmActionHandler struct {
	vms  ctlkubevirtv1alpha3.VirtualMachineClient
	vmis ctlkubevirtv1alpha3.VirtualMachineInstanceClient
}

func (h *vmActionHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	action := vars["action"]
	namespace := vars["namespace"]
	name := vars["name"]

	switch action {
	case startVM, stopVM:
		running := action == startVM
		if err := h.updateVMRunningStatus(name, namespace, action, running); err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			rw.Write([]byte(err.Error()))
			return
		}

		rw.WriteHeader(http.StatusNoContent)
	case restartVM:
		if err := h.restartVM(name, namespace); err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			rw.Write([]byte(err.Error()))
		}
	default:
		rw.WriteHeader(http.StatusBadRequest)
		rw.Write([]byte("Unsupported action"))
	}
}

func (h *vmActionHandler) updateVMRunningStatus(name, namespace, action string, running bool) error {
	vm, err := h.vms.Get(namespace, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if vm.Spec.Running != nil && running == *vm.Spec.Running {
		return nil
	}

	vmCopy := vm.DeepCopy()
	vmCopy.Spec.Running = &running
	_, err = h.vms.Update(vmCopy)
	if err != nil {
		return errors.Wrapf(err, "Failed to %s to virtual machine %s:%s", action, namespace, name)
	}

	return nil
}

func (h *vmActionHandler) restartVM(name, namespace string) error {
	if err := h.vmis.Delete(namespace, name, &metav1.DeleteOptions{}); err != nil {
		return errors.Wrapf(err, "Failed to delete virtualMachineInstance %s:%s", namespace, name)
	}

	return nil
}
