package migration

import (
	"context"
	"crypto/tls"
	"net/http"

	"k8s.io/client-go/rest"

	"github.com/harvester/harvester/pkg/config"
	virtv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
)

const (
	vmiControllerName         = "migrationTargetController"
	vmimControllerName        = "migrationAnnotationController"
	vmimMetricsControllerName = "migrationMetricsController"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	copyConfig := rest.CopyConfig(management.RestConfig)
	virtv1Client, err := virtv1.NewForConfig(copyConfig)
	if err != nil {
		return err
	}
	rqs := management.HarvesterCoreFactory.Core().V1().ResourceQuota()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	pods := management.CoreFactory.Core().V1().Pod()
	vmis := management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	vmims := management.VirtFactory.Kubevirt().V1().VirtualMachineInstanceMigration()
	endpoints := management.CoreFactory.Core().V1().Endpoints()
	handler := &Handler{
		namespace:     options.Namespace,
		rqs:           rqs,
		rqCache:       rqs.Cache(),
		vmiCache:      vmis.Cache(),
		vms:           vms,
		vmCache:       vms.Cache(),
		pods:          pods,
		podCache:      pods.Cache(),
		endpointCache: endpoints.Cache(),
		restClient:    virtv1Client.RESTClient(),
		vmims:         vmims,

		httpClient: http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}

	vmis.OnChange(ctx, vmiControllerName, handler.OnVmiChanged)
	vmims.OnChange(ctx, vmimControllerName, handler.OnVmimChanged)
	vmims.OnChange(ctx, vmimMetricsControllerName, handler.OnVmimChangedUpdateMetrics)
	return nil
}
