package templateversion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"

	"github.com/harvester/harvester/pkg/util"
)

func Test_validateVolumeClaimTemplateString(t *testing.T) {
	var testCases = []struct {
		name               string
		expectError        bool
		newTemplateVersion *harvesterv1.VirtualMachineTemplateVersion
	}{
		{
			name:        "no volume claim template string annotation",
			expectError: false,
			newTemplateVersion: &harvesterv1.VirtualMachineTemplateVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foobar-no-volume-claim-template-annotation",
				},
				Spec: harvesterv1.VirtualMachineTemplateVersionSpec{
					VM: harvesterv1.VirtualMachineSourceSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{},
						},
					},
				},
			},
		},
		{
			name:        "empty JSON data in volume claim template string annotation",
			expectError: false,
			newTemplateVersion: &harvesterv1.VirtualMachineTemplateVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foobar-empty-json",
				},
				Spec: harvesterv1.VirtualMachineTemplateVersionSpec{
					VM: harvesterv1.VirtualMachineSourceSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								util.AnnotationVolumeClaimTemplates: "",
							},
						},
					},
				},
			},
		},
		{
			name:        "valid JSON data in volume claim template string annotation",
			expectError: false,
			newTemplateVersion: &harvesterv1.VirtualMachineTemplateVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foobar-valid-json",
				},
				Spec: harvesterv1.VirtualMachineTemplateVersionSpec{
					VM: harvesterv1.VirtualMachineSourceSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								util.AnnotationVolumeClaimTemplates: "[]",
							},
						},
					},
				},
			},
		},
		{
			name:        "invalid JSON data in volume claim template string annotation",
			expectError: true,
			newTemplateVersion: &harvesterv1.VirtualMachineTemplateVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foobar-invalid-json",
				},
				Spec: harvesterv1.VirtualMachineTemplateVersionSpec{
					VM: harvesterv1.VirtualMachineSourceSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								util.AnnotationVolumeClaimTemplates: "[{}]}",
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		err := validateVolumeClaimTemplateString(tc.newTemplateVersion)
		if tc.expectError {
			assert.NotNil(t, err, tc.name)
		} else {
			assert.Nil(t, err, tc.name)
		}
	}
}
