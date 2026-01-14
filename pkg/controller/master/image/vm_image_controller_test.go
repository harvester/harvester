package image

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakecore "k8s.io/client-go/kubernetes/fake"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	fakegenerated "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/cdi"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const (
	testNamespace = "default"
	testImageName = "test-image"
	testSCName    = "longhorn-test"
)

// TestVMImageHandler_OnChanged_UploadImageInitialization tests that an upload-type
// VMImage enters the initialized state when the DataVolume is not found.
func TestVMImageHandler_OnChanged_UploadImageInitialization(t *testing.T) {
	type args struct {
		vmImage      *harvesterv1.VirtualMachineImage
		dataVolume   *cdiv1.DataVolume
		storageClass *storagev1.StorageClass
	}

	tests := []struct {
		name              string
		args              args
		wantErr           bool
		wantInitialized   bool
		wantConditionTrue bool
	}{
		{
			name: "Upload VMImage without DataVolume should enter initialized state",
			args: args{
				vmImage: &harvesterv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testImageName,
						Namespace: testNamespace,
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						DisplayName:            "Test Upload Image",
						SourceType:             harvesterv1.VirtualMachineImageSourceTypeUpload,
						Backend:                harvesterv1.VMIBackendCDI,
						TargetStorageClassName: testSCName,
					},
					Status: harvesterv1.VirtualMachineImageStatus{},
				},
				dataVolume: nil, // No DataVolume exists yet
				storageClass: &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: testSCName,
					},
					Provisioner: "driver.longhorn.io",
				},
			},
			wantErr:           false,
			wantInitialized:   true,
			wantConditionTrue: true,
		},
		{
			name: "Upload VMImage with existing DataVolume should not re-initialize",
			args: args{
				vmImage: &harvesterv1.VirtualMachineImage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testImageName,
						Namespace: testNamespace,
					},
					Spec: harvesterv1.VirtualMachineImageSpec{
						DisplayName:            "Test Upload Image",
						SourceType:             harvesterv1.VirtualMachineImageSourceTypeUpload,
						Backend:                harvesterv1.VMIBackendCDI,
						TargetStorageClassName: testSCName,
					},
					Status: harvesterv1.VirtualMachineImageStatus{
						Conditions: []harvesterv1.Condition{
							{
								Type:   "Initialized",
								Status: "True",
							},
						},
					},
				},
				dataVolume: &cdiv1.DataVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testImageName,
						Namespace: testNamespace,
					},
					Status: cdiv1.DataVolumeStatus{
						Phase: cdiv1.UploadReady,
					},
				},
				storageClass: &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: testSCName,
					},
					Provisioner: "driver.longhorn.io",
				},
			},
			wantErr:           false,
			wantInitialized:   true,
			wantConditionTrue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fake clients
			var coreClientset = fakecore.NewSimpleClientset()
			var harvFakeClient = fakegenerated.NewSimpleClientset()

			// Add StorageClass
			if tt.args.storageClass != nil {
				err := coreClientset.Tracker().Add(tt.args.storageClass)
				assert.Nil(t, err, "should add StorageClass to tracker")
			}

			// Add VMImage
			if tt.args.vmImage != nil {
				err := harvFakeClient.Tracker().Add(tt.args.vmImage)
				assert.Nil(t, err, "should add VMImage to tracker")
			}

			// Add DataVolume if exists
			if tt.args.dataVolume != nil {
				err := harvFakeClient.Tracker().Add(tt.args.dataVolume)
				assert.Nil(t, err, "should add DataVolume to tracker")
			}

			// Create VMIOperator
			vmio, err := common.GetVMIOperator(
				fakeclients.VirtualMachineImageClient(harvFakeClient.HarvesterhciV1beta1().VirtualMachineImages),
				fakeclients.VirtualMachineImageCache(harvFakeClient.HarvesterhciV1beta1().VirtualMachineImages),
				http.Client{},
			)
			assert.Nil(t, err, "should create VMIOperator")

			// Create CDI backend
			cdiBackend := cdi.GetBackend(
				context.Background(),
				fakeclients.DataVolumeClient(harvFakeClient.CdiV1beta1().DataVolumes),
				fakeclients.StorageClassClient(coreClientset.StorageV1().StorageClasses),
				fakeclients.PersistentVolumeClaimCache(coreClientset.CoreV1().PersistentVolumeClaims),
				vmio,
			)

			// Create backends map
			backends := map[harvesterv1.VMIBackend]backend.Backend{
				harvesterv1.VMIBackendCDI: cdiBackend,
			}

			// Create handler
			h := &vmImageHandler{
				vmiClient:     fakeclients.VirtualMachineImageClient(harvFakeClient.HarvesterhciV1beta1().VirtualMachineImages),
				vmiController: fakeclients.VirtualMachineImageClient(harvFakeClient.HarvesterhciV1beta1().VirtualMachineImages),
				vmio:          vmio,
				backends:      backends,
			}

			// Call OnChanged
			result, err := h.OnChanged("", tt.args.vmImage)

			// Verify error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("OnChanged() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify result
			if result == nil {
				t.Error("OnChanged() returned nil result")
				return
			}

			// Check if VMImage was initialized
			if tt.wantInitialized {
				// Get the updated VMImage from the tracker
				gvr := schema.GroupVersionResource{
					Group:    "harvesterhci.io",
					Version:  "v1beta1",
					Resource: "virtualmachineimages",
				}
				obj, err := harvFakeClient.Tracker().Get(gvr, tt.args.vmImage.Namespace, tt.args.vmImage.Name)
				assert.Nil(t, err, "should get VMImage from tracker")

				updatedVMI, ok := obj.(*harvesterv1.VirtualMachineImage)
				assert.True(t, ok, "should be a VirtualMachineImage")

				// Check if Initialized condition is set and StorageClassName is set
				if tt.wantConditionTrue {
					isInitialized := harvesterv1.ImageInitialized.IsTrue(updatedVMI)
					assert.Equal(t, testSCName, updatedVMI.Status.StorageClassName, "VMImage should have StorageClassName set to %s", testSCName)
					assert.True(t, isInitialized, "VMImage should have Initialized condition set to True")
				}
			}
		})
	}
}
