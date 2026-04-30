package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCheckDisplayName(t *testing.T) {
	maxLenDisplayName := strings.Repeat("a", 63)
	tooLongDisplayName := strings.Repeat("a", 64)

	testCases := []struct {
		name           string
		displayName    string
		existingImages []*harvesterv1.VirtualMachineImage
		expectErr      bool
		errContains    string
	}{
		{
			name:        "rejects empty displayName",
			displayName: "",
			expectErr:   true,
			errContains: "displayName is required",
		},
		{
			name:        "accepts displayName with 63 chars",
			displayName: maxLenDisplayName,
			expectErr:   false,
		},
		{
			name:        "rejects displayName with more than 63 chars",
			displayName: tooLongDisplayName,
			expectErr:   true,
			errContains: "must be no more than 63 characters",
		},
		{
			name:        "rejects displayName with invalid Kubernetes label value",
			displayName: "Invalid/Name",
			expectErr:   true,
			errContains: "displayName is not a valid Kubernetes label value",
		},
		{
			name:        "rejects duplicate displayName",
			displayName: "duplicate-name",
			existingImages: []*harvesterv1.VirtualMachineImage{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-image",
						Namespace: "default",
						UID:       "existing-uid",
						Labels: map[string]string{
							util.LabelImageDisplayName: "duplicate-name",
						},
					},
				},
			},
			expectErr:   true,
			errContains: "A resource with the same name exists",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientSet := fake.NewSimpleClientset()
			for _, image := range tc.existingImages {
				err := clientSet.Tracker().Add(image)
				assert.NoError(t, err)
			}

			validator := &vmiValidator{
				vmiCache: fakeclients.VirtualMachineImageCache(clientSet.HarvesterhciV1beta1().VirtualMachineImages),
			}

			vmi := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-image",
					Namespace: "default",
				},
				Spec: harvesterv1.VirtualMachineImageSpec{
					DisplayName: tc.displayName,
				},
			}

			err := validator.CheckDisplayName(vmi)
			if tc.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
