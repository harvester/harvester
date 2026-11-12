package common_test

import (
	"errors"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func cloneFixture() (*corev1.PersistentVolumeClaim, metav1.OwnerReference) {
	owner := metav1.OwnerReference{
		APIVersion: harvesterv1.SchemeGroupVersion.String(),
		Kind:       "VirtualMachineBackup",
		Name:       "backup",
		UID:        "backup-uid",
		Controller: ptr.To(true),
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "clone", Namespace: "default", UID: "clone-uid",
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(snapshotv1.GroupName), Kind: "VolumeSnapshot", Name: "snapshot",
			},
			DataSourceRef: &corev1.TypedObjectReference{
				APIGroup: ptr.To(snapshotv1.GroupName), Kind: "VolumeSnapshot", Name: "snapshot",
			},
		},
	}
	return pvc, owner
}

func TestValidateClonePVC(t *testing.T) {
	tests := []struct {
		name   string
		change func(*corev1.PersistentVolumeClaim)
		valid  bool
	}{
		{name: "matching references", valid: true},
		{
			name: "no dataSourceRef", valid: true,
			change: func(pvc *corev1.PersistentVolumeClaim) { pvc.Spec.DataSourceRef = nil },
		},
		{
			name: "explicit local namespace", valid: true,
			change: func(pvc *corev1.PersistentVolumeClaim) {
				pvc.Spec.DataSourceRef.Namespace = ptr.To(pvc.Namespace)
			},
		},
		{
			name:   "unowned PVC",
			change: func(pvc *corev1.PersistentVolumeClaim) { pvc.OwnerReferences = nil },
		},
		{
			name:   "previous backup with same name",
			change: func(pvc *corev1.PersistentVolumeClaim) { pvc.OwnerReferences[0].UID = "old-uid" },
		},
		{
			name:   "different owner name",
			change: func(pvc *corev1.PersistentVolumeClaim) { pvc.OwnerReferences[0].Name = "other" },
		},
		{
			name:   "different owner kind",
			change: func(pvc *corev1.PersistentVolumeClaim) { pvc.OwnerReferences[0].Kind = "Job" },
		},
		{
			name:   "different owner API",
			change: func(pvc *corev1.PersistentVolumeClaim) { pvc.OwnerReferences[0].APIVersion = "v1" },
		},
		{
			name: "not the controller",
			change: func(pvc *corev1.PersistentVolumeClaim) {
				pvc.OwnerReferences[0].Controller = ptr.To(false)
			},
		},
		{
			name:   "missing data source",
			change: func(pvc *corev1.PersistentVolumeClaim) { pvc.Spec.DataSource = nil },
		},
		{
			name:   "missing source group",
			change: func(pvc *corev1.PersistentVolumeClaim) { pvc.Spec.DataSource.APIGroup = nil },
		},
		{
			name:   "different source group",
			change: func(pvc *corev1.PersistentVolumeClaim) { pvc.Spec.DataSource.APIGroup = ptr.To("other") },
		},
		{
			name:   "different source kind",
			change: func(pvc *corev1.PersistentVolumeClaim) { pvc.Spec.DataSource.Kind = "PersistentVolumeClaim" },
		},
		{
			name:   "different snapshot",
			change: func(pvc *corev1.PersistentVolumeClaim) { pvc.Spec.DataSource.Name = "other" },
		},
		{
			name: "conflicting reference group",
			change: func(pvc *corev1.PersistentVolumeClaim) {
				pvc.Spec.DataSourceRef.APIGroup = ptr.To("other")
			},
		},
		{
			name:   "conflicting reference kind",
			change: func(pvc *corev1.PersistentVolumeClaim) { pvc.Spec.DataSourceRef.Kind = "PersistentVolumeClaim" },
		},
		{
			name:   "conflicting reference name",
			change: func(pvc *corev1.PersistentVolumeClaim) { pvc.Spec.DataSourceRef.Name = "other" },
		},
		{
			name: "cross namespace reference",
			change: func(pvc *corev1.PersistentVolumeClaim) {
				pvc.Spec.DataSourceRef.Namespace = ptr.To("other")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pvc, owner := cloneFixture()
			if tt.change != nil {
				tt.change(pvc)
			}
			err := common.ValidateClonePVC(pvc, owner, "snapshot")
			if tt.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.ErrorContains(t, err, "default/clone")
		})
	}
	pvc, owner := cloneFixture()
	owner.UID = ""
	require.Error(t, common.ValidateClonePVC(pvc, owner, "snapshot"))
	_, owner = cloneFixture()
	require.Error(t, common.ValidateClonePVC(pvc, owner, ""))
}

