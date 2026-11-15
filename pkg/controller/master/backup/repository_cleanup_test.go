package backup

import (
	"context"
	"errors"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util/datamover"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

func TestOrphanCleanupChecksParentLive(t *testing.T) {
	for _, scenario := range []string{"exists", "missing", "replaced", "API failure"} {
		t.Run(scenario, func(t *testing.T) {
			parent := &harvesterv1.VirtualMachineRestore{ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "tenant", UID: "parent-uid"}}
			job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", Namespace: joblifecycle.Namespace, UID: "job-uid"}}
			joblifecycle.Track(&job.ObjectMeta, parent, datamover.RestoreKind)
			client := fake.NewSimpleClientset(job)
			if scenario != "missing" {
				stored := parent.DeepCopy()
				if scenario == "replaced" {
					stored.UID = "replacement"
				}
				_, err := client.HarvesterhciV1beta1().VirtualMachineRestores(parent.Namespace).Create(context.Background(), stored, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			failure := errors.New("unavailable")
			if scenario == "API failure" {
				client.PrependReactor("get", "virtualmachinerestores", func(clienttesting.Action) (bool, runtime.Object, error) { return true, nil, failure })
			}
			cleaner := repositoryCleaner{client: fakeclients.KubernetesClient(client), restores: restoreLookup{client: client}}
			err := cleaner.cleanup(job.ObjectMeta)
			if scenario == "API failure" {
				require.ErrorIs(t, err, failure)
			} else {
				require.NoError(t, err)
			}
			jobs, err := client.BatchV1().Jobs(joblifecycle.Namespace).List(context.Background(), metav1.ListOptions{})
			require.NoError(t, err)
			if scenario == "exists" || scenario == "API failure" {
				require.Len(t, jobs.Items, 1)
			} else {
				require.Empty(t, jobs.Items)
			}
		})
	}
}

type restoreLookup struct {
	ctlharvesterv1.VirtualMachineRestoreClient
	client *fake.Clientset
}

func (r restoreLookup) Get(namespace, name string, opts metav1.GetOptions) (*harvesterv1.VirtualMachineRestore, error) {
	return r.client.HarvesterhciV1beta1().VirtualMachineRestores(namespace).Get(context.Background(), name, opts)
}

func TestDeletedParentCleansIndependentSnapshots(t *testing.T) {
	parent := &harvesterv1.VirtualMachineBackup{ObjectMeta: metav1.ObjectMeta{
		Name: "backup", Namespace: joblifecycle.Namespace, UID: "deleted-parent",
	}}
	original := &snapshotv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{
		Name: "original", Namespace: parent.Namespace, UID: "original-uid",
	}, Spec: snapshotv1.VolumeSnapshotSpec{Source: snapshotv1.VolumeSnapshotSource{PersistentVolumeClaimName: ptr.To("source")}}}
	imported := &snapshotv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{
		Name: "imported", Namespace: joblifecycle.Namespace, UID: "imported-uid",
	}, Spec: snapshotv1.VolumeSnapshotSpec{Source: snapshotv1.VolumeSnapshotSource{VolumeSnapshotContentName: ptr.To("imported")}}}
	content := &snapshotv1.VolumeSnapshotContent{ObjectMeta: metav1.ObjectMeta{Name: "imported", UID: "content-uid"},
		Spec: snapshotv1.VolumeSnapshotContentSpec{DeletionPolicy: snapshotv1.VolumeSnapshotContentRetain},
	}
	for _, meta := range []*metav1.ObjectMeta{&original.ObjectMeta, &imported.ObjectMeta, &content.ObjectMeta} {
		joblifecycle.Track(meta, parent, datamover.BackupKind)
	}
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, batchv1.AddToScheme(scheme))
	require.NoError(t, snapshotv1.AddToScheme(scheme))
	dynamic := dynamicfake.NewSimpleDynamicClient(scheme, original, imported, content)
	client := fake.NewSimpleClientset()
	cleaner := repositoryCleaner{
		client: fakeclients.KubernetesClient(client), snapshots: datamover.Snapshots{Client: dynamic},
		backups: fakeclients.VMBackupClient(client.HarvesterhciV1beta1().VirtualMachineBackups),
	}
	var err error
	for i := 0; i < 8; i++ {
		err = cleaner.deletedParent(parent.Namespace+"/"+parent.Name, datamover.BackupKind)
		if err == nil {
			break
		}
		require.ErrorIs(t, err, datamover.ErrPending)
	}
	require.NoError(t, err)
	var deleted []string
	for _, action := range dynamic.Actions() {
		if action.GetVerb() == "delete" {
			deleted = append(deleted, action.GetResource().Resource+"/"+action.(clienttesting.DeleteAction).GetName())
		}
	}
	require.Equal(t, []string{
		"volumesnapshots/imported", "volumesnapshotcontents/imported", "volumesnapshots/original",
	}, deleted)
}

func TestStagingClaimEnqueuesParentNamespace(t *testing.T) {
	parent := &harvesterv1.VirtualMachineRestore{ObjectMeta: metav1.ObjectMeta{
		Name: "restore", Namespace: "tenant", UID: "restore-uid",
	}}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "stage", Namespace: joblifecycle.Namespace}}
	joblifecycle.Track(&pvc.ObjectMeta, parent, datamover.RestoreKind)
	queue := &restoreQueue{}
	handler := RestoreHandler{vmrController: queue}
	_, err := handler.PersistentVolumeClaimOnChange("", pvc)
	require.NoError(t, err)
	require.Equal(t, []string{"tenant/restore"}, queue.keys)
}

type restoreQueue struct {
	ctlharvesterv1.VirtualMachineRestoreController
	keys []string
}

func (q *restoreQueue) Enqueue(namespace, name string) {
	q.keys = append(q.keys, namespace+"/"+name)
}
