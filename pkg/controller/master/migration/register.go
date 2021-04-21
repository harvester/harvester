package migration

import (
	"context"

	"k8s.io/client-go/rest"

	"github.com/harvester/harvester/pkg/config"
	virtv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
)

const (
	vmiControllerName  = "migrationTargetController"
	vmimControllerName = "migrationAnnotationController"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	copyConfig := rest.CopyConfig(management.RestConfig)
	virtv1Client, err := virtv1.NewForConfig(copyConfig)
	if err != nil {
		return err
	}
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	pods := management.CoreFactory.Core().V1().Pod()
	vmis := management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	vmims := management.VirtFactory.Kubevirt().V1().VirtualMachineInstanceMigration()
	handler := &Handler{
		namespace:  options.Namespace,
		vmiCache:   vmis.Cache(),
		vms:        vms,
		vmCache:    vms.Cache(),
		pods:       pods,
		podCache:   pods.Cache(),
		restClient: virtv1Client.RESTClient(),
	}

	vmis.OnChange(ctx, vmiControllerName, handler.OnVmiChanged)
	vmims.OnChange(ctx, vmimControllerName, handler.OnVmimChanged)
	return nil
}
