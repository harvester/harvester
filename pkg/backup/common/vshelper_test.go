package common_test

import (
	"context"
	"errors"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const (
	snapshotNamespace = "default"
	snapshotDriver    = "driver.example.io"
)

func TestBuildOwnerReferenceWithMissingTypeMeta(t *testing.T) {
	fixture := newSnapshotFixture()
	fixture.vmb.TypeMeta = metav1.TypeMeta{}

	owner := fixture.helper.BuildOwnerReference(fixture.vmb)
	require.Equal(t, "VirtualMachineBackup", owner.Kind)
	require.Equal(t, harvesterv1.SchemeGroupVersion.String(), owner.APIVersion)
}

func TestPrepareSnapshotsWaitsForAllSnapshots(t *testing.T) {
	fixture := newSnapshotFixture()

	ready, err := fixture.helper.PrepareSnapshots(fixture.vmb, fixture.classes)
	require.NoError(t, err)
	require.False(t, ready)
	snapshots := listSnapshots(t, fixture.clientset, snapshotNamespace)
	require.Len(t, snapshots, 2)
	require.Equal(t, "snapshot-class", *snapshots[0].Spec.VolumeSnapshotClassName)
	require.False(t, ptr.Deref(fixture.vmb.Status.VolumeBackups[0].ReadyToUse, false))
	require.False(t, ptr.Deref(fixture.vmb.Status.VolumeBackups[1].ReadyToUse, false))
	require.Equal(t, 1, actionCount(fixture.clientset.Actions(), "update", "virtualmachinebackups"))

	markSnapshotReady(t, fixture.clientset, snapshotNamespace, "backup-volume-disk-1")
	ready, err = fixture.helper.PrepareSnapshots(fixture.vmb, fixture.classes)
	require.NoError(t, err)
	require.False(t, ready)

	markSnapshotReady(t, fixture.clientset, snapshotNamespace, "backup-volume-disk-2")
	ready, err = fixture.helper.PrepareSnapshots(fixture.vmb, fixture.classes)
	require.NoError(t, err)
	require.True(t, ready)
	require.Len(t, listSnapshots(t, fixture.clientset, snapshotNamespace), 2)
}

func TestPrepareSnapshotsRollsBackPartialBatch(t *testing.T) {
	fixture := newSnapshotFixture()
	createCount := 0
	createErr := errors.New("snapshot API unavailable")
	fixture.clientset.PrependReactor(
		"create",
		"volumesnapshots",
		func(clienttesting.Action) (bool, runtime.Object, error) {
			createCount++
			if createCount == 2 {
				return true, nil, createErr
			}
			return false, nil, nil
		},
	)

	ready, err := fixture.helper.PrepareSnapshots(fixture.vmb, fixture.classes)
	require.ErrorIs(t, err, createErr)
	require.False(t, ready)
	require.Empty(t, listSnapshots(t, fixture.clientset, snapshotNamespace))
}

type snapshotFixture struct {
	vmb       *harvesterv1.VirtualMachineBackup
	helper    *backupcommon.VolumeSnapshotHelper
	clientset *fake.Clientset
	classes   map[string]snapshotv1.VolumeSnapshotClass
}

func newSnapshotFixture() *snapshotFixture {
	sourceUID := types.UID("source-uid")
	vmb := &harvesterv1.VirtualMachineBackup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: harvesterv1.SchemeGroupVersion.String(),
			Kind:       "VirtualMachineBackup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backup",
			Namespace: snapshotNamespace,
			UID:       "backup-uid",
		},
		Spec: harvesterv1.VirtualMachineBackupSpec{
			Source: corev1.TypedLocalObjectReference{
				Kind: kubevirtv1.VirtualMachineGroupVersionKind.Kind,
				Name: "vm",
			},
			Type: harvesterv1.Restic,
		},
		Status: harvesterv1.VirtualMachineBackupStatus{
			SourceUID: &sourceUID,
			VolumeBackups: []harvesterv1.VolumeBackup{
				volumeBackup(snapshotNamespace, "disk-1", snapshotDriver),
				volumeBackup(snapshotNamespace, "disk-2", snapshotDriver),
			},
		},
	}
	pvc1 := sourcePVC(snapshotNamespace, "disk-1", snapshotDriver)
	pvc2 := sourcePVC(snapshotNamespace, "disk-2", snapshotDriver)
	clientset := fake.NewSimpleClientset(vmb, pvc1, pvc2)
	vmbo := backupcommon.NewVMBackupOperatorBuilder().
		WithClient(fakeclients.VMBackupClient(clientset.HarvesterhciV1beta1().VirtualMachineBackups)).
		WithVMICache(fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances)).
		Build()
	helper := backupcommon.NewVolumeSnapshotHelper(
		fakeclients.VolumeSnapshotCache(clientset.SnapshotV1().VolumeSnapshots),
		fakeclients.VolumeSnapshotClient(clientset.SnapshotV1().VolumeSnapshots),
		nil,
		nil,
		vmbo,
		fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
		nil,
	)
	classes := map[string]snapshotv1.VolumeSnapshotClass{
		snapshotDriver: {ObjectMeta: metav1.ObjectMeta{Name: "snapshot-class"}},
	}

	return &snapshotFixture{
		vmb:       vmb,
		helper:    helper,
		clientset: clientset,
		classes:   classes,
	}
}

func volumeBackup(namespace, pvcName, driver string) harvesterv1.VolumeBackup {
	return harvesterv1.VolumeBackup{
		Name:          ptr.To("backup-volume-" + pvcName),
		CSIDriverName: driver,
		ReadyToUse:    ptr.To(false),
		PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
			ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: namespace},
		},
	}
}

func sourcePVC(namespace, name, driver string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				util.AnnStorageProvisioner: driver,
			},
		},
	}
}

func listSnapshots(
	t *testing.T,
	clientset *fake.Clientset,
	namespace string,
) []snapshotv1.VolumeSnapshot {
	t.Helper()
	list, err := clientset.SnapshotV1().VolumeSnapshots(namespace).List(
		context.Background(), metav1.ListOptions{},
	)
	require.NoError(t, err)
	return list.Items
}

func markSnapshotReady(t *testing.T, clientset *fake.Clientset, namespace, name string) {
	t.Helper()
	client := clientset.SnapshotV1().VolumeSnapshots(namespace)
	vs, err := client.Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	vs.Status = &snapshotv1.VolumeSnapshotStatus{ReadyToUse: ptr.To(true)}
	_, err = client.UpdateStatus(context.Background(), vs, metav1.UpdateOptions{})
	require.NoError(t, err)
}

func actionCount(actions []clienttesting.Action, verb, resource string) int {
	count := 0
	for _, action := range actions {
		if action.GetVerb() == verb && action.GetResource().Resource == resource {
			count++
		}
	}
	return count
}
