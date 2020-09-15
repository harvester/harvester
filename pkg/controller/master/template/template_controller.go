package template

import (
	"reflect"

	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	ctlapisv1alpha1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/ref"
)

// templateHandler sets status.Version to template objects
type templateHandler struct {
	templates            ctlapisv1alpha1.VirtualMachineTemplateClient
	templateVersions     ctlapisv1alpha1.VirtualMachineTemplateVersionClient
	templateVersionCache ctlapisv1alpha1.VirtualMachineTemplateVersionCache
	templateController   ctlapisv1alpha1.VirtualMachineTemplateController
}

func (h *templateHandler) OnChanged(key string, tp *apisv1alpha1.VirtualMachineTemplate) (*apisv1alpha1.VirtualMachineTemplate, error) {
	if tp == nil || tp.DeletionTimestamp != nil {
		return tp, nil
	}

	copyTp := tp.DeepCopy()
	templateID := ref.Construct(copyTp.Namespace, copyTp.Name)

	latestVersion, latestVersionObj, err := getTemplateLatestVersion(copyTp.Namespace, templateID, h.templateVersions)
	if err != nil {
		return nil, err
	}

	if latestVersion == 0 {
		return copyTp, nil
	}

	//set the first version as the default version
	defaultVersionID := copyTp.Spec.DefaultVersionID
	if defaultVersionID == "" && latestVersion == 1 {
		defaultVersionID = ref.Construct(latestVersionObj.Namespace, latestVersionObj.Name)
		if tp.Spec.DefaultVersionID != defaultVersionID {
			copyTp.Spec.DefaultVersionID = defaultVersionID
			if _, err = h.templates.Update(copyTp); err != nil {
				return nil, err
			}
			return tp, nil
		}
	}

	defaultVersion := copyTp.Status.DefaultVersion
	if defaultVersionID != "" {
		versionNs, versionName := ref.Parse(defaultVersionID)
		version, err := h.templateVersionCache.Get(versionNs, versionName)
		if err != nil {
			return nil, err
		}

		if !apisv1alpha1.VersionAssigned.IsTrue(version) {
			h.templateController.Enqueue(tp.Namespace, tp.Name)
			return tp, nil
		}
		defaultVersion = version.Status.Version
	}

	copyTp.Status.LatestVersion = latestVersion
	copyTp.Status.DefaultVersion = defaultVersion

	if !reflect.DeepEqual(tp, copyTp) {
		if _, err = h.templates.UpdateStatus(copyTp); err != nil {
			return nil, err
		}
	}

	return copyTp, nil
}
