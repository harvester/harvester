package template

import (
	"reflect"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
)

// templateHandler sets status.Version to template objects
type templateHandler struct {
	templates            ctlharvesterv1.VirtualMachineTemplateClient
	templateVersions     ctlharvesterv1.VirtualMachineTemplateVersionClient
	templateVersionCache ctlharvesterv1.VirtualMachineTemplateVersionCache
	templateController   ctlharvesterv1.VirtualMachineTemplateController
}

func (h *templateHandler) OnChanged(key string, tp *harvesterv1.VirtualMachineTemplate) (*harvesterv1.VirtualMachineTemplate, error) {
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

		if !harvesterv1.VersionAssigned.IsTrue(version) {
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
