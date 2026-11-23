package restic

import (
	"context"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	snapshotv6 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/backup/engine"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/datamover"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

func cloneFixture() (
	*ResticEngine, *harvesterv1.VirtualMachineBackup,
	*corev1.PersistentVolumeClaim, *fake.Clientset,
) {
	vmb := &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default", UID: "backup-uid"},
		Spec:       harvesterv1.VirtualMachineBackupSpec{Type: harvesterv1.Restic},
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
	re := &ResticEngine{vmbo: vmbo, vsHelper: vsHelper}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      re.clonePVCName(vmb, &vmb.Status.VolumeBackups[0]),
			Namespace: joblifecycle.Namespace, UID: "clone-uid",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(snapshotv1.GroupName), Kind: "VolumeSnapshot", Name: re.importedSnapshot(vmb, &vmb.Status.VolumeBackups[0]),
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	joblifecycle.Track(&pvc.ObjectMeta, vmb, vmBackupKind)
	source := &snapshotv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{Name: backupcommon.SnapshotName(vmb, &vmb.Status.VolumeBackups[0]), Namespace: vmb.Namespace, UID: "snapshot-uid"}}
	joblifecycle.Track(&source.ObjectMeta, vmb, vmBackupKind)
	clientset := fake.NewSimpleClientset(pvc, source)
	re.pvcCache = fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims)
	re.pvcClient = fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims)
	re.jobClient = fakeclients.JobClient(clientset.BatchV1().Jobs)
	re.vsClient = fakeclients.VolumeSnapshotClient(clientset.SnapshotV1().VolumeSnapshots)
	re.clientset = fakeclients.KubernetesClient(clientset)
	scheme := runtime.NewScheme()
	_ = snapshotv6.AddToScheme(scheme)
	original := &snapshotv6.VolumeSnapshot{ObjectMeta: source.ObjectMeta, Status: &snapshotv6.VolumeSnapshotStatus{ReadyToUse: ptr.To(true), BoundVolumeSnapshotContentName: ptr.To("source-content")}}
	content := &snapshotv6.VolumeSnapshotContent{ObjectMeta: metav1.ObjectMeta{Name: "source-content"}, Spec: snapshotv6.VolumeSnapshotContentSpec{Driver: "lvm", VolumeSnapshotRef: corev1.ObjectReference{Name: original.Name, Namespace: original.Namespace, UID: original.UID}}, Status: &snapshotv6.VolumeSnapshotContentStatus{SnapshotHandle: ptr.To("handle")}}
	name := re.importedSnapshot(vmb, &vmb.Status.VolumeBackups[0])
	imported := &snapshotv6.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: joblifecycle.Namespace}, Spec: snapshotv6.VolumeSnapshotSpec{Source: snapshotv6.VolumeSnapshotSource{VolumeSnapshotContentName: ptr.To(name)}}, Status: &snapshotv6.VolumeSnapshotStatus{ReadyToUse: ptr.To(true)}}
	alias := &snapshotv6.VolumeSnapshotContent{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: snapshotv6.VolumeSnapshotContentSpec{Driver: "lvm", DeletionPolicy: snapshotv6.VolumeSnapshotContentRetain, Source: snapshotv6.VolumeSnapshotContentSource{SnapshotHandle: ptr.To("handle")}, VolumeSnapshotRef: corev1.ObjectReference{Name: name, Namespace: joblifecycle.Namespace}}}
	joblifecycle.Track(&imported.ObjectMeta, vmb, vmBackupKind)
	joblifecycle.Track(&alias.ObjectMeta, vmb, vmBackupKind)
	re.imports = datamover.Snapshots{Client: dynamicfake.NewSimpleDynamicClient(scheme, original, content, imported, alias)}

	return re, vmb, pvc, clientset
}

