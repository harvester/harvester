package template

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	apisv1alpha1 "github.com/rancher/harvester/pkg/apis/vm.cattle.io/v1alpha1"
	ctlapisv1alpha1 "github.com/rancher/harvester/pkg/generated/controllers/vm.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/ref"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TemplateLabel = "template.vm.cattle.io/templateID"
)

// templateVersionHandler sets metadata and status to templateVersion objects,
// including labels, ownerReference and status.Version.
type templateVersionHandler struct {
	templates        ctlapisv1alpha1.TemplateClient
	templateCache    ctlapisv1alpha1.TemplateCache
	templateVersions ctlapisv1alpha1.TemplateVersionClient
	mu               sync.RWMutex //use mutex to avoid create duplicated version
}

func (h *templateVersionHandler) OnChanged(key string, tv *apisv1alpha1.TemplateVersion) (*apisv1alpha1.TemplateVersion, error) {
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

	h.mu.Lock()
	defer h.mu.Unlock()

	//set version
	if !apisv1alpha1.VersionAssigned.IsTrue(copyObj) {
		existLatestVersion, err := h.getTemplateLatestVersion(tv.Namespace, tv.Name, tv.Spec.TemplateID)
		if err != nil {
			return nil, err
		}

		latestVersion := existLatestVersion + 1
		copyObj.Status.Version = latestVersion
		apisv1alpha1.VersionAssigned.True(copyObj)

		copyTemplate := template.DeepCopy()
		copyTemplate.Status.LatestVersion = latestVersion
		if _, err := h.templates.Update(copyTemplate); err != nil {
			return nil, errors.Wrapf(err, "failed to set latest version for template %s:%s", template.Namespace, template.Name)
		}
	}

	if !reflect.DeepEqual(copyObj, tv) {
		return h.templateVersions.Update(copyObj)
	}

	return copyObj, nil
}

func (h *templateVersionHandler) getTemplateLatestVersion(templateVersionNs, templateVersionName, templateID string) (int, error) {
	var latestVersion int
	selector := fmt.Sprintf("metadata.name!=%s", templateVersionName)
	list, err := h.templateVersions.List(templateVersionNs, metav1.ListOptions{FieldSelector: selector})
	if err != nil {
		return latestVersion, err
	}

	var tvs []apisv1alpha1.TemplateVersion
	for _, v := range list.Items {
		if v.Spec.TemplateID == templateID {
			tvs = append(tvs, v)
		}
	}

	if len(tvs) == 0 {
		return 0, nil
	}

	sort.Sort(templateVersionByCreationTimestamp(tvs))
	for _, v := range tvs {
		if apisv1alpha1.VersionAssigned.IsTrue(v) {
			return v.Status.Version, nil
		}
	}

	return 0, nil
}

// templateVersionByCreationTimestamp sorts a list of TemplateVersion by creation timestamp, using their names as a tie breaker.
type templateVersionByCreationTimestamp []apisv1alpha1.TemplateVersion

func (o templateVersionByCreationTimestamp) Len() int      { return len(o) }
func (o templateVersionByCreationTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o templateVersionByCreationTimestamp) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[j].CreationTimestamp.Before(&o[i].CreationTimestamp)
}

func isVersionOwnedByTemplate(version *apisv1alpha1.TemplateVersion, template *apisv1alpha1.Template) bool {
	for _, v := range version.OwnerReferences {
		if v.UID == template.UID {
			return true
		}
	}
	return false
}
