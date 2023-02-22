package template

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
)

const (
	TemplateLabel = "template.harvesterhci.io/templateID"
)

// templateVersionHandler sets metadata and status to templateVersion objects,
// including labels, ownerReference and status.Version.
type templateVersionHandler struct {
	templateCache      ctlharvesterv1.VirtualMachineTemplateCache
	templateVersions   ctlharvesterv1.VirtualMachineTemplateVersionClient
	templateController ctlharvesterv1.VirtualMachineTemplateController
	vmImageCache       ctlharvesterv1.VirtualMachineImageCache
	mu                 sync.RWMutex //use mutex to avoid create duplicated version
}

func (h *templateVersionHandler) OnChanged(key string, tv *harvesterv1.VirtualMachineTemplateVersion) (*harvesterv1.VirtualMachineTemplateVersion, error) {
	if tv == nil || tv.DeletionTimestamp != nil {
		return nil, nil
	}

	ns, templateName := ref.Parse(tv.Spec.TemplateID)
	template, err := h.templateCache.Get(ns, templateName)
	if err != nil {
		return nil, err
	}

	copyObj := tv.DeepCopy()

	//set labels
	if copyObj.Labels == nil {
		copyObj.Labels = make(map[string]string)
	}
	if _, ok := copyObj.Labels[TemplateLabel]; !ok {
		copyObj.Labels[TemplateLabel] = templateName
	}

	//set ownerReference
	flagTrue := true
	ownerRef := []metav1.OwnerReference{{
		Name:               template.Name,
		APIVersion:         template.APIVersion,
		UID:                template.UID,
		Kind:               template.Kind,
		BlockOwnerDeletion: &flagTrue,
		Controller:         &flagTrue,
	}}

	if len(copyObj.OwnerReferences) == 0 {
		copyObj.OwnerReferences = ownerRef
	} else if !isVersionOwnedByTemplate(copyObj, template) {
		copyObj.OwnerReferences = append(copyObj.OwnerReferences, ownerRef...)
	}

	if isReady, err := h.isVMImagesReady(tv); err != nil {
		return nil, err
	} else if isReady {
		harvesterv1.TemplateVersionReady.True(copyObj)
	} else {
		harvesterv1.TemplateVersionReady.False(copyObj)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	//set version
	if !harvesterv1.VersionAssigned.IsTrue(copyObj) {
		existLatestVersion, _, err := getTemplateLatestVersion(tv.Namespace, tv.Spec.TemplateID, h.templateVersions)
		if err != nil {
			return nil, err
		}

		latestVersion := existLatestVersion + 1
		copyObj.Status.Version = latestVersion
		harvesterv1.VersionAssigned.True(copyObj)
	}

	if !reflect.DeepEqual(copyObj, tv) {
		if _, err = h.templateVersions.Update(copyObj); err != nil {
			return copyObj, err
		}
		h.templateController.Enqueue(ns, templateName)
	}

	return copyObj, nil
}

func (h *templateVersionHandler) isVMImagesReady(tv *harvesterv1.VirtualMachineTemplateVersion) (bool, error) {
	volumeClaimTemplatesStr, ok := tv.Spec.VM.ObjectMeta.Annotations[util.AnnotationVolumeClaimTemplates]
	if !ok || volumeClaimTemplatesStr == "" {
		return true, nil
	}

	var volumeClaimTemplates []corev1.PersistentVolumeClaim
	if err := json.Unmarshal([]byte(volumeClaimTemplatesStr), &volumeClaimTemplates); err != nil {
		return false, fmt.Errorf("can't unmarshal %s annotation, err: %w", util.AnnotationVolumeClaimTemplates, err)
	}

	for _, volumeClaimTemplate := range volumeClaimTemplates {
		imageID, ok := volumeClaimTemplate.Annotations[util.AnnotationImageID]
		if !ok || imageID == "" {
			continue
		}

		imageNs, imageName := ref.Parse(imageID)
		if image, err := h.vmImageCache.Get(imageNs, imageName); err != nil {
			return false, err
		} else if !harvesterv1.ImageImported.IsTrue(image) {
			return false, nil
		}
	}
	return true, nil
}

func getTemplateLatestVersion(templateVersionNs, templateID string, templateVersions ctlharvesterv1.VirtualMachineTemplateVersionClient) (int, *harvesterv1.VirtualMachineTemplateVersion, error) {
	var latestVersion int
	list, err := templateVersions.List(templateVersionNs, metav1.ListOptions{})
	if err != nil {
		return latestVersion, nil, err
	}

	var tvs []harvesterv1.VirtualMachineTemplateVersion
	for _, v := range list.Items {
		if v.Spec.TemplateID == templateID {
			tvs = append(tvs, v)
		}
	}

	if len(tvs) == 0 {
		return 0, nil, nil
	}

	sort.Sort(templateVersionByCreationTimestamp(tvs))
	for _, v := range tvs {
		if harvesterv1.VersionAssigned.IsTrue(v) {
			return v.Status.Version, &v, nil
		}
	}

	return 0, nil, nil
}

// templateVersionByCreationTimestamp sorts a list of TemplateVersion by creation timestamp, using their names as a tie breaker.
type templateVersionByCreationTimestamp []harvesterv1.VirtualMachineTemplateVersion

func (o templateVersionByCreationTimestamp) Len() int      { return len(o) }
func (o templateVersionByCreationTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o templateVersionByCreationTimestamp) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[j].CreationTimestamp.Before(&o[i].CreationTimestamp)
}

func isVersionOwnedByTemplate(version *harvesterv1.VirtualMachineTemplateVersion, template *harvesterv1.VirtualMachineTemplate) bool {
	for _, v := range version.OwnerReferences {
		if v.UID == template.UID {
			return true
		}
	}
	return false
}