func TestClonePVCReuseAndCleanup(t *testing.T) {
	for _, scenario := range []string{
		"matching clone", "mirrored source", "different source reference", "unowned PVC", "previous backup", "different snapshot", "terminating clone",
	} {
		t.Run(scenario, func(t *testing.T) {
			re, vmb, pvc, clientset := cloneFixture()
			conflict := true
			switch scenario {
			case "matching clone":
				conflict = false
			case "mirrored source", "different source reference":
				pvc.Spec.DataSourceRef = &corev1.TypedObjectReference{
					APIGroup: pvc.Spec.DataSource.APIGroup, Kind: pvc.Spec.DataSource.Kind, Name: pvc.Spec.DataSource.Name,
				}
				conflict = scenario == "different source reference"
				if conflict {
					pvc.Spec.DataSourceRef.Name = "other"
				}
			case "unowned PVC":
				pvc.Labels = nil
			case "previous backup":
				pvc.Labels[joblifecycle.LabelParentUID] = "old-backup-uid"
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
			err = re.ensureClonePVC(vmb, vb, pvc.Name)
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
			err = re.Reconcile(vmb, 0, nil)
			if !conflict {
				require.ErrorIs(t, err, engine.ErrRetryLater)
				_, err = re.pvcClient.Get(pvc.Namespace, pvc.Name, metav1.GetOptions{})
				require.True(t, apierrors.IsNotFound(err))
				return
			}
			require.Error(t, err)
			current, err := re.pvcClient.Get(pvc.Namespace, pvc.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, pvc, current)
			for _, action := range clientset.Actions() {
				require.False(t, action.Matches("delete", "persistentvolumeclaims"))
			}
		})
	}
}

func TestEnsureClonePVCCreatesClone(t *testing.T) {
	re, vmb, pvc, clientset := cloneFixture()
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

	require.NoError(t, re.ensureClonePVC(vmb, vb, pvc.Name))
	created, err := re.pvcClient.Get(pvc.Namespace, pvc.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, joblifecycle.Validate(vmb, vmBackupKind, created.ObjectMeta))
	require.Equal(t, re.importedSnapshot(vmb, vb), created.Spec.DataSource.Name)
	require.Empty(t, created.Spec.VolumeName)
	require.Nil(t, created.Spec.DataSourceRef)
	require.Equal(t, corev1.PersistentVolumeBlock, *created.Spec.VolumeMode)
}

func TestDeferredBindingAllowsJobCreation(t *testing.T) {
	for _, mode := range []storagev1.VolumeBindingMode{storagev1.VolumeBindingImmediate, storagev1.VolumeBindingWaitForFirstConsumer} {
		t.Run(string(mode), func(t *testing.T) {
			re, vmb, pvc, client := cloneFixture()
			vb := &vmb.Status.VolumeBackups[0]
			sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "test-sc"}, VolumeBindingMode: &mode}
			_, createErr := client.StorageV1().StorageClasses().Create(context.Background(), sc, metav1.CreateOptions{})
			require.NoError(t, createErr)
			re.scCache = fakeclients.StorageClassCache(client.StorageV1().StorageClasses)
			pvc.Spec.StorageClassName = &sc.Name
			pvc.Status.Phase = corev1.ClaimPending
			vb.PersistentVolumeClaim.Spec.StorageClassName = &sc.Name
			_, updateErr := client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Update(context.Background(), pvc, metav1.UpdateOptions{})
			require.NoError(t, updateErr)
			checkErr := re.ensureClonePVC(vmb, vb, pvc.Name)
			if mode == storagev1.VolumeBindingWaitForFirstConsumer {
				require.NoError(t, checkErr)
			} else {
				require.ErrorIs(t, checkErr, engine.ErrRetryLater)
			}
		})
	}
}

func TestCleanupTemporaryResources(t *testing.T) {
	re, vmb, pvc, client := cloneFixture()
	vb := &vmb.Status.VolumeBackups[0]
	require.Nil(t, vb.ReadyToUse)
	sourceName := backupcommon.SnapshotName(vmb, vb)
	importedName := re.importedSnapshot(vmb, vb)
	var err error
	for pass := 0; pass < 8; pass++ {
		err = re.Cleanup(vmb)
		if err == nil {
			break
		}
		require.ErrorIs(t, err, engine.ErrRetryLater)
	}
	require.NoError(t, err)
	_, err = client.CoreV1().PersistentVolumeClaims(pvc.Namespace).
		Get(context.Background(), pvc.Name, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
	_, err = client.SnapshotV1().VolumeSnapshots(vmb.Namespace).
		Get(context.Background(), sourceName, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
	_, err = re.imports.Client.Resource(snapshotv6.SchemeGroupVersion.WithResource("volumesnapshots")).
		Namespace(joblifecycle.Namespace).Get(context.Background(), importedName, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
	_, err = re.imports.Client.Resource(snapshotv6.SchemeGroupVersion.WithResource("volumesnapshotcontents")).
		Get(context.Background(), importedName, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
	require.NoError(t, re.Cleanup(vmb), "cleanup must be idempotent")
}
