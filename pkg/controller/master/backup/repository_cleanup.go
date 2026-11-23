package backup

import (
	"context"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util/datamover"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

// registerRepositoryCleanup recovers resources whose parent no longer exists.
// Normal deletion uses the engine's cleanup finalizer. These watches also cover
// forced parent removal and resources discovered when the controller starts.
func registerRepositoryCleanup(
	ctx context.Context, management *config.Management,
	controllers *backupControllerSet, snapshots dynamic.Interface,
) {
	restores := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore()
	cleaner := repositoryCleaner{
		client: management.ClientSet, snapshots: datamover.Snapshots{Client: snapshots},
		backups: controllers.vmbs, restores: restores,
	}
	controllers.vmbs.OnChange(ctx, "repository-deleted-backups", cleaner.onBackup)
	restores.OnChange(ctx, "repository-deleted-restores", cleaner.onRestore)
	controllers.jobs.OnChange(ctx, "repository-orphan-jobs", cleaner.onJob)
	controllers.pvcs.OnChange(ctx, "repository-orphan-claims", cleaner.onClaim)
	controllers.pvs.OnChange(ctx, "repository-orphan-volumes", cleaner.onVolume)
	controllers.vss.OnChange(ctx, "repository-orphan-snapshots", cleaner.onSnapshot)
	controllers.vscs.OnChange(ctx, "repository-orphan-contents", cleaner.onContent)
}

func (c repositoryCleaner) deletedParent(key, kind string) error {
	namespace, name, ok := strings.Cut(key, "/")
	if !ok || namespace == "" || name == "" {
		return nil
	}
	// Any remaining resource identifies the parent's immutable UID. Cleanup
	// checks the parent live, so a stale delete event cannot remove new work.
	resources := []schema.GroupVersionResource{
		{Group: "batch", Version: "v1", Resource: "jobs"},
		{Version: "v1", Resource: "persistentvolumeclaims"},
		{Version: "v1", Resource: "persistentvolumes"},
		{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshots"},
		{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshotcontents"},
	}
	for _, resource := range resources {
		if err := c.scanParent(resource, kind, namespace, name); err != nil {
			return err
		}
	}
	return nil
}

func (c repositoryCleaner) scanParent(
	resource schema.GroupVersionResource, kind, namespace, name string,
) error {
	list, err := c.listResources(resource, kind)
	if err != nil {
		return err
	}
	for i := range list.Items {
		if err := c.cleanupResource(&list.Items[i], namespace, name); err != nil {
			return err
		}
	}
	return nil
}

func (c repositoryCleaner) listResources(
	resource schema.GroupVersionResource, kind string,
) (*unstructured.UnstructuredList, error) {
	scope := ""
	if resource.Resource == "jobs" || resource.Resource == "persistentvolumeclaims" {
		scope = joblifecycle.Namespace
	}
	client := c.snapshots.Client.Resource(resource).Namespace(scope)
	return client.List(context.Background(), metav1.ListOptions{
		LabelSelector: joblifecycle.LabelParentKind + "=" + kind,
	})
}

func (c repositoryCleaner) cleanupResource(obj metav1.Object, namespace, name string) error {
	annotations := obj.GetAnnotations()
	if annotations[joblifecycle.AnnotationParentNamespace] != namespace ||
		annotations[joblifecycle.AnnotationParentName] != name {
		return nil
	}
	meta := metav1.ObjectMeta{
		Name: obj.GetName(), Namespace: obj.GetNamespace(), Labels: obj.GetLabels(),
		Annotations: annotations, OwnerReferences: obj.GetOwnerReferences(),
	}
	return c.cleanup(meta)
}

type repositoryCleaner struct {
	client    kubernetes.Interface
	snapshots datamover.Snapshots
	backups   ctlharvesterv1.VirtualMachineBackupClient
	restores  ctlharvesterv1.VirtualMachineRestoreClient
}

func (c repositoryCleaner) cleanup(meta metav1.ObjectMeta) error {
	parent, kind := resourceParent(meta)
	if parent == nil {
		return nil
	}
	exists, err := c.parentExists(parent, kind)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return c.cleanupOrphan(parent, kind)
}

func resourceParent(meta metav1.ObjectMeta) (*metav1.PartialObjectMetadata, string) {
	kind := meta.Labels[joblifecycle.LabelParentKind]
	name := joblifecycle.ParentName(meta, kind)
	namespace := meta.Annotations[joblifecycle.AnnotationParentNamespace]
	if name == "" || namespace == "" {
		return nil, kind
	}
	parent := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: namespace, UID: types.UID(meta.Labels[joblifecycle.LabelParentUID]),
	}}
	return parent, kind
}

func (c repositoryCleaner) cleanupOrphan(parent metav1.Object, kind string) error {
	if err := datamover.StopJobs(c.client, parent, kind); err != nil {
		return err
	}
	if kind == datamover.RestoreKind {
		return (datamover.Volumes{Client: c.client}).Cleanup(parent)
	}
	return c.cleanupBackup(parent)
}

// parentExists checks the API directly; a cache miss must never authorize deletion.
func (c repositoryCleaner) parentExists(parent metav1.Object, kind string) (bool, error) {
	var current metav1.Object
	var err error
	switch kind {
	case datamover.BackupKind:
		current, err = c.backups.Get(parent.GetNamespace(), parent.GetName(), metav1.GetOptions{})
	case datamover.RestoreKind:
		current, err = c.restores.Get(parent.GetNamespace(), parent.GetName(), metav1.GetOptions{})
	default:
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return current.GetUID() == parent.GetUID(), nil
}

func (c repositoryCleaner) cleanupBackup(parent metav1.Object) error {
	claims, err := c.client.CoreV1().PersistentVolumeClaims(joblifecycle.Namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: joblifecycle.Selector(parent, datamover.BackupKind),
	})
	if err != nil {
		return err
	}
	for _, claim := range claims.Items {
		if err := datamover.DeleteClaim(c.client, parent, datamover.BackupKind, claim.Name); err != nil {
			return err
		}
	}
	return c.snapshots.Cleanup(parent)
}
