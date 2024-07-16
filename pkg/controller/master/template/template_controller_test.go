package template

import (
	"context"
	"testing"

	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	typeharv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
)

func TestTemplateHandler_OnChanged(t *testing.T) {
	type input struct {
		key             string
		template        *harvesterv1.VirtualMachineTemplate
		templateVersion *harvesterv1.VirtualMachineTemplateVersion
	}
	type output struct {
		template *harvesterv1.VirtualMachineTemplate
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
				template: &harvesterv1.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						DeletionTimestamp: &metav1.Time{},
					},
				},
			},
			expected: output{
				template: &harvesterv1.VirtualMachineTemplate{
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
				template: &harvesterv1.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterv1.VirtualMachineTemplateSpec{
						DefaultVersionID: "",
					},
				},
			},
			expected: output{
				template: &harvesterv1.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterv1.VirtualMachineTemplateSpec{
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
				template: &harvesterv1.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterv1.VirtualMachineTemplateSpec{
						DefaultVersionID: "default/test",
					},
				},
				templateVersion: &harvesterv1.VirtualMachineTemplateVersion{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "fake",
					},
					Spec: harvesterv1.VirtualMachineTemplateVersionSpec{
						Description: "fake_description",
						TemplateID:  "default/test",
						ImageID:     "fake_image_id",
						VM:          harvesterv1.VirtualMachineSourceSpec{},
					},
					Status: harvesterv1.VirtualMachineTemplateVersionStatus{
						Version: 1,
						Conditions: []harvesterv1.Condition{
							{
								Type:   harvesterv1.VersionAssigned,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: output{
				template: nil,
				err:      errors.NewNotFound(schema.GroupResource{Group: "harvesterhci.io", Resource: "virtualmachinetemplateversions"}, "test"),
			},
		},
		{
			name: "directly return as the template version is the same",
			given: input{
				key: "default/test",
				template: &harvesterv1.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterv1.VirtualMachineTemplateSpec{
						DefaultVersionID: "default/test",
					},
					Status: harvesterv1.VirtualMachineTemplateStatus{
						DefaultVersion: 1,
					},
				},
				templateVersion: &harvesterv1.VirtualMachineTemplateVersion{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterv1.VirtualMachineTemplateVersionSpec{
						Description: "fake_description",
						TemplateID:  "fake_template_id",
						ImageID:     "fake_image_id",
						VM:          harvesterv1.VirtualMachineSourceSpec{},
					},
					Status: harvesterv1.VirtualMachineTemplateVersionStatus{
						Version: 1,
						Conditions: []harvesterv1.Condition{
							{
								Type:   harvesterv1.VersionAssigned,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: output{
				template: &harvesterv1.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterv1.VirtualMachineTemplateSpec{
						DefaultVersionID: "default/test",
					},
					Status: harvesterv1.VirtualMachineTemplateStatus{
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
				template: &harvesterv1.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterv1.VirtualMachineTemplateSpec{
						DefaultVersionID: "default/test",
					},
					Status: harvesterv1.VirtualMachineTemplateStatus{
						DefaultVersion: 1,
						LatestVersion:  1,
					},
				},
				templateVersion: &harvesterv1.VirtualMachineTemplateVersion{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterv1.VirtualMachineTemplateVersionSpec{
						Description: "fake_description",
						TemplateID:  "default/test",
						ImageID:     "fake_image_id",
						VM:          harvesterv1.VirtualMachineSourceSpec{},
					},
					Status: harvesterv1.VirtualMachineTemplateVersionStatus{
						Version: 2,
						Conditions: []harvesterv1.Condition{
							{
								Type:   harvesterv1.VersionAssigned,
								Status: v1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: output{
				template: &harvesterv1.VirtualMachineTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: harvesterv1.VirtualMachineTemplateSpec{
						DefaultVersionID: "default/test",
					},
					Status: harvesterv1.VirtualMachineTemplateStatus{
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
			templates:            fakeTemplateClient(clientset.HarvesterhciV1beta1().VirtualMachineTemplates),
			templateVersions:     fakeTemplateVersionClient(clientset.HarvesterhciV1beta1().VirtualMachineTemplateVersions),
			templateVersionCache: fakeTemplateVersionCache(clientset.HarvesterhciV1beta1().VirtualMachineTemplateVersions),
		}
		var actual output
		actual.template, actual.err = handler.OnChanged(tc.given.key, tc.given.template)
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

type fakeTemplateClient func(string) typeharv1.VirtualMachineTemplateInterface

func (c fakeTemplateClient) Create(template *harvesterv1.VirtualMachineTemplate) (*harvesterv1.VirtualMachineTemplate, error) {
	return c(template.Namespace).Create(context.TODO(), template, metav1.CreateOptions{})
}

func (c fakeTemplateClient) Update(template *harvesterv1.VirtualMachineTemplate) (*harvesterv1.VirtualMachineTemplate, error) {
	return c(template.Namespace).Update(context.TODO(), template, metav1.UpdateOptions{})
}

func (c fakeTemplateClient) UpdateStatus(template *harvesterv1.VirtualMachineTemplate) (*harvesterv1.VirtualMachineTemplate, error) {
	return c(template.Namespace).UpdateStatus(context.TODO(), template, metav1.UpdateOptions{})
}

func (c fakeTemplateClient) Delete(namespace, name string, opts *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *opts)
}

func (c fakeTemplateClient) Get(namespace, name string, opts metav1.GetOptions) (*harvesterv1.VirtualMachineTemplate, error) {
	return c(namespace).Get(context.TODO(), name, opts)
}

func (c fakeTemplateClient) List(namespace string, opts metav1.ListOptions) (*harvesterv1.VirtualMachineTemplateList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeTemplateClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeTemplateClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterv1.VirtualMachineTemplate, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c fakeTemplateClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*harvesterv1.VirtualMachineTemplate, *harvesterv1.VirtualMachineTemplateList], error) {
	panic("implement me")
}

type fakeTemplateVersionCache func(string) typeharv1.VirtualMachineTemplateVersionInterface

func (c fakeTemplateVersionCache) Get(namespace, name string) (*harvesterv1.VirtualMachineTemplateVersion, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeTemplateVersionCache) List(_ string, _ labels.Selector) ([]*harvesterv1.VirtualMachineTemplateVersion, error) {
	panic("implement me")
}

func (c fakeTemplateVersionCache) AddIndexer(_ string, _ generic.Indexer[*harvesterv1.VirtualMachineTemplateVersion]) {
	panic("implement me")
}

func (c fakeTemplateVersionCache) GetByIndex(_, _ string) ([]*harvesterv1.VirtualMachineTemplateVersion, error) {
	panic("implement me")
}

type fakeTemplateVersionClient func(string) typeharv1.VirtualMachineTemplateVersionInterface

func (c fakeTemplateVersionClient) Create(templateVersion *harvesterv1.VirtualMachineTemplateVersion) (*harvesterv1.VirtualMachineTemplateVersion, error) {
	return c(templateVersion.Namespace).Create(context.TODO(), templateVersion, metav1.CreateOptions{})
}

func (c fakeTemplateVersionClient) UpdateStatus(templateVersion *harvesterv1.VirtualMachineTemplateVersion) (*harvesterv1.VirtualMachineTemplateVersion, error) {
	return c(templateVersion.Namespace).UpdateStatus(context.TODO(), templateVersion, metav1.UpdateOptions{})
}

func (c fakeTemplateVersionClient) Update(templateVersion *harvesterv1.VirtualMachineTemplateVersion) (*harvesterv1.VirtualMachineTemplateVersion, error) {
	return c(templateVersion.Namespace).Update(context.TODO(), templateVersion, metav1.UpdateOptions{})
}

func (c fakeTemplateVersionClient) Delete(namespace, name string, opts *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *opts)
}

func (c fakeTemplateVersionClient) Get(namespace, name string, opts metav1.GetOptions) (*harvesterv1.VirtualMachineTemplateVersion, error) {
	return c(namespace).Get(context.TODO(), name, opts)
}

func (c fakeTemplateVersionClient) List(namespace string, opts metav1.ListOptions) (*harvesterv1.VirtualMachineTemplateVersionList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeTemplateVersionClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeTemplateVersionClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterv1.VirtualMachineTemplateVersion, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

func (c fakeTemplateVersionClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*harvesterv1.VirtualMachineTemplateVersion, *harvesterv1.VirtualMachineTemplateVersionList], error) {
	panic("implement me")
}
