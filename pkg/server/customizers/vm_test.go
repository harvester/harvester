package customizers

import (
	"testing"

	"github.com/rancher/apiserver/pkg/types"
	corev1 "k8s.io/api/core/v1"
)

func Test_DropRevisionStateIfNeededWithEmptyState(t *testing.T) {
	resource := &types.RawResource{
		APIObject: types.APIObject{
			Object: &corev1.ConfigMap{},
		},
	}

	DropRevisionStateIfNeeded(nil, resource)
}
