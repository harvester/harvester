package upgrade

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func Test_checkForcePreloadImportAnnotation(t *testing.T) {
	var testCases = []struct {
		name      string
		upgrade   *v1beta1.Upgrade
		errString string
	}{
		{
			name:      "annotation is nil",
			upgrade:   &v1beta1.Upgrade{},
			errString: "",
		},
		{
			name: "annotation is true",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{forcePreloadImportAnnotation: "true"},
				},
			},
			errString: "",
		},
		{
			name: "annotation is false",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{forcePreloadImportAnnotation: "false"},
				},
			},
			errString: "",
		},
		{
			name: "annotation is something else",
			upgrade: &v1beta1.Upgrade{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{forcePreloadImportAnnotation: "True"},
				},
			},
			errString: fmt.Sprintf("annotation %s should be either 'true' or 'false'", forcePreloadImportAnnotation),
		},
	}

	for _, testCase := range testCases {
		err := checkForcePreloadImportAnnotation(testCase.upgrade)
		if testCase.errString != "" {
			assert.Equal(t, testCase.errString, err.Error())
		} else {
			assert.Nil(t, err)
		}
	}
}
