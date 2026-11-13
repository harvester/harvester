package kopia

import (
	"context"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/backup/engine"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func cloneFixture() (
	*KopiaEngine, *harvesterv1.VirtualMachineBackup,
	*corev1.PersistentVolumeClaim, *fake.Clientset,
) {
	vmb := &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default", UID: "backup-uid"},
		Status: harvesterv1.VirtualMachineBackupStatus{
			VolumeBackups: []harvesterv1.VolumeBackup{{
				Name: ptr.To("snapshot"),
				PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
					ObjectMeta: metav1.ObjectMeta{Name: "source"},
				},
			}},
		},
	}
	vmbo := backupcommon.NewVMBackupOperatorBuilder().Build()
	vsHelper := backupcommon.NewVolumeSnapshotHelper(nil, nil, nil, nil, vmbo, nil, nil)
	ke := &KopiaEngine{vmbo: vmbo, vsHelper: vsHelper}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ke.clonePVCName(vmb, &vmb.Status.VolumeBackups[0]),
			Namespace: vmb.Namespace, UID: "clone-uid",
			OwnerReferences: []metav1.OwnerReference{vsHelper.BuildOwnerReference(vmb)},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(snapshotv1.GroupName), Kind: "VolumeSnapshot", Name: "snapshot",
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	clientset := fake.NewSimpleClientset(pvc)
	ke.pvcCache = fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims)
	ke.pvcClient = fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims)
	ke.jobClient = fakeclients.JobClient(clientset.BatchV1().Jobs)
	ke.vsClient = fakeclients.VolumeSnapshotClient(clientset.SnapshotV1().VolumeSnapshots)
	return ke, vmb, pvc, clientset
}

func TestClonePVCReuseAndCleanup(t *testing.T) {
	for _, scenario := range []string{
		"matching clone", "unowned PVC", "previous backup", "different snapshot", "terminating clone",
	} {
		t.Run(scenario, func(t *testing.T) {
			ke, vmb, pvc, clientset := cloneFixture()
			conflict := true
			switch scenario {
			case "matching clone":
				conflict = false
			case "unowned PVC":
				pvc.OwnerReferences = nil
			case "previous backup":
				pvc.OwnerReferences[0].UID = "old-backup-uid"
			case "different snapshot":
				pvc.Spec.DataSource.Name = "unrelated-snapshot"
			case "terminating clone":
				pvc.DeletionTimestamp = ptr.To(metav1.Now())
				conflict = false
			}
			_, err := clientset.CoreV1().PersistentVolumeClaims(pvc.Namespace).Update(
				context.Background(), pvc, metav1.UpdateOptions{},
			)
			require.NoError(t, err)
			clientset.ClearActions()
			vb := &vmb.Status.VolumeBackups[0]
			err = ke.ensureClonePVC(vmb, vb, pvc.Name)
			if conflict {
				require.Error(t, err)
				require.NotErrorIs(t, err, engine.ErrRetryLater)
			} else if scenario == "terminating clone" {
				require.ErrorIs(t, err, engine.ErrRetryLater)
			} else {
				require.NoError(t, err)
			}

			// A ready backup enters cleanup without calling ensureClonePVC.
			vb.ReadyToUse = ptr.To(true)
			err = ke.Reconcile(vmb, 0, nil)
			if !conflict {
				require.NoError(t, err)
				_, err = ke.pvcClient.Get(pvc.Namespace, pvc.Name, metav1.GetOptions{})
				require.True(t, apierrors.IsNotFound(err))
				return
			}
			require.Error(t, err)
			current, err := ke.pvcClient.Get(pvc.Namespace, pvc.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, pvc, current)
			for _, action := range clientset.Actions() {
				require.False(t, action.Matches("delete", "persistentvolumeclaims"))
			}
		})
	}
}

func TestEnsureClonePVCCreatesClone(t *testing.T) {
	ke, vmb, pvc, clientset := cloneFixture()
	resource := corev1.SchemeGroupVersion.WithResource("persistentvolumeclaims")
	require.NoError(t, clientset.Tracker().Delete(resource, pvc.Namespace, pvc.Name))
	vb := &vmb.Status.VolumeBackups[0]
	vb.PersistentVolumeClaim.Spec = corev1.PersistentVolumeClaimSpec{
		VolumeName: "source-pv",
		VolumeMode: ptr.To(corev1.PersistentVolumeBlock),
		DataSourceRef: &corev1.TypedObjectReference{
			Kind: "PersistentVolumeClaim", Name: "old-source",
		},
	}

	require.NoError(t, ke.ensureClonePVC(vmb, vb, pvc.Name))
	created, err := ke.pvcClient.Get(pvc.Namespace, pvc.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, backupcommon.ValidateClonePVC(
		created, ke.vsHelper.BuildOwnerReference(vmb), "snapshot",
	))
	require.Empty(t, created.Spec.VolumeName)
	require.Nil(t, created.Spec.DataSourceRef)
	require.Equal(t, corev1.PersistentVolumeBlock, *created.Spec.VolumeMode)
}