func TestDeleteClonePVC(t *testing.T) {
	pvc, owner := cloneFixture()
	clientset := fake.NewSimpleClientset(pvc)
	client := fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims)
	clientset.PrependReactor("delete", "persistentvolumeclaims",
		func(action clienttesting.Action) (bool, runtime.Object, error) {
			options := action.(clienttesting.DeleteAction).GetDeleteOptions()
			require.NotNil(t, options.Preconditions)
			require.Equal(t, pvc.UID, ptr.Deref(options.Preconditions.UID, ""))
			return false, nil, nil
		})

	require.NoError(t, common.DeleteClonePVC(client, pvc.Namespace, pvc.Name, owner, "snapshot"))
	_, err := client.Get(pvc.Namespace, pvc.Name, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
	require.NoError(t, common.DeleteClonePVC(client, pvc.Namespace, pvc.Name, owner, "snapshot"))
}

func TestDeleteClonePVCReplacement(t *testing.T) {
	pvc, owner := cloneFixture()
	clientset := fake.NewSimpleClientset(pvc)
	client := fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims)
	resource := corev1.SchemeGroupVersion.WithResource("persistentvolumeclaims")
	clientset.PrependReactor("delete", "persistentvolumeclaims",
		func(action clienttesting.Action) (bool, runtime.Object, error) {
			// Model replacement between the live GET and DELETE. The fake tracker
			// does not enforce API preconditions, so emulate the server's UID check.
			replacement := pvc.DeepCopy()
			replacement.UID = "replacement-uid"
			replacement.OwnerReferences = nil
			require.NoError(t, clientset.Tracker().Update(resource, replacement, pvc.Namespace))
			options := action.(clienttesting.DeleteAction).GetDeleteOptions()
			if options.Preconditions != nil && options.Preconditions.UID != nil &&
				*options.Preconditions.UID != replacement.UID {
				return true, nil, apierrors.NewConflict(resource.GroupResource(), pvc.Name,
					errors.New("UID precondition failed"))
			}
			return false, nil, nil
		})

	err := common.DeleteClonePVC(client, pvc.Namespace, pvc.Name, owner, "snapshot")
	require.True(t, apierrors.IsConflict(err))
	current, err := client.Get(pvc.Namespace, pvc.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "replacement-uid", string(current.UID))
}

func TestDeleteClonePVCErrors(t *testing.T) {
	for _, verb := range []string{"get", "delete"} {
		t.Run(verb, func(t *testing.T) {
			pvc, owner := cloneFixture()
			clientset := fake.NewSimpleClientset(pvc)
			client := fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims)
			apiErr := errors.New("API unavailable")
			clientset.PrependReactor(verb, "persistentvolumeclaims",
				func(clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, apiErr
				})
			err := common.DeleteClonePVC(client, pvc.Namespace, pvc.Name, owner, "snapshot")
			require.ErrorIs(t, err, apiErr)
		})
	}
	t.Run("missing UID", func(t *testing.T) {
		pvc, owner := cloneFixture()
		pvc.UID = ""
		clientset := fake.NewSimpleClientset(pvc)
		client := fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims)
		require.ErrorContains(t, common.DeleteClonePVC(client, pvc.Namespace, pvc.Name, owner, "snapshot"), "no UID")
	})
	t.Run("deleted concurrently", func(t *testing.T) {
		pvc, owner := cloneFixture()
		clientset := fake.NewSimpleClientset(pvc)
		client := fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims)
		clientset.PrependReactor("delete", "persistentvolumeclaims",
			func(clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewNotFound(
					schema.GroupResource{Resource: "persistentvolumeclaims"}, pvc.Name,
				)
			})
		require.NoError(t, common.DeleteClonePVC(client, pvc.Namespace, pvc.Name, owner, "snapshot"))
	})
}
