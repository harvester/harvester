package migration

import (
	"context"

	"k8s.io/client-go/rest"

	"github.com/rancher/harvester/pkg/config"
	virtv1 "github.com/rancher/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
)

const (
	controllerName = "migrationTargetController"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	copyConfig := rest.CopyConfig(management.RestConfig)
	virtv1Client, err := virtv1.NewForConfig(copyConfig)
	if err != nil {
		return err
	}
	vmis := management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	handler := &Handler{
		namespace:  options.Namespace,
		restClient: virtv1Client.RESTClient(),
	}

	vmis.OnChange(ctx, controllerName, handler.OnVmiChanged)
	return nil
}
