package pvcbackup

import (
	"context"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/pvcbackup/common"
	"github.com/harvester/harvester/pkg/pvcbackup/driver"
	"github.com/harvester/harvester/pkg/pvcbackup/longhorn"
)

const (
	pvcbackupControllerName      = "pvc-backup-controller"
	volumeSnapshotControllerName = "volume-snapshot-controller"
	pvcrestoreControllerName     = "pvc-restore-controller"
	pvControllerName             = "pvc-controller"
)

type pvcbackupHandler struct {
	pbController ctlharvesterv1.PVCBackupController
	pbCache      ctlharvesterv1.PVCBackupCache
	pbo          common.PVCBackupOperator
	operations   map[harvesterv1.PVCBackupType]driver.BackupOperation
}

type pvcrestoreHandler struct {
	prController ctlharvesterv1.PVCRestoreController
	prCache      ctlharvesterv1.PVCRestoreCache
	pro          common.PVCRestoreOperator
	operations   map[harvesterv1.PVCRestoreType]driver.RestoreOperation
}

type volumeSnapshotHandler struct {
	resolvers []ownerRefResolver
}

// pvcHandler reconciles PVCs and enqueues owners based on owner references
type pvcHandler struct {
	resolvers []ownerRefResolver
}

// ownerRefResolver resolves owner references and enqueues the owner for reconciliation
type ownerRefResolver interface {
	resolveRef(ns string, controllerRef *metav1.OwnerReference) (namespace string, name string, found bool)
	resolveAnnotation(ns string, annotations map[string]string) (namespace string, name string, found bool)
	getResourceType() string
	enqueue(namespace, name string)
}

// resourceCache is a generic interface for getting resources that have UIDs
type resourceCache interface {
	Get(namespace, name string) (metav1.Object, error)
}

// resolveOwnerRef is a generic helper to resolve owner references
func resolveOwnerRef(ns string, controllerRef *metav1.OwnerReference, expectedKind string, cache resourceCache) (string, string, bool) {
	if controllerRef.Kind != expectedKind {
		return "", "", false
	}

	obj, err := cache.Get(ns, controllerRef.Name)
	if err != nil {
		return "", "", false
	}

	if obj.GetUID() != controllerRef.UID {
		return "", "", false
	}

	return ns, controllerRef.Name, true
}

// resolveAnnotationRef is a generic helper to resolve annotation-based references
// It parses the annotation value in the format "namespace/name" or just "name" (uses ns param as namespace)
// and verifies the resource exists in the cache
func resolveAnnotationRef(ns string, annotations map[string]string, annotationKey string, cache resourceCache) (string, string, bool) {
	ref, exists := annotations[annotationKey]
	if !exists || ref == "" {
		return "", "", false
	}

	// Parse the reference in the format "namespace/name" or just "name"
	namespace, name := ns, ref
	for i := 0; i < len(ref); i++ {
		if ref[i] == '/' {
			namespace = ref[:i]
			name = ref[i+1:]
			break
		}
	}

	// Verify the resource exists in cache
	_, err := cache.Get(namespace, name)
	if err != nil {
		return "", "", false
	}

	return namespace, name, true
}

// pvcBackupCacheAdapter adapts PVCBackup cache to resourceCache interface
type pvcBackupCacheAdapter struct {
	cache interface {
		Get(namespace, name string) (*harvesterv1.PVCBackup, error)
	}
}

func (a *pvcBackupCacheAdapter) Get(namespace, name string) (metav1.Object, error) {
	return a.cache.Get(namespace, name)
}

// pvcBackupResolver implements volumeSnapshotRefResolver for PVCBackup
type pvcBackupResolver struct {
	handler *pvcbackupHandler
}

func (r *pvcBackupResolver) resolveRef(ns string, controllerRef *metav1.OwnerReference) (string, string, bool) {
	// Wrap the typed cache in an adapter that implements resourceCache
	cacheAdapter := &pvcBackupCacheAdapter{cache: r.handler.pbCache}
	return resolveOwnerRef(ns, controllerRef, pvcBackupKind.Kind, cacheAdapter)
}

func (r *pvcBackupResolver) resolveAnnotation(ns string, annotations map[string]string) (string, string, bool) {
	// PVCBackup doesn't use annotation-based references
	return "", "", false
}

func (r *pvcBackupResolver) getResourceType() string {
	return pvcBackupKindName
}

func (r *pvcBackupResolver) enqueue(namespace, name string) {
	r.handler.pbController.Enqueue(namespace, name)
}

// pvcRestoreCacheAdapter adapts PVCRestore cache to resourceCache interface
type pvcRestoreCacheAdapter struct {
	cache interface {
		Get(namespace, name string) (*harvesterv1.PVCRestore, error)
	}
}

func (a *pvcRestoreCacheAdapter) Get(namespace, name string) (metav1.Object, error) {
	return a.cache.Get(namespace, name)
}

// pvcRestoreResolver implements volumeSnapshotRefResolver for PVCRestore
type pvcRestoreResolver struct {
	handler *pvcrestoreHandler
}

func (r *pvcRestoreResolver) resolveRef(ns string, controllerRef *metav1.OwnerReference) (string, string, bool) {
	// Wrap the typed cache in an adapter that implements resourceCache
	cacheAdapter := &pvcRestoreCacheAdapter{cache: r.handler.prCache}
	return resolveOwnerRef(ns, controllerRef, pvcRestoreKind.Kind, cacheAdapter)
}

