package common

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	cditype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/cdi.kubevirt.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	werror "github.com/harvester/harvester/pkg/webhook/error"
)

func TestPVCClones(t *testing.T) {
	tests := []struct {
		name       string
		namespace  string
		source     *cdiv1.DataVolumeSourcePVC
		phase      cdiv1.DataVolumePhase
		claimName  string
		pvcName    string
		deleting   bool
		listErr    error
		getErr     error
		wantErr    string
		wantReason metav1.StatusReason
	}{
		{
			name:       "bound target PVC is still cloning",
			source:     &cdiv1.DataVolumeSourcePVC{Namespace: "images", Name: "image"},
			phase:      cdiv1.CloneInProgress,
			pvcName:    "clone",
			wantErr:    "cannot delete image images/image: being used by clone PVC default/clone",
			wantReason: metav1.StatusReasonConflict,
		},
		{
			name:       "completed cross namespace clone",
			source:     &cdiv1.DataVolumeSourcePVC{Namespace: "images", Name: "image"},
			phase:      cdiv1.Succeeded,
			pvcName:    "clone",
			wantErr:    "cannot delete image images/image: being used by clone PVC default/clone",
			wantReason: metav1.StatusReasonConflict,
		},
		{
			name:       "terminating clone PVC still exists",
			source:     &cdiv1.DataVolumeSourcePVC{Namespace: "images", Name: "image"},
			phase:      cdiv1.Succeeded,
			pvcName:    "clone",
			deleting:   true,
			wantErr:    "cannot delete image images/image: being used by clone PVC default/clone",
			wantReason: metav1.StatusReasonConflict,
		},
		{
			name:       "target PVC name comes from DataVolume status",
			source:     &cdiv1.DataVolumeSourcePVC{Namespace: "images", Name: "image"},
			claimName:  "target",
			pvcName:    "target",
			wantErr:    "cannot delete image images/image: being used by clone PVC default/target",
			wantReason: metav1.StatusReasonConflict,
		},
		{
			name:      "missing status target does not match PVC with DataVolume name",
			source:    &cdiv1.DataVolumeSourcePVC{Namespace: "images", Name: "image"},
			claimName: "target",
			pvcName:   "clone",
		},
		{
			name:   "clone PVC no longer exists",
			source: &cdiv1.DataVolumeSourcePVC{Namespace: "images", Name: "image"},
			phase:  cdiv1.Succeeded,
		},
		{
			name:       "same namespace source omits namespace",
			namespace:  "images",
			source:     &cdiv1.DataVolumeSourcePVC{Name: "image"},
			pvcName:    "clone",
			wantErr:    "cannot delete image images/image: being used by clone PVC images/clone",
			wantReason: metav1.StatusReasonConflict,
		},
		{
			name:    "source namespace defaults to DataVolume namespace",
			source:  &cdiv1.DataVolumeSourcePVC{Name: "image"},
			pvcName: "clone",
		},
		{
			name:    "same source name in another namespace",
			source:  &cdiv1.DataVolumeSourcePVC{Namespace: "other", Name: "image"},
			pvcName: "clone",
		},
		{
			name:    "different source name",
			source:  &cdiv1.DataVolumeSourcePVC{Namespace: "images", Name: "other"},
			pvcName: "clone",
		},
		{
			name:    "DataVolume without a PVC source",
			pvcName: "clone",
		},
		{
			name:       "DataVolume lookup fails",
			listErr:    errors.New("cache unavailable"),
			wantErr:    "failed to get image PVC clones: cache unavailable",
			wantReason: metav1.StatusReasonInternalError,
		},
		{
			name:       "target PVC lookup fails",
			source:     &cdiv1.DataVolumeSourcePVC{Namespace: "images", Name: "image"},
			getErr:     errors.New("cache unavailable"),
			wantErr:    "failed to get clone PVC default/clone: cache unavailable",
			wantReason: metav1.StatusReasonInternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace := tt.namespace
			if namespace == "" {
				namespace = "default"
			}
			dv := &cdiv1.DataVolume{
				ObjectMeta: metav1.ObjectMeta{Name: "clone", Namespace: namespace},
				Status:     cdiv1.DataVolumeStatus{Phase: tt.phase, ClaimName: tt.claimName},
			}
			if tt.source != nil {
				dv.Spec.Source = &cdiv1.DataVolumeSource{PVC: tt.source}
			}
			clientset := fake.NewSimpleClientset(dv)
			if tt.pvcName != "" {
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{Name: tt.pvcName, Namespace: namespace},
					Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
				}
				if tt.deleting {
					now := metav1.Now()
					pvc.DeletionTimestamp = &now
				}
				assert.NoError(t, clientset.Tracker().Add(pvc))
			}
			if tt.listErr != nil {
				clientset.PrependReactor("list", "datavolumes", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.listErr
				})
			}
			if tt.getErr != nil {
				clientset.PrependReactor("get", "persistentvolumeclaims", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.getErr
				})
			}
			validator := &vmiValidator{
				pvcCache: fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
				dvCache: fakeclients.DataVolumeCache(func(string) cditype.DataVolumeInterface {
					return clientset.CdiV1beta1().DataVolumes()
				}),
			}
			image := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "image", Namespace: "images"},
			}
			err := validator.checkPVCClones(image)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
			var admitErr werror.AdmitError
			if assert.ErrorAs(t, err, &admitErr) {
				assert.Equal(t, tt.wantReason, admitErr.AsResult().Reason)
			}
		})
	}
}

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

func TestCheckUpdateDisplayName(t *testing.T) {
	testCases := []struct {
		name           string
		oldDisplayName string
		newDisplayName string
		expectErr      bool
		errContains    string
	}{
		{
			name:           "accepts unchanged valid displayName",
			oldDisplayName: "valid-name",
			newDisplayName: "valid-name",
			expectErr:      false,
		},
		{
			name:           "rejects changed displayName",
			oldDisplayName: "old-name",
			newDisplayName: "new-name",
			expectErr:      true,
			errContains:    "displayName cannot be modified",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientSet := fake.NewSimpleClientset()
			validator := &vmiValidator{
				vmiCache: fakeclients.VirtualMachineImageCache(clientSet.HarvesterhciV1beta1().VirtualMachineImages),
			}

			oldVMI := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-image",
					Namespace: "default",
				},
				Spec: harvesterv1.VirtualMachineImageSpec{
					DisplayName: tc.oldDisplayName,
				},
			}

			newVMI := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-image",
					Namespace: "default",
				},
				Spec: harvesterv1.VirtualMachineImageSpec{
					DisplayName: tc.newDisplayName,
				},
			}

			err := validator.CheckUpdateDisplayName(oldVMI, newVMI)
			if tc.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
