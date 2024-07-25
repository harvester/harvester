package addon

import (
	"strings"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/rancher/wrangler/v3/pkg/relatedresource"
	"k8s.io/apimachinery/pkg/runtime"
)

// this file includes some workarounds on HelmChart

const (
	helmChartJobDeletePrefix = "helm-delete"
)

// downstream helmchart change will also trigger Addon OnChange
func (h *Handler) ReconcileHelmChartOwners(_ string, _ string, obj runtime.Object) ([]relatedresource.Key, error) {
	if hc, ok := obj.(*helmv1.HelmChart); ok {
		for _, v := range hc.GetOwnerReferences() {
			if strings.ToLower(v.Kind) == "addon" {
				return []relatedresource.Key{
					{
						Name:      v.Name,
						Namespace: hc.Namespace,
					},
				}, nil
			}
		}
	}

	return nil, nil
}
