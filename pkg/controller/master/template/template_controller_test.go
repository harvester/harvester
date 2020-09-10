package template

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	virtv1alpha3 "kubevirt.io/client-go/api/v1alpha3"

	harvesterapis "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/generated/clientset/versioned/fake"
	harvestertype "github.com/rancher/harvester/pkg/generated/clientset/versioned/typed/harvester.cattle.io/v1alpha1"
	ctrlapis "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
)

func TestTemplateHandler_OnChanged(t *testing.T) {
	type input struct {
		key             string
		template        *harvesterapis.VirtualMachineTemplate
		templateVersion *harvesterapis.VirtualMachineTemplateVersion
	}
	type output struct {
		template *harvesterapis.VirtualMachineTemplate
		err      error
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "nil resource",
			given: input{
				key:      "",
				template: nil,
			},
			expected: output{
				template: nil,
				err:      nil,
			},
		},
		{
			name: "deleted resource",
			given: input{
				key: "default/test",
				template: &harvesterapis.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						DeletionTimestamp: &metav1.Time{},
					},
				},
			},
			expected: output{
				template: &harvesterapis.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						DeletionTimestamp: &metav1.Time{},
					},
				},
				err: nil,
			},
		},
		{
			name: "blank default version ID",
			given: input{
				key: "default/test",
				template: &harvesterapis.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterapis.VirtualMachineTemplateSpec{
						DefaultVersionID: "",
					},
				},
			},
			expected: output{
				template: &harvesterapis.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterapis.VirtualMachineTemplateSpec{
						DefaultVersionID: "",
					},
				},
				err: nil,
			},
		},
		{
			name: "not corresponding version template",
			given: input{
				key: "default/test",
				template: &harvesterapis.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterapis.VirtualMachineTemplateSpec{
						DefaultVersionID: "default:test",
					},
				},
				templateVersion: &harvesterapis.VirtualMachineTemplateVersion{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "fake",
					},
					Spec: harvesterapis.VirtualMachineTemplateVersionSpec{
						Description: "fake_description",
						TemplateID:  "default:test",
						ImageID:     "fake_image_id",
						VM:          virtv1alpha3.VirtualMachineSpec{},
					},
					Status: harvesterapis.VirtualMachineTemplateVersionStatus{
						Version: 1,
						Conditions: []harvesterapis.Condition{
							{
								Type:   harvesterapis.VersionAssigned,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: output{
				template: nil,
				err:      errors.NewNotFound(schema.GroupResource{Group: "harvester.cattle.io", Resource: "virtualmachinetemplateversions"}, "test"),
			},
		},
		{
			name: "directly return as the template version is the same",
			given: input{
				key: "default/test",
				template: &harvesterapis.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterapis.VirtualMachineTemplateSpec{
						DefaultVersionID: "default:test",
					},
					Status: harvesterapis.VirtualMachineTemplateStatus{
						DefaultVersion: 1,
					},
				},
				templateVersion: &harvesterapis.VirtualMachineTemplateVersion{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterapis.VirtualMachineTemplateVersionSpec{
						Description: "fake_description",
						TemplateID:  "fake_template_id",
						ImageID:     "fake_image_id",
						VM:          virtv1alpha3.VirtualMachineSpec{},
					},
					Status: harvesterapis.VirtualMachineTemplateVersionStatus{
						Version: 1,
						Conditions: []harvesterapis.Condition{
							{
								Type:   harvesterapis.VersionAssigned,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: output{
				template: &harvesterapis.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterapis.VirtualMachineTemplateSpec{
						DefaultVersionID: "default:test",
					},
					Status: harvesterapis.VirtualMachineTemplateStatus{
						DefaultVersion: 1,
					},
				},
				err: nil,
			},
		},
		{
			name: "update template version",
			given: input{
				key: "default/test",
				template: &harvesterapis.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterapis.VirtualMachineTemplateSpec{
						DefaultVersionID: "default:test",
					},
					Status: harvesterapis.VirtualMachineTemplateStatus{
						DefaultVersion: 1,
						LatestVersion:  1,
					},
				},
				templateVersion: &harvesterapis.VirtualMachineTemplateVersion{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterapis.VirtualMachineTemplateVersionSpec{
						Description: "fake_description",
						TemplateID:  "default:test",
						ImageID:     "fake_image_id",
						VM:          virtv1alpha3.VirtualMachineSpec{},
					},
					Status: harvesterapis.VirtualMachineTemplateVersionStatus{
						Version: 2,
						Conditions: []harvesterapis.Condition{
							{
								Type:   harvesterapis.VersionAssigned,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: output{
				template: &harvesterapis.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterapis.VirtualMachineTemplateSpec{
						DefaultVersionID: "default:test",
					},
					Status: harvesterapis.VirtualMachineTemplateStatus{
						DefaultVersion: 2,
						LatestVersion:  2,
					},
				},
				err: nil,
			},
		},
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.template != nil {
			var err = clientset.Tracker().Add(tc.given.template)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.templateVersion != nil {
			var err = clientset.Tracker().Add(tc.given.templateVersion)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		var handler = &templateHandler{
			templates:            fakeTemplateClient(clientset.HarvesterV1alpha1().VirtualMachineTemplates),
			templateVersions:     fakeTemplateVersionClient(clientset.HarvesterV1alpha1().VirtualMachineTemplateVersions),
			templateVersionCache: fakeTemplateVersionCache(clientset.HarvesterV1alpha1().VirtualMachineTemplateVersions),
		}
		var actual output
		actual.template, actual.err = handler.OnChanged(tc.given.key, tc.given.template)
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

type fakeTemplateClient func(string) harvestertype.VirtualMachineTemplateInterface

func (c fakeTemplateClient) Create(template *harvesterapis.VirtualMachineTemplate) (*harvesterapis.VirtualMachineTemplate, error) {
	return c(template.Namespace).Create(context.TODO(), template, metav1.CreateOptions{})
}

func (c fakeTemplateClient) Update(template *harvesterapis.VirtualMachineTemplate) (*harvesterapis.VirtualMachineTemplate, error) {
	return c(template.Namespace).Update(context.TODO(), template, metav1.UpdateOptions{})
}

func (c fakeTemplateClient) UpdateStatus(template *harvesterapis.VirtualMachineTemplate) (*harvesterapis.VirtualMachineTemplate, error) {
	return c(template.Namespace).UpdateStatus(context.TODO(), template, metav1.UpdateOptions{})
}

func (c fakeTemplateClient) Delete(namespace, name string, opts *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *opts)
}

func (c fakeTemplateClient) Get(namespace, name string, opts metav1.GetOptions) (*harvesterapis.VirtualMachineTemplate, error) {
	return c(namespace).Get(context.TODO(), name, opts)
}

func (c fakeTemplateClient) List(namespace string, opts metav1.ListOptions) (*harvesterapis.VirtualMachineTemplateList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeTemplateClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeTemplateClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterapis.VirtualMachineTemplate, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type fakeTemplateVersionCache func(string) harvestertype.VirtualMachineTemplateVersionInterface

func (c fakeTemplateVersionCache) Get(namespace, name string) (*harvesterapis.VirtualMachineTemplateVersion, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeTemplateVersionCache) List(namespace string, selector labels.Selector) (ret []*harvesterapis.VirtualMachineTemplateVersion, err error) {
	panic("implement me")
}

func (c fakeTemplateVersionCache) AddIndexer(indexName string, indexer ctrlapis.VirtualMachineTemplateVersionIndexer) {
	panic("implement me")
}

func (c fakeTemplateVersionCache) GetByIndex(indexName, key string) (ret []*harvesterapis.VirtualMachineTemplateVersion, err error) {
	panic("implement me")
}

type fakeTemplateVersionClient func(string) harvestertype.VirtualMachineTemplateVersionInterface

func (c fakeTemplateVersionClient) Create(templateVersion *harvesterapis.VirtualMachineTemplateVersion) (*harvesterapis.VirtualMachineTemplateVersion, error) {
	return c(templateVersion.Namespace).Create(context.TODO(), templateVersion, metav1.CreateOptions{})
}

func (c fakeTemplateVersionClient) UpdateStatus(templateVersion *harvesterapis.VirtualMachineTemplateVersion) (*harvesterapis.VirtualMachineTemplateVersion, error) {
	return c(templateVersion.Namespace).UpdateStatus(context.TODO(), templateVersion, metav1.UpdateOptions{})
}

func (c fakeTemplateVersionClient) Update(templateVersion *harvesterapis.VirtualMachineTemplateVersion) (*harvesterapis.VirtualMachineTemplateVersion, error) {
	return c(templateVersion.Namespace).Update(context.TODO(), templateVersion, metav1.UpdateOptions{})
}

func (c fakeTemplateVersionClient) Delete(namespace, name string, opts *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *opts)
}

func (c fakeTemplateVersionClient) Get(namespace, name string, opts metav1.GetOptions) (*harvesterapis.VirtualMachineTemplateVersion, error) {
	return c(namespace).Get(context.TODO(), name, opts)
}

func (c fakeTemplateVersionClient) List(namespace string, opts metav1.ListOptions) (*harvesterapis.VirtualMachineTemplateVersionList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeTemplateVersionClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeTemplateVersionClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterapis.VirtualMachineTemplateVersion, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}
