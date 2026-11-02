package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/harvester/harvester/pkg/settings"
)

func TestImageFromHelmValues(t *testing.T) {
	helmValues := map[string]interface{}{
		"generalJob": map[string]interface{}{
			"image": map[string]interface{}{
				"repository":      "rancher/harvester-upgrade",
				"tag":             "v1.9.1",
				"imagePullPolicy": "IfNotPresent",
			},
		},
		"kubevirt-operator": map[string]interface{}{
			"containers": map[string]interface{}{
				"handler": map[string]interface{}{
					"image": map[string]interface{}{
						"repository": "registry.suse.com/suse/sles/15.7/virt-handler",
						"tag":        12.2,
					},
				},
			},
		},
		"broken": map[string]interface{}{
			"image": "not-an-object",
		},
	}

	var testCases = []struct {
		name      string
		keyNames  []string
		expected  settings.Image
		expectErr bool
	}{
		{
			name:     "image with pull policy",
			keyNames: []string{"generalJob", "image"},
			expected: settings.Image{Repository: "rancher/harvester-upgrade", Tag: "v1.9.1", ImagePullPolicy: corev1.PullIfNotPresent},
		},
		{
			name:     "nested image with a numeric tag",
			keyNames: []string{"kubevirt-operator", "containers", "handler", "image"},
			expected: settings.Image{Repository: "registry.suse.com/suse/sles/15.7/virt-handler", Tag: 12.2},
		},
		{
			name:      "missing path",
			keyNames:  []string{"support-bundle-kit", "image"},
			expectErr: true,
		},
		{
			name:      "value that isn't an image",
			keyNames:  []string{"broken", "image"},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		image, err := ImageFromHelmValues(helmValues, tc.keyNames)
		if tc.expectErr {
			assert.Error(t, err, "case %q", tc.name)
			continue
		}
		assert.NoError(t, err, "case %q", tc.name)
		assert.Equal(t, tc.expected, image, "case %q", tc.name)
	}
}

func TestCattleRegistryFromHelmValues(t *testing.T) {
	var testCases = []struct {
		name       string
		helmValues map[string]interface{}
		expected   string
	}{
		{
			name:       "no global values",
			helmValues: map[string]interface{}{},
		},
		{
			name: "registry is set",
			helmValues: map[string]interface{}{
				"global": map[string]interface{}{
					"cattle": map[string]interface{}{"systemDefaultRegistry": "registry.example.com"},
				},
			},
			expected: "registry.example.com",
		},
		{
			name: "registry isn't a string",
			helmValues: map[string]interface{}{
				"global": map[string]interface{}{
					"cattle": map[string]interface{}{"systemDefaultRegistry": 1},
				},
			},
		},
	}

	for _, tc := range testCases {
		assert.Equal(t, tc.expected, CattleRegistryFromHelmValues(tc.helmValues), "case %q", tc.name)
	}
}