func (r *pvcRestoreResolver) resolveAnnotation(ns string, annotations map[string]string) (string, string, bool) {
	// Check for the PVCRestore reference annotation
	annotationKey := driver.AnnotationPVCRestoreRef
	return resolveAnnotationRef(ns, annotations, annotationKey, &pvcRestoreCacheAdapter{cache: r.handler.prCache})
}

func (r *pvcRestoreResolver) getResourceType() string {
	return pvcRestoreKindName
}

func (r *pvcRestoreResolver) enqueue(namespace, name string) {
	r.handler.prController.Enqueue(namespace, name)
}

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	pbs := management.HarvesterFactory.Harvesterhci().V1beta1().PVCBackup()
	vss := management.SnapshotFactory.Snapshot().V1().VolumeSnapshot()
	vsClasses := management.SnapshotFactory.Snapshot().V1().VolumeSnapshotClass()
	vscs := management.SnapshotFactory.Snapshot().V1().VolumeSnapshotContent()
	pvcs := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	scs := management.StorageFactory.Storage().V1().StorageClass()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	prs := management.HarvesterFactory.Harvesterhci().V1beta1().PVCRestore()

	pbo := common.GetPVCBackupOperator(pbs, pvcs.Cache(), scs.Cache(), settings.Cache())

	// Create shared volumeSnapshotHandler
	vsh := &volumeSnapshotHandler{
		resolvers: []ownerRefResolver{},
	}

	// Create shared pvcHandler
	pvch := &pvcHandler{
		resolvers: []ownerRefResolver{},
	}

	// Register backup and restore handlers, passing the shared handlers
	registerBackupHandlers(ctx, pbs, vss, vsClasses, vscs, pvcs, scs, pbo, vsh)
	registerRestoreHandlers(ctx, prs, pbs, vss, vsClasses, vscs, pvcs, scs, settings, pbo, vsh, pvch)

	// Register the shared volumeSnapshotHandler once
	vss.OnChange(ctx, volumeSnapshotControllerName, vsh.onChanged)

	// Register the shared pvcHandler once
	pvcs.OnChange(ctx, pvControllerName, pvch.onChanged)

	return nil
}

func registerBackupHandlers(
	ctx context.Context,
	pbs ctlharvesterv1.PVCBackupController,
	vss ctlsnapshotv1.VolumeSnapshotController,
	vsClasses ctlsnapshotv1.VolumeSnapshotClassController,
	vscs ctlsnapshotv1.VolumeSnapshotContentController,
	pvcs ctlcorev1.PersistentVolumeClaimController,
	scs ctlstoragev1.StorageClassController,
	pbo common.PVCBackupOperator,
	vsh *volumeSnapshotHandler,
) {
	operations := map[harvesterv1.PVCBackupType]driver.BackupOperation{
		harvesterv1.PVCBackupLH: longhorn.GetLHBackupOperation(
			pbo,
			vss.Cache(),
			vss,
			vsClasses.Cache(),
			vscs.Cache(),
			pvcs.Cache(),
			scs.Cache(),
		),
	}

	pvcbackupHandler := &pvcbackupHandler{
		pbController: pbs,
		pbCache:      pbs.Cache(),
		pbo:          pbo,
		operations:   operations,
	}

	// Add the backup resolver to the shared handler
	vsh.resolvers = append(vsh.resolvers, &pvcBackupResolver{handler: pvcbackupHandler})

	pbs.OnChange(ctx, pvcbackupControllerName, pvcbackupHandler.OnChanged)
	pbs.OnRemove(ctx, pvcbackupControllerName, pvcbackupHandler.OnRemove)
}

func registerRestoreHandlers(
	ctx context.Context,
	prs ctlharvesterv1.PVCRestoreController,
	pbs ctlharvesterv1.PVCBackupController,
	vss ctlsnapshotv1.VolumeSnapshotController,
	vsClasses ctlsnapshotv1.VolumeSnapshotClassController,
	vscs ctlsnapshotv1.VolumeSnapshotContentController,
	pvcs ctlcorev1.PersistentVolumeClaimController,
	scs ctlstoragev1.StorageClassController,
	settings ctlharvesterv1.SettingController,
	pbo common.PVCBackupOperator,
	vsh *volumeSnapshotHandler,
	pvch *pvcHandler,
) {
	pro := common.GetPVCRestoreOperator(prs, pvcs.Cache(), scs.Cache(), settings.Cache(), pbs.Cache(), pbo)

	operations := map[harvesterv1.PVCRestoreType]driver.RestoreOperation{
		harvesterv1.PVCRestoreLH: longhorn.GetLHRestoreOperation(
			pro,
			pbo,
			pbs.Cache(),
			vss.Cache(),
			vss,
			vsClasses.Cache(),
			vscs.Cache(),
			vscs,
			pvcs.Cache(),
			pvcs,
			scs.Cache(),
		),
	}

	pvcrestoreHandler := &pvcrestoreHandler{
		prController: prs,
		prCache:      prs.Cache(),
		pro:          pro,
		operations:   operations,
	}

	// Add the restore resolver to the shared handlers
	vsh.resolvers = append(vsh.resolvers, &pvcRestoreResolver{handler: pvcrestoreHandler})
	pvch.resolvers = append(pvch.resolvers, &pvcRestoreResolver{handler: pvcrestoreHandler})

	prs.OnChange(ctx, pvcrestoreControllerName, pvcrestoreHandler.OnChanged)
	prs.OnRemove(ctx, pvcrestoreControllerName, pvcrestoreHandler.OnRemove)
}
