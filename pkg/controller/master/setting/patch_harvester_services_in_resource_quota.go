package setting

import (
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) syncPatchHarvesterServicesInResourceQuota(setting *harvesterv1.Setting) error {
	if setting.Value == "true" || setting.Value == "false" {
		resourceQuotas, err := h.resourceQuotaController.List(corev1.NamespaceAll, metav1.ListOptions{})
		if err != nil {
			logrus.WithError(err).Error("failed to list resource quotas")
			return err
		}
		for _, resourceQuota := range resourceQuotas.Items {
			resourceQuotaCopy := resourceQuota.DeepCopy()
			if resourceQuotaCopy.Annotations == nil {
				resourceQuotaCopy.Annotations = make(map[string]string)
			}
			resourceQuotaCopy.Annotations[util.AnnotationTriggerUpdateTime] = time.Now().Format(time.RFC3339)
			if _, err = h.resourceQuotaController.Update(resourceQuotaCopy); err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"resourceQuota": resourceQuota.Name,
				}).Error("failed to update resource quota")
				return err
			}
		}
	}
	return nil
}
