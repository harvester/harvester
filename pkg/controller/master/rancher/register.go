package rancher

import (
	"context"

	rancherv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/harvester/harvester/pkg/config"
)

const (
	controllerRancherName     = "harvester-rancher-controller"
	cattleSystemNamespaceName = "cattle-system"
	rancherExposeServiceName  = "rancher-expose"
	rancherAppLabelName       = "app"
)

type Handler struct {
	RancherSettings rancherv3.SettingClient
	Services        ctlcorev1.ServiceClient
}

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	if options.RancherEmbedded {
		rancherSettings := management.RancherManagementFactory.Management().V3().Setting()
		services := management.CoreFactory.Core().V1().Service()
		h := Handler{
			RancherSettings: rancherSettings,
			Services:        services,
		}

		rancherSettings.OnChange(ctx, controllerRancherName, h.RancherSettingOnChange)
		return h.registerRancherExposeService()
	}
	return nil
}

func (h *Handler) registerRancherExposeService() error {
	_, err := h.Services.Get(cattleSystemNamespaceName, rancherExposeServiceName, v1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if apierrors.IsNotFound(err) {
		svc := &corev1.Service{
			ObjectMeta: v1.ObjectMeta{
				Name:      rancherExposeServiceName,
				Namespace: cattleSystemNamespaceName,
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeNodePort,
				Selector: map[string]string{
					rancherAppLabelName: "rancher",
				},
				Ports: []corev1.ServicePort{
					{
						Name:       "https-internal",
						NodePort:   30443,
						Port:       443,
						Protocol:   corev1.ProtocolTCP,
						TargetPort: intstr.FromInt(444),
					},
				},
			},
		}
		if _, err := h.Services.Create(svc); err != nil {
			return err
		}
	}
	return nil
}
