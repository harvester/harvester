package longhorn

import (
	"context"
	"errors"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

type contentClient struct {
	ctlsnapshotv1.VolumeSnapshotContentClient
	client *fake.Clientset
}

func (c contentClient) Delete(name string, opts *metav1.DeleteOptions) error {
	return c.client.SnapshotV1().VolumeSnapshotContents().Delete(context.Background(), name, *opts)
}

func cleanupFixture() (
	*LonghornRestoreEngine, *harvesterv1.VirtualMachineRestore, *snapshotv1.VolumeSnapshotContent, *fake.Clientset,
) {
	vmr := &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "default", UID: "restore-uid"},
		Status: harvesterv1.VirtualMachineRestoreStatus{
			VolumeRestores: []harvesterv1.VolumeRestore{{VolumeBackupName: "volume"}},
		},
	}
	vsc := &snapshotv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{Name: "restore-default-restore-volume", UID: "content-uid"},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			Driver: longhornDriver, DeletionPolicy: snapshotv1.VolumeSnapshotContentRetain,
			VolumeSnapshotRef: corev1.ObjectReference{Namespace: "default", Name: "restore-restore-volume"},
		},
	}
	client := fake.NewSimpleClientset()
	engine := &LonghornRestoreEngine{
		vmro:      restorecommon.NewVMRestoreOperatorBuilder().Build(),
		vscCache:  fakeclients.VolumeSnapshotContentCache(client.SnapshotV1().VolumeSnapshotContents),
		vscClient: contentClient{client: client},
	}
	return engine, vmr, vsc, client
}

func TestDeleteWithoutSource(t *testing.T) {
	for _, scenario := range []string{
		"legacy", "recorded", "snapshot", "restic", "other-driver", "other-policy",
		"other-namespace", "other-snapshot", "absent",
	} {
		t.Run(scenario, func(t *testing.T) {
			engine, vmr, vsc, client := cleanupFixture()
			switch scenario {
			case "recorded":
				vmr.Annotations = map[string]string{restorecommon.BackupTypeAnnotation: string(harvesterv1.Backup)}
			case "snapshot", "restic":
				vmr.Annotations = map[string]string{restorecommon.BackupTypeAnnotation: scenario}
			case "other-driver":
				vsc.Spec.Driver = "other.csi.io"
			case "other-policy":
				vsc.Spec.DeletionPolicy = snapshotv1.VolumeSnapshotContentDelete
			case "other-namespace":
				vsc.Spec.VolumeSnapshotRef.Namespace = "other"
			case "other-snapshot":
				vsc.Spec.VolumeSnapshotRef.Name = "other"
			}
			if scenario != "absent" {
				_, err := client.SnapshotV1().VolumeSnapshotContents().Create(context.Background(), vsc, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			client.ClearActions()
			// No source VMBackup, VolumeSnapshot, or Longhorn engine is available.
			err := engine.Delete(vmr)
			require.NoError(t, err)
			err = engine.Delete(vmr)
			require.NoError(t, err)
			for _, action := range client.Actions() {
				if action.GetVerb() == "delete" {
					opts := action.(clienttesting.DeleteAction).GetDeleteOptions()
					require.Equal(t, vsc.UID, *opts.Preconditions.UID)
				}
			}
			_, err = client.SnapshotV1().VolumeSnapshotContents().Get(context.Background(), vsc.Name, metav1.GetOptions{})
			if scenario == "legacy" || scenario == "recorded" || scenario == "absent" {
				require.True(t, apierrors.IsNotFound(err))
				return
			}
			require.NoError(t, err, "unrelated content must remain")
		})
	}
}

func TestDeleteRetries(t *testing.T) {
	for _, verb := range []string{"get", "delete"} {
		t.Run(verb, func(t *testing.T) {
			engine, vmr, vsc, client := cleanupFixture()
			_, err := client.SnapshotV1().VolumeSnapshotContents().Create(context.Background(), vsc, metav1.CreateOptions{})
			require.NoError(t, err)
			apiErr := errors.New("API unavailable")
			client.PrependReactor(verb, "volumesnapshotcontents", func(clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, apiErr
			})
			err = engine.Delete(vmr)
			require.ErrorIs(t, err, apiErr)
		})
	}
}
