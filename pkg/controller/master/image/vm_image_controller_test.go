package image

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestVMImageHandler_OnChanged(t *testing.T) {
	type input struct {
		key   string
		image *harvesterv1.VirtualMachineImage
	}
	type output struct {
		image *harvesterv1.VirtualMachineImage
		sc    *storagev1.StorageClass
		err   error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "Test case 2: Image is not nil",
			given: input{
				key: "test-key",
				image: &harvesterv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-image",
						Namespace: "default",
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						SourceType:  "download",
						URL:         "https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/x86_64/alpine-standard-3.20.2-x86_64.iso",
						DisplayName: "test-image",
					},
				},
			},
			expected: output{
				image: &harvesterv1.VirtualMachineImage{},
				err:   nil,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.given.image)
			k8sclientset := k8sfake.NewSimpleClientset()

			handler := &vmImageHandler{
				backingImages:     fakeclients.BackingImageClient(clientset.LonghornV1beta2().BackingImages),
				backingImageCache: fakeclients.BackingImageCache(clientset.LonghornV1beta2().BackingImages),
				storageClasses:    fakeclients.StorageClassClient(k8sclientset.StorageV1().StorageClasses),
				storageClassCache: fakeclients.StorageClassCache(k8sclientset.StorageV1().StorageClasses),
				images:            fakeclients.VirtualMachineImageClient(clientset.HarvesterhciV1beta1().VirtualMachineImages),
				imageController:   fakeclients.VirtualMachineImageClient(clientset.HarvesterhciV1beta1().VirtualMachineImages),
				httpClient: http.Client{
					Timeout: 15 * time.Second,
				},
				pvcCache: fakeclients.PersistentVolumeClaimCache(k8sclientset.CoreV1().PersistentVolumeClaims),
			}

			actualImage, actualErr := handler.OnChanged(tc.given.key, tc.given.image)

			scs, _ := k8sclientset.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{})
			for _, sc := range scs.Items {
				fmt.Println(sc.Name)
				fmt.Println(sc.Parameters)
			}

			bis, _ := clientset.LonghornV1beta2().BackingImages("default").List(context.TODO(), metav1.ListOptions{})
			for _, bi := range bis.Items {
				fmt.Println(bi.Name)
				fmt.Println(bi.Spec.SourceParameters)
				fmt.Println(bi.Spec.SourceType)
			}

			assert.Equal(t, tc.expected.image, actualImage)
			assert.Equal(t, tc.expected.err, actualErr)
		})
	}
}
