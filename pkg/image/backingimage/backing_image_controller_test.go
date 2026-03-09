package backingimage

import (
	"context"
	"net/http"
	"testing"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	fakegenerated "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const (
	testNamespace = "default"
	testImageName = "test-image"
	testSCName    = "longhorn-test"
	testRetry     = 3
)

// TestBackingImageHandler_OnChanged tests that a BackingImage is reconciled when a VirtualMachineImage is changed
// VMImage enters the initialized state when the DataVolume is not found.
func TestBackingImageHandler_OnChanged_RetryLimitExceeded(t *testing.T) {
	tests := []struct {
		name           string
		vmImage        *harvesterv1.VirtualMachineImage
		backingImage   *lhv1beta2.BackingImage
		result         *lhv1beta2.BackingImage
		expectedFailed int
		err            error
	}{
		{
			name: "BackingImage should be reconciled when a VirtualMachineImage is changed",
			vmImage: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testImageName,
					Namespace: testNamespace,
				},
				Spec: harvesterv1.VirtualMachineImageSpec{
					DisplayName:            "Test Upload Image",
					SourceType:             harvesterv1.VirtualMachineImageSourceTypeUpload,
					Backend:                harvesterv1.VMIBackendBackingImage,
					TargetStorageClassName: testSCName,
					Retry:                  testRetry,
				},
				Status: harvesterv1.VirtualMachineImageStatus{
					Failed: testRetry - 1,
					Conditions: []harvesterv1.Condition{
						{
							Type:   harvesterv1.ImageInitialized,
							Status: "True",
						},
					},
				},
			},
			backingImage: &lhv1beta2.BackingImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testImageName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						util.AnnotationImageID: ref.Construct(testNamespace, testImageName),
					},
				},
				Status: lhv1beta2.BackingImageStatus{
					DiskFileStatusMap: map[string]*lhv1beta2.BackingImageDiskFileStatus{
						"test-disk": {
							State:   lhv1beta2.BackingImageStateFailed,
							Message: "test error",
						},
					},
				},
			},
			result:         nil,
			expectedFailed: testRetry,
			err:            nil,
		},
		{
			name: "BackingImage should not be reconciled when retry limit is exceeded",
			vmImage: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testImageName,
					Namespace: testNamespace,
				},
				Spec: harvesterv1.VirtualMachineImageSpec{
					DisplayName:            "Test Upload Image",
					SourceType:             harvesterv1.VirtualMachineImageSourceTypeUpload,
					Backend:                harvesterv1.VMIBackendBackingImage,
					TargetStorageClassName: testSCName,
					Retry:                  testRetry,
				},
				Status: harvesterv1.VirtualMachineImageStatus{
					Failed: testRetry,
					Conditions: []harvesterv1.Condition{
						{
							Type:   harvesterv1.ImageRetryLimitExceeded,
							Status: "True",
						},
					},
				},
			},
			backingImage: &lhv1beta2.BackingImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testImageName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						util.AnnotationImageID: ref.Construct(testNamespace, testImageName),
					},
				},
				Status: lhv1beta2.BackingImageStatus{
					DiskFileStatusMap: map[string]*lhv1beta2.BackingImageDiskFileStatus{
						"test-disk": {
							State:   lhv1beta2.BackingImageStateFailed,
							Message: "test error",
						},
					},
				},
			},
			result: &lhv1beta2.BackingImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testImageName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						util.AnnotationImageID: ref.Construct(testNamespace, testImageName),
					},
				},
				Status: lhv1beta2.BackingImageStatus{
					DiskFileStatusMap: map[string]*lhv1beta2.BackingImageDiskFileStatus{
						"test-disk": {
							State:   lhv1beta2.BackingImageStateFailed,
							Message: "test error",
						},
					},
				},
			},
			expectedFailed: testRetry,
			err:            nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fake clients
			var harvFakeClient = fakegenerated.NewSimpleClientset()
			require.NoError(t, harvFakeClient.Tracker().Add(tt.vmImage))

			// Create VMIOperator
			vmio, err := common.GetVMIOperator(
				fakeclients.VirtualMachineImageClient(harvFakeClient.HarvesterhciV1beta1().VirtualMachineImages),
				fakeclients.VirtualMachineImageCache(harvFakeClient.HarvesterhciV1beta1().VirtualMachineImages),
				http.Client{},
			)
			require.Nil(t, err, "should create VMIOperator")

			// Create handler
			h := &backingImageHandler{
				vmiCache: fakeclients.VirtualMachineImageCache(harvFakeClient.HarvesterhciV1beta1().VirtualMachineImages),
				vmio:     vmio,
			}

			// Call OnChanged
			result, err := h.OnChanged("", tt.backingImage)
			require.Equal(t, tt.result, result)
			require.Equal(t, tt.err, err)

			updated, err := harvFakeClient.HarvesterhciV1beta1().
				VirtualMachineImages(testNamespace).
				Get(context.Background(), testImageName, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, tt.expectedFailed, updated.Status.Failed)
		})
	}
}
