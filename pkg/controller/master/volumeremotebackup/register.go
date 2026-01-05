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
	"github.com/harvester/harvester/pkg/volumeremotebackup/common"
	"github.com/harvester/harvester/pkg/volumeremotebackup/driver"
	"github.com/harvester/harvester/pkg/volumeremotebackup/longhorn"
)

const (
	pvcbackupControllerName      = "pvc-backup-controller"
	volumeSnapshotControllerName = "volume-snapshot-controller"
	pvcrestoreControllerName     = "pvc-restore-controller"
	pvControllerName             = "pvc-controller"
)

type remoteBackupHandler struct {
	vrbController ctlharvesterv1.VolumeRemoteBackupController
	vrbCache      ctlharvesterv1.VolumeRemoteBackupCache
	bo            common.BackupOperator
	operations    map[harvesterv1.VolumeRemoteBackupType]driver.BackupOperation
}

type remoteRestoreHandler struct {
	vrrController ctlharvesterv1.VolumeRemoteRestoreController
	vrrCache      ctlharvesterv1.VolumeRemoteRestoreCache
	ro            common.RestoreOperator
	operations    map[harvesterv1.VolumeRemoteRestoreType]driver.RestoreOperation
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
func resolveOwnerRef(
	ns string,
	controllerRef *metav1.OwnerReference,
	expectedKind string,
	cache resourceCache,
) (string, string, bool) {
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
func resolveAnnotationRef(
	ns string,
	annotations map[string]string,
	annotationKey string,
	cache resourceCache,
) (string, string, bool) {
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

type remoteBackupCacheAdapter struct {
	cache interface {
		Get(namespace, name string) (*harvesterv1.VolumeRemoteBackup, error)
	}
}

func (a *remoteBackupCacheAdapter) Get(namespace, name string) (metav1.Object, error) {
	return a.cache.Get(namespace, name)
}

type remoteBackupResolver struct {
	handler *remoteBackupHandler
}

func (r *remoteBackupResolver) resolveRef(ns string, controllerRef *metav1.OwnerReference) (string, string, bool) {
	// Wrap the typed cache in an adapter that implements resourceCache
	cacheAdapter := &remoteBackupCacheAdapter{cache: r.handler.vrbCache}
	return resolveOwnerRef(ns, controllerRef, remoteBackupKind.Kind, cacheAdapter)
}

func (r *remoteBackupResolver) resolveAnnotation(ns string, annotations map[string]string) (string, string, bool) {
	// PVCBackup doesn't use annotation-based references
	return "", "", false
}

func (r *remoteBackupResolver) getResourceType() string {
	return remoteBackupKindName
}

func (r *remoteBackupResolver) enqueue(namespace, name string) {
	r.handler.vrbController.Enqueue(namespace, name)
}

// pvcRestoreCacheAdapter adapts PVCRestore cache to resourceCache interface
type pvcRestoreCacheAdapter struct {
	cache interface {
		Get(namespace, name string) (*harvesterv1.VolumeRemoteRestore, error)
	}
}

func (a *pvcRestoreCacheAdapter) Get(namespace, name string) (metav1.Object, error) {
	return a.cache.Get(namespace, name)
}

type remoteRestoreResolver struct {
	handler *remoteRestoreHandler
}

func (r *remoteRestoreResolver) resolveRef(ns string, controllerRef *metav1.OwnerReference) (string, string, bool) {
	// Wrap the typed cache in an adapter that implements resourceCache
	cacheAdapter := &pvcRestoreCacheAdapter{cache: r.handler.vrrCache}
	return resolveOwnerRef(ns, controllerRef, remoteRestoreKind.Kind, cacheAdapter)
}

func (r *remoteRestoreResolver) resolveAnnotation(ns string, annotations map[string]string) (string, string, bool) {
	// Check for the PVCRestore reference annotation
	annotationKey := driver.AnnotationPVCRestoreRef
	return resolveAnnotationRef(ns, annotations, annotationKey, &pvcRestoreCacheAdapter{cache: r.handler.vrrCache})
}

func (r *remoteRestoreResolver) getResourceType() string {
	return remoteRestoreKindName
}

func (r *remoteRestoreResolver) enqueue(namespace, name string) {
	r.handler.vrrController.Enqueue(namespace, name)
}

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	pbs := management.HarvesterFactory.Harvesterhci().V1beta1().VolumeRemoteBackup()
	vss := management.SnapshotFactory.Snapshot().V1().VolumeSnapshot()
	vsClasses := management.SnapshotFactory.Snapshot().V1().VolumeSnapshotClass()
	vscs := management.SnapshotFactory.Snapshot().V1().VolumeSnapshotContent()
	pvcs := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	scs := management.StorageFactory.Storage().V1().StorageClass()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	prs := management.HarvesterFactory.Harvesterhci().V1beta1().VolumeRemoteRestore()

	pbo := common.NewBackupOperator(pbs, pvcs.Cache(), scs.Cache(), settings.Cache())

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
	vrbs ctlharvesterv1.VolumeRemoteBackupController,
	vss ctlsnapshotv1.VolumeSnapshotController,
	vsClasses ctlsnapshotv1.VolumeSnapshotClassController,
	vscs ctlsnapshotv1.VolumeSnapshotContentController,
	pvcs ctlcorev1.PersistentVolumeClaimController,
	scs ctlstoragev1.StorageClassController,
	bo common.BackupOperator,
	vsh *volumeSnapshotHandler,
) {
	operations := map[harvesterv1.VolumeRemoteBackupType]driver.BackupOperation{
		harvesterv1.VolumeRemoteBackupLH: longhorn.GetLHBackupOperation(
			bo,
			vss.Cache(),
			vss,
			vsClasses.Cache(),
			vscs.Cache(),
			pvcs.Cache(),
			scs.Cache(),
		),
	}

	remotebackupHandler := &remoteBackupHandler{
		vrbController: vrbs,
		vrbCache:      vrbs.Cache(),
		bo:            bo,
		operations:    operations,
	}

	// Add the backup resolver to the shared handler
	vsh.resolvers = append(vsh.resolvers, &remoteBackupResolver{handler: remotebackupHandler})

	vrbs.OnChange(ctx, pvcbackupControllerName, remotebackupHandler.OnChanged)
	vrbs.OnRemove(ctx, pvcbackupControllerName, remotebackupHandler.OnRemove)
}

func registerRestoreHandlers(
	ctx context.Context,
	vrrs ctlharvesterv1.VolumeRemoteRestoreController,
	vrbs ctlharvesterv1.VolumeRemoteBackupController,
	vss ctlsnapshotv1.VolumeSnapshotController,
	vsClasses ctlsnapshotv1.VolumeSnapshotClassController,
	vscs ctlsnapshotv1.VolumeSnapshotContentController,
	pvcs ctlcorev1.PersistentVolumeClaimController,
	scs ctlstoragev1.StorageClassController,
	settings ctlharvesterv1.SettingController,
	bo common.BackupOperator,
	vsh *volumeSnapshotHandler,
	pvch *pvcHandler,
) {
	ro := common.NewRestoreOperator(vrrs, pvcs.Cache(), scs.Cache(), settings.Cache(), vrbs.Cache(), bo)

	operations := map[harvesterv1.VolumeRemoteRestoreType]driver.RestoreOperation{
		harvesterv1.VolumeRemoteRestoreLH: longhorn.GetLHRestoreOperation(
			ro,
			bo,
			vrbs.Cache(),
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

	remoteRestoreHandler := &remoteRestoreHandler{
		vrrController: vrrs,
		vrrCache:      vrrs.Cache(),
		ro:            ro,
		operations:    operations,
	}

	// Add the restore resolver to the shared handlers
	vsh.resolvers = append(vsh.resolvers, &remoteRestoreResolver{handler: remoteRestoreHandler})
	pvch.resolvers = append(pvch.resolvers, &remoteRestoreResolver{handler: remoteRestoreHandler})

	vrrs.OnChange(ctx, pvcrestoreControllerName, remoteRestoreHandler.OnChanged)
	vrrs.OnRemove(ctx, pvcrestoreControllerName, remoteRestoreHandler.OnRemove)
}
