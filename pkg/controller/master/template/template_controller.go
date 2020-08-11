package template

import (
	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/vm.cattle.io/v1alpha1"
	ctlapisv1alpha1 "github.com/rancher/harvester/pkg/generated/controllers/vm.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/ref"
)

// templateHandler sets status.Version to template objects
type templateHandler struct {
	templates            ctlapisv1alpha1.TemplateClient
	templateVersionCache ctlapisv1alpha1.TemplateVersionCache
}

func (h *templateHandler) OnChanged(key string, tp *apisv1alpha1.Template) (*apisv1alpha1.Template, error) {
	if tp.Spec.DefaultVersionID == "" {
		return tp, nil
	}

	versionNs, versionName := ref.Parse(tp.Spec.DefaultVersionID)
	version, err := h.templateVersionCache.Get(versionNs, versionName)
	if err != nil {
		return nil, err
	}

	if tp.Status.DefaultVersion == version.Status.Version {
		return tp, nil
	}

	copyTp := tp.DeepCopy()
	copyTp.Status.DefaultVersion = version.Status.Version
	return h.templates.Update(copyTp)
}
