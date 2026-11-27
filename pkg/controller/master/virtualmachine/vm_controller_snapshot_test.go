package virtualmachine

import (
	"context"
	"testing"

	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCleanupPVCAndSnapshotNamespaceIsolation(t *testing.T) {
	for _, namespace := range []string{"ns-a", "ns-b"} {
		t.Run(namespace, func(t *testing.T) {
			ctx := context.Background()
			client := fake.NewSimpleClientset()
			indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			backupCache := generic.NewCache[*harvesterv1.VirtualMachineBackup](indexer, schema.GroupResource{
				Group: harvesterv1.SchemeGroupVersion.Group, Resource: "virtualmachinebackups",
			})
			backupCache.AddIndexer(indexeres.VMBackupBySourceVMNameIndex, indexeres.VMBackupBySourceVMName)

			var targetVM *kubevirtv1.VirtualMachine
			var backups []*harvesterv1.VirtualMachineBackup
			for _, ns := range []string{"ns-a", "ns-b"} {
				vm := &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "test-vm",
						UID:       types.UID(ns + "-vm"),
					},
				}
				_, err := client.KubevirtV1().VirtualMachines(ns).Create(ctx, vm, metav1.CreateOptions{})
				require.NoError(t, err)
				if ns == namespace {
					targetVM = vm
				}

				backup := &harvesterv1.VirtualMachineBackup{
					ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "snapshot"},
					Spec: harvesterv1.VirtualMachineBackupSpec{
						Source: corev1.TypedLocalObjectReference{Kind: "VirtualMachine", Name: vm.Name},
						Type:   harvesterv1.Snapshot,
					},
					Status: harvesterv1.VirtualMachineBackupStatus{SourceUID: &vm.UID},
				}
				backup, err = client.HarvesterhciV1beta1().VirtualMachineBackups(ns).Create(ctx, backup, metav1.CreateOptions{})
				require.NoError(t, err)
				require.NoError(t, indexer.Add(backup))
				backups = append(backups, backup.DeepCopy())
			}

			// The production name index returns both namespaces; cleanup must filter them.
			matches, err := backupCache.GetByIndex(indexeres.VMBackupBySourceVMNameIndex, targetVM.Name)
			require.NoError(t, err)
			require.Len(t, matches, 2)

			h := &VMController{
				vmBackupCache:  backupCache,
				vmBackupClient: fakeclients.VMBackupClient(client.HarvesterhciV1beta1().VirtualMachineBackups),
			}
			_, err = h.cleanupPVCAndSnapshot(util.NamespacedName(targetVM.Namespace, targetVM.Name), targetVM)
			require.NoError(t, err)

			for _, backup := range backups {
				actual, err := client.HarvesterhciV1beta1().VirtualMachineBackups(backup.Namespace).Get(ctx, backup.Name, metav1.GetOptions{})
				if backup.Namespace == namespace {
					require.True(t, apierrors.IsNotFound(err), "target namespace's snapshot should be deleted: %v", err)
				} else {
					require.NoError(t, err)
					require.Equal(t, backup, actual, "other namespace's snapshot must remain unchanged")
				}
			}
		})
	}
}
