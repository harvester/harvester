package datamover

import (
	"context"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/utils/ptr"

	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

func snapshotFixture(t *testing.T) (Snapshots, *metav1.PartialObjectMetadata) {
	t.Helper()
	parent := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "tenant", UID: "backup-uid"}}
	original := &snapshotv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{Name: "original", Namespace: parent.Namespace, UID: "snapshot-uid", OwnerReferences: []metav1.OwnerReference{{Name: parent.Name, UID: parent.UID, Kind: BackupKind}}},
		Status: &snapshotv1.VolumeSnapshotStatus{ReadyToUse: ptr.To(true), BoundVolumeSnapshotContentName: ptr.To("original-content")},
	}
	joblifecycle.Track(&original.ObjectMeta, parent, BackupKind)
	content := &snapshotv1.VolumeSnapshotContent{ObjectMeta: metav1.ObjectMeta{Name: "original-content"}, Spec: snapshotv1.VolumeSnapshotContentSpec{
		Driver: "lvm.driver", DeletionPolicy: snapshotv1.VolumeSnapshotContentDelete, SourceVolumeMode: ptr.To(corev1.PersistentVolumeBlock),
		VolumeSnapshotRef: corev1.ObjectReference{Name: original.Name, Namespace: original.Namespace, UID: original.UID},
	}, Status: &snapshotv1.VolumeSnapshotContentStatus{SnapshotHandle: ptr.To("backend-snapshot"), ReadyToUse: ptr.To(true)}}
	scheme := runtime.NewScheme()
	require.NoError(t, snapshotv1.AddToScheme(scheme))
	return Snapshots{Client: dynamicfake.NewSimpleDynamicClient(scheme, original, content)}, parent
}

func TestImportRetainsBackendOwnershipAndVolumeMode(t *testing.T) {
	snapshots, parent := snapshotFixture(t)
	require.ErrorIs(t, snapshots.Import(parent, "original", "imported"), ErrPending)
	imported, err := snapshots.content("imported")
	require.NoError(t, err)
	require.Equal(t, snapshotv1.VolumeSnapshotContentRetain, imported.Spec.DeletionPolicy)
	require.Equal(t, "backend-snapshot", *imported.Spec.Source.SnapshotHandle)
	require.Equal(t, corev1.PersistentVolumeBlock, *imported.Spec.SourceVolumeMode)
	require.Equal(t, joblifecycle.Namespace, imported.Spec.VolumeSnapshotRef.Namespace)
	require.Empty(t, imported.OwnerReferences)
	_, err = snapshots.Client.Resource(snapshotResource).Namespace(joblifecycle.Namespace).Patch(context.Background(), "imported", types.MergePatchType, []byte(`{"status":{"readyToUse":true}}`), metav1.PatchOptions{})
	require.NoError(t, err)
	require.NoError(t, snapshots.Import(parent, "original", "imported"))
	for i := 0; i < 3; i++ {
		err = snapshots.Delete(parent, "imported")
		if err == nil {
			break
		}
		require.ErrorIs(t, err, ErrPending)
	}
	require.NoError(t, err)
	original, err := snapshots.content("original-content")
	require.NoError(t, err)
	require.Equal(t, snapshotv1.VolumeSnapshotContentDelete, original.Spec.DeletionPolicy)
	_, err = snapshots.snapshot(parent.Namespace, "original")
	require.NoError(t, err)
}

func TestImportRejectsInvalidSource(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content bool
		patch   string
		message string
	}{
		{name: "snapshot pending", patch: `{"status":{"readyToUse":false}}`},
		{name: "missing binding", patch: `{"status":{"boundVolumeSnapshotContentName":null}}`},
		{
			name: "foreign backup", message: "does not belong",
			patch: `{"metadata":{"labels":{"harvesterhci.io/job-parent-uid":"another"}}}`,
		},
		{
			name: "stale snapshot binding", content: true, message: "different binding",
			patch: `{"spec":{"volumeSnapshotRef":{"uid":"another"}}}`,
		},
		{name: "missing handle", content: true, patch: `{"status":{"snapshotHandle":null}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshots, parent := snapshotFixture(t)
			client := snapshots.Client.Resource(snapshotResource).Namespace(parent.Namespace)
			name := "original"
			if tc.content {
				client = snapshots.Client.Resource(contentResource)
				name = "original-content"
			}
			_, err := client.Patch(
				context.Background(), name, types.MergePatchType,
				[]byte(tc.patch), metav1.PatchOptions{},
			)
			require.NoError(t, err)
			err = snapshots.Import(parent, "original", "imported")
			_, createdErr := snapshots.content("imported")
			require.True(t, apierrors.IsNotFound(createdErr), "invalid source must not create imported content")
			if tc.message != "" {
				require.ErrorContains(t, err, tc.message)
				return
			}
			require.ErrorIs(t, err, ErrPending)
		})
	}
}

func TestImportRejectsChangedHandleAndForeignIdentity(t *testing.T) {
	for _, patch := range []string{
		`{"spec":{"source":{"snapshotHandle":"different"}}}`,
		`{"metadata":{"labels":{"harvesterhci.io/job-parent-uid":"another"}}}`,
		`{"spec":{"sourceVolumeMode":"Filesystem"}}`,
		`{"spec":{"volumeSnapshotRef":{"uid":"different-snapshot"}}}`,
	} {
		t.Run(patch, func(t *testing.T) {
			snapshots, parent := snapshotFixture(t)
			require.ErrorIs(t, snapshots.Import(parent, "original", "imported"), ErrPending)
			_, err := snapshots.Client.Resource(contentResource).Patch(context.Background(), "imported", types.MergePatchType, []byte(patch), metav1.PatchOptions{})
			require.NoError(t, err)
			err = snapshots.Import(parent, "original", "imported")
			require.Error(t, err)
			require.NotErrorIs(t, err, ErrPending)
		})
	}
}
