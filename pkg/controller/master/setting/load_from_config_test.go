package setting

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	typeharv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
)

func TestTemplateHandler_OnChanged(t *testing.T) {
	type input struct {
		key     string
		setting *harvesterv1.Setting
	}
	type output struct {
		setting *harvesterv1.Setting
		err     error
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "No preconfig annotation, do nothing",
			given: input{
				setting: &harvesterv1.Setting{
					Default: "fooDefault",
					Value:   "fooValue",
				},
			},
			expected: output{
				setting: nil,
			},
		},
		{
			name: "Got preconfig annotation and no loaded condition, update setting CR",
			given: input{
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: "fooSetting",
						Annotations: map[string]string{
							"harvesterhci.io/preconfigValue": "fooPreconfigured",
						},
					},
					Default: "fooDefault",
					Value:   "fooValue",
				},
			},
			expected: output{
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: "fooSetting",
						Annotations: map[string]string{
							"harvesterhci.io/preconfigValue": "fooPreconfigured",
						},
					},
					Status: harvesterv1.SettingStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   harvesterv1.SettingPreconfigLoaded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					Default: "fooDefault",
					Value:   "fooPreconfigured",
				},
			},
		},
		{
			name: "Preconfig is loaded, will not overwrite Value",
			given: input{
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: "fooSetting",
						Annotations: map[string]string{
							"harvesterhci.io/preconfigValue": "fooPreconfigured",
						},
					},
					Status: harvesterv1.SettingStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   harvesterv1.SettingPreconfigLoaded,
								Status: corev1.ConditionTrue,
							},
						},
					},
					Default: "fooDefault",
					Value:   "fooModifiedAfterPreconfigLoaded",
				},
			},
			expected: output{},
		},
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.setting != nil {
			var err = clientset.Tracker().Add(tc.given.setting)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		var handler = &LoadingPreconfigHandler{
			settings: fakeSettingClient(clientset.HarvesterhciV1beta1().Settings),
		}
		var actual output
		actual.setting, actual.err = handler.settingOnChanged(tc.given.key, tc.given.setting)
		if actual.setting != nil {
			for i := range actual.setting.Status.Conditions {
				actual.setting.Status.Conditions[i].LastUpdateTime = ""
				actual.setting.Status.Conditions[i].LastTransitionTime = ""
			}
		}
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

type fakeSettingClient func() typeharv1.SettingInterface

func (c fakeSettingClient) Create(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	return c().Create(context.TODO(), setting, metav1.CreateOptions{})
}

func (c fakeSettingClient) Update(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	return c().Update(context.TODO(), setting, metav1.UpdateOptions{})
}

func (c fakeSettingClient) UpdateStatus(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	return c().UpdateStatus(context.TODO(), setting, metav1.UpdateOptions{})
}

func (c fakeSettingClient) Delete(name string, opts *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *opts)
}

func (c fakeSettingClient) Get(name string, opts metav1.GetOptions) (*harvesterv1.Setting, error) {
	return c().Get(context.TODO(), name, opts)
}

func (c fakeSettingClient) List(opts metav1.ListOptions) (*harvesterv1.SettingList, error) {
	return c().List(context.TODO(), opts)
}

func (c fakeSettingClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c fakeSettingClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *harvesterv1.Setting, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}
