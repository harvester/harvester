package virtualmachineimage

import (
	"testing"

	longhorntypes "github.com/longhorn/longhorn-manager/types"
	"github.com/stretchr/testify/assert"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corefake "k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_virtualmachineimage_mutator(t *testing.T) {
	tests := []struct {
		name             string
		storageClassName string
		patchOps         []string
		errCheck         func(err error) bool
	}{
		{
			name: "storageClassName is empty",
			patchOps: []string{
				`{"op": "add", "path": "/spec/storageClassParameters", "value": {"migratable":"true","numberOfReplicas":"3","staleReplicaTimeout":"30"}}`,
			},
			storageClassName: "",
			errCheck:         nil,
		},
		{
			name:             "storageClassName is not exist",
			patchOps:         nil,
			storageClassName: "not-existing-sc",
			errCheck:         apierrors.IsNotFound,
		},
		{
			name: "storageClassName is exist",
			patchOps: []string{
				`{"op": "add", "path": "/spec/storageClassParameters", "value": {"diskSelector":"nvme","migratable":"true","nodeSelector":"node1","numberOfReplicas":"1","staleReplicaTimeout":"30"}}`,
			},
			storageClassName: "existing-sc",
			errCheck:         nil,
		},
	}

	defaultStorageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "longhorn",
			Annotations: map[string]string{
				"storageclass.kubernetes.io/is-default-class": "true",
			},
		},
		Provisioner: longhorntypes.LonghornDriverName,
		Parameters: map[string]string{
			"baseImage":           "",
			"fromBackup":          "",
			"migratable":          "true",
			"numberOfReplicas":    "3",
			"staleReplicaTimeout": "30",
		},
	}

	existStorageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "existing-sc",
		},
		Provisioner: longhorntypes.LonghornDriverName,
		Parameters: map[string]string{
			"baseImage":           "",
			"fromBackup":          "",
			"migratable":          "true",
			"numberOfReplicas":    "1",
			"staleReplicaTimeout": "30",
			"nodeSelector":        "node1",
			"diskSelector":        "nvme",
		},
	}
	fakeStorageClassList := []*storagev1.StorageClass{defaultStorageClass, existStorageClass}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			coreclientset := corefake.NewSimpleClientset()
			for _, fakeStorageClass := range fakeStorageClassList {
				err := coreclientset.Tracker().Add(fakeStorageClass.DeepCopy())
				assert.Nil(t, err, "Mock resource should add into fake controller tracker")
			}

			fakeStorageClassCache := fakeclients.StorageClassCache(coreclientset.StorageV1().StorageClasses)
			mutator := NewMutator(fakeStorageClassCache).(*virtualMachineImageMutator)

			image := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-test",
					Namespace: "default",
					Annotations: map[string]string{
						util.AnnotationStorageClassName: tc.storageClassName,
					},
				},
			}

			actual, err := mutator.patchImageStorageClassParams(image)
			if tc.errCheck != nil {
				assert.Equal(t, tc.errCheck(err), true)
			} else {
				assert.Nil(t, err, tc.name)
			}
			assert.Equal(t, tc.patchOps, actual)
		})
	}
}
